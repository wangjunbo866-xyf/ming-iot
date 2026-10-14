# 百万级设备低频上报场景下 VerneMQ 可行性研究

## 核心判断

从产品定位和能力边界看，**VerneMQ 可以作为你这个场景的候选 Broker，而且数量级并不离谱**。官方文档把它定义为高性能、分布式 MQTT Broker，强调可以在通用硬件上横向和纵向扩展；官网 FAQ 还直接回答了“如何处理超过一百万连接”的问题。功能上，VerneMQ 明确支持 MQTT 3.1/3.1.1/5.0、QoS 0/1/2、集群、共享订阅、LevelDB 离线消息存储，以及 Prometheus / `$SYS` 监控；管理文档也明确提到集群里“可能存在数百万 MQTT session”。这说明从**协议能力**和**产品意图**上，它是覆盖你场景的。 citeturn22view0turn23view0turn24view0turn8view0

但如果把问题改成“**能不能默认配置直接上生产**”，答案就是否定的。VerneMQ 官方并没有给出一个可以直接套用的“某硬件 = 某连接数”的标准 sizing 表，反而明确建议使用自家的 `vmq_mzbench` 按你的硬件、消息体、延迟目标去压测，以反推规格。官方博客确实引用过一个 KubeCon 2018 的案例分享，标题就是“在 Kubernetes 上服务 500 万消息连接之旅”；另一个官方案例又提到他们曾为客户优化过 **7000 万离线消息** 的启动和迁移问题。这些都说明 VerneMQ 的架构上限足以覆盖你的数量级，但**不能把这些案例当成你项目的硬件保证书**。 citeturn3view7turn17view0turn17view1

如果只给一个结论，我会把 VerneMQ 的判断定为：**谨慎可行**。它很适合你这种“设备很多、单设备很闲、整体是 fan-in”的上报型场景；但前提是你接受 QoS 0 在极端 cluster netsplit 情况下的丢失风险，并且愿意认真做集群化、粘性负载均衡、连接内存与文件句柄调优，以及真实故障压测。相反，如果你的真实需求是“任意节点故障、任意网络分区、任意后端抖动都不能丢任何上报”，那在 99% 仍采用 QoS 0 的前提下，问题已经不是 VerneMQ 行不行，而是**消息语义本身**就没有把“不丢”作为保证目标。 citeturn7view0turn3view7turn20view1

截至 **2026-05-10**，GitHub Releases 页面显示 **2.1.1** 仍是 latest 稳定版，而 **2.1.3 RC1** 已于 **2026-04-22** 发布，说明项目仍在维护；但还要注意一个很实际的点：源码仓库标注为 **Apache-2.0**，而官方 binary packages 和 Docker images 在 release 说明里又明确写了 **受 VerneMQ EULA 约束**，并要求官方包的商业使用支付订阅费。也就是说，**技术可行**之外，还要把**分发方式与商业许可**一起纳入选型。 citeturn14view0turn3view8turn13view2turn13view0

## 场景画像

按你给出的约束，先不谈 Broker，单看负载本身：

| 指标 | 估算 |
|---|---:|
| 设备数 | 1,000,000 |
| 单设备上报周期 | 300 秒 |
| 平均上报速率 | 约 3,333 条/秒 |
| 你给的 QoS0 JSON 样例大小 | 约 2,598 字节 |
| 纯 payload 平均吞吐 | 约 8.66 MB/s |
| QoS1/2 若占 1% | 约 33 条/秒 |

这组数最关键的含义是：**你的 steady-state 压力并不在消息吞吐，而在连接管理**。每秒三千多条、单条约 2.6KB，这对一个分布式 MQTT Broker 来说不是离谱数字；真正吃资源的是**一百万长连接本身**，以及设备批量重连时的**认证、TLS、session 恢复、offline queue 迁移**。VerneMQ 官方的调优文档把每连接 TCP buffer 直接称为“关注内存使用时最重要的话题”，Open File Limits 文档则强调每个 TCP 连接至少要占一个 file descriptor；数据库认证文档进一步指出，登录路径上的密码校验方式会直接决定新连接到达率上限。 citeturn3view0turn19view0turn11view0

对你的场景来说，这又带来一个很积极的结论：**99% 的 QoS0 是利好**。QoS0 避开了大部分持久化、重传和 inflight 管理的成本，而 VerneMQ 对 QoS1/2 的默认保护项——例如 `max_inflight_messages`、`max_offline_messages`、`max_online_messages`——主要影响的是可靠传输与排队路径。既然你的 QoS1/2 只占极小比例，理论上完全可以把 Broker 的设计重点放在“高密度连接 + 稳定摄入 + 少量可靠控制/补报”上，而不是把 VerneMQ 当成一个长期、大规模、强持久化的消息堆积系统。 citeturn3view4turn23view0

## VerneMQ 为什么匹配这个场景

VerneMQ 官方的“典型部署”文档几乎就是在描述你的工作负载：**大量 MQTT 客户端持续向集群 fan-in 上报数据，由后端服务消费**。官方明确写到，在这种情况下，普通订阅会把同一份消息复制给所有消费者，无法解决单消费者吃不下的问题；因此推荐使用 **shared subscriptions** 将同一主题下的消息分摊到多台后端消费客户端上。更重要的是，VerneMQ 还提供 `local_only` 分发策略，让消息只投递给**本节点的共享订阅消费者**，减少跨节点转发开销，这一点对上报型场景非常贴切。并且 shared subscriptions 在 VerneMQ 中不仅支持 MQTT 5，也向 3.1.1 做了 backport。 citeturn20view1turn7view1

它的网络模型也很适合“设备很多、单设备带宽很低、后端少而强”的结构。VerneMQ 官方博客专门讲过 TCP buffers，并明确举出**fan-in 场景**：发布侧应该用更小的 TCP buffers，消费侧可以用更大的 buffers；而且这两类连接**不必共用同一个 listener**，可以分别配置。对你的场景，这意味着可以把一百万设备放在“发布 listener”上，尽量用小 buffers 降低连接密度成本；而把后端 shared subscribers 放在“消费 listener”上，用更大的 buffers 提高集群到后端的出流效率。这个设计思路和你的负载结构是高度契合的。 citeturn17view2turn20view2

还有一个常被忽略、但对你很重要的适配点：**如果设备基本只上报、不需要离线下行，就尽量不要给它们开持久会话**。VerneMQ 的典型部署文档把“persistent sessions”定义为 broker 会在客户端离线时保存**发往这些客户端的 QoS1/2 消息**；session 生命周期文档也说明，返回的持久客户端在唤醒时可能涉及 queue 初始化、离线消息迁移以及重复连接清理。换句话说，**只有那些确实需要下行离线命令/补偿的客户端，才值得承担持久 session 的复杂度**；纯上报设备如果用非持久会话，等于主动砍掉一大块不必要的状态成本。 citeturn20view1turn11view1

## 需要重点警惕的约束

最现实的第一道约束，是**每连接内存和文件句柄**。VerneMQ 文档指出，Erlang 网络 I/O 驱动会为每个 TCP 连接维护内部 application-side buffer，并且 VerneMQ 会根据 OS 的 TCP send/receive buffer 计算自己的 buffer；文档示例里，若 OS 参数对应到 32KB，那么 VerneMQ 每个连接的内部 buffer 就是 **32KB**。同一份文档还专门强调，这正是 RAM 使用的头号关注点。另一方面，Open File Limits 文档明确写到：每个 TCP 连接至少需要一个 file handle，而默认 listener 配置里 `listener.max_connections = 10000`。这不是架构上限，但它说明：**VerneMQ 默认配置离你的目标差了两个数量级**，如果不显式改 listener、ulimit、`fs.file-max`，连 10 万级都很难稳。 citeturn3view0turn9view1turn19view0

第二道约束，是**报文体积偏大带来的瞬时内存压力**。你给出的样例约 **2.6KB**，在 IoT 遥测里不算异常，但确实已经不是几十到几百字节的小心跳。稳态均匀上报时，100 万设备每 5 分钟一条只产生约 **8.66MB/s** 的纯 payload；可如果设备在 10 秒内集中上报，同样的业务量会瞬间变成约 **260MB/s** 的纯 payload，还没有计入 topic、协议头、TLS、队列结构、插件处理和跨节点转发开销。QoS0 不落盘只能降低持久化成本，并不代表消息在接收、匹配、投递、session queue、TCP buffer 或消费者背压路径上完全不占内存。VerneMQ 的 `max_online_messages` 也明确是为了保护在线 session queue，超过阈值时可以丢弃任意 QoS 的消息。 citeturn3view4turn3view0

这里需要避免一个过度简化的说法：Erlang/OTP 对较大的 binary 通常会使用 reference-counted off-heap binary，多进程之间传递时未必每次完整复制整段 payload；但是只要 mailbox、session queue、hook/plugin、跨节点转发链路或慢消费者继续持有引用，这段 binary 就不能及时释放。也就是说，大报文风险的核心不是“每路由一次必然全量拷贝”，而是**短时积压会把每条消息的驻留成本放大**，从而引发 GC 抖动、调度延迟和节点内存水位持续上升。对你的模型，这条风险可评为**中风险**；一旦存在集中上报、消费者降速、跨节点投递不均或异常大包，就会升为**高风险**。 citeturn0view0

第三道约束，是**持久会话的迁移成本**。官方文档明确说，在 VerneMQ 里，一个持久 session 只存在于某个 client 当前连着的那台节点上；如果后续它连到另一台节点，session 和相关 offline messages 就会被迁移过去。官方因此建议在负载均衡器前使用**session-affine** 的策略，例如 source hash，避免随机打散；在 Kubernetes 指南里又进一步建议：大规模场景下优先用外部负载均衡、提供 session affinity、在 VerneMQ 外做 TLS termination、并且“宁可少量但很强的 Pod，也不要大量弱 Pod”。这些建议本质上都在说明：**大规模 VerneMQ 不是不能横向扩，但横向扩展必须是“有粘性、少迁移、少震荡”的扩展**。 citeturn20view1turn7view2

第四道约束，是**集群网络分区下的 QoS0 语义**。VerneMQ 的 netsplit 文档写得非常直白：它能检测网络分区；默认情况下会停止处理 `CONNECT`、`PUBLISH`、`SUBSCRIBE`、`UNSUBSCRIBE`；对已经发生在不确定窗口内的情况，QoS0 publish **会丢**，而 subscription 元数据又是 eventually consistent，因此发布与订阅在分区期间可能看不到彼此的新变化。此外，文档还承认在“Window of Uncertainty”中可能出现 duplicate clients。对你的场景，这意味着：**如果 99% 的上报是 QoS0，那么你必须业务上接受极端 netsplit 时的少量遥测丢失**。对于状态遥测，这通常是可接受的；对于控制指令、计费、审计，则通常不可接受。 citeturn7view0

第五道约束，是**认证与重连风暴**。VerneMQ 的数据库认证文档指出，ACL 会在客户端连接时缓存，因此 publish/subscribe 不必对数据库做每包查询；这是好事。但同一文档也提醒，如果你把慢哈希放在 Broker 侧做，例如 client-side `bcrypt`，它会直接吃掉 Broker 的 CPU，并把新客户端到达率压得很低。文档甚至给了例子：若一次 `bcrypt` 校验耗时 0.5 秒，那么单线程核每秒只能做 2 次验证，4 核也只有 8 次/秒。这对 steady-state 没什么问题，但对**几万到几十万设备同时回连**就是灾难。所以在百万设备场景里，**连接风暴经常比稳态发布更危险**。 citeturn11view0

第六道约束，是**不要把 shared subscriptions 当 Kafka 消费组**。VerneMQ 文档明确说，shared subscriptions 仍然严格遵循 MQTT 语义，并不存在 Kafka 式的“全组共享离线队列”；每个 shared subscriber 依然拥有自己的 offline queue。如果把后端 shared consumers 做成持久会话、又允许它们离线，那么积压会分散到各自队列里，而不是自然形成一个统一 backlog。对遥测 ingest 这类系统，通常更好的做法是：**shared subscribers 保持在线、尽量无状态，不要让 Broker 承担它们长期离线时的排队职责**。 citeturn7view1

最后还有一个**队列保护**边界。VerneMQ 的 `max_online_messages` 文档明确写到，这个设置是为了保护 session queue，超过阈值时可以**丢弃任何 QoS 的消息**；而 `max_offline_messages` 则限制离线队列中可保存的 QoS1/2 消息数。也就是说，VerneMQ 可以做缓冲，但它首先是 MQTT Broker，其次才是有限度的缓冲层。如果你真正想要“后端挂两小时，Broker 也要把所有上报完整兜住”，那必须非常谨慎地验证积压、恢复、重启和迁移路径。官方 70M offline messages 的案例恰恰说明了：**极大 backlog 不是不行，但运维复杂度会显著抬升**。 citeturn3view4turn17view1

## 建议的部署与配置方向

基于你给出的消息大小与上报频率，再结合 VerneMQ 文档对每连接 buffer、文件句柄和“少量强节点”的强调，我会把 **4–6 台高规格 Broker 节点** 作为**首轮 PoC 的起配范围**，把单节点连接数先控制在 **16–25 万**这个区间，再用压测去验证是否能继续压缩节点数。这不是官方承诺值，而是我基于你这个负载结构做的工程推断：在你的场景里，消息吞吐不夸张，**连接经济学**才是主维度；只要单节点连接密度别贪得太满，VerneMQ 成功概率会高很多。如果 TLS 还要在 VerneMQ 上终止，而不是在外部 LB 终止，那么节点数和 CPU 冗余都应该再留更大的安全边际。 citeturn3view0turn19view0turn7view2turn3view7

在入口层，我倾向于使用 **L4 外部负载均衡 + 粘性策略**。官方“典型部署”文档把 random、source hash、甚至基于 MQTT client-id 的 protocol-aware balancing 都列为可以考虑的策略，并强调 sticky/session-affine 会显著减少持久会话迁移的额外开销。若你在云上或 K8s 上部署，官方进一步建议外部 LB、session affinity、外部 TLS termination；如果通过代理转发客户端真实地址或证书信息，VerneMQ 也支持 PROXY protocol，包括从 PROXY v2 转递 TLS client certificate details。对于大量设备跑在运营商 NAT 后面的场景，我会更偏向于**能基于 client-id 做粘性**的方案；如果基础设施只能做 source hash，那也至少比 random 强得多。这里前半句是官方建议，后半句关于运营商 NAT 的取舍是基于官方能力做的工程推断。 citeturn20view2turn3view6turn7view2

我会优先显式配置下面这些项，而不是依赖默认值：

- **分离 publisher / subscriber listeners，并为两者设置不同的 `buffer_sizes`**。你的设备侧 listener 目标是“高连接密度、低单连接吞吐”，宜偏小；后端 shared consumers 的 listener 目标是“少连接、高吞吐”，宜偏大。VerneMQ 官方博客对 fan-in 场景就是这样建议的。 citeturn17view2turn20view2
- **显式提高 `listener.max_connections` 与 `listener.nr_of_acceptors`，并同步提高 `fs.file-max` 与 `nofile`**。官方默认 `listener.max_connections=10000`，而且每 TCP 连接至少需要一个 file descriptor；systemd 场景下还要确保 `LimitNOFILE` 生效。 citeturn9view1turn19view0turn22view0
- **纯上报设备尽量使用非持久会话**；只有少量确实需要离线下行、QoS1/2 可靠投递的客户端，才使用持久会话。这样可以显著减少 session 迁移与 offline queue 的复杂度。 citeturn20view1turn11view1
- **后端消费者统一走 shared subscriptions，并优先考虑 `local_only`**，这样可以把 local node 上的摄入和消费尽量闭环，降低跨节点消息转发开销。 citeturn20view1turn7view1
- **把 `max_online_messages`、`max_offline_messages` 和 `max_message_size` 一起当成内存预算来设计**。`max_offline_messages` 应设到“最低可接受值”而不是无限大；`max_online_messages` 要能限制慢消费者或异常 session 的在线积压；`max_message_size` 则建议按协议真实上限设置，例如从 8KB 或 16KB 起测，避免错误固件或恶意客户端发送远超正常遥测大小的包。官方文档对这些保护项都有直接说明。 citeturn3view4turn18view0turn3view5turn20view1
- **不要把 Broker 当长时间持久堆积层**。如果后端处理链路可能慢很多，应该尽快把 shared subscribers 后面的数据卸载到更适合做持久 backlog 的系统，而不是依赖 VerneMQ 无限排队。这个判断基于 VerneMQ 的 queue shedding 机制与官方 70M offline messages 案例。 citeturn3view4turn17view1

真正决定成败的，不是文档，而是你的压测是否覆盖了**最坏时刻**。我认为至少要做下面几类验证，而且这些验证比“稳态 3333 msg/s 能不能跑起来”更重要：

- **一百万稳定连接 + 真实消息体 + 稳态 3333 msg/s**，连续跑到你能接受的观察窗口，验证内存、FD、CPU、队列、延迟。官方推荐使用 `vmq_mzbench` 做这种场景化压测。 citeturn3view7
- **大报文突发与背压测试**：使用 2.6KB 真实 payload，分别模拟 10 秒、30 秒、60 秒集中上报，并在测试中短暂停止或降速 shared subscribers，观察 RSS、Erlang binary memory、process mailbox、session queue、GC、调度延迟和消息丢弃计数。这个测试比平均 3333 msg/s 更能暴露大报文积压风险。
- **连接风暴测试**：例如 5%–10% 设备在几分钟内集中重连，观察认证路径、TLS 握手、session 恢复和负载均衡是否成为瓶颈。数据库认证与慢哈希对峰值连接速率的影响，官方已经明确提醒。 citeturn11view0
- **节点故障与 session migration**：在有持久会话的小比例客户端存在时，验证某个 Broker 下线、恢复、或设备被打到别的节点之后，session/offline queue 迁移带来的延迟与超时。 citeturn20view1turn11view1
- **netsplit 测试**：不是只测“节点宕机”，而是真测 cluster 网络分区，确认业务是否能接受 QoS0 丢失、短时不可发布，以及最终的一致性恢复过程。 citeturn7view0
- **后端消费者降速或离线**：验证 shared subscribers 吃不动、或者短时离线时，`max_online_messages` / `max_offline_messages` 对实际业务的影响。 citeturn3view4turn7view1

在可观测性上，VerneMQ 原生支持 Prometheus、`$SYS` 和 `vmq-admin metrics show`；示例指标里已经包括 `mqtt_publish_sent`、`queue_initialized_from_storage`、`retain_memory` 等。再结合 `vmq-admin session show` 可以看在线状态、session 所在节点、队列创建时间、offline/online 消息数量等。对百万连接场景，这些不是可选项，而是上线前就要接好的基础设施。 citeturn8view1turn8view0turn25search0

## 最终结论

如果你的业务本质是**海量设备遥测上报**，单设备每 5 分钟一条、99% 用 QoS0，而且你可以接受极端网络分区下极少量 QoS0 遥测丢失，那么 **VerneMQ 是可行的**。而且从你给出的样例消息看，真正的瓶颈绝不是 steady-state 消息量，而是**一百万在线连接的资源模型**、**2.6KB 报文在突发/背压下的瞬时积压成本**、**重连风暴**、以及**少量持久 session 的恢复/迁移路径**。VerneMQ 官方提供的共享订阅、listener 级 buffer 配置、Prometheus 监控、集群与 sticky LB 建议，恰好都能对上这些问题。 citeturn20view1turn17view2turn8view1turn24view0

但如果你的诉求更像下面这样：**上报消息不能因 netsplit 丢、后端挂很久也要由 Broker 完整兜底、并且最好拿现成镜像直接商用部署**，那你对 VerneMQ 的预期就需要更谨慎。原因不是它不能做，而是你会同时踩到 QoS0 语义、队列保护/负载剪切、巨大 backlog 复杂度、以及官方二进制发行许可这几条线。我的建议不是“一票否决”，而是：**把 VerneMQ 放进 shortlist，但只在完成上述故障压测和许可确认后再拍板**；生产上优先使用当前 latest 稳定版 **2.1.1**，而不是 RC。 citeturn7view0turn3view4turn17view1turn14view1turn13view0

综合打分的话，我会这样归类：**作为“百万上报连接 Broker”的候选方案，VerneMQ 值得进场；作为“不经严谨压测即可默认放心”的方案，它还不够。** 在你的约束下，它最像一个**需要认真工程化落地的强候选**，而不是一个“只要开箱就自然成功”的选择。 citeturn3view7turn20view1turn24view0
