# Core 迁移与 goctl 生成链设计

状态：已确认
日期：2026-07-23

## 1. 背景

本项目将联犀开源版 `core` 的源码迁移到 `ming-iot/backend/core`，并采用联犀文章所描述的工程组织原则：Go Workspace、稳定的 API/RPC 服务骨架、一个 RPC 运行单元内按业务域拆分多个 `.proto`，以及 gRPC/direct 两种调用方式共用同一套业务实现。

源仓库与目标仓库当前都是独立 Git 仓库：

- 源仓库：`/home/wangjunbo/workspace-iot-platform/core`，基线提交 `1b3110e5`；
- 目标仓库：`/home/wangjunbo/workspace-iot-platform/ming-iot`，迁移前只有空 README；
- 源 Go module：`gitee.com/unitedrhino/core`，Go 1.24.4；
- 源运行时依赖官方 `github.com/zeromicro/go-zero v1.8.4`；
- 定制 goctl 是开发期生成工具，不等同于运行时 go-zero 依赖。

联犀公开 goctl fork 当前基线固定为：

- 仓库：<https://gitee.com/unitedrhino/go-zero>；
- commit：`6579a20bfd96919a80af5a9cb4e12f03598d4bb3`；
- 禁止根据其仍然写死的 `1.9.2-1` 版本字符串判断工具身份。

参考资料：

- [联犀：为什么选择 go-zero](https://doc.unitedrhino.com/blog/tech-arch-02/)；
- [联犀公开版 sys.proto](https://gitee.com/unitedrhino/core/blob/master/service/syssvr/proto/sys.proto)；
- [联犀 goctl fork](https://gitee.com/unitedrhino/go-zero)。

## 2. 目标

本设计必须同时达到以下目标：

1. 把完整、可编译的 core module 迁入 `ming-iot/backend/core`；
2. 保留现有 API、RPC、direct client 和独立部署能力；
3. 将 2297 行的 `sys.proto` 按领域拆成多个文件，降低人和 AI 的无效上下文；
4. 保持现有 gRPC wire contract 完全兼容；
5. 保持现有 HTTP method、path、handler、请求和响应契约兼容；
6. 补齐 API 的业务描述和权限元数据，使 `.api` 成为可信契约源；
7. 建立固定版本、临时目录生成、语义比较和幂等校验组成的安全生成闭环；
8. 保留 AGPL-3.0 许可证、来源和上游基线信息，不带入真实密钥。

## 3. 非目标

本次迁移明确不做：

- 不修改 `gitee.com/unitedrhino/core` module path；
- 不迁移或内嵌外部 `gitee.com/unitedrhino/share` 仓库；
- 不把 `service/timed` 重命名或合并成 `timedsvr`；
- 不实现 `backend/cmd/allInOne`，但保留 `coreExport`、`sysdirect` 等组合入口；
- 不改变任何 protobuf package、service、method、message 或字段编号；
- 不把一个领域 proto 误拆成一个独立部署进程；
- 不在本次迁移中把权限字符串 `supper` 改名为 `platform`；
- 不顺带升级运行时 go-zero；
- 不创建文章中存在、但当前源码没有业务内容的 `checkIn.proto`、`mall.proto` 等空文件；
- 不直接追踪联犀 goctl 的移动 `master`。
- 不迁入依赖联犀私有基础镜像和远程前端下载的旧 Dockerfile；容器化在可编译迁移基线稳定后单独设计。

## 4. 核心决策

### 4.1 一个运行单元、多个领域 proto

“一个 RPC 服务多个 proto”在本项目中的含义是：

- `syssvr` 是一个可独立部署的 RPC 运行单元；
- `syssvr` 内注册 15 个现有 protobuf service；
- 每个现有 protobuf service 初始对应一个领域 proto；
- 所有领域 proto 生成到同一个 Go package `pb/sys`；
- 远程 gRPC 与进程内 direct 调用继续复用相同 server 实现。

它不意味着把 `syssvr` 拆成 15 个进程或 15 个 Go module。

### 4.2 兼容优先、分阶段收敛

迁移时立即完成 proto 物理拆分，但不在同一个步骤中使用尚不稳定的 goctl master 覆盖全部生成层：

1. 先建立旧契约快照；
2. 拆分 proto，并只在语义兼容检查通过后替换 PB；
3. 保留现有 client/server/logic/direct 作为首个可运行基线；
4. 建立项目级多 proto 编排器；
5. 在临时目录验证修订后的 goctl 能够重生成；
6. 只有生成结果通过契约、编译和幂等门禁后，才允许同步到工作区。

### 4.3 运行时与生成器独立升级

运行时 go-zero 与 goctl fork 是两个独立版本轴：

- 本次继续使用源仓库的运行时依赖版本；
- goctl 由 URL、完整 commit SHA、项目补丁集和构建校验值共同锁定；
- 不因升级生成器而顺带升级运行时框架。

## 5. 目标目录

```text
ming-iot/
├── backend/
│   ├── go.work
│   ├── core/
│   │   ├── go.mod
│   │   ├── go.sum
│   │   ├── LICENSE
│   │   ├── NOTICE
│   │   ├── service/
│   │   │   ├── apisvr/
│   │   │   ├── syssvr/
│   │   │   ├── datasvr/
│   │   │   └── timed/
│   │   └── share/
│   ├── tools/
│   │   ├── go.mod
│   │   ├── goctl.lock
│   │   ├── patches/goctl/
│   │   └── cmd/rpcgen/
│   └── bin/                    # Git 忽略的构建产物
├── docs/
└── README.md
```

约束如下：

- `backend/core` 保持为一个 Go module。direct client 会引用服务内的 `internal/svc`，把服务拆成多个 module 会违反 Go internal 可见性规则；
- `backend/go.work` 初期只纳入已经存在的本地 module；
- `backend/tools` 是独立开发工具 module，不进入业务二进制；
- `backend/cmd` 和 AllInOne 在有独立设计前不创建虚假实现；
- 构建产物只能进入 `backend/bin`，不得占用未来作为源码组合入口的 `backend/cmd`。

## 6. proto 拆分

### 6.1 文件布局

```text
backend/core/service/syssvr/proto/
├── sys.proto           # 兼容聚合文件，只做 public import
├── base.proto          # 跨域叶类型，不含 service
├── access.proto        # AccessManage
├── app.proto           # AppManage
├── area.proto          # AreaManage
├── common.proto        # Common
├── data.proto          # DataManage
├── department.proto    # DepartmentManage
├── dict.proto          # DictManage
├── log.proto           # Log
├── module.proto        # ModuleManage
├── notify.proto        # NotifyManage
├── ops.proto           # ops
├── project.proto       # ProjectManage
├── role.proto          # RoleManage
├── tenant.proto        # TenantManage
└── user.proto          # UserManage
```

全部文件继续声明：

```proto
package sys;
option go_package = "pb/sys";
```

`sys.proto` 是兼容导航入口，不是单文件代码生成入口。代码生成必须读取显式 manifest 中的全部输入文件。

### 6.2 base.proto 边界

`base.proto` 仅拥有 13 个无业务归属、被多个领域复用的叶类型：

- `ThirdAppConfig`；
- `IDList`；
- `Attachment`；
- `Point`；
- `Empty`；
- `PageInfo`；
- `Compare`；
- `CompareString`；
- `CompareInt64`；
- `WithID`；
- `WithIDCode`；
- `WithCode`；
- `WithAppCodeID`。

`common.proto` 不能同时承担共享基础类型。`Common` RPC 会依赖部门领域，而部门领域也需要 `Empty`、`PageInfo` 等类型；混在一个文件会形成 import cycle。

### 6.3 领域归属

| proto | protobuf service | RPC 数 | message 数 |
| --- | --- | ---: | ---: |
| `access.proto` | `AccessManage` | 9 | 9 |
| `app.proto` | `AppManage` | 7 | 6 |
| `area.proto` | `AreaManage` | 8 | 9 |
| `common.proto` | `Common` | 13 | 14 |
| `data.proto` | `DataManage` | 15 | 19 |
| `department.proto` | `DepartmentManage` | 17 | 18 |
| `dict.proto` | `DictManage` | 13 | 13 |
| `log.proto` | `Log` | 4 | 9 |
| `module.proto` | `ModuleManage` | 13 | 14 |
| `notify.proto` | `NotifyManage` | 23 | 22 |
| `ops.proto` | `ops` | 6 | 6 |
| `project.proto` | `ProjectManage` | 13 | 11 |
| `role.proto` | `RoleManage` | 13 | 18 |
| `tenant.proto` | `TenantManage` | 31 | 31 |
| `user.proto` | `UserManage` | 30 | 44 |

合计必须保持 15 个 service、215 个 RPC 和 256 个顶层 message。`ops` 必须保持小写，否则完整 gRPC 方法路径会改变。

跨域实体归权威领域。例如 `UserInfo` 属于 user、`RoleInfo` 属于 role、`DeptInfo` 属于 department、`DataProject` 属于 data、`MessageInfo` 属于 notify。使用方通过显式 import 引用，不把领域实体下沉到 `base.proto`。

### 6.4 import DAG

依赖关系必须无环：

```text
base
├── role
├── module
├── area
├── data
├── notify
├── department ──> role
├── common ──────> department
├── project ─────> area
├── user ────────> data + department + notify + role
└── tenant ──────> user + module
```

使用 `google.protobuf` wrapper 的文件必须直接 import，不能依赖传递性 import。

## 7. gRPC 兼容契约

新旧 descriptor 必须进行语义比较，而不是比较二进制或生成文件文本。比较器至少覆盖：

- protobuf package；
- service 完整名称；
- method 名称、输入和输出完整类型名；
- client/server streaming 标志；
- message 完整名称；
- 字段名称、编号、类型、label、JSON name；
- oneof、map、enum 和 reserved 信息；
- 影响 wire 或生成 API 的 options。

允许变化：

- 定义所在的 proto 文件名；
- descriptor 的文件归属；
- 生成 PB 文件的物理拆分；
- generated source 注释中的源文件名。

不允许变化：

- `/sys.<Service>/<Method>` 完整路径；
- protobuf type 完整名称；
- 字段编号、wire type 或 streaming 语义；
- 当前服务注册集合。

`File_sys_proto` 的 descriptor 内容必然因物理拆分而变化，它不属于承诺的 gRPC wire contract。仓内必须确认不存在依赖其内容的代码；聚合 `sys.proto` 用于保留导航入口和对应 Go 文件符号。

## 8. HTTP API 契约

### 8.1 兼容边界

迁移不得改变：

- HTTP method；
- 完整 path；
- handler；
- 请求和响应类型；
- 中间件顺序；
- 对外状态码与业务错误语义。

现有契约定义中的上传、下载、WebSocket 和调试相关 6 条非 POST 路由作为显式例外保留，其中 `system/oss.api` 的 2 条仍保持不可达，实际运行时注册的是 4 条 GET。WebSocket 握手和下载接口不能为了形式统一而强行改成 POST。当前 `/debug`、`/debug-tencent`、`/download-file` 由 `coreExport` 无条件注册，并非只在开发环境开放；本次以 route golden 原样保持这一事实，不趁迁移静默加环境门禁。调试接口会回显 headers/body，生产暴露收敛必须另开安全 ADR 和兼容变更实施。

### 8.2 契约补全

40 个 `.api` 文件继续按领域组织，顶层 `api.api` 聚合 `common.api` 和领域文件。源码一共定义 244 条路由，但当前只有 242 条可达并注册：`system/oss.api` 没有被任何聚合文件 import，其中 2 条上传、下载路由属于历史游离契约。本次同时冻结“defined=244”和“registered=242”，不得为了凑数擅自导入 `system/oss.api`、新增运行时接口。244 条已定义路由都必须显式填写：

- `summary`；
- `description`；
- `authType`；
- `resource`；
- `action`；
- `group`；
- `recordLogMode`。

`description` 必须根据 handler、logic 和类型定义描述业务用途、关键行为和约束，禁止批量填入同一空泛模板。

生成物中已有、但 `.api` 源文件缺失的 `AppleAppConfig` 必须补回契约源。补齐后，`.api` 应能独立解析，不得依赖历史生成物掩盖源定义缺失。

### 8.3 supper 决策

本次保留旧值 `supper` 及其既有语义：系统级超级管理员、根租户、允许跨租户操作。原因是本次目标是兼容迁移，而不是权限词汇重构。

- `.api`、历史 access JSON 和运行时转换器继续接受 `supper`；
- 项目补丁使定制 goctl 校验器接受该值；
- 文档明确其系统级含义，避免与租户管理员混淆；
- 不引入 `supper -> platform` 数据迁移或双写；
- 将来的权限枚举重构必须通过独立 ADR 处理。

## 9. 生成代码所有权

| 类别 | 文件 | 所有权规则 |
| --- | --- | --- |
| 契约源 | `.api`、领域 `.proto`、manifest | 人工维护，必须通过校验 |
| 完全生成 | API `internal/types`、`routes.go`、PB 和 gRPC PB | 禁止手改，可整体重建 |
| 脚手架后转人工 | API/RPC handler、logic | 首次生成；已有文件永不覆盖 |
| goctl 生成 | RPC client、server、direct client | 可重建，但只允许从临时目录同步 |
| 应用级人工代码 | main、config、svc、startup、`sysdirect`、`coreExport` | 不允许逐 proto goctl 覆盖 |
| 项目生成 | compatibility aliases、service registry 校验/产物 | 由 `rpcgen` 依据 manifest 维护 |

现有巨型 `sys.proto` 使每个 client package 意外导出全部 message alias。迁移首期保留现有 client；允许重生成前，`rpcgen` 必须生成 compatibility alias，以免现有 Go 调用方因 proto 拆分而丢失公开类型别名。此兼容层未来只能在显式主版本变更中收缩。

## 10. 项目级生成闭环

### 10.1 manifest

`backend/tools/cmd/rpcgen` 读取显式 manifest，不使用 glob 推断领域和顺序。示意：

```yaml
runtime: syssvr
package: sys
goPackage: pb/sys
aggregate: sys.proto
shared:
  - base.proto
services:
  - proto: access.proto
    service: AccessManage
  - proto: app.proto
    service: AppManage
  - proto: area.proto
    service: AreaManage
  - proto: common.proto
    service: Common
  - proto: data.proto
    service: DataManage
  - proto: department.proto
    service: DepartmentManage
  - proto: dict.proto
    service: DictManage
  - proto: log.proto
    service: Log
  - proto: module.proto
    service: ModuleManage
  - proto: notify.proto
    service: NotifyManage
  - proto: ops.proto
    service: ops
  - proto: project.proto
    service: ProjectManage
  - proto: role.proto
    service: RoleManage
  - proto: tenant.proto
    service: TenantManage
  - proto: user.proto
    service: UserManage
```

manifest 校验必须拒绝：

- package 或 go_package 不一致；
- 聚合文件没有 public import manifest 中的全部 shared/领域 proto；
- 同名 service 重复；
- 领域 proto 没有或包含多个 service；
- manifest service 与 descriptor 不一致；
- import cycle；
- 实际 service 集合与运行时注册集合不一致。

### 10.2 安全生成流程

所有生成都在临时目录进行：

1. 校验工具版本和 checksum；
2. 一次性对 manifest 全部 proto 运行 protoc；
3. 按 manifest 稳定顺序对每个领域 proto 调用固定 goctl；
4. 为 imported RPC 类型和旧公开 Go API 生成 compatibility aliases；
5. 生成或验证 15 个 service 的聚合注册；
6. 执行 descriptor、HTTP 路由、权限、编译和生成路径 allowlist 校验；
7. 在与 `backend/core` 同一文件系统中构造完整 Core candidate，只允许生成物 allowlist 产生差异，并在 candidate 内完成编译；
8. 全部成功后，由项目 `coretxn` 用 Linux `renameat2(RENAME_EXCHANGE)` 原子交换整个 `backend/core` 与 candidate；交换前必须持锁并复核 live tree 指纹，禁止逐文件提交；
9. 交换前任一步失败都丢弃 candidate，工作区保持不变；交换是唯一 commit point，交换后的清理失败只记录为可恢复告警，不把已提交事务误报为失败。

`coretxn` 的 staging/journal 位于被 Git 忽略的 `backend/.coretxn/`。每次运行先恢复或清理上次事务遗留；若当前内核或文件系统不支持同文件系统目录交换，则在改动 live tree 前直接失败，不降级为多文件覆盖。这样 PB、RPC 与 API 生成共享同一套批量事务语义，不会留下“前几个文件已更新、后几个文件仍是旧版”的半同步状态。

所有 protoc/goctl/Go 校验子进程都显式使用 `GOWORK=off`。固定 fork 会在发现上级 workspace 时尝试执行 `go work use .`；关闭 workspace 发现既保证 candidate 按自身 `go.mod` 编译，也防止生成失败时意外改写 live `backend/go.work`。

禁止开发者直接在业务目录执行 goctl。该约束用于避免当前 fork 对共享文件首次生成后不聚合、强制覆盖 client/server，以及 WebSocket route 误删等问题。

### 10.3 goctl 补丁集

项目不复制整个 go-zero fork。`backend/tools/goctl.lock` 固定上游 URL 和完整 SHA，`backend/tools/patches/goctl` 保存小型、可审计补丁：

- 允许既有 `supper` API 元数据；
- 修复 `api new` 模板缺少强制元数据；
- 修复 WebSocket routes 构建后未写回的问题；
- 保留并验证 unary/streaming direct client；
- 修复生成器自身与严格校验不一致的测试 fixture。

多 proto 的应用级编排由项目 `rpcgen` 负责，而不是假设公开 goctl 的 `-m` 支持多输入。`-m` 仍只表示单个 proto 内按 protobuf service 分组目录。

## 11. 迁移阶段

### 阶段 0：冻结基线

- 记录源 commit、工具版本和 Git 状态；
- 生成旧 gRPC descriptor 语义快照；
- 分别生成 244 条已定义 HTTP route 与 242 条可达、已注册 route/handler/type 快照；
- 记录 15 个 service 注册集合；
- 记录源仓已知编译和契约校验失败；
- 审计 `.env`、配置凭据和许可证冲突。

### 阶段 1：迁移模块骨架

- 迁移所有必需 tracked source；
- 排除 `.git`、`.env`、二进制、日志、缓存和 `node_modules`；
- 建立 `backend/go.work`；
- 保持 module path；
- 修复 `.gitignore`，确保 `go.sum` 被跟踪；
- 修复 Makefile，构建全部现有入口并输出到 `backend/bin`；
- 保留 AGPL-3.0 LICENSE，新增 NOTICE；
- 将真实凭据替换为环境变量或示例占位符。

### 阶段 2：拆分 RPC 契约

- 生成旧 descriptor 基线；
- 新建 base 和 15 个领域 proto；
- 创建兼容聚合 `sys.proto`；
- 运行 import DAG 与 descriptor 语义比较；
- 使用固定插件版本生成拆分 PB；
- 保留现有 client/server/logic/direct 和应用级注册；
- 编译验证全部旧调用方。

### 阶段 3：补全 API 契约

- 修复 `AppleAppConfig` 源定义；
- 为 244 条路由补齐七项元数据；
- 生成 OpenAPI 和 access 到临时/独立生成目录；
- 比较 HTTP 路由快照和权限语义；
- 不自动覆盖历史 `etc/init/access`，审核通过后再升级初始化数据。

### 阶段 4：闭合生成工具链

- 建立 tools module、goctl lock 和补丁集；
- 实现 manifest 校验和临时目录生成；
- 建立 compatibility alias；
- 建立 service 注册完整性检查；
- 验证 client/server/direct 可重生成；
- 连续执行两次生成，第二次必须无 Git diff。

### 阶段 5：最终验证

- 构建 5 个现有 main 入口；
- 运行 core 与 tools 的单元/契约测试；
- 验证 gRPC/direct 接口一致；
- 验证 HTTP 路由和授权决策不变；
- 验证无真实密钥；
- 确认原 core 仓库仍然干净。

## 12. 错误处理与回退

- 原 core 仓库在整个迁移中只读；
- 每个阶段独立提交，不把机械迁移、契约拆分、API 描述和生成器修复混为一个 commit；
- descriptor 不一致时，在替换 PB 之前失败；
- route snapshot 不一致时，在替换 API 生成物之前失败；
- 工具版本或 checksum 不匹配时禁止生成；
- 临时生成失败时不允许同步任何部分产物；
- 删除陈旧生成文件只能依据 manifest 计算出的精确 allowlist，禁止对宽目录递归清理；
- 如果源仓已有失败，必须在基线记录中区分“迁移前已存在”和“迁移引入”；最终目标仓仍需达到本设计的验收标准。

## 13. 验收标准

迁移完成必须满足：

1. `ming-iot/backend/core` 是完整、单一、可编译的 core module；
2. `backend/go.work` 能正确解析 core 与 tools；
3. 5 个现有服务入口全部构建成功；
4. 15 个 service、215 个 RPC、256 个 message 语义保持一致；
5. 244 条已定义 HTTP 路由以及 242 条可达、已注册路由的 method/path/handler/request/response 集合分别保持一致，对外 type 的字段名、类型和 tag 保持一致；
6. 每条 API 都有准确、非模板化的七项元数据；
7. `supper` 权限语义没有变化；
8. direct 和 gRPC client 继续实现相同接口；
9. 运行时注册集合与 manifest 完全一致；
10. `go test ./...` 通过，需外部基础设施的测试已明确分层；
11. 生成命令幂等，第二次运行 Git 零变化；
12. 仓库无真实密钥、`.env` 和构建产物；
13. LICENSE、NOTICE 和上游基线信息完整；
14. 原 core 工作树保持干净。

## 14. 提交边界

实施时至少拆为以下原子提交：

1. 建立 workspace 并迁移 core 骨架；
2. 修复构建、配置和许可证边界；
3. 拆分 syssvr proto 并加入兼容测试；
4. 补全 API 契约和 HTTP 快照测试；
5. 加入固定 goctl 与 rpcgen 生成闭环；
6. 完成全量构建与验收修复。

任何提交不得同时包含无关架构重构。
