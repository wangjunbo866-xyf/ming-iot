# nats-server 网络 I/O / 缓冲机制分析与借鉴

> **文档定位**：本文件是对 `nats-server/`（本地 fork 源码）网络读路径与缓冲池机制的工程分析，目的是**为 `mqtt-gateway/` 的实现提供可借鉴 / 不可借鉴的边界**。
> **关联文档**：[7.实现红线](./7.cloud-mqtt-gateway-implementation-red-lines.md)（尤其 §3 bounded-memory、§5 队列有界）、[1.架构](./1.cloud-mqtt-gateway-architecture.md)。
> **分析对象版本**：`nats-server/`（本地目录，commit 见 `git -C nats-server log -1`）。
> **状态**：第一稿（网络读/缓冲/解析）+ §2.4 全树 sync.Pool 清点（2026-06-18 补，27 个池实例）+ §2.4.6 nbPoolSmall/writev 深度分析（2026-06-18 补，mqtt-gateway 池化阈值的核心设计依据）+ §2.4.7 含指针 struct 池化的 GC mark 分析与 sync.Pool 评估指标体系（2026-06-18 补，未来含指针 struct 池的评估方法论）。**后续待补充**见末尾「待补充清单」。

---

## 0. 三个问题的速答

| # | 问题 | 速答 | 关键源码 |
|---|---|---|---|
| 1 | 是否反复复用一个固定读缓冲？ | **YES（自适应缩放）** | `client.go:1442` `b := make([]byte, c.in.rsz)`，`client.go:1592-1607` grow/shrink |
| 2 | 是否使用 sync.Pool 池化缓冲？ | **YES（约 20 个池），但读缓冲不池化** | `client.go:369-401` `nbPoolSmall/Medium/Large`；全树无读缓冲池 |
| 3 | 是否零拷贝流式解析？ | **YES（快速路径零拷贝，跨读 split 时 copy，分发时永远 copy）** | `parser.go:424-444` `PUB_ARG`，`parser.go:446-452` jump-ahead，`parser.go:491-499` `MSG_END_N` |

---

## 1. 读缓冲：每 goroutine 一份、自适应缩放、不入池

### 1.1 关键源码

读循环入口（`client.go:1403` `readLoop`），缓冲只分配一次：

```go
// client.go:1418
c.in.rsz = startBufSize
// client.go:1442
b := make([]byte, c.in.rsz)
```

尺寸常量（`client.go:111-113`）：

```go
startBufSize    = 512   // 初始：刚够装 INFO/CONNECT 块
minBufSize      = 64    // 收缩下限：够装 PING/PONG
maxBufSize      = 65536 // 扩张上限：64KB
```

主读循环反复读进同一块 `b`（`client.go:1461-1477`）：

```go
for {
    n, err = reader.Read(b)
    if n == 0 && err != nil { ... return }
    // ... parse(bufs) ...
}
```

自适应缩放（`client.go:1592-1607`）——满读 ×2、连续短读 ÷2：

```go
if n >= cap(b) && cap(b) < maxBufSize {
    c.in.rsz = int32(cap(b) * 2)
    b = make([]byte, c.in.rsz)          // grow：重新分配
} else if n < cap(b) && cap(b) > minBufSize && c.in.srs > shortsToShrink {
    c.in.rsz = int32(cap(b) / 2)
    b = make([]byte, c.in.rsz)          // shrink：重新分配
}
```

### 1.2 设计要点

- **不用 `bufio.Reader`**。nats 直接 `net.Conn.Read` 进自管缓冲，绕开 bufio 的额外抽象与拷贝层。
- **每连接 goroutine 独占一份缓冲**，无锁、无竞争，自然避开 pool 在读路径上的争用。
- **grow/shrink 用 `make` 重新分配**（不是 `append`），即扩容瞬间会有一次旧 buffer 的 GC；常态下读缓冲生命周期与连接等长，不进 `sync.Pool`。
- WebSocket 路径有 `bufs [2][]byte` 的 scatter-gather 适配；普通 TCP 路径是栈上 1 元素数组 `var _bufs [1][]byte`（`client.go:1448-1449`），避免 escape。

---

## 2. sync.Pool：约 20 个池，读缓冲例外

### 2.1 与网络 I/O 最相关：分级 outbound 池

`client.go:369-401` 定义三档 outbound 字节数组池（注意是固定大小数组指针 `*[N]byte`，不是 slice）：

```go
var nbPoolSmall = &sync.Pool{New: func() any { b := [nbPoolSizeSmall]byte{};  return &b }}
var nbPoolMedium= &sync.Pool{New: func() any { b := [nbPoolSizeMedium]byte{}; return &b }}
var nbPoolLarge = &sync.Pool{New: func() any { b := [nbPoolSizeLarge]byte{};  return &b }}

func nbPoolGet(sz int) []byte {
    switch {
    case sz <= nbPoolSizeSmall:  return nbPoolSmall.Get().(*[nbPoolSizeSmall]byte)[:0]
    case sz <= nbPoolSizeMedium: return nbPoolMedium.Get().(*[nbPoolSizeMedium]byte)[:0]
    default:                     return nbPoolLarge.Get().(*[nbPoolSizeLarge]byte)[:0]
    }
}
```

分级阈值（典型 IoT 体感参考）：512B / 4KB / 64KB。

**用途仅限 outbound**：`queueOutbound`（`client.go:2532-2536`）把要发给订阅者的字节 `copy` 进池化帧：

```go
for len(toBuffer) > 0 {
    new := nbPoolGet(len(toBuffer))
    n := copy(new[:cap(new)], toBuffer)
    c.out.nb = append(c.out.nb, new[:n])
    toBuffer = toBuffer[n:]
}
```

### 2.2 其他池清单（按子系统）

| 子系统 | 池 | 文件 | 池化对象 |
|---|---|---|---|
| JetStream Stream | `inMsgPool` `jsPubMsgPool` `cMsgPool` `fastBatchPool` `dgPool` | `stream.go` | 入队消息 / 发布消息 / 消费者通知 / 批处理 / 直拿请求 |
| Consumer | `jsAckMsgPool` `jsGetNextPool` `wrPool` `wdPool` `nextMsgReqPool` | `consumer.go` | ack / getNext / waiting request/delivery |
| Raft | `cePool` `entryPool` `aePool` `pePool` `arPool` | `raft.go` | 已提交项 / 日志项 / appendEntry / proposedEntry / appendResponse |
| Filestore | `blkPoolTiny/Small/Medium/Big` | `filestore.go:1004-1027` | 256KB / 1MB / 4MB / 8MB 字节数组 |
| Gateway / sendq / events | `subPool` `outMsgPool` `pubMsgPool` | 多处 | 订阅 / 出队消息 / 内部事件 |
| WebSocket | `decompressorPool` | `websocket.go:100` | permessage-deflate 解压状态 |
| 通用队列 | `ipqueue` pool | `ipqueue.go:89` | `*[]T` 元素切片 |

### 2.3 关键不变量

> **读缓冲（`readLoop` 的 `b`）从不进 `sync.Pool`。**

理由：每连接独占、与连接等长、避池争用；grow/shrink 用 `make` 比 pool 复用更简单可证。**这是 nats 工程取舍中很重要的一条：池化不是越多越好，读路径上宁可不池。**

### 2.4 池化与 mcache 的边界：nats 池什么、不池什么

> 本节回答一个常见质疑：「nats 的 nbPoolSmall 是 512B，为什么 mqtt-gateway 的 §6.1 不池化 ≤4KB 的 payload？」——答案在于**对象类型 + 复用模式**，不只是尺寸。
> 数据来源：2026-06-18 对 `/nats-server/server` 全树 `sync.Pool` 实例的完整清点（27 个定义点）。

#### 2.4.1 全树池化实例清单（按对象类型分组）

| 类型 | 池数 | 典型尺寸 | 代表 |
|---|---|---|---|
| `*[N]byte` 固定数组指针（返回为 `[]byte`） | 7 | 512B/4KB/64KB（outbound），256KB/1MB/4MB/8MB（filestore） | nbPool*、blkPool* |
| `*struct` 指针（**含指针字段**） | 18 | 24B~160B | entryPool、inMsgPool、wrPool、cePool... |
| `io.ReadCloser`（flate 解压器，重状态） | 1 | 状态机，构造贵 Reset 便宜 | decompressorPool |
| `*[]T`（泛型，per-instance） | 1 模板 / N 实例 | 初始 cap=32 | ipQueue[T].pool |

**全树无任何 ≤256B 纯字节数组池。** 最小的字节池是 nbPoolSmall（512B，outbound `net.Buffers` 帧拼装场景）。

#### 2.4.2 nats 池化三类动机

**类 1：池化「mcache 够不着的大字节数组」（教科书用法）**

nbPool（64KB）+ blkPool（256KB~8MB）。Go allocator 对 >32KB 的对象退出 mcache/mcentral 的 size-class fast path，直接走 mheap 分配 page（甚至 mmap），且 8MB 的 `make` 含 8MB memclr 清零。sync.Pool 的几百 ns Get/Put 完全摊销掉。`filestore.go:361` 注释明说 256KB 的 Tiny 档"不是真实 block size，是给 pool 垫的小缓冲"——池化下限明确画在"大对象"侧。

**类 2：池化「含指针的小 struct」（省 GC mark，与 mcache 正交）**

18 个 struct 池，尺寸 24B~160B。表面看很小，按"小对象别池"原则似乎不该池——关键在这些 struct **含指针字段**（slice header、内嵌指针）。`[]byte` 是 noscan（GC 不扫描内容），但含指针对象 GC mark 阶段要展开扫描其指针字段，百万级对象 = 百万次 mark 展开。**mcache 的 fast path 省的是分配开销，不省 GC mark/sweep 开销**；sync.Pool 通过"复用而非丢弃"让对象不进 GC 周期——这是 mcache 给不了的收益。

> **深入展开见 §2.4.7**——含指针 struct 的 GC mark 成本量化（轴 1 分配 vs 轴 2 mark 正交性）、mcache 能力边界、sync.Pool 选型的 7 项评估指标体系、以及 nats 三个代表池（Entry/waitingRequest/jsAckMsg）的打分对比，作为 mqtt-gateway 未来引入「含指针 struct 池」的评估方法论依据。

这也是这 18 个池几乎都配 `recycle()`/`returnToPool()` 方法（Put 前清零字段，防悬挂引用拖住大 backing array）的原因：池化含指针对象的额外复杂度，nats 认为值得。

**类 3：池化「构造贵的重状态对象」**

decompressorPool（flate 解压器）。构造一次要初始化整个哈夫曼表 + 滑动窗口状态机，但 `Reset` 很便宜。与尺寸无关，mcache 帮不上（构造本身 CPU 密集）。

#### 2.4.3 nats 对 mcache 的明确表态（三处证据）

1. **读缓冲（最高频小/中对象场景）坚决不池**——§2.3 不变量。`readLoop` 的 `b` 尺寸在 KB 级（自适应到几十 KB），正好是 mcache / per-P span 覆盖的区间，nats 把它**交还给 Go allocator**。这是"中对象交给 allocator"哲学的最强证据。
2. **ipQueue 用 `*[]T` 规避 SA6002**（`ipqueue.go:91-92` 注释明引 staticcheck）：

   ```go
   // Reason we use pointer to slice instead of slice is explained
   // here: https://staticcheck.io/docs/checks#SA6002
   res := make([]T, 0, 32)
   return &res
   ```

   Put 一个 slice 值（3 字段结构体）会触发不必要分配/拷贝，正确做法是 `*[]T`。这是 nats 对"小对象 allocator 行为"的主动规避。
3. **blkPool 的 Tiny 档是 256KB，不是 256B**。注释明说存在这档是为让 pool 有小缓冲可垫，但"小"是 256KB——nats 的字节池化下限明确画在大对象侧，不为 tiny allocator 覆盖的对象建池。

#### 2.4.4 对照表：对象类型 × mcache × 该不该池

| 对象类型 | mcache 能否帮上 | 该不该池 | nats 做法 |
|---|---|---|---|
| 大字节数组（>32KB） | ❌ 退出 fast path | ✅ 必须 | nbPool(64KB)、blkPool(256KB~8MB) |
| 中字节数组（512B~32KB） | ✅ per-P span | ⚠️ 看热度+复用率 | nbPoolSmall(512B)/Medium(4KB)**池化**（outbound 极热）；读缓冲（KB 级）**不池** |
| 小字节数组（≤256B） | ✅ tiny allocator | ❌ 别池 | nats 无任何 ≤256B 字节池 |
| 含指针小 struct（24~160B） | ✅ 分配免费 | ✅ 该池（省 GC mark） | 18 个 struct 池 |
| 重状态对象 | ❌ 构造贵 | ✅ 该池 | decompressorPool |

#### 2.4.5 对 mqtt-gateway §6.1 的校准

§6.1 定稿"payload ≤4KB 不池、>4KB 走 4KB/16KB 分级池"，与 nats 哲学一致，但**不能简单照搬 nats 的 512B 档**，差异在复用模式：

| 维度 | nats nbPoolSmall(512B) | 我们 payload buffer |
|---|---|---|
| 分发模型 | fan-out，copy 进帧缓冲 | single-sink → Kafka，原样进 Record |
| 复用模式 | copy 完、flush 完立刻归还（紧凑复用） | 跨 goroutine 驻留（read loop → ingest worker → ack callback） |
| 复用率 | 每条消息 copy 进帧 + flush 即归还，命中率极高 | 一个 payload 一条 Kafka Record，不复用帧 |
| 归还窗口 | 微秒级（一次 flush） | ms~s 级（等 Kafka durable ack） |

nbPoolSmall(512B) 能赢，靠的是 outbound 那种"copy 进帧、flush 完立刻归还"的**紧凑高复用**——省的是 alloc 计数和 memclr。我们 payload 的复用模式是"一 payload 一 Record、跨 goroutine 驻留"，**紧凑复用不成立**，512B 档的 pool 命中率撑不起来 Get/Put 的几百 ns 开销。故 §6.1 的分档（4KB/16KB 两档、≤4KB 不池）针对**我们的复用模式**是合理的。

> **重要修正（2026-06-18 补，详见 §2.4.6）**：上表把 nbPoolSmall(512B) 笼统理解为"copy 进帧"的缓冲，**未点破其真实身份**。512B 档**不是只服务 ≤512B 的小消息**，而是 `net.Buffers`(writev) **chunked-copy 的"碎片/尾部填充"档**——一条 5KB 消息会被切成 4KB+512B+512B 多块，512B 档承载任意大小消息的尾部碎片。所以 nats 池化 512B 的判据不是"512B 足够小"，而是 **writev I/O 模型 + chunked copy + 极短归还窗口**三者同时成立。这正是 mqtt-gateway 不照搬的核心：我们不具备 writev chunked-copy 模型（payload 原样进单个 Kafka Record，无尾部填充语义）。

**两点延伸结论**（不在 §6.1 当前范围，留作后续优化线）：

1. 若 broker 后续从纯 `[]byte` payload 扩展到**含指针字段**的结构体（如带 slice header 的 `RawEnvelope`），那一类的 GC mark 成本才该考虑池化——**但那是 struct 池，不是 payload 字节池**，是另一条线（对应 nats 的类 2）。**评估方法论见 §2.4.7**（7 项指标体系 + 归还点字段清理敬畏点）。
2. 若未来做 Broker→device 下行帧拼装（类似 nats outbound `net.Buffers`），那种"copy 进帧、flush 完立刻归还"的紧凑复用场景才适合引入 nbPoolSmall 级的 512B 池——对应 §6.3 的 outbound 分级池（当前暂缓，等下行通道那圈）。

#### 2.4.6 【设计依据】nbPoolSmall(512B) 深度分析：为什么 nats 池化 512B 是对的，而我们不池是对的

> 本节回答一个会被反复质疑的问题：「nats 的 nbPoolSmall 才 512B 都池化了，mqtt-gateway 的 §6.1 为什么不池化 ≤4KB payload？难道 nats 的工程师不懂 mcache？」
> 答案：**两边都对，差异不在尺寸阈值，在 I/O 模型**。本节用 4 处源码证据讲透，是 mqtt-gateway payload 分级池阈值（§6.1）的**核心设计依据**。

**证据 1：`c.out.nb` 是 `net.Buffers`（writev），不是普通缓冲（client.go:350）**

```go
nb  net.Buffers  // Pending buffers for send, each has fixed capacity as per nbPool below.
```

`net.Buffers` 是 Go 对 `writev(2)` scatter-gather I/O 的封装——把**多个不连续的 `[]byte`** 在一次 syscall 里聚合成一个 TCP 流写出。这是 nats 出站的核心优化：不把多条消息 memcpy 进一块大缓冲，而是让每个 nb entry 指向独立 buffer，内核做 gather。`nbPool*` 三个池就是给这些独立 buffer 供内存的。

**证据 2：`sendClientBoundMsg` 是 chunked copy，512B 是任意大小消息的"尾部填充档"（client.go:2529-2537）**

```go
// 先 tail-top：填满上一个未满 buffer（client.go:2518-2527）
if len(c.out.nb) > 0 {
    last := &c.out.nb[len(c.out.nb)-1]
    if free := cap(*last) - len(*last); free > 0 {
        *last = append(*last, toBuffer[:free]...)
        toBuffer = toBuffer[free:]
    }
}
// 再按剩余长度切档申请（client.go:2532-2537）
for len(toBuffer) > 0 {
    new := nbPoolGet(len(toBuffer))        // 按剩余长度选 512B/4KB/64KB 档
    n := copy(new[:cap(new)], toBuffer)
    c.out.nb = append(c.out.nb, new[:n])   // 进 writev 向量
    toBuffer = toBuffer[n:]
}
```

**关键认知**：一条 5KB 消息会被切成 `4KB(medium) + 512B(small) + 512B(small)`（考虑 tail-top 先填满上一个未满 buffer）。**512B 档不是只服务 ≤512B 的小消息——它是任意大小消息的"尾部碎片/填充"档**。这是对 §2.4.5 表述的重要修正：nbPoolSmall 的高频次来自"每条消息都可能产生一个 512B 尾部"，不是"有很多 512B 小消息"。

**证据 3：归还窗口极短——flush 完即 Put（client.go:1680, 5919-5922）**

```go
// flushOutbound 里，WriteTo 消费完立即归还（client.go:1680）
nbPoolPut(c.out.wnb[i])
// 连接关闭时兜底归还（client.go:5919-5922）
for i := range c.out.nb {
    nbPoolPut(c.out.nb[i])
}
```

`net.Buffers.WriteTo` 在一次 IO 完成后归还所有 buffer。从 `nbPoolGet` 到 `nbPoolPut` 的窗口 = **一次 flush 的时长（µs~ms 级）**，且**同一连接的 flush 串行**。这意味着 512B buffer 周转极快、pool 命中率极高——Get 出来的 buffer 极可能就是上一轮刚 Put 进去的。

**证据 4：tail-topping（client.go:2518-2527）主动减少碎片**——先填满上一个未满 buffer 再申请新的。这进一步坐实 512B 档的"凑整填充"语义：它的存在是为了让 writev 向量里的每个 entry 都尽量填满，减少 syscall 次数和 nb entry 数量。

**综合判断：nbPoolSmall(512B) 满足池化的三个前提，故 nats 池化是对的**

对照本报告 §2.4.2「小字节数组不该池化」的原则（tiny allocator + per-P size class + noscan），nbPoolSmall 是**有意识的反例**，因为它同时满足三个前提：

| 前提 | nbPoolSmall(512B) | 「小对象不该池」原则的前提 |
|---|---|---|
| **极短归还窗口** | 一次 flush（µs~ms），串行，命中率极高 | 跨 goroutine 长期驻留 → 不成立 |
| **超高频次** | 每条消息的尾部碎片都可能触发，出站 QPS 极高 | 偶尔分配 → 不成立 |
| **chunked copy 让 512B 成为任意消息的填充档** | 5KB 消息也用 512B 填尾 | 只服务 ≤512B 小消息 → 不成立 |

省下的是**高频碎片 alloc + memclr**（512B 的 memclr 虽比 64KB 便宜，但乘以极高频次后总量可观），且归还窗口短到 pool 命中率接近 100%。**净收益为正，nats 池化 512B 是 writev chunked-copy 场景下的正确选择**。

**设计依据：mqtt-gateway payload 路径为什么不池 ≤4KB（与 nats 并非矛盾，而是 I/O 模型不同）**

mqtt-gateway 的上行 payload 路径**不具备 nbPoolSmall 池化的三个前提**，故不池化是对的：

| 前提 | nats 出站 nbPoolSmall | mqtt-gateway payload 路径 |
|---|---|---|
| I/O 模型 | `net.Buffers` writev，chunked copy 填充 | single-sink → Kafka，payload 原样进**一个** Record |
| 512B 档的语义 | 任意消息的尾部填充档（高频） | **不存在**——payload 是整块，无"尾部填充"语义 |
| 归还窗口 | flush 完即 Put（µs~ms） | Kafka durable ack（ms~s），跨 goroutine |
| 复用率 | 串行 flush，命中率≈100% | read loop→ingest worker→ack callback，复用率低 |

**核心结论（mqtt-gateway 设计依据）**：

> **buffer 池化阈值不由绝对尺寸决定，由"I/O 模型是否提供『短归还窗口 + 高频次 + 紧凑复用』"决定。**
>
> - nats 出站是 writev chunked-copy → 三前提全成立 → 连 512B 都值得池。
> - mqtt-gateway payload 是 single-sink Kafka Record → 三前提全不成立 → ≤4KB 不池（交给 Go allocator 的 tiny/per-P fast path），只池 4KB/16KB（memclr/mallocgc 成本显著、且即便复用率低也因尺寸大而净收益为正）。
> - 若未来 mqtt-gateway 做下行帧拼装（Broker→device，writev 场景，对应 §6.3），三前提将成立 → 那时引入 512B 档才是对的，与 nats 一致。

这条判据是 mqtt-gateway §6.1（payload 分级池）和 §6.3（outbound 暂缓）的共同设计依据，也是回答"为什么 nats 池 512B、我们不池"的终极答案。

#### 2.4.7 【设计依据】含指针小 struct 的池化：GC mark 与 mcache 正交，sync.Pool 的评估指标

> 本节展开 §2.4.2 的「类 2」，回答一个深层质疑：「对象才几十字节，Go 的 mcache 分配几乎免费（per-P 指针 bump、无锁），为什么 nats 还要为 24~160B 的 struct 建 sync.Pool？这不是过度优化吗？」
> 答案：**mcache 省的是「分配开销」，不省「GC mark/sweep 开销」**；含指针 struct 的 GC mark 成本与对象大小无关、与分配路径无关，只与**指针数量 × 对象存活数量**有关。sync.Pool 通过「复用而非丢弃」让对象**不进 GC 周期**——这是 mcache 给不了的收益，也是这一类池化的根本动机。本节是 mqtt-gateway 后续若引入「含指针 struct 池」（见 §2.4.5 延伸结论 1）的**评估方法论依据**。

##### a. 为什么 GC mark 与 mcache 正交：两个独立的成本轴

Go 的 GC 是并发标记-清除（concurrent mark-sweep），成本分两个阶段，对应两个完全独立的轴：

**轴 1：分配开销（allocation）—— mcache 管这一段，对小对象近乎免费**

```
mallocgc → mcache.fastIndex(sizeclass) → 指针 bump → memclr
```

对 ≤32KB 的对象：per-P mcache 里对应 sizeclass 有空闲 span，分配是**指针 bump + memclr**，无锁、无系统调用。24B 的 struct 走 sizeclass=24（或邻近），分配成本约 20~40ns。**这条轴上，mcache 已经把成本压到接近物理下限，sync.Pool 的 Get/Put（procPin + 原子，几百 ns）反而更贵。** 这正是「小对象不该池化」原则的来源——但**它只看了轴 1**。

**轴 2：GC mark/sweep 开销（marking）—— mcache 完全不管，sync.Pool 管这一段**

```
GC 周期触发 → mark root → 沿指针图遍历 → 每个含指针对象展开扫描其指针字段 → sweep
```

每个**含指针**的对象，GC mark 阶段都要：① 把对象本身作为一个 mark 单元入队；② 展开（scan）它的每个指针字段，跟随指针看指向的对象是否存活。**这部分成本与对象大小无关**（一个 24B 含 1 指针的 Entry 和一个 24B 不含指针的 [24]byte，mark 成本不同），**与分配路径无关**（不管是 mcache 分的还是 sync.Pool 复用的，只要它存活在 heap 上、含指针，mark 阶段一视同仁地扫）。

关键点：**只要对象存活在 heap 上，轴 2 的成本就必然发生，每轮 GC 都发生**。mcache 让分配免费（轴 1≈0），但对象一旦分配出来进入 live heap，轴 2 的 mark 成本就**逃不掉**。

**sync.Pool 的真正价值在轴 2**：通过 `Get`/`Put` 复用，让对象**不变成 transient garbage**（短命垃圾）——它不在「分配→用完→成垃圾→下一轮 GC 回收」的循环里，而是「Get→用→Put→Get→用→Put」持续存活在 pool 里。这带来两个效果：

1. **减少 GC 周期的 mark 工作量**：transient 对象每轮 GC 都要被 mark 一次（即使马上要被回收，mark 阶段也得扫到它）。pool 复用后，稳态下**新产生的 transient 对象数≈0**，mark 工作量只来自 pool 里那批常驻对象（数量固定、可控）。
2. **降低 GC 触发频率**：GC 触发点 ≈ `live_heap × (1 + GOGC/100)`。transient 对象在两次 GC 之间累积成 live heap 增量，是触发 GC 的主因。pool 复用消掉这部分增量 → GC 触发频率下降 → 整体 CPU 中 GC 占比下降。

**所以"含指针小 struct 池化"的本质是：牺牲一点轴 1（pool 的几百 ns Get/Put > mcache 的几十 ns 分配），换取轴 2 的大幅下降（mark 工作量 + GC 频率）。当对象「含指针 + 高频创建销毁」时，轴 2 的节省远大于轴 1 的额外开销，净收益为正。**

##### b. mcache 的能力边界：它救不了含指针高频对象

把 mcache 对"含指针小 struct"的帮助讲精确——**它只让分配便宜，不减少任何 GC 压力**：

| 维度 | mcache 能帮 | mcache 帮不了 |
|---|---|---|
| 分配速度（轴 1） | ✅ per-P 指针 bump，~30ns | — |
| memclr 成本 | ✅ 24B 清零很便宜 | — |
| **GC mark 工作量（轴 2）** | ❌ | 含指针对象每轮 GC 都要扫 |
| **GC 触发频率** | ❌ | transient 对象累积成 live heap 增量 |
| **对象生命周期控制** | ❌ | 分配出去就交给 GC 管了 |

结论：**对于「noscan 的纯字节数组」（§2.4.2 类 1 的小 payload），轴 2 成本极低（无指针可扫），mcache 的轴 1 优势主导 → 不该池化。对于「含指针的高频 struct」，轴 2 成本主导，mcache 帮不上 → 该池化。** 这就是为什么 nats 的 18 个 struct 池全部是含指针类型，而纯字节缓冲只在 ≥512B 且 writev 场景才池化（§2.4.6）。

##### c. nats 的证据：它清楚指针字段的 GC 含义（三处）

**证据 1：`waitingRequest.recycle()` 显式清零指针字段（consumer.go:3980-3984）**

```go
func (wr *waitingRequest) recycle() {
    if wr != nil {
        wr.next, wr.acc, wr.interest, wr.reply = nil, nil, _EMPTY_, _EMPTY_
        wr.priorityGroup = nil   // 含指针字段全部置 nil
        wrPool.Put(wr)
    }
}
```

`waitingRequest`（consumer.go:3947）含 4 个指针/引用字段（`next *waitingRequest`、`acc *Account`、`priorityGroup *PriorityGroup`，外加 string 的底层指针）。Put 前必须把这些字段清零——**不是为了防 use-after-Put（那是正确性问题），而是为了切断指针引用**：不清零的话，复用的 struct 还指着旧的 Account/PriorityGroup，阻止那些大对象被 GC 回收（悬挂引用拖住大 backing object）。nats 这么做说明它**清楚指针字段在 GC mark 中的语义**——池化含指针对象必须配套指针清理。

**证据 2：`Entry{Type; Data []byte}` 池化的是 slice header 的指针成本（raft.go:2689, 2795）**

```go
type Entry struct {
    Type EntryType
    Data []byte   // slice header = (指针, len, cap)，GC 要扫 Data 的 backing array 指针
}
```

24B 的 Entry，看着很小，但 `Data []byte` 的 slice header 含一个指向 backing array 的指针。每轮 GC mark 这个 Entry 时，要跟随这个指针看 backing array 是否存活。Raft 在集群复制热路径上高频创建/销毁 Entry（`raft.go:934,1020,1059,2044,2889...` 十几处 `newEntry`），不池化就是每轮 GC 扫十万个 Entry header。**池化后稳态新 transient Entry≈0，mark 工作量骤降。** 这是"小对象但含指针"该池化的典型。

**证据 3：ipQueue 的 cap 守卫 `ipQueueDefaultMaxRecycleSize = 4096`（ipqueue.go:22, 215-227）——回收价值随尺寸递减的工程体现**

```go
const ipQueueDefaultMaxRecycleSize = 4 * 1024
// recycle():
if cap(*elts) > q.mrs {
    return  // 超过 4096 cap 的 slice 直接丢弃，不归还，让它进 GC
}
```

这条守卫是评估指标的关键证据：**nats 不盲目回收所有对象**。slice 的 cap 超过 4096 就不 Put，任由 GC 回收。理由是——大 slice 占的 live heap 多，pool 留着它会让 pool 的常驻内存膨胀（pool entry 算 live heap，见 §2.4.3）。**这揭示了一个评估指标：pool 的回收价值随对象尺寸递减**——对象越大，pool 占的常驻 live heap 越多，边际收益越低，存在一个"回收上限"。

##### d. sync.Pool 支撑评估的指标体系（选型决策表）

把上述分析收敛成**可量化、可压测的评估指标**。判断"某个含指针 struct 该不该上 sync.Pool"，逐项打分：

| 指标 | 含义 | 量化/判断方式 | 池化倾向 |
|---|---|---|---|
| **① 指针密度** | struct 含指针字段数（含 string/slice header 的隐藏指针） | 看类型定义，数指针字段 | 越高越该池（轴 2 成本大） |
| **② 创建销毁频率** | 热路径上每秒 new 该对象的次数 | pprof alloc profile 的对象计数 | 越高越该池（transient 增量大） |
| **③ 生命周期跨度** | 从 new 到可回收的时长（是否跨 goroutine/跨 RPC） | 代码审查归还路径 | **越短越该池**（长生命周期难归还，易 use-after-Put） |
| **④ 归还复杂度** | Put 前需要清零/重置的字段数 | 看是否含指针（需 nil 化防悬挂） | 越简单越该池（复杂 reset 易出 bug） |
| **⑤ 对象尺寸** | sizeof(struct) + 指向的 backing object | 看字段构成 | 中等最优；过大（>cap 守卫）不归还，过小（noscan）轴 2 无成本 |
| **⑥ noscan 与否** | 对象是否含指针（决定轴 2 是否有成本） | `reflect.TypeOf(x).Kind()` 或看字段 | **含指针才该池；noscan 走 mcache** |
| **⑦ 指向的 backing object 大小** | slice/string 背后拖住的对象 | 如 `Data []byte` 指向几 KB 的 array | 拖住大对象的指针字段必须严格 nil 化 |

**核心判据**：⑥是门槛（noscan 直接否决池化，走 mcache）；①②③④⑤⑦共同决定净收益。**六个维度都偏正向才上 sync.Pool，否则交给 mcache**。

##### e. 对比展开：nats 的三个代表池按指标体系打分

| 指标 | Entry（raft） | waitingRequest（consumer） | jsAckMsg（consumer） |
|---|---|---|---|
| ① 指针密度 | 中（1 slice header） | 高（4 指针字段 + strings） | 中（2 strings + 1 slice） |
| ② 创建频率 | 极高（Raft 复制热路径） | 高（pull 请求） | 高（每条 ack） |
| ③ 生命周期 | 中（跨 RPC） | 中（等待期） | 短（入队即处理） |
| ④ 归还复杂度 | 低（2 字段重置） | 中（4 字段 nil 化） | 低（字段少） |
| ⑤ 尺寸 | 小（~24B） | 中（~160B） | 小（~56B） |
| ⑥ noscan？ | ❌ 含指针 | ❌ 含指针 | ❌ 含指针 |
| ⑦ backing object | Data 的 array | acc/PG 等大对象 | msg 的 array |
| **结论** | ✅ 该池 | ✅ 该池 | ✅ 该池 |

三者都过门槛（⑥含指针）且①②③④正向，故 nats 池化它们是轴 2 净收益为正的正确决策。

##### f. 对 mqtt-gateway 的设计指导

mqtt-gateway 当前的 payload 路径是纯 `[]byte`（noscan，§2.4.2 类 1），**轴 2 无成本，不满足门槛⑥，不该池化小 payload**——这与 §6.1 一致。但报告 §2.4.5 延伸结论 1 提到：若未来从纯 `[]byte` payload 扩展到**含指针字段的结构体**（如带 slice header、tenant 指针的 `RawEnvelope`），则按本节的指标体系评估：

1. **先过门槛⑥**：struct 是否含指针？不含 → 走 mcache，别池。
2. **含指针则逐项打分**：①指针密度 ②创建频率（pprof）③生命周期跨度（能否在 ingest worker / Kafka ack callback 处安全归还）④归还复杂度（含指针字段需配套 nil 化 reset，防悬挂拖住大对象，参考 nats `waitingRequest.recycle()`）⑤尺寸 ⑦backing object。
3. **特别关注 ③④**：mqtt-gateway 的多段生命周期（read loop → ingest worker → Kafka ack callback）使归还点比 nats 更复杂，**含指针 struct 池化的正确性风险集中在归还点的字段清理**——这点比 nats 更难，是 mqtt-gateway 若引入这类池的唯一敬畏点。

**判据**：mqtt-gateway 的含指针 struct 池化（若未来需要）应**在 payload 分级池（§6.1）稳定运行、且 pprof 显示含指针对象成为 GC mark 热点后**才评估引入，且必须配套归还点字段清理机制（仿 nats `recycle()` 模式）。**当前阶段（payload 是 noscan `[]byte`）不引入这类池**，与 §6.1 一致。

---

## 3. 零拷贝流式解析：热路径零拷贝，split 才 copy，deliver 永远 copy

### 3.1 解析器结构（嵌在 client 里，零额外分配）

`parser.go:25-36` 的 `parseState` 直接内嵌于 `client`（`client.go:268`）：

```go
type parseState struct {
    state   parserState
    op      byte
    as      int            // 参数起始下标（进 buf）
    drop    int            // CR 丢弃计数
    pa      pubArg         // 已解析的 PUB 参数
    argBuf  []byte         // 仅 split 时分配
    msgBuf  []byte         // 仅 split 时分配
    header  http.Header
    scratch [MAX_CONTROL_LINE_SIZE]byte    // 4096B 固定数组，不进堆
    argsa   [MAX_HMSG_ARGS + 1][]byte      // 参数指针数组，固定
}
```

### 3.2 快速路径：命令参数零拷贝

`parser.go:424-444` `PUB_ARG` state —— 命中 `\n` 时直接切 `buf`：

```go
case '\n':
    var arg []byte
    if c.argBuf != nil {
        arg = c.argBuf              // split 路径：用预拷贝缓冲
        c.argBuf = nil
    } else {
        arg = buf[c.as : i-c.drop]  // 零拷贝：直接 slice 读缓冲
    }
    if err := c.processPub(arg); err != nil { return err }
```

所有 `*_ARG` state（SUB / UNSUB / CONNECT / INFO / HPUB 等）同模式：`c.argBuf == nil` 即快速路径。

### 3.3 快速路径：payload 跳扫（不逐字节）

`parser.go:446-452` —— 解析出 `pa.size` 后**直接把游标跳过 payload**：

```go
c.drop, c.as, c.state = 0, i+1, MSG_PAYLOAD
if c.msgBuf == nil {
    i = c.as + c.pa.size - LEN_CR_LF   // 一次跳过整个 payload
}
```

到 `MSG_END_N`（`parser.go:491-499`）也是直接切 `buf`：

```go
case MSG_END_N:
    if b != '\n' { goto parseErr }
    if c.msgBuf != nil {
        c.msgBuf = append(c.msgBuf, b)
    } else {
        c.msgBuf = buf[c.as : i+1]     // 零拷贝：直接 slice 读缓冲
    }
```

随后 `processInboundMsg(c.msgBuf)`（`parser.go:520`）拿到的 `msgBuf` 是读缓冲的别名引用，**热路径全程零拷贝**。

### 3.4 split 慢路径：copy 进内嵌 scratch

若参数 / payload 横跨两次 `Read`（end-of-parse 时仍处 `*_ARG` 或 `MSG_PAYLOAD` state），`parser.go:1182-1229` 检测到后 copy 进 `scratch[4096]`；payload 超 4KB 则 `make` 一块专用 buffer：

```go
if c.pa.size > cap(c.scratch)-len(c.argBuf) {
    c.msgBuf = make([]byte, lrem, c.pa.size+LEN_CR_LF)
    copy(c.msgBuf, buf[c.as:])
} else {
    c.msgBuf = c.scratch[len(c.argBuf):len(c.argBuf)]
    c.msgBuf = append(c.msgBuf, buf[c.as:]...)
}
```

### 3.5 分发：永远 copy

`processInboundMsg → deliverMsg → queueOutbound` 这一段**必然 copy 进 `nbPool` 帧缓冲**（见 §2.1）。原因：读缓冲要被下一次 `Read` 覆盖，订阅者未必及时取走。

### 3.6 拷贝语义一览

| 数据 | 快速路径 | split 路径 | 分发 |
|---|---|---|---|
| 命令参数（topic / size / ...） | 零拷贝 `buf[as:i-drop]` | copy 进 `scratch[4096]` | — |
| 消息 payload | 零拷贝 `buf[as:i+1]` | copy 进 `scratch` 或 `make()` | — |
| 投递到订阅者 | — | — | **永远 copy 进 `nbPool`** |

---

## 4. 整体缓冲生命周期

```
ALLOC
├─ 读缓冲：make([]byte, 512) per readLoop goroutine（不入池）
│   └─ 自适应：满读 ×2、短读 ÷2，区间 [64, 65536]
├─ parseState：内嵌 client，scratch[4096] + argsa[8][]byte，零堆分配
└─ outbound 缓冲：nbPoolSmall/Medium/Large（512B/4KB/64KB 分级）

READ → PARSE（in-place，热路径零拷贝）
├─ reader.Read(b)
├─ parse(buf) 逐字节扫命令，遇 payload 一次性跳过
└─ processInboundMsg(msgBuf) 同步分发；msgBuf 是 buf 的别名（热路径）

DISPATCH（永远 copy）
└─ queueOutbound(mh/msg) → copy 进 nbPool 帧 → 追加到 net.Buffers（writev）

REUSE / DISPOSAL
├─ 读缓冲：下一轮 Read 覆盖
├─ argBuf/msgBuf：每包后置 nil（parser.go:523）
├─ scratch：随 argBuf=scratch[:0] 隐式复用
└─ nbPool 帧：flushOutbound/writeLoop 后 Put 回池
```

六条关键工程取舍：

1. **不用 `bufio.Reader`**，自管缓冲拿全控。
2. **parser 是热路径核心**：jump-ahead 跳 payload，单 64KB 读可一次性 parse 多条完整 PUB，O(buffer) 而非 O(payload sum)。
3. **零拷贝解析 + 拷贝分发**：解析不 copy，分发必 copy（读缓冲要让位）。
4. **outbound 分级池**匹配典型消息体量，避免过分配 + 高频 GC。
5. **struct 指针也池化**（JetStream / Raft / Consumer），不只字节数组。
6. **读缓冲不池**：per-goroutine 独占、与连接等长，比 pool 更简单可证。

---

## 5. 对 `mqtt-gateway` 的对照

### 5.1 当前实现速览

- `mqtt/packet.go:46` `ReadPacket`：无应用级读缓冲，直接 `io.ReadFull(conn, …)`。
- `mqtt/packet.go:69` `readRemainingLength`：**逐字节 syscall** 读变长字段（最多 4 字节）。
- `mqtt/packet.go:60` body：**每包 `make([]byte, remaining)` 独立分配**。
- `mqtt/publish.go:80` payload：`r.buf[r.pos:]` —— **别名 `pkt.Body`**（不是别名读缓冲，是别名 `ReadPacket` 已分配的 exact-size body）。
- `server/server.go:340-374` `handlePublish`：QoS0 走 `TryAppendAsync`，QoS1 走 `SubmitAndWait`，ack 后才回 PUBACK。
- 全项目**无 `sync.Pool`**（待 grep 复核；首稿基于已读源码）。

### 5.2 对照表

| 维度 | nats-server | mqtt-gateway 现状 | 是否可借鉴 |
|---|---|---|---|
| 读缓冲 | 1 份/连接，反复复用，自适应 | 无应用级缓冲，`io.ReadFull` 直读 | **部分**（见 §6.2） |
| 变长字段读取 | parser 一次性扫完 | `readRemainingLength` 逐字节 syscall | **可借鉴批量读**（§6.2） |
| body 分配 | 零拷贝切片进读缓冲 | 每包 `make([]byte, remaining)` | **可借鉴池化**（§6.1） |
| payload 引用语义 | 零拷贝别名读缓冲 | 别名 `pkt.Body`（exact-size 独占） | **不可借鉴零拷贝进读缓冲**（§7.1） |
| outbound 池化 | 27 个 sync.Pool（§2.4.1），含 nbPool writev 模型 | 无 | **条件可借鉴**：上行 payload 不照搬（§6.1+§2.4.6）；下行 writev 场景才引入 512B 档（§6.3+§2.4.6） |
| 状态机 in-place 解析 | 是 | `ParsePublish` 已 in-place 于 body | 已对齐 |
| 多包合并 parse | jump-ahead 一次解析多条 | 每包独立处理 | **不可借鉴**（§7.2） |

---

## 6. 可借鉴清单（按 ROI 排序）

### 6.1 【高 ROI】per-packet body 池化

**问题**：`packet.go:60` `body := make([]byte, remaining)` 每包一次堆分配。百万设备 × 每 5 分钟一条 ≈ 3.3k 包/秒，QoS1 在 `SubmitAndWait` 期间还要驻留，对 GC 是稳定压力。

**借鉴方案**：引入分级 `sync.Pool`，`Get` 后 `[:remaining]` 切精确长度，**在 Kafka ack 回执之后 Put 回池**：

- `server.go:364` `SubmitAndWait` 返回 `nil` 之后 → 归还 QoS1 buffer；
- `server.go:361` `TryAppendAsync` 异步路径 → 需要在 queue 内部 release，**不能在 `handlePublish` 内归还**（异步还没消费）。

**分档阈值（2026-06-17 修订）**：原方案列了 256B / 4KB / 64KB 三档，经复核**砍掉 256B 小档、把 64KB 调整为 16KB（对齐 `max_payload_size`）**，定稿为 **4KB / 16KB 两档**。理由：Go 运行时对 ≤16B 的 noscan 对象走 tiny allocator（多对象合并、per-P 指针 bump、无锁），对 ≤32KB 走 per-P size class（锁-free），且 `[]byte` 是 noscan（GC 不扫描内容）。因此小 slice 池化时 pool 的 `Get`/`Put` 开销（procPin + 原子，几百 ns）> 它省下的分配成本（几十 ns），GC 成本两边几乎相同，且 pool entry 算 live heap、每 GC 周期被 `poolCleanup` 清一次——**池化小对象既占内存又没省 GC，净负收益**。池化只对大 payload（memclr / mallocgc / live-bytes 成本显著）成立。**小 payload（≤ 4KB）直接 `make`，不进池**。分档阈值必须由 pprof alloc profile 的真实尺寸分布压测确认，可按真实负载加中间档，但禁止预置 ≤256B 小档。

> **为什么 nats 的 nbPoolSmall 才 512B 都池化了，我们却不池 ≤4KB？** 两者都对，差异不在尺寸阈值，在 I/O 模型——详见 §2.4.6 的 4 处源码证据。一句话：nats 出站是 `net.Buffers` writev chunked-copy，512B 是**任意大小消息的尾部填充档**（高频 + 归还窗口 µs~ms + 命中率≈100%，三前提全成立）；我们 payload 是 single-sink Kafka Record，无尾部填充语义、归还窗口 ms~s、复用率低，三前提全不成立。**池化阈值不由绝对尺寸决定，由 I/O 模型决定**——这是本条分档的核心设计依据。

**红线契合性**：红线 §5.1.1「payload buffer 的分配来源」/ §5.1「先预算后分配」仍满足——池化只是分配来源优化，**生命周期仍是 exact-size、仍 bounded、仍 per-client + global byte budget 双重预算**。需要把池化归还点接入 §5.1 的预算释放点。

**风险点**：池化的 buffer 若被 queue 异步路径持有，必须保证**消费完才归还**，否则 use-after-Put。这是接入时最容易出 bug 的地方。

**为何不仿 NATS 读缓冲方案**：NATS 的"per-goroutine 不入池"哲学只对其读缓冲成立——它是 fan-out broker，deliver 时必然 copy 进 nbPool，解析期零拷贝是净收益。本 broker 是 single-sink → Kafka，payload 要跨 goroutine 驻留（read loop → ingest worker → ack callback），不能别名读缓冲（§7.1）；读缓冲不能做大（§7.3），也就拿不到"一次 64KB 读装多条 PUB + jump-ahead"的收益。故 payload buffer 只能走 sync.Pool 分级池，不照搬 NATS 读缓冲模型。本结论已收口进 doc 7 §5.1.1。

### 6.2 【中 ROI】小读缓冲吸收 header + control 包

**问题**：MQTT fixed header（1B type/flags + ≤4B remaining length）+ control 包（PINGREQ 2B / PUBACK 4B / DISCONNECT 2B）当前要拆 2–4 次 syscall。

**借鉴方案**：64B 栈缓冲批量读 header（最多 5B），body 仍走独立 `io.ReadFull`。**不要做 nats 的大块读缓冲 + 跨包共享**（与红线 §3 冲突，见 §7.1）。

**红线契合性**：栈缓冲仅用于 header，body 仍独立分配，红线 §3 不受影响。

### 6.3 【中 ROI】分级 outbound 池（如果后续做下行）

**问题**：第一阶段暂无 Broker→device 主动写路径（控制通道 TODO），但 CONNACK / PUBACK / PINGRESP 已经是 `EncodeConnAck(nil, …)` 这种「每次 append 到 nil slice」的零启动分配。

**借鉴方案**：将来做下行 / 批量 CONNACK 时，引入 `nbPoolSmall`（512B）匹配典型控制帧尺寸。**当前阶段（单连接单写）ROI 偏低**，等下行通道那圈再评估。

> **何时该引入的判据（设计依据见 §2.4.6）**：只有当 mqtt-gateway 的下行路径具备 writev chunked-copy 模型（批量帧聚合 + `net.Buffers` 一次 syscall 写出）时，"短归还窗口 + 高频次 + 紧凑复用"三前提才成立，此时引入 512B 档才与 nats 一致地净收益为正。若下行仍是单连接单写（每次一个 `EncodeConnAck` append 到 nil slice），三前提不成立，512B 池是负收益——这正是当前阶段 ROI 偏低的根本原因，不是"还没空做"。

---

## 7. 不可借鉴清单（红线约束）

### 7.1 【禁止】零拷贝别名大读缓冲

nats 的热路径核心——payload 零拷贝切片进 64KB 读缓冲——**与红线 §3 直接冲突**。原因：

- nats 是 **fan-out**：一条 PUB 投递给 N 个订阅者，deliver 时必然 copy 进 nbPool，所以解析层零拷贝是净收益。
- 我们是 **single-sink → Kafka**：raw log ack 周期（毫秒到秒级）远长于一次 Read（微秒级）。若 payload 别名读缓冲，下一次 `Read` 会**覆盖未确认数据**。
- 红线 §3 已显式写明「禁止 queue 里引用大 read buffer 的 subslice」，当前 `ParsePublish` 别名 `pkt.Body`（exact-size 独占分配）是合规的最优解。

**结论**：nats 的 `parser.go:491-499` `msgBuf = buf[c.as : i+1]` 这一行绝对不能学。

### 7.2 【禁止】大块读缓冲 + 跨包共享 + jump-ahead

nats 的 `parser.go:446-452` `i = c.as + c.pa.size - LEN_CR_LF`（一次性跳过整个 payload）依赖「单次 64KB 读装多条完整 PUB」。这要求读缓冲被多包共享，同样踩红线 §3。

**结论**：我们每包独立处理（`ReadPacket` 一包一读一 body），拿不到 jump-ahead 的收益，强行做会和 §3 冲突。

### 7.3 【禁止】读缓冲动态 grow 到 64KB

`client.go:1599` grow 到 64KB 是因为 nats 要一次装多条 PUB。我们每包独立处理，64KB 读缓冲的复用收益拿不到，反而：

- 增大每连接稳态内存（百万连接 × 64KB = 64GB，红线 §3/§4 直接爆）；
- 触发红线 §3 的「大 read buffer」反模式。

**结论**：读缓冲要么不引入（现状），要么只做 §6.2 的小栈 header 缓冲（64B 级）。

---

## 8. 决策汇总

| 借鉴项 | 决策 | 触发时机 | 关联红线 |
|---|---|---|---|
| per-packet body 分级 sync.Pool | **采纳** | QoS1 性能压测前 | §3 / §4 / §7（release 点） |
| 64B 栈 header 批量读 | **采纳** | 与上一项同批 | §3（不动） |
| outbound 分级池（nbPool） | **暂缓** | 下行控制通道那圈 | — |
| 零拷贝别名大读缓冲 | **禁止** | — | §3 |
| 大块读缓冲 + jump-ahead | **禁止** | — | §3 |
| 读缓冲动态 grow 到 64KB | **禁止** | — | §3 / §4 |

---

## 9. 待补充清单（后续追加）

> 本文档定位为「nats-server 借鉴分析的活页」，后续分析追加在对应小节。

- [ ] **outbound / write path**：nats 的 `queueOutbound` + `net.Buffers`（writev）写路径完整分析，为后续 Broker→device 下行通道设计铺垫。
- [ ] **TLS 路径**：nats 的 TLS buffer / `decompressorPool`（websocket permessage-deflate）是否有可借鉴的池化模式。
- [ ] **JetStream append 路径**：nats 的 `jsPubMsgPool` / `fastBatchPool` 的批量写模式，对比我们的 Kafka producer batch。
- [ ] **慢消费者 / 背压**：nats 的 subscription pending 字节数 / drop 策略，对照[6.背压设计](./6.cloud-mqtt-gateway-pressure-backpressure-design.md)。
- [ ] **`ipqueue` 通用队列**：nats 的 `ipqueue.go` 分片 / 批处理实现，对照我们的 `ingest/queue.go`。
- [ ] **百万连接内存账本**：按 nats 的 per-conn 内存结构反推我们 broker 的稳态 RSS 预估（喂给[4.成本分析](./4.cloud-mqtt-gateway-cost-analysis.md)）。

---

## 附录 A：源码引用速查

| 主题 | 文件 | 行 |
|---|---|---|
| readLoop 入口 | `client.go` | 1403 |
| 读缓冲首次分配 | `client.go` | 1442 |
| 读缓冲尺寸常量 | `client.go` | 111-113 |
| 读主循环 | `client.go` | 1461-1477 |
| 读缓冲 grow/shrink | `client.go` | 1592-1607 |
| `nbPoolSmall/Medium/Large` | `client.go` | 369-388 |
| `nbPoolGet` 分级选择 | `client.go` | 393-401 |
| `queueOutbound` copy | `client.go` | 2532-2536 |
| `parseState` 结构 | `parser.go` | 25-36 |
| `parse()` 入口 | `parser.go` | 137 |
| `PUB_ARG` 零拷贝 | `parser.go` | 424-444 |
| payload jump-ahead | `parser.go` | 446-452 |
| `MSG_END_N` 零拷贝 | `parser.go` | 491-499 |
| split arg copy | `parser.go` | 1182-1200 |
| split payload copy | `parser.go` | 1203-1229 |
| Filestore 分级块池 | `filestore.go` | 1004-1027 |
| WebSocket decompressor 池 | `websocket.go` | 100 |
| `ipqueue` 通用池 | `ipqueue.go` | 89-95 |
