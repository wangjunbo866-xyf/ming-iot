# Ming IoT

Ming IoT 单仓。已有后端位于 `backend/saas`、`backend/share`、`backend/things`；旧 Core、Share、Things 的来源与 M2 增量保留在迁移记录中，不再建立并列产品仓。

- [日常开发入口](docs/runbook/daily-development.md)：从本仓 main 开始，在独立工作树修改，使用仓内知识与工具。
- [开发与验证](deploy/README.md)：`make help`、`make verify-pr` 和明确选择的本地集成套件。
- [知识导航](docs/README.md)：当前规格、团队约定、来源资料与迁移记录。
- [工具入口](tooling/README.md)：仓内运行，无需旧 ai-native 或研发资料兄弟仓。
- [协作约定](AGENTS.md)：任务范围、当前 Goal、验证与远程操作边界。

```sh
make knowledge-check
make verify-pr
python3 tooling/knowledge.py specs -- --help
```

ThingsBoard 和 AirMQ 是仓外学习参考，不纳入迁移、产品构建或日常开发依赖。产品网关、前端尚未提供实际源码，本候选不生成占位实现。迁移 PR #1 已合并；M5 负责日常入口和工具切换。旧仓与私有备份保留，小黄反馈任务已按用户要求废弃，未发生真人验收。当前状态和 LoopX 路由限制见日常开发入口。

## M4 单仓 CI 与发布核对

参见 [验证说明](docs/runbook/monorepo-ci.md) 和 [许可证地图](LICENSES.md)。
25 文件范围与本次 GitHub 候选推送已获确认；`publication-ready` 核对该范围和授权。已有来源限制继续保留，不据技术检查推定整仓权属、生产发布或 M5 验收。
