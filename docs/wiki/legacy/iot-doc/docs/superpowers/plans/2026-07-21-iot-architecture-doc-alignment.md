# IoT Architecture Documentation Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align the enterprise IoT architecture document with the accepted workspace, authorization, protocol isolation, compact batch, and capacity decisions.

**Architecture:** Keep `saas`, `things`, and `protocol` as explicit Go Workspace domain boundaries. Separate logical runtime responsibilities from deployable processes, use compact schema-indexed Canonical batches on the high-throughput path, and leave the cross-process Canonical boundary subject to Phase 0 throughput evidence.

**Tech Stack:** Markdown, Go Workspace conventions, NATS JetStream, TDengine, Kubernetes.

---

### Task 1: Align workspace and service ownership

**Files:**
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`

- [x] Replace the `platform/` tree with `backend/{saas,things,protocol,share,cmd}`.
- [x] Restore explicit API/RPC ownership under `saas/service/{apisvr,syssvr}` and `things/service/{apisvr,dmsvr}`.
- [x] Remove `control-api` and `data-worker` as catch-all module names.
- [x] Describe ProtocolApplication as a repeatable deployment type rather than one singleton protocol service.

### Task 2: Add hierarchical tenant authorization

**Files:**
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
- Reference: `亿级点位高吞吐架构/authz/tenant-hierarchy.md`

- [x] Define `sys_tenant.parent_id` as the authoritative edge and the closure table as the derived authorization index.
- [x] Keep department hierarchy tenant-scoped and separate from the tenant tree.
- [x] State that `saas/service/syssvr` owns hierarchy transactions and scope compilation.
- [x] State that IoT data retains its direct owner tenant (which may itself have children) and that `things` consumes a versioned authorization scope instead of querying hierarchy tables.

### Task 3: Freeze the compact Canonical batch contract

**Files:**
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
- Reference: `亿级点位高吞吐架构/small-object-large-object/`

- [x] Superseded on 2026-07-22: one logical report now produces exactly one complete `CanonicalDataBatch`; point/byte limits are admission guards rather than transport-splitting thresholds.
- [x] Specify schema-stable point ordinals, typed scalar lanes, text arena, five-state plane, independent quality/value presence, and batch limits.
- [x] Forbid hot-path maps, `any`, point-name strings, JSON, and per-point nested messages.
- [x] Separate in-process pooled ownership from the compact cross-process wire representation.
- [x] Allow protocol decode and time-series materialization to fuse in one process while keeping their code boundaries.

### Task 4: Synchronize the overview and capacity language

**Files:**
- Modify: `亿级点位高吞吐架构/README.md`

- [x] Replace `data-worker` in the overview with ProtocolApplication and time-series materialization terminology.
- [x] Explain that 16.67 million points/s is the Phase 1 double-peak qualification target, not steady-state traffic.
- [x] Add the compact-batch and no-small-object invariant.

### Task 5: Verify documentation consistency

**Files:**
- Verify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
- Verify: `亿级点位高吞吐架构/README.md`

- [x] Run `rg -n "data-worker|control-api|platform/|标准点位|repeated Point|map\\[string\\]any"` and classify every remaining occurrence.
- [x] Run a script to verify local Markdown links resolve.
- [x] Recalculate `reports/s × points/report` for average, active steady, 60-second peak, and double-peak qualification rows.
- [x] Inspect `git diff --check` and the scoped diff for accidental edits outside the two target documents and this plan.
