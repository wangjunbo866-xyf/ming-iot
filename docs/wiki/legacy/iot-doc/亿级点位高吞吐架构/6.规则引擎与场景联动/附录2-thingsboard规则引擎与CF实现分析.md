# 附录2：ThingsBoard Rule Engine 与 Calculated Fields 实现分析（定义态/运行态的存储与恢复）

> **非规范性研究资料。**
>
> 导航：[README](README.md) | [现行主规格总纲：场景联动 Runtime 设计](场景联动%20Runtime%20主规格总纲.md) | [Runtime 调度、分片与故障转移分章](Scene%20Runtime%20调度、分片与故障转移.md#runtime-scope) | [附录：竞品状态方案调研](附录-竞品状态方案调研.md)
>
> 原调研日期与源码基线：2026-07-28；本地 `thingsboard/` 仓库 master `b94811000c`（2026-07-17，当时为 4.3 开发线）。官方文档以原调研日查阅的 PaaS EU 用户指南及 monolithic/msa 架构页为准，本次未联网更新。
>
> 调研动机：源码级核对 ThingsBoard Rule Engine 与 Calculated Fields 的定义态、运行态、持久化和恢复，为现行 TB-style 场景设计提供第三方证据与容量参照，但本文不是项目决策源。
>
> 规范性边界：本文不是本项目的架构决策源；现行架构决策按职责位于上述主规格总纲与 Runtime 调度分章。两份架构源没有优先级或“后写覆盖”关系，任何冲突都必须联合修订。方法上逐条 grep/read 核实，并区分「源码实证」「文档明说」「推断/未找到」。

## 0. ThingsBoard 研究结论

1. **TB 4.x 有两套独立体系**：Rule Engine（规则链，2.x 至今）与 Calculated Fields（4.0 新增）。规则链负责消息路由与动作编排，**运行态基本全在 JVM 堆内，重启即丢**；CF 则持久化自身运行态并提供恢复流程。两者在 tb-rule-engine 服务内并存，CF 输出可选地作为消息重新进入规则链。
2. **规则链的「定义态在 PG、运行态在堆内、可靠性靠 Kafka 重放」**：定义存 `rule_chain`/`rule_node`/`relation` 三表，启动全量急切加载进 actor 树；消息侧整 pack 处理完成才 `commitSync`（at-least-once）；节点内存状态（dedup/delay/计时器/计数器）无任何持久化与交接，`DefaultTbContext.java:178` 留着作者 TODO。`rule_node_state` 表全库仅被**已废弃**的 `TbDeviceProfileNode` 用于告警 duration 计时，且两个开关默认关闭。
3. **CF 的状态模型 = 每 (cfId, entityId) 一棵状态树，每次变更即全量持久化**：没有周期 checkpoint，每条触发计算的消息处理完，在 finally 块把整个 `CalculatedFieldState` 序列化成 Protobuf 全量写出。Kafka 部署写 compacted changelog topic `tb_cf_state`（删除用 tombstone）；in-memory 单体部署写本地 RocksDB（sync 写）。
4. **CF 使用先 state 后 event 的恢复协议**：partition 变更后，先从头读完 state 分区，重建 actor 内存状态并执行 `init(true)` 恢复钩子（聚合补区间、propagation 与 DB 对账、告警重评估）；对应 state 分区恢复完成后，才开始消费事件分区。
5. **at-least-once 路径未见通用去重**：CF 与规则链都靠 Kafka 重放兜底，崩溃窗口内「已计算/已输出但 offset 未提交」的消息会被重放重算。源码中未见通用幂等/去重机制；告警类输出另由 `alarm` 表查询 originator+type 下已有活动告警。

## 1. 两套体系的定位与关系

### 1.1 Rule Engine（规则链）

- 消息驱动：一切入口流量（遥测、属性、生命周期事件、RPC）先进队列再进根规则链。消息为不可变结构 `{msgId(time-based UUID), originator, type, payload(JSON), metadata}`（[官方文档](https://thingsboard.io/docs/paas/eu/user-guide/rule-engine/) 明说）。
- 规则链 = RuleNode 有向图，节点类型即 Java 类全名；每租户一个 Root Rule Chain，Device Profile 可覆盖默认链与队列。
- 处理结果三态：Success / Failure / Timeout；失败沿 FAILURE relation 路由，无 FAILURE 出边则整条消息判失败，回到队列层按队列的 ProcessingStrategy 重试或丢弃（`ProcessingStrategyType.java:19`：SKIP_ALL_FAILURES / RETRY_ALL / RETRY_FAILED / RETRY_TIMED_OUT / RETRY_FAILED_AND_TIMED_OUT / SKIP_ALL_FAILURES_AND_TIMED_OUT）。

### 1.2 Calculated Fields（CF，4.0+）

- 定位：在「保存遥测/属性」路径上挂接的**有状态派生计算**——滑动窗口、脚本聚合、地理围栏、跨实体聚合、告警规则。类型枚举见 `CalculatedFieldState.java:46-54`（SIMPLE/SCRIPT/GEOFENCING/ALARM/PROPAGATION/RELATED_ENTITIES_AGGREGATION/ENTITY_AGGREGATION）。文档明说 4.0 引入、4.3 增加聚合与地理围栏类型。
- 触发入口：`DefaultTelemetrySubscriptionService` 在 save timeseries/attributes 后调 `calculatedFieldQueueService.pushRequestToQueue(...)`（`application/.../service/telemetry/DefaultTelemetrySubscriptionService.java:172,208,274`）→ 匹配相关 CF → 写专用事件 topic `tb_cf_event`（`DefaultTbClusterService.java:356`，`DataConstants.java:147` `CF_QUEUE_NAME="CalculatedFields"`）。规则链内也有专用节点 `TbCalculatedFieldsNode`（`rule-engine-components/.../TbCalculatedFieldsNode.java:42-55`），可让消息**绕过落库**直接触发 CF。
- 输出两路（`DefaultCalculatedFieldProcessingService.java:145-156`）：IMMEDIATE 策略直接写 ts_kv/attributes（不进规则链，携带 `previousCalculatedFieldIds` 防 CF 自触发循环，`AbstractCalculatedFieldProcessingService.java:426-434`）；RULE_CHAIN 策略或告警结果封装为 TbMsg 送入规则链（`:406-424`）。
- 链式执行防环（文档明说）：一个 CF 的输出可触发下一个 CF，但同一 CF 在一次执行链中出现两次即截断。

### 1.3 部署形态

- Monolith（`queue.type=in-memory`）：全进程内，CF 状态走 RocksDB；官方文档明说内存队列崩溃丢消息。
- 微服务：CF 组件全部标注 `@TbRuleEngineComponent`（`DefaultTbCalculatedFieldConsumerService.java:72`），跑在 **tb-rule-engine** 服务；tb-core 只负责把遥测写入 `tb_cf_event`。官方 msa 文档明说：*"Every service is stateless — all persistent state lives in the database and cache"*，节点故障后 Zookeeper 检测 → partition 重分配 → actor 从存储状态重建 → 从 Kafka 分区继续消费。

## 2. Rule Engine：定义态怎么存，运行态怎么办

### 2.1 定义态存储（PostgreSQL 三表）

| 内容 | 存储 | 证据 |
| --- | --- | --- |
| 规则链 | `rule_chain` 表：`id/tenant_id/name/type/first_rule_node_id/root/debug_mode/configuration/additional_info/version`（乐观锁） | `dao/src/main/resources/sql/schema-entities.sql:160-175`；保存时校验 version（`BaseRuleChainService.java:198-199`） |
| 规则节点 | `rule_node` 表：`id/rule_chain_id/type（Java 类全名）/name/configuration(json)/configuration_version/debug_settings/singleton_mode/queue_name/additional_info`；画布坐标在 `additional_info` | `schema-entities.sql:177-190`；`BaseRuleChainService.java:296-299` |
| 节点连线 | **不在** rule 表里，存通用 `relation` 表：链→节点为 type=CONTAINS、group=RULE_CHAIN；节点→节点的 relation type 就是 UI 连线类型（"Success"/"Failure"…）、group=RULE_NODE | `BaseRuleChainService.java:252-253, 277-283` |
| 链间连线 | 物化为一个 type=`TB_RULE_CHAIN_INPUT_NODE` 的特殊 rule node + 两条 relation | `BaseRuleChainService.java:285-316` |
| `RuleChainMetaData`（connections/ruleChainConnections/firstNodeIndex） | **纯 API 组装视图**，由 rule_node + relation 反查重建，不落库 | `common/data/.../rule/RuleChainMetaData.java:32-54`；`BaseRuleChainService.java:348-387` |

要点：TB 把「画布模型」与「存储模型」解耦——存储侧只有节点集合 + 通用关系表，UI 的 connections 是查询时重建的投影。

### 2.2 加载：启动全量急切加载，变更靠广播

- 启动：`ApplicationReadyEvent` → 广播 `APP_INIT_MSG`（`DefaultActorService.java:127-131`）→ AppActor 为所有租户建 TenantActor（`AppActor.java:77-79`）→ 本进程是 TB_RULE_ENGINE 且该租户 partition 归本节点时 `initRuleChains()`（`TenantActor.java:95-121`）→ **分页遍历该租户全部 CORE 链逐条建 actor**（`RuleChainManagerActor.java:61-71`）→ 链 actor start 时读全量节点建子 actor 并用 relation 建路由表 `nodeRoutes`（`RuleChainActorMessageProcessor.java:98-116, 165-198`）→ 节点 actor 反射实例化节点类并 `tbNode.init(ctx, config)`（`RuleNodeActorMessageProcessor.java:65-73`）。
- 变更下发：保存规则链 → `broadcastEntityStateChangeEvent`（`DefaultTbRuleChainService.java:267-270`）→ `ComponentLifecycleMsgProto` 发所有 RE 实例的 notifications topic（`DefaultTbClusterService.java:597-636`）→ 链/节点 actor 收到后重新读库 diff：新节点建 actor、配置变化的节点 **destroy + re-init**（内存状态随之重建）、删除的节点收 DELETED 消息（`RuleChainActorMessageProcessor.java:119-148`；`RuleNodeActorMessageProcessor.java:76-99`）。
- 注意：`dao/rule` 下无 `@Cacheable`，链/节点读取每次直查 DB，靠 actor 常驻内存消化读放大。

### 2.3 actor 模型与消息可靠性

- 层级：AppActor → TenantActor → RuleChainActor（每链）→ RuleNodeActor（每节点），跑在 work-stealing 的 RULE_DISPATCHER 池。
- 消息流：Kafka pack → `forwardToRuleEngineActor`（`TbRuleEngineQueueConsumerManager.java:180-210`）→ TenantActor 按 ruleChainId 分发 → 链 actor 投 firstNode → 节点 `tbNode.onMsg` → `ctx.tellNext` 回链 actor 路由；**跨 partition 出边自动把消息复制回 Kafka**（`RuleChainActorMessageProcessor.java:365-387`）。
- **offset 提交 = 整 pack at-least-once**：poll 一 pack → 提交全部消息 → 等 `packProcessingTimeout` → 按队列 ProcessingStrategy 决策 → `decision.isCommit()` 才 `commitSync`（`TbRuleEngineQueueConsumerManager.java:133-170`；`TbKafkaConsumerTemplate.java:221-225`）。单条消息要走完所有下游分支（fork 的多条出边由 `MultipleTbQueueTbMsgCallbackWrapper` join，`RuleChainActorMessageProcessor.java:330-363`）才算成功。官方队列文档明说同一语义：*"If all succeeded — ack the pack and commit the queue offset. If any failed/timed out — loop back to submit strategy to resubmit only the failed messages."*
- Singleton 节点：非本 partition 时不实例化 TbNode，收到消息写回其所属 partition 的队列并 ack 源消息（`RuleNodeActorMessageProcessor.java:192-217`）。

### 2.4 运行态：默认全内存，唯一例外已废弃

| 节点/机制 | 状态形态 | 持久化与恢复 |
| --- | --- | --- |
| dedup（`TbMsgDeduplicationNode`） | `HashMap<EntityId, DeduplicationData>` + 内存 self-tick 超时（`:73,233-235`） | 无，重启全丢；**先 ack 再暂存**，丢的消息不会从 Kafka 重放 |
| delay（`TbMsgDelayNode`，已 deprecated） | `HashMap<UUID, TbMsg>`（`:52,57`）；nodeDetails 自己声明 ack 后只留在内存不保证处理（`:41-44`） | 无 |
| 计数（`TbMsgCountNode`） | `AtomicLong` + `lastScheduledTs`（`:48-53`） | 无，重启清零 |
| delta（`CalculateDeltaNode`） | SOFT 引用内存缓存 | 缓存丢后回源 ts_kv_latest（`:123-126`），语义上可恢复 |
| 告警 duration/repeating（`TbDeviceProfileNode`，4.x 已 @Deprecated） | `ConcurrentHashMap<DeviceId, DeviceState>`，内含 `AlarmRuleState{lastEventTs, duration, eventCount}`（`AlarmRuleState.java:60-76`） | **全库唯一写 `rule_node_state` 的节点**，且 `persistAlarmRulesState`/`fetchAlarmRulesStateOnStart` **默认均 false**（`TbDeviceProfileNodeConfiguration.java:27-28`） |
| 所有 `tellSelf` 定时器 | actor system 的 `ScheduledExecutorService.schedule`（`ActorSystemContext.java:932-940`） | 无；`DefaultTbContext.java:178` 作者亲留 `//TODO: add persistence layer` |

`rule_node_state` 表（`schema-entities.sql:192-201`）：`(rule_node_id, entity_type, entity_id, state_data varchar(16384))`，唯一键 `(rule_node_id, entity_id)`。内容为 JSON 序列化的 `PersistedDeviceState{alarmStates: Map<alarmType, {createRuleStates, clearRuleState}>}`（`profile/state/PersistedDeviceState.java:24-27` 等，均随节点标 @Deprecated）。写法是**每次遥测/属性/事件处理后若状态变化即同步写一行**（`DeviceState.java:181-184, 216-219`）；读法是节点 init/partition change 时分页拉全量按 `isLocalEntity` 过滤重建（`TbDeviceProfileNode.java:83-105, 153-157`）。grep 确认 `saveRuleNodeState` 的调用方只有 `TbDeviceProfileNode`/`DeviceState`——**规则链体系没有任何其它节点使用该表**。

### 2.5 重启/迁移恢复：靠 Kafka，不靠状态

- 进程重启：actor 树从 PG 重建定义态（§2.2），消息从未提交 offset 重放（at-least-once）；所有节点内存状态丢失，**无 handoff、无 shutdown checkpoint**——actor 系统 `@PreDestroy` 只是线程池 shutdown + 等 3 秒 + `actors.clear()`（`DefaultTbActorSystem.java:216-229`）。
- partition 迁移（微服务扩缩容/故障）：`PartitionChangeMsg` 广播 → 不再管理该租户的节点 `destroyRuleChains()`（`TenantActor.java:282-315`），新节点重建 actor；除 deprecated 的 device profile 节点会从 `rule_node_state` 重拉外，**内存状态直接随 actor 销毁丢弃**。
- 告警 duration 的丢失语义（默认不开持久化时）：重启后 `eventCount/duration` 清零重计，且 `evalDuration` 只按遥测消息 ts 差值累计（`AlarmRuleState.java:228-246`）——**重启期间流逝的时间不计入 duration**，即停机会让 duration 告警"变慢"而不是误报。

## 3. Calculated Fields：状态怎么存、怎么恢复（重点）

### 3.1 定义态

`calculated_field` 表（`schema-entities.sql:927-941`）：`id/tenant_id/entity_id/type/name/configuration(varchar(1000000) JSON)/version/debug_settings/additional_info`，唯一约束 `(entity_id, type, name)`；`entity_id` 可是 DEVICE/ASSET，也可是 **DEVICE_PROFILE/ASSET_PROFILE**（profile 级定义、实体级实例化）。配置核心是 `arguments`（参数可引用本实体/关联实体的遥测、属性或 owner/relation 动态源）+ `expression`（TBEL/JS 脚本）+ `output`（TIME_SERIES/ATTRIBUTES × IMMEDIATE/RULE_CHAIN 策略）（`common/data/.../cf/configuration/BaseCalculatedFieldConfiguration.java:26-32`）。另有 `cf_debug_event` 表存调试事件（文档明说 debug 事件落库、默认 TTL 一周、每租户 5 万条/小时上限）。

### 3.2 执行链路与状态的内存表示

- actor 层：`CalculatedFieldManagerActor`（每 tenant，管 CF 定义与 entity 链接）→ `CalculatedFieldEntityActor`（每实体，`CalculatedFieldEntityMessageProcessor` 持有 `states: Map<CalculatedFieldId, CalculatedFieldState>`）。
- 状态接口 `CalculatedFieldState`（`service/cf/ctx/state/CalculatedFieldState.java:55-95`）：`getArguments()/getLatestTimestamp()/update()/performCalculation()/getPartition()`；基类 `BaseCalculatedFieldState`（`:41-55`）核心为 `Map<String, ArgumentEntry> arguments` + readiness + `partition`。
- 参数项即状态主体：`SingleValueArgumentEntry`（单值+ts）、`TsRollingArgumentEntry`（**滑动窗口 = `TreeMap<Long, Double> tsRecords` + limit + timeWindow**，`:42-44`；文档明说每 rolling 参数上限 1000 点）、`GeofencingArgumentEntry`、`PropagationArgumentEntry`、聚合类的 `AggIntervalEntry`（带 `lastArgsRefreshTs/lastMetricsEvalTs`）。
- 状态大小限额（文档明说）：tenant profile 限额 **state size = 32 KB/CF**；源码侧 `maxStateSize/maxSingleValueArgumentSize` 控制（`CalculatedFieldState.java:94-105`），**超限不写盘而是 `deleteState`**（`CalculatedFieldEntityMessageProcessor.java:572-586`）。

### 3.3 状态持久化：每次变更即全量写（无周期 checkpoint）

- **时机**：每条触发计算的消息处理完，在 `processStateIfReady` 的 **finally 块** `cfStateService.persistState(ctxId, state, callback)`（`CalculatedFieldEntityMessageProcessor.java:548-549`）。不是周期 checkpoint，是"算一次写一次、全量写"。
- **粒度与格式**：key = `(tenantId, cfId, entityId)`（`CalculatedFieldEntityCtxId.java:24-26`）；value = Protobuf `CalculatedFieldStateProto`（`common/proto/src/main/proto/queue.proto:928-939`，含单值/rolling/geofencing/alarmState/aggregationArguments 等全部分量），转换在 `CalculatedFieldUtils.java:103-144, 227`。
- **两种互斥实现**（公共入口 `AbstractCalculatedFieldStateService.java:55-62` 先查 size）：
  - **Kafka 模式**（`queue.type=kafka`）：`KafkaCalculatedFieldStateService` 把 proto 发到 **compacted topic `tb_cf_state`**（`:130-147`），删除 = null value tombstone（`:150-152`）；topic 配置 `cleanup.policy:compact, retention.ms:-1`（`thingsboard.yml:1900-1901`）。state consumer `readFromBeginning(true)` + `stopWhenRead(true)`、**不用 consumer group**（`KafkaTbRuleEngineQueueFactory.java:382-390`）。
  - **RocksDB 模式**（`queue.type=in-memory` 单体）：`RocksDBCalculatedFieldStateService` 写本地 RocksDB（默认 `~/.rocksdb/cf_states`，`thingsboard.yml:2101`），`WriteOptions.setSync(true)`（`CfRocksDb.java:27-33`）。

### 3.4 状态恢复：先 state 后 event 的两阶段协议

触发点是 **partition 变更事件**（启动分配 partition、重平衡、迁移都算）：`DefaultTbCalculatedFieldConsumerService.onPartitionChangeEvent` → `stateService.restore(queueKey, partitions)`（`:128-134`）。

Kafka 模式恢复流程（`KafkaQueueStateService.java:52-85`）：

1. 订阅 state topic 对应分区，**从头读到末端**——compacted topic 保证每个 key 读到的是最新状态，tombstone 即删除；
2. 每条 state 转成 `CalculatedFieldStateRestoreMsg` 高优先级发给目标 entity actor，`states.put(cfId, state)`（`CalculatedFieldEntityMessageProcessor.java:127-139`）；
3. 一个 state 分区读完 → 发 `CalculatedFieldStatePartitionRestoreMsg`，actor 对该分区所有 state 调 `state.init(true)`（`:141-148`）执行**恢复钩子**：
   - EntityAggregation：`fillMissingIntervals()` 补齐停机期间缺失的聚合区间（`EntityAggregationCalculatedFieldState.java:90-95`）——对应文档明说的 *"Reevaluation check interval = 60s — how often the system checks for and processes missed aggregation intervals"*；
   - Propagation：与 DB 对账实体集合（`PropagationCalculatedFieldState.java:67-80`）；
   - RelatedEntitiesAggregation：重新调度 reevaluation（`:90-95`）；Alarm：重评估；
4. **state 分区恢复完成后，才把对应的 event（`tb_cf_event`）分区加入消费**（`KafkaQueueStateService.java:74-80`）——恢复期间不处理新事件，避免"边恢复边算"。

RocksDB 模式：启动时 `cfRocksDb.forEach` 全量回放后走同一 restore 流程（`RocksDBCalculatedFieldStateService.java:62-84`）。

### 3.5 定义变更与参数级初始化

- **全新状态 / REINIT**：`createState → initState → fetchArguments` 从 DB 拉初始参数——单值取 `timeseriesService.findLatest`，rolling 窗口拉窗口内历史 ts（`CalculatedFieldEntityMessageProcessor.java:480-510`；`AbstractCalculatedFieldProcessingService.java:135-158,322-365`）。这是**参数级初始化**（把状态的输入补全），不是把历史重算一遍输出。
- **定义变更分级**（`CalculatedFieldManagerMessageProcessor.java:483-495`）：类型变 → `RECREATE`（删旧 state 重建）；状态相关配置变（`hasStateChanges`）→ `REINIT`（`state.reset()` + 重拉参数）；仅上下文变 → `REPROCESS/REFRESH_CTX`。
- **迟到数据**（文档明说）：聚合区间结束后 await timeout 内到达的迟到遥测触发该区间重算；**Produce intermediate result** 每 5 分钟为未完成区间产出中间聚合。

### 3.6 两个存疑/边界点（诚实标注）

- **Reprocessing（历史重算回填）**：官方文档明说 4.1+ 支持——*"applies the current calculation logic to historical telemetry... to backfill results for data that arrived before the field was created"*，以后台任务运行（Task Manager，pending/running/completed/failed 带进度）。但在 CE master 源码中**未找到**对应实现（代码里的 "reprocess" 属无关的 Job/Housekeeper 功能，`JobManager.java:32`）。判断：该能力很可能是 PE 专属或走另一条未识别的路径；在 CE 代码线上，"从 ts_kv 重建"只有 §3.5 的参数级初始化。**若要参考 reprocessing 设计，不能只看 CE 仓库。**
- **重复输出窗口**：持久化顺序是"算完即写 state、event offset 异步 commit"。崩溃点落在「state 已写/输出已发，offset 未提交」之间时，重启后该事件重放重算——at-least-once，**源码未见幂等/去重机制**；state 先写只是让重算基于最新状态，不防输出重复。TB 的兜底是告警类输出靠 `alarm` 表查已有活动告警天然判重（`TbCreateAlarmNode.java:97`）。

## 4. 正面回答：「Rule 的状态是怎么存储、怎么恢复的」

按三层区分（这正是 TB 文档不说清、源码才看得见的部分）：

| 层 | 存什么 | 存在哪 | 怎么恢复 |
| --- | --- | --- | --- |
| 规则**定义态** | 规则链/节点 JSON 配置、连线 | PG `rule_chain`/`rule_node`/`relation` 三表 | 启动全量急切加载进 actor 树；变更经 notifications topic 广播，节点 destroy+re-init |
| 规则链**运行态** | dedup/delay 缓冲、计数器、self-tick 定时器、（废弃节点的）告警 duration | JVM 堆内；唯一例外是 `rule_node_state` 表且默认关闭 | **不恢复**：Kafka 从未提交 offset 重放消息（at-least-once），节点状态重启即丢；告警 duration 重计且停机时间不计入 |
| CF **运行态** | 每 (cfId, entityId) 的参数树（单值/rolling TreeMap/告警中间态/聚合区间），≤32KB | Kafka compacted topic `tb_cf_state`（每次变更全量 protobuf 写入，tombstone 删除）；单体 in-memory 模式写本地 RocksDB sync | partition 变更时先从头回放 state 分区 → actor `init(true)` 恢复钩子（补区间/对账/重评估）→ 恢复完才消费 event 分区；参数级 REINIT 从 ts_kv 拉最新值与窗口历史 |

## 5. 研究观察与使用边界

1. **Kafka CF 路径是本次研究的主要状态参考**：`tb_cf_state` 以 compacted topic 保存每个 `(cfId, entityId)` 的完整 Protobuf state，变更后全量写，tombstone 表示删除；恢复时先完成 state 分区回放，再开放对应 event 分区。它提供了可核查的 state-before-event 实例，但第三方实现本身不是项目决策。
2. **RocksDB 只代表 TB 的单体部署事实**：`queue.type=in-memory` 时，TB 将 CF state 同步写入本机 RocksDB，并在启动时全量遍历恢复。该路径不应被解释为云端分区快照方案，也不能据此推出本项目需要 RocksDB。
3. **容量数据只作量级参照**：32 KiB/CF、每个 rolling 参数最多 1000 点，以及每次变更全量写 state，共同界定 TB CF 的状态与写放大边界。是否适用于其他负载，仍需按各自消息频率、状态大小和恢复 SLO 实测。
4. **Rule Engine 只提供行为证据**：定义态 PG、运行态 JVM 内存、pack 级 at-least-once 和 actor 重建解释了 TB 规则链的恢复边界；它不恢复或支持一个独立 RuleGo/RuleChain runtime 结论。
5. **本项目只作简短定位**：现行方案采用异步 Kafka compacted full `scene_state` 与 state-before-event；Scene 计算和动作编排合一，因此动作进入 PG Action Outbox，并用 `effectId` 收敛重复；Runtime 只有 Scene 与 Flink。具体语义、失败窗口与待验证项按职责见顶部主规格总纲与 Runtime 调度分章。

## 6. 关键证据索引

| 主题 | 文件（均相对 `thingsboard/` 仓库根） |
| --- | --- |
| 规则链定义存储 | `dao/src/main/resources/sql/schema-entities.sql:160-201`；`dao/.../rule/BaseRuleChainService.java:198-387` |
| 规则链加载/变更 | `application/.../actors/service/DefaultActorService.java:127-137`；`actors/ruleChain/RuleChainManagerActor.java:61-71`；`RuleChainActorMessageProcessor.java:98-198`；`RuleNodeActorMessageProcessor.java:59-108` |
| pack 提交与重试 | `application/.../service/queue/ruleengine/TbRuleEngineQueueConsumerManager.java:133-170`；`common/queue/.../kafka/TbKafkaConsumerTemplate.java:221-225` |
| 运行态内存化 | `application/.../actors/TbContext/DefaultTbContext.java:178-181`；`TbMsgDeduplicationNode.java:73`；`TbMsgDelayNode.java:41-57`；`TbDeviceProfileNode.java:83-157`；`dao/.../rule/BaseRuleNodeStateService.java:33-118` |
| CF 定义存储 | `schema-entities.sql:927-943`；`common/data/.../cf/CalculatedField.java:50-92` |
| CF 执行链路 | `DefaultTelemetrySubscriptionService.java:172-274`；`service/cf/DefaultCalculatedFieldQueueService.java:84-93`；`DefaultTbCalculatedFieldConsumerService.java:71-134`；`actors/calculatedField/CalculatedFieldEntityMessageProcessor.java:127-148, 480-586` |
| CF 状态模型 | `service/cf/ctx/state/CalculatedFieldState.java:46-105`；`BaseCalculatedFieldState.java:41-55`；`TsRollingArgumentEntry.java:42-44`；`common/proto/src/main/proto/queue.proto:928-939` |
| CF 持久化 | `KafkaCalculatedFieldStateService.java:55-152`；`RocksDBCalculatedFieldStateService.java:36-84`；`CfRocksDb.java:27-33`；`thingsboard.yml:1898-1901, 2087-2105` |
| CF 恢复 | `service/cf/ctx/state/KafkaQueueStateService.java:52-85`；`AbstractCalculatedFieldStateService.java:55-103`；`EntityAggregationCalculatedFieldState.java:90-95`；`PropagationCalculatedFieldState.java:67-80`；`CalculatedFieldManagerMessageProcessor.java:483-495` |
| 官方文档 | calculated-fields 主页（类型/限额/reprocessing/debug）、time-series-data-aggregation（迟到重算/中间结果/60s 漏算补偿）、rule-engine 主页与 queues 页（pack 提交语义/重试策略）、reference/monolithic 与 msa（无状态服务 + actor 重建 failover） |
