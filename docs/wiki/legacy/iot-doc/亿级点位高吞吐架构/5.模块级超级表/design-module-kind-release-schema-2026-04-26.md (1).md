# ModuleKind 与发布快照视图设计
## 目标
本文是 `design-module-level-supertable-2026-04-15.md` 和 `design-module-level-supertable-optimization-2026-04-23.md` 的补充设计。

关联后续优化：

+ [design-category-module-snapshot-release-2026-04-28.md](/home/wangjunbo/workspace-aux/doc/module-level-supertable/design-category-module-snapshot-release-2026-04-28.md) 针对公司 ASN / ASG 高复用场景，明确不新增 `dm_point_catalog` / `dm_schema_profile`，使用 `dm_product_category` 表达物模型族。
+ 本文关于“`DmProductReleaseSchema` 逐行冻结字段明细、不新增 `DmProductReleaseModule`”的结论，是通用逐行快照基线；在 ASN / ASG 多产品共用同一模块点位的场景下，由 2026-04-28 文档的“模块级 JSON 快照 + `DmProductReleaseModule` 引用”覆盖。
+ `ModuleKind` 的核心约束不变：只存于 `dm_module_info`，创建后绝对不可修改，不冗余到 `DmProductReleaseSchema`、`DmModuleSchemaSnapshot` 或 `DmProductReleaseModule`。

要解决的问题：

+ 模块不应混合承载 `property` / `event` / `action`
+ event/action 不应写入 property 模块超级表
+ property 模块要区分是否进入 TDengine
+ 在逐行快照基线下，不新增 `DmProductReleaseModule`
+ 在逐行快照基线下，已发布 release 的字段集合和模块归属由 `DmProductReleaseSchema` 决定
+ 模块类型由不可变的 `DmModuleInfo.ModuleKind` 决定，不冗余到 `DmProductReleaseSchema`

最终结论：

+ 用 `ModuleKind` 合并“物模型能力类型”和“存储策略”
+ `DmModuleInfo.ModuleKind` 是模块创建时确定的结构性分类，创建后绝对不允许修改
+ 逐行快照基线下，发布时只把 `module_id / module_code / identifier / DmSchemaCore` 冻结到 `DmProductReleaseSchema`
+ 逐行快照基线下，派生 `periodic_decode / active_read / full` 视图时，通过 `DmProductReleaseSchema.module_id` join `DmModuleInfo.ModuleKind` 过滤
+ `DmProductModule` 继续只做管理端草稿绑定，不参与运行时
+ 逐行快照基线下不引入 `DmProductReleaseModule`；高复用场景采用 2026-04-28 模块快照方案时可以引入，且不形成两份 release 真相

---

## 1. 设计原则
### 1.1 模块只承载一种物模型能力
一个模块只能属于以下一种类型：

+ property
+ event
+ action

不允许一个模块同时定义 property 和 event，或同时定义 event 和 action。

原因：

+ property 是连续状态采样，适合列式时序存储
+ event 是离散发生记录，适合事件日志
+ action 是控制能力定义，执行结果走 send/action log
+ 三者生命周期、数据结构、查询方式都不同，混在一个模块里会让运行时路由和管理端语义变复杂

### 1.2 TDengine 只承载 property_tsdb
TDengine 模块超级表只用于 `property_tsdb` 模块。

不进入 TDengine 的类型：

+ `property_no_store`
+ `event`
+ `action`

event/action 可以按模块管理和发布，但数据不写入 `model_module__{moduleCode}`。

### 1.3 运行时真相拆分
发布后，运行时不读取 `DmProductModule`。

原因：

+ `DmProductModule` 是产品草稿绑定，管理端会继续编辑
+ 如果运行时回查 `DmProductModule`，老 release 语义会被后续草稿修改污染
+ `DmProductReleaseSchema` 已经冻结了 `release_id + module_id + module_code + identifier`
+ `DmModuleInfo.ModuleKind` 创建后不可修改，因此运行时按 `module_id` join 读取它不会破坏历史 release 语义

所以：

+ `DmProductModule` = draft only
+ `DmProductReleaseSchema` = release 字段集合和模块归属的运行时真相
+ `DmModuleInfo.ModuleKind` = 模块类型的全局不可变真相

---

## 2. ModuleKind 定义
### 2.1 枚举
```go
type ModuleKind int64

const (
    ModuleKindPropertyTsdb    ModuleKind = 1
    ModuleKindPropertyNoStore ModuleKind = 2
    ModuleKindEvent           ModuleKind = 3
    ModuleKindAction          ModuleKind = 4
)
```

### 2.2 语义
| ModuleKind | 定义内容 | 是否周期解码 | 是否写 TDengine | 是否支持历史查询 | 数据存储路径 |
| --- | --- | --- | --- | --- | --- |
| `property_tsdb` | 属性点位 | 是 | 是 | 是 | 模块超级表 |
| `property_no_store` | 属性点位 | 否 | 否 | 否 | 主动读取 / 当前值 |
| `event` | 事件定义 | 否 | 否 | 走事件日志能力 | event log |
| `action` | 动作定义 | 否 | 否 | 走控制日志能力 | send/action log |


注意：

+ `property_tsdb` 也可以被平台主动读取
+ `property_no_store` 只用于安装、调试、售后、诊断等低频主动读取
+ `event/action` 不是 TDengine 属性列，不进入 `property_tsdb` 的 TDengine 路由；其中 `event` 会进入 `periodic_decode` scope 用于事件校验和 payload 组装

---

## 3. SQL 模型调整
### 3.1 DmModuleInfo 增加 ModuleKind
```go
type DmModuleInfo struct {
    ID         int64  `gorm:"column:id;type:bigint;primary_key;AUTO_INCREMENT"`
    ModuleCode string `gorm:"column:module_code;uniqueIndex:idx_dm_module_info_code;type:varchar(100);NOT NULL"`
    ModuleName string `gorm:"column:module_name;type:varchar(100);NOT NULL"`
    ModuleKind int64  `gorm:"column:module_kind;type:smallint;default:1"` // 1:property_tsdb 2:property_no_store 3:event 4:action
    // 其他字段保持不变
}
```

`DmModuleInfo.ModuleKind` 是模块创建时确定的结构性分类，不是普通配置项。

变更约束：

+ `ModuleKind` 创建后绝对不允许修改，即使模块尚未被产品绑定、尚未发布、尚未创建 TDengine 超级表也不允许原地修改
+ 管理端不提供编辑 `ModuleKind` 的入口；后端 update 接口也必须拒绝 `module_kind` 变更
+ 如果要从 `property_no_store` 改成 `property_tsdb`，应创建新模块或新 `moduleCode`，再通过新 release 生效
+ 如果要从 `property_tsdb` 改成 `event/action`，应视为新模块，不允许原地修改

### 3.2 DmProductReleaseSchema 不冗余 ModuleKind
逐行快照基线下不新增 `DmProductReleaseModule`。  
`DmProductReleaseSchema` 不增加 `module_kind` 字段。

```go
type DmProductReleaseSchema struct {
    ID         int64  `gorm:"column:id;type:bigint;primary_key;AUTO_INCREMENT"`
    ReleaseID  int64  `gorm:"column:release_id;uniqueIndex:idx_dm_product_release_schema_identifier;uniqueIndex:idx_dm_product_release_schema_module_identifier;type:bigint;NOT NULL"`
    ProductID  string `gorm:"column:product_id;index:idx_dm_product_release_schema_product;type:varchar(100);NOT NULL"`
    ModuleID   int64  `gorm:"column:module_id;uniqueIndex:idx_dm_product_release_schema_module_identifier;index:idx_dm_product_release_schema_module;type:bigint;NOT NULL"`
    ModuleCode string `gorm:"column:module_code;index:idx_dm_product_release_schema_module;type:varchar(100);NOT NULL"`
    Identifier string `gorm:"column:identifier;uniqueIndex:idx_dm_product_release_schema_identifier;uniqueIndex:idx_dm_product_release_schema_module_identifier;type:varchar(100);NOT NULL"`
    DmSchemaCore
    stores.NoDelTime
    DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:idx_dm_product_release_schema_identifier;uniqueIndex:idx_dm_product_release_schema_module_identifier"`
}
```

不冗余原因：

+ `ModuleKind` 是 `DmModuleInfo` 的创建期结构字段，绝对不允许修改，不存在“历史 release 被后续修改污染”的风险
+ 每条 release schema 都冗余 `module_kind` 会产生大量重复字段，且容易让实施者误以为 release 内可以覆盖模块类型
+ 视图派生、TDengine 建表、写入和查询需要 `module_kind` 时，统一通过 `dm_product_release_schema.module_id = dm_module_info.id` 获取
+ 运行时仍然不读取 `DmProductModule`，避免草稿绑定污染已发布 release

### 3.3 DmProductModule 保持草稿语义
`DmProductModule` 不增加运行时职责。

用途：

+ 管理端编辑产品草稿
+ 选择产品下一版要包含哪些模块
+ 逐行快照基线下，发布时读取它，复制生成 `DmProductReleaseSchema`
+ 高复用模块快照方案下，发布时读取它，生成 `DmModuleSchemaSnapshot + DmProductReleaseModule`

禁止：

+ 设备上报时读取 `DmProductModule`
+ TDengine 写入时读取 `DmProductModule`
+ 历史查询时读取 `DmProductModule`
+ `ekuiper` schema lookup 读取 `DmProductModule`

---

## 4. 发布流程
### 4.1 发布时读取并校验 ModuleKind
```latex
ProductReleasePublish(productID, releaseNo)
    -> 读取 DmProductModule 草稿
    -> 读取 DmModuleInfo，拿到 module_id / module_code / module_kind
    -> 读取 DmModuleSchemaInfo，拿到模块点位定义
    -> 校验同一 release 下 identifier 唯一性
    -> 校验模块 kind 与点位定义类型匹配
    -> 创建 DmProductRelease
    -> bulk insert DmProductReleaseSchema
       每条记录只冻结 module_id / module_code / identifier / DmSchemaCore
    -> 对 property_tsdb 模块准备 TDengine STABLE
    -> 更新 current_release_id
```

注意：

+ 发布流程只使用 `ModuleKind` 做校验和 TDengine 准备，不把它复制进 `DmProductReleaseSchema`
+ 后续派生 scope 视图时继续通过 `module_id` join `DmModuleInfo` 读取 `ModuleKind`

### 4.2 发布校验
必须校验：

+ `property_tsdb` 模块只能包含 property 定义
+ `property_no_store` 模块只能包含 property 定义
+ `event` 模块只能包含 event 定义
+ `action` 模块只能包含 action 定义
+ 同一 release 的 property namespace 下，`identifier` 全局唯一
+ 同一 release 的 event namespace 下，`identifier` 全局唯一
+ 同一 release 的 action namespace 下，`identifier` 全局唯一

说明：

+ `property_tsdb` 和 `property_no_store` 同属 property namespace，因此两者之间的 property identifier 也不能重复
+ event/action 是否允许与 property 复用同名 identifier，取决于协议是否天然区分 namespace；V1 建议管理端禁止同名，降低排查成本

---

## 5. schema 派生视图与 cache scope
### 5.1 FullSchema
来源：

+ `DmProductReleaseSchema`

包含：

+ `property_tsdb`
+ `property_no_store`
+ `event`
+ `action`

用途：

+ 管理端详情
+ 发布校验
+ 调试导出
+ 低频完整查看

缓存 key：

```latex
things:schema:product:release:{productID}:{releaseID}:full
things:schema:product:current:{productID}:full
```

`full` 不作为高频运行时默认 scope。

### 5.2 PeriodicDecodeSchema
过滤条件：

```sql
select prs.*
from dm_product_release_schema prs
inner join dm_module_info mi on mi.id = prs.module_id and mi.deleted_time = 0
where prs.release_id = ?
  and prs.deleted_time = 0
  and mi.module_kind in (ModuleKindPropertyTsdb, ModuleKindEvent)
```

用途：

+ `acxPushDecode`
+ 设备定时上传 Modbus 数据块解码
+ `ThingLogic.HandleProperty`
+ event 上报参数校验 / event payload 组装
+ TDengine 建表
+ TDengine 写入
+ 属性历史查询

缓存 key：

```latex
things:schema:product:release:{productID}:{releaseID}:periodic_decode
things:schema:product:current:{productID}:periodic_decode
```

缓存 value：

+ 可以包含 `property_tsdb` 和 `event` 两类定义
+ `property_tsdb` 仍然需要保存 `identifier -> moduleCode` 或等价路由，否则无法按模块分桶写 TDengine
+ `event` 只用于事件定义、事件参数校验和事件 payload 组装，不写入 `model_module__{moduleCode}`
+ 不包含 `property_no_store`，避免周期解码热路径加载冷点位

### 5.3 ActiveReadSchema
过滤条件：

```sql
select prs.*
from dm_product_release_schema prs
inner join dm_module_info mi on mi.id = prs.module_id and mi.deleted_time = 0
where prs.release_id = ?
  and prs.deleted_time = 0
  and mi.module_kind in (ModuleKindPropertyTsdb, ModuleKindPropertyNoStore)
```

用途：

+ `acxThingForward`
+ `acxThingForwardResponse`
+ 平台主动读取指令拼包
+ 主动读取回包解码
+ 调试 / 安装 / 售后诊断读取

缓存 key：

```latex
things:schema:product:release:{productID}:{releaseID}:active_read
things:schema:product:current:{productID}:active_read
```

说明：

+ 主动读取可能读取 `property_tsdb` 点，也可能读取 `property_no_store` 点
+ `active_read` 不包含 `event/action`
+ 主动读取请求必须显式携带 identifiers，不能因为拿到 `active_read` schema 就读取全量点位
+ 读块计划由 `acxThingForward` / 协议适配逻辑按厂家规则动态生成，不预先写进 schema cache
+ 主动读取结果默认不自动写 TDengine 历史，即使读取的点属于 `property_tsdb`

### 5.4 EventSchema / ActionSchema
V1 不把 `event` / `action` 作为主要运行时 cache scope 单独拆出。

event：

+ 定义进入 `FullSchema`
+ 定义也进入 `PeriodicDecodeSchema`
+ 数据继续走现有 event log / event 存储路径
+ 不写入 `model_module__{moduleCode}`

action：

+ 定义进入 `FullSchema`
+ 不进入 `PeriodicDecodeSchema`
+ 不进入 `ActiveReadSchema`
+ action 执行结果继续走 action / send log
+ 不写入 `model_module__{moduleCode}`

如果后续 event/action 处理频率明显升高，可以再补可选 scope：

```latex
things:schema:product:release:{productID}:{releaseID}:event
things:schema:product:current:{productID}:event
things:schema:product:release:{productID}:{releaseID}:action
things:schema:product:current:{productID}:action
```

### 5.5 Scope 汇总
| scope | 包含 ModuleKind | 主要使用方 | 是否高频 |
| --- | --- | --- | --- |
| `periodic_decode` | `property_tsdb` + `event` | `acxPushDecode`、设备周期上报 | 是 |
| `active_read` | `property_tsdb` + `property_no_store` | `acxThingForward`、`acxThingForwardResponse` | 低频到中频 |
| `full` | 全部 kind | 管理端、发布校验、调试导出 | 低频 |


---

## 6. 数据存储边界
### 6.1 property_tsdb
写入：

+ 设备周期上报
+ `PeriodicDecodeSchema` 解码
+ 按模块分桶
+ 写入 TDengine 模块超级表

查询：

+ 属性历史
+ latest
+ 聚合

物理表：

```latex
model_module__{moduleCode}
device_module__{productID}__{deviceName}__{moduleCode}
```

### 6.2 property_no_store
写入：

+ 不参与周期上报写入
+ 不写 TDengine

读取：

+ 平台主动读取
+ 调试 / 安装 / 售后诊断

缓存：

+ 可选短 TTL 当前值缓存

```latex
device:thing:property:active_read:last:{productID}:{deviceName}
```

### 6.3 event
定义：

+ 可以按模块管理
+ 字段明细发布后冻结到 `DmProductReleaseSchema`
+ 模块类型由 `DmModuleInfo.ModuleKind=event` 判定

数据：

+ 继续写 event log / 现有 event 存储
+ event params 作为 event payload 保存
+ 不按模块超级表列存储

说明：

+ event 是离散事实，不是周期采样列
+ 不应复用 property 模块超级表

### 6.4 action
定义：

+ 可以按模块管理
+ 字段明细发布后冻结到 `DmProductReleaseSchema`
+ 模块类型由 `DmModuleInfo.ModuleKind=action` 判定

数据：

+ action 下发、响应、失败、超时继续走 send/action log
+ 不写 property 模块超级表

---

## 7. 逐行快照基线为什么不新增 DmProductReleaseModule
在 `DmProductReleaseSchema` 逐行快照基线下，不新增的原因：

+ `DmProductReleaseSchema` 已经能表达 release 下的字段明细和模块归属
+ 模块集合可以从 `DmProductReleaseSchema` 按 `release_id + module_id + module_code` 聚合得到
+ 多一张 `DmProductReleaseModule` 会引入“模块清单”和“字段明细”两份 release 真相
+ `ModuleKind` 创建后绝对不可修改，直接从 `DmModuleInfo` 读取即可，不需要为了冻结它再新增 release module 表或冗余字段

因此逐行快照基线采用：

```latex
DmProductReleaseSchema + immutable DmModuleInfo.ModuleKind = runtime truth
```

而不是：

```latex
DmProductReleaseModule + DmProductReleaseSchema = 两层 release truth
```

但在 ASN / ASG 多产品共用同一模块点位的高复用场景下，逐行 `DmProductReleaseSchema` 会造成点位行数按产品和 release 放大。  
这类场景改用 [design-category-module-snapshot-release-2026-04-28.md](/home/wangjunbo/workspace-aux/doc/module-level-supertable/design-category-module-snapshot-release-2026-04-28.md)：

```latex
DmProductRelease
  + DmProductReleaseModule
  + DmModuleSchemaSnapshot
  + immutable DmModuleInfo.ModuleKind
= runtime truth
```

注意：新方案不再把 `DmProductReleaseSchema` 作为运行时真相，因此不会形成“两层 release truth”。

---

## 8. 关键接口影响
### 8.1 ProductSchemaRepo.GetByRelease
应支持 scope：

+ `full`
+ `periodic_decode`
+ `active_read`

最小 V1：

+ `periodic_decode`
+ `active_read`
+ `full`

event/action 独立 scope 暂不作为 V1 主路径。

### 8.2 DeviceSchemaRepo.GetData
设备周期上报路径应使用 `periodic_decode` scope：

```latex
DeviceSchemaRepo.GetData(dev, scope=periodic_decode)
```

或者保留现有 `GetData(dev)`，但其语义必须明确为 periodic decode runtime view。

如果需要 full view，应使用显式方法：

```latex
DeviceSchemaRepo.GetFullData(dev)
```

主动读取应使用显式 scope：

```latex
DeviceSchemaRepo.GetData(dev, scope=active_read)
```

避免调用方误把 full schema 送进周期解码或 TDengine 写路径。

### 8.3 runtime-schema-batch
规则引擎周期解码：

```json
{
  "items": [
    {"productID": "p1", "releaseID": 1001, "scope": "periodic_decode"}
  ]
}
```

主动读取：

```json
{
  "items": [
    {"productID": "p1", "releaseID": 1001, "scope": "active_read"}
  ]
}
```

---

## 9. 实施顺序
1. 在 `DmModuleInfo` 增加 `module_kind`
2. 管理端和后端 update 接口禁止修改 `module_kind`
3. `DmProductReleaseSchema` 不增加 `module_kind`
4. 发布流程读取 `DmModuleInfo.ModuleKind` 做校验，但不复制到 release schema
5. `periodic_decode` scope 通过 join `DmModuleInfo` 按 `property_tsdb + event` 生成
6. `active_read` scope 通过 join `DmModuleInfo` 按 `property_tsdb + property_no_store` 生成
7. TDengine 建表 / 写入 / 历史查询只消费 `periodic_decode` 中的 `property_tsdb` 部分
8. event/action 数据路径保持现状，不写入 property 模块超级表

---

## 10. 一句话结论
`ModuleKind` 是模块的运行时类别，不只是展示字段。  
逐行快照基线不新增 `DmProductReleaseModule`，也不把 `ModuleKind` 冗余到 `DmProductReleaseSchema`；高复用场景可采用 2026-04-28 模块快照方案，引入 `DmProductReleaseModule` 但停止把 `DmProductReleaseSchema` 作为运行时真相。`ModuleKind` 创建后绝对不可修改，运行时始终按 `module_id` join `DmModuleInfo.ModuleKind` 获取模块类型。  
`periodic_decode` 提供给 `acxPushDecode`，包含 `property_tsdb + event`；`active_read` 提供给 `acxThingForward/acxThingForwardResponse`，包含 `property_tsdb + property_no_store`。TDengine 只处理其中的 `property_tsdb`，event/action 数据仍走各自日志路径。

