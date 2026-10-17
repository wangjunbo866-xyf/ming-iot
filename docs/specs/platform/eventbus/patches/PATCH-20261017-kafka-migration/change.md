---
status: draft
revision: R2
---

# PATCH-20261017-kafka-migration：跨服务 Eventbus 从 NATS 迁往 Kafka

- 类型：patch；替换既有跨服务事件传输并补齐目标契约，不能声明“规格无变化”。
- 模块：[platform.eventbus](../../README.md)；当前基线：[observed-v0](../../spec.md) 的 BUS-SCOPE、BUS-ENVELOPE、BUS-DISPATCH、BUS-DELIVERY、BUS-VERIFY。模块 README 反向指向本 Patch。
- 来源：[raw.md](../../raw.md)。原话只追加；R1 为首次受理，R2 为调用盘点和选项细化。D1–D4 继续适用，新增 D5；R2 的建议不因写进文档就成为用户决定。
- 本轮认领：用户决定业务取舍；`rdoc-pilot-codex` 盘点与维护文档；独立 AI 复核源码、方案与候选。长期模块负责人未分配。共享影响含 Share、SaaS、Things、协议 SDK、定时、缓存与 WebSocket。
- 本轮只改需求、导航与资料索引；保留已有候选，不切换实现、不启动迁移、不推送、不操作旧环境或旧数据。后续实施另核授权、路径认领与规格映射。
- 当前交付：本地需求草稿；没有 Kafka 迁移运行、托管 CI、生产验收或正式团队采用结论。

## 1. 已确认什么

| 决定 | 真实来源 / 日期 | 已确认范围 | 不包含 |
| --- | --- | --- | --- |
| D1 | RAW-001；用户，2026-10-17 | Eventbus 从 NATS 全面迁到 Kafka，先细化 | 未批准所有可靠性与路由设计 |
| D2 | RAW-002；用户，2026-10-17 | 仅开发测试、按一次切换设计 | 不需要在线客户双写/滚动兼容；不授权丢弃旧消息 |
| D3 | 环境准备授权；用户，2026-10-17 | 隔离通知基线，可重复启动 | 保留旧环境、旧仓、备份、历史；不接生产或删除数据 |
| D4 | RAW-003；用户，2026-10-17 | 用真实需求沉淀团队流程与 harness | 不等于方法正式采用 |
| D5 | RAW-004；用户，2026-10-17 | 在同一 Patch 给出调用范围、可靠性、旧消息、验收的具体选项与推荐 | 本轮不切换实现或推送；旧环境和数据保留 |

下文 Q2a、Q2b、Q3 是需要人的业务决定；Q1、Q4 和技术配置是有依据的建议，仍可调整。没有回答的项目保持 pending，不使用默认选项替人确认。后续答复追加来源及决定适用范围，改变方案含义时再升 revision。

## 2. Q1：全面迁移覆盖哪些调用

盘点基于 `25c9a446ba4fc4c992d4d2002dd73e3388054170`；是源码证据，不是部署清单。**推荐范围 S1：仓内实际跨服务事件链及直接 NATS 调用全部承接，历史/未接线能力明确处置，目标开发组合启动和运行无需 NATS。** 仓外 SDK 使用者尚未知，不能声称已替它们迁移。

替代方案 S0 是仅给 FastEvent 增加 Kafka：成本低，但 DG、定时等直连仍依赖 NATS，不能满足 D1。S2 是同时重做 MQTT 网关、HTTP/RPC 和定时调度体系：范围大、缺业务依据，不推荐。外部设备仍用 MQTT，HTTP/gRPC、Redis/Asynq 调度和进程内 AsyncEventBus 保持职责；其内部跨服务事件段纳入 S1。

| 编号 / 消息族 | 当前发布 → 消费与用途（源码入口） | 目标承接和盘点边界 |
| --- | --- | --- |
| C1 设备上行 | [DG pubInner](../../../../../../backend/things/service/dgsvr/internal/repo/event/publish/pubInner) / [SDK Core](../../../../../../backend/things/sdk/protocol/coreProtocol.go) → `device.up.*` → [DM InitEventBus](../../../../../../backend/things/service/dmsvr/internal/startup/init.go) 的 thing/ota/ext/config/log/shadow/gateway/status 处理 | 真实 DG 与 SDK 库分别验收；同一逻辑 DM 消费组竞争处理，不能每个副本重复执行业务。SDK 库有实现不证明仓外实例已接入；shadow 已注册但业务处理仍 TODO，本迁移不顺手补影子功能 |
| C2 设备下行与回执 | [DM pubDev](../../../../../../backend/things/service/dmsvr/internal/repo/event/publish/pubDev) → [DG subInner](../../../../../../backend/things/service/dgsvr/internal/repo/event/subscribe/subInner) / SDK 下行订阅；应答经上行返回 | 直接 NATS 与 FastEvent 两路都检查；保留协议、设备、请求关联，明确超时和迟到回执；Kafka 成功写入不代表设备执行成功 |
| C3 SaaS 跨域事实 | [用户/项目/区域管理](../../../../../../backend/saas/service/syssvr/internal/logic) 发布 `server.core.*` → DM InitEventBus 清理绑定、共享关系或重算 | 用户/项目/区域删除有实际消费者。用户创建/更新、租户创建、工单完成等有发布，不将仅有 topic 常量的下游补成虚构链路 |
| C4 Things 生命周期与协议配置 | [设备/产品/协议管理](../../../../../../backend/things/service/dmsvr/internal/logic) 发布 `server.things.dm.*`；[DG server订阅](../../../../../../backend/things/service/dgsvr/internal/repo/event/subscribe)、SDK Cloud 配置订阅等 | 核对每个消费者实际注册和启动；配置/生命周期按对象版本处理，不能把“收到最后一条”当全量配置恢复 |
| C5 缓存失效广播 | [Cache.SetData / Subscribe](../../../../../../backend/share/caches/cache.go) → `server.cache.sync.<keyType>` → 所有持有本地缓存的实例，删除本地项及通知插槽 | 必须所有相关活跃实例失效，不能共用竞争组导致仅一个实例清缓存；重连需恢复一致性，缓存故障不可变成永久旧权限 |
| C6 WebSocket 实时广播 | [DM ServiceContext](../../../../../../backend/things/service/dmsvr/internal/svc/serviceContext.go) 的 UserSubscribe → `CoreApiUserPublish` → [SaaS API ServiceContext](../../../../../../backend/saas/service/apisvr/internal/svc/serviceContext.go) 的 StartWsDp → 本节点在线连接 | 实际入口使用 SaaS websocket；[Share实现](../../../../../../backend/share/websocket)与[SaaS同名目录](../../../../../../backend/saas/share/websocket)都盘点，不能仅迁移同名副本。各 API 实例均需获知，断线用户补当前快照 |
| C7 定时结果发事件 | [ProcessTask.Queue](../../../../../../backend/saas/service/timed/timedjobsvr/internal/logic/processTask/queue.go) → [PubJob](../../../../../../backend/saas/service/timed/internal/repo/event/publish/pubJob/job.go) → 任务配置指定 subject | 直接 NATS 生产路径；数据库中的 topic/GroupSubType 也是迁移对象，不能只全文替换 Go 文件。PubJob 的 natsJs 构造与 Publish 分支不一致是现状缺陷，需实施时处理 |
| C8 收事件触发任务 | [QueueTrigger](../../../../../../backend/saas/service/timed/timedschedulersvr/internal/timer/queueTrigger.go) 的 task.Topics 动态订阅 → TaskSend → SQL/script/queue 等任务 | 每个逻辑任务都能收到匹配事件，任务副本不重复调度；保留订阅启停及原始 subject。SQL/script 不能统一视为可安全重试 |
| C9 内部周期协作 | [DM InitEventBus](../../../../../../backend/things/service/dmsvr/internal/startup/init.go) 的 OTA/在线统计等；[syssvr startup](../../../../../../backend/saas/service/syssvr/internal/startup/init.go) 的周期同步；[timedjob startup](../../../../../../backend/saas/service/timed/timedjobsvr/internal/startup/init.go) 的清理订阅 | DM 显式 Start；独立 syssvr/timedjob 的注册不能证明 Start 已执行。启动验收需覆盖独立进程和实际组合，历史未启动缺陷不保留为目标行为 |
| C10 应用事件、旧接口与配置 | [pubApp](../../../../../../backend/things/service/dmsvr/internal/repo/event/publish/pubApp/pubApp.go)、旧 [subDev](../../../../../../backend/things/service/dmsvr/internal/repo/event/subscribe/subDev)、[server订阅](../../../../../../backend/things/service/dmsvr/internal/repo/event/subscribe/server)、[EventConf](../../../../../../backend/share/conf/event.go)、部署样例 | subDev 启动调用被注释且 NATS Subscribe 为空，不能统计成运行链；对其、pubApp 下游及其他无已知消费者条目逐项标“迁移/停用/保留兼容代码”，不能静默漏掉。当前活动配置与SDK样例须无 NATS 必需依赖；历史资料、旧环境不删 |

实施前形成 C1–C10 的逐 subject / 发布 API / 消费 API / 启动方式 / 测试对应表，并扫描 `NatsClient`、`nats.Conn`、`nats.Subscription`、FastEvent、动态 task.Topics 和部署配置；当前分族清单不宣称穷尽仓外调用。无法证明已停用的调用不能从最终范围直接排除。

## 3. Q2：按业务后果选择可靠性

### Q2a：重要事实发生后，是否要求最终补处理

**推荐 A：核心事实可靠补处理，允许内部重投，但业务结果不能重复。** 例如“项目已删除”的事实在短暂故障后仍须完成设备关系处理；设备上报已被入口持久接受后，消费者重启可继续处理。成本是增加事件身份、可靠待发记录、去重和失败可见性，不能仅替换客户端。

替代 B：本次只达到“成功进入 Kafka 后可恢复”，数据库改完但消息未发出的缺口暂时保留。改动较小，但会出现“项目删了，设备关联未处理”，不能称业务不丢事件。不推荐把这种限制藏在技术细节中。

| 类别 | 推荐的目标规则（待 Q2a / Q2b 确认） | 顺序、重复与失败处置 |
| --- | --- | --- |
| F1 数据库产生的生命周期事实（C3/C4） | 业务数据与待发事件同事务落盘（outbox：待发记录）；发送失败保留待办并告警；消费结果持久成功后才提交消费进度 | 以 tenant + 对象 ID 分组；eventId 稳定，重复处理不重复解绑/新增记录；复合副作用逐步记账。不能用 Kafka 事务替代数据库原子性 |
| F2 已持久接受的设备事实（C1） | 入口持久确认后可恢复；下游写入成功再提交进度。入口失败应明确失败/背压，禁止只打印日志后当成功 | 同设备键保留平台接受顺序，去重有稳定来源 ID；设备原始报文无稳定 ID 时不得承诺去重掉所有设备重发。设备采样时间乱序与 Kafka 顺序分别处理；不是全球时序承诺 |
| F3 指令、OTA触发及有副作用任务（C2/C8/C9） | 任务投递可恢复，具体设备动作是否重发按 Q2b；带 commandId/任务执行ID、有效期和结果状态 | 调度重复不等于执行重复。按设备/任务实例串行，回执关联同一请求；迟到/过期不能再次触发动作；任意 SQL/script 不自动补执行 |
| F4 缓存与配置刷新（C5及C4配置） | 传播当前状态，允许重复失效；重启/重连先失效并重读权威状态，随后继续接收，处理恢复窗口中的更新 | 不要求逐条回放旧失效通知；必须多副本都更新。权限/租户缓存恢复未完成时不得继续信任旧值；这部分仍需安全回归 |
| F5 在线展示（C6） | 活跃 API 实例广播到本地订阅者；断线后读取当前快照，不补播全部过期刷新 | 不是站内持久通知替代品。重复刷新不产生重复持久消息；不能把 websocket 的 best-effort 政策套到 F1/F2 |
| F6 定时消息（C7/C8/C9） | 按最终业务载荷归类；历史事实走 F1/F2，指令走 F3，可重新计算的统计采用明确时间窗 | taskId + 计划时间/触发eventId 区分同一任务与不同轮次；漏掉的过期操作不能恢复后盲目补跑 |

可靠性的起点要分清：HTTP业务事务、DG/SDK收到报文、Kafka确认、业务数据库提交、设备确认是不同阶段。当前[项目删除](../../../../../../backend/saas/service/syssvr/internal/logic/projectmanage/projectInfoDeleteLogic.go)在数据库事务之后 Publish；[FastEvent](../../../../../../backend/share/eventBus/fastEvent.go)异步 handler 不把结果交回传输层。A 需要改变这些边界和失败返回链，不能沿用提前 ACK 后宣称可靠。

推荐首次隔离验收采用“小批数据 + 受控重启”验证，保留窗口至少覆盖测试停机与重试窗口；初始候选为 72 小时，去重记录至少覆盖消息可能重放的窗口。该数值是待测试的开发参数，不是生产 SLA；不会用于修改旧 NATS 保存策略。

失败处理建议：可重试错误先有界退避，超过次数进入可查询的失败记录并告警；有序业务停止该分区继续推进，保留后续积压。不能“移到失败队列后继续后面的事件”同时宣称原顺序不变。恢复和跳过要有明确操作记录；取消、死信和过期属于可观察终态，不是成功。具体重试间隔、容量、分区数由技术方案和故障测试确定，不让用户猜参数。现有部分 SDK 定时、DM OTA/在线状态/统计回调会丢弃超过 2 秒的消息；逐项按 F3/F6 决定过期或补算，不能机械搬运过滤后宣称可恢复。

### Q2b：设备可能执行成功、但回执丢失，如何处理

例子：平台发送一次设备操作，设备已执行，网络在回执返回前断开。**推荐 A：显示“结果待核实”，不因超时自动再执行；优先查询状态，人工确认后才发新操作。** 尚未发到设备的持久待办可以在有效期内继续投递；真正无法判断是否已发送的情况不能当成“肯定未执行”。

替代 B：仅对已经证明设备端按同一 commandId 去重、并且有明确有效期的操作自动重发；其他操作仍用 A。成本是逐协议验证设备能力，Kafka 的 producer 幂等无法代替设备端去重。

不推荐 C：所有超时统一重发。比如动作型指令可能执行两次，OTA/脚本也可能重复副作用。当前同步调用默认约 5 秒，部分调用自定超时；这不是目标统一有效期。更关键的是，[异步 Action](../../../../../../backend/things/service/dmsvr/internal/logic/deviceinteract/actionSendLogic.go)经定时任务与[serverEvent](../../../../../../backend/things/service/dmsvr/internal/event/serverEvent/server.go)已存在检查并重发的路径。因此 A 将收紧现有自动重发行为，是需要用户确认的变化；B 也需证明设备能力，不能把当前重发当安全证明。无论选 A 或 B，都不能承诺硬件动作严格“恰好一次”。

## 4. Q3：旧消息与返回旧版本

**推荐 H1：保留原环境及数据，旧消息不自动导入 Kafka；新隔离环境从独立测试数据开始。** 例子：昨天的一条测试设备操作留在旧系统用于查证，但明天启动 Kafka 不会重新执行它。已有业务记录也不删除；“不导入消息”与“清空数据库”是两件事。

| 选项 | 实际做法 | 代价 / 适用场景 |
| --- | --- | --- |
| H1 推荐：保留旧现场，新消息起步 | 后续获准切换时先停止旧入口和调度，记录在途/失败清单，再按现有私有备份方式保存配置、数据及可保存的消息；建立独立新环境和新消费组 | 最小开发切换成本；旧积压不自动成为新系统待办。需要用户接受这个业务边界 |
| H2：先处理旧待办，再切换 | 区分事实和指令，人工核对结果未知/过期项；只让批准的旧工作完成，完成清单核对后切换 | 适合确有必须完成的开发待办；“清空队列”不等于业务完成，旧实现的提前 ACK 可能掩盖未处理项 |
| H3：选择性导入旧事实 | 仅导入清单批准的事实，保留原ID/来源时间、映射与数量核对；指令、过期触发和历史页面刷新不自动重放 | 迁移成本最高；须先建立去重和副作用隔离，再独立验证导入，不将方案批准当成执行授权 |

保留限制：Core NATS 本身不保存离线历史；当前 JetStream 配置有两分钟 MaxAge 等保留限制，过去已失去的消息不能从不存在的历史中恢复。保留旧容器/卷也不会自动冻结仍在运行的过期策略。需要快照时必须在后续获准操作中明确暂停/备份时点、快照可恢复性和无法恢复部分；本轮没有连接旧 broker 或调整其策略。

返回旧版本的原则：新旧环境和数据分开，先停新入口再恢复旧入口，避免同一真实设备/调度双重执行。不能仅切回旧二进制就声称新产生的数据和已执行动作也回退了；先核对新旧差异，禁止双写补偿或把新命令倒灌旧系统。本轮只制定方案，不实施这些动作。

## 5. Q4：如何验收，不用一个通知测试冒充全迁移

推荐先运行 V1 验证广播语义，再以 **V2 设备上报 → DG → Kafka → DM → 物模型属性持久化** 作为首条真实业务链。已有通知 RPC 基线仅作 V8 回归。下表全部为拟实施验收，当前未运行。

| ID / 覆盖 | 输入与故障场景 | 可观察的通过条件 |
| --- | --- | --- |
| V1 广播 / C5 | 两个真实 Cache 实例预热同一测试对象；通过 SetData 更新；一个实例断开再恢复 | 两个实例最终均读取新状态；恢复后不得继续使用旧值；竞争消费不能漏掉另一个实例。权限缓存另验恢复边界 |
| V2 上行 / C1 | 隔离 MQTT 模拟设备，走现有 DG 入口，DM处理真实 property报文；两个 DM 副本；至少两台测试设备和不同租户 | 不启动 NATS 仍有实际属性写入，租户/设备不串；逐条核对输入ID和存储结果，关键进度可追踪。不以临时测试消费者代替 DM |
| V3 事实可靠性 / C1/C3/C4 | 消费前宕机、数据库提交后但进度提交前宕机、broker断开、重复投递；对事务型事件注入数据库已提交/发送未完成故障 | 依 Q2a 的已确认起点恢复；同eventId不重复副作用；发送缺口有待发记录，错误/失败积压可查询。涉及删除的用例只在将来获准的新隔离样例中运行 |
| V4 指令 / C2 | 协议模拟器记录commandId及执行次数，丢回执、迟到回执、重复投递、过期、目标离线和目标实例切换 | 路由到正确设备；按 Q2b 得到成功/明确失败/待核实/过期，不能把 Kafka ACK 当设备成功；重复回执不重复动作 |
| V5 路由 / C7/C8/C9 | 两个相同任务副本、两个不同逻辑任务同时匹配；启停/重新启用动态订阅，验证通配和不匹配消息 | 同一任务不重复执行，不同任务都能获得各自事件；重启不重建并丢失消费进度；停用后不再调度；旧任务配置有明确映射或阻止启动的诊断 |
| V6 顺序/坏消息/隔离 | 同一设备连续变化，注入第一个失败、非法payload、缺租户/无效追踪、其他设备正常消息 | 同键后续不越过失败项；不承诺跨分区全局顺序。无效追踪不得静默丢掉租户身份，非法租户拒绝或隔离；失败记录不伪装成功 |
| V7 覆盖与切换 / C1–C10 | 新组合的各独立服务及SDK测试入口；检查配置、直连与topic映射；按 H1/H2/H3 演练 | 新组合无 NATS 运行依赖，清单每项有结果或明确待办；旧环境/数据仍在并可按约定恢复；未纳入的仓外使用者单列，不能宣称已完成 |
| V8 无关回归 / C6及通知 | API实例多节点在线订阅与重连快照；现有通知四场景；API校验与既有测试 | 实时消息发到正确订阅者，断线不补播全部旧刷新；通知昵称及持久化规则保持原样；真实事件验收与通知结果分别记录 |

V2 仍须补齐隔离 DG/DM、真实存储与模拟设备配置；现有通知容器不足以证明它能运行。V1通过也不能提前关闭 V2–V7。容量、吞吐与延迟先记录基线再给数值，不凭文档承诺生产性能。

## 6. 技术建议、缺口与推进顺序

以下是 AI 技术提案，不要求用户选择 Kafka 参数：

- 少量按职责划分的物理 topic；原 subject、事件类型、schema版本、租户、对象、时间和追踪进入受控 envelope。设备 key 含租户/产品/设备；不能每台设备或每条 NATS subject 新建 topic。通配在受控路由层解释并做正反例，不把 NATS `*`/`>` 原样当 Kafka topic 匹配规则。
- 竞争消费按逻辑服务/任务分组；广播按需要接收的实例区分，给实例和组明确稳定性/生命周期。设备长连接属于哪个实例需显式路由，不能让任意同组实例拿到下行后因没有连接而丢掉。
- 稳定 eventId、必要的对象版本、关联ID和 expiresAt；保留现有 payload/context 的业务含义，同时校验租户与错误路径。相同 key 只帮助分区顺序，多生产者、跨topic、设备原始采样乱序和消费并发还需另外约束。
- 复用现有 [Kafka foundation](../../../../../../backend/share/foundation/kafka) 的 producer/consumer 安全构造；现有 [topic catalog](../../../../../../backend/share/contracts/kafka/catalog.go)只接受数据平面既定模板，[RawRecord](../../../../../../backend/share/contracts/kafka/raw_record.go)仅允许有限原始数据headers，不能把控制事件硬塞进去或全局放宽校验。实施前为控制事件明确独立契约或有界扩展，并保留现有测试。
- 现有 FastEvent 是进程单例，首次构造的配置和 serverName 决定身份；不能按每个 ServiceContext 各自名称推断消费组。现有 DG 的 `device.down.urMqtt.>` 与 `.status.fix` 订阅重叠，组也不同，必须用真实消息确认分流，不让状态修复误进设备指令路径。
- Consumer关闭自动提交只是起点；业务成功/失败必须传回提交边界，异步 handler、吞掉的错误、多个副作用和下游外部操作要逐项整改。Kafka事务不自动覆盖 PostgreSQL、Redis、WebSocket或设备动作。

推进顺序：本轮完成 R2、用户选项与独立复审 → 根据实际答复收敛决定 → 补逐调用方映射、目标规格和实施切片 → 另行获准后构建隔离 V1/V2 与故障验证 → 实现其他族并完成 V3–V8 → 整体复审后收敛主规格与吸收关系。机器检查投影在进入行为实施前补齐，当前不生成假装已经 confirmed 的 check.json。

现阶段本 Patch 的完成标准是“源码范围有依据，选择及后果可解释，待决项与验证方法明确”；不是业务迁移完成。团队维护成本记录在本轮 Goal 回执：文件数、实际人决策数、复审发现与返工；未测量工时写 unknown，不能用文档完整度推导收益。

## 技术依据与适用性

2026-10-17 核对官方资料，仅用于解释机制；业务政策仍以上面的真实决定为准：

- [Apache Kafka Design：交付语义](https://kafka.apache.org/41/design/design/)：处理后提交可能重投；Kafka内部事务不自动包含外部存储或设备动作。
- [NATS Core概念](https://docs.nats.io/learn/core-nats/)与[JetStream概念](https://docs.nats.io/concepts/jetstream)：Core无离线历史，JetStream的实际恢复能力受流/消费者配置约束。

未升级broker/SDK版本，未借参考资料改变当前业务权威。源码现状在主规格，R2保存迁移建议、选择与验收；同一问题不另建平行规格。
