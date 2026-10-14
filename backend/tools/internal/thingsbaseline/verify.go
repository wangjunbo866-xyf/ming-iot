package thingsbaseline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const maxReportedTreeDifferences = 64

const maxGateSummaryBytes = 4 << 10

const maxGateStreamSummaryBytes = 1900

// TreeDifference is intentionally content-free. Hashes and modes are enough
// to diagnose deterministic drift without copying source or secrets to logs.
type TreeDifference struct {
	Path           string `json:"path"`
	ExpectedMode   string `json:"expectedMode,omitempty"`
	ActualMode     string `json:"actualMode,omitempty"`
	ExpectedSHA256 string `json:"expectedSha256,omitempty"`
	ActualSHA256   string `json:"actualSha256,omitempty"`
}

// TargetComparison is one read-only, fd-relative target inventory.
type TargetComparison struct {
	TreeDigest       string                  `json:"treeDigest"`
	TotalDifferences int                     `json:"totalDifferences"`
	Truncated        bool                    `json:"truncated"`
	Differences      []TreeDifference        `json:"differences"`
	Scan             SecurityScanReport      `json:"scan"`
	Credentials      CredentialProfileReport `json:"credentials"`
	Manifest         []ManifestEntry         `json:"-"`
}

type GateSpecification struct {
	ID             string
	DisplayCommand string
	Path           string
	Args           []string
	Dir            string
	Env            []string
	RedactValues   []string
	goTool         gateGoTool
	outputContract *gateOutputContract
}

type gateOutputContract struct {
	directory      string
	logicalRoot    string
	names          []string
	mode           uint32
	afterFirstRead func(string)
}

type preparedGateOutputs struct {
	directoryFD   int
	directoryStat unix.Stat_t
}

type retainedGateArtifact struct {
	name     string
	file     *os.File
	state    unix.Stat_t
	sha256   string
	evidence GateArtifact
}

type VerifyRequest struct {
	Source         string
	Backend        string
	Lock           *Lock
	StructuralOnly bool
}

type moduleFileRecord struct {
	Exists bool
	Mode   string
	SHA256 string
}

type moduleInventory map[string]moduleFileRecord

type verifyDependencies struct {
	now                func() time.Time
	elapsed            func(time.Time) time.Duration
	readSource         func(context.Context, string, *Lock) (SourceSnapshot, error)
	captureSourceState func(context.Context, string, *Lock) (SourceRepositoryState, error)
	render             func(context.Context, string, SourceSnapshot, *Lock) (*Candidate, error)
	compareTarget      func(*Candidate) (TargetComparison, error)
	createIsolation    func(*Candidate) (*VerificationIsolation, error)
	captureModules     func(string) (moduleInventory, error)
	buildGates         func(VerifyRequest, *VerificationIsolation) ([]GateSpecification, func(), error)
	gateRunner         verificationGateRunner
	gateTimeout        time.Duration
}

type verificationGateRunner interface {
	Run(context.Context, GateSpecification, time.Duration) GateResult
}

type execVerificationGateRunner struct{}

func (execVerificationGateRunner) Run(ctx context.Context, specification GateSpecification, timeout time.Duration) (result GateResult) {
	started := time.Now()
	result = GateResult{ID: specification.ID, Command: specification.DisplayCommand, Result: ResultFail}
	if timeout <= 0 {
		timeout = time.Minute
	}
	gateContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := gateContext.Err(); err != nil {
		setGateContextFailure(&result, ctx, started, specification, "prepare build outputs: "+err.Error())
		return result
	}
	var preparedOutputs *preparedGateOutputs
	cleanupOutputs := false
	if specification.outputContract != nil {
		if specification.goTool == nil || specification.goTool.attestedVersion() != lockedGoVersion {
			result.Execution = ExecutionSpawnError
			result.LogSHA256 = combineCommandLogHashes(sha256Digest(nil), sha256Digest(nil))
			result.Summary = composeGateSummary(redactedRunnerSummarySection("prepare build outputs: retained Go tool attestation is unavailable", specification))
			result.DurationMS = time.Since(started).Milliseconds()
			return result
		}
		var err error
		preparedOutputs, err = prepareGateOutputContract(gateContext, specification.outputContract)
		if err != nil {
			if gateContext.Err() != nil {
				setGateContextFailure(&result, ctx, started, specification, "prepare build outputs: "+err.Error())
				return result
			}
			result.Execution = ExecutionSpawnError
			result.LogSHA256 = combineCommandLogHashes(sha256Digest(nil), sha256Digest(nil))
			result.Summary = composeGateSummary(redactedRunnerSummarySection("prepare build outputs: "+err.Error(), specification))
			result.DurationMS = time.Since(started).Milliseconds()
			return result
		}
		cleanupOutputs = true
		defer func() {
			if cleanupOutputs {
				if err := cleanupGateOutputContract(specification.outputContract, preparedOutputs); err != nil {
					result.Result = ResultFail
					result.Artifacts = nil
					result.Summary = composeGateSummary(result.Summary, redactedRunnerSummarySection("cleanup build outputs: "+err.Error(), specification))
				}
			}
			_ = unix.Close(preparedOutputs.directoryFD)
		}()
		if err := specification.goTool.revalidate(gateContext); err != nil {
			if gateContext.Err() != nil {
				setGateContextFailure(&result, ctx, started, specification, "prepare build outputs: revalidate retained Go tool: "+err.Error())
				return result
			}
			result.Execution = ExecutionSpawnError
			result.LogSHA256 = combineCommandLogHashes(sha256Digest(nil), sha256Digest(nil))
			result.Summary = composeGateSummary(redactedRunnerSummarySection("prepare build outputs: revalidate retained Go tool: "+err.Error(), specification))
			result.DurationMS = time.Since(started).Milliseconds()
			return result
		}
	}
	if err := gateContext.Err(); err != nil {
		setGateContextFailure(&result, ctx, started, specification, "start command: "+err.Error())
		return result
	}
	command := exec.Command(specification.Path, specification.Args...)
	command.Dir = specification.Dir
	command.Env = append([]string(nil), specification.Env...)
	if specification.goTool != nil && specification.goTool.inheritedFile() != nil {
		command.ExtraFiles = []*os.File{specification.goTool.inheritedFile()}
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout := newGateOutputBuffer(specification)
	stderr := newGateOutputBuffer(specification)
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		result.Execution = ExecutionSpawnError
		result.LogSHA256 = combineCommandLogHashes(stdout.SHA256(), stderr.SHA256())
		result.Summary = composeGateSummary(redactedRunnerSummarySection(err.Error(), specification))
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	var waitErr error
	select {
	case waitErr = <-waited:
	case <-gateContext.Done():
		_ = unix.Kill(-command.Process.Pid, unix.SIGKILL)
		waitErr = <-waited
		if errors.Is(ctx.Err(), context.Canceled) {
			result.Execution = ExecutionCancelled
		} else {
			result.Execution = ExecutionTimeout
		}
	}
	result.LogSHA256 = combineCommandLogHashes(stdout.SHA256(), stderr.SHA256())
	result.Summary = composeGateSummary(
		gateSummarySection("stdout", stdout.String()),
		gateSummarySection("stderr", stderr.String()),
	)
	result.DurationMS = time.Since(started).Milliseconds()
	if result.Execution == ExecutionTimeout || result.Execution == ExecutionCancelled {
		return result
	}
	if err := gateContext.Err(); err != nil {
		setGateContextFailure(&result, ctx, started, specification, "complete command: "+err.Error())
		return result
	}
	result.Execution = ExecutionExited
	exitCode := commandExitCode(waitErr)
	result.ExitCode = &exitCode
	if waitErr == nil && exitCode == 0 {
		if preparedOutputs != nil {
			artifacts, err := validateGateOutputContract(gateContext, specification.outputContract, preparedOutputs, specification.goTool.attestedVersion())
			if err != nil {
				if gateContext.Err() != nil {
					setGateContextFailure(&result, ctx, started, specification, "validate build outputs: "+err.Error())
					return result
				}
				result.Summary = composeGateSummary(result.Summary, redactedRunnerSummarySection("validate build outputs: "+err.Error(), specification))
				return result
			}
			if err := specification.goTool.revalidate(gateContext); err != nil {
				if gateContext.Err() != nil {
					setGateContextFailure(&result, ctx, started, specification, "validate build outputs: revalidate retained Go tool: "+err.Error())
					return result
				}
				result.Summary = composeGateSummary(result.Summary, redactedRunnerSummarySection("validate build outputs: revalidate retained Go tool: "+err.Error(), specification))
				return result
			}
			if err := gateContext.Err(); err != nil {
				setGateContextFailure(&result, ctx, started, specification, "validate build outputs: "+err.Error())
				return result
			}
			result.Artifacts = artifacts
		}
		if err := gateContext.Err(); err != nil {
			setGateContextFailure(&result, ctx, started, specification, "complete command: "+err.Error())
			return result
		}
		result.Result = ResultPass
		cleanupOutputs = false
	}
	return result
}

func setGateContextFailure(result *GateResult, parent context.Context, started time.Time, specification GateSpecification, diagnostic string) {
	result.Result = ResultFail
	result.ExitCode = nil
	result.Artifacts = nil
	if errors.Is(parent.Err(), context.Canceled) {
		result.Execution = ExecutionCancelled
	} else {
		result.Execution = ExecutionTimeout
	}
	if result.LogSHA256 == "" {
		result.LogSHA256 = combineCommandLogHashes(sha256Digest(nil), sha256Digest(nil))
	}
	result.Summary = composeGateSummary(result.Summary, redactedRunnerSummarySection(diagnostic, specification))
	result.DurationMS = time.Since(started).Milliseconds()
}

func prepareGateOutputContract(ctx context.Context, contract *gateOutputContract) (*preparedGateOutputs, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateGateOutputContractDefinition(contract); err != nil {
		return nil, err
	}
	directoryFD, directoryStat, err := openOrCreateAbsoluteDirectoryNoSymlinks(contract.directory)
	if err != nil {
		return nil, fmt.Errorf("output directory is not a real non-symlink directory: %w", err)
	}
	prepared := &preparedGateOutputs{directoryFD: directoryFD, directoryStat: directoryStat}
	for _, name := range contract.names {
		if err := ctx.Err(); err != nil {
			_ = unix.Close(directoryFD)
			return nil, err
		}
		if err := unix.Unlinkat(directoryFD, name, 0); err != nil && err != unix.ENOENT {
			_ = unix.Close(directoryFD)
			return nil, fmt.Errorf("remove prior contracted output %q: %w", name, err)
		}
	}
	for _, name := range contract.names {
		if err := ctx.Err(); err != nil {
			_ = unix.Close(directoryFD)
			return nil, err
		}
		var stat unix.Stat_t
		if err := unix.Fstatat(directoryFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != unix.ENOENT {
			_ = unix.Close(directoryFD)
			if err == nil {
				return nil, fmt.Errorf("prior contracted output %q remains", name)
			}
			return nil, fmt.Errorf("verify removal of contracted output %q: %w", name, err)
		}
	}
	if err := ctx.Err(); err != nil {
		_ = unix.Close(directoryFD)
		return nil, err
	}
	return prepared, nil
}

func cleanupGateOutputContract(contract *gateOutputContract, prepared *preparedGateOutputs) error {
	if err := validateGateOutputContractDefinition(contract); err != nil {
		return err
	}
	if prepared == nil || prepared.directoryFD < 0 {
		return fmt.Errorf("prepared output directory is unavailable")
	}
	var directoryStat unix.Stat_t
	if err := unix.Fstat(prepared.directoryFD, &directoryStat); err != nil ||
		!sameFileIdentity(prepared.directoryStat, directoryStat) || prepared.directoryStat.Mode != directoryStat.Mode {
		return fmt.Errorf("prepared output directory identity or mode changed")
	}
	var cleanupErrors []error
	for _, name := range contract.names {
		if err := unix.Unlinkat(prepared.directoryFD, name, 0); err != nil && err != unix.ENOENT {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove contracted output %q: %w", name, err))
		}
	}
	for _, name := range contract.names {
		var stat unix.Stat_t
		if err := unix.Fstatat(prepared.directoryFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != unix.ENOENT {
			if err == nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("contracted output %q remains", name))
			} else {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("verify contracted output %q removal: %w", name, err))
			}
		}
	}
	return errors.Join(cleanupErrors...)
}

func openOrCreateAbsoluteDirectoryNoSymlinks(path string) (int, unix.Stat_t, error) {
	var empty unix.Stat_t
	parentPath, name := filepath.Dir(path), filepath.Base(path)
	if name == "" || name == "." || name == ".." || strings.Contains(name, string(filepath.Separator)) {
		return -1, empty, fmt.Errorf("output directory name is unsafe")
	}
	parentFD, _, err := openAbsoluteDirectoryNoSymlinks(parentPath)
	if err != nil {
		return -1, empty, fmt.Errorf("open output directory parent: %w", err)
	}
	defer unix.Close(parentFD)
	var namedBefore unix.Stat_t
	err = unix.Fstatat(parentFD, name, &namedBefore, unix.AT_SYMLINK_NOFOLLOW)
	if err == unix.ENOENT {
		if err := unix.Mkdirat(parentFD, name, 0o755); err != nil {
			return -1, empty, fmt.Errorf("create output directory: %w", err)
		}
		if err := unix.Fstatat(parentFD, name, &namedBefore, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return -1, empty, fmt.Errorf("stat created output directory: %w", err)
		}
	} else if err != nil {
		return -1, empty, fmt.Errorf("inspect output directory: %w", err)
	}
	if namedBefore.Mode&unix.S_IFMT != unix.S_IFDIR {
		return -1, empty, fmt.Errorf("output path is not a directory")
	}
	directoryFD, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, empty, fmt.Errorf("open output directory: %w", err)
	}
	var opened, namedAfter unix.Stat_t
	if err := unix.Fstat(directoryFD, &opened); err != nil {
		_ = unix.Close(directoryFD)
		return -1, empty, fmt.Errorf("stat opened output directory: %w", err)
	}
	if err := unix.Fstatat(parentFD, name, &namedAfter, unix.AT_SYMLINK_NOFOLLOW); err != nil ||
		!sameFileIdentity(namedBefore, opened) || !sameFileIdentity(opened, namedAfter) ||
		namedBefore.Mode != opened.Mode || opened.Mode != namedAfter.Mode {
		_ = unix.Close(directoryFD)
		return -1, empty, fmt.Errorf("output directory identity or mode changed while opening")
	}
	pathFD, pathStat, err := openAbsoluteDirectoryNoSymlinks(path)
	if err != nil {
		_ = unix.Close(directoryFD)
		return -1, empty, fmt.Errorf("reopen output directory path: %w", err)
	}
	_ = unix.Close(pathFD)
	if !sameFileIdentity(opened, pathStat) || opened.Mode != pathStat.Mode {
		_ = unix.Close(directoryFD)
		return -1, empty, fmt.Errorf("output directory path changed while opening")
	}
	return directoryFD, opened, nil
}

func validateGateOutputContractDefinition(contract *gateOutputContract) error {
	if contract == nil {
		return fmt.Errorf("output contract is nil")
	}
	if !filepath.IsAbs(contract.directory) || filepath.Clean(contract.directory) != contract.directory || contract.directory == string(filepath.Separator) {
		return fmt.Errorf("output contract directory is not a clean absolute path")
	}
	if contract.logicalRoot != "backend/bin" || contract.mode != 0o755 {
		return fmt.Errorf("output contract metadata is invalid")
	}
	wantNames := []string{"thingsvr", "dgsvr", "dmsvr"}
	if !equalStringSlices(contract.names, wantNames) {
		return fmt.Errorf("output contract names are invalid")
	}
	return nil
}

func openAbsoluteDirectoryNoSymlinks(path string) (int, unix.Stat_t, error) {
	var empty unix.Stat_t
	rootFD, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, empty, err
	}
	defer unix.Close(rootFD)
	parts := strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator))
	directoryFD, err := openExistingDirectoryAt(rootFD, parts)
	if err != nil {
		return -1, empty, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(directoryFD, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		_ = unix.Close(directoryFD)
		if err != nil {
			return -1, empty, err
		}
		return -1, empty, fmt.Errorf("output path is not a directory")
	}
	return directoryFD, stat, nil
}

func validateGateOutputContract(ctx context.Context, contract *gateOutputContract, prepared *preparedGateOutputs, goVersion string) ([]GateArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateGateOutputContractDefinition(contract); err != nil {
		return nil, err
	}
	if prepared == nil || prepared.directoryFD < 0 {
		return nil, fmt.Errorf("prepared output directory is unavailable")
	}
	directoryFD, directoryStat, err := openAbsoluteDirectoryNoSymlinks(contract.directory)
	if err != nil {
		return nil, fmt.Errorf("reopen output directory: %w", err)
	}
	defer unix.Close(directoryFD)
	if !sameFileIdentity(prepared.directoryStat, directoryStat) || prepared.directoryStat.Mode != directoryStat.Mode {
		return nil, fmt.Errorf("output directory identity or mode changed")
	}
	namesBefore, err := listDirectoryNames(directoryFD)
	if err != nil {
		return nil, fmt.Errorf("list build outputs: %w", err)
	}
	expectedNames := append([]string(nil), contract.names...)
	sort.Strings(expectedNames)
	if !equalStringSlices(namesBefore, expectedNames) {
		return nil, fmt.Errorf("build output names differ from exact contract")
	}
	retained := make([]retainedGateArtifact, 0, len(expectedNames))
	defer func() {
		for index := range retained {
			_ = retained[index].file.Close()
		}
	}()
	for _, name := range expectedNames {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var before unix.Stat_t
		if err := unix.Fstatat(directoryFD, name, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return nil, fmt.Errorf("stat contracted output %q: %w", name, err)
		}
		artifact, err := openStableGateArtifact(ctx, directoryFD, name, before, contract, goVersion)
		if err != nil {
			return nil, fmt.Errorf("validate contracted output %q: %w", name, err)
		}
		retained = append(retained, artifact)
	}
	namesAfter, err := listDirectoryNames(directoryFD)
	if err != nil || !equalStringSlices(namesBefore, namesAfter) {
		return nil, fmt.Errorf("build output names changed during scan")
	}
	artifacts := make([]GateArtifact, 0, len(retained))
	for index := range retained {
		if err := revalidateRetainedGateArtifact(ctx, directoryFD, &retained[index], contract); err != nil {
			return nil, fmt.Errorf("revalidate contracted output %q: %w", retained[index].name, err)
		}
		artifacts = append(artifacts, retained[index].evidence)
	}
	namesFinal, err := listDirectoryNames(directoryFD)
	if err != nil || !equalStringSlices(namesBefore, namesFinal) {
		return nil, fmt.Errorf("build output names changed during final validation")
	}
	var directoryAfter unix.Stat_t
	if err := unix.Fstat(directoryFD, &directoryAfter); err != nil || !sameFileIdentity(directoryStat, directoryAfter) || directoryStat.Mode != directoryAfter.Mode {
		return nil, fmt.Errorf("output directory identity or mode changed during scan")
	}
	pathFD, pathStat, err := openAbsoluteDirectoryNoSymlinks(contract.directory)
	if err != nil {
		return nil, fmt.Errorf("reopen output directory after scan: %w", err)
	}
	_ = unix.Close(pathFD)
	if !sameFileIdentity(directoryAfter, pathStat) || directoryAfter.Mode != pathStat.Mode {
		return nil, fmt.Errorf("output directory path changed during scan")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return artifacts, nil
}

const maxGateArtifactBytes = int64(1 << 30)

func openStableGateArtifact(ctx context.Context, directoryFD int, name string, before unix.Stat_t, contract *gateOutputContract, goVersion string) (retainedGateArtifact, error) {
	if err := ctx.Err(); err != nil {
		return retainedGateArtifact{}, err
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 {
		return retainedGateArtifact{}, fmt.Errorf("not a single-link regular file")
	}
	if before.Mode&0o7777 != contract.mode {
		return retainedGateArtifact{}, fmt.Errorf("mode = %04o, want %04o", before.Mode&0o7777, contract.mode)
	}
	if before.Size <= 0 || before.Size > maxGateArtifactBytes {
		return retainedGateArtifact{}, fmt.Errorf("size is outside the allowed nonempty range")
	}
	fd, err := unix.Openat(directoryFD, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return retainedGateArtifact{}, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return retainedGateArtifact{}, fmt.Errorf("wrap artifact file descriptor")
	}
	keepOpen := false
	defer func() {
		if !keepOpen {
			_ = file.Close()
		}
	}()
	var opened unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil || !sameGateArtifactState(before, opened) {
		return retainedGateArtifact{}, fmt.Errorf("metadata changed before read")
	}
	firstSHA, firstSize, err := hashGateArtifact(ctx, file)
	if err != nil || firstSize != before.Size {
		return retainedGateArtifact{}, fmt.Errorf("content or size changed during first read")
	}
	if contract.afterFirstRead != nil {
		contract.afterFirstRead(name)
	}
	if err := ctx.Err(); err != nil {
		return retainedGateArtifact{}, err
	}
	var middle unix.Stat_t
	if err := unix.Fstat(fd, &middle); err != nil || !sameGateArtifactState(opened, middle) {
		return retainedGateArtifact{}, fmt.Errorf("metadata changed during first read")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return retainedGateArtifact{}, fmt.Errorf("rewind artifact: %w", err)
	}
	secondSHA, secondSize, err := hashGateArtifact(ctx, file)
	if err != nil || secondSize != before.Size || secondSHA != firstSHA {
		return retainedGateArtifact{}, fmt.Errorf("content changed across reads")
	}
	var afterFD unix.Stat_t
	if err := unix.Fstat(fd, &afterFD); err != nil || !sameGateArtifactState(middle, afterFD) {
		return retainedGateArtifact{}, fmt.Errorf("metadata changed during second read")
	}
	var afterPath unix.Stat_t
	if err := unix.Fstatat(directoryFD, name, &afterPath, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameGateArtifactState(afterFD, afterPath) {
		return retainedGateArtifact{}, fmt.Errorf("path identity or metadata changed during read")
	}
	keepOpen = true
	return retainedGateArtifact{
		name: name, file: file, state: afterFD, sha256: firstSHA,
		evidence: GateArtifact{
			LogicalPath: contract.logicalRoot + "/" + name,
			Mode:        fmt.Sprintf("%04o", before.Mode&0o7777),
			Size:        before.Size,
			SHA256:      firstSHA,
			GoVersion:   goVersion,
		},
	}, nil
}

func revalidateRetainedGateArtifact(ctx context.Context, directoryFD int, artifact *retainedGateArtifact, contract *gateOutputContract) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if artifact == nil || artifact.file == nil {
		return fmt.Errorf("retained artifact is unavailable")
	}
	fd := int(artifact.file.Fd())
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil || !sameGateArtifactState(artifact.state, before) {
		return fmt.Errorf("retained file identity or metadata changed")
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || before.Mode&0o7777 != contract.mode || before.Size <= 0 {
		return fmt.Errorf("retained file contract changed")
	}
	if _, err := artifact.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind retained artifact: %w", err)
	}
	sha, size, err := hashGateArtifact(ctx, artifact.file)
	if err != nil || size != artifact.state.Size || sha != artifact.sha256 {
		return fmt.Errorf("retained file content changed")
	}
	var afterFD unix.Stat_t
	if err := unix.Fstat(fd, &afterFD); err != nil || !sameGateArtifactState(artifact.state, afterFD) {
		return fmt.Errorf("retained file changed during final read")
	}
	var afterPath unix.Stat_t
	if err := unix.Fstatat(directoryFD, artifact.name, &afterPath, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameGateArtifactState(artifact.state, afterPath) {
		return fmt.Errorf("retained path identity or metadata changed")
	}
	return nil
}

func hashGateArtifact(ctx context.Context, file *os.File) (string, int64, error) {
	hasher := sha256.New()
	buffer := make([]byte, 128*1024)
	var offset int64
	for {
		if err := ctx.Err(); err != nil {
			return "", offset, err
		}
		read, err := file.ReadAt(buffer, offset)
		if read > 0 {
			offset += int64(read)
			if offset > maxGateArtifactBytes {
				return "", offset, fmt.Errorf("artifact exceeds maximum size")
			}
			_, _ = hasher.Write(buffer[:read])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", offset, err
		}
		if read == 0 {
			return "", offset, fmt.Errorf("artifact read made no progress")
		}
	}
	if err := ctx.Err(); err != nil {
		return "", offset, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), offset, nil
}

func sameGateArtifactState(left, right unix.Stat_t) bool {
	return sameFileIdentity(left, right) &&
		left.Mode == right.Mode && left.Size == right.Size && left.Nlink == right.Nlink &&
		left.Mtim.Sec == right.Mtim.Sec && left.Mtim.Nsec == right.Mtim.Nsec &&
		left.Ctim.Sec == right.Ctim.Sec && left.Ctim.Nsec == right.Ctim.Nsec
}

func combineCommandLogHashes(stdout, stderr string) string {
	hasher := sha256.New()
	writeFramedString(hasher, stdout)
	writeFramedString(hasher, stderr)
	return hex.EncodeToString(hasher.Sum(nil))
}

func composeGateSummary(sections ...string) string {
	nonEmpty := make([]string, 0, len(sections))
	for _, section := range sections {
		if section != "" {
			nonEmpty = append(nonEmpty, section)
		}
	}
	return validUTF8Tail(strings.Join(nonEmpty, "\n\n"), maxGateSummaryBytes)
}

func gateSummarySection(label, value string) string {
	tail := validUTF8Tail(strings.TrimSpace(strings.ToValidUTF8(value, "\uFFFD")), maxGateStreamSummaryBytes)
	if tail == "" {
		return ""
	}
	return "[" + label + "]\n" + tail
}

func redactedRunnerSummarySection(value string, specification GateSpecification) string {
	redacted := redactCommandTail(value, GoCommand{Dir: specification.Dir, Env: specification.Env})
	for _, sensitive := range specification.RedactValues {
		if sensitive != "" {
			redacted = strings.ReplaceAll(redacted, sensitive, "[REDACTED]")
		}
	}
	for _, argument := range specification.Args {
		_, possiblePath, hasEquals := strings.Cut(argument, "=")
		if hasEquals && filepath.IsAbs(possiblePath) {
			redacted = strings.ReplaceAll(redacted, possiblePath, "[REDACTED]")
		}
	}
	if filepath.IsAbs(specification.Path) {
		redacted = strings.ReplaceAll(redacted, specification.Path, "[REDACTED]")
	}
	if specification.outputContract != nil && specification.outputContract.directory != "" {
		redacted = strings.ReplaceAll(redacted, specification.outputContract.directory, "[REDACTED]")
	}
	return gateSummarySection("runner", redacted)
}

func validUTF8Tail(value string, limit int) string {
	value = strings.ToValidUTF8(value, "\uFFFD")
	if len(value) <= limit {
		return value
	}
	start := len(value) - limit
	for start < len(value) && !utf8.RuneStart(value[start]) {
		start++
	}
	return value[start:]
}

// digestDiagnosticCapture hashes every raw byte while independently retaining
// a bounded, streaming-redacted diagnostic tail. Render commands and final
// gates share this implementation so literal and URL decisions use the same
// original raw byte coordinates.
type digestDiagnosticCapture struct {
	hasher     hash.Hash
	diagnostic *unifiedDiagnosticRedactor
	tail       *boundedDiagnosticTail
}

func newDigestDiagnosticCapture(limit int, literals []string) *digestDiagnosticCapture {
	buffer := &digestDiagnosticCapture{hasher: sha256.New(), tail: newBoundedDiagnosticTail(limit)}
	buffer.diagnostic = newUnifiedDiagnosticRedactor(literals, buffer.tail.Write)
	return buffer
}

func (buffer *digestDiagnosticCapture) Write(data []byte) (int, error) {
	_, _ = buffer.hasher.Write(data)
	buffer.diagnostic.Write(data)
	return len(data), nil
}

func (buffer *digestDiagnosticCapture) SHA256() string {
	return hex.EncodeToString(buffer.hasher.Sum(nil))
}

func (buffer *digestDiagnosticCapture) String() string {
	buffer.diagnostic.Finalize()
	return buffer.tail.String()
}

type gateOutputBuffer struct{ *digestDiagnosticCapture }

func newGateOutputBuffer(specification GateSpecification) *gateOutputBuffer {
	return &gateOutputBuffer{newDigestDiagnosticCapture(maxGateSummaryBytes, gateRedactionLiterals(specification))}
}

func gateRedactionLiterals(specification GateSpecification) []string {
	values := append([]string{specification.Dir}, commandEnvironmentValues(specification.Env)...)
	values = append(values, specification.RedactValues...)
	for _, argument := range specification.Args {
		_, possiblePath, hasEquals := strings.Cut(argument, "=")
		if hasEquals && filepath.IsAbs(possiblePath) {
			values = append(values, possiblePath)
		}
	}
	if filepath.IsAbs(specification.Path) {
		values = append(values, specification.Path)
	}
	if specification.outputContract != nil {
		values = append(values, specification.outputContract.directory)
	}

	return values
}

type boundedDiagnosticRedactor struct {
	*unifiedDiagnosticRedactor
	tail *boundedDiagnosticTail
}

func newBoundedDiagnosticRedactor(limit int, values []string) *boundedDiagnosticRedactor {
	tail := newBoundedDiagnosticTail(limit)
	return &boundedDiagnosticRedactor{
		unifiedDiagnosticRedactor: newUnifiedDiagnosticRedactor(values, tail.Write),
		tail:                      tail,
	}
}

func (redactor *boundedDiagnosticRedactor) String() string {
	redactor.Finalize()
	return redactor.tail.String()
}

type unifiedDiagnosticRedactor struct {
	destination    func([]byte)
	automaton      literalAutomaton
	literalState   int
	urlCandidate   []byte
	inURLAuthority bool
	delay          int
	pending        []pendingDiagnosticByte
	ranges         []diagnosticRedactionRange
	nextPosition   int64
	inRedaction    bool
	finalized      bool
}

func newUnifiedDiagnosticRedactor(values []string, destination func([]byte)) *unifiedDiagnosticRedactor {
	automaton := newLiteralAutomaton(values)
	delay := automaton.maxLiteralLength
	if delay < len("https://") {
		delay = len("https://")
	}
	return &unifiedDiagnosticRedactor{
		destination:  destination,
		automaton:    automaton,
		delay:        delay,
		urlCandidate: make([]byte, 0, len("https://")),
	}
}

func (redactor *unifiedDiagnosticRedactor) Write(data []byte) {
	for _, value := range data {
		if len(redactor.pending) == redactor.delay {
			redactor.flushFirstPending()
		}
		position := redactor.nextPosition
		redactor.nextPosition++
		redactor.pending = append(redactor.pending, pendingDiagnosticByte{value: value, position: position})
		var matchLength int
		redactor.literalState, matchLength = redactor.automaton.advance(redactor.literalState, value)
		if matchLength > 0 {
			redactor.addRedactionRange(position+1-int64(matchLength), position+1)
		}
		redactor.observeURLByte(value, position)
	}
}

func (redactor *unifiedDiagnosticRedactor) Finalize() {
	if redactor.finalized {
		return
	}
	partialPrefixLength := redactor.automaton.stateDepth(redactor.literalState)
	if partialPrefixLength > len(redactor.pending) {
		partialPrefixLength = len(redactor.pending)
	}
	if partialPrefixLength > 0 {
		redactor.addRedactionRange(redactor.nextPosition-int64(partialPrefixLength), redactor.nextPosition)
	}
	for len(redactor.pending) > 0 {
		redactor.flushFirstPending()
	}
	redactor.finalized = true
}

func (redactor *unifiedDiagnosticRedactor) observeURLByte(value byte, position int64) {
	if redactor.inURLAuthority {
		if isURLAuthorityDelimiter(value) {
			redactor.inURLAuthority = false
			return
		}
		redactor.addRedactionRange(position, position+1)
		return
	}

	redactor.urlCandidate = append(redactor.urlCandidate, value)
	for len(redactor.urlCandidate) > 0 {
		isPrefix, complete := classifyURLSchemeCandidate(redactor.urlCandidate)
		if complete {
			redactor.urlCandidate = redactor.urlCandidate[:0]
			redactor.inURLAuthority = true
			return
		}
		if isPrefix {
			return
		}
		redactor.urlCandidate = redactor.urlCandidate[1:]
	}
}

func (redactor *unifiedDiagnosticRedactor) flushFirstPending() {
	item := redactor.pending[0]
	redactor.pending = redactor.pending[1:]
	for len(redactor.ranges) > 0 && redactor.ranges[0].end <= item.position {
		redactor.ranges = redactor.ranges[1:]
	}
	redacted := len(redactor.ranges) > 0 && redactor.ranges[0].start <= item.position && item.position < redactor.ranges[0].end
	if redacted {
		if !redactor.inRedaction {
			redactor.destination([]byte("[REDACTED]"))
		}
		redactor.inRedaction = true
		return
	}
	redactor.inRedaction = false
	redactor.destination([]byte{item.value})
}

func (redactor *unifiedDiagnosticRedactor) addRedactionRange(start, end int64) {
	for len(redactor.ranges) > 0 {
		last := redactor.ranges[len(redactor.ranges)-1]
		if start > last.end {
			break
		}
		if last.start < start {
			start = last.start
		}
		if last.end > end {
			end = last.end
		}
		redactor.ranges = redactor.ranges[:len(redactor.ranges)-1]
	}
	redactor.ranges = append(redactor.ranges, diagnosticRedactionRange{start: start, end: end})
}

type boundedDiagnosticTail struct {
	limit int
	tail  []byte
}

func newBoundedDiagnosticTail(limit int) *boundedDiagnosticTail {
	return &boundedDiagnosticTail{limit: limit}
}

func (tail *boundedDiagnosticTail) Write(data []byte) {
	if tail.limit <= 0 {
		return
	}
	if len(data) >= tail.limit {
		tail.tail = append(tail.tail[:0], data[len(data)-tail.limit:]...)
		return
	}
	tail.tail = append(tail.tail, data...)
	if overflow := len(tail.tail) - tail.limit; overflow > 0 {
		tail.tail = append(tail.tail[:0], tail.tail[overflow:]...)
	}
}

func (tail *boundedDiagnosticTail) String() string {
	return strings.TrimSpace(validUTF8Tail(string(tail.tail), tail.limit))
}

type pendingDiagnosticByte struct {
	value    byte
	position int64
}

type diagnosticRedactionRange struct {
	start int64
	end   int64
}

type literalAutomaton struct {
	nodes            []literalAutomatonNode
	maxLiteralLength int
}

type literalAutomatonNode struct {
	transitions map[byte]int
	failure     int
	matchLength int
	depth       int
}

func newLiteralAutomaton(values []string) literalAutomaton {
	literals := normalizeRedactionLiterals(values)
	automaton := literalAutomaton{nodes: []literalAutomatonNode{{transitions: make(map[byte]int)}}}
	for _, literal := range literals {
		state := 0
		for _, value := range []byte(literal) {
			next, exists := automaton.nodes[state].transitions[value]
			if !exists {
				next = len(automaton.nodes)
				automaton.nodes = append(automaton.nodes, literalAutomatonNode{
					transitions: make(map[byte]int),
					depth:       automaton.nodes[state].depth + 1,
				})
				automaton.nodes[state].transitions[value] = next
			}
			state = next
		}
		automaton.nodes[state].matchLength = len(literal)
		if len(literal) > automaton.maxLiteralLength {
			automaton.maxLiteralLength = len(literal)
		}
	}
	automaton.buildFailureLinks()
	return automaton
}

func normalizeRedactionLiterals(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	literals := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := unique[value]; exists {
			continue
		}
		unique[value] = struct{}{}
		literals = append(literals, value)
	}
	sort.Slice(literals, func(left, right int) bool {
		if len(literals[left]) != len(literals[right]) {
			return len(literals[left]) > len(literals[right])
		}
		return literals[left] < literals[right]
	})
	return literals
}

func (automaton *literalAutomaton) buildFailureLinks() {
	queue := make([]int, 0, len(automaton.nodes))
	for _, child := range automaton.nodes[0].transitions {
		queue = append(queue, child)
	}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		for value, child := range automaton.nodes[state].transitions {
			failure := automaton.nodes[state].failure
			for failure != 0 {
				if next, exists := automaton.nodes[failure].transitions[value]; exists {
					failure = next
					break
				}
				failure = automaton.nodes[failure].failure
			}
			if failure == 0 {
				if next, exists := automaton.nodes[0].transitions[value]; exists {
					failure = next
				}
			}
			automaton.nodes[child].failure = failure
			if inherited := automaton.nodes[failure].matchLength; inherited > automaton.nodes[child].matchLength {
				automaton.nodes[child].matchLength = inherited
			}
			queue = append(queue, child)
		}
	}
}

func (automaton literalAutomaton) advance(state int, value byte) (int, int) {
	for state != 0 {
		if next, exists := automaton.nodes[state].transitions[value]; exists {
			return next, automaton.nodes[next].matchLength
		}
		state = automaton.nodes[state].failure
	}
	if next, exists := automaton.nodes[0].transitions[value]; exists {
		return next, automaton.nodes[next].matchLength
	}
	return 0, 0
}

func (automaton literalAutomaton) stateDepth(state int) int {
	if state < 0 || state >= len(automaton.nodes) {
		return 0
	}
	return automaton.nodes[state].depth
}

var gateURLSchemes = [...][]byte{[]byte("http://"), []byte("https://")}

func classifyURLSchemeCandidate(candidate []byte) (bool, bool) {
	for _, scheme := range gateURLSchemes {
		if len(candidate) > len(scheme) || !equalFoldASCII(candidate, scheme[:len(candidate)]) {
			continue
		}
		return true, len(candidate) == len(scheme)
	}
	return false, false
}

func equalFoldASCII(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		leftValue, rightValue := left[index], right[index]
		if leftValue >= 'A' && leftValue <= 'Z' {
			leftValue += 'a' - 'A'
		}
		if rightValue >= 'A' && rightValue <= 'Z' {
			rightValue += 'a' - 'A'
		}
		if leftValue != rightValue {
			return false
		}
	}
	return true
}

func isURLAuthorityDelimiter(value byte) bool {
	switch value {
	case '/', '?', '#', ' ', '\t', '\r', '\n', '\f', '\v':
		return true
	default:
		return false
	}
}

type VerificationIsolation struct {
	mu          sync.Mutex
	backendPath string
	backendFD   int
	name        string
	path        string
	rootFD      int
	cleaned     bool
	policy      securityPolicy
	treeDigest  string
}

// Verify performs a two-render, read-only-target verification. Final command
// gates run against an isolation copy, never backend/things.
func Verify(ctx context.Context, request VerifyRequest) (Evidence, error) {
	return verifyWithDependencies(ctx, request, verifyDependencies{
		now:                time.Now,
		elapsed:            time.Since,
		readSource:         ReadSource,
		captureSourceState: CaptureSourceRepositoryState,
		render:             renderVerificationCandidate,
		compareTarget:      compareCandidateTarget,
		createIsolation:    createVerificationIsolation,
		captureModules:     captureModuleInventory,
		buildGates:         buildDefaultVerificationGates,
		gateRunner:         execVerificationGateRunner{},
		gateTimeout:        20 * time.Minute,
	})
}

func verifyWithDependencies(ctx context.Context, request VerifyRequest, dependencies verifyDependencies) (evidence Evidence, returnErr error) {
	if dependencies.now == nil {
		dependencies.now = time.Now
	}
	if dependencies.elapsed == nil {
		dependencies.elapsed = func(started time.Time) time.Duration {
			return dependencies.now().Sub(started)
		}
	}
	started := dependencies.now()
	evidence = Evidence{
		SchemaVersion: EvidenceSchemaVersion,
		LoopID:        "L0",
		FinalEvidence: !request.StructuralOnly,
		StartedAt:     started,
		Target: TargetEvidence{
			LogicalPath: "backend/things",
		},
	}
	defer func() {
		elapsed := dependencies.elapsed(started)
		if elapsed < 0 {
			elapsed = 0
		}
		// Derive the serialized finish timestamp from the monotonic elapsed
		// duration. A wall-clock correction during verification must not make
		// startedAt/finishedAt disagree with durationMs.
		evidence.FinishedAt = started.Add(elapsed)
		evidence.DurationMS = elapsed.Milliseconds()
	}()
	if request.Lock == nil {
		evidence.OverallResult = ResultFail
		return evidence, fmt.Errorf("verify lock is nil")
	}
	evidence.Target.Module = request.Lock.Target.Module
	evidence.Contracts = request.Lock.Contracts
	if err := ValidateLock(request.Lock); err != nil {
		evidence.OverallResult = ResultFail
		evidence.Phases = append(evidence.Phases, PhaseResult{ID: "lock", Result: ResultFail, FailureKind: "CONTRACT", Summary: err.Error()})
		return evidence, err
	}
	if dependencies.readSource == nil || dependencies.captureSourceState == nil || dependencies.render == nil || dependencies.compareTarget == nil {
		evidence.OverallResult = ResultFail
		return evidence, fmt.Errorf("verify dependencies are incomplete")
	}

	snapshot, err := dependencies.readSource(ctx, request.Source, request.Lock)
	if err != nil {
		evidence.OverallResult = ResultFail
		evidence.Phases = append(evidence.Phases, failedPhase("source-read", "SOURCE", err))
		return evidence, err
	}
	evidence.Source = SourceEvidence{
		Repository:        request.Lock.Source.Repository,
		Ref:               request.Lock.Source.Ref,
		Commit:            request.Lock.Source.Commit,
		Tree:              request.Lock.Source.Tree,
		Before:            snapshot.RepositoryState,
		SelectedFiles:     request.Lock.Source.SelectedFiles,
		SelectedBlobBytes: request.Lock.Source.SelectedBlobBytes,
		ArchiveSHA256:     request.Lock.Source.ArchiveSHA256,
	}
	evidence.Phases = append(evidence.Phases, PhaseResult{ID: "source-read", Result: ResultPass})

	first, err := dependencies.render(ctx, request.Backend, snapshot, request.Lock)
	if err != nil {
		evidence.OverallResult = ResultFail
		evidence.Phases = append(evidence.Phases, failedPhase("render-1", "RENDER", err))
		return evidence, err
	}
	defer first.Cleanup()
	evidence.Phases = append(evidence.Phases, PhaseResult{ID: "render-1", Result: ResultPass})
	second, err := dependencies.render(ctx, request.Backend, snapshot, request.Lock)
	if err != nil {
		evidence.OverallResult = ResultFail
		evidence.Phases = append(evidence.Phases, failedPhase("render-2", "RENDER", err))
		return evidence, err
	}
	defer second.Cleanup()
	evidence.Phases = append(evidence.Phases, PhaseResult{ID: "render-2", Result: ResultPass})
	evidence.Render1, evidence.Render2, err = verifyRenderReproducibility(first, second)
	if err != nil {
		evidence.Reproducibility = failedPhase("reproducibility", "NON_DETERMINISTIC", err)
		evidence.Phases = append(evidence.Phases, evidence.Reproducibility)
		evidence.OverallResult = ResultFail
		return evidence, err
	}
	evidence.Reproducibility = PhaseResult{ID: "reproducibility", Result: ResultPass}
	evidence.Phases = append(evidence.Phases, evidence.Reproducibility)
	evidence.Transform = evidence.Render1.Transform

	targetBefore, err := dependencies.compareTarget(first)
	if err != nil {
		evidence.OverallResult = ResultFail
		evidence.Phases = append(evidence.Phases, failedPhase("target-before", "TARGET_SCAN", err))
		return evidence, err
	}
	setTargetComparisonEvidence(&evidence.Target, targetBefore, true)
	if targetBefore.TotalDifferences != 0 {
		err = fmt.Errorf("verification target has %d differences", targetBefore.TotalDifferences)
		evidence.OverallResult = ResultFail
		evidence.Phases = append(evidence.Phases, failedPhase("target-compare", "TREE_DIFFERENCE", err))
		return evidence, err
	}
	evidence.Phases = append(evidence.Phases, PhaseResult{ID: "target-compare", Result: ResultPass})

	if request.StructuralOnly {
		targetAfter, targetErr := dependencies.compareTarget(first)
		if targetErr != nil || targetAfter.TreeDigest != targetBefore.TreeDigest || !reflect.DeepEqual(targetAfter.Manifest, targetBefore.Manifest) {
			if targetErr == nil {
				targetErr = fmt.Errorf("verification target changed during structural verification")
			}
			evidence.OverallResult = ResultFail
			evidence.Phases = append(evidence.Phases, failedPhase("target-unchanged", "TARGET_DRIFT", targetErr))
			return evidence, targetErr
		}
		setTargetComparisonEvidence(&evidence.Target, targetAfter, false)
		evidence.Phases = append(evidence.Phases, PhaseResult{ID: "target-unchanged", Result: ResultPass})
		finalState, stateErr := dependencies.captureSourceState(ctx, request.Source, request.Lock)
		evidence.Source.After = finalState
		if stateErr != nil || finalState != snapshot.RepositoryState {
			if stateErr == nil {
				stateErr = fmt.Errorf("source repository state changed during structural verification")
			}
			evidence.OverallResult = ResultFail
			evidence.Phases = append(evidence.Phases, failedPhase("source-unchanged", "SOURCE_DRIFT", stateErr))
			return evidence, stateErr
		}
		evidence.Phases = append(evidence.Phases, PhaseResult{ID: "source-unchanged", Result: ResultPass})
		evidence.OverallResult = ResultNotFinal
		return evidence, nil
	}

	if dependencies.createIsolation == nil || dependencies.captureModules == nil || dependencies.buildGates == nil || dependencies.gateRunner == nil {
		evidence.OverallResult = ResultFail
		return evidence, fmt.Errorf("final verify dependencies are incomplete")
	}
	isolation, err := dependencies.createIsolation(second)
	if err != nil {
		evidence.OverallResult = ResultFail
		evidence.Phases = append(evidence.Phases, failedPhase("isolation-copy", "ISOLATION", err))
		return evidence, err
	}
	defer isolation.Cleanup()
	evidence.Phases = append(evidence.Phases, PhaseResult{ID: "isolation-copy", Result: ResultPass})
	modulesBefore, err := dependencies.captureModules(request.Backend)
	if err != nil {
		evidence.OverallResult = ResultFail
		evidence.Phases = append(evidence.Phases, failedPhase("module-before", "MODULE_INVENTORY", err))
		return evidence, err
	}
	specifications, cleanupGates, err := dependencies.buildGates(request, isolation)
	if err != nil {
		evidence.OverallResult = ResultFail
		evidence.Phases = append(evidence.Phases, failedPhase("gate-prepare", "GATE_PREPARE", err))
		return evidence, err
	}
	defer cleanupGates()
	evidence.Gates = runGateSequence(ctx, dependencies.gateRunner, specifications, dependencies.gateTimeout)
	gateFailed := false
	for _, gate := range evidence.Gates {
		if gate.Result != ResultPass {
			gateFailed = true
			break
		}
	}
	if gateFailed {
		evidence.Phases = append(evidence.Phases, PhaseResult{ID: "command-gates", Result: ResultFail, FailureKind: "GATE"})
	} else {
		evidence.Phases = append(evidence.Phases, PhaseResult{ID: "command-gates", Result: ResultPass})
	}
	isolationErr := isolation.VerifyUnchanged()
	if isolationErr != nil {
		evidence.Phases = append(evidence.Phases, failedPhase("isolation-unchanged", "ISOLATION_DRIFT", isolationErr))
	} else {
		evidence.Phases = append(evidence.Phases, PhaseResult{ID: "isolation-unchanged", Result: ResultPass})
	}

	modulesAfter, moduleErr := dependencies.captureModules(request.Backend)
	moduleChanged := moduleErr != nil || !reflect.DeepEqual(modulesBefore, modulesAfter)
	if moduleChanged {
		if moduleErr == nil {
			moduleErr = fmt.Errorf("module/workspace inventory changed during verification")
		}
		evidence.Phases = append(evidence.Phases, failedPhase("module-clean", "MODULE_DRIFT", moduleErr))
	} else {
		evidence.Phases = append(evidence.Phases, PhaseResult{ID: "module-clean", Result: ResultPass})
	}
	targetAfter, targetErr := dependencies.compareTarget(first)
	targetChanged := targetErr != nil
	if !targetChanged {
		setTargetComparisonEvidence(&evidence.Target, targetAfter, false)
		targetChanged = targetAfter.TreeDigest != targetBefore.TreeDigest || !reflect.DeepEqual(targetAfter.Manifest, targetBefore.Manifest) || targetAfter.TotalDifferences != 0
	}
	if targetChanged {
		if targetErr == nil {
			targetErr = fmt.Errorf("verification target changed during gates")
		}
		evidence.Phases = append(evidence.Phases, failedPhase("target-unchanged", "TARGET_DRIFT", targetErr))
	} else {
		evidence.Phases = append(evidence.Phases, PhaseResult{ID: "target-unchanged", Result: ResultPass})
	}
	finalState, sourceErr := dependencies.captureSourceState(ctx, request.Source, request.Lock)
	evidence.Source.After = finalState
	sourceChanged := sourceErr != nil || finalState != snapshot.RepositoryState
	if sourceChanged {
		if sourceErr == nil {
			sourceErr = fmt.Errorf("source repository state changed during verification")
		}
		evidence.Phases = append(evidence.Phases, failedPhase("source-unchanged", "SOURCE_DRIFT", sourceErr))
	} else {
		evidence.Phases = append(evidence.Phases, PhaseResult{ID: "source-unchanged", Result: ResultPass})
	}
	if gateFailed || isolationErr != nil || moduleChanged || targetChanged || sourceChanged {
		evidence.OverallResult = ResultFail
		return evidence, fmt.Errorf("one or more final verification gates failed")
	}
	evidence.OverallResult = ResultPass
	return evidence, nil
}

func failedPhase(id, kind string, err error) PhaseResult {
	summary := ""
	if err != nil {
		summary = err.Error()
	}
	return PhaseResult{ID: id, Result: ResultFail, FailureKind: kind, Summary: summary}
}

func setTargetComparisonEvidence(target *TargetEvidence, comparison TargetComparison, before bool) {
	if before {
		target.BeforeTreeSHA256 = comparison.TreeDigest
		target.TotalDifferences = comparison.TotalDifferences
		target.Differences = append([]TreeDifference(nil), comparison.Differences...)
		target.DifferencesCutOff = comparison.Truncated
	} else {
		target.AfterTreeSHA256 = comparison.TreeDigest
	}
}

func verifyRenderReproducibility(first, second *Candidate) (RenderEvidence, RenderEvidence, error) {
	if first == nil || second == nil || first.publishable || second.publishable {
		return RenderEvidence{}, RenderEvidence{}, fmt.Errorf("reproducibility requires two verification candidates")
	}
	firstReport, secondReport := first.Report(), second.Report()
	firstEvidence := renderEvidenceFromReport(firstReport)
	secondEvidence := renderEvidenceFromReport(secondReport)
	if !renderReportsReproducible(firstReport, secondReport) {
		return firstEvidence, secondEvidence, fmt.Errorf("independent renders differ")
	}
	return firstEvidence, secondEvidence, nil
}

func renderReportsReproducible(first, second CandidateReport) bool {
	firstCommands := append([]CommandReport(nil), first.Commands...)
	secondCommands := append([]CommandReport(nil), second.Commands...)
	first.Commands = nil
	second.Commands = nil
	if !reflect.DeepEqual(first, second) || len(firstCommands) != len(secondCommands) {
		return false
	}
	for index := range firstCommands {
		if firstCommands[index].Name != secondCommands[index].Name || firstCommands[index].ExitCode != secondCommands[index].ExitCode {
			return false
		}
	}
	return true
}

func renderEvidenceFromReport(report CandidateReport) RenderEvidence {
	return RenderEvidence{
		ManifestSHA256: digestFullManifest(report.Manifest),
		TreeSHA256:     report.TreeDigest,
		Files:          len(report.Manifest),
		Transform:      report.Transform,
		Credentials:    report.Credentials,
		Scan:           report.Scan,
		Commands:       append([]CommandReport(nil), report.Commands...),
	}
}

func digestFullManifest(manifest []ManifestEntry) string {
	hasher := sha256.New()
	for _, entry := range manifest {
		writeFramedString(hasher, entry.Path)
		writeFramedString(hasher, entry.Mode)
		writeFramedString(hasher, entry.SHA256)
		writeFramedString(hasher, strconv.FormatInt(entry.Size, 10))
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func createVerificationIsolation(candidate *Candidate) (_ *VerificationIsolation, returnErr error) {
	if candidate == nil {
		return nil, fmt.Errorf("verification candidate is nil")
	}
	candidate.mu.Lock()
	defer candidate.mu.Unlock()
	if !candidate.sealed || candidate.cleaned || candidate.published || candidate.publishable || candidate.rootFD < 0 || candidate.backendFD < 0 {
		return nil, fmt.Errorf("candidate is not a sealed verification candidate")
	}
	path, err := os.MkdirTemp(candidate.backendPath, ".thingsbaseline-verify-")
	if err != nil {
		return nil, fmt.Errorf("create verification isolation: %w", err)
	}
	name := filepath.Base(path)
	if filepath.Dir(path) != candidate.backendPath || !strings.HasPrefix(name, ".thingsbaseline-verify-") {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("verification isolation escaped backend")
	}
	if err := os.Chmod(path, 0o700); err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("set verification isolation mode: %w", err)
	}
	rootFD, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("open verification isolation: %w", err)
	}
	backendFD, err := unix.Dup(candidate.backendFD)
	if err != nil {
		_ = unix.Close(rootFD)
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("duplicate backend descriptor: %w", err)
	}
	isolation := &VerificationIsolation{backendPath: candidate.backendPath, backendFD: backendFD, name: name, path: path, rootFD: rootFD}
	defer func() {
		if returnErr != nil {
			_ = isolation.Cleanup()
		}
	}()
	if err := isolation.verifyIdentity(); err != nil {
		return nil, err
	}
	copied := make(map[string]RenderedFile, len(candidate.policy.expectedFiles))
	for _, entry := range candidate.report.Manifest {
		expected, exists := candidate.policy.expectedFiles[entry.Path]
		if !exists {
			return nil, fmt.Errorf("sealed candidate policy misses %q", entry.Path)
		}
		data, mode, err := readRenderedFileFromRoot(candidate.rootFD, entry.Path, candidate.policy.lock.Security.MaxFileBytes)
		if err != nil {
			return nil, fmt.Errorf("copy sealed candidate path %q: %w", entry.Path, err)
		}
		if mode != expected.Mode || !bytes.Equal(data, expected.Data) {
			return nil, fmt.Errorf("sealed candidate changed before isolation copy")
		}
		copied[entry.Path] = RenderedFile{Mode: mode, Data: data}
	}
	if err := writeRenderedTree(rootFD, copied); err != nil {
		return nil, err
	}
	if err := normalizeCandidatePermissions(rootFD, copied); err != nil {
		return nil, err
	}
	policy := candidate.policy
	policy.afterDirectoryList = nil
	policy.afterPathStat = nil
	if _, manifest, _, err := scanTreeFromFD(path, rootFD, policy); err != nil {
		return nil, fmt.Errorf("scan verification isolation: %w", err)
	} else if digestManifest(manifest) != candidate.report.TreeDigest {
		return nil, fmt.Errorf("verification isolation digest differs from sealed candidate")
	}
	isolation.policy = policy
	isolation.treeDigest = candidate.report.TreeDigest
	return isolation, nil
}

func readRenderedFileFromRoot(rootFD int, path string, maxFileBytes int64) ([]byte, string, error) {
	parts := strings.Split(path, "/")
	parentFD, err := openExistingDirectoryAt(rootFD, parts[:len(parts)-1])
	if err != nil {
		return nil, "", err
	}
	defer unix.Close(parentFD)
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, parts[len(parts)-1], &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, "", fmt.Errorf("source is not a regular file")
	}
	data, err := readRegularAt(parentFD, parts[len(parts)-1], stat, maxFileBytes)
	if err != nil {
		return nil, "", err
	}
	mode := "100644"
	if stat.Mode&0o777 == 0o755 {
		mode = "100755"
	} else if stat.Mode&0o777 != 0o644 {
		return nil, "", fmt.Errorf("source mode is not 0644/0755")
	}
	return data, mode, nil
}

func (isolation *VerificationIsolation) Path() string {
	if isolation == nil {
		return ""
	}
	isolation.mu.Lock()
	defer isolation.mu.Unlock()
	if isolation.cleaned {
		return ""
	}
	return isolation.path
}

func (isolation *VerificationIsolation) verifyIdentity() error {
	var opened, named unix.Stat_t
	if err := unix.Fstat(isolation.rootFD, &opened); err != nil {
		return fmt.Errorf("verification isolation identity changed: %w", err)
	}
	if err := unix.Fstatat(isolation.backendFD, isolation.name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameFileIdentity(opened, named) || opened.Mode != named.Mode || named.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("verification isolation identity or metadata changed")
	}
	return nil
}

func (isolation *VerificationIsolation) VerifyUnchanged() error {
	if isolation == nil {
		return fmt.Errorf("verification isolation is nil")
	}
	isolation.mu.Lock()
	defer isolation.mu.Unlock()
	if isolation.cleaned {
		return fmt.Errorf("verification isolation is already cleaned")
	}
	if err := isolation.verifyIdentity(); err != nil {
		return err
	}
	_, manifest, _, err := scanTreeFromFD(isolation.path, isolation.rootFD, isolation.policy)
	if err != nil {
		return fmt.Errorf("rescan verification isolation: %w", err)
	}
	if digestManifest(manifest) != isolation.treeDigest {
		return fmt.Errorf("verification isolation changed during gates")
	}
	return nil
}

func (isolation *VerificationIsolation) Cleanup() error {
	if isolation == nil {
		return nil
	}
	isolation.mu.Lock()
	defer isolation.mu.Unlock()
	if isolation.cleaned {
		return nil
	}
	if err := isolation.verifyIdentity(); err != nil {
		return err
	}
	if err := os.RemoveAll(isolation.path); err != nil {
		return err
	}
	_ = unix.Close(isolation.rootFD)
	_ = unix.Close(isolation.backendFD)
	isolation.rootFD = -1
	isolation.backendFD = -1
	isolation.cleaned = true
	return nil
}

func runGateSequence(ctx context.Context, runner verificationGateRunner, specifications []GateSpecification, timeout time.Duration) []GateResult {
	results := make([]GateResult, 0, len(specifications))
	stopped := false
	for _, specification := range specifications {
		if stopped {
			results = append(results, GateResult{ID: specification.ID, Command: specification.DisplayCommand, Result: ResultNotRun, Execution: ExecutionNotRun})
			continue
		}
		result := runner.Run(ctx, specification, timeout)
		result.ID = specification.ID
		result.Command = specification.DisplayCommand
		results = append(results, result)
		if result.Result != ResultPass {
			stopped = true
		}
	}
	return results
}

var moduleInventoryPaths = []string{
	"go.work", "go.work.sum",
	"saas/go.mod", "saas/go.sum",
	"tools/go.mod", "tools/go.sum",
	"things/go.mod", "things/go.sum",
}

func captureModuleInventory(backend string) (moduleInventory, error) {
	absolute, err := filepath.Abs(backend)
	if err != nil {
		return nil, fmt.Errorf("resolve backend for module inventory: %w", err)
	}
	absolute = filepath.Clean(absolute)
	physical, err := filepath.EvalSymlinks(absolute)
	if err != nil || filepath.Clean(physical) != absolute {
		return nil, fmt.Errorf("module inventory backend contains a symlink")
	}
	rootFD, err := unix.Open(absolute, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open module inventory backend: %w", err)
	}
	defer unix.Close(rootFD)
	result := make(moduleInventory, len(moduleInventoryPaths))
	for _, relative := range moduleInventoryPaths {
		parts := strings.Split(relative, "/")
		parentFD, err := openExistingDirectoryAt(rootFD, parts[:len(parts)-1])
		if err != nil {
			return nil, fmt.Errorf("open module inventory parent for %q: %w", relative, err)
		}
		var stat unix.Stat_t
		err = unix.Fstatat(parentFD, parts[len(parts)-1], &stat, unix.AT_SYMLINK_NOFOLLOW)
		if err != nil {
			_ = unix.Close(parentFD)
			return nil, fmt.Errorf("module inventory path %q is missing or unreadable: %w", relative, err)
		}
		if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Mode&0o777 != 0o644 {
			_ = unix.Close(parentFD)
			return nil, fmt.Errorf("module inventory path %q is not a single-link 0644 regular file", relative)
		}
		data, err := readRegularAt(parentFD, parts[len(parts)-1], stat, 10<<20)
		_ = unix.Close(parentFD)
		if err != nil {
			return nil, fmt.Errorf("read module inventory path %q: %w", relative, err)
		}
		sum := sha256.Sum256(data)
		result[relative] = moduleFileRecord{Exists: true, Mode: "100644", SHA256: hex.EncodeToString(sum[:])}
	}
	return result, nil
}

func buildDefaultVerificationGates(request VerifyRequest, isolation *VerificationIsolation) ([]GateSpecification, func(), error) {
	if isolation == nil || isolation.Path() == "" {
		return nil, nil, fmt.Errorf("verification isolation is unavailable")
	}
	dependencies, err := productionRenderDependencies()
	if err != nil {
		return nil, nil, err
	}
	dependenciesOwned := true
	defer func() {
		if dependenciesOwned {
			dependencies.closeGoTool()
		}
	}()
	makePath := "/usr/bin/make"
	if info, err := os.Lstat(makePath); err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("trusted make binary is unavailable")
	}
	backend, err := filepath.Abs(request.Backend)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve gate backend: %w", err)
	}
	backend = filepath.Clean(backend)
	cacheRoot, err := os.MkdirTemp(backend, ".thingsbaseline-gates-")
	if err != nil {
		return nil, nil, fmt.Errorf("create verification gate cache: %w", err)
	}
	buildOutput := filepath.Join(cacheRoot, "build-output")
	cleanup := func() {
		dependencies.closeGoTool()
		_ = os.RemoveAll(cacheRoot)
	}
	environment, err := fixedCandidateGoEnvironment(cacheRoot, dependencies.goCache)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	makeEnvironment, err := strictMakeEnvironment(cacheRoot, dependencies.goCache)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	workspacePath := filepath.Join(cacheRoot, "go.work")
	workspace := fmt.Sprintf("go 1.24.4\n\nuse (\n\t%s\n\t%s\n\t%s\n)\n", strconv.Quote(filepath.Join(backend, "saas")), strconv.Quote(isolation.Path()), strconv.Quote(filepath.Join(backend, "tools")))
	if err := os.WriteFile(workspacePath, []byte(workspace), 0o600); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("write verification workspace: %w", err)
	}
	workspaceEnvironment := setEnvironmentValue(environment, "GOWORK", workspacePath)
	redactions := []string{backend, isolation.Path(), cacheRoot, dependencies.goCache.Build, dependencies.goCache.Modules}
	redactions = append(redactions, transportProxyEnvironmentValues(environment, makeEnvironment)...)
	specifications := []GateSpecification{
		{ID: "tools-test", DisplayCommand: "cd backend/tools && GOWORK=off go test -count=1 ./...", Path: dependencies.goBinary, Args: []string{"test", "-count=1", "./..."}, Dir: filepath.Join(backend, "tools"), Env: environment, RedactValues: redactions},
		{ID: "saas-test", DisplayCommand: "cd backend/saas && GOWORK=off go test -count=1 ./...", Path: dependencies.goBinary, Args: []string{"test", "-count=1", "./..."}, Dir: filepath.Join(backend, "saas"), Env: environment, RedactValues: redactions},
		{ID: "things-test", DisplayCommand: "cd verification-tree && GOWORK=off go test -count=1 ./...", Path: dependencies.goBinary, Args: []string{"test", "-count=1", "./..."}, Dir: isolation.Path(), Env: environment, RedactValues: redactions},
		{ID: "workspace-test", DisplayCommand: "go test -count=1 Ming workspace modules", Path: dependencies.goBinary, Args: []string{"test", "-count=1", "gitee.com/ming-iot/saas/...", "gitee.com/ming-iot/things/...", "ming-iot/backend/tools/..."}, Dir: backend, Env: workspaceEnvironment, RedactValues: redactions},
		{
			ID: "things-build", DisplayCommand: "cd verification-tree && make BIN_DIR=backend/bin build", Path: makePath,
			Args: []string{"GO=/proc/self/fd/3", "BIN_DIR=" + buildOutput, "build"}, Dir: isolation.Path(), Env: makeEnvironment, RedactValues: redactions,
			goTool: dependencies.goTool, outputContract: &gateOutputContract{directory: buildOutput, logicalRoot: "backend/bin", names: []string{"thingsvr", "dgsvr", "dmsvr"}, mode: 0o755},
		},
	}
	dependenciesOwned = false
	return specifications, cleanup, nil
}

func strictMakeEnvironment(privateRoot string, cache sharedGoCachePaths) ([]string, error) {
	candidateEnvironment, err := fixedCandidateGoEnvironment(privateRoot, cache)
	if err != nil {
		return nil, err
	}
	candidateValues := make(map[string]string, len(candidateEnvironment))
	for _, item := range candidateEnvironment {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			candidateValues[key] = value
		}
	}
	allowedGoAndGit := []string{
		"HOME", "GOCACHE", "GOMODCACHE", "GOTMPDIR",
		"GOENV", "GOFLAGS", "GOWORK", "GOTOOLCHAIN", "GOPROXY", "GOSUMDB", "GOPRIVATE", "GONOPROXY", "GONOSUMDB", "GOINSECURE",
		"GIT_NO_REPLACE_OBJECTS", "GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_GLOBAL", "GIT_TERMINAL_PROMPT", "GIT_OPTIONAL_LOCKS", "GIT_LITERAL_PATHSPECS", "GIT_ATTR_NOSYSTEM",
		"LC_ALL", "LANG",
	}
	result := make([]string, 0, len(allowedGoAndGit)+24)
	for _, key := range allowedGoAndGit {
		value, exists := candidateValues[key]
		if !exists {
			return nil, fmt.Errorf("strict make environment is missing %s", key)
		}
		result = append(result, key+"="+value)
	}
	result = append(result,
		"PATH=/usr/bin:/bin",
		"MAKEFILES=", "MAKEFLAGS=", "GNUMAKEFLAGS=", "MFLAGS=", "MAKEOVERRIDES=", "MAKELEVEL=0", "MAKE_RESTARTS=", "MAKE_TERMOUT=", "MAKE_TERMERR=",
		"GO_BUILD=", "SHELL=/bin/sh",
		"BASH_ENV=", "ENV=", "CDPATH=", "SHELLOPTS=", "BASHOPTS=", "KSH_ENV=", "ZDOTDIR=",
	)
	for _, key := range transportProxyEnvironmentKeys {
		if value, exists := candidateValues[key]; exists {
			result = append(result, key+"="+value)
		}
	}
	sort.Strings(result)
	return result, nil
}

func setEnvironmentValue(environment []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(environment)+1)
	replaced := false
	for _, item := range environment {
		if strings.HasPrefix(item, prefix) {
			result = append(result, prefix+value)
			replaced = true
		} else {
			result = append(result, item)
		}
	}
	if !replaced {
		result = append(result, prefix+value)
	}
	sort.Strings(result)
	return result
}

func compareCandidateTarget(candidate *Candidate) (TargetComparison, error) {
	if candidate == nil {
		return TargetComparison{}, fmt.Errorf("verification candidate is nil")
	}
	candidate.mu.Lock()
	defer candidate.mu.Unlock()
	if !candidate.sealed || candidate.cleaned || candidate.published || candidate.publishable || candidate.backendFD < 0 {
		return TargetComparison{}, fmt.Errorf("candidate is not a sealed verification candidate")
	}

	var named unix.Stat_t
	if err := unix.Fstatat(candidate.backendFD, candidate.targetName, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil || named.Mode&unix.S_IFMT != unix.S_IFDIR {
		return TargetComparison{}, fmt.Errorf("verification target is not a real directory")
	}
	targetFD, err := unix.Openat(candidate.backendFD, candidate.targetName, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return TargetComparison{}, fmt.Errorf("open verification target: %w", err)
	}
	defer unix.Close(targetFD)
	var opened unix.Stat_t
	if err := unix.Fstat(targetFD, &opened); err != nil || !sameFileIdentity(named, opened) || opened.Mode != named.Mode {
		return TargetComparison{}, fmt.Errorf("verification target identity or metadata changed before scan")
	}

	policy := candidate.policy
	policy.allowTreeDifferences = true
	targetPath := filepath.Join(candidate.backendPath, candidate.targetName)
	scan, manifest, credentials, err := scanTreeFromFD(targetPath, targetFD, policy)
	if err != nil {
		return TargetComparison{}, fmt.Errorf("scan verification target: %w", err)
	}
	var after unix.Stat_t
	if err := unix.Fstatat(candidate.backendFD, candidate.targetName, &after, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameFileIdentity(opened, after) || after.Mode != opened.Mode {
		return TargetComparison{}, fmt.Errorf("verification target identity or metadata changed during scan")
	}

	differences, total := compareManifests(candidate.report.Manifest, manifest)
	return TargetComparison{
		TreeDigest:       digestManifest(manifest),
		TotalDifferences: total,
		Truncated:        total > len(differences),
		Differences:      differences,
		Scan:             scan,
		Credentials:      credentials,
		Manifest:         append([]ManifestEntry(nil), manifest...),
	}, nil
}

func compareManifests(expected, actual []ManifestEntry) ([]TreeDifference, int) {
	expectedByPath := make(map[string]ManifestEntry, len(expected))
	actualByPath := make(map[string]ManifestEntry, len(actual))
	paths := make(map[string]struct{}, len(expected)+len(actual))
	for _, entry := range expected {
		expectedByPath[entry.Path] = entry
		paths[entry.Path] = struct{}{}
	}
	for _, entry := range actual {
		actualByPath[entry.Path] = entry
		paths[entry.Path] = struct{}{}
	}
	sortedPaths := make([]string, 0, len(paths))
	for path := range paths {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)
	differences := make([]TreeDifference, 0)
	total := 0
	for _, path := range sortedPaths {
		want, wantOK := expectedByPath[path]
		got, gotOK := actualByPath[path]
		if wantOK && gotOK && want.Mode == got.Mode && want.SHA256 == got.SHA256 {
			continue
		}
		total++
		if len(differences) >= maxReportedTreeDifferences {
			continue
		}
		difference := TreeDifference{Path: path}
		if wantOK {
			difference.ExpectedMode = want.Mode
			difference.ExpectedSHA256 = want.SHA256
		}
		if gotOK {
			difference.ActualMode = got.Mode
			difference.ActualSHA256 = got.SHA256
		}
		differences = append(differences, difference)
	}
	return differences, total
}
