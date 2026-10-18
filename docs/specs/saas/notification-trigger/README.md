# saas.notification-trigger：通知触发人展示

- 当前规则：[spec.md@current-v1](spec.md)，覆盖触发人名称、场景按钮文案和正文兼容；不是整个通知领域。
- 规格来源基线：`c165a56531f58421a870a0b8794f24556b60d3b2`。当前主规格已随 [PR #5](https://github.com/wangjunbo866-xyf/ming-iot/pull/5) 合并；核对至 main `56006502324800448c9775944d28478254be6142`，本模块相关实现未变，继续适用 `current-v1`。这不代表已部署或通过真实服务验收。
- 人的长期负责人：未分配。手动昵称优先由当前用户在 R2/D2 确认，本轮用户授权收敛文档；任务责任不变成长期所有权。
- 实现、调用、验证命令及未验证边界均在主规格；当前没有需叠加的有效覆盖或未完成业务 Patch。
- 合并后验证及当前环境缺口见 [PR #6 后续记录](../../../changelog/2026-10-17-governance-merged-next-plan.md)；[S1 记录](../../../changelog/2026-10-15-specs-current-baseline.md)保留首次收敛时点的候选与验证事实，不承担当前交付状态。
- 理解真实发送路径时，先读主规格的 [调用边界](spec.md#call-sites)，区分辅助函数行为与发送层是否调用；新上下文接手及文档修正见 [S3 记录](../../../changelog/2026-10-16-specs-clean-clone-takeover.md)。

## 按任务读取

日常理解从当前 spec 的规则、调用边界和验收开始；只有追查需求来源或准备改变这些行为时，再读取相关历史 Patch。规则不变的小修复直接引用条款，认领与验证回原任务/PR；新的行为调整按[团队 Patch 约定](../../../runbook/team-development.md#patch-convergence)维护。本轮仅改导航，不改变 current-v1 规则。

<a id="display-name-patches"></a>
## 历史与吸收关系

[展示名称 Patch R2](patches/PATCH-20261014-display-name/change.md) 已 absorbed 到 current-v1 的 DISPLAY-NAME、BODY-COMPATIBILITY 和 BOUNDARY。原决定及验收不变；后续新行为需另行记录确认与版本。

- [observed-v0 原文快照](spec.observed-v0.md)：修复前行为，其“当前/候选”描述仅指当时。
- [原试跑记录](../../../changelog/2026-10-14-ming-specs-handoff-pilot.md)与 [handoff.md](patches/PATCH-20261014-display-name/handoff.md)：原样保留历史回执。旧仓 SHA 不保证在新仓解析，不用于证明当前候选已验证。
- 小黄反馈任务已停止并废弃，未取得真人验收；不恢复、不催办。日常接手从本入口和当前主规格开始，无需新交接包。
