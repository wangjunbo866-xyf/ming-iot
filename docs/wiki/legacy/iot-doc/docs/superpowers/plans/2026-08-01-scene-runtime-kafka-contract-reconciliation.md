# Scene Runtime Kafka Contract Reconciliation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the RocksDB/Kafka-EOS and immutable-affiliation Scene Runtime contracts and make all normative documents expose one executable `scene_event`/`scene_state` plus current-`SubjectContext` V1 contract while preserving Raw → Canonical transactions.

**Architecture:** Treat the scene master specification as the owner of Scene Runtime semantics, with the Kafka design and parent architecture consuming that contract. Replace, rather than rename, the old automation topics and recovery protocol; use processing-time current affiliation instead of immutable affiliation snapshots/barriers; retain Kafka transactions for non-Scene flows such as Raw → Canonical.

**Tech Stack:** Markdown specifications, Git, `rg`, `sed`, `git diff --check`

---

## File map

- Modify `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md`: remove the old Scene RocksDB/EOS design, install the current topic/integration contract, and preserve non-Scene Kafka semantics.
- Modify `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`: remove immutable affiliation snapshot/cutover as a Scene matching requirement and define backlog/redelivery by current `SubjectContext`.
- Modify `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`: add the cross-directory replacement matrix and reciprocal consistency rule.
- Modify `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`: replace unilateral precedence with same-change joint revision.
- Reference `docs/superpowers/specs/2026-08-01-scene-runtime-kafka-contract-reconciliation-design.md`: approved design and scope boundary; do not alter during implementation.

### Task 1: Correct Kafka scope and authority protocol

**Files:**
- Modify: `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md:3-16`
- Modify: `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md:18-56`
- Modify: `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md:93-120`
- Modify: `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md:146-155`

- [ ] **Step 1: Capture the conflicting baseline**

Run:

```bash
rg -n '深度绑定 Kafka|本地 RocksDB|state \+ input next-offset|按本文第 8 节收敛|不能单方面' \
  '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'
```

Expected: matches in the scope, conclusion diagram, §1.2, and solution comparison proving that the old contract is still normative.

- [ ] **Step 2: Replace the Scene-specific scope and conclusion**

Keep Raw → Canonical transaction wording unchanged. Replace the Go Automation Runtime branch with this semantic content:

```text
Canonical / lifecycle / schedule facts
  -> Scene admission and normalization
  -> scene_event
  -> Go Scene Runtime
       - partition-local Actor in-memory state
       - asynchronous best-effort scene_state full-state append
       - PG Action Outbox + deterministic effectId
       - no RocksDB, Kafka EOS, or cross-topic transaction
```

Change the scope sentence so the Kafka document covers “Scene Runtime 的 Kafka 接入与物理边界” and explicitly delegates Scene state/recovery semantics to the scene master specification.

- [ ] **Step 3: Rewrite §1.2 as a completed joint revision**

Preserve the rule that conflicts must be repaired in the same change. Replace the old forward-overwrite matrix with a decision block containing:

```markdown
- Scene Runtime 的状态后端、恢复、Topic、offset 与动作提交语义由场景主规格定义；
- 本文只细化与该主规格一致的 Kafka 物理边界；
- 本次联合修订撤销本文旧 §5.3/§8 的 RocksDB/EOS 方案；
- Raw → Canonical transaction、Domain Outbox 和非 Scene Kafka 契约不受影响。
```

Update the comparison table so Kafka Streams + RocksDB/EOS remains only a rejected comparison, never a Go Scene V1 requirement.

- [ ] **Step 4: Verify the authority protocol**

Run:

```bash
sed -n '1,170p' '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'
```

Expected: one scope boundary, a current Scene diagram, reciprocal same-change conflict repair, and unchanged Raw → Canonical transaction claims.

- [ ] **Step 5: Commit the scope correction**

```bash
git add -- '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'
git commit -m 'docs: 修正 Kafka 设计的 Scene Runtime 边界'
```

### Task 2: Replace Kafka Scene topics, keys, and producer coupling

**Files:**
- Modify: `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md:241-383`
- Modify: `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md:387-579`
- Modify: `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md:628-655`

- [ ] **Step 1: Replace §5.3 with the current physical catalog**

The normative table must contain only:

| Logical role | Physical contract | Delivery semantics |
| --- | --- | --- |
| Scene data lane | `scene_event` | one physical topic; SOURCE/LINKED/Schedule/lifecycle/timer kinds; at-least-once/best-effort failure windows per master spec |
| Scene state | `scene_state` | compacted full state; asynchronous best-effort append; not atomic with event offsets |
| Binding propagation | `scene_binding_lifecycle` | PG current Binding + transactional lifecycle Outbox relay; configuration convergence only |

Immediately follow it with the complete old-contract disposition matrix from the approved design. State that old and new records are not wire-compatible and that no alias or dual-write is permitted.

- [ ] **Step 2: Remove old replay and key contracts**

Rewrite §5.5 so generic Raw/Canonical replay namespaces remain available but Scene V1 does not create `iot.replay.*.automation.input/state`, does not promise Canonical historical replay, and cannot promote replay output into LIVE.

Replace `AutomationSubjectKeyV1` and `AutomationChangelogKeyV1` as Scene normative codecs with references to the scene master specification §14.1:

```text
scene_event route key = routeKeyBytes(tenantCode, subjectKey)
scene_state key       = stateKeyBytes(tenantCode, bindingId, evaluationSubjectKey)
partition             = toPositive(kafkaMurmur2(routeKeyBytes)) % partitionCount
```

Keep Raw and Domain key contracts unchanged. Rename §6.4 to cover Canonical/Domain envelopes only; Scene uses its own `SceneEvent` and `SceneState` contracts from the master specification.

Remove `affiliationBindingId`, immutable affiliation snapshot and affiliation-generation fields wherever the Kafka document presents them as Scene matching inputs. Replace them with current `SubjectContext`/`aggregateVersion` semantics and state that delayed Linked, restore, and lifecycle paths revalidate current scope.

- [ ] **Step 3: Decouple Scene admission from Raw → Canonical transaction**

In §7.3, delete the step that writes `automation.input.live` in the Raw transaction. Preserve atomic submission of Canonical/quarantine/outcome plus Raw next-offset. Add this boundary after the transaction algorithm:

```markdown
Scene admission is a downstream contract. It normalizes admitted facts into `scene_event` and is not part of the Raw → Canonical Kafka transaction; Scene V1 does not require a Canonical/`scene_event` cross-Topic transaction.
```

- [ ] **Step 4: Run focused contract scans**

Run:

```bash
rg -n 'automation\.input\.live|automation\.state|automation\.checkpoint|commit-attestation|snapshot-manifest|state-migration-progress|automation\.schedule' \
  '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'
```

Expected: matches occur only in the explicitly labelled old-contract disposition matrix; no active topic catalog, replay example, key contract, or transaction algorithm uses them.

- [ ] **Step 5: Commit the topic replacement**

```bash
git add -- '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'
git commit -m 'docs: 替换 Scene Runtime Kafka 物理契约'
```

### Task 3: Replace Kafka §8 and remove downstream EOS requirements

**Files:**
- Modify: `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md:707-1017`
- Modify: `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md:1060-1097`
- Modify: `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md:1099-1328`

- [ ] **Step 1: Replace all of §8 with a Scene V1 integration summary**

Delete the old §8.1–§8.11 implementation protocol. The replacement section must have these subsections and no second recovery design:

```markdown
### 8.1 规范边界与统一入口
Reference master spec §§4–5 and require all Scene event kinds to use `scene_event`.

### 8.2 状态与恢复
Reference master spec §§8–9; in-memory Actor + async compacted `scene_state`; state-first restore; no RocksDB/snapshot/checkpoint/EOS.

### 8.3 Event commit 与动作顺序
Reference master spec §§12.3 and 13; PG Outbox commit gates semantic transition, then memory publish/timer update/async state send, then event offset; document the accepted best-effort windows.

### 8.4 Rebalance、配置与生命周期
Reference master spec §§5.5, 6, 11, 13.3; local generation guard, current Binding, no durable owner epoch, release artifact, config barrier, or immutable affiliation snapshot.

### 8.5 已撤销的旧协议
Repeat the concise retirement statement and link to the disposition matrix in §5.3.
```

- [ ] **Step 2: Update ACL and naming examples**

Remove Scene `transactional.id`, checkpoint/attestation/migration producer permissions, and replay state permissions. The Scene principal should read/write only the current topics required by the master specification plus PG Outbox access. Keep ProtocolApplication transactional identity and permissions unchanged.

- [ ] **Step 3: Update capacity, errors, metrics, tests, and risks**

Replace old terms with current measurable quantities:

```text
scene_event records/s and bytes/s
scene_state append records/s and bytes/s
PG Action Outbox rows/s
state-before-event restore bytes and RTO
async state send failures and callback latency
rebalance late-send/overwrite window
```

Remove tests and risks for RocksDB deletion, remote snapshot digest, checkpoint/LSO comparison, commit-attestation projector, PREPARED gate, release barrier, state-generation GC, and Scene transactional producer fencing. Retain broker, ProtocolApplication transaction, Raw retention, Domain Outbox, command, generic consumer, and security tests.

Add Scene tests matching master spec §19 items 6–13: `scene_state` size/throughput, state send failure, pack/commit failure, restore order, local generation guard, late state overwrite window, and `effectId` convergence.

- [ ] **Step 4: Verify no retired Scene protocol remains normative**

Run:

```bash
rg -n -i 'rocksdb|checkpoint|commit-attestation|snapshot-manifest|state-migration-progress|prepared_batch_effect|automation_committed_batch|state-generation GC' \
  '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'
```

Expected: RocksDB may appear only in the rejected Kafka Streams comparison or old-contract disposition; all other retired terms appear only in explicitly historical/withdrawn text. No ACL, capacity formula, failure test, or risk control requires them for Scene V1.

- [ ] **Step 5: Commit the runtime protocol replacement**

```bash
git add -- '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'
git commit -m 'docs: 撤销 Scene RocksDB 与 Kafka EOS 协议'
```

### Task 4: Align the parent architecture with current affiliation matching

**Files:**
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md:250-265`
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md:417-424`
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md:446-519`
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md:818-870`
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md:1182-1188`
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md:1263-1299`

- [ ] **Step 1: Remove immutable affiliation from the Scene input contract**

Remove `affiliationBindingId` from the Canonical envelope as a Scene scope source, delete the requirement to resolve `DeviceAffiliationSnapshot` by `rawRef`, and stop making accepted transitions inherit immutable affiliation. Replace the former §7.2 rule with:

```markdown
Canonical freezes device identity and source provenance, not Product/Project/Area/group membership for Scene matching. Scene admission emits SOURCE_DELTA without an affiliation snapshot; the source lane resolves current `SubjectContext` when processing and fails closed when current context cannot be verified.
```

- [ ] **Step 2: Remove affiliation from Raw offset cutover requirements**

In ProtocolApplication routing and release timeline sections, remove affiliation snapshot/cache/cutover duties. Keep protocol, model, processing-mode and ownership histories or barriers where independently required. State explicitly that an affiliation change does not rewrite Raw/Canonical and does not create an Automation effective offset.

- [ ] **Step 3: Rewrite migration/backlog behavior and acceptance tests**

Replace immutable-snapshot expectations with:

```text
SubjectLifecycle updates the partition-local current SubjectContext.
Backlog/redelivery is matched against the context known at processing time.
Delayed Linked and restored state must revalidate current scope/version.
An older payload cannot resurrect membership removed by a newer aggregateVersion.
```

Delete Phase 0 and ADR gates that freeze `DeviceAffiliationSnapshot/AffiliationCutover`. Replace cutover tests with current-context tests for migration followed by backlog/redelivery, delayed Linked, restore reconciliation, lifecycle duplication/out-of-order delivery, and version gaps.

- [ ] **Step 4: Verify the parent architecture no longer gives the opposite result**

Run:

```bash
rg -n 'affiliationBindingId|DeviceAffiliationSnapshot|AffiliationCutover|不可变.*affiliation|不可变.*归属|当前 SubjectContext|current affiliation|backlog|redelivery' \
  '亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md'
```

Expected: no current Scene matching, backlog, replay, Phase 0, or acceptance-test requirement uses immutable affiliation or an affiliation offset barrier; current-context processing is explicit. Any retained `ownershipBindingId` or RawOffsetBarrier text is clearly unrelated to affiliation matching.

- [ ] **Step 5: Commit the parent-architecture correction**

```bash
git add -- '亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md'
git commit -m 'docs: 统一场景归属匹配为 current context'
```

### Task 5: Install reciprocal consistency rules in Scene documents

**Files:**
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md:1509-1525`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md:7-24`

- [ ] **Step 1: Add the cross-directory replacement matrix to the master spec**

After the current document hierarchy in §18, add a “跨目录规范一致性” subsection. It must link to the Kafka design and parent architecture, state the same-change repair rule, preserve the Raw → Canonical exception, and include all eleven rows from the approved old-contract disposition matrix, including the three affiliation rows.

- [ ] **Step 2: Replace the README unilateral precedence paragraph**

Use this rule:

```markdown
主规格拥有 scene-linkage 架构决策；Kafka 设计拥有非 Scene Kafka 物理契约。任一文档发现涉及同一 Runtime 的跨目录冲突时，必须在同一次变更中显式修订全部受影响的规范性文档，不能依靠权威排序或后写文档自动覆盖。同步完成前，冲突部分不得作为已冻结实施契约。
```

Add direct links from both documents to each other.

- [ ] **Step 3: Verify reciprocal links and rules**

Run:

```bash
rg -n '同一次变更|不能依靠.*覆盖|Raw.*Canonical|automation\.input\.live|scene_event|snapshot-manifest' \
  '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/README.md'
```

Expected: both normative documents express the same repair rule; the replacement matrix is visible from both directory entry points; Raw → Canonical remains explicitly outside the withdrawal.

- [ ] **Step 4: Commit the Scene-side joint revision**

```bash
git add -- \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/README.md'
git commit -m 'docs: 统一 Scene 与 Kafka 冲突解决协议'
```

### Task 6: Cross-document verification

**Files:**
- Verify: all four modified normative documents
- Verify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
- Verify: `docs/superpowers/specs/2026-08-01-scene-runtime-kafka-contract-reconciliation-design.md`

- [ ] **Step 1: Run formatting and repository checks**

```bash
git diff HEAD~4 --check
git status --short
```

Expected: no whitespace errors; only intentional commits/files are present and the worktree is clean.

- [ ] **Step 2: Prove retired topic names are historical only**

```bash
rg -n 'automation\.input\.live|automation\.state|automation\.checkpoint|automation\.commit-attestation|automation\.snapshot-manifest|automation\.state-migration-progress|automation\.schedule' \
  '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md'
```

Expected: every match is inside an explicitly labelled replacement/retirement matrix, never a current catalog or algorithm.

- [ ] **Step 3: Prove the current Scene contract is discoverable**

```bash
rg -n 'scene_event|scene_state|scene_binding_lifecycle|无 `transactional.id`|无 `sendOffsetsToTransaction`|无跨 Topic 事务' \
  '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md'
```

Expected: Kafka §5.3/§8 and the master spec agree on topic names, async state, and non-transactional Scene delivery.

- [ ] **Step 4: Prove affiliation backlog semantics are unified**

```bash
rg -n 'DeviceAffiliationSnapshot|AffiliationCutover|affiliationBindingId|current SubjectContext|current affiliation|backlog.*current|redelivery.*current' \
  '亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md' \
  '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md'
```

Expected: current normative text gives one result—migration backlog/redelivery uses processing-time current affiliation; immutable snapshot/barrier names occur only in an explicit retired-contract matrix.

- [ ] **Step 5: Prove Raw → Canonical transaction survived**

```bash
sed -n '/### 7.3 Raw → Canonical transaction/,/### 7.4 外部 sink/p' \
  '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'
```

Expected: Canonical/quarantine/outcome and Raw next-offset still commit atomically; only Scene admission is outside that transaction.

- [ ] **Step 6: Review the complete implementation diff**

```bash
git diff 2e81f7a..HEAD --stat
git diff 2e81f7a..HEAD -- \
  '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md' \
  '亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/README.md'
```

Expected: the diff implements every section of the approved design and contains no unrelated edits.
