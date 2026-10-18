---
status: draft
version: 0.1.1-draft
date: 2026-10-18
---

# LoopX 已纳管任务约定

仅在任务负责人明确选择 LoopX，或已接受的任务明确由其管理时读取本页。本页是工具专用补充，不是团队默认开发流程。共同规则在 [AGENTS.md](../../AGENTS.md)；普通任务不依赖此运行环境。已接受的纳管任务不能通过改称“普通开发”绕开已有身份、Todo、写范围或 Gate。

按本页[本机接入](#local-setup)定位已注册项目。缺配置时恢复或报告精确缺口，不重建 Goal、不复制他人 SQLite、不借用他人的 Agent 身份。当前试点复用同一 ming-iot-knowledge-pilot。

## 开始修改前

1. 读取当前状态：`tooling/loopx status --goal-id ming-iot-knowledge-pilot`。
2. 读取当前 LoopX Todo、写范围和用户 Gate，不依赖旧聊天推断任务。
3. 旧工作区 `iot-doc`、`things` 来源仓保持只读；当前产品实现位于本仓 `backend/things`，可按获批任务修改。

## 持久资料登记

- 需要跨 Chat 保留的设计、Proposal、研究资料、SOP、Evidence 或外部资料按 `loopx-doc-registry` 的正文映射与正式登记流程处理；宿主未安装该 Skill 时可读正式 CLI 帮助，保持相同登记边界。
- 固定顺序是：先在项目权威表面建立可持久读取的正文映射并验证正文可读，再 dry-run，通过 LoopX 正式登记来源（由 LoopX 保存脱敏指纹），执行 `refresh-state`，最后回读状态。
- 正文与项目文档目录不是 `.loopx/registry.json`；不得手工修改该 Registry 完成登记。
- Git 跟踪的项目文件在独立 worktree 修改；`.loopx/` 与 `.codex/` 是本机私有状态，只能由 LoopX 在已注册项目根维护，worktree 不创建第二份。
- 正文无法定位、目标 Goal 不明确或读取权限不明确时，停止登记并写入 Todo/Blocker。
- `source_ref_sha256` 是 raw locator 的脱敏摘要，不是文件定位器、正文内容哈希，也不是正式登记命令的 source 输入；没有可读取文档映射的记录不是完整登记。Authority 或文档目录变化后必须运行项目侧聚焦校验。

## 修改与完成

- 所有 Git 跟踪文件修改必须绑定当前 Todo，并在独立 worktree 内且符合获批的 write scope。
- `.loopx/`、`.codex/` 私有状态变更同样必须绑定当前 Todo 和获批边界，并只从已注册项目根通过 LoopX 执行。
- Agent Todo 按 Loop、阶段成果或需要独立交接的拓扑节点登记；实施计划中的 Task/Step 只记录在 Plan 与 Evidence 中，不得默认逐项转成 Agent Todo。只有跨仓库、跨负责人或独立延期交接时才拆分新的 Agent Todo。
- 普通 Agent Todo 在获批范围内完成验证及所需独立 Review 后，自行闭环并接续已授权工作；Plan Task/Step、checkpoint commit 和 Todo 完成本身均不触发 User Gate。只有新方向、范围变更、风险接受或下文的高风险授权边界需要用户决策时才建立 Gate；已有精确 Gate 仍须满足。
- 实施 → Review → 接续：实施者提供不可变候选 Head、验证结果和精确交付物；独立 Reviewer 只写获准的 Review/Evidence，不顺手修改实施代码。接续 Todo 以 Review Todo 完成及同一 Head 的有效回执为前置条件；启动 Reviewer、声明完成或仅有 PASS 字样都不能替代回读。候选变更后重新验证并提交受影响范围的 Review。
- Todo/阶段完成不等于 Goal 验收完成。阶段结束核对剩余验收、后继 Todo 与授权边界，更新当前 Agent Vision；仅在真实终止或明确暂停当前 lane 时使用 `no_followup`，并说明恢复条件。等待 Review 不得关闭 Reviewer 的可运行 lane。
- 宿主续跑单独核验：LoopX 注册和 `should_run=true` 只是准入，不证明 Codex `/goal` 或 Claude `/loop` 已运行；从对应宿主读取激活状态，同一身份不得重复拉起。缺少宿主控制能力时报告一次精确恢复动作，不用忙轮询冒充续跑。
- 完成前运行与变更相关的项目测试和静态检查。
- LoopX 状态不替代 Git diff、测试结果或用户检查。


## LoopX 上下文与 Evidence 热路径

- 纳管任务先读当前准入契约、选中 Todo 与必要的有界证据；已有诊断包足够时不机械重复 diagnose/status/quota，命令以当前 CLI 帮助为准；不得默认将整份 `ACTIVE_GOAL_STATE.md` 加载为上下文。只有诊断状态漂移、修复投影或执行获批的状态迁移时才读取全文。
- Todo `evidence`、`note` 和 `reason` 只保存公开安全的简短结果或稳定证据指针，不内联完整 Review、原始日志、命令输出或大段分析正文。
- 完整、脱敏、供人阅读的 Review 写入本仓任务证据或 `docs/changelog/`；原始机器 Evidence 留在当前 Goal 的私有证据目录。旧 `docs/wiki/legacy/rd/reviews/` 仅保留历史。Todo 回执只保留 `verdict`、`exact_head`、`review_ref/evidence_ref`、摘要哈希与 blocker/warning 计数。

## 长期 Smoke、Monitor 与自修复

- 只长期保留能验证已发布行为、可复用控制面契约、安全边界、真实回归或代表性端到端路径的 Smoke；仅证明一次临时分析、候选排序或过渡结论的检查留在当次 Evidence，不扩大永久测试面。
- 项目特定的周期观察登记为 LoopX `continuous_monitor` Todo，并保存稳定目标、调度信息和简短 Evidence；宿主唤醒 Prompt 保持通用，不写死某个 PR、迁移或临时任务。
- `continuous_monitor` 只负责观察和报告状态变化，不自动授予代码修改、合并、发布或生产操作权限；发生实质变化时再创建或唤醒普通 Agent Todo。
- 出现 LoopX 投影矛盾、进展过小、状态漂移或用户指出流程错误时，暂停普通交付并执行 `loopx-self-repair`；修复落到最低且可持续的责任层，例如 Todo、边界投影、交互规则、项目文档或聚焦 Smoke，不依赖聊天记忆或降低 Gate。


## 独立委托与高风险动作

纳管任务可委托独立 Agent 在全新 clone 中按限定版本、写范围和验收工作。被委托者无需接管控制面；协调者负责将真实交付物和验证绑定原 Todo。复制维护者私有配置不是接手条件。

当前用户指令优先于旧阶段推荐；改变任务选择时先通过正式接口更新和重新准入。同一 Turn 的写回及结算按当前契约执行，不能把状态操作当产品验证。

Agent 合并需要绑定精确 PR、Head、目标分支和方式的 User Gate；版本发布、生产、凭据处理和权限变更及重写已发布共享功能分支同样需要精确 Gate。根入口的 Git、公开和保护边界始终适用。本页不修改调度器、身份、数据库或宿主自动化配置。


<a id="local-setup"></a>
## 本机接入

已有 `ming-iot-knowledge-pilot` 的维护机器继续使用同一 Goal、Agent 身份和 SQLite 数据库。经隔离验证及独立复审，已通过 LoopX 项目迁移入口把注册和 `.loopx/goals/` 状态目录迁入本仓；共享运行时、数据库与完整历史留在原位。此次采用固定本地候选，恢复时需使用支持该路由协议的运行包。具体证据与验收边界见 [M5 记录](../changelog/2026-10-15-m5-cutover.md)。

在 Git 主工作区创建被忽略的 `.local/loopx.json`，填写本机已有 CLI 和 Registry 的绝对路径：

```json
{
  "binary": "/absolute/path/to/loopx",
  "registry": "/absolute/path/to/registered/registry.json"
}
```

`tooling/loopx` 从 Git 公共目录定位该配置，独立工作树复用同一配置。它只转发参数，配置缺失即失败；不连接新项目、不建 Goal、不复制 Registry 或数据库。新同事不复制维护者的私有文件，应按自己获授权的 LoopX 项目完成接入；普通构建和知识检查不依赖 LoopX。

```sh
tooling/loopx --format json status --goal-id ming-iot-knowledge-pilot
```

Agent 的进展写回继续使用原准入身份和 Turn，并通过 `--delivery-workspace-path` 指定实际产出所在的独立工作树。该参数记录交付位置，不改变 Goal 注册路径。自动化使用稳定的主工作区入口，不指向临时工作树。

## 迁移恢复与历史保留

旧源码仓、原候选、私有备份继续保留，日常代码和文档改动在本仓进行。旧项目 Registry 已标记为退休，旧状态文件作为迁移前快照保留；当前写入由本仓注册位置承接。路由核验仍读取退休标记，部分历史材料仍引用旧来源，不能据此删除旧目录。

如新入口失效，先核对 `.local/loopx.json` 指向已审运行包，再验证同一 Goal 的身份、Todo 和历史。旧版本会拒绝新路由，不能将换回旧二进制当作恢复。迁移中断只能按保留的原计划和摘要，通过迁移入口重试；先停止调度并排空在途写入。代码回退使用新的修复 PR，不覆盖旧仓、不重写主分支、不从备份重建同名 Goal。迁移计划、原始回执和备份仅私有保存。

Codex Project 是工作区入口。创建或切换到 ming-iot Project 不需要创建第二个同名 Goal；当前对话可以继续使用上述稳定入口。明确继续该 Goal 的新聊天应读取现有 Goal，而不是重新初始化；普通开发聊天按通用入口开始。

小黄反馈及依赖该反馈的任务已停止并废弃。现有代码与文档仍可供开发者理解和验证，历史 AI 交接演练保持原结论，未记录真人验收通过。
