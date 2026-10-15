# Matt Pocock 如何从 Spec 推进到代码

核查日期：2026-10-11。研究依据为 Matt Pocock 官方公开仓库及对应说明，固定在 `mattpocock/skills` 的 main 提交 [`49dd158d1076134a641b33efb035946536778336`](https://github.com/mattpocock/skills/commit/49dd158d1076134a641b33efb035946536778336)，提交日期 2026-10-09，仓库 `package.json` 版本 `1.3.1`。这里分析的是公开 Skill 的行为约定，不是对 Matt 所有实际项目的执行审计。本轮没有安装或运行这些 Skills，没有创建远端 Issue、提交或 PR。

## 结论

最重要的适配差异：Matt 把 Spec 定位为一次工作的决策快照，没有自动同步机制，交付后可丢弃；长期知识沉淀到术语表和 ADR。这与我们希望持续维护的 Feature 主 Spec 不同，不能直接照搬其文档生命周期。[Spec 说明](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/docs/engineering/to-spec.md)

Matt 的多会话开发路径是：先把讨论整理成 Spec，再拆成带依赖的可验证 Ticket，随后通过 TDD 编码与 Review 交付。独立的 `plan.md` 和 `tasks.md` 不是必经产物；任务计划主要由 Ticket 的范围、验收与阻塞关系表达。单次上下文能完成的小变更可以直接进入实现。这个流程来自当前官方路由说明，而非必须全部执行的固定阶段清单。[ask-matt](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/skills/engineering/ask-matt/SKILL.md)

```text
讨论、澄清与已有项目上下文
  ├─ 小变更：implement
  └─ 多会话：to-spec → to-tickets
                         ├─ 每张 Ticket：implement
                         └─ 整个任务图：implement-spec
实现使用 TDD；收尾使用 code-review
```

## Spec 和 Ticket 各装什么

`to-spec` 接收已有讨论与代码库理解，使用项目术语并遵守相关 ADR，把已经决定的内容整理后发布到配置的 Tracker。Spec 包含问题、解决方案、用户故事、实现决策、测试决策、范围外事项等；所以它并非只有需求陈述。生成前还会确认用于验证功能的公开接口边界，优先采用已有且较高层的边界。它不重新进行需求访谈，也不应为补齐模板擅自发明决定。[to-spec](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/skills/engineering/to-spec/SKILL.md)

`to-tickets` 可直接读取 Spec、Plan、当前讨论，或用户给出的路径／Issue 引用。它把工作拆成贯穿相关层的窄功能切片，每张能独立演示或验证，并适合一个全新上下文完成。发布前需要确认粒度与依赖；每张 Ticket 包含交付行为、验收条件、前置 Ticket，来源是已有 Issue 时关联父 Issue。因此，“Ticket 自包含”指新会话能够按它开展工作，不意味着复制整个 Spec。大范围机械重构允许采用先兼容扩展、再分批迁移、最后删除旧形式的顺序。[to-tickets](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/skills/engineering/to-tickets/SKILL.md)

## 放在哪：Tracker 与 Git 文件不是一回事

首次配置选择 Tracker、状态标签及术语／ADR 的位置，并把项目约定写入 `docs/agents/`，通过 `AGENTS.md` 或已有 `CLAUDE.md` 指向它们。Tracker 可选择 GitHub、GitLab、本地 Markdown，或记录其它系统的操作约定。配置 Skill 是提示驱动流程，不是确定性安装脚本。[setup-matt-pocock-skills](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/skills/engineering/setup-matt-pocock-skills/SKILL.md)

| Tracker 选择 | Spec | 实现 Ticket |
| --- | --- | --- |
| GitHub / GitLab | 作为 Issue 保存 | 每张对应一个 Issue，关联父项和依赖 |
| 本地 Markdown | `.scratch/<feature>/spec.md` | `.scratch/<feature>/issues/<NN>-<slug>.md`，每票一文件 |

远端两类模板明确把 Spec 和 Issue 都放在对应 Tracker。本地模板规定上述路径、状态行与追加评论，但没有规定这些文件必须被 Git 跟踪，或必须加入 `.gitignore`。因此不能得出“只有 Spec 进 Git，Task/Plan 不进 Git”；使用远端 Tracker 时，连 Spec 本身也不必是仓库中的 Markdown 文件。[GitHub 模板](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/skills/engineering/setup-matt-pocock-skills/issue-tracker-github.md)、[GitLab 模板](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/skills/engineering/setup-matt-pocock-skills/issue-tracker-gitlab.md)、[本地模板](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/skills/engineering/setup-matt-pocock-skills/issue-tracker-local.md)

## 实现与审查如何连接

单张 Ticket 使用 `implement`：读取来源，在先前约定的测试边界驱动 TDD，过程中常做类型／测试反馈，最后执行完整测试、代码审查，再提交当前分支。这里的 commit 是源码里的执行约定，本研究没有执行它。[implement](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/skills/engineering/implement/SKILL.md)

整个 Spec 使用 `implement-spec`：把 Ticket 当依赖图，创建集成分支；就绪 Ticket 由实现子 Agent 在独立 worktree／分支推进；合并器把结果集成到同一分支，最终统一审查和收尾。沟通尽量传递 Spec、Ticket、研究记录及 commit 的引用，避免复制上下文。源码规定了并行协作的操作流程，但不能凭此证明某项目已经正确处理所有冲突或通过团队集成验收。[implement-spec](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/skills/engineering/implement-spec/SKILL.md)

当前 `tdd` 正文强调一次一个行为，先确认测试失败，再实现到通过；测试期望来自 Spec 或独立例子。重构由审查阶段推动。不要用较宽泛的旧式“Red-Green-Refactor”概述，替代核对当前正文的边界。[tdd](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/skills/engineering/tdd/SKILL.md)

`code-review` 固定比较起点，通过两个上下文隔离的子 Agent 分别审查代码标准与 Spec 符合度；找不到 Spec 来源时允许跳过该维度并说明。它是 Agent 审查协议，不等于独立人类批准，也不是服务端 CI 或保护分支的强制门禁。[code-review](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/skills/engineering/code-review/SKILL.md)

编写 PR 时，`pr` 要求简明展示变化、提供变更前后的执行或截图证据，并说明回退难度与影响范围。这是对可审查产物的要求，不等同于已经批准或发布。[pr](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/skills/engineering/pr/SKILL.md)

## 与我们的目标最重要的差异

除了开头所述的 Spec 生命周期区别，官方说明还提醒：父 Spec 的 `ready-for-agent` 标签可能被无人值守 Agent 误当成整项实现任务，不能只凭标签判断应执行哪个 Ticket。[Spec 说明](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/docs/engineering/to-spec.md)

官方 Ticket 说明承认过度拆分、按层拆分、验收条件不可证伪、执行后状态未及时更新等问题。它建议检查每张票独立可演示的行为及会令验收失败的观察；`to-tickets` 本身只产出票据，不负责自动派工。说明中关于手工执行的段落与后续 `implement-spec` 自动编排选项并存，适用入口要区分。[Ticket 说明](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/docs/engineering/to-tickets.md)

## 旧名字与当前名字

官方 CHANGELOG 记录了 `to-prd` 改为 `to-spec`，`to-plan` 与 `to-issues` 合并为 `to-tickets`；后续又将本地单个 `tickets.md` 修正为每张 Ticket 一个文件，并使用 `spec.md` 替代 `PRD.md`。旧课程或文章里仍能看到 PRD 与多阶段 Plan，但不能把新旧名字拼成一套当前强制流程。[CHANGELOG](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/CHANGELOG.md)

## 适配 map-delivery 与 LoopX 的建议

以下是针对当前团队的设计建议，不是 Matt 原生规则，也不是已批准的迁移方案。

用户已明确：**只有长程任务使用 LoopX，普通 Patch 开发不需要 LoopX。** 分为两条路线：

- **普通 Patch**：既有 Spec／Patch + 可选短清单 → 实现与 TDD → 独立 Review → 验收。不引入 LoopX，不必再建独立 Spec、Plan 和 Tickets。
- **长程、跨会话任务**：Spec／Patch + 自包含 Tickets 与依赖图 → LoopX 推进 → 各切片 TDD 与 Review → 集成验收。LoopX 承担长程控制；Matt 的 `implement-spec` 是另一种整图编排选项，同一任务不能由两者同时掌控推进与完成状态。

1. 保留我们需要的长期 Feature 主 Spec 与 Patch 历史；借鉴 Matt 的“先明确决策，再拆可验收切片”，不要直接采用交付后丢弃 Spec 的生命周期。
2. 长程工作需要拆 Ticket 时，每张只记录该切片的交付行为、验收、负责人、前置项及确定版本的 Spec／Patch 指针。计划可以由依赖图表达，不再强制重复维护 `plan.md` 和 `tasks.md`。
3. 需要跨仓发布顺序、兼容策略或回滚安排时，再保存一份共享 Plan；个人 Agent 的试验顺序与临时 TODO 留在执行上下文。团队 Task 状态只保有一个权威位置，避免 Tracker、LoopX 与 Git 清单反复手工同步。
4. 普通 Patch 的直接实现入口与长程工作中的单票编码，都可借鉴 Matt 的 TDD 和 Review，不以部署长程控制器为前提。
5. 把我们已选定的 D／R 分离、来源齐全、最终候选版本证据、合并前最低验证、发布前完整集成验收作为项目门禁补上。Matt 的两个 Review 子 Agent 可以提供证据，但不替代不同成员承担的人类责任。

以“遥测温度支持小数”为例，第一票可交付模拟设备上报小数、入库并由查询接口读回的最窄闭环；第二票在此前提上支持页面展示和精度边界；兼容旧整数的场景应归属于对应闭环的验收，而不是等待最后另一个团队补齐。具体切片取决于真实架构，不应机械地划成“数据库票、后端票、前端票”。
