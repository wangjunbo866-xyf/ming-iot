# Scene Waterline and Read Barrier Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增《水线与读取屏障详解》，集中解释 required waterline、actual waterline、retry / fail closed / freshness 和 Kafka 局部阻塞语义，并将导读中的超宽 Mermaid 替换为完整案例 HTML 外链。

**Architecture:** 采用“解释文档集中、规范文档保留权威定义”的分层方式。新文档串起 SceneEvent → matching / Binding → Current Fact key / source route → fixed barrier → writer waterline → freshness → predicate 的完整链路；导读和 README 提供入口，三份规范文档只增加反向链接，不复制算法。

**Tech Stack:** Markdown、Git、POSIX shell、`rg`；现有单文件 HTML 仅作为链接目标，不修改。

---

## File map

- Create: `亿级点位高吞吐架构/6.规则引擎与场景联动/水线与读取屏障详解.md`
  - 非规范性的唯一通俗解释入口；拥有示例、流程、决策图、retry 阻塞范围和排障说明。
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md`
  - 删除超宽 Mermaid，链接完整案例 HTML；将水线长文缩成摘要并修正章节编号。
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`
  - 第一次阅读改指完整案例图；按问题查阅增加新文档入口。
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene current fact设计.md`
  - 在 §5.2 增加新解释文档链接，规范内容不动。
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`
  - 在 §6.1 增加新解释文档链接，决策顺序不动。
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md`
  - 在 §5 增加 retry 阻塞范围的解释链接，调度契约不动。
- Do not modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动端到端时序图.html`
- Do not modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html`

The worktree already contains user-owned changes in `README.md`, `场景联动完整案例端到端时序图.html`, `附录-竞品状态方案调研.md`, and `附录2-thingsboard规则引擎与CF实现分析.md`. Preserve them. In particular, do not stage the pre-existing README hunk when committing this implementation.

### Task 1: Create the standalone explainer

**Files:**
- Create: `亿级点位高吞吐架构/6.规则引擎与场景联动/水线与读取屏障详解.md`
- Reference: `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene current fact设计.md:115-181`
- Reference: `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md:342-380`
- Reference: `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md:106-165`

- [ ] **Step 1: Run the precondition check and record protected-file hashes**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/iot-doc
test -f '亿级点位高吞吐架构/6.规则引擎与场景联动/水线与读取屏障详解.md'
sha256sum \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动端到端时序图.html' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
git status --short
```

Expected: `test` exits non-zero because the new document does not exist. Record both SHA-256 values. `git status` still shows the four pre-existing user changes and no new implementation files.

- [ ] **Step 2: Create the explainer with the exact structure and semantics below**

Use `apply_patch` to create the file with this content:

````markdown
# 水线与读取屏障详解

> **定位：解释性导读，不是新的规范来源。** Current Fact 投影与 barrier 的规范以
> [《Scene Current Fact 设计》](Scene%20current%20fact设计.md#52-水线与-barrier)为准，
> 求值顺序以[《算子语义与术语》§6.1](算子语义与术语.md#61-水线新鲜度与-fail-closed-的配合)为准，
> lane 与 Kafka completion 以[《Scene Runtime 调度、分片与故障转移》§5](Scene%20Runtime%20调度、分片与故障转移.md#5-正常调度与完成边界)为准。

本文的“读取屏障”（read barrier）只指 **Scene Runtime 读取 Current Fact 时捕获的固定 source-end vector**。它不是配置发布屏障、Binding activation barrier、restore cut，也不是 Scene Runtime consumer group 自己提交的 offset。

<a id="one-minute"></a>

## 1. 一分钟结论

一次求值需要设备 A、设备 B 等多个来源的事实时，Runtime 不能直接把存储里“看起来最新”的几行拼起来。正确顺序是：

1. 根据 Binding 找出所有必要参数及其 source route；
2. 首次读取时，通过 Kafka `ListOffsets` 捕获这些 route 当时的 broker end，形成固定 `readBarrierId`；
3. 将这个“要求水线”逐 route 与 Current Fact writer 发布的实际水线 `readyThroughNextOffset` 比较；
4. 所有 route 都追平后，才读取 Current Fact、检查 freshness，再执行 predicate / duration / action；
5. 未追平时复用同一个 barrier 做 bounded retry；重试耗尽后 fail closed，本轮不产生动作。

下文用说明性名称 `capturedEndNextOffset` 表示 barrier 中某条 route 的 broker end。它不要求成为新的 wire 字段。

```text
对 readBarrierId 中的每条 route r：

actual.readyThroughNextOffset(r, generation)
    >= required.capturedEndNextOffset(r)
```

一句话区分三者：

- **水线 / readiness**：数据投影完整了吗？
- **freshness**：已经完整读到的数据还够新吗？
- **fail closed**：无法证明完整或可信时，禁止产生业务副作用。

<a id="why-barrier"></a>

## 2. 为什么需要读取屏障

假设规则是“设备 A 温度超过 30℃，并且设备 B 湿度低于 40%，就启动风机”。A 的新温度已经被 Scene Admission 消费并触发 Runtime，但 Current Fact Materializer 对 B 所在 route 还在追赶。

此时存储里可能同时存在：

- A 的新温度：31℃；
- B 上一次投影成功的旧湿度：35%；
- B 所在 Kafka route 中尚未完成投影的新记录。

如果 Runtime 直接读取 latest，就会拿“新 A + 旧 B”求值得到 true，并发出不可逆的设备命令。稍后 B 的新湿度 60% 才投影完成，也无法撤销已经接受或下发的动作。

读取屏障要求 Runtime 先证明：捕获 barrier 时已经位于 broker 中的必要来源记录，Current Fact writer 都已连续处理完成。证明不了就不求值。

### 屏障保证什么

- 保证必要 route 的投影连续进度至少达到固定下界；
- 避免把投影空洞误判成“没有值”或拿旧值继续求值；
- 让 fail closed、重放和排障有明确的 source position 证据。

### 屏障不保证什么

- 它不是跨 partition 的 event-time 原子快照；
- 它不是历史 `as-of` 查询时间戳；
- 屏障满足后，Current Fact 可以返回 comparator 已接受的、更晚值；
- 不同 stream、partition 或 projection generation 的 offset 不能互相比较，只能逐 route 判断。

<a id="scene-to-routes"></a>

## 3. 从 SceneEvent 到必要参数和 source route

### 3.1 matching / fanout 先找到需要求值的 Binding

Runtime 收到的不是“直接读取 B 湿度”的指令，而是 `SceneEvent / SourceDelta`。处理顺序是：

1. 事件携带 `tenantCode`、`sourceSubjectKey`、fact delta 和 source position；
2. Runtime 用 `ExactTriggerIndex[(factType, factKey)]` 找到候选 TriggerPosting；
3. 经过 enabled、source、scope、target 等过滤；
4. 按 `evaluationSubjectKey` 聚合并去重为完整 Binding bundle；
5. Runtime 查看 Binding 的参数定义，得到本次求值所需的全部事实。

### 3.2 查询事实和定位来源是两条不同链路

以“读取设备 B 的湿度属性”为例，Binding 参数至少能确定来源主体、fact type 和 fact key。由此形成两种不同的 key：

```text
事实查询：Binding 参数
        -> (tenantCode, factOwnerKey=B, PROPERTY, factKey=humidity)
        -> SceneCurrentFactKey

来源路由：Binding 参数中的来源主体 B
        -> routeKeyBytes(tenantCode, sourceSubjectKey=B)
        -> partition function
        -> (sourceStreamId, partition)
```

`factKey` 回答“读哪个事实”；`routeKey → partition` 回答“该来源记录落在哪条 Kafka route”。因此不能把它理解成“先由 factKey 生成 routeKey，再由 routeKey 计算 partition”。

更准确的说法是：同一个 Binding 参数同时提供构造 Current Fact query key 和 source route 所需的信息。

多个参数可能落在同一 route，也可能跨多个 stream / partition。Runtime 对 route 去重后形成 barrier 注册集合，但仍按完整 Current Fact key 逐事实读取。

<a id="waterline-compare"></a>

## 4. 要求水线从哪里来，和谁比较

### 4.1 要求水线：首次读取时捕获的 broker end

一次 logical trigger 首次读取 Current Fact 时：

1. Runtime 向 `CurrentFactBarrierSnapshotProvider` 登记全部必要 source route；
2. provider 将并发登记做 micro-batch，避免每次规则求值都单独调用 Kafka；
3. 只有“登记完成之后才开始”的 capture cycle 可以满足该请求；
4. capture cycle 对 route 并集调用 Kafka `ListOffsets`，取得各 partition 的 end next-offset；
5. provider 返回不可变 source-end vector，并为它生成 `readBarrierId`。

这组 end next-offset 就是本次求值的**要求水线**。当前触发记录所在 route 的 captured end 至少覆盖 `recordOffset + 1`。

### 4.2 实际水线：Current Fact writer 的连续完成进度

每种 Current Fact 来源有唯一 writer。writer 只有在以下顺序完成后才能推进水线：

1. 当前事实行或明确的 terminal no-write 结果已经 durable；
2. 该 partition 之前的记录不存在空洞；
3. 再发布 `readyThroughNextOffset`。

因此 `readyThroughNextOffset` 是实际投影进度，不是 Runtime 从 Kafka 猜出来的值，也不是 TDengine 的最大时间戳。

### 4.3 数字算例

假设首次 capture 得到：

| source route | projection generation | 要求 `capturedEndNextOffset` | 实际 `readyThroughNextOffset` | 结果 |
| --- | --- | ---: | ---: | --- |
| telemetry / p3 | g17 | 101 | 101 | READY |
| telemetry / p7 | g17 | 260 | 257 | BEHIND |

虽然 p3 已经 ready，但整个 barrier 仍未满足，因为 p7 还差 3 个 next-offset。Runtime 返回 `BEHIND` 并 retry；当 p7 的实际水线达到 260 时，才进入 Current Fact 读取。

### 4.4 为什么 retry 必须复用同一个 barrier

如果每次 retry 都重新调用 `ListOffsets`，broker end 可能持续从 260 变成 270、280，Runtime 就一直追逐移动目标，既难以收敛，也无法说明本次求值究竟在等哪个边界。

所以同一 actor attempt 的 bounded retry 复用第一次捕获的 `readBarrierId`。只有 crash / rebalance 使进程内 in-flight barrier 丢失时，规范才允许为同一 logical trigger 重新捕获；这不等于系统拥有 durable MVCC read timestamp。

<a id="decision-order"></a>

## 5. retry、fail closed 与 freshness 如何配合

```text
捕获或复用固定 readBarrierId
        |
        v
waterline / readiness 是否覆盖？ -- 否 --> bounded retry
        |                                  |
       是                                  +-- retry 耗尽 --> fail closed
        v
coverage seed / conflict 是否通过？ -- 否 --> retry 或明确失败处置
        |
       是
        v
读取 Current Fact 并检查 freshness
        |
        +-- 过旧 --> missing/freshness policy
        |
        +-- 新鲜 --> predicate / edge / duration / timer / action
```

| 概念 | 回答的问题 | 失败含义 | 处理 |
| --- | --- | --- | --- |
| 水线 / readiness | 必要来源是否完整投影到固定 barrier | 不知道事实是否完整 | 同一 barrier bounded retry；耗尽后 fail closed |
| coverage / conflict | 无 row 是否真能解释为 never seen，事实身份是否可信 | 完整性证据不足或存在冲突 | retry、quarantine 或获批准的最终失败处置 |
| freshness | 已知事实是否仍在业务允许年龄内 | 数据完整，但太旧 | 按 missing/freshness policy 返回 NOT_READY、保持 ARMED 或取消 |
| fail closed | 证据不足时能否继续产生副作用 | 本次求值不可信 | 不进入 predicate，不执行动作，记录可观测的最终失败处置 |

关键规则：

- 水线未 ready 时，不检查 freshness，不用 store 中的旧值降级；
- 水线失败不是业务 missing，fail closed 也不是把 predicate 偷偷判成 false；
- fail closed 后，本次 input 仍须完成 quarantine 或获批准的最终失败处置，之后才能满足 partition completion；
- bounded retry 让短暂投影延迟有机会追平，fail closed 防止一条输入无限钉住 lane 和 offset frontier；
- 后续新事件可以作为新的 logical trigger 重新捕获 barrier 并再次求值。

### FULL 与 DIRECT_TD

- **FULL**：严格执行 barrier → coverage → freshness → predicate；水线未 ready 时 retry，耗尽后 fail closed。
- **DIRECT_TD**：严格 barrier 降级为软 lag 门，用更弱的一致性换低延迟；它不能声称拥有 FULL 的可恢复完整性保证。

<a id="retry-kafka"></a>

## 6. retry 会不会阻塞 Kafka 其他消息

直接答案：**不会阻塞 Consumer Group 的 poll / heartbeat 线程，但会在顺序边界内造成局部阻塞，并可能触发 partition 级背压。**

| 范围 | retry 期间发生什么 |
| --- | --- |
| Consumer poll / heartbeat | 不得在 poll、rebalance callback 或协调线程中 `sleep`、同步等待远端水线；retry 由 callback 外的异步调度承载 |
| 当前 source/evaluation-subject lane | 当前 input 未完成时，同 lane 的后续输入不能超车 |
| 同一 Kafka partition 的其他 lane | 不同 subject-key lane 仍可并发执行，但较晚完成的 offset 不能越过较早未完成 offset 提交 |
| partition completion | 最早未完成 input 会钉住连续 next-offset frontier；in-flight 达到上限后可以 pause / throttle 该 partition |
| 其他 ACTIVE partition | 使用独立 `PartitionRuntimeContext`，仍可 dispatch、完成和提交；consumer 仍持续 poll 以维持 group heartbeat |

例如 partition 0 的 offset 100 正在等 barrier，而 101、102 已在其他 lane 完成：

```text
p0: 100=RETRY, 101=DONE, 102=DONE
    ^ commit frontier 不能跳过 100

p1: 50=DONE, 51=DONE
    ^ p1 可按自己的连续 frontier 提交
```

因此“会不会阻塞”要分层回答：它会阻塞同 lane 的顺序，并可能暂时阻塞同 partition 的 offset 提交；不会把整个 Kafka consumer 的 poll / heartbeat 卡死，也不会天然阻塞其他 partition。

当 bounded retry 耗尽并完成 fail-closed 的最终失败处置后，offset 100 才能进入完成态，p0 的连续 frontier 随后可以越过它。这也是 retry 必须有界的原因之一。

<a id="complete-case"></a>

## 7. 映射到完整案例

完整案例是：

```text
AVG(temp-01, temp-02, temp-03, 5m) > 30℃
AND door-01 = CLOSED FOR 2m
THEN start fan-01 + start fan-02 + Webhook
```

对应关系如下：

1. 三个温度点进入 Flink 共享计算，`ComputedMetricCurrentFactProjector` 写 5 分钟平均温度；
2. `door-01` 属性由 Timeseries Materializer 写 PROPERTY Current Fact；
3. 本例由 `ComputedMetricUpdated` 作为 TRIGGER 经 Admission 形成 SceneEvent；门状态作为 CONTEXT_ONLY 写入 Current Fact，供本次 latest-value join 使用；
4. matching / fanout 找到 Binding，Binding 要求“平均温度”和“门状态”两个参数；
5. 两个参数可能来自不同 stream / partition / writer，所以读取屏障是 route vector，不是单个 offset；
6. 第一次求值通过后只武装 2 分钟 duration；timer 到期回到同一 evaluation-subject lane，再次执行 barrier、coverage、freshness 和 predicate 检查；
7. 复核仍通过后，PG action-acceptance transaction 才同时接受两台风机命令和 Webhook。

完整五阶段图请在浏览器打开：
[《场景联动完整案例端到端时序图》](场景联动完整案例端到端时序图.html)。

<a id="troubleshooting"></a>

## 8. 常见误解与排障顺序

| 问题 | 正确答案 |
| --- | --- |
| `ListOffsets` 拿到的值跟谁比较？ | 逐 route 与对应 generation 的 writer `readyThroughNextOffset` 比较 |
| `ListOffsets` 返回的是实际投影进度吗？ | 不是；它是首次 capture 时的 broker end，即要求达到的下界 |
| 没有 Current Fact row 就是业务 missing 吗？ | 不一定；先看水线和 coverage seed，只有证据完整后才能解释为 `READY_NEVER_SEEN` |
| 可以读取 TDengine/latest 中已有旧值先算吗？ | FULL 档不可以；这会把投影空洞伪装成完整事实 |
| retry 能重新抓一个新 broker end 吗？ | 同一 actor attempt 不可以；必须复用固定 barrier，crash/rebalance 丢失 in-flight barrier 是例外 |
| 同 partition 就是同一个 Actor lane 吗？ | 不是；partition 是 ownership/completion 边界，lane 按 subject key 串行 |
| barrier ready 就是历史快照吗？ | 不是；它只证明固定下界，读取时可以看到更新值 |
| fail closed 会让 offset 永远不能提交吗？ | 不会；quarantine 或获批准的最终失败处置完成后 input 可完成，但本轮不产生动作 |

建议按以下顺序排障：

1. 看错误状态：`PROJECTION_NOT_READY / BEHIND / LOOKUP_FAILED / PROJECTION_CONFLICT`；
2. 对照 `readBarrierId` 的 route vector 和 writer `readyThroughNextOffset`，确认 generation 一致；
3. 查看 waterline lag、barrier capture latency / age、retry exhausted 数量；
4. 查看最早未完成 offset、partition in-flight 和 pause/backpressure；
5. readiness 全部通过后，再检查 Current Fact 状态、coverage seed 和 freshness policy；
6. 最后才检查 predicate、duration 和 Action Outbox。

## 9. 规范入口

- Current Fact key、writer、barrier、coverage、读取状态和双档部署：
  [《Scene Current Fact 设计》](Scene%20current%20fact设计.md)
- readiness → coverage → freshness → predicate/FSM：
  [《算子语义与术语》§6.1](算子语义与术语.md#61-水线新鲜度与-fail-closed-的配合)
- lane、completion sequencer、partition pause 和 rebalance：
  [《Scene Runtime 调度、分片与故障转移》](Scene%20Runtime%20调度、分片与故障转移.md)
- Binding、TriggerPosting、matching 和 fanout：
  [《控制面与匹配契约》](控制面与匹配契约.md)
````

- [ ] **Step 3: Run focused content checks**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/iot-doc
doc='亿级点位高吞吐架构/6.规则引擎与场景联动/水线与读取屏障详解.md'
test -f "$doc"
rg -q '<a id="waterline-compare"></a>' "$doc"
rg -q 'actual\.readyThroughNextOffset' "$doc"
rg -q 'required\.capturedEndNextOffset' "$doc"
rg -q '<a id="retry-kafka"></a>' "$doc"
rg -q '不会阻塞 Consumer Group 的 poll / heartbeat' "$doc"
rg -q 'fail closed 也不是把 predicate' "$doc"
rg -q '同 partition 就是同一个 Actor lane.*不是' "$doc"
git diff --check -- "$doc"
```

Expected: every command exits 0 with no output from `git diff --check`.

- [ ] **Step 4: Commit only the new explainer**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/iot-doc
git add -- '亿级点位高吞吐架构/6.规则引擎与场景联动/水线与读取屏障详解.md'
git diff --cached --check
git diff --cached --name-only
git commit -m 'docs: explain scene read barriers and waterlines'
```

Expected: the cached name list contains only `水线与读取屏障详解.md`; commit succeeds.

### Task 2: Replace the unreadable Mermaid and shorten the guide

**Files:**
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md:58-304`
- Reference only: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html`

- [ ] **Step 1: Run checks that demonstrate the current guide problem**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/iot-doc
guide='亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'
test "$(rg -c '^```mermaid$' "$guide")" -eq 0
test "$(rg -c '^## 5\.' "$guide")" -eq 1
rg -q '\[《水线与读取屏障详解》\]\(水线与读取屏障详解\.md\)' "$guide"
```

Expected: all three checks fail: the guide has one Mermaid block, two `## 5.` headings, and no new explainer link.

- [ ] **Step 2: Replace the Mermaid block with the HTML reading entry**

Use `apply_patch` to replace the complete fenced Mermaid block under `## 5. 端到端时序图` with exactly:

```markdown
这张图覆盖“3 个温度点求 5 分钟平均值 + 门状态持续满足 + 启动两台风机并调用 Webhook”的完整链路。参与者和失败边界较多，不再把整张 Mermaid 压缩进 Markdown。

> **[在浏览器中打开大字版完整时序图](场景联动完整案例端到端时序图.html)**
>
> HTML 按总览和五张阶段细图拆分，可使用浏览器的 `Ctrl/Cmd + +/-` 缩放或全屏查看；两份文档不再维护重复的时序图源码。
```

Change the fallback heading from:

```markdown
### 图不显示时，按这五步读
```

to:

```markdown
### 无法打开 HTML 时，按这五步读
```

Keep the existing five-row fallback table unchanged.

- [ ] **Step 3: Replace the duplicated long waterline section with a concise section 6**

Replace the current block from `## 5. 为什么水线宁可失败也不用 TDengine 里的“最新数据”` through its FULL / DIRECT_TD table and trailing reference with:

```markdown
## 6. 为什么水线宁可失败也不用“看起来最新”的值

一句话：**Runtime 必须先证明所有必要来源都已完整投影，再进行业务求值。** 否则它可能把设备 A 的新值和设备 B 的旧值拼在一起；动作一旦被接受或下发，随后到达的新数据无法撤销错误副作用。

首次读取时，Runtime 通过 Kafka `ListOffsets` 捕获固定的要求水线；Current Fact writer 的 `readyThroughNextOffset` 是实际投影水线。FULL 档逐 route 比较：

```text
actual.readyThroughNextOffset(r, generation)
    >= required.capturedEndNextOffset(r)
```

未覆盖时，Runtime 复用同一个 `readBarrierId` 做 bounded retry；耗尽后 fail closed。本轮不进入 freshness、predicate 或动作。只有水线和 coverage 都通过后，才检查事实是否过旧。

> 要求水线从哪里来、与谁比较，以及 retry 对 Kafka 其他消息的影响，见
> [《水线与读取屏障详解》](水线与读取屏障详解.md)。规范定义仍见
> [《Scene Current Fact 设计》§5.2](Scene%20current%20fact设计.md#52-水线与-barrier)和
> [《算子语义与术语》§6.1](算子语义与术语.md#61-水线新鲜度与-fail-closed-的配合)。
```

This intentionally removes the incorrect statement that devices in one partition necessarily share one Actor lane.

- [ ] **Step 4: Renumber the remaining top-level sections and add the new reading entry**

Apply these exact heading changes:

```text
## 6. 三类状态不要混在一起     -> ## 7. 三类状态不要混在一起
## 7. 重投为什么不应重复动作   -> ## 8. 重投为什么不应重复动作
## 8. 本例的关键身份怎样传下去 -> ## 9. 本例的关键身份怎样传下去
## 9. 接下来按问题读             -> ## 10. 接下来按问题读
```

In the `## 10. 接下来按问题读` table, insert this row immediately before the Current Fact / duration row:

```markdown
| 要求水线、实际水线、读取屏障、retry / fail closed / freshness，以及 retry 是否阻塞 Kafka | [水线与读取屏障详解](水线与读取屏障详解.md) |
```

- [ ] **Step 5: Verify the compact guide**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/iot-doc
guide='亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'
test "$(rg -c '^```mermaid$' "$guide" || true)" -eq 0
test "$(rg -c '^## 5\.' "$guide")" -eq 1
test "$(rg -c '^## 6\.' "$guide")" -eq 1
rg -q '\[在浏览器中打开大字版完整时序图\]\(场景联动完整案例端到端时序图\.html\)' "$guide"
rg -q '\[《水线与读取屏障详解》\]\(水线与读取屏障详解\.md\)' "$guide"
! rg -q '同一 partition.*同一 Actor lane' "$guide"
test -f '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
git diff --check -- "$guide"
```

Expected: every command exits 0; no Mermaid block or duplicate section 5 remains.

- [ ] **Step 6: Commit the guide change**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/iot-doc
git add -- '亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'
git diff --cached --check
git diff --cached --name-only
git commit -m 'docs: link readable complete scene sequence diagram'
```

Expected: the cached name list contains only `00-15分钟读懂场景联动.md`; commit succeeds.

### Task 3: Connect README and normative documents

**Files:**
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md:7-27`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene current fact设计.md:140-145`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md:342-344`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md:106-111`

- [ ] **Step 1: Add the README reading paths while preserving its existing dirty hunk**

In `README.md`, replace the paragraph at current lines 13-15 with:

```markdown
阅读任何一篇文档时，都可以打开[《场景联动完整案例端到端时序图》](场景联动完整案例端到端时序图.html)对照定位。它用“3 个温度点求 5 分钟平均值 + 门状态持续满足 + 启动两台风机并调用 Webhook”串起 Durable Canonical、共享计算、Current Fact、Scene 求值、动作接受和回复归并；请在浏览器中缩放或全屏阅读。
```

Insert these two rows in `## 按问题查阅`, keeping the Runtime row above them and the Binding row below them:

```markdown
| 理解要求水线、实际水线、read barrier、retry / fail closed / freshness，以及 retry 是否阻塞 Kafka | [水线与读取屏障详解](水线与读取屏障详解.md) |
| 实现 Current Fact、水线/barrier、coverage seed、冲突 fence、双档部署或存储选型 | [Scene Current Fact 设计](Scene%20current%20fact设计.md) |
```

Delete the old row beginning with `| 查询 Current Fact、水线/barrier` so there is only one Scene Current Fact implementation row.

Do not alter the user-owned percent-encoding change elsewhere in `README.md`.

- [ ] **Step 2: Replace the old guide link in the Current Fact specification**

In `Scene current fact设计.md` §5.2, replace the current design-tradeoff blockquote at lines 140-145 with:

```markdown
> **设计取舍注记**：水线滞后是写入者 durability 承诺的一部分，不是 Materializer 太慢。Scene Runtime
> 故意等待 `readyThroughNextOffset` 覆盖 `readBarrierId`，而不是直接读取 TDengine 中的最新 last 行，
> 是为了避免跨 partition 场景下“新 A 配旧 B”的部分值错误——这种错误一旦产生动作就无法被下一条数据
> 冲掉，百万级设备下会按工单计价。完整流程、数值算例与 retry 阻塞范围见
> [《水线与读取屏障详解》](水线与读取屏障详解.md#waterline-compare)；水线、freshness 与 fail closed
> 的规范决策顺序见[《算子语义与术语》§6.1](算子语义与术语.md#61-水线新鲜度与-fail-closed-的配合)。
```

- [ ] **Step 3: Add a short explainer link to the operator semantics**

Immediately after `## 6.1 水线、freshness 与 fail closed 的配合` in `算子语义与术语.md`, insert:

```markdown
> 如果需要先建立直观模型，再看本节规范顺序，可阅读
> [《水线与读取屏障详解》§5](水线与读取屏障详解.md#decision-order)。
```

- [ ] **Step 4: Add a short retry-scheduling link to the Runtime specification**

Immediately after the introductory paragraph ending in `此处不复制：` in Runtime §5, insert:

```markdown
> barrier retry 对 poll / heartbeat、Actor lane、partition completion 和其他 partition 的影响，见
> [《水线与读取屏障详解》§6](水线与读取屏障详解.md#retry-kafka)；以下条款仍是调度行为的唯一规范来源。
```

- [ ] **Step 5: Verify navigation and authority boundaries**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/iot-doc
dir='亿级点位高吞吐架构/6.规则引擎与场景联动'
rg -q '场景联动完整案例端到端时序图\.html' "$dir/README.md"
rg -q 'retry 是否阻塞 Kafka.*水线与读取屏障详解' "$dir/README.md"
test "$(rg -c '实现 Current Fact、水线/barrier' "$dir/README.md")" -eq 1
rg -q '水线与读取屏障详解\.md#waterline-compare' "$dir/Scene current fact设计.md"
rg -q '水线与读取屏障详解\.md#decision-order' "$dir/算子语义与术语.md"
rg -q '水线与读取屏障详解\.md#retry-kafka' "$dir/Scene Runtime 调度、分片与故障转移.md"
git diff --check -- \
  "$dir/README.md" \
  "$dir/Scene current fact设计.md" \
  "$dir/算子语义与术语.md" \
  "$dir/Scene Runtime 调度、分片与故障转移.md"
```

Expected: every command exits 0 and `git diff --check` prints nothing.

- [ ] **Step 6: Commit only the three clean normative files**

Because `README.md` contained a user-owned change before this task, leave the merged README change unstaged. Commit only the three previously clean normative documents:

```bash
cd /home/wangjunbo/workspace-iot-platform/iot-doc
git add -- \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/Scene current fact设计.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md'
git diff --cached --check
git diff --cached --name-only
git commit -m 'docs: connect scene barrier guidance'
```

Expected: the cached name list contains exactly the three normative Markdown files. `README.md` remains modified but unstaged, containing both the user's earlier change and this task's navigation edits.

### Task 4: Run final documentation verification

**Files:**
- Verify all files from Tasks 1-3
- Verify unchanged: both HTML files

- [ ] **Step 1: Verify the complete explanation chain and stable anchors**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/iot-doc
dir='亿级点位高吞吐架构/6.规则引擎与场景联动'
doc="$dir/水线与读取屏障详解.md"
for anchor in one-minute why-barrier scene-to-routes waterline-compare decision-order retry-kafka complete-case troubleshooting; do
  rg -q "<a id=\"$anchor\"></a>" "$doc" || exit 1
done
for target in \
  '水线与读取屏障详解.md' \
  '场景联动完整案例端到端时序图.html' \
  'Scene current fact设计.md' \
  '算子语义与术语.md' \
  'Scene Runtime 调度、分片与故障转移.md' \
  '控制面与匹配契约.md'; do
  test -e "$dir/$target" || exit 1
done
rg -q 'SceneEvent / SourceDelta' "$doc"
rg -q 'ExactTriggerIndex' "$doc"
rg -q 'SceneCurrentFactKey' "$doc"
rg -q 'CurrentFactBarrierSnapshotProvider' "$doc"
rg -q 'ListOffsets' "$doc"
rg -q 'readyThroughNextOffset' "$doc"
rg -q 'freshness' "$doc"
rg -q 'predicate / edge / duration / timer / action' "$doc"
```

Expected: all commands exit 0.

- [ ] **Step 2: Scan for the two corrected misconceptions**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/iot-doc
dir='亿级点位高吞吐架构/6.规则引擎与场景联动'
! rg -n 'factKey\s*[-=]+>\s*routeKey' "$dir/水线与读取屏障详解.md" "$dir/00-15分钟读懂场景联动.md"
! rg -n '同一 partition.*同一 Actor lane|同 partition.*同一 Actor lane' "$dir/00-15分钟读懂场景联动.md"
rg -n '不能写成|不是；partition 是 ownership/completion 边界' "$dir/水线与读取屏障详解.md"
```

Expected: the first two searches produce no output and exit 0 via `!`; the final search prints the explicit corrections.

- [ ] **Step 3: Verify protected HTML files and Git hygiene**

Run the same `sha256sum` command from Task 1 and compare the values with the recorded pre-edit values:

```bash
cd /home/wangjunbo/workspace-iot-platform/iot-doc
sha256sum \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动端到端时序图.html' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
git diff --check
git diff --cached --check
git status --short
git log -4 --oneline
```

Expected:

- both HTML hashes exactly match Task 1;
- both diff checks print nothing;
- no HTML file is staged by this implementation;
- `README.md` and the three pre-existing user files remain unstaged;
- recent history contains the explainer, compact-guide, and normative-link commits;
- the implementation did not commit the user's pre-existing README or appendix/HTML changes.

- [ ] **Step 4: Review the final rendered reading path**

Open `00-15分钟读懂场景联动.md` in VS Code Markdown Preview and verify:

1. no Mermaid render error or microscopic sequence diagram appears;
2. the “大字版完整时序图” link resolves to the existing complete-case HTML;
3. the five-stage fallback table remains readable;
4. the waterline section links to the new explainer;
5. the new explainer's tables and fenced text diagrams render without horizontal overflow at normal editor width.

Then open `场景联动完整案例端到端时序图.html` in a browser and verify browser zoom works. Do not edit the HTML while checking it.
