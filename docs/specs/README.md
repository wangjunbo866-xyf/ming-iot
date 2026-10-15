---
status: draft
version: 0.2.0-draft
---

# Ming IoT 业务地图与规格入口（草案）

本入口随 [团队开发约定](../runbook/team-development.md) 起草，包含工具模块试跑与通知展示规则候选，尚未整体迁移业务规格。以实际采纳范围为实施依据，不能把目录存在视为业务已实现。

## 当前材料

- [团队开发约定](../runbook/team-development.md)：人、AI、Skill 和工具的责任，认领、变更、生效与交付规则。
- [ming-specs Skill](../../.agents/skills/ming-specs/SKILL.md)：按任务执行认领、变更、差异检查和收敛。
- [记录模板](../../.agents/skills/ming-specs/references/templates.md)：只在实际需要的环节使用。
- [草案材料目录](../meta/DOC_REGISTRY.yaml)：本次草案的正文定位与采纳状态。

## 模块地图

| 模块 ID / 业务能力 | 模块入口 | 人的负责人 | 前台 / 后台 / 代码范围 | 当前基线及有效补丁 | 在途变更 |
| --- | --- | --- | --- | --- | --- |
| engineering.api-contract / HTTP API 契约校验 | [模块入口](engineering/api-contract/README.md) | 长期未分配；本次责任见认领 | 无前台；apicheck lint CLI；cmd/apicheck 两文件 | observed-v0 现状基线，无有效覆盖 | [lint 文件选择 Patch](engineering/api-contract/patches/PATCH-20261013-lint-selection/change.md)，confirmed、本地试跑 |
| saas.notification-trigger / 通知触发人展示 | [模块入口](saas/notification-trigger/README.md) | 长期未分配；本轮用户确定业务取舍 | 无前台写入；SaaS 通知格式化两文件 | observed-v0 现状基线，无有效覆盖 | [展示名称 Patch](saas/notification-trigger/patches/PATCH-20261014-display-name/change.md)，confirmed、R2、本地试跑 |

由模块负责人确认新条目；AI 可以起草地图，但不能虚构负责人、路由或有效状态。同一业务模块跨仓时，在条目中分别给出仓库与契约版本，不复制多份权威正文。

## 建议结构

下面是创建真实模块时的模板，目录尚未按示例创建：

```text
docs/specs/<业务域>/<模块>/
├── README.md                  # 认领、当前版本、有效覆盖、在途变更
├── raw.md                     # 原始需求与澄清来源，可用可访问的来源引用代替
├── spec.md                    # 业务意图、条款 ID、验收和适用状态
├── design.md                  # 需要时拆出的技术方案；小任务可在 spec 中分节
├── patches/<变更ID>/change.md  # 局部演进；实现修正可说明规格无变化
└── sunset/<变更ID>/change.md   # 退出、替代、迁移和验证

docs/wiki/blue-print/<蓝图ID>.md # 探索材料，正式采纳前不并入现行规格
```

模块入口展示“基线 + 已生效覆盖”，并把草稿/已确认未实现的候选单独列出。`changelog` 只导航到变更及证据，`architecture` 保留跨模块架构决策的单一正文。原始资料可以留在 wiki 的来源区，提炼后以来源链接连接。

现有 `backend/saas`、`backend/share`、`backend/things` 是代码组织线索；业务地图仍需按实际能力细分。巡检的同仓设计方向不代表它已经有实现或已确认的业务规格。
