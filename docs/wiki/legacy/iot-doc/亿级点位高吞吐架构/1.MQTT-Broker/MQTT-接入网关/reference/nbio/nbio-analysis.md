# nbio 源码拆解：百万连接是怎么做到的，mqtt-gateway 能借鉴什么

> 对 [nbio](https://github.com/lesismal/nbio)（Go 自研 epoll/kqueue reactor 网络库）做逐文件源码拆解，回答两个问题：① 它凭什么能稳跑 **1M WebSocket 连接 / 100k TPS / 内存 ~1GB**；② mqtt-gateway 现状的阻塞 goroutine-per-conn 模型能从它借鉴什么。
>
> 本文是**源码拆解 + 借鉴清单**，与同目录的 [gnet 可行性评估](../gnet/gnet-feasibility-assessment.md) 是**互补关系**，不是二选一：
> - gnet 评估回答"**要不要换 reactor**"（基于内存/GC/STW 的量化论证，结论：明文多数派值得评估）。
> - 本文回答"**换的话 reactor 长什么样、有哪些可偷的设计**"（基于 nbio 源码的逐点拆解）。
>
> 两篇一起读：先 gnet 评估看 ROI，再看本文看具体技术细节。
>
> 配套：[GC mark 计算模型](../go-gc-mark-cost-model.md)、[容量规划](../../11.cloud-mqtt-gateway-capacity-planning.md)、[TLS 接入设计](../../network-defense/13.cloud-mqtt-gateway-tls-design.md)、[实现红线](../../7.cloud-mqtt-gateway-implementation-red-lines.md)。

---

## 0. 结论先行

```text
nbio 稳跑百万连接，靠的不是某一行黑魔法，而是"反 std net 模型"的整套资源纪律。
拆成 8 个机制，按对 mqtt-gateway 的杠杆从大到小：

  ① 去掉"每连接 1 goroutine"        —— 第一性因素，1M 连接 1M goroutine → 几十个
  ② 连接表是 fd 索引数组，不是 map   —— 百万级 map 的 GC/锁全消除
  ③ poller 级共享读 buffer           —— 读路径 GC churn 与连接数解耦
  ④ epoll 批量取事件（1024/次）      —— syscall 摊薄
  ⑤ per-conn jobList 串行化          —— 无锁的"每 conn 一个逻辑流"
  ⑥ writev + 尾包合并                —— 写 syscall 数量降一个数量级
  ⑦ mempool（sync.Pool）回收写 buffer —— 写路径 GC churn 消除
  ⑧ MaxConnReadTimesPerEventLoop 封顶 —— 公平性兜底，防慢连接饿死 poller

mqtt-gateway 现状正是 nbio README 警告的 std 阻塞模型：
  - server.go:158  `go s.handleConn(conn)`       → 1 conn = 1 goroutine（撞 goroutine 红线）
  - server.go:83   `map[net.Conn]struct{}`       → 百万级 map
  - packet.go:63   `make([]byte, remaining)`     → 每包 malloc（注释自己标注了待改）

判断：mqtt-gateway 的"业务纪律"（红线 #1-#9、bounded queue、fencing）已到位，
      缺的是"I/O 模型"那一层。这正是 nbio 唯一真正能帮到的地方。
```

ROI 排序的借鉴清单见 [§10](#10-借鉴清单按-roi-排序)。

---

## 1. nbio 是什么

- 纯 Go 的 epoll（Linux）/ kqueue（BSD/Darwin）reactor 网络库，对标 evio/gnet，但 API 更接近"可注册回调的 net"。
- 主仓库 `nbio/` 是底层 transport；`nbhttp/` 是建在其上的 HTTP/WebSocket 实现（含自研 TLS）。
- README 基准：**1M WebSocket 连接 / 1k payload echo / 内存 1GB / TPS 104713**（[README §1M Websocket Connections Benchmark](https://github.com/lesismal/nbio#1m-websocket-connections-benchmark)，硬件 10 核）。
- 自我定位（README 原文）：*"For massive connection scenarios, combined with memory pools and goroutine pools, it can effectively reduce the number of goroutines and objects, thereby lowering memory consumption and GC pressure, avoiding OOM and significant STW."* —— 注意它把"大连接数"和"mempool + goroutine pool"并列为前提，这正是 §3、§7、§9 要拆的。

源码引用下文统一格式：`文件:行号`，对应 `mqtt-gateway/../nbio/` 目录下的 vendored 源码。

---

## 2. 机制 ①：没有"每连接一个 goroutine"（第一性因素）

**这是 nbio 能上百万的根本原因，其它机制都是为它服务的。**

### 2.1 std net 模型的隐含约束

标准库 `net` 是 blocking Read：`conn.Read()` 阻塞直到有数据。要用一个连接就必须有一个 goroutine 卡在 Read 上。于是 **1 conn = 1 goroutine** 是 std 模型的硬约束。1M 连接的代价：

| 项 | 量级 | 说明 |
|---|---|---|
| goroutine 栈 RSS | ≥2GB（最小 2KB 起步，实际多在 8KB） | 栈由独立分配器管，不在 GC heap，但吃 RSS/GOMEMLIMIT |
| 栈指针（GC 根） | ~200万（50万 conn × ~4 指针/栈） | mark 阶段要扫，残留塞进 STW → 5-15ms（[GC 模型 §8.2](../go-gc-mark-cost-model.md)） |
| 调度器开销 | 1M runnable 扫描 | runtime scheduler 每 tick 扫 runnable 队列 |

### 2.2 nbio 的反向做法：N 个 poller goroutine 管全部 fd

```go
// nbhttp/engine.go:1011-1018  NewEngine 的默认 NPoller
if conf.NPoller <= 0 {
    conf.NPoller = runtime.NumCPU() / 4   // LT 模式默认 CPU/4
    if conf.AsyncReadInPoller && conf.EpollMod == nbio.EPOLLET {
        conf.NPoller = 1                  // ET 异步读模式默认 1 个 poller
    }
    if conf.NPoller == 0 {
        conf.NPoller = 1
    }
}
```

```go
// engine_unix.go:70-87  start 时为每个 poller 起一个 goroutine、分配一个读 buffer
for i := 0; i < g.NPoller; i++ {
    g.pollers[i] = &poller{...}
    g.pollers[i].ReadBuffer = make([]byte, g.ReadBufferSize)  // 默认 64KB，见 engine.go:23
    go g.pollers[i].readWriteLoop()                           // 一个 poller 一个 goroutine
}
```

结果：**goroutine 总数 = NPoller（几十）+ worker pool（固定）**，与连接数无关。10 核机器默认 ~3 个 poller goroutine 管 100 万连接。这正是 [CLAUDE.md goroutine 红线](../../CLAUDE.md)（`总数 ≤ conn × 1.x`）的极致版——它做到了 ×0.0001。

### 2.3 对 mqtt-gateway 的含义

`server.go:158` 的 `go s.handleConn(conn)` 是 std 模型的典型写法。要做到百万，这一行必须消失。但**消失的方式有两种**：

- **换 nbio**（[§11](#11-借鉴点-1换-nbio-作为-transport-最高-roi-but-最大坑是-tls)）：直接拿 nbio 的 poller + 回调。
- **借 nbio 的 poller 思路自研**（工作量更大，但可控）。

gnet 评估 [§5](../gnet/gnet-feasibility-assessment.md) 已论证：在"NLB 卸载 TLS 明文多数派 + 慢层已解耦"两前提下，这个改造的复杂度收敛到 **codec 增量化 + CONNECT 状态机**两个包，不是全量重写。

---

## 3. 机制 ②：连接表是 fd 索引数组，不是 map

```go
// engine_unix.go:28  MaxOpenFiles 长度的数组，按下标 = fd
g.connsUnix = make([]*Conn, MaxOpenFiles)   // MaxOpenFiles = 2M（setrlimit）

// poller_epoll.go:89  注册时直接按下标塞
p.g.connsUnix[fd] = c

// poller_epoll.go:248  事件回来时按下标取，O(1)，无 hash、无 mutex
c := g.connsUnix[int(fd)]
```

### 3.1 为什么这个点被严重低估

百万级 `map[int]*Conn` 本身就很贵：

| 项 | map 方案（mqtt-gateway 现状） | 数组方案（nbio） |
|---|---|---|
| 查找 | hash + 桶遍历 + 可能扩容 | 数组下标，O(1) |
| 并发 | 必须mutex（`server.go:82 s.mu`） | 无锁（poller 内 fd 唯一归属） |
| GC | 扫 1M entry（key+value+溢出桶） | 扫一个 `[]*Conn` 切片头 + 指针 |
| 内存 | bucket 数组 + 渐进扩容副本 | 一个连续数组，2M × 8B = 16MB |

### 3.2 mqtt-gateway 现状的对应位置

`server.go:83`：
```go
conns   map[net.Conn]struct{}   // 仅用于 Stop 时强制关闭所有连接
```
配合 `server.go:82` 的 `mu sync.Mutex`，每次 `trackConn`/`untrackConn`（accept/断连）都加锁。百万连接下：
- GC 扫 1M entry（`net.Conn` 是 interface，value 是空 struct 但桶还在）；
- accept 风暴时 mutex 争用（日出百万设备重连，accept 线程和断连 handler 抢同一把锁）。

**借鉴前提**：换了 reactor 拿到裸 fd 后，这张表才能从 map 改数组。明文路径可行；mTLS 少数派继续 map 无妨（量小）。

---

## 4. 机制 ③：poller 级共享读 buffer（读路径零分配）

```go
// engine_unix.go:87  每个 poller 只分配一次 64KB buffer（与连接数无关）
g.pollers[i].ReadBuffer = make([]byte, g.ReadBufferSize)   // DefaultReadBufferSize = 64KB, engine.go:23

// engine.go:378-396  默认的 OnReadBufferAlloc 直接返回 poller 自己的 buffer 指针
func (g *Engine) OnReadBufferAlloc(h func(c *Conn) *[]byte) {
    g.onReadBufferAlloc = h
}
// 读事件触发时：buffer 是"借"给回调的，用完 onReadBufferFree 还回去
```

### 4.1 关键语义：借出 → 回收，不是拥有

`onData` 回调拿到的 `[]byte` 是 **poller 的 buffer 的引用**，处理完必须 `Free`（或回调返回后自动回收）。这意味着：
- N 个 poller = N 个 64KB buffer，**总读 buffer 内存恒定 = NPoller × 64KB**，与连接数完全解耦。
- 读路径**零 malloc**，无 GC churn。

### 4.2 mqtt-gateway 现状对比

`packet.go:63`（注释已自承认是热路径待改）：
```go
body := make([]byte, remaining)   // 每个 PUBLISH 包一次 malloc
if _, err := io.ReadFull(r, body); err != nil { ... }
```

设备高频上报场景（逆变器秒级 PUBLISH），1M 连接 × 每秒 1 包 = 100万次 malloc/秒 → 分配器 + GC 压力直接拉满。这是 mqtt-gateway 读路径最大的 GC churn 来源，**比"goroutine 栈"更容易先撞墙**。

### 4.3 借鉴分两步

- **短期（不换 nbio）**：把 `make([]byte, remaining)` 换成 `sync.Pool` 取。直接抄 nbio `mempool/mempool.go`（几十行，基于 `sync.Pool` + 按 size class 分桶）。**这是全文档 ROI 最高、风险最低的单点改造**（[§13 借鉴点 2](#13-借鉴点-2读-buffer-池化不换-nbio-也该做)）。
- **长期（换 nbio 后）**：用 poller 共享 buffer + 借出语义。

⚠️ **与红线 #3 的冲突要重审**：红线 #3 要求"payload 必须 bounded-memory，先预算后分配，禁止 queue 里引用大 read buffer 的 subslice"。nbio 的"借出"语义正好是"引用 read buffer 的 subslice"——**换 nbio 后，`handlePublish` 里仍需把 payload copy 到独立 buffer 再交给 ingest queue**。这一点 mqtt-gateway 现在靠"`pp.Payload 别名 pkt.Body`，ReadPacket 每包独占分配"绕过（`server.go:355` 注释）。换共享 buffer 后这个假设失效，必须显式 copy。算进改造工作量。

---

## 5. 机制 ④：epoll 批量取事件（syscall 摊薄）

```go
// poller_epoll.go:225  每次最多取 1024 个事件
events := make([]syscall.EpollEvent, 1024)

// poller_epoll.go:236  一次 syscall 拿一批
n, err := syscall.EpollWait(p.epfd, events, msec)
```

`EpollWait` 一次返回最多 1024 个就绪 fd，在 poller loop 里批量处理。对比"每连接独立 Read 阻塞"：后者每个连接的唤醒是一次独立的调度，前者把 1024 个连接的唤醒合并成一次 syscall + 一次 loop 迭代。

这对"百万连接 + 低 QPS/连接"（逆变器典型：每设备秒级 1 包）特别有利——大量连接同时有数据时，批处理摊薄了 syscall 和调度成本。

### 5.1 mqtt-gateway 能否直接借

不能——阻塞模型下没有"批量就绪"的概念，每个 goroutine 独立 Read。这是 reactor 模型独有的收益，归到"换 nbio"那一档。

---

## 6. 机制 ⑤：per-conn jobList 串行化（无锁的"每 conn 一个逻辑流"）

```go
// conn.go:185  Execute 把 job 推到 conn 的 jobList
func (c *Conn) Execute(job func()) bool {
    // ...
    isHead := (len(c.jobList) == 0)
    c.jobList = append(c.jobList, job)
    // 只有队首 job 才真正调度到共享 worker pool 去跑
    if isHead {
        // ... go run all jobs in c.jobList sequentially
    }
}

// conn.go:174-176  注释原文（关键不变量）
// This guarantees there's at most one flow or goroutine running job/jobs
// for one connection at the same time.
// This guarantees all the jobs are executed in order.
```

### 6.1 这解决什么问题

reactor 模型下，一个连接可能同时有**两类回调**：
- **读回调**（poller 收到数据）；
- **写回调**（异步回执到了，要发 PUBACK / 下行命令）。

如果两者并发跑，会撞同一 session 状态。加 mutex 是常规解法，但 mutex 在高并发下会阻塞唤醒、有 cache line 争用。

nbio 的 `jobList` 做法：**每 conn 一个 FIFO 队列，保证同一时刻只有一个 job 在跑、且按序**。新 job 来了，如果队列空 → 调度一个 worker 顺序跑完队列；如果非空 → 只入队（worker 会顺带跑）。这是**无锁的串行化**——没有 mutex 阻塞，只有"是否队首"的一次判断。

### 6.2 对 mqtt-gateway 红线 #7 的直接价值

红线 #7：
> append callback 不直接操作 session：callback 只产出 `AppendResult{ConnID, SessionVersion}`，经 bounded 回执队列投递回 session 拥有者，用 `ConnID + SessionVersion` fencing 校验后才发 PUBACK。

换 reactor 后，"回执到达 → 发 PUBACK"这个回调，和"poller 收到下一个 PUBLISH"这个读回调，**可能并发撞同一 session**。jobList 模式天然提供：
- 串行点（每 conn 同时只有一个流）；
- 有序（PUBACK 一定在它对应的 append 之后发出）；
- 无锁（不阻塞 poller）。

这比 mqtt-gateway 给 session 加 mutex 更轻，且和红线 #2"Session 必须 pointer-poor，禁止 channel"兼容——jobList 在 conn 上，不在 session 上。

**这是 nbio 设计里最值得 mqtt-gateway 抄的"模式"**，即使不换 nbio，下行通道（[设计 14](../../14.cloud-mqtt-gateway-downlink-topic-routing-design.md)）实现时也该用这个模式来串行化"下行投递回调"和"读回调"。

---

## 7. 机制 ⑥⑦：写路径的 writev 合并 + mempool 回收

### 7.1 writev + 尾包合并（机制 ⑥）

```go
// conn_unix.go:33  写不完的数据 copy 进 pooled buffer，累积到阈值再 flush
func (c *Conn) newToWriteBuf(buf []byte) { ... }

// conn_unix.go:402  多个 toWrite 用 writev 一次写出
n, err = c.writev(in)

// conn_unix.go:734  writev 实现
func (c *Conn) writev(in [][]byte) (int, error) { ... }
```

nbio 的写策略：多次 `Write()` 调用先攒在 `toWrite` 队列里，达到 `MaxWriteCacheOrFlushSize`（默认 64KB，可配）才真正 `flush`，且用 `writev` 把队列里多个 buffer 一次 syscall 写出。

收益：广播/下行场景（一个 broker 给大量设备发命令），syscall 数量降一个数量级。对逆变器 OTA / 命令下发的批量场景尤其有用。

### 7.2 mempool 回收写 buffer（机制 ⑦）

```go
// conn_unix.go:81  写完的 buffer 还回 mempool（基于 sync.Pool）
func (c *Conn) releaseToWrite(t *toWrite) { ... }

// mempool/allocator.go  全局 DefaultMemPool
var DefaultMemPool = New(1024, 1024*1024*1024)   // bufSize=1KB, freeSize=1GB
```

`mempool` 是 `sync.Pool` 的封装 + 按 size 分桶 + 总空闲内存上限（`freeSize`，默认 1GB）。写 buffer 用完不丢，回池复用，消除写路径 GC churn。

### 7.3 对 mqtt-gateway 的两个注意

1. **合并写不能破红线 #1（PUBACK 时序）**：QoS1 的 PUBACK 必须在 durable ack 之后。nbio 的"尾包合并到 64KB 再 flush"发生在 **socket 层**，durable ack 是**上层独立信号**——只要 PUBACK 编码进 toWrite 队列的时刻在 durable ack 之后，合并 flush 不影响语义。但实现时要确保：durable ack 回执 → 进 jobList → encode PUBACK 进 toWrite，这条链路里 encode 时刻就是时序锚点，flush 延迟只是传输层优化。
2. **`freeSize=1GB` 默认值要按 mqtt-gateway 红线 #4 重设**：红线 #4 要求"所有 queue 必须有 bytes 上限"。mempool 的 `freeSize` 是"池里能囤多少空闲内存"，不是"分配上限"，但仍吃 RSS。要和 `MaxQueueBytes` 协同配置，否则池子本身成了隐性无界内存。

---

## 8. 机制 ⑧：MaxConnReadTimesPerEventLoop（公平性兜底）

```go
// engine.go:29  默认每个 fd 每轮 poller loop 最多读 3 次
DefaultMaxConnReadTimesPerEventLoop = 3

// conn_unix.go:166 / 206  读循环封顶
for i := 0; i < g.MaxConnReadTimesPerEventLoop; i++ {
    // read once
}
```

即便用 ET 模式（边缘触发，数据来了必须读完），也不会让一个慢连接一次灌大包把整个 poller loop 占死——读完 3 次就 yield，让其它连接有机会处理。

### 8.1 这个点对 mqtt-gateway 的安全意义

如果 mqtt-gateway 自研 reactor 但**漏了这层封顶**，会出现：
- 一个设备（或攻击者）一次发超大 PUBLISH，poller 卡在它的读循环里；
- 同 poller 上其它设备的 PINGREQ 读不到 → keepalive 超时 → 被误判死连 → presence 删除雪崩。

这直接破坏红线 #6（presence 正确性）和 [网络攻防设计](../../network-defense/9.cloud-mqtt-gateway-network-attack-defense.md) 的慢速攻击防御。**自研 reactor 时务必抄这个默认值**。

---

## 9. mqtt-gateway 现状对照表（逐机制）

| # | nbio 机制 | nbio 源码 | mqtt-gateway 现状 | 差距 |
|---|---|---|---|---|
| ① | 去 goroutine-per-conn | `engine_unix.go:70` | `server.go:158 go s.handleConn` | 1M 连接 = 1M goroutine（撞 goroutine 红线） |
| ② | fd 索引数组连接表 | `engine_unix.go:28` | `server.go:83 map[net.Conn]struct{}` | 百万级 map GC + mutex |
| ③ | poller 共享读 buffer | `engine_unix.go:87` | `packet.go:63 make([]byte, remaining)` | 每包 malloc（热路径 GC churn） |
| ④ | epoll 批量取事件 | `poller_epoll.go:225` | 阻塞 Read，无批量 | reactor 独有，阻塞模型拿不到 |
| ⑤ | per-conn jobList 串行 | `conn.go:185` | 无（未来下行回调会撞 session） | 下行通道实现时必须解决 |
| ⑥ | writev + 尾包合并 | `conn_unix.go:402` | 每次 `conn.Write` 一次 syscall | 下行广播场景 syscall 放大 |
| ⑦ | mempool 回收写 buffer | `conn_unix.go:81` | 无 | 写路径 GC churn |
| ⑧ | 读次数封顶 | `engine.go:29` | 阻塞 Read 无此问题，但自研 reactor 必须有 | 自研时务必抄 |

**已做对的（不用借鉴）**：
- worker pool 解耦慢层（auth queue `auth/queue.go`、ingest queue `ingest/queue.go`）—— nbio 的 Execute/taskpool 同构，这一层 mqtt-gateway 已到位。
- bounded queue + fencing（红线 #4/#7/#8）—— 比 nbio 更严格，nbio 不关心业务 fencing。

---

## 10. 借鉴清单（按 ROI 排序）

| 借鉴点 | 改造范围 | 预期收益 | 风险 | 何时做 |
|---|---|---|---|---|
| **2. 读 buffer 池化（sync.Pool）** | `internal/mqtt/packet.go` 一处 | 砍掉热路径 GC churn（读路径最大头） | 极低（纯局部） | **立即**，不依赖换 reactor |
| **1. 换 nbio 作 transport** | `internal/server` + `internal/mqtt` codec | goroutine 1M→几十，内存翻倍，STW→<1ms | 中高（TLS 是坑，见 §11） | gnet 评估做完决策后 |
| **3. 连接表 map→数组** | `internal/server`（依赖①拿到裸 fd） | 消除百万级 map GC/锁 | 低（但要换 reactor 才有 fd） | 随 ① 一起 |
| **6. per-conn jobList 串行模式** | 下行通道实现时引入 | 无锁串行化读/写/回执回调 | 低（模式借鉴，可自实现） | 下行通道（设计 14）那圈 |
| **4. writev + 尾包合并** | 下行 socket 写路径 | 下行广播 syscall 降一个数量级 | 中（不能破 PUBACK 时序） | 下行通道那圈 |
| **5. 读次数封顶** | 自研 reactor 时 | 防慢连接饿死 poller | 低 | 仅自研 reactor 需要 |
| **7. mempool 回收写 buffer** | 写路径 | 写路径 GC churn | 低（注意 freeSize 上限） | 随 ④ 或下行那圈 |

详细展开最高 ROI 的 ①②两点：

---

## 11. 借鉴点 1：换 nbio 作为 transport（最高 ROI, but 最大坑是 TLS）

### 11.1 改造范围（对齐 gnet 评估 §5）

慢层（auth/ingest/presence worker pool）**一行不动**——它们本来就是 worker pool，和 nbio 的 Execute 完全兼容。要改的收敛在两个包：

| 层 | 现状 | 换 nbio 后 | 工作量 |
|---|---|---|---|
| 慢层（auth/ingest/presence） | bounded 队列 + 固定 worker | **不动** | 0 |
| QoS0 路径 | `TryAppendAsync` 已非阻塞 | 几乎不动 | ~0 |
| QoS1 PUBACK | read goroutine 阻塞 `<-result`（`server.go:380`） | 回执触发 `conn.AsyncWrite(PUBACK)`，fencing 已就绪（红线 #7） | 小 |
| **MQTT codec** | `ReadPacket(io.Reader)` + `io.ReadFull` 阻塞（`packet.go`） | 增量可重入：喂 buffer 字节，半包挂起续读 | **中（真功夫）** |
| **CONNECT 状态机** | 线性阻塞（read→auth→presence→connack） | 每连接小状态机 `AWAIT_CONNECT→AWAIT_AUTH→AWAIT_PRESENCE→CONNECTED` | **中（新代码）** |

### 11.2 TLS 是最大的坑（必须诚实标注）

mqtt-gateway 现状 `main.go:121` `tls.NewListener`，用标准库 `crypto/tls`。nbio 自带的 TLS 是 `nbhttp` 里的**自研 TLS 实现**（`extension/tls/tls.go`，已标注 `// deprecated`，底层是 `lesismal/llib/std/crypto/tls`），**和标准库 `crypto/tls` 不兼容**。

这正是 gnet 评估 [§2.1](../gnet/gnet-feasibility-assessment.md) 的翻转前提要解决的：**NLB 卸载 TLS 后，多数派连接到网关时已是明文**，网关内没有 `crypto/tls`。于是：

```text
普通设备（百万级，明文）  → nbio 监听器（:1883）          ← 拿全部收益，TLS 坑绕过
高安全设备（千~万级，mTLS）→ 经典 net+crypto/tls+goroutine-per-conn（:8883）  ← 少数派，留经典路径
```

mTLS 少数派 goroutine 数小到不触发 STW、不占多少内存，继续 goroutine-per-conn 完全没事。这就是 gnet 评估 [§6](../gnet/gnet-feasibility-assessment.md) 的"混合监听器"策略，nbio 完全适用。

**如果 mTLS 少数派也要上 nbio**：两条路（都不推荐，列出来供排除）：
- 用 nbio 自研 TLS 栈（`extension/tls`，已 deprecated，且不兼容 std，运维心智成本高）；
- accept 后先 std TLS 握手，再把底层 `net.Conn` 塞进 nbio（`engine.AddConn`，`engine.go:268`）——但握手阶段仍是阻塞 goroutine，收益打折。

结论：**明文路径上 nbio，mTLS 留经典路径**。

### 11.3 与红线 #3 的冲突（必须重审）

现状 `server.go:355` 注释：
> pp.Payload 别名 pkt.Body（ReadPacket 每包独占分配，非共享 read buffer，零拷贝安全，红线 #3）

换 nbio 后这个假设**失效**：poller 共享 buffer 是"借出"语义，`onData` 拿到的字节用完即还。所以 `handlePublish` 里 **必须把 payload 显式 copy 到独立 buffer** 再交给 ingest queue，否则 queue 里的 subslice 指向会被下一轮读覆盖的 poller buffer——直接违反红线 #3。这要算进 codec 改造的工作量。

---

## 12. 一个诚实的反建议：别盲目学 nbio 的 `go:norace`

nbio 源码里 `//go:norace` 满屏（`engine.go`、`conn.go` 大量函数）。这个 build tag 的含义是**"生产构建不带 race detector"**，不是"这里真的没有竞态"。它存在的理由是 nbio 追求极致性能、race detector 有开销。

mqtt-gateway 的 TDD 红线（[CLAUDE.md](../../CLAUDE.md)：`go test -race ./...` 并发相关改动必须跑）**必须保留**。借鉴 nbio 的设计时，把它的"无锁串行"模式学过来（jobList、fd 分片），但不要照搬 `go:norace`——那会让红线 #7/#8 的 fencing 正确性失去 race 检测保障。

---

## 13. 借鉴点 2：读 buffer 池化（不换 nbio 也该做）

**这是全文档 ROI 最高、风险最低的单点改造。** 即使最终决定不换 reactor，这一步也该做。

### 13.1 现状问题

`internal/mqtt/packet.go:63`：
```go
body := make([]byte, remaining)   // 每个 PUBLISH 包一次 malloc
if _, err := io.ReadFull(r, body); err != nil { ... }
```

注释（`packet.go:48`）已自承认：
> 注意：每次调用按 remaining length 精确分配 body。CONNECT 等低频报文可接受；高频 PUBLISH 热路径后续需改为"先预算后分配"（红线 §5.1），不要直接复用本函数。

### 13.2 改法（直接抄 nbio mempool）

`nbio/mempool/mempool.go` 是 `sync.Pool` + 按 size class 分桶 + 总空闲上限。简化版用于 packet buffer：

```go
// 伪代码，示意（实际实现走 TDD）
var packetPool = sync.Pool{
    New: func() any { b := make([]byte, 0, 1024); return &b },
}

func ReadPacket(r io.Reader, maxBodySize int) (Packet, error) {
    // ... 读 remaining length ...
    bufp := packetPool.Get().(*[]byte)
    buf := (*bufp)[:remaining]   // 需要按 size class 扩容
    // ... io.ReadFull(r, buf) ...
    // 注意：Body 生命周期结束后必须 packetPool.Put(bufp)
}
```

### 13.3 与红线 #3 的协同

红线 #3："payload 必须 bounded-memory，先预算后分配"。池化 buffer 正好是这个红线的实现手段之一——池有上限（`freeSize`），不会无界增长；取 buffer 前先校验 `remaining > maxBodySize`（已有，`packet.go:59`）保证 bounded。

**但要注意**：池化后 buffer 是复用的，**不能把它的 subslice 直接塞进 ingest queue**（红线 #3 后半句"禁止 queue 里引用大 read buffer 的 subslice"）。现状 mqtt-gateway 靠"每包独占 make"绕过这点——池化后绕不过了，`handlePublish` 里要么 copy payload，要么让 queue 持有 buffer 引用并在消费后 Put 回池。前者简单，推荐先做前者。

### 13.4 落地顺序（对齐 TDD 红线）

1. 先写失败测试：`BenchmarkReadPacket-8  allocs/op = 1`（现状），目标 `0 allocs/op`。
2. 实现 pool 化 ReadPacket。
3. 跑 `go test -race ./internal/mqtt`。
4. 压测验证 GC churn（`GODEBUG=gctrace=1`）下降。

---

## 14. 决策记录

| 项 | 决策 | 理由 |
|---|---|---|
| 是否立即做读 buffer 池化（借鉴点 2） | **是** | ROI 最高、风险最低、不依赖换 reactor；红线 #3 协同 |
| 是否换 nbio 作 transport（借鉴点 1） | **值得评估（非立即）** | 与 gnet 同档收益（内存翻倍 + STW→<1ms）；前提是 NLB 卸载 + 慢层解耦均已具备；评估优先级低于 Session 压缩 |
| mTLS 少数派是否上 nbio | **否** | TLS-over-nbio 坑大（自研 TLS 栈 deprecated 且不兼容 std）；留经典 goroutine-per-conn |
| 是否学 nbio 的 `go:norace` | **否** | 破坏 TDD race 检测红线，fencing 正确性失去保障 |
| 连接表是否改 fd 数组 | **随换 reactor 一起** | 依赖拿到裸 fd；明文路径可行，mTLS 留 map |
| 下行通道是否用 jobList 串行模式 | **是** | 无锁串行化读/写/回执回调，与红线 #2/#7 兼容 |
| 单节点是否冲百万 | **技术可达，运维不推荐** | 与 gnet 评估 [§7.1](../gnet/gnet-feasibility-assessment.md) 一致：故障爆炸半径指向多节点分摊（25-50万/节点） |
| 与 gnet 评估的关系 | **互补，非二选一** | gnet 评估看 ROI（要不要换），本文看技术细节（换的话长啥样） |

```text
核心结论：nbio 不是"万能药"，它是一整套 I/O 模型纪律的参考实现。mqtt-gateway 的业务纪律
（红线、bounded queue、fencing）已到位，缺的是 I/O 模型那一层。借鉴分两步：
  立即：读 buffer 池化（借鉴点 2，不换 reactor 也做）。
  后续：gnet 评估决策后，若换 reactor，nbio 的 poller 共享 buffer / fd 数组表 / jobList 串行 /
        writev 合并 / 读次数封顶都是现成参考；明文路径上，mTLS 留经典路径。
两步都对齐红线，不破坏 TDD。
```

---

## 15. 参考资料

- [nbio GitHub](https://github.com/lesismal/nbio) / [1M 连接基准](https://github.com/lesismal/nbio#1m-websocket-connections-benchmark)
- 配套设计：
  - [gnet 可行性评估](../gnet/gnet-feasibility-assessment.md)（互补：看 ROI，本文看技术细节）
  - [GC mark 计算模型](../go-gc-mark-cost-model.md)（§7.2 live 指针、§8.2 STW 栈扫描）
  - [容量规划](../../11.cloud-mqtt-gateway-capacity-planning.md)（§2.2 每连接内存、§10.3 爆炸半径）
  - [TLS 接入设计](../../network-defense/13.cloud-mqtt-gateway-tls-design.md)（§2 两种拓扑、§4 mTLS 约束）
  - [架构 §11.5 拓扑](../../1.cloud-mqtt-gateway-architecture.md)（NLB 卸载多数派）
  - [下行路由设计](../../14.cloud-mqtt-gateway-downlink-topic-routing-design.md)（jobList 串行模式的落地场景）
- 代码：
  - nbio（vendored）：[engine.go](../../../../../nbio/engine.go)、[engine_unix.go](../../../../../nbio/engine_unix.go)、[poller_epoll.go](../../../../../nbio/poller_epoll.go)、[conn.go](../../../../../nbio/conn.go)、[conn_unix.go](../../../../../nbio/conn_unix.go)、[mempool/mempool.go](../../../../../nbio/mempool/mempool.go)
  - mqtt-gateway：[internal/server/server.go](../../../../../mqtt-gateway/internal/server/server.go)（当前 goroutine-per-conn）、[internal/mqtt/packet.go](../../../../../mqtt-gateway/internal/mqtt/packet.go)（待池化的 ReadPacket）
- 红线：[实现红线](../../7.cloud-mqtt-gateway-implementation-red-lines.md)（#2 pointer-poor、#3 bounded-memory、#7 异步回执 fencing）
