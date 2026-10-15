package rpcgen

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidateManifest(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		m := &Manifest{
			Package:   "sys",
			GoPackage: "pb/sys",
			Aggregate: "sys.proto",
			Services: []ServiceEntry{
				{Proto: "a.proto", Service: "A", PackageDir: "a", ClientFile: "a.go", ServerFile: "aServer.go"},
			},
		}
		if err := ValidateManifest(m); err != nil {
			t.Fatalf("expected valid, got %v", err)
		}
	})
	t.Run("package/goPackage mismatch", func(t *testing.T) {
		m := &Manifest{Package: "sys", GoPackage: "other", Aggregate: "sys.proto", Services: []ServiceEntry{{Proto: "a.proto", Service: "A", PackageDir: "a", ClientFile: "a.go", ServerFile: "aS.go"}}}
		if err := ValidateManifest(m); err == nil {
			t.Fatal("expected mismatch error")
		}
	})
	t.Run("duplicate service", func(t *testing.T) {
		m := &Manifest{Package: "sys", GoPackage: "pb/sys", Aggregate: "sys.proto", Services: []ServiceEntry{
			{Proto: "a.proto", Service: "A", PackageDir: "a", ClientFile: "a.go", ServerFile: "aS.go"},
			{Proto: "b.proto", Service: "A", PackageDir: "b", ClientFile: "b.go", ServerFile: "bS.go"},
		}}
		if err := ValidateManifest(m); err == nil {
			t.Fatal("expected duplicate service error")
		}
	})
	t.Run("empty field", func(t *testing.T) {
		m := &Manifest{Package: "sys", GoPackage: "pb/sys", Aggregate: "sys.proto", Services: []ServiceEntry{{Proto: "a.proto", Service: "", PackageDir: "a", ClientFile: "a.go", ServerFile: "aS.go"}}}
		if err := ValidateManifest(m); err == nil {
			t.Fatal("expected empty field error")
		}
	})
}

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	content := `
version: 1
runtime: syssvr
protoDir: service/syssvr/proto
pbDir: service/syssvr/pb/sys
package: sys
goPackage: pb/sys
aggregate: sys.proto
shared:
  - base.proto
registry: service/syssvr/sysdirect/sysdirect.go
services:
  - proto: access.proto
    service: AccessManage
    packageDir: accessmanage
    clientFile: accessManage.go
    serverFile: accessManageServer.go
`
	path := filepath.Join(dir, "rpcgen.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	writeManifestFile(t, dir, "service/syssvr/proto/base.proto")
	writeManifestFile(t, dir, "service/syssvr/proto/access.proto")
	writeManifestFile(t, dir, "service/syssvr/proto/sys.proto")
	writeManifestFile(t, dir, "service/syssvr/sysdirect/sysdirect.go")
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if m.Package != "sys" || m.GoPackage != "pb/sys" {
		t.Fatalf("unexpected package/goPackage: %q/%q", m.Package, m.GoPackage)
	}
	if len(m.Services) != 1 || m.Services[0].Service != "AccessManage" {
		t.Fatalf("unexpected services: %+v", m.Services)
	}
	if len(m.Shared) != 1 || m.Shared[0] != "base.proto" {
		t.Fatalf("unexpected shared: %+v", m.Shared)
	}
	if got := manifestStringField(t, m, "ModulePath"); got != "gitee.com/ming-iot/saas" {
		t.Fatalf("v1 modulePath = %q", got)
	}
	if got := manifestStringField(t, m, "RuntimeDir"); got != "service/syssvr" {
		t.Fatalf("v1 runtimeDir = %q", got)
	}
	if got := manifestStringField(t, m, "PBSelector"); got != "sys" {
		t.Fatalf("v1 pbSelector = %q", got)
	}
	if got := manifestStringSliceField(t, m, "SupportingProtos"); !reflect.DeepEqual(got, []string{"base.proto"}) {
		t.Fatalf("v1 supportingProtos = %#v", got)
	}
	if got := manifestRegistry(t, m); got != (RegistrySpec{Path: "service/syssvr/sysdirect/sysdirect.go", Selector: "sys"}) {
		t.Fatalf("v1 registry = %#v", got)
	}
	if got := m.Services[0].Registration; got != "required" {
		t.Fatalf("v1 registration = %q", got)
	}
	if err := ValidateManifest(m); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
}

func TestLoadManifestV2UsesExplicitFields(t *testing.T) {
	dir := t.TempDir()
	content := `
version: 2
runtime: syssvr
modulePath: example.com/acme/saas
runtimeDir: service/custom
protoDir: proto
pbDir: pb/sys
package: sys
goPackage: pb/sys
pbSelector: custom
aggregate: aggregate.proto
supportingProtos:
  - shared/base.proto
registry:
  path: registry/sys.go
  selector: custom
services:
  - proto: access.proto
    service: AccessManage
    packageDir: accessmanage
    clientFile: access.go
    serverFile: accessServer.go
    registration: preserve_absent
`
	path := filepath.Join(dir, "rpcgen.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"proto/shared/base.proto", "proto/access.proto", "proto/aggregate.proto", "registry/sys.go"} {
		writeManifestFile(t, dir, name)
	}
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest v2: %v", err)
	}
	if got := manifestStringField(t, m, "ModulePath"); got != "example.com/acme/saas" {
		t.Fatalf("modulePath = %q", got)
	}
	if got := manifestStringField(t, m, "RuntimeDir"); got != "service/custom" {
		t.Fatalf("runtimeDir = %q", got)
	}
	if got := manifestStringField(t, m, "PBSelector"); got != "custom" {
		t.Fatalf("pbSelector = %q", got)
	}
	if got := manifestStringSliceField(t, m, "SupportingProtos"); !reflect.DeepEqual(got, []string{"shared/base.proto"}) {
		t.Fatalf("supportingProtos = %#v", got)
	}
	if got := manifestRegistry(t, m); got != (RegistrySpec{Path: "registry/sys.go", Selector: "custom"}) {
		t.Fatalf("registry = %#v", got)
	}
	if got := m.Services[0].Registration; got != "preserve_absent" {
		t.Fatalf("registration = %q", got)
	}
}

func TestLoadManifestRejectsUnknownAndSecondDocument(t *testing.T) {
	base := `
version: 1
runtime: syssvr
protoDir: proto
pbDir: pb/sys
package: sys
goPackage: pb/sys
aggregate: aggregate.proto
registry: registry.go
services:
  - proto: access.proto
    service: AccessManage
    packageDir: accessmanage
    clientFile: access.go
    serverFile: accessServer.go
`
	for _, tt := range []struct {
		name   string
		suffix string
		want   string
	}{
		{name: "unknown field", suffix: "unknown: true\n", want: "manifest:"},
		{name: "second document", suffix: "---\nversion: 1\n", want: "manifest:"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "rpcgen.yaml")
			if err := os.WriteFile(path, []byte(base+tt.suffix), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadManifest(path); err == nil || !strings.HasPrefix(err.Error(), tt.want) {
				t.Fatalf("LoadManifest error = %v, want prefix %q", err, tt.want)
			}
		})
	}
}

func TestLoadManifestV2DoesNotFillMissingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rpcgen.yaml")
	content := `
version: 2
runtime: syssvr
protoDir: proto
pbDir: pb/sys
package: sys
goPackage: pb/sys
aggregate: aggregate.proto
registry:
  path: registry.go
  selector: sys
services: []
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil || !strings.HasPrefix(err.Error(), "manifest:") {
		t.Fatalf("LoadManifest error = %v, want missing v2 field", err)
	}
}

func TestLoadManifestV1OverridesV2OnlyPolicyFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rpcgen.yaml")
	content := `version: 1
runtime: syssvr
protoDir: proto
pbDir: pb/sys
package: sys
goPackage: pb/sys
aggregate: aggregate.proto
shared: [base.proto]
registry:
  path: registry.go
  selector: custom
services:
  - {proto: access.proto, service: AccessManage, packageDir: accessmanage, clientFile: access.go, serverFile: accessServer.go, registration: preserve_absent}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"proto/base.proto", "proto/access.proto", "proto/aggregate.proto", "registry.go"} {
		writeManifestFile(t, dir, name)
	}
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Registry.Selector != "sys" || m.Services[0].Registration != RegistrationRequired {
		t.Fatalf("v1 policy fields not normalized: registry=%+v service=%+v", m.Registry, m.Services[0])
	}
}

func TestValidateManifestV2RejectsInvalidRegistration(t *testing.T) {
	m := duplicateManifestFixture()
	m.Services[0].Registration = "sometimes"
	if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "registration") {
		t.Fatalf("ValidateManifest error = %v", err)
	}
}

func TestValidateManifestV2RejectsUnsafePaths(t *testing.T) {
	tests := map[string]func(*Manifest){
		"runtime traversal":     func(m *Manifest) { m.RuntimeDir = "../outside" },
		"absolute proto dir":    func(m *Manifest) { m.ProtoDir = "/tmp/proto" },
		"package dir traversal": func(m *Manifest) { m.Services[0].PackageDir = "../service" },
		"nested client file":    func(m *Manifest) { m.Services[0].ClientFile = "nested/client.go" },
		"backslash server file": func(m *Manifest) { m.Services[0].ServerFile = `nested\server.go` },
		"registry traversal":    func(m *Manifest) { m.Registry.Path = "../registry.go" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			m := duplicateManifestFixture()
			mutate(m)
			if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "path") {
				t.Fatalf("ValidateManifest error = %v", err)
			}
		})
	}
}

func TestValidateManifestRejectsAllReferenceDuplicates(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{name: "clientFile", mutate: func(m *Manifest) { m.Services[1].ClientFile = m.Services[0].ClientFile }, want: "clientFile"},
		{name: "serverFile", mutate: func(m *Manifest) { m.Services[1].ServerFile = m.Services[0].ServerFile }, want: "serverFile"},
		{name: "packageDir", mutate: func(m *Manifest) { m.Services[1].PackageDir = m.Services[0].PackageDir }, want: "packageDir"},
		{name: "service proto", mutate: func(m *Manifest) { m.Services[1].Proto = m.Services[0].Proto }, want: "duplicate proto"},
		{name: "supporting and aggregate", mutate: func(m *Manifest) { m.SupportingProtos = []string{m.Aggregate} }, want: "duplicate proto"},
		{name: "supporting and service", mutate: func(m *Manifest) { m.SupportingProtos = []string{m.Services[0].Proto} }, want: "duplicate proto"},
		{name: "aggregate and service", mutate: func(m *Manifest) { m.Aggregate = m.Services[0].Proto }, want: "duplicate proto"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := duplicateManifestFixture()
			tt.mutate(m)
			if err := ValidateManifest(m); err == nil || !strings.HasPrefix(err.Error(), "manifest:") || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateManifest error = %v, want manifest error containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateManifestFilesRejectsMissingReferences(t *testing.T) {
	root := t.TempDir()
	m := duplicateManifestFixture()
	m.ProtoDir = "proto"
	m.Registry.Path = "registry.go"
	m.ClientContractGolden = "client/public-api.json"
	for _, name := range []string{"proto/base.proto", "proto/a.proto", "proto/b.proto", "proto/aggregate.proto", "registry.go"} {
		writeManifestFile(t, root, name)
	}
	if err := ValidateManifestFiles(m, root); err == nil || !strings.Contains(err.Error(), "client/public-api.json") {
		t.Fatalf("ValidateManifestFiles error = %v", err)
	}
	writeManifestFile(t, root, "client/public-api.json")
	if err := ValidateManifestFiles(m, root); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "proto/a.proto")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateManifestFiles(m, root); err == nil || !strings.Contains(err.Error(), "proto/a.proto") {
		t.Fatalf("ValidateManifestFiles error = %v", err)
	}
}

func TestValidateManifestFilesRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.proto")
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	m := duplicateManifestFixture()
	m.ProtoDir = "proto"
	m.Registry.Path = "registry.go"
	for _, name := range []string{"proto/base.proto", "proto/b.proto", "proto/aggregate.proto", "registry.go"} {
		writeManifestFile(t, root, name)
	}
	if err := os.MkdirAll(filepath.Join(root, "proto"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "proto/a.proto")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateManifestFiles(m, root); err == nil || !strings.Contains(err.Error(), "escapes module root") {
		t.Fatalf("ValidateManifestFiles error = %v", err)
	}
}

func TestProtoAbsPaths(t *testing.T) {
	m := &Manifest{
		ProtoDir:         "service/syssvr/proto",
		Aggregate:        "sys.proto",
		Shared:           []string{"base.proto"},
		SupportingProtos: []string{"common/base.proto"},
		Services: []ServiceEntry{
			{Proto: "a.proto"},
			{Proto: "b.proto"},
		},
	}
	paths := m.ProtoAbsPaths("/core")
	want := []string{
		"/core/service/syssvr/proto/common/base.proto",
		"/core/service/syssvr/proto/a.proto",
		"/core/service/syssvr/proto/b.proto",
		"/core/service/syssvr/proto/sys.proto",
	}
	if len(paths) != len(want) {
		t.Fatalf("got %d paths, want %d: %v", len(paths), len(want), paths)
	}
	for i, p := range paths {
		if p != want[i] {
			t.Errorf("path[%d] = %q, want %q", i, p, want[i])
		}
	}
}

func writeManifestFile(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func manifestStringField(t *testing.T, m *Manifest, name string) string {
	t.Helper()
	field := reflect.ValueOf(m).Elem().FieldByName(name)
	if !field.IsValid() || field.Kind() != reflect.String {
		t.Fatalf("Manifest.%s is missing or not a string", name)
	}
	return field.String()
}

func manifestStringSliceField(t *testing.T, m *Manifest, name string) []string {
	t.Helper()
	field := reflect.ValueOf(m).Elem().FieldByName(name)
	if !field.IsValid() || field.Type() != reflect.TypeOf([]string{}) {
		t.Fatalf("Manifest.%s is missing or not []string", name)
	}
	return field.Interface().([]string)
}

func manifestRegistry(t *testing.T, m *Manifest) RegistrySpec {
	t.Helper()
	field := reflect.ValueOf(m).Elem().FieldByName("Registry")
	if !field.IsValid() || field.Type() != reflect.TypeOf(RegistrySpec{}) {
		t.Fatalf("Manifest.Registry is missing or has wrong type")
	}
	return field.Interface().(RegistrySpec)
}

func duplicateManifestFixture() *Manifest {
	return &Manifest{
		Version:          2,
		Runtime:          "syssvr",
		ModulePath:       "example.com/acme/saas",
		RuntimeDir:       "service/syssvr",
		ProtoDir:         "proto",
		PbDir:            "pb/sys",
		Package:          "sys",
		GoPackage:        "pb/sys",
		PBSelector:       "sys",
		Aggregate:        "aggregate.proto",
		SupportingProtos: []string{"base.proto"},
		Registry:         RegistrySpec{Path: "registry.go", Selector: "sys"},
		Services: []ServiceEntry{
			{Proto: "a.proto", Service: "A", PackageDir: "a", ClientFile: "a.go", ServerFile: "aServer.go", Registration: "required"},
			{Proto: "b.proto", Service: "B", PackageDir: "b", ClientFile: "b.go", ServerFile: "bServer.go", Registration: "required"},
		},
	}
}

func TestManifestErrorsHaveStablePrefix(t *testing.T) {
	for _, err := range []error{
		ValidateManifest(nil),
		ValidateManifest(&Manifest{}),
	} {
		if err == nil || !strings.HasPrefix(err.Error(), "manifest:") {
			t.Fatalf("error = %v, want manifest: prefix", err)
		}
	}
}

func TestRegistrySpecRejectsMalformedYAML(t *testing.T) {
	var spec RegistrySpec
	if err := spec.UnmarshalYAML(nil); err == nil {
		t.Fatal("expected nil YAML node error")
	}
	_ = fmt.Sprint(io.EOF)
}
