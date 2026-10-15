# Raw → Canonical 契约层：语义、成本与分期

日期：2026-07-24
状态：分析与决策参考。解释 [主架构](../enterprise-iot-platform-architecture-design.md) §4.2/§7 契约层"为什么、买什么、何时付"。本文不修改主架构任何冻结项；§6 的分期建议涉及冻结项，采纳需走正式变更评审。

## 1. 定位：拓扑不超规格，契约密度才超规格

Raw → Canonical 双 Topic 经常被质疑"没有主流物联网平台这么做"。事实要分两层说：

**双跳拓扑是云级/工业级的标准形态**，先例充分：

| 平台/体系 | 做法 |
| --- | --- |
| Azure IoT Hub | 内置 Event Hubs 兼容端点即 durable raw，payload 为原始设备字节，保留 1~7 天；规范化交给下游 |
| 微软官方参考架构（iomt-fhir 等） | Ingest Event Hub → Normalize → **第二个 Event Hub**，即产品化的 Raw → Canonical |
| Azure IoT Operations | MQTT broker + dataflow（source → transform → destination）+ schema registry，边缘侧产品化双跳 |
| AWS IoT Analytics（历史） | Channel（原始消息持久存储、pipeline 变更可重算）→ Pipeline（ETL）→ Datastore（处理后存储），带重放 |
| Eclipse Hono（博世系） | protocol adapter 写租户级 durable Kafka topic，规范化交给 Ditto 等下游，即"只有 Raw、schema-on-read"形态 |
| ThingsBoard 微服务模式 | 设备数据实际过 Kafka 两次（`tb_transport.api.requests` → core → `tb_rule_engine`），但两跳均为服务解耦，**没有语义转换边界** |
| 实时数仓通用拓扑 | Kafka → Flink 实时 ETL 标准化 → Kafka → 下游；Raw → Canonical 是该模式在 IoT 的名字 |

**反面对照同样重要：国内云厂商与消费级 PaaS 一致选择了"无 durable Raw"形态**——

| 平台 | 数据通路 | 解码位置与形态 | durable 层 | 重放/结局语义 |
| --- | --- | --- | --- | --- |
| 阿里云 IoT 平台 | 设备 → 平台 → 数据解析脚本（云端脚本把透传/自定义格式转 Alink JSON）→ 规则引擎云产品流转 | 平台边缘，每产品控制台脚本 + 物模型 TSL | 服务端订阅队列（AMQP/MNS/RocketMQ），承载**解析后** Alink 消息 | 无 durable raw、无重放、无结局语义 |
| 华为云 IoTDA | 设备 → IoTDA → 在线编解码插件（二进制 → 平台 JSON；JSON 直通免插件）→ 规则引擎数据转发 → DIS/Kafka/OBS/GaussDB/ROMA | 平台边缘，在线编解码插件 | 转发到 DIS/Kafka 等，承载**解析后** JSON | 同上 |
| 腾讯云物联网开发平台 | 设备 → 数据模板（物模型）+ 数据解析脚本 → 规则引擎 → CKafka/CMQ/CTSDB/MySQL/第三方 | 平台边缘，控制台脚本 | 转发到 CKafka 等，承载**解析后**消息 | 同上 |
| 涂鸦智能（含 Cube 私有云） | 设备（涂鸦 DP 协议）→ 涂鸦云归一化为 DP 模型 → 定制 Pulsar 消息订阅（AES 加密）推给开发者 | 平台即协议：DP 模型固定，不存在客户 codec | Pulsar 订阅，承载**解析后** DP 状态/事件 | 同上 |

这组对照给出三个事实判断：

1. **四家一致把 durable 层放在解码之后**——服务端订阅/转发队列承载的都是 canonical-ish 消息，没有一家向客户暴露 durable raw。即它们站在"无 durable Raw、durable 只留 Canonical 侧"（与 JetLinks 同侧）；该形态成立的前提恰是客户没有重放与协议治理诉求——§5 规则 2 警告的"砍 Raw 不可逆、证据无法补录"，对观测型数据不是损失而是成本节约。
2. **它们的协议治理是控制台脚本/插件级的轻治理**：脚本随产品走，无不可变 release、无生效边界、无重放需求——因为其设备以 JSON/标准 DP 为主，二进制场景靠"在线插件"个案解决。本设计面向的多厂商二进制工业协议（Modbus 全量上报等）、codec release 治理与修复后重放，是四家都没有也不需要的能力。
3. **涂鸦代表极端形态**：平台即协议（DP 模型固定），连 codec 治理都不存在；其 Cube 私有云同样只提供解析后 Pulsar 订阅——说明即使私有化交付，消费级 PaaS 也不需要 Raw 层。"私有化"本身不构成要 Raw 的理由，客户数据形态才是。

**结构性上限进一步证实：几百点位的单设备全量上报落在前三家的设计域之外**——

| 平台 | 官方结构上限 | 对 500 点/设备·次全量上报的含义 |
| --- | --- | --- |
| 阿里云 | 单次最多上报物模型属性 **30 个**；单模块功能总数 ≤300；MQTT 消息 ≤256KB 超限直接拒绝 | 一条全量报文须拆 17+ 条消息；500 点物模型须拆多个模块 |
| 腾讯云 | MQTT 消息 ≤**16KB** 超限直接拒绝；设备上行约 20 msg/s | 500 点 JSON 单报文物理装不下 |
| 华为云 | 物模型属性上报按"服务"组织小属性集；另有 ≤1MB "消息上报"通道，但平台**不解析、不存储**，须转发下游自行处理 | 大批量场景被推向透传通道 = 平台明示放弃解析职责 |

并且**三家的解析是接入受理路径上的同步动作**：消息超限在 MQTT 层直接拒绝；物模型校验失败给错误应答或丢弃；解析脚本异常即丢弃（仅云端运行日志留痕）。"解析成功=上传成功、解析失败=上传失败"意味着**失败即终态**——没有 durable raw，就没有"修好脚本/插件后重放"的第二次机会。这不是少了一跳的偷懒，而是与其设备侧契约（小消息、请求-应答、同步校验）自洽的整体设计：受理与解释合一，才能保证应答语义即时可信。代价就是设计域被封死——几百点位二进制全量上报从消息上限开始就不在它们的场景内。这恰是本设计把 decode 移出受理路径、用 durable raw 将"受理"与"解释"解耦的根本原因：500 点全量报文必须先被无条件接住，解释的成败与对错是后面独立可修复的事。

参考来源：[阿里云 IoT 功能特性](https://help.aliyun.com/zh/iot/product-overview/features)、[阿里云 IoT 使用限制](https://help.aliyun.com/zh/iot/product-overview/limits)、[华为云 IoTDA 产品介绍](https://support.huaweicloud.com/productdesc-iothub/iot_04_0002.html)、[华为云设备数据上报](https://support.huaweicloud.com/intl/zh-cn/usermanual-iothub/iot_01_0322.html)、[华为云数据转发流程](https://support.huaweicloud.com/intl/zh-cn/usermanual-iothub/iot_01_0024.html)、[腾讯云规则引擎概览](https://cloud.tencent.com/document/product/1081/61101)、[腾讯云产品限制](https://cloud.tencent.com/document/product/1081/34730)、[腾讯云物联网通信产品限制](https://cloud.tencent.com/document/product/634/15241)、[腾讯云规则引擎转发目标](https://cloud.tencent.com/document/product/1081/103633)、[涂鸦 Pulsar 消息推送](https://developer.tuya.com/cn/docs/iot/Pulsar-SDK-get-message?id=Kan0klj9qbv3l)。

**真正没有主流平台（含云厂商）产品化的是契约密度**：outputs+offsets 同事务、`NO_OUTPUT`、分级 quarantine、offset-vector release timeline、ProcessingProvenance。因此"是不是太严谨"的辩论应该开在契约密度上，而不是跳数上。跳数上没有可省的钱；契约密度上有可分期的工程量。

五项契约合起来只实现一条总不变量：

> **每一条 Raw 都有且仅有一个可审计的确定性结局，且该结局用当时生效的不可变 release 解释、可复算。**

## 2. 五项契约逐项拆解

每项按同一结构评估：消灭的故障 → 主流替代 → 真实成本 → 结论。

### 2.1 outputs + offsets 同一 Kafka 事务（主架构 §7.2）

- **消灭的故障**：双写竞态。每处理一条 Raw 要做两件持久化的事——产出 Canonical/quarantine/`NO_OUTPUT`、推进 Raw offset。不做事务就有两个窗口：先发布 output 后提交 offset，崩溃 → Canonical 重复；先提交 offset 后发布，崩溃 → 静默丢失。`sendOffsetsToTransaction` 把窗口压成零，下游 `read_committed`。
- **主流替代**：Kafka Streams EOS 是同一原语——在流处理框架世界已产品化，在 IoT 平台世界没有。ThingsBoard 走的是更省事的路：**靠 TSDB upsert 语义吸收重复**（同 device+ts 重写即覆盖）。本项目 TDengine 同时间戳 update 语义同理，点值层面也能这么吸收。
- **诚实评估**：对点值本身，幂等 sink（`outputId`）+ TDengine upsert 已能兜底。事务不可替代的价值在点值之外：quarantine、`NO_OUTPUT`、audit outcome 这些结局记录不是点值，没有 ts+key 可覆盖去重——**"每条 Raw 恰有一个结局"只有事务能证明**。
- **成本**：zombie producer fencing（`transactional.id` + epoch）、`read_committed` 的 LSO 延迟、poison message 的 abort 风暴要有界重试、事务不能长（对策已冻结：先在有界内存冻结 manifest，再开事务）。
- **结论**：概念保留；是五项中**最值得分期**的一项（见 §6）。

### 2.2 NO_OUTPUT

- **消灭的故障**：无法回答"这条 Raw 是处理完但零产出、被丢了、还是根本没处理？"。没有显式结局，Raw 侧计数与 Canonical 侧计数永远对不上账，"平台没丢数据"不可证明。有了 `NO_OUTPUT`，`count(outcomes) == count(raw)` 成为可机器校验的不变量。
- **主流替代**：没有。DLQ 只接失败，"合法零产出"在所有平台里都是静默消失。对账思想存在于银行/数仓 ETL 的 control totals——本设计把它从批处理搬进了流式管道。
- **成本**：每条零产出 Raw 多一条小 record（心跳/纯 ACK 帧多的业务里数量不小但 payload 极小），外加 audit 消费者闭环。
- **结论**：便宜、诊断价值极高。客户第一次说"你们丢数据了"时，5 分钟自证和 5 天排查的差别。保留。

### 2.3 分级 quarantine（主架构 §7.2）

- **消灭的故障**：四个，两两相反——
  - 毒丸阻塞：一条坏消息让一个 partition 永远卡死（Kafka 消费者经典死法）；
  - 静默丢弃：skip-and-log，数据没了连痕迹都没有；
  - 暂态污染终态：catalog 抖动被写成永久 quarantine，告警噪声淹没真问题（对策：暂态 = pause partition + abort + backoff，**绝不写 quarantine，也绝不跳过该 Raw**）；
  - 历史改写：修主数据后篡改原 quarantine（对策：quarantine 不可变，纠错只能走新 `replayRunId` 显式重算）。
- **分级**：per-output（错误下仍能稳定确定 output 边界的单项语义错误，其余输出照常）/ raw-level（整体解析失败或聚合预算超限，一条有界记录）/ pre-resolution（设备不存在、gateway-child 拓扑冲突）。各级恢复路径不同。
- **主流替代**：单层 DLQ + 重试（Kafka Connect `errors.tolerance`、retry topic 模式）。无暂态/终态区分、无 per-output 粒度、可被清理改写。**主流 DLQ 是垃圾桶，本设计的 quarantine 是台账**。
- **成本**：quarantine topic + audit 投影 + 人工检查修复工作流 + replay controller；per-output 粒度要求 codec 证明"错误下仍能稳定确定 output 边界"，是对 codec 契约的真实复杂度税。
- **结论**：暂态/终态区分是承重墙，不能砍；**粒度分级可以分期**——V1 只做 raw-level，代价是一条批量历史补传中一个坏 report 会隔离整条 Raw 的 N 个 report，试点期可接受。

### 2.4 offset-vector release timeline（主架构 §7.1/§7.3）

- **消灭的故障**：**用错的 release 解释历史**。`currentReleaseId` 只是新数据指针——翻指针瞬间，backlog 会按新 codec 解码，尽管设备按旧格式发送。没有 barrier 历史：重放是拿今天的 codec 解三个月前的 Raw，garbage-in；在线处理在每次发布激活时也有竞态窗口。用 offset vector 而非裸 offset，是因为 Raw 从 V1 就是多 partition 日志，frontier 必须逐 partition 表达。
- **主流替代（一个真实的设计分叉）**：
  - Confluent/Schema Registry 流派：每条消息自带 schema id，按消息精确查版本，代价是每消息字节开销；
  - 本设计：Raw 不携带任何版本字段（§7.1 明确否决 per-message `schemaVersion`），版本信息全部移到控制面 barrier——省每消息开销，付运维复杂度；
  - 真正的同类是 **Debezium 的 schema history topic**：保存 schema 变更历史，让任意位置的 binlog 事件都能用当时生效的 schema 解释。本契约 ≈ schema history + 按 offset vector 索引。
- **不可消除的洞**：offset barrier 不能证明设备何时停止发送旧 wire payload（§7.3 原文承认）。对策是一条规则而不是更多机械：同一 key 内新 decoder 必须在冻结窗口内向后兼容，或 payload 自带可验证版本，做不到就发新 protocolKey 并迁移 Topic/ACL。**这是正确的严谨观：每个机制对应一个具体故障故事，故障故事的边界就是机制的边界。**
- **成本**：五项中最贵。cutover 编排（停拉取、排空 in-flight、提交逐 partition frontier、按 offset vector 发布）、制品长期保留（被 retention/replay 引用的全部 codec/model 字典与镜像）、partition 变更时的新 topic generation 迁移。
- **结论**：**记录**（不可变 barrier 历史）是重放能力的地基，必须冻结——砍了重放就退化成重新灌数；**编排自动化可以 V2**，V1 用维护窗口手工翻指针，barrier 记录照写。

### 2.5 ProcessingProvenance

- **消灭的故障**："TDengine 里这个 42.7 是哪来的？"大多数平台只能回答"设备发的"。有了 provenance：来自 Raw 的 topic/partition/offset（可回去读原始字节）、由 `semanticReleaseSetHash` 锁定的不可变 release 集合解释（兼容的镜像滚动不改变 hash，避免每次发版搅动 provenance）、可确定性复算。这是记录级监管链（chain of custody）。
- **主流替代**：OpenLineage/Apache Atlas 是**数据集级**血缘，不是**记录级** provenance。记录级的同类还是 Debezium（envelope 的 `source` 块 ≈ 本设计的 `rawRef`）和制药行业 ALCOA+ 数据完整性原则（attributable）。
- **成本**：每 report envelope 几十字节 + 填充纪律。
- **结论**：五项中性价比最高。保留，无争议。

## 3. 对比坐标系：这套设计的真实血统

| 本设计契约 | 主流世界的对应物 | 所在世界 |
| --- | --- | --- |
| outputs+offsets 事务 | Kafka Streams EOS | 流处理框架（已产品化） |
| `NO_OUTPUT` | ETL control totals 对账 | 银行/数仓批处理 |
| 分级 quarantine | DLQ（本设计是其超集） | 消息生态（半成品存在） |
| release timeline | Debezium schema history | 数据库 CDC |
| ProcessingProvenance | Debezium `source` 块 / ALCOA+ attributable | CDC / 制药合规 |

**这套设计不像一个物联网平台，像一个 event-sourced 的数据库 CDC 系统套上了 MQTT 接入层。**"没有 IoT 平台这么做"和"这么做成立"同时为真：IoT 平台不做，是因为消费级/轻工业市场把设备数据当**观测**——丢一个温度点无感、JSON 设备让 codec 版本治理蒸发、交付速度压过可审计性；CDC 和受监管行业做，是因为那里的数据是**账**——错一条、丢一条、解释错一条都有真金白银或合规后果。

## 4. 成本总账（诚实版）

- **写放大**：双峰值约 133 MB/s 入口 × Raw RF=3 × Canonical RF=3，再加各 consumer group 读取。数据面存储与网络开销约为单跳方案的 2 倍写 + 6 倍复制。这是 replay + 多消费者 decode-once 的价格，主架构 §3 已要求两条通道按 MB/s 验收。
- **运维复杂度**：最高项是事务契约与 cutover 编排，不是"两个 Topic"本身。
- **契约密度是复利税**：五项机制本身实现成本有限；真正贵的是它们让所有后续 feature 变贵——每加一种 output 都要回答结局语义、provenance 字段、release 交互、预算行为四个问题；新人要先学完契约才能碰热路径；测试矩阵必须包含 abort/redelivery/zombie 故障注入。主流平台不付这个税不是没想到，是其市场不为此付钱。
- **不可消除的不确定性**：设备侧 wire 版本不可证明（§2.4）。契约密度买到的是**平台侧的确定性**，设备侧永远是概率性的——这正是机制应该在规则处停下的地方。

**什么时候这套契约不该要**：客户数据是"观测"不是"账"——全是 JSON 直连标准设备、不需要重放纠错、单消费者（只写 TDengine）。此时应退回单跳融合直写（即主架构 §1 第 7 条否掉的候选）。§1 的国内四家平台正是该形态的产品化证据：durable 层只放在解码后、控制台脚本轻治理、无重放——它们的形态与其市场自洽，本设计与其差异不是对错而是目标客群不同。主架构押注的是"数据是账"的客户比例足够高（多厂商私有协议、能源计量、工业告警、合规审计）；这个赌注对不对是商业问题，不是技术问题。

## 5. 两条决策规则

1. **数真实的、需要全量点值流的独立消费者**：≥2 个且演进节奏不同 → Canonical 保留（decode-once 收益成立）；只有 Materializer 一个 → 可融合直写，砍掉 Canonical（Raw 仍在，随时可重建）。
2. **不可逆性排序**：Raw 是证据，不可再生；Canonical 是派生物，可再生（主架构 §4.2 本身就靠 Raw 重建 Canonical）。砍任何东西，先砍可再生的——砍 Canonical 后反悔有现成重建机制，砍 Raw 后反悔历史证据无法补录。

## 6. 分期建议：概念全冻结，自动化分期

原则：**概念层 retrofit 不可能**（数据流起来之后补不了 provenance 字段和 release 历史），作为冻结契约几乎免费；**自动化层可以延期**，它只影响运维成本不影响正确性地基。

| 机制 | 概念层（V1 冻结契约） | V1 试点实现 | Enterprise GA 补全 |
| --- | --- | --- | --- |
| outputs+offsets 事务 | 结局与 offset 原子性语义、幂等 `outputId` | at-least-once + 幂等 sink 先跑，窗口期重复由幂等消化 | `sendOffsetsToTransaction` + 下游 `read_committed` |
| `NO_OUTPUT` | 结局完备性（canonical / quarantine / NO_OUTPUT 三选一） | 全量实现 | 同左 |
| 分级 quarantine | 暂态≠终态、quarantine 不可变、纠错走新 `replayRunId` | 暂态 pause/backoff + raw-level quarantine | per-output 粒度 + 人工修复工作流 |
| release timeline | 不可变 `RawOffsetBarrier` 记录、回滚只追加不改写 | 维护窗口手工 cutover，barrier 记录照写 | 自动排空/发布编排、shadow/canary |
| ProcessingProvenance | envelope 字段与 `semanticReleaseSetHash` 语义 | 全量实现（字段便宜） | 同左 |

此表若被采纳，涉及主架构 §1 第 7 条与 §7.2"生产语义始终经 Kafka transaction"的冻结项，必须走正式变更评审，不允许以"试点简化"名义静默偏离。

## 7. 与主架构的对应关系

- 主架构 §4.2 回答"为什么同时保留 Raw 与 Canonical"——本文回答"这套契约买的是什么、什么时候付账"；
- 主架构 §7.1/§7.2/§7.3 是契约的正式定义，以主架构为准；本文的故障故事与成本分析是对其的非规范性注释；
- 主架构 §12（Phase 0 验证）中的"Raw→Canonical 生产契约"待办，可按本文 §6 把验证项拆成 V1 试点与 GA 两批。
