# ModuleKind / Schema Scope 改造计划（iot + ekuiper-cluster）
## 0. 目标
本文是当前有效设计的实施计划，覆盖：

+ `iot`
+ `ekuiper-cluster`

当前有效设计以以下文档为准：

+ `doc/module-level-supertable/design-module-level-supertable-2026-04-15.md`
+ `doc/module-level-supertable/design-module-level-supertable-optimization-2026-04-23.md`
+ `doc/module-level-supertable/design-module-kind-release-schema-2026-04-26.md`
+ `doc/module-level-supertable/2026-04-25-ekuiper-schema-cache-design.md`

旧计划 `plan-module-level-supertable-tsdb-schema-scope-refactor-2026-04-24.md` 已废弃，不再按 `ModuleMode + scope=tsdb/full` 实施。

## 1. 固定结论
必须遵守：

+ `DmModuleInfo.ModuleKind` 创建后绝对不允许修改。
+ `ModuleKind` 不冗余到 `DmProductReleaseSchema`。
+ `DmProductReleaseSchema` 只冻结 `release_id + product_id + module_id + module_code + identifier + DmSchemaCore`。
+ 视图派生、TDengine 建表、写入和查询需要模块类型时，通过 `dm_product_release_schema.module_id` join `dm_module_info.module_kind` 获取。
+ schema scope 只有 `periodic_decode`、`active_read`、`full` 三种。
+ `periodic_decode = property_tsdb + event`，用于 `acxPushDecode` 和周期上报。
+ `active_read = property_tsdb + property_no_store`，用于 `acxThingForward` / `acxThingForwardResponse`。
+ `full = 全部 ModuleKind`，只用于管理端、发布校验、调试导出和完整展示。
+ `DmDeviceInfo.Version` 是固件版本，不参与 schema release 路由。
+ 设备 release 路由固定为 `SchemaReleaseIDOverride > 0 ? override : product.current_release_id`。
+ 不做设备级 Redis SchemaBundle 常驻缓存。
+ `acxPushDecode` 遇到单个子设备 schema 缺失或 release 解析失败时跳过该子设备，不能拖垮整包。

## 2. 当前代码基线
从当前仓库看，已有部分新链路，但仍有旧口径残留。

| 模块 | 当前状态 | 目标状态 |
| --- | --- | --- |
| `iot/rpc/things/internal/relationdb/model/module.go` | 仍是 `ModuleModeTsdb / ModuleModeNoStore` 和 `module_mode` | 改为 `ModuleKindPropertyTsdb / PropertyNoStore / Event / Action` 和 `module_kind` |
| `iot/pkg/domain/schema/runtime_bundle.go` | 仍是 `SchemaScopeTsdb / SchemaScopeFull`，默认 `tsdb` | 改为 `periodic_decode / active_read / full`，默认 `periodic_decode` |
| `iot/rpc/things/internal/relationdb/repo/assemble_release.go` | `AssembleSchemaBundleOptions.ModuleModeByCode`，scope 只过滤 Tsdb | 改为 `ModuleKindByID`，按三种 scope 派生视图 |
| `iot/rpc/things/internal/cache/schemaBundleCache.go` | cache key scope 仍默认 `tsdb`，失效只覆盖 `tsdb/full` | scope 改为三类，失效覆盖 `periodic_decode/active_read/full` |
| `iot/rpc/things/internal/logic/moduleschemaservice/runtime_meta_batch_logic.go` | `RuntimeSchemaBatch` 已有 scope，但默认仍依赖旧 `NormalizeSchemaScope` | 跟随新 scope，部分失败继续返回单项 error |
| `iot/api/gateway/apifile/things/device/auth.api` | 当前文件未声明 `runtime-meta-batch` / `runtime-schema-batch` | 补齐 auth API 定义并重新生成 handler/types |
| `ekuiper-cluster/extensions/impl/cloudx/ps_lookup.go` | scope 只允许 `tsdb/full`，默认 `tsdb` | scope 只允许 `periodic_decode/active_read/full`，默认 `periodic_decode` |
| `ekuiper-cluster/extensions/impl/cloudx/product/release_schema_cache.go` | schema L3 请求默认 `tsdb` | 默认 `periodic_decode`，支持三 scope |
| `ekuiper-cluster/extensions/impl/cloudx/client/batch_client.go` | `ProductReleaseKey.String()` 默认 `tsdb` | 默认 `periodic_decode` |
| `ekuiper-cluster/extensions/functions/acxPushDecode/handle_push_data.go` | 已有“跳过单子设备”逻辑 | 保留并补回归测试 |
| `ekuiper-cluster/extensions/functions/acxThingForward*` | 依赖 lookup 传入 schema | 规则层必须 join `pslookup_active_read` |


## 3. 目标运行链路
### 3.1 iot schema cache
```latex
ProductSchemaRepo.GetCurrent(productID, scope)
    -> 读取 dm_product_info.current_release_id
    -> 复用 GetByRelease(productID, currentReleaseID, scope)

ProductSchemaRepo.GetByRelease(productID, releaseID, scope)
    -> 读取 dm_product_release_schema
    -> join dm_module_info 获取 module_kind
    -> 按 scope 过滤
    -> 返回 SchemaBundle{Model, Route, Meta}

DeviceSchemaRepo.GetData(dev, scope)
    -> 读取 cache:things:device:runtime:{productID}:{deviceName}
    -> SchemaReleaseIDOverride > 0 ? GetByRelease(..., scope) : GetCurrent(..., scope)
```

### 3.2 ekuiper schema lookup
V1 `ps_lookup` 使用三类 iot 接口：

+ `dgHttpUrl`: `api/v1/things/device/auth/gateway-subset`
+ `metaBatchUrl`: `api/v1/things/device/auth/runtime-meta-batch`
+ `psHttpUrl`: `api/v1/things/product/auth/schema`

不再使用旧的逐设备 `diHttpUrl: api/v1/things/device/auth/read`。

schema 获取策略：

+ 先通过 `gateway-subset` 获取子设备列表。
+ 再通过 `runtime-meta-batch` 获取每个子设备的 `SchemaReleaseIDOverride` 和 `EffectiveReleaseID`。
+ 按 `(productID, effectiveReleaseID, scope)` 去重。
+ 对 override 设备读 `releaseSchemaCache`，对未 override 设备读 `currentSchemaCache`。
+ cache L1/L2 miss 时由 `psHttpUrl` 回源，带 `{productID, releaseID?, scope}`，返回 `SchemaBundle`。

规则层必须拆成两个 lookup 实例。这里的“两个实例”含义必须明确：

+ 仍然共用同一个 `pslookup` 插件类型、同一个 `ProductSchemaLookupSource` Go 实现、同一套 Redis / NATS / HTTP 配置能力。
+ 不是复制两份 `ps_lookup.go`，也不是维护两套 schema lookup 代码。
+ 是在 ekuiper 规则配置中创建两个不同名字的 lookup source/table 实例，二者配置基本相同，只有 `scope` 不同。
+ 两个实例各自持有自己的 `ProductSchemaLookupSource.scope` 固定值；lookup 返回结构仍保持 `subDevicesSchema / subDevicesInfo / timeZone` 不变。
+ cache key、请求去重 key、Redis schema key 都必须包含 `scope`，因此两个实例即使查询同一 `(productID, releaseID)`，也不会误复用对方的 schema value。

实例划分：

+ `pslookup_periodic_decode`：source type 仍是 `pslookup`，配置 `scope=periodic_decode`，只给 `acxPushDecode`。
+ `pslookup_active_read`：source type 仍是 `pslookup`，配置 `scope=active_read`，只给 `acxThingForward` / `acxThingForwardResponse`。

配置示意：

```yaml
pslookup_periodic_decode:
  type: pslookup
  redisaddr: ${REDIS_ADDR}
  dgHttpUrl: http://iot/api/v1/things/device/auth/gateway-subset
  metaBatchUrl: http://iot/api/v1/things/device/auth/runtime-meta-batch
  psHttpUrl: http://iot/api/v1/things/product/auth/schema
  scope: periodic_decode

pslookup_active_read:
  type: pslookup
  redisaddr: ${REDIS_ADDR}
  dgHttpUrl: http://iot/api/v1/things/device/auth/gateway-subset
  metaBatchUrl: http://iot/api/v1/things/device/auth/runtime-meta-batch
  psHttpUrl: http://iot/api/v1/things/product/auth/schema
  scope: active_read
```

规则使用示意：

```sql
-- 周期上报规则：只 join periodic_decode lookup
SELECT acxPushDecode(..., pslookup_periodic_decode.subDevicesSchema, ...)
FROM mqtt_push_stream
FULL JOIN pslookup_periodic_decode
ON meta(SN) = pslookup_periodic_decode.deviceName
AND pslookup_periodic_decode.productID = "col01";

-- 主动读取下发 / 回包规则：只 join active_read lookup
SELECT acxThingForward(..., pslookup_active_read.subDevicesSchema, ...)
FROM thing_forward_stream
FULL JOIN pslookup_active_read
ON meta(SN) = pslookup_active_read.deviceName
AND pslookup_active_read.productID = "col01";
```

禁止做法：

+ 不允许周期上报和主动读取共用同一个名为 `pslookup` 的实例。
+ 不允许用 `full` scope 给 `acxPushDecode`。
+ 不允许用 `periodic_decode` scope 给主动读取，因为它缺少 `property_no_store` 点位。

## 4. 实施原则
执行顺序：

1. 先改 iot domain/model/scope 的底层契约。
2. 再改 iot release assemble/cache/API。
3. 再改 iot 写入、查询和发布路径。
4. 最后改 ekuiper-cluster lookup、缓存和规则配置。

TDD 要求：

+ 每个任务先写失败测试。
+ 测试失败原因必须是缺少目标行为，而不是编译错误或测试写错。
+ 通过最小实现让测试变绿。
+ 重构时保持测试绿。

工作区要求：

+ 建议使用独立 worktree 或 feature branch。
+ 改造前记录当前基线测试结果。
+ 不要混入无关格式化和大范围重排。

## 5. iot 改造任务
### A. SchemaScope 契约
| 任务 | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| A1 | `iot/pkg/domain/schema/runtime_bundle.go` | 增加 `SchemaScopePeriodicDecode`、`SchemaScopeActiveRead`、`SchemaScopeFull`；删除或停止使用 `SchemaScopeTsdb`；`NormalizeSchemaScope("")` 返回 `periodic_decode` | `go test ./pkg/domain/schema` |
| A2 | `iot/pkg/domain/schema/runtime_bundle_test.go` | 增加默认 scope、三 scope 合法、`tsdb` 非法的测试 | `go test ./pkg/domain/schema -run Scope` |
| A3 | `iot/rpc/things/internal/cache/schemaBundleCache.go` | `ProductReleaseKey.String()` 默认 `periodic_decode`；失效 scope 列表改为三类 | `go test ./rpc/things/internal/cache -run ProductReleaseKey` |
| A4 | `iot/rpc/things/internal/cache/schemaBundleCache_scope_test.go` | 更新 `tsdb/full` 断言为 `periodic_decode/active_read/full` | `go test ./rpc/things/internal/cache -run Scope` |


### B. ModuleKind 模型
| 任务 | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| B1 | `iot/rpc/things/internal/relationdb/model/module.go` | `ModuleMode` 改为 `ModuleKind`；字段 `module_mode` 改为 `module_kind`；枚举为 `PropertyTsdb/PropertyNoStore/Event/Action` | `go test ./rpc/things/internal/relationdb/model` |
| B2 | `iot/rpc/things/internal/relationdb/model/module_test.go` | 增加 GORM tag 断言：`column:module_kind;type:smallint;default:1` | `go test ./rpc/things/internal/relationdb/model -run Module` |
| B3 | `iot/rpc/things/internal/relationdb/repo/moduleInfo.go` | `FindModuleModeByCodes` 改为 `FindModuleKindByIDs` 或 `FindModuleKindByCodes`；优先按 `module_id` 查询 | `go test ./rpc/things/internal/relationdb/repo -run ModuleInfo` |
| B4 | `iot/rpc/things/internal/logic/moduleschemaservice/module_info_update_logic.go` | update 接口拒绝修改 `module_kind`，返回明确业务错误 | `go test ./rpc/things/internal/logic/moduleschemaservice -run ModuleInfoUpdate` |
| B5 | `doc/module-level-supertable/migration.sql` | 已有 `module_kind`、CHECK、trigger；实施时落库并确认 `dm_product_release_schema` 不增加 `module_kind` | 执行迁移后用 `\d dm_module_info` 和 `\d dm_product_release_schema` 验证 |


### C. Release 组装与 scope 派生
| 任务 | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| C1 | `iot/rpc/things/internal/relationdb/repo/assemble_release.go` | `AssembleSchemaBundleOptions.ModuleModeByCode` 改为 `ModuleKindByID map[int64]model.ModuleKind` | `go test ./rpc/things/internal/relationdb/repo -run Assemble` |
| C2 | `iot/rpc/things/internal/relationdb/repo/assemble_release.go` | `periodic_decode` 保留 `property_tsdb + event`；`active_read` 保留 `property_tsdb + property_no_store`；`full` 不过滤 | `go test ./rpc/things/internal/relationdb/repo -run Scope` |
| C3 | `iot/rpc/things/internal/relationdb/repo/assemble_release_scope_test.go` | 用 4 类 ModuleKind 构造 release rows，断言三 scope 的 properties/events/actions 数量 | `go test ./rpc/things/internal/relationdb/repo -run AssembleReleaseScope` |
| C4 | `iot/rpc/things/internal/relationdb/repo/assemble_release.go` | Route 从 scoped rows 生成；`periodic_decode` Route 只含 `property_tsdb`；`active_read` Route 含 `property_tsdb + property_no_store` | 同 C3 |
| C5 | `iot/rpc/things/internal/cache/schemaBundleCache.go` | `loadProductReleaseSchemaBundle` 先查 release schema rows，再按 `module_id` 查 ModuleKind map，传入 assemble | `go test ./rpc/things/internal/cache -run SchemaBundle` |
| C6 | `iot/rpc/things/internal/cache/schemaBundleCache_test.go` | 覆盖 current/release cache 三 scope key 和默认 scope | `go test ./rpc/things/internal/cache` |


### D. Product / Device schema cache 合约
| 任务 | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| D1 | `iot/rpc/things/internal/cache/schemaBundleCache.go` | `GetSchemaBundleForDevice` 增加 `scope` 参数；保留旧 `GetData` 包装时只能默认 `periodic_decode` | `go test ./rpc/things/internal/cache -run Device` |
| D2 | `iot/rpc/things/internal/cache/schemaBundleCache_test.go` | 测 override 设备读 release cache，未 override 读 current cache，scope 透传 | `go test ./rpc/things/internal/cache -run Device` |
| D3 | `iot/api/gateway/apifile/things/product/schema.api` 或对应 auth api | 产品 auth schema 请求增加 `releaseID` 和 `scope`；响应为 `SchemaBundle` | 重新生成 API 后 `go test ./api/gateway/internal/logic/things/product/auth` |
| D4 | `iot/api/gateway/internal/logic/things/product/auth/schema_logic.go` | `releaseID>0` 走 release cache；否则走 current cache；默认 scope 为 `periodic_decode` | `go test ./api/gateway/internal/logic/things/product/auth` |
| D5 | `iot/api/gateway/internal/types/things_product_schema.go` | 确认生成后的 request/response 类型包含 `releaseID/scope` 和 bundle 结构 | `go test ./api/gateway/...` 的相关包 |


### E. Runtime meta / batch 接口
| 任务 | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| E1 | `iot/api/gateway/apifile/things/device/auth.api` | 增加 `runtime-meta-batch`；请求 `{devices:[{productID,deviceName}]}`；响应逐设备 meta | 重新生成 API 后编译 |
| E2 | `iot/api/gateway/apifile/things/device/auth.api` | 如保留 `runtime-schema-batch`，请求必须是 `{releases:[{productID,releaseID,scope}]}`，scope 为三类之一 | 重新生成 API 后编译 |
| E3 | `iot/rpc/things/internal/logic/moduleschemaservice/runtime_meta_batch_logic.go` | `RuntimeSchemaBatch` 默认 scope 改为 `periodic_decode`，非法 scope 单项失败，不整批失败 | `go test ./rpc/things/internal/logic/moduleschemaservice -run Runtime` |
| E4 | `iot/api/gateway/internal/logic/things/device/auth/runtime_meta_batch_logic.go` | 网关层透传 RPC，保持部分成功语义 | `go test ./api/gateway/internal/logic/things/device/auth -run Runtime` |
| E5 | `iot/api/gateway/internal/logic/things/device/auth/runtime_schema_batch_logic.go` | 如果用于调试/兜底，返回 scoped `SchemaBundle` 或至少返回 scoped `Model`；禁止忽略 scope | 同 E4 |


### F. 发布流程与 TDengine
| 任务 | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| F1 | `iot/rpc/things/internal/logic/moduleschemaservice/product_release_publish_logic.go` | 发布时读取 `DmModuleInfo.ModuleKind` 校验模块类型和 affordance 类型匹配，不复制 `module_kind` 到 release schema | `go test ./rpc/things/internal/logic/moduleschemaservice -run ProductReleasePublish` |
| F2 | `iot/rpc/things/internal/logic/moduleschemaservice/product_release_publish_logic_test.go` | 覆盖 property/event/action 混放时发布失败；`DmProductReleaseSchema` 不含 `module_kind` | 同 F1 |
| F3 | `iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/moduleManage.go` | 只为 `ModuleKindPropertyTsdb` 创建/扩展模块超级表 | `go test ./rpc/things/internal/tsDB/tdengine/schemaDataRepo -run Module` |
| F4 | `iot/rpc/things/internal/event/handler/deviceMsgEvent/thing.go` | 设备周期上报只用 `periodic_decode` bundle；未知 identifier 按策略跳过或报业务错误，不回源 SQL | `go test ./rpc/things/internal/event/handler/deviceMsgEvent` |
| F5 | `iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/moduleData.go` | 模块批量 insert 只消费 `periodic_decode` 中的 `property_tsdb` Route | `go test ./rpc/things/internal/tsDB/tdengine/schemaDataRepo -run ModuleData` |


## 6. ekuiper-cluster 改造任务
### G. Scope 与 cache key
| 任务 | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| G1 | `ekuiper-cluster/extensions/impl/cloudx/client/batch_client.go` | `ProductReleaseKey.String()` 默认 `periodic_decode`；测试中不再出现 `tsdb` | `go test ./extensions/impl/cloudx/client` |
| G2 | `ekuiper-cluster/extensions/impl/cloudx/client/batch_client_test.go` | 更新 scope 测试为 `periodic_decode/active_read/full` | 同 G1 |
| G3 | `ekuiper-cluster/extensions/impl/cloudx/product/release_schema_cache.go` | `loadSchemaBundle` 默认 scope 改为 `periodic_decode`；HTTP 请求传三 scope | `go test ./extensions/impl/cloudx/product` |
| G4 | `ekuiper-cluster/extensions/impl/cloudx/schema/cache_types.go` | 保持与 iot `SchemaBundle` JSON 兼容；必要时支持 `model/route/meta` 小写字段 | `go test ./extensions/impl/cloudx/schema` |


### H. ps_lookup
| 任务 | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| H1 | `ekuiper-cluster/extensions/impl/cloudx/ps_lookup.go` | scope 校验改为 `periodic_decode/active_read/full`，默认 `periodic_decode`；拒绝 `tsdb` | `go test ./extensions/impl/cloudx -run Scope` |
| H2 | `ekuiper-cluster/extensions/impl/cloudx/ps_lookup_scope_test.go` | 测默认 `periodic_decode`、接受 `active_read/full`、拒绝 `tsdb` | 同 H1 |
| H3 | `ekuiper-cluster/extensions/impl/cloudx/ps_lookup.go` | 移除旧 `diHttpUrl` 依赖；保留 `dgHttpUrl/metaBatchUrl/psHttpUrl` | `go test ./extensions/impl/cloudx` |
| H4 | `ekuiper-cluster/extensions/impl/cloudx/ps_lookup.go` | 对子设备先取 runtime meta，再按 `(productID,effectiveReleaseID,scope)` 去重获取 schema，避免同 release 重复 cache miss 回源 | 新增单测，断言同 key 只调用一次 schema cache |
| H5 | `ekuiper-cluster/extensions/impl/cloudx/ps_lookup.go` | 单个子设备 meta/schema 失败只跳过该子设备，整包继续返回其他子设备 | 新增单测，断言部分成功 |
| H6 | `ekuiper-cluster/extensions/sources/cloudx/pslookup.yaml` 和 `pslookup.json` | 同一个 `pslookup` 插件类型支持通过 `scope` 配置创建两个规则实例：`pslookup_periodic_decode(scope=periodic_decode)`、`pslookup_active_read(scope=active_read)`；不复制 `ProductSchemaLookupSource` 代码 | 配置加载测试或手工启动验证 |


### I. acxPushDecode
| 任务 | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| I1 | `ekuiper-cluster/extensions/functions/acxPushDecode/handle_push_data.go` | 保留当前 skip 单子设备策略；确认 properties 缺失、schema 缺失、decode 失败均跳过该子设备 | `go test ./extensions/functions/acxPushDecode` |
| I2 | `ekuiper-cluster/extensions/functions/acxPushDecode/handle_push_data_test.go` | 增加回归测试：一个子设备 schema 缺失时，其他子设备仍 decode 并返回事件消息 | 同 I1 |
| I3 | 规则配置 | `acxPushDecode` 所在规则只 join `pslookup_periodic_decode` | 本地规则启动后检查 lookup scope 日志 |


### J. acxThingForward / acxThingForwardResponse
| 任务 | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| J1 | `ekuiper-cluster/extensions/functions/acxThingForward/acx_thing_forward.go` | 确认主动读取只依赖 `active_read` schema 的 properties，不要求 event/action | `go test ./extensions/functions/acxThingForward` |
| J2 | `ekuiper-cluster/extensions/functions/acxThingForwardResponse/acx_thing_forward_response.go` | schema 缺失时返回当前请求失败，但不影响 `ps_lookup` 对其他设备的缓存；必要时补更明确错误 | `go test ./extensions/functions/acxThingForwardResponse` |
| J3 | 规则配置 | `acxThingForward` 和 `acxThingForwardResponse` 所在规则只 join `pslookup_active_read` | 本地规则启动后检查 lookup scope 日志 |


## 7. 集成验证
### 7.1 iot 单测
```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./pkg/domain/schema
go test ./rpc/things/internal/relationdb/model
go test ./rpc/things/internal/relationdb/repo
go test ./rpc/things/internal/cache
go test ./rpc/things/internal/logic/moduleschemaservice
go test ./api/gateway/internal/logic/things/device/auth
go test ./api/gateway/internal/logic/things/product/auth
go test ./rpc/things/internal/tsDB/tdengine/schemaDataRepo
go test ./rpc/things/internal/event/handler/deviceMsgEvent
```

### 7.2 ekuiper-cluster 单测
```bash
cd /home/wangjunbo/workspace-aux/ekuiper-cluster
go test ./extensions/impl/cloudx/client
go test ./extensions/impl/cloudx/product
go test ./extensions/impl/cloudx/schema
go test ./extensions/impl/cloudx
go test ./extensions/functions/acxPushDecode
go test ./extensions/functions/acxThingForward
go test ./extensions/functions/acxThingForwardResponse
```

### 7.3 端到端场景
| 场景 | 输入 | 预期 |
| --- | --- | --- |
| 周期上报 | 网关下 50 台子设备，其中 49 台 schema 正常，1 台 release 无效 | 49 台成功 decode，1 台 warn skip，整包返回成功 |
| 周期 scope | `pslookup_periodic_decode` | schema 只包含 `property_tsdb + event` |
| 主动读取 scope | `pslookup_active_read` | schema 只包含 `property_tsdb + property_no_store` |
| full scope | 管理端或调试接口 | 返回全部 ModuleKind |
| override 设备 | `SchemaReleaseIDOverride > 0` | 读取指定 release schema |
| 默认设备 | `SchemaReleaseIDOverride = 0` | 跟随 `product.current_release_id` |
| ModuleKind 修改 | update `dm_module_info.module_kind` | DB trigger 和业务接口都拒绝 |
| TDengine 写入 | `active_read` schema 被误传入写入路径 | 测试应失败；写入路径必须只接受 `periodic_decode` |


## 8. 发布与回滚
发布顺序：

1. 上线 iot 数据库迁移。
2. 上线 iot schema scope / ModuleKind / cache / API 改造。
3. 预热产品 current/release 三类 scope cache。
4. 上线 ekuiper-cluster 新 lookup scope 校验和缓存逻辑。
5. 切规则：周期上报规则使用 `pslookup_periodic_decode`，主动读取规则使用 `pslookup_active_read`。
6. 观察缓存命中率、schema value 大小、ps_lookup P95/P99、子设备 skip 数。

回滚策略：

+ 若 iot schema cache 出现问题，先回滚 ekuiper 规则到旧 lookup，不改数据库。
+ 若 `module_kind` 迁移失败，不继续发布 iot 应用。
+ `module_kind` 不允许回滚为可编辑字段；若建错模块类型，只能新建模块。
+ `DmProductReleaseSchema` 不新增 `module_kind`，因此不涉及 release schema 字段回滚。

## 9. 不做事项
V1 明确不做：

+ 不新增 `DmProductReleaseModule`。
+ 不在 `DmProductReleaseSchema` 冗余 `module_kind`。
+ 不支持 `scope=tsdb`。
+ 不做 `Version -> Release` 固件版本映射。
+ 不缓存每台设备的最终 SchemaBundle。
+ 不恢复 `struct/array`。
+ 不新增点位级 `active_read_enabled`。
+ 不把 Modbus 读块计划预存在 schema cache。

