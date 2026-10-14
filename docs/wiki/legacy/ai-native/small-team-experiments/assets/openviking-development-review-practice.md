# OpenViking 项目自身如何保存开发文档并验收 PR

核查日期：2026-10-11。范围是 `volcengine/OpenViking` 自身的公开协作实践，不是 OpenViking 产品的记忆／RAG 功能，也不把它分发给用户的 Agent Skill 当成维护本仓库的统一流程。

本轮 main 固定为 [`4c9a755bff4d31fd9bd9a3be1a19a6054ef3cea5`](https://github.com/volcengine/OpenViking/commit/4c9a755bff4d31fd9bd9a3be1a19a6054ef3cea5)，提交时间 2026-10-10 14:46:50 UTC。使用完整 Git Tree、文件正文、两个已合并 PR 的文件清单／描述／Review／评论核查；没有安装、运行测试或下载演示压缩包。PR 描述和评论可继续编辑，不随该 SHA 冻结。

## 结论

OpenViking 没有采用“所有 Spec／Plan／Evidence 都提交”或“全部不提交”的一刀切方式。临时开发目录、诊断脚本和报告通常被忽略；需要共享的具体设计、分阶段方案、产品说明和精选演示证据也确实被 Git 跟踪。抽查的普通 Patch 使用 Issue、源码、回归测试、API 文档和 PR 验证说明完成交接，没有再提交一套独立 Spec／Plan。

公开规则把 Review 的输入分成几个部分：Issue／Discussion／PR 说明意图和兼容性，模块导航帮助定位代码与责任人，具体代码规则和历史故障约束审查，测试／CI／作者验证提供证据。不能把其中任何一项单独当作全部验收依据；真实 Review 留言也未公开维护者的完整推理过程。

## Git 中保留什么

固定版本的 [`.gitignore`](https://github.com/volcengine/OpenViking/blob/4c9a755bff4d31fd9bd9a3be1a19a6054ef3cea5/.gitignore) 忽略 `specs/`、`docs/superpowers/`、`.loopx/`、`.scratch/`、`test_scripts/`、`reports/`、`test-reports/`、`*.log`，也包括 `AGENTS.md`／`CLAUDE.md`。贡献指南要求临时复现、诊断、压力脚本放 `test_scripts/`。这是明确的目录管理选择；Git 忽略规则不会取消已有跟踪，也不能推出仓库其它位置不存在设计或证据。

实际被 Git 跟踪且已读取正文的正向实例：

| 文件 | 保存的内容 | 事实边界 |
| --- | --- | --- |
| [VikingBot 会话压缩改造方案](https://github.com/volcengine/OpenViking/blob/4c9a755bff4d31fd9bd9a3be1a19a6054ef3cea5/bot/docs/zh/design/vikingbot-openviking-context-plan.md) | 目标／非目标、现状差异、设计原则、需改文件、分阶段实施 | 证明具体 Plan 会留 Git，不证明每次变更都要求这种文件 |
| [反馈观测设计](https://github.com/volcengine/OpenViking/blob/4c9a755bff4d31fd9bd9a3be1a19a6054ef3cea5/bot/docs/zh/design/vikingbot-feedback-observability-design.md) | 设计与分阶段实现状态，明确区分当前能力和目标 | 正文仍标为 Revised Draft，不能视作全部功能已经验收 |
| [Codex 插件设计](https://github.com/volcengine/OpenViking/blob/4c9a755bff4d31fd9bd9a3be1a19a6054ef3cea5/examples/codex-memory-plugin/DESIGN.md) | 状态、并发、失败恢复及选择理由 | 这是某组件的技术设计，不是维护 OpenViking 的全局工作流 |
| [演示证据说明](https://github.com/volcengine/OpenViking/blob/4c9a755bff4d31fd9bd9a3be1a19a6054ef3cea5/examples/pi-experimental-context-management/demo-evidence/README.md)与[脱敏压缩包](https://github.com/volcengine/OpenViking/blob/4c9a755bff4d31fd9bd9a3be1a19a6054ef3cea5/examples/pi-experimental-context-management/demo-evidence/pi-ctxwin-demo.zip) | 演示环境、运行记录、复现入口和脱敏说明 | README 称包内保留 transcript、debug log、请求和归档；本轮确认压缩包是已跟踪文件，未解压或独立重跑 |

压缩包在固定提交下为 836,296 字节，Git blob `7110648c99e89cf8e35d9ae5a5f50cdc887b23f9`。这足以否定“Evidence 一律不进 Git”；不能进一步推断每个 PR 的原始日志都永久提交。关键词扫描未展示统一的根级 Spec／Plan／ADR 台账，但不据此断言其它名字、历史提交或内部协作中从未使用这些产物。

## 主例：普通 Patch #5555

[PR #5555：Python SDK 暴露 grep 遍历深度](https://github.com/volcengine/OpenViking/pull/5555)，2026-10-10 合并，最终 Head 为 `b35a78ff83d7d7af4fcd96d37536e2cff7d08202`。

- **意图依据**：关联 [Issue #5554](https://github.com/volcengine/OpenViking/issues/5554)。Python 客户端缺少已有服务端能力；文档写默认深度为 5，实际服务端默认是 10。Issue 要求保留 10 的兼容行为并纠正文档。新增可选参数还要保留既有位置参数与省略字段行为。
- **提交产物**：仅四个文件，包含 SDK 源码、回归测试及中英文检索 API 文档；没有独立的 Spec、Plan 或 Evidence 文件。
- **证据位置**：PR Testing 段记录修复前回归结果为四失败、二通过，修复后 SDK 148 项及兼容类六项通过，列出检查命令；明确未运行真实服务、模型请求或 GPU 实验，因为修改边界是 SDK 请求构造。
- **实际 Review**：[sponge225 的批准](https://github.com/volcengine/OpenViking/pull/5555#pullrequestreview-5454137591)，API 标记为 COLLABORATOR，批准正文为空，无行内评论。本轮能确认批准发生，不能补写其未公开的审查过程。

因此，这个实际例子支持“现有接口文档＋一条清楚的 Issue／PR 变更说明＋回归测试＋验证记录”即可承载一个小 Patch，而不是每次另造完整规格套件。上述测试结果是作者记录，本轮没有重新执行。

它也具体说明 Review 为什么不能机械地只按既有地图或文档判定：文档本身可能已经偏离运行契约，需要结合本次明确的兼容性意图、源码与回归场景判定哪个应改。

## 对照：较大 Feature #5730

[PR #5730：Studio 编辑启动配置并由 ROOT 重启](https://github.com/volcengine/OpenViking/pull/5730)，2026-10-10 合并，最终 Head 为 `bc68d609223d04bfa25b9f7771219a1f83c0d887`。

PR 没有关联 Issue。设计约束直接写在描述中，包括文件／表单编辑边界、环境引用保留、修订版本检查、原子保存与备份、ROOT 权限、重启支持范围及失败处理。实际提交 27 个文件：后端、前端、测试，以及八份 API／配置文档，没有独立 Spec／Plan 文件。

验证说明列出前端 49 项、后端 63 项、代码检查及生产构建，并声明未重跑全项目 TypeScript 与真实浏览器／部署验证。[唯一人类 Review](https://github.com/volcengine/OpenViking/pull/5730#pullrequestreview-5479378014) 来自 COLLABORATOR ZaynJarvis，说明按维护者请求批准，等待集成测试变绿再合并；没有行内评论。本轮未见其逐条对照某份设计文档的公开记录。

需要保留一个证据边界：PR Testing 绑定的是 `604ffb864`，查询到的最终 Head 是上述 `bc68d609…`，正文还保留 Draft 提醒。这说明**这段描述本身不足以证明最终 Head 的完整验证**，不等于断言维护者实际没有验证；要判定最终候选，应再核对对应 CI 运行及其它记录。本研究没有扩大为该 PR 的完整验收审计。

## 项目规则如何帮助 Review

[CONTRIBUTING.md](https://github.com/volcengine/OpenViking/blob/4c9a755bff4d31fd9bd9a3be1a19a6054ef3cea5/CONTRIBUTING.md) 对重大 API、持久化、异步队列、租户或跨模块变更要求先通过 Issue／Discussion 对齐，交代现状、提议、具体例子与兼容性；它也提供 Domain → Area → Paths → Maintainer 的导航表。后者帮助定位实现与责任人，不会自动替代本次需求说明。

贡献规则还要求作者说明真实执行路径、精确验证命令，区分实际复现与推断；使用 AI 不转移作者责任。它要求 CI 通过，同时明确 CI 不能替代作者验证和维护者审查。本轮核查的是公开规则，未验证保护分支如何强制执行每一项。

[`.pr_agent.toml`](https://github.com/volcengine/OpenViking/blob/4c9a755bff4d31fd9bd9a3be1a19a6054ef3cea5/.pr_agent.toml) 中有 Ticket 分析开关；详细规则使用触发条件、审查动作和原因组织，连接到实际历史缺陷，例如异步阻塞、队列重复处理、兼容性、检索排序、并发、回归测试和文档一致性。它说明项目把经验转成可重复使用的 Review 约束，不只是让 Reviewer 判断“是否按开发者 Plan 写完”。配置存在不证明机器人在每个 PR 上都实际运行；本轮实际 Review 小节仅据已读取的人类批准记录陈述。

## CI 证据在哪里

[PR 工作流](https://github.com/volcengine/OpenViking/blob/4c9a755bff4d31fd9bd9a3be1a19a6054ef3cea5/.github/workflows/pr.yml) 根据路径调度组件检查。[API 测试工作流](https://github.com/volcengine/OpenViking/blob/4c9a755bff4d31fd9bd9a3be1a19a6054ef3cea5/.github/workflows/api_test.yml) 生成 HTML 报告，并以 `always` 条件上传 API／CLI 报告及服务端日志为 GitHub Actions Artifact。有无模型相关 secrets 会影响覆盖范围。

这说明默认执行证据可以在 CI 制品中，PR 提供命令、摘要与限制；精选且有长期复查价值的证据则可能进入 Git。工作流名不能代替正文检查：本轮看到的 [`_test_full.yml`](https://github.com/volcengine/OpenViking/blob/4c9a755bff4d31fd9bd9a3be1a19a6054ef3cea5/.github/workflows/_test_full.yml) 主要安装／构建，不能因为名称含 Full 就说它已经执行所有测试。

## 对当前团队的建议

以下是适配建议，不声称 OpenViking 已统一采用我们的设计：

1. 业务地图主要负责入口、模块边界、责任人和既有契约；本次变化的意图与验收条件放唯一的 Issue／Spec／Patch 说明。Reviewer 同时使用两者，再检查代码、测试和证据。
2. 普通 Patch 不必另建独立 Spec／Plan／Evidence 文件，也不用 LoopX；把问题、行为差异、回归场景、执行命令与结果写清楚即可。
3. 跨模块或复杂工作先对齐设计，是否形成独立 Plan 由交接需要决定。长程任务才交给 LoopX，执行状态不要再人工同步到多个台账。
4. 按用途区分长期共享资产与临时过程材料，而不是按文件名禁止 Plan 或 Evidence。验收摘要绑定候选版本，原始日志和报告使用可访问、具备合适保留期的制品位置。
