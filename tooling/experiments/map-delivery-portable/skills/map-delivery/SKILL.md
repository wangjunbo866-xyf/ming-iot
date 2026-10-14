---
name: map-delivery
description: Build or use a business capability map to maintain module specifications and deliver traceable changes. Use when the user asks for map-delivery, adopts this development mode, or changes specifications, plans, reviews, or evidence in an existing business map. Ordinary code edits without an established business map do not require adoption.
---

# 业务地图交付

通过“业务域 → 模块 → 现行规格 / 候选变更”组织开发上下文，让接手者能找到要求、实施进度和实际验证结果。本 Skill 提供可移植的写作与交付工作流；业务术语、文档语言、代码平台、审批和生效条件以目标项目为准。

## 定位与接入

1. 读取项目及目标目录适用的 `AGENTS.md` 和现有交付约定。用随 Skill 分发的脚本观察上下文：

   ```bash
   python3 <skill-dir>/scripts/context.py --project-root <project> --json
   ```

   用户已选定文档工作树时加 `--docs-root <selected-docs-worktree>`。`<skill-dir>` 是本文件所在目录。脚本只定位路径和观察本地 Git 提交，不联网、不创建文件，也不判断审批或交付是否通过。

2. 已有业务地图：沿地图入口、模块入口定位权威规格和本次 Change，保留项目现有目录与规则。首次接入、独立文档仓或定位失败时，读取 [接入说明](references/adoption.md)。缺失配置或规格不构成新建模块的授权；只有用户要求建立地图或批准了新模块范围，才起草相应结构。

3. 处理一次变更或继续交付时，读取 [交付工作流](references/workflow.md)，根据当前产物与已核实结果选择下一步。仅分析设计可停留在讨论；授权实施后完成所需实现与验证，不以文档写完代替交付完成。

## 持续有效的约束

- 现行规格保存完整行为；候选变更记录本次差异及变更后的完整要求，直到满足项目生效条件再更新现行内容。
- 使用稳定要求锚点和可观察的场景；计划关联要求，验证结果关联场景及实际候选版本。场景或候选变化后重新判断哪些结果仍然有效。
- 如实区分规格确认、实施完成、评审、合入和部署；版本、Issue/PR、审批及测试结果来自真实观察。项目未要求远端 Issue 时，可使用本地稳定 Change ID。
- 沿用目标项目更严格的审批、文件、校验与发布要求。调用 Skill 不改变执行权限，也不自动安装 Hook 或接入平台。

## 产物与完成判断

默认以模块 `README.md`、`spec.md` 和 `changes/<stable-id>/change.md` 起步。`change.md` 承担本次导航、范围和小型计划；复杂交付按需分出 Plan，验证实际执行后记录 Evidence，独立审查实际发生后记录 Review。模板只按需取用，见工作流中的 [模板选择](references/workflow.md#模板选择)。项目已有 Hub 或强制文件清单时沿用。

项目有校验命令时运行并报告实际结果；没有时执行工作流列出的人工检查，明确写出“人工检查”。本包没有原项目专用校验器的完整结构检查与激活能力，`context.py` 也不承担这些检查。

交付回复指出当前权威规格、候选范围、实际验证和剩余事项。用具体缺口引导下一步，例如“补测新增加的权限场景”或“核实候选版本的独立审查”，避免仅根据文件存在推断通过。

最短接入用法：`$map-delivery 为这个项目建立业务地图，先覆盖 <业务能力>。`

已有地图的用法：`$map-delivery 按现有地图完成 <需求>，关联受影响场景和实际验证结果。`
