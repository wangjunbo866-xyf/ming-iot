package thingsbaseline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvidenceMarshallingIsStableAndModelsProcessFailures(t *testing.T) {
	started := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	exitZero := 0
	evidence := Evidence{
		SchemaVersion: EvidenceSchemaVersion,
		LoopID:        "L0",
		FinalEvidence: true,
		StartedAt:     started,
		FinishedAt:    started.Add(1500 * time.Millisecond),
		DurationMS:    1500,
		Source: SourceEvidence{
			Repository: "https://gitee.com/unitedrhino/things",
			Ref:        "refs/remotes/origin/master",
			Commit:     strings.Repeat("a", 40),
			Tree:       strings.Repeat("b", 40),
			Before:     SourceRepositoryState{Head: strings.Repeat("c", 40), Ref: strings.Repeat("a", 40), StatusSHA256: strings.Repeat("d", 64), IndexSHA256: strings.Repeat("e", 64)},
			After:      SourceRepositoryState{Head: strings.Repeat("c", 40), Ref: strings.Repeat("a", 40), StatusSHA256: strings.Repeat("d", 64), IndexSHA256: strings.Repeat("e", 64)},
		},
		Render1:   RenderEvidence{Credentials: CredentialProfileReport{Paths: CredentialPathCounts{DatabaseDSN: 1}}},
		Contracts: ContractCounts{APIFiles: 34, ProtoFiles: 18, GeneratedProtoFiles: 9, TestFiles: 166},
		Target:    TargetEvidence{LogicalPath: "backend/things", Module: "gitee.com/ming-iot/things"},
		Phases:    []PhaseResult{{ID: "render-1", Result: ResultPass}},
		Gates: []GateResult{
			{ID: "tools-test", Command: "go test ./...", Result: ResultPass, Execution: ExecutionExited, ExitCode: &exitZero, LogSHA256: strings.Repeat("e", 64)},
			{ID: "things-test", Command: "GOWORK=off go test ./...", Result: ResultFail, Execution: ExecutionTimeout, ExitCode: nil, LogSHA256: strings.Repeat("f", 64), Summary: "timed out"},
			{ID: "workspace-test", Command: "go test workspace modules", Result: ResultNotRun, Execution: ExecutionNotRun, ExitCode: nil},
			{ID: "things-build", Command: "make BIN_DIR=backend/bin build", Result: ResultPass, Execution: ExecutionExited, ExitCode: &exitZero, Artifacts: []GateArtifact{
				{LogicalPath: "backend/bin/dgsvr", Mode: "0755", Size: 2, SHA256: strings.Repeat("1", 64), GoVersion: "go1.25.4"},
				{LogicalPath: "backend/bin/dmsvr", Mode: "0755", Size: 3, SHA256: strings.Repeat("2", 64), GoVersion: "go1.25.4"},
				{LogicalPath: "backend/bin/thingsvr", Mode: "0755", Size: 4, SHA256: strings.Repeat("3", 64), GoVersion: "go1.25.4"},
			}},
		},
		OverallResult: ResultFail,
	}
	first, err := MarshalEvidence(evidence)
	if err != nil {
		t.Fatalf("MarshalEvidence() error: %v", err)
	}
	second, err := MarshalEvidence(evidence)
	if err != nil {
		t.Fatalf("MarshalEvidence() second error: %v", err)
	}
	if !bytes.Equal(first, second) || len(first) == 0 || first[len(first)-1] != '\n' {
		t.Fatalf("evidence JSON is not stable/newline-terminated:\n%s", first)
	}
	var decoded map[string]any
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatal(err)
	}
	gates := decoded["gates"].([]any)
	if gates[1].(map[string]any)["exitCode"] != nil || gates[2].(map[string]any)["exitCode"] != nil {
		t.Fatalf("timeout/not-run exitCode must be JSON null: %s", first)
	}
	artifacts := gates[3].(map[string]any)["artifacts"].([]any)
	if len(artifacts) != 3 || artifacts[0].(map[string]any)["logicalPath"] != "backend/bin/dgsvr" || artifacts[0].(map[string]any)["goVersion"] != "go1.25.4" {
		t.Fatalf("build artifacts are not stable lower-camel evidence: %s", first)
	}
	contracts := decoded["contracts"].(map[string]any)
	for _, key := range []string{"apiFiles", "protoFiles", "generatedProtoFiles", "testFiles"} {
		if _, exists := contracts[key]; !exists {
			t.Fatalf("contracts missing stable lower-camel key %q: %s", key, first)
		}
	}
	source := decoded["source"].(map[string]any)
	for _, stateName := range []string{"before", "after"} {
		state := source[stateName].(map[string]any)
		if got := state["indexSha256"]; got != strings.Repeat("e", 64) {
			t.Fatalf("source.%s.indexSha256 = %v", stateName, got)
		}
	}
	paths := decoded["render1"].(map[string]any)["credentials"].(map[string]any)["paths"].(map[string]any)
	for _, key := range []string{
		"databaseDSN", "tsdbDSN", "ossAccessKeyID", "ossAccessKeySecret", "authWhiteUsersPassword",
		"devLinkOpenApiApiKey", "devLinkOpenApiSecretKey", "devLinkMqttOpenApiApiKey", "devLinkMqttOpenApiSecretKey",
	} {
		if _, exists := paths[key]; !exists {
			t.Fatalf("credential paths missing stable lower-camel key %q: %s", key, first)
		}
	}
}

func TestEvidenceRejectsInvalidStateCombinationsAndNonFinalPass(t *testing.T) {
	base := minimalValidEvidenceForTest()
	tests := []struct {
		name   string
		mutate func(*Evidence)
		want   string
	}{
		{"schema", func(e *Evidence) { e.SchemaVersion++ }, "schemaVersion"},
		{"loop", func(e *Evidence) { e.LoopID = "L1" }, "loopId"},
		{"duration mismatch", func(e *Evidence) { e.DurationMS++ }, "durationMs"},
		{"non-final pass", func(e *Evidence) { e.FinalEvidence = false }, "NOT_FINAL"},
		{"spawn with exit", func(e *Evidence) {
			code := 1
			e.Gates[0] = GateResult{ID: "gate", Command: "cmd", Result: ResultFail, Execution: ExecutionSpawnError, ExitCode: &code}
		}, "exitCode"},
		{"pass without exit", func(e *Evidence) {
			e.Gates[0] = GateResult{ID: "gate", Command: "cmd", Result: ResultPass, Execution: ExecutionExited, ExitCode: nil}
		}, "exitCode"},
		{"non-build exit-zero failure", func(e *Evidence) {
			code := 0
			e.Gates[0] = GateResult{ID: "gate", Command: "cmd", Result: ResultFail, Execution: ExecutionExited, ExitCode: &code, Summary: "postcondition"}
		}, "things-build"},
		{"not-run execution", func(e *Evidence) {
			e.Gates[0] = GateResult{ID: "gate", Command: "cmd", Result: ResultNotRun, Execution: ExecutionExited}
		}, "NOT_RUN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := base
			evidence.Gates = append([]GateResult(nil), base.Gates...)
			tt.mutate(&evidence)
			if _, err := MarshalEvidence(evidence); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("MarshalEvidence() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestEvidenceRejectsIncompleteOrContradictoryFinalPass(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Evidence)
		want   string
	}{
		{"missing phase", func(e *Evidence) { e.Phases = e.Phases[:len(e.Phases)-1] }, "PASS phases"},
		{"phase order", func(e *Evidence) { e.Phases[0], e.Phases[1] = e.Phases[1], e.Phases[0] }, "PASS phases"},
		{"failed phase", func(e *Evidence) { e.Phases[4].Result = ResultFail }, "PASS phase"},
		{"missing gate", func(e *Evidence) { e.Gates = e.Gates[:len(e.Gates)-1] }, "PASS gates"},
		{"gate order", func(e *Evidence) { e.Gates[0], e.Gates[1] = e.Gates[1], e.Gates[0] }, "PASS gates"},
		{"not-run gate", func(e *Evidence) {
			e.Gates[2].Result = ResultNotRun
			e.Gates[2].Execution = ExecutionNotRun
			e.Gates[2].ExitCode = nil
		}, "PASS gate"},
		{"nonzero gate", func(e *Evidence) { code := 7; e.Gates[2].ExitCode = &code }, "PASS"},
		{"reproducibility field", func(e *Evidence) { e.Reproducibility.Result = ResultFail }, "reproducibility"},
		{"render manifest mismatch", func(e *Evidence) { e.Render2.ManifestSHA256 = strings.Repeat("9", 64) }, "render digests"},
		{"render tree mismatch", func(e *Evidence) { e.Render2.TreeSHA256 = strings.Repeat("8", 64) }, "render digests"},
		{"target differences", func(e *Evidence) { e.Target.TotalDifferences = 1 }, "target"},
		{"target drift", func(e *Evidence) { e.Target.AfterTreeSHA256 = strings.Repeat("7", 64) }, "target"},
		{"source drift", func(e *Evidence) { e.Source.After.IndexSHA256 = strings.Repeat("6", 64) }, "source"},
		{"stdout tail budget", func(e *Evidence) { e.Render1.Commands[0].StdoutTail = strings.Repeat("x", maxCommandDiagnosticTail+1) }, "stdoutTail"},
		{"combined tail budget", func(e *Evidence) {
			e.Render1.Commands[0].StdoutTail = strings.Repeat("x", maxCommandDiagnosticTail)
			e.Render1.Commands[0].StderrTail = strings.Repeat("y", maxCommandDiagnosticTail+1)
		}, "stderrTail"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := minimalValidEvidenceForTest()
			tt.mutate(&evidence)
			if _, err := MarshalEvidence(evidence); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("MarshalEvidence() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestEvidenceAcceptsFinalFailurePrefixAndStrictStructuralNotFinal(t *testing.T) {
	failed := minimalValidEvidenceForTest()
	failed.OverallResult = ResultFail
	failed.Phases = append([]PhaseResult(nil), failed.Phases[:7]...)
	failed.Phases[6] = PhaseResult{ID: "command-gates", Result: ResultFail, FailureKind: "GATE"}
	exitFailure := 3
	failed.Gates[0] = GateResult{ID: "tools-test", Command: "go test ./...", Result: ResultFail, Execution: ExecutionExited, ExitCode: &exitFailure, LogSHA256: strings.Repeat("a", 64)}
	for index := 1; index < len(failed.Gates); index++ {
		failed.Gates[index] = GateResult{ID: finalGateIDs[index], Command: "not run", Result: ResultNotRun, Execution: ExecutionNotRun}
	}
	if _, err := MarshalEvidence(failed); err != nil {
		t.Fatalf("MarshalEvidence() rejected valid final FAIL prefix: %v", err)
	}

	structural := minimalStructuralEvidenceForTest()
	if _, err := MarshalEvidence(structural); err != nil {
		t.Fatalf("MarshalEvidence() rejected valid structural NOT_FINAL: %v", err)
	}
	for _, mutate := range []func(*Evidence){
		func(e *Evidence) { e.Phases = e.Phases[:6] },
		func(e *Evidence) { e.Phases[0], e.Phases[1] = e.Phases[1], e.Phases[0] },
		func(e *Evidence) { e.Phases[6].Result = ResultFail },
		func(e *Evidence) {
			e.Gates = []GateResult{{ID: "unexpected", Command: "unexpected", Result: ResultNotRun, Execution: ExecutionNotRun}}
		},
	} {
		candidate := minimalStructuralEvidenceForTest()
		mutate(&candidate)
		if _, err := MarshalEvidence(candidate); err == nil {
			t.Fatalf("MarshalEvidence() accepted contradictory structural NOT_FINAL: %+v", candidate)
		}
	}
}

func TestEvidenceAllowsFailedNonFinalDiagnostic(t *testing.T) {
	evidence := minimalValidEvidenceForTest()
	evidence.FinalEvidence = false
	evidence.OverallResult = ResultFail
	evidence.Phases[0] = PhaseResult{ID: "structural", Result: ResultFail, FailureKind: "FIXTURE"}
	if _, err := MarshalEvidence(evidence); err != nil {
		t.Fatalf("MarshalEvidence() rejected failed non-final diagnostic: %v", err)
	}
}

func TestEvidenceValidatesBuildArtifactsAndAllowsExitZeroPostconditionFailure(t *testing.T) {
	evidence := minimalValidEvidenceForTest()
	exitZero := 0
	evidence.OverallResult = ResultFail
	evidence.Gates = []GateResult{{
		ID: "things-build", Command: "make BIN_DIR=backend/bin build", Result: ResultPass, Execution: ExecutionExited, ExitCode: &exitZero,
		Artifacts: []GateArtifact{
			{LogicalPath: "backend/bin/dgsvr", Mode: "0755", Size: 1, SHA256: strings.Repeat("1", 64), GoVersion: lockedGoVersion},
			{LogicalPath: "backend/bin/dmsvr", Mode: "0755", Size: 2, SHA256: strings.Repeat("2", 64), GoVersion: lockedGoVersion},
			{LogicalPath: "backend/bin/thingsvr", Mode: "0755", Size: 3, SHA256: strings.Repeat("3", 64), GoVersion: lockedGoVersion},
		},
	}}
	if _, err := MarshalEvidence(evidence); err != nil {
		t.Fatalf("MarshalEvidence() rejected valid build artifacts: %v", err)
	}

	invalid := []struct {
		name   string
		mutate func(*GateResult)
	}{
		{"missing", func(g *GateResult) { g.Artifacts = g.Artifacts[:2] }},
		{"absolute logical path", func(g *GateResult) { g.Artifacts[0].LogicalPath = filepath.Join(t.TempDir(), "dgsvr") }},
		{"unsorted", func(g *GateResult) { g.Artifacts[0], g.Artifacts[1] = g.Artifacts[1], g.Artifacts[0] }},
		{"wrong mode", func(g *GateResult) { g.Artifacts[0].Mode = "0644" }},
		{"empty", func(g *GateResult) { g.Artifacts[0].Size = 0 }},
		{"sha", func(g *GateResult) { g.Artifacts[0].SHA256 = "bad" }},
		{"go version", func(g *GateResult) { g.Artifacts[0].GoVersion = "go1.99" }},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			candidate := evidence
			candidate.Gates = append([]GateResult(nil), evidence.Gates...)
			candidate.Gates[0].Artifacts = append([]GateArtifact(nil), evidence.Gates[0].Artifacts...)
			tt.mutate(&candidate.Gates[0])
			if _, err := MarshalEvidence(candidate); err == nil {
				t.Fatalf("MarshalEvidence() accepted invalid artifacts: %+v", candidate.Gates[0].Artifacts)
			}
		})
	}

	evidence.Gates[0] = GateResult{ID: "things-build", Command: "make BIN_DIR=backend/bin build", Result: ResultFail, Execution: ExecutionExited, ExitCode: &exitZero, Summary: "[runner]\nbuild output contract failed"}
	if _, err := MarshalEvidence(evidence); err != nil {
		t.Fatalf("MarshalEvidence() rejected exit-zero postcondition failure: %v", err)
	}
}

func TestEvidenceMarshallingUsesArraysForEmptyCollections(t *testing.T) {
	evidence := minimalValidEvidenceForTest()
	evidence.FinalEvidence = false
	evidence.OverallResult = ResultFail
	evidence.Gates = nil
	evidence.Target.Differences = nil
	evidence.Render1.Commands = nil
	evidence.Render2.Commands = nil

	data, err := MarshalEvidence(evidence)
	if err != nil {
		t.Fatalf("MarshalEvidence() error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	assertJSONArray := func(value any, field string) {
		t.Helper()
		if _, ok := value.([]any); !ok {
			t.Fatalf("%s must be a JSON array, got %#v", field, value)
		}
	}
	assertJSONArray(decoded["gates"], "gates")
	assertJSONArray(decoded["target"].(map[string]any)["differences"], "target.differences")
	assertJSONArray(decoded["render1"].(map[string]any)["commands"], "render1.commands")
	assertJSONArray(decoded["render2"].(map[string]any)["commands"], "render2.commands")
}

func TestEvidenceAtomicWriteRejectsUnsafeBoundariesAndSymlinks(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	backend := filepath.Join(root, "project", "backend")
	docs := filepath.Join(root, "project", "docs", "evidence")
	for _, directory := range []string{source, backend, docs} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	evidence := minimalValidEvidenceForTest()

	for _, unsafe := range []string{filepath.Join(source, "result.json"), filepath.Join(backend, "result.json")} {
		if err := WriteEvidenceAtomic(unsafe, evidence, []string{source, backend}); err == nil || !strings.Contains(err.Error(), "outside") {
			t.Fatalf("WriteEvidenceAtomic(%q) error = %v", unsafe, err)
		}
	}

	linkParent := filepath.Join(root, "linked-evidence")
	if err := os.Symlink(docs, linkParent); err != nil {
		t.Fatal(err)
	}
	if err := WriteEvidenceAtomic(filepath.Join(linkParent, "result.json"), evidence, []string{source, backend}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink-parent evidence error = %v", err)
	}

	target := filepath.Join(docs, "result.json")
	escape := filepath.Join(root, "escape.json")
	if err := os.WriteFile(escape, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(escape, target); err != nil {
		t.Fatal(err)
	}
	if err := WriteEvidenceAtomic(target, evidence, []string{source, backend}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink-target evidence error = %v", err)
	}
	if got, _ := os.ReadFile(escape); string(got) != "keep\n" {
		t.Fatalf("symlink escape was modified: %q", got)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	if err := WriteEvidenceAtomic(target, evidence, []string{source, backend}); err != nil {
		t.Fatalf("safe WriteEvidenceAtomic() error: %v", err)
	}
	first, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	evidence.DurationMS++
	evidence.FinishedAt = evidence.FinishedAt.Add(time.Millisecond)
	if err := WriteEvidenceAtomic(target, evidence, []string{source, backend}); err != nil {
		t.Fatalf("atomic evidence replacement error: %v", err)
	}
	second, err := os.ReadFile(target)
	if err != nil || bytes.Equal(first, second) {
		t.Fatalf("evidence was not replaced: equal=%v err=%v", bytes.Equal(first, second), err)
	}
	entries, err := os.ReadDir(docs)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "result.json" {
		t.Fatalf("atomic writer left temporary files: %v", entries)
	}
}

func TestEvidenceAtomicWriteRejectsAbsolutePathLeaks(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	backend := filepath.Join(root, "backend")
	docs := filepath.Join(root, "docs")
	for _, directory := range []string{source, backend, docs} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	evidence := minimalValidEvidenceForTest()
	evidence.Phases[0].Summary = "leak: " + backend
	if err := WriteEvidenceAtomic(filepath.Join(docs, "result.json"), evidence, []string{source, backend}); err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("absolute-path evidence error = %v", err)
	}
}

func minimalValidEvidenceForTest() Evidence {
	started := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	exitCode := 0
	commit := strings.Repeat("a", 40)
	state := SourceRepositoryState{
		Head: strings.Repeat("b", 40), Ref: commit,
		StatusSHA256: strings.Repeat("c", 64), IndexSHA256: strings.Repeat("d", 64),
	}
	command := func(name string) CommandReport {
		return CommandReport{Name: name, ExitCode: 0, StdoutSHA256: strings.Repeat("3", 64), StderrSHA256: strings.Repeat("4", 64)}
	}
	render := RenderEvidence{
		ManifestSHA256: strings.Repeat("1", 64), TreeSHA256: strings.Repeat("2", 64), Files: 1,
		Commands: []CommandReport{command("go-mod-tidy"), command("go-mod-verify")},
	}
	phases := make([]PhaseResult, len(finalPhaseIDs))
	for index, id := range finalPhaseIDs {
		phases[index] = PhaseResult{ID: id, Result: ResultPass}
	}
	gates := make([]GateResult, len(finalGateIDs))
	for index, id := range finalGateIDs {
		gates[index] = GateResult{ID: id, Command: "fixture " + id, Result: ResultPass, Execution: ExecutionExited, ExitCode: &exitCode, LogSHA256: strings.Repeat("5", 64)}
	}
	gates[len(gates)-1].Artifacts = []GateArtifact{
		{LogicalPath: "backend/bin/dgsvr", Mode: "0755", Size: 1, SHA256: strings.Repeat("6", 64), GoVersion: lockedGoVersion},
		{LogicalPath: "backend/bin/dmsvr", Mode: "0755", Size: 2, SHA256: strings.Repeat("7", 64), GoVersion: lockedGoVersion},
		{LogicalPath: "backend/bin/thingsvr", Mode: "0755", Size: 3, SHA256: strings.Repeat("8", 64), GoVersion: lockedGoVersion},
	}
	return Evidence{
		SchemaVersion: EvidenceSchemaVersion,
		LoopID:        "L0",
		FinalEvidence: true,
		StartedAt:     started,
		FinishedAt:    started.Add(time.Second),
		DurationMS:    1000,
		Source: SourceEvidence{
			Repository: "https://gitee.com/unitedrhino/things", Ref: "refs/remotes/origin/master", Commit: commit, Tree: strings.Repeat("e", 40),
			Before: state, After: state, SelectedFiles: 1, SelectedBlobBytes: 1, ArchiveSHA256: strings.Repeat("f", 64),
		},
		Render1:         render,
		Render2:         render,
		Reproducibility: PhaseResult{ID: "reproducibility", Result: ResultPass},
		Target: TargetEvidence{
			LogicalPath: "backend/things", Module: "gitee.com/ming-iot/things",
			BeforeTreeSHA256: strings.Repeat("2", 64), AfterTreeSHA256: strings.Repeat("2", 64), Differences: []TreeDifference{},
		},
		Phases:        phases,
		Gates:         gates,
		OverallResult: ResultPass,
	}
}

func minimalStructuralEvidenceForTest() Evidence {
	evidence := minimalValidEvidenceForTest()
	evidence.FinalEvidence = false
	evidence.OverallResult = ResultNotFinal
	evidence.Phases = make([]PhaseResult, len(structuralPhaseIDs))
	for index, id := range structuralPhaseIDs {
		evidence.Phases[index] = PhaseResult{ID: id, Result: ResultPass}
	}
	evidence.Gates = nil
	return evidence
}
