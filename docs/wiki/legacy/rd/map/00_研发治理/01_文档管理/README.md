# 文档管理

本模块定义研发文档如何按业务地图组织、如何关联 Delivery Issue 与 PR，以及如何让人和不同 Coding Agent 可靠地找到当前有效信息。

> **当前状态：设计中。** 本模块尚无已生效的 `spec.md`。当前仓库规则仍以根目录 [AGENTS.md](../../../AGENTS.md) 及现有顶层交付目录为准。

## 模块主规格

先阅读 [spec.md](./spec.md)：它定义本模块完整要求，是首次实施与后续 Change 的规格基线。当前修订尚未合入生效；不能将“已有规格”理解为“能力已验收”。

## 活动 Change

| Delivery ID | 类型 | 名称 | 状态 | Change 入口 |
|---|---|---|---|---|
| `ming-iot-rd-docs#IKEOFY` | `FEATURE` | 业务地图文档管理首次落地 | Verifying | [打开](./changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/README.md) |

## 编写历史与生效边界

启动期先编写了 IKEOFY Change/Plan，后补模块主规格；这是本次补正的编写顺序问题，不是推荐流程。修订后的职责是“先定义模块主规格，再以 Change 组织实施和增量”。已有提交和 Review 保留真实历史，不倒改其当时依据。

本次仅修订文档层级，不宣称 IKEOFY 已生效，不执行旧文档迁移、其他阶段或合并。
