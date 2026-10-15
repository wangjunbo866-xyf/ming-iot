# 模块级 TDengine 超级表优化建议
## 目标
本文是 [2026-04-15-module-level-supertable-design.md](/home/wangjunbo/workspace-aux/doc/2026-04-15-module-level-supertable-design.md) 的补充优化文档。

关联后续优化：

+ [design-category-module-snapshot-release-2026-04-28.md](/home/wangjunbo/workspace-aux/doc/module-level-supertable/design-category-module-snapshot-release-2026-04-28.md) 针对公司 ASN / ASG 高复用场景，明确不新增 `dm_point_catalog` / `dm_schema_profile`，使用 `dm_product_category` 表达物模型族，并把 release 快照从“逐点复制 `DmProductReleaseSchema`”优化为“模块级 JSON 快照 + `DmProductReleaseModule` 引用”。
+ 本文的冷热分层、scope 缓存、`periodic_decode/active_read/full` 设计继续有效；底层 release schema 来源可由 2026-04-28 文档的模块快照方案替代。

主设计文档已经确定以下基线，不在本文中推翻：

+ 运行时 schema 路由固定为：`device.schema_release_id_override ? override : product.current_release_id`
+ `DmDeviceInfo.Version` 只表示固件版本，不再参与 schema 路由
+ `dm_product_info.current_release_id` 表示产品默认运行 release / fleet default，不等同于最新发布 release
+ 运行时唯一版本概念仍是 `DmProductRelease`
+ 属性时序存储主路径仍是“模块超级表”
+ `ekuiper-cluster` 仍按 `effectiveReleaseID` 获取设备运行时 schema

本文只讨论在上述基线之上的优化项，目标是：

+ 降低解码热路径对“大而全 schema”的依赖
+ 避免把大量冷点位推入 TDengine
+ 降低 Redis / 本地缓存 / JSON 反序列化的体积和频率
+ 为后续大规模点位产品提供更清晰的冷热分层方案

---

## 背景判断
当前业务出现了一个新的现实约束：

+ 单产品点位规模可达上千
+ 真正需要周期入 TDengine 的热点位可能只有 200~400 个
+ 其余上千点位大多数只在调试、安装、售后、偶发页面查看或按需诊断时才会读取
+ 当前如果把这些点位全部放进同一份 schema cache，单个产品 schema 可达 `600KB`

这里的核心问题不是 Redis 放不放得下 `600KB`，而是：

+ `acxPushDecode`、`ps_lookup`、`iot` 运行时链路会在热路径反复读取、反序列化这份 schema
+ 本地 L1 缓存、Go 对象展开和 GC 会进一步放大这份内存占用
+ 大量根本不参与周期解码的冷点位，被迫跟着热路径一起加载

结论：

+ `600KB` 作为“管理端全量 schema / 发布快照完整视图”可以接受
+ `600KB` 作为“在线解码热路径的 schema value”不合适

---

## 优化总览
| 优化项 | 目的 | 是否建议进入近期实现 |
| --- | --- | --- |
| 模块级冷热分层（`property_tsdb` / `property_no_store` / `event` / `action`） | 把周期存储、主动读取、事件和动作模块拆开 | 是 |
| schema 多视图缓存（`periodic_decode/active_read/full`） | 让不同算子只拿各自需要的 schema 视图 | 是 |
| 分类复用与模块级 release 快照 | ASN / ASG 多产品共用模型时，避免按产品逐点复制发布快照 | 是，详见 2026-04-28 优化文档 |
| Redis schema view 支持 `scope=periodic_decode/active_read/full` | `acxPushDecode` 读 `periodic_decode`，主动读取读 `active_read`，管理调试读 `full` | 是 |
| 主动读取规则引擎配置 | 按厂家 / 协议维护点读与块读策略 | 是 |
| Modbus 按寄存器块读计划 | 避免按单点逐个下发实时读 | 是，但不预存在 schema cache |
| `active_read` latest cache | 降低重复点读成本 | 是 |
| schema payload 压缩/二进制序列化 | 进一步压缩大 schema | 否，作为后续可选优化 |


---

## 1. 模块级冷热分层
### 1.1 原则
V1 优化不再使用点位级 `CollectMode/StorageClass` 作为主模型。  
改为：

+ **模块级**区分是否进入 TDengine 主链
+ **规则引擎 / 协议配置**决定平台主动读取如何下发

建议模块表增加 `module_kind` 字段：

```go
type DmModuleInfo struct {
    // ... 现有字段保持不变 ...
    ModuleCode string `gorm:"column:module_code;uniqueIndex:idx_dm_module_info_code;type:varchar(100);NOT NULL"`
    ModuleName string `gorm:"column:module_name;type:varchar(100);NOT NULL"`
    // ...
    ModuleKind int64  `gorm:"column:module_kind;type:smallint;default:1"` // 1:property_tsdb 2:property_no_store 3:event 4:action
    // ...
}
```

枚举定义：

```go
type ModuleKind int64

const (
    ModuleKindPropertyTsdb    ModuleKind = 1
    ModuleKindPropertyNoStore ModuleKind = 2
    ModuleKindEvent           ModuleKind = 3
    ModuleKindAction          ModuleKind = 4
)
```

语义：

+ `ModuleKindPropertyTsdb`
    - 参与周期上报解码
    - 参与 TDengine 建表 / 写入 / 历史查询
    - 可被平台主动读取
+ `ModuleKindPropertyNoStore`
    - 不参与周期上报解码
    - 不参与 TDengine 建表 / 写入 / 历史查询
    - 可作为平台主动读取候选点位，但不自动生成读计划
+ `ModuleKindEvent`
    - 进入周期解码 schema，用于 event 定义、参数校验和 payload 组装
    - 不进入 TDengine 属性模块超级表
+ `ModuleKindAction`
    - 只进入完整视图和控制链路
    - 不进入周期解码、主动读取和 TDengine 属性模块超级表

`ModuleKind` 不是普通展示字段，它决定 release 派生视图和 TDengine 物理结构，因此需要作为结构性字段处理：

变更约束：

+ `ModuleKind` 创建后绝对不允许修改，即使模块尚未被产品绑定、尚未发布、尚未创建 TDengine 超级表也不允许原地修改
+ 管理端不提供编辑 `ModuleKind` 的入口；后端 update 接口也必须拒绝 `module_kind` 变更
+ 如果业务确实要调整一个模块的 kind，应新建模块 / 模块版本 / 新 `moduleCode`，再通过新的 product release 生效

存储与读取策略：

+ `ModuleKind` 只存在于 `dm_module_info`，不冗余到 `dm_product_release_schema`
+ 理由：`ModuleKind` 是创建期结构字段，创建后绝对不可修改，因此 `dm_module_info.module_kind` 在模块生命周期内是稳定的
+ cache 视图派生时（发布、预热或回填），通过 `dm_product_release_schema.module_id` join `dm_module_info` 读取 `module_kind`，按 scope 过滤出不同 schema 视图
+ 运行时不需要在 release snapshot 中额外维护 `module_kind` 副本

安全保障：

+ `ModuleKindPropertyTsdb` 表示需要建立对应模块超级表，并进入写入、查询和历史数据语义
+ `ModuleKind` 全生命周期不可变，保证了 `dm_module_info.module_kind` 与任何历史 release 的语义一致
+ current release 切换不会污染已经按精确 `releaseID` 解析的设备运行时结果
+ 避免出现 Redis / 本地缓存仍认为是 `property_no_store`，但 TD 路径已经按 `property_tsdb` 建表或写入的中间状态

### 1.2 canonical 归属规则
需要写死两条边界：

+ 一个点位只属于一个 canonical 模块
+ 不允许为了“主动抄读”再复制第二份点位定义

同一产品下的多个模块不能写入相同 `identifier` 的点位。这里的“不能重复”不是管理端展示限制，而是运行时和 TDengine 路由的硬约束。

原因：

+ release snapshot 的 `Route` 是 `identifier -> moduleCode / physical column` 的一对一路由表；如果同一产品 release 下两个模块都有同名点位，运行时无法确定该 identifier 应落到哪张模块超级表
+ 周期上报通常只携带 `{identifier: value}`，不会携带“这个值属于哪个模块”的上下文；重复 identifier 会导致同一条设备消息在写入时产生歧义
+ TDengine 历史查询以 identifier 作为业务查询入口，重复 identifier 会让查询层不知道应该查 A 模块超级表、B 模块超级表，还是两者合并
+ `PeriodicDecodeSchema`、`ActiveReadSchema`、主动读取规则和 latest cache 都以 identifier 作为业务键；重复 identifier 会导致视图派生、读块计划、解码结果覆盖和 latest cache 回填出现不确定行为
+ 即使两个模块里的点位定义完全相同，也不能视为安全重复；因为模块级超级表的物理位置不同，后续其中一个模块演进字段类型、地址或读写策略时，会破坏另一个模块的语义

因此发布校验必须保证：

+ 同一 product release 内 `identifier` 全局唯一
+ 同一 product release 内 `Route[identifier]` 只能指向一个 canonical module
+ 如果确实存在两个物理寄存器表达同一业务含义，应在模型层给出不同 identifier，再由上层业务做别名、展示名或聚合映射
+ 如果两个产品复用同名 identifier 没问题；唯一性边界是“同一产品的同一 release”，不是全系统全局唯一

也就是说：

+ 某个属性点如果需要存 TD，又偶尔需要主动抄读，它仍然只属于 `property_tsdb` 模块
+ 某个属性点如果只用于主动抄读，不进入周期上报和历史查询，它属于 `property_no_store` 模块
+ 主动读取 scope 是派生视图，不作为第二套人工维护模块，不承担 canonical 真相

### 1.3 主动读取的正确定位
V1 需要单独生成 `ActiveReadSchema`。主动读取不能读取 `FullSchema`，否则会把 event/action、管理端元数据和不相关字段带入低频链路，也会让 scope 语义不清。

原因：

+ `acxThingForward` 需要根据 Nats 消息中的 identifiers，查找点位的 `base-addr`、`vdata_type`、`offset` 等配置，并拼装 Modbus 请求数据
+ `acxThingForwardResponse` 需要根据设备返回的 Modbus 数据块，找到对应点位的 decode config，并解析成平台可读 JSON
+ 主动读取需要的点位集合可能包含 `property_tsdb` 模块中的点，也可能包含 `property_no_store` 模块中的冷点位
+ `FullSchema` 应保留给管理端、发布校验、调试导出和完整展示，不应成为运行时默认读取视图
+ 不同厂家、不同协议、不同功能码的读取策略差异很大
+ Modbus 块合并需要 function code、寄存器类型、地址间隔、最大读取长度、跨空洞策略等厂家规则，不能在 schema cache 中写死

因此 V1 收口为：

+ `PeriodicDecodeSchema` 负责设备定时上报、event 定义校验、TDengine 建表 / 写入 / 历史查询
+ `ActiveReadSchema` 负责主动读取指令拼包和回包解码
+ 主动读取必须由请求中的 identifiers 限定目标点位，不能因为读取 `ActiveReadSchema` 就生成全量点位读计划
+ Modbus 读块计划由 `acxThingForward` / 规则引擎按厂家规则动态生成，不预存在 schema cache
+ `property_no_store` 模块表示“不进入周期解码和 TD 历史”，不等于自动生成主动读取执行计划

`ActiveReadSchema` 是 release snapshot 派生视图，不是预计算读块计划：

```latex
ActiveReadSchema = property_tsdb + property_no_store
```

V1 不增加点位级 `active_read_enabled`。如果某个 `property_no_store` 点位不允许主动读取，应在主动读取入口、权限或规则配置侧限制，不在 schema snapshot 中拆出第三套点位开关。

### 1.4 推荐建模规则
建议：

+ `property_tsdb` 模块：只放真正需要周期解码和历史存储的属性点
+ `property_no_store` 模块：放只用于安装、调试、售后、诊断的主动读取属性点
+ `event` 模块：只放事件定义
+ `action` 模块：只放动作定义
+ 如果一个“业务模块”里冷热点混杂很多，应拆成两个逻辑模块，例如：
    - `common_runtime`：`property_tsdb`
    - `common_diag`：`property_no_store`

这样可以避免：

+ `PeriodicDecodeSchema` 被冷点位撑大
+ TD 路径再做二次筛选
+ `ekuiper` 反复加载几百 KB 的无效 schema

---

## 2. schema 多视图缓存
### 2.1 问题
当前 release schema 是一份“完整视图”：

+ 管理端需要它
+ 发布校验需要它
+ 规则引擎解码也在读它
+ 平台主动读取可能需要点位定义和解码配置，但不应把 Modbus 读块计划放进 schema cache

但这些场景实际需要的数据集合并不相同。

### 2.2 建议 V1 拆成两种逻辑视图
#### FullSchema
用途：

+ 管理端详情
+ 发布校验
+ 产品点位列表
+ 调试导出

内容：

+ release 下全部字段
+ 包括 `property_tsdb`、`property_no_store`、`event`、`action`
+ 保留完整展示元数据

#### PeriodicDecodeSchema
用途：

+ `ekuiper-cluster`
+ `acxPushDecode`
+ 周期上报实时解码
+ event 上报参数校验 / payload 组装
+ TDengine 建表 / 写入 / 历史查询

内容：

+ 包含 `ModuleKindPropertyTsdb` 和 `ModuleKindEvent`
+ 仅保留运行时热路径和 TD 路径必需字段

建议包含：

+ `identifier`
+ `dataType`
+ `define`
+ `moduleCode`
+ `modbus decode config`
+ `Route`

不要带：

+ 管理端 UI 元数据
+ 长文案
+ `property_no_store` 模块
+ `action` 模块

#### ActiveReadSchema
平台主动读取、显式点读、批量点读、诊断读取、读回 Modbus 数据块后的 JSON 解码，统一使用 `ActiveReadSchema` 获取属性定义和 `extendConfig`。

这里需要明确两条边界：

+ 使用 `ActiveReadSchema` 不代表读取全量点位；主动读取请求必须显式携带 identifiers，读块计划只基于本次请求的 identifiers 生成
+ `ActiveReadSchema` 只包含 `property_tsdb + property_no_store`，不包含 `event/action` 和管理端完整展示元数据

也就是说，V1 有三种 schema 视图：

+ `PeriodicDecodeSchema` 面向 `acxPushDecode`、周期解码、event 上报校验和 TDengine
+ `ActiveReadSchema` 面向 `acxThingForward` / `acxThingForwardResponse` 主动读取
+ `FullSchema` 面向管理端完整展示、发布校验和调试导出

`ActiveReadSchema` 不应设计成 `map[identifier]ReadPoint`、`map[blockID]ReadBlock`、预计算 Modbus 读块计划或厂家协议规则配置。它只是从 release snapshot 派生的运行时 schema 视图。

拼包、分块、function code 判断、输入寄存器 / 保持寄存器拆分、地址间隔合并等规则，全部交给 `acxThingForward` / `acxThingForwardResponse` 这类厂家定制逻辑处理，不放入 schema cache。

### 2.3 缓存键建议
逻辑键建议：

+ `things:schema:product:release:{productID}:{releaseID}:full`
+ `things:schema:product:release:{productID}:{releaseID}:periodic_decode`
+ `things:schema:product:release:{productID}:{releaseID}:active_read`
+ `things:schema:product:current:{productID}:full`
+ `things:schema:product:current:{productID}:periodic_decode`
+ `things:schema:product:current:{productID}:active_read`

实现仍然走统一 `caches.Cache`：

+ L1: otter
+ L2: redis
+ L3: relationdb / release snapshot assemble

其中需要特别区分两类语义：

#### release scoped view
`things:schema:product:release:{productID}:{releaseID}:{scope}` 表示一个精确发布快照的派生视图。

特点：

+ `releaseID` 是显式输入
+ value 只由该 release snapshot 派生
+ 不受 `dm_product_info.current_release_id` 后续变化影响
+ 适合设备运行时、`ekuiper-cluster`、TDengine 建表 / 写入 / 查询等已经解析出 `effectiveReleaseID` 的路径

例如：

+ `things:schema:product:release:{productID}:{releaseID}:periodic_decode`
    - 包含 `property_tsdb + event`
    - 是 `ps_lookup(scope=periodic_decode)` / `acxPushDecode` 的目标缓存
    - 不应回退读取 `things:schema:product:current:{productID}:*`
+ `things:schema:product:release:{productID}:{releaseID}:active_read`
    - 包含 `property_tsdb + property_no_store`
    - 是 `acxThingForward` / `acxThingForwardResponse` 主动读取链路的目标缓存
+ `things:schema:product:release:{productID}:{releaseID}:full`
    - 包含 release 下全部字段
    - 是管理端调试、发布校验和完整展示的目标缓存

#### current scoped view
`things:schema:product:current:{productID}:{scope}` 表示产品当前发布指针对应的派生视图。

特点：

+ key 内不包含 `releaseID`
+ 由 `dm_product_info.current_release_id` 解析到当前 release
+ `current_release_id` 变化时只淘汰 current view
+ 适合产品详情、产品点位列表、管理端默认展示等“当前产品视图”
+ 该视图表示产品默认运行视图，不表示最新 release；新 release 发布后，如果未执行 `ProductReleaseSetDefault`，current view 不变化

注意：

+ `things:schema:product:release:{productID}:{releaseID}:periodic_decode` 和 `things:schema:product:current:{productID}:periodic_decode` 不能混用
+ 设备运行时只要已经拿到 `effectiveReleaseID`，后续 schema 读取必须使用 release scoped view
+ `current` 只是“当前指针视图”，不是“某个设备当前正在使用的最终 schema”
+ 设备 `schema_release_id_override` 变化只影响 runtime meta，不应为单设备额外维护 Redis 级 SchemaBundle

### 2.4 目标
建议把热路径的 `PeriodicDecodeSchema` 控制到：

+ 理想：`< 100KB`
+ 可接受：`100KB ~ 200KB`
+ 超过 `300KB` 应继续拆模块或精简 DTO

而：

+ `FullSchema` 达到 `600KB` 仍可接受
+ 主动读取使用 `ActiveReadSchema`，避免把 event/action 和管理端完整元数据带入主动读取链路
+ Modbus 读块计划不进入 schema cache，由规则引擎 / 厂家协议配置动态生成

### 2.5 视图派生逻辑
本节只描述 scope 维度带来的增量。cache 的整体解析流程（releaseID 解析、L1/L2/L3 三级缓存、失效规则等）以主设计文档 §1.2 发布快照匹配规则和 §3.4 Redis Cache 为准。

#### scope 维度的增量
主设计文档的 cache key 形如：

+ `things:schema:product:release:{productID}:{releaseID}`
+ `things:schema:product:current:{productID}`

优化后增加 scope 后缀：

+ `things:schema:product:release:{productID}:{releaseID}:full`
+ `things:schema:product:release:{productID}:{releaseID}:periodic_decode`
+ `things:schema:product:release:{productID}:{releaseID}:active_read`
+ `things:schema:product:current:{productID}:full`
+ `things:schema:product:current:{productID}:periodic_decode`
+ `things:schema:product:current:{productID}:active_read`

cache 框架、失效规则、release 解析流程不变，只是每个 key 从一份 value 变成按 scope 各存一份。

#### L3 回填（relationdb / release snapshot assemble）
scope=full：

```latex
直接查询 dm_product_release_schema WHERE release_id = ?
→ 得到该 release 下全部字段
→ 组装 SchemaBundle{Model, Route}
→ 写入 things:schema:product:release:{productID}:{releaseID}:full
```

scope=periodic_decode：

```latex
1. 查询 dm_product_release_schema 并按 module_id join dm_module_info
2. 根据 dm_module_info.module_kind 筛选 property_tsdb / event
3. 组装周期解码 SchemaBundle
   → 写入 things:schema:product:release:{productID}:{releaseID}:periodic_decode
```

scope=active_read：

```latex
1. 查询 dm_product_release_schema 并按 module_id join dm_module_info
2. 根据 dm_module_info.module_kind 筛选 property_tsdb / property_no_store
3. 组装主动读取 SchemaBundle
   → 写入 things:schema:product:release:{productID}:{releaseID}:active_read
```

`periodic_decode` 可以合并为单条 SQL：

```sql
SELECT prs.*
FROM dm_product_release_schema prs
INNER JOIN dm_module_info mi ON mi.id = prs.module_id AND mi.deleted_time = 0
WHERE prs.release_id = ?
  AND prs.deleted_time = 0
  AND mi.module_kind IN (ModuleKindPropertyTsdb, ModuleKindEvent)
```

current scoped view 的派生逻辑相同，只是 releaseID 由 `dm_product_info.current_release_id` 解析而来。

#### 运行时读取路径
与主设计文档 §1.2 一致，不因 scope 改变：

```latex
设备上报 / 查询到达
  → 读取 cache:things:device:runtime:{productID}:{deviceName}
  → 解析 effectiveReleaseID：
      schema_release_id_override > 0 ? override : current_release_id
  → 按 (productID, effectiveReleaseID, scope) 读取 release scoped schema view
      周期解码 / TD 写入 → scope=periodic_decode
      主动读取 → scope=active_read
      管理端 / 发布校验 / 调试导出 → scope=full
```

关键约束：

+ `periodic_decode` 的 `Route` 只包含 `property_tsdb` 模块的 identifier；event 定义只用于事件校验和 payload 组装
+ `active_read` 包含 `property_tsdb + property_no_store`，不包含 event/action
+ `FullSchema` 的 `Route` 包含全部模块的 identifier
+ scope 缺省值为 `periodic_decode`，用于兼容旧调用方

---

## 3. 规则引擎与主动读取的运行时接口
### 3.1 基本要求
`ps_lookup` 当前已经改成按 `effectiveReleaseID` 拉 schema。  
下一步应继续收口为：

+ `ps_lookup` 热路径直接读取本地 L1 / Redis 中的设备运行时元信息，并由 iot 服务端按产品 current 计算有效 release：
    - `schemaReleaseIDOverride`
    - `version`
    - `timeZone`
    - `rs485Addr`
+ `ps_lookup` 热路径直接按 `(productID, releaseID, scope)` 读取 Redis 中的 release scoped schema view
+ `runtime-schema-batch` 可以保留为管理、调试或非热路径接口，但不作为 `ps_lookup` 的海量设备热路径依赖

逻辑请求项：

```json
{
  "items": [
    {"productID": "p1", "releaseID": 1001, "scope": "periodic_decode"},
    {"productID": "p1", "releaseID": 1001, "scope": "active_read"}
  ]
}
```

### 3.2 scope 设计
建议支持：

+ `scope=full`
+ `scope=periodic_decode`
+ `scope=active_read`

规则：

+ `acxPushDecode` 默认只请求 `scope=periodic_decode`
+ `acxThingForward` / `acxThingForwardResponse` 主动读取使用 `scope=active_read`
+ 管理端调试、发布校验、完整展示使用 `scope=full`
+ 平台主动读取虽然读取 `active_read`，但执行计划必须只覆盖请求 identifiers，不允许生成全量点位读计划

实现约束：

+ `ps_lookup` 内部 schema 请求结构必须显式携带 `scope`
+ `(productID, releaseID, scope)` 是一次请求项的完整语义，不允许服务端忽略 `scope`
+ `scope` 缺省值可以为 `periodic_decode`，用于兼容旧 `ekuiper` 调用方，但服务端日志应能识别缺省请求，便于后续清理
+ `scope=periodic_decode` 只允许返回 `things:schema:product:release:{productID}:{releaseID}:periodic_decode`
+ `scope=active_read` 只允许返回 `things:schema:product:release:{productID}:{releaseID}:active_read`
+ `scope=full` 只允许返回 `things:schema:product:release:{productID}:{releaseID}:full`

在 `ekuiper-cluster` 切换前，必须先完成：

+ Redis release scoped schema view 支持 `scope`
+ dmsvr 发布 / 失效链路负责把 release scoped view 写入或回填到 Redis
+ `ps_lookup` 去重维度从 `(productID, releaseID)` 扩展为 `(productID, releaseID, scope)`
+ `ps_lookup` 请求显式传 `scope=periodic_decode`
+ 主动读取规则显式传 `scope=active_read`

否则即使文档上已经拆出 `periodic_decode`，实际运行时仍可能继续返回 full schema，无法达到降低热路径 schema payload 的目标。

### 3.3 本地缓存建议
`ekuiper-cluster` 本地缓存键改为：

+ `(productID, releaseID, scope)`

不要再缓存：

+ `(productID, deviceName) -> full schema`

原因：

+ 设备数基数过大
+ 同 release 的设备天然可复用
+ `periodic_decode`、`active_read`、`full` 三类用途不同，不应共用一份缓存 value

### 3.4 ps_lookup 需要 scope 化
当前 `ps_lookup` 如果仍然只有一个逻辑 lookup，并且规则统一这样关联：

```sql
SELECT atf(local_event_bus_south_thing.*, meta(explain), pslookup.subDevicesSchema, meta(deviceName))
FROM local_event_bus_south_thing
FULL JOIN pslookup
ON meta(SN) = pslookup.deviceName
AND pslookup.productID = "col01"
```

那么 `pslookup.subDevicesSchema` 的 scope 会变成所有使用方共享的隐含全局选择。

这会带来问题：

+ `acxPushDecode` 处理设备规律性上报时，需要 `things:schema:product:release:{productID}:{releaseID}:periodic_decode`
+ `acxThingForward` 和 `acxThingForwardResponse` 处理主动读取 / 回包时，需要 `things:schema:product:release:{productID}:{releaseID}:active_read`
+ 如果同一个 `pslookup` 同时返回 `full`，周期上报每次执行 `acxPushDecode` 也会加载 full schema
+ 如果同一个 `pslookup` 只返回 `periodic_decode`，主动读取链路又拿不到 `property_no_store` 冷点位

因此 `ps_lookup` 必须 scope 化，不能再作为一个无 scope 的通用 schema lookup。

这里的“拆出两个 lookup”不是拆两套代码：

+ 仍然共用同一个 `pslookup` 插件类型和同一个 `ProductSchemaLookupSource` 实现
+ `ProductSchemaLookupSource` 增加配置项 `scope`
+ 规则配置中创建两个不同名字的 lookup 实例 / lookup table
+ `pslookup_periodic_decode` 的配置固定 `scope=periodic_decode`
+ `pslookup_active_read` 的配置固定 `scope=active_read`
+ 两个实例的 Redis、NATS、HTTP URL 配置可以完全相同
+ lookup 返回结构保持兼容，仍返回 `subDevicesSchema / subDevicesInfo / timeZone`
+ 区别只在内部读取 Redis schema view 时使用不同 scope
+ 本地缓存 key、Redis schema key 和请求去重 key 都必须包含 scope，因此同一 `(productID, releaseID)` 的两个实例不会误复用 schema value

示意：

```yaml
pslookup_periodic_decode:
  type: pslookup
  scope: periodic_decode
  dgHttpUrl: http://iot/api/v1/things/device/auth/gateway-subset
  metaBatchUrl: http://iot/api/v1/things/device/auth/runtime-meta-batch
  psHttpUrl: http://iot/api/v1/things/product/auth/schema

pslookup_active_read:
  type: pslookup
  scope: active_read
  dgHttpUrl: http://iot/api/v1/things/device/auth/gateway-subset
  metaBatchUrl: http://iot/api/v1/things/device/auth/runtime-meta-batch
  psHttpUrl: http://iot/api/v1/things/product/auth/schema
```

规则使用边界：

+ `acxPushDecode` 所在规则只 join `pslookup_periodic_decode`
+ `acxThingForward` 所在规则只 join `pslookup_active_read`
+ `acxThingForwardResponse` 所在规则只 join `pslookup_active_read`
+ 本地缓存 key、Redis schema key 和请求去重 key 都必须包含 scope

这样可以保证：

+ 规律性上报不会触发 full / active_read schema 读取
+ 主动读取不会污染 `PeriodicDecodeSchema` 热路径缓存
+ 两条链路仍可复用同一套 Redis / NATS / HTTP 能力和设备运行时元信息缓存；schema value 通过不同 `scope` key 隔离

### 3.5 热路径禁止 HTTP / DB 回源
海量设备场景下，`ps_lookup` 不应把 HTTP / DB 作为缓存 miss 的兜底路径。

禁止模式：

+ L1 miss
+ Redis miss 或 Redis 访问失败
+ `ps_lookup` 通过 HTTP 调 dmsvr
+ dmsvr 回源关系库组装 schema / runtime meta

这个模式在小流量下看似提高可用性，但在海量设备规律性上报时会把 Redis 故障、缓存冷启动或局部 miss 放大成 HTTP 风暴和数据库风暴。

`ps_lookup` 的热路径应采用：

+ 优先读进程内 L1
+ L1 miss 后读 Redis
+ Redis miss / Redis error 时，不回源 HTTP / DB
+ 可选：短时间使用 L1 stale value
+ 无 stale 可用时返回明确错误或空 schema，并打点告警

配套要求：

+ schema release view 由 dmsvr 发布链路、预热任务或后台回填任务写入 Redis
+ device runtime meta 由设备档案变更、绑定关系变更、release override 变更等事件维护 Redis
+ `ps_lookup` 只消费 Redis 中已经准备好的运行时数据
+ Redis 故障恢复后，由后台任务或下一次非热路径预热补齐数据，不由设备上报热路径触发 DB 回源

也就是说，`ps_lookup` 是热路径读取器，不是缓存构建器。

### 3.6 Redis 重启恢复语义
`ps_lookup` 不应因为 Redis 短暂不可用而主动 stop rule，也不需要为每条规则实现一套自定义 Redis 重连循环。

运行时建议按以下语义处理：

+ Redis client / connection pool 作为长生命周期对象复用
+ Redis 连接断开或 Redis 重启期间，单次 Redis 读写会返回错误
+ 后续 Redis 命令应继续使用同一个 client，由底层 Redis client / connection pool 在下一次命令时重新建连
+ Redis 恢复后，只要地址、认证、DB 配置不变，后续 lookup 应能继续正常读取
+ rule 不需要因为 Redis 重启而整体重建，除非 source connect 阶段依赖的其他组件也不可用，或实现中把 Redis 错误错误地升级成了不可恢复状态

Redis 不可用期间的行为：

+ L1 命中：可以继续返回已有本地缓存
+ L1 miss + Redis error：不回源 HTTP / DB
+ 有可接受的 L1 stale value：可短时间返回 stale，并打 `ps_lookup_schema_stale_hit`
+ 无 stale value：返回明确 lookup error 或空 schema，由规则侧按失败处理，并打 `ps_lookup_schema_redis_error`

Redis 重启成功后的行为需要区分“连接恢复”和“数据恢复”：

+ 如果 Redis 开启持久化或主从切换后 schema / runtime meta 仍存在，后续 lookup 会随着 Redis client 重新建连恢复正常
+ 如果 Redis 重启后数据为空，连接可以恢复，但 `things:schema:product:release:{productID}:{releaseID}:{scope}` 和设备 runtime meta 会变成 Redis miss
+ Redis miss 仍然不能由 `ps_lookup` 热路径触发 HTTP / DB 回源
+ 数据为空时，应依赖 dmsvr 发布链路、预热任务、后台回填任务或运维恢复流程重新写入 Redis
+ 回填完成前，rule 可以保持运行，但相关设备消息会继续走 miss / error / stale 逻辑，不能保证完整解码

因此最终目标是：

+ Redis 短暂重启不要求 rule stop / restart
+ Redis 恢复且数据仍在时，rule 自动恢复正常读取
+ Redis 恢复但数据丢失时，rule 继续运行，但必须等 schema view 和 runtime meta 被重新预热后才能恢复完整业务结果
+ 监控必须区分 `redis_error` 和 `redis_miss`，否则无法判断是连接故障还是缓存数据缺失

---

## 4. 主动读取的规则引擎方案
### 4.1 目标
对于上千个冷点位：

+ 不进入周期解码
+ 不进入 TDengine 建表
+ 但要支持平台主动抄读、指令拼装和结果解码

同时还要允许：

+ 一部分 `Tsdb` 模块里的点，也出现在主动读取场景中
+ 不同厂家设备使用不同主动读取策略

### 4.2 推荐读取流程
```latex
页面/规则/诊断发起点读
    → DeviceRuntimeMetaCache 取设备信息
    → 解析 effectiveReleaseID
    → 读取 scope=active_read 的 ActiveReadSchema
    → 根据目标 identifiers 获取点位定义 / decode config / 协议分类字段
    → acxThingForward / 规则引擎按厂家 / 协议 / function code 生成 Modbus 读块
    → 对读块去重 / 合并 / 截断
    → 下发 Modbus 块读取
    → acxThingForwardResponse 按 ActiveReadSchema 中的点位 decode config 解码回 JSON
    → 返回当前值
    → 可选：写入短 TTL active_read latest cache
```

### 4.3 Modbus 读块规则建议
不要按单点逐个读：

+ Modbus 单点读会放大 RTT
+ 上千点位会把设备和网关打爆
+ 同一地址段点位天然应合并读取

读块计划不放在 schema cache 中，建议放在规则引擎 / 协议配置中维护。

规则示例：

+ 不同厂家可维护不同规则模板
+ 按协议类型分组，例如 `modbus`
+ 按 function code 分组，输入寄存器和保持寄存器必须分开读取
    - `04` input register 单独成组
    - `03` holding register 单独成组
+ 同一 function code 下，按寄存器地址排序
+ 相邻点位地址差距不超过配置阈值时可以合并为一个块，例如 `80 byte`
+ 超过阈值、跨 function code、跨寄存器类型、跨设备地址时必须拆块
+ 每个块仍需受设备最大读取长度限制，超过设备上限时继续拆块
+ 空洞合并策略必须可配置，不同厂家设备对跨空洞读取的容忍度可能不同
+ 字节序、字序、寄存器单位换算、特殊偏移规则由厂家协议配置维护

也就是说，执行计划是“读块”，不是“读模块”，更不是“读全量点位”。

V1 不要求在 release schema 中预生成读块；规则引擎可以在点读请求时根据目标 identifiers 和厂家规则动态生成读块。

### 4.4 主动读取 latest cache
建议新增：

+ `device:thing:property:active_read:last:{productID}:{deviceName}`

特点：

+ 仅缓存主动抄读结果
+ 短 TTL，建议 `1m ~ 10m`
+ 不作为历史数据源
+ 不替代实时读，只用于降低短时间重复读取成本

### 4.5 查询语义
需要明确：

+ `NoStore` 模块下的点位默认**不支持历史查询**
+ 历史 API 若请求到 `NoStore` 模块点位，应直接返回“不支持历史”
+ `Tsdb` 模块点位即使被主动读取，也不代表“主动读取结果自动写 TD 历史”
+ 若后续业务确实要求“手动抄读也落一条历史”，应单独设计开关，不作为默认行为

---

## 5. 写路径与 TDengine 路径优化
### 5.1 TD 路径只看 periodic_decode 中的 property_tsdb
运行时必须写死：

+ TDengine 建表：只基于 `periodic_decode` scope 中的 `property_tsdb`
+ TDengine 写入：只基于 `periodic_decode` scope 中的 `property_tsdb`
+ 历史查询：只基于 `periodic_decode` scope 中的 `property_tsdb`
+ `property_no_store`、`event`、`action` 完全不进入 TD 属性模块超级表路径

这样可以避免：

+ 在 TD 热路径里每次再按点位筛选
+ 把本不该入 TD 的点误写进 SQL

### 5.2 平台主动抄读与 TD 写入解耦
需要明确：

+ 平台主动读取使用 `ActiveReadSchema` 获取点位级拼包 / 解码配置
+ 平台主动读取使用规则引擎 / 协议读配置生成读块
+ 主动读取回包解码使用 `ActiveReadSchema` 中的点位 decode config
+ 即使某个点属于 `property_tsdb` 模块，主动读取默认也只是“当前值读取”，不自动写 TD 历史

这样可以避免：

+ 主动抄读与周期上报双重写历史
+ 因人工操作放大 TD 写入量

### 5.3 发布校验
发布时应增加以下校验：

+ `PeriodicDecodeSchema` 点位总数
+ `PeriodicDecodeSchema` 最坏 payload 大小
+ `ActiveReadSchema` 点位总数
+ `ActiveReadSchema` 最坏 payload 大小
+ 同一 product release 内 identifier 唯一
+ `ModuleKindPropertyTsdb` 模块是否都具备 TDengine 必需字段
+ `ActiveReadSchema` 中主动读取可能依赖的 decode config 做基础格式校验，不在发布阶段强行生成 Modbus 读块

建议阈值：

+ 若 `PeriodicDecodeSchema` 体积超过目标值，应优先考虑拆模块
+ 若 `ActiveReadSchema` 体积超过目标值，应优先考虑拆 `property_no_store` 模块或精简主动读取 DTO
+ 若规则引擎后续发现某厂家点读无法聚合成较少读块，应在规则配置侧给出告警，不阻断 schema 发布

---

## 6. 缓存与序列化优化
### 6.1 当前建议
先做“视图拆分”，再考虑“序列化压缩”。  
不要在一份 600KB full schema 上直接上 gzip/二进制协议来掩盖模型问题。

优先级应是：

1. 先拆 `FullSchema / PeriodicDecodeSchema / ActiveReadSchema`
2. 再观察 `PeriodicDecodeSchema` 的体积和反序列化成本
3. 再观察主动读取对 `ActiveReadSchema` 的读取频率和反序列化成本
4. 如 schema payload 仍偏大，再考虑压缩或更紧凑 DTO

### 6.2 可选的后续优化
仅在以下场景再考虑：

+ `PeriodicDecodeSchema` 拆分后仍然偏大
+ `ps_lookup` Redis schema value 反序列化 P95/P99 受 payload 影响明显
+ `ekuiper` L1 缓存内存仍然偏高
+ 主动读取频率升高，`ActiveReadSchema` 读取成本开始影响 P95/P99
+ 主动读取规则侧生成的读块数量过多，影响设备响应和网关吞吐

可选方案：

+ Redis value 压缩存储
+ 内部 DTO 使用更紧凑结构
+ 对 `PeriodicDecodeSchema` / `ActiveReadSchema` 单独定义轻量序列化结构
+ 对主动读取规则做厂家级模板、块合并阈值和最大读取长度优化

这部分不建议进入当前第一阶段开发。

---

## 7. 监控与验收指标
### 7.1 schema 体积指标
建议新增：

+ `schema_full_bytes`
+ `schema_tsdb_bytes`

维度：

+ `productID`
+ `releaseID`

### 7.2 运行时指标
建议新增：

+ `ps_lookup_release_group_count`
+ `ps_lookup_schema_l1_hit`
+ `ps_lookup_schema_redis_hit`
+ `ps_lookup_schema_redis_miss`
+ `ps_lookup_schema_redis_error`
+ `ps_lookup_schema_stale_hit`
+ `runtime_schema_bytes`
+ `active_read_block_count`
+ `active_read_identifier_count`
+ `active_read_latest_cache_hit`
+ `active_read_latest_cache_miss`

### 7.3 目标
建议目标：

+ 单次网关消息下 schema 请求次数接近 release 组数，而不是子设备数
+ `PeriodicDecodeSchema` 显著小于 `FullSchema`
+ `property_no_store` 模块不进入周期解码热路径
+ 主动读取使用 `ActiveReadSchema`，但不污染 `PeriodicDecodeSchema` 热路径缓存
+ 大量冷点位不再进入 TDengine 列定义

---

## 8. 推荐实施顺序
### 第一阶段
+ 在模块表增加 `ModuleKind`
+ 增加 `ModuleKind` 变更约束：创建后绝对不允许修改 kind，管理端和后端 update 都必须拒绝
+ 明确 `property_tsdb`、`property_no_store`、`event`、`action` 模块的边界
+ 发布时生成三种 release 视图：
    - `FullSchema`
    - `PeriodicDecodeSchema`
    - `ActiveReadSchema`
+ release scoped cache key 支持 `things:schema:product:release:{productID}:{releaseID}:{scope}`
+ current scoped cache key 支持 `things:schema:product:current:{productID}:{scope}`
+ Redis schema view 支持 `scope=periodic_decode/active_read/full`，并按 `(productID, releaseID, scope)` 读取 release scoped view
+ `ps_lookup` 热路径改为只读 L1 / Redis，禁止 Redis miss 后 HTTP / DB 回源
+ dmsvr 发布链路或后台预热任务负责写入 / 回填 Redis schema view
+ `ProductSchemaLookupSource` 增加固定 `scope` 配置
+ 规则层用同一个 `pslookup` 插件类型创建两个不同名字的 lookup 实例：`pslookup_periodic_decode(scope=periodic_decode)` / `pslookup_active_read(scope=active_read)`，底层复用同一份 `ProductSchemaLookupSource` 代码
+ `acxPushDecode` 规则只 join `pslookup_periodic_decode`
+ `acxThingForward` / `acxThingForwardResponse` 规则只 join `pslookup_active_read`
+ `ekuiper` 本地缓存键同步改为 `(productID, releaseID, scope)`

### 第二阶段
+ 新增可维护的协议读配置 / Modbus 读计划配置，不预存在 schema cache
+ 规则引擎支持按厂家 / 协议 / function code 生成点读计划
+ 支持输入寄存器和保持寄存器分开读取
+ 支持同 function code 下按地址差距阈值合并读块，例如不超过 `80 byte` 合并
+ 支持按设备最大读取长度继续拆块
+ 引入 `active_read:last` 短 TTL 缓存

### 第三阶段
+ 如仍有性能压力，再评估压缩/轻量序列化
+ 如主动读取规则生成的读块过碎，再优化厂家规则模板和块合并策略

---

## 9. 一句话结论
当前主设计可以继续成立，但对于“单产品上千点位、其中大多数只是偶尔读取”的场景，优化方向应收口为：

+ canonical 真相仍按模块维护
+ 模块按 `ModuleKind` 区分 `property_tsdb`、`property_no_store`、`event`、`action`
+ `acxPushDecode` 只加载 `PeriodicDecodeSchema`，包含 `property_tsdb + event`
+ 平台主动读取加载 `ActiveReadSchema`，包含 `property_tsdb + property_no_store`，并且只针对请求 identifiers 生成读块
+ `FullSchema` 只服务管理端、发布校验、调试导出和完整展示
+ 主动读取由 `acxThingForward` / 规则引擎按厂家、function code 和地址合并策略生成 Modbus 读块

