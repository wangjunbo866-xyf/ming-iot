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

本节于 2026-10-12 在隔离父仓中补记，仅记录下列历史提交的源码与测试断言，不改变原正文的状态、历史基线、验收标准或审批边界。第 2.2 节“当前不存在 Kafka client”等表述保留为原设计时点的基线；第 5.4 节的历史措辞也不改写。本节不表示本轮实现、测试、独立评审或最终验收已完成。

### 快照版本与修复范围

- 产品实际 HEAD：`fa9ee2765884333b4696bee4bfab7352e52561cf`。
- 修复前父提交：`50418c637c36642eba3bc84cda644f23e39699b4`。
- 父仓 HEAD：`69674f6eec3c65506b2e10f9119e15f94f754fab`。
- 父仓 `HEAD:repos/ming-iot` 固定版本（gitlink）：`fa9ee2765884333b4696bee4bfab7352e52561cf`，与产品实际 HEAD 一致；父仓 index 中的 gitlink 也一致。

只读核对了产品 `HEAD^` 到 `HEAD` 的文件清单与定点差异。修复只改动 `backend/share/foundation/kafka/` 下的 `consumer.go`、`consumer_test.go`、`config.go`、`config_test.go`。下文链接均指向本隔离副本中上述产品 HEAD 的源码；它们不是执行回执。

### Consumer factory 与测试断言

[consumer.go](../../repos/ming-iot/backend/share/foundation/kafka/consumer.go) 的 `NewReadCommittedConsumer` 不再显式设置 `kgo.GroupProtocol`；[config.go](../../repos/ming-iot/backend/share/foundation/kafka/config.go) 的 `commonClientOptions` 也未设置该选项。相对父提交，修复删除了 `kgo.GroupProtocol("classic")`，并将函数注释改为“标准 consumer 分组类型”。

[consumer_test.go](../../repos/ming-iot/backend/share/foundation/kafka/consumer_test.go) 中 `TestNewReadCommittedConsumerAppliesEffectiveOptions` 的断言从期待 `"classic"` 改为期待 `"consumer"`，连续原文如下：

```go
	if got := client.OptValue(kgo.GroupProtocol); got != "consumer" {
		t.Fatalf("GroupProtocol = %#v, want consumer", got)
	}
```

因此，此快照的事实是“未显式覆盖 GroupProtocol，测试期待值为 consumer”，不能把旧规格中的 classic 措辞当作仍存在的 `GroupProtocol("classic")` 调用，也不能仅据此推断真实 broker 的协议协商结果或一次测试已通过。

已有消费约束仍保留在 factory 的选项中，以下为连续原文：

```go
	consumerOpts := []kgo.Opt{
		kgo.ConsumeTopics(topicNames...),
		kgo.ConsumerGroup(config.GroupID),
		kgo.Balancers(kgo.CooperativeStickyBalancer()),
		kgo.FetchIsolationLevel(kgo.ReadCommitted()),
		kgo.DisableAutoCommit(),
	}
```

factory 仍校验稳定 group ID、受管且不重复的 Topic；正式档仍使用 `kgo.NoResetOffset()`，本地档仍使用 `kgo.NewOffset().AtStart()`。同一 consumer 测试保留对单个 `cooperative-sticky` balancer、read-committed（选项值 `int8(1)`）、`DisableAutoCommit == true`、正式档 `NoResetOffset` 和禁止自动创建 Topic 的断言；`TestNewReadCommittedConsumerUsesAtStartForLocal` 保留本地档 AtStart 断言。冻结签名测试仍要求 factory 仅接收 `ConsumerConfig`，不接受可变选项覆盖。关闭自动提交保留了显式处理 offset 的消费约束，但这些源码和断言不证明业务侧实际完成了手动提交。

### Broker 地址空白校验

[config.go](../../repos/ming-iot/backend/share/foundation/kafka/config.go) 的 `validateClientConfig` 从父提交的 `strings.TrimSpace(broker) != broker`（仅拒绝首尾空白），改为在每个 broker 字符串的任意位置查找 `unicode.IsSpace` 所识别的 Unicode 空白；命中即拒绝，不做静默裁剪。当前连续原文如下：

```go
		if strings.IndexFunc(broker, unicode.IsSpace) >= 0 {
			return fmt.Errorf("broker %q must not contain whitespace", broker)
		}
```

这一检查位于 local/production 分支之前，对两种合法 profile 的每个 broker 都适用。空列表、空字符串、重复 broker 的拒绝仍保留。本修复不扩张 host/port 地址解析语义；这里的“任意 Unicode 空白”严格以 `unicode.IsSpace` 判定为界。

[config_test.go](../../repos/ming-iot/backend/share/foundation/kafka/config_test.go) 中 `TestValidateClientConfigRejectsInvalidInputs` 保留首部空格样例，并新增内部空格样例，连续原文如下：

```go
		{name: "broker has surrounding whitespace", cfg: func() ClientConfig { cfg := base(); cfg.Brokers = []string{" 127.0.0.1:9092"}; return cfg }},
		{name: "broker has internal whitespace", cfg: func() ClientConfig { cfg := base(); cfg.Brokers = []string{"host :9093"}; return cfg }},
```

该表驱动测试逐项调用 `validateClientConfig(test.cfg())`，若错误为 nil 则失败，表达了两个样例均应被拒绝的断言。样例使用 local profile 和 ASCII 空格；读取到的 broker 测试未逐项覆盖所有 Unicode 空白、所有位置及两个 profile 的组合。实现范围由源码判定式支持，测试源码只说明已写下哪些样例和断言。

### 证据与验收边界

本次只做文档维护和静态源码、测试断言、Git 差异核对，未运行产品测试、Kafka 集成探针、构建或历史操作，也未进行新的独立评审。当前未提供这次修复的原始产品运行证据；测试文件存在、断言内容以及提交消息中的“独立复审 PASS”均不能作为本次测试执行证明，也不是本次独立验证过的历史执行或评审回执。不能据此断言历史测试已被证明通过。

原规格中的版本裁决、Evidence 要求和“独立 Reviewer 后进入用户最终验收”等边界继续原样保留。本补记不升级文档状态、不授权实现或发布、不迁移 map、不启用 OpenSpec。规格索引只增加到本节的导航；本次实际文档检查及其局部/全库覆盖差异记录在父仓根目录的 `report.json`。
