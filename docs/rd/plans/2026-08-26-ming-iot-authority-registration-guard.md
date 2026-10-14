# ming-iot Authority 登记防错实施计划

> **面向执行 Agent：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，按复选框逐项实施并在每个 Task 后复核。

**目标：** 让未来 Agent 在不依赖旧 Chat 的情况下，从 `ming-iot` Active Goal 找到项目文档目录，再按 Topic 定位正文；任何裸哈希、孤立来源或失效路径都能被聚焦校验发现。

**架构：** 保持 LoopX 官方能力不变。`ming-iot-rd-docs/meta/DOC_REGISTRY.yaml` 是正文定位目录，`ming-iot/.loopx/registry.json` 保留脱敏 Authority 契约，`ming-iot/AGENTS.md` 固化操作顺序，Python 标准库校验器负责两者交叉检查。Git 跟踪文件在独立 worktree 修改；LoopX 私有 Active Goal 仍由已注册的主项目根维护，因为 `.codex/` 不随 Git worktree 复制。

**技术栈：** LoopX CLI、Python 3 标准库、`unittest`、Git worktree、Markdown、简单 YAML。

---

## 已完成的前置状态

- [x] 用户已批准设计规格：`specs/2026-08-26-ming-iot-authority-registration-guard-design.md`。
- [x] 已创建独立 worktree：`ming-iot/.worktrees/authority-registration-guard`，分支 `authority-registration-guard`。
- [x] 已运行 `ming-iot/backend/tools` 基线测试，`go test ./...` 通过。
- [x] 已在 LoopX 主项目根创建 Agent Todo：`todo_9677142604e3`。
- [ ] 尚未修改实现文件，尚未 commit、merge 或 push。

## Task 1：校正当前 Todo 的写入边界

**文件：**

- 修改（LoopX 本地私有）：`ming-iot/.loopx/registry.json`
- 读取：`ming-iot/.codex/goals/ming-iot-framework-v1/ACTIVE_GOAL_STATE.md`

- [ ] **Step 1：从注册根重读 Todo 与配额**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
loopx todo list --goal-id ming-iot-framework-v1 --role agent --format json
loopx --format json quota should-run --goal-id ming-iot-framework-v1 --agent-id ming-peer-01
```

预期：`todo_9677142604e3` 为 `open` 且 `claimed_by=ming-peer-01`；`should_run=true`。若不是，停止并报告，不猜测或新建重复 Todo。

- [ ] **Step 2：分别预览旧边界清理与新边界写入**

LoopX 的 `checkpointed_boundary_authority[]` 是追加式审批历史；新增边界不会自动停用旧 Graph 边界，`--clear-boundary-authority` 也不能与新增边界字段同一命令执行。因此必须先分别 dry-run，不能把两步误写成一个“替换”命令。

先预览清理：

```bash
loopx configure-goal \
  --goal-id ming-iot-framework-v1 \
  --clear-boundary-authority
```

预期：只改变 `checkpointed_boundary_authority`，不改变 `write_scope`、注册 Agent、配额或编排模式。

再预览新边界字段是否可被 CLI 接受：

```bash
loopx configure-goal \
  --goal-id ming-iot-framework-v1 \
  --write-scope AGENTS.md \
  --write-scope .codex/goals/ming-iot-framework-v1/ACTIVE_GOAL_STATE.md \
  --write-scope .loopx/registry.json \
  --boundary-authority-scope AGENTS.md \
  --boundary-authority-scope .codex/goals/ming-iot-framework-v1/ACTIVE_GOAL_STATE.md \
  --boundary-authority-scope .loopx/registry.json \
  --boundary-authority-source user-approved-design:2026-08-26-authority-registration-guard \
  --boundary-authority-decision-id user-approval:2026-08-26-authority-registration-guard
```

预期：新边界字段合法；该 dry-run 会显示“旧边界 + 新边界”的追加结果，这一步只用于验证新条目，不得直接据此执行。

- [ ] **Step 3：备份 LoopX 私有状态**

```bash
loopx backup-state \
  --project /home/wangjunbo/workspace-iot-platform/ming-iot \
  --current-project-only \
  --backup-id before-authority-registration-guard-20260826 \
  --execute
```

预期：在 LoopX 私有 runtime backup 目录产生 archive 与 manifest；不得把备份放入 Git 仓库。

- [ ] **Step 4：清理旧边界并立即写入新边界**

先执行：

```bash
loopx configure-goal \
  --goal-id ming-iot-framework-v1 \
  --clear-boundary-authority \
  --execute
```

确认返回 `ok=true`、`written=true` 后，立即执行 Step 2 的新边界命令并追加 `--execute`。若任一步失败，停止后续实现，保留备份并报告，不手工编辑 Registry。

- [ ] **Step 5：回读最终边界**

运行：

```bash
loopx --format json quota should-run \
  --goal-id ming-iot-framework-v1 \
  --agent-id ming-peer-01 \
  | jq '.goal_boundary'
```

预期：`checkpointed_boundary_authority.active_count=1`，当前 checkpointed boundary 只覆盖三项路径；没有旧 Graph scopes，也没有扩大到 `backend/**`、整个仓库或生产系统。`coordination.write_scope` 可以保留历史配置，但运行时 checkpointed authority 必须是本次三项边界。

## Task 2：TDD 实现 Authority 目录校验器

**文件：**

- 新建：`ming-iot-rd-docs/tests/test_authority_catalog.py`
- 新建：`ming-iot-rd-docs/tools/check_authority_catalog.py`

- [ ] **Step 1：先写第一个行为测试**

先只写 `test_valid_catalog_and_registry_pass` 及其最小夹具。附录 A 是最终测试文件，不得在 Red 前一次性复制全部测试。

测试夹具固定使用以下最小 Registry 形状，避免依赖真实用户目录：

```json
{
  "goals": [
    {
      "id": "ming-iot-framework-v1",
      "authority_registry": {
        "project_materials": {
          "design-source": {
            "id": "design-source",
            "source_ref_sha256": "abc123"
          }
        },
        "topic_authority": {
          "design": "design-source"
        }
      }
    }
  ]
}
```

每个测试使用 `tempfile.TemporaryDirectory()` 创建 `docs_root`、`DOC_REGISTRY.yaml` 和 `registry.json`。测试只断言错误代码，不断言机器绝对路径。

- [ ] **Step 2：建立可运行的 Red**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot-rd-docs
python3 -m unittest tests.test_authority_catalog -v
```

第一次运行可能因模块不存在而报导入错误；这只是测试环境错误，不算 Red。随后只创建最小接口脚手架：`Issue`、`Catalog` 和返回 `[Issue("NOT_IMPLEMENTED")]` 的 `validate`，不得实现解析或校验逻辑。再次运行，必须看到 `test_valid_catalog_and_registry_pass` 因期望 `[]`、实际得到 `NOT_IMPLEMENTED` 而断言失败，这才是有效 Red。

- [ ] **Step 3：按行为逐项 Red-Green 实现最小校验器**

先实现让有效目录测试通过的最小解析与校验；确认 Green 后，再按附录 A 的顺序每次增加一个负向测试，先观察该测试按预期失败，再增加最小实现。附录 B 只作为最终接口与边界参考，不得跳过逐项 Red-Green 直接复制。

实现约束：

1. `read_catalog` 只解析顶层 `default_entry_docs` 列表和 `topic_authority` 字典；忽略其他元数据，不引入 PyYAML。
2. 路径必须是 POSIX 仓库内相对路径；拒绝绝对路径、反斜杠、空路径、`.`、`..`、软链接和非普通文件。
3. Registry 必须存在唯一 `id == goal_id` 的 Goal，并读取其 `authority_registry.project_materials` 与 `authority_registry.topic_authority`。
4. Registry 中每个 Topic 必须存在于目录，且其 Source ID 必须存在于 `project_materials`。
5. 每个 `project_materials` 条目必须至少被一个 Registry Topic 引用；目录中尚未激活的额外 Topic 允许存在。
6. 错误代码固定为：
   - `CATALOG_UNREADABLE`
   - `CATALOG_FORMAT_INVALID`
   - `DOC_PATH_INVALID`
   - `DOC_NOT_FOUND`
   - `REGISTRY_UNREADABLE`
   - `GOAL_NOT_FOUND`
   - `TOPIC_DOCUMENT_MISSING`
   - `TOPIC_SOURCE_MISSING`
   - `MATERIAL_UNREFERENCED`
7. CLI 成功输出真实计数；本项目完成迁移后的结果必须是 `authority catalog: OK topics=8 materials=8`。
8. CLI 失败时每行只输出错误代码及非敏感的 `topic`、`source_id`、原始相对 `path`；不得输出 `Path.resolve()` 结果、原始 URL、正文或 Registry 全文。

CLI 参数固定为 `--catalog`、`--docs-root`、`--registry` 和 `--goal-id`；真实项目命令在 Task 3 Step 2 中给出，不允许执行者自行猜路径。

- [ ] **Step 4：运行 Green**

```bash
python3 -m unittest tests.test_authority_catalog -v
```

预期：7 个测试全部通过。

- [ ] **Step 5：运行文档仓库全量策略测试**

```bash
python3 -m unittest discover -s tests -v
```

预期：既有归档与本地仓库策略测试仍通过。

## Task 3：建立 `DOC_REGISTRY.yaml` 权威表面

**文件：**

- 新建：`ming-iot-rd-docs/meta/DOC_REGISTRY.yaml`

- [ ] **Step 1：创建目录文件**

使用以下内容，不写绝对路径或 URL：

```yaml
version: 1
updated_at: 2026-08-26
purpose: ming-iot 项目长期研发资料入口
status_definitions:
  current: 当前实施与决策依据
  historical: 仅用于理解历史约束
default_entry_docs:
  - README.md
topic_authority:
  development_control_plane: specs/2026-08-24-ming-iot-direct-loopx-harness-retirement-design.md
  first_delivery_decision: proposals/2026-08-23-first-real-delivery-loop.md
  legacy_graph_retirement: specs/2026-08-23-ming-iot-rd-docs-archive-and-legacy-graph-retirement-design.md
  local_share_migration: proposals/2026-08-25-ming-iot-local-share-migration.md
  dmsvr_proto_domain_split: proposals/2026-08-25-dmsvr-proto-domain-split.md
  engineering_organization_reference: references/unitedrhino-tech-arch-02.md
  proto_generation_historical_reference: archives/snapshots/2026-08-23/ming-iot/docs/superpowers/specs/2026-07-23-core-goctl-migration-design.md
  project_doc_registry: meta/DOC_REGISTRY.yaml
```

- [ ] **Step 2：运行真实目录与当前 Registry 校验**

从 workspace 根运行：

```bash
python3 ming-iot-rd-docs/tools/check_authority_catalog.py \
  --catalog ming-iot-rd-docs/meta/DOC_REGISTRY.yaml \
  --docs-root ming-iot-rd-docs \
  --registry ming-iot/.loopx/registry.json \
  --goal-id ming-iot-framework-v1
```

预期：当前 7 个已登记来源全部有 Topic 与正文映射，校验通过；`project_doc_registry` 此时可以作为尚未激活的候选 Topic 存在。

- [ ] **Step 3：做负向真实夹具检查**

只在临时目录复制目录文件，删除一个 Topic 或改成不存在路径后运行校验器；预期分别得到 `TOPIC_DOCUMENT_MISSING` 或 `DOC_NOT_FOUND`。不得修改真实 Registry 制造失败。

## Task 4：把固定顺序写入跨 Agent 入口

**文件：**

- 修改：`ming-iot/.worktrees/authority-registration-guard/AGENTS.md`

- [ ] **Step 1：证明旧规则缺失**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/authority-registration-guard
rg -n "持久资料登记|loopx-doc-registry|source_ref_sha256" AGENTS.md
```

预期：无匹配。

- [ ] **Step 2：新增最小规则**

在“开始修改前”之后增加：

```markdown
## 持久资料登记

- 需要跨 Chat 保留的设计、Proposal、研究资料、SOP、Evidence 或外部资料必须使用 `loopx-doc-registry`。
- 固定顺序是：先更新 Active Goal 指向的项目权威表面并验证正文可读，再 dry-run，正式登记脱敏哈希，执行 `refresh-state`，最后回读状态。
- 正文无法定位、目标 Goal 不明确或读取权限不明确时，停止登记并写入 Todo/Blocker。
- 只有 `source_ref_sha256` 而没有可读取文档映射的记录不是完整登记；Authority 或文档目录变化后必须运行项目侧聚焦校验。
```

不得把本机绝对路径、外部私有 URL 或 LoopX 内部实现细节写入 `AGENTS.md`。

- [ ] **Step 3：验证规则可被任意 Agent 发现**

```bash
rg -n "持久资料登记|loopx-doc-registry|固定顺序|不是完整登记|聚焦校验" AGENTS.md
git diff --check -- AGENTS.md
```

预期：五项关键语义都可检索，`git diff --check` 通过。

## Task 5：补齐 Active Goal 入口并登记目录摘要

**文件：**

- 修改（LoopX 本地私有）：`ming-iot/.codex/goals/ming-iot-framework-v1/ACTIVE_GOAL_STATE.md`
- 修改（LoopX 本地私有）：`ming-iot/.loopx/registry.json`

> 此 Task 必须从已注册主项目根 `ming-iot/` 执行。`.codex/` 是本地私有状态，不随 Git worktree 复制；不要在 worktree 内伪造第二份 Goal。

- [ ] **Step 1：更新 Active Goal 的 Authority Sources**

在 `## Authority Sources` 首项增加：

```markdown
- 项目文档目录位于项目根相对路径 `../ming-iot-rd-docs/meta/DOC_REGISTRY.yaml`；新 Chat 必须先读取该目录，再按 Topic 打开正文。
- `.loopx/registry.json` 中的 `source_ref_sha256` 只是来源指纹，不是文件定位器；没有目录映射时不得声称资料可恢复。
```

同时更新 front matter 的 `updated_at`，不删除既有 Todo 元数据。

- [ ] **Step 2：dry-run 导入文档目录摘要**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
loopx import-doc-registry-authority \
  --goal-id ming-iot-framework-v1 \
  --source-id ming-iot-doc-registry \
  --doc-registry-path ../ming-iot-rd-docs/meta/DOC_REGISTRY.yaml \
  --source-kind doc_registry \
  --role project_authority_catalog \
  --freshness current \
  --owner-status owner_approved \
  --gate-status readable \
  --boundary local_private \
  --revision approved-2026-08-26 \
  --conflict-rule '项目目录负责正文定位；单项 Authority 的 freshness、revision 与 conflict_rule 继续负责语义裁决。' \
  --topic project_doc_registry \
  --dry-run
```

预期：新增一个 `ming-iot-doc-registry` material，并只新增 `project_doc_registry` Topic；现有 7 个 Topic 到单项来源的映射保持不变。

- [ ] **Step 3：正式导入并刷新**

确认 dry-run 后，移除 `--dry-run` 执行同一命令，再运行：

```bash
loopx refresh-state \
  --goal-id ming-iot-framework-v1 \
  --agent-id ming-peer-01 \
  --progress-scope goal
```

- [ ] **Step 4：回读 Registry 与状态**

```bash
jq '.goals[] | select(.id == "ming-iot-framework-v1") | .authority_registry | {topic_authority, project_material_count, topic_authority_count}' .loopx/registry.json
loopx --format json status --goal-id ming-iot-framework-v1
loopx --format json diagnose --goal-id ming-iot-framework-v1
```

预期：8 个 material、8 个 Topic；`project_doc_registry -> ming-iot-doc-registry`；状态仍指向稳定 Goal，没有新健康 blocker。

- [ ] **Step 5：重新运行真实交叉校验**

```bash
cd /home/wangjunbo/workspace-iot-platform
python3 ming-iot-rd-docs/tools/check_authority_catalog.py \
  --catalog ming-iot-rd-docs/meta/DOC_REGISTRY.yaml \
  --docs-root ming-iot-rd-docs \
  --registry ming-iot/.loopx/registry.json \
  --goal-id ming-iot-framework-v1
```

预期：`authority catalog: OK topics=8 materials=8`。

## Task 6：最终验证、独立复核与用户检查点

**文件：**

- 检查：`ming-iot/.worktrees/authority-registration-guard/AGENTS.md`
- 检查：`ming-iot-rd-docs/meta/DOC_REGISTRY.yaml`
- 检查：`ming-iot-rd-docs/tools/check_authority_catalog.py`
- 检查：`ming-iot-rd-docs/tests/test_authority_catalog.py`
- 检查（本地私有）：`ming-iot/.codex/goals/ming-iot-framework-v1/ACTIVE_GOAL_STATE.md`
- 检查（本地私有）：`ming-iot/.loopx/registry.json`

- [ ] **Step 1：运行最小完整验证集**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot-rd-docs
python3 -m unittest discover -s tests -v

cd /home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/authority-registration-guard
git diff --check

cd /home/wangjunbo/workspace-iot-platform
python3 ming-iot-rd-docs/tools/check_authority_catalog.py \
  --catalog ming-iot-rd-docs/meta/DOC_REGISTRY.yaml \
  --docs-root ming-iot-rd-docs \
  --registry ming-iot/.loopx/registry.json \
  --goal-id ming-iot-framework-v1
```

- [ ] **Step 2：使用 `superpowers:requesting-code-review` 做独立复核**

复核重点：

1. 是否修改了 LoopX 源码或引入非标准库依赖；
2. 是否存在绝对路径、原始 URL、正文或 Registry 全文泄漏；
3. 校验器是否允许目录候选 Topic，同时拒绝 Registry 裸哈希；
4. 当前 Todo、Active Goal、Registry 和 Git diff 是否一致；
5. 是否误把聚焦校验接入全量构建或 pre-push。

- [ ] **Step 3：展示变更，不提交**

```bash
git -C /home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/authority-registration-guard status --short
git -C /home/wangjunbo/workspace-iot-platform/ming-iot-rd-docs status --short
```

只展示预期文件，并明确区分：

- `ming-iot/AGENTS.md` 是待合并的公开仓库变更；
- `.codex/` 与 `.loopx/` 是本地私有控制面状态；
- `ming-iot-rd-docs` 是本地事实源，不得添加 remote 或 push。

- [ ] **Step 4：等待单独授权**

此处停止。未经用户分别授权，不执行 `git add`、`git commit`、merge 或 push，也不完成 `todo_9677142604e3`。

用户批准提交与合并后，再执行以下收尾：

1. 提交并合并 `AGENTS.md`；
2. 在最终树重跑 Task 6 Step 1；
3. 用 `loopx todo complete` 完成 `todo_9677142604e3`，附上验证结果；
4. 原子创建后继 Agent Todo：为 Things 可重复启动与依赖契约基线编写并审批实施规格；
5. `refresh-state --progress-scope goal` 并回读状态。

## 非目标检查

- [ ] 未修改 `/home/wangjunbo/workspace-loopx/loopx`。
- [ ] 未创建 `authority_source_registration_v1`、Locator Store 或 Resolve API。
- [ ] 未把校验器加入每次全量构建、pre-push 或生产流程。
- [ ] 未修改 `iot-doc/`、`things/` 或 `archives/` 中的历史快照。
- [ ] 未自动 commit、merge、push 或写远端系统。

## 附录 A：`tests/test_authority_catalog.py` 完整初始测试

```python
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from tools.check_authority_catalog import validate


REPO = Path(__file__).resolve().parents[1]
SCRIPT = REPO / "tools" / "check_authority_catalog.py"


class AuthorityCatalogTest(unittest.TestCase):
    def make_fixture(self, parent: Path) -> tuple[Path, Path, Path]:
        docs_root = parent / "docs"
        (docs_root / "specs").mkdir(parents=True)
        (docs_root / "archives").mkdir()
        (docs_root / "README.md").write_text("entry\n", encoding="utf-8")
        (docs_root / "specs" / "design.md").write_text("design\n", encoding="utf-8")
        (docs_root / "archives" / "historical.md").write_text("history\n", encoding="utf-8")
        catalog = docs_root / "DOC_REGISTRY.yaml"
        registry = parent / "registry.json"
        self.write_catalog(catalog)
        self.write_registry(registry)
        return docs_root, catalog, registry

    def write_catalog(
        self,
        path: Path,
        *,
        design_path: str = "specs/design.md",
        history_path: str = "archives/historical.md",
        include_design: bool = True,
    ) -> None:
        topic_lines = []
        if include_design:
            topic_lines.append(f"  design: {design_path}")
        topic_lines.append(f"  history: {history_path}")
        path.write_text(
            "\n".join(
                [
                    "version: 1",
                    "default_entry_docs:",
                    "  - README.md",
                    "topic_authority:",
                    *topic_lines,
                    "",
                ]
            ),
            encoding="utf-8",
        )

    def write_registry(
        self,
        path: Path,
        *,
        design_source: str = "design-source",
        extra_material: bool = False,
        raw_source_ref: str | None = None,
    ) -> None:
        materials: dict[str, object] = {
            "design-source": {
                "id": "design-source",
                "source_ref_sha256": "abc123",
            },
            "history-source": {
                "id": "history-source",
                "source_ref_sha256": "def456",
            },
        }
        if raw_source_ref is not None:
            materials["design-source"]["source_ref"] = raw_source_ref  # type: ignore[index]
        if extra_material:
            materials["orphan-source"] = {
                "id": "orphan-source",
                "source_ref_sha256": "orphan",
            }
        document = {
            "goals": [
                {
                    "id": "ming-iot-framework-v1",
                    "authority_registry": {
                        "project_materials": materials,
                        "topic_authority": {
                            "design": design_source,
                            "history": "history-source",
                        },
                    },
                }
            ]
        }
        path.write_text(json.dumps(document), encoding="utf-8")

    def test_valid_catalog_and_registry_pass(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root, catalog, registry = self.make_fixture(Path(temporary))
            self.assertEqual(
                validate(catalog, docs_root, registry, "ming-iot-framework-v1"),
                [],
            )

    def test_unreferenced_material_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root, catalog, registry = self.make_fixture(Path(temporary))
            self.write_registry(registry, extra_material=True)
            codes = {issue.code for issue in validate(catalog, docs_root, registry, "ming-iot-framework-v1")}
            self.assertIn("MATERIAL_UNREFERENCED", codes)

    def test_topic_pointing_to_missing_source_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root, catalog, registry = self.make_fixture(Path(temporary))
            self.write_registry(registry, design_source="missing-source")
            codes = {issue.code for issue in validate(catalog, docs_root, registry, "ming-iot-framework-v1")}
            self.assertIn("TOPIC_SOURCE_MISSING", codes)

    def test_missing_document_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root, catalog, registry = self.make_fixture(Path(temporary))
            self.write_catalog(catalog, design_path="specs/missing.md")
            issues = validate(catalog, docs_root, registry, "ming-iot-framework-v1")
            self.assertIn(("DOC_NOT_FOUND", "specs/missing.md"), {(issue.code, issue.path) for issue in issues})

    def test_absolute_and_parent_paths_are_rejected(self) -> None:
        for invalid in ("/tmp/private.md", "../private.md", "specs\\private.md"):
            with self.subTest(path=invalid), tempfile.TemporaryDirectory() as temporary:
                docs_root, catalog, registry = self.make_fixture(Path(temporary))
                self.write_catalog(catalog, design_path=invalid)
                issues = validate(catalog, docs_root, registry, "ming-iot-framework-v1")
                self.assertIn(("DOC_PATH_INVALID", invalid), {(issue.code, issue.path) for issue in issues})

    def test_historical_archive_document_is_allowed(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root, catalog, registry = self.make_fixture(Path(temporary))
            issues = validate(catalog, docs_root, registry, "ming-iot-framework-v1")
            self.assertNotIn("DOC_PATH_INVALID", {issue.code for issue in issues})
            self.assertEqual(issues, [])

    def test_cli_output_does_not_leak_absolute_path_or_raw_source_ref(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root, catalog, registry = self.make_fixture(Path(temporary))
            self.write_catalog(catalog, design_path="specs/missing.md")
            secret = "https://internal.example.invalid/private-doc"
            self.write_registry(registry, raw_source_ref=secret)
            result = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--catalog",
                    str(catalog),
                    "--docs-root",
                    str(docs_root),
                    "--registry",
                    str(registry),
                    "--goal-id",
                    "ming-iot-framework-v1",
                ],
                cwd=REPO,
                check=False,
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            combined = result.stdout + result.stderr
            self.assertEqual(result.returncode, 1)
            self.assertIn("DOC_NOT_FOUND", combined)
            self.assertIn("path=specs/missing.md", combined)
            self.assertNotIn(str(docs_root), combined)
            self.assertNotIn(secret, combined)


if __name__ == "__main__":
    unittest.main()
```

## 附录 B：`tools/check_authority_catalog.py` 完整最小实现

```python
from __future__ import annotations

import argparse
import json
import sys
from dataclasses import dataclass
from pathlib import Path, PurePosixPath
from typing import Any


class AuthorityCatalogError(RuntimeError):
    pass


@dataclass(frozen=True, order=True)
class Issue:
    code: str
    topic: str = ""
    source_id: str = ""
    path: str = ""

    def render(self) -> str:
        fields = [self.code]
        if self.topic:
            fields.append(f"topic={self.topic}")
        if self.source_id:
            fields.append(f"source_id={self.source_id}")
        if self.path:
            fields.append(f"path={self.path}")
        return " ".join(fields)


@dataclass(frozen=True)
class Catalog:
    default_entry_docs: tuple[str, ...]
    topic_authority: dict[str, str]


def _scalar(value: str) -> str:
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {'"', "'"}:
        value = value[1:-1]
    if not value:
        raise AuthorityCatalogError("CATALOG_FORMAT_INVALID")
    return value


def read_catalog(path: Path) -> Catalog:
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except OSError as error:
        raise AuthorityCatalogError("CATALOG_UNREADABLE") from error

    defaults: list[str] = []
    topics: dict[str, str] = {}
    section: str | None = None
    for raw in lines:
        stripped = raw.strip()
        if not stripped or stripped.startswith("#"):
            continue
        if not raw.startswith((" ", "\t")):
            key, separator, remainder = stripped.partition(":")
            if separator and not remainder.strip() and key in {"default_entry_docs", "topic_authority"}:
                section = key
            else:
                section = None
            continue
        if section == "default_entry_docs":
            if not stripped.startswith("- "):
                raise AuthorityCatalogError("CATALOG_FORMAT_INVALID")
            defaults.append(_scalar(stripped[2:]))
        elif section == "topic_authority":
            topic, separator, value = stripped.partition(":")
            topic = topic.strip()
            if not separator or not topic or topic in topics:
                raise AuthorityCatalogError("CATALOG_FORMAT_INVALID")
            topics[topic] = _scalar(value)

    if not defaults or not topics:
        raise AuthorityCatalogError("CATALOG_FORMAT_INVALID")
    return Catalog(tuple(defaults), topics)


def read_goal_authority(
    registry_path: Path,
    goal_id: str,
) -> tuple[dict[str, object], dict[str, str]]:
    try:
        document = json.loads(registry_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise AuthorityCatalogError("REGISTRY_UNREADABLE") from error
    goals = document.get("goals")
    if not isinstance(goals, list):
        raise AuthorityCatalogError("REGISTRY_UNREADABLE")
    matches = [goal for goal in goals if isinstance(goal, dict) and goal.get("id") == goal_id]
    if len(matches) != 1:
        raise AuthorityCatalogError("GOAL_NOT_FOUND")
    authority = matches[0].get("authority_registry")
    if not isinstance(authority, dict):
        raise AuthorityCatalogError("REGISTRY_UNREADABLE")
    materials = authority.get("project_materials")
    topics = authority.get("topic_authority")
    if not isinstance(materials, dict) or not isinstance(topics, dict):
        raise AuthorityCatalogError("REGISTRY_UNREADABLE")
    if not all(isinstance(key, str) and isinstance(value, dict) for key, value in materials.items()):
        raise AuthorityCatalogError("REGISTRY_UNREADABLE")
    if not all(isinstance(key, str) and isinstance(value, str) for key, value in topics.items()):
        raise AuthorityCatalogError("REGISTRY_UNREADABLE")
    return materials, topics


def _document_issue(docs_root: Path, relative: str) -> Issue | None:
    if "\\" in relative:
        return Issue("DOC_PATH_INVALID", path=relative)
    pure = PurePosixPath(relative)
    if pure.is_absolute() or not pure.parts or any(part in {"", ".", ".."} for part in pure.parts):
        return Issue("DOC_PATH_INVALID", path=relative)
    candidate = docs_root.joinpath(*pure.parts)
    if candidate.is_symlink():
        return Issue("DOC_PATH_INVALID", path=relative)
    try:
        root_resolved = docs_root.resolve(strict=True)
        candidate_resolved = candidate.resolve(strict=True)
        candidate_resolved.relative_to(root_resolved)
    except (OSError, ValueError):
        return Issue("DOC_NOT_FOUND", path=relative)
    if not candidate_resolved.is_file():
        return Issue("DOC_NOT_FOUND", path=relative)
    return None


def validate(
    catalog_path: Path,
    docs_root: Path,
    registry_path: Path,
    goal_id: str,
) -> list[Issue]:
    catalog = read_catalog(catalog_path)
    materials, registry_topics = read_goal_authority(registry_path, goal_id)
    issues: set[Issue] = set()

    for relative in (*catalog.default_entry_docs, *catalog.topic_authority.values()):
        issue = _document_issue(docs_root, relative)
        if issue is not None:
            issues.add(issue)

    for topic, source_id in registry_topics.items():
        if topic not in catalog.topic_authority:
            issues.add(Issue("TOPIC_DOCUMENT_MISSING", topic=topic, source_id=source_id))
        if source_id not in materials:
            issues.add(Issue("TOPIC_SOURCE_MISSING", topic=topic, source_id=source_id))

    referenced = set(registry_topics.values())
    for source_id in materials:
        if source_id not in referenced:
            issues.add(Issue("MATERIAL_UNREFERENCED", source_id=source_id))

    return sorted(issues)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Validate project Authority sources against a readable document catalog")
    parser.add_argument("--catalog", required=True, type=Path)
    parser.add_argument("--docs-root", required=True, type=Path)
    parser.add_argument("--registry", required=True, type=Path)
    parser.add_argument("--goal-id", required=True)
    arguments = parser.parse_args(argv)
    try:
        issues = validate(arguments.catalog, arguments.docs_root, arguments.registry, arguments.goal_id)
        materials, topics = read_goal_authority(arguments.registry, arguments.goal_id)
    except AuthorityCatalogError as error:
        print(str(error), file=sys.stderr)
        return 1
    if issues:
        for issue in issues:
            print(issue.render(), file=sys.stderr)
        return 1
    print(f"authority catalog: OK topics={len(topics)} materials={len(materials)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
```
