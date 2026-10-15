# Scene Runtime 调度、分片与故障转移规范性分章 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Scene Runtime 的 Manager、partition ownership、Actor lane、per-partition completion、state-before-event restore、graceful revoke、硬故障与 Zombie 重连协议，从主规格中的分散段落迁入一篇可独立阅读的规范性分章，并同步全部活跃规范的权威关系、索引与故障验收。

**Architecture:** 主规格继续拥有产品边界、wire、Binding/SceneBindingState 业务语义、求值、Action Outbox 事务、route/hash、容量与总体取舍；新分章唯一拥有 Runtime 调度协作协议。每个 Kafka TopicPartition 对应独立 `PartitionRuntimeContext`，其 token、restore gate、completion、Actor/timer/in-flight index 与生命周期不得和同进程其他 partition 混用。V1 只采用进程内 best-effort fencing 与稳定 `effectId` 幂等托底，不新增 durable owner epoch、lease、Kafka transaction、Actor snapshot 或第三方 Actor framework。

**Tech Stack:** Markdown、Mermaid `sequenceDiagram` / `stateDiagram-v2`、GFM 相对链接、Bash、`rg`、`awk`、Git worktree、Node.js 20、`@mermaid-js/mermaid-cli@11.4.2`、`markdown-link-check@3.13.7`。

---

## File map

**Create**

- `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md`：Manager、partition、Actor lane、completion、restore、failover、Zombie 和 best-effort fencing 的唯一规范正文。

**Modify**

- `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`：新增规范性分章索引；把单文件权威改为“主规格总纲 + Runtime 调度分章”规范集。
- `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`：保留现有标题/稳定锚点，迁出完整调度算法，只留业务语义摘要与唯一正文链接。
- `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md`：更新权威声明和按问题下钻入口。
- `亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md`：更新阅读卡、权威声明和 scheduling/ownership 交叉链接；不移动 matching 正文。
- `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`：更新阅读卡、Actor/partition 术语归属与链接；不复制状态机。
- `亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md`：新增多 partition、硬故障、Zombie、恢复中 revoke、timer activation 和 maintenance gate 故障验收。
- `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`：平台级 Scene 规范入口改指两份架构源，并把完整 completion 算法下沉为摘要链接。
- `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md`：逐项迁移 Scene restore/completion/rebalance 的直接引用；保持 §7.2 客户端基线不变。
- `亿级点位高吞吐架构/6.规则引擎与场景联动/附录-竞品状态方案调研.md`：只更新顶部导航和规范边界。
- `亿级点位高吞吐架构/6.规则引擎与场景联动/附录2-thingsboard规则引擎与CF实现分析.md`：只更新顶部导航和规范边界。

**Read but do not modify**

- `docs/superpowers/specs/2026-08-09-scene-runtime-scheduling-partition-failover-chapter-design.md`：已书面通过的设计输入；不得把它链接成运行时规范。
- `docs/superpowers/specs/2026-08-04-scene-temperature-alarm-e2e-flow.html`：主工作树中已有用户修改，不得修改、暂存或提交。
- `docs/superpowers/specs/2026-08-07-scene-canonical-fact-projection-e2e-sequence.html`：主工作树中已有用户修改，不得修改、暂存或提交。
- `.superpowers/`：主工作树中已有用户未跟踪目录，不得修改、暂存或提交。
- `things/`、`thingsboard/`：本任务只改文档，不改产品代码或第三方源码证据。

## Frozen decisions used by every task

- Kafka partition 是消费 ownership、Actor state/timer ownership、restore、completion 与故障转移单位；它不是 Actor。
- 一个 Runtime 可以同时承载多个 partition，但每个 partition 的 token、status、restore boundary/gate、completion sequencer、Actor/timer/in-flight index 和 SubjectContext cache 必须隔离。
- Actor 只表示 `(tenantCode, evaluationSubjectKey)` keyed serial lane；不承诺一 Actor 一 goroutine、独立 mailbox、远程协议、supervision tree 或 snapshot。
- 硬故障转移由 Kafka Consumer Group Coordinator 发起；原 Runtime A 不参与也不构成新 owner 激活前提。
- Zombie A 恢复网络后必须先失效旧 token，再以新 group session rejoin；重新拿到同一 partition 也必须用新 token 完整 restore，不能复用旧 Actor/timer/offset sequencer。
- `NormalEffectGate = capturedToken == currentToken && status == ACTIVE && partition 仍属于 current assignment`。
- `RESTORING` 只允许 `RestoreMaintenanceGate` 下的隔离与纯 tombstone；禁止普通求值、非 tombstone state、Linked、Action Outbox 和 timer progress。
- 新 owner 捕获一次 bounded restore cut `S0`，只恢复 `[start,S0)`；达到 `S0` 后复验 assignment/token，失权即丢弃全部 staging。
- 已 due timer 必须进入不可丢弃的 activation queue；timer 安装完成后才打开 restore gate 和 unpause `scene_event`。
- V1 best-effort fencing 只能阻止 Runtime 已知丢权后的新副作用，不能撤销已进入 Kafka Producer/PG 或已提交的请求。
- 稳定 `effectId` 收敛同一逻辑动作的重复；它不能证明迟到旧 `scene_state` 不会覆盖新 state，也不能把状态回退错误变成幂等成功。
- Kafka 客户端只继承 Kafka 规范 §7.2 已冻结的 `franz-go v1.21.5 + classic group + cooperative-sticky`；本任务不新选 assignor、consumer protocol 或线程模型。
- 恢复 ADR、容量 POC、故障注入和 SLO 仍是“未冻结”；文档拆分本身不等于证据通过。
- 本次权威切换必须原子：Tasks 2–9 只做阶段 RED/GREEN 检查，不 stage、不产生部分规范 commit；Task 10 对十一份正式 Markdown 的完整 staged diff 做双重审查后只创建一个 implementation commit。该约束优先于通常的“每小任务提交”，因为任意中间提交都会留下可检出的双真源或断链权威。
- 新分章每个 H2 前使用稳定的显式 HTML anchor；跨文档链接统一使用
  `Scene%20Runtime%20调度、分片与故障转移.md#runtime-*`，不依赖中英文混排标题自动生成的 GFM slug：

  ```text
  runtime-scope
  runtime-model
  runtime-components
  runtime-partition-isolation
  runtime-normal-completion
  runtime-state-machine
  runtime-restore
  runtime-failover
  runtime-best-effort
  runtime-partition-routing
  runtime-observability
  runtime-main-spec-index
  ```

### Task 1: Create an isolated clean worktree and freeze the red baseline

**Files:**

- Read: `docs/superpowers/specs/2026-08-09-scene-runtime-scheduling-partition-failover-chapter-design.md`
- Read: all files in the File map
- Do not modify any formal document in this task

- [ ] **Step 1: Use the required worktree skill and create the feature worktree**

  Invoke `superpowers:using-git-worktrees`, then run from the primary `iot-doc/` repository:

  ```bash
  set -euo pipefail
  repo='/home/wangjunbo/workspace-iot-platform/iot-doc'
  worktree="$repo/.worktrees/scene-runtime-scheduling-partition-failover"
  branch='docs/scene-runtime-scheduling-partition-failover'
  plan='docs/superpowers/plans/2026-08-09-scene-runtime-scheduling-partition-failover-chapter-implementation.md'

  git -C "$repo" ls-files --error-unmatch "$plan"
  git -C "$repo" diff --quiet HEAD -- "$plan"
  git -C "$repo" check-ignore -q .worktrees
  test ! -e "$worktree"
  test -z "$(git -C "$repo" branch --list "$branch")"
  git -C "$repo" worktree add "$worktree" -b "$branch"
  cd "$worktree"
  test -z "$(git status --porcelain)"
  task_base_sha="$(git rev-parse HEAD)"
  test -n "$task_base_sha"
  base_record='/tmp/scene-runtime-scheduling-partition-failover-base-sha'
  printf '%s\n' "$task_base_sha" > "$base_record"
  test "$(wc -l < "$base_record")" -eq 1
  printf 'TASK_BASE_SHA=%s\n' "$task_base_sha"
  ```

  Expected: the approved implementation plan is already tracked and committed before worktree creation, `.worktrees/` is ignored, the new worktree is clean, and one 40-hex `TASK_BASE_SHA` is printed and stored in the exact `/tmp` record. If the plan is not tracked/clean, stop before creating any branch/worktree. Preserve the printed SHA in the execution log as a second copy and use it in Task 10; do not derive the final range from `HEAD~N`.

- [ ] **Step 2: Verify the approved design is in the baseline**

  Run in the feature worktree:

  ```bash
  set -euo pipefail
  git merge-base --is-ancestor 2b0969be5f7da8d4cf0dcccb6dcb87bf26ccbdc5 HEAD
  test -f docs/superpowers/specs/2026-08-09-scene-runtime-scheduling-partition-failover-chapter-design.md
  test -f docs/superpowers/plans/2026-08-09-scene-runtime-scheduling-partition-failover-chapter-implementation.md
  git ls-files --error-unmatch \
    docs/superpowers/plans/2026-08-09-scene-runtime-scheduling-partition-failover-chapter-implementation.md
  test "$(rg -c '^## (3\. 规范权威关系|7\. PartitionRuntimeContext|10\. Assignment 与 state-before-event restore|12\. V1 best-effort fencing 与幂等边界|14\. 文档迁移策略|16\. 验收条件)$' \
    docs/superpowers/specs/2026-08-09-scene-runtime-scheduling-partition-failover-chapter-design.md)" -eq 6
  ```

  Expected: exit 0; all six approved design sections are present.

- [ ] **Step 3: Prove the new normative chapter and index do not yet exist**

  Run:

  ```bash
  set -euo pipefail
  scene_dir='亿级点位高吞吐架构/6.规则引擎与场景联动'
  target="$scene_dir/Scene Runtime 调度、分片与故障转移.md"

  test ! -e "$target"
  if rg -n 'Scene(%20| )Runtime(%20| )调度、分片与故障转移\.md' "$scene_dir/README.md"; then
    exit 1
  fi
  ```

  Expected: exit 0 and no output from `rg`.

- [ ] **Step 4: Prove the old single-source statements and full algorithms are still present**

  Run:

  ```bash
  set -euo pipefail
  scene_dir='亿级点位高吞吐架构/6.规则引擎与场景联动'
  main="$scene_dir/场景联动 Runtime 主规格总纲.md"

  authority_files=(
    "$scene_dir/README.md"
    "$main"
    "$scene_dir/00-15分钟读懂场景联动.md"
    "$scene_dir/控制面与匹配契约.md"
    "$scene_dir/算子语义与术语.md"
    "$scene_dir/SLO故障测试与实施.md"
    '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'
  )
  for marker in \
    '唯一.*规范性架构决策源' \
    '当前规范性算法只存在于场景主规格'
  do
    rg -n "$marker" "${authority_files[@]}"
  done

  for marker in \
    '读取对应 scene_state compacted partition 到恢复边界' \
    '完成 state partition restore' \
    '再开放对应 scene_event partition'
  do
    sed -n '/^### 9\.1 /,/^### 9\.2 /p' "$main" | rg -n "$marker"
  done
  for marker in '全部候选' 'completion sequencer' '连续 next-offset'; do
    sed -n '/^### 13\.2 /,/^### 13\.3 /p' "$main" | rg -n "$marker"
  done
  for marker in 'localPartitionGeneration' 'bounded drain' '旧 owner state send'; do
    sed -n '/^### 13\.3 /,/^## 14\./p' "$main" | rg -n "$marker"
  done
  ```

  Expected: every command returns matches. This is the RED evidence: authority is still single-file and the full restore/completion/rebalance algorithms still have a second future owner location.

- [ ] **Step 5: Record the allowed diff scope before editing**

  The only allowed implementation files are the eleven formal Markdown files listed under Create/Modify. The approved design and this plan are baseline history, not implementation targets. Every commit must use explicit pathspecs; never run `git add .`.

### Task 2: Create the normative chapter foundation and normal execution path

**Files:**

- Create: `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md`
- Read: `docs/superpowers/specs/2026-08-09-scene-runtime-scheduling-partition-failover-chapter-design.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`

- [ ] **Step 1: Create the authority header and sections 1–2**

  Create the file with these exact top-level headings:

  ```markdown
  # Scene Runtime 调度、分片与故障转移

  > **规范地位**：本文与[场景联动 Runtime 主规格总纲](场景联动 Runtime 主规格总纲.md)
  > 共同组成 Scene Runtime 的规范性架构决策源。本文唯一拥有 partition ownership、Manager/Actor
  > 执行、PartitionRuntimeContext 状态机、completion、restore、rebalance 与 Zombie 协议；主规格
  > 唯一拥有产品边界、wire、业务状态/求值、Action Outbox 事务、route/hash、容量与总体取舍。
  > 两者无“后写覆盖”关系；冲突必须在同一次变更中联合修订。
  >
  > **恢复 ADR：未冻结。** 本章定义待实施和验证的协议，不表示恢复 RTO、compaction 行为、
  > Zombie 故障注入或容量证据已经通过。

  <a id="runtime-scope"></a>

  ## 1. 范围、权威关系与不承诺项

  <a id="runtime-model"></a>

  ## 2. 一分钟模型与稳定身份
  ```

  In §1, state the exact ownership split from the header and include a compact “本文拥有 / 主规格拥有 / 从属契约拥有” table. List all non-promises: no durable DB epoch/lease, Kafka transaction/EOS, cross-store atomicity, Actor framework, Actor snapshot, partition-to-machine pinning, or frozen restore SLO.

  In §2, include this exact hierarchy and explain every edge:

  ```text
  Kafka Consumer Group
  └─ scene_event TopicPartition
     └─ PartitionRuntimeContext[p]
        └─ Actor lane (tenantCode, evaluationSubjectKey)
           └─ SceneBindingState (tenantCode, bindingId, evaluationSubjectKey)
  ```

  Explicitly distinguish `sceneTopicGeneration`, Consumer Group session, `localPartitionGenerationToken`, Actor key, BindingState key, `transitionId/effectId`, and `timerFireId`; none may be described as interchangeable generations.

- [ ] **Step 2: Add sections 3–4 for responsibilities and multi-partition isolation**

  Add exactly:

  ```markdown
  <a id="runtime-components"></a>

  ## 3. Manager、PartitionRuntimeContext、Actor lane 与 BindingState

  <a id="runtime-partition-isolation"></a>

  ## 4. 同一 Runtime 的多 partition 隔离
  ```

  §3 must freeze these responsibilities:

  - Consumer Group Coordinator assigns TopicPartitions; it does not manage BindingState.
  - `PartitionRuntimeContext[p]` owns lifecycle/token/restore/completion and the indexes needed to enumerate only `p` resources.
  - Manager validates `ProcessingContext`, reads one complete ProjectIndex view, performs matching and target bundle aggregation, but is neither a third Runtime nor a durable owner.
  - Actor lane serializes all events/timers for one evaluation subject; a lane may contain multiple independent BindingStates.
  - BindingState is business state, not a Go object snapshot; only its schema-defined value enters `scene_state`.

  §4 must include this exact semantic table:

  | 进程内可共享 | 必须按 TopicPartition 隔离 |
  | --- | --- |
  | ProjectIndex、编译产物 | status、token、group session relation |
  | worker pool、scheduler implementation | restore boundary/gate、completion sequencer |
  | Kafka Producer、PG pool | Actor/BindingState、timer、in-flight reverse index |
  | serializer、script runtime | input position、partition-local SubjectContext cache |
  | process quota/metrics exporter | revoke/close/activation scope |

  State explicitly that B owning `p0` and `p1` does not merge state; revoking `p0` must not close `p1`. Even when both are local, cross-entity targets still use terminal `LINKED_DELTA`; only identical source/evaluation subject keys may use local delivery.

- [ ] **Step 3: Add §5 normal dispatch, state/action ordering, timer return, and completion**

  Add:

  ```markdown
  <a id="runtime-normal-completion"></a>

  ## 5. 正常调度与完成边界
  ```

  The normal path must be an ordered list with this order:

  1. record enters the matching `PartitionRuntimeContext` carrying topic/partition/offset and captured token;
  2. Manager validates context and completes matching/bundle aggregation in one ProjectIndex read view;
  3. same-subject target enters its lane, cross-subject target is routed through Kafka to the target partition;
  4. lane applies one complete bundle and evaluates each Binding at most once;
  5. every state-advancing branch first serializes/validates tentative state and the 32 KiB quota;
  6. only an action branch inserts all Action Outbox rows in one PG local transaction; no-action state/FSM changes skip PG;
  7. after validation and, when applicable, PG commit, publish in-memory state/timer and perform one bounded best-effort `scene_state` handoff attempt;
  8. the partition-local completion sequencer advances only contiguous completed next-offsets.

  Reference main-spec §12.3 as the sole Action Outbox transaction definition; do not recreate its DDL or `effectId` formula here.

  Then reproduce the complete input completion predicate from approved design §9, including candidate/final-failure, actual Outbox commit/failure policy, Actor state/timer publication, all non-state pack callback/retry/quarantine, `scene_state` serialization + one bounded handoff attempt, and contiguous earlier next-offset. State explicitly that completion does not mean Webhook/notification/DeviceCommand external execution and is not PG/Kafka atomicity.

  Timer callbacks must capture `{TopicPartition, token}` and return to the current subject lane; current Binding/current-slot/business identity checks remain owned by the main spec and operator contract.

- [ ] **Step 4: Verify sections 1–5 and commit**

  Run:

  ```bash
  set -euo pipefail
  target='亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md'

  for heading in \
    '## 1. 范围、权威关系与不承诺项' \
    '## 2. 一分钟模型与稳定身份' \
    '## 3. Manager、PartitionRuntimeContext、Actor lane 与 BindingState' \
    '## 4. 同一 Runtime 的多 partition 隔离' \
    '## 5. 正常调度与完成边界'
  do
    test "$(rg -Fxc "$heading" "$target")" -eq 1
  done

  for anchor in \
    runtime-scope runtime-model runtime-components \
    runtime-partition-isolation runtime-normal-completion
  do
    test "$(rg -Fxc "<a id=\"$anchor\"></a>" "$target")" -eq 1
  done

  for marker in \
    'Kafka Consumer Group' \
    'PartitionRuntimeContext\[p\]' \
    'SceneBindingState' \
    '32 KiB' \
    'completion sequencer' \
    '连续.*next-offset' \
    '不表示.*Webhook'
  do
    rg -n "$marker" "$target"
  done
  git diff --check
  test -z "$(git diff --name-only)"
  git diff --cached --quiet
  untracked="$(git -c core.quotePath=false ls-files --others --exclude-standard)"
  test "$untracked" = "$target"
  ```

  Expected: every heading occurs once; all invariant markers match; only the target file is new.

  Do not stage or commit yet. A partially written file already declares normative ownership, so Tasks 2–9 must remain one atomic working-tree change and be committed only after Task 10 review.

### Task 3: Add the per-partition state machine and bounded-cut restore protocol

**Files:**

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md`
- Read: main-spec §§8–10 and approved design §§7–10

- [ ] **Step 1: Add §6 with the exact runtime context and two gates**

  Add:

  ```markdown
  <a id="runtime-state-machine"></a>

  ## 6. PartitionRuntimeContext 状态机
  ```

  Define the in-process context with all of these fields and no wire/schema implication:

  ```text
  PartitionRuntimeContext {
    sceneTopicGeneration
    topicPartition
    status
    localPartitionGenerationToken
    restoreBoundary / restoreGate
    completionSequencer
    actorIndex / timerIndex / inFlightIndex
    groupSessionToken
  }
  ```

  Include both exact gates:

  ```text
  NormalEffectGate =
    capturedToken == currentToken
    && partitionStatus == ACTIVE
    && partition 仍属于 current assignment

  RestoreMaintenanceGate =
    RESTORING
    && capturedToken == currentToken
    && partition 仍属于 current assignment
    && output 仅为坏 key 隔离或纯 tombstone
  ```

  Explicitly prohibit old tasks from re-reading and borrowing a new token. Data-plane manager/per-state timers must belong to a target partition and capture its token; a process-global callback pool cannot bypass the gate.

- [ ] **Step 2: Add the standard `stateDiagram-v2`**

  Insert this diagram in §6 and explain transitions below it:

  ```mermaid
  stateDiagram-v2
    [*] --> DETACHED
    DETACHED --> RESTORING: assigned / create new token
    RESTORING --> ACTIVE: restore to S0 / activate
    RESTORING --> RESTORING: retry or quarantine recoverable key
    RESTORING --> SUSPENDED: assignment or session uncertain
    ACTIVE --> REVOKING: graceful revoke
    ACTIVE --> SUSPENDED: lost, fenced, or session uncertain
    REVOKING --> DETACHED: bounded drain and close
    SUSPENDED --> DETACHED: destroy old context
  ```

  Freeze `DETACHED`, `RESTORING`, `ACTIVE`, `REVOKING`, `SUSPENDED`. `REVOKING` and `SUSPENDED` invalidate the token before drain/rejoin/cleanup. Only `ACTIVE` passes `NormalEffectGate`.

  Map franz-go integration exactly:

  - `OnPartitionsRevoked(p)` → affected contexts enter `REVOKING`, token invalidated first;
  - `OnPartitionsLost`, group-session context cancellation, member/generation fencing, or consumer close/recreate → affected contexts enter `SUSPENDED`, token invalidated first;
  - `OnPartitionsAssigned(p)` → create a new token and `RESTORING` context only;
  - `RESTORING` 必须持续维持 Consumer Group 的 `poll/heartbeat`，只暂停受影响 partition 的普通消息 dispatch；
  - callbacks do only atomic state/token change, pause and work registration; drain/restore/Actor rebuild occur outside callbacks;
  - 完整 restore、drain 和 Actor 重建不得阻塞 Consumer Group 协调循环；
  - callback adapter 建立先行关系：revoke/lost 的 token 失效必须先于后续 `OnPartitionsAssigned` 处理；自动 rejoin 不得沿用旧 token；
  - ordinary broker request failure alone does not prove ownership lost; that observation gap remains best-effort risk.

- [ ] **Step 3: Add §7 with the complete restore cut and activation order**

  Add:

  ```markdown
  <a id="runtime-restore"></a>

  ## 7. Assignment 与 state-before-event restore
  ```

  Use one numbered algorithm containing all of these indivisible steps:

  1. wait until full config cache and ProjectIndex are ready;
  2. create a new token/context in `RESTORING`;
  3. capture current available start and fixed end `S0`, then read exactly `[start,S0)` until next-offset reaches `S0`;
  4. validate each state against current BindingCtx into staging; isolate corrupt keys; deleted/disabled/incompatible state does not enter staging and may emit only a pure tombstone through `RestoreMaintenanceGate`;
  5. construct Actor transient objects but collect timer descriptors only; membership-dependent items set `scopeReconcileRequired` and have zero effects before current-scope reconciliation;
  6. at `S0`, recheck assignment and token; if either changed, discard the complete staging/context and restart from a newly captured cut;
  7. in the activation critical section publish staged Actor/state, install future timers, put already-due timers into a non-droppable activation queue, switch to `ACTIVE`, enqueue due timers to their lanes with the new token, and only after timer installation open `restoreGate`/unpause `scene_event`.

  State that `[start,S0)` is a bounded restore view, not an endlessly moving tail. Records arriving after `S0`, including an old owner's late state, are not pulled into this staging and remain §9 risk. Different partitions restore independently; restoring a newly assigned `p0` must not suspend an already `ACTIVE` `p1`.

- [ ] **Step 4: Verify the state machine/restore sections and render the first diagram**

  Run:

  ```bash
  set -euo pipefail
  target='亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md'
  verify_dir="$(mktemp -d)"

  test "$(rg -Fxc '## 6. PartitionRuntimeContext 状态机' "$target")" -eq 1
  test "$(rg -Fxc '## 7. Assignment 与 state-before-event restore' "$target")" -eq 1
  for marker in \
    'NormalEffectGate' \
    'RestoreMaintenanceGate' \
    'OnPartitionsRevoked' \
    'OnPartitionsLost' \
    'SUSPENDED' \
    'poll/heartbeat' \
    '不得阻塞.*Consumer Group 协调循环' \
    '必须先于.*OnPartitionsAssigned' \
    '不得沿用旧 token' \
    '\[start,S0\)' \
    'activation queue' \
    'scopeReconcileRequired'
  do
    rg -n "$marker" "$target"
  done

  awk -v out="$verify_dir/state.mmd" '
    /^```mermaid$/ { block++; capture=(block==1); next }
    capture && /^```$/ { capture=0; next }
    capture { print > out }
  ' "$target"
  test -s "$verify_dir/state.mmd"
  npx -y @mermaid-js/mermaid-cli@11.4.2 \
    -p <(node -e 'process.stdout.write(JSON.stringify({args:["--no-sandbox","--disable-setuid-sandbox"]}))') \
    -i "$verify_dir/state.mmd" -o "$verify_dir/state.svg"
  test -s "$verify_dir/state.svg"
  git diff --check
  ```

  Expected: all markers match and Mermaid prints `Generating single mermaid chart` with exit 0.

  Do not stage or commit; continue on the same file in Task 4.

### Task 4: Complete the chapter with failover, Zombie, best-effort boundaries, and ownership index

**Files:**

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md`
- Read: approved design §§11–16
- Read: Kafka design §7.2

- [ ] **Step 1: Add §8 and the standard failover sequence diagram**

  Add:

  ```markdown
  <a id="runtime-failover"></a>

  ## 8. Graceful revoke、硬故障与 Zombie
  ```

  Use three subsections: `### 8.1 Graceful revoke`, `### 8.2 原 Runtime 硬故障`, and `### 8.3 Zombie Runtime 重连`. Freeze these meanings:

  - graceful revoke: `REVOKING` + token invalidation first, stop new records/effects, per-partition bounded drain, submit only the safe contiguous next-offset admitted by that partition's completion sequencer, close only the revoked context, then `DETACHED`;
  - hard failure: Coordinator expires A and assigns B; B's restore does not wait for A cleanup;
  - Zombie: A invalidates all old tokens before rejoin, joins as a new session, receives whatever assignment the Coordinator chooses, and never resumes old memory even if assigned the same partition.

  Include this standard Mermaid diagram; keep Coordinator, old A, new B, `scene_state`, and `scene_event` as distinct participants:

  ```mermaid
  sequenceDiagram
    autonumber
    participant KC as Kafka Coordinator
    participant A as Runtime A / old owner p0
    participant B as Runtime B / owns p1
    participant SS as Kafka scene_state[p0]
    participant SE as Kafka scene_event[p0]

    A->>SS: async handoff old state (request may remain in flight)
    A--xKC: heartbeat lost
    KC->>KC: expire A group session
    KC->>B: assign p0 in addition to p1
    B->>B: create p0 context + new token, status RESTORING
    B->>SE: pause ordinary p0 dispatch
    B->>SS: capture fixed S0 and read [start,S0)
    SS-->>B: full p0 state records before S0
    B->>B: stage Actor/state/timer descriptors
    B->>B: recheck assignment/token, activate p0
    B->>SE: unpause p0 after timers installed
    SE-->>B: deliver p0 records while p1 remains independently ACTIVE
    B->>SS: handoff newer p0 state
    A-->>SS: old in-flight state may arrive late
    Note over A,SS: V1 cannot revoke an already handed-off producer request
    A->>A: invalidate every old partition token
    A->>KC: rejoin with a new group session
    KC-->>A: assignment may be p0, another partition, or none
    Note over A,B: any newly assigned partition uses a new context/token and full restore
  ```

- [ ] **Step 2: Add §9 with an exact guarantee/risk matrix**

  Add:

  ```markdown
  <a id="runtime-best-effort"></a>

  ## 9. Best-effort fencing、幂等与残余窗口
  ```

  Explain that ownership uncertainty fails closed, but no persistent owner authority exists. Include the approved Action/Timer/Linked/`scene_state` table with columns “对象 / 收敛手段 / 不承诺”. Preserve this explicit distinction:

  > A stable `effectId` absorbs a repeated logical action. An old `scene_state` append that becomes the compacted latest can roll business state backward and cause a different future transition; that is not the same logical effect and is not proven safe by idempotency.

  List non-revocable work: not-yet-observed ownership loss, requests already handed to Kafka Producer, PG transactions entered/committed, and downstream-accepted effects.

- [ ] **Step 3: Add §§10–12 and the A/B/p0/p1 example**

  Add exactly:

  ```markdown
  <a id="runtime-partition-routing"></a>

  ## 10. 固定 partition、动态分配与扩容引用

  <a id="runtime-observability"></a>

  ## 11. 可观测性、故障注入与实施门槛

  <a id="runtime-main-spec-index"></a>

  ## 12. 与主规格的职责索引
  ```

  §10 must state:

  - route bytes/hash, fixed partition count, co-partitioning and Topic generation cutover remain in main-spec §14;
  - partition is dynamically assigned and never pinned to a machine;
  - inherit Kafka §7.2 `franz-go v1.21.5 + classic group + cooperative-sticky` verbatim, without creating a new decision;
  - include the approved `A owns p0; B owns p1 → A fails → B also owns p0 → A rejoins` example and state there is no B→A memory handoff.

  §11 must link main-spec §§16/19 and SLO §§5–8. At minimum require metrics for partition status/token changes, restore boundary/progress/duration, discarded staging, activation-queue depth/age, per-partition completion gap, revoke/lost callback, gate rejection by reason, late state callback/overwrite evidence, duplicate-effect convergence, and independent p0/p1 progress. It must keep recovery ADR “未冻结”.

  §12 must be a responsibility matrix covering main §§4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 16, 19 and this chapter §§2–11, so readers can decide which file owns data flow, wire, matching, evaluation, state, timer, Schedule/scope, transaction, delivery, partition and evidence questions without duplicating algorithms.

- [ ] **Step 4: Verify the complete chapter and commit**

  Run:

  ```bash
  set -euo pipefail
  target='亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md'
  verify_dir="$(mktemp -d)"

  for n in $(seq 1 12); do
    test "$(rg -c "^## $n\\." "$target")" -eq 1
  done
  test "$(rg -c '^## [0-9]+\.' "$target")" -eq 12
  for anchor in \
    runtime-scope runtime-model runtime-components runtime-partition-isolation \
    runtime-normal-completion runtime-state-machine runtime-restore runtime-failover \
    runtime-best-effort runtime-partition-routing runtime-observability runtime-main-spec-index
  do
    test "$(rg -Fxc "<a id=\"$anchor\"></a>" "$target")" -eq 1
  done
  test "$(rg -c '^```mermaid$' "$target")" -eq 2
  for marker in \
    'Coordinator.*A' \
    'new group session' \
    'best-effort' \
    'effectId' \
    'compacted latest' \
    'cooperative-sticky' \
    '恢复 ADR.*未冻结' \
    'p0' \
    'p1'
  do
    rg -n "$marker" "$target"
  done

  awk -v dir="$verify_dir" '
    /^```mermaid$/ { block++; capture=1; next }
    capture && /^```$/ { capture=0; next }
    capture { print > (dir "/diagram-" block ".mmd") }
    END { if (block != 2) exit 1 }
  ' "$target"
  for diagram in "$verify_dir"/diagram-*.mmd; do
    npx -y @mermaid-js/mermaid-cli@11.4.2 \
      -p <(node -e 'process.stdout.write(JSON.stringify({args:["--no-sandbox","--disable-setuid-sandbox"]}))') \
      -i "$diagram" -o "${diagram%.mmd}.svg"
    test -s "${diagram%.mmd}.svg"
  done
  git diff --check
  ```

  Expected: 12 unique H2 sections, two renderable Mermaid diagrams, and all boundary markers present.

  Do not stage or commit. Until the old main-spec algorithms and all authority statements are migrated, committing this chapter would create a permanent double-truth commit.

### Task 5: Migrate main-spec §§4–11 without breaking stable anchors

**Files:**

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`
- Read: new Runtime scheduling chapter §§2–7

- [ ] **Step 1: Update the main-spec reading card and §4–§6 ownership text**

  Keep every existing H2/H3 heading. Change reading-card lines 12/14/15 to say the main spec is the **总纲**, narrow its answer scope to product/wire/business state/evaluation/Outbox/capacity, add the scheduling chapter quick entry, and remove “本文仍是唯一规范性架构决策源”. Keep the top recovery ADR “未冻结”.

  Apply the approved migration boundary exactly:

  - §4 keeps one overall data-flow diagram and high-level Manager/Actor responsibility summary; replace detailed scheduling arrows with one link to chapter §§3–5.
  - §5.2 keeps `SOURCE_DELTA` wire, SubjectContext and matching business gates; record→Manager token/context execution belongs only to chapter §5.
  - §§5.3–5.5 keep payload, complete bundle, applicability, member/version and business no-op semantics; local-lane/cross-partition execution mechanics belong only to chapter §5.
  - §6.2 keeps “ProjectIndex replicated to each node; state/timer partitioned” as an architecture fact, but the per-partition Actor registry/close rules belong to chapter §§3–6.
  - §6.3 keeps ProjectIndex atomic update and cleanup business outcome; token/generation execution belongs to chapter §6.
  - §6.4 keeps config-ready/revision-gap business gate; assignment activation ordering belongs to chapter §7.
  - §6.6 keeps matching steps 1–6 and `c/λ` business definitions; replace step 7 `local target 直接投 Actor；remote target 发送 LinkedDelta` with “形成完整 target bundle；递交见调度分章 §5”.

  Where a full algorithm is removed, add exactly one link to the owning chapter section; do not leave an unlinked vague sentence.

- [ ] **Step 2: Replace §9.1 with a bounded summary and preserve §§9.2–9.5**

  Keep the heading `### 9.1 状态优先`, but replace its full restore procedure with this meaning:

  ```markdown
  新 owner 必须先完成对应 `scene_state` partition 的 bounded-cut staging restore、Actor/timer
  激活和 assignment/token 复验，才开放同 partition 的 `scene_event`。`[start,S0)`、staging、
  timer activation queue 与失权丢弃规则由[Runtime 调度分章 §7](Scene%20Runtime%20调度、分片与故障转移.md#runtime-restore)
  唯一规定。本节后续只规定 state 的业务有效性、scope reconciliation、损坏隔离与冷启动语义；
  恢复 ADR 仍未冻结。
  ```

  Do not alter the business validity/cold-start rules in §§9.2–9.5. If those sections need a lifecycle link, add it without copying the state machine.

- [ ] **Step 3: Trim §8/§10 scheduling mechanics while retaining business contracts**

  - §8.1 keeps the complete `SceneBindingState` schema and business meaning. Replace Actor hierarchy/partition ownership prose with one chapter §§2–4 link.
  - §8.2 keeps physical Topic, key/value, compaction, ACL, handoff and quotas. Keep the co-partition requirement but point dynamic ownership/restore to chapter §§6–7.
  - §10.1 keeps `logicalTimerId/timerFireId`, current Binding/current slot/business timing state and stale callback no-op. Move callback→lane, partition token capture and activation queue mechanics to chapter §§5–7 and leave one link.
  - §10.2 reliable delayed Action Outbox semantics remain unchanged.

  Also migrate the two §11 execution leaks while preserving all Schedule/scope/lifecycle business semantics:

  - §11.1 keeps target enumeration, pagination, target cap, `scheduleOccurrenceId/scheduleEvaluationId`, current BindingCtx and Actor business evaluation. Replace step 7 “Scene Entity Actor 读取/初始化 state 并求值” as a direct execution algorithm with “形成完整 `SCHEDULE_EVALUATE` target bundle；target-partition/lane delivery 见调度分章 §5”, then retain the business statement that the receiving Actor reads state, evaluates gates/FSM and decides Outbox.
  - §11.3 keeps Product/Project/Area/Entity-delete result semantics, `DELETE_STATE/INIT_STATE/ADD_MEMBER/REMOVE_MEMBER`, state close/tombstone and the rule that committed effects are not auto-revoked. Add one link stating cross-target delivery and partition-scoped lifecycle/close execution follow chapter §§5/8; do not turn a business `DELETE_STATE` into assignment revoke.

- [ ] **Step 4: Run bounded migration assertions, then continue directly to Task 6 without committing**

  Run:

  ```bash
  set -euo pipefail
  main='亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md'
  chapter='Scene%20Runtime%20调度、分片与故障转移.md'

  for heading in \
    '## 4. 总体数据流' \
    '## 5. `scene_event` 合约' \
    '## 6. Binding 生命周期、ProjectIndex 与 matching' \
    '## 8. `scene_state` 逻辑与物理模型' \
    '## 9. 恢复模型' \
    '### 9.1 状态优先' \
    '## 10. Timer 与 delayed action' \
    '## 11. Schedule 与 scope'
  do
    test "$(rg -Fxc "$heading" "$main")" -eq 1
  done

  test "$(sed -n '/^### 9\.1 /,/^### 9\.2 /p' "$main" | rg -c "$chapter")" -eq 1
  if sed -n '/^### 9\.1 /,/^### 9\.2 /p' "$main" | rg -q \
    '读取对应 scene_state compacted partition 到恢复边界|完成 state partition restore|再开放对应 scene_event partition'; then
    exit 1
  fi
  if sed -n '/^### 5\.4 /,/^### 5\.5 /p' "$main" | rg -Fq 'Manager 直接向当前 Actor 递交 local target bundle'; then
    exit 1
  fi
  if sed -n '/^### 6\.3 /,/^### 6\.4 /p' "$main" | rg -Fq \
    -e 'bindingId -> active BindingState refs' \
    -e '修改前仍检查 localPartitionGeneration'; then
    exit 1
  fi
  if sed -n '/^### 6\.6 /,/^## 7\./p' "$main" | rg -Fq \
    'local target 直接投 Actor；remote target 发送 LinkedDelta'; then
    exit 1
  fi
  if sed -n '/^### 10\.1 /,/^### 10\.2 /p' "$main" | rg -Fq \
    -e '回到相同 evaluation-subject lane' \
    -e '本地 partition generation 仍有效' \
    -e '它只是进程内 callback 回 lane'; then
    exit 1
  fi
  if sed -n '/^### 11\.1 /,/^### 11\.2 /p' "$main" | rg -Fq \
    '7. Scene Entity Actor 读取/初始化 state 并求值'; then
    exit 1
  fi
  test "$(sed -n '/^### 11\.1 /,/^### 11\.2 /p' "$main" | rg -c 'Scene%20Runtime%20调度、分片与故障转移\.md')" -eq 1
  test "$(sed -n '/^### 11\.3 /,/^## 12\./p' "$main" | rg -c 'Scene%20Runtime%20调度、分片与故障转移\.md')" -eq 1
  for marker in 'ProjectIndex' 'SubjectContext' 'stateKey' 'logicalTimerId' 'timerFireId' '32 KiB'; do
    rg -n "$marker" "$main"
  done
  git diff --check
  ```

  Expected: old headings and business markers remain; §9.1 contains exactly one chapter link and no old full algorithm.

  Do **not** stage or commit this half-migrated main spec. No other agent may edit this file. Continue immediately with Task 6; all eleven formal documents are committed together in Task 10.

### Task 6: Migrate main-spec §§12–21 and expose the normative set in README

**Files:**

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`
- Read: new chapter §§5–12

- [ ] **Step 1: Keep §12.3 transaction ownership and replace §13.2 completion details**

  §12.3 remains the sole full definition of tentative state serialization, PG Action Outbox transaction, commit, in-memory state/timer publication, and async state handoff. Add a link to chapter §5 only for how this order joins the partition-local completion frontier.

  Keep heading `### 13.2 Event commit`, but replace the full predicate with a short delivery summary containing:

  - Scene input remains at-least-once;
  - non-state pack callback/retry/quarantine and state best-effort handoff promises remain unchanged;
  - the **complete** predicate and contiguous next-offset algorithm exist only in [Runtime 调度分章 §5](Scene%20Runtime%20调度、分片与故障转移.md#runtime-normal-completion);
  - offset commit does not mean external action execution and does not create cross-store atomicity.

- [ ] **Step 2: Replace §13.3 with the exact best-effort summary**

  Keep heading `### 13.3 Rebalance 与本地 generation guard`, but replace its full list/state transition algorithm with a concise statement:

  ```markdown
  V1 只提供进程内 status + local generation token 的 best-effort fencing：Runtime 一旦知道 revoke、
  session lost 或 ownership 无法证明，就先使旧 token 失效并拒绝新的数据面副作用。它不能撤销已进入
  Kafka Producer/PG 的请求，也不能保证迟到旧 `scene_state` 不覆盖新 state。完整状态机、graceful
  revoke、硬故障、Zombie 重连和残余窗口由[Runtime 调度分章 §§6–9](Scene%20Runtime%20调度、分片与故障转移.md#runtime-state-machine)
  唯一规定；本设计不增加 DB owner epoch、分布式 lease 或 Kafka transaction。
  ```

  The section may retain the TB comparison fact, but must not retain a second entry-gate list or revoke algorithm.

- [ ] **Step 3: Preserve §14 route/hash and move only dynamic ownership mechanics**

  Keep all route-key bytes, NFC, unsigned-varint, Kafka Murmur2, state key, co-partition and fixed-count rules in §14.1. Keep “consumer instances dynamically join/leave” and “partition is not pinned to a machine” as facts, followed by one link to chapter §§6/10 for the ownership state machine. Keep §14.2 Topic generation cutover unchanged.

- [ ] **Step 4: Update §§17–21, preserve their evidence counts, and do not claim success**

  - §17 keeps ThingsBoard source evidence; rows about Actor/partition/rebalance link to the new chapter instead of restating the implementation.
  - §18 changes the authority statement to “主规格总纲 + Runtime 调度分章共同组成规范性架构决策源”; keep the historical migration matrix.
  - §18.1 splits Scene ownership accurately: the main spec owns wire/business/transaction/route; the new chapter owns scheduling/restore/failover.
  - §19 keeps exactly the existing 26 numbered evidence gates. Expand its introduction to say the tested Zombie/restore/late-send protocol is owned by chapter §11 and remains unfrozen. Strengthen existing items without changing their numbers: item 8 includes independent p0/p1 `[start,S0)` bounded-cut restore; item 11 includes graceful/lost/missed-revoke/Zombie token invalidation; item 12 remains stale-state late-send; item 13 remains old/new-owner `effectId`; item 15 includes due-timer `activation queue` and scope reconciliation; item 21 includes `RESTORING` + `RestoreMaintenanceGate` maintenance-only rejection. These are test labels and links, not a duplicate algorithm. Do not append 27–30 and do not write “已通过”.
  - §20 keeps rejected durable lease/epoch/EOS items and links chapter §9 for the accepted V1 boundary.
  - §21 keeps the product/data/business conclusions and points its Runtime scheduling/recovery conclusion to the new chapter.

- [ ] **Step 5: Add the README index and normative-set statement**

  In `README.md`, change the first “按问题查阅” row into two explicit rows:

  ```markdown
  | 确认产品边界、wire、Binding/state 业务语义、求值、Outbox 事务、route/hash 或容量 | [主规格总纲](场景联动 Runtime 主规格总纲.md) |
  | 理解 Manager、Actor lane、多 partition 隔离、completion、恢复、rebalance、硬故障或 Zombie | [Runtime 调度、分片与故障转移][runtime-coordination] |
  ```

  Replace the authority paragraph with:

  ```markdown
  [主规格总纲](场景联动 Runtime 主规格总纲.md)与《Runtime 调度、分片与故障转移》规范性分章
  共同组成 scene-linkage 的规范性架构决策源：前者拥有产品、wire、业务状态/事务、route 与容量，
  后者唯一拥有 partition ownership、Manager/Actor 执行、completion、restore、rebalance 与 Zombie 协议。
  三份配套文档是从属实施契约，导读只负责解释，两份附录只提供非规范性研究。两份架构源没有
  “后写覆盖”关系；冲突必须在同一次变更中联合修订。

  [runtime-coordination]: Scene%20Runtime%20调度、分片与故障转移.md
  ```

  Rewrite README §“跨目录同步边界” as well: the parent owns platform/non-Scene boundaries, Kafka owns non-Scene physical contracts, and the **main total + scheduling chapter** jointly own Scene according to their split. Remove “主规格拥有 Scene Runtime 架构决策” as a single-owner sentence. Neither architecture source has priority; conflicts require same-change joint revision.

  Keep the README recovery ADR “未冻结” warning unchanged in meaning, but allow its §19 link to remain on the main spec because evidence ownership stays there.

- [ ] **Step 6: Verify the main/README migration and commit**

  Run:

  ```bash
  set -euo pipefail
  scene_dir='亿级点位高吞吐架构/6.规则引擎与场景联动'
  main="$scene_dir/场景联动 Runtime 主规格总纲.md"
  readme="$scene_dir/README.md"

  slice() {
    awk -v start="$1" -v stop="$2" '
      $0 == start { emit=1 }
      emit && $0 == stop { exit }
      emit { print }
    ' "$main"
  }

  for heading in \
    '## 12. Outbox 与动作幂等' \
    '### 12.3 状态与 Outbox 顺序' \
    '## 13. Kafka 交付、事务与失败窗口' \
    '### 13.2 Event commit' \
    '### 13.3 Rebalance 与本地 generation guard' \
    '## 14. Partition 约束' \
    '## 17. 与 ThingsBoard 4.4 的对齐与差异' \
    '## 18. 已完成的单一规格迁移' \
    '## 19. POC 与 ADR 冻结门槛' \
    '## 20. 已拒绝或撤销的方案' \
    '## 21. 最终结论'
  do
    test "$(rg -Fxc "$heading" "$main")" -eq 1
  done

  test "$(sed -n '/^### 13\.2 /,/^### 13\.3 /p' "$main" | rg -c 'Scene%20Runtime%20调度、分片与故障转移\.md')" -eq 1
  test "$(sed -n '/^### 13\.3 /,/^## 14\./p' "$main" | rg -c 'Scene%20Runtime%20调度、分片与故障转移\.md')" -eq 1
  if sed -n '/^### 13\.3 /,/^## 14\./p' "$main" | rg -q '以下位置在产生新副作用前检查|失效后：|同时执行 bounded drain'; then
    exit 1
  fi
  test "$(sed -n '/^## 17\./,/^## 18\./p' "$main" | rg -c '^\|')" -eq 27
  test "$(sed -n '/^### 18\.1 /,/^## 19\./p' "$main" | rg -c '^\|')" -eq 14
  test "$(sed -n '/^## 19\./,/^## 20\./p' "$main" | rg -c '^[0-9]+\. ')" -eq 26
  test "$(sed -n '/^## 20\./,/^## 21\./p' "$main" | rg -c '^- ')" -eq 33
  for marker in 'Zombie' '\[start,S0\)' 'activation queue' 'RestoreMaintenanceGate' '恢复 ADR.*未冻结'; do
    rg -n "$marker" "$main"
  done

  while IFS=$'\t' read -r start stop; do
    test "$(slice "$start" "$stop" | rg -c 'Scene%20Runtime%20调度、分片与故障转移\.md')" -eq 1
  done < <(printf '%s\t%s\n' \
    '## 4. 总体数据流' '## 5. `scene_event` 合约' \
    '### 5.4 终端 handler 与统一求值入口' '### 5.5 Subject lifecycle 与终端 lifecycle bundle' \
    '### 6.2 每节点全量规则缓存，运行时 state 分区' '### 6.3 单 Project 原子更新' \
    '### 6.3 单 Project 原子更新' '### 6.4 冷启动、revision gap 与恢复' \
    '### 6.4 冷启动、revision gap 与恢复' '### 6.5 无 barrier 的完整 lookup' \
    '### 6.6 SOURCE matching 与 fanout' '## 7. 计算语义' \
    '### 8.1 完整状态' '### 8.2 物理 Topic' \
    '### 9.1 状态优先' '### 9.2 有效 state' \
    '### 10.1 普通 timer' '### 10.2 Reliable delayed action' \
    '### 11.1 借鉴 TB current-target reevaluation 的 fanout' '### 11.2 Scope overlap' \
    '### 11.3 Product、Project、Area 生命周期' '## 12. Outbox 与动作幂等' \
    '### 12.3 状态与 Outbox 顺序' '## 13. Kafka 交付、事务与失败窗口' \
    '### 13.2 Event commit' '### 13.3 Rebalance 与本地 generation guard' \
    '### 13.3 Rebalance 与本地 generation guard' '## 14. Partition 约束' \
    '### 14.1 V1 固定 partition count' '### 14.2 扩容' \
    '## 17. 与 ThingsBoard 4.4 的对齐与差异' '## 18. 已完成的单一规格迁移' \
    '## 18. 已完成的单一规格迁移' '### 18.1 跨目录规范一致性' \
    '### 18.1 跨目录规范一致性' '## 19. POC 与 ADR 冻结门槛' \
    '## 19. POC 与 ADR 冻结门槛' '## 20. 已拒绝或撤销的方案' \
    '## 20. 已拒绝或撤销的方案' '## 21. 最终结论')
  test "$(sed -n '/^## 21\./,$p' "$main" | rg -c 'Scene%20Runtime%20调度、分片与故障转移\.md')" -eq 1

  if rg -n -F \
    -e '本文仍是本目录唯一规范性架构决策源' \
    -e '唯一的规范性架构决策源' \
    -e '与本规格冲突时，以本规格为准' \
    -e '以本规格为决策基线' \
    -e '本文拥有 Scene Runtime 的 Topic、matching、状态、恢复、offset、配置/归属与动作语义' \
    "$main"; then
    exit 1
  fi
  test "$(rg -o 'Scene%20Runtime%20调度、分片与故障转移\.md' "$readme" | wc -l)" -eq 1
  test "$(rg -o '\]\[runtime-coordination\]' "$readme" | wc -l)" -eq 1
  test "$(rg -c '^\[runtime-coordination\]: Scene%20Runtime%20调度、分片与故障转移\.md$' "$readme")" -eq 1
  if rg -n '主规格拥有 Scene Runtime 架构决策|冲突时.*以主规格为准' "$readme"; then
    exit 1
  fi
  rg -n '共同组成.*规范性架构决策源' "$readme" "$main"
  for file in "$readme" "$main"; do
    rg -n '恢复 ADR.*未冻结' "$file"
  done
  git diff --check
  ```

  Do not stage or commit; the subordinate contracts and cross-directory references are not synchronized yet.

### Task 7: Update the guide, subordinate contracts, and non-normative appendix navigation

**Files:**

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/附录-竞品状态方案调研.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/附录2-thingsboard规则引擎与CF实现分析.md`

- [ ] **Step 1: Update the 15-minute guide without turning it into another protocol copy**

  Replace its single-main-spec authority claim with a link to both architecture sources. In the final down-drill table, split “总体 Runtime/§4–§14” into:

  - product/wire/fact/state/action transaction/route → main-spec total;
  - Manager/Actor lane/partition isolation/completion/restore/failover → Runtime scheduling chapter.

  Keep the multi-device sequence diagram, facts and transaction explanation unchanged. Add only a short note near `SceneBindingState` explaining that Actor lane and partition lifecycle are in the new chapter; do not copy `PartitionRuntimeContext` fields or its state machine into the guide.

- [ ] **Step 2: Update control-plane and operator-contract reading cards**

  In `控制面与匹配契约.md`:

  - change authority to the two-file normative set;
  - keep this file subordinate and authoritative only for Binding/Scope/ProjectIndex/matching;
  - route local Actor versus cross-partition delivery, active-state registry, and failure-window links to the new chapter;
  - retain matching result/payload/applicability algorithms here.

  In `算子语义与术语.md`:

  - change authority to the two-file normative set;
  - change “Runtime 数量、Topic、分区...” ownership into: main spec owns Runtime/product/Topic/schema/route; scheduling chapter owns partition lifecycle/Actor execution/restore;
  - define the existing word `Actor lane` as actor-inspired subject-keyed serialization and link chapter §§2–3;
  - keep Reducer, ComputationPlan, Current Fact, state/operator/timer business semantics unchanged;
  - do not add a duplicate `DETACHED/RESTORING/...` state machine.

- [ ] **Step 3: Update only appendix navigation and normative boundary**

  In each appendix, add the new chapter to the top navigation and replace the **entire** top normative-boundary sentence—not only its first clause—with: current architecture decisions live in the main total and Runtime scheduling chapter; neither has later-write/priority precedence; any conflict requires joint revision. Remove `如有冲突，以主规格为准` / `本文与主规格冲突时，以主规格为准`. Keep every research date, ThingsBoard source fact, comparison table and conclusion unchanged. Do not fix unrelated historical wording such as the recorded development-line description in Appendix 2.

- [ ] **Step 4: Verify no subordinate document became a second protocol source and commit**

  Run:

  ```bash
  set -euo pipefail
  scene_dir='亿级点位高吞吐架构/6.规则引擎与场景联动'
  files=(
    "$scene_dir/00-15分钟读懂场景联动.md"
    "$scene_dir/控制面与匹配契约.md"
    "$scene_dir/算子语义与术语.md"
    "$scene_dir/附录-竞品状态方案调研.md"
    "$scene_dir/附录2-thingsboard规则引擎与CF实现分析.md"
  )

  for file in "${files[@]}"; do
    rg -n 'Scene(%20| )Runtime(%20| )调度、分片与故障转移\.md' "$file"
  done
  if rg -n '主规格.*唯一.*规范性架构决策源|现行架构.*只见.*主规格|只见顶部主规格' "${files[@]}"; then
    exit 1
  fi
  for subordinate in \
    "$scene_dir/控制面与匹配契约.md" \
    "$scene_dir/算子语义与术语.md"
  do
    if sed -n '1,24p' "$subordinate" | rg -n \
      '无条件以主规格为准|冲突时.*以主规格为准|一律以主规格为准'; then
      exit 1
    fi
  done
  for appendix in \
    "$scene_dir/附录-竞品状态方案调研.md" \
    "$scene_dir/附录2-thingsboard规则引擎与CF实现分析.md"
  do
    if sed -n '1,24p' "$appendix" | rg -n '以主规格为准|主规格冲突时'; then
      exit 1
    fi
  done
  if rg -n 'OnPartitionsRevoked|groupSessionToken|RestoreMaintenanceGate|RESTORING --> ACTIVE' \
    "$scene_dir/00-15分钟读懂场景联动.md" \
    "$scene_dir/控制面与匹配契约.md" \
    "$scene_dir/算子语义与术语.md"; then
    exit 1
  fi

  base_record='/tmp/scene-runtime-scheduling-partition-failover-base-sha'
  task_base_sha="$(tr -d '\n' < "$base_record")"
  git diff --unified=0 "$task_base_sha" -- \
    "$scene_dir/附录-竞品状态方案调研.md" \
    "$scene_dir/附录2-thingsboard规则引擎与CF实现分析.md" | \
  awk '
    /^[+-][^+-]/ &&
    $0 !~ /(导航|规范性边界|本项目的?现行选择|本项目只作简短定位|完整约束|具体语义)/ {
      print
      bad=1
    }
    END { exit bad }
  '
  git diff --check
  ```

  Expected: every file links the chapter; old single-source wording and copied state-machine internals are absent.

  Do not stage or commit; continue to SLO and cross-directory synchronization.

### Task 8: Extend SLO and fault injection for multi-partition ownership and Zombie recovery

**Files:**

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md`
- Read: new chapter §§6–11
- Read: main-spec §19

- [ ] **Step 1: Update SLO authority and implementation ownership**

  Change the header/read-card authority to the two-file normative set. Keep §10 RACI and recovery ADR “未冻结”. In §1, state that metrics/data/business semantics remain in main §§15–16, while restore/failover execution semantics come from scheduling chapter §§6–11.

  Update the final admission checklist too: replace its existing “主规格仍被标记为唯一规范性架构权威” check with “主规格总纲与 Runtime 调度分章仍共同构成规范性架构源，本文未形成平行 ADR”. In the top 24 lines and final checklist, `无条件以主规格为准` / single-main-source wording must be absent.

  In implementation artifacts/rollout sections, require a per-partition context registry, callback adapter, token gates, staging restore, activation queue, independent completion sequencer and partition-scoped close. Do not invent Go package names, struct fields beyond the normative context abstraction, DDL, Topic or Consumer Group names.

- [ ] **Step 2: Audit every direct §13.2/§13.3 reference, not only F-18–F-20**

  First run `rg -n '§13\.2|§13\.3'` over the complete SLO document and classify every hit. At minimum update F-07, F-13, F-14, F-18～F-22, F-27～F-30 and the metrics/rollout/checklist prose:

  - F-07/F-27/F-28/F-29 and the completion part of F-21/F-22/F-30 point to scheduling chapter §5; payload/quarantine business rules may still point to their main-spec owners.
  - F-13 keeps main §8.2 for `scene_state` physical handoff and points completion semantics to chapter §5.
  - F-14 points late old-owner state/overwrite risk to chapter §9 and must not relabel it as idempotently solved.
  - F-18 keeps Binding/current-slot/timer identity from main/operator contract and points callback token/lane ownership to chapter §§5–7.
  - F-19 explicitly covers graceful revoke plus lost/missed-revoke and points to chapter §§6/8.
  - F-20 keeps stable `transitionId/effectId` duplicate convergence in main §12.2 and points residual owner overlap to chapter §9.

  After the edit, no SLO reference may use bare main `§13.2` or `§13.3` as the owner of completion/rebalance algorithms. A reference to those retained summary sections is allowed only when the same sentence names the new chapter's exact owning section.

- [ ] **Step 3: Append two bounded, non-conflicting F-31–F-32 rows to §6.5**

  Existing IDs end at F-30. Add these two composite rows, using the same five-column table format and renaming the last column from “主规格门槛” to “规范门槛” where needed:

  - `F-31`: B already owns ACTIVE `p1` when it receives `p0`; cover both hard failure and Zombie/missed-revoke rejoin. Forbid waiting for A cleanup, sharing generation/restore/completion/Actor/timer state, reusing A's old context, or pausing/closing `p1`. Verify Coordinator-initiated assignment, old-token invalidation before rejoin, new token/full restore for every new assignment, independent contexts and independent progress. If `p0` is revoked during restore, discard only `p0` staging/activation queue.
  - `F-32`: while `p0` is `RESTORING`, allow only quarantine and pure tombstone through `RestoreMaintenanceGate`; forbid non-tombstone state, Linked, Outbox, evaluation and business-timer progress. Verify fixed `[start,S0)`, assignment/token recheck, already-due timer activation queue, timer return to lane only after activation, and zero effect when scope reconciliation fails.

  Both rows reference scheduling chapter §§6–9 and the strengthened existing main POC items 8/11/12/13/15/21. F-14 remains the separate proof for stale state overwrite; F-31 must not claim Kafka group fencing covers Producer/PG.

  Update every literal range `F-01～F-30` in rollout, evidence grouping, canary and final checklists to `F-01～F-32`. Do not leave a mixed 30/32 scope.

- [ ] **Step 4: Extend rollout/POC checks and verify the IDs**

  Add explicit canary/POC evidence for:

  - callback ordering (`lost/suspend` token invalidation before rejoin/assigned handling);
  - `p0` restore while `p1` remains ACTIVE;
  - restoration cut `S0`, staging discard and activation queue;
  - `RESTORING` gate rejection counters by effect kind;
  - stale state overwrite evidence versus duplicate-effect convergence as separate results.

  Run:

  ```bash
  set -euo pipefail
  slo='亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md'

  for n in $(seq -w 1 32); do
    test "$(rg -c "^\\| F-$n \\|" "$slo")" -eq 1
  done
  test "$(rg -c '^\| F-[0-9]{2} \|' "$slo")" -eq 32
  test "$(rg -o '^\| F-[0-9]{2} \|' "$slo" | sort | uniq -d | wc -l)" -eq 0
  if rg -n 'F-01～F-30' "$slo"; then
    exit 1
  fi
  for marker in \
    'F-01～F-32' \
    'F-31' \
    'F-32' \
    'Zombie' \
    'activation queue' \
    'RestoreMaintenanceGate' \
    '恢复 ADR.*未冻结'
  do
    rg -n "$marker" "$slo"
  done
  rg -n 'Scene(%20| )Runtime(%20| )调度、分片与故障转移\.md' "$slo"
  if { sed -n '1,24p' "$slo"; sed -n '/^## 11\./,$p' "$slo"; } | \
    rg -n '无条件以主规格为准|主规格仍被标记为唯一规范性架构权威'; then
    exit 1
  fi
  if rg -n '§13\.2|§13\.3' "$slo" | rg -v 'Runtime 调度|调度分章'; then
    exit 1
  fi
  if rg -n 'Zombie.*(严格|durable).*fenc|F-31.*Producer.*自动' "$slo"; then
    exit 1
  fi
  git diff --check
  ```

  Do not stage or commit; the parent/Kafka owners are still pending.

### Task 9: Synchronize parent architecture and Kafka boundaries

**Files:**

- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
- Modify: `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md`
- Read: new chapter §§1, 5–10

- [ ] **Step 1: Update the parent architecture's Scene authority and completion summary**

  At parent §11.1, replace the single Scene source with two links and the same ownership split used by README. Keep platform product boundaries and non-Scene subsystem decisions in the parent.

  At the current paragraph that spells out the whole completion predicate, retain only the product-level promise:

  - Action acceptance is PG Outbox commit;
  - input completion is partition-local and does not mean external execution;
  - state is async best-effort and not cross-store atomic;
  - exact predicate/contiguous frontier belongs to scheduling chapter §5.

  Audit the current direct references near lines 293/306 (offset/failure window), 657 (evaluation-subject lane), 674 (authority), 721 and 1269 (duplicated full completion), 804 (input offset table), 859/861 (owner/timer recovery), 1177 (local generation) and 1299 (continuous next-offset/restore reconciliation). Route scheduling/recovery mechanics to the chapter while leaving matching, identity and §12.3 Action Outbox ordering on the main spec. Do not copy the full predicate again.

- [ ] **Step 2: Update every Kafka Scene reference by contract owner**

  Audit the complete Kafka file, not only §8:

  ```bash
  set -euo pipefail
  kafka='亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'
  rg -n '场景主规格|Scene.*主规格|scene-linkage|§(4|5|6|8|9|10|12|13|14)' "$kafka"
  ```

  Classify and rewrite each result:

  - `SceneEvent/SceneBindingState` wire, physical topic/key, Action Outbox transaction, route/hash and Topic generation → main spec;
  - Manager/Actor lane, completion, state-before-event restore, dynamic ownership, rebalance and Zombie → scheduling chapter;
  - statements covering both → link both and name each responsibility;
  - historical rejection/disposition → keep the historical fact, but replace “current algorithm exists only in main §5–§14” with the two-file normative set.

  In particular update file header/§0, §1.2, §5.3, §5.5, §§6.1–6.2, §8 preface and §§8.2–8.5. §8.2 restore points to chapter §7, §8.3 completion to chapter §5, and §8.4 rebalance to chapter §§6–9.

- [ ] **Step 3: Prove Kafka §7.2 client baseline did not drift**

  Before and after the edit, preserve these exact facts:

  ```text
  github.com/twmb/franz-go
  v1.21.5
  assignment strategy = cooperative-sticky
  classic consumer group
  ```

  The scheduling chapter may inherit them, but this Kafka section remains their only decision owner. Do not add `group.instance.id`, new consumer protocol, transactional Scene producer, lease fencing or assignor configuration.

  Verify the entire section is byte-for-byte unchanged relative to Task 1, not merely that four words still exist:

  ```bash
  set -euo pipefail
  base_record='/tmp/scene-runtime-scheduling-partition-failover-base-sha'
  task_base_sha="$(tr -d '\n' < "$base_record")"
  kafka='亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'

  diff -u \
    <(git show "$task_base_sha:$kafka" | \
      sed -n '/^### 7\.2 Consumer 基线$/,/^### 7\.3 Raw → Canonical transaction$/p') \
    <(sed -n '/^### 7\.2 Consumer 基线$/,/^### 7\.3 Raw → Canonical transaction$/p' "$kafka")
  ```

  Expected: no diff and exit 0.

- [ ] **Step 4: Verify cross-directory ownership and commit**

  Run:

  ```bash
  set -euo pipefail
  parent='亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md'
  kafka='亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'

  for file in "$parent" "$kafka"; do
    rg -n 'Scene(%20| )Runtime(%20| )调度、分片与故障转移\.md' "$file"
  done
  for marker in \
    'github.com/twmb/franz-go' \
    'v1.21.5' \
    'assignment strategy = cooperative-sticky' \
    'classic consumer group'
  do
    rg -Fn "$marker" "$kafka"
  done
  if rg -n '当前规范性算法只存在于场景主规格|Scene Runtime 的 Topic、matching、状态、恢复与失败窗口以.*主规格.*为规范性来源' "$parent" "$kafka"; then
    exit 1
  fi
  git diff --check
  ```

  Expected: both files link the new chapter, the old single-owner phrases are gone, and all four Kafka baseline markers remain.

  Do not commit individual files. Proceed to Task 10, stage exactly the complete eleven-file normative set, review that staged diff, and create one atomic implementation commit.

### Task 10: Run mechanical verification, fresh-reader review, and final quality review

**Files:**

- Verify: all eleven implementation files
- Do not modify historical specs/plans/HTML or product code

- [ ] **Step 1: Stage exactly the atomic eleven-file set and verify its scope**

  ```bash
  set -euo pipefail
  base_record='/tmp/scene-runtime-scheduling-partition-failover-base-sha'
  test -s "$base_record"
  task_base_sha="$(tr -d '\n' < "$base_record")"
  test "$(printf '%s' "$task_base_sha" | wc -c)" -eq 40
  git cat-file -e "$task_base_sha^{commit}"
  git merge-base --is-ancestor "$task_base_sha" HEAD

  git add -- \
    '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/README.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/附录-竞品状态方案调研.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/附录2-thingsboard规则引擎与CF实现分析.md' \
    '亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md'

  test -z "$(git diff --name-only)"
  test -z "$(git ls-files --others --exclude-standard)"
  git diff --cached --check

  actual="$(git -c core.quotePath=false diff --cached --name-only | sort)"
  expected="$(printf '%s\n' \
    '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/README.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/附录-竞品状态方案调研.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/附录2-thingsboard规则引擎与CF实现分析.md' \
    '亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md' | sort)"
  test "$actual" = "$expected"
  test -z "$(git -c core.quotePath=false diff --cached --name-only --diff-filter=DRCTUXB)"
  test "$(git -c core.quotePath=false diff --cached --name-only --diff-filter=A | wc -l)" -eq 1
  test "$(git -c core.quotePath=false diff --cached --name-only --diff-filter=M | wc -l)" -eq 10
  test "$(git -c core.quotePath=false diff --cached --name-only --diff-filter=A)" = \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md'
  ```

  Expected: there are no unstaged tracked changes; the staged diff-check passes and contains exactly the eleven formal Markdown files—no historical design/plan/HTML, `.superpowers/`, product code, rename or deletion. Do not commit yet; Steps 2–7 review this exact staged normative set.

- [ ] **Step 2: Verify structure, unique ownership, and retained anchors**

  Run:

  ```bash
  set -euo pipefail
  scene_dir='亿级点位高吞吐架构/6.规则引擎与场景联动'
  target="$scene_dir/Scene Runtime 调度、分片与故障转移.md"
  main="$scene_dir/场景联动 Runtime 主规格总纲.md"

  for n in $(seq 1 12); do
    test "$(rg -c "^## $n\\." "$target")" -eq 1
  done
  for anchor in \
    runtime-scope runtime-model runtime-components runtime-partition-isolation \
    runtime-normal-completion runtime-state-machine runtime-restore runtime-failover \
    runtime-best-effort runtime-partition-routing runtime-observability runtime-main-spec-index
  do
    test "$(rg -Fxc "<a id=\"$anchor\"></a>" "$target")" -eq 1
  done
  test "$(rg -c '^```mermaid$' "$target")" -eq 2
  test "$(rg -o 'Scene%20Runtime%20调度、分片与故障转移\.md' "$scene_dir/README.md" | wc -l)" -eq 1
  test "$(rg -o '\]\[runtime-coordination\]' "$scene_dir/README.md" | wc -l)" -eq 1

  for heading in \
    '## 4. 总体数据流' \
    '## 5. `scene_event` 合约' \
    '## 6. Binding 生命周期、ProjectIndex 与 matching' \
    '## 8. `scene_state` 逻辑与物理模型' \
    '## 9. 恢复模型' \
    '## 10. Timer 与 delayed action' \
    '## 11. Schedule 与 scope' \
    '## 12. Outbox 与动作幂等' \
    '## 13. Kafka 交付、事务与失败窗口' \
    '## 14. Partition 约束' \
    '## 17. 与 ThingsBoard 4.4 的对齐与差异' \
    '## 18. 已完成的单一规格迁移' \
    '## 19. POC 与 ADR 冻结门槛' \
    '## 20. 已拒绝或撤销的方案' \
    '## 21. 最终结论'
  do
    test "$(rg -Fxc "$heading" "$main")" -eq 1
  done

  active=(
    "$scene_dir/README.md"
    "$main"
    "$target"
    "$scene_dir/00-15分钟读懂场景联动.md"
    "$scene_dir/控制面与匹配契约.md"
    "$scene_dir/算子语义与术语.md"
    "$scene_dir/SLO故障测试与实施.md"
    '亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md'
    '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'
    "$scene_dir/附录-竞品状态方案调研.md"
    "$scene_dir/附录2-thingsboard规则引擎与CF实现分析.md"
  )
  if rg -n \
    '主规格.*唯一.*规范性架构决策源|唯一的规范性架构决策源|主规格拥有 Scene Runtime 架构决策|当前规范性算法只存在于场景主规格|Scene 的全部调度/恢复算法以单个主规格文件为唯一来源|现行架构(决策)?只见上述主规格|只见顶部主规格|与本规格冲突时，以本规格为准|以本规格为决策基线' \
    "${active[@]}"; then
    exit 1
  fi

  for file in \
    "$scene_dir/控制面与匹配契约.md" \
    "$scene_dir/算子语义与术语.md" \
    "$scene_dir/SLO故障测试与实施.md" \
    "$scene_dir/附录-竞品状态方案调研.md" \
    "$scene_dir/附录2-thingsboard规则引擎与CF实现分析.md"
  do
    if sed -n '1,24p' "$file" | rg -n \
      '无条件以主规格为准|以主规格为准|主规格冲突时|主规格仍被标记为唯一规范性架构权威'; then
      exit 1
    fi
  done
  ```

- [ ] **Step 3: Verify algorithm markers exist only in the owning chapter**

  Run:

  ```bash
  set -euo pipefail
  scene_dir='亿级点位高吞吐架构/6.规则引擎与场景联动'
  target="$scene_dir/Scene Runtime 调度、分片与故障转移.md"
  main="$scene_dir/场景联动 Runtime 主规格总纲.md"

  for marker in \
    'PartitionRuntimeContext \{' \
    'NormalEffectGate' \
    'RestoreMaintenanceGate' \
    'OnPartitionsLost' \
    'SUSPENDED' \
    'poll/heartbeat' \
    '不得阻塞.*Consumer Group 协调循环' \
    '必须先于.*OnPartitionsAssigned' \
    '不得沿用旧 token' \
    '\[start,S0\)' \
    'activation queue' \
    'Zombie Runtime' \
    'compacted latest'
  do
    rg -n "$marker" "$target"
  done

  if sed -n '/^### 9\.1 /,/^### 9\.2 /p' "$main" | rg -q '逐条直接重建|activation queue|RestoreMaintenanceGate'; then
    exit 1
  fi
  if sed -n '/^### 13\.2 /,/^### 13\.3 /p' "$main" | rg -q '完整 input completion 谓词|只有连续的 earlier next-offset'; then
    exit 1
  fi
  if sed -n '/^### 13\.3 /,/^## 14\./p' "$main" | rg -q 'OnPartitionsRevoked|OnPartitionsLost|SUSPENDED|groupSessionToken'; then
    exit 1
  fi

  for marker in 'routeKeyBytes' 'kafkaMurmur2' 'unsigned-varint' '固定 partition count'; do
    rg -n "$marker" "$main"
  done
  for marker in 'SceneBindingState' 'logicalTimerId' 'timerFireId' 'Action Outbox'; do
    rg -n "$marker" "$main"
  done

  test "$(sed -n '/^## 17\./,/^## 18\./p' "$main" | rg -c '^\|')" -eq 27
  test "$(sed -n '/^### 18\.1 /,/^## 19\./p' "$main" | rg -c '^\|')" -eq 14
  test "$(sed -n '/^## 19\./,/^## 20\./p' "$main" | rg -c '^[0-9]+\. ')" -eq 26
  test "$(sed -n '/^## 20\./,/^## 21\./p' "$main" | rg -c '^- ')" -eq 33

  slo="$scene_dir/SLO故障测试与实施.md"
  for n in $(seq -w 1 32); do
    test "$(rg -c "^\\| F-$n \\|" "$slo")" -eq 1
  done
  test "$(rg -c '^\| F-[0-9]{2} \|' "$slo")" -eq 32
  if rg -n 'F-01～F-30' "$slo"; then
    exit 1
  fi
  ```

  Expected: context/state-machine/restore/failover internals occur in the new chapter; route/business/transaction markers remain in the main spec.

- [ ] **Step 4: Render every Mermaid diagram and check local Markdown links**

  Run:

  ```bash
  set -euo pipefail
  target='亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md'
  verify_dir="$(mktemp -d)"
  base_record='/tmp/scene-runtime-scheduling-partition-failover-base-sha'
  task_base_sha="$(tr -d '\n' < "$base_record")"

  awk -v dir="$verify_dir" '
    /^```mermaid$/ { block++; capture=1; next }
    capture && /^```$/ { capture=0; next }
    capture { print > (dir "/diagram-" block ".mmd") }
    END { if (block != 2) exit 1 }
  ' "$target"
  for diagram in "$verify_dir"/diagram-*.mmd; do
    npx -y @mermaid-js/mermaid-cli@11.4.2 \
      -p <(node -e 'process.stdout.write(JSON.stringify({args:["--no-sandbox","--disable-setuid-sandbox"]}))') \
      -i "$diagram" -o "${diagram%.mmd}.svg"
    test -s "${diagram%.mmd}.svg"
  done

  printf '%s\n' '{"ignorePatterns":[{"pattern":"^https?://"}]}' > "$verify_dir/markdown-link-check.json"
  git -c core.quotePath=false diff --cached --name-only -- '*.md' | while IFS= read -r file; do
    npx -y markdown-link-check@3.13.7 \
      --quiet --config "$verify_dir/markdown-link-check.json" "$file"
  done

  kafka='亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'
  diff -u \
    <(git show "$task_base_sha:$kafka" | \
      sed -n '/^### 7\.2 Consumer 基线$/,/^### 7\.3 Raw → Canonical transaction$/p') \
    <(sed -n '/^### 7\.2 Consumer 基线$/,/^### 7\.3 Raw → Canonical transaction$/p' "$kafka")
  ```

  Expected: both diagrams render; every relative file/anchor link in every modified Markdown file resolves. External research URLs are intentionally excluded because this task does not refresh third-party evidence.

- [ ] **Step 5: Check status language and accidental placeholders**

  Run:

  ```bash
  set -euo pipefail
  scene_dir='亿级点位高吞吐架构/6.规则引擎与场景联动'
  base_record='/tmp/scene-runtime-scheduling-partition-failover-base-sha'
  task_base_sha="$(tr -d '\n' < "$base_record")"

  if git diff --cached --unified=0 -- '*.md' | \
    rg -n '^\+[^+].*(TODO|TBD|FIXME|XXX|<placeholder>|待补充|待填写)'; then
    exit 1
  fi
  for file in \
    "$scene_dir/README.md" \
    "$scene_dir/场景联动 Runtime 主规格总纲.md" \
    "$scene_dir/Scene Runtime 调度、分片与故障转移.md" \
    "$scene_dir/SLO故障测试与实施.md"
  do
    rg -n '恢复 ADR.*未冻结|未冻结.*恢复 ADR' "$file"
  done

  git diff --cached --unified=0 -- \
    "$scene_dir/附录-竞品状态方案调研.md" \
    "$scene_dir/附录2-thingsboard规则引擎与CF实现分析.md" | \
  awk '
    /^[+-][^+-]/ &&
    $0 !~ /(导航|规范性边界|本项目的?现行选择|本项目只作简短定位|完整约束|具体语义)/ {
      print
      bad=1
    }
    END { exit bad }
  '
  ```

  Expected: no accidental placeholder token; recovery ADR remains explicitly unfrozen in all four required documents. Legitimate `[待 ADR]` and POC-status language are not placeholders and are intentionally not banned.

- [ ] **Step 6: Run a fresh-reader comprehension review**

  Spawn a fresh subagent with no prior conversation context and give it only README, the main spec, the new chapter and SLO. Require direct answers with supporting headings to these questions:

  1. B 同时拥有 `p0`、`p1` 时，二者 state/timer/offset 是否混在一起？
  2. A 硬故障时，谁发起 `p0` 转移；是否需要 A 回调？
  3. Zombie A 恢复 Kafka 连接后是否自动拿回旧 partition/Actor？
  4. best-effort fencing 能拦住什么，不能撤销什么；`effectId` 为什么不能解决 stale state rollback？
  5. restore 时已过期 timer 如何处理，何时才能开放 `scene_event`？

  Passing answers must mention: independent `PartitionRuntimeContext`, Kafka Coordinator, new session/new token/full restore, local-known-loss boundary plus in-flight Producer/PG exception, stale compacted latest risk, activation queue, and timer installation before restore gate opens. If any answer requires reading the historical design spec or guessing, fix the formal docs and rerun Tasks 10.1–10.5 before repeating the review.

- [ ] **Step 7: Request final document and semantic reviews**

  Invoke `superpowers:requesting-code-review`. Tell both reviewers that the implementation is the complete `git diff --cached` against `TASK_BASE_SHA` (HEAD has intentionally not advanced yet). Ask one reviewer to inspect exact migration/scope/link/anchor compliance and another to inspect runtime semantics, specifically:

  - no main/chapter double truth;
  - p0/p1 isolation and partition-scoped close;
  - callback ordering and fail-closed SUSPENDED;
  - bounded `[start,S0)` restore, staging discard and due-timer activation;
  - no accidental durable fencing/EOS/Actor-framework promise;
  - Action idempotency versus stale state risk remains honest;
  - recovery ADR remains unfrozen.

  Resolve every Critical/Important finding using `superpowers:receiving-code-review`, re-stage only the explicit affected path(s), and rerun Tasks 10.1–10.5 against the complete staged diff. Do not commit individual review fixes. Minor wording findings may be deferred only if they cannot change implementation interpretation, with the reason recorded.

- [ ] **Step 8: Perform final verification after all review fixes**

  Invoke `superpowers:verification-before-completion`. After the complete staged diff has passed Steps 1–7, create the single normative implementation commit:

  ```bash
  set -euo pipefail
  base_record='/tmp/scene-runtime-scheduling-partition-failover-base-sha'
  task_base_sha="$(tr -d '\n' < "$base_record")"

  test -z "$(git diff --name-only)"
  test -z "$(git ls-files --others --exclude-standard)"
  test "$(git -c core.quotePath=false diff --cached --name-only | wc -l)" -eq 11
  git diff --cached --check
  git commit -m 'docs: split scene runtime scheduling specification'

  test "$(git rev-list --count "$task_base_sha"..HEAD)" -eq 1
  test -z "$(git status --porcelain)"
  git diff --check "$task_base_sha"..HEAD

  actual="$(git -c core.quotePath=false diff --name-only "$task_base_sha"..HEAD | sort)"
  expected="$(printf '%s\n' \
    '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/README.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/Scene Runtime 调度、分片与故障转移.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/附录-竞品状态方案调研.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/附录2-thingsboard规则引擎与CF实现分析.md' \
    '亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md' | sort)"
  test "$actual" = "$expected"
  test -z "$(git -c core.quotePath=false diff --name-only --diff-filter=DRCTUXB "$task_base_sha"..HEAD)"
  test "$(git -c core.quotePath=false diff --name-only --diff-filter=A "$task_base_sha"..HEAD | wc -l)" -eq 1
  test "$(git -c core.quotePath=false diff --name-only --diff-filter=M "$task_base_sha"..HEAD | wc -l)" -eq 10

  if git diff --unified=0 "$task_base_sha"..HEAD -- '*.md' | \
    rg -n '^\+[^+].*(TODO|TBD|FIXME|XXX|<placeholder>|待补充|待填写)'; then
    exit 1
  fi
  ```

  Finally rerun the structure/ownership commands from Steps 2–3, render both diagrams again, and run the link checker over the eleven files returned by
  `git -c core.quotePath=false diff --name-only "$task_base_sha"..HEAD -- '*.md'`. Do not reuse an earlier render or staged-diff result as final evidence. The terminal evidence must show:

  ```text
  git status --porcelain        => empty
  git diff --check BASE..HEAD   => exit 0
  changed files                 => exactly 11 allowed Markdown files
  Mermaid renders              => 2/2
  local Markdown links         => all passed
  old single-source phrases    => 0
  recovery ADR                 => still unfrozen in 4 required files
  fresh-reader review          => 5/5 answered from formal docs
  Critical / Important review  => 0 / 0
  implementation commits       => exactly 1 after BASE
  ```
