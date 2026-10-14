# OpenSpec 如何关联主规格与 Patch，并避免两者的语义二义性？

Type: research
Label: wayfinder:research
Status: resolved
Assignee: article_research
Parent: [小团队运行方案与 IoT 试点决策地图](../map.md)
Blocked by: 12

## Question

核查 OpenSpec 对 Patch → 主规格及主规格 → 相关 Patch 两个方向的实际支持，区分差量结构关联、自动生成的链接、CLI 可查询信息和需要团队补充的导航。说明候选、已同步但未归档、已归档、撤销和并行冲突时的阅读语义；给出符合既定薄导航路线、避免双份正文的组织建议。

## Context

用户引用天猫 Patch 双向指针以消除主文档与补丁的语义二义性，追问 OpenSpec 如何实现。这是既有路线下的机制核查，不重新选择 OpenSpec 或要求用户再次批准已选路线。研究采用固定版本官方源码，不执行业务文档迁移。

- 前置研究：[OpenSpec 原生规格、差量及归档如何支持功能与补丁的演进？](12-openspec-change-semantics.md)。
- 当前决定：[Feature/Patch 与业务地图映射](11-feature-patch-map.md)。

## Answer

2026-10-10 固定 OpenSpec v1.14.1、SHA `87c3595ace6a2e22957f39ebe5b74c2f2316e0cb` 核查完成：Patch 差量以 capability 路径和 Requirement 名指向主规格要求；默认不自动生成双向 Markdown 指针，主规格查询也不提供相关 Change 列表。

原生查询和差量可作为团队生成反向导航的来源。建议在已有 map 能力页生成主规格 → Patch 索引，Patch 引用当前入口与固定基线，避免手写两份覆盖清单。这是接入建议，尚未实现或单独定案。活动目录可能包含已同步甚至部分同步的 Change，归档也不证明批准或上线；必须分开读取这些事实。

- [机制解释与最小补齐方案](../assets/13-patch-bidirectional-context.md)
- [固定源码研究报告](/home/wangjunbo/.cache/ai-native-wayfinder-worktrees/openspec-context-20261010/docs/research/2026-10-10-openspec-patch-links.md)
- 研究分支：`research/openspec-patch-links-20261010`；提交 `f7ef7f44809883beec858bdd9eab937a236a78f3`；可恢复文件为该提交下 `docs/research/2026-10-10-openspec-patch-links.md`。
- 限制：未安装或执行 OpenSpec。提交前配置仓 52 项测试和技能校验通过，不等于 OpenSpec 合并或反向索引已验证。本票据解决机制事实，实际索引和读取规则仍由接入票据细化。
