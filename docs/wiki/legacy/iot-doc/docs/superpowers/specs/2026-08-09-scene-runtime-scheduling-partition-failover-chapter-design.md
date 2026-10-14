# Scene Runtime 调度、分片与故障转移规范性分章设计

- 日期：2026-08-09
- 状态：待书面评审
- 目标分章：`亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md`
- 联合修订入口：同目录 `README.md` 与 `场景联动 Runtime 主规格总纲.md`

> 本文是规范拆分的变更设计工件。实施完成后，它作为非规范性历史记录保留；运行时实现只能引用
> 目标分章、主规格总纲和从属实施契约，不能把本文当成第二份运行时协议正文。

## 1. 背景与问题

当前 Scene Runtime 的核心协作协议分散在主规格 §4～§14：Manager 负责什么、一个 Runtime
如何承载多个 Kafka partition、Actor lane 如何串行化、state/timer 如何恢复、offset 如何推进，
以及 Runtime 失联、重连和成为 Zombie 时如何收敛。单独阅读任一节都不能回答完整的接管问题。

这些内容既复杂又决定动作正确性，不适合继续作为主规格中的零散段落，也不能复制到一篇非规范性
说明中形成双真源。本次把它们重组为独立的规范性分章，同时补齐此前仅被隐含的
`PartitionRuntimeContext`、Consumer Group session 失效和 Zombie 重连边界。

## 2. 目标与非目标

### 2.1 目标

- 让读者单独阅读一章即可串起 `Manager → partition → Actor lane → BindingState`；
- 明确同一 Runtime 承载多个 partition 时哪些资源共享、哪些状态必须隔离；
- 明确正常处理、per-partition completion、timer 回 lane 和 state-before-event restore；
- 分开描述 graceful revoke、原 Runtime 硬故障和错过 revoke 的 Zombie；
- 冻结 V1 best-effort fencing 与幂等托底的准确承诺；
- 保留主规格现有稳定标题和锚点，使历史链接仍可阅读。

### 2.2 非目标

- 不引入 DB owner epoch、分布式 lease、Kafka transaction 或跨存储 exactly-once；
- 不把 Scene Actor 改成通用 Actor System，也不选择第三方 Go Actor 框架；
- 不序列化 Go Actor、mailbox、锁、goroutine 或 timer handle；
- 不修改 `SceneBindingState` schema、Topic、route hash、Action Outbox 事务或物理 partition count；
- 不重新选择 Kafka assignor 或 Go 客户端；消费端继承 Kafka 规范已冻结的
  `franz-go + classic group + cooperative-sticky` 基线，但本章不冻结线程模型；
- 不把恢复 ADR、容量 POC 或故障测试状态从“未冻结”改成“已通过”。

## 3. 规范权威关系

实施后不再把一个文件称为 scene-linkage 唯一规范性架构源。架构决策由主规格总纲与 Runtime
调度分章共同拥有；三份从属实施契约只在各自范围内细化它们，不得反向覆盖架构决策：

| 文档 | 规范职责 |
| --- | --- |
| 主规格总纲 | 产品边界、wire、Binding/SceneBindingState 业务语义、求值、Action Outbox 事务、容量与总体取舍 |
| Runtime 调度分章 | partition ownership、Manager/Actor 执行、PartitionRuntimeContext 状态机、completion、restore、rebalance 与 Zombie 协议 |
| 控制面与匹配契约 | 从属实施契约：Binding、ProjectIndex、posting、scope 与 matching 细节 |
| 算子语义与术语 | 从属实施契约：fact/state/operator/timer 的业务语义和身份字典 |
| SLO 故障测试与实施 | 从属实施契约：指标、POC、故障注入、上线门槛和 RACI |

主规格总纲与 Runtime 调度分章共同组成 Scene Runtime 的规范性架构决策源。两者没有“后写覆盖”
关系；从属契约只能细化、不能改写两份架构源。任意冲突都必须在同一次变更中联合修订，未收敛前
相关契约不得标为已冻结。README 只负责阅读路由和权威声明，不复制协议正文。

## 4. 一分钟心智模型

```text
Kafka Consumer Group
└─ 将 scene_event TopicPartition 分配给 Runtime 实例
   └─ PartitionRuntimeContext[p]
      ├─ localPartitionGeneration token / lifecycle / restore gate
      ├─ per-partition completion sequencer
      ├─ Actor lane index / timer index / in-flight index
      └─ Actor lane (tenantCode, evaluationSubjectKey)
         └─ 多个 SceneBindingState
            (tenantCode, bindingId, evaluationSubjectKey)
```

这里必须冻结四个边界：

1. Kafka partition 是实例级消费权、Actor state/timer ownership 和故障转移单位；它不是 Actor。
2. 一个 partition 可以承载大量 Actor lane，一个 Actor lane 可以持有多个 BindingState。
3. ProjectIndex 在每个 Runtime 全量复制；Actor state/timer 只存在于当前拥有其 partition 的 Runtime。
4. Kafka assignment 是动态的，partition 不固定到某台 Runtime，也不存在“原主机永久拥有的分区”。

## 5. Actor 术语边界

本设计只借用 Actor 模式的三个性质：

- 以 `(tenantCode, evaluationSubjectKey)` 形成 subject-keyed serial lane；
- 同一 lane 的状态变化和 timer callback 串行进入统一求值入口；
- 外部调用通过消息或任务进入 lane，不直接并发修改 BindingState。

`Actor lane` 不承诺一个 Actor 一条 goroutine、独立 mailbox、远程 Actor 协议、supervision tree、
transparent location 或 Actor snapshot。实现可以用共享 worker pool、分片队列或 keyed executor，
只要保持 lane 串行化、容量上限和本章的 partition 生命周期门禁。因此不能因术语为 Actor 而强制
采用现成 Go Actor 框架。

## 6. 同一 Runtime 承载多个 partition

Runtime B 同时获得 `p0`、`p1` 时是“一个进程承载两个独立 ownership”，不是把两条流和状态合成
一个状态域：

| 可共享 | 必须按 TopicPartition 隔离 |
| --- | --- |
| ProjectIndex 和编译产物 | lifecycle/status 与 localPartitionGeneration token |
| worker pool、调度器实现 | state restore boundary 与 restore gate |
| Kafka Producer、PG 连接池 | input position 与 completion sequencer |
| 序列化器、脚本运行库 | Actor/BindingState、timer、in-flight 反向索引 |
| 进程级配额和指标出口 | partition-local SubjectContext cache 与 revoke/close 范围 |

Actor key 通过冻结的 route 函数唯一映射到一个 partition；该 Actor 的全部 BindingState、timer 和
目标消息都归属同一 `PartitionRuntimeContext`。实现允许使用全局 map，但必须能从 partition 精确
枚举和关闭所属 Actor，禁止 revoke `p0` 时清理 `p1`，也禁止遗留无法归属的 Actor/timer。

即使 `p0`、`p1` 都由 B 持有，跨实体 target 仍写成终端 `LINKED_DELTA` 并按 target key 进入
目标 partition。只有 `sourceSubjectKey == evaluationSubjectKey` 时才本地直递；此时 source 与
target 天然处于同一 route key，不形成跨 partition 快捷路径。

## 7. PartitionRuntimeContext

`PartitionRuntimeContext` 是进程内实现抽象，不进入 wire 或 `SceneBindingState`。每个当前或正在
接管的 TopicPartition 至少包含：

```text
PartitionRuntimeContext {
  sceneTopicGeneration
  topicPartition
  status
  localPartitionGenerationToken
  restoreBoundary / restoreGate
  completionSequencer
  actorIndex / timerIndex / inFlightIndex
  groupSessionToken
}
```

`sceneTopicGeneration` 是 §14.2 Topic generation，不是 Consumer Group generation；
`groupSessionToken` 是本次进程内 Consumer Group session 的不透明身份，不进入业务 identity。
`restoreGate` 在该 partition 完成恢复前阻断普通 event dispatch。

`localPartitionGenerationToken` 实现主规格已有的 `localPartitionGeneration`：它是一次本地
activation 的不可变能力标识。所有普通 record、timer、async continuation 和数据面副作用请求
在创建时捕获 token；执行前必须通过 `NormalEffectGate`：

```text
capturedToken == currentToken
&& partitionStatus == ACTIVE
&& partition 仍属于 current assignment
```

新 assignment 必须创建新 token。禁止原地复活旧 token，也禁止旧任务在执行时重新读取并借用
新 token。token 仅驻进程内，不进入 state key、`effectId` 或持久化身份。

`RESTORING` 只有一个窄例外：`RestoreMaintenanceGate` 可在 token/assignment 仍有效时隔离坏 key，
并对 current Binding 已删除、禁用或语义不兼容的旧 state 异步发送纯 tombstone。它不得读取 current
facts 后求值，不得写非 tombstone state、Linked 或 Action Outbox，也不得创建/推进业务 timer。
该例外只清除已判无效的恢复输入，不授予普通数据面副作用权限。

任何可能推进数据面 state 或动作的 manager/per-state timer，在 target route 确定后都必须归属
对应 partition 并捕获同一个 token；不能把 manager timer 放进不受 partition 生命周期约束的全局
回调池。纯配置刷新任务不取得数据面副作用权限。

## 8. PartitionRuntimeContext 状态机

```text
DETACHED
  └─ assign(p, new token) → RESTORING

RESTORING
  ├─ config/state restore complete → ACTIVE
  ├─ assignment lost / group session uncertain → SUSPENDED → DETACHED
  └─ recoverable restore error → 保持 RESTORING，按主规格 retry/quarantine

ACTIVE
  ├─ graceful revoke → REVOKING → DETACHED
  └─ group ownership cannot be proven → SUSPENDED → DETACHED/rejoin

REVOKING / SUSPENDED
  └─ token 已先失效；禁止重新产生副作用
```

- `RESTORING`：继续维持 Consumer Group poll/heartbeat，但 pause 该 partition 的普通 event
  dispatch；可以读取配置和 state、创建隔离的恢复产物，不能执行普通数据面副作用；
- `ACTIVE`：唯一允许接收新 input、推进 Actor/timer 和进入副作用入口的状态；
- `REVOKING`：已收到明确 revoke，先失效 token，再停止新工作并 bounded drain；
- `SUSPENDED`：Consumer Group session、assignment 或 owner 身份无法证明，必须 fail closed；
- `DETACHED`：无 partition 权限，也不保留可再次激活的旧 Actor/timer/context。

`franz-go` 集成必须把客户端事件确定性映射到上述状态：

- `OnPartitionsRevoked(p)`：受影响 partition 先进入 `REVOKING` 并失效 token，再执行 drain；
- `OnPartitionsLost`、group session context cancellation、member/generation 被 fence，或主动
  close/recreate Consumer：所有受影响 partition 先进入 `SUSPENDED` 并失效 token；
- `OnPartitionsAssigned(p)`：只能从无旧激活权限的 context 创建新 token 并进入 `RESTORING`；
- callback adapter 必须保证 lost/revoke 的 token 失效先于后续 assigned callback，不能让客户端
  自动 rejoin 在应用层沿用旧 token。

上述 callback 只做原子状态/token 切换、pause 和登记后续工作；完整 drain、state restore、Actor
重建均在 callback 外执行，不能阻塞 Consumer Group 协调循环。`OnPartitionsAssigned` 只创建
`RESTORING` context，不在 callback 内同步读取完整 state partition。

普通 broker request 的瞬时失败不单独证明 session 已丢失；在客户端尚未通知 lost 的窗口内仍属于
V1 best-effort 风险。POC 必须覆盖 Kafka 隔离、错过 revoke、自动 rejoin 和 callback 乱序注入，
验证上述先后关系，而不能只测 graceful revoke。

## 9. 正常处理与完成边界

1. Consumer record 进入对应 `PartitionRuntimeContext`，携带 topic、partition、offset 和 token。
2. Manager 验证 ProcessingContext，在同一个完整 ProjectIndex read view 中完成 matching 和 target
   bundle 聚合；Manager 是 Scene Runtime 内部组件，不是第三套 Runtime 或 partition owner。
3. 同 subject target 直接进入当前 Actor lane；跨实体 target 经 Kafka 重新路由到目标 partition。
4. Actor lane 串行应用完整 bundle，每个 Binding 最多求值一次，形成 tentative state/timer/action。
5. 任何会推进 state 的分支都先规范序列化 tentative state，并校验点数、timer state 和 32 KiB
   配额；失败时不写 Outbox、不发布内存状态、不注册 timer，也不发送 state。
6. 有动作时，才在一个 PG 本地事务中插入本轮全部 Action Outbox rows；commit 失败不发布
   tentative state，并按主规格 retry/quarantine/最终失败策略处理。无动作的参数/FSM 更新跳过 PG。
7. 校验成功且有动作分支已 commit 后，才发布 Actor 内存 state 并注册/取消 timer；有 state 变化时
   对步骤 5 已校验的完整 `scene_state` 做一次有界异步 best-effort handoff。无 state 变化且无动作
   的求值是 no-op。
8. per-partition completion sequencer 只有在更早 input 都达到下述完整谓词后，才推进连续
   next-offset；不同 partition 的 sequencer 不能合并。

同一 partition 内可以并发运行不同 Actor lane，Kafka record 的物理顺序不等于整个 partition
单线程执行；completion sequencer 负责防止较晚完成的 offset 越过较早未完成的 offset。

完整 input completion 谓词必须迁入最终分章，不能只写“按主规格处理”：

- 本 input 的全部候选已完成求值，或已进入批准的最终失败策略；
- 实际 Action Outbox 已 commit，或 Outbox 失败已完成 retry/quarantine/最终失败处置；
- Actor 内存 state/timer 已按上述顺序发布；
- SOURCE、Linked、LifecycleTarget 等所有非-state data-event pack 已完成 callback、有界 retry，
  或在耗尽后完成 quarantine/批准的最终失败处置；
- `scene_state` 只要求规范序列化成功并完成一次有界 producer handoff attempt：立即拒绝被记录，
  producer 接受后不等待 broker ACK/callback，后续异步失败不反向阻塞 input offset；
- 只有连续的 earlier next-offset 全部完成，才能提交当前 next-offset。

该 completion 不表示 Webhook、通知或 DeviceCommand 已由外部系统执行，也不构成 state、Outbox 与
input offset 的 PG/Kafka 原子事务。

## 10. Assignment 与 state-before-event restore

新的 partition owner 必须按以下顺序激活：

1. 确认全量配置缓存和 ProjectIndex ready；
2. 为 partition 创建新的 `PartitionRuntimeContext/token`，状态为 `RESTORING`；
3. 捕获对应 `scene_state` partition 的当前 available start offset 与恢复 end offset `S0`，
   读取区间 `[start, S0)` 并应用到 next-offset 达到 `S0`；`S0` 之后才到达的记录不属于本轮
   恢复切面；
4. 在 staging 中用 current BindingCtx 校验和恢复每条兼容 BindingState；坏 key 被隔离，已删除、
   禁用或语义不兼容的 state 不进入 staging，并通过 `RestoreMaintenanceGate` 异步补纯 tombstone；
5. 构造 Actor transient 对象，但只收集待激活的 timer descriptor，不在 `RESTORING` 中暴露会立即
   fire 的 handle；membership-dependent state/timer 标记 `scopeReconcileRequired`，建立“完成
   current-scope reconciliation 前零副作用”的门禁，不要求开放 partition 前全量查询成员；
6. next-offset 达到 `S0` 后再次校验 assignment/token。若已丢权或恢复需重启，丢弃全部 staging，
   不与下一轮恢复拼接；下一次 assignment 重新捕获 start/`S0`；
7. 在 partition activation 临界区内发布 staging Actor/state，安装尚未到期的 timer descriptor，
   并把已经 due 的 timer 放入不可丢弃的 activation queue；随后切换为 `ACTIVE`，以新 token 将
   activation queue 依次投到对应 Actor lane。timer batch 安装完成后才打开 restore gate、unpause
   对应 `scene_event` 业务 dispatch；激活期间到期的 callback 必须留在 queue，不能因状态尚非
   `ACTIVE` 而 no-op 丢弃。

即使同一 Runtime 在重连后再次获得相同 partition，也必须创建新 token 并执行上述 restore，不能
续用失联前的 Actor、timer、offset sequencer 或内存状态。不同 partition 独立恢复；一个新增
partition 的 restore 不得在应用层阻塞仍为 ACTIVE 的其他 partition。

`S0` 之后到达的旧 owner state 不追赶进本轮 staging，也不不断延长恢复边界；它属于 §12 的迟到
state 风险。以上定义的是待实施和验证的恢复协议，不表示 restore RTO、compaction 行为或故障 POC
已经通过；最终分章和 README 必须继续保留“恢复 ADR 未冻结”。

## 11. 三类故障转移

### 11.1 Graceful revoke

旧 owner 收到 revoke 后：

1. 原子切换为 `REVOKING` 并立即失效 token；
2. 停止接收该 partition 的新 record；
3. 禁止新计算、Linked、Outbox、state 和 timer fire；
4. 取消可取消任务，关闭该 partition 的 Actor/timer，并 bounded drain；
5. 按既有 per-partition completion 规则处理可安全提交的连续 offset；
6. 销毁 context，进入 `DETACHED`。

bounded drain 不是严格屏障，已提交到 Kafka Producer 或 PG 的请求不能撤销。

### 11.2 原 Runtime 硬故障

故障转移不由原 Runtime A 发起。Kafka Consumer Group Coordinator 在 A 心跳超时后移除 A，
重新分配 partition；新 owner B 按 §10 restore 后激活。A 的进程内 Actor/timer 是否完成清理不构成
B 接管前提。

### 11.3 Zombie A 重连

A 的网络恢复只表示可以连接 Kafka，不表示恢复旧 ownership。A 必须以新的 Consumer Group
session rejoin，且在 rejoin 或重建 Consumer 之前先使所有旧 partition token 失效。未获得新
assignment 的旧 context 永久关闭；重新获得相同 partition 也按新 token 完整 restore。

Consumer Group 能约束 assignment 和旧 generation 的 offset commit，但不会自动 fence Kafka
Producer、PG 事务、已取到的 record 或本地 timer。Producer 恢复连接时也不会询问它是否仍是
该 partition 的 Consumer owner。

## 12. V1 best-effort fencing 与幂等边界

V1 冻结为本地 best-effort fencing：

- 一旦 Runtime 得知 revoke、session lost 或 ownership 无法证明，新的 Actor/timer/continuation、
  Linked producer、PG Outbox 入口和 state producer 入口都由 status + token gate 拒绝；
- ownership 不确定时宁可暂停该 partition，也不继续控制设备；
- 不新增持久 owner epoch、lease、Kafka transactional producer 或跨 PG/Kafka transaction。

它不能撤销：

- Runtime 尚未察觉丢权期间已经产生的工作；
- revoke/suspend 前已进入 Kafka Producer 的 Linked/state；
- 已进入或已提交的 PG Outbox 事务；
- 已经被下游接受的外部动作。

幂等只吸收“同一逻辑效果的重复”，不掩盖状态语义错误：

| 对象 | 收敛手段 | 不承诺的内容 |
| --- | --- | --- |
| Action | 稳定 `transitionId/effectId`、PG unique key、下游按 `effectId` 幂等 | 非幂等下游的物理 exactly-once |
| Timer | 稳定 `timerFireId`、current-slot、Binding 和 token gate | 已接受 delayed action 的自动撤销 |
| Linked/lifecycle | 稳定事件身份、member/aggregate version 与 current-scope guard | 跨 Topic 事务屏障 |
| `scene_state` | 完整快照、后续合法 state、reconciliation/reinit | 旧 send 绝不迟到，或 compacted latest 绝不回退 |

旧 state 在新 owner state 之后到达，可能使 compacted latest 回退，并在未来 restore 后产生错误新
transition；这不是“同一 effect 重复”，不能宣称由 `effectId` 完全吸收。它保持为必须由故障注入、
监控、current fact reconciliation 和显式 reinit/补偿量化的 V1 风险。

## 13. A/B/p0/p1 接管示例

```text
初始：A owns p0；B owns p1

A hard failure
  → Coordinator expires A
  → B receives p0 assignment
  → B creates PartitionRuntimeContext[p0, token=b0]
  → B restores scene_state-p0
  → p0 ACTIVE；若 assignor 未 revoke p1，原 p1 保持独立 ACTIVE

A reconnects
  → A invalidates all old tokens before rejoin
  → Coordinator may assign p0、p1、其他 partition 或 none
  → 若 p0 从 B 转出：B 只关闭 revoked p0 context
  → 新 owner以新 token恢复 p0；没有 B→A Actor 内存搬运
```

具体客户端基线由 Kafka 规范 §7.2 拥有，本章直接继承 `franz-go + classic group +
cooperative-sticky`，不建立第二个选择点。cooperative-sticky 减少未迁移 partition 的暂停，但正确性
仍不能依赖“原 partition 一定还给 A”。未来若 Kafka 规范改变 assignor，必须联合验证本章状态机；
即使某策略暂时 revoke 多个 partition，也必须分别执行各自的 token 失效和恢复，不能合并 context。

## 14. 文档迁移策略

新分章成为调度协作协议的唯一正文拥有者；主规格保留原 H2/H3 标题、短摘要和稳定锚点，不保留
第二份完整算法。迁移边界如下：

| 现有准确位置 | 主规格保留/改法 | 新分章目标位置与唯一正文 |
| --- | --- | --- |
| §4 Manager/Actor 总体流 | 保留一张总图和职责摘要，调度箭头改为链接 | 新分章 §3～§5：组件、lane 与正常调度 |
| §5.2 SOURCE handler | 保留 wire、SubjectContext 与 matching 业务门禁 | 新分章 §5：record→Manager 的 partition/token 上下文 |
| §5.3～§5.5 target/lifecycle handler | 保留 payload、完整 bundle、applicability 与业务 no-op | 新分章 §5：local lane 与跨 partition Kafka 递交 |
| §6.2、§6.3 末段、§6.4 启动门禁 | 保留 ProjectIndex 全复制、配置 ready 与 cleanup 业务结果 | 新分章 §3、§6～§8：ownership、context registry 与激活 |
| §8.1 首尾、§8.2 共置 bullet | 保留完整 state schema、Topic value、ACL 和配额 | 新分章 §2～§4：Actor/BindingState 层级和 partition 隔离 |
| §9.1 | 改成 state-before-event 摘要和稳定链接 | 新分章 §7：完整 assignment/restore/activation 算法 |
| §9.2～§9.5 | 保留 state 有效性、scope reconciliation、缺失/损坏和冷启动语义 | 新分章只引用，不复制类型化业务表 |
| §10.1 callback/generation 段 | 保留 timer identity、业务 slot 与 fire 门禁 | 新分章 §5、§7：timer 归属、回 lane 和恢复激活队列 |
| §11.1、§11.3 | 保留 Schedule/scope/lifecycle 业务语义 | 新分章 §5：target partition 递交与 revoke close |
| §12.3 | 保留 Action Outbox PG 事务及 tentative state 发布顺序 | 新分章 §5 只引用该事务，并拥有其与 completion 的衔接 |
| §13.2 | 改成交付承诺摘要和稳定链接 | 新分章 §5：完整 per-partition completion predicate |
| §13.3 | 改成 best-effort 承诺摘要和稳定链接 | 新分章 §6、§8～§9：revoke、Zombie、token 与残余窗口 |
| §14.1 | 保留 route bytes/hash、固定 count；动态分配只留摘要 | 新分章 §2、§10：ownership 不绑机器和 Kafka 基线引用 |
| §14.2 | 保留 Topic generation 扩容协议 | 新分章只解释 `sceneTopicGeneration` 并链接，不复制 cutover |
| §15～§16 | 容量公式、配额和指标定义 | 只引用这些容量/SLO，不复制公式和阈值 |
| §17 | ThingsBoard 对齐证据和差异摘要 | Actor/partition/rebalance 的规范算法改为链接新分章 |
| §18/§18.1 | 跨目录一致性历史与处置矩阵 | 权威声明改为“总纲 + 调度分章”；不迁移历史矩阵 |
| §19 | POC/ADR 门槛继续由主规格和 SLO 拥有 | 引用 Zombie、restore、late-send 的被测协议，不声称已通过 |
| §20 | 保留已拒绝的 lease、owner epoch、EOS 等方案 | 链接 best-effort fencing 边界，不复制论证全文 |
| §21 | 保留总括结论 | Manager/Actor/partition/failover 只留摘要和新分章入口 |

同一次实施必须：

- README 增加新分章索引，并把“一个唯一规范文件”改为“主规格总纲 + 调度分章规范集”；
- 主规格 §4～§14 按上表迁移算法，§17～§21 按上表修订引用和权威声明；
- `00-15分钟读懂场景联动.md`、`控制面与匹配契约.md`、`算子语义与术语.md`、
  `SLO故障测试与实施.md` 的阅读卡、权威声明和直接章节链接改指唯一正文；
- 父架构 §11.1 的 Scene 规范入口改为总纲 + 调度分章；
- Kafka 设计文件头/§0、§1.2、§5.3、§5.5、§6.1～§6.2、§8 前言及 §8.2～§8.5 中指向
  单一主规格的入口，以及文中所有 `场景主规格 §...` 直接引用全部逐项迁移；“当前规范性算法
  只存在于主规格”的文字必须删除或改成总纲 + 调度分章职责引用；Kafka §7.2 客户端基线保持不变；
- 两份非规范性 ThingsBoard/竞品附录只检查阅读卡和直接链接；研究证据不升级为规范，也不因拆章
  回写结论；
- 不回改 `docs/superpowers/specs/`、`plans/` 中的历史决策记录；稳定旧锚点保证历史链接可读。

## 15. 目标分章结构

最终规范性分章采用以下结构：

1. 范围、权威关系与不承诺项；
2. 一分钟模型与稳定身份；
3. Manager、PartitionRuntimeContext、Actor lane、BindingState 的职责；
4. 多 partition 共享与隔离；
5. 正常消息调度、timer 和 completion；
6. PartitionRuntimeContext 状态机；
7. assignment 与 state-before-event restore；
8. graceful revoke、硬故障和 Zombie 重连；
9. best-effort fencing、幂等与残余窗口；
10. 固定 partition、动态分配和扩容引用；
11. 可观测性、故障注入与实施门槛；
12. 与主规格各章节的职责索引。

正文至少包含一张标准故障转移时序图和一张 PartitionRuntimeContext 状态图。图必须把 Kafka Coordinator、
旧 Runtime A、新 owner B、`scene_state` 和 `scene_event` 画成不同参与者；不能把 Topic、程序节点和
状态存储合并成抽象方框。

## 16. 验收条件

- 新分章可以独立回答“一进程多 partition 是否混 state”和“A 恢复后谁决定分区归属”；
- 明确硬故障不由旧 Runtime 发起，Zombie 重连不恢复旧 assignment；
- 每个 partition 独立 token、restore gate、completion、Actor/timer index 和 revoke 生命周期；
- `SUSPENDED`/`REVOKING` 均先失效 token，再进行 rejoin、drain 或清理；
- 旧任务捕获旧 token，不能借用新 assignment 的 token；
- 明确 best-effort fencing、动作幂等和 stale state 风险三者边界；
- 没有新增 durable owner epoch、lease、Kafka EOS、Actor snapshot 或第三方 Actor 框架；
- 主规格旧锚点仍存在，完整协议只有一个正文拥有者；
- README 和所有活跃规范的权威声明、相对链接一致；
- Mermaid 可渲染，Markdown/link 检查通过，恢复 ADR 仍标为未冻结。

### 16.1 机械检查

实施计划必须把以下检查写成可执行断言：

```text
1. 目标分章存在，且 §1～§12 标题各恰好一次；README 到该分章的入口恰好一次。
2. 主规格原 §4、§5、§6、§8、§9、§10、§12、§13、§14、§17～§21 标题仍存在。
3. 活跃规范中以下旧权威说法为 0：
   - “主规格是唯一的规范性架构决策源”
   - “当前规范性算法只存在于场景主规格”
   - “Scene 的全部调度/恢复算法以单个主规格文件为唯一来源”
4. 为 §14 迁移矩阵逐行建立“旧位置允许保留内容 + 必须迁出的 marker + 新位置 heading”断言，
   至少包括：
   - §4 只保留总体数据流图，不新增 PartitionRuntime 状态机正文；
   - §5.4 的“Manager 直接向当前 Actor 递交/跨实体回写”调度算法迁入新分章，wire 与
     applicability 语义仍保留；
   - §6.2/§6.3 的 active-state ownership、`localPartitionGeneration` 执行算法迁入新分章，
     ProjectIndex 与 cleanup 业务结果仍保留；
   - §9.1 六步 restore block 被摘要 + 链接替换，完整 `[start,S0)`/staging/activation 只在新分章；
   - §10.1 的 callback 回 lane 与 partition token 调度迁入新分章，timer identity/current-slot
     业务门禁仍保留；
   - §13.2 完整 completion predicate、§13.3 完整 generation/revoke/Zombie 算法只在新分章；
   - §14.1 route/hash 仍在主规格，动态 assignment 的状态机只在新分章。
   每个改为摘要的旧 H2/H3 必须恰有一个目标分章链接，且上述完整算法 marker 在旧位置为 0。
5. 逐一解析本次修改的 Markdown 相对链接，目标文件和 GFM heading 均存在。
6. 抽取目标分章全部 Mermaid block，以固定 Mermaid CLI 版本逐图渲染成功。
7. implementation plan 必须在独立、初始 clean 的 worktree 执行并记录任务 `BASE_SHA`；最终验收前
   先提交全部任务变更，`git status --porcelain` 必须为空。随后 `git diff --check BASE_SHA..HEAD`
   通过，`git diff --name-only BASE_SHA..HEAD` 只包含 §14 列出的活跃规范、目标分章和
   implementation plan，不包含产品代码或历史 HTML；禁止用只看 commit range 的空结果掩盖
   staged、unstaged 或 untracked 文件。
8. 占位符扫描只覆盖目标分章与本次实际修改的活跃规范 Markdown，不扫描作为历史记录保留的
   本设计稿/plan；结果为 0。恢复 ADR“未冻结”声明在 README、主规格、目标分章和 SLO 中各可定位。
```

### 16.2 有界故障场景

除现有 graceful rebalance 用例外，验收必须覆盖：

- B 已持有 `p1` 时接管 `p0`，两个 context 的 generation、restore、completion、Actor/timer 不混用；
- A 硬故障，无 A 参与时 Coordinator 把 `p0` 分配给 B；
- A 错过 revoke 后重连，lost/session callback 先使旧 token 失效，再处理新 assignment；
- A 重新获得原 `p0` 时仍丢弃旧 Actor/timer，使用新 token 完整 restore；
- restore 中再次 revoke，staging 和 timer activation queue 全部丢弃，其他 ACTIVE partition 不受影响；
- 已 due timer 在 activation 期间不丢失，且 scope reconciliation 失败时 zero effect；
- `RESTORING` 只允许隔离和纯 tombstone，任何非 tombstone state、Linked、Outbox 或求值均被拒绝；
- revoke 前已 handoff 的 state 迟到覆盖、新旧 owner 同 effect 重算分别产生可审计证据，不能把前者
  错报为已被幂等完全吸收。
