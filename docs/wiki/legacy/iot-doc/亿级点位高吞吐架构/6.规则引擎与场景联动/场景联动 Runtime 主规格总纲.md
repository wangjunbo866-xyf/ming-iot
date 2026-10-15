# 场景联动 Runtime：TB 式事件路由、计算状态与动作可靠性设计

> 日期：2026-07-30
> 设计状态：**核心方案已确认，文档迁移已完成；恢复 ADR 与实施证据仍待 §19**
> 恢复 ADR 状态：**未冻结，须完成 §19 证据、POC / 故障注入 / 容量实测与 RACI 签字后另行冻结**
> 适用范围：`亿级点位高吞吐架构/6.规则引擎与场景联动/` 及其在主架构、Kafka 设计中的交叉引用
> ThingsBoard 源码基线：`4.4.0-SNAPSHOT`，commit `b94811000c0d3a2be5d7cd4b44941c2a19a8720b`

> **阅读卡**
>
> - 第一次阅读：先看[《15 分钟读懂场景联动》](00-15分钟读懂场景联动.md)，无需顺序通读本文。
> - 本文定位：Scene Runtime 的**主规格总纲**。
> - 本文回答：产品边界、wire、Binding/SceneBindingState 业务语义、求值、Action Outbox 事务与容量边界。
> - 本文不回答：Raw/Canonical 的非 Scene 物理契约，以及字段/算子/SLO 的从属实施细节；partition ownership、Manager/Actor 调度、restore、rebalance 与 Zombie 协议见调度分章。
> - 快速跳转：[总体数据流](#4-总体数据流) · [`scene_event`](#5-scene_event-合约) · [`scene_state`](#8-scene_state-逻辑与物理模型) · [Timer](#10-timer-与-delayed-action) · [Outbox](#12-outbox-与动作幂等) · [Kafka 失败窗口](#13-kafka-交付事务与失败窗口) · [Runtime 调度、分片与故障转移](Scene%20Runtime%20调度、分片与故障转移.md#runtime-model)
> - 权威关系：本主规格总纲与[Runtime 调度分章](Scene%20Runtime%20调度、分片与故障转移.md#runtime-scope)共同组成规范性架构决策源；导读和三份从属实施契约只能细化，不得覆盖两份架构源。

## 1. 目标与非目标

### 1.1 目标

本设计为物联网平台定义一个高基数场景联动 Runtime，统一完成：

1. 按 ming-iot 的 tenant、project、area、product、device 组织模型进行高基数依赖匹配；
2. 按计算主体维护 ThingsBoard Calculated Field（下称 TB CF）式独立状态；
3. 支持有界 per-entity latest、rolling、脚本、duration、debounce、cooldown、edge、hysteresis、Alarm/FSM 与轻量 timer；
4. 将告警、命令、Webhook、通知等多输出统一写入 PostgreSQL Outbox；
5. 在非金融级、允许极小概率状态丢失或近似恢复的前提下，控制恢复模型和实现复杂度；
6. 与 Flink 的严格事件时间、跨实体 JOIN、CEP、session 和复杂窗口形成明确边界。
7. 以受信任的处理上下文隔离生产实时计算：只有 `processingNamespace=production && processingMode=LIVE` 的数据面 evaluation trigger 可进入生产 Scene，并修改 live business state、timer、Alarm 或 Action Outbox。

### 1.2 非目标

V1 不提供：

- Kafka EOS 或跨 Topic/数据库原子事务；
- 金融级零丢失、零重复；
- Scene 状态的 RocksDB、远端 snapshot、manifest/CAS/GC；
- Canonical 历史回放或 `snapshot + tail` 恢复；
- 独立 Go Rule Chain Runtime；
- 独立 Go CF Runtime；
- 跨场景共享 `SourceSeries/stateNode`；
- 严格事件时间对齐的跨设备 JOIN；
- 无界窗口、无界数组、任意 I/O 脚本；
- 全局 Inbox 或历史事件 ID 去重表；
- 原地增加 Kafka partition 并保持旧 hash 映射；
- 将 binding 配置版本与 Kafka data offset 对齐的 activation barrier；
- immutable binding release/index generation；
- 全量设备表或 rule × device 展开索引；
- scope 退出时自动撤销已经提交的命令、Webhook、通知或自动清除既有 Alarm。
- 禁止对缺失、未知或不匹配的处理上下文采用 `LIVE` fallback；此类输入一律 fail closed，不能进入生产 Scene。

## 2. 只有两个 Runtime

| Runtime | 职责 | 不承担 |
| --- | --- | --- |
| **Scene Runtime** | 高基数 matching、TB CF 式 per-subject 计算、有界 rolling/script、FSM/timer、多输出 PG Outbox | 严格事件时间 JOIN、CEP、session、超大窗口 |
| **Flink** | time-aligned JOIN、CEP、session、区域/群组 time-window、复杂事件时间和大状态 | 普通高基数设备场景匹配与动作编排 |

Scene Runtime 是“升级版 CF”：

- 输入像 CF：device、area、project 等 subject 的遥测与属性作为 arguments；
- 计算像 CF：按计算主体保存 latest、rolling、script 和 Alarm/FSM；
- 输出像 Rule Chain：产生告警、命令、Webhook、通知等多输出；
- 但它不是第三个 Rule Chain Runtime，也不存在单独的 Go CF 服务。

RuleGo 如被采用，只能作为表达式或 DSL 库，不能形成节点状态、消费组或独立 Runtime。

## 3. 组织模型、解析模型与核心身份

### 3.1 ming-iot 组织模型

V1 只承认下列三条组织轴：

```text
tenant -> project -> device
tenant -> project -> area tree -> device
tenant -> product -> device
```

约束：

- 跨服务租户身份统一使用 `tenantCode`；
- `projectId` 是 Binding 的必填归属和隔离边界；
- area 是 project 内的可选树形选择器，设备最多位于一个有效 area path；
- product 是 tenant 级产品目录，与 project 正交；
- device 用稳定内部 `deviceId` 参与 subject identity，`(productId, deviceName)` 是设备业务键；
- V1 禁止跨 project 的 source、evaluation subject、scope target 和 action target；
- V1 不提供 tenant-wide anchor。租户级统一策略由同一模板发布多个 project Binding 实现。

`projectId` 不进入 state key，因为稳定 `bindingId` 已唯一归属于一个 project；所有读取和写入仍必须校验 Binding、source 与 target 属于同一 tenant/project，不能据此放松隔离。

### 3.2 Scope、计算主体与参数来源

三个概念必须分开：

```text
ScopeSelector
  决定哪些设备适用该 Binding

EvaluationSubjectResolver
  决定状态和动作归谁所有

ArgumentSource
  决定每个参数从哪个 subject/fact 取得
```

V1 支持：

- `ScopeSelector`：project all、area exact/subtree、product、fixed device，以及它们在同一 project 内的有界组合；
- `EvaluationSubjectResolver`：`MATCHED_DEVICE`、`FIXED_DEVICE`、`FIXED_AREA`、`PROJECT`；
- `ArgumentSource`：`SELF`、`EVALUATION_SUBJECT`、`FIXED_SUBJECT`；
- argument trigger mode：`TRIGGER` 或 `CONTEXT_ONLY`。

普通“完整条件成立时执行动作”的多条件判断，其谓词引用的全部动态 argument 都必须是
`TRIGGER`；不能按第一个字段或条件顺序只选择一个主触发参数。只有产品/DSL 明确建模为
`WHEN ... IF ...` 时，IF 附加门禁才可为 `CONTEXT_ONLY`。rolling 样本依赖及 Edge/Change、
Duration、Debounce、Hysteresis 状态作用域内的全部动态依赖也必须是 `TRIGGER`；Cooldown 不新增
数据 trigger。发布物必须逐 argument 显式记录模式，Runtime 不按字段顺序猜测。

多个 `TRIGGER` 都能独立启动求值，但普通动作共享完整谓词的 `Edge(RISING)`：连续 true 区间只在
第一次 `false -> true` 形成 action set，而不是每个参数各执行一次。发布编译、显式上下文与动作门
的唯一完整定义见
[控制面与匹配契约 §4.2](控制面与匹配契约.md#42-trigger-与-context_only)及
[场景有状态计算规范 §§4.1.1～4.1.2](场景有状态计算规范.md#stateful-trigger-dependency-closure)。

`sourceSubjectKey` 表示本次事实来自谁，例如 `device:A`；`evaluationSubjectKey` 表示这份状态属于谁，例如 `area:R1`。subject key 是 tenant-local 的 typed stable key，外层 `tenantCode` 才是唯一租户分量，不在 subject key 中重复编码 tenant。

跨实体例子：

```text
Binding room-climate
owner project: project:P1
evaluation subject: area:R1

tempA     <- device:A.temperature       (TRIGGER)
humidityB <- device:B.humidity          (TRIGGER)
setpoint  <- area:R1.targetTemperature  (CONTEXT_ONLY)
```

三个输入最终汇入同一状态：

```text
(tenant-1, binding:room-climate, area:R1)
```

固定 source 向巨大 target scope 主动 fanout 默认拒绝。应改为 `CONTEXT_ONLY`、设置有界 fanout，或把区域聚合/time-window 交给 Flink。area/project target 也不能表达“最后一个任意设备值”；此类聚合必须先由 Flink 产生规范化事实。

### 3.3 稳定 Binding 与状态身份

每个 Binding、每个计算主体拥有独立状态：

```text
(tenantCode, bindingId, evaluationSubjectKey)
```

约束：

- `bindingId` 在 Binding 编辑、启停和重新发布期间保持稳定；
- `bindingId` 一经创建不得复用，Binding 的 owner `projectId` 不允许原地变更；跨 project 迁移必须表现为旧 Binding disable/delete + 新 `bindingId` create；
- `configRevision` 是 PG current Binding、生命周期事件和 state metadata 中的单调版本，只用于缓存收敛、审计和兼容性判断；
- `configRevision` 不进入 state key、Kafka key、partition key、timer identity、transitionId 或 effectId；
- 不同 Binding 不共享 CF 状态；同一模板的多个 Binding 也不能共享状态；
- `operatorId` 位于完整 state value 内，不进入 Kafka state key；
- 配置更新按当前最新配置处理：兼容变更刷新 context；状态语义变化由控制面 close/reset+tombstone 并标记 next-live bootstrap，禁用或删除异步 tombstone；
- 不保存或路由 immutable `bindingReleaseId`，输入也不携带 expected config revision。

这与 TB 的 `(tenantId, cfId, targetEntityId)` 一致：稳定 CF/Binding 身份拥有状态，配置版本是当前 context，而不是每次发布创建一套新状态。

### 3.4 输入身份

Canonical 为源事实提供可透传的 `sourceEventId`。它用于：

- lineage 与审计；
- 场景因果链和 hop-limit；
- PG Outbox `effectId`；
- 调试与错误关联。

处理上下文是所有 Scene 输入共同的 wire 契约：

```text
ProcessingContext {
  processingNamespace
  processingMode = LIVE | BACKFILL | REPLAY
}
```

`processingMode` 表示处理用途，不能与处理时间、事件时间或设备时间混淆：处理时间是 Runtime 接收/执行时刻；事件时间是事实发生时间（`eventTs`）；设备时间是设备产生/上报的时间（如存在）。重投递必须原样保留 `ProcessingContext`，不能因重试、rebalance 或重新封装而改写。合法组合以下表为准；缺失、未知枚举值、表外 namespace/mode 组合或未受信任的上下文一律 fail closed；禁止 default `LIVE`，绝不默认 `LIVE`。

可信 `ProcessingContext` 只能继承自以下受信入口：ProtocolApplication 产生的 Canonical、领域 reducer 的 AcceptedStateTransition、受信 Flink 规范化输出，或受信平台 schedule/subject-lifecycle producer。禁止从 topic 名、event/device time、到达延迟、设备 payload 或设备自报 metadata 推断、补齐或提升处理上下文。

生产 admission 按以下组合决定，且来源必须同时可信：

| `processingNamespace` / `processingMode` | 决定 |
| --- | --- |
| `production` + `LIVE` | 进入生产 Scene 数据面 handler |
| `production` + `BACKFILL` | 合法在线补传/历史 output；deterministic bypass，不是 quarantine，不写 `scene_event`，不产生 live effects |
| `backfill/<jobId>` + `BACKFILL` | 合法显式 backfill job；同样 deterministic bypass，不是 quarantine，不写 `scene_event`，不产生 live effects |
| `replay/<id>` + `REPLAY` | 不进入生产 Scene |
| `shadow/<id>` + 任意 mode | 不进入生产 Scene |
| 缺失、未知、组合不一致或来源不可信 | quarantine/fail closed |

同一条 Canonical 事实被 Admission 重试或被转成 LinkedDelta 时，payload 中的
`sourceEventId` 与 `ProcessingContext` 必须保持不变；Kafka queue record ID 仍可随机。Scene 计算结果若被发布为
新的规范化事实，则获得新的 `sourceEventId`，并通过 `parentEventId` 保留因果关系。

只有受信任的 `processingNamespace=production && processingMode=LIVE` 可进入生产 Scene。`BACKFILL` 在候选匹配前旁路，不产生 `scene_event`，也不产生生产 Scene 的 state、timer、Alarm 或 Outbox 效果；`REPLAY` 和任何 shadow namespace 都不得进入生产 Scene。`SubjectContext` 只解析 tenant/project/area/product 等 scope，绝不改变或推断处理 mode。same-subject complete target bundle、cross-subject `LINKED_DELTA` 与 Scene-derived 路径都原样传播 `ProcessingContext`；若 Scene 输出成为新的 Canonical 事实，新的 envelope 仍继承该上下文。

它不用于：

- 全局 Inbox；
- 历史 `seenEventIds`；
- 通用参数 revision；
- 强制确定性 Linked Kafka record ID。
- 配置或组织关系的 activation barrier；
- 指定某条 SOURCE_DELTA 必须按哪个 `configRevision` 匹配。

SingleValue/latest 必须使用
[《场景有状态计算规范》§5.1](场景有状态计算规范.md#state-component-latest)冻结的精确
`compareLatest`：先验证事实身份与顺序域，再比较 `eventTs` 和来源 FactContract 的可选版本；
duplicate 不修改 value、不刷新 freshness，任何 identity/order/payload conflict 都 fail closed。
value merge 与 `triggerRight` 分开，后者由可恢复 Binding input receipt 独立裁决。

Rolling 必须使用
[《场景有状态计算规范》§5.2](场景有状态计算规范.md#state-component-rolling)冻结的稳定 sample
identity：LATEST 使用 `factDeltaId`，OCCURRENCE 使用 `occurrenceId`；`eventTs` 只参与排序和
cutoff，同一时间戳的不同身份必须并存。不得以时间戳作为样本唯一 key，也不额外引入 Kafka
offset revision。

## 4. 总体数据流

控制面和数据面分开：PG current Binding 是配置真源，Canonical 是输入事实入口，Kafka `scene_state` 是可恢复计算状态；三者不能混成同一“真源”。

```text
PG current Binding + 同事务 lifecycle Outbox
                 |
                 v
Kafka scene_binding_lifecycle（控制面 Topic）
                 |
                 v
每个 Scene Runtime 节点
  - 全量缓存所有 enabled Binding 的配置与 matching metadata
  - 按 (tenantCode, projectId) 组织 ProjectIndex
  - 不缓存全量设备表，不保存 rule x device 展开
```

数据面：

```text
Canonical record（一次设备上报可含多个点位，带受信任 ProcessingContext）
                 |
                 v
Scene Event Admission
  - 在 interest filtering 前校验 ProcessingContext
  - BACKFILL 在 candidate matching 前旁路：不写 scene_event、无 live effects
  - 仅 production + LIVE 进入 production Scene
  - existence/relevance check
  - 过滤出本 record 中 Scene-relevant factDeltas[]
                 |
                 v
scene_event: SOURCE_DELTA（每个 admitted canonical record 一条）
key = (tenantCode, sourceSubjectKey)
                 |
                 v
Scene Runtime
  - Manager 在一次完整 ProjectIndex read view 中匹配并形成完整 target bundle
  - Actor 按 evaluation subject 串行应用 bundle，求值并维护独立 BindingState
                 |
        +--------+--------+
        |                 |
        v                 v
PG action Outbox       async scene_state
                      compacted full state
```

Manager、PartitionRuntimeContext、Actor lane 与 BindingState 的职责、target 递交与
completion 顺序由[Runtime 调度分章 §§3–5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-components)
唯一规定；本节只保留产品级数据流与 Manager/Actor 职责摘要。

Schedule、subject lifecycle 也进入同一 `scene_event` 物理 Topic，并由各自产生者盖章为受信任的 `production + LIVE + SCENE_ID_V2`。Flink 派生事实必须先规范化为带 `origin=FLINK` 的 SOURCE_DELTA，再走相同 matching；任何入口都不能绕过 Scene Entity Actor 直接产生动作。

`scene_binding_lifecycle` 是配置传播 Topic，不属于 Canonical → `scene_event` → `scene_state` 三层数据链路，也不承载运行时 state。它是经 ACL/producer identity 信任的**无 state-value control**流：可安装配置、close/reset 内存、发送 tombstone，并标记 next-live bootstrap；close/reset 必须取消该 state 已有 timer slot/handle，但不得读取 current facts、构造完整 non-tombstone business state、执行 predicate/FSM，或创建、arm、推进、fire timer/Alarm/Action Outbox。explicit reinit 与之相同，只是无 state-value control operation。控制面不提供立即业务求值；V1 若未来需要，必须另立契约定义新的 `SceneEvent` kind，当前不得虚构或绕过 admission gate。

Admission 先验证受信任 `ProcessingContext`，再维护由 current Binding 派生的粗粒度 tenant/fact interest filter。无效上下文在进入 filter、candidate matching 或 handler 前 quarantine/fail closed；`BACKFILL` 在 candidate matching 前旁路，绝不写 `scene_event`。filter 只允许 false positive（多送一条 SOURCE），不承担 evaluation target matching；filter 不可用时仅对已验证为 production+LIVE 的输入在租户速率上限内 fail open，不能因本地索引故障静默丢掉全部 Scene 输入。配置传播窗口仍采用 TB 式最终收敛，不承诺新 Binding 发布瞬间之前的事实自动补算。

## 5. `scene_event` 合约

### 5.1 一个物理 Topic、一个入口

V1 使用一个物理 `scene_event` Topic。统一 Protobuf envelope 以 `oneof` 承载：

```text
SceneEvent {
  eventId
  required processingContext
  required identityAlgorithmVersion = SCENE_ID_V2
  oneof payload {
    SourceDelta          sourceDelta
    LinkedDelta          linkedDelta
    ScheduleEvaluate     scheduleEvaluate
    SubjectLifecycle     subjectLifecycle
    LifecycleTargetDelta lifecycleTargetDelta
  }
}
```

`processingContext` 是独立的 required wire 字段，不在 `lineage` 中表达。`identityAlgorithmVersion` 是独立的 required 身份编码版本；本次切换后的新记录只能写 `SCENE_ID_V2`，所有 §5.5/§10～§12 的 Scene 派生 hash 都把该稳定版本 tag 作为首个输入。producer 必须在发布前拒绝/quarantine 缺失 context/version 的 SourceDelta/Event；consumer 仍必须二次拒绝。每一条生产 `scene_event` 必须是 `processingNamespace=production && processingMode=LIVE`。同一 Scene Runtime consumer entrypoint 收到消息后，所有 handler 都必须在 lookup 或 evaluation 前验证此上下文与版本；无效上下文/版本进入 quarantine/fail closed，且不得构造 arguments、修改 live business state/timer/Alarm 或写 Outbox。不是用两个函数盲目消费同一 payload，也不为 SOURCE/LINKED 拆出不同 Topic。无法识别的 kind、非法 key/payload 进入 quarantine。

外部生产者必须使用独立最小权限角色：Scene Admission 只发布 `SourceDelta`，受信 Scheduler 只发布 `ScheduleEvaluate`，Things SubjectLifecycle relay 只发布 `SubjectLifecycle`；Scene Runtime 只发布其内部派生的 `LinkedDelta`/`LifecycleTargetDelta`。Kafka ACL 只能约束 Topic，不能识别 Protobuf `oneof`；因此 role→kind + production+LIVE 限制必须由各受认证 producer 的封闭 publish API/application contract 在发送前强制，不能暴露任意 envelope publish 接口。Runtime consumer 二次校验的是全局 allowed kind、oneof/payload/key/route 一致性、ProcessingContext 与 identity version；Kafka record 不携带 broker 认证后的 producer principal，普通 header 也不能充当该证据，所以 Runtime 不得宣称能按 record 证明 role→kind。`scene_binding_lifecycle` relay 只能写独立控制 Topic，不共用任一数据面 producer role。若威胁模型要求在 producer 已被攻陷时仍由 broker/consumer 强制 per-kind 身份，必须另立 ADR 选择 broker 可寻址的独立 Topic 或可验证签名；V1 的单 `scene_event` Topic 不作此承诺。

### 5.2 批量 SOURCE_DELTA

一个 Canonical record 最多形成一条 SOURCE_DELTA，不按点位拆成 N 条物理消息。`SourceDelta` 在公共 envelope 中继承受信任 Canonical 的 `processingContext`，由 Admission 盖章 `identityAlgorithmVersion=SCENE_ID_V2`，二者都不得由后续 handler 重写。只有 production+LIVE Canonical 可以在 Admission 已验证上下文并盖章版本后继续 interest filtering；若一次设备上报 200 个点位，其中 12 个至少被一个 enabled Binding 依赖，则产生一条包含 12 个 `factDeltas` 的 SOURCE_DELTA；没有相关点位时不接纳。

```text
SourceDelta {
  tenantCode
  sourceEventId
  sourceSubjectKey
  factDeltas[] {
    factType
    factKey
    value / tombstone
    eventTs
    optional factVersion
  }
  receivedAt
  lineage
}
```

Kafka key：

```text
(tenantCode, sourceSubjectKey)
```

SOURCE_DELTA 不冻结 Product/Project/Area snapshot，也不携带 expected `configRevision`。source lane 在 matching、lookup 或 evaluation 前再次验证 envelope 为 production+LIVE+SCENE_ID_V2；验证失败时不构造 arguments、不修改 state/timer/Alarm、不写 Outbox。随后才在处理时解析 current SubjectContext：

```text
SubjectContext {
  sourceSubjectKey
  subjectType = DEVICE | AREA | PROJECT
  entityId
  projectId
  optional deviceId
  optional areaPath[]
  optional productId
  aggregateVersion
}
```

它来自 partition-local 的有界 active-subject cache：device context 由 SubjectLifecycle 更新，area/project 等 typed subject 由 Things current store 的对应实体记录解析；首次命中或 rebalance 后缺失时也从 current store 懒加载。`aggregateVersion` 是该 typed subject 当前组织/关系记录的单调版本，不是 immutable affiliation snapshot。节点不因此缓存全量设备表。context 不可用时整条 SOURCE_DELTA retry/fail closed；不能根据旧 payload 猜测 scope，更不能将 `SubjectContext` 用作 processing mode 的来源。这样 backlog telemetry 与 TB 一样按处理时已知的 current affiliation 匹配，而不是在旧 Product/Area 下复活已删除 state。

Admission relevance filter 只做 tenant/fact 级无漏接过滤，不以瞬时 Product/Area membership 排除某个 fact；具体 scope matching 只在 Manager 当前 ProjectIndex 中完成。SOURCE_DELTA handler 是唯一执行 reverse-dependency matching 的数据 handler，并且只处理已验证的 production+LIVE+SCENE_ID_V2 输入。

source bundle 有 fact 数和 record bytes 硬上限。超限时整条 Canonical record fail closed/quarantine；V1 不做拆包、reassembly 和“分片到齐后再求值”。

consumer record 如何在 partition/token 上下文中进入 Manager，由
[Runtime 调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)
唯一规定；本节只保留 SOURCE_DELTA wire、SubjectContext 与 matching 业务门禁。

### 5.3 稀疏 LINKED_DELTA target bundle

LINKED_DELTA 只携带本次发生变化的参数映射：

```text
LinkedDelta {
  tenantCode
  projectId
  sourceEventId
  sourceSubjectKey
  sourceAggregateVersion
  evaluationSubjectKey
  argumentDeltas[] {
    bindingId
    argId
    value / tombstone
    eventTs
    optional factVersion
  }
  lineage
}
```

Kafka key：

```text
(tenantCode, evaluationSubjectKey)
```

Manager 必须先匹配 SOURCE_DELTA 的全部 `factDeltas[]`，再按 `evaluationSubjectKey` 分组，并形成两类彼此独立的 payload：

- **same-subject complete target bundle**：`evaluationSubjectKey == sourceSubjectKey`；不编码、也不产生 `LINKED_DELTA` physical record；
- **cross-subject target-keyed `LINKED_DELTA`**：`evaluationSubjectKey != sourceSubjectKey`；同一 source record、同一 evaluation target 的全部 binding/arg 命中编码为一条 target-keyed `LINKED_DELTA` physical record。

两类 payload 都必须原样传播源 `SceneEvent.processingContext/identityAlgorithmVersion` 以及 provenance（`sourceEventId`、`sourceSubjectKey`、`sourceAggregateVersion`、`lineage`）；不得由 Manager、Actor 或 scope resolver 改写、补全或重新盖章。Actor 应用某个 evaluation target 的完整 bundle 后，每个 Binding 最多求值一次。

这里的“完整 bundle”是指该 source record 对该 evaluation target 的全部变化映射，不是把场景所有参数都重复装入消息。cross-subject wire 中的 `LINKED_DELTA.argumentDeltas[]` 仍然是稀疏变化；未变化参数来自目标 Actor 已恢复/维护的 SceneBindingState；若 state 不存在则按 §9 从 current latest store 初始化。

这是正确性约束，不只是 producer batching 优化：

- lookup 必须返回该 source bundle 的完整匹配结果；
- target bundle 有 binding、argument 和 record bytes 硬上限；
- 超限时整个 target bundle fail closed/quarantine，不能拆成可独立求值的多条消息；
- same-subject complete target bundle 也必须先汇总完整 bundle，再按 Binding 各求值一次。

### 5.4 终端 handler 与统一求值入口

LinkedDelta handler：

- 在任何 ProjectIndex lookup、argument 构造或 evaluation 前验证 envelope 为受信任 production+LIVE+SCENE_ID_V2；否则 quarantine/fail closed，且不修改 state/timer/Alarm 或 Outbox；
- 不查询 reverse-dependency index；
- 不生成新的 LinkedDelta；
- 不重新执行 reverse matching，但必须从 ProjectIndex 校验 payload 中每个 Binding 仍 enabled，并校验 source 与 evaluation target 的 current tenant/project/scope 仍符合 Binding/target resolver；
- 对共享 area/project/fixed target，先以 `sourceAggregateVersion` 检查 member version guard；已处理更高版本的 REMOVE 后到达的旧 Linked 必须丢弃；
- state/member 不存在，或本地 guard 无法证明 applicability 时，必须先从 Things current store 读取 source/target current context并验证版本与 scope；验证失败 retry/fail closed，不能直接 bootstrap 复活旧 scope；
- 通过上述 applicability guard 后，只更新 payload 明确指定的 Binding/argument；
- 应用完整 `LINKED_DELTA` bundle 后，进入与 same-subject complete target bundle 相同的 `ArgumentDeltaBundle` 求值入口。

明确判定为 disabled/out-of-scope/stale 的 Binding entry 是幂等 no-op；如果任一所需 index/context lookup 出错，则整个 target bundle retry/quarantine，不能先应用其余 entry。

这不是 config/data offset barrier：Linked 仍按 target lane 异步到达，只是携带 source handler 已读取的 current aggregate version。若对应 `LifecycleTargetDelta` 已成功处理更高版本，旧 Linked 会稳定 no-op；若 lifecycle target 最终丢失，仍按 §13.2 的 best-effort 窗口由 quarantine replay/reinit 修复。

如果 Scene 输出要继续触发其他场景，必须发布为新的规范化事实，再获得新的 `sourceEventId`，并用 `parentEventId`、`causedBySceneId` 和 bounded lineage 保留因果链。因此不存在 `LinkedDelta -> LinkedDelta` 的递归几何放大。

SOURCE-side same-subject direct delivery 与 cross-subject Kafka delivery 的递交边界由
[Runtime 调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)
唯一规定。same-subject 与 cross-subject 两类路径仍使用同一
state schema、evaluator、Outbox 和 latest-value 语义。

### 5.5 Subject lifecycle 与终端 lifecycle bundle

Things 侧设备组织关系变更在同一业务事务中递增 device `aggregateVersion` 并写 Outbox，再经 Kafka 产生 SubjectLifecycle；产生者必须盖章受信任 `production + LIVE + SCENE_ID_V2`，不再使用 NATS/FastEvent：

```text
SubjectLifecycle {
  tenantCode
  lifecycleEventId
  sourceSubjectKey
  deviceId
  kind = PRODUCT_CHANGED
       | PROJECT_CHANGED
       | AREA_MEMBERSHIP_ADDED
       | AREA_MEMBERSHIP_REMOVED
       | ENTITY_DELETED
  oldContext { projectId, areaPath[], productId }
  newContext { projectId, areaPath[], productId }
  aggregateVersion
}
```

Kafka key 是 `(tenantCode, sourceSubjectKey)`，这里的组织生命周期和 `ENTITY_DELETED` 均以 device 为 subject。area subtree 换父/删除必须先由 Things 展开为受影响 device lifecycle job；area/project 自身作为 fact source 的 current context 则按 typed `sourceSubjectKey` 从 authoritative current row/version 解析，实体删除由控制面先完成 Binding 与设备引用清理。它没有 config/offset barrier；`lifecycleEventId` 和 `aggregateVersion` 只用于重复、乱序防护。

一个 SubjectLifecycle record 只占一个严格递增 aggregateVersion；若同一业务事务需要表达多个组织变化，Outbox 必须按确定顺序分配连续 version，不能让两个不同 event 共用同一 version。`PROJECT_CHANGED` 的 old/new context 已包含该次迁移对应的 area path，不再额外发送同 version 的 area event。

source handler 不得先推进 context version 再发送 operation，否则中途失败后的重试会被误判为重复。固定顺序是：

```text
1. 读取已接受的 current SubjectContext
2. 构造 tentative new context
3. 确保所需 ProjectIndex ready，计算全部受影响 operation 并形成完整 target bundles
4. 完成各 terminal bundle 的 delivery outcome
5. 才发布 tentative context 与 aggregateVersion
```

崩溃重试可以重复发送同一 lifecycleEventId；target state 对每个 source member 保存最后 `aggregateVersion`，使 ADD/REMOVE 幂等且拒绝旧操作。发现 version gap 时，以本地已接受 context 为 old、Things current context 为 new 做一次直接收敛。若 lane 首次加载没有 old context，则先加载 current context，并在恢复/首次触达的 BindingState 注册 timer 或求值前按 current scope 做懒 reconciliation；不构造历史 affiliation snapshot。

若某个 terminal bundle 在有界重试后仍失败并被 quarantine，source lane 仍刷新为 Things current context并标记 `reconcileRequired`，避免后续 telemetry 继续按旧 scope 匹配；quarantine 必须保存原 lifecycle、已计算 operation 和失败 target，由 Scene 运维 replay worker 有界重试，超过 SLO 后告警并要求显式 reinit。这是明确接受的 best-effort 窗口，不伪装成原子迁移。

不同 evaluation subject 时，完整 terminal payload 定义为 LifecycleTargetDelta；具体递交/route
由[Runtime 调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)
唯一规定：

```text
LifecycleTargetDelta {
  tenantCode
  projectId
  lifecycleEventId
  aggregateVersion
  evaluationSubjectKey
  operations[] {
    bindingId
    operation = INIT_STATE | REINIT_STATE | DELETE_STATE | ADD_MEMBER | REMOVE_MEMBER
    sourceSubjectKey
    lifecycleEvaluationId
  }
}
```

LifecycleTargetDelta 原样传播其 SubjectLifecycle 的 `processingContext/identityAlgorithmVersion`。SubjectLifecycle 与 LifecycleTargetDelta handler 均须先验证 production+LIVE+SCENE_ID_V2，才可读取 current context、构造 operation 或触达 state/timer/Alarm/Outbox；无效输入 fail closed。ScheduleEvaluate 产生者同样盖章 production+LIVE+SCENE_ID_V2，Schedule handler 也遵循此先验验证规则。

它与 LinkedDelta 一样是 one-project-one-target-one-bundle、target-keyed、终端投递；target handler 不再 matching，但必须应用 §8.1 的 state/member aggregate-version gate。所有 terminal bundle 的 target delivery 均遵循上述调度分章边界。
Project change 的旧 project cleanup 与新 project init 因此是两个独立完整 bundle；它们可能先后成功，失败 target 由 lifecycle quarantine replay/reinit 收敛。具体 Product/Project/Area 语义见 §11.3。

每个 operation 的因果身份固定为：

```text
lifecycleRequestId = lifecycleEventId

lifecycleEvaluationId =
  H(SCENE_ID_V2, processingNamespace, tenantCode, bindingId, evaluationSubjectKey,
    lifecycleEventId, operation, sourceSubjectKey)
```

同 evaluation subject 与不同 evaluation subject 两条语义路径必须使用同一派生式，
并继承共同的 `ProcessingContext/identityAlgorithmVersion`；分页、retry、rebalance 后不得重新生成随机 ID。

## 6. Binding 生命周期、ProjectIndex 与 matching

### 6.1 PG/Kafka 配置真源

PG current Binding 是唯一配置真源。一次创建、修改、启停或删除事务必须同时：

1. 写 current Binding，递增 `bindingConfigRevision`（下文 state metadata 简写为 `configRevision`）；
2. 递增 `(tenantCode, projectId)` 的 `projectConfigRevision`；
3. 把本次 `projectConfigRevision` 记入 Binding current row 的 `lastChangedProjectRevision`；
4. 写配置 lifecycle Outbox，包含稳定 eventId、tenantCode、projectId、bindingId、bindingConfigRevision、projectConfigRevision 和操作类型。

Outbox relay 至少一次投递到 `scene_binding_lifecycle`，Kafka key 为 `(tenantCode, projectId)`，使同 project 事件共置。Kafka key 本身不能保证并发 relay 的发送先后；relay 正常路径按 project revision 排序且同 project 只保留一个 in-flight send，但 consumer 正确性仍以 §6.3 revision gate 为准：乱序跳跃触发 rebuild，晚到旧事件忽略。它与 action Outbox 是两张职责不同的表：前者传播配置变化，后者投递 Alarm/command/webhook/notification。

每个 Scene Runtime 节点使用独立配置 consumer group（例如 `scene-binding-cache-{nodeId}`）接收全部 lifecycle partition，而不是让多个节点共享一个只投递一次的配置 group。任何 op 都必须先读/验证 PG current 状态，不能先删 posting 再验证：create/update/enable 要求 row 的 `lastChangedProjectRevision` 与 event 一致；disable 仅在同 revision row 已 disabled 时删除；若 row 已被更高 revision 越过或 disable event 读到 later delete，则 rebuild；delete 仅在 row absent 且该永不复用 bindingId 的 event 通过 project revision gate 后删除。所有插入/删除最终都按 §6.3 write lock 原子应用。Binding owner project 不允许原地修改。事件 payload 不是不可变 Binding release，也不承载完整执行快照。

### 6.2 每节点全量规则缓存，运行时 state 分区

每个 Runtime 节点缓存全部 enabled Binding 的配置和 matching metadata，内部按 `(tenantCode, projectId)` 组织为 ProjectIndex。节点不缓存：

- 全量设备业务表；
- 全量 telemetry/state；
- rule × device 展开结果；
- 其他 Kafka partition 的 Actor state。

因此是“规则索引全节点复制，Actor state/timer 按 `scene_event` partition 分片”。ProjectIndex 是本地编译、锁和故障隔离单元，不是 Kafka partition owner。
该架构事实不定义 per-partition Actor registry、ownership 或 close 算法；这些执行约束由
[Runtime 调度分章 §§3–6](Scene%20Runtime%20调度、分片与故障转移.md#runtime-components)
唯一规定。

选择 ProjectIndex 而不是 TenantIndex/AreaIndex：project 是所有 Binding 的必填 owner 和 V1 禁跨边界；tenant 粒度会把一个项目更新扩大为全租户锁/重建，area 又只是可选、可重叠的 selector，无法容纳 project/product/device scope。实现仍可在最外层用 tenant map 定位 project，但原子换代单位固定是 project。

宽 scope 使用 project、area path、product、device 的符号 posting。逻辑匹配结果仍可表示为：

```text
(tenantCode, sourceSubjectKey, factType, factKey, currentSubjectContext)
  -> [(bindingId, evaluationSubjectKey, argId), ...]
```

实现可以分别维护 exact-source、project、area exact/subtree、product、device postings 和 `bindingId -> BindingCtx`，但不能在发布时把 project/area/product scope 展开成每设备 posting。

### 6.3 单 Project 原子更新

V1 每个 ProjectIndex 使用独立 RWMutex：

- 同一 `(tenantCode, projectId)` 的 lifecycle 先进入本地串行 apply queue；ProjectIndex 同时保存 `installedProjectRevision` 和每个 Binding 的 `installedBindingRevision`；
- `event.projectConfigRevision <= installedProjectRevision` 时视为重复/旧事件；大于 `installedProjectRevision + 1` 时视为 gap 并触发 rebuild；
- 普通 binding lifecycle apply 包含 create/update/enable/disable：锁外读取并验证 PG current row，enabled 时编译完整新 BindingCtx，disabled 时形成完整 remove plan；若 row 的 `lastChangedProjectRevision` 已高于正在处理的 event，说明 PG current row 已越过该事件，放弃增量并 rebuild，不能拿未来 row 冒充当前 revision；delete 只按 §6.1 的 absent + 永不复用规则形成 remove plan；
- 取得 project write lock 后再次校验 `installedProjectRevision == event.projectConfigRevision - 1` 且本 Binding 未安装更高 `bindingConfigRevision`，再删除该 binding 的全部旧 postings、插入全部新 postings、替换 BindingCtx并推进两个 installed revision；任一校验失败即丢弃锁外产物并按当前 watermark 重试/重建；
- SOURCE lookup：从第一个 factDelta matching 到全部 target 分组完成，始终持有同一 project read lock；
- full rebuild：记录 rebuild 起点的 local installed revision，在 PG `REPEATABLE READ` 中取得 current rows 与 snapshot project revision R，锁外构建/校验 staging；短暂取得 write lock，确认 local installed revision 仍等于起点后 pointer swap 并设为 R；随后按 revision 应用 rebuild 期间缓存的 lifecycle；
- 禁止持有 write lock执行 PG 查询、脚本编译或全量 rebuild。

revision 校验不连续时放弃本次增量产物并触发 §6.4 project rebuild，不能“尽量插入能找到的 posting”。

读取者只能看到完整旧版本或完整新版本，不能看到“旧 posting 已删、新 posting 只插了一半”的 bundle。不同节点或 partition 在配置传播期间允许短暂使用不同的完整版本。

ProjectIndex 换代保留以下业务结果：

- 兼容变更刷新 BindingCtx；
- 参数/计算语义变化按编译结果 close/reset 并 tombstone，使用配置 lifecycle eventId 作为稳定 `controlStateManagementRequestId`（仅用于审计/幂等 state management）；下一条合格 live trigger 再 bootstrap；
- disable/delete 或 scope narrowing 使已有 state 不再适用时 close+tombstone；
- scope widening 下尚不存在的 state 默认在首个数据、Schedule 或 SubjectLifecycle 时懒 INIT，不为此加载全量设备表；
- 如产品要求发布即覆盖全部设备，必须作为有配额的显式 rollout job，而不是普通 index update 的隐藏 fanout。

配置广播仍由所有节点接收，但上述 state/timer cleanup 业务结果只由对应
data partition 的当前 owner 执行。per-partition Actor registry、token/generation 门禁与
generation-conditional cleanup 由
[Runtime 调度分章 §6](Scene%20Runtime%20调度、分片与故障转移.md#runtime-state-machine)
唯一规定。

### 6.4 冷启动、revision gap 与恢复

节点启动：

```text
1. 记录 scene_binding_lifecycle 各 partition 的当前 end offset E0
2. 从 E0 启动配置 consumer 并暂存后续 lifecycle
3. PG REPEATABLE READ 扫描全部 current enabled Binding/project revision
4. 编译并安装全量 ProjectIndex
5. 按 project revision 应用暂存 lifecycle
6. 所有 ProjectIndex ready 后才加入 scene_event data consumer group
```

这里的 `E0` 只用于避免“PG 扫描期间漏掉配置 lifecycle”，不定义任何 `scene_event` data record 的生效版本，也不是 activation barrier/effectiveOffset。

配置 ready/revision-gap 是本节的业务门禁；data partition assignment 的激活、restore gate
与开放顺序由
[Runtime 调度分章 §7](Scene%20Runtime%20调度、分片与故障转移.md#runtime-restore)
唯一规定。

PG snapshot 安装时把 snapshot project revision 作为 `installedProjectRevision`，只按严格连续且大于该 watermark 的顺序应用暂存事件。重复或旧 revision 可忽略；发现 project revision gap、current row 越过 event、未知 binding、编译产物损坏或周期性 PG revision 对账不一致时，只重建该 ProjectIndex。重建成功前保留旧的完整 index；若没有任何可验证 index，则该 project 的数据 retry/fail closed，不能临时返回部分结果或在线拼 PG/Redis 查询结果。

### 6.5 无 barrier 的完整 lookup

本设计没有 expected config revision、effective Kafka offset、per-partition barrier 或全局 config snapshot：

- 一条 SOURCE_DELTA 使用处理节点当时已安装的一个完整 ProjectIndex read view；
- in-flight record 可能使用旧完整配置，稍后的 record 使用新完整配置；
- 不回头重算已经按旧配置完成的事件；
- `projectConfigRevision` 只用于缓存 gap 检测和审计，不与 data offset 对齐。

lookup timeout、索引损坏或内部不变量失败时，整个 SOURCE_DELTA retry；超过有界重试后整条 quarantine 并告警。绝不能返回部分 binding/arg，因部分结果会破坏 one-target-one-bundle，造成遗漏或重复求值。

### 6.6 SOURCE matching 与 fanout

```text
1. 解析并校验 current SubjectContext 与 tenant/project
2. 取得对应 ProjectIndex read lock
3. 对 SOURCE_DELTA.factDeltas[] 逐项匹配
4. 以 (evaluationSubjectKey, bindingId, argId) 去重/合并
5. 按 evaluationSubjectKey 形成完整 target bundle
6. 释放 read lock
7. 按 same-subject / cross-subject 做 wire/payload 分类；递交见下方说明
```

same-subject complete target bundle 与 cross-subject target-keyed `LINKED_DELTA` 的实际递交由
[Runtime 调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)
唯一规定。

`c` 表示每条 admitted SOURCE_DELTA 最终求值的 distinct BindingState 数，已包含 same-subject + cross-subject 两条路径；`lambda` 表示直接影响的 distinct cross-subject evaluation target key 数，也等于 one-target-one-bundle 下 target-keyed `LINKED_DELTA` physical record 数，不表示物理远端机器。高 fanout 必须在发布期估算并受运行期硬上限约束。`LINKED_DELTA` 不递归，所以 `lambda` 不是级数模型。

## 7. 计算语义

§7～§10 只保留 Runtime 架构摘要。字段级规则、状态机和失败语义不得从本摘要反推；规范归属如下：

| 主题 | 唯一规范入口 |
| --- | --- |
| Binding 的 `TRIGGER`/`CONTEXT_ONLY`、完整条件依赖、动作门与发布编译 | [《控制面与匹配契约》§4.2](控制面与匹配契约.md#42-trigger-与-context_only)及[《场景有状态计算规范》§§4.1.1～4.1.2](场景有状态计算规范.md#stateful-trigger-dependency-closure) |
| `SceneBindingState`、`latest`、`rolling`、内建有状态算子、Timer 语义 | [《场景有状态计算规范》](场景有状态计算规范.md) |
| Current Fact 写入、projection frontier、terminal no-write | [《Scene Current Fact 设计》](Scene%20current%20fact设计.md#3-写入者与完成谓词) |
| 读取屏障、水线、新鲜度、retry、fail closed | [《水线与读取屏障详解》](水线与读取屏障详解.md) |
| Runtime 拓扑、执行阶段与跨组件协议 | [本主规格](场景联动%20Runtime%20主规格总纲.md#4-总体数据流) |

### 7.1 统一求值与 Latest-value JOIN

所有 same-subject bundle、cross-subject target-keyed `LINKED_DELTA`、Schedule、数据面
`SubjectLifecycle` 和 Timer fire 最终都回到对应 evaluation-subject lane。跨组件调用顺序为：

```text
producer / Admission
  -> ProcessingContext 与身份门禁
  -> ProjectIndex matching / target bundle
  -> partition owner、assignment token 与 evaluation-subject lane
  -> current Binding / SubjectContext scope reconciliation
  -> Current Fact readiness / coverage / conflict
  -> freshness / missing
  -> tentative latest / rolling / receipt / FSM / Timer
  -> predicate / transition candidate
  -> PostgreSQL Action Outbox 接受边界
  -> 发布内存 next state，并异步 handoff 完整 scene_state
```

数据 bundle 只有在至少一个 TRIGGER argument 取得 `triggerRight` 时才进入业务求值；每个 Binding
每个完整 bundle 最多求值一次。Latest-value JOIN 使用本轮 tentative state 中的当前参数视图，不要求
不同设备参数具有相同 `eventTs`；需要同一 event-time 切面、完整事件时间或大窗口时必须使用 Flink。
不同设备/参数的独立 Trigger bundle 仍各自求值，但同一 evaluation-subject lane 串行更新完整谓词
共享的 `Edge(RISING)`；只有 `false -> true` 形成一次 action set，后续 `true -> true` 不创建新
Outbox。该门复用现有 `SceneBindingState` 与异步 best-effort `scene_state`，不新增 durable gate；
因此只承诺 active owner/lane 正常持有 accepted state 期间的单次上升沿，不承诺跨崩溃、rebalance
或滞后恢复副本严格去重。

统一求值顺序、`NOT_READY` 和动作接受细节见
[《场景有状态计算规范》§4、§10](场景有状态计算规范.md#4-统一求值顺序与-latest-value-join)，
完整谓词动作门见[§4.1.2](场景有状态计算规范.md#aggregate-predicate-rising-gate)。

### 7.2 SingleValue 更新规则

`latest` 的精确规则只见
[《场景有状态计算规范》§5.1 `compareLatest`](场景有状态计算规范.md#state-component-latest)。
Runtime、Current Fact 与 bootstrap overlay 必须调用同一 comparator；不得用 Kafka offset、到达顺序、
配置 revision 或最后写赢补充排序。

架构上必须把 value merge 与 `triggerRight` 分开：comparator 判定 duplicate 时不修改 value、也不刷新
freshness；数据求值权由 `SceneBindingState` 中可恢复的 Binding input receipt 独立裁决。若 Current
Fact 只是被独立 projector 抢先写入，当前输入即使得到 `DUPLICATE_NOOP`，仍可能取得该 Binding
尚未消费过的首次 `triggerRight`。receipt 已记录的 redelivery 不再取得触发权；任何事实身份、顺序域
或同序不同 payload 冲突都必须在 grant 前 fail closed/quarantine。

### 7.3 Rolling 更新规则

`rolling` 的精确规则只见
[《场景有状态计算规范》§5.2](场景有状态计算规范.md#state-component-rolling)。LATEST 样本按
`factDeltaId`、OCCURRENCE 样本按 `occurrenceId` 的稳定身份去重；`eventTs` 只用于窗口排序和
cutoff，同一 timestamp 的不同身份必须并存。相同身份且 envelope 相同的 duplicate 不 append、
不刷新 freshness；envelope 漂移在触发权 grant 前 fail closed/quarantine。

样本是否首次触发由可恢复 Binding input receipt 与 `runtimeTriggerObserved` 独立裁决，并与 tentative
next state 原子推进。后续 redelivery 不再取得触发权，不能依赖 Outbox `effectId` 掩盖 rolling
重复求值。

### 7.4 脚本限制

Script 只能消费当前 `SceneBindingState` 的有界参数，必须确定性、无外部 I/O、无随机数且不直接读取
wall-clock，并受 CPU、内存、执行时长与结果大小限制。复杂事件时间和大状态转入 Flink；Script 与
Alarm/FSM 的扩展边界见[《场景有状态计算规范》§7](场景有状态计算规范.md#7-script-与-alarmfsm-扩展边界)。

## 8. `scene_state` 逻辑与物理模型

### 8.1 完整状态

Kafka 每个 record 只保存一个 Binding 的可恢复业务状态，而不是序列化整个 Go Actor、mailbox、锁、缓存或 timer handle。Actor lane、BindingState 的层级、partition ownership 与进程内 transient 边界由
[Runtime 调度分章 §§2–4](Scene%20Runtime%20调度、分片与故障转移.md#runtime-model)
唯一规定。`SceneBindingState` 是状态容器，`latest`/`rolling` 是状态组件；五类有状态算子和 Timer
不得与这两层混称。

下列结构仅用于说明跨组件持久化边界：**架构示意，非机器契约；字段约束见
[《场景有状态计算规范》§3](场景有状态计算规范.md#state-container-scene-binding-state)，YAML 机器契约
建立并冻结后以其为准，本文不声明其已经建立。**

```text
SceneBindingState {
  identity + trusted metadata
  arguments / rollingArguments
  inputReceipts { stable input identity -> runtimeTriggerObserved }
  bounded members + scope/version guards
  operator / extension states
  timerState[] { context + identity + anchor + current-slot identity }
}
```

生产 `scene_state` 的非 tombstone value 只能来自受信任的
`production + LIVE + SCENE_ID_V2`；完整 state、receipt 与 Timer slot 必须保留其 ProcessingContext、
身份版本和必要 provenance。缺失、未知、冲突或来源不可信时 fail closed/quarantine，绝不默认补
`LIVE`。控制面只可发送无 value tombstone；任何 bootstrap、reconciliation 或 timer fire 求值都必须
重新通过同一数据面门禁和 lane/effect gate。

`configRevision` 只是说明最后一次处理该 state 时使用的 current config。恢复时以 ProjectIndex 中的当前 BindingCtx 为准，并通过 `semanticHash` 判断 preserve 或 close+tombstone+next-live bootstrap；不能因为 revision 数字不同就创建新 key。

共享 target 的 member slice、REMOVE guard 和 device-target 的 `lastSubjectAggregateVersion` 只是
best-effort last-known scope view，不是 barrier。版本更高、member inactive、guard/state 缺失或恢复
后的首次业务求值都必须读取 current `SubjectContext` 做 reconciliation；迟到 Linked/INIT 不得凭
payload 或缺失 state 复活 scope。

### 8.2 物理 Topic

`scene_state` 是 Kafka compacted Topic：

```text
record key =
  (tenantCode, bindingId, evaluationSubjectKey)
```

要求：

- `scene_event` 与 `scene_state` 的 partition count 相同；
- Linked/Schedule/LifecycleTarget 与 state 按同一 `(tenantCode, evaluationSubjectKey)` route key 共置，并统一使用 §14.1 冻结的 route bytes/hash；
- `scene_state` record key 保持完整复合身份以支持 compaction；
- state partitioner 从复合 key 中提取 `(tenantCode, evaluationSubjectKey)`，不能依赖 client 默认 key hash；
- SOURCE/SubjectLifecycle 按 `(tenantCode, sourceSubjectKey)` 进入 source lane，跨 lane 只能通过终端 target bundle；
- 每轮成功接受且 `SceneBindingState` 实际变化时，对该 key handoff 一条完整 next image；
  duplicate/stale 且无状态变化的 no-op 不 append，删除/退出/reset 使用下述 tombstone；
- state producer 为异步发送；是否被 producer 接受或收到 ACK 都不构成动作接受，也不进入 Runtime
  partition completion 谓词；
- `scene_state` Kafka ACL 只允许 Scene Runtime `state-writer` principal 写入；producer identity、build 与 contract version 记录在 bounded audit headers；
- restore 的来源信任来自受管 Topic 与独占写 ACL，consumer 不声称能仅凭 payload 字段证明 principal；但仍必须校验 payload 的 namespace/mode；不新增 commit-attestation；
- send failure 只记录日志、指标和告警；
- tombstone 用于 Binding 禁用/删除、Product/Project scope 退出、entity 删除和显式 reset；
- 控制面发送的 state 管理记录只能是无 value 的 tombstone；不得用控制面构造或覆盖完整 business state；
- 兼容配置更新或下一条 live bootstrap 仍使用原 key，不因 revision 变化留下旧 key。

Compaction 只最终限制同 key 的稳定存储量与恢复重放量，不减少完整 image 的规范序列化、Producer
发送、leader append、副本复制与 Cleaner I/O；实时容量必须使用 §15 的 state append/s 和 bytes
计算，不能用 compact 后的 live key/bytes 替代。

上述 event/state 共 partition 是物理契约；partition 的动态 ownership 与 state-before-event
restore 分别由[Runtime 调度分章 §§6–7](Scene%20Runtime%20调度、分片与故障转移.md#runtime-state-machine)
唯一规定。

### 8.3 状态配额

状态容量与 Scene/Flink 边界见[《场景有状态计算规范》§11](场景有状态计算规范.md#11-scene-小状态与-flink-大状态的边界)，
冻结值与故障验收见[《SLO、故障测试与实施》§2.1](SLO故障测试与实施.md#21-已冻结硬约束)。
Runtime 的架构不变量只有一个：完整 next state 必须先整体校验；任一状态组件、receipt、member、FSM
或 Timer slot 超限时整个 Binding fail closed，不发布 tentative state/动作，不得静默裁剪后继续求值。

## 9. 恢复模型

### 9.1 状态优先

新 owner 必须先完成对应 `scene_state` partition 的 bounded-cut staging restore、Actor/timer
激活和 assignment/token 复验，才开放同 partition 的 `scene_event`。`[start,S0)`、staging、
timer 恢复激活与失权丢弃规则由[Runtime 调度分章 §7](Scene%20Runtime%20调度、分片与故障转移.md#runtime-restore)
唯一规定。`scene_state` handoff 是异步 best-effort 恢复副本；state-before-event、partition
ownership 和 activation queue 不改变 PostgreSQL Outbox 的动作接受边界。恢复 ADR 仍未冻结。

### 9.2 有效 state

恢复只接受受信 `production + LIVE + SCENE_ID_V2` 的完整 state，并把它绑定到 ProjectIndex 中
current enabled BindingCtx。`semanticHash` 兼容才可 preserve；不兼容、Binding 禁用/删除或身份不可信
时 close/quarantine+tombstone，并等待 next-live bootstrap。ProjectIndex 未 ready 时不得开放该
partition 的事件处理。

恢复不变量为：单 key 损坏只隔离该 key；状态组件、算子/扩展、receipt 和 Timer business state
必须作为同一完整 image 恢复；依赖 membership 的 state/Timer 在首次合格数据 trigger 或恢复 Timer
fire 求值前必须用 current `SubjectContext` reconciliation。失配时 close+tombstone/cancel，lookup
失败时 retry/fail closed；恢复线程和 callback 都不得越过 lane、assignment/token、current-slot 或
effect gate 直接产生副作用。

### 9.3 缺失、损坏或不可用 state

缺失、损坏或不可用 state 只允许由已验证的 production+LIVE SOURCE/LINKED、Schedule 或数据面
`SubjectLifecycle` trigger 在对应 handler 内惰性初始化；不执行全租户/全实体扫描、历史
`scene_event`/Canonical replay 或 RocksDB/snapshot/tail 恢复。`REPLAY` 在 V1 零 Scene 求值。

Binding lifecycle 与 explicit reinit 只做 control-only close/reset/tombstone、取消 Timer 并标记
next-live bootstrap；不得读取 Current Fact 或立即创建业务 identity、FSM、Timer、Alarm、transition
和 Outbox。

上述 control-only reset 仍有持久化残余窗口：若 async tombstone 交付失败，同一 state key 后续以 enabled 且 compatible semanticHash re-enable/重新适用，并在成功 next-live bootstrap 或新的完整 `scene_state` 覆盖旧值之前发生 rebalance，新 owner 可能从 compacted Topic 恢复旧 state 与 ARMED timer。进程内 next-live bootstrap 标记以及 current enabled + semanticHash 只能证明当前配置允许加载兼容 state，不能证明 reset 已持久。

V1 将其显式接受为[Runtime 调度分章 §9](Scene%20Runtime%20调度、分片与故障转移.md#runtime-best-effort)定义的残余窗口。effectId 只能收敛相同 logical action，不能证明 state 安全；本设计不新增 durable reset watermark、reset epoch 或其它持久 reset authority。

跨组件懒建顺序为：

```text
current BindingCtx 建立 tentative 空 state
  -> 并行读取必要 Current Fact 与受限 rolling history
  -> readiness / coverage / conflict
  -> 以查询结果为 baseline，按新规范 §5 合并当前完整 bundle
  -> init(false) / NOT_READY 或使用当前 trigger identity 求值
  -> 经过 Outbox 接受边界后发布内存 next state
  -> 异步 handoff 新的完整 scene_state
```

当前 Delta 在查询结果之后 overlay，但不能无条件覆盖：latest 必须使用新规范 §5.1 的
`compareLatest`，rolling 必须使用 §5.2 的稳定样本身份。必要参数或 coverage 不足时保留已接受参数并
进入 `NOT_READY`，zero-effect，等待后续合格 trigger；没有后续 trigger 时可以长期不推进。

### 9.4 ProcessingContext 强门禁 rollout 与历史迁移

本次同时把 namespace 加入 timer/schedule/lifecycle/transition/effect identity。新算法固定为
`SCENE_ID_V2`，旧无 namespace 算法仅命名为
`LEGACY_NO_NAMESPACE_V1` 以识别已接受的历史 Outbox；它不得再创建新的
SceneEvent、state、timer、transition 或 effect。Runtime 绝不为历史记录默认 `LIVE`，也不允许同一
trigger 被两种算法双求值。

rollout 必须按以下门禁顺序执行并保留逐阶段证据：

1. inventory 旧 Admission、Scheduler、SubjectLifecycle relay、Runtime 派生 producer、
   `scene_event` backlog、context-less state、identity-bearing Timer/transition、Action Outbox 与旧
   dispatcher，并冻结 maintenance window、容量和 RTO。
2. 每个旧 producer 先停止获取其已捕获 input frontier 之后的新工作，排空该 frontier 内工作及全部
   publish callback 后停写；所有旧 producer 停止后，捕获生产 `scene_event` 的 final end offsets。
3. 旧 Runtime 的 consumer committed next-offset 必须逐 partition 到达上述 final end，并排空全部
   Timer callback、async `scene_state` send、PostgreSQL/Kafka in-flight 后停写。
4. 旧 dispatcher 先停止取得新 lease/dequeue，排空已领取的外发、status update 与 PostgreSQL
   in-flight，冻结稳定 legacy pending set；完成后才撤销全部旧 writer 与 dispatcher credentials。
5. 无法排空的旧 event/in-flight 只能由受审计迁移工具隔离，并绑定 §13.2 最终损失审批。旧 state
   只有在 ACL/audit 能证明来自 production live writer，且不含 Timer slot、armed/pending trigger、
   未决 transition 等旧 identity-bearing 状态时，才允许补齐
   `production + LIVE + SCENE_ID_V2`；其余 state 一律 tombstone 并按类型化冷启动。
6. legacy pending Outbox 只有在来源证据充分时，才允许补齐 `production + LIVE` 与
   `LEGACY_NO_NAMESPACE_V1`，并保持原 `effectId`、payload、target 和 due time 不变，只投递、
   不重算；证据不足者进入 `BLOCKED_CONTEXT`。
7. 启用 required wire、producer、Actor、restore 与 dispatcher 的 V2 全链路门禁后再恢复生产；
   只有 legacy pending set 全部进入终态并超过审计保留窗口，才可移除 legacy 只读兼容。

上述 producer input frontier、final `scene_event` end、Runtime committed next-offset、dispatcher
pending set 和 credential revocation 都是 cutover 证据。deployment cutover frontier 与 Runtime
调度的 per-partition completion frontier、Current Fact projection frontier 都不是同一概念。

首条 `SCENE_ID_V2` 记录被接受即是 identity point-of-no-return：之后的运行回滚只能
切到仍强制 required context/version、且只生成 `SCENE_ID_V2` 的兼容 binary，或停写后
向前修复；不得恢复任何 `LEGACY_NO_NAMESPACE_V1` event/state/timer writer。只有尚未
产生任何 V2 record 时，才允许在保持 fence 并确认迁移未生效后撤回整个 maintenance
window。

### 9.5 类型化冷启动语义

完整类型化规则见
[《场景有状态计算规范》§10](场景有状态计算规范.md#10-冷启动恢复not_ready新鲜度与-fail-closed)。
架构摘要如下：latest 以 Current Fact 为 baseline 后按 `compareLatest` overlay；rolling 只接受可证明
coverage 的受限窗口；Edge/Change、Hysteresis 只建 baseline，首轮不补历史 transition；Cooldown
无法恢复时重置并显式接受已量化的少量重复风险；Geofence 当前坐标/围栏只建 baseline，历史
ENTER/LEAVE 不补；已接受 delayed action 只从 PG Outbox 恢复。

Duration/Debounce 不得使用“必要 latest 最大 timestamp”近似 `armedAt`。DATA trigger 默认
`OBSERVED_AT`，且只取本轮取得 `triggerRight` 的 TRIGGER deltas；`EVENT_TS` 仅在
`schedulerDeadlinePolicyRef` 可确定性校验/映射时间域时允许。合法非数据 trigger 只使用其契约冻结
anchor，例如 Schedule 的 `plannedFireTs`；没有合格 anchor 时保持 `NOT_READY`，不得 arm。

## 10. Timer 与 delayed action

### 10.1 普通 timer

普通 Timer 是 Duration、Debounce、Cooldown gate 清理和 Alarm duration 的可恢复运行机制，不是业务
算子；精确 slot、anchor 与 fire 语义只见
[《场景有状态计算规范》§8](场景有状态计算规范.md#8-timer运行机制)。进程内 handle 只是调度加速，
不序列化也不是事实来源；`scene_state` 只保存恢复所需的 context/version、operator/semantic identity、
冻结 anchor、`logicalTimerId/timerFireId`、`armedAt/dueAt` 与 current-slot 状态，不建立 PG
`scene_timer`。

retry、redelivery、rebalance 和 restore 必须复用 slot 中已冻结的 anchor、`dueAt` 与 fire identity。
callback 原样继承 slot context/version，经当前 partition assignment/token 回到同一 evaluation-subject
lane，再检查 current Binding、scope、readiness/freshness、predicate/candidate 与
`logicalTimerId/timerFireId/current-slot`。恢复后仍需 scope reconciliation 时，Timer fire 可以作为
首个合格数据面 evaluation trigger，但不能绕过 reconciliation；失配 close/cancel，lookup 失败
retry/fail closed。slot 已删除、lane/token 失效或 identity 不匹配的旧 callback 稳定 no-op。handle
安装、activation queue 和 revoke 次序只见
[Runtime 调度分章 §§5～8](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)。

### 10.2 Reliable delayed action

普通条件 Timer 与 Reliable Delayed Action 的分界是 PostgreSQL Action Outbox commit。带动作的
transition、关联 FSM/Timer/Cooldown next state 只有在整组 Outbox 事务 commit 成功后才被业务接受；
commit 失败不得发布这些 tentative 变化。`scene_state` producer 接受/ACK 不构成动作接受边界。

已接受 delayed action 以 PG Outbox 行保存不可变 payload 与 `available_at/due_at`，dispatcher 只领取
到期记录。V1 默认 `CONTINUE_AFTER_ACCEPT`：后续 scope 变化不自动撤销已 commit 行；需要撤销时必须
产生显式补偿动作，不扫描并回滚旧 transition，也不另建 PG state/timer 行。完整事务顺序见 §12.3。

## 11. Schedule 与 scope

### 11.1 借鉴 TB current-target reevaluation 的 fanout

Schedule fire 时：

```text
1. 形成稳定 scheduleOccurrenceId
2. 在 ProjectIndex read lock 下取得本次 fire 的 current compiled BindingCtx，然后释放锁
3. FIXED_DEVICE/FIXED_AREA/PROJECT 直接解析 target；MATCHED_DEVICE 通过 Things current membership API 按稳定 device key 做有界 keyset pagination
4. 每页校验 tenant/project、按 evaluationSubjectKey 排序去重，并用 `limit + 1` 探测超限；任何查询/页失败都 retry 整个 fire，不能发送已读到的前几页
5. 先在内存中形成不超过硬上限的本次 target list；超限则在发送前拒绝整个 fire
6. 按 evaluationSubjectKey 汇总目标 Binding
7. 形成完整 `SCHEDULE_EVALUATE` target bundle；递交见下方说明
```

完整 `SCHEDULE_EVALUATE` target bundle 的 target-partition/lane delivery 由
[Runtime 调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)
唯一规定。

受信平台 Scheduler 产生的 fire 与每个 `SCHEDULE_EVALUATE` handler 必须继承并验证唯一公共 `SceneEvent` envelope 中受信任的 `production + LIVE + SCENE_ID_V2` context/version；wire source 只有公共 envelope，不在 ScheduleEvaluate payload 重复字段。缺失、非法、非生产组合、错误 version 或来源不可信时 fail closed/quarantine，不枚举 target、不初始化 state、不求值。

稳定身份：

```text
scheduleOccurrenceId =
  H(SCENE_ID_V2, processingNamespace, tenantCode, scheduleId, plannedFireTs)

scheduleEvaluationId =
  H(SCENE_ID_V2, scheduleOccurrenceId, bindingId, evaluationSubjectKey)
```

`SCHEDULE_EVALUATE` 至少携带：

```text
ScheduleEvaluate {
  tenantCode
  projectId
  evaluationSubjectKey
  scheduleId
  scheduleOccurrenceId
  plannedFireTs
  evaluations[] {
    bindingId
    scheduleEvaluationId
  }
}
```

Schedule/config revision 可随 payload 记录用于审计，但不进入 occurrence/effect identity；同一 scheduleId 的同一计划时刻就是同一 logical occurrence。

ProjectIndex 只保存规则，不保存全量设备，因此不能凭本地 index 枚举 project/area/product 下的所有设备。一次进程内 fanout 固定使用步骤 2 取得的完整 BindingCtx；membership page 使用查询时的 Things current 数据。步骤 5 得到的是一次 fire 的有界、进程内 target list，但不是持久化 affiliation snapshot：分页期间 membership 可变化，崩溃重试从头按 current membership 枚举，可能出现重复或少量边界差异；已经形成的相同 `(bindingId, evaluationSubjectKey)` 仍得到相同 `scheduleEvaluationId` 并由 effectId 收敛。

Things membership API 的规范输入至少是 `(tenantCode, projectId, compiled ScopeSelector, afterDeviceKey, limit)`，输出按稳定 device key 升序；API 必须在数据库侧同时执行 project 与 area/product/device selector，Runtime 不能取 project 全量后在内存补过滤。

keyset cursor 只用于一次运行内的有界分页，不跨重启持久化。V1 对每次 fire 的 target 总数、page size、查询并发和发送速率设置硬上限；超过上限时不发送任何 target bundle并告警，不引入 immutable target snapshot、跨重启游标或 offset barrier。要求全量、稳定大群体调度时应拆分 project/area 或交给专用批任务/Flink 作业。

Scheduler/fanout 不直接写动作 Outbox。

接收端 Scene Entity Actor 仍需：

- 读取/恢复 state；
- 评估 IF/current-state gate；
- 更新 FSM；
- 决定是否写 Outbox。

### 11.2 Scope overlap

- 不同场景可以同时匹配同一实体；
- 同一场景/template 的多个 enabled binding 若 scope 重叠，发布时拒绝；
- 所有 selector 先与 Binding 的 `projectId` 相交，跨 project 结果发布时拒绝；
- area subtree、product 和 device scope 使用符号索引，不做 rule × device 展开；
- Schedule fanout 有 per-fire 目标数、速率和 tenant/project 配额；
- 同一 evaluation subject 收到一个 target bundle，bundle 内每个 Binding 最多求值一次。

### 11.3 Product、Project、Area 生命周期

本节采用 TB 的事件分类处理，不建立统一 immutable affiliation snapshot、ENTER/STAY/EXIT 状态机或 activation barrier。

所有 SubjectLifecycle 先验证 old/new context，并在所需 ProjectIndex 均 ready 时计算完整受影响集合；缺任一 index 就 retry/fail closed，不能用部分 lookup 结果生成 operation。重复 operation 必须幂等。跨 evaluation target、跨 old/new project 的多个 bundle 不具备原子提交：单个成功 bundle 必须完整，某些 target 最终失败时按 §5.5 标记并 quarantine/replay，不能把整个迁移描述成原子切换。

跨 target 的业务 bundle 递交遵循
[Runtime 调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)。
Kafka assignment revoke 下的 partition context close 是同一分章 §8 的 ownership lifecycle；
本节的业务 `DELETE_STATE` 只关闭单个 BindingState，不是 Kafka assignment revoke，
也不改变 partition ownership。

**Product change（对应 TB Device Profile change）**：

```text
old-product-only Binding
  -> DELETE_STATE，或从共享 evaluation target REMOVE_MEMBER

new-product-only Binding
  -> INIT_STATE，或向共享 evaluation target ADD_MEMBER

old/new 都仍匹配的同一 bindingId
  -> ArgumentSource/semanticHash 兼容时保留原 state，否则 REINIT
```

INIT/ADD 时从 latest/attribute store 读取当前所需参数，合并 lifecycle 触发后立即求值；不回放历史 Canonical。

**Project change**：Project 是强隔离边界，不能照搬 TB 只刷新 `CURRENT_OWNER` 参数的做法。

```text
旧 project 的全部适用 Binding -> DELETE_STATE / REMOVE_MEMBER
新 project 的全部适用 Binding -> INIT_STATE / ADD_MEMBER
```

V1 不允许同一 Binding 跨两个 project 延续状态。old/new project index 必须按稳定顺序取得完整 read view，避免锁顺序死锁。

**Area membership add/remove（对应 TB Entity Relation change）**：

- 先按 old/new area path 求 Binding 集合差；同一 bindingId/evaluation target 在共同祖先或 project/product scope 下仍匹配时保留 state；
- device-target Binding：只对 old-only scope 删除旧状态，对 new-only scope 从当前值 INIT；
- fixed area/project target：只对该设备的有界 member/argument slice 执行 REMOVE_MEMBER 或 ADD_MEMBER，然后重算目标 Binding；
- 一次 area move 是 old path remove + new path add，不承诺二者与 telemetry 形成原子切面；
- area subtree 的非空节点换父级必须由 Things 控制面展开成受影响设备的有界 lifecycle job；V1 若无法可靠展开则拒绝该操作。

**Entity delete**：删除该设备拥有的全部 BindingState，并向受影响的 cross-subject evaluation target 发送 REMOVE_MEMBER；不存在 state 时视为幂等成功。

DELETE_STATE 的本地效果固定为：

```text
1. 从 Actor map 移除 BindingState
2. close state，取消内存 timer handle
3. 异步发送相同 scene_state key 的 tombstone
```

它不产生通用补偿输出：

- 已 active Alarm 不因 scope exit 自动 clear；
- 已 commit action Outbox 继续投递；
- 已写 telemetry、已发 command/webhook/notification 不撤销；
- 若业务要求 clear/补偿，必须由显式场景动作表达。

## 12. Outbox 与动作幂等

### 12.1 多输出 Outbox

一次 transition 可以在同一 PG 本地事务中插入多条：

- AlarmSignal；
- DeviceCommand；
- Webhook；
- Notification；
- ReliableDelayedAction。

上述都是 Action Outbox row 的动作 kind/payload；`AlarmSignal/ActionIntent` 不是另建一组与 state/timer 同事务提交的 Scene 状态表。

Action Outbox 是动作可靠投递的唯一事实来源；它与 §6.1 的 Binding lifecycle Outbox 分表、分 dispatcher、分指标。

每个 Action Outbox row 必须保存 `processingNamespace`、`processingMode` 与 `identityAlgorithmVersion`。新 row 只能写 `SCENE_ID_V2`；§9.4 迁移的 `LEGACY_NO_NAMESPACE_V1` pending row 只允许 dispatcher 以原 `effectId` 只读兼容投递，任何 Runtime/工具都不得以该版本创建或重算 row。只有 Scene Actor DB role 可 INSERT Action Outbox；dispatcher role 只可 SELECT 和更新投递状态，无 INSERT、改 payload、context 或 identity version 权限。dispatcher 在 dequeue 与每次外发前都只接受受信任 `production + LIVE` 与允许的 identity version；context+schema/version 校验仍必需，缺失、未知、组合不一致、非生产组合或来源不可信时不得外发，不能依赖下游幂等掩盖误投递。该 durable trust boundary 不新增 commit-attestation。

非法 pending row 的 terminal status 为 `BLOCKED_CONTEXT`。dispatcher 必须以 CAS/本地事务将该 row 转为 `BLOCKED_CONTEXT`，记录 bounded reason、audit 与 quarantine reference，并从 due index 移除；它不重试、不外发、也不阻塞无关 row。若配置严格 target 顺序，必须暂停该 target lane 并告警/page，禁止后序越过。不得原地修改 context/version 后重回 pending；经人工判定后只能显式 cancel，或创建一条新的受信动作。

### 12.2 确定性 effectId

```text
effectId = H(
  SCENE_ID_V2,
  processingNamespace,
  tenantCode,
  bindingId,
  evaluationSubjectKey,
  sourceEventId | timerFireId | scheduleEvaluationId | lifecycleEvaluationId,
  transitionId,
  actionId,
  targetKey
)
```

`transitionId` 必须由本次 trigger 和语义 transition 确定性导出，不能使用进程内自增序号：

```text
transitionId = H(
  SCENE_ID_V2,
  processingNamespace,
  tenantCode,
  bindingId,
  evaluationSubjectKey,
  operatorId,
  sourceEventId | timerFireId | scheduleEvaluationId | lifecycleEvaluationId,
  transitionKind
)
```

同一 logical trigger 被旧/新 partition owner 重算时，必须得到相同 `transitionId` 和 `effectId`，并原样保留共同的 ProcessingContext。`configRevision` 不进入两个 identity；Binding 修改通过 stable bindingId、当前 state 兼容策略和稳定 actionId 管理。

`actionId` 是 Binding 内不可变的动作定义身份。一旦修改会影响输出类型、payload 模板、target、delivery policy 或业务语义，控制面必须创建新 `actionId` 并停用旧动作；仅修改展示名称等非语义 metadata 才可保留。禁止复用同一 actionId 表示两种输出，否则跨配置重试会错误命中旧 effectId。

数据库约束：

```text
UNIQUE(tenantCode, effectId)
```

重复处理使用 `INSERT ... ON CONFLICT DO NOTHING` 收敛。

### 12.3 状态与 Outbox 顺序

对会产生动作的 transition：

```text
1. 以当前 accepted state 为只读 base，在逻辑隔离的 mutation view 中计算 tentative next state
2. 规范序列化 tentative state，并校验全部点数/timer state/32 KiB 配额
3. 开启 PG 事务
4. 插入全部 Outbox rows
5. commit
6. 将 tentative state 发布为 Actor 当前内存状态
7. 注册/取消内存 timer
8. 异步发送步骤 2 已校验的完整 scene_state
```

这里的“逻辑隔离”是提交语义，不是要求每轮深拷贝完整 `SceneBindingState`。推荐实现为
`immutable accepted base + bounded mutation overlay`，或者只复制本轮触及组件的 copy-on-write；
规范序列化器可直接遍历 base+overlay 生成完整 next image。实现也可采用结构共享的不可变容器，
但不得让共享 map、slice、rolling index 或可变子对象把 tentative mutation 泄漏到 accepted base。

因此，百万设备不会同时多出百万份 tentative state：accepted state 按 partition owner 分布并常驻，
tentative mutation 的同时存活数量由每节点实际 evaluation/PG in-flight 并发上限约束。真正需要控制的
是每次 mutation/编码的分配字节、每秒分配带宽、Producer in-flight bytes 与 GC/heap headroom。默认
实现不得无条件深拷贝完整对象图；若选择完整复制，必须通过 §15.5、§19 的容量与故障 POC。

freeze/序列化/配额失败或 PG rollback 后，accepted state、receipt、FSM、Timer slot 和进程内 Timer
handle 必须与本轮开始前完全一致。成功时在 lane 内一次 publish/指针替换 next state；步骤 2 产生的
encoded bytes 在 Producer 接管或 callback 完成前必须保持不可变，不得被 buffer pool 提前复用。

步骤 2 失败时 fail closed：不写 Outbox、不替换内存 state、不注册 timer，也不发送
超限 state。

Outbox 事务失败：

- 不发布 tentative transition；
- 不发送对应 next state；
- 处理标记失败；
- 先按配置的次数/退避重试整条 transition；耗尽后写入含原 trigger 与错误原因的 quarantine；
- quarantine 成功或达到人工配置的最终失败策略后才允许提交输入 offset；因此承诺是“Outbox commit 后可靠投递”，commit 前仍存在明确动作丢失窗口，不是端到端零丢失。

不产生动作的普通参数更新或 FSM 更新也必须先完成相同的规范序列化与配额校验，才可
推进内存状态并异步发送完整 state。

状态删除、scope exit 或后续配置变化不会扫描并撤销已经 commit 的 Action Outbox。Alarm clear、命令撤销或其他补偿只有在场景显式产生新的 transition 时才形成新的 Outbox row。

上述 tentative state 序列化、Action Outbox 事务、commit、内存 state/timer 发布与异步
state handoff 顺序仍由本节唯一定义；它如何进入 partition-local completion frontier
由[Runtime 调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)
唯一规定。

## 13. Kafka 交付、事务与失败窗口

### 13.1 Producer

SOURCE/Linked/LifecycleTarget/Schedule 生产采用：

- `acks=all`；
- producer retries；
- Kafka producer idempotence；
- 正常路径等待发送 callback；
- 无 `transactional.id`；
- 无 `sendOffsetsToTransaction`；
- 无跨 Topic 事务。

Kafka producer idempotence 只处理 producer session 内重试，不等于业务 deterministic event ID。

Binding lifecycle relay 同样至少一次发送，但使用独立配置 Topic；它的 project revision 只用于缓存收敛，不建立与 `scene_event` 的事务或 offset 切面。

### 13.2 Event commit

Scene input 仍是 at-least-once。SOURCE/Linked/LifecycleTarget 等非 state data-event pack
的正常路径等待 producer callback；lookup/index 失败时不产生部分 bundle，
producer failure 与 pack timeout 执行有界 retry/取消，耗尽后保存完整原 record、
错误原因与失败 target 到 quarantine，或进入批准的最终失败处置。
Linked/LifecycleTarget 的最终失败仍保留小概率 target 更新丢失窗口。

`scene_state` 仍是 best-effort 例外：只对 §12.3 已校验的完整 value 做一次
有界异步 producer handoff attempt。立即拒绝必须记录；producer 接受后不等待 broker ACK/callback，
后续异步失败只记录日志、指标和告警。因此仍存在
Linked/lifecycle target 丢失、state 落后或迟到、rolling 重算、output 形成前
动作丢失，以及 commit 失败后新 owner 重放而旧副作用已落地的重复窗口。

完整 input completion predicate、per-partition sequencer 与安全连续 next-offset 算法只存在
[Runtime 调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)。
offset commit 不表示 Webhook、通知或 DeviceCommand 已由外部系统执行，也不在
`scene_state`、Action Outbox 与 input offset 之间创建跨存储原子性。Outbox `effectId`
只吸收相同逻辑动作的重复，不把整条输入链变成 exactly-once。

### 13.3 Rebalance 与本地 generation guard

V1 只提供进程内 status + local generation token 的 best-effort fencing：Runtime 一旦知道 revoke、
session lost 或 ownership 无法证明，就先使旧 token 失效并拒绝新的数据面副作用。它不能撤销已进入
Kafka Producer/PG 的请求，也不能保证迟到旧 `scene_state` 不覆盖新 state。完整状态机、graceful
revoke、硬故障、Zombie 重连和残余窗口由[Runtime 调度分章 §§6–9](Scene%20Runtime%20调度、分片与故障转移.md#runtime-state-machine)
唯一规定；本设计不增加 DB owner epoch、分布式 lease 或 Kafka transaction。

## 14. Partition 约束

### 14.1 V1 固定 partition count

`scene_event` 与 `scene_state`：

- partition count 相同；
- 冻结共享 `routeKeyBytes(tenantCode, subjectKey)`：两个字段先做 UTF-8 NFC 规范化，再各以前置 unsigned-varint 长度编码并顺序拼接；禁止 delimiter 拼字符串；
- 冻结共享 partition 函数：`partition = toPositive(kafkaMurmur2(routeKeyBytes)) % partitionCount`，其中 `toPositive(x) = x & 0x7fffffff`；Go/Java producer 必须通过同一跨语言 golden vectors；
- SOURCE/SubjectLifecycle 的 `subjectKey = sourceSubjectKey`；Linked/Schedule/LifecycleTarget 的 `subjectKey = evaluationSubjectKey`；所有 producer 都调用上述共享函数，不能交给各 client 默认 partitioner 自行解释复合 key；
- `scene_event` message key 就是对应 `routeKeyBytes`；`stateKeyBytes` 使用相同的“unsigned-varint length + normalized UTF-8 bytes”原语依次编码 `(tenantCode, bindingId, evaluationSubjectKey)` 以支持 compaction，state partitioner 解析后必须用 tenant/evaluation subject 重建完全相同的 `routeKeyBytes`；
- V1 生命周期内不原地增加 partition；
- consumer 实例动态加入/退出，由标准分区分配接管；
- “固定 partition count”不等于把分区固定给机器。

consumer 动态加入/退出和 partition 不绑定机器是架构事实；assignment/context
的 ownership 状态机与 Kafka 基线引用由
[Runtime 调度分章 §§6、10](Scene%20Runtime%20调度、分片与故障转移.md#runtime-state-machine)
唯一规定。

### 14.2 扩容

需要增加 partition 时：

```text
1. 创建新 topic generation
2. 使用新的固定 partition count
3. 建立明确 cutover epoch
4. 停止旧 generation 新写入
5. 切换 Admission / producer / consumer
6. 按新 generation 懒恢复
7. 旧 generation 按保留期下线
```

不在同一 Topic 上原地改变 hash 映射。

这里的 topic-generation cutover 只解决物理 partition 数变化，不是 Binding config activation barrier，也不为普通配置更新计算 effectiveOffset。

## 15. 容量模型

### 15.1 数据面变量

必须同时统计 Kafka 物理 record 和 bundle 内逻辑 delta，不能再把一个点位变化等同于一条 SOURCE_DELTA。

| 符号 | 含义 |
| --- | --- |
| `i` | 来源类别，例如 DEVICE_CANONICAL、FLINK、SCENE_DERIVED |
| `R_i` | 进入 Scene Admission/normalization 前的 source input physical records/s；不是 MQTT packet/s 或点位数/s |
| `f_i` | source input 中至少含一个 Scene-relevant fact、最终形成 SOURCE_DELTA 的比例；已规范化来源取 `f_i = 1` 或直接给出 `S_i` |
| `S_i` | admitted SOURCE_DELTA physical records/s，`S_i = R_i × f_i` |
| `d_i` | 每条 admitted SOURCE_DELTA 平均 relevant `factDeltas` 数 |
| `F_i` | source logical factDeltas/s，`F_i = S_i × d_i` |
| `lambda_i` | 每条 SOURCE_DELTA 平均产生的 cross-subject target-keyed LINKED_DELTA physical records，即 distinct cross-subject evaluation target key 数 |
| `q_i` | 每个 cross-subject target-keyed LINKED_DELTA physical record/bundle 平均 `argumentDeltas` 数 |
| `c_i` | 每条 SOURCE_DELTA 最终求值的 distinct BindingState 数，已含 same-subject + cross-subject 两条路径且 bundle 内去重 |
| `t_i` | candidate evaluation 产生 semantic transition 的比例 |
| `u_i` | candidate evaluation 产生有效 state append 的比例 |
| `eS_i/eL_i` | SOURCE/Linked 平均 physical record bytes |
| `b_i` | event path 每次 append 的平均 `scene_state` physical record bytes，含 key/envelope/record overhead |
| `G` / `g` | `SCHEDULE_EVALUATE` target bundle records/s / 每 bundle 平均 evaluations 数 |
| `J` / `mu` | `SubjectLifecycle` records/s / 每条平均 cross-subject target-keyed LifecycleTargetDelta physical bundles 数 |
| `o` | 每条 SubjectLifecycle 平均影响的 same-subject + cross-subject BindingState/member operations 数 |
| `T` | 到期并进入 lane 的 timer fires/s |
| `eG/eJ/eD` | Schedule / SubjectLifecycle / LifecycleTargetDelta 平均 physical record bytes |
| `uG/uJ/uT` | Schedule evaluation / lifecycle operation / timer fire 产生 state append 的比例 |
| `tG/tJ/tT` | Schedule evaluation / lifecycle operation / timer fire 产生 semantic transition 的比例 |
| `bG/bJ/bT` | 上述三条路径每次 `scene_state` append 的平均 physical record bytes，口径同 `b_i`，tombstone 按实际 bytes 计 |
| `RF` | Kafka replication factor |
| `a` | 每次 total transition 平均产生的 Action Outbox rows |
| `w` | 每条 Action Outbox 生命周期平均 PG write operations |

### 15.2 数据面公式

```text
SourcePhysicalRecords       = sum_i(S_i)
SourceLogicalFactDeltas     = sum_i(S_i * d_i)

LinkedPhysicalRecords       = sum_i(S_i * lambda_i)
LinkedLogicalArgumentDeltas = sum_i(S_i * lambda_i * q_i)

SchedulePhysicalRecords        = G
SubjectLifecycleRecords        = J
LifecycleTargetPhysicalRecords = J * mu

SceneEventPhysicalRecords   =
    SourcePhysicalRecords
  + LinkedPhysicalRecords
  + SchedulePhysicalRecords
  + SubjectLifecycleRecords
  + LifecycleTargetPhysicalRecords

EventCandidateEvals     = sum_i(S_i * c_i)
ScheduleCandidateEvals  = G * g
LifecycleStateOps       = J * o
TimerCandidateEvals     = T

EventTransitions     = sum_i(S_i * c_i * t_i)
ScheduleTransitions  = G * g * tG
LifecycleTransitions = J * o * tJ
TimerTransitions     = T * tT

EventStateAppends     = sum_i(S_i * c_i * u_i)
ScheduleStateAppends  = G * g * uG
LifecycleStateAppends = J * o * uJ
TimerStateAppends     = T * uT

StateAppends =
    EventStateAppends
  + ScheduleStateAppends
  + LifecycleStateAppends
  + TimerStateAppends
```

物理带宽：

```text
SceneEventPayloadBytes =
    sum_i(S_i * eS_i + S_i * lambda_i * eL_i)
  + G * eG
  + J * eJ
  + J * mu * eD

SceneEventClusterBytes = SceneEventPayloadBytes * RF

StateClusterBytes = RF * (
    sum_i(S_i * c_i * u_i * b_i)
  + G * g * uG * bG
  + J * o * uJ * bJ
  + T * uT * bT
)
```

总 transition 与动作：

```text
TotalTransitions =
    EventTransitions
  + TimerTransitions
  + ScheduleTransitions
  + LifecycleTransitions

OutboxRows = TotalTransitions * a
PgWrites   = OutboxRows * w
```

Schedule 与 lifecycle 的逻辑规模还必须用业务侧基数交叉校验：

```text
ScheduleEvaluations = ScheduleFires/s * CurrentTargetsPerFire

LifecycleOperations =
  SubjectLifecycle/s * AvgAffectedBindingTargets
```

这里的 `G` 是按 target 汇总后的 Schedule 物理 bundle 数，通常不等于 `ScheduleFires/s`；`J * mu` 只计算 cross-subject target-keyed `LifecycleTargetDelta` physical bundles，same-subject operation 已包含在 `J * o` 的 state/action 负载中。timer callback 不新写 `scene_event`，但仍进入 state、transition 和 Outbox 容量。

口径说明：

- `f` 回答“多少 source input record 最终形成 SOURCE_DELTA”；`d` 回答“每条 admitted SOURCE_DELTA 内有多少 relevant factDeltas”；
- `lambda` 回答“每条 source record 形成多少条 cross-subject target-keyed LINKED_DELTA physical records”，等于 distinct cross-subject evaluation target key 数；不是物理远端机器，也不是 argument 数；
- `q` 回答“每个 cross-subject target-keyed LINKED_DELTA physical record/bundle 内有多少 argument delta”；
- `c` 已是 bundle 后 distinct state evaluation 数，不再乘 `d`、`q` 或 `1+lambda`；
- `u=1` 是保守容量基线，旧版本/no-op 时实际可小于 1；
- Flink/Scene 派生事实作为独立来源 `i` 完整计入；Schedule/timer/lifecycle 不产生 SOURCE_DELTA，但必须进入各自 physical record、state、transition 和 action 项。

### 15.3 POC 算例，不是产品硬编码

示例只放入 DEVICE_CANONICAL，假定 Flink/Scene 派生输入以及 Schedule、SubjectLifecycle、timer 负载均为 0；因此表内是 telemetry event-path 基线，不代表开启这些路径后的全链路总量：

```text
f      = 20%          每 5 条 Canonical record 接纳 1 条
d      = 8            每条 SOURCE_DELTA 平均 8 个相关 factDeltas
lambda = 0.25         每 4 条 SOURCE_DELTA 平均 1 条 cross-subject target-keyed LINKED_DELTA physical record/bundle
q      = 3            每个 cross-subject target-keyed LINKED_DELTA physical record/bundle 平均 3 个 argumentDeltas
c      = 3            每条 SOURCE_DELTA 最终求值 3 个 distinct state
t      = 8%
u      = 1
eS     = 1.2 KiB
eL     = 0.6 KiB
b      = 1 KiB
RF     = 3
a      = 0.6
w      = 2
```

| 指标 | 20K cruise | 200K sustained | 5M burst offered |
| --- | ---: | ---: | ---: |
| Canonical physical records/s | 20,000 | 200,000 | 5,000,000 |
| SOURCE_DELTA physical records/s | 4,000 | 40,000 | 1,000,000 |
| source logical factDeltas/s | 32,000 | 320,000 | 8,000,000 |
| LINKED_DELTA physical records/s | 1,000 | 10,000 | 250,000 |
| linked logical argumentDeltas/s | 3,000 | 30,000 | 750,000 |
| telemetry-path scene_event physical records/s | 5,000 | 50,000 | 1,250,000 |
| telemetry-path scene_event RF=3 估算流量 | 16.6 MB/s | 165.9 MB/s | 4.15 GB/s |
| candidate eval/s | 12,000 | 120,000 | 3,000,000 |
| event transition/s | 960 | 9,600 | 240,000 |
| telemetry-path scene_state append/s | 12,000 | 120,000 | 3,000,000 |
| telemetry-path state RF=3 复制流量 | 36.9 MB/s | 368.6 MB/s | 9.2 GB/s |
| event-path Action Outbox rows/s | 576 | 5,760 | 144,000 |
| event-path PG writes/s | 1,152 | 11,520 | 288,000 |

5M 列是 offered demand。V1 若 evaluator 只支持 40K source-equivalent/s：

- state 和 PG 实际写率维持 sustained 档；
- 60 秒 burst 约积压 57.6M SOURCE_DELTA；
- 包含未来 LinkedDelta 后，物理处理债务约 72M records；
- 流量回落到 4K source/s 后，约 26 分 40 秒追平；
- 若持续保持 40K source/s，则没有 catch-up 余量。

所有 record size、`d/lambda/q/c/t/u` 和写放大必须由 POC 实测替换。

### 15.4 每节点 ProjectIndex 容量

配置缓存与数据面容量分开计算：

```text
PerNodeIndexBytes =
    EnabledBindings * AvgCompiledBindingCtxBytes
  + PostingEntries  * AvgPostingEntryBytes
  + MapAndAllocatorOverhead

ClusterIndexBytes = PerNodeIndexBytes * RuntimeNodeCount
```

POC 必须分别测量十万、百万 Binding 规模下的每节点内存、冷启动 PG 扫描量、编译 CPU、普通 update write-lock 时间、SOURCE read-lock p99、单 project full rebuild 峰值内存和完成时间。若全量规则缓存不能满足容量目标，再立 ADR 讨论分片配置索引；V1 不提前引入远程 hot-path index。

### 15.5 每节点 SceneBindingState 内存与分配带宽

32 KiB 是单条 `scene_state` **规范序列化后的硬上限**，不是平均 state 大小、Go 解码对象大小或
每节点内存预算。accepted state 按当前分配给节点的 partition 驻留；容量必须至少分开测量：

| 符号 | 含义 |
| --- | --- |
| `N_state,node` | 节点当前驻留的 accepted `SceneBindingState` 数；按 BindingState 计，不等于设备数 |
| `m_decoded` | 单个 accepted state 的平均解码常驻 bytes，包含容器、索引和 allocator overhead |
| `W_txn,node` | 节点准许同时存活的 tentative `StateTxn` 数，含等待 Outbox commit 的事务 |
| `m_mutation` | 单个 `StateTxn` 的 mutation overlay 与临时 scratch bytes，不含 accepted base |
| `N_send,node` | 节点准许已交给或等待交给 Producer、尚未释放 encoded bytes 的 state record 数 |
| `m_encoded` | 单条冻结后完整 state image 的平均 encoded bytes |
| `E_node` | 节点 candidate evaluation/s |
| `a_eval` | 每次 evaluation 的平均临时 allocation bytes，包含 mutation 与编码过程分配 |

基础预算公式为：

```text
AcceptedStatePlanningBytes_node = N_state,node * avg(m_decoded)
TentativePlanningBytes_node     = W_txn,node * p99(m_mutation)
ProducerPlanningBytes_node      = N_send,node * avg(m_encoded)

StateWorkingSetPlanningBytes_node =
    AcceptedStatePlanningBytes_node
  + TentativePlanningBytes_node
  + ProducerPlanningBytes_node
  + ActorTimerAndIndexOverhead_node

TentativeAllocationPlanningBytesPerSecond_node = E_node * avg(a_eval)
```

这些公式是必须实测并留余量的预算口径，不是仅靠平均值即可验收。还必须纳入 partition/tenant
倾斜、rolling 点数分布、p99 state 大小、Producer callback 变慢、PG commit 延迟、allocator 放大与
GC pause。上述是 p99/平均规划值，不是数学上的峰值上界；accepted resident bytes、`StateTxn`
in-flight records/bytes、Producer in-flight records/bytes 和 heap 使用还必须分别设置并验证硬
admission 上限，不能依赖 OOM 充当背压。

仅作量纲说明：100 万设备、平均每设备 3 个驻留 BindingState 时共有 300 万个 state；若平均解码
常驻量是 1 KiB，未计 Actor/Timer/allocator 放大前约为 2.86 GiB 集群内存，若是 4 KiB 则约为
11.44 GiB。它们由 partition owner 分摊，但仍需按最坏倾斜给每节点留余量。若 POC 持续档的集群
为 120K eval/s，且实现每轮深拷贝 4 KiB，则集群还会在编码前产生约 468.75 MiB/s 临时分配；
这正是默认采用 mutation overlay、同时按节点测量分配带宽和 GC 的原因。

## 16. 配额与可观测性

### 16.1 发布期/运行期配额

至少包括：

- per-tenant / project / event candidate cap；
- `lambda` / linked target fanout cap；
- SOURCE `factDeltas`、Linked/LifecycleTarget bundle entries 和 record bytes cap；
- binding/scene/argument/action 数；
- 每节点 compiled Binding/Posting 内存和单 project index/rebuild 上限；
- state 32 KiB、rolling 1000 点；
- 每节点 accepted state count/decoded bytes、tentative in-flight/bytes、Producer in-flight records/bytes、
  evaluation allocation bytes/s 与 heap headroom；
- duration/debounce/cooldown 上限；
- active state/timer 数；
- schedule fire 频率、targets/fire、page size、membership query concurrency 和 action fanout；
- SubjectLifecycle/s、每次受影响 Binding/target 数、area subtree migration job 规模；
- partition-local active SubjectContext cache entries/bytes 与懒加载并发；
- per-Actor deletion guard entries/TTL 与 current-store fallback 并发；
- script CPU、timeout、内存和输出大小；
- Outbox rows/s、pending rows、due horizon；
- 单租户占共享 worker/Kafka/PG 容量比例；
- noisy tenant 隔离和 quarantine。

`maxFSMLifetime` 可以保留为产品配额，但不再是 Canonical replay 正确性的前提。

### 16.2 核心指标

Admission/路由：

- `scene_admission_mode_total{namespace_class,mode,decision}`
- `scene_processing_context_invalid_total{reason}`
- 上述及所有 ProcessingContext 相关指标的标签必须是 bounded enum/class；`jobId`、`replayId` 等高基数标识不得作为 label，只可进入受控日志、trace 或审计字段；
- `canonical_record_rate`
- `scene_source_record_rate`
- `scene_source_fact_delta_rate`
- `f_scene_relevant`
- relevant factDeltas/source p50/p95/p99/max
- `linked_target_bundle_per_source`
- `linked_argument_delta_rate`
- source/linked physical record rate and bytes
- linked send failure/timeout
- Linked stale sourceAggregateVersion / current-scope reject / bootstrap guard load
- route partition mismatch 与 cross-language golden-vector failure

配置/ProjectIndex：

- Binding lifecycle relay lag/failure/duplicate
- node/project last applied revision 与 PG revision drift
- revision-gap rebuild count/duration/failure
- enabled Binding、posting entries、cache bytes
- cold-start scan/compile/install time
- ProjectIndex read/write lock wait/hold p95/p99
- atomic update/swap count
- stale/duplicate lifecycle ignored、future-row rebuild、revision rollback prevented
- cache-not-ready fail-closed/quarantine

Matching/计算：

- candidates/source-record p50/p95/p99/max
- candidate eval/no-op/transition rate
- state ready/not-ready
- script CPU/timeout
- per-tenant throttle/reject

State：

- `scene_state_append_rate`
- `scene_state_processing_context_invalid_total{reason}`（`reason` 为 bounded enum）
- logical/replicated bytes
- serialized size p50/p95/p99/max
- accepted state count、decoded resident bytes 与 per-partition/tenant skew
- tentative StateTxn in-flight、mutation bytes p50/p95/p99 与 allocation bytes/eval、allocation bytes/s
- state encode allocation bytes/record、Producer in-flight records/bytes 与 callback age
- heap live/goal/headroom、GC CPU 与 pause p95/p99/max
- state send failure
- state send callback after local generation revoked
- compacted live key/bytes
- uncompacted bytes/compaction lag
- restore records/bytes/RTO
- cold bootstrap rate/latency/failure
- tombstone rate/reason（binding delete、scope exit、entity delete、reset）

Subject lifecycle：

- lifecycle rate/duplicate/stale aggregateVersion
- SubjectContext cache hit/miss/load latency/failure/version-gap reconciliation
- Product/Project state delete/init
- Area member add/remove
- lifecycle target bundle rate/failure
- lifecycleEvaluationId duplicate、quarantine age/replay attempts/reconcileRequired SLO breach
- member inactive-version guard hit/eviction/current-store fallback
- state deletion guard hit/eviction/current-store fallback
- state close 与 timer cancel latency
- non-empty area move job size/reject

Rebalance：

- partition restore time
- local generation guard drops
- late continuation count
- actor/timer cleanup latency

Outbox：

- insert rows/s
- terminal update writes/s
- unique conflict count
- pending/due lag
- retry/DLQ
- dispatcher drain rate
- `scene_dispatcher_processing_context_invalid_total{reason}`（`reason` 为 bounded enum，不能含 `effectId`、`jobId`、`replayId` 等高基数标识）
- `scene_outbox_blocked_context_total{reason}`（`reason` 为 bounded enum，不能含 `effectId`、`jobId`、`replayId` 等高基数标识）
- WAL/index/table bloat

Lag：

- `scene_event` source backlog
- linked work debt
- consumer lag age
- catch-up headroom/time

## 17. 与 ThingsBoard 4.4 的对齐与差异

| 方面 | TB CF | 本设计 |
| --- | --- | --- |
| 多实体参数 | source telemetry + linked telemetry 重路由到 target entity | SOURCE_DELTA + 稀疏 LinkedDelta 重路由到 evaluation subject |
| 计算语义 | target state latest-value join | 相同 |
| Processing Mode | 未发现 TB 公共消息 envelope 上等价的 `LIVE/BACKFILL/REPLAY` 副作用权限字段；Queue `ProcessingStrategy` 是失败重试/跳过策略，CF reprocessing 是独立历史任务能力 | 平台自有硬契约：必填 `ProcessingContext`，只有 production+LIVE+SCENE_ID_V2 进入生产 Scene，BACKFILL 前置旁路，REPLAY 在 V1 零求值 |
| 状态身份 | `(tenantId, cfId, targetEntityId)` | `(tenantCode, bindingId, evaluationSubjectKey)`，同样使用稳定配置实体 ID |
| 配置更新 | PG current CF + lifecycle 通知 + mutable cache；RECREATE/REINIT/REPROCESS/REFRESH_CTX | PG current Binding + lifecycle Outbox/Kafka + ProjectIndex；同样无 release/offset barrier |
| Profile-targeted CF / Binding 定义更新 fanout | 挂在 Profile 上的 CF 定义更新时，枚举当前 Profile 实体并逐 Actor 应用 | 已存在 state 产生 §6.3 定义的业务 cleanup/update 结果；新 scope 默认懒 INIT，发布即全覆盖需显式 rollout job |
| 节点缓存 | 每节点缓存 CF definitions/links、Profile membership 与 Owner mapping；不缓存完整 Relation 图 | 每节点缓存全部 enabled Binding 的 matching metadata，按 ProjectIndex 隔离 |
| 设备 Product/Profile 变化 | 设备换 Profile 时删除旧 Profile CF state，INIT 新 Profile CF state并从 DB 取当前参数立即计算 | Product old-only 删除、new-only INIT；同一 bindingId 仍匹配时保留 state |
| Project/Owner 变化 | 正常 Device/Asset owner-only lifecycle 只刷新 `CURRENT_OWNER` 参数，不是通用 scope | Project 是强隔离边界，旧 project cleanup、新 project INIT |
| Area/Relation 变化 | 按 CF 类型增量维护：Aggregation delete 会删 slice、重算并持久化；Propagation delete 只更新内存 membership，恢复时再对账 | Area add/remove 增量维护 device state 或 cross-subject evaluation-target member slice，并按本设计统一求值/持久化 |
| 状态格式 | `queue.type=kafka` 时为 keyed compacted full state；`queue.type=in-memory` 有本地 RocksDB state service | 本设计部署形态固定为 Kafka compacted full state，不提供本地 RocksDB 分支 |
| state ACK | `queue.type=kafka` 路径 async send，立即 callback success | 与 TB Kafka 路径相同 |
| 缺 state | 首事件懒 DB bootstrap | 相同 |
| 历史回放 | 无 | 无 |
| 普通 timer | in-process ScheduledFuture；Alarm 持久化 eventCount/firstEventTs/lastCheckTs 等业务 timing state | handle 仅驻内存且业务 timing state 随 state 恢复；额外增加 deterministic timerFireId/current-slot guard，不增加 PG timer/RocksDB snapshot |
| SingleValue | 按 TB CF 源码规则比较时间与可选版本 | 使用新规范 §5.1 的精确 `compareLatest`、冲突 fail closed，并以可恢复 Binding input receipt 独立裁决 `triggerRight` |
| Rolling | TB CF 原实现以时间字段组织窗口样本 | 按 `factDeltaId/occurrenceId` 稳定身份去重；`eventTs` 只排序/cutoff，同时间戳不同身份并存 |
| Linked queue ID | random UUID | 不要求确定性 |
| 通用 Inbox | 无 | 无 |
| 通用 effectId | 无 | 有，因 Scene 直接产生多输出动作 |
| Outbox | 无通用 Outbox | PG Outbox |
| Rebalance fencing | 无 owner epoch/generation | 进程内 best-effort token/status guard；Actor、partition 与 rebalance 执行由[Runtime 调度分章 §§6–9](Scene%20Runtime%20调度、分片与故障转移.md#runtime-state-machine)唯一规定，不增加 durable owner epoch |
| 输出 | TS/attribute/Rule Engine/Alarm 的类型特定路径 | Alarm/command/webhook/notification 统一 Outbox |
| state 删除后的 Alarm | close 只取消 timer/state，不自动 clear active Alarm | 相同默认；只有显式补偿动作才 clear |
| 已产生输出 | 不通用撤销 | 已 commit Action Outbox 不因 scope exit 自动取消 |

本设计主要增加以下业务级机制：

1. **PG Outbox + deterministic effectId**：Scene 是“计算 + 反应”合一，不能让重复 command/webhook 只依赖下游自然幂等；
2. **ProjectIndex 完整 lookup 与本地原子更新**：one-target-one-bundle 不能容忍部分 posting，但不升级为跨节点 barrier；
3. **best-effort partition ownership guard**：相对 TB 增加进程内门禁，但以上表所引 Runtime 调度分章为唯一执行定义，不引入持久化 fencing；
4. **恢复兼容 metadata**：state 保存 `configRevision/semanticHash`，用于 current Binding 下的 preserve 或 close+tombstone+next-live bootstrap 判断；TB state proto 没有这两个字段；
5. **timer stale-callback guard**：deterministic `logicalTimerId/timerFireId` 与 current-slot 检查使 scope 删除后的旧 callback no-op；这是 Scene 加固，不是 TB 原样协议。

输入、当前配置、Profile/Relation 式生命周期、状态和恢复总体遵循 TB CF 的轻量 best-effort 取舍，但上表已明确列出 Scene 为多输出和 stale delivery 增加的差异。特别不增加 immutable release、config activation offset、通用 affiliation snapshot、scope-exit Alarm clear 或 pending-output cancel。Scene Processing Mode 是本平台 Raw/Canonical provenance 与生产副作用隔离契约，不是从 TB Queue `ProcessingStrategy` 或 CF reprocessing 复制来的同名能力；TB 事实依据见[附录 2](附录2-thingsboard规则引擎与CF实现分析.md)。

## 18. 已完成的单一规格迁移

本文作为**主规格总纲**，与[Runtime 调度、分片与故障转移](Scene%20Runtime%20调度、分片与故障转移.md#runtime-scope)
共同组成 scene-linkage 的规范性架构决策源。本总纲拥有产品边界、wire、Runtime 拓扑、跨组件协议、
Action Outbox 事务边界、route/hash 与容量；调度分章唯一拥有 partition ownership、Manager/Actor
执行、completion、restore、rebalance 与 Zombie 协议；[《场景有状态计算规范》](场景有状态计算规范.md)
唯一拥有 `SceneBindingState`、latest/rolling、五类内建有状态算子、Timer 与有状态求值语义。
[目录入口](README.md)只负责稳定导航和状态告知，不形成另一份 ADR。

配套规范与实施文档只负责在架构源的职责边界内细化可实施内容，不得覆盖相邻 owner；任何冲突都必须在同一次变更中联合修订：

- [控制面与匹配契约](控制面与匹配契约.md)；
- [场景有状态计算规范](场景有状态计算规范.md)；
- [Scene Current Fact 设计](Scene%20current%20fact设计.md)；
- [水线与读取屏障详解](水线与读取屏障详解.md)；
- [算子语义与术语](算子语义与术语.md)（共享术语与旧锚点兼容入口）；
- [SLO、故障测试与实施](SLO故障测试与实施.md)。

两份附录是**非规范性研究资料**，只提供竞品、源码事实及比较背景，不承载架构决策：

- [附录：竞品状态方案调研](附录-竞品状态方案调研.md)；
- [附录 2：ThingsBoard 规则引擎与 CF 实现分析](附录2-thingsboard规则引擎与CF实现分析.md)。

旧编号 1～6 篇的有效内容已提取进两份架构源和上述从属文档，旧文件已从现行文档集删除。不得从历史 Git 恢复旧文件或旧结论；后续修订必须以两份架构源的职责拆分为决策基线。

### 18.1 跨目录规范一致性

[父架构](../enterprise-iot-platform-architecture-design.md)拥有平台产品边界、容量锚点和非 Scene 子系统决策；[Kafka 设计](../4.1.Kafka/1.nats-to-kafka-greenfield-design.md)拥有 Raw、Canonical、领域事件等非 Scene Kafka 物理契约。本主规格总纲拥有 Scene 产品边界、wire、Runtime 拓扑、跨组件协议、Action Outbox 事务边界、route/hash 与容量；[Runtime 调度分章](Scene%20Runtime%20调度、分片与故障转移.md#runtime-main-spec-index)拥有 partition ownership、Manager/Actor 执行、completion、restore、rebalance 与故障转移；[《场景有状态计算规范》](场景有状态计算规范.md)唯一拥有 `SceneBindingState`、latest/rolling、五类内建有状态算子、Timer 与有状态求值语义。

任一文档发现涉及同一 Runtime 的跨目录冲突时，必须在同一次变更中显式修订全部受影响的规范性文档，不能依靠权威排序或后写文档自动覆盖。同步完成前，冲突部分不得作为已冻结实施契约。本次联合修订的裁决如下：

| 旧跨目录契约 | 当前 Scene V1 处置 |
| --- | --- |
| `automation.input.live` | 由 `scene_event` 取代；不是改名，key、envelope、事件分类和处理语义均以本文 §5、§14 为准 |
| `automation.state` | 由异步 best-effort `scene_state` 取代；不与 input offset 原子提交 |
| `automation.checkpoint` | 废弃；V1 不维护事务 checkpoint Topic |
| `automation.commit-attestation` | 废弃；动作可靠性由 PG Action Outbox、稳定 `effectId` 和 dispatcher 幂等收敛 |
| `automation.snapshot-manifest` | 废弃；V1 不使用 RocksDB 或远端 snapshot/manifest/CAS/GC |
| `automation.state-migration-progress` | 废弃；V1 固定 partition count，扩容另立迁移设计并重新评审 |
| 独立 `automation.schedule` lane | 废弃；Schedule 进入统一 `scene_event` |
| release artifact/active-release data-plane barrier | 不属于 Scene V1；配置由 PG current Binding 与 `scene_binding_lifecycle` 最终收敛 |
| `affiliationBindingId` / `DeviceAffiliationSnapshot` | 不作为 Scene scope candidate/matching 输入；处理时解析 current `SubjectContext` |
| `AffiliationCutover` offset barrier | 对 Scene affiliation matching 废弃；SubjectLifecycle 与 current-context reconciliation 最终收敛 |
| 迁区前 backlog/redelivery 按旧归属匹配 | 废弃；按处理时已知的 current affiliation 匹配 |
| state、timer、AlarmSignal/ActionIntent 与 Outbox 在同一 PG 本地事务后才推进 offset | 对 Scene V1 废弃；PG Action Outbox 事务与 state/timer 发布顺序由本总纲 §12.3 拥有，producer callback/retry/quarantine 交付终态由 §13.2 拥有；完整 completion predicate、per-partition sequencer 与 revoke offset handoff 由上述 Runtime 调度分章 §§5、8 拥有，不存在 state/Outbox/input offset 跨存储原子切面 |

上述撤销只覆盖 Scene Runtime。Raw → Canonical 的 consume-transform-produce Kafka transaction，以及 protocol/model/ownership 自身确需的 cutover 继续由父架构和 Kafka 设计规范，不能因名称中出现 transaction/barrier 而一并删除。

## 19. POC 与 ADR 冻结门槛

恢复 ADR 在满足下列证据前保持“未冻结”。Zombie、bounded-cut restore、late-send
与多 partition 隔离协议的注入观察点由
[Runtime 调度分章 §11](Scene%20Runtime%20调度、分片与故障转移.md#runtime-observability)
唯一定义；以下 26 项仍是待取证门槛，不是实施成功声明：

1. 一条 Canonical 多点上报只产生一条 SOURCE_DELTA，全部 factDeltas 在一次 ProjectIndex read view 中形成完整 same-subject complete target bundle / cross-subject target-keyed `LINKED_DELTA`；对同一 `(tenantCode,sourceSubjectKey)` 连续输入两条 SOURCE_DELTA，固定延迟首条的 Manager matching/index lookup，断言后一条不得越过，且 matching、SubjectContext/version 观察、same-subject 求值与 cross-subject fanout 都按 source-lane 输入顺序；
2. lookup 故障、索引损坏、bundle 超限时绝不产生部分结果，retry/quarantine 行为可验证；
3. 每节点十万/百万 Binding full cache 内存、冷启动、PG 扫描和编译时间实测；
4. 普通 Binding 原子更新期间 lookup 只见完整旧/新 posting；旧 revision 锁外编译晚完成也不能覆盖新 revision；project gap、future current row、rebuild 与周期对账可恢复；
5. 不同节点短暂使用不同 current config 时，单条 bundle 仍完整且无 activation barrier 依赖；
6. `scene_state` 32 KiB 配额、平均/p99 序列化大小，以及每节点 accepted state count/解码常驻内存、
   partition/tenant skew 实测；在 120K candidate eval/s 和 Producer/PG 慢响应下，测量 tentative
   mutation bytes、allocation bytes/eval 与 bytes/s、Producer in-flight bytes、GC pause 和 heap
   headroom；证明默认路径没有无条件完整深拷贝，并用共享 map/slice/rolling alias fixture 证明失败
   mutation 不污染 accepted state；
7. 120K+ state append/s 与 Kafka RF=3 带宽、Producer buffer/callback age、compaction/Cleaner I/O
   实测；不得用最终 compact 后的 live bytes 代替实时 append/复制容量；
8. 所有 event kind 与 state key 的跨语言 route-key golden vectors 落在同 partition；同一 Runtime 内 `p0`/`p1` 使用独立 bounded-cut restore、completion 与 Actor/timer context，`p0` restore 期间 `p1` 持续推进，且 partition restore RTO 可验证；固定 raw-key latest-outcome fixture 验证 later tombstone/corrupt/disabled/incompatible 遮蔽 earlier valid，坏 raw-key bytes 仍按原始 bytes 选择 latest outcome 并遮蔽 earlier valid，最终 staging 不得 fallback 复活 earlier valid state/timer。另以 `V1 < S0`、`V2 >= S0` 的同 key 更新分别注入扫描前/扫描中 compaction，并使用可控 `delete.retention.ms` 的 tombstone-retention fixture；断言每次 attempt 只 replay 实际观察到的 `< S0` record，`>= S0` 全部排除，完成时捕获 `S1`，`S1 > S0` 必须丢弃 staging 并从新 log-start/新 cut 重试；不得 fallback、虚构未观察 tombstone 或宣称获得严格时间点快照，retention/full-scan RTO 前提或 bounded retry 耗尽时 fail closed 不激活；
9. missing/corrupt state 懒 bootstrap 延迟和数据库冲击；
10. producer failure、pack timeout、commit failure、state send failure故障注入；
11. graceful revoke、`OnPartitionsLost`、missed revoke 与 Zombie 重连故障注入均能观测旧 token 失效，并验证尚未进入 producer/PG 的 timer、continuation、Linked、Outbox 与 state send 被门禁拦截；固定 offset fixture 先发送较低 normal commit 并延迟其 ACK，再触发含较高 frozen frontier 的 graceful revoke；断言 per-partition/generation pipeline 同一时刻最多一个 in-flight，低 commit ACK 前不发 final，未决/失败/超时则 skip final 且 callback 外不重试，不得让较低 normal commit 在较高 final commit 之后落 broker；
12. revoke 前 in-flight state send 迟到并覆盖新 owner state 的概率、错误 transition 窗口与可接受性；固定 fixture 为 `failed reset tombstone -> compatible re-enable -> bootstrap 前 rebalance`，分别测量旧 state/timer 复活、错误 transition 窗口，以及 effectId 不能代替 state 安全；
13. Outbox `effectId` 在旧/新 owner 重叠、rolling 重算时收敛；
14. Product change 旧 state/timer 删除、新 state 当前值 INIT；Project change 两侧独立完整 cleanup/init；Area add/remove member slice；lifecycleEvaluationId 在 retry/rebalance 后保持稳定；
15. Product/Project/Area 改变后的 backlog telemetry 按 current SubjectContext 匹配；延迟 Linked 在更高 aggregateVersion REMOVE 或 current-scope guard 后不能复活旧 state/member；恢复 membership-dependent timer 在首条普通数据到达前 fire 时，已 due timer 进入 activation queue 且 callback 在求值前完成 current-scope reconciliation，失配/查询失败均 zero effect；固定 activation batch/barrier fixture 验证 activation batch/barrier 先于任何 resumed scene_event 与 direct timer work，future timer handle 安装及 activation queue 登记全部完成后才开放 restore gate；
16. scope state 删除不会自动清 active Alarm，也不会取消已 commit Outbox；显式补偿动作可独立验证；
17. SubjectLifecycle 重复、乱序、version gap、cross-subject target delivery failure 和 non-empty area move job 故障注入；对同一 `(tenantCode,sourceSubjectKey)` 连续输入两条 source-side SubjectLifecycle record 并延迟首条 SubjectContext/reconciliation，断言后一条不得越过，version 检查、current context 更新与 same-/cross-subject fanout 全部按 source-lane 输入顺序；
18. Schedule current-config target cap、稳定分页排序/去重、`limit+1` 超限和中间页失败均不发送部分 target；重复 fanout effectId 收敛；
19. production+LIVE 正常产生 production `scene_event`，并可产生 live state/timer/Alarm/Outbox 与 dispatcher delivery；`production + BACKFILL` 与显式 job BACKFILL 均正常 bypass，二者及 REPLAY、shadow、缺失、未知、表外 namespace/mode mismatch、错误 identity version 与来源不可信分别验证 zero production `scene_event`（适用时）、zero live state/timer/Alarm/Outbox mutation、zero dispatcher delivery；
20. 误路由 `scene_event` 与错误 Action Outbox row 均被 Actor/dispatcher 第二门禁拦截；redelivery 始终保留 ProcessingContext、identityAlgorithmVersion 以及 `lifecycleEvaluationId`、timer、schedule、transition/effect identity；Admission/Scheduler/Things SubjectLifecycle/Runtime 使用独立 producer role，正向只能通过各自封闭 publish API 发布允许的 oneof kind + production+LIVE+SCENE_ID_V2。跨 Scene Topic 由 broker ACL 拒绝，role→kind/context/version 由 producer contract 拒绝，consumer 对未知/畸形 kind、payload/key/route 不一致及非法 context/version 二次 fail closed；验收不得伪称 consumer 能从普通 record/header 认证 producer role；
21. invalid/context-less `scene_state` 与 timer slot restore 不得加载、注册 timer、求值或产生 effect；`RESTORING` 仅允许通过 `RestoreMaintenanceGate` 执行坏 key 隔离或纯 tombstone，对非 tombstone state、Linked、Outbox、普通求值与业务 timer 全部拒绝；有效 production+LIVE+SCENE_ID_V2 restore 作为对照验证；Binding lifecycle/explicit reinit 只 close/reset/tombstone/标记 next-live bootstrap，必须取消已有 timer slot/handle，且 zero current-fact read/predicate/FSM/timer create|arm|advance|fire/Alarm/Outbox；
22. ProcessingContext/`SCENE_ID_V2` rollout 的旧 producer input frontier 与 `scene_event` backlog 排空、旧 Action dispatcher lease/外发/status in-flight drain、稳定 pending-set frontier、credential revocation、context-less event 隔离、identity-bearing state tombstone、legacy pending Outbox 原 ID 投递及 state rewrite/compaction/cold-bootstrap/blocked backlog 容量与 RTO 实测；逐类型 cold-start 的重复/延迟效果完成风险签字，首条 V2 record 后的 rollback 只使用 V2-compatible binary；同一 pre-cutover trigger 不得跨算法重算或形成第二个 effect；
23. `BLOCKED_CONTEXT` 不 hot-loop、不阻塞无关 row；严格 target lane 时后序不得越过被阻塞 row；
24. 60 秒 burst backlog 与 catch-up 时间实测；
25. 单租户高 `f/d/lambda/q/c` 时隔离有效；
26. 文档中 SOURCE/Linked/Schedule/SubjectLifecycle/LifecycleTarget/timer 各路径的 physical records、logical deltas、bytes 和 state/action 写放大假设均由观测数据替换。

## 20. 已拒绝或撤销的方案

V1 已接受的进程内 best-effort fencing、幂等托底与残余窗口边界由
[Runtime 调度分章 §9](Scene%20Runtime%20调度、分片与故障转移.md#runtime-best-effort)
唯一规定。下列 durable lease/epoch/EOS 与其他方案仍保持拒绝或撤销：

- Scene evaluator 直接消费 Canonical；
- 把 Canonical 多物理 Topic 绑定为同号 partition bundle；
- 为 SOURCE_DELTA 与 LINKED_DELTA 建不同物理 Topic/Runtime；
- 单实体直连、跨实体另建完全不同 Runtime；
- LinkedDelta 递归生成 LinkedDelta；
- 每个 LinkedDelta 携带完整场景参数；
- 独立 Go CF Runtime；
- Go Rule Chain / RuleGo 节点 Runtime；
- 不同场景共享 CF state；
- `(tenantCode, bindingReleaseId, evaluationSubjectKey)` 版本化 state key；
- immutable BindingRelease/ProjectIndexGeneration artifact；
- 以 PG/Redis 作为每条 SOURCE_DELTA 的远程 hot-path index；
- 每个 Runtime 节点全量缓存设备表或展开 rule × device postings；
- lookup 失败时返回已经查到的部分 bundle；
- Scene RocksDB；
- RocksDB 远端 snapshot；
- snapshot + tail；
- Canonical bounded replay；
- 将 Processing Mode 误写为恢复拒绝理由（RocksDB、EOS、Canonical replay 与 `snapshot + tail` 恢复仍按本节独立拒绝）；
- compacted state 与 snapshot/tail 并存；
- PG 普通 timer；
- 全局 Inbox；
- Kafka offset 作为参数 revision；
- Binding config `effectiveOffset`、per-partition activation barrier 或全局 config snapshot；
- SOURCE_DELTA 携带 expected `configRevision`；
- Schedule immutable affiliation/target snapshot 与可恢复版本游标；
- 通用 immutable affiliation generation + ENTER/STAY/EXIT 迁移协议；
- scope exit 默认自动 clear active Alarm；
- scope exit 扫描并取消已经 commit 的 Action Outbox；
- deterministic Linked Kafka record ID；
- Kafka EOS / 跨 Topic 事务；
- DB owner epoch / 分布式 lease fencing；
- 原地增加 Topic partition。

## 21. 最终结论

V1 的唯一 Binding/matching 模型是：

```text
PG current Binding + transactional lifecycle Outbox
  -> Kafka lifecycle broadcast
  -> 每节点全量 compiled Binding cache
  -> per-project RWMutex 完整旧/新原子可见
  -> gap 时 PG rebuild
```

它不含 release snapshot、config/offset barrier 或远程 partial index。

V1 的唯一数据路由模型是：

```text
Canonical record
  -> 一条批量 SOURCE_DELTA.factDeltas[]
  -> ProjectIndex 完整 matching
  -> 按 evaluationSubjectKey 形成完整 target payload
     -> evaluationSubjectKey == sourceSubjectKey：same-subject complete target bundle（不生成 LINKED_DELTA）
     -> evaluationSubjectKey != sourceSubjectKey：cross-subject target-keyed LINKED_DELTA（Kafka）
  -> target current-scope/member-version guard
  -> target Actor 每 Binding 最多求值一次
```

所有 source/target event 与 state 通过同一冻结 `routeKeyBytes + kafkaMurmur2` 函数共置；这只是固定分区实现，不是配置 activation barrier。

V1 的 Scene state 与恢复业务模型是：

```text
内存工作状态
  + async Kafka compacted full scene_state
  + stable (tenantCode, bindingId, evaluationSubjectKey)
  + missing state lazy business-store bootstrap
  + type-specific cold semantics
```

partition ownership、Manager/Actor 执行、completion、state-before-event bounded-cut restore、
rebalance 与故障转移的唯一结论见
[Runtime 调度分章 §§2–10](Scene%20Runtime%20调度、分片与故障转移.md#runtime-model)；
本总纲只保留 state key、业务有效性、懒 bootstrap 与类型化冷启动语义。

组织关系变化固定采用 TB 式分类处理：Product/Profile 删除旧 state并初始化新 state，Project 执行强隔离 cleanup/init，Area/Relation 增量维护 member slice。state close 取消轻量内存 timer，但不自动 clear active Alarm 或撤销已 commit 输出。

动作可靠性独立为：

```text
PG multi-output Outbox
  + deterministic effectId
  + downstream idempotency
```

高级流计算独立为：

```text
Flink configuration + Flink runtime
```

这不是多套可任选的恢复路线，而是按职责固定的两个 Runtime、一个 current-config matching 模型、一个 Scene state 模型和一个动作投递模型。
