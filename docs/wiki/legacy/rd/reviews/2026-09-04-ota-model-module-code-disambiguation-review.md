---
document_kind: review
status: pass
delivery_issue: ming-iot#IKDFC5
code_repository: git:gitee.com/ming-industry/ming-iot
exact_head: 278a11f985bfa88410f976d609923d24ddc5d8fa
related_documents:
  - proposals/2026-09-04-ota-model-module-code-disambiguation.md
  - specs/2026-09-05-ota-model-module-code-disambiguation-activation-design.md
  - plans/2026-09-05-ota-model-module-code-disambiguation.md
---

# OTA 固件模块编码与物模型模块编码消歧提案（方案 C）— 独立评审

**评审人：** ming-review-claude（独立评审 lane；非提案作者）
**日期：** 2026-09-04
**评审对象：** `proposals/2026-09-04-ota-model-module-code-disambiguation.md` @ exact head `278a11f985bfa88410f976d609923d24ddc5d8fa`
**核查基线：** 产品仓 `origin/master@a6b28035c577131c1a4b0262af6ddbdc393104c5`（与提案 §3 声明一致）
**评审方式：** 通读提案 180 行；对 §3 全部代码事实断言在 a6b2803 独立复核；全仓 `module_code|moduleCode|ModuleCode` 扫描核对 §7 覆盖面。未修改 Proposal 或产品代码，零 git 写操作（本评审文件除外）。

> 追踪说明（2026-09-05）：正式 Delivery Issue `ming-iot#IKDFC5` 已建立。本次仅修复稳定交付身份；原评审对象、精确 Head 与 PASS 结论不变。本文件不是对后续实施规格或产品代码的 Review。

> 激活说明（2026-09-05）：Owner 已通过 `todo_d191f4c30c23` 批准后续实施规格，并授权形成实施计划。新增关联只用于恢复交付链；本评审的 `exact_head`、评审对象和 PASS 结论仍只绑定原 Proposal。

## 一、前提（§2）核验

五项前提属 Owner 事实声明（无生产库/无已发布客户端/无固件依赖/无第三方调用方/全新项目），仓库证据与之相容：CI 于 !3 方才建立且无发布流水线（W1 跟踪项）、things-baseline 为开发环境、历史评审一贯"不迁移生产数据"姿态。前提不可由仓库独立证伪，但 §2 附带了正确的失效保险——任一前提变化即暂停重估。**成立（带Owner声明属性标注）。**

## 二、字段映射（§1/§4）核验

- Go/Proto-JSON/PG 三层命名映射（`FirmwareModuleCode`/`firmwareModuleCode`/`firmware_module_code` 与 `ModelModuleCode`/`modelModuleCode`/`model_module_code`）自洽且符合各层惯例。
- §3.1/§3.2 事实全部复核属实：`dm_device_module_version.module_code`（product_id+device_name+version 唯一索引，注释"固件名称"）✓；`deviceManage.proto` 三处 `string moduleCode = 4` ✓；`otaManage.proto` 两处 ✓；`dm_module_info.module_code` 租户内唯一、`ModelModuleID` 稳定身份、Bundle 路由字段语义 ✓。
- §4.3 禁止关系（不跨域查询/不建通用类型/文档禁用无修饰"模块编码"）闭合了 W3 的原始混淆源。**成立。**

## 三、类型收敛（§3.3）核验

`moduleVersion.api:41/48/59` 确为 `ModuleCode int64`，与 Proto/DB 的 `string` 不一致——**真实类型缺陷，复核确认**。将其并入同一收敛（`string firmwareModuleCode`）并要求实施规格以生成结果+测试证明三层一致，处置正确。**成立。**

## 四、兼容性取舍（§5/§6）核验

A（仅标注）/B（别名双写）被拒的理由与项目自身"禁止双写 Property 模型"教义一致；C 的代价（开发面一次性同步+重建开发库）与其收益（消除永久认知债务）在前述前提下成立。字段编号"尽量不变但不承诺兼容"的表述诚实。无别名/无双写/无弃用窗口与 §8.3 验收（旧名归零）互为闭环。**成立。**

## 五、实施边界（§7/§10）核验与补充

激活条件（Delivery Issue → 用户批准**实施规格**而非本提案 → 有界单 Todo 多 Task → 独立评审）符合项目流程且防御完备。§7 文件清单标注"至少核查"，**复审者扫描发现以下同域表面须在实施规格中一并枚举**（不构成 blocker，属清单补全）：

1. `relationDB/modelOta.go`（含 1 处 `module_code`）与 `relationDB/otaModuleInfo.go:172` 的无修饰常量 `ModuleCodeDefault = "default"`（按 §4.3-4 应同步更名）；
2. `things/share/domain/deviceMsg/msgOta/ota.go`（设备消息域 OTA 编码表面）；
3. 测试夹具：`productPropertyDraft_test.go`、`productModelModule_migration_test.go`、`modelReleaseRepo_integration_test.go` 等；
4. **物模型侧改名会改变 `SchemaBundle.propertyRoutes` 的 JSON 字段名**（`moduleCode` → `modelModuleCode`）——确定性 hash 与 golden 向量将发生一次性合法位移，实施规格应显式记录新旧 hash 并更新 golden（属预期变更，非漂移）。

## 结论

**verdict = PASS**（0 blocker / 0 warning / 4 补全 note）

方案 C 的前提链、三层字段映射、类型纠偏、取舍论证与实施边界全部经独立复核成立；W3 自两轮延期后获得完整处置。提案仅授权文档层，产品代码实施须按 §10 激活条件另行立项（Delivery Issue + 用户批准实施规格 + 独立评审）。建议实施规格采纳本评审第五节的四处表面补全。

---

**English verdict:** PASS. Scheme C's premises, three-tier field mapping, the (verified) int64/string type defect in moduleVersion.api, the compatibility trade-off rationale, and the activation boundaries all hold under independent re-verification against origin/master@a6b2803. Four coverage notes (modelOta.go, the ModuleCodeDefault constant, the device-message OTA surface, test fixtures, and the expected one-time SchemaBundle golden/hash shift) should be folded into the implementation spec. W3 is fully dispositioned; product implementation remains gated per §10.
