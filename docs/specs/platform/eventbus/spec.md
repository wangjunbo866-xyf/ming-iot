---
status: confirmed
version: p1-v1
---

# Eventbus：当前候选规则与保留的旧路径

本版本只收敛 [Kafka Patch R9](patches/PATCH-20261017-kafka-migration/change.md) 第 10 节已授权的 **P1 显式接入能力**；不是全面 NATS 切换或线上生效。P0 基线为 `6a59904edbf7d019e266fe910fb1436d3b1ee4c4`。最终候选 SHA、真实验证与独立评审由同一 Goal 外部回执绑定，部署事实另记。原话及确认见 [raw.md](raw.md)。

<a id="p1-contract"></a>
## BUS-P1-CONTRACT：受控公共契约

- [contracts/eventbus](../../../../backend/share/contracts/eventbus) 与已有 data-plane catalog / RawRecord 分离；P1 只开放 `coord.cache.invalidate`、`iot.bus.coordination.v1.g01`。其它通道仍是设计，没有声明可执行 schema。
- JSON 信封含稳定 eventId、type/schemaMajor、source、occurredAt/acceptedAt、scope/tenant、object、correlationId，causationId 可选；payload 是 CacheInvalidation{cacheType,key}。对象身份、payload、legacySubject 与 JSON tuple 分区键必须一致；身份不是业务授权。
- Codec 限长并拒绝未知 type/major、重复键、字段别名、null、未知字段、畸形载荷或错配身份。P1 选择严格字段白名单：新增可选字段也须先升级消费者，不承诺旧消费者自动前向兼容；这收窄 P0 的兼容性提案。
- 显式 KafkaBus 仅接受可信 system 缓存，不接受 tenant 信封、不解释任意 subject。`PublishEnvelope` 可复用事件身份，普通 Publish 每次调用生成一次身份；确认超时不能解释为 broker 一定未收到。失效本身可重复执行，不能借此宣称所有业务幂等。
- 选择器工具校验精确段、`*` 一个非空段、末尾 `>` 一个或多个段；仅测试兼容匹配语义，没有接入定时任务调度或数据库订阅配置。

<a id="p1-completion"></a>
## BUS-P1-COMPLETION：消费完成与身份

[foundation bus](../../../../backend/share/foundation/kafka/bus.go)使用显式 group、read-committed、禁自动提交、有界 poll 和串行同步 callback。只有 callback 成功返回后才向 broker 提交该记录；处理、解码、提交失败停止当前消费，后继记录不越过失败项。处理后的崩溃或提交结果未知仍可重投，因此业务去重、outbox 与设备动作账本留待后续阶段。

首版按一个 consumer 串行执行全部 key，保证同键副作用及分区成功前缀；暂不声称不同 key 的并行推进。不同 KafkaBus 实例拥有自己的客户端与生命周期，不使用 FastEvent 的全局第一 serverName。广播组包含稳定 role/instance；同组副本竞争使用 foundation 的显式 group，缓存实例不得共享同一实例身份。调用者负责稳定唯一实例 ID 与退出组回收。

取消与 rebalance 使缓存失去 ready；同步 handler 必须响应 context，不能在 handler 内等待 Close。无法安全继续的处理/提交错误 fail-stop。独立 consumer 可按相同稳定身份重新构造并 Run；接入 Cache 的服务必须重启整个缓存进程，因为同 KeyType 的 NewCache 不会绑定新 bus。空闲期间的可恢复断网仍在原 bus、原进程内恢复。不通过跳过坏消息或自动推进 offset 恢复；没有持久隔离队列或管理员修复界面。

<a id="p1-cache"></a>
## BUS-P1-CACHE：真实缓存恢复

- [KafkaBus](../../../../backend/share/eventBus/kafka.go)以显式构造接入 [Cache](../../../../backend/share/caches/cache.go)，旧配置工厂不变；每个 Cache 仍是进程内按 KeyType 单例。广播验收必须使用两个独立进程。
- `CacheState` 给出 generation 和 ready；创建客户端不算 ready，需分区分配、逐分区订阅位点确认及当前代际全部已分配分区 leader 的成功查询。任一 broker 可 Ping 不代表协调分区可消费；空增量分配保留仍拥有分区的确认，新增分区单独等待确认。检测到断连、重分配或处理失败后撤销 ready，generation 改变；网络故障检测有请求超时窗口，不承诺物理断网瞬间就知道故障。
- Cache 在读取前同步代际，变化时清 L1；未 ready 明确返回 ErrCacheNotReady，不继续返回旧权限值。读期间收到通知或发生恢复时，旧结果不能回填，返回 ErrCacheInvalidated 后允许重新读取。
- 新路径从 Redis 或配置的权威回调恢复；禁用旧的异步 DB→Redis 回填，避免删除后旧在途读取再次写回。任意键通知保守地让同一 Cache 其它在途读重试，有吞吐成本；Redis miss 也可能增加 DB 读取。
- `SetData` 的 Redis 与 Publish 错误必须返回，不能吞错后宣告成功。两步非原子，错误时 Redis 可能已更新；不能据此保证所有远端缓存立刻失效，需要调用者协调重试/修复。旧事件实现也获得此错误传播修正，但不自动获得 Kafka 代际恢复协议。

<a id="p1-validation"></a>
## BUS-P1-VERIFY：复验与边界

执行 `make verify-pr`、Share 相关包 race，以及 `make verify-integration VERIFY_SUITE=eventbus`。隔离运行入口为 [run.py](../../../../deploy/profiles/eventbus-baseline/run.py)，详细冻结参数与运行回执仍在同一 [Patch 第 10 节](patches/PATCH-20261017-kafka-migration/change.md)。测试专属 Kafka/Redis 与两个真实 Cache 进程，无宿主端口、不复用旧卷，停止保留数据。

P1 不覆盖业务 outbox、设备命令、完整通道接线、动态任务或 D12/V9 遥测积压隔离。后续只按实际完成的族吸收规则；整个多阶段 Patch 继续在途，不提前标 absorbed。

## 保留的 NATS 运行路径现状（observed-v0）

以下为 `c304bccd1663ee1299dc73d99855c61339b6ef4a` 的来源观察。P1 新入口没有替换这些业务调用，已知缺陷不是批准规则。

<a id="bus-scope"></a>
## BUS-SCOPE：边界与实际使用者

[FastEvent](../../../../backend/share/eventBus/fastEvent.go)是跨服务入口，当前工厂实现 nats、natsJs、mqtt；[配置](../../../../backend/share/conf/event.go)虽列出 direct，工厂未实现该分支，也未实现 Kafka。另有调用方直接使用 [NatsClient](../../../../backend/share/clients/nats.go)，只修改 FastEvent 工厂不能完成迁移。[AsyncEventBus](../../../../backend/share/eventBus/eventbus.go)是进程内分发，应与跨服务传输分别盘点。

| 调用范围 | 当前源码入口 | 迁移需核对的职责 |
| --- | --- | --- |
| 设备上报、下行及协议 SDK | [协议核心](../../../../backend/things/sdk/protocol/coreProtocol.go)、[DG 事件目录](../../../../backend/things/service/dgsvr/internal/repo/event)、[DM 启动订阅](../../../../backend/things/service/dmsvr/internal/startup/init.go) | 动态 subject、通配、队列消费、指令和回执 |
| 应用与设备事实事件 | [DM 应用事件发布](../../../../backend/things/service/dmsvr/internal/repo/event/publish/pubApp/pubApp.go) | application 事件及上下文 |
| SaaS 与缓存 / 定时协作 | [SaaS API 启动](../../../../backend/saas/service/apisvr/internal/startup/init.go)、[syssvr 启动](../../../../backend/saas/service/syssvr/internal/startup/init.go)、[定时服务](../../../../backend/saas/service/timed) | 缓存同步、服务事件、定时触发、WebSocket 关联调用；实施前逐调用方登记 |

<a id="envelope"></a>
## BUS-ENVELOPE：消息与上下文

[events.go](../../../../backend/share/events/events.go)封装 JSON 消息，传递数据、时间、追踪与用户/租户上下文。迁移不能仅比较消息 payload；调用方依赖的 subject、上下文、发布时间与错误返回也需要验收。当前字段来自源码，未来格式及兼容要求见当前 Patch 的待决事项。

<a id="dispatch"></a>
## BUS-DISPATCH：广播、队列和回调

FastEvent 区分 Subscribe 与 QueueSubscribe，后者使用 serverName 作为队列；本地多个 handler 再异步调用。Subscribe 注册与 Start 启动是两件事：独立 syssvr 的启动代码注册事件但未调用 FastEvent.Start，SaaS apisvr 启动路径才调用它。syssvr RPC 测试通过不证明事件消费者运行。

直接 NATS 调用、通配 subject、动态订阅与实例身份都存在。Kafka topic、key、consumer group 如何承接这些语义是设计问题，尚无批准的映射。

<a id="delivery"></a>
## BUS-DELIVERY：可靠性现状与已知风险

- [NatsSubscription](../../../../backend/share/events/nats.go)在解码和业务回调前调用 Ack；FastEvent 的异步 handler 失败主要记日志。不能把客户端配置中的重投次数解释为业务失败会重试。
- NatsClient 的 JetStream 流创建配置为 InterestPolicy、DiscardOld、MaxAge 两分钟；订阅使用 DeliverLast，队列订阅还会删除同名 consumer 后建立。应按具体调用路径验证其后果，不把“持久消息系统”当成任意历史可重放的证据。
- Kafka 若改成业务成功后提交 offset，可靠性与重复投递行为都会改变。是否采用、失败重试上限、幂等责任和历史处理都需形成明确的目标规则。

<a id="verification-boundary"></a>
## BUS-VERIFY：现有基础与证据边界

[Kafka foundation](../../../../backend/share/foundation/kafka)、[contracts](../../../../backend/share/contracts/kafka)和[隔离 Kafka 配置](../../../../deploy/kafka/compose.yaml)已经存在；当前 SaaS / Things 业务运行链路尚未接入这些实现。已有 Kafka 工具或 broker 可用不等于 Eventbus 已迁移。

当前通知链为真实 NotifyManage RPC → 模板与触发人处理 → 站内消息持久化，见[通知主规格](../../saas/notification-trigger/spec.md)及[发送实现](../../../../backend/saas/service/syssvr/internal/logic/notifymanage/sendMsg.go)。仓内虽有场景告警 topic 常量，盘点未找到“设备告警/场景 → Eventbus → 通知 RPC”的完整实现。不得新增测试订阅器后称其为现有业务链。

此次现状盘点覆盖源码；真实依赖、故障恢复、全调用方交付与性能数据需分别补证。候选和最终复审身份由同一 Goal 的回执绑定。
