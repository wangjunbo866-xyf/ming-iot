# 产品分类复用与模块快照发布优化设计
## 目标
本文是以下两篇文档的补充优化：

+ [design-module-level-supertable-optimization-2026-04-23.md](/home/wangjunbo/workspace-aux/doc/module-level-supertable/design-module-level-supertable-optimization-2026-04-23.md)
+ [design-module-kind-release-schema-2026-04-26.md](/home/wangjunbo/workspace-aux/doc/module-level-supertable/design-module-kind-release-schema-2026-04-26.md)

对应执行计划：

+ [plan-category-module-snapshot-release-2026-04-28.md](/home/wangjunbo/workspace-aux/doc/module-level-supertable/plan-category-module-snapshot-release-2026-04-28.md)

本次优化针对公司的真实业务模型：

+ 公司有几十种产品，后续还会继续增加
+ 物模型大体只有 ASN / ASG 两类
+ ASN 下几十种子产品大部分共用同一套点位模型，ASG 同理
+ 点位来自公司已有总点表或业务资料， IoT 平台不需要再维护一套运行时 `dm_point_catalog` （总点表）
+ 不希望每个产品每次发布都把相同的 `dm_module_schema_info` 逐点复制一遍

最终结论：

+ 不新增维护 `dm_point_catalog`
+ 使用 `dm_product_category` 表达 ASN / ASG 这类 schema family
+ 使用 `dm_category_module` 表达分类默认模块集合
+ 发布快照不再逐行复制 `dm_module_schema_info` 到 `dm_product_release_schema`
+ 改为“模块级 JSON 快照 + 产品 release 引用模块快照”
+ 当前仍处于开发阶段，旧表 `dm_product_release_schema` 没有值得保留的历史数据，V1 直接硬删除，不再作为可选物化索引保留

---

## 1. 为什么不需要 dm_point_catalog
`dm_point_catalog` 的价值是做全公司点位主数据。

但当前业务已经有“总点表”来源，IoT 平台真正需要的是：

+ 模块里有哪些点位
+ 点位如何解码
+ 点位是否进入 TDengine
+ 点位属于 property / event / action 哪类能力
+ 某个产品发布版本最终使用哪些模块快照

这些能力已经由以下表承担：

+ `dm_module_info`
+ `dm_module_schema_info`
+ `dm_category_module`
+ `dm_product_module`
+ `dm_product_release`
+ 新增的 `dm_module_schema_snapshot`
+ 新增的 `dm_product_release_module`

因此 V1 不再引入 `dm_point_catalog`。

如果后续仍需要“公司总点表”能力，应作为外部主数据或导入工具存在，不作为设备解码、发布快照、TDengine 写入的运行时依赖。

---

## 2. 为什么不需要 dm_schema_profile
`dm_schema_profile` 的本质是“可复用物模型族”。

当前系统已有 `dm_product_category`，并且产品已有 `category_id`：

```go
type DmProductInfo struct {
    ProductID  string
    CategoryID int64
    // ...
}
```

因此可以直接用产品分类表达物模型族：

| 业务概念 | 建议落表 |
| --- | --- |
| ASN 物模型族 | `dm_product_category` 中的 ASN 分类节点（id=1） |
| ASG 物模型族 | `dm_product_category` 中的 ASG 分类节点（id=3） |
| ASN 通用模块集合 | `dm_category_module(category_id=1)` |
| ASG 通用模块集合 | `dm_category_module(category_id=3)` |
| 某个产品独有模块 | `dm_product_module(product_id=xxx)` |
| 某个产品默认运行版本 | `dm_product_info.current_release_id`，表示未设置 override 设备的 fleet default，不等同于最新 release |


这样不需要再新增一张 `dm_schema_profile`。

分类层级建议：

```latex
产品分类
├── ASN
│   ├── ASN-子产品分类A
│   └── ASN-子产品分类B
└── ASG
    ├── ASG-子产品分类A
    └── ASG-子产品分类B
```

模块继承规则：

+ 产品发布时，从产品 `category_id` 沿 `id_path` 读取分类链路上的 `dm_category_module`
+ 再叠加该产品自己的 `dm_product_module`
+ V1 不做同名模块覆盖，最终模块集合按 `module_id` 去重
+ 发布时把最终 `module_id` 集合冻结到 `dm_product_release_module`
+ 发布完成后，运行时和已发布 release 详情不再动态查 `dm_product_category`、不再沿 `id_path` 上溯、也不再读取 `dm_category_module`
+ 如果多个模块内出现相同 `identifier`，发布失败并返回明确的 identifier 冲突错误

---

## 3. 核心取舍：按模块快照，不按点位逐行快照
旧设计：

```latex
dm_product_release
  └── dm_product_release_schema
      ├── identifier_1
      ├── identifier_2
      ├── ...
      └── identifier_N
```

问题：

+ ASN 下几十个产品共用同一套模块时，每个产品 release 都会复制同一批点位行
+ 一个模块上千点位时，发布行数按“产品数 × release 数 × 点位数”放大
+ `dm_product_release_schema` 既是字段明细，又承担模块归属，后续再引入模块级缓存时会重复表达

优化后：

```latex
dm_product_release
  └── dm_product_release_module
      ├── module_id=1 -> module_schema_snapshot_id=101
      ├── module_id=2 -> module_schema_snapshot_id=102
      └── module_id=3 -> module_schema_snapshot_id=103

dm_module_schema_snapshot
  ├── id=101, module_id=1, schema_json=[...]
  ├── id=102, module_id=2, schema_json=[...]
  └── id=103, module_id=3, schema_json=[...]
```

优点：

+ 相同模块内容只存一份快照
+ 多个产品 release 只引用同一批模块快照
+ 某个模块修改后，只生成该模块的新快照，不影响其他未变模块
+ 运行时仍能按 release 精确复现历史 schema
+ `ModuleKind` 仍只从不可变的 `dm_module_info.module_kind` 读取，不冗余到快照表

### 3.1 模块快照存储形态：大 JSON 作为权威
模块级快照有两种落库形态：

| 方案 | 优点 | 缺点 | V1 结论 |
| --- | --- | --- | --- |
| 模块内逐点行快照 | SQL 查询单个 `identifier` 方便；可用唯一索引约束 `snapshot_id + identifier`；管理端分页、检索、diff 直接 | 每次模块发布仍要插入几百到上千行；事务和索引写入成本高；运行时需要读多行再组装；减少快照数量的收益不彻底 | 不作为权威 |
| 模块大 JSON 快照 | 一个模块版本只插入一行；天然适合 hash 去重；发布原子性好；运行时按 release 只读少量模块快照；适合构建 Redis `SchemaBundle` | SQL 级按点查询不方便；JSON 内部唯一性、类型合法性需要发布前校验；管理端 diff/search 需要应用层展开或额外索引 | 作为 V1 权威 |


V1 明确采用：

```latex
dm_module_schema_snapshot.schema_json = 模块快照权威
```

不采用：

```latex
dm_module_schema_snapshot_item = 模块快照权威
```

原因：

+ 快照发布目标是减少“产品数 × release 数 × 点位数”的行级放大
+ 运行时按 release 获取 schema 时，天然是按模块读取，而不是按单点随机读取
+ Redis `SchemaBundle` 构建需要完整模块字段集合，逐点行表反而要再聚合
+ 一个产品 release 的全部 schema 放成一个大 JSON 也不推荐，因为 ASN / ASG 多产品复用时仍会在每个产品 release 上重复存储；模块级 JSON 才能被多个产品 release 共享引用

如果后续管理端存在高频 SQL 检索、分页或 diff 需求，可以增加可重建的物化索引表，但该表不能成为 release 语义真相。

---

## 4. 表设计
### 4.1 dm_product_category
继续使用现有表，不新增 `dm_schema_profile`。

用途：

+ 表达 ASN / ASG 这类物模型族
+ 表达产品分类树
+ 作为分类默认模块集合的归属方

约束：

+ 管理端需要明确哪些分类节点可作为“物模型族”
+ 产品发布时只读取产品所属分类链路上的模块，不读取其他分类
+ 分类删除或移动前，需要校验是否存在已发布产品引用

当前已新增了三个产品分类

1	逆变器ASN

2	采集器

3	逆变器ASG



### 4.2 dm_category_module
继续使用现有表。

用途：

+ 维护某个分类默认包含哪些模块
+ ASN / ASG 共用模块优先放在分类层，不要复制到每个产品

约束：

+ `category_id + module_id + deleted_time` 唯一
+ 分类模块只是草稿/配置，不是运行时真相
+ 已发布 release 不回查 `dm_category_module`
+ 分类模块绑定变化只影响后续新发布的 release，不影响已经发布的 `dm_product_release_module`

### 4.3 dm_product_module
继续使用现有表。

用途：

+ 维护产品独有模块
+ 维护少数产品相对分类默认模型的扩展模块

约束：

+ `product_id + module_id + deleted_time` 唯一
+ 产品模块只是草稿/配置，不是运行时真相
+ V1 不支持“覆盖分类模块”，只支持叠加
+ 如需替换分类模块，应调整分类模块绑定或创建新的产品分类

### 4.4 dm_module_schema_snapshot
新增模块 schema 快照表。

```go
type DmModuleSchemaSnapshot struct {
    ID         int64  `gorm:"column:id;type:bigint;primary_key;AUTO_INCREMENT"`
    ModuleID   int64  `gorm:"column:module_id;uniqueIndex:idx_dm_module_schema_snapshot_hash;type:bigint;NOT NULL"`
    SchemaHash string `gorm:"column:schema_hash;uniqueIndex:idx_dm_module_schema_snapshot_hash;type:varchar(64);NOT NULL"`
    SchemaJSON string `gorm:"column:schema_json;type:json;NOT NULL"`
    ItemCount  int64  `gorm:"column:item_count;type:integer;default:0"`
    stores.NoDelTime
    DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:idx_dm_module_schema_snapshot_hash"`
}
```

字段说明：

+ `module_id`：所属模块
+ `schema_hash`：模块点位规范化 JSON 的 SHA-256
+ `schema_json`：该模块在某一时刻的完整点位快照
+ `item_count`：快照内点位数量，方便发布校验和观测

hash 复用与冲突处理：

+ 唯一索引为 `module_id + schema_hash + deleted_time`。
+ 发布时先生成稳定 `schema_json`，再计算 SHA-256 `schema_hash`。
+ 如果同一 `module_id + schema_hash` 没有现存快照，则插入新快照。
+ 如果已存在快照，必须把现存 `schema_json` 与本次生成的 `schema_json` 做字节级等值比较；完全一致才允许复用已有 `snapshot_id`。
+ 如果 `schema_hash` 相同但 `schema_json` 不一致，V1 直接返回 `ErrModuleSchemaHashConflict` 并阻断发布，不做字段级 diff、不尝试换 hash、不自动插入第二条冲突快照。
+ SHA-256 真碰撞概率极低；出现该错误更可能是稳定编码、hash 计算或数据污染问题，应 fail closed 并人工排查。

快照生命周期与 GC：

+ V1 不做 `dm_module_schema_snapshot` 垃圾回收。即使 `publish_failed` release 被软删、对应 `dm_product_release_module` 引用变为 0，也保留 orphan snapshot 行。
+ 不做 GC 的原因是 V1 数据量可控，例如 100 个模块 × 每模块 10 个版本约 1000 行，远小于逐点快照方案的行数放大。
+ 后续如果 snapshot 行数成为维护负担，可新增可选后台 GC：只允许清理没有任何未软删 `dm_product_release_module` 引用、并超过保留期的 orphan snapshot；被任何 release-module 引用的 snapshot 永远不能删除。

快照 JSON 建议格式：

```json
{
  "schemaVersion": 2,
  "moduleId": 1001,
  "itemCount": 1,
  "items": [
    {
      "identifier": "temp_out",
      "tag": 1,
      "type": 1,
      "extendConfig": {
        "base-addr": 40001,
        "vdata_type": "S16",
        "scaler": 1,
        "bit_position": 0,
        "byte_order": "ABCD",
        "word_order": "AB"
      },
      "required": 2,
      "name": "出水温度",
      "langKey": "module.aux_asn_rt.temp_out",
      "desc": "逆变器出水温度",
      "descLangKey": "module.aux_asn_rt.temp_out.desc",
      "isCanSceneLinkage": 1,
      "funcGroup": 1,
      "controlMode": 1,
      "userPerm": 1,
      "recordMode": 1,
      "isPassword": 2,
      "affordance": {
        "isUseShadow": false,
        "isNoRecord": false,
        "define": {
          "type": "double",
          "mapping": {},
          "min": "-40",
          "max": "125",
          "start": "0",
          "step": "0.1",
          "unit": "°C",
          "specs": [],
          "arrayInfo": null
        },
        "mode": "r"
      },
      "order": 100
    }
  ]
}
```

`items[]` 必须完整保留 `dm_module_schema_info.identifier + DmSchemaCore` 的运行时字段，不允许只截取解码字段或 TDengine 字段。

完整字段清单：

| JSON 字段 | 来源字段 | 说明 |
| --- | --- | --- |
| `identifier` | `DmModuleSchemaInfo.Identifier` | 点位标识符 |
| `tag` | `DmSchemaCore.Tag` | 物模型标签 |
| `type` | `DmSchemaCore.Type` | 1: property，2: event，3: action |
| `extendConfig` | `DmSchemaCore.ExtendConfig` | 协议、地址、倍率、告警码等扩展配置；SQL 中是 JSON 字符串，快照中必须保存为规范化 JSON object |
| `required` | `DmSchemaCore.Required` | 是否必须，保持 SQL 层取值 |
| `name` | `DmSchemaCore.Name` | 功能名称 |
| `langKey` | `DmSchemaCore.LangKey` | 名称多语言键 |
| `desc` | `DmSchemaCore.Desc` | 描述 |
| `descLangKey` | `DmSchemaCore.DescLangKey` | 描述多语言键；event 可用于 suggestion |
| `isCanSceneLinkage` | `DmSchemaCore.IsCanSceneLinkage` | 是否支持场景联动 |
| `funcGroup` | `DmSchemaCore.FuncGroup` | 功能分类 |
| `controlMode` | `DmSchemaCore.ControlMode` | 控制模式 |
| `userPerm` | `DmSchemaCore.UserPerm` | 用户权限 |
| `recordMode` | `DmSchemaCore.RecordMode` | 历史记录策略；V1 property_tsdb 固定按 `RecordModeAll` 校验 |
| `isPassword` | `DmSchemaCore.IsPassword` | 是否密码类型 |
| `affordance` | `DmSchemaCore.Affordance` | 类型专属定义；SQL 中是 JSON 字符串，快照中必须保存为规范化 JSON object |
| `order` | `DmSchemaCore.Order` | 管理端排序 |


`affordance` 的完整结构按 `type` 区分：

```json
{
  "property": {
    "isUseShadow": false,
    "isNoRecord": false,
    "define": {
      "type": "double",
      "mapping": {},
      "min": "-40",
      "max": "125",
      "start": "0",
      "step": "0.1",
      "unit": "°C",
      "specs": [],
      "arrayInfo": null
    },
    "mode": "r"
  },
  "event": {
    "type": "alert",
    "params": [
      {
        "identifier": "alarm_code",
        "name": "告警码",
        "define": {
          "type": "int",
          "mapping": {},
          "min": "0",
          "max": "999999",
          "start": "0",
          "step": "1",
          "unit": "",
          "specs": [],
          "arrayInfo": null
        },
        "langKey": "module.aux_inverter_event.alarm_code"
      }
    ]
  },
  "action": {
    "dir": "down",
    "input": [
      {
        "identifier": "enable",
        "name": "使能",
        "define": {
          "type": "bool",
          "mapping": {
            "0": "关闭",
            "1": "开启"
          },
          "min": "",
          "max": "",
          "start": "",
          "step": "",
          "unit": "",
          "specs": [],
          "arrayInfo": null
        },
        "langKey": "module.aux_action.enable"
      }
    ],
    "output": []
  }
}
```

说明：

+ 上面的 `property/event/action` 只是展示三种 `affordance` 结构；实际单个模块由 `ModuleKind` 限定，不允许混放 property、event、action。
+ V1 不支持 `struct/array` 创建，但 `define` 仍完整列出 `specs/arrayInfo` 字段，保持快照结构稳定。
+ `extendConfig` 和 `affordance` 在 SQL 草稿层可以继续是字符串；生成快照时必须解析为 object 后再写入 `schema_json`，避免 JSON-in-JSON。

#### schemaVersion=2 JSON 规范
`schemaVersion=2` 的强制规则：

+ `extendConfig` 必须是 JSON object；空字符串、空白字符串、`null`、数组、数字、字符串和非法 JSON 一律发布失败，不自动写 `{}`。
+ `affordance` 必须是 JSON object，并且按 `type` 解析成 `PropertyAffordance` / `EventAffordance` / `ActionAffordance` 对应结构。
+ 不允许把 `extendConfig` 或 `affordance` 以 escaped JSON string 写入 `schema_json`；例如 TEXT 中存 `"{\"base-addr\":1028}"` 属于 JSON string，不是 JSON object，发布失败。
+ 发布时如果 `extend_config` / `affordance` 为空、`null`、非法 JSON、非 JSON object、字段结构与 `type` 不匹配，发布失败。
+ `schema_hash` 基于解析后的 object 做稳定 JSON 编码后计算，不受 SQL 原始字符串空格、字段顺序和转义差异影响。
+ 新项目 V1 只写 `schemaVersion=2`；如果导入工具发现旧格式 escaped string，应在导入阶段转换为 object，不能把旧格式继续写入新快照。

发布侧处理规则：

+ `dm_module_schema_info.extend_config` 当前是 `TEXT`，但发布时必须按 JSON object 解析；解析失败或解析结果不是 object，直接阻断发布。
+ `dm_module_schema_info.affordance` 虽然在 PostgreSQL 中是 `JSON`，发布时仍必须校验其顶层类型是 object，且内容能反序列化为对应 affordance 结构。
+ 如果某个点位确实没有扩展配置，草稿层也必须显式保存 `{}`，不能保存空字符串或 `null`。

CPU 成本边界：

+ SQL 草稿层的 `extend_config` / `affordance` 双字段解析只允许发生在发布、导入、cache miss、bundle 预热这类低频路径。
+ `ProductSchemaRepo.GetRelease` 构建 `SchemaBundle` 时，可以解析 `schema_json` 并编译出 `Model + Route + DecodePlan`。
+ Redis 中可保存 `SchemaBundle` JSON，但进程内 L1 必须保存已解析的 Go struct。
+ `ekuiper-cluster` 的 `ps_lookup` 应按 `(productID, releaseID, scope)` 缓存已解析 schema，不能在每包 MQTT 数据解码时重新解析 `extendConfig/affordance`。
+ `acxPushDecode` 热路径只能使用已解析的 decode plan；不得对每个点位做二次 `json.Unmarshal`。

规范化规则：

+ 按 `identifier` 升序排序
+ 不写入 `id / created_at / updated_at / deleted_time`
+ 完整保留 `identifier + DmSchemaCore` 的运行时字段，不按当前消费路径裁剪
+ JSON 使用稳定编码后计算 `schema_hash`
+ 发布前必须在应用层完成 `identifier` 唯一性、类型合法性、TDengine 列名合法性和模块 kind 匹配校验；不能依赖 JSON 内部的数据库约束兜底

### 4.5 dm_product_release_module
新增产品 release 到模块快照的引用表。

```go
type DmProductReleaseModule struct {
    ID                     int64  `gorm:"column:id;type:bigint;primary_key;AUTO_INCREMENT"`
    ReleaseID              int64  `gorm:"column:release_id;uniqueIndex:idx_dm_product_release_module;type:bigint;NOT NULL"`
    ProductID              string `gorm:"column:product_id;index:idx_dm_product_release_module_product;type:varchar(100);NOT NULL"`
    ModuleID               int64  `gorm:"column:module_id;uniqueIndex:idx_dm_product_release_module;type:bigint;NOT NULL"`
    ModuleCode             string `gorm:"column:module_code;type:varchar(100);NOT NULL"`
    ModuleSchemaSnapshotID int64  `gorm:"column:module_schema_snapshot_id;type:bigint;NOT NULL"`
    SourceCategoryID       int64  `gorm:"column:source_category_id;type:bigint;default:0"`
    SourceProductID        string `gorm:"column:source_product_id;type:varchar(100);default:''"`
    stores.NoDelTime
    DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:idx_dm_product_release_module"`
}
```

字段说明：

+ `release_id + module_id + deleted_time` 唯一
+ `module_code` 是发布时从 `dm_module_info.module_code` 冻结过来的冗余字段，主要用于 Route 组装、导出和排查
+ `module_schema_snapshot_id` 指向该 release 使用的模块字段快照
+ `product_id` 是该 release-module 归属的产品 ID，必须等于 `dm_product_release.product_id`；它用于查询、缓存 key、导出和权限边界，不表示模块来源
+ `source_category_id` 仅用于审计，表示发布时该模块最终按哪个分类绑定进入 release；运行时不能据此回查分类链路
+ `source_product_id` 仅用于审计，表示发布时该模块最终按哪个产品独有绑定进入 release

来源填充规则：

+ 只来自分类链路：`source_category_id` 填最直接的分类 ID，`source_product_id` 为空字符串。这里的“最直接”指分类链路上离产品所属分类最近、且绑定了该 `module_id` 的分类节点。
+ 只来自产品绑定：`source_product_id` 填当前 `product_id`，`source_category_id` 填 0。
+ 分类链路和产品绑定同时包含同一个 `module_id`：仍只写一行 `dm_product_release_module`，V1 按“产品显式绑定优先”记录来源，即 `source_product_id = product_id`，`source_category_id = 0`。
+ 分类链路上多个分类节点都绑定同一个 `module_id`：仍只写一行，`source_category_id` 记录离产品所属分类最近的分类节点。
+ 上述“优先”只用于审计来源归因，不是 schema 内容覆盖规则；同一 `module_id` 的 schema 仍只有一份 `dm_module_schema_info` 当前模板。

`product_id` 与 `source_product_id` 的关系：

+ `product_id` 必填，表示 release 归属；分类来源和产品来源都会填。
+ `source_product_id` 可空，只在模块来自 `dm_product_module` 时填。
+ 在 V1 中，`source_product_id` 如果非空，通常等于 `product_id`。这不是运行时重复字段，而是为了区分“该模块来自产品显式绑定”还是“该模块来自分类默认绑定”的审计字段。

存储边界：

+ 存 `module_code`：用于减少 Route 组装对 `dm_module_info` 的依赖，并方便排查；它是发布时冻结的只读冗余字段。
+ 不存 `module_kind`：scope 过滤、TDengine 建表和写入仍统一从不可变的 `dm_module_info.module_kind` 获取。
+ `module_code` 不允许在 release 内修改；如果发现它和 `dm_module_info.module_code` 不一致，应记录数据异常，但已发布 release 的 Route 可继续使用 `dm_product_release_module.module_code`。
+ `module_code` 不是模块类型判断依据，不能替代 `ModuleKind`。

### 4.6 dm_product_release_schema
在本优化方案中，`dm_product_release_schema` 直接删除。

原因：

+ 当前系统仍处于开发阶段，旧表没有需要迁移或审计的历史数据。
+ 继续保留旧表会形成“双真相”误导，实施者可能继续从旧表回填 `SchemaBundle`。
+ V1 权威只保留 `dm_product_release + dm_product_release_module + dm_module_schema_snapshot`。

约束：

+ migration 直接执行 `DROP TABLE IF EXISTS dm_product_release_schema;`。
+ 代码删除 `DmProductReleaseSchema` model、`ProductReleaseSchemaRepo`、旧表 AutoMigrate 注册和 cache L3 旧表回填逻辑。
+ 发布只写 `dm_module_schema_snapshot + dm_product_release_module`。
+ 运行时 bundle 只从 `dm_product_release_module + dm_module_schema_snapshot + dm_module_info` 组装。
+ 如果未来确实需要按 release + identifier 做 SQL 检索，新增 `dm_module_schema_snapshot_item` 作为可重建索引，不复用旧表名。

### 4.7 dm_module_schema_snapshot_item（可选物化索引）
V1 不要求创建该表。只有当管理端确实需要高频按点位 SQL 检索、分页、筛选或 diff 时，才考虑增加。

```go
type DmModuleSchemaSnapshotItem struct {
    ID         int64  `gorm:"column:id;type:bigint;primary_key;AUTO_INCREMENT"`
    SnapshotID int64  `gorm:"column:snapshot_id;uniqueIndex:idx_dm_module_schema_snapshot_item_identifier;type:bigint;NOT NULL"`
    ModuleID   int64  `gorm:"column:module_id;index:idx_dm_module_schema_snapshot_item_module;type:bigint;NOT NULL"`
    Identifier string `gorm:"column:identifier;uniqueIndex:idx_dm_module_schema_snapshot_item_identifier;type:varchar(100);NOT NULL"`
    DataType   string `gorm:"column:data_type;type:varchar(32);NOT NULL"`
    Type       int64  `gorm:"column:type;type:smallint;default:0"`
    stores.NoDelTime
    DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:idx_dm_module_schema_snapshot_item_identifier"`
}
```

约束：

+ `dm_module_schema_snapshot.schema_json` 仍是权威
+ `dm_module_schema_snapshot_item` 只能由 `schema_json` 展开生成
+ 运行时 bundle 构建、设备解码、TDengine 写入和 release 解析不得依赖该表
+ 索引表缺失、落后或损坏时，应允许从 `schema_json` 全量重建
+ 不允许管理端直接编辑索引表；编辑只能发生在 `dm_module_schema_info` 草稿层

---

## 5. 发布流程
### 5.1 输入
产品发布时读取：

+ `dm_product_info.category_id`
+ 分类链路上的 `dm_category_module`
+ 产品自己的 `dm_product_module`
+ 每个模块的 `dm_module_info`
+ 每个模块当前的 `dm_module_schema_info`

### 5.2 组装模块集合
```latex
product.category_id
  -> category.id_path
  -> dm_category_module(module_id...)
  -> union dm_product_module(module_id...)
  -> distinct module_id
  -> freeze into dm_product_release_module
```

关键约束：

+ 分类链路只在发布时解析一次。
+ `product.category_id` 必须大于 0，且对应 `dm_product_category` 必须存在、未删除；否则发布失败，不退化为只读 `dm_product_module`。
+ `category.id_path` 上的每个分类节点都必须存在、未删除；如果链路中任一节点软删或缺失，发布失败，不跳过。
+ 这些分类错误属于发布前置条件错误，推荐在创建 `dm_product_release(publishing)` 前完成校验；如果实现上已经创建了 `publishing` release，则必须按三阶段发布规则把 release 标记为 `publish_failed`，不能产出 `published` release。
+ 发布成功后，`dm_product_release_module` 是该 release 的模块集合真相。
+ 运行时 `ProductSchemaRepo.GetRelease` 只读取 `dm_product_release_module + dm_module_schema_snapshot + dm_module_info`。
+ 运行时禁止重新读取 `dm_product_category.id_path` 或 `dm_category_module` 来推导模块集合。
+ 分类模块后续新增、删除、调整顺序，只影响下一次发布；旧 release 的模块集合不变。
+ 分类链路和产品草稿如果绑定了同一个 `module_id`，只按 `module_id` 去重即可；schema 内容不区分“分类优先”或“产品优先”，因为同一 `module_id` 指向同一份 `dm_module_schema_info` 当前模板。
+ V1 不支持用 `dm_product_module` 覆盖分类中的同一个 `module_id`；如需产品独有差异，应创建新的模块或新的产品分类。

分类链路边界条件：

+ `product.category_id = 0`：发布失败，返回明确的分类未绑定错误，不允许退化为只发布 `dm_product_module`。
+ `product.category_id` 指向的分类不存在或已软删：发布失败，返回明确的分类不存在/已删除错误。
+ `category.id_path` 中任一节点不存在或已软删：发布失败，返回明确的分类链路损坏错误；不能跳过该节点继续发布。
+ `dm_category_module` 中绑定的 `module_id` 不存在、已软删、`ModuleKind` 非法，或没有任何有效 `dm_module_schema_info`：发布失败。
+ 分类链路与 `dm_product_module` 重复绑定同一个 `module_id`：按 `module_id` 去重，只生成一条 release-module 记录；来源审计按产品显式绑定优先，填 `source_product_id = product_id`、`source_category_id = 0`。

校验：

+ 产品必须绑定有效分类
+ 分类链路必须完整且未软删
+ 模块必须存在且未删除
+ `ModuleKind` 必须合法
+ 同一 release 内 `identifier` 全局唯一
+ 同一模块内 `identifier` 唯一
+ `property_tsdb` 模块必须通过 TDengine 列名、类型、列数、SQL 长度预校验
+ `property_no_store/event/action` 不触发 TDengine 建表

### 5.3 生成模块快照
对每个模块：

+ 读取该模块全部 `dm_module_schema_info`
+ 规范化为稳定 JSON
+ 计算 `schema_hash`
+ 按 `module_id + schema_hash` 查找 `dm_module_schema_snapshot`
+ 不存在则插入新快照
+ 存在则比较 `schema_json` 字节内容，完全一致才复用旧快照
+ 如果 `schema_hash` 一致但 `schema_json` 不一致，返回 `ErrModuleSchemaHashConflict`，该 release 不能进入 `published`

V1 不做字段级 diff。冲突错误只需要返回 `module_id/schema_hash/existing_snapshot_id` 和当前 release 信息，方便人工排查稳定编码或数据污染问题。

### 5.4 生成产品 release
发布使用三阶段状态机，不做跨 PostgreSQL 与 TDengine 的分布式事务。

状态定义：

+ `publishing`：SQL 快照已创建，但 TDengine 物理结构和 bundle 校验尚未全部完成，不可运行时引用
+ `published`：发布成功，可被 `GetRelease`、设备 override、`ProductReleaseSetDefault` 引用
+ `publish_failed`：发布失败，禁止运行时引用，保留失败现场用于排查或重试

第一阶段，在 SQL 事务内：

+ 插入 `dm_product_release`，状态为 `publishing`
+ 插入或复用 `dm_module_schema_snapshot`
+ 插入多行 `dm_product_release_module`
+ 不更新 `dm_product_info.current_release_id`

第二阶段，事务外执行物理资源准备：

+ 为 `property_tsdb` 模块创建或扩展 TDengine 超级表
+ 构建并校验 `release + scope` schema bundle

第三阶段成功时，在 SQL 事务内完成生效：

+ 将 `dm_product_release.status` 更新为 `published`
+ 写入 `published_at`
+ 不更新 `dm_product_info.current_release_id`
+ 预热 `release + scope` schema cache

这里的“生效”只表示该 release 从 `publishing` 进入 `published`，成为可被显式引用的不可变快照；它不表示产品默认运行 schema 已切换。

发布成功后的缓存规则：

+ 只预热 `things:schema:product:release:{productID}:{releaseID}:{scope}`，让管理端预览、设备 override、OTA 成功后的单设备切换可以直接命中新 release。
+ 不淘汰 `things:schema:product:current:{productID}:{scope}`，因为 `dm_product_info.current_release_id` 没有变化，未 override 设备仍应继续使用旧 current release。
+ 只有显式执行 `ProductReleaseSetDefault(productID, releaseID)` 并成功更新 `dm_product_info.current_release_id` 后，才淘汰 current cache，让后续 current 读取加载新的默认 release。
+ `ProductReleasePublish` 中禁止调用 set default 逻辑，也禁止通过“清 current cache”暗示新 release 已默认生效。

第三阶段失败时，在 SQL 事务内记录失败：

+ 将 `dm_product_release.status` 更新为 `publish_failed`
+ 写入 `failed_at`
+ 写入 `publish_error`
+ 保留第一阶段已经写入的 `dm_product_release_module`
+ 保留已经插入或复用的 `dm_module_schema_snapshot`
+ 不更新 `dm_product_info.current_release_id`
+ 不写入或预热 release/current cache

失败后不软删 `dm_product_release_module` 的原因：

+ 便于排查当时发布候选版本到底包含哪些模块和模块快照
+ 便于对同一个 failed release 做精确 retry
+ 避免 TDengine 外部副作用无法回滚时，SQL 侧又丢失证据

重要约束：

+ `ProductReleasePublish` 只表示“生成一个已发布且可被引用的 schema 快照”
+ `ProductReleasePublish` 不等于“全产品设备立刻切到新 schema”
+ `product.current_release_id` 由独立的 `ProductReleaseSetDefault` 操作修改
+ 新 release 发布后，默认不会影响任何未设置 override 的线上设备
+ `ProductSchemaRepo.GetRelease`、`ProductReleaseSetDefault`、`DeviceSchemaReleaseOverrideUpdate` 必须拒绝 `publishing` 和 `publish_failed` release
+ `published` 是运行时唯一可见状态

失败策略：

+ `dm_product_release.release_no` 冲突返回明确的 `ErrReleaseVersionConflict`
+ 模块快照插入 hash 冲突时只做 `schema_json` 字节级等值复查；内容一致则复用，不一致返回 `ErrModuleSchemaHashConflict`
+ TDengine 建表、扩列或 bundle 构建失败时，转为 `publish_failed`，不更新 `current_release_id`
+ 已插入但未生效的 release 不得被设备运行时解析

重试策略：

+ `publish_failed` 可以提供显式 `ProductReleaseRetry(releaseID)` 管理动作
+ retry 必须复用该 release 已冻结的 `dm_product_release_module + dm_module_schema_snapshot`
+ retry 不允许重新读取当前 `dm_category_module` 或 `dm_module_schema_info`，否则会改变失败 release 的语义
+ retry 成功后状态从 `publish_failed -> published`
+ 如果需要使用最新草稿内容，应创建新的 `release_no`，而不是 retry 旧失败 release

后台清理策略：

+ `publishing` 超过发布超时时间仍未进入终态，后台任务应标记为 `publish_failed`，`publish_error` 写入 `publishing timeout`
+ `publish_failed` 默认保留一段时间用于审计和排查，例如 30~90 天
+ 清理 `publish_failed` 时，可软删 `dm_product_release` 和对应 `dm_product_release_module`
+ V1 不做 snapshot GC；即使软删 release-module 后 `dm_module_schema_snapshot` 引用数变为 0，也保留 orphan snapshot。后续如需 GC，再按 §4.4 的可选策略单独实现
+ `published` release 不允许自动清理；必须先确认没有 product current、device override、历史查询或审计依赖

### 5.5 Release 生效与 OTA 解耦
`dm_product_info.current_release_id` 的语义必须收窄为“产品默认运行 release / fleet default”，不能理解为“最新发布 release”。

原因：

+ 新增点位通常需要设备固件升级后才会上报或支持主动读取
+ 如果发布新 schema 后立即切 `current_release_id`，未升级固件的设备会使用新 schema 解码旧数据块，可能出现字段缺失、偏移错误或无效读写
+ `DmDeviceInfo.Version` 只表示固件版本，不参与运行时 schema 路由；运行时不维护“固件版本 -> schema release”的大而全映射表

推荐流程：

1. 管理端发布新 `DmProductRelease`，状态变为 `published`，但不切 `current_release_id`
2. OTA 任务选择目标设备批次，并记录本次 OTA 对应的目标 schema release，例如 `target_schema_release_id`
3. 设备 OTA 成功确认后，更新 `dm_device_info.version`
4. 同一事务或同一业务动作中，将该设备 `dm_device_info.schema_release_id_override` 设置为 `target_schema_release_id`
5. 清理或回填该设备 `cache:things:device:runtime`
6. 已升级设备开始使用新 release；未升级设备继续跟随旧 `product.current_release_id`
7. 当绝大多数设备完成升级后，管理端可选择执行 `ProductReleaseSetDefault`，把 `product.current_release_id` 切到新 release
8. 切默认前若仍存在少量旧固件设备，应保持不切，或给旧设备反向设置 `schema_release_id_override = old_release_id`

运行时规则保持不变：

```latex
device.schema_release_id_override ? override : product.current_release_id
```

但管理语义变为：

+ `published release`：可被引用的 schema 快照
+ `current_release_id`：未 override 设备的默认运行快照，不保证是最新
+ `schema_release_id_override`：单设备或批量设备的精确快照绑定，可指向新 release，也可指向历史 release
+ `DmDeviceInfo.Version`：固件版本元数据，只由 OTA、展示、诊断使用

需要提供的管理动作：

+ `ProductReleasePublish(productID, releaseNo)`：只发布快照，不切默认
+ `ProductReleaseSetDefault(productID, releaseID)`：显式切产品默认 release
+ `DeviceSchemaReleaseOverrideBatchUpdate(productID, releaseID, devices)`：批量给已升级设备绑定新 schema
+ `DeviceSchemaReleaseOverrideBatchClear(productID, devices)`：批量清除 override，恢复跟随产品默认
+ `OtaJobSuccessCallback(device, version, targetSchemaReleaseID)`：OTA 成功后写设备版本和 schema override

override 更新 / 清除的缓存规则：

+ `schema_release_id_override` 是 `DeviceRuntimeMeta.SchemaReleaseIDOverride` 的来源字段；update / clear 后必须清理对应设备的 `cache:things:device:runtime:{productID,deviceName}`。
+ 不淘汰任何 SchemaBundle cache。`things:schema:product:release:{productID}:{releaseID}:{scope}` 的 key 已包含 releaseID，旧 release cache 与新 release cache 可以并存；`things:schema:product:current:{productID}:{scope}` 也不因单设备 override 变化而失效。
+ 下次设备请求会先重新加载 `DeviceRuntimeMeta`：如果 `SchemaReleaseIDOverride > 0`，走 `ProductSchemaRepo.GetRelease(productID, override, scope)`；如果清除为 0，走 `ProductSchemaRepo.GetCurrent(productID, scope)`。
+ OTA 成功回调必须复用同一规则：设备版本和 `schema_release_id_override` 落库后，清理该设备 runtime meta cache；不能通过淘汰 SchemaBundle cache 来间接切 schema。

### 5.6 首次发布自动切默认
**问题：** 产品首次创建并发布时，没有旧设备需要保护。如果 `ProductReleasePublish` 不回写 `current_release_id`，则 `current_release_id` 保持为 0，导致：

+ `SchemaBundleCurrentCache` 返回 `ErrProductUnpublished`
+ `RuntimeSchemaBatch(ReleaseID=0)` 解析到 `current_release_id=0`，返回 "product not published"
+ ekuiper 无法获取 schema，设备无法解码
+ 必须额外调用一次 `ProductReleaseSetDefault`，增加操作复杂度

**方案：** `ProductReleasePublish` 成功后，若 `product.current_release_id == 0`（首次发布），自动执行 `SetDefault`：

+ 将 `dm_product_release.is_default` 设为 1
+ 回写 `dm_product_info.current_release_id = releaseRecord.ID`
+ 淘汰 `things:schema:product:current:{productID}:*` 缓存

**安全性：**

+ `current_release_id == 0` 表示该产品从未发布过，不存在旧设备需要保护
+ 所有设备都是新的或尚未绑定 schema，不存在"新 schema 解码旧数据"的风险
+ 后续发布（`current_release_id > 0`）仍遵循 5.5 节的 OTA 解耦规则，不自动切默认

**判断条件：**

```go
// 在 ProductReleasePublish Phase 3 成功后
if productInfo.CurrentReleaseID == 0 {
    // 首次发布：自动切默认
    releaseDB.SetDefault(ctx, productID, releaseRecord.ID)
    cache.InvalidateProductSchemaBundles(ctx, currentCache, releaseCache, productID)
}
```

### 5.7 端到端业务操作流程
从"模块草稿变更"到"所有相关产品完成升级"的完整操作链路。

#### 5.6.1 阶段一：编辑草稿
操作员修改模块模板：

```latex
1. 选择目标 dm_module_info
2. 增删改 dm_module_schema_info（点位草稿）
3. 草稿仅影响后续新发布，不影响任何已发布 release 或线上设备
```

草稿编辑没有"发布模块"步骤。模块模板始终是可编辑草稿，发布只发生在产品级别。

#### 5.6.2 阶段二：发现受影响产品
模块被修改后，需要确定哪些产品受影响。查询路径：

```latex
dm_module_schema_info.module_id
  -> dm_category_module.module_id  -> 产品（通过 category_id 沿 id_path 关联）
  -> dm_product_module.module_id   -> 产品（直接关联）
```

管理端应提供"模块影响分析"视图：

+ 列出所有绑定该模块的产品（分类来源 / 产品来源分别标注）
+ 每个产品的当前 release 状态和 current_release_id
+ 每个产品当前 release 中该模块的 snapshot_hash 与草稿最新 hash 的对比（是否确实有变化）

V1 不要求自动推送或自动发布，但建议提供批量发布入口，减少逐产品操作的手工成本。

#### 5.6.3 阶段三：逐产品发布
对每个受影响产品，操作员执行：

```latex
ProductReleasePublish(productID, releaseNo)
  -> 读取该产品绑定的所有模块的当前草稿
  -> 生成/复用模块快照 dm_module_schema_snapshot
  -> 冻结到新的 dm_product_release + dm_product_release_module
  -> 状态变为 published
  -> 不改变 product.current_release_id
  -> 不影响任何线上设备
```

此时新 release 已可用于 override 和 setDefault，但未 override 设备仍运行旧 schema。

批量场景：

+ 如果 ASN 下几十个产品都需要接入新点位，需要对每个产品分别调用 `ProductReleasePublish`
+ 可以使用相同的 `releaseNo` 命名规范（如 "v1.2.0"），但 release_no 只在产品内唯一，不同产品可以相同
+ 多个产品发布会复用同一个 `dm_module_schema_snapshot`（因为内容相同、hash 相同），避免重复存储

#### 5.6.4 阶段四：设备级切换（OTA + override）
新产品 release 发布后，需要将设备逐批切换到新 schema：

```latex
1. 创建 OTA 任务，指定 target_schema_release_id
2. 设备 OTA 成功后回调：
   - 更新 dm_device_info.version（固件版本）
   - 设置 dm_device_info.schema_release_id_override = target_schema_release_id
   - 清理该设备 runtime meta cache
3. 已升级设备开始使用新 release 解码
4. 未升级设备继续跟随旧 product.current_release_id
```

批量操作：

+ `DeviceSchemaReleaseOverrideBatchUpdate(productID, releaseID, devices)`：批量设置 override
+ 适用于不需要 OTA 固件升级、只需切换 schema 的场景（如新增纯云端解读点位）

#### 5.6.5 阶段五：产品切默认（全量切换）
当绝大多数设备完成升级后：

```latex
ProductReleaseSetDefault(productID, releaseID)
  -> 更新 dm_product_info.current_release_id
  -> 更新 dm_product_release.is_default
  -> 淘汰 things:schema:product:current:{productID}:* cache
  -> 未 override 设备自动跟随新 current
```

前置条件：

+ 目标 release 状态必须为 `published`
+ 建议确认大部分设备已完成 OTA
+ 如果仍有少量旧固件设备，可选择：
    - 暂不切默认，等全部设备升级完
    - 切默认 + 给旧设备反向设置 `schema_release_id_override = old_release_id`

#### 5.6.6 回滚流程
如果新 schema 存在问题需要回滚：

```latex
1. 方案 A：切回旧默认
   ProductReleaseSetDefault(productID, oldReleaseID)
   -> 未 override 设备立即回退到旧 schema
   -> 已 override 设备不受影响（仍指向各自的 release）

2. 方案 B：批量清除 override
   DeviceSchemaReleaseOverrideBatchClear(productID, devices)
   -> 指定设备恢复跟随 product.current_release_id

3. 方案 C：给问题设备指定旧 release
   DeviceSchemaReleaseOverrideBatchUpdate(productID, oldReleaseID, problemDevices)
   -> 精确控制哪些设备回退
```

旧 release 不会被删除，只是 `is_default` 不再为 1。历史 release 始终可用于回滚或审计。

#### 5.6.7 典型场景示例
**场景：ASN 模块新增 3 个点位，需同步到 ASN 下所有产品**

```latex
1. 编辑草稿
   - 在 dm_module_schema_info 中为 ASN_RT 模块新增 3 个点位

2. 影响分析
   - 查询发现 ASN 分类下有 20 个产品绑定了该模块

3. 批量发布
   - 对 20 个产品分别调用 ProductReleasePublish，releaseNo = "v1.2.0"
   - 20 个 release 共享同一个 dm_module_schema_snapshot（内容相同）

4. OTA 升级
   - 创建 OTA 任务，分批升级设备固件
   - 每批设备 OTA 成功后自动设置 schema_release_id_override

5. 切默认
   - 2 周后 95% 设备完成升级
   - 对剩余 5% 设备设置 override 指向旧 release
   - 调用 ProductReleaseSetDefault 切换全产品默认

6. 如需回滚
   - ProductReleaseSetDefault(productID, oldReleaseID)
   - 所有未 override 设备立即回退
```

---

## 6. 运行时 bundle 组装
运行时仍保持既有设备 release 解析规则：

```latex
device.schema_release_id_override ? override : product.current_release_id
```

`DmDeviceInfo.Version` 仍只表示固件版本，不参与 schema 路由。

`product.current_release_id` 表示产品默认运行 release，不保证是最新发布 release。  
发布新 release 后，只有显式执行 `ProductReleaseSetDefault` 或给设备设置 `schema_release_id_override`，运行时 schema 才会变化。

### 6.1 ProductSchemaRepo.GetCurrent
```latex
ProductSchemaRepo.GetCurrent(productID, scope)
  -> 读取 dm_product_info.current_release_id
  -> ProductSchemaRepo.GetRelease(productID, releaseID, scope)
```

### 6.2 ProductSchemaRepo.GetRelease
```latex
ProductSchemaRepo.GetRelease(productID, releaseID, scope)
  -> 校验 dm_product_release.status = published
  -> 批量读取 dm_product_release_module
  -> 批量读取 dm_module_schema_snapshot
  -> 批量读取 dm_module_info 获取 ModuleKind
  -> 按 ModuleKind + scope 过滤
  -> 组装 SchemaBundle{Model, Route}
```

L3 回填 SQL 策略必须避免 N+1。

推荐查询顺序：

```sql
-- 1. 校验 release，并保证 product_id / status 正确
SELECT id, product_id, status
FROM dm_product_release
WHERE id = ?
  AND product_id = ?
  AND deleted_time = 0;

-- 2. 一次读取 release 下全部模块引用；module_code 可直接用于 Route 组装
SELECT release_id, product_id, module_id, module_code, module_schema_snapshot_id
FROM dm_product_release_module
WHERE release_id = ?
  AND product_id = ?
  AND deleted_time = 0;

-- 3. 一次批量读取所有模块快照，禁止按模块逐条查询
SELECT id, module_id, schema_hash, schema_json, item_count
FROM dm_module_schema_snapshot
WHERE id IN (?)
  AND deleted_time = 0;

-- 4. 一次批量读取模块元信息；这里只需要 ModuleKind
SELECT id, module_kind
FROM dm_module_info
WHERE id IN (?)
  AND deleted_time = 0;
```

如果数据库访问层支持 JOIN，也可以把第 2、3、4 步合并成一次查询：

```sql
SELECT
    prm.release_id,
    prm.product_id,
    prm.module_id,
    prm.module_code,
    prm.module_schema_snapshot_id,
    mi.module_kind,
    s.schema_hash,
    s.schema_json,
    s.item_count
FROM dm_product_release_module prm
JOIN dm_module_schema_snapshot s
  ON s.id = prm.module_schema_snapshot_id
 AND s.deleted_time = 0
JOIN dm_module_info mi
  ON mi.id = prm.module_id
 AND mi.deleted_time = 0
WHERE prm.release_id = ?
  AND prm.product_id = ?
  AND prm.deleted_time = 0;
```

实现约束：

+ `schemaBundleCache.go` 的 L3 loader 不允许对每个 module 单独查询 `dm_module_schema_snapshot`。
+ 快照查询必须使用 `IN (...)` 批量查询或 JOIN 一次查出。
+ `ModuleKind` 查询可以复用当前批量能力，例如 `FindModuleKindByIDs`，但不能退化为逐 module 查询。
+ 查询结果必须校验完整性：`dm_product_release_module` 中每个 `module_schema_snapshot_id` 都必须能找到快照，每个 `module_id` 都必须能找到 `dm_module_info`。
+ 如果缺失 snapshot 或 module info，`GetRelease` 返回明确错误，不写入 L1/L2 cache。
+ Route 组装优先使用 `dm_product_release_module.module_code`，不必为了 Route 再从 `dm_module_info` 读取 `module_code`。
+ 反序列化 `schema_json` 后按 `module_kind + scope` 过滤，再组装 `SchemaBundle`；不要在 SQL 中硬编码 scope 过滤，避免 `ModuleKind` 规则分散。
+ 对同一个 `(productID, releaseID, scope)` 的并发 L3 回填应复用 `caches.Cache` 自带的 `syncx.SingleFlight`（`sf.Do(keyStr, ...)`），避免 Redis miss 时多个 goroutine 同时批量查询同一 release；`schemaBundleCache.go` 不额外引入 `singleflight.Group`。

scope 规则不变：

| scope | 包含模块 |
| --- | --- |
| `periodic_decode` | `property_tsdb + event` |
| `active_read` | `property_tsdb + property_no_store` |
| `full` | `property_tsdb + property_no_store + event + action` |


### 6.3 DeviceSchemaRepo.GetData
```latex
DeviceSchemaRepo.GetData(dev)
  -> 读取 DeviceRuntimeMeta.SchemaReleaseIDOverride
  -> override > 0: ProductSchemaRepo.GetRelease(productID, override, scope)
  -> override = 0: ProductSchemaRepo.GetCurrent(productID, scope)
```

不在设备缓存里存最终 `ResolvedReleaseID`。

原因：

+ 默认设备跟随产品 current
+ 产品 current 切换时，不需要大规模淘汰设备缓存
+ 只需要淘汰产品 schema cache

---

## 7. Cache 设计影响
cache key 继续沿用 scope 维度：

```latex
things:schema:product:current:{productID}:{scope}
things:schema:product:release:{productID}:{releaseID}:{scope}
```

cache value 仍是 `SchemaBundle{Model, Route}`。

变化点：

+ cache value 不感知 `dm_product_release_schema`
+ cache 构建源从逐行 release schema 改为模块快照
+ cache 里可以只保留运行时需要的模块和字段，不需要保留 `module_schema_snapshot_id`
+ `ModuleKind` 不进入 cache key，它只参与构建时过滤

缓存淘汰：

+ `dm_product_info.current_release_id` 变化：淘汰 `things:schema:product:current:{productID}:*`
+ 新 release 发布：只影响 `things:schema:product:release:{productID}:{releaseID}:{scope}`；不淘汰 current cache，因为 product current 仍指向旧 release
+ `dm_device_info.schema_release_id_override` 变化：不淘汰 SchemaBundle，只清理或回填对应设备 runtime meta；下次设备解析会基于新的 `DeviceRuntimeMeta.SchemaReleaseIDOverride` 选择 release cache 或 current cache
+ 模块模板变更：不淘汰已发布 release cache；只有新发布才生成新模块快照
+ 模块快照不可变，不需要按内容变更淘汰

---

## 8. 对 ekuiper-cluster 的影响
`ekuiper-cluster` 不需要感知 `dm_module_schema_snapshot`。

规则层仍通过 iot 接口获取 schema：

+ 周期上报解码使用 `scope=periodic_decode`
+ 主动抄读使用 `scope=active_read`
+ 管理端调试或全量查看才使用 `scope=full`

`ps_lookup` 的批量分组键保持：

```latex
(productID, effectiveReleaseID, scope)
```

其中：

+ `effectiveReleaseID` 由 iot 根据 `schema_release_id_override ? override : current_release_id` 解析
+ 不再按固件 `Version` 分组
+ 不再按产品 schema 全量大 JSON 分组

解码失败策略不变：

+ 某个子设备 release 或 schema 缺失，只跳过该子设备
+ 其他子设备继续解码
+ 错误明细写入日志和结果诊断字段

---

## 9. TDengine 影响
只有 `ModuleKindPropertyTsdb` 模块创建 TDengine 超级表。

模块快照不会改变 TDengine 表命名规则：

```latex
model_module__{moduleCode}
device_module__{productID}__{deviceName}__{moduleCode}
```

发布校验需要基于模块快照计算：

+ 列名是否合法
+ 列类型是否合法
+ 单模块列数是否超过 TDengine 限制
+ 最坏 SQL 长度是否超过 `maxSQLLength`
+ 单条 INSERT 是否可能超过异步批量阈值

当模块新增 `property_tsdb` 字段：

+ 先 `ALTER STABLE ADD COLUMN`
+ TDengine 成功后才允许 release 进入 `published`
+ 是否切产品 current 或绑定设备 override，由 OTA 和管理端生效流程单独决定
+ 旧 release 写入时只写自己的字段子集

当模块删除字段：

+ 不建议物理删除 TDengine 列
+ 新模块快照不再包含该字段
+ 历史 release 仍可按旧快照解析历史数据



### 9.1 PackReport 按时间戳合并写入
设备 `PackReport` 消息的 `properties` 字段为 `[]*TimeParams`，每个 `TimeParams` 携带一个时间戳和一组点位值。

**问题：** 当设备将同一时间戳的多个点位拆成多个 `TimeParams` 发送时（例如 5 个点位各自一条，时间戳相同），`InsertPackReport` 逐条遍历 `pTp`，每个元素独立调用 `splitByModule → InsertModuleData`，导致生成 5 条独立 INSERT：

```sql
-- 实际：每个点位一条 INSERT（低效）
INSERT INTO `device_module__...` USING `model_module__aux_asg_rt` (...) TAGS (...) (`ts`,`AC_U_LOAD_R`) VALUES (...)
INSERT INTO `device_module__...` USING `model_module__aux_asg_rt` (...) TAGS (...) (`ts`,`BAT1_MAX_MODULE_TEMPERATURE`) VALUES (...)
...
```

**期望：** 同一时间戳、同一模块的多个点位合并为一条多列 INSERT：

```sql
-- 期望：同模块同时间戳合并为一条 INSERT（高效）
INSERT INTO `device_module__...` USING `model_module__aux_asg_rt` (...) TAGS (...)
  (`ts`,`AC_U_LOAD_R`,`BAT1_MAX_MODULE_TEMPERATURE`,`BAT_FLOAT_VOLTAGE`,`DSP2_CERTIFIED_VERSION`,`LOAD_SIDE_AC_COUPING_STATUS`)
  VALUES ('2026-05-09T16:14:22+08:00',0.0,0.0,53.6,'0000',0)
```

**改造方案：**

在 `InsertPackReport` 中，循环写入前先按时间戳合并所有 `TimeParam`，再按模块分组写入：

1. 遍历 `pTp`，将相同时间戳的 `Params` 合并到 `map[int64]map[string]Param`
2. 对每个时间戳，调用 `splitByModule` 按模块分组
3. 对每个模块调用 `InsertModuleData`，一次写入所有列

这样 `GenInsertModuleSql` 生成的 SQL 天然包含多列，无需修改 TDengine 层代码。

**收益：**

+ 减少 TDengine SQL 条数：N 个同时间戳同模块点位从 N 条 INSERT 降为 1 条
+ 减少 SQL 解析开销和 USING/TAGS 重复字符串
+ 对 `HandlePropertyReport`（单时间戳）无影响，该路径已天然合并

---

## 10. 查询与管理端影响
### 10.1 产品详情 / 点位列表
产品详情默认展示 current release：

```latex
product.current_release_id
  -> dm_product_release_module
  -> dm_module_schema_snapshot.schema_json
  -> 展开为模块点位列表
```

如果需要展示草稿：

```latex
category modules + product modules
  -> dm_module_schema_info
  -> 展开为草稿点位列表
```

必须明确区分：

+ 草稿视图：可编辑，不可用于 TDengine 读写
+ release 视图：不可变，可用于运行时

### 10.2 按 identifier 查询
V1 优先从 cache 或展开后的模块快照查询。

如果后续管理端大量需要 SQL 级检索：

+ 增加 `dm_module_schema_snapshot_item` 作为模块快照级物化索引
+ 索引由 `dm_product_release_module + dm_module_schema_snapshot` 展开生成
+ 索引可删除重建，不影响 release 运行语义
+ 不复用已删除的 `dm_product_release_schema` 表名，避免重新引入旧表语义

---

## 11. 与旧设计的关系
### 11.1 保留的设计结论
以下结论继续保留：

+ `ModuleKind` 合并 affordance type 与存储策略
+ `ModuleKind` 创建后绝对不允许修改
+ `ModuleKind` 不冗余到 release schema 或模块快照
+ `DmDeviceInfo.Version` 不参与 schema release 路由
+ 设备默认跟随 `product.current_release_id`，但 current 是 fleet default，不等于最新 release
+ 设备可通过 `schema_release_id_override` 固定到指定快照；该快照可以是新 release，也可以是历史 release
+ scope 分为 `periodic_decode / active_read / full`

### 11.2 被本优化覆盖的设计结论
以下旧结论在 ASN/ASG 高复用场景下不再推荐：

+ `DmProductReleaseSchema` 逐行复制 `dm_module_schema_info`
+ 不新增 `DmProductReleaseModule`
+ release 字段集合完全由 `DmProductReleaseSchema` 表达

新的 V1 真相是：

```latex
DmProductRelease
  + DmProductReleaseModule
  + DmModuleSchemaSnapshot
  + immutable DmModuleInfo.ModuleKind
= release runtime truth
```

---

## 12. 可选 V2：分类级 release
V1 推荐仍保留产品级 `dm_product_release`。

原因：

+ 设备运行时已经按产品 current release 路由
+ 产品级 release 更容易和现有接口、缓存、权限模型兼容
+ 即使几十个产品共用 ASN 模型，也只是重复少量 release-module 引用行，不再重复上千点位行

如果未来 ASN / ASG 要作为独立发布对象，可再引入：

```latex
dm_category_release
dm_category_release_module
```

产品 release 再引用 `category_release_id`。

V1 暂不引入，避免在没有强需求时把发布模型做重。

---

## 13. 实施建议
实施顺序：

1. 新增 `dm_module_schema_snapshot`
2. 新增 `dm_product_release_module`
3. 发布逻辑改为生成模块快照并写 release-module 引用
4. `ProductSchemaRepo.GetRelease` 改为从模块快照组装 bundle
5. 将 `ProductReleasePublish` 与 `ProductReleaseSetDefault` 拆开，发布新 release 不自动切 `current_release_id`
6. OTA 成功链路支持写入 `schema_release_id_override`
7. 删除 `dm_product_release_schema` 结构和相关 model/repo/cache 旧路径
8. 管理端区分草稿视图、release 视图和产品默认运行视图
9. `ekuiper-cluster` 保持现有 scope 接口，不感知底层快照表变化

回滚策略：

+ 当前是开发期硬切，`dm_product_release_schema` 已删除，不能作为运行时回滚路径。
+ 如果模块快照方案需要回滚，应回滚本次代码和 migration，而不是在运行时重新启用旧表。
+ 切换点应以 `ProductSchemaRepo.GetRelease` 为边界一次性完成，避免 release bundle 同时存在两套来源。

---

## 14. 一句话结论
公司的 ASN / ASG 共用模型不需要额外 `dm_schema_profile`，直接用 `dm_product_category` 表达模型族即可；发布快照也不应继续逐点复制，而应按模块生成不可变 JSON 快照，再由产品 release 引用这些模块快照。这样既保留产品级 release 运行时语义，又避免几十个产品重复存储同一套上千点位。

