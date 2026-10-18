# ming-iot 协作入口

本文件是人和 Agent 的统一入口地图（≤120 行）。细节由链接指向的正文维护。
从这里按任务进入；子目录的更具体约定同时适用。
链接带章节锚点时，先读对应章节；确有依赖再展开，不默认通读整份规则。

## 按任务找入口

| 现在要做什么 | 去哪里 |
| --- | --- |
| 理解项目目标、路线和完成标准 | [总纲](docs/project-charter.md) |
| 理解或修改业务能力 | [业务地图](docs/specs/README.md) → Feature 入口 → 当前 spec；已知 Feature 可直接进入 |
| 维护规格、需求决定或检查差异 | [ming-specs](.agents/skills/ming-specs/SKILL.md)，按请求选择读取与写回路径 |
| 沉淀操作方法、维护入口或团队命令 | [ming-runbook](.agents/skills/ming-runbook/SKILL.md)，按任务维护所属正文与受影响导航 |
| 首次使用某个 Agent 或新 clone | [跨 Agent 接入](docs/runbook/cross-agent-development.md) |
| 准备开发、运行或测试环境 | [日常开发](docs/runbook/daily-development.md)、[部署入口](deploy/README.md) |
| 查架构、操作知识、外部来源或历史 | [知识导航](docs/README.md)、[来源索引](docs/index.yaml) |

## 仓库地图

| 位置 | 职责 |
| --- | --- |
| `backend/saas/` | SaaS 服务；具体业务归属看 Feature |
| `backend/share/` | 公共契约与基础能力 |
| `backend/things/` | 设备接入、管理及相关服务；Core/Share/Things 已合入本仓 |
| `docs/specs/` | 域、Feature 当前知识及单次变更 |
| `docs/architecture/`、`docs/runbook/` | 系统关系、共享设计及操作方法 |
| `docs/changelog/`、`docs/vendor/`、`docs/wiki/` | 演进证据、外部来源、讨论与历史 |
| `tooling/`、`.agents/skills/` | 团队检查工具与可选操作方法 |
| `deploy/` | 隔离环境与部署说明 |

## 快速命令

从当前仓库根运行；启动服务前按部署说明选择环境。

| 场景 | 命令 / 详情 |
| --- | --- |
| 查可用开发与验证入口 | `make help` |
| 文档及方法变更检查 | `make knowledge-check`；[检查范围](docs/runbook/document-checks.md) |
| 根入口或导航/命令变化检查 | `make agents-check`；[结果与失败处理](docs/runbook/document-checks.md#agents-local-check) |
| 普通代码 PR 验证 | `make verify-pr`；[CI 与验证层次](docs/runbook/monorepo-ci.md) |
| 检查实际提交 / PR 差异 | [完整差异入口](.agents/skills/ming-specs/references/pr-check.md) |

## 关键约定速查

| 触发条件 | 一句话约定 | 细节 |
| --- | --- | --- |
| 准备修改文件 | 先明确受影响 Feature、条款与修改范围，协调并行改动 | [认领与并行](docs/runbook/team-development.md#module-claim) |
| 理解或更新规则 | 当前 spec 完整描述自身业务；原因与历史按需追溯 | [知识完整性](docs/runbook/team-development.md#feature-knowledge-completeness) |
| 处理小改动 | 先判断约定行为是否改变，再选择最小记录与验证 | [小修改](docs/runbook/team-development.md#small-changes) |
| 编写实现、修复或重构 | 按适用范围用测试驱动实现，交付时完成独立审计；具体 Skill 自选 | [TDD 与证据](docs/runbook/team-development.md#tdd-scope)、[独立审计](docs/runbook/team-development.md#independent-audit) |
| 提交、交付或高风险操作 | 必读动作授权与公开边界；验证及独立评审绑定实际候选 | [交付边界](docs/runbook/team-development.md#delivery-boundaries)、[差异检查](docs/runbook/team-development.md#spec-code-check) |
| 留存决定与验证 | 在原任务/Issue/PR 记录范围、决定及实际结果 | [交付与接手](docs/runbook/team-development.md#handoff) |

## 当前状态入口

状态以链接正文的适用版本和证据为准，本页不维护第二份逐轮进度。

| 范围 | 当前入口 |
| --- | --- |
| 已有通知与 API 校验规则 | [通知 Feature](docs/specs/saas/notification-trigger/README.md)、[API 校验 Feature](docs/specs/engineering/api-contract/README.md) |
| Eventbus 演进与协议适配候选 | [Eventbus](docs/specs/platform/eventbus/README.md)、[上行 Feature](docs/specs/protocol/uplink-normalization/README.md) |
| 知识体系试点及遗留项 | [总纲的阶段与验收](docs/project-charter.md)、[团队约定适用范围](docs/runbook/team-development.md#adoption-scope) |
| 迁移依据与历史材料 | [日常入口切换记录](docs/changelog/2026-10-15-m5-cutover.md)、[历史导航](docs/wiki/legacy/README.md) |
