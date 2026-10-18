---
status: draft
version: 0.5.0-draft
---

# Ming IoT 业务地图与规格入口（草案）

本入口随 [团队开发约定](../runbook/team-development.md) 起草，已收敛工具校验和通知展示两个局部模块的 current-v1，尚未整体迁移业务规格。以实际采纳范围为实施依据，不能把目录存在视为业务已实现。

## 当前材料

- [团队开发约定](../runbook/team-development.md)：人、AI、Skill 和工具的责任，认领、变更、生效与交付规则。
- [ming-specs Skill](../../.agents/skills/ming-specs/SKILL.md)：按任务执行认领、变更、差异检查和收敛。
- [记录模板](../../.agents/skills/ming-specs/references/templates.md)：只在实际需要的环节使用。
- [草案材料目录](../index.yaml)：本次草案的正文定位与采纳状态。

首次收到目标时，按[受理到交付操作页](../runbook/requirement-to-delivery.md)进入模块。业务/能力域、Feature、服务与变更类型的区别见[划分标准](../runbook/team-development.md#domain-feature-service)；不通过目录深度判断类型。Eventbus 的 P1 已在本地候选实施验证，其余 Kafka 迁移和 ProtocolApplication 设计不能据此视为完成。

## 域与能力导航

[protocol / 协议适配](protocol/README.md)维护协议解释、标准化及适配版本等职责，本轮只建立上行闭环 Feature；ProtocolApplication 是拟承载这些职责的应用实现名称，部署方式未定。`platform`、`saas`、`engineering` 继续作为现有能力分组；本轮不重排其他 Feature。

## 模块地图

| 模块 ID / 业务能力 | 模块入口 | 人的负责人 | 前台 / 后台 / 代码范围 | 当前基线及有效补丁 | 在途变更 |
| --- | --- | --- | --- | --- | --- |
| platform.eventbus / 跨服务事件总线 | [模块入口](platform/eventbus/README.md) | 长期未分配；用户决定与适用范围见当前 Patch | Share、SaaS、Things 与 SDK；具体认领见当前 Patch | [p1-v1](platform/eventbus/spec.md)，本地 P1 当前规则及旧链路观察；未推送或切换旧环境 | [Kafka 迁移 Patch](platform/eventbus/patches/PATCH-20261017-kafka-migration/change.md)，分段实施，后续设计未完成 |
| protocol.uplink-normalization / 设备上行解码与标准化 | [Feature 入口](protocol/uplink-normalization/README.md) | 长期未分配；本轮文档认领见入口 | 既有 DG、SDK、DM 为现状线索；本轮无产品代码认领 | [0.2.1-draft](protocol/uplink-normalization/spec.md)，当前设计候选，尚无新链路生效版本 | 本目录 raw/spec 知识已收敛；批量/编号/迟到取舍已确认，真实协议与恢复接线待冻结，尚未实施 |
| engineering.api-contract / HTTP API 契约校验 | [模块入口](engineering/api-contract/README.md) | 长期未分配；本次责任见认领 | 无前台；apicheck lint CLI；cmd/apicheck 两文件 | [current-v1](engineering/api-contract/spec.md)，无需叠加历史 | 无；lint 选择 Patch 已 absorbed |
| saas.notification-trigger / 通知触发人展示 | [模块入口](saas/notification-trigger/README.md) | 长期未分配；本轮用户确定业务取舍 | 无前台写入；SaaS 通知格式化两文件 | [current-v1](saas/notification-trigger/spec.md)，仅 manual 昵称优先 | 无；展示名称 Patch R2 已 absorbed |

通知与 API 校验两份主规格以 `c165a56531f58421a870a0b8794f24556b60d3b2` 为来源基线，已随 PR #5 合并。核对至 main `56006502324800448c9775944d28478254be6142`，两个模块相关实现未变，继续适用 `current-v1`；当前合并验证与环境缺口见 [PR #6 后续记录](../changelog/2026-10-17-governance-merged-next-plan.md)。[S1 记录](../changelog/2026-10-15-specs-current-baseline.md)及各模块 observed 原文保留原时点，不作为当前候选或部署状态。业务地图目前有两个已合并试点及 Eventbus 的本地 P1，不表示全平台规格已完成。

上行 Feature 的需求、澄清及当前设计已完整归入[协议适配域的本地入口](protocol/uplink-normalization/README.md)；[Kafka Patch R13](platform/eventbus/patches/PATCH-20261017-kafka-migration/change.md#protocol-domain-extraction)保留原始决定、历史评估和跨域依赖。新文档入口不表示 `tooling/specs-map.json` 已覆盖产品代码；具体实施前按实际认领补映射。

由模块负责人确认新条目；AI 可以起草地图，但不能虚构负责人、路由或有效状态。同一业务模块跨仓时，在条目中分别给出仓库与契约版本，不复制多份权威正文。

## 建议结构

下面是可选结构；按任务需要创建，不要求每个小修改新建 raw、design 或 Patch 文件：

```text
docs/specs/<业务域>/README.md  # 有需要时维护域职责、能力导航与共享契约链接
docs/specs/<业务域>/<模块>/     # 长期功能模块（Feature），无须再加 features 层
├── README.md                  # 认领、当前版本、有效覆盖、在途变更
├── raw.md                     # 原始需求与澄清来源，可用可访问的来源引用代替
├── spec.md                    # 业务意图、条款 ID、验收和适用状态
├── design.md                  # 需要时拆出的技术方案；小任务可在 spec 中分节
├── patches/<变更ID>/change.md  # 局部演进；实现修正可说明规格无变化
└── sunset/<变更ID>/change.md   # 退出、替代、迁移和验证

docs/wiki/blue-print/<蓝图ID>.md # 探索材料，正式采纳前不并入现行规格
```

模块入口默认指向完整当前主规格，在途候选与历史另列。已有“基线 + 已生效覆盖”仅为待收敛过渡状态；新行为在同一候选内完成主规格收敛再合并。`changelog` 只导航到变更及证据，`architecture` 保留跨模块架构决策的单一正文。原始资料可以留在 wiki 的来源区，提炼后以来源链接连接。

现有 `backend/saas`、`backend/share`、`backend/things` 是代码组织线索；业务地图仍需按实际能力细分。巡检的同仓设计方向不代表它已经有实现或已确认的业务规格。
