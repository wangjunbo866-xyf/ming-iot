package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func testBackend(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(workingDirectory, "..", "..", ".."))
}

// runApicheck 以子进程方式运行 go run ./cmd/apicheck，返回 combined output。
func runApicheck(t *testing.T, args ...string) (string, int) {
	t.Helper()
	toolsDir := filepath.Join(testBackend(t), "tools")
	cmdArgs := append([]string{"run", "./cmd/apicheck"}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = toolsDir
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "GOWORK=") {
			cmd.Env = append(cmd.Env, item)
		}
	}
	cmd.Env = append(cmd.Env, "GOWORK=off")
	out, err := cmd.CombinedOutput()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run go: %v", err)
	}
	return string(out), code
}

func TestRunApicheckIgnoresInheritedWorkspace(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "go.work")
	if err := os.WriteFile(workspace, []byte("go 1.24.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOWORK", workspace)

	out, code := runApicheck(t, "snapshot", "--backend", "..")
	if code == 0 {
		t.Fatalf("snapshot without --update must exit non-zero\n%s", out)
	}
	if strings.Contains(out, "not one of the workspace modules") {
		t.Fatalf("runApicheck inherited the parent workspace:\n%s", out)
	}
	if !strings.Contains(out, "refusing to write without --update") {
		t.Fatalf("expected apicheck refusal, got:\n%s", out)
	}
}

func TestBackendUsesCurrentCheckout(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Clean(filepath.Join(workingDirectory, "..", "..", ".."))
	if got := testBackend(t); got != want {
		t.Fatalf("testBackend() = %q, want current checkout %q", got, want)
	}
}

func TestCLIVerifyGoldenMatch(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real core tree and built golden")
	}
	if _, err := os.Stat(filepath.Join(testBackend(t), "saas/service/apisvr/http/testdata/definitions.golden.json")); err != nil {
		t.Skipf("golden not generated yet: %v", err)
	}
	out, code := runApicheck(t, "verify", "--backend", "..")
	if code != 0 {
		t.Fatalf("verify exit=%d, want 0\n%s", code, out)
	}
	for _, want := range []string{
		"total: defined=244 reachable=242 registered=242 detached=2",
		"metadata complete=244",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("verify output missing %q\n%s", want, out)
		}
	}
}

func TestCLISnapshotRefusesWithoutUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real core tree")
	}
	out, code := runApicheck(t, "snapshot", "--backend", "..")
	if code == 0 {
		t.Fatalf("snapshot without --update must exit non-zero\n%s", out)
	}
	if !strings.Contains(out, "refusing to write without --update") {
		t.Errorf("expected refusal message, got:\n%s", out)
	}
}

func TestCLIVerifyMetadataNone(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real core tree and built golden")
	}
	if _, err := os.Stat(filepath.Join(testBackend(t), "saas/service/apisvr/http/testdata/definitions.golden.json")); err != nil {
		t.Skipf("golden not generated yet: %v", err)
	}
	out, code := runApicheck(t, "verify", "--backend", "..", "--metadata=none")
	if code != 0 {
		t.Fatalf("verify --metadata=none exit=%d, want 0\n%s", code, out)
	}
	if strings.Contains(out, "metadata complete=") {
		t.Errorf("metadata=none must not print metadata counts\n%s", out)
	}
}

func TestCLILintCleanFile(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real core tree")
	}
	out, code := runApicheck(t, "lint", "--backend", "..",
		"--files", "service/apisvr/http/system/role.api")
	if code != 0 {
		t.Fatalf("lint role.api exit=%d, want 0\n%s", code, out)
	}
	if !strings.Contains(out, "lint: ok") {
		t.Errorf("expected lint ok, got:\n%s", out)
	}
}

// Use the built CLI: go run wraps a child's exit 2 in its own exit 1.
func TestCLILintFileSelection(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "apicheck")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build apicheck: %v\n%s", err, out)
	}
	valid := "service/apisvr/http/system/role.api"
	for _, tc := range []struct {
		name, files string
		code        int
		message     string
	}{
		{"missing", "service/apisvr/http/system/not-a-real-file.api", 2, "--files"},
		{"whitespace", " ,  , ", 2, "--files"},
		{"mixed", valid + ",does-not-exist.api", 2, "--files"},
		{"directory", "service/apisvr/http/system", 2, "--files"},
		{"outside", "../tools/go.mod", 2, "--files"},
		{"absolute", filepath.Join(testBackend(t), "saas", valid), 2, "--files"},
		{"normalized-duplicate", " ./" + valid + "," + valid + ", ", 0, "lint: ok (1 files)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, "lint", "--backend", testBackend(t), "--files", tc.files)
			out, err := cmd.CombinedOutput()
			code := 0
			if err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					code = exit.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if code != tc.code || !strings.Contains(string(out), tc.message) {
				t.Fatalf("exit=%d want=%d, message=%q\n%s", code, tc.code, tc.message, out)
			}
			if tc.code != 0 && strings.Contains(string(out), "lint: ok") {
				t.Fatalf("invalid selection reported success:\n%s", out)
			}
		})
	}
}

func TestCLILintNormalizedSelectionStillChecksMetadata(t *testing.T) {
	core := t.TempDir()
	files := map[string]string{
		"service/apisvr/http/api.api": "syntax = \"v1\"\nimport \"broken.api\"\nimport \"types.api\"\n",
		"service/apisvr/http/broken.api": `syntax = "v1"
@server (
	group: demo
	prefix: /demo
)
service api {
	@doc "缺少完整元数据"
	@handler ping
	get /ping
}
`,
		"service/apisvr/http/types.api":             "syntax = \"v1\"\ntype Reply {\n OK bool\n}\n",
		"service/apisvr/internal/handler/routes.go": "package handler\n",
		"service/apisvr/internal/types/types.go":    "package types\n",
		"outside.api":                               "syntax = \"v1\"\n",
		"service/apisvr/http/testdata/keep.json":    "{\"unchanged\":true}\n",
	}
	for rel, content := range files {
		path := filepath.Join(core, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		selection string
		success   bool
		message   string
	}{
		{"./service/apisvr/http/broken.api", false, "metadata problems"},
		{"service/apisvr/http/../http/broken.api", false, "metadata problems"},
		{"service/apisvr/http/types.api", true, "lint: ok (1 files)"},
		{"outside.api", false, "--files"},
		{"service/apisvr/http/testdata/keep.json", false, "--files"},
	} {
		t.Run(tc.selection, func(t *testing.T) {
			out, code := runApicheck(t, "lint", "--backend", core, "--files", tc.selection)
			if (code == 0) != tc.success || !strings.Contains(out, tc.message) {
				t.Fatalf("exit=%d, success=%v, expected %q\n%s", code, tc.success, tc.message, out)
			}
		})
	}
	for rel, content := range files {
		got, err := os.ReadFile(filepath.Join(core, rel))
		if err != nil || string(got) != content {
			t.Errorf("lint modified input %s: %v", rel, err)
		}
	}
}

// TestThingsCheckedInAccessHasOnlyKnownLegacyGaps freezes the three access
// routes already missing before the model-module/model-release API was added.
// Any newly managed route missing from the checked-in access.json is a
// regression; the historical gaps remain visible instead of being silently
// folded into the current change.
func TestThingsCheckedInAccessHasOnlyKnownLegacyGaps(t *testing.T) {
	backend := testBackend(t)
	goldenData, err := os.ReadFile(filepath.Join(
		backend, "things/service/apisvr/http/testdata/authorization.golden.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	accessData, err := os.ReadFile(filepath.Join(
		backend, "things/service/apisvr/http/access.json",
	))
	if err != nil {
		t.Fatal(err)
	}

	var golden []struct {
		Method          string `json:"method"`
		FullPath        string `json:"fullPath"`
		ManagedByAccess bool   `json:"managedByAccess"`
	}
	if err := json.Unmarshal(goldenData, &golden); err != nil {
		t.Fatalf("parse authorization golden: %v", err)
	}
	var access struct {
		Access map[string]struct {
			APIs []struct {
				Method string `json:"method"`
				Route  string `json:"route"`
			} `json:"apis"`
		} `json:"Access"`
	}
	if err := json.Unmarshal(accessData, &access); err != nil {
		t.Fatalf("parse checked-in access: %v", err)
	}

	registered := make(map[string]bool)
	for _, group := range access.Access {
		for _, api := range group.APIs {
			registered[strings.ToUpper(api.Method)+" "+api.Route] = true
		}
	}
	var missing []string
	for _, route := range golden {
		if !route.ManagedByAccess {
			continue
		}
		key := route.Method + " " + route.FullPath
		if !registered[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	want := []string{
		"POST /api/v1/things/device/secure-control/key-grant",
		"POST /api/v1/things/user/device/share/multi-delete-token",
		"POST /api/v1/things/user/device/share/multi-get-token-list",
	}
	if strings.Join(missing, "\n") != strings.Join(want, "\n") {
		t.Fatalf("checked-in Things access missing routes:\n%s\nwant only known legacy gaps:\n%s",
			strings.Join(missing, "\n"), strings.Join(want, "\n"))
	}
}

// TestCLIVerifyAccessRunsRealGoctl 用固定 goctl 在临时目录生成 access.json，
// 再用 verify-access（默认 operType/集合 门禁）校验，必须 exit 0。
// goctl fork 路径来自 GOCTL_BIN 环境变量；未提供则跳过。
func TestCLIVerifyAccessRunsRealGoctl(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real core tree and goctl binary")
	}
	goctlBin := os.Getenv("GOCTL_BIN")
	if goctlBin == "" {
		goctlBin = filepath.Join(testBackend(t), "bin", "goctl")
	}
	if _, err := os.Stat(goctlBin); err != nil {
		t.Skipf("goctl binary not found at %s: %v", goctlBin, err)
	}
	for _, svc := range []struct{ name, api string }{
		{"apisvr", filepath.Join(testBackend(t), "saas/service/apisvr/http/api.api")},
		{"datasvr", filepath.Join(testBackend(t), "saas/service/datasvr/http/data.api")},
	} {
		t.Run(svc.name, func(t *testing.T) {
			dir := t.TempDir()
			outDir := filepath.Join(dir, "access")
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(goctlBin, "api", "access",
				"--api", svc.api, "--dir", outDir, "--filename", "access.json",
				"--host", "localhost", "--basepath", "/")
			if ac, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("goctl api access %s: %v\n%s", svc.name, err, ac)
			}
			accessJSON := filepath.Join(outDir, "access.json")
			out, code := runApicheck(t, "verify-access", "--backend", "..",
				"--service", svc.name, "--file", accessJSON)
			if code != 0 {
				t.Fatalf("verify-access %s exit=%d, want 0 (default mode)\n%s", svc.name, code, out)
			}
			// operType 必须零 mismatch。
			if strings.Contains(out, "operType-mismatch=0") == false {
				t.Errorf("expected operType-mismatch=0 in\n%s", out)
			}
		})
	}
}
