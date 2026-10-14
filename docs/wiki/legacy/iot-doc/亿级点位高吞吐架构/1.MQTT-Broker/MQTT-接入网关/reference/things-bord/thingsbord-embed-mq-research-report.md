# ThingsBoard 内置 MQTT Broker 与当前 MQTT Gateway 设计对比

> 日期：2026-06-08  
> 对比对象：ThingsBoard Server 内置 MQTT Transport / MQTT Device API，而不是 ThingsBoard 团队独立推出的 TBMQ。  
> 当前设计参考：[百万级设备 MQTT 接入层架构设计](./1.cloud-mqtt-gateway-architecture.md)。

---

## 1. 核心结论

ThingsBoard Server 的确可以作为标准 MQTT 接入端使用。官方 MQTT API 文档明确说明，ThingsBoard 可接受 MQTT v3.1、v3.1.1 和 v5.0 客户端连接，默认端口是 `1883`，TLS 端口是 `8883`，并支持 QoS0 / QoS1。它还内置设备认证、设备模型、规则引擎、遥测/属性入库、服务端 RPC、持久 RPC、Dashboard 和 WebSocket 推送。因此，如果目标是“快速搭建一个完整 IoT 平台”，ThingsBoard 内置 MQTT 是很强的现成方案。

但它和我们当前设计解决的问题不完全一样。

我们的当前设计不是要做一个通用 IoT 平台，而是要做：

```text
百万级逆变器 telemetry ingress
  + durable raw log
  + 规则引擎下行编码
  + Command Service 状态机
  + 低频南向定向投递
```

关键差异在于：ThingsBoard 内置 MQTT Transport 会把设备上行 payload 转成 ThingsBoard 统一消息并推入 Rule Engine；我们当前设计则把 Broker 定位为极简接入层，QoS1 上行以 Kafka / Redpanda raw log ack 作为承诺点，Broker 不解码 payload，不写时序库，不执行规则引擎。

结论可以归纳为：

```text
想快速获得设备管理、规则链、仪表盘、RPC、用户体系：
  ThingsBoard 内置 MQTT 更省工程量。

想把百万设备原始上报稳定写入 Kafka raw log，
并保持 Broker 极简、可控、低 GC / 低状态：
  当前自研 MQTT Gateway 更贴合。

如果真正想使用 ThingsBoard 团队的高性能通用 MQTT Broker：
  应评估 TBMQ，而不是 ThingsBoard Server 内置 MQTT Transport。
```

---

## 2. 需要先区分两个 ThingsBoard MQTT 产品

### 2.1 ThingsBoard Server 内置 MQTT Transport

这是 ThingsBoard IoT 平台自带的设备接入层。官方文档把它描述成标准 MQTT broker，设备可以通过 MQTT 连接到 ThingsBoard，并使用固定或可配置的设备 API topic 上报遥测、属性、RPC 等。

典型链路：

```text
Device MQTT client
  -> ThingsBoard MQTT Transport
  -> Core credential validation / cache
  -> Message Queue
  -> Rule Engine
  -> Timeseries / Attributes / Alarm / Dashboard
```

官方架构文档说明：设备通过 MQTT / HTTP / CoAP / LwM2M 连接到 Transport layer；Transport 解析凭证、识别租户、把协议 payload 转成统一消息，并通过 message queue 推给 Rule Engine。

### 2.2 TBMQ

TBMQ 是 ThingsBoard 团队单独推出的 MQTT Broker 产品，不等同于 ThingsBoard Server 内置 MQTT Transport。TBMQ 官方架构文档强调它面向 MQTT broker 本身的 fan-in、fan-out、P2P 场景，并以 Kafka 作为持久化与分发骨干；QoS1/2 的 PUBACK/PUBREC 在 Kafka 持久化确认后才返回。

这和我们当前设计的“QoS1 以上行 raw log durable ack 作为承诺点”更接近。

因此选型时不能混为一谈：

| 对象 | 定位 |
|---|---|
| ThingsBoard Server 内置 MQTT Transport | IoT 平台设备接入协议层 |
| TBMQ | 通用高性能 MQTT Broker |
| 当前自研 MQTT Gateway | 公司场景专用 MQTT 接入层 + raw log appender |

---

## 3. ThingsBoard 内置 MQTT 的能力画像

### 3.1 MQTT 接入能力

ThingsBoard MQTT API 支持：

```text
MQTT v3.1 / v3.1.1 / v5.0
1883 plain MQTT
8883 MQTT over TLS
QoS0 / QoS1
Access Token
MQTT Basic
X.509 Certificate
```

它支持两套 topic 格式：

```text
短格式：
  v2/t
  v2/a
  v2/r/req/+
  v2/r/res/$id

标准格式：
  v1/devices/me/telemetry
  v1/devices/me/attributes
  v1/devices/me/rpc/request/+
  v1/devices/me/rpc/response/$id
```

设备 profile 里还可以配置 MQTT transport 参数，包括自定义 telemetry / attributes topic filter、JSON / Protobuf payload 格式、Sparkplug B 等。这里对第三方设备有帮助，但它仍然是围绕 ThingsBoard 的设备 API 模型来处理消息。

### 3.2 内部消息队列

ThingsBoard 服务间通信依赖 message queue，生产集群建议使用 Kafka。官方文档说明：

```text
TB_QUEUE_TYPE=in-memory  // 开发 / 单节点演示，无持久化
TB_QUEUE_TYPE=kafka      // 生产 / 集群，磁盘持久化
```

ThingsBoard 内部 queue 会按 entity ID 分区，让同一设备的消息落到同一分区，便于 actor system 保序处理。

### 3.3 上行处理模型

ThingsBoard 上行不是“Broker 收到 MQTT PUBLISH 后原样写 raw log，云端业务直接订阅 Kafka raw”。它的官方路径更像：

```text
Device PUBLISH
  -> MQTT Transport
  -> 认证 / 租户解析
  -> 协议 payload 转成 ThingsBoard 统一消息
  -> Message Queue
  -> Rule Engine
  -> 遥测 / 属性 / 告警 / 外部动作
```

这对通用 IoT 平台很舒服，但对我们当前的 raw binary 设计有两个差异：

```text
1. Broker 不再只是 raw log appender，而是进入 ThingsBoard 平台语义。
2. raw binary 重放、重新解码、点表版本修正，需要单独设计存储路径。
```

### 3.4 下行 RPC

ThingsBoard 提供 server-side RPC：

```text
REST API / Dashboard / Rule Engine
  -> Core / Device actor
  -> Transport
  -> MQTT PUBLISH 到设备
```

RPC 分一向和双向：

| 类型 | 含义 |
|---|---|
| One-way RPC | 只要求发到设备，不要求设备回复 |
| Two-way RPC | 设备必须在超时时间内回复 |

RPC 又分 lightweight 和 persistent：

| 类型 | 特征 |
|---|---|
| Lightweight RPC | 短生命周期、内存态；设备离线时命令丢失 |
| Persistent RPC | 存 DB，带 TTL；设备离线时等待重连 |

Persistent RPC 状态包括：

```text
QUEUED
SENT
DELIVERED
SUCCESSFUL
TIMEOUT
EXPIRED
FAILED
```

这和我们当前 Command Service 状态机在理念上接近：都把“命令是否创建、是否发送、是否设备 ACK、是否业务成功、是否超时”显式状态化。

---

## 4. 当前自研设计画像

当前设计的主路径：

```text
设备
  -> MQTT Gateway
  -> Kafka / Redpanda raw log
  -> Decoder / Rule Engine / Storage
```

Broker 职责刻意收敛：

```text
TCP / TLS / MQTT codec
CONNECT / PUBLISH / PUBACK / PING / DISCONNECT
认证
Presence 写入
基础 ACL
QoS0 / QoS1
上行 append raw log
下行本地 MQTT PUBLISH
背压
```

Broker 不做：

```text
复杂规则引擎
Modbus 解码
厂商 payload 编码
时序库写入
长期离线命令队列
通用订阅树
通用 shared subscription
```

下行链路：

```text
Business API
  -> Command Service 写 DB 状态机
  -> outbox / Kafka: command.encode.request
  -> 规则引擎编码为厂商 mqtt_topic / mqtt_payload / qos / ack_policy
  -> Command Service / Dispatch Worker 消费 command.encoded
  -> 查 Presence
  -> downlink.{broker_id}
  -> Broker 本地 MQTT PUBLISH
  -> 设备 cmd_ack 上行
  -> Kafka raw log
  -> 上行 Adapter / 规则引擎解码
  -> Command Service 更新 APP_ACK_RECEIVED
```

这个设计的核心是：

```text
DB 是命令状态权威。
Kafka 是 raw log / 编码事件流 / 审计。
规则引擎负责统一命令与厂商 MQTT 消息之间的编码/解码。
Broker 保持透明、轻量、可压测。
```

---

## 5. 逐项对比

| 维度 | ThingsBoard 内置 MQTT | 当前自研 MQTT Gateway |
|---|---|---|
| 产品定位 | IoT 平台设备接入层 | 专用 MQTT 接入层 |
| 是否开箱即用 | 是，设备、规则链、仪表盘、RPC 都有 | 否，需要实现 Broker / Command Service / Decoder |
| MQTT 标准兼容 | 支持标准 MQTT 客户端接入 | 第一阶段 MQTT 3.1.1 子集 |
| Topic 模型 | ThingsBoard 设备 API topic，可自定义部分 topic filter | 按产品接入配置 + raw log envelope |
| 第三方设备 | 可通过 custom topic filter、Protobuf、Sparkplug B、Gateway / Integration 适配 | 通过产品接入配置 + 规则引擎编码/解码 |
| 上行语义 | 转成 ThingsBoard 统一消息进入 Rule Engine | 原始 payload 进入 Kafka raw log |
| QoS1 承诺点 | 文档强调 MQTT QoS0/1，但没有把“raw log durable ack”作为产品边界 | 明确要求 Kafka / Redpanda durable ack 后才 PUBACK |
| Raw binary 保留 | 需要另行设计或通过规则链保存 | 主链路天然保留 raw binary |
| 云端消费者 | Rule Engine / ThingsBoard API / DB / WebSocket | Kafka consumer group / Decoder / Command Service |
| 下行命令 | ThingsBoard RPC / Persistent RPC | Command Service DB 状态机 + 规则引擎编码 + 私有通道投递 |
| 离线命令 | Persistent RPC 存 DB + TTL | Command Service 存 DB + TTL，不堆 Broker |
| 规则引擎位置 | ThingsBoard 内置 Rule Engine 是核心路径 | 外置/平台规则引擎在 Broker 外，Broker 不执行规则 |
| 百万连接目标 | 需要按 ThingsBoard Server/Transport 真实压测确认 | 设计目标即百万连接接入 |
| Broker 状态复杂度 | 与 ThingsBoard Core、Actor、Rule Engine、DB 深度耦合 | Broker 极简，状态主要是 session + presence |
| 运维复杂度 | 平台组件多但成熟一体化 | 组件少但需要自研和专项压测 |
| 二次开发边界 | 需要理解 ThingsBoard 扩展模型、规则链、设备 profile | 公司完全控制协议和状态机 |

---

## 6. ThingsBoard 内置 MQTT 的优势

### 6.1 平台能力完整

ThingsBoard 的最大优势不是“它有 MQTT 端口”，而是 MQTT 后面已经接好了：

```text
设备模型
租户 / 用户 / 权限
设备凭证
遥测 / 属性
规则引擎
告警
Dashboard
RPC
持久 RPC
WebSocket 实时推送
```

如果我们希望快速交付一个完整 IoT 平台，ThingsBoard 会极大减少工程量。

### 6.2 认证和设备接入形态成熟

ThingsBoard 支持 Access Token、MQTT Basic、X.509，并且官方架构说明会缓存设备凭证，避免每条消息都查 DB。这一点对高吞吐 MQTT 接入很重要。

### 6.3 RPC 状态机已有

Persistent RPC 已经有 DB 存储、TTL、重试、状态机、Rule Engine 事件。我们当前 Command Service 设计中的很多状态机思想，在 ThingsBoard 里已经以 RPC 的方式存在。

### 6.4 第三方设备适配入口更多

ThingsBoard 有 device profile 的 MQTT transport 配置、custom topic filters、JSON / Protobuf、Sparkplug B、IoT Gateway、MQTT Integration、Rule Engine 等多种入口。对“客户买了第三方设备，协议各不相同”的场景，这些平台能力比从零写适配层更省事。

---

## 7. ThingsBoard 内置 MQTT 的风险与不匹配点

### 7.1 它不是纯 raw log 接入层

我们的设计要求：

```text
设备 PUBLISH QoS1
  -> append Kafka / Redpanda raw log
  -> durable ack
  -> Broker PUBACK
```

ThingsBoard 内置 MQTT 的官方路径是：

```text
Transport
  -> unified message
  -> message queue
  -> Rule Engine
  -> action nodes persist
```

这并不等于“所有上行 payload 原样进入 Kafka raw log，并以 raw log durable ack 作为 MQTT PUBACK 承诺点”。如果我们要保留 raw binary 7/30 天、支持重新解码、点表版本修正和故障回放，需要在 ThingsBoard 里额外设计 raw 存储链路。

### 7.2 Broker 热路径不可控

ThingsBoard 的热路径包含 Transport、Core credential validation、Actor System、Rule Engine queue、Rule Engine nodes、DB action 等平台组件。它是完整平台，不是 pointer-poor、bounded-memory、只 append raw log 的极简 Broker。

这不一定是坏事，但和我们当前“百万连接接入层必须极简”的红线冲突。

### 7.3 百万逆变器场景需要专项压测

ThingsBoard 官方文档说明其架构支持集群和 Kafka 消息队列，但这不等于 ThingsBoard Server 内置 MQTT Transport 在我们的消息体、连接风暴、日出日落模型下可以直接承载百万连接。

这里要区分：

```text
TBMQ 官方明确按 MQTT Broker 大规模场景设计。
ThingsBoard Server 内置 MQTT Transport 是 IoT 平台接入层。
```

如果要使用 ThingsBoard 内置 MQTT 替代当前自研 Broker，必须压测：

```text
百万 MQTT 连接
日出 CONNECT 风暴
日落 DISCONNECT 风暴
5 分钟周期上报风暴
3KB / 5KB / 16KB payload
Kafka queue 抖动
Rule Engine 降速
DB 写入抖动
RPC 下行 + APP ACK
第三方 topic / payload 适配
```

### 7.4 第三方厂商协议不是只靠 MQTT broker 解决

第三方设备的难点通常不是 MQTT 连接本身，而是：

```text
topic 差异
payload 差异
命令 topic 生成
命令 payload 编码
ACK / reply topic 识别
状态码语义
设备身份映射
```

ThingsBoard 可以通过 Rule Engine、Gateway、Integration 或 custom topic filters 做适配，但这仍然是适配工程。它不会自动知道每个厂商的 command payload 应该怎么编码。

### 7.5 云端 Kafka 消费模型不同

我们当前设计要求云平台订阅 Kafka raw log。ThingsBoard 更自然的消费模型是：

```text
Rule Engine
Timeseries DB
REST API
WebSocket
Integrations
```

如果云端系统已经围绕 Kafka consumer group、raw log replay、Decoder、Command Service 状态机建立，直接切换 ThingsBoard 会改变系统边界。

---

## 8. 与当前“DB 状态机 + Kafka 编码 + 规则引擎”流程的关系

我们刚补充的当前流程是：

```text
Command Service 先写 DB 状态机
Kafka 负责异步编码、事件通知、审计和解耦
规则引擎负责将平台统一消息编码成厂商 MQTT topic / payload
Broker 原样投递已编码 MQTT 消息
```

ThingsBoard 的思想与它有相似处：

```text
设备上行：
  Transport -> unified message -> Rule Engine

服务端 RPC：
  REST / Rule Engine -> Device actor -> Transport -> MQTT PUBLISH

Persistent RPC：
  DB 状态机 + TTL + retry
```

不同点是：ThingsBoard 把这些能力做在一个 IoT 平台内部；我们当前设计把它拆成：

```text
MQTT Gateway
Command Service
Rule Engine / Adapter
Kafka raw log
Decoder
Storage
```

拆开的好处是每层职责清晰，Broker 热路径极简；代价是工程量更大。

---

## 9. 选型建议

### 9.1 如果选择 ThingsBoard 内置 MQTT

适合条件：

```text
1. 更看重完整 IoT 平台，而不是极简 raw log 接入层。
2. 设备数量和上报峰值经过压测确认 ThingsBoard 可承载。
3. 可以接受 ThingsBoard 的设备 API / Rule Engine / RPC 模型。
4. 第三方厂家适配愿意放到 ThingsBoard profile / Rule Chain / Gateway / Integration。
5. 云端业务不强依赖直接订阅 raw Kafka。
```

建议验证：

```text
1. QoS1 PUBLISH 在 ThingsBoard 内部 queue / Rule Engine / DB 抖动下的 PUBACK 时机。
2. Kafka queue backend 与 in-memory backend 在故障下的差异。
3. Rule Engine 降速时 MQTT Transport 是否背压、丢弃、断连或积压。
4. Persistent RPC 在大规模离线设备上的 DB 压力。
5. 第三方厂家 topic / payload / ACK 是否能通过 profile + Rule Engine 完整表达。
```

### 9.2 如果选择当前自研设计

适合条件：

```text
1. 首要目标是百万逆变器上行 raw telemetry ingress。
2. 必须保留 raw binary，支持重放、重新解码、点表修正。
3. QoS1 承诺点必须是 Kafka / Redpanda durable raw log ack。
4. Broker 必须极简、可控、低长期状态。
5. 下行命令需要公司自定义 Command Service 状态机。
6. 第三方厂家协议适配由规则引擎 / Adapter 管理，Broker 不参与。
```

主要代价：

```text
自研 Broker 和压测成本
Command Service / Presence / 编码链路开发成本
Dashboard、设备管理、规则链等平台能力需要复用现有 things 或另建
```

### 9.3 如果只想要高性能通用 MQTT Broker

应单独评估 TBMQ、EMQX、VerneMQ、HiveMQ 等，而不是 ThingsBoard Server 内置 MQTT Transport。

TBMQ 与我们设计最接近的点是：

```text
Kafka 作为 broker 消息持久化骨干
QoS1/2 ack 等 Kafka durable ack
fan-in / fan-out / P2P 都作为一等场景
```

但 TBMQ 是通用 MQTT Broker；我们的设计是专用逆变器接入层。即使使用 TBMQ，Command Service、规则引擎编码、raw log 解码、设备业务 ACK 状态机仍然需要设计。

---

## 10. 推荐结论

不建议把 ThingsBoard Server 内置 MQTT 直接等价为我们当前自研 MQTT Gateway 的替代品。

更准确的结论是：

```text
ThingsBoard 内置 MQTT = IoT 平台设备接入能力。
当前自研 MQTT Gateway = 百万设备 raw telemetry 专用接入能力。
TBMQ = ThingsBoard 团队的通用高性能 MQTT Broker 产品。
```

如果公司目标是快速搭建 IoT 平台、减少业务后台研发，ThingsBoard 值得认真评估。

如果公司目标是支撑百万逆变器稳定接入，并严格控制上行 QoS1 durable raw log 语义、Kafka replay、Command Service 状态机和规则引擎厂商编码链路，那么当前设计更贴合。

最稳妥的路径不是二选一，而是做一个短 PoC：

```text
PoC A：ThingsBoard 内置 MQTT
  10万连接
  真实 payload
  Kafka queue backend
  Rule Engine 编码/解码
  Persistent RPC
  第三方 topic

PoC B：自研 MQTT Gateway
  10万连接
  QoS1 raw log ack
  Kafka ack 抖动
  Command Service 编码链路
  Presence 定向下行
```

两个 PoC 用同一套设备模拟器、同一套 payload、同一套 command/ack 流程比较：

```text
连接密度
上行 ACK 延迟
Kafka lag
Rule Engine lag
内存 / GC / CPU
下行命令成功率
离线命令恢复
第三方协议适配成本
运维复杂度
```

压测结果出来后再决定是否采用 ThingsBoard、TBMQ，还是继续当前自研接入层。

---

## 资料来源

- ThingsBoard MQTT API: <https://thingsboard.io/docs/reference/mqtt-api/getting-connected/>
- ThingsBoard Architecture Overview: <https://thingsboard.io/docs/reference/architecture/>
- ThingsBoard Message Queue: <https://thingsboard.io/docs/reference/architecture/queue/>
- ThingsBoard Command & Control / RPC: <https://thingsboard.io/docs/user-guide/command-and-control/>
- ThingsBoard TBMQ Architecture: <https://thingsboard.io/docs/mqtt-broker/architecture/>
- Why TBMQ: <https://thingsboard.io/docs/mqtt-broker/why-tbmq/>
