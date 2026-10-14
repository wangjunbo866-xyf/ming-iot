# saas.notification-trigger：通知触发人展示

- 人的长期负责人：未分配；本次需求决定由当前用户作出。
- 入口采纳：仅 2026-10-14 获授权的局部试跑；不迁移整个通知领域权威。
- 当前材料：[observed-v0 现状基线](spec.md)，来源为实际代码与测试，无有效 Patch。
- 实现：`backend/saas/service/syssvr/internal/logic/notifymanage/triggerUserParams.go`。
- 调用：同目录 `appPush.go`、`sendMsg.go`；本次不修改通道、载荷、数据库或前端。

<a id="display-name-patches"></a>
## 当前候选

[展示名称优先级 Patch](patches/PATCH-20261014-display-name/change.md)：confirmed、R2；仅 manual 优先昵称。本地候选已实现，尚未达到 effective 条件；代码版本与验证结论以 Patch 和[试跑报告](../../../changelog/2026-10-14-ming-specs-handoff-pilot.md)为准，不覆盖主分支现状。交接记录在同目录 [handoff.md](patches/PATCH-20261014-display-name/handoff.md)。
