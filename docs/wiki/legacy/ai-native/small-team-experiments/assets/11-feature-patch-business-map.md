# feature / patch 与业务地图：OpenSpec 原生组织方案

日期：2026-10-10。状态：用户要求至少保留 feature、patch，并已选择“业务域 → 功能，跨仓连接实现”。组织决定见 [Feature/Patch 与业务地图决策](../issues/11-feature-patch-map.md#answer)。本文提供该路线的映射和示例；具体业务域清单、模块名称和迁移顺序仍待实际接入确认，没有改动现行业务规范或执行迁移。

## 1. 方式 1 可以支持什么

可以保留 feature、patch 的功能，并为 sunset、blueprint 留清晰位置，不需要因此定制 schema。OpenSpec 提供现行 capability spec、Change 内的 proposal/design/tasks/delta specs 以及同步/归档机制；四个英文标签是团队可采用的分类，不是 OpenSpec 默认内置的四种文档模板。

依据：[v1.14.1 官方概念](https://github.com/Fission-AI/OpenSpec/blob/v1.14.1/docs/concepts.md)、[默认 schema](https://github.com/Fission-AI/OpenSpec/blob/v1.14.1/schemas/spec-driven/schema.yaml)。具体实现由关联研究核查，团队分类与产品原生字段分开说明。

完整事实与固定源码出处见 [OpenSpec 原生功能、补丁与退役演进核查](/home/wangjunbo/.cache/ai-native-wayfinder-worktrees/openspec-context-20261010/docs/research/2026-10-10-openspec-feature-patch.md)。源码版本为 `87c3595ace6a2e22957f39ebe5b74c2f2316e0cb`；本次未运行实际接入试验。

本轮需要澄清 feature 的两种用法：

- **一个长期功能**：需要可直接阅读的现行完整行为说明，对应 capability 的 spec。
- **一次新增功能的交付**：对应一个 FEATURE Change；它结束后成为该功能的历史，而功能规格继续演进。

二者可以同时存在，不应把“第一次做这个功能的 proposal”长期当作现行完整规格。类似地，patch 是一次有范围的修订，不应让所有历史 patch 永久叠加为读者每次必须重建的当前真相。

## 2. 四类内容的落点

| 团队概念 | 建议含义 | 原生或轻量落点 |
| --- | --- | --- |
| Feature 功能正文 | 该功能当前的完整行为要求和场景 | `openspec/specs/<capability-id>/spec.md`；地图只链接 |
| FEATURE 变更 | 首次建立或显著扩展功能 | `openspec/changes/feature-<name>/`，使用原生 proposal/design/tasks/specs |
| PATCH 变更 | 对既有功能局部增加、修改或移除行为/方案 | `openspec/changes/patch-<name>/`；行为有变则写对应 delta；纯技术调整放 design/tasks |
| SUNSET 变更 | 有计划地退役能力、接口或旧行为 | `openspec/changes/sunset-<name>/`；描述替代方案、兼容窗口、迁移与恢复条件，并表达规格移除/替代 |
| Blueprint 蓝图 | 尚未决定采用的探索或未来方案 | 沿用现有 proposals/references 等探索资料位置；地图标为探索，选定后链接 FEATURE/PATCH Change |

`feature-`、`patch-`、`sunset-` 是便于人识别的命名约定，不会让 CLI 自动改变审批规则或业务状态。Blueprint 不强制再建一个 wiki 平台或目录。如果探索尚无值得保存的结论，可以先用 explore；有长期复用价值再保存材料。

类型和业务影响可以写在 proposal 正文，受影响 capability 沿用原生栏目；本方案不向 `.openspec.yaml` 凭空添加 `type` 等未获支持的字段。如果以后需要机器强制分类，再决定是否增加轻量校验，而不把这一点误称为默认内建类型管理。

原生 specs 描述行为；详细实现不塞进 spec：一次变更的设计在 design，长期现行架构在已有架构入口/必要 ADR。若希望 Feature 页面能看全貌，由地图把这些来源连起来，而不是复制成一份越来越大的 feature.md。

## 3. 建议目录与阅读入口

下面是文档根内的示例结构，不是已创建的业务目录。Capability ID 先用稳定的扁平名称，业务层次由 map 表达；不依赖复杂 schema 或额外目录发现逻辑。

固定 v1.14.1 源码也支持层次 capability 路径；本示例选择扁平 ID 只是便于第一版理解与接入，并非产品只能使用扁平目录。Change 名称仍是单层 slug，不能把它推断成可按 `feature/foo` 嵌套的 Change。

```text
map/
  README.md
  data/
    README.md
    telemetry-ingest.md             # 薄导航：功能入口与相关变更链接

openspec/
  specs/
    telemetry-ingest/spec.md         # Feature 的现行行为正文
    tenant-data-access/spec.md       # 复用的数据隔离要求
  changes/
    feature-telemetry-ingest/        # 首次交付，尚在进行时
      proposal.md
      design.md
      tasks.md
      specs/telemetry-ingest/spec.md # ADDED：候选新增要求
    patch-telemetry-temperature/     # 后续局部变更示例
      proposal.md
      design.md
      tasks.md
      specs/telemetry-ingest/spec.md # MODIFIED/ADDED：候选差异
    archive/
      <归档日期>-<change-name>/       # 保存完整变更背景与历史

proposals/
  telemetry-v2-blueprint.md          # 尚未决定采用的探索，按需创建
```

树中首次 FEATURE 与后续 PATCH 用于展示两种结构，通常发生在不同时间。不能把首次功能尚未完成、后续补丁却默认基于它已完成当作推荐顺序；若确需重叠，必须显式声明基线与依赖。

在地图上可以提供熟悉的功能页视图：

```text
遥测上报
  功能说明 / Feature → 现行 spec
  当前架构与接口     → 架构、代码、API 来源
  进行中的 Patch     → 相应 Change
  退役计划 / Sunset  → 相应 Change（有需要时）
  蓝图 / Blueprint   → 探索材料（有需要时）
  历史               → 已归档 Change / PR 索引
```

它在阅读体验上把资料聚合到一个功能下，但正文按各自生命周期保存。自动生成的 Change 列表是可重建索引；运行进度仍由已有交付系统/LoopX 管理。

## 4. 一次 Patch 怎样收敛

示例：现行遥测规格只定义整数温度，本次希望支持小数。数值和场景仅用于解释文档机制，不是已批准的 IoT 需求。

1. 地图指向现行 `telemetry-ingest` spec，同时可发现候选 `patch-telemetry-temperature`。
2. Patch 的 proposal 说明为什么改、影响的 capability 与兼容范围；design 描述解析/存储等技术选择，tasks 记录实施和验证。
3. Delta 使用 ADDED/MODIFIED/REMOVED 表达行为差异。MODIFIED 携带被修改的那一整条 Requirement 及其所需场景，不只是“把整数改成小数”一句话，也不必复制整个 Feature 全文。
4. 实施与独立验收仍执行既有规则。按团队确定的文档基线收敛时点，将差异合入当前 spec；归档保留 Patch 的原因、设计和任务。
5. 下一个普通任务直接读更新后的 Feature spec；追问原因时再读归档 Patch。正在处理该 Patch 的任务则同时读取其基线与候选差异。

原生 sync/archive 不是批准、代码合并或部署证明。本项目已确认的文档 PR / 代码 PR 顺序、完整集成在合并后发布前完成等规则继续适用；目标环境实际使用哪个版本从现有发布/证据记录确认。

若两个 Patch 同时修改同一 Requirement，独立 Change 文件夹也不能消除语义冲突。后合入者需要对照最新规格重新检查；跨多个 capability 的一次变更可以只有一个 Change，多个模块入口引用它，避免每个模块复制同一补丁。

修复实现偏离既有承诺时，不应该为了“有 Patch 文档”捏造规格改动。例如整数温度原本承诺可查询但实现报错，正确做法是引用既有场景并交付修复证据。当前本地治理草案另有 BUGFIX 类型，应保留该语义，不因用户重点讨论四类内容就静默删除已有分类。

v1.14.1 支持 Change 元数据 `skip_specs: true` 表示本次没有行为规格差异，仍可保留 proposal/design/tasks；它与某次 archive 命令临时跳过同步不同。完整退役某项 capability 可表达 REMOVED，最后一条要求移除另需显式 `retire_capabilities: true`，并受原生保护检查约束。这些字段不替代退役设计、数据迁移和项目验收。

## 5. 业务地图怎样划分

用户已选择 **业务域 → 功能，跨仓连接实现**。具体拆分建议以可独立描述和验收的业务目标、行为契约和责任归属判断；服务、仓库、前后端是实现标签。一项功能可以跨前端、API、网关与存储。不要每个 API 单独建一个 Feature，也不要把整个 Things 服务塞进一个超大 Feature。

以下仅是便于讨论的候选分类，不是对现有已实现能力的清单：

| 一级业务域 | 二级功能示例 | 跨域引用举例 |
| --- | --- | --- |
| 租户与权限 | 租户/成员管理、角色授权、设备数据访问 | 遥测查询、远程控制引用统一数据访问要求 |
| 设备管理 | 产品与物模型、设备档案、分组/标签 | 接入和遥测引用设备身份与物模型 |
| 设备接入 | 设备认证、协议接入、连接状态 | 连接状态由设备管理展示，但不复制接入规格 |
| 数据服务 | 遥测上报、最新值查询、历史查询 | 查询引用租户隔离，规则消费相同数据语义 |
| 设备控制 | 指令下发、执行回执、超时处理 | 引用设备身份、连接状态和授权 |
| 规则与告警 | 规则触发、告警生成、通知 | 引用遥测事件、设备状态和收件人权限 |

每个 capability 只有一个权威位置和稳定 ID，可以出现在多个导航视图。一次模块拆分应有独立验收价值；若两个部分总是一起修改、无法单独描述，可以先保留为同一功能，不为分类完整而过度拆分。

一种最小功能入口可以是：

```text
名称：遥测上报
能力 ID：telemetry-ingest
边界：校验并接收设备遥测；查询功能另见遥测查询
责任人：认领时填写
Feature：现行 spec 的唯一链接
Patch：相关候选 Change 的索引
实现：网关 / 设备管理 / API 的实际路径
契约：消息与数据定义的来源
运行与验证：适用配置查询、验证命令和操作说明的入口
```

已有 `saas / things` 分类保留为技术视图或实现标签；本轮未选择继续以它们作为业务地图的一级入口。这个阅读视角不会自动修改代码仓边界、成员权限或现行目录。

## 6. 现有项目的边界与后续接入

只读核对：现有 [业务地图入口](/home/wangjunbo/workspace-iot-platform/ming-iot-rd-docs/map/README.md) 标为设计试点，并提出 `00_研发治理 / 01_saas / 02_things`；其模块目录草案仍包含 spec 与 changes 正文。采用本方案时，这些正文需在明确切换后由 OpenSpec 单一承接，map 保留导航，不能同时维护两套。

当前 [治理草案](/home/wangjunbo/workspace-iot-platform/ming-iot-rd-docs/map/00_研发治理/01_文档管理/spec.md) 已区分模块规格与 FEATURE/PATCH/BUGFIX/SUNSET Change。它尚不是本次路线的 OpenSpec 实现，也不能据此推断迁移或验收已经完成。

业务地图的主阅读视角已确认。目录名称、真实模块清单与源路径由后续接入和试点进一步核对；本示例列出的六个业务域不因本轮选择而自动成为已批准的完整分类。用户已经要求保留 feature、patch，本方案说明如何在原生结构下满足，未重新询问是否需要它们。
