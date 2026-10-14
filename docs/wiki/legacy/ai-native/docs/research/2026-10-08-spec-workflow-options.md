# 开源 Coding Harness / Workflow 选型核查

核查日期：2026-10-08。本文比较 OpenSpec、Spec Kit、BMAD 的规格与交付流程层，以及 Claude Code Harness、Symphony、OpenHands、DeepSeek Harness 的执行与调度能力。依据官方仓库、发布标签、文档和关键源码进行静态核查；没有安装、运行这些工具，也没有完成本机 Codex、团队多仓或生产接入试验。

## 结论与适用范围

| 候选 | 适合解决什么 | 对本工作区的建议 | 不能直接当作什么 |
| --- | --- | --- | --- |
| OpenSpec | 把需求、增量规格、方案与任务组织成可回读的变更产物；可选独立规划仓 | 优先作为可选规格层评估，先解决与 map-delivery 的唯一事实源映射 | 独立审批系统、可信执行隔离、远端合并与发布门禁 |
| Spec Kit | 规格驱动流程，以及带持久状态、shell、人工 gate 的 YAML 工作流运行器 | 若当前缺的是可运行的阶段协调器，值得独立小试点；无需先采用它的全部规格模板 | 自带身份认证、摘要绑定审批与权限沙箱的团队控制面 |
| BMAD | 按规模组合需求、架构、任务、实现、审查和复盘方法 | 团队缺完整 SDLC 时再评估整套；已有流程先借用审查与返工设计 | 安装后自动成立的独立 Reviewer 权限、不可跳过的合并门禁 |

这里的“建议”是结合现有仓库约束的判断，不是三者在本机的运行排名。下文分别标注实际脚本/运行器与给 Agent 的流程指令。

## 当前版本与来源范围

本次 GitHub API 读取时三个仓库均未归档；发布版与核读的默认分支分开记录，不能把 main 的每一项变化都算进最新发布版。

| 项目 | API 返回的最新正式 Release / 发布时间（UTC） | 核读 main 提交 | 许可证 |
| --- | --- | --- | --- |
| [Fission-AI/OpenSpec](https://github.com/Fission-AI/OpenSpec) | [v1.14.1](https://github.com/Fission-AI/OpenSpec/releases/tag/v1.14.1)，2026-10-06 | [`9111a7654d7800391459431fff4eaf66e33a3d2e`](https://github.com/Fission-AI/OpenSpec/commit/9111a7654d7800391459431fff4eaf66e33a3d2e) | [MIT](https://github.com/Fission-AI/OpenSpec/blob/9111a7654d7800391459431fff4eaf66e33a3d2e/LICENSE) |
| [github/spec-kit](https://github.com/github/spec-kit) | [v1.1.2](https://github.com/github/spec-kit/releases/tag/v1.1.2)，2026-10-07 | [`1e933c49fd6d5d5390b28f18faefa5728c95b2e3`](https://github.com/github/spec-kit/commit/1e933c49fd6d5d5390b28f18faefa5728c95b2e3) | [MIT](https://github.com/github/spec-kit/blob/1e933c49fd6d5d5390b28f18faefa5728c95b2e3/LICENSE) |
| [bmad-code-org/BMAD-METHOD](https://github.com/bmad-code-org/BMAD-METHOD) | [v6.12.1](https://github.com/bmad-code-org/BMAD-METHOD/releases/tag/v6.12.1)，2026-10-04 | [`bda3c5929f672019b90f39a3ea27259d35d00974`](https://github.com/bmad-code-org/BMAD-METHOD/commit/bda3c5929f672019b90f39a3ea27259d35d00974) | [MIT](https://github.com/bmad-code-org/BMAD-METHOD/blob/bda3c5929f672019b90f39a3ea27259d35d00974/LICENSE) |

已额外直接读取 Spec Kit **v1.1.2 标签**的 workflow 文档、engine.py 和 Codex 集成源码，确认运行器不是仅存在于 main 的待发布功能。BMAD 的安装说明确实有版本差异：v6.12.1 README 使用 `npx bmad-method install`；核读 main README 已改成 Skills CLI 或 Claude Code/Codex 插件，再运行 `bmad setup`。若试点应先固定标签，再读该标签文档，不能混用。[Spec Kit 发布版工作流](https://github.com/github/spec-kit/blob/v1.1.2/docs/reference/workflows.md)、[BMAD 发布版 README](https://github.com/bmad-code-org/BMAD-METHOD/blob/v6.12.1/README.md)、[BMAD 核读 main README](https://github.com/bmad-code-org/BMAD-METHOD/blob/bda3c5929f672019b90f39a3ea27259d35d00974/README.md)。

## OpenSpec：规格与增量变更层

典型路径是 explore → propose → apply → archive；propose 创建 proposal、增量 specs、design、tasks，后续把当前行为留在 `openspec/specs/`，活动与归档变更留在 `openspec/changes/`。它允许迭代修订，不要求把每次小改动都变成完整瀑布流程。[README](https://github.com/Fission-AI/OpenSpec/blob/9111a7654d7800391459431fff4eaf66e33a3d2e/README.md)。

Codex 适配生成 `.agents/skills/openspec-*/SKILL.md`，使用 `$openspec-*` 技能入口；Claude Code、Cursor、Copilot、Gemini 等有各自适配。宿主支持在此意味着生成对应技能/命令文件，不能推导为每个桌面环境都已运行验证。[Supported Tools](https://github.com/Fission-AI/OpenSpec/blob/9111a7654d7800391459431fff4eaf66e33a3d2e/docs/supported-tools.md)。

持久化由仓库中的 Markdown/YAML 产物承担。源码 `detectCompleted` 通过产物文件是否存在判断 artifact completed；这不是内容已审查或实现已验证。`verify` 技能明确为 advisory；archive 有真正的规格校验逻辑，但也有 `--no-validate` 和确认路径，因此不能拿 archive 成功充当不可绕过的质量批准。[状态检测](https://github.com/Fission-AI/OpenSpec/blob/9111a7654d7800391459431fff4eaf66e33a3d2e/src/core/artifact-graph/state.ts)、[verify 模板](https://github.com/Fission-AI/OpenSpec/blob/9111a7654d7800391459431fff4eaf66e33a3d2e/src/core/templates/workflows/verify-change.ts)、[archive 实现](https://github.com/Fission-AI/OpenSpec/blob/9111a7654d7800391459431fff4eaf66e33a3d2e/src/core/archive.ts)。

Stores beta 很贴近独立研发文档仓：规划可放单独 Git 仓，业务仓通过 store 指针或只读 references 访问。它不自动 clone/pull/push，也不把一个任务清单自动分派到多个代码仓；本地 checkout 过旧就会读到旧规格。store registry/worksets 是每台机器配置，格式仍可能变化。[Stores User Guide](https://github.com/Fission-AI/OpenSpec/blob/9111a7654d7800391459431fff4eaf66e33a3d2e/docs/stores-beta/user-guide.md)。

本地判断：它最有价值的是统一可回读的规格和变更结构。我们必须先选择唯一权威，才能采用；不能同时保留 `map/.../spec.md` 和 `openspec/specs/...` 两份可独立修改的现行规格，也不能把两套 Change 状态并排运行。

## Spec Kit：规格流程与实际工作流运行器

当前核心 SDD 路径是 constitution（项目级）→ specify → plan → tasks → implement → converge；clarify/checklist/analyze 按需启用。bug 的 assess/fix/test 与 idea assessment 是可选扩展入口，不是每次 SDD 必经阶段。[当前 README](https://github.com/github/spec-kit/blob/1e933c49fd6d5d5390b28f18faefa5728c95b2e3/README.md)。

Codex 集成默认使用 `.agents/skills/speckit-*/SKILL.md`；v1.1.2 的 `build_exec_args` 构造 `codex exec <prompt>`，可加 model 和 JSON 输出，因而存在非交互 CLI 调用适配。多宿主集成另有安装清单与默认集成状态；这不是自动证明所有宿主或 Codex 桌面运行器均兼容。[发布版 Codex 源码](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/integrations/codex/__init__.py)、[集成管理](https://github.github.io/spec-kit/reference/integrations.html)。

必须分开看两条执行路径：

| 路径 | 能确认的检查与状态 | 门禁边界 |
| --- | --- | --- |
| 在 Agent 聊天中逐个运行技能 | `check-prerequisites` 是可执行脚本，缺所需目录、plan、tasks 等时退出失败；文档和任务清单可持久回读 | implement 中的清单检查、TDD、完成判定和调用 hooks 仍有提示词部分；未勾清单允许询问用户后继续。不是服务器审批或身份验证 |
| `specify workflow run/resume/status` | YAML 运行器执行 command/prompt/shell/gate、条件与循环、fan-out/fan-in；运行记录位于 `.specify/workflows/runs/<run_id>/` | gate 能控制该次运行的推进，但没有权限沙箱；运行器成功不证明业务验收、真人批准或远端合并 |

第一行依据[前置检查脚本](https://github.com/github/spec-kit/blob/1e933c49fd6d5d5390b28f18faefa5728c95b2e3/scripts/bash/check-prerequisites.sh)与[implement 模板](https://github.com/github/spec-kit/blob/1e933c49fd6d5d5390b28f18faefa5728c95b2e3/templates/commands/implement.md)；第二行依据[v1.1.2 工作流文档](https://github.com/github/spec-kit/blob/v1.1.2/docs/reference/workflows.md)与[运行器源码](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/engine.py)。

**运行器的真实批准边界**：gate 的 `verdict_input` 可由工作流输入供给；匹配选项的非空值自动作出判断，非空 default 也会在首次运行时被消费。输入为空时，TTY 询问，非 TTY 暂停；随后可 `resume --input spec_verdict=approve`。因此可以无人值守运行已配置的流程，但“谁能给运行器传值”与“谁有业务批准权”需要项目另行绑定。普通 gate 也不校验下一步命令或自动绑定被审产物摘要。[发布版 Gate Verdict Inputs 与 Interpolation and Shell Safety](https://github.com/github/spec-kit/blob/v1.1.2/docs/reference/workflows.md#gate-verdict-inputs)。

`state.json` 保存运行与步骤状态；核读 engine 包含锁内临时文件加 `os.replace` 的状态写入，不只是让 Agent 改 Markdown。与此同时，官方明确 shell 使用调用者权限，`requires` 是声明性前提而非 capability gate。不能把输入型批准、进程状态锁或 JSON 记录称为独立身份、产物授权或安全隔离。[发布版 engine](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/engine.py)、[发布版工作流边界](https://github.com/github/spec-kit/blob/v1.1.2/docs/reference/workflows.md#step-types)。

本地判断：若目标是把“产出 → 验证 → 审查 → 返工/推进”变成可运行流水线，Spec Kit 的运行器值得和自己维护薄协调器进行比较。可先只评估本地 YAML 编排能力，不能因采用运行器就顺带生成第二套规格。未核实它开箱即用地满足 Gitee 多仓审批、我们的 exact_head/planDigest 或受影响下游自动失效规则。

## BMAD：完整方法库与按规模裁剪

核读 main 的小变更入口为 `bmad-build`；较大目标先 `bmad-spec`，再由 `bmad-ticket` 拆成有顺序的 stories，每个 story 一次 Build，最后做 epic 复盘。PRD、UX、架构等是按缺口选用的工具，不必都执行。产物默认放 `_bmad-output` 下，ticket 树和加入其中的 plans 保存工作安排及构建状态。[Planning Path](https://github.com/bmad-code-org/BMAD-METHOD/blob/bda3c5929f672019b90f39a3ea27259d35d00974/docs/plan/choose-a-planning-path.md)。

Build 的 plan frontmatter 包含 `baseline_revision`、draft/ready-for-dev/in-progress/in-review/built/done 等状态、risk、review、lenses_ran、review_loop_iteration。Review 流程要求独立子代理视角；无子代理能力时把材料交给单独会话并暂停。findings 按 intent_gap/bad_plan/patch/defer 路由；审查回退计数超过 5 时停止并交人处理。这些规则具有可借鉴性，但主要由 Agent 读取并执行步骤文件，未发现它们自动成为我们仓库的不可绕过权限门禁。[计划模板](https://github.com/bmad-code-org/BMAD-METHOD/blob/bda3c5929f672019b90f39a3ea27259d35d00974/skills/bmad-build/plan-template.md)、[Review 步骤](https://github.com/bmad-code-org/BMAD-METHOD/blob/bda3c5929f672019b90f39a3ea27259d35d00974/skills/bmad-build/step-04-review.md)、[Workflow 指令](https://github.com/bmad-code-org/BMAD-METHOD/blob/bda3c5929f672019b90f39a3ea27259d35d00974/skills/bmad-build/workflow.md)。

组织级文档明确 PRD、架构、epic 有各自负责人，现有 tracker 和审查机制继续保留；下游发现需求缺口要改回来源，再更新受影响产物。它的职责与返工设计比“装几个角色提示词”具体；但角色名称、多个技能或子代理调用本身仍不能证明独立身份与只读隔离。[Plan Inside an Organization](https://github.com/bmad-code-org/BMAD-METHOD/blob/bda3c5929f672019b90f39a3ea27259d35d00974/docs/plan/plan-inside-an-organization.md)。

本地判断：已有“业务地图、正向 delivery、反向 review”的团队可以抽取检查项和返工分类，暂不引入整套 PRD/spec/tickets 状态。若采用整套，必须重新确定每种文档和状态的权威位置；“导入现有文档”不自动解决双份规格漂移。main 的 Codex 插件入口已可在官方 README 确认；本次未验证 v6.12.1 安装器在当前 Codex CLI/桌面的具体安装结果。

## 与现有 map-delivery、独立文档仓和十人团队衔接

当前 [AGENTS.md](../../AGENTS.md) 指定 `skills/map-delivery` 为 canonical；业务规格与交付记录留在研发文档仓。[现有 workflow](../../skills/map-delivery/references/workflow.md) 已规定唯一主 Change、跨模块引用而不复制正文、逐仓完整 base/exact_head、Review/Evidence，以及 approved/implementing/verifying/effective 的区别。[SDLC 原则](../SDLC/00_目标与设计原则.md) 已有业务地图/delivery/review 分工。因此下面是兼容性建议，不是本次迁移：

1. **先定唯一权威。** 保持现有 map 文档体系时，新运行器只引用它；若决定迁移 OpenSpec 规格/变更层，就同时迁移旧写入口与校验，并把 map-delivery 缩为导航及项目治理衔接。不要维护两个可独立推进的 Change。
2. **协调状态和交付状态分开。** runner 的 completed、BMAD 的 built、OpenSpec 的 archived 只说明各自流程动作；effective、remote merged、deployed 必须由项目策略读取真实证据后判定。
3. **跨仓证据保持精确。** 任务引用文档仓 identity/提交/路径/摘要，每个代码仓记录 base/exact_head、验证和独立审查。Spec Kit run_id 只作执行证据索引，不取代 Delivery Issue 或 Change ID。
4. **权限放在可执行边界。** 协调器可以调用现有验证器和产品 `make verify-fast/pr/integration/release`；服务器 CI、保护分支和平台授权负责真实合并/外部写入。Agent 不能通过修改流程文件、清单或传入 approve 自证获得批准。
5. **职责按工作划分。** 领域负责人维护事实来源；交付负责人拥有 Change 和跨仓依赖；实现者交出候选与验证；独立 Reviewer 交绑定候选的 findings；工具维护者维护固定版本和门禁。十个人不必对应十个 Agent 或十种角色。

## 最小评估与未确认项

试点建议分两步：先用单代码仓低风险变更核查执行链，再扩展到“一份文档仓、两个代码仓、一条小需求”，一次只试一个候选。要求第二个新会话能找到准确来源和未完成工作；缺依赖、失败验证、需求/代码变化使旧审查失效；拒绝后能返回修复并在预算内停止；所有跨仓结果仍可对账到实际 Head。再比较人工工作、等待、返工和故障介入，不用星数或流程文件数量判断质量。

本次能确认的是公开源码、文档、发布标签和上述静态实现边界；不能确认本机安装成功、桌面实际加载、模型执行可靠性、十人并发正确性、远端 Gitee 接入、项目不可绕过门禁或提效数字。没有把先前 harness-minimal PoC 的测试结果转移给这些第三方项目，也没有运行新的 Agent A/B。

## 执行 Harness、任务调度与平台候选

以下为 2026-10-08 对官方仓库文档、宿主登记文件和公开 GitHub 元数据的核查；没有安装候选项目、运行其测试，或接入团队账号。支持等级是项目声明，不能替代本工作区验收。

| 项目 | 所属层与可复用机制 | 已核实边界 | 许可证 |
| --- | --- | --- | --- |
| [Claude Code Harness](https://github.com/Chachamaru127/claude-code-harness) | plan/work/review/sync/release；规格、计划、复核、有限修复循环；Go 检查入口和宿主 hooks。 | README 把 Codex CLI 列为 supported，Codex app 列为 candidate；[宿主登记](https://github.com/Chachamaru127/claude-code-harness/blob/2b2b74805321089bd9b660a1064fa97556299703/hosts/registry.json)将 Codex 的安全模型记为 pre_use_bash_plus_post_gate。不能承诺不同工具入口等强度，也未验证本机。 | MIT |
| [OpenAI Symphony](https://github.com/openai/symphony) | 持续读取任务、按 Issue 创建工作区、启动 Agent；配置并发、重试与状态协调；仓库内 WORKFLOW.md 保存配置及提示。 | [参考实现](https://github.com/openai/symphony/blob/be10a1b79df723d6d7612b5651c8522704dafb2e/elixir/README.md)自称 evaluation prototype；内置 Linear、GitHub Issues、Jira Cloud、Asana、GitLab，没有列出 Gitee。它调度任务，不自动认证需求或代码正确性。 | Apache-2.0 |
| [OpenHands Agent Canvas](https://github.com/OpenHands/OpenHands) | 当前主仓负责自托管控制界面和后端连接；支持多种 Agent，提供定时/事件触发自动化。 | 当前体系分为 Canvas、software-agent-sdk、automation 等仓库；[预置自动化](https://docs.openhands.dev/openhands/usage/agent-canvas/prebuilt-automations)提供 Issue→PR、PR Review 等起点。团队阶段验收和跨仓发布策略仍需配置/集成，不能由“支持自动化”推导为已具备。 | 本次核查 Canvas 主仓为 MIT；其他组件分别核查 |
| [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) | 插件化 Agent Harness，适合需要定制执行宿主和插件接口时研究。 | [README](https://github.com/deepseek-ai/deepseek-harness/blob/5badb15009ae1756c3afe0ae0cef1faafc290ccc/README.md)标明 developer preview 和兼容性破坏；[SAFETY](https://github.com/deepseek-ai/deepseek-harness/blob/master/SAFETY.md)明确未经安全审计、不能当作生产就绪。不是本工作区阶段治理的首选补丁。 | MIT |

Symphony 的 [SPEC](https://github.com/openai/symphony/blob/main/SPEC.md)明确不定位为通用工作流引擎；重启依靠任务平台和文件系统重新协调，不恢复完全相同的内存状态。评估常驻自动执行时，需另验中途退出、等待人类输入、重复派发、远端写入幂等性与接管，而不能把“能重新启动”理解为精确断点恢复。

CCH 的旧 hardening 文档与新宿主登记在部分宿主上存在日期/表述差异，本次只采用 README 与 registry 相符的 Codex CLI 支持及受限拦截声明。没有将其他宿主的支持或拦截能力类推给当前桌面环境。

### 源码定位快照

这些是核查时 GitHub API 返回的默认分支 Head。已成功按对应完整 SHA 获取下列文档；它们用于定位此次文档观察，不是已安装或已通过验证的版本。

| 仓库 | Head | 本轮读取的固定版本文件 |
| --- | --- | --- |
| Chachamaru127/claude-code-harness | 2b2b74805321089bd9b660a1064fa97556299703 | README.md、hosts/registry.json |
| openai/symphony | be10a1b79df723d6d7612b5651c8522704dafb2e | elixir/README.md |
| OpenHands/OpenHands | bf92bcb6b337ca8d8f109aef0acaade447774b35 | README.md |
| deepseek-ai/deepseek-harness | 5badb15009ae1756c3afe0ae0cef1faafc290ccc | README.md |

### 对当前工作区的建议

本节为设计判断。当前 [ai-native README](../../README.md)与[仓库约定](../../AGENTS.md)已经明确：团队共享能力由 ai-native 分发，业务规格和交付记录留在文档仓，产品仓提供自己的验证与合并约束。

若重点是可执行的阶段控制，优先在单仓低风险变更上评估 Spec Kit v1.1.2 的自定义 workflow runner：让 prompt/shell 步骤引用现有业务文档与验证命令，先核查 Codex 实际调用、暂停恢复和失败阻塞，再决定是否接入。其 gate 的输入放行不能替代独立审查身份或产物摘要绑定；这些仍由团队回执与 CI 验证承担。若重点是现成的 Agent 执行闭环，隔离试用 CCH。若只缺轻量规格及变更管理，优先 OpenSpec。需要规格工具时，OpenSpec、Spec Kit、BMAD 应择一承担相应规格/规划责任，不同时维护彼此冲突的需求、计划与完成状态。若选 OpenSpec，map-delivery 引用其规格和变更，团队 Review/Evidence 继续记录真实审查与验证，不再复制一份最新规格。

试点用同一任务检验：需求缺项是否阻止进入实现；Review 失败是否确实退回修复；新提交是否使旧审查失效；中断后能否恢复准确状态；当前宿主所有相关入口是否受预期约束；CI 是否针对准备合并的完整版本拒绝缺证据。现有 harness-minimal 只是此前验证过的局部门禁样例，不因本次选型研究获得新的集成能力。

等阶段合同和项目验证稳定后，再评估 Symphony 自动接单。若目标同时包括自托管工作界面、多 Agent 后端和常驻自动化，再比较 OpenHands Canvas。Gitee 以及多个仓库共同完成一个交付的身份绑定、依赖顺序、验收与发布，需要单独验证；本次没有发现足以证明这些在当前团队已开箱可用的证据。
