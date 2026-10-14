# dmsvr Proto 按领域兼容拆分提案

> 日期：2026-08-25
>
> Goal：`ming-iot-framework-v1`
>
> 状态：后继顺序和兼容边界已获用户批准；尚未激活，不创建实现 Todo，不构成代码修改授权
>
> 推荐执行配置：实施使用 `gpt-5.6-sol`、`reasoning=high`；独立兼容性 Review 可使用 `xhigh`

## 1. 结论

在 Things 最小服务启动基线和本地 Share 模块迁移均完成后，将
`ming-iot/backend/things/service/dmsvr/proto/dm.proto` 从 2713 行巨型契约文件拆成一个兼容聚合
入口、一个严格受限的共享叶类型文件和 10 个 service 领域文件。

本次只改变 Proto 定义的物理归属，不改变 protobuf/gRPC 语义、Go package、服务运行单元或业务实现。

## 2. 参考依据

- [联犀工程组织说明（本地归档）](../references/unitedrhino-tech-arch-02.md)：monorepo、一个 RPC 运行单元内按
  业务域拆分多个 Proto、定义先行和生成校验；
- 历史参考：
  `archives/snapshots/2026-08-23/ming-iot/docs/superpowers/specs/2026-07-23-core-goctl-migration-design.md`；
- 当前契约：`ming-iot/backend/things/service/dmsvr/proto/dm.proto` 与 `schemaInfo.proto`；
- 当前生成工具基础：`backend/tools/internal/protocompat`、`rpcgen` 和固定 protobuf 工具链。

历史 Core 设计只提供 descriptor、manifest、临时生成和事务同步原则，不直接成为 Things 实施规范。

## 3. 已确认基线

2026-08-25 当前 `dm.proto`：

- 2713 行；
- `package dm`；
- `option go_package = "pb/dm"`；
- 10 个 service；
- 170 个 RPC；
- 261 个顶层 message；
- 现有 client/server 目录已经按 10 个 service 分组；
- 除生成物和开发说明外，未发现业务代码直接依赖 `File_dm_proto`。

## 4. 目标结构

```text
service/dmsvr/proto/
├── dm.proto                 # 兼容聚合入口，只做 public import
├── dmShare.proto            # 跨领域叶类型，不包含 service
├── schemaInfo.proto         # 现有物模型类型，暂不改变业务语义
├── deviceManage.proto       # DeviceManage
├── productManage.proto      # ProductManage
├── schemaManage.proto       # SchemaManage
├── protocolManage.proto     # ProtocolManage
├── deviceGroup.proto        # DeviceGroup
├── remoteConfig.proto       # RemoteConfig
├── deviceMsg.proto          # DeviceMsg
├── deviceInteract.proto     # DeviceInteract
├── otaManage.proto          # OtaManage
└── userDevice.proto         # userDevice
```

正式规格必须根据旧 descriptor 和 message 引用图给出精确归属表。规则如下：

- 一个 service 初始归属一个领域 Proto；
- 业务实体归权威领域文件，其他领域显式 import；
- 只有无明确业务归属、被多个领域复用的叶类型进入 `dmShare.proto`；
- `dmShare.proto` 不得包含 service，也不得退化成新的巨型文件；
- import DAG 必须无环；
- `dm.proto` 是导航和兼容聚合入口，不是单文件代码生成入口。

## 5. Compatibility Contract

必须保持：

1. protobuf package `dm`；
2. Go package `pb/dm`；
3. 10 个 service、170 个 RPC、261 个顶层 message；
4. service 和 method 的完整名称及 `/dm.<Service>/<Method>` 路径；
5. input/output 类型和 unary/streaming 标志；
6. message、enum、oneof、map、reserved 和字段名称、编号、类型、label、JSON name；
7. 现有 `pb/dm` Go import path；
8. client、server、direct 调用接口与运行时注册集合；
9. 对外 HTTP API 和业务错误语义。

允许变化：

- 定义所在的 Proto 文件名；
- descriptor 的文件归属；
- PB/GRPC 生成文件的物理拆分；
- generated source 注释中的源文件名；
- 不属于 protobuf/gRPC/Go 公共契约的生成顺序细节。

## 6. 生成与校验闭环

1. 用固定 protobuf 工具从旧 `dm.proto` 生成 descriptor 语义基线；
2. 使用显式 manifest 列出 aggregate、shared、schema 和 10 个 service Proto；
3. manifest 校验 package、go_package、service 唯一性、import DAG 和运行时注册集合；
4. 在临时目录一次性生成 PB、gRPC、client、server 和 direct 候选产物；
5. 复用并按需小幅泛化 `protocompat`，比较新旧语义；
6. 候选树中完成编译、测试、注册完整性和启动基线回归；
7. 全部通过后再事务化同步到业务树，禁止逐文件留下半生成状态；
8. 连续生成两次，第二次必须 Git 零变化。

正式规格必须先审计现有 `rpcgen` 对 SaaS/Core 路径的硬编码，再决定最小泛化边界；不得简单复制一套几乎
相同的生成器，也不得为了复用工具而把本次范围扩大成全仓生成平台重构。

## 7. 建议 write scopes

```text
backend/things/service/dmsvr/proto/**
backend/things/service/dmsvr/pb/dm/**
backend/things/service/dmsvr/client/**
backend/things/service/dmsvr/internal/server/**
backend/things/service/dmsvr/dmdirect/**
backend/things/service/dmsvr/etc/**             # 仅运行时注册/生成清单需要时
backend/tools/cmd/**                             # 仅获批的生成/兼容命令
backend/tools/internal/protocompat/**
backend/tools/internal/rpcgen/**                 # 仅获批的最小泛化
backend/tools/scripts/**                         # 仅固定生成入口
backend/tools/README.md
```

正式 Todo 必须把这些建议范围收敛到规格确认的精确文件；不得预先授权整个 `backend/things/**`。

## 8. Acceptance

1. 新旧 descriptor 语义比较零不允许差异；
2. 10 个 service、170 个 RPC、261 个顶层 message 和运行时注册集合保持一致；
3. 所有 Proto 使用同一 `package dm` 与 `go_package = "pb/dm"`；
4. import DAG 无环，`dmShare.proto` 只含审核通过的共享叶类型；
5. 现有 client、server、direct 调用方无业务源代码兼容性修改，生成文件物理变化除外；
6. dmsvr、apisvr、dgsvr 和相关工具测试通过；
7. Things 最小服务启动基线拆分前后结果一致；
8. 生成失败不改变业务树，生成成功后第二次运行零 diff；
9. 开发文档不再指导开发者直接对巨型 `dm.proto` 在业务树执行 goctl；
10. Evidence 保存旧基线摘要、领域归属表、descriptor 比较、注册集合、生成幂等和启动回归结果。

## 9. Non-goals

- 不拆分 dmsvr 为多个进程或 Go module；
- 不修改 service、method、message、字段编号、业务语义或数据库逻辑；
- 不重构 `schemaInfo.proto` 的物模型定义；
- 不实施模块级物模型、Kafka、Reducer 或 Scene；
- 不顺带重命名 `userDevice` 等既有公开 service；
- 不直接在 live 业务目录运行未经项目 wrapper 约束的 goctl/protoc。

## 10. 主要风险

- 领域 message 相互引用可能形成 import cycle，必须先建立完整引用图再移动定义；
- goctl 对多个 Proto 的 client/server/注册文件处理可能覆盖已有产物；
- 文件归属变化会改变 reflection descriptor 的文件结构，必须与真正承诺的 wire/Go contract 区分；
- 生成工具泛化过度会把一次契约拆分扩大成基础设施重写；
- 如果启动基线只验证进程存活而不验证 ready，无法证明 direct/grpc 注册仍完整。

## 11. 激活条件

只有同时满足以下条件，Planner 才能创建 dmsvr Proto 拆分 Todo：

1. Things 最小服务启动基线完成并 Review 通过；
2. 本地 Share 迁移完成，旧 Share import path 已归零并通过启动回归；
3. 用户批准正式规格中的 message 归属表、manifest 和生成工具最小泛化范围；
4. 实施 Todo 使用 `gpt-5.6-sol/high`，独立兼容性 Review 可使用 `xhigh`；
5. commit、merge、push 继续分别等待用户明确授权。
