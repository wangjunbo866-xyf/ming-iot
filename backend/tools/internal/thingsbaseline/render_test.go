package thingsbaseline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"
)

func TestRenderTargetTemplatesAreStableAndComplete(t *testing.T) {
	_, lock, snapshot, dependencies := syntheticRenderFixture(t)
	report := TransformReport{CorePrefixes: 6, ThingsPrefixes: 3, CredentialLines: 1, PrivateKeyBlocks: 1, VetFixes: 1, SchemaFixtureIdentifiers: 2, NonYAMLCredentialValues: 5}

	first, err := renderTargetTemplates(&lock, snapshot, report)
	if err != nil {
		t.Fatalf("renderTargetTemplates() error: %v", err)
	}
	second, err := renderTargetTemplates(&lock, snapshot, report)
	if err != nil {
		t.Fatalf("renderTargetTemplates() second error: %v", err)
	}
	if len(first) != 4 || !renderedFilesEqual(first, second) {
		t.Fatalf("templates are not four deterministic files: first=%v second=%v", sortedRenderedPaths(first), sortedRenderedPaths(second))
	}
	for _, path := range []string{"README.md", "NOTICE", "PROVENANCE.yaml", "Makefile"} {
		file, ok := first[path]
		if !ok || file.Mode != "100644" || len(file.Data) == 0 {
			t.Fatalf("template %q = %+v, want non-empty mode 100644", path, file)
		}
		if bytes.Contains(file.Data, []byte("{{")) || bytes.Contains(file.Data, []byte("}}")) {
			t.Fatalf("template %q retained a token", path)
		}
	}
	if got := string(first["NOTICE"].Data); !strings.Contains(got, lock.Source.Repository) || !strings.Contains(got, lock.Source.Commit) || !strings.Contains(got, "AGPL-3.0") || strings.Contains(strings.ToLower(got), "apache") {
		t.Fatalf("NOTICE provenance/license contract failed:\n%s", got)
	}
	if got := string(first["PROVENANCE.yaml"].Data); strings.Contains(got, dependencies.backendAbsolutePathSentinel) || strings.Contains(got, ".thingsbaseline-") {
		t.Fatalf("PROVENANCE leaked local/random path:\n%s", got)
	}
	if got := string(first["PROVENANCE.yaml"].Data); !strings.Contains(got, "schemaFixtureIdentifierCount: 2\n") || strings.Contains(got, "{{SCHEMA_FIXTURE_IDENTIFIER_COUNT}}") {
		t.Fatalf("PROVENANCE schema fixture transform count is missing or unresolved:\n%s", got)
	}
	if got := string(first["PROVENANCE.yaml"].Data); !strings.Contains(got, "nonYamlCredentialValueCount: 5\n") || strings.Contains(got, "{{NON_YAML_CREDENTIAL_VALUE_COUNT}}") {
		t.Fatalf("PROVENANCE non-YAML credential transform count is missing or unresolved:\n%s", got)
	}
	want := templateGoldens(lock, snapshot, report)
	for path, wantData := range want {
		if got := string(first[path].Data); got != wantData {
			t.Fatalf("template %s is not byte-golden:\n--- got ---\n%s\n--- want ---\n%s", path, got, wantData)
		}
	}
}

func TestProvenanceTransformsMatchTransformReportJSONExactly(t *testing.T) {
	_, lock, snapshot, _ := syntheticRenderFixture(t)
	report := TransformReport{
		CorePrefixes:             2,
		ThingsPrefixes:           3,
		CredentialLines:          5,
		PrivateKeyBlocks:         7,
		VetFixes:                 11,
		SchemaFixtureIdentifiers: 13,
		NonYAMLCredentialValues:  17,
	}

	rendered, err := renderTargetTemplates(&lock, snapshot, report)
	if err != nil {
		t.Fatalf("renderTargetTemplates() error: %v", err)
	}
	reportJSON, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal(TransformReport) error: %v", err)
	}
	var want map[string]int
	if err := json.Unmarshal(reportJSON, &want); err != nil {
		t.Fatalf("json.Unmarshal(TransformReport) error: %v", err)
	}
	var provenance struct {
		Transforms map[string]int `yaml:"transforms"`
	}
	if err := yaml.Unmarshal(rendered["PROVENANCE.yaml"].Data, &provenance); err != nil {
		t.Fatalf("yaml.Unmarshal(PROVENANCE.yaml) error: %v", err)
	}
	if !reflect.DeepEqual(provenance.Transforms, want) {
		t.Fatalf("PROVENANCE transforms = %#v, want exact TransformReport JSON map %#v", provenance.Transforms, want)
	}
}

func TestRenderTemplateDataRejectsUnknownOrInjectedTokens(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		data   string
		tokens map[string]string
	}{
		{"unknown", "value={{UNKNOWN}}\n", map[string]string{}},
		{"schema fixture count", "value={{SCHEMA_FIXTURE_IDENTIFIER_COUNT}}\n", map[string]string{}},
		{"non-YAML credential count", "value={{NON_YAML_CREDENTIAL_VALUE_COUNT}}\n", map[string]string{}},
		{"injected by value", "value={{KNOWN}}\n", map[string]string{"{{KNOWN}}": "{{UNKNOWN}}"}},
		{"unbalanced open", "value={{BROKEN\n", map[string]string{}},
		{"unbalanced close", "value=BROKEN}}\n", map[string]string{}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if _, err := renderTemplateData("fixture", []byte(fixture.data), fixture.tokens); err == nil {
				t.Fatalf("renderTemplateData() accepted %q", fixture.data)
			}
		})
	}
}

func TestRenderCandidateTransformsBeforeCreatingDiskState(t *testing.T) {
	backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
	lock.Transforms.PrivateKeySourceSHA256 = strings.Repeat("0", 64)

	candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
	if err == nil || candidate != nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("renderCandidateWithDependencies() = %v, %v; want pre-disk transform failure", candidate, err)
	}
	assertDirectoryEmpty(t, backend)
	if len(dependencies.runner.calls) != 0 {
		t.Fatalf("go runner called before transform succeeded: %+v", dependencies.runner.calls)
	}
}

func TestRenderCandidateRejectsInvalidSnapshotAndTemplateBeforeDiskState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Lock, *SourceSnapshot)
	}{
		{"commit", func(_ *Lock, snapshot *SourceSnapshot) { snapshot.Commit = strings.Repeat("f", 40) }},
		{"tree", func(_ *Lock, snapshot *SourceSnapshot) { snapshot.Tree = strings.Repeat("e", 40) }},
		{"file count", func(_ *Lock, snapshot *SourceSnapshot) { snapshot.Files = snapshot.Files[:len(snapshot.Files)-1] }},
		{"blob byte count", func(_ *Lock, snapshot *SourceSnapshot) { snapshot.SelectedBlobBytes++ }},
		{"unsorted paths", func(_ *Lock, snapshot *SourceSnapshot) {
			snapshot.Files[0], snapshot.Files[1] = snapshot.Files[1], snapshot.Files[0]
		}},
		{"unsupported mode", func(_ *Lock, snapshot *SourceSnapshot) { snapshot.Files[0].Mode = "120000" }},
		{"template token injection", func(lock *Lock, _ *SourceSnapshot) { lock.Source.Repository = "https://example.invalid/{{UNKNOWN}}" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
			tt.mutate(&lock, &snapshot)
			candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
			if err == nil || candidate != nil {
				t.Fatalf("render = %v, %v; want pre-disk failure", candidate, err)
			}
			assertDirectoryEmpty(t, backend)
			if len(dependencies.runner.calls) != 0 {
				t.Fatalf("runner called: %+v", dependencies.runner.calls)
			}
		})
	}
}

func TestRenderCandidateSealsSanitizedTreeAndReturnsDeepCopyReport(t *testing.T) {
	backend, lock, snapshot, dependencies := syntheticRenderFixture(t)

	candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
	if err != nil {
		t.Fatalf("renderCandidateWithDependencies() error: %v", err)
	}
	t.Cleanup(func() { _ = candidate.Cleanup() })
	if !candidate.sealed {
		t.Fatal("candidate was returned before sealing")
	}
	report := candidate.Report()
	wantFiles := len(snapshot.Files) + 4
	if len(report.PreTidyManifest) != wantFiles || len(report.Manifest) != wantFiles || report.Scan.RegularFiles != wantFiles {
		t.Fatalf("candidate file counts = pre %d final %d scan %d, want %d", len(report.PreTidyManifest), len(report.Manifest), report.Scan.RegularFiles, wantFiles)
	}
	if report.TreeDigest == "" || report.GoVersion != "go1.25.4" || report.Transform.CredentialLines != 1 || report.Transform.NonYAMLCredentialValues != 5 {
		t.Fatalf("candidate report incomplete: %+v", report)
	}
	if len(report.Commands) != 2 || report.Commands[0].Name != "go-mod-tidy" || report.Commands[1].Name != "go-mod-verify" {
		t.Fatalf("command report = %+v, want tidy then verify", report.Commands)
	}
	if report.Credentials != (CredentialProfileReport{Files: 1, Active: 1, Paths: CredentialPathCounts{DatabaseDSN: 1}}) {
		t.Fatalf("credential report = %+v", report.Credentials)
	}
	encodedReport, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{backend, candidate.path, ".thingsbaseline-", ".thingsbaseline-cache-"} {
		if strings.Contains(string(encodedReport), forbidden) {
			t.Fatalf("Candidate.Report() leaked local/random path %q", forbidden)
		}
	}

	report.Manifest[0].Path = "mutated-by-caller"
	report.Commands[0].Name = "mutated-by-caller"
	again := candidate.Report()
	if again.Manifest[0].Path == "mutated-by-caller" || again.Commands[0].Name == "mutated-by-caller" {
		t.Fatal("Candidate.Report() exposed mutable internal slices")
	}

	root := candidate.path
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, forbidden := range [][]byte{[]byte("literal-db-value"), []byte("fixture-body"), privateKeyBeginMarker(), []byte(testCoreModule)} {
			if bytes.Contains(data, forbidden) {
				return fmt.Errorf("%s retained forbidden source material", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := candidate.Cleanup(); err != nil {
		t.Fatalf("Candidate.Cleanup() error: %v", err)
	}
	if err := candidate.Cleanup(); err != nil {
		t.Fatalf("Candidate.Cleanup() second call: %v", err)
	}
	assertDirectoryEmpty(t, backend)
}

func TestCandidatePublishIsAtomicNoReplaceAndCleanupIsDisarmed(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
		candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
		if err != nil {
			t.Fatalf("render candidate: %v", err)
		}
		before := candidate.Report()
		oldCandidatePath := candidate.path
		if err := candidate.Publish(); err != nil {
			t.Fatalf("Candidate.Publish() error: %v", err)
		}
		if _, err := os.Stat(filepath.Join(backend, "things", "PROVENANCE.yaml")); err != nil {
			t.Fatalf("published target is incomplete: %v", err)
		}
		if _, err := os.Lstat(oldCandidatePath); !os.IsNotExist(err) {
			t.Fatalf("old candidate name still exists: %v", err)
		}
		if err := candidate.Cleanup(); err != nil {
			t.Fatalf("cleanup published candidate: %v", err)
		}
		if _, err := os.Stat(filepath.Join(backend, "things", "LICENSE")); err != nil {
			t.Fatalf("Cleanup() removed published target: %v", err)
		}
		if after := candidate.Report(); after.TreeDigest != before.TreeDigest {
			t.Fatalf("published report changed: before=%+v after=%+v", before, after)
		}
	})

	t.Run("concurrent target", func(t *testing.T) {
		backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
		candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
		if err != nil {
			t.Fatalf("render candidate: %v", err)
		}
		t.Cleanup(func() { _ = candidate.Cleanup() })
		target := filepath.Join(backend, "things")
		want := []byte("concurrent target must survive\n")
		if err := os.WriteFile(target, want, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := candidate.Publish(); err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("Candidate.Publish() error = %v, want no-replace rejection", err)
		}
		got, err := os.ReadFile(target)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("concurrent target changed: got %q, err=%v", got, err)
		}
	})

	t.Run("EXDEV has no copy fallback", func(t *testing.T) {
		backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
		dependencies.renameNoReplace = func(int, string, int, string) error { return unix.EXDEV }
		candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
		if err != nil {
			t.Fatalf("render candidate: %v", err)
		}
		t.Cleanup(func() { _ = candidate.Cleanup() })
		if err := candidate.Publish(); err == nil || !strings.Contains(err.Error(), "cross-device") {
			t.Fatalf("Candidate.Publish() error = %v, want EXDEV", err)
		}
		if _, err := os.Lstat(filepath.Join(backend, "things")); !os.IsNotExist(err) {
			t.Fatalf("EXDEV created target via fallback: %v", err)
		}
	})
}

func TestCandidateFreezesCallerLockAndSnapshotBeforeSealing(t *testing.T) {
	backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
	candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
	if err != nil {
		t.Fatalf("render candidate: %v", err)
	}
	t.Cleanup(func() { _ = candidate.Cleanup() })
	wantDigest := candidate.Report().TreeDigest

	lock.Source.Module = "attacker.invalid/module"
	lock.Source.Exclude = append(lock.Source.Exclude, "service")
	lock.Transforms.CredentialPaths.DatabaseDSN = 999
	for index := range snapshot.Files {
		if len(snapshot.Files[index].Data) > 0 {
			snapshot.Files[index].Data[0] ^= 0xff
		}
	}
	if candidate.Report().TreeDigest != wantDigest {
		t.Fatal("caller mutation changed sealed report")
	}
	if err := candidate.Publish(); err != nil {
		t.Fatalf("caller mutation changed publish policy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(backend, "things", "LICENSE")); err != nil {
		t.Fatalf("frozen candidate did not publish: %v", err)
	}
}

func TestRenderCandidateRejectsEveryPreexistingTargetKind(t *testing.T) {
	tests := []struct {
		name   string
		create func(*testing.T, string)
	}{
		{"file", func(t *testing.T, path string) { t.Helper(); mustWriteFile(t, path, []byte("keep"), 0o600) }},
		{"symlink", func(t *testing.T, path string) {
			t.Helper()
			destination := filepath.Join(t.TempDir(), "destination")
			mustWriteFile(t, destination, []byte("keep"), 0o600)
			if err := os.Symlink(destination, path); err != nil {
				t.Fatal(err)
			}
		}},
		{"empty directory", func(t *testing.T, path string) {
			t.Helper()
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"non-empty directory", func(t *testing.T, path string) {
			t.Helper()
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			mustWriteFile(t, filepath.Join(path, "keep"), []byte("keep"), 0o600)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
			target := filepath.Join(backend, "things")
			tt.create(t, target)
			before, err := os.Lstat(target)
			if err != nil {
				t.Fatal(err)
			}

			candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
			if err == nil || candidate != nil || !strings.Contains(err.Error(), "already exists") {
				t.Fatalf("renderCandidateWithDependencies() = %v, %v; want existing-target rejection", candidate, err)
			}
			after, err := os.Lstat(target)
			if err != nil || before.Mode() != after.Mode() || before.Size() != after.Size() {
				t.Fatalf("preexisting target changed: before=%v after=%v err=%v", before, after, err)
			}
		})
	}
}

func TestVerificationCandidateRequiresRealTargetAndCanNeverPublish(t *testing.T) {
	t.Run("existing real target", func(t *testing.T) {
		backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
		target := filepath.Join(backend, "things")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}
		candidate, err := renderVerificationCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
		if err != nil {
			t.Fatalf("renderVerificationCandidateWithDependencies() error: %v", err)
		}
		defer candidate.Cleanup()
		if err := candidate.Publish(); err == nil || !strings.Contains(err.Error(), "verification candidate") {
			t.Fatalf("verification Candidate.Publish() error = %v", err)
		}
		assertDirectoryEmpty(t, target)
	})

	for _, kind := range []string{"missing", "file", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
			target := filepath.Join(backend, "things")
			switch kind {
			case "file":
				mustWriteFile(t, target, []byte("keep\n"), 0o600)
			case "symlink":
				if err := os.Symlink(t.TempDir(), target); err != nil {
					t.Fatal(err)
				}
			}
			candidate, err := renderVerificationCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
			if err == nil || candidate != nil {
				t.Fatalf("verification render for %s target = %v, %v", kind, candidate, err)
			}
			entries, readErr := os.ReadDir(backend)
			if readErr != nil {
				t.Fatal(readErr)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".thingsbaseline-") {
					t.Fatalf("failed verification render left %q", entry.Name())
				}
			}
		})
	}
}

func TestRenderCandidateRejectsSymlinkedBackendAndNestedTarget(t *testing.T) {
	t.Run("backend symlink", func(t *testing.T) {
		_, lock, snapshot, dependencies := syntheticRenderFixture(t)
		realBackend := t.TempDir()
		link := filepath.Join(t.TempDir(), "backend-link")
		if err := os.Symlink(realBackend, link); err != nil {
			t.Fatal(err)
		}
		candidate, err := renderCandidateWithDependencies(context.Background(), link, snapshot, &lock, dependencies.renderDependencies)
		if err == nil || candidate != nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("symlink backend render = %v, %v", candidate, err)
		}
		assertDirectoryEmpty(t, realBackend)
	})

	t.Run("nested target", func(t *testing.T) {
		backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
		lock.Target.Directory = "nested/things"
		candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
		if err == nil || candidate != nil || !strings.Contains(err.Error(), "direct backend child") {
			t.Fatalf("nested target render = %v, %v", candidate, err)
		}
		assertDirectoryEmpty(t, backend)
	})
}

func TestRenderCandidateUsesFixedEnvironmentAndTwoRendersAreIdentical(t *testing.T) {
	t.Setenv("MING_GOPROXY", "")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOPROXY", "https://user:password@example.invalid")
	t.Setenv("GIT_CONFIG_GLOBAL", "/hostile/gitconfig")
	t.Setenv("GIT_TRACE", "1")
	t.Setenv("HOME", "/hostile/home")

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
	firstEnvironment := environmentMap(dependenciesOne.runner.calls[0].Env)
	assertRenderPrivateDirectoriesRemoved(t, firstEnvironment)
	assertSharedGoCacheDirectories(t, shared)
	second, err := renderCandidateWithDependencies(context.Background(), backendTwo, snapshotTwo, &lockTwo, dependenciesTwo.renderDependencies)
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	t.Cleanup(func() { _ = second.Cleanup() })
	secondEnvironment := environmentMap(dependenciesTwo.runner.calls[0].Env)
	assertRenderPrivateDirectoriesRemoved(t, secondEnvironment)
	assertSharedGoCacheDirectories(t, shared)

	firstReport, secondReport := first.Report(), second.Report()
	if firstReport.TreeDigest != secondReport.TreeDigest || !manifestEntriesEqual(firstReport.Manifest, secondReport.Manifest) || !manifestEntriesEqual(firstReport.PreTidyManifest, secondReport.PreTidyManifest) {
		t.Fatalf("independent renders differ:\nfirst=%+v\nsecond=%+v", firstReport, secondReport)
	}
	if len(dependenciesOne.runner.calls) != 2 || len(dependenciesTwo.runner.calls) != 2 {
		t.Fatalf("runner calls = %d/%d, want 2/2", len(dependenciesOne.runner.calls), len(dependenciesTwo.runner.calls))
	}
	for _, runner := range []*recordingGoRunner{dependenciesOne.runner, dependenciesTwo.runner} {
		for _, call := range runner.calls {
			env := environmentMap(call.Env)
			for key, want := range map[string]string{
				"GOENV": "off", "GOFLAGS": "", "GOWORK": "off", "GOTOOLCHAIN": "local",
				"GOPROXY": "https://goproxy.cn,direct", "GOSUMDB": "sum.golang.org",
				"GOPRIVATE": "", "GONOPROXY": "", "GONOSUMDB": "", "GOINSECURE": "",
				"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_TRACE": "",
			} {
				if got := env[key]; got != want {
					t.Fatalf("env[%s] = %q, want %q", key, got, want)
				}
			}
			joined := strings.Join(call.Env, "\n")
			for _, forbidden := range []string{"user:password", "/hostile/gitconfig", "/hostile/home", "GIT_TRACE=1", "GOFLAGS=-mod=mod"} {
				if strings.Contains(joined, forbidden) {
					t.Fatalf("controlled env retained %q", forbidden)
				}
			}
			if call.Path != "/trusted/go" || call.Dir == "" {
				t.Fatalf("go command boundary = %+v", call)
			}
			if env["GOCACHE"] != shared.Build || env["GOMODCACHE"] != shared.Modules {
				t.Fatalf("shared cache environment = %q/%q, want %q/%q", env["GOCACHE"], env["GOMODCACHE"], shared.Build, shared.Modules)
			}
		}
	}
	for _, key := range []string{"HOME", "GOTMPDIR"} {
		if firstEnvironment[key] == "" || secondEnvironment[key] == "" || firstEnvironment[key] == secondEnvironment[key] {
			t.Fatalf("private %s values = %q and %q, want nonempty and different", key, firstEnvironment[key], secondEnvironment[key])
		}
	}
	for _, candidate := range []*Candidate{first, second} {
		if err := candidate.Cleanup(); err != nil {
			t.Fatalf("Candidate.Cleanup() error: %v", err)
		}
	}
	assertSharedGoCacheDirectories(t, shared)
	assertDirectoryEmpty(t, backendOne)
	assertDirectoryEmpty(t, backendTwo)
}

func TestFixedCandidateGoEnvironmentUsesMingGoProxyAndPreservesTransportProxy(t *testing.T) {
	t.Setenv("GOPROXY", "https://hostile.example.invalid")
	t.Setenv("MING_GOPROXY", "https://proxy.golang.org,direct")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:7890")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:7890")
	t.Setenv("http_proxy", "http://127.0.0.1:7891")
	t.Setenv("https_proxy", "http://127.0.0.1:7891")

	environment, err := fixedCandidateGoEnvironment(t.TempDir(), testSharedGoCachePaths(t))
	if err != nil {
		t.Fatalf("fixedCandidateGoEnvironment() error: %v", err)
	}
	env := environmentMap(environment)
	if got := env["GOPROXY"]; got != "https://proxy.golang.org,direct" {
		t.Fatalf("child GOPROXY = %q, want MING_GOPROXY value", got)
	}
	for key, want := range map[string]string{
		"HTTP_PROXY":  "http://127.0.0.1:7890",
		"HTTPS_PROXY": "http://127.0.0.1:7890",
		"http_proxy":  "http://127.0.0.1:7891",
		"https_proxy": "http://127.0.0.1:7891",
	} {
		if got := env[key]; got != want {
			t.Fatalf("child %s = %q, want %q", key, got, want)
		}
	}
	if _, ok := env["MING_GOPROXY"]; ok {
		t.Fatalf("child environment retained MING_GOPROXY")
	}
}

func TestFixedCandidateGoEnvironmentRejectsMissingSharedCache(t *testing.T) {
	privateRoot := t.TempDir()
	_, err := fixedCandidateGoEnvironment(privateRoot)
	if err == nil || !strings.Contains(err.Error(), "shared Go cache paths are unavailable") {
		t.Fatalf("fixedCandidateGoEnvironment() error = %v, want missing shared-cache rejection", err)
	}
	for _, path := range []string{filepath.Join(privateRoot, "gocache"), filepath.Join(privateRoot, "gomodcache")} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("legacy private cache %q exists or has unexpected error: %v", path, statErr)
		}
	}
}

func TestFixedCandidateGoEnvironmentRejectsUnsafeMingGoProxyWithoutEchoingValue(t *testing.T) {
	for _, tt := range []struct {
		name          string
		value         string
		wantErrorPart string
	}{
		{name: "plain HTTP", value: "http://proxy.example.invalid", wantErrorPart: "must use HTTPS"},
		{name: "credentials", value: "https://user:secret@proxy.example.invalid", wantErrorPart: "must not contain credentials"},
		{name: "non-empty query", value: "https://proxy.example.invalid?token=secret", wantErrorPart: "must not contain a query"},
		{name: "empty query delimiter", value: "https://proxy.example.invalid?", wantErrorPart: "must not contain a query"},
		{name: "empty fragment delimiter", value: "https://proxy.example.invalid#", wantErrorPart: "must not contain a fragment"},
		{name: "trailing empty second entry", value: "https://proxy.example.invalid|", wantErrorPart: "entry 2 is empty"},
		{name: "relative URL", value: "proxy.example.invalid", wantErrorPart: "must be an absolute HTTPS URL"},
		{name: "malformed URL escape", value: "https://proxy.example.invalid/%zz", wantErrorPart: "must be an absolute HTTPS URL"},
		{name: "missing host", value: "https://", wantErrorPart: "must include a host"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MING_GOPROXY", tt.value)
			_, err := fixedCandidateGoEnvironment(t.TempDir(), testSharedGoCachePaths(t))
			if err == nil {
				t.Fatalf("fixedCandidateGoEnvironment() accepted unsafe value")
			}
			if !strings.Contains(err.Error(), tt.wantErrorPart) {
				t.Fatalf("error = %v, want safe fragment %q", err, tt.wantErrorPart)
			}
			if strings.Contains(err.Error(), tt.value) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("error echoed configured value: %v", err)
			}
		})
	}
}

func TestResolveCandidateGoProxyAcceptsSupportedEntries(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: "https://goproxy.cn,direct"},
		{name: "direct", value: "direct", want: "direct"},
		{name: "off", value: "off", want: "off"},
		{name: "https proxy comma direct", value: "https://proxy.golang.org,direct", want: "https://proxy.golang.org,direct"},
		{name: "https proxy pipe direct", value: "https://proxy.golang.org|direct", want: "https://proxy.golang.org|direct"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MING_GOPROXY", tt.value)
			got, err := resolveCandidateGoProxy()
			if err != nil {
				t.Fatalf("resolveCandidateGoProxy() error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveCandidateGoProxy() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderCandidateCleansFailuresAndRejectsTidyOrVerifyMutation(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*recordingGoRunner)
	}{
		{"tidy error", func(r *recordingGoRunner) { r.failCommand = "mod tidy" }},
		{"tidy changes README", func(r *recordingGoRunner) { r.tidyMutationPath = "README.md" }},
		{"tidy chmods README", func(r *recordingGoRunner) { r.tidyChmodPath = "README.md" }},
		{"tidy replaces NOTICE with same bytes", func(r *recordingGoRunner) { r.tidyReplacePath = "NOTICE" }},
		{"tidy adds path", func(r *recordingGoRunner) { r.tidyAddedPath = "unexpected.txt" }},
		{"tidy adds empty directory", func(r *recordingGoRunner) { r.tidyAddedDirectory = "unexpected-empty" }},
		{"verify error", func(r *recordingGoRunner) { r.failCommand = "mod verify" }},
		{"verify mutation", func(r *recordingGoRunner) { r.verifyMutationPath = "NOTICE" }},
		{"scan rejects private key in go.sum", func(r *recordingGoRunner) { r.tidyGoSum = []byte("-----" + "BEGIN DSA PRIVATE KEY" + "-----\n") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
			tt.configure(dependencies.runner)
			candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
			if err == nil || candidate != nil {
				t.Fatalf("render = %v, %v; want failure", candidate, err)
			}
			assertDirectoryEmpty(t, backend)
		})
	}
}

func TestRenderCandidateCleansInjectedDigestFailure(t *testing.T) {
	backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
	dependencies.beforeDigest = func() error { return fmt.Errorf("fixture digest failure") }
	candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
	if err == nil || candidate != nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("render = %v, %v; want digest failure", candidate, err)
	}
	assertDirectoryEmpty(t, backend)
}

func TestRenderCandidateCreatesPrivateRootBeforeWritingAndRejectsInjectedSymlink(t *testing.T) {
	backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
	escape := t.TempDir()
	dependencies.afterCandidateCreated = func(path string) error {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode().Perm() != 0o700 {
			return fmt.Errorf("initial candidate mode = %04o", info.Mode().Perm())
		}
		return os.Symlink(escape, filepath.Join(path, "service"))
	}

	candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
	if err == nil || candidate != nil {
		t.Fatalf("render = %v, %v; want injected symlink rejection", candidate, err)
	}
	assertDirectoryEmpty(t, backend)
	assertDirectoryEmpty(t, escape)
}

func TestRenderCandidateRejectsRootIdentityExchangeWithoutWritingEscapeOrLeavingCandidate(t *testing.T) {
	backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
	escape := t.TempDir()
	dependencies.afterCandidateCreated = func(path string) error {
		moved := filepath.Join(filepath.Dir(path), ".thingsbaseline-moved-fixture")
		if err := os.Rename(path, moved); err != nil {
			return err
		}
		return os.Symlink(escape, path)
	}

	candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
	if err == nil || candidate != nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("root exchange render = %v, %v; want identity rejection", candidate, err)
	}
	assertDirectoryEmpty(t, backend)
	assertDirectoryEmpty(t, escape)
}

func TestRenderCandidateFinalModesDoNotDependOnUmask(t *testing.T) {
	oldUmask := unix.Umask(0o077)
	defer unix.Umask(oldUmask)
	backend, lock, snapshot, dependencies := syntheticRenderFixture(t)
	initialModeChecked := false
	dependencies.afterCandidateCreated = func(path string) error {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		initialModeChecked = info.Mode().Perm() == 0o700
		return nil
	}
	candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies.renderDependencies)
	if err != nil {
		t.Fatalf("render with restrictive umask: %v", err)
	}
	defer candidate.Cleanup()
	if !initialModeChecked {
		t.Fatal("candidate initial mode was not 0700")
	}
	root := candidate.path
	if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if info.Mode().Perm() != 0o755 {
				return fmt.Errorf("directory %s mode=%04o", path, info.Mode().Perm())
			}
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		expectedMode := os.FileMode(0o644)
		for _, entry := range candidate.Report().Manifest {
			if entry.Path == filepath.ToSlash(relative) && entry.Mode == "100755" {
				expectedMode = 0o755
			}
		}
		if info.Mode().Perm() != expectedMode {
			return fmt.Errorf("file %s mode=%04o want=%04o", path, info.Mode().Perm(), expectedMode)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRenderRealLockedSourceTwiceWhenEnabled(t *testing.T) {
	repository := os.Getenv("THINGS_BASELINE_SOURCE_REPOSITORY")
	lockPath := os.Getenv("THINGS_BASELINE_LOCK")
	backend := os.Getenv("THINGS_BASELINE_BACKEND")
	if repository == "" || lockPath == "" || backend == "" {
		t.Skip("set THINGS_BASELINE_SOURCE_REPOSITORY, THINGS_BASELINE_LOCK, and THINGS_BASELINE_BACKEND")
	}
	lock, err := LoadLock(lockPath)
	if err != nil {
		t.Fatalf("LoadLock(%q): %v", lockPath, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	snapshot, err := ReadSource(ctx, repository, lock)
	if err != nil {
		t.Fatalf("ReadSource(): %v", err)
	}
	first, err := renderVerificationCandidate(ctx, backend, snapshot, lock)
	if err != nil {
		t.Fatalf("renderVerificationCandidate(first): %v", err)
	}
	defer first.Cleanup()
	second, err := renderVerificationCandidate(ctx, backend, snapshot, lock)
	if err != nil {
		t.Fatalf("renderVerificationCandidate(second): %v", err)
	}
	defer second.Cleanup()

	firstReport, secondReport := first.Report(), second.Report()
	if len(firstReport.Manifest) != 980 || firstReport.Scan.RegularFiles != 980 || firstReport.Scan.TrailingWhitespace != 97 {
		t.Fatalf("real report counts = manifest %d files %d trailing %d", len(firstReport.Manifest), firstReport.Scan.RegularFiles, firstReport.Scan.TrailingWhitespace)
	}
	if firstReport.Credentials != (CredentialProfileReport{Files: 9, Active: 35, Commented: 20, Paths: lock.Transforms.CredentialPaths}) {
		t.Fatalf("real credential profile = %+v", firstReport.Credentials)
	}
	if firstReport.Transform.NonYAMLCredentialValues != 5 {
		t.Fatalf("real non-YAML credential transform count = %d, want 5", firstReport.Transform.NonYAMLCredentialValues)
	}
	if firstReport.TreeDigest != secondReport.TreeDigest || !manifestEntriesEqual(firstReport.Manifest, secondReport.Manifest) {
		t.Fatalf("two real renders differ: %s vs %s", firstReport.TreeDigest, secondReport.TreeDigest)
	}
}

func TestRenderCandidateExecutesRetainedGoToolWhenNamedPathIsReplaced(t *testing.T) {
	backend, lock, snapshot, fixture := syntheticRenderFixture(t)
	toolRoot := t.TempDir()
	goPath := filepath.Join(toolRoot, "go")
	trustedScript := []byte(`#!/bin/sh
case "$1:$2" in
mod:tidy)
	printf 'fixture tidy sum\n' > go.sum
	;;
mod:verify)
	;;
*)
	exit 64
	;;
esac
`)
	if err := os.WriteFile(goPath, trustedScript, 0o755); err != nil {
		t.Fatal(err)
	}
	retained, err := os.Open(goPath)
	if err != nil {
		t.Fatal(err)
	}
	tool := &attestedGoTool{file: retained, goVersion: lockedGoVersion, buildPath: "cmd/go"}
	t.Cleanup(func() { _ = tool.Close() })
	if err := os.Rename(goPath, goPath+".attested"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goPath, []byte("#!/bin/sh\nexit 97\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unix.Fstat(int(retained.Fd()), &tool.state); err != nil {
		t.Fatal(err)
	}
	tool.sha256, err = hashRetainedFile(context.Background(), retained, tool.state.Size, maxGoToolBytes)
	if err != nil {
		t.Fatal(err)
	}

	dependencies := fixture.renderDependencies
	dependencies.runner = execGoRunner{}
	dependencies.goBinary = goPath
	dependencies.goTool = tool
	candidate, err := renderCandidateWithDependencies(context.Background(), backend, snapshot, &lock, dependencies)
	if err != nil {
		t.Fatalf("render with retained Go tool after named-path replacement: %v", err)
	}
	t.Cleanup(func() { _ = candidate.Cleanup() })
}

type recordingGoRunner struct {
	calls              []GoCommand
	failCommand        string
	tidyMutationPath   string
	tidyChmodPath      string
	tidyReplacePath    string
	tidyAddedPath      string
	tidyAddedDirectory string
	verifyMutationPath string
	tidyGoSum          []byte
}

func (runner *recordingGoRunner) Run(_ context.Context, command GoCommand) (CommandReport, error) {
	command.Args = append([]string(nil), command.Args...)
	command.Env = append([]string(nil), command.Env...)
	runner.calls = append(runner.calls, command)
	name := strings.Join(command.Args, " ")
	if runner.failCommand == name {
		return CommandReport{Name: strings.ReplaceAll(name, " ", "-"), ExitCode: 1}, fmt.Errorf("fixture command failure")
	}
	if name == "mod tidy" {
		goSum := runner.tidyGoSum
		if goSum == nil {
			goSum = []byte("fixture tidy sum\n")
		}
		if err := os.WriteFile(filepath.Join(command.Dir, "go.sum"), goSum, 0o600); err != nil {
			return CommandReport{}, err
		}
		if runner.tidyMutationPath != "" {
			mustMutateCommandFile(command.Dir, runner.tidyMutationPath)
		}
		if runner.tidyChmodPath != "" {
			if err := os.Chmod(filepath.Join(command.Dir, runner.tidyChmodPath), 0o400); err != nil {
				return CommandReport{}, err
			}
		}
		if runner.tidyReplacePath != "" {
			path := filepath.Join(command.Dir, runner.tidyReplacePath)
			data, err := os.ReadFile(path)
			if err != nil {
				return CommandReport{}, err
			}
			temporary := path + ".replacement"
			if err := os.WriteFile(temporary, data, 0o600); err != nil {
				return CommandReport{}, err
			}
			if err := os.Rename(temporary, path); err != nil {
				return CommandReport{}, err
			}
		}
		if runner.tidyAddedPath != "" {
			if err := os.WriteFile(filepath.Join(command.Dir, runner.tidyAddedPath), []byte("added"), 0o600); err != nil {
				return CommandReport{}, err
			}
		}
		if runner.tidyAddedDirectory != "" {
			if err := os.Mkdir(filepath.Join(command.Dir, runner.tidyAddedDirectory), 0o700); err != nil {
				return CommandReport{}, err
			}
		}
		return CommandReport{Name: "go-mod-tidy", ExitCode: 0}, nil
	}
	if name == "mod verify" {
		if runner.verifyMutationPath != "" {
			mustMutateCommandFile(command.Dir, runner.verifyMutationPath)
		}
		return CommandReport{Name: "go-mod-verify", ExitCode: 0}, nil
	}
	return CommandReport{}, fmt.Errorf("unexpected go command %q", name)
}

type syntheticRenderDependencies struct {
	renderDependencies
	runner                      *recordingGoRunner
	backendAbsolutePathSentinel string
}

func syntheticRenderFixture(t *testing.T) (string, Lock, SourceSnapshot, syntheticRenderDependencies) {
	t.Helper()
	t.Setenv("MING_GOPROXY", "")
	backend := t.TempDir()
	files, lock := fullTransformFixture(t)
	// The real lock excludes the upstream root README. Preserve its synthetic
	// module-count fixture under a non-template path.
	reference := files["README.md"]
	delete(files, "README.md")
	reference.Path = "source-reference.md"
	files[reference.Path] = reference
	licenseData := []byte("fixture AGPL license\n")
	files["LICENSE"] = SourceFile{Path: "LICENSE", Mode: "100644", Data: licenseData}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	snapshot := SourceSnapshot{Commit: lock.Source.Commit, Tree: lock.Source.Tree}
	for _, path := range paths {
		file := files[path]
		file.Data = append([]byte(nil), file.Data...)
		snapshot.Files = append(snapshot.Files, file)
		snapshot.SelectedBlobBytes += int64(len(file.Data))
	}
	lock.Source.TrackedFiles = len(snapshot.Files)
	lock.Source.SelectedFiles = len(snapshot.Files)
	lock.Source.SelectedBlobBytes = snapshot.SelectedBlobBytes
	lock.Security.MaxFileBytes = 1 << 20
	runner := &recordingGoRunner{}
	licenseSHA := sha256.Sum256(licenseData)
	dependencies := syntheticRenderDependencies{
		runner:                      runner,
		backendAbsolutePathSentinel: backend,
		renderDependencies: renderDependencies{
			runner:             runner,
			goBinary:           "/trusted/go",
			goVersion:          "go1.25.4",
			goCache:            testSharedGoCachePaths(t),
			expectedLicenseSHA: fmt.Sprintf("%x", licenseSHA),
		},
	}
	return backend, lock, snapshot, dependencies
}

func renderedFilesEqual(left, right map[string]RenderedFile) bool {
	if len(left) != len(right) {
		return false
	}
	for path, leftFile := range left {
		rightFile, ok := right[path]
		if !ok || leftFile.Mode != rightFile.Mode || !bytes.Equal(leftFile.Data, rightFile.Data) {
			return false
		}
	}
	return true
}

func sortedRenderedPaths(files map[string]RenderedFile) []string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func assertDirectoryEmpty(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", directory, err)
	}
	if len(entries) != 0 {
		t.Fatalf("directory %q is not empty: %v", directory, entries)
	}
}

func assertRenderPrivateDirectoriesRemoved(t *testing.T, environment map[string]string) {
	t.Helper()
	home := environment["HOME"]
	gotmp := environment["GOTMPDIR"]
	if home == "" || gotmp == "" {
		t.Fatalf("render environment private directories = HOME %q, GOTMPDIR %q", home, gotmp)
	}
	homeRoot := filepath.Dir(home)
	gotmpRoot := filepath.Dir(gotmp)
	if homeRoot == "" || gotmpRoot == "" || homeRoot != gotmpRoot {
		t.Fatalf("render private roots = %q and %q, want one nonempty root", homeRoot, gotmpRoot)
	}
	for _, key := range []string{"HOME", "GOTMPDIR"} {
		path := environment[key]
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("private %s directory %q still exists: %v", key, path, err)
		}
	}
	if _, err := os.Stat(homeRoot); !os.IsNotExist(err) {
		t.Fatalf("private render root %q still exists: %v", homeRoot, err)
	}
}

func assertSharedGoCacheDirectories(t *testing.T, cache sharedGoCachePaths) {
	t.Helper()
	for _, path := range []string{cache.Build, cache.Modules} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("shared Go cache directory %q is unavailable: %v", path, err)
		}
	}
}

func mustWriteFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

func mustMutateCommandFile(root, path string) {
	filename := filepath.Join(root, filepath.FromSlash(path))
	data, err := os.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filename, append(data, []byte("mutation\n")...), 0o600); err != nil {
		panic(err)
	}
}

func environmentMap(environment []string) map[string]string {
	result := make(map[string]string, len(environment))
	for _, item := range environment {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}

func manifestEntriesEqual(left, right []ManifestEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func templateGoldens(lock Lock, snapshot SourceSnapshot, report TransformReport) map[string]string {
	return map[string]string{
		"README.md": fmt.Sprintf(`# Ming IoT Things

这是 Ming IoT 的 Things 服务 L0 兼容迁移基线，来源固定为 %s 的 commit %s。

## 服务入口

- API：`+"`./service/apisvr`"+`
- 设备网关：`+"`./service/dgsvr`"+`
- 设备管理：`+"`./service/dmsvr`"+`

## 本地验证与构建

先进入 `+"`backend/things`"+`，再执行：

`+"```bash"+`
GOWORK=off go test ./...
GOWORK=off go vet ./...
make build
`+"```"+`

本目录只证明 L0 可编译兼容基线，不代表已经完成 Kafka、模块级物模型、Reducer 或 Scene Runtime 改造。禁止未经后续 Loop 门禁直接用于生产部署。
`, "`"+lock.Source.Repository+"`", "`"+snapshot.Commit+"`"),
		"NOTICE": fmt.Sprintf(`Ming IoT Things migration baseline

Source repository: %s
Source ref: %s
Source commit: %s
Source tree: %s
Source module: %s
Target module: %s

The migrated source remains governed by AGPL-3.0. See LICENSE.
`, lock.Source.Repository, lock.Source.Ref, snapshot.Commit, snapshot.Tree, lock.Source.Module, lock.Target.Module),
		"PROVENANCE.yaml": fmt.Sprintf(`schemaVersion: 1
loopId: L0
source:
  repository: %s
  ref: %s
  commit: %s
  tree: %s
  module: %s
target:
  module: %s
  saasModule: %s
dependencies:
  shareModule: %s
  shareVersion: %s
license: AGPL-3.0
files:
  source: %d
  target: %d
transforms:
  corePrefixCount: %d
  thingsPrefixCount: %d
  credentialLineCount: %d
  privateKeyBlockCount: %d
  vetFixCount: %d
  schemaFixtureIdentifierCount: %d
  nonYamlCredentialValueCount: %d
`, lock.Source.Repository, lock.Source.Ref, snapshot.Commit, snapshot.Tree, lock.Source.Module, lock.Target.Module, lock.Target.SaaSModule, lock.Target.ShareModule, lock.Target.ShareVersion, lock.Source.SelectedFiles, lock.Source.SelectedFiles+4, report.CorePrefixes, report.ThingsPrefixes, report.CredentialLines, report.PrivateKeyBlocks, report.VetFixes, report.SchemaFixtureIdentifiers, report.NonYAMLCredentialValues),
		"Makefile": `BACKEND_DIR := $(abspath ..)
BIN_DIR := $(BACKEND_DIR)/bin
GO ?= go
GO_BUILD := GOWORK=off $(GO) build

.PHONY: build build.api build.dg build.dm clean

build: build.api build.dg build.dm

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build.api: | $(BIN_DIR)
	umask 022 && $(GO_BUILD) -o $(BIN_DIR)/thingsvr ./service/apisvr

build.dg: | $(BIN_DIR)
	umask 022 && $(GO_BUILD) -o $(BIN_DIR)/dgsvr ./service/dgsvr

build.dm: | $(BIN_DIR)
	umask 022 && $(GO_BUILD) -o $(BIN_DIR)/dmsvr ./service/dmsvr

clean:
	rm -f $(BIN_DIR)/thingsvr $(BIN_DIR)/dgsvr $(BIN_DIR)/dmsvr
`,
	}
}
