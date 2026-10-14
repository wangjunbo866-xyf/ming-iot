# Thingsbaseline Shared Go Cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `thingsbaseline` reuse the trusted host `GOCACHE` and `GOMODCACHE` without weakening candidate/output isolation or allowing Go test-result cache to replace a real test run.

**Architecture:** Add one focused cache-path resolver that asks the attested Go binary for `GOCACHE/GOMODCACHE`, validates and prepares those directories, and injects the resulting value object into render and verification dependencies. Keep `HOME`, `GOTMPDIR`, candidate trees, temporary workspaces, and build outputs per-run; add `-count=1` to every test Gate and a real two-random-path regression before accepting shared `GOCACHE`.

**Tech Stack:** Go 1.25.4, `os/exec`, `encoding/json`, `golang.org/x/sys/unix`, existing `thingsbaseline` fake runners and Graph control plane.

**Design:** `docs/superpowers/specs/2026-08-19-thingsbaseline-shared-go-cache-design.md`

**Commit authorization:** Repository policy overrides automatic-commit instructions. At every commit checkpoint, show `git status` and the staged file list, then wait for explicit user approval before running `git commit`.

---

## File map

- Create `backend/tools/internal/thingsbaseline/go_cache.go`: shared cache value object, trusted `go env` resolver, bounded JSON parsing, path validation/preparation.
- Create `backend/tools/internal/thingsbaseline/go_cache_test.go`: resolver/path tests and the real random-candidate-path cache regression.
- Modify `backend/tools/internal/thingsbaseline/render.go`: carry resolved cache paths through production/test dependencies and build private command environments around them.
- Modify `backend/tools/internal/thingsbaseline/render_test.go`: update fixed-environment assertions and prove shared caches plus distinct private directories.
- Modify `backend/tools/internal/thingsbaseline/verify.go`: reuse shared caches in all Gates, add `-count=1`, preserve private workspace/output cleanup.
- Modify `backend/tools/internal/thingsbaseline/verify_test.go`: verify Gate argv/display, shared cache lifetime, redaction, and unchanged `backend/bin` behavior.
- Modify `docs/superpowers/specs/2026-08-14-l0-things-compilation-migration-baseline-design.md`: mark only the old private-cache clauses as superseded.
- Modify `backend/tools/README.md`: document shared cache semantics, real test execution, troubleshooting, and manual cleanup boundary.

### Task 1: Add the shared cache resolver contract

**Files:**
- Create: `backend/tools/internal/thingsbaseline/go_cache.go`
- Test: `backend/tools/internal/thingsbaseline/go_cache_test.go`

- [ ] **Step 1: Write failing parser and resolver tests**

Create `go_cache_test.go` with tests that require absolute prepared directories and generic errors that do not echo cache paths:

```go
package thingsbaseline

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestResolveSharedGoCachePathsUsesGoEnvJSON(t *testing.T) {
	root := t.TempDir()
	want := sharedGoCachePaths{
		Build:   filepath.Join(root, "go-build"),
		Modules: filepath.Join(root, "pkg-mod"),
	}
	payload, err := json.Marshal(map[string]string{
		"GOCACHE":    want.Build,
		"GOMODCACHE": want.Modules,
	})
	if err != nil {
		t.Fatal(err)
	}
	goBinary := filepath.Join(t.TempDir(), "go")
	script := "#!/bin/sh\nprintf '%s' " + strconv.Quote(string(payload)) + "\n"
	if err := os.WriteFile(goBinary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveSharedGoCachePaths(context.Background(), goBinary)
	if err != nil {
		t.Fatalf("resolveSharedGoCachePaths() error: %v", err)
	}
	if got != want {
		t.Fatalf("shared cache = %+v, want %+v", got, want)
	}
	for _, path := range []string{got.Build, got.Modules} {
		info, statErr := os.Stat(path)
		if statErr != nil || !info.IsDir() {
			t.Fatalf("shared cache directory %q is unavailable: %v", path, statErr)
		}
	}
}

func TestParseSharedGoCachePathsRejectsUnsafeValuesWithoutEcho(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		build   string
		modules string
	}{
		{name: "empty build", modules: filepath.Join(t.TempDir(), "mod")},
		{name: "empty modules", build: filepath.Join(t.TempDir(), "build")},
		{name: "relative build", build: "relative/build", modules: filepath.Join(t.TempDir(), "mod")},
		{name: "unclean build", build: filepath.Join(t.TempDir(), "parent", "..", "build"), modules: filepath.Join(t.TempDir(), "mod")},
		{name: "root build", build: string(filepath.Separator), modules: filepath.Join(t.TempDir(), "mod")},
		{name: "module cache is file", build: filepath.Join(t.TempDir(), "build"), modules: file},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{"GOCACHE": tt.build, "GOMODCACHE": tt.modules})
			if err != nil {
				t.Fatal(err)
			}
			_, err = parseSharedGoCachePaths(payload)
			if err == nil {
				t.Fatal("parseSharedGoCachePaths() accepted unsafe cache paths")
			}
			if (tt.build != "" && strings.Contains(err.Error(), tt.build)) ||
				(tt.modules != "" && strings.Contains(err.Error(), tt.modules)) {
				t.Fatalf("error leaked cache path: %v", err)
			}
		})
	}
}

func TestResolveSharedGoCachePathsBoundsGoEnvOutput(t *testing.T) {
	payloadPath := filepath.Join(t.TempDir(), "oversized-go-env")
	if err := os.WriteFile(payloadPath, bytes.Repeat([]byte("x"), maxGoEnvCacheOutputBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	goBinary := filepath.Join(t.TempDir(), "go")
	script := "#!/bin/sh\nexec /bin/cat " + strconv.Quote(payloadPath) + "\n"
	if err := os.WriteFile(goBinary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := resolveSharedGoCachePaths(context.Background(), goBinary)
	if err == nil || !strings.Contains(err.Error(), "outside the allowed size") {
		t.Fatalf("oversized go env error = %v", err)
	}
	if strings.Contains(err.Error(), payloadPath) {
		t.Fatalf("oversized go env error leaked path: %v", err)
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail for the missing contract**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline -run 'Test(ResolveSharedGoCachePaths|ParseSharedGoCachePaths)' -count=1
```

Expected: compile failure because `sharedGoCachePaths`, `resolveSharedGoCachePaths`, and `parseSharedGoCachePaths` do not exist.

- [ ] **Step 3: Implement the minimal resolver**

Create `go_cache.go`:

```go
package thingsbaseline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const maxGoEnvCacheOutputBytes = 64 << 10

type sharedGoCachePaths struct {
	Build   string
	Modules string
}

func resolveSharedGoCachePaths(ctx context.Context, goBinary string) (sharedGoCachePaths, error) {
	if err := ctx.Err(); err != nil {
		return sharedGoCachePaths{}, err
	}
	if !filepath.IsAbs(goBinary) || filepath.Clean(goBinary) != goBinary {
		return sharedGoCachePaths{}, fmt.Errorf("trusted Go binary path is invalid")
	}
	command := exec.CommandContext(ctx, goBinary, "env", "-json", "GOCACHE", "GOMODCACHE")
	command.Env = os.Environ()
	stdout, err := command.StdoutPipe()
	if err != nil {
		return sharedGoCachePaths{}, fmt.Errorf("prepare shared Go cache response")
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return sharedGoCachePaths{}, fmt.Errorf("resolve shared Go cache paths")
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, maxGoEnvCacheOutputBytes+1))
	if readErr != nil || len(output) > maxGoEnvCacheOutputBytes {
		_ = command.Process.Kill()
		_ = command.Wait()
		if readErr != nil {
			return sharedGoCachePaths{}, fmt.Errorf("read shared Go cache response")
		}
		return sharedGoCachePaths{}, fmt.Errorf("shared Go cache response is outside the allowed size")
	}
	if err := command.Wait(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return sharedGoCachePaths{}, ctxErr
		}
		return sharedGoCachePaths{}, fmt.Errorf("resolve shared Go cache paths")
	}
	if len(output) == 0 {
		return sharedGoCachePaths{}, fmt.Errorf("shared Go cache response is outside the allowed size")
	}
	return parseSharedGoCachePaths(output)
}

func parseSharedGoCachePaths(output []byte) (sharedGoCachePaths, error) {
	var values struct {
		GOCACHE    string
		GOMODCACHE string
	}
	if err := json.Unmarshal(output, &values); err != nil {
		return sharedGoCachePaths{}, fmt.Errorf("decode shared Go cache response")
	}
	cache := sharedGoCachePaths{Build: values.GOCACHE, Modules: values.GOMODCACHE}
	if err := prepareSharedGoCacheDirectory("GOCACHE", cache.Build); err != nil {
		return sharedGoCachePaths{}, err
	}
	if err := prepareSharedGoCacheDirectory("GOMODCACHE", cache.Modules); err != nil {
		return sharedGoCachePaths{}, err
	}
	return cache, nil
}

func prepareSharedGoCacheDirectory(name, path string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || path == string(filepath.Separator) {
		return fmt.Errorf("%s is not a safe absolute directory", name)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("%s is unavailable", name)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%s is unavailable", name)
	}
	probe, err := os.CreateTemp(path, ".ming-cache-probe-*")
	if err != nil {
		return fmt.Errorf("%s is not writable", name)
	}
	probePath := probe.Name()
	if closeErr := probe.Close(); closeErr != nil {
		_ = os.Remove(probePath)
		return fmt.Errorf("%s write probe failed", name)
	}
	if err := os.Remove(probePath); err != nil {
		return fmt.Errorf("%s write probe cleanup failed", name)
	}
	return nil
}
```

- [ ] **Step 4: Run focused tests**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline -run 'Test(ResolveSharedGoCachePaths|ParseSharedGoCachePaths)' -count=1
```

Expected: PASS; both test-created cache directories exist, and invalid-path errors contain only the variable name/reason.

- [ ] **Step 5: Pause for commit authorization**

Run `git status --short` and `git diff -- backend/tools/internal/thingsbaseline/go_cache.go backend/tools/internal/thingsbaseline/go_cache_test.go`. After explicit approval only:

```bash
git add -- backend/tools/internal/thingsbaseline/go_cache.go backend/tools/internal/thingsbaseline/go_cache_test.go
git commit -m "feat(tools): resolve shared Go cache paths"
```

### Task 2: Use shared caches while keeping private command state

**Files:**
- Modify: `backend/tools/internal/thingsbaseline/render.go:100-109, 160-185, 307-320, 638-680`
- Modify: `backend/tools/internal/thingsbaseline/render_test.go:450-610, 850-910`
- Test: `backend/tools/internal/thingsbaseline/go_cache_test.go`

- [ ] **Step 1: Write failing environment and random-path tests**

Add a package-wide test helper to `go_cache_test.go` so render and Gate tests use the same fixture contract:

```go
func testSharedGoCachePaths(t *testing.T) sharedGoCachePaths {
	t.Helper()
	root := t.TempDir()
	cache := sharedGoCachePaths{
		Build:   filepath.Join(root, "go-build"),
		Modules: filepath.Join(root, "pkg-mod"),
	}
	for _, path := range []string{cache.Build, cache.Modules} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return cache
}
```

In `TestRenderCandidateUsesFixedEnvironmentAndTwoRendersAreIdentical`, replace the existing two fixture/render blocks with this ordering so both dependency sets receive one shared cache before either render starts:

```go
backendOne, lockOne, snapshotOne, dependenciesOne := syntheticRenderFixture(t)
backendTwo, lockTwo, snapshotTwo, dependenciesTwo := syntheticRenderFixture(t)
shared := testSharedGoCachePaths(t)
dependenciesOne.goCache = shared
dependenciesTwo.goCache = shared

first, err := renderCandidateWithDependencies(context.Background(), backendOne, snapshotOne, &lockOne, dependenciesOne.renderDependencies)
if err != nil {
	t.Fatalf("first render: %v", err)
}
t.Cleanup(func() { _ = first.Cleanup() })
second, err := renderCandidateWithDependencies(context.Background(), backendTwo, snapshotTwo, &lockTwo, dependenciesTwo.renderDependencies)
if err != nil {
	t.Fatalf("second render: %v", err)
}
t.Cleanup(func() { _ = second.Cleanup() })
```

Replace the old “independent cache roots” assertion with shared-cache and private-directory assertions:

```go

firstEnv := environmentMap(dependenciesOne.runner.calls[0].Env)
secondEnv := environmentMap(dependenciesTwo.runner.calls[0].Env)
for key, want := range map[string]string{"GOCACHE": shared.Build, "GOMODCACHE": shared.Modules} {
	if firstEnv[key] != want || secondEnv[key] != want {
		t.Fatalf("%s was not shared: first=%q second=%q want=%q", key, firstEnv[key], secondEnv[key], want)
	}
}
for _, key := range []string{"HOME", "GOTMPDIR"} {
	if firstEnv[key] == "" || secondEnv[key] == "" || firstEnv[key] == secondEnv[key] {
		t.Fatalf("%s was not private: first=%q second=%q", key, firstEnv[key], secondEnv[key])
	}
}
```

Add the real cross-path regression to `go_cache_test.go`:

```go
func TestSharedGoCacheSurvivesDeletedCandidatePath(t *testing.T) {
	shared := testSharedGoCachePaths(t)
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	var firstPath string
	for run := 1; run <= 2; run++ {
		candidate := filepath.Join(t.TempDir(), "candidate")
		if err := os.MkdirAll(candidate, 0o700); err != nil {
			t.Fatal(err)
		}
		files := map[string]string{
			"go.mod":          "module example.com/sharedcachefixture\n\ngo 1.24.4\n",
			"fixture.go":      "package fixture\n\nfunc Value() int { return 42 }\n",
			"fixture_test.go": "package fixture\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) { t.Log(\"EXECUTED-SHARED-CACHE-FIXTURE\"); if Value() != 42 { t.Fatal(Value()) } }\n",
		}
		for name, data := range files {
			if err := os.WriteFile(filepath.Join(candidate, name), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		privateRoot := filepath.Join(t.TempDir(), "private")
		environment, err := fixedCandidateGoEnvironment(privateRoot, shared)
		if err != nil {
			t.Fatal(err)
		}
		command := exec.Command(goBinary, "test", "-count=1", "-v", "./...")
		command.Dir = candidate
		command.Env = environment
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("run %d failed: %v\n%s", run, err, output)
		}
		if !bytes.Contains(output, []byte("EXECUTED-SHARED-CACHE-FIXTURE")) || bytes.Contains(output, []byte("(cached)")) {
			t.Fatalf("run %d did not execute tests:\n%s", run, output)
		}
		if firstPath != "" && bytes.Contains(output, []byte(firstPath)) {
			t.Fatalf("run %d output referenced deleted candidate %q:\n%s", run, firstPath, output)
		}
		if run == 1 {
			firstPath = candidate
			if err := os.RemoveAll(candidate); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestExecGoRunnerRedactsSharedGoCachesFromCommandReport(t *testing.T) {
	shared := testSharedGoCachePaths(t)
	environment, err := fixedCandidateGoEnvironment(t.TempDir(), shared)
	if err != nil {
		t.Fatal(err)
	}
	report, runErr := (execGoRunner{}).Run(context.Background(), GoCommand{
		Path: "/bin/sh",
		Dir:  t.TempDir(),
		Args: []string{"-c", `printf '%s\n%s\n' "$GOCACHE" "$GOMODCACHE"; exit 31`},
		Env:  environment,
	})
	if runErr == nil || report.ExitCode != 31 {
		t.Fatalf("cache echo report=%+v error=%v", report, runErr)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte("[REDACTED]")) {
		t.Fatalf("command report did not record redaction: %s", encoded)
	}
	for _, path := range []string{shared.Build, shared.Modules} {
		if bytes.Contains(encoded, []byte(path)) || strings.Contains(runErr.Error(), path) {
			t.Fatalf("command report leaked shared cache path: report=%s error=%v", encoded, runErr)
		}
	}
}
```

Add `os/exec` and `runtime` to `go_cache_test.go` imports; `bytes` was already added in Task 1.

- [ ] **Step 2: Run tests and verify the desired API fails to compile**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline -run 'Test(RenderCandidateUsesFixedEnvironment|SharedGoCacheSurvivesDeletedCandidatePath|ExecGoRunnerRedactsSharedGoCachesFromCommandReport)' -count=1
```

Expected: compile failures because `renderDependencies.goCache` and the two-argument `fixedCandidateGoEnvironment` are not implemented.

- [ ] **Step 3: Carry the cache value through production dependencies**

In `render.go`, add the field and resolve it immediately after Go-tool attestation:

```go
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
```

```go
goCache, err := resolveSharedGoCachePaths(context.Background(), goBinary)
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
```

Extend the dependency completeness check with `dependencies.goCache.Build == "" || dependencies.goCache.Modules == ""`.

- [ ] **Step 4: Change the fixed environment to share only Go caches**

Replace the private-cache creation in `renderCandidateForIntentWithDependencies` with a private environment root:

```go
privateRoot, err := os.MkdirTemp(backendPath, ".thingsbaseline-cache-")
if err != nil {
	return nil, fmt.Errorf("create private Go environment: %w", err)
}
defer os.RemoveAll(privateRoot)
goEnvironment, err := fixedCandidateGoEnvironment(privateRoot, dependencies.goCache)
if err != nil {
	return nil, err
}
```

Change the environment function:

```go
func fixedCandidateGoEnvironment(privateRoot string, cache sharedGoCachePaths) ([]string, error) {
	goProxy, err := resolveCandidateGoProxy()
	if err != nil {
		return nil, err
	}
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
```

Extend the `renderDependencies` literal in `syntheticRenderFixture`:

```go
renderDependencies: renderDependencies{
	runner:             runner,
	goBinary:           "/trusted/go",
	goVersion:          "go1.25.4",
	goCache:            testSharedGoCachePaths(t),
	expectedLicenseSHA: fmt.Sprintf("%x", licenseSHA),
},
```

Update the direct calls in
`TestFixedCandidateGoEnvironmentUsesMingGoProxyAndPreservesTransportProxy` and
`TestFixedCandidateGoEnvironmentRejectsUnsafeMingGoProxyWithoutEchoingValue`:

```go
environment, err := fixedCandidateGoEnvironment(t.TempDir(), testSharedGoCachePaths(t))
```

```go
_, err := fixedCandidateGoEnvironment(t.TempDir(), testSharedGoCachePaths(t))
```

- [ ] **Step 5: Run focused and package tests**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline -run 'Test(RenderCandidateUsesFixedEnvironment|FixedCandidateGoEnvironment|SharedGoCacheSurvivesDeletedCandidatePath|ExecGoRunnerRedactsSharedGoCachesFromCommandReport)' -count=1
GOWORK=off go test ./internal/thingsbaseline -count=1
```

Expected: PASS. The cross-path test runs twice without `(cached)` or the deleted first path.

- [ ] **Step 6: Pause for commit authorization**

Show `git status --short` and the diff for `go_cache*`, `render.go`, and `render_test.go`. After explicit approval only:

```bash
git add -- backend/tools/internal/thingsbaseline/go_cache_test.go backend/tools/internal/thingsbaseline/render.go backend/tools/internal/thingsbaseline/render_test.go
git commit -m "feat(tools): reuse trusted Go caches"
```

### Task 3: Apply shared caches and real-test semantics to verification Gates

**Files:**
- Modify: `backend/tools/internal/thingsbaseline/verify.go:1681-1775`
- Modify: `backend/tools/internal/thingsbaseline/verify_test.go:1260-1345`

- [ ] **Step 1: Write failing Gate-contract assertions**

In `TestDefaultGateSpecificationsBindIsolationPinBuildToolchainAndOwnOutputs`, insert this block after
`cacheRoot := filepath.Dir(buildOutput)` so `build`, `buildOutput`, and `cacheRoot` are already defined:

```go
firstEnvironment := environmentMap(specifications[0].Env)
sharedBuildCache := firstEnvironment["GOCACHE"]
sharedModuleCache := firstEnvironment["GOMODCACHE"]
if sharedBuildCache == "" || sharedModuleCache == "" {
	t.Fatal("shared Go cache paths are empty")
}
for index, specification := range specifications[:4] {
	if !slices.Contains(specification.Args, "-count=1") {
		t.Fatalf("test gate %s omitted -count=1: %q", specification.ID, specification.Args)
	}
	environment := environmentMap(specification.Env)
	if environment["GOCACHE"] != sharedBuildCache || environment["GOMODCACHE"] != sharedModuleCache {
		t.Fatalf("gate %s did not share Go caches", specification.ID)
	}
	if index == 0 && (strings.HasPrefix(sharedBuildCache, cacheRoot+string(filepath.Separator)) ||
		strings.HasPrefix(sharedModuleCache, cacheRoot+string(filepath.Separator))) {
		t.Fatal("shared Go caches were placed under the private Gate root")
	}
}
buildCacheEnvironment := environmentMap(build.Env)
if buildCacheEnvironment["GOCACHE"] != sharedBuildCache || buildCacheEnvironment["GOMODCACHE"] != sharedModuleCache {
	t.Fatal("build gate did not share Go caches with test gates")
}
if slices.Contains(build.Args, "-count=1") {
	t.Fatalf("build gate received a test-only flag: %q", build.Args)
}
for _, specification := range specifications {
	if !slices.Contains(specification.RedactValues, sharedBuildCache) ||
		!slices.Contains(specification.RedactValues, sharedModuleCache) {
		t.Fatalf("gate %s omitted explicit shared-cache redaction", specification.ID)
	}
	if strings.Contains(specification.DisplayCommand, sharedBuildCache) ||
		strings.Contains(specification.DisplayCommand, sharedModuleCache) {
		t.Fatalf("gate %s display command leaked a shared cache path", specification.ID)
	}
}
```

Replace the existing post-`cleanup()` cache-root assertion with:

```go
cleanup()
cleaned = true
if _, err := os.Stat(cacheRoot); !os.IsNotExist(err) {
	t.Fatalf("private Gate root still exists after cleanup: %q, err=%v", cacheRoot, err)
}
for _, path := range []string{sharedBuildCache, sharedModuleCache} {
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("shared Go cache was removed by cleanup: err=%v", err)
	}
}
```

Add a direct Evidence-redaction regression next to
`TestDefaultGateSpecificationsRedactInheritedTransportProxies`:

```go
func TestDefaultGateSpecificationsRedactSharedGoCachesFromEvidence(t *testing.T) {
	backend, _, candidate := verificationTargetFixture(t)
	isolation, err := createVerificationIsolation(candidate)
	if err != nil {
		t.Fatal(err)
	}
	defer isolation.Cleanup()
	specifications, cleanup, err := buildDefaultVerificationGates(VerifyRequest{Backend: backend}, isolation)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	specification := specifications[0]
	environment := environmentMap(specification.Env)
	cachePaths := []string{environment["GOCACHE"], environment["GOMODCACHE"]}
	specification.ID = "shared-cache-echo"
	specification.DisplayCommand = "echo redacted shared Go caches"
	specification.Path = "/bin/sh"
	specification.Args = []string{"-c", `printf '%s\n%s\n' "$GOCACHE" "$GOMODCACHE"; exit 31`}
	specification.Dir = isolation.Path()
	specification.goTool = nil
	specification.outputContract = nil
	result := (execVerificationGateRunner{}).Run(context.Background(), specification, time.Second)
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result != ResultFail || !bytes.Contains(encoded, []byte("[REDACTED]")) {
		t.Fatalf("shared-cache echo result = %s", encoded)
	}
	for _, path := range cachePaths {
		if path == "" || bytes.Contains(encoded, []byte(path)) {
			t.Fatalf("Gate evidence leaked shared cache path: %s", encoded)
		}
	}
}
```

- [ ] **Step 2: Run the focused test and verify it fails**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline -run 'TestDefaultGateSpecifications(BindIsolationPinBuildToolchainAndOwnOutputs|RedactSharedGoCachesFromEvidence)$' -count=1
```

Expected: FAIL because the four Gate argv arrays omit `-count=1`, caches are still private, and explicit cache redactions are absent.

- [ ] **Step 3: Pass shared cache paths into Gate and Make environments**

Update Gate environment construction:

```go
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
```

Change the helper signature and call:

```go
func strictMakeEnvironment(privateRoot string, cache sharedGoCachePaths) ([]string, error) {
	candidateEnvironment, err := fixedCandidateGoEnvironment(privateRoot, cache)
```

Update the four existing direct `strictMakeEnvironment` calls in
`TestExecVerificationGateRunnerPinsMakeToAbsoluteGoDespitePATH`, both subtests of
`TestStrictMakeEnvironmentPreservesOptionalTransportProxies`, and
`realThingsBuildSpecification` to pass an explicit test cache. Use the first form where the test already has `root`, and the second form where it currently passes `t.TempDir()` inline:

```go
cache := testSharedGoCachePaths(t)
makeEnvironment, err := strictMakeEnvironment(root, cache)
```

```go
environment, err := strictMakeEnvironment(t.TempDir(), testSharedGoCachePaths(t))
```

Include cache paths in explicit Gate redactions even though absolute environment values are already redacted by the shared diagnostic code:

```go
redactions := []string{backend, isolation.Path(), cacheRoot, dependencies.goCache.Build, dependencies.goCache.Modules}
redactions = append(redactions, transportProxyEnvironmentValues(environment, makeEnvironment)...)
```

- [ ] **Step 4: Add `-count=1` to every test Gate**

Replace the four test specifications with:

```go
{ID: "tools-test", DisplayCommand: "cd backend/tools && GOWORK=off go test -count=1 ./...", Path: dependencies.goBinary, Args: []string{"test", "-count=1", "./..."}, Dir: filepath.Join(backend, "tools"), Env: environment, RedactValues: redactions},
{ID: "saas-test", DisplayCommand: "cd backend/saas && GOWORK=off go test -count=1 ./...", Path: dependencies.goBinary, Args: []string{"test", "-count=1", "./..."}, Dir: filepath.Join(backend, "saas"), Env: environment, RedactValues: redactions},
{ID: "things-test", DisplayCommand: "cd verification-tree && GOWORK=off go test -count=1 ./...", Path: dependencies.goBinary, Args: []string{"test", "-count=1", "./..."}, Dir: isolation.Path(), Env: environment, RedactValues: redactions},
{ID: "workspace-test", DisplayCommand: "go test -count=1 Ming workspace modules", Path: dependencies.goBinary, Args: []string{"test", "-count=1", "gitee.com/ming-iot/saas/...", "gitee.com/ming-iot/things/...", "ming-iot/backend/tools/..."}, Dir: backend, Env: workspaceEnvironment, RedactValues: redactions},
```

- [ ] **Step 5: Run Gate and package tests**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline -run 'Test(DefaultGateSpecificationsBindIsolationPinBuildToolchainAndOwnOutputs|DefaultGateSpecificationsRedactInheritedTransportProxies|DefaultGateSpecificationsRedactSharedGoCachesFromEvidence|StrictMakeEnvironmentPreservesOptionalTransportProxies)$' -count=1
GOWORK=off go test ./cmd/thingsbaseline ./internal/thingsbaseline -count=1
```

Expected: PASS. Cleanup removes only `.thingsbaseline-gates-*`; both shared cache directories still exist, and `backend/bin/coresvr` remains byte-identical.

- [ ] **Step 6: Pause for commit authorization**

Show status and the exact diff. After explicit approval only:

```bash
git add -- backend/tools/internal/thingsbaseline/verify.go backend/tools/internal/thingsbaseline/verify_test.go
git commit -m "feat(tools): reuse Go caches in L0 gates"
```

### Task 4: Remove the documentation contradiction

**Files:**
- Modify: `docs/superpowers/specs/2026-08-14-l0-things-compilation-migration-baseline-design.md:175,357`
- Modify: `backend/tools/README.md:17-50`

- [ ] **Step 1: Add explicit supersession notes to the L0 design**

Immediately after the private-cache paragraph around line 175 and again after the verify environment paragraph around line 357, add:

```markdown
> **缓存策略更新（2026-08-19）：** 本段的“每轮独立私有 `GOCACHE/GOMODCACHE`”要求已由
> [Thingsbaseline 共享 Go 缓存设计](./2026-08-19-thingsbaseline-shared-go-cache-design.md)替代。
> candidate、`HOME`、`GOTMPDIR`、workspace 和构建输出仍逐轮隔离；只有受信 Go 解析出的
> `GOCACHE/GOMODCACHE` 跨轮复用，所有测试显式使用 `-count=1`。
```

- [ ] **Step 2: Update the operator README**

Replace “两个独立 cache 重渲染” with “两个独立 candidate 重渲染，共享宿主 Go 模块/编译缓存”。Replace the final cache cleanup paragraph with:

```markdown
`thingsbaseline` 通过受信 Go 工具的 `go env GOCACHE GOMODCACHE` 复用宿主缓存；candidate、
`HOME`、`GOTMPDIR`、临时 workspace 和构建输出仍逐轮隔离。四个 `go test` Gate 都显式使用
`-count=1`，因此只复用下载和编译产物，不复用测试结果。缓存损坏时工具会 fail-closed，但不会自动
执行 `go clean -cache` 或 `go clean -modcache`；清理共享缓存必须由操作者明确执行。

`--structural-only` 仅用于首次导入后的快速诊断，evidence 会明确写成
`finalEvidence=false`、`overallResult=NOT_FINAL`，不能作为 L0 完成证据。候选和隔离目录由工具清理；
外部强制终止仍可能留下带 `.thingsbaseline-*` 前缀的私有临时目录，但不会再为每轮复制独立
`GOCACHE/GOMODCACHE`。不要人工编辑生成的 `backend/things`，修改迁移规则应从 lock、转换器及其
测试开始。
```

- [ ] **Step 3: Verify links, formatting, and terminology**

Run:

```bash
git diff --check
rg -n '每轮.*私有.*GOCACHE|两个独立 cache' docs/superpowers/specs/2026-08-14-l0-things-compilation-migration-baseline-design.md backend/tools/README.md
```

Expected: `git diff --check` exits zero. Any old private-cache wording is immediately followed by the supersession note; README no longer claims caches are per-run.

- [ ] **Step 4: Pause for commit authorization**

After showing status and diff, obtain explicit approval, then:

```bash
git add -- docs/superpowers/specs/2026-08-14-l0-things-compilation-migration-baseline-design.md backend/tools/README.md
git commit -m "docs(tools): explain shared Go cache semantics"
```

### Task 5: Run full verification twice and measure the hot-cache result

**Files:**
- Verify only; do not update canonical Evidence or delete the existing 6.3 GiB directory.

- [ ] **Step 1: Recompute Graph context from the complete change set**

Run from `backend/tools`:

```bash
GOWORK=off go run ./cmd/graphctl validate --repo-root ../..
GOWORK=off go run ./cmd/graphctl context --repo-root ../..
GOWORK=off go run ./cmd/graphctl scope-check --repo-root ../..
```

Expected: Registry valid; no `UNOWNED_PATH`/`AMBIGUOUS_OWNER`; affected Loops are machine-derived rather than manually assumed.

- [ ] **Step 2: Run focused correctness checks**

```bash
GOWORK=off go test ./internal/thingsbaseline -run 'Test(ResolveSharedGoCachePaths|ParseSharedGoCachePaths|SharedGoCacheSurvivesDeletedCandidatePath|RenderCandidateUsesFixedEnvironment|DefaultGateSpecifications)' -count=1
GOWORK=off go test ./cmd/thingsbaseline ./internal/thingsbaseline -count=1
```

Expected: PASS with the random-path fixture executed twice and no `(cached)` result.

- [ ] **Step 3: Run all tools tests, race tests, and vet**

```bash
GOWORK=off go test ./... -count=1
GOWORK=off go test -race ./... -count=1
GOWORK=off go vet ./...
```

Expected: all commands exit zero. A failure in the random-path, race, or vet checks stops the implementation; do not fall back silently or continue to the real L0 run.

- [ ] **Step 4: Record the real `backend/bin` state without modifying it**

Run before and after the two L0 runs:

```bash
find ../bin -maxdepth 1 -type f -exec sha256sum {} + | sort
```

Expected: the file list and SHA-256 values are identical. Do not delete or rebuild files in the real directory.

- [ ] **Step 5: Run the first complete L0/Graph Gate**

```bash
MING_GOPROXY=https://proxy.golang.org,direct \
GOWORK=off go run ./cmd/graphctl ci --repo-root ../..
```

Expected: command exits zero and prints a run ID with G0/L0 Gate results. Record the run ID, L0 total duration, individual Gate durations, and run JSON path. If it fails with an old candidate path, the shared `GOCACHE` design is rejected and implementation stops.

- [ ] **Step 6: Run the same complete Gate again with the now-hot cache**

Run the identical command from Step 5 without cleaning either Go cache.

Expected: PASS. Record the second run ID and timings. Confirm no new per-run `gocache` or `gomodcache` directory exists under `.thingsbaseline-gates-*`; the hot-run experience target is approximately three minutes or less. If correctness passes but the target is missed, report that cache reuse is valid but pre-push latency remains unresolved.

- [ ] **Step 7: Confirm worktree and ignored-output boundaries**

```bash
git status --short
find .. -maxdepth 2 -type d \( -name '.thingsbaseline-cache-*' -o -name '.thingsbaseline-gates-*' \) -print
```

Expected: no unexpected tracked/untracked files. Do not remove the pre-existing `.thingsbaseline-gates-84768608`; list it separately for later user-authorized cleanup.

### Task 6: Independent review and handoff

**Files:**
- Review the complete branch diff; modify only files required by concrete review findings.

- [ ] **Step 1: Request independent code review**

Use `superpowers:requesting-code-review` against the design and this plan. The reviewer must specifically inspect:

- whether `go env` output is bounded and paths are never echoed;
- whether test-result caching is disabled everywhere required;
- whether shared caches survive cleanup while private roots do not;
- whether the old random candidate path failure is truly exercised;
- whether real `backend/things`, `backend/bin`, and `../things` remain untouched;
- whether any automatic cache deletion was introduced.

- [ ] **Step 2: Process findings rigorously**

For each finding, use `superpowers:receiving-code-review`: reproduce or prove the issue before changing code. Rerun the smallest affected test, then the package suite. Do not accept scope expansion into FAST/FULL, Hook changes, cache daemons, or unrelated tools.

- [ ] **Step 3: Run final verification-before-completion checks**

Use `superpowers:verification-before-completion` and rerun at least:

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl validate --repo-root ../..
GOWORK=off go run ./cmd/graphctl scope-check --repo-root ../..
GOWORK=off go test ./cmd/thingsbaseline ./internal/thingsbaseline -count=1
GOWORK=off go vet ./...
git status --short
```

Report fresh outputs plus the two full Gate run IDs. Do not claim the three-minute target if the measured hot run exceeded it.

- [ ] **Step 4: Pause for any final commit/merge/push authorization**

Show the exact remaining changes and branch history. Do not commit fixes, merge, push, delete the 6.3 GiB stale directory, or change `core.hooksPath` without a new explicit user authorization.
