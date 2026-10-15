# Scene Runtime 与 Kafka 物理契约联合修订设计

- 日期：2026-08-01
- 状态：设计已确认
- 修订范围：Scene Runtime 的 Kafka 契约与 affiliation matching 契约；Raw → Canonical Kafka transaction 及 protocol/model/ownership 自身的 cutover 保持不变

## 1. 问题

`4.1.Kafka/1.nats-to-kafka-greenfield-design.md` 把 Scene Runtime 定义为本地 RocksDB 热状态、Kafka compacted changelog、state/checkpoint/input offsets 同一 Kafka transaction，并要求 checkpoint、commit-attestation、snapshot-manifest 等 Topic。

`6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md` 对同一 Runtime 采用另一套 V1 契约：进程内 Actor 状态、异步 best-effort `scene_state`、无 Kafka EOS、无跨 Topic transaction、无 RocksDB 和远端 snapshot/manifest/CAS/GC。

两份文档的冲突处理规则也不一致。Kafka 文档要求冲突必须在同一次变更中显式修订；场景 README 却允许主规格单方面覆盖且暂不修改跨目录文档。当前状态无法给实现者提供唯一可执行契约。

父架构 `enterprise-iot-platform-architecture-design.md` 还要求 Scene 按 Canonical 中的 `affiliationBindingId` 读取不可变 `DeviceAffiliationSnapshot`，并以 `AffiliationCutover` offset barrier 固定迁区前后的归属。场景主规格则明确拒绝快照和 affiliation activation barrier，要求处理时解析 current `SubjectContext`，因此迁区后的 backlog/redelivery 按处理时已知的 current affiliation 匹配。两者对同一积压遥测给出相反结果。

## 2. 决策

在同一次变更中完成跨目录联合修订，并以场景主规格当前 V1 方案替换 Kafka 文档中的 Scene Runtime 专属设计。替换是架构契约变更，不描述成 Topic 改名或 wire-compatible 迁移。

Scene scope matching 统一采用处理时 current `SubjectContext`。`affiliationBindingId/DeviceAffiliationSnapshot` 不再作为 Scene candidate/matching 输入，affiliation 变化不再建立 `AffiliationCutover` Raw offset barrier。迁区后的 backlog/redelivery 按新 current affiliation 匹配是明确接受的 V1 语义，不描述为误匹配。

Kafka 文档继续规范 Raw、Canonical、领域事件、Kafka 客户端和非 Scene Runtime 的 transaction。Raw → Canonical 的 consume-transform-produce transaction 不受本次修订影响。protocol/model/ownership 因自身一致性需要保留的 release/owner cutover 也不因撤销 affiliation barrier 而自动删除。

## 3. 文档修改

### 3.1 Kafka 设计

1. 修正标题元信息、结论图、术语和方案比较中对 Scene Runtime 的 RocksDB/EOS 描述。
2. 修订 §1.2：保留“冲突必须在同一次变更中显式修订”的协议，并记录本次联合修订已经完成；删除要求场景文档向旧 §8 收敛的反向覆盖矩阵。
3. 重写 §5.3，列出当前 Scene V1 的三个物理 Topic：`scene_event`、`scene_state`、`scene_binding_lifecycle`，并引用主规格作为 key、partition、envelope、retention 和交付语义的唯一来源。
4. 将原 §8 的 RocksDB、transaction checkpoint、commit attestation、snapshot、barrier、state-generation GC 协议整体撤销，替换为当前 Scene Runtime 的 Kafka 集成边界摘要和对主规格 §8、§9、§13、§14 的规范性引用。
5. 清理 §5.5、§6、§10–§17 中依赖旧 Scene EOS 方案的 key、ACL、容量、恢复、风险、故障注入和实施门禁，避免已撤销协议继续以验收要求存在。

### 3.2 场景主规格与 README

1. 在主规格增加跨文档兼容说明和替换矩阵，明确当前 Scene V1 契约取代 Kafka 文档旧方案。
2. 将 README 的“以后同步、主规格单方面覆盖”改为双向一致性规则：发现跨目录冲突时必须在同一次变更中修订所有规范性文档，未完成前不得声称契约已冻结。

### 3.3 父架构

1. 从 ProtocolApplication 路由、Canonical envelope 和 Automation candidate 规则中删除 Scene 对 `affiliationBindingId/DeviceAffiliationSnapshot` 的依赖。
2. 将设备迁区/换组的 Scene 语义改为：SubjectLifecycle 更新 current `SubjectContext`，SOURCE_DELTA、延迟 Linked、恢复 reconciliation 和 backlog/redelivery 均按处理时 current affiliation 验证。
3. 从 affiliation 变更中撤销 `AffiliationCutover` Raw offset barrier、不可变 affiliation artifact 保留和相应 Phase 0/验收门禁。
4. 保留 Raw → Canonical transaction，以及 protocol/model/ownership 独立要求的 release/owner cutover；不得把 affiliation 修订扩大成全部 cutover 的撤销。

## 4. 旧契约处置矩阵

| Kafka 旧契约 | 当前 Scene V1 处置 |
| --- | --- |
| `automation.input.live` | 由 `scene_event` 取代；不是简单改名，key、envelope、事件分类和处理语义均以主规格为准 |
| `automation.state` | 由异步 best-effort `scene_state` 取代；不与 input offset 原子提交 |
| `automation.checkpoint` | 废弃；V1 不维护事务 checkpoint Topic |
| `automation.commit-attestation` | 废弃；动作可靠性由 PG Outbox、稳定 `effectId` 和 dispatcher 幂等收敛 |
| `automation.snapshot-manifest` | 废弃；V1 不使用 RocksDB 或远端 snapshot/manifest/CAS/GC |
| `automation.state-migration-progress` | 废弃；V1 固定 partition count，扩容必须另立迁移设计并重新评审 |
| 独立 `automation.schedule` lane | 废弃；Schedule 进入统一 `scene_event` |
| release artifact/active-release data-plane barrier | 不属于当前 Scene V1 物理契约；配置由 PG current Binding 与 `scene_binding_lifecycle` 最终收敛 |
| `affiliationBindingId` / `DeviceAffiliationSnapshot` | 不再作为 Scene scope candidate/matching 输入；Scene 在处理时解析 current `SubjectContext` |
| `AffiliationCutover` offset barrier | 对 Scene affiliation matching 废弃；迁区/换组通过 SubjectLifecycle 与 current-context reconciliation 最终收敛 |
| 迁区前积压遥测按旧归属匹配 | 废弃；backlog/redelivery 按处理时已知的 current affiliation 匹配 |

## 5. 一致性边界

- `scene_state` 是 compacted full-state 恢复来源，但其发送是异步 best-effort；它不是 Kafka EOS changelog。
- Scene consumer offset 可以在 `scene_state` callback 失败窗口内推进，恢复语义、允许的状态回退和告警以主规格 §9、§13 为准。
- Scene 动作先在 PG 本地事务中写 Outbox，再发布 Actor 内存状态并异步发送 `scene_state`；Kafka transaction 不覆盖 PG。
- `scene_event` 与 `scene_state` 的固定 partition/key 约束继续成立，但不能据此推导跨 Topic 原子提交。
- 任何保留的 Kafka transaction 示例必须明确属于 Raw → Canonical 或其他非 Scene Runtime 流程。
- Canonical 不再携带供 Scene scope matching 使用的不可变 affiliation snapshot ID；Scene 不能从旧 payload 恢复历史 area/group membership。
- protocol/model/ownership cutover 与 affiliation matching 是不同边界；本次只撤销后者的 snapshot/offset barrier。

## 6. 验证

修订完成后执行以下检查：

1. 搜索 Kafka 文档中的 `RocksDB`、`checkpoint`、`commit-attestation`、`snapshot-manifest`、`transactional.id`、`sendOffsetsToTransaction`，逐处确认没有残留的 Scene V1 实施要求。
2. 搜索 `automation.input.live`、`automation.state`、`automation.schedule`，只允许出现在明确标为旧契约/处置矩阵的上下文中。
3. 核对 Kafka §5.3、§8、场景主规格和 README 对 Topic、状态后端、恢复协议、offset 提交与冲突解决规则的表述一致。
4. 核对 Raw → Canonical transaction 的结论图、§7.3、客户端配置和故障测试未被误删或弱化。
5. 搜索父架构中的 `affiliationBindingId`、`DeviceAffiliationSnapshot` 和 `AffiliationCutover`，确认没有残留的 Scene scope matching、backlog 或验收要求；同时确认 protocol/model/ownership cutover 未被误删。
6. 核对父架构与场景主规格对迁区后 backlog/redelivery 的结果一致，均为处理时 current affiliation。
7. 检查所有新增相对链接有效，并审阅 diff，确保没有修改无关文档。

## 7. 完成标准

实现者从 Kafka 文档或父架构进入 Scene Runtime 章节时，只能得到场景主规格定义的 V1 物理与 affiliation 契约；不能再据任一规范性段落实现 RocksDB/EOS/checkpoint/attestation/snapshot 方案，也不能让迁区后的 backlog 按不可变旧 affiliation 匹配。同时，Raw → Canonical 的 Kafka transaction 仍保持原有规范强度。
