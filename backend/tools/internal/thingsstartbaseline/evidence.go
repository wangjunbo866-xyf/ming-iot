package thingsstartbaseline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// StageEvidence records the outcome and timing of one ordered startup stage.
type StageEvidence struct {
	ID         StageID   `json:"id"`
	Result     string    `json:"result"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	DurationMS int64     `json:"durationMs"`
	Component  string    `json:"component,omitempty"`
	Code       ErrorCode `json:"code,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	LogRef     string    `json:"logRef,omitempty"`
}

// Evidence is the final, machine-readable record for one baseline run.
type Evidence struct {
	SchemaVersion  int               `json:"schemaVersion"`
	RunID          string            `json:"runId"`
	GitRevision    string            `json:"gitRevision"`
	ContractSHA256 string            `json:"contractSha256"`
	OverallResult  string            `json:"overallResult"`
	Stages         []StageEvidence   `json:"stages"`
	Binaries       []Artifact        `json:"binaries"`
	Images         map[string]string `json:"images"`
}

// EvidenceBundle contains the complete immutable publication for one run.
type EvidenceBundle struct {
	Evidence            Evidence
	Versions            map[string]string
	ComposeConfigSHA256 string
}

type versionsEvidence struct {
	SchemaVersion int               `json:"schemaVersion"`
	Components    map[string]string `json:"components"`
}

type binarySHA256Evidence struct {
	SchemaVersion int                    `json:"schemaVersion"`
	GitRevision   string                 `json:"gitRevision"`
	Artifacts     []binarySHA256Artifact `json:"artifacts"`
}

type binarySHA256Artifact struct {
	Role   string `json:"role"`
	Binary string `json:"binary"`
	SHA256 string `json:"sha256"`
}

var requiredVersionComponents = []string{
	"docker", "compose", "redis", "etcd", "nats", "tdengine", "emqx",
	"sys", "timedjob", "dm", "dg", "api",
}

// MarshalEvidence validates evidence and redacts caller-supplied secrets from
// every JSON string value before publication.
func MarshalEvidence(evidence Evidence, secrets []string) ([]byte, error) {
	if err := validateEvidence(evidence); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return nil, fmt.Errorf("marshal evidence: %w", err)
	}
	var document any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode evidence for redaction: %w", err)
	}
	document = redactJSONStrings(document, normalizedSecrets(secrets))
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal redacted evidence: %w", err)
	}
	return append(data, '\n'), nil
}

// WriteEvidenceAtomically publishes evidence.json and prepares its logs directory.
func WriteEvidenceAtomically(runDirectory string, evidence Evidence, secrets []string) error {
	data, err := MarshalEvidence(evidence, secrets)
	if err != nil {
		return err
	}
	if runDirectory == "" {
		return fmt.Errorf("run directory is empty")
	}
	if err := os.MkdirAll(runDirectory, 0o755); err != nil {
		return fmt.Errorf("create run directory %q: %w", runDirectory, err)
	}
	info, err := os.Lstat(runDirectory)
	if err != nil {
		return fmt.Errorf("inspect run directory %q: %w", runDirectory, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("run directory %q must be a real directory", runDirectory)
	}
	if err := os.MkdirAll(filepath.Join(runDirectory, "logs"), 0o755); err != nil {
		return fmt.Errorf("create evidence logs directory: %w", err)
	}

	temporary, err := os.CreateTemp(runDirectory, ".evidence.json.tmp-*")
	if err != nil {
		return fmt.Errorf("create evidence temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write evidence: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync evidence: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close evidence: %w", err)
	}
	if err := os.Rename(temporaryName, filepath.Join(runDirectory, "evidence.json")); err != nil {
		return fmt.Errorf("publish evidence: %w", err)
	}
	if err := syncManifestDirectory(runDirectory); err != nil {
		return fmt.Errorf("sync evidence directory: %w", err)
	}
	return nil
}

// WriteEvidenceBundleAtomically publishes the complete run directory with one
// directory rename, so readers never observe a partial evidence generation.
func WriteEvidenceBundleAtomically(runDirectory string, bundle EvidenceBundle, secrets []string) error {
	secrets = normalizedSecrets(secrets)
	evidenceData, err := MarshalEvidence(bundle.Evidence, secrets)
	if err != nil {
		return err
	}
	if !isHexDigest(bundle.ComposeConfigSHA256, sha256.Size*2) {
		return fmt.Errorf("compose config SHA-256 must contain 64 hexadecimal characters")
	}
	for name, value := range bundle.Versions {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(value) == "" {
			return fmt.Errorf("version component names and values must not be empty")
		}
	}
	for _, name := range requiredVersionComponents {
		if strings.TrimSpace(bundle.Versions[name]) == "" {
			return fmt.Errorf("version component %s is missing", name)
		}
	}
	if len(bundle.Versions) != len(requiredVersionComponents) {
		return fmt.Errorf("versions must contain exactly %d fixed components", len(requiredVersionComponents))
	}
	cleanRunDirectory, err := filepath.Abs(runDirectory)
	if err != nil {
		return fmt.Errorf("resolve run directory: %w", err)
	}
	cleanRunDirectory = filepath.Clean(cleanRunDirectory)
	if filepath.Base(cleanRunDirectory) != bundle.Evidence.RunID {
		return fmt.Errorf("run directory must end with evidence run ID")
	}
	parent := filepath.Dir(cleanRunDirectory)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create evidence runs directory: %w", err)
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("evidence runs directory must be a real directory")
	}
	if _, err := os.Lstat(cleanRunDirectory); !os.IsNotExist(err) {
		if err == nil {
			return fmt.Errorf("run directory already exists")
		}
		return fmt.Errorf("inspect run directory: %w", err)
	}

	temporary, err := os.MkdirTemp(parent, "."+bundle.Evidence.RunID+".tmp-")
	if err != nil {
		return fmt.Errorf("create evidence bundle temporary directory: %w", err)
	}
	temporaryOwned := true
	defer func() {
		if temporaryOwned {
			_ = os.RemoveAll(temporary)
		}
	}()
	if err := os.Mkdir(filepath.Join(temporary, "logs"), 0o755); err != nil {
		return fmt.Errorf("create evidence logs directory: %w", err)
	}
	versionsData, err := marshalRedactedJSON(versionsEvidence{SchemaVersion: 1, Components: bundle.Versions}, secrets)
	if err != nil {
		return fmt.Errorf("marshal versions evidence: %w", err)
	}
	binaryArtifacts := make([]binarySHA256Artifact, 0, len(bundle.Evidence.Binaries))
	for _, artifact := range bundle.Evidence.Binaries {
		binaryArtifacts = append(binaryArtifacts, binarySHA256Artifact{Role: artifact.Role, Binary: artifact.Binary, SHA256: artifact.SHA256})
	}
	binaryData, err := marshalRedactedJSON(binarySHA256Evidence{
		SchemaVersion: 1,
		GitRevision:   bundle.Evidence.GitRevision,
		Artifacts:     binaryArtifacts,
	}, secrets)
	if err != nil {
		return fmt.Errorf("marshal binary SHA-256 evidence: %w", err)
	}
	files := map[string][]byte{
		"evidence.json":         evidenceData,
		"versions.json":         versionsData,
		"compose-config.sha256": []byte(strings.ToLower(bundle.ComposeConfigSHA256) + "\n"),
		"binary-sha256.json":    binaryData,
	}
	for name, data := range files {
		if err := writeSyncedFile(filepath.Join(temporary, name), data); err != nil {
			return err
		}
	}
	for _, stage := range bundle.Evidence.Stages {
		if stage.LogRef == "" {
			continue
		}
		logData := []byte(fmt.Sprintf("stage=%s\nresult=%s\ncode=%s\nreason=%s\n", stage.ID, stage.Result, stage.Code, commandRedact(stage.Reason, secrets)))
		logPath := filepath.Join(temporary, filepath.FromSlash(stage.LogRef))
		if !pathWithin(temporary, logPath) {
			return fmt.Errorf("evidence log reference %q escapes the run directory", stage.LogRef)
		}
		if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
			return fmt.Errorf("create evidence log directory: %w", err)
		}
		if err := writeSyncedFile(logPath, logData); err != nil {
			return err
		}
		if err := syncManifestDirectory(filepath.Dir(logPath)); err != nil {
			return fmt.Errorf("sync evidence log directory: %w", err)
		}
	}
	if err := syncManifestDirectory(filepath.Join(temporary, "logs")); err != nil {
		return fmt.Errorf("sync evidence logs directory: %w", err)
	}
	if err := syncManifestDirectory(temporary); err != nil {
		return fmt.Errorf("sync evidence bundle directory: %w", err)
	}
	if err := os.Rename(temporary, cleanRunDirectory); err != nil {
		return fmt.Errorf("publish evidence bundle: %w", err)
	}
	temporaryOwned = false
	if err := syncManifestDirectory(parent); err != nil {
		return fmt.Errorf("sync evidence runs directory: %w", err)
	}
	return nil
}

func marshalRedactedJSON(value any, secrets []string) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var document any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	document = redactJSONStrings(document, secrets)
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func writeSyncedFile(filename string, data []byte) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create evidence file %q: %w", filepath.Base(filename), err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write evidence file %q: %w", filepath.Base(filename), err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync evidence file %q: %w", filepath.Base(filename), err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close evidence file %q: %w", filepath.Base(filename), err)
	}
	return nil
}

func validateEvidence(evidence Evidence) error {
	if evidence.SchemaVersion != 1 {
		return fmt.Errorf("evidence schema version must be 1")
	}
	if !isSafeRunID(evidence.RunID) {
		return fmt.Errorf("evidence run ID is empty or unsafe")
	}
	if !isHexDigest(evidence.GitRevision, 40) && !isHexDigest(evidence.GitRevision, 64) {
		return fmt.Errorf("evidence Git revision must be a 40 or 64 character hexadecimal digest")
	}
	if !isHexDigest(evidence.ContractSHA256, sha256.Size*2) {
		return fmt.Errorf("evidence contract SHA-256 must contain 64 hexadecimal characters")
	}
	if evidence.OverallResult != "PASS" && evidence.OverallResult != "FAIL" {
		return fmt.Errorf("evidence overall result must be PASS or FAIL")
	}
	if len(evidence.Stages) != len(requiredStages) {
		return fmt.Errorf("evidence must contain exactly %d stages", len(requiredStages))
	}
	hasFailure := false
	hasNotRun := false
	for index, stage := range evidence.Stages {
		if stage.ID != requiredStages[index] {
			return fmt.Errorf("evidence stage %d must be %s", index, requiredStages[index])
		}
		if stage.Result != "PASS" && stage.Result != "FAIL" && stage.Result != "NOT_RUN" {
			return fmt.Errorf("evidence stage %s result must be PASS, FAIL, or NOT_RUN", stage.ID)
		}
		if stage.Result == "NOT_RUN" {
			hasNotRun = true
			if !hasFailure {
				return fmt.Errorf("evidence stage %s is NOT_RUN before a failed stage", stage.ID)
			}
			if !stage.StartedAt.IsZero() || !stage.FinishedAt.IsZero() || stage.DurationMS != 0 || stage.Code != "" || stage.Reason == "" || stage.LogRef != "" {
				return fmt.Errorf("evidence NOT_RUN stage %s has invalid fields", stage.ID)
			}
			continue
		}
		if hasFailure {
			return fmt.Errorf("evidence stage %s ran after a failed stage", stage.ID)
		}
		if stage.StartedAt.IsZero() || stage.FinishedAt.IsZero() || stage.FinishedAt.Before(stage.StartedAt) {
			return fmt.Errorf("evidence stage %s has invalid timestamps", stage.ID)
		}
		if stage.DurationMS < 0 {
			return fmt.Errorf("evidence stage %s duration must not be negative", stage.ID)
		}
		if stage.Result == "FAIL" {
			hasFailure = true
			if stage.Code == "" {
				return fmt.Errorf("evidence failed stage %s requires an error code", stage.ID)
			}
		} else if stage.Code != "" {
			return fmt.Errorf("evidence passing stage %s must not contain an error code", stage.ID)
		}
		if stage.LogRef != "" && !isSafeLogReference(stage.LogRef) {
			return fmt.Errorf("evidence stage %s log reference is unsafe", stage.ID)
		}
	}
	if evidence.OverallResult == "PASS" && hasFailure {
		return fmt.Errorf("evidence PASS result contains a failed stage")
	}
	if evidence.OverallResult == "PASS" && hasNotRun {
		return fmt.Errorf("evidence PASS result contains a NOT_RUN stage")
	}
	if evidence.OverallResult == "FAIL" && !hasFailure {
		return fmt.Errorf("evidence FAIL result contains no failed stage")
	}
	if err := validateBuildManifest(BuildManifest{
		SchemaVersion: 1,
		GitRevision:   evidence.GitRevision,
		Artifacts:     evidence.Binaries,
	}); err != nil {
		return fmt.Errorf("validate evidence binaries: %w", err)
	}
	if len(evidence.Images) != len(requiredManifestArtifacts) {
		return fmt.Errorf("evidence must contain exactly %d images", len(requiredManifestArtifacts))
	}
	for _, required := range requiredManifestArtifacts {
		if strings.TrimSpace(evidence.Images[required.role]) == "" {
			return fmt.Errorf("evidence image %s is empty", required.role)
		}
	}
	return nil
}

func isSafeRunID(runID string) bool {
	return runID != "" && runID != "." && runID != ".." && filepath.Base(runID) == runID
}

func isHexDigest(value string, length int) bool {
	if len(value) != length {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func isSafeLogReference(reference string) bool {
	if filepath.IsAbs(reference) || filepath.Separator != '/' && strings.ContainsRune(reference, filepath.Separator) {
		return false
	}
	cleaned := filepath.ToSlash(filepath.Clean(reference))
	return cleaned == reference && strings.HasPrefix(cleaned, "logs/") && cleaned != "logs/"
}

func redactJSONStrings(value any, secrets []string) any {
	switch typed := value.(type) {
	case string:
		for _, secret := range secrets {
			if secret != "" {
				typed = strings.ReplaceAll(typed, secret, "[REDACTED]")
			}
		}
		return typed
	case []any:
		for index := range typed {
			typed[index] = redactJSONStrings(typed[index], secrets)
		}
		return typed
	case map[string]any:
		for key := range typed {
			typed[key] = redactJSONStrings(typed[key], secrets)
		}
		return typed
	default:
		return value
	}
}

func normalizedSecrets(secrets []string) []string {
	unique := make(map[string]struct{}, len(secrets))
	for _, secret := range secrets {
		if secret != "" {
			unique[secret] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for secret := range unique {
		result = append(result, secret)
	}
	sort.Slice(result, func(left, right int) bool { return len(result[left]) > len(result[right]) })
	return result
}
