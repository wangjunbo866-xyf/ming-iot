# Multica 与 LoopX：小团队协作与长程执行的边界研究

日期：2026-10-09。用途：为“OpenSpec＋可复现验证环境＋关键节点验收＋清晰责任”的团队方案提供选型依据。用户已确定继续用 LoopX；本研究不替团队决定引入 Multica，不安装服务，不修改运行中的 Goal。

## 结论与证据范围

**Multica 可以作为团队派工与 Agent 身份、运行记录的入口；把它接到 LoopX 上仍是一个待验证的集成方案。** 两者都有任务状态、唤醒、重试和协调能力，不能简单叠加后假定只各管一层。第一轮真实 IoT 需求试点，保留现有 Issue/PR＋LoopX 是有效的基线；当手工找执行者、交接、观察阻塞成为明确成本时，再比较 Multica 是否省下更多团队时间。

本报告中的“已核查”仅指官方文档和固定源码静态核查；未部署 Multica、未运行其测试、未做真实 Gitee 或 LoopX 端到端集成。下文分为：**[事实]** 官方文档或代码可直接支持；**[推断]** 从边界推导的集成风险；**[建议]** 尚未执行的设计或验收实验。

| 对象 | 固定基线 | 范围 |
| --- | --- | --- |
| Multica | [`10a7e519da96e8e1819934c24f89bc55a71f88b5`](https://github.com/multica-ai/multica/tree/10a7e519da96e8e1819934c24f89bc55a71f88b5)，commit 时间 2026-10-09 16:50:47 +08:00 | 官方仓库的当前源码快照；不把 main 行为倒推成某个已安装版本 |
| Multica tag | `git ls-remote --tags` 观察到最高语义版本标签 `v0.6.1`，解引用 commit `2ea01ae4ef55de4310b99af192d2dbd367832883` | 没有逐项检查该 tag；GitHub releases API 403，不能声称本次确认了最新稳定发布或镜像可用性 |
| LoopX | 本机干净源码 [`fc25014937ebdf90a8519825049f34f861d7d3fd`](https://github.com/loopx-project/loopx/tree/fc25014937ebdf90a8519825049f34f861d7d3fd)，`pyproject.toml` 版本 `1.3.1` | 用户现有源码基线；不代表所有宿主都启用了可选事务或硬租约 |

## Multica 提供什么，以及哪些说法不能直接成立

### 许可证

**[事实]** 当前 README 称其为 *source-available*。LICENSE 是完整 Apache 2.0 文本加额外条件组成的“Multica License”，不能简称 Apache 2.0 开源许可证。条文明确单组织内部使用不需商业授权；面向第三方托管（包括免费公开服务）与商业产品嵌入需要商业授权，并有 UI 品牌保留和非 UI 使用的署名条件。仓库 About 仍写 open-source，评估应以固定版本 LICENSE 为准。本项目当前讨论的内部团队使用，正好是其明示允许的场景。[固定 LICENSE](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/LICENSE)、[README](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/README.md)

### 身份、派工与运行

**[事实]** Multica Agent 是可复用的身份及执行配置，有 owner、instructions、provider/model、runtime、skills、并发和访问设置；它不是一直运行的进程。Issue 保存工作目标、讨论、assignee 和状态；Run 保存一次触发和执行结果，一项 Issue 可以有很多 Runs。一个 runtime 可以服务多个 Agent。[Agents](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/apps/docs/content/docs/agents.mdx)、[Runs](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/apps/docs/content/docs/tasks.mdx)

**[事实]** Squad 由一个 Agent leader 与人或 Agent 成员组成。分派给 Squad 先唤醒 leader，leader 通过明确 mention 委派；成员返回结果后触发再次协调。它不默认让所有成员并行，也不会自动增加并发。Squad role description 是上下文，不授予权限；系统管理的 leader protocol 规定分派后停止、交付前保持 `in_progress`，整体交付再 `in_review`。因此 Squad 有实际的协作协议，并非仅一个成员分组。[Squads](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/apps/docs/content/docs/squads.mdx)

**[事实]** Workspace 的 `owner/admin/member` 主要管理团队与设置；每个 Agent 的 Access 另行决定谁可以运行，默认 Only me，管理员不因角色自动获准运行私有 Agent。run token 由服务端绑定运行身份；这能帮助区分谁执行，不等于业务责任自动分配。[角色](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/apps/docs/content/docs/members-roles.mdx)、[Agent Access](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/apps/docs/content/docs/agents.mdx)、[认证代码](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/server/internal/middleware/auth.go)

**[建议]** 团队仍须指定真实人类责任人：需求验收人、实现提交人、独立 reviewer。Agent 名称和 Squad leader 记录执行分工，不能充当“谁对提交结果负责”的替代品。

### Review 与完成不是同一个状态

**[事实]** 官方明确：Run `completed` 只表示本次运行正常结束，不证明 Issue 目标达到。Issue 的 `in_progress/in_review` 由 Agent 显式写入，不由 Run 开始或结束自动推导；`done` 通常来自人工确认或关联 PR 合并。配置中也允许 PR 合并后自动迁移 Issue 状态。[Issue 状态](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/apps/docs/content/docs/issues.mdx)、[Runs](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/apps/docs/content/docs/tasks.mdx)

**[推断]** 这些状态约定不能证明已经强制独立 reviewer、绑定准确 commit 的证据、必需检查或禁止自批。所读 Issue 更新处理包含状态合法性、revision 与事务检查；不能由此推出业务验收合同已执行。应把正式验收放在项目约定与 Git 托管的权限/必需检查中，再向看板投影结果。[Issue 更新代码](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/server/internal/handler/issue.go)

### Git 与 Gitee

**[事实]** 仓库资源虽然类型名为 `github_repo`，官方说明可以接受 runtime 能访问的任意 Git URL；本地目录也可使用，支持原目录串行与 Git worktree 两种模式。普通 checkout 能力与 PR/CI 集成是两层能力。[Project resources](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/apps/docs/content/docs/project-resources.mdx)

**[事实]** 原生 PR/CI 集成包括 GitHub，以及自托管 Multica 的 Forgejo/Gitea/GitLab。源码 VCS provider enum 只有后三项，GitHub 独立实现。本次搜索 `apps/server/packages/README*/LICENSE` 没有 `gitee` 命中。因此只能说**未发现 Gitee 原生适配**；Gitea 不是 Gitee。Gitee 仓库可以按普通 Git 资源准备，而 PR 链接、CI 读回、合并状态同步仍需现有工具或新增适配，尚未验证。[VCS 文档](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/apps/docs/content/docs/vcs-integration.mdx)、[Provider enum](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/server/internal/integrations/vcs/vcs.go)

### 部署和维护成本

**[事实]** 自托管包含 Go API/WebSocket、Next.js 前端和 PostgreSQL 17，官方支持 Docker Compose；执行机器另需 Multica CLI/daemon 与已安装登录的 coding CLI。它不是仅增加一个 Skill。运维范围包括服务/数据库、备份恢复、客户端与 coding CLI 兼容、runtime 在线状态，以及配置和凭据的分发。[Self-hosting](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/SELF_HOSTING.md)、[CLI/Daemon](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/CLI_AND_DAEMON.md)、[CLI 兼容代码](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/server/pkg/agent/version.go)

**[事实]** 默认 run 使用 daemon 操作系统用户的权限；独立工作目录、run-scoped Agent state/API token 不构成文件系统沙箱。官方建议用专用用户、容器或 VM 建立实际执行边界。[Security model](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/apps/docs/content/docs/security-model.mdx)

**[建议]** 用户所说“可复现验证环境”可成为统一接口：无论个人工具还是 Multica Agent，均使用相同的锁定依赖、测试数据和验证入口；Multica 部署成功不能替代验证环境验收。

## LoopX 已有能力及集成证据

**[事实]** 所查 LoopX 架构把 Goal/Todo/claim/gate/monitor/quota、接受写回、恢复与调度判定归 Kernel；Agent/Host 执行有界工作。Host 仍负责实际唤醒，Git/CI/独立验证器各有自己的职责。显式集成的 Turn 流程可以包括执行、类型化结果、验证、持久写回、quota、scheduler ack；不是所有宿主工具调用自动受其覆盖。[运行职责](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/docs/architecture.md)、[一轮工作](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/docs/book/chapters/03-one-turn.md)、[边界](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/docs/book/chapters/04-runtime-boundaries.md)

**[事实]** 当前 Turn driver 支持 `codex-cli/claude-code/dsh/generic-cli`；完成写回代码在锁内比较 Todo source snapshot。硬租约为可选协议：启用它的宿主还须正确传递租约身份、版本与写范围，普通 `claimed_by` 不自动升级为硬排他。[Turn driver](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/loopx/control_plane/turn_driver/driver.py)、[Completion transaction](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/loopx/control_plane/todos/completion_transaction.py)、[Host 协议](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/docs/reference/protocols/host-integration-surface-v0.md)

**[事实]** LoopX 源码中搜到 Multica 产品形态参考和一项历史“documentation only”的社区采用说明，没有发现 Multica runtime/provider adapter。Multica 上述检索范围也无 LoopX 命中。历史 PR 链接本次返回 404，未据此重新确认其合并事实。**没有证据表明两者开箱即用集成**；“都能驱动同一种 coding CLI”不足以证明生命周期契约兼容。[LoopX 参考](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/docs/product/foundations/server-client-product-shape.md)、[社区说明](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/docs/community/ecosystem-adoption.md)

## 推荐的状态所有权：区分工作对象，不同步一堆同名状态

以下是**待团队选择的建议**，不是产品现成功能。OpenSpec 表达业务要求，Issue 表达责任与交付入口，LoopX 表达执行推进；各自状态含义不同。

| 工作事实 | 建议唯一权威 | 其他工具记录什么 |
| --- | --- | --- |
| 需求、场景、计划、验收条件 | 文档仓 OpenSpec 的明确 revision | 链接、ID 和摘要；不再手写另一份完整规格 |
| 人类责任、当前交付分派 | 选定一个 Issue 系统 | Multica 若加入，决定其是入口还是镜像；禁止 Gitee 与 Multica 各自自由改同一个 owner |
| 长程执行下一步、gate/monitor/quota | 绑定的 LoopX Goal | 看板呈现带时间戳的执行摘要、阻塞原因与证据引用 |
| 一次 Agent 进程/会话状态 | 实际启动它的 runtime/宿主 | LoopX 读取运行结果后判定后续；不能用进程退出推导业务验收 |
| 代码、候选版本与 review | Git/PR＋项目验收记录 | 保存 repo、完整 SHA、PR、reviewer 和证据入口 |
| 团队 Skills 内容 | 已选择的 Git 发布源及版本 | Multica/OpenViking 可用于发现或分发，避免同时成为可编辑权威副本 |

Multica Skills 从 runtime 导入是文件快照；本地修改不会自动同步。URL 来源的 refresh 会替换工作区内容，并覆盖该副本本地编辑。若 OpenViking 也管理 Skills，此处尤其需要版本来源约定。[Skills](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/apps/docs/content/docs/skills.mdx)

### 两条可以比较的路径

**A：保留现有 Issue/PR＋LoopX，先改善交接。** 一条 IoT 需求绑定 OpenSpec change、责任人、PR 与验证命令；LoopX 推进长程工作。以重复解释次数、等待责任人时间、证据退回次数、人工维护时间为基线。适合当前规模还小、主要痛点是验收和上下文的团队。这里不需要为了“AI Native”先引入新协调服务。

**B：在 A 的合同上试用 Multica 管派工和 Agent 运行。** 首次仅挑一个有明确输入输出的任务，不建复杂 Squad。Issue 给出绑定 Goal 和规格 revision，worker 读取当前 LoopX 合同；输出回原 Issue。通过试点后，再比较 Squad 是否省下真实路由工作。需要一个明确的唤醒责任人：可以保留现有 LoopX 宿主作为唯一续跑者，Multica 先记录和派一次工作；或者由适配层让 Multica 成为 LoopX 的宿主。后一项是开发与验证工作，不是现有配置开关。

LoopX 的持久 Goal/registry 应绑定稳定存储，不能只放在会清理的 per-run worktree；重试换目录后仍须找回同一状态。本项为集成设计要求，尚未验证。

“Multica 管团队、LoopX 管单项”在逻辑上可行，前提是把**一个业务 Issue 的长期目标**与**一次 execution Run**分开，并限制每个 Goal 的唤醒、重试和写回来源。不要同时给同一 Goal 打开 Multica Autopilot、Squad 自动续派和另一宿主的独立 heartbeat，然后期待双方自然去重。

### 必须处理的具体冲突

| 冲突 | 已知产品事实 | 建议协议 |
| --- | --- | --- |
| 重试与重复工作 | Multica 对部分基础设施失败自动重试；手动 retry 可重用前次目录/会话 | 同一交付使用稳定 `issue ↔ goal` 映射；每次 run 携带唯一 attempt/turn 身份；读回旧效果再决定续做，不能重新创建 Goal |
| 换人后旧执行仍在跑 | 改 assignee/status 不停止已开始的 run，重新指派 Agent 会触发新 run | 明确停止旧 run、确认旧写权限退出，再转交；保留 pending/unknown，不能把看板改名当完成接管 |
| 旧 run 被重新启动 | 点击历史 retry 使用历史 Agent；`issue rerun` 使用当前 assignee | 重试前检查当前 owner、Goal revision、工作目录和输入版本；不能凭历史 run 名字获得新写权 |
| 同一分支竞争 | Multica 有 per-run 工作目录、本地目录锁和 worktree 模式 | Git 分支/写范围与 LoopX lease 对齐；不同工具各自的锁不能假定互通 |
| 提前宣告完成 | Run completed 与 Issue done 分离；LoopX Todo 完成也不自动证明 Goal 验收 | 只投影已验证产物；独立 review 和验收后才关闭业务交付 |

前三行依据：[Runs](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/apps/docs/content/docs/tasks.mdx)、[Assigning issues](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/apps/docs/content/docs/assigning-issues.mdx)。工作目录依据：[Project resources](https://github.com/multica-ai/multica/blob/10a7e519da96e8e1819934c24f89bc55a71f88b5/apps/docs/content/docs/project-resources.mdx)。LoopX 完成边界见前述运行职责与一轮工作文档。

## 最小验证场景与采用条件

以下仅是**建议的实验**，本次均未运行。

1. **一条需求到待审产物。** 输入已有 IoT OpenSpec change 和固定代码基线；完成一个行为切片，返回 red/green 测试记录、候选 SHA、文档一致性说明、独立 reviewer 意见。验收检查执行者确实可用另一成员环境复验；看板不能在 review 前把交付说成验收通过。
2. **中断后续跑，不重复副作用。** 在已产生 commit 或已提交验证结果后制造可控进程/网络中断，恢复时使用同一 Goal，核对是否只补缺失步骤。记录是否产生双运行、重复 PR/评论/资源操作、旧证据误复用。仅用隔离 fixture，不在真实业务状态中注入故障。
3. **交接与权限。** 运行中改 assignee，按设计停旧 run 并交接；验证旧 worker 无法继续接受新写回，个人 Agent Access 不被 Squad 或管理员角色绕过；从个人上下文交接时只传必要引用，遵守用户已确定的私有记忆边界。
4. **Gitee 的真实可用范围。** 用团队实际托管方式验证 checkout/branch/PR 创建、精确 SHA 的 CI/readback、review 和合并后的状态。原生缺失项明确走现有 Gitee 工具，测量维护与同步成本，不用 GitHub 演示替代。

采用 Multica 的判断应来自两轮可比任务：是否减少“谁来处理、现在卡在哪、证据在哪里”的人工协调；同时统计额外服务维护、重复状态修复、失败恢复时间和新成员上手时间。先记录基线再设阈值，避免用一次 agent 演示或 token 数量替代团队交付效果。若 B 的协调节省不足以覆盖平台/适配成本，继续 A，并保留统一的文档和验收接口即可。

## 本次验证记录

- 官方仓库浅克隆只用于只读研究；没有运行安装脚本、daemon、容器或远端写入。
- 固定 Multica/LoopX commit，并检查相关文档、LICENSE、VCS enum、Issue 更新、身份认证和 LoopX Turn/完成写回代码。
- `gitee/loopx` 的检索是指定源码范围的否定证据，不宣称全互联网不存在第三方插件。
- 提交前执行 `python3 -m unittest discover -s tests -v`：52 项通过；`python3 tools/team_skills.py validate`：通过。检查仅证明研究文档所在配置仓库的既有检查保持通过，不能充当上述产品能力的端到端验收。
