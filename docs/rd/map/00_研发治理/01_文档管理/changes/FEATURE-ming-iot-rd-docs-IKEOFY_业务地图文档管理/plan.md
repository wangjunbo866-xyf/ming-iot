---
document_kind: plan
status: approved
delivery_id: ming-iot-rd-docs#IKEOFY
issue: https://gitee.com/ming-industry/ming-iot-rd-docs/issues/IKEOFY
hub: map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/README.md
change_spec: map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/change.md
affected_repositories:
  - git:gitee.com/ming-industry/ming-iot-rd-docs
  - git:gitee.com/ming-industry/ming-iot
---

# 业务地图文档管理实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `ming-iot-rd-docs#IKEOFY` 建立按业务域和功能模块组织的文档系统，使 Delivery Issue、Change Spec、Plan 与 PR 可互相定位，并用仓库脚本、Git Hook 和 Gitee CI 执行确定性校验。

**Architecture:** `ming-iot-rd-docs` 保存唯一文档模型、`map-delivery` Skill、Python CLI、测试和当前规格；`ming-iot` 只晋升代码贡献者必须知道的文档入口与 PR 契约。实施采用“文档仓基础能力 PR → 产品仓契约 PR → 文档仓权威激活 PR”的三段顺序，任何中断都保持旧顶层目录为当前权威。

**Tech Stack:** Python 3 标准库、`unittest`、Markdown/YAML Front Matter、POSIX shell、Git linked worktree、Gitee Go YAML、Agent Skills、Go 1.24 仓库契约测试。

---

**模块主规格（完整要求，先读）：** [spec.md](../../spec.md)

**本次 Change（首次落地范围，再读）：** [change.md](./change.md)

**Delivery Issue：** [ming-iot-rd-docs#IKEOFY](https://gitee.com/ming-industry/ming-iot-rd-docs/issues/IKEOFY)

## 2026-09-17 PR !9 独立复审修复

Owner 对 `959b7327c78a63a359db8644f68e252562630b4b` 的 [复审](https://gitee.com/ming-industry/ming-iot-rd-docs/pulls/9#note_51240711) 为 REQUEST_CHANGES。沿用阶段 A 的修复交付，不进入 B/C，不合并：

- N1：分离真实锚点定位与原始段落比较；行内代码、围栏配置和缩进代码的变更必须触发默认 check/真实 Hook 的激活核验，保留未变化历史的放行控制。
- N2：完整闭合及标题范围决定内联导航；段落中的伪引用定义不算导航，保留实际使用的合法引用。原有失效引用测试补上定义前空行，使其测量真正的引用链接而不是段落文字。
- N3：独立核对 Issue 与 Hub 身份，允许明确的 `ming-industry/ming-iot` → `ming-industry/ming-iot-rd-docs` 组合，同时保持可信主机、组织、仓库及 map Change README 路径边界；工具仍只 GET。
- 同步阶段 A Issue 模板，说明退出 2 → 人工补唯一导航块 → 回读确认，不再声称自动改写。

完成本地验证后更新原 PR 的精确候选及 Evidence，交回 Owner 独立复审。执行者本地检查不替代独立审查，旧 Head 的通过不覆盖本轮。

## 2026-09-13 PR !9 审查修订（优先于下文原始步骤）

用户批准仅修复 [Owner Review R1～R3](https://gitee.com/ming-industry/ming-iot-rd-docs/pulls/9#note_51185613)，更新原 PR 并交回独立复审；不合并、不推进 B/C。

- R1：原 Task 4 的 GET/PATCH/GET 不具备并发保护，停止自动写 Issue 正文和标题。工具只生成导航块或 GET 回读；待人工更新退出 2，成功验证退出 0。下文原始写入步骤不再执行。
- R2：默认 check、pre-push、CI 共用候选检查；相对可信基线新增/修改的 effective Change 及目标主规格变化复用 activate 门禁。没有默认基线则保守检查全部；不将普通 verifying 和未变化历史 effective 误判为新激活。
- R3：导航只计入真正使用的链接；引用定义本身不是可见导航，仍可检查其目标是否失效。
- 先复现每条反例，再补实现与回归测试；保留原历史 Review 的精确 Head，不能当作修复后 Head 的通过结论。修复后补新验证摘要，等待 Owner 独立复审。

## 执行拓扑与提交边界

### 2026-09-13 编写顺序修订

正常顺序为“模块初版主规格 → Change/Plan → 实施与验证 → 生效后维护主规格”。本次已补正模块初版，不能再把 Task 8 理解为首次定义模块；Task 8 只负责在前置条件满足时核验并激活已定义的规格，以及明确另行获批的迁移。主规格可先行定义，能力完成和权威切换仍需对应验收事实。

本修订只调整文档层级及主规格准备时点，不批准执行 B/C、自动迁移旧文档或合并。未迁移模块后续由用户手工组织，以下旧迁移步骤须在重新确认具体范围后执行。

历史上本计划先发布 Approved Change Spec 和 Approved Plan，后补主规格；已有合入和评审事实保持不变。以下三个仓库级阶段仍用于交付依赖；Task/Step 是 Todo 内部检查点，不逐项创建 LoopX Todo。

| 阶段 | Agent Todo 所属仓库 | Tasks | 产物 | 进入条件 |
|---|---|---|---|---|
| A：文档基础能力 | `git:gitee.com/ming-industry/ming-iot-rd-docs` | 1–6 | 校验器、Skill、初始化器、本地与 CI 门禁 | 本 Plan 已合入 master |
| B：产品仓契约晋升 | `git:gitee.com/ming-industry/ming-iot` | 7 | Agent 入口、PR 模板、仓库契约测试 | 阶段 A PR 已合入 |
| C：权威激活与迁移 | `git:gitee.com/ming-industry/ming-iot-rd-docs` | 8–10 | 已定义主规格的激活核验、ADR、获批迁移指针、Review、Evidence | 阶段 B PR 已合入；迁移范围重新确认 |

每个阶段从对应仓库最新 `origin/master` 创建独立 worktree。阶段 B 不检出私有文档仓到产品 CI；阶段 C 通过已合入的文档仓 revision 和产品仓 revision 核验跨仓事实。三个阶段分别形成 PR，不能宣称跨仓原子提交。

### 中断恢复

- 阶段 A 未合入：删除候选 worktree 即可，旧文档规则完全不变。
- 阶段 A 已合入、阶段 B 失败：新工具保持可用但 `map/` 仍是试点，根 `AGENTS.md` 不切换权威。
- 阶段 B 已合入、阶段 C 失败：产品仓已经知道新入口，但入口明确显示“设计试点”；旧顶层目录继续权威。
- 阶段 C 候选与任一已合入 Head 不一致：重新基于 `origin/master` 生成候选，重新执行 Review 和 Evidence，不复用旧 verdict。

## 文件职责

### 阶段 A：`ming-iot-rd-docs`

| 路径 | 操作 | 单一职责 |
|---|---|---|
| `tools/map_docs.py` | 新建 | 解析、初始化和校验业务地图文档；不调用模型和 Gitee |
| `tests/test_map_docs.py` | 新建 | 覆盖目录、元数据、链接、锚点、冲突、初始化和激活门禁 |
| `tools/gitee_issue.py` | 新建 | 幂等写入并回读 Issue 中的 Change 导航托管块 |
| `tests/test_gitee_issue.py` | 新建 | 使用本地 HTTP 服务验证 Issue 更新和脱敏边界 |
| `.gitee/ISSUE_TEMPLATE/研发治理.md` | 新建 | 统一研发治理 Issue 的人工录入结构 |
| `.agents/skills/map-delivery/SKILL.md` | 新建 | 唯一正式文档写作流程和触发描述 |
| `.agents/skills/map-delivery/references/workflow.md` | 新建 | Skill 按需读取的字段、生命周期和命令参考 |
| `tools/setup_workspace_skills.py` | 新建 | 为各 Coding Agent 创建本地 Skill 注册链接 |
| `tests/test_setup_workspace_skills.py` | 新建 | 验证跨工作区链接、幂等和拒绝覆盖行为 |
| `.githooks/pre-push` | 修改 | 保留 remote 限制并增加 `map_docs.py check` |
| `.workflow/map-docs-check.yml` | 新建 | 在文档仓 PR 上运行确定性测试和 map 检查 |
| `tests/test_repository_policy.py` | 修改 | 固定 Hook、Workflow 和 canonical Skill 契约 |

### 阶段 B：`ming-iot`

| 路径 | 操作 | 单一职责 |
|---|---|---|
| `AGENTS.md` | 修改 | 告诉 Agent 如何从产品仓定位 Delivery Hub 和安装本地 Skill 注册 |
| `.gitee/PULL_REQUEST_TEMPLATE.zh-CN.md` | 修改 | 将旧 `RFC / Spec / Plan` 表改为 Change Hub 四向导航字段 |
| `.gitignore` | 修改 | 忽略 setup 脚本生成的 `/.agents/`、`/.claude/` 本地注册 |
| `backend/tools/internal/repositorycontract/verification_profile_test.go` | 修改 | 机器校验产品仓 PR 模板和 Agent 入口 |

### 阶段 C：`ming-iot-rd-docs`

| 路径 | 操作 | 单一职责 |
|---|---|---|
| `map/README.md` | 修改 | 从试点入口切换为默认业务地图入口 |
| `map/00_研发治理/01_文档管理/README.md` | 修改 | 提供当前规格、活动 Change 和历史 Change 索引 |
| `map/00_研发治理/01_文档管理/spec.md` | 新建 | 汇总文档管理模块已经生效的完整当前规格 |
| `map/00_研发治理/01_文档管理/adr/0001-business-map-delivery-authority.md` | 新建 | 记录采用业务地图和 Change 增量模型的长期决策 |
| `map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/{README.md,change.md,plan.md,review.md,evidence.md}` | 修改/新建 | 完成本 Change 的状态、导航、评审与证据闭环 |
| `AGENTS.md` | 修改 | 将新交付默认入口切换为 `map/`，保留未迁移模块的旧目录规则 |
| `README.md` | 修改 | 说明业务地图、旧目录和归档边界 |
| `meta/DOC_REGISTRY.yaml` | 修改 | 把已迁移的文档管理 Authority topic 指向模块主规格或 Change |
| `specs/2026-08-30-ming-iot-dual-repository-and-rd-docs-worktree-cutover-design.md` | 修改 | 增加 Superseded 指针，不改写历史正文 |
| `specs/2026-09-03-ming-iot-gitee-delivery-and-verification-profile-design.md` | 修改 | 状态改为 superseded 并指向模块主规格 |
| `specs/2026-09-04-gitee-draft-pr-automation-and-rd-docs-authorization-alignment-design.md` | 修改 | 状态改为 superseded 并指向模块主规格 |
| `plans/2026-08-30-ming-iot-dual-repository-and-rd-docs-worktree-cutover.md` | 修改 | 增加历史计划说明与新入口 |
| `plans/2026-09-03-ming-iot-gitee-delivery-and-verification-profile.md` | 修改 | 状态改为 completed 并指向本 Change |
| `plans/2026-09-04-gitee-draft-pr-automation-and-rd-docs-authorization-alignment.md` | 修改 | 状态改为 completed 并指向本 Change |

## Task 0：批准并合入设计与计划

**Files:**

- Modify: `map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/plan.md`
- Modify: `map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/README.md`

- [x] **Step 1：用户评审本 Plan**

确认任务边界、三段 PR 顺序、迁移文件清单和恢复路径。通过后把 `plan.md` Front Matter 的 `status` 从 `draft` 改为 `approved`，并把 Change `README.md` 中 Plan 状态同步为 Approved。

- [x] **Step 2：验证设计分支只包含文档**

Run:

```bash
python -m unittest discover -s tests -v
python tools/check_delivery_metadata.py --docs-root . --since 2026-09-10
git diff --check origin/master...HEAD
git diff --name-only origin/master...HEAD
```

Expected: 单元测试全部通过；旧元数据检查返回 `delivery metadata: OK`；变更只包含 `map/README.md` 和 `map/00_研发治理/01_文档管理/**`。旧元数据检查尚未覆盖 `map/` 的事实必须写进 PR，不作为新结构已验证的证据。

- [x] **Step 3：发布设计与计划 PR**

将当前候选分支推送到 `origin`，使用 `tools/gitee_pr.py` 创建 Draft PR。PR 正文填写 `ming-iot-rd-docs#IKEOFY`、Change Hub、本 Plan、实际测试结果和精确 Head；不执行合并。

- [x] **Step 4：人工合入后回读基线**

Run:

```bash
git fetch origin master
planning_revision="$(git rev-parse origin/master)"
git show "$planning_revision:map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/plan.md" \
  | sed -n '1,20p'
```

Expected: 文件存在且 Front Matter 中 `status: approved`。把 `planning_revision` 的完整 40 位值写入阶段 A Hub；不使用设计分支的候选 Head 代替合入后 revision。

## Task 1：用失败测试锁定 `map/` 结构和元数据

**Files:**

- Create: `tests/test_map_docs.py`
- Create: `tools/map_docs.py`

- [x] **Step 1：建立临时文档树测试夹具**

在 `tests/test_map_docs.py` 定义只使用临时目录的夹具，并固定以下测试身份：

```python
DELIVERY_ID = "ming-iot-rd-docs#IKEOFY"
ISSUE_URL = "https://gitee.com/ming-industry/ming-iot-rd-docs/issues/IKEOFY"
MODULE = "map/00_研发治理/01_文档管理"
CHANGE = (
    MODULE
    + "/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理"
)
EXACT_HEAD = "a" * 40
```

夹具函数只创建测试声明的文件，不读取真实仓库，不跟随逃出临时根目录的符号链接。

- [x] **Step 2：写合法树和目录错误测试**

测试方法固定为：

```text
test_valid_active_change_passes
test_change_directory_type_and_delivery_id_must_match_metadata
test_duplicate_delivery_id_is_rejected
test_change_must_live_under_its_target_module
test_symlink_or_parent_escape_is_rejected
test_module_root_plan_is_rejected
test_navigation_readme_may_omit_front_matter
```

Run:

```bash
python -m unittest -v tests.test_map_docs
```

Expected: FAIL，因为 `tools.map_docs` 尚未提供 `validate()` 和 CLI。

- [x] **Step 3：写文档职责和四向导航失败测试**

覆盖以下稳定错误码：

```text
MAP_FRONT_MATTER_INVALID
MAP_DOCUMENT_KIND_INVALID
MAP_STATUS_INVALID
MAP_DELIVERY_ID_INVALID
MAP_ISSUE_URL_INVALID
MAP_HUB_INVALID
MAP_REPOSITORY_INVALID
MAP_RELATED_PATH_INVALID
MAP_DOCUMENT_MISSING
MAP_DELIVERY_DUPLICATE
MAP_MODULE_PLAN_FORBIDDEN
```

测试断言错误按 `path + code` 稳定排序。缺少的 Plan、PR、Review 或 Evidence 必须在 Hub 中以状态文本表示，不能用指向不存在文件的 Markdown 链接。

- [x] **Step 4：写锚点与活动覆盖冲突测试**

测试方法固定为：

```text
test_existing_module_change_requires_stable_target_anchor
test_new_module_feature_may_use_module_root_anchor
test_two_active_changes_cannot_cover_the_same_anchor
test_rejected_and_effective_changes_do_not_conflict_with_active_change
test_effective_change_requires_spec_provenance_link
test_effective_change_anchor_must_exist_in_module_spec
```

活动状态集合固定为 `approved`、`implementing`、`verifying`；`draft` 允许并行讨论但不授权实施，`rejected`、`effective`、`superseded` 不占用活动覆盖锁。

## Task 2：实现 `check` 和 `init` 命令

**Files:**

- Modify: `tools/map_docs.py`
- Modify: `tests/test_map_docs.py`

- [x] **Step 1：实现受限 Front Matter 解析器**

只使用 Python 标准库，接受本契约定义的标量与缩进两空格的字符串列表。公共数据结构固定为：

```python
@dataclass(frozen=True, order=True)
class Problem:
    path: str
    code: str

@dataclass(frozen=True)
class FrontMatter:
    scalars: dict[str, str]
    lists: dict[str, tuple[str, ...]]

@dataclass(frozen=True)
class MapDocument:
    path: Path
    front_matter: FrontMatter
```

路径名为 `README.md` 的根、模块和 Change 导航页由路径确定职责，可以不带 Front Matter，但必须另行校验其索引和链接。其他 `map/**/*.md` 必须带 Front Matter；解析失败必须产生 `MAP_FRONT_MATTER_INVALID`，不能跳过文件。

按 `document_kind` 分别校验必填字段和状态：

| `document_kind` | 必填字段 | 有限状态 |
|---|---|---|
| `spec` | `module` | `current` |
| `change` | `change_type`、`delivery_id`、`issue`、`hub`、`target_module`、`affected_repositories`、`affected_modules`、`target_anchors` | `draft`、`approved`、`implementing`、`verifying`、`effective`、`rejected`、`superseded` |
| `plan` | `delivery_id`、`issue`、`hub`、`change_spec`、`affected_repositories` | `draft`、`approved`、`in_progress`、`completed`、`superseded` |
| `review` | `delivery_id`、`issue`、`hub`、`repository_heads` | `pass`、`changes_required` |
| `evidence` | `delivery_id`、`issue`、`hub`、`repository_heads` | `verified`、`failed` |
| `adr` | `module` | `accepted`、`superseded` |
| `reference` | `module`、`source` | `current`、`superseded` |

`repository_heads` 每项使用 `git:gitee.com/ming-industry/repository@40-hex-head` 的单行形式，并与正文中的 `repository/base/exact_head/verdict` 表交叉校验。Delivery ID 接受 `ming-iot#I...` 和 `ming-iot-rd-docs#I...`；Git identity 只接受 `git:gitee.com/ming-industry/repository`；Issue URL 的仓库和编号必须与 Delivery ID 一致。

- [x] **Step 2：实现整树 `check`**

CLI 固定为：

```bash
python tools/map_docs.py check --docs-root .
```

成功输出：

```text
map docs: OK
```

失败时每行只输出稳定错误码和仓库相对路径，例如 `MAP_HUB_INVALID path=map/00_研发治理/01_文档管理/spec.md`；返回 1，不输出正文、绝对路径或凭据。检查范围是 `map/**/*.md`，并显式拒绝模块根 `plan.md`、未知 Change 类型、重复 Delivery ID、坏链接、错误 Hub、路径逃逸和活动锚点冲突。

- [x] **Step 3：实现幂等 `init`**

CLI 固定为：

```bash
python tools/map_docs.py init \
  --docs-root . \
  --module map/00_研发治理/01_文档管理 \
  --type FEATURE \
  --delivery-id ming-iot-rd-docs#IKEOFY \
  --name 业务地图文档管理 \
  --issue https://gitee.com/ming-industry/ming-iot-rd-docs/issues/IKEOFY \
  --repository git:gitee.com/ming-industry/ming-iot-rd-docs
```

`init` 创建 Change `README.md` 和 `change.md`，不创建 `plan.md`、`review.md` 或 `evidence.md`。目标已存在且内容与请求一致时输出 `map change: existing`；同一 Delivery ID 指向不同路径或已有非空冲突文件时返回 1，不覆盖人工正文。

- [x] **Step 4：运行 Red/Green 验证并提交**

Run:

```bash
python -m unittest -v tests.test_map_docs
python tools/map_docs.py check --docs-root .
git diff --check
```

Expected: 所有 map 测试通过，真实树输出 `map docs: OK`。提交：

```bash
git add tools/map_docs.py tests/test_map_docs.py
git commit -m "feat: validate and scaffold map deliveries"
```

## Task 3：实现双向指针和激活门禁

**Files:**

- Modify: `tools/map_docs.py`
- Modify: `tests/test_map_docs.py`

- [x] **Step 1：写激活候选失败测试**

新增以下测试：

```text
test_activation_requires_effective_status_in_candidate_tree
test_activation_requires_review_and_evidence_files
test_activation_requires_pass_for_every_repository_head
test_activation_requires_all_prerequisite_deliveries_merged
test_activation_requires_module_history_index
test_activation_requires_spec_anchor_and_nearby_provenance
test_activation_rejects_unrelated_active_anchor_conflict
```

Review 与 Evidence 的仓库表必须逐行包含 `repository`、40 位 `base`、40 位 `exact_head` 和有限结论；跨仓库缺一行即失败。阶段 A 和 B 这类前置交付必须已合入；当前阶段 C Draft PR 是该候选树的载体，允许保持 open，不可能在合入前证明自己已合入。

- [x] **Step 2：实现稳定锚点解析**

主规格锚点只接受：

```html
<a id="lowercase-ascii-kebab-case"></a>
```

Change Front Matter 的 `target_anchors` 保存不带 `#` 的锚点名。新模块 Feature 在实施阶段可使用唯一保留值 `__module_root__`；进入 activation candidate 前必须替换为主规格中真实存在的锚点。

- [x] **Step 3：实现 `activate` 只读门禁**

CLI 固定为：

```bash
python tools/map_docs.py activate \
  --docs-root . \
  --change map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理
```

该命令不生成业务正文、不调用 Gitee、不修改文件。实施者先在同一候选 diff 中更新模块 `spec.md`、Change 状态、来源指针、Hub、Review 和 Evidence；`activate` 只验证这棵候选树合入后是否自洽。成功输出 `map activation: OK`，失败返回稳定错误码。

- [x] **Step 4：运行聚焦测试并提交**

Run:

```bash
python -m unittest -v tests.test_map_docs
git diff --check
```

Expected: 激活、锚点和冲突测试全部通过。提交：

```bash
git add tools/map_docs.py tests/test_map_docs.py
git commit -m "feat: gate map delivery activation"
```

## Task 4：实现 Gitee Issue 反向链接

**Files:**

- Create: `tools/gitee_issue.py`
- Create: `tests/test_gitee_issue.py`
- Create: `.gitee/ISSUE_TEMPLATE/研发治理.md`
- Read: `tools/gitee_pr.py`

- [x] **Step 1：用本地 HTTP 服务写失败测试**

复用 `tests/test_gitee_pr.py` 的安全边界，但建立独立 Issue fixture。测试覆盖：

```text
test_link_inserts_one_managed_block_and_reads_it_back
test_link_updates_existing_managed_block_without_duplication
test_link_rejects_multiple_managed_blocks
test_link_rejects_issue_repository_or_number_mismatch
test_link_rejects_redirect_without_forwarding_authorization
test_link_error_never_prints_token_or_issue_body
test_cli_rejects_real_token_for_local_test_server
```

托管块边界固定为：

```markdown
<!-- map-delivery:start -->
Delivery ID: `ming-iot-rd-docs#IKEOFY`
Change Hub: <https://gitee.com/ming-industry/ming-iot-rd-docs/blob/master/map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/README.md>
<!-- map-delivery:end -->
```

- [x] **Step 2：实现幂等 `link` 命令**

CLI 固定为：

```bash
python tools/gitee_issue.py link \
  --owner ming-industry \
  --repo ming-iot-rd-docs \
  --number IKEOFY \
  --delivery-id ming-iot-rd-docs#IKEOFY \
  --hub-url https://gitee.com/ming-industry/ming-iot-rd-docs/blob/master/map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/README.md
```

正式调用只从 `GITEE_ACCESS_TOKEN` 读取令牌并只发送到 `https://gitee.com`。命令 GET 当前 Issue、替换一个托管块、PATCH、再 GET 回读；成功只输出 Issue 编号、URL 和 `linked=true`。不得创建、关闭或删除 Issue。

- [x] **Step 3：建立 Issue 模板**

模板固定包含“背景、目标、非目标、验收条件、Delivery 导航”五段，并在导航段要求真实 Delivery ID。模板说明自然语言结构本身不是门禁，正式关联仍由 `gitee_issue.py link` 回读确认。

- [x] **Step 4：验证并提交**

Run:

```bash
python -m unittest -v tests.test_gitee_issue tests.test_gitee_pr
git diff --check
```

Expected: 所有 Gitee 工具测试通过，测试请求只发往进程内回环地址。提交：

```bash
git add tools/gitee_issue.py tests/test_gitee_issue.py .gitee/ISSUE_TEMPLATE/研发治理.md
git commit -m "feat: link Gitee issues to map deliveries"
```

## Task 5：建立唯一 `map-delivery` Skill 和跨 Agent 注册

**Files:**

- Create: `.agents/skills/map-delivery/SKILL.md`
- Create: `.agents/skills/map-delivery/references/workflow.md`
- Create: `tools/setup_workspace_skills.py`
- Create: `tests/test_setup_workspace_skills.py`

- [x] **Step 1：写 setup 脚本失败测试**

在临时目录建立 `workspace/ming-iot` 与 `workspace/ming-iot-rd-docs` 两个 Git 仓库，测试以下目标：

```text
workspace/.agents/skills/map-delivery
workspace/.claude/skills/map-delivery
workspace/ming-iot/.agents/skills/map-delivery
workspace/ming-iot/.claude/skills/map-delivery
workspace/ming-iot-rd-docs/.claude/skills/map-delivery
```

测试脚本创建相对符号链接、重复运行不改变结果、目标被普通文件占用时失败、目标逃出 workspace 时失败，并把产品仓本地注册加入该仓实际 Git dir 的 `info/exclude`。不得改写受跟踪的产品文件。

- [x] **Step 2：实现 setup CLI**

CLI 固定为：

```bash
python tools/setup_workspace_skills.py \
  --workspace-root .. \
  --product-repo ../ming-iot
```

脚本从自身位置确定 canonical Skill，不接受另一个业务规则来源。成功输出每个目标的 `created` 或 `existing`；拒绝覆盖非符号链接、拒绝指向另一来源的符号链接。Windows 或禁用 symlink 的环境返回明确非零错误，并提示由成员使用宿主官方安装机制指向 canonical Skill，不静默复制规则。

- [x] **Step 3：编写 Skill**

实施时必须使用 `skill-creator`、`superpowers:writing-skills` 和 `writing-for-agents` 检查 Skill。Front Matter description 必须同时覆盖显式与隐式触发：创建或更新模块 Spec、Change、Plan、Patch、Bugfix、Sunset、Review、Evidence，或关联 Gitee Issue/PR 时使用。

`SKILL.md` 固定流程：

```text
定位 workspace 与文档仓
→ 读取根/模块 AGENTS.md
→ 读取 Change Hub、模块 spec 和活动 Change
→ 校验真实 Delivery Issue
→ 只调用 map_docs.py 生成或检查结构
→ 起草语义正文
→ 回写四向导航
→ 运行 map_docs.py check
→ 报告当前权威与候选边界
```

Skill 不包含 Git 回调，不声称自己是门禁，不复制 Python 校验规则。`references/workflow.md` 保存字段表、状态迁移和命令示例，`SKILL.md` 只保留决策顺序。

- [x] **Step 4：验证并提交**

Run:

```bash
python -m unittest -v tests.test_setup_workspace_skills
python tools/map_docs.py check --docs-root .
git diff --check
```

Expected: 临时目录测试、map 检查和 diff 检查通过。候选 worktree 内只验证 setup 逻辑，不把真实工作区链接指向可删除的候选分支。提交：

```bash
git add .agents/skills/map-delivery tools/setup_workspace_skills.py tests/test_setup_workspace_skills.py
git commit -m "feat: add portable map delivery skill"
```

## Task 6：接入文档仓本地与 CI 门禁并交付阶段 A

**Files:**

- Modify: `.githooks/pre-push`
- Create: `.workflow/map-docs-check.yml`
- Modify: `tests/test_repository_policy.py`
- Modify: `map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/{README.md,change.md,plan.md}`

- [x] **Step 1：先扩展仓库策略测试**

增加断言：pre-push 仍拒绝非批准 remote，并从 Git 根调用：

```bash
python tools/map_docs.py check --docs-root .
```

Workflow 必须在 PR 到 `master` 时执行：

```bash
python -m unittest discover -s tests -v
python tools/check_delivery_metadata.py --docs-root . --since 2026-09-03
python tools/map_docs.py check --docs-root .
git diff --check
```

测试还必须断言 `.agents/skills/map-delivery/SKILL.md` 是普通受跟踪文件，而不是指向仓库外的符号链接。

- [x] **Step 2：修改 Hook 和 Gitee Workflow**

`pre-push` 使用 `git rev-parse --show-toplevel` 定位根目录；先保留现有 Gitee remote 白名单，再运行 map 检查。Workflow 参考产品仓 `ming-iot/.workflow/build-test-vet-202608301305.yml` 的 Gitee Go YAML 格式，`strategy.blocking: true`，不读取 Gitee token，不访问兄弟仓库。

- [x] **Step 3：把 Change 推进到 Verifying**

把 `change.md` 状态改为 `verifying`，`plan.md` 保持 `approved`，Hub 记录阶段 A PR、base 和 exact head。按 2026-09-13 修订，模块初版 `spec.md` 先行定义，不必等到阶段 C 才编写；不得因此切换根 README/AGENTS 权威或宣称模块已验收。

- [x] **Step 4：运行阶段 A 验证**

Run:

```bash
python -m unittest discover -s tests -v
python tools/check_delivery_metadata.py --docs-root . --since 2026-09-03
python tools/map_docs.py check --docs-root .
git diff --check
git status --short --branch
```

Expected: 全部通过；状态只包含阶段 A 文件。

- [ ] **Step 5：提交、独立 Review 并创建阶段 A Draft PR**

执行检查点：阶段 A 候选已通过本地 159 项测试及独立 Review，并发布为 [PR !9](https://gitee.com/ming-industry/ming-iot-rd-docs/pulls/9)，保持 Draft。本步骤余下的真实注册、Issue master 反链和阶段 B 进入条件，等待人工确认云端 CI 并合入；因此不提前勾选整个步骤。

先提交阶段 A 的全部候选变更：

```bash
git add .githooks/pre-push .workflow/map-docs-check.yml tests/test_repository_policy.py map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理
git commit -m "ci: gate business-map documents"
git rev-parse HEAD
```

由独立 Reviewer 绑定该完整 exact head，复跑 Task 6 Step 4 并检查 Change Spec 覆盖。如修复改变 Head，重新提交并 Review。验证通过后推送并用 `tools/gitee_pr.py` 创建阶段 A Draft PR，不执行合并。人工合入后记录真实 merge revision，并使用 `gitee_issue.py link` 把 Issue 指向 master 上的 Change Hub。

阶段 A 合入后，从 workspace 根目录运行正式注册，使链接指向文档仓主工作树中已合入的 canonical Skill：

```bash
git -C ming-iot-rd-docs fetch origin master
git -C ming-iot-rd-docs merge --ff-only origin/master
python3 ming-iot-rd-docs/tools/setup_workspace_skills.py \
  --workspace-root . \
  --product-repo ming-iot
```

Expected: 五个注册入口均指向 `ming-iot-rd-docs/.agents/skills/map-delivery`，两个仓库的 `git status --short` 不出现生成的本地链接。

## Task 7：在产品仓晋升 Delivery Hub 契约并交付阶段 B

**Files:**

- Modify: `ming-iot/AGENTS.md`
- Modify: `ming-iot/.gitee/PULL_REQUEST_TEMPLATE.zh-CN.md`
- Modify: `ming-iot/.gitignore`
- Modify: `ming-iot/backend/tools/internal/repositorycontract/verification_profile_test.go`

- [ ] **Step 1：从最新产品仓远端创建独立 worktree**

Run from workspace root:

```bash
git -C ming-iot fetch origin master
git -C ming-iot worktree add .worktrees/ikeofy-map-delivery-product \
  -b docs/ikeofy-map-delivery-product origin/master
```

Expected: 新 worktree clean；不得从当前落后于远端的本地 `master` 建分支。

- [ ] **Step 2：先修改 Go 仓库契约测试并验证 RED**

在现有 `TestVerificationProfile` 的 `pull request template` 子测试中，将必需标题改为：

```go
required := []string{
    "## Delivery Issue",
    "## Delivery Hub",
    "## 精确候选 Head",
    "## 独立 Review",
    "## 风险、兼容性与回滚",
}
```

同时断言 `AGENTS.md` 包含 `ming-iot-rd-docs/map/`、`map-delivery` 和 `tools/setup_workspace_skills.py`，并断言 `.gitignore` 包含 `/.agents/` 与 `/.claude/`。

Run:

```bash
go -C backend test ./tools/internal/repositorycontract -run TestVerificationProfile -count=1
```

Expected: FAIL，指出三个产品仓文件尚未提供新契约。

- [ ] **Step 3：更新产品仓 Agent 入口和忽略规则**

`AGENTS.md` 增加稳定规则：新 Delivery 从 Gitee Issue 进入，并读取文档仓 Change Hub、模块主规格和该 Change Plan；从产品仓启动时运行：

```bash
python3 ../ming-iot-rd-docs/tools/setup_workspace_skills.py \
  --workspace-root .. \
  --product-repo ../ming-iot
```

`.gitignore` 增加：

```gitignore
/.agents/
/.claude/
```

不把 Skill 复制进产品仓，不让产品构建依赖私有文档仓。

- [ ] **Step 4：更新 PR 模板**

把 `## RFC / Spec / Plan` 改为 `## Delivery Hub`，字段固定为：

以下尖括号是最终 PR 模板供填写者替换的字面占位符，不是本 Plan 的未决项：

```markdown
- Change Hub：`map/<业务域>/<功能模块>/changes/<Change>/README.md`
- 模块主 Spec：`map/<业务域>/<功能模块>/spec.md` 或“新模块，随本 Change 激活”
- Change Spec：`map/<业务域>/<功能模块>/changes/<Change>/change.md`
- 实施 Plan：`map/<业务域>/<功能模块>/changes/<Change>/plan.md`
- 研发文档已合并 revision：`<完整 40 位 Git revision>`
```

PR 仍保留 Delivery Issue、Verification Profile、exact head、独立 Review 和回滚字段。模板不声称这些字段已通过校验。

- [ ] **Step 5：验证、提交、Review 并创建产品 Draft PR**

Run:

```bash
go -C backend test ./tools/internal/repositorycontract -run TestVerificationProfile -count=1
make verify-pr
git diff --check
git status --short --branch
```

Expected: 聚焦测试和 `make verify-pr` 通过；变更只有四个阶段 B 文件。验证通过后提交候选：

```bash
git add AGENTS.md .gitee/PULL_REQUEST_TEMPLATE.zh-CN.md .gitignore backend/tools/internal/repositorycontract/verification_profile_test.go
git commit -m "docs: link product deliveries to map hubs"
git rev-parse HEAD
```

由独立 Reviewer 绑定该完整 exact head 复跑本 Step 的验证。如修复改变 Head，重新提交并 Review。通过后推送并创建产品 Draft PR。PR 引用 `ming-iot-rd-docs#IKEOFY`、阶段 A 已合入 revision 和 Change Hub；人工合入后记录产品 merge revision，不执行发布或生产操作。

## Task 8：核验已定义主规格并形成 activation candidate

**Files:**

- Create: `map/00_研发治理/01_文档管理/spec.md`
- Create: `map/00_研发治理/01_文档管理/adr/0001-business-map-delivery-authority.md`
- Modify: `map/README.md`
- Modify: `map/00_研发治理/01_文档管理/README.md`
- Modify: `AGENTS.md`
- Modify: `README.md`
- Modify: `meta/DOC_REGISTRY.yaml`
- Modify: six legacy Spec/Plan files listed in “文件职责”
- Modify: current Change files

- [ ] **Step 1：从阶段 A 合入后的最新 master 创建 activation worktree**

Run:

```bash
git fetch origin master
git worktree add .worktrees/ikeofy-map-delivery-activation \
  -b docs/ikeofy-map-delivery-activation origin/master
stage_a_revision="$(git rev-parse origin/master)"
git show "$stage_a_revision:tools/map_docs.py" >/dev/null
git show "$stage_a_revision:.agents/skills/map-delivery/SKILL.md" >/dev/null
```

Expected: worktree clean，两个 `git show` 返回 0，证明当前 `origin/master` 已包含阶段 A 的校验器和 canonical Skill。把 `stage_a_revision` 的完整 40 位值写入 Hub；不得用旧候选 Head 猜测。

- [ ] **Step 2：核验模块主规格与交付结果**

读取先行定义的模块 `spec.md`，对照交付结果核验完整要求与实现差异；有差异时显式修订、评审，不能由实现结果倒推需求。保留以下稳定锚点：

```text
document-authority
delivery-identity
module-layout
change-lifecycle
four-way-navigation
bidirectional-pointers
review-and-evidence
agent-skill-discovery
deterministic-gates
multi-repository-delivery
migration-and-recovery
```

主规格正文是模块的完整规范，不以本 Change 为上位需求来源。首次落地激活时核对规范与真实交付，为已生效的交付覆盖段落添加追溯指针；指针只表达实现或变更来源，不代表主规格要等 Change 完成才生成。后续增量生效时在原主规格中更新对应段落，不复制旧 Spec 原文。

- [ ] **Step 3：写 ADR 并更新模块索引**

ADR Front Matter 使用 `document_kind: adr`、`status: accepted` 和本模块路径，正文记录选择业务地图、模块主 Spec、per-Delivery Change、单一 `map-delivery` Skill 和确定性门禁的理由与后果。模块 README 把 IKEOFY 从活动 Change 移到历史 Change，并把 `spec.md` 与 ADR 设为当前入口。

- [ ] **Step 4：给旧文档建立迁移指针**

三份旧 Spec 顶部加入明确的 Superseded 指针；带 Front Matter 的两份将 `status` 改为 `superseded`。三份旧 Plan 标为 completed 或历史计划，并链接 IKEOFY Hub。只增加状态和指针，不删除历史正文，不把旧行为重新写成当前规则。

- [ ] **Step 5：切换根入口和 Authority Catalog**

根 `AGENTS.md` 规定：新交付默认进入 `map/`；未迁移模块暂时继续使用旧顶层目录；任何主题只能有一个当前 Authority。根 README 把 `map/` 列为默认入口，把旧目录描述为未迁移或历史材料。

`meta/DOC_REGISTRY.yaml` 至少把以下 topic 改指向模块当前规格或 IKEOFY Change：

```yaml
rd_docs_dual_repository_worktree_governance: map/00_研发治理/01_文档管理/spec.md
gitee_delivery_verification_profile: map/00_研发治理/01_文档管理/spec.md
gitee_delivery_verification_implementation: map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/plan.md
gitee_delivery_workflow: map/00_研发治理/01_文档管理/spec.md
gitee_draft_pr_automation_and_rd_docs_authorization: map/00_研发治理/01_文档管理/spec.md
gitee_draft_pr_automation_and_rd_docs_authorization_implementation: map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/plan.md
```

- [ ] **Step 6：生成 activation candidate 状态**

把 Change `target_anchors` 设为主规格真实锚点，把 `change.md` 状态设为 `effective` 候选，把 `plan.md` 状态设为 `completed` 候选。Hub 记录阶段 A 文档 PR、阶段 B 产品 PR、各自 merge revision，并记录阶段 C Draft PR 状态。

## Task 9：验证迁移唯一权威和跨仓事实

**Files:**

- Modify: `tests/test_map_docs.py`
- Read: `meta/DOC_REGISTRY.yaml`
- Read: `ming-iot` 阶段 B merge revision

- [ ] **Step 1：增加真实仓库迁移契约测试**

新增测试断言六个迁移 topic 不再指向旧顶层 Spec/Plan；旧文档包含新 Authority 指针；模块 spec 包含全部稳定锚点；IKEOFY Hub 中列出的两个已合入 revision 都是完整 40 位小写十六进制值。

- [ ] **Step 2：运行激活前候选验证**

Run:

```bash
python tools/map_docs.py check --docs-root .
python tools/check_delivery_metadata.py --docs-root . --since 2026-09-03
python -m unittest discover -s tests -v
git diff --check
```

Expected: map 检查、旧元数据命令和全部测试通过。此时尚未产生绑定候选 Head 的 Review/Evidence，因此不提前运行 `activate`。旧元数据检查继续负责尚未迁移的顶层文档；map checker 负责新结构，两者都不能跳过解析错误。

- [ ] **Step 3：验证产品仓合入事实**

Run:

```bash
git -C ming-iot fetch origin master
product_revision="$(git -C ming-iot rev-parse origin/master)"
git -C ming-iot show "$product_revision:AGENTS.md" \
  | rg 'ming-iot-rd-docs/map/'
git -C ming-iot show "$product_revision:.gitee/PULL_REQUEST_TEMPLATE.zh-CN.md" \
  | rg '## Delivery Hub'
```

Run from workspace root. Expected: 两个 `rg` 都匹配，证明当前产品 `origin/master` 已提供阶段 B 入口和 PR 契约。把 `product_revision` 的完整 40 位值写入 Hub；失败时 Change 保持 Verifying，不进入 activation PR。

- [ ] **Step 4：运行私有材料边界扫描**

使用当前 LoopX 项目扫描入口只扫描本阶段候选文件。Expected: 不含凭据、原始模型对话、缓存、可再生日志或本机绝对路径；检测到任何一项都停止提交并移除。

## Task 10：形成 Review、Evidence 和阶段 C PR

**Files:**

- Create: `map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/review.md`
- Create: `map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/evidence.md`
- Modify: `map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/README.md`

- [ ] **Step 1：创建阶段 C Draft PR 并固定候选 Head**

提交 Task 8–9 的候选，推送分支并使用 `tools/gitee_pr.py` 创建 Draft PR：

```bash
git add map AGENTS.md README.md meta/DOC_REGISTRY.yaml specs plans tests/test_map_docs.py
git commit -m "docs: activate business-map document authority"
git push -u origin docs/ikeofy-map-delivery-activation
candidate_head="$(git rev-parse HEAD)"
cat > /tmp/ikeofy-stage-c-title.txt <<'EOF'
docs: activate business-map document authority
EOF
cat > /tmp/ikeofy-stage-c-body.txt <<EOF
Delivery Issue: https://gitee.com/ming-industry/ming-iot-rd-docs/issues/IKEOFY
Change Hub: map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/README.md
Change Spec: Approved
Plan: Completed candidate
Exact candidate Head: ${candidate_head}
Validation: Task 9 Steps 1-4 passed
Rollback: revert this PR; the former top-level document authority remains recoverable from Git history.
EOF
python tools/gitee_pr.py \
  --repo-root . \
  --base master \
  --title-file /tmp/ikeofy-stage-c-title.txt \
  --body-file /tmp/ikeofy-stage-c-body.txt \
  --draft
```

从工具的回读输出中取得经验证的 PR URL，写入 Change Hub，再提交并推送导航变更：

```bash
git add map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/README.md
git commit -m "docs: link business-map activation PR"
git push
git rev-parse HEAD
git status --short --branch
```

Expected: 得到完整 40 位 Head，worktree clean，Hub 和 PR 已能互相定位。重新生成 body 文件中的 `Exact candidate Head` 并再运行同一 `gitee_pr.py` 命令，回读确认 PR Head 与当前 HEAD 相同。该 Head 是独立 Review 的目标。

- [ ] **Step 2：执行独立双轴 Review**

Reviewer 同时检查：

1. Standards：目录、元数据、链接、Hook、CI、Skill 和迁移规则是否符合仓库契约；
2. Spec：是否满足 IKEOFY Change Spec 的十项验收标准。

Reviewer 必须复跑 Task 9 的命令并绑定 Task 10 Step 1 的 exact head。任何 blocker 都返回实施者修复；修复改变 Head 后必须重新 Review。

- [ ] **Step 3：写入 Review 和 Evidence**

`review.md` 记录文档仓基础能力候选、产品仓候选、activation 候选三行 `repository/base/exact_head/verdict`；`evidence.md` 记录命令、结论、稳定 Artifact 或 PR 指针，不内联原始日志。Hub 补齐三个 PR 和所有 Head。把三个只读回执文件作为一个提交：

```bash
git add \
  map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/review.md \
  map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/evidence.md \
  map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/README.md
git commit -m "docs: record business-map activation review"
```

完成该提交后，才运行 `activate`。

- [ ] **Step 4：验证最终只读回执增量**

提交 Review/Evidence 后，最终 Head 相对被评审 Head 只允许修改：

```text
review.md
evidence.md
README.md
```

重新运行：

```bash
git diff --name-only HEAD^..HEAD
python tools/map_docs.py check --docs-root .
python tools/map_docs.py activate \
  --docs-root . \
  --change map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理
python -m unittest discover -s tests -v
git diff --check
```

Expected: 全部通过。若最终回执提交触及主规格、工具、规则或迁移指针，必须重新固定 Head 并执行完整独立 Review。

- [ ] **Step 5：更新阶段 C Draft PR 并等待人工合入**

推送 Review/Evidence 回执提交，重新生成 `/tmp/ikeofy-stage-c-body.txt` 中的 `Exact candidate Head` 并再运行 Step 1 的 `gitee_pr.py` 命令，使已有 Draft PR 绑定当前完整 Head，同时在 Review 中保留被评审的候选 Head。PR 正文引用 Issue、Hub、Approved Change、Completed Plan、三个精确候选 Head、实际校验与回滚边界。人工合入后回读 `origin/master`、Issue 托管块和模块主规格；不执行产品发布或生产操作。

## 最终验收矩阵

| Change Spec 要求 | 实施任务 | 证明 |
|---|---|---|
| 业务域/功能模块目录 | Tasks 1–3、8 | `map_docs.py check`、模块 spec |
| Issue/Spec/Plan/PR 四向导航 | Tasks 2、4、7、8 | Hub、Issue 托管块、PR 模板 |
| 主规格与 Change 双向指针 | Tasks 3、8 | `map_docs.py activate` |
| FEATURE/PATCH/BUGFIX/SUNSET | Tasks 1–2 | 类型枚举测试 |
| 多人并行与锚点冲突 | Tasks 1、3 | 活动覆盖冲突测试 |
| Review/Evidence 绑定精确 Head | Tasks 3、10 | 单文件多仓 Head 表 |
| 唯一 `map-delivery` Skill | Task 5 | canonical 文件和 setup 测试 |
| Git Hook 与 CI 硬门禁 | Task 6 | repository policy 测试、阻断 Workflow |
| 多仓非原子恢复 | Tasks 6–10 | 三段 PR 和回读 revision |
| 旧布局迁移且无双重权威 | Tasks 8–9 | Authority Catalog 和旧文档指针测试 |

## 实施完成条件

只有以下条件全部成立，IKEOFY 才能保持 `Effective`：

1. 设计/计划、阶段 A、阶段 B、阶段 C PR 均已由人审阅并合入各自 `master`。
2. Gitee Issue 托管块指向文档仓 master 上真实存在的 Change Hub。
3. `map_docs.py check` 与 `activate`、旧目录元数据校验、文档仓全量单测全部通过。
4. 产品仓 `make verify-pr` 和仓库契约测试通过。
5. 模块 `spec.md` 是已迁移文档管理主题的唯一当前 Authority；旧文档都有明确状态与指针。
6. Skill 从工作区根、产品仓和文档仓启动路径可发现；缺少 Skill 时 Hook/CI 仍能拒绝不合法结构。
7. Review/Evidence/Hub 保存真实 PR、base、exact head 与结论，不包含凭据和原始日志。
