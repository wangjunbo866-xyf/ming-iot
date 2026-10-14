package apicontract

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	goparser "github.com/zeromicro/go-zero/tools/goctl/api/parser"
	"github.com/zeromicro/go-zero/tools/goctl/api/spec"
)

// Backend 描述单个 HTTP 服务的入口与运行时注册位置。
type Backend struct {
	Name        string   // apisvr / datasvr
	ServiceDir  string   // service/apisvr/http
	EntryAPI    string   // service/apisvr/http/api.api
	RoutesGo    string   // service/apisvr/internal/handler/routes.go
	TypesGo     string   // service/apisvr/internal/types/types.go
	ExtraRoutes []string // 额外的 REST 注册文件（如 datasvr 的 wsRoutes.go 不应计入，留空）
}

// Backends 返回 coreRoot 下实际存在的 HTTP 服务入口配置。SaaS 同时包含
// apisvr/datasvr，Things 当前只包含 apisvr；不存在的服务不得被虚构出来。
func Backends(coreRoot string) map[string]Backend {
	candidates := map[string]Backend{
		"apisvr": {
			Name:       "apisvr",
			ServiceDir: filepath.Join(coreRoot, "service/apisvr/http"),
			EntryAPI:   filepath.Join(coreRoot, "service/apisvr/http/api.api"),
			RoutesGo:   filepath.Join(coreRoot, "service/apisvr/internal/handler/routes.go"),
			TypesGo:    filepath.Join(coreRoot, "service/apisvr/internal/types/types.go"),
		},
		"datasvr": {
			Name:       "datasvr",
			ServiceDir: filepath.Join(coreRoot, "service/datasvr/http"),
			EntryAPI:   filepath.Join(coreRoot, "service/datasvr/http/data.api"),
			RoutesGo:   filepath.Join(coreRoot, "service/datasvr/internal/handler/routes.go"),
			TypesGo:    filepath.Join(coreRoot, "service/datasvr/internal/types/types.go"),
		},
	}
	out := make(map[string]Backend, len(candidates))
	for name, backend := range candidates {
		if dirExists(backend.ServiceDir) {
			out[name] = backend
		}
	}
	return out
}

// LoadBackend 解析 backendRoot 下全部 HTTP 服务的契约快照，按服务名返回。
// backendRoot 既可指向直接包含 service/ 的目录（backend/saas），也可指向
// 仓库的 backend 目录（此时自动识别 saas 子目录）。这样 tools 目录里执行
// `apicheck verify --backend ..` 与 `--backend ../core` 都能工作。
func LoadBackend(backendRoot string) (map[string]Snapshot, error) {
	if backendRoot == "" {
		return nil, errors.New("backend root is required")
	}
	st, err := os.Stat(backendRoot)
	if err != nil {
		return nil, fmt.Errorf("backend root: %w", err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("backend root %q is not a directory", backendRoot)
	}

	coreRoot := ResolveCoreRoot(backendRoot)
	out := map[string]Snapshot{}
	for _, b := range Backends(coreRoot) {
		snap, err := LoadService(b)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", b.Name, err)
		}
		out[b.Name] = snap
	}
	return out, nil
}

// ResolveCoreRoot 在 root 下定位真正包含 service/ 的目录。
// root 既可指向直接包含 service/ 的目录（backend/saas），也可指向仓库的
// backend 目录（此时识别 saas 子目录）。导出供 CLI 决定 golden 写入位置。
func ResolveCoreRoot(root string) string {
	if dirExists(filepath.Join(root, "service", "apisvr")) {
		return root
	}
	if dirExists(filepath.Join(root, "saas", "service", "apisvr")) {
		return filepath.Join(root, "saas")
	}
	return root
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// LoadService 解析单个服务的完整 Snapshot。
func LoadService(b Backend) (Snapshot, error) {
	// 1. Reachable：入口 import graph 全量解析（类型已解析）。
	entrySpec, err := goparser.Parse(b.EntryAPI)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse entry %s: %w", b.EntryAPI, err)
	}
	reachable := routesFromSpec(b.Name, entrySpec)
	sortRoutes(reachable)

	// 2. Defined：Reachable ∪ Detached。先按 import graph 求出可达文件集合，
	//    再扫描游离文件（声明了 service 但不在可达图中）。
	reachableFiles := reachableImportFiles(b.EntryAPI)
	reachableSet := map[string]bool{}
	for _, r := range reachable {
		reachableSet[routeKey(r)] = true
	}
	detached, err := detachedRoutes(b, reachableSet, reachableFiles)
	if err != nil {
		return Snapshot{}, err
	}
	defined := append([]Route{}, reachable...)
	defined = append(defined, detached...)
	sortRoutes(defined)

	// 3. Registered：解析 routes.go。
	registered, err := RegisteredRoutes(b)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse routes.go: %w", err)
	}
	sortRoutes(registered)

	// 4. Authorization：对 Defined 推导授权决策（按访问生成器算法）。
	auth := authorizations(defined, entrySpec, b.Name)
	sortAuth(auth)

	// 5. Types：入口 import graph 对外 request/response 及递归可达类型。
	types, undefined := typeContracts(entrySpec, b.TypesGo)

	return Snapshot{
		Defined:        defined,
		Reachable:      reachable,
		Registered:     registered,
		Detached:       detached,
		Authorization:  auth,
		Types:          types,
		UndefinedTypes: undefined,
	}, nil
}

// routesFromSpec 把 goctl spec 的 Group/Route 扁平化为 Route，补齐 FullPath
// 与 Method 大小写，并归一化 @server prefix 的前导 '/'。
func routesFromSpec(service string, sp *spec.ApiSpec) []Route {
	var out []Route
	for _, g := range sp.Service.Groups {
		prefix := normalizePrefix(g.Annotation.Properties["prefix"])
		mws := middlewares(g.Annotation.Properties["middleware"])
		for _, r := range g.Routes {
			out = append(out, Route{
				Service:      service,
				Method:       strings.ToUpper(r.Method),
				FullPath:     joinPath(prefix, r.Path),
				Handler:      r.Handler,
				RequestType:  typeName(r.RequestType),
				ResponseType: typeName(r.ResponseType),
				Middlewares:  mws,
			})
		}
	}
	return out
}

// middlewares 解析 @server middleware 字段为有序、去重的中间件名列表。
// middleware 字段形如 "CheckTokenWare,InitCtxsWare"。
func middlewares(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var out []string
	seen := map[string]bool{}
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// normalizePrefix 保证 prefix 以单个 '/' 开头；空 prefix 返回空串。
func normalizePrefix(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// joinPath 拼接 prefix 与 route path，避免重复 '/'。
func joinPath(prefix, path string) string {
	prefix = normalizePrefix(prefix)
	path = strings.TrimSpace(path)
	if path == "" {
		return prefix
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// 单一前导 '/'，不合并后续段（保持 /api/v1/.. 与原注册一致）。
	return prefix + path
}

// routeKey 是路由在去重时的稳定键（method+fullPath+handler）。
func routeKey(r Route) string {
	return r.Method + " " + r.FullPath + " " + r.Handler
}

// detachedRoutes 扫描服务目录全部 .api，找到声明了 service 块但不在可达
// import graph 中的游离文件，解析它们并返回其路由。这些文件（如 system/oss.api）
// 未被入口 import graph 引用，因此类型解析依赖文件自身的依赖；遇到解析错误
// 时按游离处理并报错。reachableFiles 是入口 import graph 覆盖的绝对路径集合。
func detachedRoutes(b Backend, reachableRouteKeys map[string]bool, reachableFiles map[string]bool) ([]Route, error) {
	var files []string
	err := filepath.Walk(b.ServiceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".api") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	var out []Route
	for _, f := range files {
		if reachableFiles[filepath.Clean(f)] {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		// 仅处理声明了 service 块的文件（纯聚合/type 文件无路由）。
		if !declaresService(string(data)) {
			continue
		}
		sp, err := goparser.Parse(f)
		if err != nil {
			// 游离文件若依赖跨文件类型会解析失败；尝试相对其目录解析仍失败则报错。
			return nil, fmt.Errorf("parse detached file %s: %w", f, err)
		}
		for _, r := range routesFromSpec(b.Name, sp) {
			if !reachableRouteKeys[routeKey(r)] {
				out = append(out, r)
			}
		}
	}
	sortRoutes(out)
	return out, nil
}

// importLineRe 匹配 .api 里的 import 语句，支持单行与多行块两种形式。
var importLineRe = regexp.MustCompile(`import\s+"([^"]+)"`)

// importBlockItemRe 匹配多行 import 块里的单独一行路径。
var importBlockItemRe = regexp.MustCompile(`^\s*"([^"]+)"`)

// reachableImportFiles 从入口 .api 出发，BFS 遍历 import 语句，返回全部可达
// .api 文件的绝对路径集合。goctl parser.Parse 已递归解析 import，但这里需要
// 文件级可达集合来识别游离文件，故独立遍历。
func reachableImportFiles(entry string) map[string]bool {
	abs, err := filepath.Abs(entry)
	if err != nil {
		abs = entry
	}
	out := map[string]bool{}
	var queue []string
	queue = append(queue, abs)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		cur = filepath.Clean(cur)
		if out[cur] {
			continue
		}
		out[cur] = true
		for _, imp := range apiImports(cur) {
			imp = filepath.Clean(imp)
			if !out[imp] {
				queue = append(queue, imp)
			}
		}
	}
	return out
}

// apiImports 读取一个 .api 文件，返回它直接 import 的 .api 绝对路径。
// import 路径相对于该文件所在目录。
func apiImports(file string) []string {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	dir := filepath.Dir(file)
	content := string(data)
	var out []string
	seen := map[string]bool{}

	// 单行：import "x.api"
	for _, m := range importLineRe.FindAllStringSubmatch(content, -1) {
		p := filepath.Join(dir, m[1])
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	// 多行块项：	"x.api"  // comment
	for _, line := range strings.Split(content, "\n") {
		if m := importBlockItemRe.FindStringSubmatch(line); m != nil {
			// 跳过已被单行形式捕获的（带 import 前缀的行不会匹配纯缩进项）。
			if strings.Contains(line, "import") {
				continue
			}
			p := filepath.Join(dir, m[1])
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

var serviceDeclRe = regexp.MustCompile(`(?m)^\s*service\s+(api|data)\b`)

// declaresService 粗判文件是否声明 service 块，避免逐文件完整解析开销。
func declaresService(content string) bool {
	return serviceDeclRe.MatchString(content)
}

// authorizations 按访问生成器算法推导每条 Defined 路由的授权决策。
//
// 算法（与 goctl/api/access/generate/parser.go 对齐，但使用 parser 已剥引号
// 的属性值，避免 strconv.Unquote 失败导致回退到默认值）：
//   - EffectiveAuthType = route @doc authType → server defaultAuthType → "all"
//   - EffectiveNeedAuth = route @doc isNeedAuth（true/false）→ server
//     defaultNeedAuth → false
//   - EffectiveOperType = route @doc operType → GetBusinessType(fullPath)
//   - ManagedByAccess   = 所属 @server 块带 accessGroup
//
// Detached 路由（不在 entrySpec 中）按其自身文件解析推导。
func authorizations(defined []Route, entrySpec *spec.ApiSpec, service string) []Authorization {
	// 建立路由键 → @server properties / route @doc 的查询表。
	//
	// 关键：路由必须按它所属的 @server group 关联，而不是通过 prefix 反查。
	// 多个 group 可能共享同一 prefix（例如 system/user/self 下既有带 accessGroup
	// 的鉴权块，也有只发验证码的 public 块），按 prefix 反查会被后写块覆盖或被
	// 最长前缀误匹配，导致 ManagedByAccess/effectiveAuthType 漂移。goctl access
	// 生成器是按 group 逐块处理的，这里对齐它的语义。
	type groupCtx struct {
		props   map[string]string
		managed bool
	}
	ctxByRoute := map[string]groupCtx{} // method+fullPath -> 该路由所属 group 的属性
	docByRoute := map[string]map[string]string{}
	for _, g := range entrySpec.Service.Groups {
		prefix := normalizePrefix(g.Annotation.Properties["prefix"])
		_, managed := g.Annotation.Properties["accessGroup"]
		gc := groupCtx{props: g.Annotation.Properties, managed: managed}
		for _, r := range g.Routes {
			fp := joinPath(prefix, r.Path)
			key := strings.ToUpper(r.Method) + " " + fp
			ctxByRoute[key] = gc
			docByRoute[key] = r.AtDoc.Properties
		}
	}

	var out []Authorization
	for _, r := range defined {
		var doc map[string]string
		var props map[string]string
		var managed bool
		if d, ok := docByRoute[r.Method+" "+r.FullPath]; ok {
			doc = d
		}
		if gc, ok := ctxByRoute[r.Method+" "+r.FullPath]; ok {
			props = gc.props
			managed = gc.managed
		}
		if doc == nil {
			doc = map[string]string{}
		}
		out = append(out, Authorization{
			Method:            r.Method,
			FullPath:          r.FullPath,
			EffectiveAuthType: effectiveAuthType(doc, props),
			EffectiveNeedAuth: effectiveNeedAuth(doc, props),
			EffectiveOperType: GetBusinessType(r.FullPath, doc["operType"]),
			ManagedByAccess:   managed,
		})
	}
	return out
}

func effectiveAuthType(doc, server map[string]string) string {
	if v := strings.TrimSpace(doc["authType"]); v != "" {
		return v
	}
	if v := strings.TrimSpace(server["defaultAuthType"]); v != "" {
		return v
	}
	return "all"
}

func effectiveNeedAuth(doc, server map[string]string) bool {
	if v, ok := doc["isNeedAuth"]; ok {
		if b, decided := parseBool(v); decided {
			return b
		}
	}
	if v, ok := server["defaultNeedAuth"]; ok {
		if b, decided := parseBool(v); decided {
			return b
		}
	}
	return false
}

// parseBool 解析 "true"/"false"（带或不带引号），返回值与是否可决断。
func parseBool(v string) (bool, bool) {
	v = strings.Trim(strings.TrimSpace(v), `"`)
	switch strings.ToLower(v) {
	case "true", "1":
		return true, true
	case "false", "0", "":
		return false, true
	}
	return false, false
}

// GetBusinessType 复刻访问生成器对 URL 末段的 operType 推导。
// 显式 operType 优先；否则按末段映射到 add/modify/delete/find/other。
func GetBusinessType(path string, or string) string {
	if or != "" {
		return or
	}
	paths := strings.Split(path, "/")
	opt := paths[len(paths)-1]
	switch opt {
	case "create", "multi-create", "import":
		return "add"
	case "update", "multi-update":
		return "modify"
	case "delete", "multi-delete":
		return "delete"
	case "index", "read", "count", "tree":
		return "find"
	default:
		if strings.HasSuffix(opt, "send") {
			return "modify"
		}
		if strings.HasSuffix(opt, "read") || strings.HasSuffix(opt, "index") {
			return "find"
		}
		return "other"
	}
}

// typeContracts 收集入口 spec 全部 DefineStruct，并按 request/response 的
// 递归引用闭包识别源缺失的引用类型。后者用历史 types.go 兜底补入 shape，
// 并把其名字记入 UndefinedTypes（源漂移报告）。types.go 里未被任何路由引用
// 的历史类型不计入源闭包，因此不会误报。
func typeContracts(entrySpec *spec.ApiSpec, typesGo string) ([]TypeContract, []string) {
	byName := map[string]spec.DefineStruct{}
	for _, t := range entrySpec.Types {
		if ds, ok := t.(spec.DefineStruct); ok {
			if _, exists := byName[ds.RawName]; !exists {
				byName[ds.RawName] = ds
			}
		}
	}

	// 源闭包：从全部路由的 request/response 类型出发，递归收集可达的自定义类型。
	closure := map[string]bool{}
	var visit func(name string)
	visit = func(name string) {
		if name == "" || closure[name] {
			return
		}
		ds, ok := byName[name]
		if !ok {
			return
		}
		closure[name] = true
		for _, m := range ds.Members {
			visit(memberTypeName(m.Type))
		}
	}
	for _, g := range entrySpec.Service.Groups {
		for _, r := range g.Routes {
			visit(typeName(r.RequestType))
			visit(typeName(r.ResponseType))
		}
	}

	// 源缺失但被引用的类型：从历史 types.go 补入 shape 并记为 undefined。
	var undefined []string
	if hist, err := parseTypesGo(typesGo); err == nil {
		for _, name := range hist.order {
			if closure[name] {
				continue
			}
			if _, ok := byName[name]; ok {
				continue
			}
			// 仅当它是某 closure 类型的成员引用（递归可达）时才补入。
			if !referencedInClosure(name, byName, closure) {
				continue
			}
			byName[name] = hist.byName[name]
			closure[name] = true
			undefined = append(undefined, name)
		}
	}
	sort.Strings(undefined)

	// 输出顺序：先 closure 中按名字排序的，确保稳定。
	var names []string
	for n := range closure {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]TypeContract, 0, len(names))
	for _, name := range names {
		ds := byName[name]
		tc := TypeContract{Name: name}
		for _, m := range ds.Members {
			tc.Fields = append(tc.Fields, TypeField{
				Name: m.Name,
				Type: normalizeGoType(typeName(m.Type)),
				Tag:  m.Tag,
			})
		}
		out = append(out, tc)
	}
	return out, undefined
}

// referencedInClosure 判断 name 是否被 closure 中任一类型的成员递归引用。
func referencedInClosure(name string, byName map[string]spec.DefineStruct, closure map[string]bool) bool {
	for c := range closure {
		ds := byName[c]
		for _, m := range ds.Members {
			if memberTypeName(m.Type) == name {
				return true
			}
		}
	}
	return false
}

// memberTypeName 提取成员自定义类型名（剥除 *、[]、map 外壳），用于递归闭包。
func memberTypeName(t spec.Type) string {
	if t == nil {
		return ""
	}
	switch v := t.(type) {
	case spec.DefineStruct:
		return v.RawName
	case spec.PointerType:
		return memberTypeName(v.Type)
	case spec.ArrayType:
		return memberTypeName(v.Value)
	case spec.MapType:
		return memberTypeName(v.Value)
	default:
		return ""
	}
}

// normalizeGoType 把 spec.Type 的 Name() 归一化为稳定的字段类型字符串，
// 例如剥除前导 '*'、'[]' 等外壳保留元素类型名（与 types.go 生成一致）。
func normalizeGoType(s string) string {
	s = strings.TrimSpace(s)
	// 形如 *User、[]*User、map[string]User 等保留原 spec 表达。
	return s
}

func typeName(t spec.Type) string {
	if t == nil {
		return ""
	}
	return t.Name()
}

func sortRoutes(rs []Route) {
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].FullPath != rs[j].FullPath {
			return rs[i].FullPath < rs[j].FullPath
		}
		if rs[i].Method != rs[j].Method {
			return rs[i].Method < rs[j].Method
		}
		return rs[i].Handler < rs[j].Handler
	})
}

func sortAuth(as []Authorization) {
	sort.SliceStable(as, func(i, j int) bool {
		if as[i].FullPath != as[j].FullPath {
			return as[i].FullPath < as[j].FullPath
		}
		return as[i].Method < as[j].Method
	})
}

// GoldenFiles 是单个服务 testdata 目录下的六类 golden 文件名。
var GoldenFiles = []string{
	"definitions.golden.json",
	"reachable.golden.json",
	"registered.golden.json",
	"detached-routes.golden.json",
	"authorization.golden.json",
	"types.golden.json",
}

// CompareGolden 把 actual 与 goldenDir 下六类 golden 文件做语义比较。
// golden 缺失视为需要重新生成（由 snapshot --update 写入），返回需更新的错误。
func CompareGolden(actual Snapshot, goldenDir string) error {
	type file struct {
		name string
		v    any
	}
	files := []file{
		{"definitions.golden.json", actual.Defined},
		{"reachable.golden.json", actual.Reachable},
		{"registered.golden.json", actual.Registered},
		{"detached-routes.golden.json", actual.Detached},
		{"authorization.golden.json", actual.Authorization},
		{"types.golden.json", actual.Types},
	}
	var errs []string
	for _, f := range files {
		gp := filepath.Join(goldenDir, f.name)
		want, err := os.ReadFile(gp)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: golden missing (run `apicheck snapshot --update`)", f.name))
			continue
		}
		got, err := jsonMarshalStable(f.v)
		if err != nil {
			return err
		}
		var w, g any
		if err := json.Unmarshal(want, &w); err != nil {
			return fmt.Errorf("%s: parse golden: %w", f.name, err)
		}
		if err := json.Unmarshal(got, &g); err != nil {
			return fmt.Errorf("%s: parse actual: %w", f.name, err)
		}
		if !deepEqualJSON(w, g) {
			errs = append(errs, fmt.Sprintf("%s: mismatch", f.name))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("golden comparison failed:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// jsonMarshalStable 以稳定缩进序列化，保证 --update 写入与比较一致。
func jsonMarshalStable(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// deepEqualJSON 比较两个从 JSON 反序列化的任意值。
func deepEqualJSON(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !deepEqualJSON(v, bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !deepEqualJSON(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

// WriteGolden 把 actual 写入 goldenDir 下六类 golden 文件。
func WriteGolden(actual Snapshot, goldenDir string) error {
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		return err
	}
	type file struct {
		name string
		v    any
	}
	files := []file{
		{"definitions.golden.json", actual.Defined},
		{"reachable.golden.json", actual.Reachable},
		{"registered.golden.json", actual.Registered},
		{"detached-routes.golden.json", actual.Detached},
		{"authorization.golden.json", actual.Authorization},
		{"types.golden.json", actual.Types},
	}
	for _, f := range files {
		buf, err := jsonMarshalStable(f.v)
		if err != nil {
			return err
		}
		buf = append(buf, '\n')
		if err := os.WriteFile(filepath.Join(goldenDir, f.name), buf, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ---- 历史 types.go 兜底解析 ----

type typesGoSnapshot struct {
	byName map[string]spec.DefineStruct
	order  []string
}

// parseTypesGo 用 Go AST 解析历史生成的 types.go，复刻 .api 类型 shape。
func parseTypesGo(path string) (typesGoSnapshot, error) {
	var snap typesGoSnapshot
	snap.byName = map[string]spec.DefineStruct{}
	src, err := os.ReadFile(path)
	if err != nil {
		return snap, err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return snap, err
	}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec0 := range gd.Specs {
			ts, ok := spec0.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			ds := spec.DefineStruct{RawName: ts.Name.Name}
			if st.Fields != nil {
				for _, field := range st.Fields.List {
					for _, name := range field.Names {
						ds.Members = append(ds.Members, spec.Member{
							Name: name.Name,
							Type: spec.PrimitiveType{RawName: exprType(field.Type)},
							Tag:  fieldTag(field),
						})
					}
				}
			}
			snap.byName[ts.Name.Name] = ds
			snap.order = append(snap.order, ts.Name.Name)
		}
	}
	return snap, nil
}

func exprType(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprType(t.X)
	case *ast.ArrayType:
		return "[]" + exprType(t.Elt)
	case *ast.SelectorExpr:
		return exprType(t.X) + "." + t.Sel.Name
	case *ast.MapType:
		return "map[" + exprType(t.Key) + "]" + exprType(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	default:
		return ""
	}
}

func fieldTag(field *ast.Field) string {
	if field.Tag == nil {
		return ""
	}
	return field.Tag.Value
}
