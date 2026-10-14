# syssvr 多 Proto 契约拆分实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把单体 `sys.proto` 拆成 `base.proto`、15 个领域 proto 和一个 public-import 聚合文件，同时保持现有 gRPC wire contract、Go protobuf package、运行时注册和旧 client/direct 调用完全兼容。

**Architecture:** 先用旧单体 proto 生成不含 source info 的 descriptor golden，再实现按 package 聚合且忽略文件归属的语义比较器。领域文件只移动定义，不改 package、service、method、message 或 field。PB 只从临时目录按精确 allowlist 切换；现有 goctl client/server/logic/direct 在本阶段不重生成。

**Tech Stack:** Protocol Buffers、protoc 34.1、protoc-gen-go v1.31.0、protoc-gen-go-grpc v1.3.0、Go AST/descriptorpb、gRPC-Go、Go 1.24.4。

---

## 前置条件

- 已完成 `2026-07-23-core-migration-foundation.md`。
- `backend/core` 可执行 `go test -run '^$' ./...`。
- 源单体 proto 位于 `backend/core/service/syssvr/proto/sys.proto`，必须仍与源提交一致。
- 开始前运行：

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
test -z "$(git status --porcelain)"
test -z "$(git -C ../core status --porcelain)"
cmp backend/core/service/syssvr/proto/sys.proto ../core/service/syssvr/proto/sys.proto
```

## 领域归属清单

每个 message 只属于一个文件；名字大小写原样保留，包括 `configResp` 和 `userInfoUpdateReq`。

- `base.proto`（13）：`ThirdAppConfig`, `IDList`, `Attachment`, `Point`, `Empty`, `PageInfo`, `Compare`, `CompareString`, `CompareInt64`, `WithID`, `WithIDCode`, `WithCode`, `WithAppCodeID`。
- `access.proto`（9）：`AuthApiInfo`, `ApiInfoIndexReq`, `ApiInfo`, `ApiInfoIndexResp`, `AccessInfoMultiImportReq`, `AccessInfoMultiImportResp`, `AccessInfoIndexReq`, `AccessInfo`, `AccessInfoIndexResp`。
- `app.proto`（6）：`AppInfo`, `AppInfoIndexReq`, `AppInfoIndexResp`, `AppModuleMultiUpdateReq`, `AppModuleIndexReq`, `AppModuleIndexResp`。
- `area.proto`（9）：`AreaInfo`, `AreaWithID`, `AreaProfile`, `AreaProfileReadReq`, `AreaProfileIndexReq`, `AreaProfileIndexResp`, `AreaInfoReadReq`, `AreaInfoIndexReq`, `AreaInfoIndexResp`。
- `common.proto`（14）：`ServiceInfo`, `SlotInfo`, `WeatherAir`, `WeatherReadReq`, `WeatherReadResp`, `QRCodeReadReq`, `QRCodeReadResp`, `SlotInfoMultiCreateReq`, `SlotInfoIndexReq`, `SlotInfoIndexResp`, `ThirdDeptInfoIndexReq`, `ThirdDeptInfoReadReq`, `Map`, `configResp`。
- `data.proto`（19）：`OpenAccess`, `OpenAccessIndexReq`, `OpenAccessIndexResp`, `DataProject`, `DataProjectDeleteReq`, `DataProjectMultiDeleteReq`, `DataProjectSaveReq`, `DataProjectMultiSaveReq`, `DataProjectIndexReq`, `DataProjectIndexResp`, `DataArea`, `DataAreaMultiUpdateReq`, `DataAreaIndexReq`, `DataAreaIndexResp`, `DataAreaMultiDeleteReq`, `UserAreaApplyIndexReq`, `UserAreaApplyInfo`, `UserAreaApplyIndexResp`, `UserAreaApplyDealReq`。
- `department.proto`（18）：`DeptUserIndexReq`, `DeptUserIndexResp`, `DeptUser`, `DeptUserMultiDeleteReq`, `DeptUserMultiCreateReq`, `DeptRoleIndexReq`, `DeptRoleIndexResp`, `DeptRoleMultiSaveReq`, `DeptSyncJobReadReq`, `DeptSyncJob`, `DeptSyncJobIndexReq`, `DeptSyncJobIndexResp`, `DeptInfoReadReq`, `DeptSyncJobExecuteReq`, `DeptSyncJobExecuteResp`, `DeptInfo`, `DeptInfoIndexReq`, `DeptInfoIndexResp`。
- `dict.proto`（13）：`DictInfo`, `DictInfoReadReq`, `DictInfoIndexReq`, `DictInfoIndexResp`, `DictDetailReadReq`, `DictMultiImportReq`, `DictMultiImportResp`, `DictMultiExportReq`, `DictMultiExportResp`, `DictDetail`, `DictDetailIndexReq`, `DictDetailIndexResp`, `DictDetailMultiCreateReq`。
- `log.proto`（9）：`DateRange`, `LoginLogIndexReq`, `LoginLogInfo`, `LoginLogIndexResp`, `OperLogIndexReq`, `OperLogInfo`, `OperLogIndexResp`, `LoginLogCreateReq`, `OperLogCreateReq`。
- `module.proto`（14）：`ModuleInfo`, `ModuleInfoIndexReq`, `ModuleInfoIndexResp`, `ModuleMultiImportReq`, `ModuleMultiImportResp`, `ModuleMultiExportReq`, `ModuleMultiExportResp`, `MenuMultiImportReq`, `MenuMultiImportResp`, `MenuMultiExportReq`, `MenuMultiExportResp`, `MenuInfoIndexReq`, `MenuInfo`, `MenuInfoIndexResp`。
- `notify.proto`（22）：`SendOption`, `NotifyConfig`, `NotifyConfigIndexReq`, `NotifyConfigIndexResp`, `NotifyTemplate`, `NotifyChannel`, `NotifyChannelIndexReq`, `NotifyChannelIndexResp`, `ThirdEmail`, `ThirdSms`, `NotifyTemplateIndexReq`, `NotifyTemplateIndexResp`, `MessageInfoIndexReq`, `MessageInfoIndexResp`, `MessageInfoSendReq`, `MessageTriggerUser`, `MessageInfo`, `NotifyConfigTemplateDeleteReq`, `NotifyConfigTemplateIndexReq`, `NotifyConfigTemplateIndexResp`, `NotifyConfigTemplate`, `NotifyConfigSendReq`。
- `ops.proto`（6）：`OpsWorkOrder`, `OpsWorkOrderIndexReq`, `OpsWorkOrderIndexResp`, `OpsFeedback`, `OpsFeedbackIndexReq`, `OpsFeedbackIndexResp`。
- `project.proto`（11）：`ProjectCrudIndexReq`, `ProjectCrud`, `ProjectCrudIndexResp`, `ProjectWithID`, `ProjectInfo`, `ProjectProfile`, `ProjectProfileReadReq`, `ProjectProfileIndexReq`, `ProjectProfileIndexResp`, `ProjectInfoIndexReq`, `ProjectInfoIndexResp`。
- `role.proto`（18）：`RoleAccessMultiUpdateReq`, `RoleAccessIndexReq`, `RoleAccessIndexResp`, `RoleMenuIndexReq`, `RoleMenuIndexResp`, `RoleAppIndexReq`, `RoleAppIndexResp`, `RoleModuleIndexReq`, `RoleModuleIndexResp`, `RoleAppMultiUpdateReq`, `RoleModuleMultiUpdateReq`, `RoleMenuMultiUpdateReq`, `RoleApiAuthReq`, `RoleApiAuthResp`, `RoleInfoIndexReq`, `RoleInfoIndexResp`, `RoleInfo`, `RoleAppUpdateReq`。
- `tenant.proto`（31）：`ThirdApp`, `AppleAppConfig`, `TenantAgreement`, `TenantAgreementIndexReq`, `TenantAgreementIndexResp`, `TenantOpenCheckTokenReq`, `TenantOpenCheckTokenResp`, `TenantOpenWebHook`, `TenantInfoCreateReq`, `TenantInfo`, `TenantInfoIndexReq`, `TenantInfoIndexResp`, `TenantConfig`, `TenantConfigRegisterAutoCreateProject`, `TenantConfigRegisterAutoCreateArea`, `TenantAccessMultiSaveReq`, `TenantAccessIndexReq`, `TenantAccessIndexResp`, `TenantAppInfo`, `TenantAppWithIDOrCode`, `TenantAppModule`, `TenantAppIndexReq`, `TenantAppIndexResp`, `TenantAppMultiUpdateReq`, `TenantModuleIndexReq`, `TenantModuleIndexResp`, `TenantAppMenuIndexReq`, `TenantAppMenuIndexResp`, `TenantAppMenu`, `TenantModuleCreateReq`, `TenantModuleWithIDOrCode`。
- `user.proto`（44）：`UserDataProjectIndexReq`, `UserDataProjectIndexResp`, `UserDataAreaIndexReq`, `UserDataAreaIndexResp`, `UserPushClientReportReq`, `UserProfile`, `UserProfileIndexReq`, `UserProfileIndexResp`, `UserMessage`, `UserMessageStatisticsResp`, `UserMessageStatistics`, `UserMessageIndexReq`, `UserMessageIndexResp`, `UserAreaApplyCreateReq`, `TimeRange`, `UserInfo`, `UserCreateResp`, `UserLogoutReq`, `UserCheckTokenReq`, `UserCheckTokenResp`, `ProjectAuth`, `UserInfoCreateReq`, `UserInfoIndexReq`, `UserInfoIndexResp`, `userInfoUpdateReq`, `UserInfoReadReq`, `UserInfoDeleteReq`, `JwtToken`, `UserCaptchaReq`, `UserCaptchaResp`, `UserForgetPwdReq`, `UserCodeToUserIDReq`, `UserCodeToUserIDResp`, `UserChangePwdReq`, `UserLoginReq`, `UserLoginResp`, `UserRegisterReq`, `UserBindAccountReq`, `UserRoleIndexReq`, `UserRoleIndexResp`, `UserRoleMultiUpdateReq`, `UserDeptIndexReq`, `UserDeptIndexResp`, `UserDeptMultiSaveReq`。

总计必须是 256 个顶层 message。

### Task 1：建立 tools module 和 descriptor 语义模型

**Files:**

- Create: `backend/tools/go.mod`
- Create: `backend/tools/go.sum`
- Modify: `backend/go.work`
- Create: `backend/tools/internal/protocompat/contract.go`
- Test: `backend/tools/internal/protocompat/contract_test.go`

- [ ] **Step 1: 创建 tools module**

Create `backend/tools/go.mod`:

```go
module ming-iot/backend/tools

go 1.24.4

require google.golang.org/protobuf v1.36.6
```

Modify `backend/go.work`:

```go
go 1.24.4

use (
	./core
	./tools
)
```

- [ ] **Step 2: 为 descriptor 归一化写失败测试**

`contract_test.go` 必须用 `descriptorpb.FileDescriptorSet` 构造两个等价 fixture：定义分别位于一个文件和多个文件，声明顺序不同。断言 `Build(set, "sys")` 得到完全相等的 `Contract`。

再写表驱动测试覆盖：嵌套 message、嵌套 enum、map entry、oneof、proto3 optional、reserved name/range 和 method streaming flags。

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/protocompat -run TestBuild -v
```

Expected: compile fail，提示 `Build`/`Contract` 尚不存在。

- [ ] **Step 3: 实现按完整名称聚合的 Contract**

`contract.go` 必须导出：

```go
type Contract struct {
	Package          string
	Services         map[string]Service
	Messages         map[string]Message
	TopLevelMessages map[string]struct{}
	Enums            map[string]Enum
}

func Load(path, packageName string) (Contract, error)
func Build(set *descriptorpb.FileDescriptorSet, packageName string) (Contract, error)
```

内部类型必须按以下语义保存：

- `Service`: 完整名称、按 method name 索引的 method；
- `Method`: input/output 完整类型、client/server streaming、确定性序列化后的 options；
- `Message`: 完整名称、按 field number 索引的 field、oneof、reserved names/ranges、options；
- `Field`: name、json_name、number、type、type_name、label、oneof name、proto3_optional、default、map key/value、options；
- `Enum`: 完整名称、name/number 对、reserved names/ranges、options。

`Messages` 递归收集嵌套 message 和 map-entry 供兼容比较；`TopLevelMessages` 只标记文件顶层业务 message，CLI 的“256 messages”使用它计数。明确忽略 file name、file dependency、source info、声明顺序。`FileOptions` 只在入口验证 protobuf package 与 `go_package`，不作为拆分前后相等项。

- [ ] **Step 4: 运行测试并提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go mod tidy
go test ./internal/protocompat -run TestBuild -v

cd ../..
git add backend/go.work backend/tools/go.mod backend/tools/go.sum backend/tools/internal/protocompat
git diff --cached --check
git commit -m "实现 protobuf 契约归一化模型"
```

### Task 2：实现稳定的契约差异比较器和 CLI

**Files:**

- Create: `backend/tools/internal/protocompat/compare.go`
- Test: `backend/tools/internal/protocompat/compare_test.go`
- Create: `backend/tools/cmd/protocompat/main.go`
- Test: `backend/tools/cmd/protocompat/main_test.go`

- [ ] **Step 1: 写逐项变更的失败测试**

表驱动测试必须分别变更以下一项并断言返回稳定、带路径的 diff：

```text
package sys -> sysv2
service sys.ops -> sys.Ops
method input/output type
client_streaming/server_streaming
message deletion
field name/number/type/type_name/label/json_name
oneof membership
map key/value
proto3_optional
reserved name/range
enum name/value number/reserved
service/method/message/field/enum options
```

另断言跨文件移动和声明重排产生空 diff。

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/protocompat -run TestCompare -v
```

Expected: compile fail，提示 `Compare` 尚不存在。

- [ ] **Step 2: 实现 Compare**

Export:

```go
type Difference struct {
	Path string `json:"path"`
	Old  string `json:"old,omitempty"`
	New  string `json:"new,omitempty"`
}

func Compare(oldContract, newContract Contract) []Difference
```

所有 map key 排序后比较，diff 最终按 `Path`、`Old`、`New` 排序。路径格式固定为：

```text
service:sys.ops
service:sys.UserManage/method:userLogin/input
message:sys.UserInfo/field:1/type
enum:sys.Example/value:READY/number
```

- [ ] **Step 3: 实现只读 CLI**

CLI 接口固定为：

```bash
protocompat -old OLD.protoset -new NEW.protoset -package sys
```

无差异时 stdout：

```text
compatible: 15 services, 215 methods, 256 messages
```

有差异时把排序后的 diff 逐行输出到 stderr 并 exit 1；输入损坏或 package 不存在 exit 2。

- [ ] **Step 4: 验证并提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/protocompat ./cmd/protocompat

cd ../..
git add backend/tools/internal/protocompat backend/tools/cmd/protocompat
git diff --cached --check
git commit -m "增加 protobuf 语义兼容门禁"
```

### Task 3：冻结旧 descriptor 和服务总数

**Files:**

- Create: `backend/core/service/syssvr/proto/testdata/sys-v1.protoset`
- Create: `backend/core/service/syssvr/proto/testdata/sys-v1.summary.json`

- [ ] **Step 1: 解析标准 proto include 路径**

本机 protoc 位于 Conda 前缀时，从其相邻 `include` 目录解析标准 proto；每个后续 shell 都重新解析，不能依赖跨命令环境变量：

```bash
protoc_bin="$(command -v protoc)"
protoc_include="$(cd "$(dirname "$protoc_bin")/../include" && pwd)"
test -f "$protoc_include/google/protobuf/wrappers.proto"
test "$(protoc --version)" = "libprotoc 25.3"
```

用于 golden 的 descriptor 不带 `--include_source_info`，避免注释和文件位置进入契约。

- [ ] **Step 2: 从未拆分 sys.proto 生成 golden**

```bash
protoc_bin="$(command -v protoc)"
protoc_include="$(cd "$(dirname "$protoc_bin")/../include" && pwd)"
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/core/service/syssvr/proto
mkdir -p testdata
"$protoc_bin" -I . -I "$protoc_include" \
  --include_imports \
  --descriptor_set_out=testdata/sys-v1.protoset \
  sys.proto
```

- [ ] **Step 3: 用 protocompat 输出并保存确定性摘要**

为 CLI 增加 `-summary-json` 参数，将 package、service/method/message/enum 数和完整服务名写成排序 JSON。生成 `sys-v1.summary.json` 后断言：

```text
package = sys
services = 15
methods = 215
messages = 256
service sys.ops exists
```

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go run ./cmd/protocompat \
  -old ../core/service/syssvr/proto/testdata/sys-v1.protoset \
  -new ../core/service/syssvr/proto/testdata/sys-v1.protoset \
  -package sys \
  -summary-json ../core/service/syssvr/proto/testdata/sys-v1.summary.json
```

Expected: compatibility 行包含 15/215/256。

- [ ] **Step 4: 提交不可变基线**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add backend/core/service/syssvr/proto/testdata backend/tools/cmd/protocompat
git diff --cached --check
git commit -m "冻结 syssvr RPC 契约基线"
```

### Task 4：按领域物理拆分 proto，暂不替换 PB

**Files:**

- Create: `backend/core/service/syssvr/proto/base.proto`
- Create: `backend/core/service/syssvr/proto/{access,app,area,common,data,department,dict,log,module,notify,ops,project,role,tenant,user}.proto`
- Replace: `backend/core/service/syssvr/proto/sys.proto`

- [ ] **Step 1: 为每个文件创建一致头部**

所有文件必须从以下内容开始：

```proto
syntax = "proto3";

package sys;
option go_package = "pb/sys";
```

`base.proto` 不声明 service。每个领域 proto 恰好声明一个同名旧 service：

| 文件 | service | RPC 数 |
| --- | --- | ---: |
| access | `AccessManage` | 9 |
| app | `AppManage` | 7 |
| area | `AreaManage` | 8 |
| common | `Common` | 13 |
| data | `DataManage` | 15 |
| department | `DepartmentManage` | 17 |
| dict | `DictManage` | 13 |
| log | `Log` | 4 |
| module | `ModuleManage` | 13 |
| notify | `NotifyManage` | 23 |
| ops | `ops` | 6 |
| project | `ProjectManage` | 13 |
| role | `RoleManage` | 13 |
| tenant | `TenantManage` | 31 |
| user | `UserManage` | 30 |

从旧文件机械移动 service 和“领域归属清单”中的 message；字段、编号、注释、options、RPC 名和声明大小写一律不改。

- [ ] **Step 2: 写入显式无环 imports**

所有领域文件直接 import `base.proto`。另外只允许：

```text
department.proto -> role.proto
common.proto     -> department.proto
project.proto    -> area.proto
user.proto       -> data.proto, department.proto, notify.proto, role.proto
tenant.proto     -> user.proto, module.proto
```

以下文件直接 import `google/protobuf/wrappers.proto`：`tenant`, `project`, `area`, `user`, `dict`, `department`, `module`, `app`, `role`。不得依赖传递 import。

- [ ] **Step 3: 把 sys.proto 改成纯聚合文件**

除统一头部外，`sys.proto` 只包含以下 public imports，按此稳定顺序：

```proto
import public "base.proto";
import public "access.proto";
import public "app.proto";
import public "area.proto";
import public "common.proto";
import public "data.proto";
import public "department.proto";
import public "dict.proto";
import public "log.proto";
import public "module.proto";
import public "notify.proto";
import public "ops.proto";
import public "project.proto";
import public "role.proto";
import public "tenant.proto";
import public "user.proto";
```

- [ ] **Step 4: 在同一个 shell 中做语法检查和语义比较**

```bash
set -euo pipefail
tmp_set="$(mktemp)"
trap 'rm -f "$tmp_set"' EXIT
protoc_bin="$(command -v protoc)"
protoc_include="$(cd "$(dirname "$protoc_bin")/../include" && pwd)"
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/core/service/syssvr/proto
"$protoc_bin" -I . -I "$protoc_include" \
  --include_imports \
  --descriptor_set_out="$tmp_set" \
  base.proto access.proto app.proto area.proto common.proto data.proto \
  department.proto dict.proto log.proto module.proto notify.proto ops.proto \
  project.proto role.proto tenant.proto user.proto sys.proto
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go run ./cmd/protocompat \
  -old ../core/service/syssvr/proto/testdata/sys-v1.protoset \
  -new "$tmp_set" \
  -package sys
```

Expected: `compatible: 15 services, 215 methods, 256 messages`。任何 diff 都必须回到 proto 修正；不得放宽比较器来消除真实差异。

- [ ] **Step 5: 提交纯契约拆分**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add backend/core/service/syssvr/proto/*.proto
git diff --cached --check
git commit -m "按领域拆分 syssvr proto"
```

此提交不包含 PB、client、server、logic 或 direct 变化。

### Task 5：抽取并验证 15 个运行时服务注册

**Files:**

- Modify: `backend/core/service/syssvr/sysdirect/sysdirect.go`
- Test: `backend/core/service/syssvr/sysdirect/registration_test.go`

- [ ] **Step 1: 写失败的注册集合测试**

测试创建 `grpc.NewServer()`，调用尚不存在的 `registerServers(server, nil)`，再比较 `server.GetServiceInfo()` 的 key 与以下精确集合：

```go
[]string{
	"sys.AccessManage", "sys.AppManage", "sys.AreaManage", "sys.Common",
	"sys.DataManage", "sys.DepartmentManage", "sys.DictManage", "sys.Log",
	"sys.ModuleManage", "sys.NotifyManage", "sys.ProjectManage", "sys.RoleManage",
	"sys.TenantManage", "sys.UserManage", "sys.ops",
}
```

排序后比较，并单独断言存在 `sys.ops`、不存在 `sys.Ops`。

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/core
go test ./service/syssvr/sysdirect -run TestRegisterServers -v
```

Expected: compile fail，提示 `registerServers` 不存在。

- [ ] **Step 2: 原样抽取注册函数**

Add:

```go
func registerServers(grpcServer grpc.ServiceRegistrar, svcCtx *svc.ServiceContext) {
	// 原 Run 中的 15 条 sys.Register*Server 调用按原样移到这里。
}
```

在 `Run` 的 `services.MustNewServer` callback 中调用 `registerServers(grpcServer, svcCtx)`，然后保持 `reflection.Register(grpcServer)`。不要调整注册顺序、interceptor 或 startup。

- [ ] **Step 3: 验证并提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/core
go test ./service/syssvr/sysdirect -run TestRegisterServers -v
go test ./service/syssvr/internal/server/... ./service/syssvr/client/...

cd ../..
git add backend/core/service/syssvr/sysdirect
git diff --cached --check
git commit -m "校验 syssvr 服务注册完整性"
```

### Task 6：固定 protobuf 三件套并在临时目录生成拆分 PB

**Files:**

- Create: `backend/tools/protobuf-tools.lock`
- Create: `backend/tools/scripts/install-protobuf-tools.sh`
- Create: `backend/tools/scripts/generate-syspb.sh`
- Create: `backend/tools/scripts/generate-syspb_test.sh`
- Modify: `backend/tools/go.mod`
- Modify: `backend/tools/go.sum`
- Create: `backend/tools/internal/coretxn/transaction.go`
- Create: `backend/tools/internal/coretxn/transaction_linux.go`
- Create: `backend/tools/internal/coretxn/transaction_unsupported.go`
- Test: `backend/tools/internal/coretxn/transaction_test.go`
- Replace generated: `backend/core/service/syssvr/pb/sys/*.pb.go`

- [ ] **Step 1: 记录并校验工具版本**

`protobuf-tools.lock` 固定：

```yaml
goVersion: go1.24.4
target:
  goos: linux
  goarch: amd64
  cgoEnabled: false
protoc:
  version: "34.1"
  archive: https://github.com/protocolbuffers/protobuf/releases/download/v34.1/protoc-34.1-linux-x86_64.zip
protocGenGo:
  version: "v1.31.0"
  module: google.golang.org/protobuf/cmd/protoc-gen-go
protocGenGoGrpc:
  version: "v1.3.0"
  module: google.golang.org/grpc/cmd/protoc-gen-go-grpc
```

安装脚本必须：

1. 把可执行文件安装为 `backend/bin/protobuf-tools/{protoc,protoc-gen-go,protoc-gen-go-grpc}`，标准 proto 放在 `backend/bin/protobuf-tools/include/google/protobuf/`；
2. 下载官方 `https://github.com/protocolbuffers/protobuf/releases/download/v34.1/protoc-34.1-linux-x86_64.zip`；
3. 用 `GOTOOLCHAIN=go1.24.4 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go install -trimpath -ldflags=-buildid= google.golang.org/protobuf/cmd/protoc-gen-go@v1.31.0`；
4. 用 `GOTOOLCHAIN=go1.24.4 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go install -trimpath -ldflags=-buildid= google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0`；
5. 打印并验证三个 `--version`；
6. 提供且只提供 `bootstrap` 与 `install` 两个 mode；
7. `bootstrap` 在 checksum 尚未写入时构建工具，stdout 只按固定顺序输出 archive、protoc 和两个 plugin binary 的四行 canonical SHA-256，日志全部写 stderr；
8. `install` 要求四个 checksum 齐全并逐一验证，缺少或不符必须失败，且只校验、不静默更新。

首次可信 bootstrap 必须执行两次并比较：

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
bash backend/tools/scripts/install-protobuf-tools.sh bootstrap > /tmp/protobuf-tools-1.sha256
bash backend/tools/scripts/install-protobuf-tools.sh bootstrap > /tmp/protobuf-tools-2.sha256
diff -u /tmp/protobuf-tools-1.sha256 /tmp/protobuf-tools-2.sha256
```

人工审核后，把四个值分别写入 lock 的 `protoc.archiveSha256`、`protoc.binarySha256`、`protocGenGo.binarySha256`、`protocGenGoGrpc.binarySha256`，再运行：

```bash
bash backend/tools/scripts/install-protobuf-tools.sh install
```

脚本不得使用系统中当前的 protoc 25.3、protoc-gen-go 1.36.6 或 grpc plugin 1.6.1 代替。

- [ ] **Step 2: 先实现可复用的整棵 Core 原子事务**

在 tools module 固定 `golang.org/x/sys v0.33.0`；`coretxn` 的唯一 live target 是仓库内 `backend/core`，事务目录固定在同级 `backend/.coretxn/`。实现协议：

1. 对 live Core 建立包含文件路径、mode、内容 SHA-256 的稳定指纹；
2. 在 `.coretxn/<id>/candidate` 构造完整 Core 副本，拒绝 symlink、special file 和越出 Core 的路径；
3. 校验 candidate 与 live 的 diff 只落在调用方给出的精确 allowlist；
4. candidate 的所有 Go 命令显式设置 `GOWORK=off`，避免向上发现只包含 `./core` 的 `backend/go.work`；
5. 在 commit 前获取 `.coretxn/lock` 独占锁，重新计算 live 指纹并拒绝并发漂移；
6. journal 用 `prepare -> exchange -> committed` 状态，每次落盘后 fsync journal 和父目录；
7. 预探测同文件系统和 `unix.Renameat2(..., unix.RENAME_EXCHANGE)` 支持，再以一次目录 exchange 原子交换 `backend/core` 与 candidate；不支持时在修改 live 前失败，禁止逐文件 fallback；
8. exchange 是唯一 commit point。之后即使删除旧 tree 或 journal 失败，也返回 committed + cleanup warning，不把已提交事务报告成失败；下一次运行先按 journal 和新旧指纹完成幂等清理。

测试至少覆盖：candidate 校验失败、live 指纹漂移、lock 冲突、exchange 注入失败时 live 零变化；成功交换时读者只能看到完整旧树或完整新树；exchange 后 cleanup 注入失败仍报告 committed，下一次 recovery 只清理旧树；candidate 中 `GOWORK=off go test -run '^$' ./...` 不受父 workspace 影响。

Run red then green:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go get golang.org/x/sys@v0.33.0
go test ./internal/coretxn -run TestCommit -v
go test ./internal/coretxn -v
```

- [ ] **Step 3: 写生成脚本的失败保护测试**

`generate-syspb.sh` 必须在以下情况退出且不改 live Core：版本不符、descriptor 不兼容、缺少预期文件、出现 allowlist 外文件、任一生成命令失败、candidate 编译失败或 `coretxn` commit 前失败。用 before/after Core 指纹证明失败路径零变化。

`generate-syspb_test.sh` 为自包含 test runner：创建最小临时 Core fixture，以 fake protoc/protocompat/coretxn 逐项注入上述错误，且每个 case 都比较 live fixture 的路径/mode/content SHA-256；最后用真实 `coretxn` 测一次完整 candidate commit。Run：

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
bash backend/tools/scripts/generate-syspb_test.sh
```

预期文件集合固定为：

```text
base.pb.go
access.pb.go access_grpc.pb.go
app.pb.go app_grpc.pb.go
area.pb.go area_grpc.pb.go
common.pb.go common_grpc.pb.go
data.pb.go data_grpc.pb.go
department.pb.go department_grpc.pb.go
dict.pb.go dict_grpc.pb.go
log.pb.go log_grpc.pb.go
module.pb.go module_grpc.pb.go
notify.pb.go notify_grpc.pb.go
ops.pb.go ops_grpc.pb.go
project.pb.go project_grpc.pb.go
role.pb.go role_grpc.pb.go
tenant.pb.go tenant_grpc.pb.go
user.pb.go user_grpc.pb.go
sys.pb.go
```

旧 `sys_grpc.pb.go` 不在新集合内，只能在新集合全部生成、descriptor compare 通过之后精确删除。

- [ ] **Step 4: 实现 candidate 生成、预同步编译和原子提交**

脚本必须执行：

```text
在 backend/.coretxn/<id>/candidate 复制完整 live Core
在 candidate 中显式列出 17 个 proto 输入运行 protoc
生成临时 FileDescriptorSet
调用 protocompat 比较旧 golden
比较实际生成 basename 与固定 allowlist
只在 candidate/pb/sys 中安装 32 个预期文件并精确删除 sys_grpc.pb.go
gofmt candidate 生成目录
在 candidate 中以 GOWORK=off 运行 focused tests 和 go test -run '^$' ./...
由 coretxn 复核仅 pb/sys 固定集合变化，再原子交换整棵 backend/core
```

禁止对 live `pb/sys` 逐文件写入，禁止 `rm -rf`、glob delete 或 `rsync --delete`。candidate 内可重建生成目录，但同步 commit 必须走 `coretxn`。

- [ ] **Step 5: 运行生成并验证旧调用方**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
test -z "$(rg -n 'File_sys_proto' backend/core --glob '!service/syssvr/pb/sys/*.pb.go')"
bash backend/tools/scripts/install-protobuf-tools.sh install
bash backend/tools/scripts/generate-syspb.sh

cd backend/core
go test ./service/syssvr/client/... ./service/syssvr/internal/server/... ./service/syssvr/sysdirect
go build -o /dev/null ./service/syssvr
```

Expected: 编译通过；server 继续嵌入对应 `Unimplemented*Server`；完整 gRPC method path 不变。

- [ ] **Step 6: 提交事务工具、工具锁和 PB 切换**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add backend/tools/go.mod backend/tools/go.sum \
  backend/tools/protobuf-tools.lock backend/tools/scripts \
  backend/tools/internal/coretxn \
  backend/core/service/syssvr/pb/sys
git diff --cached --check
git commit -m "切换 syssvr 多 proto 生成物"
```

### Task 7：冻结现有 client 公开 Go API 面

**Files:**

- Create: `backend/tools/internal/clientcontract/contract.go`
- Test: `backend/tools/internal/clientcontract/contract_test.go`
- Create: `backend/tools/cmd/clientcontract/main.go`
- Create: `backend/core/service/syssvr/client/testdata/public-api.json`

- [ ] **Step 1: 写 Go AST 公开契约提取测试**

测试 fixture 同时覆盖括号 type block、单条 type alias、公开 interface、公开 constructor、普通私有定义和非 `sys` selector。契约 JSON 包含 `aliases`、`interfaces`、`functions` 三个有序集合；alias 项为：

```go
type Alias struct {
	Package string `json:"package"`
	Name    string `json:"name"`
	Target  string `json:"target"`
}
```

按 package/name/target 排序。interface 保存导出方法及完整参数/返回语法；function 保存 `New<Service>`、`NewDirect<Service>` 的完整签名。忽略私有实现 struct 和 method body。

- [ ] **Step 2: 实现 snapshot/verify CLI**

接口：

```bash
clientcontract snapshot --root CLIENT_DIR --out PUBLIC_API_GOLDEN
clientcontract verify --root CLIENT_DIR --golden PUBLIC_API_GOLDEN
```

`snapshot` 只有显式调用才写文件；`verify` 只读，缺失/新增/改向均 exit 1。

- [ ] **Step 3: 生成并核对实际旧公开面**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/clientcontract ./cmd/clientcontract
go run ./cmd/clientcontract snapshot \
  --root ../core/service/syssvr/client \
  --out ../core/service/syssvr/client/testdata/public-api.json
go run ./cmd/clientcontract verify \
  --root ../core/service/syssvr/client \
  --golden ../core/service/syssvr/client/testdata/public-api.json
```

Expected: 14 个 client package 各 254 个 alias，`usermanage` 255 个，总计 3811；同时冻结 15 个导出 client interface 及其 `New`/`NewDirect` constructors，不虚构“每包 256”。

- [ ] **Step 4: 提交 client contract golden**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add backend/tools/internal/clientcontract backend/tools/cmd/clientcontract \
  backend/core/service/syssvr/client/testdata/public-api.json
git diff --cached --check
git commit -m "冻结 syssvr client 公开 API 契约"
```

### Task 8：阶段验收

- [ ] **Step 1: 契约与工具测试**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/protocompat ./cmd/protocompat \
  ./internal/clientcontract ./cmd/clientcontract ./internal/coretxn
go run ./cmd/clientcontract verify \
  --root ../core/service/syssvr/client \
  --golden ../core/service/syssvr/client/testdata/public-api.json
```

- [ ] **Step 2: Core focused tests 和五入口构建**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/core
go test ./service/syssvr/client/... ./service/syssvr/internal/server/... ./service/syssvr/sysdirect
go test -run '^$' ./...
make build
```

- [ ] **Step 3: 重新生成到临时目录并确认无工作区差异**

从 clean worktree 运行：

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
bash backend/tools/scripts/generate-syspb.sh
git diff --exit-code
test -z "$(git status --porcelain)"
test -z "$(git -C ../core status --porcelain)"
```

Expected: 所有命令 exit 0。此时现有 client/server/logic/direct 仍未由 goctl 重生成，可以进入 HTTP 契约计划。
