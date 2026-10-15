package apicontract

import (
	"path/filepath"
	"testing"
)

// minimalAccessJSON 是一个最小可用的 goctl access 生成器输出结构：
//   Access.<code>.{authType, isNeedAuth, apis[]: {method, route, authType, isNeedAuth, operType}}
const minimalAccessJSON = `{
  "Access": {
    "demoAdd": {
      "code": "demoAdd",
      "authType": "all",
      "isNeedAuth": 1,
      "apis": [
        {
          "accessCode": "demoAdd",
          "method": "POST",
          "route": "/api/v1/demo/create",
          "isNeedAuth": 1,
          "operType": "add",
          "authType": "supper"
        }
      ]
    },
    "demoFind": {
      "code": "demoFind",
      "authType": "all",
      "isNeedAuth": 2,
      "apis": [
        {
          "accessCode": "demoFind",
          "method": "POST",
          "route": "/api/v1/demo/index",
          "isNeedAuth": 2,
          "operType": "find",
          "authType": "all"
        }
      ]
    }
  }
}`

// buildGoldenWithAuth 构造一个临时 core 根并写入两条 ManagedByAccess=true 的
// authorization golden 路由，返回 (coreRoot, service) 供 VerifyAccess 比较。
func buildGoldenWithAuth(t *testing.T, auths []Authorization) string {
	t.Helper()
	root := t.TempDir()
	snap := Snapshot{Authorization: auths}
	dir := filepath.Join(root, "service", "demo", "http", "testdata")
	if err := WriteGolden(snap, dir); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	return root
}

func TestVerifyAccessMatch(t *testing.T) {
	root := buildGoldenWithAuth(t, []Authorization{
		{Method: "POST", FullPath: "/api/v1/demo/create", EffectiveAuthType: "supper", EffectiveNeedAuth: true, EffectiveOperType: "add", ManagedByAccess: true},
		{Method: "POST", FullPath: "/api/v1/demo/index", EffectiveAuthType: "all", EffectiveNeedAuth: false, EffectiveOperType: "find", ManagedByAccess: true},
	})
	rep, err := VerifyAccess(root, "demo", []byte(minimalAccessJSON), VerifyAccessOptions{FailOnAuth: true})
	if err != nil {
		t.Fatalf("VerifyAccess: %v\n%s", err, rep.Report())
	}
	if rep.ManagedGolden != 2 || rep.GeneratedAPIs != 2 {
		t.Errorf("counts: managed=%d gen=%d, want 2/2\n%s", rep.ManagedGolden, rep.GeneratedAPIs, rep.Report())
	}
	if rep.AuthMismatch != 0 || rep.NeedAuthMismatch != 0 || rep.OperTypeMismatch != 0 {
		t.Errorf("expected zero mismatches, got auth=%d need=%d oper=%d\n%s",
			rep.AuthMismatch, rep.NeedAuthMismatch, rep.OperTypeMismatch, rep.Report())
	}
}

// TestVerifyAccessOperTypeCaptured: golden 里 operType 被改（add->modify），
// 必须 mismatch>=1。证明 operType 是被比较的字段。
func TestVerifyAccessOperTypeCaught(t *testing.T) {
	root := buildGoldenWithAuth(t, []Authorization{
		{Method: "POST", FullPath: "/api/v1/demo/create", EffectiveAuthType: "supper", EffectiveNeedAuth: true, EffectiveOperType: "modify", ManagedByAccess: true},
	})
	rep, err := VerifyAccess(root, "demo", []byte(minimalAccessJSON), VerifyAccessOptions{})
	if err == nil {
		t.Fatalf("expected mismatch error, got nil")
	}
	if rep.OperTypeMismatch == 0 {
		t.Errorf("operType mismatch should be detected\n%s", rep.Report())
	}
}

// TestVerifyAccessAuthCaught: golden authType=supper，但生成 JSON 写成 all，
// FailOnAuth=true 时必须失败。证明 authType 是被比较的字段。
func TestVerifyAccessAuthCaught(t *testing.T) {
	root := buildGoldenWithAuth(t, []Authorization{
		{Method: "POST", FullPath: "/api/v1/demo/create", EffectiveAuthType: "supper", EffectiveNeedAuth: true, EffectiveOperType: "add", ManagedByAccess: true},
	})
	// 把生成的 authType 改成 all（与 golden supper 不同）。
	const gen = `{"Access":{"demoAdd":{"authType":"all","isNeedAuth":1,"apis":[
		{"method":"POST","route":"/api/v1/demo/create","isNeedAuth":1,"operType":"add","authType":"all"}]}}}`
	rep, err := VerifyAccess(root, "demo", []byte(gen), VerifyAccessOptions{FailOnAuth: true})
	if err == nil {
		t.Fatalf("expected auth mismatch error, got nil")
	}
	if rep.AuthMismatch == 0 {
		t.Errorf("auth mismatch should be detected\n%s", rep.Report())
	}
}

// TestVerifyAccessNeedAuthCaught: golden needAuth=true 但生成 isNeedAuth=2(false)。
func TestVerifyAccessNeedAuthCaught(t *testing.T) {
	root := buildGoldenWithAuth(t, []Authorization{
		{Method: "POST", FullPath: "/api/v1/demo/create", EffectiveAuthType: "supper", EffectiveNeedAuth: true, EffectiveOperType: "add", ManagedByAccess: true},
	})
	const gen = `{"Access":{"demoAdd":{"authType":"all","isNeedAuth":1,"apis":[
		{"method":"POST","route":"/api/v1/demo/create","isNeedAuth":2,"operType":"add","authType":"supper"}]}}}`
	rep, err := VerifyAccess(root, "demo", []byte(gen), VerifyAccessOptions{FailOnAuth: true})
	if err == nil {
		t.Fatalf("expected needAuth mismatch error, got nil")
	}
	if rep.NeedAuthMismatch == 0 {
		t.Errorf("needAuth mismatch should be detected\n%s", rep.Report())
	}
}

// TestVerifyAccessExtraInGenerated: 生成的 access 多出一条 golden managed 子集
// 没有的路由，必须报告 extra>0 并失败。
func TestVerifyAccessExtraCaught(t *testing.T) {
	root := buildGoldenWithAuth(t, []Authorization{
		{Method: "POST", FullPath: "/api/v1/demo/create", EffectiveAuthType: "supper", EffectiveNeedAuth: true, EffectiveOperType: "add", ManagedByAccess: true},
	})
	const gen = `{"Access":{
		"demoAdd":{"authType":"all","isNeedAuth":1,"apis":[
			{"method":"POST","route":"/api/v1/demo/create","isNeedAuth":1,"operType":"add","authType":"supper"}]},
		"demoExtra":{"authType":"all","isNeedAuth":1,"apis":[
			{"method":"POST","route":"/api/v1/demo/extra","isNeedAuth":1,"operType":"other","authType":"all"}]}
	}}`
	rep, err := VerifyAccess(root, "demo", []byte(gen), VerifyAccessOptions{FailOnAuth: true})
	if err == nil {
		t.Fatalf("expected extra-set error, got nil")
	}
	if rep.ExtraGenerated == 0 {
		t.Errorf("extra-in-generated should be detected\n%s", rep.Report())
	}
}

// TestVerifyAccessMissingGenerated: golden managed 路由在生成 access 中缺失。
func TestVerifyAccessMissingCaught(t *testing.T) {
	root := buildGoldenWithAuth(t, []Authorization{
		{Method: "POST", FullPath: "/api/v1/demo/create", EffectiveAuthType: "supper", EffectiveNeedAuth: true, EffectiveOperType: "add", ManagedByAccess: true},
		{Method: "POST", FullPath: "/api/v1/demo/index", EffectiveAuthType: "all", EffectiveNeedAuth: false, EffectiveOperType: "find", ManagedByAccess: true},
	})
	// 生成只有 create，缺 index。
	const gen = `{"Access":{"demoAdd":{"authType":"all","isNeedAuth":1,"apis":[
		{"method":"POST","route":"/api/v1/demo/create","isNeedAuth":1,"operType":"add","authType":"supper"}]}}}`
	rep, err := VerifyAccess(root, "demo", []byte(gen), VerifyAccessOptions{FailOnAuth: true})
	if err == nil {
		t.Fatalf("expected missing-set error, got nil")
	}
	if rep.MissingGenerated == 0 {
		t.Errorf("missing-in-generated should be detected\n%s", rep.Report())
	}
}
