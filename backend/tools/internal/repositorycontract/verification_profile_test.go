package repositorycontract

import (
	"bytes"
	"context"
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
	"time"
)

const verificationCommandTimeout = 3 * time.Second

func TestVerificationProfile(t *testing.T) {
	repoRoot := findRepositoryRoot(t)

	t.Run("root makefile exists", func(t *testing.T) {
		info, err := os.Stat(filepath.Join(repoRoot, "Makefile"))
		if err != nil {
			t.Fatalf("stat root Makefile: %v", err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("root Makefile mode = %s, want a regular file", info.Mode())
		}
	})

	t.Run("help lists public targets", func(t *testing.T) {
		result := runVerificationMake(t, repoRoot, "help")
		if result.err != nil {
			t.Fatalf("make help: %v\n%s", result.err, result.output)
		}
		for _, target := range []string{
			"build",
			"test",
			"vet",
			"contract-static",
			"verify-fast",
			"verify-pr",
			"verify-integration",
			"verify-release",
		} {
			if !strings.Contains(result.output, target) {
				t.Errorf("make help does not list public target %q\n%s", target, result.output)
			}
		}
	})

	t.Run("verify pr dependency order", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
		if err != nil {
			t.Fatalf("read root Makefile: %v", err)
		}
		got, ok := makeRuleDependencies(string(content), "verify-pr")
		if !ok {
			t.Fatal("root Makefile has no verify-pr dependency rule")
		}
		want := []string{"context", "diff-check", "build", "test", "vet", "contract-static", "boundary-scan"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("verify-pr dependencies = %q, want %q", got, want)
		}
	})

	t.Run("things baseline contract is static", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(repoRoot, "deploy", "Makefile"))
		if err != nil {
			t.Fatalf("read deploy Makefile: %v", err)
		}
		block, ok := makeTargetBlock(string(content), "things-baseline-contract")
		if !ok {
			t.Fatal("deploy Makefile has no things-baseline-contract target")
		}
		for _, required := range []string{
			"mktemp",
			"chmod 0600",
			"contract --contract $(CONTRACT)",
			"config --quiet",
			"trap",
		} {
			if !strings.Contains(block, required) {
				t.Errorf("things-baseline-contract is missing %q\n%s", required, block)
			}
		}
		for _, forbidden := range []string{"things-baseline-build", "things-baseline-image", " up ", " up\n"} {
			if strings.Contains(block, forbidden) {
				t.Errorf("things-baseline-contract contains runtime/build operation %q\n%s", forbidden, block)
			}
		}
	})

	t.Run("integration and release lifecycle", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
		if err != nil {
			t.Fatalf("read root Makefile: %v", err)
		}
		integration, ok := makeTargetBlock(string(content), "verify-integration")
		if !ok {
			t.Fatal("root Makefile has no verify-integration target")
		}
		assertTextOrder(t, integration, []string{
			"\n\t\tkafka) \\\n",
			"cleanup()",
			"kafka-down",
			"trap",
			"kafka-up",
			"kafka-ready",
			"kafkacontract",
			"--integration",
		})
		assertTextOrder(t, integration, []string{
			"\n\t\tthings-startup) \\\n",
			"cleanup()",
			"things-baseline-logs",
			"things-baseline-down",
			"trap",
			"things-baseline-up",
			"things-baseline-verify",
		})
		for _, forbidden := range []string{"things-baseline-reset", "kafka-reset", "down -v", "down --volumes", "DROP DATABASE", "DROP ROLE", "TRUNCATE", "kubectl", "helm"} {
			if strings.Contains(integration, forbidden) {
				t.Errorf("verify-integration contains forbidden lifecycle operation %q", forbidden)
			}
		}

		release, ok := makeTargetBlock(string(content), "verify-release")
		if !ok {
			t.Fatal("root Makefile has no verify-release target")
		}
		if !strings.Contains(release, "MING_HEAVY_VERIFY") || !strings.Contains(release, "!= 1") {
			t.Errorf("verify-release does not require MING_HEAVY_VERIFY=1\n%s", release)
		}
		assertTextOrder(t, release, []string{
			"verify-pr",
			"verify-integration VERIFY_SUITE=kafka",
			"verify-integration VERIFY_SUITE=things-startup",
		})
		for _, forbidden := range []string{"deploy", "publish", "release-upload", "kubectl", "helm", "DROP DATABASE", "--volumes"} {
			if strings.Contains(release, forbidden) {
				t.Errorf("verify-release contains forbidden production operation %q", forbidden)
			}
		}
	})

	failureCases := []struct {
		name string
		args []string
	}{
		{name: "verify fast requires scope", args: []string{"verify-fast"}},
		{name: "verify fast rejects unknown scope", args: []string{"verify-fast", "VERIFY_SCOPE=unknown"}},
		{name: "verify integration requires suite", args: []string{"verify-integration"}},
		{name: "verify integration rejects unknown suite", args: []string{"verify-integration", "VERIFY_SUITE=unknown"}},
		{name: "verify release requires heavy confirmation", args: []string{"verify-release"}},
	}
	for _, testCase := range failureCases {
		t.Run(testCase.name, func(t *testing.T) {
			assertVerificationFailsClosed(t, repoRoot, testCase.args...)
		})
	}
}

func TestRepositoryDeliveryBoundary(t *testing.T) {
	t.Run("path classification", func(t *testing.T) {
		testCases := []struct {
			path string
			want bool
		}{
			{path: ".loopx/registry.json", want: true},
			{path: ".codex/goals/example/state.md", want: true},
			{path: "backend/bin/thingsvr", want: true},
			{path: "deploy/.runtime/state.json", want: true},
			{path: ".env", want: true},
			{path: "deploy/service/.env", want: true},
			{path: "deploy/service/.env.example", want: false},
			{path: "credentials/server." + "pem", want: true},
			{path: "credentials/private." + "key", want: true},
			{path: "credentials/id_" + "ed25519", want: true},
			{path: "logs/service." + "log", want: true},
			{path: "backend/share/log/logger.go", want: false},
		}
		for _, testCase := range testCases {
			t.Run(strings.ReplaceAll(testCase.path, "/", "_"), func(t *testing.T) {
				if got := repositoryDeliveryPathForbidden(testCase.path); got != testCase.want {
					t.Fatalf("repositoryDeliveryPathForbidden(%q) = %t, want %t", testCase.path, got, testCase.want)
				}
			})
		}
	})

	t.Run("diagnostics contain paths only", func(t *testing.T) {
		repoRoot := t.TempDir()
		runFixtureGit(t, repoRoot, "init", "-q")

		secret := "fixture-" + "sensitive-" + "payload"
		writeFixtureFile(t, repoRoot, "deploy/service/.env", secret)
		writeFixtureFile(t, repoRoot, "deploy/service/.env.example", "reviewed placeholder")
		writeFixtureFile(t, repoRoot, "keys/private."+"pem", secret)
		writeFixtureFile(t, repoRoot, "logs/runtime."+"log", secret)
		runFixtureGit(t, repoRoot, "add", "-f", ".")

		violations, err := repositoryDeliveryViolations(repoRoot)
		if err != nil {
			t.Fatalf("scan fixture repository: %v", err)
		}
		want := []string{"deploy/service/.env", "keys/private.pem", "logs/runtime.log"}
		if !reflect.DeepEqual(violations, want) {
			t.Fatalf("fixture violations = %q, want %q", violations, want)
		}
		diagnostic := strings.Join(violations, "\n")
		if filepath.IsAbs(diagnostic) {
			t.Fatalf("boundary diagnostic contains an absolute path: %q", diagnostic)
		}
		if strings.Contains(diagnostic, repoRoot) {
			t.Fatalf("boundary diagnostic leaks repository root: %q", diagnostic)
		}
		if strings.Contains(diagnostic, secret) {
			t.Fatalf("boundary diagnostic leaks file contents: %q", diagnostic)
		}
	})

	t.Run("deleted tracked paths are absent from candidate", func(t *testing.T) {
		repoRoot := t.TempDir()
		runFixtureGit(t, repoRoot, "init", "-q")

		const relativePath = "logs/retired.log"
		writeFixtureFile(t, repoRoot, relativePath, "retired evidence")
		runFixtureGit(t, repoRoot, "add", "-f", relativePath)
		if err := os.Remove(filepath.Join(repoRoot, filepath.FromSlash(relativePath))); err != nil {
			t.Fatalf("remove tracked fixture: %v", err)
		}

		violations, err := repositoryDeliveryViolations(repoRoot)
		if err != nil {
			t.Fatalf("scan fixture repository: %v", err)
		}
		if len(violations) != 0 {
			t.Fatalf("deleted tracked path remains in candidate violations: %q", violations)
		}
	})

	t.Run("tracked repository", func(t *testing.T) {
		repoRoot := findRepositoryRoot(t)
		violations, err := repositoryDeliveryViolations(repoRoot)
		if err != nil {
			t.Fatalf("scan tracked repository boundary: %v", err)
		}
		if len(violations) != 0 {
			t.Fatalf("forbidden files are tracked:\n%s", strings.Join(violations, "\n"))
		}
	})
}

func TestPullRequestTemplate(t *testing.T) {
	repoRoot := findRepositoryRoot(t)
	path := filepath.Join(repoRoot, ".gitee", "PULL_REQUEST_TEMPLATE.zh-CN.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Gitee pull request template: %v", err)
	}
	text := string(content)

	for _, heading := range []string{
		"## Delivery Issue",
		"## 变更目的与边界",
		"## RFC / Spec / Plan",
		"## 稳定项目契约晋升",
		"## Verification Profile",
		"## 精确候选 Head",
		"## 独立 Review",
		"## 风险、兼容性与回滚",
		"## 高风险操作声明",
	} {
		if count := strings.Count(text, heading); count != 1 {
			t.Errorf("template heading %q count = %d, want 1", heading, count)
		}
	}

	for _, required := range []string{
		"#<Issue ident>",
		"ming-iot-rd-docs",
		"仓库相对路径",
		"已合并 revision",
		"40 位",
		"verdict",
		"exact head",
		"review ref",
		"blocker",
		"warning",
		"未运行项",
		"原因",
		"补验证方式",
		"不包含发布、生产、凭据和权限操作",
	} {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(required)) {
			t.Errorf("template is missing required instruction %q", required)
		}
	}

	for _, forbidden := range []string{"verdict: pass", "verdict：pass", "[x]", "[X]"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("template pre-approves evidence with %q", forbidden)
		}
	}
}

func TestGiteeWorkflow(t *testing.T) {
	repoRoot := findRepositoryRoot(t)
	path := filepath.Join(repoRoot, ".workflow", "build-test-vet-202608301305.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Gitee workflow: %v", err)
	}
	text := string(content)

	for _, required := range []string{
		`golangVersion: "1.24"`,
		"- set -eu",
		"- go version",
		"- go env GOPATH GOMODCACHE GOCACHE",
		`env GOPROXY="${MING_GOPROXY:-https://goproxy.cn,direct}" make verify-pr`,
		"- /go/pkg/mod",
		"- /root/.cache/go-build",
		"triggers:\n  pr:\n    branches:\n      precise:\n        - master",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("Gitee workflow is missing %q", required)
		}
	}
	if count := strings.Count(text, "MING_GOPROXY"); count != 1 {
		t.Errorf("Gitee workflow MING_GOPROXY count = %d, want 1", count)
	}
	if count := strings.Count(text, "make verify-pr"); count != 1 {
		t.Errorf("Gitee workflow make verify-pr count = %d, want 1", count)
	}

	for _, forbidden := range []string{
		"make -C backend/saas build",
		"make -C backend/things build",
		"go -C backend test",
		"go -C backend vet",
		"gitee.com/ming-iot/share/...",
		"ming-iot/backend/tools/...",
		"artifacts:",
		"BUILD_ARTIFACT",
		"mkdir -p output",
		"verify-integration",
		"verify-release",
		"docker compose",
		"docker-compose",
		"trigger: manual",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("Gitee PR workflow contains forbidden duplicate/heavy operation %q", forbidden)
		}
	}
}

type makeResult struct {
	output   string
	err      error
	timedOut bool
	called   []string
}

func runVerificationMake(t *testing.T, repoRoot string, args ...string) makeResult {
	t.Helper()

	realMake, err := exec.LookPath("make")
	if err != nil {
		t.Fatalf("locate make: %v", err)
	}
	fakeBin := t.TempDir()
	calledPath := filepath.Join(fakeBin, "called")
	for _, name := range []string{"docker", "docker-compose", "go"} {
		script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' %s >> %s\nexit 97\n", shellQuote(name), shellQuote(calledPath))
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte(script), 0o755); err != nil {
			t.Fatalf("write fake %s command: %v", name, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), verificationCommandTimeout)
	defer cancel()
	commandArgs := []string{
		"-C", repoRoot,
		"VERIFY_SCOPE=",
		"VERIFY_SUITE=",
		"MING_HEAVY_VERIFY=0",
	}
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(ctx, realMake, commandArgs...)
	command.Env = verificationCommandEnvironment(fakeBin)
	output, commandErr := command.CombinedOutput()

	result := makeResult{output: string(output), err: commandErr, timedOut: errors.Is(ctx.Err(), context.DeadlineExceeded)}
	called, readErr := os.ReadFile(calledPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read fake command record: %v", readErr)
	}
	result.called = strings.Fields(string(called))
	return result
}

func assertVerificationFailsClosed(t *testing.T, repoRoot string, args ...string) {
	t.Helper()

	guardedPaths := []string{"backend/bin", "deploy/.runtime"}
	before := make(map[string][]repositoryPathState, len(guardedPaths))
	for _, path := range guardedPaths {
		state, err := snapshotRepositoryPath(repoRoot, path)
		if err != nil {
			t.Fatalf("snapshot %s before fail-fast command: %v", path, err)
		}
		before[path] = state
	}

	result := runVerificationMake(t, repoRoot, args...)
	if result.timedOut {
		t.Fatalf("make %s did not fail within %s\n%s", strings.Join(args, " "), verificationCommandTimeout, result.output)
	}
	if result.err == nil {
		t.Fatalf("make %s succeeded, want fail-closed\n%s", strings.Join(args, " "), result.output)
	}
	if len(result.called) != 0 {
		t.Fatalf("make %s invoked expensive command(s) before validating parameters: %q\n%s", strings.Join(args, " "), result.called, result.output)
	}
	for _, path := range guardedPaths {
		after, err := snapshotRepositoryPath(repoRoot, path)
		if err != nil {
			t.Fatalf("snapshot %s after fail-fast command: %v", path, err)
		}
		if !reflect.DeepEqual(after, before[path]) {
			t.Fatalf("make %s changed %s on a fail-fast path", strings.Join(args, " "), path)
		}
	}
}

func verificationCommandEnvironment(fakeBin string) []string {
	blocked := map[string]struct{}{
		"PATH":              {},
		"MAKEFLAGS":         {},
		"MAKEOVERRIDES":     {},
		"MFLAGS":            {},
		"VERIFY_SCOPE":      {},
		"VERIFY_SUITE":      {},
		"MING_HEAVY_VERIFY": {},
	}
	environment := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, found := blocked[name]; found {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment,
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"VERIFY_SCOPE=",
		"VERIFY_SUITE=",
		"MING_HEAVY_VERIFY=0",
	)
}

type repositoryPathState struct {
	path       string
	mode       fs.FileMode
	size       int64
	modifiedNS int64
	linkTarget string
}

func snapshotRepositoryPath(repoRoot, relativePath string) ([]repositoryPathState, error) {
	root := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if _, err := os.Lstat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var states []repositoryPathState
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		state := repositoryPathState{
			path:       filepath.ToSlash(relative),
			mode:       info.Mode(),
			size:       info.Size(),
			modifiedNS: info.ModTime().UnixNano(),
		}
		if info.Mode()&os.ModeSymlink != 0 {
			state.linkTarget, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		states = append(states, state)
		return nil
	})
	return states, err
}

func makeRuleDependencies(content, target string) ([]string, bool) {
	logicalLines := make([]string, 0)
	var current strings.Builder
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		trimmedRight := strings.TrimRight(line, " \t")
		continued := strings.HasSuffix(trimmedRight, "\\")
		if continued {
			trimmedRight = strings.TrimSuffix(trimmedRight, "\\")
		}
		if current.Len() > 0 {
			current.WriteByte(' ')
		}
		current.WriteString(strings.TrimSpace(trimmedRight))
		if continued {
			continue
		}
		logicalLines = append(logicalLines, current.String())
		current.Reset()
	}

	prefix := target + ":"
	for _, line := range logicalLines {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		return strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, prefix))), true
	}
	return nil, false
}

func makeTargetBlock(content, target string) (string, bool) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	start := -1
	for index, line := range lines {
		if strings.HasPrefix(line, target+":") {
			start = index
			break
		}
	}
	if start < 0 {
		return "", false
	}
	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		line := lines[index]
		if line == "" || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, ":") {
			end = index
			break
		}
	}
	return strings.Join(lines[start:end], "\n"), true
}

func assertTextOrder(t *testing.T, content string, markers []string) {
	t.Helper()
	offset := 0
	for _, marker := range markers {
		index := strings.Index(content[offset:], marker)
		if index < 0 {
			t.Fatalf("content does not contain %q after previous marker\n%s", marker, content)
		}
		offset += index + len(marker)
	}
}

func repositoryDeliveryViolations(repoRoot string) ([]string, error) {
	deletedCommand := exec.Command("git", "-c", "core.quotepath=false", "-C", repoRoot, "ls-files", "-z", "--deleted")
	deletedOutput, err := deletedCommand.Output()
	if err != nil {
		return nil, fmt.Errorf("list deleted tracked files: %w", err)
	}
	deleted := make(map[string]struct{})
	for _, rawPath := range bytes.Split(deletedOutput, []byte{0}) {
		if len(rawPath) == 0 {
			continue
		}
		deleted[filepath.ToSlash(string(rawPath))] = struct{}{}
	}

	command := exec.Command("git", "-c", "core.quotepath=false", "-C", repoRoot, "ls-files", "-z")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("list tracked files: %w", err)
	}

	var violations []string
	for _, rawPath := range bytes.Split(output, []byte{0}) {
		if len(rawPath) == 0 {
			continue
		}
		path := filepath.ToSlash(string(rawPath))
		if _, removed := deleted[path]; removed {
			continue
		}
		if repositoryDeliveryPathForbidden(path) {
			violations = append(violations, path)
		}
	}
	sort.Strings(violations)
	return violations, nil
}

func repositoryDeliveryPathForbidden(path string) bool {
	path = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), "./")
	lower := strings.ToLower(path)
	for _, prefix := range []string{".loopx/", ".codex/", "backend/bin/", "deploy/.runtime/"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	base := strings.ToLower(filepath.Base(lower))
	if base == ".env" {
		return true
	}
	for _, name := range []string{"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519"} {
		if base == name {
			return true
		}
	}
	for _, extension := range []string{".pem", ".key", ".p12", ".pfx", ".log"} {
		if strings.HasSuffix(base, extension) {
			return true
		}
	}
	return false
}

func runFixtureGit(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repoRoot}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func writeFixtureFile(t *testing.T, repoRoot, relativePath, content string) {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory for %s: %v", relativePath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", relativePath, err)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
