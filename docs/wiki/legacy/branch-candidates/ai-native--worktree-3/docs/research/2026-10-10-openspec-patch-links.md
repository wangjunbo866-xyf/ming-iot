# OpenSpec 主规格与 Patch：定位、反向索引和同步边界

日期：2026-10-10。固定 OpenSpec `v1.14.1`，源码 SHA `87c3595ace6a2e22957f39ebe5b74c2f2316e0cb`。沿用 research 方法，核读官方文档及固定源码；未安装或执行 OpenSpec。本文解释事实与提出阅读约定，不修改业务规格。

## 结论

OpenSpec 用 **capability 相对路径 + Requirement 名字**关联差量与主规格；它不是靠“Patch 覆盖某个 Feature 文档”的手写声明来合并。默认模板没有保证双向 Markdown 链接，主规格 JSON 也没有关联 Change 列表。团队可以从原生查询和差量生成反向索引，但那是团队补充。

最需要防止的二义性是混淆三种事实：主规格当前写了什么、某个 Patch 当时提议了什么、哪些代码已经部署。Change 在 active 目录不代表尚未同步；归档也不代表已批准或上线。

## 原生如何定位

1. 差量 `changes/patch-example/specs/things-telemetry-query/spec.md` 对应主规格 `specs/things-telemetry-query/spec.md`；源码递归发现 delta，并将相同相对 capability 路径映射到主规格。不是根据 Change 的 `patch-` 前缀或 proposal 标题推断目标。[findSpecUpdates](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/specs-apply.ts#L118)
2. `MODIFIED` 按工具规范化后的 Requirement 名匹配现行块，并替换为提供的完整新块；标题/场景缺失会触发相应检查。它不是按自然语言“意思相似”寻找目标。[MODIFIED 实现](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/specs-apply.ts#L527)
3. Proposal 的 `Capabilities` 分 New/Modified，两者写 capability path；Modified 要求使用现有精确路径，`Impact` 描述代码/API/依赖影响。模板示例是反引号包裹的标识符和说明，**没有强制生成可点击正向链接，更没有固化当时主规格 revision**。[proposal 模板](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/schemas/spec-driven/templates/proposal.md)
4. `skip_specs: true` 的实现修复可没有 delta，且不应伪造 Modified Capabilities。因此仅扫描 delta 的索引会漏掉这种 Patch；如需从能力追踪全部修复，应在 proposal/Impact 或团队关系记录中明确受影响能力及链接。[metadata](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/change-metadata/schema.ts)、[proposal 模板](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/schemas/spec-driven/templates/proposal.md)

第 4 项最后一句是团队建议，不是 OpenSpec 已有的自动关联能力；不要把未知 metadata 字段当成受支持的新接口。

## 哪些真实查询能辅助生成反向索引

| 已核实命令/表面 | 实际输出 | 不能推断的内容 |
| --- | --- | --- |
| `openspec list --specs --json` | `specs[]` 的 `id`、`requirementCount`，以及适用时的 `root` | 不返回某个能力的相关 Changes。[list.ts](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/list.ts#L273) |
| `openspec show <capability> --type spec --json` | `id/title/overview/requirementCount/requirements/metadata`，适用时有 `root` | 默认输出没有 `activeChanges`、`archivedChanges` 或反向链接；这里的 metadata version 也不能当 Patch 基线 commit。[spec.ts](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/commands/spec.ts#L129) |
| `openspec list --all --json`；或 `--archived` | 此版本 CLI 明确注册两选项；条目含 `name/completedTasks/totalTasks/lastModified/status`，包含归档范围时有 `archived` | `status` 来自任务计数，不是批准、规格同步或发布状态。CLI 文档选项表可能未列全，以固定源码为准。[CLI 注册](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/cli/index.ts#L391)、[JSON 组装](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/list.ts#L197) |
| `openspec show <active-change> --type change --json` | `id/title/deltaCount/deltas`；delta schema 有 `spec/operation/description`，可有 requirement(s)/rename；`--diff` 可补 MODIFIED 对当前主规格的差异 | 不是固定旧基线的历史 diff；缺 proposal 的 Change 也可能无法得到这份 JSON。读取归档历史可直接使用 archive 目录内保留的 proposal/delta，不假定普通 active show 自动遍历归档。[change.ts](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/commands/change.ts#L93)、[Delta schema](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/schemas/change.schema.ts#L10) |

以上足以作为生成 `capability → active/archived Change → operation/Requirement` 索引的输入，但仍需团队编写生成逻辑并处理缺失、无 delta、重命名及读取失败。本文没有实现索引，也没有假定 OpenSpec 存在“查询相关 Patch”的专用命令。

## sync、归档及并行冲突：两个执行机制不能混说

- **`/opsx:sync` 是宿主 Agent 的语义合并工作流。** 它要求读取主规格和 delta、保留未涉及内容，并明确说明同步后 Change 仍 active；“重复运行应同结果”是工作流指引，不是全局数学保证。[sync 源码](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/templates/workflows/sync-specs.ts#L248)
- **CLI `openspec archive` 的 `specs-apply` 是程序化块处理。** 相同 ADDED/MODIFIED 内容、已完成的删除/重命名有 no-op 处理；同时检查重名、目标缺失、操作冲突和场景遗漏。这些检查不能证明两个改动的语义兼容。[specs-apply](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/specs-apply.ts#L439)
- **不能宣称三方合并。** 核读的 Change metadata 没有基线 commit/requirement hash；程序化 MODIFIED 读取当前主块，然后设为 delta 新块，未与“作者起笔时的旧块”三方比较。[metadata](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/change-metadata/schema.ts#L23)、[替换代码](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/specs-apply.ts#L527)
- **静态风险示例（未运行反例）**：A、B 修改同名 requirement，保留相同场景名但给出不同超时值；A 后 B 的块替换可能把值改成 B 的值，再处理 A 又可能改回 A。场景名字齐全不证明新正文包含别人的语义变化，所以“多次同步安全”只在输入/基线和操作相容等前提下成立。
- bulk archive 工作流能发现选中 Changes 触及同一 capability，并让 Agent 检查实现、选择顺序；其中“较新覆盖较旧”也是 Agent 工作流策略，不是保存双方修改的三方合并证明。[bulk archive](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/templates/workflows/bulk-archive-change.ts#L126)
- **取消不同于失败恢复。** CLI archive 有本次操作快照、失败恢复及并发改动保护；这不等于成功 sync 之后删掉/取消 Change 会自动反向修改主规格。核读表面没有这种取消即撤销机制；应依据 Git 版本和后续变更明确恢复。[archive 恢复实现](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/archive.ts#L1137)

官方允许随时编辑规划文件，不提供批准阶段锁；因此批准、代码 merge、部署必须另读各自事实，不能由 active/archive 目录位置推出。[Editing Changes](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/editing-changes.md)

## rename、removed、retire 对导航的影响

`RENAMED` 表达 Requirement 的 FROM/TO，不是自动移动 capability 目录；同一 delta 又修改正文时，MODIFIED 使用新名字。REMOVED 使当前主规格不再有那个要求，历史链接应指向固定旧 revision；不能期待旧 Requirement 锚点继续出现在当前全文。整个 capability 删除最后一个要求时，还要满足 `retire_capabilities: true` 与内容保护检查。[处理顺序及重命名](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/specs-apply.ts#L304)、[退役说明](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/cli.md#openspec-archive)

退役会检查整个 spec 中未被标题、Purpose、要求和场景解释的额外内容；在主规格追加 `Related Changes` 之类手写正文可能触发保护，不能假设它不影响工具行为。[退役内容检查](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/specs-apply.ts#L165)

## 建议的阅读规则与薄导航（团队补充，未实施）

1. **看当前行为**：从“业务域 → 功能”模块入口进入唯一主 spec；不把 active Patch 当自动叠加层。入口区分规划仓当前文本、指定候选及目标环境已生效版本。
2. **看一次修改**：Patch 链接 capability 当前入口，同时记录当时基线的不可变仓库 revision/路径；前者回答“现在在哪里”，后者回答“当时改了什么”。可另链接结果 revision，不能用移动分支链接冒充历史基线。
3. **看所有关联修改**：在独立模块入口生成反向索引，保留来源 revision/生成时刻；按能力列出 Change、delta 操作与 Requirement，补录无 delta 的实现修复关系。无需每条 Requirement 堆手写“被哪些 Patch 覆盖”。
4. **避免双写**：索引是可重建投影，不是另一份规格或执行状态。同步、批准、合并、发布各列只使用已有可验证来源；来源不足明确 `unknown`，不从 active/archived 或 checkbox 猜测。
5. **处理并行修改**：同一 requirement 被多个活动 Change 修改时，明确顺序并基于最新主规格复核完整块；记录语义协调结果。默认工具检查不替代这一步。主规格如需回到地图，只保留必要的稳定入口；完整反向清单放在独立导航，避免额外正文影响 retire。

## 核查与提交记录

本文仅新增研究文件，不写业务 Change、不执行同步/归档、不修改主树票据。所有 OpenSpec 能力结论来自固定源码静态核读，尚无本机 CLI/宿主运行验证。

提交前按 README 检查：`python3 -m unittest discover -s tests -v`，52 项通过（2.124 秒）；`python3 tools/team_skills.py validate` 返回 `validate: OK (map-delivery)`。这些配置仓基线检查不证明 OpenSpec 合并正确性或本文建议已实现。
