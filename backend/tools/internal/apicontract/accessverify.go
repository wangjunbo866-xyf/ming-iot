package apicontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// generatedAccess 是 goctl `api access` 输出的 JSON 结构的子集，仅取比较所需
// 字段。goctl fork 写出的形态（access/generate/entities.go）：
//
//	{ "Access": { "<code>": { "authType":.., "isNeedAuth":..,
//	    "apis": [ { method, route, isNeedAuth, operType, authType, ... } ] } } }
type generatedAccess struct {
	Access map[string]generatedAccessInfo `json:"Access"`
}

type generatedAccessInfo struct {
	AuthType   string             `json:"authType"`
	IsNeedAuth int64              `json:"isNeedAuth"`
	Apis       []generatedAPIInfo `json:"apis"`
}

type generatedAPIInfo struct {
	Method     string `json:"method"`
	Route      string `json:"route"`
	IsNeedAuth int64  `json:"isNeedAuth"`
	OperType   string `json:"operType"`
	AuthType   string `json:"authType"`
}

// VerifyAccessOptions 控制 VerifyAccess 的失败语义。
//
// goctl fork 的 access 生成器对已被 parser 去引号的 @doc 值调用
// strconv.Unquote 会失败，导致生成的 authType/isNeedAuth 对带显式 @doc
// authType/isNeedAuth 的路由坍缩为 "all"/2。因此 authType/needAuth 的逐字段
// 相等在 fork 当前版本不可达；operType 与路由集合比较是可信的。
//
//   - FailOnAuth=true：authType/needAuth 不一致即失败（用于发现 fork 退化）。
//   - FailOnAuth=false：仅 operType/集合不一致失败；auth/need 仅报告。
type VerifyAccessOptions struct {
	FailOnAuth bool
}

// AccessReport 是 VerifyAccess 的结构化结果。
type AccessReport struct {
	ManagedGolden    int
	GeneratedAPIs    int
	MissingGenerated int // golden managed 路由在生成 access 中缺失的数量
	ExtraGenerated   int // 生成 access 中不在 golden managed 子集的路由数量
	OperTypeMismatch int
	AuthMismatch     int
	NeedAuthMismatch int
	// Details 是前若干条逐路由差异，供人类阅读。
	Details []string
}

// Report 渲染为单行摘要 + Details。
func (r AccessReport) Report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "managed-golden=%d generated=%d missing=%d extra=%d operType-mismatch=%d auth-mismatch=%d needAuth-mismatch=%d\n",
		r.ManagedGolden, r.GeneratedAPIs, r.MissingGenerated, r.ExtraGenerated,
		r.OperTypeMismatch, r.AuthMismatch, r.NeedAuthMismatch)
	for _, d := range r.Details {
		fmt.Fprintf(&b, "  %s\n", d)
	}
	return b.String()
}

// VerifyAccess 把 goctl 生成的 access JSON 与某服务 authorization golden 比较。
//
// 它只比较 ManagedByAccess=true 的 golden 路由（即落在带 accessGroup 的 @server
// 块、会被 goctl access 实际采纳的路由），对每条比较 method/fullPath/authType/
// needAuth/operType。needAuth 以布尔语义比较（goctl isNeedAuth: 1=true, 2=false）。
// 集合关系：生成的 access 路由集合必须等于 golden managed 子集。
//
// backendRoot 既可指向直接含 service/ 的目录，也可指向仓库 backend 目录。
func VerifyAccess(backendRoot, service string, generatedJSON []byte, opts VerifyAccessOptions) (AccessReport, error) {
	coreRoot := ResolveCoreRoot(backendRoot)
	rep := AccessReport{}

	// 1. 加载 authorization golden。
	dir := filepath.Join(coreRoot, "service", service, "http", "testdata")
	golden, err := loadAuthorizationGolden(dir)
	if err != nil {
		return rep, err
	}
	managed := []Authorization{}
	for _, a := range golden {
		if a.ManagedByAccess {
			managed = append(managed, a)
		}
	}
	rep.ManagedGolden = len(managed)

	// 2. 解析生成的 access JSON。
	var gen generatedAccess
	dec := json.NewDecoder(bytes.NewReader(generatedJSON))
	dec.UseNumber()
	if err := dec.Decode(&gen); err != nil {
		return rep, fmt.Errorf("parse generated access json: %w", err)
	}
	type gkey struct{ method, route string }
	genByRoute := map[gkey]generatedAPIInfo{}
	var genKeys []gkey
	for _, info := range gen.Access {
		for _, api := range info.Apis {
			k := gkey{method: strings.ToUpper(api.Method), route: api.Route}
			if _, dup := genByRoute[k]; !dup {
				genKeys = append(genKeys, k)
			}
			genByRoute[k] = api
		}
	}
	rep.GeneratedAPIs = len(genByRoute)

	// 3. 逐条比较 golden managed vs 生成。
	goldenByKey := map[gkey]Authorization{}
	for _, a := range managed {
		goldenByKey[gkey{a.Method, a.FullPath}] = a
	}
	genKeySet := map[gkey]bool{}
	for k := range genByRoute {
		genKeySet[k] = true
	}

	// 顺序遍历保证 Details 稳定。
	sort.Slice(managed, func(i, j int) bool {
		if managed[i].FullPath != managed[j].FullPath {
			return managed[i].FullPath < managed[j].FullPath
		}
		return managed[i].Method < managed[j].Method
	})
	sort.Slice(genKeys, func(i, j int) bool {
		if genKeys[i].route != genKeys[j].route {
			return genKeys[i].route < genKeys[j].route
		}
		return genKeys[i].method < genKeys[j].method
	})

	addDetail := func(format string, args ...any) {
		if len(rep.Details) < 50 {
			rep.Details = append(rep.Details, fmt.Sprintf(format, args...))
		}
	}

	for _, a := range managed {
		k := gkey{a.Method, a.FullPath}
		g, ok := genByRoute[k]
		if !ok {
			rep.MissingGenerated++
			addDetail("MISSING %s %s in generated access", a.Method, a.FullPath)
			continue
		}
		if g.OperType != a.EffectiveOperType {
			rep.OperTypeMismatch++
			addDetail("operType %s %s: golden=%q generated=%q", a.Method, a.FullPath, a.EffectiveOperType, g.OperType)
		}
		genNeed := g.IsNeedAuth == 1
		if genNeed != a.EffectiveNeedAuth {
			rep.NeedAuthMismatch++
			addDetail("needAuth %s %s: golden=%v generated=%v(isNeedAuth=%d)", a.Method, a.FullPath, a.EffectiveNeedAuth, genNeed, g.IsNeedAuth)
		}
		if g.AuthType != a.EffectiveAuthType {
			rep.AuthMismatch++
			addDetail("authType %s %s: golden=%q generated=%q", a.Method, a.FullPath, a.EffectiveAuthType, g.AuthType)
		}
	}
	for _, k := range genKeys {
		if _, ok := goldenByKey[k]; !ok {
			rep.ExtraGenerated++
			addDetail("EXTRA %s %s in generated access not in golden managed", k.method, k.route)
		}
	}

	// 4. 失败判定。
	var errs []string
	if rep.MissingGenerated != 0 {
		errs = append(errs, fmt.Sprintf("%d managed golden routes missing from generated access", rep.MissingGenerated))
	}
	if rep.ExtraGenerated != 0 {
		errs = append(errs, fmt.Sprintf("%d generated access routes not in golden managed subset", rep.ExtraGenerated))
	}
	if rep.OperTypeMismatch != 0 {
		errs = append(errs, fmt.Sprintf("%d operType mismatches", rep.OperTypeMismatch))
	}
	if opts.FailOnAuth {
		if rep.AuthMismatch != 0 {
			errs = append(errs, fmt.Sprintf("%d authType mismatches", rep.AuthMismatch))
		}
		if rep.NeedAuthMismatch != 0 {
			errs = append(errs, fmt.Sprintf("%d needAuth mismatches", rep.NeedAuthMismatch))
		}
	}
	if len(errs) != 0 {
		return rep, fmt.Errorf("access verification failed: %s\n%s", strings.Join(errs, "; "), rep.Report())
	}
	return rep, nil
}

// loadAuthorizationGolden 读取 authorization.golden.json。
func loadAuthorizationGolden(testdataDir string) ([]Authorization, error) {
	p := filepath.Join(testdataDir, "authorization.golden.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read authorization golden: %w", err)
	}
	var auths []Authorization
	if err := json.Unmarshal(data, &auths); err != nil {
		return nil, fmt.Errorf("parse authorization golden: %w", err)
	}
	return auths, nil
}
