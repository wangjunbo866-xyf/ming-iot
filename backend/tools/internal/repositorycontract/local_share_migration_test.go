package repositorycontract

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

const legacyShareModulePath = "gitee.com/" + "unitedrhino/share"

// localShareProjectOwnedExtensions 列出本项目新增或明确维护的精确扩展；不改变保留的上游署名。
var localShareProjectOwnedExtensions = []string{
	"contracts/eventbus",
	"eventBus/kafka.go",
	"eventBus/kafka_test.go",
	"caches/cache.go",
	"caches/recovery.go",
	"caches/recovery_test.go",
	"contracts/kafka",
	"foundation/kafka",
	"go.mod",
	"go.sum",
}

func TestLocalShareModulePath(t *testing.T) {
	repoRoot := localShareRepositoryRoot(t)
	path := "backend/share/go.mod"
	content := localShareReadFile(t, repoRoot, path)

	parsed, err := modfile.Parse(path, content, nil)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if parsed.Module == nil {
		t.Fatalf("%s has no module directive", path)
	}

	const want = "gitee.com/ming-iot/share"
	if got := parsed.Module.Mod.Path; got != want {
		t.Errorf("%s module path = %q, want %q", path, got, want)
	}
}

func TestLocalShareSourceMetadata(t *testing.T) {
	repoRoot := localShareRepositoryRoot(t)
	path := "backend/share/SOURCE.json"
	content := localShareReadFile(t, repoRoot, path)

	type sourceMetadata struct {
		SchemaVersion              string `json:"schema_version"`
		SourceRepository           string `json:"source_repository"`
		SourceTag                  string `json:"source_tag"`
		Commit                     string `json:"commit"`
		Tree                       string `json:"tree"`
		TrackedFileCount           int    `json:"tracked_file_count"`
		ArchiveSHA256              string `json:"archive_sha256"`
		ArchiveFormat              string `json:"archive_format"`
		ArchiveCommand             string `json:"archive_command"`
		NormalizedTreeSHA256       string `json:"normalized_tree_sha256"`
		License                    string `json:"license"`
		AuthorizationRoute         string `json:"authorization_route"`
		AuthorizationDate          string `json:"authorization_date"`
		ThirdPartyNoticesPreserved bool   `json:"third_party_notices_preserved"`
	}
	var source sourceMetadata
	if err := json.Unmarshal(content, &source); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	want := sourceMetadata{
		SchemaVersion:              "ming_iot_local_share_source_v1",
		SourceRepository:           "gitee:unitedrhino/share",
		SourceTag:                  "v1.6.4",
		Commit:                     "ece02ca9d6defd63356931a59e653950f27ac572",
		Tree:                       "21bdb52ed508152cf440b8d8a3b3ae3943a4b84a",
		TrackedFileCount:           255,
		ArchiveSHA256:              "8c90b2b3853ad86b422f9929d801d617645bac3dbf68f600e3736b3a32505acf",
		ArchiveFormat:              "git-archive-tar",
		ArchiveCommand:             "git archive --format=tar v1.6.4",
		NormalizedTreeSHA256:       "edc6dfc450f14d4602b118a2f676fcee78b63a8f0c597cfafd39be9db4e4cfc0",
		License:                    "Apache-2.0",
		AuthorizationRoute:         "rights_holder_or_authorized_company_owner",
		AuthorizationDate:          "2026-08-27",
		ThirdPartyNoticesPreserved: true,
	}
	fields := []struct {
		name string
		got  any
		want any
	}{
		{name: "schema_version", got: source.SchemaVersion, want: want.SchemaVersion},
		{name: "source_repository", got: source.SourceRepository, want: want.SourceRepository},
		{name: "source_tag", got: source.SourceTag, want: want.SourceTag},
		{name: "commit", got: source.Commit, want: want.Commit},
		{name: "tree", got: source.Tree, want: want.Tree},
		{name: "tracked_file_count", got: source.TrackedFileCount, want: want.TrackedFileCount},
		{name: "archive_sha256", got: source.ArchiveSHA256, want: want.ArchiveSHA256},
		{name: "archive_format", got: source.ArchiveFormat, want: want.ArchiveFormat},
		{name: "archive_command", got: source.ArchiveCommand, want: want.ArchiveCommand},
		{name: "normalized_tree_sha256", got: source.NormalizedTreeSHA256, want: want.NormalizedTreeSHA256},
		{name: "license", got: source.License, want: want.License},
		{name: "authorization_route", got: source.AuthorizationRoute, want: want.AuthorizationRoute},
		{name: "authorization_date", got: source.AuthorizationDate, want: want.AuthorizationDate},
		{name: "third_party_notices_preserved", got: source.ThirdPartyNoticesPreserved, want: want.ThirdPartyNoticesPreserved},
	}
	for _, field := range fields {
		if !reflect.DeepEqual(field.got, field.want) {
			t.Errorf("%s %s = %#v, want %#v", path, field.name, field.got, field.want)
		}
	}
}

func TestLocalShareUpstreamOwnedSourceRemainder(t *testing.T) {
	repoRoot := localShareRepositoryRoot(t)
	count, got := localShareNormalizedTreeDigest(t, filepath.Join(repoRoot, "backend/share"))

	if count != 252 {
		t.Errorf("upstream-owned Share source file count = %d, want 252", count)
	}
	// M4 removes embedded example credentials in two files; exact transforms are
	// recorded in tooling/policy/PUBLICATION_TRANSFORMS.json. Keep the full remainder
	// frozen, including those files, rather than adding directory exclusions.
	// P1 只将获批修改的 caches/cache.go 移入精确项目维护清单；此摘要由
	// 不可变基线 6a59904edbf7d019e266fe910fb1436d3b1ee4c4 的其余252个文件计算，
	// 未从待验收工作树重新生成；新增 bus 文件单列，不排除整个 caches/eventBus 目录。
	const want = "50721d9737e2236349b89335806ba8ed6254393857148488970a622ba85dec62"
	if got != want {
		t.Errorf("upstream-owned Share source remainder digest = %s, want %s", got, want)
	}
}

func TestLocalShareNormalizedTreeDigestUsesGitFileModes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "fixture.go")
	if err := os.WriteFile(path, []byte("package fixture\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, regularDigest := localShareNormalizedTreeDigest(t, root)
	if err := os.Chmod(path, 0o664); err != nil {
		t.Fatalf("chmod fixture to 0664: %v", err)
	}
	_, groupWritableDigest := localShareNormalizedTreeDigest(t, root)
	if groupWritableDigest != regularDigest {
		t.Fatalf("non-executable Git file digest changed with worktree permission bits: 0644=%s, 0664=%s", regularDigest, groupWritableDigest)
	}

	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod fixture to 0755: %v", err)
	}
	_, executableDigest := localShareNormalizedTreeDigest(t, root)
	if executableDigest == regularDigest {
		t.Fatalf("executable Git file digest = regular file digest %s", regularDigest)
	}
}

func TestLocalShareWorkspaceUse(t *testing.T) {
	repoRoot := localShareRepositoryRoot(t)
	path := "backend/go.work"
	content := localShareReadFile(t, repoRoot, path)

	parsed, err := modfile.ParseWork(path, content, nil)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	const want = "./share"
	for _, use := range parsed.Use {
		if use.Path == want {
			return
		}
	}
	t.Errorf("%s does not use %s", path, want)
}

func TestLocalShareLegacyReferenceAllowlist(t *testing.T) {
	repoRoot := localShareRepositoryRoot(t)
	got := localShareGitGrepFiles(t, repoRoot, legacyShareModulePath)
	want, err := localShareHistoricalReferencePaths(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(want)

	if !reflect.DeepEqual(got, want) {
		missing, unexpected := localSharePathDifference(got, want)
		t.Errorf(
			"legacy Share reference allowlist mismatch: got %d files, want %d; missing: %s; unexpected: %s",
			len(got), len(want), localSharePathSummary(missing), localSharePathSummary(unexpected),
		)
	}
}

func TestLocalShareGitGrepIncludesUntrackedFiles(t *testing.T) {
	repoRoot := localShareRepositoryRoot(t)
	dir := filepath.Join(repoRoot, "backend/tools/internal/repositorycontract")
	file, err := os.CreateTemp(dir, "local-share-untracked-*.txt")
	if err != nil {
		t.Fatalf("create untracked marker file: %v", err)
	}
	t.Cleanup(func() {
		_ = file.Close()
		if err := os.Remove(file.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("remove untracked marker file: %v", err)
		}
	})

	marker := "LocalShareUntracked" + "GitGrepMarker"
	if _, err := file.WriteString(marker); err != nil {
		t.Fatalf("write untracked marker file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close untracked marker file: %v", err)
	}

	relativePath, err := filepath.Rel(repoRoot, file.Name())
	if err != nil {
		t.Fatalf("resolve untracked marker path: %v", err)
	}
	relativePath = filepath.ToSlash(relativePath)
	for _, path := range localShareGitGrepFiles(t, repoRoot, marker) {
		if path == relativePath {
			return
		}
	}
	t.Errorf("git grep did not include untracked repository file %s", relativePath)
}

func TestLocalShareLicenseNotices(t *testing.T) {
	repoRoot := localShareRepositoryRoot(t)
	paths := []string{
		"backend/share/utils/keyMatch.go",
		"backend/share/utils/notify/notify_linux.go",
		"backend/share/utils/notify/notify_other.go",
		"backend/share/utils/notify/notify_windows.go",
	}
	markers := []string{
		"Copyright",
		"Licensed under the Apache License, Version 2.0",
	}

	for _, path := range paths {
		t.Run(strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			content := localShareReadFile(t, repoRoot, path)
			for _, marker := range markers {
				if !strings.Contains(string(content), marker) {
					t.Errorf("%s is missing required license text %q", path, marker)
				}
			}
		})
	}
}

func localShareRepositoryRoot(t *testing.T) string {
	t.Helper()

	output, err := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		t.Fatalf("locate repository root: %v\n%s", err, output)
	}
	repoRoot := strings.TrimSuffix(string(output), "\r\n")
	return strings.TrimSuffix(repoRoot, "\n")
}

func localShareReadFile(t *testing.T, repoRoot, path string) []byte {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("read repository file %s: %v", path, err)
	}
	return content
}

func localShareGitGrepFiles(t *testing.T, repoRoot, pattern string) []string {
	t.Helper()

	command := exec.Command(
		"git", "-c", "color.ui=false", "-C", repoRoot,
		"grep", "--no-color", "--untracked", "-F", "-l", "-z", pattern, "--", ".",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) || exitError.ExitCode() != 1 {
			t.Fatalf("list files containing legacy Share references: %v\n%s", err, output)
		}
	}

	trimmed := strings.TrimSuffix(string(output), "\x00")
	if trimmed == "" {
		return nil
	}
	paths := strings.Split(trimmed, "\x00")
	sort.Strings(paths)
	return paths
}

func localSharePathDifference(got, want []string) (missing, unexpected []string) {
	gotSet := make(map[string]struct{}, len(got))
	for _, path := range got {
		gotSet[path] = struct{}{}
	}
	wantSet := make(map[string]struct{}, len(want))
	for _, path := range want {
		wantSet[path] = struct{}{}
	}

	for _, path := range want {
		if _, ok := gotSet[path]; !ok {
			missing = append(missing, path)
		}
	}
	for _, path := range got {
		if _, ok := wantSet[path]; !ok {
			unexpected = append(unexpected, path)
		}
	}
	return missing, unexpected
}

func localSharePathSummary(paths []string) string {
	const limit = 10
	if len(paths) == 0 {
		return "none"
	}
	if len(paths) <= limit {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s, ... (%d more)", strings.Join(paths[:limit], ", "), len(paths)-limit)
}

func localShareNormalizedTreeDigest(t *testing.T, root string) (int, string) {
	t.Helper()

	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)
		if relativePath == "LICENSE" || relativePath == "SOURCE.json" {
			return nil
		}
		if localShareIsProjectOwnedExtension(relativePath) {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported Share source entry %s with type %s", relativePath, entry.Type())
		}
		paths = append(paths, relativePath)
		return nil
	})
	if err != nil {
		t.Fatalf("walk normalized Share source tree: %v", err)
	}
	sort.Strings(paths)

	hash := sha256.New()
	for _, relativePath := range paths {
		path := filepath.Join(root, filepath.FromSlash(relativePath))
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat normalized Share source file %s: %v", relativePath, err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read normalized Share source file %s: %v", relativePath, err)
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%04o\x00", relativePath, localShareGitFileMode(info.Mode()))
		_, _ = hash.Write(content)
		_, _ = hash.Write([]byte{0})
	}
	return len(paths), fmt.Sprintf("%x", hash.Sum(nil))
}

// localShareIsProjectOwnedExtension 将迁入后由 ming-iot 自行维护的扩展排除在上游归档摘要之外。
func localShareIsProjectOwnedExtension(relativePath string) bool {
	for _, prefix := range localShareProjectOwnedExtensions {
		if relativePath == prefix || strings.HasPrefix(relativePath, prefix+"/") {
			return true
		}
	}
	return false
}

// localShareGitFileMode 将工作树权限归一化为 Git 可持久化的普通文件模式，仅保留可执行语义。
func localShareGitFileMode(mode fs.FileMode) fs.FileMode {
	if mode.Perm()&0o111 != 0 {
		return 0o755
	}
	return 0o644
}
