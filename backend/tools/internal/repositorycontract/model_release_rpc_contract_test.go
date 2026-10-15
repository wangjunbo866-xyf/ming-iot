package repositorycontract

import (
	"strings"
	"testing"
)

func TestModelReleaseRPCContractDeclaresExplicitResources(t *testing.T) {
	repoRoot := localShareRepositoryRoot(t)
	content := string(localShareReadFile(t, repoRoot, "backend/things/service/dmsvr/proto/productManage.proto"))
	wantRPCs := []string{
		"modelModuleCreate", "modelModuleUpdate", "modelModuleDelete", "modelModuleIndex",
		"productModelModuleBind", "productModelModuleUnbind", "productModelModuleIndex",
		"modelModulePropertyCreate", "modelModulePropertyUpdate", "modelModulePropertyDelete", "modelModulePropertyIndex",
		"productModelReleasePublish", "productModelReleaseActivate", "productModelReleaseRead", "productModelReleaseIndex", "productModelReleaseTslRead",
	}
	for _, name := range wantRPCs {
		if !strings.Contains(content, "rpc "+name+"(") {
			t.Errorf("productManage.proto is missing rpc %s", name)
		}
	}
	for _, forbidden := range []string{"ModuleKind", "OwnerTenantCode", "ModuleSchemaSnapshot"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("productManage.proto contains forbidden concept %s", forbidden)
		}
	}
}

func TestModelReleaseRPCRequestsUseExplicitResourceIDs(t *testing.T) {
	repoRoot := localShareRepositoryRoot(t)
	content := string(localShareReadFile(t, repoRoot, "backend/things/service/dmsvr/proto/productManage.proto"))
	for _, field := range []string{"string productID", "int64 modelModuleID", "int64 releaseID"} {
		if !strings.Contains(content, field) {
			t.Errorf("productManage.proto is missing explicit request field %q", field)
		}
	}
}
