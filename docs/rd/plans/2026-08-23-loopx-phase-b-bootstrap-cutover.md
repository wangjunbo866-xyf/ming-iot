# LoopX Phase B 引导切换与资格验收实施计划

> **面向 Agent Worker：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans` 逐任务实施。步骤使用复选框跟踪；任何实现或修复先使用 `superpowers:test-driven-development`，完成声明前使用 `superpowers:verification-before-completion`。

**目标：** 将 `ming-iot` 从旧 Graph 日常控制面安全切换到可追溯的 LoopX `stable` release snapshot 和最小静态 Harness，并完成 Gate C 的 Worker、Reviewer、恢复与临时删除资格验收。

**架构：** Phase B 在独立 `ming-iot` worktree 中先建立连接安全种子，再安装并验证 LoopX、连接唯一 Goal 和注册同级 peers。连接后的 Todo 管理静态 Harness 与原子 cutover；旧 Graph 在本阶段只作回退基线，不删除、不接收新状态。研发规格、计划、Evidence、Review 和 Proposal 只写入本地 `ming-iot-rd-docs`。

**技术栈：** LoopX `stable` release snapshot、Python 3 标准库 `unittest`、JSON 静态合同、Git worktree、Go 工具链与既有 `thingsbaseline`。

---

## 1. 执行边界与已完成前置

实施 worktree：

```text
/home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/loopx-phase-b-bootstrap-cutover
branch: phase-b/loopx-bootstrap-cutover
base: 403b9a6d6b7e2c60a1ff96ef0f681da272d04c36
```

已完成只读前置：

- `.worktrees/` 已由 `ming-iot/.gitignore` 忽略；
- 旧 Registry `validate` 通过；
- 新路径 `automation/loopx/project-contract.json` 被旧 Graph 返回 `UNOWNED_PATH`；
- `backend/tools` 基线 `GOWORK=off go test ./...` 通过，耗时约 18 秒；
- 本机现有 LoopX 是 `0.5.2` Python distribution，但没有 release manifest/source lineage；
- 官方 `stable` 与 Tag `v0.5.2` 当前均指向 `423035f402e2f1703f076c3cfe60c14c5803433f`。

`UNOWNED_PATH` 是本次 Bootstrap 的已知引导边界。不得通过新增 `.graph` owner 解决；本计划、已批准设计和用户授权共同构成一次性迁移依据。旧 Graph 仍用于已有路径的基线核对，但不再获得新动态状态。

提交约束：任务中的 commit 命令只作为精确 checkpoint 方案。当前仓库规则仍要求用户明确授权 commit；授权前不得执行 `git add`、`git commit`、merge 或 push。可以连续完成未提交实现、测试和 Evidence 草稿，最终一次性报告待提交范围。

## 2. 文件映射

### `ming-iot` worktree

| 文件 | 责任 |
|---|---|
| `.gitignore` | 忽略 LoopX、Codex Goal 与 Host 本地状态 |
| `AGENTS.md` | 短而稳定的 LoopX/Harness 入口 |
| `.githooks/pre-push` | 只运行快速无网络静态检查 |
| `automation/loopx/release-lock.json` | 官方 stable release identity 与资格状态 |
| `automation/loopx/project-contract.json` | Goal、仓库、Bootstrap scope 与运行时边界 |
| `automation/loopx/authority-catalog.json` | `iot-doc` 逻辑权威引用与 revision |
| `automation/loopx/role-contracts.json` | peers、临时角色、风险拓扑和用户 Gate |
| `automation/loopx/multi-agent-spec.public.json` | 官方 visible Runner 的最小公开角色规格 |
| `automation/loopx/mcp-policy.md` | 外部能力和敏感副作用准入 |
| `automation/loopx/rules/repository-policy.md` | 仓库、worktree、scope、commit/push、恢复规则 |
| `automation/loopx/skills/*/SKILL.md` | Planner/Worker/Reviewer 的临时角色方法 |
| `automation/loopx/check.py` | 无网络、确定性的静态合同检查 |
| `automation/loopx/test_check.py` | verifier 正反例与回归测试 |
| `automation/loopx/README.md` | 安装、连接、状态、验证、恢复与回退命令 |
| `backend/tools/README.md` | 将 graphctl 标为历史待退役，保留业务 verifier 说明 |

### `ming-iot-rd-docs`

| 文件 | 责任 |
|---|---|
| `evidence/2026-08-23-loopx-phase-b-bootstrap-cutover/evidence.md` | Gate C 压缩 Evidence |
| `reviews/2026-08-23-loopx-phase-b-bootstrap-cutover-review.md` | 独立 Review |
| `proposals/2026-08-23-first-real-delivery-loop.md` | 首个真实 IoT Loop 排序提案 |

## 3. 实施任务

### Task 1：用 TDD 建立连接安全种子

**Files:**
- Modify: `ming-iot/.worktrees/loopx-phase-b-bootstrap-cutover/.gitignore`
- Create: `ming-iot/.worktrees/loopx-phase-b-bootstrap-cutover/automation/loopx/project-contract.json`
- Create: `ming-iot/.worktrees/loopx-phase-b-bootstrap-cutover/automation/loopx/release-lock.json`
- Create: `ming-iot/.worktrees/loopx-phase-b-bootstrap-cutover/automation/loopx/check.py`
- Create: `ming-iot/.worktrees/loopx-phase-b-bootstrap-cutover/automation/loopx/test_check.py`

- [ ] **Step 1：先写失败测试**

使用 `unittest` 和临时仓库 fixture，写入以下可执行测试骨架：

```python
import json
from pathlib import Path
import shutil
import tempfile
import unittest

from check import ContractError, verify_repository


class StaticContractTest(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp(prefix="loopx-contract-test-"))

    def tearDown(self):
        shutil.rmtree(self.root)

    def write_text(self, relative, content):
        target = self.root / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")

    def write_json(self, relative, payload):
        self.write_text(relative, json.dumps(payload, ensure_ascii=False, indent=2) + "\n")

    def seed_repo(self):
        self.write_text(
            ".gitignore",
            "/.loopx/\n/.codex/goals/\n/.codex/skills/\n/.local/\n/.opencode/goals/\n",
        )
        self.write_json("automation/loopx/project-contract.json", {
            "schemaVersion": 1,
            "goalId": "ming-iot-framework-v1",
            "objective": "按 iot-doc 的架构与语义，实现完整、可验证、可持续演进的工业物联网平台 ming-iot。",
            "repositoryIdentity": "git:gitee.com/ming-industry/ming-iot",
            "bootstrapWriteScopes": ["automation/loopx/**"],
            "runtimeIgnoredPaths": ["/.loopx/", "/.codex/goals/", "/.codex/skills/", "/.local/", "/.opencode/goals/"],
        })
        self.write_json("automation/loopx/release-lock.json", {
            "schemaVersion": 1,
            "channel": "github_archive_stable",
            "repository": "huangruiteng/loopx",
            "ref": "stable",
            "version": "0.5.2",
            "sourceCommit": "423035f402e2f1703f076c3cfe60c14c5803433f",
            "qualificationStatus": "candidate",
            "releaseId": None,
            "manifestSha256": None,
        })

    def test_rejects_missing_runtime_ignores(self):
        self.seed_repo()
        self.write_text(".gitignore", "/.loopx/\n")
        with self.assertRaisesRegex(ContractError, "RUNTIME_IGNORE_MISSING"):
            verify_repository(self.root, phase="connection_seed")

    def test_rejects_wrong_goal_id(self):
        self.seed_repo()
        path = self.root / "automation/loopx/project-contract.json"
        payload = json.loads(path.read_text(encoding="utf-8"))
        payload["goalId"] = "duplicate-goal"
        self.write_json("automation/loopx/project-contract.json", payload)
        with self.assertRaisesRegex(ContractError, "GOAL_ID_INVALID"):
            verify_repository(self.root, phase="connection_seed")

    def test_rejects_wrong_stable_commit(self):
        self.seed_repo()
        path = self.root / "automation/loopx/release-lock.json"
        payload = json.loads(path.read_text(encoding="utf-8"))
        payload["sourceCommit"] = "0" * 40
        self.write_json("automation/loopx/release-lock.json", payload)
        with self.assertRaisesRegex(ContractError, "RELEASE_SOURCE_INVALID"):
            verify_repository(self.root, phase="connection_seed")

    def test_rejects_personal_absolute_path(self):
        self.seed_repo()
        path = self.root / "automation/loopx/project-contract.json"
        payload = json.loads(path.read_text(encoding="utf-8"))
        payload["localDocs"] = "/home/example/private"
        self.write_json("automation/loopx/project-contract.json", payload)
        with self.assertRaisesRegex(ContractError, "PRIVATE_PATH_FORBIDDEN"):
            verify_repository(self.root, phase="connection_seed")

    def test_accepts_connection_seed_mode(self):
        self.seed_repo()
        result = verify_repository(self.root, phase="connection_seed")
        self.assertEqual(result["status"], "PASS")
```

`connection_seed` 模式只校验 `.gitignore`、`project-contract.json` 和 release candidate，不要求尚未创建的角色/Harness 文件。

- [ ] **Step 2：确认测试先失败**

Run：

```bash
python3 -m unittest discover -s automation/loopx -p 'test_*.py' -v
```

Expected：FAIL，原因是 `check.py` 或 `verify_repository(repo_root, phase="connection_seed")` 尚不存在。

- [ ] **Step 3：实现最小 verifier 和种子合同**

`project-contract.json` 固定：

```json
{
  "schemaVersion": 1,
  "goalId": "ming-iot-framework-v1",
  "objective": "按 iot-doc 的架构与语义，实现完整、可验证、可持续演进的工业物联网平台 ming-iot。",
  "repositoryIdentity": "git:gitee.com/ming-industry/ming-iot",
  "bootstrapWriteScopes": [
    ".gitignore",
    "AGENTS.md",
    ".githooks/pre-push",
    "automation/loopx/**",
    "backend/tools/README.md"
  ],
  "runtimeIgnoredPaths": ["/.loopx/", "/.codex/goals/", "/.codex/skills/", "/.local/", "/.opencode/goals/"]
}
```

`release-lock.json` 初始状态固定为：

```json
{
  "schemaVersion": 1,
  "channel": "github_archive_stable",
  "repository": "huangruiteng/loopx",
  "ref": "stable",
  "version": "0.5.2",
  "sourceCommit": "423035f402e2f1703f076c3cfe60c14c5803433f",
  "qualificationStatus": "candidate",
  "releaseId": null,
  "manifestSha256": null
}
```

`check.py` 使用 `argparse` 暴露：

```text
python3 automation/loopx/check.py verify --repo-root . --phase connection_seed
python3 automation/loopx/check.py verify --repo-root . --phase cutover
```

实现必须只使用 Python 标准库；扫描 committed contract 内容时拒绝 `/home/`、`/root/`、Windows 用户目录、凭据字段和不允许的绝对路径。

- [ ] **Step 4：补齐忽略项并运行测试**

在 `.gitignore` 增加：

```gitignore
/.loopx/
/.codex/goals/
/.codex/skills/
/.local/
/.opencode/goals/
```

Run：

```bash
env PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s automation/loopx -p 'test_*.py' -v
env PYTHONDONTWRITEBYTECODE=1 python3 automation/loopx/check.py verify --repo-root . --phase connection_seed
git check-ignore .loopx/registry.json .codex/goals/ming-iot-framework-v1/ACTIVE_GOAL_STATE.md .codex/skills/ming-worker/SKILL.md .local/probe .opencode/goals/probe
```

Expected：全部 PASS，五个 probe 均显示被忽略。

- [ ] **Step 5：记录 checkpoint，但不越过提交授权**

```bash
git status --short
git diff --check
```

授权后使用的精确提交命令：

```bash
git add .gitignore automation/loopx/check.py automation/loopx/test_check.py automation/loopx/project-contract.json automation/loopx/release-lock.json
git commit -m "build(loopx): seed phase b connection contract"
```

### Task 2：安装并证明官方 stable release snapshot

**Files:**
- Modify: `automation/loopx/release-lock.json`

- [ ] **Step 1：重新核对 stable，不使用旧快照假设**

Run：

```bash
loopx --format json update --check
git ls-remote https://github.com/huangruiteng/loopx.git refs/heads/stable refs/tags/v0.5.2
```

Expected：版本 `0.5.2`，两个 ref 都是 `423035f402e2f1703f076c3cfe60c14c5803433f`。不一致则停止，不安装。

- [ ] **Step 2：预演并执行官方安装**

Run：

```bash
loopx --format json update --dry-run
loopx --format json update --execute
```

Expected：安装成功，官方 stable release snapshot 写入本地 release 目录；不修改项目文件。

- [ ] **Step 3：使用明确路径读回来源**

Run：

```bash
/root/.local/bin/loopx --format json doctor
/root/.local/bin/loopx --format json update --check
```

Expected：默认命令指向 release snapshot、release manifest 可用、source commit 与刚刚读回的 `stable` 一致，且 `source_identity_matches=true`。

说明：官方 no-clone 安装不会创建带 Git lineage 的 `loopx-canary`。因此 `update --check` 的 `runtime_activation_qualification` 仍可能返回 `activation_qualification_required`，其含义是“没有 canary/源码 checkout 可用于 installed-versus-target ancestry 比较”，不等于 release snapshot 来源不合格。Phase B 的安装来源资格由同一轮 `git ls-remote`、release manifest 的 repo/ref/source commit、默认命令 readback 和 manifest 文件摘要共同证明；不得把该状态误写成 `runtime_active`。

- [ ] **Step 4：将公开 release identity 写回 lock**

把 `qualificationStatus` 改为 `qualified`，填入 doctor 返回的 `releaseId` 和 release manifest 文件 SHA-256。不得写入绝对安装路径或完整 doctor JSON。

- [ ] **Step 5：回归验证**

```bash
python3 -m unittest discover -s automation/loopx -p 'test_*.py' -v
python3 automation/loopx/check.py verify --repo-root . --phase connection_seed
```

Expected：PASS。

### Task 3：连接唯一 Goal、注册 peers 并写入 Bootstrap Todo

**Files:**
- Runtime only, ignored: `.loopx/**`, `.codex/goals/**`, `~/.codex/loopx/**`

- [ ] **Step 1：连接 dry-run**

```bash
/root/.local/bin/loopx connect \
  --project . \
  --goal-id ming-iot-framework-v1 \
  --objective "按 iot-doc 的架构与语义，实现完整、可验证、可持续演进的工业物联网平台 ming-iot。" \
  --domain industrial-iot-platform \
  --write-scope 'automation/loopx/**' \
  --write-scope AGENTS.md \
  --write-scope .gitignore \
  --write-scope .githooks/pre-push \
  --write-scope backend/tools/README.md \
  --allowed-domain harness \
  --allowed-domain review \
  --allowed-domain planning \
  --no-onboarding-scan \
  --codex-app-heartbeat no \
  --dry-run
```

Expected：只预览 `.loopx/registry.json`、ACTIVE_GOAL_STATE 和全局 route，不生成业务 Todo。

- [ ] **Step 2：执行连接并读回**

重复上一步但移除 `--dry-run`，然后运行：

```bash
/root/.local/bin/loopx status --goal-id ming-iot-framework-v1 --format json
/root/.local/bin/loopx registry
git status --short --ignored
```

Expected：唯一 Goal/objective 正确，运行时路径只显示为 ignored，不进入普通 `git status`。

- [ ] **Step 3：preview 并注册三个 peers**

```bash
/root/.local/bin/loopx register-agent --goal-id ming-iot-framework-v1 \
  --agent-id ming-peer-01 --agent-id ming-peer-02 --agent-id ming-peer-03 --require-new
/root/.local/bin/loopx register-agent --goal-id ming-iot-framework-v1 \
  --agent-id ming-peer-01 --agent-id ming-peer-02 --agent-id ming-peer-03 --require-new --execute
```

Expected：三个身份均为同级 `peer_v1`，没有永久角色或 Leader。

- [ ] **Step 4：记录已有用户决策的有界 Gate**

先加 `--dry-run` 预演，确认无敏感内容后执行：

```bash
/root/.local/bin/loopx operator-gate \
  --goal-id ming-iot-framework-v1 \
  --gate phase_bcd_sequence \
  --decision approve \
  --reason-summary "用户于 2026-08-23 选择分阶段方案 A，授权按 Phase B、C、D 顺序继续；commit、merge、push 与 Gate F 不在此授权内。" \
  --recommended-action "完成 Phase B Bootstrap 与 Gate C，随后进入 Phase C。"
```

- [ ] **Step 5：创建并绑定 Harness Todo**

```bash
/root/.local/bin/loopx todo add \
  --goal-id ming-iot-framework-v1 \
  --role agent \
  --text "实现并验证 Phase B 静态 Harness 与原子 cutover" \
  --task-class advancement_task \
  --action-kind implement_phase_b_harness \
  --task-domain harness \
  --task-repository git:gitee.com/ming-industry/ming-iot \
  --required-write-scope 'automation/loopx/**' \
  --required-write-scope AGENTS.md \
  --required-write-scope .githooks/pre-push \
  --required-write-scope backend/tools/README.md \
  --required-capability filesystem_write \
  --required-capability shell \
  --claimed-by ming-peer-01 \
  --validation-command-json '["python3","-m","unittest","discover","-s","automation/loopx","-p","test_*.py","-v"]' \
  --validation-timeout-seconds 20
```

Expected：返回结构化 todo id；将该 ID 记录到本地 Evidence 摘要。

### Task 4：TDD 完成静态 Harness、角色合同和可见 Runner spec

**Files:**
- Create: `automation/loopx/authority-catalog.json`
- Create: `automation/loopx/role-contracts.json`
- Create: `automation/loopx/multi-agent-spec.public.json`
- Create: `automation/loopx/mcp-policy.md`
- Create: `automation/loopx/rules/repository-policy.md`
- Create: `automation/loopx/skills/planner/SKILL.md`
- Create: `automation/loopx/skills/worker/SKILL.md`
- Create: `automation/loopx/skills/reviewer/SKILL.md`
- Create: `automation/loopx/README.md`
- Modify: `automation/loopx/check.py`
- Modify: `automation/loopx/test_check.py`

- [ ] **Step 1：扩展失败测试**

在 Task 1 的临时仓库 fixture 上增加 `write_cutover_contracts()`，写入 authority、role 和 launch spec；随后增加：

```python
def write_cutover_contracts(self):
    self.seed_repo()
    release_path = self.root / "automation/loopx/release-lock.json"
    release = json.loads(release_path.read_text(encoding="utf-8"))
    release.update({
        "qualificationStatus": "qualified",
        "releaseId": "v0.5.2-423035f4",
        "manifestSha256": "a" * 64,
    })
    self.write_json("automation/loopx/release-lock.json", release)
    self.write_json("automation/loopx/authority-catalog.json", {
        "schemaVersion": 1,
        "sources": [{
            "repository": "iot-doc",
            "revision": "287d41a7ca2f5d444ceb34264fbe1ac515c42836",
            "path": "亿级点位高吞吐架构/README.md",
            "classification": "normative",
        }],
    })
    self.write_json("automation/loopx/role-contracts.json", {
        "schemaVersion": 1,
        "agentModel": "peer_v1",
        "peers": [
            {"agentId": "ming-peer-01"},
            {"agentId": "ming-peer-02"},
            {"agentId": "ming-peer-03"},
        ],
        "separation": {"reviewerMustDifferFromWorker": True},
    })
    self.write_json("automation/loopx/multi-agent-spec.public.json", {
        "schema_version": "generic_multi_agent_launch_spec_v0",
        "goal_id": "ming-iot-framework-v1",
        "session_name": "ming-iot-framework-v1-visible",
        "default_reasoning_effort": "high",
        "roles": [
            {"lane_id": "planner", "agent_id": "ming-peer-03", "role_id": "planner", "scope": "只读提出有界提案。"},
            {"lane_id": "worker", "agent_id": "ming-peer-01", "role_id": "worker", "scope": "只执行获批 Todo。"},
            {"lane_id": "reviewer", "agent_id": "ming-peer-02", "role_id": "reviewer", "scope": "独立复核 exact revision。"},
        ],
    })
    self.write_text("AGENTS.md", "# LoopX 协作入口\n\n先读取 LoopX status 和 automation/loopx 静态合同。\n")
    self.write_text(
        ".githooks/pre-push",
        "#!/usr/bin/env bash\nset -euo pipefail\nrepo_root=$(git rev-parse --show-toplevel)\nexec python3 \"$repo_root/automation/loopx/check.py\" verify --repo-root \"$repo_root\" --phase cutover\n",
    )
    self.write_text("backend/tools/README.md", "# Tools\n\nR&D Graph historical implementation — RETIRED_PENDING_REMOVAL\n")
    self.write_text("automation/loopx/mcp-policy.md", "# MCP policy\n\n默认只读；外部写入需要用户 Gate。\n")
    self.write_text("automation/loopx/rules/repository-policy.md", "# Repository policy\n\n修改必须绑定 worktree 与 write scope。\n")
    for role in ("planner", "worker", "reviewer"):
        self.write_text(
            f"automation/loopx/skills/{role}/SKILL.md",
            f"# {role}\n\n读取当前 Todo，只在临时角色边界内工作并提交 Evidence。\n",
        )

def test_rejects_role_bearing_durable_agent_id(self):
    self.write_cutover_contracts()
    path = self.root / "automation/loopx/role-contracts.json"
    payload = json.loads(path.read_text(encoding="utf-8"))
    payload["peers"][0]["agentId"] = "ming-worker-peer-01"
    self.write_json("automation/loopx/role-contracts.json", payload)
    with self.assertRaisesRegex(ContractError, "DURABLE_ROLE_ID_FORBIDDEN"):
        verify_repository(self.root, phase="cutover")

def test_rejects_worker_as_reviewer(self):
    self.write_cutover_contracts()
    path = self.root / "automation/loopx/role-contracts.json"
    payload = json.loads(path.read_text(encoding="utf-8"))
    payload["separation"]["reviewerMustDifferFromWorker"] = False
    self.write_json("automation/loopx/role-contracts.json", payload)
    with self.assertRaisesRegex(ContractError, "REVIEW_SEPARATION_REQUIRED"):
        verify_repository(self.root, phase="cutover")

def test_rejects_absolute_authority_path(self):
    self.write_cutover_contracts()
    path = self.root / "automation/loopx/authority-catalog.json"
    payload = json.loads(path.read_text(encoding="utf-8"))
    payload["sources"][0]["path"] = "/home/example/iot-doc"
    self.write_json("automation/loopx/authority-catalog.json", payload)
    with self.assertRaisesRegex(ContractError, "PRIVATE_PATH_FORBIDDEN"):
        verify_repository(self.root, phase="cutover")

def test_rejects_runner_agent_not_in_role_contract(self):
    self.write_cutover_contracts()
    path = self.root / "automation/loopx/multi-agent-spec.public.json"
    payload = json.loads(path.read_text(encoding="utf-8"))
    payload["roles"][0]["agent_id"] = "unknown-peer"
    self.write_json("automation/loopx/multi-agent-spec.public.json", payload)
    with self.assertRaisesRegex(ContractError, "RUNNER_AGENT_UNKNOWN"):
        verify_repository(self.root, phase="cutover")

def test_rejects_dynamic_state_in_static_contract(self):
    self.write_cutover_contracts()
    self.write_json("automation/loopx/dynamic-state.json", {"todoStatus": "done"})
    with self.assertRaisesRegex(ContractError, "DYNAMIC_STATE_FORBIDDEN"):
        verify_repository(self.root, phase="cutover")

def test_accepts_complete_cutover_contract(self):
    self.write_cutover_contracts()
    result = verify_repository(self.root, phase="cutover")
    self.assertEqual(result["status"], "PASS")
```

Run：同 Task 1。Expected：新测试 FAIL。

- [ ] **Step 2：实现完整合同**

固定 peers 为 `ming-peer-01..03`，`agentModel=peer_v1`；角色仅为 Todo 临时职责。`authority-catalog.json` 使用 `repository: iot-doc`、相对路径和已提交 revision `287d41a7ca2f5d444ceb34264fbe1ac515c42836`，不写工作区绝对路径。

`multi-agent-spec.public.json` 使用：

```json
{
  "schema_version": "generic_multi_agent_launch_spec_v0",
  "goal_id": "ming-iot-framework-v1",
  "session_name": "ming-iot-framework-v1-visible",
  "default_reasoning_effort": "high",
  "roles": [
    {"lane_id": "planner", "agent_id": "ming-peer-03", "role_id": "planner", "scope": "只读提出一个有界交付提案。", "skill": {"name": "ming-planner", "source": "skills/planner/SKILL.md"}},
    {"lane_id": "worker", "agent_id": "ming-peer-01", "role_id": "worker", "scope": "只执行已获批 Todo 和 write scope。", "skill": {"name": "ming-worker", "source": "skills/worker/SKILL.md"}},
    {"lane_id": "reviewer", "agent_id": "ming-peer-02", "role_id": "reviewer", "scope": "以 fresh context 独立复核 exact revision。", "skill": {"name": "ming-reviewer", "source": "skills/reviewer/SKILL.md"}}
  ]
}
```

三个 Skill 只写角色决策、Evidence 和交接；不得复制 tmux、quota、LoopX 项目连接或调度实现。

- [ ] **Step 3：实现 `cutover` 模式检查**

除 JSON Schema/交叉引用外，检查：

- `AGENTS.md` 与 Hook 不含 `.graph/registry`、`graphctl` 或 `docs/superpowers`；
- `backend/tools/README.md` 含 `RETIRED_PENDING_REMOVAL`，且旧命令只在该历史段中存在；
- `automation/loopx/**` 不含动态 Todo/Gate 完成状态；
- 所有路径引用为逻辑仓库 + 相对路径；
- launch spec peers 与 role contracts 完全一致。

- [ ] **Step 4：运行单测**

```bash
env PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s automation/loopx -p 'test_*.py' -v
```

Expected：PASS。实际仓库的 `cutover` 检查此时仍应因旧 AGENTS/Hook 失败，证明测试有区分力。

### Task 5：原子切换 AGENTS、Hook 和工具说明

**Files:**
- Modify: `AGENTS.md`
- Modify: `.githooks/pre-push`
- Modify: `backend/tools/README.md`
- Modify: `backend/tools/internal/graphctl/changes/changes_test.go`
- Modify if needed: `.gitignore`

- [ ] **Step 1：写仓库级负例测试并确认失败**

测试实际入口必须拒绝旧 `graphctl` Hook 和 Graph 权威 AGENTS。Run：

```bash
python3 automation/loopx/check.py verify --repo-root . --phase cutover
```

Expected：FAIL，列出旧 AGENTS/Hook。

- [ ] **Step 2：替换为最小入口**

新 `AGENTS.md` 只包含：

1. 动态状态读取 `loopx status --goal-id ming-iot-framework-v1`；
2. 静态合同读取 `automation/loopx/`；
3. `iot-doc` 架构默认只读、`things` 永久只读；
4. 修改必须绑定 Todo/worktree/write scope；
5. 完成前运行适用测试与静态 verifier；
6. commit、merge、push、生产和历史强推需要各自明确授权。

同时在 `.gitignore` 增加 `/automation/loopx/__pycache__/`。该目录只是 Python 本地可再生缓存，
必须保持 ignored 且 untracked，不属于静态 Harness 资产。

Hook 固定为：

```bash
#!/usr/bin/env bash
set -euo pipefail
repo_root=$(git rev-parse --show-toplevel)
exec python3 "$repo_root/automation/loopx/check.py" verify --repo-root "$repo_root" --phase cutover
```

`backend/tools/README.md` 的 Graph 小节标题改为 `R&D Graph historical implementation — RETIRED_PENDING_REMOVAL`，删除日常命令和权威措辞，指明 Phase C 将删除源码；保留 `thingsbaseline` 等业务 verifier 文档。

旧 `changes_test.go` 在 Phase C 删除前仍属于 `go test ./...` 的现行回归面。它必须停止断言 Hook 转发给
graphctl，改为精确验证 Hook 调用 LoopX cutover verifier；不得通过在新 Hook 中增加 graphctl 回退来迁就旧测试。

- [ ] **Step 3：验证原子切换**

```bash
env PYTHONDONTWRITEBYTECODE=1 python3 automation/loopx/check.py verify --repo-root . --phase cutover
env PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s automation/loopx -p 'test_*.py' -v
env PYTHONDONTWRITEBYTECODE=1 /usr/bin/time -f '%e' python3 automation/loopx/check.py verify --repo-root . --phase cutover
git diff --check
```

Expected：全部 PASS；热缓存目标小于 10 秒。

- [ ] **Step 4：运行适用 Go 回归**

```bash
cd backend/tools
env GOWORK=off go test ./...
env GOWORK=off go test -race ./internal/thingsbaseline ./cmd/thingsbaseline
env GOWORK=off go vet ./...
```

Expected：PASS。

- [ ] **Step 5：准备 cutover checkpoint**

授权后使用：

```bash
git add AGENTS.md .gitignore .githooks/pre-push automation/loopx backend/tools/README.md
git commit -m "feat(loopx): cut over phase b static harness"
```

### Task 6：预演并启动官方 visible multi-agent Runner

**Files:**
- Runtime only: tmux session `ming-iot-framework-v1-visible`
- Local Evidence summary

- [ ] **Step 1：dry-run launch spec**

```bash
/root/.local/bin/loopx multi-agent launch \
  --spec automation/loopx/multi-agent-spec.public.json \
  --workspace . \
  --session-name ming-iot-framework-v1-visible \
  --no-auto-wake \
  --format json
```

Expected：三个 pane 的 agent/role/scope 正确，机器 JSON 走 artifact，不打印私有 state；不启动 tmux。

- [ ] **Step 2：启动可见会话但不阻塞当前 Agent**

```bash
/root/.local/bin/loopx multi-agent launch \
  --spec automation/loopx/multi-agent-spec.public.json \
  --workspace . \
  --session-name ming-iot-framework-v1-visible \
  --execute \
  --auto-wake
tmux list-windows -t ming-iot-framework-v1-visible
```

Expected：三个真实 Codex TUI pane 存活；不使用 `--attach` 阻塞实施进程。需要人工观察时执行 `tmux attach -t ming-iot-framework-v1-visible`。

- [ ] **Step 3：逐 lane 检查 quota/status**

```bash
/root/.local/bin/loopx quota should-run --goal-id ming-iot-framework-v1 --agent-id ming-peer-01 --runtime-profile codex_cli --turn-envelope
/root/.local/bin/loopx status --goal-id ming-iot-framework-v1 --agent-id ming-peer-02 --format json
/root/.local/bin/loopx status --goal-id ming-iot-framework-v1 --agent-id ming-peer-03 --format json
```

Expected：Worker 可见 Harness Todo；Reviewer/Planner 不因 pane 名称自动获得写权限。

### Task 7：完成 Worker Evidence、独立 Reviewer 拒绝与修复闭环

**Files:**
- Modify: LoopX runtime Todo/Evidence
- Create: `ming-iot-rd-docs/reviews/2026-08-23-loopx-phase-b-bootstrap-cutover-review.md`

- [ ] **Step 1：Worker claim/lease 与验证**

从 `loopx todo list` 读取 exact todo id；使用 `ming-peer-01` claim，并以公开安全 idempotency key 获取只覆盖 Phase B 文件的 task lease。运行 Task 5 的全部验证，记录 exact Git revision、diff digest 和命令结果。

- [ ] **Step 2：构造“不充分 Evidence”审查输入**

第一次交接故意只给出“测试通过”声明，不给 exact revision、命令和 scope。Reviewer 使用 `ming-peer-02` fresh pane，必须在 Review 文档写：

```text
verdict: REJECTED
reason: 缺少 exact revision、验证命令和 write-scope 证明
```

不得伪造机器 PASS，不得让 Reviewer 修改代码。

- [ ] **Step 3：Worker 补齐 Evidence 并重审**

补齐 exact revision/diff digest、测试命令、输出摘要、scope 和剩余风险。Reviewer 重跑验证并将最终 verdict 更新为 `APPROVED_FOR_GATE_C`；Review 必须同时保留首次拒绝记录。

- [ ] **Step 4：完成 Todo 或保留提交 Gate blocker**

若已有明确 commit 授权和 checkpoint revision，使用 LoopX `todo complete` 绑定 validation receipt；否则将 Todo 标为 blocked，原因固定为 `checkpoint_commit_authorization_required`，不得用 dirty tree 冒充 exact revision。

### Task 8：证明 fresh context 恢复与无 transcript handoff

**Files:**
- Runtime only: LoopX handoff/status/history
- Local Evidence summary

- [ ] **Step 1：生成紧凑 handoff**

```bash
/root/.local/bin/loopx review-packet --goal-id ming-iot-framework-v1 --agent-id ming-peer-02 --handoff-only --format json
/root/.local/bin/loopx evidence-log --goal-id ming-iot-framework-v1 --agent-id ming-peer-01 --thin
```

Expected：包含 Goal、selected Todo、Gate、scope、revision/Evidence 引用和 next safe action，不包含 transcript。

- [ ] **Step 2：停止旧 Worker pane并用 fresh pane恢复**

停止只对应 Worker 的 pane或整个测试 session，再用同一 spec 重新启动。fresh pane 只读取 LoopX status、Git/worktree 和 handoff packet，回答：当前 Goal、Todo、scope、revision、blocker、下一安全动作。

- [ ] **Step 3：对比恢复结果**

恢复结果必须与 LoopX/Git 一致；若依赖旧 Chat 才能回答，记录失败并创建 repair Todo，不得通过人工补上下文宣称成功。

### Task 9：在临时 worktree 验证 Phase C 可删除性

**Files:**
- Disposable detached worktree only
- Local Evidence summary

- [ ] **Step 1：解析精确删除目标**

```bash
git ls-files .graph docs/superpowers backend/tools/cmd/graphctl backend/tools/internal/graphctl
```

Expected：只包含规格声明的旧控制面路径。

- [ ] **Step 2：创建一次性 detached worktree并删除目标**

使用 `mktemp -d` 创建明确临时目录，`git worktree add --detach <path> <cutover-revision>`，只在该 worktree 执行：

```bash
git rm -r .graph docs/superpowers backend/tools/cmd/graphctl backend/tools/internal/graphctl
```

不得在主 worktree 或 Phase B 实施 worktree执行删除。

- [ ] **Step 3：运行删除候选验证**

```bash
python3 automation/loopx/check.py verify --repo-root . --phase cutover
cd backend/tools
env GOWORK=off go test ./internal/thingsbaseline ./cmd/thingsbaseline
env GOWORK=off go test -race ./internal/thingsbaseline ./cmd/thingsbaseline
env GOWORK=off go vet ./internal/thingsbaseline ./cmd/thingsbaseline
```

并从原实施 worktree运行 `loopx status --goal-id ming-iot-framework-v1`，证明项目动态状态不依赖删除候选树。

- [ ] **Step 4：安全移除一次性 worktree**

先确认目标是刚创建的精确临时路径，再执行 `git worktree remove --force <path>`；不得使用宽泛递归删除。

### Task 10：形成 Gate C Evidence、Proposal 与 Phase C handoff

**Files:**
- Create: `ming-iot-rd-docs/evidence/2026-08-23-loopx-phase-b-bootstrap-cutover/evidence.md`
- Create: `ming-iot-rd-docs/proposals/2026-08-23-first-real-delivery-loop.md`
- Finalize: `ming-iot-rd-docs/reviews/2026-08-23-loopx-phase-b-bootstrap-cutover-review.md`

- [ ] **Step 1：写压缩 Evidence**

必须包含 release lineage、Goal/peers readback、Todo/lease、exact revision 或明确 commit blocker、验证命令、Reviewer 拒绝/通过、恢复结果、临时删除结果和剩余风险。只写摘要、digest 和 artifact ID，不复制原始 transcript/完整 doctor JSON。

- [ ] **Step 2：生成最多三个真实交付候选**

从 `iot-doc` 权威依赖与 `ming-iot` 现状排序，至少比较：

1. Things 迁移后的最小服务启动/契约基线；
2. Kafka 基础；
3. 模块级物模型基础。

Proposal 只提出 objective、dependencies、write scopes、acceptance、non-goals 和 risks，不创建业务实现 Todo。

- [ ] **Step 3：完成独立 Review 与 Gate C 检查表**

逐项映射设计规格 15 条完成定义。任何一项缺失时状态为 `GATE_C_BLOCKED`；全部满足时才能写 `BOOTSTRAP_QUALIFIED / GATE_C_ACCEPTED`。

- [ ] **Step 4：准备 Phase C handoff**

列出待删除 `.graph`、graphctl、旧 README/Hook 引用，保留的 `thingsbaseline`/Go verifier，验证命令、回退点和不得执行的历史强推。Phase C 不继承 Gate F。

- [ ] **Step 5：最终只读核对**

```bash
git -C /home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/loopx-phase-b-bootstrap-cutover status --short
git -C /home/wangjunbo/workspace-iot-platform/ming-iot-rd-docs status --short
/root/.local/bin/loopx status --goal-id ming-iot-framework-v1 --format json
```

报告所有预期和意外变更。未经明确提交授权，不执行 commit；任何情况下不执行 merge 或 push。

## 4. 计划自审

### 4.1 规格覆盖

- release snapshot 与 lineage：Task 2；
- Goal、peers 和用户 Gate：Task 3；
- 静态 Harness、Rules、Skills、MCP policy：Task 1、4、5；
- visible Runner：Task 6；
- Worker/Reviewer 拒绝闭环：Task 7；
- fresh context 恢复：Task 8；
- 临时删除验证：Task 9；
- Evidence、Proposal、Phase C handoff：Task 10；
- commit/push/Gate F 边界：全计划与 Task 10。

### 4.2 非目标检查

计划不实现 IoT 业务、不 fork LoopX、不开发 Runner/Dashboard、不修改 `things`、不删除主树旧 Graph、不执行历史重写、merge 或 push。

### 4.3 完成声明

本计划最多产生 `BOOTSTRAP_QUALIFIED / GATE_C_ACCEPTED`。若 commit 授权尚未获得或 exact revision 缺失，只能产生 `GATE_C_BLOCKED`，不得降低完成谓词。
