# Scene Linkage Documentation Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract the still-valid material from the legacy scene-linkage series into three focused companion documents, make the migrated runtime specification the sole architecture authority, and remove the obsolete 1–6 documents without leaving broken links or normative legacy conclusions.

**Architecture:** Keep `场景联动 Runtime 主规格总纲.md` as the only architecture decision source. The three new documents are subordinate implementation companions: control-plane/matching contracts, operator semantics/glossary, and SLO/failure/rollout gates. Research appendices remain non-normative evidence; the old numbered series is deleted after extraction.

**Tech Stack:** Markdown, repository-local relative links, `rg`, Git diff/rename detection.

---

### Task 1: Extract control-plane and matching contracts

**Files:**
- Create: `亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/2.scope与匹配引擎.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`

- [ ] **Step 1: Declare document authority and vocabulary**

  State that the runtime specification is authoritative. Define `SceneTemplate` as an optional authoring/reuse artifact with no runtime state identity, and `SceneBinding` as the stable runtime/configuration entity with immutable owner project and stable `bindingId`.

- [ ] **Step 2: Extract the valid organization facts**

  Document the three approved axes (`tenant→project→device`, `tenant→project→area→device`, `tenant→product→device`), current `SubjectContext`, area-path semantics, and the prohibition on cross-project matching. Do not copy group/tag/immutable affiliation snapshot assumptions.

- [ ] **Step 3: Freeze matching-set semantics**

  Define matching as the intersection of current scope postings, exact fact dependency postings, and enabled `BindingCtx`; define `TRIGGER` versus `CONTEXT_ONLY`, one complete read view, target grouping, and fail-closed behavior. V1 trigger facts use exact `(factType,factKey)` dependencies; whole-record/wildcard triggers are rejected unless represented by an explicit bounded typed fact.

- [ ] **Step 4: Preserve overlap and capacity rules**

  Retain publish-time rejection of overlapping enabled bindings in the same template/subject space, candidate/fanout estimation, and an example showing a multi-fact SOURCE bundle becoming one target bundle. Remove release snapshot, reverse affiliation snapshot, and activation barrier material.

- [ ] **Step 5: Verify prohibited legacy terms are absent**

  Run:

  ```bash
  rg -n 'bindingReleaseId|AffiliationSnapshot|effectiveFromOffset|ReverseScopeIndex|offset barrier|rule × device' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md'
  ```

  Expected: no positive architecture use; `rule × device` may appear only in an explicit prohibition.

### Task 2: Extract operator semantics and glossary

**Files:**
- Create: `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/3.状态语义与容量.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`

- [ ] **Step 1: Define shared terms**

  Define scope selector, source subject, evaluation subject, argument source, latest-value join, semantic trigger time, NOT_READY, transition, action acceptance, and cold bootstrap.

- [ ] **Step 2: Define bounded operator state machines**

  Give minimal state, arm/update/cancel/fire rules, and one example for `edge/change`, `duration`, `debounce`, `cooldown`, and `hysteresis`. Keep timer handles in memory and recover business timing state from `scene_state`; do not introduce PG timer or RocksDB.

- [ ] **Step 3: Define missing/freshness and initialization behavior**

  Explain baseline-only cold start for edge/hysteresis, approximate reconstruction for duration/debounce, cooldown reset on missing state, latest timestamp/version ordering, NOT_READY behavior, and why event-time JOIN/session/CEP belongs to Flink.

- [ ] **Step 4: Separate reliable delayed actions**

  State that delayed action acceptance is a PG Action Outbox concern and is not a duration/debounce timer. Scope exit does not cancel committed output unless a compensating action is explicitly modeled.

- [ ] **Step 5: Verify physical-model leakage is absent**

  Run:

  ```bash
  rg -n 'scene_timer|Canonical bounded replay|snapshot.?\+.?tail|bindingReleaseId|RocksDB' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md'
  ```

  Expected: no matches except explicit rejected-model notes, if any.

### Task 3: Extract SLO, failure-test, and rollout gates

**Files:**
- Create: `亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/6.配额正确性与可观测性.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`

- [ ] **Step 1: Split frozen limits from POC candidates**

  Carry over only approved hard limits such as 32 KiB state and 1000 rolling samples. Put historical `5min/30min` lag and `25%` tenant-share values in a clearly labeled candidate table that is non-binding until POC sign-off.

- [ ] **Step 2: Define overload behavior by boundary**

  Require publish-time rejection for statically provable excess, whole-record/bundle quarantine for runtime completeness violations, per-tenant admission/fair scheduling for resource isolation, and no specificity-based partial candidate shedding.

- [ ] **Step 3: Build a failure-window matrix**

  Cover ProjectIndex gaps/rebuild, stale Linked, lifecycle target loss, state-send loss, Outbox pre/post commit, timer callback after close, rebalance stale owner, schedule page failure, and partition-route mismatch. Link every row to the relevant POC gate in the runtime specification.

- [ ] **Step 4: Define implementation artifacts and rollout sequence**

  List the required follow-on artifacts: PG DDL, Protobuf contracts, Things current-store APIs, topic/bootstrap configuration, Actor/timer/quarantine packages, feature flags, canary, rollback, and named ownership. Do not invent schemas in this migration.

- [ ] **Step 5: Verify rejected recovery models are absent**

  Run:

  ```bash
  rg -n 'Canonical bounded replay|SourceSeries|stateNode|snapshot.?\+.?tail|PG.*timer|scene_timer' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md'
  ```

  Expected: no positive architecture use.

### Task 4: Make the migrated specification and README authoritative

**Files:**
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`

- [ ] **Step 1: Replace the future six-file migration plan**

  Rewrite specification §18 so it records the completed single-spec migration: the specification is normative; the three companion documents are subordinate; appendices are non-normative; the old numbered documents are removed.

- [ ] **Step 2: Rewrite README as the stable entry point**

  Remove the old three-runtime/recovery summary and numbered-series navigation. Link the sole architecture specification, the three companion documents, and the two research appendices. State that recovery ADR remains unfrozen pending §19 evidence.

- [ ] **Step 3: Check navigation targets**

  Run `rg -n '\]\('` on README and verify every repository-relative target exists.

### Task 5: De-normativize the research appendices

**Files:**
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/附录-竞品状态方案调研.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/附录2-thingsboard规则引擎与CF实现分析.md`

- [ ] **Step 1: Replace legacy navigation headers**

  Link only README, the current runtime specification, and the peer appendix. Add an explicit `非规范性研究资料` status.

- [ ] **Step 2: Remove stale conclusions about this platform**

  Replace assertions that this platform uses Canonical replay, independent Go CF, or snapshot+tail with a short note that those were historical alternatives rejected by the current specification.

- [ ] **Step 3: Preserve source evidence**

  Keep third-party source paths, behavior descriptions, and comparison evidence. In the TB appendix, retain Kafka-versus-in-memory/RocksDB deployment distinctions and state-before-event evidence; do not turn those facts into a second architecture decision source.

### Task 6: Remove the obsolete numbered documents and mark the old plan historical

**Files:**
- Delete: `亿级点位高吞吐架构/6.规则引擎与场景联动/1.分层与边界.md`
- Delete: `亿级点位高吞吐架构/6.规则引擎与场景联动/2.scope与匹配引擎.md`
- Delete: `亿级点位高吞吐架构/6.规则引擎与场景联动/3.状态语义与容量.md`
- Delete: `亿级点位高吞吐架构/6.规则引擎与场景联动/4.物理状态子系统.md`
- Delete: `亿级点位高吞吐架构/6.规则引擎与场景联动/5.共享计算与框架选型.md`
- Delete: `亿级点位高吞吐架构/6.规则引擎与场景联动/6.配额正确性与可观测性.md`
- Modify: `docs/superpowers/plans/2026-07-28-cf-state-recovery-redesign.md`

- [ ] **Step 1: Add a superseded notice to the historical plan**

  State that the plan describes a rejected independent-CF/snapshot design and that its referenced numbered files have been removed; link the current runtime specification.

- [ ] **Step 2: Delete all six obsolete files**

  Use explicit file paths only. Do not recursively delete the directory or either appendix.

### Task 7: Verify migration integrity

**Files:**
- Verify all files under `亿级点位高吞吐架构/6.规则引擎与场景联动/`

- [ ] **Step 1: Verify the final file set**

  Expected active directory:

  ```text
  README.md
  场景联动 Runtime 主规格总纲.md
  控制面与匹配契约.md
  算子语义与术语.md
  SLO故障测试与实施.md
  附录-竞品状态方案调研.md
  附录2-thingsboard规则引擎与CF实现分析.md
  ```

- [ ] **Step 2: Check for stale filenames and positive legacy architecture terms**

  Run:

  ```bash
  rg -n --glob '*.md' \
    '1\.分层与边界|2\.scope与匹配引擎|3\.状态语义与容量|4\.物理状态子系统|5\.共享计算与框架选型|6\.配额正确性与可观测性' .

  rg -n --glob '*.md' \
    'bindingReleaseId|DeviceAffiliationSnapshot|Canonical bounded replay|SourceSeries/stateNode|scene_timer' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动'
  ```

  Expected: filename references remain only in explicitly marked historical material; legacy architecture terms remain only in rejected/history/research context.

- [ ] **Step 3: Check Markdown and Git diff**

  Run `git diff --check`, verify all code fences are paired, resolve every README/appendix relative Markdown link, and inspect `git diff --find-renames --summary` plus `git status --short`.

- [ ] **Step 4: Do not commit from the dirty shared worktree**

  Leave all changes unstaged and report the exact created, modified, and deleted files for user review.
