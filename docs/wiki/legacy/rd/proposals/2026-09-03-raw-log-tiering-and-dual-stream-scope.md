# Raw Log 分层与双流范围提案

> 日期：2026-09-03
>
> Goal：`ming-iot-framework-v1`
>
> 状态：候选方案比较，**非权威**；待用户选择方案后另立 RFC/规格。本提案不创建实现 Todo，不构成代码修改授权
>
> 推荐执行配置：决策与规格使用 `gpt-5.6-sol`、`reasoning=high`；独立架构 Review 可使用 `xhigh`

## 1. 结论

建议将「设备原始报文双流（Raw + Canonical）」拆成两个能力等级，并在 Phase 1 只交付其中较轻的一级：

- **Tier A · 取证级**：Raw 原帧旁路写入对象存储，携带 `RawRef`、checksum 与 offset-range manifest，保留 30/90 天，**不进 Kafka 事务、不参与 offset、不要求 RF=3**；
- **Tier B · 重算级**：当前主架构冻结的完整形态——Raw 72h（RF=3）+ `RESTORE_ORIGINAL` / `REPROCESS_MIGRATED` + `RawOffsetBarrier` + shadow group + 归档导入重放工具链。

Phase 1 生产数据面**继续保留 Raw**，但只承担运行时职责（durable handoff、削峰、故障隔离、恢复），保留期从 72h 下调到 12~24h 档位；「重解释/全量重算」职责交给 Tier A，按需人工触发。Tier B 在升配触发条件命中后再立项。

**必须同时说清的一点**：本提案省下的是「长期重解释链路」的重资产（归档契约、replay tooling、存储与持续运维），**不省** Raw→Canonical 事务、`RawOffsetBarrier` 与 fencing——后者是并发正确性，与保留期无关。方案间的实际差异小于直觉。

## 2. 已确认事实

### 2.1 本仓库与产品代码

- 主架构基线：`iot-doc/亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
  - §4.2：Raw 承担「协议/物模型修复后的原版本重放与迁移重算」；
  - §12.3：「Raw Kafka hot retention 生产基线为 72 小时」，30/90 天对象归档按合同启用。
- **`ming-iot/backend/share/contracts/kafka/catalog.go` 已将 72h 固化为代码契约**：

  ```139:141:ming-iot/backend/share/contracts/kafka/catalog.go
  	if spec.FactKind == FactRaw && spec.Retention < 72*time.Hour {
  		return Topic{}, fmt.Errorf("raw retention must be at least 72h")
  	}
  ```

- `ming-iot-rd-docs/specs/2026-08-28-kafka-foundation-contract-sdk-activation-design.md` §5.1 要求 Catalog 校验「Raw retention 小于 72 小时」必须拒绝；§10 明确不实现 Raw→Canonical transaction。
- 该 Loop 的契约包已落地：`backend/share/contracts/kafka/` 存在 `catalog.go`、`names.go`、`raw_record.go`、`raw_ref.go` 及对应测试。
- `RawRef` 稳定来源身份已冻结为 `sourceClusterId/topicId/partition/offset`，可直接被 Tier A 复用。

### 2.2 行业对照（用于界定「双流是否通用」）

- **ThingsBoard**：解码发生在持久化边界**之前**，进 Kafka 的是已归一化的 `TbMsg`。源码亲验：

  ```1144:1152:thingsboard/common/transport/transport-api/src/main/java/org/thingsboard/server/common/transport/service/DefaultTransportService.java
          TbMsg tbMsg = TbMsg.newMsg()
                  .queueName(queueName)
                  .type(tbMsgType)
                  .originator(deviceId)
                  .customerId(customerId)
                  .copyMetaData(metaData)
                  .data(gson.toJson(json))
                  .ruleChainId(ruleChainId)
                  .build();
  ```

  `originator` / `customerId` 在入队前已解析，`data` 为 `gson.toJson(json)` 字符串，**原始 wire bytes 不进入持久队列**。
- **JetLinks Community**：`iot-doc` §16.2 调研结论为 EventBus 未形成可验证的持久化、offset、ACK 与 replay 语义。
- **Azure IoT Hub**：内置 Event Hubs 兼容端点保留最长 7 天、创建时设定（[Microsoft Learn](https://learn.microsoft.com/en-us/azure/iot-hub/iot-hub-devguide-messages-read-builtin)，已核实）。
- **AWS IoT Core**：无内置持久存储，是否持久化由用户选择的规则动作决定（一般认知，本提案未逐条核实官方文档）。

**判断**：双流不是行业通用形态，而是「解码逻辑会变 + 设备不能重传」两个条件同时成立时的特化解。这两个条件在本项目场景成立，因此**不推翻双流方向**，只讨论范围与投资时机。

## 3. Objective

在 Raw→Canonical transaction、Access Adapter 与 ProtocolApplication 等生产数据面 Loop 启动之前，确定 Raw 流的职责边界与保留档位，避免：

1. 为一个**年级低频**能力（全量重解释）预先投资生产级重资产；
2. 或因过度裁剪而丢失本项目最本质的架构优势——**解码发生在持久化边界之后**。

## 4. Dependencies

1. Kafka 基础契约与 SDK Loop（`specs/2026-08-28-…`）已完成或已明确其稳定契约范围；
2. `ming-iot` 本地 Share 迁移完成并回归通过；
3. dmsvr Proto 领域拆分完成；
4. 用户在本提案中选定方案 A / B / C 之一；
5. 若选定 B，需另行授权对 §7 所列已批准契约的最小变更；
6. `iot-doc` 属独立权威仓，本提案不修改它；若采纳 B，需在 `iot-doc` 侧单独立项同步 §4.2 / §12.3，**同步完成前 iot-doc 仍为权威**。

## 5. 备选方案比较

| 维度 | A. 维持完整双流 | B. Tier 化（推荐） | C. 单流 Canonical |
| --- | --- | --- | --- |
| Raw 保留期 | 72h（RF=3） | 12~24h 档位（RF=3） | 无独立 Raw |
| 长期原始帧 | 30/90 天 Kafka 级归档（受控契约） | 30/90 天旁路对象存储（best-effort） | 无 |
| 解码位置 | 持久化边界之后 | 持久化边界之后 | **持久化边界之前** |
| 自动全量重算 | ✅ 受控 replay 两种模式 | ❌ 人工脚本，慢 | ❌ 不可能 |
| 取证能力 | 72h 内 | 30/90 天，**更强** | ❌ |
| Raw→Canonical 事务 | 需要 | 需要 | 不适用 |
| `RawOffsetBarrier`/fencing | 需要 | 需要 | 不适用 |
| 归档契约 + replay tooling | 人月级 | 轻（dump + manifest） | 无 |
| 与现状契约冲突 | 无 | 需改 72h 硬编码 | 需推翻主架构 §4.2 |
| 事后补建代价 | — | 中（数据已在 Tier A） | **高**（须重写接入热路径，切换点前历史不可补） |

**方案 C 明确拒绝**，理由不是"多数平台这么做"，而是：把解码移到持久化边界之前属于**实质不可逆**决策——事后想补 Raw 必须重写接入热路径，且切换点之前的历史永远无法恢复。这与本项目「厂商私有二进制协议 + 现场反复调试 + 设备夜间断电不可重传」的前提直接冲突。

**方案 A 与 B 的差异集中在三点**：Raw 保留期档位、长期重算的自动化程度、以及重资产投资的时机。二者在生产数据面的并发正确性要求上**完全相同**。

## 6. 推荐方案 B 的具体形态

### 6.1 生产数据面（不变）

```text
Device/网关 → Adapter → Kafka Raw（12~24h 档位，RF=3, acks=all, LogAppendTime）
   → ProtocolApplication（事务：Canonical outputs + quarantine + Raw offsets）
   → Kafka Canonical（≥24h）→ Materializer / Automation / Reducer / Audit
```

Raw 继续提供：durable handoff、削峰、故障隔离、consumer 崩溃恢复、解码在持久化边界之后。**这些能力不依赖 72h。**

### 6.2 Tier A · 取证级归档（新增，轻量）

- Adapter 在 append Raw 成功的同时，**异步旁路**投递一份原帧到对象存储；
- 每条记录至少携带：`RawRef`（已有契约）、`LogAppendTime`、key、headers、payload、checksum；
- 按 `topic/partition` 生成**连续 offset-range manifest**，用于完整性校验；
- 不在 Kafka 事务内、不参与 offset 提交、不要求 RF=3；
- 旁路失败不得阻塞主链路，只允许记指标与告警；
- 保留期 30/90 天由合同决定，可整体启停。

### 6.3 Tier B 升配触发条件（可判定）

满足任一即启动 Tier B 立项：

1. 12 个月内发生第 2 次「需要对 >24h 窗口的历史数据执行全量重解释」的真实事件（协议/物模型发布缺陷，或客户争议）；
2. 客户合同明确要求「平台可在 X 小时内自动重算最近 Y 天遥测」，且 `Y > 24h`；
3. shadow group 对新解码器做上线前对照成为常态流程（例如每季度 ≥2 次协议变更）。

### 6.4 容量估算（按 100 万设备 × 500 点 × 4KB/report 锚点，待 Phase 0 实测校准）

| 项 | 数值 |
| --- | --- |
| 单副本日增 | ≈ 0.58 TB/天 |
| Raw RF=3 @ 72h | ≈ 5.2 TB |
| Raw RF=3 @ 24h | ≈ 1.7 TB |
| Raw RF=3 @ 12h | ≈ 0.86 TB |
| Canonical @ 24h（按 1.5× Raw 估） | ≈ 2~3 TB |
| Tier A 旁路归档（单副本） | 30 天 ≈ 17 TB；90 天 ≈ 52 TB |
| **60s 集中峰值突发增量** | ≈ 12 TB（200 MB/s × 60s），**与保留档位无关，须单独预留** |

**结论**：存储不是主要成本。方案 B 相对 A 的存储节省约 3.5 TB 三副本 SSD，真正的收益是**推迟归档契约与 replay tooling 的人月级投入**。

## 7. 与已批准决策的冲突及最小变更

若采纳方案 B，至少需要：

| # | 冲突点 | 最小变更建议 |
| --- | --- | --- |
| 1 | `backend/share/contracts/kafka/catalog.go:139-141` 硬编码 `72h` | 改为「允许档位集合」校验（如 `{12h, 24h, 72h}`），档位由 deployment profile + ADR 决定；local profile 允许更短；**保留**「Raw 必须有显式 retention、禁止依赖 broker 默认」的约束 |
| 2 | `specs/2026-08-28-kafka-foundation-contract-sdk-activation-design.md` §5.1 | 同步 §5.1 校验项措辞，不改其余范围 |
| 3 | `catalog_test.go` 中 72h 相关断言 | 按新档位表补齐用例，保留负向用例 |
| 4 | 主架构 §12.3 的 72h 基线 | 不修改 `iot-doc`；另立 ADR 记录 Phase 1 档位与 Tier B 触发条件，并在 `iot-doc` 侧单独立项同步 |

**不因本提案改动**：`RawRecord` / `RawRef` 契约、producer 耐久档位（`acks=all` / RF=3 / minISR=2）、`LogAppendTime`、禁止动态 Topic 与 wildcard 的规则。

## 8. 强制约束

- 解码仍必须在持久化边界**之后**——本提案在任何方案下都不改变这一点；
- Raw 仍必须有显式 retention，不得依赖 broker 默认值；
- Tier A 旁路失败只能降级为「无归档」，不得阻塞或失败主链路；
- Tier A 不得引入第二个「Raw 事实来源」——它只是副本，`RawRef` 仍以 Kafka Raw 为准；
- 不得为实现 Tier A 新增通用 `Publish/Subscribe` 或 wildcard API；
- 不在本提案范围内启动 ProtocolApplication、Access Adapter、Materializer 或 Scene Runtime。

## 9. Acceptance

1. 用户在本提案中明确选定 A / B / C；
2. 若选定 B：`catalog.go` 的 72h 硬编码按 §7 变更，且 `FactRaw` 显式 retention 约束仍生效；
3. Tier A 归档记录可通过 `RawRef` 反查到对应 Kafka Raw 位置，checksum 与 offset-range manifest 完整可校验；
4. 旁路归档停止或不可用时，主链路 produce/consume 不受影响，只产生指标与告警；
5. `kafkacontract` 校验器对档位集合、profile 混用和负向用例全部通过；
6. 新增一条 ADR 记录档位选择、Tier B 触发条件与残余风险；
7. Evidence 记录档位、容量估算来源、旁路投递失败注入测试结果与回归结果。

## 10. Non-goals

- 不修改 `iot-doc` 中的任何文件；
- 不实现 Raw→Canonical transaction、`RawOffsetBarrier`、shadow group 或 `RESTORE_ORIGINAL` / `REPROCESS_MIGRATED`；
- 不实现 Tier B 的自动重算与归档导入工具；
- 不改变 Canonical 保留期基线（仍由 consumer 最长中断 + 追赶时间决定）；
- 不引入第二个消息队列运行时；
- 不修改现有 NATS 依赖或业务消息链路；
- 不创建实现 Todo，不执行 commit / merge / push。

## 11. 主要风险

| 风险 | 说明与控制 |
| --- | --- |
| **省得比预期少** | 事务、barrier、fencing 与保留期无关，方案 B 不省这部分。必须在决策时明确认知，避免以"能大幅简化"为由采纳 |
| **72h 已固化为代码契约** | 已落地且有测试；变更须走 §7 的完整清单，不能只改常量 |
| **Tier A 与 Kafka Raw 不一致** | 旁路是异步 best-effort，可能缺失个别记录。manifest 必须能表达 gap，且不得把它当作可重放的权威来源 |
| **突发余量被忽略** | 60s 集中峰值瞬时增量 ≈ 12 TB，与 retention 无关；磁盘必须按「稳态基线 + 最大突发增量」预留，否则时间保留 + 不可删除策略会在洪峰时撞上磁盘上限 |
| **事后补建 Tier B 更贵** | 若重解释需求实际高频，方案 A 一次建好更划算。§6.3 的触发条件就是为限制这一风险敞口 |
| **取证能力被误当成重算能力** | Tier A 能回答"设备当时发了什么"，不能自动回答"按新模型应该怎么算"。对客承诺时必须区分 |
| **Phase 0 压测未完成** | 所有容量数字均为锚点估算，不得作为对客承诺依据 |

## 12. 激活条件

只有同时满足以下条件，Planner 才能创建本提案后继工作的 Todo：

1. 用户选定方案 A / B / C，并明确 Raw 保留档位；
2. 若选定 B：用户批准 §7 的契约变更清单与写范围；
3. Kafka 基础契约与 SDK Loop、本地 Share 迁移、dmsvr Proto 拆分均已完成并回归通过；
4. 新增 ADR 记录档位选择与 Tier B 升配触发条件；
5. 若需同步 `iot-doc` §4.2 / §12.3，已在 `iot-doc` 侧单独立项，且在同步完成前不宣称本提案推翻主架构；
6. 实施 Todo 使用 `gpt-5.6-sol/high`，独立架构 Review 可使用 `xhigh`；
7. commit、merge、push 继续分别等待用户明确授权。
