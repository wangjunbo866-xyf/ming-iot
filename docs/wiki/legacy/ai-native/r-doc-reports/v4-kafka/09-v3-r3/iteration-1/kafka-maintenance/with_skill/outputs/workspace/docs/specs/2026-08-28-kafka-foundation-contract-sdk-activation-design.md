# Kafka 基础契约与 SDK 激活设计

**日期：** 2026-08-28  
**状态：** 待验收  
**适用项目：** `ming-iot`  
**对应 Agent Todo：** `todo_01555fb1588d`  
**前置决策：** `todo_f666aca7d078` 已批准优先建设 Kafka 基础契约与 SDK  

## 1. 目的

本设计激活 `ming-iot` 的第一个 Kafka 实施 Loop：先建立跨业务模块可复用、可机器验证的 Kafka 契约和安全客户端底座，再由后续 Loop 分别实现协议接入、物模型发布、设备状态 Reducer 和场景联动。

本 Loop 不以“把 NATS API 换成 Kafka API”为目标，也不直接迁移现有业务消息链路。它解决的是更靠前的问题：让后续业务代码只能使用经过约束的 Topic、Key、Header、客户端档位和部署配置，避免各服务自行发明 Kafka 语义。

## 2. 权威来源与当前基线

### 2.1 权威来源

1. `iot-doc/亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
   - `share/contracts` 拥有跨领域稳定契约；
   - `share/foundation` 拥有纯基础设施能力；
   - 生产数据路径统一采用 Kafka Raw/Canonical 语义；
   - `protocol`、`things/runtime` 和 Scene Runtime 分属后续领域 Loop。
2. `iot-doc/亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md`
   - 采用 Kafka-only 绿地设计，不建设 NATS 兼容层、双写层或通用 EventBus；
   - Kafka 基线为 `4.3.1`、KRaft；
   - Topic 必须来自受管 Catalog，禁止动态创建每设备、每租户或每场景 Topic；
   - Raw、Canonical、quarantine、replay 和领域事件具有不同语义；
   - durable producer 使用 `acks=all` 和幂等写；正式 consumer 使用 `read_committed`、关闭自动提交并显式处理 offset；
   - Raw 的稳定来源身份为 `sourceClusterId/topicId/partition/offset`。
3. `ming-iot-rd-docs/proposals/2026-08-23-first-real-delivery-loop.md`
   - 将 Kafka 基础契约、SDK、部署配置和机器校验器列为首个真实交付候选。
4. `ming-iot/backend/go.work`
   - 当前工作区使用 Go `1.24.4`；`share` 与 `tools` 已加入 workspace，尚无 `protocol` module。

以上来源在本设计获批并进入正式文档目录后，应先登记到项目 `DOC_REGISTRY.yaml`，通过项目文档目录校验，再按 `loopx-doc-registry` 顺序登记 LoopX Authority。当前 worktree 中的文件仍是候选文档，不提前登记裸哈希。

### 2.2 代码事实

- 当前业务代码仍有 NATS/JetStream 依赖；
- 当前不存在 Kafka client、受管 Topic Catalog 或 Kafka 领域实现；
- `backend/share` 已存在，适合作为跨领域契约和纯基础设施的归属模块；
- `backend/tools` 已采用 `cmd/<tool>` 与 `internal/<tool>` 的机器校验器组织方式；
- `deploy` 已拥有统一 Makefile、镜像和基线 profile，Kafka 配置应沿用同一部署入口。

## 3. 决策

采用“共享契约 + 共享基础设施 + 独立机器校验器”的小步激活方案：

```text
backend/share/contracts/kafka/     # 纯值对象、Catalog、编码与校验契约
backend/share/foundation/kafka/    # franz-go 配置档位与受约束 client builder
backend/tools/cmd/kafkacontract/   # 可执行校验入口
backend/tools/internal/kafkacontract/
deploy/kafka/                      # Kafka 4.3.1 KRaft 配置与版本锁
```

这三个边界分别回答：

- **允许发送和消费什么：** `share/contracts/kafka`；
- **如何安全连接和运行：** `share/foundation/kafka`；
- **仓库是否仍满足契约：** `kafkacontract`。

### 3.1 方案比较

| 方案 | 优点 | 主要问题 | 结论 |
| --- | --- | --- | --- |
| A. `share/contracts` + `share/foundation` | Protocol、Things Reducer、Scene Admission 和工具均可复用；职责与总体架构一致 | 需要严格阻止 foundation 演化成通用 EventBus | **采用** |
| B. 全部放进未来 `backend/protocol` | 协议接入代码集中 | Things projection 和其他领域会反向依赖 Protocol；在 ProtocolApplication 尚未设计时先创建空壳模块 | 拒绝 |
| C. 本 Loop 完成全部 NATS→Kafka 迁移 | 表面上一步到位 | 写范围跨 SaaS、Things、Protocol、部署和状态语义，无法形成可审查的小 Loop | 拒绝 |

## 4. 版本裁决

### 4.1 Kafka

- 本地和验证环境固定 Apache Kafka `4.3.1`、KRaft；
- 使用明确版本和镜像 digest 的版本锁文件，禁止 `latest`；
- 本地允许单个 combined broker/controller，但配置和文档必须明确标注“仅本地开发，不证明 HA”；
- 生产静态配置契约要求至少 3 broker、RF=3、`min.insync.replicas=2`、`acks=all`、关闭自动建 Topic 和 unclean leader election。

### 4.2 franz-go 与 Go 工具链冲突

Kafka 总纲冻结的 `franz-go v1.21.5` 要求 Go `1.25.0`，而当前 `ming-iot` workspace 为 Go `1.24.4`。本 Loop 不隐式升级整个 workspace，因此：

- 本 Loop 临时固定 `github.com/twmb/franz-go v1.20.5`；
- `v1.20.5` 的模块声明为 Go `1.24.0`，与当前 workspace 兼容；
- 该选择只表示“可进入 Phase 0 兼容性验证”，不自动宣称支持 Kafka 4.3.1 的所有事务、rebalance 和故障语义；
- Kafka 4.3.1 集成验证结果必须进入 Evidence；
- 升级 workspace Go 1.25 后，另立依赖升级动作恢复 Kafka 总纲目标版本，重新执行兼容性门禁。

本节是对当前实现版本的显式冲突裁决，不能在 `go get` 时静默改成其他版本。

## 5. 契约设计

### 5.1 Topic Catalog

Catalog 是编译期/配置期受控集合，不是运行时自由字符串注册表。每项至少包含：

- 逻辑名、物理名和 generation；
- plane、fact kind、schema major；
- partition 数和 key codec；
- cleanup policy、retention、`LogAppendTime`；
- producer owner、consumer owner；
- ACL/profile 标识。

Catalog 校验必须拒绝：

- 空名称、通配符和非法 token；
- 每设备、每租户、每场景动态 Topic 模式；
- 没有 generation 的有序或有状态物理 Topic；
- Raw retention 小于 72 小时；
- 依赖 broker 默认 retention 或 timestamp 类型；
- 生产 RF=1、`minISR<2` 或开启 auto-create；
- 未声明 owner、key codec 或 schema major 的条目。

第一版只冻结后续 Loop 确定会使用的受管数据面骨架，不为未实施业务伪造完整 Catalog。至少覆盖：

- `iot.data.raw.v1.<protocolKey>` 的模板约束；
- Canonical telemetry/device-event/lifecycle/command-reply/outcome；
- ingress/protocol quarantine。

Scene Topic 的 wire、route、partition count 和恢复协议不在本 Loop 冻结，避免 Kafka 基础包成为 Scene 语义的第二权威来源。

### 5.2 Raw Record

`RawRecord` 契约只描述 Kafka record 边界：

```text
topic      = Catalog 根据 protocolKey 解析出的受管 physical topic
key        = transport identity codec 的稳定 bytes
value      = 原始 transport payload bytes
required   = message-kind header
optional   = mqtt-qos、mqtt-retain、broker-received-at、traceparent
timestamp  = broker LogAppendTime
```

约束：

- Adapter 不解析 tenant、内部设备 ID、物模型或业务 schema；
- key 和 value 不允许空切片被隐式修补；
- Header 名和值采用受控编码，未知必填字段、重复唯一 Header 和越界值 fail closed；
- Raw codec 只负责确定性验证/组装，不执行网络 I/O。

### 5.3 RawRef

稳定来源引用固定为：

```text
sourceClusterId/topicId/partition/offset
```

`RawRef` 必须提供确定性编码、严格解码和 round-trip 测试，并拒绝：

- 缺少任一组成字段；
- 负 partition 或 offset；
- 使用 Topic 名冒充不可变 `topicId`；
- 非法分隔、额外字段和非规范编码。

### 5.4 客户端档位

不提供 `Publish(topic string, any)`、`Subscribe(pattern string)` 或仿 NATS EventBus 接口。Foundation 只暴露按职责收窄的配置和 builder：

- durable producer：`acks=all`、幂等写、有界 batch/queue、回调错误不可丢；
- read-committed consumer：关闭自动提交、classic cooperative-sticky、正式档 `NoResetOffset`；
- transactional builder：只提供 Kafka→Kafka 原子流程所需配置，不在本 Loop 实现 ProtocolApplication transaction；
- local profile：允许单 broker 和显式开发凭据；
- production profile：TLS/SASL 配置必填，不接受 PLAINTEXT、空 principal、auto-create 或随机 group/client identity。

所有 builder 只返回已配置的 franz-go client 或 options；业务处理、重试状态机、partition worker、Raw→Canonical transaction 和 Scene producer 均属于后续领域 Loop。

## 6. 部署与配置

`deploy/kafka` 提供：

- Kafka 4.3.1 KRaft 本地单节点 compose/profile；
- 版本锁，记录镜像引用和 digest；
- 显式 Topic bootstrap/catalog 输入；
- 生产静态配置契约样例；
- readiness 检查和可重复清理说明；
- 统一由 `deploy/Makefile` 暴露启动、停止、检查入口。

不得把真实密码、证书私钥或本地 `.env` 写入 Git。示例只给变量名和非敏感默认值。

## 7. 机器校验器

`kafkacontract` 是仓库门禁，不是运行时服务。它至少执行：

1. 校验 Catalog 的名称、generation、key、retention、owner 和生产耐久策略；
2. 校验 local/production profile 不混用；
3. 校验 Kafka/franz-go/Go 版本锁；
4. 检查禁止的 wildcard、动态 Topic 和 auto-create；
5. 运行 RawRecord、RawRef 的确定性向量；
6. 可选连接本地 Kafka 进行真实 produce/consume 探针；
7. 生成确定性 Evidence JSON。

未传集成测试开关时，纯契约校验不得要求 Docker 或网络。显式开启集成测试时，Kafka 不可达必须失败，不能伪装成跳过后 PASS。

## 8. Evidence

最终 Evidence 至少记录：

- source commit、dirty 状态和执行时间；
- Go、Kafka、franz-go、镜像 digest；
- Catalog 条目数量和摘要；
- 单元测试、静态校验和集成探针结果；
- 真实 record 的 topic、partition、offset、key/header/value digest；
- 失败用例摘要；
- 配置摘要与敏感字段脱敏结果。

Evidence 必须可由同一 source commit 重跑，且不得包含密码、token、私钥、完整设备 payload 或环境秘密。

## 9. 精确写范围

后续实施 Todo 只允许修改：

```text
backend/share/contracts/kafka/**
backend/share/foundation/kafka/**
backend/share/go.mod
backend/share/go.sum
backend/go.work.sum
backend/tools/cmd/kafkacontract/**
backend/tools/internal/kafkacontract/**
backend/tools/README.md
deploy/kafka/**
deploy/Makefile
deploy/README.md
```

说明：

- `backend/go.work` 不需要修改，因为 `share` 与 `tools` 已在 workspace；
- `backend/go.work.sum` 是 Go 命令可能产生的机械变更，但必须检查并提交，不能忽略；
- 若实现发现必须越出以上范围，应停止并重新审批范围，不能顺手修改。

## 10. 非目标

本 Loop 明确不做：

- 修改 SaaS、Things 或现有服务的 NATS producer/consumer；
- 删除 NATS 依赖或部署；
- 建设 NATS→Kafka adapter、双写或兼容抽象；
- 创建 ProtocolApplication、Access Adapter 或 `backend/protocol` module；
- 实现 Raw→Canonical transaction、物模型、Materializer、Reducer、Scene Runtime 或下行命令；
- 建设生产 Kafka 集群或宣称 HA/性能达标；
- 升级整个 Go workspace；
- commit、merge 或 push，除非用户另行明确授权。

## 11. 验收标准

1. 纯契约包不依赖网络、Docker、业务服务或 franz-go；
2. Topic Catalog、RawRecord 和 RawRef 的正反向测试通过；
3. 生产配置无法降级为 auto-create、PLAINTEXT、RF=1、`minISR<2` 或非 `acks=all`；
4. Foundation 没有通用 `Publish/Subscribe` 或 wildcard API；
5. `franz-go v1.20.5` 与 Go 1.24.4 的锁定可机器验证；
6. 本地 Kafka 4.3.1 探针证明 key、header、value 和 offset 可观测且保持一致；
7. Kafka 不可达、非法 Catalog、错误 RawRef 和错误 profile 均产生可解释失败；
8. Evidence 可重复生成、无秘密、绑定 source commit；
9. `go test`、`go vet`、格式检查和仓库既有相关门禁通过；
10. 独立 Reviewer 检查范围、契约语义和 Evidence 后，才进入用户最终验收。

## 12. 风险与回滚

| 风险 | 控制措施 |
| --- | --- |
| 基础包演化为万能 EventBus | 只暴露受约束 Catalog、codec 和职责化 builder；禁止字符串 Topic 和 wildcard API |
| franz-go 版本低于架构目标 | 显式登记 Go 版本冲突；Phase 0 实测；Go 1.25 升级后单独恢复目标版本 |
| 单节点本地环境被误认为生产基线 | profile、README、校验器同时标注 local-only；生产配置另行验证 |
| Catalog 过早冻结业务细节 | 只冻结已有权威来源明确要求的骨架；领域 payload 和 runtime 留给后续 Loop |
| 实施顺手迁移 NATS | 精确写范围 + 非目标 + diff 校验三重约束 |

回滚方式是删除本 Loop 新增的共享 Kafka 包、校验器和部署 profile，并恢复 `share` 依赖文件。由于本 Loop 不接管任何现有生产路径，回滚不涉及业务数据迁移或双写收敛。


<a id="historical-kafka-client-fix"></a>

## 历史实现快照（Kafka 客户端修复）

本节于 2026-10-13 在隔离父仓中补记，只记录下列历史提交的源码与测试断言。前文原始正文逐字节保留：状态仍为“待验收”，第 2.2 节“当前不存在 Kafka client”等是 2026-08-28 设计时的历史基线，第 5.4 节的 `classic cooperative-sticky` 表述及原审批、Evidence、独立 Review 和最终验收边界也保持不变。本节不重新激活或批准实施，不将历史基线改写为本次检出事实，也不表示原验收已完成。

### 快照版本与差异范围

| 对象 | 本次 Git 实测结果 |
| --- | --- |
| 产品子模块 `repos/ming-iot` 的实际 HEAD（修复提交） | `fa9ee2765884333b4696bee4bfab7352e52561cf` |
| 修复提交的直接父提交 | `50418c637c36642eba3bc84cda644f23e39699b4` |
| 父仓 HEAD | `69674f6eec3c65506b2e10f9119e15f94f754fab` |
| 父仓 HEAD 中 `repos/ming-iot` 的固定版本（gitlink） | `fa9ee2765884333b4696bee4bfab7352e52561cf` |

子模块已初始化，实际 HEAD 与父仓 gitlink 一致；调查开始时父子仓工作树均无改动。核对方式为 `git ls-tree HEAD repos/ming-iot`、`git submodule status`、`git -C repos/ming-iot rev-parse HEAD HEAD^`，并检查修复提交相对父提交的路径清单及限定路径差异。此次修复只修改 `backend/share/foundation/kafka/` 下的 `consumer.go`、`consumer_test.go`、`config.go`、`config_test.go`。以下结论绑定该产品 HEAD，不代表其他版本或运行环境。

### Consumer factory 与测试断言

[consumer.go](../../repos/ming-iot/backend/share/foundation/kafka/consumer.go) 中的 `NewReadCommittedConsumer` **没有显式设置 `kgo.GroupProtocol`**。相对父提交，删除的是 `kgo.GroupProtocol("classic")`；公共选项构造函数 `commonClientOptions` 也没有追加该选项。

[consumer_test.go](../../repos/ming-iot/backend/share/foundation/kafka/consumer_test.go) 的 `TestNewReadCommittedConsumerAppliesEffectiveOptions` 将 `client.OptValue(kgo.GroupProtocol)` 的预期值从 `"classic"` 改为 **`"consumer"`**。准确含义是“移除 factory 的显式覆盖，并由测试断言期待默认值 consumer”，不能写成 factory 显式调用了 `kgo.GroupProtocol("consumer")`。旧规格中的 `classic` 字样也不能继续作为这里 GroupProtocol 字符串取值的依据。本次没有运行 client 或 broker，也没有检查外部依赖源码来验证默认值的运行时效果。

同一 factory 中下列约束及对应测试断言在这次差异中保留：

| 消费约束 | 源码与测试的静态依据 |
| --- | --- |
| 协作式 sticky 分配 | `kgo.Balancers(kgo.CooperativeStickyBalancer())`；测试要求仅有一个 balancer，`ProtocolName()` 为 `"cooperative-sticky"` |
| read-committed | `kgo.FetchIsolationLevel(kgo.ReadCommitted())`；测试要求隔离级别为 `int8(1)` |
| 关闭自动提交，保留业务手动提交责任 | `kgo.DisableAutoCommit()`；测试期待该选项为 `true`，不证明业务已实际提交 offset |
| 生产档禁止自动重置 offset | `ProfileProduction` 分支使用 `kgo.ConsumeResetOffset(kgo.NoResetOffset())`；测试期待 `NoResetOffset()` |
| 本地档起始 offset | 非生产分支使用 `kgo.NewOffset().AtStart()`；`TestNewReadCommittedConsumerUsesAtStartForLocal` 保留对应断言 |
| 受管 Topic、稳定 group 与不可追加任意选项 | factory 校验 Topic 和 GroupID，签名仍为 `func(ConsumerConfig) (*kgo.Client, error)`；受管 Topic、group、非法配置和冻结签名测试仍在 |

有效选项测试还保留 `AllowAutoTopicCreation == false` 的断言；factory 没有开启自动建 Topic。这些是源码和断言的保留情况，不是本次消费、rebalance 或故障恢复实测结果。

### Broker 地址空白校验

[config.go](../../repos/ming-iot/backend/share/foundation/kafka/config.go) 的 `validateClientConfig` 原来用 `strings.TrimSpace(broker) != broker` 拒绝首尾空白；修复后改为 `strings.IndexFunc(broker, unicode.IsSpace) >= 0`。因此现在拒绝地址任意位置被 `unicode.IsSpace` 判定为空白的 Unicode 字符，包括首尾和内部空白，策略记为 `any_unicode`。它在 local/production 分支之前遍历每个 broker，适用于两个有效档位。空 broker 列表、空地址和重复地址的既有拒绝逻辑仍保留；本修复不增加完整 host/port、DNS 或网络可达性校验，也不能泛化为拒绝所有不可见字符。

[config_test.go](../../repos/ming-iot/backend/share/foundation/kafka/config_test.go) 的 `TestValidateClientConfigRejectsInvalidInputs` 保留 `" 127.0.0.1:9092"` 首部空格反例，新增 `"host :9093"` 内部空格反例；表驱动循环要求 `validateClientConfig(test.cfg())` 返回非 nil 错误。这两项使用 local 基础配置，证明测试源码声明了首部和内部 ASCII 空格的拒绝预期。任意 Unicode 空白及两个档位的覆盖范围来自共享校验代码，不应宣称测试已穷举 Unicode、尾部空白或 production 空白组合，更不代表本次已执行这些测试。

### 证据与本次维护边界

本次只追加本文与规格索引，并新增根目录 `report.json` 记录引用和检查结果。没有修改产品源码、原文状态、历史基线、审批边界、项目适配配置或 map，也没有启用 OpenSpec。

没有运行产品测试、集成探针或文档中的历史操作，也没有开展新的独立评审。测试文件的存在只证明断言存在；修复提交消息中的“独立复审 PASS”只是提交者的历史陈述，不等于本次测试通过，也不是已独立验证的历史执行回执。当前未提供此次修复的原始产品运行证据，因此不能认定历史测试已被证明通过，不能据此完成原规格验收。文档定点检查、项目元数据检查与显式全库审计分别记录于 `report.json`，均不替代产品运行证据。
