package rpcgen

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/descriptorpb"
	"ming-iot/backend/tools/internal/clientcontract"
	"ming-iot/backend/tools/internal/protocompat"
)

func TestCaptureCompatibilityFromSetFreezesRuntimeSurface(t *testing.T) {
	root, manifest, set := compatibilityFixture(t)
	snapshot, err := captureCompatibilityFromSet(root, manifest, set)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(snapshot.DirectMethods); got != 10 {
		t.Fatalf("direct methods = %d", got)
	}
	if got := len(snapshot.DirectFacades); got != 10 {
		t.Fatalf("direct facades = %d", got)
	}
	if got := len(snapshot.Registrations); got != 9 {
		t.Fatalf("registrations = %d", got)
	}
	if got := len(snapshot.ClientAPI.Functions); got != 20 {
		t.Fatalf("client constructors = %d", got)
	}
	if _, ok := snapshot.ProtectedSHA256["service/dmsvr/proto/schemaInfo.proto"]; !ok {
		t.Fatalf("protected paths = %v", snapshot.ProtectedSHA256)
	}
}

func TestCaptureCompatibilityFromSetRejectsNonDirectAdapter(t *testing.T) {
	root, manifest, set := compatibilityFixture(t)
	path := filepath.Join(root, "service/dmsvr/client/service0/service0.go")
	bad := `package service0
type Req = dm.Req
type Service0 interface { Read(ctx context.Context, in *Req, opts ...grpc.CallOption) (*Resp, error) }
func NewService0(cli any) Service0 { return nil }
func NewDirectService0(svc any, svr dm.Service0Server) Service0 { return nil }
type directService0 struct { svr dm.Service0Server }
func (d *directService0) Read(ctx context.Context, in *Req, opts ...grpc.CallOption) (*Resp, error) {
	if in == nil { return nil, nil }
	return d.svr.Read(ctx, in)
}`
	if err := os.WriteFile(path, []byte(bad), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := captureCompatibilityFromSet(root, manifest, set); err == nil || !strings.Contains(err.Error(), "direct") {
		t.Fatalf("err = %v", err)
	}
}

func TestCaptureCompatibilityFromSetRejectsChangedDirectArguments(t *testing.T) {
	root, manifest, set := compatibilityFixture(t)
	path := filepath.Join(root, "service/dmsvr/client/service0/service0.go")
	bad := `package service0
type Req = dm.Req
type Resp = dm.Resp
type Service0 interface { Read(ctx context.Context, in *Req, opts ...grpc.CallOption) (*Resp, error) }
func NewService0(cli any) Service0 { return nil }
func NewDirectService0(svc any, svr dm.Service0Server) Service0 { return nil }
type directService0 struct { svr dm.Service0Server }
func (d *directService0) Read(ctx context.Context, in *Req, opts ...grpc.CallOption) (*Resp, error) {
	return d.svr.Read(ctx, nil)
}`
	if err := os.WriteFile(path, []byte(bad), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := captureCompatibilityFromSet(root, manifest, set); err == nil || !strings.Contains(err.Error(), "arguments") {
		t.Fatalf("err = %v", err)
	}
}

func TestCaptureCompatibilityFromSetRejectsProtectedSymlink(t *testing.T) {
	root, manifest, set := compatibilityFixture(t)
	outside := t.TempDir()
	link := filepath.Join(root, "service/dmsvr/etc/outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := captureCompatibilityFromSet(root, manifest, set); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompareCompatibilityAllowsAdditiveGeneratedClientSurface(t *testing.T) {
	before := CompatibilitySnapshot{
		Descriptor: protocompat.Contract{Package: "dm"},
		ClientAPI: clientcontract.Document{
			Aliases:    []clientcontract.Alias{{Package: "productmanage", Name: "OldReq", Target: "dm.OldReq"}},
			Interfaces: []clientcontract.Interface{{Package: "productmanage", Name: "ProductManage", Methods: []clientcontract.Method{{Name: "Old", Signature: "Old(ctx context.Context)"}}}},
			Functions:  []clientcontract.Function{{Package: "productmanage", Name: "NewProductManage", Signature: "func NewProductManage() ProductManage"}},
		},
		DirectMethods: []DirectMethod{{Package: "productmanage", Receiver: "directProductManage", Name: "Old", CallsSVR: "Old", Signature: "Old(ctx context.Context)"}},
	}
	after := before
	after.ClientAPI.Aliases = append(append([]clientcontract.Alias{}, before.ClientAPI.Aliases...), clientcontract.Alias{Package: "productmanage", Name: "NewReq", Target: "dm.NewReq"})
	after.ClientAPI.Interfaces = []clientcontract.Interface{{Package: "productmanage", Name: "ProductManage", Methods: []clientcontract.Method{
		{Name: "Old", Signature: "Old(ctx context.Context)"},
		{Name: "Added", Signature: "Added(ctx context.Context)"},
	}}}
	after.DirectMethods = append(append([]DirectMethod{}, before.DirectMethods...), DirectMethod{Package: "productmanage", Receiver: "directProductManage", Name: "Added", CallsSVR: "Added", Signature: "Added(ctx context.Context)"})
	if err := CompareCompatibility(before, after); err != nil {
		t.Fatalf("additive generated surface rejected: %v", err)
	}
}

func TestCompareCompatibilityRejectsRemovedOrChangedGeneratedClientSurface(t *testing.T) {
	base := CompatibilitySnapshot{
		Descriptor: protocompat.Contract{Package: "dm"},
		ClientAPI: clientcontract.Document{
			Aliases:    []clientcontract.Alias{{Package: "productmanage", Name: "Req", Target: "dm.Req"}},
			Interfaces: []clientcontract.Interface{{Package: "productmanage", Name: "ProductManage", Methods: []clientcontract.Method{{Name: "Read", Signature: "Read(ctx context.Context)"}}}},
		},
		DirectMethods: []DirectMethod{{Package: "productmanage", Receiver: "directProductManage", Name: "Read", CallsSVR: "Read", Signature: "Read(ctx context.Context)"}},
	}
	tests := map[string]CompatibilitySnapshot{
		"removed alias": {
			Descriptor:    base.Descriptor,
			ClientAPI:     clientcontract.Document{Interfaces: base.ClientAPI.Interfaces},
			DirectMethods: base.DirectMethods,
		},
		"changed method": {
			Descriptor: base.Descriptor,
			ClientAPI: clientcontract.Document{
				Aliases:    base.ClientAPI.Aliases,
				Interfaces: []clientcontract.Interface{{Package: "productmanage", Name: "ProductManage", Methods: []clientcontract.Method{{Name: "Read", Signature: "Read(ctx context.Context, id int64)"}}}},
			},
			DirectMethods: base.DirectMethods,
		},
		"removed direct method": {Descriptor: base.Descriptor, ClientAPI: base.ClientAPI},
	}
	for name, after := range tests {
		t.Run(name, func(t *testing.T) {
			if err := CompareCompatibility(base, after); err == nil {
				t.Fatal("destructive generated surface change was accepted")
			}
		})
	}
}

func TestCompareCompatibilityReportsProtectedPath(t *testing.T) {
	root, manifest, set := compatibilityFixture(t)
	before, err := captureCompatibilityFromSet(root, manifest, set)
	if err != nil {
		t.Fatal(err)
	}
	after, err := captureCompatibilityFromSet(root, manifest, set)
	if err != nil {
		t.Fatal(err)
	}
	path := "service/dmsvr/proto/schemaInfo.proto"
	after.ProtectedSHA256[path] = "changed"
	if err := CompareCompatibility(before, after); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("err = %v", err)
	}
}

func TestCompareCompatibilityNormalizesSnapshotOrder(t *testing.T) {
	root, manifest, set := compatibilityFixture(t)
	before, err := captureCompatibilityFromSet(root, manifest, set)
	if err != nil {
		t.Fatal(err)
	}
	after, err := captureCompatibilityFromSet(root, manifest, set)
	if err != nil {
		t.Fatal(err)
	}
	reverseStrings(after.Registrations)
	reverseStrings(after.DirectFacades)
	for left, right := 0, len(after.DirectMethods)-1; left < right; left, right = left+1, right-1 {
		after.DirectMethods[left], after.DirectMethods[right] = after.DirectMethods[right], after.DirectMethods[left]
	}
	for left, right := 0, len(after.ClientAPI.Functions)-1; left < right; left, right = left+1, right-1 {
		after.ClientAPI.Functions[left], after.ClientAPI.Functions[right] = after.ClientAPI.Functions[right], after.ClientAPI.Functions[left]
	}
	if err := CompareCompatibility(before, after); err != nil {
		t.Fatalf("order-only change rejected: %v", err)
	}
}

func TestCompareCompatibilityReportsDirectSignatureChange(t *testing.T) {
	root, manifest, set := compatibilityFixture(t)
	before, err := captureCompatibilityFromSet(root, manifest, set)
	if err != nil {
		t.Fatal(err)
	}
	after, err := captureCompatibilityFromSet(root, manifest, set)
	if err != nil {
		t.Fatal(err)
	}
	after.DirectMethods[0].Signature = "Read(ctx context.Context, in any) (any, error)"
	if err := CompareCompatibility(before, after); err == nil || !strings.Contains(err.Error(), "direct methods") {
		t.Fatalf("err = %v", err)
	}
}

func TestDMSVRGeneratedCompatibility(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	backendRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	moduleRoot := filepath.Join(backendRoot, "things")
	manifest, err := LoadManifest(filepath.Join(moduleRoot, "service", "dmsvr", "proto", "rpcgen.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := CaptureCompatibility(moduleRoot, manifest, filepath.Join(backendRoot, "bin", "protobuf-tools"))
	if err != nil {
		t.Fatal(err)
	}
	descriptorSet, err := GenerateDescriptorSet(manifest, moduleRoot, filepath.Join(backendRoot, "bin", "protobuf-tools"))
	if err != nil {
		t.Fatal(err)
	}
	schemaMessages := 0
	for _, file := range descriptorSet.File {
		if file.GetName() == "schemaInfo.proto" {
			schemaMessages = len(file.MessageType)
		}
	}
	methodCount := 0
	for _, service := range snapshot.Descriptor.Services {
		methodCount += len(service.Methods)
	}
	counts := map[string]int{
		"services":         len(snapshot.Descriptor.Services),
		"methods":          methodCount,
		"domainMessages":   len(snapshot.Descriptor.TopLevelMessages) - schemaMessages,
		"schemaMessages":   schemaMessages,
		"enums":            len(snapshot.Descriptor.Enums),
		"clientInterfaces": len(snapshot.ClientAPI.Interfaces),
		"clientFunctions":  len(snapshot.ClientAPI.Functions),
		"clientAliases":    len(snapshot.ClientAPI.Aliases),
		"directMethods":    len(snapshot.DirectMethods),
		"directFacades":    len(snapshot.DirectFacades),
		"registrations":    len(snapshot.Registrations),
	}
	want := map[string]int{
		"services": 10, "methods": 186, "domainMessages": 285, "schemaMessages": 6, "enums": 0,
		"clientInterfaces": 10, "clientFunctions": 20, "clientAliases": 2644,
		"directMethods": 186, "directFacades": 10, "registrations": 9,
	}
	for name, expected := range want {
		if counts[name] != expected {
			t.Errorf("%s = %d, want %d", name, counts[name], expected)
		}
	}
	for _, registration := range snapshot.Registrations {
		if strings.Contains(registration, "SchemaManage") {
			t.Errorf("SchemaManage must remain absent from runtime registrations: %s", registration)
		}
	}
}

func reverseStrings(values []string) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func compatibilityFixture(t *testing.T) (string, *Manifest, *descriptorpb.FileDescriptorSet) {
	t.Helper()
	root := t.TempDir()
	m := &Manifest{
		Version: 2, Runtime: "dmsvr", ModulePath: "example.com/things", RuntimeDir: "service/dmsvr",
		ProtoDir: "service/dmsvr/proto", PbDir: "service/dmsvr/pb/dm", Package: "dm", GoPackage: "pb/dm",
		PBSelector: "dm", Aggregate: "dm.proto", SupportingProtos: []string{"schemaInfo.proto"},
		Registry: RegistrySpec{Path: "service/dmsvr/dmdirect/dmdirect.go", Selector: "dm"},
	}
	goPackage := &descriptorpb.FileOptions{GoPackage: strPtr("pb/dm;dm")}
	aggregate := file("dm.proto", strPtr("dm"), nil, nil, goPackage)
	shared := file("schemaInfo.proto", strPtr("dm"), nil, nil, goPackage)
	shared.MessageType = []*descriptorpb.DescriptorProto{{Name: strPtr("Req")}, {Name: strPtr("Resp")}}
	set := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{aggregate, shared}}

	var registry strings.Builder
	registry.WriteString("package dmdirect\nimport dm \"example.com/things/service/dmsvr/pb/dm\"\nfunc register() {\n")
	var facades strings.Builder
	facades.WriteString("package dmdirect\n")
	for i := 0; i < 10; i++ {
		service := fmt.Sprintf("Service%d", i)
		protoName := fmt.Sprintf("service%d.proto", i)
		registration := RegistrationRequired
		if i == 9 {
			registration = RegistrationPreserveAbsent
		}
		m.Services = append(m.Services, ServiceEntry{
			Proto: protoName, Service: service, PackageDir: fmt.Sprintf("service%d", i),
			ClientFile: fmt.Sprintf("service%d.go", i), ServerFile: fmt.Sprintf("service%dServer.go", i), Registration: registration,
		})
		aggregate.Dependency = append(aggregate.Dependency, protoName)
		aggregate.PublicDependency = append(aggregate.PublicDependency, int32(len(aggregate.Dependency)-1))
		svcFile := file(protoName, strPtr("dm"), []string{"schemaInfo.proto"}, nil, goPackage, service)
		svcFile.Service[0].Method = []*descriptorpb.MethodDescriptorProto{{
			Name: strPtr("Read"), InputType: strPtr(".dm.Req"), OutputType: strPtr(".dm.Resp"),
		}}
		set.File = append(set.File, svcFile)
		client := fmt.Sprintf(`package service%d
type Req = dm.Req
type Resp = dm.Resp
type Service%d interface { Read(ctx context.Context, in *Req, opts ...grpc.CallOption) (*Resp, error) }
func NewService%d(cli any) Service%d { return nil }
func NewDirectService%d(svc any, svr dm.Service%dServer) Service%d { return nil }
type directService%d struct { svr dm.Service%dServer }
func (d *directService%d) Read(ctx context.Context, in *Req, opts ...grpc.CallOption) (*Resp, error) {
	return d.svr.Read(ctx, in)
}
`, i, i, i, i, i, i, i, i, i, i)
		writeFixtureFile(t, root, filepath.Join("service/dmsvr/client", fmt.Sprintf("service%d/service%d.go", i, i)), client)
		facades.WriteString(fmt.Sprintf("func NewService%d(runSvr bool) any { return nil }\n", i))
		if i < 9 {
			registry.WriteString(fmt.Sprintf("dm.RegisterService%dServer(nil, nil)\n", i))
		}
		writeFixtureFile(t, root, filepath.Join("service/dmsvr/proto", protoName), "syntax = \"proto3\";\n")
	}
	registry.WriteString("}\n")
	aggregate.Dependency = append([]string{"schemaInfo.proto"}, aggregate.Dependency...)
	for i := range aggregate.PublicDependency {
		aggregate.PublicDependency[i]++
	}
	aggregate.PublicDependency = append([]int32{0}, aggregate.PublicDependency...)
	writeFixtureFile(t, root, "service/dmsvr/dmdirect/dmdirect.go", registry.String())
	writeFixtureFile(t, root, "service/dmsvr/dmdirect/facades.go", facades.String())
	writeFixtureFile(t, root, "service/dmsvr/proto/dm.proto", "syntax = \"proto3\";\n")
	writeFixtureFile(t, root, "service/dmsvr/proto/schemaInfo.proto", "syntax = \"proto3\";\n")
	writeFixtureFile(t, root, "service/dmsvr/etc/dm.yaml", "Name: dm\n")
	writeFixtureFile(t, root, "service/dmsvr/internal/logic/logic.go", "package logic\n")
	writeFixtureFile(t, root, "service/dmsvr/internal/repo/repo.go", "package repo\n")
	return root, m, set
}

func writeFixtureFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
