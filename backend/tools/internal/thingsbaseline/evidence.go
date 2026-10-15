package thingsbaseline

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const EvidenceSchemaVersion = 1

var finalPhaseIDs = [...]string{
	"source-read", "render-1", "render-2", "reproducibility", "target-compare",
	"isolation-copy", "command-gates", "isolation-unchanged", "module-clean",
	"target-unchanged", "source-unchanged",
}

var structuralPhaseIDs = [...]string{
	"source-read", "render-1", "render-2", "reproducibility", "target-compare",
	"target-unchanged", "source-unchanged",
}

var finalGateIDs = [...]string{"tools-test", "saas-test", "things-test", "workspace-test", "things-build"}

var renderCommandNames = [...]string{"go-mod-tidy", "go-mod-verify"}

type Result string

const (
	ResultPass     Result = "PASS"
	ResultFail     Result = "FAIL"
	ResultNotRun   Result = "NOT_RUN"
	ResultNotFinal Result = "NOT_FINAL"
)

type Execution string

const (
	ExecutionExited     Execution = "EXITED"
	ExecutionSpawnError Execution = "SPAWN_ERROR"
	ExecutionTimeout    Execution = "TIMEOUT"
	ExecutionCancelled  Execution = "CANCELLED"
	ExecutionNotRun     Execution = "NOT_RUN"
)

type PhaseResult struct {
	ID          string `json:"id"`
	Result      Result `json:"result"`
	FailureKind string `json:"failureKind,omitempty"`
	Summary     string `json:"summary,omitempty"`
}

type GateArtifact struct {
	LogicalPath string `json:"logicalPath"`
	Mode        string `json:"mode"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	GoVersion   string `json:"goVersion"`
}

type GateResult struct {
	ID         string         `json:"id"`
	Command    string         `json:"command"`
	Result     Result         `json:"result"`
	Execution  Execution      `json:"execution"`
	ExitCode   *int           `json:"exitCode"`
	LogSHA256  string         `json:"logSha256,omitempty"`
	Summary    string         `json:"summary,omitempty"`
	DurationMS int64          `json:"durationMs"`
	Artifacts  []GateArtifact `json:"artifacts,omitempty"`
}

type SourceEvidence struct {
	Repository        string                `json:"repository"`
	Ref               string                `json:"ref"`
	Commit            string                `json:"commit"`
	Tree              string                `json:"tree"`
	Before            SourceRepositoryState `json:"before"`
	After             SourceRepositoryState `json:"after"`
	SelectedFiles     int                   `json:"selectedFiles"`
	SelectedBlobBytes int64                 `json:"selectedBlobBytes"`
	ArchiveSHA256     string                `json:"archiveSha256"`
}

type RenderEvidence struct {
	ManifestSHA256 string                  `json:"manifestSha256"`
	TreeSHA256     string                  `json:"treeSha256"`
	Files          int                     `json:"files"`
	Transform      TransformReport         `json:"transform"`
	Credentials    CredentialProfileReport `json:"credentials"`
	Scan           SecurityScanReport      `json:"scan"`
	Commands       []CommandReport         `json:"commands"`
}

type TargetEvidence struct {
	LogicalPath       string           `json:"logicalPath"`
	Module            string           `json:"module"`
	BeforeTreeSHA256  string           `json:"beforeTreeSha256,omitempty"`
	AfterTreeSHA256   string           `json:"afterTreeSha256,omitempty"`
	TotalDifferences  int              `json:"totalDifferences"`
	Differences       []TreeDifference `json:"differences"`
	DifferencesCutOff bool             `json:"differencesTruncated"`
}

type Evidence struct {
	SchemaVersion   int             `json:"schemaVersion"`
	LoopID          string          `json:"loopId"`
	FinalEvidence   bool            `json:"finalEvidence"`
	StartedAt       time.Time       `json:"startedAt"`
	FinishedAt      time.Time       `json:"finishedAt"`
	DurationMS      int64           `json:"durationMs"`
	Source          SourceEvidence  `json:"source"`
	Render1         RenderEvidence  `json:"render1"`
	Render2         RenderEvidence  `json:"render2"`
	Reproducibility PhaseResult     `json:"reproducibility"`
	Target          TargetEvidence  `json:"target"`
	Transform       TransformReport `json:"transform"`
	Contracts       ContractCounts  `json:"contracts"`
	Phases          []PhaseResult   `json:"phases"`
	Gates           []GateResult    `json:"gates"`
	OverallResult   Result          `json:"overallResult"`
}

func MarshalEvidence(evidence Evidence) ([]byte, error) {
	normalizeEvidenceCollections(&evidence)
	if err := validateEvidence(evidence); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal evidence: %w", err)
	}
	return append(data, '\n'), nil
}

func normalizeEvidenceCollections(evidence *Evidence) {
	if evidence.Phases == nil {
		evidence.Phases = []PhaseResult{}
	}
	if evidence.Gates == nil {
		evidence.Gates = []GateResult{}
	}
	if evidence.Target.Differences == nil {
		evidence.Target.Differences = []TreeDifference{}
	}
	if evidence.Render1.Commands == nil {
		evidence.Render1.Commands = []CommandReport{}
	}
	if evidence.Render2.Commands == nil {
		evidence.Render2.Commands = []CommandReport{}
	}
}

func validateEvidence(evidence Evidence) error {
	if evidence.SchemaVersion != EvidenceSchemaVersion {
		return fmt.Errorf("evidence schemaVersion = %d, want %d", evidence.SchemaVersion, EvidenceSchemaVersion)
	}
	if evidence.LoopID != "L0" {
		return fmt.Errorf("evidence loopId = %q, want L0", evidence.LoopID)
	}
	if evidence.StartedAt.IsZero() || evidence.FinishedAt.Before(evidence.StartedAt) || evidence.DurationMS < 0 {
		return fmt.Errorf("evidence time range is invalid")
	}
	if evidence.FinishedAt.Sub(evidence.StartedAt).Milliseconds() != evidence.DurationMS {
		return fmt.Errorf("evidence durationMs does not match startedAt/finishedAt")
	}
	if evidence.FinalEvidence {
		if evidence.OverallResult != ResultPass && evidence.OverallResult != ResultFail {
			return fmt.Errorf("final evidence overallResult must be PASS or FAIL")
		}
	} else if evidence.OverallResult != ResultNotFinal && evidence.OverallResult != ResultFail {
		return fmt.Errorf("non-final evidence overallResult must be NOT_FINAL or FAIL")
	}
	if evidence.Target.LogicalPath != "backend/things" || evidence.Target.Module == "" {
		return fmt.Errorf("evidence target must use logical backend/things path and module")
	}
	seenPhases := make(map[string]struct{}, len(evidence.Phases))
	for _, phase := range evidence.Phases {
		if phase.ID == "" {
			return fmt.Errorf("evidence phase id is empty")
		}
		if _, exists := seenPhases[phase.ID]; exists {
			return fmt.Errorf("duplicate evidence phase id %q", phase.ID)
		}
		seenPhases[phase.ID] = struct{}{}
		if phase.Result != ResultPass && phase.Result != ResultFail && phase.Result != ResultNotRun {
			return fmt.Errorf("phase %q has invalid result %q", phase.ID, phase.Result)
		}
	}
	seenGates := make(map[string]struct{}, len(evidence.Gates))
	for _, gate := range evidence.Gates {
		if gate.ID == "" || gate.Command == "" || gate.DurationMS < 0 {
			return fmt.Errorf("evidence gate identity/duration is invalid")
		}
		if _, exists := seenGates[gate.ID]; exists {
			return fmt.Errorf("duplicate evidence gate id %q", gate.ID)
		}
		seenGates[gate.ID] = struct{}{}
		if err := validateGateResult(gate); err != nil {
			return fmt.Errorf("gate %q: %w", gate.ID, err)
		}
		if gate.LogSHA256 != "" && !isLowercaseHex(gate.LogSHA256, 64) {
			return fmt.Errorf("gate %q logSha256 is invalid", gate.ID)
		}
	}
	if err := validateRenderCommandDiagnostics("render1", evidence.Render1.Commands); err != nil {
		return err
	}
	if err := validateRenderCommandDiagnostics("render2", evidence.Render2.Commands); err != nil {
		return err
	}
	if evidence.FinalEvidence && evidence.OverallResult == ResultPass {
		return validateFinalPassEvidence(evidence)
	}
	if !evidence.FinalEvidence && evidence.OverallResult == ResultNotFinal {
		return validateStructuralEvidence(evidence)
	}
	return nil
}

func validateRenderCommandDiagnostics(render string, commands []CommandReport) error {
	for index, command := range commands {
		if len(command.StdoutTail) > maxCommandDiagnosticTail {
			return fmt.Errorf("%s command %d stdoutTail exceeds %d bytes", render, index, maxCommandDiagnosticTail)
		}
		if len(command.StderrTail) > maxCommandDiagnosticTail {
			return fmt.Errorf("%s command %d stderrTail exceeds %d bytes", render, index, maxCommandDiagnosticTail)
		}
		if len(command.StdoutTail)+len(command.StderrTail) > maxCommandDiagnosticTotal {
			return fmt.Errorf("%s command %d diagnostic tails exceed %d bytes", render, index, maxCommandDiagnosticTotal)
		}
		if !utf8.ValidString(command.StdoutTail) || !utf8.ValidString(command.StderrTail) {
			return fmt.Errorf("%s command %d diagnostic tail is not valid UTF-8", render, index)
		}
		if command.StdoutSHA256 != "" && !isLowercaseHex(command.StdoutSHA256, 64) {
			return fmt.Errorf("%s command %d stdoutSha256 is invalid", render, index)
		}
		if command.StderrSHA256 != "" && !isLowercaseHex(command.StderrSHA256, 64) {
			return fmt.Errorf("%s command %d stderrSha256 is invalid", render, index)
		}
	}
	return nil
}

func validateFinalPassEvidence(evidence Evidence) error {
	if err := validatePassPhases(evidence.Phases, finalPhaseIDs[:]); err != nil {
		return err
	}
	if len(evidence.Gates) != len(finalGateIDs) {
		return fmt.Errorf("final PASS gates count = %d, want %d", len(evidence.Gates), len(finalGateIDs))
	}
	for index, id := range finalGateIDs {
		gate := evidence.Gates[index]
		if gate.ID != id {
			return fmt.Errorf("final PASS gates are not in the fixed order at index %d", index)
		}
		if gate.Result != ResultPass || gate.Execution != ExecutionExited || gate.ExitCode == nil || *gate.ExitCode != 0 {
			return fmt.Errorf("final PASS gate %q is not PASS/EXITED/0", id)
		}
		if !isLowercaseHex(gate.LogSHA256, 64) {
			return fmt.Errorf("final PASS gate %q requires logSha256", id)
		}
	}
	return validateSuccessfulVerificationEvidence(evidence)
}

func validateStructuralEvidence(evidence Evidence) error {
	if err := validatePassPhases(evidence.Phases, structuralPhaseIDs[:]); err != nil {
		return fmt.Errorf("structural NOT_FINAL: %w", err)
	}
	if len(evidence.Gates) != 0 {
		return fmt.Errorf("structural NOT_FINAL gates must be empty")
	}
	return validateSuccessfulVerificationEvidence(evidence)
}

func validatePassPhases(phases []PhaseResult, expected []string) error {
	if len(phases) != len(expected) {
		return fmt.Errorf("successful PASS phases count = %d, want %d", len(phases), len(expected))
	}
	for index, id := range expected {
		if phases[index].ID != id {
			return fmt.Errorf("successful PASS phases are not in the fixed order at index %d", index)
		}
		if phases[index].Result != ResultPass {
			return fmt.Errorf("successful PASS phase %q has result %q", id, phases[index].Result)
		}
	}
	return nil
}

func validateSuccessfulVerificationEvidence(evidence Evidence) error {
	if evidence.Reproducibility.ID != "reproducibility" || evidence.Reproducibility.Result != ResultPass {
		return fmt.Errorf("successful evidence reproducibility is not PASS")
	}
	if evidence.Reproducibility != evidence.Phases[3] {
		return fmt.Errorf("successful evidence reproducibility field contradicts phases")
	}
	if err := validateSuccessfulRender("render1", evidence.Render1); err != nil {
		return err
	}
	if err := validateSuccessfulRender("render2", evidence.Render2); err != nil {
		return err
	}
	if evidence.Render1.ManifestSHA256 != evidence.Render2.ManifestSHA256 || evidence.Render1.TreeSHA256 != evidence.Render2.TreeSHA256 {
		return fmt.Errorf("successful evidence render digests differ")
	}
	if evidence.Render1.Files != evidence.Render2.Files || evidence.Render1.Transform != evidence.Render2.Transform || evidence.Render1.Credentials != evidence.Render2.Credentials || evidence.Render1.Scan != evidence.Render2.Scan {
		return fmt.Errorf("successful evidence render reports contradict each other")
	}
	if evidence.Transform != evidence.Render1.Transform {
		return fmt.Errorf("successful evidence transform contradicts render1")
	}
	if evidence.Target.TotalDifferences != 0 || len(evidence.Target.Differences) != 0 || evidence.Target.DifferencesCutOff {
		return fmt.Errorf("successful evidence target contains differences")
	}
	if !isLowercaseHex(evidence.Target.BeforeTreeSHA256, 64) || evidence.Target.BeforeTreeSHA256 != evidence.Target.AfterTreeSHA256 || evidence.Target.BeforeTreeSHA256 != evidence.Render1.TreeSHA256 {
		return fmt.Errorf("successful evidence target tree hashes contradict render output")
	}
	if err := validateSuccessfulSourceEvidence(evidence.Source); err != nil {
		return err
	}
	return nil
}

func validateSuccessfulRender(name string, render RenderEvidence) error {
	if !isLowercaseHex(render.ManifestSHA256, 64) || !isLowercaseHex(render.TreeSHA256, 64) || render.Files <= 0 {
		return fmt.Errorf("successful %s digest/file evidence is incomplete", name)
	}
	if len(render.Commands) != len(renderCommandNames) {
		return fmt.Errorf("successful %s commands count = %d, want %d", name, len(render.Commands), len(renderCommandNames))
	}
	for index, commandName := range renderCommandNames {
		command := render.Commands[index]
		if command.Name != commandName || command.ExitCode != 0 {
			return fmt.Errorf("successful %s command %d is not %s/0", name, index, commandName)
		}
		if !isLowercaseHex(command.StdoutSHA256, 64) || !isLowercaseHex(command.StderrSHA256, 64) {
			return fmt.Errorf("successful %s command %q output hashes are incomplete", name, commandName)
		}
	}
	return nil
}

func validateSuccessfulSourceEvidence(source SourceEvidence) error {
	if source.Repository == "" || source.Ref == "" || !isLowercaseHex(source.Commit, 40) || !isLowercaseHex(source.Tree, 40) || !isLowercaseHex(source.ArchiveSHA256, 64) || source.SelectedFiles <= 0 || source.SelectedBlobBytes <= 0 {
		return fmt.Errorf("successful evidence source identity is incomplete")
	}
	if source.Before != source.After {
		return fmt.Errorf("successful evidence source before/after states differ")
	}
	state := source.Before
	if !isLowercaseHex(state.Head, 40) || state.Ref != source.Commit || !isLowercaseHex(state.StatusSHA256, 64) || !isLowercaseHex(state.IndexSHA256, 64) {
		return fmt.Errorf("successful evidence source repository state is incomplete")
	}
	return nil
}

func validateGateResult(gate GateResult) error {
	switch gate.Result {
	case ResultPass:
		if gate.Execution != ExecutionExited || gate.ExitCode == nil || *gate.ExitCode != 0 {
			return fmt.Errorf("PASS requires EXITED with exitCode 0")
		}
	case ResultFail:
		switch gate.Execution {
		case ExecutionExited:
			if gate.ExitCode == nil || *gate.ExitCode < 0 {
				return fmt.Errorf("failed EXITED gate requires a non-negative exitCode")
			}
			if *gate.ExitCode == 0 {
				if gate.ID != "things-build" {
					return fmt.Errorf("only things-build may fail after exitCode 0")
				}
				if gate.Summary == "" {
					return fmt.Errorf("failed EXITED things-build with exitCode 0 requires a postcondition summary")
				}
			}
		case ExecutionSpawnError, ExecutionTimeout, ExecutionCancelled:
			if gate.ExitCode != nil {
				return fmt.Errorf("%s gate exitCode must be null", gate.Execution)
			}
		default:
			return fmt.Errorf("FAIL has invalid execution %q", gate.Execution)
		}
	case ResultNotRun:
		if gate.Execution != ExecutionNotRun || gate.ExitCode != nil {
			return fmt.Errorf("NOT_RUN requires NOT_RUN execution and null exitCode")
		}
	default:
		return fmt.Errorf("invalid result %q", gate.Result)
	}
	if gate.Result != ResultPass && len(gate.Artifacts) != 0 {
		return fmt.Errorf("only PASS may contain artifacts")
	}
	if gate.ID != "things-build" {
		if len(gate.Artifacts) != 0 {
			return fmt.Errorf("only things-build may contain artifacts")
		}
		return nil
	}
	if gate.Result != ResultPass {
		return nil
	}
	expectedPaths := []string{"backend/bin/dgsvr", "backend/bin/dmsvr", "backend/bin/thingsvr"}
	if len(gate.Artifacts) != len(expectedPaths) {
		return fmt.Errorf("PASS things-build requires exactly three artifacts")
	}
	for index, artifact := range gate.Artifacts {
		if artifact.LogicalPath != expectedPaths[index] || filepath.IsAbs(artifact.LogicalPath) {
			return fmt.Errorf("artifact logicalPath is invalid or unsorted")
		}
		if artifact.Mode != "0755" || artifact.Size <= 0 {
			return fmt.Errorf("artifact mode/size is invalid")
		}
		if !isLowercaseHex(artifact.SHA256, 64) {
			return fmt.Errorf("artifact sha256 is invalid")
		}
		if artifact.GoVersion != lockedGoVersion {
			return fmt.Errorf("artifact goVersion is invalid")
		}
	}
	return nil
}

// WriteEvidenceAtomic writes only outside all forbidden roots and never
// follows a parent or target symlink. Existing regular evidence is atomically
// replaced; special files fail closed.
func WriteEvidenceAtomic(path string, evidence Evidence, forbiddenRoots []string) error {
	data, err := MarshalEvidence(evidence)
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve evidence path: %w", err)
	}
	absolute = filepath.Clean(absolute)
	for _, root := range forbiddenRoots {
		rootAbsolute, err := filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("resolve forbidden evidence root: %w", err)
		}
		rootAbsolute = filepath.Clean(rootAbsolute)
		if pathWithin(absolute, rootAbsolute) {
			return fmt.Errorf("evidence path must be outside source/backend boundaries")
		}
		if bytes.Contains(data, []byte(rootAbsolute)) {
			return fmt.Errorf("evidence contains a forbidden absolute path")
		}
	}
	parent := filepath.Dir(absolute)
	physicalParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return fmt.Errorf("resolve evidence parent: %w", err)
	}
	if filepath.Clean(physicalParent) != parent {
		return fmt.Errorf("evidence parent contains a symlink")
	}
	parentFD, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open evidence parent: %w", err)
	}
	defer unix.Close(parentFD)
	name := filepath.Base(absolute)
	if name == "." || name == ".." || strings.Contains(name, string(filepath.Separator)) {
		return fmt.Errorf("evidence filename is unsafe")
	}
	var existing unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &existing, unix.AT_SYMLINK_NOFOLLOW); err == nil {
		if existing.Mode&unix.S_IFMT == unix.S_IFLNK {
			return fmt.Errorf("evidence target is a symlink")
		}
		if existing.Mode&unix.S_IFMT != unix.S_IFREG || existing.Nlink != 1 {
			return fmt.Errorf("evidence target is not a single-link regular file")
		}
	} else if err != unix.ENOENT {
		return fmt.Errorf("inspect evidence target: %w", err)
	}
	temporaryName, temporaryFD, err := createEvidenceTemp(parentFD)
	if err != nil {
		return err
	}
	temporaryExists := true
	defer func() {
		if temporaryFD >= 0 {
			_ = unix.Close(temporaryFD)
		}
		if temporaryExists {
			_ = unix.Unlinkat(parentFD, temporaryName, 0)
		}
	}()
	remaining := data
	for len(remaining) > 0 {
		written, err := unix.Write(temporaryFD, remaining)
		if err != nil {
			return fmt.Errorf("write evidence temp: %w", err)
		}
		if written == 0 {
			return fmt.Errorf("write evidence temp made no progress")
		}
		remaining = remaining[written:]
	}
	if err := unix.Fchmod(temporaryFD, 0o644); err != nil {
		return fmt.Errorf("set evidence mode: %w", err)
	}
	if err := unix.Fsync(temporaryFD); err != nil {
		return fmt.Errorf("sync evidence temp: %w", err)
	}
	if err := unix.Close(temporaryFD); err != nil {
		return fmt.Errorf("close evidence temp: %w", err)
	}
	temporaryFD = -1
	if err := unix.Renameat(parentFD, temporaryName, parentFD, name); err != nil {
		return fmt.Errorf("atomically install evidence: %w", err)
	}
	temporaryExists = false
	if err := unix.Fsync(parentFD); err != nil {
		return fmt.Errorf("sync evidence parent: %w", err)
	}
	return nil
}

func pathWithin(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func createEvidenceTemp(parentFD int) (string, int, error) {
	for attempt := 0; attempt < 32; attempt++ {
		var random [12]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", -1, fmt.Errorf("generate evidence temp name: %w", err)
		}
		name := ".thingsbaseline-evidence-" + hex.EncodeToString(random[:])
		fd, err := unix.Openat(parentFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
		if err == nil {
			return name, fd, nil
		}
		if err != unix.EEXIST {
			return "", -1, fmt.Errorf("create evidence temp: %w", err)
		}
	}
	return "", -1, fmt.Errorf("create evidence temp: name collisions exhausted")
}
