# TBMQ 与 EMQX 的订阅状态、集群路由源码核验

核验日期：2026-10-05。核验方式：读取官方源码、官方文档与发布说明；未运行集群或进行内存压测。

## 1. 核验范围与直接结论

- EMQX 主核验版本：正式 tag `6.2.2`，commit [`70ec105db2a2d17382f8dca12bec400ea9661583`](https://github.com/emqx/emqx/tree/70ec105db2a2d17382f8dca12bec400ea9661583)。使用本地仓库 `/home/wangjunbo/workspace-mqtt/emqx` 的 `git show 6.2.2:路径` 读取，不切换工作区分支、不修改源码。当前本地工作区 HEAD 是另一个开发提交，未把它冒称为正式版本。
- EMQX 历史对照：tag `v5.8.8`，commit [`e1bbd8d45f5eb512226ed2c1b3052826572ac124`](https://github.com/emqx/emqx/tree/e1bbd8d45f5eb512226ed2c1b3052826572ac124)。本报告没有声称所有 5.x/6.x 的复制实现完全相同。
- TBMQ 主核验版本：`v2.4.0`，commit [`2f60cfef8e058f4cd663b46bc4553ff23dfe89cc`](https://github.com/thingsboard/tbmq/tree/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc)；源码证据链见后续 TBMQ 核验节。

对普通在线 MQTT 订阅，应分别讨论两层数据：

| 数据 | TBMQ v2.4.0 | EMQX 6.2.2 普通内存会话路径 |
|---|---|---|
| Topic Filter → 客户端级订阅者/订阅选项 | 各 Broker 维护集群全量订阅索引 | 每个节点维护本地订阅者与选项 |
| Topic Filter → 应接收消息的 Broker 节点 | 先由全局订阅索引找到客户端，再查全局会话元数据确定节点 | 独立的集群路由记录，全局复制 |
| 实际 socket/连接对象 | 仅接入节点 | 仅接入节点 |

EMQX 普通订阅与全局路由不同，不能说它与 TBMQ 一样复制所有普通客户端的订阅明细；也不能因此说 EMQX 每节点只保存本地 Topic。它仍持有集群路由信息。共享订阅另有客户端成员级复制，见第 4 节。本表不覆盖 EMQX Durable Storage 会话实现的全部状态，适用于本次设备在线、普通订阅、CleanSession=true 的讨论。

## 2. EMQX：客户端级普通订阅为何是本地数据

`emqx_broker:create_tabs/0` 创建三个 ETS 表，源码注释直接给出各自的数据模型：

- `SUBOPTION`：`{TopicFilter, SubPid} → SubOption`。
- `SUBSCRIPTION`：`SubPid → TopicFilter 集合`。
- `SUBSCRIBER`：`TopicFilter → SubPid 集合`，高扇出时附有本地分桶结构。

订阅入口取 `SubPid = self()`，将本地会话进程与 Topic 写入 ETS；普通订阅与共享订阅随后走不同分支。这些表不是 `mria:create_table` 创建的全局订阅明细表。[6.2.2 建表定义](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_broker.erl#L115-L131)、[订阅入口及分支](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_broker.erl#L162-L210)。官方集群介绍也将 Subscription Table 描述为按节点分布的本地映射。[官方说明](https://docs.emqx.com/en/emqx/latest/develop/cluster/introduction.html#cluster-data-structures)

本地某 Topic 首次出现订阅者时才增加相应集群路由；该节点最后一个订阅者消失时再删除路由。因此，同一节点的 1 万个普通客户端订阅相同过滤器，仍只需要该过滤器指向这个节点的一条普通路由；客户端级 1 万条关系仍在该节点本地。[首次订阅检查](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_broker.erl#L653-L658)、[首次增加/最后删除路由](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_broker.erl#L802-L810)

## 3. EMQX：全局路由究竟保存什么

普通路由的目标是 Broker 节点。`emqx_router` 将目标类型定义为 `node() | {group(), node()}`：后者用于共享订阅的组与节点。v3 建表注释的示例也是 `#route{topic = ..., dest = 节点}`，不含普通订阅客户端列表。[目标类型](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_router.erl#L86-L93)、[v3 路由结构与建表](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_router.erl#L137-L195)

精确过滤器进入 route 表，含 `+`/`#` 的过滤器进入另一个 `emqx_topic_index` 索引表；消息匹配读取本地已有的路由表和过滤器索引。[写入与查询](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_router.erl#L377-L432)

消息投递分两层：目标是本机时直接 dispatch；是远端节点时进行节点转发。目标节点再查询本地 `SUBSCRIBER`，投递给客户端会话进程。因此节点 B 为发送消息给节点 A 的客户端，不需要先复制 A 上全部普通客户端 PID 与订阅选项。[按节点路由](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_broker.erl#L400-L405)、[远端接收与本地订阅者查询](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_broker.erl#L475-L531)

例如 device001 连接在 A，普通订阅 `devices/device001/command`：

```text
A 的本地订阅表：devices/device001/command → device001 的会话进程
A、B、C 的集群路由：devices/device001/command → A
```

这里的路由通常以订阅 Topic Filter 为键；不应把“所有已发布过的 Topic 名称”也算成必须保留的路由。

## 4. 共享订阅是重要例外

在 6.2.2 的 `emqx_shared_sub` 内，`SHARED_SUBSCRIPTION` 是 Mria 的 `ram_copies` 表，记录为 `{group, topic, subpid}`。`handle_subscribe` 通过 `mria:dirty_write` 写入成员记录，`subscribers(Group, Topic)` 从该表查候选 PID；另外还有只在本地创建的 `SHARED_SUBSCRIBER` ETS 表。[成员表结构](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_shared_sub.erl#L108-L122)、[成员读取及本地 ETS](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_shared_sub.erl#L434-L445)、[成员写入与组路由](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_shared_sub.erl#L500-L510)

这意味着 EMQX 的传统共享订阅路径确实有集群复制的成员级状态，不能概括成“EMQX 任何场景都只复制 Topic→Node”。本次百万设备的各自遥控/OTA 普通订阅不应套用共享订阅表；业务后台 worker 的 `$share/...` 订阅需要单独估算成员数。本文也没有把 Durable Session 的共享消费实现一并宣称为同一路径。

## 5. EMQX v2 与 v3：复制范围相似，写入方式有版本边界

| 核验对象 | 路由复制/写入机制 | 能否称为各节点仅保存本地路由 |
|---|---|---|
| 5.8.8 的 v2 路由表 | 普通 Mria 表，注册 `rlog_shard`、`ram_copies` | 不能；全局路由复制 |
| 6.2.2 的 v2 兼容路径 | 仍保留普通 Mria 表路径 | 不能 |
| 6.2.2 的 v3 路由表 | `merge_table=true`，节点直接更新属于自己的记录，再向其余节点异步复制 | 不能；“仅自己能写的行”不等于“仅自己保存的行” |

证据：5.8.8 的[路由表定义](https://github.com/emqx/emqx/blob/e1bbd8d45f5eb512226ed2c1b3052826572ac124/apps/emqx/src/emqx_router.erl#L133-L160)；6.2.2 的 [v3/v2 建表分支](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_router.erl#L131-L226)。6.2.2 的 broker 明确注释：v3 使用 merge tables，Core 与 Replicant 上的路由写入都是本地操作，因而不再需要为 Core/Replicant 延迟设置的 router syncer。[源码](https://github.com/emqx/emqx/blob/70ec105db2a2d17382f8dca12bec400ea9661583/apps/emqx/src/emqx_broker.erl#L824-L844)

官方新设计文档说明：Regular 表更新由 Core 协调，Replicant 接收副本；Merge 表由每个节点更新自己的记录并异步复制到集群其他节点。[官方设计：Regular 与 Merge](https://docs.emqx.com/en/emqx/latest/develop/design/clustering.html#merge-tables)

官方 5.10.4 发布记录将 routing schema v3 标注为新机制，并说明与 v2 集群的兼容模式及切换要求。因此不能仅凭“5.x”或“6.x”版本大号判断运行时路由 schema。[5.10.4 发布记录](https://github.com/emqx/emqx/discussions/17435)

文档边界：当前官方集群入门页仍有“路由由 Core 更新、Replicant 只读”的宽泛描述，与新设计页对 Merge 表的细分不完全一致。本报告以固定 6.2.2 的 `merge_table` 配置及调用分支为准，不把入门页的旧式说明扩展到所有新版本路由表。旧 4.x/早期文档已有“本地订阅表、全局 Topic→Node 路由”的概念，但不应把其 Mnesia 架构直接等同于现行 v3。[历史官方文档仓库](https://github.com/emqxarchive/emqx-docs-en/blob/master/source/cluster.rst)

## 6. 百万设备的容量推导

以下是数据结构推导，未测得实际内存字节。假设 100 万设备在线，分别订阅两个设备独有的精确 Topic，200 万条普通订阅均匀分布在 A/B/C，排除系统订阅、额外业务订阅及暂态冗余。

| 每个节点的逻辑数据规模 | TBMQ | EMQX |
|---|---:|---:|
| 本地实际设备连接 | 约 33.3 万 | 约 33.3 万 |
| 用于普通消息投递的客户端级订阅关系视图 | 全集群约 200 万 | 本地约 66.7 万 |
| 集群 TopicFilter→Node 路由 | 使用订阅索引结合会话元数据确定节点 | **仍约 200 万条** |

EMQX 路由条目数近似等于所有普通订阅中不同 `(TopicFilter, Node)` 二元组的数量。设备独有 Topic 无法借助同节点多客户端订阅相同 Topic 来合并，因此在此场景仍有约 200 万条全局路由。反之，若 100 万设备都普通订阅同一个型号 OTA 过滤器，并分布在 3 个节点，则该过滤器只产生约 3 条普通路由，但各节点本地仍保留自己的客户端成员关系。这个推导来源于第 2 节首次订阅加路由、最后订阅删路由的代码。

因此，不能仅由“EMQX 普通订阅明细是本地的”推导出每节点全局索引很小，也不能仅由“TBMQ 复制了客户端级订阅”断言其总成本一定更差。数据结构字节开销、Topic 长度、会话选项、索引实现、连接内存、恢复与更新吞吐均需在相同业务负载下实测。

## 7. TBMQ 固定版本核验

重新沿启动恢复、实时更新及消息匹配三条路径检查后，结论为：**TBMQ v2.4.0 的各 Broker 保存客户端级全局订阅元数据，确实不只是 TopicFilter→Broker 的节点级路由。** “全量”的准确范围是集群当前有效、仍保留会话的订阅关系；异步传播可能暂时落后。它不代表全部历史发布 Topic、每台设备的完整运行状态或全部 socket。

### 7.1 从 Kafka 全量恢复到本地客户端级 Trie

| 环节 | 固定源码中的实际行为 | 证据 |
|---|---|---|
| Kafka 记录 | clientId 是 record key；value 是包含订阅集合和 source 的 ClientSubscriptionsProto，不把 clientId 误写成 value 内字段 | [记录 key](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/SubscriptionPersistenceServiceImpl.java#L124)、[protobuf](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/common/queue/src/main/proto/queue.proto#L136) |
| 节点启动 | 每节点以 serviceId + 启动时间建独立消费组；initLoad 收集全部客户端订阅快照 | [独立组及初始化](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ClientSubscriptionConsumerImpl.java#L83) |
| 启动过滤 | BrokerInitializer 移除找不到会话的 MQTT_CLIENT 订阅，再交完整 map 初始化；没有仅保留本节点客户端的过滤条件 | [初始化与过滤](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/BrokerInitializer.java#L208) |
| 本地索引构建 | 遍历全部传入客户端，写 clientSubscriptionsMap，再调用 subscriptionService.subscribe(clientId, subscriptions) | [ClientSubscriptionServiceImpl.init](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/subscription/ClientSubscriptionServiceImpl.java#L73) |
| Trie 值 | 按 TopicFilter 写入 ClientSubscription，内容为 clientId、QoS、shareName、options、subscriptionId；不是 brokerId | [Trie 写入](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/subscription/SubscriptionServiceImpl.java#L49)、[ClientSubscription 字段](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/subscription/ClientSubscription.java#L29) |

因此百万设备各有两条有效普通订阅时，各节点的索引需要表示约 200 万条客户端级关系。Trie 可共享前缀，clientId 字符串和对象也可能共享引用；本报告不把逻辑关系条数直接等同于固定内存字节数或完全独立的对象副本数量。

### 7.2 远端实时更新也会进入索引

实时链路为 `ClientSubscriptionConsumerImpl.processPack → ClientSubscriptionServiceImpl.processSubscriptionsUpdate → ClientMqttActorManagerImpl → ClientActor → SubscriptionChangesManagerImpl`。来源为本节点、已经本地执行过的变更会跳过；远端变更即使没有已有本地 Actor，也会创建 Actor 处理。订阅变化管理器对新旧集合做 diff，再调用内部 subscribe/unsubscribe，没有按“客户端必须连接在本节点”筛选。[远端事件与 Actor 创建](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/session/ClientMqttActorManagerImpl.java#L169)、[Actor 分派](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/ClientActor.java#L163)、[订阅差异应用](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/subscription/SubscriptionChangesManagerImpl.java#L36)

这闭合了“启动时全量恢复”和“运行中继续维护全局视图”两条证据链；不是仅凭独立 consumer group 推断内存必然保留全部订阅。

### 7.3 会话元数据、路由结果与本地连接

ClientSessionServiceImpl 初始化时复制全部会话，实时接收其他节点的 put/remove；ClientSessionCtxServiceImpl 另有由实际连接注册/注销的本地 context map。消息分发先从 Trie 得到客户端订阅，再查询全局会话元数据确定所属 Broker；全局元数据与本地 channel/TLS 连接上下文是不同对象。[全局会话初始化](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/actors/client/service/session/ClientSessionServiceImpl.java#L57)、[本地连接 map](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/mqtt/client/session/ClientSessionCtxServiceImpl.java#L37)、[分发查会话](https://github.com/thingsboard/tbmq/blob/2f60cfef8e058f4cd663b46bc4553ff23dfe89cc/application/src/main/java/org/thingsboard/mqtt/broker/service/processing/MsgDispatcherServiceImpl.java#L225)

本次源码重审保留前面的 TBMQ 结论，同时补足全量范围和证据。与 EMQX 比较时必须区分“客户端级订阅关系”与“节点级路由记录”；两者都可能具有百万级全局索引，不能只用全量/非全量两个词评价容量或成本。
