# NATS 源码分析

> 分析对象：nats-server（工作区 vendored checkout：`nats-server/`）
> 目标：逐函数、逐行级别的源码走读，作为自建 cloud MQTT ingress broker（`mqtt-gateway/`）的实现参考。
> 与 [`nats-analysis.md`](./nats-analysis.md) 的区别：那份是**架构/设计**层面的总览（含简化伪代码）；本文档是**真实源码**走读。
>
> **约定**
> - 源码引用格式为 `` `路径:行号` ``，均相对工作区根目录（如 `nats-server/server/client.go:1403`）。
> - 代码块为 nats-server **真实源码节选**（保留原注释），非伪代码。
> - 分析版本：v2.10.x（Core + JetStream + 内置 MQTT 网关）。

---

## 目录

- [第一节：readLoop —— 连接读循环与数据流](#第一节readloop--连接读循环与数据流)
- [第二节：writeLoop 写循环与批量 flush 与慢客户端背压](#第二节writeloop-写循环与批量-flush-与慢客户端背压)
- 第三节：（待补充，候选：processInboundClientMsg / SubList 订阅扇出、route/leaf/gateway 集群转发、MQTT 会话与 QoS1/2 持久化……）

---

## 第一节：readLoop —— 连接读循环与数据流

> 本节回答三个问题：① `readLoop` 从哪里来、被谁启动？② 它怎么把 socket 数据读进缓冲？③ 读进来的数据最终去了哪里？
> 重点结论先行：nats-server **所有协议（NATS / MQTT / Route / Gateway / Leaf / WebSocket）共用同一个 `readLoop` 实现**，靠 `parse()` 里按连接类型分支走不同解析器；MQTT 没有独立的 socket reader。

### 1.1 readLoop 在哪里，又被谁启动

#### 唯一定义

`readLoop` 在整个工作区只有**一个定义**（自建的 `mqtt-gateway/` 没有这个函数，它的连接循环叫 `handleConn`，见 `mqtt-gateway/internal/server/server.go:200`）：

```go
// readLoop is the main socket read functionality.
// Runs in its own Go routine.
func (c *client) readLoop(pre []byte) {        // nats-server/server/client.go:1403
```

签名里的 `pre []byte` 是一个关键参数：它是**在 readLoop 启动前已经被从 socket 读走的字节**，readLoop 首轮会先把它喂给解析器，避免丢字节。

#### 6 个启动点

全部是 `s.startGoRoutine(func(){ c.readLoop(pre) })`，每个连接类型一个：

| 连接类型 | 启动点 | `pre` 缓冲内容 |
|---|---|---|
| **MQTT 客户端** | `nats-server/server/mqtt.go:646` | 4 字节 TLS 嗅探（明文 MQTT 时为 `nil`）|
| NATS 客户端 | `nats-server/server/server.go:3531` | TLS/PROXY 嗅探字节 |
| 集群 Route | `nats-server/server/route.go:2047` | `nil` |
| Gateway | `nats-server/server/gateway.go:928` | `nil` |
| WebSocket | `nats-server/server/websocket.go:1470` | `nil` |
| Leafnode | `nats-server/server/leafnode.go:1479` | WS 握手字节或 `nil` |

#### MQTT 的完整启动链（最相关）

```
startMQTT()                                              mqtt.go:501
  └─ go acceptConnections(hl, "MQTT", createMQTTClient)   mqtt.go:535
       └─ 每个 Accept → 一个 goroutine 调 createMQTTClient(conn)   server.go:2879
            └─ s.startGoRoutine(func(){ c.readLoop(pre) })         mqtt.go:646
```

`acceptConnections` 是通用的“每个 socket 起一个 goroutine”的 accept 循环（`nats-server/server/server.go:2879`）：

```go
if !s.startGoRoutine(func() {
    s.reloadMu.RLock()
    createFunc(conn)
    s.reloadMu.RUnlock()
    s.grWG.Done()
}) {
    conn.Close()
}
```

⚠️ **一个容易踩的认知误区**：MQTT 的 `CONNECT` 报文**不是在 readLoop 之前预读的**。`pre` 参数只是 TLS-vs-明文的 4 字节嗅探（`mqtt.go:608-620`），并非 CONNECT：

```go
var pre []byte
if tlsRequired && opts.AllowNonTLS {
    pre = make([]byte, 4)
    c.nc.SetReadDeadline(time.Now().Add(secondsToDuration(opts.MQTT.TLSTimeout)))
    n, _ := io.ReadFull(c.nc, pre[:])
    c.nc.SetReadDeadline(time.Time{})
    pre = pre[:n]
    if n > 0 && pre[0] == 0x16 {   // 0x16 = TLS handshake
        tlsRequired = true
    } else {
        tlsRequired = false
    }
}
```

CONNECT 本身是在 readLoop 内部、由 MQTT 解析器解析的。`mqtt.go:644-647` 的启动代码：

```go
// No Ping timer for MQTT clients...

s.startGoRoutine(func() { c.readLoop(pre) })
s.startGoRoutine(func() { c.writeLoop() })
```

> **并发模型**：每个连接 **2 个 goroutine**——一个 readLoop（读+解析），一个 writeLoop（批量写）。没有“每报文一个 goroutine”。`startGoRoutine` 注册到 `s.grWG`（WaitGroup），所以 `Shutdown` 会阻塞到所有 readLoop/writeLoop 退出；这也是为什么 route/gateway/leaf 必须在 spawn readLoop **之前**把连接加入 `tempClients`（否则 spawn 与注册之间发生 shutdown 会泄漏一个 server 关不掉的 readLoop）。

---

### 1.2 readLoop 内部：读进缓冲（逐步源码）

#### 步骤 ① 缓冲尺寸常量

`nats-server/server/client.go:110-116`：

```go
// For controlling dynamic buffer sizes.
startBufSize    = 512   // For INFO/CONNECT block
minBufSize      = 64    // Smallest to shrink to for PING/PONG
maxBufSize      = 65536 // 64k
shortsToShrink  = 2     // Trigger to shrink dynamic buffers
maxFlushPending = 10    // Max fsps to have in order to wait for writeLoop
readLoopReport  = 2 * time.Second
```

#### 步骤 ② 初始化读缓冲大小

在初始的 `c.mu.Lock()` 块内（`client.go:1418`）：

```go
c.in.rsz = startBufSize   // 首块按 512 字节
```

#### 步骤 ③ 挂 MQTT reader（仅 MQTT 连接）

`client.go:1415-1417`：

```go
if c.isMqtt() {
    c.mqtt.r = &mqttReader{reader: nc}
}
```

⚠️ **关键**：这个 `mqttReader` 的 `reader: nc` 字段**只用来设 ReadDeadline，从不直接读 socket**。真正读 socket 的始终是 readLoop 自己。MQTT 解析器是吃 readLoop 喂进来的字节切片，不是自己 `Read()`。

#### 步骤 ④ 分配读缓冲 `b`、确定读源 `reader`

`client.go:1441-1459`：

```go
// Start read buffer.
b := make([]byte, c.in.rsz)

// Websocket clients will return several slices ... For non WS clients though, we
// will always have 1 slice per loop iteration. So we define this here ...
var _bufs [1][]byte
bufs := _bufs[:1]
...
var decompress bool
var reader io.Reader
reader = nc
```

> **关于“固定缓冲”的澄清**：`b` 是一个局部 `[]byte`，它在每轮循环中**会被重新分配**（见步骤⑧的 grow/shrink），所以严格说是**动态伸缩缓冲**，不是死固定大小。但在单次 `Read` 调用内它是一段固定容量的连续内存。

#### 步骤 ⑤ 每轮循环：先消费 `pre`，否则唯一一次 socket 读

`client.go:1461-1477`：

```go
for {
    var n int
    var err error

    // If we have a pre buffer parse that first.
    if len(pre) > 0 {
        b = pre
        n = len(pre)
        pre = nil
    } else {
        n, err = reader.Read(b)
        // If we have any data we will try to parse and exit at the end.
        if n == 0 && err != nil {
            c.closeConnection(closedStateForErr(err))
            return
        }
    }
```

- 首轮若有 `pre`，直接当成“已读字节”用（`b = pre; n = len(pre)`），不重复读 socket。
- 之后每轮 `n, err = reader.Read(b)`——**这是整轮循环里唯一一次 socket 读**。
- `n == 0 && err != nil`（干净 EOF 或读出错且无数据）→ 直接关连接返回。

#### 步骤 ⑥ 读到的字节打包进 `bufs`（非 WS 分支）

`client.go:1495-1511`（WS 走 `wsReadAndParse` 自己解码帧+解析；非 WS 把 `b[:n]` 装进单元素 `bufs`）：

```go
if ws {
    err = c.wsReadAndParse(wsr, reader, b[:n])
    ...
    goto postParse
} else {
    bufs[0] = b[:n]
}
```

#### 步骤 ⑦ 交给解析器（**数据在这里离开 readLoop**）

`client.go:1515-1516`：

```go
for i := 0; i < len(bufs); i++ {
    if err := c.parse(bufs[i]); err != nil {
```

非 WS 客户端 `len(bufs)==1`，等价于一次 `c.parse(b[:n])`。`parse` 内部会把解析出的消息扇出到目标连接的 outbound，并累加它们的 `fsp`（flush-signal-pending）计数。

#### 步骤 ⑧ postParse：刷新下游 + 动态伸缩缓冲

- `client.go:1580`：`last := c.flushClients(0)`——通知本轮收到消息的订阅者去 flush 它们的 writeLoop（`0` = 非阻塞）。
- `client.go:1592-1607`：在 `c.mu.Lock()` 下做缓冲 grow/shrink（已逐字核对）：

```go
if n >= cap(b) {
    c.in.srs = 0
} else if n < cap(b)/2 { // divide by 2 b/c we want less than what we would shrink to.
    c.in.srs++
}

// Update read buffer size as/if needed.
if n >= cap(b) && cap(b) < maxBufSize {
    // Grow
    c.in.rsz = int32(cap(b) * 2)
    b = make([]byte, c.in.rsz)
} else if n < cap(b) && cap(b) > minBufSize && c.in.srs > shortsToShrink {
    // Shrink, for now don't accelerate, ping/pong will eventually sort it out.
    c.in.rsz = int32(cap(b) / 2)
    b = make([]byte, c.in.rsz)
}
```

**伸缩策略**：读满了（`n >= cap(b)`）且未达 64KiB → **翻倍**；连续 `shortsToShrink=2` 次短读（`n < cap(b)/2`）且大于 64B → **减半**。于是读缓冲在 `64B ↔ 64KiB` 之间自适应报文大小，既省内存又抗大报文。

随后回到 `for` 循环顶部，进入下一轮 `reader.Read(b)`。

---

### 1.3 读取的数据去了哪里：parse 总入口

步骤⑦的 `c.parse(buf)` 是**协议分发总入口**（`nats-server/server/parser.go:137`，已逐字核对）：

```go
func (c *client) parse(buf []byte) error {
    // Branch out to mqtt clients. c.mqtt is immutable, but should it become
    // an issue (say data race detection), we could branch outside in readLoop
    if c.isMqtt() {
        return c.mqttParse(buf)
    }
    ...
}
```

- **MQTT 连接** → `c.mqttParse(buf)`（`mqtt.go:742`）。
- **NATS 连接** → 继续往下走 NATS 文本协议手写状态机（`parser.go:143+`：`OP_START → OP_P → OP_PU → OP_PUB → ...`，详见 `nats-analysis.md` §2.2.2）。

> 设计精髓：readLoop 完全协议无关，只负责“读一块字节 + 交给 parse”。协议差异收敛在 `parse` 的一个 `if` 分支里。

---

### 1.4 MQTT 路径详解（`mqttParse`）

`mqttParse` 拿到 readLoop 喂来的 `buf` 后，第一件事是 `r.reset(buf)`（`mqtt.go:759`），把它设为解析器的内存缓冲，然后**纯内存解析 MQTT 报文，不再碰 socket**。

#### 1.4.1 半包重组（最巧妙的部分）

MQTT 报文可能跨多个 readLoop chunk。`mqttReader` 用 `r.pos`（当前解析偏移）、`r.pstart`（在途报文起点）、`r.pbuf`（暂存的半包尾巴）三件套实现重组。

`reset` 把上一轮没收完的 `r.pbuf` 拼到新 chunk 前面（`mqtt.go:5818`）：

```go
func (r *mqttReader) reset(buf []byte) {
    if l := len(r.pbuf); l > 0 {
        tmp := make([]byte, l+len(buf))
        copy(tmp, r.pbuf)
        copy(tmp[l:], buf)
        buf = tmp
        r.pbuf = nil
    }
    r.buf = buf
    r.pos = 0
    r.pstart = 0
}
```

`readPacketLenWithCheck` 解析 MQTT 变长 remaining-length，并检查整包是否都在缓冲里；**不够就把尾巴存进 `r.pbuf` 并返回 `complete=false`**，让 `mqttParse` 退出循环（`mqtt.go:5848`）：

```go
func (r *mqttReader) readPacketLenWithCheck(check bool) (int, bool, error) {
    m := 1; v := 0
    for {
        var b byte
        if r.pos != len(r.buf) { b = r.buf[r.pos]; r.pos++ } else { break }
        v += int(b&0x7f) * m
        if (b & 0x80) == 0 {
            if check && r.pos+v > len(r.buf) { break }   // 整包不完整
            return v, true, nil
        }
        m *= 0x80
        if m > 0x200000 { return 0, false, errMQTTMalformedVarInt }
    }
    r.pbuf = make([]byte, len(r.buf)-r.pstart)
    copy(r.pbuf, r.buf[r.pstart:])                        // 暂存半包
    return 0, false, nil
}
```

> 结论：`mqttReader` 是一个**逐块喂数据的有状态游标**，**不是 `io.Reader`**。`reader: nc` 字段仅用于 `SetReadDeadline`（见 `mqtt.go:753`）。

#### 1.4.2 报文类型分派

解出 fixed-header 后按 packet type switch（`mqtt.go:804-959`），以 PUBLISH 为例（`mqtt.go:838-850`）：

```go
case mqttPacketPub:
    pp := c.mqtt.pp
    pp.flags = b & mqttPacketFlagMask
    err = c.mqttParsePub(r, pl, pp, hasMappings)
    if trace { ... }
    if err == nil {
        err = s.mqttProcessPub(c, pp, trace)
    }
```

其它：CONNECT → `mqttProcessConnect`；SUBSCRIBE → `mqttProcessSubs` + `mqttEnqueueSubAck`；PING → `mqttEnqueuePingResp`。

---

### 1.5 一条 MQTT PUBLISH 的完整旅程（端到端数据流）

下面追踪一次设备 PUBLISH 从 socket 字节到投递给订阅者的全链路（行号均为源码核对）：

1. **解析 PUBLISH** — `mqtt.go:4062 mqttParsePub`：读 topic，转成 NATS subject，切出 payload：
   ```go
   // mqtt.go:4086-4094
   // Convert the topic to a NATS subject. This call will also check that
   // there is no MQTT wildcards (Spec [MQTT-3.3.2-2] and [MQTT-4.7.1-1])
   pp.subject, err = mqttTopicToNATSPubSubject(pp.topic)
   ```
   payload 切片 `pp.msg = r.buf[start:r.pos]`（按 remaining-length 减去已消费的变长头计算）。`pp` 现持有：原始 topic、转换后的 NATS subject、payload、QoS、packet id。

2. **按 QoS 分派** — `mqtt.go:4263 mqttProcessPub`：
   ```go
   // mqtt.go:4275-4291
   switch qos {
   case 0:
       return s.mqttInitiateMsgDelivery(c, pp)
   case 1:
       // [MQTT-4.3.2-2]. Initiate onward delivery of the Application Message, Send PUBACK.
       err := s.mqttInitiateMsgDelivery(c, pp)
       if err == nil {
           c.mqttEnqueuePubResponse(mqttPacketPubAck, pp.pi, trace)
       }
       return err
   ```
   QoS2 → `mqttStoreQoS2MsgOnce`（store-then-PUBREC，投递延后到 PUBREL）。

3. **包装成 NATS 消息，交给标准入站路径** — `mqtt.go:4311 mqttInitiateMsgDelivery`：
   ```go
   // mqtt.go:4311-4335
   func (s *Server) mqttInitiateMsgDelivery(c *client, pp *mqttPublish) error {
       natsMsg, headerLen := mqttNewDeliverableMessage(pp, false)
       // Set the client's pubarg for processing.
       c.pa.subject = pp.subject
       ...
       _, permIssue := c.processInboundClientMsg(natsMsg)
       if permIssue { return nil }
   ```
   `mqttNewDeliverableMessage` 在 payload 前加一个 `NATS/1.0` 头块（含 `Nmqtt-Pub:<qos>`）。QoS1/2 还会把消息持久化到 JetStream 的 `$MQTT.msgs.<subject>`。

4. **标准 NATS 扇出** — `client.go:4311 processInboundClientMsg`：
   - `client.go:4366-4369`：MQTT 客户端先处理 retain 标志（`c.mqttHandlePubRetain()`）。
   - 用 subject 匹配订阅表 `acc.sl.Match(string(c.pa.subject))`。
   - `c.processMsgResults(...)` 把消息扇出到所有匹配订阅者——包括 NATS 订阅者、MQTT 订阅者（其 SUBSCRIBE 已被映射到等价 subject）、route/leaf/gateway。

> **复用要点**：MQTT PUBLISH 与 NATS PUB 共享**整条下游投递管线**（订阅表匹配、队列组、集群路由、leaf/gateway 转发）。一个 MQTT 订阅者和一个 NATS 订阅者，只要 subject 等价，就能看到同一条消息。topic↔subject 的转换规则：MQTT `/` → NATS `.`，字面量 `.` 转义为 `//`；SUBSCRIBE 通配符 `+`/`#` → NATS `*`/`>`。

---

### 1.6 关键设计点与对自建 broker 的启发

1. **单一 readLoop 服务所有协议。** 协议差异收敛在 `parse()` 的一个 `if` 分支里，新增协议只需加一个解析器。你的 `mqtt-gateway` 是单协议（MQTT），`handleConn` 直接读+解析是合理的；但若以后要扩协议，这个“读循环协议无关 + 解析器可插拔”的分层值得借鉴。

2. **MQTT 没有独立 socket reader。** nats-server 的 `mqttReader` 是吃 readLoop 喂来的字节切片的有状态游标，`reader: nc` 只用于设 deadline。**半包重组（`r.pbuf` 暂存尾巴、下轮 `reset` 拼接）是最值得抄的点**——TCP 是字节流，报文跨 chunk 是必然，你的 `handleConn` 也必须处理。

3. **“固定缓冲”其实是自适应伸缩缓冲。** 读满翻倍、连续短读减半，区间 `64B ↔ 64KiB`（`client.go:1592-1607`）。比固定大小更省内存又抗大报文，是一个低成本的优化项。

4. **MQTT topic 与 NATS subject 的桥接落在投递入口。** `mqttInitiateMsgDelivery` 设 `c.pa.subject = pp.subject` 后直接调 `processInboundClientMsg`，让 MQTT 复用 NATS 的全部扇出能力。你的 broker 如果后端也要对接统一总线（NATS/Kafka），这种“接入层只做协议转换、投递复用底层管线”的分层是可参考的架构方向。

5. **并发模型：每连接 2 个 goroutine（read+write），无 per-packet goroutine。** 扇出靠目标连接的 writeLoop + `fsp` 信号驱动批量 flush（见 `nats-analysis.md` §3.4）。这避免了海量 goroutine 抖动，也是百万连接的前提。

---

### 1.7 完整数据流图

```
MQTT socket accept
  → createMQTTClient                     mqtt.go:542
  → go readLoop(pre)                     mqtt.go:646
       │  reader.Read(b)                 client.go:1471   ← 唯一 socket 读，读进缓冲 b
       │  bufs[0] = b[:n]                client.go:1510
       └→ c.parse(bufs[i])               client.go:1516   ← 数据在此离开 readLoop
            └→ c.mqttParse(buf)          parser.go:137
                 └→ r.reset(buf)         mqtt.go:759      ← 纯内存解析 + 半包重组(r.pbuf)
                      └→ 报文 switch      mqtt.go:804
                           ├─ CONNECT → mqttProcessConnect
                           ├─ SUBSCRIBE → mqttProcessSubs + SubAck
                           └─ PUBLISH:
                                mqttParsePub            mqtt.go:4062  (topic→subject, 切 payload)
                                → mqttProcessPub        mqtt.go:4263  (按 QoS 分派)
                                → mqttInitiateMsgDelivery mqtt.go:4311
                                     → processInboundClientMsg client.go:4311
                                          → acc.sl.Match(subject)
                                          → processMsgResults → 订阅者 writeLoop / route / leaf / gateway
                                          (QoS1/2 另持久化到 JetStream $MQTT.msgs.<subject>)
```

---

*第一节完。下一节见下。*

---

## 第二节：writeLoop 写循环与批量 flush 与慢客户端背压

> 本节回答三个问题：① `writeLoop` 靠什么唤醒（不是 channel，是 `sync.Cond`）？② 多条消息怎么合并成一次 syscall（`writev`）？③ 慢客户端怎么不拖垮 broker 内存（背压 + 硬断连）？
> 重点结论先行：nats-server 用 **`sync.Cond` + 池化 `net.Buffers` + `writev`** 实现写合并，**没有周期性 flush 定时器**；每客户端出站 pending 有 **64MiB 硬上限**，超限直接**断连**（不是丢订阅，是关整个连接）。
>
> ⚠️ **版本说明**：本工作区的 nats-server 是较新的 `main`，写路径已重构。网上很多老资料里的 `writeString`/`writeDirect`/`setFlushTimer`/`kick` 在这份代码里**都不存在**——append 统一走 `queueOutbound`，唤醒是 `sync.Cond`，没有周期 flush 定时器。本节所有结论均按这份 checkout 的真实源码。

### 2.1 写路径全景：每连接一个 writeLoop

`writeLoop` 定义在 `nats-server/server/client.go:1312`，和 readLoop 成对：每连接 2 个 goroutine（见第一节"2-goroutine 模型"讨论）。启动点紧跟 readLoop：

| 连接类型 | writeLoop 启动点 | 说明 |
|---|---|---|
| MQTT | `mqtt.go:647` | 紧跟 readLoop |
| NATS client | `server.go:3534` | 紧跟 readLoop |
| Route | `route.go:2050` | |
| Gateway | `gateway.go:931` | |
| WebSocket | `websocket.go:1471` | |
| Leafnode | `leafnode.go:1484` / `3688` | **条件**：solicited（主动外连）的 leaf 会**延迟**到 INFO 处理 + 可选 TLS 之后才起 writeLoop（`leafnode.go:3688`）；非 solicited 立即起 |

### 2.2 唤醒机制：`sync.Cond`，不是 channel

writeLoop 不阻塞在 channel 上，而是阻塞在一个**绑定到 `c.mu` 的条件变量**上：

```go
// client.go:716（initClient，持锁）
c.out.sg = sync.NewCond(&(c.mu))
```

谁唤醒它？**生产者的 readLoop**（不是每个订阅者一个生产者 goroutine——一个 publisher 的 readLoop 扇出信号给多个订阅者的 writeLoop）。唤醒原语极简（`client.go:2045`）：

```go
func (c *client) flushSignal() {
    // Check that sg is not nil, which will happen if the connection is closed.
    if c.out.sg != nil {
        c.out.sg.Signal()
    }
}
```

**完整信号路径**（publisher 发布一条消息 → 订阅者 writeLoop 醒）：

1. publisher 的 readLoop `parse` 出 PUB → `processMsg` → `deliverMsg`（`client.go:3923`）。
2. `deliverMsg` 把消息序列化进订阅者的出站缓冲，**不 syscall**：`client.queueOutbound(mh)` + `client.queueOutbound(msg)`（一条 NATS 消息 = header + body 两个 vector 项，并入同一个 `c.out.nb`）。
3. `addToPCD`（`client.go:3968`）把订阅者记进 publisher 的 `c.pcd`（`map[*client]struct{}`）集合，并 `cp.out.fsp++`（`fsp` = 每生产者待处理 flush 信号数）。
4. 回到 readLoop 顶部，`flushClients(0)`（`client.go:1362`）排空 `c.pcd`：对每个订阅者 `fsp--`，预算内就地 `flushOutbound()`，否则 `flushSignal()` → `sg.Signal()`。
5. 订阅者的 writeLoop 从 `sg.Wait()` 醒来 → `flushOutbound()` → `writev`。

**关键去重（批量的来源）**：`c.pcd` 是集合，`addToPCD` 只在订阅者**不在集合里**时插入。所以一个 publisher 在一次 readLoop pass 里给**同一订阅者**投递 N 条消息，`fsp` 只 +1、`pcd` 里只出现一次 → `flushClients` 只发一次信号。信号数 = **O(不同订阅者)**，不是 O(消息数)。

```go
// client.go:1362-1390（flushClients，readLoop 每轮调用，排空生产者的 pcd 集合）
func (c *client) flushClients(budget time.Duration) time.Time {
    last := time.Now()
    for cp := range c.pcd {
        delete(c.pcd, cp)
        cp.mu.Lock()
        cp.last = last
        cp.out.fsp--                  // 抵消 addToPCD 的 fsp++
        if cp.isClosed() { cp.mu.Unlock(); continue }
        if budget > 0 && cp.out.lft < 2*budget && cp.flushOutbound() {
            budget -= cp.out.lft      // 订阅者便宜 flush → 生产者就地写，省一次 cond 弹跳
        } else {
            cp.flushSignal()          // 否则只发信号，让订阅者自己的 writeLoop 写
        }
        cp.mu.Unlock()
    }
    return last
}
```

### 2.3 写合并：故意"等一等"再 flush（coalescing）

writeLoop 主循环（`client.go:1328-1356`）的核心是一条**故意多等一会儿**的条件：

```go
for {
    c.mu.Lock()
    if closed = c.isClosed(); !closed {
        owtf := c.out.fsp > 0 && c.out.pb < maxBufSize && c.out.fsp < maxFlushPending
        if waitOk && (c.out.pb == 0 || owtf) {
            c.out.sg.Wait()           // 没数据就等；有数据但"还能再攒"也继续等
            closed = c.isClosed()     // Wait 期间释放过锁，醒来复查是否被关
        }
    }
    if closed { ... flushAndClose + closeConnection + reconnect; return }
    waitOk = c.flushOutbound()        // 排空 → 一次 writev
    c.mu.Unlock()
}
```

`owtf`（"OK to wait / 还能攒"）= 还有未消费的生产者信号（`fsp > 0`）**且** pending 字节 < 64KiB（`maxBufSize`，`client.go:113`）**且** 待处理信号 < 10（`maxFlushPending`，`client.go:115`）。

含义：writeLoop **不会**一收到第一个信号就醒着写一条，而是**继续 `sg.Wait()`**，让后续若干生产者的 append 也落进 `c.out.nb`，攒到 10 个信号或 64KiB 才排空。这就是"N 条消息 → 1 次 writev"的合并点。

### 2.4 出站缓冲：池化 `net.Buffers`，append 零 syscall

每连接出站状态（`client.go:349-361`，已逐字核对）：

```go
type outbound struct {
    nb  net.Buffers        // 待发缓冲（writev 向量），每个 []byte 定容（来自池）
    wnb net.Buffers        // flush 工作副本；部分写时残留项留到下一轮
    pb  int64              // 总 pending 字节
    fsp int32              // 每生产者待处理 flush 信号数
    wtp WriteTimeoutPolicy // 写超时策略
    sg  *sync.Cond         // 唤醒 writeLoop
    wdl time.Duration      // 写超时快照
    mp  int64              // MaxPending 快照（每客户端硬上限）
    lft time.Duration      // 上次 flush 耗时（给 flushClients 做预算）
    stc chan struct{}      // 背压 stall chan（扇入生产者 overrun 时用）
    cw  *s2.Writer
}
```

append 唯一入口 `queueOutbound`（`client.go:2501`）：只搬字节、不写 socket——先填满 `nb` 尾部 frame 的剩余容量，再从 `sync.Pool`（512/4096/65536 三档）取新定容 frame 追加：

```go
// client.go:2507-2508
c.out.pb += int64(len(data))    // 先乐观加 pending 字节
// ...顶部填满尾部 frame 的 free 容量 → 剩余部分按 512/4096/65536 从池里取 frame 追加进 c.out.nb
```

池化使分配摊销到接近 0；append 路径完全无 syscall。

### 2.5 排空：一次 `writev`

`flushOutbound`（`client.go:1665-1886`）把 pending 合并成尽量少的 syscall：

1. `collapsePtoNB()` 快照 `(nb, pb)`，随即 `c.out.nb = nil`——**生产者可继续往新 `nb` 追加**，flush 持有的是旧快照。
2. 快照并入工作副本 `c.out.wnb`。
3. **释放 `c.mu`**（写期间不持锁，生产者并发追加不受阻塞）。
4. 循环 `wnb.WriteTo(nc)`，每批截到 ≤ `nbMaxVectorSize`(=1024) 个 buffer（≤64MB）：

```go
// client.go:1786-1788（整条写路径里唯一的 socket 写，即 scatter-gather writev）
nc.SetWriteDeadline(time.Now().Add(wdl))
wn, err = wnb.WriteTo(nc)       // net.Buffers.WriteTo = 一次 writev，覆盖向量里所有 buffer
nc.SetWriteDeadline(time.Time{})
```

`net.Buffers.WriteTo` 在 Linux 走 `writev`——**N 条排队的消息 = 1 次（或少数几次分块的）writev**，不是 N 次 `write`。部分写则剩余项前移到 `wnb` 头部，下一轮继续；已消费的池 frame `nbPoolPut` 归还。写完重新加 `c.mu`，按实际写入字节扣 `pb`。

### 2.6 没有周期性 flush 定时器（重要）

这份 checkout **没有** `setFlushTimer`/`flushTB`/`kick` 之类的周期 flush 定时器（grep 全 `server/` 零命中；唯一 `kick` 是 `filestore.go` 里无关的 bool）。延迟上界完全靠：

1. 每个 `queueOutbound → flushSignal` 路径**立即** `sg.Signal()`；
2. writeLoop 攒到 `maxFlushPending`(10) 个信号或 `maxBufSize`(64KiB) 字节**必须排空**；
3. 生产者 readLoop **每轮** `flushClients` 排空 `pcd`。

注意区分两个 deadline：`wdl`（`opts.WriteDeadline` 快照，`client.go:719`）是**单批 writev 最长多久**（慢客户端保护），**不是**"数据等多久才 flush"。keepalive/RTT 的 ping 计时器是另一回事。

### 2.7 慢客户端背压：64MiB 硬上限 + 75% 软背压

**硬上限**：`MAX_PENDING_SIZE = 64 MiB`（`server/const.go:102`），`opts.MaxPending==0` 时取默认（`server/opts.go:6064`），连接建立时快照进 `c.out.mp`（`client.go:746`）。

**检测在"生产者侧"**（append 时，持锁），不在 writeLoop——`queueOutbound` 乐观加完 `pb` 立即查（`client.go:2541-2557`）：

```go
// client.go:2541-2557（仅 kind==CLIENT 检查字节上限；超限 = 关整个连接，不丢订阅）
if c.kind == CLIENT && c.out.pb > c.out.mp {
    c.out.pb -= int64(len(data))          // 回滚这次乐观加
    atomic.AddInt64(&c.srv.slowConsumers, 1)
    c.srv.scStats.clients.Add(1)
    if c.acc != nil { c.acc.stats.slowConsumers++ }
    c.Noticef("Slow Consumer Detected: MaxPending of %d Exceeded", c.out.mp)
    c.markConnAsClosed(SlowConsumerPendingBytes)   // 直接标记关闭
    return                                          // 这条数据丢弃，不入队
}
```

**决策 = 关整个连接**（全链路没有任何"丢订阅"的分支）。两条互补的触发条件：
- **字节上限**：`pb > mp`（`SlowConsumerPendingBytes`），生产者侧 append 时检测。
- **写超时**：`writev` 超时（`client.go:1840` → `handleWriteTimeout` `1891`），writeLoop 侧检测；CLIENT 一律 `markConnAsClosed(SlowConsumerWriteDeadline)`。

**75% 软背压**（在硬上限之前先减速，`client.go:2563-2565`）：

```go
// client.go:2563-2565（pending 超 75% → 建 stall chan，阻塞扇入生产者）
if c.out.pb > c.out.mp/4*3 && c.out.stc == nil {
    c.out.stc = make(chan struct{})
}
```

`flushOutbound` 把 pending 排到 <75% 或写完整批后 `close(stc)` 释放生产者（`client.go:1875`）。慢客户端原因断连时 `skipFlushOnClose=true`（`client.go:1962`），不再做无谓的最后一次写。

> **bounded-memory 红线对照**：`c.out.pb` 是每客户端排队字节的唯一增长轴；字节上限 + 回滚使其对 CLIENT 严格有界（≤64MiB）。一个慢设备最多吃 64MiB 就被踢，拖不垮其他连接——这正对应你的红线 #4（所有 queue 必须有 bytes 上限）。注意：route/gateway/leaf **不**受这个 `pb/mp` 检查（只走写超时路径 + `WriteTimeoutPolicy`）。

### 2.8 对自建 broker 的启发（闭环第一节与并发取舍讨论）

你定了**不做 per-conn writeLoop**（红线 `goroutine ≤ conn×1.x`），所以 writeLoop 本身不照搬；但里面几个机制对**下行 worker pool**（命令服务 → `downlink.{broker_id}` → broker → 设备）直接有用：

1. **`sync.Cond` / 信号驱动，而非定时轮询**。下行 worker pool 用 cond/channel 信号驱动：命令到来才醒，idle 设备零开销。别给每连接挂 `time.Timer`（红线 #2 禁止）。
2. **出站 pending 硬上限 + 生产者侧检测 + 75% 软背压**。抄 nats 的"乐观加再回滚"检测：下行 inflight 队列（红线 #7 `[4]DownlinkSlot`）写满前先 `stc` 式背压，超限丢弃/断连（红线 #4/#5 已要求）。
3. **写时不持连接锁**。nats `flushOutbound` 释放 `c.mu` 再 `writev`；你的写 worker 写设备 socket 时，该设备的 read goroutine 不受影响（Go `net.Conn` 读写本就独立，且每设备 1 inflight 天然避免帧交错）。
4. **池化 buffer**。下行帧用 `sync.Pool` 复用，对应红线 #3（bounded-memory、先预算后分配）。
5. **写合并（writev）对你价值有限**。nats 靠 writev 合并是因为"少量连接 × 高扇出双向"；你是"海量连接 × 上行重 / 下行稀疏 + 每设备 1 inflight"，下行没什么可合并的——worker pool 对各设备 socket 各写一次即可，不必上 writev。

一句话：**该抄的是"信号驱动 + 有界 pending + 写不持锁 + 池化"这套内存纪律，不是 writeLoop 这个 per-conn goroutine**。这和第一节的结论一致——nats 值得学的是内存纪律（你已写成红线 #2/#3/#4），不是它的 2-goroutine 模型。

### 2.9 完整写路径图

```
publisher readLoop: parse(PUB) → processMsg → deliverMsg   client.go:3923
     │  client.queueOutbound(header) + queueOutbound(body)  ← 入订阅者 c.out.nb，零 syscall
     │  addToPCD: 订阅者加入 publisher.c.pcd，订阅者.out.fsp++   client.go:3968
     └→ readLoop 顶部 flushClients(0) 排空 c.pcd             client.go:1362
           对每个订阅者: fsp--；预算内就地 flushOutbound，否则 flushSignal()
                └→ sg.Signal()                              client.go:2045
subscriber writeLoop (阻塞在 sg.Wait) 醒来                   client.go:1333
     └→ flushOutbound                                       client.go:1665
          快照 nb→wnb，c.out.nb=nil（生产者可继续追加）
          释放 c.mu → wnb.WriteTo(nc)  ← 一次 writev         client.go:1787
          写完重加锁，扣 pb；若写期又有数据 → 再 flushSignal
     ← owtf 条件决定下一轮"继续攒"还是"立即排空"              client.go:1331

慢客户端:
  生产者侧 append: pb > mp(64MiB) → markConnAsClosed + 丢弃   client.go:2541
  75% 软背压: 建 stc 阻塞扇入生产者                           client.go:2563
  写超时: writev 超时 → 关连接                                client.go:1840/1891
```

---

*第二节完。下一节候选：`processInboundClientMsg` / SubList 订阅扇出、route/leaf/gateway 集群转发、MQTT 会话与 QoS1/2 持久化。*
