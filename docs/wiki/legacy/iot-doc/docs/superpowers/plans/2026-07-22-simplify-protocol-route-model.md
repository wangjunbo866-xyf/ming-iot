# Simplify Protocol Raw Route Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the V1 Raw routing ownership four-tuple with a low-cardinality `routeGroupId` subject contract while preserving immutable release provenance, safe rollout, replay, horizontal scaling, and tenant isolation.

**Architecture:** A RouteSnapshot resolves each new ingress message to one immutable route group and freezes the selected protocol/application/model/automation releases in `RawDeviceEnvelope`. JetStream dispatches by `raw.<routeGroupId>.<live|backfill>`; one ProtocolApplication deployment subscribes to one or a few group subjects, and its replicas share an `AckExplicit` durable consumer. `rawHashEpoch`, `rawBucket`, and `routeEpoch` are removed from the V1 contract; physical sharding is deferred until measurements justify a bounded `streamShardId`.

**Tech Stack:** Markdown architecture contracts, NATS JetStream subject filters and durable pull consumers, Kubernetes workloads, immutable release/snapshot metadata.

---

### Task 1: Rewrite the authoritative routing contract

**Files:**
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`

- [x] **Step 1: Replace the topology-level ownership tuple**

  Change the architecture diagram and Access Adapter/Gateway description to publish to `raw.<routeGroupId>.<live|backfill>`. State that subject tokens are low-cardinality dispatch metadata while tenant/product/device identity remains in the envelope.

- [x] **Step 2: Freeze the RouteSnapshot and envelope fields**

  Define `routeGroupId` as an immutable deployment-routing unit and retain `routeSnapshotId`, `protocolReleaseId`, and `protocolApplicationReleaseId` as durable provenance. Remove `rawHashEpoch`, `rawBucket`, and `routeEpoch` from `RawDeviceEnvelope`, `CanonicalDataBatch`, `IngressContext`, `EventEvidence`, cache entries, and `CommandAttempt`.

- [x] **Step 3: Define subject/body duplication and validation**

  Require the publisher and consumer to verify that the subject group, envelope `routeGroupId`, RouteSnapshot binding, and application release agree. Route mismatch must enter a durable security quarantine and must never fall back to the current route or release.

### Task 2: Rewrite rollout, ordering, replay, and scaling semantics

**Files:**
- Modify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`

- [x] **Step 1: Replace epoch/bucket cutover with route-group rollout**

  Create a new immutable route group for a new application release or canary lane, atomically publish new RouteSnapshots for selected scopes, route only new appends to the new group, and retain the old deployment until its consumer drains. Document optional ingress pause-and-drain for transitions that cannot overlap.

- [x] **Step 2: State the stateless decode requirement**

  V1 ProtocolApplication decode must be deterministic from Raw plus immutable artifacts and must not require cross-message device state or per-device delivery order. Parallel replicas may process one device out of order; history/latest/automation use their existing idempotence and total-order/frontier rules.

- [x] **Step 3: Define consumer and HPA ownership**

  Bind one `AckExplicit` durable consumer to each active route-group/mode pair, let all replicas of the bound application release share it, and keep Pod leases/generations out of message contracts. Preserve original replay through the old RouteSnapshot/artifacts and use isolated target RouteSnapshots for migrated replay.

- [x] **Step 4: Defer physical sharding explicitly**

  Remove V1 hash/bucket migration ADRs. Keep a Phase 0/ADR decision for stream placement, consumer naming, large-tenant isolation, and a small fixed `streamShardId` only if measured JetStream/consumer throughput requires it.

### Task 3: Synchronize the overview

**Files:**
- Modify: `亿级点位高吞吐架构/README.md`

- [x] **Step 1: Update the endpoint diagram**

  Show `RouteSnapshot -> raw.<routeGroupId>.<live|backfill> -> bound ProtocolApplication release`, and note that each workload has one or a few stable subject filters.

- [x] **Step 2: Add the concise routing decision**

  Explain shared versus dedicated route groups, subject/envelope responsibility, immutable group rollout/drain, and the deferred `streamShardId` trigger.

- [x] **Step 3: Update the Phase 0 risk item**

  Require validation of route-group isolation, subject/envelope mismatch quarantine, shared-consumer scaling, rollout/drain, replay, and rollback.

### Task 4: Verify consistency

**Files:**
- Verify: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
- Verify: `亿级点位高吞吐架构/README.md`

- [x] **Step 1: Scan for removed concepts**

  Run: `rg -n 'rawHashEpoch|rawBucket|routeEpoch|hash epoch|bucket 单活|旧四元组' 亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md 亿级点位高吞吐架构/README.md`

  Expected: no matches.

- [x] **Step 2: Scan for required concepts**

  Run: `rg -n 'routeGroupId|raw\.<routeGroupId>|streamShardId|protocolApplicationReleaseId' 亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md 亿级点位高吞吐架构/README.md`

  Expected: routing contract, rollout, envelope, scaling, security, Phase 0, ADR, and README all contain aligned references.

- [x] **Step 3: Review the diff**

  Run: `git diff --check && git diff -- 亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md 亿级点位高吞吐架构/README.md`

  Expected: no whitespace errors; no accidental changes outside the confirmed route-model scope.

This execution does not stage or commit files because the architecture documents already contain user-owned uncommitted work.

---

## 实施后偏差记录（2026-07-22 修补）

本计划按 `routeGroupId + raw.<routeGroupId>.<live|backfill>` 设计，最终落地为以下形态，语义目标不变（低基数 dispatch、不可变 provenance、安全灰度/回滚）：

1. **`routeGroupId` 由 `protocolKey` 取代**：路由 token 直接复用设备接入契约中的稳定逻辑协议通道，Subject 为 `raw.v1.<protocolKey>.<d|g>.<identity...>.<messageKind>`，不再单设 route group 维度；
2. **Subject 不再携带 `live|backfill`**：`processingMode` 改为 ProtocolApplication 对 `(messageKind, Raw store timestamp, decoded sample time, 不可变 processingModePolicyRelease)` 的纯函数判定，避免设备自报 mode，也避免按 mode 分裂 subject；
3. **Raw 不再定义业务 Envelope**：事实 = NATS Subject + 原 payload + 少量可选 headers + JetStream commit metadata；tenant/owner/release 不在 Adapter 写入，改由 ProtocolApplication 按可信 identity + cutover history 解析并写入 `ProcessingProvenance`；
4. **ACL 采用 client attributes 双占位符**：MQTT 与 NATS 的 identity 均保持 product/device 两级 token，投影是纯 level→token 映射。设备认证 username 固定为 `<productKey>.<deviceKey>`，EMQX `mqtt.client_attrs_init` 拆出两个属性，授权规则用 `${client_attrs.productKey}/${client_attrs.deviceKey}` 双占位符通配（见 `authz/mqtt-acl.md`）；file ACL 对 `${client_attrs.*}` 的支持与否列入 Phase 0 裁决，不支持则落 Built-in DB authorizer。曾考虑的备选（identity 合并为单个 MQTT level + `${username}` 单占位符）已被本方案取代；
5. **保留并强化**：单 production owner/durable consumer、多副本共享、shadow canary 隔离 namespace、ACK-floor fencing handoff、`streamShardId` 仅在实测证明瓶颈后引入——这些与本计划 Task 2 一致。
