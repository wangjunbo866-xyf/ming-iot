# ming-iot 双仓库管理与研发文档 Worktree 切换设计

## 1. 背景

`ming-iot` 保存可交付代码，`ming-iot-rd-docs` 保存规格、计划、Proposal、Evidence、Review、Authority Catalog 与历史归档。后者已经连接私有 Gitee 仓库 `https://gitee.com/ming-industry/ming-iot-rd-docs.git`，具备稳定的 Git repository identity。

当前研发文档 linked worktree 位于工作区级目录 `.rd-docs-worktrees/`。两个旧 worktree 均落后文档 `master` 两个提交、没有独立提交，但仍包含未迁入 `master` 的文件。直接删除会丢失材料，直接从旧分支提交则会把落后的工作树与重复文件混入新历史。

## 2. 已批准决策

1. `ming-iot` 与 `ming-iot-rd-docs` 采用正式双仓库管理。
2. 同一业务 Goal 可以跨两个仓库；写代码的 Todo 归属 `ming-iot`，写正式研发文档的 Todo 归属 `ming-iot-rd-docs`。
3. `ming-iot-rd-docs` 的本地 linked worktree 统一放入仓库内的 `.worktrees/`，不再使用工作区级 `.rd-docs-worktrees/`。
4. `.worktrees/` 是纯本地执行目录，必须加入 `ming-iot-rd-docs/.gitignore`，不得提交到 Gitee。
5. 旧 `.rd-docs-worktrees/` 只有在全部独有内容迁入、校验通过并完成逐文件等价复核后才允许删除。

## 3. 仓库职责

### 3.1 ming-iot

- 保存平台源代码、部署配置、构建工具和机器契约。
- LoopX 写入型 Todo 使用 `task_repository=git:gitee.com/ming-industry/ming-iot`。
- 独立 Reviewer 对候选代码只读，不得以 Review Todo 修改实现。

### 3.2 ming-iot-rd-docs

- 保存 `specs/`、`plans/`、`proposals/`、`evidence/`、`reviews/`、`references/`、`meta/` 与 `archives/`。
- LoopX 文档写入型 Todo 使用 `task_repository=git:gitee.com/ming-industry/ming-iot-rd-docs`。
- Todo 必须声明精确或有界的 `required_write_scopes`；Review 默认只允许一个精确 `reviews/*.md` 文件以及显式声明的验证产物。
- `.loopx/`、`.codex/` 和 `.worktrees/` 均为本地运行状态，不进入正式文档提交。

## 4. 目标目录

```text
workspace-iot-platform/
├── ming-iot/
└── ming-iot-rd-docs/
    ├── .worktrees/                 # 忽略的本地 linked worktree 池
    │   └── <loop-or-todo-name>/
    ├── specs/
    ├── plans/
    ├── proposals/
    ├── evidence/
    ├── reviews/
    ├── references/
    ├── meta/
    └── archives/
```

新 worktree 必须从最新的本地 `master` 创建；在需要远端新鲜度保证时，先显式 fetch 并验证 `master` 与 `origin/master` 的关系。worktree 名称使用稳定、可读的 Loop 或 Todo 语义名称，不使用模型会话 id。

## 5. 旧 Worktree 无损收口

### 5.1 已识别的独有内容

`kafka-foundation-activation`：

- `plans/2026-08-28-kafka-foundation-contract-sdk.md`
- `specs/2026-08-28-kafka-foundation-contract-sdk-activation-design.md`

`local-share-migration-activation`：

- `evidence/2026-08-27-ming-iot-local-share-migration-activation/source-freeze.json`
- `specs/2026-08-27-ming-iot-local-share-migration-activation-design.md`
- `meta/DOC_REGISTRY.yaml` 与当前 `master` 内容不同，必须按条目语义合并，禁止整文件覆盖。

### 5.2 迁移方法

1. 在 `ming-iot-rd-docs/master` 工作区确认起点 clean，且没有其他并发写入。
2. 将四个缺失文件按原始字节内容迁入对应目录。
3. 比较旧 worktree 与当前 `meta/DOC_REGISTRY.yaml`，只补充仍有效且正文真实存在的 Authority 条目；保留 `master` 已有更新。
4. 将 `/.worktrees/` 加入 `.gitignore`。
5. 运行 YAML/目录登记、私有材料扫描、格式检查及 `git diff --check`。
6. 输出逐文件迁移清单和内容摘要；未经用户明确授权，不执行 `git add`、`git commit` 或 `git push`。
7. 对两个旧 worktree 使用全量未跟踪文件模式重新比较。只有所有独有文件都能在 `master` 找到内容相同或经批准合并后的对应物，才允许删除。
8. 使用 `git worktree remove` 删除明确的两个旧 worktree，删除失去用途的本地分支，随后执行 `git worktree prune`。
9. 删除空的工作区级 `.rd-docs-worktrees/` 目录。

## 6. LoopX 路由规则

同一个 `ming-iot-framework-v1` Goal 可以产生跨仓库 Todo，但每个写入型 Todo 只能归属一个仓库：

```text
代码实现 Todo
task_repository = git:gitee.com/ming-industry/ming-iot
required_write_scopes = backend/...

文档或 Review Todo
task_repository = git:gitee.com/ming-industry/ming-iot-rd-docs
required_write_scopes = specs/<exact>.md | plans/<exact>.md | reviews/<exact>.md
```

不得手工复制两个仓库的 `.loopx/registry.json` 或 `.codex` 状态。项目连接、Agent 身份和共享 Goal 投影通过 LoopX 官方命令建立并同步到共享本地控制面。

## 7. 安全与失败处理

- Gitee 仓库必须保持私有；任何改为公开的操作都需要重新执行全仓私有信息审计和独立用户批准。
- 迁移过程中不删除来源 worktree，不改写旧分支历史。
- `DOC_REGISTRY.yaml` 出现条目冲突时停止自动合并，保留双方内容并报告具体 key。
- 任一校验失败时保留旧 worktree，记录失败原因，不执行强制删除。
- worktree 删除不等于分支删除；两者分别解析和验证。
- commit、push、merge、删除远端分支仍分别遵循仓库 `AGENTS.md` 的用户授权要求。

## 8. 验收标准

1. 四个缺失文件已进入 `ming-iot-rd-docs` 主工作区且内容摘要与来源一致。
2. `DOC_REGISTRY.yaml` 保留 master 现有条目，并纳入仍有效的旧 worktree 独有条目。
3. `.gitignore` 明确包含 `/.worktrees/`。
4. 文档校验、Authority Catalog 测试和 `git diff --check` 全部通过。
5. 删除前的全量对比证明旧 worktree 没有未迁移内容。
6. `git worktree list` 不再出现两个旧 worktree，工作区级 `.rd-docs-worktrees/` 不再存在。
7. 后续新建的研发文档 worktree 位于 `ming-iot-rd-docs/.worktrees/`，且该目录不会出现在 Git 变更中。
8. LoopX 文档写入 Todo 可以使用稳定仓库身份 `git:gitee.com/ming-industry/ming-iot-rd-docs` 进行路由。
