# 是否让 map-delivery 独立承接业务规格与变更，吸收 OpenSpec 的成熟机制？

Type: grilling
Label: wayfinder:grilling
Status: open
Assignee:
Parent: [小团队运行方案与 IoT 试点决策地图](../map.md)
Blocked by: 09, 13

## Question

用户认为 OpenSpec 加业务地图会增加两套 Skill 和适配维护，提出独立发展 map-delivery、吸收 OpenSpec 优点的路线。比较独立实现与单入口封装 OpenSpec 的实际成本、已有基础、缺口和首个试点通过条件，判断哪个更适合以业务功能为入口的小团队。

随后用户补充天猫六个 collar-* Skill 的拆分方式。本次同时区分两个维度：底层规格/工具是否依赖 OpenSpec，以及 Agent 指导是否按知识职责拆分。给出可组合的方案，不把多 Skill 误认为必须存在多套文档权威或多个调度器。

本票据重新评估此前工具选择，不把“是否更优”当作已批准重写或迁移。规格和变更只能有一处权威、LoopX 负责长程推进、独立审查与证据验收等原则继续成立；OpenSpec 产品本身的取舍可以调整。

## Context

- [OpenSpec 全栈上下文能力核查](09-openspec-project-context.md)
- [主规格与 Patch 的关联机制](13-patch-backlinks.md)
- [现有薄导航样例与接入问题](10-context-adoption.md)
- 现有团队 Skill 源：`ai-native/skills/map-delivery/`；项目校验器位于 `ming-iot-rd-docs/tools/map_docs.py`。可移植草案的上下文查找器不等于完整独立交付工具。

## Assessment — 2026-10-10

- [独立 map-delivery 选型评估](../assets/14-standalone-map-delivery-assessment.md)
- 建议：把独立 map-delivery 作为有界试点的首选；依据是业务模型更贴合、现有模块交付基础可复用及适配成本，而非 Skill 数量本身。单入口封装 OpenSpec 仍是合理备选。
- 原项目的 init/check/activate、影响模块和精确版本规则已有代码基础；自动索引、可复核的主规格演进及通用化边界仍需补齐。可移植草案不是完整交付引擎。
- 独立只读复核补充：锚点冲突检查当前仅覆盖主 target_module；版本校验核对文档中的三元组，不验证远端事实或独立审查身份。相关限制已计入评估，避免高估现有自动化。
- 本轮是分析建议，等待用户对路线的后续选择；不把问“是否更优”记录为已经批准迁移或重写。旧 OpenSpec 接入工作据此先不继续推广。
- 用户追加多 Skill 方案后，已核读天猫 PDF 第 4–5 页与图表，并在评估中补充六类知识职责、共享契约、兼容入口、按需触发和遥测场景。独立工具路线与模块化 Skills 可以组合，尚未创建或安装这些候选 Skills。
