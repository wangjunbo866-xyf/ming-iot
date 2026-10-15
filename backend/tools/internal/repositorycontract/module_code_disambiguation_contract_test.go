package repositorycontract

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestModuleCodeDisambiguation(t *testing.T) {
	repoRoot := findRepositoryRoot(t)

	t.Run("OTA", func(t *testing.T) {
		deviceProto := moduleCodeReadFile(t, repoRoot, "backend/things/service/dmsvr/proto/deviceManage.proto")
		moduleCodeRequireRegexpCount(t, "deviceManage.proto firmwareModuleCode field 4", deviceProto, `(?m)^\s*string\s+firmwareModuleCode\s*=\s*4\s*;`, 3)

		otaProto := moduleCodeReadFile(t, repoRoot, "backend/things/service/dmsvr/proto/otaManage.proto")
		moduleCodeRequireRegexpCount(t, "otaManage.proto firmwareModuleCode field 16", otaProto, `(?m)^\s*string\s+firmwareModuleCode\s*=\s*16\s*;`, 1)
		moduleCodeRequireRegexpCount(t, "otaManage.proto firmwareModuleCode field 14", otaProto, `(?m)^\s*string\s+firmwareModuleCode\s*=\s*14\s*;`, 1)

		moduleVersionAPI := moduleCodeReadFile(t, repoRoot, "backend/things/service/apisvr/http/things/device/moduleVersion.api")
		moduleCodeRequireRegexpCount(t, "moduleVersion.api string firmwareModuleCode", moduleVersionAPI, `(?m)^\s*FirmwareModuleCode\s+string\s+`+"`"+`json:\"firmwareModuleCode,optional\"`+"`"+``, 3)

		firmwareAPI := moduleCodeReadFile(t, repoRoot, "backend/things/service/apisvr/http/things/ota/firmware.api")
		moduleCodeRequireRegexpCount(t, "firmware.api firmwareModuleCode", firmwareAPI, `(?m)^\s*FirmwareModuleCode\s+string\s+`+"`"+`json:\"firmwareModuleCode(?:,optional)?\"`+"`"+``, 2)

		deviceModel := moduleCodeReadFile(t, repoRoot, "backend/things/service/dmsvr/internal/repo/relationDB/model.go")
		moduleCodeRequireContains(t, "device module version PostgreSQL column", deviceModel, "FirmwareModuleCode string `gorm:\"column:firmware_module_code;")

		otaModel := moduleCodeReadFile(t, repoRoot, "backend/things/service/dmsvr/internal/repo/relationDB/modelOta.go")
		moduleCodeRequireContains(t, "OTA firmware PostgreSQL column", otaModel, "FirmwareModuleCode string")
		moduleCodeRequireContains(t, "OTA firmware PostgreSQL column", otaModel, "column:firmware_module_code;")

		otaDomain := moduleCodeReadFile(t, repoRoot, "backend/things/share/domain/deviceMsg/msgOta/ota.go")
		moduleCodeRequireContains(t, "OTA default constant", otaDomain, `FirmwareModuleCodeDefault = "default"`)

		moduleCodeRejectLegacyIdentifiers(t, repoRoot, []string{
			"backend/things/service/dmsvr/proto/deviceManage.proto",
			"backend/things/service/dmsvr/proto/otaManage.proto",
			"backend/things/service/apisvr/http/things/device/moduleVersion.api",
			"backend/things/service/apisvr/http/things/ota/firmware.api",
			"backend/things/service/dmsvr/internal/repo/relationDB/deviceModuleVersion.go",
			"backend/things/service/dmsvr/internal/repo/relationDB/modelOta.go",
			"backend/things/share/domain/deviceMsg/msgOta/ota.go",
		}, `\bModuleCode\b|\bmoduleCode\b|\bmodule_code\b`)
	})

	t.Run("Model", func(t *testing.T) {
		productProto := moduleCodeReadFile(t, repoRoot, "backend/things/service/dmsvr/proto/productManage.proto")
		moduleCodeRequireRegexpCount(t, "productManage.proto modelModuleCode field 1", productProto, `(?m)^\s*string\s+modelModuleCode\s*=\s*1\s*;`, 1)
		moduleCodeRequireRegexpCount(t, "productManage.proto modelModuleCode field 2", productProto, `(?m)^\s*string\s+modelModuleCode\s*=\s*2\s*;`, 2)
		moduleCodeRequireRegexpCount(t, "productManage.proto modelModuleCode field 3", productProto, `(?m)^\s*string\s+modelModuleCode\s*=\s*3\s*;`, 1)

		modelModuleAPI := moduleCodeReadFile(t, repoRoot, "backend/things/service/apisvr/http/things/product/modelModule.api")
		moduleCodeRequireRegexpCount(t, "modelModule.api modelModuleCode", modelModuleAPI, `(?m)^\s*ModelModuleCode\s+string\s+`+"`"+`json:\"modelModuleCode(?:,optional)?\"`+"`"+``, 4)

		modelRelease := moduleCodeReadFile(t, repoRoot, "backend/things/service/dmsvr/internal/repo/relationDB/modelReleaseModel.go")
		moduleCodeRequireContains(t, "model module PostgreSQL column", modelRelease, "ModelModuleCode string")
		moduleCodeRequireContains(t, "model module PostgreSQL column", modelRelease, "column:model_module_code;")

		moduleCodeRejectLegacyIdentifiers(t, repoRoot, []string{
			"backend/things/service/dmsvr/proto/productManage.proto",
			"backend/things/service/apisvr/http/things/product/modelModule.api",
			"backend/things/service/dmsvr/internal/repo/relationDB/modelReleaseModel.go",
			"backend/things/service/dmsvr/internal/repo/relationDB/modelModule.go",
		}, `\bModuleCode\b|\bmoduleCode\b|\bmodule_code\b`)
	})

	t.Run("CrossDomain", func(t *testing.T) {
		moduleCodeRejectLegacyIdentifiers(t, repoRoot, []string{
			"backend/things/service/dmsvr/internal/repo/relationDB/model.go",
		}, `\bModuleCode\b|\bmoduleCode\b|\bmodule_code\b`)

		for _, path := range []string{
			"backend/things/service/dmsvr/proto/deviceManage.proto",
			"backend/things/service/dmsvr/proto/otaManage.proto",
			"backend/things/service/apisvr/http/things/device/moduleVersion.api",
			"backend/things/service/apisvr/http/things/ota/firmware.api",
			"backend/things/service/dmsvr/internal/repo/relationDB/deviceModuleVersion.go",
			"backend/things/service/dmsvr/internal/repo/relationDB/modelOta.go",
		} {
			moduleCodeRejectLegacyIdentifiers(t, repoRoot, []string{path}, `\bModelModuleID\b|\bmodelModuleID\b|\bmodel_module_id\b`)
		}
	})
}

func moduleCodeReadFile(t *testing.T, repoRoot, path string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repoRoot, path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func moduleCodeRequireRegexpCount(t *testing.T, label, content, pattern string, want int) {
	t.Helper()
	got := len(regexp.MustCompile(pattern).FindAllStringIndex(content, -1))
	if got != want {
		t.Errorf("%s count = %d, want %d", label, got, want)
	}
}

func moduleCodeRequireContains(t *testing.T, label, content, want string) {
	t.Helper()
	if !strings.Contains(content, want) {
		t.Errorf("%s missing %q", label, want)
	}
}

func moduleCodeRejectLegacyIdentifiers(t *testing.T, repoRoot string, paths []string, pattern string) {
	t.Helper()
	legacy := regexp.MustCompile(pattern)
	for _, path := range paths {
		root := filepath.Join(repoRoot, path)
		err := filepath.WalkDir(root, func(entryPath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !moduleCodeContractSourceFile(entryPath) {
				return nil
			}
			content, err := os.ReadFile(entryPath)
			if err != nil {
				return err
			}
			if legacy.Match(content) {
				relative, relErr := filepath.Rel(repoRoot, entryPath)
				if relErr != nil {
					relative = entryPath
				}
				t.Errorf("legacy cross-domain identifier remains in %s", relative)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("inspect %s: %v", path, err)
		}
	}
}

func moduleCodeContractSourceFile(path string) bool {
	switch filepath.Ext(path) {
	case ".api", ".go", ".proto":
		return true
	default:
		return false
	}
}
