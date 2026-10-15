# 业务地图文档（设计试点）

`map/` 用业务地图和功能模块组织研发文档，使人和 Agent 从业务入口找到当前规格与相关交付记录。

> **当前状态：设计试点。** 在 `ming-iot-rd-docs#IKEOFY` 获批并完成工具、门禁和迁移前，仓库现有的 `specs/`、`plans/`、`reviews/`、`evidence/` 等目录仍是当前权威；不得把本目录当作已生效的新规范。

## 结构约定候选

```text
map/
├── 00_研发治理/
│   └── 01_文档管理/
├── 01_saas/
└── 02_things/
```

业务域下的每个功能模块最终包含：

```text
<功能模块>/
├── README.md
├── spec.md
├── adr/
├── references/
└── changes/
    └── <TYPE>-<Delivery-ID路径形式>_<名称>/
```

- `README.md`：模块边界、当前规格入口和 Change 索引。
- `spec.md`：模块已经生效的完整规格。
- `adr/`：该模块已经生效且长期有效的决策记录。
- `references/`：可被多个 Change 复用的外部依据和研究材料。
- `changes/`：按 Delivery 保存功能、补丁、缺陷修复和退役的语义变更；它不是 Git commit 日志。
- 模块根目录不设置 `plan.md`；实施计划只属于一次具体 Change。

完整规则正在以下 Change 中设计：

- [FEATURE-ming-iot-rd-docs-IKEOFY：业务地图文档管理](./00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/README.md)
