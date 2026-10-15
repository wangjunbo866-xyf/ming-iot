# 模块级超级表重构 — 多 Agent 并行开发计划
## 1. 总体策略
### 1.1 原则
+ **文件独占**：每个 Stream 独占一组文件，不同 Stream 不修改同一文件，避免合并冲突
+ **接口先行**：跨 Stream 依赖通过"先定义接口类型，后实现"解耦
+ **阶段门控**：每个阶段有明确的完成标准，全部通过后才进入下一阶段
+ **测试隔离**：每个 Stream 自带测试，不依赖其他 Stream 的实现细节

### 1.2 分支策略
每个 Stream 使用独立 worktree 分支，阶段完成后合并到 `feature/module-supertable` 集成分支。

```plain
feature/module-supertable          ← 集成分支，阶段完成后 merge
├── stream/a-domain-types         ← Stream A：领域类型
├── stream/b-sql-models           ← Stream B：SQL 模型
├── stream/c-tdengine-batch       ← Stream C：TDengine 异步批处理
├── stream/d-release-assembly     ← Stream D：发布快照组装
├── stream/e-module-validation    ← Stream E：模块校验逻辑
├── stream/f-cache-refactor       ← Stream F：缓存层重构
├── stream/g-tdengine-module      ← Stream G：TDengine 模块存储
├── stream/h-write-path           ← Stream H：写入路径
├── stream/i-read-path            ← Stream I：读取路径
├── stream/j-ekuiper              ← Stream J：规则引擎
└── stream/k-proto-api            ← Stream K：Proto/RPC/API
```

---

## 2. 阶段划分与依赖图
```plain
阶段 1（基础层，可完全并行）
  Stream A ─┐
  Stream B ─┼─→ 阶段门控 1
  Stream C ─┘

阶段 2（组装与校验，依赖阶段 1）
  Stream D ─┐
  Stream E ─┼─→ 阶段门控 2
  Stream G ─┘
  Stream K (proto 定义部分)

阶段 3（缓存与存储，依赖阶段 2）
  Stream F ─→ 阶段门控 3

阶段 4（业务路径，依赖阶段 3）
  Stream H ─┐
  Stream I ─┼─→ 阶段门控 4
  Stream J ─┘
  Stream K (API handler 部分)
```

---

## 2.5 设计变更影响记录（2026-04-23）
设计文档 §1.2 从"version → release 匹配"变更为"override ? override : current_release_id"直接路由。受影响的 Stream 及变更点：

| Stream | 变更点 | 具体影响 |
| --- | --- | --- |
| **A** | 错误类型重命名 | `ErrReleaseResolveMiss/Ambiguous/VersionConflict` → `ErrProductUnpublished/DeviceReleaseOverrideInvalid/ReleaseNoConflict` |
| **B** | DmProductRelease 移除 Version | 唯一索引 `(product_id, release_no)` 替代 `(product_id, version)`；新增 B8b 给 DmDeviceInfo 追加 `SchemaReleaseIDOverride`；删除 B14 `NormalizeVersion` |
| **E** | 发布入参不含 version | E8 入参改为 `releaseNo`；新增 E9/E10 设备 override 管理逻辑 |
| **F** | DeviceSchemaRepo.GetData 路由重写 | F2 `DeviceRuntimeMeta` 增加 `SchemaReleaseIDOverride`；F12 从 `version→release` 改为 `override→GetByRelease / current→GetCurrent` |
| **H** | 错误处理更新 | H4 从 "miss/ambiguous" 改为 "product_unpublished/device_override_invalid" |
| **J** | ekuiper 按 releaseID 而非 version | J1 返回 `effectiveReleaseID`；J2/J3/J4 全部改为 `(productID, releaseID)` |
| **K** | 新增 override RPC | K5b 新增 `DeviceSchemaReleaseOverride` message/RPC；K8/K9 入参改为 `{productID, releaseID}` |
| C | 无影响 | — |
| D | 无影响 | — |
| G | 无影响 | — |
| I | 无影响 | — |


---

## 3. Stream 详细任务
---

### Stream A：领域类型与公共契约
**分支**：`stream/a-domain-types`  
**仓库**：`iot`  
**独占文件**：

+ `iot/pkg/domain/schema/runtime_bundle.go`（新增）
+ `iot/pkg/domain/schema/runtime_bundle_test.go`（新增）
+ `iot/pkg/domain/deviceMsg/msgThing/repo_module.go`（新增）

**可修改文件**（仅限追加，不改动现有内容）：

+ `iot/pkg/domain/deviceMsg/msgThing/repo.go`（仅追加 module 方法签名到 interface）

**不碰文件**：schema/ 下所有现有文件、relationdb/、cache/、tsDB/

#### 任务清单
| # | 任务 | 产出 | 测试验证 |
| --- | --- | --- | --- |
| A1 | 定义 `SchemaRouteEntry` struct | `SchemaRouteEntry{ModuleID int64, ModuleCode string, PropertyType schema.Type}` | — |
| A2 | 定义 `SchemaBundleMeta` struct | `SchemaBundleMeta{ProductID string, ReleaseID int64}` | — |
| A3 | 定义 `SchemaBundle` struct | `SchemaBundle{Model schema.Model, Route map[string]SchemaRouteEntry, Meta SchemaBundleMeta}` | — |
| A4 | 实现 `SchemaBundle` 基本方法 | `GetRoute(identifier) (SchemaRouteEntry, bool)`；`ModuleCodes() []string`（去重）；`Model.String()` 不受 Route 影响 | 单元测试 |
| A5 | 定义 `BatchSplit` / `ModuleParams` 类型 | `BatchSplit{Modules []ModuleParams, Unknown map[string]Param}`；`ModuleParams{ModuleID, ModuleCode, Columns map[string]Param}` | — |
| A6 | 定义模块相关 domain 错误 | `ErrProductUnpublished`（产品未发布，current_release_id==0）、`ErrDeviceReleaseOverrideInvalid`（设备 override 指向不存在/跨产品/不可用的 release）、`ErrReleaseNoConflict`（并发发布 releaseNo 冲突）、`ErrModuleCodeConflict`、`ErrTDengineSQLTooLarge`、`ErrIdentifierNotInRelease` | — |
| A7 | 追加 `SchemaDataRepo` 模块方法签名 | 新增 `CreateModuleStable`、`AlterModuleStableAddColumns`、`DropModuleStable`、`CreateModuleTable`、`DropModuleTable`、`InsertModuleData`、`GetModuleDataByFilter`、`GetModulePropertyByID`、`GetLatestModulePropertyByID`、`GetLatestModuleData` | 编译通过 |
| A8 | 定义 `SanitizeTablePart` 函数 | 按 §2.8 规则实现：trim → 替换非法字符 → 压缩连续下划线 → 去首尾下划线 → 空值占位 → 数字开头补前缀 → 截断 → hash 防冲突 | 单元测试覆盖各种边界 |


#### 完成标准
+ `go test ./pkg/domain/schema/...` 通过
+ `go test ./pkg/domain/deviceMsg/...` 通过（编译级别）
+ `go build ./rpc/things/...` 通过（新类型仅追加，不影响现有编译）

---

### Stream B：SQL 模型
**分支**：`stream/b-sql-models`  
**仓库**：`iot`  
**独占文件**（全部新增）：

+ `iot/rpc/things/internal/relationdb/model/module.go`（DmModuleInfo、DmCategoryModule、DmProductModule）
+ `iot/rpc/things/internal/relationdb/model/release.go`（DmProductRelease、DmProductReleaseSchema）
+ `iot/rpc/things/internal/relationdb/model/module_schema.go`（DmModuleSchemaInfo、ModuleSchemaFilter）
+ `iot/rpc/things/internal/relationdb/repo/moduleInfo.go`
+ `iot/rpc/things/internal/relationdb/repo/categoryModule.go`
+ `iot/rpc/things/internal/relationdb/repo/productModule.go`
+ `iot/rpc/things/internal/relationdb/repo/productRelease.go`
+ `iot/rpc/things/internal/relationdb/repo/moduleSchema.go`
+ `iot/rpc/things/internal/relationdb/repo/moduleInfo_test.go`
+ `iot/rpc/things/internal/relationdb/repo/categoryModule_test.go`
+ `iot/rpc/things/internal/relationdb/repo/productModule_test.go`
+ `iot/rpc/things/internal/relationdb/repo/productRelease_test.go`
+ `iot/rpc/things/internal/relationdb/repo/moduleSchema_test.go`

**可修改文件**（仅追加字段，不改动现有字段）：

+ `iot/rpc/things/internal/relationdb/model/model.go`（给 `DmProductInfo` 追加 `CurrentReleaseID`；给 `DmDeviceInfo` 追加 `SchemaReleaseIDOverride`）

**不碰文件**：schema/、cache/、tsDB/、event/

#### 任务清单
| # | 任务 | 产出 | 测试验证 |
| --- | --- | --- | --- |
| B1 | 定义 DmModuleInfo 模型 | 按 §1.1 定义 GORM struct，含软删除 | 编译 |
| B2 | 定义 DmCategoryModule 模型 | 同上 | 编译 |
| B3 | 定义 DmProductModule 模型 | 同上 | 编译 |
| B4 | 定义 DmProductRelease 模型 | **不含 Version 字段**；唯一索引 `(product_id, release_no, deleted_time)`；不再依赖 `DmDeviceInfo.Version` 做运行时匹配 | 编译 |
| B5 | 定义 DmProductReleaseSchema 模型 | 含 `(release_id, identifier, deleted_time)` 和 `(release_id, module_id, identifier, deleted_time)` 唯一索引 | 编译 |
| B6 | 定义 DmModuleSchemaInfo 模型 | 含 `(module_id, identifier, deleted_time)` 唯一索引 | 编译 |
| B7 | 定义 ModuleSchemaFilter | 按 §1.3 定义查询过滤器 | 编译 |
| B8 | 给 DmProductInfo 追加 CurrentReleaseID | 仅追加 `CurrentReleaseID int64` 字段，golang tag 带 `default:0` | 编译 + 现有测试不回归 |
| B8b | 给 DmDeviceInfo 追加 SchemaReleaseIDOverride | 仅追加 `SchemaReleaseIDOverride int64` 字段，golang tag 带 `default:0`；0 表示不覆盖，>0 表示固定使用指定历史发布快照 | 编译 + 现有测试不回归 |
| B9 | 实现 ModuleInfoRepo CRUD | Create/Update/Delete/Index/Read，含 moduleCode 全局唯一校验 | 单元测试 |
| B10 | 实现 CategoryModuleRepo CRUD | MultiUpdate/Index，含去重逻辑 | 单元测试 |
| B11 | 实现 ProductModuleRepo CRUD | MultiUpdate/Index，含展平后 identifier 唯一校验（预留接口，实际校验需要读 moduleSchema，此处仅做 product+module 去重） | 单元测试 |
| B12 | 实现 ProductReleaseRepo CRUD | Publish（insert release + bulk insert schemas）、Index、Read、SetDefault、按 (productID, releaseNo) 唯一查询；**不再有按 version 查询的需求** | 单元测试含并发 releaseNo 冲突场景 |
| B13 | 实现 ModuleSchemaRepo CRUD | Create/Update/Delete/Index/TslImport，按 ModuleSchemaFilter 查询 | 单元测试 |


#### 完成标准
+ `go test ./rpc/things/internal/relationdb/...` 通过
+ `go build ./rpc/things/...` 通过
+ 现有 relationdb 测试不回归

---

### Stream C：TDengine 异步批处理改造
**分支**：`stream/c-tdengine-batch`  
**仓库**：`saas`  
**独占文件**：

+ `saas/share/clients/tdEngine.go`（重写 AsyncInsert 相关逻辑）
+ `saas/share/clients/tdEngine_batch_test.go`（新增）
+ `saas/share/clients/tdEngine_test.go`（新增或追加）

**可修改文件**：

+ `saas/share/conf/database.go`（追加 TSDB 配置项）

**不碰文件**：iot/ 下所有文件、ekuiper-cluster/

#### 向后兼容背景
`conf.TSDB` 是 `saas` 公共类型，被以下服务消费：

+ `iot/rpc/things` — 通过 YAML 加载，目前只设 `DSN`，`DBType`/`Driver` 依赖 struct tag default
+ `saas/rpc/timed/timedjob` — **直接构造 **`conf.TSDB{}`（硬编码 inline），不读 YAML
+ `saas/share/clients/tdEngine.go` — `NewTDengine()` 只读 `Driver` 和 `DSN`，不读新字段
+ `saas/share/stores/conn.go` — `InitTsConn()` 只读 `DBType` 和 `DSN`，不读新字段

新增配置项**必须**使用 go-zero struct tag `json:",default=..."` 提供默认值，确保：

+ 现有 YAML 不追加新字段时，go-zero 自动填充默认值
+ `conf.TSDB{}` 硬编码构造走 Go 零值（int 为 0），`NewTDengine` 内部需将 0 映射为实际默认值
+ 不影响 `InitTsConn` 等不读新字段的代码路径

#### 任务清单
| # | 任务 | 产出 | 测试验证 |
| --- | --- | --- | --- |
| C1 | 扩展 TSDB 配置 | 增加 `AsyncFlushMs`（`json:",default=50"`）、`AsyncMaxSQLBytes`（`json:",default=786432"`）、`AsyncMaxArgs`（`json:",default=4096"`）、`AsyncWarnSQLBytes`（`json:",default=524288"`） | 编译 |
| C2 | 扩展 ExecArgs | 增加 `QueryBytes int` 和 `ArgCount int` 字段 | 编译 |
| C3 | `NewTDengine` 内部零值保护 | 当 `DataSource.AsyncFlushMs == 0` 时内部赋值为 50；`AsyncMaxSQLBytes == 0` 时赋值为 786432；`AsyncMaxArgs == 0` 时赋值为 4096；`AsyncWarnSQLBytes == 0` 时赋值为 524288。覆盖 `conf.TSDB{}` 硬编码构造场景 | 单元测试：构造 `conf.TSDB{}` 验证内部使用正确默认值 |
| C4 | 重写 asyncInsertRuntime | 从"按条数 flush"改为"按 sqlBytes + argCount"双阈值 flush；内部保留 `hardMaxBatchCnt=64` 兜底 | 单元测试 |
| C5 | 实现 candidateSQLBytes 精确计算 | 维护 `len("insert into ") + Σ(item.Query + " ") + len(";")` | 单元测试验证与最终 SQL 长度一致 |
| C6 | 实现单条 SQL 超限保护 | 单条 `QueryBytes > AsyncMaxSQLBytes` 或 `ArgCount > AsyncMaxArgs` 时直接报错不入队 | 单元测试 |
| C7 | 实现监控日志 | 每次 flush 记录 `reason/sqlBytes/argCount/batchCount/duration`；超 warn 阈值打 warn；单条超限打 error | 单元测试验证日志调用 |
| C8 | 保留向后兼容 | `AsyncInsert(query, args...)` 接口签名不变，内部自动计算 `QueryBytes` 和 `ArgCount` | 现有调用方零改动 |
| C9 | **跨服务编译验证** | 验证 `saas` 全量编译通过；验证 `iot` 引用 `conf.TSDB` 的代码编译通过（`NewTDengine`/`InitTsConn` 调用点不受影响）；验证 `saas/rpc/timed/timedjob` 中 `conf.TSDB{}` 构造不受影响 | `go build ./...` 通过（saas）；`go build ./rpc/things/...` 通过（iot，需先 go mod edit replace 指向本地 saas） |


#### 完成标准
+ `go test ./share/clients/ -run TestBatch` 通过
+ `go test ./share/clients/ -run TestAsyncInsert` 通过
+ 覆盖：拆批 by sqlBytes、拆批 by args、内部 count cap、单条超限、空 batch flush、SQL/args 顺序稳定性
+ `go build ./...` 通过（saas 全量编译不报错）
+ **C9 跨服务验证通过**：iot 和 timedjob 在未追加 YAML 配置时编译 + 运行正常

---

### Stream D：发布快照组装
**分支**：`stream/d-release-assembly`  
**仓库**：`iot`  
**依赖**：Stream A（SchemaBundle 类型）、Stream B（DmProductReleaseSchema 模型）  
**合并后开始**

**独占文件**（新增）：

+ `iot/rpc/things/internal/relationdb/repo/assemble_release.go`
+ `iot/rpc/things/internal/relationdb/repo/assemble_release_test.go`

**不碰文件**：cache/、tsDB/、event/、logic/

#### 任务清单
| # | 任务 | 产出 | 测试验证 |
| --- | --- | --- | --- |
| D1 | 实现 `ReleaseSchemaToModel` | `[]DmProductReleaseSchema → schema.Model`；复用现有 `ToPropertyDo` 逻辑模式 | 单元测试 |
| D2 | 实现 `ReleaseSchemaToRoute` | `[]DmProductReleaseSchema → map[string]SchemaRouteEntry` | 单元测试 |
| D3 | 实现 `AssembleSchemaBundle` | 组合 Model + Route + Meta → `*SchemaBundle` | 单元测试 |
| D4 | 实现 identifier 重复校验 | 展平后 identifier 重复时返回错误 | 单元测试 |
| D5 | 实现 identifier 命名校验 | 不满足 `^[a-z][a-z0-9_]{0,63}$` 时返回错误 | 单元测试 |
| D6 | 实现非法类型校验 | 包含 struct/array 时返回错误 | 单元测试 |
| D7 | 实现单模块 SQL 大小估算 | 估算最坏列数 / 最坏行宽 / 最坏 INSERT SQL 长度，超阈值返回错误；阈值可作为参数传入 | 单元测试 |
| D8 | 组装 DmSchemaCore 映射 | 将 `DmSchemaCore` 中的 `Affordance` JSON 反序列化为 `schema.PropertyAffordance`，构造 `schema.Property` | 单元测试覆盖全部标量类型 |


#### 完成标准
+ `go test ./rpc/things/internal/relationdb/repo/ -run TestAssembleRelease` 通过
+ 覆盖：正常组装、identifier 重复、非法命名、struct/array 拒绝、空模块、多模块组合

---

### Stream E：模块校验与业务逻辑骨架
**分支**：`stream/e-module-validation`  
**仓库**：`iot`  
**依赖**：Stream A（错误类型）、Stream B（Repo CRUD）  
**合并后开始**

**独占文件**（新增）：

+ `iot/rpc/things/internal/logic/schemamanage/module_schema_create_logic.go`
+ `iot/rpc/things/internal/logic/schemamanage/module_schema_create_logic_test.go`
+ `iot/rpc/things/internal/logic/schemamanage/module_schema_update_logic.go`
+ `iot/rpc/things/internal/logic/schemamanage/module_schema_update_logic_test.go`
+ `iot/rpc/things/internal/logic/schemamanage/module_schema_tsl_import_logic.go`
+ `iot/rpc/things/internal/logic/schemamanage/module_schema_tsl_import_logic_test.go`
+ `iot/rpc/things/internal/logic/schemamanage/module_schema_validate.go`（共享校验函数）
+ `iot/rpc/things/internal/logic/productmanage/module_info_crud_logic.go`
+ `iot/rpc/things/internal/logic/productmanage/category_module_logic.go`
+ `iot/rpc/things/internal/logic/productmanage/product_module_logic.go`
+ `iot/rpc/things/internal/logic/productmanage/product_release_publish_logic.go`
+ `iot/rpc/things/internal/logic/productmanage/product_release_publish_logic_test.go`

**依赖接口**（E 通过 `SchemaDataRepo` 接口调用 TDengine 操作，测试用 mock，实现由 Stream G 提供，合并后自动接线）：

+ `msgThing.SchemaDataRepo.CreateModuleStable`
+ `msgThing.SchemaDataRepo.AlterModuleStableAddColumns`

**不碰文件**：cache/、tsDB/（直接实现层）、event/、现有 logic 文件

#### 任务清单
| # | 任务 | 产出 | 测试验证 |
| --- | --- | --- | --- |
| E1 | 实现 `validateScalarTypeOnly` | 共享校验函数：遍历 property define，struct/array 直接拒绝；允许 bool/smallInt/smallIntUnsigned/.../enum/timestamp 全集 | 单元测试覆盖每个类型 |
| E2 | 实现 ModuleSchemaCreate logic | 调用 `validateScalarTypeOnly` + ModuleSchemaRepo.Create | 单元测试 |
| E3 | 实现 ModuleSchemaUpdate logic | 计算新增字段集合 → 调 `SchemaDataRepo.AlterModuleStableAddColumns(moduleCode, newProperties)` → **仅在 ALTER 成功后**才提交 `dm_module_schema_info` 模板更新；ALTER 失败则整体失败不写入 DB | 单元测试：mock SchemaDataRepo 验证 ALTER 调用顺序和失败回滚 |
| E4 | 实现 ModuleSchemaTslImport logic | Excel/TSL 批量导入 → `validateScalarTypeOnly` → 批量 Create | 单元测试 |
| E5 | 实现 ModuleInfo CRUD logic | Create 含 moduleCode 命名校验 + 全局唯一校验；Update/Delete/Index/Read | 单元测试 |
| E6 | 实现 CategoryModule 绑定 logic | MultiUpdate/Index | 单元测试 |
| E7 | 实现 ProductModule 草稿 logic | MultiUpdate/Index；展平 identifier 唯一校验 | 单元测试 |
| E8 | 实现 ProductReleasePublish | 读草稿 → 读模板 → 调 assemble 校验（依赖 Stream D） → **首次发布某 moduleCode 时调 **`SchemaDataRepo.CreateModuleStable` → **发布版本有新增字段时先调 **`SchemaDataRepo.AlterModuleStableAddColumns` → ALTER 成功后才 insert release（`product_id + release_no`，**不含 Version**） + schemas → 可选更新 current_release_id | 单元测试含：重复 identifier 失败、struct/array 失败、标量全类型通过、releaseNo 冲突、默认 release 更新、首次创建 STABLE、追加列成功、追加列失败整体回滚 |
| E9 | 实现 DeviceSchemaReleaseOverrideUpdate | 校验 `releaseID` 属于同一 `productID` 且可用 → 更新 `dm_device_info.schema_release_id_override` → 淘汰对应 `cache:things:device:runtime` | 单元测试含：正常 override、跨产品 override 拒绝、不存在 release 拒绝 |
| E10 | 实现 DeviceSchemaReleaseOverrideClear | 更新 `dm_device_info.schema_release_id_override = 0` → 淘汰对应 `cache:things:device:runtime` | 单元测试 |


#### E3 / E8 与 Stream G 的衔接机制
E3 和 E8 通过 `msgThing.SchemaDataRepo` 接口调用 TDengine 操作，**不直接依赖 Stream G 的具体实现**：

```go
// E8 伪代码
func (l *ProductReleasePublishLogic) Publish(req *PublishReq) error {
    // req.ReleaseNo 为产品发布版本号（如 r1 / 2026.04）
    // 1. 读草稿、读模板、assemble 校验
    // 2. 对每个 moduleCode，判断是否首次发布：
    //    首次 → l.svcCtx.SchemaDataRepo.CreateModuleStable(moduleCode, properties)
    //    非首次且有新增字段 → l.svcCtx.SchemaDataRepo.AlterModuleStableAddColumns(moduleCode, newProps)
    //    任一 TDengine 操作失败 → 直接返回错误，不写入 release snapshot
    // 3. 全部 TDengine 操作成功后，才执行 DB 事务：
    //    insert DmProductRelease(product_id, release_no)  ← 不含 Version
    //    bulk insert DmProductReleaseSchema
    // 4. 可选更新 current_release_id
}
```

```plain
E 的测试：mock SchemaDataRepo，验证调用顺序和失败场景
         │
         │  合并阶段 2 门控时自动接线
         ▼
G 的实现：schemaDataRepo.CreateModuleStable / AlterModuleStableAddColumns
         真正执行 TDengine DDL
```

合并时不需要额外集成任务：E 已经通过接口完整调用 G，只要 `ServiceContext.SchemaDataRepo` 指向 G 提供的实现即可。

#### 完成标准
+ `go test ./rpc/things/internal/logic/schemamanage/...` 通过
+ `go test ./rpc/things/internal/logic/productmanage/...` 通过
+ 编译通过
+ E8 测试覆盖：首次创建 STABLE、追加列成功、追加列失败回滚（mock 验证 DB 未写入）

---

### Stream F：缓存层重构
**分支**：`stream/f-cache-refactor`  
**仓库**：`iot`  
**依赖**：Stream A、B、D 全部合并后开始  
**复杂度**：高 — 需要理解现有缓存链路

**独占文件**（新增）：

+ `iot/rpc/things/internal/cache/schemaBundleCache.go`（产品 current/release SchemaBundle 缓存）
+ `iot/rpc/things/internal/cache/schemaBundleCache_test.go`
+ `iot/rpc/things/internal/cache/deviceRuntimeMetaCache.go`
+ `iot/rpc/things/internal/cache/deviceRuntimeMetaCache_test.go`
+ `iot/rpc/things/internal/cache/deviceOtaConfirmCache.go`
+ `iot/rpc/things/internal/cache/deviceSecretCache.go`

**可修改文件**（追加，保留旧接口）：

+ `iot/rpc/things/internal/cache/schemaRepo.go`（旧 ristretto 缓存保留 legacy，新入口指向 schemaBundleCache）
+ `iot/rpc/things/internal/svc/service_context.go`（追加新 Cache 字段）
+ `iot/rpc/things/internal/startup/init.go`（追加新 Cache 初始化）
+ `iot/rpc/things/internal/logic/assembleDevice.go`（追加 runtime meta 组装函数，不改动现有详情拼装）

**不碰文件**：tsDB/、event/、logic/ 下业务逻辑

#### 任务清单
| # | 任务 | 产出 | 测试验证 |
| --- | --- | --- | --- |
| F1 | 定义 `ProductReleaseKey` | `type ProductReleaseKey struct { ProductID string; ReleaseID int64 }`，实现 `String()` | — |
| F2 | 定义 `DeviceRuntimeMeta` | 按 §3.4 定义轻量 struct：TenantCode/ProjectID/AreaID/AreaIDPath/ProductID/DeviceName/**SchemaReleaseIDOverride**/Version（固件版本，仅供展示/OTA，不参与 schema 路由）/IsOnline/IsEnable/Status/DeviceType/FirstLogin/LastBind/UserID/BelongGroup/Tags | — |
| F3 | 定义 `DeviceOtaConfirmMeta` | `NeedConfirmVersion string; NeedConfirmJobID string` | — |
| F4 | 定义 `DeviceSecretInfo` | `ProductID string; DeviceName string; Secret string` | — |
| F5 | 实现 ProductSchemaBundleCache（current） | 基于 `caches.Cache[SchemaBundle, string]`；L3 loader 调 `ProductSchemaRepo` 读 release + assemble；失效策略按 §3.4 | fake loader 测试 |
| F6 | 实现 ProductSchemaBundleCache（release） | 基于 `caches.Cache[SchemaBundle, ProductReleaseKey]`；L3 loader 调 `ProductSchemaRepo.GetByRelease` | fake loader 测试 |
| F7 | 实现 DeviceRuntimeMetaCache | 基于 `caches.Cache[DeviceRuntimeMeta, devices.Core]`；L3 loader 从 `dm_device_info` 组装轻字段；禁止拼 OSS URL/ProductImg 等 | fake loader 测试 |
| F8 | 实现 DeviceOtaConfirmCache | 独立缓存，OTA 状态变更只淘汰此缓存 | fake loader 测试 |
| F9 | 实现 DeviceSecretCache | 独立缓存，仅认证链路使用 | fake loader 测试 |
| F10 | 实现 ProductSchemaRepo.GetCurrent | `current_release_id → GetByRelease(productID, releaseID)` | 集成测试 |
| F11 | 实现 ProductSchemaRepo.GetByRelease | `dm_product_release_schema → AssembleSchemaBundle → 缓存` | 集成测试 |
| F12 | 实现 DeviceSchemaRepo.GetData | **路由逻辑重写**：读取 `DeviceRuntimeMetaCache` 中的 `SchemaReleaseIDOverride` → 若 `> 0`：校验 override release 存在且属于同一产品 → `GetByRelease(productID, override)` → 若 `== 0`：直接调 `GetCurrent(productID)` → 若产品未发布（`current_release_id==0`）返回 `ErrProductUnpublished` → 若 override 非法返回 `ErrDeviceReleaseOverrideInvalid` | fake loader 测试覆盖：正常跟随 current、使用 override、override 非法、产品未发布 |
| F13 | 实现失效逻辑 | 跨副本 pub/sub 广播；按 §3.4 失效触发事件淘汰对应缓存 | 单元测试 |
| F14 | 注册到 ServiceContext | 追加新 Cache 字段和初始化逻辑 | 编译通过 |


#### 完成标准
+ `go test ./rpc/things/internal/cache/...` 通过
+ 所有 fake loader 测试覆盖：GetCurrent/GetByRelease/GetData/失效/product_unpublished/device_override_invalid
+ 编译通过
+ 旧 `SchemaRepo`（ristretto）未删除，legacy 路径仍可工作

---

### Stream G：TDengine 模块存储
**分支**：`stream/g-tdengine-module`  
**仓库**：`iot`  
**依赖**：Stream A（SchemaBundle、SanitizeTablePart）、Stream C（新的 AsyncInsert）  
**可与 Stream D/E 并行**

**独占文件**（新增）：

+ `iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/moduleData.go`
+ `iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/moduleData_test.go`
+ `iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/moduleManage.go`
+ `iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/moduleManage_test.go`

**可修改文件**（追加方法，不改动现有方法）：

+ `iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/schema.go`（追加 `GetModuleStableName`、`GetModuleTableName`）

**不碰文件**：propertyData.go、eventData.go、productManage.go、deviceManage.go、relationdb/、cache/、logic/

#### 任务清单
| # | 任务 | 产出 | 测试验证 |
| --- | --- | --- | --- |
| G1 | 追加 SchemaStore 模块命名方法 | `GetModuleStableName(moduleCode) → "model_module__" + SanitizeTablePart(moduleCode, ...)`；`GetModuleTableName(pid, dname, mc) → "device_module__" + ...` | 单元测试 |
| G2 | 实现 CreateModuleStable | `CREATE STABLE IF NOT EXISTS` + 动态列定义（从 `[]schema.Property` 读取类型映射） + 标准 TAGS | 单元测试 |
| G3 | 实现 AlterModuleStableAddColumns | `ALTER STABLE ADD COLUMN` 批量执行 | 单元测试 |
| G4 | 实现 DropModuleStable | `DROP STABLE IF EXISTS` | 单元测试 |
| G5 | 实现 CreateModuleTable | `INSERT ... USING` 首次写入时自动创建，但提供显式创建方法 | 单元测试 |
| G6 | 实现 DropModuleTable | `DROP TABLE IF EXISTS` | 单元测试 |
| G7 | 实现 GenInsertModuleSql | 按模块生成 INSERT SQL：列名 = identifier，参数按类型映射，TAGS 从 Optional 填充 | 单元测试验证 SQL 生成正确性 |
| G8 | 实现 InsertModuleData | 调 GenInsertModuleSql → AsyncInsert | — |
| G9 | 实现 GetModuleDataByFilter | 按 moduleCode + FilterOpt 查询子表，支持 product_id/device_name 过滤 | 单元测试 |
| G10 | 实现 GetModulePropertyByID | 单点位查询 | 单元测试 |
| G11 | 实现 GetLatestModulePropertyByID | 最新值查询 | 单元测试 |
| G12 | 实现 GetLatestModuleData | 单设备单模块全量最新 | 单元测试 |
| G13 | 实现发布前 SQL 大小估算辅助 | 公开函数：给定 `[]schema.Property` + moduleCode，估算最坏 INSERT SQL 字节数和参数数 | 单元测试 |


#### 完成标准
+ `go test ./rpc/things/internal/tsDB/tdengine/schemaDataRepo/ -run TestModule` 通过
+ SQL 生成测试覆盖：单属性、多属性、全类型、含 string 大字段
+ 编译通过

---

### Stream H：写入路径重构
**分支**：`stream/h-write-path`  
**仓库**：`iot`  
**依赖**：Stream A、D、F、G 全部合并后开始  
**复杂度**：高 — 核心上报链路

**独占文件**（新增）：

+ `iot/rpc/things/internal/event/handler/deviceMsgEvent/module_batch_split.go`
+ `iot/rpc/things/internal/event/handler/deviceMsgEvent/module_batch_split_test.go`

**可修改文件**（改动现有逻辑）：

+ `iot/rpc/things/internal/event/handler/deviceMsgEvent/thing.go`（删除 DeviceSchemaReportAutoCreate 分支；HandlePropertyReport 改为 SchemaBundle.Route 分桶）

**不碰文件**：propertyData.go（方法保留但不再被 thing.go 调用）、eventData.go、cache/、relationdb/

#### 任务清单
| # | 任务 | 产出 | 测试验证 |
| --- | --- | --- | --- |
| H1 | 实现 `VerifyAndSplitBatch` | 基于 `SchemaBundle.Route` 的 O(N) 内存分桶；输出 `BatchSplit{Modules, Unknown}` | 单元测试覆盖：单模块、跨模块、全部未知、部分未知 |
| H2 | 重构 HandlePropertyReport | 删除 `DeviceSchemaReportAutoCreate`；改为：DeviceSchemaRepo.GetData → VerifyAndSplitBatch → 逐模块 InsertModuleData → Unknown 记日志丢弃 | 集成测试 |
| H3 | 处理全未知批次 | 全部 identifier 未命中 Route 时返回参数错误 | 单元测试 |
| H4 | 处理 release resolve 失败 | 产品未发布（`ErrProductUnpublished`）或设备 override 非法（`ErrDeviceReleaseOverrideInvalid`）时拒绝写入，不静默回退 | 单元测试 |
| H5 | 保持 shadow/latest 更新 | 模块写入成功后仍按 identifier 维度更新 per-property latest cache | 验证现有 API 不回归 |
| H6 | 保持 PubApp/WebHook | 模块写入成功后仍触发消息推送，形态不变 | 验证 |


#### 完成标准
+ `go test ./rpc/things/internal/event/handler/deviceMsgEvent/...` 通过
+ 核心场景覆盖：正常上报、跨模块上报、未知 identifier、产品未发布、设备 override 非法
+ 不再有 auto-create 分支

---

### Stream I：读取路径重构
**分支**：`stream/i-read-path`  
**仓库**：`iot`  
**依赖**：Stream A、F、G 全部合并后开始

**可修改文件**：

+ `iot/rpc/things/internal/logic/devicemsgmanage/property_log_index_logic.go`
+ 其他 property 查询 logic 文件（按实际需要）

**不碰文件**：thing.go、cache/、tsDB/ 下的文件（G 已完成）

#### 任务清单
| # | 任务 | 产出 | 测试验证 |
| --- | --- | --- | --- |
| I1 | PropertyLogIndex 改为按 ModuleCode 分桶 | N 个 dataID 批查 → 通过 Route 分桶 → M 条模块 SQL（M ≤ 模块数） | 单元测试 |
| I2 | 单点位查询路由 | 单次 map 查找 → 目标 moduleCode → 单条 SQL | 单元测试 |
| I3 | ProductSchemaIndex 切到 GetCurrent | 从 `SchemaBundle.Model` 返回产品当前视图 | 验证 |
| I4 | PropertyLatestIndex 保持不变 | 继续读 per-property latest cache | 验证不回归 |
| I5 | DeviceModuleDataIndex 新接口 | 按模块查设备时序数据，逻辑上按版本投影 | 单元测试 |
| I6 | DeviceModuleDataLatest 新接口 | 返回当前解析版本下可见字段 | 单元测试 |


#### 完成标准
+ 批量查询 SQL 数量从 N 降到 M
+ 现有 PropertyLatestIndex/GetStatus 不回归

---

### Stream J：规则引擎（ekuiper-cluster）
**分支**：`stream/j-ekuiper`  
**仓库**：`ekuiper-cluster`  
**依赖**：Stream K 提供 runtime-meta-batch / runtime-schema-batch HTTP 接口

**独占文件**：

+ `ekuiper-cluster/extensions/impl/cloudx/ps_lookup.go`（重写 lookup 逻辑）
+ `ekuiper-cluster/extensions/impl/cloudx/device/cache.go`（重写设备 schema 缓存）
+ `ekuiper-cluster/extensions/impl/cloudx/client/`（新增 batch HTTP client）

**可修改文件**：

+ `ekuiper-cluster/extensions/functions/acxPushDecode/handle_push_data.go`（skip 策略）
+ `ekuiper-cluster/extensions/functions/acxPushDecode/acx_push_decode.go`（参数约定更新）

**不碰文件**：iot/ 下所有文件

#### 任务清单
| # | 任务 | 产出 | 测试验证 |
| --- | --- | --- | --- |
| J1 | 实现 runtime-meta-batch HTTP client | 批量输入 `{productID, deviceName}[]` → 批量返回 `{productID, deviceName, effectiveReleaseID, timeZone, rs485Addr}[]`；其中 `effectiveReleaseID` 由 iot 服务端按 `schema_release_id_override ? override : current_release_id` 计算 | 单元测试 |
| J2 | 实现 runtime-schema-batch HTTP client | 批量输入去重 `{productID, releaseID}[]` → 批量返回 `{productID, releaseID, tsl}[]`；支持部分成功 | 单元测试 |
| J3 | 重写 ps_lookup lookup 流程 | `gateway-subset → runtime-meta-batch → runtime-schema-batch`；按 `(productID, effectiveReleaseID)` 分组去重 | 集成测试 |
| J4 | 实现按 (productID, releaseID) schema 缓存 | 替代旧的 productSchemaCache；LRU/TTL 有界缓存 | 单元测试 |
| J5 | 实现 skip 策略 | 单设备 schema 缺失/解码失败时跳过，不拖垮整批 | 单元测试 |
| J6 | 更新 acxPushDecode 参数约定 | schemasInfo 来源从 product-level 改为 device-level；每个子设备独立 schema | 单元测试 |


#### 完成标准
+ 同版本子设备不重复拉 schema
+ 单设备 schema 异常不拖垮整批
+ `go test ./extensions/...` 通过

---

### Stream K：Proto / RPC / API
**分支**：`stream/k-proto-api`  
**仓库**：`iot`

分两个阶段：

**阶段 2（与 D/E 并行）— Proto 定义**：

**独占文件**：

+ `iot/rpc/things/proto/schema_module.proto`（新增）
+ 或在现有 proto 文件中追加 message/RPC

| # | 任务 | 产出 |
| --- | --- | --- |
| K1 | 定义 ModuleInfo CRUD message/RPC | Create/Update/Delete/Index/Read |
| K2 | 定义 ModuleSchema CRUD message/RPC | Create/Update/Delete/Index/TslImport |
| K3 | 定义 CategoryModule message/RPC | MultiUpdate/Index |
| K4 | 定义 ProductModule message/RPC | MultiUpdate/Index |
| K5 | 定义 ProductRelease message/RPC | Publish（入参为 `releaseNo`，**不含 **`version`）/Index/Read/SetDefault |
| K5b | 定义 DeviceSchemaReleaseOverride message/RPC | OverrideUpdate（设置设备 schema_release_id_override）/OverrideClear（清除 override） |
| K6 | 定义 DeviceModuleData message/RPC | Index/Latest |
| K7 | goctl 生成代码 | handler/types/server stub |


**阶段 4（依赖 F）— HTTP API + Logic**：

**独占文件**（新增）：

+ `iot/api/gateway/apifile/things/device/auth.api`（追加 batch 接口定义）
+ `iot/api/gateway/internal/logic/things/device/auth/runtime_meta_batch_logic.go`
+ `iot/api/gateway/internal/logic/things/device/auth/runtime_schema_batch_logic.go`
+ `iot/api/gateway/internal/handler/things/device/auth/runtime_meta_batch_handler.go`
+ `iot/api/gateway/internal/handler/things/device/auth/runtime_schema_batch_handler.go`

| # | 任务 | 产出 | 测试验证 |
| --- | --- | --- | --- |
| K8 | 实现 runtime-meta-batch | 批量输入 `{productID, deviceName}[]` → 批量返回轻量运行时元数据含 `effectiveReleaseID`（服务端按 `override ? override : current_release_id` 计算） | 单元测试 |
| K9 | 实现 runtime-schema-batch | 按 `{productID, releaseID}[]` 批量返回 runtime schema；支持部分成功 | 单元测试 |
| K10 | K7 生成的 RPC handler 接入 E 的 logic | 连接 proto stub → logic 实现 | 编译 + 基本验证 |


#### 完成标准
+ 阶段 2：proto 编译通过，goctl 生成代码编译通过
+ 阶段 4：HTTP batch 接口可调通；RPC handler 连接 logic

---

## 4. 阶段门控检查
### 门控 1（阶段 1 完成后）
合并 Stream A + B + C 到集成分支，验证：

```bash
# iot 仓库
cd /home/wangjunbo/workspace-aux/iot
go build ./...
go test ./pkg/domain/schema/...
go test ./pkg/domain/deviceMsg/...
go test ./rpc/things/internal/relationdb/...

# saas 仓库
cd /home/wangjunbo/workspace-aux/saas
go build ./...
go test ./share/clients/ -run TestBatch
go test ./share/clients/ -run TestAsyncInsert

# 跨服务编译验证（C9）
# iot go.mod 指向本地 saas 后确认编译通过
cd /home/wangjunbo/workspace-aux/iot
go mod edit -replace github.com/.../saas=../saas  # 按实际 module path
go build ./rpc/things/...
go build ./api/gateway/...
```

### 门控 2（阶段 2 完成后）
合并 Stream D + E + G + K(proto 部分)，验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go build ./...
go test ./rpc/things/internal/relationdb/repo/ -run TestAssembleRelease
go test ./rpc/things/internal/logic/schemamanage/...
go test ./rpc/things/internal/logic/productmanage/...
go test ./rpc/things/internal/tsDB/tdengine/schemaDataRepo/ -run TestModule
```

**额外验证（E****↔****G 衔接）**：

+ 确认 `ProductReleasePublish` logic 的 mock 测试覆盖了 `CreateModuleStable` 和 `AlterModuleStableAddColumns` 调用
+ 确认 ALTER 失败时 DB 未写入 release snapshot（回滚正确性）
+ 确认 `ServiceContext.SchemaDataRepo` 类型同时满足旧接口和新增模块方法

### 门控 3（阶段 3 完成后）
合并 Stream F，验证：

```bash
cd /home/wangjunbo/workspace-aux/iot
go build ./...
go test ./rpc/things/internal/cache/...
# 验证 legacy 路径不回归
go test ./rpc/things/internal/event/handler/deviceMsgEvent/...
```

### 门控 4（阶段 4 完成后）
合并 Stream H + I + J + K(API 部分)，全量验证：

```bash
# iot 全量
cd /home/wangjunbo/workspace-aux/iot
go build ./...
go test ./rpc/things/...
go test ./api/gateway/...

# ekuiper
cd /home/wangjunbo/workspace-aux/ekuiper-cluster
go test ./extensions/...
```

---

## 5. Agent 分配建议
| Agent | 主 Stream | 辅助 Stream | 说明 |
| --- | --- | --- | --- |
| Agent 1 | Stream A | — | 领域类型，纯新增，无风险 |
| Agent 2 | Stream B | — | SQL 模型 + Repo CRUD，纯新增 |
| Agent 3 | Stream C | — | TDengine 批处理改造，独立仓库 |
| Agent 4 | Stream D + E | — | 合并 A+B 后开始；发布快照组装 + 校验逻辑 |
| Agent 5 | Stream G | — | 合并 A 后开始；TDengine 模块存储 |
| Agent 6 | Stream F | — | 合并 D 后开始；缓存层重构，复杂度高 |
| Agent 7 | Stream H | — | 合并 D+F+G 后开始；写入路径 |
| Agent 8 | Stream I + K(API) | — | 合并 F+G 后开始；读取路径 + HTTP API |
| Agent 9 | Stream J | — | 独立仓库；合并 K(proto) 后开始 |
| Agent 10 | Stream K(proto) | — | 阶段 2 开始；proto 定义 + goctl 生成 |


### 并行度时间线
```plain
时间 →   T1          T2          T3          T4          T5
         ┃           ┃           ┃           ┃           ┃
Agent 1  [Stream A]  · merged ·  ·           ·           ·
Agent 2  [Stream B]  · merged ·  ·           ·           ·
Agent 3  [Stream C]  · merged ·  ·           ·           ·
Agent 4  · waiting ·  [Stream D] · merged ·  ·           ·
                     [Stream E]             ·           ·
Agent 5  · waiting ·  [Stream G] · merged ·  ·           ·
Agent 6  · waiting ·  · waiting · [Stream F] · merged ·  ·
Agent 7  · waiting ·  · waiting · · waiting · [Stream H] ·
Agent 8  · waiting ·  · waiting · · waiting · [Stream I] ·
                                                  [K-API]
Agent 9  · waiting ·  · waiting · · waiting · [Stream J] ·
Agent 10 · waiting ·  [K-Proto]  · merged ·  ·           ·

T1: 阶段 1 开始（3 agent 并行）
T2: 阶段 1 完成 → merge → 阶段 2 开始（4 agent 并行）
T3: 阶段 2 完成 → merge → 阶段 3 开始（1 agent）
T4: 阶段 3 完成 → merge → 阶段 4 开始（3 agent 并行）
T5: 全部完成 → 集成测试
```

---

## 6. 风险与缓解
| 风险 | 影响 | 缓解措施 |
| --- | --- | --- |
| Stream F 缓存重构影响现有链路 | 高 | 旧 ristretto 缓存保留不删；新缓存独立入口；渐进切换 |
| Stream H 写入路径重构引入 bug | 高 | 先写 VerifyAndSplitBatch 单测；thing.go 改动最小化；保留旧方法体便于回滚 |
| **E****↔****G 衔接：发布时 TDengine DDL 调用无人认领** | 高 | E3/E8 通过 `SchemaDataRepo` 接口调用 `CreateModuleStable` / `AlterModuleStableAddColumns`，测试用 mock；G 提供实现；合并时 `ServiceContext` 自动接线，无需额外集成任务 |
| **E****↔****G 衔接：ALTER 失败后 DB 与 TDengine 不一致** | 高 | E3/E8 强制"先 ALTER 成功再提交 DB"；ALTER 失败直接返回错误，DB 不写入；门控 2 增加回滚正确性验证 |
| Stream D 依赖 A+B 合并延迟 | 中 | D 可在 A/B 未合并时用 stub 类型先行开发，合并后替换 |
| Stream J 与 K 存在跨仓库依赖 | 中 | J 可先用 mock HTTP server 开发，K 完成后对接 |
| TDengine 列数/SQL 长度超限 | 中 | G13 提供发布前估算函数，E8 发布逻辑调用校验 |
| 多 Stream 合并冲突 | 中 | 文件独占原则；model.go 仅 B 追加字段；schema.go 仅 G 追加方法 |


---

## 7. 每个 Stream 的 Agent Prompt 模板
每个 Agent 启动时应传入以下信息：

```plain
## 角色
你是 Stream {X} 的开发 Agent。

## 任务
实现 {Stream X 名称} 的所有任务（{X1}~{Xn}）。

## 约束
- 分支：stream/{x-branch-name}
- 仓库：{repo-path}
- 独占文件：{file-list}
- 可修改文件：{file-list}（仅限追加/指定改动）
- 禁止修改：{file-list}
- 依赖：{dependency-streams} 已合并到集成分支

## 设计文档
参考 doc/2026-04-15-module-level-supertable-design.md 的 §{relevant-sections}。

## 开发流程
1. 从集成分支创建 worktree
2. 先写失败测试
3. 写最小实现
4. 确保本 Stream 所有测试通过
5. 提交到 stream/{x-branch-name}

## 完成标准
{from section above}
```

