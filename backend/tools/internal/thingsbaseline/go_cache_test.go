package thingsbaseline

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

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

func TestResolveSharedGoCachePathsForcesLocalToolchain(t *testing.T) {
	root := t.TempDir()
	toolchainPath := filepath.Join(root, "toolchain")
	want := sharedGoCachePaths{
		Build:   filepath.Join(root, "go-build"),
		Modules: filepath.Join(root, "pkg-mod"),
	}
	goBinary := filepath.Join(root, "go")
	writeGoEnvFixture(t, goBinary, want, toolchainPath)
	t.Setenv("GOTOOLCHAIN", "go1.99.0+path")

	got, err := resolveSharedGoCachePaths(context.Background(), goBinary)
	if err != nil {
		t.Fatalf("resolveSharedGoCachePaths() error: %v", err)
	}
	if got != want {
		t.Fatalf("shared cache = %+v, want %+v", got, want)
	}
	toolchain, err := os.ReadFile(toolchainPath)
	if err != nil {
		t.Fatalf("read GOTOOLCHAIN capture: %v", err)
	}
	if got := string(toolchain); got != "local" {
		t.Fatalf("cache discovery GOTOOLCHAIN = %q, want local", got)
	}
}

func TestSharedGoCacheDiscoveryEnvironmentPreservesInputsAndForcesLocalToolchain(t *testing.T) {
	environment := []string{
		"HOME=/host/home",
		"GOENV=/host/goenv",
		"GOCACHE=/host/build",
		"GOMODCACHE=/host/modules",
		"UNRELATED=value",
		"GOTOOLCHAIN=go1.99.0+path",
		"GOTOOLCHAIN=go1.98.0+path",
	}

	got := sharedGoCacheDiscoveryEnvironment(environment)
	values := make(map[string]string, len(got))
	toolchainEntries := 0
	for _, item := range got {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			t.Fatalf("environment entry %q is malformed", item)
		}
		if key == "GOTOOLCHAIN" {
			toolchainEntries++
		}
		values[key] = value
	}
	for key, want := range map[string]string{
		"HOME":       "/host/home",
		"GOENV":      "/host/goenv",
		"GOCACHE":    "/host/build",
		"GOMODCACHE": "/host/modules",
		"UNRELATED":  "value",
	} {
		if got := values[key]; got != want {
			t.Fatalf("environment[%s] = %q, want %q", key, got, want)
		}
	}
	if toolchainEntries != 1 || values["GOTOOLCHAIN"] != "local" {
		t.Fatalf("GOTOOLCHAIN entries = %d, value = %q, want one local entry", toolchainEntries, values["GOTOOLCHAIN"])
	}
}

func TestResolveSharedGoCachePathsFromRetainedFileUsesOriginalExecutable(t *testing.T) {
	root := t.TempDir()
	original := sharedGoCachePaths{
		Build:   filepath.Join(root, "original-build"),
		Modules: filepath.Join(root, "original-mod"),
	}
	replacement := sharedGoCachePaths{
		Build:   filepath.Join(root, "replacement-build"),
		Modules: filepath.Join(root, "replacement-mod"),
	}
	toolchainPath := filepath.Join(root, "toolchain")
	goBinary := filepath.Join(root, "go")
	writeGoEnvFixture(t, goBinary, original, toolchainPath)
	retained, err := os.Open(goBinary)
	if err != nil {
		t.Fatalf("open original executable: %v", err)
	}
	defer retained.Close()
	replacementPath := filepath.Join(root, "replacement")
	writeGoEnvFixture(t, replacementPath, replacement, "")
	if err := os.Rename(replacementPath, goBinary); err != nil {
		t.Fatalf("replace original executable path: %v", err)
	}
	t.Setenv("GOTOOLCHAIN", "go1.99.0+path")

	got, err := resolveSharedGoCachePathsFromRetainedFile(context.Background(), retained)
	if err != nil {
		t.Fatalf("resolveSharedGoCachePathsFromRetainedFile() error: %v", err)
	}
	if got != original {
		t.Fatalf("shared cache = %+v, want original executable output %+v", got, original)
	}
	toolchain, err := os.ReadFile(toolchainPath)
	if err != nil {
		t.Fatalf("read GOTOOLCHAIN capture: %v", err)
	}
	if got := string(toolchain); got != "local" {
		t.Fatalf("retained cache discovery GOTOOLCHAIN = %q, want local", got)
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
		{name: "unclean build", build: filepath.Join(t.TempDir(), "parent") + string(filepath.Separator) + ".." + string(filepath.Separator) + "build", modules: filepath.Join(t.TempDir(), "mod")},
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
	root := t.TempDir()
	pidPath := filepath.Join(root, "producer.pid")
	goBinary := filepath.Join(t.TempDir(), "go")
	script := "#!/bin/sh\nprintf '%s\\n' \"$$\" > " + strconv.Quote(pidPath) + "\nwhile :; do\n  printf '%s' " + strconv.Quote(strings.Repeat("x", 4096)) + "\ndone\n"
	if err := os.WriteFile(goBinary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	_, err := resolveSharedGoCachePaths(ctx, goBinary)
	elapsed := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "outside the allowed size") {
		t.Fatalf("oversized go env error = %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("oversized go env resolution took too long: %v", elapsed)
	}

	pidPayload, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("read producer pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidPayload)))
	if err != nil || pid <= 1 {
		t.Fatalf("producer pid = %q: %v", strings.TrimSpace(string(pidPayload)), err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			t.Fatalf("check producer pid %d: %v", pid, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("producer pid %d still exists after resolver returned", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestParseSharedGoCachePathsRejectsMalformedJSONWithoutEcho(t *testing.T) {
	sensitivePath := filepath.Join(t.TempDir(), "private-cache")
	payload := `{"GOCACHE":"` + sensitivePath + `","GOMODCACHE":`

	_, err := parseSharedGoCachePaths([]byte(payload))
	if err == nil {
		t.Fatal("parseSharedGoCachePaths() accepted malformed JSON")
	}
	if strings.Contains(err.Error(), payload) || strings.Contains(err.Error(), sensitivePath) {
		t.Fatalf("malformed JSON error leaked payload/path: %v", err)
	}
}

func TestResolveSharedGoCachePathsRejectsNonzeroGoEnvWithoutEcho(t *testing.T) {
	sensitivePath := filepath.Join(t.TempDir(), "private-cache")
	stderr := "go env failed: " + sensitivePath
	goBinary := filepath.Join(t.TempDir(), "go")
	script := "#!/bin/sh\nprintf '%s' " + strconv.Quote(stderr) + " >&2\nexit 7\n"
	if err := os.WriteFile(goBinary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := resolveSharedGoCachePaths(context.Background(), goBinary)
	if err == nil {
		t.Fatal("resolveSharedGoCachePaths() accepted nonzero Go env exit")
	}
	if strings.Contains(err.Error(), stderr) || strings.Contains(err.Error(), sensitivePath) {
		t.Fatalf("nonzero Go env error leaked stderr/path: %v", err)
	}
}

func TestSharedGoCacheSurvivesDeletedCandidatePath(t *testing.T) {
	shared := testSharedGoCachePaths(t)
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	var firstPath string
	for run := 1; run <= 2; run++ {
		candidatePath := filepath.Join(t.TempDir(), "candidate")
		if err := os.MkdirAll(candidatePath, 0o700); err != nil {
			t.Fatal(err)
		}
		for name, data := range map[string]string{
			"go.mod":          "module example.com/sharedcachefixture\n\ngo 1.24.4\n",
			"fixture.go":      "package fixture\n\nfunc Value() int { return 42 }\n",
			"fixture_test.go": "package fixture\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) { t.Log(\"EXECUTED-SHARED-CACHE-FIXTURE\"); if Value() != 42 { t.Fatal(Value()) } }\n",
		} {
			if err := os.WriteFile(filepath.Join(candidatePath, name), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
		}

		privateRoot := filepath.Join(t.TempDir(), "private")
		environment, err := fixedCandidateGoEnvironment(privateRoot, shared)
		if err != nil {
			t.Fatal(err)
		}
		command := exec.Command(goBinary, "test", "-count=1", "-v", "./...")
		command.Dir = candidatePath
		command.Env = environment
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("run %d go test: %v\n%s", run, err, output)
		}
		if strings.Contains(string(output), "(cached)") {
			t.Fatalf("run %d unexpectedly used cached test output:\n%s", run, output)
		}
		if !strings.Contains(string(output), "EXECUTED-SHARED-CACHE-FIXTURE") {
			t.Fatalf("run %d did not execute fixture:\n%s", run, output)
		}
		if firstPath != "" && strings.Contains(string(output), firstPath) {
			t.Fatalf("run %d output referenced deleted candidate %q:\n%s", run, firstPath, output)
		}
		if run == 1 {
			firstPath = candidatePath
			if err := os.RemoveAll(firstPath); err != nil {
				t.Fatalf("delete candidate A: %v", err)
			}
		}
	}
}

func TestExecGoRunnerRedactsSharedGoCachesFromCommandReport(t *testing.T) {
	privateRoot := t.TempDir()
	shared := testSharedGoCachePaths(t)
	environment, err := fixedCandidateGoEnvironment(privateRoot, shared)
	if err != nil {
		t.Fatalf("fixedCandidateGoEnvironment() error: %v", err)
	}
	command := GoCommand{
		Path: "/bin/sh",
		Dir:  t.TempDir(),
		Args: []string{"-c", `printf '%s\n' "$GOCACHE" "$GOMODCACHE"; exit 31`},
		Env:  environment,
	}
	report, err := (execGoRunner{}).Run(context.Background(), command)
	if err == nil {
		t.Fatal("execGoRunner.Run() succeeded, want exit 31")
	}
	if report.ExitCode != 31 {
		t.Fatalf("execGoRunner.Run() report exit code = %d, want 31", report.ExitCode)
	}
	encoded, marshalErr := json.Marshal(report)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if !strings.Contains(string(encoded), "[REDACTED]") {
		t.Fatalf("command report did not contain redaction: %s", encoded)
	}
	for _, cachePath := range []string{shared.Build, shared.Modules} {
		if strings.Contains(string(encoded), cachePath) || strings.Contains(err.Error(), cachePath) {
			t.Fatalf("shared cache path leaked in report/error: %q\nreport=%s\nerror=%v", cachePath, encoded, err)
		}
	}
}

func writeGoEnvFixture(t *testing.T, path string, cache sharedGoCachePaths, toolchainPath string) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"GOCACHE":    cache.Build,
		"GOMODCACHE": cache.Modules,
	})
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n"
	if toolchainPath != "" {
		script += "printf '%s' \"$GOTOOLCHAIN\" > " + strconv.Quote(toolchainPath) + "\n"
	}
	script += "printf '%s' " + strconv.Quote(string(payload)) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}
