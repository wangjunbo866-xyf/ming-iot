# 模块级 TDengine 超级表设计
## 目标
将 dmsvr 的时序存储从“产品级超级表（每个属性一张表）”重构为“模块导向的超级表（每个逻辑模块一张表，模块内所有点位作为列，版本只存在于逻辑 schema 和产品发布快照中）”，从而支持通过模块组合快速创建产品。

## 背景
公司属于新能源硬件制造场景。产品天然是模块化的，例如一个大的逆变器系列会共享一整套 modbus 寄存器表，点位规模可达数千个，再按地址段拆分为多个功能模块。具体某个小产品只会扫描其中一部分模块。当前系统里每个属性都会对应一个 TDengine 超级表，最终产生大量表。新设计希望把一个逻辑模块族映射为少量按模块划分、可追加扩列的超级表。

## 业务模型
```latex
产品品类（现有，例如“逆变器系列”）
├── 模块：Common  （地址 1000~1500）— 功率、调节、语言等
├── 模块：Alarm   （地址 2000~2500）— 故障/告警点位
├── 模块：Storage （地址 2500~2700）— 电池/储能点位
├── 模块：...
│
├── 产品 A（categoryID=100）→ 使用：Common + Alarm + Storage
├── 产品 B（categoryID=100）→ 使用：Common + Alarm
└── 产品 C（categoryID=100）→ 使用：Common + Storage
```

关键业务约束：

+ 每个模块包含 50~200 个点位
+ 同一产品下多个模块可能位于同一张 modbus 数据表中，一次扫描/一次设备上报可同时覆盖多个模块
+ 模块是 schema 复用与版本演进单元，不等于一次上报批次
+ 同一 `product_id` 下可能同时存在不同固件版本的设备，并同时在线上报
+ 一个产品至多使用某个模块类型的一个实例
+ 模块模板可能增加或删除点位
+ 大面积且非兼容的点表变更（如寄存器大范围重排、字段语义/类型/单位整体变化、旧解析逻辑明显失效）视为新产品，而不是当前产品的兼容升级
+ 事件保持不变（本次不在范围内）
+ V1 作为新项目重构，时序存储不再同时维护 legacy“按属性超级表”和模块“按模块超级表”两套路由；legacy schema / legacy 数据迁移 / 旧表兼容另行设计，不纳入本方案

兼容性与落地约束：

+ 现有对外 API 继续使用扁平的 `identifier` / `dataID` 语义，模块只是内部建模与存储概念
+ repo/cache 层统一返回 `SchemaBundle{Model, Route}`；对外协议、RPC 和大多数业务逻辑继续只消费其中展平后的 `Model`
+ 对于模块化产品，展平后的有效标识符在所有已绑定模块之间必须全局唯一
+ V1 不再保留设备自定义 schema 的运行时 auto-create / 按属性存储路径；schema 变更只能通过模块建模、产品装配和发布快照生效
+ 运行时上报不得触发模块扩列、自动建模或 schema 变更
+ 新项目下，模块 schema 只支持现有标量类型全集：`bool`、`smallInt`、`smallIntUnsigned`、`tinyInt`、`tinyIntUnsigned`、`int`、`intUnsigned`、`bigInt`、`bigIntUnsigned`、`float`、`double`、`string`、`enum`、`timestamp`
+ `struct` / `array` 不再支持；若需要表达复杂点位分组，统一通过“模块拆分 + 扁平 identifier”建模，而不是通过容器类型建模
+ 本方案不承载 legacy 产品中已存在 `struct/array` 物模型的在线迁移；若后续需要接入，必须另立 legacy 迁移/兼容专项方案
+ 新项目下，属性历史记录策略统一为“每次上报都记录历史”；V1 固定按 `RecordModeAll` 处理，不再支持 `RecordModeAuto/None`
+ 设备属性消息不要求携带模块版本；运行时 schema 路由不再依赖 `DmDeviceInfo.Version`，消息本身只上报值
+ `DmDeviceInfo.Version` 明确表示设备固件版本号，仅用于 OTA、展示、诊断和协议侧元数据，不再参与 schema 路由
+ V1 的真实业务前提是：未设置 override 的设备跟随产品 `current_release_id`；固件升级批次可通过设备级 override 显式切到指定发布快照
+ V1 不引入“固件版本 -> schema 快照”的全量映射表，因为该关系在业务上无人维护，长期不可持续
+ `current_release_id` 是产品默认运行版本 / fleet default，不等同于最新发布版本
+ `ReleaseNo` 是管理端展示版本；设备运行时 schema 只依赖“设备 override + 产品 current”组合解析
+ 模块不再拥有独立的运行时版本；运行时唯一版本概念是 `DmProductRelease`
+ 同一个 `identifier` 在同一逻辑模块的后续版本中不得复用为新语义；若字段语义、单位、类型或计算口径发生变化，必须使用新的 `identifier`
+ 当前方案只优化兼容性演进：新增少量采集点位、废弃旧点位、局部字段调整；不要求兼容“大面积非兼容点表改造”

本次业务需求补充：

+ `DmDeviceInfo.Version` 是产品固件版本号，不是 schema 快照版本号；多个固件版本可能共用一套点位
+ 现场不会维护“所有固件版本对应哪套 schema”的映射关系
+ 产品维度存在一个默认运行 schema 快照，即 `dm_product_info.current_release_id`，但它不自动等于最新发布快照
+ 设备可以在 OTA 成功后通过 `schema_release_id_override` 显式绑定到新发布快照；也可以在兼容场景下绑定到历史快照
+ 因此 V1 运行时解析规则固定为：`device.schema_release_id_override ? override : product.current_release_id`

建模边界：

+ 模块是内部 schema 复用、产品装配和时序存储路由单元，不是对外物模型命名空间
+ 采集批次（report batch）是一次 modbus 扫描/一次设备上报，可同时覆盖多个模块
+ 对外唯一契约仍是扁平的 `identifier` / `dataID`；`moduleCode` 仅用于后台配置和 TDengine 路由
+ 模块化产品必须先在 repo/cache 层物化为扁平 `schema.Model`，再暴露给上层逻辑、协议处理和 API
+ SQL 关系关联默认使用 `ModuleID`；`ModuleCode` 是业务唯一键和物理命名输入，不作为主要外键

风险点 1 处理策略：防止“模块”概念外溢成新的对外物模型层：

+ V1 中模块只承载 `property` 的复用、组合和存储；`event` / `action` 继续保持现有模型，不引入模块命名空间
+ 任何协议处理、缓存键、对外 RPC 入参与响应体都不应要求调用方感知 `moduleCode`；调用方继续只使用扁平 `identifier` / `dataID`
+ repo / cache / tsDB 路由层内部维护 `identifier -> {moduleID, moduleCode, propertyType}` 映射；该映射不向上层暴露
+ `ProductSchemaRepo.GetCurrent()` / `ProductSchemaRepo.GetByRelease()` / `DeviceSchemaRepo.GetData()` 返回结果统一为 `SchemaBundle{Model, Route}`；`ThingLogic`、协议层和现有查询 API 默认只消费 `bundle.Model`，`bundle.Route` 仅供内部路由层使用
+ 新增 `Module*` RPC 的定位是管理端和内部装配能力，用于模块定义、版本发布、产品绑定和模块级时序查询；现有面向业务侧的产品/设备/属性接口继续保持扁平语义
+ 后续若需要支持“同类模块多实例”或对外暴露模块维度，应作为新的模型版本单独设计，不能在本方案上直接演化破坏现有扁平契约

---

## 1. 领域模型（SQL 数据库）
### 1.1 新增表
#### DmModuleInfo — 模块定义
```go
type DmModuleInfo struct {
    ID         int64  `gorm:"column:id;type:bigint;primary_key;AUTO_INCREMENT"`
    ModuleCode string `gorm:"column:module_code;uniqueIndex:idx_dm_module_info_code;type:varchar(100);NOT NULL"` // 模块业务唯一标识，发布后不可修改
    ModuleName string `gorm:"column:module_name;type:varchar(100);NOT NULL"`                                     // 模块名称
    ModuleKind int64  `gorm:"column:module_kind;type:smallint;default:1"`                                         // 1:property_tsdb 2:property_no_store 3:event 4:action；创建后不可修改
    Desc       string `gorm:"column:desc;type:varchar(200)"`
    AddrStart  int64  `gorm:"column:addr_start;type:integer"`                                                    // Modbus 参考起始地址（展示/排序辅助字段，不要求模块内点位连续）
    AddrEnd    int64  `gorm:"column:addr_end;type:integer"`                                                      // Modbus 参考结束地址（展示/排序辅助字段，不作为严格边界）
    Status     int64  `gorm:"column:status;type:smallint;default:1"`                                             // 1:启用 2:禁用
    stores.NoDelTime
    DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:idx_dm_module_info_code"`
}
```

#### DmCategoryModule — 品类与模块绑定（可用模块池）
```go
type DmCategoryModule struct {
    ID         int64  `gorm:"column:id;type:bigint;primary_key;AUTO_INCREMENT"`
    CategoryID int64  `gorm:"column:category_id;uniqueIndex:idx_dm_category_module;type:bigint;NOT NULL"`
    ModuleID   int64  `gorm:"column:module_id;uniqueIndex:idx_dm_category_module;type:bigint;NOT NULL"` // 引用 dm_module_info.id
    stores.NoDelTime
    DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:idx_dm_category_module"`
}
```

#### DmProductModule — 产品模块草稿（管理端编辑态）
```go
type DmProductModule struct {
    ID        int64  `gorm:"column:id;type:bigint;primary_key;AUTO_INCREMENT"`
    ProductID string `gorm:"column:product_id;uniqueIndex:idx_dm_product_module;type:varchar(100);NOT NULL"`
    ModuleID  int64  `gorm:"column:module_id;uniqueIndex:idx_dm_product_module;type:bigint;NOT NULL"` // 引用 dm_module_info.id
    stores.NoDelTime
    DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:idx_dm_product_module"`
}
```

#### DmProductRelease — 产品发布快照头
```go
type DmProductRelease struct {
    ID           int64  `gorm:"column:id;type:bigint;primary_key;AUTO_INCREMENT"`
    ProductID    string `gorm:"column:product_id;uniqueIndex:idx_dm_product_release_no;type:varchar(100);NOT NULL"`
    ReleaseNo    string `gorm:"column:release_no;uniqueIndex:idx_dm_product_release_no;type:varchar(64);NOT NULL"` // 产品发布版本号，如 r1 / 2026.04；同一产品下唯一
    IsDefault    int64  `gorm:"column:is_default;type:smallint;default:0"` // 1:默认发布快照
    Status       int64  `gorm:"column:status;type:smallint;default:1"`     // 1:启用 2:停用
    Desc         string `gorm:"column:desc;type:varchar(200)"`
    stores.NoDelTime
    DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:idx_dm_product_release_no"`
}
```

#### DmProductReleaseSchema — 产品发布快照字段明细（运行时字段集合真相）
```go
type DmProductReleaseSchema struct {
    ID         int64  `gorm:"column:id;type:bigint;primary_key;AUTO_INCREMENT"`
    ReleaseID  int64  `gorm:"column:release_id;uniqueIndex:idx_dm_product_release_schema_identifier;uniqueIndex:idx_dm_product_release_schema_module_identifier;type:bigint;NOT NULL"` // 引用 dm_product_release.id
    ProductID  string `gorm:"column:product_id;index:idx_dm_product_release_schema_product;type:varchar(100);NOT NULL"`
    ModuleID   int64  `gorm:"column:module_id;uniqueIndex:idx_dm_product_release_schema_module_identifier;index:idx_dm_product_release_schema_module;type:bigint;NOT NULL"` // 引用 dm_module_info.id
    ModuleCode string `gorm:"column:module_code;index:idx_dm_product_release_schema_module;type:varchar(100);NOT NULL"`
    Identifier string `gorm:"column:identifier;uniqueIndex:idx_dm_product_release_schema_identifier;uniqueIndex:idx_dm_product_release_schema_module_identifier;type:varchar(100);NOT NULL"`
    DmSchemaCore
    stores.NoDelTime
    DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:idx_dm_product_release_schema_identifier;uniqueIndex:idx_dm_product_release_schema_module_identifier"`
}
```

#### DmModuleSchemaInfo — 模块 Schema 模板（管理端当前态）
```go
type DmModuleSchemaInfo struct {
    ID         int64  `gorm:"column:id;type:bigint;primary_key;AUTO_INCREMENT"`
    ModuleID   int64  `gorm:"column:module_id;uniqueIndex:idx_dm_module_schema_identifier;type:bigint;NOT NULL"` // 引用 dm_module_info.id
    Identifier string `gorm:"column:identifier;uniqueIndex:idx_dm_module_schema_identifier;type:varchar(100);NOT NULL"`
    DmSchemaCore
    stores.NoDelTime
    DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:idx_dm_module_schema_identifier"`
}
```

说明：

+ `ID` 是 SQL 关系主键；`ModuleCode` 是业务唯一键，用于管理端识别、导入导出、i18n key 和 TDengine 物理命名
+ `ModuleCode` 在 V1 中要求**全局唯一**，不按 `categoryID` 分区；`dm_module_info.module_code` 的唯一索引和管理端前置校验必须同时存在
+ `ModuleKind` 是创建期结构字段，创建后绝对不允许修改；管理端不提供编辑入口，后端 update 接口也必须拒绝 `module_kind` 变更
+ `DmCategoryModule` 管理逻辑上的模块池；`DmProductModule` 是管理端草稿；运行时以 `DmProductRelease + DmProductReleaseSchema` 作为不可变产品版本快照
+ `DmProductRelease` 表示一份不可变的 schema 发布快照；`DmProductReleaseSchema` 冻结该发布快照对应的最终字段集合和模块归属
+ `DmProductReleaseSchema` 不冗余 `ModuleKind`；scope 视图派生、TDengine 建表、写入和查询需要模块类型时，通过 `dm_product_release_schema.module_id` join `dm_module_info.module_kind` 获取
+ `DmModuleSchemaInfo` 是模块模板当前态，可持续编辑；已发布设备看到的始终是 `DmProductReleaseSchema`，不会受后续模板修改影响
+ `ModuleCode` 发布后不允许修改；如确需变更，应创建新的逻辑模块并迁移绑定，避免影响历史 lang_key 和 TDengine 表命名
+ SQL 关系层、绑定关系层、发布快照层默认使用 `ModuleID` 做关联；只有 TDengine 命名和路由层使用 `ModuleCode`
+ `categoryID` 只存在于模块池绑定（`DmCategoryModule`）和产品/品类管理语义中；不进入 TDengine 物理接口签名
+ `AddrStart/AddrEnd` 仅表示模块点位地址的大致覆盖范围，服务于管理端展示、排序和人工理解；不要求模块点位在地址上连续，也不作为运行时路由或发布校验的唯一依据
+ 模块内点位的真实地址分布以 `DmModuleSchemaInfo` 明细为准；若后续需要显式建模“一个模块对应多段不连续地址”，应新增独立的模块地址段表，而不是继续扩展单个 `AddrStart/AddrEnd`
+ `ProductModuleMultiUpdate` 必须校验所有选中模块在展平之后的标识符全局唯一
+ `DmDeviceInfo.Version` 继续保留为固件版本号；V1 中它不再参与 schema 路由，只作为 OTA、展示和诊断元数据

### 1.2 发布快照匹配规则
运行时匹配输入：

+ `schema_release_id_override`：来自 `dm_device_info.schema_release_id_override`
+ `current_release_id`：来自 `dm_product_info.current_release_id`

发布快照解析顺序（V1）：

1. 先读取 `dm_device_info.schema_release_id_override`
2. 若 `schema_release_id_override > 0`：
    - 校验该 `release_id` 存在
    - 校验该 `release_id` 属于同一 `product_id`
    - 校验该 `release_id` 已发布且可用
    - 校验成功后，直接使用该历史发布快照
3. 若 `schema_release_id_override == 0`：
    - 读取 `dm_product_info.current_release_id`
    - 若 `current_release_id > 0`，则使用该产品默认发布快照
4. 若产品未发布（`current_release_id == 0`），则返回 `product_unpublished`

解析结果处理：

+ `schema_release_id_override > 0` 且校验通过：记录 `release_resolve_by_device_override` 指标；`DeviceSchemaRepo.GetData()` 使用该精确发布快照
+ `schema_release_id_override == 0` 且 `current_release_id > 0`：记录 `release_resolve_by_product_current` 指标；`DeviceSchemaRepo.GetData()` 使用产品默认发布快照
+ `current_release_id == 0`：`DeviceSchemaRepo.GetData()` 返回 `ErrProductUnpublished`，模块化写入/单设备运行时查询拒绝继续
+ `schema_release_id_override` 指向不存在、已删除、跨产品或不可用的快照：`DeviceSchemaRepo.GetData()` 返回 `ErrDeviceReleaseOverrideInvalid`，禁止静默回退到 `current_release_id`
+ `dm_product_info.current_release_id` 用于 `ProductSchemaRepo.GetCurrent(productID, scope)` 生成产品默认视图；同时也是“未 override 设备”的默认运行时快照来源
+ `current_release_id` 不代表最新 release，只代表 fleet default；发布新 release 不应自动修改它
+ 固件升级完成后若需要切 schema，应由 OTA 成功链路或管理端批量写入 `schema_release_id_override`

固件版本处理：

+ `DmDeviceInfo.Version` 在 V1 中不再参与 schema 发布快照匹配
+ 固件版本仍按现有业务继续维护，可用于 OTA、展示和诊断
+ 版本规范化规则（V1）继续仅做首尾空白 trim，但该规则只影响固件版本存储和展示，不影响 schema 路由
+ 设备定时上报 `Version` 只更新固件元数据，不触发 schema 自动切换

并发发布冲突处理：

+ `DmProductRelease(product_id, release_no)` 的唯一索引是最终裁决者
+ `ProductReleasePublish` 可以在写库前先做“是否已存在”的前置校验，但这只能优化用户提示，不能替代最终并发保护
+ 若两个操作员并发对同一 `(product_id, release_no)` 发布，后提交的一方命中 DB duplicate key 时，服务层必须将底层错误统一转换为业务错误 `ErrReleaseNoConflict`
+ 对外 API / RPC 不应直接暴露底层 duplicate key / unique index 异常文本

### 1.3 修改表
#### DmProductInfo — 增加 CurrentReleaseID
```go
// 新增字段
CurrentReleaseID int64 `gorm:"column:current_release_id;type:bigint;default:0"`
// 产品当前默认发布快照 ID；0 表示产品尚未发布
// 用于 ProductSchemaRepo.GetCurrent(productID, scope) 的默认视图
```

#### DmDeviceInfo — 增加 SchemaReleaseIDOverride
```go
// 新增字段
SchemaReleaseIDOverride int64 `gorm:"column:schema_release_id_override;type:bigint;default:0"`
// 设备 schema 发布快照覆盖字段
// 0  表示不覆盖，默认跟随 dm_product_info.current_release_id
// >0 表示该设备固定使用指定历史发布快照
```

#### `dm_schema_info` 继续作为 legacy 产品/设备 schema 存储
`dm_schema_info` 保持当前职责不变：

+ `product_id=X, device_name=NULL` → 产品级 schema（老产品）
+ `product_id=X, device_name=Y` → 设备级 schema（不变）

模块级 schema 定义存储在 `dm_module_schema_info` 中，不再混入 `dm_schema_info`。

原因：

+ `dm_schema_info.product_id` 当前已经是主访问路径的一部分，直接复用会破坏现有 cache/repo 链路
+ 现有产品/设备 schema 代码路径保持稳定，模块化产品只在读取时被物化为相同形态的扁平 `schema.Model`

#### 产品 / 设备 Schema 物化规则
repo 合约：

```go
type ProductSchemaRepo interface {
    // 当前产品视图：给产品详情、点位列表、默认展示使用
    // 语义上等于 current_release_id 对应的 bundle
    GetCurrent(ctx context.Context, productID string, scope string) (*schema.SchemaBundle, error)

    // 精确发布快照视图：给发布校验、版本预览、设备运行时解析复用
    GetByRelease(ctx context.Context, productID string, releaseID int64, scope string) (*schema.SchemaBundle, error)
}

type DeviceSchemaRepo interface {
    // 设备运行时视图：内部先解析 releaseID，再读取该发布快照的最终 schema
    GetData(ctx context.Context, dev devices.Core, scope string) (*schema.SchemaBundle, error)
}
```

物化规则：

+ `ProductSchemaRepo.GetCurrent(productID, scope)`：
    - 读取 `dm_product_info.current_release_id`，然后调用 `GetByRelease(productID, currentReleaseID, scope)`
+ `ProductSchemaRepo.GetByRelease(productID, releaseID, scope)`：
    - 从 `dm_product_release_schema(release_id, module_id, module_code, identifier, ...)` 直接拉取该发布快照冻结的字段集合
    - 同时构建 `identifier -> SchemaRouteEntry`
    - 返回该发布快照的精确 `SchemaBundle`
+ `DeviceSchemaRepo.GetData(dev, scope)` 在模块化产品下：
    - 先读取设备运行时元信息缓存 `cache:things:device:runtime`
    - 读取其中的 `schema_release_id_override`
    - 若 `schema_release_id_override > 0`，先校验该 `release_id` 属于同一产品且处于可用状态
    - override 校验通过时，调用 `ProductSchemaRepo.GetByRelease(productID, schema_release_id_override, scope)`
    - 若 `schema_release_id_override == 0`，则调用 `ProductSchemaRepo.GetCurrent(productID, scope)`
    - 若产品未发布，返回 `ErrProductUnpublished`
    - 若 override 非法，返回 `ErrDeviceReleaseOverrideInvalid`
    - 返回设备最终运行时 `SchemaBundle`
+ `DeviceSchemaRepo.GetData()` 的运行时快照来源固定为“设备 override 或产品 current”，不再从 `DmDeviceInfo.Version` 推导 `release_id`
+ `schema.Model` 的对外形态不变；模块化逻辑必须在暴露给调用方之前完成 release 解析和 route 构建
+ `dm_device_info.version` 变化后，不需要因此切换 schema 路由；后续请求仍只受 `schema_release_id_override` 和 `current_release_id` 影响
+ 默认发布快照 `current_release_id` 既影响 `ProductSchemaRepo.GetCurrent(..., scope)`，也影响所有“未 override 的设备”运行时 bundle；但不应污染已设置 override 的设备

接口语义对照：

| 视图 / 接口 | 数据来源 | 是否已发布 | 典型用途 | 是否可直接用于设备运行时写入 / 单设备精确运行时读 |
| --- | --- | --- | --- | --- |
| `ProductSchemaRepo.GetCurrent(productID, scope)` | `dm_product_info.current_release_id -> dm_product_release_schema -> scoped view` | 是 | 产品详情、点位列表、默认展示、产品级默认字段视图 | 否。它只表示“当前默认发布视图”，不代表某台设备真实命中的 release |
| `ProductSchemaRepo.GetByRelease(productID, releaseID, scope)` | 指定 `release_id -> dm_product_release_schema -> scoped view` | 是 | 发布校验、发布详情、版本预览、显式按 release 的查询 | 不能单独替代设备运行时解析；它是 `DeviceSchemaRepo.GetData()` 的下层复用能力 |
| `DeviceSchemaRepo.GetData(dev, scope)` | `cache:things:device:runtime -> schema_release_id_override ? GetByRelease(..., scope) : GetCurrent(..., scope)` | 是 | 设备属性上报、控制、单设备查询、规则引擎设备级 schema 读取 | 是。这是设备运行时唯一正确 schema 视图 |
| 草稿读取（不走以上 repo） | `DmProductModule` 草稿 + `dm_module_schema_info` 模板 | 否 | 管理端编辑草稿、发布前预览、发布前校验 | 否。草稿未冻结为 `DmProductReleaseSchema`，也不应参与线上 TDengine 读写 |


说明：

+ `GetCurrent()` 不是“未发布 schema”，而是“已发布且被设为默认版本的产品视图”
+ 未发布内容属于产品草稿/模块模板范畴，应由管理端草稿接口读取，不应复用 `GetCurrent()` / `GetByRelease()` / `DeviceSchemaRepo.GetData()`
+ TDengine 物理层是否已准备完毕，取决于发布动作和后续 `ADD COLUMN` / 首报 `INSERT ... USING`，不取决于 `GetCurrent()` 这个接口本身
+ 设备运行时写入/查询必须以 `DeviceSchemaRepo.GetData(dev, scope)` 为准；`GetCurrent(..., full)` 只能用于“没有具体设备上下文”的默认展示视图

#### Identifier 路由表（核心性能设计）
为避免写入/查询路径上反复执行"点位 → 模块"的 SQL 查询，`ProductSchemaRepo.GetCurrent()`、`ProductSchemaRepo.GetByRelease()` 和 `DeviceSchemaRepo.GetData()` 在物化 flat `schema.Model` 的同时，**必须一起构建并返回一份 identifier 路由表**。路由表与 flat schema 共享同一份缓存条目、同一套失效策略，上层调用方一次取值即可同时拿到"逻辑形态"和"物理路由"。

路由表数据结构：

```go
// SchemaRouteEntry 描述一个对外 identifier/dataID 的物理落点
type SchemaRouteEntry struct {
    ModuleID     int64
    ModuleCode   string       // TDengine 物理路由键
    PropertyType schema.Type  // 类型信息，用于参数编解码
}

// SchemaBundle 是 ProductSchemaRepo / DeviceSchemaRepo 的统一返回形态
type SchemaBundle struct {
    Model schema.Model                 // 对外不变的扁平物模型
    Route map[string]SchemaRouteEntry  // identifier -> 物理路由；key 为扁平后的 dataID
    Meta  SchemaBundleMeta             // 当前 bundle 的来源信息
}

type SchemaBundleMeta struct {
    ProductID  string
    ReleaseID  int64 // legacy 为 0
}
```

构建与使用约束：

+ 路由表在 `ProductSchemaRepo.GetCurrent()` / `ProductSchemaRepo.GetByRelease()` / `DeviceSchemaRepo.GetData()` 内部一次性构建；调用方不允许独立再查 SQL 来推导路由
+ V1 路由表只包含已绑定模块的 identifier；`ModuleCode` 为物理路由键，物理列名直接等于 `identifier`
+ Identifier 路由表与 flat schema 共享缓存条目与失效事件；任何一次 schema cache 失效（产品发布切换、设备版本变化、发布快照重建）都必须同时淘汰路由表
+ 路由表**不向对外协议层和对外 API 暴露**；它只被 tsDB 写入/查询层、`HandleProperty` 校验分组、PropertyLogIndex/DataIndex 等内部路由逻辑消费
+ `release_id` 只用于决定某个 identifier 在当前发布快照下"是否可见"：
    - 写入路径：若请求携带的 identifier 不存在于当前 `release_id` 的快照中，按"未知 identifier"策略处理，不进入模块 INSERT 列清单
    - 查询路径：返回投影时按当前 `release_id` 的快照过滤；已从当前快照移除的旧字段默认隐藏，管理端调试/历史导出场景显式传 `includeDeprecated=true` 才返回
+ 对于同一 batch 上报，路由表在内存中一次 O(N) 分桶即可得到"每个 moduleCode → 属于本模块的字段清单"，无任何额外 DB/Redis 往返
+ 对于 N 个点位的批量查询（报表场景），通过路由表分桶后只需发出 M 条 SQL（M ≤ 涉及的 moduleCode 数，通常 3~5），**严格优于 legacy 的 N 条 SQL**

路由表构建的 SQL（模块化产品，device schema 物化时）：

```sql
-- 1) 先按 §1.2 解析出 release_id（略）
-- 2) 批量拉取该 release 下所有模块的叶子字段
SELECT prs.module_id,
       prs.module_code,
       prs.identifier,
       prs.affordance,      -- 用于读取属性定义/类型信息
       prs.data_type,
       mi.module_kind       -- 不冗余在 DmProductReleaseSchema；按 module_id 从 DmModuleInfo 获取
FROM dm_product_release_schema prs
INNER JOIN dm_module_info mi ON mi.id = prs.module_id AND mi.deleted_time = 0
WHERE prs.release_id = ? AND prs.deleted_time = 0;
```

缓存条目形态：

```latex
# 产品当前视图（用于产品详情 / 点位列表 / 默认展示）
cache key:   things:schema:product:current:{productID}:{scope}
cache value: SchemaBundle { Model, Route, Meta{ReleaseID=current_release_id or 0} }

# 产品精确发布快照（用于设备 release 解析复用）
cache key:   things:schema:product:release:{productID}:{releaseID}:{scope}
cache value: SchemaBundle { Model, Route }
```

`scope` 取值：

+ `periodic_decode`：`property_tsdb + event`，供 `acxPushDecode`、设备周期上报解码、event 上报校验、TDengine 建表 / 写入 / 查询使用；TDengine 只消费其中 `property_tsdb`
+ `active_read`：`property_tsdb + property_no_store`，供 `acxThingForward` / `acxThingForwardResponse` 主动抄读拼包和回包解码使用
+ `full`：全部模块，供管理端、发布校验、调试导出和完整展示使用

失效触发事件（新增/明确）：

+ `DmProductRelease` / `DmProductReleaseSchema` 变更 → 淘汰对应 release 的 `things:schema:product:release:{productID}:{releaseID}:*`；只有该 release 正是 current 时才淘汰 `things:schema:product:current:{productID}:*`
+ `dm_product_info.current_release_id` 变更 → 仅淘汰对应 `things:schema:product:current:{productID}:*`
+ `dm_device_info.version` 变更 → 不淘汰 Redis SchemaBundle，也不改变 schema release；若 OTA 成功需要切 schema，必须显式更新 `schema_release_id_override`
+ `dm_module_schema_info` 变更并影响到产品重新发布时 → 后续新 `release` 生效；已发布 `release` 不受影响
+ `ALTER STABLE ADD COLUMN` 成功且对应新 release 发布后 → 预热或淘汰该新 release 的 `things:schema:product:release:*`；不自动淘汰 current，除非随后执行 `ProductReleaseSetDefault`

#### ModuleSchemaFilter — `dm_module_schema_info` 的专用过滤器
```go
type ModuleSchemaFilter struct {
    ModuleID      int64
    ModuleIDs     []int64
    ModuleCode    string
    ModuleCodes   []string
    Type          int64
    Types         []int64
    Identifiers   []string
    Name          string
}
```

### 1.4 i18n 兼容性
模块模板和发布快照字段都携带 `lang_key`，其余字段沿用现有 `DmSchemaCore`。现有 `lang` + `lang_translate` 表和 Excel 导入流程可以复用。模块字段的 `lang_key` 约定为：`module.{moduleCode}.{identifier}`；发布时原样复制到 `dm_product_release_schema`。

带 i18n 的批量查询示例：

```sql
SELECT prs.module_id, prs.module_code, prs.identifier, prs.lang_key, prs.affordance, lt.value AS name
FROM dm_product_info pi
INNER JOIN dm_product_release_schema prs
    ON prs.release_id = pi.current_release_id
   AND prs.deleted_time = 0
LEFT JOIN lang_translate lt ON lt.lang_key = prs.lang_key AND lt.lang_type = ?
WHERE pi.product_id = ? AND pi.deleted_time = 0
ORDER BY prs.module_code, prs.order;
```

---

## 2. TDengine 存储
### 2.1 超级表：每个逻辑模块一张
```sql
-- 命名：model_module__{moduleCode}
-- 每个逻辑模块对应一张可追加扩列的物理 STABLE
-- release 只决定当前字段集合；物理表命名只依赖 moduleCode
-- 一次上报对应一行
CREATE STABLE IF NOT EXISTS `model_module__{moduleCode}` (
    `ts`          TIMESTAMP,
    `{identifier_1}`   {TYPE_1},
    `{identifier_2}`   {TYPE_2},
    ...  -- 50~200 列
) TAGS (
    `product_id`  BINARY(100),
    `device_name` BINARY(100),
    `tenant_code` BINARY(50),
    `project_id`  BIGINT,
    `area_id`     BIGINT,
    `area_id_path` BINARY(100),
    ... -- 与当前 tdengine.GenTagsParams(...) 生成的动态 group tags 保持一致
);
```

类型映射（V1，仅支持标量类型）：

| Schema 类型 | TDengine 类型 |
| --- | --- |
| bool | BOOL |
| smallInt | SMALLINT |
| smallIntUnsigned | SMALLINT UNSIGNED |
| tinyInt | TINYINT |
| tinyIntUnsigned | TINYINT UNSIGNED |
| int | INT |
| intUnsigned | INT UNSIGNED |
| bigInt | BIGINT |
| bigIntUnsigned | BIGINT UNSIGNED |
| float | FLOAT |
| double | DOUBLE |
| string | BINARY(5000) |
| enum | SMALLINT |
| timestamp | TIMESTAMP |


说明：

+ V1 直接沿用现有 `tdengine.GetTdType(...)` 的精细化数值映射，不再将所有整数统一压成 `BIGINT`、所有浮点统一压成 `DOUBLE`
+ 类型是 schema 的显式稳定契约，不通过 `min/max` 在发布时自动推断物理类型
+ `struct` / `array` 不进入模块超级表；模块 schema CRUD、导入和发布前校验必须显式拒绝这两种类型

TDengine 命名限制（官方规则）：

+ 合法字符默认是英文字符、数字和下划线；默认规则下不能以数字开头
+ 使用反引号后，可以保留更多字符，甚至可以数字开头或使用保留关键字，但**名称中仍不能包含 **`.`
+ 表名最大长度 `192` 字节（不含数据库名前缀）
+ 列名最大长度 `64` 字节
+ 官方参考：[https://docs.taosdata.com/reference/taos-sql/limit/](https://docs.taosdata.com/reference/taos-sql/limit/)

外部 `dataID` 到物理列的映射：

+ V1 中物理列名直接等于 `identifier`
+ 例如：`temperature` → 列 `temperature`
+ 因此 `identifier` 必须满足 TDengine 安全命名约束：`^[a-z][a-z0-9_]{0,63}$`
+ 发布后物理列名不可复用到新语义；字段类型/单位/语义变化必须表现为“废弃旧字段 + 新增新字段”

### 2.2 子表：每设备每模块一张
```sql
-- 命名：device_module__{productID}__{deviceName}__{moduleCode}
-- 首次数据上报时通过 INSERT...USING 自动创建
CREATE TABLE IF NOT EXISTS `device_module__{productID}__{deviceName}__{moduleCode}`
    USING `model_module__{moduleCode}`
    TAGS ('{productID}', '{deviceName}', '{tenantCode}', {projectID}, {areaID}, '{areaIDPath}', ...);
```

### 2.3 数据写入：每批次按模块拆分 INSERT
```sql
INSERT INTO `device_module__{productID}__{deviceName}__{moduleCode}`
    USING `model_module__{moduleCode}`
    TAGS ('{productID}', '{deviceName}', '{tenantCode}', {projectID}, {areaID}, '{areaIDPath}', ...)
    (`ts`, `{point_1}`, `{point_2}`, ...)
    VALUES (?, ?, ?, ...);
```

行为约束：

+ 写入入口按“采集批次”做一次校验，再按逻辑模块拆分为多条 INSERT；运行时先解析 `release_id`，再按快照中的 `moduleID/moduleCode` 分桶，物理落表只依赖 `moduleCode`
+ 同一采集批次拆分出的多条模块 INSERT 使用同一业务时间戳
+ 模块属性必须按“整模块一行”写入
+ 为保证这一约束，V1 所有属性统一按“每次上报都记录历史”处理
+ 每条 INSERT 的列集合只包含本次批次中、当前 release 快照允许写入的字段；已废弃/已删除字段不再进入列清单
+ V1 只保留模块化时序写入路径，不再保留 device-custom / legacy 按属性写入路径
+ 未定义 identifier 不自动建模、不自动扩列、不自动创建模块列
+ 若一个批次同时包含“已定义字段 + 未定义字段”，则只写入已定义模块字段；未定义字段记录日志/指标后丢弃
+ 若一个批次中所有字段都未命中当前 `SchemaBundle.Route`，则返回参数错误并拒绝写入

### 2.4 异步批量执行约束
现有 TDengine client 虽然支持 `AsyncInsert`，但当前实现本质上是“先把多条 INSERT SQL 暂存到内存，再在 flush 时拼成一条更长的 `insert into ...` 统一执行”。模块化之后，单条模块 INSERT 的 SQL 长度和参数个数都会明显大于 legacy 的“单属性一条 INSERT”，因此 V1 必须把异步批量能力从“主要按条数 flush”升级为“主要按 SQL 大小和参数量 flush”。

官方参考文档：

+ TDengine Connector 参数（`maxSQLLength` 位于 Native connector 参数区，review 时可对照其适用范围）：[https://docs.taosdata.com/reference/components/taosc/](https://docs.taosdata.com/reference/components/taosc/)
+ TDengine SQL 命名与边界（包含 SQL 最大长度、列数、单行宽度等限制）：[https://docs.taosdata.com/reference/taos-sql/limit/](https://docs.taosdata.com/reference/taos-sql/limit/)
+ taosAdapter 组件说明（review 时可对照其对批量 SQL 的保守拆分策略）：[https://docs.taosdata.com/reference/components/taosadapter/](https://docs.taosdata.com/reference/components/taosadapter/)

设计原则：

+ 业务层写入单元固定为“一个采集批次按模块分桶后，每个 `moduleCode` 生成一条模块 INSERT”
+ 业务层不再负责跨模块、跨设备拼批；批量合并统一下沉到 TDengine client 层
+ TDengine client 必须在应用层主动控制单次提交的 SQL 大小；不能依赖驱动、taosWS 或服务端在超长 SQL 时兜底报错
+ 即使底层使用 `taosWS`，也按保守 SQL 长度阈值做拆批；V1 默认目标是“明显低于 TDengine 常见 1MB~4MB 限制区间”，避免不同版本/部署方式下出现兼容性抖动

新增配置（`TSDB`）：

+ `AsyncFlushMs`：flush 周期，默认 `50ms`
+ `AsyncMaxSQLBytes`：单批 SQL 文本总长度上限，默认 `786432`（768KB）
+ `AsyncMaxArgs`：单批参数总数上限，默认 `4096`
+ `AsyncWarnSQLBytes`：单批 SQL 告警阈值，默认 `524288`（512KB）

client 聚合算法：

+ `ExecArgs` 入队时除 `Query`、`Args` 外，额外记录 `QueryBytes=len(query)` 和 `ArgCount=len(args)`
+ `QueryBytes` 必须是**该条子 SQL 片段的真实字节长度**，不能按列数、参数个数或 identifier 数量做近似估算
+ worker 维护当前 batch 的 `sqlBytes/argCount/items`
+ `sqlBytes` 的定义必须与最终发送给 TDengine 的完整 SQL 完全一致，即：
    - `finalSQL = "insert into " + strings.Join(item.Query, " ") + ";"`
    - `sqlBytes = len(finalSQL)`
+ `joinOverhead` 不是模糊概念，V1 固定拆成三部分：
    - 批次前缀：`len("insert into ")`
    - 子 SQL 之间的分隔符：每追加一条 `len(" ")`
    - 批次后缀：`len(";")`
+ 因此增量判断时建议直接维护“若此刻 flush，会发送出去的完整 SQL 长度”：
    - 当前 batch 为空时：`candidateSQLBytes = len("insert into ") + item.QueryBytes + len(";")`
    - 当前 batch 非空时：`candidateSQLBytes = currentSQLBytes + len(" ") + item.QueryBytes`
+ 新 item 到来时，先判断：
    - `candidateSQLBytes > AsyncMaxSQLBytes`
    - `argCount + item.ArgCount > AsyncMaxArgs`
+ 任一条件命中时，先 flush 当前 batch，再尝试放入新 item
+ 为避免极端场景下一次 flush 合并过多条很小的 SQL，实现内部仍可保留一个**不对外暴露的** `hardMaxBatchCnt` 常量（例如 `64`）作为公平性保护；它不属于 V1 的公开配置，也不是 TDengine 协议边界
+ flush 时仍生成一条 `insert into ...`，但其大小和参数量已被应用层约束住

单条 SQL 保护：

+ 如果单条模块 INSERT 自身已经超过 `AsyncMaxSQLBytes` 或 `AsyncMaxArgs`，不得进入批队列
+ 该场景应直接记录错误日志和指标，并返回明确错误（例如 `ErrTDengineSQLTooLarge`）
+ 这类错误说明不是“批量器拆分不够”，而是“单个模块一行写入本身已过宽”，需要回到 schema 设计层收缩模块列数、字段类型或单列长度

监控与日志：

+ 记录每次 flush 的 `batchCount/sqlBytes/argCount/duration/error`
+ 默认 `info` 日志打印 flush 摘要：`reason/sqlBytes/argCount/batchCount/maxItemBytes/duration`
+ 为方便联调和压测，`debug` 级别允许打印完整 `finalSQL` 及其字节长度；测试环境可放开，生产环境默认不在 `info` 级打印完整 SQL，避免日志量过大和参数值泄露
+ 对以下场景，至少输出 `warn/error` 级详细日志：
    - `single_sql_oversize`
    - 执行失败重试后仍失败
    - `sqlBytes >= AsyncWarnSQLBytes`
+ 详细日志应至少包含：`sqlBytes`、`argCount`、`batchCount`、失败原因，以及可选的完整 `finalSQL`
+ 当 `sqlBytes >= AsyncWarnSQLBytes` 时打印 warn 日志并上报指标
+ 区分三类错误指标：`batch_split_by_internal_count_cap`、`batch_split_by_sql_bytes`、`batch_split_by_args`
+ 额外记录 `single_sql_oversize`，用于发现异常宽模块或异常字符串字段

与模块化模型的关系：

+ 模块化改造的第一收益来自“100 个属性写入收敛为 1~3 条模块 INSERT”，不是来自更激进的 SQL 聚合
+ 因此 V1 必须先保证“模块一行一写 + 安全批量 flush”；不要求业务层再做更复杂的跨模块批处理
+ 若后续压测显示仍需继续优化，只允许在 client 层继续增强“同一 flush 窗口内的小批量合并”；不得把 release 解析、字段分桶和批处理状态机重新上提到 `ThingLogic`

发布阶段补充校验：

+ 发布 `DmProductReleaseSchema` 前，除校验 identifier 命名外，还应估算“单模块单行写入”的最坏列数、最坏 SQL 长度和最坏行宽
+ 若模块定义已经明显逼近 TDengine 单行/单 SQL 上限，则发布应直接失败，而不是把风险留到运行时首次上报

### 2.5 查询模式
```sql
-- 单设备、单模块、全量点位
SELECT * FROM `device_module__{pid}__{dname}__{moduleCode}`
WHERE ts > ? ORDER BY ts DESC LIMIT ?;

-- 单设备、单点位
SELECT ts, {point} FROM `device_module__{pid}__{dname}__{moduleCode}`
WHERE ts > ?;

-- 跨产品：查询某个模块的所有设备
SELECT ts, {point}, product_id, device_name
FROM `model_module__{moduleCode}`
WHERE ts > now - 7d;

-- 按产品过滤
SELECT ts, {point}, device_name
FROM `model_module__{moduleCode}`
WHERE product_id = ? AND ts > ?;

-- 聚合
SELECT _wstart AS ts, AVG({point}) AS avg_val
FROM `model_module__{moduleCode}`
WHERE product_id = ?
INTERVAL(1h) FILL(PREV);
```

跨版本查询规则：

+ 同一 `moduleCode` 的历史数据始终落在同一张物理超级表中，标准查询不再因发布版本扇出到多张 STABLE
+ service 层仍需根据“设备 `schema_release_id_override` 或产品 `current_release_id`”解析生效的 `release_id`，再通过 `dm_product_release_schema` 决定当前查询应暴露哪些逻辑字段
+ 标准 API 默认只返回目标发布快照中可见的字段；已废弃/隐藏字段只在管理端调试、兼容导出等场景显式开放
+ 兼容性新增字段通过 `ADD COLUMN` 进入同一物理表；查询旧时间段时这些新列自然为空值，不需要切换物理表

### 2.6 模块模板演进与发布快照
```sql
-- 首次发布模块时创建物理 STABLE
CREATE STABLE IF NOT EXISTS `model_module__{moduleCode}` (...);

-- 兼容性新增字段时追加列
ALTER STABLE `model_module__{moduleCode}` ADD COLUMN `{new_point}` {TYPE};
```

版本规则：

+ 已发布的 `DmProductReleaseSchema` 快照不可变；模块模板后续修改不会影响已发布 release
+ TDengine 物理表采用“追加式演进”策略：一个 `ModuleCode` 默认只维护一张 STABLE，常规升级只允许 `ADD COLUMN`，不允许 `DROP COLUMN` / `CHANGE COLUMN`
+ 新增字段：必须先对 `model_module__{moduleCode}` 执行 `ADD COLUMN` 并确认成功，再提交 `dm_module_schema_info` 模板更新；若 `ALTER STABLE` 失败，则整次更新失败，不允许出现“SQL 模板已新增但 TDengine 物理列不存在”的临时不一致
+ 修改字段：必须表现为“废弃旧字段 + 新增新字段”；旧字段在新 release 快照中移除，新字段使用新的 `identifier` / 新列名
+ 删除字段：只从模块模板和新 release 快照中移除；物理列保留，默认不再对标准 API 暴露
+ 同一个 `identifier` 只能保持同一业务语义；禁止复用旧 `identifier` 表达新含义
+ 标准写入和查询永远以“解析出的 release 快照”决定字段可见性，以“moduleCode”决定物理表路由
+ `DmProductModule` 只管理产品草稿，不直接影响线上设备
+ `DmProductRelease` 不再以 `DmDeviceInfo.Version` 做运行时匹配；固件版本与 schema 发布快照在 V1 中完全解耦
+ 若某次变更已达到“新产品”判定条件，则应创建新的 `product_id`，而不是继续在原产品上累积 `product_release`
+ 老 release 只有在满足以下条件后才能退役：没有在线设备仍在上报该版本、且所需历史保留/导出已经完成（暂不实现）
+ 极少数无法通过追加式兼容处理的破坏性变更，才允许新建物理表；这不作为 V1 常规路径，默认应优先视为“新产品”或“新逻辑模块”

### 2.7 V1 存储收口
V1 作为新项目重构，时序属性统一收口到“模块超级表”这一条物理路径：

+ 超级表命名：`model_module__{moduleCode}`
+ 子表命名：`device_module__{pid}__{dname}__{moduleCode}`
+ 写入模式：每个采集批次按模块拆成 1~N 条 INSERT
+ TAGS：继续沿用现有设备维度标签，保证按产品/项目/区域过滤能力不变
+ 设备内建属性：保持现有专用表设计，不纳入模块超级表
+ 事件：保持现有事件表设计，不纳入本次改造

因此本方案不再同时维护“按属性超级表”与“按模块超级表”两套属性写入/查询链路；旧表迁移、双写或历史兼容若后续需要，必须另立专项方案。

### 2.8 SchemaStore 命名层
在 `schema.go` 中给 `SchemaStore` 增加新方法：

```go
func (s SchemaStore) GetModuleStableName(moduleCode string) string {
    return "model_module__" + SanitizeTablePart(moduleCode, 192-len("model_module__"))
}

func (s SchemaStore) GetModuleTableName(productID, deviceName, moduleCode string) string {
    return "device_module__" +
        SanitizeTablePart(productID, 48) + "__" +
        SanitizeTablePart(deviceName, 48) + "__" +
        SanitizeTablePart(moduleCode, 48)
}
```

legacy 命名方法继续保留。

命名转换规则：

+ **列名**：V1 不做运行时转义；要求业务侧 `identifier` 在建模阶段就满足 `^[a-z][a-z0-9_]{0,63}$`
+ **模块 code**：`moduleCode` 也要求满足同一安全字符集；若录入值不满足，管理端创建/更新模块时直接拒绝，不走自动纠正
+ **模块 code 命名规范**：V1 管理端应强制采用“全局唯一 + 可读前缀”的命名规范，推荐 `"{categoryPrefix}_{moduleName}"` 形式，例如 `inv_common`、`inv_alarm`、`ems_storage`
+ **前缀约束**：`categoryPrefix` 不是运行时路由字段，只是全局命名空间约定；其目的是在长期维护中降低跨品类模块命名冲突
+ **表名组成部分**（如 `productID`、`deviceName`）：允许来自现有业务字段，因此在进入 TDengine 表名之前必须执行 `SanitizeTablePart`

`SanitizeTablePart` 规则：

1. `TrimSpace`
2. 非 `[A-Za-z0-9_]` 字符统一替换为 `_`
3. 连续多个 `_` 压缩成单个 `_`
4. 去掉首尾 `_`
5. 若结果为空，替换为固定占位 `x`
6. 若首字符为数字，前面补前缀 `x_`
7. 若包含 `.`，同样按非法字符替换为 `_`
8. 按字节截断到调用方给定的 `maxLen`
9. 若截断后可能产生冲突，追加短 hash 后缀（例如 `_a1b2c3`），并再次保证总长度不超过 `maxLen`

冲突处理约束：

+ 表名生成不能依赖“仅做字符替换后自然唯一”
+ 若两个不同的 `productID/deviceName/moduleCode` 经 `SanitizeTablePart` 后得到相同结果，必须通过短 hash 后缀打散
+ 业务逻辑不应依赖从表名反向无损恢复原始 `productID/deviceName/moduleCode`

接口边界约束：

+ relationdb / 管理端逻辑：优先使用 `moduleID`
+ tsDB / 路由层：优先使用 `moduleCode`
+ `CreateModuleStable` / `AlterModuleStableAddColumns` / `DropModuleStable` 不携带 `categoryID`，因为物理 STABLE 只依赖全局唯一的 `moduleCode`
+ 物理表名使用双下划线 `__` 作为业务字段分隔符，避免 `productID` / `deviceName` / `moduleCode` 中普通下划线 `_` 造成歧义
+ 业务逻辑不应依赖“从物理表名反向解析出 productID/deviceName/moduleCode”；这些维度应来自 tags、入参或 relationdb，而不是字符串拆分
+ 若未来出现“同名模块只在分类内唯一”的需求，不在 V1 上兼容扩展；应改为新的全局 `moduleCode` 规范，而不是把 `categoryID` 混入物理表命名

---

## 3. 数据流
### 3.1 设备属性上报
```latex
设备 MQTT 上报
    → DeviceMsgHandle → ThingLogic.Handle()
    → HandleProperty()
    → 通过 DeviceSchemaRepo.GetData(dev, scope=periodic_decode) 加载该设备当前运行时 release 的 SchemaBundle
    → 将本次 MQTT/Modbus 上报视为一个采集批次（report batch）
    → 设备消息本身不要求携带模块版本
    → 从 DeviceSchemaRepo 一次性取回 SchemaBundle{Model, Route}
        · cache hit：直接命中，零 DB 往返
        · cache miss：按 §1.2 规则解析 release_id、装配模块 schema、构建 Route，
                     然后整体回填 cache
        · 若产品未发布或设备 override 非法：直接拒绝写入并记录指标，
                     严禁静默回退到其他 release 写入错误模块表
    → 仍然基于 Bundle.Model 对整个批次做 VerifyReqParam（形态不变）
    → 通过 Bundle.Route 逐 identifier 分桶，输出：
        a) 按 moduleCode 聚合的 ModuleParams 列表
        b) Unknown：未命中路由表的 identifier
    → 分桶过程是内存 O(N) map 查找，不发生任何额外 DB/Redis 查询
    → 对于模块属性：
        → 同一批次可命中多个模块；每个 moduleCode 产生一条 INSERT
        → 每条 INSERT 的列清单只包含 Route 中当前 release 快照可见的 identifier，
           废弃字段不进入列清单
        → 调用 GenInsertModuleSql(productID, deviceName, moduleCode, params)
        → 所有模块组共享同一业务 timestamp
        → AsyncInsert（每个 moduleCode 一条 SQL）
    → 对于 Unknown：
        → 记录日志/指标并丢弃，不自动建模，不自动扩列
        → 若本批次没有任何合法模块属性，则返回参数错误
    → PubApp / WebHook / UserSubscribe 行为保持不变
    → shadow 更新和 per-property latest/first cache 行为保持不变
```

### 3.2 Schema 校验
```go
// 一次批次校验 + 分桶的输出
type BatchSplit struct {
    Modules []ModuleParams             // 每个 moduleCode 一组
    Unknown map[string]Param           // 未命中路由表的 identifier
}

// 按模块分组后的参数
type ModuleParams struct {
    ModuleID   int64
    ModuleCode string
    // key 为 identifier；V1 中 identifier 同时就是 TDengine 物理列名
    Columns    map[string]Param
}

// 输入已是 SchemaBundle{Model, Route}，不再内部查 SQL
func VerifyAndSplitBatch(reqParams map[string]any, bundle SchemaBundle) (BatchSplit, error) {
    // 1. 基于 bundle.Model 做参数校验（类型/范围/必填，形态不变）
    // 2. 遍历 reqParams：
    //    - 命中 bundle.Route → 依据 ModuleCode 聚合到对应 ModuleParams，列名直接取 identifier
    //    - 未命中           → 放入 Unknown
    // 3. 对命中路由的条目，若该 identifier 不存在于当前 release 快照，
    //    按 Unknown 处理而不是写入（防止旧设备写新已删字段）
    // 4. 过程全内存，不发起 DB/Redis 查询
}
```

校验规则：

+ `ProductModuleMultiUpdate` 必须拒绝展平后标识符重复的模块组合
+ `ModuleSchemaCreate` / `ModuleSchemaUpdate` / `ModuleSchemaTslImport` 必须显式拒绝 `struct` / `array`
+ 运行时上报绝不能触发模块 schema 变更、模块扩列或自动建模
+ 未知 identifier 如果无法映射到任何已绑定模块，则记录日志/指标并丢弃；不能静默写入模块表
+ 若一次上报全部为未知 identifier，则返回参数错误，不写入任何模块数据

### 3.3 产品生命周期
**创建模块（管理端）：**

```latex
ModuleInfoCreate
    → 校验 moduleCode 满足 `^[a-z][a-z0-9_]{0,63}$`
    → 校验 moduleCode 满足命名规范（推荐带 categoryPrefix）
    → 按全局维度检查 dm_module_info.module_code 是否已存在
    → 若已存在则返回明确业务错误 `ErrModuleCodeConflict`，不向上暴露底层 duplicate key
    → DmModuleInfo insert
    → ModuleSchemaCreate 前校验：仅允许标量类型；若出现 struct/array 直接拒绝
    → ModuleSchemaCreate（批量）→ dm_module_schema_info insert（module_id + identifier）
    → 首次发布时 CreateModuleStable(moduleCode, properties) → TDengine CREATE STABLE
    → 不携带 categoryID；同一逻辑模块跨品类复用同一张 STABLE
```

**编辑模块 schema 模板（兼容性演进）：**

```latex
ModuleSchemaUpdate(moduleID)
    → 更新前校验：仅允许标量类型；若出现 struct/array 直接拒绝
    → 先计算本次新增字段集合
    → 若存在新增字段，则先对 model_module__{moduleCode} 执行 ALTER STABLE ADD COLUMN
    → 仅在全部 ALTER STABLE 成功后，才提交 dm_module_schema_info 当前模板更新
    → 若任一 ALTER 失败（TDengine 不可用、列名冲突、超列数/行宽限制等），则本次更新整体失败，dm_module_schema_info 保持旧状态
    → 若存在废弃/删除字段，则只影响后续新发布的 release，不做物理 DROP COLUMN
```

**编辑产品草稿（管理端）：**

```latex
ProductInfoCreate
    → ProductModuleMultiUpdate → DmProductModule insert/update（product_id + module_id）
    → 仅更新草稿，不直接影响线上设备
```

**发布产品版本（管理端）：**

```latex
ProductReleasePublish(productID, releaseNo)
    → 读取 DmProductModule 草稿
    → 读取 DmModuleInfo，拿到 module_id / module_code / module_kind；ModuleKind 只用于校验和 scope 派生，不复制到 DmProductReleaseSchema
    → 读取每个 moduleID 对应的 dm_module_schema_info 当前模板
    → 校验 release 内所有字段类型均属于允许的标量类型全集；若存在 struct/array 则禁止发布
    → 校验模块 kind 与点位定义类型匹配
    → 校验展平后 identifier 全局唯一
    → 校验每个 identifier 均满足 TDengine 列名约束 `^[a-z][a-z0-9_]{0,63}$`
    → 估算每个 moduleCode 的最坏列数 / 最坏单行宽度 / 最坏 INSERT SQL 长度；超阈值则禁止发布
    → 前置校验发布号唯一：校验 `(product_id, release_no)` 是否已存在
    → DmProductRelease insert（product_id + release_no）
    → 若 insert 命中 unique index 冲突，则返回 `ErrReleaseNoConflict`
    → DmProductReleaseSchema bulk insert（release_id + product_id + module_id + module_code + identifier + DmSchemaCore；不含 module_kind）
    → 完成。新 release 只是可被引用的已发布快照，不自动影响线上设备
```

**设置产品默认发布版本（管理端）：**

```latex
ProductReleaseSetDefault(productID, releaseID)
    → 校验 releaseID 属于同一 productID
    → 校验 releaseID 已发布且可用
    → 校验必要的 TDengine 物理结构和 SchemaBundle 均已准备完成
    → 更新 dm_product_info.current_release_id
    → 淘汰 things:schema:product:current:{productID}:*
    → 后续未设置 override 的设备才会跟随新的 current release
```

**设备 schema 快照覆盖（管理端）：**

```latex
DeviceSchemaReleaseOverrideUpdate(productID, deviceName, releaseID)
    → 校验 releaseID 属于同一 productID
    → 校验 releaseID 已发布且可用
    → 更新 dm_device_info.schema_release_id_override = releaseID
    → 清理对应设备档案缓存
    → 后续 `DeviceSchemaRepo.GetData()` 固定命中该指定快照；该快照可以是新 release，也可以是历史 release
```

**清除设备 schema 快照覆盖（管理端）：**

```latex
DeviceSchemaReleaseOverrideClear(productID, deviceName)
    → 更新 dm_device_info.schema_release_id_override = 0
    → 清理对应设备档案缓存
    → 后续 `DeviceSchemaRepo.GetData()` 恢复跟随产品 current_release_id
```

**设备版本同步：**

```latex
设备上线 / ReportInfo
    → 更新 dm_device_info.version（整机版本）
    → 清理对应设备档案缓存
    → 版本变化只影响 OTA / 展示 / 诊断元数据，不直接改变 schema 路由
```

**OTA 成功后绑定 schema release：**

```latex
OtaJobSuccessCallback(productID, deviceName, version, targetSchemaReleaseID)
    → 更新 dm_device_info.version（整机版本）
    → 若 targetSchemaReleaseID > 0，则校验 releaseID 属于同一 productID 且已发布可用
    → 更新 dm_device_info.schema_release_id_override = targetSchemaReleaseID
    → 清理对应设备档案缓存
    → 后续该设备固定命中新 schema release
```

**设备首次上报：**

```latex
INSERT...USING 自动创建各模块子表。不需要显式 InitDevice。
```

### 3.4 Redis Cache
Schema cache 是本方案性能的基石：所有写入/查询路径的"点位 → 模块 → 物理表/列"解析都必须在 schema cache 中一次完成，**不允许在热路径上回源 SQL 做路由**。

实现约束必须明确：

+ 新的 release-aware schema cache **不再继续扩展旧的 **`iot/rpc/things/internal/cache/schemaRepo.go`** 中的 **`ristretto`** 单机实现**
+ V1 统一复用现有 `saas/share/caches.Cache` 二级缓存框架：L1 为 `otter`，L2 为 `redis`，L3 才是 DB / release snapshot 组装
+ 因此本方案不是“在旧 `ristretto` schema cache 之上再叠一层 Redis”，而是“用通用 otter+redis 二级缓存替换旧 `SchemaRepo` 主链实现”
+ 下文中 `things:schema:product:current:*`、`things:schema:product:release:*` 表示**逻辑 key 语义**；实际落到 `caches.Cache` 后，Redis key 形如 `cache:{KeyType}:{key}`

同时，现有 `cache:things:device` 存的是完整 `DeviceInfo` 详情对象，包含产品补充字段、图片/文件 URL、分享展示字段等大对象内容；它不适合作为百万、千万级活跃设备消息热路径的统一缓存。V1 必须把"设备运行时元信息"、"设备详情视图"和"设备密钥"拆成三类职责明确的缓存。

```latex
-- [强制] Schema Bundle：flat Model + identifier 路由表，一起缓存
things:schema:product:current:{productID}:{scope}
    → SchemaBundle { Model: schema.Model, Route: map[identifier]SchemaRouteEntry }
    → 等于 current_release_id 对应的 scoped bundle
    → 由 ProductSchemaRepo.GetCurrent(productID, scope) 统一填充
    → 实际实现：`caches.Cache[SchemaBundle, ProductReleaseKey]{KeyType: "things:schema:product:current"}`
    → 实际 Redis key：`cache:things:schema:product:current:{productID}:{scope}`

things:schema:product:release:{productID}:{releaseID}:{scope}
    → SchemaBundle { Model, Route, Meta.ReleaseID=releaseID }
    → 仅表示一个精确发布快照的 scoped bundle
    → 由 ProductSchemaRepo.GetByRelease(productID, releaseID, scope) 统一填充
    → 实际实现：`caches.Cache[SchemaBundle, ProductReleaseKey]{KeyType: "things:schema:product:release"}`
    → 实际 Redis key：`cache:things:schema:product:release:{productID}:{releaseID}:{scope}`

-- [强制] 设备运行时元信息缓存：热路径唯一设备档案来源
schema/device runtime meta:
cache:things:device:runtime:{productID}:{deviceName}
    → DeviceRuntimeMeta {
        TenantCode, ProjectID, AreaID, AreaIDPath,
        ProductID, DeviceName,
        Version,
        SchemaReleaseIDOverride,
        IsOnline, IsEnable, Status, DeviceType,
        FirstLogin, LastBind,
        UserID,
        BelongGroup, Tags
      }
    → 仅包含运行时热路径需要的轻字段
    → 给 DeviceSchemaRepo.GetData / ThingLogic / Auth / CheckIsOnline / online fix / 日志过滤 等路径使用
    → 实际实现继续复用 `caches.Cache`，不单独维护第三套本地缓存

-- [新增] OTA 待确认状态缓存：仅服务 OTA 待确认/确认升级链路
cache:things:device:ota_confirm:{productID}:{deviceName}
    → DeviceOtaConfirmMeta { NeedConfirmVersion, NeedConfirmJobID }
    → 只给 OTA 任务创建、任务取消、固件删除、设备确认升级等链路使用
    → 不进入 schema resolve / Thing / Auth / CheckIsOnline 等热路径，避免 OTA 状态变更频繁淘汰 runtime meta
    → 该缓存字段属于“低频但非静态”状态，和 `Version/IsOnline/ProjectID` 这类通用运行时元信息拆开维护

-- [保留] 设备详情缓存：后台详情页 / 分享 / 导入 / 展示视图使用
cache:things:device:{productID}:{deviceName}
    → DeviceInfo
    → 包含 DeviceAlias / ProductName / ProductImg / Gateway / SchemaAlias / 文件图片等详情字段
    → 不作为 Thing / 鉴权 / schema resolve / 在线判断 的热路径依赖

-- [新增] 设备密钥缓存：认证 / 网关注册签名校验专用
cache:things:device:secret:{productID}:{deviceName}
    → DeviceSecretInfo { ProductID, DeviceName, Secret }
    → 仅给设备认证、网关自动注册 / 绑定签名校验使用

-- [强制] 不做 Redis 级别的设备 SchemaBundle 缓存
-- DeviceSchemaRepo.GetData(scope) = 读取 `cache:things:device:runtime` -> 取 SchemaReleaseIDOverride -> override 命中则 GetByRelease(..., scope)，否则走 things:schema:product:current:{productID}:{scope}
-- V1 不额外维护“设备最终 release 结果”的 Redis 缓存；只缓存产品 current/release bundle 和设备 override 元信息

-- [强制] Schema Bundle 失效规则（任一事件触发对应淘汰）
| 事件                                                    | 淘汰范围                                   |
|--------------------------------------------------------|-------------------------------------------|
| dm_module_schema_info 新增字段并 ALTER STABLE 成功      | 后续新发布 release 可见；已发布 release 不变 |
| dm_product_release / dm_product_release_schema 变更     | 对应 release；若该 release 正是 current，则同时淘汰 product current |
| dm_product_info.current_release_id 变更                 | 对应 product current                      |
| dm_device_info.schema_release_id_override 变更 | 不需要淘汰 Redis SchemaBundle；清理/回填 `cache:things:device:runtime` 后自然切换到 override 或 current |
| dm_device_info.version 变更 | 不淘汰 Redis SchemaBundle；只清理/回填设备运行时元数据，schema 是否切换取决于 `schema_release_id_override` 是否同时变化 |

-- [保留] 现有 per-property cache，作为 legacy 兼容 API 的权威来源
device:thing:property:last:{productID}:{deviceName}
device:thing:property:first:{productID}:{deviceName}
    → PropertyLatestIndex / GetStatus / diff 检查逻辑保持不变
    → 新模型下写入后仍按 identifier 维度回填，确保现有 API 语义不变

-- [可选] 模块维度 latest cache，仅服务新的 DeviceModuleDataLatest API
device:thing:module:last:{productID}:{deviceName}:{moduleCode}
    → 保存 {releaseID, values}
    → values 只包含当前解析出的 release 快照可见点位
    → 与 per-property last cache 同时更新；二者是视图关系，不是主从关系

-- ProductSchemaIndex（列产品点位）读取：
    → 从 things:schema:product:current:{productID}:full.Model 直接返回
```

缓存一致性要点：

+ **Bundle 原子性**：Model 和 Route 必须作为同一个 value 原子读写，禁止分两次 Redis 调用拼装，否则会出现 Model 已更新但 Route 仍指向旧物理列的窗口
+ **失效优先于回填**：所有 schema 变更事务必须"先发 cache 失效、后返回成功"；回填由下一次读请求按需触发，避免双写不一致
+ **失败不失效**：若 `ALTER STABLE` / 模板更新 / 发布快照复制任一步失败，不得提前清理 current/release cache；只有数据库模板更新和必须的物理变更都成功后，才允许发失效事件
+ **release 语义隔离**：`things:schema:product:current:*` 和 `things:schema:product:release:*` 是不同层次的视图；`current_release_id` 变化不应污染已精确解析 release 的设备运行时结果
+ **热路径零 SQL**：`HandleProperty`、`PropertyLogIndex`、`DeviceModuleDataIndex` 等热路径只允许读 SchemaBundle；若 Route 中找不到某 identifier，直接按"未知 identifier"处理，不回源 SQL 二次确认
+ **跨进程失效**：dmsvr 多副本部署时，schema 变更后必须通过 Redis pub/sub 或 etcd watch 通知所有副本淘汰本地进程缓存（如有），仅淘汰 Redis 不足以保证强一致
+ **统一缓存框架**：schema current/release、runtime meta、detail、secret 全部落在 `saas/share/caches.Cache` 上，避免 `ristretto`、`otter`、手写 Redis key 三套实现并存
+ **避免大基数缓存**：`schema:device:{productID}:{deviceName}` 这类“一设备一份 SchemaBundle”的 Redis 常驻缓存不进入 V1；海量设备场景只缓存产品 release bundle
+ **设备档案缓存拆分**：`cache:things:device:runtime` 是热路径唯一设备档案来源；`cache:things:device` 退化为后台详情缓存；认证和网关签名校验使用独立 `cache:things:device:secret`；OTA 待确认状态使用独立 `cache:things:device:ota_confirm`
+ **轻缓存不做详情拼装**：`DeviceRuntimeMeta` 组装阶段禁止生成 OSS 签名 URL、禁止拼 `ProductImg` / `Gateway` / `SchemaAlias` 这类大字段，避免把后台详情成本带入消息热路径
+ **设备 override 只动轻缓存**：`dm_device_info.schema_release_id_override`、`is_online`、`project/area/group` 等通用运行时字段变更，优先淘汰/回填 runtime meta；不需要因此为每台设备维护 Redis 级别 SchemaBundle
+ **OTA 状态隔离**：`need_confirm_job_id`、`need_confirm_version` 只淘汰/回填 `cache:things:device:ota_confirm`，不应连带淘汰 `cache:things:device:runtime`
+ **secret 单独隔离**：设备 `Secret` 不进入通用运行时元信息缓存，避免控制、上报、日志链路无意义地携带密钥字段
+ **外部消费者一致性**：`ekuiper-cluster` 这类外部消费者若缓存设备级 TSL，必须订阅 release 相关变更；触发条件至少包括 `dm_device_info.schema_release_id_override`、`dm_product_info.current_release_id`、`dm_product_release_schema` 变更以及所依赖模块 schema 变更。`dm_device_info.version` 单独变化不触发 schema cache 失效

设备档案缓存迁移顺序：

+ 第一批切换到 `DeviceRuntimeMetaCache`：`ThingLogic`、`Auth`、`CheckIsOnline`、`WithDeviceTenant`、`HandleOnlineFix`
+ 第二批切换到 `DeviceRuntimeMetaCache`：日志查询、告警处理、规则引擎兼容所依赖的 release 解析链路
+ OTA 待确认/确认升级链路切换到独立 `DeviceOtaConfirmMetaCache`
+ `DeviceInfoRead` / `DeviceInfoIndex` / 分享 / 导入 等需要 `DeviceAlias`、`ProductImg`、`Gateway`、`SchemaAlias` 的后台接口继续使用 `cache:things:device`
+ 网关自动注册、设备认证、签名校验链路改读 `cache:things:device:secret`
+ 迁移完成后，代码层面应将当前 `DeviceCache` 重命名为 `DeviceDetailCache`，避免后续新逻辑继续误把详情缓存接入热路径

### 3.5 规则引擎兼容（ekuiper-cluster）
背景：

+ `ekuiper-cluster` 负责订阅设备 MQTT 消息，`acxPushDecode` 将 Modbus 二进制数据块解码为 `iot` 可直接读取的扁平 JSON
+ 当前 `ps_lookup` 通过 `productID` 拉取产品级 TSL；该方案只适用于“一个 `product_id` 只有一个有效 schema”的 legacy 模型
+ 在模块化产品下，同一 `product_id` 下可能并存多个 release，因此规则引擎**不能**再以 `productID` 作为子设备解码 schema 的唯一键

兼容原则：

+ 规则引擎继续只消费扁平 TSL，不感知 `moduleCode`、`SchemaBundle.Route`
+ release 解析只在 `iot` 服务内进行，`ekuiper-cluster` 不复制一套 release resolve 逻辑
+ `iot` 对规则引擎暴露的兼容形态仍然是 `bundle.Model.String()`；也就是“设备最终运行时视图”的扁平 TSL JSON

HTTP 接口改造：

+ 当前 `ps_lookup` 依赖 3 个 iot HTTP 接口：
    - `dgHttpUrl = POST /api/v1/things/device/auth/gateway-subset`
    - `diHttpUrl = POST /api/v1/things/device/auth/read`
    - `psHttpUrl = POST /api/v1/things/product/auth/schema`
+ 其中 `gateway-subset` 只负责返回网关下的 `{deviceName: productID}` 子设备清单，该语义在新模型下仍成立，可以保留
+ `device/auth/read` 当前返回完整 `DeviceInfo` JSON；`ps_lookup` 实际只需要网关 `timeZone`、子设备 `RS485_ADDR` 和该设备当前生效的 `effectiveReleaseID`，继续逐设备拉完整 `DeviceInfo` 会造成过重的返回体和 N 次重复调用
+ `product/auth/schema` 当前只按 `productID` 返回产品 current view；在 release-aware 模型下，它不能再作为在线子设备二进制解码的主 schema 来源
+ 因此，V1 对 `ps_lookup` 的接口组合调整为：
    - 保留：`POST /api/v1/things/device/auth/gateway-subset`
    - 新增：`POST /api/v1/things/device/auth/runtime-meta-batch`
        * 批量输入一组 `{productID, deviceName}`
        * 批量返回轻量运行时元数据 `{productID, deviceName, effectiveReleaseID, schemaReleaseIDOverride, version, timeZone, rs485Addr}`
        * 其中 `effectiveReleaseID` 由服务端按 `schema_release_id_override ? override : current_release_id` 实时计算
        * 供 `ps_lookup` 一次拉取“网关本体 + 所有子设备”的 runtime meta，替代逐设备 `device/auth/read`
    - 新增：`POST /api/v1/things/device/auth/runtime-schema-batch`
        * 批量输入去重后的 `{productID, releaseID, scope}` 列表
        * 批量返回对应 runtime schema `tsl`
        * 响应必须支持部分成功；若 `releaseID` 不存在、跨产品或已不可用，则返回对应业务错误，而不是整批失败
+ 旧接口处理策略：
    - `device/auth/read` 保留给 legacy 调试、认证或其他后台调用方，不再作为 `ps_lookup` 主链路
    - `product/auth/schema` 保留“产品 current view / 点位列表 / 默认展示”语义，不再作为 `ps_lookup` 在线解码入口
    - `device/schema/tsl-read` 仅作为最后兜底或调试接口；`ps_lookup` 不应把“逐设备调用 `DeviceSchemaTslRead`”作为常规主方案

读取路径调整：

+ `ProductSchemaTslRead(productID)` / 产品级 schema cache 仅表示 `ProductSchemaRepo.GetCurrent(productID, full).Model.String()`，用于产品详情、产品点位列表、默认展示
+ `ekuiper-cluster` 对子设备二进制解码时，语义上必须使用“设备命中的 runtime schema”，但 lookup 方式不能简单退化成“每个子设备单独调一次 `DeviceSchemaTslRead`”
+ `ps_lookup` 应先通过 `gateway-subset` 获取网关子设备清单，再通过 `runtime-meta-batch` 一次获取网关本体和子设备的 runtime meta，从中拿到 `productID + effectiveReleaseID`
+ 然后按 `(productID, effectiveReleaseID, scope)` 分组复用 schema；同一网关下若 50 个子设备实际只分布在 2~3 个生效快照和同一个 scope 上，则 schema 拉取次数应收敛到 2~3 次，而不是 50 次
+ 推荐优先级：
    - 第一选择：`iot` 提供按 `(productID, releaseID, scope)` 批量读取 schema 的 `runtime-schema-batch` 接口，供 `ps_lookup` 一次拉取多个 release 组
    - 第二选择：`ps_lookup` 本地先按 `(productID, effectiveReleaseID, scope)` 去重，再逐组调用单组 schema 读取接口
    - 最后兜底：只有拿不到 `effectiveReleaseID` 或单组读取失败时，才逐设备调用 `DeviceSchemaTslRead(productID, deviceName, withProductSchema=true)`
+ 服务端内部仍通过 `cache:things:device:runtime -> schema_release_id_override ? ProductSchemaRepo.GetByRelease(..., scope) : ProductSchemaRepo.GetCurrent(..., scope)` 解析，不要求 Redis 设备级 bundle cache
+ `DeviceSchemaTslRead` 在模块化产品下必须返回 `DeviceSchemaRepo.GetData(dev, scope).Model.String()`，即该设备解析出的 runtime release 对应 TSL
+ `ps_lookup` 不能再对所有子设备复用同一个 product schema；但可以对“同一 `(productID, effectiveReleaseID, scope)` 组”的子设备复用同一份 runtime schema

性能要求：

+ 要求评估 `ps_lookup` 的调用频率、单次网关消息下的子设备数量分布、冷缓存/热缓存命中率和 `iot` schema 读取 P95/P99 延迟
+ 在网关下挂大量同快照子设备的常见场景中，schema lookup 次数应与“release 组数”近似，而不是与“子设备数”线性相关
+ `ps_lookup` 切换到 batch 接口后，冷缓存场景下单次网关消息的远程 HTTP 调用应优先收敛到：
    - 1 次 `gateway-subset`
    - 1 次 `runtime-meta-batch`
    - 1 次 `runtime-schema-batch`  
而不是 `1 + 子设备数 + 子设备数` 的线性放大模型
+ 若压测表明逐组串行读取仍有明显尾延迟，应补充批量 schema 读取 RPC，而不是把逐设备直调继续向下堆缓存

规则引擎缓存形态：

+ 规则引擎不应维护无界的 `(productID, deviceName) -> schema.Model` 常驻缓存
+ 若需要缓存，优先按 `(productID, releaseID)` 做小基数缓存；只有拿不到这些信息时，才允许按 `(productID, deviceName)` 做**有界 LRU/TTL** 本地缓存
+ 规则引擎不直接消费 `SchemaBundle`，只缓存 `SchemaBundle.Model`
+ 原有 product schema cache 可以保留，但仅用于产品级展示/调试，不再作为在线二进制解码的主 schema 来源
+ `ps_lookup` 内部应优先命中 `(productID, releaseID)` 维度的小基数缓存，避免同一网关消息内对相同生效快照子设备重复发起 schema 请求

解码与失败处理：

+ `acxPushDecode` / `binaryTransform` 的入参契约保持不变，仍然只依赖扁平 schema 中的 `properties/events/extendConfig`
+ 当某个子设备出现以下任一情况时：
    - schema 不存在
    - `DeviceSchemaTslRead` 返回 `product_unpublished`
    - `DeviceSchemaTslRead` 返回 `device_release_override_invalid`
    - schema 反序列化失败
    - 该子设备对应的二进制块解码失败  
规则引擎**必须跳过该子设备，继续处理其他子设备和采集器本体数据**
+ 跳过策略要求：
    - 记录日志和指标，带上 `productID`、`deviceName` 和失败原因
    - 从输出 payload 的 `subDevices` 中移除该子设备；**不能**保留一个空壳 subDevice 占位
    - 其他子设备继续正常解码并输出
    - 若所有子设备都被跳过，但采集器本体消息仍合法，则整包仍然输出，只是 `subDevices=[]`
    - 只有在 MQTT 消息整体结构非法、采集器关键信息缺失、或整包无法构造基础 event payload 时，才允许整包失败

这样做的目的：

+ 避免“某一台设备 schema/release 异常”拖垮整个网关上报批次
+ 避免规则引擎因复用错误的产品级 schema 而把二进制块解码成错误 JSON
+ 保持 `iot` 与 `ekuiper-cluster` 之间“扁平 JSON”契约不变，降低模块化改造对规则引擎的侵入

---

## 4. API / RPC 接口
### 4.1 新增接口
| RPC | 说明 |
| --- | --- |
| `ModuleInfoCreate` | 创建模块定义 |
| `ModuleInfoUpdate` | 更新模块信息 |
| `ModuleInfoDelete` | 在所有 schema 版本都退役后删除逻辑模块定义 |
| `ModuleInfoIndex` | 模块列表 |
| `ModuleInfoRead` | 读取模块详情 |
| `ModuleSchemaCreate` | 创建模块 schema 模板 |
| `ModuleSchemaUpdate` | 更新模块 schema 模板 |
| `ModuleSchemaDelete` | 删除未被产品草稿引用的模块 schema 模板 |
| `ModuleSchemaIndex` | 查询模块点位列表（带 lang_key i18n） |
| `ModuleSchemaTslImport` | 从 Excel / TSL 批量导入模块点位 |
| `CategoryModuleMultiUpdate` | 绑定/解绑品类可用模块 |
| `CategoryModuleIndex` | 查询品类可用模块列表 |
| `ProductModuleMultiUpdate` | 编辑产品模块草稿（module id 集合） |
| `ProductModuleIndex` | 查询产品模块草稿 |
| `ProductReleasePublish` | 基于产品模块草稿发布一个产品版本快照 |
| `ProductReleaseIndex` | 查询产品发布版本列表 |
| `ProductReleaseRead` | 读取产品发布版本详情 |
| `ProductReleaseSetDefault` | 设置产品默认发布版本 |
| `DeviceModuleDataIndex` | 查询设备模块时序数据（物理上按 `moduleCode` 单表查询，逻辑上按版本投影） |
| `DeviceModuleDataLatest` | 查询设备模块最新数据（返回当前解析版本下可见字段） |


### 4.2 现有接口兼容策略
| 现有接口 | 处理方式 |
| --- | --- |
| `ProductSchemaRepo.GetCurrent` / `ProductSchemaRepo.GetByRelease` / `DeviceSchemaRepo.GetData` | 统一返回 `SchemaBundle{Model, Route}`：产品级明确区分 current view 与 exact release view；设备级 `GetData` 返回该设备解析出的运行时 release bundle |
| `ReportInfo` / `DeviceInfoUpdate` | 继续维护 `DmDeviceInfo.Version` 作为固件版本元数据；schema 路由改由 `schema_release_id_override + current_release_id` 决定，版本变化本身不再驱动 release 解析 |
| `ProductSchemaIndex` | 直接从 `ProductSchemaRepo.GetCurrent(productID, full).Model` 输出产品当前发布视图 |
| `PropertyLogIndex` (N 个 dataID 的历史批查) | 单设备查询走 `DeviceSchemaRepo.GetData(dev, periodic_decode)`；产品/品类级默认走 `ProductSchemaRepo.GetCurrent(productID, periodic_decode)`；若需查看某个精确发布版本的字段集，则显式传 `releaseID` 并走 `GetByRelease(..., periodic_decode)` |
| `ProductSchemaTslRead` / 产品级 schema cache | 在模块化产品下仅返回 `GetCurrent(productID).Model.String()`；适合产品默认视图，不适合作为在线设备二进制解码 schema 来源 |
| `DeviceSchemaTslRead` / 设备级 schema 读取 | 在模块化产品下返回 `DeviceSchemaRepo.GetData(dev, scope).Model.String()`；`acxPushDecode` 使用 `periodic_decode`，主动读取使用 `active_read` |
| `PropertyLatestIndex` | 继续读取现有 per-property latest cache；请求形态不变 |
| `HandlePropertyGetStatus` | 继续沿用现有 per-property latest cache / shadow 协议 |
| 所有对外 RPC | 入参/出参均以扁平 identifier 为契约；`SchemaBundle.Route` 是内部结构，不允许通过任何 RPC 暴露 |


### 4.3 SchemaDataRepo 接口扩展
```go
type SchemaDataRepo interface {
    // === 保留现有方法 ===
    InsertPropertiesData(...)
    GetPropertyDataByID(...)
    InitProduct(...)
    CreateProperty(...)
    // ...

    // === 新增：模块方法 ===
    // 注意：
    // 1. tsDB 物理接口全部使用 moduleCode，不使用 moduleID/categoryID
    // 2. moduleID 仅用于 relationdb 关联和管理端逻辑
    CreateModuleStable(ctx context.Context, moduleCode string, properties []schema.Property) error
    AlterModuleStableAddColumns(ctx context.Context, moduleCode string, properties []schema.Property) error
    DropModuleStable(ctx context.Context, moduleCode string) error
    CreateModuleTable(ctx context.Context, productID, deviceName, moduleCode string) error
    DropModuleTable(ctx context.Context, productID, deviceName, moduleCode string) error
    InsertModuleData(ctx context.Context, productID, deviceName, moduleCode string,
                     params map[string]Param, ts time.Time, optional Optional) error
    GetModuleDataByFilter(ctx context.Context, moduleCode string,
                          filter FilterOpt) ([]*ModuleData, error)
    GetModulePropertyByID(ctx context.Context, moduleCode, dataID string,
                          filter FilterOpt) ([]*PropertyData, error)
    GetLatestModulePropertyByID(ctx context.Context, moduleCode, dataID string,
                                filter LatestFilter) (*PropertyData, error)
    GetLatestModuleData(ctx context.Context, productID, deviceName, moduleCode string) (*ModuleData, error)
}
```

---

## 5. 关键改动文件
| 文件 | 改动 |
| --- | --- |
| `iot/rpc/things/internal/relationdb/model/model.go` | 增加 `DmModuleInfo`、`DmCategoryModule(moduleID)`、`DmProductModule(moduleID, draft)`、`DmProductRelease`、`DmProductReleaseSchema`、`DmModuleSchemaInfo(moduleID+identifier)`；给 `DmProductInfo` 增加 `CurrentReleaseID`，给 `DmDeviceInfo` 增加 `SchemaReleaseIDOverride`；`DmSchemaInfo` 保持 legacy-only |
| `iot/rpc/things/internal/relationdb/repo/` | 新增 `moduleInfo.go`、`moduleSchema.go`、`categoryModule.go`、`productModule.go`、`productRelease.go`（Repo CRUD、发布快照复制与物化辅助） |
| `iot/rpc/things/internal/relationdb/repo/assemble.go` | 增加 `DmProductReleaseSchema` 到扁平 `schema.Model` 的组装逻辑，以及 identifier 冲突 / 命名约束校验 |
| `iot/pkg/domain/deviceMsg/msgThing/repo.go` | 为 `SchemaDataRepo` 增加模块方法 |
| `iot/rpc/things/internal/svc/service_context.go` | 将 schema cache 拆分为 `ProductSchemaRepo.GetCurrent(productID, scope)`、`ProductSchemaRepo.GetByRelease(productID, releaseID, scope)` 两类 `caches.Cache` 入口；新增 `DeviceRuntimeMetaCache`、`DeviceOtaConfirmMetaCache`、`DeviceDetailCache`、`DeviceSecretCache` 四类设备档案缓存；`DeviceSchemaRepo.GetData(dev, scope)` 内部改为 `cache:things:device:runtime -> schema_release_id_override ? GetByRelease(..., scope) : GetCurrent(..., scope)` |
| `iot/rpc/things/internal/startup/init.go` | 初始化 `DeviceRuntimeMetaCache` / `DeviceOtaConfirmMetaCache` / `DeviceDetailCache` / `DeviceSecretCache`，并把现有 `DeviceCache` 的初始化逻辑拆出；运行时轻缓存、OTA 待确认缓存和后台详情缓存分别配置 TTL/失效策略 |
| `iot/rpc/things/internal/logic/assembleDevice.go` | 将完整 `DeviceInfo` 的详情拼装保留在 detail cache 路径；禁止在 runtime meta cache 组装中生成签名 URL、产品图片、文件地址等大字段 |
| `iot/pkg/domain/schema/` | 新增 `SchemaBundle`、`SchemaRouteEntry` 类型定义，作为 repo/cache/tsDB 之间的公共契约 |
| `iot/rpc/things/internal/cache/schemaRepo.go` | 旧 `ristretto` schema cache 仅保留 legacy 兼容；新主链 schema cache 改为基于 `saas/share/caches.Cache` 统一读写产品 current/release 两类 `SchemaBundle`；V1 不新增设备级“最终 release 结果”缓存；实现 §3.4 中列出的失效逻辑，包含跨副本广播 |
| `saas/share/caches/cache.go` | 复用现有 otter(L1)+redis(L2)+DB/L3 二级缓存框架承载 `SchemaBundle`、`DeviceRuntimeMeta`、`DeviceOtaConfirmMeta` 等新缓存类型；必要时补 KeyType、NoExpire 和预热/失效辅助逻辑 |
| `saas/share/conf/database.go` | 为 `TSDB` 增加 `AsyncFlushMs`、`AsyncMaxSQLBytes`、`AsyncMaxArgs`、`AsyncWarnSQLBytes` 配置项；条数上限仅保留为 client 内部保护常量，不作为公开配置 |
| `saas/share/clients/tdEngine.go` | `AsyncInsert` 从“主要按条数 flush”改为“主要按 SQL 字节数 + 参数个数拆批”；实现内部可保留固定 `hardMaxBatchCnt` 作为公平性保护；单条 SQL 超限时直接报错并打指标 |
| `saas/share/clients/tdEngine_batch_test.go` | 新增纯单元测试，覆盖 `sqlBytes/args` 拆批、内部 count cap、单条超限、边界值、`sqlBytes == len(finalSQL)`、`joinOverhead` 计算和 SQL/args 顺序稳定性 |
| `iot/rpc/things/internal/logic/devicemanage/device_schema_tsl_read_logic.go` | `DeviceSchemaTslRead` 改为返回设备运行时 release 对应的 `bundle.Model.String()`，供 `ekuiper-cluster` 逐设备解码使用 |
| `iot/rpc/things/internal/logic/productmanage/product_schema_tsl_read_logic.go` | `ProductSchemaTslRead` 明确只返回产品 current view，不再承诺可用于在线设备解码 |
| `iot/api/gateway/apifile/things/device/auth.api` | 为规则引擎新增 `runtime-meta-batch`、`runtime-schema-batch` 两个 auth 侧 batch 接口；`gateway-subset` 保留；`read` 不再作为 `ps_lookup` 主链路 |
| `iot/api/gateway/internal/logic/things/device/auth/` | 新增 `runtime_meta_batch_logic.go`、`runtime_schema_batch_logic.go`；前者批量返回轻量设备运行时元数据和 `effectiveReleaseID`，后者按 `(productID, releaseID)` 批量返回 runtime schema |
| `iot/api/gateway/apifile/things/product/auth.api` | `product/auth/schema` 保留产品 current view 语义，但从规则引擎在线解码主链移除 |
| `iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/schema.go` | 增加不带版本的 `GetModuleStableName`、`GetModuleTableName`；V1 直接使用 `identifier` 作为 TDengine 列名，不再维护独立 `dataID -> column` 映射 |
| `iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/` | 新增 `moduleData.go`（InsertModuleData、GetModuleData、GenInsertModuleSql、模块表上的 latest-property 查询），并支持追加列 |
| `iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/productManage.go` | 增加 CreateModuleStable / AlterModuleStableAddColumns / DropModuleStable |
| `iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/deviceManage.go` | 增加不带版本的 CreateModuleTable / DropModuleTable |
| `iot/rpc/things/internal/event/handler/deviceMsgEvent/thing.go` | 删除 `DeviceSchemaReportAutoCreate` 及其二次加载 schema 分支；`HandleProperty` 改为通过 `SchemaBundle.Route` 在内存中分桶（零 DB），只拆分模块属性 / 未知 identifier 两路；未知 identifier 仅记录日志/指标并丢弃，若整批无合法字段则直接报错 |
| `iot/rpc/things/internal/logic/devicemsgmanage/property_log_index_logic.go` 等查询 logic | 批量点位查询时通过 `SchemaBundle.Route` 按 `ModuleCode` 分桶，N 条 SQL 合并为 M 条；单点位查询一次 map 查找直达目标物理表 |
| `ekuiper-cluster/extensions/impl/cloudx/device/cache.go` | 若需要缓存设备级 TSL，改为按 `(productID, releaseID)` 做小基数缓存；按设备名缓存时必须是有界 LRU/TTL 本地缓存 |
| `ekuiper-cluster/extensions/impl/cloudx/ps_lookup.go` | 子设备 schema lookup 从 `gateway-subset + device/auth/read + product/auth/schema` 改为 `gateway-subset + runtime-meta-batch + runtime-schema-batch`；实现上优先按 `(productID, effectiveReleaseID, scope)` 分组去重/批量读取，避免同一网关下按子设备线性放大 schema 请求；`acxPushDecode` 固定 `scope=periodic_decode`，`acxThingForward` / `acxThingForwardResponse` 固定 `scope=active_read`；单台失败时返回 skip 而非整批失败 |
| `ekuiper-cluster/extensions/functions/acxPushDecode/handle_push_data.go` | 遇到子设备 schema 缺失、release 解析失败或单台解码失败时，跳过该子设备并继续处理其他子设备；输出时移除被跳过的 subDevice 占位 |
| `ekuiper-cluster/extensions/functions/acxPushDecode/acx_push_decode.go` | 更新 `schemasInfo` 参数约定，明确其来源是 device-level schema lookup，而不是 product-level schema lookup |
| `iot/rpc/things/internal/cache/schemaRepo.go`（已列于上） | — |
| `iot/rpc/things/internal/logic/productmanage/` | 新增模块 CRUD、品类模块绑定、产品草稿编辑、产品发布版本管理和默认版本切换逻辑 |
| `iot/rpc/things/internal/logic/schemamanage/` | 新增模块 schema CRUD 逻辑；`ModuleSchemaCreate/Update/TslImport` 仅允许标量类型全集，显式拒绝 `struct/array` |
| `iot/rpc/things/internal/logic/devicemsgmanage/` | 对外 API 形态不变，内部按模块绑定/版本做查询路由 |
| `iot/rpc/things/proto/` | 在现有 `schema.proto`、`product.proto`、`device.proto` 等协议文件中增加 ModuleInfo、CategoryModule、ProductModule 相关 message 和 RPC |


---

## 6. 规模预估
| 指标 | Legacy | 模块化 |
| --- | --- | --- |
| 每个系列的超级表数量 | N/A（按产品 x 属性） | 约 10~20 个逻辑模块 / 基本等同于物理 STABLE 数 |
| 30 个产品、每个 100 个属性的超级表数量 | 3,000 | 约 10~20 |
| 30 个产品 x 100 台设备的子表数量 | 300,000 | 9,000（平均每产品 3 个模块） |
| 单设备一次 100 点上报的 INSERT 数 | 100+ | 1~3（按模块） |
| 新产品创建成本 | 需要创建 100+ 张超级表 | 不需要创建产品级超级表，按需创建子表 |


---

## 7. 初稿：重构实施计划（终稿：2026-04-23-module-level-supertable-multi-agent-plan.md）
### 7.1 执行原则
+ 严格按 `RED -> GREEN -> REFACTOR` 执行；先写失败测试，再写最小实现
+ 优先改 contract、模型和校验边界，再改写路径、读路径和外围系统兼容
+ 每一批都要求“代码可编译 + 本批相关测试通过”，不接受跨批次的大爆炸式重构
+ 第一批不改 `thing.go` 主上报链路、不改 TDengine 模块写入实现、不改 `ekuiper-cluster`；先把 release snapshot 和 `SchemaBundle` 契约立住

### 7.2 分批范围
**第一批：contract + release snapshot model + 类型边界**

+ 引入 `SchemaBundle` / `SchemaRouteEntry`
+ 落 `DmModuleInfo` / `DmProductModule` / `DmProductRelease` / `DmProductReleaseSchema`
+ 实现 `DmProductReleaseSchema -> SchemaBundle` 组装
+ 模块 schema CRUD / 导入显式拒绝 `struct` / `array`
+ 发布逻辑先完成 release snapshot 复制与校验，不接入 TDengine 建表

**第二批：cache + 发布链路**

+ 拆 `DeviceRuntimeMetaCache` / `DeviceDetailCache` / `DeviceSecretCache`
+ `ProductSchemaRepo.GetCurrent/GetByRelease`、`DeviceSchemaRepo.GetData` release-aware 化
+ `ProductSchemaTslRead` / `DeviceSchemaTslRead` 切换到新 contract

**第三批：TDengine + 写路径**

+ 模块 STABLE / 子表 / `InsertModuleData`
+ `AsyncInsert` 按 `SQLBytes + Args` 公开双阈值拆批改造（内部保留固定 count cap）
+ `thing.go` 删除 auto-create，改为 `SchemaBundle.Route` 分桶写模块表

**第四批：读路径 + ekuiper + 清理**

+ 历史查询、latest 查询改为按 `ModuleCode` 分桶
+ `ps_lookup` / `acxPushDecode` 切到设备级 schema
+ 清理冲突的 legacy runtime 分支，保留必要兼容层

### 7.3 第一批详细任务
| 任务 | 文件 | 预期代码 | 验证 |
| --- | --- | --- | --- |
| 1. 建立测试基线 | 仓库根目录 | 不改代码；记录 `iot` 相关包当前测试状态 | `git worktree list`；`cd /home/wangjunbo/workspace-aux/iot && go test ./pkg/domain/schema ./rpc/things/internal/cache ./rpc/things/internal/relationdb/... ./rpc/things/internal/logic/schemamanage ./rpc/things/internal/logic/productmanage` |
| 2. 先写 `SchemaBundle` 失败测试 | `iot/pkg/domain/schema/schema_test.go`、新增 `iot/pkg/domain/schema/runtime_bundle_test.go` | 新增 `SchemaBundle` / `SchemaRouteEntry` 的最小行为测试：`Model/Route/Meta`、`identifier -> moduleCode`、`bundle.Model.String()` 不受 `Route` 影响 | `cd /home/wangjunbo/workspace-aux/iot && go test ./pkg/domain/schema` |
| 3. 实现 `SchemaBundle` 契约 | 新增 `iot/pkg/domain/schema/runtime_bundle.go` | 增加 `SchemaBundle`、`SchemaBundleMeta`、`SchemaRouteEntry`；只放公共 contract，不放 repo 逻辑 | `cd /home/wangjunbo/workspace-aux/iot && go test ./pkg/domain/schema` |
| 4. 先写 release snapshot 组装测试 | 新增 `iot/rpc/things/internal/relationdb/assemble_release_schema_test.go` | mock `DmProductReleaseSchema` 数据；断言能组装扁平 `schema.Model` 和 `Route`；重复 `identifier` 时返回错误 | `cd /home/wangjunbo/workspace-aux/iot && go test ./rpc/things/internal/relationdb` |
| 5. 扩 relationdb model | `iot/rpc/things/internal/relationdb/model/model.go` | 增加 `DmModuleInfo`、`DmProductModule`、`DmProductRelease`、`DmProductReleaseSchema`；给 `DmProductInfo` 增加 `CurrentReleaseID` | `cd /home/wangjunbo/workspace-aux/iot && go test ./rpc/things/internal/relationdb/...` |
| 6. 实现 release snapshot 组装 | `iot/rpc/things/internal/relationdb/assemble.go` | 增加 `[]DmProductReleaseSchema -> SchemaBundle` 组装函数；原子构造 `Model` 和 `Route`；校验非法类型和重复 `identifier` | `cd /home/wangjunbo/workspace-aux/iot && go test ./rpc/things/internal/relationdb` |
| 7. 先写 cache/repo contract 测试 | 新增 `iot/rpc/things/internal/cache/schemaRepo_test.go` | 用 fake loader 测 `ProductSchemaRepo.GetCurrent/GetByRelease` 和 `DeviceSchemaRepo.GetData` 的语义；不依赖真 Redis/DB | `cd /home/wangjunbo/workspace-aux/iot && go test ./rpc/things/internal/cache` |
| 8. 改 cache contract，但先保守兼容 | `iot/rpc/things/internal/cache/schemaRepo.go` | 增加 `GetCurrent` / `GetByRelease` / `GetData` 的新入口；内部以 `SchemaBundle` 为主；旧的 `*schema.Model` 入口先保留 adapter，避免第一批炸全链路 | `cd /home/wangjunbo/workspace-aux/iot && go test ./rpc/things/internal/cache` |
| 9. 先写“拒绝 struct/array”测试 | 新增 `iot/rpc/things/internal/logic/schemamanage/module_schema_create_logic_test.go`、`module_schema_update_logic_test.go`、`module_schema_tsl_import_logic_test.go` | `struct` / `array` 输入失败；`smallInt/tinyInt/int/bigInt/float/double/...` 全部允许 | `cd /home/wangjunbo/workspace-aux/iot && go test ./rpc/things/internal/logic/schemamanage` |
| 10. 实现模块 schema 标量校验 | 新增 `iot/rpc/things/internal/logic/schemamanage/module_schema_create_logic.go`、`module_schema_update_logic.go`、`module_schema_tsl_import_logic.go` | 抽共享校验函数；模块 schema 仅允许标量类型全集，显式拒绝 `struct/array` | `cd /home/wangjunbo/workspace-aux/iot && go test ./rpc/things/internal/logic/schemamanage` |
| 11. 先写发布逻辑失败测试 | 新增 `iot/rpc/things/internal/logic/productmanage/product_release_publish_logic_test.go` | 重复 `identifier` 失败、`struct/array` 失败、标量类型全集通过、默认 release 更新行为正确 | `cd /home/wangjunbo/workspace-aux/iot && go test ./rpc/things/internal/logic/productmanage` |
| 12. 实现发布逻辑最小骨架 | 新增 `iot/rpc/things/internal/logic/productmanage/product_release_publish_logic.go` | 从 `DmProductModule` 读取草稿，复制成 `DmProductRelease` + `DmProductReleaseSchema`；调用第 6 步的 assemble 校验；暂不接入 TDengine 建表 | `cd /home/wangjunbo/workspace-aux/iot && go test ./rpc/things/internal/logic/productmanage` |


### 7.4 第一批完成标准
+ `SchemaBundle` contract 已落地，并有单元测试覆盖
+ `DmModuleInfo` / `DmProductModule` / `DmProductRelease` / `DmProductReleaseSchema` 已进入 relationdb model
+ 可以从 `DmProductReleaseSchema` 组装出 `SchemaBundle`
+ 模块 schema CRUD / 导入已显式拒绝 `struct` / `array`
+ 发布逻辑已能生成 release snapshot，并完成类型、identifier 唯一性校验
+ 以下测试集全部通过：
    - `go test ./pkg/domain/schema`
    - `go test ./rpc/things/internal/relationdb/...`
    - `go test ./rpc/things/internal/cache`
    - `go test ./rpc/things/internal/logic/schemamanage`
    - `go test ./rpc/things/internal/logic/productmanage`

