package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"ming-iot/backend/tools/internal/thingsbaseline"
)

func TestCLIRejectsInvalidCommandsAndArgumentsWithExitTwo(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"missing subcommand", nil},
		{"unknown subcommand", []string{"unknown"}},
		{"import missing lock", []string{"import", "--source", "/source", "--backend", "/backend"}},
		{"import missing source", []string{"import", "--lock", "lock", "--backend", "/backend"}},
		{"import missing backend", []string{"import", "--lock", "lock", "--source", "/source"}},
		{"verify missing evidence", []string{"verify", "--lock", "lock", "--source", "/source", "--backend", "/backend"}},
		{"unknown flag", []string{"import", "--lock", "lock", "--source", "/source", "--backend", "/backend", "--wat"}},
		{"duplicate flag", []string{"import", "--lock", "one", "--lock", "two", "--source", "/source", "--backend", "/backend"}},
		{"mixed-form duplicate flag", []string{"import", "-lock", "one", "--lock", "two", "--source", "/source", "--backend", "/backend"}},
		{"extra positional", []string{"import", "--lock", "lock", "--source", "/source", "--backend", "/backend", "extra"}},
		{"structural on import", []string{"import", "--lock", "lock", "--source", "/source", "--backend", "/backend", "--structural-only"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			executor := &recordingCLIExecutor{}
			if exitCode := runWithExecutor(context.Background(), tt.args, &stdout, &stderr, executor); exitCode != 2 {
				t.Fatalf("runWithExecutor() exit = %d, want 2; stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
			}
			if executor.calls != 0 || strings.Contains(stderr.String(), "/home/") {
				t.Fatalf("invalid CLI invoked executor or leaked path: calls=%d stderr=%q", executor.calls, stderr.String())
			}
		})
	}
}

func TestCLIParsesImportAndVerifyAndReturnsExecutorStatus(t *testing.T) {
	for _, fixture := range []struct {
		name           string
		args           []string
		wantCommand    string
		wantStructural bool
	}{
		{"import", []string{"import", "--lock", "lock.yaml", "--source", "/source", "--backend", "/backend"}, "import", false},
		{"verify", []string{"verify", "--lock", "lock.yaml", "--source", "/source", "--backend", "/backend", "--evidence", "/evidence/result.json"}, "verify", false},
		{"structural verify", []string{"verify", "--structural-only", "--lock", "lock.yaml", "--source", "/source", "--backend", "/backend", "--evidence", "/evidence/result.json"}, "verify", true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			executor := &recordingCLIExecutor{output: []byte("{\"result\":\"PASS\"}\n")}
			if exitCode := runWithExecutor(context.Background(), fixture.args, &stdout, &stderr, executor); exitCode != 0 {
				t.Fatalf("runWithExecutor() exit = %d, stderr=%q", exitCode, stderr.String())
			}
			if executor.calls != 1 || executor.command != fixture.wantCommand || executor.options.StructuralOnly != fixture.wantStructural {
				t.Fatalf("executor call = count %d command %q options %+v", executor.calls, executor.command, executor.options)
			}
			if executor.options.LockPath != "lock.yaml" || executor.options.Source != "/source" || executor.options.Backend != "/backend" || !bytes.Equal(stdout.Bytes(), executor.output) {
				t.Fatalf("parsed/output = %+v / %q", executor.options, stdout.String())
			}
		})
	}

	t.Run("execution failure", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		executor := &recordingCLIExecutor{err: errors.New("fixture failed")}
		args := []string{"import", "--lock", "lock", "--source", "/source", "--backend", "/backend"}
		if exitCode := runWithExecutor(context.Background(), args, &stdout, &stderr, executor); exitCode != 1 || !strings.Contains(stderr.String(), "fixture failed") {
			t.Fatalf("failure exit/stderr = %d/%q", exitCode, stderr.String())
		}
	})
}

func TestCLIPassesCancellationContextToExecutor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	executor := &recordingCLIExecutor{requireCancelled: true, err: context.Canceled}
	args := []string{"import", "--lock", "lock", "--source", "/source", "--backend", "/backend"}
	if exitCode := runWithExecutor(ctx, args, &bytes.Buffer{}, &bytes.Buffer{}, executor); exitCode != 1 || executor.calls != 1 {
		t.Fatalf("cancelled CLI exit/calls = %d/%d", exitCode, executor.calls)
	}
}

func TestCLIImportSummaryDoesNotPrintFullManifests(t *testing.T) {
	report := thingsbaseline.CandidateReport{
		PreTidyManifest: []thingsbaseline.ManifestEntry{{Path: "before.txt"}},
		Manifest:        []thingsbaseline.ManifestEntry{{Path: "after.txt"}, {Path: "second.txt"}},
		TreeDigest:      strings.Repeat("a", 64),
		Transform:       thingsbaseline.TransformReport{CorePrefixes: 94, ThingsPrefixes: 2726},
	}
	output, err := marshalCLIJSON(summarizeImportReport(976, report))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"preTidyManifest", "manifest\"", "before.txt", "after.txt", "second.txt"} {
		if bytes.Contains(output, []byte(forbidden)) {
			t.Fatalf("import summary retained %q: %s", forbidden, output)
		}
	}
	for _, required := range []string{"sourceFiles", "targetFiles", report.TreeDigest, "2726"} {
		if !bytes.Contains(output, []byte(required)) {
			t.Fatalf("import summary missed %q: %s", required, output)
		}
	}
}

type recordingCLIExecutor struct {
	calls            int
	command          string
	options          commandOptions
	output           []byte
	err              error
	requireCancelled bool
}

func (executor *recordingCLIExecutor) Execute(ctx context.Context, command string, options commandOptions) ([]byte, error) {
	executor.calls++
	executor.command = command
	executor.options = options
	if executor.requireCancelled && !errors.Is(ctx.Err(), context.Canceled) {
		return nil, errors.New("context was not cancelled")
	}
	return append([]byte(nil), executor.output...), executor.err
}
