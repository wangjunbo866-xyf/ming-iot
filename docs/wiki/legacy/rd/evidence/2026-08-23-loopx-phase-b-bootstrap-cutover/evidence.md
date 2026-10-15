# LoopX Phase B 引导切换 Evidence

> 日期：2026-08-23
>
> Goal：`ming-iot-framework-v1`
>
> 结论：`IMPLEMENTATION_APPROVED / GATE_C_BLOCKED`
>
> 唯一 Gate blocker：`checkpoint_commit_authorization_required`

本文只保留可复核摘要、digest、命令和判定，不复制原始 Agent transcript、完整 doctor JSON、凭据或可再生
运行日志。

## 1. Release lineage

静态锁定并读回：

```text
channel: github_archive_stable
repository: huangruiteng/loopx
ref: stable
version: 0.5.2
sourceCommit: 423035f402e2f1703f076c3cfe60c14c5803433f
qualificationStatus: qualified
releaseId: 20260823T091615Z
manifestSha256: 71dba222713104deb19463345b9add5bb6afb728d9386370f12038b4ce406eb5
```

安装 release、manifest/source commit、项目静态锁和 doctor readback 一致。项目没有修改 LoopX 安装目录或
fork LoopX。

## 2. Goal、Agent 与权威材料

LoopX 当前只管理一个项目 Goal：`ming-iot-framework-v1`。

| Agent | 持久身份 | 本阶段职责 |
|---|---|---|
| Worker | `ming-peer-01` | 仅执行获批 Todo 与 write scope |
| Reviewer | `ming-peer-02` | fresh context 只读复核 |
| Planner | `ming-peer-03` | 只读提出有界交付提案 |

visible multi-agent dry-run 成功，Planner/Worker/Reviewer 三个 pane 曾启动；Worker 的显式
`quota should-run --runtime-profile codex_cli` 能选中当前 Todo，Planner/Reviewer 没有获得 Worker
写权限。

项目本地 `.loopx/registry.json` 是 ignored、untracked、禁止 push 的私有 Registry。已通过
`register-authority-source` 登记四个脱敏材料：

1. 总体平台架构；
2. Kafka greenfield 设计；
3. 模块级物模型/超级表设计；
4. 设备状态缓存设计。

readback 为 `topic_authority_count=4`、`project_material_count=4`，raw 本地路径未写入共享投影。

## 3. Todo、lease 与未提交快照

当前 Todo：

```text
todoId: todo_d74bd2b9737f
title: 实现并验证 Phase B 静态 Harness 与原子 cutover
owner: ming-peer-01
status: blocked
reason: checkpoint_commit_authorization_required
```

该阻塞已投影为独立、精确的用户门禁，不再依赖聊天记忆：

```text
userGateTodoId: todo_cb8a6915286b
taskClass: user_gate
decisionScope: write_scope:action:phase_b_checkpoint_commit
boundAgent: ming-peer-01
blocksAgent: ming-peer-01
unblocksTodoId: todo_d74bd2b9737f
decision: 是否明确授权创建 Phase B checkpoint commit（不含 merge、push、生产或历史重写）
```

第二轮修复后 hard lease：

```text
idempotencyKey: phase-b-bootstrap-cutover-worker-v2
version: 2
leaseEpoch: 2
reviewTimeStatus: active
finalStatus: released
```

实现和评审期间的精确 write scopes：

```text
.gitignore
automation/loopx/**
AGENTS.md
.githooks/pre-push
backend/tools/README.md
backend/tools/internal/graphctl/changes/changes_test.go
```

当前 Git 事实：

```text
branch: phase-b/loopx-bootstrap-cutover
HEAD: 403b9a6d6b7e2c60a1ff96ef0f681da272d04c36
checkpointCommit: NONE
changedFileCount: 18
worktreeScopeDigest: 3d397d3c3d80b455259b03ba737250f18aab593a4d78e79550ad3437c23c718c
trackedDiffDigest: 948a2f85ffe8804698c2564933b77e3b681b8e2263286afadd44425314a4c3ff
```

`worktreeScopeDigest` 输入为 `head <HEAD>`，随后按 Git porcelain 顺序为每个非 ignored 变更写
`file <octal-mode> <sha256> <path>`，对带末尾 LF 的完整文本计算 SHA-256。
`trackedDiffDigest` 为 `git diff --binary` 的 SHA-256。

该摘要覆盖五个 tracked 文件和 `automation/loopx/` 下十三个 untracked 静态文件。它不是 commit
identity，不得作为 checkpoint revision。

最终独立 Review 完成后，Todo 已 fail closed 为 blocked，hard lease version 2 已释放；没有继续占用可执行
Worker lane。

首次更新等待状态时，LoopX 发现 `ming-peer-01` 没有持久化的 per-agent vision baseline，投影
`vision_checkpoint_missing`。按 self-repair 合同写入最小的
`vision_blocked_on_user_gate` 基线后，readback 结果为：

```text
shouldRun: false
waitingOn: controller
interactionMode: user_gate
schedulerAction: backoff_waiting_for_user
visionCheckpointMissing: false
remainingVisionGap: vision_acceptance_gap
```

剩余 `vision_acceptance_gap` 是正常的未完成条件：只有门禁决策或快照漂移才重新规划；授权前不得发明
替代任务、commit、merge、push、删除旧 Graph 或进入 Phase C。

## 4. 机器验证

修复后的 fresh 结果：

| 命令 | 结果 |
|---|---|
| `PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s automation/loopx -p 'test_*.py' -v` | 81/81 PASS，约 3.3 秒 |
| `PYTHONDONTWRITEBYTECODE=1 python3 automation/loopx/check.py verify --repo-root . --phase cutover` | PASS |
| `git diff --check` | PASS |
| `cd backend/tools && GOWORK=off go test -count=1 ./...` | PASS，约 20.2 秒 |
| `cd backend/tools && GOWORK=off go test -race -count=1 ./internal/thingsbaseline ./cmd/thingsbaseline` | PASS，约 22.5 秒 |
| `cd backend/tools && GOWORK=off go vet ./...` | PASS |

新 `pre-push` 只运行快速、无网络的静态 cutover verifier。遗留
`TestHookForwardsExactlyOneRefRecord` 已改为 `TestHookInvokesLoopXCutoverVerifier`，正向断言
Hook 的精确 Python 调用；没有增加 graphctl fallback。

## 5. 独立 Review 闭环

### 5.1 第一次拒绝：Evidence 不充分

Worker 只给出“测试通过”。Reviewer 返回：

```text
verdict: REJECTED
reason: 缺少 exact revision、验证命令和 write-scope 证明。
```

### 5.2 第二次拒绝：发现真实 Go 回归

补齐 17 文件摘要后，fresh Reviewer 强制执行 `go test -count=1 ./...`，发现旧测试仍断言 graphctl
Hook，返回 `CHANGES_REQUIRED`。该错误没有被归类为上游降级，也没有因 Phase C 将删除 graphctl 而忽略。

### 5.3 修复后批准

Worker 先将 Todo/lease 精确扩展到冲突测试文件，再修正测试并重跑全部验证。另一个 fresh Reviewer 独立
重算 18 文件摘要、核对六项 scope 并重跑门禁，返回：

```text
implementation_verdict: APPROVED_FOR_GATE_C
gate_c_state: GATE_C_BLOCKED
findings: 无 P0/P1/P2 实现缺陷。
```

完整 Review 见
`reviews/2026-08-23-loopx-phase-b-bootstrap-cutover-review.md`。

## 6. Fresh context 与无 transcript 恢复

紧凑 handoff artifact：

```text
sha256: 5c7ec8f707f7ea37490e8689033201a0219a4fee8a24612e1b067412024ad7ac
lineCount: 12
charCount: 914
withinBudget: true
```

完全 fresh、只读的 Agent 不读取旧聊天，依靠 handoff、LoopX Todo/lease、Git 和静态合同恢复出 Goal、当前
Todo、owner、当时的五项 scope、HEAD/branch/dirty 状态、下一安全动作及“不得写入”的边界，判定
`RECOVERY_PASS_WITH_GAP`。

已知 gap：LoopX 的 `review-packet --handoff-only` 没有投影
`worktreeScopeDigest` 与 `checkpoint_commit_authorization_required`。即使先把两者写回 Todo
Evidence，重新生成的 packet digest 仍未变化。恢复 Agent 可以通过 Todo/Git 补齐，但紧凑 packet 自身不是
完整 checkpoint handoff；该问题留给 LoopX 上游，不伪报 full pass。

## 7. visible Runner 已知降级

`auto-research frontier` 从项目 worktree 启动时将当前目录硬编码为公共合同扫描根，因此看到 21 个历史
public-boundary 错误并返回 `frontier_failed`；从中性目录使用同一 Registry/runtime root 则 exit 0。
缺少 scheduler execution context 是另一项上游一致性问题，但不是本次 exit 1 的直接原因。

项目没有为此加入绕过。为避免重复消耗，已停止精确 auto-wake scheduler；显式 quota/status 验收路径通过。
Task 6 独立判定为 `APPROVED_WITH_KNOWN_DEGRADATION`。

## 8. Phase C 临时删除资格

精确候选共 88 个 tracked 文件：

```text
.graph
docs/superpowers
backend/tools/cmd/graphctl
backend/tools/internal/graphctl
```

在 `/tmp` 下创建 detached worktree，机械重放当前 tracked diff 与十三个 Harness 静态文件，再仅删除上述
四个目标。第一次普通 `git rm` 因目标内测试文件已有修改而被 Git 安全拦截，候选完整清理；第二次在新的
一次性 worktree 中确认精确目标后使用 `git rm -r -f`。

当前 18 文件快照上的结果：

| 验证 | 结果 |
|---|---|
| cutover verifier | PASS |
| `thingsbaseline` fresh test | PASS，约 17.2 秒 |
| `thingsbaseline` fresh race | PASS，约 20.4 秒 |
| `thingsbaseline` vet | PASS |
| 原实施 worktree 的 LoopX status | PASS，0 contract error |
| 一次性 worktree 与父临时目录清理 | PASS |

正式 Phase B worktree 没有删除旧 Graph。

## 9. Gate C 完成定义检查

| # | 条件 | 状态 | Evidence |
|---|---|---|---|
| 1 | stable release/manifest/doctor | PASS | §1 |
| 2 | 唯一 Goal 与 Registry 一致 | PASS | §2 |
| 3 | runtime paths ignored/untracked | PASS | Harness + Registry boundary |
| 4 | 三个中性 peer，权限来自 Todo | PASS | §2 |
| 5 | Worker 受 worktree/scope 约束 | PASS | §3 |
| 6 | 独立 Reviewer 拒绝与批准 | PASS | §5 |
| 7 | fresh context 无 transcript 恢复 | PASS_WITH_GAP | §6 |
| 8 | AGENTS/Hook 原子切换 | PASS | §4 |
| 9 | 新任务不再写旧 Graph 文档面 | PASS | 静态合同 |
| 10 | Harness 在预算内，业务 verifier 可运行 | PASS | §4 |
| 11 | Phase C 临时删除验证 | PASS | §8 |
| 12 | Evidence/Review 绑定 exact revision | **BLOCKED** | 当前只有 dirty snapshot |
| 13 | 未永久删除、重写历史或 push | PASS | Git/readback |
| 14 | 首个真实 IoT Loop 排序 Proposal | PASS | `proposals/2026-08-23-first-real-delivery-loop.md` |
| 15 | Phase C handoff 完整 | PASS | §10 |

因此最终状态必须是：

```text
IMPLEMENTATION_APPROVED
GATE_C_BLOCKED
blocker=checkpoint_commit_authorization_required
```

## 10. Phase C handoff

### 10.1 进入前置

- 必须先获得本次 checkpoint commit 的明确授权；
- commit 后重新计算并绑定 exact revision，独立 Reviewer 对该 revision 做只读复核；
- Gate C 接受后才创建 Phase C 删除 Todo/worktree/lease。

### 10.2 删除与保留

删除：

```text
.graph/**
docs/superpowers/**
backend/tools/cmd/graphctl/**
backend/tools/internal/graphctl/**
```

同时删除 `backend/tools/README.md` 中
`R&D Graph historical implementation — RETIRED_PENDING_REMOVAL` 小节。

保留：

```text
automation/loopx/**
.githooks/pre-push
AGENTS.md
backend/tools/cmd/{apicheck,clientcontract,protocompat,rpcgen,saastxn,thingsbaseline}/**
backend/tools/internal/{apicontract,clientcontract,protocompat,rpcgen,saastxn,thingsbaseline}/**
backend/tools/templates/things-baseline/**
```

Phase C 必须先给 Harness 增加“retired”负例/正例：断言四个旧目标不存在，移除对 README 历史占位文本的
依赖，再原子切换 Hook 到新的 retired phase。不能先删除旧代码再让 verifier 失去定义。

### 10.3 验证

```text
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s automation/loopx -p 'test_*.py' -v
PYTHONDONTWRITEBYTECODE=1 python3 automation/loopx/check.py verify --repo-root . --phase retired
git diff --check
cd backend/tools && GOWORK=off go test -count=1 ./...
cd backend/tools && GOWORK=off go test -race -count=1 ./internal/thingsbaseline ./cmd/thingsbaseline
cd backend/tools && GOWORK=off go vet ./...
loopx status --goal-id ming-iot-framework-v1 --format json
```

### 10.4 回退与禁止事项

- 回退点必须是未来获批的 Phase B checkpoint commit；当前 base HEAD 加 dirty digest 不是可执行回退点；
- 保留 Phase A bundle、本地分支和 LoopX stable release snapshot，直至 Phase C 独立 Review 结束；
- Phase C 不继承 Gate F，不执行历史清洗、force-push、普通 push、merge 或生产部署。

## 11. 剩余风险

1. exact checkpoint revision 尚不存在，Gate C 不能接受；
2. LoopX frontier 扫描根和 scheduler context 传递需要上游修复；
3. 紧凑 handoff 缺少 digest/blocker 投影；
4. 本地 Review/Evidence/Proposal 尚未提交，且本地研发文档仓库按策略没有 remote；
5. `iot-doc` 当前存在用户自己的未跟踪设计文档，本阶段只读且未触碰。
