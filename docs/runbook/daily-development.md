# 日常开发入口

产品源码、当前文档、团队工具和 CI 的入口统一为 [ming-iot](https://github.com/wangjunbo866-xyf/ming-iot)。从 `main` 创建分支和独立工作树；改动通过 PR 合入。

```sh
git clone https://github.com/wangjunbo866-xyf/ming-iot.git
cd ming-iot
git worktree add -b feature/my-change .worktrees/my-change main
cd .worktrees/my-change
make knowledge-check
python3 tooling/knowledge.py where
make verify-pr
```

主规格从 [业务地图](../specs/README.md) 进入；认领、需求确认、Patch 双向指针和提交差异提醒遵循 [团队约定](team-development.md)。`.agents/skills/ming-specs` 随 clone 获取；宿主实际加载和团队正式采用需分别确认。

历史研发材料统一放在 [wiki/legacy](../wiki/legacy/README.md)，包括 `rd` 与 `superpowers`。历史工具在 `tooling/experiments`，不自动安装或发现。文档来源状态在 [index.yaml](../index.yaml)，机器检查清单在 [tooling/policy](../../tooling/policy/README.md)。

## LoopX 本机接入

已有 `ming-iot-knowledge-pilot` 的维护机器继续使用同一 Goal、Agent 身份和 SQLite 数据库。当前 LoopX 不支持该 canonical Goal 跨项目根迁移，因此本轮只切换命令和自动化入口；原注册目录仍是运行依赖，暂不能删除。此项是 M5 的未完成条件，不能据新入口可用宣称物理迁移成功。

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

旧源码仓、原候选、私有备份继续保留，日常代码和文档改动在本仓进行。旧 LoopX 注册区暂时仍有运行写入；它不能被当成完全停用目录。

如新入口失效，先恢复本机路由配置并验证原 Goal 的身份、Todo 和历史。代码回退使用新的修复 PR；不覆盖旧仓、不重写主分支、不从备份重建同名 Goal。数据库迁移须等支持 canonical relocation 的正式接口，并单独验证一致性。

小黄反馈及依赖该反馈的任务已停止并废弃。现有代码与文档仍可供开发者理解和验证，历史 AI 交接演练保持原结论，未记录真人验收通过。
