# OTA 固件模块编码与物模型模块编码消歧提案

> 日期：2026-09-04
>
> Goal：`ming-iot-framework-v1`
>
> Delivery Issue：`ming-iot#IKDFC5`
>
> 状态：方案 C 与实施规格已获用户批准，正式 Delivery Issue 和实施计划已建立；当前不授权产品代码修改或合并
>
> 来源：Model Release PR #2 合并后独立处置实施 Review 的 W3、Owner 对项目发布状态的补充确认，以及 Gitee Delivery Issue `ming-iot#IKDFC5`

## 1. 结论

`ming-iot` 中现有两套 `module_code` / `moduleCode` 表示不同领域概念：

- OTA 与设备版本域表示 **OTA 固件模块编码**；
- Model Release 与产品物模型域表示 **物模型模块编码**。

项目当前没有生产数据库、已发布客户端、设备固件或第三方调用方，因此采用方案 C，在首次发布前直接收敛公开契约：

| 领域 | Go 字段 | Proto / JSON 字段 | PostgreSQL 列 |
|---|---|---|---|
| OTA / 设备版本 | `FirmwareModuleCode` | `firmwareModuleCode` | `firmware_module_code` |
| 产品物模型 | `ModelModuleCode` | `modelModuleCode` | `model_module_code` |

不保留旧 `ModuleCode`、`moduleCode`、`module_code` 别名或兼容层。开发数据库按新结构重建，不为尚未发布的旧结构设计生产迁移路径。

## 2. 采用方案 C 的前提

Owner 已明确确认：

1. 当前是全新项目；
2. 没有生产数据库和必须保留的生产数据；
3. 没有已发布客户端；
4. 没有依赖旧字段名的设备固件；
5. 没有依赖旧字段名的第三方调用方。

因此，旧字段只是开发阶段遗留实现，不是需要长期兼容的已发布产品契约。现在直接改正命名，比为不存在的调用方长期维护兼容层更简单、更清晰。

一旦任一前提发生变化，实施必须暂停并重新评估迁移与兼容策略，不能继续按本提案假设执行。

## 3. 问题背景与已核实事实

核查基于产品仓 `origin/master@a6b28035c577131c1a4b0262af6ddbdc393104c5`。

### 3.1 OTA 固件模块域

- `dm_device_module_version.module_code` 随 `product_id`、`device_name` 和 `version` 保存设备固件版本事实；
- `DmDeviceModuleVersion.ModuleCode` 当前数据库模型注释为“固件名称”；
- `deviceManage.proto` 的 `DeviceModuleVersion*` 消息使用 `string moduleCode`；
- `otaManage.proto` 的固件信息与任务消息使用 `string moduleCode`；
- 该编码服务于固件包、升级任务和设备版本生命周期，不属于产品物模型发布快照。

### 3.2 物模型模块域

- `dm_module_info.module_code` 是租户内可读、可搜索的物模型模块业务编码；
- `DmModelModule.ID` / `ModelModuleID` 是物模型模块的稳定身份；
- RPC 类型使用 `ModelModule`，HTTP 资源使用 `/model-module`；
- 物模型模块只组织 Property，并在产品发布时编译进不可变 `SchemaBundle`；
- Bundle 中的 `moduleId` 与 `moduleCode` 只描述 Property 的物模型模块路由。

### 3.3 同步修复的类型错误

`backend/things/service/apisvr/http/things/device/moduleVersion.api` 当前把 `DeviceModuleVersion*` 的 `ModuleCode` 声明为 `int64`，而 Proto、数据库模型和仓储过滤器使用 `string`。

方案 C 实施时将其一并收敛为 `string firmwareModuleCode`。这是同一字段契约的类型纠正，不再单独保留一个兼容性债务；实现规格仍须用生成结果和测试证明 HTTP、Proto、数据库三层类型一致。

## 4. 目标领域契约

### 4.1 OTA / 设备版本

- 中文规范术语：OTA 固件模块、OTA 固件模块编码；
- 英文规范术语：OTA Firmware Module、Firmware Module Code；
- 公开字段统一为 `firmwareModuleCode`；
- Go 字段统一为 `FirmwareModuleCode`；
- 数据库列统一为 `firmware_module_code`；
- 字段类型统一为 `string`；
- 其身份和生命周期继续由现有 OTA 契约决定，不与 `ModelModuleID` 建立关系。

### 4.2 产品物模型

- 中文规范术语：物模型模块、物模型模块编码；
- 英文规范术语：Model Module、Model Module Code；
- 公开字段统一为 `modelModuleCode`；
- Go 字段统一为 `ModelModuleCode`；
- 数据库列统一为 `model_module_code`；
- 字段类型统一为 `string`；
- `DmModelModule.ID` / `ModelModuleID` 继续作为稳定身份；`ModelModuleCode` 只是租户内可读的业务编码，不能代替 ID。

### 4.3 禁止关系

1. 不得用 OTA 固件模块编码查询、绑定或标识 `ModelModule`；
2. 不得用 `ModelModuleID` 或物模型模块编码定位设备固件版本；
3. 不得新建跨领域通用 `ModuleCode` 类型掩盖差异；
4. 跨领域文档、注释、错误信息和变量名不得使用无修饰的“模块编码”。

## 5. 首次发布前收敛规则

实施必须一次性同步以下契约面：

1. PostgreSQL 模型、查询条件、索引定义和开发初始化结构；
2. Proto 源文件、字段名及对应 JSON name；
3. HTTP API 源文件、请求和响应字段；
4. Go 类型、仓储、服务层、转换逻辑与测试夹具；
5. 通过项目固定生成器产生的 Go、HTTP、RPC、Swagger 等生成物；
6. 维护文档、错误信息和可观察字段说明。

约束：

- Proto 字段类型保持 `string`；字段编号原则上保持不变，以减少无意义的 wire 变化，但本项目不因此承诺旧客户端兼容；
- 不手工修改可再生文件，必须修改源定义后使用固定生成流程重建；
- 不新增旧字段别名、双写逻辑、优先级规则或弃用窗口；
- 开发数据库允许删除并按新 schema 重建；本提案不设计生产迁移脚本；
- 不借命名收敛改变 OTA 或 Model Release 的业务行为。

## 6. 候选方案比较

### A. 保留旧字段，仅做领域标注（拒绝）

短期改动小，但会把 `moduleCode` 的歧义永久留在数据库、Proto、HTTP 和 Go 接口中。对尚未发布的新项目而言，这是一笔没有兼容收益的长期认知债务。

### B. 增加新别名字段并废弃旧字段（拒绝）

会引入双字段一致性、序列化优先级、双写和废弃窗口，而当前没有任何已发布调用方需要这层复杂度。

### C. 首次发布前全面收敛（采用）

直接建立清晰、唯一的领域语言，避免以后每个维护者都依赖类型上下文猜测 `moduleCode` 的含义。代价是开发阶段需要同步修改并重新生成相关表面，但没有生产迁移和外部兼容成本。

## 7. 后续实施边界

正式实施以 Gitee Delivery Issue `ming-iot#IKDFC5` 为稳定交付身份，并以已批准规格 `specs/2026-09-05-ota-model-module-code-disambiguation-activation-design.md` 和实施计划 `plans/2026-09-05-ota-model-module-code-disambiguation.md` 为执行依据。至少核查以下源定义与消费者：

```text
backend/things/service/dmsvr/internal/repo/relationDB/model.go
backend/things/service/dmsvr/internal/repo/relationDB/deviceModuleVersion.go
backend/things/service/dmsvr/proto/deviceManage.proto
backend/things/service/dmsvr/proto/otaManage.proto
backend/things/service/apisvr/http/things/device/moduleVersion.api
相关仓储、服务层、生成物、Swagger 与聚焦测试
```

实施规格必须：

1. 列出 OTA 与 Model 两个领域的全部字段定义和调用点；
2. 明确固定生成器、生成命令与生成物归属；
3. 明确开发数据库重建步骤；
4. 为旧名称消失、新名称一致、类型一致和领域不串联建立机器检查；
5. 将代码实现与独立 Review 交给不同 Agent。

## 8. 验收候选

1. 活跃产品源码中，OTA 契约只暴露 `FirmwareModuleCode` / `firmwareModuleCode` / `firmware_module_code`；
2. 活跃产品源码中，物模型契约只暴露 `ModelModuleCode` / `modelModuleCode` / `model_module_code`；
3. 旧无修饰字段名在本次纳入范围内归零，不存在别名、双写或兼容分支；
4. `moduleVersion.api`、Proto 和数据库模型统一使用 `string`；
5. `ModelModuleID` 仍是物模型模块稳定身份，两种模块之间没有新增身份映射；
6. 固定代码生成流程可重复执行，生成后工作树无意外漂移；
7. OTA 与 Model Release 聚焦测试、适用的 PR Gate 和契约扫描通过；
8. 独立 Review 明确确认命名、类型、生成物和领域边界均已收敛。

## 9. Non-goals

- 不改变 OTA 模块、固件包或升级任务的业务模型；
- 不改变物模型模块、产品绑定或 Model Release 的业务模型；
- 不统一两套模块 ID；
- 不为尚未存在的生产数据设计迁移方案；
- 不保留旧字段兼容层；
- 不因 Proposal 获批而自动修改产品代码；
- 不因 Proposal 获批而授权 commit、merge 或 push。

## 10. 激活条件

只有同时满足以下条件，Planner 才能创建产品实现 Todo：

1. 创建 Gitee Delivery Issue 并明确代码仓交付边界；
2. 用户批准实施规格，而非只批准本 Proposal；
3. Spec 固定字段映射、类型、生成流程、数据库重建与验收合同；
4. Plan 将实现组织为一个有界 Agent Todo 内的多个 Task，不为每个 Plan Task 建立 User Gate；
5. 独立 Review 审核精确候选提交，不由实现 Agent 自审代替；
6. 产品代码修改、commit、merge 和 push 继续服从各自适用的项目规则与精确 Gate。

当前激活进度：第 1～4 项已完成。第 5 项将在产品候选形成后由独立 Review Agent 执行，第 6 项在整个交付期间持续生效。User Gate `todo_d191f4c30c23` 只批准规格和计划，不授权产品实现；产品实现仍须取得单独的有界 Agent Todo 授权。
