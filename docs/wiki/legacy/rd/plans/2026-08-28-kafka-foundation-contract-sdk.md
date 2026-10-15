# Kafka 基础契约与 SDK 实施计划

> **执行说明：** 实施者必须按任务顺序执行，先写失败测试，再写最小实现。Plan Task 是一个 Agent Todo 内部的工程检查点，不得为每个 Task 创建 LoopX Todo 或 User Gate；整个 Todo 完成并验证后只创建一次汇总验收 Gate。commit、merge、push 仍需用户分别明确授权。

**目标：** 在不修改现有 NATS 业务路径的前提下，为 `ming-iot` 建立可复用、可机器验证的 Kafka Topic/Raw 契约、安全 franz-go 客户端档位、本地 Kafka 4.3.1 环境和 Evidence 生成能力。

**架构：** 纯契约进入 `backend/share/contracts/kafka`，franz-go 基础设施进入 `backend/share/foundation/kafka`，仓库门禁进入 `backend/tools/internal/kafkacontract`，本地与生产配置契约进入 `deploy/kafka`。业务服务只能在后续领域 Loop 中通过这些收窄接口接入 Kafka。

**技术栈：** Go 1.24.4、`github.com/twmb/franz-go v1.20.5`、Apache Kafka 4.3.1 KRaft、Docker Compose、Go `testing`。

**设计依据：** `specs/2026-08-28-kafka-foundation-contract-sdk-activation-design.md`

## 执行边界

允许写入：

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

禁止修改 SaaS、Things、NATS、Protocol、Reducer、Scene 业务代码。发现范围不足时停止执行并报告，不扩大范围。

## Task 1：建立 Topic Catalog 纯契约

**文件：**

- 新建：`backend/share/contracts/kafka/catalog.go`
- 新建：`backend/share/contracts/kafka/catalog_test.go`
- 新建：`backend/share/contracts/kafka/names.go`
- 新建：`backend/share/contracts/kafka/names_test.go`

### 步骤 1：写失败测试

覆盖：

- 合法逻辑名、物理 generation 名和 protocolKey Raw 模板；
- wildcard、空 token、设备/租户/场景动态 Topic 被拒绝；
- 有序/有状态 Topic 缺 generation 被拒绝；
- 缺 key codec、owner、schema major 或 timestamp policy 被拒绝；
- Raw retention 小于 72 小时被拒绝；
- production RF=1、`minISR<2`、auto-create 被拒绝；
- Catalog 排序和摘要在相同输入下确定一致。

运行：

```bash
cd backend
go test ./share/contracts/kafka -run 'TestCatalog|TestTopicName' -count=1
```

预期：包尚未实现，测试失败。

### 步骤 2：实现最小契约

实现不可变值对象、枚举、构造校验和确定性 Catalog 摘要。不得引入 franz-go、Docker、环境变量或业务服务依赖。

### 步骤 3：验证

```bash
cd backend
gofmt -w share/contracts/kafka/*.go
go test ./share/contracts/kafka -run 'TestCatalog|TestTopicName' -count=1
```

预期：测试通过。

## Task 2：建立 RawRecord 与 RawRef 契约

**文件：**

- 新建：`backend/share/contracts/kafka/raw_record.go`
- 新建：`backend/share/contracts/kafka/raw_record_test.go`
- 新建：`backend/share/contracts/kafka/raw_ref.go`
- 新建：`backend/share/contracts/kafka/raw_ref_test.go`

### 步骤 1：写失败测试

覆盖：

- Raw key/value、`message-kind` 必填 Header 和可选 Header；
- 空 key/value、重复唯一 Header、未知必填 Header、越界 QoS 被拒绝；
- `RawRef` 严格 round-trip；
- 缺字段、负 partition/offset、非法分隔、额外字段和 Topic 名冒充 topicId 被拒绝；
- 相同输入编码字节完全一致。

运行：

```bash
cd backend
go test ./share/contracts/kafka -run 'TestRawRecord|TestRawRef' -count=1
```

预期：测试失败。

### 步骤 2：实现最小 codec

只实现纯内存验证、规范化编码和解码；不执行 Kafka I/O，不解析 tenant、设备、物模型或 payload schema。

### 步骤 3：验证整个契约包

```bash
cd backend
gofmt -w share/contracts/kafka/*.go
go test ./share/contracts/kafka -count=1
```

预期：全部通过。

## Task 3：实现 franz-go 安全配置档位

**文件：**

- 新建：`backend/share/foundation/kafka/config.go`
- 新建：`backend/share/foundation/kafka/config_test.go`
- 新建：`backend/share/foundation/kafka/producer.go`
- 新建：`backend/share/foundation/kafka/producer_test.go`
- 新建：`backend/share/foundation/kafka/consumer.go`
- 新建：`backend/share/foundation/kafka/consumer_test.go`
- 新建：`backend/share/foundation/kafka/transaction.go`
- 新建：`backend/share/foundation/kafka/transaction_test.go`
- 修改：`backend/share/go.mod`
- 修改：`backend/share/go.sum`
- 修改：`backend/go.work.sum`

### 步骤 1：锁定兼容版本

运行并记录：

```bash
cd backend/share
GOWORK=off go get github.com/twmb/franz-go@v1.20.5
GOWORK=off go list -m -json github.com/twmb/franz-go
```

预期：模块版本为 `v1.20.5`，声明 Go 版本不高于当前 workspace 的 1.24.4。检查 `go.mod/go.sum/go.work.sum` 只有预期依赖变化。

### 步骤 2：写失败测试

覆盖：

- durable producer 强制 `acks=all`、幂等写和有界队列；
- consumer 强制 `read_committed`、关闭自动提交、classic cooperative-sticky；
- production 使用 `NoResetOffset`；
- local 与 production profile 不能混用；
- production 缺 TLS/SASL、principal、稳定 client/group identity 时失败；
- 不能通过 option 覆盖器降级安全约束；
- 公共 API 不出现通用字符串 Topic 的 `Publish/Subscribe`。

运行：

```bash
cd backend
go test ./share/foundation/kafka -count=1
```

预期：测试失败。

### 步骤 3：实现职责化 builder

分别实现 durable producer、read-committed consumer 和 transactional producer 的配置构造。builder 可以返回 franz-go options/client，但不实现业务 handler、重试状态机、Raw→Canonical transaction 或任意 Topic 发布接口。

### 步骤 4：验证

```bash
cd backend
gofmt -w share/foundation/kafka/*.go
go test ./share/foundation/kafka -count=1
go vet ./share/contracts/kafka ./share/foundation/kafka
```

预期：全部通过。

## Task 4：建立 Kafka 4.3.1 部署契约

**文件：**

- 新建：`deploy/kafka/compose.yaml`
- 新建：`deploy/kafka/version-lock.json`
- 新建：`deploy/kafka/config/local/server.properties`
- 新建：`deploy/kafka/config/production/server.properties`
- 新建：`deploy/kafka/catalog/data-plane.yaml`
- 新建：`deploy/kafka/scripts/bootstrap-topics.sh`
- 新建：`deploy/kafka/scripts/readiness.sh`
- 新建：`deploy/kafka/README.md`
- 修改：`deploy/Makefile`
- 修改：`deploy/README.md`

### 步骤 1：先定义静态失败检查

在后续 `kafkacontract` 测试 fixture 中先声明以下失败条件：

- 镜像使用 `latest` 或缺 digest；
- local profile 未标注 local-only；
- production 开启 auto-create、unclean leader election 或 PLAINTEXT；
- production RF、offset/transaction state RF 或 minISR 不满足基线；
- Topic 未显式使用 `LogAppendTime`；
- 脚本依赖隐藏的本机绝对路径或提交真实凭据。

### 步骤 2：实现部署文件

本地 profile 使用单节点 combined KRaft，只承担开发和集成测试。生产文件只表达机器可校验的静态基线，不在本 Loop 部署生产集群。

镜像 digest 必须通过实际 registry inspect 获取并写入版本锁；不得猜测或填写虚构值。

### 步骤 3：验证 Compose 和脚本

```bash
docker compose -f deploy/kafka/compose.yaml config --quiet
bash -n deploy/kafka/scripts/bootstrap-topics.sh
bash -n deploy/kafka/scripts/readiness.sh
```

预期：配置和脚本语法通过，输出不包含秘密。

## Task 5：实现 `kafkacontract` 门禁和真实探针

**文件：**

- 新建：`backend/tools/cmd/kafkacontract/main.go`
- 新建：`backend/tools/cmd/kafkacontract/main_test.go`
- 新建：`backend/tools/internal/kafkacontract/contract.go`
- 新建：`backend/tools/internal/kafkacontract/contract_test.go`
- 新建：`backend/tools/internal/kafkacontract/probe.go`
- 新建：`backend/tools/internal/kafkacontract/probe_test.go`
- 新建：`backend/tools/internal/kafkacontract/testdata/**`
- 修改：`backend/tools/README.md`

### 步骤 1：写 CLI 与配置校验失败测试

测试合法 fixture PASS，以下 fixture FAIL 且输出稳定原因码：

- 动态/wildcard Topic；
- retention、generation、owner、key codec 缺失；
- 生产耐久和安全配置降级；
- Kafka/franz-go/Go 版本锁不一致；
- 集成模式请求后 broker 不可达。

运行：

```bash
cd backend
go test ./tools/internal/kafkacontract ./tools/cmd/kafkacontract -count=1
```

预期：测试失败。

### 步骤 2：实现纯契约模式

默认命令只做文件和 Go 契约校验，不需要 Docker 或网络。输出稳定 JSON 摘要和人类可读错误。

### 步骤 3：实现显式集成探针

探针必须：

1. 连接受管测试 Topic；
2. 发送固定 key、`message-kind` 与可选 Header、固定 value；
3. 读取同一 record 并校验 key/header/value；
4. 记录 topic、partition、offset 和 digest；
5. 删除或隔离本轮测试资源；
6. broker 不可达、校验不一致时返回非零状态。

探针不得接管业务 Topic，不得使用 wildcard，不得输出完整 payload 或凭据。

### 步骤 4：验证

```bash
cd backend
go test ./tools/internal/kafkacontract ./tools/cmd/kafkacontract -count=1
go run ./tools/cmd/kafkacontract --repo-root ..
```

预期：纯契约模式 PASS，并生成稳定摘要。

启动本地 Kafka 后运行：

```bash
make -C deploy kafka-up
make -C deploy kafka-ready
cd backend
go run ./tools/cmd/kafkacontract --repo-root .. --integration
```

预期：真实探针 PASS，输出 record 元数据和 digest。结束验证后使用统一 Makefile 停止本地 profile。

## Task 6：Evidence、全量验证和评审交接

**文件：**

- 新建或由命令生成：`backend/tools/internal/kafkacontract/evidence.go`
- 新建：`backend/tools/internal/kafkacontract/evidence_test.go`
- 修改：`backend/tools/README.md`
- 修改：`deploy/kafka/README.md`

### 步骤 1：写 Evidence 失败测试

覆盖：

- 相同输入产生相同业务内容摘要；
- source commit、版本、Catalog 摘要、测试和探针结果齐全；
- 时间、临时路径和随机端口不污染契约摘要；
- password、token、private key、完整 payload 被拒绝或脱敏；
- dirty workspace 被明确记录，不伪装成 clean。

### 步骤 2：实现 Evidence 生成

Evidence 产物写入 LoopX 私有 evidence 目录或明确的本地输出目录，不把运行时日志和临时数据混入产品提交。记录失败也是证据，不能只保留最后一次 PASS。

### 步骤 3：运行聚焦和全量验证

```bash
cd backend
gofmt -w share/contracts/kafka/*.go share/foundation/kafka/*.go tools/cmd/kafkacontract/*.go tools/internal/kafkacontract/*.go
go test ./share/contracts/kafka ./share/foundation/kafka ./tools/internal/kafkacontract ./tools/cmd/kafkacontract -count=1
go test -race ./share/contracts/kafka ./share/foundation/kafka ./tools/internal/kafkacontract -count=1
go vet ./share/contracts/kafka ./share/foundation/kafka ./tools/internal/kafkacontract ./tools/cmd/kafkacontract
go test ./saas/... ./share/... ./things/... ./tools/... -count=1
```

```bash
docker compose -f deploy/kafka/compose.yaml config --quiet
bash -n deploy/kafka/scripts/bootstrap-topics.sh
bash -n deploy/kafka/scripts/readiness.sh
git diff --check
git status --short
```

预期：相关测试、race、vet、全量测试和静态检查通过；`git status` 只出现精确写范围内的文件。

### 步骤 4：独立 Review

Reviewer 必须基于同一候选 source commit 检查：

- 是否越出精确写范围；
- 是否出现通用 EventBus、字符串 Topic、wildcard 或 NATS 兼容层；
- local/production 配置是否混淆；
- franz-go 版本冲突是否被如实保留；
- Evidence 是否绑定 source commit 且不泄密；
- 失败路径是否真实执行。

Review 结论写入本地 R&D docs 或 LoopX 私有 evidence，并与候选提交哈希绑定。

### 步骤 5：一次性用户验收

全部 Task、机器验证和独立 Review 完成后，创建一个汇总 User Gate。该 Gate 只请求验收整个 Kafka 基础 Loop；若需要 commit、merge 或 push，分别请求明确授权，不从“验收通过”推导 Git 权限。
