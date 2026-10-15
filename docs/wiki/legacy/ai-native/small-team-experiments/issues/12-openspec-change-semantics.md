# OpenSpec 原生规格、差量及归档如何支持功能与补丁的演进？

Type: research
Label: wayfinder:research
Status: resolved
Assignee: article_research
Parent: [小团队运行方案与 IoT 试点决策地图](../map.md)
Blocked by:

## Question

从官方文档与固定源码核查：现行 capability spec 与 Change delta 的关系；ADDED/MODIFIED/REMOVED 合并、归档后的可读状态；层次 capability 路径与 change 命名；只修复实现、不变行为时的表达；功能全部退役的能力与限制。说明哪些是原生机制，哪些只是团队分类约定。

## Context

服务于 [feature、patch、sunset、blueprint 如何映射到 OpenSpec 与业务地图？](11-feature-patch-map.md)。沿用此前核查的 v1.14.1 与 main 快照，以官方来源补核必要实现，不安装或修改业务仓库。

## Answer

2026-10-10 固定 OpenSpec v1.14.1、SHA `87c3595ace6a2e22957f39ebe5b74c2f2316e0cb` 核查完成。现行 capability 全文与 Change 差量足以承接 Feature/Patch；四种标签是团队分类而非内置四套类型。MODIFIED 包含受影响单条 Requirement 的完整新正文与场景，不必复制整个 Feature；同步后的主规格承接整合结果，归档保留历史。

已核实层次 capability 路径、无规格变化的 `skip_specs`、显式完整退役 `retire_capabilities`，以及单层 Change slug 的边界。sync/archive 不证明批准、代码合并或环境生效。

- [OpenSpec 原生功能、补丁与退役演进核查](/home/wangjunbo/.cache/ai-native-wayfinder-worktrees/openspec-context-20261010/docs/research/2026-10-10-openspec-feature-patch.md)
- 研究分支：`research/openspec-feature-patch-20261010`；提交：`ea376084b8bb35a34fd29dc62988761cdeb1ea51`。
- 可恢复内容：该提交下 `docs/research/2026-10-10-openspec-feature-patch.md`。
- 仅静态核查，未安装或实测 OpenSpec。提交前 52 项测试与技能校验通过，只代表研究所在配置仓基线检查通过。
