---
document_kind: review
status: pass
delivery_issue: ming-iot#IKD4OR
code_repository: git:gitee.com/ming-industry/ming-iot
exact_head: fb04bf129d73261d4b7f892217aab57986f29661
related_documents:
  - AGENTS.md
  - CONTEXT.md
---

# Todo Gate 修复（普通完成不触发 Gate / 精确风险 Gate 保留）— 独立评审

**评审人：** ming-review-claude（独立评审 lane，非实现 Agent）
**日期：** 2026-09-07
**候选（两仓完整 Head，均已回读）：**
- 产品仓 ming-iot `fix/todo-gate-handoff-repair` = `fb04bf129d73261d4b7f892217aab57986f29661`（base `2c899b5e888af4b620c693b5c4bfde340ff42684`，单提交 "docs: gate decisions instead of ordinary todo completion"，仅 `AGENTS.md` +4/−1）
- 文档仓 ming-iot-rd-docs `fix/todo-gate-handoff-repair` = `af27185c1f4828b643b30ea0d32a416b6c482d61`（base `d5938a4f7c2ae3100261fe80198596e557cdde77`，单提交 "docs: align todo gates and independent review handoff"，`AGENTS.md` +3、`CONTEXT.md` +1/−1、`tests/test_repository_policy.py` +12）

**依据：** LoopX Agent Todo `todo_ae28a6a4cc28`（实施 Todo `todo_22db31896d4a` 延期等待本 Review 的同 Head 回执）。delivery_issue 沿用 `ming-iot#IKD4OR` 授权对齐 lane（同 `specs/2026-09-04-gitee-draft-pr-automation-and-rd-docs-authorization-alignment-design.md` 先例）。
**评审方式：** 独立文档 worktree（ming-iot-rd-docs `.worktrees/todo-gate-handoff-repair-review`，branch base = 候选 `af27185`）通读两仓完整差异；独立复跑文档测试与边界扫描。未修改任何实施文件，未 commit/merge/push（本 Review 文件除外），不替实施者收尾。

## 一、评审范围与锚定

改动面为纯策略文本 + 一条策略钉扎测试：产品仓 `AGENTS.md` 将"同一 Agent Todo 执行期间不得创建验收 User Gate…"替换为四条新策略（普通闭环、实施→Review→接续、阶段≠Goal 验收、宿主续跑单独核验）；文档仓 `AGENTS.md`/`CONTEXT.md` 同步对齐，并在 `tests/test_repository_policy.py` 新增 `test_todo_completion_preserves_review_and_decision_boundaries` 钉扎关键句。两仓 Head 均为评审者从对应 worktree `git rev-parse` 独立回读，与实施 Todo evidence 记录一致。

## 二、独立复跑证据（exact heads：code `fb04bf1` / docs `af27185`）

| 门禁 | 结果 |
|---|---|
| `python3 -m unittest discover -s tests`（docs fix worktree @`af27185`） | **Ran 64 tests — OK**（含新增钉扎测试；与实施者 `pytest=64_passed` 声明一致） |
| `loopx check --scan-path`（改动面 4 文件：两仓 AGENTS.md、CONTEXT.md、test_repository_policy.py） | **clean：errors=0, warnings=0，"public boundary scan clean: 4 files"** |
| `loopx check --scan-root`（docs fix worktree 全仓） | errors=10，全部位于 `archives/snapshots/2026-08-23/**`、`meta/DOC_REGISTRY.yaml`、`plans/2026-08-26*`、`plans/2026-09-04*`、`specs/2026-09-04*` —— 均非 `af27185` 触及文件，系既有告警 |
| `loopx check --scan-root`（产品 fix worktree 全仓） | errors=23，全部位于 `backend/**`、`deploy/profiles/**`、`docs/superpowers/**` —— 均非 `fb04bf1` 触及文件（该提交仅改 `AGENTS.md`），在 base `2c899b5` 上同样存在，系既有告警 |

结论：实施 Todo evidence 的三项声明 `pytest=64_passed`、`changed_scope_scan=PASS`、`full_scan=existing_findings` 全部独立复核成立。

## 三、专项核验（handoff 四项）

1. **普通完成无 Gate**：两仓均明确"普通 Agent Todo 在获批范围内完成验证及所需独立 Review 后，自行闭环并接续已授权工作"；产品仓进一步钉死"Plan Task/Step、checkpoint commit 和 Todo 完成本身均不触发 User Gate"，文档仓 CONTEXT.md 同步改写为"Todo 完成本身不触发 User Gate"。✅
2. **精确风险 Gate 保留**：两仓新策略均保留"只有新方向、范围变更、风险接受或下文的高风险授权边界需要用户决策时才建立 Gate；**已有精确 Gate 仍须满足**"；两仓 AGENTS.md 的高风险授权章节（产品 `## Git 与高风险授权边界`、文档 `## Git、Gitee PR 与高风险授权边界`）原文未动。✅
3. **Review 前不可接棒**：产品仓"接续 Todo 以 Review Todo 完成及**同一 Head 的有效回执**为前置条件；启动 Reviewer、声明完成或仅有 PASS 字样都不能替代回读；候选变更后重新验证并提交受影响范围的 Review"；文档仓"接续者回读同一 Head 的有效回执后才接棒""等待 Review 不关闭 Reviewer lane"。✅
4. **宿主激活不冒充已运行**：产品仓"LoopX 注册和 `should_run=true` 只是准入，不证明 Codex `/goal` 或 Claude `/loop` 已运行；从对应宿主读取激活状态，同一身份不得重复拉起…不用忙轮询冒充续跑"；文档仓"LoopX 准入与宿主续跑分别验证，同一身份不得重复拉起宿主"。✅

新增测试以子串钉扎上述关键句（"Todo 完成本身不触发 User Gate""同一 Head 的有效回执后才接棒""等待 Review 不关闭 Reviewer lane""已有精确 Gate 仍须满足""宿主续跑分别验证"），与本仓既有策略测试手法一致。

## 四、结论

**Verdict: PASS**（blocker：无；warning：无）。

- 四项策略语义全部落地且两仓一致；三项验证声明全部独立复现。
- 观察（不构成 warning，不阻断接续）：全仓边界扫描的既有告警（docs 10 / 产品 23，`public_boundary_violation` 类，多为历史 archives 与配置样例中的 credential 模式命中）先于本次改动存在，与本次改动面零交集，按仓库规则应关联独立修复事项处理，不属于本 Todo 范围。
- 本评审仅写入获准的 `reviews/2026-09-07-todo-gate-handoff-repair-review.md`（独立 worktree），未改实施文件、未 commit/merge/push、未替实施者收尾；实施 Todo `todo_22db31896d4a`（及其文档侧配对）可在回读本 Review 与两仓上述 Head 后按新策略接续。

**English verdict:** PASS — no blockers, no warnings. Both candidate heads independently re-read (code `fb04bf129d73261d4b7f892217aab57986f29661`, docs `af27185c1f4828b643b30ea0d32a416b6c482d61`); all four policy semantics verified in both repositories; 64/64 doc tests green; changed-surface boundary scan clean; all full-repo findings confirmed pre-existing and disjoint from the changed files.
