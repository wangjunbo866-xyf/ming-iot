---
document_kind: change
change_type: feature
status: verifying
delivery_id: ming-iot-rd-docs#IKEOFY
issue: https://gitee.com/ming-industry/ming-iot-rd-docs/issues/IKEOFY
hub: map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/README.md
target_module: map/00_研发治理/01_文档管理
affected_repositories:
  - git:gitee.com/ming-industry/ming-iot-rd-docs
  - git:gitee.com/ming-industry/ming-iot
affected_modules:
  - map/00_研发治理/01_文档管理
target_anchors:
  - document-authority
  - delivery-identity
  - module-layout
  - change-lifecycle
  - four-way-navigation
  - bidirectional-pointers
  - review-and-evidence
  - agent-skill-discovery
  - deterministic-gates
  - multi-repository-delivery
  - migration-and-recovery
---

# Change Spec：业务地图文档管理

## 2026-09-13 主规格层级修订

本模块的完整要求由 [模块主规格](../../spec.md) 定义。本文承载 `ming-iot-rd-docs#IKEOFY` 的首次落地范围、实施取舍和交付条件，不充当模块主规格；后续新 Change 也应先读取主规格，再声明本次增量。

启动期本文与 Plan 先于主规格编写。保留下文历史设计及已有 Review 的真实依据，不倒改历史为“当时已有主规格”。从本次修订起，完整规则读取主规格；本文下文重复的目录和行为描述作为首次交付的设计记录。首次落地的变更前状态是旧顶层文档组织及缺少 map 交付能力，不是已经实现的主规格。

本次修正不改变 verifying 状态，不表示工具、跨仓推广或迁移已完成。未迁移模块由用户后续手工组织，本文旧迁移设想不授权本轮执行。下文与本节不一致的编写顺序以本节为准。

## 文档状态与权威边界

本文是 `ming-iot-rd-docs#IKEOFY` 已于 2026-09-10 通过评审的 Change Spec。Approved 状态确定实施目标并允许编写实施计划；在配套规则、工具、门禁与迁移完成并经 PR 合入前，仓库根目录 `AGENTS.md` 和现有 `specs/`、`plans/`、`reviews/`、`evidence/` 仍是当前权威。

本文使用的是候选 `map/` 元数据格式。现有 `tools/check_delivery_metadata.py` 只扫描顶层交付目录，不会验证本文。校验命令即使返回成功，也不能证明 `map/` 结构或元数据有效。消除这段启动期盲区是后续实施的验收项。

根 `AGENTS.md` 目前还把 Delivery ID 限定为 `ming-iot#<Issue ident>`。本设计把它推广为 `<Gitee 仓库名>#<Issue ident>`，以支持文档仓治理 Issue 和未来多个代码仓；在规则正式修改前，本文中的 `ming-iot-rd-docs#IKEOFY` 也是待审候选格式。

交付导航见同目录 [README.md](./README.md)。

## 背景

当前交付文档按文档类型集中放在顶层目录。随着业务模块、多人并行开发和跨仓库交付增加，同一功能的主规格、增量规格、计划、Issue、PR、Review 与 Evidence 分散在不同位置。人或 Agent 持有其中一个对象时，需要依赖命名猜测或全文检索才能拼回上下文；新增补丁也难以判断它覆盖主规格的哪一段。

目标结构以“业务域 → 功能模块”为第一导航维度，以 Delivery Change 为该模块的增量历史。模块主规格保存当前完整事实，Change 保存为什么以及怎样改变这些事实。

## 目标

1. 持有 Delivery Issue、Change Spec、实施 Plan 或 PR 中任意一个，都能直接找到其余对象。
2. Spec、Plan、Patch、Bugfix 与 Sunset 按业务域和功能模块组织，避免全仓同类型文件堆积。
3. 主规格与 Change 使用双向指针，读者从任一侧进入都能判断当前生效内容和历史原因。
4. 让多名成员和不同 Coding Agent 使用同一套可版本化规则，同时用确定性程序检查结构和链接。
5. 允许 `ming-iot` 未来拆成多个代码仓库，而研发文档、产品和测试方案继续集中在 `ming-iot-rd-docs` 协作。

## 不在本次设计范围内

- 不把 `changes/` 做成逐 commit 的 Changelog；Git 继续记录代码级时间线。
- 不把完整对话、Agent 运行日志或 LoopX 内部状态复制进文档仓库。
- 不要求一次迁移全部历史文档。
- 不在本 Draft 中实现 Skill、脚本、Git hook、CI 或文档迁移。
- 不恢复 OpenSpec 的全局 `specs/`、全局 `changes/` 布局。

## 目标目录模型

```text
map/
└── <业务域>/
    └── <功能模块>/
        ├── README.md
        ├── spec.md
        ├── adr/
        ├── references/
        └── changes/
            └── <TYPE>-<Delivery-ID路径形式>_<名称>/
                ├── README.md
                ├── change.md
                ├── plan.md
                ├── review.md
                └── evidence.md
```

### 业务域与功能模块

- 业务域是稳定的产品或治理边界，例如 `00_研发治理`、`01_saas`、`02_things`。
- 功能模块是具有独立业务边界和当前规格的能力，例如 `01_文档管理`。编号只用于稳定排序，不表达发布顺序。
- 功能模块目录是语义归属。涉及多个模块的 Change 选择一个主模块落盘，并在元数据的 `affected_modules` 中列出全部受影响模块；其他模块的 `README.md` 建立反向索引。
- 模块根目录不设置 `plan.md`。路线、里程碑和跨 Change 优先级属于 Issue、Proposal 或项目管理系统；可执行计划只属于一次具体 Change。
- `adr/` 保存该模块已经生效、长期且难逆的决策；`references/` 保存可被多个 Change 复用的依据。只服务一次交付的评审和证据留在该 Change 内，避免同一材料出现两份权威副本。

### 模块文件职责

| 文件 | 职责 | 是否描述当前事实 |
|---|---|---|
| `README.md` | 模块边界、主规格入口、活动与历史 Change 索引 | 只做导航 |
| `spec.md` | 模块已生效行为的完整规格 | 是 |
| `adr/` | 模块长期决策、取舍和替代关系 | 只描述已生效决策 |
| `references/` | 可复用的外部依据、研究摘要和来源指针 | 否 |
| `changes/.../README.md` | 一次交付的 Issue/Spec/Plan/PR/Review/Evidence 导航中心 | 只做导航 |
| `changes/.../change.md` | 相对主规格的完整语义增量、取舍与覆盖范围 | 在生效前是候选 |
| `changes/.../plan.md` | 该 Change 的实施步骤、仓库顺序、验证与回滚 | 否 |
| `changes/.../review.md` | 绑定精确候选 Head 的独立评审结论 | 否 |
| `changes/.../evidence.md` | 稳定验证摘要和原始证据指针 | 否 |

一次 Change 固定使用一个 `review.md` 和一个 `evidence.md`。涉及多个仓库或多个候选 Head 时，文件内按仓库列出 `repository`、`base`、`exact_head`、结论和证据，不在顶层目录再复制同一份 Review 或 Evidence。

### Change 类型和命名

允许的目录类型为：

| 类型 | 含义 |
|---|---|
| `FEATURE` | 新能力或已有能力的显著扩展 |
| `PATCH` | 在既有方向内替换或修正局部方案 |
| `BUGFIX` | 恢复已经承诺但实现偏离的行为 |
| `SUNSET` | 退役能力、接口、数据或兼容行为 |

Delivery ID 使用可跨会话定位的真实 Gitee Issue 身份，例如 `ming-iot-rd-docs#IKEOFY`。由于 `#` 不适合作为路径分隔语义的一部分，目录名把它转换为 `-`：

```text
FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理
```

文件元数据和正文仍保留原始 Delivery ID，不得用路径形式替代。一个 Delivery ID 只对应一个主 Change 目录；若同一交付影响多个模块，通过 `affected_modules` 和模块索引表达，不复制 Change 文档。

## 四向导航契约

每个 Change 的 `README.md` 是固定导航中心。四类对象必须形成以下链路：

| 从哪里进入 | 必须包含什么 | 如何找到其余对象 |
|---|---|---|
| Delivery Issue | Delivery ID、Change `README.md` 仓库链接 | 先进入导航中心 |
| `change.md` | Delivery ID、Issue URL、`hub` 路径 | 先回到同目录导航中心 |
| `plan.md` | 同一 Delivery ID、`hub` 路径 | 先回到同目录导航中心 |
| PR | Delivery ID、Change `README.md` 链接、仓库与 Head | 先进入导航中心 |

导航中心分别记录研发文档 PR 和所有产品仓库 PR，不能只提供一个含糊的 `PR` 字段。跨仓库交付不是原子提交：`plan.md` 必须声明合并顺序、每一步的可观察状态和中断后的恢复动作。

外部 Issue 与 PR 中的反向链接由创建或更新命令写入，CI 只能校验仓库内文档和 PR 元数据，不能假设 Agent 曾在对话中记住关联关系。若权限不足以更新 Issue，导航中心应明确标记“反向链接未写入”，并让门禁失败或进入需人工处理的已知状态。

## 主规格与 Change 的双向指针

### 稳定锚点

模块 `spec.md` 中可被增量覆盖的规范段落必须具有稳定锚点。锚点标识不随标题措辞改变，例如：

```html
<a id="project-refine-goals-api"></a>
## 项目目标细化接口
```

Change 在 `change.md` 的“覆盖范围”中记录：目标模块、锚点、变更前行为、变更后行为及生效条件。Change 必须可独立阅读，不能要求读者先阅读旧主规格才能理解新行为。

### 生效前

- Draft、Approved、Implementing 或 Verifying 状态的 Change 只出现在模块 `README.md` 的活动索引中。
- 未生效 Change 不改写 `spec.md` 的规范正文，也不把候选行为描述成当前事实。
- 同一模块同一锚点存在多个活动 Change 时，校验器报告冲突，由人决定合并、排序或拒绝；Agent 不自行选择谁覆盖谁。

### 生效时

当实现、Review 与 Evidence 满足验收条件后，在同一个研发文档候选中完成：

1. 把模块 `spec.md` 对应锚点更新为新的完整当前行为。
2. 在该段附近添加简短来源指针，链接到已生效 Change。
3. 把 Change 的覆盖记录标记为 Effective，并记录相关 PR 与精确 Head。
4. 把模块 `README.md` 中的 Change 从活动索引移到历史索引。

主规格保留清晰的当前文本和来源指针，不长期堆积大段删除线。被替代的完整旧行为保存在 Change 的变更前描述和 Git 历史中。若新 Change 再次覆盖同一锚点，新 Change 同时指向主规格锚点和此前生效 Change，形成可追溯链。

### 示例

Change 中：

```markdown
## 覆盖范围

| 主规格锚点 | 变更前 | 变更后 | 状态 |
|---|---|---|---|
| `project-refine-goals-api` | SSE 直连模型服务 | 改走通用 TPP 代理 | Effective |
```

主规格中：

```markdown
<a id="project-refine-goals-api"></a>
## 项目目标细化接口

接口通过通用 TPP 代理调用模型服务。

> 来源：对应 PATCH Change Hub 中的 `change.md`
```

## 生命周期

Change 使用有限状态：

```text
Draft → Approved → Implementing → Verifying → Effective
   └──────────────→ Rejected
Approved/Effective → Superseded
```

- `Draft`：讨论中，不授权实施。
- `Approved`：语义方案已批准，可以编写或执行获批 Plan。
- `Implementing`：至少一个实现仓库正在产生候选变更。
- `Verifying`：实现候选已固定到精确 Head，正在验证或评审。
- `Effective`：所需代码和文档均已合入，模块主规格已经刷新。
- `Rejected`：候选被放弃，主规格保持不变。
- `Superseded`：批准或已生效内容被后续决策替代，必须指向替代 Change。

状态迁移必须由可检查事件支持，不能仅由 Agent 根据语气推断。语义批准、冲突裁决和风险接受由人决定；实现、验证及合入事实由工具回读。

## 多人并行与跨仓库交付

- 每个 Delivery 使用独立目录，常规新增文档不会争用全局 Changelog 文件。
- 模块 `README.md` 和 `spec.md` 是共享热点。PR 合入前必须基于目标分支重新检查活动 Change、锚点冲突和当前规格；发生冲突时重新生成候选并再次评审。
- Change 的 `affected_repositories` 列出所有文档仓和代码仓；导航中心为每个仓库分别记录 PR、base、exact head 和状态。
- 跨仓库合入期间，Change 保持 `Verifying`，导航中心显示每个仓库的实际状态。只有全部生效条件满足后才刷新主规格并进入 `Effective`。
- 中途失败时按 `plan.md` 的恢复步骤继续或回滚。任何代码已合入而文档尚未激活的窗口，都必须在导航中心显式记录，不能提前把候选写成当前事实。

## 人、Skill 与确定性门禁的职责

### 人负责

- 确认业务边界、当前行为、验收标准和取舍。
- 批准 Change Spec 与实施 Plan。
- 裁决多个 Change 覆盖同一锚点、跨模块归属和风险接受。

### `map-delivery` Skill 负责

- 从 Issue、对话和仓库现状收集信息，起草或更新 Change、Plan、Review 与 Evidence。
- 在编辑前读取模块主规格、活动 Change 和导航中心，避免把候选内容冒充当前事实。
- 引导补齐覆盖范围、反向链接、跨仓库顺序和恢复步骤。
- 调用确定性命令并解释结果。

Skill 是面向大模型的版本化工作说明，不承担 Git 回调，也不构成强制门禁。它可以由用户显式调用，也可以由 Coding Agent 根据 `SKILL.md` 的 description 在相关任务中自动选择。无论如何触发，最终只有一个 `map-delivery` 流程可以写正式交付文档；研究、评审、调试等辅助 Skill 只提供输入或独立评审结论，不能各自生成一套权威 Spec/Plan。

### 程序和 CI 负责

实施阶段提供无模型依赖的仓库脚本，例如：

```text
python tools/map_docs.py init ...
python tools/map_docs.py check ...
python tools/map_docs.py activate ...
```

脚本负责目录命名、元数据枚举、Delivery ID 唯一性、相对路径、文件存在性、锚点、活动覆盖冲突和仓库内双向链接。Git hook 提供本地快速反馈，CI 对 PR 重新执行同一检查并作为合入门禁。需要查询 Gitee Issue/PR 的检查在具备凭据的 CI 中执行；离线检查不得伪装成已经验证外部反向链接。

模型是否遵守 `AGENTS.md` 或 Skill 是概率性的；只有退出码会被 Git hook、CI 或分支保护确定性执行。因此“流程门禁”专指脚本失败会阻止提交、推送或合入的链路，不把提示词约束称为硬门禁。

## 多 Coding Agent 的 Skill 发现

权威 Skill 计划随文档仓版本化在：

```text
ming-iot-rd-docs/.agents/skills/map-delivery/
```

Agent 不会可靠地扫描任意兄弟仓库。工作区初始化脚本 `tools/setup_workspace_skills.py` 应为各宿主建立指向同一权威目录的本地注册：

```text
workspace-iot-platform/.agents/skills/map-delivery
ming-iot/.agents/skills/map-delivery
workspace-iot-platform/.claude/skills/map-delivery
ming-iot/.claude/skills/map-delivery
```

支持 Agent Skills 标准的宿主直接读取同一 `SKILL.md`；宿主需要不同入口时只增加薄适配文件，不复制业务规则。工作区和产品仓 `AGENTS.md` 还应记录权威路径作为未自动发现时的回退入口。

这些工作区注册是每位成员执行初始化脚本生成的本地配置，不作为兄弟仓之间的 Git 依赖提交。Skill 缺失只影响辅助写作体验；`tools/map_docs.py` 和 CI 仍独立执行相同结构规则，使成员使用不同 Coding Agent 时不会绕过仓库契约。

## 迁移与权威切换

迁移采用按模块切换，而不是长期保留两套当前规格：

1. **启动期**：本文已 Approved，`map/` 仍仅作试点，现有顶层目录继续权威。
2. **能力就绪**：实现并验证元数据规则、CLI、测试、Git hook、CI、Skill 和工作区初始化；更新根 `AGENTS.md` 与 `README.md`。
3. **模块迁移**：逐个模块盘点现有当前文档，建立模块 `spec.md` 与 Change 索引；同一 PR 将旧入口标记为 Superseded 并指向新入口。未迁移模块继续使用旧目录。
4. **默认切换**：所有新 Delivery 只能进入 `map/`；旧顶层目录只保存尚未迁移的当前文档或历史材料。
5. **收尾**：最后一个模块迁移后，旧顶层交付目录停止承载当前权威，归档规则和目录说明同步更新。

权威目录以模块为单位只有一个。迁移清单必须能回答每个旧文档是“仍为当前”“已迁移”“历史保留”还是“被替代”，不能让 Agent 在两份无优先级说明的 Spec 之间自行选择。

## 异常与恢复

- Change 被拒绝或取消：标记 `Rejected`，保留原因与 Issue 链接，不改主规格。
- Delivery ID 重复或目录位置错误：CLI 拒绝生成或校验失败，由人确认主归属后移动或合并。
- 主规格锚点不存在或已经变化：激活失败；先基于目标分支修订覆盖范围并重新评审。
- 多个活动 Change 覆盖同一锚点：校验失败；由人确定先后关系或合并方案。
- 外部 Issue/PR 暂不可访问：仓库内检查照常运行，外部链接状态记为未验证；需要外部完整性时不得进入 `Effective`。
- 跨仓库只完成一部分：保持 `Verifying` 并记录实际合入状态，按 Plan 恢复；不更新主规格为最终状态。
- Agent 未加载 Skill：CLI、hook 和 CI 仍能阻止结构不合法的候选进入受保护分支。

## 实施验收标准

后续实施全部满足时，本 Change 才能进入 `Effective`：

1. `map/` 的目录、文件职责、Change 类型、状态和元数据有唯一机器可读定义。
2. `tools/map_docs.py check` 覆盖 `map/`，并有针对合法结构、断链、重复 Delivery ID、无效锚点、覆盖冲突和路径逃逸的测试。
3. Git hook 与 CI 调用同一校验入口；CI 失败能够阻止 PR 合入。
4. Change 初始化后，从 Issue、Change Spec、Plan 和 PR 任一对象可在一步内进入导航中心，再找到其余对象；缺失对象明确显示状态而不是生成坏链接。
5. 模块主规格只描述当前生效行为；待处理 Change 只进入活动索引；激活操作同时写入新当前文本和来源指针。
6. `map-delivery` 只有一份权威 `SKILL.md`，工作区初始化可让 Codex、Claude Code 及其他支持 Agent Skills 的宿主使用它；从工作区根目录或任一子项目启动都有明确发现或回退路径。
7. 选择一个现有模块完成迁移演练，证明旧入口不会与 `map/` 同时冒充当前权威，并记录可逆恢复步骤。
8. 根 `AGENTS.md`、根 `README.md`、PR 模板与文档校验说明统一使用最终规则，不再把顶层类型目录描述为所有新交付的默认位置。
9. Review 与 Evidence 绑定精确候选 Head；跨仓库交付分别记录真实 PR 和 Head，不宣称原子提交。
10. 本 Change 导航中心补齐最终 Plan、PR、Review 与 Evidence，并由人确认文档管理规则可以正式切换。

## 实施计划已确定的细节

以下实施选择已在 [plan.md](./plan.md) 中按仓库现状排定，不改变本设计的核心语义：

- `tools/gitee_issue.py link` 只从环境变量读取令牌。按 PR !9 R1 修订：在未验证远端原子条件更新能力前，禁止自动 PATCH Issue 正文/标题；生成待补托管块，或回读验证已有导航。缺失/过期块以退出 2 明确交回人工补齐，不声称 linked。
- 现有 `pre-push` 在保留 remote 白名单的基础上运行 map 快速检查；Gitee CI 运行全量测试和同一 map 检查。
- 第一个迁移演练对象是 `00_研发治理/01_文档管理`，并显式映射六份现有 Spec/Plan。
