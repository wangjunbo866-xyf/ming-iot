# HTTP API 契约校验

- 模块 ID：`engineering.api-contract`；这是研发工具能力，不是 SaaS 业务域。
- 长期模块负责人：未分配。本次任务由当前用户委托，Codex 实施；不据此授予长期所有权。
- 代码：`backend/tools/cmd/apicheck/`；底层 `internal/apicontract/` 仅阅读。
- 基线：[spec.md](spec.md)，`observed-v0`，由既有代码、说明及实测重建，尚未整体批准为正式需求。
- 本次认领及候选：[PATCH-20261013-lint-selection](patches/PATCH-20261013-lint-selection/change.md)。
- 范围外：业务 API、鉴权语义、golden、生成器和部署；其他规格入口维持原状。

## 当前适用关系

<a id="lint-selection-patches"></a>

`LINT-SELECT` 的旧行为见 [基线条款](spec.md#lint-select)。
候选 [PATCH-20261013-lint-selection](patches/PATCH-20261013-lint-selection/change.md)
状态为 `confirmed`，拟替换该条；尚未标记 effective 或 absorbed。
`LINT-META`、`LINT-READONLY` 保持不变。本次无其他已生效覆盖。

本地候选的测试及评审见 [试跑报告](../../../changelog/2026-10-13-ming-specs-pilot.md)。
本地实现完成、合并、制度采纳和部署分别记录；不能用本入口的存在证明已经上线。
