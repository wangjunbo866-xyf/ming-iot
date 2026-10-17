---
status: observed
version: observed-v0
source_revision: c304bccd1663ee1299dc73d99855c61339b6ef4a
---

# Eventbus：当前源码现状

这是首次接入规格体系的现状基线，证据为上述固定提交的源码，**不是已批准的可靠性承诺，也不是运行验收**。迁移决定和剩余提案在 [Kafka Patch（当前 R3）](patches/PATCH-20261017-kafka-migration/change.md)，原话在 [raw.md](raw.md)。没有独立 Eventbus 当前规格的历史缺口在这里显式保留。

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
