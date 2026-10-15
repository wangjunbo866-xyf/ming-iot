---
document_kind: spec
status: approved
delivery_issue: ming-iot#IKD4OR
code_repository: git:gitee.com/ming-industry/ming-iot
related_documents:
  - specs/2026-09-03-ming-iot-gitee-delivery-and-verification-profile-design.md
---

# Gitee Draft PR 自动化与研发文档仓授权对齐设计

## 1. 目标

本改造解决两个已经确认的流程摩擦：

1. 候选分支推送后仍需人工打开 Gitee、粘贴正文并创建 Draft PR；
2. `ming-iot-rd-docs` 仍要求每次 `git add`、`git commit` 单独授权，与 `ming-iot` 的结果级 Agent Todo 授权不一致。

本设计不修改 LoopX，不自动合并 PR，也不降低发布、生产、凭据和权限操作的 User Gate。

## 2. 方案

### 2.1 项目级 Gitee PR 命令

在私有研发文档仓新增 `tools/gitee_pr.py`。核心开发环境已经要求同时具备 `ming-iot-rd-docs` 与 `ming-iot`，因此该命令以 `--repo-root` 操作任一仓库，不在两个仓库复制实现。

命令最小接口为：

```bash
python3 tools/gitee_pr.py \
  --repo-root <repository> \
  --base master \
  --title-file <title-file> \
  --body-file <body-file> \
  --draft
```

命令必须：

1. 从 `origin` 解析且只接受 `gitee.com/<owner>/<repo>`；
2. 拒绝从 `master` 创建 PR，拒绝 dirty worktree；
3. 获取当前分支和完整 40 位 Head，并验证 `origin/<branch>` 指向同一 Head；
4. 按 `head + base` 查询现有 open PR；不存在时创建，存在唯一匹配时更新标题、正文和 Draft 状态；匹配不唯一时失败；
5. 创建和更新均通过 Gitee API v5，并使用 `Authorization: Bearer` 请求头；
6. 成功后回读 PR，验证源分支、目标分支、Draft 状态和 exact head，最后只输出 PR 编号、URL、状态和 Head；
7. 不实现 merge、close、删除分支、发布或生产操作；
8. 不输出 token、请求头、完整 API 错误响应或私有 PR 正文。

正式 Gitee 调用只读取环境变量 `GITEE_ACCESS_TOKEN`，并只允许发送到 `https://gitee.com`。本地伪服务测试改读独立的 `GITEE_PR_TEST_TOKEN`；localhost 模式检测到 `GITEE_ACCESS_TOKEN` 时必须在联网前失败。两类令牌均不得写入仓库、命令参数、PR 正文、Evidence 或日志。任何曾出现在聊天、终端历史或可持久日志中的令牌都视为已泄露，必须撤销并轮换。

### 2.2 幂等与失败边界

工具对同一 `owner/repo/head/base` 幂等：重复执行更新现有 PR，不创建第二个 PR。以下情况 fail-closed：

- 缺少令牌或 API 返回认证失败；
- `origin` 不是批准的 Gitee 仓库；
- 本地与远端候选 Head 不一致；
- 源分支或目标分支不明确；
- 找到多个候选 PR；
- 创建或更新后的回读与请求不一致。

失败不回退到人工拼接 URL，也不把未验证的 PR URL 写入 LoopX Evidence。Agent 应记录简短 blocker，由用户处理凭据或 Gitee 服务问题。

### 2.3 研发文档仓授权策略

`ming-iot-rd-docs/AGENTS.md` 改为与 `ming-iot/AGENTS.md` 相同的分层授权：

- 所有跟踪文件修改必须绑定获批 Agent Todo、精确 `task_repository`、有界 write scope 和独立 worktree；
- 验证、归档校验和私有边界扫描通过后，Agent 可自主执行 `git add`、commit、推送候选分支以及创建或更新 Draft PR；
- 所有变更通过 PR 进入 `master`，禁止直接 push `master`；
- 人在 Gitee 执行合并即构成该次合并授权；只有要求 Agent 合并时才建立绑定精确 PR、Head、目标分支和合并方式的 User Gate；
- 版本发布、生产变更、凭据创建/读取/轮换、权限变更和共享历史重写继续要求精确 User Gate；
- 使用已由用户预先配置的受限令牌创建或更新 Draft PR，属于已批准 Todo 内的常规候选发布；改变令牌本身或扩大权限不属于常规发布。

该规则只授权批准范围内的候选交付，不授权 Agent 自行扩大 Todo、读取无关私有资料或合并 PR。

## 3. 测试与验收

新增 `tests/test_gitee_pr.py`，使用本地临时 Git 仓库和伪 HTTP 服务覆盖：

- remote、分支、dirty worktree 与 exact-head 校验；
- 无匹配 PR 时创建 Draft PR；
- 唯一匹配 PR 时更新而不重复创建；
- 多匹配、认证失败、回读不一致时失败；
- 输出与异常中不出现 token、Authorization 请求头或 PR 正文；
- 工具不存在 merge/close/delete-branch 操作入口。

最终验证：

```bash
python3 -m pytest -q tests/test_gitee_pr.py tests/test_repository_policy.py
python3 -m pytest -q
python3 tools/check_delivery_metadata.py --docs-root . --since 2026-09-03
git diff --check
```

真实 Gitee 验收使用一个已推送的非 `master` 测试分支和轮换后的令牌：首次调用创建 Draft PR，第二次调用更新同一 PR，回读 URL 与 exact head 一致。真实调用只创建/更新 Draft PR，不合并。

## 4. 非目标

- 不安装或强制依赖 `mcp-gitee`；
- 不使用 Gitee 评审模式向 `master` 发起 push；
- 不把 PR 创建逻辑加入 LoopX；
- 不自动处理 Review 结论、CI 失败或合并动作；
- 不持久化任何 Gitee 令牌。
