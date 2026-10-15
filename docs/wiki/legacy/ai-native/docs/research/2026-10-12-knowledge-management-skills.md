# 面向 AI 上下文的知识库管理 Skill 调研

核查日期：2026-10-12（Asia/Shanghai）。目标是寻找可借鉴的项目知识组织、维护与校验机制，对照 mindInsight 的 specs、changelog、architecture、runbook、vendor、wiki 六类职责。本文依据公开第一方仓库的 SKILL.md、引用文件和部分脚本静态核查；没有安装或运行第三方 Skill，也没有验证它们在当前 Codex、多仓环境或候选文档工作流中的实际效果。

选型状态澄清（2026-10-12）：用户明确项目尚未采用 OpenSpec，它只是待定方案；当前考虑以 r-doc 为基础试点、测试和优化，尚未确定正式采用。本文此前将 OpenSpec 写成既定权威的建议已纠正。应保留的是“唯一规格事实源”原则，具体工具及目录须由试点和后续决策确定。

后续验证：本轮静态调研之后完成了 [r-doc 首轮隔离试点](2026-10-12-r-doc-pilot.md)，包含脚本回归、两项候选改进和真实 Agent 配对样例；运行结论与限制见该报告。

## 核心判断

现有候选提供了相近的组成部分。Microsoft Deep Wiki 最接近分工明确的知识生成套件；Every 的 ce-compound/ce-compound-refresh 更适合已验证经验的沉淀与更新；`r-doc` 更接近“AGENTS.md → 索引 → 专题文档”的治理层；Microsoft HVE documentation 更接近文档审计与代码文档漂移检查。没有一套在本次调查中被验证可以原样替代六个 collar Skill。r-doc 值得作为局部试点候选，验证导航、引用、查重、更新和检查机制；规格管理方案仍待确定，试点不应产生并行的业务事实源。

## 来源快照

GitHub API 本次直接读取返回如下；star 是读取时的动态数值，仅帮助辨认项目，不代表能力或效果。skills.sh 安装数、star 展示与搜索摘要存在不同步，因此不作为选型依据。

| 项目 | 默认分支 HEAD | Star | 许可证 |
| --- | --- | --- | --- |
| [microsoft/hve-core](https://github.com/microsoft/hve-core) | [49dfb73866f8c5f81bf734d6ca808dac25a2a330](https://github.com/microsoft/hve-core/commit/49dfb73866f8c5f81bf734d6ca808dac25a2a330) | 1519 | MIT |
| [riesaexe/r-doc](https://github.com/riesaexe/r-doc) | [bc23420d7df5ec1d2c5fb45e066c919790ab9cf9](https://github.com/riesaexe/r-doc/commit/bc23420d7df5ec1d2c5fb45e066c919790ab9cf9) | 36 | MIT |

来源：[HVE API](https://api.github.com/repos/microsoft/hve-core)、[r-doc API](https://api.github.com/repos/riesaexe/r-doc)。这两项的正文链接固定到读取默认分支时记录的提交；后续补充候选链接使用本次读取的 main 路径，实际试点前应另行锁定版本。

## r-doc：最接近知识库入口、导航和生命周期治理

实际入口：[skills/r-doc/SKILL.md](https://github.com/riesaexe/r-doc/blob/bc23420d7df5ec1d2c5fb45e066c919790ab9cf9/skills/r-doc/SKILL.md)。

它将根 `AGENTS.md` 作为项目入口，按任务读取文档索引、专题索引、目标文档及其必要链接。支持 minimal、standard、strict 三档，保留合理的既有结构；不要求一次性生成固定六目录或批量空模板。standard/strict 强调索引可导航、主题单一、变更时同步入口。可以借鉴为 mindInsight 式父仓的“知识路由规则”，而不是一个只负责润色文字的写作 Skill。[入口及流程](https://github.com/riesaexe/r-doc/blob/bc23420d7df5ec1d2c5fb45e066c919790ab9cf9/skills/r-doc/SKILL.md)。

其可借鉴的具体机制：

- 文档区分 `draft → proposed → active → superseded → archived`；ID、owner、review_after、related_docs、related_code、planned_code、supersedes 描述维护责任和关系。已被替代的内容必须能够追到后继文档，避免过期规则继续进入上下文。[元数据与状态](https://github.com/riesaexe/r-doc/blob/bc23420d7df5ec1d2c5fb45e066c919790ab9cf9/skills/r-doc/references/metadata-schema.md)。
- 规划、设计、实现、评审、发布、维护分别有文档同步清单；变更不能仅因代码可运行就视为文档完成。旧决策被替代时保留历史与后继链接。[生命周期检查](https://github.com/riesaexe/r-doc/blob/bc23420d7df5ec1d2c5fb45e066c919790ab9cf9/skills/r-doc/references/lifecycle-checklists.md)。
- `audit_docs.py` 实际包含断链、锚点、索引、ID、日期、状态、关联路径和替代关系等机械校验；可配置 docs_root、必需文档类型、关系要求与阶段门禁。`repair_docs.py` 提供预览及显式应用的结构修复流程。[审计源码](https://github.com/riesaexe/r-doc/blob/bc23420d7df5ec1d2c5fb45e066c919790ab9cf9/skills/r-doc/scripts/audit_docs.py)、[配置说明](https://github.com/riesaexe/r-doc/blob/bc23420d7df5ec1d2c5fb45e066c919790ab9cf9/skills/r-doc/references/project-config.md)、[验证说明](https://github.com/riesaexe/r-doc/blob/bc23420d7df5ec1d2c5fb45e066c919790ab9cf9/skills/r-doc/references/verification.md)。
- 可选 decision notes 解释“为什么这样决策”，当前事实仍在 docs；不把每次操作都写成笔记。新决策反转旧决策时保留 supersedes 关系。这种区分有助于保留推理而不让历史方案冒充现行规范。[决策笔记](https://github.com/riesaexe/r-doc/blob/bc23420d7df5ec1d2c5fb45e066c919790ab9cf9/skills/r-doc/references/decision-notes.md)。

**限制与本项目适配判断：** `related_code` 的存在检查不能证明文档描述与代码行为一致，结构校验通过也不代表需求获批或验收完成。可在限定范围内试验其需求、设计、入口路由、索引完整性、过期/替代关系和冲突报告能力，不能仅因 OpenSpec 也在候选名单中就排除这些能力。若未来同时采用 OpenSpec，两者的文档类型、元数据和 decision notes 生命周期存在职责重叠，需要先定义唯一规格事实源及状态归属，避免维护两套同义的 requirements/design/decision 状态。本次尚未验证 r-doc 的原生 OpenSpec 集成，也不以该集成为试点前提。

## Microsoft HVE documentation：最值得借鉴代码文档漂移与检查分工

实际入口：[.github/skills/hve-core/documentation/SKILL.md](https://github.com/microsoft/hve-core/blob/49dfb73866f8c5f81bf734d6ca808dac25a2a330/.github/skills/hve-core/documentation/SKILL.md)。

四种模式集中在一个 Skill 内，再由薄的 Documentation agent 按模式调用：audit 查覆盖与遗漏；drift 从变更分析文档是否失真；validate 执行已有检查；author 按模板写指南或参考文档。它明确不负责 ADR、BRD、PRD 等规划产物，因此与规格管理器的职责边界较容易分开。[Skill 入口](https://github.com/microsoft/hve-core/blob/49dfb73866f8c5f81bf734d6ca808dac25a2a330/.github/skills/hve-core/documentation/SKILL.md)、[调用 agent](https://github.com/microsoft/hve-core/blob/49dfb73866f8c5f81bf734d6ca808dac25a2a330/.github/agents/hve-core/documentation.agent.md)。

其可借鉴的具体机制：

- 把“代码路径变化 → 先检查哪个配对参考页 → 再检查哪些概念文档”写成映射表。Skill 的正文、references、scripts 都映射到同一个资产参考页；先关注行为、命令和路径事实，纯外观改动可跳过。对 IoT 可借鉴为“协议/网关/API/数据库变更 → 受影响规格、架构连接与操作说明”，但需项目自行建立映射。[代码文档映射](https://github.com/microsoft/hve-core/blob/49dfb73866f8c5f81bf734d6ca808dac25a2a330/.github/skills/hve-core/documentation/references/code-doc-mapping.md)。
- audit 先限定文档范围，再对照实现、命令和工作流查缺口；报告包含具体位置、观察到的问题、建议修复，避免泛泛的“补全所有文档”。[覆盖方法](https://github.com/microsoft/hve-core/blob/49dfb73866f8c5f81bf734d6ca808dac25a2a330/.github/skills/hve-core/documentation/references/coverage-method.md)。
- 区分本地检查、修复命令与 CI 检查；记录 Passed、Failed、Pending CI、Skipped、Deferred、Unavailable，未运行的 CI 项不得算通过。这一点能直接支持我们“证据跟随具体候选”的交付约定。[验证工具链](https://github.com/microsoft/hve-core/blob/49dfb73866f8c5f81bf734d6ca808dac25a2a330/.github/skills/hve-core/documentation/references/validation-toolchain.md)。
- 文档写作规则引用仓库已有的 Markdown、写作风格、Docusaurus 指令，避免在 agent 与 Skill 内复制规范。这里的 canonical 指该套文档能力的实现入口，不表示它替项目解决了全部业务事实权威问题。[约定入口](https://github.com/microsoft/hve-core/blob/49dfb73866f8c5f81bf734d6ca808dac25a2a330/.github/skills/hve-core/documentation/references/conventions.md)。

**限制与本项目适配判断：** 当前映射表针对 HVE 自身的 agents/prompts/skills/scripts；检查命令使用仓库的 npm 与 Docusaurus 工具链，不能复制后就声称适用于 Go/React/MQTT。drift 是 Agent 根据映射和启发式开展的检查，未看到“自动证明代码文档语义等价”的保证。它也不提供 mindInsight 所列的完整业务地图模型、runbook 分类和 vendor 同步机制。适合提取“影响映射 + 有边界的漂移检查 + 明确验证状态”作为我们知识维护能力；业务规格工具待定，长程状态仍按现有 LoopX 分工处理。

## 两者对六类职责的覆盖边界

| mindInsight 职责 | r-doc 可借鉴 | HVE documentation 可借鉴 | 仍需项目定义 |
| --- | --- | --- | --- |
| specs：要做什么 | 需求/设计文档、入口导航、状态与关系 | 文档覆盖和一致性审计 | 唯一规格事实源及变更模型；具体工具待试点与决策 |
| changelog：已做/将做 | 发布同步清单、变更追溯 | 检查现有命令/行为描述是否陈旧 | 已发布事实与未来计划的状态区分 |
| architecture：系统连接 | 设计文档关系、决策历史 | 按代码变更触发架构文档检查 | IoT 连接类型、跨仓版本与接口证据 |
| runbook：过程知识 | operations 类型、专题和生命周期 | 指南模板与准确性检查 | 事件/故障/部署等分类及事实来源 |
| vendor：外部源码 | 没有对应专门能力 | 没有对应专门能力 | 选择性同步、来源、版本和许可证记录 |
| wiki：人工知识 | 单主题、索引、维护责任 | 写作及链接验证 | 人工维护边界、是否允许 AI 自动修改 |

本表是依据上述源码作出的映射判断，不是两个项目作者宣称的六类兼容性。当前推荐级别均为“值得借鉴、可准备局部试点”，尚非“已验证可在本项目直接使用”。

## Microsoft Deep Wiki：最接近按职责拆分的知识库技能套件

[第一方目录与 README](https://github.com/microsoft/skills/tree/main/.github/plugins/deep-wiki)。其中 wiki-architect 先读实际代码并设计层次目录，wiki-page-writer 根据目录写带源码引用的页面，wiki-changelog 从 Git 历史提取已发生变化，wiki-agents-md 生成缺失的入口；另有 research、qa、onboarding、llms.txt 等技能。默认面向技术 wiki，可生成 wiki/ 及 VitePress 站点。

已直接读取：[wiki-architect](https://github.com/microsoft/skills/blob/main/.github/plugins/deep-wiki/skills/wiki-architect/SKILL.md)、[wiki-page-writer](https://github.com/microsoft/skills/blob/main/.github/plugins/deep-wiki/skills/wiki-page-writer/SKILL.md)、[wiki-changelog](https://github.com/microsoft/skills/blob/main/.github/plugins/deep-wiki/skills/wiki-changelog/SKILL.md)、[wiki-agents-md](https://github.com/microsoft/skills/blob/main/.github/plugins/deep-wiki/skills/wiki-agents-md/SKILL.md)。值得借鉴的是先规划目录、沿源码追踪事实、页面互链、按读者分层。父仓场景应将引用改为子仓 URL + 固定 SHA + 路径，避免 branch 引用随时间改变。

边界：原生 README 面向 Copilot CLI；单个 Markdown Skill 可移植，但完整插件编排未在 Codex 验证。AGENTS 生成器只创建缺失文件，跳过已有入口；未看到完整的增量更新和去重淘汰协议。其技术 wiki 不等同于文章的人工中文自由知识库。page-writer 的每页至少 3–5 张图、至少 5 个源码文件，以及完整 generate 的大篇幅 onboarding 规则可能过重，建议去掉固定数量要求。[完整生成流程](https://github.com/microsoft/skills/blob/main/.github/plugins/deep-wiki/commands/generate.md)。

## Every Compound Engineering：沉淀知识，并持续核对旧知识

当前名称为 [ce-compound](https://github.com/EveryInc/compound-engineering-plugin/blob/main/skills/ce-compound/SKILL.md) 与 [ce-compound-refresh](https://github.com/EveryInc/compound-engineering-plugin/blob/main/skills/ce-compound-refresh/SKILL.md)，不沿用搜索站上的旧 compound-docs 名字。前者只记录已解决、已验证，且最终代码或测试不能轻易还原的重要理由；默认一项经验一份 docs/solutions/<category>/<slug>.md。后者对存量经验核对当前代码，处理过期、重叠、替代与矛盾。

可借鉴机制：先查已有知识，高重叠时更新原文件；对行为主张回到源码求证；机械校验 frontmatter、路径、链接与 SHA 声明，必要时再做语义核验；检查下一位 Agent 是否能发现文档。无法验证不等于错误，规范与实现冲突也不能直接通过修改规范消除。[研究流程](https://github.com/EveryInc/compound-engineering-plugin/blob/main/skills/ce-compound/references/research.md)、[组装与校验](https://github.com/EveryInc/compound-engineering-plugin/blob/main/skills/ce-compound/references/assembly.md)、[刷新分类](https://github.com/EveryInc/compound-engineering-plugin/blob/main/skills/ce-compound-refresh/references/classify.md)。

边界：它更适合 runbook 中经过验证的经验，不能装入所有过程草稿。docs_root 只修改根目录，solutions 子目录仍固定；接入 docs/runbook 需要保留索引映射或修改路径协议。还可能维护 CONCEPTS.md，refresh 默认提交自身改动，不是纯只读巡检。第一方 README 已提供 Codex 安装支持，但本次未验证运行。建议仅借知识捕获和维护能力，避免引入另一套计划/执行权威。[README](https://github.com/EveryInc/compound-engineering-plugin)。

## opensrc：接近 vendor 的外部源码上下文入口

[vercel-labs/opensrc 的官方 Skill](https://github.com/vercel-labs/opensrc/blob/main/skills/opensrc/SKILL.md) 配合 CLI 抓取依赖或 Git 仓源码，按版本缓存到 ~/.opensrc/；npm 可读取锁文件，PyPI/crates 可指定版本，Git 仓可指定 ref。它现在既有 CLI，也有 Skill，不应只归类为命令行工具。

这适合查看 ThingsBoard、TBMQ 等外部实现时按明确版本获取上下文，但并不直接实现 docs/vendor 的选择性文件同步、保留理由和更新审查。未见其声明自动解析 Go/Java 依赖锁定；该类项目应使用显式仓库 SHA。若要形成团队知识库，还需记录来源、版本、选择范围、用途及许可信息。[CLI 文档](https://github.com/vercel-labs/opensrc/blob/main/packages/opensrc/README.md)。

## 已有与轻量候选

- [mattpocock writing-for-agents](https://github.com/mattpocock/skills/blob/main/skills/productivity/writing-for-agents/SKILL.md)：借鉴“这个链接是什么、何时读取”的上下文指针，以及按需加载、单一权威和删除重复材料的原则；不是全库同步器。
- [mattpocock domain-modeling](https://github.com/mattpocock/skills/blob/main/skills/engineering/domain-modeling/SKILL.md)：澄清领域术语、讨论边界场景、记录值得长期保存的 ADR。上游已使用 GLOSSARY.md，本地安装版仍使用 CONTEXT.md；本轮未升级。
- [Anthropic doc-coauthoring](https://github.com/anthropics/skills/blob/main/skills/doc-coauthoring/SKILL.md)：最值得借鉴的是只给新读者文档和问题，检验隐含前提、歧义与可回答性。适合人工 wiki、架构解释和 runbook 的写作验收，不提供存量漂移治理。
- [wshobson architecture-decision-records](https://github.com/wshobson/agents/blob/main/plugins/documentation-generation/skills/architecture-decision-records/SKILL.md)：提供 ADR 格式、状态、替代关系及索引建议，适合保存设计理由，不是整个系统连接地图。
- [OpenSpec](https://github.com/Fission-AI/OpenSpec)：业务规格与变更计划的候选方案，项目尚未采用。若后续选定它，可以让 docs/specs 业务地图只做导航并指向 openspec/specs 和 openspec/changes；目前不要求引入这些目录。无论选择何种工具，代码生成 wiki 描述实际实现，不能因此自动改写业务意图。

## 按六类职责组合的建议

| 职责 | 优先借鉴 | 必须保留的边界 |
| --- | --- | --- |
| specs | 试验 r-doc 的需求/设计组织能力；OpenSpec 仍为备选 | 唯一规格事实源，业务地图负责导航，正文不复制到 wiki |
| changelog | wiki-changelog；未来项链接权威计划或变更记录 | commit 历史不能代表路线图和承诺；记录工具尚待确定 |
| architecture | wiki-architect/page-writer；domain-modeling/ADR | 实现事实、批准决策和推断分别标明 |
| runbook | ce-compound + ce-compound-refresh | 已验证经验与过程草稿分开 |
| vendor | opensrc + 项目来源清单 | 按需源码缓存不等于选择性同步治理 |
| wiki | doc-coauthoring + research | 保留用户指定的人工维护边界 |

试点基础候选：r-doc 的文档组织、入口、索引与替代关系；可按实际缺口借鉴 HVE documentation 的影响映射与漂移检查。无需为了六个目录而强制安装六个技能；每个维护动作应说明触发条件、输入、允许写入范围、唯一来源及检查方式。候选和借鉴建议均不等于正式选型。

## 生态指标与获取入口

以下是 2026-10-12 的快照。Stars 来自 GitHub API，代表整个仓库；Installs 来自 skills.sh 页面，代表该目录统计，不是全部用户数，且可能滞后。低安装量候选作为设计参考，尚不称为经过广泛验证的方案。

| 候选 | 仓库 Stars | skills.sh Installs | 目录入口 |
| --- | ---: | ---: | --- |
| Microsoft wiki-architect | 3101 | 8 | [目录](https://skills.sh/microsoft/skills/wiki-architect) |
| Every ce-compound | 25485 | 约 3.6K | [目录](https://skills.sh/everyinc/compound-engineering-plugin/ce-compound) |
| Microsoft documentation | 1519 | 32 | [目录](https://skills.sh/microsoft/hve-core/documentation) |
| r-doc | 36 | 17 | [目录](https://skills.sh/riesaexe/r-doc/r-doc) |
| opensrc | 3015 | 约 1.3K | [目录](https://skills.sh/vercel-labs/opensrc/opensrc) |
| wshobson ADR | 40449 | 约 19.8K | [目录](https://skills.sh/wshobson/agents/architecture-decision-records) |

目录提供的获取命令如下，仅记录供评估，并未执行 add。它们安装的是单个技能，不等同于安装完整插件及全部依赖；试点时应先锁定被审阅版本并检查所需 references/scripts 是否齐全。

```bash
npx skills add https://github.com/microsoft/skills --skill wiki-architect
npx skills add https://github.com/EveryInc/compound-engineering-plugin --skill ce-compound
npx skills add https://github.com/microsoft/hve-core --skill documentation
npx skills add https://github.com/riesaexe/r-doc --skill r-doc
npx skills add https://github.com/vercel-labs/opensrc --skill opensrc
npx skills add https://github.com/wshobson/agents --skill architecture-decision-records
```

搜索记录：先查 skills.sh，再尝试 npx skills find 'knowledge base'；当前 Node.js 18.19.1 运行 skills CLI 1.7.0 因缺少 node:util.styleText 失败，随后使用网页目录、GitHub API 与第一方源码完成检索，没有升级运行时或安装技能。对 collar-specs/collar-vendor/mindInsight 的精确词检索未找到可核验的原套件公共地址；这不等于证明其未公开。
