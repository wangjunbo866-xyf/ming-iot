# engineering.api-contract：HTTP API 契约校验

- 当前规则：[spec.md@current-v1](spec.md)，只覆盖 `apicheck lint` 的选择、元数据与只读退出契约。
- 规格来源基线：`c165a56531f58421a870a0b8794f24556b60d3b2`。当前主规格已随 [PR #5](https://github.com/wangjunbo866-xyf/ming-iot/pull/5) 合并；核对至 main `56006502324800448c9775944d28478254be6142`，本模块相关实现未变，继续适用 `current-v1`。这不代表已部署或通过真实服务验收。
- 人的长期负责人：未分配。原修复来自用户任务级委托，本轮用户授权主规格收敛；不据此授予长期所有权。
- 实现、验收与验证命令在主规格；范围外为业务 API 行为、生成器、鉴权策略调整和部署。
- 当前无须叠加的有效覆盖或未完成业务 Patch。合并后验证及当前环境缺口见 [PR #6 后续记录](../../../changelog/2026-10-17-governance-merged-next-plan.md)；[S1 记录](../../../changelog/2026-10-15-specs-current-baseline.md)保留首次收敛时点的事实，不承担当前交付状态。

<a id="lint-selection-patches"></a>
## 历史与吸收关系

[PATCH-20261013-lint-selection](patches/PATCH-20261013-lint-selection/change.md) 已 absorbed 到 current-v1 的 LINT-SELECT，LINT-META 与 LINT-READONLY 保持既有约束。

[observed-v0 原文快照](spec.observed-v0.md)仅描述修复前现状。[原试跑记录](../../../changelog/2026-10-13-ming-specs-pilot.md)保留当时证据；旧仓 SHA 不保证在新仓解析。不要重写旧回执来证明新候选，也不需要从这些历史资料拼接当前规则。
