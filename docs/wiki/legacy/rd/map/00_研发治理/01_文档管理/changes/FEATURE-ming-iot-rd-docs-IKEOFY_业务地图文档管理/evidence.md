---
document_kind: evidence
status: verified
delivery_id: ming-iot-rd-docs#IKEOFY
issue: https://gitee.com/ming-industry/ming-iot-rd-docs/issues/IKEOFY
hub: map/00_研发治理/01_文档管理/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/README.md
repository_heads:
  - git:gitee.com/ming-industry/ming-iot-rd-docs@bbcd4d1aae007223d5cdddc61d290bae1e56dc4a
  - git:gitee.com/ming-industry/ming-iot-rd-docs@b344699ad266f8fc5b7d06d24a1a85e9446b2d8f
  - git:gitee.com/ming-industry/ming-iot-rd-docs@b362ea3843f96747923c3c0b693ffcab604ec057
---

# 阶段 A PR !9 修复验证摘要

[Change Hub](./README.md) · [Review](./review.md) · [Plan](./plan.md)

| repository | base | exact_head | verdict |
|---|---|---|---|
| git:gitee.com/ming-industry/ming-iot-rd-docs | 18b15dcbcb2d0ea63d25e0837eec0f69d055930b | bbcd4d1aae007223d5cdddc61d290bae1e56dc4a | verified |
| git:gitee.com/ming-industry/ming-iot-rd-docs | 18b15dcbcb2d0ea63d25e0837eec0f69d055930b | b344699ad266f8fc5b7d06d24a1a85e9446b2d8f | verified |
| git:gitee.com/ming-industry/ming-iot-rd-docs | 18b15dcbcb2d0ea63d25e0837eec0f69d055930b | b362ea3843f96747923c3c0b693ffcab604ec057 | verified |

## 2026-09-17 N4/N5 修复结果

依据：[Owner 对 e694424 的独立复审](https://gitee.com/ming-industry/ming-iot-rd-docs/pulls/9#note_51242592)。该复审关闭 N1～N3 及模板 D1，要求修复 N4/N5；本节是执行者验证回执，不是新候选的独立审查结论。

- N4：普通 Review/Evidence 表检查复用既有完整提交 SHA 校验器检查 `base`。两类文档的真实 CLI 正反控制覆盖完整 SHA 通过，以及 `master`、`origin/master`、短 SHA、空值、非十六进制 40 字符拒绝。非空的可变/无效基线在修改前会被放行，修改后拒绝；空值原有拒绝行为继续保留，不改变 SHA 大小写兼容规则。
- N5：canonical Skill 明确新模块先写完整初版 Spec、验收与真实锚点，再初始化引用它的 Change/Plan；初版存在不等于实施或生效。既有模块继续增量流程，缺少既有需求时记录缺口，不以“新模块”绕过。`__module_root__` 仅保留历史/过渡兼容，不再是新交付推荐顺序。
- Skill 行为核对：独立子 Agent 仅依据修改前两份 Skill 文件，对“获批新模块、紧急 demo、建议先 scaffold”场景选择先 init；修改后的独立场景核对改为先 Spec 再 Change，并对既有 PATCH 保持现行规格、对缺失既有需求停止初始化。没有真实安装、执行新交付或代替 Owner Review。
- 补充受限 Markdown 排版说明：必要导航优先独立段落或顶层列表；四空格嵌套列表、双反斜杠前链接仍可能误拒，不宣称完整 CommonMark 支持，不扩大解析器改造。

| 命令 | 本轮结果 |
|---|---|
| python3 -m unittest discover -s tests -q | 182 项通过，退出 0，包含归档及真实 Hook 回归；保留既有非失败 socket ResourceWarning |
| python3 tools/map_docs.py check --docs-root . | map docs: OK |
| python3 tools/check_delivery_metadata.py --docs-root . --since 2026-09-03 | delivery metadata: OK |
| git diff --check / sh -n .githooks/pre-push | 均退出 0 |
| Skill quick_validate.py | Skill is valid |

本轮四个实现/测试/Skill 文件边界扫描通过；控制面另报等待复审投影需更新，不是文件泄漏。上表 `b362ea3` 固定本轮实现；其后只追加 Hub/Evidence，最终 PR Head 在平台及正文绑定，交回独立复审。未运行云端 CI、正式 Skill 注册、阶段 B/C 或合并。

## 2026-09-17 N1～N3 修复结果

依据：[Owner 对 959b732 的 REQUEST_CHANGES](https://gitee.com/ming-industry/ming-iot-rd-docs/pulls/9#note_51240711)。本节是执行者本地验证，不是 Owner 独立审查通过。

| 反馈 | 实现及验证 |
|---|---|
| N1：代码样式规格值漏检 | 使用等长遮罩定位真实锚点，再比较原始段落；行内代码、fenced YAML、缩进代码及普通正文变更提交后，默认 check 和真实 pre-push 均拒绝冲突；未改历史仍通过，代码中的伪锚点不成为段落边界 |
| N2：不可见文字计入导航 | 核对完整闭合/标题区间，排除跨空段落的伪链接、title 内引用和段落伪定义；合法 inline/full/collapsed/shortcut、标题、围栏/分隔线后引用继续通过 |
| N3：跨仓 Hub 被拒绝 | 产品 Issue 可确认文档仓 map Change README；文档同仓继续可用，错误组织/仓库/路径拒绝且零 HTTP；正向场景两次 GET、零正文写入 |
| 模板同步 | 明确退出 2 → 有权限的人补唯一导航块 → 回读确认；不再要求工具自动写入或禁止人工补块 |

首次 5 项定向测试出现 10 个预期失败，分别覆盖以上缺陷；本地只读增量复核又发现“围栏/分隔线后的合法引用误拦”和“跨空段落伪闭合仍放行”。补充反例先失败，再修复；保留原始正文的块边界，避免调用者提前删除围栏后再解析。原有失效引用测试增加定义前空行，继续验证真正的失效引用而非普通段落文字。

| 命令 | 本轮结果 |
|---|---|
| python3 -m unittest discover -s tests -q | 181 项通过，退出 0；包含归档及真实 Hook 回归 |
| python3 -m unittest tests.test_map_docs -q | 78 项通过，退出 0 |
| python3 tools/map_docs.py check --docs-root . | map docs: OK |
| python3 tools/check_delivery_metadata.py --docs-root . --since 2026-09-03 | delivery metadata: OK |
| git diff --check / sh -n .githooks/pre-push | 均退出 0 |

测试使用临时文档/Git 仓库与 dummy token 的回环 HTTP，无真实 Issue 正文写入。保留既有非失败 socket ResourceWarning。LoopX 边界扫描对本轮工具、测试、模板及主规格 6 文件通过；扫描整个模块时，原 Plan 未变化的 `gitee_draft_pr_automation_and_rd_docs_authorization` 模块路径触发 credential 误报，已与被审 Head 对照确认是原有文档路径，不是凭据，未为消除误报改写历史。

上表新代码 Head 固定了工具、测试和模板；后续文档提交包含用户已要求的主规格层级修订和本回执。最终 PR Head 在 PR 正文/平台绑定，须包含全部增量交回 Owner 复审。未执行云端 CI、正式 Skill 注册、阶段 B/C 或合并。

## 历史结果（2026-09-13，bbcd4d1）

在独立文档工作树执行，代码/测试 blob 与上表候选一致：

| 命令 | 结果 |
|---|---|
| python3 -m unittest discover -s tests -q | 175 项通过，退出 0 |
| python3 -m unittest tests.test_map_docs tests.test_gitee_issue tests.test_repository_policy -q | 110 项通过，退出 0 |
| python3 tools/check_delivery_metadata.py --docs-root . --since 2026-09-03 | delivery metadata: OK |
| python3 tools/map_docs.py check --docs-root . | map docs: OK |
| git diff --check | 退出 0 |
| sh -n .githooks/pre-push | 修复执行者复跑，退出 0 |

## R1～R3 对应验证

来源：[Owner REQUEST_CHANGES](https://gitee.com/ming-industry/ming-iot-rd-docs/pulls/9#note_51185613)，被审查 Head 为 `606dbd5ce4645102898936d25649e080cb7a2b92`。以下是执行者修复与验证回执，不是独立复审结论。

| 反馈 | 修复 | 回归证据 |
|---|---|---|
| R1：GET/PATCH/GET 并发覆盖正文和标题 | 无已验证远端 CAS 能力时停止自动 PATCH；生成导航块或只读确认；人工待办明确退出 2 | HTTP fixture 在首个 GET 快照后、响应前插入人工标题/正文修改，确认没有 PATCH、不丢人工编辑、不误报 linked；已有块并发追加可正确回读 |
| R2：默认门禁遗漏 effective 候选 | check 相对可信 Git 基线识别候选，复用 activate；默认 merge-base(origin/master, HEAD)，无基线保守检查 | 缺 Review/Evidence、活动冲突均拒绝；普通 verifying、合法 effective、未变化历史 effective 允许；目标段落变化重新核验；真实 pre-push 对已经提交的错误候选仍拒绝 |
| R3：引用定义冒充导航 | 分离实际使用链接与仅待校验定义 | 根 README/Hub 只有未使用定义时拒绝；full/collapsed/shortcut 使用通过；图片、转义引用不计导航；未使用定义失效目标仍拒绝 |

R1～R3 的关键反例均先在旧实现得到意外通过/错误行为，再修复为当前通过。验证使用本地临时 Git 仓库和回环 HTTP；未制造真实 Gitee 并发写入。新增 16 项测试，完整测试数由 159 增至 175。测试期间出现既有 PR 工具的非失败 socket ResourceWarning，未扩大范围修复。

## 测试先行与隔离

### Skill 可用性建议补充（2026-09-13）

**后续用户范围决定：** 未迁移模块由用户手工迁移。本轮将 Skill 入口改为 map → 模块 README → 当前 spec / Change Hub，移除下述早期修订中的 Catalog 依赖及未迁移回退。保留现行/候选判断、参数化命令与失败分流；缺少既有规格只记录缺口，不自动迁移。本次未删除 Catalog、未改旧目录或执行阶段 B/C。以下 Catalog/三分支描述仅记录上一轮修改，不作为最新指引。

上轮只完成命令示例标注，未完整处理 Owner 的五条 Skill 建议。本轮补齐两份 Skill 文件：移出硬编码阶段/交付例外、参数化 init、拆分新模块/已迁移/未迁移读取路径、直链 Catalog 并定义当前权威、引用既有状态说明、增加三类失败分流，以及将 PATCH/BUGFIX/SUNSET 明确归为 Change TYPE。

- 修改前检索场景明确返回“当前阶段查询入口未规定”“明确文件路径未规定”，普通文档错误与工具缺陷处理也未规定；修改后的独立场景参与者能从两文件找到对应路径和下一动作。这是 Skill 指引测试，不是 Owner PR Review，也不证明跨宿主实际加载效果。
- 主执行者核验 Skill 全部相对链接/章节指针；在临时目录执行参考文件原命令块：缺变量时拒绝且零文件写入，设备模块参数初始化到设备模块而非治理/IKEOFY。未连接真实 Issue；使用格式合法的合成编号。
- 实测同时确认 init 只生成骨架，新目录缺索引时 check 应拒绝；参考文件已拆开 init、正文/索引补齐与 check 的顺序，没有把骨架生成写成完整交付通过。
- 本轮未修改 Python 工具或测试，完整 175 项测试与 map/旧元数据检查通过，保留既有非失败 ResourceWarning。最新 Skill/回执增量的 exact Head 绑定在 PR 正文，仍待 Owner 独立复审。

- Map checker/init 从缺少 API 的失败测试开始；activate 的 8 项初始测试先全部失败，再通过。
- Issue 工具仅测试本地 HTTP 服务与 dummy token；重定向、身份不匹配、重复/损坏托管块和错误脱敏均有回归。
- 注册只在临时双仓 fixture 内验证；越界路径、反向忽略和源文件符号链接均先 RED 再 GREEN。
- 激活最终三项来源/锚点/历史索引漏检先复现错误通过，再修复为拒绝。
- 归档清单与既有测试在整仓测试中继续通过；未修改 archives 或产品仓跟踪文件。

## 证据适用范围

verified 只指以上本地验收，不包含云端 CI、真实 Issue 反链和跨仓合入。旧 `review.md` 的通过仅绑定 `ea78594c4e2715f24b4e2761154cb99e19f25a3b`，不能覆盖本次修复 Head；当前等待 Owner 独立复审。最终 PR Head 在平台和 PR 正文绑定，并包含本验证摘要的文档增量，避免摘要自引用提交哈希。未合并、不推进 B/C；正式注册与权威激活仍按前置合入条件执行。
