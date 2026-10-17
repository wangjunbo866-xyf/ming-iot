---
status: local_candidate
version: cross-agent-1
date: 2026-10-18
---

# 跨 Agent 接入与全新 clone 小修改试跑

用户授权补齐通用入口、区分普通开发和 LoopX，再用另一 coding agent 试跑小修改并独立复审；不推送、不启用新强制门禁。现有 Goal、协议契约待办、旧仓、私有备份和历史均保留。

## 交付设计

根 AGENTS 保留共同规则，LoopX 控制面要求移入[条件入口](../runbook/loopx-development.md)；新成员从[通用接入](../runbook/cross-agent-development.md)开始。ming-specs 继续指向同一团队约定和模块主规格，不要求每位成员安装同一宿主或私有调度环境。已纳管任务保留原准入约束。

## 独立试跑任务

在从完整本地候选创建、没有原项目私有状态的全新 clone 中，委托 ZCode 补充通知模块 SCENE-BUTTON 的字符数和同长首项回归，并完善现有规格的验证说明。只允许改原测试文件及当前 spec；不修改生产代码、行为、映射或已吸收 Patch。按 current-v1 的既有规则采用 no_behavior_change，不新建需求 Patch。

试跑提交目标固定为 clone 起点，新 Head 与声明使用真实 Git 对象。先用缺声明反例验证检查拒绝，再核验真实声明。协调者独立复跑文档、包级和完整 verify-pr，Reviewer 复核最终候选。工具已有不代表实际执行完成；最终结果待下节回填并绑定外部回执。

## 结果与限制

准备中的本地候选；ZCode 连通性已有真实响应，编辑、检查和独立复审尚未完成。暂不声称跨 Agent 验收通过。完整日志、精确输入和机器结果私有保留，最终提交由 Goal 外部回执绑定，避免报告引用自身提交。
