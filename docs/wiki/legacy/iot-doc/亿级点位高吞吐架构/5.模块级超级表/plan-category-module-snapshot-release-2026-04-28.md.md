# 分类复用 + 模块快照发布执行计划
## 0. 目标
本文是以下设计的执行计划：

+ [design-category-module-snapshot-release-2026-04-28.md](/home/wangjunbo/workspace-aux/doc/module-level-supertable/design-category-module-snapshot-release-2026-04-28.md)
+ [design-module-level-supertable-optimization-2026-04-23.md](/home/wangjunbo/workspace-aux/doc/module-level-supertable/design-module-level-supertable-optimization-2026-04-23.md)
+ [design-module-kind-release-schema-2026-04-26.md](/home/wangjunbo/workspace-aux/doc/module-level-supertable/design-module-kind-release-schema-2026-04-26.md)

实施目标：

+ 用 `dm_product_category + dm_category_module` 表达 ASN / ASG 模型族。
+ 发布时把最终模块集合冻结到 `dm_product_release_module`。
+ 用 `dm_module_schema_snapshot.schema_json` 作为模块级快照权威。
+ 当前仍处于开发阶段，旧表 `dm_product_release_schema` 直接删除，不再作为可选物化索引保留。
+ `ProductSchemaRepo.GetRelease` 从模块快照组装 `SchemaBundle`。
+ `ProductReleasePublish` 只发布快照，不切 `current_release_id`。
+ OTA 成功或管理端批量操作通过 `schema_release_id_override` 切单设备 schema。
+ `ekuiper-cluster` 继续通过 `runtime-meta-batch + runtime-schema-batch` 获取运行时 schema。

非目标：

+ 不做 legacy `struct/array` 兼容迁移。
+ 不做“固件版本 -> schema release”全量映射。
+ 不做设备级 Redis SchemaBundle 常驻缓存。
+ 不把 `ModuleKind` 冗余进 release module 或 snapshot。

---

## 1. 固定约束
必须遵守：

+ 所有代码改动按 TDD：先写失败测试，再实现，再重构。
+ `ProductSchemaRepo.GetData() *schema.Model` 旧契约尽量保留；新增/改造内部 bundle 能力，不要求所有调用方一次性改完。
+ `DmModuleInfo.ModuleKind` 创建后不可修改。
+ `DmProductReleaseModule.ModuleCode` 是发布时冻结的只读冗余字段，用于 Route、导出、排查。
+ `DmProductReleaseModule` 不存 `ModuleKind`。
+ `schema_json` 必须是 `schemaVersion=2`，`extendConfig/affordance` 必须是 object，不允许 JSON-in-JSON。
+ `schemaBundleCache.go` L3 loader 必须批量查 snapshot，禁止 N+1。
+ `current_release_id` 是 fleet default，不是最新 release。
+ `Version` 只表示固件版本，不参与 schema 路由。
+ `acxPushDecode` 遇到单个子设备 release/schema 失败时跳过单设备，不允许整包失败。

---

## 2. 当前代码基线
当前仓库已有部分基础，但还没有完全落到模块快照方案：

| 区域 | 当前状态 | 需要改成 |
| --- | --- | --- |
| `iot/rpc/things/internal/relationdb/model/release.go` | 有 `DmProductRelease`、`DmProductReleaseSchema` | 删除 `DmProductReleaseSchema`，增加 `DmProductReleaseModule`、release 状态常量 |
| `iot/rpc/things/internal/relationdb/model/module_schema.go` | 有 `DmModuleSchemaInfo` | 增加 `DmModuleSchemaSnapshot` |
| `iot/rpc/things/internal/relationdb/repo/productRelease.go` | 发布仍围绕 `DmProductReleaseSchema` | 删除 `ProductReleaseSchemaRepo`，增加 release module / snapshot repo，发布写 release-module |
| `iot/rpc/things/internal/relationdb/repo/assemble_release.go` | 从 `[]DmProductReleaseSchema` 组装 bundle | 改为从模块 snapshot JSON 反序列化出的内存行组装 bundle，不再绑定旧表 model |
| `iot/rpc/things/internal/cache/schemaBundleCache.go` | L3 仍查 `dm_product_release_schema` | 改为 release-module + snapshot + module-kind 批量查询 |
| `iot/rpc/things/internal/logic/moduleschemaservice/product_release_publish_logic.go` | 发布 bulk insert `DmProductReleaseSchema` | 发布生成/复用模块 snapshot，写 `DmProductReleaseModule`，不切 current |
| `iot/rpc/things/internal/logic/moduleschemaservice/product_release_set_default_logic.go` | 需要确认默认切换语义 | 只在显式 set default 时更新 `current_release_id` |
| `iot/rpc/things/internal/logic/moduleschemaservice/device_schema_override_*` | 当前是 TODO | 需要真实更新 `schema_release_id_override` 并清 runtime meta |
| `ekuiper-cluster/extensions/impl/cloudx` | 已有 runtime batch 和 scope 骨架 | 确认 scope、cache key、skip 单设备失败、已解析结构缓存 |


---

## 3. 执行顺序
执行顺序必须从底层契约向上推进：

1. 数据模型和 AutoMigrate。
2. snapshot JSON codec 与 hash。
3. release-module / snapshot Repo。
4. snapshot -> SchemaBundle assembler。
5. 发布逻辑改造。
6. schema bundle cache L3 loader 改造。
7. runtime batch API 和旧 `GetData()` 兼容。
8. TDengine 发布前准备与 set default 解耦。
9. ekuiper lookup 验证。
10. 数据回填和端到端验证。

---

## 4. 任务拆解
### A. 基线与迁移
| ID | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| A1 | `/home/wangjunbo/workspace-aux/doc/module-level-supertable/migration.sql` | 审核现有完整迁移脚本，而不是只确认建表存在：`dm_product_release_module.module_code` 对新建表必须与 Go model 保持 `NOT NULL`；对旧表补列时必须先从 `dm_module_info` 回填，确认 `module_code IS NULL OR module_code=''` 的记录数为 0 后，再执行 `ALTER COLUMN module_code SET NOT NULL`；ASN / ASG 分类种子数据必须校验 `id_path`，例如 ASN `id=1,parent_id=0,id_path='1-'`，ASG `id=3,parent_id=1,id_path='1-3-'`，确保 `utils.GetIDPath("1-3-")` 可得到 `[1,3]` | `rg -n "module_code |
| A2 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/model/release_test.go` | 先写失败测试：`DmProductReleaseSchema` 不再存在；`DmProductReleaseModule.TableName()`、字段 `ReleaseID/ProductID/ModuleID/ModuleCode/ModuleSchemaSnapshotID/SourceCategoryID/SourceProductID` 存在 | `cd /home/wangjunbo/workspace-aux/iot && go test ./rpc/things/internal/relationdb/model` 应先失败 |
| A3 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/model/release.go` | 删除 `DmProductReleaseSchema`，增加 `DmProductReleaseModule`，增加 release status 常量：`publishing/published/publish_failed`，给 `DmProductRelease` 增加 `PublishError/PublishedAt/FailedAt` | A2 测试通过 |
| A4 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/model/module_schema_test.go` | 先写失败测试：`DmModuleSchemaSnapshot.TableName()`、字段 `ModuleID/SchemaHash/SchemaJSON/ItemCount` 存在 | `go test ./rpc/things/internal/relationdb/model` 应先失败 |
| A5 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/model/module_schema.go` | 增加 `DmModuleSchemaSnapshot`，`SchemaJSON` 使用 `json.RawMessage` 或 `string`，GORM 类型 `jsonb/json` | A4 测试通过 |
| A6 | `/home/wangjunbo/workspace-aux/iot/rpc/things/automigrate/auto_migrate.go` | AutoMigrate 追加 `DmModuleSchemaSnapshot`、`DmProductReleaseModule` | `cd /home/wangjunbo/workspace-aux/iot && go test ./rpc/things/automigrate` |


### B. Snapshot Codec
| ID | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| B1 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/module_schema_snapshot_test.go` | 先写失败测试：从 `DmModuleSchemaInfo` 生成 `schemaVersion=2` JSON；`extendConfig/affordance` 是 object；完整保留 `DmSchemaCore` 字段 | `go test ./rpc/things/internal/relationdb/repo -run Snapshot` 应先失败 |
| B2 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/module_schema_snapshot.go` | 增加 `BuildModuleSchemaSnapshotDoc(moduleID, rows)`，输出稳定 JSON 和 SHA-256 hash | B1 测试通过 |
| B3 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/module_schema_snapshot_test.go` | 增加 hash 稳定性测试：`extend_config` 字段顺序/空格不同但语义相同，hash 相同 | `go test ./rpc/things/internal/relationdb/repo -run SnapshotHash` |
| B4 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/module_schema_snapshot.go` | 规范化：items 按 `identifier` 排序；`extend_config/affordance` 必须是非空 JSON object；空字符串、`null`、double-escaped string、数组、非法 JSON、type 不匹配全部返回错误 | B3 和非法 JSON 测试通过 |
| B5 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/module_schema_snapshot_test.go` | 增加 hash 冲突测试：同一 `module_id + schema_hash` 下 `schema_json` 字节一致时复用，不一致时返回 `ErrModuleSchemaHashConflict` | `go test ./rpc/things/internal/relationdb/repo -run SnapshotHashConflict` |
| B6 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/module_schema_snapshot_test.go` | 增加 `struct/array` 被拒绝测试，确保新项目 V1 不允许创建发布 | `go test ./rpc/things/internal/relationdb/repo -run IllegalType` |


### C. Repo 层
| ID | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| C1 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/productReleaseModule_test.go` | 先写失败测试：`BulkInsert`、`FindByRelease(productID, releaseID)`、`module_code` 可读 | `go test ./rpc/things/internal/relationdb/repo -run ProductReleaseModule` 应先失败 |
| C2 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/productReleaseModule.go` | 删除 `ProductReleaseSchemaRepo`，新增 `ProductReleaseModuleRepo`：`MultiInsert`、`FindByRelease`、`FindSnapshotIDsByRelease` | C1 测试通过 |
| C3 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/moduleSchemaSnapshotRepo_test.go` | 先写失败测试：`FindByIDs` 一次批量返回全部 snapshot；缺失 ID 可被调用方发现 | `go test ./rpc/things/internal/relationdb/repo -run ModuleSchemaSnapshotRepo` |
| C4 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/moduleSchemaSnapshot.go` | 新增 `ModuleSchemaSnapshotRepo`：`FindByHash`、`FindByIDs`、`InsertIfAbsent` | C3 测试通过 |
| C5 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/productReleaseSnapshotQuery_test.go` | 先写失败测试：release 包含 10 个模块时，snapshot 查询走 `IN` 或 JOIN，不允许 N+1 | `go test ./rpc/things/internal/relationdb/repo -run ReleaseSnapshotQuery` |
| C6 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/productRelease.go` | 增加 `FindReleaseSnapshotRows(productID, releaseID)`，返回 release-module + snapshot + module_kind 的批量结果 | C5 测试通过 |


### D. Snapshot Bundle 组装
| ID | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| D1 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/assemble_snapshot_test.go` | 先写失败测试：从 `dm_product_release_module + schema_json + module_kind` 组装 `SchemaBundle{Model, Route}` | `go test ./rpc/things/internal/relationdb/repo -run AssembleSnapshot` 应先失败 |
| D2 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/assemble_snapshot.go` | 增加 `SnapshotRowsToSchemaBundle(productID, releaseID, scope, rows)` | D1 测试通过 |
| D3 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/assemble_snapshot_test.go` | 增加 scope 测试：`periodic_decode=property_tsdb+event`、`active_read=property_tsdb+property_no_store`、`full=全部` | `go test ./rpc/things/internal/relationdb/repo -run SnapshotScope` |
| D4 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/assemble_snapshot.go` | Route 使用 `DmProductReleaseModule.ModuleCode`，不要为了 Route 回查 `DmModuleInfo.ModuleCode` | D3 测试通过 |
| D5 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/assemble_snapshot_test.go` | 增加重复 identifier、非法 TDengine identifier、非法类型、缺 snapshot、缺 module info 测试 | `go test ./rpc/things/internal/relationdb/repo -run SnapshotInvalid` |


### E. 发布逻辑
| ID | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| E1 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/product_release_publish_logic_test.go` | 改写/新增失败测试：发布 ASN 产品时沿分类链路 + 产品模块生成最终模块集合，并冻结到 `DmProductReleaseModule` | `go test ./rpc/things/internal/logic/moduleschemaservice -run ProductReleasePublish` 应先失败 |
| E1a | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/product_release_module_resolver_test.go` | 先写失败测试：分类模块解析器能把 `category.id_path="1-3-"` 解析为 `[1,3]`，一次批量查询 `dm_category_module WHERE category_id IN (1,3)`，再与 `dm_product_module` 做 union；断言按 `module_id` 去重，并正确填充 `source_category_id` / `source_product_id` | `go test ./rpc/things/internal/logic/moduleschemaservice -run ProductReleaseModuleResolver` 应先失败 |
| E1b | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/product_release_module_resolver.go` | 新增分类模块解析器：读取 `product.category_id` 对应分类；用 `utils.GetIDPath(category.IDPath)` 解析链路；批量校验 `dm_product_category WHERE id IN (...)` 全链路存在且未软删；批量查 `CategoryModuleRepo.FindByFilter(CategoryIDs: pathIDs)`；查 `ProductModuleRepo.FindByFilter(ProductID: productID)`；按 `module_id` 去重，纯分类来源填离产品最近的 `source_category_id`，纯产品来源填 `source_product_id=productID`，分类+产品重复时产品来源优先；输出可直接写入 `DmProductReleaseModule` 的模块集合草稿 | E1a 测试通过 |
| E2 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/product_release_publish_logic.go` | 发布流程改为：调用分类模块解析器生成最终模块集合 -> 创建 release(publishing) -> 生成/复用 module snapshot -> 写 release module -> 校验 bundle -> status=published | E1 和 E1a 测试通过 |
| E2a | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/model/release_test.go`、`/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/productRelease_status_test.go` | 增加状态机测试：`publishing=0/published=1/publish_failed=2` 常量固定；`ProductReleaseRepo.Publish` 必须能保存 `status=publishing`，不能被 GORM 默认值改成 `published` | `go test ./rpc/things/internal/relationdb/model ./rpc/things/internal/relationdb/repo -run 'ProductReleaseStatus |
| E2b | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/product_release_publish_logic.go` | `ProductReleasePublish` 必须先写 `status=publishing`；TDengine/Bundle 成功后更新为 `published`；失败后更新为 `publish_failed` 并保留 release-module/snapshot | E2a 和 E11 测试通过 |
| E2c | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/cache/schemaBundleCache_scope_test.go`、`/home/wangjunbo/workspace-aux/iot/rpc/things/internal/cache/schemaBundleCache.go` | 增加测试：`ProductSchemaRepo.GetRelease` / L3 loader 必须拒绝 `status != published` 的 release | `go test ./rpc/things/internal/cache -run RejectsNonPublishedRelease` |
| E3 | 同上测试文件 | 增加测试：分类模块绑定发布后变更，旧 release 的 `DmProductReleaseModule` 不变 | `go test ./rpc/things/internal/logic/moduleschemaservice -run CategoryFreeze` |
| E4 | 同上实现文件 | 保证 `dm_category_module` 只在发布时解析；运行时不回查分类 | E3 测试通过 |
| E5 | 同上测试文件 | 增加测试：`category_id=0`、产品分类不存在、产品分类软删、`id_path` 中间节点不存在、`id_path` 中间节点软删时发布失败，不退化为只读 `dm_product_module` | `go test ./rpc/things/internal/logic/moduleschemaservice -run CategoryBoundary` |
| E6 | 同上实现文件 | 发布前校验产品分类有效、`id_path` 全链路存在且未软删；尽量在创建 `publishing` release 前失败，若已创建则必须转 `publish_failed` | E5 测试通过 |
| E7 | 同上测试文件 | 增加测试：分类链路和产品模块重复绑定同一 `module_id` 时只保留一条 release-module；来源审计按产品显式绑定优先，填 `source_product_id=product_id`、`source_category_id=0` | `go test ./rpc/things/internal/logic/moduleschemaservice -run ModuleDedup` |
| E8 | 同上实现文件 | 模块集合按 `module_id` 去重；纯分类来源记录离产品最近的 `source_category_id`；纯产品来源记录 `source_product_id`；分类+产品重复时按产品来源记录；如果需要产品差异，只允许新模块或新分类 | E7 测试通过 |
| E9 | 同上测试文件 | 增加测试：发布新 release 不更新 `dm_product_info.current_release_id` | `go test ./rpc/things/internal/logic/moduleschemaservice -run PublishDoesNotSetCurrent` |
| E10 | 同上实现文件 | 移除发布时 set default 行为：`ProductReleasePublish` 不更新 `dm_product_info.current_release_id`，不调用 `ProductReleaseSetDefault`，不淘汰 `things:schema:product:current:{productID}:*`；发布成功后只预热 `things:schema:product:release:{productID}:{releaseID}:{scope}`，供管理端预览、设备 override 和 OTA 成功后的单设备切换使用 | E9 测试通过；F5 覆盖 current cache 未被发布动作淘汰 |
| E11 | 同上测试文件 | 增加测试：TDengine/Bundle 校验失败时 release 状态变为 `publish_failed`，`publish_error` 有值，`DmProductReleaseModule` 保留 | `go test ./rpc/things/internal/logic/moduleschemaservice -run PublishFailedKeepsEvidence` |
| E12 | 同上实现文件 | 实现失败分支：第二阶段失败后用 SQL 事务更新 `status=publish_failed, failed_at, publish_error`，不删除 release-module/snapshot，不写 cache | E11 测试通过 |
| E13 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/product_release_retry_logic_test.go` | 先写失败测试：`ProductReleaseRetry(releaseID)` 只复用已冻结 release-module/snapshot，不回读当前分类或模板 | `go test ./rpc/things/internal/logic/moduleschemaservice -run ProductReleaseRetry` |
| E14 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/product_release_retry_logic.go` | 实现 retry：`publish_failed -> publishing -> published/publish_failed`，成功后写 `published_at`，失败后更新 `publish_error` | E13 测试通过 |
| E15 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/product_release_set_default_logic_test.go` | 先写失败测试：只有显式 `ProductReleaseSetDefault` 才更新 `current_release_id`，且 release 必须 published | `go test ./rpc/things/internal/logic/moduleschemaservice -run SetDefault` |
| E16 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/product_release_set_default_logic.go`、`/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/repo/productRelease_status_test.go` | 实现 set default：校验 product/release/status，`status != published` 必须拒绝，更新 current，淘汰 current cache | E15 测试通过 |
| E17 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/device_schema_override_update_logic_test.go` | 先写失败测试：override 必须同 product、published，更新设备字段并清 runtime meta cache；不得淘汰 current/release SchemaBundle cache | `go test ./rpc/things/internal/logic/moduleschemaservice -run DeviceSchemaOverride` |
| E18 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/device_schema_override_update_logic.go`、`device_schema_override_clear_logic.go` | 实现 update/clear，不再 TODO：用 `DeviceInfoRepo.UpdateWithField` 写 `schema_release_id_override`，再调用 `DeviceRuntimeMetaCache.SetData(ctx, devices.Core{ProductID, DeviceName}, nil)` 清理 `cache:things:device:runtime`；不调用 SchemaBundle current/release cache 失效 | E17 测试通过 |
| E18a | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/cache/schemaBundleCache_test.go`、`/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/device_schema_override_update_logic_test.go` | 增加测试：override 从 0 更新为 releaseID 后，下次 `GetDevice(dev, scope)` 重新加载 runtime meta，并走 `GetByRelease(productID, releaseID, scope)`；clear 后重新加载 runtime meta，并走 `GetCurrent(productID, scope)`；旧 SchemaBundle cache 不需要淘汰 | `go test ./rpc/things/internal/cache -run Override` 和 `go test ./rpc/things/internal/logic/moduleschemaservice -run DeviceSchemaOverrideCache` |
| E18b | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/relationdb/model/modelOta.go`、`/home/wangjunbo/workspace-aux/iot/rpc/things/proto/ota.proto`、`/home/wangjunbo/workspace-aux/doc/module-level-supertable/migration.sql` | 为 OTA job 增加 `target_schema_release_id`（默认 0）：创建 OTA 任务时可记录本次 OTA 成功后要绑定的 schema release；`target_schema_release_id=0` 表示只更新固件版本，不自动写 schema override | model/proto 生成和迁移脚本检查通过 |
| E18c | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/event/handler/deviceMsgEvent/ota_schema_release_callback_test.go` | 先写失败测试：OTA 进度成功（`DeviceStatusSuccess` / step=1）且 job 有 `target_schema_release_id` 时，回调更新设备 `version` 和 `schema_release_id_override`，清理 DeviceRuntimeMetaCache，不淘汰 SchemaBundle cache；跨产品、未 published release 必须拒绝写 override | `go test ./rpc/things/internal/event/handler/deviceMsgEvent -run OtaJobSuccessCallback` 应先失败 |
| E18d | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/event/handler/deviceMsgEvent/ota.go`、`/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/device_schema_override_update_logic.go` | 实现 `OtaJobSuccessCallback(device, version, targetSchemaReleaseID)` 或等价内部函数：OTA 成功后在同一业务动作中更新 `dm_device_info.version` 与 `schema_release_id_override`，然后清理 `DeviceRuntimeMetaCache`；复用 override 校验逻辑，失败时记录明确错误，不影响其他设备 OTA 状态处理 | E18c 测试通过 |
| E19 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/product_release_cleanup_test.go` | 先写失败测试：超时 `publishing` 转 `publish_failed`；过期 `publish_failed` 可软删 release 和 release-module；V1 不做 snapshot GC，软删 release-module 后引用数变为 0 的 orphan snapshot 也必须保留 | `go test ./rpc/things/internal/logic/moduleschemaservice -run ProductReleaseCleanup` |
| E20 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/product_release_cleanup.go` | 实现后台清理服务或内部 repo 方法，供定时任务调用；只处理 release/release-module，不删除 `dm_module_schema_snapshot` | E19 测试通过 |


### F. Cache L3 Loader
| ID | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| F1 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/cache/schemaBundleCache_snapshot_test.go` | 先写失败测试：L3 loader 不读取 `dm_product_release_schema`，从 release-module + snapshot 组装 bundle | `go test ./rpc/things/internal/cache -run Snapshot` 应先失败 |
| F2 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/cache/schemaBundleCache.go` | `loadProductReleaseSchemaBundle` 改为调用批量 snapshot repo 和 snapshot assembler | F1 测试通过 |
| F3 | 同测试文件 | 增加测试：release 10 个模块，snapshot repo 调用次数为 1，module kind 批量查询次数为 1 | `go test ./rpc/things/internal/cache -run NoNPlusOne` |
| F4 | 同实现文件 | 确认 L3 loader 复用 `caches.Cache` 自带的 `syncx.SingleFlight`（`sf.Do(keyStr, ...)`），不在 `schemaBundleCache.go` 额外引入 `singleflight.Group` | F3 测试通过；grep 确认 `schemaBundleCache.go` 未新增 `singleflight.Group` |
| F5 | 同测试文件 | 增加测试：发布新 release 不影响 current cache；set default 才淘汰 current | `go test ./rpc/things/internal/cache -run CurrentInvalidation` |
| F6 | 同实现文件 | 调整 `InvalidateProductSchemaBundles`：支持按 release 精确失效和按 current 精确失效，避免全产品全 release 扫描作为默认行为 | F5 测试通过 |


### G. RPC / Gateway 批量接口
| ID | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| G1 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/runtime_meta_batch_logic_test.go` | 补测试：`effectiveReleaseID = override ? override : current`；`Version` 不参与解析 | `go test ./rpc/things/internal/logic/moduleschemaservice -run RuntimeMetaBatch` |
| G2 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/runtime_meta_batch_logic.go` | 确认/修正 runtime meta 批量返回，单设备失败返回 item error，不影响其他设备 | G1 测试通过 |
| G3 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/moduleschemaservice/runtime_schema_batch_logic.go` 或当前实际文件 | 补/改测试：按 `{productID, releaseID, scope}` 批量返回 `SchemaBundle` JSON，单 release 失败返回 item error | `go test ./rpc/things/internal/logic/moduleschemaservice -run RuntimeSchemaBatch` |
| G4 | 同实现文件 | 接入 `SchemaBundleReleaseCache`，scope 默认为 `periodic_decode` | G3 测试通过 |
| G5 | `/home/wangjunbo/workspace-aux/iot/api/gateway/internal/logic/things/device/auth/runtime_meta_batch_logic.go` | 确认网关透传 RPC 返回结构；单项失败不让 HTTP 整体失败 | `cd /home/wangjunbo/workspace-aux/iot && go test ./api/gateway/internal/logic/things/device/auth` |
| G6 | `/home/wangjunbo/workspace-aux/iot/api/gateway/internal/logic/things/device/auth/runtime_schema_batch_logic.go` | 确认网关支持 `scope`，返回 `tsl` 字符串 | 同 G5 |


### H. TDengine 与写入路径
| ID | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| H1 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/moduleManage_test.go` | 补测试：只为 `ModuleKindPropertyTsdb` 模块建 STABLE，列名使用 identifier | `go test ./rpc/things/internal/tsDB/tdengine/schemaDataRepo -run Module` |
| H2 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/tsDB/tdengine/schemaDataRepo/schema.go`、`moduleManage.go` | 确保 `CreateModuleStable` 输入包含 product category 或按最新签名实现；只消费 property_tsdb 字段 | H1 测试通过 |
| H3 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/logic/tsDBHandle.go`、`thing.go` 相关写入路径 | 使用 `DeviceSchemaModuleCache.GetSchemaBundle(... periodic_decode)` 或等价路径拿 bundle，按 Route 分模块写入 | `go test ./rpc/things/internal/logic/...` 可跑的包通过 |
| H4 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/tsDB/tdengine/` | 保持 AsyncInsert 按 SQL bytes / 参数数拆批，不因模块大列数生成超限 SQL | `go test ./rpc/things/internal/tsDB/tdengine/...` |


### I. 旧接口兼容
| ID | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| I1 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/cache/schemaRepo.go` | 保留旧 `ProductSchemaRepo.GetData(productID) *schema.Model`，内部可转调 `GetCurrent(productID, full).Model` 或继续 legacy，需明确边界 | `go test ./rpc/things/internal/cache` |
| I2 | `/home/wangjunbo/workspace-aux/iot/rpc/things/internal/cache/schemaBundleCache.go` | 对外新增清晰方法：`GetCurrent(productID, scope)`、`GetByRelease(productID, releaseID, scope)`、`GetDevice(dev, scope)` | cache 单测通过 |
| I3 | 调用点：`iot/rpc/things/internal/event/handler/deviceMsgEvent/*.go`、`iot/rpc/things/internal/logic/devicemsgmanage/*.go`、`iot/rpc/things/internal/logic/deviceinteractmanage/*.go` | 热路径逐步从旧 `schema.Model` 改为 `SchemaBundle`；短期无法改的调用点只读 `.Model` | 相关包测试或至少 `go test ./rpc/things/internal/...` 编译通过 |


### J. ekuiper-cluster
| ID | 文件 | 预期改动 | 验证 |
| --- | --- | --- | --- |
| J1 | `/home/wangjunbo/workspace-aux/ekuiper-cluster/extensions/impl/cloudx/client/batch_client.go` | 确认 `ProductReleaseKey.String()` 默认 `periodic_decode`，key 包含 scope | `cd /home/wangjunbo/workspace-aux/ekuiper-cluster && go test ./extensions/impl/cloudx/client` |
| J2 | `/home/wangjunbo/workspace-aux/ekuiper-cluster/extensions/impl/cloudx/product/release_schema_cache.go` | 确认 `Fmt` 后进程内缓存是已解析 struct；不得在每包消息重复解析 `extendConfig/affordance` | `go test ./extensions/impl/cloudx/product` |
| J3 | `/home/wangjunbo/workspace-aux/ekuiper-cluster/extensions/impl/cloudx/ps_lookup.go` | 确认按 `(productID, effectiveReleaseID, scope)` 分组；两个 lookup 实例只差 scope | `go test ./extensions/impl/cloudx -run PsLookup` |
| J4 | `/home/wangjunbo/workspace-aux/ekuiper-cluster/extensions/functions/acxPushDecode/handle_push_data.go` | 保持单子设备 schema 缺失时 skip，不整包失败 | `go test ./extensions/functions/acxPushDecode/...` |
| J5 | ekuiper 规则配置文件所在目录 | 增加/更新两个 lookup 实例：`pslookup_periodic_decode`、`pslookup_active_read` | 启动 ekuiper 本地规则，确认两个 source 初始化成功 |


### K. 数据回填与发布演练
| ID | 文件/命令 | 预期动作 | 验证 |
| --- | --- | --- | --- |
| K1 | `/home/wangjunbo/workspace-aux/doc/module-level-supertable/migration.sql` | 在测试库执行迁移，确认表和索引存在 | `psql ... -f doc/module-level-supertable/migration.sql`，再 `\d dm_module_schema_snapshot`、`\d dm_product_release_module` |
| K2 | 数据导入脚本目录，例如 `/home/wangjunbo/workspace-aux/dataImport/module/` | 导入 10 个模块：`aux_inverter_basic`、`aux_inverter_event`、`aux_asg_rt`、`aux_asn_rt`、`aux_asg_afci`、`aux_asg_energy`、`aux_asn_energy`、`aux_inverter_no_store`、`aux_asg_bms_event`、`aux_col_rt` | SQL 查询 `dm_module_info` 和 `dm_module_schema_info` 数量 |
| K3 | 管理端或临时发布脚本 | ASN 产品发布：模块 `1,2,4,7,8`；ASG 产品发布：模块 `1,2,3,5,6,8,9` | 查询 `dm_product_release_module` 模块集合与设计一致 |
| K4 | 同上 | 修改一个共享模块，比如 `module_id=1` 新增点位，只生成一个新 module snapshot，多产品 release 复用 | 查询相同 `module_id + schema_hash` 只有一行 snapshot |
| K5 | 同上 | 发布新 release 后不切 current；创建带 `target_schema_release_id` 的 OTA job；模拟 OTA 成功回调写设备 `version + schema_release_id_override` 并清理 runtime meta cache | 未 override 设备仍命中旧 current；OTA 成功设备下次请求重新加载 DeviceRuntimeMeta，并通过 release cache 命中新 release；旧 current/release SchemaBundle cache 均未被无关淘汰 |


---

## 5. 验证矩阵
### iot 最小验证
```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./pkg/domain/schema
go test ./rpc/things/internal/relationdb/model
go test ./rpc/things/internal/relationdb/repo
go test ./rpc/things/internal/cache
go test ./rpc/things/internal/logic/moduleschemaservice
go test ./api/gateway/internal/logic/things/device/auth
```

### iot 扩展验证
```bash
cd /home/wangjunbo/workspace-aux/iot
go test ./rpc/things/internal/tsDB/tdengine/...
go test ./rpc/things/internal/logic/...
go test ./...
```

如果 `go test ./...` 因历史无关包失败，必须记录失败包和错误，不得声称全量通过。

### ekuiper-cluster 验证
```bash
cd /home/wangjunbo/workspace-aux/ekuiper-cluster
go test ./extensions/impl/cloudx/...
go test ./extensions/functions/acxPushDecode/...
go test ./...
```

### 手工集成验证
必须覆盖：

+ ASN 产品发布后 `dm_product_release_module` 只有 `1,2,4,7,8`。
+ ASG 产品发布后 `dm_product_release_module` 只有 `1,2,3,5,6,8,9`。
+ `dm_module_schema_snapshot.schema_json` 中 `extendConfig/affordance` 是 object，不是 escaped string。
+ `ProductReleasePublish` 后 `current_release_id` 不变。
+ `ProductReleaseSetDefault` 后 `current_release_id` 变化，current cache 失效。
+ OTA 成功后写 `schema_release_id_override`，该设备命中新 release。
+ `runtime-schema-batch` 对一个失败 release 返回单项 error，不影响其他 release。
+ `acxPushDecode` 对单个子设备 schema 缺失 skip，其他子设备继续解码。

---

## 6. 风险与控制
| 风险 | 控制 |
| --- | --- |
| L3 loader 变成 N+1 查询 | 单测注入 mock repo 统计调用次数；必须批量 `IN` 或 JOIN |
| JSON-in-JSON 导致热路径 CPU 放大 | 快照 `schemaVersion=2` 强制 object；发布时解析；L1 缓存 Go struct |
| 发布新 release 误切 current | `ProductReleasePublish` 测试断言 current 不变；只有 `SetDefault` 可改 |
| TDengine 失败后 SQL 侧半成品污染运行时 | `GetRelease/SetDefault/Override` 只允许 `published`；失败 release 保留证据但不可运行时引用 |
| `publishing` 状态因进程崩溃卡住 | 后台清理把超时 publishing 标记为 `publish_failed`，记录 `publish_error` |
| orphan snapshot 行数增长 | V1 不做 snapshot GC；预估行数可控。后续如有需要，单独实现“无未软删 release-module 引用 + 超过保留期”的可选 GC |
| 分类绑定变化污染旧 release | 旧 release 只读 `dm_product_release_module`；测试覆盖发布后改分类不影响旧 release |
| 分类缺失导致发布出“半产品模型” | `category_id=0`、分类不存在、分类链路软删一律发布失败，不退化为只读产品模块 |
| 分类和产品重复绑定同一模块导致覆盖误解 | 按 `module_id` 去重；同一 `module_id` 没有产品/分类优先级 |
| `module_code` 冗余被误当真相 | 文档和代码注释标明只读冗余；类型判断只用 `ModuleKind` |
| 设备 override 指向跨产品 release | override update 测试必须拒绝跨产品、未 published release |
| `DmProductReleaseSchema` 旧路径继续被运行时读取 | `schemaBundleCache` 单测禁止查旧 repo；运行时组装入口只走 snapshot |


---

## 7. 建议里程碑
### M1：底层模型可用
完成 A、B、C、D。

验收：

+ 能从 `dm_module_schema_info` 生成 `dm_module_schema_snapshot`。
+ 能从 `dm_product_release_module + snapshot + module_kind` 组装 `SchemaBundle`。

### M2：发布链路可用
完成 E、F。

验收：

+ `ProductReleasePublish` 生成 release module 和 snapshot，不切 current。
+ `ProductReleaseSetDefault` 可显式切 current。
+ cache L3 从 snapshot 回填，无 N+1。

### M3：运行时接口可用
完成 G、I。

验收：

+ `runtime-meta-batch` 和 `runtime-schema-batch` 支持 override/current 解析。
+ 旧 `GetData()` 调用不被一次性打断。

### M4：规则引擎兼容
完成 J。

验收：

+ `pslookup_periodic_decode` 和 `pslookup_active_read` 两个实例可同时工作。
+ 子设备单项失败 skip。

### M5：灰度发布演练
完成 K。

验收：

+ ASN / ASG 产品可按模块集合发布。
+ 新 release 不自动影响未升级设备。
+ OTA 成功设备可通过 override 命中新 schema。

# `  
`
