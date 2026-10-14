---
status: inventory-complete-design-candidate
date: 2026-10-15
goal: ming-iot-knowledge-pilot
---

# 单仓迁移 M1：全量盘点与目录设计

当前依据：[全项目单仓迁移方案](../architecture/monorepo-migration.md)。本次完成源清单与目录设计，尚未备份、导入源码、推送或删除旧仓。

## 盘点方法与边界

对全部顶层项目读取 HEAD、refs、Git 跟踪树、工作树、状态、模块文件及来源声明；计数来自本机快照，未 fetch，不代表所有远程最新分支。私有机器清单保存在本机迁移 evidence 中，公开文档仅保留必要汇总；内容检查不输出凭据值。

采集时间：`2026-10-14T16:55:04.039315+00:00`。完整机器清单 SHA-256：`48ded2fda09fe41c8ac9571c15bcf39f4bea91f79f9f571b1e122c86531d1bc7`。该摘要绑定盘点清单，不是源码备份摘要。

## 九个 Git 来源

| 来源 | HEAD | 本地 refs | HEAD 文件数 | HEAD 文件大小 MiB | 未提交状态条数 |
| --- | --- | ---: | ---: | ---: | ---: |
| AirMQ | `6e42efaf4f9215acedbac6cb138f2dc34c59a7c4` | 3 | 172 | 1.88 | 0 |
| ai-native | `777d68e54942b1f6cf661e9c3ed7887050c22b61` | 9 | 22 | 0.12 | 81（含本轮清单 1 条） |
| core | `9c027986e1bbcd86d33a5d9f5c2146f52cba247c` | 69 | 1188 | 20.08 | 0 |
| iot-doc | `287d41a7ca2f5d444ceb34264fbe1ac515c42836` | 3 | 129 | 5.96 | 6 |
| ming-iot | `6742201e9666fd3b3067216b2efbb3b8ba7338d7` | 28 | 2764 | 27.77 | 0 |
| ming-iot-rd-docs | `b16030c10cf2565ecfa16ab19ee43a3a273bec57` | 27 | 155 | 22.66 | 1 |
| share | `ece02ca9d6defd63356931a59e653950f27ac572` | 228 | 255 | 0.78 | 0 |
| things | `0fcfcfde8f3a66a774ad4443706274ea8e952b59` | 219 | 1207 | 112.07 | 0 |
| thingsboard | `0439df09f31b0e8101b9f0a6be556c087d86818d` | 216 | 13865 | 165.23 | 0 |

refs 包含本地分支、远程跟踪分支和标签，数量不代表独立业务分支。文件数和大小只统计各源 HEAD，不含 Git 历史、额外候选内容和未提交文件。九个 HEAD 均未发现 gitlink。

## 六个额外工作树与候选

| 所属仓 | 工作树/分支 | HEAD | 采集时状态 |
| --- | --- | --- | --- |
| ai-native | coordination-20261009 | `bf75f133b79e84f45061075a09bbca419e2356d6` | clean |
| ai-native | memory-20261009 | `3c6b1740f63cb99a34e3f8788eb04b0dda2cb0ae` | clean |
| ai-native | openspec-context-20261010 | `f7ef7f44809883beec858bdd9eab937a236a78f3` | clean |
| ai-native | practices-20261009 | `884b563d85d3a2a115f927f7e2921af446b05601` | clean |
| ming-iot | team-specs-draft | `b8435c6704c4f32a7d0ea110a1240522c47546b8` | clean |
| ming-iot-rd-docs | map-delivery-ai-native-source | `ef9a37f57c5584f30fbe5fdcbdfb8036e3738a4c` | clean |

ming-iot 候选 `b8435c6704c4f32a7d0ea110a1240522c47546b8` 比主线领先 13 提交，29 文件变化，主线无独有提交。内容含 SaaS 昵称通知行为、API 检查工具、Things 基线测试，以及团队文档；不得作为纯文档草稿丢弃。本轮 M1 文档变更在该候选上继续，最终提交与独立复审另以准确 Head 记录。

## Core / Share / Things 重复与遗漏

| 源 → 目标 | 原来源基线 | 字节相同 | 仅 import 前缀变化 | 其他差异 | 源同路径独有 | 目标独有 |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| core → backend/saas | `1b3110e5e17d5a0066d38930b67bc44fdcc3c6fc` | 97 | 940 | 103 | 48 | 85 |
| share → backend/share | `ece02ca9d6defd63356931a59e653950f27ac572` | 162 | 82 | 11 | 0 | 18 |
| things → backend/things | `243ab1aa03b8a3f106761a81cdd9bd8827d448c8` | 104 | 762 | 111 | 230 | 92 |

上表目标固定为 ming-iot 主线 `6742201e9666fd3b3067216b2efbb3b8ba7338d7`，来源固定为前表各旧仓 HEAD；不以 team-specs-draft 候选替代主线。比较对象为 Git 跟踪文件的相同相对路径；import 归一化仅替换三个旧模块前缀，不证明业务等价。“源独有”也不等于遗漏，部分文件已有改名、生成替代或明确排除。

已定位的后续增量：

- Core 从固定基线到当前源 HEAD 有 9 文件变化，其中 7 个 Go 文件；目标 6 个仍等于旧基线，1 个双方均改。涉及服务上下文、中间件和 WebSocket。
- Things 有 71 文件变化，其中 49 个 Go 文件；目标 30 个仍等于旧基线、5 个双方均改、14 个同路径缺失。缺失包含 `checkControlAllowed.go`、`deviceStatusMaintenanceLock.go`、`historyRecord.go` 及相关回归测试。
- Share 当前源 HEAD 与已记录导入提交一致，但目标另有演进；不重新覆盖。
- 旧 Things 的 `tools/areastat/go.mod` 未在目标找到，作为工具单独核对。
- 目标已有物模型发布、生成契约、Kafka 契约和基础库的独有内容，旧源整体覆盖会丢失它们。

以上是源码对比的风险清单，尚未判断每个旧增量应采用还是被目标替代。M2 为每项补齐行为、依赖、测试和处理决定。

来源证据：[Core 来源](../superpowers/evidence/2026-07-23-core-source-baseline.md)、[Share SOURCE](../../backend/share/SOURCE.json)、[Things PROVENANCE](../../backend/things/PROVENANCE.yaml)。保留现有 `backend/go.work` 的四个模块；托管平台迁移与 Go import 改名分开处理。

## 脏改动、未跟踪与忽略内容

- ai-native 除本轮清单外有 80 条状态：一处 `.gitignore` 修改，其余为研究、实验、CONTEXT 等未跟踪内容。不能只复制 22 个 HEAD 文件。
- iot-doc 有一处删除和五份未跟踪资料；需要保留删除意图，不能从 HEAD 恢复后声称已无损迁移。
- ming-iot-rd-docs 有一份未跟踪 proposal；另有候选工作树和历史工具。
- 各仓被忽略文件量较大：ai-native 34,838，ThingsBoard 107,278；ming-iot、研发资料仓等也有本机状态。忽略不代表无价值：r-doc candidate、real-eval、reports 与临时运行环境需区分，源码/证据先备份，缓存不入公开源树。
- 根目录 `.agents/skills/map-delivery` 与 `.claude/skills/map-delivery` 是指向 ai-native 的链接；删除旧仓前必须迁移依赖并验证发现路径。

## 非 Git 内容全部分类

| 来源 | 本次观测 | 处理 |
| --- | --- | --- |
| 根 docs | 1 份 ADR | 保留历史、标记被取代拓扑，现行依据转仓内 architecture |
| 根 AGENTS.md / CONTEXT.md | 入口约定与概念表 | 合并有效条目，避免旧跨仓术语误导 |
| map-delivery-portable | 34 文件（不含缓存） | 实验源码/测试/说明入 tooling/experiments；dist 和验证临时树单列 |
| .agents / .claude | 两个发现链接 | 改为单一仓内来源后验证，不复制重复 Skill |
| .vscode | 1 文件 | 检查可移植性后选择共享配置 |
| .local | 1,074 文件（排除缓存型目录） | 本机备份/评审材料分类，不能作为产品复制 |
| .scratch | 7 文件（同上） | 实验来源与可再生输出分类保存 |
| .zcode | 1 文件 | 本机状态，私有保留 |

上述计数不是内容完整性校验；未跟踪、忽略及工作树快照的逐文件摘要与恢复验证留给 M2。

## 公开目标与第三方边界

已通过 GitHub 仓库接口核实目标为 `wangjunbo866-xyf/ming-iot`，public、size 0、默认分支 main。接口权限含 push，但本地 Git 推送认证尚未验证。本轮未改变远程或权限。

- AirMQ 当前 HEAD 根 LICENSE 标题为 Apache License 2.0。
- Core / Things 根 LICENSE 为 GNU AGPL v3；目标已有相同声明，需要保留。
- ThingsBoard 当前 HEAD 根 LICENSE 为 Business Source License 文本；不能凭项目名称假定为另一版本的许可证，也不能因作为参考源码就忽略其条款。
- Share、ai-native、iot-doc、研发资料仓以及 ming-iot 根未发现顶层许可证文件；不据此推断全树无许可证或已获发布授权，M2 继续核对各来源声明。
- 发现旧 Core 的环境配置、旧 Things/ThingsBoard 的证书或密钥类路径，未在本报告读取/输出值；这些可能是样例，但必须在公开前检查实际内容和历史。
- 旧 Things 跟踪了约 79.3 MiB 的可执行文件；研发资料仓跟踪了历史 Git bundle。它们不作为普通文档/源码直接公开导入，先做备份与用途判断。

本次未执行全历史秘密扫描、许可证兼容性判断或完整构建；没有声称公开发布已安全。

## 缺口与下一步

1. 工作区未发现独立 mqtt-gateway 或产品前端 Git 根；AirMQ/web、ThingsBoard/ui 是第三方项目自带 UI。目标目录仅保留预期位置，不能伪造迁入完成。若实际源码在工作区外，后续需补入确切来源。
2. Core/Things 的后续增量、旧分支和六个工作树须在 M2 逐项收敛，保留双方独有成果。
3. 本机 LoopX 当前仍依赖 ai-native 的入口；SQLite 与 Goal 路由迁移应在新入口验证后进行，本轮不移动数据库。
4. r-doc 正式采用、真人试跑反馈不再作为单仓迁移前提；历史评测原始状态保留。

M1 的验证只覆盖清单、路径映射、文档链接与当前规划一致性；后续源码完整性、构建、集成和新 clone 验收分别按迁移方案推进。

## M2 私有备份、恢复与本地导入演练

M1 上文是原始盘点，不回写成迁入完成记录。本附录记录 M2 的实际结果。实施候选为独立的本地 Git 仓，精确 Head **`53b9cc4ee8be539c33b586b2b7764583774eba0a`**，基于原有候选 `7e7573a273ba7f18c6be723dc1a5c7634ef31648`。原 ming-iot 主线与 team-specs-draft 的业务代码未被覆盖；本工作树仅更新迁移文档。所有源仓保留，未推送任何远程。

### 备份与实际恢复

- 9 个主仓保存全部 refs/HEAD 可达历史；75 个嵌套实验 Git 仓另存历史和 index/status/二进制差异。逐个从 bundle 恢复 mirror，核对 refs 并运行完整对象检查，全部通过。
- 15 个实际工作树（9 主仓 + 6 额外工作树）保存 **276,460 个文件、7,325,252,038 字节**，包含未提交、未跟踪、忽略文件及删除意图；根非 Git 内容另存 **1,124 个文件、85,608,831 字节**。以上是工作树内容计数，不是去重后的独有文件数。
- 从备份恢复到独立目录，逐文件核对内容摘要、权限、链接和文件集合；15 个工作树的 Git index 与状态对照通过。额外实验仓的历史恢复与本地状态元数据保存分别留证，不混称为 75 次完整工作区重建。
- 备份目录受本机私有目录权限保护。原始 Git 管理目录不直接复制到目标；可达历史由 bundle 保存，链接工作树分别保存；自身迁移输出排除以避免递归，非普通运行对象列入排除清单。本次不承诺未引用 reflog 对象恢复，也未创建异地备份。

### 导入与逐项裁决

路径映射包含 **274,362 项**，每项记载来源、快照、目标/私有保留去向和处理理由；旧源的所有文件并非全部进入产品构建。资料仓与第三方源码按目录草案进入普通目录；缓存、本机状态、风险文件名和旧二进制等明确保存在私有快照中。历史 AGENTS/CLAUDE 改为材料文件名；实验工具未接入默认 Skill/Hook 发现。

| 核对范围 | 结论 |
| --- | --- |
| Core 基线后 9 项 | 6 项直接补入、1 项三方融合、2 项旧依赖版本变化由目标本地模块图处理；保留 syssvr 的缓存初始化 |
| Things 基线后 71 项 | 31 项补入、14 项缺失 Go 文件补齐、2 项三方融合、2 项映射到拆分后的 proto/生成物；18 项历史保留、2 项保留目标依赖图、1 项保留已清理死代码、1 项已被目标覆盖 |
| Core 旧同路径独有 48 项 | 全部在原导入基线已存在；逐项区分改名/生成替代、历史资料、外部部署与私有配置，不重复引入旧聚合 RPC |
| Things 旧同路径独有 230 项 | 逐项区分迁址、14 项真实遗漏、拆分 RPC、历史资料/运维、风险材料及 2 项 areastat；不将其全部当作遗漏功能 |
| 原候选保留 | 原 2,789 个文件无删除；42 项原文件修改均在 56 个显式 overlay 内，另外 14 项为新增文件；其余原候选逐文件不变 |
| Things 目标独有 92 项 | 除 2 个明确追加 netTypes 字段的拆分契约文件，其余摘要均保留 |

最终共应用 **56 个文件增量（Core 7、Things 49）**。已复核 **14,982 个有源摘要的导入文件**，对应源字节一致；其余辅助来源有独立快照/历史映射。Share 保持目标已有演进，Go module 名称本轮不变。演练仓无远程、无 gitlink、无嵌套 `.git`。

第三方目录是经过私有边界筛选的参考快照，不保证能作为完整第三方产品直接运行。75 个嵌套实验仓仍以私有历史保存，不重新制造产品子仓结构。`components/mqtt-gateway` 与产品前端尚无实际来源，不创建占位源码或冒认第三方 UI。

### 验证结果与明确遗留

- 全部主仓/工作树备份恢复、额外 75 仓历史恢复、路径映射和候选保留检查：**PASS**。
- `make verify-pr`：**FAIL**。SaaS/Things 构建与业务包测试通过；失败为 `TestLocalShareLegacyReferenceAllowlist`，归档后实际 16 个旧引用文件、预期 5 个。多出的 11 项来自已导入历史资料。保持原始失败与日志，不删历史文本或扩大豁免来取得绿色。
- 补跑 `make vet contract-static boundary-scan`：**PASS**；这是失败后的独立检查，不替代整套 `verify-pr`。
- 引入历史/第三方资料的原始空白与 CRLF 提示另存；56 项实现 overlay 的差异格式检查通过，不声称整个历史导入差异无格式提示。
- 真实 MySQL、Redis 竞争/TTL、TDengine、设备及前端联调：**NOT_RUN**。新增 MySQL 测试因没有专用 DSN 跳过；静态和 fake store 结果不等于这些行为验收。小黄昵称显示验证仍待反馈。

M3 的具体顺序：先使历史来源分类与旧引用检查相容，同时继续拒绝活动代码中的旧依赖；适配选定工具的新路径与已失活模板；修复 Core 文档图片/联调脚本导航；处置 JWKS 云函数、历史 SQL/运维的离线归档与 Things areastat。areastat 调用缺失 RPC 且只取首设备聚合值，本轮不启用、不擅自决定业务统计语义。完成入口适配后重跑完整门禁和独立复审。

M2 证明可恢复及本地导入路径，不表示 M3 构建门禁已通过、工具已被团队采用、公开历史已审查或迁移 Goal 已完成。公开历史/许可证检查及新 clone 验证在 M4，LoopX 运行入口迁出旧目录在 M5 前完成；旧源删除不属于本轮。

### 可复核证据索引

本附录以 `ming-iot-monorepo-m2-20261015` 登记于本 Goal；机器证据相对定位见 M2 Todo 回执。本文件不嵌入备份正文或本机绝对路径；来源登记中的脱敏摘要不是文件定位器。

| 文件标识 | SHA-256 |
| --- | --- |
| `backup-restore-receipt.json` | `f57c794abc7c1a7ffc5b45bf7aae6b5e4b8c0b50f11a43b92ea5a70be6d6c601` |
| `nested-backup-restore-receipt.json` | `38a8ba82511a776541e612b36986407ba4beb0f325e2ff3cd4e01c5d0189cf3a` |
| `import-map.jsonl` | `cb205102347b438c08120477d5b3f44043fc1576a9be479751c49565e9fb5e9f` |
| `reconciliation/core/manifest.json` | `1ea93350790a143e2988d9845e0e2b76f50849812d366d132a2ec2cb466eac46` |
| `reconciliation/things/manifest.json` | `9dc8a8ff643063aac5d20abf1b4a0cb4a9aab64beb01344521151a6a0dafff69` |

原始目录另含 `preservation-receipt.json`、`nested-state-receipt.json`、逐项分类与 overlay、首轮验证失败日志和独立复审回执。文档摘要不替代精确候选及原始日志。

## M3 范围修正与仓内入口适配

用户确认 ThingsBoard/AirMQ 是学习参考仓，不迁入产品；Core、Share、Things 已合并到 ming-iot。本轮以 M2 候选为基础保留所有审定增量，仅在独立 M3 候选移除参考源码副本。上文 M1/M2 是当时观察与演练结果，不回写成从未导入；两个原仓、M2 备份及私有历史均保留。

实施内容：

- 旧 Share 引用检查使用 `docs/meta/HISTORICAL_REFERENCES.json` 逐文件核对限定路径、内容摘要和历史理由；不按目录全部豁免。新活动代码、未登记历史文件、摘要变化和越界/符号链接继续失败。
- 新增 `tooling/knowledge.py` 与 `make knowledge-check`，在任意检出目录定位本仓正文并调用原 ming-specs 检查；`verify-pr` 的原七个阶段保留，在静态契约阶段增加知识入口与回归检查。
- 根 README、AGENTS、docs 导航给出当前模块、命令、关键约定和状态。旧团队分发与研发工具放入 experiments；不要求邻接旧仓、不自动安装旧 Skill/Hook。r-doc 候选移入保留上游层级的实验目录，补齐备份中可核对的测试夹具、辅助程序和依赖声明，适配失活模板链接；不覆盖原候选代码/回归，不改变未正式采用的结论。
- 修复 SaaS 任务管理图片和本地联调入口；42 份旧 Core/Things 附属材料按来源摘要保存，执行性内容以 `.source.txt` 保留。私有配置/私钥示例不导入；旧 SQL、云函数和远端脚本不执行。areastat 保持停用，业务语义缺口见对应说明。

完整验证范围为 `make verify-pr`、已有 ming-specs 回归、入口正反例、r-doc 受影响候选回归，以及从本地新 clone 验证入口可用。结果与独立复审绑定精确 Head，机器回执由当前 Goal 的 M3 Todo 定位；原 M2 FAIL 不抹除。真实数据库、设备、前端和真人接收未验证的边界保持不变。

后续 M4 先核查拟公开文件/历史、来源许可证与最终目录；特别排除 M2 私有演练历史中的参考源码和本机资料。当前不得推送远程或删除旧仓。LoopX 运行状态仍在已注册本机入口，不在候选复制第二份数据库。

### M3 最终回执

- 实现候选：`487c2f068a66f0a4878eef04dafad1241270a535`；此候选在独立私有演练工作树完成，业务源仓不被替换。
- `make verify-pr`：**PASS**，包括构建、全包测试、vet、静态契约、交付边界，以及新增知识检查；原七阶段不缩减。
- 知识入口 2 项、ming-specs 25 项回归：**PASS**。历史引用新增正反例覆盖未登记路径、活动代码、历史内容变化及符号链接。
- r-doc v3 / v4：**144 / 161 项 PASS**，在已安装候选声明依赖的 Python 环境运行；Skill 资源验证也通过。初轮缺配套资源、系统 Python 缺 pytest 的失败日志保留。此项为原工具离线回归，不是模型效果评估或采用决定。
- 独立新 clone：**PASS**，无对象替代目录或原工作树符号链接；根知识检查、从嵌套目录调用入口/规格检查帮助、历史引用契约通过。完整后端门禁在实现候选运行，新 clone 本轮未重复完整构建。
- M2 的 56 个实现增量逐文件摘要一致；原九仓 Head、refs、15 个工作树状态核对通过，仅原候选分支内三份迁移文档按授权更新；旧源、备份及原始失败回执保留。
- 独立复审：`PASS`，阻塞 `0`。完整审查正文与警告项由当前 Goal 的 M3 Todo 回执定位；不会把非阻塞的后继事项写成已完成。

验证只证明本地 M3 入口与不依赖外部服务的检查通过。真实数据库、设备、前端、真人接收及宿主实际加载 Skill 未新增验收；LoopX 运行入口尚未切换；未推送 GitHub、未删除旧仓。

后继先做 **M4 本地公开候选准备**：核对拟公开文件、可达历史、来源与许可证，排除学习参考仓、私有运行资料及不应公开的旧实验历史，准备 CI/新 clone 验证。当前授权仍不包含远程推送或旧源退役。

| 私有证据标识 | SHA-256 |
| --- | --- |
| `verify-pr-receipt.json` | `7e6dfe20b961d76a0dccc64c35fb2d85b07dab4047d00a44b47be9d447b42b7a` |
| `fresh-clone-receipt.json` | `cf81fd84ba79cafd0b4bd21ea41143b43a4612b5e51628896d2de99ec1e5fc8e` |
| `preservation.json` | `13f36f7f5a261bdaa7f380d6d7b426379b0f4dbb7eb6381691f5f0a9ff5e56c7` |
| `review/review.json` | `56d999fa2d286296c3763121c9aea216ced3999c42a51dcf3468a8000f5e3c75` |
| `review/review.md` | `f4204e806f0ff684dc824a9218cb6f73cb8a144a6c8fa4c03e34a622b454005d` |

## M4A 本地候选完整验证与独立复审

2026-10-15 用户授权 M4 本地准备、全新 clone 完整验证和独立复审；继续排除 ThingsBoard/AirMQ，保留旧候选及私有备份，不推送 GitHub、不删除旧仓。**本轮 M4A 已完成，公开状态仍为 HOLD**；“本地通过”不代表整个 M4 或迁移 Goal 已验收。

### 精确候选与文件保全

- 来源 M3 Head：`487c2f068a66f0a4878eef04dafad1241270a535`，保持原样。
- 最终 M4 Head：`25e9cf73d91c566cc59161fc218ad9739da2f07a`；Tree：`bd2075baa247a63df0bbee663a234b3c92ddaf67`。独立本地 Git 仓，无远程；从审定 M3 树建立新历史，旧完整提交历史保留在私有来源/备份。新候选不包含旧历史祖先，不可称为保留了原提交历史的公开仓。
- 全部 3,952 个来源文件有逐项映射：3,937 个字节不变、5 个脱敏、10 个 M4 适配；另增 12 个，最终 3,964 个跟踪文件。225 个归档 `.codex` 文件改为 `codex-materials` 路径，字节不变且有原路径/摘要表，避免误当运行状态或被忽略。
- 5 项脱敏包括 OAuth 测试中的应用凭据示例、AWS STS 文档中的账号/密钥/桶示例、3 份设备鉴权测试的固定产品密钥。测试改用确定性假值并重新计算签名期望；不访问真实凭据服务。Share 253 文件冻结检查仅更新两项已登记脱敏内容对应的整体摘要，没有扩大排除项。56 个 M2 业务增量逐文件摘要不变，未更改业务行为。
- 原九仓 Head/refs 和 15 个工作树状态按 M2 基准核对；只允许原文档候选的三份迁移正文/登记更新及私有迁移证据、LoopX 记账。九个 Git bundle 共 662,214,356 字节，独立复核摘要与恢复回执一致；不删除或替换任何备份。

### 完整可达历史与公开边界

旧 M2/M3 公共对象库的全部 refs 加 HEAD 覆盖 158 个提交、6,282 个 tree、21,296 个 blob；逐对象及提交/标签文本扫描，并递归展开识别到的 ZIP/GZIP/TAR，共 11 个归档叶项。扫描得到 853 个模式命中，其中包含第三方测试夹具和普通误报，**不能据此称有 853 个真实秘密**。旧历史整体保留私有，未混入新候选。

新候选全部 refs 加 HEAD 覆盖 9 个提交、1,300 个 tree、3,473 个 blob及 11 个归档叶项。逐提交拒绝学习参考仓、私有运行目录、gitlink 和符号链接；内容扫描的 38 个命中均有精确内容摘要、规则、行号及理由，对应来源哈希、Go 校验和、确定性测试值或普通文本，未解决命中为 0。例外不按目录放行，归档内内容也不继承外层例外。扫描器覆盖已删除但仍可达对象以及其他本地分支；压缩格式按内容识别，限制深度、大小、数量，并拒绝无法可靠检查的已识别归档。

这些是字节模式与路径检查，不包括图片 OCR、所有任意二进制格式或权利判定。历史资料内的旧路径/地址仍可能作为语境保留，不代表实际连接配置已启用。原始日志和失败证据私有保存，公开文档不复制秘密值或本机私有定位器。

### 许可证核对与未闭环事项

原 SaaS/Things 的 AGPL 声明、Share 的 Apache-2.0 及来源、r-doc 的 MIT 及署名保留；Share 历史规格已经记录的权利人确认继续有效，不重复要求授权。10 个既有 LICENSE/NOTICE/UPSTREAM 文件字节不变，自动策略还核对来源/基线声明等共 12 个文件。没有给整个混合来源仓擅自加统一许可证。Go 依赖清单覆盖 661 个模块元数据，429 个有本机内容，其中 417 个模块包含根目录许可/声明文件，共 453 项；这是存在性盘点，不是完整依赖闭包权利或运行合规结论。

| 编号 | 尚未闭环的依据 | 本地后继工作 |
| --- | --- | --- |
| LIC-01 | 外部文章全文及 6 张图片的再发布依据 | 查明已有许可；否则形成仅摘要/来源链接进入公开树的方案，原文私有保留 |
| LIC-02 | 混合历史文档、品牌/二维码等素材的逐来源权利分类 | 逐文件关联既有授权或明确私有保留范围，不能整体推定有权或无权 |
| LIC-03 | 历史 Mermaid bundle 的精确来源、版本和完整许可包 | 核对发行物及依赖声明，或提出公开树排除清单 |
| LIC-04 | 本项目原创工具/文档的公开许可范围 | 整理既有权利人决定和许可边界；AI 不代选许可证 |

以上材料仍保留在本地完整候选中；技术扫描 PASS 不会把 `PUBLICATION_POLICY.json` 的 HOLD 自动改为可发布。缺少许可证不等于权利人不能发布自己的作品；本轮只登记证据缺口，不作法律结论。除四类事项外，本轮也没有远程发布授权。

### 单仓 CI 与全新 clone 验证

候选 `.github/workflows/monorepo.yml` 已准备：Go/Python 环境，校验摘要后安装本仓工具，工作流语法检查、全部可达历史扫描、完整 `make verify-pr` 及工作树清洁检查。Actions 固定提交，权限仅 `contents: read`，不保存 checkout 凭据，不含发布或部署动作。实际 GitHub 托管运行尚未执行。

最终候选使用 `--no-local --no-hardlinks` 新 clone，无对象替代目录，移除本地 origin；从无 `.cache`、无 `backend/bin` 开始，工具重新下载并验证摘要。复用宿主已有 Go 模块/构建缓存，**没有声称空机器离线构建**。

| 验证 | 结果与范围 |
| --- | --- |
| 工具安装、actionlint、publication-check | PASS，绑定最终 Head；技术扫描无未解决命中，公开状态仍 HOLD |
| 全新 clone 的 `make verify-pr` | PASS，112.97 秒；原七阶段完整执行，含构建、全包测试、vet、静态契约、交付边界；结束后工作树干净 |
| 知识/公开检查回归与 ming-specs | 8 项（含 6 项公开扫描回归）及 25 项 PASS；嵌套目录入口亦通过 |
| r-doc v3 / v4 离线回归 | 144 / 161 项 PASS；安装声明的 Python 测试依赖后运行，不代表工具正式采用或模型效果通过 |
| 独立实现与规格复审 | 最终 Head PASS，未闭合实现缺陷 0；许可事项按公开 HOLD 单列 |

独立复审发现并推动修复了归档扫描漏检及脱敏测试签名期望两项问题；归档回归使用真实扫描器验证已删除普通内容、ZIP、GZIP、tar.gz、嵌套归档及其他分支内的秘密能被检出。另修复 Share 摘要基线及验证进程错误继承 `umask 077` 导致的既有文件权限用例失败；验证进程改用仓库预期 `umask 022`，未修改产品权限实现。初轮/次轮 clone、失败日志、旧复审意见、系统 Python 缺 pytest 的失败均保留，最后一轮通过没有抹除历史。

尚未验证 GitHub 托管 CI、真实数据库/设备/前端联调、真人接收或宿主实际加载 Skill；不以资源可读替代宿主加载验收。LoopX 运行入口尚未迁出旧工作区，小黄验证仍单独待反馈。后继只先整理本地权利/公开范围的具体处置方案，M4B 远程交付及 M5 退役继续未执行。

### M4A 私有证据索引

完整证据由当前 Goal 的 M4 Todo 与 `final-result.json` 定位；下表摘要绑定本轮精确实现。最终文档提交和文档独立复核另在私有最终回执记录，避免正文包含自己的提交哈希。

| 私有证据标识 | SHA-256 |
| --- | --- |
| `fresh-clone-receipt.json` | `0db04778bba1bc741ebb04ee4c134b1d103912255468a2e2fa160ad2e1fd4403` |
| `rdoc-regressions.json` | `457eec3913e4f088b40eebba9e6892c714321d8d85b1f30fbee581a9fd1f9f93` |
| `audit/candidate-file-inventory.json` | `e4a085ef4fd3ceb36b1b52381bfb51e7e2d38984631524a846553f7b6b06d312` |
| `audit/old-history-recursive.json` | `6007d10acf7747073a15bf5c2820d135abdacec4bc498be1815815c79fb7b931` |
| `audit/license-review.json` | `5c913a6561b09e6bf2677e9f4ff4bb23b7d2ea9ce3311a714eeb8142a8f603a1` |
| `audit/go-dependency-inventory.json` | `0e268d50dc9b9d2b8e17d000a9ef0d71694deea4b4cb0a065b96e1d248697640` |
| `source-preservation-final.json` | `ea95d702fd67a04817593181f7ae822e300d3040d0a4d214ec208e6e48c764fd` |
| `review/standards-v3.json` | `8b6b3b4b6e1bc62891f3873e0b4cefc0e2926ecad4d6068069b3c8c467795243` |
| `review/spec-final.json` | `00bccd8da719ed4b1efefa14e040849ec89beec3d6c219669ff933d88bb16083` |

## M4B 前置：公开材料与权利处置方案

状态：本地方案，尚未实施公开删减或新增许可。Goal：ming-iot-knowledge-pilot。实现固定 Head：25e9cf73d91c566cc59161fc218ad9739da2f07a。原 M4A 验证、候选、旧历史及备份保持原样。

### 本次复盘结论

不能把“四个 HOLD 全部交给用户”当作后继动作。当前可以自行完成 Mermaid 精确发行物核对、许可包整理和历史资料来源映射；只有具体公开范围变化与自有材料权利/许可选择才需相应责任人决定。Share 冻结版本已有确认继续有效。原 M4A 技术验收不重复运行，除非后续实际变更候选。

本轮逐文件清单覆盖候选全部 3,964 文件，记录路径、SHA-256、M3 来源映射、许可声明线索、问题归属和拟处理动作。这是内容身份及证据盘点，不是逐篇完成法律权属判定：没有凭路径、Git 作者、AI 生成或文件存在就认定原创/可公开；图片未做 OCR。完整清单在私有 rights-file-ledger.json。所有处理动作 implemented=false。

| 分组 | 文件数 | 已有事实与拟处理 |
| --- | ---: | --- |
| SaaS/Share/Things 现有组件 | 2,582 | 保留对应 AGPL/Apache 声明、NOTICE 和已记录 Share 授权；不改写上游许可，不据此宣称所有依赖或素材已清权 |
| r-doc v3/v4 实验组件 | 235 | 保留 MIT 与原署名、候选身份；不升级为正式采用 |
| Things 基线派生模板 | 5 | 有现存上游来源 NOTICE；排除在新增“原创许可”范围之外 |
| 项目工具/入口/现行及早期文档等待确认范围 | 288 | 原创许可候选清单，其中 backend/tools 有 122 项，另含根入口和部署资料；不是已证明全部原创的清单 |
| 历史正文和实验材料 | 828 | 保留原稿与来源线索；按来源与内容继续判断，不能用统一根许可证直接覆盖 |
| 外部文章及图片 | 7 | 一篇全文、六张图片；建议原件私有保留，公开位置改自写摘要与来源链接；尚未实施 |
| 历史素材与产品例子 | 18 | 17 个图像/品牌/二维码素材及一份产品 JSON 例子；建议在取得覆盖依据前仅私有保留；尚未实施 |
| Mermaid bundle | 1 | 已确定精确版本和字节，继续补依赖许可包；有明确保留路线，不必直接删除 |

以上分组互斥，总计 3,964；LIC 编号可能交叉，不得将问题计数相加当作文件总数。许可保留策略中的 12 项声明/来源摘要均仍匹配。已知 56 项 M2 业务增量、M4A 全新 clone 与三份独立审查回执仍有效，本轮没有修改这些候选。

### LIC-01：外部文章及六图

范围：docs/rd/references/unitedrhino-tech-arch-02.md 及其 assets/unitedrhino-tech-arch-02/ 六图，均在清单逐项记录摘要。来源为联犀官方文章 https://doc.unitedrhino.com/blog/tech-arch-02/ 。本轮查阅该页面，没有找到覆盖本仓全文/图片再分发的明确声明；这只是所查页面的结果，不代表已经穷举所有权利证据，也不认定侵权。

具体方案：保留私有完整副本及备份；拟公开原位置改为我们自己的短摘要，说明其用于工程组织参考，保留作者、日期和链接，删除公开文件对本地六图的引用。摘要写法示例：“该资料作为代码生成、模块划分与统一工程入口的外部参考。Ming IoT 的实施要求以本仓当前规格为准。”正式替换须在公开范围决定后实施；本轮没有修改正文或文件树。

选择保留全文时，后续证据必须明确覆盖该文章和六图，而不是仅提供 Core/Share 代码授权。当前不能将 Share 已有授权外推到网站材料。

### LIC-02：18 项历史素材及其余历史知识

历史 Core 素材包括架构/组态/设备连接图、六个客户品牌图片、联系/赞赏二维码及产品例子。该目录 18 文件逐项列入方案清单；“品牌”“二维码”是从原路径得到的筛查线索，未声称已视觉识别内容或确认第三方权利。

建议先不公开这 18 项，保留所有原件与恢复链；已能出示覆盖依据的文件可逐项保留，不要求重复授权。其余 828 项历史正文/实验文件继续保留，不能为快速使检查变绿而整体删掉 wiki/rd。后继按原仓及内容来源，分清项目自己的讨论记录、外部复制内容、历史 Skill 副本和上游派生源码；不能仅凭来自私有仓就判无权公开，也不能凭已提交就判有权公开。

将文章七项与历史素材十八项均仅私有保留，是 **25 文件**的具体公开范围选项；加上 Mermaid 也私有保留的备选则为 **26 文件、15,526,441 字节**。后者仅是备选，优先补齐 Mermaid 声明。该清单不是完整公开放行清单，处理这 26 项后也不会自动关闭剩余历史来源和权利问题。

对 26 项做了完整路径/文件名文本检索，发现 26 处引用候选，分布在 8 个文件：LEGACY_MATERIALS、PUBLICATION_POLICY、旧研发 DOC_REGISTRY、references/README 及四份历史方案/规格。这是限定规则的引用线索，不是完整语义依赖图。后续改变公开范围时需逐项复核正文、导航和元数据；历史决定留存，只调整材料去向说明。

### LIC-03：来源已确定，依赖许可仍未闭环

从本地 bundle 的版本常量得到 11.4.0 线索；随后从官方 npm registry 读取固定版本元数据和发行包，校验 SHA-512 integrity，再按内存读取 tar 成员，未运行包内程序或安装脚本。

- 元数据：https://registry.npmjs.org/mermaid/11.4.0
- 发行包：https://registry.npmjs.org/mermaid/-/mermaid-11.4.0.tgz
- 发行包 SHA-256：26f7d5d5498cd889b32f911349b3772d517d99cb7c021f6a9cadca0d19f20e12
- package/dist/mermaid.min.js 与本地文件均为 2,571,838 字节，SHA-256 同为 1bca2e3c09f0e33ccc6e701e9c79d5cfa87d5e929b35d3a4aea4e1ec069dc87b。
- 发行包 LICENSE 为 MIT，SHA-256 ec9fb67dcb25eccc416ed56e1aab819222c805a2a4bfe4cb19e7556bf2ffde80；同版本上游声明 https://github.com/mermaid-js/mermaid/blob/mermaid@11.4.0/LICENSE 。
- 固定发行包 source map 包含 1,090 项来源、70 个 pnpm 依赖标识，含并存版本、peer 后缀和带 patch 标记的 roughjs。不能按 package.json 的版本范围重新安装最新版来替代这些具体来源。

建议后继先按该 source map 整理精确依赖 LICENSE/NOTICE 与补丁来源，将结果作为候选的独立许可包方案。保留 bundle 原有尾部声明；核对生成器/本仓解析器源码等非 pnpm 来源。完成源映射、许可文本与例外核对并独立复核后，再把 LIC-03 从“完整 HOLD”更新为实际已关闭部分。当前仅 **精确来源/版本子问题已解决**；不将 Mermaid 的 MIT 一份文本当作所有 bundled dependencies 的完整证明。

### LIC-04：自有范围与开源许可选择分开

此前范围聚焦 docs/tooling/skills，本轮增加根入口、部署资料及后端验证工具；其中五项 Things 基线模板有明确上游来源，不能放入项目自有授权清单。其余 288 项只是需确认的项目范围；与 828 项历史正文一起还需逐文件列明第三方例外，不能根据目录宣称都归本项目所有。

需要确认的是：谁有权决定这些具体自有内容的公开与再分发、哪些文件属于第三方例外。**不强制迁移先选 MIT 或 Apache。** GitHub 官方说明可以不选择新许可证，公开可见与开放源代码许可不是同一件事；公开仓的访问/fork 行为另受平台条款约束：https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/licensing-a-repository 。这一说明不能取消既有 AGPL/Apache/MIT 义务，也不能用于他人的复制内容。

供权利人决定的具体路线（均不在本轮执行）：

1. 确认自有文件清单后，为该清单明确采用 Apache-2.0；保留所有第三方例外，不用仓根一份文本覆盖其他组件。文本依据：https://www.apache.org/licenses/LICENSE-2.0 。
2. 对同一已确认自有清单采用 MIT，并保留必要版权/许可声明。文本依据：https://opensource.org/license/mit 。
3. 对已确认自有内容暂不新增开放许可，保留原有权利状态；不宣称整个仓库都可按某一开源许可证复用。第三方内容继续沿用原许可。

在自有范围及例外清单进一步收敛前，不要求用户给所有 1,121 个涉及 LIC-04 的条目一揽子授权。也不把“缺少新许可证”单独解释为权利人不能公开自己的文件。本轮不改已验证候选的 HOLD 配置，后续依据明确证据和决定调整规则。

### 候选历史与实际实施顺序

当前 M4A 的九个提交已经包含上述材料。如果最终决定不公开某些材料，只在末端做一个删除提交不能使这些旧对象退出可达历史。应从经确认的文件选择建立一个 **新的独立本地候选历史**，保留 M4A 原候选和全部私有来源；不重写旧仓，不把旧 refs 当作新公开 refs。新增的文档最终结果也需重新纳入该候选。

顺序为：补齐可自行核实的来源/声明 → 独立复核逐文件清单与例外 → 权利人只决定剩余具体公开范围及自有许可路线 → 构造新候选并检查全部可达历史 → 重跑受影响检查及完整新 clone/独立复审 → 具体远程动作另按用户授权执行。未决定的删减、许可证授予、GitHub 推送、旧源删除、数据库/设备联调均未执行。

小黄对昵称显示的实际验证仍独立待反馈，不阻塞本地材料核对。M5 的 LoopX 运行入口切换与旧仓退役仍未完成。

### 原始证据与验收边界

rights-file-ledger.json 为结构化逐文件表；rights-summary.json 为互斥分类统计；narrow-option-link-impact.json 是文本引用线索；mermaid-provenance-probe.json 与官方发行包/source map 元数据记录精确来源。外部查阅是只读，不向网站发送项目代码或凭据。本轮没有完成整套法律意见、所有传递依赖许可分析、图片视觉识别或所有历史文档的内容清权。

### 材料方案证据摘要

以下证据保持私有，全部动作仍是方案，未修改 M4A 候选。独立复核回执由当前 Goal 定位，不将检查成功当作材料许可全部完成。

| 证据标识 | SHA-256 |
| --- | --- |
| `rights-plan.md` | `bdb9217a2f4640662434bd5d0966a82499c02b330057504171ddb264b65f0611` |
| `audit/rights-file-ledger.json` | `e1690cabd9b1aad95bedca31ab832d2cc1d8ea2b584a6099582b3b3046ed769c` |
| `audit/rights-summary.json` | `da5df8facb7637a08a44ceca2a8892824814ae25571a7c9908d43539708d73c8` |
| `audit/narrow-option-link-impact.json` | `b94e43139c9234063b73574595712a4368cfbaf9cebb9e0c1fcc15b68d916fa8` |
| `audit/mermaid-provenance-probe.json` | `485227595d9884b490bfc073e97a994f429756487356ff165d02fb60c5ab04c3` |
| `audit/mermaid-bundled-source-map.json` | `a9b1a8081e5cb1a84ee00d45585209eec58523431a8ff44f31b1b3cf4b128a07` |

## M4B 来源与许可声明核对进展

状态：本地证据与声明包草稿，公开仍 HOLD，当前 Todo 保持进行中。固定候选为 `25e9cf73d91c566cc59161fc218ad9739da2f07a`；本轮没有修改该候选、公开文件范围或任何许可证授予。

### Mermaid：固定依赖的来源已逐项对应

官方 npm Mermaid 11.4.0 的 bundle 与候选文件完全相同；沿用上一轮已验证的发行包及 source map。本轮按其中的精确标识读取官方 npm 元数据和发行包，没有安装依赖或执行包内脚本。70 个依赖包全部通过发行完整性摘要检查，全部取得 LICENSE/COPYING/NOTICE 类文本；peer 后缀、并存版本和 patch 标记保留在逐项表中。

- 769 个 pnpm 来源片段中，745 个与发行包普通文件完全相同；另外 24 个与同一发行包的嵌套 source map 中源码完全相同，覆盖 DOMPurify、marked 和 vscode-uri。24 个文件最初记录的 member_absent 是普通归档成员不存在，已由第二层 source map 解释，不是遗失源码。
- 321 个非 pnpm 片段中，292 个与 Mermaid 11.4.0 标签指向的固定提交 `56f522ec50b5c746be5532686dd465a06215273d` 完全相同；12 个与 `@mermaid-js/parser@0.3.0` 发行包完全相同。
- 其余 17 个是构建产物：16 个包含 Jison 0.4.18 生成头的解析器；1 个由上游 JSON Schema 插件输出的默认配置。已定位对应语法文件、schema 和转换实现，但没有重建这 17 个产物，不将其记为源码字节相同。
- roughjs 的 patch 标识与固定上游 lockfile 一致。原补丁仅修改 `bin/rough.d.ts` 的导出类型声明；source map 涉及的一个 roughjs 源码条目与未打补丁的 npm 4.6.6 归档完全相同。补丁原文及摘要已留存，不需要推测另一份运行时代码。

从而，1,090 个来源条目中有 **1,073 个源码字节对应**，另 **17 个已定位生成路径但未重建**。这说明文件来源的核对程度，不是完整法律意见、运行合规或构建可重现性证明。source map 本身的来源也绑定到此前已校验的官方发行包。

`MERMAID-THIRD-PARTY-NOTICES.draft.md` 保留 70 包声明、Mermaid 与 parser 的许可，以及 Jison README 中的完整许可段。它目前仅是私有草稿：`jison-lex@0.3.4` 的精确发行包 README 只写 MIT，没有完整版权/许可正文，需继续定位适用于该版本的声明。生成器输出中应保留的运行时代码声明也须核对。候选 bundle 原有尾部声明未动，不能用根目录一个 MIT 文本覆盖所有依赖。

官方精确来源：

- Mermaid：<https://registry.npmjs.org/mermaid/11.4.0>；固定源码：<https://github.com/mermaid-js/mermaid/tree/56f522ec50b5c746be5532686dd465a06215273d>。
- Parser：<https://registry.npmjs.org/@mermaid-js%2fparser/0.3.0>。
- Jison：<https://registry.npmjs.org/jison/0.4.18>；Jison Lex：<https://registry.npmjs.org/jison-lex/0.3.4>。
- 70 包各自的精确元数据 URL、归档 URL、完整性检查方式、文本成员和 SHA-256 均在 `mermaid-dependencies.json`，不是按版本范围重新解析出的依赖树。

### 历史知识：828 文件已全部记录来源路径

对上一轮的同一组 828 文件逐一复核摘要，共有 525 种不同内容；没有据此去重或删除文件。825 项关联 M2 导入或历史材料映射，另外 3 项确认是迁移时生成的导航 README，并绑定最初加入提交及完全相同的内容摘要。来源记录不等于所有权证明。

| 内容分组 | 文件数 | 本轮证据与边界 |
| --- | ---: | --- |
| 与固定 r-doc 上游完全相同 | 204 | 与上游 `bc23420d7df5ec1d2c5fb45e066c919790ab9cf9` 的文件逐字节相同，并取得该提交 MIT 文本；应保留上游署名及许可，不作为本项目新授“原创许可”内容 |
| r-doc 改写捕获副本 | 24 | 共 5 种不同内容，逐种保存相对上述上游文件的差异；保留上游派生线索，不能推定改写部分作者或权限 |
| 实验输出和评审 | 220 | 试跑工作区、输出、评审记录；实验产物包含业务材料，不能因 AI 生成就判定原创 |
| 研发治理及项目历史 | 130 | 旧研发仓的规格、计划、评审、快照和导航；记录原仓/路径/版本或快照 |
| IoT 设计与研究资料 | 130 | 早期架构、部署和研究记录；引用和代码块仍需内容层面的复核 |
| 小团队试验记录 | 57 | 需求记录、资产正文及试验导航；“assets”目录名不意味着全部是图像，也不意味着已清权 |
| 团队研究、导航与工作区材料 | 39 | 团队约定、研究和迁移导航；包括新生成导航的提交证据 |
| 旧产品运维材料 | 24 | 历史脚本、部署及说明；按原内容保留，不自动恢复为可执行工具 |

合计 828。至少 228 个 r-doc 原样/改写副本应排除在一揽子“项目原创”声明之外；剩余 **600 项只是后续内容复核范围，不是已证明原创的清单**。已有组件声明和 Share 授权不重复索取，也不外推覆盖这些混合资料。

补充的全文规则扫描发现 82 项带来源/引用字样、272 项含代码围栏、3 项含版权/许可关键词、1 项含图像引用、53 项含提示/响应字样；这些计数交叉。关键词只帮助排查，不是引用合规或权属判定，未声称每篇均已完成人工语义审查。r-doc 改写副本保留 5 份精确差异，可以按内容种类核对，不必逐个重复处理 24 份。

上游依据：<https://github.com/riesaexe/r-doc/tree/bc23420d7df5ec1d2c5fb45e066c919790ab9cf9>。没有将上游 MIT 外推至所有试验输出或本项目业务文档。

### 剩余工作和决定清单

当前仍可自主推进的工作：补齐 Jison Lex 精确版本的完整声明及生成器运行时来源；核对声明包遗漏；按来源/引用标记对其余历史正文及 5 种 r-doc 改写做内容级例外复核。它们继续留在同一个进行中 Todo，不转成笼统的“等待用户授权”。

待上述复核收敛后，给权利人集中呈现两类具体决定：

1. **公开范围**：此前已列明的文章及六图、18 项历史素材共 25 文件，是保留完整公开副本还是仅私有保留并公开摘要/来源链接。Mermaid 优先补声明后保留，26 文件方案仅为额外排除它的备选。选择尚未执行。
2. **项目自身内容及第三方例外**：仅针对最终可审阅的文件/内容组确认公开权限；新增 MIT/Apache 等许可是另一个可选决定，暂不新增开放许可也是可表达的状态。当前不能把“600 项历史材料 + 288 项项目范围”直接称为全部原创，也不要求对此一揽子授权。

没有让权利人替我们排查仍可核实的技术来源。后续公开范围有变化时，要从最终选择建立新的独立本地历史，保留 M4A 与所有私有备份；仅删末端文件不能排除已有提交里的对象。GitHub 推送仍未获本阶段授权，M5 运行入口迁移及旧源退役尚未完成。小黄对昵称规则的真人反馈单独等待，不阻塞这些材料核对。

### 验证范围

本轮只新增私有证据及原文档工作树内的三份迁移文档；产品候选未变，复用 M4A 精确 Head 的完整新 clone 验证。对新证据检查条目覆盖、哈希一致、分组互斥、声明来源和状态边界，并独立复审；评审回执与最终文档 Head 单独登记。未重新执行产品测试、GitHub 托管 CI、设备/数据库联调或真人验收。

### 来源核对证据摘要

证据保存在当前 Goal 的私有材料中；这里记录内容摘要，避免公开本机定位器。进展的独立评审及精确文档 Head 以 Goal 回执为准。

| 证据标识 | SHA-256 |
| --- | --- |
| `provenance-progress.md` | `31856618bbbe2d8f39d24cddb40a746d687eca39fd442f0c7210b3e1eccd068d` |
| `MERMAID-THIRD-PARTY-NOTICES.draft.md` | `f5e349c027797f8308a3e533a35e9e38f92ad24af0c8e9028773079c57df83f3` |
| `audit/mermaid-dependencies.json` | `ad6c2949f5fde7c5b6c616646c79562f8bdededbcb51ca1daf8fa2ad08f24e8c` |
| `audit/mermaid-resolution-final.json` | `f240617a366b6d7a43b8d38c20ebb4b22f8d23b516e891bebe81fb8c7b5445e8` |
| `audit/historical-source-groups-final.json` | `de69648b4712086a37f8240b0b841e5b3484d21c7cdc0cec8b715b87fa2525c6` |
| `audit/historical-reconciliation.json` | `6f4f9347bd38d85bb48383e5e93767e118bcd38a16ab1ead163a366139163731` |

## M4B 生成器来源与历史正文例外

Goal：`ming-iot-knowledge-pilot`；当前 Todo：`todo_398074be37e2e5ba36893b67`，继续进行中。本轮为可核查的来源补证，不是公开放行。M4A 候选仍为 `25e9cf73d91c566cc59161fc218ad9739da2f07a`，未改动产品或候选文件。

### Mermaid 生成器运行时补证

Jison Lex 0.3.4 的 npm 包有 7 个文件与[固定官方源码](https://github.com/zaach/jison-lex/tree/28aa5ac329310ac74eff0d4259fcf594429134f7)逐字对应；包内另有 `.npmignore`，不冒称 8 个文件全部对应。原 README 和源码头明确标示 MIT。本轮检索抓取到的官方仓库全部可达历史：33 个提交、46 个 blob、500,864 bytes，无跳过对象；没有找到完整许可正文或名为 LICENSE/COPYING/NOTICE 的文件。这是限定范围的声明材料缺口，不是“没有许可证”或侵权结论；不凭空补造版权年份或权利声明。

16 个生成解析器中，19 个词法运行时函数分别与原 Jison Lex 代码逐字一致，共 304 次函数比较。16 个 `parseError` 函数与 Jison 0.4.18 原码逐字一致；16 个 `parse` 入口的完整语法树在应用上游的错误恢复移除及 token stack 变换后对应。检查仅静态解析源码，没有执行生成器或被审阅代码。语法树对应不等于原文逐字一致，函数比较次数也不是 source map 条目数。

前轮 1,090 个条目中的 1,073 个完整源码字节对应结论保持不变；17 个生成条目仍未完整重建。本轮补齐其中 16 个解析器的运行时来源，不声称语法动作、表、生成配置或最终 bundle 可重现。完整重建是另外一种证据，不把它自动增加为一切许可核对的前置条件。

声明包新增精确来源、原 MIT 标签与作者元数据，保持私有草稿。完整原生声明未找到的限制继续披露；未向候选安装通知，也未关闭 LIC-03。

### 历史内容复审的实际覆盖

828 项历史材料已有来源记录：204 项 r-doc 上游原样，24 项改写副本对应 5 种差异，其余 600 项去重为 494 份正文。独立复审完整阅读全部 5 种差异，但未重新通读这 24 份改写文件，不能将其归为项目原创。

494 份正文中，本轮全文读 49 份、局部读 2 份，443 份未作语义审阅，分别对应 49、2、549 条路径。85 份优先标记内容中，48 份全文读、1 份局部读、36 份未读。完整阅读台账保存实际行段与同摘要副本，不以关键词扫描、路径分类或 Git 作者代替内容审查。

| 已识别的具体例外 | 本轮证据与后续动作 |
| --- | --- |
| NATS 源码分析 | 包含源码和英文注释；文内 v2.10.x 与较新 main 表述混用。继续从保全材料及固定上游核对片段、版本、LICENSE/NOTICE，不把当前 main 冒认为原始来源 |
| gnet 走查及 byteslice 引用 | 文内短提交已解出 `fd7ce4bef97582920c174b56a96cbbda7a5bcd42`；固定归档、完整 Apache-2.0 文本及相关源码版权头已取得并经复审。部分连续行对应不证明所有摘录逐字相同；下一步逐段保留出处与通知，不能再列为“缺少许可、请用户授权” |
| nbio 分析 | 包含 README 引用和源码片段，尚无固定版本；继续恢复版本并核对相应许可与出处，伪代码不冒认为上游逐字原文 |
| comqtt 提取方案 | 混有 comqtt、gnet、Go 示例及项目适配代码；历史 MIT/mochi 声明不是独立核实回执。按具体片段核对版本和归属，不整篇判为原创或外部复制 |
| Go GC 成本分析 | 已读部分有 Go GC guide 英文引文；继续核对引文及适用复用条款，局部阅读未覆盖中间正文 |
| Core 历史 package 元数据 | ISC 字段与既有 Core AGPL 来源记录存在适用范围待厘清项。不能据一个 package 字段替换整仓许可证，也不能先断定二者必然冲突违法 |
| Core 迁移计划 | 有 Core 前后代码示例、拟定 NOTICE 及模板句子；示例保留 Core 来源，拟定声明不当作已获得的许可，不因一句模板扩大到整篇外部归属 |

已全文阅读的若干 AI/Skill 调研材料以摘要、链接和本地判断为主，没有观察到整篇外部文章或完整 Skill 源码复制；没有逐句与外部原文比对，不据此认定原创或权利闭环。旧参考 checkout 当前不可用，继续使用保全材料及固定上游来源。以上是具体例外与覆盖缺口，不提出整体删除 600 项正文。

### 后续执行及必要的人工作用

先自主核对 NATS、nbio、comqtt 的来源版本和 Go 引文，补 gnet 摘录归属，并继续阅读优先标记长文与源码分析资料。对来源检索给出限定范围与结果；查不到时形成具体未决片段或处置选择，不能无限空查或改成要求用户为开源上游重新授权。

此前文章/图片 7 项与历史素材 18 项共 25 文件的“仅私有保留”方案，以及连同 Mermaid 的 26 项备选，仍未实施。需要权利人决定的是这些可审阅材料的公开范围，以及有明确自有范围后是否采用新的开放许可；已有组件授权继续复用。自有内容暂不增加开放许可不自动等于不能公开，远程推送另按用户授权处理。

产品未变，沿用 M4A 精确候选的全新 clone 验证；本轮补做证据摘要、文件身份、来源保全、三份文档及独立复审检查。公开 HOLD、原仓与私有备份保持；GitHub 推送、托管 CI、真实联调、小黄反馈及 M5 切换均未完成。

### 正文例外证据摘要

完整声明草稿、逐文件阅读台账和原始归档保留在当前 Goal 私有证据中。这里只记录摘要，不复制本机定位器。独立证据及精确文档 Head 复审以 Goal 回执为准。

| 证据标识 | SHA-256 |
| --- | --- |
| `content-exceptions-progress.md` | `8276962451dd7deb4a92127507382db558915bb67f7eddf3b5f5c3715fed4ff0` |
| `MERMAID-NOTICES-source-supplement.draft.md` | `778e796a63972c4faeec30c194e41b0d717c712148d74e88e414730cc73a51d4` |
| `audit/generator-notice-evidence.json` | `e734216393e20f9a86cc457dc0e1a874a606e2dc6120f950cd8bcb72df38d032` |
| `audit/generator-runtime-comparison.json` | `c04ed1db863c04dcb523971792df0e56e14567dccf9faca4d5684550b91950de` |
| `audit/jison-lex-history-match.json` | `938510bb424b8975e3e1b7f8d4bb90d83c7a68d600f22df1117653f9c9d217bd` |
| `audit/gnet/provenance.json` | `b468d1702ef2447223f33521c8817d9959d30eee2a9a30a7d524cd0c16e8c46c` |
| `audit/gnet/source-snippet-evidence.json` | `5d408c7711b77d618fb7472b946b71f5275464251b9dab2e163fb1a00b784de6` |
| `review/history-content-review.md` | `a2168a8d5c520ebae31e06f2c9861ed3e9e1a152bcbb4b96b0b97809865cea79` |
| `review/history-content-review.json` | `13073da167b513f2b45fa3eee881f343f36be4c64694cbdc3bf96121169af6c9` |

## M4B 历史片段来源与勘误进展

本轮保留 M4A 候选 `25e9cf73d91c566cc59161fc218ad9739da2f07a` 的 3,964 个文件及原有私有备份；只推进来源核对、历史阅读、声明和勘误草案。当前 Todo 保持 open / in_flight_continuation，公开 HOLD 未解除。

### 固定对照来源

| 对照材料 | 固定提交 | 上游声明 |
| --- | --- | --- |
| NATS，历史文档最近提交日期前的默认分支快照 | `c1ca29ba0d040c259bcf23e1a7451c38609ab14a` | Apache-2.0，The NATS Authors |
| NATS v2.10.26，代表性版本补充对照 | `14fa949d74bcb07a8cce684a59ce39262bd4b00b` | Apache-2.0 |
| nbio，同样按文档日期选取 | `8ad01e6461af31d510d27b679f670c3dddbe417c` | MIT，2020 lesismal |
| comqtt，同样按文档日期选取 | `c68325655488ddc2d9f798c218a68e88fd08d913` | 根 MIT；packets 文件保留 mochi-mqtt / mochi-co SPDX 归属 |
| gnet，延续上一轮文档版本线索 | `fd7ce4bef97582920c174b56a96cbbda7a5bcd42` | Apache-2.0，The Gnet Authors |

这些是可复核的对照快照，不等于已恢复 NATS、nbio、comqtt 文档作者当时的源码 checkout。三份候选文档与保留 iot-doc 源文件逐字一致，最近修改提交为 `f18be053ac8458b6246d533e768f325744bd8611`。本地选定依赖元数据没有给出所需服务端源码版本；NATS 客户端版本不能代替服务端身份。

62 个 Go 代码块（NATS 28、nbio 13、comqtt 21）中，15 个整块 token 对应、9 个局部对应、38 个未达到匹配阈值。比对省略注释和分号，要求至少 16 个 token / 60 个字面字符；结果不是原始字节相等、语义验证或原创证明。未匹配可包含本地示意、短片段和改写，不能据此推断没有来源。

comqtt Put 短函数另经定向 token 比对对应 `mqtt/mempool/bufpool.go:51–54`，在通用字面字符阈值之外，不改变 62 块统计。两段 byteslice 摘录对应 gnet 固定来源；保留摘录和改写属性。独立声明草案保存上游许可正文与 packets 头部，尚未装入候选，不产生新的项目授权。

### Go 引文、Core 元数据与历史准确性

Go GC 旧来源 URL 返回 404；固定网站源码证实两句引文中第二句已经改写。来源记录分别保存站点文字与代码条款。nbio 固定源码使用 mutex，与历史文档的无锁说法不符；所谓原注释也有改写。net/http 初始化示例在一个固定 server.go 中未找到同样声明，不据此声称搜遍历史。以上形成单独勘误草案，历史材料保持原样。

Core 的 ISC package.json 元数据随 SSH 部署工具在 `1407948792f1d741b1a81ecc700b46c270b346a0` 引入，根 AGPL LICENSE 在引入前后、早期记录和保留 Head 的 SHA-256 相同：`20b067f86de375aae6db0f283ab2e65de24d537733b89bd58432c101259d84cf`。这不支持整个 Core 改授 ISC；保留两种记录及适用范围疑问，不替权利人改写许可。

### 独立正文复审与剩余工作

本轮新增全文读 23 份（269,765 bytes），并补完 Go GC 文档。600 条历史路径去重后的 494 份正文累计为 73 份全文读、1 份局部读、420 份未语义审阅；路径计数为 73 / 1 / 526。85 份优先材料已有 72 份全文读，余 13 份（319,576 bytes）未读，主要为 3 个大型实施计划和 10 个试验规格变体。gnet 走查仍局部读，前轮 5 种 r-doc 改写差异结论沿用，不扩成 24 份完整文件审阅。

新增具体来源线索是 go-zero v1.8.4 配置声明；共享字段名本身不是复制证明，应只比对文档指向的声明与原始文件头。Go GC 内部数值矛盾及 Raw 文档厂商强结论作为历史研究的证据限制保留，不转为产品故障或版权判定。所有未读项保持显式未读，不将全文阅读无差别设为新的发布必过条件，也不宣称已完成全部历史审计。

下一步按阅读台账处理剩余 13 份优先材料，核对 go-zero 固定版本声明；将已定位片段、许可及勘误形成具体候选修改方案。对照来源已经取得，不再把此事项笼统记为“全部源码未知”。原始 checkout 未恢复与实际复用归属分别记录，避免无边界搜索。现有 25 文件私有保留方案及含 Mermaid 的 26 文件备选仍只是提案；不实施公开范围删减、不授新许可、不推送、不删除旧仓。

上一轮 Jison Lex / Mermaid 证据沿用，生成器局部对应不升级成完整重建，也不重复无边界许可正文查找。M4A 产品测试结果因候选不变沿用，本轮只对证据、文档和源状态做针对性验证。GitHub 托管 CI、运行时整合和小黄真人反馈仍未验证，且不据本轮补证宣称 Goal 完成。

### 片段核对证据摘要

完整固定源码、原始许可、声明与勘误草案、阅读台账保留在 Goal 私有证据中。本表只记录脱敏标识及内容摘要；独立复审以同一摘要集合和精确文档 Head 的回执为准。

| 证据标识 | SHA-256 |
| --- | --- |
| `snippet-provenance-progress.md` | `8202472f837ec3d1b8a9e4ba34d0f95940469737c1be54b48bae5b559f75c3f5` |
| `HISTORICAL-ERRATA.draft.md` | `a92d947d6a866e9ecc28e7c1b4560046799f2fc6afc47a03e5401cd20a29013f` |
| `HISTORICAL-SNIPPET-NOTICES.draft.md` | `a04303222362b85607e8c6545adb6a0284e8791b61818274526bf95602ecf87e` |
| `audit/local-source-search.json` | `baf714abd7605a9000475b3fd36edcbc9cb6899f5048d9528c1bdeea50d5d4f8` |
| `audit/snippet-correspondence.json` | `09556d47bf61ebe938c0c9097f827582315ac139e9d1e6005b5becbb50adde89` |
| `audit/targeted-source-checks.json` | `1f8d2c48764bfacfaace2f1b893828b364269d9b4387336275e1b7bdb5cb4bba` |
| `audit/core-license-scope.json` | `c7934d3eb37fb7b039de9f5cbd490443cc1e3211efab3bd114a065a6ee560309` |
| `audit/go-guide/quote-evidence.json` | `86c4b0cd1bb2c3f48d057f6c1d664091d597aa41a40f4317d081e1761b135653` |
| `audit/go-guide/http-example-evidence.json` | `918056aeadfba32f21a3a8e4b4656336ea48524e41819cddb37a9904d8bdfbfd` |
| `audit/go-guide/web-fetch.json` | `4130a9a1f3c54a285d7728f2496009c0b3fb0a64b0870e9c3ce47bec47f183b1` |
| `review/history-expansion-review.json` | `c73284f9b08467f9d068fb64703d748d9829c5a1d779f0cf40546cb660ccee2a` |
| `review/history-expansion-review.md` | `6b509bd6a6e0e96c6717ba696512722b0a0b8618cdc2929317ff5ddef6372585` |
| `audit/nats-comparison/provenance.json` | `9e0e9ab78d5bcc304ea73bdd71c9fc6312f4208bd903219151b2b282d9b50519` |
| `audit/nats-v21026/provenance.json` | `3543b487d4b4fb641f4da3d7eef949d3ab86b2ad5310bf20bebdddf89fb3bc70` |
| `audit/nbio-comparison/provenance.json` | `9ce4258e986d14359ffa55696a165d9a64bc4b1a45f2d5e4767f9102b427fe02` |
| `audit/comqtt-comparison/provenance.json` | `d62cbfa18d6ba972d8a93c36d253534bf50dcbadda800a956e51c99c1194e712` |

## M4B 优先历史复审与声明旁注演练

本轮在同一来源核对 Todo 内推进；原 M4A `25e9cf73d91c566cc59161fc218ad9739da2f07a`、其 3,964 个文件及私有备份保留。新增的独立本地演练提交为 `2e8150ea5919fbe64368cc92c12c3b3f2b706a4b`，只叠加归属、原许可、勘误和导航，不替换 M4A，不解除公开 HOLD。

### 优先阅读完成的范围

剩余 13 份优先正文全部实际读完，共 319,576 bytes、6,111 行。494 份独立历史正文累计为 86 份全文读、1 份局部读、407 份未语义审阅；对应 600 条路径为 86 / 1 / 513。85 份优先组全部全文，未读优先项为 0。gnet 走查仍局部读，前轮 5 种 r-doc 差异仍不等于 24 份完整文件复审。

本轮已读材料没有新增已确认的第三方逐字复制；这不是原创或完整权利证明。十份 OTA/Kafka 变体指向项目固定历史源码和回执，不能变成当前批准或运行结果；三份大型计划中的顶部覆盖关系、生成就绪与运行时实现的区别继续保留。其他 407 份不一概升级为全文阅读门禁，也不宣称通过全历史事实或权属审计。

新增具体方法问题：Proto 计划用两次 Git 状态列表相同作为生成不漂移证据，不能证明文件内容字节相同。该项已进入阅读回执，后继应在旁注中建议比较内容清单及摘要；本轮先前生成的 11 文件旁注尚未包含此项。三处 Superpowers 短开头延续既有模板来源线索，只需限定比较这些短片段，不扩成三份正文均为第三方复制。

### go-zero 固定声明比对

历史文档明确参考 v1.8.4，已固定到上游提交 `0f2b589d4db9e7db2251c1a95fae867493058bb3`，取得原 MIT LICENSE（2022 zeromicro）及两组配置源码。DevServer 共有 8 个同名同类型字段，标签均改为 YAML，并增加 2 个本地字段；Telemetry 共有 7 个同名同类型字段，标签同样改变，省略上游 Disabled。它们是明确参考后的本地设计，不是声明原样复制，也没有证明语义等价。字段相同本身不能判定受保护表达复制。

原文所称 Things v1.8.4 不能作为当前事实：M4A 的 Things 依赖为 v1.9.4，SaaS 和 Share 仍为 v1.8.4。已在拟加入的旁注中区分历史参考版本与当前依赖。[固定上游声明](https://github.com/zeromicro/go-zero/blob/0f2b589d4db9e7db2251c1a95fae867493058bb3/internal/devserver/config.go)、[原许可](https://github.com/zeromicro/go-zero/blob/0f2b589d4db9e7db2251c1a95fae867493058bb3/LICENSE) 可复核，不据此推断原始 checkout 或整个文档的权利范围。

### 11 文件旁注的具体落位与演练

- 新增 `docs/vendor/historical-snippets/`：NATS、nbio、comqtt、gnet、go-zero 的 5 份原 LICENSE，以及来源表、NOTICE 和导航，共 8 文件。保留 NATS 文件头、mochi SPDX、gnet 归属和 Go 网站文字来源；不以统一许可证覆盖历史全文。
- 新增 `docs/wiki/legacy/ERRATA.md`：记录已定位的 nbio、Go GC、net/http 示意、厂商分析和 go-zero 版本旁注，历史正文原样保留。
- 仅向现有 `docs/wiki/legacy/README.md` 与 `LICENSES.md` 追加入口。合计新增 9 文件、修改 2 文件、删除 0 文件；业务代码、历史正文和 `PUBLICATION_POLICY.json` 内容均不变。

上述 11 文件先形成带原文件及目标文件摘要的覆盖清单，再应用到独立 clone 的工作树。`make knowledge-check` 与 `make publication-check` 通过，完整可达对象扫描覆盖 10 commits、1,307 trees、3,484 blobs，38 个已有例外，0 未解决项。`make publication-ready` 保持非零退出，因为公开 HOLD 仍在；这是已验证的阻止发布行为，不能写成发布就绪。新工作树首次缺少 gitleaks 的前置失败保留，安装并核验项目固定工具后才通过扫描。

本轮是文档旁注演练，未重跑全部产品 Profile；业务代码与 M4A 完全相同，原产品验证保持绑定原 Head。正式最终候选仍需完整验证及独立复审，不能把本轮聚焦检查当成整个平台验收。无 GitHub 推送、托管 CI、旧仓删除或真人验收。

### 后继收敛

来源工作已从泛化未知项收敛到具体材料：补入 Proto 方法旁注并处理三处短模板线索，汇合既有 Mermaid 许可草稿与这套历史旁注，核对组装后的归属和链接。原始 checkout 未恢复作为明确限制保留，不反复泛化搜索。25 文件私有保留方案与包含 Mermaid 的 26 文件备选仍未实施；后续向权利人呈现具体公开范围选择。确认自有内容无需为迁移强制新增 MIT 或 Apache；既有组件许可与 Share 授权继续保留。

当前 Todo 保持进行中，Goal 未完成。小黄昵称显示验证仍等待真实反馈，与本轮本地资料工作分别跟踪。

### 历史复审与旁注演练证据摘要

完整回执、固定归档及11文件覆盖草稿保留在 Goal 私有证据。下表保存本轮18项输入摘要；独立复审与精确文档提交绑定结果见 Goal 回执。

| 证据标识 | SHA-256 |
| --- | --- |
| `audit/go-zero/provenance.json` | `dbce09d903827b869cb7fb3adcb7ae0c5db7b4faca0aedb43f041f00987b83d6` |
| `audit/gozero-declarations.json` | `32f0fbbacc0e862404f1866db32c88677f137f0c1a279b10a8a54b49efe2b635` |
| `candidate-overlay.json` | `62a3b1cbd6f6683f00b431c3be94b67ce369333ae62b3eeda4a9fc73c2545cc5` |
| `proposed-files/LICENSES.md` | `8cbaae6002edf68a22cbe86b9fa9dd8591673a38f8dc382755aaddbcab0da2f3` |
| `proposed-files/docs/vendor/historical-snippets/NOTICE.md` | `83af4a12bba55da3272df3d009c2481bd7e43c7f07d728b51f2099f4a69d52af` |
| `proposed-files/docs/vendor/historical-snippets/README.md` | `529607e4dc36241503d3700640da728ef4775d3062e3e3762f3667cab9a100ab` |
| `proposed-files/docs/vendor/historical-snippets/licenses/NATS-LICENSE.txt` | `c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4` |
| `proposed-files/docs/vendor/historical-snippets/licenses/comqtt-LICENSE.txt` | `bfa2aa54489d516c7009775973bec1079b9f45da1a376014c01762b7bef6f5bb` |
| `proposed-files/docs/vendor/historical-snippets/licenses/gnet-LICENSE.txt` | `4f81ecf128a847a1808c98e379be68fb3e4cd798fc86f61504a15e44d9d9cad0` |
| `proposed-files/docs/vendor/historical-snippets/licenses/go-zero-LICENSE.txt` | `a31424b15cfcbb5f0269b4e9ba4ee6f24fdb4c0a991a9c7891fb184078c4eb98` |
| `proposed-files/docs/vendor/historical-snippets/licenses/nbio-LICENSE.txt` | `6777f2154d58ba63a644c41fa631cfa82071860c6f33ce89a6d557689d727d1e` |
| `proposed-files/docs/vendor/historical-snippets/sources.json` | `2c5677a3465a8642c857203d27765b0ab4fc00c937e12b5f1195b9f7481b27bd` |
| `proposed-files/docs/wiki/legacy/ERRATA.md` | `9e9863856671a74d5c9bdb98480ab5f0656e6ed7452598a296a0a6e972ca02e4` |
| `proposed-files/docs/wiki/legacy/README.md` | `515725eb274644d8ddb790d3ff17e461c5feb0b485ec35965ef1d0a4d965b878` |
| `history-gozero-progress.md` | `10bc10202dc5bfaacff153b8d3f21a280b85ec42ca3067a127685d823c013fd7` |
| `review/history-priority-review.json` | `42509e46adf7ec0237e4bce0a5038f3d692a3db2ba1a4874128bbe2581f5cb60` |
| `review/history-priority-review.md` | `7409616bbbf3e8e4f1b19fb6d9b389fb9fdc5eb035af0e6e36cd65100d087679` |
| `audit/rehearsal-validation.json` | `d3d46c0d56918ba353286dddd527a0e49b442dc80ea276a62eb9e4226afe68e9` |

## M4B 声明合并与具体公开范围

Goal ming-iot-knowledge-pilot；同一来源核对 Todo 继续。本轮建立独立演练 Head `302170043c6ce8dd81ff2a9826156d941ea7deb7`，保留原 M4A `25e9cf73d91c566cc59161fc218ad9739da2f07a` 和上一轮演练 `2e8150ea5919fbe64368cc92c12c3b3f2b706a4b`。累计17文件：新增15、修改2、删除0；相对上一演练为新增6、修改3。只涉及声明、原许可、勘误和导航，业务与历史正文未变，PUBLICATION_POLICY未变。公开HOLD继续。

## 本轮完成

Proto历史生成检查的旁注已落到候选：相同Git状态不能推出相同内容。临时最小仓库复现状态相同但内容SHA不同；不是产品生成器验证。建议比较完整生成范围的路径、类型/模式、内容/链接目标及越界写入，包含新增、未跟踪及忽略的生成文件。

三处Superpowers开头与固定[b36e0829c6d0140e93cfef2ca599b1b07d4a7797](https://github.com/obra/superpowers/blob/b36e0829c6d0140e93cfef2ca599b1b07d4a7797/skills/writing-plans/SKILL.md)对照：Scene原样对应；业务地图仅行内代码格式改变；Proto中文改写并加入本项目约束。原MIT许可保留2025 Jesse Vincent声明，不推定整份计划来自第三方、不假称恢复原始安装版本、不执行历史段落内指令。

Mermaid声明包包含70个精确pnpm标识的71份许可/声明文本，以及Mermaid、parser、Jison各一份，总74原文块。所有块的字节偏移、长度与摘要已核对；70依赖归档重新验SHA。Jison Lex README中的MIT标签另在README保留，不把它计作完整许可文本。原bundle和内嵌声明、roughjs补丁均保留。沿用1073源码字节对应、17生成条目未重建的范围，不重复源码审计，也不据此宣布完全可重现。

Jison Lex完整原生版权/许可正文在之前限定检索中未定位，该具体剩余项显式保留；不改写成没有许可、侵权或强制全量重建。此前固定来源与证据摘要通过复核，不新增无边界搜索。

## 验证与限制

本候选知识检查、发布技术检查、完整verify-pr均通过；verify-pr耗时185.13秒。发布扫描覆盖11提交、1315树、3493blob与11归档叶，38发现均有既有审阅例外，未解决0；publication-ready按HOLD退出2。完整检查在新的独立工作树运行，**不是全新clone验收**；最终公开文件选择后仍需重新构造历史并做全新clone完整验证。未运行GitHub托管CI、生产/设备/数据库联调或真人验收。

原M4A3964文件身份、旧演练Head和干净状态、旧源HEAD/refs及15工作树状态保留。源仓检查范围是HEAD/refs/status，不冒称重新全量重哈希原始备份。本轮无需增加永久测试；临时方法反例属于迁移证据。

85份优先历史材料仍全读；累计86全文、1局部、407未全文读。本轮仅处理具体例外，不提高全文覆盖数字，也不把407全部升为新门禁。完整来源/演练独立复审和三文档差异复审分别保存，本正文不替代其回执。

## 最小公开范围决定

私有证据 `monorepo-attribution-assembly-20261015` 中的具体25文件清单及其JSON逐项绑定原M4A摘要：外部文章及六图7文件、历史素材/产品例子18文件。建议原件私有保留并改公开摘要/去向导航；未执行排除。Mermaid为独立选择，保留为25文件方案，额外暂不分发则26文件方案。本轮已完成声明装配路线，精确剩余声明缺口列明。

项目288项范围及600条历史正文路径不宣称全为原创；228条r-doc原样/派生副本和已知模板/摘录排除在整仓原创声明之外。对实际有权公开的自有内容可暂不新增开放许可，不能把新MIT/Apache作为迁移前提；Share已有授权不重复索取。范围决定不授权推送，最终GitHub动作仍按用户明确边界。

下一步核对最终公开范围决定和确切第三方例外，依据选择构造新的独立候选历史，纳入最新规划与声明，完成全新clone、全历史扫描和独立复审。没有新证据不重复优先正文阅读、Jison全历史空查或继续制造同类声明草稿。小黄对通知R2的真实反馈单独待回传，不阻塞可自主工作；M5仍未启动。

### 固定证据摘要

| 证据 | SHA-256 |
| --- | --- |
| `audit/template-comparison.json` | `f99e36372332662aa0a9631e4d84a4107617a7d7aae93390f21ade5b0a2ebcee` |
| `audit/superpowers/provenance.json` | `dbabf2bfa49218717664f08465126c726631daa210503fd896c8e9d9bf4e3e5a` |
| `audit/superpowers/writing-plans.md` | `48508f44bbfd7d24b029fbf3a314f3cd14c9615599059366e922f47b8dc08cf2` |
| `audit/superpowers/LICENSE` | `a37e0e9697144819e1d965176ac4ae5bc3fa02d11e7812036bbcadf6dafe2400` |
| `audit/proto-counterexample.json` | `bc755fa635dc00ce3e9aa63075dab480e5aa568013a7ea64223b757db0f1057d` |
| `audit/notice-assembly.json` | `b7876c32bd1c3d5aeee5c6c1ca240dc7b37345755f983834018377cc0f8c9c00` |
| `audit/public-scope-options.json` | `f55ff07deef59a70bd899351744b193b527718a4b004ea3792cdfa24ae05e8f0` |
| `candidate-overlay.json` | `b407cb4eb06fa47a0b9463daa530c113108afaf7ed9467fc89d4761ff7daea4b` |
| `audit/rehearsal-checks.json` | `d6fd647bbe898da8fe8038e7efa08fc893ccd14a744fe324b10787ea78d78f5d` |
| `audit/focused-validation.json` | `fed7d9bc021284dcc93fc659c655f3100e7f04abd9b5ff3615fadd8a142cea26` |
| `audit/source-preservation.json` | `ea95d702fd67a04817593181f7ae822e300d3040d0a4d214ec208e6e48c764fd` |
| `public-scope-decision.md` | `a1d26f92a3408ec7aa003df6a2974ef808cd6d6509934d35f90fc8958d5d2bda` |
| `attribution-assembly-progress.md` | `1303a2480755ab91344d91f5828fc96d9391fdaf6812401a0fccedbac5d992c7` |

## M4C 用户确认25文件范围

用户于2026-10-15选择25文件方案，提案SHA-256为 `a1d26f92a3408ec7aa003df6a2974ef808cd6d6509934d35f90fc8958d5d2bda`。原件私有保留，文章替换为自写摘要，其余24文件不进入新候选；Mermaid保留已整理声明及已知限制。现有候选、源历史和私有备份保留，不推送GitHub或删除旧仓。

另建独立Git历史，导入已审业务、工具和声明，只调整明确的公开范围和对应导航。`monorepo-public25-20261015` 保存原件身份、候选差异、全新clone检查和独立复审；该包的精确版本回执为本阶段验收依据。此前0.4.7及更早段落是历史状态，不再作为“范围未决定”的依据。
