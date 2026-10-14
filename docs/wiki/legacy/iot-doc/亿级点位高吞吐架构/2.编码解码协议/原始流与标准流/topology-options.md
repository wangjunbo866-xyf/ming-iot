# 三种接入拓扑方案对比：Raw → Canonical 边界放在哪

日期：2026-07-24
状态：方案对比与决策参考。回答一个具体问题：**解码边界该放在 Kafka 的前面、后面，还是两层都放？**
本文不修改[主架构](../enterprise-iot-platform-architecture-design.md)任何冻结项，仅为选型提供横向坐标系。

> 阅读前置：本文与同目录 [README.md](./README.md) 互补。
> - README 回答"双 Topic 契约层**买什么、何时付**"（契约密度问题）。
> - 本文回答"**解码边界放哪一层**"（跳数与故障边界问题）。
> 两者正交：本文的三种方案，在每种方案内部仍可独立选择要不要那套契约密度。

---

## 0. 一句话决策

| 你的需求 | 选 |
| --- | --- |
| "解码可能慢/会崩/要灰度，但设备必须一直传得上来" | **方案 C**（Raw + 进程内分发）或**方案 A** |
| "协议标准、JSON 直连、解码不会出问题，只要最省" | **方案 B**（解码前置单 Canonical） |
| "还要修 codec 后回填历史" | **方案 C**（+ replay Pod，见 §4.6）或**方案 A** |
| "还要解码↔下游在线隔离、跨进程多消费者独立扩缩、每条 Raw 可审计结局" | **方案 A**（双 Topic） |

绝大多数工业/多厂商场景的答案在 A 与 C 之间，**不是 B**。理由见 §4。

---

## 1. 三张拓扑图

### 方案 A：双 Topic（Raw + Canonical）

主架构 §4.2 的当前冻结方案。

```text
设备 ─ MQTT ─ EMQX ─ Access Adapter
                          │
                          ▼  机械投影（零业务查询，极快）
                 ┌─────────────────────┐
                 │ Kafka Durable Raw   │  ← 隔离①：入口 ↔ 解码
                 │ RF=3, 72h retention │     解码慢/崩/升级，设备无感
                 └─────────────────────┘
                          │
                          ▼  ProtocolApplication 有界解码
                 ┌─────────────────────┐
                 │ Kafka Canonical     │  ← 隔离②：解码 ↔ 下游
                 │ RF=3, 24h+ retention│     TD/规则挂了，解码无感
                 └─────────────────────┘
                          │
            ┌─────────────┼─────────────┐
            ▼             ▼             ▼
      Materializer   Automation     Audit
      (独立 group)   (独立 group)   (独立 group)
```

### 方案 B：解码前置（单 Canonical）

即"把 ProtocolApplication 提到 Kafka 之前"。ThingsBoard / JetLinks / 阿里华为腾讯云的形态。

```text
设备 ─ MQTT ─ EMQX ─ ProtocolApplication 解码  ──►  Kafka Canonical
                          │                          │
                          │ 解码在入口路径上          ▼
                          │              ┌─────────────┼─────────────┐
                       无 durable        ▼             ▼             ▼
                       缓冲        Materializer   Automation     Audit
                       (解码即入口)
```

### 方案 C：Raw + 进程内分发（轻量折中）

保留 Raw 做入口隔离与缓冲，**砍掉 Canonical topic**，解码结果在进程内分发给各消费者。

```text
设备 ─ MQTT ─ EMQX ─ Access Adapter
                          │
                          ▼  机械投影
                 ┌─────────────────────┐
                 │ Kafka Durable Raw   │  ← 隔离①：入口 ↔ 解码（保留）
                 │ RF=3, 72h retention │
                 └─────────────────────┘
                          │
                          ▼  ProtocolApplication 解码
                 ┌─────────────────────┐
                 │ 进程内分发（不落 Kafka）│
                 │ 用幂等键兜底重复      │
                 └─────────────────────┘
                          │
            ┌─────────────┼─────────────┐
            ▼             ▼             ▼
      Materializer   Automation     Audit
      (同进程内      (同进程内       (同进程内
       订阅/队列)     订阅/队列)      订阅/队列)
```

---

## 2. 方案 A：双 Topic（Raw + Canonical）

### 2.1 它隔的是什么

**两层独立的故障边界**：

- **隔离①（Raw）**：保护入口。ProtocolApplication 解码慢/崩溃/OOM/灰度升级时，设备数据照样能进 Kafka，入口 SLO 不被解码拖累。
- **隔离②（Canonical）**：保护解码产物。TDengine 慢、规则引擎重启、Audit 投影故障，不会回压到解码；解码按自己的节奏推进 Raw offset，下游各 consumer group 独立 catch-up。

### 2.2 买到什么

1. **REPROCESS 历史重算**——codec/物模型修 bug 后，从 Raw 用新 release 重算 Canonical（主架构 §12.3 `REPROCESS_MIGRATED`）。Raw 是唯一能重解释历史的资产。
2. **decode-once、多消费者独立消费**——Materializer/Automation/Reducer/Audit 各有独立 consumer group、独立 offset、独立扩缩节奏，不互相阻塞（主架构 §4.2 line 183-189）。
3. **故障隔离完整**——上面两层都在。
4. **原始报文证据/合规审计**——Raw payload 是记录级证据。

### 2.3 真实成本

1. **写放大 + 复制放大**：每条消息写 Raw + Canonical，都 RF=3。双峰值 133 MB/s 入口，实际磁盘与网络开销约为单跳方案的 2 倍写 + 6 倍复制（README §4 已算）。
2. **CTP transaction 复杂度**：`sendOffsetsToTransaction` + `read_committed` + transactional.id fencing + LSO 延迟 + poison record abort 风暴（主架构 §12.1）。
3. **双 retention 维护**：Raw 72h + Canonical 24h+，两条线都受 `T_hot >= T_detect + T_repair + T_catchup + T_safety` 约束，都不能提前删（主架构 §7.2 line 516）。
4. **故障域绑定**：Raw 与 Canonical 必须留在**同一个 Data Kafka**，否则 CTP transaction 边界失效（主架构 §4.3 line 204）。这限制了未来拆集群的自由度。

### 2.4 适合谁

- 工业/能源/制造：多厂商私有协议，codec 经常修，需要历史回填。
- 合规/审计要求原始报文留存。
- 多消费者演进节奏不同（时序、规则、告警、审计各自独立扩缩）。
- 团队能扛 Kafka transaction + 双 retention + replay matrix 的运维负担。

### 2.5 不适合谁

- 协议标准、JSON 直连、点位固定、历史错了就错了的消费级场景。
- 追求最低成本、最快接入、团队规模小。
- 单消费者（只写时序库），不需要 decode-once。

---

## 3. 方案 B：解码前置（单 Canonical）

> ⚠️ 本节先描述它"是什么"，§4 再说为什么它对工业场景是错的。

### 3.1 形态

把 ProtocolApplication 解码提到 Kafka 之前：`设备 → EMQX → 解码 → Kafka Canonical → 下游`。这是 **ThingsBoard CE、JetLinks Community、阿里/华为/腾讯云 IoT、涂鸦**的共同形态（详见 README §1 的反面对照表）。它们一致把 durable 层放在**解码之后**。

### 3.2 它换来的好处

1. **省掉一整层**：没有 Raw topic、没有 Raw→Canonical transaction、没有双 retention、没有 release timeline 的 Raw 侧记录。最省的形态。
2. **延迟更低**：少一跳 Kafka。
3. **与主流开源/云厂商生态对齐**：文档、SDK、运维经验、人员招聘都更容易。

### 3.3 它的前提（必须全部成立才合理）

| 你必须接受 | 后果 |
| --- | --- |
| 解码必须永远又快又稳 | codec 崩/OOM/被坏 payload 打挂 → 该 protocolKey 设备数据直接断流 |
| 协议标准化、点位稳定 | 比如纯 MQTT+JSON、消费级设备，codec 基本不出问题 |
| 协议/物模型改了，历史数据将错就错 | 没有 Raw 可回填，只能从现在开始用新解释 |
| 解码不了的报文无处可放 | 要么丢、要么发明一个"原始字节 quarantine topic"——后者等于重新发明 Raw |

第 4 条是结构性矛盾：工业现场"未知点位、payload 炸弹、协议变种"是日常，这些都需要原始字节存活。解码后再持久化，原始字节就消失了。如果为了兜底再单开一个 quarantine topic 存原始字节，那就是**把 Raw 改名重发明一遍**，不如一开始就用方案 A 或 C。

### 3.4 它的可靠边界：对什么场景可靠，对什么场景不够

**关键认知：方案 B 不是"不可靠"，而是它的可靠边界不覆盖工业大报文 + 多厂商私有协议场景。** 对它设计的目标场景，B 是完全可靠且最优的。

B 的所有"故障路径"（codec 崩 → 断流、解析失败 → 丢数据、无 Raw → 不能重算）能否被触发，取决于输入有多"干净"。对照两类场景：

| B 的故障路径 | 云厂商目标场景（不可达 → B 可靠） | 本项目工业场景（活风险 → B 不够） |
| --- | --- | --- |
| codec 崩 → 断流 | JSON/标准 DP 解析几乎不会崩 | 多厂商私有二进制 codec，panic/OOM 是常态 |
| 解析失败 → 丢数据 | 请求-应答契约：失败即终态，设备重发即可 | 网关离线补传的历史数据，设备无法重发 |
| 无 Raw → 不能重算历史 | 数据是"观测"，错了就错了 | 工业告警/能源计量是"账"，要修 codec 回填 |
| 累积压力无缓冲层 | 消费级设备上报频率低、规模受控 | 万台设备并发补传的累积解码压力无处吸收 |

注意：**payload 大小（几 KB 还是几 MB）不是 B 是否可靠的判据。** 一个 500 点二进制全量上报报文就几 KB，完全装得进 MQTT，与腾讯云 16KB 上限无关——那条上限挡的是另一类东西（超长 JSON、base64 图片），不是合法工业报文。B 对工业场景不够的真正理由是上面**与 payload 大小无关的三条**：codec 复杂度风险、工业补传场景、无历史重算能力。

**所以严格说法**：架构与场景是协同设计的，不能拆开看。云厂商不是"选了 B 然后顺便定了小报文上限"，而是"业务定位决定只做标准协议 + 数据即观测，于是 B 刚好够用"。B 的可靠性边界，就是它目标客群的契约边界。本项目要接的工业大报文 + 私有协议 + 数据即账，超出了这个边界——这不是 B 坏，是 B 的信封装不下这封信。

### 3.5 适合谁

- 标准协议（MQTT + JSON/标准 DP）、消费级设备、codec 基本不出问题。
- 不需要历史回填、不在意协议版本治理。
- 把设备数据当"观测"而非"账"（README §4 的判据）。
- 这是国内四家云厂商的选择，与其市场自洽——**对它们的客群，B 是最优解，不是将就。**

---

## 4. 方案 C：Raw + 进程内分发（轻量折中）

这是介于 A 和 B 之间的中间档，也是**大多数人真实需求的最优解**。

### 4.1 它保留了什么

**只保留 Raw，只做一件事：入口 ↔ 解码的故障隔离与缓冲。**

- 解码慢/崩/升级 → 设备无感，数据先进 Raw。
- 集中补传洪峰 → Raw 有界吸收，解码慢慢追。
- 入口 99.99% SLO 只由 `EMQX → Kafka Raw append` 这一段决定，与解码、与下游全部解耦。

这一条是"设备要能一直上传、数据暂时不处理但能追回来"的命根子。

### 4.2 它砍掉了什么

- **砍掉 Canonical topic**：解码结果不落 Kafka，在 ProtocolApplication 进程内通过有界队列分发给各消费者。
- **砍掉 CTP transaction**：改用 at-least-once + 各消费者稳定幂等键（`outputId`/`reportId`/`evaluationId`）兜底，重复由幂等 sink 吸收。
- **砍掉 decode-once 多消费者独立 offset**：各消费者在进程内共享同一份解码结果，没有独立 consumer group、独立扩缩。某消费者慢了会拖累同进程的其他消费者（需要进程内有界队列 + 背压）。

> 注：Raw 仍在，**重解码 Raw 的能力没有丢**——只是没有 Canonical replay topic 这层现成的隔离基础设施，需要靠"临时 replay Pod + 存储层隔离"补回来。详见 §4.6。

### 4.3 它的代价（诚实版）

| 代价 | 说明 |
| --- | --- |
| 失去跨进程多消费者独立消费 | Materializer/Automation/Audit 必须部署在同一进程或同一组进程内，不能像方案 A 那样各自独立扩缩、独立故障。某消费者挂了要靠进程内重试，不能靠"它的 consumer group 落后、其他消费者照跑" |
| 失去 Raw→Canonical 的原子结局语义 | "每条 Raw 恰有一个可审计结局"这个不变量（README §2）不再由事务保证，只能靠各 sink 的幂等 + 对账近似。审计粒度下降 |
| 重放需要补隔离基础设施 | 没有 Canonical replay topic，但 Raw 还在；重解码 Raw 的能力没丢，只是缺现成的隔离层。靠"临时 replay Pod + 存储层表前缀隔离"补回来后，重放能力与方案 A 基本拉平（见 §4.6）。真实差距收敛为：replay 产物合并进生产表是一次额外 ETL，以及 C 本身没有隔离②（在线消费期间下游故障会回压到解码）|
| TDengine 挂了怎么扛 | TDengine 挂 → Materializer 进程内队列堆 → 满了要么背压到解码、要么解码 pause Raw partition（让 Raw 承接积压）。后者可行但 pause 粒度是 partition，不如方案 A 各 group 独立优雅 |

### 4.4 适合谁

- **这正是"数据可以暂时不处理但要追回来"那套需求的最优解**。
- 工业场景、多厂商协议、需要入口高可用，但**不需要**修 codec 后回填几个月前的历史数据。
- 团队想避开 Kafka transaction 的复杂度，但仍要 Raw 的故障隔离。
- 演进路径：C 是 A 的真子集，将来需要 REPROCESS 或多消费者独立扩缩时，**加一个 Canonical topic 即可升级到 A**，Raw 契约不用动。

### 4.5 关键提醒：C 不是"A 的简化版"

一个常见误解是"C 就是 A 砍掉 Canonical"。准确说法是：**C 保留的是隔离①，砍掉的是隔离②**。所以选 C 之前要确认：

- 下游（TDengine、规则、审计）的故障，你接受用"Raw 在上游缓冲 + 追平"来扛，不要求"下游挂了解码照跑"这种独立隔离。
- 各消费者接受同进程部署或共享扩缩节奏，不要求各自独立 horizontal scaling。

如果这两条你都需要，那你要的其实是方案 A，不是 C。

### 4.6 C + 临时 replay Pod：重放的标准 SOP

一个常见误解是"没有 Canonical topic = 不能重放"。这不准确。**重放能力的承重是"独立写入路径"，不是"Canonical topic"。** 方案 A 用 Kafka 层（独立 replay topic + 隔离 namespace）实现独立写入路径；方案 C 完全可以用**"临时 replay Pod + 存储层隔离"**实现等价的独立写入路径——而 kubectl 恰好是拉起这种临时 Pod 最自然的工具。

这不是凑合的 workaround，而是架构上正交且合法的隔离手段：**namespace 隔离的承重是"独立写入路径"，不是"哪一层做隔离"**。Kafka topic 隔离和 Pod 隔离是实现同一目标的两条路径。

#### 隔离的三件事，replay Pod 都能提供

隔离 namespace 本质是三件事，方案 A 和 replay Pod 各自的实现对照：

| 要素 | 方案 A 怎么实现 | C 的 replay Pod 怎么实现 | 必须性 |
| --- | --- | --- | --- |
| 独立 consumer group | `replay-run-<id>` | `replay-run-<id>`，与生产 group 不同 | **必须**——否则和生产消费抢 partition |
| 独立 `replayRunId` | A 的 replay contract 显式 | Pod 启动参数带唯一 ID | **必须**——否则幂等键碰撞，覆盖生产数据 |
| 独立写入目标 | 独立 Canonical replay topic + 隔离 history table | **隔离 TDengine database / 表前缀**（如 `replay_<runId>_`） | **必须**——否则直接写生产表 |
| 在线数据保护 | replay 不 seek 生产 group | Pod 用独立 group，不动生产 group 的 committed offset | 自动满足 |
| 外部动作默认关闭 | replay outbox 隔离 | **Webhook/告警 outbox 关闭或写隔离** | **必须**——否则 replay 期间重复触发告警通知客户 |

第三条是 C 与 A 的关键差异：**A 的隔离在 Kafka 层（topic 隔离），C 的隔离在存储层（database/表前缀隔离）**。功能等价，代价是 replay Pod 里所有 writer 的目标表都要加 `replay_<runId>_` 前缀。

第五条最容易漏、也最危险：replay 用新 codec 重解码一个月前的告警数据，**如果不关 outbox，会把一个月前的告警重发一遍给客户**。这条在 A 里是契约默认项（主架构 §12.3 明确 replay 的外部动作默认关闭），C 的 replay Pod 必须显式实现。

#### 一个具体场景走一遍

假设上线一个月后发现某 Modbus 寄存器解析错了，温度全少算一个系数，要修正。

**方案 A**：开一个 replay run → 用新 codec 从 Raw 重解码 → 写隔离 history 表 → 先查隔离表确认"对，新值 = 旧值 ×10"→ 审批晋升 → 生产 TDengine 受控更新。期间在线数据照常走生产 path，互不干扰。算错了？丢隔离表，改完 codec 再 replay 一次，生产数据没被动过。

**方案 C + replay Pod**：

```text
1. kubectl apply 一个 replay Pod
   ├─ 参数：replayRunId=r20260724, sourceProtocolKey=xxx,
   │        rawRange=<offset范围>, targetCodecRelease=v2, targetRelease=RESTORE_ORIGINAL
   ├─ 用独立 consumer group（replay-r20260724）读 Raw，不动生产 group 的 offset
   ├─ 解码后所有 writer 目标加前缀：replay_r20260724_<原表名>
   └─ outbox 关闭（或写隔离 outbox，不外发）

2. 重解码产物全部落 replay_r20260724_ 隔离表

3. 验证：对比 replay_r20260724_ 与生产表的差异，确认新值正确

4. 合并：把 replay_r20260724_ 的 history 用 TDengine upsert 合并进生产表
   （alarm/outbox 状态类要按幂等键去重合并，不能简单覆盖）

5. kubectl delete replay Pod，清理 replay_r20260724_ 隔离表
```

期间在线数据照常进生产表，互不干扰；算错了在第 3 步就能发现，删 Pod + 删隔离表即可，生产数据没动过。

#### 与方案 A 的真实差距（加上 replay Pod 后）

诚实讲，加上 replay Pod 后，两者在"重放能力"上**基本拉平**。差距收敛到两条运维复杂度，不是能力有无：

1. **合并进生产表是额外 ETL 步骤**。A 的隔离 namespace 有正式"晋升"契约（写隔离 → 验证 → 审批 → 合并）；C 的隔离是存储层前缀，合并进生产表是一次额外操作。
   - 简单情况（纯 history 覆盖同时间戳）：TDengine upsert 即可，几乎无额外成本；
   - 复杂情况（alarm 状态、outbox 已触发）：要按幂等键去重合并，不能简单覆盖。这是真实差距，但绝大多数 history 回填走 upsert 就够了。
2. **C 本身没有隔离②（解码↔下游）**。这条与重放无关，是 C 原本的取舍。replay Pod 解决的是"重放"的隔离，不解决"在线消费"的隔离。

#### 硬前提：Raw retention 要覆盖回填窗口

这个 SOP 有一个硬前提：**Raw 72h + 对象归档要覆盖你想回填的时间窗口**。

- 想修一周前的 codec bug？Raw 还在，直接拉 Pod 消费即可，很轻。
- 想修一个月前的 bug？Raw 72h 早删了，**必须先从对象归档（30/90 天）恢复 Raw** 到 replay topic，再让 Pod 消费。归档恢复本身有成本（offset-range manifest、checksum、解密），不是 kubectl up 就能跑的。

**所以 C + replay Pod 的回填时间窗口上限 = 归档保留期。这条 A 也一样，不是 C 特有的限制。**

#### 结论

| 重放形态 | 能力 | 谁提供隔离 |
| --- | --- | --- |
| C 裸奔（进程内分发，无 Pod） | 能重解码 Raw，但产物直接灌生产表，**不安全、不可验证、不可撤销** | 无 |
| **C + 临时 replay Pod（推荐）** | **独立 group + 隔离表 + 验证后合并——与方案 A 基本等价** | **Pod + 存储层表前缀（替代 Canonical topic）** |
| A（有 Canonical replay topic） | 独立 replay topic + 隔离 namespace + 正式晋升契约 | Kafka 层原生 |

**C + replay Pod 在重放能力上和 A 基本拉平，差的是"合并进生产表的 ETL"和"在线消费的独立隔离"这两条运维复杂度，不是能力有无。** 对绝大多数工业回填场景（纯 history 覆盖），这个差距完全可接受。

---

## 5. 横向对比矩阵

### 5.1 能力对照

| 维度 | A 双 Topic | B 解码前置 | C Raw + 进程内分发 |
| --- | --- | --- | --- |
| 入口↔解码隔离（设备一直能传） | ✅ Raw | ❌ 解码在入口路径 | ✅ Raw |
| 解码↔下游隔离（下游挂解码照跑） | ✅ Canonical | ❌ | ❌（靠 Raw 上游缓冲近似） |
| 集中补传洪峰有界吸收 | ✅ Raw | ❌ 无缓冲层，累积压力回压设备 | ✅ Raw |
| codec 崩溃不影响入口 | ✅ | ❌ 断流 | ✅ |
| codec 灰度/回滚不动入口 | ✅ shadow group | ❌ 换入口逻辑 | ✅ |
| decode-once 多消费者独立 offset | ✅ 各 group | ✅（Canonical 多 group） | ❌ 进程内共享 |
| 跨进程独立扩缩消费者 | ✅ | ✅ | ❌ |
| REPROCESS 历史重算（新 codec 回填） | ✅ 从 Raw 重算，有隔离 namespace | ❌ 无 Raw | ✅ + replay Pod（存储层隔离，能力基本等价，见 §4.6） |
| 原始报文证据留存 | ✅ Raw 72h+归档 | ❌ | ✅ Raw 72h+归档 |
| 每条 Raw 可审计确定性结局 | ✅ 事务保证 | ❌ | ❌ 靠幂等近似 |
| 写放大/复制开销 | 最高（2×写+6×复制） | 最低（1×写+3×复制） | 中（1×写+3×复制，但保留 Raw） |
| Kafka transaction 复杂度 | 高 | 无 | 无 |
| 运维负担 | 最高 | 最低 | 中 |
| 与主流云厂商生态对齐 | 否 | 是 | 部分 |

### 5.2 故障故事对照（最能说明问题的一张表）

> 每行是一个真实故障，看三种方案各自会发生什么。

| 故事 | A 双 Topic | B 解码前置 | C Raw + 进程内分发 |
| --- | --- | --- | --- |
| **codec panic/OOM** | 该 protocolKey 解码停，Raw 堆积，**其他设备入口无感** | **该 protocolKey 设备数据断流**，设备传不上来 | 该 protocolKey 解码停，Raw 堆积，**入口无感** |
| **TDengine 慢 30 分钟** | Materializer group 落后，Canonical 缓冲，解码照跑，30 分钟内追平 | 解码被回压 → 回压到设备 | 解码进程内队列满 → pause Raw partition，Raw 承接积压，追平 |
| **集中补传洪峰**（万台设备同时补传） | Raw 有界吸收，解码按节奏追 | **无缓冲层，累积解码压力直接回压设备**（单条几 KB 不打满 CPU，但并发量大且无可吸收层） | Raw 有界吸收，解码按节奏追 |
| **上线 1 月后发现 codec 算错单位** | 从 Raw 按原 release 重算 Canonical，修正历史 | **历史数据将错就错，无法回填** | 从 Raw 重解码 + replay Pod 隔离重放，能力与 A 基本拉平（见 §4.6） |
| **收到一条未知 payload 炸弹** | 原始字节进 Raw，解码失败进 quarantine，可日后重放 | **解码失败 = 丢数据**（无处存原始字节） | 原始字节进 Raw，解码失败进进程内 DLQ |
| **升级 codec 想灰度** | shadow group 独立消费 Raw，只写隔离 namespace | 换入口逻辑，**必须停入口或冒不一致风险硬切** | 解码在 Raw 之后，可独立灰度 |
| **MQTT 投递确认 ↔ 持久化非原子窗口** | Raw 切成两段，各自幂等兜底 | 窗口暴露在入口，重启丢"已解码但未持久化"的批 | Raw 切成两段，各自幂等兜底 |
| **某消费者（如 Audit）永久落后** | 它的 group 落后，不影响其他消费者 | 取决于实现 | **拖累同进程其他消费者**（进程内队列背压） |

这张表的核心结论：**B 在 codec 出问题和补传洪峰这两类故障上会直接背叛"设备一直能传"的需求；A 和 C 都不会。**

### 5.3 与契约密度的关系（避免混淆）

[README.md](./README.md) 讨论的"五项契约密度"（outputs+offsets 事务、`NO_OUTPUT`、分级 quarantine、release timeline、ProcessingProvenance）是**正交的另一维度**：

| | 契约密度全开 | 契约密度简化（at-least-once + 幂等） |
| --- | --- | --- |
| **方案 A（双 Topic）** | 主架构当前冻结形态 | 可行（Canonical topic 还在，但不走 CTP transaction） |
| **方案 B（解码前置）** | 不适用（无 Raw，契约无处生根） | ThingsBoard/JetLinks/国内云厂商的实际形态 |
| **方案 C（Raw + 进程内）** | 不适用（无 Canonical topic，事务契约无处生根） | **本文推荐的中档**：Raw 保留，契约简化 |

也就是说：选完跳数（A/B/C），还要再选一次契约密度（全开/简化）。**两者是两个独立开关。**

---

## 6. 决策框架

### 6.1 两个独立问题，分别回答

**问题 1：解码边界放哪？（选 A/B/C）**

```
你需要"设备一直能传、解码挂了入口不死"吗？
├─ 否（协议标准、JSON、解码不会出问题） → 方案 B
└─ 是（工业、多厂商、codec 会变）
   ├─ 还需要"修 codec 后回填历史 + 跨进程多消费者独立扩缩"吗？
   │  ├─ 是 → 方案 A
   │  └─ 否 → 方案 C
```

**问题 2：契约密度开多少？（见 README §6 分期建议）**

```
你把设备数据当"账"还是"观测"？
├─ 账（错一条有真金白银/合规后果） → 契约密度全开（V1 概念层冻结，GA 自动化补全）
└─ 观测（丢一个温度点无感） → 契约密度简化（at-least-once + 幂等 sink）
```

### 6.2 三种典型客户画像的推荐组合

| 客户画像 | 跳数 | 契约密度 | 理由 |
| --- | --- | --- | --- |
| 能源计量/工业告警/合规审计 | A | 全开 | 数据是账，codec 会修，要多消费者独立扩缩 |
| 多厂商工业设备、试点期 | C | 简化 | 要入口高可用，但试点不需要回填历史，避 transaction 复杂度 |
| 消费级/标准协议/SaaS | B | 简化 | 协议标准，解码不出问题，成本最低 |

### 6.3 演进路径（重要）

三种方案**不是互斥的终态，是一条演进链**：

```text
B（解码前置）
 │  └─ 加 Raw → 升级到 C（最常见的第一步：发现解码挡了入口）
 │
 C（Raw + 进程内分发）
 │  └─ 加 Canonical topic → 升级到 A（需要 REPROCESS 或多消费者独立扩缩时）
 │
 A（双 Topic）
```

- **C 是 A 的真子集**：C 的 Raw 契约与 A 完全相同，将来加一个 Canonical topic 即可升级，无需返工。
- **B 到 C 是有代价的迁移**：要把解码从入口路径挪到 Raw 之后，重新部署。
- **C 到 A 的触发条件**：不是"需要重放"（C + replay Pod 已能重放，见 §4.6），而是"需要解码↔下游的在线隔离"或"需要跨进程多消费者独立扩缩"。
- **建议**：如果拿不准 A 还是 C，先上 C。C 反悔成本低（加 topic 即可），A 反悔也只是"多用了一层"。**从 B 起步反悔成本最高**（要重构入口）。

---

## 7. 对"上一轮对话"那个具体问题的直接回答

> 问："云端要 99.99 能连，设备要能一直上传，数据可以暂时不处理但要尽快追回来——这个需求下，把 ProtocolApplication 提前到 EMQX 之后、Kafka 之前（方案 B）可行吗？"

**对这套需求，方案 B 不是合适选择**（注意：不是说 B 不可靠，而是 B 的可靠边界不覆盖这套需求，理由见 §3.4）。理由：

1. 方案 B 把解码放进入口路径，codec 任何问题（panic/OOM/坏 payload）都会直接断该 protocolKey 的设备上传——违反"设备一直能传"。
2. 方案 B 没有缓冲层，万家设备并发补传的累积解码压力无处吸收，会回压到设备——违反"99.99 能连"。（注：单条几 KB 不会打满 CPU，但并发量大且无可吸收层时压力会传导。）
3. "数据暂时不处理但要追回来"需要的是**入口↔解码隔离**，方案 B 删掉的正是这层隔离。

满足该需求的最低方案是 **C（Raw + 进程内分发）**：用 Raw 兜住"一直能传 + 暂时不处理"，用幂等键兜底"追回来"。C 自身就能通过临时 replay Pod 实现修 codec 后回填历史（见 §4.6），不需要立刻升级到 A。只有当你还需要"解码↔下游隔离"或"跨进程多消费者独立扩缩"时，才需要加 Canonical topic 升级到 A。

**方案 A 的全部契约密度（CTP transaction、release timeline、replay matrix）不是为"一直能传 + 追回来"这套需求准备的——它们是为"重新解释历史 + 每条 Raw 可审计结局 + 多消费者独立"准备的。** 这套需求用不到它们全部，C 就够了。

---

## 8. 与主架构、README 的关系

- [主架构](../enterprise-iot-platform-architecture-design.md) §4.2/§7 冻结的是**方案 A + 契约密度全开**。本文不修改该冻结项。
- [README.md](./README.md) 回答"方案 A 的契约密度买什么、何时付、能否分期"。本文回答"**该不该选方案 A 这个跳数**"。
- 本文 §6.3 的演进路径中，**方案 C 涉及主架构 §1 第 7 条与 §7.2"生产语义始终经 Kafka transaction"的冻结项**。若采纳 C 作为 V1 试点形态，必须走正式变更评审，不允许以"试点简化"名义静默偏离。
- 本文不构成对主架构的修订，仅为选型提供横向对比与决策框架。
