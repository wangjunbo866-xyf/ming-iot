package main

import (
	"bytes"
	"debug/elf"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ming-iot/backend/tools/internal/thingsstartbaseline"
)

func TestRunParsesSevenCommandsWithAbsolutePaths(t *testing.T) {
	contract := relativeContractPath(t)
	temp := t.TempDir()
	env := filepath.Join(temp, "startup.env")
	if err := os.WriteFile(env, []byte("PASSWORD=test\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	paths := map[string]string{
		"contract":     contract,
		"repo":         filepath.Join(temp, "repo"),
		"bin-dir":      filepath.Join(temp, "bin"),
		"output":       filepath.Join(temp, "manifest.yaml"),
		"env-file":     env,
		"runtime-root": filepath.Join(temp, "runtime"),
		"compose-file": filepath.Join(temp, "compose.yaml"),
	}
	tests := []struct {
		command         string
		flags           []string
		want            []string
		wantValidations int
		wantExecutions  int
		wantLastStep    string
	}{
		{"contract", []string{"--contract", paths["contract"], "--env-file", paths["env-file"]}, []string{"contract", "env-file"}, 1, 0, "contract"},
		{"manifest", []string{"--contract", paths["contract"], "--repo", paths["repo"], "--bin-dir", paths["bin-dir"], "--output", paths["output"]}, []string{"contract", "repo", "bin-dir", "output"}, 0, 1, "manifest"},
		{"stage", []string{"--contract", paths["contract"], "--repo", paths["repo"], "--env-file", paths["env-file"], "--runtime-root", paths["runtime-root"]}, []string{"contract", "repo", "env-file", "runtime-root"}, 0, 1, "stage"},
		{"image", []string{"--contract", paths["contract"], "--repo", paths["repo"], "--runtime-root", paths["runtime-root"]}, []string{"contract", "repo", "runtime-root"}, 0, 1, "image"},
		{"up", []string{"--contract", paths["contract"], "--repo", paths["repo"], "--env-file", paths["env-file"], "--compose-file", paths["compose-file"], "--runtime-root", paths["runtime-root"]}, []string{"contract", "repo", "env-file", "compose-file", "runtime-root"}, 1, 6, "verify"},
		{"verify", []string{"--contract", paths["contract"], "--repo", paths["repo"], "--env-file", paths["env-file"], "--compose-file", paths["compose-file"], "--runtime-root", paths["runtime-root"]}, []string{"contract", "repo", "env-file", "compose-file", "runtime-root"}, 1, 1, "verify"},
		{"logs", []string{"--contract", paths["contract"], "--repo", paths["repo"], "--env-file", paths["env-file"], "--compose-file", paths["compose-file"], "--runtime-root", paths["runtime-root"]}, []string{"contract", "repo", "env-file", "compose-file", "runtime-root"}, 0, 1, "logs"},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			executor := &recordingExecutor{}
			args := append([]string{tt.command}, tt.flags...)
			if code := run(args, executor, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
				t.Fatalf("run() exit = %d", code)
			}
			if executor.validationCalls != tt.wantValidations || executor.executeCalls != tt.wantExecutions || executor.command != tt.wantLastStep {
				t.Fatalf("executor = validations %d executions %d command %q", executor.validationCalls, executor.executeCalls, executor.command)
			}
			got := executor.options.pathMap()
			for _, name := range tt.want {
				if got[name] == "" || !filepath.IsAbs(got[name]) {
					t.Fatalf("%s path = %q, want absolute", name, got[name])
				}
			}
		})
	}
}

func TestRunRejectsInvalidArguments(t *testing.T) {
	contract := absoluteContractPath(t)
	temp := t.TempDir()
	env := filepath.Join(temp, "startup.env")
	if err := os.WriteFile(env, []byte("PASSWORD=test\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	validUp := []string{"up", "--contract", contract, "--repo", filepath.Join(temp, "repo"), "--env-file", env, "--compose-file", filepath.Join(temp, "compose.yaml"), "--runtime-root", filepath.Join(temp, "runtime")}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing command", nil, "usage"},
		{"unknown command", []string{"wat"}, "usage"},
		{"missing required", []string{"stage", "--contract", contract, "--repo", "repo", "--env-file", env}, "--runtime-root"},
		{"unknown flag", append(append([]string{}, validUp...), "--wat", "x"), "unknown parameter"},
		{"duplicate flag", append(append([]string{}, validUp...), "--repo", "again"), "duplicate parameter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &recordingExecutor{}
			var stderr bytes.Buffer
			if code := run(tt.args, executor, &bytes.Buffer{}, &stderr); code != 2 {
				t.Fatalf("run() exit = %d, want 2; stderr=%q", code, stderr.String())
			}
			if executor.totalCalls() != 0 || !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("executor calls/stderr = %d/%q", executor.totalCalls(), stderr.String())
			}
		})
	}
}

func TestRunRejectsResetCommand(t *testing.T) {
	executor := &recordingExecutor{}
	var stderr bytes.Buffer
	if code := run([]string{"reset"}, executor, &bytes.Buffer{}, &stderr); code != 2 {
		t.Fatalf("run(reset) exit = %d, want 2; stderr=%q", code, stderr.String())
	}
	if executor.totalCalls() != 0 || !strings.Contains(stderr.String(), "usage") {
		t.Fatalf("executor calls/stderr = %d/%q, want no execution and usage", executor.totalCalls(), stderr.String())
	}
}

func TestDeploymentMakefileAndDocumentationContract(t *testing.T) {
	root := repositoryRootPath(t)
	makefile := readContractFile(t, filepath.Join(root, "deploy", "Makefile"))
	for _, required := range []string{
		"SHELL := /usr/bin/env bash",
		"ROOT := $(abspath ..)",
		"PROFILE := $(ROOT)/deploy/profiles/things-baseline",
		"ENV_FILE ?= $(PROFILE)/.env.local",
		"EXAMPLE_ENV := $(PROFILE)/.env.example",
		"CONTRACT := $(PROFILE)/contract.yaml",
		"TOOLS_DIR := $(ROOT)/backend/tools",
		"BASELINE := cd $(TOOLS_DIR) && GOWORK=off go run ./cmd/thingsstartbaseline",
		"COMPOSE := THINGS_BASELINE_ENV_FILE=$(ENV_FILE) docker compose --project-directory $(PROFILE) --env-file $(ENV_FILE) -f $(PROFILE)/compose.yaml",
		"$(MAKE) -C $(ROOT)/backend/saas build.baseline",
		"$(MAKE) -C $(ROOT)/backend/things build.baseline",
		"$(BASELINE) manifest --contract $(CONTRACT) --repo $(ROOT) --bin-dir $(ROOT)/backend/bin --output $(ROOT)/deploy/.runtime/build-manifest.json",
		"$(BASELINE) stage --contract $(CONTRACT) --repo $(ROOT) --env-file $(ENV_FILE) --runtime-root $(ROOT)/deploy/.runtime",
		"$(BASELINE) image --contract $(CONTRACT) --repo $(ROOT) --runtime-root $(ROOT)/deploy/.runtime",
		"$(BASELINE) up --contract $(CONTRACT) --repo $(ROOT) --env-file $(ENV_FILE) --compose-file $(PROFILE)/compose.yaml --runtime-root $(ROOT)/deploy/.runtime",
		"$(BASELINE) verify --contract $(CONTRACT) --repo $(ROOT) --env-file $(ENV_FILE) --compose-file $(PROFILE)/compose.yaml --runtime-root $(ROOT)/deploy/.runtime",
		"$(BASELINE) logs --contract $(CONTRACT) --repo $(ROOT) --env-file $(ENV_FILE) --compose-file $(PROFILE)/compose.yaml --runtime-root $(ROOT)/deploy/.runtime",
	} {
		if !strings.Contains(makefile, required) {
			t.Errorf("deploy/Makefile missing fixed entry %q", required)
		}
	}
	for _, target := range []string{
		"things-baseline-build",
		"things-baseline-image",
		"things-baseline-up",
		"things-baseline-verify",
		"things-baseline-logs",
		"things-baseline-down",
		"things-baseline-fast",
	} {
		if !strings.Contains(makefile, "\n"+target+":") {
			t.Errorf("deploy/Makefile missing target %q", target)
		}
	}
	for _, dependency := range []string{
		"things-baseline-image: things-baseline-build",
		"things-baseline-up: things-baseline-image",
		"things-baseline-fast: things-baseline-build",
		"things-baseline-verify:\n",
		"things-baseline-logs:\n",
		"things-baseline-down:\n",
	} {
		if !strings.Contains(makefile, "\n"+dependency) {
			t.Errorf("deploy/Makefile missing exact dependency contract %q", dependency)
		}
	}
	if strings.Contains(makefile, "things-baseline-reset") {
		t.Error("deploy/Makefile must not define a reset target")
	}
	downRecipe := makeTargetBlock(t, makefile, "things-baseline-down")
	if !strings.Contains(downRecipe, "$(COMPOSE) down") {
		t.Errorf("things-baseline-down recipe = %q, want Compose down", downRecipe)
	}
	for _, forbidden := range []string{"--volumes", "DROP DATABASE", "DROP ROLE", "TRUNCATE"} {
		if strings.Contains(downRecipe, forbidden) {
			t.Errorf("things-baseline-down must not contain %q", forbidden)
		}
	}
	fastRecipe := makeTargetBlock(t, makefile, "things-baseline-fast")
	for _, forbidden := range []string{"$(BASELINE) up", "$(BASELINE) image", "$(BASELINE) stage", "$(COMPOSE) up", "docker compose up", "docker compose run", "docker compose restart"} {
		if strings.Contains(fastRecipe, forbidden) {
			t.Errorf("things-baseline-fast must not contain %q", forbidden)
		}
	}

	deployReadme := readContractFile(t, filepath.Join(root, "deploy", "README.md"))
	for _, required := range []string{
		"make -C deploy things-baseline-*",
		"deploy/docker-compose.yml",
		"deploy/saas/Dockerfile",
		"install -m 0600 .env.example .env.local",
		"down 保留数据库和数据卷",
		"reset 不属于默认命令",
	} {
		if !strings.Contains(deployReadme, required) {
			t.Errorf("deploy/README.md missing contract %q", required)
		}
	}

	toolsReadme := readContractFile(t, filepath.Join(root, "backend", "tools", "README.md"))
	for _, command := range []string{"contract", "manifest", "stage", "image", "up", "verify", "logs"} {
		if !strings.Contains(toolsReadme, "`"+command+"`") {
			t.Errorf("backend/tools/README.md missing command %q", command)
		}
	}
	for _, required := range []string{"七个子命令", "up/verify", "统一产生 Evidence", "脱敏日志"} {
		if !strings.Contains(toolsReadme, required) {
			t.Errorf("backend/tools/README.md missing workflow contract %q", required)
		}
	}
}

func TestRunExecutesBoundedCommandWorkflows(t *testing.T) {
	contract := absoluteContractPath(t)
	temp := t.TempDir()
	env := filepath.Join(temp, "startup.env")
	if err := os.WriteFile(env, []byte("PASSWORD=test\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	common := []string{
		"--contract", contract,
		"--repo", filepath.Join(temp, "repo"),
		"--env-file", env,
		"--compose-file", filepath.Join(temp, "compose.yaml"),
		"--runtime-root", filepath.Join(temp, "runtime"),
	}
	tests := []struct {
		command string
		want    []string
	}{
		{
			command: "up",
			want:    []string{"contract", "manifest", "stage", "database", "infra", "roles", "verify"},
		},
		{
			command: "verify",
			want:    []string{"contract", "verify"},
		},
		{
			command: "logs",
			want:    []string{"logs"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			executor := &recordingExecutor{}
			var stderr bytes.Buffer
			args := append([]string{tt.command}, common...)
			if code := run(args, executor, &bytes.Buffer{}, &stderr); code != 0 {
				t.Fatalf("run() exit = %d, stderr=%q", code, stderr.String())
			}
			if got, want := strings.Join(executor.steps, ","), strings.Join(tt.want, ","); got != want {
				t.Fatalf("workflow = %q, want %q", got, want)
			}
		})
	}
}

func TestRunStopsCommandWorkflowAtFirstFailure(t *testing.T) {
	contract := absoluteContractPath(t)
	temp := t.TempDir()
	env := filepath.Join(temp, "startup.env")
	if err := os.WriteFile(env, []byte("PASSWORD=test\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	executor := &recordingExecutor{
		failStep:   "infra",
		executeErr: errors.New("fixture infra failure"),
	}
	var stderr bytes.Buffer
	code := run([]string{
		"up",
		"--contract", contract,
		"--repo", filepath.Join(temp, "repo"),
		"--env-file", env,
		"--compose-file", filepath.Join(temp, "compose.yaml"),
		"--runtime-root", filepath.Join(temp, "runtime"),
	}, executor, &bytes.Buffer{}, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "fixture infra failure") {
		t.Fatalf("run() exit/stderr = %d/%q", code, stderr.String())
	}
	if got, want := strings.Join(executor.steps, ","), "contract,manifest,stage,database,infra"; got != want {
		t.Fatalf("workflow before failure = %q, want %q", got, want)
	}
}

func TestRunReportsContractMismatchForProtectionFailure(t *testing.T) {
	contract := absoluteContractPath(t)
	envFile := filepath.Join(t.TempDir(), "startup.env")
	if err := os.WriteFile(envFile, []byte("THINGS_BASELINE_DB=iThings\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &recordingExecutor{validationErr: errors.New("protected database target")}
	var stderr bytes.Buffer
	code := run([]string{"contract", "--contract", contract, "--env-file", envFile}, executor, &bytes.Buffer{}, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), string(thingsstartbaseline.CodeContractMismatch)) {
		t.Fatalf("run() exit/stderr = %d/%q, want CONTRACT_MISMATCH", code, stderr.String())
	}
}

func TestRunFixesParameterBoundaryBehavior(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantCode  int
		wantError string
		wantRepo  string
	}{
		{
			name:     "equals form",
			args:     []string{"image", "--contract", "contract.yaml", "--repo=equals-repo", "--runtime-root", "runtime"},
			wantCode: 0,
			wantRepo: "equals-repo",
		},
		{
			name:     "equals value starts with dashes",
			args:     []string{"image", "--contract", "contract.yaml", "--repo=--value", "--runtime-root", "runtime"},
			wantCode: 0,
			wantRepo: "--value",
		},
		{
			name:      "empty equals value",
			args:      []string{"image", "--contract", "contract.yaml", "--repo=", "--runtime-root", "runtime"},
			wantCode:  2,
			wantError: "missing value for --repo",
		},
		{
			name:      "missing separated value",
			args:      []string{"image", "--contract", "contract.yaml", "--runtime-root", "runtime", "--repo"},
			wantCode:  2,
			wantError: "missing value for --repo",
		},
		{
			name:      "separated value starts with dashes",
			args:      []string{"image", "--contract", "contract.yaml", "--runtime-root", "runtime", "--repo", "--value"},
			wantCode:  2,
			wantError: "missing value for --repo",
		},
		{
			name:      "double dash",
			args:      []string{"image", "--contract", "contract.yaml", "--repo", "repo", "--runtime-root", "runtime", "--"},
			wantCode:  2,
			wantError: "unknown parameter",
		},
		{
			name:      "positional argument",
			args:      []string{"image", "--contract", "contract.yaml", "--repo", "repo", "--runtime-root", "runtime", "positional"},
			wantCode:  2,
			wantError: "unknown parameter",
		},
		{
			name:      "mixed duplicate forms",
			args:      []string{"image", "--contract", "contract.yaml", "--repo=a", "--repo", "b", "--runtime-root", "runtime"},
			wantCode:  2,
			wantError: "duplicate parameter --repo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &recordingExecutor{}
			var stderr bytes.Buffer
			if code := run(tt.args, executor, &bytes.Buffer{}, &stderr); code != tt.wantCode {
				t.Fatalf("run() exit = %d, want %d; stderr=%q", code, tt.wantCode, stderr.String())
			}
			if tt.wantCode == 0 {
				wantRepo, err := filepath.Abs(tt.wantRepo)
				if err != nil {
					t.Fatalf("resolve expected repo: %v", err)
				}
				if executor.executeCalls != 1 || executor.options.Repo != wantRepo {
					t.Fatalf("execution/repo = %d/%q, want 1/%q", executor.executeCalls, executor.options.Repo, wantRepo)
				}
				return
			}
			if executor.totalCalls() != 0 || !strings.Contains(stderr.String(), tt.wantError) {
				t.Fatalf("executor calls/stderr = %d/%q", executor.totalCalls(), stderr.String())
			}
		})
	}
}

func TestRunRejectsUnsafeEnvFiles(t *testing.T) {
	contract := absoluteContractPath(t)
	temp := t.TempDir()
	missing := filepath.Join(temp, "missing.env")
	directory := filepath.Join(temp, "env-directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatalf("create env directory: %v", err)
	}
	wrongMode := filepath.Join(temp, "wrong-mode.env")
	if err := os.WriteFile(wrongMode, []byte("PASSWORD=test\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	if err := os.Chmod(wrongMode, 0o640); err != nil {
		t.Fatalf("chmod env file: %v", err)
	}
	wrongModeInfo, err := os.Stat(wrongMode)
	if err != nil {
		t.Fatalf("stat env file: %v", err)
	}
	if got := wrongModeInfo.Mode().Perm(); got != 0o640 {
		t.Fatalf("env fixture permissions = %04o, want 0640", got)
	}
	symlinkTarget := filepath.Join(temp, "symlink-target.env")
	if err := os.WriteFile(symlinkTarget, []byte("PASSWORD=test\n"), 0o600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Chmod(symlinkTarget, 0o600); err != nil {
		t.Fatalf("chmod symlink target: %v", err)
	}
	symlink := filepath.Join(temp, "symlink.env")
	if err := os.Symlink(symlinkTarget, symlink); err != nil {
		t.Fatalf("create env symlink: %v", err)
	}
	for _, tt := range []struct {
		name string
		path string
		want string
	}{
		{"missing", missing, "no such file"},
		{"directory", directory, "regular file"},
		{"permissions", wrongMode, "0600"},
		{"symlink", symlink, "symbolic link"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			executor := &recordingExecutor{}
			args := []string{"contract", "--contract", contract, "--env-file", tt.path}
			var stderr bytes.Buffer
			if code := run(args, executor, &bytes.Buffer{}, &stderr); code != 2 {
				t.Fatalf("run() exit = %d, want 2; stderr=%q", code, stderr.String())
			}
			if executor.totalCalls() != 0 || !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("executor calls/stderr = %d/%q", executor.totalCalls(), stderr.String())
			}
		})
	}
}

func TestDefaultExecutorRoutesUpAndVerifyProductionRuntime(t *testing.T) {
	var calls []string
	executor := defaultExecutor{
		up: func(options commandOptions) error {
			calls = append(calls, "up:"+options.RequestedCommand)
			return nil
		},
		verify: func(options commandOptions) error {
			calls = append(calls, "verify:"+options.RequestedCommand)
			return nil
		},
	}
	upOptions := commandOptions{RequestedCommand: "up"}
	for _, step := range []string{"database", "infra", "roles", "verify"} {
		if err := executor.Execute(step, upOptions); err != nil {
			t.Fatalf("Execute(%q, up) error = %v", step, err)
		}
	}
	verifyOptions := commandOptions{RequestedCommand: "verify"}
	if err := executor.Execute("verify", verifyOptions); err != nil {
		t.Fatalf("Execute(verify, verify) error = %v", err)
	}
	if got, want := strings.Join(calls, ","), "up:up,verify:verify"; got != want {
		t.Fatalf("runtime calls = %q, want %q", got, want)
	}
}

func TestDefaultExecutorPropagatesProductionRuntimeFailure(t *testing.T) {
	want := errors.New("fixture runtime failure")
	executor := defaultExecutor{
		up:     func(commandOptions) error { return want },
		verify: func(commandOptions) error { return want },
	}
	if err := executor.Execute("verify", commandOptions{RequestedCommand: "up"}); !errors.Is(err, want) {
		t.Fatalf("up runtime error = %v, want %v", err, want)
	}
	if err := executor.Execute("verify", commandOptions{RequestedCommand: "verify"}); !errors.Is(err, want) {
		t.Fatalf("verify runtime error = %v, want %v", err, want)
	}
}

func TestDefaultExecutorRejectsRuntimeStepsOutsideUpWorkflow(t *testing.T) {
	for _, step := range []string{"database", "infra", "roles"} {
		err := (defaultExecutor{}).Execute(step, commandOptions{RequestedCommand: "verify"})
		if err == nil || !strings.Contains(err.Error(), "only valid during up") {
			t.Fatalf("Execute(%q, verify) error = %v", step, err)
		}
	}
}

func TestDefaultExecutorWritesManifest(t *testing.T) {
	temp := t.TempDir()
	binDir := filepath.Join(temp, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatalf("create bin directory: %v", err)
	}
	staticTestBinary(t, binDir)
	output := filepath.Join(temp, "runtime", "build-manifest.json")
	var stderr bytes.Buffer
	if code := run([]string{
		"manifest",
		"--contract", absoluteContractPath(t),
		"--repo", filepath.Join(filepath.Dir(absoluteContractPath(t)), "../../../.."),
		"--bin-dir", binDir,
		"--output", output,
	}, defaultExecutor{}, &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("run() exit = %d, stderr=%q", code, stderr.String())
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest thingsstartbaseline.BuildManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(manifest.Artifacts) != 5 {
		t.Fatalf("manifest artifacts = %d, want 5", len(manifest.Artifacts))
	}
}

func TestRunReturnsOneForExecutorFailuresAndInvalidContract(t *testing.T) {
	contract := absoluteContractPath(t)
	temp := t.TempDir()
	env := filepath.Join(temp, "startup.env")
	if err := os.WriteFile(env, []byte("PASSWORD=test\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	t.Run("execution failure", func(t *testing.T) {
		executor := &recordingExecutor{executeErr: errors.New("fixture execution failure")}
		var stderr bytes.Buffer
		if code := run([]string{"image", "--contract", contract, "--repo", "repo", "--runtime-root", "runtime"}, executor, &bytes.Buffer{}, &stderr); code != 1 || !strings.Contains(stderr.String(), "fixture execution failure") {
			t.Fatalf("run() exit/stderr = %d/%q", code, stderr.String())
		}
		if executor.validationCalls != 0 || executor.executeCalls != 1 {
			t.Fatalf("executor calls = validation %d execution %d", executor.validationCalls, executor.executeCalls)
		}
	})
	t.Run("environment validation failure", func(t *testing.T) {
		executor := &recordingExecutor{validationErr: errors.New("fixture validation failure")}
		var stderr bytes.Buffer
		if code := run([]string{"contract", "--contract", contract, "--env-file", env}, executor, &bytes.Buffer{}, &stderr); code != 1 || !strings.Contains(stderr.String(), "fixture validation failure") {
			t.Fatalf("run() exit/stderr = %d/%q", code, stderr.String())
		}
		if executor.validationCalls != 1 || executor.executeCalls != 0 {
			t.Fatalf("executor calls = validation %d execution %d", executor.validationCalls, executor.executeCalls)
		}
	})
	t.Run("contract loading", func(t *testing.T) {
		bad := filepath.Join(temp, "bad-contract.yaml")
		if err := os.WriteFile(bad, []byte("schemaVersion: 1\nunknown: true\n"), 0o600); err != nil {
			t.Fatalf("write bad contract: %v", err)
		}
		executor := &recordingExecutor{}
		if code := run([]string{"contract", "--contract", bad, "--env-file", env}, executor, &bytes.Buffer{}, &bytes.Buffer{}); code != 1 || executor.totalCalls() != 0 {
			t.Fatalf("run() exit/calls = %d/%d, want 1/0", code, executor.totalCalls())
		}
	})
}

func absoluteContractPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "../../../../deploy/profiles/things-baseline/contract.yaml"))
}

func repositoryRootPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "../../../.."))
}

func readContractFile(t *testing.T, filename string) string {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	return string(data)
}

func makeTargetBlock(t *testing.T, makefile, target string) string {
	t.Helper()
	marker := "\n" + target + ":"
	start := strings.Index(makefile, marker)
	if start < 0 {
		t.Fatalf("deploy/Makefile missing target %q", target)
	}
	block := makefile[start+1:]
	if end := strings.Index(block, "\n\n"); end >= 0 {
		block = block[:end]
	}
	return block
}

func relativeContractPath(t *testing.T) string {
	t.Helper()
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path, err := filepath.Rel(workingDir, absoluteContractPath(t))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func staticTestBinary(t *testing.T, binDir string) {
	t.Helper()
	filename, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test executable: %v", err)
	}
	file, err := elf.Open(filename)
	if err != nil {
		t.Skipf("test executable is not ELF: %v", err)
	}
	defer file.Close()
	libraries, err := file.ImportedLibraries()
	if err != nil {
		t.Fatalf("inspect test executable imports: %v", err)
	}
	if len(libraries) != 0 {
		t.Skipf("test executable is not statically linked: %v", libraries)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read test executable: %v", err)
	}
	for _, binary := range []string{"syssvr", "timedjobsvr", "dmsvr", "dgsvr", "thingsvr"} {
		if err := os.WriteFile(filepath.Join(binDir, binary), data, 0o755); err != nil {
			t.Fatalf("write %s fixture: %v", binary, err)
		}
	}
}

type recordingExecutor struct {
	validationCalls int
	executeCalls    int
	command         string
	steps           []string
	options         commandOptions
	validationErr   error
	executeErr      error
	failStep        string
}

func (e *recordingExecutor) ValidateEnvironment(command string, options commandOptions) error {
	e.validationCalls++
	e.command = command
	e.steps = append(e.steps, command)
	e.options = options
	if e.failStep != "" && e.failStep != command {
		return nil
	}
	return e.validationErr
}

func (e *recordingExecutor) Execute(command string, options commandOptions) error {
	e.executeCalls++
	e.command = command
	e.steps = append(e.steps, command)
	e.options = options
	if e.failStep != "" && e.failStep != command {
		return nil
	}
	return e.executeErr
}

func (e *recordingExecutor) totalCalls() int {
	return e.validationCalls + e.executeCalls
}
