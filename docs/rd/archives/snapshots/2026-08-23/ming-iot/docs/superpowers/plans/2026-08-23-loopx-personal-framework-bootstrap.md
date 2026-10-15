# LoopX Personal Framework Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Use `superpowers:test-driven-development` for verifier changes, `superpowers:writing-skills` for role skills, and `superpowers:verification-before-completion` before every completion claim.

**Goal:** Replace the self-built Graph as the dynamic control plane with an official, pinned LoopX `v0.5.1` bootstrap that can recover one exact `ming-iot` Goal, launch visible peer agents, enforce the static Harness, produce independent Evidence/Review, and propose—but not implement—the first real IoT delivery Loop.

**Architecture:** LoopX becomes the only owner of dynamic Goal/Todo/Gate/claim/Evidence state. `iot-doc` remains the architecture authority, Git remains the code authority, and a small standard-library Python verifier checks only versioned static contracts and reproducible architecture snapshots. Project-local runtime data stays under ignored `.loopx/`, `.codex/goals/`, and `.local/`; existing `.graph` files remain immutable historical records. Planner, Worker, and Reviewer are equal `peer_v1` identities with temporary roles, while merge, push, and sensitive effects remain user-gated.

**Tech Stack:** LoopX `0.5.1` from PyPI, Python 3.11+ standard library, Codex CLI, tmux, Git/worktrees, JSON/Markdown contracts, existing Go 1.24 project verifiers.

**Design:**

- `iot-doc/docs/superpowers/specs/2026-08-21-loopx-driven-iot-development-control-plane-design.md`
- `iot-doc/docs/superpowers/specs/2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md`

**Commit authorization:** Repository policy overrides automatic-commit instructions. At every checkpoint, show `git status --short` and `git diff --cached --name-only`, then wait for a fresh explicit user authorization before `git add` or `git commit`. Never push without a separate explicit authorization.

---

## Scope and completion boundary

This is the first implementation slice of the approved phase-one design. It finishes the control-plane bootstrap and one non-business qualification loop. It does **not** claim that the whole phase-one pilot is complete.

In scope:

- lock and install official LoopX `0.5.1` locally;
- establish the static Harness, authority catalog, role contracts, MCP policy, and deterministic verifier;
- connect the exact `ming-iot-framework-v1` Goal and register three fresh peer identities;
- dry-run and visibly launch the official multi-agent Runner;
- demonstrate Worker evidence, independent Reviewer output, process restart recovery, and user approval;
- atomically switch `AGENTS.md`, the hook, ignore rules, and documentation away from dynamic `.graph` ownership;
- retain G0/L0 and their Evidence/Review as immutable history;
- produce a ranked proposal for the first real IoT delivery Loop.

Out of scope:

- implementing any IoT business feature;
- declaring phase one complete before a real delivery Loop passes proposal, implementation, four-layer verification, Review, merge Gate, readback, and Closeout;
- adding or modifying `.graph/loops/*.yaml`, G0/L0 completion, or historical Evidence;
- forking or modifying LoopX, building `company-dev`, or building a custom Agent Runner;
- enabling Headless Turn as the default execution path;
- introducing Multica, DeerFlow, LangGraph, A2A, a new Dashboard, or a second dynamic Registry;
- modifying the external read-only `../things` repository;
- running `go work sync`;
- automatic merge, push, deployment, production credentials, or irreversible external effects.

The final bootstrap result must be reported as `BOOTSTRAP_QUALIFIED`, never `PHASE_ONE_COMPLETE`.

## Authority and state ownership

| Fact | Owner after cutover | Versioned location |
|---|---|---|
| IoT architecture and semantics | `iot-doc` | normative Markdown at a committed revision |
| Static repository rules | Git | `AGENTS.md`, `automation/loopx/rules/`, role skills and policies |
| Dynamic Goal/Todo/Gate/Evidence | LoopX | ignored `.loopx/` and host LoopX runtime |
| Code and revision | Git | `ming-iot` repository/worktrees |
| Deterministic static checks | verifier | `automation/loopx/check.py` and tests |
| Independent judgment | Reviewer peer | LoopX evidence plus committed Review summary |
| Merge, push and sensitive effects | user | explicit Gate/readback, not inferred from chat |
| Historical G0/L0 | frozen `.graph` | existing tracked files and artifacts, read-only |

## Stable public identifiers

Use these exact identifiers in every task:

```text
goal_id                 ming-iot-framework-v1
objective               按 iot-doc 的架构与语义，实现完整、可验证、可持续演进的工业物联网平台 ming-iot。
planner_agent_id        ming-planner-peer-01
worker_agent_id         ming-worker-peer-01
reviewer_agent_id       ming-reviewer-peer-01
visible_session         ming-iot-framework-v1-visible
static_contract_schema  ming_iot_loopx_static_contract_v1
snapshot_schema         architecture_snapshot_v1
```

## File map

| File | Responsibility |
|---|---|
| `.gitignore` | Keep LoopX, Codex Goal, virtualenv, session, and generated snapshot state out of Git |
| `AGENTS.md` | Short static Harness entrypoint; no dynamic Loop registry |
| `.githooks/pre-push` | Fast deterministic static check only; no multi-minute lifecycle orchestration |
| `automation/loopx/requirements.lock` | Exact LoopX wheel version and SHA-256 |
| `automation/loopx/release-lock.json` | Human/machine-readable PyPI release identity and rollback version |
| `automation/loopx/authority-catalog.json` | Classify authoritative `iot-doc` material |
| `automation/loopx/role-contracts.json` | `peer_v1` roles, risk topology, permissions, and user Gates |
| `automation/loopx/multi-agent-spec.public.json` | Official visible multi-agent launch recipe |
| `automation/loopx/check.py` | Deterministic `verify` and `snapshot` commands; no dynamic lifecycle state |
| `automation/loopx/test_check.py` | Negative and reproducibility tests for the static verifier |
| `automation/loopx/skills/*/SKILL.md` | Planner, Worker, and Reviewer role behavior only |
| `automation/loopx/rules/repository-policy.md` | Repository, worktree, commit, push, and failure rules |
| `automation/loopx/mcp-policy.md` | External capability and credential admission policy |
| `automation/loopx/legacy-graph-freeze.json` | Immutable cutover receipt for old `.graph` history |
| `automation/loopx/README.md` | Install, connect, launch, recover, rollback, and upgrade runbook |
| `backend/tools/README.md` | Mark graphctl lifecycle as historical; preserve reusable verifier documentation |
| `docs/superpowers/evidence/2026-08-23-loopx-personal-framework-bootstrap.md` | Compact, reproducible bootstrap Evidence |
| `docs/superpowers/reviews/2026-08-23-loopx-personal-framework-bootstrap-review.md` | Independent Reviewer findings and verdict |
| `docs/superpowers/proposals/2026-08-23-first-real-delivery-loop.md` | Ranked candidates and user selection Gate for the first business Loop |

---

### Task 1: Lock the official release and isolate local runtime state

**Files:**

- Create: `automation/loopx/requirements.lock`
- Create: `automation/loopx/release-lock.json`
- Modify: `.gitignore`

- [ ] **Step 1: Record the exact release artifacts**

Create `automation/loopx/requirements.lock` with exactly:

```text
loopx==0.5.1 \
    --hash=sha256:927b1b88e75fe91bfb2eab755136d80381906b54a34b411209196ebf5e37102c
```

Create `automation/loopx/release-lock.json`:

```json
{
  "schema": "ming_iot_loopx_release_lock_v1",
  "package": "loopx",
  "version": "0.5.1",
  "source": "https://pypi.org/project/loopx/0.5.1/",
  "wheel": {
    "filename": "loopx-0.5.1-py3-none-any.whl",
    "sha256": "927b1b88e75fe91bfb2eab755136d80381906b54a34b411209196ebf5e37102c"
  },
  "sdist": {
    "filename": "loopx-0.5.1.tar.gz",
    "sha256": "69da0d23aabe23b4a0aaf419107856e3e035ff2f0d5d9c3b60fe00acb95d484c"
  },
  "rollbackVersion": "0.5.1"
}
```

- [ ] **Step 2: Ignore every machine-local runtime root**

Append these anchored entries to `.gitignore`:

```gitignore
/.loopx/
/.codex/goals/
/.local/
```

Do not ignore `automation/loopx/`; it is the versioned static Harness.

- [ ] **Step 3: Verify lock parsing and ignore behavior**

Run from the repository root:

```bash
python3 - <<'PY'
import json
from pathlib import Path

lock = json.loads(Path("automation/loopx/release-lock.json").read_text())
assert lock["schema"] == "ming_iot_loopx_release_lock_v1"
assert lock["version"] == "0.5.1"
assert lock["wheel"]["sha256"] == "927b1b88e75fe91bfb2eab755136d80381906b54a34b411209196ebf5e37102c"
assert lock["sdist"]["sha256"] == "69da0d23aabe23b4a0aaf419107856e3e035ff2f0d5d9c3b60fe00acb95d484c"
requirements = Path("automation/loopx/requirements.lock").read_text()
assert "loopx==0.5.1" in requirements
assert lock["wheel"]["sha256"] in requirements
PY

git check-ignore -q --no-index .loopx/registry.json
git check-ignore -q --no-index .codex/goals/ming-iot-framework-v1.md
git check-ignore -q --no-index .local/loopx-venv/bin/loopx
test -z "$(git ls-files .loopx .codex/goals .local)"
```

Expected: every command exits `0`; `git ls-files` produces no output.

- [ ] **Step 4: Prepare the Task 1 checkpoint**

Run:

```bash
git status --short
git diff -- .gitignore automation/loopx/requirements.lock automation/loopx/release-lock.json
```

Expected scope: only the three Task 1 files. Wait for explicit commit authorization.

Suggested commit message after authorization:

```text
build(loopx): lock official bootstrap release
```

---

### Task 2: Build the deterministic static-contract verifier with TDD

**Files:**

- Create: `automation/loopx/check.py`
- Create: `automation/loopx/test_check.py`
- Create: `automation/loopx/authority-catalog.json`
- Create: `automation/loopx/role-contracts.json`
- Create: `automation/loopx/multi-agent-spec.public.json`

- [ ] **Step 1: Write failing verifier tests**

Create `automation/loopx/test_check.py`. Load `check.py` as a subprocess so the test covers the public CLI. Build each fixture inside a temporary Git repository. Include these exact cases:

```python
def test_verify_accepts_the_repository_contract(self):
    result = run_check(self.repo, "verify", "--repo-root", str(self.repo))
    self.assertEqual(result.returncode, 0, result.stderr)
    payload = json.loads(result.stdout)
    self.assertEqual(payload["schema"], "ming_iot_loopx_static_check_v1")
    self.assertEqual(payload["result"], "PASS")

def test_verify_rejects_wrong_loopx_hash(self):
    replace(self.repo / "automation/loopx/requirements.lock", "927b1b", "000000")
    result = run_check(self.repo, "verify", "--repo-root", str(self.repo))
    self.assertNotEqual(result.returncode, 0)
    self.assertIn("RELEASE_LOCK_MISMATCH", result.stderr)

def test_verify_rejects_unknown_authority_role_and_parent_escape(self):
    catalog = load_json(self.repo / "automation/loopx/authority-catalog.json")
    catalog["entries"][0]["role"] = "primary"
    catalog["entries"][0]["path"] = "../private.md"
    write_json(self.repo / "automation/loopx/authority-catalog.json", catalog)
    result = run_check(self.repo, "verify", "--repo-root", str(self.repo))
    self.assertNotEqual(result.returncode, 0)
    self.assertIn("AUTHORITY_CATALOG_INVALID", result.stderr)

def test_verify_rejects_privileged_or_reused_peer_identity(self):
    roles = load_json(self.repo / "automation/loopx/role-contracts.json")
    roles["agentModel"] = "leader_worker_v1"
    write_json(self.repo / "automation/loopx/role-contracts.json", roles)
    launch = load_json(self.repo / "automation/loopx/multi-agent-spec.public.json")
    launch["roles"][1]["agent_id"] = launch["roles"][0]["agent_id"]
    write_json(self.repo / "automation/loopx/multi-agent-spec.public.json", launch)
    result = run_check(self.repo, "verify", "--repo-root", str(self.repo))
    self.assertNotEqual(result.returncode, 0)
    self.assertIn("ROLE_CONTRACT_INVALID", result.stderr)

def test_verify_rejects_tracked_runtime_state(self):
    tracked = self.repo / ".loopx/registry.json"
    tracked.parent.mkdir(parents=True)
    tracked.write_text("{}\n")
    run_git(self.repo, "add", "-f", ".loopx/registry.json")
    result = run_check(self.repo, "verify", "--repo-root", str(self.repo))
    self.assertNotEqual(result.returncode, 0)
    self.assertIn("RUNTIME_STATE_TRACKED", result.stderr)
```

The fixture must copy the five versioned Task 1/2 contract files and initialize Git with a local test identity. Tests must not use the developer's global Git configuration.

- [ ] **Step 2: Run the tests and observe RED**

```bash
python3 -m unittest -v automation/loopx/test_check.py
```

Expected: failure because `automation/loopx/check.py` and the three Task 2 JSON contracts do not exist.

- [ ] **Step 3: Add the authority catalog**

Create `automation/loopx/authority-catalog.json` with this exact top-level contract:

```json
{
  "schema": "ming_iot_authority_catalog_v1",
  "repository": "iot-doc",
  "entries": [
    {
      "path": "亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md",
      "role": "normative"
    },
    {
      "path": "亿级点位高吞吐架构/README.md",
      "role": "guide"
    },
    {
      "path": "docs/superpowers/specs/2026-08-21-loopx-driven-iot-development-control-plane-design.md",
      "role": "normative"
    },
    {
      "path": "docs/superpowers/specs/2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md",
      "role": "normative"
    }
  ]
}
```

Only `normative`, `guide`, `research`, and `historical` are valid roles. Entries must be unique, repository-relative POSIX paths with no absolute path or `..` segment.

- [ ] **Step 4: Add equal-peer role and risk contracts**

Create `automation/loopx/role-contracts.json`:

```json
{
  "schema": "ming_iot_role_contracts_v1",
  "agentModel": "peer_v1",
  "roles": {
    "planner": {
      "repositoryWrite": false,
      "mayApprove": false,
      "mayMerge": false,
      "mayPush": false
    },
    "worker": {
      "repositoryWrite": "approved_worktree_and_write_scopes_only",
      "mayApprove": false,
      "mayMerge": false,
      "mayPush": false
    },
    "reviewer": {
      "repositoryWrite": "assigned_review_artifact_only",
      "mayModifyReviewedCode": false,
      "mayApprove": false,
      "mayMerge": false,
      "mayPush": false
    }
  },
  "riskTopologies": {
    "simple": ["worker"],
    "normal": ["worker", "reviewer"],
    "complex": ["planner", "worker", "reviewer"]
  },
  "userGates": [
    "architecture_change",
    "scope_expansion",
    "merge",
    "push",
    "production_or_sensitive_effect"
  ]
}
```

The verifier must compare these role sets exactly so an Agent cannot silently downgrade a normal or complex task.

- [ ] **Step 5: Add the official visible launch recipe**

Create `automation/loopx/multi-agent-spec.public.json`:

```json
{
  "schema_version": "generic_multi_agent_launch_spec_v0",
  "goal_id": "ming-iot-framework-v1",
  "session_name": "ming-iot-framework-v1-visible",
  "default_reasoning_effort": "high",
  "roles": [
    {
      "lane_id": "planner",
      "agent_id": "ming-planner-peer-01",
      "role_id": "planner",
      "scope": "Read architecture and repository state; propose bounded Todo graphs and handoffs; do not modify repository files or approve work.",
      "skill": {"name": "ming-iot-planner", "source": "skills/planner/SKILL.md"},
      "handoff_hints": ["Create a bounded Worker Todo only after its user Gate is approved."],
      "output_language": "zh"
    },
    {
      "lane_id": "worker",
      "agent_id": "ming-worker-peer-01",
      "role_id": "worker",
      "scope": "Claim approved work only; write inside the Todo worktree and writeScopes; validate and record public-safe evidence; never merge or push.",
      "skill": {"name": "ming-iot-worker", "source": "skills/worker/SKILL.md"},
      "handoff_hints": ["After fresh Evidence, create or unblock an independent Reviewer Todo when the risk topology requires it."],
      "output_language": "zh"
    },
    {
      "lane_id": "reviewer",
      "agent_id": "ming-reviewer-peer-01",
      "role_id": "reviewer",
      "scope": "Independently read the exact diff, authority refs, and Evidence; rerun validation; write only the assigned review artifact; never repair reviewed code.",
      "skill": {"name": "ming-iot-reviewer", "source": "skills/reviewer/SKILL.md"},
      "handoff_hints": ["Reject insufficient proof with concrete findings and a focused repair Todo; never approve your own implementation."],
      "output_language": "zh"
    }
  ]
}
```

The `skill.source` paths are relative to `automation/loopx/`, which is the spec source root expected by the official launcher.

- [ ] **Step 6: Implement only static verification**

Create `automation/loopx/check.py` with two subcommands:

```text
check.py verify   --repo-root PATH
check.py snapshot --repo-root PATH --iot-doc-root PATH --output PATH
```

For `verify`:

1. parse JSON strictly and reject duplicate keys with `object_pairs_hook`;
2. validate exact schema names, Goal ID, session name, peer IDs, role topology, and release digests;
3. validate every relative path before filesystem access;
4. require every referenced role skill to exist;
5. require `.gitignore` to contain the three anchored runtime roots;
6. run `git -c core.hooksPath=/dev/null -c credential.helper= ls-files -- .loopx .codex/goals .local` with `GIT_CONFIG_NOSYSTEM=1` and reject any output;
7. emit one compact JSON object to stdout and typed errors to stderr;
8. never read or write LoopX dynamic state.

Use this public result shape:

```json
{
  "schema": "ming_iot_loopx_static_check_v1",
  "result": "PASS",
  "goalId": "ming-iot-framework-v1",
  "agentModel": "peer_v1",
  "checkedContracts": []
}
```

Keep all output deterministic: sorted paths, no timestamps, no absolute local paths, no environment dumps.

- [ ] **Step 7: Run focused and repeatability tests**

```bash
python3 -m unittest -v automation/loopx/test_check.py
python3 automation/loopx/check.py verify --repo-root . > .local-static-check-1.json
python3 automation/loopx/check.py verify --repo-root . > .local-static-check-2.json
cmp .local-static-check-1.json .local-static-check-2.json
rm .local-static-check-1.json .local-static-check-2.json
```

Expected: all tests PASS and `cmp` reports no difference.

- [ ] **Step 8: Prepare the Task 2 checkpoint**

Show status and the exact five Task 2 files. Wait for explicit commit authorization.

Suggested commit message:

```text
feat(loopx): add static bootstrap contracts
```

---

### Task 3: Add role-local Skills, repository Rules, and MCP policy

**Required sub-skill:** `superpowers:writing-skills`

**Files:**

- Create: `automation/loopx/skills/planner/SKILL.md`
- Create: `automation/loopx/skills/worker/SKILL.md`
- Create: `automation/loopx/skills/reviewer/SKILL.md`
- Create: `automation/loopx/rules/repository-policy.md`
- Create: `automation/loopx/mcp-policy.md`
- Create: `automation/loopx/README.md`
- Modify: `automation/loopx/test_check.py`
- Modify: `automation/loopx/check.py`

- [ ] **Step 1: Extend tests for role-skill and policy boundaries**

Add negative tests that prove:

- a missing `SKILL.md` fails with `ROLE_SKILL_MISSING`;
- a role skill that contains `git push`, `--no-verify`, `production credential`, or raw transcript collection fails with `ROLE_SKILL_FORBIDDEN_CAPABILITY` unless the phrase is explicitly in a prohibition sentence recognized by the checker;
- Planner and Reviewer skills cannot grant reviewed-code writes;
- Worker skill must mention approved worktree, `writeScopes`, validation, Evidence, and no push;
- `repository-policy.md` contains the read-only `../things` rule, `go work sync` prohibition, dirty-worktree preservation, and explicit commit/push authorization;
- `mcp-policy.md` defaults external capabilities to read-only and requires a user Gate plus readback for sensitive writes.

Run and observe RED:

```bash
python3 -m unittest -v automation/loopx/test_check.py
```

Expected: new tests fail because role skills and policy files are absent.

- [ ] **Step 2: Write minimal role-local Skills**

Each Skill must have YAML frontmatter with its exact `name` and a one-sentence `description`. Keep generic LoopX mechanics out; the official kernel already injects `$loopx-project` and `$loopx-doc-registry`.

Planner behavior:

```text
read exact Goal/frontier -> read authority snapshot and Git facts -> propose at most three bounded candidates -> record dependencies/writeScopes/non-goals/acceptance/risks -> create user Gate -> stop
```

Worker behavior:

```text
read fresh quota/Gate/scope -> confirm isolated worktree -> test first -> modify only writeScopes -> run L1/L2/L3 -> write public-safe Evidence -> hand off -> never merge/push
```

Reviewer behavior:

```text
use a different agent identity and Chat -> read exact revision/diff/authority/Evidence -> rerun validation -> write findings and verdict -> reject insufficient proof -> never repair reviewed code
```

Do not copy tmux commands, pane wake mechanics, quota internals, or hidden session handling into these Skills.

- [ ] **Step 3: Write repository and MCP policies**

`repository-policy.md` must define:

- repository/worktree boundaries;
- `../things` permanently read-only;
- no `go work sync`;
- preserve unrelated and dirty changes;
- no auto-delete or auto-rollback after failure;
- checkpoint commits only inside an approved delivery scope and still subject to repository authorization;
- merge and push always require a separate user Gate;
- no `--no-verify` or hook bypass;
- unknown external effect means readback before retry.

`mcp-policy.md` must classify capabilities:

| Capability | Default | Additional requirement |
|---|---|---|
| local repository read | allowed | stay inside declared repository |
| local scoped write | denied until Todo approval | isolated worktree and `writeScopes` |
| public network read | task-dependent | no credentials in logs or Evidence |
| private service/MCP read | denied | explicit capability and minimum credential |
| external write, message, cloud, database | denied | user Gate, stable effect ID, readback |
| production/destructive operation | denied | separate explicit approval and recovery plan |

- [ ] **Step 4: Write the operator README**

Document only official commands and project policy:

- install/upgrade/rollback from the lock;
- `workflow-skills --dry-run`, install, and uninstall readback;
- `doctor`;
- connect/status/readback;
- visible launcher preview/attach/stop/retry;
- restart recovery order;
- legacy `.graph` rollback boundary;
- no custom `company-dev` or project Runner.

- [ ] **Step 5: Make policy validation structural, not phrase-counting**

Update `check.py` so it verifies required headings and normalized policy clauses. Keep the forbidden-capability test conservative: reject positive grants such as `may run git push`; allow explicit prohibitions such as `never run git push`. Do not build an NLP policy engine.

- [ ] **Step 6: Verify**

```bash
python3 -m unittest -v automation/loopx/test_check.py
python3 automation/loopx/check.py verify --repo-root .
```

Expected: PASS. Review every Skill against `superpowers:writing-skills` before the checkpoint.

- [ ] **Step 7: Prepare the Task 3 checkpoint**

Show status and wait for explicit commit authorization.

Suggested commit message:

```text
docs(loopx): define peer role harness
```

---

### Task 4: Bind the authority catalog to committed `iot-doc` content

**Files:**

- Modify: `automation/loopx/check.py`
- Modify: `automation/loopx/test_check.py`
- Runtime output only: `.local/loopx/architecture-snapshot.json`
- Cross-repository prerequisite: the two approved `iot-doc` design specs must be tracked and committed before snapshot generation

- [ ] **Step 1: Add failing snapshot tests**

Use two temporary Git repositories (`ming-iot` and `iot-doc`) and assert:

```python
def test_snapshot_binds_clean_committed_authorities_and_is_reproducible(self):
    first = run_snapshot(self.ming, self.docs, self.out1)
    second = run_snapshot(self.ming, self.docs, self.out2)
    self.assertEqual(first.returncode, 0, first.stderr)
    self.assertEqual(self.out1.read_bytes(), self.out2.read_bytes())
    payload = load_json(self.out1)
    self.assertEqual(payload["schema"], "architecture_snapshot_v1")
    self.assertEqual(payload["goalId"], "ming-iot-framework-v1")
    self.assertEqual(payload["repositories"]["iot-doc"]["revision"], docs_head(self.docs))

def test_snapshot_rejects_untracked_authority(self):
    untracked = self.docs / "untracked-authority.md"
    untracked.write_text("not committed\n")
    point_first_authority_at(self.ming, "untracked-authority.md")
    result = run_snapshot(self.ming, self.docs, self.out1)
    self.assertNotEqual(result.returncode, 0)
    self.assertIn("AUTHORITY_NOT_TRACKED", result.stderr)

def test_snapshot_rejects_modified_authority(self):
    authority = first_authority_path(self.ming, self.docs)
    authority.write_text(authority.read_text() + "worktree-only\n")
    result = run_snapshot(self.ming, self.docs, self.out1)
    self.assertNotEqual(result.returncode, 0)
    self.assertIn("AUTHORITY_WORKTREE_DIRTY", result.stderr)

def test_snapshot_rejects_missing_authority(self):
    first_authority_path(self.ming, self.docs).unlink()
    result = run_snapshot(self.ming, self.docs, self.out1)
    self.assertNotEqual(result.returncode, 0)
    self.assertIn("AUTHORITY_MISSING", result.stderr)

def test_snapshot_excludes_machine_paths_and_timestamps(self):
    result = run_snapshot(self.ming, self.docs, self.out1)
    self.assertEqual(result.returncode, 0, result.stderr)
    text = self.out1.read_text()
    payload = json.loads(text)
    self.assertNotIn(str(self.ming), text)
    self.assertNotIn(str(self.docs), text)
    self.assertNotIn("generatedAt", payload)
```

Run and observe RED:

```bash
python3 -m unittest -v automation/loopx/test_check.py
```

- [ ] **Step 2: Implement `snapshot`**

The command must:

1. resolve both repository roots with `git rev-parse --show-toplevel`;
2. require every catalog entry to be tracked at `HEAD`;
3. compare worktree bytes with `git show HEAD:path` and fail on a difference;
4. compute SHA-256 over the committed bytes;
5. include repository identity, exact `HEAD`, role, path, and digest;
6. sort entries bytewise;
7. write atomically through a sibling temporary file and `os.replace`;
8. omit timestamps, usernames, home directories, transcripts, credentials, and absolute paths.

Use this shape:

```json
{
  "schema": "architecture_snapshot_v1",
  "goalId": "ming-iot-framework-v1",
  "repositories": {
    "iot-doc": {"revision": "40-hex-git-object"},
    "ming-iot": {"revision": "40-hex-git-object"}
  },
  "authorities": [
    {"path": "repository-relative.md", "role": "normative", "sha256": "64-hex-digest"}
  ]
}
```

- [ ] **Step 3: Stop if the approved specs are not committed**

Run:

```bash
iot_doc_root="$(dirname "$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")")/iot-doc"
test -d "$iot_doc_root/.git"
git -C "$iot_doc_root" status --short -- \
  docs/superpowers/specs/2026-08-21-loopx-driven-iot-development-control-plane-design.md \
  docs/superpowers/specs/2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md
git -C "$iot_doc_root" ls-files --error-unmatch \
  docs/superpowers/specs/2026-08-21-loopx-driven-iot-development-control-plane-design.md \
  docs/superpowers/specs/2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md
```

Expected: no status output and both paths are tracked. If not, show only those two approved files and wait for explicit authorization to commit them in `iot-doc`. Do not snapshot uncommitted design text and do not include unrelated `iot-doc` changes.

- [ ] **Step 4: Generate and reproduce the real snapshot**

```bash
mkdir -p .local/loopx
iot_doc_root="$(dirname "$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")")/iot-doc"
python3 automation/loopx/check.py snapshot \
  --repo-root . \
  --iot-doc-root "$iot_doc_root" \
  --output .local/loopx/architecture-snapshot.json
cp .local/loopx/architecture-snapshot.json .local/loopx/architecture-snapshot.first.json
python3 automation/loopx/check.py snapshot \
  --repo-root . \
  --iot-doc-root "$iot_doc_root" \
  --output .local/loopx/architecture-snapshot.json
cmp .local/loopx/architecture-snapshot.first.json .local/loopx/architecture-snapshot.json
```

Expected: `cmp` succeeds; neither snapshot is tracked.

- [ ] **Step 5: Run all verifier tests**

```bash
python3 -m unittest -v automation/loopx/test_check.py
python3 automation/loopx/check.py verify --repo-root .
test -z "$(git ls-files .local/loopx)"
```

- [ ] **Step 6: Prepare the Task 4 checkpoint**

Only `check.py` and `test_check.py` belong to the `ming-iot` checkpoint. Wait for explicit authorization.

Suggested commit message:

```text
feat(loopx): attest architecture authority snapshot
```

---

### Task 5: Freeze legacy Graph and switch the static Harness atomically

**Files:**

- Create: `automation/loopx/legacy-graph-freeze.json`
- Modify: `AGENTS.md`
- Modify: `.githooks/pre-push`
- Modify: `backend/tools/README.md`
- Modify: `automation/loopx/test_check.py`
- Modify: `automation/loopx/check.py`

- [ ] **Step 1: Add failing cutover tests**

Add tests that require:

- `legacy-graph-freeze.json` identifies `403b9a6d6b7e2c60a1ff96ef0f681da272d04c36` as the last pre-cutover revision and names LoopX as successor;
- `.graph/registry.yaml`, both Loop YAML files, the schema, and all committed historical Evidence/Review are unchanged from that revision;
- `AGENTS.md` points to `automation/loopx/` and exact Goal `ming-iot-framework-v1`, while preserving commit/push authorization, `../things` read-only, and `go work sync` prohibition;
- `.githooks/pre-push` invokes only `python3 "$repo_root/automation/loopx/check.py" verify --repo-root "$repo_root"` and never invokes `graphctl ci`;
- `backend/tools/README.md` labels graphctl lifecycle as historical and does not claim `.graph` owns new work.

Run and observe RED:

```bash
python3 -m unittest -v automation/loopx/test_check.py
```

- [ ] **Step 2: Write the freeze receipt**

Create:

```json
{
  "schema": "ming_iot_legacy_graph_freeze_v1",
  "status": "READ_ONLY_HISTORICAL",
  "sourceRevision": "403b9a6d6b7e2c60a1ff96ef0f681da272d04c36",
  "successorControlPlane": "LoopX",
  "successorGoalId": "ming-iot-framework-v1",
  "frozenPaths": [
    ".graph/registry.yaml",
    ".graph/schemas/loop-v1.schema.json",
    ".graph/loops/G0-rd-graph-control-plane.yaml",
    ".graph/loops/L0-things-baseline.yaml",
    "docs/superpowers/evidence/G0-rd-graph-control-plane",
    "docs/superpowers/evidence/L0-things-baseline",
    "docs/superpowers/reviews/G0-rd-graph-control-plane"
  ],
  "newLifecycleWritesAllowed": false,
  "rollbackRequiresUserApproval": true
}
```

The verifier must compare each frozen tracked blob or tree against the source revision. A later documentation-only compatibility note may be added outside the frozen paths; no frozen history is rewritten.

- [ ] **Step 3: Replace `AGENTS.md` with a short static entrypoint**

The new file must state, in order:

1. exact Goal and LoopX as the sole dynamic control plane;
2. links to `automation/loopx/README.md`, repository policy, MCP policy, role contracts, and authority catalog;
3. before-work commands: static `check`, LoopX `status`, exact `quota should-run`, and Todo scope/Gate readback;
4. unchanged hard boundaries: no `go work sync`, `../things` read-only, preserve unrelated changes, no commit/push without explicit authorization;
5. completion rule: deterministic Evidence, independent Reviewer when required, then user Gate;
6. `.graph` is historical and must not receive new lifecycle state.

Do not copy a Loop list, gate command catalog, or long LoopX tutorial into `AGENTS.md`.

- [ ] **Step 4: Replace the slow lifecycle pre-push hook**

Use this exact hook:

```bash
#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
exec python3 "$repo_root/automation/loopx/check.py" verify --repo-root "$repo_root"
```

Retain executable mode. This hook checks static safety in seconds; fresh pre-merge Evidence and user push approval remain separate LoopX/user Gates.

- [ ] **Step 5: Mark graphctl lifecycle historical**

At the start of the existing Graph section in `backend/tools/README.md`, add a prominent cutover note:

- G0/L0 history remains readable at the freeze revision;
- new dynamic work belongs only to LoopX;
- graphctl may still be run for historical diagnosis or reusable deterministic checks;
- graphctl output cannot create or complete a new delivery Loop;
- old pre-push lifecycle orchestration is retired.

Do not delete graphctl source or tests in this bootstrap.

- [ ] **Step 6: Verify cutover as one unit**

```bash
python3 -m unittest -v automation/loopx/test_check.py
python3 automation/loopx/check.py verify --repo-root .
bash -n .githooks/pre-push
test -x .githooks/pre-push
git diff --exit-code 403b9a6d6b7e2c60a1ff96ef0f681da272d04c36 -- \
  .graph \
  docs/superpowers/evidence/G0-rd-graph-control-plane \
  docs/superpowers/evidence/L0-things-baseline \
  docs/superpowers/reviews/G0-rd-graph-control-plane
```

Expected: PASS and no frozen-path diff.

- [ ] **Step 7: User cutover Gate and checkpoint**

Show the complete Task 5 diff, the static check output, and this exact post-commit rollback recipe:

```bash
cutover_commit=$(git log -1 --format=%H --grep='^feat(loopx): cut over local development control plane$')
test -n "$cutover_commit"
git revert --no-commit "$cutover_commit"
```

Do not run the rollback during normal execution. Wait for explicit user approval before committing the cutover.

Suggested commit message:

```text
feat(loopx): cut over local development control plane
```

---

### Task 6: Install the pinned LoopX runtime and workflow skills locally

**Files:**

- Runtime only: `.local/loopx-venv/`
- Host-managed skills: the directory reported by LoopX, normally `~/.codex/skills/`
- Modify after readback: `docs/superpowers/evidence/2026-08-23-loopx-personal-framework-bootstrap.md`

- [ ] **Step 1: Install the wheel in the ignored local venv**

```bash
python3 -m venv .local/loopx-venv
.local/loopx-venv/bin/python -m pip install \
  --disable-pip-version-check \
  --no-deps \
  --require-hashes \
  -r automation/loopx/requirements.lock
.local/loopx-venv/bin/loopx version
```

Expected: the version readback contains `0.5.1`. `--no-deps` is valid because LoopX `0.5.1` declares no required runtime dependencies.

- [ ] **Step 2: Preview workflow-skill installation**

```bash
.local/loopx-venv/bin/loopx --format json workflow-skills --dry-run \
  --cli-bin "$PWD/.local/loopx-venv/bin/loopx" \
  > .local/loopx/workflow-skills-preview.json
```

Read the JSON and report the exact `skills_dir`, source distribution version, `install_required`, and planned command. Do not execute yet.

- [ ] **Step 3: Obtain the host-skill mutation Gate**

Installing workflow skills modifies host-managed files outside the repository. Show the preview and wait for explicit user approval.

After approval:

```bash
.local/loopx-venv/bin/loopx --format json workflow-skills --install \
  --cli-bin "$PWD/.local/loopx-venv/bin/loopx" \
  > .local/loopx/workflow-skills-install.json
```

Expected: `ok=true`, `operation=install`, `after.ready=true`, and distribution version `0.5.1`. Preserve the returned rollback command in Evidence.

- [ ] **Step 4: Run official diagnostics and the model-free TUI bootstrap bundle**

```bash
.local/loopx-venv/bin/loopx --format json doctor > .local/loopx/doctor.json
.local/loopx-venv/bin/loopx codex-cli-tui-bootstrap-smoke-bundle \
  --project . \
  --goal-id ming-iot-framework-v1 \
  --agent-id ming-worker-peer-01 \
  --cli-bin "$PWD/.local/loopx-venv/bin/loopx" \
  > .local/loopx/tui-bootstrap-smoke.json
python3 automation/loopx/check.py verify --repo-root .
```

Expected: doctor reports healthy installation/skills and the bundle is generated without starting a model process. If doctor fails, classify the failure and stop; do not edit LoopX or bypass the check.

- [ ] **Step 5: Start the bootstrap Evidence document**

Create `docs/superpowers/evidence/2026-08-23-loopx-personal-framework-bootstrap.md` with:

- release version and wheel digest;
- workflow-skill preview/install readback and rollback command;
- doctor result;
- static-check command/result;
- TUI bootstrap bundle result;
- exact `ming-iot` and `iot-doc` revisions from the architecture snapshot;
- explicit status `IN_PROGRESS` and `phaseOneComplete: false`.

Do not paste raw environment data, absolute home paths, credentials, transcripts, or full logs.

- [ ] **Step 6: Prepare the Task 6 checkpoint**

Only the compact Evidence Markdown may be tracked. Confirm `.local/` remains ignored and wait for explicit commit authorization.

Suggested commit message:

```text
docs(loopx): record pinned runtime qualification
```

---

### Task 7: Connect the exact Goal and register fresh equal peers

**Files:**

- Runtime only: `.loopx/`, `.codex/goals/`, host LoopX runtime
- Modify: `docs/superpowers/evidence/2026-08-23-loopx-personal-framework-bootstrap.md`

- [ ] **Step 1: Preview the exact Goal connection**

```bash
LOOPX="$PWD/.local/loopx-venv/bin/loopx"
"$LOOPX" --format json connect \
  --project . \
  --goal-id ming-iot-framework-v1 \
  --objective '按 iot-doc 的架构与语义，实现完整、可验证、可持续演进的工业物联网平台 ming-iot。' \
  --domain industrial-iot-platform \
  --role controller \
  --fine-grained \
  --no-onboarding-scan \
  --codex-app-heartbeat no \
  --dry-run \
  > .local/loopx/connect-preview.json
```

Verify the preview names only the exact Goal, local ignored state, and shared LoopX registry projection. It must not create onboarding implementation Todos or modify Git.

- [ ] **Step 2: User Goal-creation Gate**

Show the preview. After explicit approval, rerun the same command without `--dry-run` and save the readback:

```bash
"$LOOPX" --format json connect \
  --project . \
  --goal-id ming-iot-framework-v1 \
  --objective '按 iot-doc 的架构与语义，实现完整、可验证、可持续演进的工业物联网平台 ming-iot。' \
  --domain industrial-iot-platform \
  --role controller \
  --fine-grained \
  --no-onboarding-scan \
  --codex-app-heartbeat no \
  > .local/loopx/connect.json
```

- [ ] **Step 3: Preview and register three fresh identities**

For each exact identity, preview first:

```bash
for agent in ming-planner-peer-01 ming-worker-peer-01 ming-reviewer-peer-01; do
  "$LOOPX" --format json register-agent \
    --goal-id ming-iot-framework-v1 \
    --agent-id "$agent" \
    --require-new \
    > ".local/loopx/register-${agent}-preview.json"
done
```

Show the previews and obtain one user Gate for these three public-safe identities. After approval:

```bash
for agent in ming-planner-peer-01 ming-worker-peer-01 ming-reviewer-peer-01; do
  "$LOOPX" --format json register-agent \
    --goal-id ming-iot-framework-v1 \
    --agent-id "$agent" \
    --require-new \
    --execute \
    > ".local/loopx/register-${agent}.json"
done
```

Every result must report `ok=true`, `changed=true`, `written=true`, successful global sync, and verified source/global readback. If an ID already exists, stop; do not silently takeover it.

- [ ] **Step 4: Read back canonical state**

```bash
"$LOOPX" --format json registry > .local/loopx/registry-readback.json
"$LOOPX" --format json status > .local/loopx/status-readback.json
"$LOOPX" --format json todo list --goal-id ming-iot-framework-v1 \
  > .local/loopx/todos-readback.json
"$LOOPX" --format json history --goal-id ming-iot-framework-v1 \
  > .local/loopx/history-readback.json
for agent in ming-planner-peer-01 ming-worker-peer-01 ming-reviewer-peer-01; do
  "$LOOPX" --format json quota should-run \
    --goal-id ming-iot-framework-v1 \
    --agent-id "$agent" \
    > ".local/loopx/quota-${agent}.json"
done
test -z "$(git ls-files .loopx .codex/goals .local)"
```

Expected: one exact Goal, three distinct registered peers, no implicit leader, and no tracked runtime state.

- [ ] **Step 5: Update Evidence and checkpoint**

Record compact readbacks, identity separation, and any Gate/quiet state. Never copy registry files or raw runtime state into Git. Wait for explicit commit authorization.

Suggested commit message:

```text
docs(loopx): attest goal and peer registration
```

---

### Task 8: Run a visible non-business qualification loop

**Files:**

- Runtime only: LoopX Todo state and tmux session
- Modify by Worker only: `docs/superpowers/evidence/2026-08-23-loopx-personal-framework-bootstrap.md`
- Create by Reviewer only: `docs/superpowers/reviews/2026-08-23-loopx-personal-framework-bootstrap-review.md`

- [ ] **Step 1: Create one bounded Worker qualification Todo**

Preview, inspect, then execute only after user approval:

```bash
"$LOOPX" --format json todo add \
  --goal-id ming-iot-framework-v1 \
  --role agent \
  --text 'Qualify the visible LoopX bootstrap and append compact static-check evidence; do not modify business code.' \
  --task-class advancement_task \
  --action-kind qualify_visible_bootstrap \
  --task-domain tooling \
  --task-repository git:gitee.com/wangjunbo/ming-iot \
  --continuation-policy independent_handoff \
  --required-write-scope docs/superpowers/evidence/2026-08-23-loopx-personal-framework-bootstrap.md \
  --required-capability shell \
  --required-capability filesystem_write \
  --validation-command-json '["python3","automation/loopx/check.py","verify","--repo-root","."]' \
  --validation-label loopx-bootstrap-static-contract \
  --claimed-by ming-worker-peer-01 \
  --excluded-agent ming-reviewer-peer-01 \
  --agent-id ming-planner-peer-01 \
  --dry-run
```

After approval, replace `--dry-run` with `--execute`. Capture the returned Todo ID in `.local/loopx/worker-qualification-todo-id.txt`.

- [ ] **Step 2: Preview the official multi-agent launcher**

```bash
"$LOOPX" --format json multi-agent launch \
  --spec automation/loopx/multi-agent-spec.public.json \
  --workspace "$PWD" \
  --auto-wake \
  > .local/loopx/multi-agent-launch-preview.json
```

Expected: three visible lanes, the exact session name, resolved role skills, no process launch, no LoopX mutation, and no quota spend.

- [ ] **Step 3: User visible-launch Gate**

Show the preview, current Todo, exact Codex binary, and stop command. After explicit approval:

```bash
"$LOOPX" --format json multi-agent launch \
  --spec automation/loopx/multi-agent-spec.public.json \
  --workspace "$PWD" \
  --codex-bin "$(command -v codex)" \
  --auto-wake \
  --execute \
  > .local/loopx/multi-agent-launch.json
tmux attach -t ming-iot-framework-v1-visible
```

The operator may detach with `Ctrl-b d`. Do not use `--replace-existing` on first launch. The user must be able to interrupt and type into every real Codex TUI pane.

- [ ] **Step 4: Verify the Worker outcome without trusting the pane**

After the Worker claims/completes its Todo, run independently:

```bash
python3 automation/loopx/check.py verify --repo-root .
"$LOOPX" --format json todo list --goal-id ming-iot-framework-v1 \
  > .local/loopx/todos-after-worker.json
"$LOOPX" --format json evidence-log \
  --goal-id ming-iot-framework-v1 \
  --agent-id ming-worker-peer-01 \
  --thin \
  > .local/loopx/worker-evidence.json
git diff -- docs/superpowers/evidence/2026-08-23-loopx-personal-framework-bootstrap.md
```

Expected: only the assigned Evidence file changed, the static verifier independently passes, and public-safe evidence exists. If the Worker touches any other path, stop and classify `SCOPE_VIOLATION`.

- [ ] **Step 5: Create an independent Reviewer Todo**

Preview and then execute after user approval:

```bash
"$LOOPX" --format json todo add \
  --goal-id ming-iot-framework-v1 \
  --role agent \
  --text 'Independently review the LoopX bootstrap contracts and evidence; write only the assigned review artifact.' \
  --task-class advancement_task \
  --action-kind independent_review \
  --task-domain validation \
  --task-repository git:gitee.com/wangjunbo/ming-iot \
  --continuation-policy independent_handoff \
  --required-write-scope docs/superpowers/reviews/2026-08-23-loopx-personal-framework-bootstrap-review.md \
  --required-capability shell \
  --required-capability filesystem_write \
  --validation-command-json '["python3","automation/loopx/check.py","verify","--repo-root","."]' \
  --validation-label loopx-bootstrap-independent-review \
  --claimed-by ming-reviewer-peer-01 \
  --excluded-agent ming-worker-peer-01 \
  --agent-id ming-planner-peer-01 \
  --dry-run
```

After approval, use `--execute`, then send one targeted wake:

```bash
"$LOOPX" multi-agent wake \
  --session-name ming-iot-framework-v1-visible \
  --lane reviewer \
  --execute
```

The Review must record exact revision/diff, commands rerun, findings ordered by severity, verdict `PASS` or `REJECT`, and remaining risks. Reviewer may not fix implementation files.

- [ ] **Step 6: Verify identity and Chat separation**

```bash
"$LOOPX" --format json evidence-log \
  --goal-id ming-iot-framework-v1 \
  --agent-id ming-reviewer-peer-01 \
  --thin \
  > .local/loopx/reviewer-evidence.json
git diff -- docs/superpowers/reviews/2026-08-23-loopx-personal-framework-bootstrap-review.md
```

Confirm Worker and Reviewer have different agent IDs and separate TUI panes/Chats. A Worker statement cannot substitute for the Reviewer artifact.

- [ ] **Step 7: Stop safely and prepare the checkpoint**

```bash
tmux kill-session -t ming-iot-framework-v1-visible
"$LOOPX" --format json status > .local/loopx/status-after-visible-run.json
```

Expected: the tmux session stops but canonical Goal/Todo/Evidence remains. Show the Worker Evidence diff and Reviewer diff; wait for explicit commit authorization.

Suggested commit message:

```text
test(loopx): qualify visible peer workflow
```

---

### Task 9: Prove restart recovery and no-transcript handoff

**Files:**

- Runtime only: a new tmux session and LoopX readbacks
- Modify: bootstrap Evidence and Review only if new findings arise

- [ ] **Step 1: Capture a public-safe pre-restart checkpoint**

```bash
"$LOOPX" --format json status > .local/loopx/recovery-before-status.json
"$LOOPX" --format json todo list --goal-id ming-iot-framework-v1 \
  > .local/loopx/recovery-before-todos.json
git status --short > .local/loopx/recovery-before-git-status.txt
```

Do not preserve or copy transcript/session data.

- [ ] **Step 2: Relaunch fresh Codex TUI processes from canonical state**

```bash
test -z "$(tmux list-sessions -F '#S' 2>/dev/null | grep '^ming-iot-framework-v1-visible$' || true)"
"$LOOPX" --format json multi-agent launch \
  --spec automation/loopx/multi-agent-spec.public.json \
  --workspace "$PWD" \
  --codex-bin "$(command -v codex)" \
  --auto-wake \
  --execute \
  > .local/loopx/recovery-launch.json
tmux attach -t ming-iot-framework-v1-visible
```

These are fresh Chats using exact registered identities for an explicit reattach, not hidden transcript resume. In each pane, the first action must read fresh LoopX quota/frontier and Git state.

- [ ] **Step 3: Verify recovery invariants**

After detach:

```bash
"$LOOPX" --format json status > .local/loopx/recovery-after-status.json
"$LOOPX" --format json todo list --goal-id ming-iot-framework-v1 \
  > .local/loopx/recovery-after-todos.json
python3 automation/loopx/check.py verify --repo-root .
git status --short
```

Prove:

- exact Goal recovered without duplicate creation;
- Todo lifecycle and Evidence are consistent before/after restart;
- no transcript was copied;
- no out-of-scope file changed;
- no new `.graph` lifecycle write occurred;
- user can still interrupt and stop panes.

- [ ] **Step 4: Stop and record recovery Evidence**

```bash
tmux kill-session -t ming-iot-framework-v1-visible
```

Append a compact recovery section to the bootstrap Evidence. Mark this as process/Chat recovery only; WSL reboot and real delivery worktree recovery remain acceptance items for the first real delivery Loop.

- [ ] **Step 5: Prepare the Task 9 checkpoint**

Show status and wait for explicit commit authorization if Evidence changed.

Suggested commit message:

```text
docs(loopx): attest fresh chat recovery
```

---

### Task 10: Close the bootstrap slice and propose the first real delivery Loop

**Files:**

- Modify: `docs/superpowers/evidence/2026-08-23-loopx-personal-framework-bootstrap.md`
- Modify: `docs/superpowers/reviews/2026-08-23-loopx-personal-framework-bootstrap-review.md`
- Create: `docs/superpowers/proposals/2026-08-23-first-real-delivery-loop.md`

- [ ] **Step 1: Run final deterministic verification**

```bash
python3 -m unittest -v automation/loopx/test_check.py
python3 automation/loopx/check.py verify --repo-root .
cd backend/tools
GOWORK=off go test ./cmd/graphctl ./internal/graphctl/... -count=1
GOWORK=off go test -race ./cmd/graphctl ./internal/graphctl/... -count=1
GOWORK=off go vet ./cmd/graphctl ./internal/graphctl/...
cd ../..
git diff --check
git status --short
```

Expected: all commands PASS. The graphctl suite proves the historical tool was not broken; it does not make graphctl the new lifecycle owner.

- [ ] **Step 2: Finalize independent Review**

The Reviewer must inspect the exact final diff and update the Review with:

- reviewed revision/base and complete file list;
- design clauses covered and not covered;
- L1/L2/L3/L4 checks applicable to this bootstrap;
- identity separation and restart evidence;
- typed findings and verdict;
- explicit statement that no IoT business Loop has been completed.

If verdict is `REJECT`, create a focused repair Todo and return to the relevant task. Do not let the implementation Agent rewrite the Review verdict.

- [ ] **Step 3: Finalize Evidence with honest status**

Set:

```text
bootstrapStatus: BOOTSTRAP_QUALIFIED
phaseOneComplete: false
realDeliveryLoopComplete: false
legacyGraphDynamicWrites: frozen
dynamicControlPlane: LoopX
```

Include exact commands, tool versions, Git revisions, compact result digests, user Gates, known limitations, rollback steps, and links to the independent Review. Do not include raw transcripts or full runtime files.

- [ ] **Step 4: Produce a ranked first-real-loop proposal**

Create `docs/superpowers/proposals/2026-08-23-first-real-delivery-loop.md`. Analyze committed `iot-doc`, current `ming-iot`, existing L0 Things baseline, and dependency order. Produce at most three ranked candidates. Every candidate must include:

```yaml
deliveryLoopId: stable-public-id
objective: one bounded business outcome
authorityRefs: []
dependencies: []
writeScopes: []
nonGoals: []
acceptance: []
fourLayerVerification: []
risks: []
proposedRiskLevel: simple | normal | complex
proposedAgentLanes: []
```

The document must recommend one candidate with evidence-backed reasoning, but end at `WAITING_USER_SELECTION`. It must not create implementation Todos or grant Worker write permission.

- [ ] **Step 5: User bootstrap acceptance Gate**

Present:

- final static/full test results;
- LoopX Goal/peer/status readback;
- independent Review verdict;
- bootstrap Evidence;
- recovery limitations;
- ranked delivery-loop proposal;
- exact changed-file list.

Ask the user separately to:

1. accept or reject `BOOTSTRAP_QUALIFIED`;
2. select/modify/defer the first real delivery Loop;
3. authorize or decline the final checkpoint commit;
4. later authorize merge and push as separate actions.

- [ ] **Step 6: Prepare the final checkpoint**

Run:

```bash
git status --short
git diff --check
git diff --stat
git diff --cached --name-only
```

Wait for explicit commit authorization.

Suggested commit message:

```text
docs(loopx): close bootstrap qualification
```

Do not merge or push in this task.

---

## Plan acceptance matrix

| Approved design requirement | Covered here | Deferred deliberately |
|---|---:|---:|
| official pinned LoopX and workflow skills | yes | no |
| exact Goal and peer identities | yes | no |
| static Harness and authority snapshot | yes | no |
| visible Runner dry-run/launch/attach/stop | yes | no |
| Worker Evidence and independent Reviewer | yes, bootstrap scope | real business scope |
| Chat/process restart without transcript | yes, bootstrap scope | WSL reboot + business worktree |
| `.graph` read-only cutover | yes | no |
| no custom Runner/CLI/fork | yes | no |
| deterministic Headless fake-host qualification | no | before any Headless adoption |
| real IoT delivery Loop | proposal only | next separately approved plan |
| phase-one reusable company baseline | no | after real Loop Evidence |
| Multica/company platform | no | phase two |

## Final success statement allowed by this plan

Only this statement is permitted after all tasks pass and the user accepts the final Gate:

```text
ming-iot 已完成 LoopX 个人研发框架的控制面引导与可见多 Agent 资格验证；旧 Graph 已只读冻结，首个真实 IoT 交付 Loop 等待用户选定。第一阶段尚未完成。
```
