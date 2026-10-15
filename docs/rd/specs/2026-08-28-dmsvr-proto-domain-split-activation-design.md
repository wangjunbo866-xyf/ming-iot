# dmsvr Proto 按领域兼容拆分激活规格

> 日期：2026-08-28
>
> Goal：`ming-iot-framework-v1`
>
> 状态：设计已获用户逐节批准；实施 Todo 与最终 User Gate 尚未建立
>
> 前置条件：Things 可重复启动基线、本地 Share 迁移已经完成
>
> 推荐执行配置：实现使用 `gpt-5.6-sol/high`；独立兼容性 Review 可使用 `xhigh`

## 1. 结论

在不改变 dmsvr 运行单元、protobuf wire contract、gRPC contract、Go import path、客户端接口、服务端接口和当前运行时注册集合的前提下，把
`backend/things/service/dmsvr/proto/dm.proto` 从单一巨型文件拆成显式的领域 Proto 集合。

本次采用以下结构：

```text
service/dmsvr/proto/
├── dm.proto                 # 聚合入口，仅 public import，不参与代码生成
├── dmShare.proto            # 8 个跨领域叶类型
├── protocolModel.proto      # 3 个协议模型，打断 Product/Protocol 循环依赖
├── schemaInfo.proto         # 原有 6 个物模型消息，保持不变
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

所有文件继续使用：

```proto
package dm;
option go_package = "pb/dm";
```

`dm.proto` 只承担导航和兼容聚合职责。生成工具必须从 manifest 的显式输入清单生成，禁止把聚合文件再次当成单文件生成入口。

## 2. 权威来源与冲突规则

本规格的依据按优先级排序如下：

1. 本规格及用户在当前设计会话中批准的边界；
2. 当前 `dm.proto`、`schemaInfo.proto`、生成产物、client/server/direct API 和运行时注册事实；
3. `references/unitedrhino-tech-arch-02.md` 的工程组织原则；
4. 历史 Core goctl 迁移设计中的 descriptor、manifest、临时生成和幂等原则。

冲突时，本规格的兼容合同优先于物理文件布局偏好；当前代码事实优先于历史文档中的目录、数量和模块假设。历史设计不得扩大本次实施范围。

## 3. 当前基线

拆分前必须重新捕获并冻结下列基线：

| 项目 | 基线 |
|---|---:|
| `dm.proto` 行数 | 2713 |
| protobuf package | `dm` |
| Go package | `pb/dm` |
| service 数量 | 10 |
| RPC 数量 | 170 |
| `dm.proto` 顶层 message 数量 | 261 |
| `schemaInfo.proto` 顶层 message 数量 | 6 |
| client package 数量 | 10 |
| client 方法数量 | 170 |
| client 构造函数数量 | 20 |
| `NewDirect<Service>` 数量 | 10 |
| direct RPC 方法数量 | 170 |
| Streaming RPC 数量 | 0 |
| 手工维护的 `dmdirect.New<Service>` 数量 | 10 |
| 每个 client 包的公共 alias 数量 | 262 |
| 10 个 client 包的公共 alias 总数 | 2620 |
| 当前运行时注册 service 数量 | 9 |

10 个 service 的 RPC 数量必须保持：

| Service | RPC |
|---|---:|
| `DeviceManage` | 40 |
| `ProductManage` | 29 |
| `SchemaManage` | 7 |
| `ProtocolManage` | 21 |
| `DeviceGroup` | 9 |
| `RemoteConfig` | 4 |
| `DeviceMsg` | 15 |
| `DeviceInteract` | 12 |
| `OtaManage` | 18 |
| `userDevice` | 15 |

当前 `dmdirect.Run` 只注册 9 个 service，`SchemaManage` 未注册。本次必须原样保留这个运行时事实，禁止顺带修复。是否注册 `SchemaManage` 由后续独立 Bug Todo 决定。

### 3.1 Direct 生成边界基线

项目锁定的联犀 `goctl` 来源由 `backend/tools/goctl.lock` 唯一确定：仓库为
`https://gitee.com/unitedrhino/go-zero.git`，commit 为
`6579a20bfd96919a80af5a9cb4e12f03598d4bb3`，版本标识为 `1.9.2-1`。

源码与提交历史确认以下边界：

- 联犀 `goctl` 在提交 `eacd96985a76f60019b63d9830394ddc1b9f1eb2` 中加入 unary RPC 单机直连生成；
- 当前锁定提交继续生成 `direct<Service>`、`NewDirect<Service>` 和每个 RPC 的 direct 方法，并补充 Streaming Direct 支持；
- Things 在后续提交 `128dbf058d2871d73dc1faa92475600e102cfbf3` 中手工加入 `dmdirect` 装配包；
- 联犀 `goctl` 没有 `dmdirect` 目录、模板或生成阶段；`dmdirect/**` 没有 generated header，也不属于可再生输出。

因此，`client/<service>` 中的 direct adapter 是生成物；`dmdirect` 是项目手工维护的运行时装配层。两者不得在本次实施中混为同一生成边界。

## 4. 领域归属与 import DAG

### 4.1 归属原则

- 每个 service 只出现一次，并与同名领域文件绑定。
- 业务实体归属权威领域文件，其他文件显式 import。
- 只有无明确业务归属、被多个领域使用的叶类型进入 `dmShare.proto`。
- `protocolModel.proto` 只承载协议配置模型，用于打断 Product/Protocol 的引用环。
- `schemaInfo.proto` 保持现有 6 个消息及语义，不并入本次 261 个消息归属计数。
- 禁止 glob 推断输入，禁止根据文件名隐式推断 service。

### 4.2 import DAG

允许的领域依赖固定为：

```text
deviceGroup      -> dmShare
deviceInteract   -> dmShare
deviceManage     -> dmShare
deviceMsg        -> deviceManage, dmShare
otaManage        -> dmShare
productManage    -> dmShare, protocolModel
protocolManage   -> deviceManage, productManage, dmShare, protocolModel
remoteConfig     -> dmShare
schemaManage     -> productManage, dmShare
userDevice       -> deviceManage, dmShare
```

实现时必须通过 descriptor 引用图验证该图无环；任何新增边都必须先回到规格评审，不得临时把类型继续塞入 `dmShare.proto` 消除循环。

## 5. 261 个 message 的冻结归属表

每个消息必须且只能出现一次。括号内为数量。

### 5.1 `dmShare.proto`（8）

`CompareString`、`DeviceCore`、`Empty`、`FileCore`、`ImportResp`、`PageInfo`、`WithID`、`WithIDCode`。

### 5.2 `protocolModel.proto`（3）

`ProtocolConfigField`、`ProtocolConfigInfo`、`ProtocolInfo`。

### 5.3 `deviceManage.proto`（60）

`Compare`、`CompareInt64`、`DeviceBindTokenInfo`、`DeviceBindTokenReadReq`、`DeviceCountInfo`、`DeviceCountReq`、`DeviceCountResp`、`DeviceError`、`DeviceGatewayBindDevice`、`DeviceGatewayIndexReq`、`DeviceGatewayIndexResp`、`DeviceGatewayMultiCreateReq`、`DeviceGatewayMultiSaveReq`、`DeviceGatewaySign`、`DeviceGroupMultiSaveReq`、`DeviceInfo`、`DeviceInfoBindReq`、`DeviceInfoCanBindReq`、`DeviceInfoCount`、`DeviceInfoCountReq`、`DeviceInfoDeleteReq`、`DeviceInfoIndexReq`、`DeviceInfoIndexResp`、`DeviceInfoMultiBindReq`、`DeviceInfoMultiBindResp`、`DeviceInfoMultiUpdateReq`、`DeviceInfoReadReq`、`DeviceInfoUnbindReq`、`DeviceModuleVersion`、`DeviceModuleVersionIndexReq`、`DeviceModuleVersionIndexResp`、`DeviceModuleVersionReadReq`、`DeviceMoveReq`、`DeviceOtaUpgradeReq`、`DeviceOtaUpgradeResp`、`DeviceProfile`、`DeviceProfileIndexReq`、`DeviceProfileIndexResp`、`DeviceProfileReadReq`、`DeviceResetReq`、`DeviceSchema`、`DeviceSchemaIndexReq`、`DeviceSchemaIndexResp`、`DeviceSchemaMultiCreateReq`、`DeviceSchemaMultiDeleteReq`、`DeviceSchemaTslReadReq`、`DeviceSchemaTslReadResp`、`DeviceTransferReq`、`DeviceTypeCountReq`、`DeviceTypeCountResp`、`IDPath`、`IDPathWithUpdate`、`IDsInfo`、`OtaFile`、`OtaUpgradeData`、`Point`、`RootCheckReq`、`TimeRange`、`deviceOnlineMultiFix`、`deviceOnlineMultiFixReq`。

### 5.4 `productManage.proto`（35）

`CustomTopic`、`DevInit`、`ProductCategory`、`ProductCategoryExportReq`、`ProductCategoryExportResp`、`ProductCategoryImportReq`、`ProductCategoryIndexReq`、`ProductCategoryIndexResp`、`ProductCategorySchemaIndexReq`、`ProductCategorySchemaIndexResp`、`ProductCategorySchemaMultiSaveReq`、`ProductConfig`、`ProductCustom`、`ProductCustomReadReq`、`ProductCustomUi`、`ProductID`、`ProductInfo`、`ProductInfoExportReq`、`ProductInfoExportResp`、`ProductInfoImportReq`、`ProductInfoIndexReq`、`ProductInfoIndexResp`、`ProductInfoReadReq`、`ProductInitReq`、`ProductSchemaCreateReq`、`ProductSchemaDeleteReq`、`ProductSchemaIndexReq`、`ProductSchemaIndexResp`、`ProductSchemaInfo`、`ProductSchemaMultiCreateReq`、`ProductSchemaTslImportReq`、`ProductSchemaTslReadReq`、`ProductSchemaTslReadResp`、`ProductSchemaUpdateReq`、`WithIDChildren`。

### 5.5 `schemaManage.proto`（8）

`CommonSchemaCreateReq`、`CommonSchemaExportReq`、`CommonSchemaExportResp`、`CommonSchemaImportReq`、`CommonSchemaIndexReq`、`CommonSchemaIndexResp`、`CommonSchemaInfo`、`CommonSchemaUpdateReq`。

### 5.6 `protocolManage.proto`（17）

`ProtocolInfoIndexReq`、`ProtocolInfoIndexResp`、`ProtocolScript`、`ProtocolScriptDebugReq`、`ProtocolScriptDebugResp`、`ProtocolScriptDevice`、`ProtocolScriptDeviceIndexReq`、`ProtocolScriptDeviceIndexResp`、`ProtocolScriptExportReq`、`ProtocolScriptExportResp`、`ProtocolScriptImportReq`、`ProtocolScriptIndexReq`、`ProtocolScriptIndexResp`、`ProtocolService`、`ProtocolServiceIndexReq`、`ProtocolServiceIndexResp`、`PublishMsg`。

### 5.7 `deviceGroup.proto`（10）

`GroupCore`、`GroupDeviceMultiDeleteReq`、`GroupDeviceMultiSaveReq`、`GroupInfo`、`GroupInfoCreateReq`、`GroupInfoIndexReq`、`GroupInfoIndexResp`、`GroupInfoMultiCreateReq`、`GroupInfoReadReq`、`GroupInfoUpdateReq`。

### 5.8 `remoteConfig.proto`（7）

`ProductRemoteConfig`、`RemoteConfigCreateReq`、`RemoteConfigIndexReq`、`RemoteConfigIndexResp`、`RemoteConfigLastReadReq`、`RemoteConfigLastReadResp`、`RemoteConfigPushAllReq`。

### 5.9 `deviceMsg.proto`（41）

`AbnormalLogIndexReq`、`AbnormalLogIndexResp`、`AbnormalLogInfo`、`DeviceData`、`EventLogIndexReq`、`EventLogIndexResp`、`EventLogInfo`、`GatewayCanBindIndexReq`、`GatewayCanBindIndexResp`、`HubLogIndexReq`、`HubLogIndexResp`、`HubLogInfo`、`PropertyAgg`、`PropertyAggByDevice`、`PropertyAggCore`、`PropertyAggIndexReq`、`PropertyLatestAggIndexReq`、`PropertyLatestAggIndexResp`、`PropertyLatestAggResp`、`PropertyLatestAggRespDetail`、`PropertyLatestIndex2Req`、`PropertyLatestIndexReq`、`PropertyLogAggByDeviceIndexReq`、`PropertyLogAggIndexResp`、`PropertyLogAggResp`、`PropertyLogAggRespDataDetail`、`PropertyLogAggRespDetail`、`PropertyLogIndexReq`、`PropertyLogIndexResp`、`PropertyLogInfo`、`SdkLogIndexReq`、`SdkLogIndexResp`、`SdkLogInfo`、`SendLogIndexReq`、`SendLogIndexResp`、`SendLogInfo`、`ShadowIndex`、`ShadowIndexResp`、`StatusLogIndexReq`、`StatusLogIndexResp`、`StatusLogInfo`。

### 5.10 `deviceInteract.proto`（22）

`ActionRespReq`、`ActionSendReq`、`ActionSendResp`、`EdgeSendReq`、`EdgeSendResp`、`GatewayGetFoundReq`、`GatewayNotifyBindSendReq`、`PropertyControlMultiSendReq`、`PropertyControlMultiSendResp`、`PropertyControlSendMsg`、`PropertyControlSendReq`、`PropertyControlSendResp`、`PropertyGetReportMultiSendReq`、`PropertyGetReportMultiSendResp`、`PropertyGetReportSendMsg`、`PropertyGetReportSendReq`、`PropertyGetReportSendResp`、`RespReadReq`、`SendMsgReq`、`SendMsgResp`、`SendOption`、`WithProfile`。

### 5.11 `otaManage.proto`（36）

`Firmware`、`FirmwareFile`、`FirmwareInfo`、`FirmwareInfoDeleteReq`、`FirmwareInfoDeleteResp`、`FirmwareInfoIndexReq`、`FirmwareInfoIndexResp`、`FirmwareInfoReadReq`、`FirmwareInfoReadResp`、`FirmwareResp`、`OtaFirmwareDeviceCancelReq`、`OtaFirmwareDeviceConfirmReq`、`OtaFirmwareDeviceIndexReq`、`OtaFirmwareDeviceIndexResp`、`OtaFirmwareDeviceInfo`、`OtaFirmwareDeviceRetryReq`、`OtaFirmwareFile`、`OtaFirmwareFileIndexReq`、`OtaFirmwareFileIndexResp`、`OtaFirmwareFileInfo`、`OtaFirmwareFileReq`、`OtaFirmwareFileResp`、`OtaFirmwareInfo`、`OtaFirmwareInfoCreateReq`、`OtaFirmwareInfoIndexReq`、`OtaFirmwareInfoIndexResp`、`OtaFirmwareInfoUpdateReq`、`OtaFirmwareJobIndexReq`、`OtaFirmwareJobIndexResp`、`OtaFirmwareJobInfo`、`OtaJobByDeviceIndexReq`、`OtaJobDynamicInfo`、`OtaJobStaticInfo`、`OtaModuleInfo`、`OtaModuleInfoIndexReq`、`OtaModuleInfoIndexResp`。

### 5.12 `userDevice.proto`（14）

`DeviceShareInfo`、`SharePerm`、`UserDeviceCollectSave`、`UserDeviceShareIndexReq`、`UserDeviceShareIndexResp`、`UserDeviceShareInfo`、`UserDeviceShareMultiAcceptReq`、`UserDeviceShareMultiDeleteReq`、`UserDeviceShareMultiDeleteTokenReq`、`UserDeviceShareMultiGetTokenListResp`、`UserDeviceShareMultiInfo`、`UserDeviceShareMultiListItem`、`UserDeviceShareMultiToken`、`UserDeviceShareReadReq`。

归属计数必须满足：

```text
8 + 3 + 60 + 35 + 8 + 17 + 10 + 7 + 41 + 22 + 36 + 14 = 261
```

## 6. Compatibility Contract

### 6.1 Proto/gRPC 冻结项

必须保持：

- protobuf package 与 Go package；
- 10 个 service、170 个 RPC 及其完整名称；
- `/dm.<Service>/<Method>` 方法路径；
- request/response 类型和 unary/streaming 标志；
- 261 个现有顶层 message 的名称；
- message、enum、oneof、map、reserved、字段名称、字段号、字段类型、label、默认语义和 JSON name；
- `schemaInfo.proto` 的 6 个消息及语义；
- package 级 descriptor 的语义集合。

### 6.2 Go 公共 API 冻结项

必须保持：

- PB import path；
- 10 个 client interface、170 个 client 方法和 20 个构造函数，其中包括 10 个 `New<Service>` 与 10 个 `NewDirect<Service>`；
- 170 个生成的 direct RPC 方法继续直接调用传入的 protobuf Server 接口，方法签名与返回语义不变；
- 每个 client package 的 262 个公共 alias，总计 2620 个 alias；
- 10 个手工维护的 `dmdirect.New<Service>(runSvr bool)` facade 及其返回类型；
- `dmdirect.GetSvcCtx` 的共享 `ServiceContext` 单例、`startup.Init` 调用和 `RunServer` 可选代理语义；
- server/direct 的其余公开签名；
- 现有业务调用方不需要修改源码。

本次不得通过修改 `dmdirect/**` 适配生成结果。候选输出若不能在保持该手工装配层零修改的条件下编译，必须判定为兼容性失败。

### 6.3 运行时冻结项

必须精确保持当前 9 个 service 注册集合。`SchemaManage` 继续缺席，既不得自动补齐，也不得因生成器校验“全部 service 必须注册”而失败。

### 6.4 允许变化

允许变化仅包括：

- Proto 定义所在的物理文件；
- 生成的 Go 文件名和文件数量；
- generated 注释中的源 Proto 名；
- `File_*_proto` descriptor 变量名；
- descriptor 的文件归属，只要 package 级语义集合不变。

任何需要修改业务代码才能编译通过的差异，默认视为兼容性失败，而不是“拆分后的正常适配”。

## 7. Manifest v2

### 7.1 顶层结构

```yaml
version: 2
runtime: dmsvr
modulePath: gitee.com/ming-iot/things
runtimeDir: service/dmsvr
protoDir: service/dmsvr/proto
pbDir: service/dmsvr/pb/dm
package: dm
goPackage: pb/dm
pbSelector: dm
aggregate: dm.proto
supportingProtos:
  - dmShare.proto
  - protocolModel.proto
  - schemaInfo.proto
registry:
  path: service/dmsvr/dmdirect/dmdirect.go
  selector: dm
services:
  - proto: deviceManage.proto
    service: DeviceManage
    packageDir: devicemanage
    clientFile: deviceManage.go
    serverFile: deviceManageServer.go
    registration: required
  - proto: productManage.proto
    service: ProductManage
    packageDir: productmanage
    clientFile: productManage.go
    serverFile: productManageServer.go
    registration: required
  - proto: schemaManage.proto
    service: SchemaManage
    packageDir: schemamanage
    clientFile: schemaManage.go
    serverFile: schemaManageServer.go
    registration: preserve_absent
  - proto: protocolManage.proto
    service: ProtocolManage
    packageDir: protocolmanage
    clientFile: protocolManage.go
    serverFile: protocolManageServer.go
    registration: required
  - proto: deviceGroup.proto
    service: DeviceGroup
    packageDir: devicegroup
    clientFile: deviceGroup.go
    serverFile: deviceGroupServer.go
    registration: required
  - proto: remoteConfig.proto
    service: RemoteConfig
    packageDir: remoteconfig
    clientFile: remoteConfig.go
    serverFile: remoteConfigServer.go
    registration: required
  - proto: deviceMsg.proto
    service: DeviceMsg
    packageDir: devicemsg
    clientFile: deviceMsg.go
    serverFile: deviceMsgServer.go
    registration: required
  - proto: deviceInteract.proto
    service: DeviceInteract
    packageDir: deviceinteract
    clientFile: deviceInteract.go
    serverFile: deviceInteractServer.go
    registration: required
  - proto: otaManage.proto
    service: OtaManage
    packageDir: otamanage
    clientFile: otaManage.go
    serverFile: otaManageServer.go
    registration: required
  - proto: userDevice.proto
    service: userDevice
    packageDir: userdevice
    clientFile: userDevice.go
    serverFile: userDeviceServer.go
    registration: required
```

Manifest v2 的字段名固定使用本节展示的 camelCase；不得在实施中临时改名。规格中的 manifest 不授权工具通过目录扫描补全遗漏项。

### 7.2 Manifest 校验

校验器必须拒绝：

- aggregate 或 supporting Proto 被重复当成 service 输入；
- aggregate 含有 message、enum 或 service 定义，或者没有 public import 全部 supporting/service Proto；
- service、Proto、packageDir、clientFile 或 serverFile 重复；
- 文件不存在；
- package/go_package 不一致；
- import DAG 有环；
- service 缺失或多重定义；
- `registration` 不是 `required` 或 `preserve_absent`；
- required service 未注册；
- preserve_absent service 被擅自注册；
- 候选输出越过写入白名单。

## 8. 生成工具最小泛化

### 8.1 固定 `goctl` 与生成边界

`rpcgen` 必须只调用由 `backend/tools/goctl.lock` 可复现构建并校验成功的
`backend/bin/goctl`，不得解析或回退到 `PATH` 中的官方 `goctl`。执行前必须校验：

- repository、commit、版本标识和目标平台符合 lock；
- binary SHA-256 符合 lock；
- 生成命令和 Evidence 记录实际二进制路径与摘要；
- 二进制缺失或校验失败时 fail closed，不得临时改用其他 `goctl`。

联犀 `goctl` 只负责生成 client 中的 direct adapter，不负责生成 `dmdirect`。本次不扩展
联犀 fork 去生成 `dmdirect`，也不把 `dmdirect` 加入候选同步目录。

当前 170 个 RPC 均为 unary，锁定 fork 中依赖 `gitee.com/unitedrhino/share` 的 Streaming Direct
模板在本次不应进入受控产物。所有候选输出仍必须扫描旧 Share import path；发现
`gitee.com/unitedrhino/share` 时直接失败，不得依赖人工替换后继续。

`goctl` 可能在临时树产生 main、etc、startup 等辅助文件；这些文件不属于本次受控输出，必须被
allowlist 拒绝且不得同步到业务树。

### 8.2 `rpcgen`

现有工具中的 `syssvr`、`saas`、`sys`、固定 module path 和“全部 service 必须注册”假设必须改为 manifest/参数驱动：

- `modulePath`；
- `runtimeDir`；
- `pbSelector`；
- `supportingProtos`；
- 每个 service 的 registration 策略；
- registry 文件路径与 PB selector。

命令入口新增通用 `--module-root`。现有 `--saas` 作为兼容别名继续可用，SaaS v1 manifest 与现有输出必须保持通过。

registry 校验继续使用 AST，不使用文本替换。候选生成只能写入临时树；在兼容、编译和范围校验全部通过前，不得同步到业务树。

### 8.3 `clientcontract`

当前工具只识别 `sys.*` alias，必须改为从 manifest 的 `pbSelector` 捕获 alias。SaaS 仍识别 `sys`，Things 识别 `dm`。比较逻辑和输出格式不做无关重构。

### 8.4 保持不变的工具

- `protocompat` 继续比较 package 级 descriptor 语义，不把物理文件变化误报为 wire contract 变化；
- `saastxn` 继续承担现有事务同步职责，不重命名、不扩展成新的通用平台；
- 固定 protobuf/goctl 工具链版本继续沿用。

### 8.5 受控生成输出

本次生成器可以控制：

- `pb/dm` 全部生成物；
- 10 个 client 文件；
- 10 个 server 文件；
- client compatibility aliases。

本次生成器不得修改：

- logic；
- `dmdirect` 业务实现或注册代码；
- 运行时配置；
- repository、model、数据库或消息中间件代码；
- 其他服务的业务源码。

## 9. 实施顺序

1. 在独立 `ming-iot` worktree 中确认干净基线。
2. 捕获旧 descriptor、client 公共 API golden、数量基线、9 个运行时注册和 Git clean 状态。
3. 先以测试固化 manifest v2、SaaS v1 回归、动态 PB selector 和 registration 策略。
4. 按第 5 节归属表手工拆分 Proto，禁止让生成器猜测 message 归属。
5. 验证 import DAG、service 唯一性和聚合入口规则。
6. 在临时目录生成 PB/client/server 候选树。
7. 运行 descriptor、公共 API、注册集合、编译和写入范围验证。
8. 只有全部通过后，才原子同步获批的候选文件。
9. 立即再次生成，第二次必须零 diff。
10. 执行完整测试、真实冷启动、暖启动、Evidence 和独立 Review。
11. 全部完成后只创建一次总体 User Gate；commit、merge、push 继续分别等待明确授权。

任何阶段失败都必须保留原业务树，不得留下半生成结果。工具输出必须给出具体失败阶段、目标文件和稳定错误码，不得只返回“生成失败”。

## 10. 实施写入边界

允许的生产文件范围收敛为：

```text
backend/things/service/dmsvr/proto/dm.proto
backend/things/service/dmsvr/proto/dmShare.proto
backend/things/service/dmsvr/proto/protocolModel.proto
backend/things/service/dmsvr/proto/deviceManage.proto
backend/things/service/dmsvr/proto/productManage.proto
backend/things/service/dmsvr/proto/schemaManage.proto
backend/things/service/dmsvr/proto/protocolManage.proto
backend/things/service/dmsvr/proto/deviceGroup.proto
backend/things/service/dmsvr/proto/remoteConfig.proto
backend/things/service/dmsvr/proto/deviceMsg.proto
backend/things/service/dmsvr/proto/deviceInteract.proto
backend/things/service/dmsvr/proto/otaManage.proto
backend/things/service/dmsvr/proto/userDevice.proto
backend/things/service/dmsvr/proto/rpcgen.yaml
backend/things/service/dmsvr/pb/dm/**
backend/things/service/dmsvr/client/devicemanage/**
backend/things/service/dmsvr/client/productmanage/**
backend/things/service/dmsvr/client/schemamanage/**
backend/things/service/dmsvr/client/protocolmanage/**
backend/things/service/dmsvr/client/devicegroup/**
backend/things/service/dmsvr/client/remoteconfig/**
backend/things/service/dmsvr/client/devicemsg/**
backend/things/service/dmsvr/client/deviceinteract/**
backend/things/service/dmsvr/client/otamanage/**
backend/things/service/dmsvr/client/userdevice/**
backend/things/service/dmsvr/internal/server/devicemanage/**
backend/things/service/dmsvr/internal/server/productmanage/**
backend/things/service/dmsvr/internal/server/schemamanage/**
backend/things/service/dmsvr/internal/server/protocolmanage/**
backend/things/service/dmsvr/internal/server/devicegroup/**
backend/things/service/dmsvr/internal/server/remoteconfig/**
backend/things/service/dmsvr/internal/server/devicemsg/**
backend/things/service/dmsvr/internal/server/deviceinteract/**
backend/things/service/dmsvr/internal/server/otamanage/**
backend/things/service/dmsvr/internal/server/userdevice/**
backend/tools/cmd/rpcgen/**
backend/tools/internal/rpcgen/**
backend/tools/internal/clientcontract/**
backend/tools/README.md
```

测试 fixture、golden 和临时输出仅允许写入对应工具测试目录或系统临时目录。`schemaInfo.proto` 是只读兼容输入，不在生产写入范围内。`dmdirect/**` 与 `etc/**` 也不在写入范围内。

实施前必须把以上目录进一步展开成候选同步 allowlist；任何实际 diff 超出 allowlist 都必须 fail closed。

## 11. 验证矩阵

### 11.1 工具级

- `rpcgen` v1/v2 manifest 解析与错误路径测试；
- 锁定 `goctl` 路径、版本、commit 与 binary SHA-256 校验测试，并验证 PATH 中其他 `goctl` 不得被调用；
- SaaS 现有 manifest 与输出回归；
- `clientcontract` 对 `sys`、`dm` 和非法 selector 的测试；
- 10 个 `NewDirect<Service>`、170 个 direct 方法和 10 个 `dmdirect.New<Service>` facade 的合同测试；
- manifest service 唯一性、import DAG、aggregate、supporting Proto 和 registration 策略测试；
- 受控输出的旧 `gitee.com/unitedrhino/share` import 零命中测试；
- `dmdirect/**` 零修改测试；
- 候选输出 allowlist 和失败不污染业务树测试；
- 正常测试、race 和 vet。

### 11.2 契约级

- 新旧 descriptor 的 package 级语义零不允许差异；
- 10 service、170 RPC、261 message 与 `schemaInfo.proto` 6 message 计数；
- 10 client interface、170 方法、20 构造函数、2620 alias；
- 9 个运行时注册精确一致，`SchemaManage` 仍未注册；
- 第二次生成 Git 零 diff。

### 11.3 模块级

- Things 相关 `go test`；
- 涉及工具和核心包的 `go test -race`；
- `go vet`；
- dmsvr、apisvr、dgsvr 与相关构建目标通过；
- 业务调用方没有兼容性源码修改。

### 11.4 运行时回归

依次执行：

```text
make -C deploy things-baseline-fast
受保护的 cold up + verify
warm up + verify
```

运行时回归必须复用已批准的 Things 可重复启动基线，不得 reset、删除数据库或删除 volume，不得把“进程存活”当成 ready。

## 12. Evidence 与 Review

Evidence 至少包含：

- 源提交、worktree、工具链版本和 manifest 摘要；
- 实际 `goctl` 路径、锁定 commit、版本标识与 binary SHA-256；
- 拆分前后的 service/RPC/message/API/注册计数；
- 生成 direct adapter 与手工 `dmdirect` 装配层的边界检查结果；
- 完整归属表校验结果与 import DAG；
- descriptor 兼容报告；
- client/server/direct 公共 API 报告；
- 候选输出 allowlist 报告；
- 首次生成与第二次零 diff 结果；
- test、race、vet、build 结果；
- cold/warm 启动及依赖探针结果；
- 失败路径、修复记录和已脱敏日志索引；
- 最终 worktree 状态。

独立 Reviewer 必须读取本规格、manifest、关键 diff 和 Evidence，并给出 `PASS`、`BLOCK` 或 `INCONCLUSIVE`。没有正式 Review 结论不得把机器绿灯解释为完成。

## 13. Todo 与 Gate 策略

- 本规格批准后，Planner 只建立一个“dmsvr Proto 按领域兼容拆分”Agent Todo；Plan 中的内部 Task 不映射为多个 Agent Todo。
- 内部 checkpoint commit 可由 Agent 在隔离 worktree 创建，但不得因此逐 Task 打开 User Gate。
- 只有实现、验证、Evidence 和独立 Review 全部结束后，才打开一次总体 User Gate。
- commit、fast-forward merge 和 push 是三个独立授权动作，不由总体验收 Gate 自动包含。
- `SchemaManage` 未注册问题在本 Todo 关闭后登记为单独 Bug Todo，不阻塞本次兼容拆分，也不得混入本次 diff。

## 14. Non-goals

- 不把 dmsvr 拆成多个进程或 Go module；
- 不修改业务语义、数据库逻辑、Kafka/NATS/EMQX/TDengine 行为；
- 不修改 service、method、message 或字段；
- 不重构 `schemaInfo.proto`；
- 不修复 `SchemaManage` 注册缺失；
- 不重命名 `userDevice`；
- 不建设新的全仓代码生成平台；
- 不替换或 Fork LoopX；
- 不把设计批准解释为代码实施、commit、merge 或 push 授权。

## 15. 验收条件

只有同时满足以下条件，本实施 Todo 才能进入总体 User Gate：

1. 第 5 节 261 个 message 全部唯一归属且计数正确；
2. import DAG 无环，3 个 supporting Proto 的职责未膨胀；
3. 10 个 service、170 个 RPC 和全部字段语义保持；
4. Go 公共 API 和 9 个运行时注册精确保持；
5. 10 个 `NewDirect<Service>`、170 个 direct 方法和 10 个 `dmdirect.New<Service>` facade 保持，且 `dmdirect/**` 零修改；
6. 实际生成器是 lock 校验通过的联犀 `backend/bin/goctl`，受控输出不含旧 Share import；
7. SaaS v1 与 Things v2 生成路径同时通过；
8. 候选生成失败不污染业务树，成功后第二次生成零 diff；
9. test、race、vet、build、cold/warm 启动全部通过；
10. Evidence 完整、脱敏且可追溯；
11. 独立 Reviewer 给出 `PASS`；
12. 实际 diff 未越过获批 allowlist，且没有夹带 `SchemaManage` Bug 修复。

## 16. 用户已批准的设计决策

截至 2026-08-28，用户已逐节批准：

- 同一 dmsvr runtime、同一 `dm`/`pb/dm` package 的领域拆分结构；
- 261 个 message 的完整归属、import DAG 和兼容冻结边界；
- manifest v2 与 `rpcgen`/`clientcontract` 的最小泛化范围；
- 联犀 `goctl` 自动生成 direct client、Things 手工维护 `dmdirect` 装配层的职责边界；
- 强制使用 `goctl.lock` 锁定二进制、禁止 PATH 回退、禁止旧 Share import 和保持 `dmdirect/**` 零修改；
- 独立 worktree、候选生成、原子同步、零 diff、启动回归、Evidence、Review 和单一最终 User Gate 流程。

该批准只使本规格具备进入实施规划的资格。实施计划和后续 Agent Todo 仍需在本规格书面复核通过后建立。
