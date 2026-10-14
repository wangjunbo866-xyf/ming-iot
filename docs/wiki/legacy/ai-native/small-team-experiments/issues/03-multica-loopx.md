# Multica 的协作价值与 LoopX 的推进职责是否重叠？

Type: research
Label: wayfinder:research
Status: resolved
Assignee: coordination_research
Parent: [小团队运行方案与 IoT 试点决策地图](../map.md)
Blocked by:

## Question

核对 Multica 当前官方源码、文档、许可证与部署要求，分析 Agent 分工、任务与工作区、评审与权限、状态推进；与现有 LoopX 的长程目标/任务/运行职责对照。说明是否可由一个负责团队分配、另一个负责单项任务执行而避免双重调度，并检查实际集成证据。区分已验证接口与架构建议，给出采用和暂缓的成本条件。

## Context

- 官方候选来源：`https://github.com/multica-ai/multica`。
- LoopX 本地源码供只读核对：`/home/wangjunbo/workspace-skills/loopx`。
- 保留用户已采用的 LoopX；本票据不授权换用另一套长程任务权威。
- 研究分支：`research/ai-native-team-coordination-20261009`。

## Answer

2026-10-09 完成官方资料与固定源码调查。Multica 与 LoopX 均有运行协调能力，组合时须明确同一工作的唤醒、重试、停止和写回职责；未发现现成的两者 runtime 适配。Multica 已有 Issue/Run 区分、Agent/Squad 与 Skills 副本，未发现 Gitee 原生 PR/CI 适配。当前许可证是带额外条件的 source-available 许可。采用或暂缓仍是团队选择。

- [研究报告：Multica 与 LoopX 的协作边界](/home/wangjunbo/.cache/ai-native-wayfinder-worktrees/coordination-20261009/docs/research/2026-10-09-multica-loopx-coordination.md)
- 研究提交：`bf75f133b79e84f45061075a09bbca419e2356d6`，分支 `research/ai-native-team-coordination-20261009`。
- 可恢复内容：该提交下 `docs/research/2026-10-09-multica-loopx-coordination.md`。
- 证据范围：官方文档与源码，未部署、未运行 Gitee/LoopX 端到端集成。仓库现有检查通过仅说明研究文档未破坏该配置仓库。
