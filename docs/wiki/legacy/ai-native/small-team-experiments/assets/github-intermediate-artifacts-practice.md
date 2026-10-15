# GitHub 实际项目如何保存 Spec、Plan、Task 与验收证据

核查日期：2026-10-11。只使用项目官方仓库、公开 Issue 和 PR。OpenSpec、Superpowers 的实例经 GitHub Git Tree API 确认存在于固定提交，随后读取正文；Kubernetes、Rust 样本由主研究会话读取官方分支文件和贡献说明，未固定 SHA。排除仅作模板或测试 fixture 的路径。工具规则与实际留存实例分别陈述，不把文档中的“已通过”声明当成本次重新执行的验证。

## 核查结论

开源项目确实会把 Spec、设计、Plan、任务清单，甚至评估结果提交 Git；也会把需求与任务放在 Issue，把验收摘要放在 PR。这里不是“中间文件是否一律入 Git”的二选一，而是不同信息各有权威位置。公开样本可以证明某种做法确实存在，不能代表所有 GitHub 项目的比例，更不能推断某个项目从未采用其它方式。

先区分三种记录：**验证计划**说明准备测什么，**完成声明**表示作者记录已完成，**执行证据**提供实际运行结果及其环境／版本依据。三者可能出现在同一个文档，也可能分放 Git、PR、CI 或制品库，不能互相替代。

## Matt 的 Spec 由谁生成

按本轮固定的官方 Skills 提交 `49dd158d1076134a641b33efb035946536778336`：

- `grill-with-docs` 调用 `grilling` 与 `domain-modeling`，通过访谈澄清设计，并在过程中形成术语表和 ADR；它没有把整场讨论合成 Spec 的职责。[grill-with-docs 源码](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/skills/engineering/grill-with-docs/SKILL.md)
- `to-spec` 才负责将已有讨论和代码理解整理成完整 Spec，并发布到项目 Tracker。它仍会确认测试边界，但明确不重新进行需求访谈。[to-spec 源码](https://github.com/mattpocock/skills/blob/49dd158d1076134a641b33efb035946536778336/skills/engineering/to-spec/SKILL.md)

所以“前者产出决策和长期知识，后者整理成某次交付的 Spec”更准确。不能因为前者名字中有 docs，就把所有文档生成职责都归给它。

## 普通开源项目：正式设计与验证计划也会进入 Git

Kubernetes 的真实 Sidecar Containers KEP 文件保存在 `kubernetes/enhancements` 仓库，正文包括设计、测试计划、阶段准入条件、生产就绪评审，以及带勾选的发布签核清单。它直接证明设计、验证安排与发布条件会作为 Git 文件维护；勾选仍不等于本研究独立重跑验证。[实际 KEP 753](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/753-sidecar-containers/README.md)

Rust RFC 仓库则区分普通修复／文档 PR 与重要变更的 RFC 流程。实际 [RFC 3678](https://github.com/rust-lang/rfcs/blob/master/text/3678-final.md) 是被仓库保存的设计产物，正文连接 RFC PR 与实现跟踪 Issue。这说明设计留在 Git、执行跟踪留在 Issue 可以并存，也不要求所有小修复先增加一套 RFC 文件。[Rust RFC 流程](https://github.com/rust-lang/rfcs)

这两项由主研究会话直接核对官方正文；以上为分支链接，未在本轮固定 Git SHA，未来文件可能变化。下述 OpenSpec 与 Superpowers 实例另提供固定提交链接。

## OpenSpec：设计、任务与规格一起留在 Git

固定提交：[`9111a7654d7800391459431fff4eaf66e33a3d2e`](https://github.com/Fission-AI/OpenSpec/commit/9111a7654d7800391459431fff4eaf66e33a3d2e)，2026-10-06。

实际变更 `add-dsh-support` 为 OpenSpec 增加 DeepSeek Harness 支持。它的归档目录已被 Git 跟踪，包含以下真实交付文件：[归档目录](https://github.com/Fission-AI/OpenSpec/tree/9111a7654d7800391459431fff4eaf66e33a3d2e/openspec/changes/archive/2026-08-15-add-dsh-support)

| 文件 | 正文实际内容 |
| --- | --- |
| `proposal.md` | 问题、支持范围、受影响能力和代码／文档／测试位置 |
| `design.md` | 方案决策、替代方案、风险与迁移／撤销安排 |
| `tasks.md` | 元数据、检测、生成、文档及验证的分组任务，保留完成勾选 |
| `specs/ai-tool-paths/spec.md` | 对工具路径要求的变更及具体场景 |

其 `tasks.md` 记录 lint、build、针对性测试、完整测试及临时项目烟测等验证项。它证明任务与验证完成声明被保存，不能单凭勾选证明原始日志永久可取、独立验收已发生或业务已经发布。[实际 tasks.md](https://github.com/Fission-AI/OpenSpec/blob/9111a7654d7800391459431fff4eaf66e33a3d2e/openspec/changes/archive/2026-08-15-add-dsh-support/tasks.md)

与此相符，OpenSpec 仓库的贡献规则要求新功能、重大重构或架构变更先以只含 `openspec/changes/<name>/` 的 PR 讨论与批准，再开始实现。这是该仓库的贡献流程，不能泛化成所有采用 OpenSpec 的项目都必须遵循的规则。[CONTRIBUTING.md](https://github.com/Fission-AI/OpenSpec/blob/9111a7654d7800391459431fff4eaf66e33a3d2e/CONTRIBUTING.md)

## Superpowers：Spec、Plan 与筛选后的评估结果都入 Git

固定提交：[`bb92a77741419a4ab5f06e711a283343f1ada0c3`](https://github.com/obra/superpowers/commit/bb92a77741419a4ab5f06e711a283343f1ada0c3)，2026-10-10。

工具约定方面，当前 `brainstorming` 默认把设计写到 `docs/superpowers/specs/` 并提交，用户偏好可覆盖；`writing-plans` 默认保存到 `docs/superpowers/plans/`。这是生成规则，本身不足以证明实际项目执行情况。[brainstorming](https://github.com/obra/superpowers/blob/bb92a77741419a4ab5f06e711a283343f1ada0c3/skills/brainstorming/SKILL.md)、[writing-plans](https://github.com/obra/superpowers/blob/bb92a77741419a4ab5f06e711a283343f1ada0c3/skills/writing-plans/SKILL.md)

实际仓库中，“SDD 工作区按计划隔离”这项改进同时保留了三份文件：

- [设计 Spec](https://github.com/obra/superpowers/blob/bb92a77741419a4ab5f06e711a283343f1ada0c3/docs/superpowers/specs/2026-07-06-sdd-plan-scoped-workspace.md)：记录跨计划进度冲突问题和拟采用的结构设计。
- [实施 Plan](https://github.com/obra/superpowers/blob/bb92a77741419a4ab5f06e711a283343f1ada0c3/docs/superpowers/plans/2026-07-06-sdd-plan-scoped-workspace.md)：含具体任务与验证顺序；明确先提交 RED 基线证据，再改 Skill，最终整理评估结果。
- [实际评估结果](https://github.com/obra/superpowers/blob/bb92a77741419a4ab5f06e711a283343f1ada0c3/docs/superpowers/specs/2026-07-06-sdd-plan-scoped-workspace-eval-results.md)：包含方法、场景、结果、限制以及实际使用的生成器／提示附录。它是该产品变更的评估报告，虽然附录包含测试 fixture 代码，整份文件并非仅供测试加载的 fixture。

这个实例同时区分了不同证据的生命周期：评估结论进入 Git，而原始回复、临时仓库等运行产物被引用到操作系统临时目录。报告也明确写出未复现原先假设的错误、工具调用次数没有下降，不能把它误读成已证明所有预期收益。共享结果可审查，并不意味着每份原始日志都随 Git 分发，或那些临时路径在别人的机器上可访问。[评估结果与限制](https://github.com/obra/superpowers/blob/bb92a77741419a4ab5f06e711a283343f1ada0c3/docs/superpowers/specs/2026-07-06-sdd-plan-scoped-workspace-eval-results.md)

## Matt Pocock：实际项目把 Spec、Ticket 和证据分放在不同位置

核查项目：`mattpocock/course-video-manager`。本轮 main 为 [`25d750bc454ab1bb1c3dc71ab38f445710c1ef1b`](https://github.com/mattpocock/course-video-manager/commit/25d750bc454ab1bb1c3dc71ab38f445710c1ef1b)，2026-10-10。Issue／PR 正文可被继续编辑，以下是本次读取的快照，不是与该 Git SHA 一同冻结的内容。

| 实际来源 | 可确认的做法 | 不能据此推出 |
| --- | --- | --- |
| [Issue #1207：移除课程的本地仓库依赖](https://github.com/mattpocock/course-video-manager/issues/1207) | 作者 Matt；正文说明由已完成的 Wayfinder 地图及九项确定决策合成为实现规格，包含用户故事、实现决策、测试决策与范围外事项 | 不能仅凭成文结构证明当时具体调用过哪个 Skill |
| [Issue #1219：重命名视频字段](https://github.com/mattpocock/course-video-manager/issues/1219) | 引用父 PRD #1207，列出交付行为、验收条件和阻塞票据 | closed 不自动等于每条验收项有独立完整证据 |
| [Issue #1495：流水线化 Dropbox 发布](https://github.com/mattpocock/course-video-manager/issues/1495) | 另一份完整 Spec 形态的真实 Issue，含实现与测试决策 | 不能说他的所有 Spec 都只保存于 Issue |
| [PR #1991：统一使用本地 Whisper](https://github.com/mattpocock/course-video-manager/pull/1991) | PR 描述包含 Evidence、实际验证环境和运行／测试摘要、影响范围与撤销说明；2026-10-10 已合并 | 不能把 PR 作者的实测声明当作本研究独立重跑，也不能由 PR 中有证据推出 Git 中没有证据 |

PR #1991 的代码 Head 为 `bcc30af149ca82983ff599c804252a8b3472d613`。其 Evidence 描述首次启动、重启及导出与转录同时执行的结果，并注明未覆盖的观察；设计决策则引用 ADR 0033。这说明真实交付可同时使用 Git 中的长期决策与 PR 中的验收摘要，而不需要一律生成 `evidence.md`。[PR #1991](https://github.com/mattpocock/course-video-manager/pull/1991)

## 对当前团队的含义

以下为设计建议，与上述来源事实分开：

- 业务地图、术语、ADR、当前接口／行为文档入 Git；一次交付的 Spec／Patch 说明可在 Issue 或 Git 二选一保存，完成后留作交付历史，不要求持续充当当前业务事实。不要再复制一份正文到另一个位置，长期知识按需回写其权威文档。
- 小 Patch 可在现有变更文档或 Issue 中带一小段实施／验证清单，不因借鉴某工具而增加独立 Plan 或 Task 文件，也不需要 LoopX。
- 需要多人交接、跨仓依赖或较长周期时，Plan／Ticket 应有可共享、可追溯的位置；选 Git 或 Tracker 都可以，避免两边人工复制同一进度。只有长程任务才用 LoopX 推进。
- 验收证据可以分层保存：PR 保留候选版本、结论及链接；需要长期复查的评估报告可进 Git；体积大或随运行产生的日志／截图放 CI 或制品存储。无论位置如何，链接要可访问，并与所验收候选对应。
- 不从“没有 `evidence.md`”推断“没有证据”，也不从“存在勾选清单”推断“验收已经通过”。
