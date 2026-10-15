# 首个真实 IoT 交付 Loop 排序提案

> 日期：2026-08-23
>
> Goal：`ming-iot-framework-v1`
>
> 权威基线：`iot-doc@287d41a7ca2f5d444ceb34264fbe1ac515c42836`
>
> 状态：首个 Loop、真实依赖拓扑和 2026-08-25 后继顺序已获用户批准；尚未创建新的业务实现 Todo，
> 不构成代码修改授权

## 1. 结论

推荐顺序：

1. **Things 迁移后的可重复启动与依赖契约基线**；
2. **Kafka 基础契约与 SDK 骨架**；
3. **模块级物模型与不可变发布快照基础**。

首个真实交付 Loop 选择候选 1。当前 `backend/things` 只证明 L0 可编译兼容，还没有证明三个服务能在
可重复环境中启动，也没有机器可读的真实依赖边界。先取得运行事实，后续 Kafka 和模块级物模型才能针对实际
role、配置、初始化和缓存边界设计，不再只靠文档推测。

总体架构 §18 的 Phase 0 是九组风险验证的集合，不能偷换成一个“小 Kafka Loop”；本提案只选择进入
Phase 0/Phase 1 之前最小、可验证的第一步。

## 2. 候选 1：Things 可重复启动与依赖契约基线

### 2.1 Objective

为 `apisvr`、`dmsvr`、`dgsvr` 建立可重复的非生产启动验证，输出机器可读的
“role—配置—外部依赖—就绪/失败分类”契约。它验证迁移后代码的实际启动边界，不改业务语义。

这里的“最小”只表示**最小完整运行拓扑**，不表示省略强依赖或只让进程存活。成功路径必须使用真实的
PostgreSQL、TDengine、Redis、etcd、NATS、EMQX，以及 Things 启动所必需的 SaaS RPC 角色；不得用
PostgreSQL 兼任 TSDB，不得用 mock 代替成功路径中的 TDengine 或 EMQX。

### 2.2 Dependencies

- Phase B checkpoint、Phase C 旧 Graph 退役和对应门禁先完成，避免与控制面切换争用范围；
- 当前 L0 Things 编译基线继续通过；
- 使用隔离的 PostgreSQL 作为关系库、TDengine 作为时序库，并启用 Redis、etcd、NATS、EMQX；
- SaaS 只启动 Things 成功路径实际需要的 `sys.rpc`、`timedjob.rpc` 及其必要传递依赖，不启动无关业务角色；
- 参考启动顺序为“基础设施 → SaaS 必需角色 → `dmsvr` → `dgsvr` → `apisvr`”，最终顺序以代码调查
  和可重复验证结果为准；
- 上游官方 `things/` 仓库永久只读；本 Loop 只读取迁移后的 `ming-iot/backend/things`；
- 运行 Evidence 写入本地 `ming-iot-rd-docs/evidence/`，不得重新把研发过程文档写回
  `ming-iot/docs/`。

### 2.3 建议 write scopes

```text
backend/tools/cmd/thingsstartbaseline/**
backend/tools/internal/thingsstartbaseline/**
deploy/things-start-baseline/**
deploy/compose.things-start-baseline*.yml
backend/tools/README.md
```

默认不写 `backend/things/**`。如果验证证明必须增加健康探针或可测试注入点，先形成带精确文件的独立
后继 Todo，不把范围预先放大。

### 2.4 Acceptance

- 三个现有服务二进制均由固定命令构建，并在有限超时内得到明确启动结果；
- 最小完整拓扑成功时，PostgreSQL、TDengine、Redis、etcd、NATS、EMQX、`sys.rpc`、
  `timedjob.rpc` 和三个 Things role 均达到各自的可验证就绪条件；
- PostgreSQL 只承载关系数据，TDengine 只承载本基线覆盖的时序数据，不复用 PostgreSQL 的 TSDB
  兼容路径作为成功证据；
- 记录每个 role 的监听、就绪状态、依赖版本和非敏感配置键；进程存活不能单独作为 ready 证据；
- 缺少任一关键依赖时，验证器返回稳定错误类别，不无限等待、不把进程存活误判为 ready；
- 固化 `dmsvr` 的 PostgreSQL、TDengine、Redis、etcd、NATS、SaaS RPC/TimedJob 实际依赖，
  以及 `apisvr`、`dgsvr` 对 RPC、Redis、NATS、EMQX 的真实启动依赖；
- EMQX 成功路径至少验证 `dgsvr` 的连接建立与可诊断失败，不要求发送真实设备业务消息；
- 所有测试使用隔离数据，不调用产品、设备、物模型等业务写 API；
- 产出可重复命令、Evidence 摘要、失败分类表和清理证明；
- 冷启动和第二次缓存启动都记录耗时，但本 Loop 不设生产 SLO。

### 2.5 Non-goals

- 不把 NATS 改成 Kafka；
- 不重构现有 TDengine 业务写链路，不实现 ModelRelease、模块超级表、Reducer 或 Scene；
- 不执行 Phase 0 的容量、72 小时 retention、broker 故障或 Raw→Canonical transaction 压测；
- 不作为生产部署模板或生产健康判定。

### 2.6 Risks

- 现有 `NewServiceContext` 强绑定数据库、缓存、事件总线和 SaaS RPC，最小完整拓扑仍然较重；
- `commonMinDocker.yaml` 的 PostgreSQL TSDB 兼容配置与目标 TDengine 成功路径不一致，基线必须提供
  独立配置，不能把旧配置直接当作验收事实；
- EMQX 的连接可能发生在 `dgsvr` 后初始化阶段，必须确认“进程已监听”和“设备网关链路已就绪”的边界；
- 多套本地/容器配置可能产生“能编译、不能组合启动”的真实缺口；
- 若验证器为追求全绿而 mock 掉成功路径依赖，就失去发现迁移耦合的价值；
- `backend/tools` 是共享目录，必须使用独立 worktree、精确 Todo scope 和局部测试。

### 2.7 为什么现在做

`backend/things/README.md` 明确当前只是 L0 编译兼容基线。服务启动证据会给后两个候选留下三类事实：

1. 实际服务依赖图；
2. PostgreSQL、TDengine、Redis、etcd、NATS、EMQX 和 SaaS RPC 的真实初始化/调用边界；
3. 现有 schema/cache 在启动时的装配位置。

## 3. 候选 2：Kafka 基础契约与 SDK 骨架

### 3.1 Objective

建立独立于 NATS 的最小 Kafka Data Plane 契约和 SDK 骨架：Raw topic/key/header、`rawRef`、
显式 topic catalog、producer/consumer 配置边界及本地集成验证。

### 3.2 Dependencies

- 候选 1 的启动/依赖事实作为推荐前置 Evidence；
- 冻结 Go Kafka client 和本地/生产 profile 的版本兼容策略；
- 当前单 broker 环境只能作为本地验证，不能冒充 RF=3 生产基线。

### 3.3 建议 write scopes

```text
backend/share/contracts/kafka/**
backend/protocol/sdk/**
backend/protocol/runtime/**
backend/go.work
backend/go.work.sum
deploy/kafka/**
deploy/compose.kafka-*.yml
backend/tools/cmd/kafkacontract/**
backend/tools/internal/kafkacontract/**
```

### 3.4 Acceptance

- Raw 契约固定为 topic、设备 identity key、必需 `message-kind` header、原始 payload 和
  `rawRef`，不引入业务 Raw Envelope；
- catalog 拒绝设备级 topic、自动建 topic 和无界动态 topic；
- 本地单 broker profile 与 RF=3/minISR=2 生产 profile 明确分离；
- 最小集成测试验证 key/header/value 保真、`read_committed` 配置和可诊断失败；
- 旧 NATS subject、全局 EventBus、`nats.go` 不被包装成兼容层。

### 3.5 Non-goals

- 不改造 Things 的 NATS 调用点；
- 不实现 Access Adapter、ProtocolApplication、Raw→Canonical transaction 或 Scene；
- 不宣称完成总体架构 Phase 0 的性能与故障退出条件。

### 3.6 Risks 与不先做原因

Kafka 专项明确要求 greenfield 重建，而不是替换 client。没有候选 1 的运行事实时，SDK 容易脱离实际 role
和控制面边界；从旧 subject 直接动手还会把 NATS 的偶然形状带入新平台。

## 4. 候选 3：模块级物模型与不可变发布快照基础

### 4.1 Objective

只建立模块化 ModelRelease 控制面基础：`SchemaBundle`/路由契约、模块定义、产品草稿和不可变发布
快照；不接入 TDengine 模块写入或设备上报主链。

### 4.2 Dependencies

- 候选 1 的数据库、缓存和服务装配事实；
- 明确 proto/API 生成流程及 relation DB migration 策略；
- 对迁移后 `backend/things` 的精确修改范围单独授权。

### 4.3 建议 write scopes

```text
backend/things/share/domain/schema/**
backend/things/service/dmsvr/internal/repo/relationDB/**
backend/things/service/dmsvr/internal/logic/productmanage/**
backend/things/service/dmsvr/internal/logic/schemamanage/**
backend/things/service/dmsvr/proto/**
backend/things/service/dmsvr/pb/dm/**
backend/things/service/apisvr/http/things/product/**
backend/things/service/apisvr/internal/handler/things/product/**
backend/things/service/apisvr/internal/logic/things/product/**
```

### 4.4 Acceptance

- `SchemaBundle` 对外保持扁平 identifier，内部 route 明确
  `identifier → moduleCode`；
- 发布生成不可变 `DmProductRelease` 与字段快照，拒绝重复 identifier 和非法类型；
- 模块 schema 首批只接受冻结的标量集合，显式拒绝 `struct/array`；
- legacy `dm_schema_info` 保留兼容位置，本 Loop 不切换上报和查询主链；
- relation DB 组装、快照不可变性和路由唯一性均有单元测试。

### 4.5 Non-goals

- 不创建 TDengine module STABLE/子表；
- 不改设备上报、latest/history、缓存切换或 eKuiper；
- 不把模块 CRUD 扩张为完整管理端交付。

### 4.6 Risks 与不先做原因

该候选横跨控制面、proto、API、缓存和存储演进，最容易从“基础”膨胀为大重构。没有候选 1 的真实装配
证据时，新快照模型会与运行中的旧 schema/cache 边界脱节。

## 5. 推荐的下一决策点

Phase B exact checkpoint、Phase C 旧 Graph 当前树退役，以及候选 1 的 objective、建议 write scopes 和
非生产真实依赖边界均已完成或获得用户批准。候选 1 现在是下一项可激活的交付 Loop，但本提案本身仍不
创建实现 Todo，也不授权修改产品代码。

用户明确开始候选 1 后，Planner 再依据当前代码事实收敛精确 write scope，并拆成一个最小可失败测试、
一个实现 Task 和一个独立 Review Task。

规划 artifact：`planner/ming-iot-framework-v1/first-real-iot-loop/v1`。

## 6. 2026-08-25 后继顺序修订

旧 Graph 当前树已经完成退役并合并到本地 `ming-iot/master`。用户随后批准在首个真实交付序列中加入
两项必须实施、但尚未激活的后继 Proposal：

1. [Ming IoT 本地 Share 模块迁移提案](2026-08-25-ming-iot-local-share-migration.md)；
2. [dmsvr Proto 按领域兼容拆分提案](2026-08-25-dmsvr-proto-domain-split.md)。

当前批准顺序调整为：

```text
旧 Graph 当前树退役（已完成）
  → Things 可重复启动与依赖契约基线
  → 本地 Share 模块迁移
  → dmsvr Proto 按领域兼容拆分
  → Kafka 基础契约与 SDK 骨架
  → 模块级物模型与不可变发布快照基础
```

选择该顺序是为了先取得服务启动事实，再用同一基线分别定位 Share module identity 切换和 Proto 物理拆分
可能引入的回归。两份后继 Proposal 只登记边界、依赖和验收，不提前创建 Todo 或授权业务代码修改。
