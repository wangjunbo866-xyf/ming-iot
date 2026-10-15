# 高星项目怎样组织仓库自用 Skills 与项目上下文

核查日期：2026-10-11。这里只观察官方公开仓库，不安装 Skills、不运行其指令，也不审计 PR。星数用于说明项目规模与可参考性，不作为工程质量或实际采用效果的证明。文件被提交、被 AGENTS 引用，证明存在公开的维护约定；不证明每位贡献者或每次会话都执行过这些约定。

本研究把“业务地图”定义为：从领域／功能找到概念、行为规则、接口／实现、责任和验证入口。普通用户教程目录不自动满足这个定义；一个项目也可能通过多个互相链接的入口实现，而不使用 `map/` 这个名字。

## OpenClaw：功能分类与验证映射较完整，知识入口分散组合

本轮 [GitHub 元数据](https://api.github.com/repos/openclaw/openclaw) 为 391,636 stars，默认分支 `main`。固定提交 [`c6d71fae0ed1c0f7968c8d05c410345246af235b`](https://github.com/openclaw/openclaw/commit/c6d71fae0ed1c0f7968c8d05c410345246af235b)，2026-10-11 03:23:04 UTC。完整根递归树读取遇到传输错误，随后按固定根树读取 `.agents`、`.claude`、`docs` 等子树；所列路径均在固定提交下确认。

### 已核实的仓库自用 Skills

这些文件在 `.agents/skills/`，正文明确针对 OpenClaw 仓库开发和维护；根 [AGENTS.md](https://github.com/openclaw/openclaw/blob/c6d71fae0ed1c0f7968c8d05c410345246af235b/AGENTS.md) 直接连接测试、PR 等维护入口。本表不把根 `skills/`、custodian 能力或产品测试 fixture 自动计入维护 Skills。

| Skill | 实际用途 | 一手来源 |
| --- | --- | --- |
| `openclaw-pr-maintainer` | Issue／PR 调查、Review、修复与授权后的合入；区分行为符合度与仓库标准，连接测试和 CODEOWNERS | [SKILL.md](https://github.com/openclaw/openclaw/blob/c6d71fae0ed1c0f7968c8d05c410345246af235b/.agents/skills/openclaw-pr-maintainer/SKILL.md) |
| `openclaw-testing` | 按改动契约选择适量检查，区分本地、隔离、集成／发布验证，记录未覆盖部分 | [SKILL.md](https://github.com/openclaw/openclaw/blob/c6d71fae0ed1c0f7968c8d05c410345246af235b/.agents/skills/openclaw-testing/SKILL.md) |
| `channel-message-flows` | 从仓库根执行 Telegram 草稿／最终消息顺序的 QA 场景，连接场景 YAML 和 transport／plugin 代码 | [SKILL.md](https://github.com/openclaw/openclaw/blob/c6d71fae0ed1c0f7968c8d05c410345246af235b/.agents/skills/channel-message-flows/SKILL.md) |
| `claw-score` | 维护产品功能分类、质量聚合与 QA 证据映射，并生成成熟度文档 | [SKILL.md](https://github.com/openclaw/openclaw/blob/c6d71fae0ed1c0f7968c8d05c410345246af235b/.agents/skills/claw-score/SKILL.md) |

### 最接近业务地图的结构

```text
AGENTS.md                         按领域指向对应 scoped AGENTS、文档与测试
taxonomy.yaml                     surface → category → feature
  └─ docs / coverageIds           功能说明与验证身份
qa/maturity-scores.yaml            已提交的质量／完整性聚合
qa/scenarios/index.yaml            QA 场景入口
docs/maturity/                    由维护来源生成的公开视图
.github/CODEOWNERS                 敏感安全／发布路径的人类审查责任
```

[taxonomy.yaml](https://github.com/openclaw/openclaw/blob/c6d71fae0ed1c0f7968c8d05c410345246af235b/taxonomy.yaml) 实际把 Gateway 等产品面分成 API、权限、连接、生命周期等类别，每个功能有说明与 coverage ID，类别连接相关文档。根 AGENTS 再按插件、消息、Agent、存储、UI 等方向连接源码侧约束和验证指南，弥补仅有功能目录不足以定位实现的问题。

`claw-score` 明确手工权威来源是 taxonomy；聚合结果在 `qa/maturity-scores.yaml`，公开 [功能分类视图](https://github.com/openclaw/openclaw/blob/c6d71fae0ed1c0f7968c8d05c410345246af235b/docs/maturity/taxonomy.md)与[评分视图](https://github.com/openclaw/openclaw/blob/c6d71fae0ed1c0f7968c8d05c410345246af235b/docs/maturity/scorecard.md)由命令生成；单次 `qa-evidence.json` 是运行制品，不作为一套重复的 inventory 报告提交。这是“维护一次来源、生成阅读视图”的实际约定。

**判断与限制：** 这个组合接近“功能 → 契约／文档 → 代码入口 → 验证”的地图。但来源分散，并非每项功能都有同一张完整页面；[CODEOWNERS](https://github.com/openclaw/openclaw/blob/c6d71fae0ed1c0f7968c8d05c410345246af235b/.github/CODEOWNERS) 的人类责任规则主要覆盖安全和发布路径，不能说它为每个 Feature 都分配了责任人。taxonomy 顶部仍有 Draft 与旧 snapshot 信息，本轮未执行生成／校验，也不把评分等级当作独立验收结果。

## OpenCode：少量专项 Skills，加当前契约／决策／历史的清晰导航

本轮 [GitHub 元数据](https://api.github.com/repos/anomalyco/opencode) 为 212,577 stars，默认分支 `dev`。固定提交 [`0d61729b3fa005cec9aa754c075a809663c736a4`](https://github.com/anomalyco/opencode/commit/0d61729b3fa005cec9aa754c075a809663c736a4)，2026-10-11 02:03:06 UTC；完整递归树未截断。

### 已核实的仓库自用 Skills

| Skill | 实际用途 | 一手来源 |
| --- | --- | --- |
| `effect` | 本仓库 Effect v4 代码、服务、Schema 与测试约定；要求查当前依赖源码和本仓库既有用法 | [SKILL.md](https://github.com/anomalyco/opencode/blob/0d61729b3fa005cec9aa754c075a809663c736a4/.opencode/skills/effect/SKILL.md) |
| `opencode-dev` | 调试 OpenCode 自身 V2 CLI／TUI／Server，连接启动方式、API、日志和可检查的截图证据 | [SKILL.md](https://github.com/anomalyco/opencode/blob/0d61729b3fa005cec9aa754c075a809663c736a4/.opencode/skills/opencode-dev/SKILL.md) |
| `rtl-aware-development` | Web／Desktop 双向文本和 RTL 布局、交互、滚动、菜单及窗口控件的实现与验收矩阵 | [SKILL.md](https://github.com/anomalyco/opencode/blob/0d61729b3fa005cec9aa754c075a809663c736a4/.opencode/skills/rtl-aware-development/SKILL.md) |

`.opencode/skills/sample-skill` 的正文明确是格式与使用演示，未计入三项实际工程能力；`packages/core/test/config/fixture/skills/` 也排除。不能把仓库恰好测试 Skill 加载功能，误当成团队正在使用这些 fixture 开发。

### 最值得借鉴的入口

[specs/v2/README.md](https://github.com/anomalyco/opencode/blob/0d61729b3fa005cec9aa754c075a809663c736a4/specs/v2/README.md) 把内容明确分为三层：

```text
specs/v2/
  README.md                              按关注点声明权威来源
  session.md / tools.md                   当前跨模块行为契约
  event-stream-architecture.md            已接受、已实现的决策记录
  provider-policy.md                      已接受、已实现的决策记录
  catalog-config-plugin-lifecycle.md      历史比较与背景
```

README 的 Authority 表把 HTTP 操作交给 Protocol、领域形状与事件交给 Schema、运行与持久化交给 Core、开发回归约束交给根 AGENTS，并直接连接这些代码路径。它特别说明当前规格不复制精确类型；活跃工作放 GitHub Issues；已移除的计划／草稿由 Git 历史保存；不要在当前规格目录增加实施清单。[Session 契约](https://github.com/anomalyco/opencode/blob/0d61729b3fa005cec9aa754c075a809663c736a4/specs/v2/session.md)与[Tools 契约](https://github.com/anomalyco/opencode/blob/0d61729b3fa005cec9aa754c075a809663c736a4/specs/v2/tools.md)正文确实解释准入、执行、恢复或工具注册等跨模块规则，而不只是目录名叫 specs。

代码附近还有根与 package 级 AGENTS，例如 [App 约定](https://github.com/anomalyco/opencode/blob/0d61729b3fa005cec9aa754c075a809663c736a4/packages/app/AGENTS.md)把 Session／Timeline 变化和前后性能基线、真实界面验证连接起来。[CODEOWNERS](https://github.com/anomalyco/opencode/blob/0d61729b3fa005cec9aa754c075a809663c736a4/.github/CODEOWNERS)当前只列 App／Desktop 的人类责任。

**判断与限制：** 这是实用的“语义契约地图”，很接近将长期项目知识与一次交付任务分开的目标；其覆盖以 V2 核心概念为主，并非全产品领域／Feature 清单，也未把每项契约的 Owner 和测试入口都统一挂在一个功能页上。

## Supabase：任务到 Skill 的路由，加有维护规则的模块上下文地图

本轮 [GitHub 元数据](https://api.github.com/repos/supabase/supabase) 为 111,323 stars，默认分支 `master`，固定提交 [`8617532ae1a41571d5d557cd24c6f5f2565d9230`](https://github.com/supabase/supabase/commit/8617532ae1a41571d5d557cd24c6f5f2565d9230)。本轮针对相关目录和正文核查，没有审计整个递归树。

根 [AGENTS.md](https://github.com/supabase/supabase/blob/8617532ae1a41571d5d557cd24c6f5f2565d9230/AGENTS.md)明确将 `.agents/skills` 作为约定来源，要求工作前加载相关 Skills；`CLAUDE.md` 引用 AGENTS，`.claude/skills` 是指向同一来源的 Git 符号链接。不能从其他项目的选择性采用推断这里的 Skills 也都可忽略。本轮确认该目录有 22 个子目录。

| 已读 Skill | 用途 | 一手来源 |
| --- | --- | --- |
| `ask-the-docs` | 在修改 Docs 应用前按问题加载相关架构／流程参考，并用当前代码核对 | [SKILL.md](https://github.com/supabase/supabase/blob/8617532ae1a41571d5d557cd24c6f5f2565d9230/.agents/skills/ask-the-docs/SKILL.md) |
| `safe-sql-execution` | 执行 SQL 时的项目安全约定 | [SKILL.md](https://github.com/supabase/supabase/blob/8617532ae1a41571d5d557cd24c6f5f2565d9230/.agents/skills/safe-sql-execution/SKILL.md) |
| `studio-testing` | Studio 开发的测试约定 | [SKILL.md](https://github.com/supabase/supabase/blob/8617532ae1a41571d5d557cd24c6f5f2565d9230/.agents/skills/studio-testing/SKILL.md) |
| `review-the-docs` | 文档审查约定 | [SKILL.md](https://github.com/supabase/supabase/blob/8617532ae1a41571d5d557cd24c6f5f2565d9230/.agents/skills/review-the-docs/SKILL.md) |

最接近地图的实际文件是 [ask-the-docs/reference/app-map.md](https://github.com/supabase/supabase/blob/8617532ae1a41571d5d557cd24c6f5f2565d9230/.agents/skills/ask-the-docs/reference/app-map.md)：它用图和目录职责表连接路由、内容、渲染、两类流水线、Markdown 导出、集成点和验证入口。[management-api-reference.md](https://github.com/supabase/supabase/blob/8617532ae1a41571d5d557cd24c6f5f2565d9230/.agents/skills/ask-the-docs/reference/management-api-reference.md)进一步解释 API Schema、生成的参考文档、Studio／Docs 共用的权限目录及陈旧生成片段的检查。

```text
AGENTS.md / apps/studio/AGENTS.md        任务 → 相关 Skills
.agents/skills/ask-the-docs/
  SKILL.md                             什么时候加载、核对和维护参考
  reference/app-map.md                  Docs 应用的架构与定位地图
  reference/management-api-reference.md 具体链路及生成来源
  reference/...                        构建、搜索、配置、已知问题等专题
```

`ask-the-docs` 要求发现参考失准或有普遍价值的 Review 经验时，经 PR 更新相关参考；文件尽量保持约 250 行以内，过长则拆分，不重复写快速读代码即可得到的事实。它没有明确要求一定在同一个实现 PR 更新。

**判断与限制：** 这是最值得借鉴的“地图如何不腐烂”的样本：选择性加载、核对活代码、控制长度、仅保存不容易还原的知识。它是 Docs 模块架构／上下文地图，不是 Supabase 全产品的人类 Owner／业务功能地图。`apps/docs/content/guides` 的 Auth／Database／Storage 等是公开产品文档；`apps/docs/spec` 是 API／SDK 参考生成输入，不能仅凭名称当作一次交付 Spec。入口可从 [Docs 贡献说明](https://github.com/supabase/supabase/blob/8617532ae1a41571d5d557cd24c6f5f2565d9230/apps/docs/CONTRIBUTING.md)继续追踪。

## Dify：按功能就近保存上下文，Skill 指向权威约定

本轮 [GitHub 元数据](https://api.github.com/repos/langgenius/dify) 为 158,106 stars，默认分支 `main`，固定提交 [`4410f1f39fcbf17ea260cbe713e24c858a9c28b2`](https://github.com/langgenius/dify/commit/4410f1f39fcbf17ea260cbe713e24c858a9c28b2)，完整 Git 树未截断。

实际 `.agents/skills` 有五项：`backend-code-review`、`frontend-code-review`、`frontend-testing`、`how-to-write-component`、`e2e-cucumber-playwright`。本轮未深入读取 `frontend-code-review` 正文；不能把确认目录存在写成已审计其执行细节。

| 已核查 Skill | 作用／可借鉴点 | 一手来源 |
| --- | --- | --- |
| `backend-code-review` | 后端代码审查入口 | [SKILL.md](https://github.com/langgenius/dify/blob/4410f1f39fcbf17ea260cbe713e24c858a9c28b2/.agents/skills/backend-code-review/SKILL.md) |
| `frontend-testing` | 很薄的测试入口：按目标路由到 Web 或组件库的权威测试文档，不另建第二套策略 | [SKILL.md](https://github.com/langgenius/dify/blob/4410f1f39fcbf17ea260cbe713e24c858a9c28b2/.agents/skills/frontend-testing/SKILL.md) |
| `how-to-write-component` | 组件开发的共享指引 | [SKILL.md](https://github.com/langgenius/dify/blob/4410f1f39fcbf17ea260cbe713e24c858a9c28b2/.agents/skills/how-to-write-component/SKILL.md) |
| `e2e-cucumber-playwright` | Cucumber／Playwright 端到端测试入口 | [SKILL.md](https://github.com/langgenius/dify/blob/4410f1f39fcbf17ea260cbe713e24c858a9c28b2/.agents/skills/e2e-cucumber-playwright/SKILL.md) |

[web/AGENTS.md](https://github.com/langgenius/dify/blob/4410f1f39fcbf17ea260cbe713e24c858a9c28b2/web/AGENTS.md)区分应用需求、共享架构指引和 `dify-ui` 基础组件契约的归属，并要求连接权威来源而非重新描述。具体上下文就近保存：

| 实际入口 | 它连接什么 |
| --- | --- |
| [web/features/agent-v2/AGENTS.md](https://github.com/langgenius/dify/blob/4410f1f39fcbf17ea260cbe713e24c858a9c28b2/web/features/agent-v2/AGENTS.md) | 功能边界、状态归属、payload 判别规则与代码位置 |
| [packages/dify-ui/README.md](https://github.com/langgenius/dify/blob/4410f1f39fcbf17ea260cbe713e24c858a9c28b2/packages/dify-ui/README.md) | 组件类别 → 公共接口 → 专题／组件契约 → 测试 |
| [run-lifecycle/index.md](https://github.com/langgenius/dify/blob/4410f1f39fcbf17ea260cbe713e24c858a9c28b2/dify-agent/docs/dify-agent/concepts/run-lifecycle/index.md) | Agent 运行的状态、资源、失败、取消与快照迁移语义 |
| [api/AGENTS.md](https://github.com/langgenius/dify/blob/4410f1f39fcbf17ea260cbe713e24c858a9c28b2/api/AGENTS.md) | 局部契约由邻近代码的 docstring 承载，仅在其负责的行为改变时更新 |

**判断与限制：** 本轮没找到统一的“整个 Dify 业务域 → Feature”地图；已核实的是分布式模块上下文和契约归属。值得借鉴的是薄 Skill 路由和就近维护，不要求把同一契约复制到 Skill、地图和代码三处。这里的“状态归属”不等于人类责任人分配。

## n8n：共享 Skill 来源、模块文档地图与有责任人的 ADR

本轮 [GitHub 元数据](https://api.github.com/repos/n8n-io/n8n) 为 206,940 stars，默认分支 `master`，固定提交 [`7424f0ca0452f9dc9fe4cf6627ecbf0b315ed1e3`](https://github.com/n8n-io/n8n/commit/7424f0ca0452f9dc9fe4cf6627ecbf0b315ed1e3)，完整 Git 树未截断。

[.agents/skills/AGENTS.md](https://github.com/n8n-io/n8n/blob/7424f0ca0452f9dc9fe4cf6627ecbf0b315ed1e3/.agents/skills/AGENTS.md)与 [Claude 插件说明](https://github.com/n8n-io/n8n/blob/7424f0ca0452f9dc9fe4cf6627ecbf0b315ed1e3/.claude/plugins/n8n/README.md)说明，24 项共享 Skills 在 `.agents/skills` 维护，通过插件符号链接或 OpenCode 直接使用同一来源。

| 仓库自用 Skill | 已核查用途 | 一手来源 |
| --- | --- | --- |
| `reproduce-bug` | 缺陷复现工作流 | [SKILL.md](https://github.com/n8n-io/n8n/blob/7424f0ca0452f9dc9fe4cf6627ecbf0b315ed1e3/.agents/skills/reproduce-bug/SKILL.md) |
| `human-like-code-review` | 先读 Linear／Issue／Spec／代码上下文，再审 diff；Review 输出到被忽略的 `tmp/review-*.md` | [SKILL.md](https://github.com/n8n-io/n8n/blob/7424f0ca0452f9dc9fe4cf6627ecbf0b315ed1e3/.agents/skills/human-like-code-review/SKILL.md) |
| `spec-driven-development` | 规定 Spec 驱动开发及 `.agents/specs` 位置；本轮固定树没有该位置的实际已提交规格文件 | [SKILL.md](https://github.com/n8n-io/n8n/blob/7424f0ca0452f9dc9fe4cf6627ecbf0b315ed1e3/.agents/skills/spec-driven-development/SKILL.md) |

另有 [commands/plan.md](https://github.com/n8n-io/n8n/blob/7424f0ca0452f9dc9fe4cf6627ecbf0b315ed1e3/.claude/plugins/n8n/commands/plan.md)定义输出到被 Git 忽略的 `.claude/plans/<TICKET-ID>.md`；这是命令，不能计入 Skill 数量。以上说明公开约定与实际已提交内容可能不同，不能据 Skill 所规定的路径声称存在公开 Spec 库。

最有用的模块地图是 [packages/@n8n/instance-ai/CLAUDE.md](https://github.com/n8n-io/n8n/blob/7424f0ca0452f9dc9fe4cf6627ecbf0b315ed1e3/packages/%40n8n/instance-ai/CLAUDE.md)，指向同目录下这些实际文档：

```text
docs/
  architecture.md          产品功能、系统图与后端／类型／前端代码
  streaming-protocol.md    流式协议
  tools.md / memory.md     工具与记忆
  filesystem-access.md     文件系统边界
  sandboxing.md            沙箱约束
  configuration.md         配置
  e2e-tests.md             端到端验证
```

其中 [architecture.md](https://github.com/n8n-io/n8n/blob/7424f0ca0452f9dc9fe4cf6627ecbf0b315ed1e3/packages/%40n8n/instance-ai/docs/architecture.md)连接功能、系统与代码。这个模块内部的 planning／workflow-builder 属于产品运行时 Agent 的 Skills，不能混入维护 n8n 仓库的团队 Skills 清单。该模块 CLAUDE 还要求把重新生成的 expectations／fixtures 与测试一起提交；因此“执行过程文件常被忽略”不等于所有验证相关文件都应排除 Git。

[docs/adr/README.md](https://github.com/n8n-io/n8n/blob/7424f0ca0452f9dc9fe4cf6627ecbf0b315ed1e3/docs/adr/README.md)是另一种维护负担较低的长期知识规则：ADR 放在负责的 package 附近，有 Decision Owner；替换用新 ADR 和 `Supersedes`／`Superseded by` 链接，合入后文件名保持稳定；CI 校验结构、位置、Owner 与引用。实际例子是 [ADR-20260902-steps-declare-waits.md](https://github.com/n8n-io/n8n/blob/7424f0ca0452f9dc9fe4cf6627ecbf0b315ed1e3/packages/%40n8n/engine/docs/adr/ADR-20260902-steps-declare-waits.md)。ADR 不承担任务进度台账的作用。

**判断与限制：** 模块地图、执行计划、决策历史各有位置，这比用一种 Spec 包办所有生命周期更清晰。但本轮没有证明 n8n 存在统一的全产品 Feature → Owner → 测试入口地图，也没有测量其 Skills 的实际使用频率。

## 对小团队最有用的组合

以下为本研究的适配建议，不是这些项目共同公布的一套标准。样本统称“高星公开仓库”：n8n 使用 fair-code 许可，Dify 的许可包含 Apache 2.0 之外的附加条款，不能用一个“开源”标签替代逐项目许可检查。

| 可借鉴的做法 | 最明确的样本 | 给我们解决的问题 |
| --- | --- | --- |
| 当前契约、决策与历史分开；活跃任务另存 | OpenCode `specs/v2/README.md` | 一次交付完成后，不要求它继续充当当前业务事实 |
| 上下文按需加载，控制篇幅，经 PR 修正失准内容 | Supabase `ask-the-docs` | 减少地图陈旧、整库灌入上下文和重复维护 |
| Skill 只指向已有权威规则 | Dify `frontend-testing` | 不同时维护 Skill 中一套规则、文档中另一套规则 |
| 功能身份连接验证场景，导航视图自动生成 | OpenClaw `taxonomy.yaml` 与 `claw-score` | 自动聚合功能／文档／覆盖关系，避免手工复制目录 |
| ADR 就近存放、Owner 明确、稳定链接及替代链 | n8n ADR 约定 | 长期保留决策原因，又不把日常任务状态塞进地图 |

适合第一版的地图不需要复制上述任何完整框架：保留一个领域／功能索引，每个重点功能只写跨文件难以还原的概念、边界和规则，再连接权威接口／代码、责任人与验证入口。数据结构、API 精确类型和现有测试不在地图中重写。索引能从既有元数据生成时再生成；没有现成元数据时，先维护少量稳定链接，不为了自动化新增一整套平行清单。

Skill 与知识地图可以分开：地图回答“项目现在怎样工作”，Skill 回答“做某种工作时先读哪里、按什么步骤验证”。这些样本既有 Supabase 的相关 Skill 必须加载，也有面向专项任务的能力；证据支持按协作收益决定项目约定，不支持声称所有团队都采用统一或全部可选的 Skill 政策。

一次交付的 Spec／Issue 表达本次意图，Plan／任务状态承载执行过程；长期地图保存当前契约和不易从代码恢复的知识。Review 以已认可的本次变更意图、现有契约、实际代码和验证证据为依据；个人 Plan 可以辅助理解，不能自行改写验收标准，也不要求每次审查都存在 Plan。普通 Patch 保持轻量；只有跨会话长程任务才需要用户选定的 LoopX 推进。这是结合当前团队目标的建议，并非由上述仓库推导出的统一工具要求。

本轮确认的是官方文件、引用与位置，未验证成员日常采用率，也未实际运行文档生成、Skills 或 CI。因此“存在维护约定”“存在已提交地图”“产生了本次可复现证据”应继续分开表述。
