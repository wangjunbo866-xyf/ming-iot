# 废弃 DmSchemaInfo 缓存 → 迁移到 SchemaBundle 缓存
## Context
物模型已按模块存储，运行时从 `dm_product_release_module + dm_module_schema_snapshot + dm_module_info` 组装 `SchemaBundle`。  
但 `ServiceContext` 中的 `ProductSchemaRepo`/`DeviceSchemaRepo` 仍从旧 `DmSchemaInfo` 表加载 `schema.Model`，  
大量业务逻辑（消息处理、TDengine 读写、datamanage 查询、产品 CRUD）仍在使用旧缓存。

依据文档：`doc/module-level-supertable/design-category-module-snapshot-release-2026-04-28.md` Section 6/7/9/13。

目标：废弃旧缓存和旧 TDengine 接口，全面切换到 SchemaBundle + 模块级超级表。

## 用户的 6 个要求
1. 所有 `DmSchemaInfo` 相关接口废弃（逐步注释掉）
2. TDengine 层改为接受 `SchemaBundle`
3. TDengine DDL 只在 `ProductReleasePublish` 时操作；旧建表代码废弃
4. TDengine 读写按 `DmModuleInfo(ModuleKindPropertyTsdb)` 一模块一超级表；旧的一属性一超级表废弃
5. 废弃 `ProductSchemaRepo` 和 `DeviceSchemaRepo`，使用 `SchemaBundleCurrentCache`/`SchemaBundleReleaseCache`
6. `product/auth/schema` API 不使用旧缓存，直接查 DB，输出与 SchemaBundle 缓存一致

## 当前状态（2026-04-29 更新）
| 组件 | 旧路径 | 新路径 | 状态 |
| --- | --- | --- | --- |
| 产品 schema 缓存 | `ProductSchemaRepo` → `DmSchemaInfo` | `SchemaBundleCurrentCache` → release+snapshot | **Deprecated，仅 SchemaManaRepo 构造 + fallback 使用** |
| 设备 schema 缓存 | `DeviceSchemaRepo` → `DmSchemaInfo`+聚合 | `DeviceSchemaModuleCache` → release+snapshot | **Deprecated，仅 SchemaManaRepo 构造 + fallback 使用** |
| TDengine DDL | `InitProduct`/`CreateProperty`（per-property） | `CreateModuleStable`（per-module） | **Deprecated，守卫条件已加** |
| TDengine 写入 | `InsertPropertiesData`（per-property） | `InsertModuleData`（per-module） | **新路径为主** |
| TDengine 读取 | `GetPropertyDataByID`/`GetPropertyAgg`（per-property） | `GetModulePropertyByID`/`GetModulePropertyAgg`/`GetModulePropertyByIDs`（per-module） | **新路径为主，旧方法标记 Deprecated** |
| product/auth/schema | `ProductSchemaRepo.GetData` → `Model.String()` | 直接查 DB → SchemaBundle JSON | **已完成** |


## 新旧类型关系
```plain
SchemaBundle {
    Model  schema.Model               // 完全兼容旧类型
    Route  map[string]SchemaRouteEntry // identifier → moduleCode 路由
    Meta   SchemaBundleMeta
}
```

任何需要 `*schema.Model` 的调用方，均可通过 `&bundle.Model` 获取，类型完全兼容。

---

## 执行计划
### 阶段 1：product/auth/schema API 修复（对应要求 6）
**独立于其他阶段，可直接执行。**

**背景：** ekuiper 已改用 `runtime-schema-batch`（走 SchemaBundle 缓存），`product/auth/schema` 无其他调用方。本改动目的是接口一致性。

**文件：** `iot/rpc/things/internal/logic/productmanage/product_schema_tsl_read_logic.go`

**当前代码：**

```go
model, err := l.svcCtx.ProductSchemaRepo.GetData(l.ctx, in.ProductID)
return &things.ProductSchemaTslReadResp{Tsl: model.String()}, nil
```

**改为：** 直接调用 `loadProductReleaseSchemaBundle`（DB 查询，不经过缓存），返回 SchemaBundle JSON。

```go
func (l *ProductSchemaTslReadLogic) ProductSchemaTslRead(in *things.ProductSchemaTslReadReq) (*things.ProductSchemaTslReadResp, error) {
    scope := schema.SchemaScopePeriodicDecode
    if in.Scope != "" {
        var err error
        scope, err = schema.NormalizeSchemaScope(in.Scope)
        if err != nil {
            return nil, err
        }
    }
    // 解析 current_release_id
    pi, err := repo.NewProductInfoRepo(l.ctx).FindOneByFilter(l.ctx, repo.ProductFilter{
        ProductIDs: []string{in.ProductID},
    })
    if err != nil {
        return nil, err
    }
    if pi.CurrentReleaseID == 0 {
        return nil, schema.ErrProductUnpublished
    }
    // 直接查 DB，不经缓存
    bundle, err := cache.LoadProductReleaseSchemaBundle(l.ctx, l.ctx, cache.ProductReleaseKey{
        ProductID: in.ProductID,
        ReleaseID: pi.CurrentReleaseID,
        Scope:     scope,
    })
    if err != nil {
        return nil, err
    }
    bundleJSON, _ := json.Marshal(bundle)
    return &things.ProductSchemaTslReadResp{Tsl: string(bundleJSON)}, nil
}
```

**配套改动：**

+ 将 `loadProductReleaseSchemaBundle` 改为导出函数 `LoadProductReleaseSchemaBundle`
+ proto `ProductSchemaTslReadReq` 可选增加 `scope` 字段
+ API gateway `schema_logic.go` 透传 scope

**验证：** `go build ./rpc/things/...` + 手动调用接口确认返回 SchemaBundle JSON 格式。

---

### 阶段 2：废弃旧 TDengine DDL 方法（对应要求 3）
**文件：**

+ `iot/pkg/domain/deviceMsg/msgThing/repo.go` — 接口定义
+ `iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/productManage.go` — 实现
+ `iot/rpc/things/internal/tsDB/schemaDataRepo/productManage.go` — stub 实现

**改动：**

2a. 在 `SchemaDataRepo` 接口中标记以下方法为 `// Deprecated`：

+ `InitProduct(ctx, *schema.Model, productID)`
+ `CreateProperty(ctx, *schema.Property, productID)`
+ `DeleteProperty(ctx, *schema.Property, productID, identifier)`
+ `UpdateProperty(ctx, oldP, newP *schema.Property, productID)`

2b. 将调用这些方法的逻辑文件中添加守卫条件：

```go
// 如果产品已使用 release 系统（current_release_id > 0），跳过旧 DDL
if pi.CurrentReleaseID > 0 {
    return nil // DDL 由 ProductReleasePublish 负责
}
```

涉及文件（调用旧 DDL 的地方）：

+ `logic/schemamanage/common_schema_init_logic.go`
+ `logic/schemamanage/common_schema_create_logic.go`
+ `logic/schemamanage/common_schema_update_logic.go`
+ `logic/schemamanage/common_schema_delete_logic.go`
+ `logic/productmanage/product_schema_create_logic.go`
+ `logic/productmanage/product_schema_delete_logic.go`
+ `logic/productmanage/product_init_logic.go`

---

### 阶段 3：迁移 DeviceSchemaRepo 消费方到新缓存（对应要求 5）
**模式：** `DeviceSchemaRepo.GetData(ctx, dev)` → `DeviceSchemaModuleCache.GetSchemaBundle(ctx, dev)` + `&bundle.Model`

**分组执行：**

**3a. ThingLogic 入口（最高流量）**

+ `event/handler/deviceMsgEvent/thing.go` — `initMsg()` line 66
    - `l.schema = &bundle.Model` 替代旧的 `l.schema`
    - `HandleEvent()` 使用 `l.schema.Event` — 不变
    - `HandlePropertyGetStatus()` 使用 `l.schema.Property` — 不变

**3b. 设备交互**

+ `logic/deviceinteractmanage/property_control_send_logic.go` — `initMsg()` line 364
+ `logic/deviceinteractmanage/property_get_report_send_logic.go` — line 44
+ `logic/deviceinteractmanage/action_send_logic.go` — line 44
+ `logic/deviceinteractmanage/action_resp_logic.go` — line 103

**3c. 网关处理**

+ `event/handler/deviceMsgEvent/gateway.go` — `HandlePropertyGetSchema()` lines 378,390,400

**3d. TDengine 写入**

+ `logic/tsDBHandle.go` — `UpdateDevice()` line 20, `UpdateDevices()` line 36
    - 改为从 `DeviceSchemaModuleCache.GetSchemaBundle` 获取
    - `UpdateDevice` 需要改为模块级操作（用 `bundle.Route` 遍历 moduleCode）

**3e. 设备 schema CRUD（已废弃，标记 Deprecated）**

设备级自定义物模型（DmDeviceSchema）已废弃。以下逻辑文件中涉及设备级 schema 独立读写（非 release 路径）的代码标记为 Deprecated：

+ `logic/devicemanage/device_schema_tsl_read_logic.go`
+ `logic/devicemanage/device_schema_update_logic.go`
+ `logic/devicemanage/device_schema_create_logic.go`
+ `logic/devicemanage/device_schema_multi_create_logic.go`
+ `logic/devicemanage/device_schema_multi_delete_logic.go`
+ `logic/devicemanage/device_info_delete_logic.go` — 设备删除时的 schema 清理（旧路径注释掉，TDengine 清理改为按模块 drop）

**3f. 消息管理查询**

+ `logic/devicemsgmanage/property_log_index_logic.go` — lines 67
+ `logic/devicemsgmanage/property_log_latest_index_logic.go` — line 51
+ `logic/devicemsgmanage/property_agg_index_logic.go` — lines 65

**3g. 在 ServiceContext 中标记废弃**

```go
// Deprecated: 使用 SchemaBundleCurrentCache / SchemaBundleReleaseCache / DeviceSchemaModuleCache
ProductSchemaRepo *caches.Cache[schema.Model, string]
// Deprecated: 同上
DeviceSchemaRepo  *caches.Cache[schema.Model, devices.Core]
```

**验证：** 每完成一组，编译通过。

---

### 阶段 4：迁移 ProductSchemaRepo 消费方（对应要求 1、5）
**模式：** `ProductSchemaRepo.GetData(ctx, productID)` → `SchemaBundleCurrentCache.GetData(ctx, ProductReleaseKey{ProductID, Scope: full})` + `&bundle.Model`

**分组执行：**

**4a. 产品 schema CRUD**

+ `logic/productmanage/product_schema_create_logic.go`
+ `logic/productmanage/product_schema_delete_logic.go`
+ `logic/productmanage/product_schema_update_logic.go`
+ `logic/productmanage/product_schema_multi_create_logic.go`
+ `logic/productmanage/product_schema_tsl_import_logic.go`
    - cache invalidation 改为 `InvalidateProductSchemaBundles`
    - 守卫条件：`current_release_id > 0` 时，写 DmSchemaInfo 不再触发旧 TDengine DDL

**4b. 产品生命周期**

+ `logic/productmanage/product_init_logic.go`
+ `logic/productmanage/product_info_delete_logic.go`
+ `logic/productmanage/product_info_multi_export_logic.go`

**4c. 告警处理**

+ `event/handler/alarmEvent/alarm.go`

**验证：** 编译通过。

---

### 阶段 5：迁移 datamanage 查询到模块级 TDengine 方法（对应要求 2、4）
**目标：** 将 60+ 个使用旧 `GetPropertyDataByID`/`GetPropertyAgg` 等 per-property 方法的调用方迁移到模块级方法。

**5a. SchemaDataRepo 接口增加缺失的模块级方法**

文件：`iot/pkg/domain/deviceMsg/msgThing/repo.go`

```go
// 单属性模块级查询（已存在）
GetModulePropertyByID(ctx, moduleCode, dataID string, filter FilterOpt) ([]*PropertyData, error)
GetLatestModulePropertyByID(ctx, moduleCode, dataID string, filter LatestFilter) (*PropertyData, error)

// 多属性模块级查询（新增）—— 一次查询多个 identifier，按模块分组合并结果
GetModulePropertyByIDs(ctx, moduleCode string, dataIDs []string, filter FilterOpt) (map[string][]*PropertyData, error)
GetLatestModulePropertyByIDs(ctx, moduleCode string, dataIDs []string, filter LatestFilter) (map[string]*PropertyData, error)

// 模块级聚合（新增）
GetModulePropertyAgg(ctx, moduleCode string, filter FilterAggOpt) ([]*PropertyData2, error)
GetModulePropertyCount(ctx, moduleCode, dataID string, filter FilterOpt) (int64, error)
```

**多 identifier 跨模块查询模式：**

旧模式（per-property）中每个 identifier 有独立超级表，无法跨表查询。  
新模式中同一模块的多个 identifier 共享一个超级表（多列），可以单次 SQL 查询多个列。

当调用方需要查询多个 identifier 时：

```go
// 1. 通过 SchemaBundle.Route 按 moduleCode 分组
bundle, _ := svcCtx.DeviceSchemaModuleCache.GetSchemaBundle(ctx, dev)
moduleGroups := bundle.GroupByModule(dataIDs) // map[moduleCode][]dataID

// 2. 每个模块发一次查询
results := map[string][]*PropertyData{}
for moduleCode, ids := range moduleGroups {
    data, _ := dd.GetModulePropertyByIDs(ctx, moduleCode, ids, filter)
    for id, rows := range data {
        results[id] = rows
    }
}
```

**跨模块场景示例：**

+ `device_list_logic.go` 同时查 `LOADPOWER_I_PV`、`DAY_ENERGY_STORE`、`TOTAL_ENERGY_STORE`
    - 如果同属一个模块 → 1 次 TDengine 查询
    - 如果分属 2 个模块 → 2 次查询（仍优于旧的 3 次）
+ `inverter_report_list_logic.go` 查 11+ 个 identifier → 按模块分组，从 11 次降为 N 模块次
+ `report_logic.go` 的 `dataMMulti`/`dayLastScalars` → 同理按模块聚合

**5b. 实现模块级 TDengine 方法**

文件：`iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/moduleData.go`

+ 实现 `GetModulePropertyByIDs`：按 moduleCode 查询超级表，SELECT 指定列，返回 `map[string][]*PropertyData`
    - SQL 模式：`SELECT ts, identifier1, identifier2, ... FROM model_module__{moduleCode} WHERE ...`
    - 结果按 identifier 拆分返回
+ 实现 `GetLatestModulePropertyByIDs`：同上，LIMIT 1 + ORDER BY ts DESC
+ 实现 `GetModulePropertyAgg`：按 moduleCode 查询超级表，支持时间窗口聚合
    - SQL 模式：复用 `propertyDataAgg.go` 的窗口/聚合/Fill 逻辑，但 FROM 改为模块超级表
+ 实现 `GetModulePropertyCount`：按 moduleCode + identifier 统计行数

stub 文件：`iot/rpc/things/internal/tsDB/schemaDataRepo/moduleData.go`

**5c. 迁移 datamanage 调用方**

核心迁移模式：

单 identifier：

```go
// 旧
s, _ := svcCtx.DeviceSchemaRepo.GetData(ctx, dev)
p := s.Property[identifier]
data, _ := dd.GetPropertyDataByID(ctx, p, filter)
// 新
bundle, _ := svcCtx.DeviceSchemaModuleCache.GetSchemaBundle(ctx, dev)
route, _ := bundle.GetRoute(identifier)
data, _ := dd.GetModulePropertyByID(ctx, route.ModuleCode, identifier, filter)
```

多 identifier（跨模块）：

```go
// 旧：逐个调用
for _, id := range dataIDs {
    p := s.Property[id]
    data, _ := dd.GetPropertyDataByID(ctx, p, filter)
}
// 新：按模块分组，每模块一次查询
moduleGroups := bundle.GroupByModule(dataIDs)
for moduleCode, ids := range moduleGroups {
    data, _ := dd.GetModulePropertyByIDs(ctx, moduleCode, ids, filter)
    // merge results
}
```

多 identifier 聚合（如 `GetPropertyAgg`）：

```go
// 旧：传入 schema.Model（含所有属性），内部逐属性查不同超级表
dds, _ := dd.GetPropertyAgg(ctx, model, aggFilter)
// 新：按模块分组，每模块调用 GetModulePropertyAgg
moduleGroups := bundle.GroupByModule(aggIDs)
for moduleCode, ids := range moduleGroups {
    aggFilter.Aggs = filterAggsByIDs(aggFilter.Aggs, ids)
    dds, _ := dd.GetModulePropertyAgg(ctx, moduleCode, aggFilter)
}
```

**文件清单（按优先级）：**

高频查询路径：

+ `logic/datamanage/history_logic.go` — ~16 个调用点
+ `logic/datamanage/report_logic.go` — ~4 个调用点
+ `logic/datamanage/signal_logic.go` — 1 个调用点

设备状态查询：

+ `logic/datamanage/device_info_logic.go` — ~6 个调用点
+ `logic/datamanage/device_list_logic.go` — 1 个调用点
+ `logic/datamanage/device_version_logic.go` — 1 个调用点

电站级查询：

+ `logic/datamanage/plant_master_logic.go` — 1 个调用点
+ `logic/datamanage/plant_setting_master_logic.go` — 1 个调用点
+ `logic/datamanage/inverter_report_list_logic.go` — ~3 个调用点

OTA/设备管理：

+ `logic/otamanage/ota_firmware_info_group_index_logic.go` — ~2 个调用点
+ `logic/devicemanage/device_by_plant_logic.go` — ~2 个调用点
+ `logic/devicemsgmanage/event_log_index_logic.go` — 1 个调用点

**5d. 标记旧 SchemaDataRepo 方法为 Deprecated**

```go
// Deprecated: 使用 GetModulePropertyByID
GetPropertyDataByID(ctx, p *schema.Property, filter) ([]*PropertyData, error)
// Deprecated: 使用 GetLatestModulePropertyByID
GetLatestPropertyDataByID(ctx, p *schema.Property, filter) (*PropertyData, error)
// Deprecated: 使用 GetModulePropertyAgg
GetPropertyAgg(ctx, m *schema.Model, filter) ([]*PropertyData2, error)
// Deprecated: 使用 InsertModuleData
InsertPropertiesData(ctx, t *schema.Model, ...) error
InsertPropertyData(ctx, t *schema.Property, ...) error
```

**验证：** 编译通过。

---

### 阶段 6：清理旧缓存初始化（对应要求 5 最终步骤）
**文件：** `iot/rpc/things/internal/svc/service_context.go`

**6a. 移除旧缓存初始化代码**

+ 删除 `getProductSchemaModel` cache 创建（lines 159-178）
+ 删除 `getDeviceSchemaModel` cache 创建（lines 180-226）
+ 删除 `ProductSchemaRepo` 和 `DeviceSchemaRepo` 字段
+ 更新 `SchemaManaRepo` 构造函数，改为接收新缓存

**6b. 更新 SchemaManaRepo 构造**

文件：`iot/rpc/things/internal/tsDB/schemaDataRepo/schemaDataRepo.go`（或对应初始化文件）

`SchemaManaRepo` 构造函数目前接收 `getProductSchemaModel.GetData` 和 `getDeviceSchemaModel.GetData` 回调。  
改为接收 `DeviceSchemaModuleCache.GetSchemaBundle` 回调。

**6c. 清理旧 DB repo 文件**

+ 标记废弃：`repo/productSchema.go`（DmSchemaInfo 的产品级 CRUD）
+ 标记废弃：`repo/deviceSchema.go`（DmSchemaInfo 的设备级 CRUD）
+ 标记废弃：`repo/schemaInfo.go`（DmSchemaInfo 统一 repo）
+ 标记废弃：`cache/schemaRepo.go`（ristretto 缓存）

**验证：** 全量编译 `go build ./...`

---

### 阶段 7：ekuiper-cluster 确认
ekuiper 已经使用新缓存（`currentSchemaCache`/`releaseSchemaCache`），通过 `runtime-schema-batch` API 获取 `SchemaBundle`。

**确认项：**

+ ekuiper 不受阶段 1 中 `product/auth/schema` 改动影响（它用的是 `runtime-schema-batch`）
+ 如果有部署配置指向 `product/auth/schema`，需要更新为 `runtime-schema-batch`
+ 无代码改动

---

## 依赖图
```plain
阶段 1 (auth/schema API) ← 独立，可先做
阶段 2 (废弃旧 TDengine DDL) ← 独立，可先做
     ↓
阶段 3 (DeviceSchemaRepo → 新缓存) ← 依赖阶段 1/2
     ↓
阶段 4 (ProductSchemaRepo → 新缓存) ← 依赖阶段 3
     ↓
阶段 5 (datamanage 查询迁移) ← 依赖阶段 3
     ↓
阶段 6 (清理旧缓存) ← 依赖阶段 3/4/5 全部完成
     ↓
阶段 7 (ekuiper 确认) ← 无代码改动
```

阶段 1、2、3 可并行。阶段 4、5 可并行（都依赖 3）。阶段 6 是最终清理。

## 验证
每个阶段完成后：

```bash
cd /home/wangjunbo/workspace-aux/iot
go build ./pkg/domain/...
go build ./pkg/domain/deviceMsg/...
go build ./rpc/things/internal/...
go build ./api/gateway/...
go test ./pkg/domain/schema/...
go test ./rpc/things/internal/cache/...
```

端到端验证：

1. `ProductReleasePublish` 发布产品 → TDengine 模块超级表创建成功
2. 设备上报 property → `InsertModuleData` 写入模块超级表
3. `product/auth/schema` API → 返回 SchemaBundle JSON（Model + Route + Meta）
4. ekuiper `ps_lookup` → 解码成功
5. datamanage 历史查询 → 返回正确数据

