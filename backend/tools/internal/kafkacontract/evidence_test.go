// 本文件验证 Evidence 的确定性、来源绑定、脏状态诚实性和脱敏边界。
package kafkacontract

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildEvidenceDigestIgnoresExecutionTime(t *testing.T) {
	input := validEvidenceInput()
	first, err := BuildEvidence(input)
	if err != nil {
		t.Fatalf("BuildEvidence() error = %v", err)
	}
	input.GeneratedAt = input.GeneratedAt.Add(24 * time.Hour)
	second, err := BuildEvidence(input)
	if err != nil {
		t.Fatalf("BuildEvidence() second error = %v", err)
	}
	if first.ContentDigest != second.ContentDigest {
		t.Fatalf("ContentDigest changed with time: %q != %q", first.ContentDigest, second.ContentDigest)
	}
}

func TestBuildEvidenceRecordsRequiredSourceVersionsChecksAndProbe(t *testing.T) {
	evidence, err := BuildEvidence(validEvidenceInput())
	if err != nil {
		t.Fatalf("BuildEvidence() error = %v", err)
	}
	if evidence.Source.Commit == "" || !evidence.Source.Dirty || evidence.Source.WorktreeDigest == "" {
		t.Fatalf("BuildEvidence() source = %#v", evidence.Source)
	}
	if evidence.Versions.Kafka != "4.3.1" || evidence.Versions.FranzGo != "v1.20.5" || evidence.Versions.Go != "1.24.4" {
		t.Fatalf("BuildEvidence() versions = %#v", evidence.Versions)
	}
	if evidence.Catalog.Entries != 7 || evidence.Catalog.Digest == "" || len(evidence.Verification) == 0 || evidence.Probe == nil {
		t.Fatalf("BuildEvidence() missing contract content: %#v", evidence)
	}
	if evidence.Redaction.Status != "pass" || evidence.ContentDigest == "" {
		t.Fatalf("BuildEvidence() redaction = %#v, digest = %q", evidence.Redaction, evidence.ContentDigest)
	}
}

func TestBuildEvidenceRejectsSecretAndAbsoluteTemporaryPath(t *testing.T) {
	for _, command := range []string{
		"probe --password=do-not-store",
		"probe --password do-not-store",
		"probe --token:do-not-store",
		"cat /tmp/raw-payload.json",
		"cat /home/example/private-key.pem",
		"cat /root/private-key.pem",
		`type C:\Users\example\private-key.pem`,
	} {
		input := validEvidenceInput()
		input.Verification[0].Command = command
		if _, err := BuildEvidence(input); err == nil {
			t.Errorf("BuildEvidence() accepted unsafe command %q", command)
		}
	}
}

func TestBuildEvidenceRejectsUnsafeValuesOutsideVerification(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*EvidenceInput)
	}{
		{name: "goal path", mutate: func(input *EvidenceInput) { input.GoalID = "/root/private-goal" }},
		{name: "todo secret", mutate: func(input *EvidenceInput) { input.TodoID = "--token do-not-store" }},
		{name: "version path", mutate: func(input *EvidenceInput) { input.Contract.KafkaVersion = `C:\Users\example\version` }},
		{name: "probe path", mutate: func(input *EvidenceInput) { input.Probe.Topic = "/home/example/probe" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validEvidenceInput()
			test.mutate(&input)
			if _, err := BuildEvidence(input); err == nil {
				t.Fatal("BuildEvidence() accepted an unsafe serialized field")
			}
		})
	}
}

func TestEvidenceJSONDoesNotContainRawPayloadOrCredential(t *testing.T) {
	evidence, err := BuildEvidence(validEvidenceInput())
	if err != nil {
		t.Fatalf("BuildEvidence() error = %v", err)
	}
	content, err := json.Marshal(evidence)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, forbidden := range []string{"must-not-leak", "password=", "private_key=", `{"kind":"contract-probe"}`} {
		if strings.Contains(string(content), forbidden) {
			t.Errorf("evidence JSON contains forbidden value %q: %s", forbidden, content)
		}
	}
}

func validEvidenceInput() EvidenceInput {
	probe := ProbeResult{
		Status:             "pass",
		Topic:              expectedProbeTopic,
		Partition:          0,
		Offset:             1,
		ProbeRecordSummary: summarizeProbeRecord(probeKey, probeHeaders, probeValue),
		ResourceDeleted:    true,
	}
	return EvidenceInput{
		GoalID:      "ming-iot-framework-v1",
		TodoID:      "todo_966a84160f43",
		GeneratedAt: time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC),
		Source: SourceState{
			Commit:         "925103538c189140862f4e13571dfcfcc66181e1",
			Dirty:          true,
			WorktreeDigest: strings.Repeat("a", 64),
		},
		Contract: Report{
			Status:          "pass",
			KafkaVersion:    "4.3.1",
			FranzGoVersion:  "v1.20.5",
			GoVersion:       "1.24.4",
			ImageDigest:     expectedImageDigest,
			CatalogEntries:  7,
			CatalogDigest:   strings.Repeat("b", 64),
			ContractVersion: 1,
			Issues:          []Issue{},
		},
		Verification: []VerificationResult{{Name: "focused-tests", Command: "go test ./tools/internal/kafkacontract", Result: "pass"}},
		Probe:        &probe,
	}
}
