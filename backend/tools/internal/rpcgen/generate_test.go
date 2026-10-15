package rpcgen

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ming-iot/backend/tools/internal/clientcontract"
)

func TestGenerateFailureLeavesLiveTreeUnchanged(t *testing.T) {
	root := t.TempDir()
	sentinel := filepath.Join(root, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0644); err != nil {
		t.Fatal(err)
	}
	before := fileDigest(t, sentinel)
	err := Generate(context.Background(), GenerateOptions{
		Manifest:   &Manifest{Version: 2},
		ModuleRoot: root,
	})
	if err == nil || !strings.Contains(err.Error(), "stage=manifest:") {
		t.Fatalf("Generate error = %v", err)
	}
	if after := fileDigest(t, sentinel); after != before {
		t.Fatalf("live tree changed: before=%s after=%s", before, after)
	}
}

func TestBuildAllowlistExcludesDmdirectLogicAndEtc(t *testing.T) {
	m := &Manifest{
		RuntimeDir: "service/dmsvr",
		PbDir:      "service/dmsvr/pb/dm",
		Services: []ServiceEntry{{
			PackageDir: "device", ClientFile: "device.go", ServerFile: "deviceServer.go",
		}},
	}
	allow := buildAllowlist(m)
	for _, forbidden := range []string{
		"service/dmsvr/dmdirect/",
		"service/dmsvr/internal/logic/",
		"service/dmsvr/etc/",
	} {
		if pathAllowedForTest(allow, forbidden) {
			t.Fatalf("allowed %s", forbidden)
		}
	}
}

func TestRejectForbiddenImportsReportsRelativePath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "service", "bad.go")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	legacyImport := legacyShareImportPath() + "/def"
	if err := os.WriteFile(path, []byte("package service\nimport _ \""+legacyImport+"\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := RejectForbiddenImports(root, []string{legacyShareImportPath()})
	if err == nil || !strings.Contains(err.Error(), "service/bad.go") {
		t.Fatalf("error = %v", err)
	}
}

func TestVerifyProtectedDigestsReportsChangedPath(t *testing.T) {
	err := VerifyProtectedDigests(
		map[string]string{"service/dmsvr/etc/dm.yaml": "before"},
		map[string]string{"service/dmsvr/etc/dm.yaml": "after"},
	)
	if err == nil || !strings.Contains(err.Error(), "service/dmsvr/etc/dm.yaml") {
		t.Fatalf("error = %v", err)
	}
}

func TestGenerateAliasesRebuildsFromDesiredClientAPI(t *testing.T) {
	root := t.TempDir()
	m := &Manifest{
		ModulePath: "example.com/things", RuntimeDir: "service/dmsvr",
		GoPackage: "pb/dm", PBSelector: "dm",
		Services: []ServiceEntry{{PackageDir: "device"}},
	}
	clientDir := filepath.Join(m.ClientRoot(root), "device")
	if err := os.MkdirAll(clientDir, 0755); err != nil {
		t.Fatal(err)
	}
	mainFile := "package device\nimport dm \"example.com/things/service/dmsvr/pb/dm\"\ntype Existing = dm.Existing\n"
	if err := os.WriteFile(filepath.Join(clientDir, "device.go"), []byte(mainFile), 0644); err != nil {
		t.Fatal(err)
	}
	staleFile := "package device\nimport dm \"example.com/things/service/dmsvr/pb/dm\"\ntype Stale = dm.Stale\n"
	if err := os.WriteFile(filepath.Join(clientDir, "compat_aliases.go"), []byte(staleFile), 0644); err != nil {
		t.Fatal(err)
	}
	desired := clientcontract.Document{Aliases: []clientcontract.Alias{
		{Package: "device", Name: "Existing", Target: "dm.Existing"},
		{Package: "device", Name: "Needed", Target: "dm.Needed"},
	}}
	if err := generateAliasesForCandidate(m, root, desired); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(clientDir, "compat_aliases.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "Stale") || strings.Contains(text, "Existing =") || !strings.Contains(text, "Needed = dm.Needed") {
		t.Fatalf("compat_aliases.go =\n%s", text)
	}
}

func TestResetGeneratedPBRemovesStaleAggregateGrpcFile(t *testing.T) {
	candidate := t.TempDir()
	root := filepath.Join(candidate, "pb", "dm")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, "dm_grpc.pb.go")
	if err := os.WriteFile(stale, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := resetGeneratedPB(candidate, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale PB remains: %v", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("PB root not recreated: info=%v err=%v", info, err)
	}
}

func TestPrepareCandidateModfilePinsLocalSiblingModules(t *testing.T) {
	backend := t.TempDir()
	candidate := filepath.Join(backend, ".saastxn", "run", "candidate")
	if err := os.MkdirAll(candidate, 0755); err != nil {
		t.Fatal(err)
	}
	writeModule := func(dir, module string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+module+"\n\ngo 1.24.4\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeModule(filepath.Join(backend, "saas"), "gitee.com/ming-iot/saas")
	writeModule(filepath.Join(backend, "share"), "gitee.com/ming-iot/share")
	if err := os.WriteFile(filepath.Join(candidate, "go.mod"), []byte(`module gitee.com/ming-iot/things

go 1.24.4

replace gitee.com/ming-iot/saas => ../saas
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidate, "go.sum"), []byte("sum\n"), 0644); err != nil {
		t.Fatal(err)
	}
	modfile, cleanup, err := prepareCandidateModfile(context.Background(), candidate, backend)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	data, err := os.ReadFile(modfile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{
		"gitee.com/ming-iot/saas => " + filepath.Join(backend, "saas"),
		"gitee.com/ming-iot/share => " + filepath.Join(backend, "share"),
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("modfile missing %q:\n%s", expected, text)
		}
	}
	if _, err := os.Stat(strings.TrimSuffix(modfile, ".mod") + ".sum"); err != nil {
		t.Fatalf("candidate sum not copied: %v", err)
	}
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func pathAllowedForTest(allow []string, candidate string) bool {
	for _, item := range allow {
		if item == candidate || strings.HasPrefix(candidate, strings.TrimSuffix(item, "/")+"/") ||
			strings.HasPrefix(item, strings.TrimSuffix(candidate, "/")+"/") {
			return true
		}
	}
	return false
}

func TestGenerateUsesModuleRootRuntimeAndSelector(t *testing.T) {
	m := &Manifest{
		ModulePath: "gitee.com/ming-iot/things",
		RuntimeDir: "service/dmsvr",
		PbDir:      "service/dmsvr/pb/dm",
		GoPackage:  "pb/dm",
		PBSelector: "model",
		Services: []ServiceEntry{
			{PackageDir: "device", ClientFile: "device.go", ServerFile: "deviceServer.go"},
		},
	}
	root := filepath.Join(t.TempDir(), "things")
	if got, want := m.RuntimeRoot(root), filepath.Join(root, "service", "dmsvr"); got != want {
		t.Fatalf("RuntimeRoot() = %q, want %q", got, want)
	}
	if got, want := m.ClientRoot(root), filepath.Join(root, "service", "dmsvr", "client"); got != want {
		t.Fatalf("ClientRoot() = %q, want %q", got, want)
	}
	if got, want := m.PBImportPath(), "gitee.com/ming-iot/things/service/dmsvr/pb/dm"; got != want {
		t.Fatalf("PBImportPath() = %q, want %q", got, want)
	}

	allow := strings.Join(buildAllowlist(m), "\n")
	for _, want := range []string{
		"service/dmsvr/pb/dm/",
		"service/dmsvr/client/device/device.go",
		"service/dmsvr/client/device/compat_aliases.go",
		"service/dmsvr/internal/server/device/deviceServer.go",
	} {
		if !strings.Contains(allow, want) {
			t.Fatalf("allowlist %q missing %q", allow, want)
		}
	}

	aliasDir := filepath.Join(t.TempDir(), "device")
	if err := writeAliasFile(aliasDir, "device", m.PBImportPath(), m.PBSelector, []string{"B", "A"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(aliasDir, "compat_aliases.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `import model "gitee.com/ming-iot/things/service/dmsvr/pb/dm"`) ||
		!strings.Contains(text, "A = model.A") || !strings.Contains(text, "B = model.B") {
		t.Fatalf("alias file =\n%s", text)
	}
	if strings.Index(text, "A = model.A") > strings.Index(text, "B = model.B") {
		t.Fatalf("aliases are not sorted:\n%s", text)
	}

	if got, want := stagingGoMod(m), "module gitee.com/ming-iot/things\n\ngo 1.24\n"; got != want {
		t.Fatalf("stagingGoMod() = %q, want %q", got, want)
	}
}

func TestSaaSV1DerivedPathsRemainCompatible(t *testing.T) {
	m := &Manifest{
		ModulePath: "gitee.com/ming-iot/saas",
		RuntimeDir: "service/syssvr",
		GoPackage:  "pb/sys",
		PBSelector: "sys",
	}
	root := "/workspace/backend/saas"
	if got, want := m.RuntimeRoot(root), "/workspace/backend/saas/service/syssvr"; got != want {
		t.Fatalf("RuntimeRoot() = %q, want %q", got, want)
	}
	if got, want := m.PBImportPath(), "gitee.com/ming-iot/saas/service/syssvr/pb/sys"; got != want {
		t.Fatalf("PBImportPath() = %q, want %q", got, want)
	}
}
