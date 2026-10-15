---
document_kind: review
status: pass_with_warnings
delivery_issue: ming-iot#IKD4OR
code_repository: git:gitee.com/ming-industry/ming-iot
exact_head: def1ce15c010cd3c7d446299500227c758b42579
related_documents:
  - specs/2026-09-03-ming-iot-gitee-delivery-and-verification-profile-design.md
  - plans/2026-09-03-ming-iot-gitee-delivery-and-verification-profile.md
---

# ming-iot 交付与验证档位（IKD4OR / Gitee Draft PR #3）— 独立复审（修复后第二次评审）

**评审人：** ming-review-claude（LoopX 注册独立评审 lane）
**日期：** 2026-09-04
**exact head：** `def1ce15c010cd3c7d446299500227c758b42579`（分支 `feature/gitee-delivery-verification-profile`，base master@`eb13958829855cbafe0845b8f122302ed159cfd1`；两提交：`07620d3` 档位统一 + `def1ce1` 评审修复，合计 33 文件 +1,079/−477）
**前次评审：** 同名文件第一轮（head 07620d3）verdict=BLOCK（blocker 1：make verify-pr 确定性红）；本轮逐项复验。规格 `specs/2026-09-03-ming-iot-gitee-delivery-and-verification-profile-design.md`（SHA-256 `3029e6f7…`）与计划 `plans/2026-09-03-…-profile.md`（`86840e3d…`）已并入 rd-docs master（df3d9c1）为权威。
**评审方式：** 在全新独立 worktree（ming-iot `.worktrees/review-ikd4or-pr3` detached @def1ce15、rd-docs `.worktrees/ikd4or-pr3-rereview`）只读复审 base..head 全部差异并独立复跑全部档位。未修改产品实现，未 commit/merge/push（除本评审、配对 evidence 与 DOC_REGISTRY 登记外零写操作）。

## 一、评审范围与锚定

覆盖第一轮全部范围（根 Makefile 四档、`.workflow` 流水线、PR 模板、`things-baseline-contract`、四契约测试、AGENTS 指引、日志清理）+ 修复提交 `def1ce1`（23 文件：i18n 断言、broker/geoip 条件 skip、vet 修复、死代码清理、migration 摘要更新）。

## 二、独立复跑证据（exact head def1ce15）

| 命令 | 结果 |
|---|---|
| **`make verify-pr`**（context+diff-check+build+test+vet+contract-static+boundary-scan 全链） | **PASS，exit 0**（55 个 ok 包行；Kafka contract PASS digest `cc117b59…` 完好；repositorycontract 四契约测试 ok） |
| `make verify-fast VERIFY_SCOPE=tools` | PASS |
| `make contract-static`（verify-pr 内含） | PASS（apicheck 244/242/242 detached=2 + 2×rpcgen + things-baseline-contract） |
| `boundary-scan`（verify-pr 内含，TestRepositoryDeliveryBoundary） | PASS |
| 修复抽验 | i18n 断言带前缀修正（与实际 i18n 行为一致）；NATS/MQTT/geoip 环境驱动 `t.Skipf` + 跟踪 Issue `IKD8AN` + 测试隔离 reset；auth.go keyed 字段；eventLog/propertyLatest 取址免锁拷贝；StepFloat 期望按步长量化语义修正；死代码删除（areaID/deviceMsgEvent/userToken 等 89 行）不影响构建（build 步骤验证） |

## 三、前次 Blocker 复验

**B1（make verify-pr 确定性红）→ FIXED（已独立验证）**：vet 步骤在 ALL_PKGS 全绿（既有 lock-copy/unkeyed 全部修复）；test 步骤全绿（i18n 确定性断言修正、broker/geoip 缺环境时显式 skip 并关联缺陷 Issue IKD8AN——符合 AGENTS"既有失败保留真实状态并关联缺陷"守则）；全链 exit 0 由复审者在本 head 独立复跑确认。**新 PR→master blocking 门禁不再自锁，具备真实拦截能力。**

## 四、Warning 裁决

- **W1（CI 不再收集二进制 artifacts）→ ACCEPTED_FOR_PR_GATE（保留跟踪）**：本流水线定位为 PR 验证门禁，产物收集属发布流水线职责；待发布流水线设计时补齐，避免下游依赖断供。
- **W2（go-build 缓存路径假设 root runner）→ ACCEPTED**：Gitee Go 环境惯例；换 runner 用户仅缓存失效，非正确性风险。

## 五、Note（不阻塞）

- `local_share_migration_test.go` 摘要按测试修复诚实更新（`a3728a94…`，count 253）；**go.mod/go.sum 整体豁免在本分支延续**——这是 Kafka 周期评审已记录的 W 级遗留（窄化方案曾实现于未合并分支 `local-share-drift-allowlist`/`cffbbca`），超出本 PR 范围，建议随后续 share 维护任务恢复。
- 修复提交对 share 包测试的修改改变了上游归一化摘要 —— pinned digest 更新与实际文件一致（verify-pr 内 boundary/repositorycontract 测试通过即证）。
- 四契约测试为静态语义钉定，不覆盖运行时红绿；本轮门禁红绿由复审者实跑补足。

## 结论

**verdict = PASS_WITH_WARNINGS**（blocker 0 / warning 2（均接受并跟踪）/ note 3）

第一轮 BLOCK 的唯一 blocker 已按处方修复并经本复审独立全量复跑验证（`make verify-pr` exit 0）；四档机制、CI 单一入口、PR 模板、契约目标、AGENTS 守则全部成立。候选具备合并条件，交由单一汇总 User Gate（PR#3 合并审批）。

---

**English verdict:** PASS_WITH_WARNINGS. The sole first-round blocker (verify-pr deterministically red via pre-existing vet/test debt) is fixed at exact head `def1ce15` exactly as prescribed — mechanical vet fixes, an honest i18n assertion correction, and environment-gated skips linked to debt issue IKD8AN — and independently verified by a full `make verify-pr` run in this re-review (exit 0 across build/test/vet/contract-static/boundary-scan at the fresh worktree). W1 (dropped CI artifacts) and W2 (root cache path) are accepted with tracking. The PR is merge-ready pending the consolidated user gate.
