# SceneBindingState 读者化解释设计

**日期：** 2026-08-09
**状态：** 待评审
**目标文档：** `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`

## 1. 背景与问题

当前术语文档只用一行表格说明 `SceneBindingState`，运行时主设计则直接给出完整结构。
读者虽然能逐个翻译 `arguments`、`fsmState`、`timerState` 等字段，却难以回答：

- 这份状态具体属于谁；
- 为什么已有 Scene Current Fact 还需要它；
- 一次多设备规则求值中，哪些字段在什么时候变化；
- 哪些信息绝对不应写入它。

本次只改善解释层，不新增字段、不改变运行时契约，也不把术语文档扩写成另一份实现规范。

## 2. 目标读者与阅读结果

面向第一次接触场景联动运行时、但已了解设备属性和 Kafka 基础概念的研发人员。
读完新增小节后，读者应能用一句话复述：

> `SceneBindingState` 是某个 Scene Binding 对某个求值对象的可恢复计算进度；
> Current Fact 保存“现在是什么”，它保存“规则已经算到哪一步”。

读者还应能沿一条具体规则说明 `arguments`、`fsmState.duration`、`timerState`
为何存在，并判断一个新字段是否应该放进这份状态。

## 3. 文档位置与篇幅

在 `## 3. 四类上下文不得混用` 内，现有四类上下文表格和稳定身份说明之后，
新增 `### 3.1 SceneBindingState 到底保存什么`。

保持后续 H2 编号及锚点不变。新增内容控制在约 80～100 行，采用短段落、分组表格和
单笔业务走读，不再复制完整 schema。

## 4. 核心解释模型

先固定三个问题：

1. **属于谁：** 唯一身份是
   `(tenantCode, bindingId, evaluationSubjectKey)`，即“某租户的一条规则绑定，针对一个求值对象”。
2. **保存什么：** Scene Runtime 为连续求值和故障恢复保留的有界参数视图与算子进度；
   其中 duration、edge、debounce、cooldown 等历史进度不能只靠当前配置和 Current Fact
   无歧义恢复。
3. **不是什么：** 不是 Scene Current Fact 存储、不是命令执行记录，也不是 Go Actor/进程内对象快照。

为了避免逐字段罗列，将现有字段按业务职责分为六组：

| 业务职责 | 对应字段 | 回答的问题 |
| --- | --- | --- |
| 稳定身份与 schema 兼容 | 顶层 identity、`schemaVersion`、`configRevision`、`semanticHash` | 这是谁的状态，当前 Binding 语义是否还能延续它 |
| 本次规则看到的参数视图 | `arguments` | 这条 Binding 上次已接受并参与计算的各参数是什么 |
| 有记忆的算子进度 | `rollingArguments`、`scriptState`、`alarmState`、`fsmState` | 窗口、边沿、回差、持续、去抖、冷却已经推进到哪里 |
| 可恢复的业务计时 | `timerState[]` | 哪个算子因哪次触发开始等待，何时应重新检查 |
| 集合成员与生命周期门禁 | `members.{active,lastAggregateVersion,...}`；state-level 的 `metadata.lastSubjectAggregateVersion` | 前者保存共享 target 的成员切片及逐成员版本门禁；后者单独保护 device-target 求值对象的生命周期 |
| 处理来源与恢复审计 | `processingNamespace`、`processingMode`、`identityAlgorithmVersion`、`lastEvaluationMetadata` | 该进度是否来自可信生产 LIVE 流，恢复时如何 fail closed |

字段解释强调职责，不承诺所有字段都由读者直接操作；完整结构仍链接到运行时主设计 §8.1。

## 5. 用 R1 串起一笔具体业务

沿用导读中的规则 R1：

> `temp-01/02/03` 共同计算出的 5 分钟平均温度高于 30°C，且 `door-01=CLOSED`
> 持续 2 分钟，则启动 `fan-01/02` 并发送 webhook。

走读只描述一条 Binding、一个 evaluation subject 的状态变化：

1. `ComputedMetricUpdated` 触发 Scene 求值、但门状态仍缺失时，`arguments` 只记录已接受的
   参数视图；latest-value join 为 `NOT_READY`，不 arm timer，也不产生 action。
2. `door-01=CLOSED` 先由其唯一 writer 更新 PROPERTY Current Fact；R1 将门配置为
   `CONTEXT_ONLY`，所以这条门事件本身不触发 Binding 求值。下一条 `ComputedMetricUpdated`
   到达后，Runtime 读取并 overlay 平均温度与门的当前值，条件才完整为真。此时 Scene 的
   `arguments` 保存已接受的 COMPUTED_METRIC 结果值和门属性视图，duration 进入 `ARMED`，`timerState` 记录
   `armedByTriggerId/armedAt/dueAt`。R1 的 5 分钟原始样本和窗口进度属于上游
   ComputationPlan/Flink，不写入 `SceneBindingState.rollingArguments`；后者只服务于
   Binding 自己声明的轻量、有界 rolling 算子。
3. 两分钟内若门重新打开，该 `CONTEXT_ONLY` 更新仍只推进 Current Fact，不会立即取消 timer。
   若期间又有 COMPUTED_METRIC trigger，Runtime 会在那次求值发现条件为假并取消 duration；
   否则 timer 到期时重新读取 Current Fact，发现门已打开后取消且不产生 action。
   这说明状态保存的是“持续条件进度”，而门的当前值仍由 Current Fact 提供。
4. 若两分钟内条件一直成立，timer 到期触发重新求值；通过门禁后先形成 tentative state/action，
   再在一个 PostgreSQL 本地事务中插入本轮全部 Action Outbox rows；该事务不写
   `SceneBindingState` 或 timer state。事务提交后才替换内存状态并注册/取消 timer handle，
   完整 `scene_state` 随后异步发布。
5. `fan-01/02` 的命令投递与执行结果进入 CommandExecution/Attempt，不回写成
   `SceneBindingState`；后续相同输入是否受 cooldown 拦截，才属于该 Binding 的计算进度。

该例同时解释 `arguments` 是“上次参与本 Binding 求值的视图”，不等于 Scene Current Fact
本身。Current Fact 是有唯一 writer、可能落后于当前 Kafka Delta 的受信当前值投影，
而不是原始业务 source of truth；冷启动仍需读取它，再合并当前 Delta。

## 6. 明确排除项

新增小节必须明确以下内容不进入 `SceneBindingState`：

- Scene Current Fact 的完整当前值投影及其唯一-writer 职责；`arguments` 只缓存本 Binding
  上次已接受的参数视图，不把 SceneBindingState 变成另一个 Current Fact store；
- 完整 Binding 配置和已编译脚本/表达式对象；只保留既有的
  `configRevision/semanticHash` 元数据用于审计与兼容判断；
- CommandExecution/Attempt 与设备命令结果；
- Go Actor、mailbox、锁、缓存引用、`partitionGeneration`；
- Go timer handle；只持久化可重建 handle 的 timer business state；
- 已接受 action 的可靠投递事实；该事实属于 PostgreSQL Action Outbox。

## 7. 判断口诀

以五条判断规则收尾：

- 丢失后会改变下一次规则计算的历史进度，而且无法从 Current Fact 无歧义重建：放 state。
- 当前事实可作为 `arguments` 的上次已接受视图参与增量求值，但副本不是权威真相；
  冷启动仍从 Current Fact 读取并合并当前 Delta。
- 完整 Binding 配置、编译对象等可由当前契约无歧义重建的内容：不复制进 state；
  `configRevision/semanticHash` 只作为既有兼容 metadata 保留。
- 需要可靠交付给外部系统：进入 PostgreSQL Action Outbox，而不是 state。
- 只是当前进程的执行对象：不序列化，恢复时按业务 state 重建。

## 8. 非目标与验收

非目标：修改 schema、Topic、事务边界、恢复算法、配额或现有术语身份定义；新增 HTML/时序图；
逐字段重复运行时主设计。

验收条件：

- 新增小节位于 §3 内且只出现一次，后续 H2 编号不变；
- 六类字段职责均能映射到主设计 §8.1 的既有字段；
- R1 明确覆盖 `NOT_READY → ARMED → 取消或到期 → Action Outbox → 异步 scene_state`；
- 明确区分 Current Fact、SceneBindingState、CommandExecution 与 Actor transient context；
- 不引入主设计不存在的新字段或新的规范性承诺；
- Markdown 检查与链接检查通过，变更仅涉及目标术语文档及本设计/实施记录。
