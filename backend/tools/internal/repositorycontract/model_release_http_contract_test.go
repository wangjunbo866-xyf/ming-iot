package repositorycontract

import (
	"path/filepath"
	"strings"
	"testing"

	"ming-iot/backend/tools/internal/apicontract"
)

func TestModelReleaseHTTPContractDeclaresExplicitResources(t *testing.T) {
	repoRoot := localShareRepositoryRoot(t)
	productAPI := string(localShareReadFile(t, repoRoot, "backend/things/service/apisvr/http/things/product.api"))
	for _, imported := range []string{`import "product/modelModule.api"`, `import "product/modelRelease.api"`} {
		if !strings.Contains(productAPI, imported) {
			t.Errorf("product.api is missing %s", imported)
		}
	}

	moduleAPI := string(localShareReadFile(t, repoRoot, "backend/things/service/apisvr/http/things/product/modelModule.api"))
	releaseAPI := string(localShareReadFile(t, repoRoot, "backend/things/service/apisvr/http/things/product/modelRelease.api"))
	for _, forbidden := range []string{"ModuleKind", "OwnerTenantCode", "ModuleSchemaSnapshot"} {
		if strings.Contains(moduleAPI+releaseAPI, forbidden) {
			t.Errorf("HTTP contract contains forbidden concept %s", forbidden)
		}
	}
	for _, route := range []string{
		"post /create", "post /update", "post /delete", "post /index",
		"post /bind", "post /unbind", "post /binding-index",
		"post /property/create", "post /property/update", "post /property/delete", "post /property/index",
		"post /publish", "post /activate", "post /read", "post /index", "post /tsl-read",
	} {
		if !strings.Contains(moduleAPI+releaseAPI, route) {
			t.Errorf("HTTP contract is missing route %q", route)
		}
	}
	if strings.Contains(releaseAPI, `releaseID,optional`) {
		t.Fatal("releaseID must be explicit for exact release reads and activation")
	}
}

func TestModelReleaseHTTPRoutesHaveCompleteMetadata(t *testing.T) {
	repoRoot := localShareRepositoryRoot(t)
	thingsRoot := filepath.Join(repoRoot, "backend/things")
	snaps, err := apicontract.LoadBackend(thingsRoot)
	if err != nil {
		t.Fatal(err)
	}
	snap, ok := snaps["apisvr"]
	if !ok {
		t.Fatal("Things apisvr snapshot is missing")
	}
	wantFiles := map[string]bool{
		"service/apisvr/http/things/product/modelModule.api":  true,
		"service/apisvr/http/things/product/modelRelease.api": true,
	}
	for _, problem := range apicontract.LintMetadata(thingsRoot, "apisvr", snap) {
		if wantFiles[problem.File] {
			t.Errorf("new HTTP metadata: %s", problem.String())
		}
	}
}
