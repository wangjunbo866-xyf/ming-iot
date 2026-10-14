# 《AI Native 研发范式实践手册》阅读与团队环境建议

调研日期：2026-10-04。本文是针对 10 人项目组的设计建议，尚未实施，也不构成环境或权限变更。

## 原文与阅读范围

原文来自阿里公开[在线阅读器](https://ai-native.alistatic.com/app/ainativeinfra/ai-native-handbook-web/index)链接的 [PDF](https://g.alistatic.com/s/v/ainativeinfra/ai-native-handbook/0.0.1/ai-native-handbook.pdf)，共 68 页。本次协作重点核读 PDF 第 2–55 页，包括三个案例、五项挑战，以及 Harness、知识、工具和运行环境章节；未核读后续全部安全与可观测章节，不声称通读。正文印刷页码比 PDF 页码少 5，例如正文第 19 页对应 PDF 第 24 页。

PDF 已下载到本仓库忽略目录 `.local/research/alibaba-handbook/ai-native-handbook.pdf`，SHA-256 为 `6929f3e9e57fef9550129d457ba82ac40a4a9368e1c5926c6ac2206d8aaeced3`。原文文字已转曲，文本提取为空，阅读依据是逐页渲染图；未把空文本提取当作阅读结果。文件元数据的制作时间不等同于正式发布日期。

## 对设计最有用的原文依据

下表只保留设计依据，后文的具体仓库划分、优先级和试点方案是针对本团队的建议。

| 原文依据 | 定位 |
| --- | --- |
| 建设能提供真实反馈的环境，让任务随验证结果推进。 | 正文 18–19，[PDF 23–24](https://g.alistatic.com/s/v/ainativeinfra/ai-native-handbook/0.0.1/ai-native-handbook.pdf#page=23) |
| AGENTS 指令的加载与可靠遵守是两件事。 | 正文 37，[PDF 42](https://g.alistatic.com/s/v/ainativeinfra/ai-native-handbook/0.0.1/ai-native-handbook.pdf#page=42) |
| Skill、CLI、MCP 各有职责；工具评估同时看选用与执行。 | 正文 41–42，[PDF 46–47](https://g.alistatic.com/s/v/ainativeinfra/ai-native-handbook/0.0.1/ai-native-handbook.pdf#page=46) |
| 可运行项目还需要依赖、配置、数据和验证条件。 | 正文 47–50，[PDF 52–55](https://g.alistatic.com/s/v/ainativeinfra/ai-native-handbook/0.0.1/ai-native-handbook.pdf#page=52) |
| 使用量、工程质量与交付结果需要关联观察。 | 正文 22、25，[PDF 27](https://g.alistatic.com/s/v/ainativeinfra/ai-native-handbook/0.0.1/ai-native-handbook.pdf#page=27)、[PDF 30](https://g.alistatic.com/s/v/ainativeinfra/ai-native-handbook/0.0.1/ai-native-handbook.pdf#page=30) |

案例数字不能作为本团队的收益承诺。例如，万有无界案例的 89% 一次修复成功率限定于上下文充分的视觉缺陷样本；千问案例的交付周期减半是该团队 2026 年 5–8 月的实践报告，不是通用保证。[正文 13 / PDF 18](https://g.alistatic.com/s/v/ainativeinfra/ai-native-handbook/0.0.1/ai-native-handbook.pdf#page=18)、[正文 8 / PDF 13](https://g.alistatic.com/s/v/ainativeinfra/ai-native-handbook/0.0.1/ai-native-handbook.pdf#page=13)

正文 48 的电商例子是虚构示例，正文 50 的 YAML 也明确不是具体产品配置规范；落地时应使用项目真实命令与配置。[PDF 53–55](https://g.alistatic.com/s/v/ainativeinfra/ai-native-handbook/0.0.1/ai-native-handbook.pdf#page=53)

## 针对现有工作区的最小设计

建议使用现有 Codex 作为执行宿主，以 `ai-native` 管团队共享资产，以 `ming-iot-rd-docs` 管交付事实，以业务仓管可运行实现。本轮设计不引入 TeamAI，也不要求先建设自有 Agent 平台。

| 层次 | 放在哪里、做什么 | 可见的完成条件 |
| --- | --- | --- |
| 团队基线 | `ai-native` 管批准版本、必需技能、兼容的工具版本及初始化/检查入口。先支持团队实际使用的一种环境组合。 | 任意成员从干净工作区接入，检查结果能指出版本、缺项和修复方法。 |
| 项目上下文 | 文档仓保存需求、边界、验收、设计决策；业务仓保存模块入口、运行事实；工作区索引按任务指向这些来源。 | 新会话能找到正确仓库、需求与运行入口，减少重复口述和副本。 |
| 执行能力 | Skill 描述何时使用及如何推进；已有 Python、Make 和 API 工具负责确定性操作。需要跨系统标准接入时再增加 MCP。 | 工具返回可判定的状态、输出位置和错误原因；相同输入能重跑。 |
| 运行环境 | 每个业务仓维护运行时、依赖服务、配置样例、测试数据、启动与健康检查方法。任务使用独立工作树和适当隔离的测试依赖。 | 换一台成员机器或新实例，能复现同一条验收路径。 |
| 验证与记录 | 业务仓校验代码，文档仓校验交付记录，`ai-native` 校验分发和 Skill 行为。结果关联任务和提交。 | 交付能展示所测提交、执行命令、结果及尚未覆盖部分。 |

各仓 `AGENTS.md` 提供该仓的常驻约定和权威资料入口，Skill 提供按任务选择的方法。对必须遵守的要求，逐条明确对应的程序检查或人工验收；本地启动检查确认成员环境，CI 检查交付产物。CI 不能证明成员本机已安装技能，也不能证明模型阅读并遵守了技能。

当前仓库已有可复用基础，以下是本次本地文件核查结果，不代表远端流水线或合并门禁已生效：

- [team_skills.py](../../tools/team_skills.py) 已支持只指定工作区进行安装和检查；其文件锁验证本机源码与本机锁的一致性。独立的“团队当前批准提交”仍应另设来源，否则旧源码配旧锁也可以自洽。
- [产品仓 Makefile](../../../ming-iot/Makefile) 已有 `verify-fast VERIFY_SCOPE=...`、`verify-pr`、`verify-integration` 与 `verify-release`。优先复用这些入口，不另写一套同义测试包装。
- [文档仓校验配置](../../../ming-iot-rd-docs/.workflow/map-docs-check.yml) 已声明单元测试、交付元数据、地图和差异检查。接入团队时仍需在平台验证流水线确实执行、失败是否阻止合并。

当前 `README.md`、`AGENTS.md` 和 `manifest.json` 仍保留 TeamAI 接入内容。落实本方案时，应同步更新正常成员接入入口，并将已有试点记录标为历史资料。

建议最先补“批准版本 + 环境预检”。批准版本应从明确的团队发布引用或项目版本约定取得；更新时解析并记录确切 Git 提交，检测本地改动，提供返回上一批准版本的方法。`skills.lock.json` 继续负责内容完整性，两者分别检查。

工作区安装作为默认模式。真正验收时，从每位成员实际使用的 Codex 入口检查技能发现结果；需要独立打开子项目的成员再登记相应入口。安装位置与技能业务职责分开考虑：`map-delivery` 可在业务开发会话中处理文档仓交付记录，但不应为无关代码任务强加文档流程。

为 `map-delivery` 准备少量真实行为用例：文档需求触发、跨仓定位、无文档交付动作的普通任务、显式调用、文档仓缺失、同名技能冲突。除了判断能否发现技能，还要检查是否选对来源、是否产生正确结果、是否保留现有授权边界。用本团队失败案例逐步增加用例。

共享发布建议由 1 名维护者负责、另一名成员复核，其他成员通过 PR 提供经过任务验证的改进。个人偏好不必全部统一；进入团队基线的能力必须有人维护、能够检查、能够回退。暂不以 Skill 数量、Token 用量或 AI 生成代码行数给个人排名。

## 两周试点建议

| 时间 | 交付内容 | 验收方式 |
| --- | --- | --- |
| 第 1–3 天 | 明确一个批准版本，补预检与初始化流程，先由两名成员在干净目录接入。 | 双方能发现同一版必需技能，跑通已有快速检查；记录环境差异。 |
| 第 4–5 天 | 让其余成员按同一流程接入；补一条真实业务的启动、数据和验证说明。 | 10 人按实际宿主入口检查；第二个人可以按记录复现。 |
| 第 6–9 天 | 选择一条有清楚验收条件的小需求，完成需求、实现、验证、PR 和交付记录。 | 业务结果通过验收，所测提交与 PR 提交对应，失败能定位到具体环节。 |
| 第 10 天 | 复盘等待时间、返工、人工接手原因和可复用经验，决定下一版。 | 只把重复出现且已验证的经验纳入团队 Skill、工具或项目文档。 |

试点先记录需求交付时间、返工/回滚、验证成功情况、人工接手原因和运行成本，并保留任务类型与复杂度说明。样本很少时报告事实与瓶颈，不据此宣称因果提效比例。后续投入应由这些结果决定：如果主要卡在服务起不来，优先补环境；如果卡在需求误解，优先补上下文；如果反复选错工具，再改 Skill 的范围和行为用例。
