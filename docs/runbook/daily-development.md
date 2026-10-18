# 日常开发入口

目标、阶段进度和完成标准统一从 [总纲](../project-charter.md) 读取；本页只维护日常操作入口。

产品源码、当前文档、团队工具和 CI 的入口统一为 [ming-iot](https://github.com/wangjunbo866-xyf/ming-iot)。首次开展独立任务时从 `main` 创建分支和工作树；同一候选上的后续阶段优先复用已有工作树，不因阶段变化重复新建。改动通过 PR 合入。

```sh
git clone https://github.com/wangjunbo866-xyf/ming-iot.git
cd ming-iot
git worktree add -b feature/my-change .worktrees/my-change main
cd .worktrees/my-change
python3 -m pip install -r tooling/requirements-docs.txt
make knowledge-check
python3 tooling/knowledge.py where
make verify-pr
```

主规格从 [业务地图](../specs/README.md) 进入；认领、需求确认、Patch 双向指针和提交差异提醒遵循 [团队约定](team-development.md)。`.agents/skills/ming-specs` 随 clone 获取；宿主实际加载和团队正式采用需分别确认。

历史研发材料统一放在 [wiki/legacy](../wiki/legacy/README.md)，包括 `rd` 与 `superpowers`。历史工具在 `tooling/experiments`，不自动安装或发现。文档来源状态在 [index.yaml](../index.yaml)，机器检查清单在 [tooling/policy](../../tooling/policy/README.md)。

## 跨 Agent 与普通开发

新成员或更换 coding agent 时读[通用接入](cross-agent-development.md)。普通任务复用原任务/PR 的认领、决定和证据；不运行 Goal 初始化，不要求维护者私有配置。只有明确纳入 LoopX 的任务才继续下节，并遵循 [LoopX 任务约定](loopx-development.md)。

## LoopX 本机接入

已有 `ming-iot-knowledge-pilot` 的维护机器继续使用同一 Goal、Agent 身份和 SQLite 数据库。经隔离验证及独立复审，已通过 LoopX 项目迁移入口把注册和 `.loopx/goals/` 状态目录迁入本仓；共享运行时、数据库与完整历史留在原位。此次采用固定本地候选，恢复时需使用支持该路由协议的运行包。具体证据与验收边界见 [M5 记录](../changelog/2026-10-15-m5-cutover.md)。

在 Git 主工作区创建被忽略的 `.local/loopx.json`，填写本机已有 CLI 和 Registry 的绝对路径：

```json
{
  "binary": "/absolute/path/to/loopx",
  "registry": "/absolute/path/to/registered/registry.json"
}
```

`tooling/loopx` 从 Git 公共目录定位该配置，独立工作树复用同一配置。它只转发参数，配置缺失即失败；不连接新项目、不建 Goal、不复制 Registry 或数据库。新同事不复制维护者的私有文件，应按自己获授权的 LoopX 项目完成接入；普通构建和知识检查不依赖 LoopX。

```sh
tooling/loopx --format json status --goal-id ming-iot-knowledge-pilot
```

Agent 的进展写回继续使用原准入身份和 Turn，并通过 `--delivery-workspace-path` 指定实际产出所在的独立工作树。该参数记录交付位置，不改变 Goal 注册路径。自动化使用稳定的主工作区入口，不指向临时工作树。

## 保留与恢复

清理旧工作树前，确认其提交已合并或被当前保留候选包含，并核对未提交文件、被 Git 忽略的数据、证据、进程与容器挂载、符号链接和工具入口。先保全并恢复核验，再移除目录或失效登记；保留分支和提交历史，不将“移除工作树”当作“删除分支”。当前候选及其运行环境继续保留，已验证的历史证据迁入私有备份后保持可恢复。

旧源码仓、原候选、私有备份继续保留，日常代码和文档改动在本仓进行。旧项目 Registry 已标记为退休，旧状态文件作为迁移前快照保留；当前写入由本仓注册位置承接。路由核验仍读取退休标记，部分历史材料仍引用旧来源，不能据此删除旧目录。

如新入口失效，先核对 `.local/loopx.json` 指向已审运行包，再验证同一 Goal 的身份、Todo 和历史。旧版本会拒绝新路由，不能将换回旧二进制当作恢复。迁移中断只能按保留的原计划和摘要，通过迁移入口重试；先停止调度并排空在途写入。代码回退使用新的修复 PR，不覆盖旧仓、不重写主分支、不从备份重建同名 Goal。迁移计划、原始回执和备份仅私有保存。

Codex Project 是工作区入口。创建或切换到 ming-iot Project 不需要创建第二个同名 Goal；当前对话可以继续使用上述稳定入口。明确继续该 Goal 的新聊天应读取现有 Goal，而不是重新初始化；普通开发聊天按通用入口开始。

小黄反馈及依赖该反馈的任务已停止并废弃。现有代码与文档仍可供开发者理解和验证，历史 AI 交接演练保持原结论，未记录真人验收通过。
