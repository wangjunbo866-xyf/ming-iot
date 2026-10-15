# ming-specs 候选基线失败修复

- 日期：2026-10-14；Goal：`ming-iot-knowledge-pilot`；任务：`todo_a60b014dd3c41750e09a7d71`。
- 起点：`e3005cf7c0653aa2665e25e81fe9f7588615ff7b`，分支 `codex/team-specs-draft`。
- 用户授权：定位并修复两项基线测试失败，更新 AGENTS.md 旧 Goal 入口，对最终候选完成整体验证和独立复审。
- 本记录说明修复依据；最终候选完整 SHA、完整验证及独立复审由当前 Goal 登记的 `ming-specs-baseline-repair-final-20261014` 回执绑定，避免报告自引用提交。

## 认领与验收

本次属于构建验证样本的实现修正，业务规格无变化。实施身份为 `rdoc-pilot-codex`，独立评审者由固定候选回执记录。范围限 `backend/tools/internal/thingsbaseline/verify_test.go`、`AGENTS.md`、本文及文档目录登记；Things 业务源码、构建模板、生产执行器和通知 R2 均保持不变。

完成条件是原两项失败及新增隔离反例通过、完整 `make verify-pr` 对最终候选通过、入口解析到当前 Goal，以及独立复审绑定同一最终提交。局部测试成功不替代这些条件；本次不表示合并、部署或团队制度采纳。

## 根因与最小复现

`TestExecVerificationGateRunnerPinsMakeToAbsoluteGoDespitePATH` 和 `TestExecVerificationGateRunnerBuildPassesWithRestrictiveUmask` 都用临时目录中的最小 Go 模块运行真实构建。样本没有自己的 Git 边界，Go 会向上发现宿主临时目录中的 `.git`。本机该目录不是可读取的 Git 仓库，因而在 VCS stamping 阶段报告 `error obtaining VCS status: exit status 128`。

最小化到一个无依赖 Go 主程序后，同样的受限环境下得到：

| 对照 | 结果 |
| --- | --- |
| 临时样本位于无效 Git 元数据的父目录下 | 构建失败，复现原错误 |
| 仅为 Git 设置目录查找上限 | 仍失败；Go 自己的 VCS 发现仍识别到父目录元数据 |
| 同样样本放入没有该元数据的临时根 | 构建通过 |
| 在原位置为样本初始化自己的空 Git 仓库 | 构建通过 |

这将问题定位到测试样本的宿主依赖，而非 Go 工具固定、make 环境过滤或 umask 权限断言。真实隔离树创建于项目 backend 下；本次不据测试样本故障扩大修改生产执行器。

## 修复和回归

在共享样本生成函数中，复用现有受限 `runGit`，通过空模板初始化样本自身仓库，阻止构建发现不相关父仓。新增 `TestExecVerificationGateRunnerBuildIgnoresInvalidAncestorGit`：在测试自行创建的无效 `.git` 目录下生成嵌套样本，再走真实构建门禁并验证三个输出制品。

新增回归先在未修复的样本生成函数上失败；修复后与原两项测试一起通过。原有绝对 Go 工具、恶意 PATH/make 环境不生效、严格 umask 下输出权限为 0755 的断言全部保留。没有删除宿主 Git 目录、跳过测试、设置 `-buildvcs=false` 或缩小 PR 验证范围。

`AGENTS.md` 当前入口更新为 `ming-iot-knowledge-pilot`，已从该工作树回读确认注册路由有效。旧 Goal 在历史资料中的身份保留，主分支入口随本候选合入后更新。

## 与前两轮试跑的关系

- [第一次真实 Patch 试跑](2026-10-13-ming-specs-pilot.md)和[需求调整与交接试跑](2026-10-14-ming-specs-handoff-pilot.md)中的失败均是当时候选的真实记录，继续保留。
- 本次解决它们共享的 B1 验证阻碍；前轮认领、Patch 指针、通知 R2、H1/H2 接收与兼容性修复的业务结论不变。
- 本次完整验证和复审只适用于回执绑定的新候选。真人交接、真实推送通道和长期维护成本仍未由本次工具修复验证。
