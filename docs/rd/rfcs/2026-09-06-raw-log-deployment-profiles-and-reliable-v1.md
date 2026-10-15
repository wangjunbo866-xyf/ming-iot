---
document_kind: rfc
status: accepted
delivery_issue: ming-iot#IKDK3Q
code_repository: git:gitee.com/ming-industry/ming-iot
related_documents:
  - proposals/2026-09-05-raw-log-feasibility-reassessment.md
  - references/raw-log/2026-09-05-industry-practices.md
  - references/raw-log/2026-09-05-production-case-comparison.md
  - references/raw-log/2026-09-05-industrial-case-evidence.md
---

# Raw Log 部署档位与 Reliable V1

## 状态与决策边界

本 RFC 已于 2026-09-06 由 Owner 接受。它冻结 Raw Log 的长期部署档位及共同语义，但不授权编码，也不表示任何档位已经实现。

本 RFC 的接受意味着：

- 长期产品方向包含 Diagnostic、Reliable、Traceable 三个部署档位；
- 当前只允许为 **Reliable V1** 编写后续实施 Spec 与 Plan；
- Diagnostic 和 Traceable 只保留目标语义，不进入本轮实现、配置或对外承诺；
- RFC 接受后仍需独立的 Spec、Plan、实现、测试、Review 与 PR 才能交付。

关联交付事项：`ming-iot#IKDK3Q`。

## 1. 背景

ming-iot 面向工业设备和多厂家私有协议。设备上报的 Raw 消息可能是二进制载荷或厂家定制格式；ProtocolApplication 负责将它解释为平台标准化的 Canonical 消息。

只保留 Canonical 的架构更简单，但当 ProtocolApplication 停机、消费不及时或解码逻辑存在缺陷时，解码前输入可能无法恢复。EMQX 对 QoS 0 消息本身也不提供持久保存承诺，因此不能把 Broker 接收等同于平台已经获得可恢复的 Raw 记录。

另一方面，对所有部署都建设长期 Raw 归档、历史版本回放和人工处置平台，会显著增加存储、权限、运维与测试成本。行业案例支持按用途分层保留输入，但不足以证明所有工业平台都应默认保存全部原始协议载荷。研究证据及其限制记录在本 RFC 的关联文档中。

因此，本 RFC 要解决的不是“Raw 要不要永远保存”，而是：

1. 用清晰部署档位匹配不同可靠性与追溯需求；
2. 统一 Raw、Canonical、Quarantine 和可靠接管边界；
3. 在不提前建设完整归档平台的前提下，先交付可验证的 Reliable V1。

## 2. 术语

### 2.1 Raw Record

Raw Record 是平台接纳、尚未被业务解释的应用层输入及其必要上下文，包括 payload、消息键、headers、来源 topic 身份、partition、offset 和 Kafka 记录时间等。

Raw Record 不是网络抓包，不承诺包含 TLS、MQTT 或厂商链路上的全部线上字节，也不覆盖平台未接纳的消息。

框架不得假设一个 Raw Record 内部有固定数量的设备点位或业务记录。payload 的内部边界属于对应 Codec。

### 2.2 Canonical Record

Canonical Record 是 ProtocolApplication 按指定协议版本和物模型版本解释后产生的平台标准消息，供规则、存储、告警等下游消费者使用。

三种部署档位都必须将 Canonical 写入 Kafka。Raw 与 Canonical 默认使用同一个 Data Kafka 集群中的不同 topics，而不是两套 Kafka 集群。

### 2.3 Quarantine Record

Quarantine Record 是对确定性坏输入的不可变失败事实。它用于保留失败分类、必要诊断信息和可用的 Raw 引用，不是第二份 Raw 存储，也不是可修改的工单记录。

可选的 PostgreSQL 处置表属于未来的人工作业投影，可维护状态、处理人和关闭时间；它不取代 Kafka Quarantine 事实。

### 2.4 NO_OUTPUT

NO_OUTPUT 表示 Codec 成功处理 Raw，但按协议语义没有产生 Canonical 或 Quarantine 输出。它是明确结果，不是静默丢失。

## 3. 决策

### 3.1 冻结三个部署档位

#### Diagnostic

```text
EMQX → ProtocolApplication → Canonical Kafka
```

- ProtocolApplication 在解释 Raw 前，以稳定采样方式将有限 Raw 诊断信息写入 SLS 或同类日志系统；
- 对确定性失败保存有界诊断信息并写入 Quarantine；
- SLS 写入是 best-effort，不构成可重放存储，也不承诺消息恢复；
- 适用于对数据完整性要求较低、优先降低基础设施与运维成本的部署。

Diagnostic 不在本轮实现范围。

#### Reliable

```text
EMQX → Access Adapter → Raw Kafka
                         ↓
              ProtocolApplication → Canonical Kafka
                         └────────→ Quarantine Kafka
```

- Raw Kafka 是 ProtocolApplication 之前的可靠缓冲与短期恢复边界；
- Raw 默认保留 72 小时；
- ProtocolApplication 对一个 Raw Record 的 Canonical、Quarantine 或 NO_OUTPUT 结果与 Raw consumer offset 原子提交；
- 下游消费者使用 `read_committed`，不得读取未提交的事务输出；
- 适用于要求削峰、服务故障恢复和有限窗口重放，但不要求长期保留原始历史的部署。

Reliable V1 是本轮唯一允许进入后续 Spec 与 Plan 的档位。

#### Traceable

```text
Reliable
  └── Raw archive consumer → S3/OSS 分块对象 + Manifest/Catalog
```

- 在 Reliable 基础上，由独立归档消费者异步将 Raw 写入对象存储；
- 归档使用可校验、可发现、可重放的分块格式与 Manifest/Catalog；
- 长期保留期由客户合同决定；
- 适用于需要长期原始输入取证、协议纠错后历史重算或合规留存的部署。

Traceable 不在本轮实现范围。

### 3.2 部署档位是部署级选择

- 每个部署环境选择一个档位；
- 不按租户、产品或单条消息动态切换；
- 不支持不停机热切换；
- 默认生产档位为 Reliable；
- 改变档位必须经过容量评估、配置变更、验证与发布流程。

此约束避免在同一运行时引入难以解释的可靠性分支、权限差异和成本归属。

### 3.3 Reliable 的可靠性边界

Reliable 档位中，“平台已获得可恢复 Raw”的边界从 Raw Record 成功提交到 Kafka 开始。

该边界不自动涵盖：

- 设备到 EMQX 之前的链路；
- EMQX 已接收但 Access Adapter 尚未提交到 Kafka 的窗口；
- 设备自身未重发或 QoS 0 已丢失的消息；
- 超出 Raw retention 且没有 Traceable 归档的历史输入。

因此，后续产品和协议文档不得把 MQTT PUBACK、进程内接收或日志打印描述成 Kafka Raw 已持久化。

### 3.4 Reliable 的事务结果

ProtocolApplication 每次处理独立 Raw Record 时，只能形成以下一种事务结果：

1. 一个或多个 Canonical Record；
2. 一个或多个 Quarantine Record；
3. NO_OUTPUT。

结果记录与该 Raw 的 consumer offset 在同一 Kafka 事务中提交。进程在提交前退出时，事务输出对 `read_committed` 消费者不可见，Raw 会在恢复后重新处理；提交成功后不得因 offset 丢失而再次产生可见重复输出。

Diagnostic 没有 Raw Kafka consumer offset，不适用上述事务边界，也不能宣称具备同等恢复语义。

### 3.5 Quarantine 的判定与粒度

只有可以确定为输入或 Codec 语义错误、且相同输入在相同版本下重试不会恢复的问题，才进入 Quarantine。依赖不可用、超时、Kafka 故障等瞬时错误必须中止事务并重试，不得伪装成坏数据。

失败粒度由 Codec 能否稳定识别业务输出边界决定：

- 无法解析整体结构或无法确定内部边界时，生成 raw-level Quarantine；
- Codec 能稳定识别独立输出时，可为失败输出生成 per-output Quarantine，同时保留其它有效输出；
- 框架不规定一个 Raw 中有多少输出，也不要求“一处失败则放弃所有可识别输出”；
- 一个确定性坏 Raw 不得阻塞同一 poll 中其他彼此独立的 Raw Records。

不同档位保存的证据不同：

- Diagnostic：只保存有界失败证据，不承诺可重放；
- Reliable：保存 `raw_ref`，在 Raw retention 内解析原始输入；
- Traceable：初始 Quarantine 仍只保存 `raw_ref`，后续由 Manifest/Catalog 将该引用解析到归档对象；Quarantine 创建时不等待、也不强制包含 `archive_ref`。

### 3.6 受控重放

Reliable V1 支持受控、隔离的短窗口重放，但不建设自助回放门户。

重放必须显式指定：

- 输入范围；
- ProtocolRelease；
- ModelRelease；
- 输出隔离位置；
- 执行者、原因和验证方式。

不得使用“最新协议”或“最新模型”作为隐式默认值。重放输出不得直接触发设备命令、生产告警动作或覆盖在线结果；验证完成后的数据修复属于后续独立流程。

Traceable 的未来归档格式至少保留原 topic 身份、partition、offset、Kafka 记录时间、key、headers、payload 和校验值。建议以 partition 连续范围形成 Avro OCF 等分块对象，并由 Manifest/Catalog 描述覆盖范围和完整性；具体格式需在 Traceable 激活时另行设计。

## 4. Reliable V1 的运行契约

### 4.1 Kafka 基线

生产环境的 Raw topic 默认：

- retention：72 小时；
- replication factor：3；
- `min.insync.replicas`：2；
- producer：`acks=all`；
- 记录时间：`LogAppendTime`。

这些是架构下限，不是未经容量验证即可直接复制的完整部署参数。分区数、segment、压缩、磁盘水位和 `retention.bytes` 必须依据真实负载确定；过小的大小上限不得使 72 小时时间契约失效。

### 4.2 必须可观测的指标

至少监控：

- Raw consumer lag；
- 最老未处理 Raw 的年龄；
- retention headroom；
- Kafka 事务失败及中止次数；
- Quarantine 数量和比例。

只看消息条数不足以判断能否在 retention 到期前恢复。容量评估还必须覆盖真实 payload 大小、压缩率、峰值输入和消费者追赶能力。

## 5. Reliable V1 验收条件

后续实施 Spec 不得弱化以下验收场景：

1. 正常 Raw 能产生预期 Canonical；
2. Codec 可分别表达 raw-level Quarantine、per-output Quarantine 和 NO_OUTPUT；
3. 一个确定性坏 Raw 不阻塞同批其他独立 Raw；
4. 瞬时依赖故障不会写入 Quarantine，而是中止事务并可重试；
5. Raw 已提交后强制终止 ProtocolApplication，恢复后能够继续处理；
6. 事务中途失败时，`read_committed` 消费者看不到部分 Canonical 或 Quarantine；
7. 对至少 48 小时前的样本，既能用原 ProtocolRelease/ModelRelease 重放并获得相同 Canonical hash，也能用显式选择的新版本产生隔离结果；
8. 使用代表真实部署的 payload 分布、压缩率、峰值与追赶负载，证明 72 小时容量和恢复余量成立。

测试通过只证明选定环境与数据集满足基线，不能代替生产容量规划。

## 6. Reliable V1 非目标

本轮明确不包含：

- S3/OSS 长期 Raw 归档；
- Diagnostic 档位的实现或部署配置；
- Traceable 档位的实现或部署配置；
- PostgreSQL Quarantine 人工处置工作流；
- 自助式重放 UI 或通用历史重算平台；
- 按租户或产品动态选择档位；
- 自定义 Gateway ACK 与 Kafka commit 的端到端协议。

## 7. 代价与后果

### 正面后果

- ProtocolApplication 可以独立停机、扩缩容和追赶，Raw Kafka 提供削峰；
- 解码前输入在 72 小时窗口内可恢复，协议修复可以受控验证；
- 三个档位共享 Raw/Canonical 术语和失败语义，未来扩展不会重新定义边界；
- V1 不承担长期归档和通用回放平台的全部复杂度。

### 负面后果

- Reliable 相比 Diagnostic 增加 Raw topic、存储容量、Kafka 事务和监控成本；
- 72 小时是需要实测证明的运营承诺，不是仅写配置即可成立；
- Kafka 事务只覆盖 Kafka 内部结果，不能原子延伸到 PostgreSQL、SLS 或对象存储；
- Traceable 未来仍需处理归档完整性、权限、加密、对象布局和恢复演练。

## 8. 被否决或延期的方案

### 8.1 所有部署只保留 Canonical

实现最简单，但不能满足工业私有协议场景中解码服务故障恢复和协议纠错后的短期重放，因此不作为默认生产架构。

### 8.2 所有部署立即启用长期 Raw 归档

取证能力最强，但当前缺少客户合同、真实容量和运维成本证据。它被保留为 Traceable 目标，而不是 Reliable V1 的强制依赖。

### 8.3 ProtocolApplication 本地持久化 Raw

会把副本、清理、故障转移、扩缩容和一致性责任放回应用节点，通常比复用 Kafka 更复杂，因此不采用。

### 8.4 EMQX 同时异步写 Kafka 与对象存储

两个目标不具备天然原子性，成功状态和补偿边界难以证明。未来 Traceable 应从已提交的 Raw Kafka 异步归档，并通过幂等对象和 Manifest/Catalog 收敛。

## 9. 分阶段采用

1. 本 RFC 已由 Owner 接受；
2. 只为 Reliable V1 创建实施 Spec 与 Plan；
3. 完成契约、实现、故障测试、容量验证、独立 Review 和 PR；
4. 只有出现明确客户合同或长期取证需求时，另建 Traceable RFC/Spec 增量；
5. 只有出现明确低成本部署需求时，另建 Diagnostic 激活 Spec。

任何阶段都不得用“目标档位已定义”替代“该档位已实现并验证”。
