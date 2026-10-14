# Go GC mark 耗时计算模型

> 本文建立 Go GC mark 阶段耗时的估算模型，让你能自己计算任意连接数 / 任意 Session 结构体改动后的 GC 成本。是 [容量规划 §10 演进路径](../11.cloud-mqtt-gateway-capacity-planning.md) 和 [红线 §4 pointer-poor](../7.cloud-mqtt-gateway-implementation-red-lines.md) 的量化工具。
>
> §2-§7 讲 mark（最大开销阶段），§8 讲完整周期（STW + sweep + 总 CPU 占比），§9 讲压测校准。
>
> 重要前提：**理论估算只给量级（10ms / 100ms / 1000ms），精确值必须靠 `GODEBUG=gctrace=1` 压测校准。** mark 耗时受硬件（cache 命中率、内存带宽）、Go 版本、并发负载影响，同程序不同机器可差 2-3 倍。

---

## 1. 为什么不能精确算

Go GC mark 的耗时受三个不可预测的硬件因素影响：

```text
1. cache 命中率 —— mark 遍历指针图是随机内存访问（非顺序），L3 cache 命中率波动大
2. 内存带宽   —— mark 要读大量内存（指针指向的对象），带宽是瓶颈
3. 并发干扰   —— mark 和应用 goroutine 并发跑，应用占用 CPU 时 mark 变慢
```

所以同一个程序在不同机器/不同负载下，mark 耗时可差 2-3 倍。**精确值只能靠压测**（`go tool trace` / `GODEBUG=gctrace=1`）。但估算模型能给你量级判断。

---

## 2. 核心公式

```text
mark 耗时 ≈ (要扫描的指针总数 × 每指针扫描成本) / 并发 mark worker 数 × 并发干扰系数

简化：T_mark = (N_ptrs × C_per_ptr) / W_workers × F_interference
```

四个变量逐个讲。

---

## 3. 变量 1：要扫描的指针总数（最关键，你能控制）

Go GC 官方文档（[A Guide to the Go GC](https://go.dev/doc/gc/guide)）明确：

> "The average cost of marking and scanning depends on the GC implementation, but also on the behavior of the program. **More pointers means more GC cost.**"

mark 扫描的是**对象里的指针字段**，不是对象的全部内存。

### 3.1 字段类型 vs 是否被扫描

| 字段类型 | 被 GC mark 扫描？ | 扫描单元数 | 说明 |
|---|---|---|---|
| 基础类型（int/uint/float/bool/byte） | ❌ 不扫 | 0 | noscan，GC 跳过整个对象 |
| 指针（`*T`） | ✅ 扫描 | 1 | 跟随指针到目标对象 |
| string | ✅ 扫描 | 1 | string header 含 1 个指向字节数组的指针 |
| slice（`[]T`） | ✅ 扫描 | 1（T 为指针则更多） | slice header 含 1 个指向底层数组的指针 |
| `[]*T`（元素是指针的 slice） | ✅ 扫描 | 1 + len(slice) | header 指针 + 每个元素指针 |
| interface{} | ✅ 扫描 | 2 | 类型指针 + 数据指针 |
| map | ✅ 扫描 | 很多 | buckets/entries 内部大量指针 |
| `[N]T`（固定数组，T 基础类型） | ❌ 不扫 | 0 | noscan 数组，整体跳过 |
| `[N]*T`（固定数组，T 是指针） | ✅ 扫描 | N | 每个元素一个扫描单元 |

**关键洞察**：`[4]DownlinkSlot` 如果 DownlinkSlot 含指针字段，GC 扫描 DownlinkSlot 内部指针；如果整个 DownlinkSlot 都是基础类型（uint64 等），`[4]DownlinkSlot` 是 noscan 数组，**GC 完全不扫**。指针 vs 基础类型是 GC 成本的分水岭。

### 3.2 mqtt-gateway Session 的指针数计算

```go
type Session struct {
    ConnID           uint64    // ❌ noscan
    DeviceNumericID uint64    // ❌ noscan
    TenantNumericID uint32    // ❌ noscan
    SessionVersion  uint64    // ❌ noscan
    LastSeenUnix    int64     // ❌ noscan
    KeepAliveSec    uint16    // ❌ noscan
    Flags           uint16    // ❌ noscan
    PendingBytes    int32     // ❌ noscan
    InflightCount   int8      // ❌ noscan
    Downlink        [4]DownlinkSlot  // 假设 DownlinkSlot 含指针 → 4 个扫描单元
    Filters         string    // ✅ 1 个扫描单元（string 内部指针）
}
// Session 本体指针数 = 4（Downlink）+ 1（Filters）= 5
```

### 3.3 别忘了"背景指针"（Session 之外）

GC 不只扫 Session——**goroutine 栈 + 连接对象 + client 内部对象**也要扫。这些随连接数线性增长：

```text
每连接的"背景指针"（Session 之外）：
  - readLoop goroutine 栈：conn 指针 + 局部 buf 指针 + 临时变量指针 ≈ 4 个
  - net.Conn 内部对象（TCP 连接结构）≈ 2 个
  - TLS 状态对象（cipher context）≈ 1 个
  ────────────
  每连接背景指针 ≈ 7 个
```

### 3.4 每连接总扫描指针

```text
每连接总扫描指针 = Session 本体指针 + 背景指针

当前 Session（未压缩）：
  5（Session 本体）+ 7（背景）= 12 个/连接

压缩后（filterIdx + downlinkPtr 惰性）：
  1（Session 本体，downlinkPtr 99% nil）+ 7（背景不变）= 8 个/连接
```

> **注意**：压缩只降了 Session 本体（5→1），背景指针（7）不变。所以压缩的收益不是"降 80%"（那只看 Session 本体），含背景后实际降 ~33%（12→8）。

---

## 4. 变量 2：每指针扫描成本（硬件相关）

mark 一个指针的步骤：读指针值 → 跟随指针找到目标对象 → 检查对象着色 → 若非黑色则入队。这是**随机内存访问**（指针指向的对象在堆里随机分布）。

| cache 状态 | 每指针成本 | 适用场景 |
|---|---|---|
| L3 cache 命中 | 10-40 ns | live heap < L3（小规模，如 1 万连接） |
| **混合（经验值）** | **50-150 ns** | live heap > L3（mqtt-gateway 百万连接场景） |
| L3 cache miss（主存） | 100-300 ns | live heap 远超 L3 + 访问模式散乱 |

**用混合估算 ~100ns/指针**（业界经验值，适用于 live heap > L3 cache 的场景——50 万连接的堆 ~10GB，远超 L3 的 32-64MB）。

---

## 5. 变量 3：并发 mark worker 数

Go GC mark 用并发 worker goroutine（不 STW）。worker 组成（[runtime/mgc.go](https://github.com/golang/go/blob/master/src/runtime/mgc.go)）：

```text
dedicated worker：GOMAXPROCS / 4 个，全职 mark
fractional worker：部分时间 mark
idle worker：P 空闲时帮忙 mark
assist：应用 goroutine 分配过快时被迫帮忙 mark

有效并发 mark CPU 占比 ≈ GOMAXPROCS × 25%（Go GC goal）
有效 worker 数 ≈ GOMAXPROCS × 0.25
```

| GOMAXPROCS | 有效 mark worker 数 |
|---|---|
| 16 核 | 4 |
| **32 核** | **8** |
| 64 核 | 16 |

---

## 6. 变量 4：并发干扰系数

mark 和应用并发跑，互相干扰。干扰系数修正理论值到实际值：

| 负载状态 | F_interference | 说明 |
|---|---|---|
| 理想（mark 独占，无应用干扰） | 1.0 | 不现实，mark 总是并发的 |
| **正常（应用并发，中负载）** | **1.5-2.0** | mqtt-gateway 稳态 |
| 高负载（应用抢 CPU，日出握手） | 2.5-3.0 | 日出风暴期间 mark 变慢 |

---

## 7. 完整公式与代入示例

```text
T_mark ≈ (N_conn × P_per_conn × C_per_ptr) / W_workers × F_interference

  N_conn         = 连接数
  P_per_conn     = 每连接总扫描指针数（Session 本体 + 背景）
                   mqtt-gateway 当前 ≈ 12，压缩后 ≈ 8
  C_per_ptr      = 每指针扫描成本（ns）
                   乐观 = 50，混合 = 100，保守 = 200
  W_workers      = GC mark worker 数 ≈ GOMAXPROCS × 0.25
                   16 核 → 4，32 核 → 8
  F_interference = 并发干扰系数
                   正常 = 1.5-2.0，高负载 = 2.5-3.0
```

### 示例 A：25 万连接，32 核，当前 Session（未压缩）

```text
N_conn=250000, P_per_conn=12, C_per_ptr=100, W_workers=8, F=1.5

T_mark = (250000 × 12 × 100) / 8 × 1.5
       = 3亿 / 8 × 1.5
       = 3750万 × 1.5
       = 5625万 ns
       ≈ 56ms（纯 mark wall-clock）

GC CPU 占比 = mark 耗时 / GC 周期间隔
  GC 周期间隔 ≈ live_heap × GOGC / 分配速率
  稳态每 5 分钟上报一次，分配速率低 → GC 周期较长
  估算 GC CPU ≈ 5-10%（25 万连接）
```

### 示例 B：50 万连接，32 核，当前 Session（未压缩）

```text
N_conn=500000, P_per_conn=12, C_per_ptr=100, W_workers=8, F=2.0

T_mark = (500000 × 12 × 100) / 8 × 2.0
       = 6亿 / 8 × 2.0
       = 7500万 × 2.0
       = 1.5亿 ns
       ≈ 150ms

GC CPU 占比 ≈ 15-20%（50 万连接，超 15% 舒适线）
```

### 示例 C：50 万连接，32 核，Session 压缩后

```text
N_conn=500000, P_per_conn=8, C_per_ptr=100, W_workers=8, F=2.0

T_mark = (500000 × 8 × 100) / 8 × 2.0
       = 4亿 / 8 × 2.0
       = 5000万 × 2.0
       = 1亿 ns
       ≈ 100ms

GC CPU 占比 ≈ 10-13%（压缩后回到舒适线内）
```

### 三例对比

| 场景 | 指针总数 | mark 耗时 | GC CPU | 判定 |
|---|---|---|---|---|
| 25 万 / 当前 Session | 300 万 | ~56ms | 5-10% | ✅ 舒适 |
| 50 万 / 当前 Session | 600 万 | ~150ms | 15-20% | ⚠️ 超舒适线 |
| 50 万 / 压缩后 | 400 万 | ~100ms | 10-13% | ✅ 回到舒适线 |

> mark 时长公式回答的是"GC CPU 占多少"（单轮 mark 耗时 → GC CPU 比例）。但判断"会不会 STW >1ms"和"优化该减分配还是减 live 指针"，要换 Pacer 视角——算**吞吐率对比**。见 §7.1。

---

## 7.1 Pacer 视角：mark 吞吐率 vs 分配吞吐率（判断 STW + 优化方向）

> 比"mark 150ms"更本质的算法：直接算"每秒能 mark 多少指针"vs"每秒分配产生多少指针"。**这是 Pacer 的视角——速率对速率，胜负一目了然。**

### 7.1.1 mark 吞吐率（GC 能力上限）

```text
mark 吞吐率 = GC 专用的 CPU 核数 / 每指针扫描成本

  GC 专用 CPU = GOMAXPROCS × 25%（Go GC goal，含 worker + assist）
  每指针成本 = 100ns（混合 cache，§4）

32 核机器：
  mark 能力 = (32 × 25%) / 100ns
            = 8 核 / 1e-7 s
            = 8000 万指针/s

16 核机器：
  mark 能力 = 4 / 100ns = 4000 万指针/s

64 核机器：
  mark 能力 = 16 / 100ns = 1.6 亿指针/s
```

### 7.1.2 分配吞吐率（GC 负载来源）

```text
分配指针速率 = 对象分配速率 × 每对象平均指针数

mqtt-gateway 稳态（中低上报频率，每 5 分钟一次）：
  对象分配 = 每连接每 5 分钟 ~10 个临时对象（payload buf + 解析临时）
  50 万连接 → 50万 × 10 / 300s = ~16000 对象/s
  每对象指针 ~3（payload slice header 1 + 内部 1-2）
  → 分配指针速率 = 16000 × 3 = ~48000 指针/s

日出风暴（百万 CONNECT 10 分钟集中上线）：
  CONNECT 速率 = 100万 / 600s = ~1667/s
  每 CONNECT ~20 对象（session + auth temp + conn wrapper）
  → ~33300 对象/s
  每对象指针 ~5（session 5 指针 + temp）
  → 分配指针速率 = 33300 × 5 = ~166500 指针/s
```

### 7.1.3 速率对比——STW 风险一目了然

| 场景 | 分配指针速率 | mark 能力（32 核） | 分配/mark | STW 风险 |
|---|---|---|---|---|
| 稳态 50 万连接 | 48,000/s | 8000 万/s | **0.06%** | ✅ 极低 |
| 日出风暴 | 166,500/s | 8000 万/s | **0.2%** | ✅ 低 |
| 极端假设（100 万指针/s 分配） | 1,000,000/s | 8000 万/s | 1.25% | ✅ 仍低 |
| 超极端（5000 万指针/s） | 50,000,000/s | 8000 万/s | 62.5% | ⚠️ assist 暴增 |

```text
结论：mqtt-gateway 的分配指针速率远低于 mark 能力——
  日出风暴（最密集场景）也只有 mark 能力的 0.2%。
  → Pacer 不会触发 assist（mark worker 绰绰有余）
  → gcMarkDone 灰色队列每次清空
  → mark termination 残留 ~0
  → STW ~0.1ms，>1ms 概率极低
```

### 7.1.4 那 GC 成本在哪——在"live heap 重扫"，不在"分配增量"

```text
GC 的 mark 工作分两部分：
  ① 每轮重扫全部 live 指针（固定成本，随连接数线性增长）
     = 50万 Session × 12 指针 + 背景 350 万 = ~950 万指针/轮
     耗时 = 950万 / 8000万/s = 120ms（这就是 mark wall-clock）
     → 这是 GC CPU 占比 10-13% 的来源

  ② 本周期新分配指针（随分配速率变化，由 Pacer 管控）
     = 分配指针速率 × GC 周期时长
     但这部分在 concurrent marking 期间即时处理（写屏障 + assist），不攒到下轮
     → 在 mqtt-gateway 场景下（分配速率 << mark 能力），这部分几乎不增加成本

关键洞察：
  mqtt-gateway 的 GC 瓶颈是 ①（live heap 大，600-950 万指针要每轮重扫）
  不是 ②（分配增量，被 Pacer 轻松消化）
```

### 7.1.5 优化优先级修正（基于 Pacer 视角）

| 优化方向 | 减什么 | 对 ① live heap 重扫 | 对 ② 分配增量 | 实际 GC 收益 |
|---|---|---|---|---|
| **Session 指针压缩**（filterIdx + downlinkPtr） | 减 live 指针数 | ✅ **直接降**（950万→400万） | 无关 | **GC CPU 降 40%** |
| payload sync.Pool 池化 | 减 transient 分配 | 无关 | 降（但 ② 本就不是瓶颈） | **GC CPU 降 <2%** |
| GOGC=200 | 降 GC 频率 | 间接（GC 周期拉长，CPU 占比降） | 无关 | GC CPU 降 ~30% |
| 加 CPU 核数 | 加 mark 能力 | mark wall-clock 降 | Pacer 余量更大 | mark 时长线性降 |

```text
修正后的优化优先级（Pacer 视角）：
  1. Session 指针压缩 ← 唯一能直接降 GC CPU 占比的方法（减 live 重扫）
  2. GOGC=200 ← 降 GC 频率（间接降 CPU 占比）
  3. payload 池化 ← 对 mqtt-gateway 几乎无 GC 收益（分配增量本就不是瓶颈）
     （池化的价值在"减 alloc 次数 + 减 GC 频率"，但 mqtt-gateway 分配速率本就低）
  4. 加 CPU 核数 ← mark 时长线性降，但成本最高

注意：payload 池化（TODOS #6）在 NATS 报告里论证的是"writev chunked-copy 高频场景"
的收益。mqtt-gateway 不是 writev 场景，分配速率远低于 mark 能力，
池化对 GC 的收益微乎其微。但池化对"减 alloc 次数 → 减 syscall → 减 GC 触发频率"
仍有边际收益，只是不是"减 GC CPU 占比"的直接杠杆。
```

---

## 7.2 核心概念：快进快出 vs 快进慢出（理解 mqtt-gateway GC 行为的钥匙）

> §7.1 说"分配速率不影响 mqtt-gateway 的 GC"，§7.1.5 说"池化收益低、Session 压缩收益高"。本节解释**为什么**——核心是对象的生命周期分类。

### 7.2.1 两种对象生命周期

```text
快进快出（transient，短命）：
  对象分配 → 用一下 → 立即变垃圾 → 下轮 GC 回收
  特征：生命周期 << GC 周期
  例子：payload buf、MQTT 解析临时、auth 临时、Kafka record 临时

快进慢出（long-lived，长存）：
  对象分配 → 一直存活 → 跨多个 GC 周期 → 每轮 GC 都重新 mark
  特征：生命周期 >> GC 周期
  例子：Session（连接存活几小时到几天）、net.Conn 内部、TLS 状态
```

### 7.2.2 GC 对两类对象的不同处理

```text
mark 阶段只扫"可达对象"（有指针指向的）：

快进快出对象：
  分配时进 heap → 用完变垃圾（没人指向它）→ 不可达
  → mark 跳过它（不可达对象不进 mark 队列）
  → 不增加 mark 成本
  → GC 只在 sweep 阶段回收它的内存（sweep 是惰性分摊，成本低）

快进慢出对象：
  持续可达（被 Session map / conn 等持有）→ 每轮 mark 都扫
  → mark 成本 = 指针数 × 每指针扫描成本
  → 这是 GC CPU 占比的主要来源
```

### 7.2.3 mqtt-gateway 的对象分类

```text
快进慢出（长存，每轮 GC 重扫）—— GC 成本 99% 来自这里：
  Session × 50 万（连接存活几小时到几天）         → ~250 万指针
  net.Conn 内部对象 × 50 万                       → ~100 万指针
  TLS 状态 × 50 万                                → ~50 万指针
  goroutine 栈里的指针 × 50 万                     → ~200 万指针
  ────────────────────
  ~600-950 万指针/轮（每轮固定，不随分配变化）
  → 这是 GC CPU 10-15% 的来源

快进快出（短命，不进 mark 重扫）—— 对 GC mark 成本 ~0 贡献：
  payload buf（收到 → Kafka ack → 变垃圾，秒级）
  MQTT 解析临时（解析完 → 变垃圾，毫秒级）
  auth 临时（认证完 → 变垃圾，毫秒级）
  Kafka record 临时（produce → 变垃圾，毫秒级）
  → 变垃圾后不可达，mark 跳过
  → 只增加 alloc + sweep 成本（sweep 惰性，成本低）
```

### 7.2.4 为什么分配速率不影响 mqtt-gateway 的 GC

```text
mqtt-gateway 的分配速率（日出风暴 ~50000 对象/s）不算低——和普通 Web API 持平。
但分配的对象全是"快进快出"（payload/auth/Kafka 临时），秒级变垃圾。

mark 只扫可达对象 → 快进快出对象变垃圾后不可达 → mark 跳过
→ 分配速率再高，mark 工作量也不增长（被 Session 固定成本主导）

GC 成本 = 快进慢出重扫（固定 ~950 万指针）+ 快进快出 mark（~0）
       ≈ 固定值，不随分配速率变化

什么阈值下分配速率才开始影响 GC：
  快进快出对象存活时间 > GC 周期 → 变成"快进慢出"
  例：Kafka produce 极慢（5s+）→ payload 存活 5s > GC 周期
      → payload 在 mark 期间仍可达 → 进 mark → 分配速率开始影响 GC
  mqtt-gateway 当前：payload 存活秒级 << GC 周期 24 分钟 → 不受影响
```

### 7.2.5 为什么池化收益低、Session 压缩收益高

```text
payload 池化（TODOS #6）：
  payload 是快进快出 → 变垃圾后 mark 跳过 → 不进 mark 重扫
  → 池化（减分配）只减 alloc + sweep 成本（惰性，低）
  → 对 GC CPU 占比贡献 <2%
  → 池化的边际价值在"减 alloc 次数 → 减 GC 触发频率"，不是"减 mark CPU"

Session 压缩（filterIdx + downlinkPtr 惰性）：
  Session 是快进慢出 → 每轮 mark 重扫 → mark 成本的主要来源
  → 压缩（减指针）直接减 mark 工作量（950 万 → 400 万）
  → GC CPU 降 ~40%
  → 这才是 GC CPU 占比的直接杠杆
```

### 7.2.6 sync.Pool 与快进快出的关系

```text
sync.Pool 的 GC 行为：
  每次 GC 时 poolCleanup 清空 sync.Pool → 空闲对象变不可达 → mark 跳过
  → sync.Pool 里的空闲对象不进 mark 重扫，不增加 mark 成本

这意味着：
  ① sync.Pool 变大不增加 mark 成本（GC 清空它）
  ② Kafka ack 慢时在途 payload（Get 了还没 Put）不在 pool 里，在 job/Kafka-batch 里
     它们的 mark 成本（即使堆积 8 万个）也只有 Session 950 万的 0.9%，微不足道
  ③ sync.Pool 的代价不是"增加 mark"，是"GC 后命中率降低"——pool 退化成每次 New

注意：自定义池（[]*Buffer 不 GC 清空）会堆积 → 增加 mark 成本 → 不要用。
mqtt-gateway 用 sync.Pool 不存在 pool 堆积的 mark 风险。
```

### 7.2.7 一句话总结

```text
mqtt-gateway 是"快进慢出"主导的 GC 模型：
  - Session 等长存对象（快进慢出）贡献 99% 的 GC mark 成本（950 万指针/轮）
  - payload 等短命对象（快进快出）贡献 ~0 的 mark 成本（变垃圾后不可达）

推论：
  - 分配速率不影响 GC（快进快出不进 mark），即使日出风暴 50000 对象/s
  - 池化收益低（减快进快出的 alloc，不减排 mark 重扫）
  - Session 压缩收益高（减快进慢出的 mark 重扫，GC CPU 降 40%）
  - sync.Pool 不增加 mark 成本（GC 清空它），只可能降低命中率
```

---

## 8. 完整 GC 周期成本：STW + sweep + 总 CPU 占比

> §2-§7 只算了 mark（最大开销）。但 Go GC 周期还有 STW 暂停和 sweep 阶段。本节把完整周期的成本补全，并算出"GC 占运行时间的比例"。

### 8.1 Go GC 周期的五个阶段

```text
┌─────────────────────────────────────────────────────────────┐
│  GC 周期（一次完整的垃圾回收）                                │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ① Sweep Termination（STW）   ~10-100μs   清理上一轮状态     │
│  ② Mark Setup（STW）          ~10-100μs   开启写屏障         │
│  ③ Concurrent Marking          §2-§7 算的   并发标记（最大开销）│
│  ④ Mark Termination（STW）     ~10-500μs   收尾标记 + 关写屏障 │
│  ⑤ Concurrent Sweeping         见 8.3       并发清扫（惰性）   │
│                                                             │
│  ①②④ 是 STW（全停），③⑤ 是并发（应用继续跑）                  │
└─────────────────────────────────────────────────────────────┘
```

### 8.2 STW 暂停（mark setup + mark termination）

> **重要修正（2026-06-22）**：早期版本说"STW <1ms 可忽略"——这漏算了 mark termination 扫描 goroutine 栈的工作量。对 mqtt-gateway 这种 50 万 goroutine 的长连接服务，栈扫描是 mark termination 的主要工作，STW 可达 ~20ms 量级。本节重写。

Go 官方设计目标（[golang-dev: sub-millisecond GC pauses](https://groups.google.com/g/golang-dev/c/Ab1sFeoZg_8)）：**STW < 100μs（0.1ms）**——但这是针对"少量 goroutine + 小栈"的通用 Web 服务。mqtt-gateway 50 万 goroutine 的长连接场景不在此假设内。

| STW 阶段 | 做什么 | 通用服务（少量 goroutine） | mqtt-gateway（50 万 goroutine） |
|---|---|---|---|
| Sweep Termination | 清理上一轮 sweep 状态 | ~10-50μs | ~10-50μs（不随连接数增长） |
| Mark Setup | 开写屏障、扫全局变量、收缩部分栈 | ~10-50μs | ~0.1-1ms（全局变量少，但栈收缩多） |
| **Mark Termination** | **扫剩余 goroutine 栈 + 寄存器 + 一致性确认** | ~10-100μs | **~5-30ms** ★ 见 §8.2.2 |

**mark termination 不是"收尾 0.1% 残留"**——它要扫描 concurrent mark 没来得及扫的 goroutine 栈。对 50 万 goroutine 的长连接服务，这是 STW 的主要工作。

### 8.2.1 根对象与栈扫描机制（mark termination 的真正工作）

Go GC 的根对象（mark 起点）四类：

```text
① 全局变量（指向堆的全局指针）—— mark setup 阶段扫（少量，快）
② goroutine 栈 —— ★ 核心问题在这里
③ 寄存器 —— mark termination 扫（只有停了线程才能读寄存器）
④ finalizer / runtime 内部结构 —— 少量
```

**goroutine 栈扫描的分阶段**：

```text
Go 不会把所有栈都堆到 termination——有优化分摊：

mark setup（STW）：
  扫全局变量 + 收缩能收缩的 goroutine 栈（收缩时顺带扫描）

concurrent marking（并发，~100ms）：
  异步抢占 goroutine（Go 1.14+ async preemption）扫描栈
  阻塞在 conn.Read 的 goroutine 也能被信号抢占扫栈
  → 大部分栈在 concurrent 阶段扫完

mark termination（STW）：
  只扫"concurrent 期间没来得及扫的"栈残留 + 寄存器
  → 残留多少取决于 concurrent 窗口够不够扫完所有栈
```

### 8.2.2 mqtt-gateway 的栈扫描残留计算

```text
50 万 readLoop goroutine，每个栈深度 ~4-8 帧
每帧 ~3-5 个指针（conn/buf/session/pkt/临时变量）
每 goroutine 栈指针 ~15-40，取平均 ~30

总栈指针 = 50 万 × 30 = 1500 万指针

concurrent marking 的栈扫描能力：
  concurrent 窗口 ~100ms（live heap mark 占据的 wall-clock）
  栈扫描用 mark worker（8 个，和 heap mark 抢 worker）
  实际用于栈扫描的 worker 比例 ~50%（另一半扫 heap）
  → 栈扫描能力 = 100ms × 4 worker / 100ns = 400 万指针 = ~13 万 goroutine

残留到 termination 的栈 = 50万 - 13万 = 37 万 goroutine
termination 扫 37 万 goroutine × 30 指针 = 1110 万指针

termination 是 STW，全部 32 核并行扫根：
  termination STW = 1110 万 × 100ns / 32 核 = 3.5 秒？

不对——这里有修正：
  Go 1.14+ 的 async preemption 效率比估算高（信号抢占几乎是即时的）
  concurrent 阶段实际能扫掉 70-90% 的栈（不是 26%）
  残留降到 10-30% × 50 万 = 5-15 万 goroutine
  termination 扫 5-15 万 × 30 指针 = 150-450 万指针
  termination STW = 150-450 万 × 100ns / 32 核 = 5-14ms
```

**mark termination STW ≈ 5-15ms**（稳态，32 核，async preemption 高效时）。

```text
日出风暴时更糟：
  goroutine 数短时从 0 涨到 50 万（CONNECT 创建 readLoop）
  新 goroutine 的栈还没被扫过
  + GC 周期缩短（分配暴增）→ concurrent 窗口更短 → 残留更多
  → termination STW 可能 ~15-30ms

最坏情况（async preemption 失效或 concurrent 窗口极短）：
  大部分栈残留 → termination STW ~30-50ms
  → 这是需要压测观测的真实风险
```

### 8.2.3 STW 估算修正

```text
mark setup STW：~0.1-1ms（全局变量少 + 部分栈收缩）
mark termination STW：~5-15ms（稳态）/ ~15-30ms（日出风暴）

STW 总计：~5-15ms（稳态）/ ~15-30ms（日出风暴）

修正前说「<1ms 可忽略」是错的——
  漏算了 50 万 goroutine 栈扫描在 mark termination 的工作量。
  STW ~5-15ms 对 MQTT 秒级延迟可接受，但不是"可忽略"，
  是 p99 延迟抖动的一个真实来源。
```

### 8.2.4 mark termination STW 的两个来源区分

mark termination 的 STW 由两部分工作构成，要分开理解：

```text
来源 A：goroutine 栈扫描残留（§8.2.2 算的）
  = concurrent 没扫完的栈
  50 万 goroutine × 残留比例 × 30 指针 / 32 核
  稳态 ~5-15ms，日出 ~15-30ms
  ★ 这是 mqtt-gateway STW 的主要来源

来源 B：灰色对象队列残留（§8.2.5 原来讲的）
  = concurrent mark 没扫完的堆对象（应用分配快，mark 追不上）
  mqtt-gateway 分配速率低（日出 16.7 万指针/s << mark 能力 8000 万/s）
  → Pacer 不触发 assist → 灰色队列清空 → 这部分残留 ~0
  → 来源 B 对 mqtt-gateway 可忽略

结论：mqtt-gateway 的 mark termination STW 主要来自来源 A（栈扫描），
不是来源 B（灰色对象残留）。这和"分配速率低"不矛盾——
分配速率低解决的是来源 B，但解决不了来源 A（栈数量固定）。
```

### 8.2.5 早期灰色对象残留分析（保留，但对 mqtt-gateway 影响小）

> 以下分析 mark termination 的另一个 STW 来源（灰色对象残留）。对 mqtt-gateway 这种分配速率低的服务，这部分可忽略——但保留分析供其他场景参考。

Go 的 mark 是两段式——concurrent marking（并发，处理 99%+ 对象）先跑，mark termination（STW）收尾。如果 concurrent marking 退出时**灰色工作队列还有残留对象**（应用在 mark 期间修改指针过快，mark worker + mark assist 没追上），这些残留全部在 mark termination（STW）里处理 → STW 变长。

```text
灰色对象残留的 STW（来源 B，对 mqtt-gateway 可忽略）：
  正常（concurrent marking 清空灰色队列，残留 ~0.1%）：
    termination STW ~0.01-0.1ms
  残留较多（应用分配快，灰色队列退出时还有 ~1-10% 残留）：
    termination STW ~0.1-1ms
  灾难（mark assist 压不住，残留 >10%）：
    termination STW >1ms

mqtt-gateway 场景（分配速率 << mark 能力，Pacer 不触发 assist）：
  灰色对象残留 ~0 → 这部分 STW ~0.01ms ✅ 可忽略
  真正的 STW 来自栈扫描（§8.2.2），不是灰色残留
```

**STW 不是 mqtt-gateway 的 GC CPU 瓶颈**（CPU 瓶颈是 concurrent mark 的 live heap 重扫），**但 mark termination 的栈扫描 STW（~5-15ms）是 p99 延迟抖动的真实来源**，不能笼统说"可忽略"。

### 8.2.1 mark 时长能预测 STW 吗——不能直接预测，但是风险放大器

> 常见疑问：「我们算出 concurrent marking 是 150ms，能不能据此判断 STW 是否 >1ms？」
> 答案：**不能直接判断**。但 150ms 能评估"STW 风险的放大程度"。真正判断 STW 要看 **mark assist CPU**。

**为什么 mark 时长不能直接预测 STW**

STW >1ms 的唯一直接原因：mark termination 时灰色队列残留对象多。残留多少取决于**mark 速率 vs 分配速率的博弈**，不取决于 mark 时长本身：

```text
同样 150ms 的 mark：
  分配速率低（mqtt-gateway 稳态）→ mark 从容，残留 ~0 → STW ~0.1ms
  分配速率高（日出风暴）→ mark 被拖住，残留多 → STW 可能 >1ms

→ 同样 150ms，STW 可以是 0.1ms 也可以是 5ms，取决于分配速率
```

**mark 时长是 STW 风险的"放大器"**

```
mark 窗口越长 → 窗口内分配总量越多 → mark 要追的对象越多 → 残留风险越高
  50ms 窗口 × 分配速率 R → 窗口内 R×50ms 的对象
  150ms 窗口 × 分配速率 R → 窗口内 R×150ms 的对象（3 倍）

→ mark 越长不是"直接造成" STW 长，是"放大了"分配速率对 STW 的影响
```

**真正判断 STW >1ms 的方法：看 gctrace 的 assist CPU**

```text
gctrace 输出：gc 42 @5.234s 2%: 0.015+58+0.023 ms clock, 0.12+29/57/86+0.18 ms cpu
                                                            ↑      ↑  ↑
                                                       assist  dedicated  idle
                                                       (应用被强制帮忙) (后台 worker) (空闲 worker)

三个 mark CPU 分量：
  29 = assist CPU（应用 goroutine 分配过快被 Pacer 强制帮忙 mark）
  57 = dedicated + fractional worker CPU
  86 = idle worker CPU（P 空闲时帮忙）

assist 占比 = assist / (assist + dedicated + idle) = 29 / 172 = 17%
  assist < 20%：mark 从容，STW 风险低
  assist 20-40%：mark 有压力，STW 偶发风险
  assist > 40%：mark 严重落后，STW >1ms 概率高
```

**Pacer 机制补充（精确化）**：

```text
Pacer 的核心计算（单次 GC 周期内持续跑）：
  比较两个速率：
    V_mark = mark worker + assist 的指针扫描速率
    V_alloc = 应用分配速率 × 分配对象的指针密度

  V_mark ≥ V_alloc → mark 追得上 → gcMarkDone 灰色队列清空 → termination STW ~0.1ms
  V_mark < V_alloc → mark 落后 → 触发 assist：
    每个 goroutine 分配时有"GC 债务"（gcAssistBytes）
    债务 > 0 → 该 goroutine 被强制 assist（在分配点暂停，先做 mark）
    assist 强度由 Pacer 动态算：让总 GC CPU ≈ 25% goal

  最坏情况（堆远超 GC goal，mark 实在追不上）：
    Go 强制结束 concurrent marking（灰色队列不空也进 termination）
    → 残留全在 STW 处理 → STW 可能 >1ms

注意：抢占不是跨 GC 周期累积的，是单次周期内 assist 债务过高时触发。
Go 不会"主动提前进入 termination"，是"被迫结束 concurrent marking"。
```

**mqtt-gateway 的 STW 风险评估**：

```text
稳态（中低上报频率，每 5 分钟一条消息）：
  分配速率极低（每连接每 5 分钟 ~10 个临时对象）
  mark 150ms 窗口内分配量 ~2400 对象（微量）
  → mark 从容完成，assist ~0 → STW ~0.1ms ✅ 风险极低

日出风暴（百万 CONNECT 10 分钟集中上线）：
  分配速率暴增（每 CONNECT ~20 对象 × 百万 / 600s = ~33000 对象/s）
  mark 150ms 窗口内分配量 ~5000 对象（绝对量仍可控）
  → mark 能追上，但 GC 周期间隔骤降（分配暴增触发频繁 GC）
  → STW 可能 ~0.5ms，真正风险是 GC CPU 占比（assist 暴增到 20-25%）
  → STW >1ms 概率低，但需压测验证（看 gctrace assist CPU）

结论：
  150ms 的 mark 估算不能直接判断 STW，但告诉我们：
  - 稳态 STW 风险极低（分配少，mark 从容）
  - 日出风暴真正要关注的是 GC CPU 占比（assist），不是 STW
  - 验证方法：压测看 gctrace 的 assist CPU（>40% 才是 STW 高风险信号）
```

### 8.3 Sweep 阶段（并发、非阻塞、惰性）

Go 的 sweep **不是 STW，也不是单独的并发阶段**——它是**惰性的，分摊在应用分配时**。

```text
Sweep 机制：
  Mark Termination 结束后，所有内存 span 标记为 "needs sweeping"
  应用分配内存时（mallocgc），先 sweep 掉分配所在 span 的垃圾，再分配
  → sweep 成本分摊到每个分配操作上
  → 没有独立的 "sweep 暂停"
```

**Sweep 的成本模型**：

```text
Sweep 成本 ≈ 0（作为独立阶段）
  因为它不独立运行，而是嵌入在每次分配里

Sweep 对吞吐的影响：
  每次分配多了一个 "先 sweep span" 的操作（~几十 ns）
  50 万连接 × 每连接每 5 分钟分配 ~10 个对象 = 每秒 ~16000 分配
  sweep 额外开销 = 16000 × ~30ns = ~0.5ms/s ≈ 0.05% CPU

结论：sweep 成本可忽略（<0.1% CPU），不是 mqtt-gateway 的 GC 瓶颈
```

> sweep 成本低的另一个原因：Go 的 noscan 对象（基础类型数组，如 `[]byte` payload）sweep 时只改 span 的 bitmap，不遍历内容。mqtt-gateway 的 payload 是 noscan，sweep 快。

### 8.4 完整 GC 周期总成本汇总

| 阶段 | 耗时 | 是否 STW | CPU 占比 | mqtt-gateway 50 万连接估算 |
|---|---|---|---|---|
| Sweep Termination | ~10-50μs | ✅ STW | ~0% | ~0.05ms |
| Mark Setup | ~10-50μs | ✅ STW | ~0% | ~0.05ms |
| **Concurrent Marking** | **§2-§7 算的** | ❌ 并发 | **10-20%** | **~100-150ms wall-clock** |
| Mark Termination | ~100-500μs | ✅ STW | ~0% | ~0.1-0.5ms |
| Concurrent Sweeping | ~0（惰性分摊） | ❌ 并发 | ~0.1% | ~0（嵌入分配） |
| **STW 总计** | — | — | — | **~0.2-0.6ms** |
| **并发开销总计** | — | — | — | **mark ~100-150ms 占 CPU 10-20%** |

**关键结论**：

```text
STW（全停）极短（<1ms），不是 mqtt-gateway 的 GC 瓶颈。
sweep（清扫）是惰性分摊，成本可忽略（<0.1% CPU）。
真正的 GC 成本全在 concurrent marking——它不停应用，但占 10-20% CPU。
```

### 8.5 GC 占运行时间的比例（GC CPU fraction）

这是最重要的运维指标（红线 §13 的 `go_gc_cpu_fraction`）。

```text
GC CPU 占比 = GC 总 CPU 时间 / 总运行时间

一次 GC 周期的 GC CPU 时间：
  = mark CPU（worker 数 × mark wall-clock）
  + STW CPU（全核 × STW 总时长，但 STW 太短可忽略）
  + sweep CPU（惰性分摊，~0.1%）

  ≈ mark CPU
  = W_workers × T_mark
  = 8 worker × 100ms（50 万压缩后）
  = 800ms CPU（但这是 wall-clock 100ms 内的 8 核并行 CPU）
  = 0.8 核秒

GC 周期间隔（两次 GC 之间隔多久）：
  = live_heap_growth / 分配速率
  = (GOGC × live_heap) / alloc_rate

  mqtt-gateway 稳态：
    live_heap ≈ 11.5GB（50 万 Session + 背景）
    GOGC=200 → 下次 GC 在 heap 增长到 34.5GB 时触发
    但 mqtt-gateway 不是堆膨胀型应用（Session 长存，payload 短命）
    → live heap 增长主要来自 payload transient + goroutine 栈波动
    → alloc_rate ≈ 16000 对象/s × ~1KB = 16MB/s
    → heap 从 11.5GB 涨到 34.5GB 需 23GB / 16MB/s = 1437s ≈ 24 分钟
    → GC 周期间隔 ≈ 24 分钟（稳态）

  日出风暴：
    alloc_rate 暴增（百万 CONNECT 分配 session + auth + conn 对象）
    → GC 周期间隔骤降到几秒甚至更短
```

**GC CPU 占比计算**：

```text
GC CPU 占比 = GC CPU 时间 / (GC 周期间隔 × GOMAXPROCS)

稳态（50 万连接，压缩后）：
  = (8 worker × 100ms) / (1437s × 32 核)
  = 0.8 核秒 / 45984 核秒
  = 0.0017%
  → 几乎为零（因为 GC 周期间隔太长，24 分钟才一次）

日出风暴：
  GC 周期间隔降到 ~10s（分配暴增）
  = (8 worker × 150ms) / (10s × 32 核)
  = 1.2 核秒 / 320 核秒
  = 0.375%
  → 仍很低

但实际观测（GODEBUG gctrace 的第二个字段 "2%"）通常显示 5-15%：
  因为 "GC CPU 占比" 不等于 "单次 GC 周期内的占比"
  gctrace 显示的是"自程序启动以来 GC 占的总 CPU 比例"
  包含：mark worker + mark assist（应用分配触发辅助 mark）
```

**关键修正**——Go GC 的 `gctrace` 第二个字段（如 `2%`）是**累计 GC CPU 占比**，包含：

```text
gctrace "2%" 的构成：
  1. dedicated/fractional mark worker 的 CPU
  2. mark assist（应用 goroutine 分配过快被强制辅助 mark）
  3. sweep assist（惰性 sweep 分摊到分配）
  4. 写屏障开销（mark 期间每次指针写多一次屏障检查）

mark assist 是隐藏成本：
  日出风暴分配快 → Go 强制应用 goroutine 帮忙 mark → 应用变慢
  这部分 CPU 不在 "mark worker" 里，但在 "GC CPU" 里
  → 实际 GC CPU 比 (W × T_mark)/(周期×核) 高
```

**所以 mqtt-gateway 的实际 GC CPU 占比**：

| 场景 | mark worker CPU | mark assist（估算） | sweep + 屏障 | **总 GC CPU** |
|---|---|---|---|---|
| 25 万连接稳态 | 3-5% | 1-3% | ~1% | **5-10%** |
| 50 万连接稳态（未压缩） | 8-12% | 4-6% | ~2% | **15-20%** |
| 50 万连接稳态（压缩后） | 5-8% | 2-4% | ~1% | **10-13%** |
| 日出风暴（任意规模） | +5% | +5-10%（分配触发） | +2% | **+10-15% 瞬时** |

### 8.6 mqtt-gateway 的 GC 成本总账

```text
50 万连接（压缩后，32 核，GOGC=200）：

STW 暂停（全停）：
  稳态：~5-15ms（主要是 mark termination 扫 goroutine 栈残留，§8.2.2）
  日出：~15-30ms（goroutine 增长 + concurrent 窗口短 → 栈残留多）
  → 不是 CPU 瓶颈，但是 p99 延迟抖动的真实来源（不能说"可忽略"）

Concurrent mark（并发，占 CPU）：
  wall-clock：~100ms
  CPU 占比：10-13%（含 worker + assist）
  → 是 GC CPU 成本的主体

Sweep（惰性分摊）：
  ~0.1% CPU
  → 可忽略

GC 总 CPU 占比：
  稳态：10-13%（concurrent mark 主导）
  日出风暴：瞬时可达 20-25%（assist + 栈扫描压力）
  → 稳态在 15% 舒适线内，日出风暴需观测

两层成本要区分：
  GC CPU 占比（10-15%）—— 来自 concurrent mark 的 live heap 重扫
  GC STW 停顿（5-15ms）—— 来自 mark termination 的 goroutine 栈扫描
  两者来源不同：前者是"占 CPU"，后者是"全停"，优化方向也不同
```

### 8.7 STW 尾延迟的防控

mark termination 的 STW（栈扫描残留）是 mqtt-gateway p99 延迟抖动的来源。它有两个子来源，防控方式不同：

```text
来源 A（栈扫描残留，主要）——防控：
  A1. 减少 goroutine 数量（根本解，但 goroutine-per-conn 模型不能减）
  A2. 减少每 goroutine 栈帧的指针数（conn/buf/session 用 numeric 句柄替代指针）
      → 降低每栈扫描工作量 → 但增加间接寻址，性价比中等
  A3. 确保 concurrent mark 窗口足够长（让 async preemption 扫完更多栈）
      → GOGC 调低（更早 GC，窗口宽）但增频率，或增大 GC CPU goal
  A4. 压测观测：GODEBUG=gctrace=1 看 mark termination 的 STW 字段，
      日出风暴时是否 >30ms。若 >30ms 需排查 async preemption 是否高效

来源 B（灰色对象残留，对 mqtt-gateway 可忽略）——防控：
  B1. 控制 mark 期间的分配速率（减少 assist）
      → payload 池化（TODOS #6）减少 transient 分配
      → 日出窗口限流（authQueue bounded）减少 CONNECT 分配爆发
  B2. GOGC=200 / GOMEMLIMIT
  B3. mqtt-gateway 分配速率 << mark 能力 → 来源 B 风险极低，无需特别防控

综合建议：
  mqtt-gateway 的 STW 主要来自来源 A（栈扫描），不是来源 B（灰色残留）。
  优化 A2（减栈帧指针）是直接杠杆，但性价比不如 Session 压缩（后者降 GC CPU，前者降 STW）。
  两者都做：Session 压缩减 GC CPU 10-15% → 10-13%，栈帧指针优化减 STW 5-15ms → 3-10ms。
```

---

## 9. 压测校准（理论必须靠实测验证）

### 8.1 GODEBUG=gctrace=1

```bash
GODEBUG=gctrace=1 ./gateway -f etc/gateway.yaml 2>gc.log
```

每次 GC 输出一行：

```text
gc 42 @5.234s 2%: 0.015+58+0.023 ms clock, 0.12+29/57/86+0.18 ms cpu, 180->180->95 MB, 195 MB goal, 0 MB stacks, 0 MB globals, 32 P
                   ↑                              ↑
                   STW 开始(~15μs)                mark 耗时(58ms clock) ← 这个值就是实测 mark
```

关键看 **`58`** 这个值（mark 阶段的 wall-clock ms）。用不同连接数压测，记录这个值，校准模型的 `C_per_ptr` 和 `F_interference`。

### 8.2 go tool trace

```bash
# 生成 trace
go test -run XXX -trace=trace.out ./internal/server/
# 或运行时采集
curl http://localhost:6060/debug/pprof/trace?seconds=10 > trace.out

# 分析
go tool trace trace.out
# 浏览器看 "Heap" / "GC pausetimes" / "Goroutine analysis"
```

### 8.3 校准方法

```text
1. 压测某个连接数（如 10 万），记录实测 mark 耗时 T_real
2. 理论计算 T_theory = (N × P × C) / W × F
3. 反推 C_per_ptr 或 F_interference：
   C_calibrated = T_real × W / (N × P × F)
4. 用校准后的 C 代入其他连接数估算
```

### 8.4 观测指标（对齐红线 §13）

| 指标 | Prometheus name | 红线 | 说明 |
|---|---|---|---|
| GC pause p99 | `go_gc_pause_p99` | < 200ms | mark + mark termination 的尾延迟 |
| GC CPU 占比 | `go_gc_cpu_fraction` | < 15% | GC 耗 CPU 占比（GC 2% 字段） |
| live heap | `go_heap_live_bytes` | < 可用内存 50% | mark 工作量来源 |
| heap objects | `go_heap_objects` | 关注趋势 | 对象数（含 Session + 临时） |

---

## 10. 优化杠杆（按效果排序）

基于 §7.1 的 Pacer 视角和 §8.2 的 STW 分析，mqtt-gateway 有**两个不同的 GC 优化目标**——降 GC CPU（concurrent mark 占用）和降 STW（mark termination 停顿），杠杆不同：

### 10.1 降 GC CPU（concurrent mark 占用，主要成本）

GC CPU 瓶颈是 **live heap 重扫（600-950 万指针/轮）**，不是分配增量（分配指针速率仅占 mark 能力的 0.2%）。

| 杠杆 | 优化变量 | 对 live heap 重扫 | mqtt-gateway 做法 | GC CPU 收益 |
|---|---|---|---|---|
| **① Session 指针压缩** | 减 P_per_conn | ✅ **直接降** | filterIdx + downlinkPtr 惰性：12→8 | **降 ~40%** |
| **② GOGC=200** | 降 GC 频率 | 间接（周期拉长） | 环境变量 | 降 ~30% |
| ③ payload 池化 | 减 alloc 次数 | 无关（分配本非瓶颈） | sync.Pool | **<2%**（§7.1.5） |
| ④ 加 CPU 核数 | 加 W_workers | mark 时长线性降 | 32→64 核 | mark 时长降一半 |
| ⑤ 改善 cache 命中 | 降 C_per_ptr | 难控 | Session 布局紧凑 | 10-20% |

### 10.2 降 STW（mark termination 停顿，p99 延迟抖动来源）

STW 瓶颈是 **goroutine 栈扫描残留**（§8.2.2），不是灰色对象残留（mqtt-gateway 分配速率低，来源 B 可忽略）。

| 杠杆 | 优化变量 | 对栈扫描残留 | mqtt-gateway 做法 | STW 收益 |
|---|---|---|---|---|
| **⑥ 减少栈帧指针数** | 减每 goroutine 栈指针 | ✅ **直接降** | conn/session 用 numeric 句柄替代栈帧指针 | STW 降 ~30-50% |
| ⑦ 加 CPU 核数 | 全核并行扫根 | termination 线性降 | 32→64 核 | STW 降一半 |
| ⑧ 增大 concurrent 窗口 | 让 async preemption 扫更多栈 | 降残留比例 | 降 GOGC 或增大 GC goal | STW 降但 GC 更频繁 |
| ⑨ 减少 goroutine 数 | 减栈总数 | ✅ 根本解 | 换 reactor（不推荐，§前面分析） | STW 线性降但复杂度暴增 |

### 10.3 两类优化的优先级

```text
如果目标是"降 GC CPU 占比"（10-15% → 8-10%）：
  → Session 指针压缩（①）是最直接杠杆，GC CPU 降 40%
  → GOGC=200（②）辅助
  → payload 池化（③）对 GC CPU 几乎无效

如果目标是"降 STW 延迟抖动"（5-15ms → 3-8ms）：
  → 减少栈帧指针（⑥）是最直接杠杆，但增加代码间接寻址
  → 加 CPU 核数（⑦）线性降但成本高
  → Session 压缩（①）对 STW 无直接帮助（它降 heap mark，不降栈扫描）

两者来源不同，优化方向不同：
  GC CPU ← live heap 重扫 → Session 压缩
  STW    ← goroutine 栈扫描 → 减栈帧指针 / 加核
```

---

## 11. 常见误区纠正

### 误区 1："Session 压缩降 80% GC"

**纠正**：80% 只看 Session 本体（5→1 指针）。含背景指针（栈 4 + 连接对象 3）后，每连接从 12→8，实际降 ~33%。压缩后 50 万的 GC ≈ 当前 35-40 万的水平（不是 25 万）。

### 误区 2："GC 耗时 = STW 停顿"

**纠正**：Go GC 是并发的——mark 阶段不停应用（应用继续跑，只是 CPU 被 mark 占 25%）。真正 STW 只有 mark 开始/结束的微秒级窗口。所谓"GC 耗时 150ms"是 mark **wall-clock**，不是"应用停了 150ms"。应用在这 150ms 里仍在处理消息，只是慢了（CPU 被分走）。

### 误区 3："加内存能降 GC"

**纠正**：加内存（如 64GB→128GB）不降单次 mark 耗时——mark 扫的是指针数（由对象结构决定），不是总内存。加内存只降 GC 频率（live heap 占比小，GOGC 触发慢）。

### 误区 4："减少对象数能降 GC"

**纠正**：要区分"减少对象数"和"减少指针数"。`[4]uint64` 是 1 个对象但 0 指针（noscan），GC 跳过；`[4]*Slot` 是 1 个对象但 4 指针，GC 扫 4 个。**减对象数无效，减指针数才有效。**

---

## 12. 参考资料

- [A Guide to the Go Garbage Collector（go.dev 官方）](https://go.dev/doc/gc/guide)
- [Garbage Collection In Go – Semantics（Ardan Labs）](https://www.ardanlabs.com/blog/2018/12/garbage-collection-in-go-part1-semantics.html)
- [Go Runtime Source: mgc.go](https://github.com/golang/go/blob/master/src/runtime/mgc.go)
- [Understanding Go's GC（rugu.dev）](https://rugu.dev/en/blog/understanding-go-gc/)
- 配套文档：[容量规划 §10 演进路径](../11.cloud-mqtt-gateway-capacity-planning.md)、[红线 §4 pointer-poor](../7.cloud-mqtt-gateway-implementation-red-lines.md)
