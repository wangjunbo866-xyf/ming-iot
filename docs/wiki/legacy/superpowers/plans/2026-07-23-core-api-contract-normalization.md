# Core HTTP API 契约补全实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 HTTP method、path、handler、请求/响应类型、中间件顺序和旧授权决策不变的条件下，修复 `.api` 源契约漂移，并为全部 244 条定义路由补齐准确的七项元数据。

**Architecture:** 先独立冻结“定义路由、聚合可达路由、运行时注册路由、授权决策”四类 golden，再修改 `.api`。契约工具比较语义而非生成 JSON 文本；`system/oss.api` 的两条历史游离路由继续不可达。元数据按五个业务批次人工补齐，每批都先过 route/auth golden，再提交。

**Tech Stack:** go-zero `.api` AST、Go AST、JSON golden、Go 1.24.4、现有 handler/logic、后续固定的 UnitedRhino goctl fork。

---

## 不变量和已核实基线

- `.api` 文件：40 个。
- 路由定义：244 条，其中 apisvr 237、datasvr 7。
- 聚合可达且实际 REST 注册：242 条，其中 apisvr 235、datasvr 7。
- `service/apisvr/http/system/oss.api` 未被聚合文件 import，其两条定义为：

```text
PUT /open/oss/upload   handler=upload
GET /open/oss/download handler=downLoad
```

- 这两条不得在本计划中进入 `api.api`/`system.api` import graph。
- 定义中的非 POST 共 6 条；实际注册的是 4 条 GET，另外 GET/PUT 各一条属于上述游离契约。
- 现状元数据计数：summary 87、description 0、authType 72、resource/action/group/recordLogMode 0；另有 155 个简写 `@doc "..."` 和 2 个无 `@doc` 路由。
- `.api` 三处引用 `AppleAppConfig`，但只有历史生成 `types.go` 和 RPC proto 定义它；`.api` 源中缺失。
- 权限字符串 `supper` 保持不变。
- 新 `action` 不替换旧 `operType`：联犀 access 生成器仍从显式 `operType` 或 URL 后缀推导 `add/modify/delete/find/other`。本次分别冻结新元数据与旧 operType 语义。

### Task 1：实现 HTTP 契约解析器和四类 golden

**Files:**

- Create: `backend/tools/internal/apicontract/contract.go`
- Create: `backend/tools/internal/apicontract/snapshot.go`
- Create: `backend/tools/internal/apicontract/registered.go`
- Test: `backend/tools/internal/apicontract/contract_test.go`
- Create: `backend/tools/cmd/apicheck/main.go`
- Test: `backend/tools/cmd/apicheck/main_test.go`
- Modify: `backend/tools/go.mod`
- Modify: `backend/tools/go.sum`
- Create: `backend/core/service/apisvr/http/testdata/definitions.golden.json`
- Create: `backend/core/service/apisvr/http/testdata/reachable.golden.json`
- Create: `backend/core/service/apisvr/http/testdata/registered.golden.json`
- Create: `backend/core/service/apisvr/http/testdata/detached-routes.golden.json`
- Create: `backend/core/service/apisvr/http/testdata/authorization.golden.json`
- Create: `backend/core/service/apisvr/http/testdata/types.golden.json`
- Create: `backend/core/service/datasvr/http/testdata/definitions.golden.json`
- Create: `backend/core/service/datasvr/http/testdata/reachable.golden.json`
- Create: `backend/core/service/datasvr/http/testdata/registered.golden.json`
- Create: `backend/core/service/datasvr/http/testdata/authorization.golden.json`
- Create: `backend/core/service/datasvr/http/testdata/types.golden.json`

- [ ] **Step 1: 定义稳定快照结构并写失败测试**

`contract.go` 使用以下公开结构：

```go
type Route struct {
	Service      string   `json:"service"`
	Method       string   `json:"method"`
	FullPath     string   `json:"fullPath"`
	Handler      string   `json:"handler"`
	RequestType  string   `json:"requestType,omitempty"`
	ResponseType string   `json:"responseType,omitempty"`
	Middlewares  []string `json:"middlewares,omitempty"`
}

type Authorization struct {
	Method            string `json:"method"`
	FullPath          string `json:"fullPath"`
	EffectiveAuthType string `json:"effectiveAuthType"`
	EffectiveNeedAuth bool   `json:"effectiveNeedAuth"`
	EffectiveOperType string `json:"effectiveOperType"`
	ManagedByAccess   bool   `json:"managedByAccess"`
}

type TypeField struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Tag  string `json:"tag"`
}

type TypeContract struct {
	Name   string      `json:"name"`
	Fields []TypeField `json:"fields"`
}

type Snapshot struct {
	Defined       []Route         `json:"defined"`
	Reachable     []Route         `json:"reachable"`
	Registered    []Route         `json:"registered"`
	Detached      []Route         `json:"detached"`
	Authorization []Authorization `json:"authorization"`
	Types         []TypeContract  `json:"types"`
	UndefinedTypes []string       `json:"undefinedTypes,omitempty"`
}
```

测试 fixture 必须覆盖：多级 import、prefix 前导 `/` 归一化、route-level auth 覆盖 server default、needAuth 覆盖、middleware 顺序、空 request/response、同一路由重复、游离文件、API type 的字段类型/JSON tag，以及 generated `routes.go` 的 method/path/handler 解析。

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/apicontract -run TestLoad -v
```

Expected: compile fail，提示加载函数不存在。

- [ ] **Step 2: 固定 API parser 依赖**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go get github.com/zeromicro/go-zero/tools/goctl@v1.8.4
```

`apicontract` 只复用该固定版本的 parser/AST，不调用其 generator；实际生成仍使用后续锁定的联犀 fork binary。

- [ ] **Step 3: 实现 API import graph 和 effective authorization**

入口固定为：

```text
apisvr: service/apisvr/http/api.api
datasvr: service/datasvr/http/data.api
```

导出：

```go
func LoadBackend(backendRoot string) (map[string]Snapshot, error)
func CompareGolden(actual Snapshot, goldenDir string) error
```

规则：

1. `Defined` 扫描该服务目录全部 `.api`；
2. `Reachable` 只遍历入口 import graph；
3. `Registered` 从各服务实际使用的 `internal/handler/routes.go` 解析，不把未调用的 `datasvr/internal/handler/wsRoutes.go` 计入 REST 注册；
4. `Detached = Defined - Reachable`；
5. authType 优先 route `@doc`，其次 server `defaultAuthType`，最终为 `all`；
6. needAuth 优先 route `isNeedAuth`，其次 server `defaultNeedAuth`，最终 false；
7. operType 优先 route `operType`，否则沿用现 access generator 的 URL 末段规则；
8. 所属 server group 含 `accessGroup` 时 `ManagedByAccess=true`，否则 false；
9. `Types` 保存所有对外 request/response 及其递归引用类型的字段名、规范化类型和完整 tag；
10. source 缺失但历史 `internal/types/types.go` 已生成的引用类型，用 Go AST 补入相同 shape，同时仍在 `UndefinedTypes` 报告其源漂移；shape 比较忽略“来自 API 还是历史 Go 文件”的物理来源；
11. 所有 slices 按 service/method/fullPath/handler 或 type/field declaration order 稳定输出。

- [ ] **Step 4: 实现 snapshot/verify CLI**

接口固定为：

```bash
apicheck snapshot --backend BACKEND_ROOT --update
apicheck verify --backend BACKEND_ROOT --metadata=none
apicheck verify --backend BACKEND_ROOT
apicheck lint --backend BACKEND_ROOT --files comma,separated,relative.api
```

`snapshot` 没有 `--update` 必须拒绝写入；`verify --metadata=none` 比较四类契约和 source closure，但暂不要求七项元数据；无 flag 的 `verify` 还要求全部 metadata 完整。任何 verify/lint 都不得修改 golden。

- [ ] **Step 5: 生成旧基线并核对计数**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/apicontract ./cmd/apicheck
go run ./cmd/apicheck snapshot --backend .. --update
```

人工核对 golden：

```text
apisvr defined=237 reachable=235 registered=235 detached=2
datasvr defined=7 reachable=7 registered=7 detached=0
total defined=244 reachable=242 registered=242 detached=2
```

detached golden 只能是 `/open/oss/upload` 和 `/open/oss/download`。

- [ ] **Step 6: 观察源闭包预期失败并提交基线**

```bash
go run ./cmd/apicheck verify --backend .. --metadata=none
```

Expected: exit 1，只报告 `AppleAppConfig` 未定义；route/auth golden 必须已经匹配。

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add backend/tools/internal/apicontract backend/tools/cmd/apicheck \
  backend/core/service/apisvr/http/testdata \
  backend/core/service/datasvr/http/testdata
git diff --cached --check
git commit -m "冻结 Core HTTP 与授权契约基线"
```

### Task 2：把 AppleAppConfig 补回 `.api` 契约源

**Files:**

- Modify: `backend/core/service/apisvr/http/common.api`
- Verify unchanged: `backend/core/service/apisvr/internal/types/types.go`

- [ ] **Step 1: 添加精确共享类型**

在共享 HTTP types block 中添加：

```api
// Apple 登录配置
AppleAppConfig {
    AppID string `json:"appID,optional"` // Bundle ID / Services ID
    BundleID string `json:"bundleID,optional"` // 前端别名，写入时同步到 appID
    TeamID string `json:"teamID,optional"` // Apple Team ID
    KeyID string `json:"keyID,optional"` // Apple Key ID
    PrivateKey string `json:"privateKey,optional"` // Apple 私钥（PEM 格式）
    RedirectURI string `json:"redirectURI,optional"` // 回调地址
}
```

只定义一次，不复制到 `system/tenant/app.api` 或 `system/tenant/tenant.api`。

- [ ] **Step 2: 验证 source closure 与旧生成类型一致**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go run ./cmd/apicheck verify --backend .. --metadata=none

cd ../core
go test ./service/apisvr/internal/types ./service/apisvr/internal/logic/system/tenant/...
```

Expected: apicheck route/auth/source closure 通过；旧 `types.go` 无需手改。

- [ ] **Step 3: 提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add backend/core/service/apisvr/http/common.api
git diff --cached --check
git commit -m "补齐 Apple 登录 API 契约源"
```

### Task 3：实现七项元数据 lint 规则

**Files:**

- Create: `backend/tools/internal/apicontract/metadata.go`
- Test: `backend/tools/internal/apicontract/metadata_test.go`

- [ ] **Step 1: 写缺失值、非法值和模板描述的失败测试**

每条 route 必须显式具有：`summary`, `description`, `authType`, `resource`, `action`, `group`, `recordLogMode`。测试以下拒绝场景：

- 简写 `@doc "..."`；
- 任一字段缺失；
- description 与 summary 相同、少于 10 个 Unicode rune，或包含 fork 示例占位语句；
- 非法 authType/action/recordLogMode；
- route `group` 与所属 `@server group` 不同；
- metadata 修改导致 effective auth/needAuth/operType golden 变化。

- [ ] **Step 2: 实现固定枚举和赋值规则**

authType 接受 fork 原集合外加兼容值 `supper`：

```text
platform, supper, admin, all, public, device
```

本次 244 条旧路由的显式 authType 必须等于 golden 的 `EffectiveAuthType`，因此不得趁机把旧 `supper` 改为 `platform`，也不得把旧 `all + needAuth=false` 改为 `public`。

action 使用 fork 的有效集合；常见映射固定为：

```text
index/read/tree/count/list/query/detail -> view
create/multi-create -> create
update/multi-update -> update
delete/multi-delete -> delete
multi-import -> import
export/multi-export -> export
send -> send
login -> login
logout -> logout
register -> register
captcha -> captcha
change/forget password -> reset-password
websocket handshake -> connect
debug routes -> debug
```

其他 handler 必须从实际 logic 选 fork 已允许的业务动词，不使用 `get/get-list/get-one/query/detail` 作为 action。

recordLogMode：

```text
1 = 自动：读取不记，写入记；普通 view/CRUD 默认使用
2 = 全记：登录、安全配置、授权、跨租户、批量导入导出和人工处置
3 = 不记：健康/调试、WebSocket 长连接、纯技术探测
```

`resource` 使用稳定的业务对象名，不使用 URL 全路径；`group` 必须复制所属 server group；description 必须依据同名 handler/logic 写明业务效果以及关键范围/约束，禁止 244 条批量套同一句模板。

- [ ] **Step 3: 验证 lint 能看到当前缺口**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/apicontract -run TestMetadata -v
go run ./cmd/apicheck verify --backend ..
```

Expected: tests pass；全量 verify 因 244 条 metadata 尚未完成而失败，并给出第一条带 file/handler/method/path 的错误。

- [ ] **Step 4: 提交 lint**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add backend/tools/internal/apicontract backend/tools/cmd/apicheck
git diff --cached --check
git commit -m "增加 API 元数据完整性校验"
```

## 每批元数据的统一执行方式

对后续每个批次：

1. 把简写/缺失 `@doc` 改成 block；
2. summary 尽量沿用旧文案，仅修正明显错字；
3. 打开对应 `internal/handler` 与 `internal/logic`，description 写实际效果、数据范围和关键副作用；
4. authType 从 authorization golden 回填；
5. resource/action/group/recordLogMode 按 Task 3 规则人工选择；
6. 不改 method/path/handler/request/response/server middleware/import graph；
7. 每批执行 `apicheck lint --files ...` 和 `apicheck verify --metadata=none`；
8. 审核 `git diff --word-diff`，确保只有 `@doc` block。

### Task 4：批次 A——公共、开放与用户会话 40 条

**Files:**

- Modify: `backend/core/service/apisvr/http/system/common.api`（14）
- Modify: `backend/core/service/apisvr/http/system/user/self.api`（24）
- Modify: `backend/core/service/apisvr/http/system/oss.api`（2，仍不可达）

- [ ] **Step 1: 补齐 40 条 metadata**

WebSocket connect、debug、debug-tencent、download-file 和 OSS GET/PUT 保持原 method。`system/oss.api` 只补 metadata，不添加 import。

- [ ] **Step 2: 运行批次 lint 和契约门禁**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go run ./cmd/apicheck lint --backend .. --files \
service/apisvr/http/system/common.api,service/apisvr/http/system/user/self.api,service/apisvr/http/system/oss.api
go run ./cmd/apicheck verify --backend .. --metadata=none
```

- [ ] **Step 3: 提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add backend/core/service/apisvr/http/system/common.api \
  backend/core/service/apisvr/http/system/user/self.api \
  backend/core/service/apisvr/http/system/oss.api
git diff --cached --check
git commit -m "补全公共与用户会话 API 元数据"
```

### Task 5：批次 B——平台、租户与授权 74 条

**Files:**

- Modify: `backend/core/service/apisvr/http/system/access.api`（10）
- Modify: `backend/core/service/apisvr/http/system/app.api`（7）
- Modify: `backend/core/service/apisvr/http/system/module/info.api`（7）
- Modify: `backend/core/service/apisvr/http/system/module/menu.api`（6）
- Modify: `backend/core/service/apisvr/http/system/tenant/app.api`（13）
- Modify: `backend/core/service/apisvr/http/system/tenant/tenant.api`（19）
- Modify: `backend/core/service/apisvr/http/system/role.api`（12）

- [ ] **Step 1: 补齐 74 条 metadata**

所有历史 `supper` 原样回填。授权、角色、租户、模块变更默认 `recordLogMode: "2"`；只读接口按实际敏感度人工复核。

- [ ] **Step 2: 运行批次 lint、契约门禁和相关编译**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go run ./cmd/apicheck lint --backend .. --files \
service/apisvr/http/system/access.api,service/apisvr/http/system/app.api,service/apisvr/http/system/module/info.api,service/apisvr/http/system/module/menu.api,service/apisvr/http/system/tenant/app.api,service/apisvr/http/system/tenant/tenant.api,service/apisvr/http/system/role.api
go run ./cmd/apicheck verify --backend .. --metadata=none

cd ../core
go test ./service/apisvr/internal/logic/system/access/... \
  ./service/apisvr/internal/logic/system/tenant/... \
  ./service/apisvr/internal/logic/system/role/...
```

- [ ] **Step 3: 提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add backend/core/service/apisvr/http/system/{access.api,app.api,role.api} \
  backend/core/service/apisvr/http/system/module \
  backend/core/service/apisvr/http/system/tenant
git diff --cached --check
git commit -m "补全租户与授权 API 元数据"
```

### Task 6：批次 C——组织、用户与数据权限 61 条

**Files:**

- Modify: `backend/core/service/apisvr/http/system/dept.api`（14）
- Modify: `backend/core/service/apisvr/http/system/user/info.api`（5）
- Modify: `backend/core/service/apisvr/http/system/user/dept.api`（2）
- Modify: `backend/core/service/apisvr/http/system/user/role.api`（2）
- Modify: `backend/core/service/apisvr/http/system/user/data.api`（2）
- Modify: `backend/core/service/apisvr/http/system/data.api`（15）
- Modify: `backend/core/service/apisvr/http/system/project.api`（13）
- Modify: `backend/core/service/apisvr/http/system/area.api`（8）

- [ ] **Step 1: 补齐 61 条 metadata**

description 必须区分“组织隶属关系”“角色绑定”“项目/区域数据授权”和“用户申请处置”，不能统一写成“更新权限”。

- [ ] **Step 2: 运行批次 lint 和契约门禁**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go run ./cmd/apicheck lint --backend .. --files \
service/apisvr/http/system/dept.api,service/apisvr/http/system/user/info.api,service/apisvr/http/system/user/dept.api,service/apisvr/http/system/user/role.api,service/apisvr/http/system/user/data.api,service/apisvr/http/system/data.api,service/apisvr/http/system/project.api,service/apisvr/http/system/area.api
go run ./cmd/apicheck verify --backend .. --metadata=none
```

- [ ] **Step 3: 提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add backend/core/service/apisvr/http/system/{dept.api,data.api,project.api,area.api} \
  backend/core/service/apisvr/http/system/user
git diff --cached --check
git commit -m "补全组织与数据权限 API 元数据"
```

### Task 7：批次 D——通知、运维与配置 62 条

**Files:**

- Modify: `backend/core/service/apisvr/http/system/notify/channel.api`（5）
- Modify: `backend/core/service/apisvr/http/system/notify/config.api`（10）
- Modify: `backend/core/service/apisvr/http/system/notify/message.api`（4）
- Modify: `backend/core/service/apisvr/http/system/notify/template.api`（5）
- Modify: `backend/core/service/apisvr/http/system/log.api`（2）
- Modify: `backend/core/service/apisvr/http/system/dict/detail.api`（5）
- Modify: `backend/core/service/apisvr/http/system/dict/info.api`（7）
- Modify: `backend/core/service/apisvr/http/system/slot.api`（5）
- Modify: `backend/core/service/apisvr/http/system/job.api`（13）
- Modify: `backend/core/service/apisvr/http/system/ops/feedback.api`（3）
- Modify: `backend/core/service/apisvr/http/system/ops/workOrder.api`（3）

- [ ] **Step 1: 补齐 62 条 metadata**

通知发送、定时任务执行、工单处置和配置变更明确副作用；日志查询保持 view，不因为对象是日志而设置写操作。

- [ ] **Step 2: 运行批次 lint 和契约门禁**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go run ./cmd/apicheck lint --backend .. --files \
service/apisvr/http/system/notify/channel.api,service/apisvr/http/system/notify/config.api,service/apisvr/http/system/notify/message.api,service/apisvr/http/system/notify/template.api,service/apisvr/http/system/log.api,service/apisvr/http/system/dict/detail.api,service/apisvr/http/system/dict/info.api,service/apisvr/http/system/slot.api,service/apisvr/http/system/job.api,service/apisvr/http/system/ops/feedback.api,service/apisvr/http/system/ops/workOrder.api
go run ./cmd/apicheck verify --backend .. --metadata=none
```

- [ ] **Step 3: 提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add backend/core/service/apisvr/http/system/{notify,log.api,dict,slot.api,job.api,ops}
git diff --cached --check
git commit -m "补全通知与运维 API 元数据"
```

### Task 8：批次 E——datasvr 7 条

**Files:**

- Modify: `backend/core/service/datasvr/http/data.api`（7）

- [ ] **Step 1: 补齐 7 条 metadata**

保持当前 REST 路由注册；不要借此启用或删除历史 `internal/handler/wsRoutes.go`。

- [ ] **Step 2: 运行批次 lint 和全量门禁**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go run ./cmd/apicheck lint --backend .. --files service/datasvr/http/data.api
go run ./cmd/apicheck verify --backend ..
```

Expected:

```text
defined=244 reachable=242 registered=242 detached=2
metadata complete=244
authorization unchanged=242 reachable routes
```

- [ ] **Step 3: 提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add backend/core/service/datasvr/http/data.api
git diff --cached --check
git commit -m "补全数据服务 API 元数据"
```

### Task 9：阶段验收和人工描述审阅

- [ ] **Step 1: 自动契约验收**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/apicontract ./cmd/apicheck
go run ./cmd/apicheck verify --backend ..
```

- [ ] **Step 2: 检查无简写/缺失 metadata**

让 `apicheck verify` 输出按字段计数并断言：

```text
summary=244
description=244
authType=244
resource=244
action=244
group=244
recordLogMode=244
```

- [ ] **Step 3: 人工抽查每个业务文件**

逐个检查 40 个 `.api` 文件：含路由的文件至少抽查一条 read、一条 write（若存在）和一条特殊动作，沿 handler 到 logic 核对 description；纯聚合文件核对 import graph。把审阅结果记录到 `docs/superpowers/evidence/2026-07-23-core-api-metadata-review.md`，列出文件、route 数、reviewer、结论；不得只写总计。

- [ ] **Step 4: Core 编译与源仓保护**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/core
go test -run '^$' ./...
make build

cd ../..
test -z "$(git -C ../core status --porcelain)"
git diff --check
git status --short --branch
```

- [ ] **Step 5: 提交审阅证据**

```bash
git add docs/superpowers/evidence/2026-07-23-core-api-metadata-review.md
git commit -m "记录 Core API 元数据审阅结果"
```

固定 goctl 的 strict validate、Swagger 和 access 临时生成放在下一份工具链计划执行；本阶段不覆盖历史 `http/swagger.json`、`http/access.json` 或 `etc/init/access`。
