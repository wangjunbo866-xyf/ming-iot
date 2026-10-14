# LoopX 官方 AGENTS 协作策略固定版本参考

> 文档性质：外部项目协作策略的固定版本参考，不是 `ming-iot` 当前实施规格，也不自动把 LoopX 的全部仓库规则复制到本项目。
>
> 上游项目：[huangruiteng/loopx](https://github.com/huangruiteng/loopx)
>
> 上游文件：`AGENTS.md`
>
> 固定 revision：`88c45890f7d35fce38b0534d16429711f6850cab`
>
> 固定来源：[该 revision 的 AGENTS.md](https://github.com/huangruiteng/loopx/blob/88c45890f7d35fce38b0534d16429711f6850cab/AGENTS.md)
>
> 原始内容 SHA-256：`bf466114cf5d3fde70f4fa336aceb527746b827ef193b4aa146cc5f2629044f6`
>
> 本地登记日期：2026-09-02
>
> 新鲜度复核：登记时上游 `main` 为 `1c21f3c5443c447e28a1dd367c52b1173054ad67`，其 `AGENTS.md` 内容哈希与本固定 revision 相同。
>
> 冲突规则：`ming-iot` 用户批准的当前规格、项目 `AGENTS.md`、LoopX 当前 Goal/Todo/Gate 与仓库保护策略优先；本参考用于解释规则来源和后续差异审计。

## 为什么固定 revision

直接引用上游 `main` 只能说明“现在看到什么”，不能证明当时依据什么作出项目决策。固定 revision 和内容哈希共同提供可重放基线：未来上游规则变化时，可以明确区分“原决定”“上游新变化”和“本项目本地化选择”，避免 Agent 把新旧规则混在一起。

## 本项目吸收的上游原则

### 1. Worktree 与 PR 是正常交付路径

非琐碎的跟踪文件修改应在干净的独立 worktree 和候选分支中完成；进入主分支前通过 PR Review。主工作区中的脏改动不能被顺手混入候选变更，暂存时使用精确 pathspec，不使用 `git add .`。

### 2. 常规候选分支操作不逐次请求用户授权

在 Agent 已绑定获批 Todo、精确 `task_repository`、write scope 和独立 worktree，且验证与边界扫描通过后，可以自行 commit、推送候选分支并创建或更新 PR。授权对象是已经限定的交付边界，不是对任意仓库写操作的永久许可。

### 3. Monitor 作为状态，而不是写死在唤醒 Prompt 中

项目特定的周期观察写成 LoopX `continuous_monitor` Todo，并保存稳定目标、调度信息和简短 Evidence。宿主唤醒逻辑保持通用，通过 status、quota 和 Todo 投影发现到期工作；不为某个 PR 或某次迁移定制永久心跳 Prompt。

### 4. Smoke 只保留长期有效的机器行为

保留能验证已发布行为、可复用控制面契约、安全边界、真实回归或代表性端到端路径的 Smoke。仅用于证明某次临时分析、候选排序、运行复盘或过渡结论的测试不应永久进入活跃测试面。

### 5. 自修复必须留下可持续约束

遇到 LoopX 行为矛盾、进展过小、投影漂移或用户指出流程错误时，先暂停普通交付并运行自修复。修复应落到最低且可持续的责任层，例如 Todo、边界投影、交互规则、项目文档或聚焦 Smoke；不能通过降低 Gate 或依赖聊天记忆解决。

### 6. 公共/私有边界先于发布便利

候选提交不得包含凭据、原始模型对话、私有运行日志、机器绝对路径、临时 Evidence 或与交付无关的内部叙事。即使 `ming-iot` 与 `ming-iot-rd-docs` 是私有仓库，也继续执行这条边界，避免把本地运行状态误当成可协作的项目事实。

## `ming-iot` 的本地化差异

- Git 托管使用 Gitee 私有仓库；PR 默认由有权限的人在 Gitee 完成 Review 和合并。
- 人在 Gitee 点击合并本身就是该次合并授权；只有要求 Agent 执行合并时，才建立绑定精确 PR、Head、Base 和合并方式的 User Gate。
- 版本发布、生产环境变更、凭据处理和权限变更仍需绑定精确目标与动作的 User Gate。
- 禁止重写受保护分支历史；重写已发布的共享功能分支需精确 User Gate。
- LoopX 上游的 DCO、GitHub Review 评论、GitHub Release 以及其基准测试专属规则不自动移植；只有项目明确启用对应流程后才适用。
- 现有 Contract/Evidence、Authority Source、Goal/Todo/Gate 和独立 Review 规则继续有效；本次对齐不替换这些项目事实层。

## 后续复核方法

当上游 `AGENTS.md` 内容哈希变化时：

1. 以本文件记录的固定 revision 为基线做差异分析；
2. 只提取与 `ming-iot` 当前流程有关的稳定原则；
3. 把采纳、拒绝或暂缓的结论写入新的 Proposal、Spec 或 Review；
4. 经项目决策后再更新 `ming-iot/AGENTS.md`，不得直接用上游文件覆盖。
