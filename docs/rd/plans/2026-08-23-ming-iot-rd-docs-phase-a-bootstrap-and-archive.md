# Ming IoT 本地研发归档引导实施计划

> **执行模式：** 本次执行采用 Subagent-Driven Development：每个任务均由一名新实施者完成，随后进行独立规范符合性审查，再进行独立质量审查。任何未解决的 Critical 或 Important 发现均意味着该任务未完成。此模式不授权在任务 6 之前提交，也不绕过用户的明确批准；步骤继续使用复选框（`- [ ]`）追踪。

**目标：** 建立仅本地使用的 `ming-iot-rd-docs` Git 仓库，并证明在进行任何来源清理或历史重写之前，所有必需的研发文档、旧版 Graph 契约、未发布工作及完整的 LoopX 前 `ming-iot` Git 历史均已安全归档。

**架构：** 本计划是已批准迁移设计的 Phase A。无 remote 的本地 Git 仓库存放活跃研发文档；不可变、可读的快照以及包含所有本地分支、远程跟踪分支和 Tag 的 Git Bundle 则保留权威旧历史。一个基于 Python 标准库的小型清单工具对归档进行哈希；若文件缺失、新增、修改或为符号链接即失败关闭。本阶段不得进行 LoopX 切换、Graph 删除、向 `ming-iot` 提交或向 Gitee push。

**技术栈：** Git 2.43+、Python 3.12 标准库、`unittest`、SHA-256、Git Bundle、现有 Linux/WSL2 文件系统。

---

## 1. 范围与执行边界

权威设计：

- `specs/2026-08-23-ming-iot-rd-docs-archive-and-legacy-graph-retirement-design.md`

本计划仅实施 **Gate B：本地归档验收**。后续工作必须使用单独评审过的计划：

1. Phase B —— 修订并执行 LoopX Bootstrap；
2. Phase C —— 退役当前树中的旧版 Graph 控制平面；
3. Phase D —— 演练并执行 Gitee 可达历史净化。

Phase A 不得：

- 删除任何尚存的来源文档或工作树；
- 修改 `ming-iot`、`iot-doc` 或正式 `things` 仓库；
- 安装或修改 LoopX；
- 运行 `git filter-repo`；
- 未经用户明确授权即提交；
- 为 `ming-iot-rd-docs` 添加 remote 或从其 push。

被废弃的 Dashboard 草案是唯一预先批准的例外。其正文已经移除，其路径、摘要和决定记录于：

- `archives/manifests/discarded-sources.json`

## 2. 来源基线

如以下值发生变化，实施必须停止：

| 来源 | 预期修订/状态 |
|---|---|
| `../ming-iot` | `HEAD=403b9a6d6b7e2c60a1ff96ef0f681da272d04c36`，分支 `master`，比 `origin/master` 超前 7 个提交，工作树干净 |
| `../ming-iot/.worktrees/loopx-personal-framework-bootstrap` | 相同 HEAD，分支 `plan/loopx-personal-framework-bootstrap`，恰有 3 个未跟踪研发文件 |
| `../iot-doc` | `HEAD=287d41a7ca2f5d444ceb34264fbe1ac515c42836`，恰有 2 个未跟踪框架规格 |
| Gitee `origin/master` | `7bd2e7f630cb09f054f105f2e21032edb51ee964` |
| Gitee `origin/core-migration-foundation` | `7d26ed1617e6bdf6d378716310f4dfc901ee297c` |

预期未发布文件：

```text
ming-iot/.worktrees/loopx-personal-framework-bootstrap/docs/superpowers/plans/2026-08-23-loopx-personal-framework-bootstrap.md
ming-iot/.worktrees/loopx-personal-framework-bootstrap/docs/superpowers/specs/2026-08-23-loopx-plan-bilingual-publication-design.md
ming-iot/.worktrees/loopx-personal-framework-bootstrap/docs/superpowers/specs/2026-08-23-ming-iot-local-rd-docs-isolation-design.md
iot-doc/docs/superpowers/specs/2026-08-21-loopx-driven-iot-development-control-plane-design.md
iot-doc/docs/superpowers/specs/2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md
```

## 3. 最终文件映射

### 仓库策略与索引

| 文件 | 职责 |
|---|---|
| `AGENTS.md` | 仅本地文档仓库规则、来源不可变性、批准与归档边界 |
| `README.md` | 当前权威文档映射，以及活跃材料与历史归档的区分 |
| `.gitignore` | 仅忽略 Python/本地运行时文件；归档工件仍对 Git 可见 |
| `.githooks/pre-push` | 确定性拒绝所有 push 尝试 |
| `specs/README.md` | 活跃规格规则 |
| `plans/README.md` | 可执行计划规则 |
| `evidence/README.md` | 精简确定性 Evidence 规则 |
| `reviews/README.md` | 独立 Review 工件规则 |
| `proposals/README.md` | 等待用户 Gate 的提案规则 |
| `archives/README.md` | 仅历史材料警告及恢复指南 |

### 确定性工具

| 文件 | 职责 |
|---|---|
| `tools/__init__.py` | 使归档工具可供测试导入 |
| `tools/archive_manifest.py` | 为 `archives/` 构建并验证精确 SHA-256 清单 |
| `tests/__init__.py` | 保持测试模块可通过稳定点分名称导入 |
| `tests/test_repository_policy.py` | 验证无 remote、拒绝 hook 与必需目录策略 |
| `tests/test_archive_manifest.py` | 验证确定性清单、修改/新增及符号链接拒绝 |

### 生成的归档

| 路径 | 职责 |
|---|---|
| `archives/repository-bundles/ming-iot-pre-loopx-20260823.bundle` | LoopX 切换/历史重写前完整的本地分支、远程跟踪分支与 Tag 历史；排除临时 stash/工作树 refs |
| `archives/snapshots/2026-08-23/ming-iot/docs/superpowers/` | 精确、可读的 Bootstrap 工作树研发快照，含其 3 个未跟踪文件 |
| `archives/snapshots/2026-08-23/ming-iot/.graph/` | 精确的已跟踪旧版 Graph Registry/Loop/Schema 快照；无运行时内容 |
| `archives/snapshots/2026-08-23/iot-doc/untracked-framework-specs/` | 两份未发布框架规格的精确副本 |
| `archives/manifests/source-provenance.json` | 来源修订、refs、工作树和预期未发布路径 |
| `archives/manifests/unpublished-commits.json` | 精确的 7 提交清单和初步重放/归档分类 |
| `archives/manifests/archive-files.json` | 除自身外每个归档工件的精确文件集、大小、模式和 SHA-256 |
| `archives/manifests/discarded-sources.json` | 已批准的废弃来源墓碑 |

只有两份当前 `iot-doc` 框架规格会提升至活跃 `specs/`。旧隔离设计、当前 Bootstrap 草案和双语发布草案均保留在历史快照中，直至 Phase B 依据批准的架构重写它们。

## 4. 本计划的提交策略

工作区 `AGENTS.md` 禁止自动提交。因此 任务 1–5 保持为未提交检查点。任务 6 展示完整的拟提交内容，并等待一次明确的用户授权。任何任务均不得 push。

### 任务 1：初始化仅本地仓库与失败关闭策略

**文件：**

- 创建：`.gitignore`
- 创建：`.githooks/pre-push`
- 验证/保留：`AGENTS.md`
- 创建：`README.md`
- 创建：`specs/README.md`
- 创建：`plans/README.md`
- 创建：`evidence/README.md`
- 创建：`reviews/README.md`
- 创建：`proposals/README.md`
- 创建：`archives/README.md`
- 创建：`tests/__init__.py`
- 创建：`tests/test_repository_policy.py`

- [ ] **步骤 1: 确认引导目录仅包含已批准的预存文件**

在 `/home/wangjunbo/workspace-iot-platform/ming-iot-rd-docs` 中运行：

```bash
find . -mindepth 1 ! -type d -print | LC_ALL=C sort
```

预期：Git 初始化前恰有以下四个预存文件，且不得有其他非目录条目：

```text
./AGENTS.md
./archives/manifests/discarded-sources.json
./plans/2026-08-23-ming-iot-rd-docs-phase-a-bootstrap-and-archive.md
./specs/2026-08-23-ming-iot-rd-docs-archive-and-legacy-graph-retirement-design.md
```

如存在任何其他文件，必须在初始化前停止并对其分类。

- [ ] **步骤 2: 在不配置 remote 的情况下初始化 Git**

```bash
git init -b master
git remote -v
```

预期：仓库初始化成功，且 `git remote -v` 无输出。

- [ ] **步骤 3: 先编写策略测试**

```bash
mkdir -p tests
```

使用 `apply_patch` 创建空的 `tests/__init__.py`，然后创建 `tests/test_repository_policy.py`：

```python
import os
import stat
import subprocess
import unittest
from pathlib import Path


REPO = Path(__file__).resolve().parents[1]


def git(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", "-C", str(REPO), *args],
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )


class RepositoryPolicyTest(unittest.TestCase):
    def test_repository_has_no_remote(self) -> None:
        result = git("remote")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "")

    def test_repository_uses_local_hooks(self) -> None:
        result = git("config", "--get", "core.hooksPath")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), ".githooks")

    def test_pre_push_is_executable_and_rejects(self) -> None:
        hook = REPO / ".githooks" / "pre-push"
        mode = hook.stat().st_mode
        self.assertTrue(mode & stat.S_IXUSR)
        result = subprocess.run(
            [str(hook), "origin", "unused"],
            check=False,
            text=True,
            input="",
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env={**os.environ, "LC_ALL": "C"},
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("local-only", result.stderr)

    def test_required_directories_exist(self) -> None:
        for relative in (
            "specs",
            "plans",
            "evidence",
            "reviews",
            "proposals",
            "archives",
        ):
            self.assertTrue((REPO / relative).is_dir(), relative)


if __name__ == "__main__":
    unittest.main()
```

- [ ] **步骤 4: 运行策略测试并确认预期失败**

```bash
python3 -m unittest tests.test_repository_policy -v
```

预期：FAIL，因为 `core.hooksPath`、hook 和若干必需目录尚不存在。此时通过意味着测试未实际检验新策略。

- [ ] **步骤 5: 添加仓库规则与索引**

```bash
mkdir -p .githooks evidence reviews proposals
```

验证现有 `AGENTS.md` 与以下仓库约定完全一致；仅在其不同时使用 `apply_patch` 更新：

```markdown
# ming-iot-rd-docs 本地协作约定

本仓库是研发规格、计划、Evidence、Review、Proposal 和旧资料归档的本地事实源。

## 文档语言

- `specs/` 与 `plans/` 中的正式文档只使用中文编写。
- 中文正文直接使用不带语言后缀的 `*.md`，并作为唯一权威版本。
- 不得为正式文档创建 `*.zh-CN.md`、`*.en.md` 或单独的语言索引；确需翻译时，必须由用户另行明确批准，且译文不得成为实施依据。
- 文档中的命令、代码、协议字段、产品名和行业术语可以保留英文，不要求机械翻译。

## 安全边界

- 禁止添加 Git remote，禁止 push 本仓库。
- 禁止写入凭据、原始模型对话、缓存或可再生运行日志。
- `archives/` 在摘要清单封存后视为不可变；历史材料不得重新冒充当前规范。
- 归档过程中不得修改或删除来源仓库。

## 提交

- 未经用户明确授权，不得执行 `git add` 或 `git commit`。
- 请求提交授权前，必须展示 `git status --short`、归档校验结果和策略测试结果。
```

创建 `README.md`：

```markdown
# ming-iot-rd-docs

Local-only R&D authority for the Ming IoT personal development framework.

- `specs/`: current approved designs;
- `plans/`: reviewed executable plans;
- `evidence/`: compact deterministic execution evidence;
- `reviews/`: independent reviewer findings and verdicts;
- `proposals/`: alternatives awaiting a user decision;
- `archives/`: superseded material and disaster-recovery bundles, never current authority.

This repository intentionally has no remote. Product code remains in `ming-iot`; formal IoT architecture remains in `iot-doc`; dynamic Goal/Todo state remains in LoopX.
```

创建 `.gitignore`：

```gitignore
__pycache__/
*.py[cod]
.local/
```

创建 `.githooks/pre-push`：

```sh
#!/bin/sh
echo "ming-iot-rd-docs is local-only; push is forbidden" >&2
exit 1
```

创建 `specs/README.md`：

```markdown
# 活跃规格

这里只保存当前已评审的设计。被替代的设计必须移入 `archives/` 并标记为历史材料。

正式规格只使用中文，并直接保存为不带语言后缀的 `*.md`。
```

创建 `plans/README.md`：

```markdown
# 实施计划

计划必须指向已批准的规格，并写明精确文件、验证命令和用户批准点。

正式计划只使用中文，并直接保存为不带语言后缀的 `*.md`。
```

创建 `evidence/README.md`：

```markdown
# Evidence

Store compact deterministic results, revisions and digests. Do not store credentials, caches or raw transcripts.
```

创建 `reviews/README.md`：

```markdown
# Independent reviews

Reviews record the exact subject, rerun checks, ordered findings and a PASS or REJECT verdict. Reviewers do not repair reviewed work.
```

创建 `proposals/README.md`：

```markdown
# Proposals

Proposals compare bounded alternatives and remain non-authoritative until the user selects one.
```

创建 `archives/README.md`：

```markdown
# Historical archive — not current authority

Everything below this directory is retained for provenance, audit or disaster recovery. Do not execute archived plans or treat archived specifications as current requirements.

`repository-bundles/` contains authoritative pre-migration branch/Tag history. `snapshots/` contains readable point-in-time material. `manifests/` records source provenance, intentional discards and exact archive hashes.
```

- [ ] **步骤 6: 启用并验证拒绝 hook**

```bash
chmod 0755 .githooks/pre-push
git config core.hooksPath .githooks
python3 -m unittest tests.test_repository_policy -v
```

预期：4 项测试 PASS。不得仅为测试 hook 而配置 remote；测试直接调用它。

- [ ] **步骤 7: 展示未提交检查点**

```bash
git status --short
git remote -v
```

预期：仅 Phase A 本地仓库文件处于未跟踪状态；不输出 remote。不得暂存或提交。

### 任务 2：以 TDD 构建确定性归档清单验证工具

**文件：**

- 创建：`tools/__init__.py`
- 创建：`tools/archive_manifest.py`
- 创建：`tests/test_archive_manifest.py`

- [ ] **步骤 1: 创建包标记**

使用 `apply_patch` 创建空的 `tools/__init__.py`。

- [ ] **步骤 2: 编写失败的清单测试**

创建 `tests/test_archive_manifest.py`：

```python
import os
import tempfile
import unittest
from pathlib import Path

from tools.archive_manifest import ArchiveError, build_manifest, verify_manifest


class ArchiveManifestTest(unittest.TestCase):
    def make_archive(self, parent: Path) -> tuple[Path, Path]:
        archive = parent / "archives"
        manifest = archive / "manifests" / "archive-files.json"
        (archive / "snapshots").mkdir(parents=True)
        (archive / "snapshots" / "proof.txt").write_text("proof\n", encoding="utf-8")
        return archive, manifest

    def test_build_is_deterministic_and_verify_accepts_exact_tree(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            archive, manifest = self.make_archive(Path(temporary))
            first = build_manifest(archive, manifest)
            first_bytes = manifest.read_bytes()
            second = build_manifest(archive, manifest)
            self.assertEqual(first, second)
            self.assertEqual(first_bytes, manifest.read_bytes())
            verify_manifest(archive, manifest)

    def test_verify_rejects_modified_file(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            archive, manifest = self.make_archive(Path(temporary))
            build_manifest(archive, manifest)
            (archive / "snapshots" / "proof.txt").write_text("changed\n", encoding="utf-8")
            with self.assertRaisesRegex(ArchiveError, "archive file set or digest changed"):
                verify_manifest(archive, manifest)

    def test_verify_rejects_added_file(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            archive, manifest = self.make_archive(Path(temporary))
            build_manifest(archive, manifest)
            (archive / "snapshots" / "extra.txt").write_text("extra\n", encoding="utf-8")
            with self.assertRaisesRegex(ArchiveError, "archive file set or digest changed"):
                verify_manifest(archive, manifest)

    def test_build_rejects_symlink(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            archive, manifest = self.make_archive(Path(temporary))
            os.symlink(archive / "snapshots" / "proof.txt", archive / "snapshots" / "alias.txt")
            with self.assertRaisesRegex(ArchiveError, "symlink"):
                build_manifest(archive, manifest)


if __name__ == "__main__":
    unittest.main()
```

- [ ] **步骤 3: 运行测试并确认预期导入失败**

```bash
python3 -m unittest tests.test_archive_manifest -v
```

预期：FAIL，因为 `tools.archive_manifest` 不存在。

- [ ] **步骤 4: 实现最小的确定性清单工具**

创建 `tools/archive_manifest.py`：

```python
from __future__ import annotations

import argparse
import hashlib
import json
import os
import stat
import sys
import tempfile
from pathlib import Path
from typing import Any


SCHEMA_VERSION = 1


class ArchiveError(RuntimeError):
    pass


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def _validate_paths(root: Path, manifest: Path) -> tuple[Path, Path]:
    if root.is_symlink():
        raise ArchiveError(f"archive root is a symlink: {root}")
    root = root.resolve()
    manifest = manifest.resolve()
    if not root.is_dir():
        raise ArchiveError(f"archive root is not a directory: {root}")
    try:
        manifest.relative_to(root)
    except ValueError as error:
        raise ArchiveError("manifest must be inside archive root") from error
    return root, manifest


def _scan(root: Path, manifest: Path) -> list[dict[str, Any]]:
    entries: list[dict[str, Any]] = []
    for candidate in sorted(root.rglob("*"), key=lambda item: item.as_posix()):
        if candidate.is_symlink():
            raise ArchiveError(f"archive contains symlink: {candidate.relative_to(root)}")
        if candidate.is_dir():
            continue
        if not candidate.is_file():
            raise ArchiveError(f"archive contains non-regular file: {candidate.relative_to(root)}")
        if candidate.resolve() == manifest:
            continue
        relative = candidate.relative_to(root).as_posix()
        mode = stat.S_IMODE(candidate.stat().st_mode)
        entries.append(
            {
                "path": relative,
                "size": candidate.stat().st_size,
                "mode": f"{mode:04o}",
                "sha256": sha256_file(candidate),
            }
        )
    return entries


def _document(root: Path, manifest: Path) -> dict[str, Any]:
    return {
        "schemaVersion": SCHEMA_VERSION,
        "root": root.name,
        "files": _scan(root, manifest),
    }


def _write_atomic(path: Path, content: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=path.name + ".", dir=path.parent)
    temporary = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        if temporary.exists():
            temporary.unlink()


def build_manifest(root: Path, manifest: Path) -> dict[str, Any]:
    root, manifest = _validate_paths(root, manifest)
    document = _document(root, manifest)
    encoded = (json.dumps(document, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode("utf-8")
    _write_atomic(manifest, encoded)
    return document


def verify_manifest(root: Path, manifest: Path) -> None:
    root, manifest = _validate_paths(root, manifest)
    try:
        expected = json.loads(manifest.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ArchiveError(f"cannot read manifest: {error}") from error
    if expected.get("schemaVersion") != SCHEMA_VERSION:
        raise ArchiveError("unsupported archive manifest schema")
    actual = _document(root, manifest)
    if actual != expected:
        raise ArchiveError("archive file set or digest changed")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Build or verify a deterministic local archive manifest")
    subparsers = parser.add_subparsers(dest="command", required=True)
    for command in ("build", "verify"):
        subparser = subparsers.add_parser(command)
        subparser.add_argument("--root", required=True, type=Path)
        subparser.add_argument("--manifest", required=True, type=Path)
    arguments = parser.parse_args(argv)
    try:
        if arguments.command == "build":
            document = build_manifest(arguments.root, arguments.manifest)
            print(f"archive manifest built: {len(document['files'])} files")
        else:
            verify_manifest(arguments.root, arguments.manifest)
            print("archive manifest verified")
    except ArchiveError as error:
        print(f"archive manifest: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
```

- [ ] **步骤 5: 运行聚焦测试**

```bash
python3 -m unittest tests.test_archive_manifest -v
```

预期：4 项测试 PASS。

- [ ] **步骤 6: 运行全部本地仓库测试**

```bash
python3 -m unittest discover -s tests -v
```

预期：8 项测试 PASS。不得暂存或提交。

### 任务 3：冻结精确来源清单并创建灾难恢复 Bundle

**文件：**

- 创建：`archives/manifests/source-provenance.json`
- 创建：`archives/manifests/unpublished-commits.json`
- 创建：`archives/repository-bundles/ming-iot-pre-loopx-20260823.bundle`

- [ ] **步骤 1: 重新读取来源修订、remote refs 与工作树状态**

```bash
git -C ../ming-iot rev-parse HEAD
git -C ../ming-iot status --short --branch
git -C ../ming-iot worktree list --porcelain
git -C ../ming-iot ls-remote --heads --tags origin | LC_ALL=C sort -k2,2
git -C ../ming-iot/.worktrees/loopx-personal-framework-bootstrap status --short --untracked-files=all
git -C ../iot-doc rev-parse HEAD
git -C ../iot-doc status --short --untracked-files=all
```

预期：与第 2 节完全一致；remote ref 清单必须恰好为以下两个 head，且不得出现 Tag 或其他 head：

```text
7d26ed1617e6bdf6d378716310f4dfc901ee297c	refs/heads/core-migration-foundation
7bd2e7f630cb09f054f105f2e21032edb51ee964	refs/heads/master
```

如任一修订、ref 或未发布文件不同，必须停止并修订计划，而不是悄然扩大归档范围。

- [ ] **步骤 2: 编写来源溯源契约**

创建 `archives/manifests/source-provenance.json`：

```json
{
  "schemaVersion": 1,
  "captureId": "2026-08-23-pre-loopx",
  "sources": [
    {
      "id": "ming-iot-main",
      "path": "../ming-iot",
      "head": "403b9a6d6b7e2c60a1ff96ef0f681da272d04c36",
      "branch": "master",
      "workingTree": "clean",
      "originMaster": "7bd2e7f630cb09f054f105f2e21032edb51ee964",
      "originCoreMigrationFoundation": "7d26ed1617e6bdf6d378716310f4dfc901ee297c"
    },
    {
      "id": "ming-iot-loopx-bootstrap-worktree",
      "path": "../ming-iot/.worktrees/loopx-personal-framework-bootstrap",
      "head": "403b9a6d6b7e2c60a1ff96ef0f681da272d04c36",
      "branch": "plan/loopx-personal-framework-bootstrap",
      "untracked": [
        "docs/superpowers/plans/2026-08-23-loopx-personal-framework-bootstrap.md",
        "docs/superpowers/specs/2026-08-23-loopx-plan-bilingual-publication-design.md",
        "docs/superpowers/specs/2026-08-23-ming-iot-local-rd-docs-isolation-design.md"
      ]
    },
    {
      "id": "iot-doc-framework-specs",
      "path": "../iot-doc",
      "head": "287d41a7ca2f5d444ceb34264fbe1ac515c42836",
      "branch": "master",
      "untracked": [
        "docs/superpowers/specs/2026-08-21-loopx-driven-iot-development-control-plane-design.md",
        "docs/superpowers/specs/2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md"
      ]
    }
  ],
  "discardedSources": "discarded-sources.json",
  "bundleRefPolicy": {
    "include": [
      "refs/heads/*",
      "refs/remotes/*",
      "refs/tags/*"
    ],
    "exclude": [
      "refs/stash",
      "worktrees/*",
      "HEAD"
    ],
    "reason": "Ephemeral stash and worktree refs are not authoritative history and remain untouched in the source repository."
  }
}
```

- [ ] **步骤 3: 记录每个未发布提交及其初步处置**

创建 `archives/manifests/unpublished-commits.json`：

```json
{
  "schemaVersion": 1,
  "base": "7bd2e7f630cb09f054f105f2e21032edb51ee964",
  "head": "403b9a6d6b7e2c60a1ff96ef0f681da272d04c36",
  "commits": [
    {
      "sha": "edf39c1e0d6f2da14cb74adf8eb98522dafab88c",
      "subject": "docs(tools): design shared Go cache",
      "paths": [
        ".graph/loops/L0-things-baseline.yaml",
        "docs/superpowers/specs/2026-08-19-thingsbaseline-shared-go-cache-design.md"
      ],
      "disposition": "ARCHIVE_ONLY",
      "reason": "Both paths belong to the retired Graph and local R&D archive."
    },
    {
      "sha": "e1cd1794534d6f34cdaffbaba13ffd28544a7299",
      "subject": "docs(tools): plan shared Go cache implementation",
      "paths": [
        ".graph/loops/L0-things-baseline.yaml",
        "docs/superpowers/plans/2026-08-19-thingsbaseline-shared-go-cache.md"
      ],
      "disposition": "ARCHIVE_ONLY",
      "reason": "Both paths belong to the retired Graph and local R&D archive."
    },
    {
      "sha": "0bf3d2548ad9c7241f9f37f0c3e36098867f6789",
      "subject": "feat(tools): resolve shared Go cache paths",
      "paths": [
        "backend/tools/internal/thingsbaseline/go_cache.go",
        "backend/tools/internal/thingsbaseline/go_cache_test.go"
      ],
      "disposition": "REPLAY_CODE"
    },
    {
      "sha": "8982a3cbde5bc6d9c6fb63eee49b8406b0c26717",
      "subject": "feat(tools): reuse trusted Go caches",
      "paths": [
        "backend/tools/internal/thingsbaseline/go_cache.go",
        "backend/tools/internal/thingsbaseline/go_cache_test.go",
        "backend/tools/internal/thingsbaseline/render.go",
        "backend/tools/internal/thingsbaseline/render_test.go"
      ],
      "disposition": "REPLAY_CODE"
    },
    {
      "sha": "dc19c8a77e05994e1efa61c30b6e267c9048b3d2",
      "subject": "feat(tools): reuse Go caches in L0 gates",
      "paths": [
        "backend/tools/internal/thingsbaseline/verify.go",
        "backend/tools/internal/thingsbaseline/verify_test.go"
      ],
      "disposition": "REPLAY_CODE"
    },
    {
      "sha": "8a9e7930dc4e8b67aec212484a197a15f8998f88",
      "subject": "docs(tools): explain shared Go cache semantics",
      "paths": [
        "backend/tools/README.md",
        "docs/superpowers/specs/2026-08-14-l0-things-compilation-migration-baseline-design.md"
      ],
      "disposition": "SPLIT_REPLAY",
      "replayPaths": [
        "backend/tools/README.md"
      ],
      "archiveOnlyPaths": [
        "docs/superpowers/specs/2026-08-14-l0-things-compilation-migration-baseline-design.md"
      ]
    },
    {
      "sha": "403b9a6d6b7e2c60a1ff96ef0f681da272d04c36",
      "subject": "fix(tools): pin render commands to attested Go tool",
      "paths": [
        "backend/tools/internal/thingsbaseline/render.go",
        "backend/tools/internal/thingsbaseline/render_test.go"
      ],
      "disposition": "REPLAY_CODE"
    }
  ]
}
```

执行：

```bash
git -C ../ming-iot log --reverse --format='%H%x09%s' origin/master..HEAD
python3 -m json.tool archives/manifests/unpublished-commits.json >/dev/null
```

预期：Git 输出和 JSON 以相同顺序包含相同的 7 个提交。这些是 Phase A 初步分类；后续重写计划必须补充精确的重写后 SHA，而不得更改此归档记录。

- [ ] **步骤 4: 验证 JSON 与 Dashboard 墓碑**

```bash
python3 -m json.tool archives/manifests/source-provenance.json >/dev/null
python3 -m json.tool archives/manifests/unpublished-commits.json >/dev/null
python3 -m json.tool archives/manifests/discarded-sources.json >/dev/null
rg -n '38d0b16e2fcaee4f7f56cbbb55f457f5e0f9514cbf35e8a9acda78aba9ef7ec7|DISCARDED_SUPERSEDED' archives/manifests/discarded-sources.json
```

预期：JSON 有效，且输出批准的摘要和决定。

- [ ] **步骤 5: 创建完整 Git Bundle**

```bash
mkdir -p archives/repository-bundles
test ! -e archives/repository-bundles/ming-iot-pre-loopx-20260823.bundle
test ! -e archives/repository-bundles/ming-iot-pre-loopx-20260823.bundle.tmp
git -C ../ming-iot for-each-ref --format='%(refname)' refs/heads refs/remotes refs/tags |
  git -C ../ming-iot bundle create "$PWD/archives/repository-bundles/ming-iot-pre-loopx-20260823.bundle.tmp" --stdin
git bundle verify archives/repository-bundles/ming-iot-pre-loopx-20260823.bundle.tmp
mv archives/repository-bundles/ming-iot-pre-loopx-20260823.bundle.tmp \
  archives/repository-bundles/ming-iot-pre-loopx-20260823.bundle
```

预期：先创建并验证一个非空临时 Bundle，再将其原子移动到最终路径。`for-each-ref` 与 `--stdin` 保留全部当前 `refs/heads`、`refs/remotes` 和 `refs/tags` 的精确名称，同时排除 `refs/stash`、工作树私有 refs 和顶层符号 `HEAD`；该过程不更改或删除任何来源 ref。

- [ ] **步骤 6: 验证 Bundle 可读性与 refs**

```bash
git bundle verify archives/repository-bundles/ming-iot-pre-loopx-20260823.bundle
git bundle list-heads archives/repository-bundles/ming-iot-pre-loopx-20260823.bundle
```

预期：验证成功，输出包含 `refs/heads/master`、`refs/heads/plan/loopx-personal-framework-bootstrap`、`refs/remotes/origin/HEAD`、`refs/remotes/origin/master` 和 `refs/remotes/origin/core-migration-foundation`。输出不得包含 `refs/stash`、`worktrees/` 或顶层 `HEAD`，且不得出现重复 ref 名。

- [ ] **步骤 7: 确认来源仓库保持不变**

```bash
git -C ../ming-iot status --short --branch
git -C ../ming-iot/.worktrees/loopx-personal-framework-bootstrap status --short --branch --untracked-files=all
git -C ../iot-doc status --short --branch --untracked-files=all
```

预期：状态与第 2 节相同；Bundle 创建未增加来源变更。

### 任务 4：创建可读历史快照并仅提升当前权威文档

**文件：**

- 创建：`archives/snapshots/2026-08-23/ming-iot/docs/superpowers/**`
- 创建：`archives/snapshots/2026-08-23/ming-iot/.graph/**`
- 创建：`archives/snapshots/2026-08-23/iot-doc/untracked-framework-specs/*.md`
- 创建：`specs/2026-08-21-loopx-driven-iot-development-control-plane-design.md`
- 创建：`specs/2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md`

- [ ] **步骤 1: 断言快照目标尚不存在**

```bash
test ! -e archives/snapshots/2026-08-23
mkdir -p archives/snapshots/2026-08-23/ming-iot/docs
mkdir -p archives/snapshots/2026-08-23/iot-doc/untracked-framework-specs
```

预期：断言通过。预先存在的目标表明存在部分执行，必须审查，不能覆盖。

- [ ] **步骤 2: 复制精确的 Bootstrap 工作树研发目录**

```bash
cp -a ../ming-iot/.worktrees/loopx-personal-framework-bootstrap/docs/superpowers archives/snapshots/2026-08-23/ming-iot/docs/
diff -qr ../ming-iot/.worktrees/loopx-personal-framework-bootstrap/docs/superpowers archives/snapshots/2026-08-23/ming-iot/docs/superpowers
```

预期：`diff -qr` 无输出。快照包含 38 个文件：HEAD 中的 35 个文件加上 3 个未发布 Bootstrap 文档。

- [ ] **步骤 3: 仅导出已跟踪的旧版 Graph 契约**

```bash
git -C ../ming-iot/.worktrees/loopx-personal-framework-bootstrap archive --format=tar HEAD .graph | tar -xf - -C archives/snapshots/2026-08-23/ming-iot
find archives/snapshots/2026-08-23/ming-iot/.graph -type f -print | sort
```

预期：恰有以下 4 个文件，且没有 `.graph/runtime/`：

```text
archives/snapshots/2026-08-23/ming-iot/.graph/loops/G0-rd-graph-control-plane.yaml
archives/snapshots/2026-08-23/ming-iot/.graph/loops/L0-things-baseline.yaml
archives/snapshots/2026-08-23/ming-iot/.graph/registry.yaml
archives/snapshots/2026-08-23/ming-iot/.graph/schemas/loop-v1.schema.json
```

- [ ] **步骤 4: 逐字节验证每个 Graph 快照与 HEAD 一致**

```bash
while IFS= read -r graph_path; do
  cmp "../ming-iot/.worktrees/loopx-personal-framework-bootstrap/$graph_path" "archives/snapshots/2026-08-23/ming-iot/$graph_path"
done < <(git -C ../ming-iot/.worktrees/loopx-personal-framework-bootstrap ls-files .graph)
```

预期：无输出且退出码为 0。

- [ ] **步骤 5: 将两份未发布框架规格复制到历史快照**

```bash
cp -a ../iot-doc/docs/superpowers/specs/2026-08-21-loopx-driven-iot-development-control-plane-design.md archives/snapshots/2026-08-23/iot-doc/untracked-framework-specs/
cp -a ../iot-doc/docs/superpowers/specs/2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md archives/snapshots/2026-08-23/iot-doc/untracked-framework-specs/
cmp ../iot-doc/docs/superpowers/specs/2026-08-21-loopx-driven-iot-development-control-plane-design.md archives/snapshots/2026-08-23/iot-doc/untracked-framework-specs/2026-08-21-loopx-driven-iot-development-control-plane-design.md
cmp ../iot-doc/docs/superpowers/specs/2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md archives/snapshots/2026-08-23/iot-doc/untracked-framework-specs/2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md
```

预期：两条 `cmp` 命令均以退出码 0 结束。

- [ ] **步骤 6：仅提升两份当前中文框架权威规格**

```bash
cp --no-clobber ../iot-doc/docs/superpowers/specs/2026-08-21-loopx-driven-iot-development-control-plane-design.md specs/
cp --no-clobber ../iot-doc/docs/superpowers/specs/2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md specs/
cmp ../iot-doc/docs/superpowers/specs/2026-08-21-loopx-driven-iot-development-control-plane-design.md specs/2026-08-21-loopx-driven-iot-development-control-plane-design.md
cmp ../iot-doc/docs/superpowers/specs/2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md specs/2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md
```

预期：两份活跃规格均为中文正文，且与来源文件字节一致。不得创建 `.zh-CN.md`、`.en.md` 或语言索引；本阶段不得提升旧隔离设计、Bootstrap 草案或双语发布草案。

- [ ] **步骤 7: 确认有意废弃的 Dashboard 正文不存在**

```bash
test ! -e ../ming-iot/docs/superpowers/specs/2026-08-18-graph-dashboard-systemd-service-switch-design.md
test ! -e archives/snapshots/2026-08-23/ming-iot/docs/superpowers/specs/2026-08-18-graph-dashboard-systemd-service-switch-design.md
```

预期：两个断言均通过；仅保留其墓碑。

### 任务 5：封存并独立验证 Gate B

**文件：**

- 由工具创建：`archives/manifests/archive-files.json`

- [ ] **步骤 1: 构建归档清单**

```bash
python3 tools/archive_manifest.py build \
  --root archives \
  --manifest archives/manifests/archive-files.json
```

预期：`archive manifest built: 49 files`。这 49 个文件包括 Bundle、44 个快照文件、归档 README、来源溯源、未发布提交清单和废弃墓碑；清单不包括自身。

- [ ] **步骤 2: 立即验证精确归档**

```bash
python3 tools/archive_manifest.py verify \
  --root archives \
  --manifest archives/manifests/archive-files.json
```

预期：`archive manifest verified`，且退出码为 0。

- [ ] **步骤 3: 清单封存后重新验证 Bundle**

```bash
git bundle verify archives/repository-bundles/ming-iot-pre-loopx-20260823.bundle
git bundle list-heads archives/repository-bundles/ming-iot-pre-loopx-20260823.bundle
```

预期：Bundle 验证通过，且预期的本地/remote refs 仍存在。

- [ ] **步骤 4: 运行全部确定性仓库测试**

```bash
python3 -m unittest discover -s tests -v
```

预期：8 项测试 PASS。

- [ ] **步骤 5: 证明未修改或移除来源材料**

```bash
git -C ../ming-iot status --short --branch
git -C ../ming-iot/.worktrees/loopx-personal-framework-bootstrap status --short --branch --untracked-files=all
git -C ../iot-doc status --short --branch --untracked-files=all
```

预期：

- `ming-iot` 主工作树保持干净，且仍比 `origin/master` 超前 7 个提交；
- Bootstrap 工作树仍恰显示其 3 个未发布文件；
- `iot-doc` 仍恰好显示其 2 个未发布框架规格。

- [ ] **步骤 6: 证明不存在 remote 或 push 路径**

```bash
git remote -v
./.githooks/pre-push origin unused
```

预期：`git remote -v` 无输出；hook 以非零状态退出并打印 `local-only`。

- [ ] **步骤 7: 展示归档大小与不可变清单摘要**

```bash
du -sh archives
sha256sum archives/manifests/archive-files.json
```

预期：非零归档大小和一个 SHA-256 摘要。将两者记录在交接消息中；不得手动编辑 `archive-files.json`。

### 任务 6：审查并创建唯一获授权的 Phase A 提交

**文件：**

- 仅在批准后暂存：`.gitignore`, `.githooks/`, `AGENTS.md`, `README.md`, `archives/`, `evidence/`, `plans/`, `proposals/`, `reviews/`, `specs/`, `tests/`, `tools/`

- [ ] **步骤 1: 运行完整且新鲜的验证集**

```bash
python3 -m unittest discover -s tests -v
python3 tools/archive_manifest.py verify --root archives --manifest archives/manifests/archive-files.json
git bundle verify archives/repository-bundles/ming-iot-pre-loopx-20260823.bundle
git remote -v
git status --short --untracked-files=all
find . -mindepth 1 -maxdepth 1 ! -name .git -printf '%P\n' | LC_ALL=C sort
```

预期：测试、清单和 Bundle 通过；remote 输出为空；完整未跟踪状态与顶层路径清单仅包含 Phase A 文件。授权前不得运行任何形式的 `git add`，包括 `git add -n`。

顶层路径清单必须精确为：

```text
.githooks
.gitignore
AGENTS.md
README.md
archives
evidence
plans
proposals
reviews
specs
tests
tools
```

- [ ] **步骤 2: 展示 Gate B 证据并等待用户明确授权**

请求授权前，确认 `plans/` 和 `specs/` 中的正式文档均为不带语言后缀的中文 `*.md`，且不存在 `.zh-CN.md`、`.en.md` 或语言索引派生文件。

报告：

- 测试数量和结果；
- 归档文件数量和清单 SHA-256；
- Bundle 验证及所含 refs；
- 来源仓库状态；
- 本地仓库状态；
- 确认未发生额外来源删除、来源提交、remote、push 或历史重写；此前批准的 Dashboard 废弃仍仅以其墓碑表示。

在用户明确批准此精确的 Phase A 提交前，不得运行 `git add` 或 `git commit`。

- [ ] **步骤 3: 批准后暂存已审查的路径**

```bash
git add -- .gitignore .githooks AGENTS.md README.md archives evidence plans proposals reviews specs tests tools
git status --short
```

预期：仅已审查的 Phase A 路径被暂存。任何意外路径都会停止提交。

- [ ] **步骤 4: 提交本地归档**

```bash
git commit -m "docs: establish local R&D archive"
```

预期：本地创建一个根提交。后续不得有 push 命令。

- [ ] **步骤 5: 验证已提交的仅本地状态**

```bash
git status --short --branch
git log -1 --oneline
git remote -v
python3 -m unittest discover -s tests -v
python3 tools/archive_manifest.py verify --root archives --manifest archives/manifests/archive-files.json
./.githooks/pre-push origin unused
```

预期：

- 干净的 `master` 分支；
- 一个本地提交；
- 没有 remote；
- 8 项测试 PASS；
- 归档清单已验证；
- pre-push 被拒绝。

- [ ] **步骤 6: 关闭 Gate B 并准备 Phase B 输入**

在用户交接中记录本地提交 SHA、清单 SHA-256 和 Bundle SHA-256。只有在用户接受 Gate B 后，Phase B 才可开始。Phase B 必须在执行其任何命令前重写已归档的 Bootstrap 草案。

## 5. Phase A 验收清单

- [ ] 本地仓库已在无 remote 的情况下初始化为 `master`。
- [ ] 本地 pre-push 确定性拒绝。
- [ ] 仓库策略和归档清单测试通过。
- [ ] `ming-iot` Git Bundle 验证通过，且包含所有本地分支、远程跟踪分支和 Tag，并按契约排除临时 stash/工作树 refs。
- [ ] 可读的 `ming-iot/docs/superpowers` 快照包含全部预期的 38 个文件。
- [ ] 旧版 `.graph` 快照恰包含 4 个已跟踪契约文件且无运行时内容。
- [ ] 两份未发布 `iot-doc` 框架规格均存在于快照和活跃 `specs/` 中；活跃副本为中文正文且与来源字节一致。
- [ ] `plans/` 和 `specs/` 中的正式文档只使用不带语言后缀的中文 `*.md`，不存在语言派生文件。
- [ ] Dashboard 正文不存在，且其废弃墓碑有效。
- [ ] 归档清单验证精确路径、大小、模式和 SHA-256。
- [ ] 全部 7 个未发布提交均有不可变的初步处置记录。
- [ ] 来源仓库和工作树保持不变。
- [ ] 存在一个经明确批准的本地提交；未发生 push。
- [ ] Gate B 交接包含本地提交、清单和 Bundle 摘要。
