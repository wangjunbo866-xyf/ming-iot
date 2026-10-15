# Canonical → Scene Fact 投影契约补充修订设计

> 日期：2026-08-04
>
> 状态：设计方向已确认，书面规格待复核后联合修订
>
> 范围：Canonical 事实模型、来源发布物、Scene Admission、Scene Current Fact、
> `(factType, factKey)`、SingleValue/rolling 输入语义及其跨文档一致性

## 1. 问题与目标

父架构已经把 Automation 的生产输入冻结为四类受信事实来源：

- `PropertyReport`：由 `ModuleSchemaSnapshot.pointOrdinal` 解释的类型化点位报告；
- append-only `DeviceEvent`；
- 共享计算层产生的 `ComputedMetricUpdated`；
- reducer 已提交的 `AcceptedStateTransition`。

Scene Runtime 侧却只定义了批量 `SOURCE_DELTA.factDeltas[]`，并用精确
`(factType, factKey)` 建 TriggerPosting 和参数引用。现有 Scene 主规格与从属文档没有定义四类
Canonical/派生事实如何得到稳定 `factKey`，也没有冻结：

- 一个 `pointOrdinal` 如何映射为 Scene 可长期引用的逻辑属性；
- 物模型 Identifier、原始类型或 TDengine 列发生演进时，Scene 身份是否延续；
- `ABSENT/VALUE/NULL/RESET/INVALID/quality` 如何投影；
- `eventTs`、可选 `factVersion` 和重复/冲突如何排序；
- 哪些事实有 Current Fact，哪些事实每次发生都独立；
- Current Fact 投影落后时，冷启动如何避免把“尚未投影”误判为“业务缺值”；
- active Scene interest 与不可变物模型快照之间如何解耦。

这使实现者无法可靠实现 Admission、relevance filter、Current Fact bootstrap、
latest-value join、rolling 或 mixed-version 设备兼容。本次设计补齐这一集成缝，且保持
此前已经确认的 Scene V1 边界：

- 只有 `processingNamespace=production && processingMode=LIVE` 进入生产 Scene；
- 不引入 Scene replay evaluator、RocksDB、远端 snapshot、Kafka EOS 或跨 Topic 事务；
- Scene scope matching 继续读取处理时的 current `SubjectContext`；
- Scene state/timer 继续采用内存热状态 + 异步 best-effort `scene_state`，Action Outbox
  仍是唯一进入 Scene PG 本地事务的 durable action acceptance。

## 2. 方案比较与选择

### 2.1 方案 A：Scene 维护独立 FactDefinition Registry

为每个物模型属性、事件和计算指标另建全局 FactDefinition，由配置人员在每次发布时
判断复用还是新建。该方案可以统一元数据，但会造成双重真源、跨域生命周期协调和高
运维负担；模型首次发布时通常还没有 Scene，修改 Scene 也不应反向要求重新发布物模型。
拒绝。

### 2.2 方案 B：直接使用 Identifier 或 `(snapshotId, pointOrdinal)`

Identifier 同时服务 TDengine 列与外部点名。`voltage_a` 因原始类型变化而必须新建
`voltage_a_v2` 时，Identifier 会断裂；`pointOrdinal` 又只在单个
`ModuleSchemaSnapshot` 内稳定。两者都不能表达跨快照语义延续，会迫使用户为在线的
旧、新模型设备维护两套 Scene。拒绝。

### 2.3 方案 C：来源域拥有稳定语义 ID，Scene 只消费编译契约

属性、事件、指标输出和 accepted transition 分别由其来源域生成永不复用的稳定 ID，
并把 ID 与类型、单位、时间、可用性和版本策略冻结进不可变来源发布物。Scene 只引用
`(factType, factKey)`；统一的 `FactContract` 是从来源发布物编译出的运行时契约，不是
独立人工管理实体。采用。

该选择同时遵循三个原则：

1. **来源拥有身份**：谁定义事实，谁生成并维护其稳定语义 ID；
2. **发布物冻结解释**：历史 Canonical 必须按它携带的历史 release/snapshot 解释，
   不能查询“当前物模型”猜测；
3. **Scene 只声明兴趣**：Scene 创建、修改、启停只改变动态 interest/index，不改写来源
   快照。

## 3. 来源身份、时间形态与所有权

### 3.1 四类稳定语义 ID

| `factType` | `factKey` | 所有者与冻结位置 | `temporalKind` | Scene Current Fact | Scene 参数角色 |
| --- | --- | --- | --- | --- | --- |
| `PROPERTY` | `propertySemanticId` | 物模型属性定义；冻结到 `DmProductRelease` 所引用 `ModuleSchemaSnapshot` 的 ordinal dictionary | `LATEST_STATE` | 是 | `TRIGGER` 或 `CONTEXT_ONLY` |
| `DEVICE_EVENT` | `eventSemanticId` | 物模型事件定义；冻结到 `DmProductRelease` 所引用 snapshot 的 event item/dictionary | `OCCURRENCE` | 否 | 只能 `TRIGGER` |
| `COMPUTED_METRIC` | `metricOutputSemanticId` | immutable `ComputationPlan` 输出；由现有 `planReleaseId` 标识解释版本 | V1 固定 `LATEST_STATE` | 是 | `TRIGGER` 或 `CONTEXT_ONLY` |
| `STATE_TRANSITION` | `transitionSemanticId` | presence/command 等 reducer 随 `reducerVersion` 发布的 accepted-transition dictionary | `OCCURRENCE` | 否 | 只能 `TRIGGER` |

本文的逻辑术语 `ModelRelease` 落到现有 `DmProductRelease + 它引用的
DmModuleSchemaSnapshot` 发布物，不新增第二个 event registry；`ComputationPlan` 及
`planReleaseId` 沿用父架构现有命名，不另造 `PlanRelease` 实体。
若控制面/DDL 现有字段名叫 `eventDefinitionId`，它只是来源事件定义的持久化字段名，语义上
必须等于本文 `eventSemanticId`；它不是已拒绝的独立 `FactDefinitionId`，运行时也不得同时
维护两套 ID。

Computed Metric 的 immutable 发布物必须同样带可执行输出字典，而不只保存 plan 名称：

```text
ComputationPlan.outputs[] {
  outputOrdinal                 // planRelease-local，发布后冻结
  metricOutputSemanticId
  factContractRef
  semanticContractHash
  factOwnerKeyPolicyRef
  factOwnerKeyPolicyHash
  automationEligible
  automationEligibilityPolicyRef
}
```

`computationPlanContractHash` 覆盖按 outputOrdinal 编码的完整 outputs dictionary，包括静态
eligibility 与其 immutable governance policy ref；
`ComputedMetricUpdated` 必须携带 `outputOrdinal`，且其 semantic ID/ref/hash 与历史
`planId + planReleaseId` 字典逐项一致。

Accepted transition 同样不能只在 prose 中列 transitionType；reducer 发布物必须冻结：

```text
AcceptedTransitionDictionary.entries[] {
  transitionType
  transitionSemanticId
  factContractRef
  semanticContractHash
  factOwnerKeyPolicyRef
  factOwnerKeyPolicyHash
  automationEligible
  automationEligibilityPolicyRef
  allowedCauseKinds[]
  allowedCauseContractHashes[]
}
```

`acceptedTransitionDictionaryHash` 覆盖按 transitionType canonical encoding 排序后的完整 entry；
未知 entry、eligibility=false、policy ref/hash 不可解析或 cause 不在 allowlist 的 transition 都不得
形成 SOURCE_DELTA。

稳定语义 ID 必须满足：

- 平台生成、普通用户不可编辑、全生命周期永不复用；
- display name、Identifier、Modbus 地址、TDengine 列名、数据库自增行 ID 均不得替代；
- 正常编辑或发布下一版本时自动继承；
- 复制、新建或确认语义变化时自动生成新 ID；
- 同一 ID 的 canonical contract 不得发生不兼容改变；
- ID 必须进入不可变发布物及其 hash/签名，不能只留在 current 数据库行中。

### 3.2 `temporalKind` 的定义和配置责任

`temporalKind` 是“事实如何随时间演进”的来源契约，不是时间戳字段，也不是 Scene
timer 使用哪种时钟：

- `LATEST_STATE`：同一 subject、同一 fact identity 的更新竞争一个当前槽位；有
  Current Fact，可用于 Scene 冷启动和 latest-value join。每个被接受的更新仍然可以触发
  Scene，`change/edge` 再决定规范化值是否真正变化；
- `OCCURRENCE`：每次发生都是独立事实，以 `occurrenceId` 去重；不覆盖前一事件，也不写
  Current Fact。

普通物模型人员和 Scene 配置人员不直接编辑该枚举。平台按来源类型自动推导，并在发布
快照中显式固化。物模型人员已经通过“属性”或“事件”表达了业务选择，不再增加第二个
LATEST/OCCURRENCE 下拉框；Scene UI 最多只读展示“当前状态/独立事件”标签。

ThingsBoard 的 telemetry 同时具有历史时序与 latest view，Calculated Field 又在消费侧区分
latest/rolling，Rule Engine 则以 message type 区分事件；它没有一个要求设备模型逐字段配置
的统一 `LATEST_STATE/OCCURRENCE` 枚举。其 geofencing 按业务选择 status/event 输出的方式
只作为 UI 参考，不是本平台来源身份、Current Fact 或 replay 权限契约。

若某个高级派生器确实可以同时产生状态和事件，配置界面只提供业务化输出方式：
“当前状态”“独立事件”或“状态和事件都输出”。后两种输出必须获得不同的稳定语义 ID；
已发布 ID 不允许原地切换 `temporalKind`。

### 3.3 `FactContract` 是编译产物，不是 Registry

每个来源发布物必须能编译出：

```text
FactContract {
  factType
  factKey                       // 来源域稳定语义 ID
  temporalKind                  // LATEST_STATE | OCCURRENCE
  subjectType
  canonicalValueType
  payloadSchemaRef?             // DEVICE_EVENT/结构化 transition payload
  quantityKind?
  canonicalUnit?
  availabilityPolicyRef
  timestampPolicyRef
  timeDomainId
  eventTsEncoding              // V1 固定 UNIX_EPOCH_MICROS
  eventTsPrecision
  schedulerDeadlinePolicyRef?  // EVENT_TS 可驱动 deadline 时必填；冻结可信 epoch 映射/校验
  factVersionPolicyRef?         // 可选；无策略时不得伪造 version
  factVersionDomainId?
  factVersionEncoding?          // scalar/tuple 的 canonical wire encoding
  factVersionWrapPolicy?        // NONE 或来源冻结的 epoch/wrap 规则
  occurrenceOrderPolicyRef?     // 仅 OCCURRENCE，可选；V1 只用于证据/冲突校验
  occurrenceOrderDomainId?
  occurrenceOrderEncoding?
  occurrenceOrderWrapPolicy?
  semanticContractVersion
  semanticContractHash
}
```

`FactContract` 可以被编译缓存、下发和按 hash 校验，但不存在供运维独立增删改的
FactDefinition 表。Scene 编译器只能解析来源域发布的 contract，不能自行覆盖其类型、
单位、时间策略或 `temporalKind`。

同一 `(factType,factKey)` 跨 active release 必须得到**完全相同的
`semanticContractHash`**。该 hash 覆盖上述除 hash 字段本身外的全部语义字段和结构化 payload
字段身份；
FLOAT32→FLOAT64、Identifier/ordinal/协议地址变化只允许发生在 source projection/normalization
一侧，不能让 Scene 在运行时对多个 canonical contract 求“最大公约数”。原始映射、raw type、
单位换算函数和 ordinal dictionary 另形成随 snapshot 冻结的 `snapshotProjectionHash`；它可以
跨 S1/S2 不同。若不能投影到相同 semantic contract，就必须创建新的 stable semantic ID。
所有 `*PolicyRef/*SchemaRef` 都必须是可按内容 hash 校验的 immutable semantic reference，不能把
release-local 数据库行 ID 直接纳入 semantic hash 后造成语义相同、版本不同却无法兼容。

`factVersion` 的 wire 值必须符合 `factVersionEncoding`，比较只能调用
`factVersionPolicyRef` 的 canonical comparator；伪码中的 `>` 只是该 comparator 的简写。
publisher 和 Runtime 在比较前必须验证 `timeDomainId/eventTsEncoding/eventTsPrecision` 以及
`factVersionDomainId/encoding/wrapPolicy` 完全兼容，否则返回 `ORDER_DOMAIN_CONFLICT`，不得把
两个 release 的裸数字直接比较。
`eventTsEncoding=UNIX_EPOCH_MICROS` 只冻结 wire 数值，不自动证明设备时钟可信或可安排 deadline；
只有 immutable `schedulerDeadlinePolicyRef` 能把该 time domain 确定性校验/映射为平台 UNIX
epoch deadline 时，Fact 才可供 `EVENT_TS` timer 使用。`occurrenceOrder` 同样只能通过来源
contract 冻结的 domain/encoding/wrap policy 与 comparator 比较，Kafka offset 不得充当替代品。

### 3.4 `DmModuleSchemaSnapshot` V3 dictionary

本契约启用后的物模型来源发布物固定升级为 `schemaVersion=3`。V3 保留 V2 全部
`identifier + DmSchemaCore + extendConfig + affordance` 字段，并新增稳定、显式的运行时字典：

```text
items[] {
  itemOrdinal                    // snapshot-local；PROPERTY wire 中即 pointOrdinal
  itemKind                       // PROPERTY | DEVICE_EVENT | ACTION
  identifier
  propertySemanticId?            // PROPERTY 必填
  eventSemanticId?               // DEVICE_EVENT 必填
  factContractRef?                // content-addressed immutable ref；可进入 Scene 的事实必填
  semanticContractHash?          // 可进入 Scene 的 PROPERTY/EVENT 必填
  sourceProjectionRef?           // raw → canonical projection/normalization
  eventParams[]? {
    paramOrdinal                 // source snapshot/event schema-local
    eventParamSemanticId         // 来源拥有、自动继承、永不复用
    canonicalFieldOrdinal        // payloadSchema 内跨兼容 release 稳定
    identifier
    canonicalValueType
    quantityKind?
    canonicalUnit?
    availabilityPolicyRef
  }
}

snapshotProjectionHash
automationEligibleBitmap
factOwnerKeyPolicyRef
factOwnerKeyPolicyHash
```

`snapshotProjectionHash` 覆盖完整 dictionary、静态 eligibility 与 owner-key policy ref/hash；
`factContractRef` 必须解析到 §3.3 的完整 `FactContract`；实现可以把内容内嵌在 retained snapshot，
也可以指向随 release 保留、按内容 hash 校验的 immutable catalog artifact，但不能只留下一个无法
解析的 `semanticContractHash`，更不能在处理历史 input 时查询 current 草稿。DEVICE_EVENT 的
`payloadSchemaRef` 及完整 parameter contract 由该 ref 一并冻结。

`itemOrdinal/pointOrdinal` 的编号方式可以由稳定编码器决定，但发布后不得根据进程内排序重新
分配。`DeviceEvent` 的 `eventSemanticId` 只标识事件类型；Scene 若引用事件参数，Binding 保存
`eventParamSemanticId`，不能保存展示名、数组下标或临时 ParamKind。

对应 DEVICE_EVENT 的 FactDelta payload 是带 `payloadSchemaRef` 的 packed structured value：
source `paramOrdinal` 先经历史 dictionary 解析，再按
`eventParamSemanticId/canonicalFieldOrdinal` 表达 availability、canonical type 和 typed value；
事件根事实本身始终是一个 OCCURRENCE。属性/metric 的 scalar typed value 与事件 structured
payload 使用 IDL `oneof`，不得把事件参数压成不可校验的 `map<string,any>`。

V2 snapshot 不具备这些身份字段，不能直接进入本契约的新 Admission。它只能先按 §15 的
受审计 lineage migration 产生 V3 snapshot/mapping artifact；运行时不得按 Identifier 猜 ID。

## 4. 物模型演进与 mixed-version 设备

### 4.1 正常发布流程

- 首次创建属性/事件时，平台自动生成稳定语义 ID；
- 在原定义上修改展示名、描述、协议映射或兼容的规范化信息时自动继承 ID；
- 新建或复制定义时自动生成新 ID；
- 发布器将 ID、canonical contract 和 ordinal/event dictionary 一起冻结；
- Scene 选择器展示业务名称与来源，实际持久化稳定 ID。

普通发布不要求配置人员逐项回答“复用还是新建”。只有删除后重建、批量导入丢失来源
链路，或“旧 Identifier 退役、新 Identifier 接替同一业务量”等真正歧义的操作，才要求
一次显式选择：

- **升级/替换原属性，沿用已有 Scene 语义**；
- **创建独立属性，不沿用已有 Scene 语义**。

系统不得仅根据名称、Identifier、寄存器地址或显示名自动猜测语义延续。

### 4.2 Modbus 第 100 点的规范示例

```text
ModuleSchemaSnapshot S1
  schemaVersion      = 3
  modbusAddress      = 100
  pointOrdinal       = 17
  identifier         = voltage_a
  rawType            = FLOAT32
  propertySemanticId = P123
  canonicalType      = FLOAT64
  quantityKind       = VOLTAGE
  canonicalUnit      = V
  semanticContractHash = FC-P123
  snapshotProjectionHash = SP-S1

ModuleSchemaSnapshot S2
  schemaVersion      = 3
  modbusAddress      = 100
  pointOrdinal       = 23
  identifier         = voltage_a_v2
  rawType            = DOUBLE64
  propertySemanticId = P123
  canonicalType      = FLOAT64
  quantityKind       = VOLTAGE
  canonicalUnit      = V
  semanticContractHash = FC-P123
  snapshotProjectionHash = SP-S2

Scene argument
  (factType, factKey) = (PROPERTY, P123)
```

旧设备继续按 S1 解码，新设备按 S2 解码；两者都投影为各自 subject 下的 `(PROPERTY,
P123)`。不需要复制两套 Scene。TDengine 仍可保留 `voltage_a` 与 `voltage_a_v2` 两列，
因为物理存储列演进与 Scene 逻辑事实身份是正交问题。

这里的 `100` 是 Modbus 寄存器地址，不是 `pointOrdinal`。ordinal 只在各自 snapshot 内稳定，
发布器可因 Identifier 排序或 dictionary 编排给 S1/S2 分配不同 ordinal；跨快照连续性只由
`propertySemanticId=P123` 表达。

如果新字段改变的是业务量、量纲、subject、时间语义或无法安全规范化的值域，则发布器
必须拒绝继承 `P123`，要求新建语义 ID。

### 4.3 发布兼容性检查

沿用同一语义 ID 时至少检查：

- `factType`、`temporalKind` 和 subject type 完全一致；
- canonical value type、payload schema、quantity kind 和 canonical unit 完全一致；raw type widening
  或单位换算只属于 source projection，并且必须得到同一 canonical 结果；
- availability、quality 与 RESET/NULL 的 canonical 解释完全一致；
- timestamp policy、time domain、epoch encoding、precision 与 scheduler-deadline eligibility/
  immutable mapping 完全一致；
- `factVersion`/occurrence order 的 domain、canonical encoding、wrap 和 comparator 完全一致；
- 最终 `semanticContractHash` 完全一致，只有 `snapshotProjectionHash` 可以不同；
- ID 未被其他不兼容定义占用，且退役 ID 未被复用。

同一 `DmProductRelease` 所引用的 snapshot、同一 subject type 内，一个 active
`(factType, factKey)` 最多只能由一个来源定义产生；允许 S1/S2 等不同 release 沿用同一 ID，
但禁止同一设备的一条发布物中让两个点位同时竞争同一 PROPERTY identity。冗余测点需要先由
显式 reducer/metric 合成为新的稳定事实，不能靠到达顺序决定谁覆盖谁。

任何检查失败都阻止发布，不能只告警后继续。

### 4.4 来源快照 hash 与 TDengine 物理 schema 分离

把 `propertySemanticId`、FactContract 或 eligibility 加入不可变 snapshot 后，来源语义变化
可能产生新的 `ModuleSchemaSnapshot/schemaHash`，但这不等于 TDengine 必须新建 STABLE。
本设计明确采用模块级超级表已经定义的物理策略，并要求联合修正父架构中的反向表述：

```text
ModuleSchemaSnapshot/schemaHash
  = 完整不可变来源解释：identifier、ordinal、semantic ID、类型、quality、eligibility...

physicalSchemaFingerprint
  = TDengine 物理列/类型/TAG 及兼容演进计划

ModuleWritePlan
  = 某 source snapshot → moduleCode STABLE 列的版本化写入映射
```

- 同一 `moduleCode` 的 V1 常规路径只有一张 append-only STABLE；
- 新 Identifier 先 `ADD COLUMN`，旧列保留；删除只改变新 release 可见性，不 DROP 物理列；
- semantic ID、描述、Scene eligibility 或其他非物理 metadata 改变，可以产生新 source
  snapshot，但不得仅因 `schemaHash` 改变自动新建 STABLE；
- `physicalSchemaFingerprint` 改变也不必然新建表：兼容 `ADD COLUMN` 更新同一 STABLE 和
  ModuleWritePlan；只有模块级超级表契约已经定义的极少数破坏性变更才新建逻辑模块/物理表；
- `schemaHash` 继续对完整 snapshot JSON 做稳定 hash，不能为了复用物理表而排除 semantic ID；
  STABLE identity 固定只由 `moduleCode` 决定，`physicalSchemaFingerprint/ModuleWritePlan` 只用于
  schema diff、兼容校验和列写入映射，绝不能参与表名/物理身份。

## 5. 不可变快照、Scene eligibility 与动态 interest

三种概念必须分开：

1. **ordinal dictionary**：不可变 `ModuleSchemaSnapshot` 内的
   `pointOrdinal → propertySemanticId + FactContract` 映射，回答“这个值是什么”；
2. **`automationEligibleBitmap`**：由同一快照内的 `isCanSceneLinkage` 等静态能力/治理标记
   编译，回答“这个点是否允许进入 Automation/Scene”；它不表示当前已有 Scene；
3. **`InterestedOrdinalBitmap`**：控制面根据 enabled Binding 的精确 dependency、当前
   interest generation 和 snapshot dictionary 编译出的动态派生缓存，回答“Runtime 当前应为
   哪些点建立 TRIGGER posting、哪些点只作 CONTEXT”。它不属于快照 hash，也不是配置真源，
   更不能决定 Admission 是否发布一条来源输入。

因此：

- 创建、修改、启停 Scene 只更新 Binding/ProjectIndex/interest generation，并使相关
  `InterestedOrdinalBitmap` 失效重编；
- 不重新发布任何 `DmProductRelease` 或 `DmModuleSchemaSnapshot`；
- snapshot mapping 在 Scene 尚不存在时也成立，因为它描述来源语义，不描述消费者；
- 同一稳定语义 ID 若同时存在于混合在线版本中，Scene 发布器必须对 Binding scope 内全部
  applicable active release 解析 contract family：`semanticContractHash` 必须完全相同，且每个
  release 都必须 scene-eligible。Scene V1 不提供按 model release 静默过滤的 Binding；
  任一在线 release 不满足时，发布失败，直到模型迁移完成或用户显式缩小业务 scope。相同校验
  还必须挂在 source release 激活和 subject 进入动态 scope 的控制面变更上，不能只在首次发布
  Binding 时检查一次；
- 修改 `isCanSceneLinkage` 等静态 eligibility 会产生新的来源 snapshot，但不改写旧 snapshot。
  把既有事实从不可用改成可用时，不能据此伪造旧区间的 Current Fact coverage；必须执行受控
  初始构建/authoritative current rebase 并生成覆盖原 obligation 的 seed；仅等待下一条
  production+LIVE 更新不能证明它高于未投影前缀中的乱序 eventTs，仍不得得到 ready current；
- 同一规则适用于 ComputationPlan output 与 AcceptedTransitionDictionary entry 的
  `automationEligible`。新 artifact 的 false→true 对 LATEST metric 必须先建 coverage seed；
  true→false 必须在激活前撤销 posting 并 fence 对应 Current Fact。OCCURRENCE transition 没有
  Current Fact，只撤销 posting。旧 artifact backlog 的 Admission emission 仍按它自己的 immutable
  eligibility 重建，不能因 cutover 改变 ACK-uncertain retry；Runtime/current writer 再按受信
  applicable-artifact timeline 分别做零 fanout或 `SOURCE_ARTIFACT_NO_LONGER_APPLICABLE` terminal
  no-write；
- bitmap 只能优化 Runtime fanout/posting，不能改变 Fact identity、Admission 输出与 payload，
  也不能跳过发布器要求的类型/contract 校验；
- dynamic interest 必须区分 `TRIGGER_EMIT` 与 `CONTEXT_REQUIRED` 角色，但角色只在 Runtime
  消费时按当前 Binding 建 fanout：纯 CONTEXT_ONLY 或当前无人引用的事实不会取得 triggerRight，
  也不会仅因自身刷新触发 Scene；若同一事实在其他 Binding 中是 TRIGGER，则为该 Binding 建
  正常 posting；
- 每个 contract 合法的 production+LIVE 来源输入，只要含至少一个来源发布物静态声明为
  Scene-eligible 的有效更新，就必须形成一条 SOURCE_DELTA；其中携带该 input 内
  `automationEligibleBitmap`（或其他来源的等价静态 eligibility dictionary）命中的全部更新。
  输出决定和 payload 集合只由 input 引用的 immutable source artifact 与 Canonical states 决定，
  不随 Scene interest generation 漂移。Runtime 只用当前仍有 TRIGGER posting 的 fact 建 fanout，
  未被 Binding 引用的同批 fact 只作为无害 candidate。本批 Context
  作为 store baseline 之后的 merge candidate，仍须经过同一 comparator：仅 `ACCEPT` 或与当前
  sourceEventId 完全相同时使用本批值，store 严格更新时保留 store，equal-order conflict 则
  fail closed；“本批优先”绝不表示无条件 batch-wins；
- 主规格明确不提供普通 Binding config→data offset activation barrier，因此不能让 current
  interest 充当 Admission emit gate。否则“首次 publish 已成功但 ACK 丢失”与“首次 publish
  未成功”会在配置变更后的 retry 上产生不可判定分叉。Admission 对同一输入始终重建并重发
  相同 `sourceEventId` 与 immutable eligible payload；Runtime 再按消费时的 current Binding
  决定 0..N fanout。重复 record 仍按 §7.3、§8 和 §11.3 收敛；
- `InterestedOrdinalBitmap` 的 cache key 固定包含
  `(tenantCode,moduleSchemaSnapshotId,snapshotProjectionHash,tenantFactInterestGeneration)`，结果是
  tenant 内 enabled Binding 的保守并集，不按设备或 current affiliation 展开；tenant 隔离已
  由 `tenantCode` 保证，不能把 `ownershipBindingId` 加入 cache key 造成每设备 bitmap；
- bitmap 缺失、generation 不匹配或无法证明完整时，回退到 bounded dictionary lookup 或
  retry，不能返回部分 interest 结果；
- Scene Current Fact 的覆盖范围由静态 eligibility 与来源投影契约决定，不能仅依赖“此刻有
  trigger posting”的动态 bitmap，否则新建 Scene 时无法可靠 bootstrap。

## 6. FactDelta 正式语义

### 6.1 公共结构

物理 IDL 可采用紧凑 packed lane/arena，但必须表达下列逻辑字段：

```text
ProcessingContext {
  processingNamespace
  processingMode = LIVE | BACKFILL | REPLAY
  processingContextContractRef {
    platformProcessingContextArtifactId
    platformArtifactContentHash
  }
  processingModePolicyReleaseId
  processingModePolicyArtifactHash
  processingContextContractVersion = PROCESSING_CONTEXT_CONTRACT_V1
  processingContextContractHash
  processingProvenanceRef
}

SceneEvent {
  eventId
  required processingContext ProcessingContext
  identityAlgorithmVersion = SCENE_ID_V2
  oneof payload {
    SourceDelta
    LinkedDelta
    ScheduleEvaluate
    SubjectLifecycle
    LifecycleTargetDelta
  }
}

SourceDelta {
  tenantCode
  sourceKind                    // PROPERTY_REPORT | DEVICE_EVENT | COMPUTED_METRIC | ACCEPTED_TRANSITION
  sourceSubjectKey
  factOwnerKey                  // FACT_OWNER_KEY_V1 typed canonical bytes
  factOwnerKeyPolicyRef
  factOwnerKeyPolicyHash
  sourceOutputId                // Canonical/reducer/metric 原始 outputId
  sourceSemanticSetHash         // 四类来源统一的 Scene 解释集合 hash
  sourceIdentityVersion = SCENE_SOURCE_ID_V1
  factIdentityVersion = SCENE_FACT_ID_V1
  sourceEventId                 // 按下文公式派生的 Scene source trigger identity
  sourcePosition {
    sourceStreamId {
      logicalKind
      kafkaClusterId
      topicId                   // broker immutable topic UUID
      topicGeneration           // delete/recreate 或受控迁移时递增
    }
    partition
    recordOffset
  }
  sourceContractRefs[]
  factDeltas[]
}

FactDelta {
  factDeltaId
  factType
  factKey
  temporalKind

  availability = AVAILABLE | UNAVAILABLE
  unavailableReason? = NULL_VALUE | RESET | INVALID_VALUE | BAD_QUALITY
  canonicalValueType
  payload? {                    // 仅 AVAILABLE，IDL oneof
    scalarValue
    structuredValue {
      payloadSchemaRef
      fields[] {
        fieldSemanticId
        canonicalFieldOrdinal        // payloadSchema 内稳定；不是 release-local paramOrdinal
        availability
        canonicalValueType
        typedValue?
        unavailableReason?
      }
    }
  }

  eventTs
  timeDomainId
  eventTsEncoding
  eventTsPrecision
  timestampSource
  timestampPolicyRef
  observedAt                    // 平台稳定观测时间，不参与 LATEST 主排序
  factVersion? {                // 仅来源契约存在时携带
    factVersionDomainId
    factVersionEncoding
    canonicalValue
  }

  occurrenceId?                // OCCURRENCE 必填
  occurrenceOrder? {           // contract 声明可信顺序时必填
    occurrenceOrderDomainId
    occurrenceOrderEncoding
    canonicalValue
  }
  sourceContractRefIndex       // 指向 SourceDelta.sourceContractRefs[]
  sourceLocation {             // oneof，不能只放无法解析的字符串
    snapshotPoint { moduleSchemaSnapshotId, pointOrdinal }
    eventItem { moduleSchemaSnapshotId, itemOrdinal }
    computedMetric { planId, planReleaseId, outputOrdinal } // release-local frozen ordinal
    acceptedTransition { reducerKind, reducerVersion, transitionType }
  }
}
```

所有 data-origin Scene wire 必须携带并原样传播公共 lineage：

```text
SceneSourceLineage {
  identityAlgorithmVersion
  sourceIdentityVersion
  factIdentityVersion
  factOwnerKey
  factOwnerKeyPolicyRef
  factOwnerKeyPolicyHash
  sourceEventId
  sourceSemanticSetHash
}

RetainedFactProvenance {
  sceneSourceLineage
  factDeltaId
  factExecutionEnvelopeHash
  occurrenceEnvelopeHash?
  semanticPayloadHash
  temporalKind
  eventTs, timeDomainId, eventTsEncoding, eventTsPrecision
  timestampSource, timestampPolicyRef, observedAt, factVersion?
  occurrenceId?, occurrenceOrder?
  sourcePosition
  sourceContractRef
  semanticContractHash
  processingContext
}
```

`LinkedDelta`/local/remote target bundle、由该轮产生的 transition 与 Action Outbox provenance 都
复制该 lineage，不能只留下公共 `SCENE_ID_V2`。所有派生路径同时原样传播完整
`ProcessingContext`，不能只复制 namespace/mode。每个进入 Current Fact、SingleValue 或 rolling
的值/样本必须保存 `RetainedFactProvenance`；data-origin timer slot/fire、transition 与 Action Outbox
保存其 causal provenance。`SceneBindingState` 还必须保存最后接受 trigger 的 lineage/version；
restore/target handler 只接受当前
`SceneFactIdentityCutoverManifest` allowlist 中的组合。旧 pending Outbox 可按 §15 作为
dispatcher-only legacy row 保留，但绝不能加载为新 Runtime state 或触发重算。Schedule、
SubjectLifecycle 等非 data trigger 使用各自稳定 trigger identity/version，不伪造 sourceEventId，
但仍必须携带上述同一完整 `ProcessingContext`。

物理 state 可用 BindingState-local dictionary 对重复的 source contract、owner policy 与
ProcessingContext ref/hash 去重，但每个 retained cell/sample 必须有无歧义 index；不能为了 32 KiB
上限只留下 aggregate hash 而失去 comparator、freshness、artifact retention 与审计证据。

Schedule/lifecycle 的 direct-trigger identity 公式与全局
`identityAlgorithmVersion=SCENE_ID_V2` 在本修订中保持不变，不新增“半定义”的 direct-trigger
子版本；同一 direct-trigger ID 的 retry 必须重建相同完整 ProcessingContext，任一 ref/hash/value
漂移均为 `DIRECT_TRIGGER_CONTEXT_CONFLICT`。§15 通过先 drain/fence 旧 direct producer 再启用
required context，避免同一 logical occurrence 跨两套 context wire 重放。

身份公式固定为：

```text
sourceEventId =
  H(SCENE_ID_V2, SCENE_SOURCE_ID_V1, processingNamespace,
    tenantCode, sourceKind, factOwnerKey, sourceOutputId, sourceSemanticSetHash)

factDeltaId = H(SCENE_FACT_ID_V1, sourceEventId, factType, factKey)
occurrenceId = factDeltaId       // temporalKind=OCCURRENCE
```

`SCENE_ID_V2` 是既有 Scene 全局 identity namespace/version；`SCENE_SOURCE_ID_V1` 与
`SCENE_FACT_ID_V1` 是本次新增的 Canonical→Scene 子公式版本。三者职责不同，必须全部进入 wire、
hash 和 cutover allowlist，不能因为外层仍为 V2 就省略两个子版本。

`sourceSemanticSetHash` 不能把 Canonical 专用字段强塞给另外两类 producer，必须按 source kind
确定性构造：

- PropertyReport/DeviceEvent：
  `H(SCENE_SOURCE_SET_V1, CANONICAL, semanticReleaseSetHash, factOwnerKeyPolicyHash,
  processingContextContractHash, projectionContractEntries[], semanticContractEntries[])`；其中 Canonical 的
  `semanticReleaseSetHash` 继续覆盖既有
  protocol/model/processing-mode/contract releases；多 module span 时 projection hash 与
  semantic contract hash 分别形成 entry，均按
  `(artifactKind,artifactId,factType,factKey,contentHash)` 排序后做长度前缀编码，不能依赖 map
  遍历顺序；集合取完整 source output 引用的 artifact contracts，不取动态 interest 过滤后恰好
  发出的 FactDelta 子集；
- ComputedMetricUpdated：
  `H(SCENE_SOURCE_SET_V1, COMPUTED_METRIC, planId, planReleaseId, computationPlanContractHash,
  metricSemanticContractHash, factOwnerKeyPolicyHash, processingContextContractHash)`；
- AcceptedStateTransition：
  `H(SCENE_SOURCE_SET_V1, ACCEPTED_TRANSITION, causationSemanticSetHash,
  reducerKind, reducerVersion, acceptedTransitionDictionaryHash, factOwnerKeyPolicyHash,
  processingContextContractHash)`。

`causationSemanticSetHash` 覆盖 reducer 接受该 transition 所依据的 immutable cause contract：直接
由 Canonical/metric outcome 引起时继承其 `sourceSemanticSetHash`；`FactExpired`、command
timeout/cancel 或 reducer timer 等 synthetic cause 则由 reducer producer 构造
`H(REDUCER_CAUSATION_SET_V1,reducerKind,reducerVersion,triggerKind,
triggerContractHash,timerOrCommandPolicyHash,processingContextContractHash)`。字典必须为每个
`transitionType` 冻结允许的 cause kind/contract；缺 hash、current 配置拼出的 hash 或字典不允许
的组合都不得进入 Scene。

`processingContextContractHash` 不是 producer 自选字符串。平台数据契约控制面是
`PROCESSING_CONTEXT_CONTRACT_V1` 的唯一 owner，并发布 content-addressed 的
`PlatformProcessingContextContract` artifact；各可信入口域的 mode policy 则由
`processingModePolicyReleaseId` 定位 immutable policy artifact。hash 构造固定为：

```text
processingContextContractHash =
  H(PROCESSING_CONTEXT_CONTRACT_V1,
    platformProcessingContextArtifactId, platformArtifactContentHash,
    processingModePolicyReleaseId, processingModePolicyArtifactHash,
    namespaceGrammarVersion, modeEnumEncoding,
    allowedNamespaceModeMatrixHash, provenanceSchemaHash, trustPolicyHash)

processingContextCanonicalHash =
  H(PROCESSING_CONTEXT_VALUE_V1,
    processingNamespace, processingMode,
    processingContextContractHash, processingProvenanceRef)
```

policy owner 与传播规则固定为：

- ProtocolApplication/Canonical producer 冻结其 data-processing policy release；metric、reducer
  与 synthetic cause 必须继承上游 policy，或使用签名 computation/reducer artifact 明确引用的
  policy release，不能写一个裸 `LIVE`；
- 平台 Scheduler 控制面发布 content-addressed 的 `SCHEDULE_PROCESSING_POLICY_V1`，每个
  Schedule release 固定引用其 release ID/hash；Things 控制面同样拥有
  `SUBJECT_LIFECYCLE_PROCESSING_POLICY_V1`，SubjectLifecycle relay 只能引用已发布版本。两者
  虽只允许 production+LIVE，也不能用常量枚举替代 artifact ref/hash；
- `LinkedDelta/LifecycleTargetDelta` 必须逐字段继承其 parent SceneEvent 的 context，timer fire
  必须继承创建 slot 时保存的 context；它们不得分配新的 mode-policy release。Scene-derived
  Canonical/transition 若成为新来源，也必须保留 parent provenance，并按其签名 producer contract
  冻结是否继承或转换 policy；
- `SceneBindingState`、timer slot/fire、transition 与 Action Outbox row 保存完整 context ref/hash，
  restore、callback 与 dispatcher 均重验；只有 namespace/mode 两字段不再满足 wire 契约。

Canonical、metric、reducer、Scheduler、SubjectLifecycle relay、Admission、Runtime 与 Current Fact writer 必须按 wire 中的
`processingContextContractRef` 从相同 retained artifact 解析/重建，并验证该 ref、mode policy
ref 与 composite hash 的公式一致。artifact retention 不是简单跟随 hot Canonical TTL：只要 Raw/replay archive、Current
Fact、Scene state/timer、Action Outbox、quarantine 或 cutover manifest 仍引用该 contract/policy，
就必须保留可解析内容；最后一个 dependent reference 清除且审计保留期结束后才可 GC。wire
hash/ref 缺失、未知、已过 retention、内容不符或 producer 计算结果不一致均 fail closed。
contract 变更必须走 processing-mode 联合 rollout，不能读取 current 配置后静默改变历史 source
identity。

因此它覆盖所有会改变 Fact 解释的 model/snapshot、event/transition dictionary、
ComputationPlan/reducer contract、processing-mode policy 与 Fact projection contract；不能只覆盖
协议 decoder。
Admission 与 Current Fact writer 必须调用同一版本化 projection/identity library，并校验来源
producer 可选携带的预计算 hash；二者结果不一致是 permanent contract conflict，不能各自接受
一套解释。
同一 Canonical redelivery、Admission retry、rebalance、interest generation 变化或语义不变的
镜像滚动不得改变上述 ID。实现可以显式携带 `factDeltaId`，也可以按公式派生，但不得使用
进程序号、新 UUID 或仅使用裸 `outputId`。

`sourceContractRefs[]` 的每项必须包含 artifact kind、artifact ID/version、artifact contract
hash、可解析的 immutable `factContractRef`、`semanticContractHash` 与
`snapshotProjectionHash?`；每个 FactDelta 用 index 无歧义
引用。Admission 必须验证 SourceDelta 的 `factOwnerKeyPolicyRef/hash` 与历史 V3 snapshot、
ComputationPlan output 或 AcceptedTransitionDictionary entry 完全一致；不得由 current subject
类型临时挑选 policy。相同 output 在不同 `sourceSemanticSetHash` 下得到不同
`sourceEventId`，避免同一 transport output 被两套模型解释时碰撞。
这些 source/FactContract artifact 与 `factOwnerKeyPolicyRef/hash` 采用和 ProcessingContext
相同的 dependent-reference retention：Current Fact、state/timer、Outbox、quarantine、history 或
cutover manifest 仍有一个 `RetainedFactProvenance` 引用时就不得 GC；restore 时 ref 缺失、hash
不符或内容不可解析必须 fail closed，不能只凭 aggregate `sourceSemanticSetHash` 继续运行。

`semanticPayload` 定义为 availability、unavailable reason、canonical type 和规范化值；structured
payload 还包含 schema ref 及按 `(canonicalFieldOrdinal,fieldSemanticId)` 排序后的字段状态/值。hash 必须
使用版本化 deterministic typed encoding，不能依赖 JSON/map 顺序。实现可缓存 canonical payload
hash 做快速比较；来源位置、映射完成后的原始 quality、Kafka offset、trace 和处理耗时不属于
semantic payload equality。

Fact identity 的幂等/冲突判断不得只比较 `semanticPayload`。必须另算：

```text
factExecutionEnvelopeHash =
  H(FACT_EXECUTION_ENVELOPE_V1,
    factDeltaId, temporalKind, semanticContractHash, semanticPayloadHash,
    eventTs, timeDomainId, eventTsEncoding, eventTsPrecision,
    timestampSource, timestampPolicyRef,
    observedAt,
    factVersionDomainId?, factVersionEncoding?, canonicalFactVersion?,
    processingContextCanonicalHash)

occurrenceEnvelopeHash =
  H(OCCURRENCE_ENVELOPE_V1,
    occurrenceId, factExecutionEnvelopeHash,
    occurrenceOrderDomainId?, occurrenceOrderEncoding?, canonicalOccurrenceOrder?)
```

这些字段都会改变 identity、rolling/unordered aggregation、occurrence-local predicate 或 timer
结果；同一 `occurrenceId` 只有 envelope hash 也相同
才是 duplicate，任一字段不同都必须 `OCCURRENCE_IDENTITY_CONFLICT`。LATEST rolling 也使用
上述完整字段的 `factExecutionEnvelopeHash` 检测同一 `factDeltaId` 的内容漂移。contract 声明
`occurrenceOrderPolicyRef` 时 wire order 必须存在且 domain/encoding 匹配；contract 未声明却携带
order、或只存在一侧时同样是 contract conflict，不能临时按裸数字比较。

### 6.2 Canonical PropertyReport 投影

| Canonical state | Scene FactDelta |
| --- | --- |
| `ABSENT` | 不产生 Delta；表示本报告没有更新该点 |
| `VALUE` 且 quality 被 contract 接受 | `AVAILABLE + typedValue` |
| `NULL` | `UNAVAILABLE(NULL_VALUE)` |
| `RESET` | `UNAVAILABLE(RESET)` |
| `INVALID` | `UNAVAILABLE(INVALID_VALUE)`；即使原 lane 仍有可解析原值也不得当成可用值 |
| `VALUE` 但 quality 被 contract 判为不可用 | `UNAVAILABLE(BAD_QUALITY)` |

LATEST_STATE 的业务 `UNAVAILABLE` 必须作为 Current Fact value 保存，不能用 Kafka/storage
tombstone 表示。
技术 tombstone 只用于 subject 删除、projection GC 或明确的状态删除；否则 compact 后会丢失
“明确不可用”与“从未见过/尚未投影”的区别。

类型投影先按历史 `ModuleSchemaSnapshot` 读取原始 lane，再按该 snapshot 冻结的
FactContract 规范化。Admission 不查询 current model，不把原始 Identifier 当 `factKey`，
也不把 FLOAT32/DOUBLE64 等物理类型直接暴露给 Scene。

### 6.3 另外三类来源

- `DeviceEvent.eventType` 必须经其历史 `DmProductRelease` 所引用 snapshot event dictionary
  解析到 `eventSemanticId`；`occurrenceId` 使用 §6.1 的稳定 fact identity，不能以 event name
  充当身份；事件参数按 §3.4 的 V3 parameter dictionary 规范化为 structured payload，未知、
  重复或类型不兼容的 parameter 使整个 DeviceEvent quarantine，不能丢掉坏字段后触发；
- `ComputedMetricUpdated` 必须携带 `planReleaseId`、冻结的 `outputOrdinal`、对应
  `computationPlanContractHash`、
  `metricOutputSemanticId`、可按内容 hash 解析完整 FactContract 的 immutable ref 和
  `processingContextContractHash`。Scene V1 只接纳
  `finality=FINAL + production+LIVE` 输出；non-final
  revision 必须在 Admission source projection/fanout 前和 Current Fact projector value mutation 前执行
  `UNSUPPORTED_FINALITY_BYPASS` terminal no-write，不能靠本文未定义的补偿状态机放行。
  revision 可按来源契约编码为 `factVersion`，但 plan release 本身不是 fact version；
- `AcceptedStateTransition` 不新增 `ReducerRelease` 数据库实体。presence/command reducer 随
  现有 `reducerVersion` 发布 immutable `AcceptedTransitionDictionary`，把现有
  `transitionType → transitionSemanticId + FactContract` 冻结在代码发布物/签名 catalog 中；
  SourceLocation 使用 `(reducerKind,reducerVersion,transitionType)`。其稳定 outputId 和
  `causationSemanticSetHash/processingContextContractHash` 按 §6.1 派生 occurrence identity；
  对非直接 source cause 按 §6.1 的 reducer-owned 公式构造；有可信 transition sequence 时才携带
  `occurrenceOrder`。原始 Lifecycle/CommandReply 不得绕过 reducer 直接冒充 accepted transition。

诊断类 `ProtocolError/UnknownPoint` 不进入业务 Scene Fact。

## 7. 时间戳、版本与 LATEST 排序修正

### 7.1 时间策略所有权

`eventTs` 必须由受信 producer 根据版本化 `timestampPolicyRef` 产生，同时携带
`timestampSource`。Admission 只验证并复制，不根据到达延迟、设备 payload 猜测，也不在
缺失时临时取当前 wall clock。

来源策略至少冻结：首选时间字段、fallback 顺序、时钟可信条件、精度、允许范围、time
domain 和异常处理。`observedAt`/Canonical `receivedAt` 是平台观测证据，可用于 freshness
和默认 timer anchor，但不能替代 `eventTs` 的 LATEST 排序含义。

`factVersion` 是同一 subject/fact 的来源业务修订或可信顺序，只在来源 contract 明确定义
时存在。以下字段不得冒充 fact version：

- `modelReleaseId`、`moduleSchemaSnapshotId` 或 schemaHash；
- Kafka partition/offset；
- Admission 处理次数或本地自增计数；
- Scene Binding config revision。

LATEST_STATE 发布物必须二选一：提供可重建的 `factVersionPolicy`；或证明 timestamp 精度、
最大上报频率和 producer 行为不会合法地产生“同 subject/fact/eventTs、不同 payload”。若来源
允许同 timestamp correction/多次变化却没有可信 version，该 fact 不得标为 scene-eligible。
运行时 conflict 是数据契约破坏的 fail-closed 结果，不是正常高频遥测的 tie-breaker。

### 7.2 LATEST_STATE deterministic comparator

Current Fact、Scene SingleValue 参数、bootstrap/evaluation-time overlay 后合并当前 Delta 都必须
调用同一门控：

```text
compareLatest(old, new):
  if old 不存在:
    ACCEPT

  if old.factDeltaId == new.factDeltaId:
    return old.factExecutionEnvelopeHash == new.factExecutionEnvelopeHash
           ? DUPLICATE_NOOP
           : FACT_IDENTITY_CONFLICT

  if !sameOrderDomain(old, new):
    ORDER_DOMAIN_CONFLICT

  if new.eventTs > old.eventTs:
    ACCEPT
  if new.eventTs < old.eventTs:
    STALE_NOOP

  // eventTs 相同
  if exactlyOneHasFactVersion(old, new):
    VERSION_PRESENCE_CONFLICT

  if bothHaveFactVersion(old, new):
    // comparator(lhs, rhs) > 0 明确表示 lhs 比 rhs 更新
    c = compareCanonicalFactVersion(policyRef, new.factVersion, old.factVersion)
    if c > 0:
      ACCEPT
    if c < 0:
      STALE_NOOP
    return semanticPayloadEqual(old, new)
           ? DUPLICATE_NOOP
           : SAME_ORDER_DIFFERENT_PAYLOAD_CONFLICT

  // 两边都没有 version
  return semanticPayloadEqual(old, new)
         ? DUPLICATE_NOOP
         : AMBIGUOUS_SAME_TIMESTAMP_CONFLICT
```

规则含义：

- 先比较 `eventTs`；较新的时间不得因为 version 为空而被拒绝；
- 在 order 比较前先检查相同 `factDeltaId` 的完整 execution envelope；ID 相同而
  timestamp source/time/version/context/payload 任一字段漂移都是 producer contract conflict，
  不能因其中一个版本看似较新而接受；
- 比较前必须验证 semantic contract、time/version domain 与 canonical encoding；不兼容返回
  `ORDER_DOMAIN_CONFLICT`，不能进入 timestamp 分支；
- 只有 `eventTs` 相同时才比较 `factVersion`；
- value 与 `UNAVAILABLE` 使用完全相同的门控；
- 同一 order、相同 payload 是幂等重复，不刷新 freshness、不触发求值；
- 同一 order、不同 payload 不能按 Kafka offset 或到达先后“最后写赢”，必须 quarantine；
- version 一侧存在、一侧不存在时无法证明可比较，必须 quarantine；
- 更高 version 即使规范化值相同仍是被接受的来源更新；是否形成业务 transition 由
  `change/edge/FSM` 决定。

### 7.3 OCCURRENCE 门控

OCCURRENCE 不参与 Current Fact comparator：

- 新 `occurrenceId` 是独立发生；
- source producer 必须保证相同 occurrence identity 重建相同 §6.1 execution envelope。Admission 的
  bounded cache 只能合并共享同一 Kafka publish outcome 的**并发 in-flight** 请求，或检测近期
  identity/envelope conflict；它不能作为 durable terminal skip 证据。publish callback/ACK 不确定、
  Admission crash 或普通 source redelivery 时，必须不看 current interest 而重新发布相同
  occurrenceId/envelope，不能因进程内 seen 标记推进 source offset。Runtime 在每次消费时仍按
  current Binding 建 0..N fanout；
- Scene V1 不新增 source-wide durable occurrence ledger。每个活跃 BindingState 可保存有界
  occurrence dedupe/operator state，但 `scene_state` 是异步 best-effort，恢复后允许同一 logical
  trigger 重算；稳定 transition/effect identity 和 §11.3 PG EffectSetReceipt/Action Outbox 只保证
  已接受的非空动作集合不能分叉。dispatcher 的每次物理重投必须携带同一 idempotency key；目标端是否物理
  exactly-once 取决于其幂等契约，本文不作该承诺；
- identity conflict 在第一条已被接受后才发现时，只隔离冲突副本并告警，不回滚已接受的
  state/action。需要“跨所有 Binding、跨无限时间全局只计一次”的业务必须由上游 durable
  accepted-event reducer/Computed Metric 提供，不属于 Scene V1；
- `eventTs` 用于审计、semantic trigger time 和允许的 bounded window，不把多个事件折叠为
  “最后一个事件”；
- `occurrenceOrder` 只能作为来源证据、identity conflict 校验和确定性审计排序；Scene V1 没有
  ordered-prefix frontier/watermark、gap repair 或 restore baseline，因此**即使携带可信 order**，
  也不能把直接 OCCURRENCE 交给 sequence、edge、change 或依赖先后的 FSM。只允许
  occurrence-local predicate/action，以及 order-insensitive、occurrence-idempotent 的 bounded
  aggregation；需要顺序状态时先由上游 durable reducer/ComputedMetric 接受并输出最终
  AcceptedStateTransition/LATEST_STATE，Scene 不自行猜测缺失前缀；
- Kafka 到达顺序只能是执行顺序，不能被宣传为来源业务顺序。

## 8. Scene Current Fact

本节保留决策摘要；完整模块规格（数据模型、写入者与完成谓词、排序与冲突、读取 API 与
readiness、双档部署契约、存储指引、写入范围收缩、前提与验收、运维 runbook）见
`亿级点位高吞吐架构/6.规则引擎与场景联动/Scene current fact设计.md`。本文其他章节引用的
"§8.4 provider/barrier"对应模块规格 §5。

- 逻辑上必须存在独立的 `SceneCurrentFactStore`（Scene-live 槽）：可复用通用 latest 的物理
  backend，但必须使用独立 table/namespace/column family 和权限；只收受信
  `production + LIVE` 的 `LATEST_STATE`；它不是 `scene_state`，也不从 `scene_event` 反向
  构建；BACKFILL 不得创建、推进或删除其 value slot（但 frontier 照推）。
- 逻辑 current key 为 `(tenantCode, factOwnerKey, factType, factKey)`；`UNAVAILABLE` 是正常
  current value，不用 Kafka/storage tombstone 表示。
- 每种 LATEST_STATE 来源有且只有一个写入者：PROPERTY 是 Materializer 的 Scene-live 槽
  （沿用其唯一逻辑 group/checkpoint），COMPUTED_METRIC 是
  `ComputedMetricCurrentFactProjector`（独立 group
  `scene-current-fact-computed-metric-v1`）；全部启用目标 durable 后才推进连续
  `readyThroughNextOffset` 水线；Admission 与 Runtime 都不是写入者。
- 冲突写 durable fence/outcome 后按终态推进，lookup 优先 `PROJECTION_CONFLICT`；读取六
  状态，`READY_NEVER_SEEN` 须 coverage seed 背书；求值前 overlay 与 triggerRight 分离。
- 部署为一套读取接口、两个保障等级（`projectionTier = FULL | DIRECT_TD`）；DIRECT_TD 档
  以《质量码契约设计》为硬前提。差异矩阵与 tier-invariant 规则见模块规格 §6。

### 8.5 通用 latest 降级为查询层能力（V1 默认不启用）

§8.1 分槽之后，Scene 只读 Scene-live 槽，通用 latest 失去正确性消费者：其剩余读取方
（仪表盘、设备详情、latest 查询 API）都是展示型查询，语义就是"最新一行"。BACKFILL
补的老数据 eventTs 更小、赢不了 last；newest 由 LIVE 路径持续刷新；TDengine 内置
last 按主键 ts 取 max，天然免疫 BACKFILL 乱序到达。因此通用 latest 从 Materializer
的必选物化目标降级为可选部署能力，三档开关（示意名 `latestProjection`）：

- `off`（V1 默认）：不物化。业务查询由 TDengine 内置 last over history 承载
  （history database 配置 `cachemodel=last_row/both`）。Materializer 只写 history 与
  Scene-live 槽，completion predicate 相应只含这两个必选目标——唯一 writer 的耦合面
  同步缩小；
- `tdengine-table`：独立 TDengine latest 表，Materializer 按父架构冻结的 5 元组 CAS
  （effectiveTime/sourceEpoch/deviceSequence/receivedAt/reportId）显式写入，保留
  LIVE/BACKFILL mode 区分。仅当出现正确性消费者（而非展示消费者）时启用；
- `redis`：高读 QPS 场景的读扩展档，语义同 `tdengine-table`。

已知语义差异：`off` 档下同 effectiveTime 冲突按到达顺序 tie-break（展示场景可接受），
BACKFILL 修正在查询视图可见（通常正是业务想要）。需要确定性 tie-break 的业务按业务
升档，不把全局默认拖回严格 CAS。

前提（blocker，不过不启动）：

1. 全仓盘点通用 latest 读取方，确认无正确性消费者；若有，先迁入 Scene-live 槽或专用
   投影；
2. 确认 history 表主键 ts 即写入时已校验的 effectiveTime（含 deviceTime 可信度回退）；
   否则错误未来时间戳会污染 last 视图，须先在写入路径定型，或显式接受并文档化；
3. TDengine last 在目标规模（亿级点位、生产 vnode 分布）下的 p99/并发/内存 POC 签字。

实施顺序：P0 前提验证 → P1 apisvr/仪表盘查询切换为 last over history（双轨期与物化
latest 抽样比对，一致率 100%，同 ts tie-break 差异逐例记录）→ P2 关闭物化写入
（观察 Materializer lag、查询 p99、Scene 回归后清理存量；开关保留至少一个版本周期
作回退通道）→ P3 随 §13 同次文档修订收口。回退只需把开关切回 `tdengine-table`；
history 从未停写，无需数据迁移。

明确不做：不碰 Scene-live 槽及其 writer/comparator/coverage 语义；不改 Scene 任何
求值路径；不删 history 冲突 side record outcome；不承诺历史 as-of 查询。

验收：双轨比对一致率 100%（已知 tie-break 差异除外）；latest 查询 p99 不劣于现网
基线；峰值写入下 Materializer lag 不劣于降级前；§14 相关测试全绿；全仓无
"Scene 依赖 current latest store" 残留表述。

## 9. Scene Admission 规范化流程

每个来源 input 固定执行：

```text
0. 验证受信 ProcessingContext；BACKFILL 在 source projection/fanout 前 deterministic bypass，
   REPLAY/shadow 不进入生产 Scene，缺失/非法/不可信组合 quarantine
1. 执行 source-kind gate；non-final metric 为 UNSUPPORTED_FINALITY_BYPASS terminal no-write
2. 验证 source output identity、release/snapshot ref、contract version/hash
3. 按历史来源发布物解析 FactContract；不得读取 current model 猜测
4. 映射 state/quality、规范化类型/单位、验证 eventTs/version/occurrence identity
5. 校验本条输入的 fact count、bytes 和全部 semantic invariants
6. 没有静态 Scene-eligible 更新：记录 deterministic no-output 并完成输入
7. 至少一个静态 Scene-eligible 更新：按 immutable source artifact 的 eligibility dictionary 把
   本批全部合格更新装入至多一条 SOURCE_DELTA；不得读取 interest generation 决定是否发送
8. publish callback/失败策略完成后，才允许 Admission completion sequencer 推进连续 offset
```

PropertyReport 只能按 `(moduleSchemaSnapshotId, pointOrdinal)` 查 dictionary。一个完整
Canonical report 可以含多个 module span；Admission 不按点拆 Kafka record，也不重新拼接
transport batch。

一个静态 Scene-eligible fact 的永久 contract/type/identity 错误会使整个 source input quarantine，禁止
先发布其余“成功字段”。原因是部分 batch 会破坏多点位同 generation matching、one-target-one-
bundle 和每 Binding 每 bundle 最多求值一次。`ABSENT` 和按 contract 合法产生的
`UNAVAILABLE` 不属于错误。

依赖暂时不可用时：

- source artifact/catalog/cache 暂时不可用：pause partition、有界 retry，不提交越过；
- Runtime interest bitmap/cache miss：在消费侧 bounded rebuild 或 retry，不返回部分 fanout；
- source release 永久不存在、hash 冲突、未知 ordinal、类型不兼容：整条 quarantine；
- SOURCE_DELTA 发送失败：先有界 retry，耗尽后整条 quarantine/告警并按既有终态策略推进，
  不能发布部分 record 或无证据地只推进 offset；
- publish ACK 不确定或 source redelivery 时，必须从 historical source artifact 重建并重发相同的
  完整 eligible fact 集、`sourceEventId`、fact identities、eventTs、observedAt、
  factVersion/occurrenceOrder 和 payload；Scene 启停导致的 current interest generation 变化既
  不得改变同一 ID 的 record 内容，也不得把该次重试变成 no-output。

## 10. Scene 发布与 UI 契约

Scene Binding 的正式参数引用固定为
`FactSelector(factType,factKey,optional fieldSemanticId)`。scalar fact 不得带 field；结构化
DEVICE_EVENT/transition 参数必须保存稳定 `fieldSemanticId`，并在发布/active-release 再校验时
同时验证 root `semanticContractHash`、`payloadSchemaRef` 和 field contract。TriggerPosting 仍只按
root `(factType,factKey)` 建索引，field selector 只用于命中后的 typed payload 投影。发布时解析
FactContract/payload schema，并拒绝：

- 不存在、不可见、非 scene-eligible 或 `semanticContractHash` 无法解析的 fact；
- 期待类型/量纲与 canonical contract 不兼容；
- 把 OCCURRENCE 配为 `CONTEXT_ONLY`、latest bootstrap 或 freshness current source；
- 把 `CONTEXT_ONLY` 参数配置成 rolling；
- DEVICE_EVENT 参数 selector 不是 V3 dictionary 中的稳定 `eventParamSemanticId`；
- 把 OCCURRENCE 直接交给 sequence/edge/change/依赖先后的 FSM；即使有 occurrenceOrder，V1
  也没有 ordered-prefix finality；
- 同一参数跨来源使用不可比较 time domain，却选择 `EVENT_TS` timer anchor；
- 选择 `EVENT_TS` 的 Fact 缺少可解析的 `schedulerDeadlinePolicyRef`，或其 time domain 不能
  确定性校验/转换为可信平台 UNIX epoch deadline；
- 试图在 Scene 内覆盖来源 `temporalKind`、timestamp policy 或 fact-version policy；
- wildcard/whole-record 依赖，或把 source subject 拼入 fact identity。

Scene 配置人员只看到业务名称、来源、canonical 类型/单位和只读时间形态；稳定 UUID 可在
审计详情中展示但不要求人工录入。

### 10.1 timer anchor

duration、debounce 和 Alarm duration 类算子允许逐算子配置：

```text
timerAnchorPolicy = OBSERVED_AT | EVENT_TS
```

这是用户已确认的新裁决，显式替换父架构和 Scene 主规格中“deviceTime/sampleTime 绝不驱动
timer deadline、持续时间一律按处理时间”的绝对表述。替换后的精确边界是：未经验证的
device/sample 字段仍绝不能直接驱动 timer；只有经来源 `timestampPolicyRef` 规范化并通过
time-domain 发布校验的 Fact `eventTs`，才可在算子显式选择 `EVENT_TS` 时成为 anchor。

- 默认且推荐 `OBSERVED_AT`：使用来源 envelope 已冻结、redelivery 保持不变的
  `FactDelta.observedAt`，不得在 Runtime retry 时重新调用 wall clock。deadline 已经过期时，
  Runtime 在 current generation/value/freshness 复核通过后立即 fire；
- `EVENT_TS`：使用来源 contract 冻结的业务时间；除了所需参数 time domain 可比较，还必须由
  `schedulerDeadlinePolicyRef` 确定性校验/转换为可信平台 UNIX epoch deadline。boot-relative、
  仅设备局部单调或其他无法映射到 scheduler epoch 的时钟，即使彼此可比较也不得 arm；
- 一个完整 bundle 有多个取得 `triggerRight` 的 TRIGGER Delta 时，`OBSERVED_AT` anchor 取这些
  Delta 的 `max(observedAt)`，`EVENT_TS` anchor 取可比 domain 内的 `max(eventTs)`；
  CONTEXT_ONLY overlay、stale/duplicate/conflict Delta 不参与 anchor。anchor、operator generation
  与 semanticHash 必须进入 timer state/fire identity，retry、rebalance 或 state restore 后的
  `dueAt/timerFireId` 不得漂移；timer slot/fire 还必须保存 data-origin `SceneSourceLineage`，或
  schedule/lifecycle trigger identity+version，并在 restore/callback 时通过 cutover allowlist；
- `EVENT_TS` 迟到输入算出的 deadline 已过期时，同样只在 current generation/value/freshness
  复核通过后立即 fire，不回写历史时刻，也不撤销已接受动作；
- Scene V1 不因此引入 watermark、allowed-lateness retraction 或历史动作撤销；
- cooldown 固定从 §11.3 receipt 的 DB-authored immutable `acceptedAt` 开始；
- reliable delayed action 使用 PG Outbox `due_at`；
- 非 FactDelta 触发器的 anchor 采用下表，不允许实现自行读取 Runtime wall clock：

  | 求值起因 | `OBSERVED_AT` 新 arm | `EVENT_TS` 新 arm |
  | --- | --- | --- |
  | data ArgumentDeltaBundle | 取得 triggerRight 的 Delta 的 `max(observedAt)` | 同批 Delta 的可调度 `max(eventTs)` |
  | Schedule | immutable `plannedFireTs` | 禁止；本轮没有 Fact triggerRight |
  | SubjectLifecycle | 受信 lifecycle envelope 的 immutable `lifecycleObservedAt` | 禁止；本轮没有 Fact triggerRight |
  | timer fire/reconciliation | 只复用原 slot 的 `anchorTs/dueAt`，不得为该 slot 重选 | 同左；不得以 current fact 的时间重锚 |

  timer-origin 求值不能仅因读取了 Current Fact 就为另一个 duration/debounce slot 新 arm；需要新
  data、Schedule 或 SubjectLifecycle trigger 按上表取得合法 anchor。无合法 anchor 时该 operator
  fail closed；发布器能静态证明必然无 anchor 的配置直接拒绝；
- freshness 使用 `observedAt/receivedAt` 元数据；这些时钟不由 `timerAnchorPolicy` 改写；
- policy 及其时间域依赖必须进入 Binding `semanticHash`。修改后旧 timer/state 不得 preserve，
  必须 close+tombstone+next-live bootstrap。

## 11. Runtime SingleValue、rolling 与 bundle 语义

### 11.1 SingleValue

- `LATEST_STATE` 参数按 §7.2 更新；
- `OCCURRENCE` 不能作为可 bootstrap 的 SingleValue context；其 payload 只在本次触发 bundle
  内可用，求值后不能被保存为供后续 trigger join 的“最后一次事件”。Scene state 只能保存
  bounded dedupe、rolling 或显式 operator/FSM state；
- 被接受的 TRIGGER 更新可以发起一次 bundle 求值，即使规范化值相同但 version 更高；
- `change/edge` 使用自己的 previous canonical value/predicate 判断是否产生 transition；
- stale、duplicate 或 conflict 不刷新 freshness，不参与 semantic trigger time；
- 同一完整 source/target bundle 合并完后，每个 Binding 最多求值一次。

触发权和 redelivery 所需 identity 必须进入 bounded `SceneBindingState`，不能只存在进程栈：

```text
SingleValueCell {
  retainedFactProvenance
  availability/value
}

BindingEvaluationMeta {
  lastEvaluatedTriggerIdentity
  lastEvaluatedTriggerKind, lastEvaluatedTriggerVersion
  boundedRecentTriggerIdentities   // 每项含 kind/version/id；仅覆盖 in-flight/reorder budget
  lastAcceptedSourceLineage?       // data trigger 的完整 identity/version tuple
  identityCutoverManifestId
}
```

rolling cell 另保存 sample `factDeltaId`，OCCURRENCE 使用受 `maxSamples/maxStateBytes` 限制的
dedupe set；每个 sample/dedupe entry 同样保存完整 `RetainedFactProvenance`。这里不是跨无限时间的
global Inbox；异步 `scene_state` 落后或丢失后允许重算，
但必须复用相同 trigger/transition identity，并由 §11.3 receipt 同时约束完整 effect set。

### 11.2 bounded rolling

这是对父架构“V1 不保存原始窗口样本、所有窗口一律交给 Flink”旧表述的显式联合修订：
Scene V1 允许**单 evaluation subject、单参数、严格受 32 KiB state/1000 samples 等平台硬
上限约束**的小窗口；跨设备聚合、JOIN、session、CEP、watermark、迟到修正/retraction 和
超过该上限的窗口仍必须使用共享 Computed Metric/Flink。父架构、主规格、算子契约和 SLO
必须在同次变更中采用这一条边界，不能继续各写一套。

每个 rolling 参数必须是 TRIGGER，并在发布时声明 `windowDuration`、`maxSamples`、
`maxStateBytes` 和 `rollingBootstrapPolicy`。每个参数 state 同时保存有界
`sampleIdentityIndex`；LATEST 的主 identity 是 `factDeltaId`，OCCURRENCE 的主 identity 是
`occurrenceId`，timestamp 只用于窗口排序。由 window reader 安装的每个**active sample**还保存
有界 `runtimeTriggerObserved=false`；只作为 replaced/dedupe evidence 的 identity 不带触发资格。
正常 SceneEvent 首次完成该 Binding 的求值时将对应 active entry 置 true；marker 与 sample 一起
计入 byte/sample cap 并在 cutoff 时 prune。应用单个 delta 的算法固定为：

- §7.2 的跨 timestamp stale 判断只适用于 Current Fact/SingleValue；rolling 是不同 argument
  mode，窗口内较旧 timestamp 可以成为独立样本。二者只复用“同 timestamp 的 version/payload
  correction”规则，不得把 SingleValue stale-noop 套到 rolling；
- 先按主 identity 查询 `sampleIdentityIndex`。LATEST 比较 §6.1 的
  `factExecutionEnvelopeHash`，OCCURRENCE 比较 `occurrenceEnvelopeHash`：相同 identity/hash 的
  value merge 是 duplicate；通常 no-op，但 active bootstrap entry 的
  `runtimeTriggerObserved=false` 时仍须保留一次 triggerRight，并在本轮成功求值的 tentative state
  中原子改为 true。相同 identity、不同 eventTs/version/order/observedAt/payload 等任一执行字段是
  identity conflict，必须在改变 head/window 前 fail closed；
- identity 未见过后，计算 `candidateHead=max(maxAcceptedEventTs,new.eventTs)` 和
  `cutoff=candidateHead-windowDuration`；只有 `new.eventTs < cutoff` 才是 outside-window no-op，
  timestamp 等于 cutoff 的边界样本保留；
- LATEST_STATE cell key 为 `eventTs`，并保存 `factDeltaId/factVersion/semanticPayloadHash`；同
  timestamp 修订调用 §7.2 的 equal-time version/payload 分支，得到 replace/duplicate/conflict。
  replace 时旧 cell 退出数值窗口，但其旧 identity/hash 作为 bounded dedupe entry 保留到该
  timestamp 被 cutoff prune，防止旧 ID 以另一内容返回；
- OCCURRENCE cell 以 `occurrenceId` 定位，另以 `(eventTs,occurrenceId)` 建 deterministic sort
  index，允许多个不同 occurrence 共用 timestamp；
- 在 tentative copy 上 apply/replace，再删除 `eventTs < cutoff` 的 cell，然后检查
  `maxSamples/maxStateBytes`。只有全部成功才提交新 head/window；conflict 或超配额时不能推进
  `maxAcceptedEventTs`，也不能留下半个窗口。active sample、sort index、当前/被替换 identity
  entry 必须原子更新；到 cutoff 时一起 prune，且 active sample 数、identity entry 数与总 bytes
  分别受发布期 hard cap 约束，不能用大量 correction 绕过 `maxSamples/maxStateBytes`；
- retained window/dedupe horizon 内的 redelivery 不新增样本；同 retained sample/occurrence identity
  的 execution envelope 任一字段不同都必须 conflict。样本被 prune 或 best-effort state 丢失后
  不承诺全局 conflict ledger，
  仍依赖 producer identity contract 与 trigger-level effect-set receipt 收敛，不能追溯回滚旧动作；
- 同一 PROPERTY 同时被两个参数以 SingleValue 与 rolling 引用时，旧 timestamp 对前者可为
  stale、对后者可在窗口内 accepted；完整 bundle 合并后 Binding 仍最多求值一次；
- 不提供 watermark、跨设备 event-time 对齐、session/CEP 或 action retraction；
- OCCURRENCE 不论是否携带可信 order，都只允许 occurrence-local 或
  order-insensitive、occurrence-idempotent 聚合，不允许 sequence/edge/change 类解释；order
  只参与 evidence/conflict，不构成 ordered-prefix finality。

LATEST rolling 对 availability 的处理固定如下：`UNAVAILABLE` 作为带 reason 的 marker 保存，
参与同 timestamp correction 并计入 sample/byte cap，但不进入数值聚合；若最新 accepted
LATEST_STATE cell 是 UNAVAILABLE，参数按既有 missing policy 进入 NOT_READY/cancel-or-hold，
直到更新的 AVAILABLE 到来。较旧 UNAVAILABLE 不得使更新的 AVAILABLE 失效。AVAILABLE →
UNAVAILABLE → AVAILABLE、迟到 UNAVAILABLE 和同 timestamp correction 均走上述同一算法。

窗口内迟到样本被接受时，以该 FactDelta 的稳定 `sourceEventId/factDeltaId` 获得一次 rolling
triggerRight；可能产生新的 transition/effect，但已经接受的旧动作不会自动撤销或补偿。同一
trigger 重试仍以相同 trigger/transition identity 和 effect-set receipt 收敛。

rolling 只属于 `SceneBindingState` 的有界参数历史，不把完整窗口复制进 Current Fact，也不按
Scene 扫描全量 TDengine history。冷启动/恢复只能读取业务存储明确提供的受限窗口；缺失历史
不做 Canonical replay。窗口 reader 的每个样本都必须证明 trusted ProcessingContext 为
`production+LIVE`，并返回完整 `RetainedFactProvenance`、availability/unavailable marker 与
canonical value；还必须给出与这些样本一致的 source frontier/read barrier。任何样本 provenance 缺失、
混入 BACKFILL/REPLAY、marker/identity/order 字段不全或 frontier 无法证明时，结果都不能成为
`ROLLING_COVERED_AT_BARRIER`，只能 PARTIAL 或改用 ComputedMetric/Flink。

没有 watermark/allowed lateness 时，系统永远不能声称“未来不会再到达窗口内
旧 eventTs”，因此禁用含糊的 `ROLLING_READY`，只允许以下状态：

- `ROLLING_COVERED_AT_BARRIER`：受信业务存储按相同 FactContract、严格
  `production+LIVE` provenance 和 source frontier/read barrier 返回上述完整受限窗口，证明截至
  该 processing frontier 的已知合格输入均被考虑；它不保证未来无迟到；
- `ROLLING_PARTIAL_WARMING`：无受信历史，从空窗口开始；
- `ROLLING_PARTIAL_OBSERVED_WARM`：reset 后第一个 accepted production+LIVE rolling sample 将
  immutable `warmupStartedAt` 设为其 `observedAt`；每个之后 accepted sample 执行
  `maxAcceptedObservedAt=max(old,new.observedAt)`。乱序较小 observedAt 不回退起点/上界；
  AVAILABLE 与被接受的 UNAVAILABLE marker 都推进该 processing-observed 上界，duplicate、
  conflict、outside-window no-op 和非 LIVE 不推进。仅当
  `maxAcceptedObservedAt-warmupStartedAt >= windowDuration` 时进入该状态。这不是连续输入证明，
  也不是完整 event-time window；数值仍受 missing policy 门控。

`rollingBootstrapPolicy=REQUIRE_COVERED_AT_BARRIER` 是默认值：未取得第一种状态时不允许
rolling-dependent transition/action，应改用提供 watermark/迟到契约的 ComputedMetric/Flink。
高级用户可显式选择 `ALLOW_PARTIAL_AFTER_OBSERVED_WARMUP`；此时只在第三种状态后允许动作，
UI/审计必须标记“迟到数据可能继续修正、无 retraction”。**每次 restore 或 rebalance 后，无论
恢复出的 rolling state 是否非空，都必须把其 coverage status 视为未验证**：V1 的异步
`scene_state` 无法证明已覆盖 Runtime committed input frontier，禁止沿用旧 `COVERED/WARM`
资格。Runtime 必须在新 production+LIVE barrier 下从受信 window reader 全量替换重建，成功后
才回到 `ROLLING_COVERED_AT_BARRIER`；不能证明时清空旧 sample/index，进入
`ROLLING_PARTIAL_WARMING` 并按 policy 重新 warm。未来只有 state 携带并验证每个 rolling source
route frontier 覆盖 committed frontier 的新协议，才可另行允许 preserve；本文 V1 不提供。

restore/rebalance 的 rebuild 必须使用无缝 handoff，不能一边读旧 history 一边直接处理新样本：

1. actor 先把受影响 Binding 标为 `ROLLING_BOOTSTRAPPING`，禁止所有 data/schedule/lifecycle/timer
   业务求值与动作；到达的 SceneEvent 进入受 `maxBootstrapBufferedRecords/bytes` 限制的有序
   mailbox，consumer 不得提交越过这些 record；容量将满时 pause partition，不能丢弃或先求值；
2. 通过 §8.4 provider 请求一个**登记后新 capture cycle** 的 per-route
   `rollingBootstrapBarrier`，等待 window reader 证明 production+LIVE history 已完整到各 entry 的
   `readyThroughNextOffset`；旧 provider snapshot 不得复用；
3. 在 tentative state 中全量安装 reader 返回的 window、identity/sort index 和 coverage token；
   每个 active history sample 初始化 `runtimeTriggerObserved=false`，replaced/dedupe-only entry 不得
   获得该 marker。若无法证明则安装空 `ROLLING_PARTIAL_WARMING`，绝不能保留 restore 出的旧 sample；
4. 再按原 `scene_event` partition 顺序应用 mailbox。history 已含相同 fact/occurrence identity 与
   envelope 时，sample merge 是 duplicate，但尚未被该 Binding 处理的 buffered SceneEvent 仍按
   上述 marker 获得一次 triggerRight并原子置 true；identity 内容不同则 conflict。这样 reader 与 buffer 的
   overlap 既不重复样本也不吞触发；
5. mailbox 排空且 state/coverage 再校验成功后才退出 bootstrap。期间 schedule/lifecycle/timer
   callback 保持 retry/deferred；任一步失败保持 fail closed，不得用半个 snapshot 开放动作。
   退出后才到达、但已被 reader 预装的 active identity 仍走同一个 marker 规则，不能因不在
   bootstrap mailbox 中就被普通 duplicate 分支吞掉；若它在到达前已随 cutoff prune，则按正常
   outside-window 规则 no-op，不复活历史动作。

### 11.3 trigger-level effect-set 与 payload 收敛

同一 trigger 因 crash 后读取到更新的 Current Fact baseline，既可能重算出相同 `effectId`、不同
payload，也可能得到完全不同的 effect ID 集。只给单行 `effectId` 做唯一键无法阻止后一种
分叉。Action Outbox table 因此增加三种**不投递**的保留 rowKind，而不是新增通用 Inbox/state
事务：

```text
effectSetReceiptId =
  H(SCENE_ID_V2, EFFECT_SET_RECEIPT_V1,
    processingNamespace, tenantCode, bindingId, evaluationSubjectKey,
    triggerIdentity, bindingSemanticHash)

canonicalEffectSetHash =
  H(EFFECT_SET_V1, sortByEffectId(effectId, actionPayloadHash)[])

canonicalMutationPlanHash =
  H(EVALUATION_MUTATION_PLAN_V1,
    canonicalEffectSetHash,
    canonicalOperatorStateDeltaHash,
    sort(logicalTimerId,operation,anchorTs,dueAt,operatorGeneration)[],
    canonicalCooldownMutationHash)
```

每个 action row 必须保存 canonical `actionPayloadHash`，覆盖 action type、target、payload、
due/delivery policy、ProcessingContext 和完整 source/trigger identity-version provenance。产生非空
action set 时，receipt 同时保存 effect-set 与 mutation-plan hash；state delta hash 只覆盖会影响
后续业务求值的 operator/FSM semantic state，不包含 cache、trace 或非确定序列化噪声。
pre-hash action collection 中每个 `effectId` 必须唯一：同 ID/同 payload 的重复也属于非法
multiplicity，同 ID/不同 payload 是 `EFFECT_PAYLOAD_CONFLICT`。timer mutation 必须先规范化为每个
`logicalTimerId` 恰好一个 final operation；不能把同一 timer 的 cancel+arm 或多次 arm 仅排序后
交给执行器猜 precedence。

receipt 还必须保存由 PG 在首次获胜事务中生成且之后不可变的 `acceptedAt`。它不参与
`canonicalMutationPlanHash`：以 receipt acceptance 为 anchor 的 cooldown/延迟策略在 hash 中编码
anchor kind、policy 与 duration，不读取 Runtime wall clock 拼绝对时间；其他 event/scheduler
anchor 的 timer 仍保存由稳定语义时间算出的 `dueAt`。该轮按以下顺序提交：

1. 先构造完整 action set、tentative state/timer/cooldown mutation plan 及上述 hash，不能逐 action
   边算边提交；
2. 首次接受时，在**同一 PG Action Outbox 本地事务**插入
   `rowKind=EFFECT_SET_RECEIPT` 和全部 action row；唯一键竞争的获胜 insert 由 DB transaction
   timestamp 生成一次 `acceptedAt`。若 action/cooldown 使用 acceptance-relative delay，同一事务
   由该值计算并冻结 `dispatchNotBefore/cooldownUntil`；receipt 不会被 dispatcher 投递；
3. 已有 receipt 且 set hash、mutation-plan hash 都相同，是整轮幂等重算；同时逐行校验
   `effectId + actionPayloadHash`，不得补入 hash 集外的新 action。commit response 丢失、重启或
   restore 后必须读取原 receipt 的 `acceptedAt` 及派生时间，绝不能用 retry wall clock 重新起算；
4. 已有 receipt 但任一 hash 不同，原子写入以
   `(effectSetReceiptId,competingSetHash,competingMutationPlanHash)` 唯一的非投递
   `rowKind=EFFECT_SET_CONFLICT` 后结束；不得插入本轮任何新 action，当前 target/binding bundle
   的全部 tentative state/timer/cooldown 均丢弃，既有已接受 effect 不回滚；
5. conflict row durable 后，该 trigger 对本 target/binding 是 terminal quarantined，redelivery
   读取同一 outcome 并可推进 input completion，不能永久毒死 partition。它不永久熔断 Binding；
   下一个不同 trigger 可正常求值。其他 target/binding bundle 仍各自完成，不声称跨 fanout
   原子回滚；
6. 相同 `effectId`、不同 `actionPayloadHash` 仍是 row-level
   `EFFECT_PAYLOAD_CONFLICT`：在同一 Outbox table 写入以
   `(effectSetReceiptId,effectId,competingPayloadHash)` 唯一的非投递 conflict row，按同样方式
   durable terminal、整 bundle 不提交，作为 receipt/hash 实现错误、碰撞或并发损坏的第二道门禁；
7. dispatcher 每次物理投递携带相同 `effectId` 作为 idempotency key。唯一 logical Outbox row
   不等于目标端 exactly-once；目标不支持幂等时仍可能在“调用成功、投递状态未提交”窗口收到
   重复调用。

首轮求值为**空 action set**时不写 receipt，避免把 Action Outbox 变成每 candidate durable
outcome store；timer/state-only mutation 也仍遵循 Scene V1 已确认的异步 best-effort state 边界。
这类 trigger 在 input completion 前 crash 后可能按更新 baseline 重算，本文不伪装成 durable
exactly-once FSM。边界明确为：一旦任一非空 action set 已由 PG 接受，同一 trigger 的 action set
及其配套 semantic state/timer/cooldown plan 都不能分叉；acceptance-anchored cooldown 始终从该
receipt 不可变的 DB `acceptedAt` 开始，而不是从某次重试或 restore 开始。

## 12. 失败与处理结果矩阵

| 条件 | 处理结果 | 明确禁止 |
| --- | --- | --- |
| `production+LIVE` 且 contract 合法 | 按 immutable eligibility 规范化并产生 0..1 SOURCE_DELTA；interest 只在 Runtime 决定 fanout | 每点一条 Scene record，或用 current interest 抑制 Admission 输出 |
| `BACKFILL` | matching 前 deterministic bypass；Current Fact value 不变但 projector terminal frontier 推进 | 更新 Scene Current Fact/state/timer/action |
| `REPLAY/shadow` | 不进入生产 Scene | 借 Topic 名提升为 LIVE |
| non-final ComputedMetric | Admission/Current Fact projector `UNSUPPORTED_FINALITY_BYPASS` + terminal no-write | 进入 V1 Scene 或假设存在补偿 |
| ProcessingContext 缺失/非法/不可信 | fail closed + quarantine/告警 | 默认 LIVE |
| 同一 Schedule/SubjectLifecycle trigger ID 携带不同完整 ProcessingContext | `DIRECT_TRIGGER_CONTEXT_CONFLICT`，受影响 bundle 零 state/timer/action | 只比较 namespace/mode 或任选一个 context |
| Canonical `ABSENT` | 无 FactDelta | 当作 unavailable 或零值 |
| 合法 NULL/RESET/INVALID/bad quality | 明确 UNAVAILABLE | Kafka tombstone、静默丢弃 |
| source artifact/cache 暂时不可用 | partition retry/pause | 部分解析、越过 offset |
| release/hash/ordinal/type 永久冲突 | 整条 source input quarantine | 发布其余成功 fact |
| LATEST 较旧 | stale no-op | 降级 Current Fact/参数 |
| 同 order、同 payload | duplicate no-op | 刷新 freshness 或重复求值 |
| 同 factDeltaId、不同 execution envelope | `FACT_IDENTITY_CONFLICT`；writer durable terminal outcome，Runtime 隔离受影响 bundle且不给 triggerRight | 先走 exact-ID grant、按较新 timestamp 接受 |
| order domain、同 order 不同 payload或版本存在性冲突 | Current Fact writer 持久化 conflict outcome；Runtime 只 quarantine 受影响 ArgumentDeltaBundle | last-arrival-wins、声称跨 target 回滚 |
| 纯 `CONTEXT_ONLY`/当前无人引用的合格 LIVE 更新 | 唯一 writer 更新 LATEST Current Fact；Admission 仍发布稳定 SOURCE_DELTA，Runtime 当前 fanout 为零 | 因无 trigger posting 而漏写 Current Fact，或把 interest 当 emit gate |
| Current Fact durable write 失败 | writer 不推进连续 frontier，告警并重试 | 先推进 frontier 后补写 |
| Current Fact 等于当前 SceneEvent、且 BindingState 未处理该 ID | value duplicate，但当前记录保留一次 triggerRight | 因 projector 快一步而吞掉 trigger |
| Current Fact 比当前 SceneEvent 更新 | 以 cold baseline 为准，当前旧记录无 triggerRight | 回放旧 transition 或降级 current value |
| Admission publish ACK 不确定/崩溃后 redelivery | 不看 current interest，重建并重发相同稳定 ID 与 immutable eligible payload，由 Runtime/Outbox 收敛 | 改写同 ID payload、把 retry 变为 no-output，或用进程 recent-cache terminal skip 后推进 source offset |
| OCCURRENCE ID 重复且 execution envelope 相同 | hot state/cache 中 no-op；恢复重算复用 trigger/transition identity，已接受非空 action set 由一个 receipt 与逐 effect logical row 收敛 | 承诺不存在的全局 durable trigger-once/物理 exactly-once |
| OCCURRENCE ID 相同但 payload/time/observedAt/order 任一不同 | `OCCURRENCE_IDENTITY_CONFLICT`；检测到的后一个副本 quarantine/告警，已接受结果不回滚 | 只比较 value、任选 envelope 或声称可撤销已外发动作 |
| Current Fact projection behind/lookup fail/conflict fence | retry 或 `PROJECTION_CONFLICT` fail closed、零 action | 解释为业务 missing/false，或带着旧值继续求值 |
| rolling 窗口内迟到样本 | 以稳定 fact/trigger identity 插入、重算；已接受动作不撤销 | 套用 SingleValue stale-noop 或产生重试型第二个 effect |
| rolling 无可信 production+LIVE 历史/frontier | PARTIAL 状态；按 bootstrap policy 禁止动作或显式 observed-warm 后 best-effort | 混入 BACKFILL/REPLAY 或宣称完整 event-time window |
| restore/rebalance 得到非空但未证明覆盖 committed frontier 的 rolling state | 清空 sample/index，重建成功才 COVERED，否则从 PARTIAL_WARMING 重启 | 沿用旧 COVERED/WARM 资格并出动作 |
| rolling/state 超配额 | BindingState fail closed + 告警 | 裁剪有效点后继续出动作 |
| 同 trigger、不同 canonicalEffectSetHash 或 canonicalMutationPlanHash | durable `EFFECT_SET_CONFLICT`；本 target/binding 整组新 actions 与 tentative state/timer/cooldown 不提交；outcome 后 input 可完成、下一 trigger 可恢复 | action 相同便接受不同 state/timer/cooldown plan、永久卡 partition 或回滚其他 target |
| 相同 effectId、不同 actionPayloadHash | durable `EFFECT_PAYLOAD_CONFLICT`；按同一 terminal completion 协议保留旧 row并整 bundle 不提交 | 静默 first-wins/覆盖既有 Outbox |
| Action Outbox commit 失败 | 丢弃 tentative state/timer，重试 trigger | 先推进 state 或 cooldown |

建议新增的低基数指标至少包括：

- `scene_fact_projection_total{fact_type,availability,outcome}`；
- `scene_fact_contract_error_total{source_kind,reason}`；
- `scene_fact_order_total{temporal_kind,outcome}`；
- `scene_current_fact_read_total{status}`；
- `scene_current_fact_projection_lag{source_kind,partition}`；
- `scene_current_fact_outcome_total{source_kind,outcome}`；
- `scene_interest_bitmap_rebuild_total{outcome}`；
- `scene_occurrence_order_reject_total{operator_family}`；
- `scene_effect_set_conflict_total{action_kind}`；
- `scene_effect_payload_conflict_total{action_kind}`；
- `scene_current_fact_barrier_snapshot_total{outcome}`；
- `scene_current_fact_conflict_fence_total{kind,outcome}`；
- `scene_rolling_bootstrap_total{status,policy}`。

tenant、subject、fact UUID、Identifier、Topic 和 payload 不得成为普通 Prometheus label。

## 13. 联合修订范围

设计批准后必须在同一次联合文档变更中修改：

1. `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
   - Canonical 发布物、ModuleSchemaSnapshot dictionary、四类来源映射；
   - Scene Current Fact 与通用 latest 分槽；
   - Admission/Current Fact frontier、mixed-version 示例和验收门禁；
   - 把“一 snapshot/hash 一 STABLE”改为 source snapshot hash 与 moduleCode append-only STABLE
     分离；显式替换 processing-time-only timer 与 no-rolling 旧绝对表述。
2. `亿级点位高吞吐架构/5.模块级超级表/design-category-module-snapshot-release-2026-04-28.md (1).md`
   - snapshot 从 V2 升为 §3.4 的 V3 dictionary；补 property/event/param semantic ID、
     FactContract、静态 eligibility、发布兼容性，以及 source `schemaHash` 与 physical schema
     fingerprint 分离。
3. `亿级点位高吞吐架构/5.模块级超级表/design-module-level-supertable-2026-04-15.md.md`
   - Identifier/TDengine 列与 Scene 逻辑身份解耦，明确类型变化的新列不必切断语义 ID。
4. `亿级点位高吞吐架构/5.模块级超级表/design-module-kind-release-schema-2026-04-26.md (1).md`
   - release runtime truth 除 module/id/identifier/core 外必须携带稳定语义 ID 与 FactContract ref。
5. `亿级点位高吞吐架构/5.模块级超级表/design-ekuiper-schema-cache-2026-04-25.md (1).md`
   - SchemaBundle current/release cache 传输并校验 semantic ID、semantic/projection hash 与冻结
     ordinal。
6. `亿级点位高吞吐架构/5.模块级超级表/plan-category-module-snapshot-release-2026-04-28.md.md`
   - 删除 `schemaVersion=2` 硬编码，按 V3 codec/hash/migration 和测试重写执行步骤。
7. `亿级点位高吞吐架构/5.模块级超级表/plan-schema-cache-2026-04-29.md.md`
   - SchemaBundle 主迁移计划增加 V3 dictionary、projection hash 与 retained release cache。
8. `亿级点位高吞吐架构/5.模块级超级表/design-module-level-supertable-optimization-2026-04-23.md (1).md`
   - 删除把同语义物理点留给未定义 alias/aggregation 的反向契约，采用 source-owned semantic ID。
9. `亿级点位高吞吐架构/2.编码解码协议/large-binary-codec/1.design-memory-optimize-for-high-frequency-time-series-data.md.md`
   - 本地 PointID/EventID 只能是 release-local 索引，不能替代 property/event semantic identity。
10. `亿级点位高吞吐架构/2.编码解码协议/large-binary-codec/3.design-model-payload-pool-with-typed-value-arena.md (1).md`
11. `亿级点位高吞吐架构/5.模块级超级表/3.design-model-payload-pool-with-typed-value-arena.md.md`
   - `PointID` 继续是 release-local 紧凑索引；dictionary 另携带 `propertySemanticId`，不得把
     两者合并为同一业务身份；两份同源设计同时修正 NULL/INVALID 省略与现场排序分配身份的
     反向表述。
12. `亿级点位高吞吐架构/2.编码解码协议/large-binary-codec/5.design-property-typed-decode-and-verifier.md.md`
   - `PropertyDict/PropertyPointPlan` 读取 V3 冻结 ordinal、semantic ID 与 projection contract，
     不在 decode/verifier 热路径猜身份。
13. `亿级点位高吞吐架构/2.编码解码协议/large-binary-codec/6.payload-pool-impl-plan.md.md`
   - 实施计划改为读取发布时冻结 ordinal，不在进程内按 Identifier 重新分配业务身份。
14. `亿级点位高吞吐架构/2.编码解码协议/large-binary-codec/2.design-sql-builder-pool-for-tdengine-batch-write.md.md`
   - 删除 `sortedIdentifiersHash`/local PointID 可充当稳定跨发布契约的 fallback；SQL writer 只
     消费 ModuleWritePlan 的物理列映射。
15. `亿级点位高吞吐架构/2.编码解码协议/large-binary-codec/4.design-event-schema-lookup-cache.md.md`
   - event lookup meta 增加 `eventSemanticId`、parameter dictionary/contract ref；重复 event code
     对业务 Scene projection 必须 fail closed，不能沿用“保留第一条”制造不确定映射。
16. `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`
   - 正式 SourceDelta/FactDelta、Admission、Current Fact、evaluation-time Context overlay、
     identity cutover、完整 ProcessingContext、EffectSetReceipt `acceptedAt`、失败、容量、POC；
     容量模型必须按“静态 eligible 输入稳定发布、dynamic interest 只控制 Runtime fanout”重算。
17. `亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md`
   - 删除“后续 exact fact registry”歧义，改为来源 FactContract；
   - Binding 发布/active-release 再校验、dynamic interest 仅作 Runtime posting/fanout、无 Context
     hidden posting 但每次求值 overlay Current Fact，以及 temporal-kind/operator compatibility。
18. `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`
   - 修正 SingleValue comparator；补 temporalKind、availability、Current Fact、rolling 和
     timer anchor。
19. `亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md`
   - 增加 §14 的 24 项契约测试、投影 lag、冲突、ACK-ambiguity 稳定重发、receipt 时间不漂移和
     zero partial bundle 证据。
20. `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md`
   - Scene integration 的 wire、source identity、批量性、schema compatibility 与 failure matrix。
21. `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`
   - 只补导航级不变量和术语归属，不以 README 覆盖上述正式契约。
22. `亿级点位高吞吐架构/5.模块级超级表/plan-module-kind-scope-iot-ekuiper-2026-04-26.md.md`
   - 当前有效实施计划中的 `SchemaBundle{Model,Route,Meta}` 和 release row 同步增加 V3 semantic
     dictionary/FactContract ref；不能让实施计划继续生成缺少 stable semantic ID 的 runtime truth。
23. `亿级点位高吞吐架构/5.模块级超级表/plan-module-level-supertable-multi-agent-2026-04-23.md.md`
   - 若仍为有效实施入口，同步修正其中不含 semantic dictionary/ref 的 SchemaBundle；若已被后续
     计划取代，则在同次变更显式标记 superseded 并链接当前计划，不能保留为可执行的反向契约。
24. `亿级点位高吞吐架构/README.md`
   - 把 Automation 输入补齐为 PROPERTY、DEVICE_EVENT、COMPUTED_METRIC、STATE_TRANSITION
     四类；修正仍把 BACKFILL/REPLAY 写成 Automation ordering/replay 路径的目录级摘要。导航
     必须与 production+LIVE-only Scene 契约一致，不能以旧总览反向覆盖正式规格。
25. `亿级点位高吞吐架构/2.编码解码协议/small-json-codec/1.design-general-json-codec-for-low-point-devices.md`
   - small JSON 的 Property/Event dictionary 同样读取 V3 stable semantic ID、FactContract ref 与
     冻结 ordinal；不能只修 large-binary codec 而让低点位设备继续按 Identifier 猜 Scene 身份。
26. `亿级点位高吞吐架构/2.编码解码协议/原始流与标准流/topology-options.md`
   - 将 replay outbox/merge 的描述限定为允许恢复的 history/latest sink；明确不得合并回 Scene
     production state/timer/Action Outbox。若该候选已过时则显式标 non-normative/superseded，避免
     被实现者解释为本文已拒绝的 Scene replay evaluator。
27. `docs/superpowers/specs/2026-08-01-scene-processing-mode-contract-reconciliation-design.md`
28. `docs/superpowers/plans/2026-08-01-scene-processing-mode-contract-reconciliation.md`
   - 两份已实施 processing-mode 工件的两字段 `ProcessingContext`、仅 SCENE_ID_V2 的 lineage 与
     单一 cutover 描述同步升级到本文的 contract ref/hash、source/fact 子版本、三 offset 域、
     direct-trigger logical-time checkpoint/state barrier；direct-trigger identity 仍用既有 SCENE_ID_V2。
     或显式标记相关小节被本文取代，不能继续作为可执行旧 wire。
29. `docs/superpowers/plans/2026-07-23-kafka-data-plane-boundary.md`
   - 将只列 PropertyReport/DeviceEvent 的 Automation 输入补齐四类；若计划已完成归档，则显式
     标记 superseded/non-normative 并链接当前父架构与本文，避免历史计划继续充当实现契约。

30. `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
   - 按 §8.5 把通用 latest 降级为三档可选能力（V1 默认 `off`）：修订
     "TDengine history + latest store"、"mode-required latest"、"latest 使用冻结
     comparator 做投影 CAS" 与 Materializer completion predicate 等表述；Scene 文档中
     current latest store 的旧引用随第 1/16/17/18 项统一收口为 Scene Current Fact；
     `SLO故障测试与实施.md` 与容量文档补 TDengine last 查询的容量与告警口径。

31. `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene current fact设计.md`（新建）与
   `亿级点位高吞吐架构/2.编码解码协议/质量码契约设计.md`（新建）
   - Scene Current Fact 独立成章：本文 §8 收缩为决策摘要，完整契约以模块规格为准；双档
     部署契约（FULL / DIRECT_TD）、存储指引与写入范围收缩随模块规格 §6–§8 生效；
   - 质量码（availability/quality）契约归 ProtocolApplication 域，DIRECT_TD 档以其为硬前提；
   - 场景目录与 `2.编码解码协议/` 的 README 导读及顶层 README 索引同步登记。

如果其他模块快照文档仍把 Identifier、数据库 ID 或 pointOrdinal 描述为跨快照 Scene 身份，
同次变更必须一并修正；实施计划先用全仓检索确定精确清单，不能只修改上述最小列表后留下
反向契约。

## 14. 24 项验收测试矩阵

| ID | 场景 | 必须证明 |
| --- | --- | --- |
| F-01 | V3 S1/S2 同为 Modbus 地址 100、ordinal 可不同，`voltage_a/FLOAT32` 与 `voltage_a_v2/DOUBLE64` 沿用 P123 | 两种在线设备命中同一 Scene fact/semanticContractHash；TD 列分离；semantic-only snapshot 不新建 STABLE |
| F-02 | PROPERTY、DEVICE_EVENT/event parameter、metric output 与 transition dictionary 正常编辑/新发布 | 四来源 stable semantic ID 自动继承，V3/plan ordinals、FactContract、owner-policy 与静态 eligibility ref/hash 显式冻结，用户无逐项选择 |
| F-03 | 新建/复制、不兼容语义或 time/version/order/scheduler domain 改变；同一 active snapshot 重复 semantic ID | 自动新 ID 或发布失败；旧 ID 永不复用；active release 不产生两个 semanticContractHash，也不让两个点竞争同一 identity |
| F-04 | Scene 创建/修改/停用、interest generation，以及 source release eligibility true→false；旧 release backlog 晚到 | Scene 只改 interest/index/cache、不重发模型；release 激活会重校验 Binding，旧 Current Fact row 被 fence 且不可 bootstrap；晚到旧 artifact 以 `SOURCE_ARTIFACT_NO_LONGER_APPLICABLE` terminal no-write |
| F-05 | `B=10 → A trigger/bootstrap → B=20 → A trigger`，分别令 B 为纯 Context 与另一 Trigger；另测同报告 Trigger+Context 且 store older/newer | 第二次 warm 求值都 overlay B=20；纯 Context refresh 只写 Current Fact、不建 posting；本批 candidate 服从 comparator，绝不覆盖 newer store |
| F-06 | Canonical `ABSENT` | 无 Delta、无 Current Fact mutation、无 trigger |
| F-07 | dense/sparse `VALUE` 与 FLOAT32→FLOAT64/单位规范化 | 两种 lane 得到语义等价 scalar payload/type/unit，同时保留各自 source provenance |
| F-08 | NULL/RESET/INVALID/BAD_QUALITY 及 rolling AVAILABLE→UNAVAILABLE→AVAILABLE/迟到 marker | reason 不混为 tombstone/never-seen；marker 计配额但不进数值聚合；最新 unavailable 按 missing policy，旧 marker 不失效新值 |
| F-09 | 新 `eventTs`、version 为空 | 接受，不被旧的非空 version 阻挡 |
| F-10 | 旧 `eventTs`、version 更大 | SingleValue/Current Fact stale no-op，不覆盖较新时间 |
| F-11 | 同 timestamp 高/低 canonical version、不同 domain/encoding | 同 domain 按 policy comparator；高接受、低 stale；domain/encoding 不同为 ORDER_DOMAIN_CONFLICT |
| F-12 | 同 order/ID 冲突；B Current Fact 冲突 fence 后由无关 A trigger 读取；再来严格更新或 domain/identity error | writer 先 durable outcome+fence 再推进；A 零动作；仅可比较的严格更新原子清 fence，domain/identity 需 repair generation；Runtime 只隔离受影响 bundle |
| F-13 | Property Materializer/FINAL Metric projector 的 LIVE、BACKFILL、non-final 与 metric eligibility true→false→true | LIVE row durable 后推进；BACKFILL/non-final value slot 不变但 terminal frontier 可推进；false 激活前 fence、旧 backlog terminal no-write，true 需新 coverage seed；Admission/Runtime 不成为第二 writer |
| F-14 | mode/contract retention、retry、Topic recreate、interest 变化、multi-module artifact 顺序 permutation | 仅 production+LIVE；unknown/missing/mismatched ProcessingContext artifact fail closed；排列不改 sourceSemanticSetHash；interest 变化不改变静态 eligible 输入的 emission/payload；旧、新 topicGeneration offset 不可比较 |
| F-15 | DEVICE/AREA/PROJECT owner encoding；direct/synthetic accepted transition、eligibility=false/未知 dictionary 与原始 Lifecycle/CommandReply 旁路 | FACT_OWNER_KEY_V1/ref/hash 跨语言一致并隔离 tenant/tenure；synthetic cause hash 可重建；仅字典 cause+eligibility 允许的 accepted transition 进入 Scene，旁路/false/未知均拒绝 |
| F-16 | post-request barrier microbatch；seed effectiveFrom 晚于 obligation、generation 不同、uncovered prefix 后来一条 LIVE；跨 tenant 同 subject key | 并发 trigger 共用新 capture cycle而非旧 cache/per-eval RPC；所有错误 seed/后续 row 仍 NOT_READY；seed/current key 隔离 tenant；完整 coverage 才返回 READY_* |
| F-17 | Current Fact 为 older/none、exact ID、newer、semantic duplicate；warm/cold/state 落后或丢失，且另一 TRIGGER 参数 state 落后；state restore 后 catalog retention 校验 | triggerRight 与 value merge 分离；每轮 overlay 全部必要 LATEST SingleValue；完整 comparator/freshness/context/source+owner-contract provenance 可解析；恢复可重算但不降级 current baseline，缺 ref/hash fail closed |
| F-18 | OCCURRENCE publish ACK 丢失/redelivery，期间 current interest 保持、启用或停用；hot dedupe/state 丢失 | 静态 eligible 时始终重发相同 ID/envelope，interest 不改变 emission/payload；Runtime current posting 决定 0..N fanout；已接受 actions 由 receipt/rows 收敛但不宣称 exactly-once |
| F-19 | 相同 occurrenceId、payload 相同但 eventTs/observedAt/order 不同，以及 payload 不同的副本晚到 | envelope 任一差异均 conflict/quarantine 且不产生新增 mutation/effect；先前已接受 state/action 不回滚 |
| F-20 | V3 DEVICE_EVENT/structured transition fields；有/无 typed occurrenceOrder 的 sequence/edge/FSM 与无序聚合 | field/schema 与 order comparator 可验证；未知/重复 field 拒绝；有 order 也不能伪装 ordered prefix，所有顺序算子拒绝；occurrence-local/无序幂等聚合可通过 |
| F-21 | rolling identity/cutoff/warm；非空落后 restore；fresh barrier rebuild 时 scene_event 与 history overlap、buffer 超限；reader 预装 sample 的 SceneEvent 在退出 bootstrap 后才到 | identity/index/triggerObserved marker 原子且仅完整 LIVE 可 COVERED；bootstrap 期间零动作/offset 不越过，超限 pause；mailbox 内外 exact-ID 合并不重样本且各保留一次 triggerRight；replaced/pruned identity 不复活动作；失败清空 PARTIAL |
| F-22 | data/Schedule/SubjectLifecycle/timer fire 的完整 ProcessingContext、`OBSERVED_AT`/`EVENT_TS` anchor、backlog、过期 deadline 与 policy change | direct producer 可解析各自 policy ref/hash，Linked/timer 逐字段继承；非 data trigger 严格按矩阵；EVENT_TS 必须映射可信 UNIX epoch；`dueAt/timerFireId` 不漂移；过期先复核；无 anchor/不兼容拒绝；变更 reset |
| F-23 | 一个静态 Scene-eligible fact 永久非法的多点 Canonical/DeviceEvent | Admission 整条 source quarantine、zero partial SOURCE/LINKED bundle、zero action；与 Actor 局部 conflict 边界区分 |
| F-24 | 四来源 cutover + direct-trigger ProcessingContext entries/lifecycle vector/schedule checkpoint/CurrentFact generation/state tombstone ACK；已接受非空 set 后重算不同 action/state/timer/cooldown plan；PG commit response 丢失后延时 retry；首次 empty→nonempty | fact、Schedule、SubjectLifecycle producer 均以 manifest policy ref/hash、新 token/游标恢复且无旧 state 晚写；fact-input、lifecycle-input、scene_event 三个 offset 域独立排空/恢复且绝不交叉比较，schedule boundary 明确不是 offset；同 set+plan 幂等，任一分叉 terminal；retry/restore 复用原 DB `acceptedAt`、cooldownUntil/dispatchNotBefore 不漂移；empty/timer-only 重算明确属于 best-effort 边界 |

每项测试必须同时检查输出 record、Current Fact、Scene state/timer、Action Outbox 和实际
dispatcher delivery 中适用的零副作用断言，不能只验证函数返回值。

## 15. rollout 与兼容性边界

本规格不能仅根据现有文档假定线上已经存在带稳定语义 ID 的完整历史发布物。正式实施计划
必须先 inventory：

- current 与 retained `DmProductRelease/DmModuleSchemaSnapshot`；
- Scene 已引用的 Identifier/ordinal；
- 通用 latest 与待建 Scene Current Fact 的数据覆盖；
- 已存在的 event/metric/transition 定义及其可证明 lineage。

已有发布物缺 ID 时禁止按名称或地址在运行时临时生成。迁移只能在启用新 Admission 前由
受审计工具一次性分配/确认 lineage，产生不可变 mapping artifact 或新版本发布物，并冻结
mapping contract version。无法证明语义延续时必须分配新 ID。具体 DDL、迁移程序和 rollout
步骤属于后续实施计划，不在本设计中虚构已存在能力。

§6.1 的 `SCENE_SOURCE_ID_V1/SCENE_FACT_ID_V1` 会补齐此前未冻结的 Canonical→Scene identity
公式，并可能改变已有 `sourceEventId`，进而改变 transition/effect/timer 因果身份。它必须使用
一次受控 barrier cutover，禁止旧、新 Admission 双写同一 production lane：

1. 离线生成 Canonical V2→V3 property/event/parameter lineage mapping，同时分配或确认
   ComputedMetric 的 `metricOutputSemanticId` 与 AcceptedTransitionDictionary 的
   `transitionSemanticId` lineage；重编全部受影响 Binding 的
   `FactSelector(factType,factKey,fieldSemanticId?)`。即使 bindingId 保留，fact contract/identity
   改变也必须改变 Binding `semanticHash`；迁移期间冻结相关 model/plan/dictionary/Binding
   发布，避免 inventory 再漂移；
2. 先冻结 direct-trigger 控制面并形成 `nonDataTriggerBoundary`：SubjectLifecycle relay 捕获其
   upstream `subjectLifecycleInputEndVector`；每项同样包含完整 stream ID generation、partition 与
   end next-offset，处理到 vector 且完成 publish callback 后 fence。该 lifecycle-input offset 域与
   fact-input、`scene_event` 均不可比较；旧
   Scheduler 停止取得新 lease，完成 `scheduleCutoverInstant` 之前已 claim 的 fire，记录每个
   schedule 的 `lastCompletedPlannedFireTs/nextPlannedFireTs` 与 missed-fire policy 后 fence；这是
   logical-time checkpoint，不是第四种 Kafka offset。然后为
   四类事实 source stream 捕获 `sourceInputEndVector`，每项包含完整 sourceStreamId generation、
   partition 与 end next-offset。旧 Admission 必须逐 partition 处理到该 vector，包括 deterministic
   no-output、quarantine outcome 和所有 SOURCE_DELTA publish callback；PROPERTY/COMPUTED_METRIC
   的旧唯一 Current Fact writer 也必须把 row+frontier 完成到各自 vector 后 fence scene-live slot/
   projector 并记录旧 `projectionGeneration`。该 vector 只属于 upstream fact input，不能比较
   `scene_event` 或 lifecycle offset；
3. 旧 Runtime 进入 drain mode：timer scheduler 停止启动新的 callback 并等待已开始 callback/
   producer future 完成；Runtime 继续消费旧 `scene_event` 并允许完成有界 Linked fanout。每轮在
   所有当前 send callback 完成后捕获 `sceneEventEndVector[n]`，消费到该向量并完成 target bundle/
   Action Outbox terminal outcome，再捕获下一轮；直到连续向量相同且无 in-flight handler、Linked
   send、timer callback、**异步 scene_state producer send/callback** 或 Outbox transaction，才得到
   最终 `sceneEventEndVector` 并 fence 旧 Runtime/Linked/state producer、撤销旧 write token。尚未
   到期的旧 timer 不等待到未来触发，进入下一步 reset；超出有界
   fixed-point drain 的环路必须 quarantine/告警，不能带入新身份 lane；
4. 写 immutable `SceneFactIdentityCutoverManifest`，包含 old/new source/fact identity versions、
   保持不变的 direct-trigger `identityAlgorithmVersion=SCENE_ID_V2`、彼此独立的
   `sourceInputEndVector`、`subjectLifecycleInputEndVector`、
   `nonDataTriggerBoundary` 与 `sceneEventEndVector`；Canonical V3 snapshot/projection/FactContract、
   ComputationPlan/metric、AcceptedTransitionDictionary hashes；按
   `(producerKind,processingModePolicyReleaseId,processingContextContractHash)` 排序的
   ProcessingContextContract entries；`producerKind` 至少覆盖四类 fact source、`SCHEDULE` 与
   `SUBJECT_LIFECYCLE`，Linked/LifecycleTarget/timer 只登记其继承链，不另造 policy entry；按
   source artifact 排序的 factOwnerKeyPolicy ref/hash entries；old/new Current Fact
   projection generations、Scene state write generation、受影响 Binding、restore allowlist 和全部
   Admission/CurrentFact/direct-trigger/Runtime/state producer fencing token；
5. 对受影响 Binding 执行 close/reset：取消旧内存 timer；在已经证明旧 state producer 无任何
   in-flight send 且被 broker/application fence 后，由 cutover writer 发送带新
   `sceneStateWriteGeneration` 的 state tombstone，并等待每个 partition broker ACK，记录
   `sceneStateTombstoneBarrier`。新 Runtime 在该 barrier durable 前不得写 state；所有新 state
   value 必须携带 manifest generation/token，restore 只接受当前 generation。这样旧 generation
   既不能在 tombstone 后 append，也不能成为 compacted latest。随后标记 next-live bootstrap。
   旧 pending Action Outbox row 保留原 identity/payload/target/dueAt，只允许 dispatcher 只读兼容
   投递，绝不能由新 Runtime 重算或改 ID；
6. 用新 identity/projection library 构建并验证新的 Scene Current Fact generation 与
   ProjectionCoverageSeed，安装新 ProjectIndex/contract；先启动唯一 Current Fact writers，从
   各自 `sourceInputEndVector` next-offset 继续，再启动新 Runtime 于
   `sceneEventEndVector` consumer position。readiness 与 state tombstone barrier 全部通过后，
   SubjectLifecycle relay 从 `subjectLifecycleInputEndVector` 恢复；新 Scheduler 依据 manifest 的
   `nextPlannedFireTs + missed-fire policy` 做一次受审计 reconciliation，再取得新 lease；最后新
   Admission 从四类 `sourceInputEndVector` 启动。所有 direct producer 使用 unchanged
   `SCENE_ID_V2`、manifest 指定的完整 context ref/hash 与新 fencing token。upstream
   `< sourceInputEndVector` 的旧 fact input 不得用新 identity 重放，
   Runtime 不得从 `< sceneEventEndVector` 重读旧 Scene record；
7. 第一条引用新 cutover manifest/context generation 的 Outbox receipt/action，或第一条新
   source/fact identity 的 Outbox row（以先发生者为准）被接受后即到达 point-of-no-return：禁止
   回滚到两字段 ProcessingContext、按 Identifier/ordinal 或旧 sourceEventId 运行的 binary，只能
   forward-fix。

若系统尚未上线、确认没有旧 Scene state/timer/Outbox/lag，上述 drain/reset 可由审计证据判定为
空集，但仍必须生成 cutover manifest，不能靠口头假设跳过身份版本记录。

新 wire/contract 启用时必须按 schema compatibility policy 完成 producer/consumer canary、
Scene Current Fact 受控初始构建/readiness 证明和旧路径 fence；这里的初始构建是 deployment
migration，不是允许 `processingMode=BACKFILL` 写 Scene Current Fact。required 字段生效后不得
回滚到按 Identifier/ordinal 猜测 factKey 的 binary。

初始构建只能使用保留了受信 `production+LIVE` provenance 的 Canonical/派生事实，或能证明
同等 provenance 且证明覆盖完整 obligation prefix/canonical max 的 authoritative latest row；不得
直接复制可能已被 BACKFILL CAS 改写的通用 latest。证据不足的 key 保持
`PROJECTION_NOT_READY`，直到受控 initial build/rebase 补齐 coverage；下一条 LIVE 更新本身不能
解除。该投影构建
不执行 Scene Binding、不创建 Scene state/timer/Outbox，因此不是被本设计拒绝的 Scene replay
evaluator。

## 16. 明确不采纳

- 独立、人工维护的 FactDefinition Registry；
- 每次 Scene 修改重新发布所有相关物模型快照；
- 为 mixed-version 设备复制两套等价 Scene；
- 直接使用 Identifier、TDengine 列名、数据库行 ID、snapshotId 或 pointOrdinal 作为跨版本
  `factKey`；
- 用版本化 alias overlay 作为正常热路径真源；它最多用于受审计 legacy migration；
- `FactDelta { value | tombstone }` 的二元模型；
- 让 Scene 配置人员覆盖来源 temporal/time/version contract；
- 以 Kafka offset、到达顺序或 wall clock 修复同 order 冲突；
- 将 OCCURRENCE 的“最后一条事件”自动解释为当前状态；
- 让 BACKFILL/REPLAY 修改 Scene Current Fact 或通过 bootstrap 间接影响生产动作；
- 为本契约新增 Scene Topic、第二套 Runtime、RocksDB/远端 Runtime snapshot-manifest、Kafka EOS
  或跨系统事务；
- 为业务查询视图独立物化通用 latest（V1 默认不启用，按 §8.5 由 TDengine last over
  history 承载；严格 CAS 投影仅在出现正确性消费者或高读 QPS 需求时按档位启用）。

## 17. 完成判据

联合修订完成后必须满足：

- 四类来源均存在可实施的 source artifact → `(factType,factKey)` 映射；
- 全仓不再把“exact fact registry”列为未定义后续工件；
- Identifier/TD 列、pointOrdinal、稳定语义 ID 和 Scene interest 的职责无混用；
- `FactDelta` 的 availability、time、version、occurrence identity 和 provenance 可由 IDL 表达；
- V3 snapshot/metric plan/transition dictionary 均可按 immutable ref 解析完整 FactContract，
  ProcessingContextContract 的 owner/hash 构造无歧义；
- 主规格与算子文档使用完全相同的 LATEST comparator；
- Scene Current Fact 与通用 latest 的 mode 权限明确分槽；
- 每次业务求值 overlay 全部必要 LATEST SingleValue，且共享 barrier snapshot 不引入 per-event
  broker offset RPC；
- OCCURRENCE 不写 Current Fact，且 V1 对所有直接 OCCURRENCE 的顺序算子禁令可由发布器执行；
- rolling identity index、production+LIVE coverage、restore 后强制重验和 observed-warm 状态机
  均有一致实现契约；
- data/Schedule/SubjectLifecycle/timer fire 的 timer anchor 及可信 scheduler epoch 门禁无空白；
- source/fact identity version 端到端传播，迁移分别证明 fact-source input、lifecycle input 与
  `scene_event` 三个 offset 域已独立排空且从未交叉比较，scheduler logical-time checkpoint 不冒充
  offset；
- Action Outbox 对同一 trigger 的 effect set、semantic state/timer/cooldown mutation plan 与单
  effect payload 都能 fail closed，DB `acceptedAt` 在 retry/restore 后不漂移，且 terminal
  completion 不阻塞后续 input；
- Scene 创建/修改不触发 `DmProductRelease/DmModuleSchemaSnapshot` 重发；
- Modbus S1/S2 mixed-version 示例在父架构、模型快照与 Scene 文档中解释一致；
- §14 的 24 项测试均有明确工件、指标或 POC 证据入口；
- 通用 latest 三档模型与 V1 默认 `off` 在父架构、Scene 文档与 §8.5 表述一致，且 §8.5
  的 P0 前提验证（消费者盘点、effectiveTime 前提、TDengine last POC）完成。
- 不恢复已经被主规格拒绝的远端 Runtime snapshot-manifest/EOS/replay/跨 Topic 事务架构。
