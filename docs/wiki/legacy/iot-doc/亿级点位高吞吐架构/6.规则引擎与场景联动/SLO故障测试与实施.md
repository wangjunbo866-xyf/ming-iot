# 场景联动 SLO、故障测试与实施门槛

> 日期：2026-08-01
>
> 阅读入口：[场景联动与规则引擎 README](README.md) · [场景联动 Runtime 主规格](场景联动%20Runtime%20主规格总纲.md) · [Scene Runtime 调度、分片与故障转移](Scene%20Runtime%20调度、分片与故障转移.md)
>
> **规范性声明**：[场景联动 Runtime 主规格](场景联动%20Runtime%20主规格总纲.md)与 [Scene Runtime 调度分章](Scene%20Runtime%20调度、分片与故障转移.md#runtime-scope)共同构成规范性架构决策源，两者没有“后写覆盖”关系。本文只是从属的 SLO、故障验收与实施门槛，不另立 ADR，也不得改写两份架构源；冲突必须联合修订后才能收敛。
>
> **恢复 ADR 状态**：仍未冻结。冻结审批只采用本文 §10 的 RACI：A 为“指定架构决策人”，必需 C 为容量、SRE、Scene、Things、数据平台、DBA、动作平台；只有主规格 §19 证据全部完成、可复核且该 RACI 完成后，才允许另行记录冻结结论。

> **阅读卡**
>
> - 第一次阅读：先看[《15 分钟读懂场景联动》](00-15分钟读懂场景联动.md)。
> - 本文回答：冻结约束、POC 数值、故障窗口、实施工件、rollout、准入检查和 RACI。
> - 本文不回答：完整数据流、Binding/matching 规则、算子 FSM 或字段级 wire 定义。
> - 快速跳转：[约束分类](#2-约束与数值的三类清单) · [故障矩阵](#6-可审计故障窗口矩阵) · [实施工件](#7-实施与交付工件) · [Rollout](#8-rollout-顺序与-gate) · [RACI](#10-owner--raci)
> - 权威关系：本文是“主规格总纲 + Runtime 调度分章”边界内的从属验收契约；恢复 ADR 仍须完成证据与签字后另行冻结。

## 1. 文档职责与判定语言

本文只回答四类交付问题：

- 哪些正确性和容量边界已经冻结；
- 哪些数值必须通过 POC 决定；
- 故障窗口中允许损失什么、禁止损失什么、如何收敛；
- 实施工件、灰度顺序、准入信号和回滚信号是什么。

本文不复制主规格 §15～§16 的容量模型与完整指标清单。
指标、数据量与业务容量语义以主规格 §15～§16 为准。
统一求值顺序、`SceneBindingState`、latest/rolling、内建有状态算子和 Timer 的业务语义以
[《场景有状态计算规范》](场景有状态计算规范.md)为验收依据。Current Fact writer 的连续
projection frontier 与 terminal no-write 测试以
[《Scene Current Fact 设计》§3](Scene%20current%20fact设计.md#3-写入者与完成谓词)为依据；Runtime
捕获要求水线、执行读取屏障、retry/fail-closed 与 freshness 顺序的测试以
[《水线与读取屏障详解》](水线与读取屏障详解.md)为依据。

per-partition Runtime completion frontier 唯一由
[Runtime 调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)定义。
它是 Scene Runtime 输入调度的 safe contiguous completion frontier，不是 Current Fact writer 的
projection frontier，也不是读取屏障中的要求水线。
partition lifecycle、bounded-cut restore、rebalance、硬故障与 Zombie 的执行语义则以 [Runtime 调度分章 §§6～11](Scene%20Runtime%20调度、分片与故障转移.md#runtime-state-machine)为准。
本文只建立“指标 → 风险 → 验收证据”的最小映射。

本文使用以下判定词：

- **必须 / 禁止**：已冻结的正确性或交付门槛；
- **POC 待定**：必须实测并签字，当前没有产品硬上限；
- **历史起始候选**：仅用于设计实验起点，不是默认配置、SLO 或产品承诺；
- **允许窗口**：主规格明确接受的 best-effort 风险，仍必须观测、注入和量化；
- **收敛**：通过重试、quarantine 重处理、current-store 校验、幂等键，或
  由显式 reinit 完成 control-only close/reset/tombstone 后等待下一条合法
  `production + LIVE` 输入惰性 bootstrap，回到可解释状态。

全文的人工决策术语统一解析到 §10：POC/容量/SLO 数值“签字”对应“POC 容量与候选 SLO 签字”行；最终失败或不可恢复损失“批准”对应“最终失败策略与不可恢复损失接受”行；恢复 ADR “冻结”对应“恢复 ADR 冻结”行。
case closed 只表示满足 §6 终态规则，不替代上述任一审批。

## 2. 约束与数值的三类清单

### 2.1 已冻结硬约束

下表来自主规格 §5.2～§5.4、§6.4～§6.6、§12、§14，以及
[《场景有状态计算规范》§11](场景有状态计算规范.md#11-scene-小状态与-flink-大状态的边界)。
它们不是 POC 建议值，实施不得调松。

| 边界 | 已冻结内容 | 验收判定 |
| --- | --- | --- |
| 单状态大小 | 每个序列化 `SceneBindingState` 最大 **32 KiB** | 超限 state 不替换内存状态、不发送 state、不产生动作 |
| tentative 提交隔离 | accepted state 在 freeze/配额校验以及必要的 Outbox commit 前保持不可变；规范不要求完整深拷贝 | 序列化/配额/PG 失败后，accepted canonical bytes、receipt、FSM、Timer slot/handle 与开始前完全一致；共享 map/slice/rolling 不得产生 alias 污染 |
| rolling 点数 | 每个 rolling argument 最大 **1000 samples** | 超限不裁剪后继续求值；该 BindingState fail closed |
| SOURCE 完整性 | 一个已接纳 source record 的相关事实必须在同一个完整 ProjectIndex read view 中完成 matching | lookup 超时、损坏或不变量失败时，整条 SOURCE retry，耗尽后 quarantine |
| target 完整性 | 同一 source record、同一 evaluation target 的全部命中构成完整 bundle | target bundle 超限或构造失败时，整个 bundle fail closed；不得发送前 N 项 |
| candidate 完整性 | 不允许 partial candidate shedding | 不得按匹配具体度、规则宽窄或局部优先级丢掉部分 candidate |
| target 完整性 | 不允许 partial target shedding | 不得为保住吞吐只投递部分 target，也不得把一个 bundle 拆成可独立求值的残片 |
| 状态超限效果 | state 配额失败是正确性失败 | 超限状态不得产生外部动作，必须告警并要求缩小状态或迁移到适合的 Runtime |
| 分区共置 | event 与 state 使用主规格 §14.1 冻结的 route bytes/hash，partition count 相同 | 跨语言 golden vector 任一不一致即阻止上线 |
| 动作幂等 | Action Outbox 以 `(tenantCode, effectId)` 唯一收敛，executor 按 `effectId` 幂等 | 同一 logical effect 的重复处理不得形成重复外部效果 |
| ProcessingContext 门禁 | 只有受信 `processingNamespace=production && processingMode=LIVE` 可进入生产 interest/matching，并修改 live state/timer/Alarm/Outbox | `BACKFILL` 必须在候选前 bypass；`REPLAY`/shadow/无效组合的 production Scene 写入与外发均为零 |
| 控制面无 state value | `scene_binding_lifecycle`/显式 reinit 只 close/reset/tombstone/标记 next-live bootstrap | 必须 close/cancel 已有 timer handle/slot；控制操作本身 current-fact read、predicate/FSM、timer creation/arm/advance/fire、Alarm、Outbox 均为零 |

“完整”限定一个正确性单元，并不表示无限重试整条 Kafka partition。
有界重试耗尽后可以隔离该完整单元并继续处理后续记录，但隔离内容必须足以重现原输入、失败阶段和错误原因。

### 2.2 POC 待定数值

除上一节两个已冻结数值外，下列具体参数均为 **POC 待定**。
本迁移文档不填造硬上限；配置中的临时实验值也不能自动升级为产品承诺。

| 控制面 | 当前状态 | 冻结前所需证据 |
| --- | --- | --- |
| per-tenant / project / event candidate cap | POC 待定 | 分布、p95/p99/max、超限率、隔离后的 catch-up |
| cross-subject target-keyed `LINKED_DELTA` fanout cap | POC 待定 | `lambda` 分布、bundle bytes、producer callback 时延 |
| SOURCE facts 与 record bytes cap | POC 待定 | 真实多点上报大小分布、序列化和网络成本 |
| Linked/LifecycleTarget entries 与 bytes cap | POC 待定 | target bundle 大小分布、pack timeout 与 quarantine 成本 |
| 每 Binding 的 argument、FSM、timer state、member 数量 | POC 待定 | 序列化大小、CPU、内存和最坏求值耗时 |
| 每节点 accepted state count、decoded bytes 与 partition/tenant skew | POC 待定 | 真实状态分布、解码对象/allocator overhead、恢复后峰值与 failover 倾斜 |
| tentative/编码/Producer 内存与分配带宽 | POC 待定 | worker/PG in-flight 并发、mutation bytes/eval、allocation bytes/eval 与 bytes/s、Producer in-flight bytes、GC pause/CPU 和 heap headroom |
| duration、debounce、cooldown 产品上限 | POC 待定 | active state、内存 callback 数、恢复和清理时延 |
| compiled Binding / posting / 单 project rebuild 上限 | POC 待定 | 十万与百万 Binding 内存、扫描、编译、锁和 rebuild 实测 |
| script CPU、内存、timeout、输出大小 | POC 待定 | 隔离环境下的尾延迟、失败率和 worker 饥饿测试 |
| schedule fire、targets/fire、page size、并发和速率 | POC 待定 | `limit + 1`、中间页失败、重复 fanout 与 DB 压力测试 |
| SubjectLifecycle 速率、单次 operation/fanout、area job 规模 | POC 待定 | 乱序、gap、target loss、current reconciliation 实测 |
| SubjectContext/deletion guard cache 与 fallback 并发 | POC 待定 | miss、淘汰、current-store 负载和恢复时延 |
| Outbox rows/s、pending、due horizon、dispatcher 并发 | POC 待定 | WAL、索引、锁、积压、重试、DLQ 和 drain 实测 |
| 单租户共享容量占比与调度权重 | POC 待定 | noisy-tenant 注入、其他租户尾延迟与吞吐保护 |
| lag、可用性、恢复 RTO 和 catch-up 目标 | POC 待定 | cruise、持续负载、burst、故障和恢复曲线 |
| retry 次数、退避和 quarantine 等待时限 | POC 待定 | 失败窗口、partition 阻塞、数据/动作损失与运维响应；允许提交 offset 另受 §4.1 人工审批工件约束 |

每个数值冻结记录至少包含：负载模型、数据集、集群规格、软件版本、观测区间、原始指标、故障注入、误差范围、容量余量、责任人和签字日期。

`lambda` 统计的是 cross-subject target-keyed `LINKED_DELTA` 的逻辑扇出；即使 source 与 target 最终落在同一 partition 或同一 Runtime，也仍计入 `lambda`。“cross-subject delivery”只表示业务主体边界，不表示物理远程网络调用。

### 2.3 醒目警告：历史起始候选

> **警告：下列数字只来自旧文，是历史实验起点。它们不是产品承诺，不是默认告警阈值，也不是当前已冻结 SLO。必须由 POC 与容量签字结果重新填写；即使实测后数值恰好相同，也必须以新的证据和签字替换其历史身份。**

| 历史候选 | 旧文含义 | 当前处理 |
| --- | --- | --- |
| cruise lag **5 min** / 声明峰值窗口 **30 min** | 旧 `scene.eval.maxLag` 目标 | 仅作为 lag 实验起点；由 backlog 年龄、catch-up headroom 和业务容忍度共同签字 |
| 单租户共享池 **25%** | 旧文给出的共享池占比示例 | 仅作为 noisy-tenant 实验起点；每类 worker/Kafka/PostgreSQL 资源分别实测，不得直接复用 |

POC 结果可以低于、高于或取消这些候选。
未完成签字前，dashboard 可以展示实测分布，但不能把上述数字标成“平台保证”。

## 3. overload 与 fairness 的正确性边界

### 3.1 按边界处理超限

| 发生边界 | 必须行为 | 禁止行为 |
| --- | --- | --- |
| 发布/启用 | 最坏 fanout、state 或资源需求可证明超过已签字预算时拒绝发布；错误必须指向超限维度 | 带着已知超限配置进入运行时，再依赖局部丢弃维持吞吐 |
| SOURCE admission | facts 或 bytes 超限时整条 source record fail closed，先重试，耗尽后 quarantine | 截取前 N 个 facts，或拆包后让未到齐分片参与求值 |
| ProjectIndex lookup | not-ready、timeout、revision gap、损坏或内部不变量失败时不产生结果 | 返回已查到的部分 binding/argument，或临时在线拼出不完整结果 |
| target bundle | entries、binding、argument 或 bytes 超限时完整 bundle fail closed；先有界 retry，耗尽后 quarantine | 按匹配具体度丢规则、返回前 N 个 candidate，或把残片当成完整 bundle |
| state 序列化 | 完整 next state 先校验；失败时不推进 state、timer 或 Outbox | 裁剪 rolling/member/FSM 后继续产生动作 |
| Schedule fanout | 先完成有界 target list；查询失败或 `limit + 1` 证明超限时拒绝整个 fire | 发送已经读到的前几页或前 N 个 target |
| 共享资源 admission | 在 candidate worker、producer、Outbox 写入或 dispatcher 入口前按租户限流、排队、暂停或隔离 | 进入共享池后才随机牺牲单个 binding、argument、target 或动作 |

发布期拒绝和运行时 quarantine 都必须带 tenant、project、Binding/record identity、命中的配额维度与当前观测值。
配额拒绝不能只留下聚合计数，否则无法证明是否误伤了合法 bundle。

### 3.2 fairness 候选实施机制

资源隔离至少实现 per-tenant admission，并从以下候选中经 POC 选择组合：

- 在共享 worker/producer/dispatcher 前做 tenant budget 和有界 queue；
- 使用 weighted fair scheduling，并把 credit 消耗、补充和饥饿保护做成可观测状态；
- 为已知大租户配置独立 pool 或独立 shard，其余租户继续受 admission 约束；
- 对不同资源分别定参，不能用一个百分比代替 CPU、Kafka、数据库和 dispatcher 的独立容量模型。

最终权重、队列、速率、租户份额和独立 pool 触发条件全部由 POC 决定。
单租户的任何局部降级都不能破坏 bundle 正确性；资源不足时宁可拒绝发布、暂停完整输入或隔离完整正确性单元。

## 4. 外部效果承诺边界

主规格 §12～§13 拥有动作与 Kafka 物理交付承诺，[Runtime 调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)与[§9](Scene%20Runtime%20调度、分片与故障转移.md#runtime-best-effort)分别拥有 per-partition completion 与 owner 重叠残余窗口。两者共同将“动作可靠性”和“计算 state 恢复”明确分开。

| 窗口 | 对外承诺 | 允许风险 | 必须保护的正确性 |
| --- | --- | --- | --- |
| Outbox commit 前 | 还没有接受该动作 | 重试耗尽且执行 §4.1 已批准最终失败策略时，动作可能丢失 | tentative state、timer 和对应 state 均不得提前发布 |
| Outbox commit 后 | Action Outbox 负责可靠至少一次投递 | dispatcher 或下游可暂时失败、重复尝试 | executor 必须按 `effectId` 幂等；已 commit 动作不因 scope exit 自动撤销 |
| 输入 offset commit 失败 | 新 owner 可以重放同一 logical trigger | 重算、重复 Outbox insert、重复投递尝试 | `transitionId/effectId` 稳定，唯一键与下游幂等收敛重复 |
| async `scene_state` send 失败 | 完整 state 只做一次有界 producer handoff attempt；立即拒绝或后续 callback 失败都只观测，允许 state 落后或缺失 | 后续恢复可能冷启动、近似重建或等待新输入 | state record 始终是完整状态；send loss 不能成为 partial bundle 的理由，也不能把 producer 接受/ACK/callback 追加成 input completion 条件 |
| 旧 owner in-flight state 迟到 | 属于必须量化的 V1 best-effort 风险 | compacted value 可能暂时回退并在后续恢复诱发错误 transition | 未进入 producer 的旧工作必须被 generation guard 拦截；风险必须有 POC 预算 |

因此：

- 系统不是金融级 exactly-once，也不承诺 Scene state 零丢失；
- 上述 best-effort 窗口不放宽 ProcessingContext 门禁；非受信
  `production + LIVE` 不得产生生产 state/timer/Alarm/Outbox 或外部效果；
- Scene V1 不实现隔离 replay evaluator；`REPLAY` 的验收是不进入生产
  Scene，不是恢复 RocksDB、Kafka EOS 或新 replay state/outbox Runtime；
- `scene_state` 的 async loss tolerance 不能放松 matching、source bundle、target bundle 或 state size 的 fail-closed 约束；
- Outbox commit 后的“可靠至少一次”依赖 Outbox 行保留、dispatcher 重试/DLQ 和下游 `effectId` 幂等共同成立；
- rollback 不能删除或静默取消已经 commit 的 Action Outbox，必要补偿必须形成新的显式动作。

### 4.1 最终失败策略与损失风险接受工件

当 retry/quarantine 无法收敛、团队准备以最终失败策略允许提交输入 offset 时，必须先存在一份正式、版本化、尚未到期且范围精确匹配的“最终失败策略/损失风险接受”工件。工件至少记录：

- 适用的 tenant、project、事件类型与失败类型；禁止用全平台兜底条款覆盖未知故障；
- 允许提交 offset 的前置条件、触发条件和最长等待时限；
- 明确可接受的数据损失、动作损失或 target 未收敛范围，以及明确不可接受的结果；
- 告警对象、审计证据、影响清单、补救/补偿方案和执行 owner；
- 生效时间、到期时间、到期复审、撤销条件和历史批准记录；
- 指定生产风险接受人的实名审批，以及业务/产品 owner、数据 owner、SRE、Scene、动作平台和架构按风险完成会签。

Runtime 只能报告条件是否匹配并执行已经批准的决定，不能自行创建、批准、扩大或续期该策略。
没有工件、工件过期、范围不匹配或缺少指定生产风险接受人审批时，不得凭超时自动提交 offset，case 必须保持 open。
即使 offset 获准提交，也不自动代表故障 case 已关闭；关闭条件见 §6。

## 5. SLO 与验收证据映射

### 5.1 三类目标必须分开

| 类型 | 回答的问题 | 冻结方式 |
| --- | --- | --- |
| 延迟 SLO | 一个有效输入、恢复或已接受动作多久完成 | 依据业务时限、分位数、burst/catch-up 和 POC 容量签字 |
| 可用性 SLO | 一段窗口内有多少有效工作不因依赖或资源失败而停滞 | 依据成功、retry、quarantine、reconcile 和 drain 比例签字 |
| 完整性目标 | 结果是否完整、是否越 scope、是否产生重复外部效果 | 架构不变量要求零违规；不能拿延迟/可用性预算交换 |

§2.3 的两个历史数值只能保留在醒目标注的候选表中。
任何新 SLO 数值先以“候选”进入 POC 报告，容量签字后才能进入产品配置和告警规则。

### 5.2 指标 → 风险 → 证据

指标原始定义见[主规格 §16](场景联动%20Runtime%20主规格总纲.md)。
下表只挑选能关闭关键风险的组合，不替代主规格指标全集。

| 类型 | 指标/派生信号 | 主要风险 | 必须留下的验收证据 |
| --- | --- | --- | --- |
| 延迟 | consumer lag age、source backlog、catch-up headroom/time | burst 后长期追不上，current context 与业务响应变旧 | 主规格 §19 第 24、26 项；cruise/持续/burst 曲线与恢复时间 |
| 延迟 | ProjectIndex cold-start、rebuild duration、read-lock p99 | 配置未 ready 或 rebuild 阻塞数据处理 | §19 第 3、4 项；十万/百万 Binding 报告和 gap 注入时间线 |
| 延迟 | cold bootstrap latency 与 current-store load latency | missing state 导致数据库雪崩或求值超时 | §19 第 9 项；并发冷启动、NOT_READY 与 DB 冲击报告 |
| 延迟 | Outbox due lag、dispatcher drain rate | 已接受动作长期未送达 | 主规格 §12 + 本文 F-17 扩展用例；dispatcher 停机/恢复与积压排空曲线，§19 未单列该 SLO |
| 可用性 | source retry/quarantine rate、cache-not-ready fail-closed | 依赖故障扩大为 partition 长期停滞 | §19 第 2、4、10 项；注入前后成功/隔离/恢复比例 |
| 可用性 | lifecycle target failure、`reconcileRequired` age、重处理次数 | current source 已变化但 cross-subject target 长期未收敛 | §19 第 17、21 项；失败 target 列表、重处理、control-only reinit 及 next-live bootstrap 结果 |
| 可用性 | per-partition status/token 变更、固定 `[start,S0)`、scan progress/duration | 新 owner 无法及时开放，或 `p0` restore 误阻塞 `p1` | §19 第 8、11、12 项；分 partition 的 state-before-event、callback 因果线与接管报告 |
| 可用性 | Outbox pending/retry/DLQ、terminal update failure | commit 后动作可靠至少一次链路断裂 | 主规格 §12 + 本文 F-17 扩展用例；dispatcher 重试、DLQ、drain 和终态审计，§19 未单列该证据 |
| 完整性 | partial SOURCE/target bundle violation count | 漏 binding/argument 或重复求值 | 必须为零；§19 第 1、2、5 项的故障注入与 bundle trace |
| 完整性 | route partition mismatch、golden-vector failure | event 与 state 不共置，串行与恢复假设失效 | 必须为零；§19 第 8 项的 Go/Java 向量和实际 topic 抽检 |
| 完整性 | state size/max、overflow action count | 超限 state 被裁剪后产生错误动作 | overflow action 必须为零；§19 第 6、7 项 |
| 容量/完整性 | accepted decoded bytes、StateTxn in-flight/mutation bytes、allocation bytes/s、Producer in-flight bytes、GC pause/headroom | 无条件深拷贝或慢 Producer/PG 放大临时对象，导致 GC 抖动、OOM，或失败 mutation 污染 accepted state | 主规格 §15.5、§19 第 6～7 项；容量曲线、heap/GC profile，以及本文 F-16 的 canonical-before/after 与 alias 断言 |
| 完整性 | stale Linked reject、current-scope fallback、旧 member 复活 | REMOVE 后迟到 Linked 恢复旧 scope | 复活必须为零；§19 第 15 项与 current-store 校验 trace |
| 完整性 | stale timer no-op、close/cancel latency、callback side effect | state close 后旧 callback 产生新动作 | stale callback side effect 必须为零；§19 第 11、14 项 |
| 完整性 | Outbox unique conflict、downstream dedup、duplicate external effect | 重放或 owner 重叠产生重复外部效果 | duplicate external effect 必须为零；§19 第 13 项 |
| 完整性 | per-partition completion gap、最早未完成 next-offset、safe contiguous frontier | 后完成 offset 越过前置 gap，或 revoke 夹带其他 partition position | 分 partition frontier 只在完整 completion 谓词后连续推进；§19 第 10、11 项与 Runtime 调度分章 §5、§8 |
| 完整性 | effect-gate rejection by `effect_kind`/`reason`、staging discard、activation queue depth/age | 旧 token 或 `RESTORING` 普通工作穿透门禁，或 due timer 在 activation 中丢失 | `effect_kind` 与 `reason` 必须是 bounded enum；非允许 maintenance 的 restore 效果必须为零，staging/queue 丢弃只限失权 partition；§19 第 11、15、21 项 |
| 完整性 | schedule page failure、over-limit reject、partial target send | 部分页产生动作，形成不可解释人群 | partial target send 必须为零；§19 第 18 项 |
| 完整性 | `scene_admission_mode_total{namespace_class,mode,decision}`、`scene_processing_context_invalid_total{reason}` | BACKFILL/REPLAY/shadow/无效组合穿透 Admission 或被默认 LIVE | 对应 decision/reason 有界计数与 quarantine 审计；§19 第 19、20 项的 production mutation/delivery 必须为零 |
| 完整性 | `scene_state_processing_context_invalid_total{reason}` | context-less/冲突 state 或 timer slot 被加载为 live | 恢复、timer 注册、求值与 effect 必须为零；§19 第 21、22 项 |
| 完整性 | `scene_dispatcher_processing_context_invalid_total{reason}`、`scene_outbox_blocked_context_total{reason}` | 非法 pending Outbox 被重试或外发 | row 事务性进入 `BLOCKED_CONTEXT`，无 hot-loop/外发；严格 target lane 不得越过；§19 第 20、23 项 |
| 风险预算 | late in-flight state callback 与 stale overwrite observation | 旧 owner state 覆盖新 owner state | §19 第 12 项；概率、窗口和错误 transition 量化；终态损失按 §4.1 批准，ADR 可接受性按 §10 决策 |

上述 ProcessingContext 指标的 `namespace_class`、`mode`、`decision` 与
`reason` 必须是 bounded enum/class。`jobId`、`replayId`、`effectId` 等高基数
标识只进入受控日志、trace 或审计字段，禁止成为 metric label。
Runtime lifecycle/effect-gate 指标的 `effect_kind` 与 `reason` 也必须是 bounded enum；
raw token、state key 与 `effectId` 只进入受控日志/trace/audit，禁止成为 metric label。

### 5.3 证据包最低内容

每次 POC 或故障演练必须形成不可只靠 dashboard 截图解释的证据包：

- case ID、目标主规格章节与 §19 条目；
- 软件版本、配置摘要、集群与数据集说明；
- 注入时刻、注入点、持续时间和清理方式；
- source/lifecycle/schedule/timer 的稳定业务 identity；
- 原始 `processingNamespace/processingMode`、受信 producer identity 与每层 mode
  decision；redelivery 用例还必须证明 context 未改写；
- Kafka key、partition、offset/callback 结果与 consumer generation；
- 每个 TopicPartition 的 context/status/token 变更、lost/suspend 先于 rejoin/assigned 的 callback 顺序，以及其他 `ACTIVE` partition 的独立进展；
- restore 的固定 `[start,S0)`、scan frontier、staging discard、activation queue 与 timer 回 lane 次序；
- completion gap、safe contiguous frontier、revoke final-commit/no-commit 决定与 partition 范围；
- ProjectIndex revision、state key、Outbox `effectId` 或 quarantine identity；
- 涉及 state/Outbox 时的 Kafka ACL/PG DB role 审计、Outbox terminal status 及
  `BLOCKED_CONTEXT` reason/quarantine reference；
- 禁止结果断言、预期收敛断言和实际完成时间；
- 分开记录旧 owner 迟到 `scene_state` 的 stale-overwrite/compacted-latest 证据与同一 logical effect 的 `effectId` 重复收敛证据，不得用后者代替前者；
- 原始指标查询、结构化日志、trace 与必要的数据库审计结果；
- 涉及 state 容量时的 accepted state count/decoded bytes、partition/tenant skew、StateTxn/Producer
  in-flight、allocation profile、heap live/headroom 与 GC pause/CPU 原始样本；
- 未收敛项、修复工单、复测链接，以及涉及终态损失时 §4.1 指定生产风险接受人的批准记录。

## 6. 可审计故障窗口矩阵

矩阵的“禁止结果”是硬断言；“预期收敛”可以依赖有界重试、quarantine 重处理或显式 reinit，但必须在证据包中看到完整因果链。
POC 编号均指[主规格 §19](场景联动%20Runtime%20主规格总纲.md)。

所有 case 首先必须通过禁止结果断言。对正常或确定性路径（如正常
production+LIVE admitted、BACKFILL bypass、REPLAY/shadow isolation），以及故意构造
的非法安全 fixture 按契约到达 ACL reject/quarantine/`BLOCKED_CONTEXT` 且没有污染
生产状态的路径，只有实际到达定义终态，且完整 propagation/zero-effect/副作用断言与
fixture 清理都有证据时，才可 closed；它们不需要虚构 reinit 或风险接受。

对产生污染、状态偏差、未完成效果或真实业务损失的故障 case，还必须满足以下任一终态才能标记 closed：

- quarantine 重处理成功，且 current state 与已产生/未产生的外部效果完成校验；
- control-only 显式 reinit 完成且下一条合法 `production + LIVE` trigger
  已惰性 bootstrap，或补偿完成；两者都必须完成 current state 与外部效果校验；
- §4.1 的指定生产风险接受人针对该 case 正式接受不可恢复终态损失，并把批准记录、影响清单和后续复审绑定到 case。

对故障/污染 case，仅“写入 quarantine”“提交 offset”“发出告警”或“指定 owner”都不是关闭条件。
不满足上述终态的 case 必须保持 open，并阻止进入 canary 或扩大现有 canary 范围。

### 6.1 ProcessingContext 与持久化污染

| ID | 输入/注入窗口 | 禁止结果 | 预期收敛与审计重点 | 规范门槛 |
| --- | --- | --- | --- | --- |
| M-01 | 受信 `production + LIVE` Canonical | 绕过正常 interest/matching/完整 bundle 管线 | 正常 admitted；证明公共 ProcessingContext 端到端传播，并对可能的 `scene_event`/state/timer/Alarm/Outbox/delivery 执行完整路径断言 | §3.4、§5、§19；POC 19、20 |
| M-02 | `production + BACKFILL` 或 `backfill/<jobId> + BACKFILL` | 将合法在线补传误判为 mismatch/quarantine，或触发 interest/`TriggerPosting` 候选、产生逐规则 skip/生产 Scene 记录 | 两种合法 BACKFILL 都在候选前 deterministic bypass 并正常推进该输入前沿；production `scene_event`/state/timer/Alarm/Outbox/delivery 全部为零 | §3.4、§4；POC 19 |
| M-03 | `replay/<id> + REPLAY` | 进入生产 Scene 求值或把结果晋升到 production | 仅保留在 Raw/Canonical replay namespace；production `scene_event/state/outbox` 写入与 delivery 均为零 | §3.4、§9.3；POC 19 |
| M-04 | `shadow/<id> + LIVE` | 以 `LIVE` 枚举值绕过 namespace 隔离 | 不进入 production matching；production `scene_event/state/outbox` 写入与 delivery 均为零 | §3.4；POC 19 |
| M-05 | ProcessingContext 缺失、未知、表外 namespace/mode mismatch 或来源不可信 | 被错误视作 `LIVE`；触发 interest、lookup 或求值 | 在 interest 前 fail closed/quarantine/告警；使用 bounded reason 指标；全部 production mutation/delivery 为零 | §3.4、§5.1；POC 19 |
| M-06 | 非法或非 production+LIVE `SceneEvent` 错投生产 Topic | Actor 依赖 Topic 名盲信，构造 arguments、bootstrap state 或注册 timer | 入口 handler 在 lookup/evaluation 前二次 fail closed/quarantine；state/timer/Alarm/Outbox 变更与 delivery 为零 | §5.1、§5.4；POC 20 |
| M-07 | context-less/未知/冲突 `scene_state`，或缺失/非 `SCENE_ID_V2` timer slot 污染 restore | 加载为 live state、注册/取消 timer、求值或产生 effect | restore Actor fail closed/quarantine，不打开被污染 lane 值；受信 production+LIVE+SCENE_ID_V2 restore 作为对照 | §8、§9、§10；POC 21 |
| M-08 | 非法 ProcessingContext/identity version 的 pending Action Outbox row | dispatcher 重试、外发或原地改 context/version 后恢复 pending | 以 CAS/本地事务转 `BLOCKED_CONTEXT`，从 due index 移除，记录 bounded reason/audit/quarantine reference；仅 §9.4 审计迁移的 `LEGACY_NO_NAMESPACE_V1` row 可按原 `effectId` 只读投递；严格 target lane 暂停后续并 page | §12.1；POC 20、23 |
| M-09 | 非 state-writer principal 写生产 `scene_state`，或非 Actor role INSERT/改写 Outbox context | 越权写入被业务校验掩盖 | Kafka ACL 与 PG DB role 直接拒绝，并留下 bounded producer/build/contract-version 审计 | §8.2、§12.1；POC 20、22 |
| M-10 | `scene_binding_lifecycle` 或显式 reinit | 立即读 current facts、初始化 business state、predicate/FSM 求值、创建/arm/推进/fire timer、产生 Alarm/Outbox，或未取消已有 timer | 只 close/reset/tombstone/标记 next-live bootstrap 并取消已有 timer slot/handle；下一条受信 production+LIVE `SceneEvent` 才可惰性 bootstrap | §4、§9.3；POC 21 |
| M-11 | 已验证 production+LIVE 的 membership-dependent timer 恢复后，在首条普通数据到达前 due | callback 按旧 scope 求值或产生 effect；为等遥测而丢失 due | timer fire 作为第一个合格的数据面 evaluation trigger，先查 current `SubjectContext` 并 reconciliation；它是进程内 callback 回 lane，不写 `scene_event`；失配则 close+tombstone/cancel，lookup 失败则 retry/fail closed，均 zero effect | §9.2、§10.1；POC 15、21 |
| M-12 | Admission/Scheduler/Things SubjectLifecycle/Runtime producer 尝试通过公开 API 跨角色发布、发送错 oneof kind/非 production+LIVE，或写未授权 Scene Topic | 共用过宽 principal/任意 envelope API；错误声称 Kafka consumer 能从普通 record/header 认证 producer role | 每个 producer 使用独立最小权限 principal 与封闭 publish API；Kafka ACL 拒绝跨 Topic，producer contract 拒绝 role→kind/context/version；Runtime 对未知/畸形 kind、payload/key/route 不一致或非法 context/version 二次拒绝，但不虚构 shared-topic per-record principal 证明 | §5.1、§14；POC 20 |
| M-13 | `SCENE_ID_V2` cutover 前仍有旧 Admission/Scheduler/SubjectLifecycle/Runtime writer、context-less `scene_event` backlog/in-flight、旧 Action dispatcher in-flight、旧 timer identity 或同一 trigger 跨版本 redelivery | 缺失 context 被默认 LIVE；迁移 pending set 时旧 dispatcher 继续外发；旧/新公式生成两个 `effectId`；timer current-slot 失配；V2 record 后恢复 legacy writer | 先 pause/fence/撤销旧 producer、排空输入前沿与 `scene_event`/callback/DB/Kafka in-flight，再 pause/fence dispatcher并排空 lease/外发/status/PG in-flight、冻结 pending set；无法排空 event 只隔离并经损失审批；identity-bearing state tombstone+cold start 并对重复/延迟效果签字；可信 legacy pending Outbox 保留原 `effectId` 只投递；证明 zero cross-algorithm duplicate，首条 V2 record 后只允许 V2-compatible rollback | §5.1、§9.4、§12；POC 22 |

M-03 不验收 Scene replay 计算结果；Scene V1 没有隔离 replay evaluator。
未来支持必须单独 ADR 并提供独立 input/state/outbox 与禁外发 dispatcher，
不得通过本测试矩阵偷渡新 Runtime 或 Topic。

### 6.2 ProjectIndex、matching 与 bundle

| ID | 注入窗口 | 禁止结果 | 预期收敛与审计重点 | 规范门槛 |
| --- | --- | --- | --- | --- |
| F-01 | 节点启动时 ProjectIndex not-ready | data consumer 提前开放；按空/部分 index 匹配 | 所有 index ready 后才加入 data group；无可验证 index 的 project retry/fail closed | §6.4；POC 3、4 |
| F-02 | lifecycle revision gap | 跳过缺口继续增量安装 posting | 保留旧完整 index，单 project rebuild，随后按连续 revision 应用缓存事件 | §6.3～§6.4；POC 4 |
| F-03 | rebuild 与普通 update 并发 | 读者看到旧 posting 已删而新 posting 未齐 | staging 完整校验后 pointer swap；起点 revision 变化时丢弃旧 staging 重试 | §6.3；POC 4 |
| F-04 | PG current row 已是 future row | 用未来 row 冒充当前 lifecycle revision | 放弃增量产物并 rebuild；审计 future-row rebuild 和 rollback prevented | §6.1～§6.3；POC 4 |
| F-05 | SOURCE matching timeout、partial result 或 invariant failure | 返回已匹配的 binding/arg；发送部分 local/Linked bundle | 整条 SOURCE 有界 retry，耗尽后保存完整原 record 到 quarantine | §6.5；POC 1、2、5 |
| F-06 | source record 或 target bundle 超限 | 返回前 N 个 candidate/target；按匹配具体度丢部分规则 | source 以整条 record、target 以完整 bundle 有界 retry；耗尽后 quarantine；无动作 | §5.2～§5.3；POC 2 |
| F-07 | 编译普通 `A AND B`、显式 `WHEN A IF B` 与 rolling/Edge/Duration/Debounce/Hysteresis 组合；分别注入 A、B 独立消息、同 bundle 多命中和强制依赖被标为 `CONTEXT_ONLY` | 只给普通条件的第一个字段 trigger；每个 Trigger 各发一次动作；按到达时间合并消息；用 `effectId` 猜测不同 Trigger 是一次动作；遗漏状态输入 posting；Cooldown 额外制造数据 trigger | `A AND B` 的 A、B 都编译为精确 `TRIGGER`，`WHEN A IF B` 只有 A 为 Trigger；同 bundle 最多求值一次，独立 bundle 在 active owner/lane 串行共享完整谓词 `Edge(RISING)`，`false -> true` 恰好一个 action set、后续 `true -> true` 为 zero Outbox；有状态算子依赖闭包完整，冲突配置发布期拒绝；crash/rebalance 恢复残余只按 F-14 量化，不把本用例宣称成严格持久化去重 | [控制面契约 §4.2](控制面与匹配契约.md#42-trigger-与-context_only)；[有状态规范 §§4.1.1～4.1.2](场景有状态计算规范.md#stateful-trigger-dependency-closure) |

### 6.3 Linked、lifecycle 与 state

| ID | 注入窗口 | 禁止结果 | 预期收敛与审计重点 | 规范门槛 |
| --- | --- | --- | --- | --- |
| F-07 | Linked producer send loss | 把 send callback 失败记成成功且无审计 | 有界 retry；耗尽后完整失败 target 进入 quarantine；只有 §4.1 人工批准才允许按最终策略提交源 offset，并显式记录 loss window | 主规格 §5.3、§13.1～§13.2（payload、物理交付与失败策略摘要）；[调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)（completion）；POC 10 |
| F-08 | 更高版本 REMOVE 后晚到 Linked | 旧 Linked 重新激活 member/state 或产生动作 | member version guard 拒绝旧版本；guard 不足时查 current source/target scope，验证前不得 bootstrap | §5.4、§8.1；POC 15 |
| F-09 | Linked 到达时 current scope 已不适用 | 只凭 payload 中旧关系继续求值 | disabled/out-of-scope/stale entry 幂等 no-op；context 查询失败时整个 bundle retry/quarantine | §5.4；POC 15 |
| F-10 | SubjectLifecycle cross-subject target delivery loss | source 继续按旧 context matching；失败 target 永久不可定位 | source 更新 current context 并标 `reconcileRequired`；保存 operation/target 并重处理；显式 reinit 只可 close/reset/tombstone/标记 next-live bootstrap | §5.5；POC 17、21 |
| F-11 | 同一 `(tenantCode,sourceSubjectKey)` 的 SubjectLifecycle 重复/乱序，并固定延迟首条 source-side record 的 SubjectContext/reconciliation，使后一条先就绪 | 重复 ADD/REMOVE；旧 operation 覆盖新 member version；后一条越过阻塞的首条，提前更新 context/version 或发出 same-/cross-subject fanout | 两条 record 先进同一 source-key 串行 lane；首条阻塞时后一条不得越过，SubjectContext、version 检查与 fanout 均按输入顺序；同版本幂等、旧版本拒绝；稳定 lifecycle identity 在 retry/rebalance 后不变；不同 subject-key lane 可并发的对照组仍前进 | 主规格 §5.5、§8.1（业务 matching/version）；[Runtime 调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)（source lane 顺序）；POC 14、17 |
| F-12 | SubjectLifecycle aggregateVersion gap | 猜测缺失中间关系或构造历史关系快照 | 以本地已接受 context 为 old、Things current context 为 new 直接 reconciliation | §5.5；POC 17 |
| F-13 | `scene_state` handoff 立即被 producer 拒绝，或被接受后在 input offset 提交前/后 callback 报 send loss | 阻塞 input offset 等 producer 接受/ACK；写入半状态；把 ACK 当动作承诺或声称 state durable | 两种失败都只记录/告警，offset 可继续；后续完整 state 或合法 production+LIVE trigger 惰性冷启动收敛；显式 reinit 本身只做 control-only state management | 主规格 §8.2、§9.3（`scene_state` 物理 handoff 与业务收敛）；[调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)（completion）；POC 10、21 |
| F-14 | 两个独立 stale-state fixture：A）旧 owner 的 in-flight full-state late append 晚于 revoke；B）disable/scope-exit/explicit reinit 的 async reset tombstone 交付失败 → 同 key compatible re-enable/重新适用 → 成功 next-live bootstrap 或新完整 state 覆盖前 rebalance | 声称 generation guard 能撤回已 in-flight append；把 reset/next-live bootstrap 的进程内标记、current `semanticHash` 或 `effectId` 当作 reset 已持久的保证；把 stale state overwrite 等同于已被幂等吸收 | 两个 fixture 分开量化 arrival order/compacted-latest 窗口，验收旧 state 与 `ARMED` timer 是否复活、是否产生错误 transition；B 还必须留存 tombstone 失败审计、验证 control-only 显式 reinit 补发，未收敛时按已量化窗口完成生产风险接受 | [调度分章 §9](Scene%20Runtime%20调度、分片与故障转移.md#runtime-best-effort)；主规格 §9.3、§19 第 12 项 |
| F-15 | state missing 或单 key corrupt | 打不开整个 partition；历史补算外部动作；缺参仍求值 | 隔离坏 key，按 current Binding/latest store 懒 bootstrap；NOT_READY 不产生动作 | §9.2～§9.4；POC 9 |

### 6.4 Outbox、timer 与 rebalance

| ID | 注入窗口 | 禁止结果 | 预期收敛与审计重点 | 规范门槛 |
| --- | --- | --- | --- | --- |
| F-16 | tentative 已修改 rolling/map/slice、receipt、FSM/Timer 后，分别注入 freeze/序列化失败、配额失败与 Outbox pre-commit/rollback | mutation 通过共享引用污染 accepted state；发布 tentative state、注册/取消 timer、发送 next state 或声称动作已接受 | 注入前后 accepted state 的 canonical bytes/hash、receipt、FSM、Timer slot 与进程内 handle 完全一致；丢弃 mutation view 后整条 transition 从同一 stable identity 重试，结果确定；耗尽后 quarantine，只有隔离成功或 §4.1 人工批准最终策略后才提交输入 offset | 主规格 §12.3、§15.5；[有状态计算规范 §4.2](场景有状态计算规范.md#42-tentative-state提交隔离不是完整深拷贝)；POC 6、10 |
| F-17 | Outbox commit 后 dispatcher failure | 删除已接受动作、因 scope exit 取消，或生成新随机 effect identity | 行保持 pending/retry/DLQ，恢复 dispatcher 后至少一次投递；executor 以 `effectId` 幂等 | 主规格 §12 + 本文 F-17 扩展用例；§19 未单列 dispatcher 验收 |
| F-18 | state close 后 timer stale callback | callback 复活 state 或写新 Outbox | 检查 Binding、current timer slot 和 callback 捕获的 partition token；slot 不存在或 lane/token 已失效时稳定 no-op | [有状态计算规范 §8](场景有状态计算规范.md#8-timer运行机制)（Timer/current-slot）；主规格 §11.3（Binding lifecycle）；[调度分章 §§5～7](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)（callback token/lane ownership）；POC 11、14 |
| F-19 | 分别注入四个时间边界：①较低 normal commit 已发送但 ACK 延迟时触发带较高 frozen frontier 的 graceful revoke；②普通 graceful revoke；③本地已观察 `OnPartitionsLost`/session uncertain；④ truly missed-revoke/尚未观察丢权的 Zombie，随后 rejoin/assigned | 低 normal commit 与高 final commit 同时 in-flight，或低 commit 在高 final 之后才落 broker；未等低 commit 终态就发 final；先 drain 后失效 token；final commit 提交高于本次冻结的 frontier、夹带未完成 offset/其他 partition、低于 committed floor 或在 callback 外重试；cleanup 推进 offset；lost 后 commit 或产生新效果；声称 missed-revoke 在本地检测前已 zero effect；在 rejoin/assigned 时仍让 missed-revoke 的旧 generation commit 或复用 context | ① per-partition/generation `offsetCommitPipeline` 对 normal/final 严格串行且 single-flight，committed floor 只在 ACK 后推进；revoke 先停止 normal admission 并冻结 frontier，释放 lock 后在 callback budget 内等待较低 normal commit 终态；只有它 ACK 且无旧 commit in-flight 才发至多一次 final，未决/失败/超时则 skip final、接受 replay 且 callback 外不重试。② graceful revoke 必须 token-first 并 pause；在 lifecycle domain 中只冻结该 partition 的 safe contiguous frontier、last-known committed floor 与 context/generation commit admission；管线允许时到 `OnPartitionsRevoked` 返回前至多一次 bounded final commit；提交值只能是本次冻结的 safe contiguous frontier 且不低于 ACK 后 committed floor，无 eligible frontier 则不提交；失败/超时后 callback 外不得重试，cleanup 不得再推进 offset。③已观察 `OnPartitionsLost`/session uncertain 时先 invalidate token 再进入 `SUSPENDED`，绝不 commit；之后新效果在每个边界均被拒绝。④ truly missed-revoke/尚未观察丢权时不承诺检测前 zero effect，必须量化 observation gap 并关联 F-14 stale state 与 F-20 `effectId` overlap；在 rejoin/assigned 前失效全部旧 token，旧 generation 不得 commit 或复用 context，此后门禁才生效 | [调度分章 §6](Scene%20Runtime%20调度、分片与故障转移.md#runtime-state-machine)、[§8](Scene%20Runtime%20调度、分片与故障转移.md#runtime-failover)、[§9](Scene%20Runtime%20调度、分片与故障转移.md#runtime-best-effort)；POC 11、12、13 |
| F-20 | revoke 前 DB 请求已提交，输入 offset 未提交 | 新 owner 形成第二个外部效果，或将 Consumer Group 约束误报为 Producer/PG durable fencing | 相同 logical trigger 派生相同 `transitionId/effectId`，唯一键和下游幂等收敛；仍分开记录 owner 重叠窗口 | 主规格 §12.2（稳定 `transitionId/effectId`）；[调度分章 §9](Scene%20Runtime%20调度、分片与故障转移.md#runtime-best-effort)（残余 owner overlap）；POC 13 |
| F-21 | quarantine 写入失败 | 无隔离证据却静默提交输入 offset | 在没有 §4.1 有效人工批准时保持未提交并告警；获准提交时仍记录数据/动作损失，case 按本节终态规则关闭 | 主规格 §12.3（最终失败业务顺序）；[调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)（completion）；POC 10 |
| F-22 | quarantine 重处理持续失败或超 SLO | 无限无声重试或把未收敛目标记成功 | 保留原 trigger/operation/target 与 attempt；升级告警并要求重处理、补偿、control-only 显式 reinit 后的 next-live bootstrap，或人工损失接受；完成前保持 open | 主规格 §5.5、§9.3（target/reinit 业务收敛）；[调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)（completion）；POC 17、21 |

Outbox 证据边界必须保持精确：§19 第 13 项只验收旧/新 owner 重叠或 rolling 重算时的 `effectId` 收敛；第 16 项只验收 scope state 删除不撤销已 commit Outbox，以及显式补偿。
dispatcher 停机恢复、drain、DLQ 与终态审计由主规格 §12 加本文 F-17 扩展用例验收，不能冒充第 13 或第 16 项的证据。

### 6.5 Schedule、partition 与 Kafka 交付

| ID | 注入窗口 | 禁止结果 | 预期收敛与审计重点 | 规范门槛 |
| --- | --- | --- | --- | --- |
| F-23 | Schedule membership 中间页失败 | 已读取前几页先发送 target bundle | retry 整个 fire；发送前 target list 必须完整且有界 | §11.1；POC 18 |
| F-24 | Schedule `limit + 1` 探测超限 | 截断到 limit 后继续执行 | 在发送任何 target 前拒绝整个 fire 并告警 | §11.1；POC 18 |
| F-25 | Schedule 崩溃重试或重复 fanout | 同一 target 产生不同 logical identity 和重复外部效果 | 稳定 occurrence/evaluation identity；相同 effect 由 Outbox 收敛 | §11.1；POC 18 |
| F-26 | event/state partition route mismatch | 打开不共置 lane 继续处理，破坏串行与 restore 假设 | 启动/生产前向量校验失败即阻止启用；运行抽检发现 mismatch 时停用相关 writer 并隔离记录 | §14.1；POC 8 |
| F-27 | 非 state data-event pack 的 Kafka producer failure callback | 提交 offset 后丢弃未投递完整 pack，或错误套用 `scene_state` 的 ACK 非阻塞规则 | callback 触发有界 retry；耗尽后完整原 record 与错误进入 quarantine | 主规格 §13.1～§13.2（物理交付与失败窗口摘要）；[调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)（completion）；POC 10 |
| F-28 | pack timeout | 未完成 callback 的残余工作继续产生部分 bundle | 停止可停止工作；不把部分结果判成功；按完整正确性单元 retry/quarantine | 主规格 §13.1（producer callback 与有界失败处置）；[调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)（completion）；POC 2、10 |
| F-29 | 输入 offset commit failure，或同 partition 后一 offset 先完成且其 state ACK 先/后任意到达 | 把可能重放误报为 exactly-once；越过前一未完成 offset；把 state ACK 顺序当 completion frontier | completion sequencer 只推进连续 next-offset，state broker ACK/callback 不进入完成谓词；commit 失败时新 owner 可重放，稳定 identity 与 Outbox 幂等吸收动作重复，state 风险按 best-effort 观测 | 主规格 §12.2（动作幂等）；[调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)、[§9](Scene%20Runtime%20调度、分片与故障转移.md#runtime-best-effort)；POC 10、13 |
| F-30 | quarantine 成功但 source/target 长期未修复 | 隔离、提交 offset 或指定 owner 即视为业务收敛 | 仅在重处理成功且 current state/效果校验完成，或 control-only 显式 reinit 后下一条合法 production+LIVE trigger 完成 bootstrap/效果校验，或补偿及校验完成，或指定生产风险接受人正式接受不可恢复终态损失后 closed；否则保持 open 并阻止 canary/扩大范围 | 主规格 §5.5、§9.3（target/reinit 收敛）；[调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)（completion）；POC 10、17、21、26；关闭规则见本节 |
| F-31 | Runtime B 已持有 `ACTIVE p1` 时因 Runtime A 硬故障接管 `p0`；再注入 Zombie/missed-revoke 重连，并在 `p0` restore 中对 `p0` 注入 graceful revoke | 等待 A cleanup 才接管；`p0/p1` 共享 generation、restore、completion、Actor 或 timer state；复用 A 旧 context/token；暂停或关闭 `p1`；声称 Consumer Group 能自动 fence Producer/PG | Coordinator 在无 A 参与时发起 assignment；A 必须在 rejoin 前失效全部旧 token；每次新 assignment（包括重得原 partition）都使用 new token/new context 完整 restore，B 的 `p0/p1` context 独立前进。`p0` 在 `RESTORING` 收到 graceful revoke 时必须走 `RESTORING → REVOKING`，token-first 并 pause；只丢弃 `p0` staging、尚未发布的 activation queue 与 timer descriptors，`p1` 继续 `ACTIVE` 并前进。此时 `p0` 的 `scene_event` 尚未开放，通常无新的 eligible frontier；若仍存在 eligible frozen frontier，final commit 也必须按 F-19 与调度分章 §8.1 执行 | [Runtime 调度分章](Scene%20Runtime%20调度、分片与故障转移.md#runtime-state-machine) §§6～9；主规格 §19 POC 8、11、12、13、15、21 |
| F-32 | `p0` 处于 `RESTORING`：raw key K1 valid@10 → tombstone@20；K2 valid@30 → latest corrupt/disabled/incompatible@40；同 raw key bytes 的 earlier valid → 不可解析 latest；再以同 key `V1 < S0`、`V2 >= S0` 注入扫描前/扫描中 compaction，batch crossing 含 offset >= fixed S0，并运行可控 `delete.retention.ms` 的 tombstone-retention/full-scan RTO fixture；同时注入已 due timer 与 scope reconciliation 失败 | 恢复 earlier valid 或把 crossing record 纳入 staging；将 S0 宣称为严格 point-in-time state view；为 offset hole/未观察 key 虚构 tombstone 或 fallback；`S1 > S0` 仍激活；在 retention/full-scan 前提不满足时激活；通过普通 gate 求值，写非 tombstone state、Linked 或 Outbox，推进业务 timer；让 resumed event/direct timer work 越过 activation batch/barrier；在 scope 未收敛时产生任何效果 | 每次 attempt 只对实际观察到的 `< S0` record 断言 latest outcome：K1 为 missing；K2 为 quarantine/missing 且不得 fallback；不可解析 raw key bytes 仍按原始 bytes 的 latest outcome 遮蔽 earlier；`>= S0` 全部排除。扫描结束捕获 `S1`；`S1 > S0` 丢弃 staging、保持 `RESTORING` 并从新 log-start/新 cut 重试，只有 `S1 == S0` 才可继续；不宣称时间点快照。retention/full-scan 保障或 bounded retry/RTO 失败时不激活并告警/人工处置。`RestoreMaintenanceGate` 只允许坏 key quarantine 与纯 tombstone，发布前复验 assignment/token，scope reconciliation 失败时 zero effect；activation batch/barrier 必须先于 resumed event 与 direct timer work，二者均不得越过；已 due timer 仍只在 activation 后以新 token 回 Actor lane | [Runtime 调度分章](Scene%20Runtime%20调度、分片与故障转移.md#runtime-state-machine) §§6～9；主规格 §19 POC 8、11、12、13、15、21 |

## 7. 实施与交付工件

以下均是上线必需产物。
本文只规定产物和验收属性，不虚构 DDL 列、Protobuf 字段或服务接口字段；正式定义必须以版本化工件交付并与主规格逐项对照。

### 7.1 PostgreSQL current Binding 与 lifecycle Outbox

- current Binding DDL、project revision 持久化和 schema migration；
- lifecycle Outbox DDL、索引、保留/清理策略和 dispatcher；
- 创建、修改、启停、删除时 current row、两级 revision 与 lifecycle Outbox 的同事务测试；
- relay 乱序、重复、future row、gap 和数据库故障测试；
- migration dry-run、向前/向后兼容和非破坏性回退说明。

### 7.2 Action Outbox 与 executor

- Action Outbox 正式 DDL，表达主规格要求的唯一 `effectId` 约束、
  必填 `processingNamespace/processingMode/identityAlgorithmVersion`、新 row
  `SCENE_ID_V2`/legacy pending row 只读 allowlist 和 terminal `BLOCKED_CONTEXT`。
- 只有 Scene Actor DB role 可 INSERT Action Outbox；dispatcher role 只可 SELECT
  和更新投递状态，无 INSERT、改 payload 或改 ProcessingContext 权限；
- 多输出同一事务、可用时间、状态迁移和终态更新的事务测试；
- dispatcher 领取、重试、退避、DLQ、drain 和恢复 runbook；
- dispatcher dequeue/每次外发前的 `production + LIVE` 校验，以及非法 row
  CAS/本地事务转 `BLOCKED_CONTEXT`、从 due index 移除、不重试不外发的测试；
- 严格 target 顺序下，`BLOCKED_CONTEXT` 暂停 target lane、page 且禁止
  后序越过的验收；
- 各 executor 的 `effectId` 幂等契约与重复投递验收；
- pre-commit、post-commit、数据库中断和下游超时故障报告。

### 7.3 正式 Protobuf 与 envelope

- `scene_event` 正式 envelope 与所有 event kind 的 IDL；公共 envelope 必填
  `ProcessingContext { processingNamespace, processingMode=LIVE|BACKFILL|REPLAY }`
  与 `identityAlgorithmVersion=SCENE_ID_V2`；
- Admission/Scheduler/Things SubjectLifecycle/Runtime 的 producer-role → 允许 oneof kind
  → 必须 `production + LIVE` 矩阵与各自封闭 publish API；Kafka ACL 只管 Topic，
  role→kind/context 由 producer application contract 强制，Runtime 只二次验证全局
  allowed kind、payload/key/route、context/version，不声称从 record/header 认证 role；
- SOURCE/Linked/same-subject direct/cross-subject target-keyed `LINKED_DELTA`/Schedule/SubjectLifecycle/timer 原样传播 ProcessingContext
  的跨语言 golden fixtures；普通 redelivery 不得改写 context；
- lifecycle control stream、带 ProcessingContext metadata 的 `scene_state`
  full-state/tombstone 正式 schema；Binding lifecycle 仍是不承载 state value 的控制流；
- key encoding、未知 kind、非法 key/payload 和兼容性策略；
- Go/Java generated-code 兼容测试与跨版本读写样本；
- schema registry 或等价治理的 owner、升级和回退流程。

主规格中的示意结构不能直接冒充正式 IDL。

### 7.4 Things current store 与 membership API

- typed current subject 查询与 `SubjectContext` 解析契约；
- current latest/attribute/受限 rolling 读取契约；
- lifecycle Outbox、严格递增 aggregate version 和乱序/gap 收敛测试；
- membership keyset pagination、稳定 device key 排序、selector 下推和 `limit + 1` 契约；
- cache miss、current-store failure、版本对账和 area job 运维接口。

### 7.5 Kafka topic、bootstrap 与分区工件

- control、event 与 state topic 的创建/校验脚本；
- partition count、compaction、retention、replication、producer 参数的环境化配置与审阅记录；
- 生产 `scene_state` 只允许 Scene Runtime `state-writer` principal 写入的 Kafka
  ACL，以及 bounded producer identity/build/contract-version audit headers；
- Admission/Scheduler/Things SubjectLifecycle/Runtime 独立 principal 对生产
  `scene_event` 的最小 Topic ACL 正向测试，以及跨 Scene Topic/写
  `scene_state`/写 `scene_binding_lifecycle` 的反向测试；
- replay/shadow 与其他非生产 principal 无 production `scene_event/scene_state` 写权的
  ACL 反向测试；
- 主规格 §14.1 route bytes/hash 的共享实现；
- Go/Java golden vectors、topic 实际 partition 抽检和启动自检；
- 每个 partition 独立的 state-before-event bootstrap/restore；报告必须包含固定 `[start,S0)`、staging discard、activation queue、与其他 `ACTIVE` partition 的隔离证据及 RTO。

具体 partition count、retention 和容量参数仍由 POC/平台签字，不在本文编造。

### 7.6 Runtime packages

本节只列实施能力，不重分配规范 owner：

- ProjectIndex、admission 与 matching 的业务语义仍由[主规格 §§3、5～6](场景联动%20Runtime%20主规格总纲.md)与[《控制面与匹配契约》](控制面与匹配契约.md)拥有；
- Runtime 跨组件阶段与 Outbox 接受边界仍由[主规格 §§7～10、12](场景联动%20Runtime%20主规格总纲.md)拥有；
- 统一 evaluation、`SceneBindingState`、latest/rolling、Timer identity/current-slot 与冷启动语义由
  [《场景有状态计算规范》](场景有状态计算规范.md)拥有；
- quarantine、最终失败和输入提交窗口仍由[主规格 §§12～13](场景联动%20Runtime%20主规格总纲.md)
  与本文 §4、§6 故障矩阵共同拥有，不得从有状态计算规范反推。

- ProjectIndex 编译、原子 update/rebuild、ready gate 和 revision 对账 package；
- 先于 interest/candidate matching 的 trusted ProcessingContext admission gate、完整
  matching、bundle builder 和统一 router package；
- per-partition context registry、Manager/Actor lane 层级与 index 隔离按 [Runtime 调度分章 §§3～4](Scene%20Runtime%20调度、分片与故障转移.md#runtime-components)实施；
- callback adapter 的 assigned/revoke/lost 转移、new token 与 paused `RESTORING` context 按 [Runtime 调度分章 §6](Scene%20Runtime%20调度、分片与故障转移.md#runtime-state-machine)实施；
- 每个效果边界重验的 `NormalEffectGate` / `RestoreMaintenanceGate` 按 [Runtime 调度分章 §6、§9](Scene%20Runtime%20调度、分片与故障转移.md#runtime-state-machine)实施；
- 固定 `[start,S0)` 的 staging restore、已 due timer activation queue 及 activation 后回 Actor lane 按 [Runtime 调度分章 §7](Scene%20Runtime%20调度、分片与故障转移.md#runtime-restore)实施；
- tentative `StateTxn` 使用只读 accepted base 与有界 mutation overlay（或经同等测试证明的实现）；
  freeze/serialize、一次性 accepted publish、不可变 encoded bytes handoff 与 alias/rollback 门禁按
  [主规格 §12.3](场景联动%20Runtime%20主规格总纲.md#123-状态与-outbox-顺序)和
  [有状态计算规范 §4.2](场景有状态计算规范.md#42-tentative-state提交隔离不是完整深拷贝)实施；
- 每个 partition 独立的 completion sequencer 与 safe contiguous frontier 唯一按 [Runtime 调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)实施；
- partition-scoped Actor/timer/in-flight index 隔离按 [Runtime 调度分章 §4](Scene%20Runtime%20调度、分片与故障转移.md#runtime-partition-isolation)实施；revoke final-commit/no-commit、generation-conditional cleanup 与 partition-scoped close/cancel 按 [§8](Scene%20Runtime%20调度、分片与故障转移.md#runtime-failover)实施，不得影响其他 partition；
- quarantine writer、查询、重处理、control-only 显式 reinit 与审计 package；
- 上述能力的单元、并发、故障注入和容量 benchmark；业务语义与 per-partition 执行协议分别按上述 owner 验收，不因实现分包方式改变。

### 7.7 状态 schema 与兼容性

- versioned `SceneBindingState` 正式 schema 与 full-state 编解码；非 tombstone
  state 及所有 timer slot 必填 `processingNamespace/processingMode` 与
  `identityAlgorithmVersion=SCENE_ID_V2`；
- `semanticHash` 驱动的 preserve 或 close+tombstone+next-live bootstrap 判定表；restore/control 路径不得直接 REINIT/RECREATE 业务 state；
- 32 KiB、rolling 1000、member/timer 有界性和规范序列化测试；
- accepted state 解码常驻量、mutation/encode allocation、Producer in-flight、GC/headroom benchmark，
  以及 map/slice/rolling alias、freeze/配额/PG rollback 后 canonical state 不变测试；
- tombstone、corrupt key 隔离、missing state bootstrap 和跨版本 restore fixture；
- 旧 producer input frontier、`scene_event` end/committed offset、callback 与 DB/Kafka
  in-flight、旧 Action dispatcher lease/dequeue/外发/status/PG in-flight、稳定 pending-set
  frontier、credential revocation、context-less state/Outbox/identity-bearing slot inventory，
  以及 capacity/RTO 报告；
- `SCENE_ID_V2` 与 `LEGACY_NO_NAMESPACE_V1` 的正式编码、golden vectors、dispatcher
  只读 legacy allowlist 和“同一 trigger 不跨算法双求值”的契约测试；
- maintenance cutover、失败回退和恢复证明；禁止以普通滚动方式让旧/新 identity writer
  同时处理同一 trigger。

回退边界必须进入 runbook：首条 `SCENE_ID_V2` record 产生前，可以在保持所有 writer
fenced、确认迁移未生效后撤回整个 maintenance window；首条 V2 record 被接受后，
只能切到继续强制 required context/version 且只写 V2 的兼容 binary，或停写向前修复，
不得重新启用 legacy event/state/timer writer。

ProcessingContext/`SCENE_ID_V2` 迁移顺序不得交换：

1. inventory 并暂停/fence Admission、Scheduler、Things SubjectLifecycle、Runtime
   派生 producer；各 producer 排空已捕获输入前沿与 publish callback 并停止后，
   捕获最终生产 `scene_event` end offsets；旧 Runtime 排空到该最终 end，并排空 timer
   callback、async state send 与 PG/Kafka in-flight；随后 pause/fence 旧 Action
   dispatcher 新 lease/dequeue，排空已领取外发、status update 与 PG in-flight，捕获
   稳定 pending row 集合，再撤销旧 writer/dispatcher credential；
2. 无法排空的 context-less event 由受审计迁移工具隔离，并按主规格最终损失审批推进；
   新 Runtime 不得默认 LIVE 或以新算法重算；
3. 冻结窗口只把“有 production-live 证据且无 timer/pending-trigger 等旧 identity
   字段”的 state 迁到 `SCENE_ID_V2`；其余 state tombstone + 类型化 cold start；
4. 有证据的 legacy pending Outbox 补齐 context 与
   `LEGACY_NO_NAMESPACE_V1`，保留原 `effectId/payload/target/due` 只投递；
   无证据者转 `BLOCKED_CONTEXT`；
5. 量化 rewrite/compaction/cold-bootstrap/blocked backlog 与 RTO 后，才启用 required
   wire/restore/dispatcher 门禁并恢复全部 `SCENE_ID_V2` producer。

离线迁移必须保留原 record/action 的审计关联。Runtime 不得就地补齐或修改
context/version，也不得用 default `LIVE` 或双 identity 计算作为兼容策略。

### 7.8 Metrics、dashboards 与 alerts

- 主规格 §16 指标字典、标签基数预算和采样说明；
- `scene_admission_mode_total{namespace_class,mode,decision}`、
  `scene_processing_context_invalid_total{reason}`、
  `scene_state_processing_context_invalid_total{reason}`、
  `scene_dispatcher_processing_context_invalid_total{reason}` 与
  `scene_outbox_blocked_context_total{reason}` 的完整仪表盘/告警；
- 上述标签值必须是 bounded enum/class，不得包含 `jobId`、`replayId`、
  `effectId` 等高基数标识；
- tenant/project/partition/binding drill-down dashboard；
- per-partition status/token/callback 时间线、completion gap/safe frontier、固定 `[start,S0)` restore progress、staging discard 与 activation queue 专项视图；
- `RESTORING` gate rejection 的 `effect_kind` 与 `reason` 必须都是 bounded enum，且能分辨 non-tombstone state、Linked、Outbox、evaluation 与 business timer；
- raw token、state key 与 `effectId` 只进入受控日志/trace/audit，禁止成为 metric label；
- stale `scene_state` overwrite/compacted-latest 与 duplicate `effectId` 收敛必须使用独立查询与证据，不得汇总成一个“幂等成功”指标；
- 延迟、可用性、完整性三类面板，不把它们汇总成单一健康分；
- correctness invariant 立即告警与 POC 签字阈值告警；
- quarantine age/owner、Outbox pending/due、revision drift、route mismatch 和 stale owner 专项视图。

### 7.9 Feature flags、canary 与 rollback

- 按 tenant/project 可控的 admission、计算、动作和 lifecycle/schedule 能力开关；
- mode gate 是不可降级的安全前置条件，feature flag/rollback 不得绕过；
  context 缺失时必须 fail closed，不得当作 `LIVE`；
- flag 依赖顺序，避免关闭下游后上游继续接受不可交付工作；
- canary 租户/项目清单、流量边界、观察窗口和退出标准；
- binary/schema/topic 的兼容回退 runbook；
- rollback 后已 commit Outbox 的继续投递、quarantine 处置和审计规则。

### 7.10 POC、运维与责任记录

- 主规格 §19 的风险聚类测试报告及原始证据包；
- 容量工作表、SLO 候选、最终签字值和变更记录；
- 最终失败策略/损失风险接受工件、逐 case 批准记录、到期复审和撤销记录；
- 故障演练、on-call、升级路径、数据修复与显式 reinit runbook；
- ProcessingContext 迁移、`BLOCKED_CONTEXT` 审核/取消、严格 target lane 恢复
  以及禁止 runtime default `LIVE` 的 runbook；
- graceful revoke、`OnPartitionsLost`、硬故障接管、Zombie/missed-revoke rejoin、restore 中再 revoke 和按 partition cleanup 的故障演练与 runbook；
- 每个工件的 named owner、reviewer、值班组和替补；
- ADR 仍未冻结或已满足门槛后按 §10 唯一 RACI 形成的独立决策记录。

## 8. Rollout 顺序与 gate

任何阶段只允许在前一阶段证据通过后启用。
延迟/容量回滚信号使用 POC 签字阈值；partial bundle、route mismatch、越 scope、超限 state 产动作和重复外部效果等完整性违规出现一次即阻止晋级或触发回滚。

### 8.1 协议与 DDL

- **准入**：正式 IDL 已把 ProcessingContext 冻结为公共必填字段；key encoding、
  两类 Outbox DDL、`BLOCKED_CONTEXT`、current Binding 事务、Kafka ACL/PG DB
  role、兼容矩阵和 migration dry-run 全部通过。
- **验证**：跨版本 encode/decode 与 context 传播、唯一 effect、事务回滚、
  越权 state/Outbox 写入拒绝、dispatcher 停机恢复/BLOCKED_CONTEXT 终态和
  lifecycle relay 重复/乱序测试。
- **回滚信号**：未知 payload 被误处理、唯一键不能收敛、事务边界破坏、migration 导致现有读写失败。
- **回滚动作**：若尚无任何 `SCENE_ID_V2` record，保持全部 writer fenced 后可撤回 maintenance window；一旦已有 V2 record，只能停止新 writer并切到 V2-compatible binary 或向前修复。新增数据库对象保持 dormant，禁止破坏性逆迁移或恢复 legacy writer。

### 8.2 Index、admission 与 router

- **准入**：ProcessingContext gate 已部署在 interest/candidate 之前；ProjectIndex
  ready/gap/rebuild/future-row、完整 SOURCE/target bundle 和 route golden vectors 通过。
- **验证**：对应 §19 第 1～5、8、19、20 项和 M-01～M-06；`BACKFILL`
  在 matching 前 bypass，`REPLAY`/shadow/非法组合的 production 写入/外发为零；
  发布期拒绝和运行时 whole-unit quarantine 均有可追踪错误。
- **回滚信号**：任一 mode 穿透/默认 LIVE、partial candidate/target、route mismatch、
  旧编译覆盖新 revision，或 POC 阈值内无法完成 rebuild。
- **回滚动作**：关闭 canary admission，保留 control cache 对账；不让旧 router 继续产生新 target bundle。

### 8.3 Actor、state 与 timer

- **准入**：full-state/timer schema 必填 ProcessingContext；32 KiB/1000 samples、
  bootstrap、timer current-slot、state-writer ACL、Actor 二次门禁、per-partition context registry、
  callback adapter、token gates、staging restore、activation queue、独立 completion sequencer、
  partition-scoped close、context migration 和 restore 都有可重复验收入口。
- **验证**：对应 §19 第 6～15、21、22 项、M-07/M-09 与 F-18～F-20/F-31～F-32；除 corrupt/missing/context-less state/timer、send loss、close 后 callback 和 stale owner 外，还必须证明：
  lost/suspend token invalidation 先于 rejoin/assigned；`p0` restore 时 `p1` 仍为 `ACTIVE`；
  固定 `[start,S0)`、失权后仅丢弃 `p0` staging/activation queue；已 due timer 不丢且 activation 后才回 lane；
  `RESTORING` gate 对非允许 effect kind 的拒绝计数与 zero-effect 证据；以及 stale state overwrite 与 duplicate-effect 收敛是两份独立结果。
- **回滚信号**：非 production+LIVE state/timer 被加载/修改、Runtime 默认补
  `LIVE`、超限 state 产生动作、stale callback 产生效果、partition context 混用、
  restore 暂停/关闭其他 `ACTIVE` partition、任一 `RESTORING` 非 maintenance 效果或 RTO 超出签字值。
- **回滚动作**：停止该范围的新 admission/Actor 开放，保留审计与 quarantine；只切到继续强制 required context/version 且生成 `SCENE_ID_V2` 的兼容 binary，禁止恢复 legacy identity writer。

### 8.4 Outbox 与 executor

- **准入**：pre-commit 顺序、post-commit 至少一次、`effectId` 唯一、Actor-only
  INSERT/dispatcher read-status role、dispatcher `production + LIVE` 二次门禁、
  `BLOCKED_CONTEXT` 和所有 executor 幂等测试通过。
- **验证**：pre-commit/commit failure 对应 §19 第 10 项；旧/新 owner 或 rolling
  重算下 `effectId` 收敛对应第 13 项；scope state 删除不撤销 committed
  Outbox/显式补偿对应第 16 项；ProcessingContext 二次门禁、历史
  Outbox 迁移和 blocked lane 对应第 20、22、23 项与 M-08/M-09；dispatcher
  停机恢复、drain、DLQ 与终态审计仍由主规格 §12 + F-17 扩展用例验收。
- **回滚信号**：commit 前出现外部效果、非 production+LIVE row 外发/重试、
  `BLOCKED_CONTEXT` hot-loop 或严格 lane 后序越过、同一 effect 形成重复
  外部效果、pending/due lag 越过签字阈值。
- **回滚动作**：关闭新动作接受路径；已 commit 行继续由兼容 dispatcher 投递或安全 drain，不删除已接受动作。

### 8.5 Lifecycle 与 Schedule

- **准入**：Product/Project/Area、target loss、乱序/gap、late Linked、稳定分页、
  `limit + 1`、重复 fanout 与 control-only reinit 全部通过。
- **验证**：对应 §19 第 14～18、21 项与 M-10/M-11；current scope、member guard、
  reconcileRequired 均可审计；Binding lifecycle/reinit 本身 current-fact read、
  已有 timer slot/handle 必须 close/cancel，predicate/FSM、timer create/arm/advance/fire、
  Alarm、Outbox 为零。
- **回滚信号**：旧 member/state 复活、current context 未推进、部分 schedule target 已发送、稳定 identity 漂移。
- **回滚动作**：关闭该范围 lifecycle/schedule producer，停止新 fanout；保留失败
  operation 并执行重处理，或执行 control-only reinit 后等待 next-live bootstrap。

### 8.6 Failure/scale POC

- **准入**：F-01～F-32 与 M-01～M-13 自动化或可重复运行，基础负载、
  burst、单租户噪声、mode 污染和 failure-under-load 均有原始数据。
- **验证**：F-01～F-32 与 M-01～M-13 全部按 §6 终态规则 closed，并按风险
  聚类关闭 §19 第 1～26 项；容量公式中的 record、delta、bytes、state/action
  写放大由观测替换。故障证据还必须显式包含 callback 顺序、`p0` restore/`p1 ACTIVE`、
  固定 `S0`、staging discard、activation queue、`RESTORING` gate rejection by effect kind，
  以及 stale state overwrite 与 duplicate-effect convergence 的分开结果。
- **回滚信号**：任一 case 仍 open、硬不变量失败、风险窗口无法量化、历史候选未被新签字结果替换，或终态损失未按 §4.1 获批。
- **回滚动作**：不进入 canary，保持相关 feature flag 关闭，修复后从失败 case 与受影响聚类重新执行。

恢复 ADR 在本阶段结束时仍默认“未冻结”。
只有 §19 全部证据齐全，§10“恢复 ADR 冻结”行的必需 C 全部会签，并由唯一 A“指定架构决策人”批准后，才可以另行记录冻结；测试执行完不等于自动冻结。

### 8.7 Canary

- **准入**：前六阶段各自准入记录完成，F-01～F-32 与 M-01～M-13 按 §6
  终态规则 closed；named owner/on-call/替补到位；tenant/project 范围、观察窗口和
  rollback 演练已记录。
- **验证**：镜像/低风险租户后再小范围真实流量；逐步打开 admission、计算和动作，不跨过依赖阶段。观察窗口内必须能按 partition 对账 `p0` restore/`p1 ACTIVE`、completion frontier、activation queue 与 gate rejection，并将 late state overwrite 与 `effectId` 重复收敛分开展示。
- **回滚信号**：任一完整性违规、候选 SLO 的签字阈值越界、quarantine 无 owner、Outbox 无法 drain 或 current scope 不收敛。
- **回滚动作**：按 tenant/project 关闭新 admission 和相关能力；保留已接受动作的投递与隔离记录，完成影响评估后再决定恢复。

## 9. §19 证据按风险聚类

不逐条复制主规格 §19，交付看板按以下四组聚合，但每份报告仍要回链到原始条目：

| 风险组 | §19 条目 | 关闭条件 |
| --- | --- | --- |
| Index / matching / bundle | 1～5 | 单 record 完整视图、原子旧/新 index、gap/future row 收敛、无部分结果 |
| State / Kafka / rebalance / effectId | 6～13 | 配额/带宽、route、bootstrap、producer/state failure、per-partition restore/completion、硬故障/Zombie/callback 顺序与第 13 项 effect 幂等均有证据；stale state overwrite 必须与 effect 幂等分开，不含 dispatcher SLO |
| Scope / lifecycle / Schedule | 14～18 | 分类 lifecycle、current guard、scope exit、cross-subject delivery loss、分页超限和重复 fanout 均收敛 |
| ProcessingContext / state / Outbox isolation | 19～23 | 入口、Actor、restore、timer、dispatcher、权限与迁移门禁均 fail closed，非 production+LIVE 零生产副作用 |
| Capacity / tenant isolation / observability | 24～26 | burst catch-up、noisy tenant 隔离与全路径容量假设由观测替换 |

任一聚类为红色时：

- 不得宣布恢复 ADR 已冻结；
- 不得扩大 canary 范围；
- 不得用另一个聚类的吞吐成绩抵消正确性失败；
- 必须保留失败 case、owner、修复计划和复测入口。

任一 F-01～F-32 或 M-01～M-13 case 未满足 §6 终态规则也按红色处理；
quarantine 成功不改变这一判定。

## 10. Owner / RACI

下表定义角色责任，不虚构人员姓名。
Canary 准入前，每个角色必须解析为实际 named owner、on-call 联系方式和替补，只有团队名没有个人责任人不算通过。
本表是恢复 ADR 冻结审批的唯一责任矩阵；其他章节只能引用本表，不得另列不同的 A 或必需 C 组合。

| 工件/决策 | R（执行） | A（最终负责） | C（会签） | I（知会） |
| --- | --- | --- | --- | --- |
| current Binding、lifecycle Outbox、ProjectIndex | 控制面实现负责人 | Scene 技术负责人 | DBA、Kafka 平台、SRE | 产品与支持团队 |
| Things current store、SubjectContext、membership | Things 服务负责人 | Things 技术负责人 | Scene、DBA、SRE | 产品与支持团队 |
| event/state 协议、router、Actor、timer | Scene Runtime 负责人 | Scene 技术负责人 | Kafka 平台、Things、SRE | 动作 executor 团队 |
| Action Outbox 与 executor 幂等 | 动作平台负责人 | 动作平台技术负责人 | Scene、DBA、SRE | 产品与支持团队 |
| topic、partition、hash vectors、bootstrap | Kafka 平台负责人 | 数据平台技术负责人 | Scene、SRE | Things 与动作平台 |
| metrics、alerts、canary、rollback | SRE 与 Scene 值班负责人 | 生产变更负责人 | 各服务 owner、产品 | 支持团队 |
| POC 容量与候选 SLO 签字 | 性能测试负责人 | 容量责任人 | 架构、SRE、产品、各服务 owner | 生产变更负责人 |
| 最终失败策略与不可恢复损失接受 | 当值执行方 | 指定生产风险接受人 | 业务/产品 owner、数据 owner、SRE、Scene、动作平台、架构（按风险） | 生产变更负责人、支持团队 |
| 恢复 ADR 冻结 | 架构负责人 | 指定架构决策人 | 容量、SRE、Scene、Things、数据平台、DBA、动作平台 | 全体交付方 |

## 11. 最终准入检查表

- [ ] 主规格总纲与 Runtime 调度分章仍共同构成规范性架构源，本文未形成平行 ADR。
- [ ] 32 KiB、rolling 1000 samples 与完整 bundle/fail-closed 已作为硬测试。
- [ ] 所有其余数值均有 POC 报告与容量签字，或明确保持 POC 待定。
- [ ] 两个历史起始候选已经被新证据替换，未进入默认产品承诺。
- [ ] 发布期可证明超限会拒绝，运行时不会返回前 N 或按匹配具体度丢部分 candidate/target。
- [ ] per-tenant admission 与选定的公平调度/独立 pool 机制通过 noisy-tenant 测试。
- [ ] F-01～F-32 与 M-01～M-13 均有 case ID、原始数据、禁止结果断言，
  并已按 §6 终态规则 closed；故意构造且已清理、zero-effect 的安全 fixture 可按
  契约 reject/quarantine 终态关闭，真实故障/污染仅 quarantine 或提交 offset 仍为 open。
- [ ] `production + BACKFILL` 与显式 job BACKFILL 均在 matching 前正常 bypass，
  `REPLAY`/shadow/缺失/未知/表外 mismatch 均验证
  zero production `scene_event/state/timer/Alarm/Outbox` mutation 和 zero delivery。
- [ ] state-writer Kafka ACL、Actor-only Outbox INSERT role、dispatcher 二次门禁、
  `BLOCKED_CONTEXT` 与历史 event/state/Outbox 的停写、排空、迁移/隔离、
  `SCENE_ID_V2` cutover/legacy Outbox 原 ID drain 全部通过；Runtime 没有默认补
  `LIVE`、没有跨算法双求值。
- [ ] `scene_binding_lifecycle`/显式 reinit 必须 close/cancel 已有 timer slot/handle；
  本身零 current-fact read、零业务求值、零 timer create/arm/advance/fire/
  Alarm/Outbox，只由下一条合法 production+LIVE trigger 惰性 bootstrap。
- [ ] 每次按最终失败策略提交 offset 都有范围匹配、未到期的 §4.1 工件和指定生产风险接受人逐 case 批准，Runtime 未自动决定损失接受。
- [ ] Outbox commit 前后承诺、state async loss 与 exactly-once 非目标已向产品和运维说明。
- [ ] 正式 DDL、IDL、Things API、topic/hash、Runtime 实施工件、state schema、观测与 runbook 全部归档；per-partition context registry、callback adapter、token gates、staging restore、activation queue、独立 completion sequencer 与 partition-scoped close 的证据齐全。
- [ ] 七阶段 rollout 的准入、回滚信号、feature flags 和 named owner 已演练。
- [ ] 主规格 §19 第 1～26 项证据齐全，且 §10“恢复 ADR 冻结”行的必需 C 全部会签并由指定架构决策人批准前，恢复 ADR 保持未冻结。
