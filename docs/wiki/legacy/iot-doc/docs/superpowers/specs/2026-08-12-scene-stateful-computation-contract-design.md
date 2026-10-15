# 场景有状态计算统一规范与机器契约设计

> 日期：2026-08-12
> 状态：设计已确认，等待用户复核
> 范围：统一 Scene 有状态计算术语与文档入口，并在 `things` 建立机器可读契约、生成器和可审查生成物；不实现 Scene Runtime 或 FSM 求值器。

## 1. 背景与问题

现有场景联动文档把不同层次的概念放在一起：

- `SceneBindingState` 是一个 Binding 在一个计算主体上的完整运行时状态容器；
- latest、rolling 是容器内的输入状态结构；
- edge/change、duration、debounce、cooldown、hysteresis 才是具有状态迁移语义的算子；
- timer 是支撑 duration 等算子的运行机制；
- delayed action 是动作接受后的 PostgreSQL Outbox 持久效果；
- script、Alarm/FSM 是尚未冻结完整配置和状态契约的扩展能力。

这些内容目前分散在《场景联动 Runtime 主规格总纲》《算子语义与术语》《15 分钟读懂场景联动》
等文档中。相同语义存在多个详细定义位置，读者容易把状态容器、状态组件和有状态算子混称；
未来生成 Go 配置、状态类型和校验器时，也没有一个机器可读的唯一约束源。

本设计同时解决两个问题：

1. 在 `iot-doc` 建立一篇统一的人类可读规范；
2. 在 `things` 建立机器可读契约，并从中生成代码骨架、校验器、注册表、测试数据和文档目录。

## 2. 目标与非目标

### 2.1 目标

1. 统一使用“场景有状态计算”作为中文总称，并固定各层术语。
2. 新建《场景有状态计算规范》，作为状态能力详细业务语义的唯一人类入口。
3. 以 `things/contracts/scene/stateful-computation-spec.yaml` 作为字段和生成约束的机器唯一真源。
4. 覆盖 latest、rolling、五类有状态算子、script、Alarm/FSM、timer 和 delayed action，并显式标记成熟度。
5. 为 `CODEGEN_READY` 能力生成 Go 配置/状态类型、校验器、注册表和 golden-case fixtures；FSM 业务逻辑保持手写。
6. 将生成目录嵌入统一中文规范，并使用规范版本和规范化契约 hash 防止跨仓漂移。
7. 保留既有 Markdown 链接和锚点的兼容入口，不因文档整理造成断链。

### 2.2 非目标

本次不包含：

- Scene Kafka consumer、Actor、Materializer 或 Flink Runtime 实现；
- edge/duration 等 FSM 的手写求值实现；
- 从 YAML 自动生成可执行状态迁移代码；
- 补齐 script、Alarm/FSM 尚未确认的产品语义；
- 数据库迁移、线上 rollout 或生产接流；
- 把目标设计标记成当前仓库已有实现；
- 发布独立契约制品仓库或新增跨仓发布基础设施。

## 3. 术语与分类

### 3.1 总称与层次

| 层次 | 中文名称 | 英文/代码名称 | 定义 |
| --- | --- | --- | --- |
| 总称 | 场景有状态计算 | Scene Stateful Computation | Scene Runtime 内所有需要保存业务状态或持久效果的计算能力总称 |
| 容器 | 场景绑定状态 | `SceneBindingState` | 一个 Binding 在一个 `evaluationSubjectKey` 上的完整业务状态容器 |
| 状态组件 | 状态组件 | `state_components` | latest、rolling 等为算子提供输入状态的数据结构，本身不是有状态算子 |
| 算子 | 有状态算子 | `stateful_operators` | 有明确配置、状态、phase 和 transition 语义的计算单元 |
| 扩展 | 扩展能力 | `extensions` | script、Alarm/FSM 等属于有状态计算、但首版契约尚未达到代码生成门槛的能力 |
| 机制 | 运行机制 | `runtime_facilities` | timer 等支撑算子执行和恢复的机制，不是业务算子 |
| 效果 | 持久效果 | `durable_effects` | delayed action 等动作接受后脱离 `SceneBindingState`、由 durable store 承担的效果 |

### 3.2 五类有状态算子

首版保持现有“五类有界算子”口径：

1. edge/change；
2. duration/FOR；
3. debounce；
4. cooldown；
5. hysteresis。

机器契约把 edge/change 建模为一个 `edge_change` 算子族，并用显式 variant 区分 predicate edge
和 canonical value change。这样既保持五类产品口径，又允许生成不同配置与 state variant；实现不得
依赖字段是否为空猜测当前 variant。

### 3.3 不得混称

- `SceneBindingState` 只能称为状态容器或场景绑定状态，不能称为一个算子。
- rolling 只能称为有界窗口状态组件，不能单独概括全部有状态规则。
- timer 只能称为运行机制，不能与 duration、debounce 混成同一种业务算子。
- delayed action 是已接受持久效果，不属于普通条件 timer，也不进入 `SceneBindingState`。

## 4. 规范权威边界

### 4.1 人类可读规范

在 `iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动/` 新建：

```text
场景有状态计算规范.md
```

它唯一拥有以下详细业务语义：

- 状态能力分类和概念关系；
- `SceneBindingState` 内各类状态的业务含义；
- latest-value join、rolling 与统一求值顺序；
- 五类有状态算子的 arm/update/cancel/fire、输出与不变量；
- missing、freshness、`NOT_READY` 与 fail closed 对算子的影响；
- 冷启动、恢复和 timer fire 复核；
- 普通 timer 与 reliable delayed action 的边界；
- Scene 小状态和 Flink 大状态的边界；
- 完整案例和能力速查表。

《场景联动 Runtime 主规格总纲》继续拥有 Runtime 产品边界、state identity、wire、持久化、事务、
容量和总体恢复取舍；其中计算、完整状态、冷启动和 timer 章节只保留架构摘要并下钻到新规范。

《算子语义与术语》保留现有文件路径作为兼容入口，正文收敛为 Reducer、ComputationPlan、Fact、
Current Fact、Admission 和投影等共享术语与事实契约。迁出的状态组件和算子章节保留原锚点、短摘要
和指向新规范的链接，不再维护第二份详细定义。文档标题可改成“共享术语与事实契约”，但文件名
不变，以保护已有相对链接。

《15 分钟读懂场景联动》只保留案例解释和统一规范入口，不承担规范定义。《水线与读取屏障详解》
继续拥有 barrier 直观解释；新规范只摘要“水线先于 freshness”的求值前提并链接原文，不复制完整
水线协议。

### 4.2 机器可读契约

`things/contracts/scene/stateful-computation-spec.yaml` 唯一拥有：

- 稳定 ID、分类和成熟度；
- 配置字段和状态字段的名称、类型、必填性、默认值与范围；
- 枚举、互斥关系和跨字段校验；
- phase、transition 和 timer kind 的允许名称；
- state/argument/timer 的配额元数据；
- 进入 `semanticHash` 的配置字段；
- 兼容性和 breaking-change 元数据；
- 生成目标、文档锚点与 golden cases。

《场景有状态计算规范》唯一拥有人类业务解释、执行顺序和失败行为。两者不以“后写覆盖”解决冲突：
凡是同时影响机器字段和业务语义的变更，必须成对修改，并通过契约 hash、生成 diff 和 golden-case
检查。主规格的架构边界仍高于从属实施契约；如发现冲突，必须联合修订，不允许生成器静默裁决。

## 5. 机器契约结构

### 5.1 文件布局

首版采用一个业务 YAML，避免过早拆成大量文件：

```text
things/contracts/scene/
├── stateful-computation-spec.yaml
├── stateful-computation-spec.schema.json
└── gen/
    ├── types.gen.go
    ├── validators.gen.go
    ├── registry.gen.go
    ├── catalog.gen.md
    └── testdata/
        └── golden-cases.gen.json

things/tools/scene-contract-gen/
├── main.go
└── internal/
    ├── contract/
    │   ├── model.go
    │   ├── validate.go
    │   └── canonical.go
    └── generate/
        ├── golang.go
        ├── fixtures.go
        └── markdown.go
```

`stateful-computation-spec.schema.json` 是 YAML 文档结构的 meta-schema，不是第二份业务真源。
生成的 Go package 固定为 `scenecontract`，位于 `things/contracts/scene/gen`；未来 Scene Runtime
可以依赖它，但该 package 的出现不表示 Runtime 已实现。

### 5.2 顶层模型

```yaml
contract:
  id: scene-stateful-computation
  schemaVersion: 1
  specVersion: 1.0.0

stateContainer:
  maturity: CODEGEN_READY
  identity: [tenantCode, bindingId, evaluationSubjectKey]
  commonFields: []
  quotas: {}

stateComponents: []
statefulOperators: []
extensions: []
runtimeFacilities: []
durableEffects: []
```

`contract.schemaVersion` 管理 YAML 结构兼容性；`contract.specVersion` 管理业务契约发布版本。修改注释
或排版不要求提升 `specVersion`，任何会改变生成结果、校验结果、state schema 或业务含义的变更必须
提升 `specVersion`。

### 5.3 能力条目

每个能力条目使用同一公共 envelope，并按分类补充专属字段：

- `id`、`displayNameZh`、`displayNameEn`、`category`、`maturity`；
- 一句机器目录摘要和人类规范锚点；
- `configFields`：类型、必填、默认值、范围和互斥关系；
- `stateFields`：类型、可空性、配额和序列化要求；
- 输入、输出、允许 phase/transition 名称；
- timer 依赖和 timer slot kind；
- 冷启动、`NOT_READY`、freshness 和恢复策略引用；
- `semanticHashFields`；
- 兼容性与 breaking-change 规则；
- golden cases。

YAML 不表达可执行 transition table，不生成 FSM 求值代码。phase、transition、约束和 golden cases
只用于生成类型、注册元数据和约束手写实现。

## 6. 成熟度与代码生成门禁

### 6.1 成熟度定义

| 成熟度 | 含义 | 生成行为 |
| --- | --- | --- |
| `DRAFT` | 只有能力身份、分类、摘要和已知缺口，语义未冻结 | 进入文档目录，不生成 Go 类型或注册项 |
| `SPECIFIED` | 人类语义已经明确，但机器字段或实现准入未完成 | 进入文档目录，不进入生产 Go 注册表 |
| `CODEGEN_READY` | 字段、约束、兼容性和 golden cases 完整 | 生成 Go 类型、校验器、注册表和 fixtures |
| `IMPLEMENTED` | 已有手写实现，并通过契约测试 | 在 `CODEGEN_READY` 基础上登记实现 package 和测试入口 |

`CODEGEN_READY` 条目只要出现缺失字段、悬空引用、未知枚举、明显占位标记或缺失 golden case，
整个生成过程必须失败。`IMPLEMENTED` 还必须提供可解析的实现 package 和契约测试入口；首版没有
任何条目标记为 `IMPLEMENTED`。

### 6.2 首版成熟度

| 分类 | 能力 | 首版成熟度 |
| --- | --- | --- |
| 状态容器 | `SceneBindingState` | `CODEGEN_READY` |
| 状态组件 | latest、rolling | `CODEGEN_READY` |
| 有状态算子 | edge/change、duration、debounce、cooldown、hysteresis | `CODEGEN_READY` |
| 运行机制 | timer 公共 state、slot 和身份字段 | `CODEGEN_READY` |
| 持久效果 | reliable delayed action | `SPECIFIED` |
| 扩展能力 | script、Alarm/FSM | `DRAFT` |

delayed action 已有明确业务边界，但它落在 PostgreSQL Action Outbox，不生成进
`SceneBindingState`。script、Alarm/FSM 只有概念和零散字段，首版不得推测其完整配置、状态迁移
或错误契约。

## 7. 生成物边界

生成器只为 `CODEGEN_READY` 内容产生：

1. Go 配置类型和状态类型；
2. 类型安全的枚举与常量；
3. 默认值、字段范围、互斥关系及跨字段校验器；
4. 按稳定 ID 查询的只读能力注册表；
5. golden-case JSON fixtures；
6. 供文档同步使用的确定性 Markdown 目录。

生成器不产生：

- predicate 或表达式执行器；
- arm/update/cancel/fire 的业务实现；
- timer scheduler、Kafka consumer 或 Actor；
- Action Outbox DAO；
- Alarm/script 的占位运行时代码。

生成的 Go 文件必须带标准 generated header、中文文件说明和满足 `things/AGENTS.md` 的中文 Go Doc。
手工代码不得修改 `*.gen.go`；生成器再次运行应完全覆盖这些文件。

## 8. 人类规范结构与生成目录

《场景有状态计算规范》固定按以下顺序组织：

1. 阅读卡、权威关系与术语分类；
2. `SceneBindingState` 容器、身份和状态层次；
3. 统一求值顺序；
4. latest、rolling 状态组件；
5. 五类有状态算子；
6. script、Alarm/FSM 扩展能力；
7. timer 运行机制；
8. delayed action 与普通 timer 的边界；
9. 冷启动、恢复、`NOT_READY`、freshness 和 fail closed；
10. Scene 小状态与 Flink 大状态的边界；
11. 完整案例和能力速查表。

生成目录直接嵌入该文档，而不是要求读者再打开第二篇 generated 文档：

```markdown
<!-- BEGIN GENERATED: stateful-computation-catalog -->
specVersion: 1.0.0
canonicalContractSha256: sha256:[0-9a-f]{64}
| 分类 | 稳定 ID | 中文名称 | 成熟度 |
| --- | --- | --- | --- |
<!-- END GENERATED: stateful-computation-catalog -->
```

`things/contracts/scene/gen/catalog.gen.md` 保存同一目录片段，供 `things` 独立检查。跨仓同步命令
接收显式 `--doc` 路径，将该片段替换进两个 marker 之间；工具不得假定 `iot-doc` 一定位于某个
固定相对路径。

目录不写生成时间。`canonicalContractSha256` 对通过 schema 校验后的规范化模型计算：字段和 map
按稳定顺序编码，注释及纯排版变化不改变 hash，所有会改变生成模型的变化都会改变 hash。

## 9. 生成、同步与失败处理

### 9.1 处理流程

```text
读取 YAML
  -> JSON Schema 校验
  -> 跨字段和成熟度语义校验
  -> 构建规范化模型并计算 hash
  -> 在临时目录生成全部目标
  -> gofmt、fixture 和目录一致性校验
  -> 全部成功后替换 things 本地生成物
  -> 显式要求时同步 iot-doc marker 区块
```

生成器提供两类运行模式：

- generate：成功后替换目标文件；
- check：只在内存/临时目录生成并比较，发现 diff 后非零退出，不修改工作树。

文档同步是显式步骤。若未提供 `--doc`，生成器只处理 `things` 本地生成物；若提供 `--doc`，必须
先完成全部本地校验，再替换文档区块。任一失败都不得留下部分生成文件或半个 marker 区块。

### 9.2 Fail-closed 条件

以下情况必须失败：

- YAML 未知字段、重复 ID、非法成熟度或悬空引用；
- stable ID、variant、phase、transition 或 timer kind 冲突；
- `CODEGEN_READY` 缺少配置/state schema、兼容性、文档锚点或 golden cases；
- 默认值不满足自身约束；
- `semanticHashFields` 引用不存在或非配置字段；
- 生成目录的 hash、version 或能力集合与 Go 注册表不一致；
- 文档 marker 缺失、重复、逆序或已被手工破坏；
- gofmt、Go 编译或 fixture 结构校验失败。

所有输出先写入任务专用临时目录。只有全部校验通过后才逐目标原子替换；失败时保留原生成物，
并报告具体条目和字段路径。工具不执行 `git add`、`git commit` 或 `git push`。

### 9.3 跨仓一致性

`things` 与 `iot-doc` 是独立 Git 仓库，不能获得跨仓原子提交。本设计采用以下可审计约束：

- `things` 自身检查 YAML、Go 生成物、fixtures 和 `catalog.gen.md`；
- `iot-doc` 即使单独检出也能阅读完整规范，并看到所绑定的 `specVersion` 和 hash；
- 工作区集成检查同时读取两个仓库，比较 YAML 规范化 hash 与文档 marker hash，并比较目录正文；
- 任一侧过期都使集成检查失败，不允许把旧文档表述为已经同步；
- 跨仓修改必须分别列出两个仓库的预期文件，交由用户决定如何提交。

## 10. 文档迁移

### 10.1 迁入统一规范

从《算子语义与术语》迁入并去重：

- latest-value join 与 rolling；
- 统一求值顺序；
- missing/freshness 对状态计算的语义；
- edge/change、duration、debounce、cooldown、hysteresis；
- 类型化冷启动；
- 普通 timer；
- reliable delayed action；
- rolling 投影补充中属于 Scene 有界窗口的部分。

水线/coverage 的详细协议、Fact identity、Current Fact 投影和 cutover 仍由原有对应规范拥有；新规范
只保留有状态求值所需的前置顺序和链接。

### 10.2 保留兼容入口

- 原章节锚点保留一段摘要和新位置链接，不直接删除；
- 主规格 §§7～10 保留架构结论、state schema 摘要和事务边界，不复制算子 FSM；
- README 的“按问题查阅”新增《场景有状态计算规范》入口；
- 《15 分钟读懂场景联动》把 duration 案例下钻到新规范；
- 所有已有本地链接和 fragment anchor 继续通过检查。

迁移只改变信息架构，不改变已经确认的算子语义。发现现有文档之间无法机械合并的冲突时，不得
自行挑选一个版本：相关能力保持或降为 `SPECIFIED`/`DRAFT`，记录冲突并先联合修订规范，达到
`CODEGEN_READY` 门禁后才能生成生产注册项。

## 11. 验证策略

### 11.1 契约层

- JSON Schema 正反例；
- unknown-field、重复 ID、悬空引用与枚举冲突测试；
- 四级成熟度门禁测试；
- 默认值、范围、互斥和 `semanticHashFields` 引用测试；
- canonical hash 在 key 顺序/注释变化下稳定、在业务字段变化下改变。

### 11.2 生成层

- 相同输入重复生成字节一致；
- `--check` 对干净生成物成功，对任一手工漂移失败且不写文件；
- 所有 `*.gen.go` 经 gofmt 并可编译；
- registry、types、validators、fixtures 和 catalog 的 ID/版本/hash 一致；
- 每个 `CODEGEN_READY` 能力至少覆盖正常路径、边界或取消路径、`NOT_READY`/失败路径和冷启动路径；
- timer 增加稳定 identity/current-slot golden vectors；
- rolling 增加 timestamp 覆盖、去重、配额和 coverage 状态 fixtures。

golden fixtures 是手写 FSM 实现未来必须消费的契约数据，但本次不创建假的 runtime 测试通过标记。

### 11.3 文档层

- marker 替换、重复/缺失 marker 和 hash 失配测试；
- Markdown 渲染与本地链接/fragment anchor 检查；
- 文本检查确认五类术语不再混称；
- 检查详细算子定义只存在于新规范，旧章节只有兼容摘要；
- README、导读和主规格均能到达统一规范。

## 12. 实施阶段

### 阶段一：统一文档

1. 新建《场景有状态计算规范》；
2. 迁移并去重现有状态计算语义；
3. 保留兼容锚点并更新 README、导读和主规格引用；
4. 验证 Markdown、链接和术语一致性。

### 阶段二：机器契约与生成器

1. 新增 YAML 和 meta-schema；
2. 按 §6.2 写入全部能力及成熟度；
3. 以测试驱动实现 parser、语义校验、canonical hash 和生成器；
4. 生成并检查 Go 类型、validators、registry、fixtures 和 catalog；
5. 将 catalog 同步进统一中文规范并验证跨仓 hash。

两个阶段属于同一设计，但实施计划必须设置阶段检查点：阶段一不得虚构 YAML hash；阶段二完成前，
文档生成区块明确标识“机器契约尚未建立”，不得预填伪 hash。阶段二完成后该临时标识必须被正式
生成目录完整替换。

## 13. 验收标准

1. “场景有状态计算、`SceneBindingState`、状态组件、有状态算子、timer、delayed action”定义明确且不混称。
2. 《场景有状态计算规范》能独立回答有哪些状态能力、保存什么状态、如何求值、冷启动、恢复和失败。
3. 同一状态组件或算子的详细规范性语义只保留一个位置。
4. YAML 覆盖五类分类和全部约定能力，首版成熟度与 §6.2 一致，且没有 `IMPLEMENTED` 条目。
5. 所有 `CODEGEN_READY` 条目通过 schema、语义完整性和 golden-case 门禁。
6. 重新生成零 diff，Go 生成 package 可编译，生成器失败时不污染现有文件。
7. `iot-doc` marker 中的 version、canonical hash 和目录正文与 `things` 契约一致。
8. 现有 Markdown 本地链接和兼容锚点无断链。
9. Scene Runtime、FSM、Alarm/script 或数据库实现没有被本次生成基础越权创建。
10. 最终分别检查 `things` 与 `iot-doc` 的 `git status`，只出现计划内文件；工具和实施过程不自动提交或推送。
