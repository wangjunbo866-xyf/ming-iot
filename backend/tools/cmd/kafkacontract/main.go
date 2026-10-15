// kafkacontract 是 Kafka 仓库契约和显式集成探针的命令行入口。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ming-iot/backend/tools/internal/kafkacontract"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("kafkacontract", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repoRoot := flags.String("repo-root", ".", "repository root")
	format := flags.String("format", "json", "output format: json or text")
	integration := flags.Bool("integration", false, "run the explicit Kafka produce/consume probe")
	brokers := flags.String("brokers", "127.0.0.1:39092", "comma-separated integration broker addresses")
	timeout := flags.Duration("timeout", 30*time.Second, "integration probe timeout")
	evidenceOutput := flags.String("evidence-output", "", "optional private Evidence JSON output path")
	goalID := flags.String("goal-id", "", "LoopX goal ID required with evidence-output")
	todoID := flags.String("todo-id", "", "LoopX todo ID required with evidence-output")
	var verificationFlags verificationValues
	flags.Var(&verificationFlags, "verification", "repeatable name|pass-or-fail|relative-command verification summary")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *format != "json" && *format != "text" {
		fmt.Fprintln(stderr, "format must be json or text")
		return 2
	}
	absoluteRoot, err := filepath.Abs(*repoRoot)
	if err != nil {
		fmt.Fprintf(stderr, "resolve repository root: %v\n", err)
		return 2
	}

	report := kafkacontract.ValidateRepository(absoluteRoot)
	if !report.OK() {
		writeContractReport(stdout, report, *format)
		for _, issue := range report.Issues {
			fmt.Fprintf(stderr, "%s %s: %s\n", issue.Code, issue.Path, issue.Detail)
		}
		return 1
	}
	if !*integration {
		if err := publishEvidence(context.Background(), absoluteRoot, *evidenceOutput, *goalID, *todoID, report, nil, verificationFlags); err != nil {
			fmt.Fprintf(stderr, "write evidence: %v\n", err)
			return 1
		}
		writeContractReport(stdout, report, *format)
		return 0
	}

	document, err := kafkacontract.LoadCatalog(filepath.Join(absoluteRoot, "deploy", "kafka", "catalog", "data-plane.yaml"))
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", kafkacontract.CodeCatalogReadFailed, err)
		return 1
	}
	brokerList := splitBrokers(*brokers)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	probe, err := kafkacontract.RunProbe(ctx, kafkacontract.ProbeConfig{Brokers: brokerList, Topic: document.ProbeTopic.PhysicalName})
	if err != nil {
		if !strings.Contains(err.Error(), kafkacontract.CodeProbeUnavailable) {
			fmt.Fprintf(stderr, "%s: %v\n", kafkacontract.CodeProbeUnavailable, err)
		} else {
			fmt.Fprintln(stderr, err)
		}
		return 1
	}
	if err := publishEvidence(context.Background(), absoluteRoot, *evidenceOutput, *goalID, *todoID, report, &probe, verificationFlags); err != nil {
		fmt.Fprintf(stderr, "write evidence: %v\n", err)
		return 1
	}

	output := struct {
		Status   string                    `json:"status"`
		Contract kafkacontract.Report      `json:"contract"`
		Probe    kafkacontract.ProbeResult `json:"probe"`
	}{Status: "pass", Contract: report, Probe: probe}
	if *format == "text" {
		fmt.Fprintf(stdout, "Kafka contract PASS; probe topic=%s partition=%d offset=%d deleted=%t\n", probe.Topic, probe.Partition, probe.Offset, probe.ResourceDeleted)
		return 0
	}
	content, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "marshal integration output: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(content))
	return 0
}

type verificationValues []string

func (values *verificationValues) String() string {
	return strings.Join(*values, ",")
}

func (values *verificationValues) Set(value string) error {
	parts := strings.SplitN(value, "|", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || (parts[1] != "pass" && parts[1] != "fail") || strings.TrimSpace(parts[2]) == "" {
		return fmt.Errorf("verification must use name|pass-or-fail|relative-command")
	}
	*values = append(*values, value)
	return nil
}

func publishEvidence(ctx context.Context, repoRoot, outputPath, goalID, todoID string, report kafkacontract.Report, probe *kafkacontract.ProbeResult, values verificationValues) error {
	if outputPath == "" {
		return nil
	}
	if goalID == "" || todoID == "" {
		return fmt.Errorf("goal-id and todo-id are required with evidence-output")
	}
	source, err := kafkacontract.CollectSourceState(ctx, repoRoot)
	if err != nil {
		return err
	}
	verification := []kafkacontract.VerificationResult{{
		Name:    "repository-contract",
		Command: "go run ./tools/cmd/kafkacontract --repo-root ..",
		Result:  report.Status,
	}}
	for _, value := range values {
		parts := strings.SplitN(value, "|", 3)
		verification = append(verification, kafkacontract.VerificationResult{Name: parts[0], Result: parts[1], Command: parts[2]})
	}
	evidence, err := kafkacontract.BuildEvidence(kafkacontract.EvidenceInput{
		GoalID:       goalID,
		TodoID:       todoID,
		GeneratedAt:  time.Now().UTC(),
		Source:       source,
		Contract:     report,
		Verification: verification,
		Probe:        probe,
	})
	if err != nil {
		return err
	}
	return kafkacontract.WriteEvidence(outputPath, evidence)
}

func writeContractReport(output io.Writer, report kafkacontract.Report, format string) {
	if format == "text" {
		fmt.Fprintf(output, "Kafka contract %s; catalog=%d digest=%s issues=%d\n", strings.ToUpper(report.Status), report.CatalogEntries, report.CatalogDigest, len(report.Issues))
		return
	}
	content, err := report.JSON()
	if err != nil {
		fmt.Fprintf(output, "{\"status\":\"fail\",\"error\":%q}\n", err.Error())
		return
	}
	fmt.Fprintln(output, string(content))
}

func splitBrokers(value string) []string {
	parts := strings.Split(value, ",")
	brokers := make([]string, 0, len(parts))
	for _, part := range parts {
		if broker := strings.TrimSpace(part); broker != "" {
			brokers = append(brokers, broker)
		}
	}
	return brokers
}
