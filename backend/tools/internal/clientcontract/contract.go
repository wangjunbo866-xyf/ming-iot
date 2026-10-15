// Package clientcontract extracts the public Go API surface (type aliases to
// the sys package, exported interfaces, and New* constructors) from a set of
// generated client packages, producing a deterministic JSON document that can
// be snapshotted and verified.
//
// The extractor is purely AST-based: it reads source files without compiling
// them, so it works on any tree that parses. It ignores private structs and
// method bodies, and only captures aliases whose target is a selected package
// selector (the public re-export shape goctl emits).
package clientcontract

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Document is the public API surface of a client root, JSON-serializable.
type Document struct {
	Aliases    []Alias     `json:"aliases"`
	Interfaces []Interface `json:"interfaces"`
	Functions  []Function  `json:"functions"`
}

// Alias is a public type alias re-exporting a sys.* type, e.g.
// `type Empty = sys.Empty`.
type Alias struct {
	Package string `json:"package"`
	Name    string `json:"name"`
	Target  string `json:"target"`
}

// Method is one method of an exported interface.
type Method struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
}

// Interface is an exported client interface with its exported methods.
type Interface struct {
	Package string   `json:"package"`
	Name    string   `json:"name"`
	Methods []Method `json:"methods"`
}

// Function is an exported constructor (New*/NewDirect*) with its full
// signature text.
type Function struct {
	Package   string `json:"package"`
	Name      string `json:"name"`
	Signature string `json:"signature"`
}

// Extract walks root one level deep, treating each subdirectory as a client
// package, and returns the normalized public API document.
func Extract(root string) (Document, error) {
	return ExtractWithSelector(root, "sys")
}

// ExtractWithSelector walks root one level deep and captures aliases targeting
// selector, treating each subdirectory as a client package.
func ExtractWithSelector(root, selector string) (Document, error) {
	doc := Document{Aliases: []Alias{}, Interfaces: []Interface{}, Functions: []Function{}}
	if selector == "_" || !token.IsIdentifier(selector) {
		return doc, fmt.Errorf("invalid selector %q", selector)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return doc, fmt.Errorf("read dir %s: %w", root, err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pkgName, files, err := parsePackage(fset, filepath.Join(root, e.Name()))
		if err != nil || len(files) == 0 {
			// Skip directories that are not a parseable Go package.
			continue
		}
		extractFromPackage(&doc, pkgName, files, fset, selector)
	}
	sortDocument(&doc)
	return doc, nil
}

// parsePackage parses all .go files (excluding _test.go) in dir and returns
// the declared package name plus the parsed files.
func parsePackage(fset *token.FileSet, dir string) (string, []*ast.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, err
	}
	var files []*ast.File
	var pkgName string
	for _, fe := range entries {
		if fe.IsDir() || !strings.HasSuffix(fe.Name(), ".go") || strings.HasSuffix(fe.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, fe.Name()), nil, parser.ParseComments)
		if err != nil {
			return "", nil, err
		}
		files = append(files, f)
		if pkgName == "" {
			pkgName = f.Name.Name
		}
	}
	return pkgName, files, nil
}

func extractFromPackage(doc *Document, pkgName string, files []*ast.File, fset *token.FileSet, selector string) {
	for _, file := range files {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				// function decls handled below
				if fd, ok := decl.(*ast.FuncDecl); ok {
					if fd.Recv == nil && fd.Name != nil && fd.Name.IsExported() && strings.HasPrefix(fd.Name.Name, "New") {
						doc.Functions = append(doc.Functions, Function{
							Package:   pkgName,
							Name:      fd.Name.Name,
							Signature: renderFuncSignature(fset, fd),
						})
					}
				}
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Name == nil || !ts.Name.IsExported() {
					continue
				}
				switch t := ts.Type.(type) {
				case *ast.Ident:
					// `type X = Y` where Y is a plain ident: not a sys.*
					// selector; skip.
					_ = t
				case *ast.SelectorExpr:
					// `type X = pkg.Name` - only capture selected package re-exports.
					if sel := selectorString(t); strings.HasPrefix(sel, selector+".") {
						doc.Aliases = append(doc.Aliases, Alias{
							Package: pkgName,
							Name:    ts.Name.Name,
							Target:  sel,
						})
					}
				case *ast.InterfaceType:
					iface := Interface{Package: pkgName, Name: ts.Name.Name}
					iface.Methods = extractInterfaceMethods(fset, t)
					doc.Interfaces = append(doc.Interfaces, iface)
				}
			}
		}
	}
}

// selectorString renders a SelectorExpr as "pkg.Name".
func selectorString(e *ast.SelectorExpr) string {
	if e == nil {
		return ""
	}
	var pkg string
	switch x := e.X.(type) {
	case *ast.Ident:
		pkg = x.Name
	case *ast.SelectorExpr:
		pkg = selectorString(x)
	default:
		return ""
	}
	return pkg + "." + e.Sel.Name
}

// extractInterfaceMethods returns the exported methods of an interface type,
// each with its full method signature text. Embedded interfaces (no name) are
// represented as a single method whose signature is the embedded type name.
func extractInterfaceMethods(fset *token.FileSet, it *ast.InterfaceType) []Method {
	if it == nil || it.Methods == nil {
		return nil
	}
	var methods []Method
	for _, field := range it.Methods.List {
		// Embedded interface: field.Names is empty.
		if len(field.Names) == 0 {
			continue
		}
		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			methods = append(methods, Method{
				Name:      name.Name,
				Signature: renderFieldSignature(fset, field),
			})
		}
	}
	return methods
}

// renderFuncSignature renders "func Name(params) (results)" from a FuncDecl,
// including the body-free signature only.
func renderFuncSignature(fset *token.FileSet, fd *ast.FuncDecl) string {
	var b bytes.Buffer
	// Build a synthetic signature string from the func type. We render the
	// FuncType (params + results) and prefix with "func Name".
	if err := format.Node(&b, fset, fd.Type); err != nil {
		return fd.Name.Name
	}
	return "func " + fd.Name.Name + b.String()
}

// renderFieldSignature renders a single interface-method signature, e.g.
// "UserLogin(ctx context.Context, in *UserLoginReq) (*UserLoginResp, error)".
func renderFieldSignature(fset *token.FileSet, field *ast.Field) string {
	// field.Type is a FuncType for interface methods.
	ft, ok := field.Type.(*ast.FuncType)
	if !ok {
		// Non-method embedded; render the type instead.
		var b bytes.Buffer
		if err := format.Node(&b, fset, field.Type); err != nil {
			return ""
		}
		return strings.TrimSpace(b.String())
	}
	var b bytes.Buffer
	if err := format.Node(&b, fset, ft); err != nil {
		return ""
	}
	// Render method name + signature. For multiple names share one type we
	// already split per name by the caller; here use the first name.
	name := ""
	if len(field.Names) > 0 {
		name = field.Names[0].Name
	}
	return name + b.String()
}

func sortDocument(doc *Document) {
	sort.Slice(doc.Aliases, func(i, j int) bool {
		if doc.Aliases[i].Package != doc.Aliases[j].Package {
			return doc.Aliases[i].Package < doc.Aliases[j].Package
		}
		if doc.Aliases[i].Name != doc.Aliases[j].Name {
			return doc.Aliases[i].Name < doc.Aliases[j].Name
		}
		return doc.Aliases[i].Target < doc.Aliases[j].Target
	})
	sort.Slice(doc.Interfaces, func(i, j int) bool {
		if doc.Interfaces[i].Package != doc.Interfaces[j].Package {
			return doc.Interfaces[i].Package < doc.Interfaces[j].Package
		}
		return doc.Interfaces[i].Name < doc.Interfaces[j].Name
	})
	sort.Slice(doc.Functions, func(i, j int) bool {
		if doc.Functions[i].Package != doc.Functions[j].Package {
			return doc.Functions[i].Package < doc.Functions[j].Package
		}
		return doc.Functions[i].Name < doc.Functions[j].Name
	})
}
