# 默认 OpenSpec 与可定制能力覆盖了多少全栈项目上下文？

Type: research
Label: wayfinder:research
Status: resolved
Assignee: article_research
Parent: [小团队运行方案与 IoT 试点决策地图](../map.md)
Blocked by:

## Question

依据 OpenSpec 当前官方文档与源码，核查规格与变更、项目上下文、定制 schema、跨仓 stores、AI 按需获取信息的能力与边界。区分默认即有、团队可配置、需要外部导航/检索/动态查询的部分，支持“仅 OpenSpec 能否实现天猫 collar 的全栈知识库效果”的判断。

## Context

本调查服务于 [如何用最少的人工维护使规格、实现与证据持续对齐？](05-document-maintenance.md)，不重新决定已经选定的 OpenSpec，也不执行安装或迁移。研究分支：`research/openspec-project-context-20261010`。

## Answer

2026-10-10 已完成官方文档与固定源码核查。OpenSpec 自带规格/变更及其依赖、上下文注入、宿主 Agent 的按需发现指引、自定义 schema 和 beta stores；可作为全栈知识库的规格中枢。默认安装不提供完整业务到代码/运行的导航、各类长期知识维护或业务运行配置查询服务，需要团队补充这些内容和读取通道。

- [OpenSpec 项目上下文能力研究](/home/wangjunbo/.cache/ai-native-wayfinder-worktrees/openspec-context-20261010/docs/research/2026-10-10-openspec-project-context.md)
- 研究提交：`361f713b23e3157730d01c453d2cf00e376fc37f`；可恢复内容为该提交下 `docs/research/2026-10-10-openspec-project-context.md`。
- 上游版本：main `9111a7654d7800391459431fff4eaf66e33a3d2e`；v1.14.1 tag `87c3595ace6a2e22957f39ebe5b74c2f2316e0cb`；六个关键文件逐字节对照一致，其余引用分别标明主线来源。
- 边界：explore 是宿主 Agent 的发现协议，不是 CLI 已完成全库检索；stores references 索引 canonical specs，不是任意 wiki 或运行数据。现行架构也不能简单等同历次 Change design 的堆积。
- 未安装或运行 OpenSpec/存储/定制 schema；仓库 52 项测试和技能校验通过，仅证明研究所在仓库的既有检查通过。本票据解决能力事实，团队组织方式仍由关联设计票据决定。
