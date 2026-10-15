package rpcgen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"ming-iot/backend/tools/internal/protocompat"
)

// GenerateDescriptorSet invokes the pinned protoc toolchain for the explicit
// manifest inputs and returns the decoded descriptor set.
func GenerateDescriptorSet(m *Manifest, moduleRoot, protobufToolsDir string) (*descriptorpb.FileDescriptorSet, error) {
	protoc := filepath.Join(protobufToolsDir, "bin", "protoc")
	include := filepath.Join(protobufToolsDir, "include")
	if _, err := os.Stat(protoc); err != nil {
		return nil, fmt.Errorf("proto_graph: protoc not found at %s: %w", protoc, err)
	}
	protoDir := filepath.Join(moduleRoot, m.ProtoDir)
	supporting := m.SupportingProtos
	if len(supporting) == 0 {
		supporting = m.Shared
	}
	inputs := append([]string(nil), supporting...)
	for _, service := range m.Services {
		inputs = append(inputs, service.Proto)
	}
	inputs = append(inputs, m.Aggregate)

	tmpSet, err := os.CreateTemp("", "rpcgen-*.protoset")
	if err != nil {
		return nil, fmt.Errorf("proto_graph: create temporary descriptor: %w", err)
	}
	name := tmpSet.Name()
	if err := tmpSet.Close(); err != nil {
		os.Remove(name)
		return nil, fmt.Errorf("proto_graph: close temporary descriptor: %w", err)
	}
	defer os.Remove(name)

	args := []string{
		"-I", protoDir,
		"-I", include,
		"--include_imports",
		"--descriptor_set_out=" + name,
	}
	args = append(args, inputs...)
	cmd := exec.Command(protoc, args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("proto_graph: protoc generate descriptor: %w", err)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("proto_graph: read descriptor set: %w", err)
	}
	set := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(data, set); err != nil {
		return nil, fmt.Errorf("proto_graph: unmarshal descriptor set: %w", err)
	}
	return set, nil
}

// CheckDescriptor generates and validates the manifest descriptor contract.
func CheckDescriptor(m *Manifest, moduleRoot, protobufToolsDir string) error {
	set, err := GenerateDescriptorSet(m, moduleRoot, protobufToolsDir)
	if err != nil {
		return err
	}
	return CheckDescriptorSet(m, set)
}

// CheckDescriptorSet validates the file graph and package-level service set
// without invoking protoc.
func CheckDescriptorSet(m *Manifest, set *descriptorpb.FileDescriptorSet) error {
	if err := CheckProtoGraph(m, set); err != nil {
		return err
	}
	contract, err := protocompat.Build(set, m.Package)
	if err != nil {
		return fmt.Errorf("proto_graph: build package contract: %w", err)
	}
	manifestServices := map[string]bool{}
	for _, service := range m.Services {
		manifestServices[m.Package+"."+service.Service] = true
	}
	for service := range manifestServices {
		if _, ok := contract.Services[service]; !ok {
			return fmt.Errorf("proto_graph: manifest service %q not found in descriptor", service)
		}
	}
	for service := range contract.Services {
		if !manifestServices[service] {
			return fmt.Errorf("proto_graph: descriptor service %q not declared in manifest", service)
		}
	}
	return nil
}

// CheckAll runs manifest, descriptor, and registration validation.
func CheckAll(m *Manifest, moduleRoot, protobufToolsDir string) error {
	if err := ValidateManifest(m); err != nil {
		return err
	}
	if err := ValidateManifestFiles(m, moduleRoot); err != nil {
		return err
	}
	if err := CheckDescriptor(m, moduleRoot, protobufToolsDir); err != nil {
		return err
	}
	result, err := CheckRegistry(m.AbsRegistry(moduleRoot), m)
	if err != nil {
		return err
	}
	if len(result.MissingRequired) > 0 {
		return fmt.Errorf("registry: required services not registered: %v", result.MissingRequired)
	}
	if len(result.PresentPreserveAbsent) > 0 {
		return fmt.Errorf("registry: preserve_absent services were registered: %v", result.PresentPreserveAbsent)
	}
	if len(result.Extra) > 0 {
		return fmt.Errorf("registry: registered services not in manifest: %v", result.Extra)
	}
	return nil
}
