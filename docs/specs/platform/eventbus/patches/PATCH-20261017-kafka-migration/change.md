---
status: draft
revision: R8
---

# PATCH-20261017-kafka-migration：跨服务 Eventbus 从 NATS 迁往 Kafka

- 类型：patch；替换既有跨服务事件传输并补齐目标契约，不能声明“规格无变化”。
- 模块：[platform.eventbus](../../README.md)；当前基线：[observed-v0](../../spec.md) 的 BUS-SCOPE、BUS-ENVELOPE、BUS-DISPATCH、BUS-DELIVERY、BUS-VERIFY。模块 README 反向指向本 Patch。
- 来源：[raw.md](../../raw.md)。原话只追加；R1 为首次受理，R2 为调用盘点和选项细化，R3 记录 RAW-005 的真实答复，R4 按 RAW-006 试跑可选分析方法，R5 按 RAW-007 写回 Q5/Q6 的真实选择为 D10/D11；R6 按 RAW-008 核对 ThingsBoard 通道与消息模型，形成第 8 节建议及 Q7；R7 按 RAW-009 将真实 Q7 答复写为 D12，并关联 V9 与实施切片。R8 按 RAW-010 推进 P0 设计，逐调用映射、契约、路由、资源和验证落在第 9 节。D1–D12 继承有效；D9 仅确认方法与写回范围，不把分析建议算作用户批准。
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
| D6 | RAW-005 / Q2a；用户，2026-10-17 | 选择 A：重要事实故障后可靠补处理；业务修改与待发事件一起保存、消费去重 | 不接受“数据库改完但事件未发出”的发送缺口；不承诺尚未持久接受的报文或所有外部副作用恰好一次 |
| D7 | RAW-005 / Q2b；用户，2026-10-17 | 选择 A：设备操作超时或结果不明时标待核实，默认不自动重发；优先查状态 | 不将现有异步 Action 自动重发保留为默认；设备去重例外未逐协议批准；不承诺物理动作恰好一次 |
| D8 | RAW-005 / Q3；用户，2026-10-17 | 选择 H1：保留旧现场及数据，新隔离环境使用新测试消息，旧积压不自动导入 | 不删除旧记录；不授权现在暂停旧环境、备份变更或实际切换；旧消息保留受原有策略限制 |
| D9 | RAW-006；用户，2026-10-17 | 可选分析 Skill 接入团队约定与 ming-specs，并在同一 Kafka Patch 试跑、写回 | 不另建平行业务文档；不实施切换或推送；旧环境和数据保留 |
| D10 | RAW-007 / Q5；用户，2026-10-17；自 R5 适用 | 选择 A：同设备存在待核实旧操作时，提示旧结果未知，由操作者明确确认后允许提交一条新操作；旧记录继续待核实，查询照常 | 不是重发旧命令，也不把旧命令标为成功/失败；未确认不提交；不绕过既有权限、设备有效期等校验；不批准设备端自动重发例外 |
| D11 | RAW-007 / Q6；用户，2026-10-17；自 R5 适用 | 选择 A：原操作人可见关联清理未完成，由管理员查看失败原因并处理；继续 D6 的可靠补处理 | 不选择仅管理员可见；主记录已删除不表示关联清理完成；失败、取消或人工关闭不冒充成功；不授权本轮操作真实数据 |
| D12 | RAW-009 / Q7；用户，2026-10-17；自 R7 适用 | 选择 A：将“上报积压时，控制和重要业务补处理仍能独立推进”列为本次迁移必须验收；增加消费资源隔离与故障测试 | 不承诺具体毫秒 SLA、共享依赖整体故障时仍可服务或任意超载下可用；不授权实现、推送或旧环境数据操作；既有顺序、权限、确认与可靠性要求保持 |

Q2a、Q2b、Q3、Q5、Q6、Q7 的选择已确认，不再重复询问。Q1 的具体调用处置、Q4 的验收设计和技术配置仍是 AI 提案；其中与 D1/D6–D8/D10–D12 冲突的替代方案已排除。本文仍为 draft：方向确认、实现授权、运行通过分别记录。后续出现新的业务取舍时只追问受影响部分，已有决定继续适用。

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

实施前形成 C1–C10 的逐 subject / 发布 API / 消费 API / 启动方式 / 测试对应表，并映射到第 8 节的逻辑消息族；该表用于证明迁移无遗漏，不要求每个旧 subject 都建立同名 Kafka topic。另扫描 `NatsClient`、`nats.Conn`、`nats.Subscription`、FastEvent、动态 task.Topics 和部署配置；当前分族清单不宣称穷尽仓外调用。无法证明已停用的调用不能从最终范围直接排除。

## 3. Q2：按业务后果选择可靠性

### Q2a：重要事实发生后，是否要求最终补处理

**已选择 A（D6）：核心事实可靠补处理，允许内部重投，但业务结果不能重复。** 例如“项目已删除”的事实在短暂故障后仍须完成设备关系处理；设备上报已被入口持久接受后，消费者重启可继续处理。成本是增加事件身份、可靠待发记录、去重和失败可见性，不能仅替换客户端。

未选择的替代 B（仅保留比较依据）：本次只达到“成功进入 Kafka 后可恢复”，数据库改完但消息未发出的缺口暂时保留。改动较小，但会出现“项目删了，设备关联未处理”，不能称业务不丢事件。不推荐把这种限制藏在技术细节中。

| 类别 | 按 D6/D7 推导的目标设计（技术提案） | 顺序、重复与失败处置 |
| --- | --- | --- |
| F1 数据库产生的生命周期事实（C3/C4） | 业务数据与待发事件同事务落盘（outbox：待发记录）；发送失败保留待办并告警；消费结果持久成功后才提交消费进度 | 以 tenant + 对象 ID 分组；eventId 稳定，重复处理不重复解绑/新增记录；复合副作用逐步记账。不能用 Kafka 事务替代数据库原子性 |
| F2 已持久接受的设备事实（C1） | 入口持久确认后可恢复；下游写入成功再提交进度。入口失败应明确失败/背压，禁止只打印日志后当成功 | 同设备键保留平台接受顺序，去重有稳定来源 ID；设备原始报文无稳定 ID 时不得承诺去重掉所有设备重发。设备采样时间乱序与 Kafka 顺序分别处理；不是全球时序承诺 |
| F3 指令、OTA触发及有副作用任务（C2/C8/C9） | 任务投递可恢复，结果未知时按 D7 不自动重发；带 commandId/任务执行ID、有效期和结果状态 | 调度重复不等于执行重复。按设备/任务实例串行，回执关联同一请求；迟到/过期不能再次触发动作；任意 SQL/script 不自动补执行 |
| F4 缓存与配置刷新（C5及C4配置） | 传播当前状态，允许重复失效；重启/重连先失效并重读权威状态，随后继续接收，处理恢复窗口中的更新 | 不要求逐条回放旧失效通知；必须多副本都更新。权限/租户缓存恢复未完成时不得继续信任旧值；这部分仍需安全回归 |
| F5 在线展示（C6） | 活跃 API 实例广播到本地订阅者；断线后读取当前快照，不补播全部过期刷新 | 不是站内持久通知替代品。重复刷新不产生重复持久消息；不能把 websocket 的 best-effort 政策套到 F1/F2 |
| F6 定时消息（C7/C8/C9） | 按最终业务载荷归类；历史事实走 F1/F2，指令走 F3，可重新计算的统计采用明确时间窗 | taskId + 计划时间/触发eventId 区分同一任务与不同轮次；漏掉的过期操作不能恢复后盲目补跑 |

可靠性的起点要分清：HTTP业务事务、DG/SDK收到报文、Kafka确认、业务数据库提交、设备确认是不同阶段。当前[项目删除](../../../../../../backend/saas/service/syssvr/internal/logic/projectmanage/projectInfoDeleteLogic.go)在数据库事务之后 Publish；[FastEvent](../../../../../../backend/share/eventBus/fastEvent.go)异步 handler 不把结果交回传输层。A 需要改变这些边界和失败返回链，不能沿用提前 ACK 后宣称可靠。

推荐首次隔离验收采用“小批数据 + 受控重启”验证，保留窗口至少覆盖测试停机与重试窗口；R2 初始候选为 72 小时（R8 当前候选已在第 9.5 节改为 7 天），去重记录至少覆盖消息可能重放的窗口。该数值是待测试的开发参数，不是生产 SLA；不会用于修改旧 NATS 保存策略。

失败处理建议：可重试错误先有界退避，超过次数进入可查询的失败记录并告警；有序业务停止该分区继续推进，保留后续积压。不能“移到失败队列后继续后面的事件”同时宣称原顺序不变。恢复和跳过要有明确操作记录；取消、死信和过期属于可观察终态，不是成功。具体重试间隔、容量、分区数由技术方案和故障测试确定，不让用户猜参数。现有部分 SDK 定时、DM OTA/在线状态/统计回调会丢弃超过 2 秒的消息；逐项按 F3/F6 决定过期或补算，不能机械搬运过滤后宣称可恢复。

### Q2b：设备可能执行成功、但回执丢失，如何处理

例子：平台发送一次设备操作，设备已执行，网络在回执返回前断开。**已选择 A（D7）：显示“结果待核实”，不因超时自动再执行；优先查询状态。** 查不到可信结果时保持待核实。**D10 补充新操作准入：提示同设备旧结果未知，经操作者明确确认后允许提交新操作；不确认则不提交，查询照常。** 新操作表达新的意图，使用独立身份；旧记录继续待核实。新操作成功或其回执不能据此改写旧结果，旧回执也不能归入新操作；既有权限与有效期检查继续适用。尚未发到设备的持久待办可以在有效期内继续投递；真正无法判断是否已发送的情况不能当成“肯定未执行”。

未作为通用策略选择的 B（后续逐协议另议）：仅对已经证明设备端按同一 commandId 去重、并且有明确有效期的操作自动重发；其他操作仍用 A。成本是逐协议验证设备能力，Kafka 的 producer 幂等无法代替设备端去重。

不推荐 C：所有超时统一重发。比如动作型指令可能执行两次，OTA/脚本也可能重复副作用。当前同步调用默认约 5 秒，部分调用自定超时；这不是目标统一有效期。更关键的是，[异步 Action](../../../../../../backend/things/service/dmsvr/internal/logic/deviceinteract/actionSendLogic.go)经定时任务与[serverEvent](../../../../../../backend/things/service/dmsvr/internal/event/serverEvent/server.go)已存在检查并重发的路径。因此 D7 已确认要收紧现有自动重发行为；实施必须覆盖这条重发路径，而不只是修改同步超时文案。B 仍需逐协议证明与确认，不能把当前重发当安全证明。不能承诺硬件动作严格“恰好一次”。

## 4. Q3：旧消息与返回旧版本

**已选择 H1（D8）：保留原环境及数据，旧消息不自动导入 Kafka；新隔离环境从独立测试数据开始。** 例子：昨天的一条测试设备操作留在旧系统用于查证，但明天启动 Kafka 不会重新执行它。已有业务记录也不删除；“不导入消息”与“清空数据库”是两件事。

| 选项 | 实际做法 | 代价 / 适用场景 |
| --- | --- | --- |
| H1 已选择：保留旧现场，新消息起步 | 后续获准切换时先停止旧入口和调度，记录在途/失败清单，再按现有私有备份方式保存配置、数据及可保存的消息；建立独立新环境和新消费组 | 最小开发切换成本；旧积压不自动成为新系统待办。D8 已接受此业务边界，实际操作仍需后续授权 |
| H2 未选择：先处理旧待办，再切换 | 区分事实和指令，人工核对结果未知/过期项；只让批准的旧工作完成，完成清单核对后切换 | 适合确有必须完成的开发待办；“清空队列”不等于业务完成，旧实现的提前 ACK 可能掩盖未处理项 |
| H3 未选择：选择性导入旧事实 | 仅导入清单批准的事实，保留原ID/来源时间、映射与数量核对；指令、过期触发和历史页面刷新不自动重放 | 迁移成本最高；须先建立去重和副作用隔离，再独立验证导入，不将方案批准当成执行授权 |

保留限制：Core NATS 本身不保存离线历史；当前 JetStream 配置有两分钟 MaxAge 等保留限制，过去已失去的消息不能从不存在的历史中恢复。保留旧容器/卷也不会自动冻结仍在运行的过期策略。需要快照时必须在后续获准操作中明确暂停/备份时点、快照可恢复性和无法恢复部分；本轮没有连接旧 broker 或调整其策略。

返回旧版本的原则：新旧环境和数据分开，先停新入口再恢复旧入口，避免同一真实设备/调度双重执行。不能仅切回旧二进制就声称新产生的数据和已执行动作也回退了；先核对新旧差异，禁止双写补偿或把新命令倒灌旧系统。本轮只制定方案，不实施这些动作。

## 5. Q4：如何验收，不用一个通知测试冒充全迁移

推荐先运行 V1 验证广播语义，再以 **V2 设备上报 → DG → Kafka → DM → 物模型属性持久化** 作为首条真实业务链。已有通知 RPC 基线仅作 V8 回归。D12 新增必须验收 V9，测试方法与参数按第 8.7 节细化；下表全部未运行。

| ID / 覆盖 | 输入与故障场景 | 可观察的通过条件 |
| --- | --- | --- |
| V1 广播 / C5 | 两个真实 Cache 实例预热同一测试对象；通过 SetData 更新；一个实例断开再恢复 | 两个实例最终均读取新状态；恢复后不得继续使用旧值；竞争消费不能漏掉另一个实例。权限缓存另验恢复边界 |
| V2 上行 / C1 | 隔离 MQTT 模拟设备，走现有 DG 入口，DM处理真实 property报文；两个 DM 副本；至少两台测试设备和不同租户 | 不启动 NATS 仍有实际属性写入，租户/设备不串；逐条核对输入ID和存储结果，关键进度可追踪。不以临时测试消费者代替 DM |
| V3 事实可靠性 / C1/C3/C4 | 消费前宕机、数据库提交后但进度提交前宕机、broker断开、重复投递；对事务型事件注入数据库已提交/发送未完成故障 | 按 D6 恢复；同 eventId 不重复已完成的业务结果。数据库提交与待发记录原子保存，提交前失败则两者均无效，提交后发送未完成则恢复补送；消费结果与去重记录需保持原子性或有明确的可恢复执行记录。B 的发送缺口不再是可接受通过条件。错误/失败积压可查询。按 D11 注入“项目记录已删、设备解绑失败”：原操作人仍能找到关联清理未完成的状态，管理员可查看失败原因并处理；仅记录删除成功、仅管理员看见或隐藏失败均不能通过。恢复后按实际清理结果更新，不重复已成功步骤；验证租户和角色隔离。涉及删除的用例只在将来获准的新隔离样例中运行 |
| V4 指令 / C2 | 协议模拟器记录commandId及执行次数，丢回执、迟到回执、重复投递、过期、目标离线和目标实例切换 | 路由到正确设备；按 D7 得到成功/明确失败/待核实/过期；已执行丢回执、发送结果不明时持久显示待核实，进程重启和原延时检查任务均不能自动重发；迟到有效回执只更新同一 commandId 结果。Kafka ACK 不当设备成功，重复回执不重复动作。按 D10 验证旧操作待核实时再次启动/发送停机：明确提示；取消或未确认则不产生新控制；确认后可产生新 commandId，旧记录仍待核实，查询不被此状态拦截。新旧回执分别归属；拒绝无权限请求，不以待核实状态长期阻塞已确认的新操作 |
| V5 路由 / C7/C8/C9 | 两个相同任务副本、两个不同逻辑任务同时匹配；启停/重新启用动态订阅，验证通配和不匹配消息 | 同一任务不重复执行，不同任务都能获得各自事件；重启不重建并丢失消费进度；停用后不再调度；旧任务配置有明确映射或阻止启动的诊断 |
| V6 顺序/坏消息/隔离 | 同一设备连续变化，注入第一个失败、非法payload、缺租户/无效追踪、其他设备正常消息 | 同键后续不越过失败项；不承诺跨分区全局顺序。无效追踪不得静默丢掉租户身份，非法租户拒绝或隔离；失败记录不伪装成功 |
| V7 覆盖与切换 / C1–C10 | 新组合的各独立服务及SDK测试入口；检查配置、直连与topic映射；按 D8/H1 演练；在将来获准的新隔离样例中放入旧消息标记，核对新消费不会执行它 | 新组合无 NATS 运行依赖，清单每项有结果或明确待办；旧环境/数据仍在并可按约定恢复；未纳入的仓外使用者单列，不能宣称已完成 |
| V8 无关回归 / C6及通知 | API实例多节点在线订阅与重连快照；现有通知四场景；API校验与既有测试 | 实时消息发到正确订阅者，断线不补播全部旧刷新；通知昵称及持久化规则保持原样；真实事件验收与通知结果分别记录 |
| V9 积压隔离 / D12 / C1–C3 | 新隔离环境中制造持续遥测积压，再分别提交合法的新控制与项目删除事实；限速或阻塞遥测处理路径，共享依赖保持可用 | 遥测积压仍大于零期间，新控制完成实际模拟设备执行及回执持久化，项目删除关联清理完成并可查询；注入解绑失败后原操作人可见未完成，解除该故障后仍在遥测积压期间可靠补齐。只写入 Kafka、只推进 offset 或待积压清空才处理均不通过；详细条件和证据见第 8.7 节 |

V2 仍须补齐隔离 DG/DM、真实存储与模拟设备配置；现有通知容器不足以证明它能运行。V1通过也不能提前关闭 V2–V7/V9。容量、吞吐与延迟先记录基线再给数值，不凭文档承诺生产性能。

## 6. 技术建议、缺口与推进顺序

以下是 AI 技术提案，不要求用户选择 Kafka 参数：

- 少量按职责划分的物理 topic；原 subject、事件类型、schema版本、租户、对象、时间和追踪进入受控 envelope。设备 key 含租户/产品/设备；不能每台设备或每条 NATS subject 新建 topic。通配在受控路由层解释并做正反例，不把 NATS `*`/`>` 原样当 Kafka topic 匹配规则。
- 竞争消费按逻辑服务/任务分组；广播按需要接收的实例区分，给实例和组明确稳定性/生命周期。设备长连接属于哪个实例需显式路由，不能让任意同组实例拿到下行后因没有连接而丢掉。
- 稳定 eventId、必要的对象版本、关联ID和 expiresAt；保留现有 payload/context 的业务含义，同时校验租户与错误路径。相同 key 只帮助分区顺序，多生产者、跨topic、设备原始采样乱序和消费并发还需另外约束。
- 复用现有 [Kafka foundation](../../../../../../backend/share/foundation/kafka) 的 producer/consumer 安全构造；现有 [topic catalog](../../../../../../backend/share/contracts/kafka/catalog.go)只接受数据平面既定模板，[RawRecord](../../../../../../backend/share/contracts/kafka/raw_record.go)仅允许有限原始数据headers，不能把控制事件硬塞进去或全局放宽校验。实施前为控制事件明确独立契约或有界扩展，并保留现有测试。
- 现有 FastEvent 是进程单例，首次构造的配置和 serverName 决定身份；不能按每个 ServiceContext 各自名称推断消费组。现有 DG 的 `device.down.urMqtt.>` 与 `.status.fix` 订阅重叠，组也不同，必须用真实消息确认分流，不让状态修复误进设备指令路径。
- Consumer关闭自动提交只是起点；业务成功/失败必须传回提交边界，异步 handler、吞掉的错误、多个副作用和下游外部操作要逐项整改。Kafka事务不自动覆盖 PostgreSQL、Redis、WebSocket或设备动作。属性上报还会异步触发 PubApp/Webhook；外部 Webhook 无接收方去重证明时不能声称副作用恰好一次，须单列投递结果、重试和未知状态边界。

### 6.1 已确认决定具体改变哪里

以下为调用族 C1–C10 之上的重点路径映射，不替代后续逐 subject 清单；新数据结构及代码均未实施。

| 决定 / 对应调用 | 现有落点与缺口 | 拟改行为与验证 |
| --- | --- | --- |
| D6 / C3 项目、用户、区域删除 | SaaS 管理 logic 发事实，DM `startup/init.go` 注册后处理关联；[项目删除事务](../../../../../../backend/saas/service/syssvr/internal/logic/projectmanage/projectInfoDeleteLogic.go)提交后才 Publish | 在各自产生事实的业务事务内保存稳定 eventId 与待发记录；发送器可重复投递，DM 在关系处理真正成功后保存结果/去重并提交进度。先做项目删除故障样例，再逐调用展开；V3 必须覆盖事务提交前后与消费提交前后 |
| D6 / C1、C4、C7–C9 其他事实与混合载荷 | DG 上行 → DM deviceMsg；生命周期发布和定时 queue payload 还有配置、指令等含义 | 按 F1–F6 分类，不能把所有旧 subject 都当成可重放事实。设备入口定义持久接受点；缓存/展示恢复当前状态，指令按 D7。重要事实每条列明身份来源、持久边界、handler 返回和失败去向，按 V2/V3/V5/V6 验证 |
| D7 / C2 同步请求与异步 Action | [DM 下行及临时应答订阅](../../../../../../backend/things/service/dmsvr/internal/repo/event/publish/pubDev/nats.go)、[ActionSend](../../../../../../backend/things/service/dmsvr/internal/logic/deviceinteract/actionSendLogic.go)、[延时检查重发](../../../../../../backend/things/service/dmsvr/internal/event/serverEvent/server.go) | 用持久 commandId 关联发送、回执和检查；将原重发检查改为查询/核实，结果不明不得自动重发。DG/SDK 下行重复投递必须识别同一命令；发送前崩溃与发送后回执丢失分别测，V4 包含服务重启和定时重入 |
| D8 / C7、C8 持久任务配置与环境入口 | PubJob 按任务 topic 发消息，QueueTrigger 按 task.Topics 动态注册；部署仍含旧 NATS 配置 | 新环境用独立测试配置、数据和消费组；明确旧 topic 配置的映射或拒绝原因。旧定时待办、历史指令和旧积压均不自动导入；V7 检查旧标记没有进入新任务。先在新样例验证，真实旧现场保持原状 |

D6 的消费去重必须与该消费者的业务结果一致：同一数据库中尽量同事务提交；跨存储/外部操作则分步持久记录并核对结果，不先写“已处理”再执行。去重身份包含逻辑消费者，不能让一个服务的成功误挡住另一服务；超过保留窗口、无稳定来源 ID、外部服务不支持去重等边界须显式列出。事件发送成功后、待发记录标记成功前崩溃允许重投，由消费端处理重复。

D10/D11 增量映射：D10 落到 C2 的 Action/属性控制新请求准入、提示/确认及新旧命令关联，验收 V4，实施 P4；D11 落到 C3/C4 关联处理状态、操作人进度入口和管理员异常处理入口，验收 V3，实施 P3。界面、接口和持久结构由后续技术设计明确；删除对象后仍需能找到该次操作状态，但不在此提前指定新页面或接口。

D12 增量映射：C1 的遥测积压不得阻塞 C2 新控制及回执、C3/C4 的重要事实补处理。P0 明确消息族与资源边界，P1/P3/P4 设计可独立推进的处理路径，P6 用 V9 核验实际业务结果；不能仅凭分开 topic 或消费组判通过。

D7 的状态设计建议为：**持久待发 → 发送结果确认中 → 成功 / 明确失败 / 待核实 / 未发送过期**。并发工作者先持久认领同一 commandId；只有有证据证明尚未发到设备且未过期时，才可继续投递。认领后进程崩溃、网络写入返回不明或已发送后超时都不能简单恢复为待发。设备已经可能执行的命令，即使有效期已过也保留待核实，不能标为“未执行的过期”；迟到回执更新原命令，不生成新动作。查询不支持或没有可靠结果时保持待核实，明确展示需人工核对。这里的状态名、存储表和查询接口是技术草案，D7 的不盲目重发约束已确定。D10 区分“消息处理有序”与“设备结果未知”：旧命令已被持久记录为待核实后，不应长期占用消费分区或设备锁而使已确认的新操作永远不能下发；不能以跳过尚未持久处理的消息来实现此目标。具体并发、串行及确认绑定机制在 P4 验证。

### 6.2 拟实施切片与通过条件

每一行是同一需求的实施步骤，不自动拆成多个 LoopX Todo。当前仅形成计划，后续实现仍受本轮“先不切换实现”的边界约束。

| 步骤 / 依赖 | 拟修改面 | 该步结束时必须拿到的证据 |
| --- | --- | --- |
| P0 调用与规格契约；实施前先完成 | 在本文补齐消息族 / 类型与版本 / 逐 subject 与 API / 启动入口 / 竞争、广播或定向 / 分区键 / F1–F6 / 资源边界 / V1–V9 表；登记 `platform.eventbus` 路径、认领及检查投影 | C1–C10 每项都有承接方式；未接线、SDK 库、仓外未知分开列出；现状与目标不混写。只有所需决定真实确认才生成 confirmed 投影；V9 运行前固定资源、负载、观察窗口与判据，不能事后挑窗口 |
| P1 传输与路由基础；依赖 P0 | Share 受控事件契约、Kafka 封装、FastEvent 回调结果与提交边界；缓存广播恢复；按 D12 区分消费调度、执行池及共享连接池的资源边界 | 保留现有数据平面 catalog 测试；通配正反例、竞争组/广播、错误返回与 offset 验证；真实 Cache 两实例 V1；为 V9 保留控制/回执与重要事实独立推进的资源。P1 通过不等于重要事实或设备命令已可上线 |
| P2 首条真实上行；依赖 P1 | DG 上行发布、DM 真实属性处理及持久成功边界、隔离 DG/DM/存储/模拟设备配置 | V2 端到端和 V3 中设备事实的接受后重启/重复/断连；V6 设备隔离与顺序；不用临时消费者代替 DM，不扩展 shadow 业务 |
| P3 事务事实补处理；依赖 P1 | SaaS/Things 生命周期事务与待发记录、可靠发送、消费去重/结果记录；D11 的操作人进度与管理员处理入口 | 项目删除为首个样例；V3 原子保存和恢复、重复不重复业务结果；再展开到 C3/C4 全清单；V9 补处理路径不等待遥测积压清空。多副作用失败可查询、可继续，不能提前 ACK；操作人可见未完成，管理员处理与恢复结果可追踪，不能把主记录删除当整项清理完成 |
| P4 命令与回执；依赖 P1/P2 的路由与真实入口 | DM pubDev、DG/SDK 下行、ActionSend 和 serverEvent 延时检查、持久命令状态；D10 的新操作提示/确认与独立身份 | V4 全部场景并参与 V9，命令与回执均不被遥测处理长期占满资源；尤其丢回执后原延时任务、重启、消费重投均不再次执行；使用协议模拟器，不控制真实设备；设备去重例外仍单列待证；明确确认后允许新操作、未确认不发送，新旧结果不串，查询照常 |
| P5 剩余调用与配置；依赖对应 F1/F3 能力 | 生命周期配置、WebSocket、PubJob、QueueTrigger、周期任务与旧接口处置 | C4–C10 全部有结果；V5 动态路由/启停，V8 展示及通知回归；独立服务与组合入口均实际启动；按 D8 只构造新数据 |
| P6 整体验收与收敛；依赖 P2–P5 | 新隔离组合、启动说明、完整当前规格、Patch 吸收关系、原任务证据 | V1–V9 同一候选通过并独立复审，V9 必须保留积压、实际控制/解绑结果及资源条件的时间对应证据；新组合不启动 NATS，旧现场保留。旧环境快照/恢复及实际切换若未获准执行，单列未验，不用文档或新样例的通过替代 |

R8 已推进 **P0 的文档设计部分**（第 9 节）；实施认领/检查投影和运行参数仍未完成，不能据此宣称 P0 全部结束。下一实施批次建议完成 P0 的实施入口并做 P1，再做 P2/P3，待可靠状态与路由成立后进入 P4/P5；P1–P6 尚未启动。P0/P1 的范围与验收不要求用户决定 topic 数、重试间隔或分区数，由 AI 依据源码、机制和隔离测试提出。若剩余调用暴露新的业务取舍，再提交对应例子；不重新询问 D6–D8/D10–D12。

### 6.3 当前完成标准与未覆盖项

本轮完成标准：RAW-005/RAW-007/RAW-009 → D6/D7/D8/D10/D11/D12 → 受影响调用 → V3/V4/V7/V9 → 实施切片可追溯，入口和索引一致，最终文档候选通过检查与独立复审。当前目标规则仍在本 Patch；主规格 observed-v0 继续如实描述源码，实施与验收完成后才收敛为新的当前规则。

剩余工作包括逐 subject 全量表、控制事件契约、可靠状态存储设计、API/界面如何实现已确认的提示、明确确认、待核实与关联清理进度、各协议可查询能力，积压隔离的资源设计与测试参数，以及 V1–V9 的实际结果；当前均不声称完成。长期模块负责人未分配，团队正式采用和维护收益待真实交付验证。技术参数由后续方案验证，仓外 SDK 使用者不得被隐含纳入已迁移范围。

团队维护成本记录在本轮 Goal 回执：文件数、实际人决策数、复审发现与返工；R3 三项政策与 R5 两项答复保留，R7 新增 Q7 一项真实答复，不新增交接包或平行业务正文。未测量工时写 unknown，不能用文档完整度推导收益。

## 7. 可选分析 Skill 试跑：R4 发问，R5 写回

### 7.1 输入、责任与方法

- 目的：检验“补处理”和“待核实”是否足以指导异常行为及验收；本轮聚焦 D6/D7 的使用者可见结果，不重新确认 D6–D8，也不宣称审完 Kafka 的所有设计分支。
- 输入：本 Patch R3，源码与候选 `4affea4caec005426214059fef5da3c27fc2b957`，当前 observed-v0，RAW-001–006，以及[可选分析接入约定](../../../../../runbook/team-development.md#optional-analysis-skills)。
- 方法：读取本机 `grill-with-docs` 及其 `grilling`、`domain-modeling` 依赖，按其追问、事实调查、术语辨析和反例方法执行；不是只声明 Skill 已加载。源码事实由独立 AI 调查，主 Agent 组织本轮真实问答并写回本节；按用户限定，将术语/取舍放在这里，不创建 `CONTEXT.md` 或独立 ADR。
- 确认责任：本轮用户确认新增业务取舍；主 Agent 负责调查、提案和唯一正文写回，独立评审核对来源与候选。长期模块负责人仍未分配，不把 AI 调查或本次用户答复当作永久责任分配。
- 边界：只读源码并维护文档/Skill；不启动服务、不读取旧环境数据、不控制设备、不推送。分析输出不改变当前运行行为。

### 7.2 事实与术语澄清

| ID | 已查事实 / 术语 | 对方案的影响 |
| --- | --- | --- |
| G1 | [ActionSend](../../../../../../backend/things/service/dmsvr/internal/logic/deviceinteract/actionSendLogic.go) 和属性控制入口检查权限、有效期、在线状态，每次请求生成新 MsgToken；[CheckControlAllowed](../../../../../../backend/things/service/dmsvr/internal/logic/deviceinteract/checkControlAllowed.go)实际检查设备有效期，已查路径没有用旧待核实命令拦截新操作 | D7 禁止重发旧命令，不能据此推出“禁止同设备所有新操作”。R4 据此提出 Q5，R5 由 D10 确认提示后允许人确认的新操作；不扩大为已查所有 OTA/脚本/协议路径 |
| G2 | [ActionRead](../../../../../../backend/things/service/dmsvr/internal/logic/deviceinteract/actionReadLogic.go)查已有回执，未找到返回 NotFind；[消息缓存](../../../../../../backend/things/service/dmsvr/internal/repo/cache/msgRepo.go) TTL 为十分钟 | “查不到回执”不等于“设备未执行”；不能直接把现有 ActionRead 称为持久核实台账或设备执行历史查询 |
| G3 | [属性查询](../../../../../../backend/things/service/dmsvr/internal/logic/deviceinteract/propertyGetReportSendLogic.go)可请求当前属性并等待回包，但可能超时；不是所有协议都已验证支持 | “查询当前状态”与“证明某次动作执行过”分开。例如当前温度或开关值不能证明某次复位、加量动作是否发生；无法核实仍按 D7 保留待核实 |
| G4 | [项目删除](../../../../../../backend/saas/service/syssvr/internal/logic/projectmanage/projectInfoDeleteLogic.go)事务提交后才发事件，发布失败会返回错误；[DM 项目删除消费者](../../../../../../backend/things/service/dmsvr/internal/startup/init.go)单设备解绑失败仅记日志，已查路径未见持久补处理待办/人工处理入口 | “主记录已删除”“关联清理未完成”“整项业务完成”是不同事实。API 返回错误不表示删除回滚；D6 必须整改消费错误与可靠记录，长期未完成向谁显示由 Q6 形成 D11：操作人可见、管理员处理 |

G1–G4 为上述固定版本的源码证据，不是运行结果。技术可自行细化的部分包括持久身份、状态存储、错误返回、迟到回执归并、缓存替代和查询能力盘点；它们必须遵循 Q5/Q6 已确认的 D10/D11，不能替代业务选择。术语在本 Patch 维护：**待核实**表示缺少可信执行结果；**新操作**有独立身份及新的用户意图；**重投**是同一事件的传输重试，不能自动变成新的设备动作。

### 7.3 第一轮问题及实际答复

R4 基于 G1–G4 提出两个有前置依据的问题，当时均为 pending。2026-10-17 用户在同一轮实际选择如下，R5 写回；保留候选对比以说明排除范围，不再把推荐当作待答或重新询问。

| ID / 责任 / 状态 | 业务例子、候选及真实回答 | 决定、影响与依赖 |
| --- | --- | --- |
| Q5 / 本轮需求用户 / confirmed，自 R5 | 启动操作没回执，此时再次启动或想停机。A：提示旧结果未知，由人确认后提交新操作；旧记录待核实、查询照常。B：暂停全部新控制、只允许查询。用户选择“提示后由人确认，允许发新操作（推荐）” | D10；排除一律暂停所有新控制。补充 P4 的提示/确认及新旧身份分离，V4 覆盖确认、取消、权限校验及回执隔离；不改变 D7，不授权自动重发 |
| Q6 / 本轮需求用户 / confirmed，自 R5 | 项目记录已删，但设备解绑长期失败。A：操作人看到关联清理未完成，管理员处理。B：仅管理员异常待办可见。用户选择“操作人看到未完成，管理员处理（推荐）” | D11；排除仅管理员可见。P3 增加操作人进度和管理员处理入口；V3 验证未完成可见、失败处理、恢复与角色/租户隔离；继续 D6 补处理，不把失败当成功 |

真实答复追加至 RAW-007，在决定表形成 D10/D11，并同步第 3 节、V3/V4 与 P3/P4。Q5/Q6 已收敛；没有新事实要求另开问题轮，也不重问 D6–D8。本轮完成的是这一组业务取舍的分析和写回，不表示全部 Kafka 设计已确认、实现已授权或运行通过。

### 7.4 本轮维护记录

R4 在原 Patch 增加一节，记录四项事实/术语发现，提出两个真实业务问题；R5 收到两个真实回答，新增 D10/D11 并修订受影响验收与切片。一次提问轮、两项新增决定，沿用 D6–D8，零次重复确认。没有新建模块规格、访谈报告、CONTEXT 或 ADR。

维护量：R4 修改 10 个既有文件；R5 写回修改 7 个既有文件（本 Patch、raw、模块入口、业务地图、流程入口、总纲及来源索引），新增公开文档数均为 0。两阶段文件数有重叠，不相加为独立文件总数。完整检查与独立复审回执绑定各自最终候选，留在同一 Goal；R4 的 pending 回执作为历史保留，不覆盖。耗时未测量，记 unknown。

此次收益证据是明确发现并收敛了“新操作准入”和“未完成可见性”两处行为缺口，下一位接手者可从决定追到验收。代价是两个业务问题及七处来源/正文/导航写回；源码核查和文档检查不替代 Kafka/设备运行验证，也不足以证明长期效率或团队正式采用。未来应从实际任务继续评估导航维护量，不为本例增加全库门禁。

## 8. ThingsBoard 通道与消息模型：R6 源码分析

### 8.1 输入与结论边界

按 RAW-008 采用 `grill-with-docs` 的事实调查、术语辨析与反例方法；两项独立只读调查分别核对通道拓扑和消息/可靠性，主 Agent 对照 Ming IoT。术语和取舍留在本节，不另建 CONTEXT、ADR 或平行研究正文。

ThingsBoard 固定源码为 `0439df09f31b0e8101b9f0a6be556c087d86818d`，提交日期 2026-10-02；[pom.xml](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/pom.xml#L13) 标记 `4.4.0-SNAPSHOT`。这是仓外只读参考，不宣称正式版本或最新发布，也不迁入本仓。Ming IoT 对照基线为 `addcf8b18425a47dfd3a8c322936c1363a712a62` / R5。只查源码与官方说明，未运行 ThingsBoard、Kafka 或设备；[官方队列文档](https://thingsboard.io/docs/reference/architecture/queue/)是辅助说明，下面的具体结论以固定源码为准。

**方向合理，但应提炼成“按处理职责、路由及失败语义划分通道，统一基础消息契约”，不能把四类业务名称当作 ThingsBoard 的四个实际通道，也不能直接固定为四个 Kafka topic。** 原 subject 逐项盘点仍必要，用来防止遗漏；把旧 subject 命名逐个搬成新 topic 没有同等价值。

### 8.2 源码实际做了什么

| 事实 | 证据与含义 |
| --- | --- |
| TB 主体按接收服务和用途划分，而非固定四类 | [队列配置](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/application/src/main/resources/thingsboard.yml#L2330)包含 transport API 请求/响应、core、rule engine、各自 notifications，另有 OTA、usage、CF、integration 等；仅 integration 子系统出现 uplink/downlink 命名，不能外推全系统 |
| telemetry 与部分业务事件进入同一 rule-engine 路径 | [DefaultTransportService:532–549、1117–1142](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/common/transport/transport-api/src/main/java/org/thingsboard/server/common/transport/service/DefaultTransportService.java#L532)把 telemetry 构造成 TbMsg；[DefaultDeviceStateService:847–873](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/application/src/main/java/org/thingsboard/server/service/state/DefaultDeviceStateService.java#L847)也把连接/活动事件送入规则引擎。不是每个业务事件类型都有独立 topic |
| RPC 跨服务路由，设备下行复用 transport notifications | [Core RPC:173–181](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/application/src/main/java/org/thingsboard/server/service/rpc/DefaultTbCoreDeviceRpcService.java#L173)先进入规则引擎；[Rule Engine RPC:154–167](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/application/src/main/java/org/thingsboard/server/service/rpc/DefaultTbRuleEngineRpcService.java#L154)路由到设备所属 core；[CoreToTransport:31–54](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/application/src/main/java/org/thingsboard/server/service/transport/DefaultTbCoreToTransportService.java#L31)发往指定 transport 节点。不是单一“下行队列” |
| notifications 是内部低延迟、实例定向消息族，不能等同业务用户通知 | [queue.proto:1879–1901、1952–1967](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/common/proto/src/main/proto/queue.proto#L1879)包含 RPC、缓存失效、生命周期及用户通知触发等；transport 的同类消息还包含设备控制。它不只承载节点控制，也不等于邮件/短信/站内消息 |
| 竞争、定向和广播有不同实现 | [Core factory:243–264](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/common/queue/src/main/java/org/thingsboard/server/queue/provider/KafkaTbCoreQueueFactory.java#L243)的工作队列与实例 notification consumer 不同；[TopicService:133–134](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/common/queue/src/main/java/org/thingsboard/server/queue/discovery/TopicService.java#L133)构造带 serviceId 的名称；[ClusterService:193–216](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/application/src/main/java/org/thingsboard/server/service/queue/DefaultTbClusterService.java#L193)枚举实例发送广播，不是同一个竞争组让每个实例都得到一份 |
| TB 的逻辑分区不总等于 Kafka 原生分区 | [HashPartitionService:388–397](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/common/queue/src/main/java/org/thingsboard/server/queue/discovery/HashPartitionService.java#L388)按实体散列；[TopicPartitionInfo:28–45](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/common/message/src/main/java/org/thingsboard/server/common/msg/queue/TopicPartitionInfo.java#L28)默认可把逻辑分区映射到 topic 后缀。Ming IoT 不直接照搬这种拓扑，也不据此承诺跨通道全局顺序 |

### 8.3 “统一消息格式”需要统一到哪层

源码存在三层，而非全系统一个业务结构：

1. [TbProtoQueueMsg:12–40](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/common/queue/src/main/java/org/thingsboard/server/queue/common/TbProtoQueueMsg.java#L12)统一 `key + typed value + headers` 的队列接口；value 自身编码为 protobuf。
2. [queue.proto:1861–1969](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/common/proto/src/main/proto/queue.proto#L1861)分别定义 ToCoreMsg、ToCoreNotificationMsg、ToRuleEngineMsg、ToRuleEngineNotificationMsg、ToTransportMsg，各消费工厂选择对应 decoder；它们不是同一业务载荷。
3. [TbMsgProto:27–61](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/common/message/src/main/proto/tbmsg.proto#L27)统一的是规则引擎消息，含 type、originator、metadata、data 以及 rule-chain 等上下文；不应把这些规则引擎专属字段强加给本项目每条控制或通知。

对 Ming IoT 的技术建议：统一业务身份、schema 版本、事件类型、租户/对象身份、来源与发生时间、关联/因果标识和追踪的含义；各消息族定义自己的强类型 payload、必填字段和校验。命令另外校验 commandId、有效期、目标路由与确认上下文；eventId、commandId、correlationId 和 Kafka 分区 key 不能互相充当。具体字段名、编码格式及版本演进是后续契约设计，未因本次分析批准。

[现有 RawRecord](../../../../../../backend/share/contracts/kafka/raw_record.go)只接受五类原始数据 headers，[catalog](../../../../../../backend/share/contracts/kafka/catalog.go)限制数据平面模板。建议保留原始报文的数据契约，规范化业务消息再采用公共信封；不能为“统一”直接放宽冻结校验或把控制消息塞进原始遥测格式。

### 8.4 适合 Ming IoT 的分组建议（proposed）

这里的“通道”先指一组相同处理责任与投递规则的逻辑消息，不等于一个物理 topic；四类业务加内部协调是本项目的候选分类，不是 TB 原版拓扑。

| 逻辑消息族 | 对应本 Patch | 关键边界 |
| --- | --- | --- |
| 设备上报事实 | C1、部分 C4；F1/F2 | 高吞吐、对象保序、持久接受后可补处理；区分原始报文与规范化业务事实 |
| 设备命令与回执 | C2，以及 C8/C9 中的动作载荷；F3 | 命令定向到持有连接的实例，回执关联原命令；两个方向可有各自物理路由。不能把命令回执塞在大批遥测之后而忽略响应时间；具体隔离验收见 Q7 |
| 领域事实 | C3/C4 及 C7–C9 中的事实载荷；F1/F6 | 例如项目已删除；每个独立逻辑订阅者各处理一份，同一订阅者的副本竞争。按 D6 保证待发和消费结果可靠，按 D11 呈现未完成 |
| 用户通知业务 | 现有通知模块及 C6 的结果展示关联 | 持久消息、邮件/短信发送结果与 WebSocket 页面刷新分开。通知基线目前是 RPC→持久化；此次只分析分类，不把原 RPC 链路自动改成 Kafka 异步，也不新增通知产品能力 |
| 内部协调/广播 | C5、C4 配置刷新、C6 节点展示分发及相应周期协作 | 缓存失效必须覆盖所有相关活跃实例，重连恢复当前权威状态；节点定向、广播和持久业务通知不可混为一类重试策略 |

反例：项目删除事实需要可靠补处理，权限缓存失效需要各实例恢复一致，页面刷新可读取当前快照；三者不能仅因都是“通知”而共用一个竞争组和统一失败策略。业务事件与遥测也不应因都叫“event”就承诺相同保留期。物理隔离还取决于 worker、连接池和数据库等共享资源，只分 topic 不足以证明互不阻塞。

### 8.5 不能从 ThingsBoard 直接继承的承诺

- **D6**：producer 成功只是 Kafka 写入回调。[TbRpcService:78–126](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/application/src/main/java/org/thingsboard/server/service/rpc/TbRpcService.java#L78)这个具体路径在持久化成功回调后发布状态事件；本轮未见该路径业务与待发记录原子保存，不能由队列封装推导出 outbox 已满足，也不能据此断言 TB 全库没有 outbox。
- **D7/D10**：[DeviceActorMessageProcessor:237–275、529–551、893–921](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/application/src/main/java/org/thingsboard/server/actors/device/DeviceActorMessageProcessor.java#L237)已有持久 RPC 入队去重，但仍有超时重试和状态处理路径；设备未知结果默认不重发、新操作提示后确认，继续按本项目规则实现，不能把 TB 去重等同物理动作只执行一次。
- **D11**：队列统计和通用 notification 不能代替“原操作人可见未完成、管理员处理”的业务状态入口。
- [Rule Engine processing strategy](https://github.com/thingsboard/thingsboard/blob/0439df09f31b0e8101b9f0a6be556c087d86818d/application/src/main/java/org/thingsboard/server/service/queue/processing/TbRuleEngineProcessingStrategyFactory.java#L73)存在失败跳过及不同重试策略；采用成熟框架不表示自动满足本项目“失败不冒充成功、重要事实不丢”的验收。

### 8.6 决策树、实际问题与推进方式

本轮决策树：**事实核对（已完成）→ 逻辑分类与契约分层（技术建议）→ 跨消息族积压隔离承诺 Q7（R7 已确认为 D12）→ 物理 topic / 分区 / 消费组 / 资源与故障用例（按决定细化）**。D1–D11 不变，不重问已有可靠性和旧数据决定。原始需求 RAW-008 只授权分析，不把“参考 TB”当作同意其所有重试、分区或业务行为。

**Q7 / 本轮需求用户 / confirmed / R7**：问题是是否把“上报积压时，控制和重要业务补处理仍能独立推进”列为必须验收。用户在 RAW-009 实际回答“列为必须验收（推荐）”，形成 D12；原建议 B“先分类、隔离以后验证”与暂缓选择均未采纳。例子是遥测补处理期间，操作者确认的新停机操作、项目删除后的设备解绑，不必等遥测积压全部消化。

Q7 的待答状态已解除；不重复询问分区数或延迟参数。R7 把 D12 接入 V9 和 P0/P1/P3/P4/P6，具体分组、物理拓扑及编码仍为技术建议。此次真实选择只确认该业务验收目标，不把第 8 节所有建议或整个草稿算作批准。

### 8.7 D12 / V9：必须验收的设计与证据

下面把 D12 转成可复验的设计判据，当前均未执行；具体测试参数在 P0 固定，测试实现仍待后续授权。

1. **固定现场。** 只使用新隔离环境、模拟设备和新生成的测试数据，不导入旧消息。记录同一候选、topic/组/分区键、进程与 worker 配额、连接池边界、共享 broker/数据库健康状态、遥测速率/大小/积压量、控制与事实样本数。正式运行前固定有界观察窗口 W；W 从无积压基线、所选命令有效期和测试规模推导并随方案评审，不能看结果再调。W 未确定时标未就绪，不能把任意长时间“最终完成”记 PASS，也不据此承诺生产毫秒 SLA。
2. **持续积压下验证两条实际链。** 先以同一配置取得无积压基线，再通过持续新遥测和对遥测处理器限速/阻塞制造积压；broker、数据库及目标设备保持可用。控制样本走真实 DM→Kafka→DG/SDK→协议模拟器→回执路径，最终有模拟设备执行记录与原 commandId 的持久结果；领域样本走真实项目删除事务→待发→设备解绑→处理状态路径。二者都必须在 W 内且遥测积压尚未清空时完成；只收到请求、写入 Kafka、更新 offset 或临时测试消费者成功不足以通过。记录每条样本起止时间、事件/命令身份、积压曲线与响应分布。
3. **核对故障边界。** 在遥测仍积压时额外注入解绑失败，按 D11 让原操作人看到未完成、管理员看到原因；解除解绑故障后，在预先固定的恢复观察窗口内完成可靠补处理，且遥测仍积压。按 D7/D10 叠加丢回执及已确认的新操作场景时，旧操作保持待核实且不自动重发，新操作仍能推进；没有可靠回执时不伪造旧操作成功。权限、有效期、事件去重及同键顺序仍照常检查，隔离不授予绕过它们的例外。
4. **验证恢复并防止假通过。** 解除遥测限速/阻塞后，核对已持久接受的遥测能继续补处理，无越序跳过失败项或人为丢弃以清空积压。若控制/解绑只能在遥测排空后完成、只证明不同 topic 存在、超出已固定窗口或缺少实际业务结果，则 V9 不通过。与遥测负载无关的共享依赖整体故障单列为边界，继续按 V3 验证恢复，不用该情形假称 V9 通过；此类运行保留失败/无效现场，恢复后重新测试。若是本轮遥测负载耗尽共享执行池、连接池或造成存储争用，使控制/补处理无法在窗口内推进，应判 V9 失败，不能归为“依赖不健康”而豁免；原因不明时保留未通过，查明后再判。

D12 不要求所有消息跨通道全局有序，也不承诺任意超载下可用；保留已确认的每类顺序和失败语义。资源设计需覆盖 consumer 调度、业务 worker、连接池、设备连接处理与共享存储争用；只拆 topic 不足以交付 D12。此次维护量为一项真实决定、一个验收场景和四个既有文件同步，没有新增业务文档或运行能力。

优先投入通道语义和新契约，再做旧调用覆盖映射与适配；保留映射表作为无遗漏证据，不建设一个靠所有旧 subject 名称维持的新通用总线。没有运行、切换、推送或操作旧环境数据；结论、问题与实际答复留在同一 Patch。

## 技术依据与适用性

2026-10-17 核对官方资料，仅用于解释机制；业务政策仍以上面的真实决定为准：

- [Apache Kafka Design：交付语义](https://kafka.apache.org/41/design/design/)：处理后提交可能重投；Kafka内部事务不自动包含外部存储或设备动作。
- [NATS Core概念](https://docs.nats.io/learn/core-nats/)与[JetStream概念](https://docs.nats.io/concepts/jetstream)：Core无离线历史，JetStream的实际恢复能力受流/消费者配置约束。

未升级broker/SDK版本，未借参考资料改变当前业务权威。源码现状在主规格，R7 在同一 Patch 保存已确认政策、实际问答、固定源码分析、剩余提案与验收。


## 9. P0 设计细化：R8 可评审候选

### 9.1 适用边界与源码依据

依据 RAW-010 的“下一步”，继续同一 Goal 和 Patch，完成设计文档；D1–D12 不变。本节均为 **proposed** 技术选择，不把用户的一句话视为批准每个 Topic、字段或实现。当前 `spec.md@observed-v0` 不改写为目标行为；没有启动 Kafka、执行迁移或推送。

本轮源码基线为 `b0a3085270a16ddfeefff3c1e021cbc528acd84e`。以下以语义不同的实际入口逐项列出，重复调用点归到相同规则；既查调用，也查构造与启动。它是仓内静态覆盖，**不是运行环境任务表、部署实例或仓外 SDK 客户清单**。

| 依据 | 本轮核对结果与设计影响 |
| --- | --- |
| [FastEvent](../../../../../../backend/share/eventBus/fastEvent.go) | 进程单例；队列与广播 handler 启动异步任务后先返回 nil。目标必须让处理结果到达提交边界；仅加 Kafka 分支会保留提前成功缺口。组合进程不能再以首次构造的 serverName 推导所有逻辑消费者 |
| [DM pubDev](../../../../../../backend/things/service/dmsvr/internal/repo/event/publish/pubDev/nats.go)、[DG subInner](../../../../../../backend/things/service/dgsvr/internal/repo/event/subscribe/subInner/nats.go) | DM 文件名虽为 nats.go，实际通过 FastEvent 发布/临时订阅；DG 下行仍是直接 NATS。请求回执与普通上行同 subject，需在进入遥测阻塞段前识别回执 |
| [DG pubInner](../../../../../../backend/things/service/dgsvr/internal/repo/event/publish/pubInner/natsJs.go)、[SDK Core](../../../../../../backend/things/sdk/protocol/coreProtocol.go) | DG 多个 DevPub 方法最终走直接 NATS；SDK 通过 FastEvent 发布，并按协议注册下行、配置和定时处理。SDK 是库入口，不能代替外部部署证明 |
| [DM startup](../../../../../../backend/things/service/dmsvr/internal/startup/init.go)、[DG startup](../../../../../../backend/things/service/dgsvr/internal/startup/init.go) | DM 注册八类上行、领域清理和周期处理后 Start；DG 同时有宽下行选择器和精确 status.fix。目标选择器必须互斥，修复事件不可误发为设备指令 |
| [PubJob](../../../../../../backend/saas/service/timed/internal/repo/event/publish/pubJob/job.go)、[QueueTrigger](../../../../../../backend/saas/service/timed/timedschedulersvr/internal/timer/queueTrigger.go) | 任务 payload、GroupSubType、Topics 与原 subject 都影响行为；按任务身份区分独立订阅者，不能用一个服务级竞争组吞掉其他任务应收到的事件 |
| [SaaS topics](../../../../../../backend/saas/share/topics/nats.go)、[Things device](../../../../../../backend/things/share/topics/device.go) / [notify](../../../../../../backend/things/share/topics/notify.go)、[旧公共 topic](../../../../../../backend/share/eventBus/topic.go) | 多套常量并存；`server.core.project.info.delete` 有实际清理链，`server.core.project.delete` 不能仅凭相似名字视为同一已接线能力；常量存在不证明发布/消费或业务可用 |

### 9.2 消息族与逐入口承接

表中 U 是普通上报与设备事件/状态的同一有序通道，R 是控制回执，C 是下行，D 是领域事实，I 是内部协调，W 是页面实时分发。它们是目标通道代号，物理名称见下一节。`device.up.*` 必须结合受控 payload 中的方法/类型判别：例如 thing 包内既有属性上报也有 action 回执，不能只看 subject 前缀。表内 F/V 沿用第 3/5 节，不另建规则。

| 项 / 旧 subject 或 API | 当前生产 → 实际接收入口 / 状态 | 目标通道、处理与 F / V |
| --- | --- | --- |
| M01 `device.up.thing.<product>.<device>` | DG DevPubThing/DevPubMsg、SDK DevPubMsg → DM Thing | 属性 Report/ReportInfo、设备 EventPost、设备主动 Action 请求→U；ControlReply、GetReportReply、ActionReply→R，校验可信协议 token 关联。pack_report 混合包按下文 M01 补充规则。普通事实 F2，控制回执及附属属性处理按第 9.4 节；V2/V4/V6/V9 |
| M02 `device.up.{ota,ext,config,log,shadow,gateway}.<product>.<device>` | DG 对应 DevPub 方法/通用发布、SDK → DM 对应 handler；shadow handler 的 TODO 不补成新功能 | 日志及设备发起的状态/协议请求→U，可识别控制回执→R；需回复的 ext/OTA/config/gateway 协议交互走 C，不能误当可任意重放动作。F2/F3；V2/V4/V6/V7 |
| M03 `device.up.status.{connected,disconnected}` | DG PubConn、SDK 连接事件 → DM Connected/Disconnected | U，设备会话/对象身份及发生时间；更新事实可重放但防旧会话覆盖新会话。F2；V2/V3/V6 |
| M04 `device.down.<protocol>.<handle>.<product>.<device>`；PublishToDev/ReqToDeviceSync | DM pubDev → DG `urMqtt` SubToDevMsg 或 SDK RegisterDeviceMsgDownHandler | C；按注册协议和连接所有权投递。同步等待改为等持久 commandId 的状态，超时不取消账本；R 独立收回执。F3；V4/V9 |
| M05 `device.down.<protocol>.status.fix` | DM Thing 路径 → DG OnlineFix；与旧宽订阅重叠 | I 的 `connection.reconcile`，精确排除于 M04；幂等刷新当前连接状态，不当设备动作。F4；V1/V6/V7 |
| M06 `server.core.{project.info.delete,user.delete,area.info.delete}` | syssvr 对应管理逻辑（含用户删除触发项目删除）→ DM InitEventBus | D，按来源事务生成事件，DM 关联清理逻辑消费者持久结果；D11 状态独立于被删记录。F1；V3/V9 |
| M07 `server.core.{project.info.create,user.create,user.update,tenant.create,ops.workOrder.finish}` | syssvr 管理、注册/登录、deptSync 等有发布；本轮未定位全部实际下游 | D；保留事实发布的语义及来源，已配置订阅者才进入验收清单，不虚构消费者。F1；V3/V7 |
| M08 `server.things.dm.device.{info.create,info.update,info.delete,info.bind,info.unbind,transfer}` | DM 设备管理、批量更新、移动/转移等有发布；下游按具体订阅配置核对 | D，对象键/版本；源事务与 outbox 一起保存，每个逻辑订阅者去重。F1；V3/V6/V7 |
| M09 `server.things.dm.product.info.{create,update,delete}`、`server.things.dm.user.device.share.granted` | 产品管理；[分享授权事件](../../../../../../backend/things/service/dmsvr/internal/logic/userdevice/deviceShareGrantedEvent.go) → 已注册订阅者 | D；不凭 topic 名推导已有站内通知消费。分享授权 payload 的身份、租户和幂等含义保留。F1；V3/V7/V8 |
| M10 `server.things.dm.product.custom.update`、`server.things.dm.protocol.<code>.update` | 产品脚本更新→DG 清本地脚本缓存；协议创建/更新→SDK Cloud 配置回调 | I，相关实例各自收到并从权威存储恢复当前版本；不是任一副本刷新即可。若需审计事实，由 D 记录业务变化，I 仅驱动刷新。F4；V1/V6 |
| M11 `server.cache.sync.<keyType>` | Share Cache.SetData → 各使用者 Cache.Subscribe | I，按缓存类/对象失效；API、DM、DG、SDK 等持有本地缓存的活跃实例分别应用。F4；V1 |
| M12 `server.core.api.user.publish.<scope>` / UserSubscribe.Publish | DM 等聚合页面更新→SaaS websocket StartWsDp；Share 另有同名实现 | W，实例广播后按当前用户权限/订阅过滤；断线后读快照，不能作为持久业务通知已送达。F5；V8 |
| M13 `application.device...report.thing.{property,event,action}`、`application.v2.device...report.thing.property`、`application.device...status.*` | DM PubApp 有属性/事件/action 发布；普通 Connected/Disconnected 的 PubApp 调用注释，在线修复路径有断开发布；动态订阅者需查新配置 | 派生属性/事件/状态→U 的 application 类型；action 请求与结果投影→I 的 application.action.observed，保留 ReqType、Dir、MsgToken 与旧 subject 供 M16 筛选；具体形状见下文。投影不是原始命令或可信设备回执，不发新动作、不再次完成命令。F2/F6；V5/V6/V8 |
| M14 `server.things.dm.{ota.job.delay.run,ota.device.upgrade.push}`、`server.things.dm.action.check.delay` | 定时任务→DM OTA handler、直接 NATS server.Subscribe→Action 检查 | I 的受控任务触发；动作阶段进入 C，检查阶段只核实账本，禁止原自动重发。以 taskId/计划触发身份去重，不能把每次轮询都当新动作。F6/F3；V4/V5 |
| M15 `server.things.dm.device.{onlineStatus.change,static.2Hour,static.1Day,static.halfHour,static.oneMinute}`、`server.things.dg.online.{timer,timer2}`、`server.core.sync.{halfHour,day}`、`server.core.timedjob.clean` | 新任务配置→DM/DG/syssvr/timedjob；各 startup 注册；syssvr/timedjob 注册不证明独立进程 Start 已执行 | I，按逻辑任务竞争处理；名称 `static.2Hour` 与常量 OneHour 不互相纠正成新业务周期。统计/刷新按当前状态重算；清理等副作用单列可重复边界。F6；V5/V7 |
| M16 ProcessTask.Queue→PubJob.Publish；QueueTrigger 的 `task.Topics` / GroupSubType | 配置驱动发布、每任务 SubscribeWithConsumer→TaskSend→queue/SQL/script | 发布必须经注册类型映射到 U/C/D/I 等；不创建任意 Kafka topic。消费按持久 taskId+订阅 revision 独立投影，任务副本竞争；输入事件与待调度记录原子保存。SQL/script 效果不明不自动再执行。F6；V5 |
| M17 `server.things.<ServerName>.protocol.timer` | SDK RegisterTimerHandler 创建定时配置且广播订阅；当前跳过两秒前触发 | I 的协议实例定时刷新；保留每实例刷新语义，周期来自注册配置；过期刷新合并为一次当前状态重建，不泛化为业务命令超时策略。F4/F6；V1/V5/V7 |
| M18 常量-only、注释与旧封装 | CoreProjectDelete、UD/PAli 等常量；DM 旧 subDev 启动注释；Share/SaaS topic 与 websocket 副本 | 标为未接线/遗留，目标新配置发现引用即显式报错并定位配置。实际共享封装和初始化的直连仍须移除目标组合的 NATS 依赖，不以“没有 handler”豁免构造器。V7 |
| M19 通知 RPC、进程内 AsyncEventBus、设备 MQTT、HTTP/gRPC、Redis/Asynq | 现有通知持久化与外部协议/调度机制 | 保持既有职责；只替换其中跨服务 Eventbus 段，不新建 Kafka 邮件/短信产品链。V8；不把这些名字列为遗漏 NATS |

**M01 混合包与 M13 action 的补充规则：** [ThingLogic](../../../../../../backend/things/service/dmsvr/internal/event/deviceMsgEvent/thing.go) 的 HandlePackReport 同时处理本设备/子设备的 Properties、Events 和在线状态；HandleProperty 则有 ControlReply，不能把所有 property 当普通遥测。首版对 pack_report 保存完整接受记录，以父 eventId+目标设备+子项位置/类型生成稳定子项身份，并持久保存待处理/待发清单。拆分不是多个互不关联的 Publish：部分发送失败可继续同一清单，重投不产生新子项；网关与每个子设备分别校验租户、归属和绑定。子项与该子设备的直接上报进入同一设备顺序入口，串行登记接受序号、按该序号发送到同一 U key；不以 gateway key 声称所有子设备天然有序。父包接受顺序、子设备规范化接受顺序和设备采样时间分别记录；F2/V6 核对同目标设备的规范化接受顺序，不推导跨设备总序。需要前置持久清单的拆包路径以清单事务成功作为可恢复接受点；Kafka 失败仍留待发，存储失败不返回已接受。普通直报经过相同设备顺序入口，不能旁路序号。父包记录各子项结果，部分失败不能回业务整体成功；若协议只能一次性应答，业务应答等待规定子项完成，超时保持可查询未完成。传输接收确认与业务回复分开，成功过的子项不重做。新增持久入口和顺序分配属于待实现设计，需补直接/网关混合输入、子项失败/重启、重复包及解绑竞态用例，未运行。

M13 的 action 必须按原 Method/ReqType/Dir 保持三个分支：设备主动请求是 Action→req/up；[ActionSend](../../../../../../backend/things/service/dmsvr/internal/logic/deviceinteract/actionSendLogic.go) 的发送投影是 req/down；设备回复投影是 ActionReply→resp/up。它们进入不同受控 subtype，动态任务按原选择器及这些字段筛选；req 不能进入 command-result 归并，req/down 不是再次下发的指令，resp/up 投影不能二次完成原命令。真实 R 只接受原协议回执并校验持久 token 映射；缺少或矛盾的方向/类型显式拒绝，不能推断为结果。

**盘点的收口方式：** 实施前按 M01–M19 在当前候选生成调用清单，逐个记源文件/符号、注册和 Start、目标类型、消费者、处理结果与用例；缺项阻止该切片宣称完成。新测试库的任务配置也按相同规则做只读导出/校验；未注册 subject、通配歧义或未知 payload 显式拒绝，不自动落到某个通用成功队列。仓外 SDK、当前旧库任务和真实实例数量保持 unknown，本轮不为补清单读取旧环境。

### 9.3 建议的物理拓扑与路由

先以少量稳定职责划分，下面名称是 **bus v1 候选**；`g01` 为新隔离试跑代，环境由独立 broker/资源配置隔离，不复用旧数据。不要将所有现有 raw/canonical 数据管线顺带接入本次迁移：已有数据面 Topic/RawRecord 契约保留，bus 通道是对当前 Eventbus 的受控适配。以后两者衔接需明确去重及单一业务处理者，不能双消费同一事实。

| 通道 / 物理名称候选 | 分区 key 与消费身份 | 处理资源 / 验收 |
| --- | --- | --- |
| U `iot.bus.uplink.v1.g01` | 设备键；`dm.uplink.v1` 内副本竞争；普通遥测/设备事件保持同设备顺序，派生 application 类型不循环触发原处理 | 上报专用 poll、按键串行 worker、producer 缓冲与存储配额；与 C/R/D 分开；V2/V3/V6/V9 |
| C `iot.bus.command.v1.<protocol>.g01` | 设备键；只允许预注册、数量有界的协议 token；DG 共享 broker 模式用 `dg.command.<protocol>.v1` 竞争 | 控制专用池；本地连接模式见下文，不能不同协议共组后丢弃不认识的记录；V4/V9 |
| R `iot.bus.reply.v1.g01` | 设备键；`dm.command-result.v1`；按 commandId 校验归并，独立应用投影消费者有不同 group | 回执持久化专用池，不经 U consumer 或其锁；V4/V9 |
| D `iot.bus.domain.v1.g01` | 领域对象键；如 `dm.relation-cleanup.v1`，其他逻辑订阅者另组 | 领域 outbox、消费与业务存储各保留资源；V3/V9 |
| I `iot.bus.coordination.v1.g01` | 缓存/配置对象键或任务键；广播用 `coord.<role>.<instance>.v1`，任务执行用 `jobs.<logical-task>.v1` | 与遥测分开、有界广播/刷新/任务池；按类型筛选，不能用一组实现全部广播；V1/V5/V7 |
| W `iot.bus.view.v1.g01` | 租户+用户/订阅域键；`api.view.<instance>.v1` 各实例接收 | 展示池与控制/领域池分开，限流/合并仅适用于可从快照恢复的 F5；V8 |

原 subject 保留为兼容路由字段，不是 Topic 名，也不做无授权全局通配。`*` 匹配一个非空段，`>` 仅末尾且匹配一个或多个段；支持具体类型+租户/对象范围白名单，禁止跨租户漏筛。M04 与 M05 先按精确类型分流，未知形状不落入设备发送；M16 重叠选择器对同一逻辑任务只生成一份输入记录，以 eventId+taskId+订阅 revision 去重，停用和更新有持久生效版本，旧配置不因 consumer 重启复活。

**实例定向的首版提案：** DG 经共享 MQTT broker 发布时，可由同协议组内任一具备能力的实例认领；持有本地连接的 SDK 使用该协议 topic、每个活跃实例独立 group，各自读取但仅当前连接 owner 执行。连接登记含稳定 instanceId、启动 epoch 和租约；commandId 的持久发送认领与 owner/epoch 一起校验，旧 owner、双连接及重复记录不允许第二次物理发送。实例 ID 不能每次启动随机换组，退出实例的组回收与重启恢复另验；这是低下行量试跑方案，有按实例读取放大成本，不能称为已验证规模化拓扑。未找到 owner 时账本保留未投递；迁移所有权只有在可证明未开始发送时才允许继续同一命令，开始发送后崩溃按 D7 待核实，绝不因换 owner 自动重发。

首版 U 不拆普通属性/设备事件 Topic，保留同目标设备的规范化接受顺序；其他通道按各自对象/命令身份约束，C/R/D 不等待遥测清空。所有有顺序要求的处理按 key 串行执行业务副作用，前项失败或存在未完成的同对象依赖时，后项不得先写业务；只顺序提交 offset 不够。其他 key 可继续，连续 offset 规则仍须满足。跨通道真实因果以持久关联和对象版本校验，回执引起的属性投影按第 9.4 节处理，不加一个跨 U/C/R/D 的设备大锁。领域对象版本/设备会话标识检查陈旧写；批量删除按子项身份或持久清理清单分步记录。需要原设备序列的事件必须携带并核对来源序列，来源无序列时不能把 Kafka 顺序包装成采样时间顺序。[Kafka 的消费组与投递语义](https://kafka.apache.org/41/design/design/)仅提供此设计的机制依据，无法替代本项目的业务去重与设备执行账本。

### 9.4 公共契约与处理完成边界

**编码候选：** bus v1 使用有 schema 校验的 JSON 信封+按 type 定义的 payload，便于首轮 Go 调用适配与检查；不是任意 map，也不把旧 body 原样放进所有类型。公共字段不覆盖或放宽现有 RawRecord 五个 headers。类型示例 `uplink.telemetry`、`command.request`、`command.reply`、`domain.project.deleted`、`coord.cache.invalidate`；每类单独列 required 字段及未知 major 的拒绝规则，兼容的新增可选字段才能沿用 major。

| 语义 | 候选字段与校验 |
| --- | --- |
| 消息身份 | `eventId` 首次持久接受/源事务时确定，传输重试保持；`type`、`schemaMajor`、`source`、`occurredAt`、`acceptedAt` 必填；事件 ID 不使用 Kafka offset 或每次 retry 新 UUID |
| 租户与对象 | `scope` 明确 tenant/system；tenant 消息必填可信 `tenantId`，来源服务对身份负责，禁止从无可信主体的 body 直接提权。设备键为版本化无歧义编码的 tenant+product+device；领域键为 scope+对象类型+对象 ID，禁止简单点号拼接碰撞 |
| 关联与顺序 | `correlationId` 关联操作，`causationId` 关联触发事件；需要陈旧写保护时有 aggregateVersion/sessionEpoch/sourceSequence。三者不能冒充 eventId 或分区 key；类型声明顺序需求，缺字段拒绝或保持未知，不能补造序列 |
| 命令附加 | `commandId`、协议与目标设备、`expiresAt`、操作者/权限上下文；D10 确认绑定用户、设备、动作摘要及所见待核实命令集合/版本，提交时复核变化，不能复用一张泛化确认票据 |
| 回执附加 | `commandId` 或已持久登记的协议 token→commandId 映射、结果/观测时间；核对租户、设备、协议与请求类型。未知 token 单列待关联，不猜最近命令；迟到回执只更新原命令 |
| 兼容与追踪 | `legacySubject` 仅兼容已有配置/诊断；追踪字段有界且不携带完整凭据/用户上下文。payload 与 type 一致；事件中身份不等同操作授权，消费端仍检查资源边界 |

处理完成不能沿用当前“异步启动即成功”。拟建立 `Handle(record) → 已持久完成 / 已持久待核实 / 可重试失败 / 待人工处置` 的明确结果，只有业务结果或可恢复待办真实落盘才推进相应连续 offset；先存一条“done”再调用设备/数据库不成立。先按键保证业务执行顺序，再按分区提交已经持久处理的连续前缀，不能让后一个同键 worker 先产生副作用，再靠延迟提交伪装有序；rebalance/停止取消未完成工作并保留重投能力。

| 族 | 持久边界与失败处理 |
| --- | --- |
| F1 / D | 来源业务修改与 outbox 同事务；发送失败留待发，成功标记前崩溃可重投。消费者以逻辑订阅者+eventId 去重，业务结果和去重同事务；跨库清理拆有身份的子步骤，待办落盘后可释放 Kafka 分区，但业务状态持续“未完成”，恢复调度不得遗漏或越过同对象依赖 |
| F2 / U | 第 9.2 节设备顺序入口/完整包及子项清单持久保存后承诺补处理，Kafka 成功只是后续传输确认；此拟新增接受点未实现。设备侧 MQTT ACK 不自动等于该接受点。DM 的主写入、去重和后续投递待办必须可恢复；PubApp/Webhook 等异步副作用不能因启动 goroutine 算完成，Webhook 效果未知单列。协议不给稳定来源 ID 时仅保证传输重投去重，设备重复上报的识别能力单列未证 |
| F3 / C、R | commandId 先持久、发送认领前置；可能发送之后的重复投递只查状态，不再执行。ControlReply/GetReportReply/ActionReply 先持久校验/归并可信结果，并在同一事务记后续属性/展示的待办，再唤醒当前请求者；调用者退出不丢账本。原 HandleControl/GetReportReply 中的属性写入不直接放进回执关键路径；执行结果与属性投影进度分开，不能把待办落盘说成全部副作用完成。投影进入有序 U 处理时核对来源版本/序号/会话，迟到值不得覆盖更可信的新值；无可比较版本则保存历史观测并发起受控当前状态查询，不能按到达时间强行覆盖最新属性。待核实有持久记录后可提交该消息进度，让 D10 新操作继续；不同 commandId 的迟到结果不互改 |
| F4 / I | 首次/重连先建立订阅水位，再清本地缓存或加载权威版本并补水位后的更新，最后 ready；遗漏/位点过期触发重新同步。权限缓存同步失败时不能继续使用永久旧授权；不是从 latest 开始就算恢复成功 |
| F5 / W | 页面广播、持久通知与实际用户送达分开；失败提示重取快照，保留既有权限检查。控制结果/删除进度从持久业务状态读取，不能只依赖瞬时推送 |
| F6 / 动态任务 | 接收记录与待调度任务同事务，执行阶段按载荷所属族；SQL/script/设备动作若效果不明标待核实，不靠重置 offset 重跑。任务输入的幂等不等于执行副作用幂等 |

无法解析、未知版本、身份不符的记录保存最小脱敏诊断和原始记录引用，阻止误执行并告警；重要事实不能只进“死信”就宣告业务完成。若隔离坏记录释放分区，须有持久未完成状态及同对象依赖阻挡；保存隔离记录失败不提交 offset。重放仍走原身份/版本校验，管理员处理不改写历史成功状态。

**与现有库的落差：** `foundation/kafka` consumer 关闭自动提交，但只接受数据面 catalog，尚无上述 bus 契约、路由、处理结果协议。P1 需新增有界 bus catalog/客户端入口或明确类型化扩展，继续复用安全连接/缓冲规则；禁止为了让新 Topic 通过而移除现有 data-plane 限制。字段、codec、分区规则及错误测试在 P1 首先落成，不能声称此文已是可执行 schema。

### 9.5 D12 的资源边界与运行参数冻结

接收/最小鉴别层在写入 U 的持久顺序入口前识别 R、C 相关交互，分别使用 producer、缓冲额度和调度队列；若协议解析也共享重池，回执会在 Topic 分流前阻塞，V9 仍失败。未知消息不能仅为获得优先级自报成回执；最小解析核对来源和协议 token，不能借用遥测业务线程或被其占住的设备锁完成分流。

| 边界 | 首版隔离要求 | 故障验证 |
| --- | --- | --- |
| broker/client | U producer 与控制/回执、领域 outbox producer 分开且有界；不同 client 身份可配置限流，禁止共享无界发送缓冲 | 堵塞遥测发送/消费时测 C/R/D 的实际业务结果；共享 broker 资源争用仍纳入 V9，不能拿客户端分开代替验证 |
| 执行 | U、C/R、D、协调和展示分别有有界队列与并发配额；U 内按 key 串行而不同 key 可并发；C/R/D 保留最低资源；组合进程中的逻辑注册及 worker 不能被全局 FastEvent 单例合并 | 遥测 worker 满、处理器挂起、进程重启分别测；同 U key 首项失败时后项不能提前写业务，同时 C/R/D 可继续；若同进程调度/GC 仍致失败，再拆进程验证，不预称隔离成功 |
| 存储/锁 | telemetry 写入并发受限；命令状态/领域补处理保留连接预算与超时；同设备长耗时遥测不可持有控制锁 | 遥测占满连接或锁后控制/解绑错过 W 属 FAIL；即使分池仍须检查数据库服务器/磁盘的共享瓶颈 |
| 恢复/观测 | 按流记录 lag、最老消息年龄、poll/处理时间、池等待、outbox年龄、命令待核实数、清理未完成数与业务完成时间 | 用同一时轴证明 W 内完成时遥测仍有积压；去掉故障后核对已接受事实恢复，不通过丢弃积压造绿 |

本轮没有可用的 Kafka 业务基线，**不伪造已经测得的吞吐、分区数或 W**。建议本地首轮各非命令 Topic 3 分区、每个协议命令 Topic 3 分区、RF=1/minISR=1，仅用于多分区/两副本竞争与广播测试；不证明容灾。消息保留期候选 7 天，替代前文 R2 的 72 小时候选；所有待发/去重/待核实状态不得因该时间直接清除；重新放行超过去重窗口的消息必须核对记录。正式故障运行前在同一 Patch 回执固定实际 broker/客户端版本、机器资源、每池配额、消息大小/速率/总量、样本数、命令有效期与窗口 W/恢复窗口；缺任一项标 NOT_READY。W 由无积压实测与命令有效期约束先确定，再锁定进行积压场景，不能事后调宽换 PASS。

这个参数冻结是 P1–P4 建立可运行链后的技术实验前置，不是再让用户选择 Kafka 参数。本轮交付的是 P0 设计，不是 V9 就绪或运行通过。

### 9.6 下一切片及完成判据

1. **P0 实施入口（尚未执行）：** 获准进入实现后，登记 `platform.eventbus` 在 `tooling/specs-map.json` 的真实调用方；覆盖 Share、SaaS、Things/SDK 及部署/测试，已有模块交叉路径仍要同时声明。补 task 级认领和 v2 Patch 检查投影，仅已确认的决定标 confirmed；不因要过检查把整份草稿批准化。
2. **P1 最小可验切片：** 先做 bus schema/catalog、显式逻辑消费者身份与处理结果/提交边界，保留 data-plane 原契约；再接真实 Cache 两实例广播/重连和定时选择器正反例。需拿到错误不提前提交、重启重投、同组竞争/异组广播与实例恢复的证据，才能扩展业务调用。
3. **P2/P3/P4：** 先真实上报接受与持久链，再项目删除 outbox+清理状态和命令账本/回执；每条链连同 resource profile 进入 V9。P5 完成动态配置/剩余入口，P6 才做目标组合无 NATS 与 V1–V9 整体验收。

本轮验收为 M01–M19 映射可从源码追溯、D1–D12 无漂移、路由/处理边界可评审、导航与索引一致、独立复核无未处置阻塞；不包含实现/部署。遗留：长期模块负责人、仓外 SDK 清单、新环境任务配置检查、最终 schema 与真实资源参数、实施映射和全部 Kafka 运行证据。前两项不由 AI 伪造；其余按相应切片补齐，不能把缺口藏在“P0 已完成”中。
