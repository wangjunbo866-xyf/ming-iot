# Scene Current Fact 无值变更完成与水线推进设计

> 日期：2026-08-12
> 状态：设计已确认，等待实施计划
> 范围：补清 Current Fact writer 在“不修改 value slot”时的 input completion 与 `readyThroughNextOffset` 推进语义；不修改运行时代码或 wire。

## 1. 问题

现有《Scene Current Fact 设计》已经规定：全部启用目标 durable 后才能推进连续
`readyThroughNextOffset`，暂时性 store/catalog 失败时必须 retry，frontier 不得越过；同时列出
BACKFILL、OCCURRENCE、cutover 迟到记录等不修改 Scene-live value slot 的分支。

缺口在于：文档没有统一说明“不写 Current Fact 行”如何满足 input completion。解释文档又把
“事实行 durable”与“明确的 terminal no-write durable”写成通用二选一，容易产生两个误解：

1. 不修改 Current Fact 的普通输入会永久卡住 partition 水线；
2. 每条普通 no-op 都必须额外持久化一条 terminal-no-write 记录。

本设计补齐完成谓词，并保持现有高吞吐目标：每条输入都必须得到可信终态，但普通确定性 no-op
不产生逐输入审计写放大。

## 2. 术语决策

### 2.1 无值变更完成

统一使用 **无值变更完成（no-value-mutation completion）** 表示以下逻辑事实：

- writer 已完成该输入的适用性判断；
- 该输入不需要创建、推进或删除 Scene-live value slot；
- history、通用 latest 等其他启用目标已经 durable；
- 不存在尚未解决的 store/catalog/contract 错误；
- 若规范要求留下 reason-bearing terminal outcome，该 outcome 已 durable。

它是 input completion 的一个分支，不是新的 wire 枚举、Current Fact lookup 状态、数据库表或
Materializer 状态机状态。

### 2.2 terminal no-write

`terminal no-write` 保留为规范已经点名的、带原因的终态结果，例如：

- `SOURCE_ARTIFACT_NO_LONGER_APPLICABLE`；
- `UNSUPPORTED_FINALITY_BYPASS`；
- 规范要求留证的 deterministic bypass、conflict 或 quarantine outcome。

它属于无值变更完成，但不反向要求所有普通无值变更都逐条持久化独立 outcome。普通确定性
no-op 的 durable 汇总证明是连续 frontier 本身。

### 2.3 不引入 `RETRYABLE_FAILURE`

Materializer 投影没有 `RETRYABLE_FAILURE` 正式状态，本次文档不得新增该名称。暂时性失败只表示
当前 input 尚未满足完成谓词：writer 继续 retry，`readyThroughNextOffset` 不得越过。Runtime
下游可观察到的仍是既有 `PROJECTION_NOT_READY/BEHIND` 或 `LOOKUP_FAILED` 等读取结果。

## 3. 完成谓词与推进顺序

对 source route 中 offset 为 `o` 的输入，只有同时满足以下条件，才能把它计入连续完成集合：

```text
inputComplete(o) =
    applicabilityDecisionIsFinal(o)
    AND allApplicableTargetsAreDurable(o)
    AND requiredTerminalEvidenceIsDurable(o)
    AND noUnresolvedFailure(o)
```

其中：

- 需要修改 value slot 时，Current Fact 行属于 applicable target，必须先 durable；
- 不需要修改 value slot 时，该目标不产生行写入，但不会因此阻止 input completion；
- `requiredTerminalEvidenceIsDurable` 只约束规范明确要求留证的分支；普通确定性 no-op 不新增逐条记录；
- 任一暂时性或未决错误存在时，input 保持未完成并 retry。

writer 只能按 partition 连续推进：

```text
所有 offset < n 都满足 inputComplete
        ↓
先保证适用输出及必要终态证据 durable
        ↓
再发布 readyThroughNextOffset = n
        ↓
Kafka consumer checkpoint 不得越过未完成输入
```

`readyThroughNextOffset` 是应用侧共享水线，Kafka committed offset 是 consumer 恢复进度；两者
不是同一个字段。本设计不新增跨存储原子事务，只重申二者都不能领先于连续完成事实。

## 4. p3 中 B/C 的具体例子

假设 p3 的记录为：

```text
offset 100：B 湿度 60%，LATEST_STATE 且静态 eligible
offset 101：C 输入不需要修改 Scene-live value slot
broker end next-offset = 102
```

处理结果：

```text
处理 offset 100
  -> B Current Fact 行及其他启用目标 durable
  -> p3.readyThroughNextOffset = 101

处理 offset 101
  -> 完成 C 的适用性判断
  -> 其他启用目标 durable
  -> 若该分支要求 terminal outcome，则 outcome durable
  -> 不修改任何 Scene-live value slot
  -> p3.readyThroughNextOffset = 102
```

因此 A 的求值若捕获 `required(p3)=102`，只需等待 writer 完成 p3/101，不需要等待 B 再次上报。
B 的上报间隔由后续 freshness 检查处理，不由 barrier retry 处理。

“C 不需要写 Current Fact”必须是 writer 全局写入契约的结论，例如 C/fact 不在静态 eligible
范围或其 temporal kind 为 OCCURRENCE；仅仅“当前 A/B 规则没有引用 C”并不充分。若 C 被其他
Binding 静态纳入 Scene-live 投影，它仍应更新自己的 C value slot。

## 5. 分支矩阵

| 输入分支 | Scene-live value slot | 完成前必须满足 | frontier |
| --- | --- | --- | --- |
| production+LIVE、LATEST_STATE、静态 eligible | 创建或按 comparator 推进 | 所有适用行与其他启用目标 durable | 连续时推进 |
| 普通确定性 no-op | 不修改 | 适用性判断完成，其他启用目标 durable | 连续时推进 |
| OCCURRENCE | 永不写 | occurrence 的适用处理完成 | 连续时推进 |
| BACKFILL/REPLAY/shadow | 不创建、不推进、不删除 | 规范要求的 deterministic bypass terminal outcome 完成 | 连续时推进 |
| 迟到旧 release backlog | 不修改新槽位 | `SOURCE_ARTIFACT_NO_LONGER_APPLICABLE` terminal no-write 完成 | 连续时推进 |
| conflict/quarantine 最终处置 | 不产生业务值变更 | 规范要求的 durable outcome 完成 | 连续时推进 |
| 暂时性 store/catalog/contract 失败 | 未知或未完成 | 继续 retry，不得伪造终态 | 不得越过 |
| subject 删除、tenure 结束、明确 GC | 写技术 tombstone | tombstone 及其他适用目标 durable | 连续时推进 |

技术 tombstone 只表达 value slot 生命周期结束，不得拿来表示普通 no-op、BACKFILL 旁路或业务
`UNAVAILABLE`。

## 6. cutover 与迟到旧数据

owner/model/metric-plan cutover 与普通 data record 处理分开描述：

1. 受信控制面变更先进入唯一 writer；
2. semantic ID 缺失、eligible=false、contract hash 改变或 owner incarnation 改变时，在 cutover
   生效前写 visibility fence，防止旧槽位冒充新事实；只有该变更同时表示 subject 删除、ownership
   tenure 结束或明确 projection GC 时，才允许技术 tombstone；
3. cutover 后迟到的旧-release backlog 不得重新打开或推进新槽位；
4. 该迟到输入以 `SOURCE_ARTIFACT_NO_LONGER_APPLICABLE` terminal no-write 完成，然后才可计入
   连续 frontier。

## 7. 崩溃与重放

- frontier durable 前崩溃：Kafka checkpoint 不得越过该输入；恢复后允许重放并重新执行确定性判断；
- frontier durable 后崩溃：`readyThroughNextOffset=n` 汇总证明 `< n` 的输入都已经完成，不要求为
  每个普通 no-op 保留独立记录；
- 需要独立审计、修复或 quarantine 的特殊分支仍按既有规范保存 reason-bearing durable outcome；
- 同 generation 内 frontier 单调递增，任何重放不得让旧输入覆盖 comparator 已接受的新值。

## 8. 文档修改范围

1. 《Scene Current Fact 设计》§3：把长列表拆成正常写入、无值变更完成、暂时失败、cutover 四块，
   正式拥有完成谓词与 frontier 推进规则。
2. 《算子语义与术语》：在 `completion frontier / readyThroughNextOffset` 附近定义“无值变更完成”，
   明确它不是新状态或逐输入持久化要求。
3. 《水线与读取屏障详解》§4.2：修正“事实行或 terminal no-write 必须 durable”的过强表述，加入
   p3/B/C 数字例子和“不等待 B 再上报”的结论。
4. 《00-15分钟读懂场景联动》§6：只增加一句摘要并链接解释文档，不复制完成算法。

不修改两个时序图 HTML，不扩展 wire/schema，不新增 Materializer 状态，不修改 SLO 或运行时代码。

## 9. 验证

- 文本检查必须能定位：普通无值变更可推进、OCCURRENCE 可完成、暂时失败不得推进、cutover 迟到
  backlog 的 terminal no-write，以及 B 无需再次上报；
- 全目录不得把 `RETRYABLE_FAILURE` 写成正式状态；
- 新旧术语不得暗示每个普通 no-op 都必须插入 outcome 行；
- Markdown 本地链接与片段锚点全部解析；
- `git diff --check` 通过；两个 HTML 哈希不变；
- feature worktree 保持干净，父 `master` 的四处用户修改保持未暂存且内容不变。
