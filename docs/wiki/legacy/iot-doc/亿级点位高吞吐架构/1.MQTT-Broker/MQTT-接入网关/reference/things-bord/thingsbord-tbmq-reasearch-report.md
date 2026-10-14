# TBMQ 源码分析与当前自研 MQTT Gateway 技术对比

> 日期：2026-06-09
> TBMQ 源码仓库：<https://github.com/thingsboard/tbmq>
> 本次分析源码快照：`bca8e9acb7019f0aa5005303d30ecb46e3be77ba`
> 当前自研 Broker 参考：`/home/wangjunbo/workspace-iot/mqtt-gateway` 与 [百万级设备 MQTT 接入层架构设计](./1.cloud-mqtt-gateway-architecture.md)。

---

## 1. 核心结论

TBMQ 是一个完整的、Kafka-backed、分布式通用 MQTT Broker。它不是 ThingsBoard Server 内置 MQTT Transport，而是 ThingsBoard 团队单独实现的 MQTT Broker 产品。

从源码看，TBMQ 的关键设计是：

```text
Netty MQTT Server
  -> per-client Actor
  -> PUBLISH 转 PublishMsgProto
  -> Kafka tbmq.msg.all
  -> Kafka ack 后再 PUBACK / PUBREC
  -> Consumer 读取 tbmq.msg.all
  -> Subscription Trie 匹配订阅者
  -> 直接下发 / 下行 Kafka 转发 / 持久会话队列 / APPLICATION 专属 Kafka topic
```

这说明 TBMQ 和我们当前自研 Broker 在 **QoS1 承诺点后置到 Kafka ack** 这一点上是同构的。TBMQ 源码中 `MqttPublishHandler` 只有在 `msgDispatcherService.persistPublishMsg(...)` 的 Kafka producer callback 成功后，才触发 `PubAckResponseMsg` / `PubRecResponseMsg`，最终写 MQTT `PUBACK` / `PUBREC`。

但两者的产品边界完全不同：

```text
TBMQ：
  通用 MQTT Broker
  完整 MQTT 协议能力
  通用订阅 / fan-in / fan-out / P2P
  持久会话 / retain / shared subscription / QoS2
  Kafka 是 broker 内部消息骨干

当前自研 MQTT Gateway：
  专用逆变器 telemetry ingress
  MQTT 3.1.1 子集
  上行 raw payload 原样进入 telemetry.raw Kafka
  Broker 不做通用订阅树、不做规则引擎、不做离线队列
  下行由 Command Service 状态机 + Presence 精准投递
```

因此：

```text
如果目标是替代“通用 MQTT Broker 集群”：
  TBMQ 技术成熟度、协议完整性、集群能力明显强于当前自研 Broker。

如果目标是“百万逆变器 raw telemetry durable ingress + 云端直接消费 raw Kafka”：
  当前自研 Broker 的边界更贴合，TBMQ 需要额外设计 raw log 契约和 Command Service 集成。

如果使用 TBMQ：
  它可以替代 MQTT 协议接入和通用订阅投递，
  但不能替代 Command Service 命令状态机、规则引擎厂商编码、业务 APP_ACK 一致性控制。
```

---

## 2. TBMQ 源码结构

TBMQ 是 Java / Spring / Netty / Kafka / Redis / PostgreSQL 体系。

核心源码目录：

| 路径 | 作用 |
|---|---|
| `application/src/main/java/.../server` | Netty TCP/TLS/WS/WSS MQTT listener |
| `application/src/main/java/.../actors` | 自研 Actor System，ClientActor / PersistedDeviceActor |
| `application/src/main/java/.../service/processing` | PUBLISH 持久化、订阅匹配、消息分发 |
| `application/src/main/java/.../service/mqtt` | MQTT 消息生成、投递、flow control、retain、persistence |
| `application/src/main/java/.../service/subscription` | 订阅缓存、Subscription Trie、shared subscription |
| `common/queue/src/main/java/.../queue/kafka` | Kafka producer / consumer / admin 封装 |
| `common/queue/src/main/proto/queue.proto` | TBMQ 内部 Kafka 消息协议 |
| `dao` | PostgreSQL 元数据存储 |
| `ui-ngx` | 管理控制台 |
| `integration` | 外部系统集成执行器 |

可以看出，TBMQ 的 Broker 热路径并不是一个薄薄的 MQTT listener，而是一套完整产品：

```text
协议接入
认证鉴权
客户端会话管理
订阅关系管理
消息持久化
跨节点投递
持久会话离线消息
retain
shared subscription
统计监控
UI 管理
REST API
外部集成
```

---

## 3. TBMQ 网络层：Netty + MQTT Codec

TBMQ 使用 Netty 做 MQTT TCP server。

关键源码：

- `application/src/main/java/org/thingsboard/mqtt/broker/server/AbstractMqttServerBootstrap.java`
- `application/src/main/java/org/thingsboard/mqtt/broker/server/MqttSessionHandler.java`
- `application/src/main/java/org/thingsboard/mqtt/broker/adaptor/NettyMqttConverter.java`

`AbstractMqttServerBootstrap` 创建 Netty `NioEventLoopGroup`：

```text
bossGroup = new NioEventLoopGroup(...)
workerGroup = new NioEventLoopGroup(...)
ServerBootstrap
  -> NioServerSocketChannel
  -> childHandler(...)
  -> SO_KEEPALIVE
  -> WRITE_BUFFER_WATER_MARK
```

这和当前自研 Go Broker 的 `net.Listener + 每连接 goroutine` 模型不同：

| 项 | TBMQ | 当前自研 Broker |
|---|---|---|
| 网络框架 | Netty NIO | Go net/tcp |
| 并发模型 | EventLoop + Actor | 每连接 read goroutine + bounded accept |
| MQTT codec | Netty MQTT codec | 自研 MQTT 3.1.1 子集 codec |
| 写背压 | Netty write buffer watermark + flow control | 文档要求 bounded queue，当前上行已实现，完整下行待实现 |

`MqttSessionHandler` 的职责是把 Netty MQTT 消息转换为 Actor 消息：

```text
CONNECT      -> MqttConnectMsg
SUBSCRIBE    -> MqttSubscribeMsg
UNSUBSCRIBE  -> MqttUnsubscribeMsg
PUBLISH      -> MqttPublishMsg
PUBACK       -> MqttPubAckMsg
PUBREC       -> MqttPubRecMsg
PUBREL       -> MqttPubRelMsg
PUBCOMP      -> MqttPubCompMsg
PINGREQ      -> MqttPingMsg
DISCONNECT   -> MqttDisconnectMsg
```

`NettyMqttConverter.createMqttPublishMsg(...)` 会 `retain()` MQTT payload ByteBuf，然后封装成 TBMQ 内部 `PublishMsg`。后面 `ProtoConverter.convertToPublishMsgProto(...)` 再复制为 protobuf `bytes payload` 写入 Kafka。

---

## 4. TBMQ Actor 层：每客户端一个 ClientActor

关键源码：

- `application/src/main/java/org/thingsboard/mqtt/broker/actors/client/ClientActor.java`
- `application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/MqttMessageHandlerImpl.java`
- `application/src/main/java/org/thingsboard/mqtt/broker/session/ClientSessionCtx.java`

官方架构文档也说明 TBMQ 使用自研 Actor System：

```text
Client actor：
  每个 MQTT client 一个 actor
  处理 CONNECT / SUBSCRIBE / UNSUBSCRIBE / PUBLISH / ACK / DISCONNECT

Persisted Device actor：
  管理持久 DEVICE client 的离线消息存储和重投
```

源码里 `ClientActor` 会按消息类型分派：

```text
MQTT_CONNECT_MSG
MQTT_PUBLISH_MSG
MQTT_SUBSCRIBE_MSG
MQTT_UNSUBSCRIBE_MSG
PUBACK_RESPONSE_MSG
PUBREC_RESPONSE_MSG
SESSION_DISCONNECTED_MSG
CONNECTION_REQUEST_MSG
...
```

这给 TBMQ 带来几个能力：

```text
1. 单 client 的 MQTT 状态可以在 actor 内串行处理。
2. CONNECT 冲突、session takeover、clean start、session expiry 可以统一处理。
3. QoS1/QoS2 的 ACK 顺序可以通过 OrderedProcessingQueue 管理。
4. 持久会话和离线消息可以挂到 client/session 生命周期上。
```

代价是：

```text
1. 每客户端状态比当前自研 Broker 重。
2. JVM 对象图复杂度高于当前设计的 pointer-poor session 目标。
3. 源码理解和二次开发门槛明显更高。
```

当前自研 Broker 刻意不做通用 ClientActor / SessionActor：

```text
长期 session 只保留连接、身份、presence、keepalive、少量 inflight 状态。
payload 不进入长期 session 对象。
上行 payload 只从 PUBLISH 存活到 Kafka durable ack。
```

这不是 TBMQ 做错了，而是目标不同。TBMQ 需要完整 MQTT Broker 能力，自研 Broker 第一阶段只服务 telemetry ingress。

---

## 5. TBMQ PUBLISH 与 QoS ACK 源码路径

### 5.1 PUBLISH 入口

关键源码：

- `MqttSessionHandler.processPublish(...)`
- `MqttPublishHandler.process(...)`
- `MsgDispatcherServiceImpl.persistPublishMsg(...)`
- `PublishMsgQueuePublisherImpl.sendMsg(...)`
- `TbKafkaProducerTemplate.send(...)`

路径：

```text
Netty MqttPublishMessage
  -> NettyMqttConverter.createMqttPublishMsg
  -> ClientMqttActorManager.processMqttMsg
  -> ClientActor
  -> MqttMessageHandlerImpl
  -> MqttPublishHandler
```

`MqttPublishHandler.process(...)` 先做：

```text
topic alias 解析
publish 权限/主题校验
QoS1/QoS2 响应队列登记
retain 处理
persistPubMsg(...)
```

### 5.2 Kafka ACK 后才 MQTT ACK

关键代码逻辑：

```text
MqttPublishHandler.persistPubMsg(...)
  -> msgDispatcherService.persistPublishMsg(...)
  -> Kafka producer send(...)
  -> onSuccess(...)
  -> actorRef.tell(new PubAckResponseMsg / PubRecResponseMsg)
  -> processPubAckResponse / processPubRecResponse
  -> channel.write(PUBACK / PUBREC)
```

也就是说，TBMQ 的 QoS1 路径是：

```text
收到 QoS1 PUBLISH
  -> 写入 tbmq.msg.all Kafka topic
  -> Kafka producer callback success
  -> 写 MQTT PUBACK
```

QoS2 路径里还有一段源码注释非常关键：它要确保消息已经存入 Kafka，才发送 `PUBREC`，避免重复处理。这和 MQTT QoS2 第一阶段 ACK 的正确语义一致。

### 5.3 失败处理

如果 Kafka 发送失败：

```text
MQTT 5:
  返回带 reason code 的 PUBACK/PUBREC 错误响应

MQTT 3.x:
  断开连接
```

这一点和当前自研 Broker 的红线一致：

```text
QoS1 写 raw log 失败：
  不伪装成功
  不返回成功 PUBACK
  断连或让设备超时重试
```

### 5.4 Kafka 持久级别取决于生产配置

TBMQ 源码层面是 Kafka producer callback 成功后 ACK MQTT。
但 Kafka callback 成功代表什么，取决于 Kafka producer / topic 配置。

`thingsboard-mqtt-broker.yml` 中默认项包括：

```text
queue.kafka.default.producer.acks = 1
tbmq.msg.all replication.factor = 1
```

这类默认值更像开发/单节点友好配置。如果要达到生产级“Broker ACK 后不丢”，至少要按生产要求设置：

```text
replication.factor >= 3
min.insync.replicas >= 2
producer acks = all
合理 retries / delivery.timeout.ms / request.timeout.ms
Kafka broker 磁盘与跨 AZ 策略
```

当前自研 Broker 的 Kafka appender 已在代码里固定使用：

```go
kgo.RequiredAcks(kgo.AllISRAcks())
```

也就是从源码默认行为上，自研 Broker 更强制地把 QoS1 承诺点绑定到 Kafka all ISR ack；TBMQ 则是机制支持，但需要部署配置配齐。

---

## 6. TBMQ 内部 Kafka 消息格式

TBMQ `queue.proto` 定义：

```proto
message PublishMsgProto {
  string clientId = 1;
  int32 packetId = 2;
  bytes payload = 3;
  bool retain = 4;
  int32 qos = 5;
  string topicName = 6;
  repeated UserPropertyProto userProperties = 7;
  optional MqttPropertiesProto mqttProperties = 8;
  optional string clientCertCn = 9;
}
```

这说明 TBMQ 的 `tbmq.msg.all` 并不是只保存业务解码后的数据，它确实保存 MQTT payload bytes、topic、qos、retain、properties、clientId 等 broker 消息信息。

但它仍然不同于我们当前设计的 raw log envelope：

| 项 | TBMQ `PublishMsgProto` | 当前自研 `RawMessage` |
|---|---|---|
| 格式归属 | TBMQ 内部 protobuf | 公司自定义 Kafka 契约 |
| payload | MQTT payload bytes | MQTT payload bytes |
| 身份 | clientId / cert CN / MQTT 信息 | tenant_id / device_id 来自认证主体 |
| topic | MQTT topicName | MQTT topic + broker 元数据 |
| 用途 | Broker 内部路由、订阅匹配、持久会话 | 云端 raw log、Decoder、重放、审计 |
| 稳定性 | 受 TBMQ 内部 schema 影响 | 公司可控 |

所以如果云端业务直接消费 TBMQ 的 `tbmq.msg.all`：

```text
优点：
  可以拿到所有 MQTT PUBLISH 的 payload/topic/qos/clientId。

风险：
  云端消费者会耦合 TBMQ 内部 protobuf schema。
  topic、分区、保留策略、compact/retention 策略要按 TBMQ 升级兼容性评估。
  需要另做 tenant/device/product 映射、raw envelope、schema_id、ingest_ts、broker_id 等字段。
```

更稳的方式是：

```text
TBMQ 内部 topic
  -> Raw Log Exporter / Kafka Integration / 自研 Consumer
  -> 转成公司 telemetry.raw envelope
  -> 云端 Decoder / Rule Engine / Command Service 消费
```

但这样会带来一个语义差异：

```text
设备收到 TBMQ PUBACK：
  表示已进入 TBMQ 的 tbmq.msg.all。

不天然表示：
  已经进入公司 telemetry.raw topic。
```

如果公司强要求“设备 PUBACK = 公司 raw log 已落盘”，则不能只靠异步 exporter。要么改 TBMQ PUBLISH ACK 前链路，要么保留自研 Broker。

---

## 7. TBMQ 消息分发链路

官方文档与源码一致：

```text
1. 所有 PUBLISH 进入 tbmq.msg.all。
2. Kafka ack 后 Broker 才对 QoS1/2 回复。
3. tbmq.msg.all consumer 读取消息。
4. Subscription Trie 匹配订阅者。
5. 根据订阅者类型和持久策略分流。
```

`MsgDispatcherServiceImpl.processPublishMsg(...)` 做的事情：

```text
根据 topicName 查询 Subscription Trie
应用 shared subscription
应用 noLocal / qos downgrade / retain option
对非持久订阅者直接走 downlink
对持久订阅者交给 MsgPersistenceManager
```

分流逻辑：

| 订阅者类型 | TBMQ 处理 |
|---|---|
| 非持久 DEVICE | 直接投递；跨节点通过 `tbmq.msg.downlink.basic.$SERVICE_ID` |
| 持久 DEVICE | 进入 `tbmq.msg.persisted`，再写 Redis，重连后补发 |
| 持久 APPLICATION | 写入 `tbmq.msg.app.$CLIENT_ID` 专属 Kafka topic |
| APPLICATION shared subscription | 写入 `tbmq.msg.app.shared.$TOPIC_FILTER` |
| Integration | 写入 integration 相关 Kafka topic |

这正是通用 MQTT Broker 要解决的问题：

```text
发布者不知道订阅者在哪个节点。
订阅者可能在线、离线、持久、非持久、共享订阅。
Broker 需要在集群内完成路由、持久化和补发。
```

当前自研 Broker 刻意不做这件事。上行分发交给 Kafka consumer group：

```text
设备 PUBLISH
  -> MQTT Gateway
  -> telemetry.raw Kafka
  -> Decoder / Rule Engine / Command Service / Storage consumer group
```

下行也不依赖 MQTT 通用订阅树：

```text
Command Service
  -> 查 Presence Registry
  -> 找到 device 当前 broker_id
  -> downlink.{broker_id}
  -> Broker 本地 MQTT PUBLISH
```

这个差异是两套设计的根本分界。

---

## 8. TBMQ 持久会话、Retain、Shared Subscription

TBMQ 支持完整 MQTT Broker 能力：

```text
QoS0 / QoS1 / QoS2
MQTT 3.1 / 3.1.1 / 5.0
clean session / persistent session
session expiry
retain message
shared subscription
last will
topic alias
receive maximum / flow control
TLS / WS / WSS
多种认证方式
管理 UI
```

源码和配置可以看到：

```text
tbmq.msg.retained
tbmq.client.session
tbmq.client.subscriptions
tbmq.msg.persisted
tbmq.msg.downlink.basic.$SERVICE_ID
tbmq.msg.downlink.persisted.$SERVICE_ID
```

持久 DEVICE client 的离线消息使用：

```text
tbmq.msg.persisted Kafka
  -> dedicated consumer
  -> Redis 存储 DEVICE 离线消息
  -> client reconnect 后补发
```

持久 APPLICATION client 使用：

```text
tbmq.msg.app.$CLIENT_ID Kafka topic
  -> 每个 APPLICATION client 独立消费
```

这对通用 MQTT broker 很强，但对我们的逆变器场景未必是第一优先级：

```text
自产逆变器 telemetry:
  不需要 retain
  不需要 QoS2
  不需要通用 wildcard subscription
  不希望 Broker 内堆长期离线命令

云端应用消费：
  更适合 Kafka consumer group，而不是 MQTT subscription
```

---

## 9. 当前自研 MQTT Gateway 源码画像

当前自研 Broker 是 Go 实现，已经具备以下主链路：

```text
启动：
  加载配置
  连接 DB / Kafka / Redis，失败 fail fast
  启动 AuthQueue
  启动 Ingest Queue
  启动 TCP listener

CONNECT：
  解析 MQTT 3.1.1 CONNECT
  认证
  Redis presence INCR session_version
  Redis Lua CAS 写 presence
  写成功才 CONNACK accepted

PUBLISH：
  解析 MQTT PUBLISH QoS0/QoS1
  ACL 校验 topic tenant/device_id
  构造 RawMessage
  QoS0 TryAppendAsync，满则丢弃
  QoS1 SubmitAndWait，等 Kafka durable ack
  Kafka 成功才 PUBACK

DISCONNECT / 断连：
  Redis Lua fencing delete presence
```

关键源码：

- `mqtt-gateway/internal/server/server.go`
- `mqtt-gateway/internal/ingest/queue.go`
- `mqtt-gateway/internal/ingest/kafka/appender.go`
- `mqtt-gateway/internal/presence/store.go`

当前已实现的特点：

| 能力 | 当前状态 |
|---|---|
| MQTT 3.1.1 CONNECT | 已实现 |
| MQTT PUBLISH QoS0/QoS1 | 已实现 |
| QoS1 Kafka ack 后 PUBACK | 已实现 |
| Kafka acks=all | 已在 appender 中固定 |
| Presence Redis CAS | 已实现 |
| ACL topic 校验 | 已实现，当前是自产设备 topic 模型 |
| bounded accept | 已实现 |
| bounded ingest queue | 已实现 |
| TLS | 配置结构支持，完整生产证书策略需继续完善 |
| SUBSCRIBE / 下行命令 | 文档已设计，代码当前待实现 |
| 第三方产品接入配置 | 文档已补充，代码当前待实现 |
| Command Service 状态机 | 文档已设计，不在 broker repo 内 |

这说明当前自研 Broker 还不是完整 MQTT Broker，它是按“专用接入层”方向在推进。

---

## 10. 技术对比表

| 维度 | TBMQ | 当前自研 MQTT Gateway |
|---|---|---|
| 定位 | 通用分布式 MQTT Broker | 专用逆变器 raw telemetry 接入层 |
| 技术栈 | Java / Spring / Netty / Actor / Kafka / Redis / PostgreSQL | Go / net.TCP / Kafka / Redis / PostgreSQL |
| MQTT 版本 | MQTT 3.1 / 3.1.1 / 5.0 | 第一阶段 MQTT 3.1.1 子集 |
| QoS | QoS0 / QoS1 / QoS2 | QoS0 / QoS1 |
| QoS1 ACK 时机 | Kafka producer callback 成功后 ACK | Kafka ProduceSync acks=all 成功后 ACK |
| Kafka topic | `tbmq.msg.all` + 多个内部 topic | `telemetry.raw.critical` / `telemetry.raw.normal` |
| Kafka 消息格式 | `PublishMsgProto` | 公司自定义 `RawMessage` envelope |
| Producer 持久级别 | 源码支持配置，默认 yml 为 `acks=1` | 源码固定 `acks=all` |
| 上行处理 | 进入 TBMQ 内部 broker 消息流 | 原样进入 raw log |
| 订阅分发 | Subscription Trie + downlink queue + persistent queue | 不做通用订阅，云端用 Kafka consumer group |
| Shared subscription | 支持 | 不支持，明确非目标 |
| Retain | 支持 | 不支持，明确非目标 |
| 持久会话 | 支持 | 不支持，明确非目标 |
| 离线消息 | DEVICE Redis / APPLICATION Kafka topic | Command Service 管命令状态，不堆 Broker |
| 下行投递 | 按 MQTT subscription 路由，跨节点 Kafka downlink | Command Service 查 Presence 精准投递到 broker |
| 第三方设备 | 通用 MQTT topic/subscription/ACL 能力强 | 需产品接入配置 + 规则引擎编码/解码 |
| 业务命令状态机 | 不内置公司业务命令状态 | Command Service DB 状态机负责 |
| 规则引擎编码 | 非 TBMQ Broker 职责 | 外部规则引擎负责厂商 topic/payload |
| 运维能力 | UI、REST、监控、凭证管理完整 | 专用服务，需自研观测和管理 |
| 复杂度 | 高，完整 broker 产品 | 低，接入层职责收敛 |
| 二开成本 | 要理解 TBMQ 架构和内部 topic/schema | 公司完全控制，但需要自研更多能力 |

---

## 11. 与我们当前业务流程的适配分析

### 11.1 上行 telemetry

我们的目标：

```text
设备 MQTT PUBLISH QoS1
  -> raw payload 写入 telemetry.raw Kafka
  -> Kafka durable ack
  -> MQTT PUBACK
  -> 云端消费者订阅 Kafka raw log
```

TBMQ 原生目标：

```text
设备 MQTT PUBLISH QoS1
  -> PublishMsgProto 写入 tbmq.msg.all
  -> Kafka producer callback success
  -> MQTT PUBACK
  -> TBMQ consumer 做订阅匹配和投递
```

如果直接采用 TBMQ，必须回答一个问题：

```text
云端消费者订阅什么？
```

可选方案：

| 方案 | 描述 | 问题 |
|---|---|---|
| 直接消费 `tbmq.msg.all` | 云端消费者解析 TBMQ `PublishMsgProto` | 耦合 TBMQ 内部 schema 和 topic 生命周期 |
| TBMQ -> Exporter -> `telemetry.raw` | 异步转换为公司 raw envelope | 设备 PUBACK 不代表 `telemetry.raw` 已落盘 |
| 改造 TBMQ ACK 前写公司 raw log | 保留 PUBACK 语义 | 侵入 TBMQ 热路径，升级维护成本高 |
| 保留自研 Broker | raw log 是第一承诺点 | 继续承担 Broker 自研成本 |

如果“设备 PUBACK = 公司 telemetry.raw 已持久化”是硬红线，TBMQ 不改造时不能完全替代当前自研 Broker。

### 11.2 云端消费者

当前设计中，云端消费者是 Kafka consumer：

```text
Decoder
Rule Engine
Command Service ACK adapter
Storage
Audit
Replay tooling
```

TBMQ 的自然消费者是 MQTT subscriber 或 TBMQ 内部 processing consumer。
如果把云端全部改成 MQTT subscriber，可以利用 TBMQ 通用订阅能力；但这会丢掉 Kafka consumer group、offset、replay、lag、raw log 多消费组这些优势。

更现实的方式是：

```text
TBMQ 只作为 MQTT 接入层
  -> 从 tbmq.msg.all 或 Integration 导出到 telemetry.raw
  -> 云端仍按 Kafka raw log 模型消费
```

但这相当于在 TBMQ 后面再造一层 raw log pipeline，系统复杂度不一定比自研 Broker 更低。

### 11.3 下行命令

我们当前南向命令流程：

```text
Command Service 先写 DB 状态机
  -> outbox / Kafka command.encode.request
  -> 规则引擎编码厂商 MQTT topic / payload / qos / ack_policy
  -> Command Service / Dispatch Worker 消费 command.encoded
  -> 查 Presence
  -> 投递到 downlink.{broker_id}
  -> Broker 本地 MQTT PUBLISH
  -> 设备业务 ACK 上行
  -> Kafka raw log
  -> Adapter / 规则引擎解码
  -> Command Service 更新 APP_ACK_RECEIVED
```

TBMQ 可以承担的是：

```text
将已编码好的 MQTT PUBLISH 投递给订阅了对应 topic 的设备。
```

TBMQ 不会替代：

```text
Command Service DB 状态机
规则引擎厂商编码
业务 APP_ACK 关联 command_id
同步等待 / 异步等待 ACK
命令重试、取消、超时、一致性控制
```

因此使用 TBMQ 后，下行流程会变成类似：

```text
Command Service DB 状态机
  -> Kafka command.encode.request
  -> 规则引擎编码
  -> 发布 MQTT PUBLISH 到 TBMQ
  -> TBMQ 按订阅关系投递到设备
  -> 设备 ACK 上行进入 TBMQ
  -> ACK exporter / consumer 转到 Command Service
```

这里的关键风险是：业务命令状态机会跨越 TBMQ 内部 MQTT 投递语义和公司 Kafka ACK 语义，需要明确 `SENT_TO_BROKER`、`DELIVERED_MQTT`、`APP_ACK_RECEIVED` 的边界。

---

## 12. TBMQ 的优势

### 12.1 协议完整

TBMQ 已经支持完整 MQTT Broker 应有能力：

```text
MQTT 3.1 / 3.1.1 / 5.0
QoS0 / QoS1 / QoS2
retain
persistent session
shared subscription
topic alias
last will
TLS / WS / WSS
```

自研 Broker 如果要补齐这些能力，工程量会很大，也容易踩 MQTT 规范边界。

### 12.2 Kafka-backed 设计成熟

TBMQ 把 Kafka 作为 broker 消息骨干：

```text
tbmq.msg.all
tbmq.msg.persisted
tbmq.msg.app.$CLIENT_ID
tbmq.client.session
tbmq.client.subscriptions
tbmq.msg.downlink.basic.$SERVICE_ID
tbmq.msg.downlink.persisted.$SERVICE_ID
```

这套设计覆盖了通用 MQTT broker 的核心问题：

```text
跨节点消息路由
消费者重平衡
持久客户端离线消息
APPLICATION 客户端高吞吐订阅
共享订阅
节点故障恢复
```

### 12.3 管理和运维能力完整

TBMQ 有：

```text
Web UI
REST API
客户端凭证管理
订阅管理
session 管理
blocked client
unauthorized client
监控面板
integration
```

这些不是当前自研 Broker 第一阶段目标。若公司需要对外提供 MQTT broker 平台能力，TBMQ 的产品完成度明显更高。

### 12.4 第三方 MQTT 设备接入更自然

对于客户购买的第三方厂家设备，TBMQ 更像标准通用 broker：

```text
不要求所有设备遵循自产 topic 模型。
可以用 ACL / credentials 管理 MQTT topic 权限。
可以让设备按厂商 topic 订阅和发布。
```

但要注意：第三方设备的业务 payload 编码/解码仍然需要规则引擎或 Adapter，不会因为换成 TBMQ 自动消失。

---

## 13. TBMQ 的风险与不匹配点

### 13.1 它不是公司 raw log 契约

TBMQ 的 Kafka topic 是 broker 内部消息系统。
即使 `tbmq.msg.all` 保存了 payload，也不能直接等价为公司 `telemetry.raw`。

如果云端消费者直接依赖 `tbmq.msg.all`：

```text
1. 绑定 TBMQ protobuf。
2. 绑定 TBMQ 内部 topic 命名和保留策略。
3. 升级 TBMQ 时要评估内部 schema 兼容性。
4. 公司 raw envelope 字段需要另行补齐。
```

### 13.2 默认配置不等于生产强持久

源码配置默认值里 `acks=1`、`replication.factor=1` 比较常见。
这不代表 TBMQ 不能强持久，而是说明生产必须显式配置 Kafka。

如果用于承载我们的 QoS1 raw telemetry，需要把以下项纳入上线红线：

```text
Kafka topic replication.factor >= 3
min.insync.replicas >= 2
producer acks = all
unclean.leader.election.enable = false
合理 retention.bytes / retention.ms
压测 producer buffer、batch、linger、request timeout
```

### 13.3 Broker 状态比自研方案重

TBMQ 支持通用 MQTT 能力，因此每 client/session 状态自然更多：

```text
ClientSessionCtx
PubResponseProcessingCtx
AwaitingPubRelPacketsCtx
PublishedInFlightCtx
TopicAliasCtx
auth rules
client type
session info
actor state
queued messages
subscription cache
```

这对通用 broker 是合理的。
但对百万逆变器只做 5 分钟一次上行 raw telemetry 的场景，自研 Broker 的极简 session 模型更容易控制内存和 GC。

### 13.4 下行命令状态机仍需自研

TBMQ 解决的是 MQTT 投递。
我们的业务命令还有：

```text
DB 状态机
command_id 幂等
规则引擎编码
厂商 payload/topic
设备业务 ACK 解码
同步等待 / 异步等待
超时 / 重试 / 取消
审计
```

这些不能交给 TBMQ 自动完成。

### 13.5 当前自研 Broker 已有强业务边界

当前自研方案已经围绕业务建立了几个强边界：

```text
Broker 不解码
Broker 不写时序库
Broker 不做通用订阅
Broker 不堆离线消息
QoS1 PUBACK = telemetry.raw durable ack
Command Service 负责命令真相
规则引擎负责厂商编码
```

引入 TBMQ 后，这些边界需要重新定义，尤其是：

```text
raw log 承诺点
云端 Kafka 消费契约
命令状态机投递状态
第三方设备 topic/payload 适配位置
```

---

## 14. 是否用 TBMQ 替代当前自研 Broker

### 14.1 可以考虑 TBMQ 的条件

适合选择 TBMQ，如果我们决定：

```text
1. 公司要一个标准通用 MQTT Broker。
2. 第三方设备、第三方应用会大量使用 MQTT subscribe/publish。
3. 需要 retain、persistent session、shared subscription、QoS2、MQTT5。
4. 可以接受 TBMQ 内部 Kafka topic 作为 broker 内部事实源。
5. 公司愿意为 raw log 另做 exporter 或接受 tbmq.msg.all schema。
6. 运维团队能维护 Java/Spring/Netty/Kafka/Redis/PostgreSQL 体系。
```

这种选择下，建议架构变成：

```text
设备 / 第三方 MQTT client
  -> TBMQ
  -> TBMQ 内部 Kafka
  -> Raw Log Exporter
  -> telemetry.raw
  -> Decoder / Rule Engine / Command Service
```

下行：

```text
Command Service
  -> DB 状态机
  -> 规则引擎编码 MQTT topic/payload
  -> Publish 到 TBMQ
  -> TBMQ 投递到设备
```

### 14.2 不建议替换的条件

不建议直接替换，如果我们坚持：

```text
1. 设备 PUBACK 必须等于公司 telemetry.raw Kafka durable ack。
2. 云端业务必须直接订阅公司 raw Kafka，不接受 TBMQ 内部 schema。
3. Broker 第一阶段不需要通用 MQTT subscription。
4. 不需要 retain、QoS2、持久会话、shared subscription。
5. 下行低频，且由 Presence 精准路由即可。
6. 主要目标是百万逆变器连接密度、低 GC、低状态。
```

这种情况下，当前自研 Broker 仍然更贴合。

### 14.3 折中方案

可以把 TBMQ 放到“应用 MQTT Gateway / 第三方 MQTT Gateway”位置，而不是替换自产逆变器主入口：

```text
自产逆变器：
  -> 自研 MQTT Gateway
  -> telemetry.raw

第三方标准 MQTT 设备 / 应用：
  -> TBMQ
  -> Raw Log Exporter / Integration
  -> telemetry.raw 或 vendor.raw
```

这样可以避免一个系统同时背负两个方向的极端要求：

```text
自研 Broker：
  极简、高密度、raw log 承诺点强。

TBMQ：
  标准 MQTT、第三方兼容、通用订阅、管理能力强。
```

---

## 15. PoC 验证建议

如果要认真评估 TBMQ，建议不要只看源码和文档，必须做同口径压测。

### 15.1 上行可靠性压测

```text
设备数：10万 / 30万 / 100万
payload：3KB / 5KB / 16KB
QoS：QoS1
上报周期：5分钟
峰值：均匀 / 1分钟挤压 / 10秒挤压
Kafka：acks=all, replication.factor=3, min.insync.replicas=2
```

指标：

```text
PUBACK 延迟 p50/p95/p99
Kafka produce latency
tbmq.msg.all lag
JVM heap / GC pause
Netty event loop pending tasks
actor mailbox backlog
Kafka broker disk/network
消息重复率
消息丢失率
```

### 15.2 Raw log 契约验证

验证：

```text
tbmq.msg.all 是否能稳定解析出 payload/topic/clientId/qos/properties
是否能补齐 tenant_id/device_id/product_key/schema_id/ingest_ts/broker_id
TBMQ 升级时 proto 兼容性如何保证
Raw Log Exporter 落 telemetry.raw 的延迟
设备 PUBACK 与 telemetry.raw 落盘之间是否存在语义缺口
```

### 15.3 下行命令验证

验证：

```text
Command Service 状态机如何标记 SENT_TO_TBMQ / DELIVERED_MQTT / APP_ACK_RECEIVED
规则引擎编码后的 topic/payload 如何 publish 到 TBMQ
设备业务 ACK 如何从 TBMQ 上行转换回 command_id
同步等待 ACK 的延迟和超时语义
异步 ACK 的补偿和去重
```

### 15.4 第三方设备验证

验证：

```text
厂商 topic 是否可直接接入
ACL 是否能表达厂商 publish/subscribe 范围
payload 二进制/JSON/私有协议是否能无损进入 exporter
下行命令 topic/payload 是否能由规则引擎生成
设备 ACK topic/payload 是否能被 Adapter 识别
```

---

## 16. 最终建议

TBMQ 是一个值得认真评估的通用 MQTT Broker。它在协议完整性、集群消息投递、持久会话、shared subscription、管理 UI 和 Kafka-backed 可靠投递上，明显强于当前自研 Broker。

但它不是当前自研 Broker 的简单替换件。当前自研 Broker 的核心价值不是“也能收 MQTT”，而是：

```text
QoS1 PUBACK = 公司 telemetry.raw durable ack
Broker 极简
raw payload 契约公司可控
云端 Kafka consumer group 直接消费
Command Service 状态机清晰
规则引擎负责厂商编码，Broker 透明
```

因此推荐决策：

```text
短期：
  继续推进自研 MQTT Gateway 作为自产逆变器主入口。

并行：
  做 TBMQ PoC，重点验证第三方标准 MQTT 设备、通用订阅、管理 UI 和 MQTT5/QoS2/retain 等能力。

中期：
  如果第三方设备和 MQTT 应用生态诉求变强，可以让 TBMQ 承担“标准 MQTT Gateway”角色。

谨慎：
  不建议在未验证 raw log ACK 语义、Kafka 配置、内部 topic schema、Command Service 集成之前，直接用 TBMQ 替换自研 Broker。
```

---

## 资料来源

### 官方文档

- TBMQ Architecture: <https://thingsboard.io/docs/mqtt-broker/architecture/>
- Why TBMQ: <https://thingsboard.io/docs/mqtt-broker/why-tbmq/>
- TBMQ GitHub Repository: <https://github.com/thingsboard/tbmq>

### TBMQ 源码证据

- 源码快照：<https://github.com/thingsboard/tbmq/tree/bca8e9acb7019f0aa5005303d30ecb46e3be77ba>
- `MqttPublishHandler`: <https://github.com/thingsboard/tbmq/blob/bca8e9acb7019f0aa5005303d30ecb46e3be77ba/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/handlers/MqttPublishHandler.java>
- `MsgDispatcherServiceImpl`: <https://github.com/thingsboard/tbmq/blob/bca8e9acb7019f0aa5005303d30ecb46e3be77ba/application/src/main/java/org/thingsboard/mqtt/broker/service/processing/MsgDispatcherServiceImpl.java>
- `TbKafkaProducerTemplate`: <https://github.com/thingsboard/tbmq/blob/bca8e9acb7019f0aa5005303d30ecb46e3be77ba/common/queue/src/main/java/org/thingsboard/mqtt/broker/queue/kafka/TbKafkaProducerTemplate.java>
- `queue.proto`: <https://github.com/thingsboard/tbmq/blob/bca8e9acb7019f0aa5005303d30ecb46e3be77ba/common/queue/src/main/proto/queue.proto>
- `thingsboard-mqtt-broker.yml`: <https://github.com/thingsboard/tbmq/blob/bca8e9acb7019f0aa5005303d30ecb46e3be77ba/application/src/main/resources/thingsboard-mqtt-broker.yml>

### 当前自研 Broker 参考

- [当前架构设计](./1.cloud-mqtt-gateway-architecture.md)
- [Raw Log Appender 设计](./3.cloud-mqtt-gateway-raw-log-design.md)
- [实现红线](./7.cloud-mqtt-gateway-implementation-red-lines.md)
- `mqtt-gateway/internal/server/server.go`
- `mqtt-gateway/internal/ingest/queue.go`
- `mqtt-gateway/internal/ingest/kafka/appender.go`
- `mqtt-gateway/internal/presence/store.go`
