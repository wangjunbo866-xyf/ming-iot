# UnitedRhino Core 迁移基线

- Source: https://gitee.com/unitedrhino/core
- Commit: `1b3110e5e17d5a0066d38930b67bc44fdcc3c6fc`
- Module: `gitee.com/unitedrhino/core`
- Go directive: `1.24.4`
- Tracked files: `1188`
- Source worktree before migration: clean
- Baseline compile command: `GOWORK=off go test -run '^$' ./...`
- Baseline compile result: pass
- Business main packages: `service/apisvr`, `service/datasvr`, `service/syssvr`, `service/timed/timedjobsvr`, `service/timed/timedschedulersvr`

The source repository is read-only for this migration. Runtime dependencies remain external unless a later design explicitly brings them into the workspace.

## 实际执行记录

- 重跑时间：2026-07-23。
- `git -C ../core rev-parse HEAD` = `1b3110e5e17d5a0066d38930b67bc44fdcc3c6fc`，与固定 SHA 一致。
- `git -C ../core status --porcelain` 为空，工作树 clean。
- tracked files = 1188；tracked bytes ≈ 20.1 MB（plan 预期约 21.1 MB，实际略小）。
- `GOWORK=off go test -run '^$' ./...` exit 0，全部 package 编译通过。
- 五个业务 main 包入口文件分别为 `service/apisvr/api.go`、`service/datasvr/data.go`、`service/syssvr/sys.go`、`service/timed/timedjobsvr/timedjob.go`、`service/timed/timedschedulersvr/timedscheduler.go`（均为 `package main`，入口文件名不是 `main.go`）。
- 本机 Go 为 go1.25.4，`GOTOOLCHAIN=auto`；`go.mod` 声明 `go 1.24.4`，编译时自动拉取 go1.24.4 toolchain，编译结果与源仓一致。

## Foundation 阶段完成记录（计划 1 全部 Task）

执行计划：`docs/superpowers/plans/2026-07-23-core-migration-foundation.md`。

### Task 2：导入与净化

- `git archive` 导入 1161 个 tracked 文件到 `backend/core`（排除 `service/apisvr/.env`）。
- 15 个 YAML 配置的字面量凭据全部替换为 `${VAR}` 环境变量占位（DSN/AccessSecret/AccessKey/AccessKeyID/AccessKeySecret/AppSecret/Password/Map key/WeChat secret 等），复用上游 `share/utils.ConfMustLoad` 的环境变量展开。
- `service/syssvr/internal/config/config.go` 的 `ThirdJwtSecret` 默认值 `Xk9mQ2vR7pLw4nBz8jYf` 移除，改为 `optional,env=THIRD_JWT_SECRET`。
- 净化前后均跑了明文扫描门禁：净化前命中全部预期凭据，净化后零命中。
- 迁入后 `GOWORK=off go test -run '^$' ./...` exit 0，236 个 package 编译通过。

### Task 3：许可证

- README（中/英）原本声称 Apache-2.0，与 LICENSE 实际正文（GNU AGPL-3.0）矛盾；已修正为 AGPL-3.0 并标注上游 commit 来源。
- 新增 `backend/core/NOTICE`。
- LICENSE checksum `20b067f86de375aae6db0f283ab2e65de24d537733b89bd58432c101259d84cf` 未变。

### Task 4：Workspace 与忽略规则

- `backend/go.work` 只纳入 `./core`；workspace 解析出 `gitee.com/unitedrhino/core`。
- `.gitignore` 忽略 `/backend/bin/`、`/backend/.coretxn/`、`/backend/.goctl-patch-work/`、`**/.env`、`**/*.local.yaml`、`**/*.local.yml`；`backend/core/go.sum` 不被忽略。
- 补充跟踪 `backend/go.work.sum`（workspace 依赖校验，129 行，确定性可复现）。

### Task 5：构建入口

- 源仓根 Makefile 未导入（不在 archive allowlist 内），新建 `backend/core/Makefile`。
- `make build` 输出 5 个二进制到 `backend/bin`：`coresvr`(apisvr)、`syssvr`、`datasvr`、`timedjobsvr`、`timedschedulersvr`。
- Makefile 不依赖 `go mod tidy`，`clean` 只删 5 个具名文件，无 killall/nohup/rsync 副作用。
- 二进制由 `.gitignore` 忽略；`go.mod`/`go.sum` 构建前后无变化。

### Task 6：验收

- 源仓 SHA 不变、clean。
- `backend` workspace 与 `backend/core` module 两层均 `go list`/`go test -run '^$'` 通过；package 数 = 236。
- 5 个二进制全部构建且可执行。
- 无 `.env`、无被跟踪的 `backend/bin/**`、无字面量凭据。
- LICENSE/README/NOTICE 一致；目标工作树 clean。

### 备注

- Makefile 沿用 plan 指定的 `-tags no_k8s`；源码当前无任何 k8s build tag，该 tag 当前无实际效果但保留以与 plan 一致。
- plan Task 4 未提及 `go.work.sum`；本执行补充跟踪该文件以保证可复现构建。

