# gnet 可行性评估：替换标准库 net 对 mqtt-gateway 的内存 / GC / 百万连接影响

> 评估用 [gnet](https://github.com/panjf2000/gnet)（Go epoll/kqueue reactor 网络库）替换标准库 `net` + goroutine-per-conn 模型，对 mqtt-gateway 内存、GC、单节点连接上限的影响。
>
> **结论与 [zhenyi-assessment](./zhenyi/zhenyi-assessment.md) 不同**：那篇基于"broker 终结 TLS + 慢层未解耦"的旧前提，判 reactor "不值"。本文基于两个被核实的前提——**① NLB 卸载 TLS、明文多数派（架构 §11.5）；② 慢层早已是 bounded 队列 + worker pool 解耦（[ingest/queue.go](../../../../mqtt-gateway/internal/ingest/queue.go)、红线 #7）**——重判为"**对明文多数派路径值得认真评估**"。两篇不矛盾，是前提不同导致结论不同（见 §9）。
>
> 配套：[GC mark 计算模型](./go-gc-mark-cost-model.md)、[容量规划](../11.cloud-mqtt-gateway-capacity-planning.md)、[TLS 接入设计](../../network-defense/13.cloud-mqtt-gateway-tls-design.md)、[zhenyi 评估](./zhenyi/zhenyi-assessment.md)。

---

## 1. 结论

```text
gnet 在 mqtt-gateway 的价值分三档，必须分开看，否则会高估或低估：

  内存：   🟢 强  —— 去 goroutine 栈（50万×8KB=4GB）；叠加 NLB 卸载 TLS，
                     每连接常驻 21KB → ~10KB，内存上限约翻 2 倍。
  GC STW： 🟢 强  —— 消除 50万 goroutine 栈根扫描，mark termination STW
                     5-15ms（日出 15-30ms）→ <1ms。这是最初提问指向的点，判断成立。
  GC CPU： 🟡 弱  —— 只削掉"栈指针"那 ~1/4，主导成本 Session live heap 重扫纹丝不动。
                     降 GC CPU 的主杠杆仍是 Session 指针压缩，不是 gnet。

正确组合不是"gnet vs Session 压缩"二选一，而是三个正交杠杆叠乘：
  NLB 卸载 TLS（已在设计） + gnet（明文监听器） + Session 压缩
  mTLS 少数派留经典 goroutine-per-conn 路径（混合监听器，§6）。

叠满后明文路径：常驻 ~10KB、live 指针 ~200万、STW <1ms、GC CPU ~5% 量级
  → 单节点百万在内存 / GC / CPU 三轴上"够得着"。
  真正的拦路虎退化成"故障爆炸半径"这个运维取舍（容量规划 §10.3），不是网络模型。
```

---

## 2. 两个翻转前提（为什么结论与 zhenyi 评估不同）

### 2.1 前提 ①：NLB 卸载 TLS，网关收明文（多数派）

[架构 §11.5 v1 修订拓扑](../1.cloud-mqtt-gateway-architecture.md)（被 doc 9 网络攻防设计修订）正式确立**设备分流**：

```text
普通设备（绝大多数） → 云 TLS LB 终结单向 TLS → 网关收明文（拓扑 B）
高安全设备（少数）   → L4 透传 → Broker 终结 mTLS（拓扑 A）
```

含义：**绝大部分连接到网关时已是明文，网关内没有 `crypto/tls`**。

这直接拆掉了 zhenyi 评估列为头号阻断点的"TLS-over-gnet 兼容问题"——对明文多数派，gnet 看到的就是明文 MQTT 字节流，没有 TLS record 凑齐问题。同时它也抹掉了每连接的 TLS 堆成本（见 §3、§4）。

阿里云 NLB 约束（[TLS 设计 §4](../../network-defense/13.cloud-mqtt-gateway-tls-design.md)）：TCPSSL 监听的 PROXY protocol v2 不传 `pp2_ssl_cn`，所以 **mTLS 必须 broker 自己终结（拓扑 A）**。这部分无法 NLB 卸载——但它是少数派，留经典路径即可（§6）。

### 2.2 前提 ②：慢层早已解耦（bounded 队列 + worker pool + fencing）

[ingest/queue.go](../../../../mqtt-gateway/internal/ingest/queue.go) 已是 bounded 队列 + 固定 worker pool，结果经 `job.result chan` 异步回传；auth 队列同构。红线 #7 进一步把回执路径设计成异步 + fencing：

> append callback 不直接操作 session，只产出 `AppendResult{ConnID, SessionVersion}`，经 bounded 回执队列投递回 session 拥有者，用 `ConnID + SessionVersion` fencing 校验后才发 PUBACK。

这段描述正是 reactor 需要的异步回执基建。当前 `SubmitAndWait` 只是让 read goroutine 阻塞在 `<-j.result` 上——这个阻塞是 goroutine-per-conn 模型的"偷懒简化"，不是架构约束。

含义：**慢层（auth / presence / Kafka append）不用动，只有"快层 → 慢层"的边界要改**——从"阻塞等 channel"改成"提交后返回，回执触发 `conn.AsyncWrite(PUBACK)`"，fencing 已就绪。这把"异步重写 = server 包接近重写"的旧判断，收敛成"重写限于 codec + 快层边界 + CONNECT 状态机"。

---

## 3. 内存维度收益

### 3.1 心智模型前置：goroutine 栈不在 GC 堆里

```text
goroutine 栈 = 独立栈分配器管理，不属于 GC heap
  → 4GB 栈不计入 live heap，本身不触发 GC
  → 但它是真实 RSS，吃 GOMEMLIMIT / OOM 预算
  → 栈帧里的指针是 GC 根，mark（并发 + termination）要扫
```

所以"去掉 goroutine"的内存收益是 **RSS 下降**（栈），GC 收益是 **根扫描减少**（§4），两者来源不同，不要混算。

### 3.2 每连接常驻内存（明文路径，50万连接）

每连接常驻构成见 [容量规划 §2.2](../11.cloud-mqtt-gateway-capacity-planning.md)（其 21KB 假设拓扑 A 含 TLS）：

| 配置 | 栈 | conn / bufio | TLS | 读状态 | 合计 |
|---|---|---|---|---|---|
| 拓扑 A goroutine（含 TLS 基线） | 8KB | 8KB | 4KB | 1KB | ~21KB |
| 拓扑 B goroutine（今天的多数派，无网关 TLS） | 8KB | 8KB | **0** | 1KB | ~17KB |
| **拓扑 B + gnet** | **~0** | ~3KB | 0 | ~2KB | **~9-11KB** |

```text
50万连接： 栈 RSS 省 ~4GB；常驻 ~17GB→~5GB 区间（叠加 NLB 去 TLS）
100万连接：栈 RSS 省 ~8GB；常驻 ~23GB→~10GB
→ 按内存这条约束，32GB 机型从"撑百万很紧"变"撑百万宽松"，上限约翻 2 倍
```

### 3.3 不能省的坑：内核 socket buffer

每连接 net.Conn 的 8KB 里含**内核 socket buffer（SO_RCVBUF/SNDBUF）**——每 fd 的内核内存，不在用户态，**gnet 消除不了**。百万连接下这是隐性大头（可调但跑不掉）。gnet 省的是 Go 侧那一半（bufio）。容量规划的 fd / 内核内存核算不因 gnet 改变。

---

## 4. GC 维度收益（STW 强、CPU 弱）

### 4.1 STW（mark termination 栈根扫描）—— 强

[GC 模型 §8.2.2](./go-gc-mark-cost-model.md) 算清：50万 readLoop goroutine，concurrent 阶段靠 async preemption 扫掉 70-90%，残留 5-15万栈塞进 mark termination（STW）扫——**这是 5-15ms STW（日出 15-30ms）的唯一主要来源**。

gnet 把 50万 goroutine 压成 ~GOMAXPROCS 个固定 worker → 栈根从 50万 降到几十 → **STW 来源直接归零**：

```text
STW: 稳态 5-15ms → <1ms ；日出 15-30ms → <1ms     约 10-30×
```

日出场景收益更大——现在日出 STW 更糟（goroutine 数从 0 暴涨、concurrent 窗口被压短、残留更多），gnet 下"goroutine 暴涨"这个变量根本不存在。

### 4.2 并发 mark CPU（live heap 重扫）—— 弱

live heap 指针构成（[GC 模型 §7.2.3](./go-gc-mark-cost-model.md)，按拓扑分列）：

| 来源 | 拓扑 B goroutine | 拓扑 B + gnet | + Session 压缩 |
|---|---|---|---|
| Session × 50万 | 250万 | 250万 | **~50万** |
| net.Conn × 50万 | 100万 | ~100万 | ~100万 |
| TLS 状态 × 50万 | **0**（在 NLB） | 0 | 0 |
| goroutine 栈指针 × 50万 | 200万 | **~50万**（挪堆，见 §5） | ~50万 |
| **合计** | **~550万** | **~400万**（-27%） | **~200万**（-64%） |

```text
GC CPU ∝ 指针数。
gnet 单独：550万→400万，GC CPU 约 -27%（削的全是"栈"那一块）。
真正大头 Session 250万 gnet 一根没动 → 降 GC CPU 主杠杆仍是 Session 压缩。
gnet + Session 压缩叠加：550万→200万，GC CPU -64% —— 两者正交、可叠乘。
```

### 4.3 GC 频率 —— 基本不动

GC 触发 = live heap 增长到 GOGC%。栈不在堆里 → 省的 4-8GB 不改 live heap → GC 周期（稳态 ~24 分钟）基本不变。唯一**间接**好处：省下的 RSS 让 GOMEMLIMIT 余量更大，可把 GOGC 调更高 → GC 轮次更稀 → GC CPU 再降一点（二阶收益，小但真实）。

---

## 5. 异步重写的真实范围

慢层不动（§2.2），只有快层和边界要改：

| 层 | 现状 | gnet 后 | 工作量 |
|---|---|---|---|
| 慢层（auth/ingest worker pool） | 已解耦 | **不动** | 0 |
| QoS0 路径 | `TryAppendAsync` 已非阻塞 | 几乎不动 | ~0 |
| QoS1 PUBACK | read goroutine 阻塞 `<-result` | `job` 带 ConnID+SV，回执触发 `conn.AsyncWrite(PUBACK)` | 小（fencing 已就绪） |
| **MQTT codec** | `ReadPacket(io.Reader)` + `io.ReadFull` 阻塞 | 增量可重入：喂 ring buffer 字节，半包挂起续读 | **中（真功夫）** |
| **CONNECT 握手** | 线性阻塞（read→auth→presence→connack） | 每连接小状态机 `AWAIT_CONNECT→AWAIT_AUTH→AWAIT_PRESENCE→CONNECTED` | **中（新代码）** |

两个真功夫：**增量 codec**（收敛在 `internal/mqtt` 包）和 **CONNECT 状态机**（收敛在 `internal/server` 包）。不扩散到慢层。

### 5.1 必须点出的代价：解析状态从栈挪到堆

现在半包解析状态存在 read goroutine 的**栈**上（`ReadPacket` 局部变量）。gnet 下没这个栈了，状态要挪到**每连接堆结构**（gnet 的 conn context）。

```text
后果：省掉的栈指针不是"全消除"，是"200万 → 挪堆里 ~50万"。
约束：这个 per-conn 解析状态必须守住红线 #2 pointer-poor——
      别塞 buffer 指针 / map / interface，否则边省边漏，GC 收益打折。
```

这也是 §4.2 表格里"gnet 后栈指针 ~50万"而非 0 的原因。

---

## 6. TLS 处理：混合监听器分流（而非 TLS-over-gnet）

不用让 TLS 碰 gnet。顺着设计里已有的拓扑边界分流：

```text
普通设备（百万级，明文）  → gnet 监听器（:1883）          ← 拿内存 + STW 收益
高安全设备（千~万级，mTLS）→ 经典 net+crypto/tls+goroutine-per-conn（:8883）
```

mTLS 是少数派，goroutine 数小到不触发 STW 问题、不占多少内存，继续 goroutine-per-conn 完全没事。于是"TLS-over-gnet"整道绕过，不用解。这比"把解密放慢层"更省心，且不引入新结构。

**关于"TLS 解包放慢层"**（评估时被问及）：
- **握手（一次性，ECDSA ~1ms）**：天然适合慢层 worker pool，和 auth 同构。**值得做，且与 gnet 正交**——即便不上 gnet，把 ECDSA 从 accept 热路径挪到握手 worker pool 也能削日出风暴尖峰。
- **逐包 record 解密（AES-GCM，热路径）**：`crypto/tls` 不暴露"解这段密文"的纯函数，`tls.Conn` 把字节流和状态机绑死。"放慢层"只换解密在哪跑，换不掉"怎么喂字节进 TLS 状态机"这个核心难点。**不值得**——mTLS 少数派留经典路径即可。

---

## 7. 三杠杆正交叠加 + 百万 / 节点重判

百万 / 节点的四道墙，三道有正交解：

| 墙 | 解 | 状态 |
|---|---|---|
| 内存（23KB→~10KB） | gnet 去栈 + NLB 去 TLS | ✅ 上限翻 ~2 倍 |
| GC STW（栈根） | gnet | ✅ →<1ms |
| GC CPU（live 重扫） | Session 压缩（gnet 只削 1/4） | ✅ 主杠杆是压缩 |
| 握手 CPU（ECDSA 风暴） | NLB 卸载多数派 + 握手 worker pool | ✅ 多数派不落网关 |

```text
百万明文连接 × ~10KB ≈ 10GB 常驻 → 32GB 机型宽松
百万 ×（压缩后 Session ~1 指针）+ conn ≈ 200-300万指针 → GC CPU 可控
握手压力在 NLB → 网关 CPU 全给业务
→ 内存 / GC / CPU 三轴，单节点百万技术上站得住
```

### 7.1 剩余风险（gnet 解不了，必须诚实标注）

```text
1. 故障爆炸半径（运维约束，首要）：
   单节点扛百万 → 挂一台 = 百万设备同时重连。
   容量规划 §10.3 反复强调这是不上百万/节点的首要理由，与网络模型无关。
   技术能扛 ≠ 该这么部署。生产建议仍是多节点分摊（25-50万/节点）。

2. mTLS 少数派别长成多数派：
   高安全设备若涨到几十万，其 goroutine-per-conn 又把 STW 拉回来。
   这条路径的规模上限要盯住（监控 mTLS 监听器的 goroutine 数）。

3. 仍是理论，必须实测：
   - GODEBUG=gctrace=1 实测 mark termination STW（日出风暴下是否 >30ms）
   - gnet 半包 ring buffer 的真实 per-conn 内存（弹性 buffer 可能回吃一部分省的栈）
   - 增量 codec 的正确性（畸形包 / 分片边界，对齐 fuzz 红线）
```

---

## 8. 与现有结论的关系

本文不推翻 [GC 模型 §10](./go-gc-mark-cost-model.md) 的优化优先级，而是补一个维度：

```text
GC 模型 §10：降 GC CPU 用 Session 压缩，降 STW 用"减栈帧指针/加核"，
           并把"换 reactor"列为 §10.2 ⑨「不推荐，复杂度暴增」。

本文修正：§10.2 ⑨ 的"复杂度暴增"判断基于"全量 TLS-over-gnet + 慢层重写"的旧前提。
        在"NLB 卸载多数派 + 慢层已解耦 + 混合监听器"三前提下，复杂度收敛到
        codec + CONNECT 状态机两个包，reactor 从"不推荐"升为"明文路径值得评估"。
        且 gnet 同时解掉 §10.2（STW）和部分 §10.1（GC CPU 的栈那块）+ 内存，
        是少数能一次动多个维度的杠杆——代价是 codec/握手异步化的一次性投入。
```

---

## 9. 决策记录

| 项 | 决策 | 理由 |
|---|---|---|
| 明文多数派监听器是否上 gnet | **值得评估（非立即）** | 内存翻倍 + STW→<1ms；前提是 NLB 卸载 + 慢层解耦均已具备 |
| mTLS 少数派是否上 gnet | **否** | 留经典 goroutine-per-conn，绕过 TLS-over-gnet；少数派无 STW/内存问题 |
| gnet 能否替代 Session 压缩降 GC CPU | **否** | gnet 只削栈指针（~1/4），Session 重扫是主导，压缩仍是主杠杆 |
| TLS 逐包解密是否放慢层 | **否** | crypto/tls 状态机绑死字节流，放慢层不解核心难点 |
| TLS 握手是否放慢层 worker pool | **是（与 gnet 正交）** | 一次性 ECDSA，削日出尖峰，不上 gnet 也该做 |
| 单节点是否冲百万 | **技术可达，运维不推荐** | 三轴够得着，但故障爆炸半径仍指向多节点分摊（25-50万/节点） |
| 与 zhenyi 评估"reactor 不值"的关系 | **前提不同，非冲突** | 那篇基于 broker 终结 TLS + 慢层未解耦；本文基于 NLB 卸载 + 慢层已解耦 |

```text
核心结论：gnet 不是"GC 万能药"，但在"NLB 卸载 TLS 的明文多数派 + 慢层已解耦"
两前提下，它一次性吃掉内存（去栈 4-8GB）和 GC STW（栈根扫描 →<1ms）两个维度，
代价是 codec 增量化 + CONNECT 状态机两个包的异步改造。
配合 Session 压缩（降 GC CPU）和 NLB 卸载（降握手 CPU），单节点百万在
内存/GC/CPU 三轴上够得着——但生产部署仍受故障爆炸半径约束，推荐多节点分摊。
评估优先级：低于"先做 Session 压缩 + 实测 gctrace"，是后续而非当下。
```

---

## 10. 参考资料

- [gnet GitHub](https://github.com/panjf2000/gnet)
- 配套设计：
  - [GC mark 计算模型](./go-gc-mark-cost-model.md)（§7.2 快进快出、§8.2 STW 栈扫描、§10 优化杠杆）
  - [容量规划](../11.cloud-mqtt-gateway-capacity-planning.md)（§2 每连接内存、§10.3 50万/节点、爆炸半径）
  - [TLS 接入设计](../../network-defense/13.cloud-mqtt-gateway-tls-design.md)（§2 两种拓扑、§4 mTLS 约束、§5 resumption）
  - [架构 §11.5 v1 修订拓扑](../1.cloud-mqtt-gateway-architecture.md)（普通/高安全设备分流）
  - [zhenyi-base / zhenyi 评估](./zhenyi/zhenyi-assessment.md)（reactor 旧前提下的"不值"结论）
- 代码：
  - [internal/ingest/queue.go](../../../../mqtt-gateway/internal/ingest/queue.go)（慢层 bounded 队列 + worker pool）
  - [internal/server/server.go](../../../../mqtt-gateway/internal/server/server.go)（当前 goroutine-per-conn read loop）
- 红线：[实现红线](../7.cloud-mqtt-gateway-implementation-red-lines.md)（#2 pointer-poor、#7 append 异步回执 fencing）
