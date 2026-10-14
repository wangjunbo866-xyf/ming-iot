# LoopX Phase B 引导切换独立 Review

> 日期：2026-08-23
>
> Goal：`ming-iot-framework-v1`
>
> Worker：`ming-peer-01`
>
> Reviewer：`ming-peer-02`
>
> 状态：实现已批准；Gate C 因缺少 checkpoint commit 保持阻塞

## 1. Review 边界

本 Review 只判断 Phase B 静态 Harness、原子 cutover、Worker Evidence 与 Gate C 资格，不授权 commit、merge、push、生产变更或历史重写。Reviewer 使用 fresh context，只读复核，不修改被审实现。

## 2. 第一轮：不充分 Evidence 拒绝演练

### 2.1 Worker 交接原文

```text
测试通过。
```

该交接没有 exact revision、验证命令、命令结果或 write-scope 证明。Reviewer 不得自行从工作区补齐提交者遗漏的 Evidence。

### 2.2 Reviewer 判定

```text
verdict: REJECTED
reason: 缺少 exact revision、验证命令和 write-scope 证明。
```

该拒绝证明“命令曾经绿过”不能替代可复核 Evidence。

## 3. 第二轮：补齐 Evidence 后发现真实回归

### 3.1 Worker 补齐的 Evidence

当前 Git 基准与快照：

```text
HEAD: 403b9a6d6b7e2c60a1ff96ef0f681da272d04c36
checkpointCommit: NONE
worktreeScopeDigest: 408fee392cdd3a0a850a03c75f16030e8e0cbc8796d137360678f968be1f6ab1
trackedDiffDigest: ae79f881d19d3a99a96a84ee1666be2f9fab8e411992dfcdaa7e4c6279f75ae6
changedFileCount: 17
```

`worktreeScopeDigest` 的输入为：第一行 `head <HEAD>`；随后按 `git status --porcelain=v1 --untracked-files=all` 返回顺序，为每个非 ignored 变更文件写一行 `file <octal-mode> <sha256> <path>`；最后对整个 LF 文本流计算 SHA-256。它同时覆盖四个 tracked 入口文件和 `automation/loopx/` 下十三个 untracked 静态文件。`trackedDiffDigest` 是四个 tracked 入口相对 HEAD 的 `git diff --binary` 输出 SHA-256。

Todo `todo_d74bd2b9737f` 已由 `ming-peer-01` 领取，硬 lease 为 version 1，精确 write scope 是：

```text
.gitignore
automation/loopx/**
AGENTS.md
.githooks/pre-push
backend/tools/README.md
```

Worker 首次申报：

```text
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s automation/loopx -p 'test_*.py'
结果：81/81 PASS，2.292 秒

PYTHONDONTWRITEBYTECODE=1 python3 automation/loopx/check.py verify --repo-root . --phase cutover
结果：PASS

git diff --check
结果：PASS

cd backend/tools && env GOWORK=off go test ./...
结果：PASS

cd backend/tools && env GOWORK=off go test -race ./internal/thingsbaseline ./cmd/thingsbaseline
结果：PASS

cd backend/tools && env GOWORK=off go vet ./...
结果：PASS
```

visible Runner 读回：dry-run 为 `ok=true`；`ming-iot-framework-v1-visible` 的 Planner、Worker、Reviewer 三个 Codex pane 存活；Worker 的 `quota should-run --runtime-profile codex_cli` 选择当前 Todo；Reviewer 与 Planner 的 agent scope frontier 均为 `reassignment_required / quiet_noop_allowed=true`，没有获得 Worker 写权限。

已知降级：pane tick 调用 `auto-research frontier` 时返回 `frontier_failed`。独立 Reviewer 与主控的固定变量对照实验确认，直接原因是该命令硬编码以当前工作目录作为公共合同扫描根；从 Phase B worktree 扫描时 status 为 `ok=false`、产生 21 个历史合同错误，从中性目录使用同一 Registry/runtime root 执行则 exit 0 并正确选择当前 Todo。安装版与当前本地 LoopX 源码还存在另一项上游一致性缺陷：该 CLI 直接调用 quota API 却未传 `scheduler_execution_context`，子命令也没有 `--runtime-profile` 参数；但为同一不健康 status 补入 `codex_cli` 上下文仍然失败，因此它不是本次 exit 1 的直接原因。同版本显式执行 `quota should-run --runtime-profile codex_cli` 正常。项目没有修改安装目录、fork LoopX 或伪造 pane tick PASS；为避免重复消耗，已停止精确 PID 的 auto-wake scheduler，三个 pane 与 artifact 保留。

Task 6 独立 Reviewer verdict 为 `APPROVED_WITH_KNOWN_DEGRADATION`：已批准 Task 6 与 Gate C 使用的显式 quota/status 验收路径正常；无需在项目资产中绕过，扫描根一致性和 scheduler context 传递留给 LoopX 上游修复。

### 3.2 剩余边界

- 当前只有已精确摘要的 dirty worktree，没有 checkpoint commit；
- 用户尚未明确授权本次 `git add` / `git commit`；
- Reviewer 可以批准该 scope digest 上的实现与验证，但 Gate C 最终状态必须保持 blocked，直到存在 exact checkpoint revision；
- 本 Review 不授权 merge、push、生产变更或历史重写。

### 3.3 第二轮 Reviewer 判定

独立 Reviewer 使用 fresh context 强制重跑 `GOWORK=off go test -count=1 ./...`，稳定发现：

```text
implementation_verdict: CHANGES_REQUIRED
gate_c_state: GATE_C_BLOCKED
finding: TestHookForwardsExactlyOneRefRecord 仍断言旧 graphctl Hook，和已批准的 LoopX cutover Hook 冲突。
```

这证明只记录缓存命中或聚焦测试不足以覆盖原子切换。问题属于项目回归，不能记为 LoopX 上游降级，也不能因为
graphctl 将在 Phase C 删除而忽略当前红灯。

## 4. 修复与第三轮独立 Review

### 4.1 精确扩展 scope

在修改测试前，Worker 先释放 hard lease version 1，将 Todo 的 write scope 精确扩展一项：

```text
backend/tools/internal/graphctl/changes/changes_test.go
```

随后以 `phase-b-bootstrap-cutover-worker-v2` 重新取得 hard lease。评审时 owner 为
`ming-peer-01`，lease version/epoch 均为 2，六项 scope 与 Git 变更完全一致。最终 Review 后 Todo
已因 checkpoint blocker 转为 blocked，lease version 2 已释放。

修复没有给新 Hook 添加 graphctl 回退，而是把遗留测试改为正向验证以下精确调用：

```text
python3 <repo>/automation/loopx/check.py verify --repo-root <repo> --phase cutover
```

### 4.2 修复后快照

```text
HEAD: 403b9a6d6b7e2c60a1ff96ef0f681da272d04c36
checkpointCommit: NONE
worktreeScopeDigest: 3d397d3c3d80b455259b03ba737250f18aab593a4d78e79550ad3437c23c718c
trackedDiffDigest: 948a2f85ffe8804698c2564933b77e3b681b8e2263286afadd44425314a4c3ff
changedFileCount: 18
```

摘要算法与 §3.1 相同；本轮覆盖五个 tracked 文件和 `automation/loopx/` 下十三个 untracked 静态文件。

修复后 fresh 验证：

```text
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s automation/loopx -p 'test_*.py' -v
结果：81/81 PASS，约 3.3 秒

PYTHONDONTWRITEBYTECODE=1 python3 automation/loopx/check.py verify --repo-root . --phase cutover
结果：PASS

git diff --check
结果：PASS

cd backend/tools && GOWORK=off go test -count=1 ./...
结果：PASS，约 20.2 秒

cd backend/tools && GOWORK=off go test -race -count=1 ./internal/thingsbaseline ./cmd/thingsbaseline
结果：PASS，约 22.5 秒

cd backend/tools && GOWORK=off go vet ./...
结果：PASS
```

### 4.3 第三轮 Reviewer 判定

```text
implementation_verdict: APPROVED_FOR_GATE_C
gate_c_state: GATE_C_BLOCKED
findings: 无 P0/P1/P2 实现缺陷。
```

Reviewer 独立重算 18 文件摘要并核对 Todo/lease 六项 scope，结果与 Worker 申报一致。该批准只适用于上述
dirty snapshot，不把它冒充 checkpoint revision。

## 5. 最终边界

- Phase B 实现质量已通过独立 Review；
- Gate C 完成定义第 12 项仍不满足：Evidence/Review 尚未绑定 exact checkpoint commit；
- 唯一 Gate C blocker 固定为 `checkpoint_commit_authorization_required`；
- 未执行 `git add`、commit、merge、push、生产动作、旧 Graph 永久删除或历史重写。
