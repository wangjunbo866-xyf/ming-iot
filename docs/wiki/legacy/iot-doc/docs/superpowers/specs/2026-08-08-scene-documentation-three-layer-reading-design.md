# 场景联动文档三层阅读结构设计

> 日期：2026-08-08
>
> 状态：设计已确认，待实施计划
>
> 范围：`亿级点位高吞吐架构/6.规则引擎与场景联动/`

## 1. 问题

当前目录已经形成清晰的权威层级，但“权威性”和“阅读顺序”被绑定在一起：README 要求新读者
首先阅读 1,784 行的主规格，随后再阅读三份从属实施契约。四份规范性正文合计约 3,881 行：

| 文档 | 当前行数 | 当前职责 |
| --- | ---: | --- |
| `场景联动 Runtime 主规格总纲.md` | 1,784 | 唯一规范性架构决策源 |
| `控制面与匹配契约.md` | 482 | Binding、ProjectIndex 与 matching |
| `算子语义与术语.md` | 976 | 事实身份、状态与算子语义 |
| `SLO故障测试与实施.md` | 639 | SLO、POC、故障与实施门槛 |

大量概念需要跨文件反复出现。例如 `ProcessingContext`、`ProjectIndex`、timer 和 Outbox
分别承担贯穿性约束，单纯按文件顺序阅读无法先形成端到端心智模型。新读者常见结果是：知道
单词含义，却不能回答消息来自哪里、哪个程序处理、状态写在哪里，以及何时才算产生业务副作用。

问题不应通过删除必要契约或折叠 Markdown 解决。需要把“第一次理解系统”和“实现时查阅完整
契约”拆成两种阅读任务。

## 2. 目标与非目标

### 2.1 目标

- 新读者只读一篇导读，可在 15 分钟内串起一条复杂多设备规则的完整业务链路；
- 导读明确区分程序节点、Kafka Topic、Consumer Group、状态真源和事务边界；
- 导读明确标记 `[现有实现]`、`[目标设计]` 与 `[待 ADR]`，不把目标架构写成现状；
- 读者理解全链路后，可按具体问题直接下钻到唯一权威章节；
- 保持当前主规格和三份从属契约的权威关系不变；
- 所有未冻结的物理名和完整枚举保持未冻结，不为图示方便补造名称。

### 2.2 非目标

- 本轮不重新评审 Scene Runtime、Kafka、Reducer、ComputationPlan 或 Command 架构；
- 不移动、不重命名或拆分现有长文档；
- 不把导读变成第二套规范或字段全集；
- 不依靠本地 HTTP 服务或视觉伴侣才能阅读；
- 不在本轮修改产品代码、DDL、Proto、Topic 或 Consumer Group 配置。

## 3. 目标读者与成功标准

第一目标读者是第一次接触这套架构、了解 IoT/Kafka 基本概念但尚未建立 Scene 心智模型的开发、
测试或评审人员。实施人员仍以第三层规范为准。

读完导读后，读者应能不翻阅其他正文回答：

1. Scene 的四类受信事实来源分别是什么；
2. 为什么 Lifecycle/CommandReply 不能直接触发 Scene；
3. Reducer 收敛什么状态，何时发布 `AcceptedStateTransition`；
4. `ComputationPlan` 在哪里生成、在哪里执行，Scene 为什么不保存窗口样本；
5. 为什么同一来源流会被多个 Consumer Group 并行消费；
6. Current Fact、SceneBindingState 与 CommandExecution/Attempt 分别写在哪里；
7. Kafka 重投为什么不应产生重复动作；
8. 哪个事务提交后才叫 action acceptance；
9. 哪些节点已经存在，哪些只是目标设计或待 ADR。

## 4. 三层阅读结构

```text
6.规则引擎与场景联动/
├─ README.md                         # 第一层：阅读路由，最多 80 行
├─ 00-15分钟读懂场景联动.md          # 第二层：心智模型，最多 250 行
│
├─ 场景联动 Runtime 主规格总纲.md
│                                      # 第三层：唯一规范性架构源，按问题查阅
├─ 控制面与匹配契约.md                # 第三层：Binding / Scope / Matching
├─ 算子语义与术语.md                  # 第三层：Fact / State / Operator / Timer
├─ SLO故障测试与实施.md               # 第三层：SLO / POC / Rollout / RACI
│
├─ 附录-竞品状态方案调研.md           # 非规范性研究
└─ 附录2-thingsboard规则引擎与CF实现分析.md
                                       # 非规范性源码研究
```

### 4.1 第一层：README 阅读路由

README 只回答“我应该读哪里”，不解释完整架构。入口分为：

- **第一次阅读**：唯一指向 `00-15分钟读懂场景联动.md`；
- **按问题查阅**：按 Binding/Scope、Matching、Fact/Identity、State/Timer、Outbox/Command、
  Kafka/Recovery、SLO/POC 等问题链接到具体文件和章节；
- **确认权威性**：保留主规格唯一规范性架构决策源、三份从属实施契约和两份非规范性附录的关系；
- **确认状态**：保留核心方案、恢复 ADR、POC 与冻结状态说明。

README 控制在 80 行以内。它不得复制数据流、状态机、字段或失败窗口正文。

### 4.2 第二层：15 分钟导读

导读只负责建立心智模型，控制在 250 行以内。固定阅读预算如下：

| 时间 | 内容 | 读完应获得什么 |
| ---: | --- | --- |
| 1 分钟 | 一句话结论与状态图例 | 知道 Scene 做什么，以及现状/目标/待 ADR 的区别 |
| 2 分钟 | 参与者与状态所有权表 | 知道每个程序节点负责什么、写什么 |
| 6 分钟 | 标准端到端时序图 | 串起上报、计算、求值、动作和回复收敛 |
| 3 分钟 | 三类容易混淆的状态 | 区分 Current Fact、SceneBindingState、CommandExecution/Attempt |
| 2 分钟 | 失败、重投与事务边界 | 理解为何不会因普通重投直接重复动作 |
| 1 分钟 | 按问题下钻 | 找到下一篇权威章节 |

导读中的技术名词第一次出现时，先用一句业务中文解释，再给英文名称。完整 wire、hash 公式、
状态字段、故障矩阵和容量公式只链接，不复制。

### 4.3 第三层：规范与研究资料

现有主规格、三份从属实施契约和两份附录暂不拆分。每篇规范顶部增加不超过 15 行的阅读卡片：

```text
本文回答什么
本文不回答什么
谁需要读
建议从哪些章节开始
权威关系
```

长文档允许超过 1,000 行，因为它们变成按问题查阅的 reference，而不再是新读者线性必读材料。
阅读卡片不得建立新的权威覆盖规则。

## 5. 唯一贯穿示例

导读只使用一个复杂多设备规则，不在不同章节切换无关示例：

> R1 区域的 `temp-01/02/03` 计算 5 分钟平均温度；平均值超过 30℃，同时
> `door-01=CLOSED`，并持续 2 分钟后，向 `fan-01/02` 下发启动命令并发送 Webhook。
> 设备回复由 Command reducer 收敛，合法命令结果再形成 `AcceptedStateTransition`。

该示例依次覆盖：

- 多设备属性上报与 Raw/Canonical；
- 共享 `ComputationPlan` 和 Flink 窗口状态；
- `ComputedMetricUpdated` 的 Current Fact 投影与 Scene Admission；
- `door-01` Current Fact 与 latest-value join；
- duration arm、timer fire 和回 lane 复核；
- 多设备 action fanout、PG Action Outbox 与 `effectId`；
- CommandExecution/Attempt、设备回复和 Command reducer；
- `AcceptedStateTransition` 回到 Scene 输入边界；
- duplicate、stale、失败与重投的停止位置。

示例数值只用于解释流程，不成为产品默认阈值、容量基线或冻结业务规则。

## 6. 标准端到端时序图

导读必须直接嵌入标准 Mermaid `sequenceDiagram`，不能用导航卡片、流程框图或自定义泳道图冒充
时序图。图不依赖 `localhost` 服务；即使阅读器不能渲染 Mermaid，源码中的消息顺序仍须可读，
图后还要提供等价的阶段表。

### 6.1 参与者

时序图至少区分以下角色，不得把 Topic 和处理程序画成同一个节点：

- `temp-01/02/03`、`door-01`、`fan-01/02`；
- EMQX / Access Adapter；
- Kafka Raw Topic；
- ProtocolApplication；
- Kafka Canonical Telemetry / CommandReply observation Topic；
- Flink Computed Metric Runtime；
- Computed Metric 结果流；
- ComputedMetricCurrentFactProjector；
- Scene Admission；
- Kafka `scene_event`；
- Scene Runtime；
- PostgreSQL Action Outbox；
- Command Runtime；
- Command reducer。

为控制宽度，同类设备可以在一条 lifeline 中列出具体设备名；不同职责的程序和 Topic 不得为了
缩窄图片而合并。Mermaid `sequenceDiagram` 的 lifeline 统一使用 `participant`；Kafka Topic、
PostgreSQL 和状态存储必须在 participant label 中分别带 `«Kafka Topic»`、`«PostgreSQL»`、
`«State Store»` 类型前缀，不能与处理程序共用 lifeline。使用 Mermaid `box` 按 Device、
Ingress/Protocol、Computed Metric、Scene、Command 分组。

### 6.2 每条关键消息必须标注的内容

每一段 Kafka 交互都必须能回答：

```text
Topic
消息类型
Producer
Consumer Group
关键业务字段或 identity
```

已冻结的物理名直接写出；只有逻辑流已定义而物理 Topic/Group 未冻结时，统一写成
`<logical-name：待 ADR>`，禁止使用看似正式的自造名称。程序节点同时标记：

- `[现有实现]`：当前仓库能定位到对应实现；
- `[目标设计]`：架构已指定职责，但实现尚不存在；
- `[待 ADR]`：连物理契约或边界仍待冻结。

### 6.3 时序阶段

一张连续时序图按 Mermaid `rect`/`Note` 划分为五个阶段：

1. **上报与 Canonical**：设备 → Adapter → Raw → ProtocolApplication → Canonical；
2. **共享计算与事实投影**：Canonical → Flink → ComputedMetricUpdated，同时由唯一 projector
   写 Current Fact，并由独立 Scene Admission group 生成 SceneEvent；
3. **Scene 求值**：Scene Runtime 合并门状态 Current Fact，arm duration，timer 到期回同 lane
   复核；
4. **动作接受与投递**：同一 PG 事务写 action rows/effect receipt，commit 后才注册 state/timer
   并由 Command Runtime 投递两台风机；
5. **回复收敛**：CommandReply observation → Command reducer → command state/outcome →
   accepted-transition Outbox → AcceptedStateTransition。

图中使用 `par` 表达不同 Consumer Group 对同一来源流的并行消费；使用 `alt` 表达 duplicate、
stale/conflict、Outbox commit 失败和正常 accepted 分支；使用 `loop` 只表达 Kafka redelivery，
不得暗示设备动作 exactly-once。

### 6.4 图后辅助表

时序图后紧跟两张小表：

1. **节点职责与状态写入位置**：程序节点、消费输入、唯一写入状态、输出；
2. **关键 identity 传递**：`outputId → sourceEventId/triggerId → transitionId/effectId →
   commandId/attemptNo → causationOutputId`。

表格是图的索引，不重复完整字段契约。

## 7. 权威性与去重规则

- 导读是解释性入口，不是规范性架构决策源；冲突时不得用“导读更易懂”覆盖主规格；
- 导读每个关键结论必须链接到现有唯一权威章节，不能只链接到整篇文档顶部；
- 导读只保留业务解释和最小示例，字段全集、状态迁移全集、失败矩阵与配额留在第三层；
- README 不重复导读；第三层阅读卡片不重复正文；
- 同一术语在导读中只解释一次，后文使用锚点回链；
- 若实施时发现现有规范彼此冲突，先停止摘录并联合修订规范，不能在导读中自行裁决；
- 长规格未来是否拆分，必须基于实际查阅反馈另立设计，不纳入本轮。

## 8. 失败与降级处理

- **Mermaid 无法渲染**：保留可读源码，并由图后阶段表提供等价顺序，不要求启动本地服务；
- **目标锚点不存在或不稳定**：先在权威文档补稳定标题，再建立链接，不链接模糊行号；
- **Topic/Group 未冻结**：显示逻辑角色和 `[待 ADR]`，不省略，也不补造物理名；
- **现状无法证明**：降级标记为 `[目标设计]`，不能根据目录名或设计文本推断已实现；
- **导读超过 250 行**：把字段、矩阵或补充例子移回第三层，以链接替代；
- **一个图因参与者过多不可读**：先缩短 participant alias、合并同类设备并把字段移到辅助表，
  不合并职责不同的程序/Topic，也不改成非标准时序图。

## 9. 实施顺序

本轮实施保持 additive-first：

1. 新建 `00-15分钟读懂场景联动.md`，先完成正文、时序图和权威链接；
2. 修改 README，将第一次阅读入口切换到导读，并增加按问题查阅表；
3. 给四份规范性正文增加阅读卡片；
4. 验证链接、Mermaid、行数、状态标记和术语一致性；
5. 不删除、不移动、不重命名现有文件。

若导读存在问题，只需撤回新增入口和阅读卡片即可恢复原阅读方式，不影响任何规范正文。

## 10. 验收

### 10.1 机械检查

- README 不超过 80 行；
- 导读不超过 250 行；
- Markdown link checker 无失效的本地文档锚点；
- Mermaid parser 能解析 `sequenceDiagram`；
- 图中的 Kafka 交互均能在相邻文字或表格中找到 Topic、消息、Producer、Consumer Group；
- `[现有实现]` 均有仓库路径证据，未冻结项均带 `[待 ADR]`；
- 原有文件名、路径和权威声明未改变。

### 10.2 15 分钟读者测试

找一名未参与本设计的开发或测试人员，只提供 README 和导读，限时 15 分钟。随后用 §3 的九个
问题验收：至少答对八个，且不能把 Flink/Reducer 说成当前 `things` 已有实现，方可通过。

若失败，优先修正文顺序、名词解释和图中标签，不向导读继续堆叠完整契约。

## 11. 完成定义

同时满足以下条件才算三层阅读结构落地：

- 新读者入口不再要求先通读 1,784 行主规格；
- 一个复杂多设备规则贯穿文字、标准时序图和状态说明；
- 图能明确回答程序节点、Topic、Consumer Group、状态写入位置和事务边界；
- 所有关键结论均可下钻到现有权威章节；
- 导读没有新增或悄悄冻结任何架构契约；
- 15 分钟读者测试通过。
