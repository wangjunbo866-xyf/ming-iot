# 如何用最少的人工维护使规格、实现与证据持续对齐？

Type: grilling
Label: wayfinder:grilling
Status: resolved
Assignee: root
Parent: [小团队运行方案与 IoT 试点决策地图](../map.md)
Blocked by: 01, 04, 09

## Question

在 OpenSpec 已确定的前提下，选择需要人工维护的最小内容、生成或索引的视图、不可变证据的存放及关联方式。明确谁在何时更新需求和设计、变更影响如何评审、如何避免天猫式详细文档被照搬为多份相互矛盾的事实。确定文档与代码变更在何处被一起验收。

2026-10-10 用户补充重点：优先借鉴天猫 collar 的全栈结构化知识库，让 AI 从 AGENTS.md、业务地图进入规格、代码、运行上下文和参考资料。需要判断默认 OpenSpec 的覆盖范围，以及怎样补齐导航和上下文而不增加一套业务事实或大量文档维护工作。精简的是重复维护，不是有价值的项目知识。

## Context

2026-10-09 已核读 OpenSpec 官方 [变更迭代说明](https://github.com/Fission-AI/OpenSpec/blob/main/docs/editing-changes.md) 与 [定制说明](https://github.com/Fission-AI/OpenSpec/blob/main/docs/customization.md)：变更内包含 proposal、specs、design、tasks，允许迭代；verify 提供差异诊断，不构成阻断式验收，项目提示规则也不等于强制检查。这为最小文档方案提供现成结构，但本票据仍需确定团队的批准与失效规则。

2026-10-10 本地只读核查发现：产品及研发文档仓根尚无 `openspec/`；研发文档仓旧 `specs/plans/reviews/evidence` 仍是当前权威，`map/` 仍标为设计试点。接入时需要明确目标 OpenSpec 与现行元数据/PR 校验的权威切换方式，避免两套正文同时维护。事实与来源见 [责任和验收走查的接入记录](../assets/04-responsibility-acceptance-walkthrough.md#7-当前项目接入事实与待补项)。路线选择已记录于 Answer，具体接入问题另立后续票据。

- [以 OpenSpec 为核心的全栈项目上下文](../assets/05-openspec-project-context.md)：路线 A 已选定；附能力依据、IoT 上下文查找过程、维护触发及新会话验收建议。
- 关联事实调查：[默认 OpenSpec 与可定制能力覆盖了多少全栈项目上下文？](09-openspec-project-context.md)。

## Comments

2026-10-10 向用户展示两个候选路线：保留 OpenSpec 原生结构＋薄导航，或当前就用定制 schema 统一更多知识产物。用户随后明确选择：“A：OpenSpec＋薄导航，按需求补知识（推荐）”。

## Answer

第一版采用 **OpenSpec＋薄导航，按需求补知识**。OpenSpec 保留原生规格/变更结构；现有业务地图连接代码、架构、runbook、参考资料和必要的动态查询。地图记录关系与入口，不复制业务规格、设计、任务或执行进度。知识随真实需求补齐，第一版不预先把研究、上下文引用等都规定为新的固定 schema 产物。

文档维护方案沿用已确认的 D/P/R 责任及验收节点：发生变化时更新对应权威来源，其他位置用链接或生成视图；具体维护触发和走查方法见关联方案。用户本次确认的是组织路线，示例需求、具体路径、查询实现、试点指标及迁移细节仍待落实，不能把整份分析视为逐项批准的现行业务规范。

本次没有实施安装、迁移或实际试点。现行文档如何衔接到新入口、如何验证新会话可发现性，转入 [现行文档如何接入 OpenSpec 与薄导航，并验证新会话能找到正确上下文？](10-context-adoption.md)。
