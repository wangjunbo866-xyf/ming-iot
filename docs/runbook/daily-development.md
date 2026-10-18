# 日常开发入口

目标、阶段进度和完成标准统一从 [总纲](../project-charter.md) 读取；本页只维护日常操作入口。

产品源码、当前文档、团队工具和 CI 的入口统一为 [ming-iot](https://github.com/wangjunbo866-xyf/ming-iot)。独立改动从 `main` 创建分支；需要同时维护多个改动时可按需使用工作树。已有候选继续在原分支上迭代，避免覆盖他人改动。改动通过 PR 合入。

```sh
git clone https://github.com/wangjunbo866-xyf/ming-iot.git
cd ming-iot
git switch -c feature/my-change
python3 -m pip install -r tooling/requirements-docs.txt
make knowledge-check
python3 tooling/knowledge.py where
make verify-pr
```

主规格从 [业务地图](../specs/README.md) 进入；认领、需求确认、Patch 双向指针和提交差异提醒遵循 [团队约定](team-development.md)。[ming-specs](../../.agents/skills/ming-specs/SKILL.md) 与 [ming-runbook](../../.agents/skills/ming-runbook/SKILL.md) 随 clone 获取，分别维护业务规格和操作知识/入口；宿主实际加载和团队正式采用需分别确认。

历史研发材料统一放在 [wiki/legacy](../wiki/legacy/README.md)，包括 `rd` 与 `superpowers`。历史工具在 `tooling/experiments`，不自动安装或发现。文档来源状态在 [index.yaml](../index.yaml)，机器检查清单在 [tooling/policy](../../tooling/policy/README.md)。

## 接手与协作

新成员或更换 coding agent 时读[通用接入](cross-agent-development.md)。范围、决定和证据复用原任务/Issue/PR，项目规则与操作方法随源码获取。

## 保留与恢复

清理旧工作树前，确认其提交已合并或被当前保留候选包含，并核对未提交文件、被 Git 忽略的数据、证据、进程与容器挂载、符号链接和工具入口。先保全并恢复核验，再移除目录或失效登记；保留分支和提交历史，不将“移除工作树”当作“删除分支”。当前候选及其运行环境继续保留，已验证的历史证据迁入私有备份后保持可恢复。

代码回退使用新的修复 PR，保留他人未合并改动、数据和必要的恢复依据。迁移来源、私有备份和历史回执的保留要求见[日常入口切换记录](../changelog/2026-10-15-m5-cutover.md)。

<a id="loopx-本机接入"></a>
## 可选工具

明确采用 LoopX 的任务才读取[专用接入与执行说明](loopx-development.md)；普通开发按上面的步骤完成。
