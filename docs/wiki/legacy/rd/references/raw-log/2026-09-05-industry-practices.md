# Raw Log Proposal：一手行业实践引用笔记

查询日期：2026-09-05（Asia/Shanghai）。

来源范围：四个产品的官方文档；Azure、EMQX 各用两个必要的关联页面交叉核验，共六个引用 URL。以下为查询时在线文档的语义，不代表所有历史版本；EMQX 来源为 Enterprise 的 `latest` 文档。仅研究数据边界与导出约束，不开展 Kafka 通用语义或容量成本分析。

## 1. ThingsBoard：Transport 与内部队列的数据边界

查询日期：2026-09-05。

URL：[Architecture Overview](https://thingsboard.io/docs/reference/architecture/)。定位：Data Flow、Services、Inter-service Communication。

概述／可核实产品语义：架构页描述设备载荷先进入协议专属 Transport，经鉴权并转换为统一消息，再通过队列交给规则引擎，由动作节点保存遥测、属性或告警。可确认队列位于该转换之后、服务之间；接入数据、内部消息与业务持久化存在不同边界，不能把内部队列的持久性直接当作原始报文归档能力。

不足以证明之处：架构页没有逐协议、逐版本列出统一消息是否附带原始字节，也没有穷举自定义 Transport、网关或集成路径。因此不能断言“所有协议都不保留原帧”，更不能从单条 JSON 遥测路径作此推断；也不能反向宣称内部队列天然保存完整线级报文或解析失败帧。

## 2. Azure：IoT Hub 内置端点、Blob 路由与 Event Hubs Capture

查询日期：2026-09-05。

URL：[Understand Azure IoT Hub endpoints](https://learn.microsoft.com/en-us/azure/iot-hub/iot-hub-devguide-endpoints)。定位：Built-in endpoint、Azure Storage as a routing endpoint。

交叉核验 URL：[Capture events through Azure Event Hubs](https://learn.microsoft.com/en-us/azure/event-hubs/event-hubs-capture-overview)。定位：How Event Hubs Capture works、Capture windowing。

概述／可核实产品语义：端点文档确认内置端点兼容 Event Hubs 的消费接口，且可另设存储路由，按大小或时间批量写 Blob，格式为 Avro 或 JSON。Capture 文档描述的是 Event Hubs 资源上的归档，存储故障后的补写受其保留期限制。两条归档路径应分别描述，不能混用能力与配置。

不足以证明之处：消费接口兼容性不足以证明 IoT Hub 内置端点可直接启用 Capture；本次采用页面未提供该支持承诺，不能把 Event Hubs 的 Capture 能力直接移植给它。消息写入 Blob 也不证明保留完整协议包头、未接纳报文或解析失败帧，不证明原生检索、解码回放和端到端零丢失。

## 3. AWS IoT Core：规则的 S3 与 Kinesis 动作

查询日期：2026-09-05。

URL：[AWS IoT rule actions](https://docs.aws.amazon.com/iot/latest/developerguide/iot-rule-actions.html)。定位：Rule action 表中的 S3、Kinesis Data Streams、Firehose，以及 Notes。

概述／可核实产品语义：规则动作表明确区分直接把消息存入 S3 桶、发送到 Kinesis Data Streams，以及发送到 Firehose 交付流。规则引擎遇到间歇错误可能多次尝试，全部尝试失败可丢弃消息并记录错误，也允许配置错误动作。这证明存在消息导出与失败处理机制，未承诺无限重试或永久保留。

不足以证明之处：Kinesis Data Streams 动作不等于已经归档至 S3，Firehose 也是独立动作，不能混称。动作清单没有证明所有入站协议原帧、被拒绝报文或规则处理前字节都会保存；也没有证明规则默认启用、恰好一次交付、查询回放界面或配置错误动作后必然不丢失。

## 4. EMQX Enterprise：Kafka 与对象存储集成的缓冲、重试约束

查询日期：2026-09-05。

URL：[Stream MQTT Data into Apache Kafka](https://docs.emqx.com/en/emqx/latest/data-integration/data-bridge-kafka.html)。定位：Advanced Configuration 中的 Buffer Mode、Per-partition Buffer Limit、Max Batch Age、Max Retries、Reconnect Delay。

交叉核验 URL：[Ingest MQTT Data into Amazon S3](https://docs.emqx.com/en/emqx/latest/data-integration/s3.html)。定位：Advanced Settings 中的 Request TTL、Max Buffer Queue Size、Query Mode。

概述／可核实产品语义：Kafka 磁盘缓冲可在节点重启后保留数据；内存及混合模式有重启丢失，缓冲满会丢旧消息。重试次数和批次年龄默认不限，但可配置丢弃上限；断线重发不计入错误响应重试次数。S3 请求有效期默认四十五秒，另有缓冲容量上限；异步模式允许订阅者先收到消息、对象稍后写入，须按具体集成核对。

不足以证明之处：Kafka 的磁盘缓冲描述不足以承诺每条消息已同步刷盘、整机断电不丢失或跨节点复制；默认不限重试仍受缓冲容量约束。S3 请求超时也不足以判断远端一定未写入。不能将 Kafka 的恢复、重试参数套给 S3，或把 S3 条件推广至全部对象存储连接器；订阅者收到消息不证明对象已持久化。

## 对 Proposal 的有限启示（研究推论）

上述来源支持把“指定消息导出到流系统或对象存储”作为有官方实践依据的能力方向；完整原始协议帧、解析失败覆盖、逐字节回放与故障下不丢失须各自提供证据。来源能证明产品机制及其边界，不能证明行业采用率、客户付费意愿或本项目的性价比，也不足以把消息归档直接宣传成全量原帧审计能力。
