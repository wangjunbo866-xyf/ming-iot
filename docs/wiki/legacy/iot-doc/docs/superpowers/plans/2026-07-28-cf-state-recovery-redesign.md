# CF State Recovery Redesign Implementation Plan

> [!WARNING]
> **已废弃 / 仅供历史审计**
>
> 本计划记录的是已否决的“独立 CF + RocksDB snapshot/Canonical tail”方案，仅供历史审计。其引用的旧编号文件已删除；不得按本计划实施。现行规范见 [2026-07-30 场景联动运行时设计](../../../亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md)。

> **以下正文仅为历史审计记录，其中所有实施指令、步骤与勾选状态均已失效。**

**Goal:** Replace the CF engine's full-window Kafka changelog / TSDB rebuild choice with shared incremental RocksDB state, partition-level incremental snapshots, and Canonical tail replay, then align every cross-chapter conclusion.

**Architecture:** Store each physical source sample once and keep only compact per-state-node accumulators or indexes. Use local RocksDB for the hot path, periodically upload content-addressed partition checkpoints, and restore from a compatible checkpoint plus a bounded Canonical tail; keep TSDB only as an explicitly non-exact disaster-recovery source. Preserve deterministic CF execution as a platform invariant and make the state/input/output consistency cut explicit.

**Tech Stack:** Markdown architecture documents, Kafka partitions and offsets, RocksDB WriteBatch/checkpoint/SST, object storage manifests, deterministic script sandbox.

---

## File map

- `亿级点位高吞吐架构/6.规则引擎与场景联动/5.共享计算与框架选型.md`: authoritative CF state shape, recovery protocol, framework decision, and trade-offs.
- `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`: series-level summary and reading index.
- `亿级点位高吞吐架构/6.规则引擎与场景联动/1.分层与边界.md`: scene/CF/Flink boundary and dynamic-plan wording.
- `亿级点位高吞吐架构/6.规则引擎与场景联动/3.状态语义与容量.md`: scene/CF boundary and capacity wording.
- `亿级点位高吞吐架构/6.规则引擎与场景联动/4.物理状态子系统.md`: cross-reference from scene state to the distinct CF recovery model.
- `亿级点位高吞吐架构/6.规则引擎与场景联动/6.配额正确性与可观测性.md`: quotas, recovery correctness, and operational metrics.

The series is currently an untracked working draft created by splitting a deleted source document. Work in place and do not commit or stage these user-owned changes.

### Task 1: Rewrite the authoritative CF design

**Files:**
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/5.共享计算与框架选型.md`

- [x] **Step 1: Replace the CF state model**

Define `SourceSeries → Window/Pane → Reducer → Emit`, with physical `seriesId` and `stateNodeId` sharing below `ComputationPlan`. Specify fixed RocksDB stores for `samples`, `series_meta`, `node_state`, `aux_index`, `pending_outputs`, and `partition_meta`; one input must append the source sample once and update all affected small deltas in one WriteBatch.

- [x] **Step 2: Replace the A/B recovery choice**

Make partition-level incremental RocksDB checkpoint plus Canonical tail replay the default. Define pure Canonical bounded replay as the no-snapshot fallback, graceful handoff as an optimization, warm standby as a strict-RTO option, delta changelog as an optional high-cost mode, and TSDB rebuild as non-exact disaster recovery only.

- [x] **Step 3: Close the state/input/output cut**

Require a snapshot only at `stateNextOffset == outputDurableNextOffset`; document restore as pause → restore compatible snapshot at `Os` → state-only replay `[Os,Oc)` → normal replay from `Oc` → resume at live. List manifest identity, input offset, event-time, plan/config, output frontier, SST checksum, and fencing fields.

- [x] **Step 4: Update script and framework conclusions**

State that I/O, random, and wall-clock scripts are rejected rather than redirected to a changelog. Explain that Flink can technically host a generic dynamic-plan runtime but remains an alternative because broadcast-plan scale and JVM/script integration are costs; state that custom Go owns checkpoint coordination, manifest fencing, state migration, output alignment, and GC.

### Task 2: Align the rest of the series

**Files:**
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/1.分层与边界.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/3.状态语义与容量.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/4.物理状态子系统.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/6.配额正确性与可观测性.md`

- [x] **Step 1: Align the series summary**

Replace every statement that fixes CF recovery to Kafka changelog or TSDB rebuild with “shared local state + incremental snapshot + Canonical tail,” while retaining scene replay and PostgreSQL delayed-action recovery as separate mechanisms.

- [x] **Step 2: Align state-boundary and capacity wording**

Change the chapter 3 references from `script rolling + changelog` to shared source-series state and incremental checkpointing. Keep the warning that window state scales with samples, but note that physical sharing removes per-plan sample duplication rather than removing the underlying sample volume.

- [x] **Step 3: Align scene and operations cross-references**

Keep chapter 4's scene-engine replay protocol unchanged and only correct its CF references. In chapter 6, add quotas for source-series bytes, distinct state nodes, checkpoint delta/upload time, restore bytes/tail length, and snapshot age; update correctness and metrics accordingly.

### Task 3: Verify the document set

**Files:**
- Verify all five modified documents above.

- [x] **Step 1: Scan for stale conclusions**

Run:

```bash
rg -n 'CF 引擎.?=.?Kafka changelog|CF=Kafka changelog|script rolling \+ changelog|方案 B.*TSDB 重建|非确定.*退回.*changelog|§7，Kafka changelog|§7.*script rolling \+ Kafka changelog' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动'
```

Expected: no stale statement that presents these as the selected/default recovery model. Historical comparison and explicitly rejected alternatives may remain only when labeled as such.

- [x] **Step 2: Scan required invariants**

Run:

```bash
rg -n 'stateNextOffset|outputDurableNextOffset|Canonical 尾部|增量快照|SourceSeries|stateNodeId|TSDB.*非精确|warm standby' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动'
```

Expected: the authoritative chapter contains all recovery-cut and state-sharing invariants, and README/chapters 3, 4, and 6 contain consistent summaries.

- [x] **Step 3: Check formatting and placeholders**

Run:

```bash
rg -n 'TODO|TBD|待补|[[:blank:]]+$' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动'
```

Expected: no newly introduced placeholders or trailing whitespace.

- [x] **Step 4: Review the final diff**

Read all changed sections together and verify that:

1. scene engine recovery remains `Canonical replay`;
2. CF default recovery is `incremental checkpoint + Canonical tail`;
3. delayed action/terminal state remains PostgreSQL;
4. TSDB is never called an exact/default recovery source;
5. nondeterministic CF is rejected;
6. no unrelated user-owned changes are staged or committed.
