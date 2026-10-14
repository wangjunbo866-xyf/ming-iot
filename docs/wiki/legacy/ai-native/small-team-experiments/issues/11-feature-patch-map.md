# feature、patch、sunset、blueprint 如何映射到 OpenSpec 与业务地图？

Type: grilling
Label: wayfinder:grilling
Status: resolved
Assignee: root
Parent: [小团队运行方案与 IoT 试点决策地图](../map.md)
Blocked by: 05, 12

## Question

在已选择的“OpenSpec 原生结构＋薄导航、按需求补知识”路线下，至少保留 feature 和 patch 的文档语义，并说明 sunset、blueprint 的落点。区分长期功能规格与一次变更，明确候选补丁、现行规格和历史记录的关系，以及业务地图如何划分并跨仓连接。

## Context

用户本次明确要求至少具备 feature 和 patch，并询问方式 1 是否可以支持。沿用已选路线，不另建一套权威规格或实施迁移。具体目录与术语映射是本票据的设计问题；现行文档接入仍由 [现行文档如何接入 OpenSpec 与薄导航，并验证新会话能找到正确上下文？](10-context-adoption.md) 承接。

- [组织方案与 IoT 示例](../assets/11-feature-patch-business-map.md)
- [原生机制核查](12-openspec-change-semantics.md)

## Comments

2026-10-10 用户要求至少保留 feature/patch；在看到业务域与现有 SaaS/Things 两种地图视角后，明确选择：“业务域 → 功能，跨仓连接实现（推荐）”。

## Answer

继续使用方式 1，并以 **业务域 → 功能，跨仓连接实现** 作为业务地图主阅读视角。SaaS/Things 保留为代码和技术视图，不作为本轮选定的一级业务分类。

采用原生 OpenSpec 可满足至少 Feature/Patch 的要求：功能现行行为由 capability spec 承载，首次 FEATURE 与后续 PATCH 使用原生 Change 记录；需要变更行为时写 delta，收敛后更新主规格并保留变更历史。Sunset 可用退役 Change 表达，Blueprint 保留为明确未定案的探索资料。四类是团队组织语义，不宣称 OpenSpec 内置同名模板或审批状态。

地图聚合现行规格、当前架构/接口、相关变更、运行与验证入口；业务正文只保留一份。分类与生命周期的具体映射见关联方案，不在地图重复正文。

本轮确认组织原则，不把示例中的六个业务域、模块清单、扁平 capability ID 或文档切换时点当作逐项批准。真实清单、旧规范兼容和接入验证继续由后续接入票据核对；没有迁移业务文件或实施示例需求。
