# Ming IoT 本地 Share 模块迁移 Evidence

## 结论

- 当前候选分支：`local-share-migration`，基线提交：`61c9c1a928f23c97332ebf93ac84801591d170b9`。
- 冻结的 `share@v1.6.4` 已导入 `backend/share`，模块路径切换为 `gitee.com/ming-iot/share`。
- 1548 个活跃引用文件已完成机械替换；活跃源码中的旧路径为 0，只保留 5 份历史文档引用。
- Tools、SaaS、Things 的普通测试与 Race 验证未发现迁移新增失败；Things 冷启动、warm restart 与协议探针均通过。
- 本 Evidence 证明候选实现进入最终独立 Review 阶段，不代表已经获得 `git add`、commit、merge 或 push 授权。

## 执行环境

- Go：`go1.25.4 linux/amd64`；仓库 Go 指令版本为 `1.24.4`。
- Docker：client/server `29.4.0`。
- Docker Compose：`v5.1.3`。
- 运行位置：当前 WSL2；PostgreSQL 复用既有实例并使用 Things 独立数据库。
- 本地 `.env.local` mode 为 `600`，已被 Git 忽略；本文件不记录变量值。

## 来源与许可证

| 项目 | Evidence |
|---|---|
| 来源 Tag | `v1.6.4` |
| 来源 commit | `ece02ca9d6defd63356931a59e653950f27ac572` |
| 来源 tree | `21bdb52ed508152cf440b8d8a3b3ae3943a4b84a` |
| 来源跟踪文件数 | 255 |
| `git archive --format=tar` SHA-256 | `8c90b2b3853ad86b422f9929d801d617645bac3dbf68f600e3736b3a32505acf` |
| 改名后规范化来源树 SHA-256 | `85e564cd406cb90738bfbf04f7789471ee61854a98e837bb604cb09243f31867` |
| `SOURCE.json` SHA-256 | `668cabd7e43e208fd16b10912fc5000c61d6e2469483d64f90e7249d0860d086` |
| `LICENSE` | Apache-2.0 标准全文 |
| `LICENSE` SHA-256 | `cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30` |
| 权利路线 | 权利人或公司有权责任人授权；公开源码只记录结论，不记录原始对话或内部身份信息 |

四个上游第三方版权文件的 Apache-2.0 声明保持不变。`oss/AWS_STS.md` 中只保留与上游逐字一致的公开标识和 Secret 占位符；未发现 SecretAccessKey 或私钥，Evidence 不复制具体标识值。

## 候选树边界

- 已跟踪修改文件：1552。
  - 1548 个冻结 allowlist 文件：只替换旧 Share import/module path；
  - `backend/go.work`：加入 `./share` 和精确的本地版本 replace；
  - `backend/saas/Makefile`、`backend/things/Makefile`：仓库级构建不再关闭 workspace；
  - `backend/tools/internal/thingsstartbaseline/artifact_test.go`：锁定本地 Share 构建契约。
- 新导入 `backend/share` 文件：257，即 255 个冻结来源文件加 `LICENSE`、`SOURCE.json`。
- 新仓库契约：`backend/tools/internal/repositorycontract/local_share_migration_test.go`。
- 与批准路径集合比较：无额外已跟踪修改路径。
- `backend/tools/go.mod`、`backend/tools/go.sum` 与基线提交一致；`backend/go.work.sum` 未发生变更。
- 两个被机械替换的旧文件原本使用 CRLF；使用 `git -c core.whitespace=cr-at-eol diff --check` 验证，避免仅因既有 CRLF 误报尾随空白，同时保持原文件换行风格。

`backend/share/.gitignore` 继承上游并忽略 `go.sum`。最终获得暂存授权后必须单独执行 `git add -f backend/share/go.sum`，再以 `git ls-files --error-unmatch` 和 cached diff 双重验证；在此之前不得提前暂存。

## Module Graph 与仓库契约

| 命令/断言 | 结果 |
|---|---|
| `go test ./internal/repositorycontract -run LocalShare -count=1` | PASS |
| `go list -m -json gitee.com/ming-iot/share` | PASS，本地 workspace 主模块目录为 `backend/share` |
| `go list -m all` 中旧 Share module | 0 |
| 活跃源码中的旧 Share 路径 | 0 |
| 历史文档中的旧 Share 路径 | 5，全部在批准 allowlist |
| 规范化来源树摘要合同 | PASS，255 文件摘要与 `SOURCE.json` 一致 |

验证期间发现一次 `go work sync` 曾隐式升级新 Share 与 Tools 依赖。候选树随后恢复到冻结 `v1.6.4` 的 `go.mod/go.sum`，仅保留 module path 改名，并增加规范化来源树摘要合同，防止同类漂移再次漏检。

## 分层 Go 验证

| 模块 | 普通测试 | Race | Vet | 判定 |
|---|---|---|---|---|
| Tools | PASS，23 秒 | PASS，26 秒 | PASS，2 秒 | PASS |
| SaaS | PASS，72 秒 | PASS，191 秒 | FAIL，17 秒 | Vet 的 5 条结果与迁移前 master 逐项相同 |
| Things | PASS，257.41 秒 | PASS，161 秒 | FAIL，4.39 秒 | Vet 结果集合与迁移前 master 完全相同，仅输出顺序不同 |
| Share | FAIL，14.10 秒 | FAIL，78.40 秒，无 Data Race 报告 | FAIL，0.33 秒 | 失败签名与冻结上游对照一致，未出现迁移新增类别 |

Share 的既有失败包括：两个 `golang.org/x/image` checksum 缺项、NATS/MQTT 集成环境失败，以及若干已有测试断言失败。Race 命令整体失败，因此不能宣称 Share Race 全面通过；只能证明没有观察到新增 Data Race 报告，且失败签名与上游一致。

## Things 可重复启动

### 快速静态基线

`make -C deploy things-baseline-fast`：PASS。

覆盖五个静态 ELF 二进制构建、构建 manifest、聚焦测试、Compose 配置、数据库破坏性命令扫描和凭据扫描。

### 冷启动

- Verify run：`20260827T160551.269316029Z-61c9c1a928f2`。
- `overallResult`：PASS。
- Contract SHA-256：`acf209ab9f454dfb21e98d0f2c4f5ec0a22f6949d7d17cb1af695a82055037bf`。
- Evidence 指针：`deploy/.runtime/runs/20260827T160551.269316029Z-61c9c1a928f2/evidence.json`。

### Warm restart

- 执行不带 `-v` 的 Compose down，保留数据库和 volume，再次 up/verify。
- Verify run：`20260827T160746.374720227Z-61c9c1a928f2`。
- `overallResult`：PASS。
- 10 个阶段全部 PASS，失败阶段数为 0。
- Contract SHA-256 与冷启动一致。
- Evidence 指针：`deploy/.runtime/runs/20260827T160746.374720227Z-61c9c1a928f2/evidence.json`。

验证结束时，SaaS/Things 五个应用角色与 Redis、etcd、NATS、TDengine、EMQX 均为 running；健康检查型基础设施为 healthy。PostgreSQL 探针在 Evidence 的数据库阶段通过。

## 未绑定 Claude 辅助审查

2026-08-28，一个无会话持久化、禁用编辑工具的临时 Claude Code 进程只读检查了最终候选树，输出为 `APPROVED`：Critical 0、Important 0、Minor 3。该进程没有绑定 LoopX 注册 Agent `ming-review-claude`，也没有形成可由对应 Claude Code 会话领取和回写的 Reviewer Todo，因此本结果仅是辅助技术意见，不能作为正式独立 Review Evidence，也不能触发最终用户 Gate。

该辅助进程检查了冻结来源、255 文件规范化树摘要、Apache-2.0 全文、四个第三方文件头、1548 机械替换 allowlist、四个特许路径、零活跃旧模块引用、本地 Workspace module graph、未升级依赖、冷/热启动 Evidence、敏感信息边界及 `backend/share/go.sum` 强制暂存流程。辅助记录见 `reviews/2026-08-28-ming-iot-local-share-migration-review.md`。

辅助审查提出三个 Minor，正式 Reviewer 仍需独立判断其级别与处置：

1. SaaS/Things `go.sum` 中保留机械替换后的 `gitee.com/ming-iot/share v1.6.4` 未使用校验行；当前 `go.mod` 与 Workspace 使用 `v0.0.0` 本地替换，后续如需要脱离 Workspace 构建，应另立清理任务；
2. `things-baseline.lock` 的 `shareVersion: v1.6.4` 表示来源冻结版本，`go.mod` 的 `v0.0.0` 表示本地 Workspace 模块版本，语义自洽但需在后续文档中避免混淆；
3. 上游 GitHub OAuth2 测试夹具含类似 AppSecret 的固定测试字符串；经模块路径规范化后与上游逐字一致，不是本迁移新增。

## 剩余动作

1. 通过 LoopX 建立显式 `action_kind=review` 的独立 Reviewer Todo，由注册 Agent `ming-review-claude` 在其 Claude Code 会话中领取、审查并回写正式结论；
2. 正式 Review 与最终候选验证都完成后，再建立本迁移唯一一次用户验收 Gate；
3. 用户明确批准暂存/提交时，强制暂存 `backend/share/go.sum` 并验证索引；merge 与 push 仍分别等待授权。
