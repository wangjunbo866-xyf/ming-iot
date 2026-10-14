package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"ming-iot/backend/tools/internal/thingsbaseline"
)

type commandOptions struct {
	LockPath       string
	Source         string
	Backend        string
	EvidencePath   string
	StructuralOnly bool
}

type cliExecutor interface {
	Execute(context.Context, string, commandOptions) ([]byte, error)
}

type defaultCLIExecutor struct{}

type importSummary struct {
	SourceFiles int                                    `json:"sourceFiles"`
	TargetFiles int                                    `json:"targetFiles"`
	TreeDigest  string                                 `json:"treeDigest"`
	Transform   thingsbaseline.TransformReport         `json:"transform"`
	Credentials thingsbaseline.CredentialProfileReport `json:"credentials"`
	Scan        thingsbaseline.SecurityScanReport      `json:"scan"`
	GoVersion   string                                 `json:"goVersion"`
}

func (defaultCLIExecutor) Execute(ctx context.Context, command string, options commandOptions) ([]byte, error) {
	lock, err := thingsbaseline.LoadLock(options.LockPath)
	if err != nil {
		return nil, err
	}
	switch command {
	case "import":
		snapshot, err := thingsbaseline.ReadSource(ctx, options.Source, lock)
		if err != nil {
			return nil, err
		}
		candidate, err := thingsbaseline.RenderCandidate(ctx, options.Backend, snapshot, lock)
		if err != nil {
			return nil, err
		}
		defer candidate.Cleanup()
		finalState, err := thingsbaseline.CaptureSourceRepositoryState(ctx, options.Source, lock)
		if err != nil {
			return nil, err
		}
		if finalState != snapshot.RepositoryState {
			return nil, fmt.Errorf("source repository changed before import publish")
		}
		report := candidate.Report()
		if err := candidate.Publish(); err != nil {
			return nil, err
		}
		return marshalCLIJSON(summarizeImportReport(len(snapshot.Files), report))
	case "verify":
		evidence, verifyErr := thingsbaseline.Verify(ctx, thingsbaseline.VerifyRequest{
			Source: options.Source, Backend: options.Backend, Lock: lock, StructuralOnly: options.StructuralOnly,
		})
		if err := thingsbaseline.WriteEvidenceAtomic(options.EvidencePath, evidence, []string{options.Source, options.Backend}); err != nil {
			return nil, fmt.Errorf("write verification evidence: %w", err)
		}
		output, err := thingsbaseline.MarshalEvidence(evidence)
		if err != nil {
			return nil, err
		}
		if verifyErr != nil {
			return output, verifyErr
		}
		return output, nil
	default:
		return nil, fmt.Errorf("unsupported command %q", command)
	}
}

func summarizeImportReport(sourceFiles int, report thingsbaseline.CandidateReport) importSummary {
	return importSummary{
		SourceFiles: sourceFiles,
		TargetFiles: len(report.Manifest),
		TreeDigest:  report.TreeDigest,
		Transform:   report.Transform,
		Credentials: report.Credentials,
		Scan:        report.Scan,
		GoVersion:   report.GoVersion,
	}
}

func marshalCLIJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal command result: %w", err)
	}
	return append(data, '\n'), nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWithExecutor(ctx, args, stdout, stderr, defaultCLIExecutor{})
}

func runWithExecutor(ctx context.Context, args []string, stdout, stderr io.Writer, executor cliExecutor) int {
	if len(args) == 0 || args[0] != "import" && args[0] != "verify" {
		fmt.Fprintln(stderr, "usage: thingsbaseline <import|verify> [flags]")
		return 2
	}
	command := args[0]
	options, err := parseCommandOptions(command, args[1:], stderr)
	if err != nil {
		fmt.Fprintf(stderr, "thingsbaseline %s: %v\n", command, err)
		return 2
	}
	output, err := executor.Execute(ctx, command, options)
	if err != nil {
		fmt.Fprintf(stderr, "thingsbaseline %s failed: %v\n", command, err)
		return 1
	}
	if len(output) > 0 {
		if _, err := stdout.Write(output); err != nil {
			fmt.Fprintf(stderr, "thingsbaseline %s output failed: %v\n", command, err)
			return 1
		}
	}
	return 0
}

func parseCommandOptions(command string, args []string, stderr io.Writer) (commandOptions, error) {
	if err := rejectDuplicateFlags(args); err != nil {
		return commandOptions{}, err
	}
	set := flag.NewFlagSet(command, flag.ContinueOnError)
	set.SetOutput(stderr)
	var options commandOptions
	set.StringVar(&options.LockPath, "lock", "", "path to things-baseline.lock")
	set.StringVar(&options.Source, "source", "", "path to the read-only source Git repository")
	set.StringVar(&options.Backend, "backend", "", "path to the Ming IoT backend directory")
	if command == "verify" {
		set.StringVar(&options.EvidencePath, "evidence", "", "path to the JSON evidence output")
		set.BoolVar(&options.StructuralOnly, "structural-only", false, "run structural diagnostics without final gates")
	}
	if err := set.Parse(args); err != nil {
		return commandOptions{}, err
	}
	if set.NArg() != 0 {
		return commandOptions{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(set.Args(), " "))
	}
	missing := make([]string, 0, 4)
	for _, required := range []struct{ name, value string }{
		{"--lock", options.LockPath}, {"--source", options.Source}, {"--backend", options.Backend},
	} {
		if required.value == "" {
			missing = append(missing, required.name)
		}
	}
	if command == "verify" && options.EvidencePath == "" {
		missing = append(missing, "--evidence")
	}
	if len(missing) != 0 {
		return commandOptions{}, fmt.Errorf("missing required flags: %s", strings.Join(missing, ", "))
	}
	return options, nil
}

func rejectDuplicateFlags(args []string) error {
	seen := make(map[string]struct{})
	for _, argument := range args {
		if argument == "--" {
			break
		}
		if !strings.HasPrefix(argument, "-") || argument == "-" {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimLeft(argument, "-"), "=")
		name = "--" + name
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate flag %s", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}
