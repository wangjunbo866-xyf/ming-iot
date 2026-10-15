# ming-iot 直接使用 LoopX 与自定义 Harness 退役设计

> 日期：2026-08-24  
> 状态：已批准  
> 适用仓库：`ming-iot`  
> 目标：删除 `automation/loopx/` 自定义 Harness，直接使用 LoopX 原生控制面、仓库级 `AGENTS.md` 与项目测试。

## 1. 决策摘要

`ming-iot` 不再维护位于 `automation/loopx/` 的项目级自定义 Harness。

本次采用“直接使用 LoopX”方案：

- LoopX 原生管理 Goal、Todo、Gate、claim、lease、quota、长期状态与恢复；
- `AGENTS.md` 只保存少量稳定的仓库协作约定；
- 项目自身的测试、静态检查和未来 CI 判断代码是否正确；
- Git、按风险选择的 Review 与用户批准继续控制提交、合并、push 和发布；
- 不再增加替代校验器、包装命令或隐藏的小型 Harness。

此前文档中要求长期保留静态 Harness、`automation/loopx/`、Harness pre-push 和相关角色合同的内容，自本规格获批后降为历史设计。此前已经形成的 Evidence 与 Review 继续作为演进记录，但其中的 Harness 命令不再是当前操作入口。

本规格不推翻以下既有上位决策：

- LoopX 是 `ming-iot` 新工作的唯一动态研发控制面；
- `iot-doc` 继续拥有工业物联网架构与语义；
- `ming-iot-rd-docs` 继续保存本地研发规格、计划、Evidence 和 Review；
- `things` 上游仓库保持只读；
- `.graph` 与 graphctl 的退役仍按单独计划处理，不夹带到本次精简中。

## 2. 背景与问题

Phase B 在 `automation/loopx/` 下形成了十三个静态文件，其中 `check.py` 与 `test_check.py` 合计约 1800 行 Python。它同时维护项目合同、版本锁、权限目录、角色合同、Rules、Skills、MCP 策略和 pre-push 校验。

实践表明这层资产带来以下问题：

1. LoopX 不会因为目录名是 `automation/loopx/` 就自动读取其中所有 Rules、Skills 和 JSON 合同；
2. Agent 既要理解 LoopX 原生状态，又要理解另一套项目 Harness，形成重复入口；
3. 每次修改 Harness 都需要维护 verifier、测试、摘要和 Review，简单任务被放大；
4. pre-push 中的自定义校验增加等待时间，团队容易绕过；
5. 项目尚未完成首个真实 IoT 交付 Loop，过早固化通用 Harness 属于错误抽象。

因此当前阶段优先验证 LoopX 本身能否支撑真实开发，而不是继续建设 LoopX 外围框架。

## 3. 目标与非目标

### 3.1 目标

- 完整删除 `automation/loopx/`；
- 消除 `AGENTS.md`、Git Hook 和测试对该目录的依赖；
- 只保留一个动态任务事实源，即 LoopX 原生状态；
- 让代码正确性回归项目测试和未来 CI；
- 降低新成员、Agent 和模型切换时的认知成本；
- 保留明确、轻量且可复核的日常开发路径。

### 3.2 非目标

- 不修改或 fork LoopX；
- 不开发 `company-dev`、`check.py` 替代品或新的 Agent Runner；
- 不在本次删除 `.graph`、graphctl 或其它历史控制面代码；
- 不修改 IoT 业务代码；
- 不自动 commit、merge、push 或部署；
- 不把本地研发文档上传到 Gitee；
- 不把 `AGENTS.md` 描述成确定性机器门禁；
- 不为本次简单退役强制创建独立 Reviewer 或单独 Evidence 文档。

## 4. 职责边界

| 责任 | 唯一承担者 | 不再承担者 |
| --- | --- | --- |
| Goal、Todo、Gate、长期状态与恢复 | LoopX 原生控制面 | 自定义 JSON、`check.py` |
| 仓库级 Agent 协作约定 | 根 `AGENTS.md` | `rules/repository-policy.md`、项目角色 Skill |
| 编译与行为正确性 | 项目测试、静态检查、未来 CI | Harness 摘要与自定义 PASS 谓词 |
| 工作范围 | 当前 LoopX Todo 与用户批准 | 固定的 Harness 全局写范围 |
| 代码事实 | Git revision 与 worktree | LoopX 投影、研发文档 |
| 架构语义 | `iot-doc` | Harness 合同副本 |
| 提交、合并、push、发布授权 | 用户及组织流程 | LoopX、Agent、Hook |

`AGENTS.md` 是模型和 Agent 的协作入口，不具备独立机器强制力。必须机械判断的代码行为应写成项目测试或 CI；动态任务约束应写入 LoopX Todo/Gate，不再复制到静态 Harness。

## 5. 目标工作流

```text
用户提出目标
  -> LoopX 创建或更新 Goal / Todo / Gate
  -> Agent 读取 AGENTS.md、当前 Todo、Git 与必要架构文档
  -> Agent 在获批 worktree 和范围内修改代码
  -> 运行与变更相关的项目测试和静态检查
  -> 按风险执行独立 Review，或由用户检查 Source Control
  -> 用户分别决定 commit、merge、push 或发布
```

日常状态查询直接使用 LoopX 原生命令：

```bash
loopx status --goal-id ming-iot-framework-v1
loopx diagnose --goal-id ming-iot-framework-v1
loopx todo --help
loopx quota should-run
```

项目不提供上述命令的二次包装。

## 6. 删除与保留范围

### 6.1 删除

- `automation/loopx/**`；
- 当前仅调用 `automation/loopx/check.py` 的 `.githooks/pre-push`；
- `AGENTS.md` 中“读取静态合同”的入口及 Harness 标题；
- `.gitignore` 中只服务于 `automation/loopx/__pycache__/` 的规则；
- `backend/tools/internal/graphctl/changes/changes_test.go` 中只验证该 pre-push 文件及 Harness 调用的测试。

删除测试时只移除以下职责，不改动 changes 包的业务行为测试：

- pre-push 必须存在且为 `0755`；
- pre-push 必须调用 `automation/loopx/check.py`。

### 6.2 保留

- 被 Git 忽略的 `.loopx/registry.json` 与锁文件；
- 被 Git 忽略的 `.codex/goals/`；
- LoopX 安装、原生命令和原生 workflow skills；
- `AGENTS.md` 中稳定的只读仓库、授权和提交流程约定；
- `thingsbaseline`、Go 单元测试、`go vet` 及业务测试；
- `backend/tools/README.md` 的既有未提交 Graph 退役修改；该文件当前没有自定义 Harness 引用，本次不得改动；
- `ming-iot-rd-docs` 中的历史规格、Evidence 和 Review；
- 与本次无关的 `.graph`、graphctl 和业务源码，等待各自退役或交付计划。

## 7. LoopX 状态迁移

当前 LoopX 中仍存在“实现 Phase B 静态 Harness”的阻塞 Todo，以及等待 Harness checkpoint commit 的用户 Gate。选择本方案不等于批准该旧 checkpoint commit。

本规格书面获批后，必须通过 LoopX 原生命令完成以下状态迁移：

1. 将旧 Harness Todo 标记为被新决策取代，保留历史，不伪记为成功完成；
2. 将旧 checkpoint commit Gate 标记为不再适用，不解释成用户已授权 commit；
3. 创建“退役自定义 Harness”的有界 Todo，并绑定本规格与精确写范围；
4. 清理或更新仍引用 `automation/loopx/**` 的当前协调范围；
5. 验收后把下一动作切换到首个真实 IoT 交付 Loop，而不是继续扩展框架；
6. 不直接手工编辑 Registry JSON；若 LoopX 没有支持某项状态迁移的原生命令，则先记录明确的 LoopX 缺口，不为此重建 Harness。

书面规格获批后，本规格应作为脱敏的项目 authority source 登记到 `ming-iot-framework-v1`，不得把本地绝对路径写入公开文件。

## 8. 迁移步骤

1. 在现有隔离 worktree 中重新执行 `git status --short`，确认精确目标；
2. 记录 `automation/loopx/` 的最终文件清单，明确该目录当前未被 Git 跟踪；
3. 先调整 LoopX Todo/Gate，使旧 Harness 实施不再被误认为当前下一动作；
4. 删除 `automation/loopx/`；
5. 删除或精简 pre-push、AGENTS、ignore 和相关测试引用；
6. 运行聚焦校验和一次 `backend/tools` 全包校验；
7. 将测试摘要、变更结果和残余风险回写当前 LoopX Todo；
8. 展示 `git status --short`，等待用户检查；
9. 未经用户另行明确授权，不执行 `git add`、commit、merge 或 push。

`automation/loopx/` 当前是未跟踪目录，删除后不能通过 Git 恢复。本设计有意不创建另一份代码备份；已有研发文档只保存演进原因，不继续维护 Harness 源码副本。

## 9. 验收策略

### 9.1 结构验收

- `automation/loopx/` 不存在；
- 当前源码入口中不再存在 `automation/loopx`、`check.py`、`repository-policy.md` 或 `release-lock.json` 的有效引用；
- `.loopx/`、`.codex/goals/` 仍被 Git 忽略且未跟踪；
- `AGENTS.md` 不再要求 Agent 读取不存在的静态合同；
- `.githooks/pre-push` 不再承担 LoopX/Harness 生命周期判断。

### 9.2 LoopX 验收

- `loopx status --goal-id ming-iot-framework-v1` 可读取当前 Goal；
- `loopx diagnose --goal-id ming-iot-framework-v1` 不把旧 Harness checkpoint 当作下一动作；
- 旧 Harness Todo 与 Gate 以“被取代/不再适用”保留历史，而不是消失或伪记通过；
- 新 Todo 的范围只覆盖本次退役修改。

### 9.3 代码验收

- 先运行 changes 包的聚焦测试；
- 再运行一次 `backend/tools` 全包测试与 `go vet`；
- 本次不运行无关的全仓库 Race 测试，因为没有修改并发实现；
- 测试失败时只修复直接引用或真实回归，不引入新的包装层。

### 9.4 完成判定

只有以下条件同时满足，才能将退役 Todo 判为完成：

1. 删除和引用清理完成；
2. LoopX 原生状态已迁移；
3. 聚焦测试、全包测试与 `go vet` 通过；
4. LoopX Todo 已记录测试摘要、变更结果和残余风险；
5. `git status --short` 只有预期变更；
6. 用户已检查结果。

完成退役 Todo不自动授权 commit、merge 或 push。

## 10. 异常处理与回退

- LoopX 状态异常：使用 `loopx status`、`loopx diagnose` 和原生 Todo/Gate 命令诊断；不恢复 `check.py`。
- 测试失败：修复直接代码问题；不以 Harness PASS 代替项目测试。
- 原生命令缺失：记录具体 LoopX 缺口，评估上游修复或后续 Extension；当前阶段不 fork、不开发包装 CLI。
- 删除后发现确有必要规则：把该规则写入最小的 `AGENTS.md`、项目测试或 CI，按其本质选择一个位置，不整体恢复 Harness。
- tracked 文件回退：在用户批准下按精确 diff 恢复；不得使用破坏性整仓库重置。
- untracked Harness 源码：本次删除后不承诺恢复；如果未来确需类似能力，必须以新的具体需求重新设计。

## 11. 风险与接受的取舍

### 11.1 AGENTS 不是机器门禁

Agent 可能误读或忽略自然语言约定。项目接受这一边界：高风险行为仍由用户授权控制，可机械验证的代码行为逐步进入测试和 CI，而不是再建设通用规则引擎。

### 11.2 本地暂时没有托管 CI

在免费 Gitee 账号无法使用 CI 的阶段，项目依赖显式测试、Review 和用户检查。未来接入 CI 时直接复用项目测试命令，不恢复 Harness 摘要链。

### 11.3 LoopX 升级风险

动态状态只依赖 LoopX 公开命令和自身 Registry，不依赖项目私有包装协议。升级时优先使用 LoopX `doctor` 和发布说明验证兼容性。

### 11.4 规则资产暂不通用化

第一阶段先完成真实 IoT 交付。只有多次真实任务反复证明某条规则稳定、通用且值得机械化时，才考虑贡献 LoopX 上游、增加项目测试或在第二阶段公司平台中实现。

## 12. 设计结论

`ming-iot` 当前需要的是一个能持续推进真实业务交付的长任务控制面，而不是另一个需要持续开发和解释的 Harness 产品。

删除 `automation/loopx/` 后，项目形成三个清晰事实面：

```text
LoopX       = 当前任务与长程状态
Git + 测试  = 代码事实与确定性结果
AGENTS.md   = 最小仓库协作约定
```

这个边界更符合当前个人研发框架阶段，也为后续真实 IoT Loop 和公司级平台积累留下空间。
