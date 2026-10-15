// 本文件验证 kafkacontract CLI 的纯契约模式和显式集成失败路径。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ming-iot/backend/tools/internal/kafkacontract"
)

func TestRunPureContractModeProducesJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"--repo-root", commandRepositoryRoot(t), "--format", "json"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, stderr = %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"status": "pass"`) || !strings.Contains(stdout.String(), `"kafka_version": "4.3.1"`) {
		t.Fatalf("run() stdout = %s", stdout.String())
	}
}

func TestRunWritesPrivateEvidenceWithExplicitVerification(t *testing.T) {
	evidencePath := filepath.Join(t.TempDir(), "evidence.json")
	repoRoot := commandRepositoryRoot(t)
	expectedSource, err := kafkacontract.CollectSourceState(context.Background(), repoRoot)
	if err != nil {
		t.Fatalf("collect source state: %v", err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"--repo-root", repoRoot,
		"--evidence-output", evidencePath,
		"--goal-id", "ming-iot-framework-v1",
		"--todo-id", "todo_966a84160f43",
		"--verification", "focused-tests|pass|go test ./tools/internal/kafkacontract",
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, stderr = %s", exitCode, stderr.String())
	}
	content, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatalf("read evidence: %v", err)
	}
	var evidence kafkacontract.Evidence
	if err := json.Unmarshal(content, &evidence); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if evidence.Source != expectedSource {
		t.Errorf("evidence source = %+v, want %+v", evidence.Source, expectedSource)
	}
	if evidence.SchemaVersion != "ming-iot.kafka-foundation-contract-sdk-evidence.v1" {
		t.Errorf("evidence schema version = %q", evidence.SchemaVersion)
	}
	foundFocusedTests := false
	for _, verification := range evidence.Verification {
		if verification.Name == "focused-tests" {
			foundFocusedTests = true
			break
		}
	}
	if !foundFocusedTests {
		t.Errorf("evidence missing focused-tests verification: %s", content)
	}
}

func TestRunIntegrationModeDoesNotSkipUnavailableBroker(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"--repo-root", commandRepositoryRoot(t),
		"--integration",
		"--brokers", "127.0.0.1:1",
		"--timeout", "500ms",
	}, &stdout, &stderr)
	if exitCode == 0 {
		t.Fatalf("run() exit = 0, stdout = %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "KAFKA_PROBE_UNAVAILABLE") {
		t.Fatalf("run() stderr = %s", stderr.String())
	}
}

func commandRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate command test")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(filename), "..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}
