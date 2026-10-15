# 菜鸟、天猫及补充实践有哪些可迁移机制与维护成本？

Type: research
Label: wayfinder:research
Status: resolved
Assignee: article_research
Parent: [小团队运行方案与 IoT 试点决策地图](../map.md)
Blocked by:

## Question

完整阅读用户提供的两份 PDF，区分作者报告、实际机制、目标和缺失证据；解释菜鸟方案如何缩成小团队能执行的步骤，以及天猫多文档流程的维护压力来自哪里。补充少量有第一方来源的实战案例，说明可借用的具体机制、前提和不能推断的效果。研究只提出选项，不代替团队决定流程。

## Context

- 菜鸟 PDF：`/mnt/d/学研日报/SDLC 与 AI 研发实践日报/菜鸟AI研发效能实践.pdf`
- 天猫 PDF：`/mnt/c/Users/Administrator/Documents/kimi/tasks/2026-10-08/15-52-46-fd39deab/天猫AI项目研发协同实践.pdf`
- 用户已经选择 OpenSpec 作为规格与变更入口，不能引入重复维护的权威文档。
- 研究分支：`research/ai-native-team-practices-20261009`。

## Answer

2026-10-09 完成两份 PDF 全文与关键图表核读（菜鸟 13 页、天猫 25 页），并补充 Stripe、Anthropic、OpenAI 三篇第一方工程实践。菜鸟最可迁移的是带前置检查、真实产物验收和恢复的执行循环；天猫最可迁移的是职责边界、显式规格差异处理、集成态检查及告警裁定。多份文档的人工同步与多状态权威是主要维护负担，应在后续票据讨论如何映射到已有 OpenSpec。

- [研究报告：两篇实践的可迁移机制与维护成本](/home/wangjunbo/.cache/ai-native-wayfinder-worktrees/practices-20261009/docs/research/2026-10-09-ai-native-team-practices.md)
- 研究提交：`884b563d85d3a2a115f927f7e2921af446b05601`，分支 `research/ai-native-team-practices-20261009`。
- 可恢复内容：该提交下 `docs/research/2026-10-09-ai-native-team-practices.md`。
- 证据范围：PDF、关键图、官方文章与部分公开示例源码；文中业务效果属于作者自报。本团队未部署平台、未运行这些交付方案；部分 PDF 代码块横向截断，不能直接复现。
- 本票据只解决机制与证据调查；角色、验收、文档和试点采用决定未由 Agent 代定。
