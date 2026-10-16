# Ming IoT 知识导航

先读 [AI 协作研发总纲](project-charter.md)，了解总目标、设计原则、阶段路线、当前进度和完成标准。需要理解业务时进入模块当前规格，追溯来源时再读历史材料。文件被迁入不等于其内容成为当前约定。

| 任务 | 入口 |
| --- | --- |
| 总目标、路线、进度与完成标准 | [总纲](project-charter.md) |
| 日常开发、工具与 LoopX 接入 | [开发入口](runbook/daily-development.md) |
| 查业务模块、主规格与在途变更 | [specs](specs/README.md) |
| 提出一句话需求，跟进到交付与接手 | [受理到交付流程与 harness](runbook/requirement-to-delivery.md)；[Kafka 迁移实例](specs/platform/eventbus/README.md) |
| 团队协作、认领、需求确认与提交检查 | [团队开发约定](runbook/team-development.md) |
| 详细设计讨论与方法来源 | [维护提纲](wiki/team-development-outline.md)；当前路线看总纲 |
| 当前仓库迁移决定 | [单仓迁移方案](architecture/monorepo-migration.md) |
| M1/M2/M3 证据和遗留 | [迁移记录](changelog/2026-10-15-monorepo-inventory.md) |
| 文档检查范围、报错与维护方法 | [检查说明](runbook/document-checks.md) |
| 正文归属与有效状态 | [文档来源索引](index.yaml) |
| 历史研发、设计和工具材料 | [历史材料导航](wiki/legacy/README.md) |
| 本地工具如何运行 | [工具说明](../tooling/README.md) |
| 隔离通知 RPC 与持久化复验 | [通知基线](runbook/notification-baseline.md) |
| SaaS 本地联调 | [本地环境](../backend/saas/doc/开发说明/本地联调环境.md) |
| 构建与部署验证 | [部署说明](../deploy/README.md) |

`ming-specs` 仍是已获准任务中的草案候选，不以目录存在推断团队已全面采用。r-doc、map-delivery 和旧分发 Hook 的实验结果保留；日常开发不再依赖旧资料仓的安装命令。
