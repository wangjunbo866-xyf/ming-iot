package thingsstartbaseline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestValidateRuntimeEnvironmentRejectsForbiddenDatabase(t *testing.T) {
	for _, tt := range []struct {
		name    string
		dsn     string
		wantErr bool
	}{
		{name: "forbidden iThings", dsn: "postgres://baseline@localhost:5432/iThings?sslmode=disable", wantErr: true},
		{name: "wrong database", dsn: "postgres://baseline@localhost:5432/other?sslmode=disable", wantErr: true},
		{name: "empty", dsn: "", wantErr: true},
		{name: "unparseable", dsn: "not a PostgreSQL URL", wantErr: true},
		{name: "baseline database", dsn: "postgres://baseline@localhost:5432/ming_iot_things_baseline?sslmode=disable"},
		{name: "dbname query override", dsn: "postgres://fixture-user:fixture-password@localhost:5432/ming_iot_things_baseline?dbname=iThings", wantErr: true},
		{name: "database query override", dsn: "postgres://fixture-user:fixture-password@localhost:5432/ming_iot_things_baseline?database=iThings", wantErr: true},
		{name: "case insensitive encoded query override", dsn: "postgres://fixture-user:fixture-password@localhost:5432/ming_iot_things_baseline?DbNaMe=%69THINGS", wantErr: true},
		{name: "encoded dbname query key", dsn: "postgres://fixture-user:fixture-password@localhost:5432/ming_iot_things_baseline?d%62name=iThings", wantErr: true},
		{name: "encoded forbidden path database", dsn: "postgres://fixture-user:fixture-password@localhost:5432/%69THINGS", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			contract := validContractForTest()
			t.Setenv("THINGS_DB_DSN", tt.dsn)

			err := ValidateRuntimeEnvironment(&contract)
			if tt.wantErr && !errors.Is(err, ErrContractMismatch) {
				t.Fatalf("ValidateRuntimeEnvironment() error = %v, want ErrContractMismatch", err)
			}
			if err != nil && tt.dsn != "" && strings.Contains(err.Error(), tt.dsn) {
				t.Fatalf("ValidateRuntimeEnvironment() error leaked DSN: %v", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateRuntimeEnvironment() error = %v, want nil", err)
			}
		})
	}
}

func TestStageRejectsRepositoryRootSymbolicLink(t *testing.T) {
	repositoryLink := filepath.Join(t.TempDir(), "repo")
	if err := os.Symlink(stageFixtureRepository(t), repositoryLink); err != nil {
		t.Fatalf("create repository symlink: %v", err)
	}
	contract := validContractForTest()

	_, err := Stage(StageRequest{
		RepoRoot:    repositoryLink,
		RuntimeRoot: t.TempDir(),
		Contract:    &contract,
	})
	if err == nil {
		t.Fatal("Stage() succeeded with a symbolic-link RepoRoot")
	}
}

func TestStageRejectsContractConfigurationPathEscape(t *testing.T) {
	contract := validContractForTest()
	contract.Roles[0].Config = "../outside.yaml"

	_, err := Stage(StageRequest{
		RepoRoot:    stageFixtureRepository(t),
		RuntimeRoot: t.TempDir(),
		Contract:    &contract,
	})
	if !errors.Is(err, ErrContractMismatch) {
		t.Fatalf("Stage() error = %v, want ErrContractMismatch", err)
	}
}

func TestStageRejectsSymbolicLinkSource(t *testing.T) {
	repoRoot := stageFixtureRepository(t)
	binary := filepath.Join(repoRoot, "backend", "bin", "syssvr")
	target := filepath.Join(repoRoot, "backend", "bin", "syssvr-target")
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatalf("read binary fixture: %v", err)
	}
	if err := os.WriteFile(target, data, 0o755); err != nil {
		t.Fatalf("write binary target: %v", err)
	}
	if err := os.Remove(binary); err != nil {
		t.Fatalf("remove binary fixture: %v", err)
	}
	if err := os.Symlink(target, binary); err != nil {
		t.Fatalf("create source symlink: %v", err)
	}
	contract := validContractForTest()

	if _, err := Stage(StageRequest{RepoRoot: repoRoot, RuntimeRoot: t.TempDir(), Contract: &contract}); err == nil {
		t.Fatal("Stage() succeeded with a symbolic-link source")
	}
}

func TestStageRejectsFIFOWithoutBlocking(t *testing.T) {
	repoRoot := stageFixtureRepository(t)
	fifo := filepath.Join(repoRoot, "backend", "bin", "syssvr")
	if err := os.Remove(fifo); err != nil {
		t.Fatalf("remove binary fixture: %v", err)
	}
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("create FIFO fixture: %v", err)
	}
	contract := validContractForTest()
	done := make(chan error, 1)
	go func() {
		_, err := Stage(StageRequest{RepoRoot: repoRoot, RuntimeRoot: t.TempDir(), Contract: &contract})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Stage() succeeded with FIFO source")
		}
	case <-time.After(250 * time.Millisecond):
		writer, err := unix.Open(fifo, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatalf("unblock FIFO reader: %v", err)
		}
		unix.Close(writer)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Stage() remained blocked after FIFO unblock")
		}
		t.Fatal("Stage() blocked while opening FIFO source")
	}
}

func TestStagePreservesExistingGenerationWhenExchangeFails(t *testing.T) {
	contract := validContractForTest()
	runtimeRoot := t.TempDir()
	repoRoot := stageFixtureRepository(t)
	if _, err := Stage(StageRequest{RepoRoot: repoRoot, RuntimeRoot: runtimeRoot, Contract: &contract}); err != nil {
		t.Fatalf("initial Stage() error = %v", err)
	}
	marker := filepath.Join(runtimeRoot, "stage", "old-generation-marker")
	writeStageFixtureFile(t, marker, "old")

	operations := newStageOperations()
	operations.exchangeDirectories = func(string, string) error { return errors.New("injected exchange failure") }
	if _, err := stageWithOperations(StageRequest{RepoRoot: repoRoot, RuntimeRoot: runtimeRoot, Contract: &contract}, operations); err == nil {
		t.Fatal("Stage() succeeded after injected exchange failure")
	}
	assertStageFixtureFile(t, marker, "old")
}

func TestStageRetainsOldGenerationWhenPostExchangeSyncFails(t *testing.T) {
	contract := validContractForTest()
	repoRoot := stageFixtureRepository(t)
	runtimeRoot := t.TempDir()
	if _, err := Stage(StageRequest{RepoRoot: repoRoot, RuntimeRoot: runtimeRoot, Contract: &contract}); err != nil {
		t.Fatalf("initial Stage() error = %v", err)
	}
	marker := filepath.Join(runtimeRoot, "stage", "old-generation-marker")
	writeStageFixtureFile(t, marker, "old")
	operations := newStageOperations()
	syncCalls := 0
	realSync := operations.syncDirectory
	operations.syncDirectory = func(path string) error {
		syncCalls++
		if path == runtimeRoot && syncCalls == 2 {
			return errors.New("injected post-exchange sync failure")
		}
		return realSync(path)
	}
	if _, err := stageWithOperations(StageRequest{RepoRoot: repoRoot, RuntimeRoot: runtimeRoot, Contract: &contract}, operations); err == nil {
		t.Fatal("stageWithOperations() succeeded after post-exchange sync failure")
	}
	if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("published stage marker error = %v, want not exist", err)
	}
	oldGenerations, err := filepath.Glob(filepath.Join(runtimeRoot, ".stage.tmp-*", "old-generation-marker"))
	if err != nil || len(oldGenerations) != 1 {
		t.Fatalf("retained old generations = %v, %v; want one", oldGenerations, err)
	}
	assertStageFixtureFile(t, oldGenerations[0], "old")
}

func TestCopyFileWithModeIgnoresUmask(t *testing.T) {
	sourceName := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(sourceName, []byte("fixture"), 0o754); err != nil {
		t.Fatalf("write source fixture: %v", err)
	}
	source, err := os.Open(sourceName)
	if err != nil {
		t.Fatalf("open source fixture: %v", err)
	}
	defer source.Close()
	previousUmask := unix.Umask(0o077)
	t.Cleanup(func() { unix.Umask(previousUmask) })
	destination := filepath.Join(t.TempDir(), "destination")
	if err := copyFileWithMode(source, destination, 0o754); err != nil {
		t.Fatalf("copyFileWithMode() error = %v", err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("stat destination: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o754 {
		t.Fatalf("destination mode = %04o, want 0754", got)
	}
}

func TestReplaceStageAtomicallyExchangesCompleteGenerations(t *testing.T) {
	runtimeRoot := t.TempDir()
	target := filepath.Join(runtimeRoot, "stage")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("create old stage: %v", err)
	}
	writeStageFixtureFile(t, filepath.Join(target, "generation"), "old")
	temporary, err := os.MkdirTemp(runtimeRoot, ".stage.tmp-")
	if err != nil {
		t.Fatalf("create new stage: %v", err)
	}
	writeStageFixtureFile(t, filepath.Join(temporary, "generation"), "new")

	operations := newStageOperations()
	originalExchange := operations.exchangeDirectories
	called := false
	operations.exchangeDirectories = func(oldStage, newStage string) error {
		called = true
		return originalExchange(oldStage, newStage)
	}
	if _, err := replaceStage(runtimeRoot, temporary, operations); err != nil {
		t.Fatalf("replaceStage() error = %v", err)
	}
	if !called {
		t.Fatal("replaceStage() did not use the atomic exchange primitive")
	}
	assertStageFixtureFile(t, filepath.Join(target, "generation"), "new")
	if _, err := os.Lstat(temporary); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old generation temporary path error = %v, want not exist", err)
	}
}

func TestReplaceStageKeepsOldGenerationWhenExchangeFails(t *testing.T) {
	runtimeRoot := t.TempDir()
	target := filepath.Join(runtimeRoot, "stage")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("create old stage: %v", err)
	}
	writeStageFixtureFile(t, filepath.Join(target, "generation"), "old")
	temporary, err := os.MkdirTemp(runtimeRoot, ".stage.tmp-")
	if err != nil {
		t.Fatalf("create new stage: %v", err)
	}
	writeStageFixtureFile(t, filepath.Join(temporary, "generation"), "new")

	operations := newStageOperations()
	operations.exchangeDirectories = func(string, string) error { return errors.New("injected exchange failure") }
	if _, err := replaceStage(runtimeRoot, temporary, operations); err == nil {
		t.Fatal("replaceStage() succeeded after injected exchange failure")
	}
	assertStageFixtureFile(t, filepath.Join(target, "generation"), "old")
	assertStageFixtureFile(t, filepath.Join(temporary, "generation"), "new")
}

func TestValidateSecretFileEnforcesStrictBoundary(t *testing.T) {
	directory := t.TempDir()
	fixtureSecret := "fixture-secret-must-not-leak"
	valid := filepath.Join(directory, ".env.local")
	if err := os.WriteFile(valid, []byte("SECRET="+fixtureSecret+"\n"), 0o600); err != nil {
		t.Fatalf("write secret target: %v", err)
	}
	symlinkDirectory := filepath.Join(directory, "symlink")
	if err := os.Mkdir(symlinkDirectory, 0o700); err != nil {
		t.Fatalf("create symlink directory: %v", err)
	}
	symlink := filepath.Join(symlinkDirectory, ".env.local")
	if err := os.Symlink(valid, symlink); err != nil {
		t.Fatalf("create secret symlink: %v", err)
	}
	directoryFixture := filepath.Join(directory, "directory", ".env.local")
	if err := os.MkdirAll(directoryFixture, 0o700); err != nil {
		t.Fatalf("create secret directory fixture: %v", err)
	}
	wrongMode := filepath.Join(directory, "wrong-mode", ".env.local")
	if err := os.Mkdir(filepath.Dir(wrongMode), 0o700); err != nil {
		t.Fatalf("create wrong-mode directory: %v", err)
	}
	if err := os.WriteFile(wrongMode, []byte("SECRET="+fixtureSecret+"\n"), 0o644); err != nil {
		t.Fatalf("write wrong-mode secret fixture: %v", err)
	}
	if err := os.Chmod(wrongMode, 0o644); err != nil {
		t.Fatalf("chmod wrong-mode secret fixture: %v", err)
	}
	wrongModeInfo, err := os.Stat(wrongMode)
	if err != nil {
		t.Fatalf("stat wrong-mode secret fixture: %v", err)
	}
	if got := wrongModeInfo.Mode().Perm(); got != 0o644 {
		t.Fatalf("wrong-mode secret fixture permissions = %04o, want 0644", got)
	}
	for _, tt := range []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{name: "valid 0600", filename: valid},
		{name: "missing", filename: filepath.Join(directory, "missing", ".env.local"), wantErr: true},
		{name: "directory", filename: directoryFixture, wantErr: true},
		{name: "symbolic link", filename: symlink, wantErr: true},
		{name: "0644", filename: wrongMode, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretFile(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateSecretFile() error = %v, want error=%t", err, tt.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), fixtureSecret) {
				t.Fatalf("ValidateSecretFile() leaked fixture secret: %v", err)
			}
		})
	}
}

func TestWriteFileAtomicallyWithOwnershipPublishesAtomic0600File(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "emqx-api-key.conf")
	if err := writeFileAtomicallyWithOwnership(filename, []byte("fixture-key:fixture-secret\n"), 0o600, nil); err != nil {
		t.Fatalf("writeFileAtomicallyWithOwnership() error = %v", err)
	}
	assertEMQXBootstrapSecret(t, filename, "fixture-key:fixture-secret\n")
	if err := writeFileAtomicallyWithOwnership(filename, []byte("replacement-key:replacement-secret\n"), 0o600, nil); err != nil {
		t.Fatalf("writeFileAtomicallyWithOwnership(replacement) error = %v", err)
	}
	assertEMQXBootstrapSecret(t, filename, "replacement-key:replacement-secret\n")
	entries, err := os.ReadDir(filepath.Dir(filename))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(filename) {
		t.Fatalf("secret directory entries = %+v", entries)
	}
}

func TestWriteEMQXBootstrapSecretUsesEMQXRuntimeOwnership(t *testing.T) {
	runtimeRoot := t.TempDir()
	filename := filepath.Join(runtimeRoot, "secrets", "emqx-api-key.conf")
	err := WriteEMQXBootstrapSecret(runtimeRoot, "fixture-key", "fixture-secret")
	if os.Geteuid() != 0 && (os.Geteuid() != 1000 || os.Getegid() != 1000) {
		if err == nil || !strings.Contains(err.Error(), "ownership cannot be corrected") {
			t.Fatalf("WriteEMQXBootstrapSecret() error = %v, want fail-closed ownership error", err)
		}
		if _, statErr := os.Stat(filename); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("failed ownership correction published target: %v", statErr)
		}
		entries, readErr := os.ReadDir(filepath.Dir(filename))
		if readErr != nil || len(entries) != 0 {
			t.Fatalf("failed ownership correction left secret files: %+v/%v", entries, readErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("WriteEMQXBootstrapSecret() error = %v", err)
	}
	assertEMQXBootstrapSecret(t, filename, "fixture-key:fixture-secret\n")
	if err := WriteEMQXBootstrapSecret(runtimeRoot, "replacement-key", "replacement-secret"); err != nil {
		t.Fatalf("WriteEMQXBootstrapSecret(replacement) error = %v", err)
	}
	assertEMQXBootstrapSecret(t, filename, "replacement-key:replacement-secret\n")
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("secret file stat type = %T, want *syscall.Stat_t", info.Sys())
	}
	if stat.Uid != 1000 || stat.Gid != 1000 {
		t.Fatalf("secret file owner = %d:%d, want 1000:1000", stat.Uid, stat.Gid)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secret file mode = %04o, want 0600", info.Mode().Perm())
	}
}

func TestEnsureEMQXBootstrapOwnershipContract(t *testing.T) {
	t.Run("already correct does not chown", func(t *testing.T) {
		called := false
		err := ensureEMQXBootstrapOwnership(1000, 1000, 1001, 1000, 1000, func() error {
			called = true
			return nil
		})
		if err != nil {
			t.Fatalf("ensureEMQXBootstrapOwnership() error = %v", err)
		}
		if called {
			t.Fatal("ownership correction called for an already-correct owner")
		}
	})

	t.Run("non-root mismatch fails closed", func(t *testing.T) {
		called := false
		err := ensureEMQXBootstrapOwnership(1001, 1001, 1001, 1000, 1000, func() error {
			called = true
			return nil
		})
		if err == nil || !strings.Contains(err.Error(), "ownership cannot be corrected") {
			t.Fatalf("ensureEMQXBootstrapOwnership() error = %v, want fail-closed error", err)
		}
		if called {
			t.Fatal("non-root ownership mismatch attempted chown")
		}
	})

	t.Run("root correction propagates failure", func(t *testing.T) {
		injected := errors.New("injected ownership failure")
		called := false
		err := ensureEMQXBootstrapOwnership(0, 0, 0, 1000, 1000, func() error {
			called = true
			return injected
		})
		if !called {
			t.Fatal("ownership correction was not attempted")
		}
		if !errors.Is(err, injected) {
			t.Fatalf("ensureEMQXBootstrapOwnership() error = %v, want injected error", err)
		}
	})
}

func TestWriteFileAtomicallyWithOwnershipFailurePreservesTargetAndCleansTemporary(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "emqx-api-key.conf")
	if err := os.WriteFile(filename, []byte("old-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected ownership failure")
	err := writeFileAtomicallyWithOwnership(filename, []byte("new-sensitive-value\n"), 0o600, func(string) error {
		return injected
	})
	if !errors.Is(err, injected) {
		t.Fatalf("writeFileAtomicallyWithOwnership() error = %v, want injected error", err)
	}
	got, readErr := os.ReadFile(filename)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "old-value\n" {
		t.Fatalf("target content = %q, want old value", got)
	}
	temporary, globErr := filepath.Glob(filepath.Join(directory, ".emqx-api-key.conf.tmp-*"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(temporary) != 0 {
		t.Fatalf("temporary files remain after ownership failure: %v", temporary)
	}
}

func TestWriteEMQXBootstrapSecretRejectsUnsafeInputsWithoutReplacingOldSecret(t *testing.T) {
	runtimeRoot := t.TempDir()
	filename := filepath.Join(runtimeRoot, "secrets", "emqx-api-key.conf")
	// This fixture tests input rejection, not the caller's ability to chown.
	if err := os.Mkdir(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte("stable-key:stable-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		apiKey string
		secret string
	}{
		{name: "empty key", secret: "fixture-secret"},
		{name: "empty secret", apiKey: "fixture-key"},
		{name: "key delimiter", apiKey: "fixture:key", secret: "fixture-secret"},
		{name: "secret delimiter", apiKey: "fixture-key", secret: "fixture:secret"},
		{name: "key newline", apiKey: "fixture\nkey", secret: "fixture-secret"},
		{name: "secret carriage return", apiKey: "fixture-key", secret: "fixture\rsecret"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := WriteEMQXBootstrapSecret(runtimeRoot, tt.apiKey, tt.secret)
			if err == nil {
				t.Fatal("WriteEMQXBootstrapSecret() accepted unsafe input")
			}
			if tt.apiKey != "" && strings.Contains(err.Error(), tt.apiKey) || tt.secret != "" && strings.Contains(err.Error(), tt.secret) {
				t.Fatalf("WriteEMQXBootstrapSecret() leaked input: %v", err)
			}
			assertEMQXBootstrapSecret(t, filename, "stable-key:stable-secret\n")
		})
	}
}

func TestWriteEMQXBootstrapSecretRejectsUnsafeDirectoryBoundary(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(string) error
	}{
		{name: "symbolic link", setup: func(path string) error { return os.Symlink(t.TempDir(), path) }},
		{name: "regular file", setup: func(path string) error { return os.WriteFile(path, []byte("not-a-directory"), 0o600) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runtimeRoot := t.TempDir()
			if err := tt.setup(filepath.Join(runtimeRoot, "secrets")); err != nil {
				t.Fatal(err)
			}
			if err := WriteEMQXBootstrapSecret(runtimeRoot, "fixture-key", "fixture-secret"); err == nil {
				t.Fatal("WriteEMQXBootstrapSecret() accepted unsafe secret directory")
			}
		})
	}
}

func assertEMQXBootstrapSecret(t *testing.T, filename, want string) {
	t.Helper()
	info, err := os.Lstat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
		t.Fatalf("secret file mode = %v", info.Mode())
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("secret file content = %q, want %q", data, want)
	}
}

func TestStageCreatesFiveIsolatedContexts(t *testing.T) {
	contract := validContractForTest()
	runtimeRoot := t.TempDir()

	result, err := Stage(StageRequest{
		RepoRoot:    stageFixtureRepository(t),
		RuntimeRoot: runtimeRoot,
		Contract:    &contract,
	})
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	if len(result.Contexts) != 5 {
		t.Fatalf("Stage() contexts = %d, want 5", len(result.Contexts))
	}
	for _, context := range result.Contexts {
		if _, err := os.Stat(context.Path); err != nil {
			t.Fatalf("staged %s context: %v", context.Role, err)
		}
	}
	assertStageFiles(t, runtimeRoot, []string{
		"api/bin/thingsvr",
		"api/etc/api.yaml",
		"api/etc/i18n/locale.txt",
		"dg/bin/dgsvr",
		"dg/etc/dg.yaml",
		"dm/bin/dmsvr",
		"dm/etc/dm.yaml",
		"dm/etc/i18n/locale.txt",
		"dm/etc/init/init.txt",
		"sys/bin/syssvr",
		"sys/etc/i18n/locale.txt",
		"sys/etc/init/init.txt",
		"sys/etc/sys.yaml",
		"timedjob/bin/timedjobsvr",
		"timedjob/etc/timedjob.yaml",
	})
	assertStageManifest(t, runtimeRoot)

	stale := filepath.Join(runtimeRoot, "stage", "dm", "unexpected-file")
	if err := os.WriteFile(stale, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale stage fixture: %v", err)
	}
	repoRoot := stageFixtureRepository(t)
	if _, err := Stage(StageRequest{RepoRoot: repoRoot, RuntimeRoot: runtimeRoot, Contract: &contract}); err != nil {
		t.Fatalf("restage: %v", err)
	}
	if _, err := os.Lstat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale staged file error = %v, want not exist", err)
	}
}

func TestStageTemplatesUseBaselineSafetySettings(t *testing.T) {
	root := stageTestProjectRoot(t)
	for relative, want := range map[string]string{
		"deploy/profiles/things-baseline/.env.example":              "POSTGRES_ADMIN_HOST=host.docker.internal\nPOSTGRES_ADMIN_PORT=5432\nPOSTGRES_ADMIN_USER=postgres\nPOSTGRES_ADMIN_PASSWORD=\nTHINGS_BASELINE_DB=ming_iot_things_baseline\nTHINGS_BASELINE_DB_USER=ming_iot_things_baseline\nTHINGS_BASELINE_DB_PASSWORD=\nCORE_USER_TOKEN_SECRET=\nTHINGS_INTERNAL_AUTH_PASSWORD=\nTHINGS_EMQX_API_KEY=\nTHINGS_EMQX_SECRET_KEY=\nTDENGINE_ROOT_PASSWORD=taosdata\nTHINGS_BASELINE_API_PORT=17788\nTHINGS_BASELINE_MQTT_PORT=11883\nTHINGS_BASELINE_EMQX_DASHBOARD_PORT=18084\n",
		"deploy/profiles/things-baseline/config/saas/sys.yaml":      "Name: sys.rpc\nListenOn: 0.0.0.0:9180\nMode: dev\nLog:\n  Encoding: plain\nEtcd: {Hosts: [etcd:2379], Key: sys.rpc}\nDatabase: {DBType: pgsql, IsInitTable: true, DSN: \"${CORE_DB_DSN}\"}\nCacheRedis: [{Host: \"redis:6379\", Type: node}]\nEvent: {Mode: natsJs, Nats: {Url: \"nats://nats:4222\", Consumer: syssvr}}\nOssConf: {OssType: local, StorePath: /app/oss}\nUserToken: {AccessSecret: \"${CORE_USER_TOKEN_SECRET}\", AccessExpire: 3600000}\nMap: {Mode: gaode, AccessKey: baseline-disabled, AccessSecret: baseline-disabled}\nSms: {Enable: false}\nTimedJobRpc:\n  Enable: true\n  Mode: grpc\n  Conf: {Timeout: 1000000, Etcd: {Hosts: [etcd:2379], Key: timedjob.rpc}}\n",
		"deploy/profiles/things-baseline/config/saas/timedjob.yaml": "Name: timedjob.rpc\nListenOn: 0.0.0.0:3256\nMode: dev\nLog:\n  Encoding: plain\nEtcd: {Hosts: [etcd:2379], Key: timedjob.rpc}\nDatabase: {DBType: pgsql, IsInitTable: true, DSN: \"${CORE_DB_DSN}\"}\nCacheRedis: [{Host: \"redis:6379\", Type: node}]\nEvent: {Mode: natsJs, Nats: {Url: \"nats://nats:4222\", Consumer: timedjobsvr}}\n",
		"deploy/profiles/things-baseline/config/things/dm.yaml":     "Name: dm.rpc\nListenOn: 0.0.0.0:9081\nMode: dev\nLog:\n  Encoding: plain\nEtcd: {Hosts: [etcd:2379], Key: dm.rpc}\nDatabase: {DBType: pgsql, IsInitTable: true, DSN: \"${THINGS_DB_DSN}\"}\nCacheRedis: [{Host: \"redis:6379\", Type: node}]\nTSDB: {DBType: tdengine, Driver: taosRestful, DSN: \"${THINGS_TSDB_DSN}\"}\nEvent: {Mode: natsJs, Nats: {Url: \"nats://nats:4222\", Consumer: dmsvr}}\nOssConf: {OssType: local, StorePath: /app/oss}\nAuthWhite:\n  Users: [{UserName: dgsvr, Password: \"${THINGS_INTERNAL_AUTH_PASSWORD}\"}]\n  IpRange: [\"127.0.0.0/8\", \"172.16.0.0/12\", \"10.0.0.0/8\"]\nTimedJobRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Etcd: {Hosts: [etcd:2379], Key: timedjob.rpc}}}\nSysRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Etcd: {Hosts: [etcd:2379], Key: sys.rpc}}}\n",
		"deploy/profiles/things-baseline/config/things/dg.yaml":     "Name: dg.rpc\nListenOn: 0.0.0.0:6166\nMode: dev\nLog:\n  Encoding: plain\nCacheRedis: [{Host: \"redis:6379\", Type: node}]\nEvent: {Mode: natsJs, Nats: {Url: \"nats://nats:4222\", Consumer: dgsvr}}\nDmRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Endpoints: [\"dm:9081\"]}}\nTimedJobRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Endpoints: [\"timedjob:3256\"]}}\nDevLink:\n  Mode: mqtt\n  SubMode: emq\n  Mqtt: {ClientID: dg.rpc, User: dgsvr, Pass: \"${THINGS_INTERNAL_AUTH_PASSWORD}\", Brokers: [\"tcp://emqx:1883\"]}\n",
		"deploy/profiles/things-baseline/config/things/api.yaml":    "Name: api\nHost: 0.0.0.0\nPort: 7788\nMode: dev\nLog:\n  Encoding: plain\nEtcd: {Hosts: [etcd:2379], Key: dm.rpc}\nCacheRedis: [{Host: \"redis:6379\", Type: node}]\nEvent: {Mode: natsJs, Nats: {Url: \"nats://nats:4222\", Consumer: things-api}}\nOssConf: {OssType: local, StorePath: /app/oss}\nSysRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Etcd: {Hosts: [etcd:2379], Key: sys.rpc}}}\nDmRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Etcd: {Hosts: [etcd:2379], Key: dm.rpc}}}\nTimedJobRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Etcd: {Hosts: [etcd:2379], Key: timedjob.rpc}}}\nDgRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Endpoints: [\"dg:6166\"]}}\n",
	} {
		data, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		if got := string(data); got != want {
			t.Fatalf("%s differs from the locked baseline template", relative)
		}
		if strings.Contains(string(data), "TimedSchedulerRpc") {
			t.Fatalf("%s contains retired TimedSchedulerRpc", relative)
		}
	}
}

func assertStageFiles(t *testing.T, runtimeRoot string, want []string) {
	t.Helper()
	stageRoot := filepath.Join(runtimeRoot, "stage")
	var got []string
	err := filepath.WalkDir(stageRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Base(path) == "stage-manifest.json" {
			return nil
		}
		relative, err := filepath.Rel(stageRoot, path)
		if err != nil {
			return err
		}
		got = append(got, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatalf("walk stage: %v", err)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("staged files = %v, want %v", got, want)
	}
}

func assertStageManifest(t *testing.T, runtimeRoot string) {
	t.Helper()
	stageRoot := filepath.Join(runtimeRoot, "stage")
	data, err := os.ReadFile(filepath.Join(stageRoot, "stage-manifest.json"))
	if err != nil {
		t.Fatalf("read stage manifest: %v", err)
	}
	var manifest stageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode stage manifest: %v", err)
	}
	if manifest.SchemaVersion != 1 || len(manifest.Files) == 0 {
		t.Fatalf("stage manifest = %+v, want version 1 and files", manifest)
	}
	if strings.Contains(string(data), "Name: sys.rpc") || strings.Contains(string(data), "CORE_USER_TOKEN_SECRET") {
		t.Fatal("stage manifest contains staged file contents")
	}
	want := make(map[string]stagedFile)
	err = filepath.WalkDir(stageRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Base(path) == "stage-manifest.json" {
			return nil
		}
		relative, err := filepath.Rel(stageRoot, path)
		if err != nil {
			return err
		}
		fileData, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		digest := sha256.Sum256(fileData)
		key := filepath.ToSlash(relative)
		want[key] = stagedFile{Path: key, Mode: fmt.Sprintf("%04o", info.Mode().Perm()), SHA256: hex.EncodeToString(digest[:])}
		return nil
	})
	if err != nil {
		t.Fatalf("walk stage manifest inputs: %v", err)
	}
	if len(manifest.Files) != len(want) {
		t.Fatalf("stage manifest file count = %d, want %d", len(manifest.Files), len(want))
	}
	previousPath := ""
	for _, file := range manifest.Files {
		if strings.Contains(file.Path, "..") || file.SHA256 == "" || file.Mode == "" {
			t.Fatalf("unsafe stage manifest file = %+v", file)
		}
		if previousPath >= file.Path {
			t.Fatalf("stage manifest paths are not strictly sorted: %q then %q", previousPath, file.Path)
		}
		previousPath = file.Path
		if got, ok := want[file.Path]; !ok || got != file {
			t.Fatalf("stage manifest file %q = %+v, want %+v", file.Path, file, got)
		}
	}
}

func writeStageFixtureFile(t *testing.T, filename, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatalf("create stage fixture parent %s: %v", filename, err)
	}
	if err := os.WriteFile(filename, []byte(value), 0o600); err != nil {
		t.Fatalf("write stage fixture %s: %v", filename, err)
	}
}

func assertStageFixtureFile(t *testing.T, filename, want string) {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read stage fixture %s: %v", filename, err)
	}
	if got := string(data); got != want {
		t.Fatalf("stage fixture %s = %q, want %q", filename, got, want)
	}
}

func stageFixtureRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	contract := validContractForTest()
	for _, role := range contract.Roles {
		writeStageFixtureFile(t, filepath.Join(root, "backend", "bin", role.Binary), role.Binary)
		if err := os.Chmod(filepath.Join(root, "backend", "bin", role.Binary), 0o755); err != nil {
			t.Fatalf("chmod fixture binary: %v", err)
		}
		writeStageFixtureFile(t, filepath.Join(root, role.Config), "Name: "+role.ID+"\n")
		assets := roleStageAssetPaths[role.ID]
		for _, asset := range []string{assets.i18n, assets.init} {
			if asset != "" {
				writeStageFixtureFile(t, filepath.Join(root, asset, map[bool]string{true: "locale.txt", false: "init.txt"}[strings.HasSuffix(asset, "i18n")]), role.ID)
			}
		}
	}
	return root
}

func stageTestProjectRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate stage test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "../../../.."))
}
