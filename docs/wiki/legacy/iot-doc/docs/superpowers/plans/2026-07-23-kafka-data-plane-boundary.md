# Kafka Data Plane Boundary Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Update the enterprise IoT architecture documentation to freeze Kafka, a durable Canonical stream, 72-hour hot Raw retention, and the V1 single-cluster logical isolation boundary.

**Architecture:** Raw and Canonical remain in one high-throughput Kafka data plane. saas/things low-rate domain events use separate topic, principal, group, quota, and client namespaces on the same physical V1 cluster; independent bootstrap-server configuration keeps a later physical split possible. Raw is the short-term original evidence and recovery log, while Canonical is the current Materializer/Automation contract and the future Flink input.

**Tech Stack:** Markdown, Apache Kafka 4.3 concepts, PostgreSQL outbox, TDengine, EMQX/Gateway.

---

### Task 1: Freeze the selected topology and terminology

**Files:**
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md:1-345`

- [x] **Step 1:** Replace the architecture summary and main diagram with Kafka Raw and Durable Canonical topics.
- [x] **Step 2:** Remove Candidate A/B as production alternatives and state that the compact Durable Canonical topology is selected.
- [x] **Step 3:** Define the V1 physical cluster and the logical `iot.data.*`, `platform.saas.event.*`, and `platform.things.event.*` planes.
- [x] **Step 4:** Remove `protocol-materializer` as the selected deployment/checkpoint path while retaining in-process composition only as a non-production optimization boundary if still relevant.
- [x] **Step 5:** Re-read sections 1-5 and verify that every role, source of truth, checkpoint, and composition host matches the selected topology.

### Task 2: Rewrite Raw, Canonical, routing, and retention contracts

**Files:**
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md:346-620`

- [x] **Step 1:** Replace MQTT-to-NATS projection with deterministic MQTT-to-Kafka topic/key/header/value mapping.
- [x] **Step 2:** Define `rawRef=(clusterId, topicId/topicGenerationId, partition, offset)` and preserve the source identity across archive restore and replay.
- [x] **Step 3:** Define stable partition keys, consumer groups, manual offset commits, topic-generation expansion, and release/cutover offset vectors.
- [x] **Step 4:** Freeze Raw hot retention at 72 hours, `LogAppendTime`, capacity-by-hottest-partition, and optional encrypted 30/90-day object archive.
- [x] **Step 5:** Define Canonical current consumers: one Materializer checkpoint for history+latest, Automation for PropertyReport/DeviceEvent, reducers before state transitions, audit summaries, and future Flink.
- [x] **Step 6:** Define Canonical retention as required-consumer outage plus catch-up plus safety margin, with Raw replay as the regeneration fallback.

### Task 3: Rewrite reliability, replay, security, and operations

**Files:**
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md:621-1030`

- [x] **Step 1:** Replace JetStream ACK and pull-consumer semantics with Kafka append acknowledgement, consume-transform-produce transactions, consumer offsets, and external-sink idempotence.
- [x] **Step 2:** Replace ACK-floor fencing with per-partition continuous committed-offset vectors, group ownership, application fencing tokens, and topic generations.
- [x] **Step 3:** Update replay to use isolated consumer groups/topics/namespaces and retain source topic/partition/offset, headers, append time, and payload in archives.
- [x] **Step 4:** Add Kafka topic-prefix ACLs, separate principals/client IDs/group prefixes, replay quotas, and the limits of logical isolation.
- [x] **Step 5:** Add 70%/85% lag-age controls, hottest-partition retention headroom, archive completeness/checksum metrics, and Raw/Canonical/domain-plane observability.
- [x] **Step 6:** State the evidence-based triggers for splitting Data Kafka and Domain Kafka while keeping Raw and Canonical together.

### Task 4: Align phases, ADRs, acceptance criteria, and the directory README

**Files:**
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md:1031-1209`
- Modify: `亿级点位高吞吐架构/README.md:1-133`

- [x] **Step 1:** Replace JetStream-specific Phase 0 spikes with Kafka partition, batching/compression, group rebalance, transaction, retention, broker-failure, replay, and catch-up tests.
- [x] **Step 2:** Make Durable Canonical a Phase 0/Phase 1 commitment rather than an unresolved choice.
- [x] **Step 3:** Update Phase 1 delivery, enterprise acceptance criteria, and ADR priorities for the frozen topology.
- [x] **Step 4:** Update the directory README architecture diagram, sizing notes, FAQ, and checklist to match the main document.
- [x] **Step 5:** Preserve explicit historical/competitive references to NATS only where they describe legacy code or another project, not the selected platform topology.

### Task 5: Verify cross-document consistency

**Files:**
- Verify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
- Verify: `亿级点位高吞吐架构/README.md`

- [x] **Step 1:** Run `rg -n "候选 A|候选 B|protocol-materializer|JetStream|Nats-Msg-Id|AckFloor|NumAckPending|LimitsPolicy|MaxAge|MaxBytes|NATS Subject|MQTT/NATS"` on both files and classify every remaining hit as intentional or stale.
- [x] **Step 2:** Run `rg -n "72 小时|LogAppendTime|Durable Canonical|DataKafkaBootstrapServers|DomainKafkaBootstrapServers|retention|consumer group|partition|offset"` and confirm all frozen decisions are represented.
- [x] **Step 3:** Check Markdown links with a local link-target script and inspect headings/code fences for structural errors.
- [x] **Step 4:** Review `git diff --check` and the focused diff for accidental rewrites or loss of prior uncommitted content.
- [x] **Step 5:** Perform a final requirement-by-requirement review against the approved boundary and report any intentional residual NATS references.
