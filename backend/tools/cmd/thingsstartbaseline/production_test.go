package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ming-iot/backend/tools/internal/thingsstartbaseline"
)

func TestDefaultExecutorValidatesContractEnvironmentTarget(t *testing.T) {
	tests := []struct {
		name       string
		database   string
		wantErr    bool
		wantSecret string
	}{
		{name: "baseline database", database: "ming_iot_things_baseline"},
		{name: "legacy database", database: "iThings", wantErr: true},
		{name: "other database", database: "other", wantErr: true},
		{name: "missing database", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directory := t.TempDir()
			envFile := filepath.Join(directory, ".env.local")
			secret := "fixture-password-must-not-leak"
			contents := "THINGS_BASELINE_DB=" + tt.database + "\nTHINGS_BASELINE_DB_PASSWORD=" + secret + "\n"
			if err := os.WriteFile(envFile, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			err := (defaultExecutor{}).ValidateEnvironment("contract", commandOptions{
				ContractPath: absoluteContractPath(t),
				EnvFile:      envFile,
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateEnvironment() error = %v, want error=%t", err, tt.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), secret) {
				t.Fatalf("ValidateEnvironment() leaked a secret: %v", err)
			}
		})
	}
}

func TestDefaultExecutorExecutesManifestWithExplicitAndCompositePaths(t *testing.T) {
	type call struct {
		contract string
		repo     string
		binDir   string
		output   string
	}
	var calls []call
	executor := defaultExecutor{
		writeManifest: func(contract, repo, binDir, output string) error {
			calls = append(calls, call{contract: contract, repo: repo, binDir: binDir, output: output})
			return nil
		},
	}
	temp := t.TempDir()
	explicit := commandOptions{
		RequestedCommand: "manifest",
		ContractPath:     filepath.Join(temp, "contract.yaml"),
		Repo:             filepath.Join(temp, "repo"),
		BinDir:           filepath.Join(temp, "custom-bin"),
		Output:           filepath.Join(temp, "custom-manifest.json"),
	}
	if err := executor.Execute("manifest", explicit); err != nil {
		t.Fatal(err)
	}
	composite := commandOptions{
		RequestedCommand: "up",
		ContractPath:     explicit.ContractPath,
		Repo:             explicit.Repo,
		RuntimeRoot:      filepath.Join(temp, "runtime"),
	}
	if err := executor.Execute("manifest", composite); err != nil {
		t.Fatal(err)
	}
	want := []call{
		{contract: explicit.ContractPath, repo: explicit.Repo, binDir: explicit.BinDir, output: explicit.Output},
		{contract: explicit.ContractPath, repo: explicit.Repo, binDir: filepath.Join(explicit.Repo, "backend", "bin"), output: filepath.Join(composite.RuntimeRoot, "build-manifest.json")},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("manifest calls = %#v, want %#v", calls, want)
	}
}

func TestDefaultExecutorStagesContractInputs(t *testing.T) {
	var got thingsstartbaseline.StageRequest
	executor := defaultExecutor{
		stage: func(request thingsstartbaseline.StageRequest) (thingsstartbaseline.StageResult, error) {
			got = request
			if err := os.MkdirAll(request.RuntimeRoot, 0o755); err != nil {
				return thingsstartbaseline.StageResult{}, err
			}
			return thingsstartbaseline.StageResult{}, nil
		},
	}
	temp := t.TempDir()
	envFile := filepath.Join(temp, ".env.local")
	if err := os.WriteFile(envFile, []byte(validRuntimeEnvironmentFixture()), 0o600); err != nil {
		t.Fatal(err)
	}
	options := commandOptions{
		ContractPath: absoluteContractPath(t),
		Repo:         filepath.Join(temp, "repo"),
		EnvFile:      envFile,
		RuntimeRoot:  filepath.Join(temp, "runtime"),
	}
	err := executor.Execute("stage", options)
	if got.RepoRoot != options.Repo || got.RuntimeRoot != options.RuntimeRoot || got.Contract == nil || len(got.Contract.Roles) != 5 {
		t.Fatalf("stage request = %+v", got)
	}
	secretFile := filepath.Join(options.RuntimeRoot, "secrets", "emqx-api-key.conf")
	if os.Geteuid() != 0 && (os.Geteuid() != 1000 || os.Getegid() != 1000) {
		if err == nil || !strings.Contains(err.Error(), "ownership cannot be corrected") {
			t.Fatalf("Execute() error = %v, want fail-closed ownership error", err)
		}
		if _, statErr := os.Lstat(secretFile); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("failed ownership correction published target: %v", statErr)
		}
		entries, readErr := os.ReadDir(filepath.Dir(secretFile))
		if readErr != nil || len(entries) != 0 {
			t.Fatalf("failed ownership correction left secret files: %+v/%v", entries, readErr)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(secretFile)
	if err != nil {
		t.Fatalf("read staged EMQX secret: %v", err)
	}
	if got, want := string(data), "emqx-key:emqx-secret\n"; got != want {
		t.Fatalf("staged EMQX secret = %q, want %q", got, want)
	}
	if info, err := os.Stat(secretFile); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("staged EMQX secret mode = %+v/%v", info, err)
	}
	if info, err := os.Stat(filepath.Dir(secretFile)); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("EMQX secret directory mode = %+v/%v, want 0700", info, err)
	}
}

func writeOldEMQXSecretFixture(t *testing.T, runtimeRoot string) {
	t.Helper()
	// Keep fixture setup independent of privileges required for real EMQX ownership.
	directory := filepath.Join(runtimeRoot, "secrets")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "emqx-api-key.conf"), []byte("old-key:old-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultExecutorRemovesOldSecretBeforeStageFailure(t *testing.T) {
	temp := t.TempDir()
	envFile := filepath.Join(temp, ".env.local")
	if err := os.WriteFile(envFile, []byte(validRuntimeEnvironmentFixture()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(temp, "runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeOldEMQXSecretFixture(t, filepath.Join(temp, "runtime"))
	want := errors.New("stage failed")
	err := (defaultExecutor{
		stage: func(thingsstartbaseline.StageRequest) (thingsstartbaseline.StageResult, error) {
			return thingsstartbaseline.StageResult{}, want
		},
	}).Execute("stage", commandOptions{
		ContractPath: absoluteContractPath(t), Repo: filepath.Join(temp, "repo"),
		EnvFile: envFile, RuntimeRoot: filepath.Join(temp, "runtime"),
	})
	if !errors.Is(err, want) {
		t.Fatalf("Execute() error = %v, want %v", err, want)
	}
	if _, statErr := os.Lstat(filepath.Join(temp, "runtime", "secrets", "emqx-api-key.conf")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("old secret stat error = %v, want not exist", statErr)
	}
}

func TestDefaultExecutorPropagatesPublishFailureAfterRemovingOldSecret(t *testing.T) {
	temp := t.TempDir()
	envFile := filepath.Join(temp, ".env.local")
	if err := os.WriteFile(envFile, []byte(validRuntimeEnvironmentFixture()), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Join(temp, "runtime")
	if err := os.Mkdir(runtimeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeOldEMQXSecretFixture(t, runtimeRoot)
	err := (defaultExecutor{
		stage: func(request thingsstartbaseline.StageRequest) (thingsstartbaseline.StageResult, error) {
			if err := os.Mkdir(filepath.Join(request.RuntimeRoot, "secrets", "emqx-api-key.conf"), 0o700); err != nil {
				return thingsstartbaseline.StageResult{}, err
			}
			return thingsstartbaseline.StageResult{}, nil
		},
	}).Execute("stage", commandOptions{
		ContractPath: absoluteContractPath(t), Repo: filepath.Join(temp, "repo"),
		EnvFile: envFile, RuntimeRoot: runtimeRoot,
	})
	if err == nil || !strings.Contains(err.Error(), "bootstrap secret target") {
		t.Fatalf("Execute() error = %v, want publish target error", err)
	}
	data, readErr := os.ReadFile(filepath.Join(runtimeRoot, "secrets", "emqx-api-key.conf"))
	if readErr == nil || string(data) == "old-key:old-secret\n" {
		t.Fatalf("old secret remained after publish failure: data=%q error=%v", data, readErr)
	}
}

func TestDefaultExecutorRejectsUnsafeSecretPathsBeforeStage(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{name: "runtime root symlink", setup: func(t *testing.T, root string) {
			t.Helper()
			if err := os.Symlink(t.TempDir(), root); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "secret directory symlink", setup: func(t *testing.T, root string) {
			t.Helper()
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(t.TempDir(), filepath.Join(root, "secrets")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "target symlink", setup: func(t *testing.T, root string) {
			t.Helper()
			if err := os.MkdirAll(filepath.Join(root, "secrets"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(t.TempDir(), "target"), filepath.Join(root, "secrets", "emqx-api-key.conf")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "non-regular target", setup: func(t *testing.T, root string) {
			t.Helper()
			if err := os.MkdirAll(filepath.Join(root, "secrets", "emqx-api-key.conf"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			temp := t.TempDir()
			envFile := filepath.Join(temp, ".env.local")
			if err := os.WriteFile(envFile, []byte(validRuntimeEnvironmentFixture()), 0o600); err != nil {
				t.Fatal(err)
			}
			runtimeRoot := filepath.Join(temp, "runtime")
			tt.setup(t, runtimeRoot)
			called := false
			err := (defaultExecutor{
				stage: func(thingsstartbaseline.StageRequest) (thingsstartbaseline.StageResult, error) {
					called = true
					return thingsstartbaseline.StageResult{}, nil
				},
			}).Execute("stage", commandOptions{
				ContractPath: absoluteContractPath(t), Repo: filepath.Join(temp, "repo"),
				EnvFile: envFile, RuntimeRoot: runtimeRoot,
			})
			if err == nil || called {
				t.Fatalf("Execute() error = %v, stage called = %t", err, called)
			}
		})
	}
}

func TestDefaultExecutorRejectsInvalidSecretFieldsBeforeStageAndKeepsOldSecret(t *testing.T) {
	for _, tt := range []struct {
		name     string
		field    string
		oldValue string
		value    string
	}{
		{name: "empty API key", field: "THINGS_EMQX_API_KEY", oldValue: "emqx-key"},
		{name: "empty secret key", field: "THINGS_EMQX_SECRET_KEY", oldValue: "emqx-secret"},
		{name: "colon in API key", field: "THINGS_EMQX_API_KEY", oldValue: "emqx-key", value: "bad:api-key"},
		{name: "colon in secret key", field: "THINGS_EMQX_SECRET_KEY", oldValue: "emqx-secret", value: "bad:secret-key"},
		{name: "NUL in API key", field: "THINGS_EMQX_API_KEY", oldValue: "emqx-key", value: "bad\x00api-key"},
		{name: "control in secret key", field: "THINGS_EMQX_SECRET_KEY", oldValue: "emqx-secret", value: "bad\x01secret-key"},
		{name: "DEL in API key", field: "THINGS_EMQX_API_KEY", oldValue: "emqx-key", value: "bad\x7fapi-key"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			temp := t.TempDir()
			envFile := filepath.Join(temp, ".env.local")
			contents := strings.Replace(validRuntimeEnvironmentFixture(), tt.field+"="+tt.oldValue, tt.field+"="+tt.value, 1)
			if err := os.WriteFile(envFile, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			runtimeRoot := filepath.Join(temp, "runtime")
			if err := os.Mkdir(runtimeRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			writeOldEMQXSecretFixture(t, runtimeRoot)
			called := false
			err := (defaultExecutor{
				stage: func(thingsstartbaseline.StageRequest) (thingsstartbaseline.StageResult, error) {
					called = true
					return thingsstartbaseline.StageResult{}, nil
				},
			}).Execute("stage", commandOptions{
				ContractPath: absoluteContractPath(t), Repo: filepath.Join(temp, "repo"),
				EnvFile: envFile, RuntimeRoot: runtimeRoot,
			})
			if err == nil || called {
				t.Fatalf("Execute() error = %v, stage called = %t", err, called)
			}
			if tt.value != "" && strings.Contains(err.Error(), tt.value) {
				t.Fatalf("Execute() leaked invalid secret field: %v", err)
			}
			data, readErr := os.ReadFile(filepath.Join(runtimeRoot, "secrets", "emqx-api-key.conf"))
			if readErr != nil || string(data) != "old-key:old-secret\n" {
				t.Fatalf("old secret = %q/%v", data, readErr)
			}
		})
	}
}

func TestDefaultExecutorBuildsFiveImagesFromManifest(t *testing.T) {
	repo := projectRootFromContract(t)
	runtimeRoot := t.TempDir()
	manifest := imageBuildManifest(t, runtimeRoot)
	runner := &recordingCommandRunner{}
	executor := defaultExecutor{runner: runner}
	if err := executor.Execute("image", commandOptions{
		ContractPath: absoluteContractPath(t),
		Repo:         repo,
		RuntimeRoot:  runtimeRoot,
	}); err != nil {
		t.Fatal(err)
	}
	contract, err := thingsstartbaseline.LoadContract(absoluteContractPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != len(contract.Roles) {
		t.Fatalf("docker commands = %d, want %d", len(runner.commands), len(contract.Roles))
	}
	for index, role := range contract.Roles {
		got := runner.commands[index]
		wantArgs := []string{
			"build",
			"--build-arg", "VCS_REF=" + manifest.GitRevision,
			"--build-arg", "BINARY_SHA256=" + manifest.Artifacts[index].SHA256,
			"-f", filepath.Join(repo, role.Dockerfile),
			"-t", role.Image,
			filepath.Join(runtimeRoot, "stage", role.ID),
		}
		if got.Path != "docker" || got.Dir != repo || !reflect.DeepEqual(got.Args, wantArgs) {
			t.Fatalf("command %d = %#v, want docker %#v in %q", index, got, wantArgs, repo)
		}
	}
}

func TestDefaultExecutorStopsImageBuildAtFirstFailure(t *testing.T) {
	repo := projectRootFromContract(t)
	runtimeRoot := t.TempDir()
	imageBuildManifest(t, runtimeRoot)
	runner := &recordingCommandRunner{failAt: 2, err: errors.New("fixture docker failure")}
	err := (defaultExecutor{runner: runner}).Execute("image", commandOptions{
		ContractPath: absoluteContractPath(t),
		Repo:         repo,
		RuntimeRoot:  runtimeRoot,
	})
	if err == nil || !strings.Contains(err.Error(), "fixture docker failure") {
		t.Fatalf("Execute(image) error = %v", err)
	}
	if len(runner.commands) != 3 {
		t.Fatalf("docker commands = %d, want 3", len(runner.commands))
	}
}

func TestDefaultExecutorRejectsImageManifestDriftBeforeDocker(t *testing.T) {
	repo := projectRootFromContract(t)
	runtimeRoot := t.TempDir()
	manifest := imageBuildManifest(t, runtimeRoot)
	manifest.Artifacts[0].SHA256 = "not-a-digest"
	writeImageBuildManifest(t, runtimeRoot, manifest)
	runner := &recordingCommandRunner{}
	err := (defaultExecutor{runner: runner}).Execute("image", commandOptions{
		ContractPath: absoluteContractPath(t),
		Repo:         repo,
		RuntimeRoot:  runtimeRoot,
	})
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("Execute(image) error = %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("docker commands = %d, want 0", len(runner.commands))
	}
}

func TestDefaultExecutorRejectsStagedBinaryDriftBeforeDocker(t *testing.T) {
	for _, tt := range []struct {
		role   string
		binary string
	}{
		{role: "sys", binary: "syssvr"},
		{role: "api", binary: "thingsvr"},
	} {
		t.Run(tt.role, func(t *testing.T) {
			repo := projectRootFromContract(t)
			runtimeRoot := t.TempDir()
			imageBuildManifest(t, runtimeRoot)
			if err := os.WriteFile(filepath.Join(runtimeRoot, "stage", tt.role, "bin", tt.binary), []byte("changed after manifest"), 0o755); err != nil {
				t.Fatal(err)
			}
			runner := &recordingCommandRunner{}
			err := (defaultExecutor{runner: runner}).Execute("image", commandOptions{
				ContractPath: absoluteContractPath(t),
				Repo:         repo,
				RuntimeRoot:  runtimeRoot,
			})
			if err == nil || !strings.Contains(err.Error(), "does not match build manifest") {
				t.Fatalf("Execute(image) error = %v", err)
			}
			if len(runner.commands) != 0 {
				t.Fatalf("docker commands = %d, want 0", len(runner.commands))
			}
		})
	}
}

func TestNewProductionRuntimeUsesOnlyExplicitValidatedInputs(t *testing.T) {
	repo := t.TempDir()
	envFile := filepath.Join(repo, ".env.local")
	if err := os.WriteFile(envFile, []byte(validRuntimeEnvironmentFixture()), 0o600); err != nil {
		t.Fatal(err)
	}
	composeFile := filepath.Join(repo, "compose.yaml")
	if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	components, err := newProductionRuntime(commandOptions{
		ContractPath: absoluteContractPath(t),
		Repo:         repo,
		EnvFile:      envFile,
		ComposeFile:  composeFile,
		RuntimeRoot:  filepath.Join(repo, "runtime"),
	})
	if err != nil {
		t.Fatalf("newProductionRuntime() error = %v", err)
	}
	if components.contract == nil || components.runtime == nil || components.probe == nil {
		t.Fatalf("production runtime is incomplete: %+v", components)
	}
}

func TestLoadRuntimeEnvironmentFailsClosed(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "missing credential",
			content: strings.Replace(validRuntimeEnvironmentFixture(), "THINGS_EMQX_SECRET_KEY=emqx-secret\n", "", 1),
			want:    "THINGS_EMQX_SECRET_KEY",
		},
		{
			name:    "duplicate variable",
			content: validRuntimeEnvironmentFixture() + "POSTGRES_ADMIN_HOST=duplicate\n",
			want:    "duplicate POSTGRES_ADMIN_HOST",
		},
		{
			name:    "interpolation",
			content: strings.Replace(validRuntimeEnvironmentFixture(), "POSTGRES_ADMIN_HOST=host.docker.internal", "POSTGRES_ADMIN_HOST=${HOST}", 1),
			want:    "interpolation is not supported",
		},
		{
			name:    "reserved env-file override",
			content: validRuntimeEnvironmentFixture() + "THINGS_BASELINE_ENV_FILE=/tmp/other.env\n",
			want:    "THINGS_BASELINE_ENV_FILE is reserved",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), ".env.local")
			if err := os.WriteFile(filename, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := loadRuntimeEnvironment(filename)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("loadRuntimeEnvironment() error = %v, want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "emqx-secret") || strings.Contains(err.Error(), "postgres-secret") {
				t.Fatalf("loadRuntimeEnvironment() leaked a credential: %v", err)
			}
		})
	}
}

func TestRuntimePortUsesDefaultsAndRejectsInvalidValues(t *testing.T) {
	if got, err := runtimePort(map[string]string{}, "PORT", "17788"); err != nil || got != "17788" {
		t.Fatalf("runtimePort(default) = %q/%v", got, err)
	}
	for _, value := range []string{"0", "65536", "not-a-port"} {
		if _, err := runtimePort(map[string]string{"PORT": value}, "PORT", "17788"); err == nil {
			t.Fatalf("runtimePort(%q) accepted an invalid port", value)
		}
	}
}

func TestLoadRuntimeEnvironmentPreservesComposeEmptyDefaultSemantics(t *testing.T) {
	filename := filepath.Join(t.TempDir(), ".env.local")
	contents := strings.Replace(validRuntimeEnvironmentFixture(), "THINGS_BASELINE_API_PORT=17788", "THINGS_BASELINE_API_PORT=", 1)
	if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	environment, err := loadRuntimeEnvironment(filename)
	if err != nil {
		t.Fatalf("loadRuntimeEnvironment() error = %v", err)
	}
	if got, err := runtimePort(environment, "THINGS_BASELINE_API_PORT", "17788"); err != nil || got != "17788" {
		t.Fatalf("runtimePort(empty optional value) = %q/%v", got, err)
	}
}

func TestLoadRuntimeEnvironmentAllowsLiteralDollarInSingleQuotes(t *testing.T) {
	filename := filepath.Join(t.TempDir(), ".env.local")
	contents := strings.Replace(validRuntimeEnvironmentFixture(), "THINGS_EMQX_SECRET_KEY=emqx-secret", "THINGS_EMQX_SECRET_KEY='emqx$secret'", 1)
	if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	environment, err := loadRuntimeEnvironment(filename)
	if err != nil {
		t.Fatalf("loadRuntimeEnvironment() error = %v", err)
	}
	if got := environment["THINGS_EMQX_SECRET_KEY"]; got != "emqx$secret" {
		t.Fatalf("literal secret = %q", got)
	}
}

func TestPublishProductionEvidenceCollectsRuntimeIdentityAtomically(t *testing.T) {
	repo := t.TempDir()
	runtimeRoot := filepath.Join(repo, "runtime")
	manifest := imageBuildManifest(t, runtimeRoot)
	envFile := filepath.Join(repo, ".env.local")
	if err := os.WriteFile(envFile, []byte(validRuntimeEnvironmentFixture()), 0o600); err != nil {
		t.Fatal(err)
	}
	composeFile := filepath.Join(repo, "compose.yaml")
	if err := os.WriteFile(composeFile, []byte(infrastructureComposeFixture()), 0o644); err != nil {
		t.Fatal(err)
	}
	contract, err := thingsstartbaseline.LoadContract(absoluteContractPath(t))
	if err != nil {
		t.Fatal(err)
	}
	runner := &evidenceCommandRunner{}
	options := commandOptions{
		ContractPath: absoluteContractPath(t),
		Repo:         repo,
		EnvFile:      envFile,
		ComposeFile:  composeFile,
		RuntimeRoot:  runtimeRoot,
	}
	components, err := newProductionRuntime(options, runner)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := prepareProductionEvidence(options, components)
	if err != nil {
		t.Fatal(err)
	}
	runID, err := publishPreparedProductionEvidence(options, snapshot, passingProductionStages(contract), nil)
	if err != nil {
		t.Fatalf("publishProductionEvidence() error = %v", err)
	}
	runDirectory := filepath.Join(runtimeRoot, "runs", runID)
	for _, name := range []string{"evidence.json", "versions.json", "compose-config.sha256", "binary-sha256.json"} {
		if info, err := os.Stat(filepath.Join(runDirectory, name)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("published %s = %+v/%v", name, info, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(runDirectory, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	var evidence thingsstartbaseline.Evidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.OverallResult != "PASS" || evidence.GitRevision != manifest.GitRevision || len(evidence.Images) != 5 {
		t.Fatalf("evidence identity = %+v", evidence)
	}
	if secret := "postgres-secret"; strings.Contains(string(data), secret) {
		t.Fatalf("evidence leaked %q", secret)
	}
	digest, err := os.ReadFile(filepath.Join(runDirectory, "compose-config.sha256"))
	if err != nil || strings.TrimSpace(string(digest)) != strings.Repeat("c", 64) {
		t.Fatalf("compose digest = %q/%v", digest, err)
	}
	if len(runner.commands) != 13 {
		t.Fatalf("metadata commands = %d, want 13", len(runner.commands))
	}
}

func TestPrepareProductionEvidenceRejectsInvalidComposeDigestBeforeRun(t *testing.T) {
	repo := t.TempDir()
	runtimeRoot := filepath.Join(repo, "runtime")
	imageBuildManifest(t, runtimeRoot)
	envFile := filepath.Join(repo, ".env.local")
	if err := os.WriteFile(envFile, []byte(validRuntimeEnvironmentFixture()), 0o600); err != nil {
		t.Fatal(err)
	}
	composeFile := filepath.Join(repo, "compose.yaml")
	if err := os.WriteFile(composeFile, []byte(infrastructureComposeFixture()), 0o644); err != nil {
		t.Fatal(err)
	}
	options := commandOptions{ContractPath: absoluteContractPath(t), Repo: repo, EnvFile: envFile, ComposeFile: composeFile, RuntimeRoot: runtimeRoot}
	runner := &evidenceCommandRunner{composeDigest: "invalid"}
	components, err := newProductionRuntime(options, runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepareProductionEvidence(options, components); err == nil {
		t.Fatal("prepareProductionEvidence() accepted invalid Compose digest")
	}
	if entries, err := os.ReadDir(filepath.Join(runtimeRoot, "runs")); err == nil || !os.IsNotExist(err) || len(entries) != 0 {
		t.Fatalf("invalid preflight published a run: entries=%v err=%v", entries, err)
	}
}

func TestPreparedEvidencePublishesFailedRunWithoutRereadingInputs(t *testing.T) {
	repo := t.TempDir()
	runtimeRoot := filepath.Join(repo, "runtime")
	imageBuildManifest(t, runtimeRoot)
	envFile := filepath.Join(repo, ".env.local")
	if err := os.WriteFile(envFile, []byte(validRuntimeEnvironmentFixture()), 0o600); err != nil {
		t.Fatal(err)
	}
	composeFile := filepath.Join(repo, "compose.yaml")
	if err := os.WriteFile(composeFile, []byte(infrastructureComposeFixture()), 0o644); err != nil {
		t.Fatal(err)
	}
	options := commandOptions{ContractPath: absoluteContractPath(t), Repo: repo, EnvFile: envFile, ComposeFile: composeFile, RuntimeRoot: runtimeRoot}
	runner := &evidenceCommandRunner{}
	components, err := newProductionRuntime(options, runner)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := prepareProductionEvidence(options, components)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(validRuntimeEnvironmentFixture(), "POSTGRES_ADMIN_PASSWORD=postgres-secret", "POSTGRES_ADMIN_PASSWORD=replacement-secret", 1)
	if err := os.WriteFile(envFile, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	stages := failedProductionStages(components.contract, "postgres-secret")
	runID, err := publishPreparedProductionEvidence(options, snapshot, stages, errors.New("fixture startup failure"))
	if err != nil {
		t.Fatalf("publishPreparedProductionEvidence() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(runtimeRoot, "runs", runID, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"overallResult": "FAIL"`) || strings.Contains(string(data), "postgres-secret") || strings.Contains(string(data), "replacement-secret") {
		t.Fatalf("failed evidence is not a closed redacted snapshot: %s", data)
	}
}

func TestCloseProductionInputIdentityTurnsSuccessfulRunIntoConfigFailure(t *testing.T) {
	repo := t.TempDir()
	envFile := filepath.Join(repo, ".env.local")
	if err := os.WriteFile(envFile, []byte(validRuntimeEnvironmentFixture()), 0o600); err != nil {
		t.Fatal(err)
	}
	composeFile := filepath.Join(repo, "compose.yaml")
	if err := os.WriteFile(composeFile, []byte(infrastructureComposeFixture()), 0o644); err != nil {
		t.Fatal(err)
	}
	options := commandOptions{ContractPath: absoluteContractPath(t), Repo: repo, EnvFile: envFile, ComposeFile: composeFile, RuntimeRoot: filepath.Join(repo, "runtime")}
	identity, err := readProductionInputIdentity(options)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := thingsstartbaseline.LoadContract(options.ContractPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte(strings.Replace(validRuntimeEnvironmentFixture(), "POSTGRES_ADMIN_PORT=5432", "POSTGRES_ADMIN_PORT=15432", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	stages, closeErr := closeProductionInputIdentity(options, productionRuntime{inputIdentity: identity}, passingProductionStages(contract), nil)
	if closeErr == nil {
		t.Fatal("closeProductionInputIdentity() accepted changed inputs")
	}
	last := stages[len(stages)-1]
	if last.Result != "FAIL" || last.Code != thingsstartbaseline.CodeConfigInvalid {
		t.Fatalf("closed final stage = %+v", last)
	}
}

func TestCompleteProductionFailureProjectionCoversPreStageFailure(t *testing.T) {
	contract, err := thingsstartbaseline.LoadContract(absoluteContractPath(t))
	if err != nil {
		t.Fatal(err)
	}
	stages := completeProductionFailureProjection(contract, nil, errors.New("fixture state-root failure"))
	if len(stages) != len(contract.Stages) || stages[0].Result != "FAIL" || stages[0].Code != thingsstartbaseline.CodeConfigInvalid || stages[1].Result != "NOT_RUN" {
		t.Fatalf("pre-stage failure projection = %+v", stages)
	}
}

func TestVerifyProductionStagesStopsAndProjectsNotRun(t *testing.T) {
	contract, err := thingsstartbaseline.LoadContract(absoluteContractPath(t))
	if err != nil {
		t.Fatal(err)
	}
	runtime := &failingStageRuntime{failAt: thingsstartbaseline.StageDMReady}
	stages, err := verifyProductionStages(context.Background(), contract, runtime)
	if err == nil {
		t.Fatal("verifyProductionStages() succeeded despite fixture failure")
	}
	if len(stages) != len(contract.Stages) || stages[5].Result != "FAIL" || stages[6].Result != "NOT_RUN" {
		t.Fatalf("stage projection = %+v", stages)
	}
	if got, want := runtime.stages[len(runtime.stages)-1], thingsstartbaseline.StageDMReady; got != want {
		t.Fatalf("last executed stage = %s, want %s", got, want)
	}
}

func TestVerifyProductionStagesClassifiesDeadlineAsTimeout(t *testing.T) {
	contract, err := thingsstartbaseline.LoadContract(absoluteContractPath(t))
	if err != nil {
		t.Fatal(err)
	}
	runtime := &failingStageRuntime{failAt: thingsstartbaseline.StagePrecheck, failErr: context.DeadlineExceeded}
	stages, err := verifyProductionStages(context.Background(), contract, runtime)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("verifyProductionStages() error = %v", err)
	}
	if got := stages[0].Code; got != thingsstartbaseline.CodeTimeout {
		t.Fatalf("timeout diagnostic code = %s, want %s", got, thingsstartbaseline.CodeTimeout)
	}
}

func TestValidateProductionImageIDRequiresSHA256Digest(t *testing.T) {
	valid := "sha256:" + strings.Repeat("a", 64)
	if err := validateProductionImageID(valid); err != nil {
		t.Fatalf("validateProductionImageID(valid) error = %v", err)
	}
	for _, value := range []string{"", "latest", "sha256:short", "sha256:" + strings.Repeat("z", 64)} {
		if err := validateProductionImageID(value); err == nil {
			t.Fatalf("validateProductionImageID(%q) succeeded", value)
		}
	}
}

func TestDefaultExecutorLogsUsesReadOnlyBoundedComposeCommand(t *testing.T) {
	options, secret := productionLogsFixture(t)
	summary := "stdout:\nfixture log tail\nstderr:\n"
	runner := &logsCommandRunner{result: thingsstartbaseline.CommandResult{ExitCode: 0, Summary: summary}}
	var output bytes.Buffer

	err := (defaultExecutor{runner: runner, output: &output}).Execute("logs", options)
	if err != nil {
		t.Fatalf("Execute(logs) error = %v", err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("logs commands = %d, want 1", len(runner.commands))
	}
	contract, err := thingsstartbaseline.LoadContract(options.ContractPath)
	if err != nil {
		t.Fatal(err)
	}
	command := runner.commands[0]
	wantArgs := []string{
		"compose",
		"--project-name", contract.ProjectName,
		"--project-directory", filepath.Dir(options.ComposeFile),
		"--env-file", options.EnvFile,
		"-f", options.ComposeFile,
		"logs", "--no-color", "--tail", "200",
	}
	if command.Path != "docker" || command.Dir != options.Repo || !reflect.DeepEqual(command.Args, wantArgs) {
		t.Fatalf("logs command = %#v, want docker %#v in %q", command, wantArgs, options.Repo)
	}
	for _, forbidden := range []string{"build", "up", "run", "restart"} {
		for _, argument := range command.Args {
			if argument == forbidden {
				t.Fatalf("logs command contains mutating argument %q: %#v", forbidden, command.Args)
			}
		}
	}
	environment := strings.Join(command.Env, "\n")
	if !strings.Contains(environment, "POSTGRES_ADMIN_PASSWORD="+secret) || !strings.Contains(environment, "THINGS_BASELINE_ENV_FILE="+options.EnvFile) {
		t.Fatalf("logs command did not use the frozen env-file projection")
	}
	if output.String() != summary {
		t.Fatalf("logs output = %q, want CommandRunner summary %q", output.String(), summary)
	}
	if _, err := os.Stat(options.RuntimeRoot); !os.IsNotExist(err) {
		t.Fatalf("logs modified runtime root: %v", err)
	}
}

func TestDefaultExecutorLogsWritesBoundedRedactedCommandSummary(t *testing.T) {
	options, secret := productionLogsFixture(t)
	binDirectory := t.TempDir()
	docker := filepath.Join(binDirectory, "docker")
	script := "#!/bin/sh\nprintf '%s' '" + strings.Repeat("x", 5000) + "'\nprintf '%s\\n' \"$POSTGRES_ADMIN_PASSWORD\"\nprintf '%s\\n' 'LOG-TAIL-SENTINEL'\n"
	if err := os.WriteFile(docker, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
	var output bytes.Buffer

	if err := (defaultExecutor{output: &output}).Execute("logs", options); err != nil {
		t.Fatalf("Execute(logs) error = %v", err)
	}
	got := output.String()
	if len(got) > 4*1024 || !strings.Contains(got, "LOG-TAIL-SENTINEL") || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("logs summary boundary/redaction = len %d %q", len(got), got)
	}
	if strings.Contains(got, secret) {
		t.Fatalf("logs summary leaked secret %q", secret)
	}
}

func TestDefaultExecutorLogsPropagatesRunnerFailures(t *testing.T) {
	runnerFailure := errors.New("fixture runner failure")
	for _, tt := range []struct {
		name   string
		result thingsstartbaseline.CommandResult
		err    error
		want   string
	}{
		{name: "runner error", result: thingsstartbaseline.CommandResult{ExitCode: -1, Summary: "stdout:\n\nstderr:\nfixture"}, err: runnerFailure, want: "fixture runner failure"},
		{name: "non-zero exit", result: thingsstartbaseline.CommandResult{ExitCode: 17, Summary: "stdout:\n\nstderr:\nfixture"}, want: "status 17"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			options, _ := productionLogsFixture(t)
			runner := &logsCommandRunner{result: tt.result, err: tt.err}
			var output bytes.Buffer
			err := (defaultExecutor{runner: runner, output: &output}).Execute("logs", options)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Execute(logs) error = %v, want %q", err, tt.want)
			}
			if output.String() != tt.result.Summary {
				t.Fatalf("failed logs output = %q, want redacted summary %q", output.String(), tt.result.Summary)
			}
		})
	}
}

type logsCommandRunner struct {
	commands []thingsstartbaseline.Command
	result   thingsstartbaseline.CommandResult
	err      error
}

func (runner *logsCommandRunner) Run(_ context.Context, command thingsstartbaseline.Command) (thingsstartbaseline.CommandResult, error) {
	runner.commands = append(runner.commands, command)
	return runner.result, runner.err
}

func productionLogsFixture(t *testing.T) (commandOptions, string) {
	t.Helper()
	repo := t.TempDir()
	envFile := filepath.Join(repo, ".env.local")
	if err := os.WriteFile(envFile, []byte(validRuntimeEnvironmentFixture()), 0o600); err != nil {
		t.Fatal(err)
	}
	composeFile := filepath.Join(repo, "compose.yaml")
	if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return commandOptions{
		RequestedCommand: "logs",
		ContractPath:     absoluteContractPath(t),
		Repo:             repo,
		EnvFile:          envFile,
		ComposeFile:      composeFile,
		RuntimeRoot:      filepath.Join(repo, "runtime"),
	}, "postgres-secret"
}

type evidenceCommandRunner struct {
	commands      []thingsstartbaseline.Command
	composeDigest string
}

func (runner *evidenceCommandRunner) Run(_ context.Context, command thingsstartbaseline.Command) (thingsstartbaseline.CommandResult, error) {
	runner.commands = append(runner.commands, command)
	result := thingsstartbaseline.CommandResult{ExitCode: 0}
	switch {
	case reflect.DeepEqual(command.Args, []string{"--version"}):
		result.Summary = commandSummaryFixture("Docker version fixture")
	case reflect.DeepEqual(command.Args, []string{"compose", "version"}):
		result.Summary = commandSummaryFixture("Docker Compose version fixture")
	case len(command.Args) >= 1 && command.Args[len(command.Args)-1] == "config":
		result.StdoutSHA256 = runner.composeDigest
		if result.StdoutSHA256 == "" {
			result.StdoutSHA256 = strings.Repeat("c", 64)
		}
		result.Summary = commandSummaryFixture("expanded compose fixture")
	case len(command.Args) == 5 && command.Args[0] == "image" && command.Args[1] == "inspect":
		digest := sha256.Sum256([]byte(command.Args[4]))
		result.Summary = commandSummaryFixture("sha256:" + hex.EncodeToString(digest[:]))
	default:
		return thingsstartbaseline.CommandResult{ExitCode: 2}, errors.New("unexpected evidence command")
	}
	return result, nil
}

type failingStageRuntime struct {
	stages  []thingsstartbaseline.StageID
	failAt  thingsstartbaseline.StageID
	failErr error
}

func (runtime *failingStageRuntime) RunStage(_ context.Context, stage thingsstartbaseline.StageID) error {
	runtime.stages = append(runtime.stages, stage)
	if stage == runtime.failAt {
		if runtime.failErr != nil {
			return runtime.failErr
		}
		return errors.New("fixture stage failure")
	}
	return nil
}

func commandSummaryFixture(stdout string) string {
	return "stdout:\n" + stdout + "\nstderr:\n"
}

func passingProductionStages(contract *thingsstartbaseline.Contract) []thingsstartbaseline.StageEvidence {
	started := time.Now().UTC().Add(-time.Second)
	stages := make([]thingsstartbaseline.StageEvidence, len(contract.Stages))
	for index, stage := range contract.Stages {
		stages[index] = thingsstartbaseline.StageEvidence{
			ID:         stage.ID,
			Result:     "PASS",
			StartedAt:  started,
			FinishedAt: started.Add(time.Millisecond),
			DurationMS: 1,
			Component:  strings.Join(stage.Components, ","),
			LogRef:     "logs/" + strings.ToLower(string(stage.ID)) + ".log",
		}
	}
	return stages
}

func failedProductionStages(contract *thingsstartbaseline.Contract, reason string) []thingsstartbaseline.StageEvidence {
	stages := passingProductionStages(contract)
	failureIndex := 2
	stages[failureIndex].Result = "FAIL"
	stages[failureIndex].Code = thingsstartbaseline.CodeDependencyNotReady
	stages[failureIndex].Reason = reason
	for index := failureIndex + 1; index < len(stages); index++ {
		stages[index] = thingsstartbaseline.StageEvidence{
			ID:        contract.Stages[index].ID,
			Result:    "NOT_RUN",
			Component: strings.Join(contract.Stages[index].Components, ","),
			Reason:    "not run after INFRA_READY failure",
		}
	}
	return stages
}

func infrastructureComposeFixture() string {
	return strings.Join([]string{
		"services:",
		"  redis:",
		"    image: redis:7.4-alpine",
		"  etcd:",
		"    image: quay.io/coreos/etcd:v3.5.21",
		"  nats:",
		"    image: nats:2.10-alpine",
		"  tdengine:",
		"    image: tdengine/tdengine:3.3.6.34",
		"  emqx:",
		"    image: emqx/emqx:5.8.9",
	}, "\n") + "\n"
}

func validRuntimeEnvironmentFixture() string {
	return strings.Join([]string{
		"POSTGRES_ADMIN_HOST=host.docker.internal",
		"POSTGRES_ADMIN_PORT=5432",
		"POSTGRES_ADMIN_USER=postgres",
		"POSTGRES_ADMIN_PASSWORD=postgres-secret",
		"THINGS_BASELINE_DB=ming_iot_things_baseline",
		"THINGS_BASELINE_DB_USER=ming_iot_things_baseline",
		"THINGS_BASELINE_DB_PASSWORD=database-secret",
		"CORE_USER_TOKEN_SECRET=token-secret",
		"THINGS_INTERNAL_AUTH_PASSWORD=internal-secret",
		"THINGS_EMQX_API_KEY=emqx-key",
		"THINGS_EMQX_SECRET_KEY=emqx-secret",
		"TDENGINE_ROOT_PASSWORD=tdengine-secret",
		"THINGS_BASELINE_API_PORT=17788",
		"THINGS_BASELINE_EMQX_DASHBOARD_PORT=18084",
	}, "\n") + "\n"
}

type recordingCommandRunner struct {
	commands []thingsstartbaseline.Command
	failAt   int
	err      error
}

func (runner *recordingCommandRunner) Run(_ context.Context, command thingsstartbaseline.Command) (thingsstartbaseline.CommandResult, error) {
	runner.commands = append(runner.commands, command)
	if runner.err != nil && len(runner.commands)-1 == runner.failAt {
		return thingsstartbaseline.CommandResult{ExitCode: 1}, runner.err
	}
	return thingsstartbaseline.CommandResult{ExitCode: 0}, nil
}

func imageBuildManifest(t *testing.T, runtimeRoot string) thingsstartbaseline.BuildManifest {
	t.Helper()
	contract, err := thingsstartbaseline.LoadContract(absoluteContractPath(t))
	if err != nil {
		t.Fatal(err)
	}
	manifest := thingsstartbaseline.BuildManifest{
		SchemaVersion: 1,
		GitRevision:   strings.Repeat("a", 40),
		Artifacts:     make([]thingsstartbaseline.Artifact, len(contract.Roles)),
	}
	for index, role := range contract.Roles {
		contents := []byte(role.ID + "-binary-fixture")
		digest := sha256.Sum256(contents)
		manifest.Artifacts[index] = thingsstartbaseline.Artifact{
			Role:      role.ID,
			Binary:    role.Binary,
			SHA256:    hex.EncodeToString(digest[:]),
			Size:      int64(len(contents)),
			StaticELF: true,
		}
		binDir := filepath.Join(runtimeRoot, "stage", role.ID, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(binDir, role.Binary), contents, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeImageBuildManifest(t, runtimeRoot, manifest)
	return manifest
}

func writeImageBuildManifest(t *testing.T, runtimeRoot string, manifest thingsstartbaseline.BuildManifest) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeRoot, "build-manifest.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func projectRootFromContract(t *testing.T) string {
	t.Helper()
	return filepath.Clean(filepath.Join(filepath.Dir(absoluteContractPath(t)), "../../.."))
}
