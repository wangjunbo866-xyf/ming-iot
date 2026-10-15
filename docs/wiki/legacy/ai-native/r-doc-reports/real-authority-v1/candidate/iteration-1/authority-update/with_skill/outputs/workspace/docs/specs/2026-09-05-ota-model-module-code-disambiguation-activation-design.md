---
document_kind: spec
status: approved
delivery_issue: ming-iot#IKDFC5
code_repository: git:gitee.com/ming-industry/ming-iot
related_documents:
  - proposals/2026-09-04-ota-model-module-code-disambiguation.md
  - reviews/2026-09-04-ota-model-module-code-disambiguation-review.md
  - plans/2026-09-05-ota-model-module-code-disambiguation.md
---

# OTA 固件模块编码与物模型模块编码消歧实施规格

## 1. 状态与授权边界

本规格把已获批准的方案 C 转化为可验证的产品实施合同，交付身份为 `ming-iot#IKDFC5`。

Owner 已于 2026-09-05 通过 LoopX User Gate `todo_d191f4c30c23` 批准本规格，并授权据此编写实施计划。当前状态为 `approved`。

本次批准只建立实施合同和计划依据，不授权产品代码实现。实施授权形成前：

- 不创建产品代码实现 Todo；
- 不修改 `ming-iot` 产品代码或生成物；
- 不执行开发数据库重建；
- 不把研发文档 PR #3 视为产品实现 PR；
- 不授权合并、发布或生产操作。

## 2. 已确认前提

Owner 已确认项目尚未发布：没有生产数据库、已发布客户端、依赖旧字段的设备固件或第三方调用方。因此本次采用首次发布前一次性收敛：

- 不提供旧字段别名；
- 不提供双写或读优先级；
- 不提供弃用窗口；
- 不编写生产数据迁移；
- 开发数据库按新结构重建。

任一前提在实施前发生变化，当前规格立即失效，必须重新设计兼容与迁移方案。

## 3. 实施基线与事实范围

规格核查基线为产品仓 `origin/master@a6b28035c577131c1a4b0262af6ddbdc393104c5`。实施开始时必须重新拉取 `origin/master`：

- 若精确 Head 未变化，可按本规格建立实现 worktree；
- 若 Head 已变化，先重跑字段清单与生成契约扫描；
- 只有相关契约面未发生冲突时，才把新的精确 Head 记录为实现基线；
- 发现新消费者或兼容事实时，先修订规格，不得静默扩大实现范围。

当前基线同时存在两个不同领域的无修饰 `ModuleCode` / `moduleCode` / `module_code`：

1. OTA 固件与设备版本域；
2. 产品物模型模块与 Model Release 快照域。

此外，`backend/things/service/apisvr/http/things/device/moduleVersion.api` 把固件模块编码错误声明为 `int64`，而 Proto 与 PostgreSQL 模型为 `string`。

## 4. 唯一目标词汇

### 4.1 OTA 固件与设备版本域

| 表面 | 唯一名称 | 类型 |
|---|---|---|
| Go 字段 | `FirmwareModuleCode` | `string` |
| Proto / JSON | `firmwareModuleCode` | `string` |
| PostgreSQL 列 | `firmware_module_code` | `varchar` |
| 默认常量 | `FirmwareModuleCodeDefault` | `string` |

### 4.2 产品物模型与 Model Release 域

| 表面 | 唯一名称 | 类型 |
|---|---|---|
| Go 字段 | `ModelModuleCode` | `string` |
| Proto / JSON | `modelModuleCode` | `string` |
| PostgreSQL 列 | `model_module_code` | `varchar` |

`DmModelModule.ID` / `ModelModuleID` 仍是物模型模块稳定身份。`ModelModuleCode` 是租户内可读业务编码，不能替代 ID。OTA 固件模块编码与物模型模块身份之间不得新增映射。

## 5. 必须同步的源定义

### 5.1 OTA 固件与设备版本

实现至少覆盖以下活跃源定义及其调用者：

- `backend/things/service/dmsvr/internal/repo/relationDB/model.go` 中的 `DmDeviceModuleVersion`；
- `backend/things/service/dmsvr/internal/repo/relationDB/deviceModuleVersion.go` 中的过滤器与查询；
- `backend/things/service/dmsvr/internal/repo/relationDB/modelOta.go` 中的 `DmOtaFirmwareInfo`；
- `backend/things/service/dmsvr/proto/deviceManage.proto` 的设备固件版本消息；
- `backend/things/service/dmsvr/proto/otaManage.proto` 的固件创建与更新消息；
- `backend/things/service/apisvr/http/things/device/moduleVersion.api`；
- `backend/things/service/apisvr/http/things/ota/firmware.api`；
- `backend/things/share/domain/deviceMsg/msgOta/ota.go` 的默认常量；
- `devicemanage`、`otamanage` 与 HTTP adapter 中消费上述字段的逻辑。

Proto 字段编号保持原值：`deviceManage.proto` 中三处字段继续使用编号 4，`otaManage.proto` 中两处字段继续使用编号 16 和 14。保留编号只用于减少无关 wire 漂移，不构成旧字段兼容承诺。

### 5.2 产品物模型与 Model Release

实现至少覆盖以下活跃源定义及其调用者：

- `backend/things/service/dmsvr/internal/repo/relationDB/modelReleaseModel.go` 中的 `DmModelModule`；
- `backend/things/service/dmsvr/internal/repo/relationDB/modelModule.go` 中的过滤器、保留前缀、错误文本与 SQL 列名；
- `backend/things/service/dmsvr/proto/productManage.proto` 的 Model Module 消息；
- `backend/things/service/apisvr/http/things/product/modelModule.api`；
- `backend/things/share/domain/schema/bundle.go` 中的 `ModuleDraft`、`PropertyRoute`、编译与校验逻辑；
- `productmanage`、HTTP adapter、产品草稿组合器与发布仓储中消费上述字段的逻辑；
- Model Release 单元、仓储与集成测试夹具。

物模型模块基础编码的业务值与 `base_` 保留前缀不改变；只把承载该值的字段、变量、错误文本和数据库列改成带 `Model` 限定的名称。

## 6. PostgreSQL 合同

开发数据库中的列一次性改为：

- `dm_device_module_version.firmware_module_code`；
- `dm_ota_firmware_info.firmware_module_code`；
- `dm_module_info.model_module_code`。

实现须同步 GORM tag、显式 SQL 条件、更新 map、唯一约束所绑定的字段和相关测试。现有表名、主键、租户隔离、软删除语义与业务唯一性不改变。

本交付不依赖 GORM `AutoMigrate` 猜测完成旧列重命名。验收使用空的开发数据库按新模型建表；已有开发数据允许删除后重建。生产数据库迁移脚本明确不在范围内。

## 7. API、Proto 与生成物合同

源定义是唯一修改入口：

- RPC 修改 `.proto`；
- HTTP 修改 `.api`；
- 不手工编辑可再生 PB、client、server、handler、types、route、Swagger 或 API golden。

生成必须使用项目锁定 wrapper：

```bash
make protobuf-tools

go -C backend/tools run ./cmd/rpcgen generate \
  --manifest ../things/service/dmsvr/proto/rpcgen.yaml \
  --module-root ../things \
  --protobuf-tools ../bin/protobuf-tools

go -C backend/tools run ./cmd/apicheck generate \
  --backend .. \
  --goctl ../bin/goctl
```

若锁定的 `backend/bin/goctl` 尚不存在，须先按 `backend/tools/README.md` 的固定 goctl 流程构建，不得改用系统 `goctl`。生成完成后必须再运行 `rpcgen check` 与 `apicheck verify`，并证明第二次生成不会产生额外 diff。

## 8. SchemaBundle 与哈希合同

`SchemaBundle.propertyRoutes` 和编译输入中的 JSON 字段由 `moduleCode` 改为 `modelModuleCode`。这是公开快照结构的一次性预期变化，因此：

- `PropertyRoute.ModuleCode`、`ModuleDraft.ModuleCode` 及校验内部变量统一改为 `ModelModuleCode`；
- canonical JSON 中只允许 `modelModuleCode`；
- 排序规则仍以 `ModuleID` 为主、`ModelModuleCode` 为次，Property Route 仍按 identifier 排序；
- `schemaVersion`、TSL `version:"1.0"`、属性/事件/动作内容和产品级事件/动作边界不改变；
- 更新 `schema_bundle_v1.golden.json` 与对应 SHA-256；
- Evidence 同时记录旧哈希、新哈希及产生位移的唯一原因，不把预期位移误报为随机漂移。

本次不读取或迁移已发布快照，因为已确认不存在生产发布数据。

## 9. 测试先行与机器门禁

实施计划必须先建立会失败的聚焦合同，再修改实现。至少包含：

1. 源契约扫描：两个领域的新名称存在，纳入范围的旧无修饰名称归零；
2. 类型合同：HTTP、Proto、Go 与数据库字段均为 `string`；
3. Proto 合同：字段编号未变化；
4. PostgreSQL 合同：新列可在空数据库建表，查询与更新只使用新列；
5. SchemaBundle 合同：canonical JSON、golden 和 SHA-256 精确匹配；
6. 领域边界合同：不存在 OTA 编码到 `ModelModuleID` 的映射；
7. 生成合同：固定 wrapper 可重复生成，`rpcgen check` 与 `apicheck verify` 通过。

实现完成后至少运行：

```bash
make verify-fast VERIFY_SCOPE=things
make verify-pr
```

只有涉及真实外部依赖的行为发生变化时才追加显式 Integration Profile；本次纯命名与开发数据库重建不要求启动完整 Things 基线。任何既有失败必须如实记录并关联独立 Issue，不能通过缩小本交付验收范围伪装成通过。

## 10. Review、Evidence 与 PR

- 实现 Agent 与独立 Review Agent 必须不同；
- Review 绑定产品代码候选的完整 40 位 Head；
- Review 核查字段映射、旧名归零、Proto 编号、生成物、数据库列与 SchemaBundle 哈希；
- 原始机器 Evidence 留在 LoopX 私有 Evidence 目录；
- 脱敏摘要写入 `ming-iot-rd-docs/evidence/`，完整人读 Review 写入 `ming-iot-rd-docs/reviews/`；
- 产品代码与研发文档分别创建 Draft PR，并互相引用 `ming-iot#IKDFC5` 和精确 Head；
- 当前研发文档 PR #3 只承载 Proposal、Proposal Review、实施规格及后续计划，不是产品实现 PR。

## 11. 验收标准

1. OTA 活跃契约只使用 `FirmwareModuleCode` / `firmwareModuleCode` / `firmware_module_code`；
2. 产品物模型活跃契约只使用 `ModelModuleCode` / `modelModuleCode` / `model_module_code`；
3. 纳入范围的旧无修饰名称归零，无别名、双写、兼容分支或跨域通用类型；
4. `moduleVersion.api`、Proto、Go 与 PostgreSQL 的对应字段均为 `string`；
5. Proto 字段编号保持不变；
6. `ModelModuleID` 仍是物模型模块稳定身份，两种模块之间没有新增身份映射；
7. 空开发数据库按新模型建表成功，不需要生产迁移脚本；
8. 固定 RPC/API 生成流程可重复执行，生成后无意外漂移；
9. SchemaBundle golden 与哈希只发生规格声明的确定性位移；
10. `make verify-fast VERIFY_SCOPE=things`、`make verify-pr` 与独立 Review 通过。

## 12. 明确非目标

- 不改变 OTA、升级任务、设备版本、产品物模型或 Model Release 的业务行为；
- 不重构现有模块生命周期；
- 不改变租户隔离模型；
- 不引入新的第三方组件；
- 不启动完整 Things 依赖栈作为本次默认门禁；
- 不执行 PR 合并、版本发布或生产操作。

## 13. 审批结果与实施激活边界

User Gate `todo_d191f4c30c23` 已授权：

1. 将本规格状态改为 `approved`；
2. 编写绑定 `ming-iot#IKDFC5` 的实施 Plan；
3. 更新 Proposal、Proposal Review、`DOC_REGISTRY.yaml` 与研发文档 Draft PR 的追踪信息。

该 Gate 不授权修改产品代码、建立产品实现提交、合并 PR、发布或生产操作。实施 Plan 发布后，产品实现必须作为一个有界 Agent Todo 单独取得适用授权；Plan 内部 Task 不逐项创建 User Gate。

## 14. 历史实现快照

本节为 2026-10-12 在隔离父仓进行的历史回放补记，交付身份仍为 `ming-iot#IKDFC5`，产品仓身份为 `git:gitee.com/ming-industry/ming-iot`。本规格仍是该交付的正式业务意图入口，[既有实施计划](../plans/2026-09-05-ota-model-module-code-disambiguation.md)描述历史执行安排。原有正文、frontmatter（包括 `status: approved`）、第 3 节旧基线和第 1、13 节审批边界均保留原文，属于当时的历史上下文；本节不追溯改写授权，也不把文档状态解释为实现或合并授权。

本次只读 Git 核对得到：`repos/ming-iot` 实际检出的完整 40 位 HEAD 为 `e38d162feba9445c3bedb6ec78d96ee79dee9a49`，与父仓 gitlink 固定版本一致，产品工作树无改动。此 HEAD 与下述历史实施 Review、Evidence 的 `exact_head` 一致；原规格中的 `a6b28035c577131c1a4b0262af6ddbdc393104c5` 是实施前核查基线，不是本次实际检出版本。

在上述 HEAD 的在场源码中，接口字段如下（本次为静态读取，未发起接口调用）：

| 在场接口或结构 | 实际 JSON 字段 | 类型与源码依据 |
| --- | --- | --- |
| 设备版本 `/api/v1/things/device/version/index`、`/read` 的请求及 `DeviceModuleVersion` 响应项 | `firmwareModuleCode` | `string`；[moduleVersion.api](../../repos/ming-iot/backend/things/service/apisvr/http/things/device/moduleVersion.api) 第 37–60 行，三处均声明 `FirmwareModuleCode string`，JSON tag 为 `json:"firmwareModuleCode,optional"`。原规格记录的 `int64` 属于旧基线。 |
| OTA 固件创建请求 `FirmwareCreateReq` 及详情结构 `FirmwareInfo` | `firmwareModuleCode` | `string`；[firmware.api](../../repos/ming-iot/backend/things/service/apisvr/http/things/ota/firmware.api) 中创建请求带 `optional`，详情结构为 `json:"firmwareModuleCode"`。 |
| 物模型模块信息、创建、更新及查询请求 | `modelModuleCode` | `string`；[modelModule.api](../../repos/ming-iot/backend/things/service/apisvr/http/things/product/modelModule.api) 第 169–192 行，查询请求带 `optional`。 |
| SchemaBundle 的 `PropertyRoute` 及编译输入 `ModuleDraft` | `modelModuleCode` | `string`；[bundle.go](../../repos/ming-iot/backend/things/share/domain/schema/bundle.go) 第 41–60 行，均使用 `ModelModuleCode string` 与 `json:"modelModuleCode"`。 |

其中设备版本接口的连续原文（`moduleVersion.api` 第 41 行）为：

```go
        FirmwareModuleCode string `json:"firmwareModuleCode,optional"`
```

既有独立评审是 [2026-09-05 实施 Review](../reviews/2026-09-05-ota-model-module-code-disambiguation-implementation-review.md)，配对证据为 [2026-09-05 Evidence](../evidence/2026-09-05-ota-model-module-code-disambiguation/evidence.md)。两份 frontmatter 均记录 `delivery_issue: ming-iot#IKDFC5` 与 `exact_head: e38d162feba9445c3bedb6ec78d96ee79dee9a49`。Review 第 14–18 行记录评审者 `ming-review-claude` 为非实现 Agent 的独立评审 lane；第 50 行原文为：

> **verdict = PASS**（0 blocker / 0 warning / 2 note）

历史 PASS 的范围是该精确候选和当时记录的检查：Review 第 28–31 行及 Evidence 第 25–30 行记录 `make verify-pr`、`make verify-fast VERIFY_SCOPE=things`、模块编码七项合同及旧无修饰名扫描通过；Review 第 35–39 行记录命名边界、Proto 字段号、PostgreSQL 列与类型契约、SchemaBundle 哈希位移和生成物核验。上述结果均为历史记录，本次没有重跑，也不代表当前环境、其他版本、完整依赖栈或生产环境通过。

历史 PASS 保留 DSN 限制：Review 第 45 行 N1 明确验收 #7（空开发库按新模型建表）与 PostgreSQL 集成级测试未在当时评审环境重跑，需受控 DSN；第 52 行将 #7 列为依契约测试与实施证据采信。Evidence 第 35 行连续原文为：

> Note：空库建表验收采信契约测试+实施证据（DSN 级未重跑）；提案评审 4 条补全 note 全部被采纳。

因此不能将历史契约测试 PASS 写成 DSN 级建表或集成实测 PASS；本次也未读取 DSN、追取原始机器证据或核验其所指实施建表结果。上述链接仅连接现有历史 Review/Evidence，不产生新的独立评审结论。

本次仅维护本规格、规格索引并生成维护报告，未运行产品测试，未新做独立评审，未新增实现，未改变 Git HEAD/index，也未新增实现、合并、发布或生产授权。历史文档中的操作命令与审批条件保留作追溯依据，不作为本次执行指令。
