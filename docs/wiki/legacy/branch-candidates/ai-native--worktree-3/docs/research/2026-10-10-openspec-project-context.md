# OpenSpec 能否承载 collar 式全栈项目上下文？

核查日期：2026-10-10。方法：官方网页浏览、固定源码静态核读；未安装 OpenSpec，未执行真实 Agent、store 或 schema 试验。

## 结论

**OpenSpec 可以成为这套知识体系的规格中枢，并已提供相当一部分上下文发现、注入和跨仓引用能力；默认配置不等于天猫 collar 的整套知识库。** 它不只是几个文档模板：有持久规格与变更、artifact 依赖图、CLI 查询与指令生成、宿主工作流、可定制 schema，以及 beta stores。另一方面，architecture/runbook/vendor/wiki 的内容、业务导航关系和运行配置查询仍需团队组织或接入。

“只用 OpenSpec”若指不再引入第二套规格框架，完全可以围绕 OpenSpec 配置少量项目文档和读取规则；若指安装后自动获得完整、持续更新的全栈知识库，本次核查不支持这一判断。以下分别列事实与推论，不把配置可能性写成默认已实现。

## 版本与来源边界

- 官方 [仓库](https://github.com/Fission-AI/OpenSpec) 与 [网站](https://openspec.dev/) 已浏览。网站页脚显示 v1.13.0，不据此判断源码版本。
- 只读 Git 核实 tag `v1.14.1` 指向 `87c3595ace6a2e22957f39ebe5b74c2f2316e0cb`；本次主线快照为 `9111a7654d7800391459431fff4eaf66e33a3d2e`，其 `package.json` 为 1.14.1。二者不是同一个 commit。
- 已逐字节比较 tag 与主线的六个关键文件：`docs/customization.md`、`docs/stores-beta/user-guide.md`、`schemas/spec-driven/schema.yaml`、`src/core/project-config.ts`、`src/core/templates/workflows/explore.ts`、`src/core/templates/workflows/apply-change.ts`，均相同。下文据此固定引用发布 tag 对应 SHA；其他补核源码明确引用主线快照，不暗示整个主线与发布包相同。
- 证据等级为“官方说明及实现静态核读”。Git tag 和源码可读不证明实际宿主加载、模型遵循、跨仓接入或本机运行结果。

## 能力矩阵：内建、可配置、团队补充

这里的 explore 是**宿主 Agent 的发现协议**：CLI 提供规格清单、路径和指令，Agent 使用宿主文件/搜索/工具能力调查项目；不是 OpenSpec CLI 自己完成全库检索。官方也明确区分终端 CLI 与聊天工作流。[How Commands Work](https://github.com/Fission-AI/OpenSpec/blob/9111a7654d7800391459431fff4eaf66e33a3d2e/docs/how-commands-work.md)

| 能力 | 已确认事实 | 分类与对 collar 的含义 |
| --- | --- | --- |
| 持久行为规格与一次变更 | 默认 schema 有 proposal、specs、design、tasks；specs 写可观察行为/场景，design 写技术方案，tasks 写实施清单。[默认 schema](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/schemas/spec-driven/schema.yaml) | **内建**。覆盖 specs 和变更设计；不是空白模板集合，也不能把一次 Change 的 design 当完整现行架构。 |
| 能力目录与渐进读取 | 默认 proposal 和 explore 先列 `openspec list --specs`，可用 `show --type spec --json --no-scenarios` 了解用途/要求，再完整阅读相关规格及场景；支持已有层次化 capability path。[schema](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/schemas/spec-driven/schema.yaml)、[explore](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/templates/workflows/explore.ts) | **内建的规格导航**。可按业务域组织；这与跨前端/API/数据/部署的全项目地图仍有区别，目录对应关系由团队提供。 |
| 已有大项目的覆盖范围 | 官方建议只为实际触及部分逐步建立规格，不要求启动时反向整理全部代码；已有 PRD 等作为探索材料。[Existing Projects](https://github.com/Fission-AI/OpenSpec/blob/9111a7654d7800391459431fff4eaf66e33a3d2e/docs/existing-projects.md) | **内建采用策略**。起初 `specs/` 不完整是预期行为，不能将“规格列得出来”解释为整套业务都已建模。 |
| 项目背景与约束注入 | `openspec/config.yaml` 的 `context` 提供项目背景；`rules` 按 artifact ID 注入对应产物指令；可设置默认 schema。[Customization](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/customization.md) | **可配置、已有实现**。适合技术栈、约束、入口与少量索引，不需另造同类上下文注入器；内容仍由团队维护。 |
| context 的实际限制 | 源码按 UTF-8 字节限制为 `50 * 1024`；超限会警告并忽略 context 字段；错误字段采用部分配置降级。字符串没有自动文件 include/链接正文展开机制。[project-config.ts](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/project-config.ts#L248) | **注入不是知识库检索**。不要把全部 wiki 粘入 context；写路径/链接不等于其内容已经加载，需 Agent 后续读取。 |
| apply/archive 的当前上下文 | `instructions apply/archive` 返回当前选定根的 context 与独立 `operationGuidance`；选 store 时从 store 取值。上下文是提示层必读输入，guidance 是可适用的建议；不自动复制进交付物。[Customization](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/customization.md) | **可配置**。可提醒先读架构或运行手册，但“每次读新配置文件”不能证明配置中的知识仍正确。 |
| explore 读取代码与现有材料 | 工作流明确允许读取文件、搜索代码、执行只读工具，要求提问前检查相关规格、源码、测试、文档与配置，并读取所选根的 config。[explore 源码](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/templates/workflows/explore.ts) | **内建发现指引，宿主执行读取**。它能沿真实项目探索，不局限于 openspec 文件；检索质量与权限仍依赖 Agent/宿主和可发现入口。 |
| apply 的 contextFiles | CLI 枚举所选 schema 已存在的 artifact 具体路径，工作流要求读取这些文件，并根据任务修改代码。[apply](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/templates/workflows/apply-change.ts#L69)、[路径组装实现](https://github.com/Fission-AI/OpenSpec/blob/9111a7654d7800391459431fff4eaf66e33a3d2e/src/commands/workflow/instructions.ts#L591) | **内建产物上下文**。`contextFiles` 不是整仓文件依赖图，也不会自动囊括现行 runbook、所有源码或线上配置。 |
| 自定义 schema | 可以 fork/init schema，定义 artifact、输出、模板、instruction、依赖及 apply 跟踪文件；schema 可随项目版本化，也有用户级覆盖。[Customization](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/customization.md#custom-schemas) | **可配置扩展**。可以增加 context-map、接口影响、迁移说明或知识回写项；新增模板不自动产生知识，也不自带长期去重/更新策略。 |
| 跨仓规划与只读上游规格 | beta store 是独立规划仓；`references` 为消费仓提供上游 canonical specs 的摘要与读取命令；store 指针和显式选择解决规划根。[Stores guide](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/stores-beta/user-guide.md) | **可选 beta 能力**。适合单一团队规格仓，不应再复制各仓一份同义规格；要接受其接口仍可能变化。 |
| 跨仓上下文不是自动汇总 | workset 可一起打开规划仓与代码仓；不会把组件 context 复制到 store、自动选受影响仓库或授予编辑权限。references 索引 canonical specs，不包含 active changes；索引读本地 checkout，OpenSpec 不自动 clone/pull/push。[Stores guide](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/stores-beta/user-guide.md) | **有导航与可见性辅助，仍需团队同步和声明边界**。跨仓接口关系、候选版本和实际环境不能从“store 已注册”推得。 |
| 索引与健康检查 | `doctor` 检查配置根/引用健康，`context` 展示声明的工作集合；references 实现取 specs 的 ID、Purpose 摘要和读取指令，不内联全文，有大小预算及 warning 降级。[guide](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/stores-beta/user-guide.md)、[references.ts](https://github.com/Fission-AI/OpenSpec/blob/9111a7654d7800391459431fff4eaf66e33a3d2e/src/core/references.ts#L1) | **内建有限索引/健康检查**。不等于任意资料的全文/语义搜索服务，亦不证明规格、代码和运行环境语义一致或最新。 |

### 默认还没有覆盖什么

事实：核读的默认 schema 只列四类变更产物，explore/apply 通过宿主读取相关材料，references 索引的是声明 store 内的规格。官方 brownfield 指南明确不要求预先完整梳理全项目。[schema](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/schemas/spec-driven/schema.yaml)、[Existing Projects](https://github.com/Fission-AI/OpenSpec/blob/9111a7654d7800391459431fff4eaf66e33a3d2e/docs/existing-projects.md)

据此的判断：**没有证据表明默认安装会交付 collar 那种完整项目知识体系**，包括业务地图到代码/接口/数据/部署的稳定映射、现行 architecture、持久 runbook、外部 vendor 资料与团队 wiki 的分类维护，以及它们的负责人、更新触发、冲突消解和语义新鲜度检查。这个结论限定于核读的内建能力，不等于这些内容不能与 OpenSpec 共存或通过 schema/宿主工具扩展。

运行环境也要单列：本次核读未发现默认提供天猫式“按场景/Agent/环境查询数据库中的 Prompt、Skill 或运行配置”的产品连接器。OpenSpec 的 config 是其工作流配置，不是业务运行配置数据库；explore 可以调用宿主已有只读工具，但连接器、权限、环境标识、返回版本与脱敏由团队提供。此处区分接口可以调用与业务查询已经接通，未进行任何线上查询验证。

## 面向本团队的瘦身映射（建议，未实施）

| collar 结构 | 可选最小落点 | 维护责任和必要区别 |
| --- | --- | --- |
| AGENTS → 业务地图 | 短导航指向一份项目知识入口；列业务域→OpenSpec capability→代码/API/数据/部署入口 | 入口只保存关系和定位信息，不复制规格正文。OpenSpec 自带规格清单可直接复用，只有跨层关系才补。 |
| specs | `openspec/specs` 与 `openspec/changes` 保持唯一规格/变更来源 | 行为要求、设计、任务使用已有能力；不存在的业务要求不能靠读取代码自动认定已批准。 |
| changelog | 先用 Change archive 和 Git 历史；有发布受众时再生成发布摘要 | archive 是变更历史来源，不天然等于发布说明或所有代码提交的摘要。 |
| architecture | 少量现行架构入口、接口/数据模型来源，必要 ADR；Change design 链接它 | 当前架构与一次修改的设计有不同生命周期，不能靠堆积历次 design 要求读者重建全貌。 |
| runbook | 只保存可复用的启动、排障、验证和恢复步骤 | 发生真实复用需求才沉淀；普通探索不强制生成一份，避免文档噪声。 |
| vendor / wiki | 原始依据保留来源/版本；团队结论留简短解释与链接 | 不为贴合名称强行分六个目录；可与现有 docs 合并组织，知识来源不能冒充业务规格。 |
| 动态只读查询 | 先提供现有脚本/CLI 查询，确有多人/多宿主复用需求再接 MCP | 返回环境、版本/摘要与采集时间；默认只读、脱敏；不要将实时结果复制成永不过期的 context。 |

建议先使用默认 schema，`context` 只放项目简介、规则和导航链接；按 artifact 配置应读的知识类型，apply guidance 提醒受影响知识回写。只有同类变更稳定需要额外产物时才加自定义 schema。否则把 architecture/runbook/wiki 全变成每次 Change 的必填 artifact，会制造重复维护而非补足项目知识。

“全栈上下文足够”的试点可用一次新会话走查：从业务需求找到现行规格，追到前端/API/数据/异步事件/部署入口；识别当前环境配置来自哪里；遇到缺失或相互矛盾的信息时明确停在哪个判断。记录定位耗时、遗漏、重复材料和人工补充量。该建议检验知识可发现性，不是再次增加一套验收门禁，也不宣称已验证。

## 本次验证范围

- 只下载/读取第三方源码，未执行第三方代码或安装依赖；本地缓存 `/tmp/openspec-context-20261010` 不进入提交。
- 只新增本研究文档；没有调整 canonical skill、业务规格、运行状态、工作区配置或共享票据。
- 提交前按 README 检查：`python3 -m unittest discover -s tests -v`，52 项通过（2.284 秒）；`python3 tools/team_skills.py validate` 返回 `validate: OK (map-delivery)`。这些检查仅验证研究文档所在仓库的既有工具，不证明 OpenSpec 调研结论、模型行为或实际集成效果。
