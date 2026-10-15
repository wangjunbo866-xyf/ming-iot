package rpcgen

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/types/descriptorpb"
	"ming-iot/backend/tools/internal/clientcontract"
	"ming-iot/backend/tools/internal/protocompat"
)

// CompatibilitySnapshot freezes generated and hand-written compatibility
// surfaces that must survive a physical protobuf split.
type CompatibilitySnapshot struct {
	Descriptor      protocompat.Contract
	ClientAPI       clientcontract.Document
	Registrations   []string
	DirectMethods   []DirectMethod
	DirectFacades   []string
	ProtectedSHA256 map[string]string
}

// DirectMethod records one generated unary direct adapter.
type DirectMethod struct {
	Package   string
	Receiver  string
	Name      string
	CallsSVR  string
	Signature string
}

// CaptureCompatibility creates a compatibility snapshot from the current
// module tree using the pinned protobuf toolchain.
func CaptureCompatibility(moduleRoot string, m *Manifest, protobufToolsDir string) (CompatibilitySnapshot, error) {
	if err := ValidateManifest(m); err != nil {
		return CompatibilitySnapshot{}, err
	}
	if m.Version != 2 {
		return CompatibilitySnapshot{}, fmt.Errorf("manifest: compatibility capture requires manifest v2")
	}
	if err := ValidateManifestFiles(m, moduleRoot); err != nil {
		return CompatibilitySnapshot{}, err
	}
	set, err := GenerateDescriptorSet(m, moduleRoot, protobufToolsDir)
	if err != nil {
		return CompatibilitySnapshot{}, err
	}
	return captureCompatibilityFromSet(moduleRoot, m, set)
}

func captureCompatibilityFromSet(moduleRoot string, m *Manifest, set *descriptorpb.FileDescriptorSet) (CompatibilitySnapshot, error) {
	var snapshot CompatibilitySnapshot
	if err := CheckDescriptorSet(m, set); err != nil {
		return snapshot, err
	}
	contract, err := protocompat.Build(set, m.Package)
	if err != nil {
		return snapshot, fmt.Errorf("proto_graph: build compatibility contract: %w", err)
	}
	clientRoot := filepath.Join(moduleRoot, m.RuntimeDir, "client")
	clientAPI, err := clientcontract.ExtractWithSelector(clientRoot, m.PBSelector)
	if err != nil {
		return snapshot, fmt.Errorf("compatibility: capture client API: %w", err)
	}
	expectedImport := pathpkg.Join(m.ModulePath, filepath.ToSlash(m.PbDir))
	registrations, err := RegistryServices(m.AbsRegistry(moduleRoot), m.Registry.Selector, expectedImport)
	if err != nil {
		return snapshot, err
	}
	directMethods, err := captureDirectMethods(moduleRoot, m)
	if err != nil {
		return snapshot, err
	}
	directFacades, err := captureDirectFacades(filepath.Join(moduleRoot, m.RuntimeDir, "dmdirect"))
	if err != nil {
		return snapshot, err
	}
	protected, err := captureProtectedSHA256(moduleRoot, m.RuntimeDir)
	if err != nil {
		return snapshot, err
	}
	snapshot = CompatibilitySnapshot{
		Descriptor: contract, ClientAPI: clientAPI, Registrations: registrations,
		DirectMethods: directMethods, DirectFacades: directFacades, ProtectedSHA256: protected,
	}
	return snapshot, nil
}

func captureDirectMethods(moduleRoot string, m *Manifest) ([]DirectMethod, error) {
	var out []DirectMethod
	fset := token.NewFileSet()
	for _, service := range m.Services {
		path := filepath.Join(moduleRoot, m.RuntimeDir, "client", service.PackageDir, service.ClientFile)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("compatibility: parse direct client %s: %w", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			receiverVar, receiverType, ok := directReceiver(fn)
			if !ok || !strings.HasPrefix(receiverType, "direct") {
				continue
			}
			if fn.Body == nil || len(fn.Body.List) != 1 {
				return nil, fmt.Errorf("compatibility: direct method %s.%s must contain one return", receiverType, fn.Name.Name)
			}
			ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				return nil, fmt.Errorf("compatibility: direct method %s.%s must return one server call", receiverType, fn.Name.Name)
			}
			call, ok := ret.Results[0].(*ast.CallExpr)
			if !ok {
				return nil, fmt.Errorf("compatibility: direct method %s.%s does not return a call", receiverType, fn.Name.Name)
			}
			method, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || method.Sel.Name != fn.Name.Name {
				return nil, fmt.Errorf("compatibility: direct method %s.%s does not call the same server method", receiverType, fn.Name.Name)
			}
			server, ok := method.X.(*ast.SelectorExpr)
			if !ok || server.Sel.Name != "svr" {
				return nil, fmt.Errorf("compatibility: direct method %s.%s does not call receiver.svr", receiverType, fn.Name.Name)
			}
			owner, ok := server.X.(*ast.Ident)
			if !ok || owner.Name != receiverVar {
				return nil, fmt.Errorf("compatibility: direct method %s.%s calls another receiver", receiverType, fn.Name.Name)
			}
			parameters := functionParameterNames(fn)
			if len(parameters) < 2 || len(call.Args) != 2 || !identNamed(call.Args[0], parameters[0]) || !identNamed(call.Args[1], parameters[1]) {
				return nil, fmt.Errorf("compatibility: direct method %s.%s arguments must forward %s, %s", receiverType, fn.Name.Name, firstOr(parameters, 0), firstOr(parameters, 1))
			}
			out = append(out, DirectMethod{
				Package: file.Name.Name, Receiver: receiverType, Name: fn.Name.Name,
				CallsSVR: method.Sel.Name, Signature: renderDirectSignature(fset, fn),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.Package != right.Package {
			return left.Package < right.Package
		}
		if left.Receiver != right.Receiver {
			return left.Receiver < right.Receiver
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.CallsSVR != right.CallsSVR {
			return left.CallsSVR < right.CallsSVR
		}
		return left.Signature < right.Signature
	})
	return out, nil
}

func renderDirectSignature(fset *token.FileSet, fn *ast.FuncDecl) string {
	var buffer bytes.Buffer
	if fn == nil || fn.Type == nil || format.Node(&buffer, fset, fn.Type) != nil {
		return ""
	}
	return fn.Name.Name + strings.TrimPrefix(buffer.String(), "func")
}

func functionParameterNames(fn *ast.FuncDecl) []string {
	var names []string
	if fn.Type == nil || fn.Type.Params == nil {
		return names
	}
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}
	return names
}

func identNamed(expression ast.Expr, name string) bool {
	ident, ok := expression.(*ast.Ident)
	return ok && ident.Name == name
}

func firstOr(values []string, index int) string {
	if index < len(values) {
		return values[index]
	}
	return "<missing>"
}

func directReceiver(fn *ast.FuncDecl) (variable, receiverType string, ok bool) {
	if fn.Recv == nil || len(fn.Recv.List) != 1 || len(fn.Recv.List[0].Names) != 1 {
		return "", "", false
	}
	var ident *ast.Ident
	switch receiver := fn.Recv.List[0].Type.(type) {
	case *ast.StarExpr:
		ident, ok = receiver.X.(*ast.Ident)
	case *ast.Ident:
		ident, ok = receiver, true
	default:
		return "", "", false
	}
	if !ok {
		return "", "", false
	}
	return fn.Recv.List[0].Names[0].Name, ident.Name, true
}

func captureDirectFacades(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("compatibility: read direct facade directory %s: %w", root, err)
	}
	fset := token.NewFileSet()
	var out []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(root, entry.Name()), nil, 0)
		if err != nil {
			return nil, fmt.Errorf("compatibility: parse direct facade %s: %w", entry.Name(), err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name == nil || !fn.Name.IsExported() || !strings.HasPrefix(fn.Name.Name, "New") {
				continue
			}
			var buffer bytes.Buffer
			if err := format.Node(&buffer, fset, fn.Type); err != nil {
				return nil, fmt.Errorf("compatibility: render facade %s: %w", fn.Name.Name, err)
			}
			signature := strings.TrimPrefix(buffer.String(), "func")
			out = append(out, "func "+fn.Name.Name+signature)
		}
	}
	sort.Strings(out)
	return out, nil
}

func captureProtectedSHA256(moduleRoot, runtimeDir string) (map[string]string, error) {
	targets := []string{
		filepath.Join(runtimeDir, "proto", "schemaInfo.proto"),
		filepath.Join(runtimeDir, "dmdirect"),
		filepath.Join(runtimeDir, "etc"),
		filepath.Join(runtimeDir, "internal", "logic"),
		filepath.Join(runtimeDir, "internal", "repo"),
	}
	var files []string
	for _, relative := range targets {
		absolute := filepath.Join(moduleRoot, relative)
		linkInfo, err := os.Lstat(absolute)
		if err != nil {
			return nil, fmt.Errorf("compatibility: protected path %s: %w", relative, err)
		}
		if linkInfo.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("compatibility: protected path %s must not be a symlink", relative)
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return nil, fmt.Errorf("compatibility: protected path %s: %w", relative, err)
		}
		if info.Mode().IsRegular() {
			files = append(files, absolute)
			continue
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("compatibility: protected path %s is not a file or directory", relative)
		}
		if err := filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("protected path contains symlink: %s", path)
			}
			if entry.Type().IsRegular() {
				files = append(files, path)
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("compatibility: walk protected path %s: %w", relative, err)
		}
	}
	sort.Strings(files)
	digests := make(map[string]string, len(files))
	for _, path := range files {
		if err := requireManifestFile(moduleRoot, path); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("compatibility: read protected file %s: %w", path, err)
		}
		relative, err := filepath.Rel(moduleRoot, path)
		if err != nil {
			return nil, fmt.Errorf("compatibility: relative protected file %s: %w", path, err)
		}
		sum := sha256.Sum256(data)
		digests[filepath.ToSlash(relative)] = fmt.Sprintf("%x", sum[:])
	}
	return digests, nil
}

// CompareCompatibility requires exact descriptor/protected-tree equality and
// preserves every previously generated public symbol. Newly declared RPCs may
// add aliases, interface methods and direct methods; removing or changing an
// existing generated symbol remains incompatible.
func CompareCompatibility(before, after CompatibilitySnapshot) error {
	if differences := protocompat.Compare(before.Descriptor, after.Descriptor); len(differences) != 0 {
		return fmt.Errorf("compatibility: descriptor changed at %s", differences[0].Path)
	}
	if !clientAPIPreserves(before.ClientAPI, after.ClientAPI) {
		return fmt.Errorf("compatibility: client API changed (existing symbol removed or changed)")
	}
	if !reflect.DeepEqual(normalizeStrings(before.Registrations), normalizeStrings(after.Registrations)) {
		return fmt.Errorf("compatibility: registrations changed")
	}
	if !directMethodsPreserve(before.DirectMethods, after.DirectMethods) {
		return fmt.Errorf("compatibility: direct methods changed (existing method removed or changed)")
	}
	if !reflect.DeepEqual(normalizeStrings(before.DirectFacades), normalizeStrings(after.DirectFacades)) {
		return fmt.Errorf("compatibility: direct facades changed")
	}
	keys := make(map[string]bool, len(before.ProtectedSHA256)+len(after.ProtectedSHA256))
	for path := range before.ProtectedSHA256 {
		keys[path] = true
	}
	for path := range after.ProtectedSHA256 {
		keys[path] = true
	}
	paths := make([]string, 0, len(keys))
	for path := range keys {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if before.ProtectedSHA256[path] != after.ProtectedSHA256[path] {
			return fmt.Errorf("compatibility: protected file changed: %s", path)
		}
	}
	return nil
}

func clientAPIPreserves(before, after clientcontract.Document) bool {
	before = normalizeClientAPI(before)
	after = normalizeClientAPI(after)
	aliases := make(map[clientcontract.Alias]struct{}, len(after.Aliases))
	for _, item := range after.Aliases {
		aliases[item] = struct{}{}
	}
	for _, item := range before.Aliases {
		if _, ok := aliases[item]; !ok {
			return false
		}
	}
	functions := make(map[clientcontract.Function]struct{}, len(after.Functions))
	for _, item := range after.Functions {
		functions[item] = struct{}{}
	}
	for _, item := range before.Functions {
		if _, ok := functions[item]; !ok {
			return false
		}
	}
	interfaces := make(map[string]map[clientcontract.Method]struct{}, len(after.Interfaces))
	for _, item := range after.Interfaces {
		methods := make(map[clientcontract.Method]struct{}, len(item.Methods))
		for _, method := range item.Methods {
			methods[method] = struct{}{}
		}
		interfaces[item.Package+"\x00"+item.Name] = methods
	}
	for _, item := range before.Interfaces {
		methods, ok := interfaces[item.Package+"\x00"+item.Name]
		if !ok {
			return false
		}
		for _, method := range item.Methods {
			if _, ok := methods[method]; !ok {
				return false
			}
		}
	}
	return true
}

func directMethodsPreserve(before, after []DirectMethod) bool {
	available := make(map[DirectMethod]struct{}, len(after))
	for _, method := range after {
		available[method] = struct{}{}
	}
	for _, method := range before {
		if _, ok := available[method]; !ok {
			return false
		}
	}
	return true
}

// VerifyProtectedDigests requires an exact protected-tree match and reports
// the first stable repo-relative path that changed.
func VerifyProtectedDigests(before, after map[string]string) error {
	paths := make(map[string]bool, len(before)+len(after))
	for path := range before {
		paths[path] = true
	}
	for path := range after {
		paths[path] = true
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		if before[path] != after[path] {
			return fmt.Errorf("protected file changed: %s", path)
		}
	}
	return nil
}

// RejectForbiddenImports scans Go import declarations under root. It rejects
// legacy module prefixes without matching comments or arbitrary string data.
func RejectForbiddenImports(root string, forbidden []string) error {
	return filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), filePath, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse imports %s: %w", relativePath(root, filePath), err)
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("parse import %s in %s: %w", spec.Path.Value, relativePath(root, filePath), err)
			}
			for _, prefix := range forbidden {
				if importPath == prefix || strings.HasPrefix(importPath, strings.TrimSuffix(prefix, "/")+"/") {
					return fmt.Errorf("forbidden import %q in %s", importPath, relativePath(root, filePath))
				}
			}
		}
		return nil
	})
}

func legacyShareImportPath() string {
	return strings.Join([]string{"gitee.com", "unitedrhino", "share"}, "/")
}

func relativePath(root, filePath string) string {
	relative, err := filepath.Rel(root, filePath)
	if err != nil {
		return filepath.ToSlash(filePath)
	}
	return filepath.ToSlash(relative)
}

func normalizeStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func normalizeDirectMethods(values []DirectMethod) []DirectMethod {
	result := append([]DirectMethod(nil), values...)
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.Package != right.Package {
			return left.Package < right.Package
		}
		if left.Receiver != right.Receiver {
			return left.Receiver < right.Receiver
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.CallsSVR != right.CallsSVR {
			return left.CallsSVR < right.CallsSVR
		}
		return left.Signature < right.Signature
	})
	return result
}

func normalizeClientAPI(document clientcontract.Document) clientcontract.Document {
	document.Aliases = append([]clientcontract.Alias(nil), document.Aliases...)
	document.Interfaces = append([]clientcontract.Interface(nil), document.Interfaces...)
	document.Functions = append([]clientcontract.Function(nil), document.Functions...)
	sort.Slice(document.Aliases, func(i, j int) bool {
		left, right := document.Aliases[i], document.Aliases[j]
		if left.Package != right.Package {
			return left.Package < right.Package
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.Target < right.Target
	})
	for i := range document.Interfaces {
		document.Interfaces[i].Methods = append([]clientcontract.Method(nil), document.Interfaces[i].Methods...)
		sort.Slice(document.Interfaces[i].Methods, func(a, b int) bool {
			left, right := document.Interfaces[i].Methods[a], document.Interfaces[i].Methods[b]
			if left.Name != right.Name {
				return left.Name < right.Name
			}
			return left.Signature < right.Signature
		})
	}
	sort.Slice(document.Interfaces, func(i, j int) bool {
		left, right := document.Interfaces[i], document.Interfaces[j]
		if left.Package != right.Package {
			return left.Package < right.Package
		}
		return left.Name < right.Name
	})
	sort.Slice(document.Functions, func(i, j int) bool {
		left, right := document.Functions[i], document.Functions[j]
		if left.Package != right.Package {
			return left.Package < right.Package
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.Signature < right.Signature
	})
	return document
}
