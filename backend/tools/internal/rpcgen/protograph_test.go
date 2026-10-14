package rpcgen

import (
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestCheckProtoGraphAcceptsDeterministicDAG(t *testing.T) {
	m, set := protoGraphFixture()

	if err := CheckProtoGraph(m, set); err != nil {
		t.Fatalf("CheckProtoGraph() error = %v", err)
	}
	got, err := TopologicalProtoOrder(m, set)
	if err != nil {
		t.Fatalf("TopologicalProtoOrder() error = %v", err)
	}
	want := []string{"a.proto", "other.proto", "z.proto", "svc.proto"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TopologicalProtoOrder() = %v, want %v", got, want)
	}
}

func TestCheckProtoGraphRejectsAggregateDefinitions(t *testing.T) {
	m, set := protoGraphFixture()
	set.File[0].MessageType = []*descriptorpb.DescriptorProto{{Name: strPtr("NotAllowed")}}

	assertProtoGraphError(t, CheckProtoGraph(m, set), "aggregate:")
}

func TestCheckProtoGraphRejectsMissingAggregatePublicImport(t *testing.T) {
	m, set := protoGraphFixture()
	set.File[0].PublicDependency = []int32{0, 1, 2}

	assertProtoGraphError(t, CheckProtoGraph(m, set), "aggregate:")
}

func TestCheckProtoGraphRejectsDependencyCycle(t *testing.T) {
	m, set := protoGraphFixture()
	set.File[1].Dependency = []string{"z.proto"}
	set.File[2].Dependency = []string{"a.proto"}

	assertProtoGraphError(t, CheckProtoGraph(m, set), "proto_graph:")
}

func TestCheckProtoGraphRejectsServiceInWrongFileAndDuplicateService(t *testing.T) {
	t.Run("wrong file", func(t *testing.T) {
		m, set := protoGraphFixture()
		set.File[3].Service = nil
		set.File[4].Service = []*descriptorpb.ServiceDescriptorProto{{Name: strPtr("Svc")}}

		assertProtoGraphError(t, CheckProtoGraph(m, set), "proto_graph:")
	})

	t.Run("duplicate", func(t *testing.T) {
		m, set := protoGraphFixture()
		set.File[4].Service = append(set.File[4].Service, &descriptorpb.ServiceDescriptorProto{Name: strPtr("Svc")})

		assertProtoGraphError(t, CheckProtoGraph(m, set), "proto_graph:")
	})
}

func TestCheckProtoGraphRejectsExtraService(t *testing.T) {
	m, set := protoGraphFixture()
	set.File[3].Service = append(set.File[3].Service, &descriptorpb.ServiceDescriptorProto{Name: strPtr("Extra")})

	assertProtoGraphError(t, CheckProtoGraph(m, set), "proto_graph:")
}

func TestCheckProtoGraphRejectsMissingOrDuplicateExplicitFiles(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		m, set := protoGraphFixture()
		set.File = set.File[:len(set.File)-1]

		assertProtoGraphError(t, CheckProtoGraph(m, set), "proto_graph:")
	})

	t.Run("duplicate", func(t *testing.T) {
		m, set := protoGraphFixture()
		set.File = append(set.File, cloneFile(set.File[1]))

		assertProtoGraphError(t, CheckProtoGraph(m, set), "proto_graph:")
	})
}

func TestCheckProtoGraphRejectsPackageAndGoPackageMismatch(t *testing.T) {
	t.Run("package", func(t *testing.T) {
		m, set := protoGraphFixture()
		set.File[1].Package = strPtr("other")

		assertProtoGraphError(t, CheckProtoGraph(m, set), "proto_graph:")
	})

	t.Run("go package", func(t *testing.T) {
		m, set := protoGraphFixture()
		set.File[1].Options.GoPackage = strPtr("example.com/other/pb/sys;sys")

		assertProtoGraphError(t, CheckProtoGraph(m, set), "proto_graph:")
	})

	t.Run("missing go package", func(t *testing.T) {
		m, set := protoGraphFixture()
		set.File[1].Options.GoPackage = nil

		assertProtoGraphError(t, CheckProtoGraph(m, set), "proto_graph:")
	})
}

func protoGraphFixture() (*Manifest, *descriptorpb.FileDescriptorSet) {
	m := &Manifest{
		Version:          2,
		ModulePath:       "example.com/acme",
		Package:          "sys",
		GoPackage:        "pb/sys",
		Aggregate:        "aggregate.proto",
		SupportingProtos: []string{"z.proto", "a.proto"},
		Services: []ServiceEntry{
			{Proto: "svc.proto", Service: "Svc"},
			{Proto: "other.proto", Service: "Other"},
		},
	}
	option := &descriptorpb.FileOptions{GoPackage: strPtr("pb/sys;sys")}
	set := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{
		file("aggregate.proto", strPtr("sys"), []string{"z.proto", "a.proto", "svc.proto", "other.proto"}, []int32{0, 1, 2, 3}, option),
		file("z.proto", strPtr("sys"), []string{"a.proto"}, nil, option),
		file("a.proto", strPtr("sys"), nil, nil, option),
		file("svc.proto", strPtr("sys"), []string{"z.proto"}, nil, option, "Svc"),
		file("other.proto", strPtr("sys"), []string{"a.proto"}, nil, option, "Other"),
	}}
	return m, set
}

func file(name string, pkg *string, deps []string, public []int32, option *descriptorpb.FileOptions, services ...string) *descriptorpb.FileDescriptorProto {
	f := &descriptorpb.FileDescriptorProto{Name: strPtr(name), Package: pkg, Dependency: deps, PublicDependency: public, Options: cloneOptions(option)}
	for _, service := range services {
		f.Service = append(f.Service, &descriptorpb.ServiceDescriptorProto{Name: strPtr(service)})
	}
	return f
}

func cloneFile(f *descriptorpb.FileDescriptorProto) *descriptorpb.FileDescriptorProto {
	name := f.GetName()
	return file(name, f.Package, append([]string(nil), f.Dependency...), append([]int32(nil), f.PublicDependency...), f.Options, serviceNames(f.Service)...)
}

func cloneOptions(o *descriptorpb.FileOptions) *descriptorpb.FileOptions {
	if o == nil {
		return nil
	}
	return proto.Clone(o).(*descriptorpb.FileOptions)
}

func serviceNames(services []*descriptorpb.ServiceDescriptorProto) []string {
	result := make([]string, 0, len(services))
	for _, service := range services {
		result = append(result, service.GetName())
	}
	return result
}

func assertProtoGraphError(t *testing.T, err error, prefix string) {
	t.Helper()
	if err == nil || !strings.HasPrefix(err.Error(), prefix) {
		t.Fatalf("error = %v, want prefix %q", err, prefix)
	}
}

func strPtr(value string) *string { return &value }
