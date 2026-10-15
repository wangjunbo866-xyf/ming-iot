# LoopX Phase B 引导切换与资格验收设计

> 日期：2026-08-23
>
> 状态：已批准，待编写实施计划
>
> 上位规格：
> - `2026-08-21-loopx-driven-iot-development-control-plane-design.md`
> - `2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md`
> - `2026-08-23-ming-iot-rd-docs-archive-and-legacy-graph-retirement-design.md`
>
> 替代：归档的 `2026-08-23-loopx-personal-framework-bootstrap.md` 实施草案。归档草案只用于追溯，不得执行其中命令。

## 1. 决策摘要

Phase A 与 Gate B 已完成：研发文档、旧 Graph、未推送提交分类和完整 Git Bundle 已进入本地 `ming-iot-rd-docs`，且本地仓库没有 remote、pre-push 会拒绝推送。

Phase B 只负责完成 Gate C：

1. 将本机 LoopX 从缺少来源清单的普通 Python distribution 切换为官方 `stable` 通道的可追溯 release snapshot；
2. 在 `ming-iot` 建立短而稳定的静态 Harness；
3. 连接唯一长期 Goal `ming-iot-framework-v1`；
4. 注册三个同级 `peer_v1` 身份并完成 Worker、Reviewer、恢复和用户 Gate 演练；
5. 原子切换 `AGENTS.md` 与 pre-push，使新工作不再依赖 graphctl；
6. 证明临时移除 `.graph` 与 `docs/superpowers` 后，新流程和产品构建仍可工作。

Phase B 不删除旧 Graph、不重写 Git 历史、不 force-push。Gate C 通过后，Phase C 才从当前树退役旧 Graph；Phase D 再执行历史净化预演。最终 Gitee 强推仍保留为独立 Gate F，不能由本规格或此前的概括授权继承。

## 2. 当前事实基线

### 2.1 已完成的 Phase A

- `ming-iot-rd-docs` 根提交：`6e0eb1e25285997e0c07c3b0eaa3805097dbefd9`；
- 归档清单包含 49 个来源条目并通过全量 SHA-256 校验；
- 切换前 Git Bundle 已通过 `git bundle verify` 与 `git fsck`；
- `ming-iot` 当前 `master` 为 `403b9a6d6b7e2c60a1ff96ef0f681da272d04c36`，相对 `origin/master` 超前 7 个提交；
- 7 个提交已分类为 `ARCHIVE_ONLY`、`REPLAY_CODE` 或 `SPLIT_REPLAY`，不存在未分类提交。

### 2.2 当前 LoopX 安装

2026-08-23 的只读核对结果：

- 当前命令：`/root/anaconda3/bin/loopx`；
- 当前版本：`0.5.2`；
- `loopx doctor` 的必需检查通过；
- 安装类型是 `python_distribution`；
- 没有 release manifest、source commit 或 release snapshot lineage；
- `loopx update --check` 判断版本号与官方 `stable` 相同，但仍要求 `activation_qualification`；
- 官方 `stable`、Tag `v0.5.2` 指向 commit `423035f402e2f1703f076c3cfe60c14c5803433f`；
- `ming-iot` 尚未连接 LoopX，项目内不存在 `.loopx/registry.json`，全局 Goal 数为 0。

版本号相同不等于运行来源已证明。Phase B 必须安装并读回官方 release snapshot，不能把当前 PyPI 安装直接声明为合格。

### 2.3 当前旧 Graph 依赖

当前 `ming-iot` 仍存在：

- `.graph/**`；
- `backend/tools/cmd/graphctl/**`；
- `backend/tools/internal/graphctl/**`；
- 将 `.graph` 声明为唯一动态事实源的 `AGENTS.md`；
- 调用 `graphctl ci` 的 `.githooks/pre-push`；
- `backend/tools/README.md` 中的 graphctl 生命周期说明。

因此 Phase B 必须以原子切换结束，不能只连接 LoopX 而保留 Graph 日常入口，也不能先删除 Graph 再连接 LoopX。

## 3. 方案选择

### 3.1 采用方案：分阶段切换

```text
Phase A / Gate B：本地归档完成
  -> Phase B：LoopX Bootstrap 与 Gate C
  -> Phase C：旧 Graph 当前树退役与 Gate D
  -> Phase D：历史净化预演与 Gate E
  -> 独立 Gate F：精确 refs 强推
  -> Gate G：远端读回与团队切换
```

Phase B 内部采用“两段式引导”：

```text
最小静态种子
  -> 安装并验证 LoopX release snapshot
  -> dry-run 后连接唯一 Goal
  -> 注册同级 peers
  -> LoopX 管理剩余 Harness 改造
  -> 独立 Reviewer
  -> 原子 cutover
  -> 重启与无 transcript 恢复演练
  -> Gate C 验收
```

这样既让 LoopX 尽早管理自己的迁移，又保留旧 Graph 作为 Gate C 前的可回退基线。

### 3.2 未采用方案

- **一次性合并切换和删除**：安装、Goal、Harness、Graph 删除和 Hook 变化同时发生，回退面过大。
- **先删除 Graph 再连接 LoopX**：产生无动态控制面的真空期，违反 Gate 顺序。
- **长期双写 LoopX 与 Graph**：重新形成两个 Goal/Todo/Gate 真源。cutover 前 Graph 只作回退基线，不接收新状态。

## 4. 权威边界

| 事实 | Phase B 后的唯一所有者 | 位置 |
|---|---|---|
| IoT 架构与语义 | `iot-doc` | 已提交的正式架构正文 |
| 动态 Goal、Todo、Gate、claim、lease、quota、运行时 Evidence | LoopX | 被忽略的 `.loopx/`、`.codex/goals/` 与 Host runtime |
| 产品代码与稳定静态合同 | Git | `ming-iot` |
| 研发规格、计划、里程碑 Evidence、独立 Review、Proposal | 本地研发文档库 | `ming-iot-rd-docs` |
| 确定性检查 | 项目 verifier | `ming-iot/automation/loopx/` 与既有测试工具 |
| 合并、push、历史强推和敏感副作用 | 用户 | 精确 Gate 与 readback |
| 旧 Graph 历史 | 本地归档 | `ming-iot-rd-docs/archives/` |

LoopX 运行时 Evidence 与本地里程碑 Evidence 不是双真源：前者驱动当前 Todo；后者只保存能够复核 Gate C 结论的压缩快照、命令、digest 和人工判定。

## 5. Phase B 资产边界

### 5.1 提交到 `ming-iot` 的稳定资产

```text
ming-iot/
  AGENTS.md
  .gitignore
  .githooks/pre-push
  automation/loopx/
    README.md
    release-lock.json
    project-contract.json
    authority-catalog.json
    role-contracts.json
    multi-agent-spec.public.json
    mcp-policy.md
    rules/repository-policy.md
    skills/planner/SKILL.md
    skills/worker/SKILL.md
    skills/reviewer/SKILL.md
    check.py
    test_check.py
```

- `AGENTS.md`：当前权威入口、安全边界和最短开始/完成步骤；不复制动态 Todo；
- `release-lock.json`：官方通道、版本、source ref/commit、release id/manifest digest；
- `project-contract.json`：稳定 Goal ID、目标、仓库身份、允许领域和运行时忽略路径；
- `authority-catalog.json`：用逻辑 repository/path/revision 描述 `iot-doc`，不写个人绝对路径；
- `role-contracts.json`：声明 `peer_v1`、临时角色、风险拓扑、write scope 和用户 Gate；
- `multi-agent-spec.public.json`：只声明三个可见 lane、同级 agent ID、临时 role、scope 和 handoff hints；不复制 tmux、quota 或调度实现；
- `mcp-policy.md`：外部能力默认只读、最小权限，生产写入必须等待用户 Gate；
- `repository-policy.md`：worktree、参考仓库只读、commit/push、失败关闭和恢复规则；
- `skills/*/SKILL.md`：仅描述当前临时角色如何决策、提交 Evidence 和交接，不复制 LoopX 通用项目工作流；
- `check.py`：仅做快速、确定性、无网络的静态合同检查；
- `test_check.py`：覆盖缺失忽略项、绝对路径、旧 Graph 动态引用、角色越权和非法 Evidence 路径等负例。

`check.py` 不复制 LoopX 状态机，不判断 Todo 是否完成，不读取 `.loopx` 私有正文，也不替代业务测试。

### 5.2 只保存在本地研发文档库的资产

```text
ming-iot-rd-docs/
  specs/2026-08-23-loopx-phase-b-bootstrap-cutover-design.md
  plans/2026-08-23-loopx-phase-b-bootstrap-cutover.md
  evidence/2026-08-23-loopx-phase-b-bootstrap-cutover/
  reviews/2026-08-23-loopx-phase-b-bootstrap-cutover-review.md
  proposals/2026-08-23-first-real-delivery-loop.md
```

不得将这些文件重新写回 `ming-iot/docs/superpowers/**`。

### 5.3 必须忽略的运行时资产

```gitignore
/.loopx/
/.codex/goals/
/.codex/skills/
/.local/
/.opencode/goals/
```

此外，`automation/loopx/__pycache__/` 作为 Harness 本地瞬态缓存必须被忽略，且只允许保存
Git ignored 且 untracked 的可再生 `.pyc`；任何其它未知 Harness 资产、未忽略缓存或强制暂存
缓存都必须失败关闭。原始 transcript、私有 session、Token、凭据、绝对本机映射和大体积运行
日志不得进入公开提交。

## 6. LoopX 发布与连接合同

### 6.1 发布来源

Phase B 使用官方 no-clone installer 的 `stable` 通道：

1. 先执行 `loopx update --check` 与 `loopx update --dry-run`；
2. 记录目标 repo、ref、版本和 commit；
3. 通过官方 installer 或 `loopx update --execute` 安装 release snapshot；
4. 使用安装后的明确命令路径重新执行 `loopx doctor`；
5. 证明 `default_command_is_release_snapshot=true`、release manifest 可用且 source commit 与目标一致；
6. 将公开安全的 release identity 写入 `release-lock.json`，完整诊断只保存为本地 Evidence；
7. 保留可验证 rollback target；首次迁移没有自动 rollback 时记录恢复旧命令的显式步骤。

若实施时 `stable` 不再指向 `423035f402e2f1703f076c3cfe60c14c5803433f`，必须停止并更新计划，不能静默升级。

### 6.2 唯一长期 Goal

```text
goal_id: ming-iot-framework-v1
objective: 按 iot-doc 的架构与语义，实现完整、可验证、可持续演进的工业物联网平台 ming-iot。
```

连接必须先 dry-run，再实际执行；禁止接受自动生成的业务 Todo，禁止在连接轮次启动长任务。连接后必须读回：

- `.loopx/registry.json` 存在且被 Git 忽略；
- `.codex/goals/ming-iot-framework-v1/ACTIVE_GOAL_STATE.md` 存在且被 Git 忽略；
- 全局 Registry 只包含一个同 ID route；
- `loopx status --goal-id ming-iot-framework-v1` 返回同一 objective；
- 当前 user gate、agent todo 和 next safe action 可解释。

连接后应把用户在 2026-08-23 对分阶段方案 A 的批准记录为有界的 Phase B/C/D 顺序 Gate。该批准允许继续准备和实施相应阶段，但不替代仓库规定的 checkpoint commit 授权、merge/push 授权，也不替代 Gate F 对精确 old/new refs 的单独批准。

### 6.3 Agent 身份

注册三个中性、同级身份：

```text
ming-peer-01
ming-peer-02
ming-peer-03
```

不得把 `planner`、`worker`、`reviewer` 写进 durable identity 并据此推导权限。角色由当前 Todo 临时授予：

- Planner：只读提出边界和 Todo 图；
- Worker：只在获批 repository/worktree/write scope 中修改和验证；
- Reviewer：使用不同 identity、fresh context，只读复核并能拒绝 Evidence；
- 临时 task coordinator：只协调当前 bundle，不形成永久 Leader。

注册必须先 preview，再使用 `--require-new --execute`；重复 ID 或已绑定旧线程时失败关闭。

`multi-agent-spec.public.json` 先通过 `loopx multi-agent launch --spec ...` dry-run；只有 Goal、peer、scope 和 pane 启动命令均正确时才允许 `--execute --auto-wake --attach`。Runner 只负责可见 Host 生命周期，每个 pane 仍必须独立读取 LoopX quota/Todo；它不获得动态事实写入或 push 权限。

## 7. 静态 Harness 与原子 Cutover

### 7.1 最小种子

LoopX 连接前只准备：

- 运行时忽略项；
- `project-contract.json`；
- release 候选锁；
- Phase B 隔离 worktree；
- 不执行长任务的连接说明。

其余 Rules、角色合同、verifier、AGENTS 与 Hook 切换由连接后的 LoopX Todo 管理，让 LoopX 在 Phase B 内真正承担长任务恢复。

### 7.2 原子切换内容

同一个 cutover checkpoint 必须完成：

- `AGENTS.md` 改为 LoopX + 静态 Harness 入口；
- `.githooks/pre-push` 不再调用 graphctl，只运行快速静态检查和必要的变更相关门禁；
- `.gitignore` 完整忽略 LoopX/Host 本地运行时；
- `backend/tools/README.md` 将 graphctl 标为待 Phase C 删除的历史实现；
- 新 Evidence、Review、Proposal 使用本地研发文档库的逻辑 artifact ID；
- 新任务不再写 `.graph` 或 `docs/superpowers`。

旧 `.graph` 和 graphctl 源码在 Phase B 后仍可存在于当前树，但只作回退材料，不参与日常状态、Hook 或完成判断。

### 7.3 快速门禁预算

日常 `pre-push` 的静态 Harness 目标为热缓存下 10 秒以内；不得常态串行运行所有历史 Loop 的全量测试。完整业务验证由受影响 Todo、风险等级和显式 pre-merge Gate 触发。

若静态检查超时或无法确定范围，应失败关闭并给出手工完整验证命令，不得静默跳过。

## 8. 资格验收场景

### 8.1 Worker 与 write scope

- Worker 使用 `ming-peer-01`；
- 在独立 worktree 领取一个非业务 Harness Todo；
- 只能修改声明的 `automation/loopx/**`；
- 越界修改必须被 verifier 或 Review 拒绝；
- 产出确定性测试结果和可恢复 checkpoint。

### 8.2 独立 Reviewer

- Reviewer 使用 `ming-peer-02` 和 fresh context；
- 只读当前 authority、Todo、exact revision、diff 和 Evidence；
- 不依赖 Worker chat/transcript；
- 至少演练一次拒绝“不完整 Evidence”，修复后再通过；
- Reviewer 不修改被审实现，也不能批准 push。

### 8.3 Planner 与提案 Gate

- Planner 使用 `ming-peer-03`；
- 对第一个真实 IoT 交付 Loop 最多提出三个排序候选；
- 只生成 Proposal 与用户 Gate，不创建实现 Todo、不创建写 worktree；
- Proposal 保存到本地研发文档库，LoopX 只保存逻辑 artifact ID、digest 和 Gate 状态。

### 8.4 恢复与 Chat 切换

至少证明：

1. 停止当前 Agent/Chat 后，新 Chat 可通过精确 `goal_id` 恢复；
2. 新 Chat 不读取旧 transcript，只读 LoopX、Git、worktree、静态合同和 Evidence 引用；
3. Worker 转 Reviewer 时使用 fresh identity/context；
4. 有 checkpoint commit 时可继续；仅有未提交修改时产生 `recovery_required`，不得自动删除；
5. 同一 Todo 可有界复用 Chat，Todo 或职责变化时换新。

### 8.5 临时删除验证

在一次性临时 worktree 或临时候选树中移除：

```text
.graph/**
docs/superpowers/**
backend/tools/cmd/graphctl/**
backend/tools/internal/graphctl/**
```

验证 LoopX 状态、静态 Harness、`thingsbaseline`、Go 单元测试、Race、Vet 与适用构建不依赖被删控制面，且当前入口不再引用 `.graph/runtime` 或 `docs/superpowers`。该验证只证明 Phase C 可进入，不在 Phase B 主分支执行永久删除。

## 9. Evidence 与 Review

Phase B Evidence 至少记录：

- 原始与目标 LoopX 安装身份；
- `update --check`、dry-run、安装和 `doctor` readback；
- Goal 连接 dry-run、实际写入与 Registry readback；
- 三个 peer 注册与上下文边界；
- Worker revision、write scope、命令与测试结果；
- Reviewer 首次拒绝、修复和最终 verdict；
- 重启/fresh Chat 恢复包；
- cutover diff、快速门禁耗时与临时删除验证；
- 未解决风险和 Phase C handoff。

Evidence 必须压缩、公开安全且可复核；不复制完整 doctor JSON、原始 transcript 或大体积日志，只保存摘要与受控本地 artifact 引用。

独立 Review 必须由不同 Agent identity 基于 exact revision 执行，并检查：

- 是否仍有双真源；
- 是否把本地文档路径硬编码进公开文件；
- 是否将角色名错误地当作持久权限；
- 是否由 Worker 自审代替独立 Review；
- 是否把静态 Harness 扩展成第二套动态控制面；
- 是否错误声明 `.graph` 已删除或历史已净化。

## 10. 回退

Gate C 前保留 Phase A Git Bundle、原本地分支和 worktree、旧 AGENTS/Hook/Graph revision、LoopX 安装前诊断与 Phase B 隔离 worktree。

资格验收失败时：

1. 停止创建新的 LoopX Todo；
2. 保留 `.loopx` 和 `.codex/goals` 用于诊断，不提交、不删除；
3. 不执行 cutover，或通过受控回退恢复旧 AGENTS/Hook；
4. 保留失败 Evidence，创建 repair Todo；
5. 从 release、Goal、peer、Harness、恢复、Review 全链路重新验收。

禁止在失败时同时继续写 Graph 和 LoopX 以“先跑起来”。

## 11. 非目标

- 不实现 IoT 业务功能；
- 不删除 `.graph`、graphctl 或 `docs/superpowers`；
- 不修改 `things` 官方只读仓库；
- 不 fork LoopX、不开发 `company-dev`、不自研 Runner；
- 不把 Headless Turn 设为默认执行面；
- 不建立新 Dashboard；
- 不自动 merge、push、部署或使用生产凭据；
- 不给本地研发文档仓库配置 remote；
- 不把 Phase B 合格声明为个人研发框架第一阶段完成。

Phase B 最终状态只能表述为 `BOOTSTRAP_QUALIFIED / GATE_C_ACCEPTED`。第一阶段完成仍要求至少一个真实 IoT 交付 Loop 闭环。

## 12. Gate C 完成定义

以下条件必须同时满足：

1. 官方 `stable` release snapshot、manifest/source commit 和 doctor readback 合格；
2. `ming-iot-framework-v1` 是唯一项目 Goal，项目与全局 Registry 一致；
3. `.loopx/`、`.codex/goals/`、`.local/` 等运行时路径被忽略且未跟踪；
4. 三个中性 `peer_v1` 身份注册成功，角色权限只来自 Todo；
5. Worker 只能在获批 worktree/write scope 中实现并产出 Evidence；
6. 独立 Reviewer 使用不同 identity/fresh context，演练过拒绝与通过；
7. fresh Chat 可不依赖 transcript 恢复 Goal、Todo、worktree 和下一安全动作；
8. `AGENTS.md` 与 Hook 已原子切换，不再依赖 graphctl；
9. 新任务不再写 `.graph` 或 `docs/superpowers`；
10. 静态 Harness 在预算内通过，适用业务校验器仍可独立运行；
11. 临时删除验证证明 Phase C 可以安全开始；
12. 本地 Evidence 与独立 Review 绑定 exact revision 且通过；
13. 未执行旧 Graph 永久删除、历史重写或 push；
14. 形成第一个真实 IoT 交付 Loop 的排序 Proposal，但未越过用户业务范围 Gate；
15. Phase C handoff 明确列出待删除路径、保留 verifier、验证命令和回退点。
