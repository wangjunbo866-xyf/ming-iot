# 场景联动水线与读取屏障文档整理设计

## 背景与目标

当前“水线 / read barrier / freshness / retry / fail closed”分别散落在导读、Scene Current Fact 规范、算子语义和 Runtime 调度文档中。规范信息基本完整，但读者需要跨文档拼接以下关键链路：

1. SceneEvent 如何经过 matching / fanout 找到 Binding；
2. Binding 参数如何同时映射到 Current Fact 查询 key 和 Kafka source route；
3. `CurrentFactBarrierSnapshotProvider` 捕获的 Kafka 末尾与谁比较；
4. 水线、freshness、retry、fail closed 的先后关系；
5. retry 对 Kafka poll、Actor lane、partition completion 和其他 partition 的影响。

本次整理新增一篇非规范性的《水线与读取屏障详解》，作为上述问题的统一解释入口；规范性定义仍由原架构文档拥有。与此同时，将《15 分钟读懂场景联动》中不可读的超宽 Mermaid 替换为完整案例 HTML 图的外链和文字降级摘要。

## 已确认方案

采用“独立解释文档 + 规范文档保留定义并互相引用”的分层方案：

- 新文档负责建立心智模型、串起完整流程、给出算例并回答常见疑问；
- 规范文档继续拥有字段、算法、状态码、调度和失败语义，不把规范定义迁走；
- 导读只保留短结论和阅读入口，不再复制大段 barrier 说明；
- 完整案例时序图使用现有 HTML 文件，不修改两份 HTML。

## 文档权威边界

新增《水线与读取屏障详解》是解释性文档，不改变现有权威层级：

- `Scene current fact设计.md` 唯一拥有 Current Fact 投影、水线、barrier、coverage、读取 API、FULL / DIRECT_TD 语义；
- `Scene Runtime 调度、分片与故障转移.md` 唯一拥有 lane 串行、partition completion、poll/heartbeat、pause/backpressure 和故障转移协议；
- `算子语义与术语.md` 拥有 readiness、coverage、freshness、predicate/FSM 的求值顺序和术语定义；
- 新文档若与上述规范冲突，以规范为准，并要求在同一变更中修正文档冲突，不能形成另一套算法。

新文档开头必须显式说明：本文的“读取屏障”特指 Scene Runtime 读取 Current Fact 时捕获的固定 source-end vector，不是配置发布屏障、Binding activation barrier、restore cut 或 Kafka consumer committed offset。

## 变更范围

### 新增文件

- `亿级点位高吞吐架构/6.规则引擎与场景联动/水线与读取屏障详解.md`

### 修改文件

- `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md`
- `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`
- `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene current fact设计.md`
- `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`
- `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md`

后三份规范文档只增加短引用或范围提示；不迁移、不删减其规范条款。

### 明确不修改

- `场景联动端到端时序图.html`
- `场景联动完整案例端到端时序图.html`

工作区已有的 HTML、README 和附录改动属于用户，实施时必须保留并在其上做最小合并，不得覆盖或还原。

## 新文档结构

### 1. 一分钟结论

用四句话先给答案：

1. Kafka `ListOffsets` 捕获的 source-end vector 是本次读取要求达到的水线，即固定 `readBarrierId`；
2. Current Fact 写入端发布的 `readyThroughNextOffset` 是实际投影进度；
3. 只有每条必要 source route 的实际进度都覆盖要求进度，Runtime 才能读取事实并检查 freshness；
4. 未覆盖时复用同一 barrier 做有限 retry，耗尽后 fail closed，本轮不产生动作。

### 2. 为什么需要屏障

使用“设备 A 已更新、设备 B 的投影仍落后”的错误联动示例说明：直接读取存储中的 latest 会把 A 的新值和 B 的旧值拼在一起。外部动作一旦接受或下发，后续事实不能撤销先前副作用，因此系统必须先证明所需来源已经投影完整，再进行业务求值。

本节同时说明 barrier 的保证边界：

- 它是固定的可读完整性下界；
- 它不是跨 partition 的 event-time 原子快照；
- 屏障满足后允许读到比捕获末尾更新、且 comparator 接受的 Current Fact；
- 不同 `(sourceStreamId, partition, projectionGeneration)` 的 offset 不能横向比较。

### 3. 从 SceneEvent 到必要参数

按真实执行顺序解释：

1. Runtime 接收 `SceneEvent / SourceDelta`，其中包含来源主体、事实增量和 source position；
2. `ExactTriggerIndex[(factType, factKey)]` 找到候选 TriggerPosting；
3. 经过 enabled、source、scope、target 等过滤后，按 evaluation subject 聚合为 Binding bundle；
4. Binding 的参数定义给出每个必需事实，例如温度计算指标、门状态或设备 B 湿度；
5. Binding 参数只形成 Current Fact 查询集合；当前 record trigger 的 route 直接由传播的
   `SourceDelta.sourcePosition -> (sourceStreamId, partition, recordOffset)` 给出。

本节必须纠正“factKey 推导 routeKey”的误解，明确三条独立链路：

```text
事实查询：Binding 参数
        -> (tenantCode, factOwnerKey, factType, factKey)
        -> SceneCurrentFactKey

当前 record trigger route：传播的 SourceDelta.sourcePosition
        -> (sourceStreamId, partition, recordOffset)

其他必要 context 参数，以及 timer recheck / schedule / lifecycle 等非 record 求值所需 route：
        -> 发布规范 §9“必要 source route 解析契约” blocker
        -> 完整解析算法尚未冻结
```

`factKey` 只回答“读取哪个事实”，不派生 routeKey；当前 record 的 route 来自传播的
`SourceDelta.sourcePosition`。其他必要 route 必须等待发布规范 §9 的解析契约冻结，不能臆造
wire 字段或 resolver，也不能写成 `factKey -> routeKey -> partition`。

### 4. 要求水线从哪里来、和谁比较

说明 `CurrentFactBarrierSnapshotProvider` 的完整生命周期：

1. 一次 logical trigger 首次读取时登记全部必要 source route；
2. provider 对并发登记做 micro-batch；
3. 只有登记完成后启动的 capture cycle 才能满足该请求；
4. provider 对本周期 route 并集调用 Kafka `ListOffsets`；
5. 返回不可变 source-end vector，并生成 `readBarrierId`；
6. 同一 trigger 的 retry 必须复用该 barrier，不得每次重新捕获并追逐移动的 broker end。

比较公式写成：

```text
对 readBarrierId 中每个 route r：

actual.readyThroughNextOffset(r, generation)
    >= required.capturedEndNextOffset(r)
```

其中：

- `required.capturedEndNextOffset` 来自 Kafka `ListOffsets`，是要求水线；
- `actual.readyThroughNextOffset` 来自 Current Fact writer 发布的连续 input-completion 水线；只有在该
  input 的所有适用目标、规范要求的必要终态证据均 durable，且此前无 offset 空洞时才发布；
  不需要 Scene-live value-slot mutation 的 input 同样可以完成，不要求写入事实行；
- 当前触发记录所在 route 的要求值至少覆盖 `recordOffset + 1`；
- generation 不一致时不能比较，必须返回 not ready 或 conflict，而不是拼接新旧代水线。

文档给出一个两 route 数字算例。例如要求向量为 `p3=101, p7=260`，实际向量为 `p3=101, p7=257`，则整体为 `BEHIND`；当 `p7` 达到 260 后才进入事实读取。

### 5. retry、fail closed 与 freshness

固定决策顺序：

```text
捕获/复用固定 barrier
        |
        v
readiness / waterline 是否覆盖？ -- 否 --> bounded retry
        |                                  |
       是                                  +-- 耗尽 --> fail closed
        v
coverage / conflict 是否通过？ ---- 否 --> bounded retry 或明确失败处置
        |
       是
        v
读取 Current Fact 并检查 freshness
        |
        +-- 过旧 --> 按 missing/freshness policy
        |
        +-- 新鲜 --> predicate / edge / duration / timer / action
```

三者的区别通过表格表达：

| 概念 | 回答的问题 | 失败时的处理 |
| --- | --- | --- |
| 水线 / readiness | 必要来源是否已经完整投影到固定 barrier | 复用同一 barrier 有限 retry；耗尽后 fail closed |
| freshness | 已知事实是否仍满足业务允许的最大年龄 | 按显式 missing/freshness 策略处理，不冒充投影失败 |
| fail closed | 系统无法证明输入完整或可信时是否允许产生副作用 | 不进入业务 predicate，不执行动作，记录可观测的最终失败处置 |

必须强调：

- 水线未 ready 时不允许先做 freshness，也不允许读取旧值降级求值；
- fail closed 不等于把 predicate 判为 false，也不等于业务 missing；
- fail closed 后，本次 input 只有完成 quarantine 或获批准的最终失败处置才可满足 completion，避免无限占住 offset；
- freshness 失败发生在“数据完整但太旧”之后，可按策略保持 ARMED、取消或返回 NOT_READY；
- FULL 档严格执行上述顺序；DIRECT_TD 仅把严格 barrier 降级为软 lag 门，并明确牺牲的保证。

### 6. retry 会不会阻塞 Kafka 其他消息

直接回答：“不会阻塞 Consumer Group 的 poll / heartbeat 线程，但会按顺序边界造成局部阻塞和背压。”

分四层说明：

| 范围 | retry 期间的行为 |
| --- | --- |
| Consumer poll / heartbeat | 不得在回调或 poll 线程中 sleep、同步等远端水线；重试由异步调度承载 |
| 同一 source/evaluation-subject lane | 当前输入未完成时，同 lane 后续输入不能越过 |
| 同一 Kafka partition | 不同 subject lane 仍可并发，但 completion sequencer 不能越过最早未完成 offset；积压达到有界阈值后可 pause / throttle 该 partition |
| 其他 ACTIVE partition | 拥有独立 PartitionRuntimeContext，可继续 poll、dispatch、完成与提交 |

同时说明 fail closed 的必要性：bounded retry 给投影追平机会，终止重试则保证单条坏输入不会永久钉住 lane 和 partition completion frontier。重试窗口、退避、in-flight 上限和最终失败处置仍引用 Runtime / SLO 规范，不在解释文档中另造默认数值。

### 7. 完整案例映射

将概念映射到“3 个温度点求 5 分钟平均值 + 门状态持续满足 + 启动两台风机”的完整案例：

- 触发事实可以是 `ComputedMetricUpdated` 或门状态变化；
- Binding 需要读取 5 分钟平均温度和门状态两个参数；
- 两个参数可能来自不同 stream / partition / writer，因此 barrier 是 route vector；
- 首次条件成立只 arm duration；2 分钟 timer 到期后还要重新读取、重新检查 barrier、freshness 和 predicate；
- 只有复核成功，PG action-acceptance transaction 才能接受两台风机命令和 Webhook。

本节链接 `场景联动完整案例端到端时序图.html`，不复制 SVG 或 Mermaid。

### 8. 常见误解与排障

至少回答以下问题：

- `ListOffsets` 拿到的值和哪个值比较？
- 为什么不能直接查 TDengine/latest？
- 为什么 retry 不能重新抓一次更晚的 broker end？
- 同 partition 是否等于同一个 Actor lane？答案是否定的；partition 是 ownership/completion 边界，lane 按 subject key 串行。
- barrier ready 是否代表拿到历史时点快照？答案是否定的。
- 没有 Current Fact row 是业务 missing 还是投影没追上？必须结合水线与 coverage seed 判定。
- fail closed 后 Kafka offset 是否永远不提交？答案是否定的；获批准的最终失败处置完成后，input 可进入 completion，但不会产生业务动作。

结尾提供状态码和排障顺序：`PROJECTION_NOT_READY / BEHIND / LOOKUP_FAILED / PROJECTION_CONFLICT`、waterline lag、barrier age、retry exhausted、partition in-flight/backpressure。

## 《15 分钟读懂场景联动》调整

### 时序图

删除第 5 节超宽 Mermaid 源码，替换为醒目的本地 HTML 链接：

- 标题说明这是“3 个温度点求平均值 + 门状态 + 两台风机 + Webhook”的完整案例；
- 提示在浏览器打开后使用 `Ctrl/Cmd + +/-` 缩放或全屏查看；
- 保留现有“五步读图”表格作为无法打开 HTML 时的文本降级；
- 不在 Markdown 和 HTML 中维护两份同源时序，避免内容漂移。

### 水线章节

将当前长篇 barrier 说明收敛为：

- 一段“为什么不能用新 A + 旧 B”的结论；
- 一个 required waterline 与 actual waterline 的最小比较式；
- 一个指向《水线与读取屏障详解》的链接。

删除或改正“同 partition 的多台设备由同一 Actor lane 顺序处理”表述。正确说法是：同一 subject-key lane 保序；同 partition 可以包含多个并发 lane，但 offset completion 必须连续推进。

## README 与规范文档导航

- `README.md` 的“第一次阅读”改为链接完整案例 HTML；“按问题查阅”新增《水线与读取屏障详解》入口，并把“概念理解”和“实现规范”分开描述。
- `Scene current fact设计.md` 在水线 / barrier 规范段落增加一条“通俗流程与算例见详解”的链接。
- `算子语义与术语.md` 在 §6.1 增加详解链接，不重复新文档正文。
- `Scene Runtime 调度、分片与故障转移.md` 在正常调度 / completion 附近增加一条链接，指向详解中的“retry 会不会阻塞 Kafka”章节；原调度契约不变。

## 链接与命名约束

- 文件标题使用“水线与读取屏障详解”；正文第一次出现时同时给出英文 `waterline`、`read barrier`。
- 链接统一使用相对路径；含空格的既有文件名沿用仓库当前可工作的 URL 编码风格。
- 新文档使用稳定的显式 HTML anchor 或不易变化的中文标题，供其他文档定向链接。
- `readBarrierId` 表示不可变 barrier 身份；不要把单个 partition offset 或 writer waterline 也称为 `readBarrierId`。
- 用 `next-offset` 语义举例，避免把 next-offset 和最后一条已处理 record offset 混为一谈。

## 验证

实施完成后执行以下检查：

1. 确认新增文档存在，并包含 SceneEvent → Binding → query key / route → ListOffsets → fixed barrier → waterline compare → freshness → predicate 的完整链路；
2. 搜索 `00-15分钟读懂场景联动.md`，确认超宽 Mermaid 已移除，五阶段文字表仍保留，完整案例 HTML 链接有效；
3. 检查新文档明确出现 `retry`、`fail closed`、`freshness`、`poll/heartbeat`、`lane`、`completion frontier` 和“其他 partition”说明；
4. 检查公式两侧分别是 `capturedEndNextOffset` 与 `readyThroughNextOffset`，且 generation 条件存在；
5. 检查没有残留 `factKey -> routeKey -> partition` 和“同 partition 等于同 lane”的错误表述；
6. 运行仓库现有 Markdown 链接检查；若无专用工具，则以脚本验证本次新增的全部相对文件链接和锚点；
7. 比较 Git diff，确认两份 HTML 未被本次修改，用户原有 README、HTML 和附录改动均被保留；
8. 复核所有新增解释与三份规范文档一致，没有引入新的超时、重试次数或状态码默认值。

## 验收标准

- 读者只看新文档，就能回答“要求水线从哪里来、实际水线从哪里来、二者如何比较”；
- 读者能区分水线未 ready、事实不新鲜和 fail closed，不再把三者都理解成 missing；
- 读者能准确解释 retry 的局部阻塞范围，并知道它不应阻塞 Kafka poll / heartbeat；
- 《15 分钟读懂场景联动》在 VS Code 中不再渲染超宽 Mermaid，完整案例可通过 HTML 大图阅读；
- 原规范文档仍是唯一规范来源，新文档没有造成权威层级变化；
- 两份现有 HTML 内容保持不变。
