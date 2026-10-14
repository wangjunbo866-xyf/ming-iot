package thingsbaseline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

func TestCompareTargetAcceptsIdenticalTree(t *testing.T) {
	_, target, candidate := verificationTargetFixture(t)
	comparison, err := compareCandidateTarget(candidate)
	if err != nil {
		t.Fatalf("compareCandidateTarget() error: %v", err)
	}
	if comparison.TotalDifferences != 0 || comparison.Truncated || len(comparison.Differences) != 0 {
		t.Fatalf("identical target comparison = %+v", comparison)
	}
	if comparison.TreeDigest != candidate.Report().TreeDigest || len(comparison.Manifest) != len(candidate.Report().Manifest) {
		t.Fatalf("target digest/manifest mismatch: %+v", comparison)
	}
	if _, err := os.Stat(filepath.Join(target, "LICENSE")); err != nil {
		t.Fatalf("comparison modified target: %v", err)
	}
}

func TestCompareTargetReportsStableBoundedDifferencesWithoutContent(t *testing.T) {
	_, target, candidate := verificationTargetFixture(t)
	if err := os.Remove(filepath.Join(target, filepath.FromSlash(testVetSourcePath))); err != nil {
		t.Fatal(err)
	}
	secretBody := "difference-body-must-not-leak"
	if err := os.WriteFile(filepath.Join(target, "source-reference.md"), []byte(secretBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(target, "NOTICE"), 0o755); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < maxReportedTreeDifferences+5; index++ {
		path := filepath.Join(target, fmt.Sprintf("z-extra-%03d.txt", index))
		if err := os.WriteFile(path, []byte("extra\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	comparison, err := compareCandidateTarget(candidate)
	if err != nil {
		t.Fatalf("compareCandidateTarget() error: %v", err)
	}
	if comparison.TotalDifferences != maxReportedTreeDifferences+8 || !comparison.Truncated || len(comparison.Differences) != maxReportedTreeDifferences {
		t.Fatalf("bounded comparison = total %d truncated %v reported %d", comparison.TotalDifferences, comparison.Truncated, len(comparison.Differences))
	}
	paths := make([]string, len(comparison.Differences))
	for index, difference := range comparison.Differences {
		paths[index] = difference.Path
	}
	if !sort.StringsAreSorted(paths) {
		t.Fatalf("differences are not path sorted: %v", paths)
	}
	encoded, err := json.Marshal(comparison.Differences)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(secretBody)) {
		t.Fatalf("difference report leaked target content: %s", encoded)
	}
	for _, path := range []string{"NOTICE", testVetSourcePath, "source-reference.md"} {
		if !containsDifferencePath(comparison.Differences, path) {
			t.Fatalf("bounded sorted differences missed early path %q: %v", path, paths)
		}
	}
}

func TestCompareTargetRejectsLinksAndConcurrentMetadataMutation(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		_, target, candidate := verificationTargetFixture(t)
		path := filepath.Join(target, filepath.FromSlash(testVetSourcePath))
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(target, "LICENSE"), path); err != nil {
			t.Fatal(err)
		}
		if _, err := compareCandidateTarget(candidate); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("symlink target comparison error = %v", err)
		}
	})

	t.Run("hardlink", func(t *testing.T) {
		_, target, candidate := verificationTargetFixture(t)
		if err := os.Link(filepath.Join(target, "LICENSE"), filepath.Join(target, "extra-hardlink")); err != nil {
			t.Fatal(err)
		}
		if _, err := compareCandidateTarget(candidate); err == nil || !strings.Contains(err.Error(), "hardlink") {
			t.Fatalf("hardlink target comparison error = %v", err)
		}
	})

	t.Run("mode mutation during scan", func(t *testing.T) {
		_, target, candidate := verificationTargetFixture(t)
		mutated := false
		candidate.policy.afterPathStat = func(relative string) {
			if mutated || relative != testVetSourcePath {
				return
			}
			mutated = true
			if err := os.Chmod(filepath.Join(target, filepath.FromSlash(testVetSourcePath)), 0o600); err != nil {
				panic(err)
			}
		}
		if _, err := compareCandidateTarget(candidate); err == nil || !strings.Contains(err.Error(), "metadata changed") {
			t.Fatalf("concurrent target mode mutation error = %v", err)
		}
	})
}

func TestVerifyRequiresTwoMatchingIndependentRenderReports(t *testing.T) {
	_, _, first := verificationTargetFixture(t)
	_, _, second := verificationTargetFixture(t)
	firstEvidence, secondEvidence, err := verifyRenderReproducibility(first, second)
	if err != nil {
		t.Fatalf("verifyRenderReproducibility() error: %v", err)
	}
	if firstEvidence.TreeSHA256 == "" || !reflect.DeepEqual(firstEvidence, secondEvidence) {
		t.Fatalf("render evidence differs: first=%+v second=%+v", firstEvidence, secondEvidence)
	}

	second.report.Commands[0].StderrSHA256 = strings.Repeat("f", 64)
	second.report.Commands[0].StderrTail = "independent cache download order"
	if _, _, err := verifyRenderReproducibility(first, second); err != nil {
		t.Fatalf("observational command logs must not define artifact reproducibility: %v", err)
	}

	second.report.TreeDigest = strings.Repeat("0", 64)
	if _, _, err := verifyRenderReproducibility(first, second); err == nil || !strings.Contains(err.Error(), "independent renders differ") {
		t.Fatalf("mismatched render reports error = %v", err)
	}
}

func TestVerificationIsolationTreeCopiesSealedCandidateWithoutReadingTarget(t *testing.T) {
	backend, target, candidate := verificationTargetFixture(t)
	isolation, err := createVerificationIsolation(candidate)
	if err != nil {
		t.Fatalf("createVerificationIsolation() error: %v", err)
	}
	defer isolation.Cleanup()
	if filepath.Dir(isolation.Path()) != backend || !strings.HasPrefix(filepath.Base(isolation.Path()), ".thingsbaseline-verify-") {
		t.Fatalf("isolation path escaped backend: %q", isolation.Path())
	}
	path := "main.go"
	targetBefore, err := os.ReadFile(filepath.Join(target, path))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(isolation.Path(), path), []byte("mutated isolation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := isolation.VerifyUnchanged(); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("mutated isolation VerifyUnchanged() error = %v", err)
	}
	targetAfter, err := os.ReadFile(filepath.Join(target, path))
	if err != nil || !bytes.Equal(targetBefore, targetAfter) {
		t.Fatalf("isolation mutation changed target: err=%v", err)
	}
	comparison, err := compareCandidateTarget(candidate)
	if err != nil || comparison.TotalDifferences != 0 {
		t.Fatalf("isolation mutation changed candidate/target comparison: %+v, %v", comparison, err)
	}
}

func TestVerifyGateSequenceStopsAndMarksRemainingNotRun(t *testing.T) {
	exitZero, exitFailure := 0, 9
	runner := &recordingGateRunner{results: map[string]GateResult{
		"one": {ID: "one", Result: ResultPass, Execution: ExecutionExited, ExitCode: &exitZero, LogSHA256: strings.Repeat("a", 64)},
		"two": {ID: "two", Result: ResultFail, Execution: ExecutionExited, ExitCode: &exitFailure, LogSHA256: strings.Repeat("b", 64)},
	}}
	specifications := []GateSpecification{
		{ID: "one", DisplayCommand: "command one"},
		{ID: "two", DisplayCommand: "command two"},
		{ID: "three", DisplayCommand: "command three"},
	}
	results := runGateSequence(context.Background(), runner, specifications, time.Minute)
	if strings.Join(runner.calls, ",") != "one,two" || len(results) != 3 {
		t.Fatalf("gate calls/results = %v / %+v", runner.calls, results)
	}
	if results[2].Result != ResultNotRun || results[2].Execution != ExecutionNotRun || results[2].ExitCode != nil || results[2].Command != "command three" {
		t.Fatalf("third gate was not stable NOT_RUN: %+v", results[2])
	}
}

func TestExecVerificationGateRunnerModelsExitSpawnTimeoutAndRedaction(t *testing.T) {
	runner := execVerificationGateRunner{}
	t.Run("non-zero exit", func(t *testing.T) {
		secret := "gate-secret-value"
		specification := GateSpecification{
			ID: "failure", DisplayCommand: "fixture command", Path: "/bin/sh",
			Args: []string{"-c", "printf '%s\\n' 'STDOUT-FAILURE-SENTINEL'; printf '%s\\n' \"$FIXTURE_SECRET\"; head -c 9000 /dev/zero | tr '\\0' x >&2; printf '%s\\n' 'STDERR-DOWNLOAD-SENTINEL' >&2; printf '%s\\n' \"$PWD\" >&2; exit 7"},
			Dir:  t.TempDir(), Env: []string{"PATH=/usr/bin:/bin", "FIXTURE_SECRET=" + secret},
		}
		result := runner.Run(context.Background(), specification, time.Second)
		if result.Result != ResultFail || result.Execution != ExecutionExited || result.ExitCode == nil || *result.ExitCode != 7 || len(result.LogSHA256) != 64 {
			t.Fatalf("non-zero gate result = %+v", result)
		}
		if !strings.Contains(result.Summary, "[stdout]") || !strings.Contains(result.Summary, "STDOUT-FAILURE-SENTINEL") || !strings.Contains(result.Summary, "[stderr]") || !strings.Contains(result.Summary, "STDERR-DOWNLOAD-SENTINEL") {
			t.Fatalf("gate summary lost diagnostic stream: %q", result.Summary)
		}
		if strings.Contains(result.Summary, secret) || strings.Contains(result.Summary, specification.Dir) || len(result.Summary) > maxGateSummaryBytes {
			t.Fatalf("gate summary was not bounded/redacted: %q", result.Summary)
		}
	})

	t.Run("secret crosses capture boundary", func(t *testing.T) {
		secret := strings.Repeat("s", 3000)
		specification := GateSpecification{
			ID: "boundary-secret", DisplayCommand: "boundary fixture", Path: "/bin/sh",
			Args: []string{"-c", "head -c 2000 /dev/zero | tr '\\0' p; printf '%s' \"$FIXTURE_SECRET\"; head -c 1500 /dev/zero | tr '\\0' t; exit 9"},
			Env:  []string{"PATH=/usr/bin:/bin", "FIXTURE_SECRET=" + secret},
		}
		result := runner.Run(context.Background(), specification, time.Second)
		if result.Result != ResultFail || result.Execution != ExecutionExited || result.ExitCode == nil || *result.ExitCode != 9 || len(result.LogSHA256) != 64 {
			t.Fatalf("boundary-secret gate result = %+v", result)
		}
		if !strings.Contains(result.Summary, "[stdout]") || len(result.Summary) > maxGateSummaryBytes {
			t.Fatalf("boundary-secret gate summary framing = %q", result.Summary)
		}
		if strings.Contains(result.Summary, strings.Repeat("s", 32)) {
			t.Fatalf("boundary-secret gate summary leaked a recognizable secret fragment: %q", result.Summary)
		}
	})

	t.Run("URL authority crosses capture boundary", func(t *testing.T) {
		specification := GateSpecification{
			ID: "boundary-url", DisplayCommand: "URL boundary fixture", Path: "/bin/sh",
			Args: []string{"-c", "printf 'https://'; head -c 5200 /dev/zero | tr '\\0' u; printf '@host.example/path/URL-PATH-SENTINEL\\n'; exit 8"},
			Env:  []string{"PATH=/usr/bin:/bin"},
		}
		result := runner.Run(context.Background(), specification, time.Second)
		if result.Result != ResultFail || result.Execution != ExecutionExited || result.ExitCode == nil || *result.ExitCode != 8 || len(result.LogSHA256) != 64 {
			t.Fatalf("boundary-URL gate result = %+v", result)
		}
		if !strings.Contains(result.Summary, "[stdout]") || !utf8.ValidString(result.Summary) || len(result.Summary) > maxGateSummaryBytes {
			t.Fatalf("boundary-URL gate summary framing = valid %v len %d summary %q", utf8.ValidString(result.Summary), len(result.Summary), result.Summary)
		}
		if strings.Contains(result.Summary, strings.Repeat("u", 32)) {
			t.Fatalf("boundary-URL gate summary leaked a recognizable credential fragment: %q", result.Summary)
		}
		if !strings.Contains(result.Summary, "https://[REDACTED]/path/URL-PATH-SENTINEL") {
			t.Fatalf("boundary-URL gate summary lost its useful scheme/path diagnostic: %q", result.Summary)
		}
	})

	t.Run("utf-8 tail", func(t *testing.T) {
		payload := strings.Repeat("界", 3000)
		specification := GateSpecification{
			ID: "utf8", DisplayCommand: "utf8 fixture", Path: "/bin/sh",
			Args: []string{"-c", "printf '%s' \"$UTF8_PAYLOAD\"; exit 6"},
			Env:  []string{"PATH=/usr/bin:/bin", "UTF8_PAYLOAD=" + payload},
		}
		result := runner.Run(context.Background(), specification, time.Second)
		if result.Result != ResultFail || result.Execution != ExecutionExited || result.ExitCode == nil || *result.ExitCode != 6 {
			t.Fatalf("utf-8 gate result = %+v", result)
		}
		if !strings.Contains(result.Summary, "[stdout]") || !utf8.ValidString(result.Summary) || len(result.Summary) > maxGateSummaryBytes {
			t.Fatalf("utf-8 gate summary is not valid bounded UTF-8: valid=%v len=%d summary=%q", utf8.ValidString(result.Summary), len(result.Summary), result.Summary)
		}
	})

	t.Run("spawn", func(t *testing.T) {
		missingPath := filepath.Join(t.TempDir(), "missing")
		result := runner.Run(context.Background(), GateSpecification{ID: "spawn", DisplayCommand: "missing", Path: missingPath}, time.Second)
		if result.Result != ResultFail || result.Execution != ExecutionSpawnError || result.ExitCode != nil {
			t.Fatalf("spawn gate result = %+v", result)
		}
		if !strings.Contains(result.Summary, "[runner]") || strings.Contains(result.Summary, "[stdout]") || strings.Contains(result.Summary, "[stderr]") {
			t.Fatalf("spawn gate summary sections = %q", result.Summary)
		}
		if strings.Contains(result.Summary, missingPath) {
			t.Fatalf("spawn gate summary leaked executable path: %q", result.Summary)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		result := runner.Run(context.Background(), GateSpecification{ID: "timeout", DisplayCommand: "sleep", Path: "/bin/sh", Args: []string{"-c", "sleep 30"}, Env: []string{"PATH=/usr/bin:/bin"}}, 20*time.Millisecond)
		if result.Result != ResultFail || result.Execution != ExecutionTimeout || result.ExitCode != nil {
			t.Fatalf("timeout gate result = %+v", result)
		}
	})
}

func TestExecVerificationGateRunnerRejectsMissingOrUnsafeExpectedOutputs(t *testing.T) {
	tests := []struct {
		name    string
		command string
		mutate  func(*gateOutputContract, string)
	}{
		{name: "exit zero without outputs", command: ":"},
		{name: "missing one", command: gateOutputWriteCommands("thingsvr", "dgsvr")},
		{name: "symlink", command: gateOutputWriteCommands("thingsvr", "dmsvr") + `; ln -s thingsvr "$OUTPUT/dgsvr"`},
		{name: "hardlink", command: `printf thingsvr > "$OUTPUT/thingsvr"; chmod 0755 "$OUTPUT/thingsvr"; ln "$OUTPUT/thingsvr" "$OUTPUT/dgsvr"; printf dmsvr > "$OUTPUT/dmsvr"; chmod 0755 "$OUTPUT/dmsvr"`},
		{name: "wrong mode", command: gateOutputWriteCommands("thingsvr", "dgsvr", "dmsvr") + `; chmod 0644 "$OUTPUT/dgsvr"`},
		{name: "setuid mode", command: gateOutputWriteCommands("thingsvr", "dgsvr", "dmsvr") + `; chmod 4755 "$OUTPUT/dgsvr"`},
		{name: "setgid mode", command: gateOutputWriteCommands("thingsvr", "dgsvr", "dmsvr") + `; chmod 2755 "$OUTPUT/dgsvr"`},
		{name: "sticky mode", command: gateOutputWriteCommands("thingsvr", "dgsvr", "dmsvr") + `; chmod 1755 "$OUTPUT/dgsvr"`},
		{name: "empty file", command: gateOutputWriteCommands("thingsvr", "dgsvr", "dmsvr") + `; : > "$OUTPUT/dgsvr"; chmod 0755 "$OUTPUT/dgsvr"`},
		{name: "unexpected extra contracted output", command: gateOutputWriteCommands("thingsvr", "dgsvr", "dmsvr", "unexpected")},
		{
			name:    "output mutated during scan",
			command: gateOutputWriteCommands("thingsvr", "dgsvr", "dmsvr"),
			mutate: func(contract *gateOutputContract, outputDirectory string) {
				contract.afterFirstRead = func(name string) {
					if name != "dgsvr" {
						return
					}
					contract.afterFirstRead = nil
					if err := os.WriteFile(filepath.Join(outputDirectory, name), []byte("other"), 0o755); err != nil {
						panic(err)
					}
				}
			},
		},
		{
			name:    "earlier output replaced while later output is scanned",
			command: gateOutputWriteCommands("thingsvr", "dgsvr", "dmsvr"),
			mutate: func(contract *gateOutputContract, outputDirectory string) {
				contract.afterFirstRead = func(name string) {
					if name != "dmsvr" {
						return
					}
					contract.afterFirstRead = nil
					path := filepath.Join(outputDirectory, "dgsvr")
					if err := os.Remove(path); err != nil {
						panic(err)
					}
					if err := os.WriteFile(path, []byte("other"), 0o755); err != nil {
						panic(err)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specification, outputDirectory := gateOutputFixture(t, tt.command)
			if tt.mutate != nil {
				tt.mutate(specification.outputContract, outputDirectory)
			}
			result := (execVerificationGateRunner{}).Run(context.Background(), specification, time.Second)
			if result.Result != ResultFail || len(result.Artifacts) != 0 {
				t.Fatalf("unsafe output gate result = %+v", result)
			}
			if result.Execution != ExecutionExited || result.ExitCode == nil || *result.ExitCode != 0 {
				t.Fatalf("postcondition failure process state = %+v", result)
			}
			if !strings.Contains(result.Summary, "[runner]") || strings.Contains(result.Summary, outputDirectory) || len(result.Summary) > maxGateSummaryBytes {
				t.Fatalf("postcondition summary is not bounded/redacted: %q", result.Summary)
			}
		})
	}
}

func TestExecVerificationGateRunnerCancelsDuringArtifactHashAndCleansOutputs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	specification, outputDirectory := gateOutputFixture(t, gateOutputWriteCommands("thingsvr", "dgsvr", "dmsvr"))
	specification.outputContract.afterFirstRead = func(string) {
		specification.outputContract.afterFirstRead = nil
		cancel()
	}

	result := (execVerificationGateRunner{}).Run(ctx, specification, time.Second)
	if result.Result != ResultFail || result.Execution != ExecutionCancelled || result.ExitCode != nil || len(result.Artifacts) != 0 {
		t.Fatalf("cancelled postcondition result = %+v", result)
	}
	assertContractedOutputsAbsent(t, outputDirectory)
}

func TestExecVerificationGateRunnerTimesOutDuringArtifactHashAndCleansOutputs(t *testing.T) {
	ctx := context.Background()
	specification, outputDirectory := gateOutputFixture(t, gateOutputWriteCommands("thingsvr", "dgsvr", "dmsvr"))
	deadlineObserved := make(chan struct{})
	specification.outputContract.afterFirstRead = func(string) {
		specification.outputContract.afterFirstRead = nil
		for ctx.Err() == nil {
			runtime.Gosched()
		}
		close(deadlineObserved)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()

	result := (execVerificationGateRunner{}).Run(ctx, specification, time.Second)
	if result.Result != ResultFail || result.Execution != ExecutionTimeout || result.ExitCode != nil || len(result.Artifacts) != 0 {
		t.Fatalf("parent-deadline postcondition result = %+v", result)
	}
	select {
	case <-deadlineObserved:
	default:
		t.Fatal("artifact hook did not observe the parent deadline")
	}
	assertContractedOutputsAbsent(t, outputDirectory)
}

func TestExecVerificationGateRunnerDoesNotPrepareOrSpawnAfterParentCancellation(t *testing.T) {
	specification, outputDirectory := gateOutputFixture(t, `: > "$MARKER"`)
	marker := filepath.Join(filepath.Dir(outputDirectory), "command-started")
	specification.Env = append(specification.Env, "MARKER="+marker)
	stale := filepath.Join(outputDirectory, "thingsvr")
	if err := os.WriteFile(stale, []byte("stale"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := (execVerificationGateRunner{}).Run(ctx, specification, time.Second)
	if result.Result != ResultFail || result.Execution != ExecutionCancelled || result.ExitCode != nil || len(result.Artifacts) != 0 {
		t.Fatalf("pre-cancelled result = %+v", result)
	}
	if got, err := os.ReadFile(stale); err != nil || string(got) != "stale" {
		t.Fatalf("preparation ran after cancellation: %q, %v", got, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("command ran after cancellation: %v", err)
	}
}

func TestExecVerificationGateRunnerCleansFailedOutputsAndPreservesUnrelatedEntries(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		exitCode int
	}{
		{name: "nonzero build", command: gateOutputWriteCommands("thingsvr", "dgsvr", "dmsvr") + "; exit 17", exitCode: 17},
		{name: "malformed exit-zero build", command: gateOutputWriteCommands("thingsvr", "dgsvr", "dmsvr") + `; chmod 0644 "$OUTPUT/dgsvr"`, exitCode: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specification, outputDirectory := gateOutputFixture(t, tt.command)
			unrelated := filepath.Join(outputDirectory, "keep-me")
			if err := os.WriteFile(unrelated, []byte("unrelated"), 0o600); err != nil {
				t.Fatal(err)
			}

			result := (execVerificationGateRunner{}).Run(context.Background(), specification, time.Second)
			if result.Result != ResultFail || result.Execution != ExecutionExited || result.ExitCode == nil || *result.ExitCode != tt.exitCode || len(result.Artifacts) != 0 {
				t.Fatalf("failed build result = %+v", result)
			}
			assertContractedOutputsAbsent(t, outputDirectory)
			if got, err := os.ReadFile(unrelated); err != nil || string(got) != "unrelated" {
				t.Fatalf("cleanup removed or changed unrelated output: %q, %v", got, err)
			}
		})
	}
}

func TestExecVerificationGateRunnerCleansPreparedOutputsAfterSpawnError(t *testing.T) {
	specification, outputDirectory := gateOutputFixture(t, ":")
	for _, name := range []string{"thingsvr", "dgsvr", "dmsvr"} {
		if err := os.WriteFile(filepath.Join(outputDirectory, name), []byte("stale"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	specification.Path = filepath.Join(filepath.Dir(outputDirectory), "missing-command")
	result := (execVerificationGateRunner{}).Run(context.Background(), specification, time.Second)
	if result.Result != ResultFail || result.Execution != ExecutionSpawnError || result.ExitCode != nil || len(result.Artifacts) != 0 {
		t.Fatalf("spawn-error result = %+v", result)
	}
	assertContractedOutputsAbsent(t, outputDirectory)
}

func TestExecVerificationGateRunnerReportsCleanupFailureWithoutLeakingPath(t *testing.T) {
	command := `mkdir "$OUTPUT/thingsvr"; ` + gateOutputWriteCommands("dgsvr", "dmsvr")
	specification, outputDirectory := gateOutputFixture(t, command)
	result := (execVerificationGateRunner{}).Run(context.Background(), specification, time.Second)
	if result.Result != ResultFail || len(result.Artifacts) != 0 || !strings.Contains(result.Summary, "cleanup build outputs") {
		t.Fatalf("cleanup-failure result = %+v", result)
	}
	if strings.Contains(result.Summary, filepath.Dir(outputDirectory)) || len(result.Summary) > maxGateSummaryBytes {
		t.Fatalf("cleanup diagnostic is not bounded/redacted: %q", result.Summary)
	}
}

func TestExecVerificationGateRunnerRemovesOnlyStaleContractedOutputsAndReturnsEvidence(t *testing.T) {
	command := `for name in thingsvr dgsvr dmsvr; do if [ -e "$OUTPUT/$name" ] || [ -L "$OUTPUT/$name" ]; then exit 23; fi; done; ` + gateOutputWriteCommands("dmsvr", "thingsvr", "dgsvr")
	specification, outputDirectory := gateOutputFixture(t, command)
	for _, name := range []string{"thingsvr", "dgsvr", "dmsvr"} {
		if err := os.WriteFile(filepath.Join(outputDirectory, name), []byte("stale-"+name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	unrelated := filepath.Join(outputDirectory, "keep-me")
	if err := os.WriteFile(unrelated, []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Exact postcondition rejects the unrelated entry, but preparation must not
	// remove it or let the command observe any of the three stale outputs.
	result := (execVerificationGateRunner{}).Run(context.Background(), specification, time.Second)
	if result.Result != ResultFail || result.Execution != ExecutionExited || result.ExitCode == nil || *result.ExitCode != 0 {
		t.Fatalf("extra output gate result = %+v", result)
	}
	if got, err := os.ReadFile(unrelated); err != nil || string(got) != "unrelated" {
		t.Fatalf("preparation removed or changed unrelated output: %q, %v", got, err)
	}
	if err := os.Remove(unrelated); err != nil {
		t.Fatal(err)
	}

	specification, _ = gateOutputFixtureAt(t, outputDirectory, command)
	result = (execVerificationGateRunner{}).Run(context.Background(), specification, time.Second)
	if result.Result != ResultPass || result.Execution != ExecutionExited || result.ExitCode == nil || *result.ExitCode != 0 {
		t.Fatalf("fresh output gate result = %+v", result)
	}
	wantNames := []string{"dgsvr", "dmsvr", "thingsvr"}
	if len(result.Artifacts) != len(wantNames) {
		t.Fatalf("artifact count = %d, want %d: %+v", len(result.Artifacts), len(wantNames), result.Artifacts)
	}
	for index, name := range wantNames {
		artifact := result.Artifacts[index]
		if artifact.LogicalPath != "backend/bin/"+name || artifact.Mode != "0755" || artifact.Size != int64(len(name)) || artifact.SHA256 != sha256Digest([]byte(name)) || artifact.GoVersion != lockedGoVersion {
			t.Fatalf("artifact[%d] = %+v", index, artifact)
		}
	}
}

func TestExecVerificationGateRunnerCreatesMissingOutputDirectoryAndPasses(t *testing.T) {
	root := t.TempDir()
	outputDirectory := filepath.Join(root, "bin")
	command := `test -d "$OUTPUT" || exit 41; ` + gateOutputWriteCommands("thingsvr", "dgsvr", "dmsvr")
	specification := newGateOutputSpecification(outputDirectory, command)
	specification.RedactValues = []string{root}

	result := (execVerificationGateRunner{}).Run(context.Background(), specification, time.Second)
	if result.Result != ResultPass || result.Execution != ExecutionExited || result.ExitCode == nil || *result.ExitCode != 0 || len(result.Artifacts) != 3 {
		t.Fatalf("missing output directory gate result = %+v", result)
	}
	info, err := os.Lstat(outputDirectory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("created output directory = %+v, %v", info, err)
	}
}

func TestExecVerificationGateRunnerRejectsSymlinkOutputDirectoryBeforeSpawn(t *testing.T) {
	root := t.TempDir()
	realDirectory := filepath.Join(root, "real-bin")
	linkedDirectory := filepath.Join(root, "linked-bin")
	if err := os.Mkdir(realDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDirectory, linkedDirectory); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "command-started")
	specification := newGateOutputSpecification(linkedDirectory, `: > "$MARKER"`)
	specification.Env = append(specification.Env, "MARKER="+marker)
	specification.RedactValues = append(specification.RedactValues, root)
	result := (execVerificationGateRunner{}).Run(context.Background(), specification, time.Second)
	if result.Result != ResultFail || result.Execution != ExecutionSpawnError || result.ExitCode != nil || len(result.Artifacts) != 0 {
		t.Fatalf("symlink output directory result = %+v", result)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("command ran despite unsafe output directory: %v", err)
	}
	if !strings.Contains(result.Summary, "[runner]") || strings.Contains(result.Summary, root) || len(result.Summary) > maxGateSummaryBytes {
		t.Fatalf("prepare summary is not bounded/redacted: %q", result.Summary)
	}
}

func TestExecVerificationGateRunnerPinsMakeToAbsoluteGoDespitePATH(t *testing.T) {
	root := t.TempDir()
	outputDirectory := filepath.Join(root, "bin")
	fakeDirectory := filepath.Join(root, "fake")
	hostileMarker := filepath.Join(root, "hostile-make-ran")
	hostileMakefile := filepath.Join(root, "hostile.mk")
	writeMinimalThingsBuildFixture(t, root)
	if err := os.Mkdir(outputDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(fakeDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeMarker := filepath.Join(root, "fake-go-selected")
	if err := os.WriteFile(filepath.Join(fakeDirectory, "go"), []byte(fmt.Sprintf("#!/bin/sh\n: > %q\nexit 0\n", fakeMarker)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostileMakefile, []byte(fmt.Sprintf("$(shell /usr/bin/touch %s)\nGO_BUILD := /usr/bin/touch %s\n", hostileMarker, hostileMarker)), 0o644); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"PATH": fakeDirectory + ":/usr/bin:/bin", "MAKEFILES": hostileMakefile, "MAKEFLAGS": "-e -f " + hostileMakefile,
		"GNUMAKEFLAGS": "-e", "MFLAGS": "-e", "GO_BUILD": "/usr/bin/touch " + hostileMarker,
		"BASH_ENV": hostileMakefile, "ENV": hostileMakefile, "CDPATH": root, "TASK1_HOSTILE_INHERITED": "must-not-pass",
	} {
		t.Setenv(key, value)
	}
	tool, err := openAttestedGoTool(context.Background(), filepath.Join(runtime.GOROOT(), "bin", "go"))
	if err != nil {
		t.Fatal(err)
	}
	defer tool.Close()
	makeEnvironment, err := strictMakeEnvironment(root, testSharedGoCachePaths(t))
	if err != nil {
		t.Fatal(err)
	}
	specification := GateSpecification{
		ID: "things-build", DisplayCommand: "cd verification-tree && make BIN_DIR=backend/bin build", Path: "/usr/bin/make",
		Args: []string{"GO=/proc/self/fd/3", "BIN_DIR=" + outputDirectory, "build"}, Dir: root,
		Env: makeEnvironment, RedactValues: []string{root}, goTool: tool,
		outputContract: &gateOutputContract{directory: outputDirectory, logicalRoot: "backend/bin", names: []string{"thingsvr", "dgsvr", "dmsvr"}, mode: 0o755},
	}
	result := (execVerificationGateRunner{}).Run(context.Background(), specification, 30*time.Second)
	if result.Result != ResultPass || len(result.Artifacts) != 3 {
		t.Fatalf("pinned make gate result = %+v", result)
	}
	if _, err := os.Stat(fakeMarker); !os.IsNotExist(err) {
		t.Fatalf("fake go from PATH was selected: %v", err)
	}
	if _, err := os.Stat(hostileMarker); !os.IsNotExist(err) {
		t.Fatalf("hostile make environment executed injected content: %v", err)
	}
}

func TestStrictMakeEnvironmentPreservesOptionalTransportProxies(t *testing.T) {
	t.Run("configured", func(t *testing.T) {
		t.Setenv("MING_GOPROXY", "")
		for key, value := range map[string]string{
			"HTTP_PROXY":  "http://127.0.0.1:7890",
			"HTTPS_PROXY": "http://127.0.0.1:7890",
			"http_proxy":  "http://127.0.0.1:7891",
			"https_proxy": "http://127.0.0.1:7891",
		} {
			t.Setenv(key, value)
		}
		environment, err := strictMakeEnvironment(t.TempDir(), testSharedGoCachePaths(t))
		if err != nil {
			t.Fatalf("strictMakeEnvironment() error: %v", err)
		}
		env := environmentMap(environment)
		for key, want := range map[string]string{
			"HTTP_PROXY":  "http://127.0.0.1:7890",
			"HTTPS_PROXY": "http://127.0.0.1:7890",
			"http_proxy":  "http://127.0.0.1:7891",
			"https_proxy": "http://127.0.0.1:7891",
		} {
			if got := env[key]; got != want {
				t.Fatalf("strict make env[%s] = %q, want %q", key, got, want)
			}
		}
		if _, ok := env["MING_GOPROXY"]; ok {
			t.Fatalf("strict make environment retained MING_GOPROXY")
		}
		if got := env["GOPROXY"]; got != defaultCandidateGoProxy {
			t.Fatalf("strict make env[GOPROXY] = %q, want %q", got, defaultCandidateGoProxy)
		}
	})

	t.Run("absent", func(t *testing.T) {
		t.Setenv("MING_GOPROXY", "")
		for key := range map[string]struct{}{
			"HTTP_PROXY": {}, "HTTPS_PROXY": {}, "http_proxy": {}, "https_proxy": {},
		} {
			key := key
			old, existed := os.LookupEnv(key)
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if existed {
					_ = os.Setenv(key, old)
				} else {
					_ = os.Unsetenv(key)
				}
			})
		}
		environment, err := strictMakeEnvironment(t.TempDir(), testSharedGoCachePaths(t))
		if err != nil {
			t.Fatalf("strictMakeEnvironment() error without transport proxies: %v", err)
		}
		env := environmentMap(environment)
		for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
			if _, ok := env[key]; ok {
				t.Fatalf("strict make environment synthesized absent %s", key)
			}
		}
	})
}

func TestDefaultGateSpecificationsRedactInheritedTransportProxies(t *testing.T) {
	t.Setenv("MING_GOPROXY", "")
	proxies := map[string]string{
		"HTTP_PROXY":  "upper-http-user:upper-http-password@proxy.example.invalid/upper-http-path-secret?token=upper-http-query-secret",
		"HTTPS_PROXY": "proxy.example.invalid/upper-https-path-secret?credential=upper-https-query-secret",
		"http_proxy":  "http://lower-http.example.invalid/lower-http-path-secret?token=lower-http-query-secret",
		"https_proxy": "https://lower-https.example.invalid/lower-https-path-secret?token=lower-https-query-secret",
	}
	for key, value := range proxies {
		t.Setenv(key, value)
	}
	backend, _, candidate := verificationTargetFixture(t)
	isolation, err := createVerificationIsolation(candidate)
	if err != nil {
		t.Fatal(err)
	}
	defer isolation.Cleanup()
	specifications, cleanup, err := buildDefaultVerificationGates(VerifyRequest{Backend: backend}, isolation)
	if err != nil {
		t.Fatalf("buildDefaultVerificationGates() error: %v", err)
	}
	defer cleanup()

	for _, specification := range specifications {
		environment := environmentMap(specification.Env)
		for key, proxy := range proxies {
			if got := environment[key]; got != proxy {
				t.Errorf("gate %s environment[%s] = %q, want inherited proxy", specification.ID, key, got)
			}
			if !slices.Contains(specification.RedactValues, proxy) {
				t.Errorf("gate %s RedactValues omitted complete %s value", specification.ID, key)
			}
		}
	}

	for _, index := range []int{0, len(specifications) - 1} {
		specification := specifications[index]
		specification.ID += "-proxy-echo"
		specification.DisplayCommand = "echo inherited transport proxies"
		specification.Path = "/bin/sh"
		specification.Dir = isolation.Path()
		specification.Args = []string{"-c", `printf '%s\n%s\n' "$HTTP_PROXY" "$https_proxy"; printf '%s\n%s\n' "$HTTPS_PROXY" "$http_proxy" >&2; exit 31`}
		specification.goTool = nil
		specification.outputContract = nil
		result := (execVerificationGateRunner{}).Run(context.Background(), specification, time.Second)
		if result.Result != ResultFail || result.Execution != ExecutionExited || result.ExitCode == nil || *result.ExitCode != 31 {
			t.Fatalf("proxy echo gate result = %+v", result)
		}
		if !strings.Contains(result.Summary, "[REDACTED]") {
			t.Fatalf("proxy echo gate %s did not record redaction: %q", specification.ID, result.Summary)
		}
		for _, proxy := range proxies {
			if strings.Contains(result.Summary, proxy) {
				t.Errorf("proxy echo gate %s leaked complete transport proxy value %q: %q", specification.ID, proxy, result.Summary)
			}
		}
		for _, fragment := range []string{
			"upper-http-password", "upper-http-path-secret", "upper-http-query-secret",
			"upper-https-path-secret", "upper-https-query-secret",
			"lower-http-path-secret", "lower-http-query-secret",
			"lower-https-path-secret", "lower-https-query-secret",
		} {
			if strings.Contains(result.Summary, fragment) {
				t.Errorf("proxy echo gate %s leaked transport proxy fragment %q: %q", specification.ID, fragment, result.Summary)
			}
		}
	}
}

func TestDefaultGateSpecificationsRedactSharedGoCachesFromEvidence(t *testing.T) {
	backend, _, candidate := verificationTargetFixture(t)
	isolation, err := createVerificationIsolation(candidate)
	if err != nil {
		t.Fatal(err)
	}
	defer isolation.Cleanup()
	specifications, cleanup, err := buildDefaultVerificationGates(VerifyRequest{Backend: backend}, isolation)
	if err != nil {
		t.Fatalf("buildDefaultVerificationGates() error: %v", err)
	}
	defer cleanup()

	specification := specifications[0]
	specification.ID += "-cache-echo"
	specification.Path = "/bin/sh"
	specification.Dir = isolation.Path()
	specification.Args = []string{"-c", `printf '%s\n%s\n' "$GOCACHE" "$GOMODCACHE"; exit 31`}
	specification.goTool = nil
	specification.outputContract = nil
	result := (execVerificationGateRunner{}).Run(context.Background(), specification, time.Second)
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result != ResultFail || result.Execution != ExecutionExited || result.ExitCode == nil || *result.ExitCode != 31 {
		t.Fatalf("cache echo gate result = %+v", result)
	}
	if !strings.Contains(string(encoded), "[REDACTED]") {
		t.Fatalf("cache echo gate did not record redaction: %s", encoded)
	}
	environment := environmentMap(specification.Env)
	for _, key := range []string{"GOCACHE", "GOMODCACHE"} {
		cachePath := environment[key]
		if cachePath == "" {
			t.Fatalf("cache echo gate environment[%s] is empty", key)
		}
		if strings.Contains(string(encoded), cachePath) {
			t.Fatalf("cache echo gate leaked %s path %q: %s", key, cachePath, encoded)
		}
	}
}

func TestExecVerificationGateRunnerBuildPassesWithRestrictiveUmask(t *testing.T) {
	const helper = "TASK1_RESTRICTIVE_UMASK_HELPER"
	const rootEnvironment = "TASK1_RESTRICTIVE_UMASK_ROOT"
	if os.Getenv(helper) == "1" {
		root := os.Getenv(rootEnvironment)
		oldUmask := syscall.Umask(0o077)
		defer syscall.Umask(oldUmask)
		specification, closeTool := realThingsBuildSpecification(t, root)
		defer closeTool()
		result := (execVerificationGateRunner{}).Run(context.Background(), specification, 30*time.Second)
		if result.Result != ResultPass || len(result.Artifacts) != 3 {
			t.Fatalf("restrictive-umask build result = %+v", result)
		}
		for _, name := range []string{"thingsvr", "dgsvr", "dmsvr"} {
			var stat syscall.Stat_t
			if err := syscall.Lstat(filepath.Join(root, "bin", name), &stat); err != nil {
				t.Fatal(err)
			}
			if got := uint32(stat.Mode) & 0o7777; got != 0o755 {
				t.Fatalf("%s mode = %04o, want 0755", name, got)
			}
		}
		return
	}

	root := t.TempDir()
	writeMinimalThingsBuildFixture(t, root)
	command := exec.Command(os.Args[0], "-test.run=^TestExecVerificationGateRunnerBuildPassesWithRestrictiveUmask$")
	command.Env = append(os.Environ(), helper+"=1", rootEnvironment+"="+root)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("restrictive-umask helper failed: %v\n%s", err, output)
	}
}

func TestExecVerificationGateRunnerBuildIgnoresInvalidAncestorGit(t *testing.T) {
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "fixture")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMinimalThingsBuildFixture(t, root)
	specification, closeTool := realThingsBuildSpecification(t, root)
	defer closeTool()
	result := (execVerificationGateRunner{}).Run(context.Background(), specification, 30*time.Second)
	if result.Result != ResultPass || len(result.Artifacts) != 3 {
		t.Fatalf("build beneath unrelated Git metadata = %+v", result)
	}
}

func realThingsBuildSpecification(t *testing.T, root string) (GateSpecification, func()) {
	t.Helper()
	tool, err := openAttestedGoTool(context.Background(), filepath.Join(runtime.GOROOT(), "bin", "go"))
	if err != nil {
		t.Fatal(err)
	}
	environment, err := strictMakeEnvironment(root, testSharedGoCachePaths(t))
	if err != nil {
		_ = tool.Close()
		t.Fatal(err)
	}
	outputDirectory := filepath.Join(root, "bin")
	return GateSpecification{
		ID: "things-build", DisplayCommand: "cd verification-tree && make BIN_DIR=backend/bin build", Path: "/usr/bin/make",
		Args: []string{"GO=/proc/self/fd/3", "BIN_DIR=" + outputDirectory, "build"}, Dir: root,
		Env: environment, RedactValues: []string{root}, goTool: tool,
		outputContract: &gateOutputContract{directory: outputDirectory, logicalRoot: "backend/bin", names: []string{"thingsvr", "dgsvr", "dmsvr"}, mode: 0o755},
	}, func() { _ = tool.Close() }
}

func writeMinimalThingsBuildFixture(t *testing.T, root string) {
	t.Helper()
	// Give Go's VCS discovery a fixture-owned boundary instead of inheriting
	// unrelated or incomplete Git metadata above the temporary directory.
	runGit(t, root, "init", "-q", "--template=")
	template, err := os.ReadFile(filepath.Join("..", "..", "templates", "things-baseline", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Makefile"), template, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.invalid/thingsfixture\n\ngo 1.24.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"service/apisvr", "service/dgsvr", "service/dmsvr"} {
		directory := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func gateOutputFixture(t *testing.T, command string) (GateSpecification, string) {
	t.Helper()
	outputDirectory := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(outputDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	return gateOutputFixtureAt(t, outputDirectory, command)
}

func gateOutputFixtureAt(t *testing.T, outputDirectory, command string) (GateSpecification, string) {
	t.Helper()
	specification := newGateOutputSpecification(outputDirectory, command)
	specification.RedactValues = []string{filepath.Dir(outputDirectory)}
	return specification, outputDirectory
}

func newGateOutputSpecification(outputDirectory, command string) GateSpecification {
	return GateSpecification{
		ID: "things-build", DisplayCommand: "fixture build", Path: "/bin/sh", Args: []string{"-c", command},
		Env:            []string{"PATH=/usr/bin:/bin", "OUTPUT=" + outputDirectory},
		goTool:         fixtureGateGoTool{},
		outputContract: &gateOutputContract{directory: outputDirectory, logicalRoot: "backend/bin", names: []string{"thingsvr", "dgsvr", "dmsvr"}, mode: 0o755},
	}
}

type fixtureGateGoTool struct{}

func (fixtureGateGoTool) inheritedFile() *os.File              { return nil }
func (fixtureGateGoTool) attestedVersion() string              { return lockedGoVersion }
func (fixtureGateGoTool) revalidate(ctx context.Context) error { return ctx.Err() }

func gateOutputWriteCommands(names ...string) string {
	commands := make([]string, 0, len(names))
	for _, name := range names {
		commands = append(commands, fmt.Sprintf(`printf %s > "$OUTPUT/%s"; chmod 0755 "$OUTPUT/%s"`, name, name, name))
	}
	return strings.Join(commands, "; ")
}

func assertContractedOutputsAbsent(t *testing.T, outputDirectory string) {
	t.Helper()
	for _, name := range []string{"thingsvr", "dgsvr", "dmsvr"} {
		if _, err := os.Lstat(filepath.Join(outputDirectory, name)); !os.IsNotExist(err) {
			t.Errorf("contracted output %q remains after failure: %v", name, err)
		}
	}
}

func TestGateOutputBufferSanitizesURLAcrossWritesAndFinalization(t *testing.T) {
	t.Run("mixed-case URL split across writes", func(t *testing.T) {
		buffer := newGateOutputBuffer(GateSpecification{})
		chunks := []string{"before-hT", "tPs://", strings.Repeat("c", 5200), "@host.example", "/path/URL-CHUNK-SENTINEL"}
		for _, chunk := range chunks {
			if _, err := buffer.Write([]byte(chunk)); err != nil {
				t.Fatalf("gateOutputBuffer.Write() error: %v", err)
			}
			if len(buffer.diagnostic.urlCandidate) > len("https://") {
				t.Fatalf("URL candidate grew to %d bytes", len(buffer.diagnostic.urlCandidate))
			}
			if len(buffer.diagnostic.pending) > len("https://") {
				t.Fatalf("URL-only pending bytes grew to %d", len(buffer.diagnostic.pending))
			}
		}
		if got, want := buffer.SHA256(), sha256Digest([]byte(strings.Join(chunks, ""))); got != want {
			t.Fatalf("raw gate output SHA-256 = %q, want %q", got, want)
		}
		got := buffer.String()
		if strings.Contains(got, strings.Repeat("c", 32)) || !strings.Contains(got, "hTtPs://[REDACTED]/path/URL-CHUNK-SENTINEL") {
			t.Fatalf("chunked URL diagnostic = %q", got)
		}
	})

	t.Run("unterminated authority", func(t *testing.T) {
		buffer := newGateOutputBuffer(GateSpecification{})
		for _, chunk := range []string{"HTTP", "://", strings.Repeat("z", 5200)} {
			if _, err := buffer.Write([]byte(chunk)); err != nil {
				t.Fatalf("gateOutputBuffer.Write() error: %v", err)
			}
		}
		got := buffer.String()
		if strings.Contains(got, strings.Repeat("z", 32)) || got != "HTTP://[REDACTED]" {
			t.Fatalf("unterminated URL diagnostic = %q", got)
		}
	})
}

func TestGateOutputBufferPreservesConfiguredLiteralCoverageInsideURLs(t *testing.T) {
	t.Run("full URL environment literal", func(t *testing.T) {
		secret := "https://host.example/private/path-secret-token?credential=query-secret-token"
		chunks := []string{"https://host.", "example/private/path-", "secret-token?credential=query-", "secret-token"}
		buffer := newGateOutputBuffer(GateSpecification{Env: []string{"FIXTURE_SECRET=" + secret}})
		for _, chunk := range chunks {
			if _, err := buffer.Write([]byte(chunk)); err != nil {
				t.Fatalf("gateOutputBuffer.Write() error: %v", err)
			}
		}
		got := buffer.String()
		for _, fragment := range []string{"path-secret-token", "query-secret-token", secret} {
			if strings.Contains(got, fragment) {
				t.Fatalf("full-URL configured literal leaked %q in %q", fragment, got)
			}
		}
		if gotSHA, wantSHA := buffer.SHA256(), sha256Digest([]byte(strings.Join(chunks, ""))); gotSHA != wantSHA {
			t.Fatalf("full-URL raw SHA-256 = %q, want %q", gotSHA, wantSHA)
		}
	})

	t.Run("host path literal inside URL", func(t *testing.T) {
		secret := "host.example/private/path-host-secret-suffix"
		chunks := []string{"prefix-https://host.example/private/", "path-host-secret-", "suffix?visible=query"}
		buffer := newGateOutputBuffer(GateSpecification{RedactValues: []string{secret}})
		for _, chunk := range chunks {
			if _, err := buffer.Write([]byte(chunk)); err != nil {
				t.Fatalf("gateOutputBuffer.Write() error: %v", err)
			}
		}
		got := buffer.String()
		for _, fragment := range []string{"path-host-secret-suffix", secret} {
			if strings.Contains(got, fragment) {
				t.Fatalf("host/path configured literal leaked %q in %q", fragment, got)
			}
		}
		if gotSHA, wantSHA := buffer.SHA256(), sha256Digest([]byte(strings.Join(chunks, ""))); gotSHA != wantSHA {
			t.Fatalf("host/path raw SHA-256 = %q, want %q", gotSHA, wantSHA)
		}
	})
}

func TestGateOutputBufferUnifiesConfiguredAndURLRangesOnRawBytes(t *testing.T) {
	t.Run("reviewer reproducer", func(t *testing.T) {
		raw := "https://alice:password@host.example/path"
		buffer := newGateOutputBuffer(GateSpecification{RedactValues: []string{"http"}})
		if _, err := buffer.Write([]byte(raw)); err != nil {
			t.Fatalf("gateOutputBuffer.Write() error: %v", err)
		}
		got := buffer.String()
		if strings.Contains(got, "alice:password@host.example") {
			t.Fatalf("reviewer authority survived overlapping redaction: %q", got)
		}
		if gotSHA, wantSHA := buffer.SHA256(), sha256Digest([]byte(raw)); gotSHA != wantSHA {
			t.Fatalf("reviewer raw SHA-256 = %q, want %q", gotSHA, wantSHA)
		}
	})

	variants := []struct {
		name    string
		literal string
		chunks  []string
	}{
		{name: "http overlap", literal: "http", chunks: []string{"h", "tt", "ps://alice:password@host.example", "/path"}},
		{name: "https overlap", literal: "https", chunks: []string{"ht", "tps", "://alice:password@host.example/pa", "th"}},
		{name: "scheme delimiter overlap", literal: "://", chunks: []string{"https", ":", "//alice:password@host.example/path"}},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			raw := strings.Join(variant.chunks, "")
			buffer := newGateOutputBuffer(GateSpecification{RedactValues: []string{variant.literal}})
			for _, chunk := range variant.chunks {
				if _, err := buffer.Write([]byte(chunk)); err != nil {
					t.Fatalf("gateOutputBuffer.Write() error: %v", err)
				}
			}
			got := buffer.String()
			if strings.Contains(got, "alice:password@host.example") || !utf8.ValidString(got) || len(got) > maxGateSummaryBytes {
				t.Fatalf("chunked overlap diagnostic = valid %v len %d value %q", utf8.ValidString(got), len(got), got)
			}
			if variant.literal == "://" && got != "https[REDACTED]/path" {
				t.Fatalf("adjacent scheme/authority ranges were not emitted as one union: %q", got)
			}
			if gotSHA, wantSHA := buffer.SHA256(), sha256Digest([]byte(raw)); gotSHA != wantSHA {
				t.Fatalf("chunked overlap raw SHA-256 = %q, want %q", gotSHA, wantSHA)
			}
		})
	}
}

func TestBoundedDiagnosticRedactorStreamingSemantics(t *testing.T) {
	t.Run("literal split across writes", func(t *testing.T) {
		redactor := newBoundedDiagnosticRedactor(128, []string{"alpha-secret"})
		for _, chunk := range []string{"before-alpha", "-sec", "ret-after"} {
			redactor.Write([]byte(chunk))
		}
		if got, want := redactor.String(), "before-[REDACTED]-after"; got != want {
			t.Fatalf("split literal diagnostic = %q, want %q", got, want)
		}
	})

	t.Run("overlapping and contained literals redact their union", func(t *testing.T) {
		redactor := newBoundedDiagnosticRedactor(128, []string{"cde", "abc", "bc", "abc"})
		redactor.Write([]byte("left-abcde-right"))
		if got, want := redactor.String(), "left-[REDACTED]-right"; got != want {
			t.Fatalf("overlapping literal diagnostic = %q, want %q", got, want)
		}
	})

	t.Run("no match preserves bytes", func(t *testing.T) {
		redactor := newBoundedDiagnosticRedactor(128, []string{"secret"})
		redactor.Write([]byte("ordinary-diagnostic"))
		if got, want := redactor.String(), "ordinary-diagnostic"; got != want {
			t.Fatalf("no-match diagnostic = %q, want %q", got, want)
		}
	})

	t.Run("tail remains bounded", func(t *testing.T) {
		redactor := newBoundedDiagnosticRedactor(8, nil)
		redactor.Write([]byte("0123456789"))
		if got, want := redactor.String(), "23456789"; got != want {
			t.Fatalf("bounded diagnostic = %q, want %q", got, want)
		}
	})

	t.Run("tail remains valid UTF-8 across a rune boundary", func(t *testing.T) {
		redactor := newBoundedDiagnosticRedactor(8, nil)
		redactor.Write([]byte("界界界"))
		got := redactor.String()
		if !utf8.ValidString(got) || len(got) > 8 {
			t.Fatalf("UTF-8 bounded diagnostic = valid %v len %d value %q", utf8.ValidString(got), len(got), got)
		}
	})

	t.Run("final partial literal prefix is conservative", func(t *testing.T) {
		secret := strings.Repeat("q", 128) + "z"
		redactor := newBoundedDiagnosticRedactor(128, []string{secret})
		redactor.Write([]byte(strings.Repeat("q", 96)))
		if got := redactor.String(); got != "[REDACTED]" || strings.Contains(got, strings.Repeat("q", 32)) {
			t.Fatalf("partial-prefix diagnostic = %q", got)
		}
	})

	t.Run("adversarial repeated long prefix keeps bounded pending state", func(t *testing.T) {
		literal := strings.Repeat("a", 256) + "b"
		redactor := newBoundedDiagnosticRedactor(64, []string{literal})
		chunk := literal + "c"
		for iteration := 0; iteration < 1024; iteration++ {
			redactor.Write([]byte(chunk))
			if len(redactor.pending) > len(literal) {
				t.Fatalf("pending bytes grew to %d, literal length %d", len(redactor.pending), len(literal))
			}
			if len(redactor.ranges) > len(redactor.pending)+1 {
				t.Fatalf("redaction ranges grew to %d with %d pending bytes", len(redactor.ranges), len(redactor.pending))
			}
		}
		if got := redactor.String(); len(got) > 64 || strings.Contains(got, strings.Repeat("a", 32)) {
			t.Fatalf("adversarial diagnostic = %q", got)
		}
	})
}

func TestVerifyRunsTwoRendersReadOnlyTargetAndAllGates(t *testing.T) {
	backend, lock, snapshot, dependencies := preparedVerifyFixture(t)
	state := SourceRepositoryState{Head: strings.Repeat("c", 40), Ref: lock.Source.Commit, StatusSHA256: strings.Repeat("d", 64), IndexSHA256: strings.Repeat("e", 64)}
	snapshot.RepositoryState = state
	renderCalls := 0
	moduleCalls := 0
	runner := &recordingGateRunner{}
	started := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	nowCalls := 0
	verifyDependencies := verifyDependencies{
		now: func() time.Time {
			nowCalls++
			return started.Add(time.Duration(nowCalls-1) * 5 * time.Second)
		},
		elapsed: func(time.Time) time.Duration { return time.Second },
		readSource: func(context.Context, string, *Lock) (SourceSnapshot, error) {
			return cloneSourceSnapshot(snapshot), nil
		},
		captureSourceState: func(context.Context, string, *Lock) (SourceRepositoryState, error) { return state, nil },
		render: func(ctx context.Context, backend string, snapshot SourceSnapshot, lock *Lock) (*Candidate, error) {
			renderCalls++
			return renderVerificationCandidateWithDependencies(ctx, backend, snapshot, lock, dependencies.renderDependencies)
		},
		compareTarget:   compareCandidateTarget,
		createIsolation: createVerificationIsolation,
		captureModules: func(string) (moduleInventory, error) {
			moduleCalls++
			return moduleInventory{"fixture": {Exists: true, Mode: "100644", SHA256: strings.Repeat("a", 64)}}, nil
		},
		buildGates: func(_ VerifyRequest, _ *VerificationIsolation) ([]GateSpecification, func(), error) {
			return []GateSpecification{{ID: "one", DisplayCommand: "command one"}, {ID: "two", DisplayCommand: "command two"}}, func() {}, nil
		},
		gateRunner:  runner,
		gateTimeout: time.Minute,
	}
	evidence, err := verifyWithDependencies(context.Background(), VerifyRequest{Source: "/source", Backend: backend, Lock: &lock}, verifyDependencies)
	if err != nil {
		t.Fatalf("verifyWithDependencies() error: %v", err)
	}
	if evidence.OverallResult != ResultPass || !evidence.FinalEvidence || renderCalls != 2 || strings.Join(runner.calls, ",") != "one,two" || moduleCalls != 2 {
		t.Fatalf("verify outcome/calls = %+v render=%d gates=%v modules=%d", evidence, renderCalls, runner.calls, moduleCalls)
	}
	if evidence.Render1.TreeSHA256 == "" || !reflect.DeepEqual(evidence.Render1, evidence.Render2) || evidence.Target.TotalDifferences != 0 || evidence.Target.BeforeTreeSHA256 != evidence.Target.AfterTreeSHA256 {
		t.Fatalf("verify reproducibility/target evidence = %+v", evidence)
	}
	if evidence.Source.Before != state || evidence.Source.After != state || evidence.Source.Repository != lock.Source.Repository {
		t.Fatalf("verify source evidence = %+v", evidence.Source)
	}
	if evidence.DurationMS != 1000 || !evidence.FinishedAt.Equal(started.Add(time.Second)) {
		t.Fatalf("verify evidence time = started %s finished %s duration %d", evidence.StartedAt, evidence.FinishedAt, evidence.DurationMS)
	}
}

func TestVerifyFailureStopsGatesButStillChecksModulesTargetAndSource(t *testing.T) {
	backend, lock, snapshot, dependencies := preparedVerifyFixture(t)
	state := SourceRepositoryState{Head: strings.Repeat("c", 40), Ref: lock.Source.Commit, StatusSHA256: strings.Repeat("d", 64), IndexSHA256: strings.Repeat("e", 64)}
	snapshot.RepositoryState = state
	exitZero, exitFailure := 0, 3
	runner := &recordingGateRunner{results: map[string]GateResult{
		"one": {Result: ResultPass, Execution: ExecutionExited, ExitCode: &exitZero, LogSHA256: strings.Repeat("a", 64)},
		"two": {Result: ResultFail, Execution: ExecutionExited, ExitCode: &exitFailure, LogSHA256: strings.Repeat("b", 64)},
	}}
	moduleCalls, sourceCalls := 0, 0
	deps := verifyDependencies{
		now: time.Now,
		readSource: func(context.Context, string, *Lock) (SourceSnapshot, error) {
			return cloneSourceSnapshot(snapshot), nil
		},
		captureSourceState: func(context.Context, string, *Lock) (SourceRepositoryState, error) {
			sourceCalls++
			return state, nil
		},
		render: func(ctx context.Context, backend string, snapshot SourceSnapshot, lock *Lock) (*Candidate, error) {
			return renderVerificationCandidateWithDependencies(ctx, backend, snapshot, lock, dependencies.renderDependencies)
		},
		compareTarget:   compareCandidateTarget,
		createIsolation: createVerificationIsolation,
		captureModules: func(string) (moduleInventory, error) {
			moduleCalls++
			return moduleInventory{}, nil
		},
		buildGates: func(VerifyRequest, *VerificationIsolation) ([]GateSpecification, func(), error) {
			return []GateSpecification{{ID: "one", DisplayCommand: "one"}, {ID: "two", DisplayCommand: "two"}, {ID: "three", DisplayCommand: "three"}}, func() {}, nil
		},
		gateRunner: runner, gateTimeout: time.Minute,
	}
	evidence, err := verifyWithDependencies(context.Background(), VerifyRequest{Source: "/source", Backend: backend, Lock: &lock}, deps)
	if err == nil || evidence.OverallResult != ResultFail || evidence.Gates[2].Result != ResultNotRun {
		t.Fatalf("failed verify = evidence %+v error %v", evidence, err)
	}
	if strings.Join(runner.calls, ",") != "one,two" || moduleCalls != 2 || sourceCalls != 1 || evidence.Target.BeforeTreeSHA256 != evidence.Target.AfterTreeSHA256 {
		t.Fatalf("failure cleanup checks = gates %v modules %d source %d target %+v", runner.calls, moduleCalls, sourceCalls, evidence.Target)
	}
}

func TestVerifyStructuralOnlyIsExplicitlyNotFinalAndRunsNoGates(t *testing.T) {
	backend, lock, snapshot, dependencies := preparedVerifyFixture(t)
	state := SourceRepositoryState{Head: strings.Repeat("c", 40), Ref: lock.Source.Commit, StatusSHA256: strings.Repeat("d", 64), IndexSHA256: strings.Repeat("e", 64)}
	snapshot.RepositoryState = state
	runner := &recordingGateRunner{}
	deps := verifyDependencies{
		now: time.Now,
		readSource: func(context.Context, string, *Lock) (SourceSnapshot, error) {
			return cloneSourceSnapshot(snapshot), nil
		},
		captureSourceState: func(context.Context, string, *Lock) (SourceRepositoryState, error) { return state, nil },
		render: func(ctx context.Context, backend string, snapshot SourceSnapshot, lock *Lock) (*Candidate, error) {
			return renderVerificationCandidateWithDependencies(ctx, backend, snapshot, lock, dependencies.renderDependencies)
		},
		compareTarget: compareCandidateTarget,
		gateRunner:    runner,
	}
	evidence, err := verifyWithDependencies(context.Background(), VerifyRequest{Source: "/source", Backend: backend, Lock: &lock, StructuralOnly: true}, deps)
	if err != nil || evidence.FinalEvidence || evidence.OverallResult != ResultNotFinal || len(runner.calls) != 0 {
		t.Fatalf("structural verify = %+v, error=%v, calls=%v", evidence, err, runner.calls)
	}
}

func TestVerifyStructuralDriftIsNonFinalFailureEvidence(t *testing.T) {
	backend, lock, snapshot, dependencies := preparedVerifyFixture(t)
	state := SourceRepositoryState{Head: strings.Repeat("c", 40), Ref: lock.Source.Commit, StatusSHA256: strings.Repeat("d", 64), IndexSHA256: strings.Repeat("e", 64)}
	snapshot.RepositoryState = state
	drifted := state
	drifted.IndexSHA256 = strings.Repeat("f", 64)
	deps := verifyDependencies{
		now: time.Now,
		readSource: func(context.Context, string, *Lock) (SourceSnapshot, error) {
			return cloneSourceSnapshot(snapshot), nil
		},
		captureSourceState: func(context.Context, string, *Lock) (SourceRepositoryState, error) { return drifted, nil },
		render: func(ctx context.Context, backend string, snapshot SourceSnapshot, lock *Lock) (*Candidate, error) {
			return renderVerificationCandidateWithDependencies(ctx, backend, snapshot, lock, dependencies.renderDependencies)
		},
		compareTarget: compareCandidateTarget,
	}
	evidence, err := verifyWithDependencies(context.Background(), VerifyRequest{Source: "/source", Backend: backend, Lock: &lock, StructuralOnly: true}, deps)
	if err == nil || evidence.FinalEvidence || evidence.OverallResult != ResultFail {
		t.Fatalf("structural drift evidence = %+v, error=%v", evidence, err)
	}
	if _, marshalErr := MarshalEvidence(evidence); marshalErr != nil {
		t.Fatalf("MarshalEvidence() rejected structural drift diagnostic: %v", marshalErr)
	}
}

func TestModuleInventoryAlwaysCoversWorkspaceAndThreeModules(t *testing.T) {
	backend := t.TempDir()
	for _, relative := range moduleInventoryPaths {
		writeExpectedRegular(t, backend, relative, []byte(relative+"\n"), "100644")
	}
	inventory, err := captureModuleInventory(backend)
	if err != nil {
		t.Fatalf("captureModuleInventory() error: %v", err)
	}
	if len(inventory) != len(moduleInventoryPaths) {
		t.Fatalf("module inventory size = %d, want %d", len(inventory), len(moduleInventoryPaths))
	}
	for _, relative := range moduleInventoryPaths {
		if record := inventory[relative]; !record.Exists || record.Mode != "100644" || len(record.SHA256) != 64 {
			t.Fatalf("module inventory[%q] = %+v", relative, record)
		}
	}
	if err := os.Chmod(filepath.Join(backend, "things", "go.mod"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := captureModuleInventory(backend); err == nil || !strings.Contains(err.Error(), "0644") {
		t.Fatalf("unsafe module mode error = %v", err)
	}
}

func TestDefaultGateSpecificationsBindIsolationPinBuildToolchainAndOwnOutputs(t *testing.T) {
	hostileRoot := t.TempDir()
	for key, value := range map[string]string{
		"PATH": filepath.Join(hostileRoot, "fake-bin"), "MAKEFILES": filepath.Join(hostileRoot, "hostile.mk"), "MAKEFLAGS": "-e -f hostile.mk",
		"GNUMAKEFLAGS": "-e", "MFLAGS": "-e", "GO_BUILD": "hostile", "BASH_ENV": "hostile", "ENV": "hostile", "CDPATH": "hostile",
		"SHELLOPTS": "hostile", "BASHOPTS": "hostile", "KSH_ENV": "hostile", "ZDOTDIR": "hostile", "TASK1_HOSTILE_INHERITED": "must-not-pass",
	} {
		t.Setenv(key, value)
	}
	backend, _, candidate := verificationTargetFixture(t)
	isolation, err := createVerificationIsolation(candidate)
	if err != nil {
		t.Fatal(err)
	}
	defer isolation.Cleanup()
	localBin := filepath.Join(backend, "bin")
	localBinary := filepath.Join(localBin, "coresvr")
	if err := os.MkdirAll(localBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localBinary, []byte("local-only"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(localBinary, 0o755); err != nil {
		t.Fatal(err)
	}
	specifications, cleanup, err := buildDefaultVerificationGates(VerifyRequest{Backend: backend}, isolation)
	if err != nil {
		t.Fatalf("buildDefaultVerificationGates() error: %v", err)
	}
	cleaned := false
	defer func() {
		if !cleaned {
			cleanup()
		}
	}()
	if len(specifications) != 5 || specifications[2].ID != "things-test" || specifications[2].Dir != isolation.Path() || specifications[2].Dir == filepath.Join(backend, "things") {
		t.Fatalf("default gate specifications = %+v", specifications)
	}
	build := specifications[4]
	if build.ID != "things-build" || build.Path != "/usr/bin/make" || build.DisplayCommand != "cd verification-tree && make BIN_DIR=backend/bin build" {
		t.Fatalf("build gate identity = %+v", build)
	}
	if build.outputContract == nil {
		t.Fatal("build output contract is nil")
	}
	buildOutput := build.outputContract.directory
	cacheRoot := filepath.Dir(buildOutput)
	if buildOutput == localBin || filepath.Base(buildOutput) != "build-output" || filepath.Dir(cacheRoot) != backend || !strings.HasPrefix(filepath.Base(cacheRoot), ".thingsbaseline-gates-") {
		t.Fatalf("build output isolation = output %q, cache root %q", buildOutput, cacheRoot)
	}
	if !slices.Contains(build.Args, "GO=/proc/self/fd/3") || !slices.Contains(build.Args, "BIN_DIR="+buildOutput) || build.Args[len(build.Args)-1] != "build" {
		t.Fatalf("build gate does not pin verified Go/BIN_DIR: %q", build.Args)
	}
	sharedBuildCache := environmentMap(specifications[0].Env)["GOCACHE"]
	sharedModuleCache := environmentMap(specifications[0].Env)["GOMODCACHE"]
	if sharedBuildCache == "" || sharedModuleCache == "" {
		t.Fatalf("test gate shared caches = GOCACHE %q, GOMODCACHE %q", sharedBuildCache, sharedModuleCache)
	}
	for _, specification := range specifications[:4] {
		if !slices.Contains(specification.Args, "-count=1") {
			t.Errorf("test gate %s does not disable test result caching: %q", specification.ID, specification.Args)
		}
		environment := environmentMap(specification.Env)
		if environment["GOCACHE"] != sharedBuildCache || environment["GOMODCACHE"] != sharedModuleCache {
			t.Errorf("test gate %s cache environment = GOCACHE %q, GOMODCACHE %q; want shared %q, %q", specification.ID, environment["GOCACHE"], environment["GOMODCACHE"], sharedBuildCache, sharedModuleCache)
		}
	}
	buildEnvironment := environmentMap(build.Env)
	if buildEnvironment["GOCACHE"] != sharedBuildCache || buildEnvironment["GOMODCACHE"] != sharedModuleCache {
		t.Fatalf("build gate cache environment = GOCACHE %q, GOMODCACHE %q; want shared %q, %q", buildEnvironment["GOCACHE"], buildEnvironment["GOMODCACHE"], sharedBuildCache, sharedModuleCache)
	}
	if slices.Contains(build.Args, "-count=1") {
		t.Fatalf("build gate unexpectedly disables test result caching: %q", build.Args)
	}
	for _, cachePath := range []string{sharedBuildCache, sharedModuleCache} {
		relative, err := filepath.Rel(cacheRoot, cachePath)
		if err != nil {
			t.Fatalf("relate private cache root to shared cache %q: %v", cachePath, err)
		}
		if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
			t.Fatalf("shared cache %q is a descendant of private cache root %q", cachePath, cacheRoot)
		}
		for _, specification := range specifications {
			if !slices.Contains(specification.RedactValues, cachePath) {
				t.Errorf("gate %s RedactValues omitted shared cache %q", specification.ID, cachePath)
			}
			if strings.Contains(specification.DisplayCommand, cachePath) {
				t.Errorf("gate %s DisplayCommand leaked shared cache %q: %q", specification.ID, cachePath, specification.DisplayCommand)
			}
		}
	}
	if build.goTool == nil || build.goTool.inheritedFile() == nil || build.goTool.attestedVersion() != lockedGoVersion {
		t.Fatalf("build gate retained Go tool = %#v", build.goTool)
	}
	if build.outputContract.directory != buildOutput || build.outputContract.logicalRoot != "backend/bin" || !reflect.DeepEqual(build.outputContract.names, []string{"thingsvr", "dgsvr", "dmsvr"}) || build.outputContract.mode != 0o755 {
		t.Fatalf("build output contract = %#v", build.outputContract)
	}
	buildEnvironment = environmentMap(build.Env)
	for key, want := range map[string]string{
		"PATH": "/usr/bin:/bin", "MAKEFILES": "", "MAKEFLAGS": "", "GNUMAKEFLAGS": "", "MFLAGS": "", "MAKEOVERRIDES": "",
		"GO_BUILD": "", "SHELL": "/bin/sh", "BASH_ENV": "", "ENV": "", "CDPATH": "", "SHELLOPTS": "", "BASHOPTS": "", "KSH_ENV": "", "ZDOTDIR": "",
	} {
		if got := buildEnvironment[key]; got != want {
			t.Fatalf("build environment %s = %q, want %q", key, got, want)
		}
	}
	if _, exists := buildEnvironment["TASK1_HOSTILE_INHERITED"]; exists {
		t.Fatalf("build environment inherited hostile variable: %q", build.Env)
	}
	workspaceEnvironment := environmentMap(specifications[3].Env)
	workspacePath := workspaceEnvironment["GOWORK"]
	workspace, err := os.ReadFile(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(workspace, []byte(isolation.Path())) || bytes.Contains(workspace, []byte(filepath.Join(backend, "things"))) {
		t.Fatalf("temporary workspace does not isolate target:\n%s", workspace)
	}
	cleanup()
	cleaned = true
	if _, err := os.Stat(cacheRoot); !os.IsNotExist(err) {
		t.Fatalf("cache root still exists after cleanup: %q, err=%v", cacheRoot, err)
	}
	for _, cachePath := range []string{sharedBuildCache, sharedModuleCache} {
		info, err := os.Stat(cachePath)
		if err != nil || !info.IsDir() {
			t.Fatalf("shared cache %q was removed by cleanup: %v", cachePath, err)
		}
	}
	localData, err := os.ReadFile(localBinary)
	if err != nil {
		t.Fatalf("read local binary: %v", err)
	}
	if string(localData) != "local-only" {
		t.Fatalf("local binary content = %q, want %q", localData, "local-only")
	}
}

func verificationTargetFixture(t *testing.T) (string, string, *Candidate) {
	t.Helper()
	backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
	target := filepath.Join(backend, "things")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	candidate, err := renderVerificationCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
	if err != nil {
		t.Fatalf("render verification candidate: %v", err)
	}
	t.Cleanup(func() { _ = candidate.Cleanup() })
	for path, file := range candidate.policy.expectedFiles {
		writeExpectedRegular(t, target, path, file.Data, file.Mode)
	}
	return backend, target, candidate
}

func preparedVerifyFixture(t *testing.T) (string, Lock, SourceSnapshot, syntheticRenderDependencies) {
	t.Helper()
	backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
	target := filepath.Join(backend, "things")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	candidate, err := renderVerificationCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
	if err != nil {
		t.Fatalf("prepare verification candidate: %v", err)
	}
	for path, file := range candidate.policy.expectedFiles {
		writeExpectedRegular(t, target, path, file.Data, file.Mode)
	}
	if err := candidate.Cleanup(); err != nil {
		t.Fatalf("cleanup prepared candidate: %v", err)
	}
	return backend, lock, snapshot, dependencies
}

func containsDifferencePath(differences []TreeDifference, path string) bool {
	for _, difference := range differences {
		if difference.Path == path {
			return true
		}
	}
	return false
}

type recordingGateRunner struct {
	results map[string]GateResult
	calls   []string
}

func (runner *recordingGateRunner) Run(_ context.Context, specification GateSpecification, _ time.Duration) GateResult {
	runner.calls = append(runner.calls, specification.ID)
	result, exists := runner.results[specification.ID]
	if !exists {
		exitCode := 0
		result = GateResult{Result: ResultPass, Execution: ExecutionExited, ExitCode: &exitCode, LogSHA256: strings.Repeat("0", 64)}
	}
	result.Command = specification.DisplayCommand
	return result
}
