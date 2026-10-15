# Gitee 承载团队共享交付事实，LoopX 承载本地执行投影

- 状态：已接受
- 日期：2026-09-03

## 决策

Ming IoT 使用 Gitee Issue、PR 和 Git 提交作为跨成员、跨电脑的共享交付事实。一个可独立验收的交付结果使用一个 Delivery Issue 和一个主要代码 PR；实施计划中的 Task 不逐项创建 Issue。

LoopX 继续负责本地长程执行、Agent Todo、上下文恢复、Evidence 和 User Gate 投影，但不作为团队唯一的派单数据库。新电脑或新成员应先从 Gitee 和受版本控制的仓库恢复共享上下文，再把当前工作投影到本地 LoopX。

## 原因

LoopX 的本地 `.loopx/`、`.codex/` 与运行 Evidence 不进入共享 Git。若把它们当作唯一任务事实，其他成员无法仅靠克隆仓库恢复工作；若完全不用 LoopX，则会丢失本地跨轮次执行和 Agent 编排能力。两层各自承担其擅长的职责。

## 后果

- Issue/PR 生命周期、依赖关系和负责人在 Gitee 可见。
- LoopX Todo 必须引用对应 Delivery Issue，而不能创造另一套团队编号。
- 本地 LoopX 状态丢失时可以从 Gitee 与 Git 仓库重建，但原始本地执行轨迹不保证完整恢复。
