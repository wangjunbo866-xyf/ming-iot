package rpcgen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// RegistryCheck is the policy-aware comparison between manifest services and
// the hand-written runtime registration surface.
type RegistryCheck struct {
	MissingRequired       []string
	PresentPreserveAbsent []string
	Extra                 []string
}

// CheckRegistry compares AST-extracted Register*Server calls with each
// service's registration policy.
func CheckRegistry(registryPath string, m *Manifest) (RegistryCheck, error) {
	var result RegistryCheck
	if m == nil {
		return result, fmt.Errorf("registry: manifest is nil")
	}
	selector := m.Registry.Selector
	if selector == "" {
		selector = m.PBSelector
	}
	if selector == "" {
		selector = "sys"
	}
	expectedImport := ""
	if m.ModulePath != "" && m.PbDir != "" {
		expectedImport = pathpkg.Join(m.ModulePath, filepath.ToSlash(m.PbDir))
	}
	registered, err := RegistryServices(registryPath, selector, expectedImport)
	if err != nil {
		return result, err
	}
	registeredByLower := make(map[string]string, len(registered))
	for _, service := range registered {
		registeredByLower[strings.ToLower(service)] = service
	}

	declaredByLower := make(map[string]ServiceEntry, len(m.Services))
	for _, service := range m.Services {
		key := strings.ToLower(service.Service)
		declaredByLower[key] = service
		policy := service.Registration
		if policy == "" {
			policy = RegistrationRequired
		}
		_, present := registeredByLower[key]
		switch policy {
		case RegistrationRequired:
			if !present {
				result.MissingRequired = append(result.MissingRequired, service.Service)
			}
		case RegistrationPreserveAbsent:
			if present {
				result.PresentPreserveAbsent = append(result.PresentPreserveAbsent, service.Service)
			}
		default:
			return result, fmt.Errorf("registry: service %q has invalid registration %q", service.Service, policy)
		}
	}
	for lower, service := range registeredByLower {
		if _, ok := declaredByLower[lower]; !ok {
			result.Extra = append(result.Extra, service)
		}
	}
	sort.Strings(result.MissingRequired)
	sort.Strings(result.PresentPreserveAbsent)
	sort.Strings(result.Extra)
	return result, nil
}

// RegistryServices extracts Register*Server calls for selector from a Go file.
func RegistryServices(registryPath, selector string, expectedImport ...string) ([]string, error) {
	if !validSelector(selector) {
		return nil, fmt.Errorf("registry: invalid selector %q", selector)
	}
	if _, err := os.Stat(registryPath); err != nil {
		return nil, fmt.Errorf("registry: file %s: %w", registryPath, err)
	}
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, registryPath, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("registry: parse %s: %w", registryPath, err)
	}
	imports := map[string]string{}
	for _, spec := range node.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("registry: invalid import path %s: %w", spec.Path.Value, err)
		}
		alias := pathpkg.Base(importPath)
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		if alias != "_" && alias != "." {
			imports[alias] = importPath
		}
	}
	actualImport, ok := imports[selector]
	if !ok {
		return nil, fmt.Errorf("registry: selector %q is not a file import", selector)
	}
	if len(expectedImport) > 0 && expectedImport[0] != "" && actualImport != expectedImport[0] {
		return nil, fmt.Errorf("registry: selector %q imports %q, want %q", selector, actualImport, expectedImport[0])
	}
	seen := map[string]int{}
	shadowed := false
	ast.Inspect(node, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != selector {
			return true
		}
		if ident.Obj != nil {
			shadowed = true
			return true
		}
		name := sel.Sel.Name
		if !strings.HasPrefix(name, "Register") || !strings.HasSuffix(name, "Server") {
			return true
		}
		service := strings.TrimSuffix(strings.TrimPrefix(name, "Register"), "Server")
		if service != "" {
			seen[service]++
		}
		return true
	})
	if shadowed {
		return nil, fmt.Errorf("registry: selector %q is shadowed by a local identifier", selector)
	}
	out := make([]string, 0, len(seen))
	for service, count := range seen {
		if count != 1 {
			return nil, fmt.Errorf("registry: service %q registered %d times", service, count)
		}
		out = append(out, service)
	}
	sort.Strings(out)
	return out, nil
}

// AbsRegistry returns the registry path rooted at moduleRoot.
func (m *Manifest) AbsRegistry(moduleRoot string) string {
	return filepath.Join(moduleRoot, m.Registry.Path)
}
