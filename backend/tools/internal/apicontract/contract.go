// Package apicontract 解析 go-zero .api 契约源、生成的 routes.go 以及历史
// types.go，把它们归一化为稳定的 Snapshot 结构，用于冻结四类 golden 与七项
// 元数据 lint。
//
// 设计要点：
//   - 解析器复用固定版本的 github.com/zeromicro/go-zero/tools/goctl@v1.8.4
//     的 api/parser，只读其 AST/spec，不调用任何 generator。
//   - Reachable 只遍历入口 import graph；Defined 还覆盖游离（detached）文件；
//     Registered 解析实际调用的 internal/handler/routes.go，不把未注册的
//     wsRoutes.go 计入 REST 注册。
//   - 授权决策（authType/needAuth/operType）严格按 access 生成器的算法推导，
//     确保 metadata 变更不会悄悄改变线上授权语义。
package apicontract

// Route 描述一条归一化后的 HTTP 路由。FullPath 是 prefix + route path 并
// 归一化前导 '/' 之后的稳定形式，是 Defined/Reachable/Registered/Detached
// 四类 golden 共用的比较键。
type Route struct {
	Service      string   `json:"service"`
	Method       string   `json:"method"`
	FullPath     string   `json:"fullPath"`
	Handler      string   `json:"handler"`
	RequestType  string   `json:"requestType,omitempty"`
	ResponseType string   `json:"responseType,omitempty"`
	Middlewares  []string `json:"middlewares,omitempty"`
}

// Authorization 冻结一条路由的授权决策。EffectiveAuthType/NeedAuth/OperType
// 是 metadata 变更后会被 access 生成器实际采纳的值；ManagedByAccess 标记该
// 路由是否落在带 accessGroup 的 @server 块中（即会进入 access.json）。
type Authorization struct {
	Method            string `json:"method"`
	FullPath          string `json:"fullPath"`
	EffectiveAuthType string `json:"effectiveAuthType"`
	EffectiveNeedAuth bool   `json:"effectiveNeedAuth"`
	EffectiveOperType string `json:"effectiveOperType"`
	ManagedByAccess   bool   `json:"managedByAccess"`
}

// TypeField 描述对外类型的单个字段。Type 是规范化后的 Go 类型名（剥除指针、
// 切片、map 外壳后保留元素类型名），Tag 保留 .api 源里完整的反引号 tag 文本。
type TypeField struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Tag  string `json:"tag"`
}

// TypeContract 描述一个对外 request/response 类型及其递归可达字段集合。
type TypeContract struct {
	Name   string      `json:"name"`
	Fields []TypeField `json:"fields"`
}

// Snapshot 是单个服务的完整契约快照，对应 testdata 下六类 golden 文件。
// UndefinedTypes 报告源里缺失但历史 types.go 已生成的引用类型，供源闭包校验。
type Snapshot struct {
	Defined        []Route         `json:"defined"`
	Reachable      []Route         `json:"reachable"`
	Registered     []Route         `json:"registered"`
	Detached       []Route         `json:"detached"`
	Authorization  []Authorization `json:"authorization"`
	Types          []TypeContract  `json:"types"`
	UndefinedTypes []string        `json:"undefinedTypes,omitempty"`
}
