# ming-iot 本地研发文档归档、旧 Graph 退役与 Gitee 历史净化设计

> 日期：2026-08-23
>
> 状态：设计已批准；Phase A 与 Gate B 已完成；Phase B 按 `2026-08-23-loopx-phase-b-bootstrap-cutover-design.md` 推进
>
> 替代：`2026-08-23-ming-iot-local-rd-docs-isolation-design.md`

## 1. 决策摘要

`ming-iot` 不再把研发过程文档和自研 Graph 动态控制面作为 Gitee 仓库的长期组成部分。

目标状态如下：

- LoopX 负责动态 Goal、Todo、Agent 协作、长任务恢复和运行时 Evidence；
- 轻量静态 Harness 与业务校验器继续负责可重复的确定性门禁；
- 本地无远端 Git 仓库 `ming-iot-rd-docs` 保存规格、计划、Evidence、Review、Proposal 和旧 Graph 归档；
- Gitee `ming-iot` 只保留当前有效的产品代码、稳定契约和确定性校验器；
- 在 LoopX 切换并验收后，从 Gitee 所有公开可达历史中清除 `docs/superpowers/**` 和 `.graph/**`；
- 旧 graphctl 控制面从当前代码树整体退役，不能留下仍会被误认为有效的半套实现。

不能立即执行历史净化。必须先完成本地归档和 LoopX 切换，否则当前 `AGENTS.md`、Hook、Graph Registry、Evidence/Review 路径和 Bootstrap 计划会被破坏。

## 2. 设计动机

`docs/superpowers/` 记录了多轮设计推演，其中部分结论已经被后续方案替代。继续将这些材料留在 Gitee，容易使后来者或 Agent 把演进过程误当成当前规范。

`.graph/` 是为早期单机研发构建的自研动态控制面。LoopX 接管 Goal、Todo、调度、恢复和多 Agent 协作后，继续同时保留两套动态控制面会产生双真源、重复门禁和操作混乱。

本设计不删除确定性工程能力。`thingsbaseline`、单元测试、Race、Vet、静态检查及未来 IoT 业务校验器仍然保留，它们是 Harness 的执行能力，不属于需要退役的 Graph 编排层。

## 3. 已核对事实

设计时只读核对到：

- Gitee 远端为 `https://gitee.com/ming-industry/ming-iot.git`；
- `origin/master` 当前为 `7bd2e7f630cb09f054f105f2e21032edb51ee964`，包含 33 个 `docs/superpowers` 文件；
- `origin/core-migration-foundation` 当前为 `7d26ed1617e6bdf6d378716310f4dfc901ee297c`，包含 9 个 `docs/superpowers` 文件；
- 当前公开远端没有 Tag；
- 当前本地分支还有 7 个尚未推送提交，其中包含代码变化和研发文档变化；
- `ming-iot` 当前有两个 worktree；主工作区原有的一份未跟踪 Dashboard 规格已按摘要留痕后明确废弃，Bootstrap worktree 仍有三份待迁移规格/计划；
- `iot-doc` 有两份未跟踪的个人研发框架规格；
- `.graph/loops/*.yaml` 仍把规格、Evidence 和 Review 指向 `docs/superpowers/`；
- `graphctl` 的 Evidence/Review 逻辑仍校验 `docs/superpowers/{specs,evidence,reviews}`；
- 当前 LoopX Bootstrap 计划仍要求在 `docs/superpowers/` 生成 Evidence、Review 和 Proposal；
- Registry 将新的隔离规格判定为 `UNOWNED_PATH`，说明该目录已经不适合作为新研发工作的事实源。

实施前必须再次读取远端 refs。上面的 SHA 只是设计时快照，不能直接作为未来强推参数。

## 4. 目标架构与权威边界

| 内容 | 唯一事实源 | 是否上传 Gitee |
|---|---|---|
| IoT 产品代码 | `ming-iot` Git | 是 |
| IoT 架构规范 | `iot-doc` 中正式架构正文 | 是 |
| 动态 Goal/Todo/Agent 状态 | LoopX 本地运行时 | 否 |
| 研发规格、计划、Evidence、Review、Proposal | `ming-iot-rd-docs` | 否 |
| 确定性测试与业务校验器 | `ming-iot` | 是 |
| 旧 Graph、旧研发过程与切换前完整仓库 | `ming-iot-rd-docs/archives` | 否 |

任何提交到 `ming-iot` 的配置不得硬编码个人 Home 目录。开发机通过被忽略的本地配置找到 `ming-iot-rd-docs`；缺失时应明确提示，而不是退回读取 Gitee 中的旧材料。

## 5. 本地研发文档库

目标路径：

```text
/home/wangjunbo/workspace-iot-platform/ming-iot-rd-docs/
```

目标结构：

```text
ming-iot-rd-docs/
  AGENTS.md
  README.md
  specs/
  plans/
  evidence/
  reviews/
  proposals/
  archives/
    manifests/
    legacy-graph/
    repository-bundles/
```

安全约束：

- 是独立 Git 仓库；
- 不配置任何 remote；
- 本地 `pre-push` 无条件拒绝；
- `AGENTS.md` 禁止添加远端、push、凭据和原始对话 transcript；
- 只允许保存公开安全或本机安全的研发材料；
- 首次提交以及后续提交仍需用户明确授权；
- 删除任何来源之前必须先完成内容清单和 SHA-256 校验。

归档必须同时保留两种形态：

1. 可阅读的规格、计划、Evidence、Review、Proposal 和旧 Graph 快照；
2. 切换前 `ming-iot` 的完整 Git Bundle，用于灾难恢复和历史取证。

Git Bundle 只用于本地恢复，不作为日常阅读入口。旧文档首页必须标明“历史材料，不是当前规范”。

## 6. 迁移范围

必须迁移并校验：

- `ming-iot` 当前可见的完整 `docs/superpowers/`；
- `ming-iot` 尚未推送提交中的研发文档版本；
- 当前 Bootstrap worktree 的三份未跟踪规格/计划；
- `iot-doc` 中两份未跟踪的个人研发框架规格；
- `.graph/registry.yaml`、Loop manifests、Schema 和必要的历史运行说明；
- G0/L0 的 canonical Evidence、Review、completion 关联信息；
- 切换前完整 `ming-iot` Git Bundle；
- 来源 ref、commit、文件大小和 SHA-256 组成的迁移清单。

不迁移：

- 凭据、Token、代理认证信息；
- `.graph/runtime/`、Go cache、构建产物和可再生临时日志；
- Codex/Agent 原始完整 transcript；
- 与本次控制面演进无关的工业物联网正式架构正文。
- 已在 `archives/manifests/discarded-sources.json` 留痕并标记为 `DISCARDED_SUPERSEDED` 的 Dashboard 设计正文。

### 6.1 尚未推送代码的保护

历史净化不能吞掉当前 7 个尚未推送提交中的有效代码变化。实施时必须：

- 在归档清单中记录每个本地 commit、修改路径和处理结论；
- 在完整 Git Bundle 中保存原始本地分支和 worktree 基线；
- 将代码变化分为“继续发布”“被 LoopX 切换替代”“随旧 Graph 退役”三类；
- 在净化后的候选分支中重放所有“继续发布”的代码变化，排除研发文档变化；
- 对被替代或退役的变化记录明确理由，不能静默丢弃；
- 对比原候选树与净化候选树，证明除批准删除/替代的路径外，产品代码语义没有意外变化；
- 在 Gitee 新鲜 Clone 验收完成前，不删除原本地分支、worktree 或 Bundle。

旧 commit SHA 与 rewritten commit SHA 的映射必须进入迁移清单，后续 Review、问题追踪和回滚不得依赖人工记忆。

## 7. LoopX Bootstrap 必须同步修订

现有 Bootstrap 计划与本设计冲突，实施前必须修订：

- 不再把 G0/L0 Evidence/Review 作为 `ming-iot` 中的永久文件；
- Bootstrap Evidence、Review、Proposal 改写到 `ming-iot-rd-docs`；
- 不再创建新的 `docs/superpowers/**` 文件；
- 不再声明 `.graph` 是需要长期留在 Gitee 的冻结动态控制面；
- LoopX 是唯一动态控制面；
- 静态 Harness 不读取 LoopX 私有运行时，也不读取本地研发文档正文；
- `AGENTS.md` 只保留短而稳定的执行入口，不复制动态任务和长篇设计；
- Worker 与 Reviewer 的输出通过逻辑 artifact ID 交接，开发机再映射到本地文档库，公开配置中不出现个人绝对路径。

Bootstrap 验收至少证明：

- 固定版本的 LoopX 可安装并通过健康检查；
- Planner、Worker、Reviewer 使用可区分身份和独立上下文；
- 进程/Chat 重启后可从 LoopX 状态恢复；
- Worker 不能代替 Reviewer；
- 合并、push 和敏感操作仍由用户批准；
- 新任务不再写 `.graph` 或 `docs/superpowers`；
- 快速静态门禁和业务校验器仍可独立运行。

## 8. 旧 Graph 的整体退役边界

不得只删除 `.graph/` 而留下仍被当作有效控制面的其他入口。退役单元包括：

- `.graph/**`；
- `backend/tools/cmd/graphctl/**`；
- `backend/tools/internal/graphctl/**`；
- `backend/tools/README.md` 中 graphctl 生命周期说明；
- `AGENTS.md` 中 graphctl validate/context/ci 强制流程；
- `.githooks/pre-push` 中 graphctl 调用；
- 指向 `.graph/runtime/` 的 Dashboard/systemd 服务；
- 当前代码树中把 Graph 描述为动态权威的其他引用。

以下能力不随 Graph 退役：

- `thingsbaseline`；
- Go 单元测试、Race、Vet；
- API/Schema/生成物一致性校验；
- LoopX Harness 调用的确定性命令；
- 未来面向 IoT 各交付 Loop 的专用校验器。

graphctl 源码从当前树删除即可，不要求从全部 Gitee 历史抹除；完整源码和运行语义由切换前 Git Bundle 保存。`.graph/**` 与 `docs/superpowers/**` 则纳入本次远端可达历史净化范围，避免旧机器契约和旧文档继续被当作当前入口。

## 9. 分阶段执行与人工 Gate

### Gate A：远端冻结与清单

- 暂停团队向目标分支 push；
- 读取所有公开 heads/tags 及其 SHA；
- 检查分支保护和强推权限；
- 若 refs 与设计快照不同，重新生成计划并停止沿用旧 SHA；
- 确认没有待合并分支会重新引入目标路径。

### Gate B：本地归档验收

- `ming-iot-rd-docs` 无 remote；
- 本地 pre-push 能确定性拒绝；
- 所有已跟踪、未推送和未跟踪文档均已迁移；
- 可阅读归档、完整 Git Bundle 和迁移清单均存在；
- 抽样与全量摘要校验通过；
- 来源 worktree 尚未删除。

Gate B 未通过，禁止进入任何删除或历史重写。

### Gate C：LoopX 切换验收

- 修订后的 Bootstrap 已实施并通过独立 Review；
- `AGENTS.md`、Hook 和日常命令不再依赖 graphctl；
- 新 Evidence/Review 写入本地文档库；
- 至少完成一次 Worker、Reviewer、恢复和用户 Gate 演练；
- 删除 `.graph` 与 `docs/superpowers` 的临时工作树验证不破坏产品构建和新流程。

### Gate D：旧 Graph 当前树退役

- 删除当前树中的旧 Graph 动态控制面；
- 保留确定性业务校验器；
- 清除当前有效文档和配置中的旧权威引用；
- 运行受影响测试与静态检查；
- 由独立 Reviewer 检查没有形成双真源或半拆状态。

### Gate E：历史净化预演

必须在新建的临时 Mirror 中执行，不能直接在日常工作目录操作：

```text
git filter-repo \
  --path docs/superpowers \
  --path .graph \
  --invert-paths
```

具体命令、版本、临时路径和 refs 映射由实施计划固定。禁止使用 `git filter-branch`，禁止对 Gitee 直接执行未经检查的 `git push --mirror`。

预演必须输出：

- 每个远端分支的 old SHA 与 rewritten SHA；
- 被删除路径和对象统计；
- 目标分支拓扑检查；
- 新鲜 Clone 验证结果；
- 回滚命令草案；
- Gitee 强推前的精确 ref 列表。

### Gate F：最终强推

这是独立且不可继承的用户批准点。即使 Gate A-E 全部通过，也不能自动执行。

只允许对确认过的公开分支逐一执行带精确旧 SHA 的 `--force-with-lease`。禁止泛化为所有 refs，禁止删除未知平台引用，禁止使用 `--force` 代替 lease。

### Gate G：远端读回与团队切换

- 从 Gitee 新建 Clone；
- 再次核对公开 heads/tags；
- 验证所有公开可达历史不包含目标路径；
- 验证 LoopX Bootstrap、构建和确定性门禁；
- 通知所有成员废弃旧 Clone，并重新 Clone；
- 根据需要联系 Gitee 支持处理服务端不可达对象回收。

## 10. 历史净化验收谓词

在净化后的全 refs Mirror 和从 Gitee 新建的 Clone 中，以下检查都必须通过：

```text
git log --all -- docs/superpowers .graph
git rev-list --all --objects
git ls-tree -r --name-only <each-public-head>
```

验收条件：

- `git log --all -- docs/superpowers .graph` 无输出；
- 可达对象清单中不存在 `docs/superpowers/` 或 `.graph/` 路径；
- 每个公开 head 的树均不存在目标目录；
- Gitee refs 与批准的 rewritten SHA 完全一致；
- 当前树不存在把 graphctl 或 `.graph` 声明为动态权威的入口；
- `thingsbaseline` 等保留校验器通过；
- LoopX 健康检查、恢复演练和静态 Harness 通过；
- 本地归档仓库及 Git Bundle 可以读取，且仍无 remote。

## 11. 回滚与失败处理

- 本地归档或摘要失败：保留全部来源，停止；
- LoopX 切换失败：继续使用旧 Graph，不进入历史净化；
- Graph 退役导致门禁缺口：恢复当前树删除，修复 Harness 后重新评审；
- Mirror 预演异常：删除临时 Mirror，远端不受影响；
- 强推前 refs 已变化：lease 必须失败，重新冻结和评审；
- 强推结果未知：先读回远端 SHA，禁止盲目重试；
- 强推后发生紧急回滚：只能经用户再次批准，使用本地 Bundle 和已记录 old SHA 恢复；这会重新公开旧材料，因此仅用于灾难恢复；
- 旧 Clone 再次推回旧历史：立即冻结远端，识别来源并重新净化。

## 12. “永久删除”的边界

本设计能够保证：Gitee 项目所有已发现并纳入验收的公开 Branch/Tag 不再引用目标路径，普通 Clone 和历史浏览无法取得这些文件。

本设计不能远程擦除：

- 他人已经创建的 Clone、Fork、补丁或截图；
- Gitee 内部备份和短期缓存；
- 未公开且平台不允许枚举的服务端引用。

如果要求服务端对象立即物理回收，必须在强推并验收后联系 Gitee 支持。对外表述应使用“公开可达历史已净化”，不能声称已经删除所有第三方副本。

## 13. 非目标

- 不在本规格中直接安装或修改 LoopX；
- 不在本规格中执行 commit、merge、push 或 force-push；
- 不修改官方 `things` 仓库；
- 不删除 `thingsbaseline` 或其他确定性业务校验器；
- 不把 `ming-iot-rd-docs` 建设为公司级中央平台；
- 不在完成归档前删除任何 worktree 或未跟踪文档；
- 不以 `.gitignore` 代替已跟踪历史的净化；
- 不用新的自研动态 Registry 替代已经决定采用的 LoopX。

## 14. 完成定义

全部满足才算本次迁移完成：

1. 本地研发文档库已初始化、提交、无 remote 且 pre-push 拒绝；
2. 所有目标文档、旧 Graph 和完整切换前 Git Bundle 已归档并校验；
3. Bootstrap 计划已改为本地文档与 LoopX 唯一动态控制面；
4. LoopX 切换完成并通过 Worker、Reviewer、恢复和用户 Gate；
5. 旧 Graph 动态控制面已从当前代码树整体退役；
6. 确定性工程门禁仍存在且通过；
7. 用户已在 Gate F 对精确 old/new refs 单独批准；
8. Gitee 两个公开分支及实施时发现的其他公开 refs 已完成净化；
9. 新鲜 Clone 证明公开可达历史不存在 `docs/superpowers/**` 和 `.graph/**`；
10. 团队完成重新 Clone/切换通知；
11. 未经明确授权，没有执行额外删除、提交或推送。
