# ming-iot 协作入口

## 仓库地图与快速命令

- 业务代码：`backend/saas`、`backend/share`、`backend/things`；Core/Share/Things 已并入，不再寻找同名兄弟仓实现。
- 规划阶段、调整目标或判断 Goal 是否完成时，先读 [AI 协作研发总纲](docs/project-charter.md)；旧阶段计划只说明原时点，执行当前 Todo 前核对其与总纲的范围。
- 查规格或变更时，从 [知识导航](docs/README.md) 进入模块当前正文；查历史时按 [来源表](docs/index.yaml) 区分适用版本。
- 本地入口：`make knowledge-check`、`make verify-pr`；具体边界见 [工具说明](tooling/README.md) 和 [部署说明](deploy/README.md)。
- 当前状态：迁移与日常入口切换已完成，同一 Goal 注册迁移及首次真实定时接续已验证；ThingsBoard/AirMQ 为仓外学习参考，不迁入；r-doc 与旧分发工具未采纳。验收边界见 [M5 记录](docs/changelog/2026-10-15-m5-cutover.md)。
- 当前 Goal 为 `ming-iot-knowledge-pilot`。LoopX 从 `tooling/loopx` 进入；同一 SQLite Goal 的注册及状态目录已迁入本仓，共享运行时和历史保留原位。维护或恢复前读取 [日常开发入口](docs/runbook/daily-development.md)，复用现有 Goal 与已审运行包。

## 关键约定速查

| 场景 | 约定 |
| --- | --- |
| 需求与规格 | 按模块当前有效正文及已确认决定执行；历史材料不自动成为规则 |
| 代码检查 | 完整 verify-pr 与独立复审绑定同一候选，真实依赖未运行则明确记录 |
| 迁移 | 已授权 M5、PR #1 合并与入口切换；旧源和备份保留，删除与生产发布不在范围内 |


## 开始修改前

1. 读取当前状态：`tooling/loopx status --goal-id ming-iot-knowledge-pilot`。
2. 读取当前 LoopX Todo、写范围和用户 Gate，不依赖旧聊天推断任务。
3. 旧工作区 `iot-doc`、`things` 来源仓保持只读；当前产品实现位于本仓 `backend/things`，可按获批任务修改。

## 持久资料登记

- 需要跨 Chat 保留的设计、Proposal、研究资料、SOP、Evidence 或外部资料必须使用 `loopx-doc-registry`。
- 固定顺序是：先在项目权威表面建立可持久读取的正文映射并验证正文可读，再 dry-run，通过 LoopX 正式登记来源（由 LoopX 保存脱敏指纹），执行 `refresh-state`，最后回读状态。
- 正文与项目文档目录不是 `.loopx/registry.json`；不得手工修改该 Registry 完成登记。
- Git 跟踪的项目文件在独立 worktree 修改；`.loopx/` 与 `.codex/` 是本机私有状态，只能由 LoopX 在已注册项目根维护，worktree 不创建第二份。
- 正文无法定位、目标 Goal 不明确或读取权限不明确时，停止登记并写入 Todo/Blocker。
- `source_ref_sha256` 是 raw locator 的脱敏摘要，不是文件定位器、正文内容哈希，也不是正式登记命令的 source 输入；没有可读取文档映射的记录不是完整登记。Authority 或文档目录变化后必须运行项目侧聚焦校验。

## 修改与完成

- 所有 Git 跟踪文件修改必须绑定当前 Todo，并在独立 worktree 内且符合获批的 write scope。
- `.loopx/`、`.codex/` 私有状态变更同样必须绑定当前 Todo 和获批边界，并只从已注册项目根通过 LoopX 执行。
- Agent Todo 按 Loop、阶段成果或需要独立交接的拓扑节点登记；实施计划中的 Task/Step 只记录在 Plan 与 Evidence 中，不得默认逐项转成 Agent Todo。只有跨仓库、跨负责人或独立延期交接时才拆分新的 Agent Todo。
- 普通 Agent Todo 在获批范围内完成验证及所需独立 Review 后，自行闭环并接续已授权工作；Plan Task/Step、checkpoint commit 和 Todo 完成本身均不触发 User Gate。只有新方向、范围变更、风险接受或下文的高风险授权边界需要用户决策时才建立 Gate；已有精确 Gate 仍须满足。
- 实施 → Review → 接续：实施者提供不可变候选 Head、验证结果和精确交付物；独立 Reviewer 只写获准的 Review/Evidence，不顺手修改实施代码。接续 Todo 以 Review Todo 完成及同一 Head 的有效回执为前置条件；启动 Reviewer、声明完成或仅有 PASS 字样都不能替代回读。候选变更后重新验证并提交受影响范围的 Review。
- Todo/阶段完成不等于 Goal 验收完成。阶段结束核对剩余验收、后继 Todo 与授权边界，更新当前 Agent Vision；仅在真实终止或明确暂停当前 lane 时使用 `no_followup`，并说明恢复条件。等待 Review 不得关闭 Reviewer 的可运行 lane。
- 宿主续跑单独核验：LoopX 注册和 `should_run=true` 只是准入，不证明 Codex `/goal` 或 Claude `/loop` 已运行；从对应宿主读取激活状态，同一身份不得重复拉起。缺少宿主控制能力时报告一次精确恢复动作，不用忙轮询冒充续跑。
- 完成前运行与变更相关的项目测试和静态检查。
- LoopX 状态不替代 Git diff、测试结果或用户检查。

## 交付验证与 PR

- 代码变更从仓库根 `Makefile` 选择 Verification Profile；以 `make help` 的当前输出为入口说明，并按 `deploy/README.md` 的变更面选择规则运行最低档位。
- 普通代码 PR 必须运行 `make verify-pr`。需要真实 Kafka 或 Things 依赖时另跑显式 `verify-integration` suite；发布候选才运行带重型确认的 `verify-release`。重型验证结论不授予发布或生产权限。
- PR 关联需求/变更入口、本仓规格适用版本、完整 40 位候选 Head、实际 Profile 结论和同一 Head 的独立 Review。GitHub PR 模板与单仓 CI 已在本地准备；提交前按 [CI 说明](docs/runbook/monorepo-ci.md) 运行完整检查。公开前核对发布清单；HOLD 状态不可发布。`.gitee/` 保留历史，远程 CI 以关联 PR 的实际运行结果为准；本地检查不代替托管运行。
- 未运行项以非 PASS 结论记录原因和补验证方式；既有失败保留真实状态并关联独立缺陷 Issue，不通过缩小或绕过 Profile 获得绿色。

## LoopX 上下文与 Evidence 热路径

- 日常 Turn 先读取 `loopx diagnose/status/quota`、当前选中 Todo 与有界的 `evidence-log --thin --todo-id ...`；不得默认将整份 `ACTIVE_GOAL_STATE.md` 加载为上下文。只有诊断状态漂移、修复投影或执行获批的状态迁移时才读取全文。
- Todo `evidence`、`note` 和 `reason` 只保存公开安全的简短结果或稳定证据指针，不内联完整 Review、原始日志、命令输出或大段分析正文。
- 完整、脱敏、供人阅读的 Review 写入本仓任务证据或 `docs/changelog/`；原始机器 Evidence 留在当前 Goal 的私有证据目录。旧 `docs/wiki/legacy/rd/reviews/` 仅保留历史。Todo 回执只保留 `verdict`、`exact_head`、`review_ref/evidence_ref`、摘要哈希与 blocker/warning 计数。

## 长期 Smoke、Monitor 与自修复

- 只长期保留能验证已发布行为、可复用控制面契约、安全边界、真实回归或代表性端到端路径的 Smoke；仅证明一次临时分析、候选排序或过渡结论的检查留在当次 Evidence，不扩大永久测试面。
- 项目特定的周期观察登记为 LoopX `continuous_monitor` Todo，并保存稳定目标、调度信息和简短 Evidence；宿主唤醒 Prompt 保持通用，不写死某个 PR、迁移或临时任务。
- `continuous_monitor` 只负责观察和报告状态变化，不自动授予代码修改、合并、发布或生产操作权限；发生实质变化时再创建或唤醒普通 Agent Todo。
- 出现 LoopX 投影矛盾、进展过小、状态漂移或用户指出流程错误时，暂停普通交付并执行 `loopx-self-repair`；修复落到最低且可持续的责任层，例如 Todo、边界投影、交互规则、项目文档或聚焦 Smoke，不依赖聊天记忆或降低 Gate。

## Git 与高风险授权边界

以下规则在 `ming-iot` 内覆盖上级工作区对每次 commit、push 单独授权的要求：

- 已绑定获批 Agent Todo、精确 `task_repository`、write scope 和独立 worktree 的工作，Agent 可在验证和公共/私有边界扫描通过后自行 commit。只有当前任务允许远程交付时才推送候选分支或创建/更新 PR；既有发布回执只覆盖当时的候选，当前操作范围以本次用户指令和 Todo 为准；公开边界见 `tooling/policy/PUBLICATION_POLICY.json`。
- 所有跟踪文件变更通过 PR 进入默认分支；禁止直接 push 默认分支的产品变更。首次空仓可建立不含文件的根提交作为 PR 基线。
- PR 默认由有权限的人在目标托管平台完成 Review 和合并；人执行合并本身即为该次合并授权，不再要求单独的 LoopX User Gate。
- 只有要求 Agent 执行 PR 合并时，才建立绑定精确 PR、Head、目标分支和合并方式的 User Gate。
- 版本发布、生产环境变更、凭据处理和权限变更必须建立绑定精确目标与动作的 User Gate。
- 禁止重写受保护分支历史；重写已发布的共享功能分支必须建立精确 User Gate。
