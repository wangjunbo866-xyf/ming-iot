# 仓内工具入口

从仓库根运行；命令只使用当前检出的文件，不要求相邻的 ai-native 或 ming-iot-rd-docs 仓库。运行知识检查前，首次在自己的 Python 环境安装 `python3 -m pip install -r tooling/requirements-docs.txt`；`agents-check` 只使用 Python 3 标准库。检查器本身不下载或安装依赖。

文档分类、失败含义和未验证边界见[文档检查说明](../docs/runbook/document-checks.md)。Markdown 解析的选择性复用及许可见[r-doc 来源记录](vendor/rdoc/README.md)。

| 命令 | 用途与边界 |
| --- | --- |
| `make agents-check` | 按需检查根 AGENTS.md 行数与可读取性；阈值、退出码和失败处理见[本地检查](../docs/runbook/document-checks.md#agents-local-check)，不随知识检查自动运行 |
| `make knowledge-check` | 按显式范围检查链接/锚点、入口可达性、来源索引和替代关系，并运行工具及 ming-specs 回归 |
| `python3 tooling/knowledge.py where` | 输出当前检出内的知识入口 |
| `python3 tooling/knowledge.py specs -- --change <check.json> --base <commit> --head <commit>` | 调用现有 ming-specs 提交差异检查；暂存检查改用 `--staged`，不自动确认业务语义 |
| `python3 tooling/specs_pr.py --base <target-tip> --head <commit> --declaration <JSON路径>` | 检查完整 PR 三点差异、模块/条款、Patch 指针及历史归档；格式见 [PR 约定](../.agents/skills/ming-specs/references/pr-check.md) |
| `make verify-pr` | 完整后端构建、单元测试、静态分析和契约检查，另外包含上述知识检查 |

本仓可选 Skill 的文件随 clone 获得，无需安装脚本或外部符号链接：

| 要做什么 | 候选入口与使用示例 |
| --- | --- |
| 维护业务规格和提交差异 | [ming-specs](../.agents/skills/ming-specs/SKILL.md) |
| 沉淀操作方法、维护入口与团队命令 | [ming-runbook](../.agents/skills/ming-runbook/SKILL.md)；例如 `$ming-runbook 补齐新检查命令的运行方法、结果判读和受影响入口` |

在支持 Skill 的宿主中显式调用；未发现时从仓库 AGENTS.md 沿链接读取 Skill 并按任务操作。文件发现、宿主实际加载、团队采用及业务验收分别验证；候选文件存在不表示团队全面采用。维护触发及正文归属以[团队约定](../docs/runbook/team-development.md#runbook-maintenance)为准。

[knowledge.json](knowledge.json) 只维护入口和验证范围，业务正文仍在 [docs/specs](../docs/specs/README.md) 与 [团队约定](../docs/runbook/team-development.md)。`knowledge.py` 不创建规格、修改文档或启用 Hook。

`tooling/loopx` 从 Git 主工作区的 `.local/loopx.json` 读取本机 CLI 和 Registry 路由。配置缺失时明确失败；不创建新 Goal，不复制数据库。接入步骤见 [日常开发入口](../docs/runbook/daily-development.md)。

[policy](policy/README.md) 保存机器检查清单；文档来源索引位于 [docs/index.yaml](../docs/index.yaml)。

## 实验与历史工具

[experiments](experiments/README.md) 保存 r-doc、portable map-delivery 和旧团队/研发工具。它们不通过根 Skill 入口自动加载，不参与默认安装；旧 manifest 中的 canonical、旧仓地址等描述只代表当时来源。旧 Gitee 发布、TeamAI 安装、父子仓发现命令不作为本仓开发步骤运行。

## M4 单仓 CI 与发布核对

参见 [验证说明](../docs/runbook/monorepo-ci.md) 和 [许可证地图](../LICENSES.md)。
已批准的 25 文件公开范围继续有效；来源及许可限制见 [公开策略](policy/PUBLICATION_POLICY.json)。迁移 PR #1 已合并，后续改动仍需检查和独立复审。
