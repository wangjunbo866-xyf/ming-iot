package thingsbaseline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type sourceFixture struct {
	repository     string
	lock           Lock
	lockedContent  string
	headContent    string
	lockedFilePath string
}

func TestReadSourceReadsFixedCommitInStablePathOrder(t *testing.T) {
	fixture := newSourceFixture(t, false)
	before := gitState(t, fixture.repository, fixture.lock.Source.Ref)
	if before.head == fixture.lock.Source.Commit {
		t.Fatal("fixture HEAD must differ from the locked commit")
	}
	if got, want := before.ref, fixture.lock.Source.Commit; got != want {
		t.Fatalf("fixture remote ref = %q, want locked commit %q", got, want)
	}

	snapshot, err := ReadSource(context.Background(), fixture.repository, &fixture.lock)
	if err != nil {
		t.Fatalf("ReadSource() error: %v", err)
	}
	if got, want := snapshot.Commit, fixture.lock.Source.Commit; got != want {
		t.Fatalf("snapshot commit = %q, want %q", got, want)
	}
	var gotPaths []string
	for _, file := range snapshot.Files {
		gotPaths = append(gotPaths, file.Path)
	}
	wantPaths := []string{"LICENSE", "go.mod", "src/a.txt", "src/z.txt"}
	if strings.Join(gotPaths, ",") != strings.Join(wantPaths, ",") {
		t.Fatalf("snapshot paths = %v, want %v", gotPaths, wantPaths)
	}
	if got, want := snapshot.SelectedBlobBytes, fixture.lock.Source.SelectedBlobBytes; got != want {
		t.Fatalf("selected blob bytes = %d, want %d", got, want)
	}
	contentChecked := false
	for _, file := range snapshot.Files {
		if file.Path != fixture.lockedFilePath {
			continue
		}
		if got, want := string(file.Data), fixture.lockedContent; got != want {
			t.Fatalf("locked blob content = %q, want %q", got, want)
		}
		if string(file.Data) == fixture.headContent {
			t.Fatalf("locked blob content unexpectedly equals HEAD content %q", fixture.headContent)
		}
		contentChecked = true
		break
	}
	if !contentChecked {
		t.Fatalf("locked source path %q not returned", fixture.lockedFilePath)
	}
	if after := gitState(t, fixture.repository, fixture.lock.Source.Ref); after != before {
		t.Fatalf("repository state changed during ReadSource(): before=%+v after=%+v", before, after)
	}
}

func TestSourceRepositoryStateIsReturnedAndCanBeRecaptured(t *testing.T) {
	fixture := newSourceFixture(t, false)
	snapshot, err := ReadSource(context.Background(), fixture.repository, &fixture.lock)
	if err != nil {
		t.Fatalf("ReadSource() error: %v", err)
	}
	if snapshot.RepositoryState.Head == "" || snapshot.RepositoryState.Ref != fixture.lock.Source.Commit || len(snapshot.RepositoryState.StatusSHA256) != 64 || len(snapshot.RepositoryState.IndexSHA256) != 64 {
		t.Fatalf("snapshot repository state = %+v", snapshot.RepositoryState)
	}
	recaptured, err := CaptureSourceRepositoryState(context.Background(), fixture.repository, &fixture.lock)
	if err != nil {
		t.Fatalf("CaptureSourceRepositoryState() error: %v", err)
	}
	if recaptured != snapshot.RepositoryState {
		t.Fatalf("recaptured state = %+v, want %+v", recaptured, snapshot.RepositoryState)
	}

	writeFixtureFile(t, fixture.repository, "untracked-drift.txt", "drift\n")
	drifted, err := CaptureSourceRepositoryState(context.Background(), fixture.repository, &fixture.lock)
	if err != nil {
		t.Fatalf("capture drifted source state: %v", err)
	}
	if drifted.StatusSHA256 == snapshot.RepositoryState.StatusSHA256 {
		t.Fatal("untracked worktree drift did not change status digest")
	}
}

func TestSourceRepositoryStateDetectsAssumeUnchangedIndexDrift(t *testing.T) {
	fixture := newSourceFixture(t, false)
	before, err := CaptureSourceRepositoryState(context.Background(), fixture.repository, &fixture.lock)
	if err != nil {
		t.Fatalf("capture source state before index drift: %v", err)
	}

	runGit(t, fixture.repository, "update-index", "--assume-unchanged", fixture.lockedFilePath)
	t.Cleanup(func() {
		runGit(t, fixture.repository, "update-index", "--no-assume-unchanged", fixture.lockedFilePath)
	})
	after, err := CaptureSourceRepositoryState(context.Background(), fixture.repository, &fixture.lock)
	if err != nil {
		t.Fatalf("capture source state after index drift: %v", err)
	}
	if before.Head != after.Head || before.Ref != after.Ref || before.StatusSHA256 != after.StatusSHA256 {
		t.Fatalf("assume-unchanged changed HEAD/ref/status: before=%+v after=%+v", before, after)
	}
	if before.IndexSHA256 == after.IndexSHA256 {
		t.Fatalf("assume-unchanged did not change index digest: before=%+v after=%+v", before, after)
	}
}

func TestSourceRepositoryStateRejectsUnsafeIndexFiles(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{
			name: "missing",
			mutate: func(t *testing.T, indexPath string) {
				if err := os.Remove(indexPath); err != nil {
					t.Fatalf("remove index: %v", err)
				}
			},
		},
		{
			name: "symlink",
			mutate: func(t *testing.T, indexPath string) {
				data, err := os.ReadFile(indexPath)
				if err != nil {
					t.Fatalf("read index: %v", err)
				}
				target := filepath.Join(t.TempDir(), "index-target")
				if err := os.WriteFile(target, data, 0o600); err != nil {
					t.Fatalf("write symlink target: %v", err)
				}
				if err := os.Remove(indexPath); err != nil {
					t.Fatalf("remove index: %v", err)
				}
				if err := os.Symlink(target, indexPath); err != nil {
					t.Fatalf("create index symlink: %v", err)
				}
			},
		},
		{
			name: "fifo",
			mutate: func(t *testing.T, indexPath string) {
				if err := os.Remove(indexPath); err != nil {
					t.Fatalf("remove index: %v", err)
				}
				if err := unix.Mkfifo(indexPath, 0o600); err != nil {
					t.Fatalf("create index fifo: %v", err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newSourceFixture(t, false)
			tt.mutate(t, sourceIndexPath(t, fixture.repository))
			if _, err := CaptureSourceRepositoryState(context.Background(), fixture.repository, &fixture.lock); err == nil || !strings.Contains(err.Error(), "source index") {
				t.Fatalf("CaptureSourceRepositoryState() error = %v, want source-index rejection", err)
			}
		})
	}
}

func TestSourceRepositoryStateRejectsIndexReplacementDuringRead(t *testing.T) {
	fixture := newSourceFixture(t, false)
	indexPath := sourceIndexPath(t, fixture.repository)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	info, err := os.Stat(indexPath)
	if err != nil {
		t.Fatalf("stat index: %v", err)
	}
	replaced := false
	_, err = readRepositoryIndexDigestWithHook(context.Background(), fixture.repository, newExecGitRunner(), func() error {
		replaced = true
		if err := os.Rename(indexPath, indexPath+".replaced"); err != nil {
			return err
		}
		return os.WriteFile(indexPath, data, info.Mode().Perm())
	})
	if !replaced {
		t.Fatal("index replacement hook was not called")
	}
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("readRepositoryIndexDigestWithHook() error = %v, want replacement rejection", err)
	}
}

func TestSourceRepositoryStateRejectsRefDriftAndCancellation(t *testing.T) {
	fixture := newSourceFixture(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CaptureSourceRepositoryState(ctx, fixture.repository, &fixture.lock); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled CaptureSourceRepositoryState() error = %v", err)
	}

	runGit(t, fixture.repository, "update-ref", fixture.lock.Source.Ref, "HEAD")
	if _, err := CaptureSourceRepositoryState(context.Background(), fixture.repository, &fixture.lock); err == nil || !strings.Contains(err.Error(), "source.ref") {
		t.Fatalf("ref-drift CaptureSourceRepositoryState() error = %v", err)
	}
}

func TestReadSourceRejectsRefDrift(t *testing.T) {
	fixture := newSourceFixture(t, false)
	writeFixtureFile(t, fixture.repository, "drift.txt", "drift\n")
	runGit(t, fixture.repository, "add", "drift.txt")
	runGit(t, fixture.repository, "commit", "-qm", "drift")
	drifted := strings.TrimSpace(string(runGit(t, fixture.repository, "rev-parse", "HEAD")))
	runGit(t, fixture.repository, "update-ref", fixture.lock.Source.Ref, drifted)

	_, err := ReadSource(context.Background(), fixture.repository, &fixture.lock)
	if err == nil || !strings.Contains(err.Error(), "source.ref") {
		t.Fatalf("ReadSource() error = %v, want source.ref drift failure", err)
	}
}

func TestReadSourceRejectsSymlinkTreeEntry(t *testing.T) {
	fixture := newSourceFixture(t, true)

	_, err := ReadSource(context.Background(), fixture.repository, &fixture.lock)
	if err == nil || !strings.Contains(err.Error(), "120000") {
		t.Fatalf("ReadSource() error = %v, want symlink mode failure", err)
	}
}

func TestReadSourceRejectsTrackedFileCountMismatch(t *testing.T) {
	fixture := newSourceFixture(t, false)
	fixture.lock.Source.TrackedFiles++

	_, err := ReadSource(context.Background(), fixture.repository, &fixture.lock)
	if err == nil || !strings.Contains(err.Error(), "source.trackedFiles") {
		t.Fatalf("ReadSource() error = %v, want tracked-files mismatch", err)
	}
}

func TestReadSourceUsesProvidedLocalRepository(t *testing.T) {
	fixture := newSourceFixture(t, false)
	fixture.lock.Source.Repository = "https://gitee.com/unitedrhino/things"

	snapshot, err := ReadSource(context.Background(), fixture.repository, &fixture.lock)
	if err != nil {
		t.Fatalf("ReadSource() error: %v", err)
	}
	if got := sourceFileData(t, snapshot, fixture.lockedFilePath); got != fixture.lockedContent {
		t.Fatalf("locked blob = %q, want %q", got, fixture.lockedContent)
	}
}

func TestReadSourceIgnoresGitReplaceObjects(t *testing.T) {
	fixture := newSourceFixture(t, false)
	head := strings.TrimSpace(string(runGit(t, fixture.repository, "rev-parse", "HEAD")))
	runGit(t, fixture.repository, "replace", fixture.lock.Source.Commit, head)
	t.Cleanup(func() { runGit(t, fixture.repository, "replace", "-d", fixture.lock.Source.Commit) })

	snapshot, err := ReadSource(context.Background(), fixture.repository, &fixture.lock)
	if err != nil {
		t.Fatalf("ReadSource() error: %v", err)
	}
	if got := sourceFileData(t, snapshot, fixture.lockedFilePath); got != fixture.lockedContent {
		t.Fatalf("locked blob = %q, want %q", got, fixture.lockedContent)
	}
}

func TestReadSourceRejectsOversizedBlobBeforeCatFile(t *testing.T) {
	fixture := newSourceFixture(t, false)
	fixture.lock.Security.MaxFileBytes = 1
	runner := &recordingGitRunner{base: newExecGitRunner()}

	_, err := readSourceWithRunner(context.Background(), fixture.repository, &fixture.lock, runner)
	if err == nil || !strings.Contains(err.Error(), "security.maxFileBytes") {
		t.Fatalf("readSourceWithRunner() error = %v, want oversized-file failure", err)
	}
	if runner.called("cat-file") {
		t.Fatalf("cat-file ran after oversized ls-tree entry: %v", runner.commands)
	}
}

func TestReadSourceDetectsDuringReadDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, fixture sourceFixture)
	}{
		{
			name: "ref",
			mutate: func(t *testing.T, fixture sourceFixture) {
				head := strings.TrimSpace(string(runGit(t, fixture.repository, "rev-parse", "HEAD")))
				runGit(t, fixture.repository, "update-ref", fixture.lock.Source.Ref, head)
			},
		},
		{
			name: "status",
			mutate: func(t *testing.T, fixture sourceFixture) {
				writeFixtureFile(t, fixture.repository, "during-read.txt", "drift\n")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newSourceFixture(t, false)
			runner := &recordingGitRunner{base: newExecGitRunner()}
			runner.afterOutput = func(args []string) {
				if args[0] == "ls-tree" && containsArg(args, "-l") {
					tt.mutate(t, fixture)
				}
			}

			_, err := readSourceWithRunner(context.Background(), fixture.repository, &fixture.lock, runner)
			if err == nil || !strings.Contains(err.Error(), "changed while reading") {
				t.Fatalf("readSourceWithRunner() error = %v, want drift failure", err)
			}
		})
	}
}

func TestReadSourceWrapsCanceledContext(t *testing.T) {
	fixture := newSourceFixture(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := readSourceWithRunner(ctx, fixture.repository, &fixture.lock, newExecGitRunner())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("readSourceWithRunner() error = %v, want context.Canceled", err)
	}
}

func TestReadSourceIgnoresHostileGitEnvironment(t *testing.T) {
	fixture := newSourceFixture(t, false)
	for key, value := range map[string]string{
		"GIT_DIR":                          os.DevNull,
		"GIT_WORK_TREE":                    t.TempDir(),
		"GIT_INDEX_FILE":                   filepath.Join(t.TempDir(), "index"),
		"GIT_OBJECT_DIRECTORY":             t.TempDir(),
		"GIT_CONFIG_COUNT":                 "1",
		"GIT_CONFIG_KEY_0":                 "core.abbrev",
		"GIT_CONFIG_VALUE_0":               "7",
		"GIT_NAMESPACE":                    "hostile",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": t.TempDir(),
	} {
		t.Setenv(key, value)
	}

	snapshot, err := ReadSource(context.Background(), fixture.repository, &fixture.lock)
	if err != nil {
		t.Fatalf("ReadSource() error: %v", err)
	}
	if got := sourceFileData(t, snapshot, fixture.lockedFilePath); got != fixture.lockedContent {
		t.Fatalf("locked blob = %q, want %q", got, fixture.lockedContent)
	}
}

func TestReadSourceDoesNotRefreshStaleIndex(t *testing.T) {
	fixture := newSourceFixture(t, false)
	indexPath := filepath.Join(fixture.repository, ".git", "index")
	before, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index before source read: %v", err)
	}
	trackedPath := filepath.Join(fixture.repository, filepath.FromSlash(fixture.lockedFilePath))
	if err := os.Chtimes(trackedPath, time.Now().Add(2*time.Second), time.Now().Add(2*time.Second)); err != nil {
		t.Fatalf("stale index stat cache: %v", err)
	}

	if _, err := ReadSource(context.Background(), fixture.repository, &fixture.lock); err != nil {
		t.Fatalf("ReadSource() error: %v", err)
	}
	after, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index after source read: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("ReadSource() refreshed the source repository index")
	}
}

func TestFixedGitEnvironmentStripsInheritedGitVariables(t *testing.T) {
	inherited := []string{
		"HOME=/preserved/home", "PATH=/preserved/bin", "APP_SETTING=preserved",
		"LC_ALL=hostile", "LANG=hostile",
		"GIT_DIR=/hostile/git", "GIT_WORK_TREE=/hostile/worktree", "GIT_COMMON_DIR=/hostile/common",
		"GIT_INDEX_FILE=/hostile/index", "GIT_OBJECT_DIRECTORY=/hostile/objects",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES=/hostile/alternates", "GIT_NAMESPACE=hostile",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_PARAMETERS=hostile", "GIT_CONFIG_KEY_0=core.abbrev",
		"GIT_CONFIG_VALUE_0=7", "GIT_TRACE=1", "GIT_NO_REPLACE_OBJECTS=0",
	}
	got := fixedGitEnvironment(inherited)
	values := environmentValues(got)
	for key, want := range map[string]string{
		"HOME":        "/preserved/home",
		"PATH":        "/preserved/bin",
		"APP_SETTING": "preserved",
		"LC_ALL":      "C",
		"LANG":        "C",
	} {
		if got := values[key]; got != want {
			t.Fatalf("environment %s = %q, want %q", key, got, want)
		}
	}
	allowedGit := []string{
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_LITERAL_PATHSPECS=1",
		"GIT_ATTR_NOSYSTEM=1",
	}
	var gotGit []string
	for _, item := range got {
		if strings.HasPrefix(item, "GIT_") {
			gotGit = append(gotGit, item)
		}
	}
	if strings.Join(gotGit, "\n") != strings.Join(allowedGit, "\n") {
		t.Fatalf("Git environment = %q, want %q", gotGit, allowedGit)
	}
}

func TestStreamArchiveDigestCancelsBlockingStreamAndWaitsOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := newBlockingReadCloser()
	defer stream.Close()
	runner := &blockingArchiveRunner{reader: stream}
	result := make(chan error, 1)
	go func() {
		_, _, err := streamArchiveDigest(ctx, "", runner, "archive", "--format=tar", "commit")
		result <- err
	}()
	select {
	case <-stream.started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("archive stream did not start")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("streamArchiveDigest() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("streamArchiveDigest() did not return after context cancellation")
	}
	if got := runner.waits.Load(); got != 1 {
		t.Fatalf("archive wait calls = %d, want 1", got)
	}
}

func TestBoundedBufferRetainsLastEightKiB(t *testing.T) {
	var buffer boundedBuffer
	first := bytes.Repeat([]byte("a"), 5000)
	second := bytes.Repeat([]byte("b"), 5000)
	if _, err := buffer.Write(first); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := buffer.Write(second); err != nil {
		t.Fatalf("second write: %v", err)
	}
	want := append(bytes.Repeat([]byte("a"), maxGitStderrBytes-len(second)), second...)
	if !bytes.Equal(buffer.data, want) {
		t.Fatalf("bounded buffer retained %d bytes, want final %d bytes", len(buffer.data), maxGitStderrBytes)
	}
}

func TestReadTreeEntriesRejectsMalformedAndUnsafePaths(t *testing.T) {
	lock := validLockForTest()
	lock.Source.Include = []string{"src"}
	lock.Source.Exclude = []string{"excluded"}
	objectID := strings.Repeat("a", 40)
	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{"malformed", []byte("100644 blob\tbad\x00"), "malformed metadata"},
		{"unterminated", []byte("100644 blob " + objectID + " 1\tsrc/a.txt"), "unterminated"},
		{"dash size", []byte("100644 blob " + objectID + " -\tsrc/a.txt\x00"), "invalid blob size"},
		{"backslash", treeEntry(objectID, "src\\a.txt"), "unsafe source path"},
		{"traversal", treeEntry(objectID, "src/../a.txt"), "unsafe source path"},
		{"tab", treeEntry(objectID, "src/a\tb.txt"), "unsafe source path"},
		{"newline", treeEntry(objectID, "src/a\nb.txt"), "unsafe source path"},
		{"control", treeEntry(objectID, "src/a\x01b.txt"), "unsafe source path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := readTreeEntries(context.Background(), "", panicGitRunner{}, tt.raw, &lock)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("readTreeEntries() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestReadTreeEntriesVerifiesBlobObjectID(t *testing.T) {
	lock := validLockForTest()
	lock.Source.Include = []string{"src"}
	lock.Source.Exclude = []string{"excluded"}
	lock.Security.MaxFileBytes = 1024
	runner := fakeGitRunner{outputData: []byte("data")}

	_, _, err := readTreeEntries(context.Background(), "", runner, treeEntry(strings.Repeat("a", 40), "src/a.txt"), &lock)
	if err == nil || !strings.Contains(err.Error(), "object ID mismatch") {
		t.Fatalf("readTreeEntries() error = %v, want object-ID mismatch", err)
	}
	if strings.Contains(err.Error(), "data") {
		t.Fatalf("object-ID error leaked blob content: %v", err)
	}
}

func newSourceFixture(t *testing.T, withSymlink bool) sourceFixture {
	t.Helper()
	const lockedFilePath = "src/a.txt"
	const lockedContent = "locked-content\n"
	const headContent = "head-content\n"
	repository := t.TempDir()
	runGit(t, repository, "init", "-q")
	runGit(t, repository, "config", "user.email", "thingsbaseline@example.invalid")
	runGit(t, repository, "config", "user.name", "Things Baseline Test")
	writeFixtureFile(t, repository, "LICENSE", "license\n")
	writeFixtureFile(t, repository, "go.mod", "module example.test/things\n")
	writeFixtureFile(t, repository, "src/z.txt", "z\n")
	writeFixtureFile(t, repository, lockedFilePath, lockedContent)
	if withSymlink {
		if err := os.Symlink("a.txt", filepath.Join(repository, "src", "link.txt")); err != nil {
			t.Fatalf("create symlink: %v", err)
		}
	}
	runGit(t, repository, "add", ".")
	runGit(t, repository, "commit", "-qm", "locked fixture")
	commit := strings.TrimSpace(string(runGit(t, repository, "rev-parse", "HEAD")))
	runGit(t, repository, "update-ref", "refs/remotes/origin/master", commit)

	paths := []string{"LICENSE", "go.mod", "src/a.txt", "src/z.txt"}
	if withSymlink {
		paths = append(paths, "src/link.txt")
	}
	var selectedBytes int64
	for _, name := range paths {
		info, err := os.Lstat(filepath.Join(repository, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		selectedBytes += info.Size()
	}
	archive := runGit(t, repository, append([]string{"archive", "--format=tar", commit, "--"}, []string{"LICENSE", "go.mod", "src"}...)...)
	archiveSHA := sha256.Sum256(archive)
	tree := strings.TrimSpace(string(runGit(t, repository, "rev-parse", commit+"^{tree}")))
	lock := validLockForTest()
	lock.Source.Repository = repository
	lock.Source.Commit = commit
	lock.Source.Tree = tree
	lock.Source.TrackedFiles = len(paths)
	lock.Source.SelectedFiles = len(paths)
	lock.Source.SelectedBlobBytes = selectedBytes
	lock.Source.ArchiveBytes = int64(len(archive))
	lock.Source.ArchiveSHA256 = fmt.Sprintf("%x", archiveSHA)
	lock.Source.Include = []string{"LICENSE", "go.mod", "src"}
	lock.Source.Exclude = []string{"excluded"}
	lock.Security.MaxFileBytes = 1024

	writeFixtureFile(t, repository, lockedFilePath, headContent)
	runGit(t, repository, "add", lockedFilePath)
	runGit(t, repository, "commit", "-qm", "HEAD differs from locked commit")
	return sourceFixture{
		repository:     repository,
		lock:           lock,
		lockedContent:  lockedContent,
		headContent:    headContent,
		lockedFilePath: lockedFilePath,
	}
}

type repositoryState struct {
	head      string
	ref       string
	statusSHA [32]byte
}

func gitState(t *testing.T, repository, ref string) repositoryState {
	t.Helper()
	status := runGit(t, repository, "status", "--porcelain=v1", "-z")
	return repositoryState{
		head:      strings.TrimSpace(string(runGit(t, repository, "rev-parse", "HEAD"))),
		ref:       strings.TrimSpace(string(runGit(t, repository, "rev-parse", ref))),
		statusSHA: sha256.Sum256(status),
	}
}

func sourceIndexPath(t *testing.T, repository string) string {
	t.Helper()
	raw := strings.TrimSpace(string(runGit(t, repository, "rev-parse", "--git-path", "index")))
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	return filepath.Clean(filepath.Join(repository, raw))
}

func sourceFileData(t *testing.T, snapshot SourceSnapshot, path string) string {
	t.Helper()
	for _, file := range snapshot.Files {
		if file.Path == path {
			return string(file.Data)
		}
	}
	t.Fatalf("source snapshot does not contain %q", path)
	return ""
}

func treeEntry(objectID, name string) []byte {
	return []byte("100644 blob " + objectID + " 4\t" + name + "\x00")
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

type recordingGitRunner struct {
	base        gitRunner
	commands    [][]string
	afterOutput func([]string)
}

func (r *recordingGitRunner) output(ctx context.Context, repository string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, append([]string(nil), args...))
	output, err := r.base.output(ctx, repository, args...)
	if err == nil && r.afterOutput != nil {
		r.afterOutput(args)
	}
	return output, err
}

func (r *recordingGitRunner) stream(ctx context.Context, repository string, args ...string) (io.ReadCloser, func() error, error) {
	r.commands = append(r.commands, append([]string(nil), args...))
	return r.base.stream(ctx, repository, args...)
}

func (r *recordingGitRunner) called(want string) bool {
	for _, command := range r.commands {
		if len(command) > 0 && command[0] == want {
			return true
		}
	}
	return false
}

type panicGitRunner struct{}

func (panicGitRunner) output(context.Context, string, ...string) ([]byte, error) {
	panic("unexpected Git command")
}

func (panicGitRunner) stream(context.Context, string, ...string) (io.ReadCloser, func() error, error) {
	panic("unexpected Git stream")
}

type fakeGitRunner struct {
	outputData []byte
}

func (r fakeGitRunner) output(context.Context, string, ...string) ([]byte, error) {
	return r.outputData, nil
}

func (r fakeGitRunner) stream(context.Context, string, ...string) (io.ReadCloser, func() error, error) {
	return io.NopCloser(bytes.NewReader(nil)), func() error { return nil }, nil
}

type blockingArchiveRunner struct {
	reader *blockingReadCloser
	waits  atomic.Int32
}

func (*blockingArchiveRunner) output(context.Context, string, ...string) ([]byte, error) {
	panic("unexpected Git command")
}

func (r *blockingArchiveRunner) stream(context.Context, string, ...string) (io.ReadCloser, func() error, error) {
	return r.reader, func() error {
		r.waits.Add(1)
		return nil
	}, nil
}

type blockingReadCloser struct {
	started   chan struct{}
	closed    chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
}

func newBlockingReadCloser() *blockingReadCloser {
	return &blockingReadCloser{started: make(chan struct{}), closed: make(chan struct{})}
}

func (r *blockingReadCloser) Read([]byte) (int, error) {
	r.startOnce.Do(func() { close(r.started) })
	<-r.closed
	return 0, context.Canceled
}

func (r *blockingReadCloser) Close() error {
	r.closeOnce.Do(func() { close(r.closed) })
	return nil
}

func environmentValues(environment []string) map[string]string {
	values := make(map[string]string, len(environment))
	for _, item := range environment {
		key, value, found := strings.Cut(item, "=")
		if found {
			values[key] = value
		}
	}
	return values
}

func writeFixtureFile(t *testing.T, repository, name, content string) {
	t.Helper()
	filename := filepath.Join(repository, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func runGit(t *testing.T, repository string, args ...string) []byte {
	t.Helper()
	command := append([]string{
		"-c", "commit.gpgSign=false",
		"-c", "tag.gpgSign=false",
		"-c", "core.hooksPath=/dev/null",
		"-C", repository,
	}, args...)
	cmd := exec.Command("git", command...)
	cmd.Env = fixedGitEnvironment(os.Environ())
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(command, " "), err, output)
	}
	return output
}
