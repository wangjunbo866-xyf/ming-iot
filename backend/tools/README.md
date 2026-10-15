# backend/tools — 项目工具与生成器

本目录承载 ming-iot 项目专用的代码生成与契约校验工具，以及固定版本 goctl fork
的可复现构建脚手架。

## 统一 Verification Profile

仓库根 `Makefile` 是本地开发与 Gitee CI 的统一验证入口。它只组合现有 Go、部署和本目录
工具，不复制 `kafkacontract`、`thingsstartbaseline`、`apicheck` 或 `rpcgen` 的实现：

```bash
make verify-fast VERIFY_SCOPE=share    # 或 saas、things、tools
make verify-pr                         # 不启动外部服务
make verify-integration VERIFY_SUITE=kafka
make verify-integration VERIFY_SUITE=things-startup
MING_HEAVY_VERIFY=1 make verify-release
```

tools 测试和静态合同需要锁定的 protobuf 工具链。根入口通过
`backend/tools/scripts/install-protobuf-tools.sh install` 完成首次校验安装，并在
`backend/bin/protobuf-tools/` 保存与 lock、三个二进制共同绑定的本地缓存摘要；后续命中时
不重复下载。该目录由 Git 忽略，缓存失配时重新进入原安装器的 checksum 校验，不能形成 PASS。

`verify-integration` 和 `verify-release` 是显式重型入口，不属于普通 PR 门禁；它们只操作
获选的本地测试 profile，不执行生产发布、数据库 reset 或共享卷删除。

## Kafka 基础契约门禁

`kafkacontract` 默认只校验仓库内的 Topic Catalog、local/production 配置、Kafka
镜像锁和 Go/franz-go 版本，不访问 Docker 或网络：

```bash
cd backend
go run ./tools/cmd/kafkacontract --repo-root ..
```

显式 `--integration` 才会连接 `127.0.0.1:39092`，创建固定探针 Topic，写入并读回
固定 key/header/value，输出 topic、partition、offset 和三类 SHA-256，然后删除探针
Topic。broker 不可达或内容不一致时返回非零状态，不能降级成跳过。

```bash
make -C deploy kafka-up
make -C deploy kafka-ready
cd backend
go run ./tools/cmd/kafkacontract --repo-root .. --integration
make -C deploy kafka-down
```

最终 Evidence 必须写到 LoopX 私有目录或其他明确的本地输出目录，不进入产品提交。
`--verification` 可重复传入 `名称|pass-or-fail|仓库相对命令`，Evidence 只保存结果和
摘要，不保存原始日志、payload、凭据或本机绝对路径：

```bash
go run ./tools/cmd/kafkacontract \
  --repo-root .. --integration \
  --goal-id ming-iot-framework-v1 \
  --todo-id todo_966a84160f43 \
  --evidence-output /path/to/private/evidence.json \
  --verification 'focused-tests|pass|go test ./share/contracts/kafka ./share/foundation/kafka ./tools/internal/kafkacontract ./tools/cmd/kafkacontract -count=1'
```

Evidence 会同时记录 Git HEAD、`dirty` 和包含未跟踪候选文件的 worktree digest；未
commit 的候选不会被伪装成 clean。完整 payload 和凭据只参与运行时校验，不进入 JSON。

## Things 启动基线工具

`thingsstartbaseline` 提供七个子命令，并由 `deploy/Makefile` 组合成统一入口：

| 子命令 | 职责 |
| --- | --- |
| `contract` | 校验启动合同与独立基线数据库保护边界 |
| `manifest` | 校验五个静态 ELF，并生成二进制哈希清单 |
| `stage` | 把二进制和运行配置复制到隔离 staging 目录 |
| `image` | 按固定 Dockerfile 与标签构建五个服务镜像 |
| `up` | 通过十阶段状态机完成数据库 bootstrap、依赖和服务有序启动 |
| `verify` | 不构建、不启动，只用协议探针复核已有服务 |
| `logs` | 只读导出 Compose 的最近 200 行脱敏日志 |

`up/verify` 统一产生 Evidence：包括十阶段结果、二进制与镜像身份、Docker/Compose
版本和展开后 Compose 配置的完整 SHA-256；展开配置明文本身不会落盘。`logs` 只输出
既有命令运行器生成的 4 KiB 有界脱敏日志摘要，不修改 Evidence。

日常操作使用：

```bash
make -C deploy things-baseline-fast
make -C deploy things-baseline-up
make -C deploy things-baseline-verify
make -C deploy things-baseline-logs
make -C deploy things-baseline-down
```

工具没有 `reset` 子命令，Makefile 也没有 reset 目标；删除数据库或数据卷必须另行审批。

## L0 Things 可编译迁移基线

`thingsbaseline` 只从 lock 固定的 Git commit/tree/blob 读取联犀 Things；官方
`things/` 仓库始终只读。首次导入要求 `backend/things` 不存在：

```bash
cd backend/tools
go run ./cmd/thingsbaseline import \
  --lock ./things-baseline.lock \
  --source /home/wangjunbo/workspace-iot-platform/things \
  --backend ..
```

导入后用两个独立 candidate 重渲染，共享宿主 Go 模块/编译缓存，并在隔离树而非 target 上运行测试和构建：

```bash
cd backend/tools
go run ./cmd/thingsbaseline verify \
  --lock ./things-baseline.lock \
  --source /home/wangjunbo/workspace-iot-platform/things \
  --backend .. \
  --evidence ../../docs/wiki/legacy/superpowers/evidence/2026-08-14-l0-things-migration-baseline.json
```

需要切换 Go 模块镜像时，使用 Ming 工具链公共输入；它不会替代系统传输代理：

```bash
MING_GOPROXY=https://proxy.golang.org,direct \
  go run ./cmd/thingsbaseline verify \
  --lock ./things-baseline.lock \
  --source /home/wangjunbo/workspace-iot-platform/things \
  --backend .. \
  --evidence /tmp/l0-things-evidence.json
```

`HTTP_PROXY`、`HTTPS_PROXY` 及其小写形式仍会传递给受控的 Go 子进程；环境中已有的全局
`GOPROXY` 会保持隔离并被忽略。`MING_GOPROXY` 仅配置仓库工具，不配置业务服务运行时。
构建使用私有的 `.thingsbaseline-gates-*` 目录；`thingsbaseline` 构建校验不会读取、覆盖或删除
仓库现有的 `backend/bin`，也不应提交生成的二进制文件。

`thingsbaseline` 通过受信 Go 工具的 `go env GOCACHE GOMODCACHE` 复用宿主缓存；candidate、
`HOME`、`GOTMPDIR`、临时 workspace 和构建输出仍逐轮隔离。四个 `go test` Gate 都显式使用
`-count=1`，因此只复用下载和编译产物，不复用测试结果。缓存损坏时工具会 fail-closed，但不会自动
执行 `go clean -cache` 或 `go clean -modcache`；清理共享缓存必须由操作者明确执行。

`--structural-only` 仅用于首次导入后的快速诊断，evidence 会明确写成
`finalEvidence=false`、`overallResult=NOT_FINAL`，不能作为 L0 完成证据。候选和隔离目录由工具清理；
外部强制终止仍可能留下带 `.thingsbaseline-*` 前缀的私有临时目录，但不会再为每轮复制独立
`GOCACHE/GOMODCACHE`。不要人工编辑生成的 `backend/things`，修改迁移规则应从 lock、转换器及其
测试开始。

## 业务 workspace 禁止直接执行系统 goctl

业务工作区（`backend/saas/...`、`backend/cmd/...`）**禁止**直接调用 PATH 里的
任意系统 `goctl`。原因：

- 系统 goctl 版本不可控，模板与 validator 行为会随上游变化；
- 联犀 fork 的 `supper` 鉴权契约、七项 metadata、WebSocket 路由等约定必须由
  本项目固定补丁保证；
- 业务 API/RPC 生成必须走项目 wrapper（`apicheck generate`、`rpcgen generate`），
  它们在临时 Core 副本里生成、在 staging 校验、再以原子交换提交，失败绝不触碰
  业务树。

直接调用系统 goctl 生成出来的代码不会被 CI 接受。

## runtime go-zero 与 goctl fork 是两个独立的版本轴

- **运行时** `gitee.com/unitedrhino/go-zero` 锁定在 `go.mod` 里的版本（当前
  v1.8.x 跟随 core）。运行时只影响服务进程的 HTTP/gRPC/中间件行为。
- **goctl fork**（本目录固定）是代码生成器，跟运行时无版本绑定。fork 的显示
  版本字符串 `1.9.2-1` 不是可信身份，真实身份由 `goctl.lock` 里的
  `repository + commit + patches + checksum` 唯一确定。

升级运行时 go-zero 与升级 goctl fork 是两件互不相关的事，必须分别 review、
分别提交。

## `action` 与 `operType` 是两个不同的语义

- `action`（`view`/`add`/`modify`/`delete`/`find`/...）服务于 **语义描述与 API
  文档**（swagger 扩展字段、apicheck golden）。它在 `.api` 源文件里声明，跟随
  路由本身。
- `operType` 服务于 **旧审计枚举**（access 生成器读取，用于权限/审计树的
  `add/modify/delete/find/other` 五值分类）。它由 access 生成器从旧 `operType`
  字段或 URL 推导，**不**直接从 `action` 复制。

本项目**不**把 `action` 塞进 `operType`。apicheck 的 access golden 独立比较
method/fullPath/authType/needAuth/operType。

## API/RPC 生成必须走项目 wrapper

| 任务 | 命令 |
| --- | --- |
| Things RPC 生成 | `go run ./cmd/rpcgen generate --manifest ... --module-root ../things --protobuf-tools ../bin/protobuf-tools` |
| Things RPC 清单校验 | `go run ./cmd/rpcgen check --manifest ... --module-root ../things` |
| SaaS RPC 兼容生成 | `go run ./cmd/rpcgen generate --manifest ... --saas ../saas --goctl ../bin/goctl --protobuf-tools ../bin/protobuf-tools` |
| SaaS RPC 兼容校验 | `go run ./cmd/rpcgen check --manifest ... --saas ../saas` |
| API 生成 | `go run ./cmd/apicheck generate --backend .. --goctl ../bin/goctl` |
| API 契约校验 | `go run ./cmd/apicheck verify --backend ..` |

wrapper 负责：临时副本、staging 校验、allowlist 同步、原子交换、失败回滚。
直接 `goctl api go` / `goctl rpc protoc` 到业务目录不被接受。

`rpcgen generate` 不从 `PATH` 查找 goctl。它固定读取 `backend/tools/goctl.lock`，
校验并执行 `backend/bin/goctl`；manifest v2 禁止 `--goctl` 覆盖，SaaS v1 的
兼容参数即使显式提供，也必须规范化为同一个锁定路径。

Things manifest v2 在事务开始前从 live tree 冻结 descriptor、client API、
direct adapter、runtime 注册和 protected tree 摘要。PB/client/server 只在 candidate
中生成，alias 以 live client API 为准补齐；candidate 必须依次通过旧 Share import、
兼容面、protected tree、编译和 allowlist 检查后才允许原子交换。SaaS manifest v1
继续以现有 `clientContractGolden` 为权威，并在生成前与 live client API 交叉校验。

## 固定 goctl 构建

```bash
# 首次或升级 fork 后：打印实际 checksum 供 review
bash backend/tools/scripts/build-goctl.sh bootstrap

# 普通构建：校验 goctl.lock 中所有 checksum，构建到 backend/bin/goctl
bash backend/tools/scripts/build-goctl.sh build

# 测试脚本自身的契约（离线，fake git/go/sha256sum）
bash backend/tools/scripts/build-goctl_test.sh
```

`backend/bin/goctl` 由根 `.gitignore` 忽略；`backend/.goctl-patch-work/` 也被
忽略（补丁开发的临时 fork clone 目录）。

### checksum 更新必须单独 review

`goctl.lock` 中的 `sha256`、`sourceArchiveSha256`、`binarySha256` 任何变更
都必须是**独立提交**，且仅由以下两种情况触发：

1. fork 上游 commit 改变（升级）；
2. 任一 patch 内容改变。

普通 `build` 不会自动改写 lock；它只校验。`bootstrap` 打印实际值，维护者把它
写回 lock 并单独提交，附上两次 bootstrap 的 `diff` 证据。
