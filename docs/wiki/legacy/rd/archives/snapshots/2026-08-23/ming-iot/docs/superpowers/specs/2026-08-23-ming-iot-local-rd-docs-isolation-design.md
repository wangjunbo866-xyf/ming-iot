# ming-iot 研发演进文档本地隔离设计

> 日期：2026-08-23
>
> 状态：设计已口头通过，等待书面规格复核
>
> 范围：保留 Gitee 既有 `docs/superpowers/` 历史，从现在开始阻止该目录的新变化上传，并将后续研发演进文档迁入无远端的本地 Git 文档库。

## 1. 目标

同时满足四个目标：

1. Gitee `origin/master` 已有研发文档不删除、不改写历史；
2. 当前尚未推送以及未来新增的 `docs/superpowers/` 变化不进入 Gitee；
3. 本地规格、计划、Evidence、Review 和 Proposal 继续受 Git 版本管理，不因删除 worktree 丢失；
4. `ming-iot/docs/` 的其他正式产品文档仍可正常提交和上传。

## 2. 当前事实

设计时核对到：

- `origin/master` 已包含 33 个 `docs/superpowers/` 文件；
- 当前本地 `HEAD` 跟踪 35 个该目录文件；
- 当前 worktree 还有三份未跟踪文档：
  - `docs/superpowers/plans/2026-08-23-loopx-personal-framework-bootstrap.md`
  - `docs/superpowers/specs/2026-08-23-loopx-plan-bilingual-publication-design.md`
  - `docs/superpowers/specs/2026-08-23-ming-iot-local-rd-docs-isolation-design.md`
- `iot-doc` 还有两份未跟踪的个人研发框架规格，也不应误入其 Gitee 远端：
  - `docs/superpowers/specs/2026-08-21-loopx-driven-iot-development-control-plane-design.md`；
  - `docs/superpowers/specs/2026-08-23-ai-native-software-rd-two-stage-blueprint-design.md`；
- `origin/master..HEAD` 有 7 个尚未推送提交；
- 其中有三项尚未上传的研发文档变化：
  - 新增 `docs/superpowers/specs/2026-08-19-thingsbaseline-shared-go-cache-design.md`；
  - 新增 `docs/superpowers/plans/2026-08-19-thingsbaseline-shared-go-cache.md`；
  - 修改 `docs/superpowers/specs/2026-08-14-l0-things-compilation-migration-baseline-design.md`。

因此，单纯增加 `.gitignore` 不能满足目标：它不影响已经跟踪的文件，也不能清除尚未推送提交中的文档对象。

## 3. 目标架构

### 3.1 本地研发文档库

建立独立本地 Git 仓库：

```text
/home/wangjunbo/workspace-iot-platform/ming-iot-rd-docs/
```

目录结构：

```text
ming-iot-rd-docs/
  AGENTS.md
  README.md
  specs/
  plans/
  evidence/
  reviews/
  proposals/
```

该仓库：

- 不配置任何 Git remote；
- 使用独立提交保留演进过程；
- `AGENTS.md` 明确禁止添加远端、push 和自动提交；
- 本地 `pre-push` 无条件拒绝，以防日后误加 remote；
- 不存储凭据、原始 transcript 或大体量临时日志；
- 是今后框架研发演进文档的唯一写入位置。

### 3.2 ming-iot 中的历史快照

`ming-iot/docs/superpowers/` 的 Gitee 既有内容定义为冻结历史：

- `origin/master` 当前已有 33 个文件保持原样；
- 不通过普通提交删除它们；
- 不改写远端历史，不强制 push；
- 不再新增、修改、重命名或删除该目录中的文件；
- 后续 Agent 读取框架演进文档时，优先读取本地 `ming-iot-rd-docs`，旧目录只作为历史参考。

## 4. 本地文档迁移

迁移必须先复制、校验，再考虑清理来源：

1. 从当前本地 `HEAD` 导出完整 `docs/superpowers/`；
2. 合并当前 worktree 中三份尚未跟踪的规格和计划；
3. 导入 `iot-doc` 中两份尚未跟踪的个人研发框架规格，但不导入其工业物联网正式架构正文；
4. 将 `specs/`、`plans/`、`evidence/`、`reviews/` 映射到本地文档库同名目录；
5. 生成逐文件 SHA-256 清单，证明复制前后内容一致；
6. 在用户明确授权后，才在本地文档库创建首次提交；
7. 确认本地文档库无 remote 且 `pre-push` 拒绝后，才进入 `ming-iot` 发布历史净化；
8. 原 worktree 和 `iot-doc` 中的未跟踪文档在迁移验收前不得删除。

双语 Bootstrap 计划在本地文档库发布：

- `plans/2026-08-23-loopx-personal-framework-bootstrap.md`：语言入口；
- `plans/2026-08-23-loopx-personal-framework-bootstrap.zh-CN.md`：中文权威版；
- `plans/2026-08-23-loopx-personal-framework-bootstrap.en.md`：英文翻译版。

## 5. 无强推的安全发布历史

### 5.1 原则

不能直接 push 当前本地分支，因为 7 个未推送提交中包含三项研发文档变化。采用从 `origin/master` 出发的安全发布分支，只重放代码和正式工具说明，不重放 `docs/superpowers/` 变化。

### 5.2 重放边界

先创建仅本地归档分支 `archive/pre-rd-docs-isolation-20260823`，保留原始提交链。安全发布分支 `publish/no-rd-docs-20260823` 从精确 `origin/master` 创建，然后按顺序处理：

| 原提交 | 处理 |
|---|---|
| `edf39c1 docs(tools): design shared Go cache` | 不重放；文档已进入本地文档库 |
| `e1cd179 docs(tools): plan shared Go cache implementation` | 不重放；文档已进入本地文档库 |
| `0bf3d25 feat(tools): resolve shared Go cache paths` | 重放 |
| `8982a3c feat(tools): reuse trusted Go caches` | 重放 |
| `dc19c8a feat(tools): reuse Go caches in L0 gates` | 重放 |
| `8a9e793 docs(tools): explain shared Go cache semantics` | 只重放 `backend/tools/README.md`，排除 `docs/superpowers/` 修改 |
| `403b9a6 fix(tools): pin render commands to attested Go tool` | 重放 |

重放后必须满足：

```text
git diff --exit-code origin/master..publish/no-rd-docs-20260823 -- docs/superpowers/
```

该命令无输出且返回 `0`，才能证明下一次 push 不会改变 Gitee 当前 `docs/superpowers/` 树。

安全发布分支相对 `origin/master` 保持 fast-forward，不需要 `--force`。旧本地提交链只保留为本地归档，不上传。

## 6. 防误上传机制

采用三层保护：

1. `.gitignore` 添加 `/docs/superpowers/`，阻止未来未跟踪文件被普通 `git add` 收集；
2. `AGENTS.md` 声明该目录为 Gitee 冻结历史，新的演进文档必须写入本地文档库；
3. hooks/确定性检查同时覆盖：
   - pre-commit 拒绝 staged 的 `docs/superpowers/` 路径；
   - pre-push 检查待推送 revision range，发现该目录任何增删改即拒绝。

`.gitignore` 不是跟踪文件保护机制；真正防止已有文件变化上传的是 staged-diff 和 push-range 检查。

门禁不得阻止 `ming-iot/docs/` 其他目录，也不得扫描、上传或复制本地文档库正文。

## 7. 权威引用调整

Bootstrap 和后续 LoopX 配置需要区分两种文档：

- 工业物联网架构语义仍由 `iot-doc` 的正式规范拥有；
- 个人研发框架的规格、计划和过程 Evidence 由本地 `ming-iot-rd-docs` 拥有。

任何版本化 `ming-iot` 配置不得硬编码用户 Home 绝对路径。通过被忽略的本地配置记录文档库位置，并在缺失时 fail closed，提示操作者连接本地文档库。公开仓库只保存配置格式和示例，不保存真实本地路径或私有文档摘要。

## 8. 失败与回退

- 本地文档复制或 digest 校验失败：保留来源 worktree，停止后续步骤；
- 本地文档库意外存在 remote：停止并移除 remote，未经用户批准不得继续；
- cherry-pick 冲突：保留安全发布 worktree，逐项核对，不删除原始分支；
- 安全分支出现 `docs/superpowers/` diff：拒绝发布并重建安全分支；
- hook 误伤正式文档：修正路径匹配，不能通过绕过 hook 继续 push；
- 远端 push 结果未知：先读取 Gitee remote revision，禁止直接重复 push；
- 任意回退均不得删除本地文档库或原始归档引用。

## 9. 非目标

- 不删除 Gitee 已有 33 个研发文档；
- 不执行远端历史重写或强制 push；
- 不把本地文档库改造成团队知识平台；
- 不自动同步本地文档到 `iot-doc`、Gitee 或其他远端；
- 不在本规格中执行 LoopX Bootstrap 或 IoT 业务实现；
- 不以 `skip-worktree` 或 `assume-unchanged` 作为长期安全机制。

## 10. 验收

全部满足才算隔离完成：

1. 本地文档库存在、Git 状态可读且没有 remote；
2. 当前完整文档、ming-iot worktree 的三份未跟踪文档和 `iot-doc` 的两份未跟踪框架规格均已复制并通过 digest 校验；
3. 本地 `pre-push` 可以确定性拒绝任何 push；
4. 原始 `ming-iot` 本地提交链有未上传的归档引用；
5. 安全发布分支从 `origin/master` fast-forward，并保留全部非研发文档代码变化；
6. `origin/master..publish/no-rd-docs-20260823` 对 `docs/superpowers/` 无任何 diff；
7. `.gitignore`、AGENTS、pre-commit 和 pre-push 共同阻止后续误上传；
8. Gitee 既有历史未改写，没有使用 force push；
9. 双语 Bootstrap 文档只存在于本地文档库；
10. 未经用户新的明确授权，没有执行 commit、merge 或 push。
