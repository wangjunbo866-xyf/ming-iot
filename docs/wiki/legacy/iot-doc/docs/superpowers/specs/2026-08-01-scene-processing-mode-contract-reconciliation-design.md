# Scene Runtime Processing Mode 契约补充修订设计

> 日期：2026-08-01
> 状态：已确认，联合修订已实施

## 1. 问题与目标

父架构把 Canonical `processingMode=LIVE|BACKFILL|REPLAY` 与
`processingNamespace` 定义为可信硬契约：只有生产 `LIVE` 可以进入 live
Automation，`BACKFILL` 必须在候选匹配前跳过，`REPLAY` 不得改变 live state
或产生正式外部副作用。

现行 Scene 主规格及三份从属契约没有把该约束落实到 Admission、
`SceneEvent`、`SourceDelta`、`LinkedDelta`、state/timer/Outbox 和测试门禁。
因此实现者可能把补传或回放 Canonical 当作实时事实求值。本次修订的目标是
恢复端到端 mode 隔离，同时不引入 Scene replay Runtime、Kafka EOS、RocksDB
或新的 Topic 拓扑。

## 2. 方案比较与选择

### 方案 A：只在 Admission 增加 mode 判断

改动最小，但 `scene_event` 下游无法证明输入为何被接纳，也无法在错误路由、
producer 缺陷或 quarantine 重投时进行第二道校验。Linked、timer 和 Outbox 也
失去 processing provenance。拒绝。

### 方案 B：只依赖 Topic、consumer group 和 ACL 隔离

可以阻止 replay principal 直接写生产 Scene Topic，却挡不住同一生产 Canonical
流中的 `BACKFILL`，也无法处理缺失、未知或伪造 mode。ACL 不是业务语义校验。
拒绝。

### 方案 C：端到端可信 ProcessingContext

Admission 校验 Canonical 的 `processingNamespace + processingMode`，只把
`production + LIVE` 写入生产 `scene_event`；Scene wire 继续携带这两个字段，
Actor、state/timer 和 Action Outbox 再次验证并保存 provenance。该方案与父架构
一致，且不要求新增 replay 能力。采用。

## 3. 冻结语义

### 3.1 信任来源

- `processingMode` 只能继承 ProtocolApplication/reducer/Flink 规范化输出中的
  可信字段；不得根据 topic 名、业务时间、到达延迟或设备自报字段重新推断。
- 普通 broker redelivery 必须保留原 `processingNamespace/processingMode`。
- 缺失、未知、非法组合或来源不可信时 fail closed，记录 quarantine/安全指标；
  禁止默认成 `LIVE`。
- 能进入生产 Scene lane 的唯一合法组合是
  `processingNamespace=production && processingMode=LIVE`。`shadow/<id>` 即使
  携带 `LIVE`，也不能进入生产 Scene lane。

### 3.2 Admission 决策矩阵

| 输入 | 生产 Scene Admission 行为 |
| --- | --- |
| `production + LIVE` | 通过 tenant/fact interest filter，最多产生一条生产 `SOURCE_DELTA` |
| `production + BACKFILL` | 普通在线 Raw 拆出的历史/补传 output；在 interest/candidate matching 前 deterministic bypass，不是 quarantine |
| `backfill/<jobId> + BACKFILL` | 显式 backfill job；同样在 interest/candidate matching 前终止；推进该 Admission 输入的正常处理前沿，但不写 `scene_event`、逐规则 skip、state、timer、Alarm 或 Outbox |
| `replay/<id> + REPLAY` | 不进入生产 `scene_event`；只允许留在 Raw/Canonical 隔离 replay namespace |
| `shadow/<id>` | 不进入生产 `scene_event/state/outbox` |
| 缺失、未知或其他 namespace/mode 不一致 | fail closed 并 quarantine/告警，不得降级为 `LIVE` |

Scene V1 不实现隔离的 Scene replay 求值。未来若要让 `REPLAY` 或显式 backfill
执行场景规则，必须另立 ADR，定义独立 input/state/outbox namespace、dispatcher
禁外发和晋升禁令；本次修订不预留可写回生产的旁路。

### 3.3 Wire 与派生传播

`SceneEvent` 的公共 envelope 增加必填 `ProcessingContext`：

```text
ProcessingContext {
  processingNamespace
  processingMode = LIVE | BACKFILL | REPLAY
}

SceneEvent {
  eventId
  processingContext
  identityAlgorithmVersion = SCENE_ID_V2
  oneof payload { ... }
}
```

- 生产 Topic 上的所有 `SceneEvent` 必须是 `production + LIVE + SCENE_ID_V2`。
- Canonical 派生的 `SourceDelta` 继承该 context，并由 Admission 盖章
  `SCENE_ID_V2`；它不是藏在含义不明的 `lineage` 中。
- `LinkedDelta`、Scene 规范化的下游事实、timer fire 和同一因果链上的 bundle
  必须复制原 context/version，不能重新默认为 LIVE。
- Schedule、Subject lifecycle 等生产数据面触发由各自受信生产者显式写
  `production + LIVE + SCENE_ID_V2`；Actor 仍执行统一校验。Binding lifecycle 是不继承
  `ProcessingContext` 的 no-state-value control，只能 close/reset/tombstone 并标记
  next-live bootstrap，不属于这条数据面规则。

### 3.4 State、timer、身份与动作

- live state、timer 和 Action Outbox row 保存
  `processingNamespace/processingMode/identityAlgorithmVersion`；新 live timer
  只能由 `production + LIVE + SCENE_ID_V2` 输入创建或推进。
- `transitionId/effectId` 的确定性输入加入 `processingNamespace`，与父架构
  的 namespace 隔离一致；mode 作为必填 provenance 保存并校验，不用 topic
  名或默认值补齐。
- 新 PG Action Outbox row 只能是 `production + LIVE + SCENE_ID_V2`；dispatcher
  只投递 production+LIVE 且 identity version 在 allowlist 内的 row（另含 §3.5
  原 ID 只读投递的 legacy pending row）。其他组合即使因缺陷落库也必须 fail closed、
  停止外发并告警。
- current `SubjectContext` 只决定处理时 scope matching；它不能修改或替代
  输入的 processing mode。归属语义与 mode 隔离相互正交。

### 3.5 既有数据与身份算法切换

本修订把 `processingNamespace` 加入 timer/schedule/lifecycle/transition/effect
身份，必须同时冻结 `identityAlgorithmVersion=SCENE_ID_V2`，所有新公式都以该
版本 tag 作为首个 hash 输入。它是身份编码版本，不是 Scene 产品版本。

切换采用一次性停写—排空—迁移窗口，不允许旧、新算法对同一 trigger 双求值：

- 暂停并 fence 旧 Admission、Scheduler、Things SubjectLifecycle 与 Runtime 派生
  producer；各 producer 排空已捕获输入前沿与 publish callback 并停止后，捕获最终
  生产 `scene_event` end offsets；旧 Runtime 再排空这些 offsets、timer callback、
  async state send 与 PG/Kafka in-flight，证明 committed next-offset 到达最终 end，
  再 pause/fence 旧 Action dispatcher 新 lease/dequeue，排空外发/status/PG in-flight、
  捕获稳定 pending row 集合，随后撤销旧 writer/dispatcher credential；
- 无法排空的 context-less `scene_event` 只能由受审计迁移工具隔离并按终态损失
  审批推进，不能交给新 Runtime 猜成 `LIVE`；
- 历史 state 只有在可证明来自 production live writer 且不含 timer/pending-trigger
  等旧 identity-bearing slot 时才可迁到 `SCENE_ID_V2`；其余 tombstone 并按类型化
  cold start，同时量化 baseline/duration/cooldown/Alarm 的可能重复或延迟效果并签字；
- 已接受的历史 pending Outbox 若证据充分，补齐 context 后保留原 `effectId` 与
  `LEGACY_NO_NAMESPACE_V1` 标记，dispatcher 只投递、不重算；证据不足则
  `BLOCKED_CONTEXT`；
- 排空证明、state/Outbox inventory 与迁移完成后，才启用 required wire/restore/
  新版 dispatcher 强门禁并恢复全部 `SCENE_ID_V2` producer 与 dispatcher。

首条 `SCENE_ID_V2` record 被接受后即越过 identity point-of-no-return：回滚只能使用
仍强制 required context/version 且只写 V2 的兼容 binary，或停写后向前修复；不得
恢复 legacy event/state/timer writer。

该一次性 deployment cutover 不是 Binding、affiliation 或普通数据的长期
effective-offset barrier，也不引入 Scene replay evaluator。

### 3.6 Scene 提交边界联合裁决

父架构旧 P0 表述要求 state、timer、AlarmSignal/ActionIntent 与 Outbox 在同一 PG
本地事务提交后才推进 offset，这与 Scene 主规格的内存热状态 + 异步
`scene_state` 无法同时实施。本次以 Scene V1 已确认的 best-effort 模型联合修订：

1. 先规范序列化并校验 tentative state；
2. PG 本地事务只插入本次 transition 的 Action Outbox rows；
3. commit 成功后发布 Actor 内存 state，并注册/取消进程内 timer；
4. 对完整 `scene_state` 做一次有界异步 producer handoff attempt；立即失败只记录，
   producer 接受后也不等待 broker ACK/callback；
   全部候选、Action Outbox/非 state pack 及其失败策略完成后，per-partition
   completion sequencer 才推进连续 next-offset。

因此 Scene 不把 state/timer 放入 PG，不承诺 state、Action Outbox 与 input offset 的
原子切面。AlarmSignal/ActionIntent 若作为动作存在，是 Action Outbox row 的类型，
不是另一个与 state/timer 共同提交的状态表。Raw → Canonical Kafka transaction 和
其他 reducer 的本地事务不受该 Scene 范围裁决影响。

## 4. 联合修订范围

必须在同一次变更中修改：

1. 场景主规格：目标/非目标、输入身份、Admission、`SceneEvent`/SOURCE/LINKED
   wire、Actor/state/timer/Outbox、失败语义、POC 和已拒绝方案；
2. `控制面与匹配契约.md`：把 mode gate 定义为 scope/index matching 的前置条件；
3. `算子语义与术语.md`：冻结 live state/timer 与 processing-time 语义；
4. `SLO故障测试与实施.md`：增加 mode matrix、误路由、缺失字段、dispatcher
   防线和零 live mutation/零外发测试；
5. Kafka 设计：在 Scene integration、ACL、failure matrix 和 contract tests 中
   明确 production Scene 只接受 `production + LIVE`；
6. 父架构：保留非 Scene 链路的硬契约，澄清 Scene V1 对 `REPLAY` 的当前能力是
   “不进入生产 Scene”，不是已经提供隔离 Scene replay executor；同时撤销 Scene
   旧 P0 的 state/timer/Signal/Intent/Outbox 同一 PG 事务要求，改为 §3.6 的顺序。

README 只补充导航级不变量，不承担替代上述规范的职责。

## 5. 验证标准

- 失败检查先证明当前 Scene 主规格和三份从属契约缺少有效 mode gate/wire/test；
- 四份 Scene 规范均出现明确且一致的 `LIVE/BACKFILL/REPLAY` 语义；
- `SceneEvent` 明确包含 `processingNamespace/processingMode`，SOURCE/Linked
  传播规则无歧义；
- Admission、Actor 和 dispatcher 三层都规定非生产 LIVE 不得产生 live mutation
  或真实外发；
- 父架构、Kafka 设计与场景主规格都只把 Action Outbox 放入 Scene PG 事务，并一致
  定义 state/timer、异步 state handoff 与连续 offset 的非原子顺序；
- TB Queue `ProcessingStrategy`/CF reprocessing 与本平台 Processing Mode 的差异有
  明确说明，不把竞品重试/历史任务误写成同一 wire 权限契约；
- 不出现 `REPLAY -> production scene_event/scene_state/Action Outbox`、缺失 mode
  默认 LIVE、按 topic 名推断 mode 等反向契约；
- 不恢复 RocksDB、Kafka EOS、Canonical replay 恢复或 Scene replay Runtime；
- Markdown diff、链接、代码围栏和跨文档关键词检查全部通过。
