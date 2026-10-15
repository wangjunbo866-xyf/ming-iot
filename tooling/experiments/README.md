# 工具实验材料

这里保留原实验源码、测试和历史约定，便于继续比较与取舍。当前日常入口是 [仓内工具](../README.md)，没有因迁入而采纳 r-doc 或旧 map-delivery 流程。

| 材料 | 当前状态 |
| --- | --- |
| r-doc-v3、r-doc-v4 | 可离线运行原回归；模板 AGENTS 作为 `.source.txt` 保存，Skill 的模板链接已适配；模型评估和自动修文档需另选任务 |
| map-delivery-portable | 独立实验保留，不与 ming-specs 同时自动注册 |
| legacy-team-skills | 旧团队分发实现及来源锁；其多仓安装/发现契约保留历史，不作为新仓当前命令 |
| legacy-rd-tools | 旧研发资料校验、发布和安装工具；不自动将历史文档升级为现行规格，不运行旧远端写入入口 |

离线核验 r-doc 候选时，先为它准备 Python 虚拟环境，按候选根目录 `requirements-dev.txt` 安装依赖（包含 PyYAML 与 pytest）；然后用该环境的 Python 从仓库根运行 `python -m unittest discover -s tooling/experiments/r-doc-v4/skills/r-doc/tests`。不要假定系统 Python 已带 pytest。此结果仅说明该候选回归，不代表采用、模型效果或本项目全部文档符合 r-doc 模式。

候选目录按上游 `skills/r-doc` 层次补齐测试依赖；[来源清单](r-doc-support-sources.json) 绑定 M2 备份字节，细节见 [v4 迁移说明](r-doc-v4/MIGRATION.md)。配套上游文档仅供实验阅读。
