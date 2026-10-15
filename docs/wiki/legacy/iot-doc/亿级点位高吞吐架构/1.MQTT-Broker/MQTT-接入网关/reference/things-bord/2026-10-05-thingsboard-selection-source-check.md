# 自研 MQTT Gateway 与 ThingsBoard / TBMQ 双路线选型核验

核验日期：2026-10-05。范围：百万 MQTT 连接、每设备每 300 秒约 3 KB 原始 Modbus 报文、约 700 点、QoS 0/1、低频命令、已有自研平台。当前自研接入层只有设计和部分实现，未完成可用于容量判断的原型与压测；本文也未运行任何性能测试。

用户补充约束：研发只有本人一人，云服务器压测预算 5 万元。该预算不等于研发和长期运维成本预算，也不等于本次已获准支出。

用户明确的选择标准：即使 TBMQ 更费服务器，只要节省研发维护、综合更划算，也可以选择 TBMQ。两条路线均需 research，不以已有设计或单机资源效率预定结果。

阶段边界补充：用户当前要完成的是借助大模型开发的 **0.1 压测版**，不是立刻承担完整生产 broker 的全部维护。本笔记的推荐据此收敛到实验决策。

本笔记只新增研究结论，没有修改既有设计、部署组件或注册 LoopX。既有 ThingsBoard/TBMQ 研究笔记仅用于找到源码入口，下面的产品事实重新用官方文档及固定源码核验。

## 1. 决策结论

**自研 MQTT Gateway 0.1 与 TBMQ CE + 业务适配是两条平等候选，均需研究和验证；当前证据不足以选出胜者。** 一人加大模型可以开展范围收敛的 0.1 验证，长期生产维护成本不应成为否定实验的理由。也不能因已有自研设计而把 TBMQ 降为陪跑。两条路线先证明入口持久化、鉴权、背压和业务契约，再扩大连接数量，由结果决定采用哪种内核。现阶段没有仅为 MQTT 接入迁移整套 ThingsBoard 平台的充分理由。

这不是已经判定 TBMQ 必胜：其 Kafka 集成默认行为与本项目的 raw bytes、deviceId 分区、ACK 后不丢契约有差异，需要薄适配和生产配置。也不是已经判定必须自研：本项目尚未做原型、压测，没有证据证明成熟 broker 的资源成本或接入代价超过自研。

原来的“继续自研 / 全面采用 ThingsBoard / 参考 ThingsBoard 完善自研”应拆成两个不同决策：业务接入架构可以保留；MQTT 协议引擎可以自研，也可以采用 **TBMQ CE + 自研身份/遥测桥接/命令服务**。借鉴 ThingsBoard/TBMQ 并不预先决定最终要自己维护协议内核。这是基于下述证据的工程判断。

## 2. 两个不同的产品

| 对象 | 已核实定位 | 本项目的含义 |
|---|---|---|
| ThingsBoard CE 平台内 MQTT Transport | MQTT 3.1/3.1.1/5 接入，QoS 0/1；认证、设备配置、遥测/属性/RPC 与平台服务相连 | 适合采纳平台设备模型、规则链与数据服务；不能因能够单独部署 transport，就视为独立通用 broker |
| TBMQ CE | 独立 MQTT broker，独立代码库与部署，Apache-2.0；支持持久会话及通用订阅等 broker 能力 | 可以接现有平台，不要求替换现有设备、时序、规则、命令业务 |

平台协议来源：[MQTT Getting Connected](https://thingsboard.io/docs/reference/mqtt-api/getting-connected/)。平台服务拆分来源：[Microservices Architecture](https://thingsboard.io/docs/reference/architecture/microservices/)。TBMQ 来源：[Architecture](https://tbmq.io/docs/architecture/)、[LICENSE](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/LICENSE)。这里只陈述核实到的许可证，不推断未来商业策略。

当前官方 TBMQ release notes 的最新条目为 **v2.4.0，2026-08-27**；GitHub tag 指向 **`2f60cfef8e058f4cd663b46bc4553ff23dfe89cc`**。以下 TBMQ 源码结论固定在此提交，避免滚动文档与版本混用。[Release notes](https://tbmq.io/docs/releases/)、[Tag API](https://api.github.com/repos/thingsboard/tbmq/git/ref/tags/v2.4.0)

本地 ThingsBoard 是 **`4.4.0-SNAPSHOT`，提交 `562b19aa90f92c97b96c255b14816c32da7f4958`**，是源码快照，不冒充已发布稳定版本。`git describe` 的输出来自仓库可见标签，不能据此当成 v3.4 源码。[本地 pom.xml](/home/wangjunbo/workspace-iot-platform/thingsboard/pom.xml:12)

## 3. 为什么不建议仅为接入换成整个 ThingsBoard

本地 transport 热路径不是“报文原样写到任意 Kafka topic”：`processDevicePublish` 依据设备配置选择遥测/属性/RPC，调用 payload adaptor；`DefaultTransportService` 把遥测转成平台数据及 `TbMsg`，带 tenant/device/profile/rule-chain 信息进入规则引擎队列。

设备遥测 topic 可以配置，因此不应笼统称为“只支持固定 topic”。真正的约束在于它围绕平台 API、设备配置和数据模型工作。把这一层拆出来服务现有平台，需要处理平台认证与设备配置依赖、队列协议、平台内部实体、原始数据解析时点、命令/RPC 语义。

以上可直接检查：[MqttTransportHandler.java](/home/wangjunbo/workspace-iot-platform/thingsboard/common/transport/mqtt/src/main/java/org/thingsboard/server/transport/mqtt/MqttTransportHandler.java:544)、[DefaultTransportService.java](/home/wangjunbo/workspace-iot-platform/thingsboard/common/transport/transport-api/src/main/java/org/thingsboard/server/common/transport/service/DefaultTransportService.java:530)、[设备 MQTT 配置](/home/wangjunbo/workspace-iot-platform/thingsboard/common/data/src/main/java/org/thingsboard/server/common/data/device/profile/MqttDeviceProfileTransportConfiguration.java:21)。

该版本遥测 PUBACK 的成功回调关联 transport 向内部队列投递；不表示规则链全部执行成功或最终时序库已落库。[PUBACK 回调](/home/wangjunbo/workspace-iot-platform/thingsboard/common/transport/mqtt/src/main/java/org/thingsboard/server/transport/mqtt/MqttTransportHandler.java:710)、[规则引擎投递](/home/wangjunbo/workspace-iot-platform/thingsboard/common/transport/transport-api/src/main/java/org/thingsboard/server/common/transport/service/DefaultTransportService.java:1115)

**工程推断：**现有平台已有设备、解析、存储、规则和命令边界时，只为一个 MQTT listener 引入另一套平台，会增加模型和运维耦合。只有同时需要 ThingsBoard 的完整平台能力，并接受其设备/规则/数据模型时，“直接用 ThingsBoard”才值得上升为首选。

## 4. TBMQ 的 Kafka / Redis 分工

TBMQ 的 Kafka 不只是风暴缓冲区，还承担接入消息持久化、集群会话/订阅状态传播及跨节点消息流。新节点恢复本地状态；订阅匹配使用本地 Subscription Trie。Redis/Valkey 用于 DEVICE 持久订阅者的待投递消息及缓存，不能把它概括为“每条消息都到 Redis 查 topic 与连接”。[Architecture](https://tbmq.io/docs/architecture/)

TBMQ 的两类持久客户端要区分：

- DEVICE：面向设备的低速订阅/命令收件箱，待投递消息进入 Redis，带数量与过期限制。
- APPLICATION：面向后端高吞吐订阅者，使用独立 Kafka topic 和消费位点；必须配置持久会话，客户端类型由凭据/认证结果确定。

DEVICE 持久化指 **broker 向设备交付的收件箱**，不是说设备每一条上行遥测只有写 Redis 才能 PUBACK。[Persistent DEVICE client](https://tbmq.io/docs/architecture-details/persistent-device-client/)、[Persistent APPLICATION client](https://tbmq.io/docs/architecture-details/persistent-app-client/)

**工程推断：**本项目主要 fan-in、很少下行，并不需要把百万设备都变成 APPLICATION 客户端，更不需要给每台设备创建一个业务 Kafka topic。后端订阅/集成的并行度、Kafka partitions、设备数量是三个不同的参数。

## 5. 最关键的差别：PUBACK 的承诺边界

TBMQ v2.4.0 的接入路径为：

```text
MQTT PUBLISH
  -> PublishMsgProto（包含原始 payload）
  -> 内部 Kafka tbmq.msg.all 写入
       ├─ producer callback 成功 -> Actor -> QoS 1 PUBACK
       └─ 独立 consumer 读消息 -> 订阅匹配 / 持久收件箱 / 集成转发
```

源码能确认的是 **PUBACK 等待 Kafka producer 成功，不等待订阅匹配和投递完成**。ACK 回调与消息消费是独立异步分支，不能保证设备先收到 PUBACK，消费端才开始匹配；也不能保证 Kafka consumer 要等 producer callback 在接入节点执行后才能读到消息。[MqttPublishHandler.java:224](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/handlers/MqttPublishHandler.java#L224)、[MsgDispatcherServiceImpl.java:88](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/processing/MsgDispatcherServiceImpl.java#L88)。官方也明确 PUBACK 不等于订阅者已收到。[Delivery guarantees](https://tbmq.io/docs/concepts/qos/)

因此，TBMQ 可以满足“设备 ACK 后，消息已进入一个可恢复入口日志”的设计方向；但**不能把它自动等同于“现有业务 `telemetry.raw` 已经写成功”**。两种契约可以选其一：

1. 接受 TBMQ 内部 Kafka 为入口承诺点，给内部日志到业务 raw 增加可恢复、可重放、可监测的可靠交接。
2. 坚持设备 PUBACK 必须等待指定业务 raw 写成功，则标准 TBMQ 集成的异步承诺点不同，需要改 ACK 热路径或继续专用网关。

两种都不是逻辑上不可行；需要明确业务到底要求哪一种，而不是把“有 Kafka”当成证明。

### 5.1 生产可靠性不能沿用默认配置

固定 v2.4.0 配置显示：

| 配置 | 默认值 | 对本项目的影响 |
|---|---|---|
| `TB_KAFKA_DEFAULT_PRODUCER_ACKS` | `1` | 配置文件的全局默认只等待 leader，运行时覆盖需要检查 |
| `TB_KAFKA_MSG_ALL_TOPIC_PROPERTIES` 中 `replication.factor` | `1` | 默认入口日志不是多副本容灾配置 |
| `TB_MSG_ALL_ACK_STRATEGY_TYPE` | `SKIP_ALL` | broker 内部消费失败策略需要核查与调整 |
| `TB_APP_PERSISTED_MSG_ACK_STRATEGY_TYPE` / `RETRIES` | `RETRY_ALL` / `3` | APPLICATION 路径也不是无限重试的默认值 |
| `TB_IE_MSG_ACK_STRATEGY_TYPE` / `RETRIES` | `SKIP_ALL` / `5` | 集成默认会跳过失败，5 次只在切换 RETRY_ALL 后有意义 |

来源：[Broker 配置](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/resources/thingsboard-mqtt-broker.yml#L252)、[Producer 与 topic 配置](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/resources/thingsboard-mqtt-broker.yml#L413)、[Integration Executor 配置](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/integration/executor/src/main/resources/tbmq-integration-executor.yml#L29)。

这里核实的是固定版本配置文件默认值，不是任何现网部署的 effective configuration。`msg-all.additional-producer-config` 默认为空，但存在全局、topic 专用和部署环境覆盖入口；本次未启动实例、未完整追踪配置合并对象，因此应把实际 producer acks、topic RF/min ISR 和已有 topic 是否更新列入 PoC 检查。

**建议的验收配置范围：**核查所有日志 topic 的 RF、min ISR 与 producer acks；核查每一跳的失败/超时/重试/位点提交；按恢复窗口配置 retention 时间与容量，不能只看到“7 天”而漏掉同时存在的 `retention.bytes`。配置的正确性仍需故障注入证明。

### 5.2 官方 Kafka Integration 不是原样 raw 日志适配器

官方 Kafka Integration 需要 Integration Executor，经内部集成 topic 再写外部 Kafka。v2.4.0 `KafkaIntegration.java` 表明：

- Kafka record value 使用 String；勾选只发 payload 后，合法 UTF-8 转文本，否则转 Base64。
- Kafka key 来自 `config.getKey()`，这段实现并没有自动从当前设备提取 deviceId。
- 外部 producer callback 成功后才调用该集成消息的 success callback；设备 PUBACK 仅依赖内部入口写入成功，不等待外部集成回调，不能据此断言两个回调的实际先后顺序。

来源：[Kafka Integration 文档](https://tbmq.io/docs/integrations/kafka/)、[KafkaIntegration.java:150](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/integration/executor/src/main/java/org/thingsboard/mqtt/broker/integration/service/integration/kafka/KafkaIntegration.java#L150)。

所以本项目若要求 `raw payload bytes`、`key=deviceId`、既有 envelope/schema，应安排解码/重组/动态 key 的薄适配，不能以“send only payload”的名字推断字节不变。若把所有设备配置同一个静态 key，还会把所有消息压到同一个业务 Kafka partition。

官方集成默认 `SKIP_ALL` 明确不保证外部交付；固定源码中 `RETRY_ALL` 且 `retries=0` 会继续重试失败/超时消息，不因达到次数而提交该批位点。成功落外部 Kafka 但尚未提交内部位点时崩溃，会产生重放重复，需要下游幂等。[Integrations](https://tbmq.io/docs/integrations/)、[IntegrationAckStrategyFactory.java:80](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/integration/executor/src/main/java/org/thingsboard/mqtt/broker/integration/service/processing/backpressure/IntegrationAckStrategyFactory.java#L80)、[IntegrationMsgProcessorImpl.java:339](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/integration/executor/src/main/java/org/thingsboard/mqtt/broker/integration/service/processing/IntegrationMsgProcessorImpl.java#L339)

注意滚动文档一处称两种策略最终都会向前推进，与同页 `retries=0` 的说明及固定源码不完全一致；验收以固定版本实测为准。此外源码在集成 consumer **不存在已提交位点**时从末尾初始化，不能假设删除 consumer group 后会自动补回历史；需要把首次启用、位点丢失、灾后重建写入运维契约。[IntegrationMsgProcessorImpl.java:187](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/integration/executor/src/main/java/org/thingsboard/mqtt/broker/integration/service/processing/IntegrationMsgProcessorImpl.java#L187)

可比较的适配方案：配置并扩展 IE；用持久 APPLICATION MQTT consumer，业务 Kafka 成功后再确认消费；或以独立 consumer group 读取内部 protobuf 日志并转业务 raw。最后一种与内部 topic/schema 耦合，需要固定版本与升级兼容测试。以上均为待验证设计选项，本次没有实现或验证任一方案。

## 6. 官方大规模测试证明什么

TBMQ 官方 100M 测试：100,000,000 发布者、500 持久 APPLICATION 订阅者，114 bytes 消息，QoS 1；25 台 64 vCPU / 256 GiB broker 实例、9 台 Kafka。标称 6M MQTT PUBLISH/s 是 **3M 上行 + 3M 下行**；测试持续 1 小时，建连约 22k/s，PUBACK P95 55 ms。[官方性能报告](https://tbmq.io/docs/reference/100m-connections-performance-test/)

这是厂商公开测试的证据，不是本项目实测，也不是百万设备的最低配置推荐。不能按标题误读为单机一亿连接、上行 6M/s，或已经验证本项目 3 KB、TLS、设备认证、离线命令、连接风暴和节点故障。该页未给出足以在本笔记确认的完整固定源码提交与本项目等价安全配置；不能将上述数字用来直接承诺节点数和月成本。

基于用户工作负载的独立算术：

- 正常上行：`1,000,000 / 300 ≈ 3,333 报文/秒`。
- 若 3 KB 按 3,000 bytes 计：约 `10 MB/s` 原始 payload；不含协议、TLS、副本及解析膨胀。
- 700 点/包意味着约 `233 万点/秒` 的解析后处理量；这属于下游数据处理指标，不等于 MQTT 每秒报文数。

因此先验证百万连接的常驻状态、TLS/认证/重连成本、ACK 和下游故障语义，比单纯追逐 broker 百万 msg/s 更贴合本项目。

## 7. 自研路线的证据、优势假设与待验证项

自研方案已经把 MQTT 接入与业务处理分开：每 300 秒一次逆变器二进制上报进入 raw Kafka，解码在后端执行；下行由命令状态机管理，接入节点定向发送。这种需求收敛使有限自研有研究价值，但并未证明性能胜出。[现有总体设计](/home/wangjunbo/workspace-iot-platform/iot-doc/亿级点位高吞吐架构/1.MQTT-Broker/MQTT-接入网关/1.cloud-mqtt-gateway-architecture.md:79)

| 项目 | 已有设计证据 | 0.1 要取得的证据 |
|---|---|---|
| 协议范围 | MQTT 3.1.1 子集、QoS 0/1、CleanSession=true、受限命令订阅 | 真实固件与独立客户端互通，错误报文、重复 clientId、断线重连处理正确 |
| 入口承诺 | 业务 raw Kafka 成功后才回复 QoS1 PUBACK | 故障前后按消息 ID 核对已确认消息，保存原始字节并处理允许的重复 |
| 内存与背压 | 有界队列、按 bytes 预算、限制 inflight | Kafka 变慢、黑洞、连接风暴下 RSS 有上限，恢复后 FD/goroutine/内存回落 |
| 下行 | Presence + fencing + Command Service + gRPC | 旧连接晚退出不删除新路由，命令不误投，MQTT ACK 与设备执行结果分离 |
| 资源经济性 | 减少通用订阅与会话功能，路径可能更短 | 相同 TLS、可靠性、故障余量下的实测机器、网络、磁盘与成本增量 |

来源：[实现红线](/home/wangjunbo/workspace-iot-platform/iot-doc/亿级点位高吞吐架构/1.MQTT-Broker/MQTT-接入网关/7.cloud-mqtt-gateway-implementation-red-lines.md:33)、[Presence](/home/wangjunbo/workspace-iot-platform/iot-doc/亿级点位高吞吐架构/1.MQTT-Broker/MQTT-接入网关/2.cloud-mqtt-presence-registry-design.md:118)、[待完成事项](/home/wangjunbo/workspace-iot-platform/iot-doc/亿级点位高吞吐架构/1.MQTT-Broker/MQTT-接入网关/TODOS.md:43)。TODO 是文档记录，不能当作本次已复现的代码缺陷。

以下现有估算不宜直接用于选型：

- 容量稿把 broker lease 续约算成“设备连接数 / TTL”，而正式设计是每个 broker 续租。基础负载应按“broker 数 / 实际续租间隔”估算，另计设备建断连、长 TTL 刷新、下行查询等。由错误公式推出的 Redis 连接上限不成立。[错误公式](/home/wangjunbo/workspace-iot-platform/iot-doc/亿级点位高吞吐架构/1.MQTT-Broker/MQTT-接入网关/11.cloud-mqtt-gateway-capacity-planning.md:291)、[两层 lease](/home/wangjunbo/workspace-iot-platform/iot-doc/亿级点位高吞吐架构/1.MQTT-Broker/MQTT-接入网关/2.cloud-mqtt-presence-registry-design.md:175)
- 成本稿明确排除研发人力；不能据此宣称自研三年总成本较低。25 万或 50 万连接/节点、GC 百分比也仍需实测。[成本范围](/home/wangjunbo/workspace-iot-platform/iot-doc/亿级点位高吞吐架构/1.MQTT-Broker/MQTT-接入网关/4.cloud-mqtt-gateway-cost-analysis.md:33)、[容量估算](/home/wangjunbo/workspace-iot-platform/iot-doc/亿级点位高吞吐架构/1.MQTT-Broker/MQTT-接入网关/11.cloud-mqtt-gateway-capacity-planning.md:13)
- Go 的 Linux runtime 本来就使用 epoll；更换 gnet 等框架应由 profile 驱动。epoll 不能替代 TLS、认证、会话正确性和有界背压；Kafka 可缓冲已接收消息的下游波动，不能替 TCP 建连和 TLS 握手排队。[Go runtime 源码](https://go.dev/src/runtime/netpoll_epoll.go)

大模型可以帮助实现、构造测试、检查并发与分析 profile，但实现和测试可能共享错误假设。至少使用独立客户端/真实固件、消息核对账本、故障注入和原始指标交叉验证；模型生成的结论不能替代测量。

## 8. 建议的比较与停止条件

两组候选保持设备行为与业务 raw 契约一致：A = TBMQ CE + 薄适配，B = 专用自研接入网关。先验证契约和故障恢复，再逐级扩大连接数量；不为本次分析虚构工期、机器数或成本节省比例。

一人研发时，两条路线共用设备行为脚本、数据核对器和验收表：B 实现限定 MQTT 子集、raw 接入和明确 ACK 的极简 0.1；A 验证现成 broker 的配置与必要适配，不要求同时完整实现和维护两套系统。先做两者低成本正确性与单机对照，再按结果决定是否两者都进入百万规模。5 万元可设阶段支出上限：0.5 万正确性与真实设备、1 万 10–25 万连接对照、2 万百万连接、1 万故障恢复、0.5 万预留。这只是预算分配建议，不是云报价或一定足够的保证；压测客户端、云盘、流量、负载均衡也应计入，每阶段通过后才扩大。本文没有采购、创建云资源或执行压测。

| 验收场景 | 必须记录的结果 |
|---|---|
| 3 KB 正常上报，真实 QoS/TLS/认证 | 每连接 RSS、CPU、网络、PUBACK P95/P99、业务 raw 可见延迟 |
| 分时与同秒上报、断网后重连 | 建连成功率、连接恢复时间、认证与 Kafka 延迟、既有连接受影响程度 |
| Kafka leader/节点故障、ACK 前后 broker 崩溃 | 已确认消息丢失数、重复数、raw bytes/envelope 完整性 |
| 外部 Kafka 中断、IE 重启、消费位点重建 | 累积 backlog、恢复速度、是否跳过、是否可重放、retention 预算 |
| 重复 clientId、会话迁移、滚动升级 | 旧会话是否失效、命令是否误投、稳定在线设备是否被牵连 |
| 低频命令、PUBACK 丢失、设备执行 ACK 丢失 | MQTT 送达与业务执行状态能否区分、超时/重试/幂等是否正确 |

0.1 达到入口正确性和容量目标，就完成了本阶段的实验任务；不能据此自动宣称生产就绪。进入生产选型时，再比较自研实际收益、未覆盖协议边界与维护成本，以及 TBMQ 的适配与运维成本。若 TBMQ 薄适配已满足契约，可以采用它；若自研已有可测的成本或契约优势，再继续投入。无论选择哪种 broker，厂商报文解析、命令状态机、业务执行 ACK 与幂等仍属于本平台需要负责的契约。

## 9. 成本比较的统一口径

先通过必要的可靠性和设备兼容性门槛，再比较成本；不以资源低价抵消数据契约不满足。基础设施性价比与综合性价比分开记录。

```text
观察期总成本 = 首次实现/适配/验证投入
             + 观察期内云资源、模型/工具、许可/支持支出
             + 持续维护与升级投入
             + 可明确估算的延期和故障影响
```

本人投入同时记录小时数与可选的时间价值折算，不把“没有另外发工资”当作投入为零。两路共同需要的 Kafka、业务解码、数据存储和命令业务单独列账；TBMQ 内部额外持久化、桥接写入和运维依赖则记录增量。已有设计投入属于已发生成本，后续选择重点比较从现在起的增量投入。

TBMQ 即使多耗服务器，只要节省的实现、审查和维护投入大于额外资源成本，仍可能更划算。自研也不能仅因功能更少就认定更省：必须给出相同 ACK 语义、TLS、峰值负载与故障余量下的测量。报告应列出打平点、哪些变量最影响结论，以及未测量项；当前没有足够数据宣称哪条路线性价比更高。

可操作的打平公式：设 F 为从现在起的一次性实现/适配/验证成本，S 为月运行费，M 为月维护成本，T 为比较月数，则 `TCO(T) = F + T × (S + M)`。TBMQ 值得多花的月运行费上限为 `(F自研 - F_TBMQ) / T + (M自研 - M_TBMQ)`。成本中应计入本人时间、模型工具与结果审查；本次没有给这些变量填造数值。两条路线需同时展示相同资源下的能力，以及达到业务目标所需的资源；后者支持实际预算选择。

## 附录：上行 ACK、路由状态、百万 topic 与共享订阅

本次追问继续固定 TBMQ v2.4.0 提交 `2f60cfef8e058f4cd663b46bc4553ff23dfe89cc`，同时核对当前官方文档。没有进行运行时或内存容量测试。

### A. “先 Kafka、再 ACK、再匹配”哪些部分准确

上行消息先投递 `tbmq.msg.all`，QoS 1 PUBACK 等待 producer 成功回调；消费消息并匹配订阅的是另一条循环。因此可靠描述为“入口日志先行，ACK 不等待路由和客户端投递完成”，不是严格的“设备收到 ACK 后才开始路由”。第 5 节已据此修正为两个异步分支；QoS 0 根本没有 PUBACK。

路由侧 `processPublishMsg` 调用 `getAllSubscriptionsForPubMsg`，后者调用 `subscriptionService.getSubscriptions(topicName)`；拿到匹配订阅后查询本地 `ClientSessionCache`，获得 clientId、连接状态、所属 broker，再交给下行代理或持久化路径。[MsgDispatcherServiceImpl.java:99](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/processing/MsgDispatcherServiceImpl.java#L99)、[ClientSessionCache 查询:225](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/processing/MsgDispatcherServiceImpl.java#L225)

### B. topic 路由到底存在哪里

**热路径：各 broker 进程内存中的 Subscription Trie 与客户端会话缓存。持久化与集群同步：Kafka 的 `tbmq.client.subscriptions` 和 `tbmq.client.session`。** 不是每条 PUBLISH 都到 Redis 查一次订阅表。

订阅注册链可定位到固定源码：

1. `MqttSubscribeHandler` 检查 filter 格式、共享订阅条件与订阅 ACL，再调用 `subscribeAndPersist`；有效订阅的成功 SUBACK 在持久化成功回调中发送。[MqttSubscribeHandler.java:109](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/handlers/MqttSubscribeHandler.java#L109)
2. `ClientSubscriptionServiceImpl` 先调用本地 `SubscriptionService.subscribe` 更新 Trie 和客户端订阅集合，然后持久化完整客户端订阅集。[ClientSubscriptionServiceImpl.java:108](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/subscription/ClientSubscriptionServiceImpl.java#L108)
3. `SubscriptionPersistenceServiceImpl` 把订阅集编码成 protobuf，以 **clientId 为 Kafka record key** 发送；状态 topic 配置 compaction。[SubscriptionPersistenceServiceImpl.java:57](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/SubscriptionPersistenceServiceImpl.java#L57)、[key:124](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/SubscriptionPersistenceServiceImpl.java#L124)、[KafkaClientSubscriptionsQueueFactory.java](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/common/queue/src/main/java/org/thingsboard/mqtt/broker/queue/provider/KafkaClientSubscriptionsQueueFactory.java)
4. 每个 broker 用 `serviceId + 启动时间` 创建独立订阅 consumer group，恢复全部订阅后持续监听变化，重建自己的本地索引。[ClientSubscriptionConsumerImpl.java:83](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ClientSubscriptionConsumerImpl.java#L83)、[本地初始化:73](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/subscription/ClientSubscriptionServiceImpl.java#L73)

消息分担和状态复制是不同的消费模式：`tbmq.msg.all` 的共享 consumer group 分担处理消息；订阅状态独立 consumer group 让各节点持有全量路由视图。全量状态意味着增加 broker 不会自动把每节点订阅内存缩为原来的 1/N；恢复耗时与状态大小仍需实测。[入口共享 consumer group](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/common/queue/src/main/java/org/thingsboard/mqtt/broker/queue/provider/KafkaPublishMsgQueueFactory.java)

### C. 百万设备是否就是百万 topic

至少要分清三个计数：

| 对象 | 示例 | 百万设备时的数量关系 |
|---|---|---|
| MQTT 发布 topic name | `devices/device123/telemetry` | 若每设备一个上行名字，就有百万不同名字；这是命名选择 |
| MQTT subscription / topic filter | `devices/+/telemetry` | 一个后端客户端用一条 filter 可以匹配百万上行名字；无须逐设备注册百万条订阅 |
| Kafka topic | `tbmq.msg.all` | 百万 MQTT 发布名字进入同一个或少量内部 Kafka topic，通过 record key/partitions 分散；不是自动创建百万 Kafka topic |

若每设备还精确订阅自己的 `devices/{id}/command`，就会增加约百万条命令订阅关系；通配上行后端订阅不会消除这部分。若百万客户端都订阅同一条通配 filter，filter 路径虽可共享，仍有百万客户端订阅条目及潜在百万份投递；“一个 filter”不等于“一个订阅者”。这些是计数推导，不是性能实测。

固定源码的 Trie 节点包含 `children: ConcurrentHashMap` 和订阅 `values: Set`；`put` 只沿 filter 路径建节点、复用公共前缀。`get` 同时考察精确分支、`+` 与 `#`，并遍历匹配节点的 values 构造结果。[SubscriptionServiceImpl.java:49](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/subscription/SubscriptionServiceImpl.java#L49)、[ConcurrentMapSubscriptionTrie.java:64](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L64)、[put:138](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L138)、[Node:269](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L269)

所以可判断：百万条精确 filter 通常比单个消费者的一条通配 filter 占用更多节点、订阅条目和同步数据；但不能在未压测时声称每条订阅具体多少字节。匹配不是扫描百万条完整 filter，也不能普遍简化成 O(topic 长度)：**实际访问的通配/精确分支数和返回订阅者数都会耗时**；广播 fan-out 的输出量更不可能由 Trie 消除。

还有一个独立问题：`tbmq.msg.all` 的 record key 是 MQTT topicName。不同设备使用不同上行名字可按 key 分散；全部使用同一个 `telemetry` 名字会集中到一个 Kafka partition。这个热点与后端是否使用通配订阅无关。[MsgDispatcherServiceImpl.java:94](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/processing/MsgDispatcherServiceImpl.java#L94)

### D. 共享订阅支持范围与两种实现

TBMQ 支持 `$share/{ShareName}/{TopicFilter}`，**官方明确适用于其支持的各 MQTT 版本，包括 MQTT 3.1.1 和 5**；这是 TBMQ 的实现能力，不应把 MQTT 5 的标准化时间误当成 3.1.1 客户端在 TBMQ 上不能用。相同 shareName + filter 的组在一次分发中选一个成员；不同组各自分发。QoS 1 重投可能产生重复，共享订阅不意味着业务“恰好执行一次”。[Shared subscriptions](https://tbmq.io/docs/user-guide/shared-subscriptions/)

| 类型 | 组内选择与跨节点行为 | 预创建要求 |
|---|---|---|
| DEVICE | broker 根据全局订阅/会话视图选组员，当前 ROUND_ROBIN；远端 broker 上的在线客户端也可选；随后按会话所属节点走本地投递或 downlink Kafka | 不要求 APPLICATION shared entity；客户端直接 SUBSCRIBE，需通过 ACL |
| 持久 APPLICATION | broker 为 MQTT 客户端维护 Kafka consumer；同 shareName + filter 对应同 consumer group，由 Kafka 分配共享 topic 的 partitions；客户端连接在任一 broker 均可参与 | 先建 Application Shared Subscription entity，按裸 filter 唯一，创建时指定 partitions |

DEVICE 固定源码按 `(topicFilter, shareName)` 分组；在线远端 session 的本地 ctx 为 null 时仍认定有资格；轮询计数为处理节点上的内存状态，不宜宣传为一个全局严格均匀的轮询序列。无在线成员时还可能走持久组的离线处理分支，不能笼统说所有共享消息离线即丢弃。[DeviceSharedSubscriptionProcessorImpl.java:58](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/processing/shared/DeviceSharedSubscriptionProcessorImpl.java#L58)、[远端在线资格:119](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/processing/shared/DeviceSharedSubscriptionProcessorImpl.java#L119)、[轮询状态](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/shared/SharedSubscriptionProcessorImpl.java#L34)、[DownLinkProxyImpl.java:41](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/processing/downlink/DownLinkProxyImpl.java#L41)

APPLICATION 的细节会影响本项目：

- 共享 Kafka topic 按 **filter** 命名，例如 `tbmq.msg.app.shared.test.mlw` 对应 `test/#`，不是每个 shareName 建一份 topic。consumer group 则包含 **shareName + sharedAppTopic**，所以两个业务组可独立读取同一日志。固定源码先按 filter 去重，只写一份共享日志，再由不同消费组分发。[MsgPersistenceManagerImpl.java:234](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/mqtt/persistence/MsgPersistenceManagerImpl.java#L234)、[ApplicationPersistenceProcessorImpl.java:499](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/mqtt/persistence/application/ApplicationPersistenceProcessorImpl.java#L499)、[官方 topic/group 规则](https://tbmq.io/docs/architecture-details/persistent-app-client/#shared-subscriptions)
- 持久 APPLICATION 订阅未预创建的 filter 会被拒绝；管理实体的 filter 和 partition 数创建后不能通过该实体修改。并行消费成员数受 partition 数约束，不能用增加 MQTT consumer 数绕过。[MqttSubscribeHandler.java:143](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/handlers/MqttSubscribeHandler.java#L143)、[管理文档](https://tbmq.io/docs/user-guide/ui/shared-subscriptions/)
- **共享 APPLICATION 日志的 record key 为 null**，没有沿用入口 topicName/deviceId key。`sendSharedTopicMsg` 调用无 key 构造器，该构造器明确填 null。因此默认不能保证同设备的连续消息始终交给同一个 worker，也不能继承入口同 key 的分区顺序。[MsgPersistenceManagerImpl.java:207](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/mqtt/persistence/MsgPersistenceManagerImpl.java#L207)、[TbProtoQueueMsg.java:37](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/common/queue/src/main/java/org/thingsboard/mqtt/broker/queue/common/TbProtoQueueMsg.java#L37)

DEVICE 与 APPLICATION 即便 shareName/filter 相同，也按客户端类型分开处理，不能混合两类当成同一个分工组。对于本项目无状态解码，共享订阅可帮助分担；若规则、命令或去重要求设备级连续状态，需要单独设计 deviceId 分区、串行处理或顺序号检查，不能仅凭“用了共享订阅”宣称同设备保序。这些是基于默认实现的设计含义，不预设自研或 TBMQ 哪条路线胜出。

### E. 全量映射、状态传播与 Kafka 事务的区别

对“全量 Redis → Kafka 事务广播 → 各 broker 全量映射”的核验结论：最后一项基本成立，前两项需要修正。

- **持久状态**：Kafka compacted topics 保存客户端订阅与会话状态，订阅记录以 clientId 为 key、该客户端的完整订阅集合为 value。单设备变更写的是该设备的集合，不是全平台订阅表。Kafka 日志压缩异步清理同 key 的历史版本，不能把它理解成每次同步覆盖一个数据库行。[订阅持久化源码](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/SubscriptionPersistenceServiceImpl.java#L57)、[官方状态恢复说明](https://tbmq.io/docs/architecture/#session-state-and-cluster-coordination)
- **查询视图**：每节点内存中的 Trie 提供 topic filter → 订阅客户端的反向索引；会话缓存提供 clientId → 所属 broker / 在线状态。节点持有全局订阅与会话元数据，TCP/Netty 连接对象只属于实际接入该连接的节点。Redis 承担 DEVICE 待投递消息等职责，不是这张全量订阅路由表。[官方架构](https://tbmq.io/docs/architecture/)
- **传播机制**：每个节点用 `serviceId + 启动时间` 建立独立 consumer group，启动时回放恢复，随后持续消费变更。源节点已经做过本地修改，会跳过来源为自身的更新；其他节点把更新交给客户端 Actor 应用。[订阅消费者](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ClientSubscriptionConsumerImpl.java#L83)、[来源判断](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/subscription/ClientSubscriptionServiceImpl.java#L285)
- **不是事务广播**：该持久化路径调用普通 `KafkaProducer.send(record, callback)`；本次固定版本源码检查没有发现该路径开启、提交 Kafka transaction。让所有节点都收到状态的原因是独立消费组，不是事务机制。[TbKafkaProducerTemplate](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/common/queue/src/main/java/org/thingsboard/mqtt/broker/queue/kafka/TbKafkaProducerTemplate.java#L82)

订阅处理先修改接入节点本地集合与 Trie，再持久化单客户端订阅集合；有效订阅成功 SUBACK 等待 Kafka producer 成功回调。代码没有等待其他节点全部应用变更的全局屏障，所以 **SUBACK 成功不等于所有节点的 Trie 此刻均已更新**，只能描述为异步传播。所读失败回调路径未见回滚先前的本地订阅修改；应验证 Kafka 写入失败、状态消费者延迟、订阅后立即跨节点下发，以及故障恢复后的收敛，不能仅据静态阅读断言永久不一致。[本地更新与持久化](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/subscription/ClientSubscriptionServiceImpl.java#L109)、[SUBACK 回调](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/handlers/MqttSubscribeHandler.java#L156)

本项目若百万设备各保留两条订阅，则每节点需要表示约 200 万条订阅关系。假设四节点连接均匀，每节点约 25 万连接，但仍需全局约 200 万条订阅关系，不能按 50 万条估算；这是逻辑条目数推导，不是内存容量实测。评估应同时测稳态内存、重订阅造成的状态写入与各节点应用速度、冷启动恢复耗时。扩节点能分担连接和消息处理，但不会按节点数等比例缩小这份全局索引。
