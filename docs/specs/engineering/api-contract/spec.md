---
status: effective
version: current-v1
source_revision: c165a56531f58421a870a0b8794f24556b60d3b2
---

# HTTP API 契约校验：当前主规格

本规格仅覆盖开发者使用的 `apicheck lint`，不是整个 API 工具链或业务 API 的规格。适用代码为上方新仓 main 基线；选择层修复沿用 [Patch 的任务级授权与 AC1–AC5](patches/PATCH-20261013-lint-selection/change.md)，保留元数据语义及只读约束。本轮不改实现，不新增业务承诺。修复前的 [observed-v0 原文](spec.observed-v0.md) 只作历史。

`effective` 限该代码版本及以下能力；文档候选、验证和最终评审见 [S1 记录](../../../changelog/2026-10-15-specs-current-baseline.md)，不表示其他工具命令已验证或服务已部署。

<a id="lint-select"></a>
## LINT-SELECT：选择待检查文件

`lint --backend <根> --files <逗号分隔路径>` 使用相对于 SaaS 根目录的路径；根参数支持新仓 `backend` 或 `backend/saas`。

1. 列举当前服务 HTTP 扫描目录中实际存在的普通 `.api` 文件。每个非空输入条目必须属于该集合且可读；不存在、目录、非 `.api`、绝对路径或扫描范围外路径均是输入错误。
2. 各项先 TrimSpace，去除空项，按本机文件路径规则规范化 `./` 和 `..`，再按规范路径去重。仅当规范后仍属于扫描集合才接受；`..` 不构成越出扫描范围的许可。
3. 全空白/逗号列表失败；有效和无效项混合时整体失败，不能因含一个有效项而输出成功。扫描或读取失败也不输出 `lint: ok`。
4. 诊断选择与成功计数使用同一规范路径集，带 `./` 的问题文件仍被检查，重复项只计一次。

<a id="lint-meta"></a>
## LINT-META：元数据约束

继续使用现有七项元数据与 effective auth/needAuth/operType 一致性检查。选中的问题文件失败；没有路由的纯类型或 import-only `.api` 不虚构路由要求。修复只发生在选择层，不重写底层解析器、认证或审计策略，不改变生成器与 golden。

<a id="lint-readonly"></a>
## LINT-READONLY：只读与退出

lint 不改写输入、业务树或 golden。正常成功退出 0，发现元数据问题退出 1，命令输入/文件选择错误退出 2。精确退出码以编译后的 CLI 为准；`go run` 外层退出码不能替代子程序退出码。失败时不能把成功文案当作已扫描的证明。

## 实现、验收与验证入口

- [CLI 实现](../../../../backend/tools/cmd/apicheck/main.go)：`cmdLint`、`lintFileSelection`。
- [CLI 回归](../../../../backend/tools/cmd/apicheck/main_test.go)：编译后运行命令；不是仅测试选择 helper。
- [底层元数据检查](../../../../backend/tools/internal/apicontract/metadata.go)：既有约束来源；本轮不重新定义其全部语义。

从新仓根运行，不需要旧仓 commit：

```bash
go -C backend/tools test ./cmd/apicheck -count=1
```

| 条款 | 现有验证 |
| --- | --- |
| LINT-SELECT | `TestCLILintFileSelection`：缺失/空白/混合/目录/越界/绝对路径、规范化去重；`TestCLILintCleanFile`：真实树合法文件 |
| LINT-META | `TestCLILintNormalizedSelectionStillChecksMetadata`：规范化后仍识别问题，纯类型可通过 |
| LINT-READONLY | 上述编译 CLI 用例核对退出码、输入及 golden 字节；仅覆盖用例范围 |

不要用 `-short` 声称完成真实树用例。该包的测试不是整个平台 API、数据库或运行中服务的验收。长期模块负责人未分配；本次责任见 [模块入口](README.md)。

## 变更来源

[PATCH-20261013-lint-selection](patches/PATCH-20261013-lint-selection/change.md) 已吸收到 current-v1 的 LINT-SELECT；LINT-META、LINT-READONLY 延续原约束。[反向入口](README.md#lint-selection-patches) 保留历史，读者无需叠加旧 Patch 才能理解当前规则。
