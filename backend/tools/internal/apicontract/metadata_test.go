package apicontract

import (
	"strings"
	"testing"
)

// metaFixture 返回一条满足全部七项元数据的 routeMeta，单测按需改字段。
func metaFixture() routeMeta {
	return routeMeta{
		file:     "service/apisvr/http/system/role.api",
		service:  "apisvr",
		method:   "POST",
		fullPath: "/api/v1/system/role/info/create",
		handler:  "create",
		serverGrp: "system/role/info",
		doc: map[string]string{
			"summary":       "添加角色",
			"description":   "在当前租户下新建角色，含名称、备注与状态",
			"authType":      "admin",
			"resource":      "role",
			"action":        "create",
			"group":         "system/role/info",
			"recordLogMode": "2",
		},
	}
}

func TestLintOneValid(t *testing.T) {
	if probs := lintOne(metaFixture()); len(probs) != 0 {
		t.Fatalf("expected no problems, got: %+v", probs)
	}
}

func TestLintOneShorthandDoc(t *testing.T) {
	m := metaFixture()
	m.doc = map[string]string{"summary": "添加角色"} // 简写 @doc "..."
	probs := lintOne(m)
	if len(probs) != 1 {
		t.Fatalf("expected 1 problem for shorthand @doc, got %d: %+v", len(probs), probs)
	}
	if !strings.Contains(probs[0].Message, "shorthand") {
		t.Errorf("expected shorthand message, got %q", probs[0].Message)
	}
}

func TestLintOneMissingField(t *testing.T) {
	cases := []string{"summary", "description", "authType", "resource", "action", "group", "recordLogMode"}
	for _, field := range cases {
		m := metaFixture()
		delete(m.doc, field)
		probs := lintOne(m)
		found := false
		for _, p := range probs {
			if p.Field == field {
				found = true
			}
		}
		if !found {
			t.Errorf("removing %s should produce a problem; got %+v", field, probs)
		}
	}
}

func TestLintOneDescriptionEqualsSummary(t *testing.T) {
	m := metaFixture()
	m.doc["description"] = m.doc["summary"]
	probs := lintOne(m)
	if !hasField(probs, "description") {
		t.Errorf("description==summary should flag description")
	}
}

func TestLintOneDescriptionTooShort(t *testing.T) {
	m := metaFixture()
	m.doc["description"] = "太短" // 2 rune < 10
	probs := lintOne(m)
	if !hasField(probs, "description") {
		t.Errorf("short description should flag description")
	}
}

func TestLintOneDescriptionTemplatePlaceholder(t *testing.T) {
	m := metaFixture()
	m.doc["description"] = "TODO: 替换为实际业务说明"
	probs := lintOne(m)
	if !hasField(probs, "description") {
		t.Errorf("placeholder description should flag description")
	}
}

func TestLintOneInvalidAuthType(t *testing.T) {
	for _, bad := range []string{"unknown", "ROOT", "", "user"} {
		m := metaFixture()
		m.doc["authType"] = bad
		if bad == "" {
			delete(m.doc, "authType")
		}
		probs := lintOne(m)
		// "" 触发 missing 而非 invalid；非空非法值触发 invalid。
		if bad == "" {
			if !hasField(probs, "authType") {
				t.Errorf("missing authType should flag")
			}
			continue
		}
		if !hasFieldMessage(probs, "authType", "not allowed") {
			t.Errorf("invalid authType %q should be flagged; got %+v", bad, probs)
		}
	}
}

func TestLintOneValidAuthTypes(t *testing.T) {
	for _, ok := range []string{"platform", "supper", "admin", "all", "public", "device"} {
		m := metaFixture()
		m.doc["authType"] = ok
		if hasField(lintOne(m), "authType") {
			t.Errorf("authType %q should be allowed", ok)
		}
	}
}

func TestLintOneInvalidAction(t *testing.T) {
	for _, bad := range []string{"query", "detail", "get", "get-list", "foo"} {
		m := metaFixture()
		m.doc["action"] = bad
		probs := lintOne(m)
		if !hasFieldMessage(probs, "action", "not allowed") {
			t.Errorf("invalid action %q should be flagged; got %+v", bad, probs)
		}
	}
}

func TestLintOneValidActions(t *testing.T) {
	for _, ok := range []string{"view", "create", "update", "delete", "export", "import", "login", "captcha", "reset-password", "upload", "download"} {
		m := metaFixture()
		m.doc["action"] = ok
		if hasField(lintOne(m), "action") {
			t.Errorf("action %q should be allowed", ok)
		}
	}
}

func TestLintOneInvalidRecordLogMode(t *testing.T) {
	for _, bad := range []string{"0", "4", "yes"} {
		m := metaFixture()
		m.doc["recordLogMode"] = bad
		probs := lintOne(m)
		if !hasFieldMessage(probs, "recordLogMode", "1/2/3") {
			t.Errorf("recordLogMode %q should be flagged; got %+v", bad, probs)
		}
	}
}

func TestLintOneGroupMismatch(t *testing.T) {
	m := metaFixture()
	m.doc["group"] = "system/role/app" // 与 @server group system/role/info 不同
	m.serverGrp = "system/role/info"
	probs := lintOne(m)
	if !hasField(probs, "group") {
		t.Errorf("group mismatch should be flagged")
	}
}

func TestLintOneGroupMatch(t *testing.T) {
	m := metaFixture()
	m.doc["group"] = "system/role/info"
	m.serverGrp = "system/role/info"
	if hasField(lintOne(m), "group") {
		t.Errorf("matching group should not be flagged")
	}
}

// TestScanFileRoutesEndToEnd 校验文本扫描器能从 .api 片段提取 @server/@doc/
// @handler/route，包括简写 @doc 与缺 @doc 两种缺失场景。
func TestScanFileRoutesEndToEnd(t *testing.T) {
	content := `@server(
	group: system/role/info
	prefix: /api/v1/system/role/info
	defaultAuthType: "supper"
	middleware: CheckTokenWare,InitCtxsWare
)
service api {
	@doc(
		summary: "添加角色"
		description: "在当前租户下新建角色，含名称与状态"
		authType: "admin"
		resource: "role"
		action: "create"
		group: "system/role/info"
		recordLogMode: "2"
	)
	@handler create
	post /create (RoleInfo) returns (WithID)

	@doc "简写获取角色"
	@handler index
	post /index (RoleInfoIndexReq) returns (RoleInfoIndexResp)

	@handler noDoc
	post /nope ()
}
`
	routes := scanFileRoutes(content, "role.api", "apisvr")
	if len(routes) != 3 {
		t.Fatalf("scanned %d routes want 3: %+v", len(routes), routes)
	}
	byHandler := map[string]routeMeta{}
	for _, r := range routes {
		byHandler[r.handler] = r
	}
	if r := byHandler["create"]; r.fullPath != "/api/v1/system/role/info/create" || r.method != "POST" {
		t.Errorf("create route wrong: %+v", r)
	}
	if r := byHandler["create"]; r.doc["authType"] != "admin" {
		t.Errorf("create authType not captured: %+v", r.doc)
	}
	if r := byHandler["create"]; r.serverGrp != "system/role/info" {
		t.Errorf("create serverGrp wrong: %q", r.serverGrp)
	}
	// 简写 @doc：summary 捕获，但缺七项字段 -> lint 会报 shorthand。
	if r := byHandler["index"]; r.doc["summary"] != "简写获取角色" {
		t.Errorf("shorthand summary not captured: %+v", r.doc)
	}
	if len(byHandler["index"].doc) != 1 {
		t.Errorf("shorthand should only carry summary, got %+v", byHandler["index"].doc)
	}
	// 缺 @doc：doc 为空。
	if len(byHandler["noDoc"].doc) != 0 {
		t.Errorf("noDoc should have empty doc, got %+v", byHandler["noDoc"].doc)
	}
}

// TestLintMetadataAuthorizationDrift 校验 metadata 改变 effective auth 时被捕获。
func TestLintMetadataAuthorizationDrift(t *testing.T) {
	root := apisvrTree(t)
	b := Backends(root)["apisvr"]
	snap, err := LoadService(b)
	if err != nil {
		t.Fatalf("LoadService: %v", err)
	}
	// 默认 fixture 全合规，应无问题。
	if probs := LintMetadata(root, "apisvr", snap); len(probs) != 0 {
		t.Fatalf("expected no problems on clean fixture, got %+v", probs)
	}

	// 篡改 golden authorization：把 create 的 effective auth 从 admin 改成 supper，
	// 模拟 metadata 与 golden 不一致。route @doc 仍是 admin，故应报 drift。
	for i := range snap.Authorization {
		if snap.Authorization[i].FullPath == "/api/v1/system/role/info/create" {
			snap.Authorization[i].EffectiveAuthType = "supper"
		}
	}
	probs := LintMetadata(root, "apisvr", snap)
	found := false
	for _, p := range probs {
		if p.Field == "authorization" && strings.Contains(p.Message, "authType") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected authorization drift problem for create, got %+v", probs)
	}
}

func hasField(probs []MetadataProblem, field string) bool {
	for _, p := range probs {
		if p.Field == field {
			return true
		}
	}
	return false
}

func hasFieldMessage(probs []MetadataProblem, field, sub string) bool {
	for _, p := range probs {
		if p.Field == field && strings.Contains(p.Message, sub) {
			return true
		}
	}
	return false
}
