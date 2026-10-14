package rpcgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRegistry(t *testing.T) {
	dir := t.TempDir()
	// 模拟 sysdirect.go 的 registerServers 函数
	content := `package sysdirect

import "gitee.com/ming-iot/saas/service/syssvr/pb/sys"

func registerServers() {
	sys.RegisterAccessManageServer(nil, nil)
	sys.RegisterUserManageServer(nil, nil)
}`
	path := filepath.Join(dir, "sysdirect.go")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("all match", func(t *testing.T) {
		m := &Manifest{ModulePath: "gitee.com/ming-iot/saas", PbDir: "service/syssvr/pb/sys", Registry: RegistrySpec{Selector: "sys"}, Services: []ServiceEntry{
			{Service: "AccessManage"}, {Service: "UserManage"},
		}}
		result, err := CheckRegistry(path, m)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.MissingRequired) != 0 || len(result.Extra) != 0 || len(result.PresentPreserveAbsent) != 0 {
			t.Fatalf("result=%+v", result)
		}
	})
	t.Run("missing service", func(t *testing.T) {
		m := &Manifest{ModulePath: "gitee.com/ming-iot/saas", PbDir: "service/syssvr/pb/sys", Registry: RegistrySpec{Selector: "sys"}, Services: []ServiceEntry{
			{Service: "AccessManage"}, {Service: "DataManage"},
		}}
		result, err := CheckRegistry(path, m)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.MissingRequired) != 1 || result.MissingRequired[0] != "DataManage" {
			t.Fatalf("missing=%v, want [DataManage]", result.MissingRequired)
		}
		if len(result.Extra) != 1 || result.Extra[0] != "UserManage" {
			t.Fatalf("extra=%v, want [UserManage]", result.Extra)
		}
	})
}

func TestCheckRegistryPreservesAbsentService(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dmdirect.go")
	content := `package dmdirect
import dm "example.com/things/service/dmsvr/pb/dm"
func register() { dm.RegisterDeviceManageServer(nil, nil) }
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{
		ModulePath: "example.com/things", PbDir: "service/dmsvr/pb/dm",
		Registry: RegistrySpec{Selector: "dm"},
		Services: []ServiceEntry{
			{Service: "DeviceManage", Registration: RegistrationRequired},
			{Service: "SchemaManage", Registration: RegistrationPreserveAbsent},
		},
	}
	result, err := CheckRegistry(path, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.MissingRequired) != 0 || len(result.PresentPreserveAbsent) != 0 || len(result.Extra) != 0 {
		t.Fatalf("result = %+v", result)
	}

	content = `package dmdirect
import dm "example.com/things/service/dmsvr/pb/dm"
func register() {
	dm.RegisterDeviceManageServer(nil, nil)
	dm.RegisterSchemaManageServer(nil, nil)
}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	result, err = CheckRegistry(path, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PresentPreserveAbsent) != 1 || result.PresentPreserveAbsent[0] != "SchemaManage" {
		t.Fatalf("present preserve_absent = %v", result.PresentPreserveAbsent)
	}
}

func TestCheckRegistryUsesManifestSelector(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.go")
	content := `package registry
import dm "example.com/things/service/dmsvr/pb/dm"
func register() {
	dm.RegisterDeviceManageServer(nil, nil)
	sys.RegisterIgnoredServer(nil, nil)
}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{ModulePath: "example.com/things", PbDir: "service/dmsvr/pb/dm", Registry: RegistrySpec{Selector: "dm"}, Services: []ServiceEntry{{Service: "DeviceManage", Registration: RegistrationRequired}}}
	result, err := CheckRegistry(path, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.MissingRequired) != 0 || len(result.Extra) != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestCheckRegistryRejectsWrongImportDuplicateAndShadow(t *testing.T) {
	manifest := &Manifest{
		ModulePath: "example.com/things", PbDir: "service/dmsvr/pb/dm",
		Registry: RegistrySpec{Selector: "dm"},
		Services: []ServiceEntry{{Service: "DeviceManage", Registration: RegistrationRequired}},
	}
	tests := map[string]string{
		"wrong import": `package registry
import dm "example.com/other"
func register() { dm.RegisterDeviceManageServer(nil, nil) }`,
		"duplicate": `package registry
import dm "example.com/things/service/dmsvr/pb/dm"
func register() {
	dm.RegisterDeviceManageServer(nil, nil)
	dm.RegisterDeviceManageServer(nil, nil)
}`,
		"shadowed": `package registry
import dm "example.com/things/service/dmsvr/pb/dm"
type fake struct{}
func (fake) RegisterDeviceManageServer(any, any) {}
func register() {
	dm := fake{}
	dm.RegisterDeviceManageServer(nil, nil)
}`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "registry.go")
			if err := os.WriteFile(path, []byte(source), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := CheckRegistry(path, manifest); err == nil || !strings.HasPrefix(err.Error(), "registry:") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestRegistryServices(t *testing.T) {
	dir := t.TempDir()
	content := `package sysdirect
import "gitee.com/ming-iot/saas/service/syssvr/pb/sys"
func registerServers() {
	sys.RegisterAccessManageServer(nil, nil)
	sys.RegisterOpsServer(nil, nil)
}`
	path := filepath.Join(dir, "sysdirect.go")
	os.WriteFile(path, []byte(content), 0644)

	svcs, err := RegistryServices(path, "sys")
	if err != nil {
		t.Fatal(err)
	}
	// RegisterOpsServer extracts as "Ops" (Go identifier casing);
	// the proto service name is lowercase "ops", but the register function
	// uses PascalCase. RegistryServices reports the Go-extracted name.
	want := []string{"AccessManage", "Ops"}
	if len(svcs) != 2 {
		t.Fatalf("got %v, want %v", svcs, want)
	}
	found := map[string]bool{}
	for _, s := range svcs {
		found[s] = true
	}
	if !found["AccessManage"] || !found["Ops"] {
		t.Fatalf("got %v, want AccessManage + Ops", svcs)
	}
}
