# Remove Canonical Chunking Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove speculative `batchGroup/chunk/assembly` semantics from the V1 architecture and define one complete `CanonicalDataBatch` per logical device report.

**Architecture:** A bounded `DecodeManifest` deterministically enumerates every typed output of one `RawDeviceEnvelope` before downstream side effects. Each valid PropertyReport becomes exactly one compact `CanonicalDataBatch`; multi-sample backfill becomes multiple independent reports, while multiple modules from one sample remain inside one batch through `moduleSpans`. Namespaced identities, Raw aggregate budgets, terminal raw outcomes, trusted processing modes, and per-rule Automation transactions close retry and side-effect boundaries without restoring transport chunks.

**Tech Stack:** Markdown, NATS JetStream, compact schema-indexed Canonical batches, TDengine, PostgreSQL transactional rule state/outbox.

---

### Task 1: Simplify Canonical identity and physical contract

**Files:**
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`

- [x] Replace `reportId/batchGroupId/chunkId` with a bounded `DecodeManifest(outputCount, manifestHash, outputIndex/outputKind/outputId)` plus PropertyReport-specific `sourceReportId/reportIndex/reportCount/reportId`.
- [x] Namespace platform `reportId` by owner/device and a proven source epoch, or fall back to owner/device/`ingestEventId/reportIndex`; never promote a device value directly to a global ID.
- [x] State that one logical report produces exactly one complete `CanonicalDataBatch`, which may contain multiple `moduleSpans` but never arbitrary point-based transport chunks.
- [x] Change `maxPoints/maxBytes` from a splitting threshold into a product-publication and runtime admission guard; add Raw aggregate/decompression/CPU/time budgets checked before outputs.
- [x] Define backfill as `1 Raw → N independent logical reports`, normally one report per sample time; do not combine multiple sample times into one batch.

### Task 2: Rewrite Candidate A/B completion and idempotence boundaries

**Files:**
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`

- [x] Candidate A ACKs Raw only after every manifest output has a durable complete batch/event or per-output quarantine; zero outputs use a unique durable `NO_OUTPUT` outcome.
- [x] Candidate B ACKs Raw only after every manifest output has completed its required sinks/derived events or quarantine; zero outputs follow the same terminal rule.
- [x] Define output transport identity as `(outputId, routeSnapshotId, contractVersion)` and PropertyReport identity as the equivalent `reportId` form.
- [x] Keep one Materializer consumer/checkpoint: all module history/latest decisions for one complete batch succeed before its Canonical delivery is ACKed.
- [x] Preserve raw-level retry safety with deterministic manifests and sink keys; namespace terminal quarantine/`NO_OUTPUT` by `rawProcessingId` and check terminal outcomes before redelivery decode.

### Task 3: Remove Automation assembly and evaluation-job machinery

**Files:**
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`

- [x] Delete `assembly`, `READY/EVALUATED`, missing-chunk timeout, group hash, and final-chunk transaction semantics.
- [x] Define a PropertyReport frontier per `(ownerTenantId, principalDeviceId, ruleReleaseId)`; each rule atomically commits frontier CAS, rule state/timer, Signal/Outbox, outcome, and idempotence.
- [x] ACK an Automation input only after every matched rule has a durable outcome; redelivery skips completed rules and resumes the remainder.
- [x] Freeze trusted `LIVE/BACKFILL/REPLAY` behavior, including BACKFILL frontier-only advancement and isolated replay/backfill automation.
- [x] Keep Audit as an independent durable consumer/checkpoint.

### Task 4: Synchronize topology, operations, Phase 0, and README

**Files:**
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
- Modify: `亿级点位高吞吐架构/README.md`

- [x] Update topology diagrams, capability mapping, ACK table, ordering/replay rules, observability table, Phase 0 spikes, exit conditions, acceptance criteria, and ADR list.
- [x] Replace all user-visible wording that implies module chunks, Canonical chunk messages, batchGroup assembly, or durable evaluation jobs.
- [x] Cover PropertyReport and low-frequency standard outputs, raw/report quarantine, terminal outcomes, conflict side records, processing modes, and ordering frontiers.
- [x] Preserve compact typed lanes, five-state semantics, `moduleSpans`, `BatchLease/BufferLease`, the Candidate A/B Phase 0 choice, and existing replay/action safety rules.

### Task 5: Verify consistency

**Files:**
- Verify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
- Verify: `亿级点位高吞吐架构/README.md`

- [x] Run semantic searches for `batchGroup`, Canonical chunk IDs/counts/hashes, assembly, and READY/EVALUATED job terminology and require zero obsolete references.
- [x] Require positive matches for manifest/output identity, namespaced report identity, `rawProcessingId`, Raw budgets, processing modes, per-rule frontier, `moduleSpans`, and quarantine.
- [x] Run local Markdown link, heading, fence, trailing-whitespace, and scoped diff checks.
- [x] Review the scoped Git status and do not stage, commit, or modify unrelated dirty-worktree files.
