// Package rpcgen orchestrates manifest-driven protobuf and Go code generation.
package rpcgen

import (
	"bytes"
	"fmt"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	RegistrationRequired       = "required"
	RegistrationPreserveAbsent = "preserve_absent"
)

// RegistrySpec locates the hand-written runtime registry and identifies the
// protobuf package selector used by its Register*Server calls.
type RegistrySpec struct {
	Path     string `yaml:"path"`
	Selector string `yaml:"selector"`
}

// UnmarshalYAML accepts the v1 scalar form and the v2 mapping form.
func (r *RegistrySpec) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		return fmt.Errorf("manifest: registry YAML node is nil")
	}
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag != "!!str" || strings.TrimSpace(node.Value) == "" {
			return fmt.Errorf("manifest: registry scalar must be a non-empty path")
		}
		r.Path = node.Value
		return nil
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return fmt.Errorf("manifest: malformed registry mapping")
		}
		seen := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if seen[key.Value] {
				return fmt.Errorf("manifest: duplicate registry field %q", key.Value)
			}
			seen[key.Value] = true
			switch key.Value {
			case "path":
				if err := value.Decode(&r.Path); err != nil {
					return fmt.Errorf("manifest: decode registry.path: %w", err)
				}
			case "selector":
				if err := value.Decode(&r.Selector); err != nil {
					return fmt.Errorf("manifest: decode registry.selector: %w", err)
				}
			default:
				return fmt.Errorf("manifest: unknown registry field %q", key.Value)
			}
		}
		return nil
	default:
		return fmt.Errorf("manifest: registry must be a path or mapping")
	}
}

// Manifest is the normalized in-memory representation of rpcgen.yaml.
// Registry is normalized from the v1 scalar or v2 mapping form.
type Manifest struct {
	Version              int            `yaml:"version"`
	Runtime              string         `yaml:"runtime"`
	ModulePath           string         `yaml:"modulePath"`
	RuntimeDir           string         `yaml:"runtimeDir"`
	ProtoDir             string         `yaml:"protoDir"`
	PbDir                string         `yaml:"pbDir"`
	Package              string         `yaml:"package"`
	GoPackage            string         `yaml:"goPackage"`
	PBSelector           string         `yaml:"pbSelector"`
	Aggregate            string         `yaml:"aggregate"`
	Shared               []string       `yaml:"shared"`
	SupportingProtos     []string       `yaml:"supportingProtos"`
	Registry             RegistrySpec   `yaml:"registry"`
	ClientContractGolden string         `yaml:"clientContractGolden"`
	Services             []ServiceEntry `yaml:"services"`
}

// ServiceEntry describes one domain protobuf service and its generated files.
type ServiceEntry struct {
	Proto        string `yaml:"proto"`
	Service      string `yaml:"service"`
	PackageDir   string `yaml:"packageDir"`
	ClientFile   string `yaml:"clientFile"`
	ServerFile   string `yaml:"serverFile"`
	Registration string `yaml:"registration"`
}

// LoadManifest strictly decodes, normalizes and validates rpcgen.yaml.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("manifest: read %s: %w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var m Manifest
	if err := decoder.Decode(&m); err != nil {
		return nil, fmt.Errorf("manifest: decode %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("manifest: decode trailing document in %s: %w", path, err)
		}
		return nil, fmt.Errorf("manifest: multiple YAML documents in %s", path)
	}
	if err := normalizeManifest(&m); err != nil {
		return nil, err
	}
	if err := ValidateManifest(&m); err != nil {
		return nil, err
	}
	if err := ValidateManifestFiles(&m, inferModuleRoot(path, m.ProtoDir)); err != nil {
		return nil, err
	}
	return &m, nil
}

func normalizeManifest(m *Manifest) error {
	switch m.Version {
	case 1:
		m.ModulePath = "gitee.com/ming-iot/saas"
		m.RuntimeDir = "service/syssvr"
		m.PBSelector = "sys"
		m.SupportingProtos = append([]string(nil), m.Shared...)
		m.Registry.Selector = "sys"
		for i := range m.Services {
			m.Services[i].Registration = RegistrationRequired
		}
	case 2:
		if len(m.Shared) != 0 {
			return fmt.Errorf("manifest: v2 does not accept legacy shared; use supportingProtos")
		}
	default:
		return fmt.Errorf("manifest: unsupported version %d", m.Version)
	}
	return nil
}

// ValidateManifest validates semantic consistency without reading descriptors.
func ValidateManifest(m *Manifest) error {
	if m == nil {
		return fmt.Errorf("manifest: manifest is nil")
	}
	version := m.Version
	if version == 0 {
		version = 1
	}
	if version != 1 && version != 2 {
		return fmt.Errorf("manifest: unsupported version %d", m.Version)
	}
	if version == 2 {
		fields := []struct{ name, value string }{
			{"runtime", m.Runtime}, {"modulePath", m.ModulePath}, {"runtimeDir", m.RuntimeDir},
			{"protoDir", m.ProtoDir}, {"pbDir", m.PbDir}, {"package", m.Package},
			{"goPackage", m.GoPackage}, {"pbSelector", m.PBSelector}, {"aggregate", m.Aggregate},
		}
		for _, field := range fields {
			if field.value == "" {
				return fmt.Errorf("manifest: %s is empty", field.name)
			}
		}
		if len(m.SupportingProtos) == 0 {
			return fmt.Errorf("manifest: supportingProtos is empty")
		}
		if m.Registry.Path == "" || m.Registry.Selector == "" {
			return fmt.Errorf("manifest: registry path and selector are required")
		}
		if !validSelector(m.PBSelector) || !validSelector(m.Registry.Selector) {
			return fmt.Errorf("manifest: invalid protobuf selector")
		}
	}
	if err := validateManifestPaths(m); err != nil {
		return err
	}
	if m.Package == "" {
		return fmt.Errorf("manifest: package is empty")
	}
	if m.GoPackage == "" {
		return fmt.Errorf("manifest: goPackage is empty")
	}
	if m.Package != m.GoPackage && m.GoPackage != "pb/"+m.Package {
		return fmt.Errorf("manifest: package %q inconsistent with goPackage %q", m.Package, m.GoPackage)
	}
	if m.Aggregate == "" {
		return fmt.Errorf("manifest: aggregate is empty")
	}
	if len(m.Services) == 0 {
		return fmt.Errorf("manifest: no services")
	}

	seenProto := map[string]string{}
	addProto := func(name, owner string) error {
		if name == "" {
			return fmt.Errorf("manifest: %s proto is empty", owner)
		}
		if previous, ok := seenProto[name]; ok {
			return fmt.Errorf("manifest: duplicate proto %q in %s and %s", name, previous, owner)
		}
		seenProto[name] = owner
		return nil
	}
	if err := addProto(m.Aggregate, "aggregate"); err != nil {
		return err
	}
	for i, proto := range m.SupportingProtos {
		if err := addProto(proto, fmt.Sprintf("supportingProtos[%d]", i)); err != nil {
			return err
		}
	}

	seenService := map[string]bool{}
	seenPkgDir := map[string]bool{}
	seenClient := map[string]bool{}
	seenServer := map[string]bool{}
	for i, service := range m.Services {
		if service.Proto == "" || service.Service == "" || service.PackageDir == "" || service.ClientFile == "" || service.ServerFile == "" {
			return fmt.Errorf("manifest: services[%d] has empty field: %+v", i, service)
		}
		if err := addProto(service.Proto, fmt.Sprintf("services[%d]", i)); err != nil {
			return err
		}
		if seenService[service.Service] {
			return fmt.Errorf("manifest: duplicate service %q", service.Service)
		}
		if seenPkgDir[service.PackageDir] {
			return fmt.Errorf("manifest: duplicate packageDir %q", service.PackageDir)
		}
		if seenClient[service.ClientFile] {
			return fmt.Errorf("manifest: duplicate clientFile %q", service.ClientFile)
		}
		if seenServer[service.ServerFile] {
			return fmt.Errorf("manifest: duplicate serverFile %q", service.ServerFile)
		}
		if version == 2 && service.Registration != RegistrationRequired && service.Registration != RegistrationPreserveAbsent {
			return fmt.Errorf("manifest: services[%d] has invalid registration %q", i, service.Registration)
		}
		seenService[service.Service] = true
		seenPkgDir[service.PackageDir] = true
		seenClient[service.ClientFile] = true
		seenServer[service.ServerFile] = true
	}
	return nil
}

// ValidateManifestFiles verifies all explicitly referenced source and registry files.
func ValidateManifestFiles(m *Manifest, moduleRoot string) error {
	if m == nil {
		return fmt.Errorf("manifest: manifest is nil")
	}
	for _, path := range m.ProtoAbsPaths(moduleRoot) {
		if err := requireManifestFile(moduleRoot, path); err != nil {
			return err
		}
	}
	registry := m.Registry.Path
	if registry != "" {
		if err := requireManifestFile(moduleRoot, filepath.Join(moduleRoot, registry)); err != nil {
			return err
		}
	}
	if m.ClientContractGolden != "" {
		if err := requireManifestFile(moduleRoot, filepath.Join(moduleRoot, m.ClientContractGolden)); err != nil {
			return err
		}
	}
	return nil
}

func requireManifestFile(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("manifest: referenced path escapes module root: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("manifest: referenced file %s: %w", rel, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("manifest: referenced path %s is not a regular file", rel)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("manifest: resolve module root %s: %w", root, err)
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("manifest: resolve referenced file %s: %w", rel, err)
	}
	realRel, err := filepath.Rel(realRoot, realPath)
	if err != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) || filepath.IsAbs(realRel) {
		return fmt.Errorf("manifest: referenced file %s escapes module root", rel)
	}
	return nil
}

func validateManifestPaths(m *Manifest) error {
	fields := []struct{ name, value string }{
		{"runtimeDir", m.RuntimeDir}, {"protoDir", m.ProtoDir}, {"pbDir", m.PbDir},
		{"aggregate", m.Aggregate}, {"registry.path", m.Registry.Path},
	}
	for _, field := range fields {
		if field.value != "" {
			if err := validateRelativePath(field.name, field.value, true); err != nil {
				return err
			}
		}
	}
	if m.ClientContractGolden != "" {
		if err := validateRelativePath("clientContractGolden", m.ClientContractGolden, true); err != nil {
			return err
		}
	}
	for i, proto := range m.SupportingProtos {
		if err := validateRelativePath(fmt.Sprintf("supportingProtos[%d]", i), proto, true); err != nil {
			return err
		}
	}
	for i, service := range m.Services {
		if err := validateRelativePath(fmt.Sprintf("services[%d].proto", i), service.Proto, true); err != nil {
			return err
		}
		servicePaths := []struct{ name, value string }{
			{"packageDir", service.PackageDir}, {"clientFile", service.ClientFile}, {"serverFile", service.ServerFile},
		}
		for _, field := range servicePaths {
			if err := validateRelativePath(fmt.Sprintf("services[%d].%s", i, field.name), field.value, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRelativePath(name, value string, allowNested bool) error {
	if value == "" {
		return nil
	}
	if strings.Contains(value, "\\") || filepath.IsAbs(value) || filepath.VolumeName(value) != "" {
		return fmt.Errorf("manifest: path %s=%q must be relative", name, value)
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("manifest: path %s=%q escapes module root", name, value)
	}
	if !allowNested && strings.Contains(clean, string(filepath.Separator)) {
		return fmt.Errorf("manifest: path %s=%q must be one component", name, value)
	}
	return nil
}

func inferModuleRoot(manifestPath, protoDir string) string {
	dir := filepath.Clean(filepath.Dir(manifestPath))
	protoDir = filepath.Clean(filepath.FromSlash(protoDir))
	if protoDir != "." && (dir == protoDir || strings.HasSuffix(dir, string(filepath.Separator)+protoDir)) {
		for range strings.Split(protoDir, string(filepath.Separator)) {
			dir = filepath.Dir(dir)
		}
	}
	return dir
}

func validSelector(value string) bool {
	return value != "_" && token.IsIdentifier(value)
}

// ProtoAbsPaths returns all explicitly listed protobuf paths.
func (m *Manifest) ProtoAbsPaths(moduleRoot string) []string {
	supporting := m.SupportingProtos
	if len(supporting) == 0 {
		supporting = m.Shared
	}
	out := make([]string, 0, len(supporting)+len(m.Services)+1)
	for _, proto := range supporting {
		out = append(out, filepath.Join(moduleRoot, m.ProtoDir, proto))
	}
	for _, service := range m.Services {
		out = append(out, filepath.Join(moduleRoot, m.ProtoDir, service.Proto))
	}
	out = append(out, filepath.Join(moduleRoot, m.ProtoDir, m.Aggregate))
	return out
}

// ServiceCount returns the number of domain services.
func (m *Manifest) ServiceCount() int { return len(m.Services) }
