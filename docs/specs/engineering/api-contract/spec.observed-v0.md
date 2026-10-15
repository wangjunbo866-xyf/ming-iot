---
status: observed
version: observed-v0
source_revision: da80f056941e5f5e2ea1de38538ea2a062be8853
---

# HTTP API 契约校验：本次任务的最小现状基线

本文件记录已核实的现状，不是从代码自动推导出的业务批准。仅覆盖 `apicheck lint`，不声称已迁移整个 API 工具体系。来源为该版本的 `backend/tools/cmd/apicheck/main.go`、`main_test.go`、`internal/apicontract/metadata.go` 与实测。

<a id="lint-select"></a>
## LINT-SELECT：选择待检查文件

使用方是开发者及检查脚本。`lint --backend <根> --files <逗号分隔路径>` 选择相对于 SaaS/Core 根目录的 `.api` 文件；根参数支持 `backend` 或 `backend/saas`。当前实现对条目 TrimSpace、去空、去重，再用字符串匹配元数据问题的文件路径。

已观察到的缺陷：不存在的路径及全空白列表仍输出成功；带 `./` 的合法路径可能不匹配诊断。`lint: ok (N files)` 不能据此证明 N 个文件确实经过检查。

反向指针：[候选 PATCH-20261013-lint-selection](patches/PATCH-20261013-lint-selection/change.md) 替换本条的选择规则；覆盖关系与状态以 [模块入口](README.md#lint-selection-patches) 为准。不要把本条中的已知缺陷解释为需要保留的需求。

<a id="lint-meta"></a>
## LINT-META：元数据约束

继续使用现有七项元数据与 effective auth/needAuth/operType 一致性检查，选中文件有问题时失败。类型或 import-only `.api` 没有路由时，不虚构路由要求；本次不重写底层解析器、不改变认证与审计策略。

<a id="lint-readonly"></a>
## LINT-READONLY：只读与退出

lint 不写业务树或 golden。成功为退出 0；发现元数据问题退出 1；命令输入错误退出 2。精确退出码须运行编译后的命令判断，`go run` 的外层退出码不能替代子程序退出码。
