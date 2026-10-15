package thingsbaseline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"

	templates "ming-iot/backend/tools/templates/things-baseline"

	"golang.org/x/sys/unix"
)

const (
	lockedGoVersion           = "go1.25.4"
	lockedLicenseSHA256       = "20b067f86de375aae6db0f283ab2e65de24d537733b89bd58432c101259d84cf"
	defaultCandidateGoProxy   = "https://goproxy.cn,direct"
	mingGoProxyEnvironment    = "MING_GOPROXY"
	maxCommandDiagnosticTail  = 1900
	maxCommandDiagnosticTotal = 3800
)

var templateTokenPattern = regexp.MustCompile(`\{\{[A-Z0-9_]+\}\}`)

var transportProxyEnvironmentKeys = []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"}

// ManifestEntry is a stable, path-relative description of one regular file.
type ManifestEntry struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// CredentialProfileReport is the public, machine-readable credential proof.
type CredentialProfileReport struct {
	Files     int                  `json:"files"`
	Active    int                  `json:"active"`
	Commented int                  `json:"commented"`
	Paths     CredentialPathCounts `json:"paths"`
}

// SecurityScanReport summarizes a successful scan without exposing contents.
type SecurityScanReport struct {
	RegularFiles       int   `json:"regularFiles"`
	Directories        int   `json:"directories"`
	TrailingWhitespace int   `json:"trailingWhitespace"`
	TotalBytes         int64 `json:"totalBytes"`
}

// CommandReport records a controlled Go command without cwd, env, or cache paths.
type CommandReport struct {
	Name         string `json:"name"`
	ExitCode     int    `json:"exitCode"`
	StdoutSHA256 string `json:"stdoutSha256,omitempty"`
	StderrSHA256 string `json:"stderrSha256,omitempty"`
	StdoutTail   string `json:"stdoutTail,omitempty"`
	StderrTail   string `json:"stderrTail,omitempty"`
}

// CandidateReport is an immutable-by-copy report for one sealed render.
type CandidateReport struct {
	PreTidyManifest []ManifestEntry         `json:"preTidyManifest"`
	Manifest        []ManifestEntry         `json:"manifest"`
	TreeDigest      string                  `json:"treeDigest"`
	Transform       TransformReport         `json:"transform"`
	Credentials     CredentialProfileReport `json:"credentials"`
	Scan            SecurityScanReport      `json:"scan"`
	GoVersion       string                  `json:"goVersion"`
	Commands        []CommandReport         `json:"commands"`
}

// GoCommand is the fixed argv/env input accepted by the injectable runner.
type GoCommand struct {
	Path   string
	Dir    string
	Args   []string
	Env    []string
	goTool *attestedGoTool
}

type goCommandRunner interface {
	Run(context.Context, GoCommand) (CommandReport, error)
}

type renderDependencies struct {
	runner                goCommandRunner
	goBinary              string
	goVersion             string
	goTool                *attestedGoTool
	goCache               sharedGoCachePaths
	expectedLicenseSHA    string
	renameNoReplace       func(int, string, int, string) error
	afterCandidateCreated func(string) error
	beforeDigest          func() error
}

type candidateIntent uint8

const (
	candidateIntentPublish candidateIntent = iota + 1
	candidateIntentVerify
)

type securityPolicy struct {
	lock                      *Lock
	expectedFiles             map[string]RenderedFile
	expectedTemplates         map[string]RenderedFile
	expectedLicenseSHA        string
	allowedTrailingWhitespace map[trailingWhitespaceFingerprint]struct{}
	afterDirectoryList        func(string)
	afterPathStat             func(string)
	allowTreeDifferences      bool
}

// Candidate owns one sealed private tree. Its path is deliberately not part
// of the public API; callers can inspect a deep-copy report, clean, or publish.
type Candidate struct {
	mu          sync.Mutex
	backendPath string
	backendFD   int
	name        string
	path        string
	rootFD      int
	targetName  string
	sealed      bool
	cleaned     bool
	published   bool
	publishable bool
	report      CandidateReport
	policy      securityPolicy
	rename      func(int, string, int, string) error
}

// RenderCandidate performs one authoritative render using the current trusted
// Go toolchain. It never writes untransformed source bytes to disk.
func RenderCandidate(ctx context.Context, backend string, snapshot SourceSnapshot, lock *Lock) (*Candidate, error) {
	dependencies, err := productionRenderDependencies()
	if err != nil {
		return nil, err
	}
	defer dependencies.closeGoTool()
	return renderCandidateForIntentWithDependencies(ctx, backend, snapshot, lock, candidateIntentPublish, dependencies)
}

func renderVerificationCandidate(ctx context.Context, backend string, snapshot SourceSnapshot, lock *Lock) (*Candidate, error) {
	dependencies, err := productionRenderDependencies()
	if err != nil {
		return nil, err
	}
	defer dependencies.closeGoTool()
	return renderCandidateForIntentWithDependencies(ctx, backend, snapshot, lock, candidateIntentVerify, dependencies)
}

func productionRenderDependencies() (renderDependencies, error) {
	if runtime.Version() != lockedGoVersion {
		return renderDependencies{}, fmt.Errorf("renderer Go version = %q, want %q", runtime.Version(), lockedGoVersion)
	}
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	goTool, err := openAttestedGoTool(context.Background(), goBinary)
	if err != nil {
		return renderDependencies{}, fmt.Errorf("attest trusted Go binary: %w", err)
	}
	goCache, err := resolveSharedGoCachePathsFromRetainedFile(context.Background(), goTool.inheritedFile())
	if err != nil {
		_ = goTool.Close()
		return renderDependencies{}, fmt.Errorf("resolve shared Go cache: %w", err)
	}
	if err := goTool.revalidate(context.Background()); err != nil {
		_ = goTool.Close()
		return renderDependencies{}, fmt.Errorf("revalidate trusted Go binary after cache resolution: %w", err)
	}
	return renderDependencies{
		runner:             execGoRunner{},
		goBinary:           goBinary,
		goVersion:          goTool.attestedVersion(),
		goTool:             goTool,
		goCache:            goCache,
		expectedLicenseSHA: lockedLicenseSHA256,
		renameNoReplace:    linuxRenameNoReplace,
	}, nil
}

func (dependencies *renderDependencies) closeGoTool() {
	if dependencies != nil && dependencies.goTool != nil {
		_ = dependencies.goTool.Close()
		dependencies.goTool = nil
	}
}

func renderCandidateWithDependencies(ctx context.Context, backend string, snapshot SourceSnapshot, lock *Lock, dependencies renderDependencies) (_ *Candidate, returnErr error) {
	return renderCandidateForIntentWithDependencies(ctx, backend, snapshot, lock, candidateIntentPublish, dependencies)
}

func renderVerificationCandidateWithDependencies(ctx context.Context, backend string, snapshot SourceSnapshot, lock *Lock, dependencies renderDependencies) (_ *Candidate, returnErr error) {
	return renderCandidateForIntentWithDependencies(ctx, backend, snapshot, lock, candidateIntentVerify, dependencies)
}

func renderCandidateForIntentWithDependencies(ctx context.Context, backend string, snapshot SourceSnapshot, lock *Lock, intent candidateIntent, dependencies renderDependencies) (_ *Candidate, returnErr error) {
	if err := ValidateLock(lock); err != nil {
		return nil, fmt.Errorf("validate render lock: %w", err)
	}
	frozenLock := cloneRenderLock(lock)
	lock = &frozenLock
	snapshot = cloneSourceSnapshot(snapshot)
	if dependencies.runner == nil || dependencies.goBinary == "" || dependencies.goVersion != lockedGoVersion || dependencies.goCache.Build == "" || dependencies.goCache.Modules == "" || !isLowercaseHex(dependencies.expectedLicenseSHA, 64) {
		return nil, fmt.Errorf("renderer dependencies are incomplete")
	}
	if dependencies.renameNoReplace == nil {
		dependencies.renameNoReplace = linuxRenameNoReplace
	}
	if err := validateSourceSnapshot(snapshot, lock); err != nil {
		return nil, err
	}
	backendPath, backendFD, err := openPhysicalBackend(backend, lock.Target.Directory, intent)
	if err != nil {
		return nil, err
	}
	backendOwned := true
	defer func() {
		if backendOwned {
			_ = unix.Close(backendFD)
		}
	}()

	files := make(map[string]SourceFile, len(snapshot.Files))
	for _, file := range snapshot.Files {
		file.Data = append([]byte(nil), file.Data...)
		files[file.Path] = file
	}
	// This complete in-memory transform is intentionally before MkdirTemp.
	transformed, transformReport, err := TransformTree(files, lock)
	if err != nil {
		return nil, err
	}
	targetTemplates, err := renderTargetTemplates(lock, snapshot, transformReport)
	if err != nil {
		return nil, err
	}
	combined := make(map[string]RenderedFile, len(transformed)+len(targetTemplates))
	for path, file := range transformed {
		combined[path] = RenderedFile{Mode: file.Mode, Data: append([]byte(nil), file.Data...)}
	}
	for path, file := range targetTemplates {
		if _, exists := combined[path]; exists {
			return nil, fmt.Errorf("target template path %q collides with transformed source", path)
		}
		combined[path] = RenderedFile{Mode: file.Mode, Data: append([]byte(nil), file.Data...)}
	}
	if len(combined) != lock.Source.SelectedFiles+4 {
		return nil, fmt.Errorf("pre-tidy file count = %d, want %d", len(combined), lock.Source.SelectedFiles+4)
	}
	preManifest := manifestFromRendered(combined)

	candidatePath, err := os.MkdirTemp(backendPath, ".thingsbaseline-")
	if err != nil {
		return nil, fmt.Errorf("create private candidate: %w", err)
	}
	candidateName := filepath.Base(candidatePath)
	if filepath.Dir(candidatePath) != backendPath || !strings.HasPrefix(candidateName, ".thingsbaseline-") {
		_ = os.RemoveAll(candidatePath)
		return nil, fmt.Errorf("candidate escaped backend boundary")
	}
	if err := os.Chmod(candidatePath, 0o700); err != nil {
		_ = os.RemoveAll(candidatePath)
		return nil, fmt.Errorf("set private candidate mode: %w", err)
	}
	rootFD, err := unix.Open(candidatePath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = os.RemoveAll(candidatePath)
		return nil, fmt.Errorf("open private candidate: %w", err)
	}
	candidate := &Candidate{
		backendPath: backendPath,
		backendFD:   backendFD,
		name:        candidateName,
		path:        candidatePath,
		rootFD:      rootFD,
		targetName:  lock.Target.Directory,
		publishable: intent == candidateIntentPublish,
		rename:      dependencies.renameNoReplace,
	}
	backendOwned = false
	defer func() {
		if returnErr != nil {
			_ = candidate.Cleanup()
		}
	}()
	if dependencies.afterCandidateCreated != nil {
		if err := dependencies.afterCandidateCreated(candidatePath); err != nil {
			return nil, fmt.Errorf("after candidate creation: %w", err)
		}
	}
	if err := candidate.verifyNamedIdentity(); err != nil {
		return nil, err
	}

	if err := writeRenderedTree(rootFD, combined); err != nil {
		return nil, err
	}
	if err := candidate.verifyNamedIdentity(); err != nil {
		return nil, err
	}
	beforeTidy, err := readCandidateFileStates(candidatePath, lock.Security.MaxFileBytes)
	if err != nil {
		return nil, fmt.Errorf("inspect pre-tidy candidate: %w", err)
	}
	privateRoot, err := os.MkdirTemp(backendPath, ".thingsbaseline-cache-")
	if err != nil {
		return nil, fmt.Errorf("create private Go environment: %w", err)
	}
	defer os.RemoveAll(privateRoot)
	goEnvironment, err := fixedCandidateGoEnvironment(privateRoot, dependencies.goCache)
	if err != nil {
		return nil, err
	}
	commands := make([]CommandReport, 0, 2)
	tidyReport, err := dependencies.runner.Run(ctx, GoCommand{Path: dependencies.goBinary, Dir: candidatePath, Args: []string{"mod", "tidy"}, Env: goEnvironment, goTool: dependencies.goTool})
	tidyReport.Name = "go-mod-tidy"
	commands = append(commands, tidyReport)
	if err != nil {
		return nil, fmt.Errorf("controlled go mod tidy failed: %w", err)
	}
	if err := candidate.verifyNamedIdentity(); err != nil {
		return nil, err
	}
	if tidyReport.ExitCode != 0 {
		return nil, fmt.Errorf("controlled go mod tidy returned non-zero exit code %d", tidyReport.ExitCode)
	}
	afterTidy, err := readCandidateFileStates(candidatePath, lock.Security.MaxFileBytes)
	if err != nil {
		return nil, fmt.Errorf("inspect post-tidy candidate: %w", err)
	}
	if err := verifyAllowedTidyChanges(combined, beforeTidy, afterTidy); err != nil {
		return nil, err
	}
	verifyReport, err := dependencies.runner.Run(ctx, GoCommand{Path: dependencies.goBinary, Dir: candidatePath, Args: []string{"mod", "verify"}, Env: goEnvironment, goTool: dependencies.goTool})
	verifyReport.Name = "go-mod-verify"
	commands = append(commands, verifyReport)
	if err != nil {
		return nil, fmt.Errorf("controlled go mod verify failed: %w", err)
	}
	if err := candidate.verifyNamedIdentity(); err != nil {
		return nil, err
	}
	if verifyReport.ExitCode != 0 {
		return nil, fmt.Errorf("controlled go mod verify returned non-zero exit code %d", verifyReport.ExitCode)
	}
	afterVerify, err := readCandidateFileStates(candidatePath, lock.Security.MaxFileBytes)
	if err != nil {
		return nil, fmt.Errorf("inspect post-verify candidate: %w", err)
	}
	if !diskFileStateMapsEqual(afterTidy, afterVerify) {
		return nil, fmt.Errorf("go mod verify modified candidate files")
	}

	expectedFinal := cloneRenderedMap(combined)
	for _, path := range []string{"go.mod", "go.sum"} {
		file := expectedFinal[path]
		file.Data = append([]byte(nil), afterVerify[path].Data...)
		expectedFinal[path] = file
	}
	if err := normalizeCandidatePermissions(candidate.rootFD, expectedFinal); err != nil {
		return nil, err
	}
	if err := candidate.verifyNamedIdentity(); err != nil {
		return nil, err
	}
	policy := securityPolicy{
		lock:                      lock,
		expectedFiles:             expectedFinal,
		expectedTemplates:         targetTemplates,
		expectedLicenseSHA:        dependencies.expectedLicenseSHA,
		allowedTrailingWhitespace: collectSourceTrailingWhitespace(snapshot.Files),
	}
	scanReport, manifest, credentials, err := scanTreeFromFD(candidatePath, candidate.rootFD, policy)
	if err != nil {
		return nil, err
	}
	if err := candidate.verifyNamedIdentity(); err != nil {
		return nil, err
	}
	if dependencies.beforeDigest != nil {
		if err := dependencies.beforeDigest(); err != nil {
			return nil, fmt.Errorf("before candidate digest: %w", err)
		}
	}
	treeDigest := digestManifest(manifest)
	candidate.report = CandidateReport{
		PreTidyManifest: append([]ManifestEntry(nil), preManifest...),
		Manifest:        append([]ManifestEntry(nil), manifest...),
		TreeDigest:      treeDigest,
		Transform:       transformReport,
		Credentials:     credentials,
		Scan:            scanReport,
		GoVersion:       dependencies.goVersion,
		Commands:        append([]CommandReport(nil), commands...),
	}
	candidate.policy = policy
	candidate.sealed = true
	return candidate, nil
}

func cloneRenderLock(lock *Lock) Lock {
	clone := *lock
	clone.Source.Include = append([]string(nil), lock.Source.Include...)
	clone.Source.Exclude = append([]string(nil), lock.Source.Exclude...)
	return clone
}

func cloneSourceSnapshot(snapshot SourceSnapshot) SourceSnapshot {
	clone := snapshot
	clone.Files = make([]SourceFile, len(snapshot.Files))
	for index, file := range snapshot.Files {
		file.Data = append([]byte(nil), file.Data...)
		clone.Files[index] = file
	}
	return clone
}

func validateSourceSnapshot(snapshot SourceSnapshot, lock *Lock) error {
	if snapshot.Commit != lock.Source.Commit || snapshot.Tree != lock.Source.Tree {
		return fmt.Errorf("source snapshot commit/tree does not match lock")
	}
	if len(snapshot.Files) != lock.Source.SelectedFiles {
		return fmt.Errorf("source snapshot files = %d, want %d", len(snapshot.Files), lock.Source.SelectedFiles)
	}
	var total int64
	previous := ""
	for index, file := range snapshot.Files {
		if err := validateGitPath("source snapshot", file.Path); err != nil {
			return err
		}
		if file.Mode != "100644" && file.Mode != "100755" {
			return fmt.Errorf("source snapshot path %q has unsupported mode %q", file.Path, file.Mode)
		}
		if index > 0 && file.Path <= previous {
			return fmt.Errorf("source snapshot paths are not unique and sorted")
		}
		previous = file.Path
		total += int64(len(file.Data))
	}
	if total != snapshot.SelectedBlobBytes || total != lock.Source.SelectedBlobBytes {
		return fmt.Errorf("source snapshot blob bytes = %d, snapshot reports %d, lock wants %d", total, snapshot.SelectedBlobBytes, lock.Source.SelectedBlobBytes)
	}
	return nil
}

func openPhysicalBackend(backend, target string, intent candidateIntent) (string, int, error) {
	if filepath.Base(target) != target || target == "." || target == ".." {
		return "", -1, fmt.Errorf("target directory must be one direct backend child")
	}
	absolute, err := filepath.Abs(backend)
	if err != nil {
		return "", -1, fmt.Errorf("resolve backend path: %w", err)
	}
	absolute = filepath.Clean(absolute)
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", -1, fmt.Errorf("inspect backend: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", -1, fmt.Errorf("backend must be a real directory, not a symlink")
	}
	physical, err := filepath.EvalSymlinks(absolute)
	if err != nil || filepath.Clean(physical) != absolute {
		return "", -1, fmt.Errorf("backend path contains a symlink")
	}
	fd, err := unix.Open(absolute, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", -1, fmt.Errorf("open backend directory: %w", err)
	}
	var targetStat unix.Stat_t
	err = unix.Fstatat(fd, target, &targetStat, unix.AT_SYMLINK_NOFOLLOW)
	switch intent {
	case candidateIntentPublish:
		if err == nil {
			_ = unix.Close(fd)
			return "", -1, fmt.Errorf("target %q already exists", target)
		}
		if err != unix.ENOENT {
			_ = unix.Close(fd)
			return "", -1, fmt.Errorf("inspect target %q: %w", target, err)
		}
	case candidateIntentVerify:
		if err != nil {
			_ = unix.Close(fd)
			return "", -1, fmt.Errorf("verification target %q must exist as a real directory: %w", target, err)
		}
		if targetStat.Mode&unix.S_IFMT != unix.S_IFDIR {
			_ = unix.Close(fd)
			return "", -1, fmt.Errorf("verification target %q must be a real directory", target)
		}
	default:
		_ = unix.Close(fd)
		return "", -1, fmt.Errorf("unknown candidate intent")
	}
	return absolute, fd, nil
}

func renderTargetTemplates(lock *Lock, snapshot SourceSnapshot, report TransformReport) (map[string]RenderedFile, error) {
	tokens := map[string]string{
		"{{SOURCE_REPOSITORY}}":               lock.Source.Repository,
		"{{SOURCE_REF}}":                      lock.Source.Ref,
		"{{SOURCE_COMMIT}}":                   snapshot.Commit,
		"{{SOURCE_TREE}}":                     snapshot.Tree,
		"{{SOURCE_MODULE}}":                   lock.Source.Module,
		"{{TARGET_MODULE}}":                   lock.Target.Module,
		"{{SAAS_MODULE}}":                     lock.Target.SaaSModule,
		"{{SHARE_MODULE}}":                    lock.Target.ShareModule,
		"{{SHARE_VERSION}}":                   lock.Target.ShareVersion,
		"{{SOURCE_FILE_COUNT}}":               strconv.Itoa(lock.Source.SelectedFiles),
		"{{TARGET_FILE_COUNT}}":               strconv.Itoa(lock.Source.SelectedFiles + 4),
		"{{CORE_PREFIX_COUNT}}":               strconv.Itoa(report.CorePrefixes),
		"{{THINGS_PREFIX_COUNT}}":             strconv.Itoa(report.ThingsPrefixes),
		"{{CREDENTIAL_LINE_COUNT}}":           strconv.Itoa(report.CredentialLines),
		"{{PRIVATE_KEY_BLOCK_COUNT}}":         strconv.Itoa(report.PrivateKeyBlocks),
		"{{VET_FIX_COUNT}}":                   strconv.Itoa(report.VetFixes),
		"{{SCHEMA_FIXTURE_IDENTIFIER_COUNT}}": strconv.Itoa(report.SchemaFixtureIdentifiers),
		"{{NON_YAML_CREDENTIAL_VALUE_COUNT}}": strconv.Itoa(report.NonYAMLCredentialValues),
	}
	result := make(map[string]RenderedFile, 4)
	for _, name := range []string{"README.md", "NOTICE", "PROVENANCE.yaml", "Makefile"} {
		data, err := templates.FS.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read embedded template %q: %w", name, err)
		}
		data, err = renderTemplateData(name, data, tokens)
		if err != nil {
			return nil, err
		}
		result[name] = RenderedFile{Mode: "100644", Data: append([]byte(nil), data...)}
	}
	return result, nil
}

func renderTemplateData(name string, data []byte, tokens map[string]string) ([]byte, error) {
	rendered := append([]byte(nil), data...)
	for _, token := range sortedStringKeys(tokens) {
		rendered = bytes.ReplaceAll(rendered, []byte(token), []byte(tokens[token]))
	}
	if unknown := templateTokenPattern.Find(rendered); unknown != nil || bytes.Contains(rendered, []byte("{{")) || bytes.Contains(rendered, []byte("}}")) {
		return nil, fmt.Errorf("template %q contains an unknown or unreplaced token", name)
	}
	return rendered, nil
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func writeRenderedTree(rootFD int, files map[string]RenderedFile) error {
	paths := sortedRenderedFilePaths(files)
	for _, path := range paths {
		if err := validateGitPath("rendered", path); err != nil {
			return err
		}
		parts := strings.Split(path, "/")
		parentFD, err := openOrCreateDirectoryAt(rootFD, parts[:len(parts)-1])
		if err != nil {
			return fmt.Errorf("create parent for %q: %w", path, err)
		}
		fd, err := unix.Openat(parentFD, parts[len(parts)-1], unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
		if err != nil {
			_ = unix.Close(parentFD)
			return fmt.Errorf("exclusive create %q: %w", path, err)
		}
		data := files[path].Data
		for len(data) > 0 {
			written, writeErr := unix.Write(fd, data)
			if writeErr != nil {
				_ = unix.Close(fd)
				_ = unix.Close(parentFD)
				return fmt.Errorf("write %q: %w", path, writeErr)
			}
			if written == 0 {
				_ = unix.Close(fd)
				_ = unix.Close(parentFD)
				return fmt.Errorf("write %q made no progress", path)
			}
			data = data[written:]
		}
		if err := unix.Fsync(fd); err != nil {
			_ = unix.Close(fd)
			_ = unix.Close(parentFD)
			return fmt.Errorf("sync %q: %w", path, err)
		}
		if err := unix.Close(fd); err != nil {
			_ = unix.Close(parentFD)
			return fmt.Errorf("close %q: %w", path, err)
		}
		if err := unix.Fsync(parentFD); err != nil {
			_ = unix.Close(parentFD)
			return fmt.Errorf("sync parent directory for %q: %w", path, err)
		}
		if err := unix.Close(parentFD); err != nil {
			return fmt.Errorf("close parent directory for %q: %w", path, err)
		}
	}
	if err := unix.Fsync(rootFD); err != nil {
		return fmt.Errorf("sync candidate root: %w", err)
	}
	return nil
}

func openOrCreateDirectoryAt(rootFD int, parts []string) (int, error) {
	current, err := unix.Dup(rootFD)
	if err != nil {
		return -1, err
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.Contains(part, "/") {
			_ = unix.Close(current)
			return -1, fmt.Errorf("unsafe directory component")
		}
		if err := unix.Mkdirat(current, part, 0o700); err != nil && err != unix.EEXIST {
			_ = unix.Close(current)
			return -1, err
		}
		next, err := unix.Openat(current, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(current)
		if err != nil {
			return -1, err
		}
		current = next
	}
	return current, nil
}

func fixedCandidateGoEnvironment(privateRoot string, caches ...sharedGoCachePaths) ([]string, error) {
	goProxy, err := resolveCandidateGoProxy()
	if err != nil {
		return nil, err
	}
	if len(caches) != 1 {
		return nil, fmt.Errorf("shared Go cache paths are unavailable")
	}
	cache := caches[0]
	if cache.Build == "" || cache.Modules == "" {
		return nil, fmt.Errorf("shared Go cache paths are unavailable")
	}
	privateDirectories := map[string]string{
		"HOME":     filepath.Join(privateRoot, "home"),
		"GOTMPDIR": filepath.Join(privateRoot, "gotmp"),
	}
	for _, path := range privateDirectories {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return nil, fmt.Errorf("create private Go directory: %w", err)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return nil, fmt.Errorf("set private Go directory mode: %w", err)
		}
	}
	environment := make([]string, 0, len(os.Environ())+24)
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if !ok || strings.HasPrefix(key, "GO") || strings.HasPrefix(key, "GIT_") || key == mingGoProxyEnvironment || key == "HOME" || key == "LC_ALL" || key == "LANG" {
			continue
		}
		environment = append(environment, item)
	}
	fixed := []string{
		"HOME=" + privateDirectories["HOME"],
		"GOCACHE=" + cache.Build,
		"GOMODCACHE=" + cache.Modules,
		"GOTMPDIR=" + privateDirectories["GOTMPDIR"],
		"GOENV=off", "GOFLAGS=", "GOWORK=off", "GOTOOLCHAIN=local",
		"GOPROXY=" + goProxy, "GOSUMDB=sum.golang.org",
		"GOPRIVATE=", "GONOPROXY=", "GONOSUMDB=", "GOINSECURE=",
		"GIT_NO_REPLACE_OBJECTS=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_LITERAL_PATHSPECS=1", "GIT_ATTR_NOSYSTEM=1",
		"LC_ALL=C", "LANG=C",
	}
	environment = append(environment, fixed...)
	sort.Strings(environment)
	return environment, nil
}

func resolveCandidateGoProxy() (string, error) {
	goProxy := os.Getenv(mingGoProxyEnvironment)
	if goProxy == "" {
		return defaultCandidateGoProxy, nil
	}
	if err := validateCandidateGoProxy(goProxy); err != nil {
		return "", err
	}
	return goProxy, nil
}

func validateCandidateGoProxy(goProxy string) error {
	entries := splitCandidateGoProxyEntries(goProxy)
	for index, entry := range entries {
		if entry == "direct" || entry == "off" {
			continue
		}
		entryNumber := index + 1
		if entry == "" {
			return fmt.Errorf("MING_GOPROXY entry %d is empty", entryNumber)
		}
		parsed, err := url.Parse(entry)
		if err != nil || !parsed.IsAbs() {
			return fmt.Errorf("MING_GOPROXY entry %d must be an absolute HTTPS URL", entryNumber)
		}
		if parsed.Scheme != "https" {
			return fmt.Errorf("MING_GOPROXY entry %d must use HTTPS", entryNumber)
		}
		if parsed.Hostname() == "" {
			return fmt.Errorf("MING_GOPROXY entry %d must include a host", entryNumber)
		}
		if parsed.User != nil {
			return fmt.Errorf("MING_GOPROXY entry %d must not contain credentials", entryNumber)
		}
		if parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(entry, "?") {
			return fmt.Errorf("MING_GOPROXY entry %d must not contain a query", entryNumber)
		}
		if parsed.Fragment != "" || strings.Contains(entry, "#") {
			return fmt.Errorf("MING_GOPROXY entry %d must not contain a fragment", entryNumber)
		}
	}
	return nil
}

func splitCandidateGoProxyEntries(goProxy string) []string {
	entries := make([]string, 0, 1)
	start := 0
	for index := 0; index < len(goProxy); index++ {
		if goProxy[index] != ',' && goProxy[index] != '|' {
			continue
		}
		entries = append(entries, goProxy[start:index])
		start = index + 1
	}
	entries = append(entries, goProxy[start:])
	return entries
}

func normalizeCandidatePermissions(rootFD int, expected map[string]RenderedFile) error {
	directories := expectedDirectoryPaths(expected)
	for _, path := range sortedRenderedFilePaths(expected) {
		parts := strings.Split(path, "/")
		parentFD, err := openExistingDirectoryAt(rootFD, parts[:len(parts)-1])
		if err != nil {
			return fmt.Errorf("open parent while normalizing %q: %w", path, err)
		}
		fileFD, err := unix.Openat(parentFD, parts[len(parts)-1], unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(parentFD)
		if err != nil {
			return fmt.Errorf("open file while normalizing %q: %w", path, err)
		}
		var stat unix.Stat_t
		if err := unix.Fstat(fileFD, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
			_ = unix.Close(fileFD)
			return fmt.Errorf("candidate path %q is not a safe regular file", path)
		}
		mode := uint32(0o644)
		if expected[path].Mode == "100755" {
			mode = 0o755
		}
		if err := unix.Fchmod(fileFD, mode); err != nil {
			_ = unix.Close(fileFD)
			return fmt.Errorf("normalize file mode for %q: %w", path, err)
		}
		if err := unix.Close(fileFD); err != nil {
			return fmt.Errorf("close normalized file %q: %w", path, err)
		}
	}
	directoryPaths := make([]string, 0, len(directories))
	for path := range directories {
		if path != "" {
			directoryPaths = append(directoryPaths, path)
		}
	}
	sort.Slice(directoryPaths, func(i, j int) bool {
		leftDepth, rightDepth := strings.Count(directoryPaths[i], "/"), strings.Count(directoryPaths[j], "/")
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		return directoryPaths[i] < directoryPaths[j]
	})
	for _, path := range directoryPaths {
		directoryFD, err := openExistingDirectoryAt(rootFD, strings.Split(path, "/"))
		if err != nil {
			return fmt.Errorf("open directory while normalizing %q: %w", path, err)
		}
		if err := unix.Fchmod(directoryFD, 0o755); err != nil {
			_ = unix.Close(directoryFD)
			return fmt.Errorf("normalize directory mode for %q: %w", path, err)
		}
		_ = unix.Close(directoryFD)
	}
	if err := unix.Fchmod(rootFD, 0o755); err != nil {
		return fmt.Errorf("normalize candidate root mode: %w", err)
	}
	return nil
}

func openExistingDirectoryAt(rootFD int, parts []string) (int, error) {
	current, err := unix.Dup(rootFD)
	if err != nil {
		return -1, err
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.Contains(part, "/") {
			_ = unix.Close(current)
			return -1, fmt.Errorf("unsafe directory component")
		}
		next, err := unix.Openat(current, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(current)
		if err != nil {
			return -1, err
		}
		current = next
	}
	return current, nil
}

type diskFileState struct {
	Data  []byte
	Mode  uint32
	Dev   uint64
	Ino   uint64
	Nlink uint64
}

func readCandidateFileStates(root string, maxFileBytes int64) (map[string]diskFileState, error) {
	result := make(map[string]diskFileState)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("candidate contains a non-regular file")
		}
		if info.Size() > maxFileBytes {
			return fmt.Errorf("candidate file exceeds maximum size")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if err := validateGitPath("candidate", relative); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var stat unix.Stat_t
		if err := unix.Lstat(path, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
			return fmt.Errorf("candidate file identity is unsafe")
		}
		result[relative] = diskFileState{Data: data, Mode: stat.Mode, Dev: stat.Dev, Ino: stat.Ino, Nlink: stat.Nlink}
		return nil
	})
	return result, err
}

func verifyAllowedTidyChanges(rendered map[string]RenderedFile, before, after map[string]diskFileState) error {
	if len(rendered) != len(before) || len(before) != len(after) {
		return fmt.Errorf("go mod tidy changed path count from %d to %d", len(before), len(after))
	}
	for _, path := range sortedRenderedFilePaths(rendered) {
		afterState, ok := after[path]
		if !ok {
			return fmt.Errorf("go mod tidy removed path %q", path)
		}
		beforeState, existed := before[path]
		if !existed {
			return fmt.Errorf("pre-tidy tree missed path %q", path)
		}
		if path != "go.mod" && path != "go.sum" && !diskFileStatesEqual(beforeState, afterState) {
			return fmt.Errorf("go mod tidy modified or replaced forbidden path %q", path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			return fmt.Errorf("go mod tidy added path %q", path)
		}
	}
	return nil
}

func diskFileStateMapsEqual(left, right map[string]diskFileState) bool {
	if len(left) != len(right) {
		return false
	}
	for path, state := range left {
		other, ok := right[path]
		if !ok || !diskFileStatesEqual(state, other) {
			return false
		}
	}
	return true
}

func diskFileStatesEqual(left, right diskFileState) bool {
	return left.Mode == right.Mode && left.Dev == right.Dev && left.Ino == right.Ino && left.Nlink == right.Nlink && bytes.Equal(left.Data, right.Data)
}

func cloneRenderedMap(files map[string]RenderedFile) map[string]RenderedFile {
	clone := make(map[string]RenderedFile, len(files))
	for path, file := range files {
		file.Data = append([]byte(nil), file.Data...)
		clone[path] = file
	}
	return clone
}

func sortedRenderedFilePaths(files map[string]RenderedFile) []string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func manifestFromRendered(files map[string]RenderedFile) []ManifestEntry {
	manifest := make([]ManifestEntry, 0, len(files))
	for _, path := range sortedRenderedFilePaths(files) {
		data := files[path].Data
		sum := sha256.Sum256(data)
		manifest = append(manifest, ManifestEntry{Path: path, Mode: files[path].Mode, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data))})
	}
	return manifest
}

func digestManifest(manifest []ManifestEntry) string {
	hasher := sha256.New()
	for _, entry := range manifest {
		writeFramedString(hasher, entry.Path)
		writeFramedString(hasher, entry.Mode)
		writeFramedString(hasher, entry.SHA256)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func writeFramedString(writer io.Writer, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = writer.Write(length[:])
	_, _ = io.WriteString(writer, value)
}

// Report returns a deep copy and never includes candidate/backend/cache paths.
func (candidate *Candidate) Report() CandidateReport {
	candidate.mu.Lock()
	defer candidate.mu.Unlock()
	return cloneCandidateReport(candidate.report)
}

func cloneCandidateReport(report CandidateReport) CandidateReport {
	report.PreTidyManifest = append([]ManifestEntry(nil), report.PreTidyManifest...)
	report.Manifest = append([]ManifestEntry(nil), report.Manifest...)
	report.Commands = append([]CommandReport(nil), report.Commands...)
	return report
}

// Cleanup is idempotent. A published candidate is disarmed and never removes
// the target tree.
func (candidate *Candidate) Cleanup() error {
	if candidate == nil {
		return nil
	}
	candidate.mu.Lock()
	defer candidate.mu.Unlock()
	if candidate.cleaned || candidate.published {
		return nil
	}
	var cleanupErr error
	cleanupPaths := make([]string, 0, 2)
	if candidate.rootFD >= 0 {
		var opened, named unix.Stat_t
		if err := unix.Fstat(candidate.rootFD, &opened); err == nil {
			if err := unix.Fstatat(candidate.backendFD, candidate.name, &named, unix.AT_SYMLINK_NOFOLLOW); err == nil {
				if sameFileIdentity(opened, named) {
					cleanupPaths = append(cleanupPaths, candidate.path)
				} else {
					cleanupErr = fmt.Errorf("candidate directory identity changed before cleanup")
					if named.Mode&unix.S_IFMT != unix.S_IFDIR {
						cleanupPaths = append(cleanupPaths, candidate.path)
					}
				}
			}
			if recovered := findBackendEntryByIdentity(candidate.backendPath, candidate.backendFD, opened); recovered != "" && recovered != candidate.name {
				cleanupPaths = append(cleanupPaths, filepath.Join(candidate.backendPath, recovered))
			}
		}
	}
	if len(cleanupPaths) == 0 && cleanupErr == nil {
		cleanupPaths = append(cleanupPaths, candidate.path)
	}
	for _, path := range cleanupPaths {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	if len(cleanupPaths) == 0 {
		return cleanupErr
	}
	if candidate.rootFD >= 0 {
		_ = unix.Close(candidate.rootFD)
		candidate.rootFD = -1
	}
	if candidate.backendFD >= 0 {
		_ = unix.Close(candidate.backendFD)
		candidate.backendFD = -1
	}
	candidate.cleaned = true
	return cleanupErr
}

func (candidate *Candidate) verifyNamedIdentity() error {
	var opened, named unix.Stat_t
	if err := unix.Fstat(candidate.rootFD, &opened); err != nil {
		return fmt.Errorf("candidate directory identity changed: %w", err)
	}
	if err := unix.Fstatat(candidate.backendFD, candidate.name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameFileIdentity(opened, named) || named.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("candidate directory identity changed")
	}
	return nil
}

func findBackendEntryByIdentity(backendPath string, backendFD int, identity unix.Stat_t) string {
	entries, err := os.ReadDir(backendPath)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, ".thingsbaseline-") {
			continue
		}
		var stat unix.Stat_t
		if err := unix.Fstatat(backendFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err == nil && sameFileIdentity(identity, stat) {
			return name
		}
	}
	return ""
}

// Publish atomically installs the sealed tree without replacing any existing
// target. There is deliberately no copy fallback.
func (candidate *Candidate) Publish() error {
	if candidate == nil {
		return fmt.Errorf("candidate is nil")
	}
	candidate.mu.Lock()
	defer candidate.mu.Unlock()
	if !candidate.publishable {
		return fmt.Errorf("verification candidate cannot be published")
	}
	if !candidate.sealed || candidate.cleaned || candidate.published {
		return fmt.Errorf("candidate is not publishable")
	}
	scan, manifest, credentials, err := scanTreeFromFD(candidate.path, candidate.rootFD, candidate.policy)
	if err != nil {
		return fmt.Errorf("revalidate sealed candidate: %w", err)
	}
	if digestManifest(manifest) != candidate.report.TreeDigest || scan != candidate.report.Scan || credentials != candidate.report.Credentials {
		return fmt.Errorf("sealed candidate changed before publish")
	}
	var opened, named, target unix.Stat_t
	if err := unix.Fstat(candidate.rootFD, &opened); err != nil {
		return fmt.Errorf("stat sealed candidate: %w", err)
	}
	if err := unix.Fstatat(candidate.backendFD, candidate.name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil || opened.Dev != named.Dev || opened.Ino != named.Ino {
		return fmt.Errorf("sealed candidate directory identity changed")
	}
	if err := unix.Fstatat(candidate.backendFD, candidate.targetName, &target, unix.AT_SYMLINK_NOFOLLOW); err == nil {
		return fmt.Errorf("target %q already exists", candidate.targetName)
	} else if err != unix.ENOENT {
		return fmt.Errorf("inspect publish target: %w", err)
	}
	if err := candidate.rename(candidate.backendFD, candidate.name, candidate.backendFD, candidate.targetName); err != nil {
		return fmt.Errorf("publish candidate without replacement: %w", err)
	}
	// The namespace change has happened. Disarm cleanup before reporting a
	// possible durability error so a caller never receives an error and then
	// accidentally treats the published target as an owned candidate.
	candidate.published = true
	_ = unix.Close(candidate.rootFD)
	candidate.rootFD = -1
	if err := unix.Fsync(candidate.backendFD); err != nil {
		_ = unix.Close(candidate.backendFD)
		candidate.backendFD = -1
		return fmt.Errorf("sync backend after publish: %w", err)
	}
	_ = unix.Close(candidate.backendFD)
	candidate.backendFD = -1
	return nil
}

func linuxRenameNoReplace(oldDirFD int, oldName string, newDirFD int, newName string) error {
	return unix.Renameat2(oldDirFD, oldName, newDirFD, newName, unix.RENAME_NOREPLACE)
}

type execGoRunner struct{}

func (execGoRunner) Run(ctx context.Context, command GoCommand) (CommandReport, error) {
	if err := ctx.Err(); err != nil {
		return CommandReport{ExitCode: -1}, err
	}
	processPath := command.Path
	var retainedGoTool *os.File
	if command.goTool != nil {
		if command.goTool.attestedVersion() != lockedGoVersion {
			return CommandReport{ExitCode: -1}, fmt.Errorf("retained Go tool attestation is unavailable")
		}
		if err := command.goTool.revalidate(ctx); err != nil {
			return CommandReport{ExitCode: -1}, fmt.Errorf("revalidate retained Go tool before command: %w", err)
		}
		retainedGoTool = command.goTool.inheritedFile()
		if retainedGoTool == nil {
			return CommandReport{ExitCode: -1}, fmt.Errorf("retained Go tool is unavailable")
		}
		processPath = "/proc/self/fd/3"
	}
	process := exec.Command(processPath, command.Args...)
	process.Dir = command.Dir
	process.Env = append([]string(nil), command.Env...)
	if retainedGoTool != nil {
		process.ExtraFiles = []*os.File{retainedGoTool}
	}
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	redactionLiterals := commandRedactionLiterals(command)
	stdout := newDigestDiagnosticCapture(maxCommandDiagnosticTail, redactionLiterals)
	stderr := newDigestDiagnosticCapture(maxCommandDiagnosticTail, redactionLiterals)
	process.Stdout = stdout
	process.Stderr = stderr
	if err := process.Start(); err != nil {
		return commandReportFromCaptures(-1, stdout, stderr), fmt.Errorf("start controlled command: %w", err)
	}
	waited := make(chan error, 1)
	go func() { waited <- process.Wait() }()
	var err error
	cancelled := false
	select {
	case err = <-waited:
	case <-ctx.Done():
		cancelled = true
		_ = unix.Kill(-process.Process.Pid, unix.SIGKILL)
		err = <-waited
	}
	report := commandReportFromCaptures(commandExitCode(err), stdout, stderr)
	if cancelled {
		return report, ctx.Err()
	}
	if command.goTool != nil {
		if revalidateErr := command.goTool.revalidate(ctx); revalidateErr != nil {
			return report, fmt.Errorf("revalidate retained Go tool after command: %w", revalidateErr)
		}
	}
	if err != nil {
		return report, fmt.Errorf("exit code %d: %s", report.ExitCode, report.StderrTail)
	}
	return report, nil
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return -1
}

func commandReportFromCaptures(exitCode int, stdout, stderr *digestDiagnosticCapture) CommandReport {
	return CommandReport{
		ExitCode:     exitCode,
		StdoutSHA256: stdout.SHA256(),
		StderrSHA256: stderr.SHA256(),
		StdoutTail:   stdout.String(),
		StderrTail:   stderr.String(),
	}
}

func redactCommandTail(value string, command GoCommand) string {
	redactor := newBoundedDiagnosticRedactor(maxCommandDiagnosticTail, commandRedactionLiterals(command))
	redactor.Write([]byte(value))
	return redactor.String()
}

func commandRedactionLiterals(command GoCommand) []string {
	values := append([]string{command.Dir}, commandEnvironmentValues(command.Env)...)
	if filepath.IsAbs(command.Path) {
		values = append(values, command.Path)
	}
	for _, argument := range command.Args {
		_, possiblePath, hasEquals := strings.Cut(argument, "=")
		if hasEquals && filepath.IsAbs(possiblePath) {
			values = append(values, possiblePath)
		}
	}
	return values
}

func commandEnvironmentValues(environment []string) []string {
	values := transportProxyEnvironmentValues(environment)
	for _, item := range environment {
		key, value, ok := strings.Cut(item, "=")
		if !ok || value == "" {
			continue
		}
		if isTransportProxyEnvironmentKey(key) {
			continue
		} else if key == "GOPROXY" {
			for _, entry := range splitCandidateGoProxyEntries(value) {
				if strings.HasPrefix(entry, "https://") {
					values = append(values, entry)
				}
			}
		} else if environmentValueIsSensitive(key, value) {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return values
}

func transportProxyEnvironmentValues(environments ...[]string) []string {
	values := make([]string, 0, len(transportProxyEnvironmentKeys))
	seen := make(map[string]struct{}, len(transportProxyEnvironmentKeys))
	for _, environment := range environments {
		for _, item := range environment {
			key, value, ok := strings.Cut(item, "=")
			if !ok || value == "" || !isTransportProxyEnvironmentKey(key) {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return values
}

func isTransportProxyEnvironmentKey(key string) bool {
	for _, candidate := range transportProxyEnvironmentKeys {
		if key == candidate {
			return true
		}
	}
	return false
}

func environmentValueIsSensitive(key, value string) bool {
	upper := strings.ToUpper(key)
	for _, marker := range []string{"SECRET", "PASSWORD", "TOKEN", "CREDENTIAL", "PRIVATE", "API_KEY", "ACCESS_KEY", "AUTH"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	if filepath.IsAbs(value) {
		return true
	}
	return false
}
