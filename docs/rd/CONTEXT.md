# Ming IoT 研发交付术语

本文统一 `ming-iot`、`ming-iot-rd-docs`、Gitee 与本地 LoopX 之间的协作语言。它只定义概念边界，不代替具体规格、计划或验证命令。

## 核心术语

### Delivery Issue（交付 Issue）

Gitee 中跨成员、跨电脑共享的可执行工作项。一个 Delivery Issue 对应一个可独立验收的交付结果和一个主要代码 PR；Plan 内部 Task 不单独创建 Issue。
_不要混同：_ RFC、Plan Task、Agent Todo。

### RFC（架构提案）

用于讨论具有长期影响、难以逆转且存在真实取舍的架构决策。RFC 被接受只代表架构方向获准，不表示实现已经完成，也不自动成为可领取的执行任务。
_不要混同：_ 普通需求、缺陷、实施计划。

### Delivery Proposal（交付候选提案）

用于比较候选交付、说明为什么现在做以及安排先后顺序的非权威材料。它本身不授权编码；被选中的工作仍须落到 Delivery Issue，并经过适用的规格与计划阶段。
_不要混同：_ 已接受 RFC、Delivery Issue。

### Specification（规格）

描述一个 Delivery Issue 必须实现的行为、边界、兼容性与验收标准，回答“要做成什么”。
_不要混同：_ 按文件和步骤展开的实施计划。

### Implementation Plan（实施计划）

把已确认规格拆成可执行步骤、测试顺序和检查点，回答“怎样完成”。Plan Task 是实现粒度，不是团队派单粒度。
_不要混同：_ Delivery Issue、Agent Todo。

### Delivery Packet（交付包）

为一个 Delivery Issue 提供完整实施上下文的一组权威材料，包括适用的 RFC、规格、实施计划及其当前状态。实现新需求的开发者或 Agent 必须同时取得交付包和产品代码，不能仅从代码仓猜测需求。
_不要混同：_ 只用于维护当前代码的稳定项目契约。

### Stable Project Contract（稳定项目契约）

已经生效，并且后续开发者在构建、验证、修改或评审当前产品代码时必须持续遵守的规则。它应存在于产品仓可发现、可执行或可机器验证的表面中，但不负责描述下一项需求。
_不要混同：_ RFC 的决策过程、某次交付的规格或实施计划。

### Agent Todo

LoopX 面向一次有界交付或决策的本地执行单元。它可以覆盖实施计划中的多个 Task；在获批范围内完成验证及所需独立 Review 后自动闭环并接续已授权工作。Todo 完成本身不触发 User Gate，只有到达需要负责人决策的边界时才建立 Gate。
_不要混同：_ Plan 中每一个 Task。

### Verification Profile（验证配置档）

按变更风险定义的一组可重复验证命令。根 `Makefile` 是可执行入口；实施计划声明适用档位，PR 展示结果，Gitee CI 与 Reviewer 调用同一入口。这里的 Verification 不表示用户授权。
_不要混同：_ LoopX User Gate。

### User Gate（用户门）

LoopX 中需要权利人或负责人作出明确决定、授权或风险接受的边界。测试通过和 Review 结论都不能替代 User Gate。
_不要混同：_ CI 检查、Verification Profile。

### Document Promotion（文档晋升）

从私有研发材料中提炼稳定、当前且开发者必须遵守的契约，并将其写入 `ming-iot` 的 `AGENTS.md`、README、Makefile、机器测试或运行文档。晋升是提炼，不是复制整份研发档案；晋升后代码仓内容成为该执行规则的权威入口。
_不要混同：_ 把 `ming-iot-rd-docs` 整体迁入代码仓。

## 事实源边界

- Gitee Issue、PR 与两个私有 Git 仓库承载团队共享事实。
- `ming-iot` 承载产品代码、可执行验证入口和维护当前系统所需的稳定项目契约。
- `ming-iot-rd-docs` 承载新交付所需的 RFC、规格、计划，以及完整 Review、Evidence 摘要和历史归档。
- LoopX goal、Todo、Evidence 与 Agent 状态是本地长程执行投影，不是跨电脑的团队派单数据库。
