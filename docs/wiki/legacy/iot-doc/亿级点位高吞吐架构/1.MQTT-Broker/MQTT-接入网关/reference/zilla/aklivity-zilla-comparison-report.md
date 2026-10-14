# Aklivity Zilla vs mqtt-gateway 南向转发架构对比报告

> 本文对比两种"下行/南向消息转发"的架构模型，论证 mqtt-gateway 为何采用 Registry-as-index（Redis presence + gRPC 直连定向 + 四元组 fencing）而非 Zilla 式的 Kafka-as-state。
> 配套文档：[Presence Registry 详细设计](../MQTT-接入网关/2.cloud-mqtt-presence-registry-design.md)、[实现红线](../MQTT-接入网关/7.cloud-mqtt-gateway-implementation-red-lines.md)。

---

## 1. 结论

**mqtt-gateway 现有的 Registry-as-index 设计比 Zilla 的 Kafka-as-state 更适合本场景，不应改用 Zilla 模型。** 决定性理由（2026-06-20 修订，下行已从 NATS 改 Kafka）：

```text
1. 寻址准确性：session 外置 Redis（显式查询 + 四元组 fencing），不进 Kafka compacted topic
   —— Zilla 把 session 也放 Kafka，引入僵尸 session 清理 + 迁移窗口错发。
2. 中间件精简：Kafka + Redis 两套（已去 NATS）—— 职责清晰，Kafka=消息流，Redis=状态。
3. session 恢复靠设备重连（Redis presence 重建），不依赖 Kafka compacted topic 重放。
```

> **延迟优势已放弃（如实记录）**：早期设计用 NATS 承载下行时，延迟为 1~3ms（优于 Zilla 的 10~50ms）。下行改 Kafka 后，延迟与 Zilla 同量级（10~50ms），**不再有延迟优势**。这是减到 2 中间件换来的代价——架构简化优先于延迟隔离。逆变器控制指令在 10~50ms 可接受（非微秒级实时控制）。详见 §3.1。

Zilla 的优势（基础设施极简、session 持久化）在本场景价值有限：逆变器设备本就会重连，session 重建是常态而非异常；且 Kafka 持久化 session 与 mqtt-gateway 的 pointer-poor 内存模型（红线 §4）冲突。

**适用边界**：Zilla 模型适合"下行能容忍秒级延迟 + 只想维护一个 Kafka + 设备不主动重连"的场景。mqtt-gateway 的非控制指令下行（批量配置下发等）若未来出现，可考虑借鉴 Zilla 模型走独立通道，但不与实时控制指令混用。

---

## 2. 两种模型的机制对比

### 2.1 Zilla（Kafka-as-state）

```text
1. 设备连接到某个 Zilla 节点
2. 该节点持有设备 MQTT session 和 socket
3. session/subscription 状态写入 Kafka compacted sessions topic
4. 平台下行消息写入 Kafka messages/downlink topic（按 device key 分区）
5. 当前持有该设备 session 的 Zilla 节点消费对应分区
6. 该节点把消息转成 MQTT PUBLISH 发给设备
```

**本质**：把"设备位置"也当成一条数据流，和上行遥测一样进 Kafka，用 Kafka 的 compacted topic + 消费组机制做分布式状态。**一套基础设施（Kafka）扛所有事**——session、下行、上行。

### 2.2 mqtt-gateway（Registry-as-index）

```text
Command Service
  -> 查询 Redis presence:{tenant_id}:{device_id}
  -> 得到 broker_id / broker_epoch / conn_id / session_version
  -> 校验 broker lease（Redis，和 presence 同库）
  -> 投递到目标 Broker 私有控制通道 downlink.{broker_id}
  -> Broker 用四元组二次校验本地连接
  -> 本地下行 MQTT PUBLISH
```

**本质**：把"设备位置"当成一个**索引**（Redis 查询）。下行不是"广播等持有者消费"，而是"查到持有者再定向投"。Kafka 只扛上行 raw log，不扛路由状态。详见 [presence 设计 §6](../MQTT-接入网关/2.cloud-mqtt-presence-registry-design.md#6-下行查询与二次校验)。

### 2.3 机制对照表

| 维度 | Zilla（Kafka-as-state） | mqtt-gateway（Registry-as-index） |
|---|---|---|
| 寻址载体 | Kafka compacted sessions topic | Redis presence key |
| "设备在哪个 Broker" | 由 Kafka **消费位置**隐式表达 | Redis value **显式字段** broker_id/epoch/conn_id/session_version |
| 下行如何找到设备 | 写 downlink topic，**持有 session 的 Broker 消费到** | 查 Redis 拿 broker_id → **定向投递** downlink.{broker_id} |
| Broker 如何收下行 | 每个 Broker 订阅 downlink topic，Kafka 消费组 rebalance 分配 | Command Service 查 presence 后定向推，Broker 只收自己的 downlink.{broker_id} |
| session 状态位置 | Kafka compacted topic（持久、可重放） | Broker 本地内存（presence 只是路由提示，红线 §9："最终事实以目标 Broker 本地 session 为准"） |
| 故障接管 | Kafka 消费组 rebalance 重新分配 partition | 设备 TCP 断开 → 退避重连 → 新 Broker 写新 presence（session_version fencing） |
| 防"错发到旧 Broker" | Kafka offset/epoch | 四元组 fencing + Broker 二次校验 |

**核心哲学差异**：

- **Zilla**：用成熟基础设施（Kafka）换自研复杂度。一套机制理解所有。
- **mqtt-gateway**：用自研精确控制（fencing/CAS/二次校验）换性能和故障隔离。

---

## 3. 量化指标对比

### 3.1 下行寻址延迟（正常路径）

| | Zilla | mqtt-gateway |
|---|---|---|
| 步骤 | 写 Kafka `messages/downlink` → Broker 消费 | 查 Redis presence → gRPC 直连目标 Broker（不经 Kafka） |
| 延迟构成 | Kafka 端到端：producer linger（0~5ms）+ broker 处理 + consumer fetch（fetch.min.bytes 触发的等待，0~500ms） | Redis 查询（~0.5ms）+ gRPC 一跳（~1ms） |
| 典型 p99 | **10~50ms** | **1~3ms**（Redis 查询 + gRPC 直连，无 Kafka 端到端开销） |
| 可调下限 | 可压 linger=0、fetch 周期，但 Kafka 协议固有延迟难低于 5ms | 无 Kafka 参与，延迟由 Redis + gRPC 网络决定，可压到亚毫秒级 |

**结论（2026-06-24 修订）**：mqtt-gateway 下行走 **gRPC 直连单播**（Command Service 查 presence → gRPC 直连目标 Broker，详见 [presence 设计 §6](../2.cloud-mqtt-presence-registry-design.md#6-下行查询与二次校验)），**不进 Kafka**。Kafka 仅承载上行 raw log。因此下行延迟稳定在 1~3ms 量级，**比 Zilla 低一个数量级**——这是 mqtt-gateway「Registry-as-index + gRPC 定向」相对 Zilla「Kafka-as-state + 消费组」的核心优势。逆变器控制指令（调度、限功率）在 1~3ms 量级可充分满足。

### 3.2 寻址准确性（无效投递率）

| | Zilla | mqtt-gateway |
|---|---|---|
| 错发场景 | session 迁移窗口：设备重连到新 Broker，compact topic 更新有延迟，旧 Broker 可能消费到本该给新 Broker 的下行 | presence 失效窗口（Redis 复制延迟 / broker lease 未过期），但二次校验兜底 |
| 兜底机制 | Kafka offset/epoch | 四元组 fencing（broker_epoch/conn_id/session_version）+ Broker 二次校验，NOT_HERE/STALE_SESSION 立即返回 |
| 无效投递率 | 较高（消费组 rebalance + compact 延迟期间） | 极低（定向 + 二次校验） |

mqtt-gateway presence 设计 [§6.1](../MQTT-接入网关/2.cloud-mqtt-presence-registry-design.md#61-为什么下行不使用共享订阅--queue-group) 明确否定了"共享订阅随机投递"——因为被选中的 Broker 未必持有目标设备连接。Zilla 的 Kafka 消费模型在 session 迁移期错发概率更高。

### 3.3 状态存储成本与扩缩容

| | Zilla | mqtt-gateway |
|---|---|---|
| 状态载体 | Kafka compacted topic（每 device 一条 session 记录） | Redis（presence key）+ Broker 本地内存 |
| 百万设备状态量 | ~百万条 compacted record（Kafka 存） | ~百万 presence key（每个 ~200B，约 200MB）+ 本地 session |
| 扩 Broker | Kafka 消费组 rebalance（秒级~十秒级，期间下行抖动） | 无 rebalance，新 Broker 起来直接接连接，presence 各自更新 |
| Broker 缩容/崩溃 | rebalance，partition 重分配 | broker lease 过期（10~30s），设备退避重连 |

Zilla 优势：基础设施更少（Kafka 一统天下）。mqtt-gateway 优势：**设备连接的 presence 迁移不依赖下行消费组 rebalance**——设备重连靠 presence 自然迁移（Redis），下行消费组的 rebalance 只影响"新下行命令的路由"，不影响已建立连接的设备。

### 3.4 故障语义与可靠性

| | Zilla | mqtt-gateway |
|---|---|---|
| 下行可靠性 | Kafka 持久化，Broker 重启后能从 offset 继续 | Kafka 持久化（同 Zilla，Broker 重启后从 offset 继续）；消费端幂等靠四元组 fencing |
| session 持久化 | Kafka compacted，Broker 全挂也能恢复 session | Broker 本地内存，全挂则 session 全丢，设备需重连重建（Redis presence 重建） |
| Kafka 依赖度 | 极高（session + 下行 + 上行全靠它） | 中（仅上行 raw log 靠 Kafka；下行走 gRPC 直连；Redis 承载 presence + lease） |
| 故障爆炸半径 | Kafka 挂 = 全盘瘫痪 | Kafka 挂 = 仅上行断（下行 gRPC 直连不受影响）；Redis 挂 = presence + lease 不可用（[红线 §9.1](../MQTT-接入网关/2.cloud-mqtt-presence-registry-design.md#91-presence-手动降级开关phase-3默认关闭) 降级开关让 CONNECT 仍能成功） |

**（2026-06-24 修订）**：下行走 gRPC 直连单播（不进 Kafka），Kafka 故障**只影响上行**，故障爆炸半径比 Zilla 小——这是 mqtt-gateway「下行不依赖 Kafka」带来的隔离优势。mqtt-gateway 的 session 外置 Redis（不进 Kafka），Zilla 的 session 在 Kafka compacted topic——这是另一个差异：mqtt-gateway 的 session 恢复靠设备重连（Redis presence 重建），Zilla 靠 Kafka compacted topic 重放。

### 3.5 耦合度与架构简洁性

| | Zilla | mqtt-gateway |
|---|---|---|
| 基础设施耦合 | 单一 Kafka | Kafka + Redis（2 中间件，已去掉 NATS） |
| 心智复杂度 | 低（一套机制理解所有） | 中（presence + lease + fencing + 二次校验，但都在 Redis/Kafka 两套基础设施内） |
| 实现复杂度 | 中（依赖 Kafka 成熟特性） | 高（自研 fencing/CAS/Lua/条件删除 + session_version 中心化序列） |

mqtt-gateway 从 3 中间件（Kafka+NATS+Redis）降到 2（Kafka+Redis），心智复杂度降低，但 Kafka 耦合度升高（上行+下行同源）。

---

## 4. 适用性结论：为什么 mqtt-gateway 保持 Registry-as-index

按重要性排序的理由（2026-06-20 修订，下行已改 Kafka）：

### 理由 1：寻址准确性（session 外置 Redis，不进 Kafka）

mqtt-gateway 的 session/presence 外置在 Redis（显式查询 + 四元组 fencing），**不进 Kafka compacted topic**。Zilla 把 session 也放 Kafka compacted topic，引入僵尸 session 清理问题，且 session 迁移窗口的错发率更高（compact topic 更新延迟）。mqtt-gateway 的 presence 失效窗口有二次校验兜底，寻址准确性仍优于 Zilla。

### 理由 2：中间件精简（Kafka + Redis 两套）

mqtt-gateway 减到 Kafka + Redis 两中间件（去掉 NATS），运维复杂度低于 Zilla 的"全 Kafka"——虽然 Zilla 只用 Kafka，但它的 Kafka 同时承载 session（compacted topic + GC + 僵尸 session 清理），复杂度转移到 Kafka 内部。mqtt-gateway 的职责划分清晰：Kafka = 消息流（上行 raw log + 下行控制），Redis = 状态（presence + lease + 认证缓存 L2），各司其职。

### 理由 3：设备连接迁移不依赖下行消费组 rebalance

mqtt-gateway 的设备连接迁移是**连接级自然迁移**（设备重连到新 Broker，presence 在 Redis 更新）。虽然下行 Kafka 消费组在 broker 扩缩容时会 rebalance，但**设备连接的 presence 迁移不依赖下行消费组**——rebalance 只影响"新下行命令的路由"，已建立的设备连接不受影响。对"日出窗口百万设备同时上线"的场景，设备连接风暴由 presence CAS + broker lease 平滑承载，不触发下行消费组的高频 rebalance（broker 扩缩容是低频运维操作）。

### 理由 4：Zilla 的可靠性优势在本场景不成立

Zilla 的 Kafka 持久化带来的"session 丢不了"优势，在 mqtt-gateway 场景价值有限——逆变器设备**本身就会重连**（网络波动、心跳超时），session 重建是常态而非异常。mqtt-gateway 让设备重连重建 session，换取 Broker 的 pointer-poor 内存模型（[红线 §4](../MQTT-接入网关/7.cloud-mqtt-gateway-implementation-red-lines.md)，每连接内存严格受控）。Zilla 把 session 持久化进 Kafka，反而引入"僵尸 session"清理问题（compact topic 里的旧 session 记录）。

### 理由 5：协议语义匹配度

[红线 §3](../MQTT-接入网关/7.cloud-mqtt-gateway-implementation-red-lines.md) 的 QoS1 语义（不能提前 PUBACK、失败不能伪装成功）要求精细的 inflight 控制，这是 Broker 本地 session 的职责（[红线 §4.4](../MQTT-接入网关/7.cloud-mqtt-gateway-implementation-red-lines.md) Downlink Inflight）。Zilla 把 session 放 Kafka，做这种精细 inflight 控制反而绕（要读写 Kafka compacted record 更新 inflight 状态）。mqtt-gateway 的本地 session + presence 索引，让 inflight 控制和数据投递在同一进程内完成，协议语义实现更直接。

---

## 5. Zilla 模型的适用场景（不否认其价值）

Zilla 的 Kafka-as-state 在以下场景反而更优，**只是不是 mqtt-gateway 的场景**：

| 场景特征 | 为什么 Zilla 更优 |
|---|---|
| 下行能容忍秒级延迟（通知类、日志类） | Kafka 延迟不再是硬伤，换极简基础设施 |
| 基础设施团队希望极简（只维护 Kafka） | 一套机制理解所有，运维成本低 |
| session 需跨 Broker 重启持久化（设备不主动重连的被动传感器） | Kafka compacted topic 天然持久 |
| 强依赖 Kafka 生态（Streams/Connect 后续处理） | 状态天然在同构存储 |

**若 mqtt-gateway 未来出现"非控制指令的下行"**（如批量配置下发、日志收集配置），那个子场景可借鉴 Zilla 模型走独立 Kafka 通道，但**不与实时控制指令混用**——两者的延迟 SLA 差两个数量级，强行统一架构会让两边都不优。

---

## 6. 决策记录

| 项 | 决策 |
|---|---|
| 是否改用 Zilla 式 Kafka-as-state | **否** |
| 南向转发架构 | 维持 Registry-as-index（Redis presence + gRPC 直连定向 + 四元组 fencing） |
| 触发本次对比的原因 | 评估开源项目 Aklivity Zilla 的南向转发模型对 mqtt-gateway 的适用性 |
| 主要否决依据 | §3.2（寻址准确性：session 外置 Redis）、§4 理由 1/4/5 |
| 延迟优势（2026-06-24 修订） | 下行走 gRPC 直连（不进 Kafka），延迟稳定在 1~3ms，比 Zilla（Kafka 消费组，10~50ms）**低一个数量级**（§3.1） |
| Zilla 模型保留可能性 | 仅用于未来非控制指令下行的独立通道（§5），不混入实时控制路径 |

---

## 参考资料

- [mqtt-gateway Presence Registry 详细设计](../MQTT-接入网关/2.cloud-mqtt-presence-registry-design.md)
- [mqtt-gateway 实现红线清单](../MQTT-接入网关/7.cloud-mqtt-gateway-implementation-red-lines.md)
- [mqtt-gateway 架构设计](../MQTT-接入网关/1.cloud-mqtt-gateway-architecture.md)
- Aklivity Zilla 官方文档：https://github.com/aklivity/zilla
