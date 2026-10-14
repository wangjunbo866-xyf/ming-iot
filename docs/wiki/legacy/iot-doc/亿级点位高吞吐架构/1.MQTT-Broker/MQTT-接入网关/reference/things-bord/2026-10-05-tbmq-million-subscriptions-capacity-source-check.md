# TBMQ 单节点百万订阅容量：官方压测与源码证据核验

核验日期：2026-10-05。目标是区分连接数、订阅关系数、不同过滤器数和消息吞吐，判断官方证据是否覆盖“单节点百万级订阅索引”。本文使用厂商官方报告及其链接的公开脚本；未自行运行压测，没有云资源支出。

## 1. 结论边界

在本次核验的三组官方压测中，**没有找到“独立单台 TBMQ 完成 100 万/200 万设备订阅关系”的直接实测结果**。最接近的是 P2P 测试：全群 50 万订阅客户端、每客户端一个独有过滤器，由 5 台 Broker 承担业务。结合测试版本的全量订阅复制机制，可以进一步推导每个 Broker 持有约 50 万订阅关系，但这仍未覆盖用户提出的 100 万设备 × 2 个订阅 = 200 万关系。[P2P 官方报告](https://tbmq.io/docs/reference/1m-throughput-p2p-performance-test/)

同一 P2P 报告中，只有 1 台 Broker 的第一档是 **10 万发布者 + 10 万订阅者**。不能把最终 100 万连接、双向 100 万 msg/s，改写成单节点 100 万订阅。下面三项指标必须分别计数。

| 官方测试 | 发布客户端 | 订阅客户端 | 每个订阅客户端的过滤器数 | 不同订阅过滤器数 | 总订阅关系数 | Broker 数 |
|---|---:|---:|---:|---:|---:|---:|
| 100M fan-in | 1 亿 | 500 | 1 | 500 | 500 | 25 |
| 3M msg/s fan-out | 100 | 3,000 | 1 | 1 | 3,000 | 1 |
| 1M msg/s P2P 最终档 | 50 万 | 50 万 | 1 | 50 万 | 50 万 | 5 |
| P2P 第一档 | 10 万 | 10 万 | 1 | 10 万 | 10 万 | 1 |

来源：[fan-in 报告](https://tbmq.io/docs/reference/100m-connections-performance-test/)、[fan-out 报告](https://tbmq.io/docs/reference/3m-throughput-single-node-performance-test/)、[P2P 报告](https://tbmq.io/docs/reference/1m-throughput-p2p-performance-test/)。上述过滤器/关系数量也经过第 3 节脚本交叉核对；不是将页面标题作为订阅规模。

## 2. 硬件、负载和测试时长

| 测试 | 单台 Broker 硬件 | Broker 堆参数（公开脚本） | 外部服务 | 负载与持续时间 |
|---|---|---|---|---|
| 100M fan-in | `m6g.metal`，64 vCPU / 256 GiB | `-Xmx150g -Xms150g` | 9 Kafka，各 8 vCPU / 32 GiB；1 RDS | QoS 1，约 114 B；300 万入站 + 300 万出站 msg/s；1 小时 |
| 3M fan-out | `m7a.8xlarge`，32 vCPU / 128 GiB | `-Xmx90g -Xms90g` | 3 Kafka，各 2 vCPU / 8 GiB；1 RDS | QoS 0，约 66 B；1,000 入站 → 300 万出站 msg/s；30 分钟 |
| 1M P2P 最终档 | `c7a.4xlarge`，16 vCPU / 32 GiB，共 5 台 | `-Xmx24g -Xms24g` | 5 Kafka，各 2 vCPU / 4 GiB；11 Redis，各 2 vCPU / 4 GiB；1 RDS | QoS 1，约 62 B；50 万入站 + 50 万出站 msg/s；10 分钟 |

硬件与消息参数见以上三份官方报告；Broker 堆参数分别来自 [100M configmap L40](https://github.com/thingsboard/tbmq/blob/6fb6e6295e41d367f60e09813b0c364b5ee5616a/k8s/aws/tb-broker-configmap.yml#L40)、[fan-out configmap L26](https://github.com/thingsboard/tbmq/blob/2c3c11d43a5e2a9662e27c097d16f4bc4cd9c0e7/k8s/aws/tb-broker-configmap.yml#L26)、[P2P configmap L26](https://github.com/thingsboard/tbmq/blob/8f913181d622bc81cf96f24e2930bd9f4bdccfc8/k8s/aws/tb-broker-configmap.yml#L26)。堆上限不是订阅索引实际占用；不能把它除以订阅数得出每条订阅的内存成本。

P2P 报告明确 Redis 测试使用无副本 master 以节省费用。因此该性能表也不是完整生产高可用拓扑的故障恢复结果。它报告最终约 75 ms 平均端到端延迟、约 8 ms 发布确认延迟，但没有给百万订阅注册的 SUBACK 分位延迟。[P2P 官方测试方法和基础设施](https://tbmq.io/docs/reference/1m-throughput-p2p-performance-test/#infrastructure-overview)

## 3. 公开脚本对订阅规模的交叉核对

### 3.1 100M fan-in：1 亿是发布 Topic 名称，实际仅 500 个订阅过滤器

本次读取 `tb-mqtt-perf-tests` 的 `100M` 分支固定快照 `0686f88aeac0a46734e454bac6922ba97b2dc972`，解析 YAML 内嵌的 JSON：

- 发布配置有 500 个 group，每个 200,000 publisher，共 100,000,000 publisher。
- 订阅配置有 500 个 group，每个 1 subscriber，共 500 subscriber，500 个不同的 `topicFilter`。
- 每个过滤器形如 `usa/ydwvv/1/+`，覆盖该 group 中大量独有发布 Topic。
- 配置测试时长 3,600 秒，发布和订阅 QoS 都为 1。

证据：[固定发布配置](https://github.com/thingsboard/tb-mqtt-perf-tests/blob/0686f88aeac0a46734e454bac6922ba97b2dc972/k8s/broker-tests-publishers-config.yml)、[固定订阅配置](https://github.com/thingsboard/tb-mqtt-perf-tests/blob/0686f88aeac0a46734e454bac6922ba97b2dc972/k8s/broker-tests-subscribers-config.yml)。官方报告所谓“100M unique topics”属于发布名称的基数，不是订阅索引中 1 亿条过滤器/关系。

### 3.2 fan-out：单节点 300 万 msg/s 来自 3,000 个成员匹配同一个过滤器

测试工具 `3M-single-node-perf-test` 快照 `bd9d155748e91bb6d293df5c4fece2ba49ba01ca`：订阅配置仅一个 group，`subscribers=3000`，`topicFilter=ua/kyiv/#`，非持久会话，QoS 0，1,800 秒。因此是一个过滤器叶子下的大量成员扇出，不是百万不同过滤器的检索试验。[固定订阅配置](https://github.com/thingsboard/tb-mqtt-perf-tests/blob/bd9d155748e91bb6d293df5c4fece2ba49ba01ca/k8s/broker-tests-subscribers-config.yml#L58-L85)

### 3.3 P2P：最接近大量独有过滤器的公开证据

测试工具 `p2p-perf-test` 快照 `75c8fb077f9e1ca0016efe04679b5bdcd23dce44`：README 说明动态生成一对一发布/订阅 group；发布和订阅 runner 环境变量均设 `TEST_RUN_DEFAULT_PUB_SUB_GROUPS_COUNT=500000`，测试 600 秒，QoS 1。subscriber 代码针对每位客户端调用一次 `.on(group.getTopicFilter(), ...)`。这里的 50 万是全局测试 group 数，不能再乘 runner pod 数。[配置生成说明](https://github.com/thingsboard/tb-mqtt-perf-tests/blob/75c8fb077f9e1ca0016efe04679b5bdcd23dce44/k8s/README.md#L19-L47)、[订阅 runner 参数](https://github.com/thingsboard/tb-mqtt-perf-tests/blob/75c8fb077f9e1ca0016efe04679b5bdcd23dce44/k8s/broker-tests-subscribers.yml#L98-L111)、[每客户端一次订阅](https://github.com/thingsboard/tb-mqtt-perf-tests/blob/75c8fb077f9e1ca0016efe04679b5bdcd23dce44/src/main/java/org/thingsboard/mqtt/broker/service/SubscriberServiceImpl.java#L115-L125)

过滤器形如 `europe/ua/kyiv/$number/+`，每个 `$number` 对应唯一发布/订阅对，因此产生 50 万个不同过滤器；这比前两组测试更接近用户设备独有下行 Topic 的高基数场景。但其负载为持续一对一消息收发，尚不是用户的每设备 command+OTA 双订阅。[官方方法](https://tbmq.io/docs/reference/1m-throughput-p2p-performance-test/#test-methodology)

## 4. 测试版本固定及不能跨越的证据边界

| 官方报告链接的分支 | 本次固定的 Broker commit | deployment 中镜像标签 | performance tool commit |
|---|---|---|---|
| `100M` | `6fb6e6295e41d367f60e09813b0c364b5ee5616a` | `thingsboard/tbmq-node:1.0.0` | `0686f88aeac0a46734e454bac6922ba97b2dc972` |
| `3M-single-node-perf-test` | `2c3c11d43a5e2a9662e27c097d16f4bc4cd9c0e7` | `thingsboard/tbmq-node:1.1.1-SNAPSHOT` | `bd9d155748e91bb6d293df5c4fece2ba49ba01ca` |
| `p2p-perf-test` | `8f913181d622bc81cf96f24e2930bd9f4bdccfc8` | `thingsboard/tbmq-node:2.0.1` | `75c8fb077f9e1ca0016efe04679b5bdcd23dce44` |

deployment 证据：[100M](https://github.com/thingsboard/tbmq/blob/6fb6e6295e41d367f60e09813b0c364b5ee5616a/k8s/aws/tb-broker.yml#L24-L48)、[fan-out](https://github.com/thingsboard/tbmq/blob/2c3c11d43a5e2a9662e27c097d16f4bc4cd9c0e7/k8s/aws/tb-broker.yml#L24-L47)、[P2P](https://github.com/thingsboard/tbmq/blob/8f913181d622bc81cf96f24e2930bd9f4bdccfc8/k8s/aws/tb-broker.yml#L24-L67)。对应分支快照提交时间分别为 2023-06-28、2023-11-06、2024-12-31；它们是历史压测材料，不是当前 v2.4.0 的同配置实测。

镜像标签可变，公开材料没有将原始测试执行、不可变镜像 digest、全部运行期配置及源码提交绑定成独立可审计证据。因此这里固定的是“本次读到的官方配套分支快照”，不能进一步声称已经证明当年运行的二进制与此 commit 完全一致，也不是本项目的独立复现。

## 5. 已证、可推导、未覆盖

| 结论 | 证据强度 |
|---|---|
| 官方报告了 50 万独有订阅过滤器的 P2P 集群测试 | 报告与公开参数相互印证；厂商自测 |
| 单 Broker 曾在 P2P 第一档承载 10 万订阅客户端 | 官方表直接给出；厂商自测 |
| P2P 最终档每个 Broker 持有约 50 万全局订阅索引 | 同次研究已核对该历史源码的独立消费组与全量 Trie 构建，据此推导；并非报告逐节点 dump 的直接测量 |
| 当前 TBMQ v2.4.0 单 Broker 存放 100 万/200 万订阅关系的内存峰值和稳定运行 | 本次材料未提供直接测试结果 |
| 100 万设备每台两个订阅时的注册速度、SUBACK P99 | 未提供 |
| 大规模重连/重订阅时的更新延迟、Kafka lag、Trie 更新吞吐 | 未提供 |
| 节点重启后回放 200 万订阅并恢复可服务的时间 | 未提供 |
| 网络分区、Broker/Kafka/Redis 故障注入后的定量恢复/丢失情况 | 上述三份报告未提供可据以确定这些指标的试验结果 |

100M 报告的约 22k connections/sec 是建连速率，不是 SUBSCRIBE 注册速率。P2P 使用持久订阅者、说明离线消息语义，也不等于它已报告过故障注入、重连风暴与恢复耗时。公开代码中存在订阅准备阶段和自动重连能力，更不等于相关容量指标已经被测量并发表。

## 6. 当前 v2.4.0 如何处理大量订阅

本节固定 commit `2f60cfef8e058f4cd663b46bc4553ff23dfe89cc`。实现提供了百万级索引的工程基础，但源码本身不能证明具体机器、流量和故障条件下的容量上限。

### 6.1 内存 Trie 按 Topic 层级匹配

`ConcurrentMapSubscriptionTrie` 的每个节点有 `ConcurrentHashMap<String, Node<T>> children` 与并发集合 `values`。插入时用 `computeIfAbsent` 复用已有前缀；查询沿当前段的精确、`+`、`#` 分支访问，未命中的分支不继续搜索。精确过滤器、小匹配结果的场景无需扫描全部订阅关系。[节点字段](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L269)、[路径复用](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L138)、[查询过程](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L64)

因此不能把查询描述成“对 200 万订阅做 200 万次字符串比较”。也不能普遍承诺 O(1) 或只与 Topic 深度有关：通配分支、匹配的订阅者数量、字符串处理和结果对象分配都增加成本；内存规模变化还会影响缓存和 GC。这些条件限制了官网“百万订阅与千条一样快”的概括。[结果构造](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L114)、[字符串处理](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L254)

例如 `devices/{id}/command` 与 `devices/{id}/ota`，N 个唯一设备形成 `3N + 2` 个逻辑 Trie 节点（包含 root 和 devices），保存 `2N` 条订阅关系。因此百万设备示例是约 300 万树节点、200 万关系。源码节点计数 gauge 从根以外的新建节点开始累加，不能要求它与包含根的手算值完全相等。每个 Node 立即创建 map/set 对象字段，但底层容量分配另有机制；不能据此声称已测得每节点或每条关系固定多少字节。

### 6.2 查询并发、更新串行和回收

- 查询 `get()` 不获取 Trie 的那把读写锁。`put()` 获取共享 readLock，可并发插入；空节点清理获取 writeLock。`delete()` 也没有获取这把锁。因此“所有路由查询都经过全局锁”不符合实现，但也不宜将整个实现概括成完全无锁。[插入与删除](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L123)
- 每客户端另有 `clientId → subscriptions` 集合，删除某客户端时可直接定位其过滤器；同一客户端更新通过 Actor mailbox 串行执行，不同 Actor 由执行器并行处理。该集合本身是 HashSet，依赖这条并发约束。[集合与操作](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/subscription/ClientSubscriptionServiceImpl.java#L49)、[Mailbox 调度](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/common/actor/src/main/java/org/thingsboard/mqtt/broker/actors/TbActorMailbox.java#L111)
- 退订先移除订阅关系，空树路径并不立即全部回收；默认每天零点清理。清理获取写锁会影响新插入，100 ms 是等待拿锁的上限，绝非整次清理耗时上限。大量短命过滤器可能造成两次清理之间的节点积累，应检查实际节点数与内存。[清理实现](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L188)、[默认配置](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/resources/thingsboard-mqtt-broker.yml#L936)

同一 filter 下退订一个客户端还有独立成本：`delete()` 会扫描该节点的 `values`，用 clientId 谓词筛选。百万设备各订独有命令 filter 时每叶子的成员很少；百万设备都订同一个普通广播 filter 时，一次退订可能遍历大量成员。后者虽然树小，投递和退订都不能视为低成本。[删除扫描](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L161)、[clientId 谓词](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/subscription/SubscriptionServiceImpl.java#L69)

### 6.3 Kafka 负责状态恢复与传播，不能减少节点索引规模

每节点独立消费组恢复全量订阅，并持续接收变化；监听执行器单线程逐批解析、向 Actor 分派，实际应用并非整个 Broker 都由一条线程执行。变更量、单节点接收速度、Actor 队列与冷启动回放构成另一组容量约束。增加 Broker 不会分摊各节点必须持有的全局订阅索引。[消费者实现](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ClientSubscriptionConsumerImpl.java#L56)

P2P 历史分支也已交叉核对：其 consumer 按 serviceId + 启动时间建独立组，初始化汇集全部客户端订阅；订阅服务遍历整个输入构建本地 Trie。因此第 5 节的每节点约 50 万关系是有历史源码依据的推导，而非把当前版本行为未经检查套到旧测试上。[旧消费者](https://github.com/thingsboard/tbmq/blob/8f913181d622bc81cf96f24e2930bd9f4bdccfc8/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ClientSubscriptionConsumerImpl.java#L77)、[旧初始化](https://github.com/thingsboard/tbmq/blob/8f913181d622bc81cf96f24e2930bd9f4bdccfc8/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/subscription/ClientSubscriptionServiceImpl.java#L60)

## 7. 对本项目需要补的容量证据

目标负载应保留实际 Topic 长度、百万个不同 clientId、每设备 command/OTA 两条订阅，以及计划采用的连接、TLS、QoS 和消息负载。使用一两个客户端订阅 200 万 filter 不能替代这个模型，它会改变按客户端的集合、快照写入、会话和 Actor 开销。

| 维度 | 应记录的证据 |
|---|---|
| 存储与常驻内存 | 50 万、100 万、200 万关系逐档测 heap/RSS、GC、Trie 节点数；对照冷启动峰值与稳态，不把配置的 Xmx 当已用内存 |
| 稳态路由 | 定向遥控匹配与端到端下发 P95/P99、CPU、结果扇出量；用实际业务吞吐，单独测大范围 OTA 广播 |
| 更新与故障恢复 | SUBACK 延迟、其他节点可见时间、Kafka 订阅状态 lag、Actor 排队；批量重连/退订、一个节点重启后的恢复时间 |

当前官方指标区分 `subscriptions`（关系数）、`subscriptionTopicTrieSize`（Trie 内订阅数）和 `subscriptionTrieNodes`（树节点数）；三者均为各节点持有的全局视图，不能跨 Broker 相加后冒充实际规模。检查各节点是否收敛到同一计数，展示集群逻辑规模时用 max 或单节点值。`connectedSessions` 才是本节点实际连接数，可跨节点求和。[指标定义及聚合口径](https://tbmq.io/docs/prometheus-metrics/#aggregating-across-a-cluster)

这些是建议采集的验证项，不是已经完成的实验，也没有替用户确定尚未给出的延迟 SLA。当前可给出的判断是：内存 Trie 让百万级订阅具备工程可行性，公开 P2P 数据提供了几十万关系的参考；是否能在本项目预算与硬件上稳定承担 200 万关系，仍需以上实测。

## 8. 同层百万设备的查找：哈希表与有序索引

同一公司采用相同 Topic 格式，不等于所有设备使用同一个键。`devices/device001/command` 与 `devices/device002/command` 共享结构和前缀，但设备段不同；百万个同层子节点可以由哈希表按设备段定位，无需逐个比较兄弟节点。

| 实现与固定版本 | 实际查找方式 | 源码证据 |
|---|---|---|
| TBMQ v2.4.0 | Trie 的 `children` 是 `ConcurrentHashMap`，查询用 `children.get(...)` 定位当前精确段，并检查 `+`、`#` 分支 | [查询 L64](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L64)、[节点字段 L269](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L269) |
| EMQX 6.2.2 精确路由 | 精确 Topic 放在 ETS `bag`；以完整 Topic 调 `ets:lookup(route_table(), Topic)` 找目标节点 | [表类型 L145](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_router.erl#L145)、[直接查询 L424](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_router.erl#L424) |
| EMQX 6.2.2 本地订阅者 | 本地 `SUBSCRIBER` 同为 `bag`，以 Topic 为键调用 `ets:lookup_element`，然后枚举命中的本地成员 | [本地表 L127](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_broker.erl#L127)、[查询 L528](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_broker.erl#L528)、[实际按键读取 L863](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_broker.erl#L863) |
| EMQX 6.2.2 通配路由 | 独立 `ordered_set` 保存通配索引；`emqx_trie_search` 构造 seek key，经 `ets:next` 跳过不可能匹配的前缀 | [有序表 L175](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_router.erl#L175)、[底层定位 L96](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_topic_index.erl#L96)、[seek 分支 L227](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_trie_search.erl#L227) |
| Mosquitto v2.1.2 | `sub__search` 分别用 `HASH_FIND` 查当前精确段、`+` 和 `#`；uthash 先由键的哈希定位 bucket | [查找 L440](https://github.com/eclipse-mosquitto/mosquitto/blob/99fa50f30e325609394c324c8ff71cfbbe95d8ab/src/subs.c#L440)、[哈希实现 L140](https://github.com/eclipse-mosquitto/mosquitto/blob/99fa50f30e325609394c324c8ff71cfbbe95d8ab/deps/uthash.h#L140) |

所以“找到 device001 仍必须遍历百万设备”的前提不成立。TBMQ/Mosquitto 的分层索引不等于每层链表扫描；EMQX 的精确路由甚至直接使用整个 Topic 作为查找键。共同前缀仍可复用，减少重复路径，但不会消除百万个设备身份及其成员关系的存储成本。

不能把这些路径一概称为 O(1)：哈希计算、冲突、同键成员数、通配分支和结果枚举都有成本；ETS `ordered_set` 使用二叉搜索树，其查找成本随表大小对数增长。[Erlang 官方 ETS 文档](https://www.erlang.org/doc/apps/stdlib/ets.html)

若百万客户端真的普通订阅同一广播 Topic，定位该键可以很快，但仍须向百万成员投递。这里的百万级遍历是实际输出量，而非为了找到一个目标设备而扫描整个设备库。

对只允许固定 deviceId 定向 Topic 的专用网关，可以考虑解析 deviceId，再查本地连接 map 或节点归属映射。这是缩减通用 MQTT 匹配范围的设计推论，不是上述 Broker 已测出的性能优势；仍需保持授权检查、已建立的订阅语义，并用会话代际/归属校验阻止旧连接或旧路由误投递。

## 9. 设备上下线、节点切换与 Map 变更压力

以下继续以 v2.4.0 为源码基准。这里将用户所说“切换 TSDB 节点”按上下文理解为切换 TBMQ 接入节点。

对 `devices/{id}/command` 与 `devices/{id}/ota`，不是每一层都有一张百万键的 Map：root.children 只有 devices；devices.children 有 N 个设备键；N 个设备节点各有两个子键；2N 个叶子的 children 为空。每节点仍有大量小 Map、并发集合、订阅对象，以及全局会话和按客户端订阅索引，内存成本真实存在。[Node 字段](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L269)

| 设备生命周期 | 订阅与会话的实际变化 |
|---|---|
| 持久会话仍存在、同 clientId、cleanStart=false 跨节点恢复 | 断开保存 disconnected 会话；重连保存含新 serviceId/sessionId 等的新会话信息，已有订阅可以保留。该连接分支不重写订阅状态日志；客户端自行重新 SUBSCRIBE 则另计 |
| MQTT 3 CleanSession=true 断开并重连订阅 | 删除该 clientId 的订阅集合与各 filter 对应的 Trie.values 成员，持久化空集合；新订阅再写入成员和客户端集合 |
| 新设备/新 Topic，或空路径已经被周期清理 | 需创建缺失的树节点，可能伴随哈希表扩容与更多对象分配 |

生命周期证据：[断开时持久/非持久分支](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/session/SessionClusterManagerImpl.java#L342)、[重连 cleanStart 判断](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/session/SessionClusterManagerImpl.java#L177)、[订阅清除](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/subscription/ClientSubscriptionServiceImpl.java#L212)。Trie.delete 只删成员，不立即删 children 路径；相同设备/Topic 在空节点清理前重订可复用路径，不能把每次断连都算成整条树路径删除再创建。[删除成员](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L161)、[复用路径](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ConcurrentMapSubscriptionTrie.java#L138)

ConcurrentHashMap 支持并发查询和更新，读取通常不阻塞写入；扩容、冲突与热点仍有代价。更新已有键的值不要求复制整个 Map。API 的线程安全承诺不能替代跨节点会话协调、多个索引之间的正确性和 Kafka 状态可见性验证。[JDK 文档](https://docs.oracle.com/en/java/javase/21/docs/api/java.base/java/util/concurrent/ConcurrentHashMap.html)

“频繁”应折算为到达率：百万设备若平均每小时重连一次，约 278 次/秒；10 万设备在一分钟重连，约 1,667 次/秒；百万设备在一分钟重连，约 16,667 次/秒。这些只是算术场景，不是用户真实负载或 TBMQ 已测上限。一次重连涉及多个会话/订阅事件与操作，不能等同于一次 Map.put。需要测每节点状态日志消费、Actor 应用、分配/GC 与连接握手是否能跟上，及积压后的恢复速度。

本项目现阶段采用 CleanSession=true，因此必须在真实双订阅条件下验证断开清理与重订；不能用持久会话直接恢复的低订阅变更路径替代。百万键本身不是不可行的理由，但当前证据仍不足以承诺本项目的峰值重连容量。

## 10. 官方基准是否覆盖重连与订阅变更

本次进一步检索了 TBMQ 官方性能入口、相关博客、集群概念文档和官方性能工具。结论是：**有连接规模、已有订阅下的稳定消息吞吐测试，也有重连能力；本次公开材料未发现覆盖“百万设备、每设备两条订阅、CleanSession=true、跨节点重连并持续清理/重建订阅”的定量基准。**这不是断言厂商从未做过内部测试。[官方性能入口](https://tbmq.io/performance/)

| 关注点 | 公开材料实际给出的内容 | 能否直接回答本项目问题 |
|---|---|---|
| 大量订阅存在时的消息路由 | P2P 最终档 50 万订阅者、各一个独有过滤器，持久会话，稳态发送 10 分钟 | 可作全量索引与消息投递的参考；不等于 200 万关系变更压测 |
| 连接建立速率 | 100M 报告约 22k connections/sec，但只有 500 个 APPLICATION 订阅者 | 不可当作 22k 次/秒双订阅清理重建或故障后重连能力 |
| 频繁 SUBSCRIBE/UNSUBSCRIBE | 工具有订阅准备阶段和底层订阅/退订 API | 本次未发现已发表的持续变更率、SUBACK P99、跨节点收敛延迟结果 |
| Broker 故障后跨节点恢复 | 集群文档解释客户端可连到其他节点、持久会话可恢复 | 属于功能/架构说明；未给指定规模的恢复时间和重连成功率曲线 |
| 节点冷启动回放全量订阅 | 文档说明启动先重建本地会话与订阅副本再服务 | 本次未发现 100 万/200 万订阅下的回放吞吐、峰值内存、ready 时间 |

表格依据：[P2P 方法](https://tbmq.io/docs/reference/1m-throughput-p2p-performance-test/#test-methodology)、[100M 测试摘要](https://tbmq.io/docs/reference/100m-connections-performance-test/#test-summary)、[集群状态恢复与故障语义](https://tbmq.io/docs/concepts/clustering/)。这些文档没有提供足以填写上述缺失性能指标的结果。

2025-02-07 的官方博客有硬件故障时仍保持可靠投递的概括，链接回 P2P 性能测试；但文章没有给出故障注入对象、注入时刻、同时重连客户端数、订阅清理/重建到达率，以及恢复耗时分布。因此该概括不能补齐百万重连与订阅变更的实测证据。[官方博客](https://tbmq.io/blog/1-million-reasons-to-choose-tbmq-as-high-performance-mqtt-broker/)

本次新增核验的性能工具版本为 `thingsboard/tb-mqtt-perf-tests` 当前 master 快照 `a456b11ce22aa61f1405720606b22565e0b89333`（提交日期 2026-08-11）。它不是第 4 节历史压测分支，不能把新增代码自动归入当年的执行环境。

- 主流程为清理旧会话 → 连接订阅者 → 建立订阅 → 连接发布者 → 预热 → 等待集群就绪 → 持续发布 → 结束后断开和分析。该主流程未包含按指定速率反复重连/退订重订、强制跨节点切换或重启 Broker 的步骤。[runTest L103–168](https://github.com/thingsboard/tb-mqtt-perf-tests/blob/a456b11ce22aa61f1405720606b22565e0b89333/src/main/java/org/thingsboard/mqtt/broker/tests/MqttPerformanceTest.java#L103-L168)
- 客户端有自动重连开关与延迟，失败后可调度重连；这证明客户端的辅助能力，不能证明测试中实际注入过特定规模故障并测得恢复性能。[重连调度 L191–213](https://github.com/thingsboard/tb-mqtt-perf-tests/blob/a456b11ce22aa61f1405720606b22565e0b89333/src/main/java/org/thingsboard/mqtt/broker/client/mqtt/MqttClientImpl.java#L191-L213)
- `ClusterProcessServiceImpl` 按并发配置分批执行任务，记录每批耗时和是否等到 latch 完成；不是已发表的全群成功订阅吞吐。订阅成功及失败回调都将 latch 减一，超时也有单独结果。因此没有真实运行日志、成功/失败计数及跨 runner 时间对齐，不能用配置的客户端数除以某个等待时间来推导成功 SUB/s。[分批与计时 L47–73](https://github.com/thingsboard/tb-mqtt-perf-tests/blob/a456b11ce22aa61f1405720606b22565e0b89333/src/main/java/org/thingsboard/mqtt/broker/service/ClusterProcessServiceImpl.java#L47-L73)、[订阅回调 L115–125](https://github.com/thingsboard/tb-mqtt-perf-tests/blob/a456b11ce22aa61f1405720606b22565e0b89333/src/main/java/org/thingsboard/mqtt/broker/service/SubscriberServiceImpl.java#L115-L125)
- `SubscribeStats` 的 latency 实际在收到消息时记录，内容是消息端到端/处理延迟；不能因为类名包含 Subscribe 就当作 SUBACK 延迟直方图。[消息统计 L128–139](https://github.com/thingsboard/tb-mqtt-perf-tests/blob/a456b11ce22aa61f1405720606b22565e0b89333/src/main/java/org/thingsboard/mqtt/broker/service/SubscriberServiceImpl.java#L128-L139)

因此现有官方工具可以作为扩展压测的起点，但还需要显式编排本项目的重连比例、到达率、CleanSession=true 双订阅和跨节点迁移，并补充成功率、SUBACK/下行恢复延迟、状态日志积压与恢复时间等观测，才能回答第 9 节的 Map/Actor/状态复制是否跟得上。本次仅审阅公开资料和代码，未运行这些试验。

### 10.1 ThingsBoard IoT 平台的测试能否补足证据

ThingsBoard 平台与 TBMQ Broker 的性能报告需要分开解读。平台官方 Performance 页描述的是 MQTT 智能电表模拟器、单体 ThingsBoard、每项超过 24 小时的稳态测试；最大一档为 100 万设备、11,111 msg/s、33,333 数据点/s，采用 c5.9xlarge 与 Kafka、Cassandra。页面未披露 RPC/OTA 订阅数量、不同过滤器数量、Clean Session 参数、重连风暴或 transport 故障注入及恢复时间。因此不能把这里的百万设备直接换算为百万独有命令订阅者。[平台性能报告](https://thingsboard.io/docs/reference/architecture/performance/#benchmark-results)

本次另外核对官方 `thingsboard/performance-tests` 快照 `8d36cfe9aaf646ccead0d22db1d378678387c412`：MQTT 设备测试选择 telemetry/attributes 上报主题，基础发布路径使用 QoS 0；所核查主路径没有 RPC/OTA 订阅、按指定速率反复断连重订，或编排 transport 节点重启的步骤。此快照说明当前工具的测试内容，不代表已确认上述历史平台报告使用了完全相同的二进制和参数。[设备测试主题与初始化](https://github.com/thingsboard/performance-tests/blob/8d36cfe9aaf646ccead0d22db1d378678387c412/src/main/java/org/thingsboard/tools/service/device/MqttDeviceAPITest.java#L61-L118)、[基础发布路径](https://github.com/thingsboard/performance-tests/blob/8d36cfe9aaf646ccead0d22db1d378678387c412/src/main/java/org/thingsboard/tools/service/shared/BaseMqttAPITest.java#L249-L277)

平台默认服务端 RPC 订阅过滤器是 `v1/devices/me/rpc/request/+`，结合设备身份处理。它与 TBMQ 上 `devices/{唯一设备ID}/command` 的百万不同过滤器索引并不等价。即使平台另有 RPC 吞吐结果，也需要先核对路由模型，才能判断是否能用于本项目的容量推断。[平台 MQTT RPC API](https://thingsboard.io/docs/reference/mqtt-api/rpc/#server-side-rpc)
