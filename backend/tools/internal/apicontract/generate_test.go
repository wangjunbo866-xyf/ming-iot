package apicontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	goparser "github.com/zeromicro/go-zero/tools/goctl/api/parser"
)

func TestBackendsSupportsSingleAPIService(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "service", "apisvr", "http"), 0o755); err != nil {
		t.Fatal(err)
	}
	backends := Backends(root)
	if len(backends) != 1 {
		t.Fatalf("Backends() count = %d, want 1", len(backends))
	}
	if _, ok := backends["apisvr"]; !ok {
		t.Fatal("Backends() omitted existing apisvr")
	}
	if _, ok := backends["datasvr"]; ok {
		t.Fatal("Backends() included absent datasvr")
	}
}

func TestNormalizeStagingMetadataMakesLegacyRoutesGeneratable(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "legacy.api")
	content := `syntax = "v1"

info(title: "legacy")

type Req {}

@server(
    group: legacy
    prefix: /legacy
)
service api {
    @doc "旧接口"
    @handler access
    post /access (Req) returns ()

    @handler read
    post /read (Req) returns ()
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := normalizeStagingMetadata(root); err != nil {
		t.Fatal(err)
	}
	spec, err := goparser.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("normalized staging source must pass fixed goctl validation: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, `summary: "旧接口"`) {
		t.Fatalf("shorthand summary was not preserved:\n%s", got)
	}
	if strings.Count(got, "recordLogMode:") != 2 {
		t.Fatalf("each legacy route must receive one staging recordLogMode:\n%s", got)
	}
}

func TestStagingGoModUsesSelectedRepositoryModule(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module gitee.com/ming-iot/things\n\ngo 1.24.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := stagingGoMod(root)
	if err != nil {
		t.Fatal(err)
	}
	want := "module gitee.com/ming-iot/things\n\ngo 1.24\n"
	if got != want {
		t.Fatalf("stagingGoMod() = %q, want %q", got, want)
	}
}

func TestGenerateAllowlistCanAtomicallyUpdateGolden(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "service", "apisvr", "http"), 0o755); err != nil {
		t.Fatal(err)
	}
	allow := generateAllowlist(root, true)
	want := map[string]bool{
		"service/apisvr/internal/types/types.go":                   true,
		"service/apisvr/internal/handler/routes.go":                true,
		"service/apisvr/http/testdata":                             true,
		"service/apisvr/http/testdata/authorization.golden.json":   true,
		"service/apisvr/http/testdata/definitions.golden.json":     true,
		"service/apisvr/http/testdata/detached-routes.golden.json": true,
		"service/apisvr/http/testdata/reachable.golden.json":       true,
		"service/apisvr/http/testdata/registered.golden.json":      true,
		"service/apisvr/http/testdata/types.golden.json":           true,
	}
	for _, path := range allow {
		delete(want, path)
	}
	if len(want) != 0 {
		t.Fatalf("generateAllowlist() omitted paths: %v", want)
	}
}
