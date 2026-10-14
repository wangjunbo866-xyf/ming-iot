# OpenViking 能否满足个人私有、确认后共享的记忆与 Skills 需求？

Type: research
Label: wayfinder:research
Status: resolved
Assignee: memory_research
Parent: [小团队运行方案与 IoT 试点决策地图](../map.md)
Blocked by:

## Question

核对 OpenViking 当前官方源码、文档与许可证，调查个人/团队命名空间、读写权限、来源与版本、遗忘和纠错、共享晋升、Skills 存储及分发边界。区分开源已实现、企业能力、文档宣称与未验证行为；提出保持 Git/OpenSpec 权威及与 LoopX 共存的候选边界，并列出最小验证场景。不得据此宣布已满足团队隔离或已完成集成。

## Context

- 已定原则：个人私有，经本人或责任人确认后进入团队库。
- 官方候选来源：`https://github.com/volcengine/OpenViking`。
- 研究分支：`research/ai-native-team-memory-20261009`。

## Answer

2026-10-09 完成资料与固定源码调查。OpenViking 已有用户隔离、可选资源 ACL、记忆变更审计、快照和 Skills 来源/完整性能力；本次检查范围内未发现完整的强制确认后共享审批流程。个人独立 user 身份与团队正式目录权限仍须配置、验证。LoopX 已有相关适配实现，但本团队未部署或验证互操作。采用决定留给后续讨论。

- [研究报告：OpenViking 团队记忆与 Skills 边界](/home/wangjunbo/.cache/ai-native-wayfinder-worktrees/memory-20261009/docs/research/2026-10-09-openviking-team-memory.md)
- 研究提交：`3c6b1740f63cb99a34e3f8788eb04b0dda2cb0ae`，分支 `research/ai-native-team-memory-20261009`。
- 可恢复内容：该提交下 `docs/research/2026-10-09-openviking-team-memory.md`。
- 证据范围：官方文档与固定源码；报告列出的八个产品验收场景尚未执行。配置仓库检查通过不代表产品隔离或共享验收通过。
