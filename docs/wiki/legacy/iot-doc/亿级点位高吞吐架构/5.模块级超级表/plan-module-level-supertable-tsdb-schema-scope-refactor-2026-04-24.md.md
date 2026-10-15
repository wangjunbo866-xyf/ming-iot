# Full/Tsdb Schema Scope 改造计划
> 状态：已废弃，不应按本文继续实施。  
原因：后续设计已从 `ModuleMode + scope=tsdb/full` 调整为 `ModuleKind + scope=periodic_decode/active_read/full`。  
当前实施口径以 [design-module-kind-release-schema-2026-04-26.md](/home/wangjunbo/workspace-aux/doc/module-level-supertable/design-module-kind-release-schema-2026-04-26.md) 和 [design-module-level-supertable-optimization-2026-04-23.md](/home/wangjunbo/workspace-aux/doc/module-level-supertable/design-module-level-supertable-optimization-2026-04-23.md) 为准。
>

当前替代规则：

+ `acxPushDecode` 使用 `scope=periodic_decode`，包含 `property_tsdb + event`
+ `acxThingForward` / `acxThingForwardResponse` 使用 `scope=active_read`，包含 `property_tsdb + property_no_store`
+ 管理端、发布校验、调试导出使用 `scope=full`
+ 不再使用 `scope=tsdb`、`pslookup_tsdb`、`pslookup_full` 作为新实现目标

## 0. 目标
本文是 [design-module-level-supertable-optimization-2026-04-23.md](/home/wangjunbo/workspace-aux/doc/design-module-level-supertable-optimization-2026-04-23.md) 的实施计划。

本次只做 V1 范围：

+ 增加模块级 `ModuleMode`：`Tsdb` / `NoStore`
+ release schema 派生为两类视图：`FullSchema` / `TsdbSchema`
+ schema cache、`runtime-schema-batch`、`ps_lookup` 支持 `scope=full/tsdb`
+ 周期解码和 TDengine 路径只使用 `TsdbSchema`
+ 主动读取 V1 继续使用 `FullSchema`，但只基于请求 identifiers 生成读块

明确不做：

+ 不新增 `ActiveReadSchema`
+ 不新增 `active_read_enabled`
+ 不把 Modbus 读块计划预存在 schema cache
+ 不把设备维度 schema 常驻 Redis

---

## 1. 当前代码基线
已存在：

+ `iot/pkg/domain/schema/runtime_bundle.go`
    - `SchemaBundle`
    - `SchemaBundleMeta`
    - `SchemaRouteEntry`
+ `iot/rpc/things/internal/relationdb/model/module.go`
    - `DmModuleInfo`
    - 但尚无 `ModuleMode`
+ `iot/rpc/things/internal/relationdb/repo/assemble_release.go`
    - `AssembleSchemaBundle`
    - 目前不按 `scope` 过滤
+ `iot/rpc/things/internal/cache/schemaBundleCache.go`
    - `ProductReleaseKey{ProductID, ReleaseID}`
    - 目前不包含 `Scope`
+ `iot/rpc/things/internal/logic/moduleschemaservice/runtime_meta_batch_logic.go`
    - `RuntimeSchemaBatch`
    - 目前只按 release 返回一份 schema
+ `iot/rpc/things/proto/pb/things/runtime_batch.pb.go`
    - Go-only batch 类型
    - `ReleaseKeyBatch` 目前只有 `ProductID / ReleaseID`
+ `ekuiper-cluster/extensions/impl/cloudx/client/batch_client.go`
    - `ReleaseKeyBatch` 目前只有 `ProductID / ReleaseID`
+ `ekuiper-cluster/extensions/impl/cloudx/ps_lookup.go`
    - `ProductSchemaLookupSource`
    - 目前没有固定 `scope` 配置

---

## 2. 工作方式
### 2.1 当前工作区和基线验证
直接在当前工作区执行，不新建 git worktree。开始改造前先验证基线：

```bash
cd /home/wangjunbo/workspace-aux
cd iot
go test ./rpc/things/internal/relationdb/repo ./rpc/things/internal/cache
cd ../ekuiper-cluster
go test ./extensions/impl/cloudx/...
```

如果基线测试已有失败，先记录失败项，不把它算作本次改造引入。

### 2.2 TDD 纪律
每个任务按 RED / GREEN / REFACTOR 执行：

+ RED：先写失败测试，确认失败原因是缺少目标行为
+ GREEN：写最小实现让测试通过
+ REFACTOR：只在测试保持通过时整理命名和重复逻辑

如果在某个任务中先写了实现，必须撤回该实现，再补失败测试。

### 2.3 改造顺序
顺序必须从低层到运行时：

1. 模型和枚举
2. schema 派生过滤
3. cache key scope 化
4. runtime-schema-batch scope 化
5. ekuiper `ps_lookup` scope 化
6. TD / 主动读取路径验收

---

## 3. Stream A：模型与基础契约
### A1. 给模块模型增加 ModuleMode
文件：

+ `iot/rpc/things/internal/relationdb/model/module.go`
+ `iot/rpc/things/internal/relationdb/model/module_test.go`

RED：

+ 新增测试断言 `DmModuleInfo` 包含 `ModuleMode` 字段
+ 新增测试断言 GORM tag 为 `column:module_mode;type:smallint;default:1`
+ 新增测试断言默认语义常量：
    - `ModuleModeTsdb == 1`
    - `ModuleModeNoStore == 2`

GREEN：

+ 在 `DmModuleInfo` 增加：

```go
type ModuleMode int64

const (
    ModuleModeTsdb    ModuleMode = 1
    ModuleModeNoStore ModuleMode = 2
)
```

+ 在 `DmModuleInfo` 增加：

```go
ModuleMode ModuleMode `gorm:"column:module_mode;type:smallint;default:1"`
```

验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./rpc/things/internal/relationdb/model
```

### A2. 增加 ModuleMode 修改约束计划位
文件：

+ `iot/rpc/things/internal/relationdb/repo/moduleInfo.go`
+ `iot/rpc/things/internal/relationdb/repo/moduleInfo_test.go`

RED：

+ 测试已发布 release 引用某 `module_code` 时，更新 `ModuleMode` 返回错误
+ 测试未发布引用时允许更新 `ModuleMode`
+ 测试只改 `ModuleName / Desc / Status` 不触发 mode 约束

GREEN：

+ 在 `ModuleInfoRepo.Update` 或等价更新入口中检测 `ModuleMode` 是否变化
+ 如果变化，查询 `dm_product_release_schema` 是否存在同 `module_code` 的未删除行
+ 存在则拒绝更新

验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./rpc/things/internal/relationdb/repo -run 'TestModuleInfo.*Mode|TestModuleInfo'
```

---

## 4. Stream B：Schema Scope 派生
### B1. 定义 schema scope 类型
文件：

+ `iot/pkg/domain/schema/runtime_bundle.go`
+ `iot/pkg/domain/schema/runtime_bundle_test.go`

RED：

+ 测试 `NormalizeSchemaScope("") == SchemaScopeTsdb`
+ 测试 `NormalizeSchemaScope("tsdb") == SchemaScopeTsdb`
+ 测试 `NormalizeSchemaScope("full") == SchemaScopeFull`
+ 测试非法 scope 返回错误

GREEN：

+ 增加：

```go
type SchemaScope string

const (
    SchemaScopeTsdb SchemaScope = "tsdb"
    SchemaScopeFull SchemaScope = "full"
)
```

+ 增加 `NormalizeSchemaScope(in string) (SchemaScope, error)`

验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./pkg/domain/schema
```

### B2. 为 release 组装增加 scope 过滤
文件：

+ `iot/rpc/things/internal/relationdb/repo/assemble_release.go`
+ `iot/rpc/things/internal/relationdb/repo/assemble_release_scope_test.go`

RED：

+ 构造 2 个模块：
    - `runtime`：`ModuleModeTsdb`
    - `diag`：`ModuleModeNoStore`
+ 构造 3 个点位：
    - `runtime.a`
    - `runtime.b`
    - `diag.c`
+ 测试 `scope=full` 返回 3 个 property，Route 也有 3 个 identifier
+ 测试 `scope=tsdb` 只返回 `runtime.a/runtime.b`，Route 不包含 `diag.c`
+ 测试非法 scope 报错

GREEN：

+ 新增 `ModuleModeByCode map[string]model.ModuleMode` 参数或 options 结构，例如：

```go
type AssembleSchemaBundleOptions struct {
    Scope            schema.SchemaScope
    ModuleModeByCode map[string]model.ModuleMode
    Limits           *SQLSizeLimits
}
```

+ 新增 `AssembleSchemaBundleWithOptions(productID, releaseID, rows, opts)`
+ 保留旧 `AssembleSchemaBundle(productID, releaseID, rows, limits)`，内部按 `scope=full` 或现有兼容语义调用，避免一次性炸全链路
+ `scope=tsdb` 时只保留 `ModuleModeTsdb` 的 rows

验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./rpc/things/internal/relationdb/repo -run 'TestAssemble.*Scope|TestReleaseSchema'
```

### B3. 从 module 表加载 ModuleMode 映射
文件：

+ `iot/rpc/things/internal/relationdb/repo/moduleInfo.go`
+ `iot/rpc/things/internal/relationdb/repo/moduleInfo_test.go`

RED：

+ 测试按 `module_code IN (...)` 返回 `map[moduleCode]ModuleMode`
+ 测试缺失 moduleCode 返回错误或明确缺失列表
+ 测试已删除模块不参与映射

GREEN：

+ 新增 repo 方法：

```go
FindModuleModeByCodes(ctx context.Context, codes []string) (map[string]model.ModuleMode, error)
```

验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./rpc/things/internal/relationdb/repo -run 'TestModuleInfo.*Mode'
```

---

## 5. Stream C：Cache Key Scope 化
### C1. 扩展 ProductReleaseKey
文件：

+ `iot/rpc/things/internal/cache/schemaBundleCache.go`
+ `iot/rpc/things/internal/cache/schemaBundleCache_test.go`

RED：

+ 测试 `ProductReleaseKey{ProductID:"p1", ReleaseID:1001, Scope:"tsdb"}.String() == "p1:1001:tsdb"`
+ 测试空 scope 规范化为 `tsdb`
+ 测试 `full` 和 `tsdb` 是两个不同 key

GREEN：

+ 修改：

```go
type ProductReleaseKey struct {
    ProductID string
    ReleaseID int64
    Scope schema.SchemaScope
}
```

+ `String()` 包含 scope
+ 新增 `Normalize()` 或构造函数，确保空 scope 为 `tsdb`

验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./rpc/things/internal/cache -run TestProductReleaseKey
```

### C2. Release cache L3 按 scope 组装
文件：

+ `iot/rpc/things/internal/cache/schemaBundleCache.go`
+ `iot/rpc/things/internal/cache/schemaBundleCache_test.go`

RED：

+ fake release rows + fake module mode loader：
    - `scope=full` 返回全部
    - `scope=tsdb` 过滤 `NoStore`
+ 测试 L3 loader 调用 assemble 时传入 scope

GREEN：

+ `NewProductSchemaBundleReleaseCache` L3 loader：
    - 读 release
    - 读 release schema rows
    - 提取 module_code
    - 调 `FindModuleModeByCodes`
    - 调 `AssembleSchemaBundleWithOptions`

验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./rpc/things/internal/cache
```

### C3. Current cache 明确使用 full scope
文件：

+ `iot/rpc/things/internal/cache/schemaBundleCache.go`
+ `iot/rpc/things/internal/cache/schemaBundleCache_test.go`

RED：

+ 测试 current cache 解析 `current_release_id` 后调用 release cache key 时使用 `scope=full`

GREEN：

+ `NewProductSchemaBundleCurrentCache` 内部调用：

```go
ProductReleaseKey{
    ProductID: productID,
    ReleaseID: pi.CurrentReleaseID,
    Scope: schema.SchemaScopeFull,
}
```

说明：

+ current view 面向管理端默认视图，使用 full
+ 设备热路径不应通过 current cache 间接拿 full；设备热路径后续由 runtime schema batch scope 控制

验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./rpc/things/internal/cache
```

---

## 6. Stream D：runtime-schema-batch Scope 化
### D1. 扩展 Go-only batch 类型
文件：

+ `iot/rpc/things/proto/pb/things/runtime_batch.pb.go`
+ `iot/rpc/things/internal/logic/moduleschemaservice/runtime_meta_batch_logic_test.go`

RED：

+ 测试 `ReleaseKeyBatch{Scope:""}` 时服务端按 `tsdb` 处理
+ 测试 `ReleaseKeyBatch{Scope:"full"}` 返回 full
+ 测试非法 scope 在对应 entry 中返回 `Error`

GREEN：

+ 给 `ReleaseKeyBatch` 增加：

```go
Scope string `json:"scope,omitempty"`
```

+ `RuntimeSchemaBatch` 内部规范化 scope
+ cache key 携带 scope

验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./rpc/things/internal/logic/moduleschemaservice -run 'TestRuntimeSchemaBatch'
```

### D2. 兼容旧调用方
文件：

+ `iot/rpc/things/internal/logic/moduleschemaservice/runtime_meta_batch_logic.go`

RED：

+ 旧请求 `{productID, releaseID}` 不带 scope 时返回 `TsdbSchema`

GREEN：

+ `Scope` 为空时默认 `tsdb`
+ 日志记录缺省 scope，便于后续清理旧调用方

验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./rpc/things/internal/logic/moduleschemaservice -run 'TestRuntimeSchemaBatch.*Default'
```

---

## 7. Stream E：ekuiper client 与 ps_lookup Scope 化
### E1. 扩展 ekuiper batch client
文件：

+ `ekuiper-cluster/extensions/impl/cloudx/client/batch_client.go`
+ `ekuiper-cluster/extensions/impl/cloudx/client/batch_client_test.go`

RED：

+ 测试 `RuntimeSchemaBatch` 请求 JSON 中包含 `scope`
+ 测试 `ReleaseKeyBatch{Scope:"tsdb"}` 正常序列化
+ 测试响应反序列化不受 scope 字段影响

GREEN：

+ 给 `ReleaseKeyBatch` 增加：

```go
Scope string `json:"scope,omitempty"`
```

验证：

```bash
cd /home/wangjunbo/workspace-aux/ekuiper-cluster
go test ./extensions/impl/cloudx/client
```

### E2. ProductSchemaLookupSource 增加 scope 配置
文件：

+ `ekuiper-cluster/extensions/impl/cloudx/ps_lookup.go`
+ `ekuiper-cluster/extensions/impl/cloudx/ps_lookup_test.go`

RED：

+ 测试无 `scope` 配置时默认 `tsdb`
+ 测试 `scope=full` 被接受
+ 测试非法 scope 在 `Validate` 时报错

GREEN：

+ `config` 增加：

```go
Scope string `json:"scope"`
```

+ `ProductSchemaLookupSource` 增加字段：

```go
scope string
```

+ `Validate` 中规范化为 `tsdb/full`

验证：

```bash
cd /home/wangjunbo/workspace-aux/ekuiper-cluster
go test ./extensions/impl/cloudx -run 'TestProductSchemaLookupSource.*Scope|TestProductSchemaLookupSource.*Validate'
```

### E3. ps_lookup 去重维度加入 scope
文件：

+ `ekuiper-cluster/extensions/impl/cloudx/ps_lookup.go`
+ `ekuiper-cluster/extensions/impl/cloudx/ps_lookup_test.go`

RED：

+ 测试两个 lookup 实例分别配置 `tsdb` 和 `full`，同一 `(productID, releaseID)` 生成不同 schema batch request
+ 测试 release 去重 key 为 `productID:releaseID:scope`

GREEN：

+ 构造 `releaseKeys` 时加入 scope
+ 调 `client.RuntimeSchemaBatch` 时传 scope
+ 构造 `schemaMap` key 时加入 scope

验证：

```bash
cd /home/wangjunbo/workspace-aux/ekuiper-cluster
go test ./extensions/impl/cloudx -run 'TestProductSchemaLookupSource.*Scope'
```

---

## 8. Stream F：规则配置与使用边界
### F1. 拆分 lookup 配置
文件：

+ `ekuiper-cluster` 中现有规则 / 配置文件，按实际 `rg -n "pslookup"` 定位

RED：

+ 如果规则配置有测试或 golden 文件，先增加断言：
    - 周期解码规则使用 `pslookup_tsdb`
    - 主动读取规则使用 `pslookup_full`

GREEN：

+ 新增或修改 lookup source 配置：
    - `pslookup_tsdb`：`scope=tsdb`
    - `pslookup_full`：`scope=full`
+ 周期解码 join `pslookup_tsdb`
+ `acxThingForward` / `acxThingForwardResponse` join `pslookup_full`

验证：

```bash
cd /home/wangjunbo/workspace-aux/ekuiper-cluster
rg -n 'pslookup_active_read|scope=active_read|ActiveReadSchema'
rg -n 'pslookup_tsdb|pslookup_full|scope=tsdb|scope=full'
go test ./extensions/impl/cloudx/...
```

---

## 9. Stream G：TDengine 路径验收
### G1. 写入路径只消费 TsdbSchema
文件：

+ `iot/rpc/things/internal/event/handler/deviceMsgEvent/thing.go`
+ `iot/rpc/things/internal/event/handler/deviceMsgEvent/module_batch_split.go`
+ `iot/rpc/things/internal/event/handler/deviceMsgEvent/module_batch_split_test.go`

RED：

+ 构造 `SchemaBundle`，Route 中只有 Tsdb 点位
+ 上报包含 Tsdb 点位和 NoStore 点位
+ 断言 NoStore 点位进入 unknown / dropped，不生成 TD 写入

GREEN：

+ 确认写入路径使用的是 `scope=tsdb` bundle
+ 如果当前仍通过 `DeviceSchemaModuleCache.GetSchemaBundle` 默认拿 full，需要改成显式拿 tsdb release key 或增加 `GetSchemaBundle(ctx, dev, scope)`

验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./rpc/things/internal/event/handler/deviceMsgEvent
```

### G2. 历史查询路径只消费 TsdbSchema
文件：

+ `iot/rpc/things/internal/logic/devicemsgmanage/property_log_index_logic.go`
+ 其他 `property_log` / module query logic，按 `rg -n "GetSchemaBundle|SchemaBundle|Route" iot/rpc/things/internal/logic/devicemsgmanage`

RED：

+ 查询 NoStore identifier 时返回“不支持历史”或等价业务错误
+ 查询 Tsdb identifier 时正常路由

GREEN：

+ 历史查询路径显式使用 `scope=tsdb`
+ Route miss 不回源 full schema

验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./rpc/things/internal/logic/devicemsgmanage
```

---

## 10. Stream H：主动读取路径验收
### H1. 主动读取使用 FullSchema 且只处理请求 identifiers
文件：

+ `iot/rpc/things/internal/logic/things/device/interact/*`
+ `iot/rpc/things/internal/event/handler/deviceMsgEvent/*`
+ `ekuiper-cluster/extensions/impl/cloudx/*forward*`
+ 实际文件用：

```bash
rg -n 'acxThingForward|PropertyGetReport|property-get-report|interact|active_read|identifiers' iot ekuiper-cluster
```

RED：

+ 请求 identifiers 为 `["diag_c"]`
+ full schema 中有 `runtime_a/runtime_b/diag_c`
+ 断言只为 `diag_c` 生成读计划或下发请求
+ 断言不会遍历 full schema 全量生成读块

GREEN：

+ 主动读取链路显式使用 `scope=full`
+ 读块生成入口只接收请求 identifiers 对应的点位定义

验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./rpc/things/internal/logic/things/device/interact/...
cd /home/wangjunbo/workspace-aux/ekuiper-cluster
go test ./extensions/impl/cloudx/...
```

---

## 11. Stream I：观测与防回归
### I1. 搜索级防回归
验证命令：

```bash
cd /home/wangjunbo/workspace-aux
rg -n 'scope=active_read|pslookup_active_read|ActiveReadSchema|active_read_enabled' iot ekuiper-cluster doc
rg -n 'ProductReleaseKey\\{|ReleaseKeyBatch\\{' iot ekuiper-cluster
```

期望：

+ 第一条只允许命中文档中“后续可选演进”的说明，不允许命中实现代码
+ 第二条人工确认所有构造点都显式考虑 scope，或有明确默认 `tsdb`

### I2. 最小测试集合
必须通过：

```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./pkg/domain/schema
go test ./rpc/things/internal/relationdb/model
go test ./rpc/things/internal/relationdb/repo
go test ./rpc/things/internal/cache
go test ./rpc/things/internal/logic/moduleschemaservice
go test ./rpc/things/internal/event/handler/deviceMsgEvent

cd /home/wangjunbo/workspace-aux/ekuiper-cluster
go test ./extensions/impl/cloudx/client
go test ./extensions/impl/cloudx/...
```

### I3. 集成验收
验收点：

+ `scope=tsdb`：
    - Redis key 包含 `:tsdb`
    - 只返回 `ModuleModeTsdb` 点位
    - NoStore 点位不会进入周期解码 / TD 写入 / 历史查询
+ `scope=full`：
    - Redis key 包含 `:full`
    - 返回 Tsdb + NoStore 全量点位
    - 主动读取只基于请求 identifiers 生成读块
+ `ps_lookup`：
    - 周期规则实例配置 `scope=tsdb`
    - 主动读取规则实例配置 `scope=full`
    - 去重维度为 `(productID, releaseID, scope)`

---

## 12. 风险与回滚
风险：

+ `ProductReleaseKey` 增加 scope 后，旧 cache key 会失效，需要接受一次冷启动回填
+ `ReleaseKeyBatch` 增加 scope 后，旧调用方不传 scope 会默认 `tsdb`，这会改变旧接口返回 full 的行为
+ 如果有管理端仍通过 `runtime-schema-batch` 调试 full schema，必须显式传 `scope=full`

回滚策略：

+ 保留旧 `AssembleSchemaBundle` 函数作为 full 兼容入口
+ `runtime-schema-batch` 只新增字段，不删除旧字段
+ `ps_lookup` 默认 scope 为 `tsdb`，出问题时可把规则配置临时改为 `scope=full` 恢复旧行为，但这只允许短期救急

---

## 13. 完成标准
代码完成必须同时满足：

+ 所有 RED 测试先失败过，并在实现后通过
+ `DmModuleInfo.ModuleMode` 已落地，并有变更约束测试
+ `FullSchema / TsdbSchema` 组装测试覆盖 Tsdb + NoStore 混合 release
+ cache key、runtime-schema-batch、ps_lookup 都包含 scope
+ 周期路径只请求 `scope=tsdb`
+ 主动读取路径只请求 `scope=full`
+ 文档中第一阶段不再出现 `scope=active_read` / `pslookup_active_read`
+ 最小测试集合全部通过

