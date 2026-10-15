---
document_kind: review
status: pass
delivery_issue: ming-iot#IKDFC5
code_repository: git:gitee.com/ming-industry/ming-iot
exact_head: e38d162feba9445c3bedb6ec78d96ee79dee9a49
related_documents:
  - specs/2026-09-05-ota-model-module-code-disambiguation-activation-design.md
  - plans/2026-09-05-ota-model-module-code-disambiguation.md
---

# OTA/物模型模块编码消歧实施（V1）— 独立评审

**评审人：** ming-review-claude（独立评审 lane，非实现 Agent）
**日期：** 2026-09-05
**候选：** ming-iot `e38d162feba9445c3bedb6ec78d96ee79dee9a49`（base `a6b28035c577131c1a4b0262af6ddbdc393104c5`，单提交 "refactor(things): disambiguate module codes"，38 文件 +7,021/−4,050）
**依据：** rd-docs `3590e771` 的获批 Spec（SHA-256 `7d2275d3…`）与 Plan（`13679527…`），哈希复算一致
**评审方式：** 独立 worktree（ming-iot `.worktrees/review-ota-v1` detached @e38d162）通读全部非生成差异 + 抽验生成物；独立复跑规格声明的机器门禁。未修改产品实现，未 commit/merge/push（本评审/证据/登记除外）。

## 一、评审范围与锚定

38 文件中非生成面 24 个（api 3、proto 2、types/handlers/logic、relationDB 12、schema bundle 域、契约测试新增 151 行）+ 生成面（pb/*.pb.go、swagger、types.golden.json +42）。核验维度按 handoff：命名边界、Proto 字段号、PG 契约、SchemaBundle 哈希、Swagger 追平。

## 二、独立复跑证据（exact head e38d162）

| 门禁 | 结果 |
|---|---|
| `make verify-pr`（全链 build/test/vet/contract-static/boundary） | **PASS exit 0**（61 ok 包行；Kafka contract digest `cc117b59…` 完好；apicheck 244/242/242 detached=2；2×rpcgen OK） |
| `make verify-fast VERIFY_SCOPE=things` | PASS exit 0 |
| `TestModuleCodeDisambiguation`（新增源契约扫描/类型/字段号/PG/Bundle/边界/生成 七合同）+ 其余四契约测试 | **全 PASS** |
| 旧无修饰名归零扫描（评审者独立正则，排除限定名/测试/pb） | **0 命中** |

## 三、专项核验（handoff 五项）

1. **命名边界**：OTA 域全量 `FirmwareModuleCode`/`firmwareModuleCode`/`firmware_module_code`（deviceModuleVersion.go、modelOta.go、otaManage/deviceManage proto、firmware.api、ota logic）；物模型域 `ModelModuleCode`/`modelModuleCode`/`model_module_code`（modelModule.go、bundle.go、productManage、modelModule.api）。**跨域身份映射零新增**（ModelModuleID 仍为唯一稳定身份）。✅
2. **Proto 字段号**：deviceManage 三处 `= 4`、otaManage `= 16`/`= 14` **全部保持**，仅更名；注释同步为"固件模块编码"。✅（验收 #5）
3. **PostgreSQL 契约**：`dm_device_module_version.firmware_module_code`（唯一索引结构与基线一致）、物模型侧 `model_module_code`；`moduleVersion.api` 三处 `ModuleCode int64` → `FirmwareModuleCode string` —— **类型缺陷同步收敛**（HTTP/Proto/Go/PG 四层 string）。✅（验收 #1/2/4）
4. **SchemaBundle 哈希**：golden+sha256 一次性确定位移 `936e8a3d…` → `e0604e40…`，与 Spec §8 声明完全一致（propertyRoutes JSON 键 `moduleCode`→`modelModuleCode`，排序规则不变，schemaVersion/TSL 语义不变）；本位移即提案评审预判的预期变更，非漂移。✅（验收 #9）
5. **Swagger/生成物追平**：types.golden.json、pb/grpc、swagger 随源再生成；apicheck verify（244 定义全注册）与 2×rpcgen check 在 verify-pr 内通过，生成无意外漂移。✅（验收 #8）

## 四、发现

**0 blocker / 0 warning / 2 note：**

- **N1** 验收 #7（空开发库按新模型建表）与 PG 集成级测试未在本评审环境重跑（需受控 DSN）；依据为实现侧契约测试（已过）+ Spec §6 明示不依赖 AutoMigrate 猜测、开发库重建路径。实施 Evidence 应已记录建表结果，合并前可复核。
- **N2** `otaModuleInfo.go` 的 `ModuleCodeDefault` 常量按提案评审建议一并消歧（实现面已见 otaModuleInfoCreateLogic 改动）——确认提案评审 4 条补全 note 均被采纳（modelOta.go 56 行、msgOta 面、测试夹具、golden 位移记录）。

## 五、结论与交接

**verdict = PASS**（0 blocker / 0 warning / 2 note）

Spec §11 十一条验收：#1–#6、#8–#10 由本评审独立验证；#7 依契约测试+实施证据采信（见 N1）。四方案 C 目标（领域词汇唯一、旧名归零、类型一致、无兼容层）全部达成。候选可进入 Draft PR（引用 ming-iot#IKDFC5 与本 Head）；按流程合并须走 verify-pr 门禁与用户 Gate。配对脱敏 Evidence：`evidence/2026-09-05-ota-model-module-code-disambiguation/evidence.md`。

---

**English verdict:** PASS. At exact head `e38d162` the Scheme-C disambiguation is fully realized: unqualified old names are zero across active sources, Proto field numbers are preserved (4/16/14), the API/Proto/Go/PG field is consistently `string`, the SchemaBundle golden/hash shifted exactly once as declared (`936e8a3d…`→`e0604e40…`), and generated artifacts are caught up (apicheck/rpcgen green). `make verify-pr` (exit 0, 61 packages) and `verify-fast VERIFY_SCOPE=things` were independently re-run in a fresh worktree. Two non-blocking notes: the empty-DB create-table acceptance rests on contract tests plus implementer evidence (DSN-gated integration not re-run here), and all four proposal-review coverage notes were adopted. Merge-ready pending PR gate and user approval.
