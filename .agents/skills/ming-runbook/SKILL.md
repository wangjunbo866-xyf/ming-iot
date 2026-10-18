---
name: ming-runbook
description: 在 Ming IoT 中沉淀或修订可复用的操作、排障和验证方法，维护 AGENTS.md 及受影响导航。业务行为与需求取舍交回所属 Feature；单次运行记录不自动扩成新文档。
metadata:
  short-description: 按任务维护操作知识与入口地图
---

# Ming IoT 操作知识维护

版本：0.1.1-draft。本 Skill 组织操作；维护规则在团队约定，业务真相在所属 Feature。

从当前工作树 [AGENTS.md](../../../AGENTS.md) 选择入口；已读且任务未变时直接进入相关正文。读锚点对应章节，确有依赖再展开。范围、决定与验证复用原任务/Issue/PR 记录。

| 任务 | 按需读取 | 执行与写回 | 出口 |
| --- | --- | --- | --- |
| 查询操作方法 / 解释检查结果 | 已知 runbook 对应章节；未知时查[知识导航](../../../docs/README.md) | 说明适用环境、步骤及验证限制；仅咨询时不改文件 | 能按正文操作，未知项明确 |
| 沉淀排障、启动或验证经验 | [操作知识写回](../../../docs/runbook/team-development.md#runbook-maintenance)、现有相关 runbook | 优先修订同一主题正文；补可复现条件、步骤、成功判据和恢复边界，单次结果留原任务 | 新接手者可执行；已验证与未运行分别标明 |
| 新增或移动入口、改变团队命令/约定 | [入口维护](../../../docs/runbook/team-development.md#agents-maintenance)、受影响导航；涉及检查时读[本地检查](../../../docs/runbook/document-checks.md#agents-local-check) | 先更新所属正文，再依触发规则修正导航、来源索引和显式维护清单；根入口仅保留需要的路标 | 新入口可达，旧指针已处理，无第二份规则 |
| 根入口接近或超过行数预算 | [入口维护](../../../docs/runbook/team-development.md#agents-maintenance)及实际超长内容的所属正文 | 将操作细节归位并保留指针；运行本地检查，复核根入口五类职责 | 报告真实行数、退出码与剩余问题；不靠挤行或删约束过检 |

准备写入时按[认领](../../../docs/runbook/team-development.md#module-claim)明确本次正文和共享入口范围。操作经验若暴露业务规则缺失或改变，交回 [ming-specs](../ming-specs/SKILL.md) 对应路径，在所属 Feature 收敛；runbook 只保留如何操作及业务规格链接。运行破坏性命令前按[动作边界](../../../docs/runbook/team-development.md#delivery-boundaries)核对实际授权。

写完按影响运行 `make knowledge-check`；涉及根入口或其导航/命令变化时另运行 `make agents-check`。读取真实输出：警告与错误分别报告，行数通过不证明导航语义或业务正确。纯文字经验无需为了验证去操作旧环境或数据；缺现场条件时写明未运行及复现条件。

交付在原任务/PR 说明写回位置、验证、待确认项及为何改或不改根入口；按[交付约定](../../../docs/runbook/team-development.md#handoff)复审。不生成每轮摘要、平行手册或差异文件。没有受影响入口时不修改 AGENTS.md；不自动安装 Hook、启用门禁、归档资料或提交推送。
