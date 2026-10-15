package thingsstartbaseline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMarshalEvidenceRedactsSecretsFromAllFreeText(t *testing.T) {
	secret := "super-secret-value"
	evidence := validEvidenceFixture()
	evidence.Stages[2].Component = "nats-" + secret
	evidence.Stages[2].Reason = "probe used " + secret
	evidence.Images["api"] = "sha256:" + secret

	data, err := MarshalEvidence(evidence, []string{secret})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(secret)) {
		t.Fatalf("MarshalEvidence() leaked a secret: %s", data)
	}
	if occurrences := bytes.Count(data, []byte("[REDACTED]")); occurrences != 3 {
		t.Fatalf("MarshalEvidence() redactions = %d, want 3: %s", occurrences, data)
	}
	var decoded Evidence
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("MarshalEvidence() returned invalid JSON: %v", err)
	}
	if len(decoded.Stages) != len(requiredStages) {
		t.Fatalf("decoded stages = %d, want %d", len(decoded.Stages), len(requiredStages))
	}
}

func TestMarshalEvidencePreservesLargeArtifactSize(t *testing.T) {
	evidence := validEvidenceFixture()
	evidence.Binaries[0].Size = 1<<53 + 1

	data, err := MarshalEvidence(evidence, nil)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Evidence
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Binaries[0].Size != evidence.Binaries[0].Size {
		t.Fatalf("artifact size = %d, want %d", decoded.Binaries[0].Size, evidence.Binaries[0].Size)
	}
}

func TestMarshalEvidenceRejectsIncompleteOrUnsafeEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Evidence)
	}{
		{
			name: "missing stage",
			mutate: func(evidence *Evidence) {
				evidence.Stages = evidence.Stages[:len(evidence.Stages)-1]
			},
		},
		{
			name: "out of order stage",
			mutate: func(evidence *Evidence) {
				evidence.Stages[0], evidence.Stages[1] = evidence.Stages[1], evidence.Stages[0]
			},
		},
		{
			name: "escaping log reference",
			mutate: func(evidence *Evidence) {
				evidence.Stages[0].LogRef = "logs/../private.log"
			},
		},
		{
			name: "failure without code",
			mutate: func(evidence *Evidence) {
				evidence.OverallResult = "FAIL"
				evidence.Stages[4].Result = "FAIL"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := validEvidenceFixture()
			test.mutate(&evidence)
			if _, err := MarshalEvidence(evidence, nil); err == nil {
				t.Fatalf("MarshalEvidence() accepted %s", test.name)
			}
		})
	}
}

func TestWriteEvidenceAtomicallyPublishesFinalFileAndLogsDirectory(t *testing.T) {
	runDirectory := filepath.Join(t.TempDir(), "run-001")
	evidence := validEvidenceFixture()
	if err := WriteEvidenceAtomically(runDirectory, evidence, nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(runDirectory, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded Evidence
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("published evidence is invalid JSON: %v", err)
	}
	if decoded.RunID != evidence.RunID {
		t.Fatalf("published run ID = %q, want %q", decoded.RunID, evidence.RunID)
	}
	logs, err := os.Stat(filepath.Join(runDirectory, "logs"))
	if err != nil || !logs.IsDir() {
		t.Fatalf("logs directory = %+v, %v", logs, err)
	}
	entries, err := os.ReadDir(runDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".evidence.json.tmp-") {
			t.Fatalf("temporary evidence file remained after publish: %s", entry.Name())
		}
	}
}

func TestMarshalEvidenceAcceptsExplicitNotRunStagesAfterFailure(t *testing.T) {
	evidence := validEvidenceFixture()
	evidence.OverallResult = "FAIL"
	evidence.Stages[5].Result = "FAIL"
	evidence.Stages[5].Code = CodeRoleNotReady
	evidence.Stages[5].Reason = "dm probe failed"
	for index := 6; index < len(evidence.Stages); index++ {
		evidence.Stages[index].Result = "NOT_RUN"
		evidence.Stages[index].StartedAt = time.Time{}
		evidence.Stages[index].FinishedAt = time.Time{}
		evidence.Stages[index].DurationMS = 0
		evidence.Stages[index].LogRef = ""
		evidence.Stages[index].Reason = "not run after DM_READY failure"
	}
	if _, err := MarshalEvidence(evidence, nil); err != nil {
		t.Fatalf("MarshalEvidence() rejected explicit not-run stages: %v", err)
	}
}

func TestWriteEvidenceBundleAtomicallyPublishesCompleteDirectory(t *testing.T) {
	secret := "bundle-secret-sentinel"
	evidence := validEvidenceFixture()
	evidence.Stages[0].Reason = "completed with " + secret
	runsRoot := filepath.Join(t.TempDir(), "runs")
	runDirectory := filepath.Join(runsRoot, evidence.RunID)
	bundle := EvidenceBundle{
		Evidence: evidence,
		Versions: map[string]string{
			"docker": "27.5.1", "compose": "2.32.4-" + secret,
			"redis": "7.4", "etcd": "3.5.21", "nats": "2.10", "tdengine": "3.3.6.13", "emqx": "5.8.9",
			"sys": "sys-image-id", "timedjob": "timedjob-image-id", "dm": "dm-image-id", "dg": "dg-image-id", "api": "api-image-id",
		},
		ComposeConfigSHA256: strings.Repeat("c", 64),
	}
	if err := WriteEvidenceBundleAtomically(runDirectory, bundle, []string{"", secret, secret}); err != nil {
		t.Fatal(err)
	}

	wantEntries := []string{"binary-sha256.json", "compose-config.sha256", "evidence.json", "logs", "versions.json"}
	entries, err := os.ReadDir(runDirectory)
	if err != nil {
		t.Fatal(err)
	}
	gotEntries := make([]string, 0, len(entries))
	for _, entry := range entries {
		gotEntries = append(gotEntries, entry.Name())
	}
	if strings.Join(gotEntries, ",") != strings.Join(wantEntries, ",") {
		t.Fatalf("bundle entries = %v, want %v", gotEntries, wantEntries)
	}
	for _, name := range []string{"evidence.json", "versions.json", "binary-sha256.json"} {
		data, err := os.ReadFile(filepath.Join(runDirectory, name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("%s leaked a secret: %s", name, data)
		}
	}
	for _, stage := range evidence.Stages {
		logData, err := os.ReadFile(filepath.Join(runDirectory, filepath.FromSlash(stage.LogRef)))
		if err != nil {
			t.Fatalf("read stage log %s: %v", stage.LogRef, err)
		}
		if !bytes.Contains(logData, []byte("stage="+string(stage.ID))) || bytes.Contains(logData, []byte(secret)) {
			t.Fatalf("stage log %s = %q", stage.LogRef, logData)
		}
	}
	composeDigest, err := os.ReadFile(filepath.Join(runDirectory, "compose-config.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if string(composeDigest) != strings.Repeat("c", 64)+"\n" {
		t.Fatalf("compose digest = %q", composeDigest)
	}
	var binaries struct {
		SchemaVersion int    `json:"schemaVersion"`
		GitRevision   string `json:"gitRevision"`
		Artifacts     []struct {
			Role   string `json:"role"`
			Binary string `json:"binary"`
			SHA256 string `json:"sha256"`
		} `json:"artifacts"`
	}
	binaryData, err := os.ReadFile(filepath.Join(runDirectory, "binary-sha256.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(binaryData, &binaries); err != nil {
		t.Fatal(err)
	}
	if binaries.SchemaVersion != 1 || binaries.GitRevision != evidence.GitRevision || len(binaries.Artifacts) != len(evidence.Binaries) {
		t.Fatalf("binary projection = %+v", binaries)
	}
	parentEntries, err := os.ReadDir(runsRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range parentEntries {
		if strings.HasPrefix(entry.Name(), "."+evidence.RunID+".tmp-") {
			t.Fatalf("temporary evidence bundle remained: %s", entry.Name())
		}
	}
}

func TestWriteEvidenceBundleRejectsIncompleteCompanionsBeforePublication(t *testing.T) {
	evidence := validEvidenceFixture()
	versions := make(map[string]string, len(requiredVersionComponents))
	for _, component := range requiredVersionComponents {
		versions[component] = component + "-version"
	}
	delete(versions, "emqx")
	runDirectory := filepath.Join(t.TempDir(), evidence.RunID)
	err := WriteEvidenceBundleAtomically(runDirectory, EvidenceBundle{
		Evidence:            evidence,
		Versions:            versions,
		ComposeConfigSHA256: strings.Repeat("c", 64),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "emqx") {
		t.Fatalf("WriteEvidenceBundleAtomically() error = %v, want missing emqx rejection", err)
	}
	if _, statErr := os.Stat(runDirectory); !os.IsNotExist(statErr) {
		t.Fatalf("run directory was published after validation failure: %v", statErr)
	}
}

func validEvidenceFixture() Evidence {
	started := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	stages := make([]StageEvidence, 0, len(requiredStages))
	for index, stage := range requiredStages {
		stageStarted := started.Add(time.Duration(index) * time.Second)
		stages = append(stages, StageEvidence{
			ID:         stage,
			Result:     "PASS",
			StartedAt:  stageStarted,
			FinishedAt: stageStarted.Add(time.Second),
			DurationMS: 1000,
			Component:  strings.ToLower(string(stage)),
			LogRef:     "logs/" + strings.ToLower(string(stage)) + ".log",
		})
	}
	hash := strings.Repeat("a", 64)
	artifacts := make([]Artifact, 0, len(requiredManifestArtifacts))
	images := make(map[string]string, len(requiredManifestArtifacts))
	for _, required := range requiredManifestArtifacts {
		artifacts = append(artifacts, Artifact{
			Role:      required.role,
			Binary:    required.binary,
			SHA256:    hash,
			Size:      1,
			StaticELF: true,
		})
		images[required.role] = "sha256:" + hash
	}
	return Evidence{
		SchemaVersion:  1,
		RunID:          "run-001",
		GitRevision:    strings.Repeat("b", 40),
		ContractSHA256: hash,
		OverallResult:  "PASS",
		Stages:         stages,
		Binaries:       artifacts,
		Images:         images,
	}
}
