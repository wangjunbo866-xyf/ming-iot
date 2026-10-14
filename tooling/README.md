# 仓内工具入口

从仓库根运行；命令只使用当前检出的文件，不要求相邻的 ai-native 或 ming-iot-rd-docs 仓库。

| 命令 | 用途与边界 |
| --- | --- |
| `make knowledge-check` | 核对维护中的文档链接、声明的 Skill 文件、参考仓排除边界，并运行入口和现有 ming-specs 回归 |
| `python3 tooling/knowledge.py where` | 输出当前检出内的知识入口 |
| `python3 tooling/knowledge.py specs -- --change <check.json> --base <commit> --head <commit>` | 调用现有 ming-specs 提交差异检查；暂存检查改用 `--staged`，不自动确认业务语义 |
| `make verify-pr` | 完整后端构建、单元测试、静态分析和契约检查，另外包含上述知识检查 |

已跟踪的 `.agents/skills/ming-specs` 是本仓候选发现入口；clone 即获得文件，无需安装脚本或外部符号链接。文件发现、宿主实际加载、团队采用及业务验收分别验证。

[knowledge.json](knowledge.json) 只维护入口和验证范围，业务正文仍在 [docs/specs](../docs/specs/README.md) 与 [团队约定](../docs/runbook/team-development.md)。`knowledge.py` 不创建规格、修改文档或启用 Hook。

## 实验与历史工具

[experiments](experiments/README.md) 保存 r-doc、portable map-delivery 和旧团队/研发工具。它们不通过根 Skill 入口自动加载，不参与默认安装；旧 manifest 中的 canonical、旧仓地址等描述只代表当时来源。旧 Gitee 发布、TeamAI 安装、父子仓发现命令不作为本仓开发步骤运行。

## M4 单仓 CI 与发布核对

参见 [验证说明](../docs/runbook/monorepo-ci.md) 和 [许可证地图](../LICENSES.md)。
本地技术验证与公开许可分别记录；当前候选保留权利待确认材料，公开状态为 HOLD。
