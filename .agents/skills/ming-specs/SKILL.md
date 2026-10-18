---
name: ming-specs
description: 在 Ming IoT 任务中定位当前 Feature 规则，按任务维护需求、规格与 Patch，处理并行认领、提交差异及交付收敛。只读咨询可仅定位和解释；历史资料不自动成为当前规格。
metadata:
  short-description: 按任务读取、维护与验证 Ming IoT 规格
---

# Ming IoT 规格维护

版本：0.12.1-draft。本 Skill 组织操作；规则正文在 Feature 和团队约定中。

## 先选路径

从当前工作树 [AGENTS.md](../../../AGENTS.md) 定位任务入口；已读且任务未变时无需重复读取。只读当前任务相关的章节，不预先通读所有约定、raw、Patch 或模板。首次接入或不清楚任务方式时读[通用接入](../../../docs/runbook/cross-agent-development.md#task-mode)；明确纳管后才读 LoopX 专用入口。

从[业务地图](../../../docs/specs/README.md)找到 Feature README 与当前 spec；已知入口可直接读取。先判断当前规则、候选、实现和部署分别是什么状态。以下一行就是本次操作的阅读与写回契约；复合任务依次选实际涉及的行。

读取锚点链接时先定位该章节，再按其依赖补读；需要多个章节时分段读取，避免整页加载与截断后的重复读取。

| 任务 | 按需读取 | 操作与写回位置 | 本路径出口 |
| --- | --- | --- | --- |
| 只读定位、解释规则、接手理解 | Feature README → 当前 spec；需核实实现才读相关代码，需原因才读本地 raw/历史 | 解释目标、正常/异常规则、边界、版本及验收；报告具体知识缺口 | 能用当前正文回答；仅咨询时不建任务或改文件 |
| 收到一句话需求、接受澄清 | [受理流程](../../../docs/runbook/requirement-to-delivery.md)、[决定规则](../../../docs/runbook/team-development.md#decisions)及相关当前条款 | 原话/真实回答追加本 Feature raw 或现有 Change 来源节；选项、确认、未决项回同一 Change | 事实与建议可区分；确认来源及阻塞范围明确 |
| 划分域、Feature、服务或提取能力 | [划分标准](../../../docs/runbook/team-development.md#domain-feature-service)、[完整性](../../../docs/runbook/team-development.md#feature-knowledge-completeness) | 按长期职责定归属，另记服务映射；获准提取时将需求、澄清和规则归入该 Feature，旧处留历史及依赖 | 新正文独立说明本功能；现状、候选和部署不混同 |
| 开始写入、多人/多 Agent 并行 | [认领](../../../docs/runbook/team-development.md#module-claim)、相关在途任务；重叠时读[并行收敛](../../../docs/runbook/team-development.md#patch-convergence) | 在原任务/PR 记录 Feature、基线条款、写范围、责任及共享影响；协调重叠条款和接口 | 本次写范围明确；未决冲突仅阻塞相关写入 |
| 小修复、重构、补测试或说明 | 当前条款、[小修改](../../../docs/runbook/team-development.md#small-changes) | 先比约定行为；规则不变则改实际文件，在原任务留条款、理由和验证；规则缺失或改变转澄清/变更路径 | 用对应回归证明所述范围；不为了检查制造 Patch |
| 新建能力、改变行为、退出或探索 | [四种类型](../../../docs/runbook/team-development.md#change-types)中对应类型；Patch 读[收敛规则](../../../docs/runbook/team-development.md#patch-convergence) | 按归属写 feature/patch/sunset/blueprint；规则变化在同一候选更新完整主规格与双向指针 | 业务取舍有真实确认；候选状态、验收、未决项与依赖一致 |
| 引入可选分析方法 | [分析接入与写回](../../../docs/runbook/team-development.md#optional-analysis-skills) | 方法产物映射回当前 Feature/Change，保留事实、建议与真实决定；不另建平行业务正文 | 新发现有归属、确认责任和下一步 |
| 提交、PR 或交付 | [差异语义检查](../../../docs/runbook/team-development.md#spec-code-check)、[工具输入与命令](references/pr-check.md)、[动作边界](../../../docs/runbook/team-development.md#delivery-boundaries) | 对实际 staged tree 或完整 base/head 差异找全部归属；运行适用检查并处理遗漏；结果留原任务/PR | 结构、语义评审和运行证据分开；最终候选版本明确 |
| 收敛主规格、交接执行 | [Patch 收敛](../../../docs/runbook/team-development.md#patch-convergence)、[交付与接手](../../../docs/runbook/team-development.md#handoff) | 同一候选收敛当前规则；保留吸收来源，原任务提供版本、待办和验证入口 | 下一位能从当前正文接手；合并、部署分别核实 |

## 执行时补齐什么

- 任务只涉及操作经验、排障步骤或项目入口维护时，转 [ming-runbook](../ming-runbook/SKILL.md)；业务规则及需求决定继续写回所属 Feature。

- 需要记录格式时才读[模板](references/templates.md)对应段落；复用原任务/PR/Change，格式不替代事实与批准。
- 缺少现行规则时按[已有模块首次接入](../../../docs/runbook/team-development.md#observed-baseline)补可核实的最小基线；源码现状与已确认要求分别说明。
- 新事实与已有规则冲突时，回到对应条款判断实现偏差、需求歧义或业务调整；只对依赖真实取舍的部分请求确认。人的责任与已有授权按[责任约定](../../../docs/runbook/team-development.md#responsibilities)核对。
- 单 Patch 暂存区检查、离线或跨权限的固定快照仅在任务需要时读[手动检查与快照](references/pilot-regressions.md)；日常接手无需额外交接包。

## 完成检查

核对本路径出口、实际差异及未完成项：写回了唯一所属正文，规则变化已在候选内收敛，所需认领/决定/验证有依据，当前入口可达。准备交付时按约定完成独立评审；只读定位和纯概念问答不自动扩为实施。

结构检查通过只证明其确定性范围。缺证据、未运行、候选和待确认分别如实说明；本 Skill 的发现、加载或调用不等于 Hook 已安装、门禁已启用或需求已验收。
