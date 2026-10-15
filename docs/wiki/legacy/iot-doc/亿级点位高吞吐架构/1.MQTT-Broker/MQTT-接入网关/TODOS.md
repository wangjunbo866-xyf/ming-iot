# TODOS — MQTT 接入层设计

> 由 2026-06-10 框架设计 review（plan-eng-review + Codex outside voice）产出。
> 每项含完整上下文：3 个月后接手的人应能直接看懂动机、现状和入口。

## 1. Decoder 设计文档（编号顺延，doc 9 已被网络攻击防范设计占用）

- **What**：为 Modbus Decoder 写设计文档：consumer group 规划、schema_id 点表版本管理、**端到端去重机制**（去重状态存哪：独立去重存储 vs 时序库按 `(tenant_id, device_id, sample_ts)` upsert 天然幂等）、含"注入重复 QoS1 消息 → 断言下游单行"的验证方案。
- **Why**：整个 at-least-once 链路的最后一环。doc 1/3/6 反复说"下游按 dedupe key 去重"，但下游至今没有设计载体；doc 7 §14 上线红线点名"重复消息去重验证"，没这篇文档无法验收。
- **Pros**：补全 QoS1 可靠性故事的闭环；点表版本变更后 raw 重算（doc 1 §6.6）也依赖此文档。
- **Cons**：需要先收敛 Decoder 选型（自研 consumer vs Flink）与时序库选型，过早写细节可能返工。
- **Context**：权威去重 key 已定为 `(tenant_id, device_id, boot_id, seq)`（2026-06-10 review D4）；身份字段取自 Kafka envelope（认证主体），`boot_id/seq` 取自设备 payload header、由 Decoder 解析。兼容 key `(tenant_id, device_id, sample_ts, payload_crc32)`。
- **Depends on / blocked by**：去重 key 定义（已定）；Decoder/时序库选型（未定）。

## 2. Broker drain / 滚动升级协议

- **Status（2026-06-20）：已设计** → [12.drain-rolling-upgrade-design](./12.cloud-mqtt-gateway-drain-rolling-upgrade-design.md)（参考 EMQX `eviction_agent`/`node_rebalance`，裁掉会话迁移；6 阶段有序协议 + 反馈中止 + 指标/验收）。待实现（Phase 2，Phase 5 停 downlink 依赖 gRPC 下行 endpoint + Broker Registry；Phase 1~4+closer-stack 可先构成无下行阶段的 drain）。
- **What**：设计 Broker 优雅下线编排：摘 LB → 停 accept → 在途 append 排空（带超时上限）→ 下行队列拒收并回执 NOT_HERE → presence 条件删除（带 fencing）→ 进程退出；对应的 Command Service 重试行为；以及 doc 8 的演练场景。
- **Why**：doc 7 §14 把"Broker drain/shutdown 流程"列为上线门槛，但 8 篇文档没有任何一篇设计它。没有协议，每次滚动升级都是生产实验（本 review 与 Codex 跨模型一致认定，Codex #13）。
- **Pros**：滚动升级/缩容变成可演练流程；顺序错误的三种事故（下行黑洞 / 设备重发风暴 / 拒连风暴）被设计排除。
- **Cons**：kick、broker lease、下行队列尚未实现，现在写细节有猜测成分——建议等 presence/lease 落地后写。
- **Context**：进程内已有 closer-stack 优雅关闭（构造序逆序关闭）；缺的是集群/LB/presence 层面的编排。
- **Depends on / blocked by**：broker lease 实现（已选定 Redis，和 presence 同库）；下行队列（Phase 2）。

## 3. 下行投递契约：SUBSCRIBE vs 未订阅直推（固件团队决策）

- **What**：与固件团队确定：设备 SUBSCRIBE 自己的 cmd topic（Broker 需实现受限 SUBSCRIBE/SUBACK），还是固件承诺接受未订阅的服务端直推 PUBLISH。结论写进 doc 1 §7.0 / doc 3 §2.2。
- **Why**：标准 MQTT 客户端库只把"订阅过的 topic"的消息交给应用层——固件若用现成库且不订阅，下发的命令会被库静默丢弃。doc 3 §2.2 目前是"如果固件允许……需确认设备行为"的悬空状态（本 review 与 Codex #5 一致标记）。
- **Pros**：Phase 2 下行链路的前置决策显式化；同时锁定 Broker 是否要实现 SUBSCRIBE 包的工作量边界。
- **Cons**：决策权在固件团队，TODO 只能跟踪不能代拍板。
- **Context**：CLAUDE.md 协议范围已预留"受限 SUBSCRIBE/SUBACK（仅设备自己的命令 topic，禁 wildcard/跨设备/$share）"。
- **Depends on / blocked by**：固件 MQTT 协议栈选型；Phase 2 启动时间。

## 4. Broker CI/CD 与灰度发布管线

- **What**：为 `mqtt-gateway` 设计构建发布链：CI（`go test -race` + fuzz 短跑 + lint）、可重现镜像构建、版本化发布、灰度机制（按 Broker 节点滚动 + 按租户/地域引流）、回滚动作。
- **Why**：doc 7 §14 要求上线前有"灰度发布、回滚方案"，但没有文档定义二进制如何被构建、验证、推上生产、出问题怎么退。doc 8 §19 压测报告要求记录 Git commit，隐含要求可重现构建。
- **Pros**：红线的灰度/回滚要求获得设计载体；CI 自动跑 -race + fuzz 让 loop programming 的验证器持续生效。
- **Cons**：需先定部署形态（裸金属/容器/K8s，注意 things 生态有 `-tags no_k8s` 传统）。
- **Context**：当前为本地 go build + 手动运行；压测 Phase 4（集群）前必须有可重现的多节点部署方式。
- **Depends on / blocked by**：TODO 2（drain 协议）是灰度滚动的前置；部署形态决策。

## 5. QoS1 异步执行模型 + QoS1 准入策略

- **What**：把 QoS1 从当前"同步 SubmitAndWait + inflight=1"演进为异步 append-callback 模型（raw log 设计 §4.0 / 红线 §7.2：`AppendResult` + bounded 回执队列 + `ConnID`/`SessionVersion` fencing），支持 per-connection inflight>1，并装回 doc 6 §9.2 的 L1 软压挡（延迟 PUBACK 而非直接断连）；配套设计 QoS1 准入策略（纯遥测 topic 拒绝/降级 QoS1、per-tenant QoS1 配额）。
- **Why**：当前 v1 同步执行（`server.go` handlePublish 阻塞在 `ingest/queue.go` SubmitAndWait，inflight 天然=1）只有"ack-fast | disconnect"两挡，对 QoS1 把 doc 6 §9.2 的水位梯度塌缩为 L0→L3。Kafka durable-ack 抖动或"一大批设备误设 QoS1"会直接跳到大批断连 → re-auth（认证 DB）+ re-presence（Redis）风暴（doc 6 §2.3 放大链路 / §4.4 / §10）。论证已收口在 doc 6 §4.4/§4.5。
- **Pros**：恢复 L1 软压挡，Kafka 打嗝不再变 CONNECT 风暴；inflight>1 提升单设备吞吐与延迟容忍；QoS 准入从源头挡住误配置 QoS1 的 durable 容量浪费。
- **Cons**：异步引入跨 goroutine fencing 复杂度（红线 §7.2 契约已定，但实现必须 `-race` 压测）；QoS 准入的拒绝/降级偏离标准 MQTT 语义（设备等 PUBACK），需固件团队约定——与 TODO #3 下行投递契约同属固件协议决策。注意异步**买不到**更高吞吐天花板（仍受 acks=all Kafka 容量约束）与 mass-QoS1 总负载下降，那两件靠 QoS 准入 + 容量规划。
- **Context**：演进触发条件——inflight>1 的吞吐/延迟需求出现，或高 QoS1 占比场景需要 L1 软压挡吸收 Kafka 延迟抖动时再切；在此之前同步 v1 够用且更安全（单 goroutine、无 fencing、过载 fail-fast 卸载而非堆积）。纯遥测幂等（下游按 `(device, boot_id, seq)` 去重）下，逆变器 5min 全量快照本就该默认 QoS0，QoS1/critical topic 留给事件/告警。
- **Depends on / blocked by**：异步 callback 模型（红线 §7.2 契约已定，未实现）；QoS 准入需产品接入配置（`access_profile`，Phase 4）与固件 QoS 约定。

## 6. payload buffer 池化 + header 批量读

- **What**：
  (1) `mqtt/packet.go:60` 每包 `make([]byte, remaining)` 改为按尺寸分档处理：**小 payload（≤ 4KB）直接 `make`，不进池**（走 Go fast path：≤16B 走 tiny allocator、≤32KB 走 per-P size class，且 `[]byte` noscan，池化净负收益）；**大 payload（> 4KB 且 ≤ `max_payload_size`）走 4KB/16KB 分级 `sync.Pool`**，用 RAII（`payloadPool.Get(n)` 返回 `([]byte, *lease)`，`lease.Release()` 归还）收敛归还点为单一事实源。**先压测 pprof alloc profile 确认尺寸分布与 4KB 阈值**，按真实负载加中间档（如真实分布集中在 1~3KB 时加一档），禁止预置 ≤256B 小档；
  (2) `mqtt/packet.go:69` `readRemainingLength` 逐字节 syscall 改 64B 栈 header 缓冲，一次性读 fixed header（1B type + ≤4B remaining length）再切，消除每包 2~5 次 syscall（nats 报告 §6.2 中 ROI 项）；
  (3) 验证集：`-race` 下注入"归还点早于 Kafka ack"的故障用例，确保 use-after-Put 被测到；并补池化前后的 GC 指标对比（`go_heap_live_bytes` / `heap objects` / `go_gc_pause_p99` / `go_gc_cpu_fraction`，对应 doc 7 §13 压测验收项）。
- **Why**：`packet.go:60` 每包堆分配是百万 QPS 下 GC 主因；sync.Pool 分级池（只池化大 payload）是 doc 7 §5.1.1 / nats 报告 §6.1 已采纳方案；归还点是池化方案唯一敬畏点（漏 Put 等同每包 make，早 Put 写脏数据）。小 slice 池化被剔除是 Go 运行时机制决定（tiny allocator + per-P size class + `[]byte` noscan），不是调优偏好。
- **Pros**：稳态近似零分配（仅大 payload 档），直接改善 doc 7 §13 的 GC 压测红线指标；header 批量读消除每包 2~5 次 syscall，独立低风险，可同批落地。
- **Cons**：归还点逻辑跨 read loop / ingest worker / Kafka ack callback 三层，复杂度集中在此；RAII 封装有一次性设计成本。
- **Context**：归还点详见 doc 7 §5.1.1；QoS0 异步归还点钉死在 `ingest/queue.go` worker 的 `q.release(cost)` 旁；QoS1 同步路径在 `SubmitAndWait` 返回 nil 后归还（须确认 franz-go `ProduceSync` 返回时已释放 `Record.Value`）。本项与 TODO #5（异步执行模型）解耦：同步 v1 下即可落地，切异步时归还点随 callback 模型外移即可，池本身不动。
- **Depends on / blocked by**：无（独立可落地）。

## 7. §5.5 高压断连不读 payload（读包流程重构）

- **What**：当前 `packet.go:60` 在任何预算判断前就 `make+ReadFull` 整个 body。高压下（全局 append budget 不足）应先查预算、预算不足则**只读 header 不读 body 就断连**（红线 §5.5：QoS1 断连不读 payload，让设备退避重试；QoS0 drop）。本次 review（2026-06-19）只落地了 **RAM 核算**（config 新增 `maxInflightRAM`，Validate 校验 `maxConnections×maxPacketSize ≤ maxInflightRAM`），**§5.5 的"不读 payload 才断连"流程改造未做**——需要重构读包流程（先读 remaining-length → 查预算 → 预算不足拒连 / 充足才 ReadFull body）。
- **Why**：RAM 核算是启动期静态约束（防配置撞死 RAM），§5.5 是运行时动态背压（高压下不读大包、不占读缓冲内存）。两者互补，本次只做了前者。不读 payload 才断连能避免"预算已满但已 ReadFull 了整个大包"的瞬时内存尖峰。
- **Pros**：彻底实现红线 §5.5 的运行时背压语义；高压下读缓冲内存不随 payload 大小膨胀。
- **Cons**：读包流程从"顺序读"改成"两段读（header → 查预算 → body）"，`ReadPacket` 签名要变（需要传入预算检查回调或返回"预算不足"专用错误），触及 server 层 handlePublish 的调用。改动面比 RAM 核算大。
- **Context**：RAM 核算（config.Validate）已落地（commit 待提交）；per-client 预算（`maxPendingBytesPerClient`）也已落地（ingest queue 两段式 reserve）。本项是背压的最后一块拼图——前两者是"占用前判预算"，本项是"读之前判预算"。
- **Depends on / blocked by**：无（独立于 TODO #5 异步模型；同步 v1 下可落地）。

## 8. Kafka append 硬超时（防 worker/预算泄漏）

- **What**：当前 `ingest/queue.go:168` 的 worker 假设 `Appender.Append(ctx)` 按 deadline 返回，budget 只在 Append 返回后释放（`queue.go:171`）。但 franz-go 的 `ProduceSync(ctx)` 在幂等 producer 下，ctx 取消**不能保证 promptly 返回**——幂等序列安全要求 broker 确认，Kafka 黑洞/分区时 client 会等重试。结果：`SubmitAndWait` 的 caller 超时返回了，worker 仍卡在 `Append(ctx)`，payload budget 没释放（泄漏）。需给 franz-go 配硬超时上限，让 ProduceSync 在 Kafka 黑洞时确能在有限时间内失败：`RecordRetries` 上限 + `RequestTimeoutOverhead` + `TransactionTimeout`，使 append 的最坏耗时 = 配置的上界而非无穷。
- **Why**：review 2026-06-20 #2 指出：当前 queue 的 deadline 是软的（caller 超时返回但 worker 卡住 + budget 泄漏）。黑洞/分区场景下 budget 累积不释放，最终全局 queue 满、所有设备 QoS0 drop / QoS1 断连。
- **Pros**：budget 泄漏被堵住，Kafka 黑洞场景下系统能在有限时间内恢复预算（append 失败 → release → 释放 budget）。
- **Cons**：硬超时配置需和 Kafka broker 的 `request.timeout.ms` / `delivery.timeout.ms` 协调，配错会导致正常的长 RTT 请求被误杀。需压测验证。
- **Context**：`appender.go:62` 用 `ProduceSync(ctx)`；幂等 producer 的序列安全约束见 franz-go 文档。本项是 budget 管理的兜底，与 TODO #6（payload 池化归还点）的 release 时机相关但不耦合。
- **Depends on / blocked by**：无（独立可落地，但建议压测环境具备 Kafka 黑洞注入能力）。

## 9. 租户/区域级准入隔离（第 1 层 accept 限流无租户公平性）

- **What**：设计认证后的 per-tenant / per-region 连接准入配额，防止单个租户/运营商/区域的事故流量吃掉 Broker 全局准入预算（doc 21 第 1 层总量桶不分 key，一个区域的雷鸣群/固件事故会挤占其它正常区域的接入容量）。
- **Why**：doc 19 评审（2026-07-03，Codex outside voice）指出三层防御只有 global/IP/device 三个维度，无租户隔离。认证前拿不到可信租户（doc 19 §19.3.3 红线禁止按不可信 key 聚合），只能做认证后准入——属于平台级准入控制，超出 flapping 防御范围，需单独设计。
- **Pros**：多租户形态下的爆炸半径隔离；与 doc 9 A5 责任矩阵的 per-tenant 限流埋点闭环。
- **Cons**：依赖多租户容量模型（每租户配额怎么定）；认证后拒绝对设备表现为「认证成功但被拒」，需定义 CONNACK 语义与固件行为。
- **Context**：当前接入层以单一大客户形态为主，多租户公平性尚非现实痛点；1M 目标规模 + 多租户后必须面对。准入点建议放 auth 成功后、presence 写入前（与 doc 19 §19.5 黑名单检查点②同位置）。
- **Depends on / blocked by**：多租户容量模型；`access_profile` 产品接入配置（Phase 4，与 TODO #5 的 QoS 准入同载体）。

## 10. presence 写删防抖/合并（flapping 成本的互补缓解）

- **What**：短窗口内同设备重连的 presence 写删合并——如 OnClose 的 presence delete 延迟数秒执行，窗口内设备重连则原地复用条目、只推进 session_version，省掉一次 delete + 一次全新 connect 写。
- **Why**：doc 19 评审（2026-07-03，Codex outside voice）指出 flapping 设备在达到封禁阈值前要烧 ~15 × 2 次 presence Redis 写；防抖能把「快速建断连循环」的 presence 成本降一个量级，是封禁之外的互补缓解（封禁治设备，防抖降单价）。对不达阈值的低速 flapping（doc 19 §19.6.1 跨节点稀释漏网的那部分）也有效。
- **Pros**：区域性 flapping 事件下 presence Redis 写放大显著下降。
- **Cons**：**动 presence 一致性核心**——延迟 delete 与红线 #6（CONNECT 成功 = presence 写入成功）/#8（fencing）语义交互复杂：延迟窗口内 presence 条目「存活但连接已死」，下行投递会打到死连接（NOT_HERE 语义要兜住）。必须独立设计评审，不得搭 flapping 便车。
- **Context**：presence 写路径 = doc 2 §5.1 Lua CAS（connect）+ §3.1 fencing delete；presence 删除已走异步 worker pool（doc 19 §19.8 与其他防御的协作 / 三层执行流），防抖等于在该 worker 前加窗口合并层。
- **Depends on / blocked by**：doc 2 presence 设计评审（fencing 语义）；下行链路对「条目在但连接死」的容错语义（gRPC Deliver → NOT_HERE 路径）。

## 11. per-device 包级洪水实时识别（认证后 PUBLISH 洪水的热路径缺口）

- **What**：设计 per-device 上行速率限制（QoS0/PUBLISH packets/sec 或 bytes/sec）+ 实时软封禁，填补「认证成功后的恶意设备狂发上行」在热路径上只拖慢、不识别的空白。
- **Why**：doc 19 §19.1.2 边界澄清（2026-07-08，源于 NanoMQ GHSA-v57q-w88m-424r 的讨论）挖出的真实缺口——被劫持的合法设备（凭证有效）在 CONNECT 成功后灌 QoS0 PUBLISH 洪水：
  - **survival 兜住了**（全局 `max_append_queue_bytes` + per-client `max_pending_bytes_per_client` ≈ 16-32KB，broker 不崩，doc 6 §6.6 + 红线 #2/#4）；
  - **但识别 + 摘除没兜住**——热路径只让攻击者自己的 per-client budget 耗尽 → drop（合法设备的 QoS0 也一起被全局 drop，挤在同一条管道无法区分）；识别走 doc 9 §6.3 离线基线（**小时级**），kick 是 Phase 2。T=0 到离线触发之间，恶意设备与合法设备共享同一条降级路径。
  - **QoS0 尤其薄**：inflight=1 是 QoS1 专属（doc 6 §4.4），QoS0 无 PUBACK、无 MQTT 层背压；doc 6 §9.2 自承 v1 的 L0-L3 梯度对 QoS1 塌缩成 L0→L3，QoS0 更糟（只有字节 ceiling，无速率 ceiling）。
  - 与 TODO #9（租户公平性）不同维度：#9 是认证后准入配额（防区域事故吃全局），本项是认证后**单设备**包级速率（防恶意/失控设备灌包），两者互补。
- **Pros**：把「拖慢」升级为「拖慢 + 识别 + 热路径软封」，压缩 T=0 到摘除的窗口；与 doc 19 flapping 封禁（device_blacklist，认证后可信 deviceID）可复用同一执行点。
- **Cons**：per-device 速率计数表又是一个「按 deviceID 聚合的有界结构」（≤ 在线设备数，同 doc 19 §19.4.2 的 noscan 选型）——GC 成本需评估；阈值校准难（正常逆变器 5min 全量快照的稳态速率 vs 异常高频，需按型号基线，doc 9 §6.3 已有基线概念可复用）；误封代价 = 真实设备掉线。
- **Context**：触发讨论的 NanoMQ advisory 是 C/QUIC 的 UAF（代码 bug，非 flapping 模式），mqtt-gateway 结构免疫其崩溃路径（Go GC + 无 QUIC），但其「认证后恶意设备」前提在本架构真实存在。SUBSCRIBE 大规模洪水的攻击面 v1 结构性免疫（dispatch 不路由 SUB/UNSUB），故本项聚焦 PUBLISH；doc 14 动态路由落地后 SUBSCRIBE 攻击面需独立评估。
- **Depends on / blocked by**：doc 6 背压水位 L1 软压挡（TODO #5 异步执行模型，QoS0 无 inflight 背压的根因）；doc 9 §6.3 离线基线的 per-device 速率信号（可前移为热路径计数）；device_blacklist 执行点（doc 19 §19.5/§19.6，可复用）。

---

## 已落地项（2026-06-19 review 修复）

以下 5 条 review 问题已在 2026-06-19 落地实现（commit 待提交），记录于此供追溯：

- **#4 TLS 接线**（中→已修）：新建 `internal/tlsconf` 包（强制 TLS1.3 + ECDSA + 无状态 session resumption），config 加 `server.tls` 段，main 构造 `*tls.Config` 并传入 server。生产必填，明文仅开发环境（main warn）。`server.go` 已支持，本次只补 config + main 接线。
- **#2 RAM 核算**（中→已修）：config 加 `server.maxInflightRAM`，Validate 校验 `maxConnections×maxPacketSize ≤ maxInflightRAM`。注意：§5.5 运行时"不读 payload 才断连"未做，见 TODO #7。
- **#3 per-client 预算**（中→已修）：config 加 `ingest.maxPendingBytesPerClient`，ingest queue 加 `perClient map[string]int64` + 两段式 reserve（先 per-client 后 global，per-client 失败不占全局，对齐 doc §6.6 行 600-609）。单设备狂发 QoS0 不再独占全局。
- **#5 product 契约约束**（低→已修）：`record.go` mqtt_topic header 加不可信注释；raw-log 设计文档补"mqtt_topic product 段不可信"契约说明（逆变器挂网关下，单认证多 product，ACL 只校验非空）。下游务必从认证身份取 product。
- **#6 Will bit 修复**（低→已修）：`connect.go` 加 WillFlag=0 时校验 WillQoS(bit4-3)/WillRetain(bit5) 必须为 0（§3.1.2.11），否则 malformed。配 3 个 table-driven 测试。
