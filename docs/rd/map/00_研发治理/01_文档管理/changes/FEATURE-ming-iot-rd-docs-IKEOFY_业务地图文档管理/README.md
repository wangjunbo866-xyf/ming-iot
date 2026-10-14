# IKEOFY：业务地图文档管理

本文件是本次交付的导航中心。从 Issue、Change Spec、实施 Plan 或 PR 任一入口进入，都应能回到这里并找到其余对象。

## 交付导航

| 对象 | 状态 | 入口 |
|---|---|---|
| 模块主规格 | 初版规范，当前修订尚未合入生效；先于本交付读取 | [spec.md](../../spec.md) |
| Delivery Issue | 已建立 | [ming-iot-rd-docs#IKEOFY](https://gitee.com/ming-industry/ming-iot-rd-docs/issues/IKEOFY) |
| Change Spec | Verifying（阶段 A 候选，未激活） | [change.md](./change.md) |
| 实施 Plan | Approved | [plan.md](./plan.md) |
| 设计与计划 PR | 已合入 | [ming-iot-rd-docs!8](https://gitee.com/ming-industry/ming-iot-rd-docs/pulls/8) |
| 阶段 A 文档基础能力 PR | Open、未合并；N4/N5 修复候选待独立复审，云端 CI 待验证 | [ming-iot-rd-docs!9](https://gitee.com/ming-industry/ming-iot-rd-docs/pulls/9) |
| 产品仓规则 PR | 已纳入 Plan 阶段 B | 待关联 |
| Review | Owner 已关闭 N1～N3/D1；N4/N5 新 Head 待复审 | [历史 review.md](./review.md) · [Owner N4/N5](https://gitee.com/ming-industry/ming-iot-rd-docs/pulls/9#note_51242592) |
| Evidence | 阶段 A 本地验证通过，云端 CI 待验证 | [evidence.md](./evidence.md) |

## 当前阶段

2026-09-17 Owner 对 `e6944249a7a1fde5b7e12c26612ef5a275a134d4` 的独立复审关闭 N1～N3/D1，提出 N4 完整 base SHA 校验、N5 新模块 Spec-first 两项修复。实现候选 `b362ea3843f96747923c3c0b693ffcab604ec057` 已通过本地 182 项测试及 Skill 场景核对；本 Hub/Evidence 为后续回执增量，最终 Head 以 PR 正文/平台为准。仍待独立复审，不合并、不推进 B/C。

2026-09-17 回读确认 PR !8 已合入、PR !9 仍 open。Owner 对 `959b7327c78a63a359db8644f68e252562630b4b` 的复审关闭旧 R1，但发现规格代码值漏检、不可见导航及双仓 Hub 拒绝问题。本轮在现有工具中修复 N1～N3，同步 Issue 模板，并一并交付下述已获准的主规格层级修订；新候选与验证记录见 Evidence/PR 正文，仍需独立复审。没有合并，也没有推进 B/C。

2026-09-13 用户要求修正主规格与 Change 的层级：模块 `spec.md` 独立定义完整要求，IKEOFY 为其首次落地交付，Plan 负责实施。原先先写 Change/Plan、后补主规格的历史保留；本次仅修正文档职责及计划中的编写顺序，不视为阶段 B/C 执行或激活完成。以下测试和 Review 均是其所标注候选的历史事实，不自动覆盖本次本地修订。

2026-09-13 Owner 对 `606dbd5ce4645102898936d25649e080cb7a2b92` 提出 REQUEST_CHANGES。修复代码 Head 为 `bbcd4d1aae007223d5cdddc61d290bae1e56dc4a`，本地 175 项测试通过；最终 PR Head 还包含本回执文档增量，须一起复审。旧候选通过不适用于修复后的 Head。本轮只处理 R1～R3、测试、原 PR 更新与独立复审交接；阶段 B/C 及合并继续暂停。

1. Change Spec 与实施 Plan 已于 2026-09-10 通过评审。
2. 设计与计划 PR !8 于 2026-09-10 合入，合入 revision：`18b15dcbcb2d0ea63d25e0837eec0f69d055930b`。
3. 阶段 A 从该合入 revision 创建候选，按获批计划实现规则、工具、Skill 和门禁。
4. 阶段 A 至 C 形成候选 Head 后补齐 Review、Evidence 与相应 PR 链接。

阶段 A 的 base 为 `18b15dcbcb2d0ea63d25e0837eec0f69d055930b`，独立复核的实现 exact head 为 `ea78594c4e2715f24b4e2761154cb99e19f25a3b`。后续回执和导航提交只追加文档，最终 PR Head 在 PR 正文中绑定并增量复核。阶段 B 等阶段 A PR 合入，阶段 C 等阶段 B PR 合入。旧顶层目录继续作为当前权威，待阶段 C 完成后切换。

## 阶段 A 验证边界

Owner 的 Skill 可用性建议已补齐；用户随后明确未迁移文档由其手工处理，因此最新 Skill 仅从 map 定位，不依赖 Catalog 或旧目录回退。参数化命令、读取分支、失败分流及候选/现行边界保留。验证详情见 Evidence 的 Skill 补充；独立复审应绑定 PR 正文中的最新 Head，不能沿用旧 Head 通过。

- CLI、Hook、Skill 注册和 Issue 工具通过本地临时文档树/回环 HTTP 验证；尚未正式注册真实工作区或更新 Issue 正文。
- Gitee Workflow 配置使用 Python 3.12；云端运行器版本支持、PR 触发和阻断效果待真实流水线验证，不能以本地测试替代。
- `activate` 只核对候选树记录的仓库、Head、前置合入状态和来源指针，不联网验证，也不执行合并。
