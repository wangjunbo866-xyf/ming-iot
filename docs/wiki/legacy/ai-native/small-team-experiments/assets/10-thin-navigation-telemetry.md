# 遥测上报：OpenSpec 原生目录上的薄导航

2026-10-10。Wayfinder 接入草案及可运行说明样例。用户已确认：保留原生目录、以业务域 → 功能阅读、OpenSpec 保持规格和变更的唯一正文来源。以下生成约定是具体建议；样例位于规划资产中，尚未成为真实研发文档仓的接入工具。

## 1. 最小方案

人工写原本就需要的 spec、proposal、design、tasks，再为每个功能登记一次简短的导航资料。生成器维护功能页、Change 稳定入口、相关变更与历史，以及原始文档内极短的返回链接。

把地址分成两类：

- **稳定入口**：`map/data/telemetry-ingest.md`、`map/changes/patch-temperature-decimal.md`。给人和 Agent 找路，也适合 Issue 的日常导航引用。
- **实际产物**：`openspec/changes/.../proposal.md`、delta、design、tasks。按原生规则归档，稳定入口随之更新指向。

这增加的是可删除重建的索引，不增加另一份 Feature 正文、任务表或审批状态表。第一版不需要向量数据库、图数据库或定制 OpenSpec schema。

## 2. 目录示例

以下路径均相对于同一个明确的研发文档根。样例中所有业务内容和历史日期都是演示数据。

```text
map/
  catalog.json                              # 人工：功能 ID、业务域、名称、短说明、已有资料入口
  README.md                                 # 生成：业务域导航
  data/
    README.md                               # 生成：数据服务下的功能列表
    telemetry-ingest.md                      # 生成：遥测上报功能页
    telemetry-query.md                       # 生成：遥测查询功能页
  changes/
    feature-telemetry-ingest.md              # 生成：Change 的稳定入口
    patch-temperature-decimal.md             # 归档前后保留相同地址
    patch-report-timestamp.md
    patch-telemetry-buffer.md

openspec/
  specs/
    telemetry-ingest/spec.md                 # 人工：当前完整行为规格
    telemetry-query/spec.md
  changes/
    patch-temperature-decimal/               # 原生 Change 目录
      .openspec.yaml
      proposal.md
      design.md
      tasks.md
      specs/
        telemetry-ingest/spec.md             # 原生 delta
        telemetry-query/spec.md              # 同一 Change 的另一个能力差量
    patch-telemetry-buffer/
      .openspec.yaml                         # skip_specs: true
      proposal.md                            # 实现修复，无行为差量
    archive/
      2026-09-20-feature-telemetry-ingest/
        proposal.md
        specs/...
      2026-09-25-patch-report-timestamp/
        proposal.md
        specs/telemetry-ingest/spec.md

tools/nav.py                                # 演示生成和检查入口
```

`map/changes` 中每个文件只是导航卡片，指向 OpenSpec 中的唯一 Change；不能在这里再写一份 proposal 或维护完成状态。`feature-`、`patch-` 是团队分类约定，不是原生 schema 的新增类型。

功能资料只登记一次，例如：

```json
{
  "id": "telemetry-ingest",
  "title": "遥测上报",
  "domain": "data",
  "summary": "定位设备遥测接收与校验的规格；查询行为另见遥测查询。",
  "links": [{"title": "实现与运行资料入口", "path": "context/telemetry.md"}]
}
```

已有业务地图元数据若能提供这些信息，就直接复用，不另建第二份 catalog。这里用 JSON 只是让独立样例只依赖 Python 标准库。负责人、运行说明、架构、消息契约等优先链接现有来源，不复制正文；一次 Change 的 D/R/P 责任仍在原有交付记录中。

## 3. 生成功能页长什么样

已生成的实际页面：[遥测上报功能页](thin-navigation-example/map/data/telemetry-ingest.md)。简化阅读效果如下：

> 遥测上报 · 数据服务 · `telemetry-ingest`  
> 现行 Feature → 当前完整 spec  
> 实现 / 消息契约 / 运行验证 → 已有资料入口

| 分组 | 变更 | 自动读出的关联 |
| --- | --- | --- |
| 活动变更 | patch-temperature-decimal | PATCH；MODIFIED Temperature value |
| 活动变更 | patch-telemetry-buffer | PATCH；实现关联，无行为差量 |
| 归档历史 | patch-report-timestamp | PATCH；ADDED Report timestamp |
| 归档历史 | feature-telemetry-ingest | FEATURE；ADDED Temperature value |

页面链接都由真实文件位置生成。这里的“活动/归档”严格指目录位置；活动 Change 也可能已经部分同步，归档 Change 也不自动等于批准或上线。

“现行 Feature”指当前文档版本的完整能力规格；“历史 FEATURE Change”指最初交付的变更包。日常理解功能不要求把所有历史 Patch 重新叠加一遍。

## 4. 聚合来源：尽量读原来已有的信息

| 信息 | 读取哪里 | 人工是否另填 |
| --- | --- | --- |
| 功能名称、业务域、背景资料入口 | 已有地图元数据；样例为 catalog | 每个功能登记一次，按资料变化维护 |
| 现行 Feature | `openspec/specs/<capability>/spec.md` | 原来的规格正文 |
| 已写差量的影响范围 | Change 下 `specs/<capability>/spec.md` 的路径及操作/要求标题 | 不再填写反向列表 |
| proposal 已写、delta 尚未写的草案 | 原生 Capabilities 的 New/Modified 声明 | 原来的 proposal 内容 |
| 无行为差量的实现修复 | proposal 的 Impact 中显式关联能力 | 只为缺少结构关系的情况补一次 |
| FEATURE/PATCH/SUNSET 分类 | 本方案的 Change ID 前缀约定 | 命名时表达；旧 ID 未分类就如实显示 |
| 活动和归档历史 | 扫描 `changes` 直属目录与 `changes/archive` 实际目录 | 不手写 changelog 清单 |
| 已批准、已整合、已发布、基线版本 | 已有审查/整合/发布记录 | 引用真实证据；缺少就显示未提供 |

无 delta 修复的单次关联示例，放在已有 Impact 内：

```markdown
## Impact

<!-- thin-nav-meta: {"related_capabilities": ["telemetry-ingest"]} -->
```

这个标记是**团队薄导航约定**，不是 OpenSpec 原生元数据。`.openspec.yaml` 继续只使用已支持的 `skip_specs: true`，不伪造一个 Modified Capabilities 或业务 delta。已有 delta/Capabilities 的关联不再写入这个标记；生成器取集合并去重。完全没有关联的 Change 必须明确进入待处理项，不能从所有功能页静默消失；本样例直接报错。

一次 Change 涉及上报与查询时，两张功能页都链接**同一个** Change 稳定入口；该入口再列出两个 delta、一个 proposal/design/tasks。不拆成两套交付记录。全项目纯工具变更可以关联实际的工具能力或全局治理入口；不要为了消除警告硬挂到遥测，本例尚未实现全局未分类视图。

历史视图只聚合标题、类型、关联、位置、来源链接。若还需要对外发布日志，另按发布范围生成；它不能把“归档时间”当“发布日期”。Blueprint 继续链接探索资料，未形成正式 Change 前不混入当前要求和交付历史。

## 5. 双向链接怎么维护

```text
主 spec ──Purpose 中的短链接──► 功能页
                                 │
                                 ▼
                     Change 稳定入口 ──► proposal / delta / design / tasks
                                 ▲            │
                                 └────────────┘
                           proposal 的自动导航
                                              │
                                              └──► 功能页 / 当前 spec
```

主 spec 只在已有 Purpose 内放一个短导航，不给每条 Requirement 手写覆盖链。proposal 的已有 Impact 内放有边界的生成区块：

```markdown
<!-- thin-nav:begin -->
- [此变更的稳定入口](../../../map/changes/patch-temperature-decimal.md)
- [遥测上报功能页](../../../map/data/telemetry-ingest.md)
- [当前完整规格](../../specs/telemetry-ingest/spec.md)
<!-- thin-nav:end -->
```

生成器只能更新这对标记内的链接。规格要求、proposal 的动机/影响、设计、任务和 delta 正文由原作者维护。直接打开 delta 时，按其所属 Change 读取同目录体系内的 proposal；delta 文件自身不再附加一套覆盖清单。根入口的 Agent 读取规则应明确这一点，不能只读孤立的 delta 就开始实现。

源码层面，普通导航链接放入 Purpose/Impact 未发现默认解析冲突；另建主 spec 顶层 Related Changes 章节可能触及完整退役时的额外内容检查。因此动态列表集中留在 map。这个结论是固定 v1.14.1 的源码核对，仍需真实接入时运行原生校验。

## 6. 归档前后：稳定地址不变，目标和返回路径重新计算

```text
持续引用的地址：map/changes/patch-temperature-decimal.md

归档前指向：openspec/changes/patch-temperature-decimal/proposal.md
归档后指向：openspec/changes/archive/2026-10-10-patch-temperature-decimal/proposal.md
```

功能页中的链接始终指向同一张稳定入口卡片。卡片里的 proposal/delta 路径、proposal 返回地图和主规格的相对路径，都在扫描实际目录后重新计算。归档目录多一层，`../../../map/...` 会变为 `../../../../map/...`，无需人逐个修改。

需要把 **OpenSpec 操作 + 导航重建 + 校验** 放在同一次文档变更中提交。生成器会改写归档 proposal 的导航区块，这是派生链接更新；区块外历史正文保留。已提交旧版本仍可由 Git 固定 revision 读取。如果项目要求归档文件逐字节不可变，应改用只在 map 中维护返回入口的方案；本建议不同时叠加两套机制。

不能承诺任意旧裸路径永远有效：外部若已直接引用移动前的 `changes/...` 路径，事后生成一张新入口卡片不会自动修好那个旧 URL。接入时盘点并转换已有引用，以后使用两条规则：

1. **日常导航**引用稳定入口，随主分支走到最新位置。
2. **基线、审查和验收证据**引用固定 Git revision + 当时存在的路径。移动当前目录不会改变旧提交里的文件；不要拿可变的导航页作不可变证据。

Patch 的基线不是从归档日期或当前 spec 猜出来的。应复用已有交付记录里的 docs revision；缺少则显示“未记录”，首次新增能力可明确“无既有基线”。样例没有真实 PR 或基线，所以没有伪造 SHA、审批结论或证据链接。

## 7. 标识和异常约束

稳定引用能成立，需要几条很小但明确的约定：

- 同一文档根中 Change ID 永不复用，归档后也不复用；发现同名活动/归档记录时拒绝任选一个。
- 本样例 Change ID 以字母开头，不带前置日期；归档目录的日期才可明确剥离。OpenSpec 本身还允许名称已经带日期且归档时不重复加日期，旧库不能盲目按正则去日期；首次接入需要显式核对/映射那些名称。
- 跨文档根用 `root_id + change_id/capability_id` 区分；读取同一次扫描快照，不把不同 checkout/store 的内容混在一起。
- 功能页地址一旦被引用就保留。改业务域显示可以先改菜单；若确实移动入口，旧入口保留跳转说明。功能退役保留历史入口，当前 spec 缺失先显示待核对，不能自动认定已退役。
- 同一 Requirement 被多个活动 Patch 修改时，应按扫描结果提示复核。语义是否兼容仍由独立 Review 判定；不能自动按日期叠加。此冲突提示尚未写入样例生成器。
- 删除 Change、取消需求、重命名能力属于显式变更。现存稳定入口失去来源时，本样例报错，后续接入再按已有取消/退役记录生成保留入口，不能静默删除历史。

稳定 ID 及入口处理是文档治理，不是新的工作流状态机。LoopX、不同 Skills 或人工操作均可调用相同生成/检查入口。

## 8. 日常维护和验收

建议将生成结果提交 Git，所有成员和普通 Markdown 浏览器都能直接阅读。生成是确定性的，不写运行时间戳；每页的输入指纹只用于发现派生内容过期，不是 Git revision 或验收证据。

```bash
# 在样例根目录执行；命令只作用于这个样例
python3 tools/nav.py build
python3 tools/nav.py check
python3 tools/walkthrough.py
```

真实接入时的触发点：新建/修改 proposal 或 delta、同步主规格、归档、修改功能入口。提交前可执行 build；CI 执行只读 check，有差异或失效链接就退回。CI 不替开发者静默提交修复。需要同时覆盖主 spec 和 Change 的导航；归档与生成结果作为一个 Git 候选提交审查，减少读到半成品的窗口。

| 责任 | 日常工作 | 通过/退回 |
| --- | --- | --- |
| D 交付责任人 | 维护原生业务产物；新能力补一次入口；运行生成/检查 | 关联可发现、引用存在、来源明确；缺失关系或过期导航退回 |
| R 独立审查者 | 检查影响能力是否完整、当前规格与选定 Patch 的关系及真实证据 | 导航完整只是前提；遗漏跨仓影响、语义冲突或证据不符仍退回 |
| 文档 CI | 重算导航并检查差异、重名与目标存在性 | 自动区块/生成页过期、稳定入口无来源、未关联 Change 失败 |

人工不用逐张功能页补 Patch、不用在主 spec 写历史补丁清单、不用另写一个按功能排序的 Change 副本；新增功能时补入口，业务含义变化时维护原来的规格和变更正文。

## 9. 本次已验证与待接入项

运行记录见 [导航场景验证](thin-navigation-example/validation.md)。8 个检查通过：原生目录聚合和重复生成、无差量修复、只有 proposal 的草案、模拟归档后的双向链接和正文保留、归档不推断同步、重复 ID 拒绝、来源丢失拒绝、关联缺失拒绝。

归档场景使用临时目录移动模拟，检查前故意保留旧索引使其失败，再生成修复。没有调用 OpenSpec CLI，不代表原生完整归档、语义同步或团队业务验收已经通过。样例解析器仅处理所示 Markdown 形状；正式接入应复用固定版本的 OpenSpec 解析能力并补齐归档适配，避免把示例正则当作完整 Markdown 解析器。当前样例没有接入真实基线/PR 证据、并行冲突提示、入口迁移/退役映射、多 store、CI 或新会话宿主验证。

### 资料与入口

- [可运行样例说明](thin-navigation-example/README.md)
- [生成的遥测上报功能页](thin-navigation-example/map/data/telemetry-ingest.md)
- [生成的 Patch 稳定入口](thin-navigation-example/map/changes/patch-temperature-decimal.md)
- [前置双向关联解释](13-patch-bidirectional-context.md)
- [接入决策票据](../issues/10-context-adoption.md)
- [固定版本 proposal 模板](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/schemas/spec-driven/templates/proposal.md)：原生 Capabilities/Impact 及 skip_specs 说明。
- [固定版本 archive 实现](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/archive.ts)：原生归档命名、整目录移动；不自动重写自定义 Markdown 链接。
- [固定版本规格应用与退役检查](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/specs-apply.ts)：Purpose 与额外内容的区别。
