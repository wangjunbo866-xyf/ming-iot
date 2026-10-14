# gnet v2 源码走查手册

> 用途：迁移 mqtt-gateway 接入层到 gnet 之前的源码研读笔记。
> **源码位置**：工作空间 `/home/wangjunbo/workspace-iot/gnet`
> **版本**：`github.com/panjf2000/gnet/v2`，git describe = `v2.4.1-97-gfd7ce4be`（基于 v2.4.1，含若干 commit）
> 平台：Linux（epoll），文末单独标注 *BSD/Darwin（kqueue）差异。
> 所有行号对应上述源码，可直接 `gnet/<file>.go:<line>` 跳转。
>
> 配套文档：本目录 [gnet-feasibility-assessment.md](./gnet-feasibility-assessment.md)（要不要用 gnet）。

---

## 0. 怎么读这份文档

- 每一站 = 一个源码文件，按「你调用 `gnet.Run` 之后发生的事」顺序排列。
- 每站包含：**职责** / **关键结构** / **关键函数（带行号）** / **一张图**。
- 最后一章「迁移点清单」列出 gnet 帮你做了什么、你要自己做什么。

### 源码分层全景

```
┌─────────────────────────────────────────────────────────────┐
│  你写的 EventHandler (OnTraffic/OnOpen/OnClose...)          │  ← 你的业务
├─────────────────────────────────────────────────────────────┤
│  gnet.go             Run()/Rotate()/Conn/EventHandler       │  ← 对外 API 层
│  options.go          Options — 配置                          │
│  load_balancer.go    新连接分给哪个 eventloop                │
├─────────────────────────────────────────────────────────────┤
│  engine_unix.go      engine — 运行中的 gnet 实例（总指挥）   │  ← Engine 层
│  acceptor_unix.go    accept 循环（主 reactor 干的活）        │
│  listener_unix.go    socket fd 创建 + sockopt               │
├─────────────────────────────────────────────────────────────┤
│  eventloop_unix.go   ⭐ read/write/close/open/register/ticker│  ← EventLoop 层
│  reactor_default.go  rotate/orbit/run 三个循环              │
│  reactor_ultimate.go poll_opt 优化版（同三个循环）           │
├─────────────────────────────────────────────────────────────┤
│  connection_unix.go  conn — Read/AsyncWrite/buffer           │  ← Connection 层
│  connection_linux.go processIO — Linux epoll 事件分发       │
│  conn_map.go         连接集合（默认构建用这个！map 实现）   │
│  conn_matrix.go      连接集合（gc_opt 构建才用，二维数组）   │
│  context.go          per-conn user context                  │
├─────────────────────────────────────────────────────────────┤
│  pkg/netpoll         epoll/kqueue 封装（Poller.Polling）     │  ← 基础设施层
│  pkg/buffer          ring/elastic buffer                    │
│  pkg/pool            对象池                                  │
│  pkg/queue           优先级任务队列（Trigger 投递）          │
└─────────────────────────────────────────────────────────────┘
```

### 两条核心认知（贯穿全文）

**认知 1：Reactor 本质 = `poller.Polling(callback)` 死循环**

```go
// reactor_default.go:52 orbit() 的内核
err := el.poller.Polling(func(fd int, ev netpoll.IOEvent, flags netpoll.IOFlags) error {
    // fd 是哪个？listener fd → accept；conn fd → read/write/close
})
```

`Polling`（`pkg/netpoll/poller_epoll_default.go:115`）内部是经典 epoll：`for { n := EpollWait(...); ... }`。
reactor 数量 = CPU 核数，**不随连接数增长**——这是 gnet 能扛百万连接的根本。

**认知 2：跨 reactor 通信 = `poller.Trigger`**

任何「我想让某 reactor 干一件事」的操作，都是把任务投递到那个 reactor 的私有队列：

```go
// 签名：pkg/queue/queue.go:21  +  netpoll/poller_epoll_default.go:90
el.poller.Trigger(queue.HighPriority, func(arg any) error { ... }, arg)
//                    ^queue.EventPriority   ^queue.Func = func(any) error
```

reactor 在 `Polling` 循环里，epoll 唤醒后会先消费这个队列（见第 8 站）。
**因此每个 reactor 的数据结构（conn_map 等）只有自己访问，无锁无竞争。**

---

## 第 1 站 入口 `gnet.go`

**职责**：对外 API。你只接触这一层。

### 1.1 你会用到的函数（`gnet.go`）

| 函数 | 行号 | 作用 |
|---|---|---|
| `Run(handler, protoAddr string, opts ...Option)` | 678 | 单地址启动，**阻塞**直到关停（OnBoot/启动后 return，真正阻塞在 `eng.concurrency.Wait()`，见 3.2）|
| `Rotate(handler, addrs []string, opts ...Option)` | 693 | 多地址启动 |
| `Stop(ctx, protoAddr)` | 722 | **已废弃**，用 `Engine.Stop` |
| `createListeners` | 547 | 解析 Options + 建 listener |
| `determineEventLoops` | 789 | 算 eventloop 数 |
| `parseProtoAddr` | 750 | 解析 `tcp://host:port` scheme |

### 1.2 `gnet.Run` 内部做了什么

```
gnet.Run(handler, "tcp://:1883", opts...)              // gnet.go:678
  │
  ├─① createListeners(addrs, opts)                      // gnet.go:547
  │     ├─ loadOptions(opts)                            // options.go:26
  │     ├─ 处理 Logger / EdgeTriggeredIO / ReadBufferCap/WriteBufferCap（572-596）
  │     ├─ 处理 ReusePort 平台差异（622-638：BSD 关掉）
  │     └─ for addr: initListener → listener{fd,...}    // listener_unix.go:84
  │
  └─② run(handler, listeners, options, addrs)           // engine_unix.go:230
        （进入第 3 站）
```

**关于 Run 的「阻塞」**：v2.4.1 的 `run`（engine_unix.go:230）在 OnBoot 和 start 之后**直接 return**，不阻塞。阻塞发生在 `eng.concurrency.Wait()`（engine_unix.go:219 在 `stop` 里）。所以 `gnet.Run` 的返回发生在 engine 真正关停之后。如果你想在主线程拿 engine 引用，用 `OnBoot(eng Engine)` 钩子保存。

### 1.3 三个对外接口（你写业务要实现的）

**`Action`（gnet.go:40）**：
```go
const (
    None Action = iota  // 继续（默认）
    Close               // 关闭这个连接
    Shutdown            // 关停整个 engine
)
```

**`EventHandler`（gnet.go:474）** — 你必须实现（或嵌入 `BuiltinEventEngine`）：

```go
type EventHandler interface {
    OnBoot(eng Engine) (action Action)              // 启动前，可拒绝启动
    OnShutdown(eng Engine)                          // 关停后
    OnOpen(c Conn) (out []byte, action Action)      // ⭐ 新连接建立
    OnClose(c Conn, err error) (action Action)      // ⭐ 连接关闭
    OnTraffic(c Conn) (action Action)               // ⭐⭐ 有数据可读（最核心）
    OnTick() (delay time.Duration, action Action)   // 定时器（保活/超时用）
}
```

**`BuiltinEventEngine`（gnet.go:507）** — 所有方法都返回 None 的空实现，你用组合嵌入它，只覆写关心的方法：
```go
type myHandler struct{ gnet.BuiltinEventEngine }
func (myHandler) OnTraffic(c gnet.Conn) gnet.Action { ... }
```

### 1.4 `Conn` 接口（gnet.go:424）— 你最常用的 API

`Conn` 是三个子接口的组合 + 几个直接方法：

| 子接口 | 定义行 | 并发安全？ | 关键方法 |
|---|---|---|---|
| `Reader`（:232） | `io.Reader` + `io.WriterTo` | ❌ 仅 reactor 线程 | `Next/Peek/Discard/InboundBuffered` |
| `Writer`（:260） | `io.Writer` + `io.ReaderFrom` | 部分 | `Writev/Flush/OutboundBuffered/AsyncWrite/AsyncWritev/SendTo` |
| `Socket`（:312） | — | ✅ | `Fd/Dup/SetReadBuffer/SetKeepAlive/SetLinger/...` |

Conn 直接方法（gnet.go:424-468）：
- `Context()/SetContext(any)` ❌ 仅 reactor 线程 — **存 per-conn 状态的地方**（替代你的 `Principal`）
- `EventLoop() EventLoop` ✅ — 返回归属 reactor（v2.4.1 实现见 connection_unix.go:547，返回 `c.loop`）
- `LocalAddr()/RemoteAddr()` ❌
- `Wake(callback AsyncCallback)` ✅ — 从别的 goroutine 触发一次 `OnTraffic`
- `CloseWithCallback(callback)` ✅ / `Close()` ✅
- `SetDeadline/SetReadDeadline/SetWriteDeadline` — **v2.4.1 返回 `ErrUnsupportedOp`，不支持！**（connection_unix.go:551-560）

> ⚠️ **并发安全红线**：注释里写「not concurrency-safe, must invoke within EventHandler」的方法，**只能在 reactor 回调里调**（OnOpen/OnTraffic/OnClose/OnTick）。从别的 goroutine 调会数据竞争。跨 goroutine 操作要用 `AsyncWrite` / `Wake` / `EventLoop().Execute`。
>
> ⚠️ **v2.4.1 不支持 deadline**：`SetReadDeadline` 直接返回错误。你的「读超时判死」逻辑必须改用 `OnTick`（见迁移点改造 3）。

### 1.5 v2.4.1 独有：`EventLoop` 接口（gnet.go:401）与 `Engine` 增强 API

v2.4.1 引入了 `EventLoop` 接口和一批新 API（v2.8.0 里部分被注释掉了）：

```go
type EventLoop interface {                          // gnet.go:401
    Register(ctx, addr) (<-chan RegisteredResult, error)  // 主动拨出
    Enroll(ctx, c net.Conn) (<-chan RegisteredResult, error) // 接管已有 conn
    Execute(ctx, runnable Runnable) error           // ⭐ 在该 reactor 上排队执行任务
    Schedule(ctx, runnable, delay) error            // 未实现（返回 ErrUnsupportedOp）
    Close(Conn) error
}
```

`Engine`（gnet.go:54）方法：`Validate`/`CountConnections`/`Register`/`Dup`/`DupListener`/`Stop`。
这些对 mqtt-gateway 的意义：**`EventLoop().Execute` 提供了「从别的 goroutine 安全地在某 conn 的 reactor 上跑代码」的官方通道**——比手搓 `Wake` 更干净，QoS1 回写 PUBACK 可以走这个。

---

## 第 2 站 配置 `options.go`

**职责**：所有 `WithXxx(...)` 选项的归档。**整个文件就一张 `Options` 结构体表**。

### Options 字段速查（options.go:44）

| 字段 | 行 | With 函数 | 默认 | 含义 |
|---|---|---|---|---|
| `LB` | 47 | `WithLoadBalancing` | RoundRobin | 连接分配算法 |
| `ReuseAddr`/`ReusePort` | 51/55 | `WithReuseAddr/Port` | false | socket 选项 |
| `MulticastInterfaceIndex` | 59 | `WithMulticastInterfaceIndex` | 0 | UDP 多播绑定接口 |
| `BindToDevice` | 65 | `WithBindToDevice` | "" | Linux 专有，绑网卡 |
| `Multicore` | 72 | `WithMulticore` | false | eventloop 数 = NumCPU |
| `NumEventLoop` | 76 | `WithNumEventLoop` | 0=自动 | 显式设 eventloop 数（覆盖 Multicore）|
| `ReadBufferCap` | 84 | `WithReadBufferCap` | **64KB** | ⚠️ 单次 read 上限（per-eventloop 共享 `el.buffer`）|
| `WriteBufferCap` | 92 | `WithWriteBufferCap` | **64KB** | outbound buffer 初始容量 |
| `LockOSThread` | 98 | `WithLockOSThread` | false | 绑定 OS 线程 |
| `Ticker` | 101 | `WithTicker` | false | 启用 OnTick |
| `TCPKeepAlive` | 107 | `WithTCPKeepAlive` | 0=关 | TCP 保活 idle（内核层，非应用层 PINGREQ）|
| `TCPKeepInterval` | 111 | `WithTCPKeepInterval` | 0 | TCP 保活探测间隔 |
| `TCPKeepCount` | 115 | `WithTCPKeepCount` | 0 | TCP 保活探测次数 |
| `TCPNoDelay` | 125 | `WithTCPNoDelay` | TCPNoDelay | Nagle 算法（类型是 `TCPSocketOpt`）|
| `SocketRecvBuffer` | 128 | `WithSocketRecvBuffer` | 0 | 内核 socket recv buffer |
| `SocketSendBuffer` | 131 | `WithSocketSendBuffer` | 0 | 内核 socket send buffer |
| `LogPath/Level/Logger` | 139/142/146 | — | zap 默认 | 日志 |
| `EdgeTriggeredIO` | 151 | `WithEdgeTriggeredIO` | false | 边沿触发 epoll |
| `EdgeTriggeredIOChunk` | 159 | `WithEdgeTriggeredIOChunk` | 1MB(若ET开) | ET 每轮读写上限 |

### 对 mqtt-gateway 的配置建议（先记着，实战再调）

```go
gnet.Run(handler, "tcp://:1883",
    gnet.WithMulticore(true),                       // 多核
    gnet.WithLoadBalancing(gnet.LeastConnections),  // 比 RR 更均衡
    gnet.WithReadBufferCap(8 * 1024),               // MQTT 包通常 < 8KB
    gnet.WithTicker(true),                          // ⭐ 用 OnTick 实现保活判死
    gnet.WithTCPKeepAlive(2*time.Minute),           // 内核层兜底
    gnet.WithTCPNoDelay(gnet.TCPNoDelay),
    gnet.WithLogger(slogAdapter),                   // 接 slog
)
```

> **TLS 注意**：v2.4.1 的 `Options` **没有 TLS 字段**。`listener_unix.go:57` 的 `socket.TCPSocket` 建的是裸 fd，**TLS 握手要你自己实现**。但根据 [gnet-feasibility-assessment.md](./gnet-feasibility-assessment.md) §2.1，NLB 卸载 TLS 后网关收明文（多数派），所以这个限制对明文路径无影响。

> **LT/ET 选型**：上面的配置建议**故意没开 `WithEdgeTriggeredIO`**，即用默认的 **LT（水平触发）**。MQTT 包小（通常 < 8KB，一次 `unix.Read` 就读完），ET 省 epoll 唤醒次数的收益用不上；而 ET「必须循环读到 EAGAIN 否则丢数据」的风险在控制指令通道（逆变器调度/限功率）不可接受。详细论证见 **§10.9 为什么选 LT**。

---

## 第 3 站 引擎 `engine_unix.go`

**职责**：一个运行中的 gnet 实例，负责启动/关停 reactor、持有 listener 和 eventloop 集合。

### 3.1 `engine` 结构（engine_unix.go:35）

```go
type engine struct {
    listeners    map[int]*listener   // fd → listener
    opts         *Options
    ingress      *eventloop          // ⭐ 主 reactor（默认模式才有，只 accept）
    eventLoops   loadBalancer        // ⭐ sub reactor 集合（处理已建连接）
    inShutdown   atomic.Bool
    turnOff      context.CancelFunc
    eventHandler EventHandler        // 你的 handler
    concurrency  struct {            // errgroup 管理 reactor goroutine
        *errgroup.Group
        ctx context.Context
    }
}
```

### 3.2 `run()` 启动总指挥（engine_unix.go:230）

```
run(handler, listeners, opts, addrs)                  // engine_unix.go:230
  │
  ├─① numEventLoop = determineEventLoops(opts)         // gnet.go:789
  │     (Multicore? NumCPU() : 1; NumEventLoop 覆盖; 上限 gfd.EventLoopIndexMax)
  │
  ├─② 建 engine，按 opts.LB 选 loadBalancer 实现       // 251-258
  │
  ├─③ handler.OnBoot(Engine)  ← 你的钩子①              // 261
  │     返回 Shutdown → 直接 return（拒绝启动）
  │
  ├─④ eng.start(ctx, numEventLoop)   ⭐ 关键分叉（见 3.3）// 267
  │
  ├─⑤ defer eng.stop(rootCtx, e)     ← 注册优雅关停     // 272
  │
  └─⑥ for addr: allEngines.Store(addr, &eng)           // 274
```

### 3.3 `start()` 的关键分叉（engine_unix.go:190）

```go
func (eng *engine) start(ctx, numEventLoop) error {
    if eng.opts.ReusePort {
        return eng.runEventLoops(ctx, numEventLoop)    // 路径 B
    }
    return eng.activateReactors(ctx, numEventLoop)     // 路径 A（默认）
}
```

| | 路径 A：`activateReactors`（默认） | 路径 B：`runEventLoops`（ReusePort） |
|---|---|---|
| 行号 | 137 | 82 |
| reactor 角色 | **1 个 main + N 个 sub** | **N 个全能** |
| 谁来 accept | 只 main reactor（`ingress`） | 每个 reactor 都 accept（内核负载均衡）|
| main reactor 函数 | `el.rotate`（reactor_default.go:27） | — |
| sub reactor 函数 | `el.orbit`（reactor_default.go:46） | `el.run`（reactor_default.go:79）|
| ticker 跑在哪 | `eng.ingress.ticker`（单独 goroutine，engine_unix.go:181）| `el0.ticker`（eventloop 0，engine_unix.go:127）|
| 何时用 | 单进程默认 | 多进程同端口 / 高并发 |

> **mqtt-gateway 选哪个**：默认路径 A 更简单，和你现在 `server.go` 单 listener + 多 worker 的模型最接近。ReusePort 留作后续多进程部署时再考虑。

### 3.4 `stop()` 优雅关停（engine_unix.go:198）

```
stop(ctx, s)                                           // engine_unix.go:198
  ├─ <-ctx.Done()                       ← 等 shutdown() 调用（turnOff）
  ├─ handler.OnShutdown(s)              ← 你的钩子②
  ├─ 给所有 eventloop + ingress 投递 ErrEngineShutdown（HighPriority）// 205-217
  ├─ concurrency.Wait()                 ← 等所有 reactor 退出     // 219
  ├─ closeEventLoops()                  ← 关 listener + poller   // 224
  └─ inShutdown.Store(true)                                      // 227
```

外部触发关停：`Engine.Stop(ctx)`（gnet.go:153）调 `eng.shutdown(nil)`（engine_unix.go:55）→ `turnOff()` cancel context。

---

## 第 4 站 accept `acceptor_unix.go`

**职责**：accept 新连接。两个函数对应两种模式。

### 4.1 `accept0`（主 reactor 用，默认模式）— acceptor_unix.go:30

以下是 acceptor_unix.go:30-75 的**完整代码**（逐行对应源码，不省略；注释里 `// 原注:` 开头的是源码自带的英文注释翻译）：

```go
func (el *eventloop) accept0(fd int, _ netpoll.IOEvent, _ netpoll.IOFlags) error {  // :30
    // el 是主 reactor(ingress)；fd 是 listener fd。它是 listener 注册到 epoll 时绑的回调
    // (engine_unix.go:170  poller.AddRead(ln.packPollAttachment(el.accept0), true)  ← true=ET 边沿触发)
    for {                                                                            // :31 ⭐ 批量 drain
        nfd, sa, err := socket.Accept(fd)                                            // :32 非阻塞 accept，返回新连接 fd + 对端 sockaddr
        switch err {                                                                 // :33
        case nil:                                                                    // :34 成功，继续往下
        case unix.EAGAIN: // 原注: the Accept queue has been drained out            // :35 ⭐ 队列空了
            return nil                                                               // :36 退出 for，回 poller.Polling 等下次唤醒
        case unix.EINTR, unix.ECONNRESET, unix.ECONNABORTED:                        // :37 可恢复错误，重试
            // 原注: ECONNRESET/ECONNABORTED 表示 Accept 队列里有个 socket 在被
            //       Accept 之前就关了，是个 silly error，重试即可
            continue                                                                 // :41 回 for 顶部
        default:                                                                     // :42 真错误
            el.getLogger().Errorf("Accept() failed due to error: %v", err)          // :43
            return errors.ErrAcceptSocket                                            // :44 返回非 nil → rotate() 会 engine.shutdown
        }

        remoteAddr := socket.SockaddrToTCPOrUnixAddr(sa)                            // :47 sockaddr → net.Addr，给 LB 和 conn 用
        network := el.listeners[fd].network                                         // :48 "tcp"/"unix"，从 listener 取
        if opts := el.engine.opts; opts.TCPKeepAlive > 0 && network == "tcp" &&     // :49 ⭐ 仅非 Linux/FreeBSD/DragonFly 平台
            (runtime.GOOS != "linux" && runtime.GOOS != "freebsd" && runtime.GOOS != "dragonfly") { // :50
            // 原注: 非 Linux/FreeBSD/DragonFly 平台上，accepted socket 不会继承
            //       listening socket 的 keepalive 选项，必须显式设置（参见 nginx PR #337）
            if err = setKeepAlive(                                                  // :56 设到 nfd（新连接 fd），不是 listener fd
                nfd, true,
                opts.TCPKeepAlive, opts.TCPKeepInterval, opts.TCPKeepCount); err != nil {
                el.getLogger().Errorf("failed to set TCP keepalive on fd=%d: %v", fd, err) // :62
            }
        }

        // ⚠️ 注意下面这行的「变量遮蔽」: 左侧 el 把外层的主 reactor el 遮蔽了！
        // 从这行起到函数末尾，el 不再是主 reactor，而是被选中的 sub reactor。
        el := el.engine.eventLoops.next(remoteAddr)                                 // :66 ⭐ LB 选 sub reactor
        c := newStreamConn(network, nfd, el, sa, el.listeners[fd].addr, remoteAddr) // :67 ⭐ 建 conn；注意传 el（已是 sub reactor）
        err = el.poller.Trigger(queue.HighPriority, el.register, c)                 // :68 ⭐ 投递「注册」任务到 sub reactor 的队列
        if err != nil {                                                             // :69 Trigger 失败兜底
            el.getLogger().Errorf("failed to enqueue the accepted socket fd=%d to poller: %v", c.fd, err) // :70
            _ = unix.Close(nfd)                                                     // :71 关掉这个新连接 fd
            c.release()                                                             // :72 释放 conn 对象（清 ctx/buffer 等）
        }
    }                                                                               // :74 回 for 顶部继续 accept，直到 EAGAIN
}
```

**四个要点**：
1. **批量 accept**（:31 的 `for`）：epoll 一次唤醒，`for` 循环把内核 accept 队列**一次 drain 干净**（直到 :35 的 EAGAIN）。这依赖于 listener 注册成 ET 边沿触发（engine_unix.go:170 传入 `true`），ET 只在状态变化时通知一次，必须一次 drain 完否则丢连接。
2. **不起 goroutine**（:68）：新连接通过 `Trigger(register)` 投递给选中的 sub reactor，由 sub reactor 在自己的 goroutine 里调 `register` → `register0` → `OnOpen`。
3. **变量遮蔽**（:66 `el := ...`）：源码这里用了**变量遮蔽**——左边的 `el` 覆盖了外层的主 reactor `el`，从这行起 `el` 指向 sub reactor。所以 :67 的 `newStreamConn(..., el, ...)` 和 :68 的 `el.poller.Trigger(...)` 用的都是 sub reactor，不是主 reactor。**看源码时务必注意这个遮蔽**，否则会误以为主 reactor 在处理连接。
4. **`newStreamConn`**（connection_unix.go:57）传入的是 sub reactor `el`（:66 遮蔽后的），所以 conn 后续都归 sub reactor 管。注意 v2.4.1 函数名是 `newStreamConn`（统一 tcp/unix），不是 `newTCPConn`。

### 4.2 `accept`（ReusePort 模式用）— acceptor_unix.go:77

以下是 acceptor_unix.go:77-116 的**完整代码**（逐行对应源码）：

```go
func (el *eventloop) accept(fd int, ev netpoll.IOEvent, flags netpoll.IOFlags) error { // :77
    // ⚠️ 与 accept0 不同：这里 el 就是会处理这个连接的 reactor（ReusePort 下每个 reactor 都 accept）
    network := el.listeners[fd].network                                             // :78
    if network == "udp" {                                                           // :79
        return el.readUDP(fd, ev, flags)                                            // :80 UDP 走另一条路
    }

    nfd, sa, err := socket.Accept(fd)                                               // :83 ⚠️ 只 accept 一个，不批量（无 for 循环）
    switch err {                                                                    // :84
    case nil:                                                                       // :85
    case unix.EINTR, unix.EAGAIN, unix.ECONNRESET, unix.ECONNABORTED:               // :86 这里 EAGAIN 也 return nil
        return nil                                                                  // :90
    default:                                                                        // :91
        el.getLogger().Errorf("Accept() failed due to error: %v", err)              // :92
        return errors.ErrAcceptSocket                                               // :93
    }

    remoteAddr := socket.SockaddrToTCPOrUnixAddr(sa)                                // :96
    if opts := el.engine.opts; opts.TCPKeepAlive > 0 && el.listeners[fd].network == "tcp" && // :97 同 accept0 的平台判断
        (runtime.GOOS != "linux" && runtime.GOOS != "freebsd" && runtime.GOOS != "dragonfly") { // :98
        if err = setKeepAlive(nfd, true, opts.TCPKeepAlive, opts.TCPKeepInterval, opts.TCPKeepCount); err != nil { // :104
            el.getLogger().Errorf("failed to set TCP keepalive on fd=%d: %v", fd, err) // :110
        }
    }

    c := newStreamConn(network, nfd, el, sa, el.listeners[fd].addr, remoteAddr)     // :114 ⚠️ 传 el 自己（同一个 reactor）
    return el.register0(c)                                                          // :115 ⚠️ 直接注册，不经过 Trigger
}
```

> 为什么 `accept`（ReusePort 模式）不批量？两个原因：
> 1. **不需要**：ReusePort 下每个 reactor 各自监听同一个端口，内核已经做了负载均衡把连接分散到各 reactor 的 accept 队列，没必要抢着 drain。
> 2. **不用 Trigger**（:115 直接 `register0`）：因为 ReusePort 模式下 accept 这个连接的 reactor，就是要处理它的 reactor，不存在跨 reactor 投递，直接注册即可。

### 4.3 ⚠️ gnet **没有** bounded accept！

你 `server.go:155-165` 用 `s.sem` 信号量做「在途连接上限，满了立即 close」，**gnet 源码里完全没有这个机制**。迁移时需要在 `OnOpen` 里用 `atomic` 计数自己实现（第 10 章详述）。

---

## 第 5 站 Reactor 心脏 `eventloop_unix.go` + `reactor_default.go`

**职责**：这是整个 gnet 最核心的部分。reactor 循环 + epoll 事件分发到你的 EventHandler。

### 5.1 `eventloop` 结构（eventloop_unix.go:42）

```go
type eventloop struct {
    listeners    map[int]*listener
    idx          int                // 在 lb 列表里的下标
    engine       *engine
    poller       *netpoll.Poller
    buffer       []byte             // ⚠️ eventloop 级共享读缓冲（所有 conn 共用！）
    connections  connMatrix         // ⚠️ 该 reactor 管的所有 conn（无锁，只本 reactor 访问）
    eventHandler EventHandler
}
```

> ⚠️ **红线 #3 相关**：`el.buffer` 是 eventloop 级**共享**缓冲区，所有连接的 `OnTraffic` 都从这块读。`OnTraffic` 返回后这块会被复用。**你不能持有 `c.Peek()/Next()` 返回的字节切片跨调用**，要立刻拷贝。

### 5.2 三个 reactor 循环（reactor_default.go）

**内核都只有一行**：`el.poller.Polling(callback)`。

```
┌─ rotate()  reactor_default.go:27 ── 主 reactor（默认模式）─────┐
│  poller.Polling(el.accept0)       // 33 行                    │
│  → 只调 accept0（第 4 站）                                       │
│  出错 → engine.shutdown(err)       // 41                       │
└─────────────────────────────────────────────────────────────────┘

┌─ orbit()   reactor_default.go:46 ── sub reactor（默认模式）────┐
│  poller.Polling(func(fd, ev, flags) {       // 52             │
│      c := el.connections.getConn(fd)      ← 从 fd 找 conn     │
│      if c == nil: return poller.Delete(fd)  ← stale fd        │
│      return c.processIO(fd, ev, flags)    ⭐ 进入第 5.4 节     │
│  })                                                            │
│  出错 → closeConns() + engine.shutdown(err)   // 73-74        │
└─────────────────────────────────────────────────────────────────┘

┌─ run()     reactor_default.go:79 ── ReusePort 模式（全能）─────┐
│  poller.Polling(func(fd, ev, flags) {       // 85             │
│      c := el.connections.getConn(fd)                           │
│      if c == nil:                                              │
│          if listeners[fd] != nil: return el.accept(...)        │
│          return poller.Delete(fd)                              │
│      return c.processIO(fd, ev, flags)                         │
│  })                                                            │
└─────────────────────────────────────────────────────────────────┘
```

**本质区别**：`run` 既 accept 又 processIO（ReusePort），`rotate` 只 accept，`orbit` 只 processIO。

> 注意 callback 签名是 `func(fd int, ev netpoll.IOEvent, flags netpoll.IOFlags) error`（即 `PollEventHandler`，defs_poller.go:20）。这是给 `Polling` 用的；而 `Trigger` 用的是另一种签名 `func(any) error`（即 `queue.Func`，queue.go:21）。**两种回调签名不同，别搞混。**

### 5.3 `reactor_ultimate.go`（poll_opt 优化版）

`//go:build ... && poll_opt` 下，三个循环签名一样但 `Polling()` **不带 callback 参数**：
```go
err := el.poller.Polling()   // ultimate 版，reactor_ultimate.go:32
```
因为 ultimate 版把 callback 绑在 `PollAttachment.Callback` 字段上（defs_poller.go:23），poller 内部直接调，省一次间接调用。**普通使用不需要开 poll_opt**，默认 default 版即可。

### 5.4 `processIO` — Linux epoll 事件分发（connection_linux.go:28）

这是 epoll 事件 → 你的 EventHandler 的**最后一公里**：

```go
func (c *conn) processIO(_ int, ev netpoll.IOEvent, _ netpoll.IOFlags) error {  // :28
    el := c.loop
    // ① 纯错误事件（ERR/HUP/RDHUP，无读写）→ 直接关
    if ev&(netpoll.ErrEvents|unix.EPOLLRDHUP) != 0 && ev&netpoll.ReadWriteEvents == 0 {
        c.outboundBuffer.Release() // v2.4.1 特有：坏连接直接释放待发数据
        return el.close(c, io.EOF)
    }
    // ② EPOLLOUT（可写）→ 先写。写优先级 > 读（反压设计）
    if ev&(netpoll.WriteEvents|netpoll.ErrEvents) != 0 {
        if err := el.write(c); err != nil { return err }
    }
    // ③ EPOLLIN（可读）→ 读，最终调你的 OnTraffic
    if ev&(netpoll.ReadEvents|netpoll.ErrEvents) != 0 {
        if err := el.read(c); err != nil { return err }
    }
    // ④ EPOLLRDHUP（对端关）→ 再读一次榨干，然后关
    if ev&unix.EPOLLRDHUP != 0 && c.opened {
        if ev&unix.EPOLLIN == 0 { return el.close(c, io.EOF) }
        c.isEOF = true
        return el.read(c)
    }
    return nil
}
```

**为什么「写优先于读」**（注释 36-45）：系统过载时先回写响应、减少 inbound 压力。这是天然的反压。

### 5.5 ⭐ `el.read()` — 你的 OnTraffic 在这里被调用（eventloop_unix.go:254）

```go
func (el *eventloop) read(c *conn) error {
    if !c.opened { return nil }
    var recv int
    isET := el.engine.opts.EdgeTriggeredIO
    chunk := el.engine.opts.EdgeTriggeredIOChunk
loop:
    n, err := unix.Read(c.fd, el.buffer)           // ⚠️ 非阻塞读，用共享 el.buffer
    if err != nil || n == 0 {
        if err == unix.EAGAIN { return nil }
        if n == 0 { err = io.EOF }
        return el.close(c, os.NewSyscallError("read", err))
    }
    recv += n
    c.buffer = el.buffer[:n]                        // 给 conn 一个切片视图
    action := el.eventHandler.OnTraffic(c)          // ⭐⭐⭐ 调你的回调（返回 Action，无 error）
    switch action {
    case None:
    case Close:     return el.close(c, nil)
    case Shutdown:  return errorx.ErrEngineShutdown
    }
    _, _ = c.inboundBuffer.Write(c.buffer)          // ⭐ 你没读走的 → 存进 inboundBuffer
    c.buffer = c.buffer[:0]
    if c.isEOF || (isET && recv < chunk) { goto loop }   // 继续读
    if isET && n == len(el.buffer) {                // ET 模式读满了，怕饿死别的 fd
        return el.poller.Trigger(queue.LowPriority, el.read0, c)   // 手动再触发一次
    }
    return nil
}
```

**两个对你 MQTT 接入层致命重要的点**：

**点 1：`el.buffer` 是 eventloop 级共享缓冲（红线 #3）**
- 所有连接的 OnTraffic 都从这块 `el.buffer` 读。
- `OnTraffic` 返回后这块会被下一个连接复用。
- **你不能持有 `c.Peek()/Next()/Read()` 返回的字节切片跨调用**，要立刻拷贝。
- 这正好呼应你红线 #3「禁止 queue 里引用大 read buffer 的 subslice」。

**点 2：分片到达 / 半个 MQTT 包（协议正确性）**
- 看 284-285 行：`OnTraffic` 返回后，**你没读走的数据会自动 append 到 `c.inboundBuffer`**。
- 下次 `OnTraffic` 被调时，新数据接在后面。
- 你现在 `mqtt.ReadPacket` 是阻塞 `io.ReadFull`（读够一整个包才返回），迁到 gnet 要改成「从 `c.Peek(n)`/`c.Read(buf)` 流式解析」。
- gnet 已经帮你存好半包了，你只需写「尝试解析，不够就 return None 等下次」的逻辑。

### 5.6 其他回调

| 函数 | 行号 | 作用 |
|---|---|---|
| `register0(c)` | 214 | 新 conn 注册：`addEvents` + `connections.addConn` + 调 `open(c)` |
| `register(a any)` | 204 | Trigger 的包装：解包 `*conn` 或 `*connWithCallback`，调 `register0` |
| `open(c)` | 231 | `OnOpen(c)`，把返回的 out 发出去；非 ET 且有积压则 `ModReadWrite` |
| `read0(a any)` | 250 | Trigger 的包装：`el.read(a.(*conn))` |
| `write(c)` | 309 | 刷 outboundBuffer（用 writev），发完 `ModRead` 取消监听写 |
| `write0(a any)` | 302 | Trigger 的包装 |
| `close(c, err)` | 362 | `OnClose(c, err)` + 榨干残余 outbound + `release()` + close fd |
| `wake(c)` | 405 | 从别的 goroutine 触发：调一次 `OnTraffic(c)` |
| `ticker(ctx)` | 415 | `OnTick()` 循环，按返回的 delay 周期触发 |
| `readUDP` | 450 | UDP 读：`Recvfrom` + `OnTraffic` + 立即 release |

### 5.7 完整事件分发总图

```
                         reactor goroutine (orbit/rotate/run)
                         poller.Polling(callback) 死循环
                                       │ epoll_wait 返回就绪 fd
                         ┌─────────────▼─────────────┐
                         │   这个 fd 是什么?          │
                         └──┬─────────────────────┬──┘
                            │ listener fd          │ conn fd
                ┌───────────▼──────────┐  ┌───────▼────────────┐
                │ accept0 / accept     │  │ c.processIO        │
                │ 批量 Accept          │  │ (connection_linux) │
                │ → lb.next(addr)      │  └────────┬───────────┘
                │ → Trigger(register)  │    按 ev 位分发
                └──────────────────────┘    ┌──────┼──────────┐
                                            │      │          │
                                         ERR/HUP  EPOLLOUT  EPOLLIN
                                            │      │          │
                                         close  el.write   el.read
                                                          ↓
                                              ⭐ OnTraffic(c)  ← 你写这里
                                                          ↓
                                        没读走的 → c.inboundBuffer
```

---

## 第 6 站 连接 `connection_unix.go` + `connection_linux.go`

**职责**：`Conn` 接口的实现 `*conn`。缓冲区管理 + 读写实现。

### 6.1 `conn` 结构（connection_unix.go:38）

```go
type conn struct {
    fd             int                // 文件描述符
    gfd            gfd.GFD            // gnet fd（编码了 eventloop idx）
    ctx            any                // ⭐ 你的 per-conn 状态（SetContext/Context）
    remote         unix.Sockaddr
    proto          string             // ⭐ v2.4.1: "tcp"/"udp"/"unix"（v2.8.0 没有）
    localAddr      net.Addr
    remoteAddr     net.Addr
    loop           *eventloop         // 归属的 reactor
    outboundBuffer elastic.Buffer     // ⭐ 待发送数据（AsyncWrite 进这里）
    pollAttachment netpoll.PollAttachment
    inboundBuffer  elastic.RingBuffer // ⭐ 没读走的半包数据
    buffer         []byte             // 最新一次 read 的数据视图（el.buffer 的切片）
    cache          []byte             // Peek 临时缓存
    isDatagram     bool               // UDP
    opened         bool
    isEOF          bool
}
```

**两个 buffer 是核心**：
- `inboundBuffer`（RingBuffer）：gnet 帮你存的、你没消费的**入站**半包数据。
- `outboundBuffer`（elastic.Buffer）：你 `AsyncWrite` 进来的、还没发出去的**出站**数据。

### 6.2 读 API（Reader 接口实现，connection_unix.go）

| 方法 | 行 | 行为 | 返回的 `[]byte` 安全吗 |
|---|---|---|---|
| `Read(p []byte)` | 311 | 拷贝到你的 p | ✅ 安全（拷贝到你的 buf）|
| `Next(n)` | 330 | 取 n 字节并推进 | ❌ 不安全（内部 buffer 切片，注释明确写「must not be used in a new goroutine」）|
| `Peek(n)` | 349 | 取 n 字节不推进 | ❌ 不安全（注释「valid until Discard is called」）|
| `Discard(n)` | 378 | 丢弃 n 字节（释放 Peek 的 cache） | — |
| `InboundBuffered()` | 453 | 可读字节数 = inboundBuffer + buffer | — |
| `WriteTo(w)` | 436 | 把 inbound 写到 w | — |

**MQTT 包解析推荐用法**（结合 5.5 的分片问题）：

```go
func (h *handler) OnTraffic(c gnet.Conn) gnet.Action {
    // 1. 先 Peek 固定头部（最少 2 字节：1 字节 type + 1 字节 remaining length 起始）
    hdr, err := c.Peek(2)
    if err == io.ErrShortBuffer {
        return gnet.None   // 半包，等下次 OnTraffic
    }
    // 2. 解析 remaining length（变长 1-4 字节），算出整包长度 totalLen
    totalLen := decodeRemainingLength(hdr)  // 伪代码
    if c.InboundBuffered() < totalLen {
        return gnet.None   // 半包，等下次
    }
    // 3. Peek 整包（仍是内部切片，不能跨 goroutine）
    full, _ := c.Peek(totalLen)
    // 4. 立刻拷贝到你自己的 buffer，或就地解析后立刻用完
    pkt := parseMQTT(full)   // 解析完就别再持有 full
    // 5. 推进缓冲区（消费掉这个包）
    c.Discard(totalLen)
    return gnet.None
}
```

### 6.3 写 API（Writer 接口实现）

| 方法 | 行 | 并发安全 | 场景 |
|---|---|---|---|
| `Write(p)` | 405 | ❌ 仅 reactor | reactor 内同步写（内部调 `c.write` :139）|
| `Writev(bs)` | 425 | ❌ 仅 reactor | reactor 内多 buffer 写 |
| `Flush()` | 449 | ❌ 仅 reactor | 刷 outboundBuffer（调 `el.write`）|
| `AsyncWrite(buf, cb)` | 498 | ✅ | **⭐ 从别的 goroutine 写（如下行 PUBACK 回写）** |
| `AsyncWritev(bs, cb)` | 513 | ✅ | 同上 |
| `SendTo(buf, addr)` | 412 | ❌ | UDP 专用 |

**`AsyncWrite` 实现（connection_unix.go:498）**：
```go
func (c *conn) AsyncWrite(buf []byte, callback AsyncCallback) error {
    if c.isDatagram { ... } // UDP 同步发
    return c.loop.poller.Trigger(queue.HighPriority, c.asyncWrite,
        &asyncWriteHook{callback, buf})              // ⭐ 投递回 conn 所属 reactor
}
```
本质还是 `Trigger`：把写任务投递回 conn 所属 reactor，由 reactor 在自己的线程调 `c.asyncWrite`（:252）→ `c.write`（:139）。
**这保证 outboundBuffer 只被一个 reactor 访问，无锁。**

### 6.4 `c.write()` 同步写实现（connection_unix.go:139）

```go
func (c *conn) write(data []byte) (n int, err error) {
    if !c.outboundBuffer.IsEmpty() {                // 已有积压 → 排队保序
        c.outboundBuffer.Write(data); return
    }
    sent, err := unix.Write(c.fd, data)             // 直接写
    if err == EAGAIN {                              // 内核 buffer 满
        c.outboundBuffer.Write(data)                // ⭐ 暂存
        c.loop.poller.ModReadWrite(...)             // ⭐ 注册 EPOLLOUT，等可写
        return
    }
    // 写不完的尾部也暂存，等 EPOLLOUT
}
```

**关键设计**：写不出去的数据进 `outboundBuffer` + 注册 `EPOLLOUT`，下次 epoll 唤醒由 `el.write`（5.6）继续刷。

---

## 第 7 站 负载均衡 `load_balancer.go`

**职责**：新连接分给哪个 sub reactor。

### 7.1 三种算法（load_balancer.go:27）

| 算法 | 行 | next() 逻辑 | 适用 |
|---|---|---|---|
| `RoundRobin` | 29 | `eventLoops[nextIndex % size]; nextIndex++`（:106）| 默认，最简单 |
| `LeastConnections` | 33 | 遍历找 `countConn()` 最小的（:114）| 连接数不均时更均衡 |
| `SourceAddrHash` | 36 | `crc32(addr.String()) % size`（:138）| 同 IP 粘到同 reactor |

### 7.2 `loadBalancer` 接口（load_balancer.go:41）

```go
type loadBalancer interface {
    register(*eventloop)                  // 启动时注册所有 eventloop（:75）
    next(net.Addr) *eventloop             // accept0 里调，选一个
    index(int) *eventloop                 // 按 idx 取（给 Execute/Register 用，:82）
    iterate(func(int, *eventloop) bool)   // 遍历（:90）
    len() int
}
```

### 7.3 `countConn()` 的并发安全

`leastConnectionsLoadBalancer.next`（:114）会调 `el.countConn()`（eventloop_unix.go:96）读 `el.connections.loadCount()`，这是跨 reactor 读，所以 `connMatrix` 内部用 `atomic.Int32` 存计数。

---

## 第 8 站 pkg/netpoll — epoll 封装

**职责**：把 Linux epoll / BSD kqueue 包成统一的 `Poller` API。

### 8.1 核心类型（defs_poller.go）

```go
// defs_poller.go:20
type PollEventHandler func(int, IOEvent, IOFlags) error

// defs_poller.go:23
type PollAttachment struct {
    FD       int
    Callback PollEventHandler
}
```

注意 Linux 上（defs_poller_epoll.go:22-25）：`IOFlags = uint16`，`IOEvent = uint32`。

### 8.2 `Poller` 结构（poller_epoll_default.go:35）

```go
type Poller struct {
    fd                          int    // epoll fd
    efd                         int    // eventfd（用于 Trigger 唤醒）
    efdBuf                      []byte // 读 eventfd 的 8 字节 buffer
    wakeupCall                  int32  // 防重复唤醒的标志
    asyncTaskQueue              queue.AsyncTaskQueue // 低优先级任务队列
    urgentAsyncTaskQueue        queue.AsyncTaskQueue // 高优先级任务队列
    highPriorityEventsThreshold int32
}
```

### 8.3 `Poller` 关键方法（poller_epoll_default.go）

| 方法 | 行 | 作用 |
|---|---|---|
| `OpenPoller()` | 46 | `EpollCreate1` + 建 eventfd + 注册 eventfd 到 epoll |
| `Close()` | 72 | close efd 和 epoll fd |
| `Trigger(priority, fn, param)` | 90 | ⭐ 投递任务到队列 + 写 eventfd 唤醒 |
| `Polling(callback)` | 115 | ⭐ epoll_wait 死循环 |
| `AddRead(pa, edge)` | 199 | EPOLL_CTL_ADD，监听 EPOLLIN |
| `AddReadWrite(pa, edge)` | 189 | EPOLL_CTL_ADD，监听 EPOLLIN\|EPOLLOUT |
| `ModRead(pa, edge)` | 219 | EPOLL_CTL_MOD，改成只读 |
| `ModReadWrite(pa, edge)` | 229 | EPOLL_CTL_MOD，改成读写都监听 |
| `Delete(fd)` | 239 | EPOLL_CTL_DEL |

### 8.4 ⭐ `Polling` 内部（poller_epoll_default.go:115）

```go
func (p *Poller) Polling(callback PollEventHandler) error {
    el := newEventList(InitPollEventsCap)         // 128，defs_poller_epoll.go:29
    msec := -1
    for {
        n, err := unix.EpollWait(p.fd, el.events, msec)
        // ...
        for i := 0; i < n; i++ {
            ev := &el.events[i]
            if fd := int(ev.Fd); fd == p.efd {    // ⭐ eventfd 唤醒 → 有任务要处理
                doChores = true
            } else {
                err = callback(fd, ev.Events, 0)  // ⭐ 调 accept0 / processIO
            }
        }
        if doChores {                              // ⭐ 消费任务队列
            // 先 drain urgentAsyncTaskQueue（全部）
            // 再 drain asyncTaskQueue（最多 MaxAsyncTasksAtOneTime=256 个）
            // atomic.StoreInt32(&p.wakeupCall, 0)
            // 还有剩余 → 再次写 eventfd 唤醒下一轮
        }
        // 动态扩缩 events 数组（180-184）
    }
}
```

**Trigger 怎么唤醒阻塞中的 `EpollWait`**：`Trigger`（:90）把任务入队后，用 CAS 把 `wakeupCall` 从 0 置 1，成功就往 eventfd 写 8 字节。eventfd 被注册在 epoll 里监听读，所以 `EpollWait` 立即返回，`Polling` 看到 `fd == p.efd` 就置 `doChores=true`，下一轮消费任务队列。

**优先级机制**（queue.go:53）：
- `HighPriority` 任务进 `urgentAsyncTaskQueue`，每轮**全部 drain**。
- `LowPriority` 任务进 `asyncTaskQueue`，每轮**最多 256 个**（`MaxAsyncTasksAtOneTime`，defs_poller_epoll.go:35，防饿死 epoll 事件）。
- 当 urgent 队列超过阈值（`highPriorityEventsThreshold`），新来的 low 任务会被分流到 async 队列。

> 这对你 mqtt-gateway 含义：`AsyncWrite`（写 PUBACK）用 HighPriority 会尽快发；但如果你用 `EventLoop().Execute` 投递低优先级任务（如 metrics 采集），它在系统繁忙时会被延迟。

### 8.5 文件清单（pkg/netpoll/）

| 文件 | 作用 |
|---|---|
| `defs_poller.go` | PollAttachment / PollEventHandler |
| `defs_poller_epoll.go` | Linux 事件常量（EPOLLIN 等）、eventList |
| `defs_poller_kqueue.go` / `_bsd.go` | BSD 事件常量 |
| `poller_epoll_default.go` | Linux default 版 Poller（带 callback）|
| `poller_epoll_ultimate.go` | Linux ultimate 版（callback 绑 attachment）|
| `poller_kqueue_*.go` | BSD 版 + wakeup 机制（BSD 没有 eventfd）|
| `poller_unix_ultimate.go` | ultimate 版共用部分 |
| `syscall_epoll_*.go` | epoll 系统调用封装（区分 arch）|

---

## 第 8.5 站 pkg/socket — socket fd 创建 + sockopt 封装

**职责**：gnet 和内核 socket API 的**唯一接触面**。所有「创建 fd、设 socket 选项、accept、bind、listen」的系统调用都封装在这里。前面讲的 `listener_unix.go:open`（调 `TCPSocket`）、`acceptor_unix.go:32`（调 `Accept`）、`initListener`（组装 sockopt）都是这个包的上层用户。

### 8.5.1 文件清单与角色

| 文件 | 行数 | 职责 |
|---|---|---|
| `socket.go` | 64 | `TCPSocket`/`Accept`/`Option[T]` 泛型 + `execSockOpts` |
| `tcp_socket.go` | 145 | TCP 地址解析（`GetTCPSockAddr`）+ `tcpSocket`（建 fd 的平台入口）+ backlog |
| `sockopts_posix.go` | 90 | POSIX sockopt（`SetNoDelay`/`SetRecvBuffer`/`SetSendBuffer` 等）|
| `sockopts_unix.go` | 90 | Unix sockopt（`SetKeepAlive` 含 KEEPIDLE/INTVL/CNT、`SetLinger` 等）|
| `sock_cloexec.go` | 27 | Linux：`sysSocket`/`sysAccept` 一步带 `SOCK_NONBLOCK\|SOCK_CLOEXEC` |
| `sys_cloexec.go` | 45 | BSD/macOS：`sysSocket`/`sysAccept` 三步带 ForkLock 兜底 |
| `sockaddr.go` | 115 | `ipToSockaddr`/`SockaddrToTCPOrUnixAddr`（accept0 里用的地址转换）|

### 8.5.2 链路 1：建 listener fd（TCPSocket 全流程）

`listener_unix.go:53` 调 `socket.TCPSocket` → `tcpSocket`（tcp_socket.go:85）。完整流程：

```go
// tcp_socket.go:85  tcpSocket（简化）
func tcpSocket(network, addr string, passive bool, sockOptInts, sockOptStrs) (fd int, ...) {
    sa, family, tcpAddr, _, err = GetTCPSockAddr(network, addr)   // :96 ① 地址解析 + family 判断
    fd, err = sysSocket(family, unix.SOCK_STREAM, unix.IPPROTO_TCP) // :97 ② 创建 fd
    if err = execSockOpts(fd, sockOptInts); err != nil { return }   // :116 ③ 执行 sockopt 列表
    if err = execSockOpts(fd, sockOptStrs); err != nil { return }   // :117
    if passive {                                                    // :123 ④ bind + listen
        err = unix.Bind(fd, sa)
        err = unix.Listen(fd, listenerBacklogMaxSize)              // backlog=内核最大值
    } else {
        err = unix.Connect(fd, sa)                                  // client 模式（mqtt-gateway 不用）
    }
    return
}
```

#### ① 地址解析 + family 判断（GetTCPSockAddr，tcp_socket.go:33-61）

```go
tcpAddr, err = net.ResolveTCPAddr(proto, addr)    // "127.0.0.1:1883" → *net.TCPAddr
tcpVersion, err = determineTCPProto(proto, tcpAddr)
switch tcpVersion {
case "tcp4": family = unix.AF_INET; ...           // 仅 IPv4
case "tcp6": ipv6only = true; fallthrough
case "tcp":  family = unix.AF_INET6; ...          // 默认双栈（收 v4 也能收 v6）
}
```

> **对 mqtt-gateway 的含义**：配 `tcp://:1883` → gnet 建 **IPv6 双栈** socket，逆变器无论 v4/v6 都能连。若只想收 v4，配 `tcp4://:1883`。

#### ② 创建 socket fd（sysSocket，Linux 版 sock_cloexec.go:21-23）

```go
func sysSocket(family, sotype, proto int) (int, error) {
    return unix.Socket(family, sotype|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, proto)
    //                          ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
    //                          一次系统调用同时设 非阻塞 + CLOEXEC（原子）
}
```

**两个 flag**：
- `SOCK_NONBLOCK`：fd 非阻塞（reactor 必需，EAGAIN 语义依赖它）
- `SOCK_CLOEXEC`：exec 时自动关闭（防 fork+exec 时 fd 泄漏给子进程）

**Linux vs BSD**：Linux 用 `socket(type|FLAGS)` 一步搞定（sock_cloexec.go）。BSD/macOS 不支持 `SOCK_NONBLOCK|SOCK_CLOEXEC`，只能 `socket()` → `CloseOnExec` → `SetNonblock` 三步（sys_cloexec.go:25-38），中间有竞态窗口需 ForkLock 保护。这是 gnet 在 Linux 性能更好的底层原因之一。

#### ③ 执行 sockopt 列表（execSockOpts，socket.go:33-40）

```go
func execSockOpts[T int | string](fd int, opts []Option[T]) error {
    for _, opt := range opts {
        if err := opt.SetSockOpt(fd, opt.Opt); err != nil { return err }
    }
    return nil
}
```

遍历 `Option[T]` 列表（每个是 `{SetSockOpt: 函数, Opt: 值}`），逐个调 `SetSockOpt(fd, value)`。这里的 opts 是 `initListener`（listener_unix.go:84-142）组装的——**你配的每个 WithXxx 最终在这里变成 setsockopt 系统调用**。

#### ④ bind + listen（tcp_socket.go:123-131）

```go
err = unix.Bind(fd, sa)                                    // 绑定地址
err = unix.Listen(fd, listenerBacklogMaxSize)              // backlog = maxListenerBacklog()
```

`listenerBacklogMaxSize`（tcp_socket.go:30）取内核 `somaxconn` 的最大值（Linux 通常 4096+）。**backlog = 内核 accept 队列大小**，决定「来不及 accept 时能缓存多少连接」。配合 accept0 的 ET 批量 drain，抗连接风暴。

### 8.5.3 链路 2：accept 新连接（Accept）

`acceptor_unix.go:32` 调 `socket.Accept(fd)` → `sysAccept`（Linux 版 sock_cloexec.go:25-27）：

```go
func sysAccept(fd int) (nfd int, sa unix.Sockaddr, err error) {
    return unix.Accept4(fd, unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC)
    //                         ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
    //                         accept4 一次系统调用原子设 非阻塞 + CLOEXEC
}
```

**对比 Go 标准库 `net.Accept`**：标准库 `accept()` → `SetNonblock` → `CloseOnExec` 三步（带 ForkLock）；gnet（Linux）`accept4(FLAGS)` 一步原子。原子性消除「fd 创建到设 flag」之间的 fork+exec 泄漏竞态。

### 8.5.4 sockopt 落地：你的 WithXxx 怎么变成 setsockopt

`initListener`（listener_unix.go:84-142）把你的 Options 组装成 sockopt 列表：

```go
// listener_unix.go:84（简化）
func initListener(network, addr string, options *Options) (ln *listener, err error) {
    var sockOptInts []socket.Option[int]

    if options.ReusePort && network != "unix" {
        sockOptInts = append(sockOptInts, socket.Option[int]{SetSockOpt: socket.SetReuseport, Opt: 1})
    }
    if options.TCPNoDelay == TCPNoDelay && strings.HasPrefix(network, "tcp") {
        sockOptInts = append(sockOptInts, socket.Option[int]{SetSockOpt: socket.SetNoDelay, Opt: 1})
    }
    if options.SocketRecvBuffer > 0 {
        sockOptInts = append(sockOptInts, socket.Option[int]{SetSockOpt: socket.SetRecvBuffer, Opt: options.SocketRecvBuffer})
    }
    // ... SendBuffer 等
    // TCPKeepAlive 在 initListener 末尾单独调 setKeepAlive(ln.fd, ...)
    ...
}
```

下面是你常用 sockopt 的源码封装：

#### TCP_NODELAY（sockopts_posix.go:34-36）

```go
func SetNoDelay(fd, noDelay int) error {
    return unix.SetsockoptInt(fd, unix.IPPROTO_TCP, unix.TCP_NODELAY, noDelay)
}
```

关闭 Nagle 算法，数据立即发（不攒小包）。**mqtt-gateway 的 PUBACK（4 字节）走 Nagle 会被攒着延迟，必须 NoDelay**。gnet 默认开（options.go:125，类型 `TCPSocketOpt`，默认值 `TCPNoDelay`）。

#### SO_RCVBUF / SO_SNDBUF（sockopts_posix.go:40-48）

```go
func SetRecvBuffer(fd, size int) error {
    return unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, size)
}
func SetSendBuffer(fd, size int) error {
    return unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_SNDBUF, size)
}
```

内核 socket buffer 大小。⚠️ **实际值是设置值×2**（内核翻倍留开销）。设 `WithSocketRecvBuffer(64*1024)` 实际内核给 128KB。

#### SO_KEEPALIVE + TCP_KEEPIDLE/INTVL/CNT（sockopts_unix.go:43-71）⭐

```go
func SetKeepAlive(fd int, enabled bool, idle, intvl, cnt int) error {
    unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_KEEPALIVE, boolToInt(enabled))   // 开启
    if !enabled { return nil }
    unix.SetsockoptInt(fd, unix.IPPROTO_TCP, unix.TCP_KEEPIDLE, idle)   // 空闲多久开始探
    unix.SetsockoptInt(fd, unix.IPPROTO_TCP, unix.TCP_KEEPINTVL, intvl) // 探测间隔
    unix.SetsockoptInt(fd, unix.IPPROTO_TCP, unix.TCP_KEEPCNT, cnt)     // 探测次数
}
```

**四个参数怎么来的**（engine_unix.go:281-289 `setKeepAlive`）：

```go
func setKeepAlive(fd int, enabled bool, idle, intvl time.Duration, cnt int) error {
    if intvl == 0 { intvl = idle / 5 }   // 默认间隔 = idle/5
    if cnt == 0 { cnt = 5 }              // 默认探测 5 次
    return socket.SetKeepAlive(fd, enabled, int(idle.Seconds()), int(intvl.Seconds()), cnt)
}
```

**推导 mqtt-gateway 配置**（`WithTCPKeepAlive(2*time.Minute)`，不配 Interval/Count）：
- `idle = 120s`（2 分钟无活动开始探）
- `intvl = 120/5 = 24s`（每 24 秒探一次）
- `cnt = 5`（探 5 次失败判定死）
- **总判死时间 = idle + intvl × cnt = 120 + 24×5 = 240s = 4 分钟**

### 8.5.5 完整的 listener 建立时序

```
gnet.Run(handler, "tcp://:1883", WithTCPKeepAlive(2m), WithTCPNoDelay(on), ...)
  │
  ├─ createListeners (gnet.go:547)
  │    └─ initListener("tcp", ":1883", options) (listener_unix.go:84)
  │         ├─ 组装 sockOptInts: [SetNoDelay(1)]  （ReusePort 没开，不加）
  │         ├─ TCPKeepAlive > 0 且 Linux → 单独调 setKeepAlive(ln.fd,...)
  │         └─ ln.open() (listener_unix.go:53)
  │              └─ socket.TCPSocket("tcp", ":1883", passive=true, sockOptInts, [])
  │                   ├─ GetTCPSockAddr: ResolveTCPAddr → AF_INET6 双栈 sa
  │                   ├─ sysSocket(AF_INET6, SOCK_STREAM|NONBLOCK|CLOEXEC, TCP) → fd
  │                   ├─ execSockOpts(fd, [SetNoDelay(1)]):
  │                   │    └─ setsockopt(fd, IPPROTO_TCP, TCP_NODELAY, 1)
  │                   ├─ unix.Bind(fd, sa):  绑定 [::]:1883
  │                   └─ unix.Listen(fd, backlog=4096): 开始监听
  │
  ├─ initListener 末尾（listener_unix.go:127-139，Linux 分支）:
  │    └─ setKeepAlive(ln.fd, true, 120s, 0, 0):
  │         └─ setsockopt SO_KEEPALIVE=1, TCP_KEEPIDLE=120, TCP_KEEPINTVL=24, TCP_KEEPCNT=5
  │
  └─ listener fd 就绪 → engine_unix.go 主 reactor poller.AddRead(ln, ET)
```

**关键时序**：sockopt 在 bind/listen **之前**设置（tcp_socket.go:116-128），这是内核要求（SO_REUSEADDR/SO_REUSEPORT 必须在 bind 前）。

### 8.5.6 Linux 特殊待遇：keepalive 设在 listener，accept 的 conn 自动继承

`initListener`（listener_unix.go:127-139）和 `accept0`（acceptor_unix.go:49-64）的配合：

```go
// listener_unix.go:127（initListener 末尾，Linux 分支）
if options.TCPKeepAlive > 0 && ln.network == "tcp" && (runtime.GOOS == "linux" || ...) {
    err = setKeepAlive(ln.fd, true, options.TCPKeepAlive, ...)   // 设在 listener fd
}
```

```go
// acceptor_unix.go:49（accept0，accept 出来后）
if opts := el.engine.opts; opts.TCPKeepAlive > 0 && network == "tcp" &&
    (runtime.GOOS != "linux" && ...) {   // ⚠️ 注意：非 Linux 才进这个分支
    setKeepAlive(nfd, true, ...)         // 设在新 conn fd
}
```

**Linux**：accept 出来的 conn **自动继承** listener 的 `SO_KEEPALIVE` 等选项（内核行为）。所以 keepalive 只需设 listener 一次，accept 的 conn 白嫖。**macOS/BSD 不继承**，accept0 要在每个新 conn 上重新设。

### 8.5.7 对 mqtt-gateway 的四个启示

#### 启示 1：sockopt 顺序——gnet 已保证 bind 前设

`tcpSocket` 顺序是 `socket → setsockopt → bind → listen`（tcp_socket.go:96-128）。有些 sockopt（SO_REUSEADDR/SO_REUSEPORT）**必须在 bind 之前**才生效，gnet 顺序正确，不用操心。

#### 启示 2：backlog 设成内核最大值——建议同时调大 somaxconn

`unix.Listen(fd, listenerBacklogMaxSize)` 把 accept 队列设到内核允许的最大值。配合 accept0 的 ET 批量 drain，设备批量上线时能缓存数千个未 accept 连接。

**⚠️ 内核 `somaxconn` 限制**：老内核默认 `net.core.somaxconn=128`，实际 backlog 只有 128，连接风暴会 SYN 队列溢出。建议部署时调大：

```bash
# /etc/sysctl.conf
net.core.somaxconn = 65535
sysctl -p
```

#### 启示 3：KeepAlive 判死时间 = idle + intvl × cnt（4 分钟内核兜底）

`WithTCPKeepAlive(2*time.Minute)` 推导的判死时间是 4 分钟（120+24×5）。这是**内核层判死**——内核探测 5 次失败后直接 RST 连接 → epoll EPOLLERR/EPOLLHUP → processIO → el.close → OnClose。

**和应用层 OnTick 保活判死（§10.7）的关系**：

| 防线 | 层级 | 检查什么 | 判死时间 |
|---|---|---|---|
| OnTick 保活（10.7）| 应用层（MQTT）| PINGREQ/PUBLISH 活跃度 | 1.5 × MQTT keepalive（通常 90s~900s）|
| TCP KeepAlive（本节）| 内核层（TCP）| TCP 连通性 | idle + intvl × cnt（配 2m 时 4 分钟）|

**两道独立防线**：应用层先判（MQTT keepalive 通常更短），内核兜底（设备 TCP 栈假死时——不发 FIN 也不响应 PINGREQ，应用层可能漏判，内核 KeepAlive 能发现并清理）。

#### 启示 4：⚠️ TCP_NODELAY 默认开——别关

gnet 默认 `TCPNoDelay`（关闭 Nagle）。别配 `WithTCPNoDelay(gnet.TCPDelay)` 关掉它。Nagle 会攒 PUBACK（4 字节）等设备下一个包，造成 PUBACK 延迟。QoS1 场景要求 PUBACK 立即发，必须 NoDelay。

### 8.5.8 文件清单（pkg/socket/）

| 文件 | 职责 |
|---|---|
| `socket.go` | `TCPSocket`/`Accept`/`Option[T]`/`execSockOpts` |
| `tcp_socket.go` | TCP 地址解析、`tcpSocket`、backlog、`UDPSocket` |
| `sockopts_posix.go` | POSIX sockopt：NoDelay/RecvBuffer/SendBuffer/ReuseAddr |
| `sockopts_unix.go` | Unix sockopt：KeepAlive（含 IDLE/INTVL/CNT）、Linger、Multicast |
| `sock_cloexec.go` | Linux：`sysSocket`/`sysAccept` 一步带 SOCK_NONBLOCK\|SOCK_CLOEXEC |
| `sys_cloexec.go` | BSD/macOS：三步带 ForkLock 兜底 |
| `sockaddr.go` | `ipToSockaddr`/`SockaddrToTCPOrUnixAddr`（地址转换）|

---

## 第 8.6 站 pkg/logging — 日志抽象层 + 怎么接 slog

**职责**：gnet 的日志抽象层。定义极简的 `Logger` 接口（5 个 printf 方法），默认用 zap 实现，允许用 `WithLogger` 替换成任何实现该接口的 logger——**包括 slog**。

**关键洞察**：gnet 的 `logging.Logger` 接口和标准库 `log/slog.Logger` 是**两个不同的东西**。配置建议里的 `WithLogger(slogAdapter)` 需要一个「适配器」把 slog 包装成 gnet 的 Logger 接口。

### 8.6.1 Logger 接口（你实现 slog 适配器的契约）

`logging.Logger`（pkg/logging/logger.go:282-293）：

```go
type Logger interface {
    Debugf(format string, args ...any)
    Infof(format string, args ...any)
    Warnf(format string, args ...any)
    Errorf(format string, args ...any)
    Fatalf(format string, args ...any)
}
```

**5 个方法，每个都是 `Xxxf(format, args...)`（printf 风格）**。和 slog 的差异：

| 特征 | gnet Logger | slog.Logger |
|---|---|---|
| 方法风格 | `printf`（format + 占位符）| `key-value`（msg + attrs）|
| 方法签名 | `Infof(format string, args ...any)` | `Info(msg string, args ...any)` |
| 参数语义 | format 字符串替换 | 结构化 attrs |
| Fatal | gnet 的 Fatalf 内部 `os.Exit` | slog 无 Fatal，需自己包 |

**这就是为什么需要适配器**：gnet 调 `logger.Errorf("Accept() failed: %v", err)`，但 slog 的方法是 `Error(msg, args...)`，签名和语义都对不上。

### 8.6.2 默认 zap 实现（不替换时的行为）

`init()`（logger.go:97-124）在包加载时自动初始化默认 logger：

```go
func init() {
    // 环境变量 GNET_LOGGING_LEVEL 控制级别
    lvl := os.Getenv("GNET_LOGGING_LEVEL")
    ...
    // 环境变量 GNET_LOGGING_FILE 控制输出目标
    fileName := os.Getenv("GNET_LOGGING_FILE")
    if len(fileName) > 0 {
        defaultLogger, defaultFlusher, err = CreateLoggerAsLocalFile(fileName, ...)  // 写文件 + lumberjack 滚动
    } else {
        // 默认：写 stdout，dev 模式，带 [gnet] 前缀
        core := zapcore.NewCore(getDevEncoder(), zapcore.Lock(os.Stdout), defaultLoggingLevel)
        defaultLogger = zap.New(core, zap.Development(), ...).Sugar()
    }
}
```

**默认行为**（不配 WithLogger、不设环境变量）：gnet 用 zap 的 SugarLogger 写 stdout，带 `[gnet]` 前缀，级别 Info：

```
[gnet] 2026-06-26T10:30:00.123Z INFO  accept0 Accept() success
[gnet] 2026-06-26T10:30:00.124Z ERROR accept0 Accept() failed: connection reset
```

**环境变量调优**（不改代码）：
- `GNET_LOGGING_LEVEL=-1`（Debug）/`0`（Info）/`1`（Warn）...
- `GNET_LOGGING_FILE=/var/log/gnet.log`（写文件，lumberjack 滚动：100MB/文件、2 备份、15 天）

**CreateLoggerAsLocalFile**（logger.go:201-226）：用 lumberjack 做文件滚动（`MaxSize=100MB, MaxBackups=2, MaxAge=15d`），prod encoder（RFC3339 时间 + CapitalLevel），返回 logger + flush 函数。

### 8.6.3 gnet 内部怎么用 logger（getLogger）

#### options.Logger 字段（options.go:146）

```go
type Options struct {
    ...
    Logger logging.Logger    // 默认是 logging.GetDefaultLogger()（zap）
}
```

`createListeners`（gnet.go:572-576）初始化时：

```go
if options.Logger == nil {
    options.Logger = logging.GetDefaultLogger()   // 没配就用默认 zap
}
```

#### eventloop.getLogger（eventloop_unix.go:92-94）

```go
func (el *eventloop) getLogger() logging.Logger {
    return el.engine.opts.Logger
}
```

gnet 内部所有日志调用都走这个。比如 `accept0`（acceptor_unix.go:43）：

```go
el.getLogger().Errorf("Accept() failed due to error: %v", err)
```

`el.getLogger()` 返回 `opts.Logger`——**你用 WithLogger 传入的 logger**。gnet 内部的日志（accept 错误、shutdown、Trigger 失败等）全部走这一个 logger。

#### ⚠️ 两套 logger 的分裂

除了 `el.getLogger()`，logging 包还有一组**包级别函数**（`logging.Errorf`/`logging.Warnf` 等，logger.go:238-279），它们用 `defaultLogger`（init 初始化的全局 zap）。

**只配 `WithLogger(slogAdapter)` 时**：gnet 内部大多数日志走 slogAdapter（通过 el.getLogger），但少数用 `logging.Xxxf` 包级别函数的地方仍走默认 zap。**这会造成「日志去两处」**。要彻底统一，得额外调 `logging.SetDefaultLoggerAndFlusher`（见 8.6.5）。

### 8.6.4 怎么接 slog（适配器实现）

gnet 的 Logger 接口是 printf 风格，slog 是 key-value 风格，适配器要把前者翻译成后者。

#### 适配器代码（25 行）

```go
package server

import (
    "fmt"
    "log/slog"

    "github.com/panjf2000/gnet/v2/pkg/logging"
)

// slogLogger 把 slog 包装成 gnet 的 logging.Logger
type slogLogger struct {
    sl *slog.Logger
}

func NewSlogLogger(sl *slog.Logger) logging.Logger {
    return &slogLogger{sl: sl}
}

func (s *slogLogger) Debugf(format string, args ...any) {
    s.sl.Debug(fmt.Sprintf(format, args...))
}

func (s *slogLogger) Infof(format string, args ...any) {
    s.sl.Info(fmt.Sprintf(format, args...))
}

func (s *slogLogger) Warnf(format string, args ...any) {
    s.sl.Warn(fmt.Sprintf(format, args...))
}

func (s *slogLogger) Errorf(format string, args ...any) {
    s.sl.Error(fmt.Sprintf(format, args...))
}

func (s *slogLogger) Fatalf(format string, args ...any) {
    // slog 无 Fatal，降级为 Error（gnet 内部几乎不调 Fatalf，见 8.6.5）
    s.sl.Error(fmt.Sprintf(format, args...))
}
```

#### 使用

```go
sl := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

gnet.Run(handler, "tcp://:1883",
    gnet.WithLogger(server.NewSlogLogger(sl)),
    ...
)
```

### 8.6.5 接 slog 的三个细节问题

#### 问题 1：Fatal 语义不对齐

gnet 的 `Fatalf`（zap 实现里）会 `os.Exit(1)`。slog 没有 Fatal。适配器里 Fatalf 降级为 Error——日志记了但进程不退出。

**实际影响**：gnet 内部**几乎不调 Fatalf**（acceptor/engine/eventloop 里没有 Fatalf 调用，致命错误走 `engine.shutdown` return error）。所以这个语义不对齐实际不触发，适配器的 Fatalf 降级为 Error 是安全的。

#### 问题 2：printf → Sprintf 的开销

gnet 调 `logger.Errorf("Accept() failed: %v", err)`，适配器里 `fmt.Sprintf(format, args...)` 会**无条件格式化字符串**，即使日志级别低于 Error 被丢弃。

对比 slog 原生：`sl.Error(msg, "err", err)` 在级别不够时延迟求值。适配器的 `Sprintf` 丢了这个优化。

**实际影响**：gnet 内部日志量不大（主要是错误日志），Sprintf 开销可忽略。但如果你在 OnTraffic 里高频打 Debug 日志，适配器的 Sprintf 有性能损失。**对策**：OnTraffic 里用 slog 直接打（不经适配器），适配器只给 gnet 内部用。

#### 问题 3：两套 logger 分裂（彻底统一）

只配 WithLogger 时，`logging.Errorf`（包级别）仍走默认 zap。要彻底统一：

```go
// 在 gnet.Run 之前
sl := slog.New(...)
adapter := NewSlogLogger(sl)
gnet.WithLogger(adapter)                           // 给 gnet 内部用（el.getLogger）
logging.SetDefaultLoggerAndFlusher(adapter, nil)   // 给 logging.Xxxf 包级别函数用
```

**`SetDefaultLoggerAndFlusher`**（logger.go:189-193）只是改全局变量（mu.Lock + 赋值），随时可调，覆盖 init() 初始化的默认 zap。flusher 传 nil（slog 无需手动 flush，JSON handler 自带）。

### 8.6.6 对 mqtt-gateway 的启示：三方案对比

#### 方案 A：用 gnet 默认 zap（不接 slog）

```go
gnet.Run(handler, "tcp://:1883",
    gnet.WithLogPath("/var/log/gnet"),     // 写文件
    gnet.WithLogLevel(logging.InfoLevel),
    // 不配 WithLogger，用默认 zap
)
```

- **优点**：零代码，gnet 的 zap 自带 lumberjack 滚动、级别控制、文件轮转
- **缺点**：业务日志（OnTraffic/OnClose）用 slog，gnet 内部用 zap——两套日志格式不统一

#### 方案 B：接 slog（适配器）⭐ 推荐

```go
sl := slog.New(slog.NewJSONHandler(os.Stdout, ...))    // 业务也用这个 sl
gnet.Run(handler, "tcp://:1883",
    gnet.WithLogger(NewSlogLogger(sl)),
)
logging.SetDefaultLoggerAndFlusher(NewSlogLogger(sl), nil)   // 统一包级别函数
// OnTraffic 里直接用 sl（不经适配器，保性能）
```

- **优点**：gnet 内部 + 业务统一走 slog，格式一致（JSON），统一收集
- **缺点**：适配器的 Sprintf 开销（仅 gnet 内部日志，量小，可忽略）

#### 方案 C：gnet 用 zap，业务用 slog（混合）

```go
gnet.Run(handler, "tcp://:1883", gnet.WithLogPath(...))  // gnet 内部 zap
sl := slog.New(...)  // 业务用 slog
```

- **优点**：各用各的，零适配
- **缺点**：日志收集要处理两种格式，运维复杂。**不推荐**

**推荐方案 B**：mqtt-gateway 统一用 slog，写 25 行适配器让 gnet 也走 slog。OnTraffic 里用原生 slog（不经适配器，保性能），适配器只服务 gnet 内部日志。

### 8.6.7 ⚠️ reactor 不能在日志里阻塞

gnet 内部日志（通过 el.getLogger）在 **reactor goroutine** 里调用。如果你的 slog handler 是阻塞的（比如同步写远程 syslog），会阻塞 reactor——和 10.5 recover、10.10 OnClose 一样属于「reactor 线程上的代码必须非阻塞」红线。

**对策**：
- slog 用 `slog.NewJSONHandler(os.Stdout, ...)` 写 stdout，非阻塞（内核 pipe buffer）
- 如果写远程（Kafka/ELK），用异步 handler（内部 buffered + 后台 flush）
- 别在 OnTraffic 里高频打日志（每包一条 Debug 会拖慢 reactor）

---

## 第 8.7 站 pkg/pool/goroutine — ants 封装（⚠️ Server 路径用不上）

**职责**：gnet 内部的 goroutine 池，基于 [panjf2000/ants](https://github.com/panjf2000/ants)（gnet 作者的另一个项目）封装。

### 8.7.1 ⚠️ 关键事实：mqtt-gateway 用不上这个 pool

在讲 pool 本体前，先澄清最可能关心的问题：**「mqtt-gateway 的 worker pool（认证、presence 删除、raw log 投递）能用 gnet 的这个 goroutine pool 吗？」**

**不能，也不应该。** 原因：

gnet 内部的 goroutine pool（`DefaultWorkerPool`）在 Unix Server 路径下**只有一个调用点**——`eventloop_unix.go:115` 的 `enroll`，而 `enroll` 是 **Client 模式（主动拨出）** 专用。mqtt-gateway 是 Server（accept 别人连进来），**根本不触发 enroll**，所以 gnet 的 goroutine pool 对你来说**是个不会启用的死代码**。

你的认证/presence/raw log worker pool 必须**自己写**（基于 ants、自定义 channel pool、或 errgroup）。这呼应 CLAUDE.md 的 goroutine 红线「worker pool 固定数量，与 CPU 核数相关」——**这个 worker pool 是 mqtt-gateway 的，不是 gnet 的**。

下面讲清楚 pool 本体（如果你未来要做 Client 模式或想借鉴配置），再讲为什么 Server 路径不用。

### 8.7.2 pool 本体（ants 封装）

`pkg/pool/goroutine/goroutine.go`（71 行）把 ants 包了一层。

```go
// goroutine.go:26-36 三个关键常量
const (
    DefaultAntsPoolSize = 1 << 18        // 256K：pool 容量，最多 26 万并发 worker
    ExpiryDuration      = 10 * time.Second  // 空闲 worker 清理间隔
    Nonblocking         = true             // pool 满时 Submit 不阻塞，返回 ErrPoolOverflow
)

// goroutine.go:43-44  包级别全局 pool
var DefaultWorkerPool = Default()

// goroutine.go:58-70  Default 工厂
func Default() *Pool {
    options := ants.Options{
        ExpiryDuration: ExpiryDuration,
        Nonblocking:    Nonblocking,
        Logger:         &antsLogger{logging.GetDefaultLogger()},  // 接 gnet 的 logging（8.6 站）
        PanicHandler:   func(a any) {
            logging.Errorf("goroutine pool panic: %v", a)   // worker panic 不崩，记日志
        },
    }
    defaultAntsPool, _ := ants.NewPool(DefaultAntsPoolSize, ants.WithOptions(options))
    return defaultAntsPool
}
```

**三个关键配置**：

| 配置 | 值 | 含义 |
|---|---|---|
| `DefaultAntsPoolSize` | `1 << 18 = 262144`（256K）| pool 容量 |
| `ExpiryDuration` | `10s` | 空闲超过 10s 的 worker 被清理（缩容）|
| `Nonblocking` | `true` | pool 满时 Submit 立刻返回 `ants.ErrPoolOverflow`，不阻塞调用者 |

#### 为什么 Nonblocking=true（核心设计）

`enroll` 的调用链（eventloop_unix.go:72/89）可能跑在 reactor goroutine 里。如果 pool 满了且阻塞，**reactor goroutine 会被卡住**——违反「reactor 不能阻塞」红线。所以 Nonblocking=true 是必须的。

Submit 失败时，`enroll` 返回 `err`，调用方（业务代码调 `c.EventLoop().Register(...)`）拿到错误自己处理。gnet 不重试、不兜底。

#### PanicHandler 的保护

ants 的 worker 默认 panic 会让整个进程崩。配了 PanicHandler 后，panic 被捕获记日志，**worker 不崩、pool 继续工作**。这和 §10.5 在 OnTraffic 加 defer recover 是同一个思路——**任何在 reactor 相关路径上跑的代码都要防 panic**。

#### antsLogger 桥接

```go
// goroutine.go:49-56
type antsLogger struct { logging.Logger }   // 嵌入 gnet 的 Logger 接口

func (l antsLogger) Printf(format string, args ...any) {
    l.Infof(format, args...)   // ants 用 Printf 日志，桥接到 gnet Logger.Infof
}
```

ants 的 Logger 接口要求 `Printf`，gnet 的 Logger 有 `Infof`。这个桥接把 ants 日志引到 gnet 的 logging 体系。**如果你配了 `WithLogger(slogAdapter)`（§8.6），ants 的日志也会走 slog**——统一日志链路。

### 8.7.3 gnet 里唯一的调用点（enroll）

Unix Server 路径下，goroutine pool **只在 enroll 用**（eventloop_unix.go:113-150）：

```go
func (el *eventloop) enroll(c net.Conn, addr net.Addr, ctx any) (resCh chan RegisteredResult, err error) {
    resCh = make(chan RegisteredResult, 1)
    err = goroutine.DefaultWorkerPool.Submit(func() {     // ⭐ 唯一调用点
        defer close(resCh)
        var err error
        if c == nil {
            if c, err = net.Dial(addr.Network(), addr.String()); err != nil {   // 主动拨出（阻塞！）
                resCh <- RegisteredResult{Err: err}
                return
            }
        }
        defer c.Close()
        sc, ok := c.(syscall.Conn)
        // ... 把 net.Conn 的 fd 抢出来（RawConn.Control），调 el.poller.Trigger(register) 注册到 reactor
    })
    return
}
```

**enroll 做什么**：Client 模式下，gnet 主动连接远端（`net.Dial`），拿到 `net.Conn`，把底层 fd 抢出来（`syscall.Conn` + `RawConn.Control`），然后 `Trigger(register)` 注册到自己的 reactor。

**为什么要用 goroutine pool**：`net.Dial` 是**阻塞**系统调用（TCP 握手几十毫秒到几秒）。在 reactor goroutine 里直接 Dial 会卡住 reactor，所以丢到 worker 异步 Dial，完成后用 Trigger 回 reactor 注册。

**mqtt-gateway 不触发**：你是 Server，用 `net.Listen + Accept` 接受连接（accept0），连接是别人主动连你，不需要 Dial。`enroll` 是 Client 模式（`gnet.Client` 主动拨出）专用。

### 8.7.4 为什么 Server 路径完全不需要 goroutine pool

理解 gnet 设计的关键。Reactor 模型把所有「非阻塞 I/O + 纯计算」在 reactor goroutine 完成了：

| 操作 | 阻塞吗？ | 在哪跑？ |
|---|---|---|
| Accept | 非阻塞（epoll 唤醒）| reactor goroutine（accept0）|
| Read/Write | 非阻塞（EAGAIN）| reactor goroutine（el.read/el.write）|
| 解析 MQTT 包 | 非阻塞（纯内存）| reactor goroutine（OnTraffic）|
| **net.Dial（主动拨出）** | **阻塞**（TCP 握手）| **gnet 的 goroutine pool**（enroll）|
| **认证（查 DB/Redis）** | **阻塞** | **你的 worker pool**（不在 gnet 里）|
| **presence 写（Redis）** | **阻塞** | **你的 worker pool**（不在 gnet 里）|
| **raw log 投递（Kafka）** | **阻塞** | **你的 worker pool**（不在 gnet 里）|

**核心**：只有「阻塞操作」才需要 worker pool。gnet 自己只有一个阻塞操作（Client 模式的 Dial，在 enroll），所以它的 goroutine pool 只服务 enroll。**mqtt-gateway 的阻塞操作（认证/presence/raw log）是业务自己的，gnet 不管，要自己写 worker pool。**

### 8.7.5 对 mqtt-gateway 的启示：自己写 worker pool 的四个要点

虽然 gnet 的 goroutine pool 用不上，但它的**配置思路值得借鉴**（ants 的 Nonblocking + ExpiryDuration + PanicHandler 是经过验证的生产配置）。

#### 要点 0：worker pool 全景——4 类阻塞操作（先搞清楚有哪些）

从 server.go 源码核对，mqtt-gateway 的阻塞操作分两个阶段，**都要 worker pool（或对应的 bounded queue）**，gnet 的 reactor 不管这些：

| worker pool | 操作 | 源码出处 | 触发时机 | 当前状态 |
|---|---|---|---|---|
| **auth queue**（已有）| 认证（DB 查询验证设备）| server.go:262 `queue.SubmitAndWait` | CONNECT 时 | 已有 bounded queue + worker |
| **ingest queue**（已有）| raw log 投递（Kafka append）| server.go:377/380 `sink.TryAppendAsync`/`SubmitAndWait` | PUBLISH 时 | 已有 bounded queue + worker |
| **presence worker（新建）**| **注册**（NextSessionVersion + Connect，Redis 2 次）| server.go:276/290 | CONNECT 认证通过后 | ⚠️ 当前同步调，gnet 下必须异步 |
| presence worker（同上）| **删除**（Delete，Redis 1 次，带 fencing）| server.go:315 | 断连时 OnClose | ⚠️ 当前 defer 同步，gnet 下异步（§10.10）|

**⚠️ 之前文档只提了「认证 + presence 删除」是错的**。实际有 4 类（含 presence 注册和 raw log 投递）。**presence 注册（CONNECT 时 NextSessionVersion + Connect）在 gnet 下也必须异步**——它是 Redis 阻塞调用，在 auth 回调里同步等会阻塞 auth worker，拖慢整个认证吞吐。

**gnet 下各 worker pool 的协作时序**（CONNECT 全流程）：

```
reactor OnTraffic(handleConnect)
  ├─ ① authQueue.TrySubmit(creds, authCb)        ← 投递认证（非阻塞入队）
  │     └─ auth worker（查 DB）→ authCb:
  │          ├─ ② presenceQueue.TrySubmit(rec, presCb)   ← 投递 presence 注册
  │          │    └─ presence worker（Redis INCR + 条件写）→ presCb:
  │          │         └─ c.AsyncWrite(CONNACK Accepted) + s.state=Connected
  │          └─ （认证失败/presence 满）→ c.AsyncWrite(CONNACK 拒绝) + c.Close()
  └─ return None（立刻让出 reactor，不等 auth/presence 完成）
```

PUBLISH 全流程的 raw log 投递走 ingest queue（已有），见 §10.6 的 AsyncWrite 陷阱和 §10.3 改造 4。

#### 要点 1：用 ants 还是 channel 自己写

| 方案 | 优点 | 缺点 |
|---|---|---|
| **ants**（gnet 用的）| 高性能（worker 复用 + sync.Pool task）、Nonblocking、PanicHandler、自动缩容 | 多一个依赖 |
| **channel + 固定 goroutine** | 零依赖、简单、可控 | 要自己实现 worker 复用、panic 处理、缩容 |

**建议**：auth queue 和 ingest queue 已有（沿用你 server.go 的实现）。新增的 presence worker 用 channel + 固定 N 个 goroutine（注册和删除共用一个 worker pool，因为都是 Redis 操作）：

```go
type presenceWorkerPool struct {
    registerCh chan registerTask   // bounded：CONNECT 时的注册
    deleteCh   chan deleteTask     // bounded：断连时的删除
}

func newPresenceWorkerPool(n int) *presenceWorkerPool {
    p := &presenceWorkerPool{
        registerCh: make(chan registerTask, 4096),
        deleteCh:   make(chan deleteTask, 4096),
    }
    for i := 0; i < n; i++ {
        go p.registerWorker()   // 固定 n 个 goroutine 消费 registerCh
        go p.deleteWorker()     // 固定 n 个 goroutine 消费 deleteCh
    }
    return p
}
```

需求复杂（多种任务、动态扩缩容、优先级）才用 ants。

#### 要点 2：Nonblocking 入队（呼应红线 #4 bounded queue）

gnet 的 ants pool 用 `Nonblocking=true`——pool 满了 Submit 失败。你的 worker pool 的**输入 channel 也要 bounded + 非阻塞入队**：

```go
func (p *workerPool) submitAuth(t authTask) error {
    select {
    case p.authCh <- t:
        return nil
    default:
        return ErrQueueFull   // ⚠️ 不阻塞，返回错误。呼应红线 #4
    }
}
```

**满了怎么办**（按任务类型区分）：

| 任务类型 | 满了策略 | 理由 |
|---|---|---|
| 认证 | 拒连接（CONNACK ServerUnavailable），设备重连 | 认证不过的连接不该堆积 |
| **presence 注册** | **fail-closed：CONNACK ServerUnavailable + Close**（红线 #6：presence 写失败禁止成功 CONNACK）| presence 注册是 CONNECT 成功的必要条件 |
| presence 删除 | **丢弃** | presence 容忍丢失，fencing 保护（§10.10），设备重连写新 presence 覆盖 |
| raw log 投递 QoS1 | 断连不 ACK（红线 #5）| QoS1 不能静默丢失 |
| raw log 投递 QoS0 | 丢弃或降级（本地落盘）| 遥测数据容忍少量丢失 |

**千万别阻塞入队**——OnTraffic 里阻塞会让 reactor 卡住（§10.8 红线）。

#### 要点 3：PanicHandler（呼应 §10.5）

每个 worker goroutine 都要 `defer recover`，且 recover 要在 for 循环**外面**（单次任务 panic 不杀 worker，继续消费下一个）：

```go
func (p *workerPool) authWorker() {
    defer func() {
        if r := recover(); r != nil {           // ⭐ 在 for 外面
            p.log.Error("auth worker panic", "err", r)
            // recover 后这个 goroutine 继续运行（for 循环不退出）
        }
    }()
    for t := range p.authCh {
        // 处理 t（可能 panic，被上面的 recover 兜住）
    }
}
```

**注意**：如果 recover 在 for 循环**里面**，单次 panic 会让 goroutine 退出，pool 缩水。放外面才能保证 worker 长期存活。

### 8.7.6 小结：gnet 的 pool 是给 Client 用的，你的 worker pool 自己写

| | gnet 的 goroutine pool | mqtt-gateway 的 worker pool |
|---|---|---|
| 实现 | ants 封装（DefaultWorkerPool）| 自己写（channel 或 ants）|
| 触发 | `enroll`（Client 模式 Dial）| 认证 / presence 注册+删除 / raw log 投递（4 类，见 8.7.5）|
| Server 用吗 | ❌ 不触发（accept 不走 enroll）| ✅ 这是你的业务核心 |
| 容量 | 256K（Nonblocking）| 你定（建议 N × CPU 核数）|
| panic 处理 | PanicHandler（ants 内置）| 自己 defer recover（for 外）|

**一句话**：gnet 的 goroutine pool 对 mqtt-gateway 是死代码（Server 不走 enroll）。你的 worker pool 自己写，借鉴 gnet 的三个配置要点：**Nonblocking 入队 + bounded channel + recover 在 for 外**。

### 8.7.7 ⚠️ 认证洪水的容量设计（设备集体上线怎么办）

逆变器集体上线（固件升级触发、区域网络恢复、早晨批量开机）会产生**认证洪水**：瞬间大量 CONNECT 同时到达，每个触发 auth + presence 注册。如果 worker pool 设计不当，会导致认证堆积、设备超时重连、洪水放大。本节给出扛住洪水的四道闸门 + 一个超时机制。

#### 洪水的危险链

```
瞬间 1 万个设备 CONNECT
  → accept0 全部 accept（gnet 无界 accept，不拒绝！）
  → 全部 OnTraffic → handleConnect → 投递 auth queue
  → auth queue 容量 5000，前 5000 个入队，后 5000 个被拒
  → auth worker（32 个）处理速度 1600/s，前 5000 个排队 ~3 秒
  → 排队期间设备等 CONNACK，部分超时（5-10s）→ 设备断连重连 → 第二波 CONNECT
  → 洪水放大
```

#### 四道闸门（必须配齐才能扛）

##### 闸门 1：auth queue bounded（满了拒）

```go
// handleConnect 里投递认证
err := authQueue.TrySubmit(creds, authCb)
if err == ErrQueueFull {
    c.AsyncWrite(mqtt.EncodeConnAck(nil, false, mqtt.ConnAckServerUnavailable), nil)
    c.Close()   // ⭐ 拒绝，设备退避重连（红线 #9 要求设备退避带 jitter）
    return
}
```

**抗洪水作用**：auth queue 容量（建议 5000-10000）是认证洪水的第一道闸门。满了直接拒，防止 worker pool 积压导致 OOM。被拒的设备退避重连（要求设备端实现退避 + jitter，服务端不能假设设备会善意退避——红线 #9）。

##### 闸门 2：auth worker 数 = DB 连接池大小

```
auth worker 数 = 32
单次认证（DB 查询）= 10ms
认证吞吐 = 32 / 0.01s = 3200/s

DB 连接池 = 32 个连接
→ auth worker 数 > 32 无意义（多余的 worker 拿不到 DB 连接，空等）
```

**抗洪水作用**：auth worker 数决定认证吞吐上限。**必须和 DB 连接池大小匹配**。worker 数太少 → 认证慢 → 排队长 → 设备超时重连。worker 数太多（超过 DB 连接池）→ worker 空等 DB 连接，无效。

##### 闸门 3：presence queue bounded（fail-closed）

auth 通过后投递 presence 注册，也要 bounded：

```go
// authCb 里投递 presence 注册
err := presenceWorker.TrySubmitRegister(rec, presCb)
if err == ErrQueueFull {
    c.AsyncWrite(mqtt.EncodeConnAck(nil, false, mqtt.ConnAckServerUnavailable), nil)
    c.Close()   // ⭐ fail-closed：presence 写不了 = CONNECT 失败（红线 #6）
    return
}
```

**抗洪水作用**：presence queue 是 auth 之后的第二道闸门。满了 fail-closed（拒绝，不让设备进来），防止「认证过了但 presence 写不进」的连接堆积。

##### 闸门 4：OnOpen 的 MaxConnections（总连接数上限）

这是最危险的一点——**gnet 的 accept0 是无界 accept**（第 4 站讲过），不管 auth/presence queue 满不满都 accept。如果没有总连接数限制，认证洪水时 accept 进来的 conn 对象 + session 会占内存。

**对策**：OnOpen 里 `connCount` 上限（§10.3 改造 2）：

```go
func (h *handler) OnOpen(c gnet.Conn) ([]byte, gnet.Action) {
    if h.connCount.Add(1) > h.cfg.MaxConnections {   // ⭐ 总连接数闸门
        h.connCount.Add(-1)
        return nil, gnet.Close
    }
    // ... 正常登记
}
```

**注意**：MaxConnections 要大于「auth queue + presence queue 容量之和 + 已连接的活跃数」。比如 auth queue 5000 + presence queue 5000 + 活跃 50 万 = MaxConnections 至少 51 万。否则正常运行的连接会被认证洪水的排队挤掉。

#### 一个超时机制：认证超时踢除（防设备重连放大洪水）

**最隐蔽的洪水放大**：设备 CONNECT 后等 CONNACK，如果 auth+presence 排队 3-5 秒才完成，**设备的 CONNECT 超时可能先触发**（很多 MQTT 客户端 CONNECT 超时 5-10 秒）。设备超时断连重连 → 第二波 CONNECT → 洪水放大。

**对策**：给每个「等认证」的连接设超时，超过比如 3 秒还没认证完就主动踢掉（让设备退避重连，而不是堆积在 queue 里）。

⚠️ **不能用 time.Timer**（违反红线 #2 Session pointer-poor）。用 **OnTick 统一扫描**（像保活判死那样）：

```go
// session 里加字段
type session struct {
    ...
    connectTime int64   // ⭐ CONNECT 到达时间（unix nano），atomic
    state       connState
}

// handleConnect 里记录时间
s.connectTime = time.Now().UnixNano()
// 投递 auth（不等）

// OnTick 里扫描「等认证超时」的连接（复用 10.7 的 connRegistry 扫描）
func (h *handler) OnTick() (time.Duration, gnet.Action) {
    now := time.Now().UnixNano()
    h.registry.mu.RLock()
    for _, s := range h.registry.conns {
        if s.state == stateWaitingConnect && now-s.connectTime > int64(3*time.Second) {
            // ⭐ 等认证超 3 秒，踢掉
            _ = s.c.Close()
        }
        // ... 保活判死 + outboundBuffer 检查（10.7 + 10.6）
    }
    h.registry.mu.RUnlock()
    return 1 * time.Second, gnet.None   // 1 秒扫一次（认证超时需要比保活更频繁）
}
```

**效果**：认证洪水的 conn 最多在 queue 里排 3 秒，超时就被踢，腾出 queue 名额给新连接。设备被踢后退避重连（jitter 打散），不会集中重试。

#### 容量设计实例

假设单节点 50 万活跃连接，设备批量上线峰值 1 万 CONNECT/秒：

| 闸门 | 配置 | 推导 |
|---|---|---|
| auth queue 容量 | 10000 | 峰值 1 万/s，auth 吞吐 3200/s，queue 能缓冲 ~3 秒 |
| auth worker 数 | 32 | = DB 连接池大小，认证吞吐 3200/s |
| presence queue 容量 | 10000 | 同 auth（认证通过后立刻投递 presence）|
| presence worker 数 | 32 | = Redis 连接池大小（Redis 快，可适当少）|
| MaxConnections | 60 万 | 50 万活跃 + auth queue 1 万 + presence queue 1 万 + 余量 |
| 认证超时 | 3 秒 | OnTick 扫描，超时踢除防堆积 |
| OnTick 间隔 | 1 秒 | 认证超时扫描需要比保活（10 秒）更频繁 |

**验证**：峰值 1 万 CONNECT/s，auth 吞吐 3200/s，queue 缓冲 3 秒后认证超时踢除 → 实际进入认证的 ~9600 个（3200×3），剩下 400 个被超时踢 → 设备退避重连（jitter 打散）→ 稳态后认证吞吐跟上 → 洪水消退。

#### 小结：抗洪水的核心是「快拒绝 + 不堆积」

| 原则 | 体现 |
|---|---|
| **快拒绝** | auth/presence queue 满了立刻 CONNACK ServerUnavailable + Close，不排队 |
| **不堆积** | 认证超时 3 秒踢除，conn 对象和 queue 名额不长期占用 |
| **退避打散** | 被拒/被踢的设备退避 + jitter 重连，不集中重试（红线 #9）|
| **闸门分层** | accept（OnOpen MaxConnections）→ auth queue → presence queue → 认证超时，层层过滤 |

**最危险的是 gnet 的无界 accept**——accept0 不管下游 queue 满不满都 accept。所以 OnOpen 的 MaxConnections + auth/presence queue 满拒 + 认证超时踢除，这三者配合是扛认证洪水的关键。

---

## 第 9 站 连接集合 `conn_map.go` / `conn_matrix.go`

**职责**：eventloop 持有的「fd → conn」映射。**无锁**（只有归属的 reactor 访问）。

⚠️ **重要**：v2.4.1 默认构建用 **`conn_map.go`**（build tag `!gc_opt`），只有开 `-tags gc_opt` 才用 `conn_matrix.go`。

| 文件 | build tag | 实现 | 用途 |
|---|---|---|---|
| `conn_map.go` | `!gc_opt`（**默认**）| `map[int]*conn` + 1 个 `atomic.Int32` 计数 | 简单实现 |
| `conn_matrix.go` | `gc_opt` | 二维数组 `[row][column]` + 多 atomic 计数 | 高性能查找（O(1) 数组访问，减少 GC 扫描）|

### 9.1 默认实现 conn_map.go（conn_map.go:25）

```go
type connMatrix struct {       // 注意：类型名都叫 connMatrix，靠 build tag 切换
    connCount int32
    connMap   map[int]*conn
}
```

方法：`init`（:30）/ `iterate`（:34）/ `incCount`（:44）/ `loadCount`（:48）/ `addConn`（:52）/ `delConn`（:58）/ `getConn`（:63）。`loadCount` 用 atomic，因为 LB 会跨 reactor 读。

### 9.2 gc_opt 实现 conn_matrix.go（conn_matrix.go:25）

二维数组 `[gfd.ConnMatrixRowMax][]*conn`，`fd2gfd map[int]gfd.GFD` 辅助查找，删除时做 compaction（把末尾元素搬到空洞）。conn 的 `gfd` 编码了 row/column，O(1) 定位。

---

## 第 9.5 站 链路总图：四条链路怎么串起来

前面 9 站分别讲了各组件，这一站把它们**织成一张图**——看清楚 accept / read / write / control 四条链路怎么在「主 reactor + sub reactor + Trigger 桥梁」之间流转。这张图是理解 gnet 的「终局认知」。

### 9.5.1 全景图：主从 reactor + Trigger 桥梁

```
┌─────────────────────────────────────────────────────────────────────┐
│                      主 reactor (ingress)                            │
│  engine_unix.go:174  el.rotate() → poller.Polling(el.accept0)       │
│  reactor_default.go:33  Polling 循环只调 accept0                    │
│                                                                      │
│  ┌─ accept 链路（第 4 站）─────────────────────────────────────┐     │
│  │ accept0 (acceptor_unix.go:30)                               │     │
│  │   for { Accept → LB.next(addr) → newStreamConn            │ │     │
│  │         → poller.Trigger(High, el.register, c) ──────────┐ │ │     │
│  │   }                                                       │ │ │     │
│  └───────────────────────────────────────────────────────────┘ │ │     │
│                                                                │ │ │     │
│  ┌─ control 链路（OnTick）──────────────────────────────────┐  │ │     │
│  │ ticker goroutine (engine_unix.go:181)                    │  │ │     │
│  │   for { OnTick() → 扫 connRegistry → c.Close()/AsyncWrite │  │ │     │
│  │         → poller.Trigger(Low, el.close, c) ───────────────┼──┤ │     │
│  │   }                                                       │  │ │     │
│  └───────────────────────────────────────────────────────────┘  │ │     │
└─────────────────────────────────────────────────────────────────┼─┼─────┘
                                                                  │ │
                              Trigger (eventfd + 双队列)           │ │
                              poller_epoll_default.go:90           │ │
                  ┌──────────────────────────────────────────────┘ │
                  │   跨 goroutine 边界：任务入队 + CAS 唤醒         │
                  ▼                                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    sub reactor #0..#N (orbit)                        │
│  reactor_default.go:46  el.orbit() → poller.Polling(callback)       │
│                                                                      │
│  Polling 循环（poller_epoll_default.go:115）做两件事：               │
│                                                                      │
│  ① 处理 epoll 网络事件 (callback = c.processIO)                      │
│  ┌─ read 链路（第 5 站）──────────────────────────────────────┐     │
│  │ processIO (connection_linux.go:28)                         │     │
│  │   EPOLLIN → el.read (eventloop_unix.go:254)                │     │
│  │     unix.Read(c.fd, el.buffer)  ← 共享 buffer              │     │
│  │     OnTraffic(c)  ← 你的业务                                │     │
│  │     没读走的 → c.inboundBuffer (半包合并)                   │     │
│  │   EPOLLOUT → el.write (eventloop_unix.go:309)              │     │
│  │     writev 刷 outboundBuffer → 全发完 ModRead 撤销         │     │
│  └────────────────────────────────────────────────────────────┘     │
│                                                                      │
│  ② doChores：消费 Trigger 投递的任务 (144-178)                       │
│  ┌─ register 任务（从 accept0 来）────────────────────────────┐     │
│  │ el.register(c) → register0(c) (eventloop_unix.go:214)      │     │
│  │   ① AddRead: fd 加进 epoll，绑 processIO                   │     │
│  │   ② addConn: connMap[c.fd]=c (无锁，本 reactor 私有)       │     │
│  │   ③ open(c): 调你的 OnOpen                                  │     │
│  └────────────────────────────────────────────────────────────┘     │
│  ┌─ asyncWrite 任务（从业务 goroutine 来）────────────────────┐     │
│  │ c.asyncWrite(hook) → c.write(data) (connection_unix.go:139)│     │
│  │   unix.Write → EAGAIN 暂存 outboundBuffer + ModReadWrite   │     │
│  │   → callback(c, err)                                        │     │
│  └────────────────────────────────────────────────────────────┘     │
│  ┌─ close 任务（从 OnTick / 业务 来）─────────────────────────┐     │
│  │ el.close(c) (eventloop_unix.go:362)                        │     │
│  │   OnClose(c, err) → 榨干 outbound → release → close fd    │     │
│  └────────────────────────────────────────────────────────────┘     │
│                                                                      │
│  conn_map（无锁，只本 reactor 访问）：fd → *conn                     │
└──────────────────────────────────────────────────────────────────────┘
                  ▲                                                ▲
                  │                                                │
┌─────────────────┴────────────────────────────────────────────────┴───┐
│              业务 goroutine（worker pool / 任意）                     │
│                                                                      │
│  ┌─ write 链路入口（第 6 站 + 10.6）──────────────────────────┐      │
│  │ c.AsyncWrite(buf, cb) (connection_unix.go:498)            │      │
│  │   → poller.Trigger(High, c.asyncWrite, hook) ─────────────┼──→ sub│
│  │ c.Close() (connection_unix.go:540)                         │      │
│  │   → poller.Trigger(Low, el.close, c) ──────────────────────┼──→ sub│
│  │ c.EventLoop().Execute(ctx, runnable) (eventloop_unix.go:72)│      │
│  │   → poller.Trigger(Low, runnable.Run, nil) ────────────────┼──→ sub│
│  └────────────────────────────────────────────────────────────┘      │
└──────────────────────────────────────────────────────────────────────┘
```

### 9.5.2 四条链路的起止点速查

| 链路 | 起点 | 跨界方式 | 终点（你的代码）|
|---|---|---|---|
| **accept** | 主 reactor 的 epoll（listener fd 可读）| 主→sub：`Trigger(High, register)` | sub 的 `OnOpen` |
| **read** | sub reactor 的 epoll（conn fd 可读）| 同 reactor 内（无跨界）| sub 的 `OnTraffic` |
| **write** | 业务 goroutine 调 `c.AsyncWrite` | 业务→sub：`Trigger(High, asyncWrite)` | sub 的 `c.write` → `unix.Write` |
| **control** | ingress 的 ticker goroutine（OnTick）或业务 goroutine | ingress→sub 或 业务→sub：`Trigger(Low, close/...)` | sub 的 `el.close` 等 |

### 9.5.3 关键交叉点：Trigger 是唯一的跨界桥梁

四条链路里有三条要**跨 goroutine**（accept、write、control），它们**全部通过 Trigger** 实现跨界，没有例外：

```
跨界方向              调用方式                          优先级    典型任务
─────────────────────────────────────────────────────────────────────────
主reactor → sub      Trigger(High, register, c)        High     新连接注册
业务 goroutine → sub  Trigger(High, asyncWrite, hook)   High     PUBACK 回写
业务 goroutine → sub  Trigger(Low, close, c)            Low      关连接
ingress → sub        Trigger(Low, close, c)            Low      保活判死踢连接
ingress → sub        Execute → Trigger(Low, runnable)   Low      检查超时
sub → 自己           Trigger(Low, read0/write0, c)     Low      ET 模式二次读/写
sub → 自己           Trigger(High, asyncWrite, hook)   High     AsyncWrite（conn 归自己时）
```

**Trigger 的物理实现**（第 8 站）：`GetTask` + 入双队列 + `CAS(wakeupCall)` + `write(eventfd)` 唤醒对端 reactor 的 `EpollWait`。这是 gnet 并发安全的根基——**跨 goroutine 永远投递任务，绝不直接访问对端 reactor 的数据结构**。

### 9.5.4 无锁 conn_map 的成立条件

`conn_map`（conn_map.go:25）是个普通 `map[int]*conn`，**无锁**。这能成立是因为所有访问它的操作都在 **sub reactor 的单一 goroutine** 里：

| 操作 | 触发链路 | 在哪个 goroutine |
|---|---|---|
| `addConn` | accept 链路：register 任务（Trigger 投递过来）| sub reactor |
| `getConn` | read 链路：orbit 的 callback | sub reactor |
| `delConn` | read/control 链路：el.close | sub reactor |
| `iterate` | （业务层用不了，包内私有）| sub reactor |

**跨 reactor 想操作 conn_map？只能 Trigger 投递任务**。比如主 reactor 要注册新 conn，不直接 `connMap[fd]=c`，而是 `Trigger(register)` 让 sub reactor 自己 addConn。这是 reactor 模式「数据局部性 + 无锁」的物理基础。

### 9.5.5 两条 reactor 循环的对照

主 reactor 和 sub reactor 都跑 `poller.Polling`，但 callback 不同，职责完全不同：

| | 主 reactor (ingress) | sub reactor |
|---|---|---|
| 启动 | `el.rotate()`（reactor_default.go:27）| `el.orbit()`（reactor_default.go:46）|
| Polling 的 callback | `el.accept0`（只 accept）| `func(fd,ev,flags){ c.processIO(fd,ev,flags) }`（处理 conn I/O）|
| 监听的 fd | listener fd（们）| 该 reactor 管的 conn fd（们）+ eventfd |
| 管的 conn_map | 不管（ingress 没有自己的连接）| 管（`el.connections`）|
| 跑 OnTick？ | 是（engine_unix.go:181 单独 goroutine）| 否（默认模式）|
| 跑 OnTraffic/OnOpen？ | 否 | 是 |

**核心分工**：主 reactor 专心 accept 抗连接风暴，sub reactor 专心跑业务 I/O。两者通过 Trigger 协作，互不阻塞。

### 9.5.6 一个完整连接的生命周期（四链路串联）

把四条链路按时间顺序串起来，看一个连接从建立到断开的完整旅程：

```
t0  设备 TCP 连接到 listener
t1  ┌─ accept 链路 ──────────────────────────────────────────┐
    │ 主 reactor: accept0 → newStreamConn → Trigger(register)│
    │ sub reactor: register0 → AddRead + addConn + OnOpen    │
    │   你的 OnOpen: 登记 session, 返回 (nil, None)          │
    └─────────────────────────────────────────────────────────┘
t2  设备发 CONNECT 包
t3  ┌─ read 链路 ────────────────────────────────────────────┐
    │ sub reactor: processIO(EPOLLIN) → el.read → OnTraffic  │
    │   你的 OnTraffic: state=WaitingConnect → handleConnect │
    │     Peek CONNECT → ParseConnect → 投递 auth queue      │
    │     (不阻塞，return None)                              │
    └─────────────────────────────────────────────────────────┘
t4  ┌─ write 链路（认证完成后）───────────────────────────────┐
    │ worker goroutine: auth 回调 → c.AsyncWrite(CONNACK)    │
    │   → Trigger(High, asyncWrite)                          │
    │ sub reactor: c.asyncWrite → c.write → unix.Write 出网  │
    └─────────────────────────────────────────────────────────┘
t5  设备发 PUBLISH QoS1
t6  ┌─ read 链路 ────────────────────────────────────────────┐
    │ sub reactor: OnTraffic → handleData → ParsePublish     │
    │   → ACL → 投递 ingest queue（不阻塞）                  │
    └─────────────────────────────────────────────────────────┘
t7  ┌─ write 链路（durable ack 后）───────────────────────────┐
    │ worker goroutine: ack 回调 → c.AsyncWrite(PUBACK)      │
    │   → Trigger(High, asyncWrite) → 出网                   │
    └─────────────────────────────────────────────────────────┘
t8  设备 PINGREQ → read 链路 → OnTraffic 回 PINGRESP（write 链路）
t9  设备超时无活动 或 主动 DISCONNECT
t10 ┌─ control 链路（保活判死 / 正常断连）────────────────────┐
    │ ingress OnTick: 扫 connRegistry → c.Close()            │
    │   或 sub reactor: processIO(EPOLLRDHUP) → el.close     │
    │ sub reactor: el.close → OnClose → release → close fd   │
    │   你的 OnClose: deletePresence fencing + 移除 registry │
    └─────────────────────────────────────────────────────────┘
```

**每个阶段都映射到前面讲过的某一站**：
- t1 accept 链路 = 第 4 站 + 本站（register0）
- t3/t6/t8 read 链路 = 第 5 站
- t4/t7/t8 write 链路 = 第 6 站 + 10.6 AsyncWrite 陷阱
- t10 control 链路 = 10.7 OnTick 保活判死

### 9.5.7 这张图回答的三个核心问题

**Q1：为什么 gnet 能用少量 goroutine 扛百万连接？**
- 主 reactor 1 个 + sub reactor N 个（=CPU 核数）+ ticker 1 个 = 固定 goroutine 数
- 每个 sub reactor 串行处理它管的几万个 conn 的就绪事件（epoll 只返回有数据的）
- 不随连接数增长 goroutine，满足 goroutine 红线

**Q2：跨 goroutine 怎么安全通信？**
- 唯一答案：Trigger 投递任务到对端 reactor 的队列
- 绝不直接访问对端 reactor 的 conn_map / conn 字段
- Trigger 靠 eventfd 唤醒 + CAS 合并保证高效

**Q3：业务代码（OnOpen/OnTraffic/OnClose）在哪个 goroutine 跑？**
- 都在**该 conn 所属的 sub reactor 的 goroutine** 里
- 所以业务代码必须非阻塞（慢操作丢 worker pool）
- 同一 sub reactor 的多个 conn 的回调是串行的（el.buffer 等共享资源因此安全）

---

## 第 10 站 迁移点清单：你的 server.go → gnet

### 10.1 gnet **替你做**的（这些代码可以删）

| 你现在的代码 | gnet 对应 |
|---|---|
| `net.Listen + Accept()` 循环（Serve:141） | 框架自动（acceptor_unix.go:30）|
| `go s.handleConn(conn)` 每连接 goroutine（Serve:158） | ❌ 不需要，reactor 事件驱动 |
| `conn.SetReadDeadline + ReadPacket`（connectedLoop:325） | `OnTraffic(c)` + `c.Peek/Read` |
| `conn.Write(resp)`（多处） | `c.AsyncWrite(resp, nil)` 或 reactor 内 `c.Write` |
| `s.ln.Close()` + `s.conns` 遍历关闭（Stop:170） | `eng.Stop()` 框架统一关（engine_unix.go:198）|
| `trackConn/untrackConn`（server.go:186-200） | 不需要，conn_map 自动管 |
| TCP keepalive | `WithTCPKeepAlive` |

### 10.2 你**仍要自己做**的（业务逻辑不变）

| 你现在的代码 | 迁移后怎么放 |
|---|---|
| CONNECT 解析 + 认证 + presence 写（handleConn:222-253） | `OnOpen` 或第一次 `OnTraffic`（建议后者，因为 CONNECT 是首包）|
| `authAndRegister` / `writePresence`（server.go:258-309） | 同上，业务不变 |
| `deletePresence` fencing 删除（handleConn:249） | `OnClose(c, err)` 里调 |
| PUBLISH → ACL → raw log（handlePublish:356） | `OnTraffic` 里解析 PUBLISH 后调 |
| QoS1 等 durable ack 才 PUBACK（handlePublish:379） | `OnTraffic` 里投递后**不阻塞**，ack 回调里 `AsyncWrite` |
| `maxIdleWhenKeepAliveZero` 兜底（server.go:48） | `OnTick` 里判死 |
| `defer s.recoverConn` panic 隔离（handleConn:216） | ⚠️ **gnet 未内置 recover**，必须自己在 `OnTraffic`/`OnOpen`/`OnClose`/`OnTick` 里 defer recover（见 10.5）|

### 10.3 ⚠️ 需要重点改造的（gnet 模型不同）

**改造 1：MQTT 包解析从「阻塞 ReadFull」改成「流式 Peek+Discard」**
- 现在：`mqtt.ReadPacket(conn, max)` 内部 `io.ReadFull` 阻塞读够一整包。
- gnet：`OnTraffic` 被调时缓冲可能只有半包，用 `c.Peek(n)` 探测、`c.InboundBuffered()` 判断、`c.Discard(n)` 推进。
- 详见 5.5 和 6.2。

**改造 2：bounded accept（红线 #4）**
- 现在：`s.sem` 信号量（server.go:156）。
- gnet：源码没有。要在 `OnOpen` 里 `atomic.AddInt64(&connCount, 1)`，超限就 `return gnet.Close`。**注意是 close 新连接，不是拒绝 accept**（accept0 已经把它从内核队列取出来了）。

**改造 3：保活判死（红线相关的 goroutine 红线）**
- 现在：`conn.SetReadDeadline(1.5*keepalive)`（connectedLoop:325），靠 read 超时触发断连。
- gnet：**v2.4.1 的 `SetReadDeadline` 返回 `ErrUnsupportedOp`，根本不支持！**（connection_unix.go:555）。必须用 `OnTick` 周期扫描连接检查最后活跃时间。
- ⚠️ **两个隐蔽难点**：
  1. **OnTick 跑在 ingress reactor 上**（默认模式，engine_unix.go:181），不是每个 sub reactor 一个 ticker。
  2. **`Engine` 接口没有暴露 iterate 连接的能力**——`eventLoops.iterate` 是包内私有 API（只在 `CountConnections` 内部用，gnet.go:76），业务从 handler 外部**拿不到「所有 conn」的列表**。所以 OnTick 里没法直接遍历所有连接判死。
  3. ⚠️ **不要用 `c.EventLoop().Execute` 投递**——`Execute` 针对单个 conn 的 EventLoop，需要先有 conn 引用，但 OnTick 在 ingress 没有 sub reactor 的 conn 列表。
- **正确做法**：业务层自己维护一个 `connRegistry`（OnOpen 登记、OnClose 移除），OnTick 扫描这个 registry。超时的 conn 调 `c.Close()`（并发安全，内部 Trigger 到 sub reactor 执行）。完整实现见 **10.7 节**。

**改造 4：QoS1 同步等待不能阻塞 reactor**
- 现在：`s.sink.SubmitAndWait(ctx, msg)` 阻塞当前连接的 goroutine（handlePublish:380）。
- gnet：**reactor 不能阻塞**，否则饿死同 reactor 的其他连接。
- 方案：`OnTraffic` 里把消息投递给 ingest queue 后**不阻塞**，等 durable ack 到达时通过 `c.AsyncWrite` 回写 PUBACK（用 `OnClose` 的 fencing 逻辑保护）。v2.4.1 的 `EventLoop().Execute`（gnet.go:410）也能安全地把回写任务投递回 reactor。

**改造 5：per-conn 状态存放**
- 现在：函数参数传递 `Principal{TenantID, DeviceID}`。
- gnet：`c.SetContext(&session{...})`，需要时 `c.Context().(*session)`。
- **注意红线 #2**：session 要 pointer-poor，别塞 map/channel/timer。

### 10.4 gnet 红线对齐检查表

| 你的红线 | gnet 是否满足 | 说明 |
|---|---|---|
| #1 QoS1 PUBACK 只能在 durable ack 后 | ⚠️ 要改造 4 | 不能在 OnTraffic 同步阻塞等 |
| #2 Session pointer-poor | ✅ 天然契合 | reactor 模型本就无 per-conn goroutine/timer |
| #3 payload bounded-memory | ⚠️ 注意 el.buffer 共享 | OnTraffic 里拷贝，别持有切片 |
| #4 所有 queue bounded | ⚠️ 要改造 2 | gnet 无 bounded accept，自己加 |
| goroutine ≤ conn×1.x | ✅ 完美 | reactor 数 = NumCPU，固定 |

### 10.5 ⚠️ gnet **未内置** panic recover —— reactor 单点故障防护

**先纠正一个常见误解**：gnet 的 `Polling`（poller_epoll_default.go:115）对 `callback`（processIO/accept0）和 `task.Exec`（Trigger 投递的任务）**都没有 `defer recover()`**。

证据：看 `Polling` 的 132-142 行（处理网络事件）和 146-163 行（处理任务），都是裸调用 `callback(...)` / `task.Exec(...)`，没有 recover 包裹。一旦这些回调 panic，panic 会一路冒泡出 `Polling` → 出 `orbit()`/`rotate()`/`run()`（reactor_default.go）→ 被 `eng.concurrency.Go(...)` 的 errgroup 捕获成 error → 触发 `engine.shutdown`。

**后果**：一个 conn 的 OnTraffic 里 panic（比如畸形包解析、空指针），**整个 reactor goroutine 退出**，这个 reactor 管的所有连接（几千到几万）**瞬间全部失联**。其他 reactor 不受影响（这是好消息，故障被隔离在单个 reactor）。

**和你 server.go 的差异**：

| | 你现在 server.go | gnet 迁移后 |
|---|---|---|
| panic 影响 | 单连接（`defer s.recoverConn` 只 recover 自己，server.go:206）| **单 reactor 的全部连接**（几千~几万）|
| recover 责任 | 框架（你在 handleConn 顶部 defer recover）| **业务方（你在每个 EventHandler 回调里 defer recover）**|

**正确写法**：在每个可能 panic 的 EventHandler 回调里加 defer recover，recover 后 `return gnet.Close` 关闭肇事连接，保护 reactor：

```go
func (h *handler) OnTraffic(c gnet.Conn) (action gnet.Action) {
    defer func() {
        if r := recover(); r != nil {
            h.log.Error("OnTraffic panic; closing conn",
                "remote", c.RemoteAddr(), "panic", r, "stack", string(debug.Stack()))
            action = gnet.Close   // ⭐ 关闭肇事连接，不让 reactor 崩
        }
    }()
    // ... 业务逻辑（解析 MQTT、ACL、投递 raw log）
    return gnet.None
}
```

**注意几个点**：
1. `OnOpen` / `OnClose` / `OnTick` 也建议加同样的 defer recover（尤其 OnTick，它崩了会让 ingress reactor 挂掉，影响整个 engine）。
2. 通过 `EventLoop().Execute` / `Trigger` 投递的 task 也要在自己内部 recover（task 在 reactor goroutine 里执行，panic 同样会拖垮 reactor）。
3. recover 后「关连接」而不是「继续」，因为 panic 通常意味着 session 状态已不一致，继续用风险更大（呼应你 server.go:208 的处理思路）。
4. 这只是**故障隔离**（单 conn 不拖垮 reactor），不是**故障消除**。根因还是要在 codec 层尽量返回 error 而非 panic（你 server.go:205 的注释也是这个原则）。

> 红线对齐：CLAUDE.md「goroutine 红线：每连接只允许 1 个 read loop goroutine」的代价就是 reactor 单点。recover 是必须的兜底，不能省。

### 10.6 ⚠️ AsyncWrite 写路径的陷阱（QoS1 PUBACK 必看）

`c.AsyncWrite` 的全链路（业务调 → Trigger → reactor drain → `c.asyncWrite` → `c.write` → `unix.Write`，EAGAIN 时暂存 `outboundBuffer` + `ModReadWrite`，EPOLLOUT 唤醒后 `el.write` 用 writev 重发）有 5 个容易踩的陷阱，对 QoS1 PUBACK 影响最大：

#### 陷阱 1：PUBACK 必须用 HighPriority，别用 Execute/LowPriority

**反例**：
```go
// ❌ 错误：把 PUBACK 回写投递成 LowPriority
el.poller.Trigger(queue.LowPriority, writePuback, ...)
//   或 c.EventLoop().Execute(ctx, ...) —— Execute 内部用 LowPriority（eventloop_unix.go:79）
```

**原因**：LowPriority 任务进 `asyncTaskQueue`，reactor 每轮最多只 drain 256 个（`MaxAsyncTasksAtOneTime`，poller_epoll_default.go:154）。连接风暴时 async 队列里堆了大量任务，PUBACK 可能**延迟好几个 Polling 周期**才发，设备等 PUBACK 超时重发 PUBLISH，放大流量。

**正例**：`c.AsyncWrite` 默认就是 HighPriority（connection_unix.go:510），走 `urgentAsyncTaskQueue`（每轮全 drain，poller_epoll_default.go:146-153）。**PUBACK 直接用 AsyncWrite，别自己包 Trigger/Execute**。

#### 陷阱 2：callback 的 `err` ≠ 设备已收到

**误解**：`c.AsyncWrite(buf, cb)` 的 callback 拿到 `nil`，就以为设备收到了。

**真相**：callback 的 `nil` 只代表「数据进了发送路径」——要么 `unix.Write` 直接出了网，要么 EAGAIN 时被暂存进了 `outboundBuffer`（connection_unix.go:163）等下次 EPOLLOUT。**两种情况下 callback 都是 nil，但后一种数据可能好几秒才真出网**（如果设备 TCP 接收窗口一直满）。

**对你 QoS1 的含义**：

| 阶段 | 你现在 server.go | gnet 迁移后 |
|---|---|---|
| durable ack 到达 | `conn.Write(puback)` 同步，成功 = 字节进内核 send buffer | `c.AsyncWrite(puback, cb)` 异步，cb=nil = 字节进 outboundBuffer 或内核 send buffer |
| 「设备真的收到 PUBACK」 | 靠 TCP 可靠性 + 设备下次 PUBLISH 隐式确认 | **完全一样**——gnet 没改变这一点 |

**QoS1 红线「PUBACK 只能在 durable ack 之后发」的语义仍然成立**：你在 durable ack 回调里调 `c.AsyncWrite(puback, ...)`， PUBACK 字节最早在 durable ack 之后进入发送路径。但「设备真的收到」这个语义 gnet 和 server.go 都没法保证——TCP 是可靠传输但有时延，需要靠设备的下一次 PUBLISH（或 QoS2 PUBREC，你第一阶段不做）来隐式确认。

> 实务结论：不要用 AsyncWrite 的 callback 去做「设备已确认」的下游动作。callback 只适合做轻量的日志/metrics（比如「PUBACK 已进入发送路径」计数）。

#### 陷阱 3：outboundBuffer 无界增长——必须自己加两道闸门防 OOM

**先看源码事实**：`c.outboundBuffer` 是 `elastic.Buffer` 类型（elastic_ring_list_buffer.go:29），结构是 `ringBuffer（固定 maxStaticBytes，默认 64KB）+ linkedlist.Buffer（链表）`。`Write`（:84-98）超过 ringBuffer 容量的部分 `PushBack` 进链表——**链表无界增长**，没有上限。

数据进 outboundBuffer 的唯一触发条件：`c.write` / `el.write` 时 `unix.Write` 返回 `EAGAIN` 或部分写入（connection_unix.go:163、176；eventloop_unix.go:334），剩余数据进 buffer。**而 EAGAIN 的根本原因是对端（设备）不收数据**——TCP 接收窗口满或网络拥塞。

**所以 outboundBuffer 增长 = 设备已经在卡顿**，不是「可能发生」而是「设备已经出问题」的信号。

**这破坏了你的红线 #4（所有 queue bounded）**。你现在的 server.go 用 `conn.Write` 同步写，写不进就 error 断连（server.go:383）；gnet 下 `c.AsyncWrite` 永远返回 nil（投递成功），数据进 outboundBuffer 暂存，**不会立即失败**——你失去了「写失败立即感知」的能力，必须自己补两道闸门。

##### outboundBuffer 的生命周期（理解两道闸门的时机基础）

outboundBuffer 一辈子在三个状态间转换。理解这个状态机，才能准确设两道闸门的阈值：

```
        ┌─────────────┐  首次写入(EAGAIN)    ┌──────────────┐
        │  ① 空态      │ ──────────────────▶ │ ② 积压态      │
        │  rb=nil      │                      │ ring 满→list  │
        │  零内存      │ ◀────────────────── │ 无界增长      │
        └─────────────┘   全发完 ModRead     └──────┬───────┘
                                ▲                   │ EPOLLOUT 唤醒
                                │                   ▼
                                │            ┌──────────────┐
                                └────────────│ ③ 发送态      │
                                  全发完      │ el.write 刷   │
                                             └──────────────┘
```

| 阶段 | 触发 | 物理状态 | 内存占用 | 源码 |
|---|---|---|---|---|
| **① 空态** | conn 创建（newStreamConn）| `ringBuffer.rb=nil`（懒加载）+ `listBuffer` 空，只设 `maxStaticBytes=64KB` 阈值 | **0** | connection_unix.go:68 `Reset` / elastic_ring_list_buffer.go:164 |
| **② 积压态** | `unix.Write` 返回 EAGAIN（内核 send buffer 满 = 设备收不动）| 首次 `ringBuffer.Write → instance() → rbPool.Get()` 分配 rb；持续写到 64KB 后 `listBuffer.PushBack` 追加链表 | rb 大小 → 64KB → **无界**（链表无 maxLen）| c.write:163 / c.open:125 / elastic_ring_list_buffer.go:85-98 / linkedlist.go:108 |
| **③ 发送态** | 内核 send buffer 有空位 → EPOLLOUT 唤醒 → el.write | `Peek(-1)` 取 ring+list 全部（`[][]byte`）→ writev 批量发 → `Discard(n)` 丢弃已发的（list node buf 归还 bsPool）| 递减 | eventloop_unix.go:322-331 |
| **释放** | conn 关闭（el.close 的 ④⑤ 步）| ④ 榨干残余（writev 尽量发完）→ ⑤ `Release`：`ringBuffer.Done`（rb 归还 pool）+ `listBuffer.Reset`（所有 buf 归还 bsPool）| **0**（全归还 pool）| eventloop_unix.go:370-381 / connection_unix.go:112 / elastic_ring_list_buffer.go:173 |

**三个关键状态转换**：

1. **空态 → 积压态**（EAGAIN 触发）：`c.write` / `c.open` 遇 EAGAIN → 数据进 outboundBuffer + `ModReadWrite` 注册 EPOLLOUT（AsyncWrite 路径）。**这是反压信号**——设备开始收不动。
2. **ringBuffer → listBuffer 升级**（64KB 触发）：`elastic.Buffer.Write` 检测 `ringBuffer.Buffered() >= maxStaticBytes` → 新数据 `PushBack` 进链表。**这是无界增长的起点**——从此 listBuffer 只增不减（直到 el.write 发出去 + Discard）。
3. **积压态 → 空态**（全发完 + ModRead）：`el.write` 把 outboundBuffer 刷空（`IsEmpty==true`）→ `ModRead` 撤销 EPOLLOUT。**这是反压恢复**。⚠️ 注意此时 `ringBuffer.rb` 仍保留（不归还 pool，等下次写复用），只有 conn 关闭 Release 才归还。

**正常路径根本不进 outboundBuffer**：AsyncWrite → c.write → unix.Write 成功 → 直接出网，0 接触 outboundBuffer。outboundBuffer 是「设备收不动」的**异常路径缓冲**，只在反压时启用。

##### outboundBuffer 的内存复用机制（生命周期里「内存归还到哪里」的答案）

生命周期里反复出现「归还 pool」「从 pool 取」，这节讲清这些 pool 是什么、彼此什么关系。

**结论先说**：outboundBuffer 涉及 **两个独立的 pool**（`rbPool` 和 `bsPool`），不是同一个。但它们**间接共用同一个底层字节池 `bsPool`**——ringBuffer 内部的 buf 数组和 listBuffer 每个 node 的 buf，最终都从 bsPool 分配。

###### 两个 pool 的分工

```
outboundBuffer (elastic.Buffer)
├─ ringBuffer (elastic.RingBuffer)
│    └─ rb *ring.Buffer  ──────── 从 rbPool 取/还（复用整个 ring.Buffer 对象）
│         └─ buf []byte  ──────── 从 bsPool 取/还（仅 grow / ring.New 时）
│
└─ listBuffer (linkedlist.Buffer)
     └─ 每个 node.buf []byte ──── 从 bsPool 取/还（PushBack 时）
```

| pool | 包 | 结构 | 复用粒度 | 谁用 |
|---|---|---|---|---|
| **rbPool** | `pkg/pool/ringbuffer` | 1 个 sync.Pool + 自适应校准（defaultSize/maxSize）| 整个 `ring.Buffer` 结构体（含 buf 数组）| elastic.RingBuffer（inboundBuffer + outboundBuffer.ringBuffer）的 instance()/done() |
| **bsPool** | `pkg/pool/byteslice` | **32 个 sync.Pool**（按 2 的幂分桶）| 裸 `[]byte` 字节切片 | linkedlist.PushBack（listBuffer 的 node buf）+ ring.Buffer.grow（扩容）|

###### 为什么是两个 pool 而不是一个

复用粒度不同，混在一起会丢失类型信息（sync.Pool 存 `any`），强制类型断言开销大：
- **ring.Buffer 是「带状态的复合对象」**（r/w 指针 + buf + isEmpty 等字段），整体复用省「分配结构体 + 分配 buf + 初始化」的多次开销
- **[]byte 是「无状态的原生内存」**，按大小分桶复用即可

分开后 rbPool 存 `*ring.Buffer`，bsPool 存 `*byte`（`unsafe.SliceData`），类型确定，零断言开销。

###### 两层嵌套的交织关系

「内存归还到哪里」在生命周期各阶段的流向：

```
阶段 ① 空态 → ② 首次写（ringBuffer 分配）
  elastic.RingBuffer.Write → instance() → rbPool.Get()
    ├─ rbPool 有复用 → 返回旧 ring.Buffer（buf 数组是之前用过的）
    └─ rbPool 没有 → ring.New(defaultSize) → bsPool.Get(defaultSize) 分配 buf
  来源: rbPool（优先）→ bsPool（rbPool 空时新建走这里）

阶段 ② ringBuffer 持续写，需要 grow（数据超当前容量）
  ring.Buffer.grow(newCap)
    ├─ newBuf = bsPool.Get(newCap)    从 bsPool 取更大的 buf
    ├─ 旧数据 Read 到 newBuf
    └─ bsPool.Put(rb.buf)             旧 buf 归还 bsPool
  来源: bsPool（grow 时 ring 的 buf 在 bsPool 进出）

阶段 ② ringBuffer 满（64KB），升级到链表
  listBuffer.PushBack(p)
    ├─ b = bsPool.Get(n)              从 bsPool 取 n 字节
    ├─ copy(b, p)
    └─ 链表追加 node{buf: b}
  来源: bsPool（每个链表节点的 buf）

阶段 ③ el.write 发送，Discard 释放
  listBuffer.Discard → bsPool.Put(node.buf)    链表 buf 归还 bsPool
  ringBuffer.Discard → 移动 r 指针（不释放 buf）

阶段 ④ conn 关闭，Release
  elastic.Buffer.Release
    ├─ ringBuffer.Done → rbPool.Put(rb)        ring.Buffer 归还 rbPool
    │    （rb 内部的 buf 不单独还 bsPool，整个 rb 带 buf 进 rbPool）
    └─ listBuffer.Reset → bsPool.Put(node.buf) 链表 buf 归还 bsPool
```

**关键**：bsPool 是 ringBuffer（grow/new 时）和 listBuffer（每个 node）**共同的底层字节池**。rbPool 只管 ring.Buffer 结构体对象的复用，结构体内部的 buf 在 grow/new 时进出 bsPool，但正常使用时（rbPool.Get/Put 整个对象）buf 跟着结构体一起在 rbPool 流转。

###### rbPool 的自适应校准（为什么不用调 ring 大小）

`rbPool.Put`（ringbuffer.go:89-101）每 42000 次触发一次 `calibrate`（:103-140）：
1. 统计这 42000 次里各 size 桶（2^6=64 到 2^25=32MB，共 20 桶）的调用次数
2. 按调用次数降序排序
3. `defaultSize` = 调用最频繁的桶的 size（新建 ring 的默认大小）
4. `maxSize` = 覆盖 95% 调用的最大 size（超过的不回收，直接丢弃防大 ring 滞留内存）

**含义**：rbPool 会**自动学习 mqtt-gateway 的 MQTT 包大小分布**。遥测包普遍 500 字节 → defaultSize 收敛到 512； maxSize 覆盖 95% 的包。**不用手动配置 ring 大小**。如果出现异常大包（固件 OTA 几 MB），maxSize 不覆盖它，大 ring 被丢弃等 GC（少数，GC 压力可控）。

###### bsPool 的分桶（按 2 的幂）

`bsPool.Get(size)`（byteslice.go:44-57）：请求 500 字节 → index=9（2^9=512）→ 从桶 9 取 512 容量的切片，截到 500。**实际分配 512 字节，浪费 12 字节**（按 2 的幂对齐，用 32 个桶覆盖所有大小，平衡桶数和浪费率）。

`Put` 用 `unsafe.SliceData(buf)` 存底层数组指针（8 字节）而非 slice 结构体（24 字节），减少 GC 扫描。

###### 对 mqtt-gateway 的四个含义

1. **内存复用全自动**：rbPool 自适应 + bsPool 分桶，MQTT 包的内存复用无需手动配置。遥测包 500 字节稳定落在 bsPool 桶 9（512），rbPool defaultSize 收敛到 512。

2. **⚠️ pool 在 GC 时清空**：Go 的 `sync.Pool` 每次 GC 会清空（poolCleanup）。GC 后 pool 缓存归零，下次 Get 全走 `make` 新建。**GC 越频繁，pool 复用率越低**。[gnet-feasibility-assessment.md](./gnet-feasibility-assessment.md) §4 的 GC 优化（GOGC 调整、对象逃逸分析）在这里直接提升 pool 复用率。

3. **⚠️ Peek 切片不能跨调用持有（pool 层面的原因）**：bsPool 的 Put 会把 buf 归还，下次 Get 复用给别的调用者。持有 Peek 返回的切片，而 Discard 触发 listBuffer 释放（bsPool.Put），这块内存可能被别的 conn 的 PushBack 复用覆盖。**跨 goroutine 用必须先 copy**（见第 6.2 节 MQTT 解析模式）。

4. **内存监控的实用层面**：通常不需要监控到 pool 粒度（rbPool 的 maxSize / bsPool 的桶占用）。10.6 的 `OutboundBuffered()` 监控（两道闸门）是更实用的层面。pool 层面只需关注：若 GC 后 HeapInuse 飙升（pool 清空导致重新分配），考虑调 GOGC 降低 GC 频率。

###### ⚠️ inboundBuffer「借了就还」vs outboundBuffer「借了不还」的不对称

这是个容易困惑的点：**两个 buffer 都是 `elastic.RingBuffer` 类型，但归还 rbPool 的策略完全相反**。源码证据如下。

`elastic.RingBuffer.Discard`（inboundBuffer 用，elastic_ring_buffer.go:88-99）：
```go
func (b *RingBuffer) Discard(n int) (int, error) {
    if b.rb == nil { return 0, nil }
    defer b.done()    // ⭐ 每次 Discard 都 defer done()：空了立刻归还 rbPool
    return b.rb.Discard(n)
}
```

`elastic.Buffer.Discard`（outboundBuffer 用，elastic_ring_list_buffer.go:70-81）：
```go
func (mb *Buffer) Discard(n int) (discarded int, err error) {
    discarded, err = mb.ringBuffer.Discard(n)   // ⚠️ 只 Discard，不调 done()！
    if n <= discarded { return }
    n -= discarded
    m, err = mb.listBuffer.Discard(n)
    discarded += m
    return
}
```

**不对称的根源**：inboundBuffer 的 `Discard/Read/WriteTo` 都 `defer b.done()`，outboundBuffer 的对应方法（`elastic.Buffer.Discard`/`Peek`）**不调 `done()`**。outboundBuffer 的 `Done()` 只在 `Release()`（conn 关闭，:174）时调一次。

###### 归还行为对照

| | inboundBuffer | outboundBuffer.ringBuffer |
|---|---|---|
| done() 调用 | 每次 Read/Discard/WriteTo 后 `defer done()` | **不调** |
| Done() 调用 | — | 只在 `Release()`（conn 关闭）|
| 全空后行为 | ✅ **立刻归还 rbPool**，rb=nil | ❌ **保留 rb**，rb 不变 |
| rb 的生命周期 | 一个包的读取周期（毫秒级）| conn 的整个生命周期（分钟~小时级）|
| 内存含义 | 空闲 conn 占 0 | 经历过反压的 conn 占 64KB（直到关闭）|

###### 为什么 outboundBuffer 选择「不还」（三个原因）

**原因 1：outboundBuffer 的 ringBuffer 是值类型字段，Discard 路径没接 done()**

`elastic.Buffer`（:29-33）的 `ringBuffer RingBuffer` 是值类型嵌入。`elastic.Buffer.Discard` 只调 `ringBuffer.Discard(n)`（移动读指针），不触发 `RingBuffer.done()`——因为 `done()` 是 `RingBuffer` 的私有方法，`elastic.Buffer` 没有在 Discard 后主动调用它。这是结构设计导致的：`elastic.Buffer` 把 ringBuffer 当「内部存储」用，只关心读写指针的推进，不关心 pool 归还。

**原因 2：outboundBuffer 会反复「积压→清空」，保留 rb 实现零分配复用**

outboundBuffer 的使用模式是反复的 EAGAIN → 进 ringBuffer → EPOLLOUT → el.write 刷出 → 又 EAGAIN → 又进 ringBuffer。如果每次清空都 `done()` 归还 rb，下次积压又要 `instance()` 从 pool 取，频繁 Get/Put 对 sync.Pool 造成压力，且每次 instance 可能拿到不同大小的 rb（pool 自适应），导致 ringBuffer 的 buf 大小不稳定。**保留 rb 让它在 conn 生命周期内稳定复用**：第一次拿到 64KB，之后所有积压都用这个 rb，零分配。

对比 inboundBuffer：使用模式是「读一个包→消费→空→下个包」，每次读完就空了，归还 pool 让别的 conn 复用更划算（conn 多 rb 少，pool 复用率高）。

**原因 3：outboundBuffer 的 rb 大小固定（maxStaticBytes=64KB），长期持有成本明确**

outboundBuffer 的 ringBuffer **永远不会超过 64KB**（超过就升级到 listBuffer）。所以这个 rb 是个「固定 64KB 的长期资产」，长期持有的成本明确。而 inboundBuffer 的 rb 会 grow 到任意大小（可能几 MB），长期持有会浪费。

###### 对 mqtt-gateway 的两个含义

1. **⚠️ 内存预算要算「反压历史包袱」**：[gnet-feasibility-assessment.md](./gnet-feasibility-assessment.md) §3 的内存测算要补一项——经历过反压但已恢复的 conn 仍占 64KB rb。假设 50 万连接中 10% 经历过反压（5 万 conn），每个 64KB = **3.2GB**。这不是「当前积压」（那是 OutboundBuffered），而是「曾经积压过的历史包袱」。设备网络普遍较差时（偏远地区 4G）这个比例会高。

2. **防线 2 间接清理 rb**：10.6 的防线 2（OnTick 踢慢连接）不仅防 OOM，还间接清理「反压历史包袱」——被踢的 conn 会 `Release` 归还 rb。设备健康时反压比例低，被踢的慢连接正好是反压的主要来源，所以防线 2 的踢连接和 rb 归还是自然对齐的。实务上不用单独管 rb 归还。

###### 为什么 Go 的 mcache 对小对象更高效，bsPool 的小桶却保留

补充一个相关权衡（顺带回答「bsPool 为什么有 1/2/4 字节小桶」）：

Go runtime 的 mcache 把小对象（1~32KB）按 size class 分配，P 本地无锁分配 ~10ns。对 ≤16 字节对象，mcache 通常不比 sync.Pool 慢，甚至更快（sync.Pool 的 Get/Put 有 procPin/接口装箱开销）。

bsPool 仍保留小桶（index 0-3，容量 1/2/4/8）的原因是**统一代码路径换简单性**——加「size < threshold 走 make，否则走 pool」的分支会引入分支预测失败 + 代码复杂度。但 mqtt-gateway 不会触发这些小桶的热路径：MQTT 包都是几十字节起（PUBLISH 几百字节，CONNECT 几十字节），linkedlist.PushBack 传入的 `n = len(p)` 至少几十，落到桶 6+（64 字节）。1-8 字节的桶在实际运行中基本是空的（Get 走 make，Put 进去很快被 GC 清掉）。

**结论**：bsPool 的小桶是「统一路径」的微小代价，mqtt-gateway 不触发，可忽略。若 pprof 见到 `byteslice.(*Pool).Get` 对小对象的热点才有必要改进。

##### 为什么 QoS1 PUBACK 场景尤其要做

正常情况下服务端写给设备的只有 PUBACK（4 字节），看起来不会积压。但积压的后果是**协议层雪崩**：
- PUBACK 迟迟不到 → 设备认为消息没被确认 → **触发 QoS1 重发（DUP=1）** → 上行流量放大 → 服务端更慢 → PUBACK 更滞后 → 恶性循环

所以即使 PUBACK 很小，积压也会引发协议层放大效应。**QoS1 场景两道闸门都建议做**。

##### 防线 1（推荐，精确及时）：写前检查——每次 AsyncWrite 前判断

这是你 server.go「写失败立即断连」语义的等价物。在调 `AsyncWrite` 前检查当前积压量，超阈值就直接断连，不让新数据继续进 buffer：

```go
// durable ack 到达，准备回 PUBACK
func onDurableAck(c gnet.Conn, pktID uint16, cfg Config) {
    // ⭐ 写之前检查：已经积压很多 = 设备收不动，直接断连让设备重连
    if c.OutboundBuffered() > cfg.MaxOutboundBytes {   // 建议 256KB~1MB
        _ = c.Close()
        return   // 不发 PUBACK，让设备重连后重新 CONNECT + 重发 PUBLISH
    }
    puback := mqtt.EncodePubAck(nil, pktID)
    _ = c.AsyncWrite(puback, nil)
}
```

**为什么这是第一道防线**：
- **精确**：在「准备写」那一刻检查，比周期扫描及时
- **语义正确**：与其让 PUBACK 堆在 buffer 里最终超时，不如早点断连，让设备重连重发
- **零成本**：`OutboundBuffered()`（connection_unix.go:457）只读 `elastic.Buffered()`，O(1)
- **等价于 server.go 的同步写失败语义**：你原来靠 `conn.Write` 返回 error 触发断连，现在靠写前检查触发

> 注意：这个检查应该放在所有「主动写设备」的入口（PUBACK 回写、未来的下行命令），不只是 QoS1。建议封装成一个 `safeAsyncWrite(c, buf, cfg)` 辅助函数。

##### 防线 2（兜底，防 OOM）：OnTick 周期扫描——粗粒度兜底

复用 10.7 的 connRegistry 扫描循环，叠加 outboundBuffer 检查。防线 1 漏过的（比如阈值设得宽松、或下行命令场景突发积压）由它兜底：

```go
// 在 10.7 的 OnTick 里，除了保活判死，再叠加 outboundBuffer 检查
func (h *handler) OnTick() (time.Duration, gnet.Action) {
    now := time.Now().UnixNano()
    h.registry.mu.RLock()
    for _, s := range h.registry.conns {
        last := atomic.LoadInt64(&s.lastActive)
        if now-last > int64(keepAliveTimeout(s.keepAlive)) {
            _ = s.c.Close()   // 保活判死
            continue
        }
        // ⭐ 兜底：outboundBuffer 超阈值也踢（阈值要比防线 1 宽松）
        if buffered := s.c.OutboundBuffered(); buffered > h.cfg.MaxOutboundBytesTick {
            h.log.Warn("outbound buffer overflow, closing slow conn",
                "device", s.deviceID, "buffered", buffered)
            _ = s.c.Close()
        }
    }
    h.registry.mu.RUnlock()
    return 10 * time.Second, gnet.None
}
```

> ⚠️ 不要用 `eng.eventLoops.iterate` / `el.connections.iterate`——这些是 gnet 包内私有 API，业务层拿不到。必须用 10.7 的 connRegistry 方案。

##### 两道防线的关系

```
设备收不动（TCP 窗口满 → unix.Write EAGAIN → outboundBuffer 增长）
  │
  ├─ 防线 1（写前检查，精确）：每次 AsyncWrite 前判 OutboundBuffered()
  │     ├─ 超阈值（256KB~1MB）→ 立即 c.Close()，不让新数据进 buffer
  │     └─ 未超 → c.AsyncWrite，数据进 buffer
  │              ↓
  │              buffer 持续增长（防线 1 阈值宽松 / 突发流量）
  │              ↓
  └─ 防线 2（OnTick 兜底，粗粒度）：每 10 秒扫所有 conn
        └─ 超阈值（2MB~10MB）→ c.Close() 踢掉慢连接，防 OOM
```

**防线 1 是第一道，防线 2 是兜底，两道都做最稳。**

##### 阈值怎么定

阈值要和 outboundBuffer 的生命周期阶段对应（见上面「生命周期」子节）：

| 阈值 | 建议值 | 对应生命周期阶段 | 理由 |
|---|---|---|---|
| 防线 1（写前检查 `MaxOutboundBytes`）| **64KB ~ 256KB**（略大于 ringBuffer 的 64KB 上限）| 阻止从「② 积压态 ring 满」升级到「链表无界」| **最佳阈值设在 ring 即将满附近**（~60KB），能在升级到链表前拦截，这样 outboundBuffer 永远不进 listBuffer，单 conn 内存有 64KB 硬上限 |
| 防线 2（OnTick 兜底 `MaxOutboundBytesTick`）| **2MB ~ 10MB** | 已经在「② 积压态 listBuffer 无界增长」| 兜底防 OOM，比防线 1 宽松，避免误杀短暂拥塞 |

**阈值设计的关键洞察**：防线 1 如果设在 ringBuffer 上限（64KB）以内，能让 outboundBuffer **永远停留在 ring 而不升级到链表**——单 conn 内存占用有 64KB 硬上限，50 万连接最坏 32GB（可控）。一旦越过 64KB 进链表，就没有上限了，只能靠防线 2 兜底踢连接。

结合容量规划（[gnet-feasibility-assessment.md](./gnet-feasibility-assessment.md) §3）：若单节点 50 万连接、防线 1 设 64KB，最坏全部积压 = 32GB；防线 2 设 2MB 是针对「防线 1 失效后的极端情况」，最坏全部积压到 2MB 会 OOM，所以防线 2 是「踢连接保命」而非「保证不 OOM」。

#### 陷阱 4：callback 在 reactor goroutine 里执行，不能阻塞

`c.asyncWrite` 的 defer 调 `hook.callback(c, err)`（connection_unix.go:254-258），这个 callback 在 **reactor 的 goroutine** 里同步执行。如果 callback 里做重活（DB 查询、等锁、sleep），会阻塞整个 reactor——和 10.5 的 recover 一样，属于「在 reactor 线程上跑的代码必须非阻塞」红线。

**正例**：callback 只做轻量操作（日志、metrics、状态字段更新）。重活丢 worker pool。

#### 陷阱 5：并发 AsyncWrite 的顺序保证

`c.write` 的 146-148 行：outboundBuffer 非空时，新数据一律追加尾部，不直接 `unix.Write`。这保证了**同一个 conn 的多次 AsyncWrite 按调用顺序出网**。

**但注意**：多个 goroutine 并发对同一个 conn 调 AsyncWrite，**入队的顺序**不是调用顺序——入队是并发竞争的（LockFreeQueue）。如果你有「命令1 必须在命令2 之前发给同一设备」的依赖，需要业务层自己保证调用顺序（比如用一个 goroutine 串行化下行）。

不同 conn 之间的顺序完全不保证（各自走自己的 reactor + outboundBuffer）。

---

### 10.7 ⚠️ OnTick + 保活判死的完整实现（改造 3 落地）

**核心结论**：gnet 只提供「周期性触发 OnTick」这个时钟，**保活判死逻辑（判超时 + 找超时连接 + 关连接）全部业务层自己实现**。原因有二：

1. **v2.4.1 不支持 per-conn 读超时**（`SetReadDeadline` 返回 `ErrUnsupportedOp`，connection_unix.go:555），你 server.go 的 `conn.SetReadDeadline(1.5*keepalive)` 那套机制完全失效。
2. **reactor 不会主动醒来检查时间**——Polling 平时阻塞在 EpollWait，只在「有事件」时醒来。必须用 OnTick 这个外部时钟强制叫醒它。

#### 10.7.1 OnTick 怎么用

OnTick 是 `EventHandler` 接口的方法（gnet.go:501），返回 `(delay time.Duration, action Action)`：
- `delay`：**下次触发 OnTick 的间隔**。返回 0 = 不再触发。
- `action`：`None`/`Close`（无 conn 参数，无意义）/ `Shutdown`（关停 engine）。

**两个必须做的配置**：

```go
// ① 必须覆写 OnTick（嵌入 BuiltinEventEngine 的默认实现返回 0，只调一次就不再触发）
type myHandler struct{ gnet.BuiltinEventEngine }
func (h *myHandler) OnTick() (time.Duration, gnet.Action) { ... }

// ② 必须开 WithTicker(true)，否则 ticker goroutine 不启动（engine_unix.go:180）
gnet.Run(handler, "tcp://:1883", gnet.WithTicker(true), ...)
```

不开 `WithTicker(true)`，即使你写了 OnTick 方法也永远不会被调用。

#### 10.7.2 OnTick 跑在哪、能访问什么

**OnTick 跑在 ingress reactor**（默认模式，engine_unix.go:181；ReusePort 模式跑在 eventloop 0，engine_unix.go:129）。

**OnTick 里能安全做的事**：
- 读业务层自己维护的全局状态（加锁或 atomic）
- 调 `c.Close()` / `c.AsyncWrite()` / `c.Wake()`（都并发安全，内部 Trigger 到 sub reactor）

**OnTick 里不能做的事**：
- 直接访问 sub reactor 的 `connections`（conn_map）——线程不安全
- 调 `c.Read/Peek/Write/SetContext`——这些只在 conn 所属 reactor 的回调里能调

#### 10.7.3 ⚠️ 一个必须澄清的误区

> 「OnTick 里用 `eng.eventLoops.iterate` 遍历 sub reactor，用 `el.connections.iterate` 遍历连接，然后 `Trigger` 检查超时」——**这是错的**。

原因：`Engine` 接口（gnet.go:54-172）只暴露 `Validate/CountConnections/Register/Dup/DupListener/Stop` 六个方法。`eventLoops.iterate` 和 `el.connections.iterate` 都是 **gnet 包内私有 API**（`CountConnections` 内部用了，gnet.go:76，但没暴露给业务）。业务从 handler 外部**拿不到「所有 conn」的列表**。

`c.EventLoop().Execute(...)` 也救不了——它针对单个 conn 的 EventLoop，需要先有 conn 引用，但 OnTick 在 ingress 没有 sub reactor 的 conn 列表。

**所以必须业务层自己维护连接索引**（connRegistry）。

#### 10.7.4 方案 1（推荐起步）：全局 connRegistry + OnTick 扫描

**核心思路**：OnOpen 把 session 登记进全局表，OnClose 移除，OnTick 周期扫描找出超时的 conn 调 `c.Close()`。

```go
package server

import (
    "errors"
    "log/slog"
    "sync"
    "sync/atomic"
    "time"

    "github.com/panjf2000/gnet/v2"
)

// session：per-conn 状态。红线 #2：pointer-poor，别塞 map/channel/timer/logger。
type session struct {
    tenantID   int64
    deviceID   int64
    connID     uint64
    sessionVer int64
    lastActive int64      // ⭐ unix nano，跨 reactor 用 atomic 访问
    keepAlive  uint16     // 来自 CONNECT 包（秒）；0 = 用兜底窗口
    c          gnet.Conn  // ⭐ 持有 conn 引用，供 OnTick 里 Close
}

// connRegistry：业务层自维护的连接索引。所有 reactor 都会读写，必须加锁。
type connRegistry struct {
    mu    sync.RWMutex
    conns map[uint64]*session   // key = connID
}

type handler struct {
    gnet.BuiltinEventEngine
    eng      gnet.Engine
    registry *connRegistry
    connSeq  atomic.Uint64      // 本地 conn ID 序列（presence fencing 用）
    log      *slog.Logger
}

// OnBoot：保存 engine 引用
func (h *handler) OnBoot(eng gnet.Engine) gnet.Action {
    h.eng = eng
    return gnet.None
}

// OnOpen：登记。注意此时 CONNECT 还没解析，keepAlive 未知，留空。
// 在第一次 OnTraffic 解析出 CONNECT 后再填 keepAlive。
func (h *handler) OnOpen(c gnet.Conn) (out []byte, action gnet.Action) {
    s := &session{
        connID:     h.connSeq.Add(1),
        lastActive: time.Now().UnixNano(),
        c:          c,
    }
    c.SetContext(s)
    h.registry.mu.Lock()
    h.registry.conns[s.connID] = s
    h.registry.mu.Unlock()
    return nil, gnet.None
}

// OnTraffic：每次收包更新活跃时间
func (h *handler) OnTraffic(c gnet.Conn) (action gnet.Action) {
    defer func() {
        if r := recover(); r != nil {
            h.log.Error("OnTraffic panic", "err", r)
            action = gnet.Close
        }
    }()
    s := c.Context().(*session)
    atomic.StoreInt64(&s.lastActive, time.Now().UnixNano())   // ⭐ 每次收包都更新

    // 解析 MQTT；CONNECT 包解析后填 s.keepAlive（CONNECT 之后才知道）
    // ... 你的 MQTT 解析 + ACL + raw log 投递
    return gnet.None
}

// OnClose：移除登记
func (h *handler) OnClose(c gnet.Conn, err error) (action gnet.Action) {
    s := c.Context().(*session)
    h.registry.mu.Lock()
    delete(h.registry.conns, s.connID)
    h.registry.mu.Unlock()
    // deletePresence fencing 删除（你原来的逻辑，server.go:312）
    return gnet.None
}

// OnTick：扫描超时（跑在 ingress reactor）
func (h *handler) OnTick() (time.Duration, gnet.Action) {
    now := time.Now().UnixNano()
    h.registry.mu.RLock()
    for _, s := range h.registry.conns {
        idle := keepAliveTimeout(s.keepAlive)
        last := atomic.LoadInt64(&s.lastActive)
        if now-last > int64(idle) {
            // ⭐ c.Close() 并发安全（connection_unix.go:540，内部 Trigger 到 sub reactor）
            _ = s.c.Close()
        }
    }
    h.registry.mu.RUnlock()
    return 10 * time.Second, gnet.None   // 每 10 秒扫一次
}

// keepAliveTimeout：沿用你 server.go:393 的逻辑
func keepAliveTimeout(keepAlive uint16) time.Duration {
    if keepAlive == 0 {
        return 5 * time.Minute   // maxIdleWhenKeepAliveZero
    }
    return time.Duration(keepAlive) * time.Second * 3 / 2
}
```

**关键点说明**：

| 点 | 说明 |
|---|---|
| `c.Close()` 并发安全 | connection_unix.go:540 内部 `Trigger(LowPriority, ...)` 投递到 conn 所属 sub reactor 执行真正关闭，**可以从 ingress reactor 的 OnTick 里安全调用** |
| `lastActive` 用 atomic | OnTraffic（sub reactor）写，OnTick（ingress reactor）读，跨 reactor 必须原子访问 |
| `registry.mu` 全局锁 | 所有 sub reactor 的 OnOpen/OnClose 都写，OnTick 读。锁竞争是缺点（见 10.7.5）|
| OnTick 间隔 10 秒 | 取舍：太短 CPU 浪费，太长判死慢。MQTT keepalive 通常 60-600 秒，10 秒检查粒度足够（最坏滞后 10 秒判死，可接受）|
| keepAlive 初值 0 | OnOpen 时 CONNECT 还没解析，先用 0（=5 分钟兜底）；CONNECT 解析后填实际值，下次扫描生效 |

#### 10.7.5 方案 2（大规模）：分桶锁降低锁竞争

方案 1 的全局锁在高并发建断连时会成瓶颈。按 `connID % N` 分成 N 个桶（比如 16），每个桶一把锁。OnTick 一次只锁一个桶，分摊竞争：

```go
const shardCount = 16

type shardedRegistry struct {
    shards [shardCount]struct {
        mu    sync.RWMutex
        conns map[uint64]*session
    }
}

func (r *shardedRegistry) shard(connID uint64) int {
    return int(connID % shardCount)
}

// OnOpen：只锁一个桶
func (h *handler) OnOpen(c gnet.Conn) (out []byte, action gnet.Action) {
    s := &session{connID: h.connSeq.Add(1), lastActive: time.Now().UnixNano(), c: c}
    c.SetContext(s)
    sh := h.registry.shard(s.connID)
    h.registry.shards[sh].mu.Lock()
    h.registry.shards[sh].conns[s.connID] = s
    h.registry.shards[sh].mu.Unlock()
    return nil, gnet.None
}

// OnTick：每次只扫一个桶，轮流来
func (h *handler) OnTick() (time.Duration, gnet.Action) {
    now := time.Now().UnixNano()
    sh := int(h.tickRound % shardCount)   // h.tickRound 是 handler 里的 uint64 字段，每次自增
    h.tickRound++
    h.registry.shards[sh].mu.RLock()
    for _, s := range h.registry.shards[sh].conns {
        if now-atomic.LoadInt64(&s.lastActive) > int64(keepAliveTimeout(s.keepAlive)) {
            _ = s.c.Close()
        }
    }
    h.registry.shards[sh].mu.RUnlock()
    return 1 * time.Second, gnet.None   // 1 秒触发一次，16 秒扫完全部桶（=一个完整判死周期）
}
```

**取舍**：每秒只锁 1/16 的连接，锁竞争大幅降低。代价是「最坏判死延迟」从方案 1 的 10 秒变成 16 秒（一个完整扫描周期）。**建议先用方案 1，压测发现锁瓶颈再升级方案 2**。

#### 10.7.6 三种方案对比

| 方案 | 复杂度 | 锁竞争 | 最坏判死延迟 | 适合场景 |
|---|---|---|---|---|
| 你现在的 server.go（SetReadDeadline）| 最简单 | 无（per-conn 超时）| 即时（per-conn timer）| **gnet 不支持，不可用** |
| 方案 1（全局表 + OnTick 扫描）| 中等 | 全局锁 | ~10 秒 | 中小规模（< 10 万连接），推荐起步 |
| 方案 2（分桶锁 + OnTick 扫描）| 较高 | 分桶锁 | ~16 秒 | 大规模（> 10 万），压测后发现方案 1 锁瓶颈 |

#### 10.7.7 注意事项

1. **保活判死只是 OnTick 的用途之一**。你还可以在同一个 OnTick 里叠加：outboundBuffer 积压检查（10.6 陷阱 3）、metrics 采集、graceful shutdown 检查等。
2. **OnTick 不能阻塞**。扫描大量连接要快，别在 OnTick 里做 DB 查询、RPC。如果某个 conn 的关闭逻辑重（比如 presence delete），`c.Close()` 投递后，真正的 `OnClose` 回调在 sub reactor 里执行，OnTick 本身只做「标记 + 投递」。
3. **connID 用 numeric**。registry 的 key 用 `uint64`（connID），别用 string（clientID），呼应红线 #2。
4. **goroutine 数仍满足红线**。OnTick 是 ingress 的一个 ticker goroutine，不随连接数增长。connRegistry 的锁是数据竞争点，但不增加 goroutine 数。

### 10.8 ⚠️ CONNECT 认证放 OnTraffic，不放 OnOpen

这是迁移最容易放错位置的一点。**结论：CONNECT 解析 + 认证 + CONNACK 全部放 OnTraffic，OnOpen 只做轻量登记**。原因和完整实现如下。

#### 10.8.1 为什么不能放 OnOpen

你 server.go 现在的逻辑（server.go:214-253）：accept 后立刻读 CONNECT → 认证 → presence 写 → CONNACK。**认证发生在「连接刚建立」时，读第一个包之前**。直觉上这对应 gnet 的 OnOpen，但放进去有三个致命问题：

**问题 1：OnOpen 时 conn 还没数据**

`register0`（eventloop_unix.go:219）刚把 fd 加进 epoll，设备的 CONNECT 包可能还在路上。OnOpen 是「连接建立」回调，不是「第一个包到达」回调——此时 `c.Peek()` 大概率返回 `io.ErrShortBuffer`（半包/空包）：

```go
// ❌ OnOpen 里读 CONNECT，数据还没到
func (h *handler) OnOpen(c gnet.Conn) ([]byte, gnet.Action) {
    pkt, _ := c.Peek(2)   // 大概率 ErrShortBuffer！fd 刚进 epoll，CONNECT 还在网络路上
    ...
}
```

`OnOpen` → `OnTraffic` 之间有一次 epoll 往返（等 EPOLLIN），CONNECT 包通常在这个间隙到达。所以 OnOpen 里读必然踩空。

**问题 2：OnOpen 里阻塞会拖垮 reactor**

认证（auth queue）+ presence 写（Redis）是慢操作。OnOpen 在 sub reactor 的 goroutine 里同步执行（register0 → open → OnOpen，eventloop_unix.go:228-234），在这里同步等 auth+presence 会阻塞整个 sub reactor，饿死同 reactor 的其他几万个 conn。这直接违反「reactor 不能阻塞」红线。

**问题 3：OnOpen 的 out 参数语义不对**

OnOpen 的返回值 `out []byte` 是「连接建立后立刻无条件发给设备的数据」（eventloop_unix.go:234-239，`c.open(out)` 同步发）。但 CONNACK 是**条件发送**：
- 认证成功 → 发 CONNACK Accepted
- 认证失败 → 发 CONNACK 拒绝码 + 关连接
- presence 写失败 → 发 CONNACK ServerUnavailable + 关连接（红线 #6）

这种「取决于认证结果」的条件发送，和 out 的「无脑发」语义不匹配。

#### 10.8.2 正确做法：状态机 + OnTraffic 分流

**核心思路**：OnOpen 只做轻量登记（初始化 session、设状态为「等 CONNECT」），CONNECT 的解析/认证/CONNACK 全放 OnTraffic，靠 session 状态机区分「首包」和「后续包」。

##### session 状态机

```go
type connState int8
const (
    stateWaitingConnect connState = iota   // 初始：等 CONNECT 首包
    stateConnected                         // CONNECT 认证通过，正常处理
)

type session struct {
    // ... 其他字段（见 10.7.4）
    state      connState   // ⭐ 状态机
    keepAlive  uint16      // CONNECT 解析后填
}
```

只有两个状态，满足红线 #2（pointer-poor，一个 int8）。

##### OnOpen：只做登记

```go
func (h *handler) OnOpen(c gnet.Conn) ([]byte, gnet.Action) {
    // bounded accept（改造 2）：连接数超限就拒
    if h.connCount.Add(1) > h.cfg.MaxConnections {
        h.connCount.Add(-1)
        return nil, gnet.Close
    }

    // ⭐ 只做轻量登记，不碰协议
    s := &session{
        connID:     h.connSeq.Add(1),
        lastActive: time.Now().UnixNano(),
        state:      stateWaitingConnect,   // 初始状态
        c:          c,
    }
    c.SetContext(s)

    // 登记 connRegistry（10.7 保活判死要用）
    h.registry.mu.Lock()
    h.registry.conns[s.connID] = s
    h.registry.mu.Unlock()

    return nil, gnet.None   // ⭐ 不发任何东西，等 OnTraffic
}
```

##### OnTraffic：按状态分流

```go
func (h *handler) OnTraffic(c gnet.Conn) (action gnet.Action) {
    defer func() {
        if r := recover(); r != nil {     // 10.5 recover
            h.log.Error("OnTraffic panic", "err", r)
            action = gnet.Close
        }
    }()
    s := c.Context().(*session)
    atomic.StoreInt64(&s.lastActive, time.Now().UnixNano())   // 10.7 保活

    switch s.state {
    case stateWaitingConnect:
        return h.handleConnect(c, s)   // ⭐ 首包当 CONNECT 处理
    case stateConnected:
        return h.handleData(c, s)      // 后续包正常处理（PUBLISH/PINGREQ/DISCONNECT）
    default:
        return gnet.Close
    }
}
```

##### handleConnect：CONNECT 解析 + 异步认证 + CONNACK

```go
func (h *handler) handleConnect(c gnet.Conn, s *session) gnet.Action {
    // 1. Peek 固定头部，判断是不是 CONNECT（改造 1：流式解析）
    hdr, err := c.Peek(2)
    if err != nil {
        return gnet.None   // 半包，等下次 OnTraffic
    }
    if hdr[0]>>4 != mqtt.PktConnect {
        return gnet.Close   // 首包不是 CONNECT，断连
    }

    // 2. 算整包长度，判断是否到齐
    totalLen := decodeRemainingLength(hdr)   // 伪代码，参考第 6.2 节
    if c.InboundBuffered() < totalLen {
        return gnet.None   // CONNECT 包没到齐，等下次
    }

    // 3. 解析 CONNECT
    full, _ := c.Peek(totalLen)
    cp, perr := mqtt.ParseConnect(full, h.cfg.Limits)
    c.Discard(totalLen)
    if perr != nil {
        c.AsyncWrite(mqtt.EncodeConnAck(nil, false, connAckForParse(perr)), nil)
        return gnet.Close
    }

    // 4. ⭐ 异步认证 + presence（不阻塞 reactor！呼应改造 4）
    //    投递给 auth queue，回调里用 AsyncWrite 发 CONNACK
    h.authQueue.Submit(auth.CredentialsFromConnect(cp), func(res auth.Result) {
        if res.Err != nil {
            c.AsyncWrite(mqtt.EncodeConnAck(nil, false, mqtt.ConnAckServerUnavailable), nil)
            c.Close()
            return
        }
        // presence 写（红线 #6：CONNECT 成功 = 认证 + presence 都成功）
        presRec, ok := h.writePresence(res.TenantID, res.DeviceID)
        if !ok {
            c.AsyncWrite(mqtt.EncodeConnAck(nil, false, mqtt.ConnAckServerUnavailable), nil)
            c.Close()
            return
        }
        // 认证 + presence 都成功 → 发 CONNACK Accepted，切到 Connected 态
        s.tenantID = res.TenantID
        s.deviceID = res.DeviceID
        s.connID = presRec.ConnID
        s.sessionVer = presRec.SessionVersion
        s.keepAlive = cp.KeepAlive
        s.state = stateConnected                 // ⭐ 状态切换
        c.AsyncWrite(mqtt.EncodeConnAck(nil, false, mqtt.ConnAckAccepted), nil)
    })
    return gnet.None   // 投递完不等，立刻让出 reactor
}
```

#### 10.8.3 关键设计点

| 点 | 说明 |
|---|---|
| **状态机只有两态** | `stateWaitingConnect` / `stateConnected`，一个 int8，满足红线 #2 |
| **CONNECT 半包天然处理** | `Peek(2)` + `InboundBuffered()` 判断，没到齐就 return None 等下次（改造 1）|
| **认证异步化** | 投递给 auth queue 不阻塞 reactor，回调里 AsyncWrite 发 CONNACK（改造 4）|
| **CONNACK 条件发送** | 认证失败/presence 失败发不同码 + Close，成功才发 Accepted（红线 #6）|
| **状态切换在回调里** | `s.state = stateConnected` 在 auth 回调里执行。注意：回调在 worker goroutine，写 `s.state` 要保证此时 conn 还没被关——因为 auth 在途时 conn 可能已被保活判死踢掉 |

#### 10.8.4 ⚠️ 隐蔽竞态：auth 在途时 conn 被踢

上面的代码有个竞态：auth 回调里写 `s.state = stateConnected`，但如果 auth 耗时较长，期间这个 conn 可能被 OnTick 保活判死踢掉（`c.Close()`），或者设备主动断连。此时：

- `c.Close()` 已经触发，`OnClose` 可能已经执行（移除 registry、deletePresence）
- auth 回调却在写 `s.state`、调 `c.AsyncWrite(CONNACK)`

**后果**：CONNACK 发给一个已经关闭的 conn（AsyncWrite 对已关 conn 返回 error，无害但浪费），或者 `s.state` 被改成 Connected 但 conn 已经没了（脏写）。

**对策**：auth 回调里用 **fencing 校验**（呼应红线 #8），确认 conn 仍然有效再操作：

```go
h.authQueue.Submit(creds, func(res auth.Result) {
    // ⭐ fencing：如果 conn 已被关（Close 触发后会清 s.c 或置标志），直接放弃
    if !s.alive.Load() {   // s.alive 是 atomic.Bool，OnClose 时置 false
        return
    }
    // ... 认证 + presence + CONNACK
})
```

在 OnClose 里：
```go
func (h *handler) OnClose(c gnet.Conn, err error) gnet.Action {
    s := c.Context().(*session)
    s.alive.Store(false)   // ⭐ 通知所有在途回调：conn 已死
    // ... deletePresence fencing + 移除 registry
    return gnet.None
}
```

这样 auth 回调看到 `s.alive==false` 就直接 return，不会对已死 conn 操作。这和你 server.go 现有的 `ConnID + SessionVersion` fencing（红线 #8）是同一个思路——**任何跨回调的操作都要先校验 conn 仍然有效**。

#### 10.8.5 OnOpen / OnTraffic 职责对照

| | OnOpen（轻量登记）| OnTraffic（协议处理）|
|---|---|---|
| bounded accept 检查 | ✅ `connCount` 超限返回 Close | ❌ |
| session 初始化 | ✅ SetContext + connRegistry 登记 | ❌ |
| CONNECT 解析 | ❌ 数据还没到 | ✅ Peek + ParseConnect |
| 认证 | ❌ 慢操作 | ✅ 异步投递 auth queue |
| presence 写 | ❌ 慢操作 | ✅ auth 回调里写 |
| CONNACK 发送 | ❌ 条件发送，out 语义不符 | ✅ AsyncWrite 条件发 |
| 状态机切换 | ❌ | ✅ `state = stateConnected` |
| PUBLISH/PINGREQ 处理 | ❌ | ✅ state==Connected 时 |
| 保活 lastActive 更新 | ❌ | ✅ 每次都更新 |
| 返回值 | `(nil, None/Close)` | `None/Close` |

---

### 10.9 为什么选 LT（水平触发），不开 `WithEdgeTriggeredIO`

**结论**：mqtt-gateway 用默认 **LT 模式**（不配 `WithEdgeTriggeredIO(true)`）。因为 **ET 的收益在 mqtt-gateway 场景用不上，ET 的风险在控制指令通道不可接受**。

#### 10.9.1 LT 和 ET 到底差在哪

epoll 的两种触发模式（详见第 5 站 5.2-5.3）：

| | LT（水平触发，默认）| ET（边沿触发）|
|---|---|---|
| 通知时机 | 只要 fd 缓冲区**非空**就一直通知 | 只在「空→非空」**变化那一刻**通知一次 |
| 不读完的后果 | 无（下次 epoll 还会通知）| **丢数据**（剩下的永远卡着，直到新数据到来触发下次通知）|
| 代码要求 | 读一次可 return | **必须循环读到 EAGAIN** |

gnet 源码里两种模式分别在哪用：

| fd 类型 | 模式 | 出处 |
|---|---|---|
| **listener fd** | **强制 ET** | engine_unix.go:170 `AddRead(..., true)` |
| **conn fd** | 默认 LT，可选 ET | register0（eventloop_unix.go:215-218），由 `Options.EdgeTriggeredIO` 控制 |

**listener 强制 ET 是对的**——accept0（acceptor_unix.go:31）有 `for` 循环批量 drain accept 队列，符合 ET「必须一次取完」的要求，连接风暴时一次唤醒处理几十个连接，收益明确。

**conn fd 用 LT 是 mqtt-gateway 的选择**，下面论证为什么。

#### 10.9.2 ET 的理论收益——对 mqtt-gateway 用不上

ET 的核心收益是「减少 epoll 唤醒次数」。这个收益只在**大流量、单次数据大**时明显：

```
场景：一个 conn 一次发 10MB（HTTP 大文件下载、视频流）

LT 模式：epoll 唤醒 ~160 次（10MB/64KB）才能读完
ET 模式：epoll 唤醒 1 次，for 循环一次读完
→ ET 省 159 次 epoll_wait，收益明显（Nginx 用 ET 就是因为这个）
```

**但 mqtt-gateway 的真实流量不是这样**：

| MQTT 报文 | 典型大小 |
|---|---|
| CONNECT | 几十~几百字节 |
| CONNACK / PUBACK / PINGREQ / PINGRESP / DISCONNECT | 2~4 字节（固定）|
| PUBLISH QoS0/QoS1 | 几十字节~几 KB（逆变器遥测：温度/功率）|

`el.buffer` 默认 64KB（建议配 8KB）。**一个 MQTT 包通常 < 8KB，一次 `unix.Read` 就读完**：

```
mqtt-gateway 的 LT 模式：
  epoll 唤醒 1 → read 2KB（整个 PUBLISH）→ OnTraffic → return
  → 1 次唤醒处理完一个包

ET 模式：
  epoll 唤醒 1 → read 2KB → goto loop → read EAGAIN → return
  → 同样 1 次唤醒，但多一次 read（撞 EAGAIN 的空转）
```

**结论：包小时 LT 和 ET 的唤醒次数几乎一样，ET 省下的唤醒次数 ≈ 0，收益用不上。**

#### 10.9.3 ET 的风险——对 mqtt-gateway 不可接受

ET 的代价是「必须循环读到 EAGAIN，否则丢数据」。三个具体风险：

**风险 1：OnTraffic 消费不完整会卡住**

ET 模式下，如果 OnTraffic 里 Peek 了但忘 Discard（见第 6.2 节 MQTT 解析模式），数据永远在 buffer 里。**ET 的 epoll 不会再唤醒**（没新数据到达），这个包卡住，设备等 PUBACK 超时重发。LT 模式下 epoll 会反复唤醒（buffer 非空），至少能让你发现 bug。

**风险 2：ET 二次触发的延迟不确定**

el.read 的 ET 分支（eventloop_unix.go:295-297）：

```go
if isET && n == len(el.buffer) {
    return el.poller.Trigger(queue.LowPriority, el.read0, c)   // 二次读投递成 LowPriority
}
```

二次读任务是 `LowPriority`，在 sub reactor 繁忙时（大量 conn 同时活跃）可能延迟好几个 Polling 周期。**PUBLISH 延迟读取 = PUBACK 延迟 = 设备重发 = 流量放大**。mqtt-gateway 的 QoS1 场景不能接受这种延迟不确定性。

**风险 3：写侧 ET 同样复杂**

ET 模式下 conn 注册 AddReadWrite（register0:217），`el.write`（eventloop_unix.go:341-357）也要循环写 + 二次触发。PUBACK 回写多一层二次触发的延迟风险。

#### 10.9.4 红线对齐

你的 CLAUDE.md 红线要求可靠性优先：

| 红线 | LT 的保护 | ET 的风险 |
|---|---|---|
| #1 QoS1 PUBACK 只能在 durable ack 后 | LT 读/写延迟确定 | ET 二次触发延迟不确定，PUBACK 可能滞后 |
| #4 所有 queue bounded | LT 无二次触发任务 | ET 的 read0/write0 二次触发产生额外队列负载，风暴时可能堆积 |
| #6 CONNECT 成功 = 认证 + presence 成功 | LT 读 CONNECT 不丢 | ET 漏读会丢 CONNECT 包 |

**LT 的「多几次 epoll 唤醒」是个天然安全网**——即使 OnTraffic 有 bug 没读完，下次 epoll 还会唤醒补救。ET 没有这个安全网。

#### 10.9.5 四维对比总结

| 维度 | LT（默认，✅ 推荐）| ET（WithEdgeTriggeredIO(true)）|
|---|---|---|
| **代码复杂度** | 简单（读一次 return）| 复杂（goto loop + Trigger 二次读/写）|
| **epoll 唤醒次数** | 包小时和 ET 几乎一样 | 包大时省唤醒（mqtt-gateway 包小，省不了）|
| **丢数据风险** | 低（epoll 反复通知兜底）| 高（必须读到 EAGAIN，漏读就丢）|
| **延迟确定性** | 好（没读完立刻再唤醒）| 差（二次触发依赖 LowPriority 调度）|

**一句话：mqtt-gateway 是控制指令通道（逆变器调度、限功率），可靠性 > 极致性能。LT 用「多几次 epoll 唤醒」换「数据不丢、延迟确定」，这个交换划算。**

#### 10.9.6 什么时候应该重新考虑 ET

如果未来场景变化，ET 可能重新值得评估：

1. **固件升级（OTA）走 MQTT**：单包几 MB，ET「一次唤醒读完」收益明显
2. **视频/音频流接入**：持续大流量
3. **单节点连接数 > 100 万且 CPU 成为瓶颈**：epoll 唤醒开销累积成瓶颈

这些都不是第一阶段场景。**先用 LT 把可靠性做扎实，等有明确的 ET 收益场景再切**。切换时重点验证：CONNECT/PUBLISH 在 ET 下的半包处理、ET 二次触发在峰值负载下的延迟分布。

---

### 10.10 ⚠️ OnClose + presence fencing 落地（断连侧完整实现）

`el.close`（eventloop_unix.go:362-403）是连接生命周期的终点，也是你 presence fencing 删除（红线 #8）的落地处。本节讲清楚 el.close 的 7 步链路、5 个触发源、以及**最大的迁移难点：presence 删除不能在 OnClose 里同步阻塞**。

#### 10.10.1 el.close 的 7 步链路

```
┌─ el.close (eventloop_unix.go:362) ──────────────────────────┐
│                                                              │
│  ① stale 检查 (363)                                          │
│     !opened || conn_map 没有它 → return（幂等保护）          │
│                                                              │
│  ② delConn (367)  conn_map 删除 + 计数 -1                    │
│     ← conn 从此对 reactor 不可见                             │
│                                                              │
│  ③ OnClose(c, err) (368)  ⭐ 你的回调                        │
│     ← presence fencing 删除 / connRegistry 移除 /            │
│        标记 s.alive=false 通知在途回调                       │
│                                                              │
│  ④ 榨干 outboundBuffer (370-381)                             │
│     for { writev → Discard } 尽量发完残余 PUBACK 等          │
│     ← 呼应红线 #1：已投递的 PUBACK 尽量发出                  │
│                                                              │
│  ⑤ release (383)  清理 conn 字段                             │
│     c.ctx = nil（清 session）/ buffer 释放 / addr 清空       │
│     ← OnClose 必须在 release 之前（否则 c.Context()=nil）    │
│                                                              │
│  ⑥ epoll Delete + close fd (386)                             │
│     EPOLL_CTL_DEL + unix.Close（防 fd 泄漏）                 │
│                                                              │
│  ⑦ handleAction(action) (402)                                │
│     None → 结束；Shutdown → 关 engine                        │
└──────────────────────────────────────────────────────────────┘
```

**关键顺序**：`delConn`（②）→ `OnClose`（③）→ `release`（⑤）。delConn 在 OnClose 前，说明你的 OnClose 里 reactor 已经找不到这个 conn（但 `c` 指针本身仍有效，`c.Context()` 还能拿 session，因为 release 在后面）。

#### 10.10.2 el.close 的 5 个触发源

所有触发源最终都在 **conn 所属 sub reactor 的 goroutine** 里执行 el.close（包括通过 `c.Close()` → `Trigger` 投递的）。这是它能无锁操作 conn_map 的前提。

| 触发源 | 调用点 | 传入的 err | 含义 |
|---|---|---|---|
| ① read 读到 EOF（设备正常断开）| eventloop_unix.go:271 | `NewSyscallError("read", io.EOF)` | 设备发了 FIN，`unix.Read` 返回 n=0 |
| ② EPOLLRDHUP（对端关闭）| connection_linux.go:34, 62 | `io.EOF` | epoll 通知对端关了写端 |
| ③ read 真错误（网络异常）| eventloop_unix.go:271 | `NewSyscallError("read", err)` | 连接 reset、ECONNRESET 等 |
| ④ write 真错误 | eventloop_unix.go:337 | `NewSyscallError("write", err)` | 写失败（罕见，通常是连接已坏）|
| ⑤ 业务主动关（OnTick 保活判死 / 你的代码）| eventloop_unix.go:89, 103, 481, 505 | `nil` | 通过 `c.Close()` → Trigger → el.close |

**err 的用途**：你可以在 OnClose 里按 err 分类做 metrics（正常断连 EOF vs 异常 reset vs 保活判死踢）。err=nil 一定是业务主动关（⑤）。

#### 10.10.3 OnClose 里该做的 4 件事

```go
func (h *handler) OnClose(c gnet.Conn, err error) gnet.Action {
    s := c.Context().(*session)

    // ① 标记 alive=false，通知所有在途回调（auth/append ack）放弃操作
    s.alive.Store(false)                   // 呼应 10.8.4 的 fencing

    // ② presence fencing 删除（红线 #8）—— ⚠️ 见 10.10.4，不能同步阻塞
    h.deletePresenceAsync(s)

    // ③ 从 connRegistry 移除（10.7 保活判死用的）
    h.registry.mu.Lock()
    delete(h.registry.conns, s.connID)
    h.registry.mu.Unlock()
    h.connCount.Add(-1)                    // bounded accept 计数 -1（10.3 改造 2）

    // ④ 可选：metrics（按 err 分类统计断连原因）
    h.metrics.connClose(err)               // EOF/RDHUP/reset/keepalive-timeout

    return gnet.None
}
```

#### 10.10.4 ⚠️ 最大难点：presence 删除不能在 OnClose 里同步阻塞

**这是断连侧迁移最关键的点**。你 server.go 现在用 `defer s.deletePresence(rec)`（server.go:249, 312）做同步阻塞 Redis 删除：

```go
// server.go:312（goroutine-per-conn 模型）
func (s *Server) deletePresence(rec presence.Record) {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    s.presence.Delete(ctx, rec)   // ⭐ 同步阻塞，最多等 2 秒
}
```

在 goroutine-per-conn 模型下，这个阻塞只影响当前连接的 goroutine，不影响其他连接。**但 gnet 下，OnClose 在 sub reactor 的 goroutine 里执行**。如果 deletePresence 阻塞 2 秒（Redis 超时），**这个 sub reactor 卡 2 秒，它管的几万个 conn 全部失联 2 秒**——严重违反「reactor 不能阻塞」红线。

##### 方案 A（推荐起步）：OnClose 非阻塞，presence 删除丢后台 worker pool

```go
func (h *handler) OnClose(c gnet.Conn, err error) gnet.Action {
    s := c.Context().(*session)
    s.alive.Store(false)

    // 非阻塞操作：立刻完成
    h.registry.mu.Lock()
    delete(h.registry.conns, s.connID)
    h.registry.mu.Unlock()
    h.connCount.Add(-1)

    // ⭐ presence 删除丢后台 worker pool，不阻塞 reactor
    h.presenceDeleter.Submit(s.record())   // bounded queue，满了丢弃（见下文）
    return gnet.None
}

// 后台 worker（固定数量，满足 goroutine 红线）
func (h *handler) presenceDeleterLoop() {
    for rec := range h.presenceDeleteCh {  // bounded channel
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        h.presence.Delete(ctx, rec)        // 在 worker goroutine 里阻塞调 Redis
        cancel()
    }
}
```

**优点**：reactor 立刻释放，不影响其他 conn。worker 数固定（满足 goroutine 红线）。

**丢弃策略**（呼应红线 #4）：presence 删除用 bounded channel，满了 `TryEnqueue` 失败就丢弃。**presence 容忍丢失**——设备重连时会写新 presence 覆盖旧的，旧的不删也不影响新连接（fencing 保护）。

##### 方案 B（不推荐）：短超时同步调用

```go
// ⚠️ 同步调，ctx 超时 50ms，限制最大阻塞
ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
defer cancel()
h.presence.Delete(ctx, rec)   // 最多卡 50ms
```

**风险**：50ms 对 reactor 仍很久。网络抖动批量断连时，每个 OnClose 卡 50ms，sub reactor 严重延迟。**不推荐用于高并发**。

##### 方案 C（大规模）：批量异步删除

```go
// OnClose 只入队
func (h *handler) OnClose(c gnet.Conn, err error) gnet.Action {
    ...
    h.deleteQueue.TryEnqueue(s.record())   // bounded，满了丢弃
    return gnet.None
}

// 后台 worker 攒批
func (h *handler) presenceBatchDeleter() {
    batch := make([]presence.Record, 0, 100)
    ticker := time.NewTicker(100 * time.Millisecond)
    for {
        select {
        case rec := <-h.deleteQueue.ch:
            batch = append(batch, rec)
            if len(batch) >= 100 { h.flushDeletes(batch); batch = batch[:0] }
        case <-ticker.C:
            if len(batch) > 0 { h.flushDeletes(batch); batch = batch[:0] }
        }
    }
}
```

**优点**：批量 Redis 调用降 QPS，OnClose 完全非阻塞。**适合高并发**，但复杂度最高。建议第一版用方案 A，压测发现 presence 删除成瓶颈再升级 C。

##### 三方案对比

| 方案 | reactor 阻塞 | 实现复杂度 | presence 语义 | 适合 |
|---|---|---|---|---|
| A（丢 worker pool）| 无 | 低 | 异步，可能延迟/丢弃 | **推荐起步** |
| B（短超时同步）| ≤50ms | 低 | 同步，有超时上限 | 低并发，不推荐 |
| C（批量异步）| 无 | 高 | 异步，批量降 QPS | 高并发，压测后升级 |

#### 10.10.5 fencing 竞态分析（红线 #8 在 gnet 下的行为）

你红线 #8「Presence 删除必须带 fencing」。fencing 的物理含义：**deletePresence 携带 `(broker_id, broker_epoch, conn_id, session_version)`，Redis 端按这个四元组条件删除**——如果 session_version 不匹配（设备已重连到别的 broker 写了新 presence），删除失败但不影响新连接。

**这套逻辑在 gnet 下完全不变**——fencing 是 Redis 端的原子操作，和 reactor 模型无关。你 server.go 的 `presence.Delete(ctx, rec)` 逻辑直接搬，只是调用位置从 defer 改 OnClose、方式从同步改异步（方案 A）。

但异步化带来一个**新的时序**，fencing 正好保护你：

```
t0  设备断连 → OnClose 触发 → s.alive=false → 入 presenceDeleteCh
t1  设备立刻重连到同一个 broker → CONNECT → writePresence（新 session_version=N+1）
t2  旧的 deletePresence worker 才执行（携带旧 session_version=N）
t3  Redis 端：current_version(N+1) != delete_condition(N) → 删除失败（NOT_OWNER）
    ↑ 这是正确行为！新 presence 不该被旧连接的清理删掉
```

**fencing 保证了「异步延迟的清理不会误删新 presence」**。这正是红线 #8 的设计意图，在 gnet 异步模型下依然成立。

#### 10.10.6 ⚠️ 必须注意的 OnClose 顺序约束

| 操作 | 必须在 OnClose 里做 | 原因 |
|---|---|---|
| `s.alive.Store(false)` | ✅ 第一个做 | 通知 auth/append 在途回调放弃（10.8.4）|
| connRegistry 移除 | ✅ | 否则 OnTick 保活判死会对已死 conn 调 c.Close()（幂等但浪费）|
| `connCount.Add(-1)` | ✅ | 否则 bounded accept 计数只增不减，最终拒绝所有新连接 |
| presence fencing 删除 | ✅（异步）| 红线 #8，但丢 worker pool 不阻塞 |
| metrics | ✅（可选）| 按 err 分类统计断连原因 |
| **c 的 I/O 操作**（Read/Peek/Write）| ❌ | conn 已经在关闭中（delConn 已执行），I/O 无意义 |

**关键**：OnClose 里别再碰 c 的读写（c 已经在关了，outboundBuffer 榨干由 el.close 的 ④ 步做）。OnClose 只做「session 状态变更 + 外部资源清理（presence/registry/metrics）」。

#### 10.10.7 与你 server.go 的对照

| | server.go（goroutine-per-conn）| gnet 迁移后 |
|---|---|---|
| presence 删除位置 | `defer s.deletePresence(rec)`（server.go:249）| `OnClose` 里 `h.deletePresenceAsync(s)` |
| presence 删除方式 | 同步阻塞 Redis（server.go:315）| **异步丢 worker pool**（方案 A）|
| presence fencing | `ConnID + SessionVersion` 四元组（server.go:312）| **完全不变**，Redis 端原子操作 |
| 清理触发保证 | `defer`（goroutine 退出必触发）| `OnClose`（el.close 必调，幂等保护防重复）|
| 阻塞影响 | 仅当前连接 goroutine | ⚠️ 必须异步，否则卡死整个 sub reactor |

**核心迁移**：`defer deletePresence` → `OnClose + 异步 worker pool`。fencing 逻辑（四元组 + Redis 条件删除）一行不改。

---

### 10.11 主从 reactor vs ReusePort 怎么选

**结论**：mqtt-gateway 选 **`activateReactors`（主从 reactor，默认）**，不开 `WithReusePort(true)`。因为你不需要 ReusePort 的核心收益（多进程同端口），而 ReusePort 的代价（失去主从隔离、accept 不再批量）在单进程下白承担。

#### 10.11.1 分叉只看一个开关

`start()` 的分叉逻辑（engine_unix.go:190-196）：

```go
func (eng *engine) start(ctx, numEventLoop) error {
    if eng.opts.ReusePort {
        return eng.runEventLoops(ctx, numEventLoop)    // 路径 B：ReusePort
    }
    return eng.activateReactors(ctx, numEventLoop)     // 路径 A：默认（主从）
}
```

**你不配 `WithReusePort(true)`，就自动走 `activateReactors`**。不需要额外配置。

#### 10.11.2 两种模式的拓扑

##### 路径 A：activateReactors（主从 reactor，默认）

```
                    ┌─────────────────────┐
   新连接 ──Accept──▶│  主 reactor (1 个)   │  只 accept，不处理 I/O
                    │  el.rotate()         │  AddRead(listener, true=ET)
                    │  engine_unix.go:177  │
                    └──────────┬──────────┘
                               │ accept0 里 LB.next(addr) 选 sub + Trigger(register)
                    ┌──────────▼──────────┐
                    │  sub reactor #0..N   │  只处理已建立连接的 I/O
                    │  el.orbit()          │  不监听 listener
                    │  engine_unix.go:155  │
                    └─────────────────────┘
```

##### 路径 B：runEventLoops（ReusePort，每个 reactor 都 accept）

```
   新连接 ──SO_REUSEPORT──▶ 内核负载均衡到 N 个 listener fd
                    ┌─────────────────────┐
                    │  reactor #0          │  既 accept 又处理 I/O
                    │  el.run()            │  AddRead(listener, false=LT)
                    │  engine_unix.go:123  │
                    ├─────────────────────┤
                    │  reactor #1          │  既 accept 又处理 I/O
                    │  el.run()            │
                    ├─────────────────────┤
                    │  ...                 │
                    └─────────────────────┘
```

#### 10.11.3 源码差异对照

| 维度 | `activateReactors`（默认）| `runEventLoops`（ReusePort）|
|---|---|---|
| 行号 | engine_unix.go:137 | engine_unix.go:82 |
| listener fd 数 | **1 个**（共用 `eng.listeners`，:169）| **N 个**（i>0 时每个 reactor 新建一个，:88-95，都设 SO_REUSEPORT 绑同端口）|
| listener epoll 注册 | 主 reactor `AddRead(ln, true)` **ET**（:170）| 每个 reactor `AddRead(ln, false)` **LT**（:109）|
| accept 函数 | `el.accept0`（**批量 drain**，acceptor_unix.go:30）| `el.accept`（**单个**，acceptor_unix.go:77）|
| reactor 循环 | 主 `el.rotate`（:177）+ N 个 `el.orbit`（:155）| N 个 `el.run`（:123）|
| 主 reactor 存在？ | 是（`eng.ingress`，:174）| **否** |
| 新连接分发 | **软件分发**：accept0 里 `LB.next(addr)` + `Trigger`（:68）| **硬件分发**：内核 SO_REUSEPORT 哈希 |
| 分发算法可控？ | ✅ RoundRobin/LeastConnections/SourceAddrHash（第 7 站）| ❌ 内核 socket 哈希，不可预测 |
| ticker 跑在哪 | `eng.ingress.ticker`（:182，独立 reactor）| `el0.ticker`（:129，eventloop 0，和 accept+I/O 共享）|
| accept 和 I/O 的关系 | **分离**（主 accept，sub 做 I/O）| **混合**（每个 reactor 都做）|

#### 10.11.4 为什么选默认（activateReactors）

##### 理由 1：你只有一个进程，ReusePort 的核心收益用不上

SO_REUSEPORT 的主要价值是**多进程同端口**——多个进程各自 bind 同一个端口，内核做负载均衡。典型场景：
- Nginx 多 worker 进程，每个进程一个 reactor，内核把连接分散到各进程
- 多实例部署时避免「惊群」（一个连接到来，所有进程被唤醒争抢）

**但 mqtt-gateway 是单进程多 goroutine**（gnet 内部用 goroutine 不用多进程）。单进程下，N 个 reactor 都在同一个进程里，开 ReusePort 只是让内核帮你做「连接分发到哪个 goroutine」——而这个分发 gnet 用 `LB.next` 软件做也很高效（一次取模或遍历）。

**单进程开 ReusePort 的收益 ≈ 0，但代价（见下面）实打实。**

##### 理由 2：主从分离让你能独立抗连接风暴

`activateReactors` 的主从分工有个关键好处：**主 reactor 专门抗连接风暴，不和业务 I/O 抢资源**。

```
连接风暴场景（逆变器批量上线：固件升级触发、区域网络恢复）：

  activateReactors（主从分离）：
    主 reactor：疯狂 accept（ET 批量 drain），不受业务影响
    sub reactor：处理已有连接的 PUBLISH，不被 accept 打断
    → 两边互不阻塞

  runEventLoops（accept+I/O 混合）：
    每个 reactor：既 accept 又处理 PUBLISH，两者抢同一个 reactor 时间
    → 某个 reactor 卡在慢 PUBLISH 时，它的 accept 队列堆积
```

mqtt-gateway 的设备批量上线是常见场景，主从分离的抗风暴能力对你有价值。

##### 理由 3：软件 LB 给你更多控制

`activateReactors` 的连接分发走 `LB.next(addr)`，你能选算法（第 7 站）：

| 算法 | 行为 | 适合 |
|---|---|---|
| `RoundRobin`（默认）| 轮询，最简单 | 通用 |
| **`LeastConnections`** | 按连接数均衡 | **推荐 mqtt-gateway** |
| `SourceAddrHash` | 同 IP 粘连 | ⚠️ NAT 后大量设备共享 IP，会堆到一个 reactor，**别用** |

`runEventLoops` 走内核 SO_REUSEPORT 分发，**你控制不了分发算法**（Linux 内核用 socket 哈希，不可预测）。如果内核把大量连接分到同一个 reactor，你没法干预。

##### 理由 4：ticker 行为更清晰

`activateReactors` 的 ticker 跑在独立的 `eng.ingress` reactor 上（engine_unix.go:182），和业务 I/O 完全隔离。你的 OnTick（保活判死扫描，10.7）不会和 sub reactor 的 OnTraffic 互相影响。

`runEventLoops` 的 ticker 跑在 eventloop 0（:129），和 eventloop 0 的 accept + I/O 共享 goroutine。OnTick 耗时会影响 eventloop 0 的 accept 能力。

#### 10.11.5 ReusePort 的代价

如果开了 `WithReusePort(true)`，你要承担：

1. **N 个 listener fd**：每个 reactor 一个 fd（engine_unix.go:88-95），fd 占用 ×N。高连接数下 fd 资源更紧张。
2. **accept 不再批量**：`el.accept`（acceptor_unix.go:77）只 accept 一个，不用 `accept0` 的批量 drain。连接风暴时效率低（每个连接一次 epoll 唤醒）。
3. **LT 模式 accept**：runEventLoops 用 LT（:109 `false`），和 activateReactors 的 ET（:170 `true`）不同。
4. **失去主从隔离**：accept 和 I/O 混在一起，连接风暴时互相影响。

#### 10.11.6 什么时候应该重新考虑 ReusePort

只有这些场景才值得重新评估：

1. **多进程部署**：mqtt-gateway 改成多进程（比如 systemd 起 4 个进程），每进程一个 gnet engine，用 ReusePort 让 4 个进程共享 1883 端口。**这是 ReusePort 的本职用途**。但第一阶段是单进程，用不上。
2. **accept 成为瓶颈（百万级 CPS）**：单进程下主 reactor 的 accept 吞吐到极限（百万级每秒新建连接）。这时开 ReusePort 让 N 个 reactor 分担 accept。**但逆变器连接是长连接，建连是低频，远达不到这个量级。**
3. **想消除 accept 惊群**：某些老内核上，一个 listener 被多 reactor 监听会惊群。但 gnet 的 activateReactors **只有一个 reactor 监听 listener，不存在惊群**。ReusePort 反而是用来解决「多 listener 监听同端口」的惊群，单 listener 不需要。

**这三个场景 mqtt-gateway 第一阶段都不满足。先用默认 activateReactors。**

#### 10.11.7 配置建议

```go
gnet.Run(handler, "tcp://:1883",
    gnet.WithMulticore(true),                       // N 个 sub reactor
    gnet.WithLoadBalancing(gnet.LeastConnections),  // 软件分发算法（推荐）
    // ⭐ 不配 WithReusePort(true) → 自动走 activateReactors（主从 reactor）
    gnet.WithTicker(true),                          // OnTick 保活判死
    // ... 其他见第 2 站
)
```

| 选型 | 何时用 | mqtt-gateway |
|---|---|---|
| **activateReactors**（默认，不配 ReusePort）| 单进程、要抗连接风暴、要软件 LB 控制 | ✅ **第一阶段用这个** |
| runEventLoops（配 `WithReusePort(true)`）| 多进程部署、accept 百万级瓶颈 | ❌ 第一阶段用不上 |

**一句话：不配 `WithReusePort(true)`，就自动走 `activateReactors`。这是 gnet 的默认推荐，也是 mqtt-gateway 的正确选择。等到真要多进程部署时再考虑 ReusePort。**

---

## 附录 A：关键文件速查表（对应工作空间 /home/wangjunbo/workspace-iot/gnet）

| 文件 | 行数 | 一句话 |
|---|---|---|
| `gnet.go` | 802 | 对外 API：Run/Conn/EventHandler/Action/EventLoop/Engine |
| `options.go` | 327 | 所有 WithXxx 选项 |
| `load_balancer.go` | 142 | RR/LeastConn/Hash 三种 LB |
| `engine_unix.go` | 306 | engine 启动/关停/start 分叉 |
| `acceptor_unix.go` | 117 | accept0（批量）/ accept（单个）|
| `listener_unix.go` | 143 | socket fd 创建 + sockopt |
| `eventloop_unix.go` | 518 | read/write/close/open/register/ticker/Enroll/Execute |
| `reactor_default.go` | 114 | rotate/orbit/run 三循环 |
| `reactor_ultimate.go` | 84 | poll_opt 优化版三循环 |
| `connection_unix.go` | 562 | conn 结构 + Read/AsyncWrite/buffer |
| `connection_linux.go` | 70 | processIO（epoll 事件分发）|
| `connection_bsd.go` | 57 | processIO（kqueue 版）|
| `conn_map.go` | 72 | fd→conn 映射（**默认构建用这个**）|
| `conn_matrix.go` | 158 | fd→conn 映射（gc_opt 构建用）|
| `pkg/netpoll/poller_epoll_default.go` | ~245 | Linux default Poller：Polling/Trigger |
| `pkg/queue/queue.go` | 61 | EventPriority + Task 定义 |

## 附录 B：术语表

| 术语 | 含义 |
|---|---|
| Reactor | 事件循环模式：epoll 等事件 → 回调处理 |
| eventloop | gnet 的 reactor，一个 goroutine 跑一个 |
| main reactor / ingress | 默认模式下专做 accept 的 reactor（engine.ingress）|
| sub reactor | 默认模式下处理已建连接的 reactor |
| LT (Level Triggered) | 水平触发：只要 fd 可读写就一直通知（epoll 默认）|
| ET (Edge Triggered) | 边沿触发：状态变化时通知一次，必须读到 EAGAIN |
| PollAttachment | fd + 回调 的捆绑，注册到 poller（defs_poller.go:23）|
| PollEventHandler | epoll 事件回调签名 `func(int, IOEvent, IOFlags) error`（defs_poller.go:20）|
| queue.Func | Trigger 任务回调签名 `func(any) error`（queue.go:21）|
| Trigger | 跨 reactor 投递任务的方法（投递 queue.Func，不是 PollEventHandler）|
| inboundBuffer | per-conn 的入站半包缓冲（你没读走的）|
| outboundBuffer | per-conn 的出站缓冲（AsyncWrite 进来的）|
| `el.buffer` | eventloop 级共享读缓冲（⚠️ 不能跨调用持有）|
| HighPriority / LowPriority | 任务优先级，前者每轮全 drain，后者每轮最多 256（queue.go:53）|

## 附录 C：v2.4.1 vs v2.8.0 关键差异（如果你以后升级）

> 帮你识别未来升级时要改的地方。

| 点 | v2.4.1（当前） | v2.8.0 |
|---|---|---|
| `newTCPConn` / `newStreamConn` | `newStreamConn`（统一）| 改名 `newTCPConn` |
| `OnTraffic` 返回值 | `Action` | `Action`（v2.5+ 改成 `(Action, error)`，到 v2.8 稳定）|
| `Trigger` 签名 | `(priority, fn queue.Func, param any)` | `(priority, handler, arg)`（语义类似，Func 类型可能变）|
| `SetDeadline` 系列 | 返回 `ErrUnsupportedOp` | 同（仍不支持）|
| `conn.proto` 字段 | 有 | 去掉，用别的方式判断 |
| `Engine.Register/Enroll/Execute` | 完整可用（gnet.go:92）| 部分注释掉（gnet.go:174-226）|
| 默认连接集合 | `conn_map.go`（map） | `conn_matrix.go`（数组，默认）|
| `gc_opt` build tag | 需要 `-tags gc_opt` 启用 conn_matrix | 可能调整 |

## 附录 D：*BSD / Darwin 差异

- `connection_bsd.go` 替代 `connection_linux.go`：processIO 用 kqueue 的 filter/flags。
- `poller_kqueue_*.go` 替代 `poller_epoll_*.go`；kqueue 没有 eventfd，用 `poller_kqueue_wakeup*.go` 的 pipe/socketpair 唤醒。
- `createListeners`（gnet.go:622）在 BSD 上默认关 ReusePort（BSD 的 SO_REUSEPORT 不做负载均衡）。
- kqueue 关闭 fd 时自动从队列删除，epoll 要显式 `Delete`（orbit/run 里有对 stale fd 的处理，reactor_default.go:54-62）。
