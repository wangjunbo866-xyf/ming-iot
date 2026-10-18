---
status: local-candidate
version: 0.1.0
date: 2026-10-17
goal_id: ming-iot-knowledge-pilot
---

# 通知运行基线与 Kafka 需求受理试跑

本轮继承已验证的本地入口修正候选 `c304bccd1663ee1299dc73d99855c61339b6ef4a`，对应 main 快照为 `56006502324800448c9775944d28478254be6142`。按用户授权准备隔离开发环境，继续使用同一 Goal；同步将首条 Kafka 需求和团队流程落盘。未推送或合并本轮候选。

## 实际运行结果

- 真实现有 syssvr 运行于独立 Compose 项目，通过真实 NotifyManage RPC 验证 manual、auto、deviceButton message/systemNotice 四个分支，检查正文、触发信息、租户和唯一接收用户关联。
- 第一次发送产生四条带唯一标记的消息；停止再启动后，四条原消息的全部字段快照逐字一致。第二次再发送四条通过，旧数据保留。
- 统一 `verify-integration VERIFY_SUITE=notification` 随后再验证两轮，重启时十二条既有消息及接收者完整字段一致；最终停止本项目四个容器，全部数据保留。后续复跑会继续追加样例。
- 实际网络为 internal、无宿主端口。启动前已有六个容器的 ID、状态、启动时间、网络未变；已有 39 个卷的名称、创建时间、driver、labels 未变。此为本机前后快照，不能推导旧环境业务数据的全量审计。
- 初次使用普通 PostgreSQL 时 AutoMigrate 因 geometry 类型缺失失败；改为独立 PostGIS 后通过，原失败日志保留。首次失败产生的普通 PG 卷及匿名 Redis 卷保留，没有清理或重用旧业务卷。
- profile 中的公开测试配置只用于本项目内网，关闭短信、UniPush 和定时外联。运行日志可能带既有静态 gitInfo，实际构建身份由二进制 hash、镜像 ID 和最终候选回执记录。

真实通知链不经过 Eventbus，单独 syssvr 也未启动 FastEvent 消费。本轮不虚构告警上游，不宣称 NATS 消费、Kafka 切换、HTTP 鉴权、外部消息渠道、真人验收或生产发布已通过。

## 文档与方法交付

- [Eventbus 模块入口](../specs/platform/eventbus/README.md)连接原话、observed 现状与 R1；迁移目标和开发测试一次切换来自用户确认，其他语义待决。新模块尚未进入 PR 自动映射，实施前补齐认领与投影。
- [从一句话到交付](../runbook/requirement-to-delivery.md)定义结果落点、人的决定、AI 的执行、工具检查及出口标准；团队约定和 ming-specs 引用它，不复制另一份业务规则。
- [通知启动与测试说明](../runbook/notification-baseline.md)连接实际可执行入口、测试输入/预期和未覆盖范围。当前实现没有改动通知有效规则。
- 发现现有索引绑定支持主规格 version，但未支持 Patch revision；在原有白名单内补充 frontmatter.revision，并以 R1→R2 索引失配、缺失/错误类型反例验证。未扩展文档发现范围或引入新门禁。
- 总纲、业务地图、知识入口和来源索引同步当前状态；旧阶段文档保留原时点，不全库改写历史。

## 验证与独立复审

本地真实运行日志、隔离对照、构建身份及初次失败证据保存在同一 Goal 本轮私有证据目录。最终候选完整 SHA、verify-pr、完整分支差异声明、公开边界检查与独立 Review 由该 Goal 的外部回执绑定，避免本文件自引用提交。此处的运行结果不替代这些最终检查。

可复跑入口为 `make verify-pr`、`make verify-integration VERIFY_SUITE=notification` 及针对实际 base/head 的 `tooling/specs_pr.py`。没有运行的 suite 必须单列；Kafka/Things 集成与发布 profile 不是本轮通知基线的通过项。

## 成本与本轮发现

本轮同时建设环境和受理流程，不能当作一次普通需求的日常文档成本。需求材料新增四份：模块入口、原话、现状主规格、同模块 Patch；流程与运行说明分别复用 runbook 分类。没有为每个步骤建立任务、评审或交接包文件。

| 观察 | 记录与含义 |
| --- | --- |
| 人工关键澄清 | 本轮切换场景一问一答，已记入 D2；未要求用户选择技术配置 |
| 找到的真实缺口 | 通知无完整 Eventbus 上游；普通 PG 不满足 syssvr 的 geometry 依赖；Eventbus ACK 早于业务回调，可靠性不能照搬假设 |
| 日常维护成本 | unknown；Kafka 尚未实施，不能宣称全流程已闭环或提高效率 |
| 后续测量 | 在 R1 的同一任务中记录定位/文档/验证时间、补问、重复填写、误报及返工；首次建设单列 |

## 下一步

按 [R1 待决问题](../specs/platform/eventbus/patches/PATCH-20261017-kafka-migration/change.md)给出完整调用清单与具体业务例子，确认可靠性、旧消息处理和第一条真实事件验收路径；随后形成可实施方案及检查映射。迁移完成后再用实际结果评价团队流程和 harness，不以这次文档或运行基线完成关闭整个 Goal。

旧环境、旧仓、私有备份与历史回执保留；不恢复小黄反馈，不启用自动去重、自动归档或全库门禁。
