// 本文件实现 Kafka 基础 Loop 的确定性、可脱敏 Evidence 产物。
package kafkacontract

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const evidenceSchemaVersion = "ming-iot.kafka-foundation-contract-sdk-evidence.v1"

var (
	secretAssignmentPattern = regexp.MustCompile(`(?i)(?:--)?(?:password|token|private[_-]?key)(?:\s*[:=]\s*|\s+)[^\s",}]+`)
	windowsPathPattern      = regexp.MustCompile(`(?i)[a-z]:\\`)
	hex40Pattern            = regexp.MustCompile(`^[0-9a-f]{40}$`)
	hex64Pattern            = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// SourceState 将 Evidence 绑定到 Git 基线和当前未提交候选内容。
type SourceState struct {
	Commit         string `json:"commit"`
	Dirty          bool   `json:"dirty"`
	WorktreeDigest string `json:"worktree_digest"`
}

// VerificationResult 是不包含原始日志的机器验证摘要。
type VerificationResult struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Result  string `json:"result"`
}

// EvidenceInput 描述构造 Evidence 所需的已验证输入。
type EvidenceInput struct {
	GoalID       string
	TodoID       string
	GeneratedAt  time.Time
	Source       SourceState
	Contract     Report
	Verification []VerificationResult
	Probe        *ProbeResult
}

// EvidenceVersions 记录当前冲突裁决后的版本集合。
type EvidenceVersions struct {
	Kafka       string `json:"kafka"`
	FranzGo     string `json:"franz_go"`
	Go          string `json:"go"`
	ImageDigest string `json:"image_digest"`
}

// EvidenceCatalog 记录受管 Topic Catalog 的规模和确定性摘要。
type EvidenceCatalog struct {
	Entries int    `json:"entries"`
	Digest  string `json:"digest"`
}

// RedactionResult 证明输出结构经过敏感字段和本机路径检查。
type RedactionResult struct {
	Status        string   `json:"status"`
	RejectedKinds []string `json:"rejected_kinds"`
}

// Evidence 是 Kafka 基础 Loop 的机器可解析证明包。
type Evidence struct {
	SchemaVersion string               `json:"schema_version"`
	GoalID        string               `json:"goal_id"`
	TodoID        string               `json:"todo_id"`
	GeneratedAt   string               `json:"generated_at"`
	Status        string               `json:"status"`
	Source        SourceState          `json:"source"`
	Versions      EvidenceVersions     `json:"versions"`
	Catalog       EvidenceCatalog      `json:"catalog"`
	Verification  []VerificationResult `json:"verification"`
	Probe         *ProbeResult         `json:"probe,omitempty"`
	FailureCodes  []string             `json:"failure_codes"`
	Redaction     RedactionResult      `json:"redaction"`
	ContentDigest string               `json:"content_digest"`
}

// BuildEvidence 校验输入边界并生成不受执行时间影响的内容摘要。
func BuildEvidence(input EvidenceInput) (Evidence, error) {
	if strings.TrimSpace(input.GoalID) == "" || strings.TrimSpace(input.TodoID) == "" {
		return Evidence{}, fmt.Errorf("goal ID and todo ID are required")
	}
	if !hex40Pattern.MatchString(input.Source.Commit) {
		return Evidence{}, fmt.Errorf("source commit must be a 40-character lowercase Git hash")
	}
	if !hex64Pattern.MatchString(input.Source.WorktreeDigest) {
		return Evidence{}, fmt.Errorf("worktree digest must be a 64-character lowercase SHA-256")
	}
	if input.Contract.CatalogDigest == "" || input.Contract.KafkaVersion == "" || input.Contract.FranzGoVersion == "" || input.Contract.GoVersion == "" {
		return Evidence{}, fmt.Errorf("contract versions and catalog digest are required")
	}

	verification := append([]VerificationResult(nil), input.Verification...)
	sort.SliceStable(verification, func(i, j int) bool { return verification[i].Name < verification[j].Name })
	for _, check := range verification {
		if strings.TrimSpace(check.Name) == "" || strings.TrimSpace(check.Command) == "" {
			return Evidence{}, fmt.Errorf("verification name and command are required")
		}
		if check.Result != "pass" && check.Result != "fail" {
			return Evidence{}, fmt.Errorf("verification %s result must be pass or fail", check.Name)
		}
		if unsafeEvidenceString(check.Command) {
			return Evidence{}, fmt.Errorf("verification %s command contains a credential value or local absolute path", check.Name)
		}
	}

	failureCodes := make([]string, 0)
	for _, issue := range input.Contract.Issues {
		failureCodes = append(failureCodes, issue.Code)
	}
	for _, check := range verification {
		if check.Result == "fail" {
			failureCodes = append(failureCodes, "VERIFICATION_"+stableCodeToken(check.Name)+"_FAILED")
		}
	}
	if input.Probe != nil {
		if input.Probe.Status != "pass" || !input.Probe.ResourceDeleted {
			failureCodes = append(failureCodes, "KAFKA_PROBE_FAILED")
		}
		if input.Probe.KeyDigest == "" || input.Probe.HeaderDigest == "" || input.Probe.ValueDigest == "" {
			return Evidence{}, fmt.Errorf("probe digests are required")
		}
	}
	sort.Strings(failureCodes)
	failureCodes = uniqueStrings(failureCodes)

	status := "pass"
	if !input.Contract.OK() || len(failureCodes) > 0 {
		status = "fail"
	}
	evidence := Evidence{
		SchemaVersion: evidenceSchemaVersion,
		GoalID:        input.GoalID,
		TodoID:        input.TodoID,
		GeneratedAt:   input.GeneratedAt.UTC().Format(time.RFC3339Nano),
		Status:        status,
		Source:        input.Source,
		Versions: EvidenceVersions{
			Kafka:       input.Contract.KafkaVersion,
			FranzGo:     input.Contract.FranzGoVersion,
			Go:          input.Contract.GoVersion,
			ImageDigest: input.Contract.ImageDigest,
		},
		Catalog: EvidenceCatalog{
			Entries: input.Contract.CatalogEntries,
			Digest:  input.Contract.CatalogDigest,
		},
		Verification: verification,
		Probe:        cloneProbeResult(input.Probe),
		FailureCodes: failureCodes,
		Redaction:    RedactionResult{Status: "pass", RejectedKinds: []string{}},
	}
	serialized, err := json.Marshal(evidence)
	if err != nil {
		return Evidence{}, fmt.Errorf("marshal evidence for redaction scan: %w", err)
	}
	if unsafeEvidenceString(string(serialized)) {
		return Evidence{}, fmt.Errorf("evidence contains a credential value or local absolute path")
	}
	evidence.ContentDigest = evidenceContentDigest(evidence)
	return evidence, nil
}

// CollectSourceState 读取 Git HEAD、dirty 状态和包括未跟踪文件在内的候选内容摘要。
func CollectSourceState(ctx context.Context, repoRoot string) (SourceState, error) {
	commit, err := runGit(ctx, repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return SourceState{}, err
	}
	commit = strings.TrimSpace(commit)
	status, err := runGitBytes(ctx, repoRoot, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return SourceState{}, err
	}
	diff, err := runGitBytes(ctx, repoRoot, "diff", "--binary", "--no-ext-diff", "HEAD", "--")
	if err != nil {
		return SourceState{}, err
	}
	untrackedRaw, err := runGitBytes(ctx, repoRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return SourceState{}, err
	}
	untracked := splitNUL(untrackedRaw)
	sort.Strings(untracked)
	hash := sha256.New()
	writeLengthPrefixed(hash, []byte("tracked-diff"))
	writeLengthPrefixed(hash, diff)
	for _, relativePath := range untracked {
		content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			return SourceState{}, fmt.Errorf("read untracked source %s: %w", relativePath, err)
		}
		writeLengthPrefixed(hash, []byte(relativePath))
		writeLengthPrefixed(hash, content)
	}
	return SourceState{
		Commit:         commit,
		Dirty:          len(status) > 0,
		WorktreeDigest: hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

// WriteEvidence 以临时文件加原子重命名方式写入私有 Evidence 路径。
func WriteEvidence(path string, evidence Evidence) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("evidence output path is required")
	}
	content, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal evidence: %w", err)
	}
	content = append(content, '\n')
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create evidence directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".evidence-*.json")
	if err != nil {
		return fmt.Errorf("create temporary evidence: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect temporary evidence: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary evidence: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary evidence: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish evidence: %w", err)
	}
	return nil
}

func evidenceContentDigest(evidence Evidence) string {
	canonical := struct {
		SchemaVersion string               `json:"schema_version"`
		GoalID        string               `json:"goal_id"`
		TodoID        string               `json:"todo_id"`
		Status        string               `json:"status"`
		Source        SourceState          `json:"source"`
		Versions      EvidenceVersions     `json:"versions"`
		Catalog       EvidenceCatalog      `json:"catalog"`
		Verification  []VerificationResult `json:"verification"`
		Probe         *ProbeResult         `json:"probe,omitempty"`
		FailureCodes  []string             `json:"failure_codes"`
		Redaction     RedactionResult      `json:"redaction"`
	}{
		SchemaVersion: evidence.SchemaVersion,
		GoalID:        evidence.GoalID,
		TodoID:        evidence.TodoID,
		Status:        evidence.Status,
		Source:        evidence.Source,
		Versions:      evidence.Versions,
		Catalog:       evidence.Catalog,
		Verification:  evidence.Verification,
		Probe:         evidence.Probe,
		FailureCodes:  evidence.FailureCodes,
		Redaction:     evidence.Redaction,
	}
	content, _ := json.Marshal(canonical)
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func unsafeEvidenceString(value string) bool {
	if secretAssignmentPattern.MatchString(value) || windowsPathPattern.MatchString(value) {
		return true
	}
	for _, prefix := range []string{"/tmp/", "/home/", "/root/", "/Users/"} {
		if strings.Contains(value, prefix) {
			return true
		}
	}
	return false
}

func stableCodeToken(value string) string {
	var builder strings.Builder
	for _, char := range strings.ToUpper(value) {
		if char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			builder.WriteRune(char)
		} else if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "_") {
			builder.WriteByte('_')
		}
	}
	return strings.Trim(builder.String(), "_")
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func cloneProbeResult(probe *ProbeResult) *ProbeResult {
	if probe == nil {
		return nil
	}
	clone := *probe
	return &clone
}

func runGit(ctx context.Context, repoRoot string, args ...string) (string, error) {
	content, err := runGitBytes(ctx, repoRoot, args...)
	return string(content), err
}

func runGitBytes(ctx context.Context, repoRoot string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"-C", repoRoot}, args...)
	command := exec.CommandContext(ctx, "git", commandArgs...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	content, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return content, nil
}

func splitNUL(content []byte) []string {
	parts := bytes.Split(content, []byte{0})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > 0 {
			result = append(result, string(part))
		}
	}
	return result
}

func writeLengthPrefixed(hash interface{ Write([]byte) (int, error) }, content []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(content)))
	_, _ = hash.Write(length[:])
	_, _ = hash.Write(content)
}
