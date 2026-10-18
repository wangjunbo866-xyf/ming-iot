---
status: local-candidate
version: 0.2.0
date: 2026-10-18
---

# 有界文档检查

从[知识导航](../README.md)进入当前正文。[总纲](../project-charter.md)给方向，[团队约定](team-development.md)规定人如何维护模块，[工具入口](../../tooling/README.md)提供确定性检查。本文规定读取边界；不建立另一套业务规格。

## 范围先于检查

唯一机器清单为 [knowledge.json](../../tooling/knowledge.json)，schema_version 为 2。`maintained_documents` 逐文件列出当前维护正文；`document_scope` 定义历史、外部材料、文件名盘点范围及导航根。当前导航根是仓库 README 和 AGENTS.md，文件名盘点只在 `docs/`。工具、Skill、后端文档只读显式列出的文件，不递归扫描它们所在目录。

| 类别 | 本轮处理 | 不代表什么 |
| --- | --- | --- |
| maintained：当前维护正文、导航、Skill | 读取正文，检查本地链接、支持的锚点和入口可达性 | 不代表语义正确、负责人确认或业务验收 |
| historical：changelog、legacy、明确列出的已吸收 Patch 与旧规格 | 盘点文件名；作为链接目标时只检查存在。仅索引显式替代关系的非 legacy 端点读取正文核对互链 | 不批量改写旧路径、旧结论或历史版本 |
| reference：docs/vendor | 盘点文件名；作为目标时只检查存在，正文不递归读取 | 不验证第三方真实性、许可证充分性或外网有效性 |
| 未分类 | 在 docs 内出现时报告错误，要求明确归属 | 不自动纳入维护、移动或归档 |

精确维护清单优先于历史/外部模式，所以 legacy 的少量导航 README 仍要维护。其余模式重叠报错。已吸收 Patch 逐目录列出，**新 Patch 不会因为放在 patches 下就自动变成历史**；在途 Patch 应加入维护范围。此分类不修改文件中的业务状态，实际 PR 的模块与 Patch 契约仍由 [specs PR 检查](../../.agents/skills/ming-specs/references/pr-check.md)负责。

## 三组检查

1. **链接与可达性。** 支持普通/图片/引用式链接、尖括号路径与标题、括号路径、百分号编码、ATX/Setext 标题和显式 HTML a 锚点；围栏代码、行内代码与 HTML 注释不当导航。图片和未使用的引用定义不能让孤立文档变得可达。当前文档必须能沿当前文档链接从导航根到达；只登记在 YAML 或互相循环引用还不够。
2. **索引一致性。** [docs/index.yaml](../index.yaml) 的 source_id 唯一、正文/related 文件存在、生命周期明确；一个当前正文只能有一个 current 来源。历史多条来源指向同一汇总记录是允许的。声明 `body_bindings` 的版本/状态必须与正文一致；字段类型错误也失败。未声明绑定的 revision 仅为标签，不冒充正文版本校验。
3. **替代关系。** `supersedes` 用 source_id 关联，并写清 `scope`；检查目标存在、禁止自替代/循环/同正文互替代，并要求两份正文互相链接。以总纲替代旧盘点和合并后计划的执行优先级为例，旧实验、合并与验证事实仍保留。工具检查关系结构，人确认范围是否真实，不能靠一个链接证明内容已正确吸收。

索引示例（仅说明格式，真实关系在来源索引维护）：

```yaml
lifecycle: current
body_bindings:
  revision: frontmatter.version
  status: frontmatter.status
supersedes:
  - source_id: old-plan
    scope: 后续优先级；历史证据保留
```

`lifecycle` 是来源用途，不等同于正文的 draft/confirmed。历史 revision 可保留 `m5-3` 等原回执标签，不能拿它和持续维护正文版本强行比较。现有非 frontmatter Skill 可显式绑定 `skill.version`；不强制为所有旧资料补元数据。

## 运行与结果

首次在自己的 Python 环境安装 [requirements-docs.txt](../../tooling/requirements-docs.txt)，然后从仓库根运行：

```sh
python3 tooling/knowledge.py check
make knowledge-check
```

第一条仅检查并输出 JSON；第二条还运行工具和既有 ming-specs 回归。代码交付继续运行 `make verify-pr`。现有单仓 CI 会安装固定依赖并调用原有有限范围检查入口；没有新增全库扫描、Hook 或服务端必需检查配置。本地通过不代表 GitHub 已运行该候选。

`result=PASS` 仅表示本轮声明的确定性检查通过。`classification_counts`、`bodies_read`、`supersession_bodies_read` 显示实际覆盖。`unverified_fragments` 明列历史/外部正文内未读取的锚点，不能计作已验证。外部 URL 只识别为外部，不发请求，固定显示 `external_urls=NOT_FETCHED`；业务语义显示 `semantic_review=NOT_RUN`。

遇到断链先核对目的地；遇到不可达先补真实导航；遇到索引冲突核对哪个是当前正文；遇到替代冲突由变更责任人确认适用范围。工具只报告，不自动修复、去重、归档或删除。不要为了过检把当前正文标成历史或把所有文件加成导航根。

<a id="agents-local-check"></a>
## 根入口的本地行数检查

适用于包含 `tooling/agents_check.py` 的当前检出；在仓库根使用 Python 3 和 make，无需安装文档检查依赖、连接服务或取得环境凭据。新增团队命令、Skill 或修改入口时，按[入口维护规则](team-development.md#agents-maintenance)判断写回范围；可使用 [ming-runbook](../../.agents/skills/ming-runbook/SKILL.md)组织操作。检查只读取当前检出的根 `AGENTS.md`，不递归检查子目录入口，不改文件或业务数据。

```sh
make agents-check
```

命令输出 JSON；`lines` 是实际物理行数，空行计入，LF 与 CRLF 等价，末尾换行不额外计一行。`warning_at` 与 `maximum` 是当前阈值；`semantic_review=NOT_RUN` 明确表示未验证导航语义。

| 输出与条件 | 退出码 | 后续处理 |
| --- | --- | --- |
| `PASS`，少于 115 行 | 脚本和 make 均为 0 | 再核对五类入口职责及链接；行数正常不证明内容完整 |
| `WARN`，115–120 行（含两端） | 脚本和 make 均为 0 | 添加内容前审视细节归属；保留有效路标，不能把警告当作无需处理 |
| `ERROR`，超过 120 行 | 脚本为 1；make 报失败，通常为 2 | 将操作细节归位到已有维护正文，保留必要指针后重跑 |
| `ERROR`，文件不存在、为空、不是普通文件、为符号链接、无法按 UTF-8 读取或超过 2 MiB 读取上限 | 脚本为 1；make 报失败，通常为 2 | 按 `message` 核实当前检出、文件类型、编码或读取权限，修复根入口后重跑；不得删掉有效内容来消除错误 |

命令无法启动（如缺少 Python 3 或 make）属于环境失败，不会产生上述正常 JSON；补齐本地工具后重新执行。直接运行 `python3 tooling/agents_check.py` 可查看脚本结果与退出码。检查不写入任何文件，无需数据回滚；文档调整只撤销本次误改，保留他人变更及原现场。

修复后运行 `make agents-check` 和 `make knowledge-check`，分别报告行数/警告和链接、索引/回归结果。禁止通过挤行、删约束或缩小维护清单取得通过。人工还需核对根入口的任务路由、仓库地图、快速命令、关键约定速查、状态入口，以及命令语义与授权范围。

`agents-check` 是独立的本地按需命令，当前未加入 `knowledge-check`、`verify-pr`、Hook、CI 或分支保护；不要从本地成功推断已启用强制门禁。`knowledge-check` 的链接/索引检查也不能替代上述人工判断。后续调整接入方式时应同时更新命令实现和本节说明。

## 读取与解析边界

检查器拒绝越界、私有目录、`.env*`、符号链接和超出单文件 2 MB 的正文；先检查路径再读内容。指向范围外正文的锚点不会触发隐式读取。这覆盖了 r-doc v4 候选中“锚点检查绕过排除范围”的已知风险；复用仅限[纯文本解析及许可](../../tooling/vendor/rdoc/README.md)，不加载其修复/归档引擎。

这不是完整 CommonMark/HTML 渲染器：HTML href/src、复杂嵌套标签、缩进/块引用中的代码和平台特有锚点不作完整兼容承诺。关键导航使用上述受支持的 Markdown，评审仍核对实际展示效果。外链可用性、语义去重、内容过时、正文与代码业务一致性另行评审，本轮不据此启用全库门禁。
