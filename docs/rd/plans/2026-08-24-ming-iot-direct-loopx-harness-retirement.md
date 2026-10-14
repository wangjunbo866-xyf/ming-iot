# ming-iot 直接使用 LoopX 与自定义 Harness 退役实施计划

> **供 Agent Worker 使用：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans` 逐项执行本计划。每一步使用 checkbox 跟踪。

**目标：** 删除 `ming-iot/automation/loopx/` 自定义 Harness 及其 Hook/测试入口，把任务状态迁回 LoopX 原生控制面，并保留最小 `AGENTS.md` 与项目测试。

**架构：** LoopX 原生状态先取消旧 checkpoint Gate、取代旧 Harness Todo，并创建一个有界退役 Todo；源码侧随后删除十三个未跟踪 Harness 文件和唯一 pre-push Hook，只修改三个关联 tracked 文件。验收直接使用结构检查、`backend/tools` Go 测试与 `go vet`，不创建替代 verifier。

**技术栈：** LoopX CLI、Git、Bash、Go、`apply_patch`。

---

## 执行边界

- 实施 worktree：`/home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/loopx-phase-b-bootstrap-cutover`
- 权威规格：`/home/wangjunbo/workspace-iot-platform/ming-iot-rd-docs/specs/2026-08-24-ming-iot-direct-loopx-harness-retirement-design.md`
- Goal：`ming-iot-framework-v1`
- 旧用户 Gate：`todo_cb8a6915286b`
- 旧 Harness Todo：`todo_d74bd2b9737f`
- 新退役 Todo：`todo_3da92e2eab82`
- 不修改 `things`、`iot-doc`、LoopX 安装目录或 IoT 业务代码。
- 不修改 `backend/tools/README.md`；其现有 diff 属于此前 Graph 退役工作，基线摘要必须保持为 `06e7ac7b32729244b8328fb4344d0356c3ffd39f43876e6545a13b382ae38648`。
- 本计划不授权 `git add`、commit、merge、push、生产操作或历史重写。

## 文件变更图

| 路径 | 操作 | 单一责任 |
| --- | --- | --- |
| `automation/loopx/**` | 删除 | 移除项目自定义 Harness、合同、Rules、Skills 与测试 |
| `.githooks/pre-push` | 删除 | 不再运行自定义 Harness 或生命周期门禁 |
| `AGENTS.md` | 修改 | 只保留 LoopX 入口、只读边界与用户授权边界 |
| `.gitignore` | 修改 | 删除只服务于已退役 Harness 的缓存规则，保留 LoopX 本地状态忽略项 |
| `backend/tools/internal/graphctl/changes/changes_test.go` | 修改 | 删除只验证已退役 pre-push 的两个测试，保留 changes 业务测试 |

### Task 1：确认基线并登记已批准规格

**文件：**

- 读取：`ming-iot/.worktrees/loopx-phase-b-bootstrap-cutover/.loopx/registry.json`
- 读取：`ming-iot/.worktrees/loopx-phase-b-bootstrap-cutover/.codex/goals/ming-iot-framework-v1/ACTIVE_GOAL_STATE.md`
- 读取：`ming-iot-rd-docs/specs/2026-08-24-ming-iot-direct-loopx-harness-retirement-design.md`

- [ ] **Step 1：确认 source worktree 只有已知修改**

在实施 worktree 执行：

```bash
git status --short
git diff --binary -- backend/tools/README.md | sha256sum
```

预期：状态包含五个 tracked 修改与未跟踪 `automation/`；README diff 摘要严格等于：

```text
06e7ac7b32729244b8328fb4344d0356c3ffd39f43876e6545a13b382ae38648  -
```

如出现其它路径，停止并报告，不扩展本计划范围。

- [ ] **Step 2：预览规格 authority source 登记**

```bash
loopx --format json register-authority-source \
  --goal-id ming-iot-framework-v1 \
  --source-id direct-loopx-harness-retirement \
  --source-ref '/home/wangjunbo/workspace-iot-platform/ming-iot-rd-docs/specs/2026-08-24-ming-iot-direct-loopx-harness-retirement-design.md' \
  --source-kind doc \
  --role development_control_plane_decision \
  --freshness current \
  --owner-status owner_approved \
  --gate-status readable \
  --boundary private_redacted \
  --revision approved-2026-08-24 \
  --conflict-rule '本规格仅取代项目自定义 Harness 条款；iot-doc 架构语义和 LoopX 原生状态合同继续有效。' \
  --topic development_control_plane \
  --dry-run
```

预期：`ok=true`、`written=false`、`raw_source_ref_stored=false`。

- [ ] **Step 3：执行登记并验证脱敏边界**

```bash
loopx --format json register-authority-source \
  --goal-id ming-iot-framework-v1 \
  --source-id direct-loopx-harness-retirement \
  --source-ref '/home/wangjunbo/workspace-iot-platform/ming-iot-rd-docs/specs/2026-08-24-ming-iot-direct-loopx-harness-retirement-design.md' \
  --source-kind doc \
  --role development_control_plane_decision \
  --freshness current \
  --owner-status owner_approved \
  --gate-status readable \
  --boundary private_redacted \
  --revision approved-2026-08-24 \
  --conflict-rule '本规格仅取代项目自定义 Harness 条款；iot-doc 架构语义和 LoopX 原生状态合同继续有效。' \
  --topic development_control_plane \
  | jq -e '.ok and .written and (.raw_source_ref_stored == false)'
```

预期：`true`。Registry 只能保存本地路径摘要，不保存绝对路径正文。

- [ ] **Step 4：读取登记结果**

```bash
jq -e '.goals[] | select(.id == "ming-iot-framework-v1") | .authority_registry.project_materials["direct-loopx-harness-retirement"] | (.owner_status == "owner_approved" and .gate_status == "readable" and .source_ref_redacted == true)' .loopx/registry.json
```

预期：`true`。

### Task 2：用 LoopX 原生命令取代旧 Harness Todo/Gate

**文件：**

- 本地状态：`.loopx/registry.json`
- 本地状态：`.codex/goals/ming-iot-framework-v1/ACTIVE_GOAL_STATE.md`

- [ ] **Step 1：取消旧 checkpoint commit Gate，而不是批准它**

先预览：

```bash
loopx --format json todo complete \
  --goal-id ming-iot-framework-v1 \
  --role user \
  --todo-id todo_cb8a6915286b \
  --decision-outcome cancel \
  --note '用户批准直接使用 LoopX 并退役自定义 Harness；旧 Phase B checkpoint commit 不再适用，未授予 commit 权限。' \
  --dry-run
```

预期：`decision_outcome=cancel`，且 `mutation_authority.mode=exact_user_gate_decision_scope_override`。

再执行：

```bash
loopx --format json todo complete \
  --goal-id ming-iot-framework-v1 \
  --role user \
  --todo-id todo_cb8a6915286b \
  --decision-outcome cancel \
  --note '用户批准直接使用 LoopX 并退役自定义 Harness；旧 Phase B checkpoint commit 不再适用，未授予 commit 权限。' \
  | jq -e '.ok and (.decision_outcome == "cancel") and (.status == "done")'
```

预期：`true`；不得出现 `decision_outcome=approve`。

- [ ] **Step 2：创建新的有界退役 Todo**

先预览：

```bash
loopx --format json todo add \
  --goal-id ming-iot-framework-v1 \
  --role agent \
  --text '退役 automation/loopx 自定义 Harness，并恢复为 LoopX 原生状态、最小 AGENTS 与项目测试。' \
  --task-class advancement_task \
  --action-kind retire_custom_loopx_harness \
  --task-domain repository-maintenance \
  --task-repository git:gitee.com/ming-industry/ming-iot \
  --required-write-scope 'automation/loopx/**' \
  --required-write-scope '.githooks/pre-push' \
  --required-write-scope '.gitignore' \
  --required-write-scope 'AGENTS.md' \
  --required-write-scope 'backend/tools/internal/graphctl/changes/changes_test.go' \
  --required-capability filesystem_write \
  --required-capability shell \
  --claimed-by ming-peer-01 \
  --dry-run
```

预期：`todo_id=todo_3da92e2eab82`、`status=open`，且不存在 `backend/tools/README.md` 写范围。

再执行同一命令但移除 `--dry-run`，并验证：

```bash
loopx --format json todo add \
  --goal-id ming-iot-framework-v1 \
  --role agent \
  --text '退役 automation/loopx 自定义 Harness，并恢复为 LoopX 原生状态、最小 AGENTS 与项目测试。' \
  --task-class advancement_task \
  --action-kind retire_custom_loopx_harness \
  --task-domain repository-maintenance \
  --task-repository git:gitee.com/ming-industry/ming-iot \
  --required-write-scope 'automation/loopx/**' \
  --required-write-scope '.githooks/pre-push' \
  --required-write-scope '.gitignore' \
  --required-write-scope 'AGENTS.md' \
  --required-write-scope 'backend/tools/internal/graphctl/changes/changes_test.go' \
  --required-capability filesystem_write \
  --required-capability shell \
  --claimed-by ming-peer-01 \
  | jq -e '.ok and (.todo_id == "todo_3da92e2eab82") and (.status == "open")'
```

预期：`true`。

- [ ] **Step 3：先链接新 Todo，再将旧 Harness Todo 标记为 superseded**

LoopX 0.5.2 要求已存在的后继 Todo 先通过 `todo update` 链接到源 Todo，`todo supersede`
不接受 `--successor-todo-id`。先使用 `--dry-run` 预览，再执行链接：

```bash
loopx --format json todo update \
  --goal-id ming-iot-framework-v1 \
  --role agent \
  --todo-id todo_d74bd2b9737f \
  --agent-id ming-peer-01 \
  --successor-todo-id todo_3da92e2eab82 \
  --dry-run
```

预期：`ok=true`、`changed=true`，且 `successor_todo_ids` 包含 `todo_3da92e2eab82`。移除
`--dry-run` 执行后，再分别预览和执行：

```bash
loopx --format json todo supersede \
  --goal-id ming-iot-framework-v1 \
  --role agent \
  --todo-id todo_d74bd2b9737f \
  --agent-id ming-peer-01 \
  --reason '用户批准直接使用 LoopX 并退役自定义 Harness；旧 Phase B Harness 实施目标已被新规格取代。' \
  | jq -e '.ok and .superseded and (.successor_todo_ids | index("todo_3da92e2eab82") != null)'
```

预期：`true`。旧 Todo 保留历史，不记为实现成功。

- [ ] **Step 4：把 Goal 写边界收敛到本次退役文件**

先运行不带 `--execute` 的预览：

```bash
loopx --format json configure-goal \
  --goal-id ming-iot-framework-v1 \
  --replace-write-scope \
  --write-scope 'automation/loopx/**' \
  --write-scope '.githooks/pre-push' \
  --write-scope '.gitignore' \
  --write-scope 'AGENTS.md' \
  --write-scope 'backend/tools/internal/graphctl/changes/changes_test.go'
```

预期：`dry_run=true`、`changed_fields=["write_scope"]`。

再执行：

```bash
loopx --format json configure-goal \
  --goal-id ming-iot-framework-v1 \
  --replace-write-scope \
  --write-scope 'automation/loopx/**' \
  --write-scope '.githooks/pre-push' \
  --write-scope '.gitignore' \
  --write-scope 'AGENTS.md' \
  --write-scope 'backend/tools/internal/graphctl/changes/changes_test.go' \
  --execute \
  | jq -e '.ok and .written and (.after.write_scope | index("backend/tools/README.md") == null)'
```

预期：`true`。

- [ ] **Step 5：确认 LoopX 已指向新 Todo**

```bash
loopx --format json todo list --goal-id ming-iot-framework-v1 \
  | jq '{user:.user_todos.items, agent:.agent_todos.items}'
```

预期：旧 Gate 为 `done/cancel`，旧 Harness Todo 为已取代状态，新 `todo_3da92e2eab82` 为唯一当前退役工作。

### Task 3：建立删除前的 RED 结构基线

**文件：**

- 读取：`automation/loopx/**`
- 读取：`.githooks/pre-push`
- 读取：`AGENTS.md`
- 读取：`.gitignore`
- 读取：`backend/tools/internal/graphctl/changes/changes_test.go`

- [ ] **Step 1：证明目标目录当前仍存在**

```bash
test ! -e automation/loopx
```

预期：退出码 `1`。这就是删除目标的 RED 基线。

- [ ] **Step 2：列出精确删除清单**

```bash
find automation/loopx -type f -not -path '*/__pycache__/*' -print | sort
find automation/loopx/__pycache__ -type f -print 2>/dev/null || true
```

预期：第一条列出十三个已知静态文件；第二条只允许出现可再生 `.pyc`。

- [ ] **Step 3：证明入口引用当前仍存在**

```bash
rg -n 'automation/loopx|check\.py|repository-policy\.md|release-lock\.json' \
  AGENTS.md .gitignore .githooks backend/tools/internal/graphctl/changes
```

预期：至少命中 AGENTS、ignore、pre-push 与 changes 测试。

### Task 4：删除自定义 Harness 与 pre-push

**文件：**

- 删除：`automation/loopx/README.md`
- 删除：`automation/loopx/authority-catalog.json`
- 删除：`automation/loopx/check.py`
- 删除：`automation/loopx/mcp-policy.md`
- 删除：`automation/loopx/multi-agent-spec.public.json`
- 删除：`automation/loopx/project-contract.json`
- 删除：`automation/loopx/release-lock.json`
- 删除：`automation/loopx/role-contracts.json`
- 删除：`automation/loopx/rules/repository-policy.md`
- 删除：`automation/loopx/skills/planner/SKILL.md`
- 删除：`automation/loopx/skills/reviewer/SKILL.md`
- 删除：`automation/loopx/skills/worker/SKILL.md`
- 删除：`automation/loopx/test_check.py`
- 删除：`.githooks/pre-push`

- [ ] **Step 1：使用 `apply_patch` 删除十三个文本文件和 Hook**

```diff
*** Begin Patch
*** Delete File: automation/loopx/README.md
*** Delete File: automation/loopx/authority-catalog.json
*** Delete File: automation/loopx/check.py
*** Delete File: automation/loopx/mcp-policy.md
*** Delete File: automation/loopx/multi-agent-spec.public.json
*** Delete File: automation/loopx/project-contract.json
*** Delete File: automation/loopx/release-lock.json
*** Delete File: automation/loopx/role-contracts.json
*** Delete File: automation/loopx/rules/repository-policy.md
*** Delete File: automation/loopx/skills/planner/SKILL.md
*** Delete File: automation/loopx/skills/reviewer/SKILL.md
*** Delete File: automation/loopx/skills/worker/SKILL.md
*** Delete File: automation/loopx/test_check.py
*** Delete File: .githooks/pre-push
*** End Patch
```

- [ ] **Step 2：删除明确验证过的忽略缓存**

只有 Task 3 的第二条命令确认该目录只包含 `.pyc` 后，才执行：

```bash
rm -rf -- automation/loopx/__pycache__
```

该命令只针对精确目录，不得扩大到 `automation/`、worktree 根或任何变量路径。

- [ ] **Step 3：删除空目录外壳**

```bash
rmdir -- automation/loopx/skills/planner
rmdir -- automation/loopx/skills/reviewer
rmdir -- automation/loopx/skills/worker
rmdir -- automation/loopx/skills
rmdir -- automation/loopx/rules
rmdir -- automation/loopx
```

预期：全部成功；保留可能为空的父目录 `automation/`，因为它不构成 Harness。

### Task 5：精简 tracked 协作入口与测试

**文件：**

- 修改：`AGENTS.md`
- 修改：`.gitignore`
- 修改：`.graph/registry.yaml`
- 修改：`.graph/loops/G0-rd-graph-control-plane.yaml`
- 修改：`backend/tools/internal/graphctl/changes/changes_test.go`
- 修改：`backend/tools/internal/graphctl/registry/registry_test.go`

- [ ] **Step 1：将 `AGENTS.md` 替换为最小 LoopX 入口**

文件完整内容必须为：

```markdown
# ming-iot LoopX 协作入口

## 开始修改前

1. 读取当前状态：`loopx status --goal-id ming-iot-framework-v1`。
2. 读取当前 LoopX Todo、写范围和用户 Gate，不依赖旧聊天推断任务。
3. `iot-doc` 中的架构默认只读；`things` 永久只读。

## 修改与完成

- 所有修改必须绑定当前 Todo，并在独立 worktree 内且符合获批的 write scope。
- 完成前运行与变更相关的项目测试和静态检查。
- LoopX 状态不替代 Git diff、测试结果或用户检查。

## 用户授权边界

- commit、merge、push、生产变更和历史重写分别需要用户明确授权。
```

- [ ] **Step 2：删除 Harness 专用 ignore**

使用 `apply_patch` 只删除这一行：

```diff
-/automation/loopx/__pycache__/
```

必须保留：

```gitignore
/.loopx/
/.codex/goals/
/.codex/skills/
/.local/
/.opencode/goals/
```

- [ ] **Step 3：删除两个 Hook 专用 Go 测试**

从 `backend/tools/internal/graphctl/changes/changes_test.go` 完整删除：

```go
func TestHookIsExecutableAndHasValidBashSyntax(t *testing.T) {
	hook := filepath.Join(repositoryRootFromChangesTest(t), ".githooks", "pre-push")
	info, err := os.Lstat(hook)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o755 {
		t.Fatalf("pre-push mode = %v, want regular 0755", info.Mode())
	}
	command := exec.Command("bash", "-n", hook)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("bash -n pre-push: %v\n%s", err, output)
	}
}

func TestHookInvokesLoopXCutoverVerifier(t *testing.T) {
	hook := filepath.Join(repositoryRootFromChangesTest(t), ".githooks", "pre-push")
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	fakeBin := filepath.Join(root, "bin")
	for _, directory := range []string{repo, fakeBin} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fakeGit := filepath.Join(fakeBin, "git")
	writeTestFile(t, fakeBin, "git", "#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"$*\" != \"rev-parse --show-toplevel\" ]]; then exit 90; fi\nprintf '%s\\n' \"$HOOK_TEST_REPO\"\n")
	if err := os.Chmod(fakeGit, 0o755); err != nil {
		t.Fatal(err)
	}
	fakePython := filepath.Join(fakeBin, "python3")
	writeTestFile(t, fakeBin, "python3", "#!/usr/bin/env bash\nset -euo pipefail\n{ printf 'PWD=%s\\n' \"$PWD\"; printf 'ARG=%s\\n' \"$@\"; } > \"$HOOK_TEST_CAPTURE\"\n")
	if err := os.Chmod(fakePython, 0o755); err != nil {
		t.Fatal(err)
	}

	capture := filepath.Join(root, "capture.txt")
	command := exec.Command(hook)
	command.Dir = root
	command.Stdin = strings.NewReader("refs/heads/main ignored refs/heads/main ignored\n")
	command.Env = []string{
		"PATH=" + fakeBin + ":/usr/bin:/bin",
		"HOOK_TEST_REPO=" + repo,
		"HOOK_TEST_CAPTURE=" + capture,
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Hook rejected cutover verifier invocation: %v\n%s", err, output)
	}
	content, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	wantLines := []string{
		"PWD=" + root,
		"ARG=" + filepath.Join(repo, "automation", "loopx", "check.py"),
		"ARG=verify",
		"ARG=--repo-root",
		"ARG=" + repo,
		"ARG=--phase",
		"ARG=cutover",
	}
	want := strings.Join(wantLines, "\n") + "\n"
	if string(content) != want {
		t.Fatalf("cutover invocation =\n%s\nwant exactly\n%s", content, want)
	}
}
```

不要删除 `strings`、`os`、`os/exec` 或 `filepath` import；文件中的其它测试仍使用它们。

- [ ] **Step 4：格式化受影响 Go 测试**

```bash
gofmt -w backend/tools/internal/graphctl/changes/changes_test.go
```

- [ ] **Step 5：从旧 Graph G0 清单移除已退役 Hook 声明**

`GOWORK=off go test ./internal/graphctl/registry -count=1 -v` 已作为 RED 证据，确认旧 Graph
的三层契约仍将已删除的 Hook 视为有效输入：G0 `scope.owns`、G0
`evidence.subjectPaths` 和 Registry `controlPlane.globalImpact`。使用 `apply_patch`：

1. 仅从 `.graph/loops/G0-rd-graph-control-plane.yaml` 删除两处：

```yaml
      - .githooks/**
```

2. 仅从 `.graph/registry.yaml` 的 `controlPlane.globalImpact` 删除一处同样声明。
3. 仅从 `backend/tools/internal/graphctl/registry/registry_test.go` 的 `wantGlobal`
   固定期望中删除 `.githooks/**`。

不放宽 Registry 存在性测试，不删除其它 G0/Registry 声明，也不删除 graphctl
对其它仓库 Hook 配置的通用检测能力。

### Task 6：运行结构检查与项目校验

**文件：**

- 验证：`AGENTS.md`
- 验证：`.gitignore`
- 验证：`.graph/registry.yaml`
- 验证：`.graph/loops/G0-rd-graph-control-plane.yaml`
- 验证：`backend/tools/internal/graphctl/changes/changes_test.go`
- 验证：`backend/tools/internal/graphctl/registry/registry_test.go`
- 验证：`backend/tools/README.md` 未被本次修改

- [ ] **Step 1：验证 Harness 与 Hook 已完全退出**

```bash
test ! -e automation/loopx
test ! -e .githooks/pre-push
```

预期：两条命令均退出 `0`。

- [ ] **Step 2：验证没有有效入口引用**

```bash
if rg -n 'automation/loopx|check\.py|repository-policy\.md|release-lock\.json' \
  AGENTS.md .gitignore .graph .githooks backend/tools/internal/graphctl/changes; then
  exit 1
fi
```

预期：无输出，退出 `0`。

- [ ] **Step 3：验证 LoopX 本地状态继续被忽略**

```bash
git check-ignore -q .loopx/registry.json
git check-ignore -q .codex/goals/ming-iot-framework-v1/ACTIVE_GOAL_STATE.md
```

预期：两条命令均退出 `0`。

- [ ] **Step 4：证明未触碰既有 README diff**

```bash
git diff --binary -- backend/tools/README.md | sha256sum
```

预期仍为：

```text
06e7ac7b32729244b8328fb4344d0356c3ffd39f43876e6545a13b382ae38648  -
```

- [ ] **Step 5：运行聚焦 Go 测试**

在 `backend/tools` 目录执行：

```bash
GOWORK=off go test ./internal/graphctl/changes
```

预期：`ok`。

- [ ] **Step 6：运行 tools 全包测试与 Vet**

仍在 `backend/tools` 目录执行：

```bash
GOWORK=off go test ./...
GOWORK=off go vet ./...
```

预期：全部退出 `0`。本计划不运行全仓库 Race 测试。

- [ ] **Step 7：检查 diff 质量**

回到 worktree 根执行：

```bash
git diff --check
git diff --stat
```

预期：`git diff --check` 无输出。

### Task 7：用户检查后完成 LoopX 回写

**文件：**

- 本地状态：`.loopx/registry.json`
- 本地状态：`.codex/goals/ming-iot-framework-v1/ACTIVE_GOAL_STATE.md`

- [ ] **Step 1：展示 Source Control 并核对精确边界**

```bash
git status --short
git diff --name-status
git diff -- AGENTS.md .gitignore .githooks/pre-push backend/tools/internal/graphctl/changes/changes_test.go
git diff --binary -- backend/tools/README.md | sha256sum
```

预期本次退役涉及的 tracked 路径与动作仅为：

| 动作 | 路径 |
| --- | --- |
| 删除 | `.githooks/pre-push` |
| 修改 | `.gitignore` |
| 修改 | `AGENTS.md` |
| 修改 | `.graph/registry.yaml` |
| 修改 | `.graph/loops/G0-rd-graph-control-plane.yaml` |
| 修改 | `backend/tools/internal/graphctl/changes/changes_test.go` |
| 修改 | `backend/tools/internal/graphctl/registry/registry_test.go` |

以实际命令输出为准：`git status --short` 对未暂存删除显示为 ` D`，而 `git diff --name-status` 显示为 `D`；不得为了匹配示例而执行 `git add`。

`backend/tools/README.md` 仍会显示此前已有的修改，但其 diff 摘要必须保持为 `06e7ac7b32729244b8328fb4344d0356c3ffd39f43876e6545a13b382ae38648`。`automation/loopx/` 已完整删除，因此不再作为 untracked 路径出现。

- [ ] **Step 2：暂停并等待用户检查**

向用户汇报删除清单、三个测试命令结果、README 保护摘要和当前 `git status`，然后停止执行。

只有用户明确回复“通过”“验收完成”或同等含义后，才能继续 Step 3。用户检查不授权 `git add`、commit、merge 或 push。

- [ ] **Step 3：预览退役 Todo 完成与业务 successor**

```bash
loopx --format json todo complete \
  --goal-id ming-iot-framework-v1 \
  --role agent \
  --todo-id todo_3da92e2eab82 \
  --agent-id ming-peer-01 \
  --evidence 'automation/loopx 与 pre-push 已删除；结构扫描、backend/tools 全包测试和 go vet 通过；用户已检查 Source Control；残余风险为 AGENTS 不是机器门禁且当前无托管 CI；未 commit、merge 或 push。' \
  --next-agent-todo '基于本地首个真实交付 Proposal，准备首个 IoT 交付 Loop 的用户决策包。' \
  --next-task-class advancement_task \
  --next-action-kind prepare_first_real_iot_delivery_decision \
  --next-task-repository git:gitee.com/ming-industry/ming-iot \
  --next-required-capability shell \
  --next-continuation-policy independent_handoff \
  --dry-run
```

预期：当前 Todo 变为 `done`，并预览一个未领取的 successor；不得自动开始业务实现。

- [ ] **Step 4：执行 Todo 完成回写**

运行同一命令但移除 `--dry-run`，并验证：

```bash
loopx --format json todo complete \
  --goal-id ming-iot-framework-v1 \
  --role agent \
  --todo-id todo_3da92e2eab82 \
  --agent-id ming-peer-01 \
  --evidence 'automation/loopx 与 pre-push 已删除；结构扫描、backend/tools 全包测试和 go vet 通过；用户已检查 Source Control；残余风险为 AGENTS 不是机器门禁且当前无托管 CI；未 commit、merge 或 push。' \
  --next-agent-todo '基于本地首个真实交付 Proposal，准备首个 IoT 交付 Loop 的用户决策包。' \
  --next-task-class advancement_task \
  --next-action-kind prepare_first_real_iot_delivery_decision \
  --next-task-repository git:gitee.com/ming-industry/ming-iot \
  --next-required-capability shell \
  --next-continuation-policy independent_handoff \
  | jq -e '.ok and .completed and (.status == "done") and (.next_todos | length == 1)'
```

预期：`true`。

- [ ] **Step 5：清除已过期的 Goal 级 Harness 写范围**

先预览：

```bash
loopx --format json configure-goal --goal-id ming-iot-framework-v1 --clear-write-scope
```

预期：`after.write_scope=[]`。再执行：

```bash
loopx --format json configure-goal \
  --goal-id ming-iot-framework-v1 \
  --clear-write-scope \
  --execute \
  | jq -e '.ok and .written and (.after.write_scope | length == 0)'
```

预期：`true`。下一个真实交付 Loop 必须重新声明自己的 write scope。

- [ ] **Step 6：验证最终 LoopX 投影**

```bash
loopx --format json todo list --goal-id ming-iot-framework-v1 \
  | jq '{user:.user_todos.items, agent:.agent_todos.items}'
loopx --format json configure-goal --goal-id ming-iot-framework-v1 \
  | jq '{write_scope:.after.write_scope}'
loopx status --goal-id ming-iot-framework-v1
```

预期：

- 旧 checkpoint Gate 已 `cancel`；
- 旧 Harness Todo 已被取代；
- 退役 Todo 已完成并带测试摘要；
- 下一项只是首个真实 IoT Loop 的决策准备；
- Goal 级 `write_scope` 为空；
- 状态中不再要求运行 `automation/loopx/check.py`。

- [ ] **Step 7：再次确认 Source Control，不提交**

```bash
git status --short
git diff --name-status
git diff -- AGENTS.md .gitignore .githooks/pre-push backend/tools/internal/graphctl/changes/changes_test.go
```

预期与 Step 1 用户已检查的源码差异完全一致，只有 LoopX 本地状态发生了完成回写。

在此停止。未经新的明确授权，不执行 `git add`、commit、merge 或 push。
