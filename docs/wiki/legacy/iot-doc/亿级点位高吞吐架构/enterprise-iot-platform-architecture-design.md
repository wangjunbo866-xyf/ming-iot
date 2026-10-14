# 企业级物联网平台总体架构设计

日期：2026-07-23
状态：总体方向已确认，作为新平台的主架构决策文档
目标形态：JetLinks、ThingsBoard 级可私有化企业平台，而不是华为云 IoT、AWS IoT 级全球公有云

## 1. 最终决策

本项目采用：

> **独立 monorepo + 自有领域契约 + 新的数据热路径 + 选择性借鉴/复用开源项目。**

不选择以下三条路线：

- 不以当前联犀开源版 `things` 作为主仓进行深度 fork；
- 不以 JetLinks Community 或 ThingsBoard CE 作为运行内核再改写技术栈；
- 不从第一天重做一个包含通用流计算、跨地域、边缘编排和应用市场的巨型 IoT 云。

首版冻结的是代码与契约边界，不是三个固定进程：

1. `saas` 是系统管理域：租户、组织/部门、用户、角色、资源、授权和审计等。`saas/service/apisvr` 提供对外 API，`saas/service/syssvr` 是内部 RPC 与系统管理事实来源；
2. `things` 是 IoT 域：产品、设备、拓扑、物模型、查询、自动化、告警和命令。`things/service/apisvr` 提供对外 API，`things/service/dmsvr` 是 IoT 控制面 RPC/facade；`things/runtime/*` 是事件驱动的热路径与状态写入逻辑，不伪装成 API/RPC 服务；
3. `protocol` 是 Go Workspace 中的独立模块，提供共享 SDK/runtime、内置 codec 和多个 `ProtocolApplication`，而不是单一的协议服务；
4. Access Adapter/Gateway 保留两阶段：一期由 EMQX 终止 MQTT 连接并经 adapter 写入 Kafka Durable Raw Log，下一期才由 self-hosted Gateway 接管连接、会话与 PUBACK 边界；
5. V1 把“设备 MQTT Topic”冻结为入口路由契约：EMQX/Gateway 用 ACL 绑定凭证与 Topic，Adapter 只做确定性的 Kafka Raw topic/key/header 投影和原 payload append；tenant、内部 deviceId、协议与物模型 release 到 ProtocolApplication 阶段才解析；
6. Raw 不定义业务 Envelope：事实只由 Kafka Raw topic、可信 identity key、原 MQTT payload、少量 transport headers 和 Kafka commit metadata 组成；不逐消息维护 schema/topic/ingress/ingest/tenant/route/release 字段；
7. 生产数据拓扑正式选择 **Kafka Durable Raw + Durable Canonical Stream**。Canonical 当前就服务 Timeseries Materializer、Scene Admission 和 reducer，并为未来 Flink 提供稳定输入，不再保留“融合直写 TDengine、无 Canonical Stream”的生产候选；
8. V1 默认一个物理 Kafka 集群，但从第一天把高吞吐设备数据与低频 saas/things 领域事件按 topic prefix、principal、client ID、consumer-group namespace、quota 和配置端口逻辑隔离；达到明确 SLO、故障域或合规门槛后才拆成 Data Kafka 与 Domain Kafka，Raw 与 Canonical 始终留在同一个 Data Kafka。

`Protocol Runtime` 和 `Timeseries Materializer` 是逻辑边界，可以部署在同一宿主，也可以独立扩缩，但生产语义始终经过 Kafka Canonical topic 和各自 consumer group；同进程不允许改成绕过 Canonical 的 direct sink。根组合入口 `cmd/allInOne` 只组合同一份代码；拆分部署时，service 可换用 direct client 或 gRPC client，但本项目不承诺 fork goctl 或维护第二套业务实现。

本文中的 **V1 表示首版功能边界**：Phase 1 用这组功能完成单客户试点，除 Data Kafka 耐久基线外可以存在已披露的单点，并必须明确 RPO/RTO；生产试点的 Data Kafka 仍至少是 3 broker（可为合并 broker/controller 角色或等价托管服务）、Raw/Canonical RF=3、`min.insync.replicas=2`、producer `acks=all`，不允许用单 broker 试点代替生产耐久性契约。同一功能集完成 Phase 2 的全组件多副本、备份恢复和故障演练后，才能标记为 **Enterprise GA**。Phase 3 是 GA 后按瓶颈演进，不再使用含糊的 V1/V1.1 名称掩盖可靠性差异。

### 1.1 对规则引擎与 eKuiper 的新结论

- MQTT/TCP/WebSocket 接入与写入持久消息日志属于 **Access/Data Infrastructure**，不是规则；
- 厂商报文与平台标准消息之间的编解码属于 **Protocol Runtime**，不是规则；
- 物模型校验、模块路由、TDengine 批写和最新值投影属于 **Telemetry Pipeline**，不是规则；
- 阈值、持续时间、恢复、场景联动属于 **Automation/Rule**；
- 告警、通知、设备命令分别有自己的领域状态机，不能退化为无状态 sink。

V1 不把 eKuiper 放入云端主链路，也不建设“每条规则一个 Pod”的规则编排平台。共享 Automation Runtime 负责倒排索引匹配以及持续时间、去抖、冷却、迟滞、可靠延时等固定大小有限状态；`N 次/T` 留到下一阶段并施加严格 N/T/状态字节限制。SUM/AVG/MIN/MAX/COUNT 窗口先规范化为可共享 `ComputationPlan`，滑动/会话窗口、跨设备聚合、JOIN、watermark、迟到修正和分布式状态迁移优先由 Flink 等成熟引擎执行。eKuiper 保留为未来边缘站点或轻量流处理的可选适配器，不成为平台领域模型。

## 2. 产品定位与边界

### 2.1 “企业级”的含义

本项目的企业级目标是“一个企业能够稳定采购、私有化部署、二次开发和长期运营”，应具备：

- 单地域、单集群或客户独立部署；
- 多租户逻辑隔离、RBAC、配额、审计和可选租户独立部署；
- MQTT/TLS、设备凭证生命周期、稳定 OpenAPI/Webhook；
- 产品、设备、版本化物模型和协议扩展；
- 遥测历史/最新值、告警、通知、设备命令和简单业务联动；
- 可供实施和运维使用的管理台：设备诊断、趋势查询、告警、命令轨迹、DLQ 和重放；
- 至少一次处理、稳定幂等键、按各领域 reducer 收敛乱序、DLQ 和可重放；latest 使用冻结 comparator 做投影 CAS，Automation 对同一 `subjectKey` 的 `LIVE` 输入与 timer fire 使用串行 processing-order lane，不把 deviceTime、ProtocolApplication 的进程 FIFO 或跨 partition 到达顺序当成场景事件时间承诺；
- 单地域 HA、滚动升级、备份恢复以及明确的 RPO/RTO；
- 根据给定硬件，对连接数、消息率、点位率、查询并发和保留期给出可验证容量。

“百万设备”不是单一容量指标。容量承诺必须使用向量：

```text
并发连接数 / 上行 msg·s⁻¹ / 上行 MB·s⁻¹ / 解码后 points·s⁻¹
/ 规则执行数 / 下行命令数 / 查询 QPS / WebSocket 订阅数 / 数据保留量
```

V1 统一容量锚点为 100 万逻辑设备（含子设备）、500 points/report、3 module rows/report。四档口径必须分开表达：

| 档位 | 计算口径 | report/s | points/s | TD 宽行/s |
| --- | --- | ---: | ---: | ---: |
| 24h 日均 | 12h 内每设备 5 分钟一报，摊到 24h | 1,667 | 约 83 万 | 约 5,000 |
| 12h 活跃稳态 | 只在活跃时窗内摊平 | 3,333 | 约 167 万 | 约 1 万 |
| 60s 集中峰值 | 100 万设备在 60s 内各报一次 | 16,667 | 约 833 万 | 约 5 万 |
| Phase 1 双峰值压测上限 | 集中峰值的 2× | 33,334 | 约 1,667 万 | 约 10 万 |

**1,667 万 points/s 是 Phase 1 双峰值压测上限，不是常态业务速率。** 仅按当前最小数值 8 B/point 计算，该上限已约为 133 MB/s；它还未包含 descriptor/header、Kafka RF=3 复制、Raw + Canonical 双写、多 consumer group 读取和故障追赶。因此两条数据通道必须同时按 MB/s 验收，不得只看 msg/s。

所有 Spike、容量 ADR 和压测报告以这组锚点为基准记录硬件、payload、模型和副本条件。当前亿级点位场景是平台数据面的目标负载样本，不代表每个业务模块都要按全球公有云设计。

### 2.2 明确不做

V1 功能范围及首次 Enterprise GA 不承诺：

- 全球多地域 active-active 和设备会话跨地域无缝迁移；
- serverless、多云统一管控、按量计费和全球设备目录；
- 内置数据湖、通用 BI、AI 平台、数字孪生建模器和应用市场大全；
- V1 的低代码大屏/SCADA 设计器和任意业务应用搭建器；
- 同时支持所有 Broker、消息队列、时序库和规则语言；
- 端到端 exactly-once；
- 通用 BPMN、Flink 替代品或任意可视化流计算 DAG；
- 任意租户上传脚本、JAR、Go 插件或镜像到共享主链路；
- V1 的跨集群控制、离线边缘节点编排和 KubeEdge 替代品；
- 为每个客户永久维护一条核心代码 fork 分支。

## 3. 架构原则

1. **先持久化、后解释**：平台收到的设备原始报文先被 durable raw log 接管，再异步解码、校验和写入；一期 EMQX 模式下，这不等同于设备 PUBACK 会等待 Raw Log 提交；
2. **控制面与数据面分离**：关系主数据和发布控制不进入逐消息热路径；
3. **发布物不可变**：物模型、协议、规则和运行配置发布后不可原地修改；
4. **一个业务事实，一个状态机**：告警、命令、通知分别拥有唯一事实来源；
5. **至少一次 + 幂等**：不以模糊的 exactly-once 承诺掩盖 Kafka transaction/offset、外部 sink 重试和副作用边界；
6. **代码边界与部署边界分离**：API/RPC service、事件 runtime role 和进程不是同一层概念，只按压测与故障域证据组合或拆分；
7. **主链路不执行任意代码**：客户差异通过版本化适配器、受控映射或隔离运行时吸收；
8. **技术栈先固定，端口保持稳定**：V1 不为尚不存在的第二种 MQ/TSDB 写复杂通用框架；
9. **可重放是修改模型和规则的安全网**：历史解释必须能由 Raw durable position、不可变发布时间线与处理 provenance 恢复；
10. **压测证据高于项目宣传**：竞品的“海量”“集群”“高性能”不能直接成为本项目容量结论。

## 4. 总体逻辑架构

```text
                              Console / OpenAPI
                         ┌─────────┴─────────┐
                         ▼                   ▼
             saas/service/apisvr    things/service/apisvr
                         │                   │
                         ▼                   ▼
             saas/service/syssvr    things/service/dmsvr
       Tenant/Org/User/Role/Authz   IoT control + Alarm/Command facade
                         └─────────┬─────────┘
                                   ▼
                              PostgreSQL

Device ─ MQTT
          ├─ 一期：EMQX → Access Adapter
          └─ 下一期：self-hosted Gateway
                         │ 经 ACL 保护的标准 MQTT Topic
                         │ 可逆机械映射，无设备/租户/路由查询
                         ▼
                 Kafka Durable Raw Topic
 topic = iot.data.raw.v1.<protocolKey>
 key = <d|g>/<trusted identity...>；value = 原始 MQTT payload
                         │
                         ▼
             protocolKey 的唯一生产 ProtocolApplication owner
             多副本加入同一 production consumer group
             Protocol Runtime → bounded parse/validation
             按 rawRef/offset-vector cutover history 解析 ownership/协议/物模/mode
             DecodeManifest：0..N typed outputs
                         │ Kafka transaction：outputs + Raw offsets
                         ▼
              Kafka Durable Canonical Stream
               ├─ Materializer consumer group
               │  每完整 batch 一次 decode；全部模块 history
               │  + mode-required latest 后提交 Canonical offset
               ├─ Scene Admission consumer group
               │  校验 ProcessingContext 后规范化 → Kafka scene_event
               │       └─ Scene Runtime consumer group
               │          Actor 内存 state/timer + PG Action Outbox only
               │          + 异步 best-effort scene_state（无跨存储事务）
               ├─ reducer consumers：Lifecycle/CommandReply → accepted transition
               ├─ compact audit projection → Audit consumer
               └─ Flink（未来）：窗口/JOIN/跨设备有状态计算

things API / Automation → CommandExecution + Outbox
                         → ProtocolApplication encode → CommandAttempt
                           （protocolKey + 冻结的 codec/目标 topic/payload）
                         ├─ 一期：EMQX publish adapter → Device
                         └─ 下一期：owner Gateway → Device
```

图中是唯一生产拓扑。ProtocolApplication 只有在 Canonical/quarantine/`NO_OUTPUT` 输出与所消费 Raw offsets 在同一 Kafka transaction 中提交后，才完成该批 Raw；外部数据库 sink 仍使用至少一次、稳定 ID 和后置 offset commit，不把 Kafka transaction 夸大成跨 TDengine/PostgreSQL 的全局 exactly-once。下行链路中，`things/service/apisvr` 或 Automation 先创建 `CommandExecution + Outbox`，command runtime 再按 CommandAttempt 冻结的 `protocolKey/protocolReleaseId` 调用对应逻辑 ProtocolApplication 编码，并保存已编码 payload 与目标 MQTT Topic供后续重试。Gateway/EMQX publish adapter 只做 transport 投递，不重新解释物模型或执行业务规则。

### 4.1 控制面、设备数据面与领域事件面

| 平面 | 事实来源 | 主要职责 | 禁止事项 |
| --- | --- | --- | --- |
| 控制面 | PostgreSQL | `saas/service/syssvr` 管理租户/授权事实；`things/service/dmsvr` 管理 IoT 主数据/发布并作为告警/命令 facade，实际状态分别由 `things/runtime/alarm` 和 `things/runtime/command` 写入 | 不参与逐点查 schema，不保存每条实时点值 |
| 设备数据面 | Kafka Durable Raw、Durable Canonical、TDengine、latest 与规则状态 | 持久化接入、ProtocolApplication 分发与解码、时序物化、自动化以及未来 Flink | 不在 Adapter 热路径同步查设备、租户、物模型或路由表；业务模块不直接消费厂商 Raw |
| 领域事件面 | PostgreSQL 事务事实 + Outbox；Kafka 是投递日志 | 发布 `platform.saas.event.*`、`platform.things.event.*` 等低频领域事实，驱动缓存、跨域投影与集成 | Kafka event 不能反客为主覆盖 PostgreSQL 状态机；高频设备报告不能伪装成 saas/things 领域事件 |

### 4.2 为什么同时保留 Raw 与 Canonical

小型物联网平台强调把设备管理、MQTT、协议和时序写入放入融合式 `dmsvr`，减少跨进程跳转；这是合理的低延迟路线。但本项目面向可重放的工业数据链路，选择两个语义不同的 Kafka 日志边界。

Durable Raw 位于 Access Adapter/Gateway 与协议层之间，承担：

- Adapter/Gateway → 平台数据面的 durable handoff，以及入口与协议/TDengine 故障隔离；
- 有界吸收集中上报、下游停机和网关重连后的补传洪峰；
- consumer 崩溃、扩缩容和发布升级后的恢复；
- 协议/物模型修复后的原版本重放与迁移重算；
- 短期原始证据、故障诊断和 shadow/canary 对照。

Raw 不是长期时序库、领域 EventBus、Kafka 备份或无限缓冲。生产 hot retention 固定以 72 小时为基线；需要 30/90 天纠错、审计或灾难恢复时按合同启用加密对象归档。网关离线期间数据尚未 append 到云 Kafka，仍由 Gateway/Edge 的 store-and-forward 契约负责，不能把云端 72 小时写成“设备断网保留 72 小时”。

Durable Canonical 位于协议层与平台数据消费者之间。ProtocolApplication 收到 Raw 后，在内存完成有界解码并冻结 `DecodeManifest`；每个 expected output 要么成为完整 `CanonicalDataBatch`/低频标准事件，要么进入对应 quarantine，零输出则产生唯一的 `NO_OUTPUT` outcome。它们与 Raw consumer offsets 在同一 Kafka transaction 中提交，下游只读取 `read_committed` 数据。Canonical 当前就有以下必要消费者，而不是只为未来 Flink 预留：

- Timeseries Materializer：一个 consumer group 同时完成完整报告的 TDengine history 与 mode-required latest；
- Scene Admission：消费完整 PropertyReport、append-only DeviceEvent、`ComputedMetricUpdated` 与 reducer 已接受的状态迁移，只把通过 production+LIVE 门禁的事实规范化为 `scene_event`；Scene Runtime 不直接消费 Canonical；
- Presence/Command reducer：先收敛 Lifecycle/CommandReply，再发布 `AcceptedStateTransition`；
- Audit：消费 compact audit envelope、处理 outcome 和状态迁移摘要，不复制并扫描全部点值；
- Flink（未来）：只消费 Canonical/派生语义流，不让业务 Job 理解厂商 Raw。

Canonical hot retention 按必要 consumer 的“最大停机/发现修复时间 + 积压追赶时间 + 安全余量”计算，产品基线不得低于 24 小时；Phase 0 以 24/48/72 小时三档验证并冻结部署档位。Canonical 超期后的重新生成回到保留的 Raw 或对象归档，不把所有历史恢复责任压给 Canonical。

云端 V1 只实现 Kafka，不做 NATS/JetStream 与 Kafka 的双运行时兼容。难点不在抽象一个 `Publish/Consume` 接口，而在 transaction + offsets、consumer-group rebalance、`read_committed`、partition fencing、retention/归档和故障注入都是生产契约；双适配会使发布、重放、监控和测试矩阵近似翻倍，并迫使设计退化为最小公分母。未来边缘站点可以使用 eKuiper 或本地轻量总线，但通过稳定的 Raw/Canonical 契约与云端对接，不因此要求云端主链路同时维护两套 MQ 语义。

### 4.3 Kafka 逻辑隔离与物理拆分

V1 的 `DataKafkaBootstrapServers` 与 `DomainKafkaBootstrapServers` 默认指向同一物理 Kafka 集群，但两类客户端必须使用不同 principal、`client.id`、consumer-group prefix、Topic ACL、quota 和告警：

```text
Data plane:   iot.data.raw.* / iot.data.canonical.* / iot.data.quarantine.*
Domain plane: platform.saas.event.* / platform.things.event.*
```

Raw 与 Canonical 始终在同一个 Data Kafka，保留 consume-transform-produce transaction、低延迟与同故障恢复边界。Topic retention 不同不是拆集群理由；Kafka 支持逐 Topic 配置。满足以下任一条件才把 Domain Kafka 物理拆出：

- saas/things 的 SLO 明确要求在 Data Kafka 磁盘满、升级或整集群故障时继续工作；
- “设备双峰值 + 受控 replay + 一台 broker 故障”压测在 quota 后仍破坏领域事件 p99/lag；
- Raw 敏感 payload 需要物理/KMS/审计隔离；
- 数据面与领域面的容量、扩容或升级节奏已经明显不同。

Kafka quota 可以限制 principal/client 的 produce、fetch 和 request rate，却不能消除共享磁盘、page cache、controller 或整集群故障；因此“单集群”是 V1 部署基线，不是虚假的物理故障隔离承诺。拆分后，跨 Data/Domain Kafka 的状态传播继续以 PostgreSQL Outbox 和幂等 consumer 为边界，不设计跨集群事务。

V1 的一个平台部署中只有一个 `DataKafkaBootstrapServers` 目标，不实现按 tenant 动态路由到多个 Data Kafka。大租户可以在共享 Data Kafka 内使用专属 protocolKey/topic/principal/quota/workload；若合同要求 tenant 级 KMS、独立删除或物理故障域，V1 交付独立私有化平台部署（其内仍只有一个 Data Kafka），而不在同一控制面下增加第二个 Data Kafka。§4.3 的“物理拆分”只指从现有集群迁出 Domain Kafka，不是 tenant 数据分片方案。

### 4.4 两种 Access Adapter/Gateway 拓扑

一期沿用现有 EMQX，平台不拥有设备 socket：

```text
uplink:   Device → EMQX → persistent MQTT subscription/Access Adapter → Kafka Durable Raw topic
downlink: Command Outbox → ProtocolApplication codec → EMQX publish adapter → Device
presence: EMQX lifecycle event/snapshot → presence projection
```

下一期 self-hosted Gateway 才拥有 session：

```text
uplink:   Device → Gateway → Kafka Durable Raw topic（`acks=all` 且 minISR 满足后才可定义 PUBACK）
downlink: Command Outbox → ProtocolApplication codec → registry/lease → owner Gateway → Device
presence: owner Gateway lease/session event → presence projection
```

一期的设备 PUBACK、会话和下行可达性由 EMQX 语义决定；平台 Access Adapter 到 Kafka Raw commit 是第二段可靠性边界。下一期才能把设备 QoS 1 接管点与 Kafka `acks=all` 成功绑定。两种模式都通过同一 `RawRecord(topic + key + headers + value + Kafka metadata)`、`CommandAttempt` 和 Canonical reply 契约进入领域层。云端 Raw 72 小时从 Kafka append time 起算；网关/链路离线期间尚未进入 Kafka 的数据必须由边缘存储、TTL、配额、溢出和补传 ACK 契约覆盖。

`MQTT-接入网关/` 下部分早期专项文档仍采用 tenant/device Raw Envelope 或旧 Topic 示例，它们保留的是下一期 Gateway 的容量、会话和网络研究，不再是平台入口消息契约；涉及 Topic、Raw identity、tenant 解析和协议路由时一律以本文 §7.1 为准，进入 self-hosted Gateway 实施前必须专项收敛。

## 5. 代码模块、API/RPC 与可组合运行时

### 5.1 saas：系统管理域

- `saas/service/apisvr` 是租户、组织/部门、用户、角色、资源、授权和审计的对外 API；
- `saas/service/syssvr` 是系统管理事实的内部 RPC 唯一入口，独占租户层级事务、状态变更和请求侧 TenantScope 编译；
- 其他域通过 `PrincipalContext + TenantScope` 使用授权结果，不能跨域直查、直写 saas 表；
- API 和 RPC 是接口/所有权边界，可以被同一宿主组合，不意味着必须有两个 Deployment。

### 5.2 things：IoT 域

- `things/service/apisvr` 对外提供产品、设备、拓扑、物模型、history/latest 查询、自动化、告警和命令 API；
- `things/service/dmsvr` 是产品、设备、拓扑、ModelRelease 和 Scene current Binding 等 IoT 控制面的 RPC/领域入口，也是告警/命令的 API/RPC facade；它不冒充告警或命令状态写入者；
- `things/runtime/presence` 是 presence projection 唯一写入 package，统一归并 EMQX lifecycle/snapshot 与未来 Gateway lease/session event；`things/runtime/alarm` 是告警状态唯一写入 package，`things/runtime/command` 是命令/attempt/outbox 状态唯一写入 package；`things/runtime/automation` 产生非持久化求值 trace/metrics，以及已验证 production+LIVE 状态迁移对应的 AlarmSignal/正式 Action Outbox，不为每个候选创建 durable evaluation row；Scene V1 不生成隔离 replay Outbox；
- `things/runtime/{timeseries,latest}` 共同实现 Timeseries Materializer：同一完整 PropertyReport 一次 decode，全部模块 history 与 processing mode 要求的 latest CAS 决策/更新完成后才推进该 Canonical partition 的连续 next-offset。遥测与遥信都走该边界；
- runtime package 通过有界队列、lease、cursor/checkpoint 和领域 port 工作，不为“看起来像微服务”而暴露伪 API/RPC。

### 5.3 protocol：多个 ProtocolApplication

`protocol` 不是一个 `protocolsvr`。它包含共享 SDK、批次 builder、Protocol Runtime、内置 codec/声明式 mapping 和多个可独立发布的 `ProtocolApplication`：

- `protocolKey` 是设备厂家与平台在接入前约定的稳定逻辑协议通道，也是 MQTT Topic 与 Kafka Raw topic 的路由 token；它不能是 Pod、Deployment、镜像版本、tenantId 或可变的 `protocolApplicationReleaseId`；
- `protocolKey` 按“厂家 × 协议族”从紧分配：新增一个 key 的边际成本是一个受管 Raw topic/topic generation、一组 ACL 与 catalog 记录，而从共享 key 拆出部分产品的成本是设备 MQTT Topic/ACL 迁移。应按厂家和 wire 协议族预分，但也不能无限制造小 topic；具体 topic/partition 数受 Phase 0 broker/controller/文件句柄容量门禁约束；
- `ProtocolRouteCatalog` 只保存 `protocolKey → Kafka Raw topic generation、逻辑 ProtocolApplication owner、production consumer group、partition/offset release timeline 与运行状态`，不保存设备清单；
- 每个 `protocolKey` 同一时刻只有一个生产处理 owner 和一个 production consumer group。一个 ProtocolApplication workload 通常拥有一个或少量明确配置的 `protocolKey`，其多个 Pod 作为同组成员分担 partitions；shadow/canary 使用独立 group 且只能写隔离 namespace，不能与 main 双写生产 Canonical；
- ProtocolApplication 只订阅一或少量 `iot.data.raw.v1.<protocolKey>` topic，不创建设备级 Kafka topic、consumer group 或 subscription；
- `protocolReleaseId` 选择不可变 codec/mapping 语义，`protocolApplicationReleaseId` 记录实际镜像/runtime release。二者由 ProtocolApplication 根据 `protocolKey + rawRef` 和控制面 release timeline 选择并写入处理 provenance，不由 Adapter 写进 Raw；
- `ownerTenantId`、`ownershipBindingId`、canonical deviceId、`modelReleaseId` 与 `processingModePolicyReleaseId` 由 ProtocolApplication 根据 Kafka record key 中可信的 product/device identity、`rawRef` 以及 Ownership/Model/Mode cutover timeline 的本地只读发布缓存解析；缓存由发布事件预热，热路径不调用 saas/关系库。Product/Project/Area/group affiliation 不在 ProtocolApplication 按 rawRef 冻结，Scene 在处理时解析 current `SubjectContext`；网关 key 中的 child identity 仍必须在这里校验 gateway/sub-device 拓扑；
- 标准 codec 和受限 mapping 可共享 runtime/image。共享 workload 只承诺有界 worker、每 codec 配额/超时、panic recover 和有界重试；fatal、进程 crash 或 OOM 仍可能影响同 workload 的其他 codec，不能声称崩溃故障域隔离；
- 客户专属/闭源/CPU 密集型、需要独立发布/合规边界，或已出现持续 CPU/SLO 干扰、重复 panic/OOM 的 codec，必须升级为独立 ProtocolApplication workload；
- 只有独立 workload 承诺镜像发布、资源配额和 crash/OOM 故障域隔离，并可独立扩副本、灰度与回滚；
- 所有 codec 必须有 golden vectors、模糊测试、payload/CPU/内存上限和制品来源清单，V1 不在共享进程加载租户任意代码。

### 5.4 Access Adapter/Gateway

一期 EMQX Access Adapter：

- 以持久 MQTT subscription/受控 bridge 接收上行，并在 Kafka producer `acks=all` 成功后确认“EMQX 到平台”这一段投递；Raw topic 使用 RF=3、`min.insync.replicas=2`，无法满足 ISR 时必须失败而不是降级成功；
- 只校验 MQTT Topic 语法和 token allowlist，然后机械提取 `protocolKey`、可信设备 identity 与 `messageKind`：选择 `iot.data.raw.v1.<protocolKey>`，以 identity 编码作为 Kafka key、`message-kind` 作为必需 header，并把原始 MQTT payload 原样作为 value；不查询设备、tenant、物模型、协议发布或应用发布信息；
- Raw 不再有业务 Envelope，也不逐消息携带 `schemaVersion/topicSchemaVersion/ingressProfileId/ingestEventId/tenantId/routeSnapshotId`。MQTT Topic 中的全局 `v1` 已表达入口语法版本；`mqtt-qos/mqtt-retain/broker-received-at/trace` 仅在确有用途时写 Kafka Header；
- 设备凭证与允许发布的 Topic pattern 由 EMQX 本地认证/ACL 数据提前下发并默认拒绝，不能在集中上线或逐消息热路径同步调用平台 HTTP。Adapter 信任“EMQX 已按凭证完成 Topic ACL”，但仍拒绝非法 token 和不可逆映射；
- 消费 EMQX lifecycle event/snapshot 并交给 `things/runtime/presence` reducer，只有 accepted transition 才更新 presence/触发自动化；接收 transport-ready command，经 EMQX publish adapter 发布；
- 不声称拥有设备 session，也不执行 Redis owner → gRPC Gateway 寻址。

下一期 self-hosted Gateway 负责 MQTT 连接、TLS、认证、KeepAlive、限流和会话所有权；它执行相同的凭证—Topic ACL 与 Kafka record 投影，不加载 tenant、完整点模型或动态路由快照，并把 lease/session event 交给同一 presence reducer，维护 gateway owner 与投递已编码命令。

V1 只正式支持 MQTT，默认交付 EMQX adapter；TCP、WebSocket、HTTP 和工业网关以后各自建立可信 identity/channel，再投影成相同的 `RawRecord` 语义，不要求伪装成 MQTT，也不能把物模型解析塞回 Adapter。

### 5.5 运行时 role 与部署组合

逻辑 role 包括 Protocol decode、Timeseries Materializer、Presence reducer、Automation/Alarm/Action、Command 和 audit envelope projection。代码边界不规定 Pod 数量：

- ProtocolApplication 对消费批次执行 bounded decode，随后在同一 Kafka transaction 内写入完整 Canonical/event、对应 quarantine 或 `NO_OUTPUT`，并用 `sendOffsetsToTransaction` 提交各 Raw partition 的连续 next-offset；只有事务成功才完成这批 Raw，下游一律 `read_committed`；
- Timeseries Materializer 只有一个逻辑 consumer group/checkpoint。每个完整 PropertyReport 只 decode 一次；该报告全部模块 history 与 processing mode 要求的 latest CAS 决策/更新均成功后，才提交该 Canonical partition 的连续 next-offset；
- Scene Admission 消费完整 PropertyReport/append-only DeviceEvent/`ComputedMetricUpdated` 与 reducer 已提交的 accepted transition，先执行 ProcessingContext 门禁再规范化为统一 `scene_event`；Scene Runtime 使用独立 consumer group 只消费该入口。条件为假或状态未变化且无 action 的求值不写 durable outcome；动作接纳以 PG Action Outbox commit 为界，`scene_state` 是异步 best-effort 恢复副本且不与 Outbox/input offset 跨存储原子。Action 与状态发布顺序由场景主规格总纲 §12.3 拥有，partition-local completion、连续 next-offset 与故障交接由[调度分章 §§5、8](6.规则引擎与场景联动/Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)拥有；
- Audit 消费 compact audit envelope、处理 outcome 与 accepted/domain transition 摘要，不作为完整点值 Canonical 的又一个读取副本；
- 客户专属 ProtocolApplication 可独立部署。达到独立扩缩、发布或故障隔离门槛时调整 composition host，不重写领域逻辑。

### 5.6 权威能力映射

| 能力 | domain owner | source package | runtime role | state SoT | checkpoint | composition host/component ID |
| --- | --- | --- | --- | --- | --- | --- |
| saas identity/authz | saas | `saas/service/{apisvr,syssvr}`、`saas/share/authz` | 请求授权/scope 编译 | PostgreSQL saas 表、tenant parent/closure | 请求事务；无数据流 checkpoint | `saas-api`、`saas-system`、`all-in-one` |
| things IoT control | things | `things/service/{apisvr,dmsvr}` | 控制面发布/查询 | PostgreSQL product/device/model/automation definition | RPC/事务、任务租约 | `things-api`、`things-device`、`all-in-one` |
| protocol decode | protocol | `protocol/{sdk,runtime,builtin,applications}` | ProtocolApplication | Kafka Raw + `ProtocolRouteCatalog`/release timeline；Canonical/quarantine/NO_OUTPUT 是处理结果 | outputs 与 Raw offsets 在同一 Kafka transaction 提交 | `protocol-app`、`all-in-one` |
| timeseries history+latest | things | `things/runtime/{timeseries,latest}` | Timeseries Materializer | TDengine history + latest store | 单 Materializer consumer group；完整 report 的全部模块 history 与 mode-required latest 完成后提交 Canonical offset | `timeseries-runtime`、`all-in-one` |
| presence | things | `things/runtime/presence` | Presence reducer/projection | session generation/sequence + current presence；EMQX snapshot 或 Gateway lease 负责可比性 | Lifecycle/snapshot reducer CAS 后发布 accepted transition | `access-adapter`、`gateway`、`things-device`、`all-in-one` |
| automation/alarm/action | things | `things/runtime/{automation,alarm}` | Automation/Alarm/Action | partition-local Actor 内存热状态 + 异步 compacted `scene_state` 恢复副本（含 timer business slots）；PG alarm 表、Action Outbox | 动作以场景主规格总纲 §12.3 的 PG Action Outbox commit 接纳；state 异步 best-effort 且不与 Outbox/input offset 跨存储原子；partition-local completion 与安全连续前沿见[调度分章 §5](6.规则引擎与场景联动/Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion) | `automation-runtime`、`all-in-one` |
| command | things | `things/runtime/command`；`things/service/dmsvr` 仅 facade | Command runtime/reply reducer | CommandExecution/Attempt/Outbox + reply reducer outcome | command outbox lease；reply state/outcome/accepted transition 同事务后推进连续 next-offset | `command-runtime`、`things-device`、`all-in-one` |
| audit | saas | `saas/service/syssvr` audit domain | compact audit envelope projection | PostgreSQL append-only audit | audit projection consumer 的连续 committed offset | `audit-runtime`、`saas-system`、`all-in-one` |

### 5.7 allInOne

`backend/cmd/allInOne` 只组合同一份 saas、things、protocol 与 root share 代码：

- 开发/演示可使用显式标注的 in-memory adapter；
- 正式小型私有化部署仍使用 Kafka Raw + Canonical、冻结的 consumer/checkpoint 语义和数据库；
- 同进程 service 使用 direct client，拆分时替换为 gRPC client；
- 本项目不承诺 fork goctl、生成 direct/gRPC 双实现或维护第二套业务代码。
## 6. Go Workspace 与依赖边界

项目采用自有 Go Workspace；目录借鉴“有边界 monorepo、接口先行、部署可变”的思想，但不是复制联犀目录：

```text
<repo>/
├── backend/
│   ├── go.work
│   ├── cmd/
│   │   └── allInOne/
│   ├── saas/
│   │   ├── cmd/
│   │   ├── service/
│   │   │   ├── apisvr/
│   │   │   └── syssvr/
│   │   └── share/
│   │       └── authz/
│   ├── things/
│   │   ├── cmd/
│   │   ├── service/
│   │   │   ├── apisvr/
│   │   │   └── dmsvr/
│   │   ├── runtime/
│   │   │   ├── timeseries/
│   │   │   ├── latest/
│   │   │   ├── presence/
│   │   │   ├── automation/
│   │   │   ├── alarm/
│   │   │   └── command/
│   │   └── share/
│   ├── protocol/
│   │   ├── sdk/
│   │   ├── runtime/
│   │   ├── builtin/
│   │   └── applications/
│   └── share/
│       ├── contracts/
│       └── foundation/
├── frontend/
└── deploy/
```

依赖规则：

- `saas` 拥有系统管理域逻辑和数据；`things` 拥有 IoT 域逻辑和数据；二者通过版本化 service contract 交互，禁止跨域直接读写表；
- `protocol` 只依赖 `protocol/sdk`、跨域契约和纯基础设施 port；厂商 codec 不得依赖 `things`、TDengine adapter 或任何 saas/things 领域表；
- workspace 根 `share/contracts` 只保存跨域、跨进程契约，`share/foundation` 只保存纯基础设施；saas/things 领域逻辑不得下沉到根 share；
- `saas/share/authz` 是 saas 域内授权实现，不等同于可被 things 绕过 `saas/service/syssvr` 直接调用的公共库；
- `things/runtime/*` 是可组合事件处理代码；`saas/cmd`、`things/cmd`、`protocol/applications` 和 `backend/cmd/allInOne` 是组合入口；即使同一 composition root 同时注入 codec 与 things runtime，也必须经过生产 Kafka Canonical 契约；
- 对外契约均有版本、兼容规则和契约测试；代码生成只用于 protobuf/OpenAPI 等成熟工具链，本项目不 fork goctl；
- service 的 direct client 与 gRPC client 实现同一接口，部署切换不得改变领域语义。

## 7. 消息与发布契约

### 7.1 Raw Record 与 Topic 路由契约

V1 不定义业务 `RawDeviceEnvelope`。设备与厂商在接入前约定平台 MQTT Topic，EMQX/Gateway 完成凭证—Topic ACL，Adapter 只做以下确定性投影：

```text
上行（设备发布）
直连设备
MQTT  iot/v1/up/<protocolKey>/d/<productKey>/<deviceKey>/<messageKind>
Kafka topic  iot.data.raw.v1.<protocolKey>
Kafka key    d/<productKey>/<deviceKey>
Kafka header message-kind=<messageKind>

网关子设备
MQTT  iot/v1/up/<protocolKey>/g/<gatewayProductKey>/<gatewayDeviceKey>/
      <childProductKey>/<childDeviceKey>/<messageKind>
Kafka topic  iot.data.raw.v1.<protocolKey>
Kafka key    g/<gatewayProductKey>/<gatewayDeviceKey>/
             <childProductKey>/<childDeviceKey>
Kafka header message-kind=<messageKind>

下行（设备订阅；CommandAttempt 冻结的目标 Topic 使用同一语法）
MQTT  iot/v1/down/<protocolKey>/d/<productKey>/<deviceKey>/<messageKind>
      iot/v1/down/<protocolKey>/g/<gatewayProductKey>/<gatewayDeviceKey>/
      <childProductKey>/<childDeviceKey>/<messageKind>
```

MQTT Topic 与 Kafka record key 都保持 product/device 两级独立 token；Adapter 做固定 level 提取和编码，不查询任何业务表。`messageKind` 是必需 Kafka header 而不是 key 的一部分，使同一设备的不同上行类型仍映射到同一 partition。ACL 的“凭证只能操作自己的 Topic”由 EMQX client attributes 提供两个独立占位符实现：设备认证 username 固定为 `<productKey>.<deviceKey>`（直连）/ `<gatewayProductKey>.<gatewayDeviceKey>`（网关），`mqtt.client_attrs_init` 从 username 机械拆出 `productKey`、`deviceKey` 两个属性，授权规则用 `${client_attrs.productKey}/${client_attrs.deviceKey}` 双占位符写成与设备数量无关的少量静态通配（见下文约束与 [授权与鉴权/mqtt-acl.md](./3.授权与鉴权/mqtt-acl.md)）。

`productKey` 全局唯一，`deviceKey` 在产品内唯一；两者一经进入 Raw 不得回收给另一业务身份，重命名/重建应产生新 key，并保留旧 identity history。`protocolKey` 和 `messageKind` 来自发布时的有限集合。所有 token 必须使用平台规定的安全字符集和长度，禁止空 token、`/`（MQTT/record-key 分隔符）、MQTT 通配符 `+`/`#`、控制字符或编码后二义性；`protocolKey` 还必须满足受控 Kafka topic token 规则。约束在产品/设备注册时强制校验，不做运行时清洗。MQTT Topic 中的 `v1` 是全平台入口语法版本，不是设备表字段，也不在消息体重复保存。`protocolKey` 是稳定逻辑路由，不随 Pod、Deployment、镜像或 codec release 改名。

一条 Durable Raw Record 由以下部分组成：

| 位置 | 内容 | 事实语义 |
| --- | --- | --- |
| Kafka topic | `iot.data.raw.v1.<protocolKey>` | 有界数量的协议路由；ProtocolApplication 消费一或少量明确 topic，不生成设备级 topic |
| Kafka key | 直连或网关/子设备的可信 product/device identity | 由受 ACL 保护的 MQTT Topic 机械生成；也是默认 partition key |
| Kafka value | 原始 MQTT payload bytes | 未解释的厂商报文；Adapter 不反序列化、不包业务 Envelope |
| Kafka headers | 必需 `message-kind`；可选 `mqtt-qos`、`mqtt-retain`、broker receive time、trace | 只保存路由必需或确有诊断价值的 transport metadata，不作为 owner/model/release 事实来源 |
| Kafka metadata | cluster ID、topic ID、partition、offset、LogAppendTime | `rawRef=(kafkaClusterId, rawTopicId, partition, offset)`，也是在线处理、归档与重放的 durable identity |

Kafka topic ID 在同一逻辑 topic 生命周期内稳定；删除重建产生新的 topic ID，不能把相同 topic name/partition/offset 误认为旧记录。平台 inventory 另外保存 topic name、topic ID、partition count、创建/退役时间与 release barrier。若把归档 Raw 导入 replay topic，新 topic/partition/offset 只表示导入位置，派生 Canonical 仍携带归档保存的 source `rawRef`。

因此 Raw 不携带 `schemaVersion`、`topicSchemaVersion`、`ingressProfileId`、`ingestEventId`、`tenantId`、`routeSnapshotId`、protocol/application/model release。Raw payload schema 由 `protocolKey` 对应的不可变协议发布解释；Canonical wire contract 才使用平台维护的 `contractVersion`。

关键约束：

- EMQX/Gateway 对设备凭证默认拒绝：认证 username 固定为 `<productKey>.<deviceKey>`（直连）/ `<gatewayProductKey>.<gatewayDeviceKey>`（网关），`mqtt.client_attrs_init` 从中拆出 `productKey/deviceKey` 两个 client attributes，授权规则用 `${client_attrs.productKey}/${client_attrs.deviceKey}` 双占位符写少量静态通配——直连设备只能 publish `iot/v1/up/+/d/${client_attrs.productKey}/${client_attrs.deviceKey}/+`、subscribe `iot/v1/down/+/d/${client_attrs.productKey}/${client_attrs.deviceKey}/+`；网关只能 publish `iot/v1/up/+/g/${client_attrs.productKey}/${client_attrs.deviceKey}/#`、subscribe `iot/v1/down/+/g/${client_attrs.productKey}/${client_attrs.deviceKey}/#`。授权源选型（file ACL 是否支持 `${client_attrs.*}`，否则用 Built-in DB authorizer）与完整规则见 [授权与鉴权/mqtt-acl.md](./3.授权与鉴权/mqtt-acl.md)。属性由认证身份机械拆出，直连设备无法冒用其他 product/device；但直连凭证可以在 gateway grammar 位置用自己的属性伪造 child，因此 ProtocolApplication 的 gateway-child 拓扑校验不可省略，child membership 在那里复核。Adapter 的信任前提是连接终止层已完成这层 ACL；Kafka 再限制 Adapter principal 只能写 Raw topic prefix、ProtocolApplication principal 只能读自己拥有的 protocol topics并写获准的 Canonical/quarantine/audit topics；
- Adapter 只执行语法校验、MQTT Topic → Kafka topic/key/header 机械映射、payload 上限检查和 Kafka publish，不查设备表、tenant、物模型或路由缓存。映射失败必须进入有界且 durable 的 ingress security/quarantine，不能猜测目标应用；
- Kafka producer idempotence 只吸收同一 producer session/epoch 内的协议重试。EMQX 普通共享订阅没有跨 Adapter 崩溃稳定的 source ID；若 Kafka commit 后、MQTT delivery ACK 前进程崩溃，重启后可能形成两个不同 `rawRef`。时间戳不能冒充唯一 ID，也不强制生成 `ingestEventId`；
- 设备 payload 中的 `sourceReportId/deviceSequence/deviceBootId/sourceEpoch/deviceTime` 由 ProtocolApplication 解码后使用。只有明确作用域内稳定的序号才能业务去重；没有稳定源标识时，平台只保证同一 `rawRef` 的 Kafka redelivery 不产生重复副作用，不承诺识别两次独立 append；
- `ownerTenantId`、canonical deviceId、不可变 `ownershipBindingId` 与 `modelReleaseId` 在 ProtocolApplication 阶段按可信 Kafka key identity 和本地发布缓存解析并写入 Canonical。每次设备 owner tenure 变更仍产生新的 ownership binding；project/area/group/tag 变化不生成 Canonical affiliation snapshot，也不按 Raw offset 冻结 Scene scope。网关仍必须验证 gateway—child 拓扑，不能只因 child key 出现在 record key 就信任它；
- Raw topic 冻结 `message.timestamp.type=LogAppendTime`；Kafka record timestamp 是可信平台 durable-ingress time。`processingMode=LIVE|BACKFILL` 由 ProtocolApplication 对每个 output 执行纯函数 `mode = F(messageKind, LogAppendTime, decoded sample/device time, immutable protocol/model release, processingModePolicyRelease)`；设备时间仍是独立语义，不能冒充 Kafka append time。`REPLAY` 只由 replay controller 赋值；
- `ProtocolRouteCatalog` 保证一个 `protocolKey` 只有一个 production owner/group；多个 Pod 作为该 group 成员共享 partitions。shadow/canary 使用独立 group，只能写隔离 namespace。控制面必须拒绝两个 production group/owner 对同一 route 都写正式 Canonical；
- 协议、物模型、ownership 或 processing-mode 策略在确需历史解释时，以 `(scope, rawTopicId, effectiveFromOffsets{partition -> nextOffset}, releaseId)` 记录生效边界。切换先排空并提交连续完成前沿，再发布新 release/owner；ProtocolApplication 按 `rawRef` 选择这些不可变 release 并写入 ProcessingProvenance。Affiliation 变化不进入该 offset vector，也不建立 Automation effective offset/barrier；若增加 partitions 会改变 key 映射，则创建新 Raw topic generation并执行显式迁移；
- 原版本 replay 通过 `rawRef + release timeline + ProcessingProvenance` 找回原 protocol/model/ownership/mode artifact；迁移重算显式指定目标 release 和独立 `replayRunId`。被 Raw retention/归档引用的 codec、模型字典、ownership/mode timeline 和必要镜像必须保留。Scene affiliation 不由历史 Raw artifact 复原，仍按处理时 current `SubjectContext`；
- Raw topic 使用 `cleanup.policy=delete`，生产 hot retention 固定为 72 小时。`retention.ms=72h`；`retention.bytes` 要么设为 `-1`，要么其每 partition 下限按“最热 partition 在已接纳容量 envelope 内的 72h 实测压缩写入 + segment/index + 重连/故障追赶安全余量”冻结，不得先于时间承诺触发删除。磁盘/lag 进入 70%/85% 只能触发 replay/backfill 限速、扩容或受控入口保护，不是缩短 72h 的豁免；超出已接纳 envelope 时宁可按入口 SLO 背压/拒绝新 append，也不静默删除未满 72h 的 Raw。核心不变量是 `T_hot >= T_detect + T_repair + T_catchup + T_safety`，且 consumer catch-up capacity 必须大于持续 ingress；
- 需要超过 72 小时的迟发现协议/模型纠错、审计或灾难恢复时，按合同启用加密对象归档 30/90 天。归档至少保存 source rawRef、topic name/ID、partition/offset、LogAppendTime、key、headers、payload、checksum、连续 offset-range manifest，以及引用的 release/cutover/artifact；V1 不把 Kafka tiered storage 插件能力当成默认交付；
- V1 按 topic count、partition count、单 partition append、整 topic throughput、producer batching/compression、consumer-group rebalance和 broker 故障追赶实测规划分片。设备 hash 已由 Kafka key→partition 映射完成，不再设计自有 `rawHashEpoch`；需要扩 partition 时使用新 topic generation；
- Raw 层默认使用集群存储加密、最小权限和访问审计。共享部署内需要更强逻辑隔离时使用专属 protocol route、Kafka topic/principal/quota/workload；要求 tenant 级物理/KMS 隔离时使用独立私有化平台部署，不让通用 Adapter 为每条消息查询 tenant，也不在同一部署内路由到多个 Data Kafka；
- 不把完整 payload、Topic、productKey、deviceKey 或 protocolKey 放入 Prometheus 普通标签。

### 7.2 CanonicalDataBatch 与标准事件

协议层产生有限的标准语义：`PropertyReport`、`DeviceEvent`、`ServiceReply/CommandReply`、`LifecycleEvent` 和 `ProtocolError/UnknownPoint`。一条 `RawRecord` 先在任何下游 side effect 前完成 bounded parse/validation，冻结确定性的逻辑 `DecodeManifest`：

```text
DecodeManifest（bounded in-memory decode result/completion set）
├─ rawRef / processingNamespace / semanticReleaseSetHash / outputCount / manifestHash
└─ outputs[]：outputIndex / outputKind / outputId
               └─ PropertyReport additionally：sourceReportId? / reportIndex / reportCount / reportId
```

`outputIndex` 在 `[0, outputCount)` 内稳定，覆盖 PropertyReport、DeviceEvent、CommandReply、LifecycleEvent 等全部输出类型。`semanticReleaseSetHash` 覆盖会改变解释结果的 `protocolReleaseId/modelReleaseId/processingModePolicyReleaseId/contractVersion`；兼容且语义不变的镜像滚动不能改变它。`manifestHash` 对 RawRef、processing namespace、semantic release set 与有序 output identity 集合取 hash；相同 Raw redelivery 必须重建相同 manifest。它不是 transport grouping，也不按点或模块拆分，更不要求单独 durable manifest stream/write。

PropertyReport 的数据流契约是：**一条 Raw 可产生 0..N 个逻辑 report，每个通过校验的 report 恰好形成一个完整 `CanonicalDataBatch`。普通在线上报一般 N=1；批量历史/离线补传必须按采样时刻拆为 N 个独立 report，不合并多个 `sampleTime`，也不按点或模块任意拆 transport 消息。** 一个 batch 可以包含多个 `moduleSpans`，Materializer 在内部按模块写多行；绝不把它退化为每点消息或每点对象。

```text
CanonicalDataBatch
├─ envelope（每 logical report 一次）
│  rawRef / processingNamespace / outputIndex / outputCount / outputKind / outputId / manifestHash
│  sourceReportId? / reportIndex / reportCount / reportId / contractVersion
│  processingMode = LIVE | BACKFILL | REPLAY
│  ownerTenantId / ownershipBindingId
│  productId+productKey / deviceId+deviceKey
│  gatewayDeviceId+gateway product/device key（仅网关子设备链路）
│  deviceTime / receivedAt / protocolKey / protocolReleaseId / modelReleaseId
│  ProcessingProvenance：sourceRawTopic / sourceRawKey / semanticReleaseSetHash
│                       processingModePolicyReleaseId
│                       protocolApplicationReleaseId / imageDigest / processedAt / causation
└─ PropertyReport body
   ├─ moduleSpans[]（少量；moduleSchemaSnapshotId 与各 plane/lane 起止范围）
   ├─ layout = DENSE_SCHEMA | SPARSE_TYPED
   ├─ state plane：stateCode = ABSENT | VALUE | NULL | RESET | INVALID
   │  + valuePresence + quality
   ├─ intOrdinals[]?    + intValues[]
   ├─ floatOrdinals[]?  + floatValues[]
   ├─ signalOrdinals[]? + signalBits/signalValues[]
   └─ textOrdinals[]?   + StringRef[] + TextArena

DeviceEvent / ServiceReply / CommandReply / LifecycleEvent / other low-rate standard output
├─ same common envelope：rawRef / processingNamespace / outputIndex / outputCount / outputKind / outputId
│                       manifestHash / processingMode / protocol+model+contract / ProcessingProvenance
└─ typed body
   ├─ LifecycleEvent：state + trusted sessionGeneration/lifecycleSequence（若可用）
   ├─ ServiceReply/CommandReply：correlationId + commandId/attemptNo + reply/result
   ├─ DeviceEvent：eventType + eventTime + source sequence（若协议提供）
   └─ ProtocolError/UnknownPoint：bounded reason + raw/point reference
```

Canonical 冻结设备 identity、ownership 与源处理 provenance，不冻结供 Scene matching 使用的 Product/Project/Area/group snapshot。Scene admission 产生的 SOURCE_DELTA 不携带 affiliation snapshot；source lane 在处理时读取并校验 current `SubjectContext`。因此设备迁区/换组后的 backlog/redelivery 按处理时已知的新 current affiliation 匹配，而不是按旧 payload 复活历史 scope。这是 V1 明确接受的语义。current context 不可用时整条 SOURCE_DELTA retry/fail closed，不能根据 Canonical payload 猜测关系。

低频标准 output 仍属于 DecodeManifest 完成集合，但“已 durable append”不等于“已接受状态迁移”。`LifecycleEvent` 由 presence reducer、`ServiceReply/CommandReply` 由 command reducer 各自提交状态与 reducer outcome，再发布带独立确定性 `outputId = H(sourceOutputId,reducerVersion,transitionType,subjectKey,transitionSequence)` 的 `AcceptedStateTransition`；事件继承 `processingNamespace/processingMode`，并以 `causationOutputId` 保留原 output 身份，不携带供 Scene scope matching 使用的历史 affiliation。Scene 只对 accepted transition 触发有状态联动；append-only `DeviceEvent` 可进入 Scene admission，诊断类 output 不进入业务联动。

`pointOrdinal` 由不可变 `ModuleSchemaSnapshot` 发布物冻结，作用域为 `(moduleSchemaSnapshotId, pointOrdinal)`；它不是进程本地 ID，也不能因重启、编译顺序或 cache 构建顺序改变。点名、类型、单位、TDengine 列计划和规则索引留在 snapshot dictionary 中，不随每个值重复传输。所有被 retention/replay 引用的 snapshot 与 ordinal dictionary 必须保留到引用结束。

state plane 与 value lane 的物理规则：

- `DENSE_SCHEMA` 按 snapshot ordinal 提供定长 state plane；`valuePresence=1` 的 ordinal 才按 schema ordinal 顺序进入对应 typed lane。无值状态不占 value slot；
- `SPARSE_TYPED` 使用唯一且按 ordinal 排序的 `stateOrdinals[] + stateCodes[]` 表达全部非 `ABSENT` 状态，并携带对齐的 valuePresence/quality；typed ordinals + values 只表达有值项，同一 ordinal 在对应 typed lane 中至多出现一次；
- `VALUE` 必须有可解析值；`INVALID` 可以有或没有可解析值，由 valuePresence 明示，不能根据 lane 是否缺值反推 state；
- `ABSENT` 默认表示“本报告未更新该点”。`NULL/RESET/INVALID` 对 history、latest 和 automation 的投影由 contract version 固定；消费者不得把任一状态简化成“缺值”；
- dense/sparse 阈值用真实 Modbus、100/500/1000 点报告在 Phase 0 实测，不凭经验写死。

输出、报告、接纳与幂等边界：

- 所有设备侧报告标识只叫 `sourceReportId`，它是未经平台命名空间化的源证据，绝不能直接充当 `reportId`。若协议契约证明它在 source epoch 内稳定可去重，则 `reportId = H(ownershipBindingId, canonicalDeviceId, sourceEpoch/deviceBootId, sourceReportId, reportIndex, processingNamespace)`；否则 `reportId = H(rawRef, reportIndex, processingNamespace)`。`ownershipBindingId` 在一次 owner tenure 内稳定、迁移 owner 时更换，防止同一 source ID 跨租户碰撞；source epoch 缺失、只提供会回绕序号或无法区分重启时，不得宣称跨连接/重启业务去重。fallback 只能收敛同一 RawRef 的 redelivery，不能吸收 Adapter 重复 append；
- `reportIndex` 是 manifest 中 PropertyReport 子集的稳定序号，范围为 `[0, reportCount)`。PropertyReport 的 `outputId = reportId`；其他标准输出使用 `outputId = H(rawRef, processingNamespace, outputKind, outputIndex)`。通用 output transport/idempotence identity 是 `(outputId, semanticReleaseSetHash)`；该 hash 已包含 contractVersion，显式迁移重算又使用不同 processing namespace，因此不会与原结果碰撞；
- `rawProcessingId = H(rawRef, processingNamespace, semanticReleaseSetHash)` 是 semantic binding 已解析后的 raw-level quarantine/`NO_OUTPUT` outcome key；若 identity 在解析 semantic release 前即可确定为非法、产品/设备确定不存在或 gateway-child 拓扑冲突，则使用 `preResolutionProcessingId = H(rawRef, processingNamespace, PRE_RESOLUTION)` 作为 quarantine key。它们用于诊断、幂等 sink 和 replay provenance，不再承担旧逐消息 ACK 模型下的两阶段预查询；
- catalog/cache 暂时不可用不是业务终态：ProtocolApplication 必须 pause 对应 partition、abort 当前 Kafka transaction并 backoff，不能写 permanent quarantine，也不能提交越过该 Raw 的更大 offset。主数据确定非法时，quarantine record 与该 partition 的 next-offset 在同一 transaction 提交；后续修复主数据只能使用新的 `replayRunId` 显式重算，不能改写原 quarantine；
- ModelRelease/contract 同时冻结单报告 `maxPoints/maxBytes` 与 Raw 聚合 `maxOutputsPerRaw/maxReportsPerRaw/maxDecodedPointsPerRaw/maxDecodedBytesPerRaw`，并冻结解压后字节、CPU 与 wall-time 预算。parser/BatchBuilder 必须在任何 publish、sink write 或 Automation side effect 前完成这些检查；聚合、解压或 CPU/时间预算超限立即停止，只在 Kafka transaction 中写一条以 `rawProcessingId` 幂等的有界 raw-level quarantine；所有预算检查完成前禁止发布任何 output。事务失败则 abort 并重试，不提交 Raw offset；
- 单 report 超过 `maxPoints/maxBytes` 时，只有在完整 Raw 仍能于全部聚合预算内有界完成 manifest 时才把该 output 标记为 per-output quarantine；若无法有界完成则升级为单条 raw-level quarantine。能够稳定确定 output 边界的单项语义错误按 `outputIndex/outputId` quarantine；整体 parse/manifest 确定性失败同样只写 raw-level quarantine；
- ProtocolApplication 先在有界内存中完整确定 manifest，再开启/使用 Kafka transaction：逐项写入完整 batch/event 或 per-output quarantine；`outputCount=0` 写唯一 `NO_OUTPUT`；raw-level failure 只写一条 raw quarantine；最后通过 `sendOffsetsToTransaction` 提交每个已连续完成 Raw partition 的 next-offset。任何 publish/offset/commit 失败都 abort；下游使用 `read_committed`，不读取 aborted/未提交 transaction 的 records。但 Kafka transaction 不是 transport assembly 边界：已提交的同一 transaction records 仍可能跨 partition、poll 和 group member 分批交付；每个 Canonical output 必须自完整，`outputCount/manifestHash` 用于证明/诊断最终完整性，consumer 不得假设一次 poll 拿到整个 manifest；
- manifest 本身不增加独立 durable write。Kafka transaction 原子消除“Canonical 已提交但 Raw offset 未提交”的在线窗口，但稳定 `outputId` 仍用于 replay、跨 transaction 恢复和 TDengine/PostgreSQL 等外部 sink 幂等。Scene 不维护协议拼接状态：Admission 消费完整 PropertyReport/append-only DeviceEvent 与 reducer accepted transition，通过 mode 门禁后写 `scene_event`，Runtime 只消费该统一入口。

`processingMode` 语义：

- 只有受信的 `processingNamespace=production && processingMode=LIVE` 可以进入生产 live Automation lane、改变 live scene state/timer 并产生正式 Alarm/Action Outbox/外部 action；场景按平台观测/处理时间和同一 `subjectKey` 的串行 lane 求值，不把 deviceTime/sampleTime 当成 watermark、timer deadline 或迟到修正依据；
- `BACKFILL` 的 PropertyReport 默认只写 history，latest 仅由冻结的 CAS total-order contract 决定是否更新；候选匹配必须位于 BACKFILL deterministic bypass 之后，不逐规则求值或生成逐规则 skip，也不创建、推进或修改 live state/timer/alarm/action；
- Raw/Canonical 的 `REPLAY` 必须执行 §12.3 的隔离 replay matrix。普通 broker redelivery 保留原 `processingMode`；只有受控 replay controller 能把派生标准输出标记为 `REPLAY`。“REPLAY/显式 backfill automation 只能写隔离 state/outbox”是对生产副作用的禁止和未来能力边界，不代表 Scene V1 已实现隔离 replay evaluator。Scene V1 不执行 REPLAY 场景规则；未来实现必须另立 ADR，采用独立 input/state/outbox namespace、dispatcher 禁止外发且结果不得晋升 production。

Scene admission 使用的公共 `SceneEvent` envelope 必须全链路传播必填 `ProcessingContext { processingNamespace, processingMode }` 与 `identityAlgorithmVersion=SCENE_ID_V2`。该 context 只能继承受信 Canonical/reducer/Flink 规范化输出或受信平台 schedule/subject-lifecycle producer；Admission 为 Canonical 派生事件盖章 Scene identity version，其他生产者和派生 handler 原样传播/校验。不得根据 Topic 名、业务时间、到达延迟、设备 payload 或 current `SubjectContext` 推断/提升 mode，缺失、未知或表外 namespace/mode 组合时 fail closed，禁止默认 LIVE。

契约与所有权硬约束：

- 热路径禁止 `map[string]any`、`[]any`、`[]string` 点名、JSON Canonical、protobuf `Any/Struct`、`repeated Point` 子消息、`[]*Point` 或任何逐点 heap object；
- codec 从 raw bytes 直接写平台 `BatchBuilder`/typed lanes/arena，禁止先构造通用对象树再二次转换；
- 进程内 pooled batch 与 Kafka Canonical record 的紧凑 wire bytes 是两种表示。分别用 `BatchLease` 与 `BufferLease` 明确所有权；enqueue 必须复制或原子转移 lease/refcount，禁止保留无所有权裸 slice；
- 异步 writer、Kafka publish future、retry queue 和所有 in-flight 引用全部释放后，lease 才能归还底层 pool。任何异步链路都必须等最后一个引用完成；`sync.Pool` 只做对象复用，不是容量、并发或队列控制，容量由有界队列/credit/semaphore 决定；
- history/latest/automation/audit 不得默认构造完整点对象。Materializer 以 lane/cursor 读取，automation 用 ordinal/bitmap 跳过无关点，audit 只读 envelope 与处理结果；
- Kafka consumer 必须把 Canonical value 直接解码进自己的预分配 arena/cursor；普通 protobuf `repeated Point` 与 JSON 不允许。protobuf packed scalar 只是候选 physical format，必须 benchmark 后冻结。

Canonical topic 的 hot retention 不得只写配置值：`T_canonical >= max_required_consumer(T_detect + T_repair + T_catchup) + T_safety`，且最低 24 小时。Materializer、Scene Admission、必要 reducer 和 audit projection 的 oldest-uncommitted age 接近实际 retention 70% 时告警，达到 85% 时停止非必要 replay/backfill、扩容或扩盘；future Flink 在正式接入前必须进入同一容量与恢复预算。

规划中的 `small-object-large-object/` pool、typed arena、decode 与 SQL builder 文档当前只是设计/实施计划，尚无 benchmark 或 pprof 结果；可以作为候选输入，不能把其中收益写成已实测结论。

### 7.3 不可变发布

```text
Model Draft
   └─ publish → ModuleSchemaSnapshot(s)
                  └─ ModelRelease
Protocol Draft
   └─ test vectors + validation → ProtocolRelease
Processing-mode Policy Draft
   └─ publish → ProcessingModePolicyRelease
Scene Binding Draft
   └─ validate/simulate → PG current SceneBinding (stable bindingId + configRevision)
Current SceneBinding change
   └─ local transaction + lifecycle Outbox → scene_binding_lifecycle
```

发布后：

- Protocol/Model/ProcessingModePolicy 等需要历史重解释的 artifact 发布后不可编辑；Scene Binding 则按主规格使用 PG current row、稳定 `bindingId` 与递增 `configRevision`，变更保留审计但不创建 data-plane immutable release；
- Product 的 `currentReleaseId` 只是新数据默认指针，不代表 backlog/replay 跟随最新版解释；每次激活必须追加不可变的 `RawOffsetBarrier=(scope, rawTopicId, partitionCount, effectiveFromOffsets{partition -> nextOffset}, releaseId)`；
- Device 可以有经过审计的稀疏 override，但同样以生效边界追加历史，不能覆盖旧 binding；
- Scene current Binding 在 owner tenant/project 下保存 area subtree/product/device 等符号 selector 与编译配置；同一 PG 事务更新 current row/project revision 并写 lifecycle Outbox，Runtime 通过 `scene_binding_lifecycle` 最终收敛 ProjectIndex。该流程没有 Canonical topic-partition 生效 vector、config/data offset barrier 或 `AutomationBindingSnapshot`。实际 transition/effect 记录稳定 `bindingId` 与当时 `configRevision/semanticHash`，纯 no-op 只记 bounded metric/trace；
- 设备迁区/换组更新 Things current relation/version，并通过业务事务 Outbox 产生 SubjectLifecycle；它不是不可变发布物，不生成 Raw/Canonical offset barrier，也不要求修改规则 binding；
- `ProtocolRelease` 只冻结 codec/mapping 语义制品；`ProtocolApplicationRelease` 冻结容器镜像 digest、runtime/SDK 版本和资源 profile；
- `ProcessingModePolicyRelease` 是产品/协议发布时由平台冻结的策略 artifact，不是设备表字段，也不要求管理员逐设备维护；默认策略可全平台复用，只有补传语义不同的产品才显式覆盖；
- `ProtocolCutover` 按 `protocolKey + rawRef boundary` 冻结协议语义 release；`ModelCutover` 与 `ProcessingModePolicyCutover` 按 product/device scope + rawRef boundary 分别冻结物模型和 LIVE/BACKFILL 判定策略；owner/ownership binding 迁移使用独立历史。Scene Binding 仅有 current row/revision 审计与 lifecycle 最终收敛，不加入 rawRef cutover history；affiliation current relation 也不聚合进这些 snapshot/cutover；
- `protocolKey` 不得复用或改成 Pod/release 名称。应用镜像升级、扩副本和同一逻辑 owner 的替换都保持 MQTT Topic 与 Kafka Raw topic generation不变。partition offset barrier 只区分平台收到 Raw 的日志位置，不能证明设备何时停止发送旧 wire payload；因此同一 key 上的新 decoder 必须在冻结窗口内向后兼容，或 payload 自带可验证版本。无法做到时必须发布新 protocolKey并迁移 MQTT Topic/ACL，不能用 Kafka offset 冒充协议协商；
- 一个共享 protocolKey 中只有部分产品要迁移到独立 ProtocolApplication 时，需要迁移这些设备的 Topic/ACL 到新 key；这是取消动态逐消息路由后明确接受的控制面成本，不能假装平台还能透明拆分；
- 发布事件驱动缓存刷新，不在消息到达时查草稿表；
- 回滚追加新的 `RawOffsetBarrier` 并重新指向仍保留的旧 artifact；任何回滚都不改写历史 barrier、ProcessingProvenance 或 release 内容。Raw 从 V1 起就是多 partition 日志，生效边界始终是 offset vector，禁止退化成一个裸 offset。

## 8. 物模型运行时

### 8.1 分层缓存，避免设备 × 点位放大

```text
ProtocolRouteCache
protocolKey → Raw topic generation/production group/owner + partition barrier history

ProductBindingCache
productKey → productId/default owner + Model/ProcessingModePolicy/Ownership cutover history

DeviceBindingCache
(productKey, deviceKey) → deviceId/gateway topology + ownershipBindingId
                          + sparse owner/model/policy cutover history

ReleaseArtifactCache
modelReleaseId → 共享的 compiled schema/module routing/type validators
protocolReleaseId → 共享的 compiled codec/mapping
processingModePolicyReleaseId → 共享的 immutable LIVE/BACKFILL policy
protocolApplicationReleaseId → immutable image/runtime inventory
PG current SceneBinding `(tenantCode,projectId,bindingId,configRevision)` → ProjectIndex
```

这些缓存位于 ProtocolApplication/对应领域 runtime，不在 Adapter。百万设备最多保存百万个紧凑 identity/topology/ownership pointer；相同产品的设备共享 compiled release artifact，不为每个设备复制 500 点 schema。Scene 的 current `SubjectContext` 使用 partition-local 有界 active-subject cache，缺失时从 Things authoritative current store 懒加载；它不进入 ProtocolApplication 的历史 release cache。发布事件主动预热热点数据；Protocol cache/catalog 暂时不可用时 pause 对应 Kafka partition、abort transaction并 backoff，不提交 offset；确定不存在或 record key 与拓扑不一致时才在 Raw→Canonical transaction 中写有界 quarantine，禁止在逐消息路径同步调用关系库并制造请求风暴。

### 8.2 模块发布

产品模型由可复用模块组成。发布时冻结最终模块集合和模块 JSON 快照，运行时不回查可编辑分类、产品模块或点位草稿。这样可同时满足：

- ASN/ASG 等产品族复用公共模块；
- 历史 release 可精确重建；
- 修改一个模块只产生新模块快照；
- `ModuleSchemaSnapshot/schemaHash` 唯一映射一个 TDengine STABLE schema version，多个 ModelRelease 可以复用同一 snapshot，不能每发一个产品版本就复制 STABLE；
- snapshot 内为每个点冻结稳定 `pointOrdinal`，并同时生成 identifier 路由、typed lane 顺序、quality 语义、automation bitmap 与 `ModuleWritePlan`；
- `pointOrdinal` 的完整作用域是 `moduleSchemaSnapshotId`，snapshot 发布物必须保留到所有 retention/replay 引用结束，不得使用进程本地自增 ID 代替。

未知 identifier、类型不符或越界值不能在热路径自动 ALTER 表或创建点位；它们进入带原因、样本和 release 信息的 quarantine，经过人工确认再发布模型。

## 9. 协议子系统

### 9.1 协议不是规则

协议层负责：

- 从受 ACL 保护的 Kafka Raw topic/key/header 解析 protocol/product/device/gateway identity，校验设备与子设备拓扑，并按 rawRef/offset barrier history解析 owner、协议和物模型发布版本；
- 原始帧/厂商 payload → `CanonicalDataBatch`/低频标准事件；
- 平台 Command → 设备帧/topic；
- 与产品/设备/协议版本绑定；
- 确定性测试、兼容性测试、性能预算和发布回滚。

协议层不生成 TDengine SQL，上行后告警、Webhook、补数等副作用也不属于 codec；它只写入平台 BatchBuilder/标准事件，由 Timeseries Materializer 和 automation/action 处理。这样保留联犀所说“上行前/后、下行前/后”的语义区分，但不把四个阶段都塞进同一脚本 VM。

### 9.2 V1 扩展方式

- 平台内置、经过测试的 Go codec 随共享 Protocol Runtime 版本编译发布；
- 简单字段映射使用受限的声明式 mapping，不允许网络、文件和任意循环；
- CPU 密集型、客户专属或闭源 codec 使用独立 `ProtocolApplication` 镜像、版本、资源和副本，通过稳定 protocolKey 的 owner/cutover 控制灰度，不影响其他厂家；
- 所有 codec 必须从 raw bytes 直接写 BatchBuilder，并有 golden vectors、模糊测试、payload/CPU/内存上限和版本清单；
- codec 必须在下游 side effect 前稳定枚举所有输出类型并冻结 `DecodeManifest(outputCount, manifestHash, outputs[])`；其中 PropertyReport 子集再冻结 `reportCount/reportIndex`。普通在线帧通常输出一个 report，历史/离线批量帧按 `sampleTime` 输出多个独立 report；重试不得改变任何 output/report 序号或 identity；
- codec 解析出的报告标识只能写入 `sourceReportId`。平台按 §7.2 的 ownership binding/canonical device/source-epoch/processing namespace 生成 `reportId`；codec 不得把 payload 值直接提升为全局 ID。只有 source epoch、回绕与重启语义已由协议契约证明时才能声明跨连接源去重；
- ModelRelease 与 codec release 必须声明单 report 的 `maxPoints/maxBytes`，并声明 `maxOutputsPerRaw/maxReportsPerRaw/maxDecodedPointsPerRaw/maxDecodedBytesPerRaw`、解压后字节及 CPU/时间预算；发布测试和运行时都执行这些上限。parser/BatchBuilder 完成全部 bounded parse/validation 后才允许发布输出，aggregate failure 只在 Kafka transaction 中写一条以 `rawProcessingId` 唯一的 bounded quarantine并提交对应 Raw next-offset；失败则 abort，不得靠拆 transport 单元或先发布部分结果绕过上限；
- V1 不在共享进程加载 Yaegi/Goja、Go `.so` 或租户任意插件；
- 真实需求成熟后再评估 WASM/进程外 sandbox，而不是先做协议市场。

## 10. 时序物化与 TDengine

### 10.1 存储模型

关系库保存设备、产品、模型、权限、命令、告警等主数据；TDengine 只是历史遥测执行层。

受控工业物模型采用：

```text
一个 ModuleSchemaSnapshot/schemaHash → 一张版本化 STABLE
一个 device × module → 一张 subtable
同一采样时间的多个点位 → 一行多列
```

ModelRelease 只引用一组 snapshot；模块间可按保留期、采集频率、共享程度和查询模式拆分。事件、动作定义、无需历史的属性不进入 property STABLE。跨版本查询由 `moduleLogicalId → snapshot/STABLE version` 目录生成受限 UNION；只有当没有 ModelRelease、数据保留或重放批次引用时才能 GC schema。极少量无法进入稳定宽表的长尾值可进入带类型列的窄表，但不能把全平台退化成 device × property 窄表。

### 10.2 写入路径

- `ProtocolApplication` 只输出带 `rawRef`、`ProcessingProvenance`、protocol/model release、通用 output identity、PropertyReport identity、可信 `processingMode`、moduleSpans、state plane 与 typed lanes 的完整 `CanonicalDataBatch`，不得生成 TDengine SQL、参数数组或逐点对象；
- Timeseries Materializer 以 `(modelReleaseId, moduleSchemaSnapshotId)` 取得预编译 `ModuleWritePlan`，一次 decode 后按 state/valuePresence/quality、moduleSpans 和 typed lanes 同时驱动 history/latest；不得中转 `map[string]any`、`[]any` 或 `[]Point`；
- Materializer 同时物化 TDengine history 和 latest。遥测与遥信只是 schema 分类；`NULL/RESET/INVALID` 的两类投影严格执行 contract version，不能从 lane 缺值推断；
- writer 按 STABLE/vgroup/批大小组织写入。SQL text builder 与 Stmt/column bind 都只是 adapter 候选，必须在同一 `ModuleWritePlan` 输入下端到端 benchmark；
- 模型发布时预建或验证 STABLE，新设备注册/首次上线时异步准备 subtable，热写路径不自动 ALTER；
- Materializer 只有一个逻辑 consumer group/checkpoint：一个完整 Canonical batch 只 decode 一次，按其中多个 `moduleSpans` 写入对应模块行；该报告所有模块的必需 history 写入（以及该 mode 需要的 latest 决策/更新）都成功后，per-partition completion sequencer 才允许把连续完成前沿提交为 next-offset。Scene Admission、Scene Runtime 与 compact Audit projection 使用各自 group/入口，不属于 Materializer；
- Raw 在线处理以 `rawRef` 标识输入；通用标准输出 transport/checkpoint 以 `(outputId, semanticReleaseSetHash)` 幂等，PropertyReport 使用对应 `reportId`。TD 模块宽行按 `(reportId, moduleSchemaSnapshotId, sampleTime)` 幂等；同一历史时间戳遇到不同值时写 durable conflict side record，不把整份 Canonical report 改判 admission quarantine，也不追溯撤销已接受的 live report；
- enqueue 到异步 TD/latest writer 必须复制数据或原子转移 `BatchLease`；retry、driver callback、in-flight write 等全部释放后才能归还。pool 不承担 backpressure；
- TDengine 不可用时 pause Materializer 已分配 partitions 或降低 credit，让 Durable Canonical topic 承接积压，不能无限堆内存；ProtocolApplication 是否继续 Raw→Canonical 由 Canonical retention/disk headroom 决定，而不是被 TDengine 同步阻塞。

### 10.3 history 与 latest 状态分开，物化边界统一

- TDengine 是历史数据的事实来源；
- Latest State 是派生投影，可使用 Redis、专用关系表或 TDengine latest 能力实现；它与 history 有独立状态/幂等逻辑，但都由 Timeseries Materializer 读取同一紧凑批次；
- 只在 Canonical hot retention、Raw 72 小时重生成窗口、对象归档或历史数据足以覆盖时，才可宣称 latest 可完整重建；
- 多节点/高读场景再把 Redis 设为必选依赖，小型部署不因“架构完整”强制引入；
- Latest State 使用由 contract version 冻结的 total-order：`(effectiveTime, sourceEpoch, deviceSequence, receivedAt, reportId)`；`effectiveTime` 只在设备时间通过时钟质量校验时取 deviceTime，否则取 receivedAt。latest 以完整 tuple 做 CAS；BACKFILL 也只可按这套 CAS 决定是否更新，不因“补数”覆盖更新值。该 comparator 只属于 latest 投影，不作为 Automation 的事件时间或排序状态；
- Automation 只允许受信的 `processingNamespace=production && processingMode=LIVE` 改变 live scene state。经 Admission 规范化的 `SOURCE_DELTA`（含 accepted transition 来源）与 source-side SubjectLifecycle 先按 `(tenantCode, sourceSubjectKey)` 进入 source lane，Manager 的 matching、SubjectContext/version 与 fanout 在该 lane 内按输入顺序执行；same-subject bundle 可在当前同 key lane 继续，cross-subject target 以及 Linked、Schedule、LifecycleTarget、timer fire 等终端输入再按[调度分章 §5](6.规则引擎与场景联动/Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)进入对应 evaluation-subject lane。持续时间、去抖、冷却以 runtime 首次接受状态迁移时冻结的平台观测/处理时间计时，deviceTime/sampleTime 只作为业务字段和审计信息，不驱动 watermark、迟到修正或 timer deadline。`BACKFILL` 与 `REPLAY` 不推进、不创建也不修改 live state；Scene V1 不提供 replay/backfill evaluator；
- 仪表盘和自动化都不得在逐次判断时扫描 history。当前状态前置条件应读取版本化 Latest/紧凑 Current Fact，并声明 freshness 与投影滞后语义；同一 batch 已携带的值优先使用 batch 值。

### 10.4 多租户存储边界

V1 默认共享 TDengine database，但不能依赖调用方自觉过滤：

- STABLE/subtable 使用平台生成的 tenant-aware 名称，`ownerTenantId` 是不可省略的 tag，始终记录设备直接 owner；任何租户输入都不能直接拼接成表名；
- API 查询只能经过携带 `PrincipalContext + TenantScope` 的 telemetry repository，自动注入 scope predicate并做跨租户负向测试；大 scope 使用预编译 handle/join/filter，不在热查询展开巨型 `IN (...)`；
- ProtocolApplication 从可信 Kafka Raw topic/key 与 ownership/model cutover history 解析直接 owner，并把结果写入 `ProcessingProvenance`；Materializer/runtime 只接受获授权 ProtocolApplication 产生的 Canonical，不携带登录 subject，也不逐消息调用 saas 或刷新到当前 release；
- `owner tenant → database/quota` 与 `protocolKey → Raw topic/consumer group/workload` 映射由控制面维护。多数租户可共享逻辑 protocol route；大租户可在共享部署内分配专属 protocolKey、Kafka topic/principal/quota/workload 和 database/vgroup。要求 tenant 级物理/KMS/故障域隔离时交付独立私有化平台部署；V1 不由一个控制面按 tenant 选择多个 Data Kafka。已上线产品从共享 key 拆出时需要受控迁移 MQTT Topic/ACL，不能由 Adapter 透明改路由；
- 查询并发、扫描时间范围、返回点数、写入速率和保留量都执行租户配额。

## 11. 自动化、告警、通知与命令

### 11.1 V1 Automation Runtime

Scene Runtime 的规范性架构决策源由两章共同组成：[场景联动 Runtime 主规格总纲](6.规则引擎与场景联动/场景联动%20Runtime%20主规格总纲.md)拥有产品边界、wire、Binding/状态业务语义、求值、Action Outbox 事务、route/hash、容量与总体取舍；[Scene Runtime 调度、分片与故障转移](6.规则引擎与场景联动/Scene%20Runtime%20调度、分片与故障转移.md#runtime-scope)拥有 partition ownership、Manager/Actor 执行、completion、restore、rebalance 与 Zombie 协议。本节只保留平台级产品边界；两章无“后写覆盖”关系，冲突必须在同一次变更中联合修订。

Scene 生产数据面的先决门禁固定为 `processingNamespace=production && processingMode=LIVE`。Admission 必须在 existence/relevance、tenant/fact interest filter 和 candidate matching 之前验证受信 ProcessingContext；只有该组合可写生产 `scene_event`。`production + BACKFILL`（在线补传/历史 output）与 `backfill/<jobId> + BACKFILL`（显式 job）均在 matching 前 deterministic bypass 且不产生 Scene record，`replay/<id> + REPLAY`、shadow 及缺失/未知/表外 mismatch/来源不可信的 context 均不进入生产 Scene；后者 quarantine/fail closed，不得默认 LIVE。公共 `SceneEvent.processingContext` 必须被 SOURCE、Linked/local/remote bundle、Schedule、SubjectLifecycle、LifecycleTarget 与 timer fire 原样传播，Actor 在 lookup/evaluation 前再次校验。Admission、Scheduler、Things SubjectLifecycle relay 与 Scene Runtime 使用独立最小权限 producer principal 和封闭 publish API；role→kind/context 由 producer application contract 强制，Runtime 二次验证全局 kind/payload/key/route/context/version，但不从普通 record/header 认证 producer role。

生产 `scene_state` 非 tombstone value 只能由 Scene Actor/`state-writer` principal 写入，并必须保存 production+LIVE+`identityAlgorithmVersion=SCENE_ID_V2`；state restore 与 timer callback 对缺失或非法 context/version fail closed，不加载、不注册 timer、不求值。PG 只有 Scene Actor DB role 可 INSERT Action Outbox，dispatcher role 只能读取并推进状态；dispatcher 只外发 production+LIVE 且 identity version 允许的 row。非法 pending row 原子转终态 `BLOCKED_CONTEXT`，不重试、不外发；strict target lane 阻塞后继，禁止越过。Replay/shadow/backfill principal 与 namespace 无权写生产 `scene_event/scene_state` 或 Action Outbox。

历史 event/state/Outbox 不得由 Runtime 默认为 LIVE。本次把 namespace 纳入 Scene identity，必须执行一次性 `SCENE_ID_V2` maintenance cutover：先 pause/fence Admission、Scheduler、Things SubjectLifecycle 与 Runtime 旧 producer；各 producer 排空已捕获 input frontier/publish callback 并停止后，捕获最终生产 `scene_event` end offsets；旧 Runtime 再排空这些 offsets、timer callback、async state send 与 PG/Kafka in-flight，证明 committed next-offset 到达最终 end；随后 pause/fence 旧 Action dispatcher 新 lease/dequeue，排空已领取外发/status/PG in-flight并捕获稳定 pending row 集合，最后撤销旧 producer/dispatcher credential，之后才启用新门禁。无法排空的 context-less event 只隔离并绑定最终损失审批，不求值；有 production-live 证据但含旧 timer/pending-trigger identity 的 state 也必须 tombstone+类型化 cold start。只有无旧 identity-bearing 字段的可信 state 可迁到 `SCENE_ID_V2`。可信 legacy pending Outbox 保存 `LEGACY_NO_NAMESPACE_V1` 且保留原 `effectId/payload/target/due` 只投递、不重算；无法证明的 row 转 `BLOCKED_CONTEXT`。首条 V2 record 被接受后只允许 V2-compatible binary 回滚或停写向前修复，不得恢复 legacy event/state/timer writer。该一次性 deployment frontier 不是 Scene config/affiliation barrier。

逻辑模型：

```text
Trigger（一个紧凑批次、计算指标或低频标准事件）
  + Stateless Conditions（字段比较、布尔组合、变化、阈值、当前状态前置条件）
  + Bounded Stateful Operators（持续时间、去抖、冷却、迟滞、边沿）
  + Actions（AlarmSignal / Webhook / DeviceCommand / EventPublish / 可靠延时）
```

V1 的有状态算子必须具有固定大小状态，不保存原始窗口样本。发布时校验每规则条件/动作/有状态算子数、最大持续时间/延时与 `staleAfter`；运行时限制每租户 active state、可恢复 timer business slots、candidate/evaluation/state-transition/action rate。`N 次/T` 留到下一阶段；SUM/AVG/MIN/MAX/COUNT 窗口属于共享计算指标层。V1 不支持任意脚本、跨设备 JOIN、滑动/会话窗口、复杂事件时间和任意 DAG。

Scene 配置使用 PG current Binding：`bindingId` 稳定，`configRevision` 递增，`semanticHash` 决定状态是否兼容。大量规则在共享 worker 中执行；运行时先按主规格冻结的 current Scope 与 ProjectIndex 做 candidate，再与精确 fact trigger posting 求交，不得为判断构造整批点对象或扫描全部规则。规则不是 Pod，规则级扩容是 Kafka partition、consumer、routing bucket 和 worker pool 扩容。

定义作用范围与状态关联键必须分离：区域模板可以只发布一份定义，但“区域内每台设备持续超温”仍按 device 分别保存状态；只有上游已经产生的“区域平均温度”计算事实才以 area 为 subject。`scene_state` record key 固定为 `(tenantCode,bindingId,evaluationSubjectKey)`，`operatorId` 只是完整 state value 内的有界 slot 身份，不建每 operator 的 PG durable row。state metadata 保存 `processingNamespace/processingMode/identityAlgorithmVersion/configRevision/semanticHash/lastSubjectAggregateVersion`；兼容变更可 preserve，不兼容变更只 close+tombstone 并标记 next-live bootstrap，不建 release-keyed state、TTL/GC 迁移或 `KEEP/MIGRATE/DRAIN` data barrier。

Scene identity 只使用主规格 §10–§12 冻结的 `logicalTimerId/timerFireId`、`scheduleOccurrenceId/scheduleEvaluationId`、`lifecycleEvaluationId`、`transitionId/effectId`。所有新 hash 首项固定为 `identityAlgorithmVersion=SCENE_ID_V2`，其余共同不变量是 `processingNamespace + tenantCode + stable bindingId + evaluationSubjectKey + logical trigger`；`configRevision` 和任何 `bindingReleaseId` 不进入 state key 或幂等身份。timer business slot 在完整 `scene_state` 内保存 `processingNamespace/processingMode/identityAlgorithmVersion/armedByTriggerId/armedAt/dueAt`，handle 仅驻内存；这不是 PG timer row。`LEGACY_NO_NAMESPACE_V1` 只标记 cutover 前已接受、保留原 `effectId` 的 pending Outbox，Runtime 不以它新建 state/timer/effect。

Scene admission 读取完整 `CanonicalDataBatch`、append-only `DeviceEvent`、共享计算层产生的 `ComputedMetricUpdated`，或领域 reducer 已提交的 `AcceptedStateTransition`，先校验它们的受信 `processingNamespace/processingMode`，再将 production+LIVE 事实规范化为统一 `scene_event`。输入保留稳定 `outputId/processingNamespace/processingMode/routingProvenanceId`，但 SOURCE_DELTA 不冻结 affiliation。source lane 在 mode gate 通过后才解析 current `SubjectContext`，取得当前 tenant/project/area/group 与单调 `aggregateVersion`；context 不可用时整条输入 retry/fail closed，不能从旧 payload 猜 scope，也不能用 SubjectContext 改写 mode。`BACKFILL` 在 existence/relevance 和 candidate matching 前跳过；Scene V1 不以 Canonical 历史 replay 重建 LIVE state，也没有隔离 replay evaluator。

设备 affiliation 变更由 Things 在同一业务事务中更新 current relation/version 并写 Outbox，再产生 SubjectLifecycle。Scene 根据 old/new current context 计算完整 local/target operation；ProjectIndex 或 context 不完整时不发送部分结果。它没有 Raw/Canonical effective offset 或 activation barrier，跨 target 也不伪装成原子切换。

`scene_binding_lifecycle` 与 explicit reinit 只是 no-state-value control：可安装配置、close/reset 内存、发 tombstone 并标记 next-live bootstrap；close/reset 必须取消已有 timer slot/handle，但不得读 current facts、构造完整 business state、立即执行 predicate/FSM、创建/arm/推进/fire timer，或产生 Alarm/Action Outbox。下一条通过 production+LIVE 门禁的 `SceneEvent` 才能懒 bootstrap 并求值。

因此迁区/换组后的 backlog 与 redelivery 统一按处理时已知的 current affiliation 匹配。延迟 Linked、恢复 state、INIT/DELETE 与 timer fire 必须用 member/state 中的 `aggregateVersion` 和 current-scope guard 重验；更高版本 REMOVE 或 current context 已不命中时，旧消息不能复活成员。失去 scope 时关闭相关 state/timer并异步写 tombstone；已经 commit 的 reliable delayed action 默认继续，除非业务显式产生补偿 transition，不靠历史 affiliation snapshot 回滚。

持续时间等 FSM 按状态迁移写入：

```text
false → true   : 在 tentative SceneBindingState 内更新 predicate/timer slot
true  → true   : 无状态变化且无 action，不写 PG
true  → false  : 在 tentative state 内取消 slot 或递增 generation，使旧 callback 失效
timer due      : 回到相同 evaluation-subject lane，复核 ProcessingContext、current Binding、
                 timer slot/generation、predicate/freshness，再形成 transition/Action Outbox
```

普通 duration/debounce/cooldown/Alarm timer 的 handle 只在进程内，可恢复的 timing business state 随完整 `scene_state.timerState[]` 异步保存；不建 PG timer 表、领取 worker 或 `FOR UPDATE SKIP LOCKED` 事实来源。设备停止上报时“保持最后状态、`staleAfter` 取消或 offline 立即取消”必须成为 current Binding 的显式语义。重复 true 不为刷新 freshness 改写 scene state：任一 scene timer 到期时根据 Latest/Current Fact 的 `receivedAt` 复核；若产品承诺在 `staleAfter` 时刻主动取消，则 `staleAfter` 只能选有限策略桶，由 Latest/Presence 层为 `(subject,fact,policyBucket)` 共享生成一次确定性 `FactExpired/AcceptedStateTransition`，不能为每条规则滚动一只 freshness timer。没有共享 expiry signal 的配置只能承诺“下次输入或已有 timer 到期时发现过期”。duration 与 reliable delayed action 不能混为一种 timer：duration 在条件变 false 时取消并在到期时复核；已接受的 delayed action 只使用带 `available_at/due_at` 的 PG Action Outbox，默认 `CONTINUE_AFTER_ACCEPT`，如需撤销必须生成显式补偿动作。

Automation 不为条件为假、重复 true、冷却中等无状态变化/无 action 的求值写 durable outcome；这些求值允许在 redelivery 时安全重算。会产生动作时，顺序固定为：在副本中计算并规范序列化 tentative state，校验配额，在 PG 本地事务中仅插入 Action Outbox rows 并 commit，再发布 Actor 内存 state/注册或取消内存 timer，最后异步 best-effort 发送完整 `scene_state`。`AlarmSignal/ActionIntent` 在此边界只是 Action Outbox row 的动作 kind/payload，不是另一组 Scene 状态表。state/timer 不在 PG 事务中，`scene_state` 也不与 Outbox 或 input offset 原子提交；所有副作用统一以 `effectId` 唯一键收敛。

产品级成功边界固定为：动作只有 PG Action Outbox commit 后才算被平台接纳；input completion 是 partition-local 的处理边界，不表示 Webhook、通知或 DeviceCommand 已由外部系统执行；`scene_state` 只是异步 best-effort 恢复副本，state、Outbox 与 input offset 不构成跨存储原子事务。完整 completion 谓词、per-partition sequencer 与 safe contiguous next-offset frontier 只见[调度分章 §5](6.规则引擎与场景联动/Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)，本总体架构不复制该算法。确定性编译错误必须在发布前拒绝；运行时预算或表达式异常写有界规则错误并触发隔离/熔断，不能阻塞遥测历史落库。

### 11.2 何时引入成熟流引擎

V1 Automation 不因简单持续时间引入流引擎。出现固定窗口聚合、滑动/会话窗口、跨设备聚合、JOIN、watermark 或迟到修正时，引入共享 Computed Metric Runtime；云端候选为一个或少量通用 Flink Job，不得按规则创建 Job，也不得把全量设备级规则复制到 Broadcast State。

| 需求 | Bounded Automation | 共享 Flink Computed Metric | eKuiper |
| --- | --- | --- | --- |
| 单消息过滤、阈值、变化、边沿 | 适合 | 不需要 | 边缘可做 |
| 持续时间、去抖、冷却、迟滞、可靠延时 | 适合，固定大小 FSM | 不需要 | 边缘可做 |
| `N 次/T` | 后续以严格 N/T/状态字节上限实现 | 复杂语义可转入 | 可做部分 |
| 固定 TUMBLE SUM/AVG/MIN/MAX/COUNT | 只消费结果 | 适合 | 边缘可做 |
| 滑动/会话窗口、watermark、迟到修正 | 不做 | 适合 | 不作为云集群承诺 |
| 跨设备/区域聚合、JOIN、CEP | 不做 | 适合 | 不作为云集群承诺 |
| 通用 savepoint、状态迁移、弹性扩缩 | 仅应用级 FSM 兼容/GC | 适合 | 不作为承诺 |

控制面将窗口表达式规范化为共享 `ComputationPlan`，fingerprint 必须覆盖所有会改变结果的语义：

```text
(source selector/filter,
 source schema/model release + field + unit/normalization,
 keyBy,
 function + parameters,
 window type/size/slide/alignment/timezone,
 timestamp source + time semantics + allowed lateness,
 NULL/INVALID/quality + dedupe/retraction policy,
 emission/finality policy)
```

阈值、告警动作和消费该指标的场景归属不进入 fingerprint；输入 selector/filter 与聚合 group scope 必须进入，不能用含糊的 `source` 合并。多个场景引用相同 fingerprint 时共享一份 plan 和 keyed window state；控制面维护引用计数、版本 barrier 与安全 GC。Flink 状态按 `(planId,subjectKey[,window])` 分区，动态配置采用 co-partitioned/config lookup，不广播几十万设备规则。Flink 提供 keyed state/timer/checkpoint 底座，但不会自动识别并合并语义相同的场景窗口。

Flink 输出版本化 `ComputedMetricUpdated(outputId,processingNamespace,processingMode,ownerTenantId,projectId,routingProvenanceId,planId,planReleaseId,subjectType,subjectKey,windowStart,windowEnd,value,quality,revision,finality,provenance)`；其中 `outputId = H(ownerTenantId,processingNamespace,planReleaseId,subjectKey,windowStart,windowEnd,revision,finality)`，同一 revision/finality 的恢复重算必须保持不变；`routingProvenanceId` 指向 plan 使用的不可变 selector/aggregation-scope release，消费时不得重新查询当前 area/group 成员。production computed-metric Job 只消费 `LIVE` 并输出 `LIVE`；BACKFILL/REPLAY 使用隔离 input、state、topic 与 namespace，输出保留对应 mode，绝不能混入 live Automation。Automation 将该事件视为普通事实，以 `outputId` 作为 `triggerId`，只执行阈值判断或 bounded FSM，不保存窗口样本。会产生外部副作用的场景默认只消费 final metric；迟到 revision 不自动撤销已经执行的命令/Webhook，除非另行定义补偿状态机。eKuiper 继续只作为边缘候选。接受 JVM 时 Kafka Streams 是中心 bounded-state runtime 的现成候选，但动态规则索引、配置共分区和 durable one-shot timer 仍需开发；Go Scene V1 使用 partition-local Actor 内存状态/进程内 timer、异步 best-effort `scene_state` 与 PG Action Outbox，不把 PostgreSQL 或 Redis 当作高频 FSM state/timer 事实来源。

### 11.3 告警

`things/runtime/alarm` 是告警状态唯一写入 package，PostgreSQL alarm 表是事实来源；`things/service/dmsvr` 只是 API/RPC facade。生命周期与确认状态正交：

```text
lifecycle:   ACTIVE → CLEARED → CLOSED
acknowledge: UNACKNOWLEDGED → ACKNOWLEDGED
```

使用 `(ownerTenantId, subject, alarmDefinitionId)` 作为 active alarm key，只对 `ACTIVE` episode 建唯一约束；`alarmDefinitionId` 是跨 Binding 配置变更稳定的告警定义身份，opening/current `bindingId/configRevision` 只是 episode 审计属性，不参与 active 唯一键或 Scene effect identity。恢复/关闭后再次触发创建新 `episodeId`。`AlarmSignal` 使用 §11.1 引用的主规格 `effectId` 幂等，其中 `actionId=alarmDefinitionId`、`targetKey=alarm subject`；它支持 severity、首次/最后触发、恢复、确认、抑制和审计。规则只产生 Signal，不能由多个 worker 各自保存“当前是否告警”。

Binding 配置变更可为受影响的 `alarmDefinitionId` 记录 `CONTINUE|RETIRE` 业务意图，但 `scene_binding_lifecycle`/显式 reinit 只做 close/reset/tombstone/next-live bootstrap，不能直接 clear/close Alarm，也没有 binding cutover barrier。`CONTINUE` 允许下一条合格 production+LIVE 数据面信号在语义兼容时沿用当前 episode；`RETIRE` 必须由独立、显式授权的 Alarm 补偿工作流生成新审计/effect identity，再以 `RULE_RETIRED` 按 `ACTIVE → CLEARED → CLOSED` 落库，不得伪装成 Binding lifecycle 的即时求值副作用。

### 11.4 通知与外部联动

V1 只做 Webhook，采用事务 Outbox 与 §11.1 统一 `effectId` 唯一键；`targetKey` 使用版本化 endpoint identity，并记录重试预算、超时、签名和投递轨迹。短信、邮件、企微、钉钉等以后通过类型化 provider 增加，不先建设全渠道通知中心。

### 11.5 设备命令

命令是持久状态机，而不是 MQTT publish 返回值：

```text
CREATED → QUEUED → ENCODED → ROUTED → DISPATCHED
        → TRANSPORT_ACCEPTED → DEVICE_DELIVERED
        → RESULT_SUCCEEDED / RESULT_FAILED
        └→ FAILED / EXPIRED / CANCELLED / OUTCOME_UNKNOWN
```

- `things/runtime/command` 是 `CommandExecution/CommandAttempt/Outbox` 状态唯一写入 package；`things/service/dmsvr` 只是 facade。execution 和 outbox 先落库，再异步路由；
- 场景产生的命令使用统一 `effectId` 作为创建 `CommandExecution` 的幂等键，`targetKey` 是目标设备；同一 evaluation 的多设备 fanout 分别生成不同 effect，重投不得重复创建 execution；
- presence 只用于优化路由（如在线时立即投递、离线时排队/转留言策略），不是命令事实来源。一期 EMQX 下 presence 由 snapshot reconciliation 裁决，可能短暂错误；这只影响投递时机与路径选择，不改变命令状态机的事实与终态判定——终态只由 CommandReply/超时/取消决定；
- `TRANSPORT_ACCEPTED` 只表示 EMQX/Gateway 接受投递；`DEVICE_DELIVERED` 只表示协议级设备确认；单向命令可按产品契约在此终态，双向命令必须等 Canonical `CommandReply` 才进入业务结果终态；
- 每次 `CommandAttempt` 固定 `commandId/correlationId/protocolKey/protocolReleaseId/modelReleaseId`、实际编码的 `protocolApplicationReleaseId`、目标 product/device、下行 MQTT Topic（语法见 §7.1 下行契约）、编码 payload hash、目标 adapter 和 attempt number；首次编码成功后保存不可变 Topic/payload，transport 重试直接复用，不跟随当前 release 或应用 owner 漂移；相关 codec/artifact 至少保留到 attempt 终结或进入人工处置；
- 上行 `CommandReply` 经 Kafka Raw → ProtocolApplication → Durable Canonical → Command reducer，以 correlation ID 找到 execution/attempt并幂等更新状态；无法关联的回复进入诊断流；
- 同一 protocolKey 升级时，新 decoder 必须能识别所有未终结旧 `CommandAttempt` 的迟到 reply。无法兼容时，发布流程必须先停止创建旧 release attempt，等待其完成/过期并保留旧 reply decoder 到最大回复窗口；仍不能建立无歧义边界时必须使用新 protocolKey 或自描述 wire version，不能仅按 Raw arrival sequence 选择新 decoder；
- 所有重试沿用相同逻辑 commandId/idempotencyKey；
- toggle、increment、重启等非幂等操作在模糊超时后进入 `OUTCOME_UNKNOWN`，不得盲目重发；
- 不使用临时订阅或进程内 channel 作为命令状态的事实来源。

## 12. 可靠性、顺序与背压

### 12.1 生产确认、事务与 offset 边界

| 事件/阶段 | 能承诺的成功语义 | 不能承诺什么 |
| --- | --- | --- |
| MQTT PUBACK | 接入端已按部署模式接管 QoS 1 消息；一期语义由 EMQX 决定 | Kafka Raw 已写入、TDengine 已写入、规则已执行 |
| Kafka Raw produce success | producer 使用 `acks=all`，当前 ISR 满足 `min.insync.replicas=2` 时该 record 已 committed | 所有配置副本均健康、协议已解释、业务副作用成功 |
| Protocol Raw→Canonical transaction commit | manifest 的全部 Canonical/quarantine/`NO_OUTPUT` 与所覆盖 Raw partitions 的连续 next-offset 已作为一个 transaction commit outcome；`read_committed` 不返回 aborted/未提交 records | 一次 poll/同一 consumer member 收到整个 manifest；Materializer、Scene Admission/Runtime、Reducer、Audit 或外部动作已完成 |
| Materializer Canonical offset commit | 完整 PropertyReport 的全部模块 history 与 mode-required latest 决策/更新已完成；冲突 side record 也属于明确 outcome | Scene Admission/Runtime 或 Audit 已完成；外部动作成功；conflict 等于整份 report quarantine |
| state reducer offset commit | Presence/Command reducer 已提交合法状态迁移或 durable stale/conflict outcome；被接受时 `AcceptedStateTransition` 已通过本地 outbox登记 | Automation 已处理 transition、外部动作已完成 |
| Scene input offset commit | 该 partition 已达到[调度分章 §5](6.规则引擎与场景联动/Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)定义的安全连续 next-offset；动作接纳边界仍是 PG Action Outbox commit，state 仍为异步 best-effort | 外部系统已经执行 Action；state/Outbox/input offset 处于一个原子事务；offset 前崩溃不会重算候选；异步 state 已 durable |
| Audit projection offset commit | compact audit envelope/outcome 已写入 append-only audit store | 其他 stage 或外部动作最终成功 |
| Command Broker ACK | Broker/连接层接收发布 | 设备已接收或执行命令 |

所有 Kafka consumer 关闭 auto commit。普通外部 sink consumer 只提交**每个 partition 连续完成前沿的 next-offset**；若同 partition 内并行处理，必须有 completion sequencer，前一 offset 未完成时不得越过它提交更大 offset。可重试错误 pause partition并 backoff；明确 poison record 只有在 durable quarantine/outcome 完成后才能推进。

ProtocolApplication 是纯 Kafka consume-transform-produce：bounded decode/manifest 完成后，在 transaction 中发布全部输出并调用 `sendOffsetsToTransaction(nextOffsets, groupMetadata)`；失败则 abort，下游固定 `isolation.level=read_committed`。`transactional.id` 必须按受控 workload slot 唯一且可 fencing，Kafka group generation/member epoch 与 transactional producer epoch共同拒绝 zombie consumer/producer。Kafka transaction 不能覆盖 TDengine、PostgreSQL、Webhook 或设备命令；这些 sink 继续依靠稳定 `outputId/reportId`、Scene 主规格 `effectId`、本地事务和 Outbox吸收“外部写成功、offset 尚未提交”的 redelivery。

Adapter 对 MQTT source acknowledgement 与 Kafka produce 不存在原子事务。若 Kafka 已 commit、Adapter 在确认 MQTT delivery 前崩溃，重启后可能再次 append 并形成另一个 `rawRef`；Kafka producer idempotence不能消除跨 producer incarnation 的这种重复。只有协议提供稳定 `sourceReportId + sourceEpoch/deviceBootId` 时才能业务去重，时间戳不能冒充唯一 ID。

### 12.2 Partition 顺序、发布 barrier 与 owner fencing

- 每个 `protocolKey` 使用一个受管 `iot.data.raw.v1.<protocolKey>` topic generation和一个稳定 production consumer group；多个 ProtocolApplication Pod 作为同组成员获得 partition assignment。副本数超过 partition 数不会增加消费并行度；禁止按设备或产品全集生成 topic/group；
- Kafka 只保证 partition log order。默认 record key 让同一可信设备 identity 映射到同一 partition，但平台不把到达顺序等同于 deviceTime 或业务总序；history conflict 由 TD sink 记录、latest 用冻结 comparator CAS、Automation 按 `LIVE subjectKey` 串行 processing order + state generation 收敛，各领域 reducer 使用各自契约；
- V1 decoder 只依赖当前 Raw、按 rawRef/barrier 解析的不可变 artifacts 和有界只读缓存。依赖跨帧进程状态或严格 decode 顺序的协议不能偷偷进入通用 group，必须改造成自包含 payload，或建立单独 ADR/压测验证的有状态 runtime；
- 所有 Canonical output 继承 `rawRef/processingNamespace/outputIndex/outputCount/outputKind/outputId/manifestHash/processingMode/ownerTenantId/ownershipBindingId`，PropertyReport 再继承 `sourceReportId/reportIndex/reportCount/reportId`；Canonical 不携带供 Scene scope matching 使用的 affiliation snapshot。异步 TD 写、Webhook 和规则扇出仍可能完成乱序，每个 sink 单独定义幂等和顺序；
- 低频标准输出必须按类型使用确定性 reducer：Lifecycle 按可信 `sessionGeneration+lifecycleSequence` CAS；CommandReply 按 `commandId+attemptNo` 执行单调状态迁移；DeviceEvent 默认 append-only；ProtocolError/UnknownPoint 只追加诊断/quarantine。EMQX 无可比 session sequence 时，presence 由 snapshot reconciliation 裁决；
- 只有新旧构建声明相同 `protocolReleaseId`、通过相同 golden vectors且 decode 语义兼容时，才可在同一 production group 滚动。改变解释语义必须发布新 ProtocolRelease和 offset barrier，不能只换 image；
- release/owner 切换使用 `RawOffsetBarrier={rawTopicId, partitionCount, effectiveFromOffsets{partition -> nextOffset}}`：控制面先标记 `DRAINING` 并暂停旧 owner 获取 barrier 之后的新工作，捕获各 partition end-offset 快照；旧 owner处理所有 `< barrier[p]` 的记录并恰好提交到 `barrier[p]`，终止旧 group members/transactions后递增 `routeFencingToken`；新 owner从同一 committed vector继续。Kafka group/transaction epoch负责 broker 内 offset/output fencing，短期 principal + `routeFencingToken` 负责控制面发布权限，任何 Kafka 外部 sink 仍需 lease/CAS拒绝旧 token；
- canary 使用独立 shadow group，读取同一 Raw 但只写隔离 Canonical/audit namespace并比较结果；不得与 production group 竞争 partitions，也不得写正式 history/latest/Automation；
- Raw topic partition count 在 generation 内冻结。原地增加 partition 会改变 key 映射且旧数据不会重分布；需要扩分区时创建新 topic generation、冻结新 barrier vector并显式迁移 Adapter/consumer，不引入自研 `rawHashEpoch/routeEpoch/streamShardId`。

### 12.3 至少一次、重放与归档

- Raw identity 是 `(kafkaClusterId, rawTopicId, partition, offset)`；标准输出 identity 是 `(outputId, semanticReleaseSetHash)`，PropertyReport 使用 `reportId`；Materializer 行以 `reportId+module+sampleTime` 幂等；Scene state 以 `(tenantCode,bindingId,evaluationSubjectKey)` 完整 value 和主规格 version/generation 收敛，Alarm/Action 以主规格 `effectId` 幂等；
- 下游代码/状态损坏但 Canonical 尚在 hot retention 内时，只可对已定义恢复契约的 Materializer/reducer/projection 使用独立 recovery group 重跑；Scene V1 明确排除在外，不从 Canonical 历史重建 Scene state 或动作。协议、物模型、ownership 或 processing-mode 需要重新解释时，才从 Raw/归档生成隔离 Canonical replay stream；
- `RESTORE_ORIGINAL` 按 source rawRef 和 offset barrier history找回原 protocol/model/mode/ownership artifact；`REPROCESS_MIGRATED` 显式指定目标 releases。二者都使用独立 `replayRunId`、consumer group、Canonical replay topic 与 history/latest 目标 namespace，禁止 reset 或 seek production group；该约束不宣称存在 Scene state/outbox replay namespace；
- 普通 Kafka redelivery 保留原 processing namespace 和 `LIVE|BACKFILL`；transaction abort 后相同 Raw 重新生成确定性的 manifest/output identity。显式 replay 才标记 `REPLAY`；raw-level/per-output异常继续写隔离 quarantine；
- Raw 对象归档至少保存 source cluster/topic name/topic ID/partition/offset、LogAppendTime、key、headers、payload、checksum和连续 offset-range manifest。导入 replay topic时保留 source rawRef，目标 topic/offset 只表示导入位置；protocol/model/mode/ownership barrier history、ProcessingProvenance、codec/model artifact保留期不得短于归档可重放窗口；
- Raw Kafka hot retention 生产基线为 72 小时；Canonical 至少 24 小时并按必要 consumer故障+追赶公式上调。可选 30/90 天 Raw 对象归档由客户合同、敏感数据和合规策略决定；Kafka 本身不是备份，Redis Pub/Sub 与进程内 EventBus 也不是重放源。

下表统一列出 redelivery 与受控恢复的副作用矩阵。`LIVE_REDELIVERY` 只适用于普通 Kafka redelivery并保留输入原有的 `LIVE` 或 `BACKFILL`；`processingMode=REPLAY` 只能选择 `RESTORE_ORIGINAL` 或 `REPROCESS_MIGRATED`。该矩阵中的 replay 能力仅属于 Raw/Canonical/Materializer 等已定义链路；Scene V1 没有 replay/backfill evaluator。

| replay mode | history/latest | alarm/automation state | EventPublish | Webhook/notification | DeviceCommand |
| --- | --- | --- | --- | --- | --- |
| `LIVE_REDELIVERY` | 原 namespace 幂等补齐；latest 按原总序 CAS | LIVE 重算候选；状态由 version/generation、动作由 Outbox 唯一键收敛；BACKFILL 不进入 live Automation | 仅 LIVE 允许原 outbox 正常投递；复用原 `effectId` | 同左，按正常重试预算投递 | 同左；非幂等模糊结果仍进入 `OUTCOME_UNKNOWN` |
| `RESTORE_ORIGINAL` | 使用原 protocol/model/owner binding，先写 restore namespace，验证后按该 sink 独立契约受控晋升 | Scene V1 不求值；zero Scene state/outbox | Scene V1 不生成 | Scene V1 不生成 | Scene V1 不生成 |
| `REPROCESS_MIGRATED` | 显式目标 protocol/model，写 migration namespace/目标表 | Scene V1 不求值；zero Scene state/outbox | Scene V1 不生成 | Scene V1 不生成 | Scene V1 不生成 |

Scene V1 不创建隔离 Action Outbox，因而不存在通过审批把 Scene replay 结果晋升投递的通道。未来如要实现 Scene REPLAY/backfill 求值，必须另立 ADR，使用独立 input/state/outbox namespace，dispatcher 永不外发，且结果不得晋升 production。

### 12.4 故障与背压测试

至少覆盖：

- TDengine 停止 30 分钟后，Materializer 内存有界、Canonical 积压未过 retention，并能在声明 RTO 内追平；实测 `consumerCapacity > ingressRate`，不能只看 lag 最终下降；
- 在 ProtocolApplication transaction 的 begin/publish outputs/`sendOffsetsToTransaction`/commit 请求前后逐点注入崩溃、超时和 broker 故障；`read_committed` 下不得读到 aborted/未提交 records，abort 后 Raw offset 不丢，commit 后全部 expected output identities 最终可读且 Raw offsets 同步推进；另外验证跨 poll/partition 分批交付，以及 commit 响应丢失后不产生第二组 committed outputs；
- pre-resolution quarantine、resolved raw quarantine、per-output quarantine 与 `NO_OUTPUT` 都和 Raw next-offset 原子提交；临时 cache failure 必须 pause/abort，不能越过；poison record只有 durable outcome 后才能继续；
- Adapter Kafka response丢失、进程崩溃和 MQTT delivery重投覆盖 producer-session内去重与跨 incarnation重复边界；无稳定 source ID 时明确允许两个 rawRef，但下游不得产生不可控副作用；
- production group rebalance、Pod滚动、缩容和 owner/release切换验证 per-partition连续 next-offset、barrier vector、group/transactional producer fencing、旧 token 外部 sink拒绝；shadow group只能写隔离 topics；
- partition count、key skew、最热 partition、producer batching/compression、RF=3复制、ISR shrink、一台 broker故障与副本追赶都达到四档容量目标；原地增 partition被控制面拒绝，topic generation迁移可回滚；
- Raw `retention.ms=72h` 与 `retention.bytes=-1`/冻结下限在已接纳的持续流量、补传 burst和故障追赶下都不提前淘汰；Canonical按冻结档位执行同类验证；70%/85% 保护必须在违约前触发限速/扩容/入口保护，告警不得成为提前删除的允许；
- 从 48 小时前 Raw 样本执行 original replay得到相同 Canonical hash；migration replay使用目标 releases并保留 source rawRef/provenance；对象归档随机恢复一个连续 offset range，manifest无 gap且checksum全部通过；
- Data/Domain 同集群下同时注入设备双峰值、限速 replay和 broker故障，验证 principal/client quota后领域事件 p99/lag仍达标；否则触发物理拆分 ADR；
- 低频输出乱序至少覆盖旧 session OFFLINE晚于新 session ONLINE、EMQX event与snapshot冲突、CommandReply终态后的迟到中间态/冲突终态、DeviceEvent重投；
- 重复、乱序、同时间戳和迟到消息分别验证 history conflict 与 latest CAS；Automation 验证 `LIVE` processing-order、false→true/true→false、重复 true no-write、state/version 与 Action Outbox 幂等。PG Outbox/state producer 的业务失败窗口按场景主规格总纲验证，partition completion、generation gate 与接管竞态按[调度分章 §§5、8–9](6.规则引擎与场景联动/Scene%20Runtime%20调度、分片与故障转移.md#runtime-failover)验证；
- affiliation current-context 测试覆盖设备迁区/换组后的 backlog/redelivery、SubjectLifecycle 重复/乱序/version gap、延迟 Linked、恢复 state 与旧 timer 并发到期；backlog 必须按处理时 current affiliation 匹配，更高版本 REMOVE 或 current-scope guard 后不得复活旧成员，仍命中的 state 不得误清；
- 普通 timer 的业务 identity、current-slot、Binding 换版/删除、freshness 与 current `SubjectContext` reconciliation 按场景主规格总纲验证；timer 的 partition/token 归属、callback 回 lane、restore activation 与 revoke 竞态按[调度分章 §§5–8](6.规则引擎与场景联动/Scene%20Runtime%20调度、分片与故障转移.md#runtime-restore)验证，并覆盖集中到期、恢复积压、失配/查询失败 zero effect；
- 固定 LogAppendTime、sample time与 `processingModePolicyReleaseId` 后，延迟处理/redelivery/original replay得到同一 mode；BACKFILL只做 history/latest CAS且不进入 live Automation，Raw/Canonical 的 REPLAY 只走隔离 matrix，Scene V1 则为 zero evaluation/state/outbox/delivery；
- 集中重连不触发同步认证 HTTP或 cache回源风暴；Webhook/通知故障不阻塞时序主写；所有 publish future、retry和 in-flight callback释放 `BatchLease/BufferLease`；
- 缩容前 pause partitions、停止新任务、排空 in-flight并提交连续前沿；超时后由同组其他成员从 committed offset恢复。

## 13. 扩缩容与可观测性

### 13.1 扩缩单位

下表是逻辑扩缩/归属边界，不等于必须一行一个 Deployment；同一 Pod 可组合多个 role。

| 边界/role | 主要扩缩信号 | 分片/所有权 |
| --- | --- | --- |
| EMQX Access Adapter / self-hosted Gateway | MQTT subscription lag、连接、握手、带宽、Kafka Raw produce p99/throttle/error | broker adapter 或 connection/session owner；Adapter producer不拥有协议路由状态 |
| ProtocolApplication workload | Raw group lag/lag age、partition utilization/skew、output/report/s、MB/s、decode CPU/p99、transaction abort/fence、budget reject、quarantine | 一或少量 protocolKey production groups；多个副本共享 partitions，扩容上限受 partition 数限制；专属 protocolKey 可独立扩缩 |
| Timeseries Materializer | Canonical group lag/lag age、points/s、rows/s、MB/s、batch fill、conflict、TD/latest p99、retry | 单逻辑 consumer group/checkpoint；完整 report 的全部模块 history 与 mode-required latest 完成后推进连续 next-offset |
| Presence reducer | lifecycle/snapshot lag、accepted/stale/conflict、lease/session age | owner tenant/device 的 session generation/sequence CAS；EMQX 模式用 snapshot reconciliation，Gateway 模式用 owner lease |
| Automation/Alarm runtime | 派生事件 lag、candidate/evaluation/no-op/transition/action rate、candidate p95/p99/max、active timer/state bytes、timer due lag、stale timer no-op、outbox age | `scene_event` partition / evaluation-subject lane；完整 state 按 `(tenantCode,bindingId,evaluationSubjectKey)`，operator/timer slot 嵌套在 value 内 |
| Command runtime | outbox age、encode/publish p99、attempt outcome | command/protocolKey/ProtocolApplication/adapter |
| Audit envelope projection | compact audit topic lag、DB p99、retry | 独立 consumer group/partition checkpoint；不读取和解析完整点值 body |
| `saas-api` / `saas-system` | REST/RPC QPS、DB pool、authz scope compile p99 | 源码路径 `saas/service/apisvr` / `saas/service/syssvr`；无状态 API/RPC、层级事务与后台任务租约 |
| `things-api` / `things-device` | REST/RPC QPS、查询 p99、控制面任务 lag | 源码路径 `things/service/apisvr` / `things/service/dmsvr`；无状态 API/RPC 与 IoT 领域任务租约 |

Kubernetes/HPA 只是执行扩缩的工具。平台必须先证明新副本能加入既有 consumer group、获得 partitions、从 committed offsets恢复、保持 sink 幂等并在缩容时完成 revoke/drain；不能把 Pod 副本数误当成 Kafka partition 数。标准 codec 可共享 runtime/image，但只承诺有界 worker、配额与 panic recovery，不承诺 fatal/OOM/crash 隔离；闭源/不可信 codec、持续 CPU 或内存超预算、需独立发布/回滚窗口或任一次影响无关租户的 fatal/OOM，都必须升级为独立 workload，由容器/cgroup/副本提供资源与 crash 隔离。单 topic/partition 拓扑的物理瓶颈先以 producer batch/compression、最热 partition、consumer fetch、transaction、broker disk/network和副本追赶实测；需要更多 partitions 时创建新 topic generation，而不是引入平台自有分桶。

HPA 信号不要求把 `protocolKey` 暴露为普通 Prometheus label。`ProtocolRouteCatalog` 为每个 Kubernetes workload/ScaledObject 固定其一或少量 group/topic inventory，external-metrics adapter 聚合这些 group 的 records lag、oldest-uncommitted age、assigned partitions和消费速率；精确 protocolKey/topic/group→workload映射只存在控制面 inventory或受限运维指标域。Raw 层不知道 LIVE/BACKFILL，资源优先级只能在 ProtocolApplication 解码后的 Canonical consumers/内部有界队列实施；不能重新信任设备自报 mode。扩缩阈值按目标追平时间、partition 数和每副本实测 capacity冻结。

### 13.2 指标

每个进程按实际承载的 component/role 至少暴露：

```text
iot_access_messages_total{component,role,outcome}
iot_access_bytes_total{component,role,outcome}
iot_raw_append_total{component,role,outcome,reason_class}
iot_kafka_produce_total{component,role,outcome,reason_class}
iot_kafka_produce_bytes_total{component,role,outcome}
iot_kafka_transactions_total{component,role,outcome,reason_class}
iot_kafka_offset_commits_total{component,role,outcome,reason_class}
iot_kafka_rebalances_total{component,role,outcome,reason_class}
iot_kafka_fenced_total{component,role,reason_class}
iot_protocol_outputs_total{component,role,outcome,reason_class}
iot_protocol_reports_total{component,role,outcome,reason_class}
iot_protocol_points_total{component,role,outcome}
iot_canonical_wire_bytes_total{component,role,outcome}
iot_materializer_rows_total{component,role,outcome,reason_class}
iot_latest_updates_total{component,role,outcome,reason_class}
iot_presence_transitions_total{component,role,outcome,reason_class}
iot_stage_queue_depth{component,role,topology}
iot_consumer_lag{component,role,topology}
iot_consumer_lag_age_seconds{component,role,topology}
iot_raw_retention_headroom_seconds{component,role,topology}
iot_canonical_retention_headroom_seconds{component,role,topology}
iot_archive_lag_seconds{component,role,outcome}
iot_archive_offset_ranges_total{component,role,outcome,reason_class}
iot_automation_evaluations_total{component,role,outcome,reason_class}
iot_commands_total{component,role,outcome,reason_class}
iot_webhook_deliveries_total{component,role,outcome,reason_class}
iot_authz_scope_compiles_total{component,role,outcome,reason_class}
iot_api_requests_total{component,role,outcome,reason_class}
iot_runtime_info{component,role,build_revision,contract_version} 1
```

吞吐由 counter 的 `rate()` 计算。普通时序指标只允许取值集有界的 `component/role/topology/outcome/reason_class`；`iot_runtime_info` 例外仅携带 `build_revision/contract_version`。tenantId、deviceId、topic、partition、group、SQL、URL、原始错误文本、protocolKey/application/release 全量 ID 不得成为普通业务 label，完整 ID 只进控制面 workload inventory、日志和 trace。Kafka exporter 的 topic/partition/group、under-replicated/offline partition、ISR shrink、request throttle、disk/partition size、transaction abort 与 last-stable-offset lag 放在访问受限的基础设施指标域，不与业务 `iot_*` 基数预算混用。Canonical physical format 还必须报告 encode/decode MB/s、B/report、allocs/report、GC CPU 和 RSS。

基数预算是发布门禁：单个指标族在单 workload 内最多 256 种 label 组合，所有自定义 `iot_*` 在单 workload 内最多 5,000 条 active series、单集群最多 200,000 条；预发布用 Prometheus TSDB status/基数报表验证，超额必须降维或改用日志/trace/inventory，不能靠扩 Prometheus 掩盖。Raw 或 Canonical 的 oldest-uncommitted age 接近各自实际 retention 70% 时告警，达到 85% 时停止非必要 backfill/replay、执行 quota/扩容/扩盘和受控入口保护；不能等 `retention.bytes` 或磁盘水位已经淘汰未处理证据后才报错。对象归档还必须监控连续 offset-range completeness、checksum和恢复抽检。

### 13.3 自动发现与页面

- Prometheus 通过统一 label/port 和 PodMonitor 自动发现新 Pod；
- Grafana 只按有界 component/role/topology 指标聚合；按 ProtocolApplication application/release 查看时，先由 Kubernetes/Compose workload inventory 解析目标副本，再链接它的有界指标、日志和 trace，不把 release ID 注入普通 metrics；
- V1 页面展示连接、Raw append、protocol decode、Timeseries Materializer、lag、错误、DLQ 和命令轨迹；
- DLQ/quarantine 页面必须支持按 `rawRef`、`protocolKey`、`productKey/deviceKey` 和 reason class 检索。解析前（pre-resolution）quarantine 没有 tenant 归属，只面向平台运维；owner 解析成功后的 quarantine/DLQ 才进入租户可见视图，两类入口在页面上明确区分；
- 业务管理台至少提供设备详情、latest、历史趋势、告警和命令结果；运维指标使用 Prometheus/Grafana，二者不混成同一种仪表盘；
- V1 不自研通用 Kubernetes 编排平台；发布、Pod、扩缩和日志先由 K8s/Compose、Prometheus/Grafana 管理；
- 多个客户专属 ProtocolApplication 形成明确运营负担后，再增加薄的 workload 控制面，不重做 KubeEdge/Kubernetes。

## 14. 安全、多层租户与授权

用户、服务和设备使用不同身份域；tenant/department 层级、RBAC、资源策略和数据 owner 也必须分开建模。[授权与鉴权/tenant-hierarchy.md](./3.授权与鉴权/tenant-hierarchy.md) 是闭包表方案的早期参考，本文冻结的是以下混合模型，而不是其中的 GORM 示例代码。

### 14.1 租户与部门层级事实

- `sys_tenant.parent_id` 是租户树边的唯一事实来源；`sys_tenant_hierarchy` 是由同一事务维护的授权查询索引，不是第二份可独立修改的树；
- closure table 包含每个 active tenant 的 self row（`ancestor_id = descendant_id, depth = 0`），并保证 active `(ancestor_id, descendant_id)` 唯一；
- 新建、移动、停用节点前必须阻止 self-parent 和 ancestor cycle。subtree move 在单个 `saas/service/syssvr` 事务中锁定相关边、更新 `parent_id`、重建受影响 closure rows，并原子递增 `hierarchyVersion`；失败全部回滚；
- closure index 可由 parent tree 校验/重建，任何不一致都阻断授权发布而不是静默放行；
- department 使用独立部门树，节点必须绑定且只能位于同一 tenant 内；租户树不能替代部门树，部门也不能跨 tenant 挂接。

### 14.2 可见性、策略与平台管理主体

- 上级租户默认可见自身及全部下级租户数据；这是层级授予的最大可见集合；
- 角色、资源和数据策略只能在该集合内收窄到部门、设备组、产品、资源或动作，不能借策略把 scope 扩大到层级之外；
- `PlatformAdmin` 角色只能在交互式管理请求中换取限时、带原因的 `PlatformPrincipal` elevation capability，用于平台运维、事故处理和授权诊断；它不是“根租户”，不能出现在设备热路径或伪造 root tenant 参与业务归属、计费和配额；
- 所有 bypass、层级移动、scope 发布和越权拒绝都记录操作者、原因、前后版本与影响范围。

### 14.3 saas 与 things 的授权边界

- 交互式 API/查询/命令使用判别联合 `PrincipalContext = TenantPrincipal | PlatformPrincipal`，并与独立 `TenantScope` 组合。`TenantPrincipal` 至少携带 subject、`actorTenantId` 和认证 provenance；`PlatformPrincipal` 携带 operator subject、限时 elevation capability、reason/ticket，不伪造登录 tenant；
- 只有 `saas/service/syssvr` 可以执行层级事务、读取层级状态，并为 `TenantPrincipal` 把 tenant closure、department、RBAC 与资源约束编译成版本化 `TenantScope`；`TenantScope` 至少携带 `hierarchyVersion`、策略版本、允许动作、`resourceOwnerTenantId`、`policyOwnerTenantId` 和可验证 scope handle/约束；
- `things/service/apisvr`、`things/service/dmsvr` 及交互式 repository 必须同时校验 `PrincipalContext + TenantScope`；`PlatformPrincipal` 只接受受审计的管理请求且受全局运维限流，不能被业务 repository 默认当成全权 tenant subject；
- 设备身份信任链固定为：控制面提前下发 credential（username=`<productKey>.<deviceKey>`）→ EMQX/Gateway 本地认证 + client attributes 拆分 + `${client_attrs.productKey}/${client_attrs.deviceKey}` 双占位符通配 MQTT Topic ACL（默认拒绝）→ Adapter 校验 grammar并机械投影为 Kafka Raw topic/key/header → ProtocolApplication 从受保护 record解析 identity、复核 child topology，再按 ownership/model/protocol/processing-mode offset barriers生成 `ProcessingProvenance`。设备 payload 不能声明 tenant、应用 owner或 release；
- EMQX 集中上线和消息热路径不得同步调用 saas/things HTTP 获取认证或路由。凭证、ACL、禁用和轮换通过受控同步任务写入 broker 支持的本地/集群后端并完成版本确认；Adapter 不承担第二次设备数据库查询；
- `PrincipalContext` 与 `TenantScope` 继续绑定版本/过期策略；设备数据面不携带登录 subject。tenant 层级/策略变化不改写 Raw，ownership 或设备迁移则追加独立生效历史；ProtocolApplication、Materializer、Scene Admission/Runtime 不为 backlog/replay 同步调用 saas 或展开 tenant closure。things 不能直查 `sys_tenant`、`sys_tenant_hierarchy` 或自行解释“上级”；
- 小 scope 可以下推精确 tenant predicate；大范围 scope 使用 saas 预编译的 scope handle、租户分区/范围和受控 join/filter 方案，不在 TDengine/关系热查询中展开巨型 `IN (...)`；
- 授权失败默认拒绝；交互式请求的 `hierarchyVersion` 过期或 scope 无法验证时回 saas 刷新，不能退化为“仅按调用方 tenantId 过滤”。MQTT Topic ACL 或 Kafka topic/key/header grammar 不成立时入口拒绝/隔离；ownership/release cache 暂时不可用时 ProtocolApplication pause partition、abort transaction并 backoff，确定不存在或拓扑冲突时才写 quarantine，不能伪造 owner、回退当前 release或提交越过该记录的 offset。

### 14.4 直接 owner 与数据生命周期

- `ownerTenantId` 表示事件产生时直接拥有设备的 tenant，该 tenant 可以还有下级 tenant，它不表示层级中的末端节点；
- Raw Record 不记录 `ownerTenantId`；它保存 ACL 保护后的 product/device Topic identity。`CanonicalDataBatch`/派生事件同时记录直接 owner 与 `ownershipBindingId`，TDengine tag/table、quota/accounting 和 owner 已解析后的 DLQ 记录直接 owner。ProtocolApplication 解析前失败的 ingress/protocol quarantine 只能可靠记录 rawRef、protocol/product/device key 与 reason，不能伪造 tenant；
- Kafka ACL 按数据面/领域面和 workload role 授权：Access Adapter/Gateway principal 只能写获准的 `iot.data.raw.*`；ProtocolApplication principal 只能读自身一或少量 Raw topics及对应 production group，并写获准的 Canonical/quarantine/audit topics；Materializer、Scene Admission、Scene Scheduler、Things SubjectLifecycle relay、Scene Runtime/state-writer、Reducer、Audit 分别使用独立 principal/group prefix 与最小 topic 权限。Scene 各 producer 的 role→oneof kind + production+LIVE 由认证后的封闭 publish API/application contract 强制，因 Kafka ACL 不检查 record payload；Runtime 二次验证全局 allowed kind、payload/key/route 与 context/version，但不能从普通 Kafka record/header 认证 producer principal。`protocolKey` 是处理路由，不代替 tenant 数据授权；Canonical repository仍按已解析 owner执行隔离；
- 上级可见性在查询/命令授权时由 `TenantScope` 计算，不把祖先 tenant 复制成数据 owner，也不重复写多份历史数据；
- 租户层级调整只改变新 `hierarchyVersion` 下的可见范围，不改变设备直接 owner，不重写 Raw、Canonical、Kafka、TDengine 或历史 quota；Raw 本来不含 tenant，重放时通过不可变 ownership history恢复事件发生时 owner；
- 层级移动不等价于设备跨租户迁移。设备迁移必须是独立、审批、可审计的领域流程，明确新旧 owner、生效点、凭证、历史可见性和配额归属。

### 14.5 其他安全基线

- 设备凭证支持禁用、轮换、过期和批量吊销；
- TLS/X.509、topic/device 绑定和连接限流在实际连接终止层执行：一期是 EMQX，下一期才是 self-hosted Gateway；
- `ownerTenantId` 必须由 ProtocolApplication 根据 ACL 保护的 product/device identity 与 ownership history派生，不能信任设备 payload；
- 每个直接 owner tenant 有连接、消息、点位、规则、查询、Webhook 和存储配额。EMQX/Gateway 先执行 credential/device级连接与消息限流；tenant级点位/处理配额在 owner解析后执行，聚合用量走独立记账通道。Kafka 同集群逻辑隔离另外按 principal/client ID 设置 produce/fetch/request quota；live、shadow、replay使用不同身份，replay必须限速。需要入口即 tenant 逻辑硬隔离的客户使用专属 protocolKey/topic/principal/quota/workload；需要物理隔离时使用独立私有化平台部署，而不是让共享 Adapter逐消息查 tenant或动态切换 Data Kafka；
- 协议制品有来源、hash、签名、SBOM、扫描和发布审计，Secret 不进入事件、日志、指标或 release 明文；
- 共享 Raw topic/cluster只能提供平台/topic级保留、存储加密和访问审计；专属 topic 可提供独立 ACL/quota/retention，但 tenant级 Raw KMS key、独立物理删除或故障域需要独立私有化平台部署，不能在 Raw 不含 tenant 后继续作无条件承诺。Kafka ACL/quota不能隔离共享磁盘耗尽、controller故障和整集群升级；§4.3 只解决 Data/Domain 之间的物理拆分，不把共享平台扩展为 tenant→多 Data Kafka 路由层。

## 15. 主流项目对比方法

本节区分“官方可验证事实”和“本项目判断”：

- 联犀按用户指定，以 2026-05-16 发布的商业版公开技术文章为主要依据；这些文章是架构陈述，不是开源交付清单、基准报告或 SLA；
- JetLinks Community 固定比较官方 `2.11` 开发分支（文档核查日的最新稳定 Release 为 `2.10.0`），所有实现判断以链接源码为准；
- ThingsBoard 以 Community 官方文档和主仓为准，涉及 PE/Cloud 的能力不计入 CE；
- EdgeX 是边缘框架参照，不把它当云端企业平台直接竞品；
- 所有吞吐宣传只用于理解定位，不直接作为本项目容量依据。

### 15.1 总览矩阵

| 维度 | 本设计 | 联犀商业版公开架构 | JetLinks Community 2.11 | ThingsBoard CE | EdgeX Foundry |
| --- | --- | --- | --- | --- | --- |
| 产品定位 | 工业高吞吐企业 IoT 平台 | SaaS + IoT + AI 企业底座 | Java 企业 IoT 基础平台 | 完整云端多租户 IoT 平台 | 边缘互操作框架 |
| 工程组织 | 自有 Go Workspace；saas/things/protocol/root share 有界 | Go Workspace monorepo、定制 goctl | Maven 多模块单仓 | 大型 Java/Angular monorepo | 多独立微服务/SDK |
| 初始部署 | role 组合 + 可选 allInOne；专属 ProtocolApplication 可独立 | AllInOne、融合宿主或 gRPC 微服务 | 默认 1 个应用 + Redis + TimescaleDB | 单体或 Transport/TB Node/Executor 微服务 | 多个边缘微服务 |
| 接入与数据面 | EMQX/Gateway → Kafka Raw（72h）→ ProtocolApplication → Durable Canonical → Materializer / Scene Admission → `scene_event` → Scene Runtime；Flink-ready | `dmsvr` 融合 MQTT、协议、设备和时序 | Network/Gateway/Protocol → 内部 EventBus | Transport → Kafka → Rule Engine/Actor | Device Service → MessageBus → App Service |
| 原始消息持久化 | Kafka Raw 是平台入口必需边界；PUBACK 绑定仅自研 Gateway；长周期可选对象归档 | 公开文章未证明 raw-before-decode | Community EventBus 不是持久日志 | 生产 Kafka 持久，但已是归一化消息 | 默认总线不必然持久化；可选 JetStream |
| 协议扩展 | 版本化 codec/映射；多 ProtocolApplication 可灰度与隔离 | Yaegi 四阶段脚本、产品/设备绑定 | ProtocolSupport/Codec + 插件 | 标准 Transport；工业协议多经 Gateway | 每协议 Device Service/SDK |
| 物模型 | 不可变模块快照/ModelRelease | 产品/设备物模型与兼容机制 | DeviceMetadata 属性/事件/功能 | Device Profile + 动态 KV twin | Device Profile/Resource |
| 时序模型 | TDengine 模块宽表 | TDengine 物模型映射、异步批写陈述 | Timescale/TDengine 抽象，通用记录 | PostgreSQL/Cassandra 动态 KV | Event/Reading，存储可选 |
| 规则 | 简单共享 Automation，主写旁路 | 场景/Hook/脚本 | ClusterRuleEngine + 场景 | 共享 Rule Chain + Actor/Queue | eKuiper 为参考规则引擎 |
| 命令 | 持久状态机 + EMQX/Gateway adapter route | 上下行与脚本扩展 | 统一 DeviceMessage/operation | 轻量/持久 RPC + Device Actor | Core Command → Device Service |
| 边缘 | 后续站点/agent，暂不做 | 商业版声明边缘能力 | 不是主要边缘编排框架 | 有独立 Edge 产品/同步 | 核心定位 |
| 最大优势 | Raw 可重解释 + Canonical 多消费者 + 强模型 + TD 宽表 | 小团队工程组织和融合部署 | 国内协议/领域抽象、二开体验 | 功能完整、成熟集群和产品体验 | 边缘协议适配与服务边界 |
| 主要代价 | 需自己完成产品闭环 | 商业实现不可完整验证 | 默认单应用、消息持久化/集群边界需补 | 栈重、动态 KV 和通用规则路径 | 微服务多，不是云 SaaS 产品 |

## 16. 分项目分析与取舍

### 16.1 联犀商业版公开架构

官方文章公开了以下主张：

- 从旧多仓转为带边界的 Go Workspace monorepo，并将工程约束固化进定制 goctl；
- 同一接口可由 AllInOne 的 direct client 或微服务 gRPC client 调用；
- `dmsvr` 将设备管理、MQTT 主链路、协议脚本和时序仓储靠近；
- TDengine 作为时序执行层，数据先经过物模型解释；
- 协议脚本拆分上行前/后、下行前/后四种扩展点。

依据：[tech-arch-02](https://doc.unitedrhino.com/blog/tech-arch-02/)、[tech-arch-08](https://doc.unitedrhino.com/blog/tech-arch-08/)、[tech-arch-09](https://doc.unitedrhino.com/blog/tech-arch-09/)、[tech-arch-10](https://doc.unitedrhino.com/blog/tech-arch-10/)。

需要特别说明：`tech-arch-02` 中“模板控制”指 **goctl 代码生成模板和工程规范**，不是产品品类物模型模板。

本项目借鉴：

- 有边界 monorepo；
- 业务接口与部署方式分离；
- 小型 AllInOne 与较大部署共用一套业务代码；
- 高流量链路避免无意义 RPC；
- 协议四阶段的语义拆分；
- TDengine 不承载业务主数据。

本项目不照搬：

- V1 不承担定制 goctl 和 direct/gRPC 双生成链的维护成本；
- 不把 MQTT、任意脚本、TD 写入全部置于一个生产故障域；
- 不把进程内 Yaegi 放入共享高吞吐主链路；
- 不用文章的定性性能描述替代 ACK、背压、DDL、表数量和压测证据。

商业文章描述的是新版架构；当前公开 `things` 仓不能作为新版商业实现的等价源码。源码复用还需单独处理许可：官方 GitHub 页面识别 `LICENSE` 为 AGPL-3.0，而 README 文案仍称 Apache-2.0，存在公开信息冲突。在权利人书面澄清前，只借鉴思想和公开契约；复制代码必须固定 commit、记录 provenance 并完成许可证审查。[官方仓库](https://github.com/unitedrhino/things)

### 16.2 JetLinks Community 2.11

JetLinks 最值得参考的不是响应式语法，而是清晰的设备接入分层：

```text
NetworkProvider → DeviceGateway → ProtocolSupport/Codec → DeviceMessage
```

这条边界可从官方源码交叉核对：[NetworkProvider](https://github.com/jetlinks/jetlinks-community/blob/2.11/jetlinks-components/network-component/network-core/src/main/java/org/jetlinks/community/network/NetworkProvider.java) · [DeviceGateway](https://github.com/jetlinks/jetlinks-community/blob/2.11/jetlinks-components/gateway-component/src/main/java/org/jetlinks/community/gateway/DeviceGateway.java) · [ProtocolSupport](https://github.com/jetlinks/jetlinks-core/blob/1.3.1/src/main/java/org/jetlinks/core/ProtocolSupport.java) · [DeviceMessage](https://github.com/jetlinks/jetlinks-core/blob/1.3.1/src/main/java/org/jetlinks/core/message/DeviceMessage.java)。

官方仓库把 gateway、network、protocol、rule-engine、timeseries、TDengine、device manager 等做成 Maven 模块，但 `jetlinks-standalone` 和默认 Compose 将其聚合为一个 Spring Boot 应用进程；这是“模块边界细于部署边界”的真实案例。[README](https://github.com/jetlinks/jetlinks-community/blob/2.11/README.md) · [standalone POM](https://github.com/jetlinks/jetlinks-community/blob/2.11/jetlinks-standalone/pom.xml) · [Compose](https://github.com/jetlinks/jetlinks-community/blob/2.11/docker/run-all/docker-compose.yml)

`2.11` 确实装配了 `ClusterRuleEngine`、Scheduler Registry、Cluster Worker 和任务快照，不能简单说它只是单例规则引擎。[RuleEngineConfiguration](https://github.com/jetlinks/jetlinks-community/blob/2.11/jetlinks-components/rule-engine-component/src/main/java/org/jetlinks/community/rule/engine/configuration/RuleEngineConfiguration.java)

但必须同时看到另一半：Community 的 `EventBus` 虽保留过已废弃的 `persistent` 标记，公开 Community 实现没有形成可验证的持久化消息存储、offset、ACK 和 replay 语义；实现仍以进程内订阅/buffer 为主，部分集群订阅明确是企业版边界。因此 `ClusterRuleEngine` 证明了规则任务可调度/恢复，不证明输入事件无损、故障窗口不丢或 exactly-once。[EventBus](https://github.com/jetlinks/jetlinks-core/blob/1.3.1/src/main/java/org/jetlinks/core/event/EventBus.java) · [InternalEventBus](https://github.com/jetlinks/jetlinks-supports/blob/1.3.1/src/main/java/org/jetlinks/supports/event/InternalEventBus.java)

本项目借鉴：

- Network/Gateway/Protocol/DeviceMessage 分层；
- 属性、事件、功能调用的统一消息语义；
- 规则定义、任务、快照和调度分离；
- 类型化 Scene Trigger/Action provider；
- 插件只落在协议、网关、规则节点等边界清晰处。

本项目不照搬：

- 不以进程内 Reactor EventBus 作为企业可靠消息骨干；
- 不因类名包含 `Cluster` 就承诺完整 HA；
- 不把本地写缓冲当成跨节点 raw log；
- 不把所有连接、规则、插件和时序写入放在一个共享 JVM；
- 不把热加载等同于沙箱、资源配额和故障隔离；
- 不用其通用时序记录替代本项目模块宽表，除非真实压测证明更合适。

结论：JetLinks 是优秀的领域设计和二开参考，但不是本项目可靠数据内核的直接蓝本。

### 16.3 ThingsBoard Community Edition

ThingsBoard 是最接近本项目产品形态的云端基准。它支持单体和微服务两种部署；微服务模式将协议 Transport、TB Node、JS Executor 和 Web UI 分开，但 TB Node 仍聚合 Core、REST/WS、Rule Engine 和 Actor System。[Architecture](https://thingsboard.io/docs/reference/architecture/) · [Microservices](https://thingsboard.io/docs/reference/architecture/microservices/)

生产链路为 Transport 归一化消息后进入 Kafka，再由 Rule Engine/Actor 执行规则并通过 action 写时序、告警或外部系统。Kafka 按 entity ID 分区，Actor mailbox 提供实体入口串行处理。[Message Queue](https://thingsboard.io/docs/reference/architecture/queue/) · [Actor System](https://thingsboard.io/docs/reference/architecture/actor-system/) · [Rule Engine](https://thingsboard.io/docs/user-guide/rule-engine/)

顺序语义不能过度解读：分区和 Actor 保证同一实体的入口/提交顺序；Burst/Batch Rule Engine Queue 遇到异步节点时不保证最终完成顺序，需选择 sequential 类策略才会等待上一消息确认；按设备有序通常选择 Sequential by Originator，tenant/global sequential 则有更强串行代价，重试仍需幂等。[Rule Engine Queues](https://thingsboard.io/docs/user-guide/rule-engine/queues/)

ThingsBoard 将 PostgreSQL 作为实体、属性、关系、告警和事件事实来源，时序是 PostgreSQL/Cassandra 中的动态 key-value 点，并维护独立 latest 路径。[Database Layer](https://thingsboard.io/docs/reference/architecture/database/) · [Time-Series Data](https://thingsboard.io/docs/user-guide/digital-twins/time-series-data/)

它还把持久设备 RPC 区分为 `QUEUED/SENT/DELIVERED/SUCCESSFUL/TIMEOUT/EXPIRED/FAILED`，这比把 MQTT ACK 当作业务完成更成熟。[Command & Control](https://thingsboard.io/docs/user-guide/command-and-control/)

本项目借鉴：

- 单仓中模块边界细于部署边界，按负载从单体演进；
- Transport → durable queue → shared processing 的总体形态；
- entity/device 分区、partition ownership 和故障接管；
- 基础消息队列与规则队列策略分离；
- history/latest 分离；
- 告警、通知和 Persistent RPC 的状态模型；
- 租户 profile、速率、队列和配额保护。

本项目不照搬：

- 不复制 Actor 树；Go keyed worker/partition consumer 足以表达当前需求；
- 不让所有遥测必须穿过通用 Rule Chain 才能落库；
- 不采用动态窄 KV 代替不可变强类型物模型和 TDengine 模块宽表；
- V1 已选择 Kafka，但不同时引入 Cassandra、强制 Redis、额外协调系统、每协议独立容器和脚本执行器全套；
- 不把 Device Profile 的即时可变策略当成历史可重建的 ModelRelease；
- CE/PE 能力要按官方矩阵区分，不能把平台 Integrations、400+ codec 等商业能力算进 CE。[CE vs PE](https://thingsboard.io/ce-vs-pe-diff/)

### 16.4 EdgeX Foundry 与 eKuiper

EdgeX 不是本项目的云端竞品。它最适合验证边缘边界：Device Service 负责协议、发现和命令适配，Application Service 负责过滤/转换/导出，Core Command 和 Notifications 保持独立，内部通过 MessageBus 协作。[EdgeX 4.1](https://docs.edgexfoundry.org/4.1/) · [Device Service](https://docs.edgexfoundry.org/4.1/microservices/device/DeviceService/) · [MessageBus](https://docs.edgexfoundry.org/4.1/microservices/general/messagebus/)

本项目借鉴 Device Service/SDK、store-and-forward、安全入口和命令边界；不复制其微服务数量，也不把“用了 MessageBus”自动等同于 durable raw log。

eKuiper 官方定位是资源受限边缘设备上的轻量流处理引擎，规则可用 SQL 或 Graph 描述 source、processor、sink，并支持窗口、JOIN、插件和规则指标。[eKuiper Architecture](https://ekuiper.org/docs/en/latest/concepts/ekuiper.html) · [Rules](https://ekuiper.org/docs/en/latest/guide/rules/overview.html)

因此：

- eKuiper 适合作为未来边缘站点的可选 runtime；
- 平台可以通过 adapter 发布版本化规则并采集 operator 指标；
- KubeEdge/Kubernetes 负责节点和容器生命周期，平台负责业务 release；
- V1 云端不因未来边缘需求而承担 eKuiper 集群化、Manager 二开和 SQL AST 绑定成本。

## 17. 为什么不直接 fork 任何一个平台

| 路线 | 短期收益 | 长期成本 | 本项目判断 |
| --- | --- | --- | --- |
| 深 fork 联犀 OSS | Go 技术栈、设备/产品/OTA 等功能可较快获得 | 商业新版已大改；旧热路径、缓存和表模型需重做；许可需澄清 | 不选主路线 |
| 以 JetLinks CE 为底座 | 领域抽象和页面能力丰富，Apache-2.0 | Java/Reactor 技术栈、默认单应用、可靠日志/隔离仍要重做 | 不选主路线 |
| 以 ThingsBoard CE 为底座 | 产品完成度和集群能力最高 | Java/Actor/Kafka/Cassandra 体系重；强类型模型/TD 宽表要深改 | 除非接受其技术栈和数据模型，否则不选 |
| 完全从零造所有能力 | 边界最自由 | 会重复租户、权限、设备、OTA、页面、通知等大量非差异化工作 | 不选 |
| 新内核 + 选择性复用 | 控制数据面关键语义，同时吸收成熟领域经验 | 需要适配层、来源审计和严格范围控制 | **采用** |

可以复用或移植的对象以“小、纯、可测试”为准：

- 枚举、DTO、topic 解析、类型转换、物模型校验思想；
- 协议测试向量和经过许可核查的纯 codec；
- 命令/告警状态语义；
- 导入工具和兼容 adapter；
- 管理页面交互模式与公开 API 经验。

明确不复用：

- 现有联犀 `FastEvent`、NATS wrapper 和旧数据热路径；
- 设备 × 属性 TDengine 表模型及进程内异步 writer；
- 进程内 Yaegi/Goja 主链路；
- JetLinks Community 的进程内 EventBus 作为 raw log；
- ThingsBoard Actor/动态 KV 存储作为本项目核心模型；
- 任何缺少来源、许可证、测试和维护边界的大段复制代码。

## 18. 分阶段实施

### Phase 0：风险验证，不先铺全平台

先完成以下 Spike：

1. **Kafka Raw 与接入判决实验**：在四档容量口径（重点是 60s 集中峰值 16,667 msg/s 与双峰值 33,334 msg/s）实测 topic/partition 数、最热 partition、producer batch/compression、RF=3、`min.insync.replicas=2`、`acks=all`、ISR shrink、一台 broker 故障和副本追赶；冻结每个 protocolKey 的初始 partition 数与 topic generation规则。验证标准 MQTT Topic、token 注入防护、client attributes双占位符 ACL、child topology、普通 shared subscription与无同步认证 HTTP；证明 Adapter 只做 Topic→Kafka topic/key/header + 原 payload append，逐消息零设备/tenant/route/model查询；
2. **Raw 72 小时与恢复能力**：用压缩后真实 disk growth验证 `retention.ms=72h`、`retention.bytes=-1` 或每 partition 已冻结下限、segment rollover、磁盘稳态水位、集中重连/补传 burst和故障追赶；证明已接纳容量 envelope 内任何 size limit 都不早于 72 小时淘汰，超载时的限速/入口保护也不会缩短已 append Raw 的承诺。暂停 ProtocolApplication后恢复，记录 `consumerCapacity > ingressRate`、catch-up MB/s和RTO；再验证可选对象归档的 offset-range completeness、checksum、archive lag和随机恢复；
3. **Raw→Canonical transaction**：用真实 Modbus/厂商 protobuf 覆盖 transaction begin、各 output/quarantine/`NO_OUTPUT` publish、`sendOffsetsToTransaction` 和 commit前后全部崩溃点；下游 `read_committed` 不得读到 aborted/未提交 records，abort 不得推进 Raw offset，commit 后全部 expected identities 可最终读取且 Raw offsets 同步推进。专项覆盖同事务 records 跨 poll/partition 分批交付以及 commit 响应丢失，不把 transaction 当作批次组装协议；验证 transactional producer/group fencing、poison quarantine和临时 cache failure pause/abort；
4. 对 100/500/1000 点的 dense/sparse batch逐档记录 `allocs/report`、`B/report`、GC CPU、RSS、encode/decode MB/s、wire B/report、Kafka Raw+Canonical RF3复制流量，以及 Materializer、Scene Admission/Runtime、Reducer、compact Audit projection各读取方 CPU/MB/s；比较 protobuf packed scalar与至少一个紧凑格式，consumer直接 decode Kafka value进入预分配 arena/cursor；
5. 用预编译 `ModuleWritePlan` 验证 TDengine 模块宽表建表、typed lanes 直写、批写拐点、遥测/遥信 history+latest、同时间戳 conflict side record 和跨版本查询；独立验证 latest comparator，以及 Automation 对同一 `subjectKey` 的 `LIVE` processing-order、BACKFILL 不进入 live lane；
6. 验证 `rawRef=(clusterId,topicId,partition,offset)`、topic删除重建换 ID、归档导入保留 source rawRef；验证 Adapter response丢失/崩溃的 producer-session内与跨 incarnation重复边界。覆盖五状态格式、确定性 manifest、outcome keys、多 Pod production group、per-partition连续 next-offset、rebalance、shadow group、RawOffsetBarrier、compatible rolling、route/group/transaction fencing、topic generation迁移、original/migrated replay和迟到旧 wire/CommandReply；
7. 验证单 report与 Raw aggregate/解压/CPU/时间预算；aggregate failure只产生一条 bounded raw quarantine并与 next-offset原子提交。验证 Automation 倒排索引求交、false→true/true→false、重复 true no-write，以及场景主规格总纲 §12.3 拥有的 PG Action Outbox commit → Actor 内存 state/timer 发布 → async `scene_state` 业务顺序；local partition generation/token gate、partition-local completion、timer restore activation 与接管竞态按[调度分章 §§5–9](6.规则引擎与场景联动/Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)验证。分开实测 PG Action Outbox/Alarm 写率与 Kafka `scene_state` append rate，不得声称 state/timer/Outbox 同事务；
8. 验证 `BatchLease/BufferLease` 在异步 writer、Kafka publish future、retry、取消和故障路径的引用释放，并证明容量由有界队列/credit/semaphore而非 `sync.Pool` 控制；
9. 验证 tenant closure事务、`PrincipalContext + TenantScope`、MQTT Topic ACL信任链、Kafka Data/Domain topic+group prefix ACL、principal/client quota、ProtocolApplication cache、gateway-child拓扑、owner迁移历史和解析前无 tenant quarantine；同时注入“设备双峰值 + 限速 replay + broker故障”，证明单集群逻辑隔离仍满足 domain event SLO，并验证 `DataKafkaBootstrapServers/DomainKafkaBootstrapServers` 可指向不同集群而不修改领域契约。

compact batch 的硬性不变量是：**pool 已预热且容量足够时，分配次数不得随点位数线性增长。** 500 点升到 1000 点允许有效数据字节近似线性增加，不允许新增逐点 heap allocation。

Phase 0 退出条件：

- 验证并冻结唯一的 Kafka Raw → Durable Canonical生产拓扑、Raw→Canonical transaction、各 consumer group连续 next-offset、外部 sink幂等和 Raw/Canonical/Materializer 已定义链路的隔离 replay 语义；该冻结不包含 Scene replay evaluator；
- 冻结 Canonical/低频标准输出 contract、typed reducer outcome/`AcceptedStateTransition`、带稳定 `outputId/processingNamespace/processingMode/routingProvenanceId` 的 `ComputedMetricUpdated`、`DecodeManifest`/output identity、outcome keys、单 report/Raw budgets与版本化 `processingMode`；Scene affiliation 冻结为 current `SubjectContext + aggregateVersion + SubjectLifecycle` 语义，不建立 snapshot/cutover；同时验证 Scene `(tenantCode,bindingId,evaluationSubjectKey)` 完整状态键、[场景主规格总纲 §10](6.规则引擎与场景联动/场景联动%20Runtime%20主规格总纲.md#10-timer-与-delayed-action)拥有的 timer business identity/validity 与 current-slot guard、主规格总纲拥有的 schedule/lifecycle/transition/effect identities及 async `scene_state`/PG Action Outbox 失败窗口；[调度分章 §§5–9](6.规则引擎与场景联动/Scene%20Runtime%20调度、分片与故障转移.md#runtime-state-machine)拥有 partition token、timer callback 回 lane、timer restore activation、partition completion 与 revoke/ownership lifecycle；
- 冻结 MQTT Topic→Kafka topic/key/header投影、protocolKey/topic/partition预算、`rawRef`、RawOffsetBarrier、production/shadow/replay groups、group/transaction/route fencing、72小时 Raw retention、Canonical retention档位、归档字段和重复 append降级边界；
- 冻结 Data/Domain Kafka逻辑 ACL/quota和物理拆分触发条件；证明每个 ProtocolApplication workload只需一或少量 Raw topics，多个副本可共享 partitions，且无需固定设备 decode lane；
- 冻结租户层级/`PrincipalContext + TenantScope`、EMQX ACL信任边界、protocol/model/mode/ownership histories、`ownershipBindingId`和 ProcessingProvenance；
- 在四档容量口径上不存在阻断性问题，并留下可复现 benchmark/pprof/故障报告。

### Phase 1：可售试点纵向闭环（非企业 GA）

Phase 1 可以作为单客户付费试点或小型标准版交付，但必须明确应用/数据库等剩余单点、维护窗口、RPO/RTO 和容量边界；它不使用“企业 GA/高可用版”承诺。Data Kafka 是例外：生产试点从 Phase 1 就必须使用至少 3 broker、RF=3、`min.insync.replicas=2`、`acks=all`；这保证日志耐久性下限，但不代表整个平台已完成 HA 接管与故障演练。

只交付：

```text
saas tenant hierarchy / department / user / RBAC / audit
→ things product / device / topology / immutable ModelRelease
→ MQTT/TLS/Device Credential + client attributes 双占位符通配 Topic ACL（默认拒绝）
→ 3-broker Kafka Data Plane（Raw/Canonical RF3、minISR2、acks=all）
→ Kafka Durable Raw（生产 hot retention 72h）
→ ProtocolApplication → Durable Canonical Stream
→ 独立 Materializer / Automation / Reducer / compact Audit consumer groups
→ 多 ProtocolApplication 框架 + 一个真实 codec
→ Timeseries Materializer：TDengine module history + latest
→ Automation：阈值/持续时间/恢复告警
→ Webhook Outbox + 持久命令状态机
→ saas/things 最小管理 API 与设备/趋势/告警/命令页面
→ DLQ/重放/诊断/审计
```

不同时做第二种 MQ、第二套 Canonical 格式/旁路、TCP、WS、边缘、通用规则画布、全通知渠道和多种数据库。

Phase 1 退出门槛：

- 一个真实场景端到端连续运行；
- 目标硬件不少于 72 小时 soak test；
- 明确通过 24h 日均、12h 活跃稳态、60s 集中峰值和 Phase 1 双峰值四档测试；1,667 万 points/s 仅作为双峰值上限；
- 同时记录连接、report/s、points/s、rows/s、Raw + Canonical MB/s、RF3复制、各 consumer group读取、transaction和集中重连，不能只报 msg/s；
- 能在 Canonical retention内低成本重跑下游，或在 Raw 72小时/对象归档窗口内按原 release/binding确定性重建 Canonical、history、latest和告警；
- EMQX/Access Adapter、ProtocolApplication、Materializer、TDengine 中断后无不可解释丢失，并在 RTO 内追平；
- 完成一次保留 source cluster/topic ID/partition/offset rawRef 的归档恢复，以及同一 protocolKey 下的兼容滚动、shadow group、RawOffsetBarrier owner handoff、transaction fencing、回滚和 topic generation升级演练；不兼容 wire schema使用新 protocolKey或自描述版本。

### Phase 2：企业 GA 与 HA

“企业 GA/高可用版”必须完成本阶段；不能等发生事故后再建设。试点升级或满足任一条件时进入：客户要求 99.9%/无单点；峰值持续超过单实例已验证容量 50%～60%；故障后追赶接近 RTO；多个付费租户产生 noisy-neighbor；重连/查询开始影响写入。

建设：

- `saas/service/apisvr`、`saas/service/syssvr`、`things/service/apisvr`、`things/service/dmsvr`、Access Adapter、ProtocolApplication、Materializer、Scene Admission/Runtime、Reducer/Audit role 的多副本/接管；
- 在 Phase 1 Data Kafka 耐久基线上完成正式生产化：KRaft controller/broker 角色与节点/故障域布局、N+1 容量、滚动升级、监控、备份/归档和故障演练；同时完成 PostgreSQL/TDengine 正式 HA 与恢复；
- EMQX adapter HA；self-hosted Gateway 启用后再建设 session owner、跨 Gateway 命令路由和优雅滚动升级；
- 租户限流、队列、TTL、查询和通知配额；
- ProtocolApplication 独立镜像的签名、资源隔离、灰度、回滚与故障演练；
- schema migration、证书轮换，以及真实项目需要的第二协议、OTA 或设备影子。

### Phase 3：按瓶颈平台化

满足下列信号再调整部署组合：

- Access/Gateway、某个 ProtocolApplication、Materializer、Scene Admission/Runtime 之间出现超过约 3 倍的扩容差异；
- 某个组合宿主的发布节奏或故障域开始影响无关 capability；
- 同一瓶颈连续两个容量周期超过 60%～70%；
- 积压追赶持续违反 RTO；
- 大租户要求独立队列、存储、数据驻留或升级窗口；
- 多种协议已形成独立团队/发布节奏。

此时才考虑重新组合 runtime roles、专属租户 runtime、WASM、Flink、Edge agent/KubeEdge、跨 AZ；Flink只接 Durable Canonical，不理解厂商 Raw。只有 §4.3 拆分门槛命中时才迁出 Domain Kafka，Raw 与 Canonical继续留在同一 Data Kafka；不能推翻 Phase 0 冻结的 transaction/offset/replay契约，多地域只由明确合同触发。

## 19. 验收标准

### 19.1 功能闭环

- 设备从注册、认证、上线、上报、history/latest 查询，到告警、Webhook 和命令回复全部可审计；
- 一个厂商 ProtocolApplication 能用一或少量稳定 Raw topic 版本化发布、扩容、回滚、隔离故障并用固定 Raw 样本重放；多个 production Pod 共享一个 Kafka consumer group 的 partitions，shadow canary 使用独立 group 且只写隔离 namespace，release timeline、ProcessingProvenance、image/artifact 保留到 replay/audit 引用结束；
- 全类型 manifest 有 golden vectors；PropertyReport 满足一 Raw 到多个完整 report，其他标准事件也进入同一完成集合。Canonical outputs/有界 quarantine 与 Raw consumer offsets 必须归属同一 Kafka transaction commit outcome；`preResolutionProcessingId/rawProcessingId` 只用于结果幂等、诊断和溯源，不再依赖一个“先查终态再解码”的外置两阶段 checkpoint；transaction commit 响应丢失、Pod 崩溃和 rebalance 后，`read_committed` 不得读到 aborted/未提交 records，也不得跳过未提交 Raw offset。同一 transaction records 可以跨 poll/partition/member 分批交付，验收不得假设一次 poll 得到整个 manifest；
- payload 的设备报告 ID 只保存为 `sourceReportId`；可稳定去重时平台使用 `H(ownershipBindingId,canonicalDeviceId,sourceEpoch/deviceBootId,sourceReportId,reportIndex,processingNamespace)`，否则使用 `H(rawRef,reportIndex,processingNamespace)`。设备跨 owner tenant 迁移必须产生新 ownership binding，不能让相同 source ID 跨租户碰撞；source epoch 缺失或序号回绕用例不得误报跨连接去重，Adapter 重复 append 且无稳定 source ID 的用例不得虚假去重；
- `LIVE/BACKFILL/REPLAY` 只能来自可信 processing context；`LIVE/BACKFILL` 必须由 Kafka broker `LogAppendTime`、decoded sample time 与不可变 `processingModePolicyReleaseId` 纯函数决定并进入 semantic provenance，延迟处理/redelivery 不得改变 mode。BACKFILL 默认 history-only、latest 服从 CAS 且不进入 live Automation；REPLAY/显式 backfill 如未来获批规则求值，只能写隔离 state/outbox，这是“禁止生产副作用”的平台级约束，不代表 Scene V1 已有 replay evaluator；Scene V1 对 REPLAY 零场景求值；
- Scene 验收必须证明：Admission 在 existence/relevance 和 matching 前校验 Canonical/AcceptedTransition/Flink 输入 envelope 的 `ProcessingContext`，只有 `processingNamespace=production && processingMode=LIVE` 可写生产 `scene_event`；`production + BACKFILL` 与显式 job BACKFILL 都正常 bypass，不得误 quarantine。输出后的公共 `SceneEvent.processingContext/identityAlgorithmVersion` 由 SOURCE/Linked/Schedule/SubjectLifecycle/LifecycleTarget/timer 全链路原样保留。对 BACKFILL、REPLAY、shadow、缺失/未知/表外 mismatch 或来源不可信组合，生产 `scene_event/state/timer/Alarm/Action Outbox` mutation 与 dispatcher 外发均为零；
- Lifecycle/ServiceReply/CommandReply/DeviceEvent 的乱序分别按 session generation/snapshot reconciliation、命令合法单调状态机和 append-only 契约收敛；旧 session 或迟到 reply 不能回退当前状态，冲突终态可审计；
- 状态平面固定 `ABSENT/VALUE/NULL/RESET/INVALID`；dense 按 schema ordinal 全长排列，sparse 仅列唯一的非 `ABSENT` ordinal，typed lane 只保存 `valuePresence=1` 的值；quality 与 value presence 分离，非 VALUE 投影由 contract version 固定而不从缺失推断；
- ModelRelease 变更不会改变旧 rawRef 按 cutover history 得到的历史解释，旧 `ModuleSchemaSnapshot/pointOrdinal` 在引用窗口内可解析；
- Scene Binding 变更通过 PG current row/revision 与 `scene_binding_lifecycle` 最终收敛，没有 Canonical partition offset vector/checkpoint。`semanticHash` 兼容时 preserve，不兼容时只 close+tombstone+next-live bootstrap；旧 timer callback 被 current slot/generation 拦截，控制流必须 close/cancel 已有 timer slot/handle，且本身零 current-fact read/predicate/FSM/timer create|arm|advance|fire/Alarm/Outbox；
- 只有 source selector/filter、schema/field/unit、keyBy、function、窗口对齐/时区、时间/迟到、NULL/quality/dedupe 与 emission/finality 全部等价的场景窗口才规范化为同一 `ComputationPlan`；阈值和 action 不进入 fingerprint，多个场景不重复维护相同窗口状态。`ComputedMetricUpdated` 的稳定 output identity、mode/namespace、routing provenance、quality/revision/finality 可解释，外部副作用默认只消费 LIVE final 结果；
- 告警定义变更分别覆盖 `CONTINUE` 与 `RETIRE` 契约测试：`CONTINUE` 只由下一条合格 production+LIVE 信号沿用兼容 episode，`RETIRE` 由独立授权补偿工作流关闭；Binding lifecycle/reinit 本身不产生 Alarm 副作用；
- tenant subtree move 原子更新 `parent_id`、closure 和 `hierarchyVersion`；上级可见、策略只收窄、仅管理请求可用的 `PlatformPrincipal` bypass 和跨租户拒绝均有测试/审计；新 append 只接受 ACL 允许且语法合法的 Topic。protocol/model/mode/ownership 的历史解释仍按各自 cutover 与 ProcessingProvenance；Scene affiliation backlog 则按处理时 current `SubjectContext`；
- owner 解析成功后的 DLQ 可定位到 owner tenant/device/event/release/reason；解析前 quarantine 至少定位 rawRef、protocol/product/device key/reason，二者都支持受控重放；
- 告警、Webhook、命令均展示独立状态和重试轨迹。

### 19.2 可靠性

- 所有内存队列、arena 和 in-flight batch 有界；
- MQTT QoS/PUBACK 边界、Kafka producer `acks=all`、Raw→Canonical transaction、连续 next-offset、TD history/latest write 和命令完成语义均有契约测试；生产不存在跳过 Durable Canonical 的旁路；
- Raw 72 小时与 Canonical 承诺窗口的 retention headroom/consumer lag age 告警均在 70%/85% 阈值完成故障注入；在已接纳容量 envelope 内，`retention.bytes=-1` 或冻结 size floor 绝不得先于承诺时间窗口淘汰数据，70%/85% 告警只能触发限速、扩容或入口保护，不构成违约许可；
- enqueue 必须复制或原子转移 `BatchLease/BufferLease`；异步 writer、Kafka publish future、retry 和全部 in-flight 引用释放后才能归还 pool，race/poison/cancel 测试不得出现提前复用或悬挂引用；`sync.Pool` 不得当作容量或背压控制；
- 同一 rawRef/batch 的 consumer redelivery 不会产生重复副作用；跨 Raw append 只有协议提供稳定 source ID/epoch 时才承诺业务去重。Scene 的动作接纳边界是 PG Action Outbox commit，`scene_state` 是异步 best-effort 恢复副本，offset commit 不表示外部动作已执行，也不构成 state/Outbox/input offset 跨存储原子性；完整 input completion、per-partition sequencer 与安全连续 next-offset 只按[调度分章 §5](6.规则引擎与场景联动/Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)验收；
- aggregate parse/budget failure 只有一条以 `rawProcessingId` 唯一的 bounded terminal quarantine；quarantine 与该 Raw partition 的 next-offset 同一 transaction 提交，任意一项失败则整体 abort 并重读；`NO_OUTPUT`、正常 outputs 也覆盖 commit 响应丢失、abort 和 redelivery 契约测试；
- Topic/ProtocolApplication 契约测试覆盖 client attributes 双占位符通配 ACL（直连两级/网关 prefix）与默认拒绝、child topology、token 注入、MQTT Topic→Kafka topic/key/header 投影、单 production group、多副本无设备亲和的 partition 消费、shadow canary 隔离、consumer-group/transactional-id fencing、RawOffsetBarrier handoff、迟到旧 wire/CommandReply、topic generation 迁移与保留旧 artifact/source rawRef 的原版本重放；affiliation 不进入 ProtocolApplication cutover；
- history 同时间戳异值只写 conflict side record；Automation scope 必须来自处理时 current `SubjectContext`，state 中的 `lastSubjectAggregateVersion` 与 current-scope guard 防止迟到 lifecycle/Linked 复活旧成员；state key 的 subject 与 scope 仍分离。重复领取与 redelivery 由 generation/version 和统一 `effectId` 收敛；
- Scene 安全与失败注入必须证明：Replay/shadow/backfill principal 无权写生产 `scene_event/scene_state` 或 Action Outbox，生产 state 只有 `state-writer` principal 可写；Admission/Scheduler/Things SubjectLifecycle/Runtime 独立 principal 的 Topic ACL 与 producer kind/context 契约均有正反向测试；PG 只有 Actor role 可 INSERT Action Outbox，dispatcher 只读/推进状态并在 dequeue/外发前二次验证 production+LIVE。误路由 event、非法 state/timer 不得求值或变更 live state；非法 pending Outbox 必须原子转 `BLOCKED_CONTEXT`、不重试、不外发，strict target lane 后续不得越过；
- `SCENE_ID_V2` cutover 必须证明旧 Scene producer 已 pause/fence、输入前沿与 context-less `scene_event`/callback/DB/Kafka in-flight 已排空，旧 Action dispatcher lease/外发/status/PG in-flight 已 drain 且 pending-set frontier 已冻结，旧 credential 已撤销；不能排空的 event 仅隔离并经损失审批，禁止跨算法重算。identity-bearing legacy state 必须 tombstone+cold start 并对分类型重复/延迟效果签字；可信 legacy pending Outbox 保留原 `effectId` 只投递，无法证明者转 `BLOCKED_CONTEXT`，Runtime 禁止默认 LIVE；首条 V2 record 后只允许 V2-compatible binary rollback。`scene_binding_lifecycle`/explicit reinit 验证为 no-state-value control，必须 close/cancel 已有 timer slot/handle，zero current-fact read/predicate/FSM/timer create|arm|advance|fire/Alarm/Outbox，下一条合格 production+LIVE SceneEvent 才懒 bootstrap；
- 下游故障不会导致 Access Adapter/Gateway、ProtocolApplication 或 Materializer OOM；
- 层级变化不重写历史 owner 数据，重放副作用默认关闭；
- 备份恢复、升级回滚、ProtocolApplication 灰度和证书轮换完成演练。

### 19.3 性能

- 24h 日均、12h 活跃稳态、60s 集中峰值、Phase 1 双峰值和日出重连/故障追赶分别测试；报告不把 1,667 万 points/s 写成常态；
- 测试同时记录连接、report/s、MB/s、points/s、rows/s、Kafka RF3 复制、各 consumer group MB/s、规则数、查询 QPS、p95/p99 和资源占用；
- compact batch 对 100/500/1000 点分别记录 allocs/report、B/report、wire B/report、GC CPU、RSS、encode/decode MB/s 和各读取方 CPU；
- raw aggregate benchmark 覆盖 `maxOutputsPerRaw/maxReportsPerRaw/maxDecodedPointsPerRaw/maxDecodedBytesPerRaw`、解压后字节、CPU/时间预算及最坏 quarantine 大小，证明预算失败前无部分 publish；
- Automation benchmark 同时记录 input/s、scope∩trigger 后 candidate p95/p99/max、evaluation/no-op/transition/timer/action rate、active state/timer、timer due lag、PG Action Outbox/Alarm 写率、Kafka `scene_state` append rate/bytes 和恢复时间；证明无变化消息的 PG 写率不随 `input × candidates` 线性增长；
- pool 预热且容量足够时，allocs/report 不得随点位数线性增长；
- 预发布报告证明每个指标族每 workload 不超过 256 种 label 组合，所有 `iot_*` 不超过 5,000 active series/workload 和 200,000 active series/cluster，且不存在 application/release 全量 ID 普通 label；
- 任何“百万设备”“亿级点位”结论都附硬件、payload、模型、物理格式、保留期和副本条件；
- 厂商 benchmark 与规划中的 `small-object-large-object/` 设计估算只做参考，不替代本项目 benchmark/pprof。

## 20. 仍需后续 ADR 冻结的事项

1. **[P0 阻塞]** Canonical/低频标准输出 format、typed reducer outcome/`AcceptedStateTransition`、`DecodeManifest`/output identity、`semanticReleaseSetHash`、`sourceReportId` 命名空间化、outcome keys、可信 mode 和各级 budgets；Canonical 明确不冻结 Scene affiliation。同时冻结 Raw→Canonical Kafka transaction、`read_committed`、processing/replay namespace 和唯一 offset/replay 语义。
2. **[P0 阻塞]** Data Kafka 的 Raw/Canonical/quarantine topic 拓扑、partition/key、topic generation、RF/minISR/`acks`、batch/compression、retention.bytes/time、consumer-group/transactional-id 命名与 quota；冻结 Phase 1 至少 3 broker 的耐久基线、每平台部署唯一 Data Kafka 目标、Data/Domain 逻辑隔离及只迁出 Domain 的物理拆分门槛。V1 不引入设备哈希分桶或固定 decode lane，partition 数以实测吞吐、故障追赶和扩容上限冻结。
3. **[P0 阻塞]** 外部 EMQX Access Adapter 与 self-hosted Gateway 各自的量化 PUBACK/SLO、普通 shared subscription、无同步认证 HTTP、invalid Topic/payload quarantine 与无稳定 source ID 时的重复 append 边界。
4. **[P1 前冻结]** tenant `parent_id`/closure 事务、`hierarchyVersion`、department tree、`PrincipalContext + TenantScope`、client attributes 通配 ACL（直连/网关/下行订阅）与授权源选型（file ACL / Built-in DB）、`ownershipBindingId`/ownership history、缓存失效、Raw 无 tenant 时的配额/加密/删除边界及大 scope 查询计划。
5. **[P0 阻塞]** MQTT Topic→Kafka topic/key/header 语法、`messageKind` 有限集合及 protocolKey 从属关系、`protocolKey` 按“厂家 × 协议族”分配且不复用的规则、production/shadow/replay group、RawOffsetBarrier、consumer-group/transaction/route fencing、ProcessingProvenance/保留期、兼容滚动/回滚、迟到旧 wire/CommandReply 窗口、不兼容协议的新 protocolKey/自描述版本、镜像信任与声明式 mapping 能力上限。
6. **[P1 前冻结]** ModelRelease 到 TDengine STABLE 的命名、按 rawRef 生效的 ModelCutover、`pointOrdinal/ModuleWritePlan` 发布、破坏性变更和 GC；同时冻结 history conflict side record 与 latest total-order/CAS 的存储和迁移规则，以及 Automation processing-time/subject-lane 契约。
7. **[P0 阻塞]** Materializer/Scene Admission/Scene Runtime/Reducer/Audit consumer-group 命名、连续 next-offset 与外部 sink 幂等；场景主规格总纲冻结 Scene state 业务有效性、identity/timer/freshness、current `SubjectContext`、SubjectLifecycle、aggregateVersion、current-scope reconciliation 与 backlog/redelivery current-affiliation 语义，不引入 affiliation snapshot/barrier；[调度分章 §§5–9](6.规则引擎与场景联动/Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion)冻结 Scene partition completion、assignment-time state-before-event restore/activation、rebalance 与 Zombie 接管协议。
8. **[P1 前冻结]** CommandAttempt 冻结 protocol/model/encode release、下行 Topic/encoded payload 的规则，以及 `OUTCOME_UNKNOWN` 的人工处置和危险命令策略。
9. **[P0 阻塞]** Phase 1 的真实硬件、四档容量、RPO、RTO、成本上限，以及 Canonical 24/48/72 小时档位的选择依据。
10. **[P1 前冻结]** Raw 72 小时后是否进入 30/90 天加密对象归档；归档必须保存 source rawRef、Kafka topic ID/key/headers/partition/offset/`LogAppendTime`、payload、protocol/model/mode/ownership cutover history，并定义密钥轮换、解密失败与归档导入重放。
11. **[P1 前冻结]** TDengine 同时间戳字段补齐、conflict side record 的具体写法，以及 latest comparator 在 source epoch/sequence 缺失或回绕时的 tie-break、质量标记和审计规则。
12. **[引入窗口能力前冻结]** 覆盖 selector/filter、schema/unit、窗口对齐/时区、NULL/quality/dedupe 与 emission 语义的 `ComputationPlan` fingerprint，以及引用计数/版本 barrier/GC、固定窗口选项与状态配额、Flink config routing（禁止全量设备规则 Broadcast State）；同时冻结 `ComputedMetricUpdated` 的稳定 output identity、processing namespace/mode、routing provenance、revision/finality 和迟到结果对外部副作用的策略。

在这些 ADR 和 Phase 0 Spike 完成前，不应继续建设通用规则 UI、规则 Pod 编排、跨集群边缘控制或多种基础设施适配。

## 21. 知识产权与来源边界

当前只有个人重新总结的通用设计文档，没有原公司源代码。新平台应采用 clean-room 边界：

- 不复制原公司的私有源码、接口定义、数据库结构、租户数据、凭证、部署文件或专有标识；
- 领域对象和消息契约由本文需求重新定义，并以公开标准/官方开源资料为证据；
- 每段外部代码固定来源仓库、commit、文件、许可证和修改记录；
- 第三方依赖生成 SBOM，发布前执行许可证和安全扫描；
- 许可证冲突或权属不清的代码只参考思想，独立实现；
- 商业发布前仍需单独核对原劳动合同、保密义务和知识产权约定。
