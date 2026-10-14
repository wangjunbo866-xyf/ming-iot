package clientcontract

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFixture writes a Go file under root/<pkg>/<file>.go, creating the pkg
// directory (and a minimal go.mod) so it looks like a real client package.
func writeFixture(t *testing.T, root, pkg, filename, src string) {
	t.Helper()
	dir := filepath.Join(root, pkg)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func parseDir(t *testing.T, dir string) (*token.FileSet, []*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	var fs []*ast.File
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			fs = append(fs, f)
		}
	}
	return fset, fs
}

func TestExtractBracketTypeBlock(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := `package ops

import "sys"

type (
	AccessInfo        = sys.AccessInfo
	Empty             = sys.Empty
	OpsWorkOrder      = sys.OpsWorkOrder
	defaultOps        struct { cli int }
	directOps         struct { svr int }
)

type Ops interface {
	OpsWorkOrderCreate(ctx int, in *OpsWorkOrder) (*WithID, error)
}

func NewOps(cli int) Ops { return nil }
func NewDirectOps(svcCtx int, svr int) Ops { return nil }
`
	writeFixture(t, root, "ops", "ops.go", src)
	doc, err := Extract(root)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	// 3 public aliases inside the bracket block; structs are private and
	// excluded.
	wantNames := map[string]bool{"AccessInfo": true, "Empty": true, "OpsWorkOrder": true}
	if len(doc.Aliases) != 3 {
		t.Fatalf("aliases = %d, want 3: %+v", len(doc.Aliases), doc.Aliases)
	}
	for _, a := range doc.Aliases {
		if a.Package != "ops" {
			t.Errorf("alias %q package = %q, want ops", a.Name, a.Package)
		}
		if !wantNames[a.Name] {
			t.Errorf("unexpected alias %q", a.Name)
		}
		if !strings.HasPrefix(a.Target, "sys.") {
			t.Errorf("alias %q target = %q, want sys.* selector", a.Name, a.Target)
		}
	}
	if len(doc.Interfaces) != 1 || doc.Interfaces[0].Name != "Ops" {
		t.Fatalf("interfaces = %+v, want [Ops]", doc.Interfaces)
	}
	if len(doc.Functions) != 2 {
		t.Fatalf("functions = %d, want 2 (NewOps, NewDirectOps)", len(doc.Functions))
	}
}

func TestExtractSingleLineTypeAlias(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := `package app

import "sys"

type AppInfo = sys.AppInfo
type internalThing = sys.AppInfo
`
	writeFixture(t, root, "app", "app.go", src)
	doc, err := Extract(root)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(doc.Aliases) != 1 || doc.Aliases[0].Name != "AppInfo" {
		t.Fatalf("aliases = %+v, want [AppInfo]", doc.Aliases)
	}
}

func TestExtractIgnoresNonSysSelector(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := `package log

import (
	"sys"
	"other"
)

type A = sys.LoginLogInfo
type B = other.Thing
type C = int
`
	writeFixture(t, root, "log", "log.go", src)
	doc, err := Extract(root)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(doc.Aliases) != 1 || doc.Aliases[0].Name != "A" {
		t.Fatalf("aliases = %+v, want only the sys.* alias [A]", doc.Aliases)
	}
}

func TestExtractWithSelectorCapturesOnlySelectedPBAliases(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := `package client

import (
	"dm"
	"sys"
	"other"
)

type A = dm.A
type B = sys.B
type C = other.C
`
	writeFixture(t, root, "client", "client.go", src)
	doc, err := ExtractWithSelector(root, "dm")
	if err != nil {
		t.Fatalf("ExtractWithSelector: %v", err)
	}
	if len(doc.Aliases) != 1 {
		t.Fatalf("aliases = %+v, want only dm.A", doc.Aliases)
	}
	if got := doc.Aliases[0]; got.Package != "client" || got.Name != "A" || got.Target != "dm.A" {
		t.Fatalf("alias = %+v, want client.A = dm.A", got)
	}
}

func TestExtractWithSelectorRejectsInvalidSelector(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, selector := range []string{"", "pb/dm", "dm.x", "9dm"} {
		t.Run(selector, func(t *testing.T) {
			if _, err := ExtractWithSelector(root, selector); err == nil {
				t.Fatalf("ExtractWithSelector(%q) returned nil error", selector)
			}
		})
	}
}

func TestExtractInterfaceSignatures(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := `package usermanage

type UserManage interface {
	UserLogin(ctx int, in *UserLoginReq) (*UserLoginResp, error)
	UserLogout(ctx int, in *UserLogoutReq) (*Empty, error)
}
`
	writeFixture(t, root, "usermanage", "usermanage.go", src)
	doc, err := Extract(root)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(doc.Interfaces) != 1 {
		t.Fatalf("want 1 interface, got %+v", doc.Interfaces)
	}
	iface := doc.Interfaces[0]
	if len(iface.Methods) != 2 {
		t.Fatalf("methods = %d, want 2", len(iface.Methods))
	}
	// Methods preserve name + full signature text.
	if iface.Methods[0].Name != "UserLogin" {
		t.Errorf("first method = %q, want UserLogin", iface.Methods[0].Name)
	}
	if !strings.Contains(iface.Methods[0].Signature, "UserLoginReq") {
		t.Errorf("UserLogin signature missing param: %q", iface.Methods[0].Signature)
	}
}

func TestExtractConstructors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := `package ops

type Ops interface {}

func NewOps(cli zrpc.Client) Ops { return nil }
func NewDirectOps(svcCtx *svc.ServiceContext, svr sys.OpsServer) Ops { return nil }
func helper() int { return 0 }
`
	writeFixture(t, root, "ops", "ops.go", src)
	doc, err := Extract(root)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	// Only New* exported functions are captured; helper is not.
	if len(doc.Functions) != 2 {
		t.Fatalf("functions = %+v, want 2", doc.Functions)
	}
	names := map[string]bool{}
	for _, f := range doc.Functions {
		names[f.Name] = true
		if !strings.HasPrefix(f.Name, "New") {
			t.Errorf("non-New function captured: %q", f.Name)
		}
	}
	if !names["NewOps"] || !names["NewDirectOps"] {
		t.Errorf("missing constructors; got %v", names)
	}
}

func TestDocumentJSONRoundTrip(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := `package ops

import "sys"

type (
	Empty = sys.Empty
)

type Ops interface {
	Ping(ctx int) (*Empty, error)
}

func NewOps(cli int) Ops { return nil }
`
	writeFixture(t, root, "ops", "ops.go", src)
	doc, err := Extract(root)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Document
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Aliases) != len(doc.Aliases) || len(back.Interfaces) != len(doc.Interfaces) || len(back.Functions) != len(doc.Functions) {
		t.Fatalf("round trip mismatch: %+v vs %+v", back, doc)
	}
}
