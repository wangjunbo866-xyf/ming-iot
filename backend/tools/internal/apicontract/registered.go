package apicontract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// RegisteredRoutes 解析服务实际注册的 REST 路由（internal/handler/routes.go）。
//
// routes.go 由 goctl 生成，结构固定：
//
//	server.AddRoutes(
//	    rest.WithMiddlewares(
//	        []rest.Middleware{serverCtx.A, serverCtx.B},
//	        []rest.Route{ {Method:.., Path:.., Handler: pkg.FnHandler(svc)}, ... }...,
//	    ),
//	    rest.WithPrefix("/api/v1/..."),
//	)
//
// 本函数按 AddRoutes 调用块收集 prefix、middleware 名与 route 的 method/path/
// handler，把 handler 函数名（如 CreateHandler）还原为 @handler 风格（create）。
// 仅解析 routes.go，不把 wsRoutes.go 计入 REST 注册。
func RegisteredRoutes(b Backend) ([]Route, error) {
	src, err := os.ReadFile(b.RoutesGo)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, b.RoutesGo, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", b.RoutesGo, err)
	}

	var out []Route
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		for _, stmt := range fd.Body.List {
			es, ok := stmt.(*ast.ExprStmt)
			if !ok {
				continue
			}
			call, ok := es.X.(*ast.CallExpr)
			if !ok || !isAddRoutes(call) {
				continue
			}
			prefix, mws := prefixAndMiddlewares(call)
			rfs := routeFieldsFromCall(call)
			for _, rf := range rfs {
				out = append(out, Route{
					Service:     b.Name,
					Method:      rf.Method,
					FullPath:    joinPath(prefix, rf.Path),
					Handler:     rf.Handler,
					Middlewares: append([]string(nil), mws...),
				})
			}
		}
	}
	return out, nil
}

// routeCollector 解析时复用 Route，但保留临时 Path（route 内相对路径）。
type routeFields struct {
	Method  string
	Path    string
	Handler string
}

// isAddRoutes 判断 call 是否为 server.AddRoutes(...)。
func isAddRoutes(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "AddRoutes"
}

// prefixAndMiddlewares 从 AddRoutes 实参里提取 WithPrefix 与 WithMiddlewares。
func prefixAndMiddlewares(call *ast.CallExpr) (string, []string) {
	prefix := ""
	var mws []string
	for _, arg := range call.Args {
		ce, ok := arg.(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := ce.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		switch sel.Sel.Name {
		case "WithPrefix":
			if len(ce.Args) > 0 {
				prefix = litString(ce.Args[0])
			}
		case "WithMiddlewares":
			mws = middlewareNames(ce.Args[0])
		}
	}
	return prefix, mws
}

// routeFieldsFromCall 从 WithMiddlewares 的第二个实参（[]rest.Route{...}）或
// AddRoutes 直接的 rest.WithRoutes 实参里提取路由项。
func routeFieldsFromCall(call *ast.CallExpr) []routeFields {
	var routeComp *ast.CompositeLit
	for _, arg := range call.Args {
		ce, ok := arg.(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := ce.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		if sel.Sel.Name == "WithMiddlewares" && len(ce.Args) >= 2 {
			if cl, ok := ce.Args[1].(*ast.CompositeLit); ok {
				routeComp = cl
			}
		}
	}
	if routeComp == nil {
		return nil
	}
	var out []routeFields
	for _, el := range routeComp.Elts {
		cl, ok := el.(*ast.CompositeLit)
		if !ok {
			continue
		}
		rf := routeFields{}
		for _, kv := range cl.Elts {
			pair, ok := kv.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := pair.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch key.Name {
			case "Method":
				rf.Method = httpMethodConst(pair.Value)
			case "Path":
				rf.Path = litString(pair.Value)
			case "Handler":
				rf.Handler = handlerName(pair.Value)
			}
		}
		out = append(out, rf)
	}
	return out
}

// middlewareNames 解析 []rest.Middleware{serverCtx.A, serverCtx.B} 提取 A/B。
func middlewareNames(e ast.Expr) []string {
	cl, ok := e.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var out []string
	for _, el := range cl.Elts {
		sel, ok := el.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		out = append(out, sel.Sel.Name)
	}
	return out
}

// litString 提取字符串字面量值。
func litString(e ast.Expr) string {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	return strings.Trim(lit.Value, `"`)
}

// httpMethodConst 把 http.MethodPost 还原为 POST。
func httpMethodConst(e ast.Expr) string {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	return strings.ToUpper(strings.TrimPrefix(sel.Sel.Name, "Method"))
}

// handlerName 把 pkg.FnHandler(svc) 调用还原为 @handler 名。
// goctl 生成规则：@handler X -> UpperCamel(X) + "Handler"；这里逆向：
// 去掉 "Handler" 后缀并转 lowerCamel。无法确定时返回原函数名小写首字母。
func handlerName(e ast.Expr) string {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	fn := sel.Sel.Name
	fn = strings.TrimSuffix(fn, "Handler")
	return lowerCamel(fn)
}

// lowerCamel 把 PascalCase 转 lowerCamel：仅小写首字母段。
// 例如 Create -> create, MultiUpdate -> multiUpdate, MulitUpdate -> mulitUpdate,
// Index -> index。对已是 lowerCamel 的输入（少见）原样返回。
func lowerCamel(s string) string {
	if s == "" {
		return ""
	}
	// 处理连续大写首字母（如 IDList -> idList）的最小行为：仅小写第一个 rune。
	r := []rune(s)
	r[0] = lowerRune(r[0])
	return string(r)
}

func lowerRune(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}
