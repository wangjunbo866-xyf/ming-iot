---
document_kind: plan
status: approved
delivery_issue: ming-iot#IKDFC5
code_repository: git:gitee.com/ming-industry/ming-iot
related_documents:
  - specs/2026-09-05-ota-model-module-code-disambiguation-activation-design.md
  - proposals/2026-09-04-ota-model-module-code-disambiguation.md
  - reviews/2026-09-04-ota-model-module-code-disambiguation-review.md
---

# OTA 固件模块编码与物模型模块编码消歧实施计划

> **执行说明：** 实施者按任务顺序执行，行为变更采用测试先行。Plan Task 是一个仓库级 Agent Todo 内部的工程检查点，不逐项创建 LoopX Todo 或 User Gate。本文状态为 `approved` 只表示计划可作为权威实施依据；User Gate `todo_d191f4c30c23` 未授权执行产品代码修改。

**目标：** 在首次发布前一次性消除 OTA 固件模块编码与产品物模型模块编码共用 `ModuleCode` / `moduleCode` / `module_code` 的歧义，修复 HTTP 层的 `int64` 类型错误，并保持业务行为、Proto 字段编号和生成流程稳定。

**架构：** 分别沿 OTA/设备版本域和产品物模型/Model Release 域完成源定义改名；Proto 与 HTTP 生成物只通过项目锁定 wrapper 重建；PostgreSQL 使用明确的新列名；SchemaBundle 接受一次可解释、可复现的 JSON 与哈希位移。仓库合同先锁定预期差异，再逐域转绿，最后由独立 Reviewer 绑定候选精确 Head 验收。

**技术栈：** Go 1.24、go-zero API/Proto、GORM/PostgreSQL、项目锁定 `rpcgen` 与 `apicheck`、Go `testing`、GNU Make、LoopX Evidence/Review。

**设计依据：** `specs/2026-09-05-ota-model-module-code-disambiguation-activation-design.md`

**Delivery Issue：** [ming-iot#IKDFC5](https://gitee.com/ming-industry/ming-iot/issues/IKDFC5)

## 执行拓扑与授权边界

产品实施只建立一个有界 Agent Todo：

- `task_repository`: `git:gitee.com/ming-industry/ming-iot`
- 建议分支：`feature/ota-model-module-code-disambiguation`
- 写范围：`backend/things/**`、`backend/tools/internal/repositorycontract/**`
- 实现 Agent：不得使用 `ming-review-claude`
- 独立 Review Agent：`ming-review-claude` 或另一个未参与实现的 Reviewer

实施开始前重新拉取 `origin/master`。若不再是规格核查基线 `a6b28035c577131c1a4b0262af6ddbdc393104c5`，先执行 Task 1 的消费者扫描并把新的精确 Head 写入 Evidence；只有相关契约面没有冲突时才能继续。本文不授权实现、合并、发布、生产数据库操作或历史重写。

## Task 1：锁定实现基线并建立 RED 合同

**文件：**

- 新建：`backend/tools/internal/repositorycontract/module_code_disambiguation_contract_test.go`
- 核查：`backend/things/service/dmsvr/proto/deviceManage.proto`
- 核查：`backend/things/service/dmsvr/proto/otaManage.proto`
- 核查：`backend/things/service/dmsvr/proto/productManage.proto`
- 核查：`backend/things/service/apisvr/http/things/device/moduleVersion.api`
- 核查：`backend/things/service/apisvr/http/things/ota/firmware.api`
- 核查：`backend/things/service/apisvr/http/things/product/modelModule.api`
- 核查：`backend/things/service/dmsvr/internal/repo/relationDB/model.go`
- 核查：`backend/things/service/dmsvr/internal/repo/relationDB/modelOta.go`
- 核查：`backend/things/service/dmsvr/internal/repo/relationDB/modelReleaseModel.go`

- [ ] **Step 1：记录精确实现基线和消费者清单**

```bash
git fetch origin master
git rev-parse origin/master
git grep -n -E 'ModuleCode|moduleCode|module_code' origin/master -- \
  backend/things backend/tools
```

将精确 Head、命中总数及按 OTA/Model 两域分类后的文件清单保存到本次私有原始 Evidence。若发现第三个业务领域或已发布兼容事实，停止实施并修订规格。

- [ ] **Step 2：先写会失败的仓库合同**

测试至少断言：

1. OTA 源定义存在 `FirmwareModuleCode` / `firmwareModuleCode` / `firmware_module_code`，字段类型为 `string`；
2. Model 源定义存在 `ModelModuleCode` / `modelModuleCode` / `model_module_code`，字段类型为 `string`；
3. `deviceManage.proto` 三处目标字段编号仍为 4，`otaManage.proto` 两处仍为 16 和 14；
4. `productManage.proto` 的四处 Model Module 字段保留原编号；
5. 纳入清单的源定义中不再出现无修饰旧字段；
6. 没有 OTA 编码到 `ModelModuleID` 的映射或跨域通用 `ModuleCode` 类型。

运行：

```bash
go -C backend/tools test ./internal/repositorycontract \
  -run 'TestModuleCodeDisambiguation' -count=1
```

预期：因当前旧字段名和 `moduleVersion.api` 的 `int64` 类型而失败；失败只描述本 Delivery 的预期差异。

## Task 2：收敛 OTA 与设备版本域

**文件：**

- 修改：`backend/things/service/dmsvr/internal/repo/relationDB/model.go`
- 修改：`backend/things/service/dmsvr/internal/repo/relationDB/deviceModuleVersion.go`
- 修改：`backend/things/service/dmsvr/internal/repo/relationDB/modelOta.go`
- 修改：`backend/things/service/dmsvr/proto/deviceManage.proto`
- 修改：`backend/things/service/dmsvr/proto/otaManage.proto`
- 修改：`backend/things/service/apisvr/http/things/device/moduleVersion.api`
- 修改：`backend/things/service/apisvr/http/things/ota/firmware.api`
- 修改：`backend/things/share/domain/deviceMsg/msgOta/ota.go`
- 修改：`backend/things/service/dmsvr/internal/logic/devicemanage/**`
- 修改：`backend/things/service/dmsvr/internal/logic/otamanage/**`
- 修改：相关 HTTP adapter 与测试夹具

- [ ] **Step 1：改源定义，不手改生成物**

统一使用：

- Go：`FirmwareModuleCode string`
- Proto / JSON：`firmwareModuleCode`
- PostgreSQL：`firmware_module_code`
- 默认常量：`FirmwareModuleCodeDefault`

把 `moduleVersion.api` 的错误 `int64` 同时改为 `string`。保持 Proto 字段编号、现有表名、索引业务字段组合和 OTA 行为不变。

- [ ] **Step 2：更新调用者与聚焦测试**

更新仓储过滤器、查询条件、逻辑层转换、HTTP adapter、错误文本和测试夹具。先让 OTA 子测试转绿，Model 子测试仍可保持 RED：

```bash
go -C backend/tools test ./internal/repositorycontract \
  -run 'TestModuleCodeDisambiguation/OTA' -count=1
go -C backend/things test \
  ./service/dmsvr/internal/repo/relationDB/... \
  ./service/dmsvr/internal/logic/devicemanage/... \
  ./service/dmsvr/internal/logic/otamanage/... \
  -count=1
```

预期：OTA 新命名、字符串类型和数据库列合同通过；不引入旧字段别名或双写。

## Task 3：收敛产品物模型与 Model Release 域

**文件：**

- 修改：`backend/things/service/dmsvr/internal/repo/relationDB/modelReleaseModel.go`
- 修改：`backend/things/service/dmsvr/internal/repo/relationDB/modelModule.go`
- 修改：`backend/things/service/dmsvr/internal/repo/relationDB/productDraftComposer.go`
- 修改：`backend/things/service/dmsvr/internal/repo/relationDB/productInfo.go`
- 修改：`backend/things/service/dmsvr/internal/repo/relationDB/productModelModule.go`
- 修改：`backend/things/service/dmsvr/proto/productManage.proto`
- 修改：`backend/things/service/apisvr/http/things/product/modelModule.api`
- 修改：`backend/things/service/dmsvr/internal/logic/productmanage/**`
- 修改：相关 HTTP adapter、领域服务与测试夹具

- [ ] **Step 1：改源定义和仓储合同**

统一使用：

- Go：`ModelModuleCode string`
- Proto / JSON：`modelModuleCode`
- PostgreSQL：`model_module_code`

同步更新 GORM tag、过滤器、显式 SQL、更新 map、索引测试、错误文本和局部变量。`ModelModuleID` 继续作为稳定身份，`base_` 业务前缀和基础模块编码值不变；带领域含义的 helper 可保留 `BaseModelModuleCode` 名称。

- [ ] **Step 2：让 Model 合同和聚焦测试转绿**

```bash
go -C backend/tools test ./internal/repositorycontract \
  -run 'TestModuleCodeDisambiguation/Model' -count=1
go -C backend/things test \
  ./service/dmsvr/internal/domain/modelrelease/... \
  ./service/dmsvr/internal/repo/relationDB/... \
  ./service/dmsvr/internal/logic/productmanage/... \
  -count=1
```

预期：Model 新命名和数据库列合同通过；现有模块 ID、租户隔离、绑定和发布行为不变。

## Task 4：更新 SchemaBundle 公开快照与确定性哈希

**文件：**

- 修改：`backend/things/share/domain/schema/bundle.go`
- 修改：`backend/things/share/domain/schema/bundle_test.go`
- 修改：`backend/things/share/domain/schema/testdata/schema_bundle_v1.golden.json`
- 修改：`backend/things/share/domain/schema/testdata/schema_bundle_v1.sha256`

- [ ] **Step 1：先把 JSON 合同改为新字段并观察 RED**

测试明确要求 canonical JSON 只包含 `modelModuleCode`，拒绝 `moduleCode`；保留 Property Route 按 identifier 排序、模块按 `ModuleID` 后 `ModelModuleCode` 排序、TSL `version:"1.0"` 以及事件/动作内容不变。

```bash
go -C backend/things test ./share/domain/schema \
  -run 'TestCompileSchemaBundle|TestSingleModuleFlatMode' -count=1
```

预期：旧 JSON 字段或旧 golden 导致失败。

- [ ] **Step 2：实现改名并更新唯一预期位移**

将 `PropertyRoute`、`ModuleDraft` 及编译内部变量改为 `ModelModuleCode` / `modelModuleCode`，更新 golden 和 SHA-256。Evidence 记录旧哈希、新哈希，并证明 canonical payload 的语义差异只来自字段名。

再次运行同一命令，预期全部通过且重复执行哈希不变。

## Task 5：重建生成物并验证 PostgreSQL 空库合同

**文件：**

- 由 `rpcgen` 更新：`backend/things/service/dmsvr/pb/**`、`backend/things/service/dmsvr/client/**`、`backend/things/service/dmsvr/internal/server/**`、`backend/things/service/dmsvr/dmdirect/**`
- 由 `apicheck` 更新：`backend/things/service/apisvr/internal/**`、Swagger 与 API golden 等受管生成物
- 修改：`backend/things/service/dmsvr/internal/repo/relationDB/modelReleaseRepo_integration_test.go`
- 修改：必要的 OTA PostgreSQL 聚焦集成测试

- [ ] **Step 1：只使用锁定 wrapper 生成**

```bash
make protobuf-tools

go -C backend/tools run ./cmd/rpcgen generate \
  --manifest ../things/service/dmsvr/proto/rpcgen.yaml \
  --module-root ../things \
  --protobuf-tools ../bin/protobuf-tools

go -C backend/tools run ./cmd/apicheck generate \
  --backend .. \
  --goctl ../bin/goctl
```

不得手工修补 PB、client、server、handler、types、route、Swagger 或 golden。

- [ ] **Step 2：验证生成幂等**

```bash
go -C backend/tools run ./cmd/rpcgen check \
  --manifest ../things/service/dmsvr/proto/rpcgen.yaml \
  --module-root ../things
go -C backend/tools run ./cmd/apicheck verify --backend ..

generation_before="$(git diff --binary | sha256sum)"
go -C backend/tools run ./cmd/rpcgen generate \
  --manifest ../things/service/dmsvr/proto/rpcgen.yaml \
  --module-root ../things \
  --protobuf-tools ../bin/protobuf-tools
go -C backend/tools run ./cmd/apicheck generate \
  --backend .. \
  --goctl ../bin/goctl
generation_after="$(git diff --binary | sha256sum)"
test "$generation_before" = "$generation_after"
```

前后 diff 哈希必须一致，证明第二次生成没有新增差异。

- [ ] **Step 3：验证空 PostgreSQL schema**

在隔离的开发测试数据库中运行：

```bash
MING_IOT_TEST_POSTGRES_DSN='<redacted-test-dsn>' \
  go -C backend/things test -tags=integration \
  ./service/dmsvr/internal/repo/relationDB \
  -run 'TestModelReleasePostgresContract|TestModuleCodePostgresContract' \
  -count=1
```

测试确认三张表只使用 `firmware_module_code` 或 `model_module_code`，类型为 varchar，业务唯一约束仍覆盖原字段组合。只允许重建隔离开发测试库；不得操作生产库或共享数据卷。

## Task 6：完成全量静态验收与旧名归零

**文件：**

- 修改：`backend/tools/internal/repositorycontract/module_code_disambiguation_contract_test.go`
- 核查：本 Delivery 的全部产品改动和生成物

- [ ] **Step 1：执行限定旧名扫描**

```bash
git grep -n -E 'ModuleCode|moduleCode|module_code' -- \
  backend/things backend/tools
```

逐条分类剩余命中。允许带完整领域限定的 `FirmwareModuleCode`、`ModelModuleCode` 及明确不属于本 Delivery 的概念；纳入规格范围的无修饰旧字段、JSON key、数据库列和跨域通用类型必须归零。

- [ ] **Step 2：运行项目统一 Gate**

```bash
make verify-fast VERIFY_SCOPE=things
make verify-pr
git diff --check
```

预期：全部通过。本次不默认运行 Kafka 或完整 Things 启动基线；若出现与本 Delivery 无关的既有失败，记录精确命令、失败摘要和独立 Issue，不得删除测试或缩小合同伪装通过。

## Task 7：发布 Evidence、独立 Review 与 Draft PR

**文件（研发文档仓的独立后续文档 Todo）：**

- 新建：`evidence/2026-09-05-ota-model-module-code-disambiguation/evidence.md`
- 新建：`reviews/2026-09-05-ota-model-module-code-disambiguation-implementation-review.md`
- 修改：`meta/DOC_REGISTRY.yaml`
- 修改：`proposals/2026-09-04-ota-model-module-code-disambiguation.md`

- [ ] **Step 1：固定产品候选精确 Head 和机器 Evidence**

实现 Agent 完成验证后提交产品候选分支，记录完整 40 位 Head。原始命令输出、哈希和机器 Evidence 留在 LoopX 私有 Evidence 目录；研发文档摘要只保留：

- 实现基线与候选精确 Head；
- 新旧 SchemaBundle 哈希；
- Gate verdict 与稳定 Evidence 指针；
- blocker/warning 数量；
- 与产品 Draft PR、研发文档 Draft PR 和 `ming-iot#IKDFC5` 的链接。

- [ ] **Step 2：独立 Review 精确候选**

把只读候选交给未参与实现的 Reviewer。Review 必须逐项核查：

1. 两域字段映射和类型；
2. Proto 字段编号；
3. PostgreSQL 列与索引；
4. 生成物来源和幂等性；
5. SchemaBundle 新旧哈希及唯一差异；
6. 旧名归零与无跨域映射；
7. `verify-fast`、`verify-pr` 和 PostgreSQL 聚焦合同证据。

发现 blocker 时回到对应 Task 修复并生成新候选 Head；旧 Review 不得沿用到新 Head。

- [ ] **Step 3：创建或更新双仓 Draft PR**

产品仓 Draft PR 承载代码；研发文档仓 Draft PR #3 承载 Proposal、Spec、Plan、Evidence 和 Review。两者互相引用 Delivery Issue 与对方 URL。Draft PR 创建/更新使用各仓批准的自动化入口，正文必须明确“Draft 不等于批准合并”。

只有全部验收通过、独立 Review 绑定最终 exact Head 且 Owner/维护者完成最终阅读后，才进入人工 Gitee 合并决策；本计划不授权任何 PR 合并、发布或生产变更。
