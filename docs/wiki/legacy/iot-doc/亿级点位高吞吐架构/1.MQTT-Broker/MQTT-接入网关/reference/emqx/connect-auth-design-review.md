# CONNECT 认证设计 Review（对标 EMQX）

> 本文是对 [CONNECT 认证设计](../5.cloud-mqtt-connect-auth-design.md) 的 review，结合 [EMQX](https://github.com/emqx/emqx) 源码实现做对比，识别缺口与商榷点。
> Review 日期：2026-06-19。EMQX 源码版本：`/home/wangjunbo/workspace-iot/emqx`（apps/emqx_auth、apps/emqx/src/emqx_auth_cache、emqx_flapping、emqx_banned、emqx_cm_registry）。

---

## 1. 结论

认证设计文档整体质量高。在以下三个维度**优于 EMQX**：

```text
1. 缓存分层：L1（本地短 TTL）+ L2（共享长 TTL）两级，EMQX 只有单层 per-node ETS，无跨节点复用。
2. 负向缓存安全：§5.2 明确禁止 bad credential 入负向缓存（防定向 DoS），分析透彻。
3. 容量推导：§5.5.2 给出 L1 ≈ connect_rate × TTL 的公式和量级估算，EMQX 只靠 ETS max_count 硬限。
```

但对比 EMQX 后发现 **2 个防爆盲区**（flapping / banned）和 **2 个落地缺口**（即时失效手段 / 极端窗口容量）。详见 §3。

---

## 2. EMQX 认证实现摘要（对比基线）

### 2.1 认证缓存：单层 per-node ETS，不跨节点

| 维度 | EMQX authn cache | 本文档 L1 | 本文档 L2 |
|---|---|---|---|
| 实现 | ETS 表 `emqx_node_cache`（per-node，无 rlog 复制） | otter（per-process，S3-FIFO） | Redis/Tair（共享） |
| 跨节点 | **否**（ETS 仅限本节点） | 否 | **是** |
| TTL | 1min（`cache_ttl`） | 5~30min（PositiveTTL） | ≥24h（跨隔夜空窗） |
| 容量 | max_count=100万 / max_memory=100MB | MaxEntries（窗口有界） | ≥ fleet |
| 缓存失败结果 | **否**（`{nocache, Error}`） | 否（§5.2 安全约束） | 否 |
| cache key | `[id, sha256(含 password 的变量)]` | `auth:{client_id}`（不含密码，命中仍校验 secret） | 同 L1 |
| 即时失效 | 手动 REST `POST /authentication/node_cache/reset`（erpc 多节点清表） | 无（靠 TTL） | 无（靠 TTL + Phase 2 事件） |

**关键事实**：EMQX 的 authn cache 表名是 `emqx_node_cache`（注意 "node"），`ets:new` 创建、**无 rlog_shard、不跨节点同步**。与认证用户表（`emqx_authn_mnesia`，有 `rlog_shard = ?AUTHN_SHARD`，集群复制）完全不同。

### 2.2 EMQX 集群连接路由：全局 session 注册 + takeover，但无粘性

EMQX 集群也有 LB 随机分发、设备重连可能落不同节点的问题，但它用**全局 session 注册表**（`emqx_cm_registry`，mria 全集群表）+ **takeover**（`emqx_cm:takeover_session_begin` → `pick_channel` → `lookup_channels`）解决 **session 唯一性**——同一 clientid 集群内只有一个有效 session，重连到别处会跨节点迁移/踢掉旧 session。

但这**不解决认证缓存复用**。设备从节点 A 重连到节点 B，节点 B 的 authn cache 为空（不互通），照样查后端。EMQX 对此无优化，靠超短 TTL（1min）+ 巨大 ETS 硬扛。

### 2.3 EMQX 防爆机制（我们缺失的）

| 机制 | EMQX 模块 | 执行时机 | 我们的对应 |
|---|---|---|---|
| **flapping 检测** | `emqx_flapping.erl`：窗口内同 clientid CONNECT 次数超阈值 → ban | 认证**之前**（`emqx_channel.erl:624`） | **缺失** |
| **banned 黑名单** | `emqx_banned.erl`：clientid/username/peerhost/正则/CIDR 维度封禁 | 认证**之前**（`emqx_channel.erl:623`） | **缺失** |
| acceptor 连接限速 | `emqx_ranch_limiter.erl`：max_connections + 连接速率 | socket accept 阶段 | auth queue 限流（§6.1） |
| 失败登录锁定 | **无**（EMQX 只打 throttled 日志，不退避/不锁定） | — | §5.2 throttle（未落地） |

EMQX 的 CONNECT 流水线顺序（`emqx_channel.erl:609-648`）：
```
overload_protection → ... → check_banned → count_flapping → authenticate → ...
```
**封禁和 flapping 计数都在认证之前**——恶意设备不消耗认证资源。

---

## 3. 缺口与商榷点

### 3.1 缺口 1（中）：无 flapping 检测 —— EMQX 有

**现状**：§6.1 列了"每 device_id 同时认证 inflight=1"，但这只防并发不防高频串行重连。一个故障设备每秒断连重连 10 次，inflight=1 挡不住（每次释放再占）。这正是逆变器场景的真实问题：移动网络抖动导致 flapper。

**EMQX 做法**：`emqx_flapping.erl` 统计同一 clientid 在 `window_time` 内的 CONNECT/断连次数，超 `max_count` 就加入 banned 表 ban `ban_time` 秒。在认证之前执行。

**建议**：新增 flapping 检测层。窗口内同一 `(tenant_id, device_id)` 的 CONNECT 次数超阈值 → 返回 Server Unavailable（让设备退避）。与 auth cache 独立，避免冲刷凭证缓存。**Phase 1 落地**。

> **已在 [网络攻防 §5.3](../../network-defense/9.cloud-mqtt-gateway-network-attack-defense.md#53-凭证攻击执行点a5本文新增的主要机制) 展开**：flapping 检测作为 §5.3 自动封禁规则第四条（热路径实时，区别于 §6.3 离线行为基线）。借鉴 EMQX `emqx_flapping` 默认参数（window_time=1min / max_count=15 / ban_time≈5min），针对逆变器 4G 蜂窝网络抖动场景。关键点：flapping 的认证可能都成功（密码没错），不能用"认证失败计数"检测，必须单独统计 CONNECT/断连事件。

### 3.2 缺口 2（中）：无 banned 黑名单机制 —— EMQX 有

**现状**：§6.1 只提"错误凭证速率"限流，无主动封禁。遇到恶意网段扫描，没有手段在不改认证表的情况下快速封禁——只能靠 throttle 被动限速。

**EMQX 做法**：`emqx_banned.erl` 支持 clientid/username/peerhost/正则/CIDR 多维度，运维可手动 ban 一个恶意 IP 段，认证之前检查。

**建议**：Phase 2 加 banned 机制（至少 peerhost/CIDR 维度），CONNECT 校验阶段、认证之前检查。Phase 1 可用防火墙/iptables 顶。

### 3.3 缺口 3（低）：凭证轮换无即时失效手段

**现状**：§5.3 说"首版可接受短暂禁用延迟"，即时失效推到 Phase 2（凭证变更事件推送）。但缺一个"紧急情况手动清缓存"的运维手段——§9 说"DB 不可用但正向缓存未过期可允许已缓存设备登录"，反过来需要**立即**踢掉一个已缓存的危险设备时，当前无 API。

**EMQX 做法**：REST API `POST /authentication/node_cache/reset` → `erpc:multicall` 清所有节点 ETS。

**建议**：Phase 1 加运维级缓存清空 API（手动清单节点 L1），不需要 EMQX 那么重（erpc 多节点）。

### 3.4 缺口 4（低）：CONNECT 校验流水线顺序未明确

**现状**：§3.1 主流程是线性的（基础校验 → cache → queue → DB → presence → CONNACK），未明确 flapping/banned 在哪一步。

**EMQX 做法**：流水线顺序明确，封禁/flapping 在认证之前。

**建议**：在 §3.1 主流程预留位置，标注"未来 flapping/banned 在 cache 查询之前"。即使机制未实现，顺序契约先定下。

### 3.5 商榷 1：§5.2 的 throttle 未落地

**现状**：§5.2 提到 `(client_id, source_ip)` 维度的 throttle 防暴破，但 §6.1 和代码里都未实现。攻击者用海量随机 password 打同一真实 client_id（暴力破解），当前只能靠限频，而限频代码不存在。

**EMQX 做法**：根本不缓存失败结果，靠 flapping + banned 间接防。

**判断**：非设计问题，是落地缺口。throttle 需补，和 ingest per-client 预算同类。

### 3.6 商榷 2：L2 与 presence 共用 Redis 的极端窗口容量

**现状**：§5.6.4 说"L2 复用 presence 的 Redis，不引入新硬依赖"。§5.6.2 示例算出 Redis QPS ≈ 3λ = 5000 op/s（D=1M, W=600s）。但若 W=60s（1 分钟集中上线），λ=16667/s，Redis QPS ≈ 50000/s——单实例开始有压力。

**建议**：§5.6.2 补一行"极端窗口 W=60s 时 Redis QPS ≈ 50000/s"的估算，容量规划按最密集窗口而非乐观窗口。

---

## 4. L2 必要性论证：无粘性拓扑的结构性约束

> 本节回应"L2 是否过度设计"的潜在质疑。结论：L2 是无粘性拓扑下保护 DB 的必需层，非优化项。

### 4.1 拓扑约束

```text
mqtt-gateway 多节点无状态，前端 NLB 随机分发，无连接粘性。
  → 设备每次重连大概率落不同节点，新节点 L1 正向缓存为空。
  → L1 正向命中被无粘性稀释，主要职责退化为：负向洪泛闸 + 就地 flapper（§5.5.1）。
  → 跨节点的正向复用只能由共享 L2 承担。
```

这不是"日出冷启动"才有的问题——**稳态下无粘性同样让 L1 正向命中率低**。日出只是把这个问题放大到 λ 的峰值。

### 4.2 与 EMQX 的拓扑对比

| 维度 | EMQX | mqtt-gateway |
|---|---|---|
| 连接路由 | 全局 session 注册 + takeover（session 唯一性） | 无（presence + fencing 是下行路由层，与认证缓存无关） |
| LB 粘性 | 无（LB 随机分发，设备可能落任意节点） | 无（NLB 随机分发） |
| 认证缓存跨节点 | 否（per-node ETS） | 否（per-node L1）+ **是（共享 L2）** |
| 认证后端 | 两种模式：内建库 mnesia（集群内存复制）/ 外部数据源（pg/mysql/redis） | 只读 PostgreSQL 副本 |
| 为什么 L2 不需要 / 需要 | **仅内建库模式不需要**（后端是集群内存表，够快）；外部 PG 模式同样需要但 EMQX 未提供 | PostgreSQL 只读副本扛不住全量 λ，L2 把 DB 保护起来 |

**关键精确化（避免误读）**：EMQX 认证后端有两种互斥模式，需分别讨论：

```text
模式A 内建库（built_in_database）：
  用户数据存 EMQX 集群的 mnesia 表，mria 复制到所有节点（集群内存，微秒级）。
  CONNECT → ETS cache(1min) → miss → 读本地 mnesia（不查外部）。
  此模式下后端确实够快，超短 TTL + ETS 够扛，不需要 L2。

模式B 外部数据源（postgresql/mysql/redis/http）：
  用户数据在 外部 DB，EMQX 只是个客户端（源码确认：postgresql 后端走
  cached_simple_sync_query → emqx_resource → epgsql 客户端，不碰 mnesia，
  外部数据绝不同步到 mnesia）。
  CONNECT → ETS cache(1min) → miss → 查外部 DB（网络往返）。
  此模式下后端是外部 DB，处境和 mqtt-gateway 完全一样——ETS 单层不跨节点、
  无 L2、无粘性 → 日出冷启动 ETS 全空时直接打外部 DB。
```

**结论修正**：我之前"EMQX 假设后端是 mnesia 内建库足够快"只在模式A成立。**当 EMQX 也配置外部 PGSQL 时，它面临和 mqtt-gateway 完全相同的问题，且 EMQX 在这个场景下没有提供 L2 共享缓存——单层 ETS（1min TTL，不跨节点）是真实短板**。mqtt-gateway 的 L2 不是"相对于 EMQX 标准做法多了优化"，而是"在'外部 PG 认证 + 无粘性 + 百万日出'这个共同场景下，主动补了 EMQX 也没有的跨节点共享缓存层"。

补充证据（EMQX 源码确认外部数据源不碰 mnesia）：
- `emqx_authn_postgresql.erl:83`：`cached_simple_sync_query(...)` 走 resource 框架查外部 PG，grep `mnesia` 在该文件零命中
- `emqx_authn_mysql.erl:77` / `emqx_authn_redis.erl:75`：同构，都走 `cached_simple_sync_query`
- 外部数据源的查询结果只进 ETS authn cache（`emqx_node_cache`，per-node，不跨节点），不写 mnesia、不复制

#### 4.2.1 深入：EMQX 的 ETS（1min TTL）在无粘性下确实低命中——它靠什么活？

> 一个常见追问：「EMQX 的 ETS TTL 才 1 分钟，LB 又是随机分发，设备重连大概率落不同节点，ETS 正向命中率必然很低——那 EMQX 不也面临和 mqtt-gateway L1 一样的问题吗？」
> 答案：**是的，ETS 正向命中率确实低，EMQX 和我们的处境完全一样。但 EMQX 能活下来不是靠 ETS 命中，而是靠三个机制——其中只有一个在'外部 PG 场景'下成立，而那个场景下 EMQX 暴露了真实短板。**

**事实确认：EMQX ETS 在无粘性下正向命中率确实很低**

```text
ETS authn cache：
  TTL = 1 分钟（emqx_auth_cache_schema.erl:34-36，default <<"1m">>）
  跨节点 = 否（per-node ETS emqx_node_cache）
  LB 粘性 = 无（随机分发，设备重连可能落任意节点）

设备重连落同一节点的概率 = 1/N（N = 节点数）
  N=20 时，只有 5% 的重连能复用 ETS 条目 → 95% 的重连 ETS miss → 重查后端
```

**所以 EMQX 的 ETS 正向缓存，在"无粘性 + 设备会重连"的真实场景下，和 mqtt-gateway 的 L1 面临完全相同的问题——命中率被无粘性结构性稀释。** 这一点必须先承认，否则后续论证站不住。

**那 EMQX 靠什么活下来？三个机制（只有一个在所有场景成立）：**

| 机制 | 成立条件 | 对 ETS 低命中率的对冲 |
|---|---|---|
| **① 后端是 mnesia 内存表（µs 级）** | **仅内建库模式** | miss 了读 mnesia 也极快，ETS 命中率低无所谓 |
| **② acceptor 层限速** | 所有模式 | socket 层挡连接速率，间接保护后端 DB |
| **③ 长连接稳态下 ETS 不被查询** | 所有模式 | MQTT 长连接，CONNECT 只一次，稳态无认证查询 |

逐个看这三个机制在两种模式下的有效性：

```text
机制①（mnesia 兜底）—— 仅内建库模式成立：
  CONNECT → ETS(1min) miss → 读本地 mnesia 表（集群内存复制，微秒级）
  miss 了无所谓，因为 mnesia 是本节点内存哈希表，读一次几 µs，和 ETS 差不多快。
  这正是 EMQX 把 TTL 设成超短 1 分钟的底气——它不依赖 ETS 命中率，因为后端够快。
  1min TTL 只为挡"同一连接的秒级重试"（MQTT 协议层重连），不为"跨重连复用"。

机制②（acceptor 限速）—— 所有模式成立但不够：
  emqx_ranch_limiter 在 socket accept 阶段限连接速率。
  能挡住"瞬时连接洪峰"，但它是"挡连接"不是"挡 DB 查询"——
  放进来的连接照样要查后端，只是节奏被削平。

机制③（长连接稳态）—— 所有模式成立：
  MQTT 长连接，设备 CONNECT 一次成功后保持几小时/几天，期间 PUBLISH/SUBSCRIBE
  不再触发认证。所以 ETS 查询频率 = 连接建立频率，不是消息频率。
  稳态下设备都在线不重连 → ETS 根本没查询 → 命中率高不高无所谓。
  只有"日出冷启动"和"flapper 狂重连"两个场景才让 ETS 被高频查询，
  而这两个场景恰恰是 ETS 最无效的时候（冷的 / 被冲刷的）。
```

**关键转折：机制①只在内建库模式成立。一旦切到外部 PG，EMQX 就暴露短板了。**

```text
EMQX 外部 PG 模式（和 mqtt-gateway 同构）：
  CONNECT → ETS(1min) miss(95%) → 查外部 PG（ms 级网络往返）

  机制①失效：fallback 是外部 PG（ms 级），不是 mnesia（µs 级），miss 成本骤增。
  机制②不够：acceptor 限速能削峰，但放进来的连接照样打 PG。
  机制③挡不住日出：日出冷启动 ETS 全空，全量 λ 直接打 PG，这正是机制③失效的场景。

  结果：EMQX + 外部 PG 认证 + 百万设备日出 → 外部 PG 被打爆。
  这是 EMQX 社区的已知痛点——用过 EMQX + MySQL/PG 认证的人都知道，
  日出或大批重连时认证 DB 压力飙升。EMQX 对此的"应对"只是：
    ETS max_count=100万 + max_memory=100MB 硬扛（能多缓存就多缓存）
    + acceptor 限速
    但没有跨节点共享缓存（L2）。
```

**终极对照表：为什么 EMQX 不需要 L2、我们需要 L2**

| | EMQX 内建库 | EMQX 外部 PG | mqtt-gateway |
|---|---|---|---|
| ETS/L1 命中率（无粘性） | 低 | 低 | 低 |
| 命中率低是问题吗 | **否**（mnesia 兜底，µs） | **是**（PG 兜底，ms，日出打爆） | **是**（PG 兜底，ms，日出打爆） |
| 有 L2 吗 | 不需要 | **没有（短板）** | **有（Redis 共享层）** |
| TTL 策略 | 1min（够，因后端快） | 1min（不够，但 EMQX 没优化） | L1 5~30min + L2 ≥24h（刻意做长，因后端慢） |

**这张表是终极答案**：三者 ETS/L1 命中率都低（无粘性的结构性结果），区别只在"命中率低时 fallback 够不够快"。EMQX 内建库 fallback（mnesia）够快所以不需要 L2；EMQX 外部 PG 和我们 fallback 都不够快，但 EMQX 没补 L2（短板），我们用 L2 补了。

**TTL 长度是"后端快不快"的函数**：EMQX 内建库后端快 → TTL 短（1min，只挡秒级重试）；我们后端慢 → TTL 长（L1 5~30min / L2 ≥24h，让缓存在低命中率下仍有复用价值）+ 加 L2 跨节点共享层收回被无粘性稀释的命中率。两者不矛盾，是同一问题的不同解。

> 详见本报告 §2.1（ETS/mnesia/authn cache 对比表）和 §2.2（EMQX 集群连接路由）。

### 4.3 建议补入文档

§5.6.1 或 §5.5.3 补一段因果链（§4.1 的 4 行），把 L2 必要性从"日出冷启动"扩展到"无粘性拓扑的结构性约束"，更难被质疑。

---

## 5. 决策与落地建议

| 项 | 优先级 | 落地阶段 | 对应文档章节 |
|---|---|---|---|
| flapping 检测 | 中 | **Phase 1** | 新增 §6.3 或独立 §5.7 |
| banned 黑名单 | 中 | Phase 2 | §3.1 预留位置 + Phase 2 实现 |
| 运维级缓存清空 API | 低 | Phase 1 | §5.3 补充 |
| CONNECT 流水线顺序预留 | 低 | Phase 1（文档） | §3.1 补充 |
| `(client_id, source_ip)` throttle | 落地缺口 | Phase 1 | §5.2/§6.1（代码补） |
| 极端窗口 Redis 容量警示 | 低 | Phase 1（文档） | §5.6.2 补充 |
| L2 必要性论证强化 | 低 | Phase 1（文档） | §5.5.3 或 §5.6.1 补充 |

---

## 6. 哲学差异总结

| 维度 | mqtt-gateway | EMQX | 谁更适合逆变器场景 |
|---|---|---|---|
| 认证模型 | 直连 DB → Auth Service | 认证链（多后端可插拔） | Phase 1 单后端够用，简单即正确 |
| 缓存分层 | L1 + L2 两级 | 单层 ETS | **mqtt-gateway 更优**（无粘性下 L2 扛冷启动） |
| 密码方案 | SHA-256 无 salt（高熵机器凭证） | sha256+salt / bcrypt / pbkdf2 | **mqtt-gateway 正确**（机器凭证无需慢哈希） |
| 防爆机制 | auth queue + throttle + inflight=1 | flapping + banned + acceptor 限速 | **EMQX 的 flapping/banned 是我们缺的** |
| 凭证失效 | TTL + Phase 2 事件 | TTL + 手动 reset + key 含密码 | EMQX 的"key 含密码"巧妙，但我们"命中仍校验 secret"更安全 |
| 授权与认证分离 | §10 明确分离 | 完全分离的两条链 | 两者一致 |

---

## 参考资料

- [CONNECT 认证设计](../5.cloud-mqtt-connect-auth-design.md)
- [mqtt-gateway 架构设计](../1.cloud-mqtt-gateway-architecture.md)
- EMQX 源码：
  - 认证缓存：`apps/emqx_auth/src/emqx_auth_cache.erl`
  - 认证链：`apps/emqx_auth/src/emqx_authn/emqx_authn_chains.erl`
  - 密码哈希：`apps/emqx_auth/src/emqx_authn/emqx_authn_password_hashing.erl`、`apps/emqx/src/emqx_passwd.erl`
  - flapping：`apps/emqx/src/emqx_flapping.erl`
  - banned：`apps/emqx/src/emqx_banned.erl`
  - session 注册：`apps/emqx/src/emqx_cm_registry.erl`、`apps/emqx/src/emqx_cm.erl`
  - 流水线：`apps/emqx/src/emqx_channel.erl`
