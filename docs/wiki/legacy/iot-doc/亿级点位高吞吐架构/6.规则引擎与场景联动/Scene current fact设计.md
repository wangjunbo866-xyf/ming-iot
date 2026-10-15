# Scene Current Fact 设计（Scene-live 槽）

> 状态：**设计已定、前提待验**。§9 的 blocker 按其适用于所选档位或变更的范围生效；所选档位或变更的适用 blocker 未关闭前不得启动实施。
>
> 权威层级：本文是 scene-linkage 的**规范性分章**，唯一拥有 Scene Current Fact 的投影、
> 水线、读取 API、部署档位与存储选型。主规格总纲继续拥有产品、wire、业务状态/事务、route
> 与容量；与主规格/Runtime 调度分章没有"后写覆盖"关系，冲突必须在同一次变更中联合修订。
>
> 来源：本文由《Canonical → Scene Fact 投影契约补充修订设计》（2026-08-04）§8 扩展成文；
> 通用 latest 降级计划见该设计 §8.5 与 §13 第 30 项；质量码前提见
> [《质量码契约设计》](../2.编码解码协议/质量码契约设计.md)。

## 1. 职责与边界

Scene Current Fact 是 Scene 专用的 LATEST_STATE 当前值投影：每个事实一行，回答
"**现在是什么**"，并自带"**答案可信**"的证明。

管什么：

- 每个 key 存一行：availability + 规范化当前值 + 完整 provenance；`UNAVAILABLE` 是合法值；
- 支撑四类读取：latest-value join 的非触发参数、冷启动 bootstrap、每次业务求值前的 overlay
  （warm 也读）、timer 到点复核；
- 完整性证明：水线 + coverage seed，把"还没投影"和"从未见过"分开；
- 统一裁判：compareLatest 的唯一执行点，冲突写 durable fence；
- 每类事实唯一写入者。

不管什么（边界）：

- 不是窗口/历史：rolling 在 SceneBindingState，大窗口在 Computed Metric/Flink（算成新事实再绕回）；
- 不是事件：OCCURRENCE 永不写入；
- 不是 Scene 算子状态：那是 SceneBindingState/scene_state（best-effort 恢复副本）的事；
- 不是业务查询视图：那是通用 latest（已降级为可选，见设计 §8.5）；
- 不听动态 interest（静态 eligible 就写），不接受 Runtime/Admission 写入（单向只读）。

写入纪律：先裁判后落库、先 durable 后 frontier、BACKFILL 绕过但 frontier 照推、冲突 durable
后不毒死 partition。

## 2. 数据模型

逻辑 key 与 value：

```text
SceneCurrentFactKey   = (tenantCode, factOwnerKey, factType, factKey)
SceneCurrentFactValue = { temporalKind = LATEST_STATE, availability,
                          canonicalValueType, payload?, unavailableReason?,
                          retainedFactProvenance }
```

`factOwnerKey` 是 FACT_OWNER_KEY_V1 typed canonical bytes：
`deviceTenure(canonicalDeviceId, ownershipBindingId)` 或
`entityIncarnation(subjectType, canonicalSubjectId, subjectIncarnationId)`；typed subject ID
永不复用时 incarnation 由 `H(PERMANENT_SUBJECT_INCARNATION_V1, subjectIdentityPolicyRef,
subjectType, canonicalSubjectId)` 确定性生成。owner tenure/实体重建后旧事实不串新槽位。

同库存放的小元数据（独立小表/key 空间，同一可用域，读取 API 一次取齐）：

| 数据 | 体量随什么增长 | 说明 |
| --- | --- | --- |
| ProjectionCoverageSeed | 设备 × 激活次数 | "上岗记录"，见 §5 |
| CurrentFactConflictFence | 冲突数 | 出冲突才有行，见 §4 |
| durable conflict/quarantine outcome | 冲突数 | 终态证据 |
| readyThrough 水线 + projectionGeneration | partition 数 | 每 (stream, partition) 一行 |

## 3. 写入者与完成谓词

每种 LATEST_STATE 来源有且只有一个写入者：

| 来源 | 写入者 | 输入与 group/checkpoint |
| --- | --- | --- |
| PROPERTY | Timeseries Materializer 的 Scene-live 槽 | 完整 Canonical PropertyReport；沿用 Materializer 唯一逻辑 group/checkpoint |
| COMPUTED_METRIC | ComputedMetricCurrentFactProjector | ComputedMetricUpdated 原始 stream；独立 group `scene-current-fact-computed-metric-v1` |

### 3.1 正常写入与统一完成谓词

Materializer 一次解码写 history 与 Scene-live 槽（通用 latest 为可选目标，见设计 §8.5）。
对每个输入 `o`，完成语义为：

```text
inputComplete(o) =
    适用性判断已经终结
    AND 所有适用写入目标已经 durable
    AND 规范要求的 terminal evidence 已经 durable
    AND 不存在未解决的暂时性失败
```

若需要变更 value slot，该 row 就是适用写入目标，必须 durable；若不需要变更 value slot，
则不执行该 value slot 的 mutation，但不阻塞完成。只有连续的一组已完成输入才能推进
`readyThroughNextOffset`。`readyThroughNextOffset` 与 Kafka committed offset 是不同的概念，
但两者都不得越过任何未完成输入；底层 store 必须保证同一 partition 的 current rows 先 durable、
frontier 后推进。

Admission 与 Runtime 都不是写入者；即使 Admission 为静态 eligible 的 CONTEXT_ONLY 更新发布
SOURCE_DELTA，Runtime 也只能做零 fanout，不能借机写 Current Fact。写入范围 = 静态 eligible ∩ LATEST_STATE ∩ production+LIVE（收缩策略见 §8）。

### 3.2 无值变更完成

**无值变更完成（no-value-mutation completion）**是逻辑上的输入完成分支，不是新的 wire enum、
lookup state、Materializer state 或表。普通确定性 no-op 不要求逐输入持久化独立 outcome；连续
frontier 是其聚合 durable 证明。在 frontier durable 之前，Kafka checkpoint 不得越过该输入；
crash 后必须 replay，并重新执行确定性的适用性判断。frontier durable 后，它证明此前连续的输入均已完成。

`BACKFILL(历史补数)/REPLAY(重放)/shadow(影子流)` 不得创建/推进/删除 Scene-live value slot；
现有规范要求的 deterministic bypass terminal outcome 必须先 durable，才可推进 frontier。
OCCURRENCE 永不写 Current Fact；适用处理完成即为无值变更完成。技术 tombstone 只用于 subject 删除、tenure 结束或明确投影 GC，不用于普通 no-op、
bypass 或业务 UNAVAILABLE。

“当前 A/B 规则未引用 C”不等于 C 不写 Current Fact；是否写入由全局静态 eligible 与 temporal contract
决定。仍被其他 Bindings 使用的 facts 继续更新其自身 slots。

### 3.3 尚未完成与 retry

暂时性 store/catalog/contract 失败没有可信 terminal result，必须 retry，frontier 不得越过该输入。
Runtime 继续观察既有的 `PROJECTION_NOT_READY/BEHIND` 或 `LOOKUP_FAILED` 读取结果。

### 3.4 cutover 与迟到旧数据

owner/model/metric-plan cutover 经受信控制面发布流进入唯一 writer。semantic ID 缺失、
eligible=false、semanticContractHash 改变或 owner incarnation 改变时，writer 必须在 cutover 生效前
写 visibility fence，防止旧槽位继续冒充新事实。只有该变更同时表示 subject 删除、ownership tenure
结束或明确投影 GC 时，才允许写技术 tombstone。晚到的旧-release backlog 不得重新打开或推进新 slot；它以
`SOURCE_ARTIFACT_NO_LONGER_APPLICABLE` terminal no-write 完成后，才可加入连续 frontier。

`projectionGeneration` 标识一次受控构建的 coverage lineage；initial build、source
topology/route 变化或不连续重建时换代；row/frontier/seed 同属一个 generation，读者不得拼接
两个 generation；同一次受控构建可跨进程恢复，不是进程启动随机 UUID。

## 4. 排序与冲突

projector 对同一 key 调用 §7.2（设计文档）的统一 comparator（compareLatest）：先 eventTs、
同 ts 再 factVersion、domain/encoding 不兼容即 `ORDER_DOMAIN_CONFLICT`；value 与 UNAVAILABLE
同一门控；同 order 冲突禁止 last-write-wins。

冲突处置：

- comparator/identity conflict：writer 先把以 `(sourcePosition, factOwnerKey, factType,
  factKey)` 幂等的 bounded conflict/quarantine outcome durable，计为 terminal
  no-value-mutation；同时对 SceneCurrentFactKey 写 durable `CurrentFactConflictFence`
  （generation、conflict kind、双方 envelope hash/order-domain 与 source positions）。旧 value
  row 可保留审计，lookup 不得再返回 READY；
- 永久 source-contract 错误：可解析具体 key 时写 key 级 fence；artifact→key mapping 本身不可信
  时写 artifact/projection-generation 级 fence，依赖该 artifact 的全部 lookup 返回
  `PROJECTION_CONFLICT`；
- fence 清除：`SAME_ORDER/AMBIGUOUS/VERSION_PRESENCE` 类仅当同一 order domain 出现严格高于
  整个 conflict order 的 accepted row 时，"写新 row + 清 fence"作为一个 durable mutation；
  `FACT_IDENTITY_CONFLICT / ORDER_DOMAIN_CONFLICT / source-contract` 类只能经受审计
  repair/rebuild（新 projection generation）清除；
- 只有该 input 的适用性判断已终结、所有适用 mutation 已 durable，且规范要求的 reason-bearing
  terminal outcome 已 durable，才能推进连续 frontier。

## 5. 读取 API 与 readiness

### 5.1 六种状态

`READY_AVAILABLE`、`READY_UNAVAILABLE`、`READY_NEVER_SEEN`（须 seed 背书）、
`PROJECTION_CONFLICT`、`PROJECTION_NOT_READY/BEHIND`、`LOOKUP_FAILED`。
投影落后或 lookup 失败**不是业务 missing**，必须 retry/fail closed，不得进入 Scene 的
NOT_READY/false 分支后继续产生动作。

### 5.2 水线与 barrier

- 水线即写入者发布的应用侧 partition 连续 `readyThroughNextOffset`，数值采用 next-offset 约定；
  它与 Kafka committed offset 是不同概念：前者证明投影连续完成，后者记录 consumer 恢复进度，
  但二者均不得越过未完成输入；跨 topicGeneration、跨 partition 的 offset 不可比较；
- 每个进入业务求值的 logical trigger 在本轮第一次读取时，为所有必要 route 捕获固定
  source-end vector，形成 `readBarrierId`；同一 actor attempt 内的有界 retry 复用同一 barrier；
- 热路径不得逐评估调 `ListOffsets`：`CurrentFactBarrierSnapshotProvider` 将并发请求
  micro-batch，**只有登记完成之后才开始**的 capture cycle 才能满足这些 trigger；cycle 对 route
  并集批量捕获 end offsets 并返回不可变 snapshot；登记前的缓存即使很新也不得使用；
  `barrierBatchInterval/maxBarrierCaptureLatency` 需 POC/SLO 签字；超时等下一 cycle/retry，
  不退回旧 snapshot；当前 trigger 自身 route 的 captured end 至少为 `recordOffset + 1`；
- crash/rebalance 丢失 in-flight barrier 时允许为同一 logical trigger 重新捕获；系统不假装拥有
  durable MVCC read timestamp；
- barrier 是 readiness 的固定**下界**，不是历史 as-of 快照；达到后可读到更新的值；V1 不承诺
  跨 partition 的 event-time 原子快照。

### 5.3 多节点水线

水线不在任何节点内存中，而是**共享库里每 (stream, partition) 一行的小记录**：

- 谁写哪条水线由 consumer group 的 partition assignment 决定；先数据行 durable、再更新水线行
  （PG 可同事务：水线可见 ⇒ 数据必可见）；
- 任意 Runtime 节点读水线 = 点查该行；无节点间 RPC/锁/gossip；节点增减、rebalance 只改
  assignment，水线数据不动；
- rebalance 缝隙：group 协议保证单 owner（旧 owner 先 revoke）；cutover 显式撤销旧 write
  token；同 generation 内水线单调递增、重复写无害；跨 generation 不可比较。

### 5.4 coverage seed

`READY_NEVER_SEEN` 必须有 durable `ProjectionCoverageSeed` 背书：

```text
ProjectionCoverageSeed {
  tenantCode, factOwnerKey, sourceArtifactSetHash, eligibleFactSetHash,
  seedKind = OWNER_OR_MODEL_ACTIVATION | METRIC_PLAN_ACTIVATION | CONTROLLED_INITIAL_BUILD,
  entries[] { sourceStreamId, partition, projectionGeneration, effectiveFromNextOffset }
}
```

- 一条 seed 可按 eligible fact set 覆盖全部事实，不为每设备每点各写一行；
- 已有事实后开 eligibility（false→true）时，seed 不得把 effective-from 前移：只能用扫了完整
  obligation prefix 的受控初始构建，或能证明该 prefix canonical max 的 **authoritative current
  rebase** 创建新 seed；**仅等下一条 LIVE 更新不能证明它高于未投影前缀中的乱序 eventTs**；
- 每个 key 的查询结果携带 seed ref 与逐 route `projectionObligationStartNextOffset`；所有
  `READY_*` 要求 seed 的 tenant/owner/artifact/fact-set 与当前 contract 一致、generation 相同、
  offset 不等式成立；任一不满足，即使后来出现合格 LIVE row 也只能返回
  `PROJECTION_NOT_READY`，只有受控 rebase 的新 coverage proof 能解除；
- 存在当前 generation 的 fence 时优先返回 `PROJECTION_CONFLICT`；
- seed/水线/fence 与 Current Fact 行同库（见 §7），由唯一 writer 在激活/构建时写入，读取 API
  一次性校验。

### 5.5 evaluation-time overlay 与 triggerRight

每次业务求值（data/schedule/timer/lifecycle，warm 也含）前：

1. 确定所需 Current Fact → 2. 并行读取并验证 coverage/readiness → 3. 构造 tentative
BindingState → 4. 以查询结果为 baseline 按 comparator 合并当前 bundle（本批不无条件覆盖；
仅 ACCEPT 或与当前 sourceEventId 完全相同时使用本批值）→ 5. 另算 triggerRight →
6. missing/freshness、predicate/FSM、timer 逻辑。

triggerRight 规则：BindingState 已处理该 ID → 无新触发权；Current Fact 的 factDeltaId 与当前
相同但 envelope 不同 → `FACT_IDENTITY_CONFLICT` fail closed；Current Fact 恰好等于当前记录
（projector 已写）→ value duplicate 但保留一次触发权；store 语义更新 → stale 无触发权；store
较旧或不存在 → 接受并获触发权；同 order 冲突 → 受影响 bundle fail closed。BindingState 缓存的
任何 SingleValue 只是上次求值视图，不得作为"仍是 current"的证据。rolling 参数必须是 TRIGGER，
`CONTEXT_ONLY + ROLLING` 发布期拒绝。

## 6. 双档部署契约

Scene-live 槽是**一套读取接口、两个明写的保障等级**。接口（六状态 + coverage token + 批量读 +
overlay 语义）不变；部署契约显式携带 `projectionTier`。**两档不是透明互换**——等级差异必须
写成契约，否则等级差本身成为新的静默错误源。

| 能力 | FULL（独立投影档） | DIRECT_TD（直读 TDengine 档） |
| --- | --- | --- |
| `READY_AVAILABLE / UNAVAILABLE` | ✓ | ✓（前提：质量码列，见 §9） |
| `READY_NEVER_SEEN` | ✓（seed 背书） | ✗——查不到只能 `PROJECTION_NOT_READY`；"从未上报"与"还没写入"不可区分 |
| `PROJECTION_CONFLICT` | ✓（fence） | ✗——同 ts 冲突静默 LWW，列入已知残留风险 |
| 完整性证明 | 水线 + barrier provider | 无；降级为软 lag 门（Materializer lag 超阈值本轮 retry） |
| LIVE-only 隔离 | ✓（BACKFILL 绕过） | ✗——BACKFILL 可污染查询结果，列入已知残留风险 |
| provenance / factDeltaId | ✓ | ✗ 或从 history 列重算 |
| 新增设施 | Scene-live 槽库 + writer + barrier provider | 零新增（Materializer 只写 history） |

**tier-invariant（两档都必须成立）**：值合并走 Runtime 内 comparator（旧读值不得盖新
delta）；触发权来自 delta 本身；timer/schedule 到点强制重读；freshness 门（超时值判
NOT_READY 而非当 current）；EffectSetReceipt 动作收敛与本模块无关、两档同样生效。

行为差异（产品可见）：DIRECT_TD 档新建场景在稀疏点上等首条数据；冲突降级为 LWW；BACKFILL
可影响求值上下文。这些必须写入该档的"已知残留风险清单"交付运维。

档位切换：FULL→DIRECT_TD 是降级放弃证明；DIRECT_TD→FULL 需受控初始构建（§5.4）。§14 相关
验收用例按 tier 打标签：冲突类/NEVER_SEEN 类/barrier 类只在 FULL 档要求；DIRECT_TD 档另含
"BACKFILL 污染可见"的反向用例。

## 7. 存储指引

语义全部在写入者与读取 API，存储层保持 dumb（按 key upsert + 批量点查 + 小表），因此
"值放哪"是部署决策：

| 候选 | 结论 |
| --- | --- |
| **Redis**（FULL 档默认） | 点查最快、批量 mget 友好；须定死 AOF 持久化策略与"丢失即新 projection generation 重建"的 runbook |
| **PostgreSQL**（小规模/eligible 子集小） | 平台已在用，运维零新增；写 QPS 上限低，亿级全量高频不适用；选 PG 则本模块零新增物理系统 |
| TDengine 独立表 | **不作默认**：近全量 eligible 时子表数 ≈ 翻倍，vnode 元数据内存同比放大；仅 eligible 比例低时可考虑 |
| TDengine history last（DIRECT_TD 档） | 见 §6；不新增存储，语义按 DIRECT_TD 契约降级 |

规模估算（FULL 档）：行数 = eligible 点数；写 QPS = eligible 点位更新率。provenance 中重复的
contract ref/hash/ProcessingContext 以字典化去重压单行字节（不得为省字节丢失
comparator/freshness/retention/审计证据）。

## 8. 写入范围与收缩

"必须写"的下限 = 被启用 Binding 引用的 LATEST_STATE SingleValue 参数（TRIGGER ∪
CONTEXT_ONLY）。设计写到静态 eligible 超集的原因：新建场景冷启动不能等首报；Canonical 热
保留期有限，事后补建够不着老值；interest 不能当 emit gate（ACK 歧义分叉）。

正确收缩杠杆（从大到小）：

1. **静态 eligibility 默认 opt-in**（`isCanSceneLinkage` 默认关闭、产品模板开启）——写量与行数
   直接降一个数量级；配套审计"eligible 但长期无人引用"的点并回收（true→false 走 fence 路径）；
2. **租户级总开关**：从未建场景的租户整租关闭；代价是该租户首个场景需受控初始构建或等首条
   LIVE（显式产品决策）；
3. **单行字节压缩**（§7 字典化）；
4. metric 侧同理：plan output 的 `automationEligible` 默认关。

代价明示：false→true 需发布新快照 + 受控初始构建/authoritative rebase 生成 seed，不是即时
可用。

<a id="current-fact-prerequisites"></a>

## 9. 前提与未决项（blocker 清单）

以下 blocker 按其适用于所选档位或变更的范围生效；所选档位或变更的适用 blocker 未关闭前不得启动实施：

适用关系：FULL → blocker 4；DIRECT_TD → blockers 1、2；通用 latest 降级 → blockers 1、3。

1. **effectiveTime 前提（DIRECT_TD 档 / 通用 latest 降级）**：确认 history 表主键 ts 即写入时已校验的 effectiveTime（含
   deviceTime 可信度回退，不可信时钟取 receivedAt）。否则异常未来时间戳会污染 last 视图，
   DIRECT_TD 档与通用 latest 降级同时受阻。验证：模块级超级表写入路径/Materializer 源码。
2. **质量码列前提**（DIRECT_TD 档）：history 行须逐列携带质量码，且 (a) 坏状态（INVALID/
   NULL/RESET/BAD_QUALITY）成行、(b) ABSENT 与 NULL_VALUE 行内可区分。契约与落地见
   [《质量码契约设计》](../2.编码解码协议/质量码契约设计.md) §5/§7。验证：抽查 INVALID
   上报，`last_row` 末行可见 `q_summary≠0`。
3. **P0 消费者盘点（通用 latest 降级）**：全仓盘点通用 latest 的读取方并标注"展示型 / 正确性型"；发现正确性
   消费者时先迁移（本模块 FULL 档或专用投影），再允许通用 latest 降级（设计 §8.5）。
4. **必要 source route 解析契约（FULL 档）**：当前规范冻结了 `sourcePosition` 与 barrier
   capture，但尚未冻结完整 required-fact set 如何为每条必要 route 解析
   `(sourceStreamId, partition, projectionGeneration)`。该未决契约必须覆盖 SOURCE 与传播的
   LINKED lineage，以及 timer recheck、schedule、lifecycle evaluation；必须识别
   `factOwnerKey`/owner tenure、source stream/topic generation、authoritative partition mapping
   与 projection generation/cutover。严禁通过复用 `scene_event` `routeKeyBytes` 猜测
   Canonical/Computed Metric partition，也不得临时解释 `sourceContractRefs`。FULL 档整体实施在联合
   ADR/contract 冻结前一律受阻；本文不定义新字段名，也不选择解决方案。

## 10. 验收与指标

- 设计文档 §14 中 F-05/F-12/F-13/F-16/F-17 为本模块相关用例，按 §6 的 tier 打标签；
- 低基数指标（既有清单之外补充）：`scene_current_fact_barrier_wait_seconds`（直方图）、
  `scene_current_fact_read_total{status,reason}`（NOT_READY 拆 reason：无 seed / generation
  不符 / frontier 落后）；
- 钻取路径：低基数指标 → durable conflict/quarantine 记录（key 含 sourcePosition、
  factOwnerKey、factType、factKey）为运维契约，禁止靠日志定位。

## 11. 运维 runbook（骨架，随联合修订补齐）

- **投影 lag 告警**：水线 lag 超阈 → 查 writer/存储；求值延迟随之变长是预期行为（响亮而非静默）；
- **fence 修复**：按 conflict kind 分级——SAME_ORDER 类可由严格更新自动清；identity/domain 类
  需受审计 repair/rebuild（新 generation）；
- **generation 重建**：存储丢失/损坏 → 受控初始构建 + 新 seed；
- **tier 切换**：按 §6 契约执行，DIRECT_TD→FULL 必须受控初始构建；
- **软 lag 门**（DIRECT_TD 档）：Materializer lag 超阈时本轮求值 retry。
