# Scene Runtime Processing Mode Contract Reconciliation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore the trusted `processingNamespace/processingMode` contract across Scene Admission, wire schemas, state/timer/action safety gates, companion contracts, Kafka integration, and parent-architecture acceptance tests; jointly reconcile the parent P0 Scene transaction boundary with the selected best-effort Scene state model.

**Architecture:** Production Scene accepts only `processingNamespace=production && processingMode=LIVE`. The common `SceneEvent` envelope carries a mandatory `ProcessingContext`; derived events propagate it, Actor/Outbox enforce it again, `BACKFILL` bypasses before matching, and `REPLAY` remains outside Scene V1 rather than introducing a new replay runtime.

**Tech Stack:** Markdown normative contracts, Protobuf-style schema pseudocode, PostgreSQL/Kafka contract descriptions, `rg`, Git.

---

## File responsibilities

- `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`: sole Scene architecture authority; owns Admission decisions, wire, propagation, state/timer/action invariants and POC gates.
- `亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md`: makes the mode gate a precondition to all scope/index matching.
- `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`: defines processing mode versus processing time/event time and restricts live state/timers.
- `亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md`: owns mode failure matrix, observability and rollout gates.
- `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md`: owns Canonical/Scene Kafka integration, ACL and replay isolation.
- `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`: owns the platform-wide trusted mode invariant and clarifies Scene V1 capability.
- `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`: navigation-level invariant only.

### Task 1: Prove the missing contract before editing

**Files:**
- Test: the four active Scene normative documents listed above

- [x] **Step 1: Run the failing wire-contract assertion**

```bash
scene='亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md'
rg -q 'ProcessingContext \{' "$scene" &&
rg -q 'processingNamespace' "$scene" &&
rg -q 'processingMode = LIVE \| BACKFILL \| REPLAY' "$scene"
```

Expected before implementation: non-zero exit because the common Scene envelope has no `ProcessingContext`.

- [x] **Step 2: Run the failing companion-contract assertion**

```bash
for file in \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md'; do
  rg -q 'processingMode' "$file" &&
  rg -q 'BACKFILL' "$file" &&
  rg -q 'REPLAY' "$file"
done
```

Expected before implementation: non-zero exit on the first companion document.

### Task 2: Freeze mode semantics and wire propagation in the Scene master specification

**Files:**
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`

- [x] **Step 1: Add the safety goal and distinguish the three time/mode concepts**

Add to §1 and §3.4:

```text
Scene V1 必须保证只有可信 `production + LIVE` 输入能够修改 live state/timer、
创建正式 Alarm/Action Outbox 或触发外部动作。

`processingMode` 是输入的处理目的与副作用权限；processing time 是 Runtime
观察/串行处理输入的时间；event/device time 是业务采样时间。三者不能互相推导。
普通 redelivery 保留原 mode，不能按重投时刻重新分类。
```

- [x] **Step 2: Put the mode gate before the interest filter**

Replace the Admission description with this ordered decision:

```text
1. 校验 mode 来源、枚举和 namespace/mode 组合；缺失、未知、矛盾或来源不可信时
   fail closed/quarantine，禁止默认 LIVE；
2. `production + BACKFILL`（在线 Raw 拆出的历史 output）与
   `backfill/<jobId> + BACKFILL`（显式 job）均在 interest/candidate matching 前
   完成 deterministic bypass，不产生 scene_event、逐规则 skip 或任何 live mutation；
3. `replay/<id> + REPLAY`、`shadow/<id>` 不进入生产 Scene Topic/state/outbox；
4. 只有 `production + LIVE` 由 Admission 盖章 `SCENE_ID_V2` 后进入
   tenant/fact interest filter。
```

State explicitly that current `SubjectContext` affects scope only and cannot change mode.

- [x] **Step 3: Add a mandatory common ProcessingContext to the wire**

Use this schema in §5.1:

```text
ProcessingContext {
  processingNamespace
  processingMode = LIVE | BACKFILL | REPLAY
}

SceneEvent {
  eventId
  processingContext
  identityAlgorithmVersion = SCENE_ID_V2
  oneof payload {
    SourceDelta          sourceDelta
    LinkedDelta          linkedDelta
    ScheduleEvaluate     scheduleEvaluate
    SubjectLifecycle     subjectLifecycle
    LifecycleTargetDelta lifecycleTargetDelta
  }
}
```

State that every production `scene_event` record must be
`production + LIVE + SCENE_ID_V2`; SourceDelta is invalid without the common context/version,
and the value is not hidden inside `lineage`.

- [x] **Step 4: Freeze derived-event propagation and Actor validation**

Add to §§5.2–5.5 and §5.4:

```text
SourceDelta inherits the trusted Canonical ProcessingContext. LinkedDelta and every local/remote
target bundle copy context/version unchanged. Admission stamps `SCENE_ID_V2`;
Schedule/SubjectLifecycle producers explicitly stamp `production + LIVE + SCENE_ID_V2`.
Every handler validates the common context/version before lookup/evaluation;
non-production-LIVE input cannot mutate arguments, state, timers, alarms or Outbox.
```

- [x] **Step 5: Persist provenance and namespace identities**

Add `processingNamespace/processingMode` and `identityAlgorithmVersion=SCENE_ID_V2`
to the common SceneEvent, complete state metadata, timer business state and new Action Outbox
rows. Prefix every new Scene identity hash with `SCENE_ID_V2`. Change the identity examples to:

```text
transitionId = H(
  SCENE_ID_V2,
  processingNamespace,
  tenantCode,
  bindingId,
  evaluationSubjectKey,
  operatorId,
  sourceEventId | timerFireId | scheduleEvaluationId | lifecycleEvaluationId,
  transitionKind
)

effectId = H(
  SCENE_ID_V2,
  processingNamespace,
  tenantCode,
  bindingId,
  evaluationSubjectKey,
  sourceEventId | timerFireId | scheduleEvaluationId | lifecycleEvaluationId,
  transitionId,
  actionId,
  targetKey
)
```

Require the dispatcher to deliver only rows carrying `production + LIVE`; invalid rows are stopped and alerted. Freeze the remaining identities as follows so every trigger family has the same namespace boundary:

```text
logicalTimerId = H(SCENE_ID_V2, processingNamespace, tenantCode, bindingId,
                   evaluationSubjectKey, operatorId, timerKind)
timerFireId = H(SCENE_ID_V2, logicalTimerId, armedByTriggerId, armedAt, dueAt)

scheduleOccurrenceId = H(SCENE_ID_V2, processingNamespace, tenantCode, scheduleId, plannedFireTs)
scheduleEvaluationId = H(SCENE_ID_V2, scheduleOccurrenceId, bindingId, evaluationSubjectKey)

lifecycleEvaluationId = H(SCENE_ID_V2, processingNamespace, tenantCode, bindingId,
                          evaluationSubjectKey, lifecycle identity fields...)
```

Freeze a one-time identity/wire cutover: pause and fence every old Scene event producer; drain its
captured input frontier and publish callbacks, stop it, and only then capture the final production
`scene_event` end offsets. Drain the old Runtime through that end plus timer callbacks and
DB/Kafka in-flight; then pause/fence the old Action dispatcher from new leases/dequeues, drain its
external/status/PG in-flight, and capture a stable pending-row set before revoking old producer and
dispatcher credentials. Context-less events that cannot drain are isolated with explicit loss approval and are
never defaulted to LIVE. Migrate only trusted state without legacy identity-bearing slots; tombstone
timer/pending-trigger state and use typed cold start, with explicit impact/sign-off for
baseline/duration/cooldown/Alarm behavior. Trusted pending legacy Outbox rows keep their
original `effectId`, are marked `LEGACY_NO_NAMESPACE_V1`, and are delivered without recomputation;
untrusted rows become `BLOCKED_CONTEXT`. Only then enable required-wire/restore/dispatcher gates
and resume all producers on `SCENE_ID_V2`. The first accepted V2 record is the identity
point-of-no-return: subsequent rollback may use only a V2-compatible binary or halt for a forward
fix; it must never re-enable a legacy event/state/timer writer.

- [x] **Step 6: Clarify V1 replay scope, metrics and POC gates**

State that Scene V1 has no isolated replay evaluator. A future one requires a separate ADR with isolated input/state/outbox and a dispatcher that cannot externalize effects. Add counters for Admission decisions and invalid mode combinations, plus POC cases proving BACKFILL/REPLAY/shadow/unknown inputs produce zero live mutation and zero delivery.

- [x] **Step 7: Run the master-spec green assertions**

```bash
scene='亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md'
rg -q 'ProcessingContext \{' "$scene"
rg -q 'processingMode = LIVE \| BACKFILL \| REPLAY' "$scene"
rg -q 'production.*LIVE' "$scene"
rg -q 'BACKFILL.*候选|BACKFILL.*matching' "$scene"
rg -q 'dispatcher.*production.*LIVE|production.*LIVE.*dispatcher' "$scene"
rg -q '缺失.*默认.*LIVE|禁止默认.*LIVE' "$scene"
git diff --check -- "$scene"
```

Expected: all commands exit zero.

- [x] **Step 8: Commit the Scene master change**

```bash
git add '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md'
git commit -m 'docs: 补齐 Scene processing mode 主契约'
```

### Task 3: Align the three Scene companion contracts

**Files:**
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md`

- [x] **Step 1: Make mode validation the matching precondition**

In `控制面与匹配契约.md` put this before §4 matching:

```text
所有 scope/index lookup 的前置条件是已通过可信 ProcessingContext gate。
只有 `production + LIVE` 可以进入 TriggerPosting 求交；BACKFILL 在候选匹配前
bypass，REPLAY/shadow 不进入生产 matching。SubjectContext 只解析当前 scope，
不能把非 LIVE 输入升级为 LIVE。
```

- [x] **Step 2: Define operator and timer mode semantics**

In `算子语义与术语.md` add a ProcessingContext term and distinguish mode, processing time and event time. Require every operator/state/timer path to accept only `production + LIVE`; redelivery keeps its original context; a missing/unknown mode cannot initialize a baseline, refresh freshness, arm/cancel a timer, or emit a transition.

- [x] **Step 3: Add a mode failure and verification matrix**

In `SLO故障测试与实施.md` add rows for:

```text
production+LIVE       -> admitted; normal complete-path assertions
production+BACKFILL   -> bypass before matching; zero scene_event/state/timer/outbox
backfill+BACKFILL     -> explicit-job bypass before matching; same zero effects
replay+REPLAY         -> isolated outside Scene V1; zero production writes/delivery
shadow+LIVE           -> zero production writes/delivery
missing/unknown/mismatch -> fail closed + quarantine/alert; never default LIVE
misrouted event/outbox   -> Actor/dispatcher second gate blocks mutation/delivery
```

Add metrics `scene_admission_mode_total{namespace_class,mode,decision}` and `scene_processing_context_invalid_total{reason}` with bounded labels.

- [x] **Step 4: Run companion green assertions**

```bash
for file in \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md'; do
  rg -q 'processingMode' "$file"
  rg -q 'BACKFILL' "$file"
  rg -q 'REPLAY' "$file"
  rg -q 'production.*LIVE' "$file"
done
git diff --check -- '亿级点位高吞吐架构/6.规则引擎与场景联动'
```

Expected: all commands exit zero.

- [x] **Step 5: Commit the companion changes**

```bash
git add \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md'
git commit -m 'docs: 对齐 Scene processing mode 从属契约'
```

### Task 4: Jointly revise Kafka, parent architecture and README

**Files:**
- Modify: `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md`
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`

- [x] **Step 1: Correct Kafka Scene integration and ACL boundaries**

In Kafka §8 and §§10/12/14 state:

```text
Admission reads trusted Canonical processingNamespace/processingMode before relevance filtering;
only production+LIVE may write production scene_event. production+BACKFILL and
backfill/<jobId>+BACKFILL produce no Scene record;
REPLAY/shadow principals and namespaces cannot write production scene_event/state or Action Outbox.
SceneEvent preserves mandatory ProcessingContext and Actor/dispatcher fail closed on misrouting.
```

Keep Raw→Canonical transactions, Scene best-effort state and the rejection of Scene EOS/RocksDB unchanged.

- [x] **Step 2: Clarify the parent architecture without weakening its hard contract**

Retain §7.2's mode rules. Clarify that “REPLAY only writes isolated state/outbox” is a prohibition on production effects, not a statement that Scene V1 already implements an isolated replay evaluator. In §11.1 and acceptance gates require `production + LIVE` at Admission, common-context propagation and zero production side effects for every other combination.

Replace the old Scene-only P0 rule that grouped state, timer, AlarmSignal/ActionIntent and
Outbox into one PostgreSQL transaction. Freeze the Scene order as Action Outbox-only PG commit,
Actor memory state/timer publication, then one bounded asynchronous producer handoff attempt for
the complete `scene_state`. Immediate rejection is observed but does not block; producer acceptance
and broker ACK/callback are not input-completion predicates. After all
candidates and non-state pack/Outbox failure policies complete, the per-partition sequencer advances
only the continuous frontier. Explicitly state that these boundaries are not one PG/Kafka
transaction; do not weaken Raw→Canonical transactions or non-Scene reducer contracts.

- [x] **Step 3: Add the README navigation invariant**

Add one concise paragraph:

```text
Scene 数据面的生产副作用门禁固定为 `processingNamespace=production &&
processingMode=LIVE`；完整 wire、matching、算子和故障测试分别由主规格及三份
从属契约定义，README 不替代这些规范。
```

- [x] **Step 4: Run cross-directory assertions**

```bash
kafka='亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'
parent='亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md'
readme='亿级点位高吞吐架构/6.规则引擎与场景联动/README.md'
rg -q 'production.*LIVE' "$kafka"
rg -q 'BACKFILL.*Scene|BACKFILL.*scene_event' "$kafka"
rg -q 'Scene V1.*REPLAY|REPLAY.*Scene V1' "$parent"
rg -q 'processingNamespace=production' "$readme"
git diff --check -- "$kafka" "$parent" "$readme"
```

Expected: all commands exit zero.

- [x] **Step 5: Commit the joint revision**

```bash
git add \
  '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md' \
  '亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/README.md'
git commit -m 'docs: 联合冻结 Scene production LIVE 门禁'
```

### Task 5: Verify the complete contract and review the diff

**Files:**
- Verify all modified normative documents and the design/plan records

- [x] **Step 1: Run the complete positive contract check**

```bash
scene_dir='亿级点位高吞吐架构/6.规则引擎与场景联动'
for file in \
  "$scene_dir/场景联动 Runtime 主规格总纲.md" \
  "$scene_dir/控制面与匹配契约.md" \
  "$scene_dir/算子语义与术语.md" \
  "$scene_dir/SLO故障测试与实施.md"; do
  rg -q 'processingMode' "$file"
  rg -q 'BACKFILL' "$file"
  rg -q 'REPLAY' "$file"
  rg -q 'production.*LIVE' "$file"
done
rg -q 'ProcessingContext \{' "$scene_dir/场景联动 Runtime 主规格总纲.md"
rg -q 'processingNamespace' "$scene_dir/场景联动 Runtime 主规格总纲.md"
rg -q 'required identityAlgorithmVersion = SCENE_ID_V2' \
  "$scene_dir/场景联动 Runtime 主规格总纲.md"
rg -q 'state/timer 不在 PG 事务' \
  '亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md'
rg -q 'state/Outbox/input offset 原子切面' \
  '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'
rg -q 'producer 接受、broker ACK/callback 都不属于 completion predicate' \
  '亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md'
```

Expected: zero exit status.

- [x] **Step 2: Run the negative safety check**

```bash
if rg -P -n \
  '缺失.*(?:默认为|降级为).*LIVE|REPLAY.*写入生产.*(?:scene_event|scene_state|Action Outbox)|BACKFILL.*进入.*(?:候选|matching|live state)' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动' \
  '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md' \
  '亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md'; then
  exit 1
fi
```

Expected: no matches and zero exit after the conditional.

- [x] **Step 3: Verify formatting, links and fences**

```bash
git diff --check cec2d15..HEAD
for file in \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md' \
  '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md' \
  '亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md'; do
  fence_count=$(rg -o '```' "$file" | wc -l)
  test $((fence_count % 2)) -eq 0
done
echo FENCES_OK
```

Expected: `FENCES_OK` and zero exit status.

- [x] **Step 4: Inspect scope and cleanliness**

```bash
git diff --stat cec2d15..HEAD
git diff --name-only cec2d15..HEAD
git status --short
```

Expected: only the design, plan and seven named normative/navigation documents are changed; worktree is clean after commits.
