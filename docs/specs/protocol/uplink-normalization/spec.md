---
status: draft
version: 0.1.0-draft
---

# 设备上行解码与标准化闭环：候选规格

Feature：`protocol.uplink-normalization`；能力域：[protocol](../README.md)；[责任、认领与接手入口](README.md)。

本文件集中维护本 Feature 的当前设计，尚未实施或生效。它吸收 [Kafka Patch 第 11 节历史评估](../../platform/eventbus/patches/PATCH-20261017-kafka-migration/change.md#protocol-application-assessment)中与上行有关的设计方向；原始决定继续保留在该 Patch，未来当前规则在此演进。文档组织授权及双向依赖见 [R13](../../platform/eventbus/patches/PATCH-20261017-kafka-migration/change.md#protocol-domain-extraction)。不将 Kafka Patch 标为 absorbed：Eventbus 迁移仍在进行。

## 来源、当前事实与确认边界

| 来源 | 对本规格的约束 |
| --- | --- |
| Kafka Patch D13 / RAW-013 | 首版先上行解码、标准化与失败处置；命令回执须识别和分流，下行编码后续推进 |
| Kafka Patch D14 / RAW-013 | 客户适配器由团队或受控合作方发布；不允许客户直接上传任意代码运行，不代表本轮授权实际发布 |
| Kafka Patch D15 / RAW-014 | 先解码后持久化标准消息，暂不承诺原报文补算；首版不强制引入 Raw Kafka |
| Kafka Patch D6、D7、D8、D12 | 继承已接受消息的可靠补处理、操作结果未知默认不自动重发、保留旧现场不自动导入、遥测积压时控制与重要补处理独立推进的目标 |
| RAW-015 / D16 | 按协议适配能力域建立本候选入口，验证并独立复审；仅文档工作，不实现或推送 |

上述已确认边界继续适用；以下职责划分、具体契约和验收设计为技术候选。D16 不是对全部技术方案的批准。首版没有重算原报文的承诺，也没有现成的服务部署或运行验收证据。

现状核对基线为 `10a1843684e4bb12dd78f0caebbc8231530f74c9`：

- [DG getDevPublish](../../../../backend/things/service/dgsvr/internal/event/deviceSub/deviceSub.go)已有产品脚本解码，[scriptRepo](../../../../backend/things/service/dgsvr/internal/repo/cache/scriptRepo.go)缓存脚本执行环境；[SDK Core](../../../../backend/things/sdk/protocol/coreProtocol.go)主要处理已结构化的 DevPublish。新设计应承接这些职责，不能重复执行两遍解码。
- [DM 脚本处理](../../../../backend/things/service/dmsvr/internal/domain/protocol/script.go)及[属性链](../../../../backend/things/service/dmsvr/internal/event/deviceMsgEvent/thing.go)已有模型转换、业务校验和副作用；不是可直接整体搬进 Codec 的纯转换。
- [DevPublish](../../../../backend/things/share/devices/msg.go)、[RawRecord](../../../../backend/share/contracts/kafka/raw_record.go)和 P1 缓存信封分别有既有用途，均不等于本次最终 Canonical 格式。[模型发布](../../../../backend/things/service/dmsvr/internal/domain/modelrelease/service.go)虽有不可变包，实际实时处理仍需核对版本接线。

## 候选规则与职责

<a id="PA-UP-SCOPE"></a>
### PA-UP-SCOPE：目标与首版范围

把一个真实客户协议的设备 payload 解码为标准设备消息，经过持久接受后驱动 DM 属性处理；同时支持标准协议经同一边界校验与转发。首版重点验证属性闭环；上行事件和命令回执必须正确分类与分流，不承诺本轮同时完成所有下游能力。下行编码、协议市场、任意代码上传和历史原报文重算不属于首版。

<a id="PA-UP-INPUT"></a>
### PA-UP-INPUT：可信输入与版本

Access Adapter 负责认证、可信租户/设备绑定、接入元数据和协议应答，提供原始应用 payload，不能只信任 payload 自报租户。协议适配选择固定 Codec 版本；参与解释的模型/映射版本也要能追溯。网关包要逐子设备核对归属。父包身份、接收时刻、来源、稳定子项身份及版本字段的生成方与格式尚待冻结，Kafka offset 不直接充当业务身份。

<a id="PA-UP-DECODE"></a>
### PA-UP-DECODE：转换与领域处理分工

Codec 承担客户协议解释与标准化，输出零到多条带类型的转换结果；框架负责版本选择、执行限制、结果交付和失败传播。Codec 不隐式读可变业务状态、写业务数据库或执行设备动作。DM 继续承担领域权限/归属与模型校验、持久化、业务去重和副作用安排。ProtocolApplication 可承载转换框架，进程及部署拓扑仍是候选。

<a id="PA-UP-ACCEPT"></a>
### PA-UP-ACCEPT：持久接受与业务完成

首版逻辑链为 **设备 → Access Adapter → ProtocolApplication / Codec → Canonical Kafka（持久接受）→ DM（业务完成）**。箭头表示责任流，不预设每个框都单独部署。

解码成功、进入内存或接入层收到报文均不能记为持久接受。只有标准消息及必要的结果清单按后续冻结的提交契约成功持久提交，才能声明平台已接受并进入 D6 的补处理范围。DM 的领域结果及必要副作用安排可靠记录后才称业务完成；设备动作、外部通知另有完成语义。

提交成功但接入应答丢失时，调用方可能只知道结果未知；后续设计要说明重投如何保留身份、去重及查询结果，不能因重试产生重复业务效果。首版无 Raw consumer offset，不照搬历史 Raw→Canonical Kafka 事务或全链恰好一次承诺。

<a id="PA-UP-FAILURE"></a>
### PA-UP-FAILURE：失败、无输出与部分结果

解码失败不得回接受成功。确定性坏输入/坏输出、瞬时依赖失败、合法无输出和结果未知需要分别表达，避免将故障当作可忽略数据。现阶段尚未决定 Quarantine 的保存位置、保留时长或整包/逐项接受策略；失败记录不等于已具备原报文恢复能力。

同包有有效和失败子项时，必须能汇总完整结果，不能只凭部分 Canonical 成功就回整包成功。父包/子项清单、终结状态、汇总责任方及应答时序是实施前待决契约。上行命令回执不得误入遥测或因解析/重试路径触发设备命令重发。

<a id="PA-UP-ISOLATION"></a>
### PA-UP-ISOLATION：通道与资源隔离

与 Eventbus 共同满足 D12：上报积压时，控制回执及重要业务补处理能独立推进。接入最早可行位置需可靠识别消息类型并提供资源预算；只在完整解码后拆 Kafka topic 可能仍被同一解码队列阻塞。混合包和必须完整解析才可分类的协议，需要明确最小分类步骤、预算和失败方案。吞吐、超时、保留窗口及故障预算尚未确定，不能宣称任意超载下可用。

<a id="PA-UP-DEPENDENCIES"></a>
## PA-UP-DEPENDENCIES：与 Kafka 迁移的协作

| 依赖方向 | 当前可用依据与未完成部分 |
| --- | --- |
| Eventbus 基础 → 本 Feature | [p1-v1](../../platform/eventbus/spec.md)已验证局部公共信封、回调完成及缓存恢复；设备 Canonical 的类型、payload 和提交接口仍须共同设计，不能直接复用缓存用例当设备链通过 |
| 本 Feature → Kafka P2 / DM | 本规格提供协议解释与接受边界；新链路实施前需落实样本、wire schema、结果汇总、顺序/去重及 DM 接线，然后共同验证一个纵向切片 |
| 协议入口 + Eventbus 控制/业务链 → D12 / V9 | 本 Feature 验证回执入口与解码资源隔离；完整停机操作、解绑及端到端 V9 仍依赖后续 P3/P4，不由上行局部测试替代 |

这是逐阶段接口协作，不要求“Kafka 全迁完才开始 PA”或“PA 全平台做完才开始 P2”。业务语义变化回到所属 Feature；公共信封/传输变化回到 Eventbus，同一候选关联受影响条款和各自认领。当前文档登记不授予任何阶段的实施权限。

<a id="PA-UP-VERIFY"></a>
## PA-UP-VERIFY：后续运行验收设计（均未执行）

| 场景 | 应观察到的结果 | 前置契约 |
| --- | --- | --- |
| 真实协议属性上报与标准协议输入 | 单次解释、合法 Canonical 被持久接受、DM 产生预期属性结果 | 真实样本、模型/Codec 版本、schema 与 DM 接线 |
| 不可解析或非法租户/子设备 | 不回接受成功，不形成越权业务结果，失败可定位 | 身份来源、错误分类与接入应答 |
| 接受前进程故障 / 接受后 DM 暂停 | 前者不宣称平台可恢复原输入；后者恢复后继续处理且不重复业务效果 | 提交契约、去重身份、恢复测试 |
| 标准消息提交后应答丢失、上游重投 | 结果未知可辨认，按既定身份去重；无稳定身份的协议明确能力限制 | 协议重投能力、ID 与结果查询/应答 |
| 混合有效/失败子项及合法无输出 | 完整子项结果可汇总，局部成功不会伪装成整包成功 | 父包清单、接受策略、终结与汇总责任 |
| 遥测积压，同时收到命令回执 | 回执识别与资源隔离可验证，不触发物理命令重发 | 分类与预算；完整 V9 另需 P3/P4 |
| 受控适配版本变化或执行故障 | 实际使用的版本可追溯，失败不冒充成功；隔离按方案验证 | 发布/绑定方式、故障限制和部署选择 |

这些是验收要求与提案，不是已具备的测试入口或通过记录。文档层当前可运行 `make knowledge-check`；产品实现后再按实际改动选择 verify-pr 和真实依赖验证。

<a id="PA-UP-OPEN"></a>
## PA-UP-OPEN：进入实施前仍须完成

1. AI 从现有 DG 脚本、调用和可用设备样本选定首个真实协议，确认重投/顺序能力；缺少客户材料时明确请求来源，不制造样本事实。
2. AI 提出 Canonical wire schema、可信身份与版本来源、标准协议路径及现有数据契约兼容方案，由相关实现责任方复核。
3. 冻结提交/接受/业务完成/接入应答时序，父包与子项结果、部分失败政策和去重窗口；新增业务取舍由用户确认，已确认 D13–D15 不重复询问。
4. 决定 ProtocolApplication 的部署与资源隔离方式，核验遥测与回执最早可行的分流点；受控发布责任与长期维护人待实际分配。
5. 取得具体实施范围授权后，登记真实产品差异映射并建立测试/运行基线。当前规格仍为 draft，不能据文档检查通过启动 P2。
