# 亿级点位高吞吐架构 · 总览

> 一页速查：架构全貌 + 关键数字 + 核心决策 + 文档索引。用于方案评审与面试讲述时防遗漏。
> 场景：**100 万设备 × 500 测点**，每设备 5 分钟 1 条全量消息（白天上线、夜间断电），每天 12 小时。
>
> **当前主架构基线（2026-07-23）**：[企业级物联网平台总体架构设计](./enterprise-iot-platform-architecture-design.md)。目标是 JetLinks、ThingsBoard 级企业平台；本页继续保留容量与专项设计速查，平台级边界和 eKuiper 取舍以主架构文档为准。

---

## 一、端到端链路

```text
逆变器设备（100 万，Modbus 二进制全量上报）
  │
  ├─【一期·现状】EMQX 5.8.9 开源版 3 节点集群 → Access Adapter
  └─【下一期·规划】self-hosted MQTT Gateway
  │ 认证 username = <productKey>.<deviceKey>，拆为 client_attrs 双占位符通配 ACL，默认拒绝
  │ 直连只能发/收自己 product/device 的 up/down Topic；网关绑定自己的 gateway prefix，子设备拓扑在协议层复核
  │ MQTT = iot/v1/up/<protocolKey>/<d|g>/<identity...>/<messageKind>
  │ Adapter 只做语法校验、MQTT Topic→Kafka topic/key/header 投影和原 payload append；零设备/tenant/route/model 查询
  ▼
Kafka Data Plane · Durable Raw（`iot.data.raw.v1.<protocolKey>`）
  │ key = d/<productKey>/<deviceKey> 或
  │       g/<gatewayProductKey>/<gatewayDeviceKey>/<childProductKey>/<childDeviceKey>
  │ value = 原始 MQTT bytes；header 至少有 message-kind
  │ RF3 / minISR=2 / acks=all / LogAppendTime；生产 hot retention 固定 72h
  │ rawRef = (kafkaClusterId, rawTopicId, partition, offset)
  ▼
protocolKey 的唯一 production consumer group（多副本共享 partitions）
  ├─ 每个 workload 只订阅一或少量 Raw topics
  ├─ 从 key + cutover history 解析 ownership binding/device/protocol/model/mode policy
  └─ bounded decode → deterministic DecodeManifest
       │ 同一 Kafka transaction：Canonical outputs/有界 quarantine + Raw offsets
       ▼
Kafka Data Plane · Durable Canonical（生产必经，基线至少 24h）
  ├─ Materializer group → TDengine module history + latest CAS
  ├─ Automation group → PropertyReport / DeviceEvent → 告警/通知/命令/联动 Outbox
  ├─ Lifecycle/CommandReply reducers → AcceptedStateTransition → Automation
  ├─ compact Audit projection group
  └─ 未来 Flink（只消费 Canonical，不理解厂商 Raw）

Kafka Domain Plane：`platform.saas.event.*` / `platform.things.event.*`
  │ V1 与 Data Plane 共用物理集群，但 principal/client/group/ACL/quota 全部分离
  └─ `DataKafkaBootstrapServers` / `DomainKafkaBootstrapServers` 独立配置，命中 SLO/安全/容量门槛只迁出 Domain
     每个平台部署仍只有一个 Data Kafka；tenant 物理隔离使用独立私有化部署

管理面：`saas/service/apisvr` → `saas/service/syssvr`（组件 ID：`saas-api` / `saas-system`）
IoT 面：`things/service/apisvr` → `things/service/dmsvr`（组件 ID：`things-api` / `things-device`）
下行：CommandExecution + Outbox → ProtocolApplication encode
      │ 固定 protocolKey/codec、目标 MQTT Topic 与 encoded payload
      ├─ 一期：EMQX publish adapter → MQTT
      └─ 下一期：会话 owner Gateway → MQTT
```

## 二、关键数字（讲故事的主线）

| 维度 | 数字 | 出处 |
|---|---|---|
| 设备/点位 | **100 万设备 × 500 points/report**；每设备 3 个模块子表，共 **300 万子表** | [容量规划](./8.TDengine时序库/README.md) |
| 24h 日均 | **1,667 report/s ≈ 83 万 points/s ≈ 5,000 宽行/s** | 12h 活跃数据摊到全天 |
| 12h 活跃稳态 | **3,333 report/s ≈ 167 万 points/s ≈ 1 万宽行/s** | 稳态容量基线 |
| 60s 集中峰值 | **16,667 report/s ≈ 833 万 points/s ≈ 5 万宽行/s** | 峰值容量基线 |
| Phase 1 双峰值压测上限 | **33,334 report/s ≈ 1,667 万 points/s ≈ 10 万宽行/s**；这不是常态 | 集中峰值 2× |
| 最小数值体带宽 | 8 B/point 时，双峰值已约 **133 MB/s**；未含 descriptor/header、Kafka RF3、consumer fanout | Canonical 必须按 MB/s 验收 |
| 日增数据 | 4.32 亿宽行/天 ≈ **620 GB/天**（裸数据） | [容量规划](./8.TDengine时序库/README.md) |
| TDengine 存储 | 压缩 5x，90 天 KEEP ≈ 33.5 TB 集群 / **15 Ti/节点**（三副本） | 同上 |
| 写入批量 | 当前候选为单条 SQL 拼满 4MB；活跃稳态估算约 **4~6 条 SQL/s**，拐点待实测 | [高效写入](./8.TDengine时序库/high-throughput-writing.md) |
| 写入线程 | Timeseries Materializer 候选基线 **8 线程**（每线程独立连接），不是冻结值 | 同上 |
| 接入层（一期·现状） | **EMQX 5.8.9 开源版 3 节点集群**（Core-only，官方建议 ~3 节点） | [EMQX 核查](./1.MQTT-Broker/emqx-5.8-open-source-cluster-note-2026-04-14.md) |
| 接入层（下一期·规划） | 自研网关：单 Broker **20~30 万连接**（32GB/16vCPU，23KB/连接）；百万集群 = 4~5 节点 | [容量规划](./1.MQTT-Broker/MQTT-接入网关/11.cloud-mqtt-gateway-capacity-planning.md) |
| Kafka Data Plane | Phase 1 生产试点即至少 3 broker；Raw + Canonical 都按 RF3 计算；分区数、磁盘/网络和 Canonical 24/48/72h 档位由 Phase 0 按 MB/s、消费放大和故障追赶实测冻结 | [主设计 §4/7/12/18](./enterprise-iot-platform-architecture-design.md) |

## 三、核心决策与"为什么"（面试高频考点）

**1. 接入层路线：一期 EMQX 起步，下一期自研替代，为什么？**
- **一期（现状）**：EMQX 5.8.9 开源版 3 节点集群快速落地。已知约束：5.8 起开源版仅支持 **Core-only** 集群（无 Core+Replicant），官方建议规模 ~3 节点，更大规模/复制型持久会话需企业版（[核查笔记](./1.MQTT-Broker/emqx-5.8-open-source-cluster-note-2026-04-14.md)）；
- **下一期（规划）**：自研"极简接入层"替代——只做 CONNECT 认证 + 上行原始消息 append + 低频南向控制，砍掉 retain/桥接/通用路由；Kafka 是设备接入后的数据日志，不让 MQTT Broker 兼任数据处理层；
- 自研单节点 20~30 万连接的关键：pointer-poor Session（~192B 定长、GC 扫描可控）+ 23KB/连接常驻（[实现红线](./1.MQTT-Broker/MQTT-接入网关/7.cloud-mqtt-gateway-implementation-red-lines.md)）。
- 一期由 EMQX 终止 MQTT 连接，设备 PUBACK 语义由 EMQX 控制；平台侧 Adapter 不能事后把它解释为“Kafka Raw 已 `acks=all`”。严格 raw-log-before-PUBACK 只能在自研 Gateway 直接持有连接时实现。

**2. Raw Log、Canonical Stream 和下行命令的边界是什么？**
- **Raw 是设备原始事实日志**：承担 Access Adapter→ProtocolApplication 的 durable handoff、削峰、故障隔离、有界积压、恢复、重解码/迁移和短期取证。生产 hot retention 固定 **72 小时**；`retention.bytes=-1` 或按已接纳 envelope 冻结不早于 72h 的 size floor，告警/超载不构成提前删除许可。不把网关断网续传或长期归档偷换为 Kafka retention；
- MQTT Topic 是入口路由契约。Adapter 把 protocolKey 投影为少量 `iot.data.raw.v1.<protocolKey>` topics，设备/网关身份进 key，`message-kind` 进 header，payload 原样写入；不查 tenant、route snapshot、protocol release 或物模型；
- EMQX/Gateway 必须提前获得 credential→Topic ACL 并默认拒绝。集中上线和消息热路径不调用平台 HTTP；ProtocolApplication 从本地 cutover/ownership cache 解析 tenant、设备、协议和 ModelRelease，并复核网关-子设备关系；
- 每个 protocolKey 只有一个 production consumer group，多 Pod 共享 partitions；shadow/replay 用独立 group 和隔离输出。`rawRef=(kafkaClusterId,rawTopicId,partition,offset)` 只标识一次 durable append，Adapter 重试形成的两次 append 在没有稳定 source ID/epoch 时不承诺业务去重；
- ProtocolApplication 先完成有界 parse/validation 和 deterministic `DecodeManifest`，再用同一 Kafka transaction 写入全部 Canonical outputs/有界 quarantine 并提交该 Raw partition 的连续 offsets。任意一项失败则整体 abort，下游统一 `read_committed`；同事务 records 可跨 poll/partition 分批交付，每个 output 必须自完整，不把 transaction 当批次组装协议；
- **Canonical 是平台标准事实总线**：Materializer 写 history+latest；Automation 消费 PropertyReport/DeviceEvent；Lifecycle/CommandReply 先经 reducer 得到 `AcceptedStateTransition`；Audit 消费紧凑投影；未来 Flink 也只接 Canonical。基线至少 **24 小时**，最终按“必需 consumer 最长中断 + 修复 + 追赶 + 安全余量”在 24/48/72h 中实测冻结；
- Raw 与 Canonical 永远位于同一 Data Kafka，因为 Kafka consume-transform-produce transaction 不跨集群。V1 可让 Domain topics 共用同一物理集群，但必须分离 principal/client/group/ACL/quota 与 bootstrap 配置；当 domain-event SLO、故障、安全或容量门槛命中时，只把 Domain Kafka 物理拆出。每个平台部署只有一个 Data Kafka；tenant 物理隔离交付独立私有化部署。云端 V1 不做 NATS/Kafka 双运行时，边缘本地总线只在 Raw/Canonical 契约边界适配；
- `LIVE/BACKFILL` 由 Kafka `LogAppendTime`、sample time 和不可变 policy release 纯函数判定；BACKFILL 默认 history-only，latest 服从 CAS，Automation 只推进 ordering frontier 不产生实时副作用；REPLAY 默认只写隔离 state/outbox；
- **下行不是 Raw replay**：先持久化 `CommandExecution + Outbox`，再编码为固定协议版本的 transport attempt；一期走 EMQX publish adapter，自研 Gateway 阶段再按 session owner 寻址。需要超过 72 小时的原始证据时，另行进入 30/90 天加密对象归档。

**3. TDengine 的 4MB SQL 与 Stmt 如何选？**
- 当前已核查的 TDengine 3.x 版本/客户端路径中，SQL 解析主要发生在客户端，vnode 接收二进制 `SSubmitReq2`；这只说明 SQL 拼接与 Stmt 汇入相同 submit/Raft 主路径，服务端及端到端成本仍以当前版本、驱动和负载的 benchmark 为准（[高效写入第五章](./8.TDengine时序库/high-throughput-writing.md)）；
- DDL 在 ModelRelease 发布阶段完成，热写路径不自动 ALTER；SQL 拼接只是当前候选，最终与 Stmt 的选择由 Phase 0 在同一批大小、连接数和 schema 下实测决定；
- 当前源码分析显示，同 STABLE 归并和 Stmt 的主要差异可能在客户端/Adapter，但不外推为“服务端零差异”；Phase 0 必须以同 schema、批大小、连接数和数据分布做端到端吞吐、CPU、网络与错误 benchmark；
- 真正值得做的：开启传输压缩、提前建子表（省 tags 文本与解析）；新设备首写痛点是**每新表一次串行 catalog 往返**（注册时缓存预热对治）。

**4. 为什么按模块切超级表而不是一设备一张表？**
- 判据 1/2 是**磁盘控制**：超级表 KEEP（3.3.6.0+，compaction 物理清除，无需拆库/手工 DELETE）按模块差异化保留期，估算省 ~78% 存储；召测/异频模块切出避免 NULL 填充；
- 判据 3/4/5 是**物模型组织**：公共模块 schema 单一来源（防 M 份漂移）、fleet 级聚合原生路径（超级表+tag）、新 SKU 零 schema 成本；
- 详见 [切分收益分析](./5.模块级超级表/1.analysis-module-table-split-benefits-2026-07-19.md)。

**5. 容量规划的总账逻辑**
- TD 宽行：3 rows/report，活跃稳态约 1 万 rows/s，Phase 1 双峰值约 10 万 rows/s；官方数字不代替本项目三副本压测；
- 行宽红利：500 点位物化为 3 宽行，显著少于 500 窄行；但 Canonical 数值体在双峰值已约 133 MB/s，不能用行数掩盖网络、复制和 GC 成本；
- 总账必须同时算 Raw + Canonical bytes、Kafka RF3 复制、各 consumer group 读取、TDengine 三副本和故障追赶，不得只报 msg/s 或 points/s。

## 四、分层文档索引

| 层 | 目录 | 关键文档 |
|---|---|---|
| 平台总体架构 | 当前目录 | [企业级物联网平台总体架构设计（主决策）](./enterprise-iot-platform-architecture-design.md) |
| 接入层（下一期规划） | [MQTT-接入网关/](./1.MQTT-Broker/MQTT-接入网关/) | 容量、会话和网络研究可继续参考；其中早期多 MQ 选型、tenant-in-topic/Raw Envelope、下游直读 Raw 示例均已被主架构 §4/§7/§12 取代。入口契约以[主设计](./enterprise-iot-platform-architecture-design.md)为准；专项入口：[架构总设计](./1.MQTT-Broker/MQTT-接入网关/1.cloud-mqtt-gateway-architecture.md) · [容量规划](./1.MQTT-Broker/MQTT-接入网关/11.cloud-mqtt-gateway-capacity-planning.md) · [实现红线](./1.MQTT-Broker/MQTT-接入网关/7.cloud-mqtt-gateway-implementation-red-lines.md) |
| Broker（一期现状） | [MQTT-Broker/](./1.MQTT-Broker/) | EMQX 5.8.9 开源版 3 节点集群；[EMQX 5.8 开源版集群核查](./1.MQTT-Broker/emqx-5.8-open-source-cluster-note-2026-04-14.md) |
| 数据日志 / Kafka | 当前目录 | 生产基线见[主设计 §4.2～§4.3、§7、§12](./enterprise-iot-platform-architecture-design.md)；[NATS-JetStream/](./4.2.NATS-JetStream/) 只保留为历史 PoC 和容量对照，不是现行生产方案 |
| Raw→Canonical 契约层 | [原始流与标准流/](./2.编码解码协议/原始流与标准流/) | [契约语义、成本与分期](./2.编码解码协议/原始流与标准流/README.md)：五项契约的故障故事、主流对照（CDC/Kafka Streams/DLQ）、概念冻结+自动化分期建议；正式定义以主设计 §7 为准 |
| Canonical 紧凑批次 | [large-binary-codec/](./2.编码解码协议/large-binary-codec/) | pool/typed arena/写入的设计与实施计划（原 small-object-large-object 系列），**尚无 benchmark/pprof 结果**，收益不能当作实测 |
| 协议解码 | [2.编码解码协议/](./2.编码解码协议/) | [质量码契约设计](./2.编码解码协议/质量码契约设计.md)：Canonical 状态词汇、无码协议默认 GOOD、TDengine 总码+例外明细；[large-binary-codec/](./2.编码解码协议/large-binary-codec/) 多点位二进制高性能链路；[small-json-codec/](./2.编码解码协议/small-json-codec/) [低点位 JSON 通用 codec 设计](./2.编码解码协议/small-json-codec/1.design-general-json-codec-for-low-point-devices.md)：schema 驱动零分配提取、三层批策略，**尚无 benchmark 结果** |
| 多层租户授权 | [授权与鉴权/](./3.授权与鉴权/) | [EMQX Topic ACL（acl.conf 通配）](./3.授权与鉴权/mqtt-acl.md) · [租户层级闭包表早期参考](./3.授权与鉴权/tenant-hierarchy.md) · [ThingsBoard 权限体系竞品调研](./3.授权与鉴权/thingsboard-permission-research.md) · [用户模型改造：全局账号+租户成员（B 方案设计稿）](./3.授权与鉴权/user-tenant-membership-design.md)；冻结语义以主架构文档 §14 为准 |
| 时序存储 | [TDengine时序库/](./8.TDengine时序库/) | [容量规划](./8.TDengine时序库/README.md) · [高效写入评估与源码分析](./8.TDengine时序库/high-throughput-writing.md) · [集群 values](./8.TDengine时序库/tdengine-values.yaml) |
| 存储组织/物模型 | [模块级超级表/](./5.模块级超级表/) | [切分收益分析（决策入口）](./5.模块级超级表/1.analysis-module-table-split-benefits-2026-07-19.md) · [模块级超级表设计](./5.模块级超级表/design-module-level-supertable-2026-04-15.md.md) · [冷热分层优化](./5.模块级超级表/design-module-level-supertable-optimization-2026-04-23.md%20(1).md) |
| 规则引擎与场景联动 | [6.规则引擎与场景联动/](./6.规则引擎与场景联动/) | [主规格总纲](./6.规则引擎与场景联动/场景联动%20Runtime%20主规格总纲.md) · [Scene Current Fact 设计](./6.规则引擎与场景联动/Scene%20current%20fact设计.md)（水线/barrier、双档部署、存储选型）· [端到端时序图](./6.规则引擎与场景联动/场景联动端到端时序图.html) |
| 缓存层 | [缓存/](./7.缓存/) | [设备状态统计](./7.缓存/1.device-status-statistics.md) · [设备活跃缓存](./7.缓存/2.device-active-cache.md) · [项目状态缓存](./7.缓存/3.project-status-cache.md) |

## 五、待办/风险（讲述时主动交代）

- [ ] **【Phase 0 最高优先级】Kafka Data Plane 判决性压测**：四档容量下实测 Raw + Canonical 的 producer/transaction/partition 上限、RF3 写放大、consumer fanout 和故障追赶；冻结 topic/partition 数、broker/磁盘/网络容量与 Canonical 24/48/72h 档位
- [ ] **Topic 路由、ProtocolApplication 隔离与发布**：冻结 MQTT Topic→Kafka topic/key/header grammar、client attributes 双占位符通配 ACL（默认拒绝）、protocolKey 按“厂家 × 协议族”预分原则、rawRef/topic generation、release/model/mode/ownership cutover；验证 Adapter 零查询、production/shadow/replay groups、RawOffsetBarrier、group/transaction/route fencing、不兼容协议换 key、重复 append 边界、原版本重放与回滚
- [ ] **一期 EMQX 3 节点集群容量复核**：百万连接 + 日出握手风暴（设备白天集中上线）压测；Core-only 集群的扩容上限即是下一期自研的触发点
- [ ] **下一期迁移方案**：EMQX → 自研网关的连接迁移、凭证同步、灰度切换设计
- [ ] **Raw→Canonical 生产契约**：冻结 manifest/output identity/outcome keys、预算/版本化 processingMode、Kafka transaction + `read_committed`、连续 next-offset、外部 sink 幂等、per-rule frontier/事务与隔离 replay 语义
- [ ] **compact batch 实测**：100/500/1000 点记录 allocs/report、B/report、GC CPU、RSS、encode/decode MB/s、Kafka RF3 和各 consumer group 读取；预热且容量足够时分配数不得随点数线性增长
- [ ] **租户层级授权**：`parent_id + closure`、subtree move、`hierarchyVersion`、scope cache 失效和大范围查询计划
- [ ] 4MB 批大小拐点需 taosBenchmark 实测校准（1000 行 / 2500 行 / 满 4MB 三档）
- [ ] 三副本集群按四档实测：活跃稳态约 1 万 rows/s，Phase 1 双峰值约 10 万 rows/s，并记录 133 MB/s 最小数值体之上的总账
- [ ] **Data/Domain 隔离验证**：同一物理 Kafka 上注入设备双峰值、限速 replay 和 broker 故障，证明独立 principal/client/group/ACL/quota 仍能满足 domain-event SLO，并演练只迁移 Domain Kafka
- [ ] **Raw 归档边界**：评估 72h hot retention 后的 30/90 天加密对象归档，验证 source rawRef/cutover history 保留、密钥轮换和导入重放
- [ ] 新设备首写的串行 catalog 往返需配合注册时缓存预热落地
- [ ] STB KEEP 的回收节奏依赖 compaction 周期，容量按"短 KEEP + 一个 compact 周期"留余量
