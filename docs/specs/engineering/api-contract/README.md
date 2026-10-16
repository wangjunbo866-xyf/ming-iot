# engineering.api-contract：HTTP API 契约校验

- 当前规则：[spec.md@current-v1](spec.md)，只覆盖 `apicheck lint` 的选择、元数据与只读退出契约。
- 适用源码：新仓 main `c165a56531f58421a870a0b8794f24556b60d3b2`；本轮文档为 S1 候选，不等于已合并或部署。
- 人的长期负责人：未分配。原修复来自用户任务级委托，本轮用户授权主规格收敛；不据此授予长期所有权。
- 实现、验收与验证命令在主规格；范围外为业务 API 行为、生成器、鉴权策略调整和部署。
- 当前无须叠加的有效覆盖或未完成业务 Patch。新仓验证与候选状态见 [S1 记录](../../../changelog/2026-10-15-specs-current-baseline.md)。

<a id="lint-selection-patches"></a>
## 历史与吸收关系

[PATCH-20261013-lint-selection](patches/PATCH-20261013-lint-selection/change.md) 已 absorbed 到 current-v1 的 LINT-SELECT，LINT-META 与 LINT-READONLY 保持既有约束。

[observed-v0 原文快照](spec.observed-v0.md)仅描述修复前现状。[原试跑记录](../../../changelog/2026-10-13-ming-specs-pilot.md)保留当时证据；旧仓 SHA 不保证在新仓解析。不要重写旧回执来证明新候选，也不需要从这些历史资料拼接当前规则。
