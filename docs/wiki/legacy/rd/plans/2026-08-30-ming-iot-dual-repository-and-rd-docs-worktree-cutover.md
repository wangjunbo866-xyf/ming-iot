# ming-iot 双仓库管理与研发文档 Worktree 切换实施计划

> **智能体执行要求：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，按任务逐项实施本计划。各步骤使用复选框（`- [ ]`）跟踪状态。

**目标：** 无损收口两个旧研发文档 worktree，将正式材料迁入 `ming-iot-rd-docs/master`，并把后续本地 worktree 根目录切换为仓库内忽略的 `.worktrees/`。

**架构：** `ming-iot` 与 `ming-iot-rd-docs` 采用正式双仓库管理并共享统一业务 Goal；写入型 Todo 通过各自稳定的 `task_repository` 路由。迁移先恢复四个独有文件，再对子目录索引做语义并集，经过内容摘要、Authority Catalog 和 Git 边界校验后才允许提交、推送和删除旧 worktree。

**技术栈：** Git linked worktree、Markdown、YAML、Python 3 `unittest`、LoopX CLI、SHA-256

---

**对应规格：** `specs/2026-08-30-ming-iot-dual-repository-and-rd-docs-worktree-cutover-design.md`

## 文件职责

- 新建：`plans/2026-08-28-kafka-foundation-contract-sdk.md`——恢复 Kafka 基础契约与 SDK 的实施计划。
- 新建：`specs/2026-08-28-kafka-foundation-contract-sdk-activation-design.md`——恢复 Kafka 基础契约与 SDK 激活规格。
- 新建：`evidence/2026-08-27-ming-iot-local-share-migration-activation/source-freeze.json`——恢复 Share 来源冻结机器证据。
- 新建：`specs/2026-08-27-ming-iot-local-share-migration-activation-design.md`——恢复 Share 迁移激活规格。
- 修改：`meta/DOC_REGISTRY.yaml`——保留当前条目，并登记恢复后的 Share/Kafka 正式资料。
- 修改：`.gitignore`——忽略仓库内本地 worktree 池 `/.worktrees/`。
- 修改：`AGENTS.md`——固化双仓库身份、worktree 根目录和文档 Todo 写入规则。
- 新建：`specs/2026-08-30-ming-iot-dual-repository-and-rd-docs-worktree-cutover-design.md`——记录已批准设计。
- 新建：`plans/2026-08-30-ming-iot-dual-repository-and-rd-docs-worktree-cutover.md`——记录本实施计划。

### Task 1：冻结迁移前状态和来源摘要

**文件：**

- 读取：`../.rd-docs-worktrees/kafka-foundation-activation/**`
- 读取：`../.rd-docs-worktrees/local-share-migration-activation/**`
- 读取：`meta/DOC_REGISTRY.yaml`

- [ ] **Step 1：确认主工作区、远端和旧 worktree 精确状态**

运行：

```bash
git status --short --branch
git remote -v
git worktree list --porcelain
git rev-list --left-right --count master...kafka-foundation-activation-spec
git rev-list --left-right --count master...local-share-migration-activation-spec
```

预期：

- 主工作区只包含本次规格和计划的预期新增；
- `origin` 为 `https://gitee.com/ming-industry/ming-iot-rd-docs.git`；
- 两个旧分支均没有领先 `master` 的提交；
- 两个旧 worktree 的路径与本计划一致。

- [ ] **Step 2：重新计算四个独有文件摘要**

运行：

```bash
sha256sum \
  ../.rd-docs-worktrees/kafka-foundation-activation/plans/2026-08-28-kafka-foundation-contract-sdk.md \
  ../.rd-docs-worktrees/kafka-foundation-activation/specs/2026-08-28-kafka-foundation-contract-sdk-activation-design.md \
  ../.rd-docs-worktrees/local-share-migration-activation/evidence/2026-08-27-ming-iot-local-share-migration-activation/source-freeze.json \
  ../.rd-docs-worktrees/local-share-migration-activation/specs/2026-08-27-ming-iot-local-share-migration-activation-design.md
```

预期按顺序得到：

```text
73fd412baf7044e7f0c8b2ce1263495ee8a7f32b5346c1c8bed860b2aba1025a
850feee4b8ab062b7d853841c36e3d6d057c671f1477cf12f2fb7d0b503d1607
0936ab26396be464b6341f7ac69f8e878009ed3431a28ceacaf841516db22af6
63cc642c997aad44c0da92d04f885baa4d65ab1c59bd11601fd5b4a1e6cdf70d
```

任一摘要变化都停止迁移并重新盘点，不沿用后续删除授权。

### Task 2：迁入四个独有文件

**文件：**

- 新建：`plans/2026-08-28-kafka-foundation-contract-sdk.md`
- 新建：`specs/2026-08-28-kafka-foundation-contract-sdk-activation-design.md`
- 新建：`evidence/2026-08-27-ming-iot-local-share-migration-activation/source-freeze.json`
- 新建：`specs/2026-08-27-ming-iot-local-share-migration-activation-design.md`

- [ ] **Step 1：使用补丁按来源字节内容创建四个目标文件**

从 Task 1 的四个精确来源读取正文，通过 `apply_patch` 在主工作区创建同名目标文件。禁止使用递归复制，禁止把来源 worktree 的 `.git`、缓存或其他未登记内容带入。

- [ ] **Step 2：验证目标摘要与来源一致**

运行：

```bash
sha256sum \
  plans/2026-08-28-kafka-foundation-contract-sdk.md \
  specs/2026-08-28-kafka-foundation-contract-sdk-activation-design.md \
  evidence/2026-08-27-ming-iot-local-share-migration-activation/source-freeze.json \
  specs/2026-08-27-ming-iot-local-share-migration-activation-design.md
```

预期摘要与 Task 1 的四个冻结值逐项相同。

### Task 3：语义合并 Authority Catalog 并固化仓库规则

**文件：**

- 修改：`meta/DOC_REGISTRY.yaml`
- 修改：`.gitignore`
- 修改：`AGENTS.md`

- [ ] **Step 1：向 DOC_REGISTRY 增加三个有效入口**

在 `topic_authority:` 中保留全部现有条目，并加入：

```yaml
  local_share_migration_activation: specs/2026-08-27-ming-iot-local-share-migration-activation-design.md
  kafka_foundation_activation: specs/2026-08-28-kafka-foundation-contract-sdk-activation-design.md
  kafka_foundation_implementation: plans/2026-08-28-kafka-foundation-contract-sdk.md
```

不得删除当前 master 已有的三个 dmsvr 条目：

```yaml
  dmsvr_proto_domain_split_activation_candidate: specs/2026-08-28-dmsvr-proto-domain-split-activation-design.md
  dmsvr_proto_domain_split_activation: specs/2026-08-28-dmsvr-proto-domain-split-activation-design.md
  dmsvr_proto_domain_split_implementation: plans/2026-08-28-dmsvr-proto-domain-split.md
```

- [ ] **Step 2：忽略新的 worktree 根目录**

在 `.gitignore` 增加：

```gitignore
/.worktrees/
```

保留已有 Python 和 `.local/` 忽略规则。

- [ ] **Step 3：在 AGENTS.md 固化双仓库与 worktree 规则**

增加以下约束，措辞保持中文且不复制 LoopX 内部实现：

```markdown
## 双仓库与 Worktree

- 本仓库的稳定 Git identity 为 `git:gitee.com/ming-industry/ming-iot-rd-docs`，负责 specs、plans、proposals、evidence、reviews、references、meta 与 archives。
- 研发文档 linked worktree 统一创建在仓库内忽略目录 `.worktrees/`；禁止重新使用工作区级 `.rd-docs-worktrees/`。
- LoopX 文档写入型 Todo 必须把 `task_repository` 指向本仓库，并声明精确或有界的 `required_write_scopes`。
- `.loopx/`、`.codex/` 与 `.worktrees/` 是本地运行状态，不得提交到 Gitee。
```

### Task 4：运行迁移与目录契约校验

**文件：**

- 测试：`tests/test_authority_catalog.py`
- 校验器：`tools/check_authority_catalog.py`
- 读取：`../ming-iot/.loopx/registry.json`

- [ ] **Step 1：验证 Authority Catalog 单元测试**

运行：

```bash
python3 -m unittest -v tests.test_authority_catalog
```

预期：全部测试通过。

- [ ] **Step 2：验证真实目录与 LoopX Authority 投影**

运行：

```bash
python3 tools/check_authority_catalog.py \
  --catalog meta/DOC_REGISTRY.yaml \
  --docs-root . \
  --registry ../ming-iot/.loopx/registry.json \
  --goal-id ming-iot-framework-v1
```

预期：输出 `authority catalog: OK`，退出码为 0。

- [ ] **Step 3：验证 worktree 忽略和候选文件格式**

运行：

```bash
git check-ignore -v .worktrees/layout-smoke
python3 - <<'PY'
from pathlib import Path

paths = [
    "evidence/2026-08-27-ming-iot-local-share-migration-activation/source-freeze.json",
    "plans/2026-08-28-kafka-foundation-contract-sdk.md",
    "plans/2026-08-30-ming-iot-dual-repository-and-rd-docs-worktree-cutover.md",
    "specs/2026-08-27-ming-iot-local-share-migration-activation-design.md",
    "specs/2026-08-28-kafka-foundation-contract-sdk-activation-design.md",
    "specs/2026-08-30-ming-iot-dual-repository-and-rd-docs-worktree-cutover-design.md",
]

for raw_path in paths:
    path = Path(raw_path)
    data = path.read_bytes()
    if not data.endswith(b"\n"):
        raise SystemExit(f"missing final newline: {path}")
    for line_number, line in enumerate(data.splitlines(), start=1):
        trailing = line[len(line.rstrip(b" \t")):]
        if not trailing:
            continue
        if path.suffix == ".md" and trailing == b"  ":
            continue
        raise SystemExit(f"invalid trailing whitespace: {path}:{line_number}")

print(f"untracked candidate format: OK files={len(paths)}")
PY
git diff --check
git status --short --branch
```

预期：`.worktrees/layout-smoke` 命中根 `.gitignore` 的 `/.worktrees/`；只读 Python 检查输出 `untracked candidate format: OK files=6`；`git diff --check` 无输出；状态只包含本计划声明的文件。该 Python 检查只覆盖上述 6 个精确的未跟踪候选文件，不扫描更宽目录。所有文件都必须以换行结束；Markdown 文件仅允许以精确两个 ASCII 空格作为语义硬换行，仍拒绝行尾 TAB、单个空格或三个及以上空格；JSON 文件拒绝任何行尾空格或 TAB。

- [ ] **Step 4：运行候选交付的 LoopX 私有材料扫描**

运行：

```bash
loopx --registry ../ming-iot/.loopx/registry.json check \
  --scan-path .gitignore \
  --scan-path AGENTS.md \
  --scan-path meta/DOC_REGISTRY.yaml \
  --scan-path evidence/2026-08-27-ming-iot-local-share-migration-activation/source-freeze.json \
  --scan-path plans/2026-08-28-kafka-foundation-contract-sdk.md \
  --scan-path plans/2026-08-30-ming-iot-dual-repository-and-rd-docs-worktree-cutover.md \
  --scan-path specs/2026-08-27-ming-iot-local-share-migration-activation-design.md \
  --scan-path specs/2026-08-28-kafka-foundation-contract-sdk-activation-design.md \
  --scan-path specs/2026-08-30-ming-iot-dual-repository-and-rd-docs-worktree-cutover-design.md
```

这是候选交付扫描，只判定本次 9 个候选文件。全仓历史扫描可以作为诊断运行，但不得以既有不可变 `archives/` 中的关键词命中替代本次候选判定。

预期：候选交付扫描通过；若这 9 个文件报告原始对话、凭据、本地绝对路径或运行日志，停止提交并移除对应内容。

### Task 5：形成可恢复的 master 提交并推送

**文件：**

- 提交：Tasks 2–4 列出的全部预期文件

- [ ] **Step 1：展示提交边界并获取用户授权**

运行：

```bash
git status --short
git diff --stat
git diff -- .gitignore AGENTS.md meta/DOC_REGISTRY.yaml
```

向用户报告四个恢复文件的 SHA-256、校验结果和完整待提交路径。未获得明确 `git add/commit` 授权前停止；不得因为本计划获批而推断提交授权。

- [ ] **Step 2：仅在授权后按精确路径提交**

授权后运行精确 `git add`，不得使用 `git add .`。提交信息：

```bash
git commit -m "docs: 收口研发资料并统一本地 worktree"
```

预期：提交只包含计划声明文件，主工作区 clean。

- [ ] **Step 3：仅在独立 push 授权后推送**

运行：

```bash
git push origin master
git fetch origin
test "$(git rev-parse master)" = "$(git rev-parse origin/master)"
```

预期：普通 push 成功，本地与远端 `master` 精确一致。禁止 force-push。

### Task 6：删除旧 worktree 和旧分支

**文件系统目标：**

- 删除：`../.rd-docs-worktrees/kafka-foundation-activation`
- 删除：`../.rd-docs-worktrees/local-share-migration-activation`
- 删除空目录：`../.rd-docs-worktrees`
- 删除本地分支：`kafka-foundation-activation-spec`
- 删除本地分支：`local-share-migration-activation-spec`

- [ ] **Step 1：证明旧 worktree 内容已被 master 覆盖**

运行一个只读 Python 检查：

```bash
python3 - <<'PY'
from hashlib import sha256
from pathlib import Path
import subprocess

master = Path.cwd().resolve()
roots = [
    master.parent / ".rd-docs-worktrees" / "kafka-foundation-activation",
    master.parent / ".rd-docs-worktrees" / "local-share-migration-activation",
]

for root in roots:
    raw = subprocess.check_output(
        ["git", "-C", str(root), "status", "--porcelain=v1", "-z", "--untracked-files=all"]
    )
    for record in (item for item in raw.split(b"\0") if item):
        text = record.decode("utf-8")
        relative = text[3:]
        source = root / relative
        target = master / relative
        if relative == "meta/DOC_REGISTRY.yaml":
            old_lines = {
                line.strip() for line in source.read_text(encoding="utf-8").splitlines()
                if line.startswith("  ") and ": " in line
            }
            new_lines = {
                line.strip() for line in target.read_text(encoding="utf-8").splitlines()
            }
            missing = sorted(line for line in old_lines if line not in new_lines)
            if missing:
                raise SystemExit(f"catalog entries not preserved: {missing}")
            continue
        if not target.is_file():
            raise SystemExit(f"target missing: {relative}")
        if sha256(source.read_bytes()).digest() != sha256(target.read_bytes()).digest():
            raise SystemExit(f"content differs: {relative}")
print("old worktree coverage: OK")
PY
```

预期：输出 `old worktree coverage: OK`。任何缺失或差异都停止删除。

- [ ] **Step 2：确认旧分支没有 master 之外的提交**

运行：

```bash
test -z "$(git log --format=%H master..kafka-foundation-activation-spec)"
test -z "$(git log --format=%H master..local-share-migration-activation-spec)"
```

预期：两条命令均退出 0。

- [ ] **Step 3：通过 Git 删除两个明确 worktree**

仅在 Task 5 已形成可恢复提交且 Step 1–2 通过后运行：

```bash
git worktree remove --force ../.rd-docs-worktrees/kafka-foundation-activation
git worktree remove --force ../.rd-docs-worktrees/local-share-migration-activation
```

这是已批准的破坏性操作；不得把目标替换成通配符、环境变量或更宽目录。

- [ ] **Step 4：删除无独有提交的本地旧分支并清理登记**

运行：

```bash
git branch -D kafka-foundation-activation-spec
git branch -D local-share-migration-activation-spec
git worktree prune
rmdir ../.rd-docs-worktrees
```

预期：父目录为空并成功删除；上述旧分支不再存在。

### Task 7：验证新 worktree 根目录

**本地目标：**

- 临时创建并删除：`.worktrees/layout-smoke`

- [ ] **Step 1：在新根目录创建 detached smoke worktree**

运行：

```bash
git worktree add --detach .worktrees/layout-smoke HEAD
git -C .worktrees/layout-smoke rev-parse --show-toplevel
git worktree list --porcelain
```

预期：新 worktree 根目录为 `ming-iot-rd-docs/.worktrees/layout-smoke`，并出现在 worktree 列表中。

- [ ] **Step 2：验证父仓库不会追踪 worktree 内容**

运行：

```bash
git check-ignore -v .worktrees/layout-smoke
git status --short
```

预期：路径命中 `/.worktrees/`；主工作区保持 clean。

- [ ] **Step 3：删除 smoke worktree并执行最终验证**

运行：

```bash
git worktree remove .worktrees/layout-smoke
git worktree prune
git worktree list --porcelain
git status --short --branch
```

预期：只保留 `ming-iot-rd-docs` 主工作区；状态为 `master...origin/master` 且 clean。

### Task 8：为后续 LoopX Todo 使用双仓库路由

**运行状态：**

- 读取：`../ming-iot/.loopx/registry.json`
- 不提交：两个仓库下的 `.loopx/`、`.codex/`

- [ ] **Step 1：验证两个仓库的稳定 Git identity**

运行：

```bash
git -C ../ming-iot config --get remote.origin.url
git config --get remote.origin.url
```

预期分别对应：

```text
https://gitee.com/ming-industry/ming-iot.git
https://gitee.com/ming-industry/ming-iot-rd-docs.git
```

- [ ] **Step 2：约束后续 Todo 的仓库归属**

后续创建 Todo 时使用以下确定性规则，不在本次迁移中批量改写历史 Todo：

```text
实现代码：task_repository=git:gitee.com/ming-industry/ming-iot
写正式文档：task_repository=git:gitee.com/ming-industry/ming-iot-rd-docs
```

每个文档 Todo 同时声明精确或有界的 `required_write_scopes`。不得手工复制 `.loopx/registry.json` 或 ACTIVE_GOAL_STATE；首次真实文档 Todo 由 LoopX 官方路由建立对应 worktree/Agent handoff，并以 `workspace_guard` 读回为准。
