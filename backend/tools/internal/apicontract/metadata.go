package apicontract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// 七项必填元数据字段名（与 @doc block key 一致）。
const (
	metaSummary       = "summary"
	metaDescription   = "description"
	metaAuthType      = "authType"
	metaResource      = "resource"
	metaAction        = "action"
	metaGroup         = "group"
	metaRecordLogMode = "recordLogMode"
)

// RequiredMetadataFields 是每条路由必须显式声明的七项元数据，顺序固定。
var RequiredMetadataFields = []string{
	metaSummary, metaDescription, metaAuthType, metaResource,
	metaAction, metaGroup, metaRecordLogMode,
}

// ValidAuthTypes 是允许的 authType 集合（fork 原集合外加兼容值 supper）。
var ValidAuthTypes = map[string]bool{
	"platform": true,
	"supper":   true,
	"admin":    true,
	"all":      true,
	"public":   true,
	"device":   true,
}

// ValidActions 是允许的 action 集合（fork 的有效业务动词，剔除 query/detail）。
var ValidActions = map[string]bool{
	"view":           true,
	"create":         true,
	"update":         true,
	"delete":         true,
	"bind":           true,
	"unbind":         true,
	"transfer":       true,
	"enable":         true,
	"disable":        true,
	"export":         true,
	"import":         true,
	"execute":        true,
	"approve":        true,
	"accept":         true,
	"reject":         true,
	"publish":        true,
	"send":           true,
	"upload":         true,
	"download":       true,
	"login":          true,
	"logout":         true,
	"register":       true,
	"captcha":        true,
	"reset-password": true,
	"invite":         true,
	"verify":         true,
	"use":            true,
}

// ValidRecordLogModes 是允许的 recordLogMode 值。
var ValidRecordLogModes = map[string]bool{
	"1": true,
	"2": true,
	"3": true,
}

// minDescriptionRunes 是 description 至少需要的 Unicode rune 数。
const minDescriptionRunes = 10

// MetadataProblem 描述一条路由的元数据 lint 失败。
type MetadataProblem struct {
	File     string // 相对 core root 的 .api 路径，unix 风格
	Service  string
	Method   string
	FullPath string
	Handler  string
	Field    string // 缺失/非法的字段名；授权相关为 "authorization"
	Message  string
}

// String 渲染为单行可读错误。
func (p MetadataProblem) String() string {
	return fmt.Sprintf("%s: %s %s handler=%s: %s (%s)",
		p.File, p.Method, p.FullPath, p.Handler, p.Message, p.Field)
}

// routeMeta 是单条路由的元数据快照，用于 lint。
type routeMeta struct {
	file      string
	service   string
	method    string
	fullPath  string
	handler   string
	doc       map[string]string
	serverGrp string
}

// LintMetadata 校验某服务全部 Defined 路由的七项元数据完整性，并断言
// metadata 变更不改变 effective auth/needAuth/operType。coreRoot 既可以是
// backend 目录也可以是 core 目录（与 LoadBackend 同语义）。
func LintMetadata(backendRoot, service string, snap Snapshot) []MetadataProblem {
	coreRoot := ResolveCoreRoot(backendRoot)
	b := Backends(coreRoot)[service]
	metas := scanServiceRouteMetas(b, service)

	var problems []MetadataProblem
	for _, m := range metas {
		problems = append(problems, lintOne(m)...)
	}

	// 授权一致性：metadata（authType/isNeedAuth/operType）推导出的 effective 值
	// 必须等于 Snapshot.Authorization 的 golden 值。这确保补全 metadata 时不会
	// 偷偷改变线上授权语义。
	authByRoute := map[string]Authorization{}
	for _, a := range snap.Authorization {
		authByRoute[a.Method+" "+a.FullPath] = a
	}
	for _, m := range metas {
		golden, ok := authByRoute[m.method+" "+m.fullPath]
		if !ok {
			continue
		}
		if v := strings.TrimSpace(m.doc[metaAuthType]); v != "" && v != golden.EffectiveAuthType {
			problems = append(problems, MetadataProblem{
				File: m.file, Service: m.service, Method: m.method, FullPath: m.fullPath,
				Handler: m.handler, Field: "authorization",
				Message: fmt.Sprintf("authType %q would change effective auth %q", v, golden.EffectiveAuthType),
			})
		}
		if v, ok := m.doc["isNeedAuth"]; ok {
			if b, _ := parseBool(v); b != golden.EffectiveNeedAuth {
				problems = append(problems, MetadataProblem{
					File: m.file, Service: m.service, Method: m.method, FullPath: m.fullPath,
					Handler: m.handler, Field: "authorization",
					Message: fmt.Sprintf("isNeedAuth %q would change effective needAuth %v", v, golden.EffectiveNeedAuth),
				})
			}
		}
		if ot := strings.TrimSpace(m.doc["operType"]); ot != "" && ot != golden.EffectiveOperType {
			problems = append(problems, MetadataProblem{
				File: m.file, Service: m.service, Method: m.method, FullPath: m.fullPath,
				Handler: m.handler, Field: "authorization",
				Message: fmt.Sprintf("operType %q would change effective operType %q", ot, golden.EffectiveOperType),
			})
		}
	}

	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].File != problems[j].File {
			return problems[i].File < problems[j].File
		}
		return problems[i].FullPath < problems[j].FullPath
	})
	return problems
}

// lintOne 校验单条路由的七项元数据。
func lintOne(m routeMeta) []MetadataProblem {
	var probs []MetadataProblem
	add := func(field, msg string) {
		probs = append(probs, MetadataProblem{
			File: m.file, Service: m.service, Method: m.method, FullPath: m.fullPath,
			Handler: m.handler, Field: field, Message: msg,
		})
	}

	// 简写 @doc "..."：doc 为空，或仅含 summary 这种从 line-doc 还原的形态。
	// 缺失 @doc 块时 doc 也为空。两种都按缺字段处理，但当七项里缺失 5 项以上时
	// 优先给出“应改用 @doc(...) 块”的简明提示，避免一次性报六七个字段。
	if len(m.doc) == 0 {
		add(metaSummary, "shorthand @doc \"...\" or missing @doc block; must use @doc(...) with seven fields")
		return probs
	}

	missing := 0
	for _, f := range RequiredMetadataFields {
		if v := strings.TrimSpace(m.doc[f]); v == "" {
			missing++
		}
	}
	if missing >= 5 {
		add(metaSummary, "shorthand @doc \"...\" or incomplete @doc block; must declare all seven fields with @doc(...)")
		return probs
	}

	for _, f := range RequiredMetadataFields {
		if v := strings.TrimSpace(m.doc[f]); v == "" {
			add(f, fmt.Sprintf("%s is required", f))
		}
	}

	summary := strings.TrimSpace(m.doc[metaSummary])
	desc := strings.TrimSpace(m.doc[metaDescription])
	if summary != "" && desc != "" {
		if desc == summary {
			add(metaDescription, "description must differ from summary")
		}
		if utf8.RuneCountInString(desc) < minDescriptionRunes {
			add(metaDescription, fmt.Sprintf("description must be at least %d runes", minDescriptionRunes))
		}
		if isTemplatePlaceholder(desc) {
			add(metaDescription, "description contains template placeholder text")
		}
	}

	if v := strings.TrimSpace(m.doc[metaAuthType]); v != "" && !ValidAuthTypes[v] {
		add(metaAuthType, fmt.Sprintf("authType %q is not allowed", v))
	}
	if v := strings.TrimSpace(m.doc[metaAction]); v != "" && !ValidActions[v] {
		add(metaAction, fmt.Sprintf("action %q is not allowed", v))
	}
	if v := strings.TrimSpace(m.doc[metaRecordLogMode]); v != "" && !ValidRecordLogModes[v] {
		add(metaRecordLogMode, fmt.Sprintf("recordLogMode %q must be 1/2/3", v))
	}

	// route group 必须等于所属 @server group。
	if v := strings.TrimSpace(m.doc[metaGroup]); v != "" && m.serverGrp != "" && v != m.serverGrp {
		add(metaGroup, fmt.Sprintf("route group %q differs from @server group %q", v, m.serverGrp))
	}

	return probs
}

// isTemplatePlaceholder 检测 fork 模板示例占位语句。
func isTemplatePlaceholder(s string) bool {
	low := strings.ToLower(s)
	for _, marker := range []string{"替换为", "todo", "example", "示例描述", "示例接口"} {
		if strings.Contains(low, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// ---- 文本级路由/元数据扫描 ----
//
// scanServiceRouteMetas 通过逐文件文本扫描收集某服务全部 Defined 路由的
// 元数据快照（含游离文件）。这避免了 goctl parser 把 import 图扁平化后
// 丢失物理文件归属的问题，也无需类型解析。

// scanServiceRouteMetas 扫描服务目录全部 .api 文件，提取每条路由的
// (file, method, fullPath, handler, doc, serverGroup)。
func scanServiceRouteMetas(b Backend, service string) []routeMeta {
	var files []string
	filepathWalkAPI(b.ServiceDir, &files)
	sort.Strings(files)

	var out []routeMeta
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		rel := relAPIPath(b, f)
		out = append(out, scanFileRoutes(string(data), rel, service)...)
	}
	return out
}

// scanFileRoutes 解析单个 .api 文件的文本，提取路由元数据。
// 解析模型：文件由若干 @server(...) service NAME { ... } 块与 type 块交替组成。
// 每个 service 块内有多条路由：可选 @doc(...) 块 + @handler X + method /path ...。
func scanFileRoutes(content, file, service string) []routeMeta {
	var out []routeMeta

	// 切出每个 service 块：@server(...) service name { ... }。
	// 用括号配平找到 service 块的闭合大括号。
	idx := 0
	for idx < len(content) {
		// 找下一个 @server 或 service。
		serverAt := strings.Index(content[idx:], "@server")
		if serverAt < 0 {
			break
		}
		serverAt += idx
		if serverAt < 0 {
			break
		}
		// @server 后跟 (...) 然后是 service name {。
		propsStart := strings.IndexByte(content[serverAt:], '(')
		if propsStart < 0 {
			break
		}
		propsStart += serverAt
		propsEnd := matchParen(content, propsStart)
		if propsEnd < 0 {
			break
		}
		serverProps := parseKVBlock(content[propsStart+1 : propsEnd])

		// 找 service name {
		braceStart := strings.Index(content[propsEnd:], "{")
		if braceStart < 0 {
			break
		}
		braceStart += propsEnd
		braceEnd := matchBrace(content, braceStart)
		if braceEnd < 0 {
			break
		}
		body := content[braceStart+1 : braceEnd]
		prefix := normalizePrefix(serverProps["prefix"])
		serverGrp := serverProps["group"]

		out = append(out, scanServiceBodyRoutes(body, file, service, prefix, serverGrp)...)

		idx = braceEnd + 1
	}
	return out
}

// scanServiceBodyRoutes 解析单个 service 块体内的路由。
func scanServiceBodyRoutes(body, file, service, prefix, serverGrp string) []routeMeta {
	var out []routeMeta
	lines := strings.Split(body, "\n")

	currentDoc := map[string]string{}
	pendingHandler := ""
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		trim := strings.TrimSpace(raw)

		// @doc( 块开始。
		if strings.Contains(trim, "@doc(") || strings.HasPrefix(trim, "@doc(") || strings.Contains(trim, "@doc (") {
			// 收集到对应的 ')'
			doc, consumed := collectDocBlock(lines, i)
			currentDoc = doc
			i += consumed
			continue
		}
		// 简写 @doc "..."。
		if strings.HasPrefix(trim, "@doc") {
			rest := strings.TrimSpace(strings.TrimPrefix(trim, "@doc"))
			if len(rest) > 0 && (rest[0] == '"' || strings.HasPrefix(rest, "`")) {
				currentDoc = map[string]string{metaSummary: unquoteStr(rest)}
				continue
			}
		}

		// @handler X。
		if strings.HasPrefix(trim, "@handler") {
			pendingHandler = strings.TrimSpace(strings.TrimPrefix(trim, "@handler"))
			continue
		}

		// 路由行：method /path ...
		if m := routeLineRe.FindStringSubmatch(trim); m != nil && pendingHandler != "" {
			method := strings.ToUpper(m[1])
			path := m[2]
			out = append(out, routeMeta{
				file:      file,
				service:   service,
				method:    method,
				fullPath:  joinPath(prefix, path),
				handler:   pendingHandler,
				doc:       cloneDoc(currentDoc),
				serverGrp: serverGrp,
			})
			currentDoc = map[string]string{}
			pendingHandler = ""
			continue
		}
	}
	return out
}

// routeLineRe 匹配路由声明行，如 "post /create (Req) returns (Resp)"。
var routeLineRe = regexp.MustCompile(`^(get|post|put|delete|patch|head|options)\s+(/\S*)`)

// collectDocBlock 从 lines[start] 开始收集 @doc(...) 块直到匹配的 ')'，
// 返回 key->value 映射与消费的行数（不含 start 行）。
func collectDocBlock(lines []string, start int) (map[string]string, int) {
	doc := map[string]string{}
	// 找 '(' 与对应 ')'，可能跨多行。
	joined := strings.Join(lines[start:], "\n")
	open := strings.IndexByte(joined, '(')
	if open < 0 {
		return doc, 0
	}
	close := matchParen(joined, open)
	if close < 0 {
		return doc, 0
	}
	block := joined[open+1 : close]
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "(" || line == ")" {
			continue
		}
		// key: value  或 key:value
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		val := strings.TrimSpace(line[colon+1:])
		val = strings.TrimRight(val, ",")
		doc[key] = unquoteStr(strings.TrimSpace(val))
	}
	// 计算消费的行数：从 start 到 close 所在行。
	consumed := strings.Count(joined[:close+1], "\n")
	return doc, consumed
}

// matchParen 返回与 pos 处 '(' 配平的 ')' 索引；不计入字符串字面量。
func matchParen(s string, pos int) int {
	depth := 0
	inStr := byte(0)
	for i := pos; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '`', '\'':
			inStr = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// matchBrace 返回与 pos 处 '{' 配平的 '}' 索引。
func matchBrace(s string, pos int) int {
	depth := 0
	inStr := byte(0)
	for i := pos; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '`', '\'':
			inStr = c
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// parseKVBlock 解析 @server(...) 内的 key:value 对（value 可带引号）。
func parseKVBlock(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		val := strings.TrimSpace(line[colon+1:])
		val = strings.TrimRight(val, ",")
		out[key] = unquoteStr(val)
	}
	return out
}

// unquoteStr 去掉首尾的引号或反引号。
func unquoteStr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '`' && s[len(s)-1] == '`') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func cloneDoc(d map[string]string) map[string]string {
	if len(d) == 0 {
		return nil
	}
	out := make(map[string]string, len(d))
	for k, v := range d {
		out[k] = v
	}
	return out
}

// filepathWalkAPI 收集 dir 下全部 .api 文件绝对路径。
func filepathWalkAPI(dir string, out *[]string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".api") {
			*out = append(*out, path)
		}
		return nil
	})
}

// relAPIPath 返回文件相对 core root 的 unix 风格路径（用于 lint 输出/过滤）。
func relAPIPath(b Backend, file string) string {
	// b.ServiceDir = <coreRoot>/service/<svc>/http；coreRoot = 上两级再上两级。
	coreRoot := filepath.Dir(filepath.Dir(filepath.Dir(b.ServiceDir)))
	rel, err := filepath.Rel(coreRoot, file)
	if err != nil {
		return filepath.Base(file)
	}
	return filepath.ToSlash(rel)
}
