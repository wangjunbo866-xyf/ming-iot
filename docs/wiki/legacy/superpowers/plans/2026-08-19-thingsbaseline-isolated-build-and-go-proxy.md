# Thingsbaseline Isolated Build and Go Proxy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `thingsbaseline` build into a private gate directory, support the repository-level `MING_GOPROXY` contract without bypassing `HTTP(S)_PROXY`, and keep inherited transport-proxy configuration out of diagnostic summaries and errors.

**Architecture:** Keep the implementation owned by `internal/thingsbaseline` until a second Loop needs the contract. Resolve and validate `MING_GOPROXY` before creating controlled Go commands, while preserving the existing fixed-environment boundary. Treat every non-empty value of the four supported transport-proxy keys as an exact redaction literal for direct Go commands and default verification gates, including the strict-Make environment. Reuse the existing `.thingsbaseline-gates-*` lifecycle for physical build outputs and retain `backend/bin/*` only as Evidence logical paths.

**Tech Stack:** Go 1.25.4, standard library `net/url`, existing `thingsbaseline` gate runner, Go tests, `graphctl`.

---

## File map

- `backend/tools/internal/thingsbaseline/render.go`: resolve, validate, and inject `MING_GOPROXY`, and extract all four inherited transport-proxy values for controlled-command redaction.
- `backend/tools/internal/thingsbaseline/render_test.go`: prove override precedence, transport-proxy inheritance, default behavior, malformed-escape rejection, and secret-safe errors.
- `backend/tools/internal/thingsbaseline/runner_test.go`: prove direct Go-command stdout, stderr, and returned errors cannot expose complete inherited transport-proxy values or sensitive path/query fragments.
- `backend/tools/internal/thingsbaseline/verify.go`: place `things-build` physical outputs under the private gate root, preserve transport proxies in strict Make, and add their complete values to every default gate's `RedactValues`.
- `backend/tools/internal/thingsbaseline/verify_test.go`: prove local `backend/bin` remains untouched, assert the build output contract before dereference, and verify default Go/strict-Make gates propagate and redact all four transport proxies.
- `backend/tools/README.md`: document the repository-level configuration contract and its distinction from `HTTP(S)_PROXY`.
- `.graph/loops/L0-things-baseline.yaml`: register this design and plan as exact L0-owned documentation paths.

### Task 0: Register the design and plan with L0 ownership

**Files:**
- Modify: `.graph/loops/L0-things-baseline.yaml:18-24`
- Create: `docs/superpowers/plans/2026-08-19-thingsbaseline-isolated-build-and-go-proxy.md`

- [ ] **Step 1: Verify the ownership check is RED**

Run from `backend/tools` before changing the Loop declaration:

```bash
GOWORK=off go run ./cmd/graphctl context --repo-root ../.. \
  --path docs/superpowers/specs/2026-08-19-thingsbaseline-isolated-build-and-go-proxy-design.md \
  --path docs/superpowers/plans/2026-08-19-thingsbaseline-isolated-build-and-go-proxy.md \
  --format=json
```

Expected: exit non-zero with typed `UNOWNED_PATH`.

- [ ] **Step 2: Add only the two exact ownership paths**

Add these entries to `spec.scope.owns`; do not add a broad documentation glob:

```yaml
- docs/superpowers/specs/2026-08-19-thingsbaseline-isolated-build-and-go-proxy-design.md
- docs/superpowers/plans/2026-08-19-thingsbaseline-isolated-build-and-go-proxy.md
```

- [ ] **Step 3: Validate the Registry and verify ownership is GREEN**

Run:

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl validate --repo-root ../..
GOWORK=off go run ./cmd/graphctl context --repo-root ../.. \
  --path docs/superpowers/specs/2026-08-19-thingsbaseline-isolated-build-and-go-proxy-design.md \
  --path docs/superpowers/plans/2026-08-19-thingsbaseline-isolated-build-and-go-proxy.md \
  --format=json
GOWORK=off go run ./cmd/graphctl context --repo-root ../.. --format=json
```

Expected: Registry valid; both documentation paths have owner `L0-things-baseline`; diff mode reports the Loop YAML owner `G0-rd-graph-control-plane`, watcher `L0-things-baseline`, and both Loops as affected.

- [ ] **Step 4: Commit Task 0**

```bash
git add .graph/loops/L0-things-baseline.yaml docs/superpowers/plans/2026-08-19-thingsbaseline-isolated-build-and-go-proxy.md
git commit -m "chore(graph): register thingsbaseline gate fix"
```

### Task 1: Add the controlled `MING_GOPROXY` contract

**Files:**
- Modify: `backend/tools/internal/thingsbaseline/render.go:1-25,633-669`
- Test: `backend/tools/internal/thingsbaseline/render_test.go:455-525`

- [ ] **Step 1: Write failing override and safety tests**

At the start of `TestRenderCandidateUsesFixedEnvironmentAndTwoRendersAreIdentical`, pin the empty override so the existing test remains the default-value proof:

```go
t.Setenv("MING_GOPROXY", "")
```

Add these tests immediately after it:

```go
func TestFixedCandidateGoEnvironmentUsesMingGoProxyAndPreservesTransportProxy(t *testing.T) {
	t.Setenv("GOPROXY", "https://hostile.example.invalid")
	t.Setenv("MING_GOPROXY", "https://proxy.golang.org,direct")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:7890")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:7890")
	t.Setenv("http_proxy", "http://127.0.0.1:7891")
	t.Setenv("https_proxy", "http://127.0.0.1:7891")

	environment, err := fixedCandidateGoEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	values := environmentMap(environment)
	if got := values["GOPROXY"]; got != "https://proxy.golang.org,direct" {
		t.Fatalf("GOPROXY = %q", got)
	}
	for key, want := range map[string]string{
		"HTTP_PROXY":  "http://127.0.0.1:7890",
		"HTTPS_PROXY": "http://127.0.0.1:7890",
		"http_proxy":  "http://127.0.0.1:7891",
		"https_proxy": "http://127.0.0.1:7891",
	} {
		if got := values[key]; got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	if _, exists := values["MING_GOPROXY"]; exists {
		t.Fatalf("controlled child environment leaked MING_GOPROXY: %q", environment)
	}
}

func TestFixedCandidateGoEnvironmentRejectsUnsafeMingGoProxyWithoutEchoingValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "plain http", value: "http://proxy.example.invalid"},
		{name: "credentials", value: "https://user:secret@proxy.example.invalid"},
		{name: "query", value: "https://proxy.example.invalid?token=secret"},
		{name: "empty query", value: "https://proxy.example.invalid?"},
		{name: "empty fragment", value: "https://proxy.example.invalid#"},
		{name: "empty entry", value: "https://proxy.example.invalid|"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MING_GOPROXY", tt.value)
			_, err := fixedCandidateGoEnvironment(t.TempDir())
			if err == nil {
				t.Fatal("unsafe MING_GOPROXY was accepted")
			}
			if strings.Contains(err.Error(), tt.value) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("error leaked configured value: %v", err)
			}
		})
	}
}

func TestResolveCandidateGoProxyAcceptsSupportedEntries(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty uses default", want: "https://goproxy.cn,direct"},
		{name: "direct", value: "direct", want: "direct"},
		{name: "off", value: "off", want: "off"},
		{name: "comma chain", value: "https://proxy.golang.org,direct", want: "https://proxy.golang.org,direct"},
		{name: "pipe chain", value: "https://proxy.golang.org|direct", want: "https://proxy.golang.org|direct"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MING_GOPROXY", tt.value)
			got, err := resolveCandidateGoProxy()
			if err != nil || got != tt.want {
				t.Fatalf("resolveCandidateGoProxy() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline -run 'Test(FixedCandidateGoEnvironment|ResolveCandidateGoProxy)' -count=1
```

Expected: FAIL because the current implementation fixes `GOPROXY` to goproxy.cn, leaks `MING_GOPROXY` into the child environment, and accepts unsafe overrides.

- [ ] **Step 3: Implement the minimum resolver and validator**

Add `net/url` to the imports and add these constants and helpers in `render.go`:

```go
const (
	defaultCandidateGoProxy = "https://goproxy.cn,direct"
	mingGoProxyEnvironment  = "MING_GOPROXY"
)

func resolveCandidateGoProxy() (string, error) {
	value := os.Getenv(mingGoProxyEnvironment)
	if value == "" {
		return defaultCandidateGoProxy, nil
	}
	if err := validateCandidateGoProxy(value); err != nil {
		return "", err
	}
	return value, nil
}

func validateCandidateGoProxy(value string) error {
	start, entryIndex := 0, 1
	for index := 0; index <= len(value); index++ {
		if index < len(value) && value[index] != ',' && value[index] != '|' {
			continue
		}
		entry := value[start:index]
		if entry == "" {
			return fmt.Errorf("MING_GOPROXY entry %d is empty", entryIndex)
		}
		if entry != "direct" && entry != "off" {
			parsed, err := url.Parse(entry)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.Contains(entry, "#") {
				return fmt.Errorf("MING_GOPROXY entry %d must be direct, off, or an HTTPS URL without credentials, query, or fragment", entryIndex)
			}
		}
		start = index + 1
		entryIndex++
	}
	return nil
}
```

At the start of `fixedCandidateGoEnvironment`, resolve the value:

```go
goProxy, err := resolveCandidateGoProxy()
if err != nil {
	return nil, err
}
```

Filter the repository-level input from inherited child variables:

```go
if !ok || strings.HasPrefix(key, "GO") || strings.HasPrefix(key, "GIT_") || key == mingGoProxyEnvironment || key == "HOME" || key == "LC_ALL" || key == "LANG" {
	continue
}
```

Replace the fixed entry with:

```go
"GOPROXY=" + goProxy, "GOSUMDB=sum.golang.org",
```

- [ ] **Step 4: Run focused and package tests and verify GREEN**

Run:

```bash
cd backend/tools
gofmt -w internal/thingsbaseline/render.go internal/thingsbaseline/render_test.go
GOWORK=off go test ./internal/thingsbaseline -run 'TestFixedCandidateGoEnvironment|TestRenderCandidateUsesFixedEnvironment' -count=1
GOWORK=off go test ./internal/thingsbaseline -count=1
```

Expected: both commands PASS; the existing hostile global `GOPROXY` test still observes the default goproxy.cn value.

- [ ] **Step 5: Commit Task 1**

```bash
git add backend/tools/internal/thingsbaseline/render.go backend/tools/internal/thingsbaseline/render_test.go
git commit -m "fix(tools): add controlled Ming Go proxy input"
```

### Task 2: Isolate `things-build` physical outputs

**Files:**
- Modify: `backend/tools/internal/thingsbaseline/verify.go:1690-1742`
- Test: `backend/tools/internal/thingsbaseline/verify_test.go:1128-1185`

- [ ] **Step 1: Change the existing gate specification test to require isolation**

In `TestDefaultGateSpecificationsBindIsolationPinBuildToolchainAndOwnOutputs`, create a local ignored output before building specifications:

```go
localBin := filepath.Join(backend, "bin")
if err := os.MkdirAll(localBin, 0o755); err != nil {
	t.Fatal(err)
}
localBinary := filepath.Join(localBin, "coresvr")
if err := os.WriteFile(localBinary, []byte("local-only"), 0o755); err != nil {
	t.Fatal(err)
}
```

Replace `defer cleanup()` with a guarded cleanup so the test can invoke it explicitly after inspecting the generated workspace:

```go
cleaned := false
defer func() {
	if !cleaned {
		cleanup()
	}
}()
```

Replace the current `backend/bin` physical assertions with:

```go
buildOutput := build.outputContract.directory
cacheRoot := filepath.Dir(buildOutput)
if buildOutput == localBin || filepath.Base(buildOutput) != "build-output" || filepath.Dir(cacheRoot) != backend || !strings.HasPrefix(filepath.Base(cacheRoot), ".thingsbaseline-gates-") {
	t.Fatalf("build output is not private: %q", buildOutput)
}
if !slices.Contains(build.Args, "GO=/proc/self/fd/3") || !slices.Contains(build.Args, "BIN_DIR="+buildOutput) || build.Args[len(build.Args)-1] != "build" {
	t.Fatalf("build gate does not pin verified Go/private BIN_DIR: %q", build.Args)
}
if build.outputContract.logicalRoot != "backend/bin" || !reflect.DeepEqual(build.outputContract.names, []string{"thingsvr", "dgsvr", "dmsvr"}) || build.outputContract.mode != 0o755 {
	t.Fatalf("build output contract = %#v", build.outputContract)
}
```

After the existing temporary-workspace assertions, exercise cleanup and prove it removes only the private gate root:

```go
cleanup()
cleaned = true
if _, err := os.Stat(cacheRoot); !os.IsNotExist(err) {
	t.Fatalf("private gate cache still exists after cleanup: %v", err)
}
if content, err := os.ReadFile(localBinary); err != nil || string(content) != "local-only" {
	t.Fatalf("local backend/bin changed: %q, %v", content, err)
}
```

- [ ] **Step 2: Run the test and verify RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline -run TestDefaultGateSpecificationsBindIsolationPinBuildToolchainAndOwnOutputs -count=1
```

Expected: FAIL because `build.outputContract.directory` is still the shared `backend/bin`.

- [ ] **Step 3: Point build and validation at the private gate root**

In `buildDefaultVerificationGates`, define:

```go
buildOutput := filepath.Join(cacheRoot, "build-output")
```

Use it for both the Make argument and physical contract while retaining the logical root:

```go
{
	ID: "things-build", DisplayCommand: "cd verification-tree && make BIN_DIR=backend/bin build", Path: makePath,
	Args: []string{"GO=/proc/self/fd/3", "BIN_DIR=" + buildOutput, "build"}, Dir: isolation.Path(), Env: makeEnvironment, RedactValues: redactions,
	goTool: dependencies.goTool, outputContract: &gateOutputContract{directory: buildOutput, logicalRoot: "backend/bin", names: []string{"thingsvr", "dgsvr", "dmsvr"}, mode: 0o755},
},
```

- [ ] **Step 4: Run focused and package tests and verify GREEN**

Run:

```bash
cd backend/tools
gofmt -w internal/thingsbaseline/verify.go internal/thingsbaseline/verify_test.go
GOWORK=off go test ./internal/thingsbaseline -run 'TestDefaultGateSpecifications|TestExecVerificationGateRunner' -count=1
GOWORK=off go test ./internal/thingsbaseline -count=1
```

Expected: PASS; the exact three-file contract still runs against the private directory.

- [ ] **Step 5: Commit Task 2**

```bash
git add backend/tools/internal/thingsbaseline/verify.go backend/tools/internal/thingsbaseline/verify_test.go
git commit -m "fix(tools): isolate thingsbaseline build outputs"
```

### Task 3: Document the repository-level proxy contract

**Files:**
- Modify: `backend/tools/README.md:6-33`

- [ ] **Step 1: Add operator documentation**

After the `thingsbaseline verify` example, add:

````markdown
需要切换 Go 模块镜像时，使用 Ming 工具链公共输入；它不会替代系统传输代理：

```bash
MING_GOPROXY=https://proxy.golang.org,direct \
  go run ./cmd/thingsbaseline verify \
  --lock ./things-baseline.lock \
  --source /home/wangjunbo/workspace-iot-platform/things \
  --backend .. \
  --evidence /tmp/l0-things-evidence.json
```

`HTTP_PROXY`、`HTTPS_PROXY` 及小写形式仍会传给受控 Go 子进程；全局 `GOPROXY`
仍被隔离。`MING_GOPROXY` 只用于仓库工具，不配置业务服务运行时。构建使用私有
`.thingsbaseline-gates-*` 目录，仓库已有的 `backend/bin/` 不会被读取、覆盖或删除。
````

- [ ] **Step 2: Format and verify documentation diff**

Run:

```bash
git diff --check
```

Expected: exit 0 with no whitespace errors.

- [ ] **Step 3: Commit Task 3**

```bash
git add backend/tools/README.md
git commit -m "docs(tools): explain Ming Go proxy contract"
```

### Task 4: Redact inherited transport-proxy diagnostics

**Files:**
- Modify: `backend/tools/internal/thingsbaseline/render.go`
- Test: `backend/tools/internal/thingsbaseline/runner_test.go`
- Modify: `backend/tools/internal/thingsbaseline/verify.go`
- Test: `backend/tools/internal/thingsbaseline/verify_test.go`
- Test: `backend/tools/internal/thingsbaseline/render_test.go`

- [ ] **Step 1: Prove direct and default-gate leakage is RED**

Echo all four supported proxy variables with scheme-less and sensitive path/query sentinels. Require direct `execGoRunner` diagnostics and default verification-gate summaries/errors to omit both complete values and sensitive fragments. Require every generated `GateSpecification.RedactValues`, including `things-build`, to contain the complete non-empty values.

- [ ] **Step 2: Add one shared exact-key redaction helper**

Extract non-empty values only for `HTTP_PROXY`, `HTTPS_PROXY`, `http_proxy`, and `https_proxy`. Reuse the helper in `commandEnvironmentValues` and `buildDefaultVerificationGates`, and reuse the exact key list when constructing the strict-Make environment. Preserve proxy inheritance and existing `GOPROXY` behavior.

- [ ] **Step 3: Verify focused GREEN and test-quality cases**

Run the focused leakage tests, assert `build.outputContract` is non-nil before dereferencing it, and include a malformed URL escape that directly exercises `url.Parse` failure without echoing `MING_GOPROXY`.

### Task 5: Verify the branch and hand it to final integration

**Files:**
- Verify only; no new production files.

- [ ] **Step 1: Run fresh package verification**

Run:

```bash
cd backend/tools
GOWORK=off go test ./cmd/thingsbaseline ./internal/thingsbaseline -count=1
GOWORK=off go test ./cmd/graphctl ./internal/graphctl/... -count=1
```

Expected: both commands PASS.

- [ ] **Step 2: Prove Graph ownership**

Run:

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl context --repo-root ../..
GOWORK=off go run ./cmd/graphctl scope-check --repo-root ../..
```

Expected: no `UNOWNED_PATH` or `AMBIGUOUS_OWNER`. Thingsbaseline implementation and its two exact design/plan paths are owned by `L0-things-baseline`; the Loop YAML and shared `backend/tools/README.md` are owned by `G0-rd-graph-control-plane` and watched by L0, so the affected set is G0 plus L0.

- [ ] **Step 3: Inspect the final branch state**

Run:

```bash
git status --short --branch
git log --oneline --decorate -8
```

Expected: clean feature worktree and only the planned commits after `b22dea6`.

- [ ] **Step 4: Merge locally, then run the real push tuple gate from main**

Use `superpowers:finishing-a-development-branch` to fast-forward local `master`. In the main worktree, first verify that the unrelated untracked design document remains unstaged, refresh the remote-tracking ref, and merge the feature branch:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git status --short --branch
git fetch origin master
git merge --ff-only fix/thingsbaseline-isolated-gates
```

Expected: `master` advances by fast-forward; `docs/superpowers/specs/2026-08-18-graph-dashboard-systemd-service-switch-design.md` remains untracked and unstaged.

Capture the ignored binary inventory, then perform one ordinary push. The installed pre-push hook receives the real ref tuple and runs `graphctl ci`; setting `MING_GOPROXY` on `git push` makes it available to that hook without bypassing `HTTP(S)_PROXY`:

```bash
ming_bin_snapshot="$(mktemp)"
trap 'rm -f "$ming_bin_snapshot"' EXIT
find backend/bin -maxdepth 1 -type f -exec sha256sum {} + | LC_ALL=C sort >"$ming_bin_snapshot"
MING_GOPROXY=https://proxy.golang.org,direct git push origin master
find backend/bin -maxdepth 1 -type f -exec sha256sum {} + | LC_ALL=C sort | cmp - "$ming_bin_snapshot"
git fetch origin master
test "$(git rev-parse HEAD)" = "$(git rev-parse refs/remotes/origin/master)"
git status --short --branch
```

Expected: the hook reports `G0-rd-graph-control-plane` and `L0-things-baseline` PASS, `git push` succeeds without `--no-verify`, `cmp` exits 0, local `HEAD` equals `origin/master`, and the unrelated untracked design document remains present and unstaged.
