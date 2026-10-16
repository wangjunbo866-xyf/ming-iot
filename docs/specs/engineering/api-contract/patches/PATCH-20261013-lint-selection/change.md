---
status: absorbed
revision: R1
---

# PATCH-20261013-lint-selection：拒绝未实际选择文件的 lint 成功

- 类型：patch；原因：开发工具缺陷修复。
- 已吸收：[current-v1#lint-select](../../spec.md#lint-select)，LINT-META / LINT-READONLY 保持原约束；2026-10-15 用户授权 S1，原验收未变。
- 适用新仓源码：`c165a56531f58421a870a0b8794f24556b60d3b2`；文档候选与验证见 [S1 记录](../../../../../changelog/2026-10-15-specs-current-baseline.md)。
- 任务：`todo_3af9f9234500`；模块：`engineering.api-contract`。
- 基线：[observed-v0#LINT-SELECT](../../spec.observed-v0.md#lint-select)，代码来源 `da80f056941e5f5e2ea1de38538ea2a062be8853`。
- 变化：替换 `LINT-SELECT` 的文件选择规则；不变约束：[LINT-META](../../spec.md#lint-meta)、[LINT-READONLY](../../spec.md#lint-readonly)。
- 主规格反向入口：[模块入口](../../README.md#lint-selection-patches)，主规格相应位置也有回链。
- 依赖与并行：无前置 Patch；当前可见工作树和 LoopX 在途任务未发现相同代码范围的认领。不宣称已检查不可见团队任务。
- 下文为 2026-10-13 原试跑记录；其中尚未生效/未合并/待吸收和工具状态只适用于当时，不是当前状态。机器投影改用 v2，只从本文开头读取状态/revision；原回执不改写。

## 原始需求与确认

用户原话：“先从 ming-iot 选一个真实的小改动，用这套约定和 ming-specs 完整试跑一遍，重点验证模块认领、Patch 双向指针和提交差异提醒，再根据结果修订草案。”

2026-10-13 当前用户委托 AI 选择小改动并实施。本次选题是实际复现的 `apicheck lint` 对不存在文件返回成功。下面的验收与实现方案由 AI 在此授权范围内细化；不伪造用户对每条验收单独签字，也不把委托扩展为业务需求、合并或团队制度采纳。

## 本次认领

| 项目 | 记录 |
| --- | --- |
| 需求与本次范围责任 | 当前用户（授权来源：本会话上述指示）；长期模块负责人未分配 |
| 实施 | Codex；Agent `ming-specs-draft-codex`；分支 `codex/team-specs-draft` 的独立 worktree |
| 独立检查 | 独立 AI Reviewer，候选固定后执行；不冒充人的采纳或合并审批 |
| 前台、API | 无页面或业务 API 改动；唯一接口为 `apicheck lint --files` |
| 代码写范围 | `backend/tools/cmd/apicheck/main.go`、`main_test.go` |
| 文档与工具写范围 | 本模块目录、specs 导航、本次 changelog、团队约定、DOC_REGISTRY、`ming-specs` Skill 内资源 |
| 数据、权限与跨仓 | 无数据模型、权限或跨仓接口改动；只读业务 `.api` 与 golden |
| 完成条件 | 复现缺陷、最小修复、真实回归、三项协作验证、独立候选检查及草案修订 |
| 未决及阻塞范围 | 长期负责人、全团队采纳、合并未决定；不阻塞已授权的本地试跑 |

机器可检查的路径与指针见 [check.json](check.json)。该文件是本记录的操作投影，程序不据此验证人的身份、授权充分性或认领是否排他。

## 变更后的完整行为与验收

| ID | 行为与验收 |
| --- | --- |
| AC1 | 每个非空条目必须对应当前服务 HTTP 扫描目录下实际存在的普通 `.api` 文件；不存在、目录、非 `.api`、绝对路径或扫描范围外路径退出 2，指出输入，且不输出 `lint: ok` |
| AC2 | 空白/逗号列表退出 2；有效与无效条目混合时整体失败，不因有一个有效文件而成功 |
| AC3 | TrimSpace、规范化 `./` 与目录内 `..`、按规范路径去重；诊断过滤与成功计数用同一规范路径集 |
| AC4 | 合法且无元数据问题的文件继续成功；选中问题文件（包括带 `./` 的写法）仍失败；纯类型 `.api` 可以选择；保留 LINT-META |
| AC5 | 拒绝命令错误及成功 lint 均不改写输入、golden；编译后的 CLI 验证退出码，保留 LINT-READONLY |

## 技术取舍与验证安排

在 CLI 中先定位 Core 根、规范化文件选择并与实际扫描目录核对，再调用现有快照及元数据检查。只补选择层，不修改元数据语义、生成器和业务源码。重复项保留去重行为；不新增第三方依赖。

先增加 CLI 回归测试并记录旧实现失败，再实现修复。提交前比较暂存区真实差异与本 Patch；完整验证使用 `make verify-pr`。Hook/CI 暂未接入规格检查，不能把本次主动运行当成自动拦截。

最终结果、偏差处理、候选与外部评审定位见 [试跑报告](../../../../../changelog/2026-10-13-ming-specs-pilot.md)。生效及主规格吸收留待有相应决定与交付证据后处理。
