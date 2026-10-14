# 现行文档如何接入 OpenSpec 与薄导航，并验证新会话能找到正确上下文？

Type: grilling
Label: wayfinder:grilling
Status: open
Assignee:
Parent: [小团队运行方案与 IoT 试点决策地图](../map.md)
Blocked by: 04, 05, 11, 13, 14

## Question

在已选定的 OpenSpec＋薄导航路线下，确定首个试点的最小接入方案：如何从工作区及单代码仓入口发现同一文档根；旧权威文档、业务地图与 OpenSpec 如何映射并逐步切换；既有 Review/Evidence 和元数据校验如何保留；缺失或冲突的来源如何呈现。给出用新会话和跨成员交接检验上下文可发现性的步骤及通过/退回条件。

本票据交付接入决策和试点准备计划，不直接执行安装、文档迁移或业务实现；不要求先整理完整项目，也不预设增加新的平台。

## Context

- 2026-10-10 本轮用户已确认保留 OpenSpec 原生目录；当前先具体化遥测上报的薄导航样例、自动聚合、双向链接和归档后链接更新。既有文档权威切换与真实试点仍待后续核对，不由样例代替。

- 前置决定：[如何用最少的人工维护使规格、实现与证据持续对齐？](05-document-maintenance.md)。
- 组织方式已补充：[Feature/Patch 与业务地图映射](11-feature-patch-map.md) 采用业务域 → 功能、跨仓连接实现；具体业务域与模块清单仍需核对，SaaS/Things 保留为技术视图。
- 关联与阅读边界已核查：[主规格与 Patch 双向关联](13-patch-backlinks.md)。接入时细化可重建反向索引与稳定入口、固定基线链接，以及部分同步/并行修订的解释；索引位置和生成入口仍是待落实建议，不当作原生功能。
- 方案依据与建议：[以 OpenSpec 为核心的全栈项目上下文](../assets/05-openspec-project-context.md)。
- 2026-10-10 核查时，旧 specs/plans/reviews/evidence 仍为权威，map 标为设计试点；产品及研发文档仓根尚无 openspec 目录。再次制定实际接入顺序前需复核当时状态。
- 本票据决定通用的发现与权威切换方式；具体业务需求、环境参数和量化试点目标仍由试点票据承接。

## Working notes — 2026-10-10

后续用户提出独立 map-delivery 路线；接入方案先受 [工具依赖重评](14-standalone-map-delivery.md) 约束。现有样例继续作为比较材料，不据此继续推广 OpenSpec 适配。

用户已明确保留原生 OpenSpec 目录。本轮将薄导航具体化为按功能聚合的生成页、按 Change ID 固定的入口，以及 Purpose/Impact 内有界的返回导航。

- [遥测上报薄导航接入草案](../assets/10-thin-navigation-telemetry.md)
- [可运行的隔离目录样例](../assets/thin-navigation-example/README.md)
- [场景验证结果](../assets/thin-navigation-example/validation.md)

样例完成 8 项导航检查，包含模拟归档后稳定入口和相对返回链接更新；区块外正文保留。原生 OpenSpec CLI、真实文档迁移、跨仓权威切换与新会话发现仍未验证。具体生成约定作为可检查建议保存，不自动成为团队已批准规范。

本票据保持开放：剩余接入问题是现有资料映射、原有治理校验保留及真实新会话/交接验证，不能用导航样例代替。未安装工具到实际业务仓或改写其规格。
