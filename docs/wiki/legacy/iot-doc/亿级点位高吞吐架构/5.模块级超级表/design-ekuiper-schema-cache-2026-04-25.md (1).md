# ekuiper-cluster Schema 缓存重构设计
## 1. 现状问题
`ps_lookup.go` 的 `Lookup` 方法每次调用都通过 2 个 batch HTTP 请求获取子设备信息：

```plain
Lookup() 流程：
1. deviceInfoCache.GetData(ctx, gateway) → TimeZone          ← L1→L2→L3 缓存 ✓
2. deviceGatewaySubsetCache.GetData(ctx, gateway) → 子设备列表 ← L1→L2→L3 缓存 ✓
3. RuntimeMetaBatch(ctx, devices) → EffectiveReleaseID        ← 每次 HTTP ✗
4. RuntimeSchemaBatch(ctx, releases) → TSL                   ← 每次 HTTP ✗
5. 遍历子设备 deviceInfoCache.GetData → RS485_ADDR            ← L1→L2→L3 缓存 ✓
```

**问题**：

+ 步骤 3、4 每条消息都触发 HTTP 调用，完全绕过缓存层
+ 步骤 1、5 使用 `deviceInfoCache`，缓存完整 `things.DeviceInfo` protobuf，内存占用大
+ 流式处理场景（同一网关设备每秒上报），HTTP 开销不可接受

## 2. 目标
+ 所有数据访问走 `caches.Cache`（L1 otter → L2 Redis → L3 HTTP）
+ 热路径零网络开销
+ **所有缓存与 iot 完全统一**：共享 Redis 条目 + NATS 失效事件，能读能写
+ 移除 `deviceInfoCache`，用轻量级 `runtimeMetaCache` 替代

## 3. 缓存架构
### 3.1 runtimeMetaCache（替代 deviceInfoCache，共享 iot Redis）
| 属性 | 值 |
| --- | --- |
| 类型 | `caches.Cache[DeviceRuntimeMeta, device.Core]` |
| KeyType | `"things:device:runtime"`（与 iot 一致） |
| Key | `device.Core{ProductID, DeviceName}` |
| Redis Key | `cache:things:device:runtime:{productID deviceName}` |
| TTL | L1: 24h+10h jitter（实例级随机），L2: 24h 固定 |
| L3 Loader | 调用 runtime-meta-batch HTTP 接口（单设备），映射为 `DeviceRuntimeMeta` |


**在 ekuiper-cluster 中定义同构类型**（JSON tag 与 iot `DeviceRuntimeMeta` 完全一致，不跨项目引用）：

```go
type DeviceRuntimeMeta struct {
    TenantCode              string `json:"tc"`
    ProjectID               int64  `json:"pi"`
    AreaID                  int64  `json:"ai"`
    AreaIDPath              string `json:"aip"`
    ProductID               string `json:"pid"`
    DeviceName              string `json:"dn"`
    SchemaReleaseIDOverride int64  `json:"sro"`
    Version                 string `json:"v"`
    IsOnline                int64  `json:"ol"`
    IsEnable                int64  `json:"ie"`
    Status                  int64  `json:"st"`
    DeviceType              int64  `json:"dt"`
    FirstLogin              int64  `json:"fl"`
    LastBind                int64  `json:"lb"`
    UserID                  int64  `json:"uid"`
    BelongGroup             string `json:"bg"`
    Tags                    string `json:"tg"`
    TimeZone                string `json:"tz,omitempty"`  // 新增：需同步到 iot
}
```

**与 iot 共享 Redis**：

+ KeyType `"things:device:runtime"` + Key `device.Core` → 相同 Redis Key
+ 类型完全兼容，ekuiper-cluster L3 回写不影响 iot 数据
+ iot 触发 NATS 失效 → ekuiper-cluster L1 同步清除

**TimeZone**：需在 iot 侧 `DeviceRuntimeMetaFromModel` 中补充赋值。

**Tags 解析**：`Tags` 是 JSON 编码字符串（如 `{"RS485_ADDR":"1"}`），反序列化后提取 RS485_ADDR。

**EffectiveReleaseID 解析**：按 iot `getSchemaBundleForDevice` 逻辑：

+ `SchemaReleaseIDOverride > 0` → override 作为 releaseID
+ 否则 → 从 `ProductSchemaBundleCurrentCache` 获取 currentReleaseID

### 3.2 currentSchemaCache（99% 设备走此缓存）
| 属性 | 值 |
| --- | --- |
| 类型 | `caches.Cache[SchemaBundle, client.ProductReleaseKey]` |
| KeyType | `"things:schema:product:current"`（与 iot 一致） |
| Key | `client.ProductReleaseKey{ProductID, ReleaseID: 0, Scope}` |
| Redis Key | `cache:things:schema:product:current:{productID}:{scope}`，例如 `cache:things:schema:product:current:p1:periodic_decode` / `cache:things:schema:product:current:p1:active_read` |
| TTL | L1: 24h+10h jitter（实例级随机），L2: 24h 固定 |
| L3 Loader | 调用 `psHttpUrl`（`{productID, scope}`），返回当前 release 的 SchemaBundle |


**与 iot **`NewProductSchemaBundleCurrentCache`** 一致**：

+ Key 不含 releaseID（ReleaseID=0），内部由 L3 解析 currentReleaseID
+ 同一产品所有设备共享一条缓存，命中率极高
+ iot 触发 NATS 失效（产品发布新 release）→ ekuiper L1 清除

### 3.3 releaseSchemaCache（仅 SchemaReleaseIDOverride > 0 的设备走此缓存）
| 属性 | 值 |
| --- | --- |
| 类型 | `caches.Cache[SchemaBundle, client.ProductReleaseKey]` |
| KeyType | `"things:schema:product:release"`（与 iot 一致） |
| Key | `client.ProductReleaseKey{ProductID, ReleaseID, Scope}` |
| Redis Key | `cache:things:schema:product:release:{productID}:{releaseID}:{scope}`，例如 `cache:things:schema:product:release:p1:1001:periodic_decode` / `cache:things:schema:product:release:p1:1001:active_read` |
| TTL | L1: 24h+10h jitter（实例级随机），L2: 24h 固定 |
| L3 Loader | 调用 `psHttpUrl`（`{productID, releaseID, scope}`），返回指定 release 的 SchemaBundle |


**与 iot **`NewProductSchemaBundleReleaseCache`** 一致**。

**在 ekuiper-cluster 中定义同构 SchemaBundle 类型**（与 iot `schema.SchemaBundle` 完全一致，不跨项目引用）：

```go
// SchemaBundle 与 iot 的 schema.SchemaBundle JSON 格式完全一致。
// Go 无 json tag 时使用字段名：Model, Route, Meta。
type SchemaBundle struct {
    Model json.RawMessage                    // iot Model JSON（properties, events, actions）
    Route map[string]SchemaRouteEntry        // identifier → module 路由
    Meta  SchemaBundleMeta                   // release 元数据

    // 以下字段不参与 JSON 序列化（无 json tag，且 Model 用 RawMessage 接收）
    parsedModel *schema.Model                // Fmt 解析后缓存
}

type SchemaRouteEntry struct {
    ModuleID   int64
    ModuleCode string
}

type SchemaBundleMeta struct {
    ProductID string
    ReleaseID int64
}

// Fmt：从 SchemaBundle.Model（RawMessage）解析为 ekuiper-cluster 的 schema.Model
// 在 caches.CacheConfig.Fmt 中使用
func (b *SchemaBundle) ParseModel() error {
    if len(b.Model) == 0 {
        return nil
    }
    model, err := schema.NewSchemaTsl(b.Model)
    if err != nil {
        return err
    }
    b.parsedModel = model
    return nil
}

func (b *SchemaBundle) GetModel() *schema.Model {
    return b.parsedModel
}
```

**序列化兼容性**：

```plain
iot 写入 Redis:
  SchemaBundle{Model, Route, Meta} → json.Marshal → {"Model":{...},"Route":{...},"Meta":{...}}

ekuiper-cluster 读取 Redis:
  {"Model":{...},"Route":{...},"Meta":{...}} → json.Unmarshal → SchemaBundle{Model: RawMessage, Route: map, Meta: struct}
  → Fmt: ParseModel() → parsedModel = schema.Model

ekuiper-cluster 写入 Redis（L3 回写）:
  SchemaBundle{Model: RawMessage, Route: map, Meta: struct} → json.Marshal → {"Model":{...},"Route":{...},"Meta":{...}}
  → 与 iot 格式完全一致 ✓
```

**L3 Loader 返回完整 SchemaBundle**：改造 `psHttpUrl` 网关接口返回 SchemaBundle JSON（而非仅 TSL 字符串）。这样 ekuiper-cluster 回写 Redis 时包含完整 Route + Meta，不会破坏 iot 数据。

### 3.3 deviceGatewaySubsetCache
| 属性 | 值 |
| --- | --- |
| 类型 | `caches.Cache[GatewaySubset, device.Core]` |
| KeyType | `"things:gateway:subset"`（与 iot 一致，保持不变） |
| TTL | 24h base + 10h jitter |


此缓存已与 iot 共享，无需改动，仅调整 TTL 配置。

## 4. 改造后 Lookup 流程
```plain
Lookup(productID, deviceName) 流程：

1. deviceGatewaySubsetCache.GetData(ctx, Core{productID, deviceName})
   → 获取子设备列表 {deviceName: productID}           L1→L2→L3

2. 遍历子设备，逐个调用 runtimeMetaCache.GetData(ctx, Core{subProductID, subDeviceName})
   → 获取 DeviceRuntimeMeta（含 TimeZone、Tags）       L1→L2→L3

3. 遍历子设备，按 iot getSchemaBundleForDevice 相同逻辑获取 schema：
   if meta.SchemaReleaseIDOverride > 0:
       releaseSchemaCache.GetData(ctx, {productID, override, scope})   L1→L2→L3
   else:
       currentSchemaCache.GetData(ctx, {productID, 0, scope})          L1→L2→L3
   → 获取 SchemaBundle → GetModel() 返回 schema.Model

4. 组装结果返回
   → TimeZone 取第一个非空值
   → RS485_ADDR 从 Tags 反序列化获取
```

**缓存命中后的热路径**：

+ 步骤 1: L1 命中 → 0ms
+ 步骤 2: 每个子设备 L1 命中 → 0ms
+ 步骤 4: 每个唯一 release L1 命中 → 0ms
+ **总计：零网络开销**

**对比现状**：每条消息 2 次 HTTP 调用（~50-200ms）

## 5. 涉及文件
### ekuiper-cluster 侧
| 文件 | 操作 | 说明 |
| --- | --- | --- |
| `extensions/impl/cloudx/caches/cache.go` | 修改 | 添加 `ExpireJitter` TTL 抖动 |
| `extensions/impl/cloudx/client/batch_client.go` | 修改 | `ReleaseKeyBatch` → `ProductReleaseKey`；`RuntimeSchemaEntry` 改为返回 SchemaBundle JSON |
| `extensions/impl/cloudx/schema/cache_types.go` | 新增 | `SchemaBundle`、`SchemaRouteEntry`、`SchemaBundleMeta` 同构类型 |
| `extensions/impl/cloudx/device/runtime_meta.go` | 新增 | `DeviceRuntimeMeta` 同构类型 |
| `extensions/impl/cloudx/product/release_schema_cache.go` | 新增 | `GetCurrentSchema()` / `GetReleaseSchema()` / 对应 Release 函数 |
| `extensions/impl/cloudx/device/runtime_meta_cache.go` | 新增 | `GetRuntimeMeta()` / `ReleaseRuntimeMeta()` |
| `extensions/impl/cloudx/ps_lookup.go` | 修改 | 移除 deviceInfoCache + batch HTTP，使用新缓存 |


### iot 侧（配合改动）
#### A. `DeviceRuntimeMeta` 补充 TimeZone
| 文件 | 操作 | 说明 |
| --- | --- | --- |
| `cache/deviceRuntimeMetaCache.go` | 修改 | 结构体增加 `TimeZone string json:"tz,omitempty"` |
| `cache/deviceRuntimeMetaCache.go` | 修改 | `DeviceRuntimeMetaFromModel` 补充 TimeZone 赋值 |


#### B. `api/v1/things/product/auth/schema` 接口改造
**当前接口链路**：

```plain
psHttpUrl → SchemaHandler → SchemaLogic.Schema()
  → RPC ProductSchemaTslRead → ProductSchemaRepo.GetData(productID)
  → 返回 schema.Model JSON（仅 TSL，无 Route/Meta，不区分 release）
```

**改造后**：

**请求扩展**：

```go
type ProductSchemaReq struct {
    // ... 现有字段 ...
    ProductID string `json:"productID"`                 // 必填
    ReleaseID int64  `json:"releaseID,optional"`        // 新增：0=当前release
    Scope     string `json:"scope,optional"`             // 新增：periodic_decode/active_read/full，默认 periodic_decode
}
```

**响应改为 SchemaBundle**：

```go
type ProductSchemaBundleResp struct {
    Model json.RawMessage `json:"model"`  // schema.Model JSON
    Route map[string]any  `json:"route"`  // identifier → {moduleID, moduleCode}
    Meta  any             `json:"meta"`   // {productID, releaseID}
}
```

**Logic 改造**：

```go
func (l *SchemaLogic) Schema(req *types.ProductSchemaReq) (any, error) {
    scope := schema.NormalizeSchemaScope(req.Scope) // 默认 periodic_decode
    if req.ReleaseID > 0 {
        // 指定 release
        return l.svcCtx.ReleaseSchemaBundleCache.GetData(l.ctx,
            cache.ProductReleaseKey{ProductID: req.ProductID, ReleaseID: req.ReleaseID, Scope: scope})
    }
    // 当前 release
    return l.svcCtx.CurrentSchemaBundleCache.GetData(l.ctx,
        cache.ProductReleaseKey{ProductID: req.ProductID, Scope: scope})
}
```

> 注：iot 侧改动不在本次实现范围内。ekuiper-cluster 侧先定义完整结构，iot 端点更新前 schema L3 会返回旧格式 TSL（由 ekuiper-cluster 适配），不影响编译和基本运行。
>

## 6. ps_lookup.go 字段变更
**移除**：

+ `deviceInfoCache` 字段及相关 acquire/release 逻辑
+ `metaBatchConf` / `schemaBatchConf` / `diHttpConf` 字段
+ `config` 中 `metaBatchUrl`、`schemaBatchUrl`、`diHttpUrl`、`HttpPath` 字段

> 缓存共享 iot Redis 后，数据由 iot 写入 L2，ekuiper-cluster 99% 从 L1/L2 读取。L3 极少触发时复用 `dgHttpUrl`（网关子设备）和 `psHttpUrl`（schema 查询）网关配置。
>

**保留 **`psHttpUrl`：作为 `currentSchemaCache` 和 `releaseSchemaCache` 的 L3 回源接口。改造为支持两种请求模式：

```plain
请求: {productID, scope}           → 返回当前 release 的 SchemaBundle
请求: {productID, releaseID, scope} → 返回指定 release 的 SchemaBundle
```

**新增**：

+ `runtimeMetaCache *caches.Cache[DeviceRuntimeMeta, device.Core]`
+ `currentSchemaCache *caches.Cache[SchemaBundle, client.ProductReleaseKey]`
+ `releaseSchemaCache *caches.Cache[SchemaBundle, client.ProductReleaseKey]`

**保留**：

+ `deviceGatewaySubsetCache` — 获取子设备列表（不变）
+ `dgHttpConf` — 网关子设备查询 HTTP 配置（不变）

## 7. 缓存框架改动：TTL 随机抖动
**问题**：固定 TTL 导致同类缓存条目同时过期 → 批量 L3 回源 → DB/HTTP 瞬时压力。

**方案**：在 `CacheConfig` 中新增 `ExpireJitter`，L2 写入时对每条 entry 随机附加抖动时间：

```go
type CacheConfig[dataT any, keyType comparable] struct {
    // ... 现有字段 ...
    ExpireTime   time.Duration // 基础 TTL
    ExpireJitter time.Duration // 最大随机附加时间
}
```

```go
// L1 TTL 在 NewCache 时生成随机值
actualTTL := cfg.ExpireTime + time.Duration(rand.Int63n(int64(cfg.ExpireJitter)))
cache, err := otter.MustBuilder[string, *dataT](10_000).
    WithTTL(actualTTL).  // 实例级随机 TTL
    Build()
```

**L1（otter）**：实例化时生成随机 TTL = `ExpireTime + random(0, ExpireJitter)`，不同 Cache 实例 TTL 不同，避免同类缓存批量过期。

**L2（Redis）**：保持现有机制不变（`SetnxExCtx` + 固定 ExpireTime）。

**各缓存 TTL 配置**：

| 缓存 | ExpireTime | ExpireJitter | L1 TTL（实例级随机） | L2 TTL |
| --- | --- | --- | --- | --- |
| runtimeMetaCache | 24h | 10h | 24-34h | 24h（固定） |
| currentSchemaCache | 24h | 10h | 24-34h | 24h（固定） |
| releaseSchemaCache | 24h | 10h | 24-34h | 24h（固定） |
| deviceGatewaySubsetCache | 24h | 10h | 24-34h | 24h（固定） |


**优势**：

1. 数据不常变，长 TTL 安全
2. 随机抖动避免批量过期导致的缓存击穿
3. L1 短 TTL 保证及时感知 NATS 失效事件

## 8. 内存对比
| 缓存 | 单条大小（估算） | 数量级 | 总计 |
| --- | --- | --- | --- |
| **旧 deviceInfoCache** | ~5-10 KB（完整 protobuf） | 子设备数 N | 5-10 KB × N |
| **新 runtimeMetaCache** | ~200-500 B（轻量元数据） | 子设备数 N | 0.2-0.5 KB × N |
| **releaseSchemaCache** | ~5-20 KB（SchemaBundle） | 唯一 release 数 M | 5-20 KB × M |


其中 M << N（多个子设备共享同一 release 的 schema），内存占用显著降低。

## 9. NATS 失效事件统一
所有缓存订阅相同的 NATS topic 格式 `server.cache.sync.{KeyType}`：

| 缓存 | NATS Topic | 失效触发方 |
| --- | --- | --- |
| runtimeMetaCache | `server.cache.sync.things:device:runtime` | iot（设备信息变更时） |
| currentSchemaCache | `server.cache.sync.things:schema:product:current` | iot（发布新 release 时） |
| releaseSchemaCache | `server.cache.sync.things:schema:product:release` | iot（更新指定 release 时） |
| deviceGatewaySubsetCache | `server.cache.sync.things:gateway:subset` | iot（网关子设备变更时） |


iot 触发 `SetData(nil)` → 发布 NATS 消息 → ekuiper-cluster L1 自动清除 → 下次访问从 L2/DB 重新加载。

