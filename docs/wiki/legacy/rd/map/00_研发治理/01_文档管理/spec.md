---
document_kind: spec
status: current
module: map/00_研发治理/01_文档管理
---

# 文档管理模块主规格

> **初版主规格，当前修订尚未合入生效。** 本文独立定义文档管理模块的完整要求，是首次实施及后续 Change 的规格基线，不是某个 Change 完成后才生成的交付总结。现有元数据格式中的 `status: current` 不代表已部署、已验收或远端已合入；候选合入前不覆盖目标分支的现行规则。

## 模块目的与边界

让成员和 Agent 按业务模块找到“现在应该怎样工作”，并能追溯一次改变的动机、实施计划、验证及合入事实，而不依赖某个聊天窗口或个人电脑。

本模块管理文档的归属、身份、导航、生命周期、评审与验证记录。它不管理业务服务运行，不替代 Gitee 的 Issue/PR/CI，不实现 LoopX 调度，也不保存原始对话、凭据或可再生日志。

## 主规格与 Change 的关系

正常设计顺序是：**模块初版 `spec.md` → 首次实施或后续变更的 Change/Plan → 实施与验证 → 生效后维护主规格**。

- 新模块先在 `spec.md` 写清目标、边界、完整行为及验收要求，不能等到 Change 完成后才倒推模块定义。
- `changes/` 不负责定义模块本身。每次交付引用主规格的相关锚点，说明本次要实现或改变什么；Plan、Review、Evidence 留在对应 Change 内。
- 首次实施可以通过 FEATURE Change 交付主规格要求，但必须区分“规格已定义”和“能力已实现”，不能假设已有上线基线。
- 后续 Change 基于现行主规格提出增量，生效后将增量合回主规格，并在受影响段落保留来源指针。读者不必拼接多个 Change 才能理解模块。
- 本文初版不把尚未生效的 Change 标成规范来源。具体交付进度、PR、阶段依赖和编写历史留在 [模块入口](./README.md) 与对应 Hub，不混入主规格正文。

<a id="document-authority"></a>
## 1. 当前权威与读取顺序

1. 从 `map/README.md` 进入模块 `README.md`，再读取模块 `spec.md` 和本次 Change Hub。
2. 模块主规格承载完整规范：初版先定义待实现要求，实施状态单独记录；正式生效后维护完整现行行为。Change 承载首次实施范围或一次增量，计划、评审、证据不能自行改写规范。
3. 判断是否生效必须同时核对目标分支、模块入口和 Change 生效记录；仅有文件名、状态字段或一次成功校验不构成生效证明。
4. 实施某次 Change 时，读取其批准范围、Plan 和覆盖锚点，并扫描 `map/**/changes/*/change.md` 的 `affected_modules`；不能只检查当前目录内的 Change。
5. 同一主题不得存在两份优先级不明的当前规格。发现冲突时保留现行权威，记录冲突对象和依据，交有权限的人裁决。
6. map 内缺少入口或规格时，如实记录缺口。批准范围足够时可以起草候选；缺失既有需求时请用户补充，不自行恢复旧目录内容，也不把缺文件理解为新建模块的授权。

`meta/DOC_REGISTRY.yaml` 不是业务地图的必经入口，也不是 LoopX 本地状态文件。本模块的读取和交付流程不依赖它查找正文；既有索引不在本次删除或重定向。LoopX 如需登记来源，另行遵循其原生流程，不把候选登记成已生效权威。

<a id="delivery-identity"></a>
## 2. 交付身份与职责

- 一次准备实施的交付关联一个真实 Gitee Delivery Issue。身份为 `<仓库名>#<Issue编号>`，例如 `ming-iot-rd-docs#IKEOFY`；Issue URL 中的仓库和编号必须一致。
- 一个 Delivery ID 只有一个主 Change 目录。跨模块变更选择主归属，通过 `affected_modules` 和其他模块入口反向索引，不复制正文。
- Issue 管目标、范围、验收和协作状态；Change 定义完整语义增量；Plan 组织实施步骤；PR 承载一个仓库的具体候选。
- 方案讨论或研究不等于实施授权；不因新增文档就自动新建 LoopX Goal、Todo 或 User Gate。
- 使用 LoopX 执行时，沿用仓库要求的获批 Todo、精确仓库身份、有界写入范围和独立 worktree。`Plan Task ≠ Agent Todo`，Task 是 Todo 内的检查点，不逐项制造审批。

<a id="module-layout"></a>
## 3. 目录与文件职责

```text
map/<业务域>/<功能模块>/
├── README.md
├── spec.md
├── adr/
├── references/
└── changes/<TYPE>-<仓库名>-<Issue编号>_<名称>/
    ├── README.md
    ├── change.md
    ├── plan.md
    ├── review.md
    └── evidence.md
```

| 对象 | 唯一职责 |
|---|---|
| 模块 `README.md` | 模块边界、当前主规格入口、活动与历史 Change 索引 |
| 模块 `spec.md` | 初版完整要求及后续完整现行规范，不堆叠实施步骤或历史补丁正文 |
| `adr/` | 已生效、长期且难逆的决策、取舍和替代关系 |
| `references/` | 多个 Change 可复用的依据及来源 |
| Change `README.md` | 本次交付的导航中心（Hub）与逐仓状态 |
| `change.md` | 动机、目标、非目标、变更前后行为、覆盖范围和生效条件 |
| `plan.md` | 实施、验证、仓库顺序及中断恢复步骤 |
| `review.md` | 独立审查结论及其精确候选版本 |
| `evidence.md` | 稳定验证摘要与原始证据指针 |

正式正文使用中文。模块根不设 `plan.md`；单次交付材料留在 Change 内。尚未执行的 Review 在 Hub 标记“待评审”，不生成虚构通过的评审文件。

TYPE 只有 `FEATURE`（新增或显著扩展）、`PATCH`（局部方案修订）、`BUGFIX`（修复承诺与实现的偏差）、`SUNSET`（能力退役）。它们是 Change 类型，不是四种额外文档目录或主规格。

正式文件使用 YAML Front Matter。所有正式正文包含 `document_kind`、`status`；导航 README 可省略。

| 文档类型 | 其他必填字段 |
|---|---|
| spec | module |
| change | change_type、delivery_id、issue、hub、target_module、affected_repositories、affected_modules、target_anchors |
| plan | delivery_id、issue、hub、change_spec、affected_repositories |
| review / evidence | delivery_id、issue、hub、repository_heads |
| adr | module |
| reference | module、source |

Skill 的字段说明和 `tools/map_docs.py` 是这份规格的操作说明及校验实现，不反过来充当模块需求来源。发现实现与规格不一致时，记录差异并按授权处理，不能仅凭代码现状改写规范。

<a id="change-lifecycle"></a>
## 4. Change 生命周期

正常路径为 `draft → approved → implementing → verifying → effective`。

- `draft`：讨论中，不授权实施。
- `approved`：语义方案获批，按获批范围和 Plan 推进。
- `implementing`：产生实现候选。
- `verifying`：固定候选进行验证与独立 Review，或等待跨仓依赖及合入。
- `effective`：所需代码和文档已合入，主规格及来源链已同步。
- `rejected`：方案被放弃，保留原因，不改当前主规格。
- `superseded`：被后续决策替代，必须指向替代对象。

状态变化有可检查的依据。人的批准确定语义和风险取舍；测试、Review、远端合入各自提供相应事实，不能互相代替。

初版主规格应在首次实施之前定义；其存在不构成完成声明。激活候选可以在分支中准备经核验的主规格修订和 `effective` 状态，供只读激活检查与 Review 使用，但合入前仍是候选。

<a id="four-way-navigation"></a>
## 5. Issue、Change、Plan、PR 四向导航

Issue、Change、Plan 和 PR 都直接指向同一个 Change Hub；Hub 反向列出真实 Issue、Change、Plan，以及各仓库的 PR、Review 和 Evidence。

- 仓库内 Markdown 链接相对当前文件，元数据中的文档路径相对仓库根；不用个人绝对路径或本地 worktree URL 作为团队入口。
- 跨仓或远端入口使用可共享的真实仓库链接，候选与当前分支必须可区分。
- 尚未创建的对象写明“未创建/待评审/未验证”，不伪造 URL 或链接不存在的文件。
- 导航必须是读者实际能点击的链接。没有被正文使用的引用定义不算导航；但其失效目标仍需要修复。

Issue 反链工具 `tools/gitee_issue.py link` 当前只读取，不自动覆盖远端正文或标题。缺失/过期时输出待补导航块，退出 2，交有权限的人补齐；回读确认才报告 `linked=true`。退出 1 为错误，退出 0 为当次验证通过，均不保证远端以后不变。

<a id="bidirectional-pointers"></a>
## 6. 主规格与增量的双向追溯

可被改变的规范段落使用稳定 HTML 锚点，例如 `document-authority`；改标题不随意改锚点。

Change 指定目标模块、`target_anchors`、变更前行为、变更后行为及生效条件；正文应可独立理解。正常流程先定义初版主规格，再引用真实锚点。`__module_root__` 仅是工具对缺少主规格的历史/过渡候选的兼容占位，不是先建 Change、后补主规格的推荐流程；激活前必须改为真实锚点。

激活时，在同一个文档候选中完成：更新主规格为完整的新行为、在相关段落附近链接来源 Change、更新覆盖与生效记录、把模块索引中的该 Change 移到历史区。后续替代继续保留来源链，旧正文由 Change 与 Git 历史保存，不在主规格堆删除线。

`approved`、`implementing`、`verifying` 的重叠覆盖须检查冲突；发现同模块/锚点竞争时由人决定顺序或合并方案，不由 Agent 静默选胜者。

<a id="review-and-evidence"></a>
## 7. Review 与 Evidence

Review 与 Evidence 按仓库记录 `repository`、`base`、`exact_head`、结论及稳定证据引用；提交版本使用完整 SHA，不能只写可移动分支名。

- Reviewer 独立核对语义、实现和验证；在批准的 Review/Evidence 范围内写结论，不自行修改实现或合并。
- 一个 Change 固定使用其 `review.md`、`evidence.md`，多仓和多轮候选在文件内区分，不把旧 Head 的通过套到新 Head。
- 修改候选后重跑受影响验证，并交回独立复审。尚未取得结论，Hub 明确待审。
- Evidence 区分执行命令、结果、环境限制、已知失败和未运行项；本地测试通过不能写成云端 CI 或远端链接已通过。
- 正文仅保存脱敏且供人阅读的稳定结果；原始机器日志留在获准的私有证据存储，引用不得泄露凭据。
- LoopX Todo 回执只保留简短结论与稳定指针，不复制整份评审或日志；评审接棒依据同一候选的真实回执。

<a id="agent-skill-discovery"></a>
## 8. 人、Agent 与 Skill

人决定需求、范围、取舍与风险；Agent 在授权范围内生成完整语义文档、执行验证和处理反馈；`map-delivery` 是这套文档流程的统一入口，辅助研究或评审 Skill 的结果汇入相同 Change。

canonical Skill 源码位于 `git:gitee.com/ming-industry/ai-native` 的 `skills/map-delivery/`，由团队 AI 仓版本化；文档仓保存来源契约，不再跟踪技能源码。本地初始化通过 `tools/setup_workspace_skills.py` 为工作区、产品仓和文档仓的 Codex/Claude 建立指向同一来源的消费入口，不把机器专用注册提交到 Git。正式注册从已合入的主工作树执行，候选 worktree 只做临时测试。

这一来源归属修订对应 [独立工程迁移规格](../../../specs/2026-10-04-map-delivery-ai-native-source-design.md)；旧 IKEOFY 的 Change/Plan 保留其当时依据，不由来源迁移宣布整个模块已激活。

Skill 不扫描旧目录或 Catalog 来补未迁移模块，不授予文件读取或远端操作权限，也不是强制门禁。宿主没有发现 Skill 时，成员仍可按模块文档及确定性命令工作；不能据此免除检查。

<a id="deterministic-gates"></a>
## 9. 确定性校验

在文档仓 worktree 根运行：

```bash
python3 tools/map_docs.py check --docs-root .
```

检查包括目录、元数据、Delivery 唯一性、路径安全、文件和锚点、实际导航及活动覆盖冲突。默认比较 `origin/master` 与候选的 merge-base；对新增/修改的 effective Change 或其主规格段落变化，同时执行激活门禁。CI 应提供正确目标分支基线，不能用候选 HEAD 作为基线自证无变化。

`init` 只生成骨架，不能代写语义或 Review；`activate` 只读检查已准备好的激活候选，不执行合并，不修改状态，不证明远端事实。没有发生变化的历史 effective Change 不要求每次重新激活。

授权内的文档错误修正后重跑；权威或覆盖冲突交人裁决；疑似工具缺陷保留 Head、命令、退出码和最小复现，不靠放宽规则绕过失败。

Hook 与 CI 调用相同检查入口。实际是否阻止推送或合入，取决于 Hook 安装、CI 配置及分支保护是否启用；仅存在脚本或工作流文件不代表平台门禁已生效。

<a id="multi-repository-delivery"></a>
## 10. 多仓库交付与合入

文档仓承载研发事实，产品仓承载代码及稳定项目契约；各仓有自己的候选分支、worktree、验证、PR 和精确 Head。文档 worktree 放在本仓库忽略的 `.worktrees/` 内，LoopX 本地运行状态不提交。

跨仓 Change 在 `affected_repositories` 和 Hub 中完整列出参与仓库；Plan 声明合入顺序、部分合入的可观察状态和恢复动作。前置阶段未完成时保持 `verifying`，不能因为单仓通过就宣布全部生效。

激活 Hub 的逐仓表记录 `role`、`repository`、`pr`、`base`、`exact_head`、`status`、`merge_revision`。前置行必须有已回读的合并事实和完整 SHA；当前激活 PR 可尚未合入。不得省略 Plan 依赖来绕过检查。

普通范围内的验证后提交、推送候选和 Draft PR 遵循仓库授权规则；`tools/gitee_pr.py` 一次只处理一个仓库。默认由有权限的人在 Gitee 合并；要求 Agent 合并时，须有绑定精确 PR、Head、目标及方式的授权。候选更新或 Draft PR 创建均不等于合并批准。

<a id="migration-and-recovery"></a>
## 11. 生效边界与恢复

未迁移模块由用户手工组织迁移。map-delivery 不承担旧文档盘点、恢复或清理，也不为已授权的模块主规格起草引入迁移前置条件。

模块正式激活需逐项核实获批交付的实际依赖、独立 Review、Evidence、必要远端反链和合入事实，并同步当前入口及来源链。调整 Plan 的阶段或验收范围须显式记录批准，不能以初版主规格已经存在代替验收。

失败或中断时保持最后已生效规则，把候选、缺口和真实逐仓状态留在 Hub；取消则保留拒绝原因，替代则记录后继。恢复从目标分支及 Hub 的已核实事实继续，不依赖旧会话记忆，也不删除历史来消除冲突。

## 12. 模块验收要求

1. 从模块入口可先读完整主规格，再按需进入 Change，理解模块无需依赖聊天历史或拼接变更正文。
2. 每次交付具有唯一 Issue 身份和主 Change，Issue/Change/Plan/PR 可经 Hub 相互定位。
3. 新模块先定义初版规格；后续变更明确覆盖锚点，生效后主规格、来源链及模块索引一致。
4. 确定性检查拒绝无效元数据、断链、无效目标锚点和活动覆盖冲突；Hook/CI 使用相同入口，实际启用状态可核实。
5. 独立 Review 和 Evidence 绑定真实仓库与精确候选，失败、未执行和待合入不冒充通过。
6. 跨仓依赖及部分合入状态可追踪；中断后从仓库事实恢复，不依赖特定模型、宿主或个人会话。
