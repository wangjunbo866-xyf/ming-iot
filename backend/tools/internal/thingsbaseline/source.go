package thingsbaseline

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const maxGitStderrBytes = 8 << 10
const maxGitIndexBytes = int64(256 << 20)

// SourceFile is one regular file read directly from a locked Git blob.
type SourceFile struct {
	Path     string
	Mode     string
	ObjectID string
	Data     []byte
}

// SourceSnapshot is the validated, stable source selection for later tasks.
type SourceSnapshot struct {
	Commit            string
	Tree              string
	Files             []SourceFile
	SelectedBlobBytes int64
	RepositoryState   SourceRepositoryState
}

// SourceRepositoryState is best-effort drift evidence. It intentionally
// contains no repository path, status body, or worktree file names.
type SourceRepositoryState struct {
	Head         string `json:"head"`
	Ref          string `json:"ref"`
	StatusSHA256 string `json:"statusSha256"`
	IndexSHA256  string `json:"indexSha256"`
}

// ReadSource reads exactly the locked source selection from repository. It
// never reads files from the repository worktree.
func ReadSource(ctx context.Context, repository string, lock *Lock) (SourceSnapshot, error) {
	return readSourceWithRunner(ctx, repository, lock, newExecGitRunner())
}

// CaptureSourceRepositoryState reuses the source reader's hardened Git
// boundary so final verification does not grow a second Git implementation.
func CaptureSourceRepositoryState(ctx context.Context, repository string, lock *Lock) (SourceRepositoryState, error) {
	if err := ValidateLock(lock); err != nil {
		return SourceRepositoryState{}, fmt.Errorf("validate source lock: %w", err)
	}
	if err := contextError(ctx, "capture source repository state"); err != nil {
		return SourceRepositoryState{}, err
	}
	state, err := readRepositoryState(ctx, repository, lock.Source.Ref, newExecGitRunner())
	if err != nil {
		return SourceRepositoryState{}, err
	}
	if state.ref != lock.Source.Commit {
		return SourceRepositoryState{}, fmt.Errorf("source.ref %q resolved to %q, want locked commit %q", lock.Source.Ref, state.ref, lock.Source.Commit)
	}
	return exportSourceRepositoryState(state), nil
}

// gitRunner permits narrow test-time observation of Git commands without
// widening the source-reader API or adding a production runner package.
type gitRunner interface {
	output(context.Context, string, ...string) ([]byte, error)
	stream(context.Context, string, ...string) (io.ReadCloser, func() error, error)
}

func newExecGitRunner() gitRunner { return execGitRunner{} }

type execGitRunner struct{}

func (execGitRunner) output(ctx context.Context, repository string, args ...string) ([]byte, error) {
	command := newGitCommand(ctx, repository, args...)
	var stdout bytes.Buffer
	var stderr boundedBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, gitCommandError(ctx, args, err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func (execGitRunner) stream(ctx context.Context, repository string, args ...string) (io.ReadCloser, func() error, error) {
	command := newGitCommand(ctx, repository, args...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("prepare git %s stdout: %w", strings.Join(args, " "), err)
	}
	var stderr boundedBuffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return nil, nil, gitCommandError(ctx, args, err, stderr.String())
	}
	return stdout, func() error {
		if err := command.Wait(); err != nil {
			return gitCommandError(ctx, args, err, stderr.String())
		}
		return nil
	}, nil
}

// ReadSource's state snapshots are best-effort evidence of ref/worktree
// drift; the fixed commit/tree, archive hash, and every blob OID establish
// the integrity of the returned output. The reader deliberately takes no
// write lock on the source repository.
func readSourceWithRunner(ctx context.Context, repository string, lock *Lock, runner gitRunner) (SourceSnapshot, error) {
	if err := ValidateLock(lock); err != nil {
		return SourceSnapshot{}, fmt.Errorf("validate source lock: %w", err)
	}
	if err := contextError(ctx, "start source read"); err != nil {
		return SourceSnapshot{}, err
	}
	before, err := readRepositoryState(ctx, repository, lock.Source.Ref, runner)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if before.ref != lock.Source.Commit {
		return SourceSnapshot{}, fmt.Errorf("source.ref %q resolved to %q, want locked commit %q", lock.Source.Ref, before.ref, lock.Source.Commit)
	}

	tree, err := gitText(ctx, repository, runner, "rev-parse", lock.Source.Commit+"^{tree}")
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("resolve source tree: %w", err)
	}
	if tree != lock.Source.Tree {
		return SourceSnapshot{}, fmt.Errorf("source.tree for commit %q = %q, want %q", lock.Source.Commit, tree, lock.Source.Tree)
	}

	allNames, err := runner.output(ctx, repository, "ls-tree", "-rz", "-r", "--full-tree", "--name-only", lock.Source.Commit)
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("list source tracked files: %w", err)
	}
	trackedFiles, err := countNULRecords(allNames)
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("parse source tracked files: %w", err)
	}
	if trackedFiles != lock.Source.TrackedFiles {
		return SourceSnapshot{}, fmt.Errorf("source.trackedFiles = %d, want %d", trackedFiles, lock.Source.TrackedFiles)
	}

	args := append([]string{"ls-tree", "-rz", "-l", lock.Source.Commit, "--"}, lock.Source.Include...)
	entries, err := runner.output(ctx, repository, args...)
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("list locked source files: %w", err)
	}
	files, selectedBytes, err := readTreeEntries(ctx, repository, runner, entries, lock)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if len(files) != lock.Source.SelectedFiles {
		return SourceSnapshot{}, fmt.Errorf("source.selectedFiles = %d, want %d", len(files), lock.Source.SelectedFiles)
	}
	if selectedBytes != lock.Source.SelectedBlobBytes {
		return SourceSnapshot{}, fmt.Errorf("source.selectedBlobBytes = %d, want %d", selectedBytes, lock.Source.SelectedBlobBytes)
	}

	archiveArgs := append([]string{"archive", "--format=tar", lock.Source.Commit, "--"}, lock.Source.Include...)
	archiveBytes, archiveSHA, err := streamArchiveDigest(ctx, repository, runner, archiveArgs...)
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("archive locked source files: %w", err)
	}
	if archiveBytes != lock.Source.ArchiveBytes {
		return SourceSnapshot{}, fmt.Errorf("source.archiveBytes = %d, want %d", archiveBytes, lock.Source.ArchiveBytes)
	}
	if archiveSHA != lock.Source.ArchiveSHA256 {
		return SourceSnapshot{}, fmt.Errorf("source.archiveSha256 = %s, want %s", archiveSHA, lock.Source.ArchiveSHA256)
	}

	after, err := readRepositoryState(ctx, repository, lock.Source.Ref, runner)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if after != before {
		return SourceSnapshot{}, fmt.Errorf("source repository changed while reading locked objects")
	}

	return SourceSnapshot{
		Commit:            lock.Source.Commit,
		Tree:              tree,
		Files:             files,
		SelectedBlobBytes: selectedBytes,
		RepositoryState:   exportSourceRepositoryState(before),
	}, nil
}

func readTreeEntries(ctx context.Context, repository string, runner gitRunner, raw []byte, lock *Lock) ([]SourceFile, int64, error) {
	var files []SourceFile
	var total int64
	seen := make(map[string]struct{})
	for len(raw) > 0 {
		end := bytes.IndexByte(raw, 0)
		if end < 0 {
			return nil, 0, fmt.Errorf("parse git ls-tree output: unterminated entry")
		}
		entry := raw[:end]
		raw = raw[end+1:]
		file, size, err := parseTreeEntry(entry)
		if err != nil {
			return nil, 0, err
		}
		if err := validateGitPath("source", file.Path); err != nil {
			return nil, 0, err
		}
		if file.Mode != "100644" && file.Mode != "100755" {
			return nil, 0, fmt.Errorf("source path %q has forbidden Git mode %s", file.Path, file.Mode)
		}
		if !pathIsSelected(file.Path, lock.Source.Include, lock.Source.Exclude) {
			return nil, 0, fmt.Errorf("source path %q is outside locked include/exclude roots", file.Path)
		}
		if size > lock.Security.MaxFileBytes {
			return nil, 0, fmt.Errorf("source path %q size %d exceeds security.maxFileBytes %d", file.Path, size, lock.Security.MaxFileBytes)
		}
		if _, ok := seen[file.Path]; ok {
			return nil, 0, fmt.Errorf("source path %q appeared more than once", file.Path)
		}
		seen[file.Path] = struct{}{}
		data, err := runner.output(ctx, repository, "cat-file", "blob", file.ObjectID)
		if err != nil {
			return nil, 0, fmt.Errorf("read blob for source path %q: %w", file.Path, err)
		}
		if int64(len(data)) != size {
			return nil, 0, fmt.Errorf("source blob %s size = %d, ls-tree reported %d", file.ObjectID, len(data), size)
		}
		if err := verifyBlobObjectID(file.ObjectID, data); err != nil {
			return nil, 0, fmt.Errorf("source path %q: %w", file.Path, err)
		}
		file.Data = data
		files = append(files, file)
		total += size
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, total, nil
}

func parseTreeEntry(entry []byte) (SourceFile, int64, error) {
	parts := bytes.SplitN(entry, []byte{'\t'}, 2)
	if len(parts) != 2 || len(parts[1]) == 0 {
		return SourceFile{}, 0, fmt.Errorf("parse git ls-tree output: malformed entry")
	}
	fields := bytes.Fields(parts[0])
	if len(fields) != 4 || string(fields[1]) != "blob" || !isLowercaseHex(string(fields[2]), 40) {
		return SourceFile{}, 0, fmt.Errorf("parse git ls-tree output: malformed metadata")
	}
	size, err := strconv.ParseInt(string(fields[3]), 10, 64)
	if err != nil || size < 0 {
		return SourceFile{}, 0, fmt.Errorf("parse git ls-tree output: invalid blob size")
	}
	return SourceFile{Path: string(parts[1]), Mode: string(fields[0]), ObjectID: string(fields[2])}, size, nil
}

func verifyBlobObjectID(objectID string, data []byte) error {
	hash := sha1.New()
	_, _ = io.WriteString(hash, "blob ")
	_, _ = io.WriteString(hash, strconv.Itoa(len(data)))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(data)
	if actual := hex.EncodeToString(hash.Sum(nil)); actual != objectID {
		return fmt.Errorf("blob object ID mismatch: got %s, want %s", actual, objectID)
	}
	return nil
}

func streamArchiveDigest(ctx context.Context, repository string, runner gitRunner, args ...string) (int64, string, error) {
	stream, wait, err := runner.stream(ctx, repository, args...)
	if err != nil {
		if ctx.Err() != nil {
			return 0, "", fmt.Errorf("stream archive: %w", ctx.Err())
		}
		return 0, "", err
	}
	defer stream.Close()
	stopCancel := make(chan struct{})
	cancelStopped := make(chan struct{})
	go func() {
		defer close(cancelStopped)
		select {
		case <-ctx.Done():
			_ = stream.Close()
		case <-stopCancel:
		}
	}()
	hash := sha256.New()
	bytesRead, copyErr := io.Copy(hash, stream)
	close(stopCancel)
	<-cancelStopped
	waitErr := wait()
	if ctx.Err() != nil {
		return 0, "", fmt.Errorf("stream archive: %w", ctx.Err())
	}
	if copyErr != nil {
		return 0, "", fmt.Errorf("read archive stdout: %w", copyErr)
	}
	if waitErr != nil {
		return 0, "", waitErr
	}
	return bytesRead, hex.EncodeToString(hash.Sum(nil)), nil
}

func countNULRecords(raw []byte) (int, error) {
	count := 0
	for len(raw) > 0 {
		end := bytes.IndexByte(raw, 0)
		if end < 0 {
			return 0, fmt.Errorf("unterminated entry")
		}
		if end == 0 {
			return 0, fmt.Errorf("empty entry")
		}
		count++
		raw = raw[end+1:]
	}
	return count, nil
}

func pathIsSelected(name string, includes, excludes []string) bool {
	included := false
	for _, include := range includes {
		if name == include || strings.HasPrefix(name, include+"/") {
			included = true
			break
		}
	}
	if !included {
		return false
	}
	for _, exclude := range excludes {
		if name == exclude || strings.HasPrefix(name, exclude+"/") {
			return false
		}
	}
	return true
}

type sourceRepositoryState struct {
	head      string
	ref       string
	statusSHA [sha256.Size]byte
	indexSHA  [sha256.Size]byte
}

func readRepositoryState(ctx context.Context, repository, ref string, runner gitRunner) (sourceRepositoryState, error) {
	head, err := gitText(ctx, repository, runner, "rev-parse", "HEAD")
	if err != nil {
		return sourceRepositoryState{}, fmt.Errorf("resolve source HEAD: %w", err)
	}
	if !isLowercaseHex(head, 40) {
		return sourceRepositoryState{}, fmt.Errorf("source HEAD did not resolve to a 40 lowercase hex object ID")
	}
	resolvedRef, err := gitText(ctx, repository, runner, "show-ref", "--verify", "--hash", ref)
	if err != nil {
		return sourceRepositoryState{}, fmt.Errorf("resolve source.ref %q: %w", ref, err)
	}
	if !isLowercaseHex(resolvedRef, 40) {
		return sourceRepositoryState{}, fmt.Errorf("source.ref %q did not resolve to a 40 lowercase hex object ID", ref)
	}
	indexSHA, err := readRepositoryIndexDigest(ctx, repository, runner)
	if err != nil {
		return sourceRepositoryState{}, err
	}
	status, err := runner.output(ctx, repository, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return sourceRepositoryState{}, fmt.Errorf("read source status: %w", err)
	}
	return sourceRepositoryState{head: head, ref: resolvedRef, statusSHA: sha256.Sum256(status), indexSHA: indexSHA}, nil
}

func exportSourceRepositoryState(state sourceRepositoryState) SourceRepositoryState {
	return SourceRepositoryState{
		Head:         state.head,
		Ref:          state.ref,
		StatusSHA256: hex.EncodeToString(state.statusSHA[:]),
		IndexSHA256:  hex.EncodeToString(state.indexSHA[:]),
	}
}

func readRepositoryIndexDigest(ctx context.Context, repository string, runner gitRunner) ([sha256.Size]byte, error) {
	return readRepositoryIndexDigestWithHook(ctx, repository, runner, nil)
}

// readRepositoryIndexDigestWithHook exists so tests can deterministically
// replace the pathname after it has been opened. Production always passes a
// nil hook.
func readRepositoryIndexDigestWithHook(ctx context.Context, repository string, runner gitRunner, afterOpen func() error) ([sha256.Size]byte, error) {
	var empty [sha256.Size]byte
	if err := contextError(ctx, "read source index"); err != nil {
		return empty, err
	}
	rawPath, err := runner.output(ctx, repository, "rev-parse", "--git-path", "index")
	if err != nil {
		return empty, fmt.Errorf("resolve source index: %w", err)
	}
	indexPath, err := parseRepositoryIndexPath(repository, rawPath)
	if err != nil {
		return empty, err
	}
	parentFD, err := unix.Open(filepath.Dir(indexPath), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return empty, fmt.Errorf("open source index parent: %w", err)
	}
	defer unix.Close(parentFD)
	name := filepath.Base(indexPath)
	var namedBefore unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &namedBefore, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return empty, fmt.Errorf("inspect source index: %w", err)
	}
	if err := validateRepositoryIndexStat(namedBefore); err != nil {
		return empty, err
	}
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return empty, fmt.Errorf("open source index without following links: %w", err)
	}
	file := os.NewFile(uintptr(fd), "source-index")
	if file == nil {
		_ = unix.Close(fd)
		return empty, fmt.Errorf("wrap source index file descriptor")
	}
	defer file.Close()
	var opened unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil || !sameRepositoryIndexState(namedBefore, opened) {
		return empty, fmt.Errorf("source index identity or metadata changed while opening")
	}
	if afterOpen != nil {
		if err := afterOpen(); err != nil {
			return empty, fmt.Errorf("source index test hook: %w", err)
		}
	}
	digest, err := hashRetainedFile(ctx, file, opened.Size, maxGitIndexBytes)
	if err != nil {
		return empty, fmt.Errorf("hash source index: %w", err)
	}
	var afterFD, namedAfter unix.Stat_t
	if err := unix.Fstat(fd, &afterFD); err != nil ||
		unix.Fstatat(parentFD, name, &namedAfter, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		!sameRepositoryIndexState(opened, afterFD) || !sameRepositoryIndexState(afterFD, namedAfter) {
		return empty, fmt.Errorf("source index identity or metadata changed during read")
	}
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != sha256.Size {
		return empty, fmt.Errorf("source index digest is invalid")
	}
	copy(empty[:], decoded)
	return empty, nil
}

func parseRepositoryIndexPath(repository string, raw []byte) (string, error) {
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		raw = raw[:len(raw)-1]
	}
	value := string(raw)
	if value == "" || !utf8.ValidString(value) || containsASCIIControl(value) || filepath.Base(value) == "." || filepath.Base(value) == ".." || filepath.Clean(value) != value {
		return "", fmt.Errorf("source index path returned by Git is unsafe")
	}
	if !filepath.IsAbs(value) {
		absoluteRepository, err := filepath.Abs(repository)
		if err != nil {
			return "", fmt.Errorf("resolve source repository path for index")
		}
		value = filepath.Join(absoluteRepository, value)
	}
	value = filepath.Clean(value)
	if !filepath.IsAbs(value) || value == string(filepath.Separator) {
		return "", fmt.Errorf("source index path returned by Git is unsafe")
	}
	return value, nil
}

func validateRepositoryIndexStat(state unix.Stat_t) error {
	if state.Mode&unix.S_IFMT != unix.S_IFREG || state.Nlink != 1 || state.Size <= 0 || state.Size > maxGitIndexBytes {
		return fmt.Errorf("source index is not a bounded single-link regular file")
	}
	return nil
}

func sameRepositoryIndexState(left, right unix.Stat_t) bool {
	return sameFileIdentity(left, right) && left.Mode == right.Mode && left.Size == right.Size && left.Nlink == right.Nlink
}

func gitText(ctx context.Context, repository string, runner gitRunner, args ...string) (string, error) {
	output, err := runner.output(ctx, repository, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func newGitCommand(ctx context.Context, repository string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = repository
	command.Env = fixedGitEnvironment(os.Environ())
	return command
}

func fixedGitEnvironment(inherited []string) []string {
	gitOverrides := []string{
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_LITERAL_PATHSPECS=1",
		"GIT_ATTR_NOSYSTEM=1",
	}
	environment := make([]string, 0, len(inherited)+len(gitOverrides)+2)
	for _, item := range inherited {
		key, _, found := strings.Cut(item, "=")
		if !found {
			environment = append(environment, item)
			continue
		}
		if strings.HasPrefix(key, "GIT_") || key == "LC_ALL" || key == "LANG" {
			continue
		}
		environment = append(environment, item)
	}
	environment = append(environment, "LC_ALL=C", "LANG=C")
	environment = append(environment, gitOverrides...)
	return environment
}

func contextError(ctx context.Context, operation string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func gitCommandError(ctx context.Context, args []string, err error, stderr string) error {
	if ctx.Err() != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), ctx.Err())
	}
	if stderr != "" {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr)
	}
	return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
}

type boundedBuffer struct {
	data []byte
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	originalLength := len(data)
	if len(data) >= maxGitStderrBytes {
		b.data = append(b.data[:0], data[len(data)-maxGitStderrBytes:]...)
		return originalLength, nil
	}
	if overflow := len(b.data) + len(data) - maxGitStderrBytes; overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
	}
	b.data = append(b.data, data...)
	return originalLength, nil
}

func (b *boundedBuffer) String() string { return strings.TrimSpace(string(b.data)) }
