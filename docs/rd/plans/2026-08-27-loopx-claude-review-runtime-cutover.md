# LoopX Claude Review Runtime 一次性切换实施计划

> **供 Agent 执行：** 本计划在当前会话内顺序执行；不创建产品代码提交，不处理 Share 迁移 Gate，不执行 merge 或 push。

**目标：** 将本机 Claude Code 作为 `ming-iot-framework-v1` 的可运行独立 Reviewer，由 LoopX 控制 Todo、身份、门禁、证据与交接。

**架构：** LoopX 继续作为唯一控制面；Codex 实现者和 Claude Reviewer 使用不同 Agent 身份。Claude Code 通过项目级 Adapter、LoopX MCP 和原生 `/loop` 执行，外层进程仍由用户可见的 Claude Code 终端持有。

**技术栈：** LoopX 0.5.2、Claude Code 2.1.241、Claude MCP、原生 `/loop`、Git 本地 exclude。

---

### Task 1：安装项目级 Claude Adapter

**文件：**

- 本地创建：`ming-iot/.claude/commands/loopx.md`
- 本地创建：`ming-iot/.claude/settings.json`
- 本地创建：`ming-iot/.mcp.json`
- 本地修改：`ming-iot/.git/info/exclude`

- [ ] 先运行项目级 `--harden --dry-run`，确认只影响 `ming-iot` 的 Claude 配置。
- [ ] 将 `.claude/` 与 `.mcp.json` 写入 `.git/info/exclude`，避免本机运行时配置进入项目提交。
- [ ] 执行项目级 Adapter 安装并重启提示检查。
- [ ] 运行 `claude mcp get loopx`，确认 LoopX MCP 已登记。

### Task 2：显式绑定 Claude Reviewer 身份

**文件：**

- 本地创建：`ming-iot/.claude/loop.md`

- [ ] 使用 `loopx agent-onboard --agent-type claude-code` 生成 `ming-review-claude` 的官方激活包。
- [ ] 在 `.claude/loop.md` 写入 `ming-iot-framework-v1` 与 `ming-review-claude` 的 armed marker，以及 `should_run → claim_task → review/verify → complete_task` 协议。
- [ ] 运行 Claude runtime profile 的 `quota should-run`，当前 Share User Gate 下预期得到 `should_run=false`、`operator_gate_notify`，证明门禁有效。

### Task 3：固化 Review 交接合同并验收

**文件：**

- 本轮不修改 Git 跟踪的项目策略文件；Review Todo 在后续实现 Todo 完成时由 LoopX 显式创建。

- [ ] 记录后继 Review Todo 合同：`action_kind=review`、`claimed_by=ming-review-claude`、`continuation_policy=independent_handoff`，并排除实现者。
- [ ] 规定 Review 输出为 `PASS/BLOCK/INCONCLUSIVE + blockers + verifier + handoff`；Review 完成后才创建一次 User Gate。
- [ ] 检查 `git status --short`，确认 Claude 本机配置未进入 Git 变更。
- [ ] 输出用户唯一手工动作：重启 Claude Code，在 `ming-iot` 根目录运行原生 `/loop`。

## 2026-08-27 运行时兼容性发现

- LoopX 0.5.2 Claude Adapter 假设裸 `/loop` 或 `/loop 10m` 会读取项目
  `.claude/loop.md`。
- 本机 Claude Code 2.1.241 的 `/loop` 实际合同是
  `/loop [interval] <prompt>`，底层创建 Cron；空 prompt 只输出 usage，不读取
  `.claude/loop.md`。
- 因此 Adapter、MCP、Reviewer 身份与 hardening Gate 已接通，但“原生 `/loop`
  自动驱动 Reviewer”尚未成立。不得将配置就绪误报为自动 Review 就绪。
- 后续只能二选一：等待/升级到兼容的 LoopX 官方 Adapter，或经用户另行批准，
  使用外层轻量 Host Runner 在调用 Claude 前先执行 `quota should-run`。
