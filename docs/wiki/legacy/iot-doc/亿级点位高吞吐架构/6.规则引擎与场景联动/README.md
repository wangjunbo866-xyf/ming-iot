# 场景联动与规则引擎

> **状态：核心方案已确认。**
>
> **恢复 ADR：未冻结。** 只有主规格 §19 所要求的证据（包括 POC、故障注入与容量实测）和 RACI 签字全部完成，方可另行评审和冻结。当前状态不表示 POC 已通过、规格全文已冻结或 V1 已完成。

## 第一次阅读

第一次接触本模块时，推荐阅读顺序固定为：

1. [《15 分钟读懂场景联动》](00-15分钟读懂场景联动.md)
2. [《场景联动 Runtime 主规格总纲》](场景联动%20Runtime%20主规格总纲.md)
3. [《场景有状态计算规范》](场景有状态计算规范.md)
4. [《Scene Current Fact 设计》](Scene%20current%20fact设计.md)
5. [《水线与读取屏障详解》](水线与读取屏障详解.md)
6. [《算子语义与术语》](算子语义与术语.md)

导读用一个多设备规则串起程序节点、Topic、Consumer Group、状态和事务；随后依次建立总体架构、
有状态计算、Current Fact 与读取屏障模型，最后把旧术语文档仅作为共享术语和兼容入口。

阅读任何一篇文档时，都可以打开[《场景联动完整案例端到端时序图》](场景联动完整案例端到端时序图.html)对照定位。它用“3 个温度点求 5 分钟平均值 + 门状态持续满足 + 启动两台风机并调用 Webhook”串起 Durable Canonical、共享计算、Current Fact、Scene 求值、动作接受和回复归并；请在浏览器中缩放或全屏阅读。

## 按问题查阅

| 我正在解决的问题 | 直接阅读 |
| --- | --- |
| 确认产品边界、wire、Runtime 拓扑、跨组件协议、Outbox 事务边界、route/hash 或容量 | [主规格总纲](场景联动%20Runtime%20主规格总纲.md) |
| 理解 Manager、Actor lane、多 partition 隔离、completion、恢复、rebalance、硬故障或 Zombie | [Runtime 调度、分片与故障转移][runtime-coordination] |
| 实现 `SceneBindingState`、latest/rolling、五类内建有状态算子、Timer、冷启动或 `NOT_READY` | [场景有状态计算规范](场景有状态计算规范.md) |
| 理解要求水线、实际水线、read barrier、retry / fail closed / freshness，以及 retry 是否阻塞 Kafka | [水线与读取屏障详解](水线与读取屏障详解.md) |
| 实现 Current Fact、水线/barrier、coverage seed、冲突 fence、双档部署或存储选型 | [Scene Current Fact 设计](Scene%20current%20fact设计.md) |
| 实现 Binding、Scope、ProjectIndex、matching 或 fanout | [控制面与匹配契约](控制面与匹配契约.md) |
| 查询 Reducer、ComputationPlan、Fact 等共享术语或跟随旧锚点 | [算子语义与术语](算子语义与术语.md) |
| 制定 SLO、POC、故障注入、rollout 或 RACI | [SLO、故障测试与实施](SLO故障测试与实施.md) |
| 了解方案来源和竞品证据 | [竞品状态调研](附录-竞品状态方案调研.md) · [ThingsBoard 源码分析](附录2-thingsboard规则引擎与CF实现分析.md) |

## 文档权威层级

[主规格总纲](场景联动%20Runtime%20主规格总纲.md)与规范性分章共同组成
scene-linkage 的规范性架构决策源：主规格拥有产品、wire、Runtime 拓扑、跨组件协议、Action Outbox
事务边界、route/hash 与容量；
[《Runtime 调度、分片与故障转移》][runtime-coordination]唯一拥有 partition ownership、
Manager/Actor 执行、completion、restore、rebalance 与 Zombie 协议；
[《Scene Current Fact 设计》](Scene%20current%20fact设计.md)唯一拥有 Scene Current Fact 的投影、
写入 frontier、读取 API、双档部署与存储选型；[《场景有状态计算规范》](场景有状态计算规范.md)
唯一拥有 `SceneBindingState`、latest/rolling、五类内建有状态算子、Timer 与有状态求值语义。
[《水线与读取屏障详解》](水线与读取屏障详解.md)专门解释读取屏障、水线、retry/fail-closed 与
freshness；算子语义与术语只保留共享术语和旧锚点兼容入口。控制面与匹配契约、SLO 故障测试与
实施是从属实施契约，导读只负责解释，两份附录只提供非规范性研究。
架构源之间没有"后写覆盖"关系；冲突必须在同一次变更中联合修订。

## 跨目录同步边界

[父架构](../enterprise-iot-platform-architecture-design.md)拥有平台产品边界与非 Scene 子系统决策，[Kafka 设计](../4.1.Kafka/1.nats-to-kafka-greenfield-design.md)拥有 Raw、Canonical、领域事件等非 Scene Kafka 物理契约。Scene 内按上述职责拆分：主规格总纲拥有产品、wire、Runtime 拓扑、跨组件协议、Action Outbox 事务边界、route/hash 与容量；Runtime 调度分章拥有调度、partition lifecycle、completion、restore 与故障转移；[《场景有状态计算规范》](场景有状态计算规范.md)唯一拥有 `SceneBindingState`、latest/rolling、内建算子、Timer 与有状态求值语义。各规范源没有优先级；任一跨目录冲突都必须在同一次变更中显式修订全部受影响的规范性文档，不能依靠权威排序或后写文档自动覆盖。同步完成前，冲突部分不得作为已冻结实施契约。

《Scene Current Fact 设计》的 DIRECT_TD 档以[《质量码契约设计》](../2.编码解码协议/质量码契约设计.md)为硬前提（坏状态逐列成行、ABSENT 与 NULL_VALUE 可区分）；通用 latest 的降级（父架构侧）随投影契约设计 §8.5/§13 第 30 项同次联合修订。

本次已联合修订 Scene RocksDB/Kafka EOS/旧 automation Topic、immutable affiliation snapshot/offset barrier、缺失 Processing Mode，以及"state/timer/Signal/Intent 与 Outbox 同一 PG 事务"的冲突；完整处置矩阵见[主规格 §18.1](场景联动%20Runtime%20主规格总纲.md#181-跨目录规范一致性)。Raw → Canonical Kafka transaction 与 protocol/model/ownership 自身的 cutover 不在撤销范围内。

[runtime-coordination]: Scene%20Runtime%20调度、分片与故障转移.md
