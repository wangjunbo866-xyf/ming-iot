# 旧 Core 附属材料

产品实现已在 `backend/saas`，本目录保存原 Core 的非产品入口资料。逐文件来源与内容摘要见 [LEGACY_MATERIALS.json](../../../meta/LEGACY_MATERIALS.json)。已被迁址或拆分替代的导出/RPC 文件不再复制；私有环境配置、私钥示例仍留在 M2 私有备份，未导入本树。

- 任务管理图片已补回 `backend/saas/doc/assets/任务管理.png`，修复 SaaS README 图片链接。
- [原本地联调记录](local-development.md) 仅供追溯；当前启动/验证入口是 [deploy](../../../../deploy/README.md)，不使用旧远端隧道或旧机器路径。
- `deploy/tencent-scf/google-jwks-fetcher/` 保留旧云函数说明及源码材料；它不是目标已有客户端的等价替代，也没有在本轮部署。
- `scripts/` 保留数据库和远端运维来源，`shell/` 保留旧 Git 发布脚本。可执行内容均以 `.source.txt` 材料形式保存；SQL 从未在业务库执行。需要恢复功能时先提出具体环境和动作，再按当前授权验证。

这些文件无需从旧兄弟仓读取，但其公开内容边界仍须在 M4 核查。归档可读不代表旧命令在新平台可执行。
