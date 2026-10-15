---
document_kind: plan
status: approved
delivery_issue: ming-iot#IKD4OR
code_repository: git:gitee.com/ming-industry/ming-iot
related_documents:
  - specs/2026-09-04-gitee-draft-pr-automation-and-rd-docs-authorization-alignment-design.md
---

# Gitee Draft PR 自动化与研发文档仓授权对齐实施计划

> **执行说明：** 实施者按任务顺序执行，行为变更采用测试先行。Plan Task 是当前仓库级 Agent Todo 的内部工程检查点，不逐项创建 LoopX Todo 或 User Gate。

**目标：** 提供一个安全、幂等、可由 Codex 与 Claude Code 共用的 Gitee Draft PR 命令，并让 `ming-iot-rd-docs` 的候选交付授权边界与 `ming-iot` 一致。

**架构：** `tools/gitee_pr.py` 分为 Git 候选解析、Gitee HTTP 客户端和 CLI 编排三个责任区，全部使用 Python 标准库。单元测试通过临时 Git 仓库和进程内 HTTP 服务验证真实请求与脱敏行为；`AGENTS.md` 只保存稳定授权边界，不复制工具实现。

**技术栈：** Python 3 标准库、Git、Gitee API v5、pytest/unittest。

## Task 1：锁定 Git 候选与远端边界

**文件：**

- 新建：`tests/test_gitee_pr.py`
- 新建：`tools/gitee_pr.py`

- [ ] **Step 1：写 Git 候选失败测试**

测试导入 `tools.gitee_pr`，在临时 bare remote 与工作仓中覆盖：非 Git 目录、dirty worktree、当前分支是 `master`、非 Gitee origin、远端分支缺失、本地与远端 Head 不一致。稳定异常类型为 `PRToolError`，错误只包含公开安全原因。

```python
with self.assertRaisesRegex(gitee_pr.PRToolError, "worktree is dirty"):
    gitee_pr.resolve_candidate(repo, "master")
```

- [ ] **Step 2：运行聚焦测试并确认 RED**

```bash
python3 -m pytest -q tests/test_gitee_pr.py -k candidate
```

预期：因 `tools.gitee_pr` 尚不存在而失败。

- [ ] **Step 3：实现最小 Git 候选模型**

新增不可变模型：

```python
@dataclass(frozen=True)
class Candidate:
    repo_root: Path
    owner: str
    repository: str
    head_branch: str
    base_branch: str
    exact_head: str
```

`resolve_candidate(repo_root, base)` 只调用 argv 形式的 `git -C`，解析 HTTPS/SSH 两种 Gitee remote，使用由当前分支名构造的 `git ls-remote --heads origin refs/heads/feature-name` 校验远端 exact head。所有 subprocess 输出有界，异常不包含凭据 URL。

- [ ] **Step 4：运行聚焦测试并确认 GREEN**

```bash
python3 -m pytest -q tests/test_gitee_pr.py -k candidate
```

预期：候选解析测试全部通过。

## Task 2：实现幂等 Draft PR 创建与更新

**文件：**

- 修改：`tests/test_gitee_pr.py`
- 修改：`tools/gitee_pr.py`

- [ ] **Step 1：写 HTTP 行为失败测试**

进程内 HTTP 服务记录 method、path、JSON/form body 和 Authorization 是否存在，但不保存 token 原文。覆盖：

```python
created = client.ensure_draft(candidate, title="title", body="body")
self.assertEqual(created.number, 3)
self.assertEqual(server.methods, ["GET", "POST", "GET"])
```

同时断言唯一现有 PR 使用 `PATCH`、多个匹配失败、401 脱敏、回读 branch/draft/head 不一致失败、stdout/stderr 不含 token 与正文。

- [ ] **Step 2：运行 HTTP 聚焦测试并确认 RED**

```bash
python3 -m pytest -q tests/test_gitee_pr.py -k 'api or cli or redaction'
```

预期：缺少 API 客户端和 CLI 而失败。

- [ ] **Step 3：实现 Gitee 客户端与 CLI**

客户端只接受显式 token 和可覆盖的 `api_base`：

```python
class GiteeClient:
    def __init__(self, token: str, api_base: str = "https://gitee.com/api/v5") -> None: ...
    def ensure_draft(self, candidate: Candidate, title: str, body: str) -> PullRequest: ...
```

请求通过 `Authorization: Bearer` 请求头传递内存中的令牌；查询现有 open PR 后只允许 0 或 1 个 `head + base` 精确匹配。创建使用 `POST /repos/{owner}/{repo}/pulls`，更新使用 `PATCH /repos/{owner}/{repo}/pulls/{number}`，随后 `GET` 单个 PR 并校验 `draft=true`、分支和 Head。

CLI 参数固定为：

```text
--repo-root PATH --base NAME --title-file PATH --body-file PATH --draft
--api-base URL（仅本地伪服务测试使用；正式令牌只允许发送到 gitee.com）
```

正式 Gitee 令牌只从 `GITEE_ACCESS_TOKEN` 读取且只发送到 `https://gitee.com`；本地伪服务测试使用独立的 `GITEE_PR_TEST_TOKEN`。localhost 模式检测到正式令牌或缺少测试令牌时，在任何网络调用前失败。成功输出一个不含正文的 JSON 对象：

```json
{"number":3,"url":"https://gitee.com/owner/repo/pulls/3","draft":true,"exact_head":"0123456789abcdef0123456789abcdef01234567"}
```

- [ ] **Step 4：运行全部工具测试并确认 GREEN**

```bash
python3 -m pytest -q tests/test_gitee_pr.py
```

预期：全部通过，测试进程环境中使用的 marker token 不出现在捕获输出。

## Task 3：统一授权规则并完成验证

**文件：**

- 修改：`AGENTS.md`
- 修改：`tests/test_repository_policy.py`
- 修改：`meta/DOC_REGISTRY.yaml`

- [ ] **Step 1：先写授权策略失败测试**

在 `tests/test_repository_policy.py` 断言：

```python
self.assertNotIn("未经用户明确授权，不得执行 `git add` 或 `git commit`", agents)
self.assertIn("自行 commit、推送候选分支并创建或更新 Draft PR", agents)
self.assertIn("禁止直接 push `master`", agents)
self.assertIn("凭据创建、读取、轮换和权限扩大", agents)
```

同时断言 `AGENTS.md` 指向 `tools/gitee_pr.py`，且只把 merge、发布、生产、凭据/权限和共享历史重写保留为精确 Gate。

- [ ] **Step 2：运行策略测试并确认 RED**

```bash
python3 -m pytest -q tests/test_repository_policy.py
```

预期：旧逐 commit 授权文字仍存在，测试失败。

- [ ] **Step 3：修改 `AGENTS.md`**

删除旧“提交”两条，新增“Git、Gitee PR 与高风险授权边界”，写入规格 2.3 节的完整稳定规则。常规 Draft PR 发布仍要求 Todo、仓库、scope、worktree、验证和边界扫描全部满足；令牌本身的创建、读取、轮换和扩权继续由用户控制。

- [ ] **Step 4：登记实施计划入口**

在 `meta/DOC_REGISTRY.yaml` 增加：

```yaml
gitee_draft_pr_automation_and_rd_docs_authorization_implementation: plans/2026-09-04-gitee-draft-pr-automation-and-rd-docs-authorization-alignment.md
```

- [ ] **Step 5：运行最终验证**

```bash
python3 -m pytest -q tests/test_gitee_pr.py tests/test_repository_policy.py
python3 -m pytest -q
python3 tools/check_delivery_metadata.py --docs-root . --since 2026-09-03
git diff --check
```

预期：全部通过。另运行归档校验；若 linked worktree 仍因既有 `0664/0644` 模式差异失败，如实记录并引用独立 portability 修复，不改写历史归档清单。

- [ ] **Step 6：形成候选提交和脱敏 Evidence**

只暂存 `AGENTS.md`、规格、计划、`meta/DOC_REGISTRY.yaml`、`tools/gitee_pr.py`、`tests/test_gitee_pr.py`、`tests/test_repository_policy.py`。提交后重跑聚焦测试与 `git diff --check`，记录 exact head、测试计数和既有归档限制；不 push、不创建真实 PR、不 merge。

真实 Gitee 验收等待用户撤销聊天中暴露的令牌并通过安全环境注入轮换后的 `GITEE_ACCESS_TOKEN`，不阻塞本地实现与伪服务测试。
