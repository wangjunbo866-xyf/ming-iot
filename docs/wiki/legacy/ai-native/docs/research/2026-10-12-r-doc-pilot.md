# r-doc 首轮隔离试点

日期：2026-10-12。结论：值得作为 Ming IoT 父仓知识治理的试点基础；已有可复现的小幅改进，但尚未证明复杂业务任务的准确率或效率收益，未正式采用。

用户已明确：项目尚未使用 OpenSpec，它只是待定方案。代码仓继续独立，由父仓统一协调；ai-native 内容按项目专用与跨项目复用分别处理。试点不预设 OpenSpec，也不引入第二份业务事实源。

## 为什么值得继续

[r-doc 的固定版本入口](https://github.com/riesaexe/r-doc/blob/bc23420d7df5ec1d2c5fb45e066c919790ab9cf9/skills/r-doc/SKILL.md) 已包含 AGENTS → 索引 → 专题的按需读取、文档状态及替代关系、分级治理、审计与修复脚本。它允许保留现有目录，可以用来管理 specs、changelog、architecture、runbook、vendor、wiki 的入口和生命周期，无需为了六个目录先拆成六个 Skill。

它没有替项目定义业务地图、IoT 连接类型、跨仓版本证据、vendor 选择性同步或人工 wiki 的编辑边界。这些需要用真实任务验证后补充。目录全面与知识正确是两项不同的验收内容。

## 范围与来源

- 上游：[riesaexe/r-doc](https://github.com/riesaexe/r-doc)，提交 `bc23420d7df5ec1d2c5fb45e066c919790ab9cf9`，Skill 元数据版本 1.0.0。
- [来源记录](../../.scratch/r-doc-pilot/source.json)保存下载包哈希；upstream 快照保留原样。
- 试点根：[r-doc-pilot](../../.scratch/r-doc-pilot/)，被本地忽略规则排除在提交候选之外。
- baseline 是上游 Skill 加独立评测夹具；candidate 是窄范围补丁，未安装到用户全局 Skill，也未发布。
- 业务仓文档、代码及远端均未因试点而迁移或修改。此前本会话调研与决策图中“OpenSpec 已确定”的前提已更正。

## 已执行的验证

| 验证 | 结果 | 能说明什么 |
| --- | --- | --- |
| 上游原有 unittest | 136/136 通过 | 上游包在补齐声明的开发依赖后可运行 |
| 新增 8 个针对性回归用例跑原版 | 2 通过，6 失败 | 复现空格链接解析与入口可达性差异，含正常对照 |
| 同 8 个用例跑候选 | 8/8 通过 | 两项窄改进满足试点预期 |
| 候选完整 unittest | 144/144 通过 | 原有测试与新回归均通过 |
| skill-creator quick_validate | 通过 | Skill 包基本格式有效 |
| skill-up 真实 Agent 配对样例 | 原版 r-doc 与不带 r-doc 均通过 | 两组都能修正候选方案状态；未证明准确率增益 |

最初使用系统 Python 跑上游测试有 8 项失败，逐项定位为缺少上游声明的 pytest 开发依赖。隔离 venv 安装 requirements-dev.txt 后全部通过，此问题未计作 r-doc 缺陷。

日志：[上游全量](../../.scratch/r-doc-pilot/reports/upstream-unittest-with-deps.log)、[修复前](../../.scratch/r-doc-pilot/reports/pilot-regressions-before.log)、[修复后](../../.scratch/r-doc-pilot/reports/pilot-regressions-after.log)、[候选全量](../../.scratch/r-doc-pilot/reports/candidate-full-unittest.log)。

## 已完成的两项优化

1. **合法空格路径解析修复。** `[Document](<docs/device status.md>)` 及引用式同类链接原先被截断，文件存在仍报断链。候选保留尖括号边界，让既有路径解析器正确处理；覆盖中文锚点、编码空格与标题的正常对照。
2. **父仓单入口可达性强化。** 原版可以接受“断开的索引链接到主题”，但从 AGENTS 无法到达。候选只在已经审计的链接图内遍历，要求配置 docs_root 内的维护文档真正从 AGENTS 可达；根 README 可以作为通路，不强制每个根 Markdown 都被索引，原有父子索引要求保留。此项是本项目目标的契约强化，不把所有行为差异归为上游 bug。

候选同步调整 SKILL 与三份相关引用文档，统一指向这条试点契约。独立补丁审阅未发现具体 P1/P2 问题，范围限于改动及其直接调用关系。

可审查产物：[补丁](../../.scratch/r-doc-pilot/reports/candidate-fixes.patch)、[文件哈希与结果](../../.scratch/r-doc-pilot/reports/candidate-fix-summary.json)、[候选 SKILL](../../.scratch/r-doc-pilot/candidate/r-doc/SKILL.md)。候选仍保留上游版本号，需用补丁和哈希辨别，不能当作已发布的新版 r-doc。

## Agent 配对样例与限制

使用 skill-up 0.12.0 和现有 Codex 登录；不覆盖模型、provider 或 base_url。临时工作区隔离任务文件和 r-doc 安装位置，但继承宿主登录、配置及全局 Skills。因此这是同一宿主默认环境下的配对冒烟测试，不是干净环境的大规模基准。

夹具根据本项目发生过的 OpenSpec 状态混淆合成，不冒充真实业务仓快照。包含现行状态、候选提案、被替代的历史讨论和错误的索引状态。任务只允许改索引并输出结构化依据；两组各跑一次，使用相同提示、夹具及脚本判据。

两组均正确将 OpenSpec 保持为 proposed，引用当前状态，保留原有材料。逐文件字节比较确认只改了 docs/README.md 并增加 report.json。with_skill 轨迹确认实际读取了 r-doc/SKILL.md，也执行了审计，并非仅把文件放进安装目录就算启用。

**任务通过不代表文档治理门禁通过。** 本例 minimal 文档已有不完整 frontmatter；r-doc 对出现的元数据仍要求完整字段，审计前后均报 20 项原有缺失。Agent 如实报告且未越界补齐。这暴露了存量文档接入成本：需要决定渐进补齐还是明确兼容策略，本轮没有为让结果变绿而放松检查。

单次记录中 with_skill 完成 18 次 shell 调用，without_skill 为 8 次；不能据此推算整体成本，但此例没有体现效率收益。日志另有宿主 legacy_notify hook 缺失警告，任务完成正常，未修改宿主配置。

这次 Agent 配对测试对象是**未修改上游**；优化候选仅完成确定性回归，尚未跑真实 Agent 的复杂任务复测。

证据：[配对报告](../../.scratch/r-doc-pilot/reports/agent-status-smoke/iteration-1/benchmark.md)、[完整结果](../../.scratch/r-doc-pilot/reports/agent-status-smoke/iteration-1/report.json)、[轨迹观察与审计结果](../../.scratch/r-doc-pilot/reports/agent-observations.json)、[带 Skill 最终差异](../../.scratch/r-doc-pilot/reports/with_skill-fixture.diff)、[无 Skill 最终差异](../../.scratch/r-doc-pilot/reports/without_skill-fixture.diff)。

## 对 Ming IoT 的采用建议

建议先采用“上游 r-doc + 小补丁 + 项目配置”的试点方式，暂不发展成整套新框架。

- 父仓保存项目知识、目录地图、治理配置与仓库版本引用。AGENTS 保持短入口，按用户要求包含仓库地图、快速命令、关键约定、当前状态；状态应链接到有来源的记录，避免出现另一套手工事实。
- 通用解析、检查和修复逻辑适合保留在可复用工具中；IoT 术语、接口分类、文档所有者和验收约定留在父仓。通用修复可以准备向上游贡献，但本轮没有发布或发送。
- 先用一个 Skill 管理知识生命周期；确认某类工作需要独立触发、专门输入和不同写入范围后，再拆职责。六个目录不自动等于六个 Skill。
- 在旧文档的元数据兼容策略明确前，审计先作为试点诊断；不直接设置为全仓阻断门禁。

扩大试点时，选一个经现有资料确认的业务切片，比较当前方式、原版、优化版三组。关键验收如下：

| 场景 | 应验证的结果 |
| --- | --- |
| 新会话定位 | 从 AGENTS 找到权威规格、相关代码和约束，记录实际读取量与答案正确性 |
| 跨仓引用 | 父仓检出完整、子仓未检出、子仓独立打开三种情况下，能说明 repo/path/SHA 与无法验证的部分 |
| 事实变更 | 只更新拥有该事实的正文，其余入口链接它，保留历史与替代关系 |
| 失效知识 | 链接和日期正常但内容与代码冲突时，定位冲突而非只刷新日期 |
| 低影响编辑 | 改错字不会触发全库扫描、批量建模板或改业务代码 |

`related_code` 当前只验证项目根内现存路径，不提供跨仓身份与提交证据模型；单靠文件存在无法判断实现与规范一致。以上场景仍未完成验证，因此当前结论是继续有限试点，而非全量推广或已替代 OpenSpec。

## 复现入口

从试点根目录运行；依赖使用已创建的隔离 venv：

```bash
cd upstream
PYTHONDONTWRITEBYTECODE=1 ../.venv/bin/python -m unittest discover -s skills/r-doc/tests -p 'test_*.py'
```

候选完整回归副本为 candidate-regression，命令相同。新增用例的原版对照可从试点根运行：

```bash
RDOC_SCRIPTS_ROOT="$PWD/upstream/skills/r-doc/scripts" .venv/bin/python -m unittest discover -s candidate/r-doc/tests -p test_pilot_regressions.py -v
```

该对照预期有 6 项失败；不要把它作为候选回归命令。Agent 评测配置为 baseline/r-doc/evals/eval.yaml，可使用 `/root/.local/bin/skill-up validate` 检查；重新运行配对测试会产生新的模型调用。
