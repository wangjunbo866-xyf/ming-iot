# EMQX vs mqtt-gateway：全模块对比与可借鉴清单

> 本文系统对比 EMQX（业界最强通用 MQTT broker）与 mqtt-gateway（专用上行网关 + 低频下行控制）的 8 个核心模块，识别 mqtt-gateway 可借鉴的工程基础设施，以及不该借鉴的通用 broker 能力。
> 对比基线：EMQX 源码 `/home/wangjunbo/workspace-iot/emqx`（apps/emqx, apps/emqx_auth, apps/emqx_retainer 等）。
> 配套文档：[架构设计](../1.cloud-mqtt-gateway-architecture.md)、[实现红线](../7.cloud-mqtt-gateway-implementation-red-lines.md)、[Zilla 对比报告](./aklivity-zilla-comparison-report.md)、[认证设计 Review](./connect-auth-design-review.md)。

---

## 1. 判据

```text
mqtt-gateway 是「专用上行网关 + 低频下行控制」，不是通用 broker。
所以「EMQX 某模块是否值得借鉴」取决于 mqtt-gateway 是否需要那个能力：
  - 通用 broker 能力（订阅树/fan-out/retain/shared sub）→ 不借鉴，违背定位
  - 工程基础设施（可观测性/过载保护/trace/热路径优化）→ 借鉴，这是 mqtt-gateway 薄弱环节
  - 架构选型差异（集群复制/takeover/hooks）→ 记录，非优劣
```

---

## 2. 八模块对比

### 模块 1：连接管理（Channel / Session / CM）

| 维度 | EMQX | mqtt-gateway |
|---|---|---|
| Session 存哪 | mnesia 全集群复制（persistent_session_ds）或内存（session_mem） | 本地内存（pointer-poor，红线 §4） |
| 全局注册 | `emqx_cm_registry`（mria 全集群表）| 无，靠 presence（Redis）+ fencing |
| 重连 takeover | 有（跨节点迁移/踢旧 session）| 无，设备重连靠 presence 重定向 |
| 心跳/keepalive | session 进程内 timer | 规划中（LastSeenUnix 字段已预留） |

**借鉴判定：架构选型差异，不借鉴 takeover**。mqtt-gateway 的 presence+fencing 在下行延迟、故障域隔离、扩缩容无 rebalance 上更优（详见 [Zilla 对比](./aklivity-zilla-comparison-report.md)）。

### 模块 2：消息路由（Routing / PubSub）

| 维度 | EMQX | mqtt-gateway |
|---|---|---|
| 订阅树 | ETS `ordered_set` + 跳跃式前缀搜索（`emqx_trie_search.erl`），O(N·L) N=命中数 | **无**（架构 §7.6 否定每设备内部订阅） |
| fan-out | O(命中数) 遍历 + shard 分片异步投递 | **无**（single-sink → Kafka） |
| shared sub（$share） | 7 种选组策略（random/RR/sticky/hash...） | **无**（架构明确否定） |
| retain | 可插拔后端（mnesia/外部） | **无**（第一阶段砍掉） |
| 跨节点路由 | topic→node 路由表，mria 复制，RPC 转发 | **无**（broker 间不共享状态） |

**借鉴判定：完全不适用**。这些是通用 broker 的核心能力。mqtt-gateway 作为接入网关学它等于走错方向。订阅匹配/fan-out/retain/shared sub 全部违背"极简接入层"定位。

### 模块 3：认证与授权

| 维度 | EMQX | mqtt-gateway |
|---|---|---|
| 认证链 | authn chain（多后端可插拔，first-match-stop） | 直连 PG → Auth Service（Phase 2） |
| 授权链 | authz chain（file/http/mnesia sources） | 本地 ACL 规则（acl.go） |
| authn 缓存 | 单层 per-node ETS（1min TTL，详见 [认证 Review §4.2.1](./connect-auth-design-review.md)） | L1（otter）+ L2（Redis 共享） |
| authz 缓存 | 进程字典 per-connection（`emqx_authz_cache`）+ monotonic 时间戳全局失效 | 无 |
| flapping 检测 | 有（`emqx_flapping.erl`，认证前 ban） | **缺**（[认证 Review 缺口 1](./connect-auth-design-review.md)） |
| banned 黑名单 | 有（`emqx_banned.erl`，多维封禁，认证前检查） | **缺**（[认证 Review 缺口 2](./connect-auth-design-review.md)） |

**借鉴判定：flapping + banned 已在认证 Review 识别为缺口**（详见该报告 §3）。L1+L2 分层已优于 EMQX 单层 ETS。

### 模块 4：消息队列 / inflight

| 维度 | EMQX | mqtt-gateway |
|---|---|---|
| 离线队列（mqueue） | 内存优先级队列（`emqx_mqueue.erl`），重启即丢，满则丢最旧，QoS0 默认不入队 | 无（上行直接 Kafka durable；下行靠 Command Service 状态机） |
| inflight window | `gb_trees`（key=PacketId），默认 32，MQTT5 用 receive_maximum | 上行 QoS1 inflight=1（天然有序）；下行 `[4]DownlinkSlot` 预留 |
| 重传 | `retry_interval` 定时器（默认 infinity 不重传） | 上行不重传（Kafka durable 即成功）；下行规划中 |

**借鉴判定：基本不适用**。上行 inflight=1 天然无需 window 管理。**唯一参考价值**：下行 QoS1 inflight 实现时（红线 §4.4 的 `[4]DownlinkSlot`），可借鉴 EMQX 的 inflight 相位管理（wait_ack 阶段 + 超时重传），是未来工作非现在。EMQX 的"QoS0 默认不入队"印证了 mqtt-gateway 的 QoS0 fire-and-forget 语义。

### 模块 5：集群 / 分布式

| 维度 | EMQX | mqtt-gateway |
|---|---|---|
| 状态复制 | mria（mnesia 增强）全集群复制：session/路由/banned/shared sub | 无（broker 间不共享状态） |
| 集群发现 | ekka（manual/k8s/dns/mcast/etcd） | NLB（无集群发现，节点独立） |
| 跨节点协调 | 全局注册表 + takeover RPC | presence（Redis）+ fencing（CAS） |

**借鉴判定：架构选型差异，不借鉴**。EMQX 的 mria 集群让节点"共享大脑"但代价是写放大 + 一致性约束（脑裂处理复杂）。mqtt-gateway 无状态 + 外置状态（Redis）更适合接入层定位。详见 [Zilla 对比 §4](./aklivity-zilla-comparison-report.md#4-适用性结论为什么-mqtt-gateway-保持-registry-as-index)。

### 模块 6：限流与背压

| 维度 | EMQX | mqtt-gateway |
|---|---|---|
| 连接限速 | `emqx_ranch_limiter`（acceptor 层，max_connections + 速率） | auth queue 限流（§6.1）+ bounded accept |
| per-client 限流 | flapping 检测（CONNECT 频率） | per-client 预算（已落地，`maxPendingBytesPerClient`） |
| 背压传导 | mqueue 满丢消息 + delivery.dropped 指标 | QoS0 drop / QoS1 断连 + ingest queue 预算 |
| **统一过载框架** | **olp 模块**（delay/hibernation/gc/new_conn 统一决策 + 指标） | **散件，缺统一框架**（[可借鉴 ⑤](#5-统一过载保护框架--olp-指标)） |

**借鉴判定：统一过载保护框架值得借鉴**（见下文 ⑤）。per-client 预算 mqtt-gateway 已实现且比 EMQX 更细（EMQX 靠 flapping 间接防，我们是显式字节预算）。

### 模块 7：可观测性

| 维度 | EMQX | mqtt-gateway |
|---|---|---|
| 指标体系 | 十大类上百原子计数（client/message/delivery/session/authn/authz/**olp**）| 红线 §13 有但不系统 |
| 实现 | `counters`（atomic）+ `persistent_term` + inline 热路径 | 规划中 |
| Prometheus | 完整 exporter（vm/cluster/packet/message/delivery/client/session/olp/acl/authn） | 规划中 |
| trace | `emqx_trace`（clientid/topic/ip/rule 维度，落 trace 文件） | **缺**（下行诊断靠日志拼） |

**借鉴判定：最高优先级**（见下文 ①④）。EMQX 的指标分类（尤其 olp + authn cache hit/miss + delivery.dropped 子因）是 mqtt-gateway 最该补的眼睛。

### 模块 8：性能优化

| 技巧 | EMQX | mqtt-gateway 对应/借鉴价值 |
|---|---|---|
| `persistent_term` 存只读常量 | 几十处（hooks/metrics ref/limiter/trace filter/路由 schema） | Go 的 `atomic.Pointer` / 全局只读变量，[见 ②](#2-persistent_term--热路径零配置查找) |
| `binary:copy` 防 sub-binary 持有 | `emqx_frame.erl:840`（大 payload copy，小的不 copy） | 红线 §5.2 已防（exact-size 独占分配）；未来引入大 read buffer 时的护栏 |
| ranch acceptor 池（默认 16 acceptor） | Erlang 进程模型需要 | **不适用**（Go epoll 单 acceptor goroutine 足够） |
| 进程池 / worker pool | broker_pool / router_pool（gproc_pool） | Go 用 goroutine + channel，模型不同 |

---

## 3. 可借鉴清单（5 项，按优先级）

### ① 可观测性指标体系（高优先级，Phase 1）

EMQX 十大类指标中，mqtt-gateway 最该补的是三类：

| 指标类别 | EMQX 具体指标 | mqtt-gateway 用途 |
|---|---|---|
| **olp**（过载保护） | delay/hibernation/gc/new_conn 计数 | 验证 ingest queue 满/per-client 预算超限/auth queue 满时系统正确降级 |
| **authn/authz cache** | success/failure + **cache_hit/cache_miss** | 验证 L1/L2 有效性（命中率是核心验证项，[认证 Review §4.2.1](./connect-auth-design-review.md)） |
| **delivery.dropped 子因** | no_local/too_large/expired/filter/**queue_full** | 定位"消息为什么丢了"——区分全局 queue 满 vs per-client 超限 vs 超时 |

**实现方式**：Go 用 `sync/atomic` 的 `atomic.Int64` 计数器（等价 EMQX 的 `counters`），Prometheus exporter 用 `prometheus/client_golang`。热路径 inc/dec 内联（等价 EMQX 的 `-compile({inline,...})`）。

### ② persistent_term → 热路径零配置查找（中优先级，Phase 1）

EMQX 用 `persistent_term` 存只读配置（hooks 表/metrics ref/limiter group），避免热路径的 ETS/进程字典查找。

**Go 对应**：`atomic.Pointer[T]` 加载的全局只读变量。mqtt-gateway 的热路径（readLoop → 解析 → Kafka produce）每次查的配置（broker_id、max_packet_size、publish limits、Kafka topic 名、ACL 规则表），应固定到全局只读，避免间接寻址。当前 `server.Config` 值传递已接近，但 ACL 规则表/Kafka topic 映射可更系统化。

### ③ binary:copy 阈值机制（中优先级，未来护栏）

EMQX 对大于 `?SMALL_BINARY` 的 payload 做 `binary:copy`，把 sub-binary 实体化防引用膨胀；小的不 copy 省开销。

mqtt-gateway 当前 payload 是 exact-size 独占分配（`make([]byte, remaining)`），不存在 sub-binary 问题（红线 §5.2）。**但若未来引入大 read buffer（NATS 式 chunked read），就必须仿照这个阈值 copy 机制**——这是护栏，不是现在的改动。详见 [NATS 报告 §5.2](../nats/nats-server-buffer-research-report.md)。

### ④ 下行 trace 消息追踪（中高优先级，Phase 2）

EMQX 的 `emqx_trace`：按 clientid/topic/ip/rule 维度过滤，PUBLISH/SUBSCRIBE 事件落 trace 文件，filter 条件缓存到 persistent_term 便于热路径判断是否激活。

mqtt-gateway 的下行链路（Command Service → Broker → 设备）跨多组件，没有 trace 就只能靠日志拼。逆变器控制指令（限功率/OTA）如果"发了没生效"，运维需要能追踪。

**实现方式**：Phase 2 做 per-(tenant,device) 事件日志（命令下发/ack/超时/重试），存 Redis 或独立 Kafka topic。不需要 EMQX 那么重（独立 trace 文件 + formatter）。

### ⑤ 统一过载保护框架 + olp 指标（中优先级，Phase 1）

EMQX 的 olp 模块统一管理过载降级（delay/hibernation/gc/new_conn）+ 对应指标计数。

mqtt-gateway 当前过载决策散落代码：auth queue 满拒连、ingest queue 满 drop、per-client 预算超限——各自处理，没有统一决策点和指标。应有统一的 `olp.OnPressure()` 决策函数 + 指标计数，让运维从 metrics 一眼看出"系统在过载降级"。mqtt-gateway 不需要 EMQX 的 hibernation（Erlang 进程特有），但"统一过载决策 + 指标"值得学。

---

## 4. 不借鉴清单（通用 broker 能力，违背网关定位）

| 项 | 理由 |
|---|---|
| 订阅树 / 通配符路由 | 架构 §7.6 明确否定；mqtt-gateway 下行是定向投递不做通配匹配 |
| fan-out 分发 / shared subscription | single-sink → Kafka，无 fan-out；$share 架构明确否定 |
| retain 消息存储 | 第一阶段砍掉；设备配置/影子走下游服务 |
| mqueue 离线队列 | 上行直接 Kafka durable；下行靠 Command Service 状态机 |
| 复杂 inflight window（上行） | QoS1 inflight=1 天然有序，无需 gb_trees 管理 |
| mria 全集群复制 | 无状态 + presence 外置更适合接入层 |
| takeover | presence + fencing 更优（[Zilla 对比](./aklivity-zilla-comparison-report.md)） |
| hooks 可插拔流水线 | 通用平台需要插件生态；mqtt-gateway 硬编码流水线更简单快 |
| ranch acceptor 池 | Go epoll 模型与 Erlang 进程模型根本不同 |

---

## 5. 决策记录

| 借鉴项 | 优先级 | 阶段 | 关联文档 |
|---|---|---|---|
| ① 可观测性指标体系（olp/authn cache/delivery 子因） | 高 | Phase 1 | 红线 §13 |
| ② persistent_term → atomic.Pointer | 中 | Phase 1 | — |
| ③ binary:copy 阈值机制 | 中 | 未来护栏 | NATS 报告 §5.2 |
| ④ 下行 trace | 中高 | Phase 2 | 架构 §9 |
| ⑤ 统一过载框架 + olp 指标 | 中 | Phase 1 | 红线 §5.5/§6 |
| flapping + banned | 中 | Phase 1/2 | 认证 Review 缺口 1/2 |

```text
核心结论：EMQX 最值得借鉴的不是 broker 能力（订阅/分发/retain/集群复制——mqtt-gateway
明确不要），而是工程基础设施——可观测性指标体系、统一过载保护、下行 trace、热路径优化。
这五项是 mqtt-gateway 当前最薄的环节。Phase 1 补①⑤（指标+过载框架），Phase 2 补④（trace），
②③ 持续优化。通用 broker 核心能力不借鉴——学它等于把接入网关变成通用 broker，走错方向。
```

---

## 参考资料

- [架构设计](../1.cloud-mqtt-gateway-architecture.md)
- [实现红线](../7.cloud-mqtt-gateway-implementation-red-lines.md)
- [容量规划](../11.cloud-mqtt-gateway-capacity-planning.md)
- [Zilla 对比报告](./aklivity-zilla-comparison-report.md)
- [认证设计 Review](./connect-auth-design-review.md)
- EMQX 源码关键文件：
  - 指标：`apps/emqx/src/emqx_metrics.erl`、`apps/emqx_prometheus/src/emqx_prometheus.erl`
  - 过载：`apps/emqx/src/emqx_olp.erl`
  - trace：`apps/emqx/src/emqx_trace/emqx_trace.erl`
  - 路由：`apps/emqx/src/emqx_router.erl`、`apps/emqx/src/emqx_trie_search.erl`
  - 性能：`apps/emqx/src/emqx_frame.erl`（binary:copy）、`apps/emqx/src/emqx_limiter/`
  - 队列：`apps/emqx/src/emqx_mqueue.erl`、`apps/emqx/src/emqx_inflight.erl`
