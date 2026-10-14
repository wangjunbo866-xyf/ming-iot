# Ming IoT：r-doc 试点后的 GitHub 迁移准备

2026-10-13。本报告交付第二阶段最后一项的准备结果。**试点和本地 Git 演练完成，r-doc v4 正式采用仍为 NOT_ACCEPTED，GitHub 实际迁移未执行。**

建议保留有边界的小 fork 修补路线。暂不承诺全面自建，也不继续无终点地增加评测轮数。下一候选应先处理两个已复现 P2、定点任务正常退出，以及 Go/JSON 字段判据；另一个工作包处理父仓迁移对现有治理工具的影响。两者分别验收。

依据：[第二阶段验收约定](../../.loopx/goals/ming-iot-knowledge-pilot/V4_BRIEF.md)、[v4 采用决定](2026-10-13-r-doc-v4-adoption-decision.md)、[最终采用证据](../../.scratch/r-doc-pilot/reports/v4-final-acceptance.json)。本报告不改变已有业务规格或历史失败。

## 1. 已核实的边界

- 工作区根目前不是 Git 仓库。父仓名称、GitHub owner、可见性和权限尚未确定。
- 用户确认产品仓保留独立历史、PR、权限和发布；父仓统一协调知识和版本组合。
- `ming-iot` 本地 HEAD 为 `6742201e9666fd3b3067216b2efbb3b8ba7338d7`，清洁。
- `ming-iot-rd-docs` 为 `b16030c10cf2565ecfa16ab19ee43a3a273bec57`，另有未跟踪 Proposal。
- `ai-native` 为 `777d68e54942b1f6cf661e9c3ed7887050c22b61`，有 `.gitignore` 改动和多组未跟踪学习、研究、试点资料。
- `iot-doc` 仍是现有 README 声明的正式 IoT 架构来源，且有未提交删除及新增资料。本批保留来源引用，未导入它。
- 未发现名为 `mqtt-gateway` 的本地目录；前端仓身份未指定。未将 `core`、`things` 自动认定为这两个仓。
- OpenSpec 仍是待定方案。ThingsBoard、AirMQ 继续作为工作区参考仓，不自动纳入父仓分发。

详细记录：[本地仓库清单](../../.scratch/r-doc-pilot/migration-prep/inventory.json)。没有查询 Gitee/GitHub 远端 refs、权限或服务端元数据；本地可读不证明这些条件已满足。

## 2. 内容归属及首批目录

建议先把两个来源仓装入父仓的普通目录，保持内部路径，再逐批调整。六类目录先承担知识导航，避免同时迁移平台、历史、正文位置和所有校验器。

```text
<父仓名待定>/
├── AGENTS.md                 四部分入口
├── docs/
│   ├── specs/                业务地图与规格导航
│   ├── changelog/            计划、评审、证据时间线导航
│   ├── architecture/         架构入口，显式说明 iot-doc 当前权威
│   ├── runbook/              操作知识与团队接入导航
│   ├── vendor/               外部材料、版本、许可证记录
│   ├── wiki/                 人工中文知识入口
│   └── rd/                   旧研发仓完整已提交内容，普通目录
├── tooling/ai-native/        首批兼容布局，普通目录
└── repos/
    ├── ming-iot/             独立子模块
    ├── mqtt-gateway/         待确定真实来源
    └── frontend/             待确定真实来源
```

`docs/rd` 和 `tooling/ai-native` 是保真导入的中间布局，不是两个继续独立维护的子仓。物理重排前，同一正文只有一个当前位置；导航不复制一份正文。

| 来源 | 归属方案 | 首批处理 |
| --- | --- | --- |
| 研发仓 specs、map | 父仓业务规格和地图 | 保留目录、中文权威和批准状态；由 specs 导航到原正文 |
| plans、reviews、evidence | 父仓交付事实 | 保留独立评审、exact_head、历史结论；由 changelog 导航，不复制 LoopX 任务状态 |
| rfcs、adr、proposals | 父仓架构取舍和待决方案 | 保留各自有效性，候选方案不转为实施授权 |
| archives、references | 父仓历史和参考资料 | archives 保持原样；外部材料核对来源、许可和私有边界后才发布 |
| meta、tools、tests、仓库规则 | 父仓项目治理 | 先改造仓库身份与路径，再切换权威 |
| ai-native 的 map-delivery、rules、hooks | 当前带 IoT 耦合，先归父仓项目工具 | 不能因为名为 Skill 就认定可跨项目直接复用 |
| ai-native 分发/校验工具 | 通用提取候选 | 消除 Gitee、文档仓名和独立 Git 根假设后，按真实复用需求单独版本化；父仓锁定版本 |
| 未跟踪学习材料 | 待逐件归属 | 不随整仓导入自动包含；尤其不把 `.scratch`、`.loopx`、缓存和原始会话发布 |
| iot-doc | 当前架构权威，归属待定 | 保留引用；决定导入哪些主题后再做映射，避免出现第二份架构权威 |

已生成 **177 条逐文件归属与路径映射**：研发仓 155 条、ai-native 22 条，每条绑定源提交、blob 和目标路径。见 [path-map.json](../../.scratch/r-doc-pilot/migration-prep/path-map.json)。

[新的 AGENTS 提案](../../.scratch/r-doc-pilot/migration-prep/templates/AGENTS.md) 已包含仓库地图、快速命令、关键约定速查表和带日期的状态表。它仅用于本地演练，没有覆盖真实工作区入口或旧 v4 样板。

## 3. 历史保留及链接策略

本次演练为三个仓分别生成包含所有**本地 refs 和 HEAD**的 Git bundle，并恢复为独立 mirror：研发仓 27 个 refs、ai-native 9 个、产品仓 26 个，名称和对象 ID 均一致。

父仓采用无 squash 的历史合并，再以目录前缀导入研发和工具树；源 HEAD 保持原 SHA，且是父仓 HEAD 的祖先。首批源树 hash 完全相同。旧分支和标签保留在各自备份中，不把不同仓的同名分支标签混成父仓发布语义。演练证明的是选定 HEAD 祖先历史在父仓可达、全部本地 refs 可独立恢复；未证明所有远端分支、reflog 或服务端内容已保存。

历史查阅使用 `git show <原提交>:<原路径>`。新增前缀之后不能仅凭新路径的普通日志显示，就推断所有工具都能自动跨越导入点追溯。迁移映射必须保留原仓身份、原路径、原提交、新路径和切换提交。

本地行内 Markdown 扫描得到 111 条引用：77 条相对引用、34 条 Gitee 引用。74 条相对引用能在来源已提交树定位；另 3 条进入人工复核，含可能被简单扫描误识别的示例文本，**不据此声称存在 3 个真实断链**。扫描不覆盖引用式链接、裸 URL、YAML 引用和锚点，因此不能宣称全库链接检查通过。

- 内部相对路径通过整树前缀导入保持不变。
- 旧 Gitee Issue/PR 和 Delivery ID 保留原值；目标创建并核实后追加 GitHub 映射，不用新编号覆盖旧事实。
- 历史 Evidence 的 exact_head、审批状态和原仓身份保持原义；当前代码引用另写 GitHub 身份和迁移绑定。
- 旧仓在切换后保留可访问历史和迁移入口；归档或限制写入在团队确认切换后执行。

见 [链接映射](../../.scratch/r-doc-pilot/migration-prep/link-map.json)、[协作身份映射草表](../../.scratch/r-doc-pilot/migration-prep/collaboration-id-map.json)。后者只有文件内已发现引用，并非服务端 Issue/PR 完整清单。

GitHub 官方将“源码与历史”和“源码、历史及协作元数据”区分为不同迁移范围；本方案据此单列 Issue、PR、附件、Release、权限和 CI 的迁移工作。[GitHub 迁移规划](https://docs.github.com/en/migrations/overview/planning-your-migration-to-github)

## 4. 实际演练结果

演练仅使用 scratch 内新建的本地仓库及本地文件远端；没有创建 GitHub 仓库、修改真实 remote、安装候选或发布内容。

| 检查 | 结果 | 解释 |
| --- | --- | --- |
| 三仓 bundle 校验和 refs 恢复 | PASS | 62 个本地 refs 完整恢复；不包含服务端 Issue/PR 等元数据 |
| 两个前缀导入的树和祖先关系 | PASS | 原提交、内容和文件 mode 保留 |
| 全新父仓递归检出 | PASS | ming-iot 检出指定 HEAD；没有使用 Git alternates 依赖旧工作区 |
| 逐文件 blob 核对 | PASS 177/177 | 研发及工具已提交内容一致；未导入未跟踪文件 |
| 源仓 HEAD、refs、index/worktree diff 和状态 | 一致 | 演练前后未改变三个来源仓的 Git 状态 |
| 交付元数据检查 | PASS | 显式 docs-root 为 docs/rd，since=2026-09-03；不代表全量历史均重新批准 |
| map Git 基线校验 | FAIL | 父仓内报 MAP_BASELINE_INVALID；原文档仓相同基线 PASS |
| map-delivery 文档定位 | FAIL | 明确拒绝非 Git 工作树根的 docs/rd |
| 网关、前端、真实 GitHub 权限、CI、团队安装 | NOT_RUN | 不能据单个子仓检出宣称系统可用 |

证据：[演练回执](../../.scratch/r-doc-pilot/migration-prep/rehearsal.json)、[map 正确子命令的复核](../../.scratch/r-doc-pilot/migration-prep/map-check-corrected-probe.json)、[原仓对照](../../.scratch/r-doc-pilot/migration-prep/map-check-source-control.json)、[源仓保护记录](../../.scratch/r-doc-pilot/migration-prep/source-preservation.json)。第一次误用 `map_docs.py validate` 的参数错误完整保留；随后改用真实子命令 `check` 才得出上述布局兼容结论。

演练 `.gitmodules` 使用本地路径，只适用于本机证据，不能原样推送为目标配置。全部私有 bundle 也保留在 scratch 内。

## 5. 切换前需要实现的兼容改造

1. **文档根与 Git 根分离。** 定位器显式返回 parent Git root、docs root 和相对前缀，验证二者包含关系及来源身份。现有 `resolve_docs.py` 把二者视为同一路径。
2. **基线按父仓路径取证。** `map_docs.py check_candidate` 目前要求 docs root 是 Git 根，并以 `git show <base>:map/...` 读取旧正文。适配后必须区分旧仓原提交和导入后的父仓基线，不能只删除根目录检查或在取证失败时退化为通过。
3. **来源与安装契约重绑定。** `meta/SKILLS_SOURCE.json`、`setup_workspace_skills.py`、锁文件、README 命令和 origin 限制需指向批准的新身份。安装器不得覆盖未知入口；新会话发现和真实加载单独验证。
4. **平台相关交付适配。** Gitee 专用 Issue/PR 工具、PR URL 模式、`.gitee` CI 和策略测试不能直接证明 GitHub 可用。保留历史 Gitee 身份的同时，为新交付增加 GitHub 路径并验证分支保护。
5. **切换包按最终 Head 重验。** 原生文档规则、来源校验、受影响链接、子模块组合、成员权限和 CI 必需检查均需绑定实际候选。r-doc 不替代这些项目门禁。

上述是本次准备识别的实现工作，不作为已修复内容。v4 本身的锚点读取边界、无效配置类型两个 P2 同样仍未修复。

## 6. 仓库参数与后续批次

[repository-parameters.json](../../.scratch/r-doc-pilot/migration-prep/repository-parameters.json) 提供可填写的具体清单。父仓暂建议 private、默认分支 main；这些是建议，尚未设置。每个子仓保留独立权限和发布，CI 需能读取其固定提交。

开始实际迁移前还需确定：

1. GitHub owner、父仓名，各仓可见性、维护团队及必要 CI 权限。
2. 网关和前端的真实来源仓；iot-doc 保持引用还是选择性并入，以及未跟踪材料的纳入清单。
3. 采用修复后的候选，或明确另立不启用 r-doc 的仓库迁移范围；当前约定不允许借迁移部署未通过的 v4。

建议批次：

| 批次 | 工作 | 完成证据 |
| --- | --- | --- |
| A：候选修补 | 两项 P2、字段判据、定点正常退出 | 新冻结版本、受影响正反回归、同条件真实任务、独立采用结论；不改判旧失败 |
| B：迁移适配 | 文档根/父仓根分离、历史基线、来源和 GitHub 治理 | 在本演练布局通过全部适用门禁；故意错身份、错基线应拒绝 |
| C：目标预检 | 确定参数；比对远端 refs、LFS、Issue/PR/Release、历史和许可边界 | 最终待迁移清单及回退点；保留用户未提交内容 |
| D：分批切换 | 子仓历史先就位，父仓导入与 gitlink 固定，再切换团队入口 | 使用真实权限的全新递归检出、全部子仓一致、CI 和技能发现通过 |
| E：旧入口收尾 | 公布映射，旧仓转历史入口，恢复演练 | 当前权威唯一；失败可回到原仓与原版本组合 |

A、B 可作为下一阶段两个明确工作包，目标参数可同步补齐；本次没有自动启动新一轮候选开发或远端迁移。**第二阶段按“采用未通过时交付准备结果和路线调整”的约定收尾，不能把试点收尾解释为正式采用或迁移完成。**

复现与索引：[本地演练脚本](../../.scratch/r-doc-pilot/migration-prep/prepare.py)、[准备包验收](../../.scratch/r-doc-pilot/migration-prep/acceptance.json)。脚本拒绝覆盖已有演练目录，保留原始回执。
