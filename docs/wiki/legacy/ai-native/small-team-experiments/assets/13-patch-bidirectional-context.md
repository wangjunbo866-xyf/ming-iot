# Patch 双向关联：原生 OpenSpec 与薄导航各负责什么

日期：2026-10-10。性质：机制解释和接入建议，不是已实现的索引工具或已切换的业务规范。延续已选 OpenSpec＋薄导航、业务域 → 功能路线，本轮不重新选型。

## 1. 结论

OpenSpec 原生提供 **Patch 差量 → 主规格要求的结构关联**，以及把差量整合回主规格的机制；本次核查的 v1.14.1 不会自动生成天猫式双向 Markdown 指针。主规格 → 相关 Patch 的可发现性，可以在已有业务地图中补一份生成式反向索引。

需要分别解决三个问题：

1. **定位**：这个 Patch 修改哪个能力、哪些要求？
2. **发现**：从功能/主规格进入，怎样看到涉及它的 Patch？
3. **解释**：这些内容是当前文档基线、候选目标、已纳入的改动，还是历史/撤销记录？

链接能帮助前两项；第三项还依赖所读版本、整合记录和项目已有验收事实。不能仅凭“有双向链接”或目录位置裁定哪段要求已生效。

## 2. 原生的正向结构关联

示例路径（不是实际已创建的业务文件）：

```text
主规格：openspec/specs/telemetry-ingest/spec.md
补丁：  openspec/changes/patch-temperature-decimal/specs/telemetry-ingest/spec.md
```

补丁与主规格的 capability 相对路径一致；差量操作与 Requirement 标题定位具体要求。例如主规格现行约定整数温度，本次希望支持小数：

```markdown
## MODIFIED Requirements

### Requirement: Temperature value
系统 SHALL 接受整数或小数温度值。

#### Scenario: Integer temperature
- WHEN 设备上报整数温度
- THEN 按既有语义接收该值

#### Scenario: Decimal temperature
- WHEN 设备上报小数温度
- THEN 按新约定接收该值
```

上例只是差量片段。真实修改必须保留该 Requirement 仍适用的所有既有场景，而非只留下展示用场景；数值类型和行为也不是已批准的 IoT 需求。

原生关联可以理解为“文档根＋capability 路径＋要求名＋操作”。这是解析与定位规则，不等于生成了可点击链接，更没有隐含固定的原始基线版本。Requirement 重命名用 RENAMED 明确 FROM/TO，不能把任意改标题当成无影响的排版。

## 3. 最小补齐方式：关系只声明一次，反向视图生成

建议在 Patch 的原生 proposal 中引用主规格和适用基线，行为有变时沿用 Capabilities；实现修复而行为不变时在 Impact 说明关联能力，不伪造 Modified Capabilities。普通正文可以补链接和 revision，不需要往 `.openspec.yaml` 发明一个原生不认识的字段。

示例说明字段（建议，不是 OpenSpec 新增 schema）：

```text
主规格当前入口：telemetry-ingest/spec.md
本次依据：文档仓的固定 revision ＋ 同一 spec 路径
影响范围：Requirement “Temperature value”
变更意图：整数 → 整数或小数
适用性：本 Change 的候选目标；正式纳入与发布按现有交付记录判断
```

“当前入口”用于找现行全文，“固定基线”用于解释当时究竟改了什么。尤其是归档多年后，不能只链接一直变化的主分支就假装读到了原始依据。初次新增能力则明确“基线不存在”，不要补造旧 spec。

反向阅读链建议如下：

```text
功能入口 / 主规格入口
          │
          ▼
业务地图的能力页（生成相关 Change 索引）
          │
          ▼
Patch proposal + delta ────────► 当前主规格 / 固定基线
```

功能入口原本就在 map 中，可直接承载索引。为了从直接打开的主 spec 也能找回地图，接入时可在已有 Purpose 中保留简短导航链接，或由 AGENTS 的统一读取规则按 capability ID 查询模块入口；具体锚点和格式随接入验证确定。不要给每条 Requirement 手工维护一串“被谁覆盖”的历史。

如果确实要求一跳直达每个 Patch，可以生成主规格中的 Related Changes 区块，但这比独立能力页更侵入原生格式；尤其独立新增章节可能触发完整退役时的额外内容保护。第一版建议把动态索引留在 map，主 spec 只需稳定的找路方式。

## 4. 索引如何生成，而不变成第二套事实

候选索引每行至少展示：Change ID、实际文件入口、受影响能力/要求、操作、活动或归档位置、所依据来源 revision，以及已有的文档整合/交付证据链接。

固定 v1.14.1 已有的读取入口可以提供生成材料：

```bash
openspec list --all --json
openspec show patch-temperature-decimal --type change --json --deltas-only
```

上面只是命令示意，未在本项目运行。选用独立 store 时要带相同的已解析 store，不能跨不同规划根拼接来源。列表发现 Change，活动 Change JSON 中的 `deltas[].spec` 等信息帮助反查相关能力；归档内容可直接读取归档目录保留的 proposal/delta，不假定普通 show 会自动查询归档。主 spec 的 show 输出本身没有完整的反向 Change 索引。

生成过程建议：

1. 从声明的文档根读取活动及归档 Change，解析实际存在的差量与原生内容；按 capability 建反向关系。
2. 对 `skip_specs` 的修复/技术调整，读取 proposal 中明确的受影响能力链接；没有 delta 不等于不影响任何功能。无法确定时列为待关联，不自行猜测。
3. 由真实文件位置生成链接，归档或移动后重建，避免手写相对深度导致失效。Change ID 只用于同一声明文档根内定位，跨根还需根身份。
4. 文档是否已整合，读取当前内容和已记录的合并/review revision。内容一致只能说明一致，不能证明是谁批准或哪个 Patch 导致了它；缺证据明确“待核实”。
5. 生成页面记录输入快照；既有 CI 可检查链接和索引是否匹配该快照。正文、任务、审批状态不在 map 再抄写一份。

这是需要接入实现的轻量生成器/读取规则，不是 OpenSpec 默认功能。其输出可删除重建，不应成为第二份手工维护的交付状态表。

## 5. 用一条补丁说明如何消除二义性

| 文档阶段 | 主规格与 Patch 的实际关系 | AI 应如何读 |
| --- | --- | --- |
| 候选，尚未同步 | 基线仍是整数；Patch 提议整数或小数 | 解释现行基线时读主规格；实现这次变更时读基线和明确选定的 Patch，不把候选目标当已部署行为 |
| 部分或全部已同步、尚未归档 | 主规格已有部分/全部变更；Patch 仍在活动目录 | 按受影响要求核对已纳入范围，不能由“活动”推断“未同步”，也不能盲目重复覆盖 |
| 已整合并归档 | 主规格保留整合后的完整要求；Patch 保存演进原因与设计 | 普通阅读以所选文档 revision 的主规格为准，追溯再读 Patch；归档本身不证明上线 |
| 未同步即取消 | 主规格不变；取消原因由已有交付记录承载 | 不把取消内容加入当前要求；索引只引用真实取消记录 |
| 同步后取消 | 已同步的内容不会因删除/撤销 Change 自动回滚 | 需要明确的纠正变更及验证，重新核对受影响主规格；同时保留历史 |

一项 Change 可触及多个能力，而且同步可以只选部分差量，所以“是否纳入”不能只按 Change 一个布尔值从目录猜出来。

同一主规格的两个 Patch 并行时，反向索引应至少暴露共同修改的 Requirement。两者即使没有文本冲突，也可能有语义覆盖；先后合入时需对最新基线核对并复核受影响结论。主规格已反映较新的修订时，旧 Patch 不能因为仍在目录里就重新覆盖回来。

## 6. sync / archive 的精确边界

官方 `/opsx:sync` 是宿主 Agent 读取差量并编辑主规格的语义合并流程，可在归档前执行；CLI `openspec archive` 中的规格应用按要求块处理，并有要求匹配、场景遗漏等检查。二者不能笼统理解为同一个确定性三方合并器。

原生元数据没有本次 Patch 的基线 revision 契约，不能据此声称自动识别所有过期补丁。现有检查不排除两个 MODIFIED 使用同名要求和同名场景、正文语义不同的后写覆盖。双向导航让影响可发现，版本核对与独立 Review 决定是否应接受。

源代码与文档仅证实上述机制；没有运行当前团队的 CLI 或宿主集成测试。引用入口：[sync 源码](https://github.com/Fission-AI/OpenSpec/blob/v1.14.1/src/core/templates/workflows/sync-specs.ts)、[CLI 应用实现](https://github.com/Fission-AI/OpenSpec/blob/v1.14.1/src/core/specs-apply.ts)、[变更迭代说明](https://github.com/Fission-AI/OpenSpec/blob/v1.14.1/docs/editing-changes.md)。

逐项来源、真实查询输出及固定 SHA 见 [OpenSpec 主规格与 Patch 研究](/home/wangjunbo/.cache/ai-native-wayfinder-worktrees/openspec-context-20261010/docs/research/2026-10-10-openspec-patch-links.md)。

本轮解释机制并提供接入建议；具体反向索引位置、生成入口和验证方式由既有接入票据继续细化，不新增规格源，也不执行现行文档迁移。
