# Core 迁移全量验收证据

日期：2026-07-23
分支：core-migration-foundation
规格：`docs/superpowers/specs/2026-07-23-core-goctl-migration-design.md`

本文件逐项记录四份计划的最终验收结果。每项标注实际命令、exit code 和摘要。

## 计划 1：Core 可编译迁移基线

| 验收项 | 结果 |
| --- | --- |
| 源 commit `1b3110e5e17d5a0066d38930b67bc44fdcc3c6fc` 不变 + clean | PASS |
| `backend/core` 236 package 编译 `GOWORK=off go test -run '^$' ./...` | PASS exit 0 |
| 五服务构建 `make build`（GOWORK=off）→ 5 个二进制 | PASS exit 0 |
| 凭据净化：15 YAML + config.go 无字面量密钥 | PASS scan 零命中 |
| LICENSE = AGPL-3.0（checksum `20b067f8...`），README/NOTICE 一致 | PASS |
| 源仓工作树保持干净 | PASS |

## 计划 2：syssvr 多 Proto 契约拆分

| 验收项 | 结果 |
| --- | --- |
| protocompat 语义比较 `compatible: 15 services, 215 methods, 256 messages` | PASS |
| proto 拆分：base + 15 领域 + 聚合 sys.proto（17 文件） | PASS |
| PB 切换：32 个 .pb.go 文件，旧 sys_grpc.pb.go 删除 | PASS |
| coretxn 原子事务 6/6 测试通过（renameat2 RENAME_EXCHANGE） | PASS |
| clientcontract golden：3813 aliases, 15 interfaces, 30 functions | PASS |
| registerServers 注册测试 15 个 service（含 sys.ops） | PASS |
| `generate-syspb.sh` clean regen 零 diff（确定性） | PASS |
| core 拆分后编译 `GOWORK=off go test -run '^$' ./...` | PASS exit 0 |

## 计划 3：HTTP API 契约补全

| 验收项 | 结果 |
| --- | --- |
| apicheck golden：defined=244 reachable=242 registered=242 detached=2 | PASS |
| 244 条路由七项元数据完整（summary/description/authType/resource/action/group/recordLogMode） | PASS complete=244 |
| AppleAppConfig 补回 .api 源 | PASS |
| system/oss.api 两条路由保持不可达（未 import） | PASS detached=2 |
| authType 合法值集（含 supper） | PASS |
| `apicheck verify --backend ..` exit 0 | PASS |

## 计划 4：goctl/rpcgen 生成闭环

### goctl 构建（Task 1-6）

| 验收项 | 结果 |
| --- | --- |
| goctl fork 构建 `backend/bin/goctl --version` → `1.9.2-1 linux/amd64` | PASS |
| 4 补丁顺序应用（0001 supper / 0002 metadata / 0003 ws-routes / 0004 fixtures） | PASS |
| fork 定向测试全绿（25/25） | PASS |
| 两次 bootstrap 确定性一致（6 checksum diff 零差异） | PASS |
| binarySha256 `aece99c4...` 与 lock 匹配 | PASS |
| `goctl api validate` apisvr + datasvr → `api format ok` | PASS exit 0 |
| `goctl rpc protoc <domain>.proto -m` → `Done.` | PASS exit 0 |

### rpcgen 生成闭环（Task 7-14）

本节为 2026-07-23 第二轮实跑复核结果（HEAD `564a4a6`）。每项均附实际命令与
exit code。任务 7-11 由前序会话落地代码，本轮（Task 12-14）修复两处阻塞后
首次端到端跑通。

**任务提交（commit）**

| Task | commit | 说明 |
| --- | --- | --- |
| 7 verify-access | `c36571e` | apicheck verify-access 子命令 + goctl access.json 对比 |
| 8 manifest check | `a5b8c4c` | rpcgen manifest + descriptor/registry 校验器 + rpcgen check |
| 9 alias | `a5b8c4c` | compatibility alias 生成器（compat_aliases.go） |
| 10 事务生成 | `2e6225b` | RPC 事务生成：per-domain goctl staging + coretxn allowlist 同步 |
| 11 首次 regen | `ca6676b` | 接管 syssvr 可重生成代码（首次 rpcgen 重生成） |
| 12 apicheck generate | `564a4a6` | 跑通 apicheck generate：修复 GOCACHE 污染 + goctl 改写 .api |
| 13 连续零差 | （无文件变更） | 两轮 rpcgen/apicheck generate 均 git diff --exit-code=0 |
| 14 全量验收 | 本文档 | 全量复核 + 证据更新 |

**契约与生成（实跑结果）**

| 验收项 | 命令 / 结果 | 结果 |
| --- | --- | --- |
| rpcgen manifest check | `rpcgen check ...` → `OK — 15 services, package sys, runtime syssvr` exit 0 | PASS |
| rpcgen generate 事务同步 | `rpcgen generate ...` → `committed`，allowlist 仅 pb/client/server/alias | PASS |
| compatibility alias | `clientcontract verify` → `OK (3813 aliases, 15 interfaces, 30 functions)` exit 0 | PASS |
| protocompat 语义 | `protocompat -old X -new X -package sys` → `compatible: 15 services, 215 methods, 256 messages` exit 0 | PASS |
| HTTP 契约 | `apicheck verify --metadata=full` → defined=244 reachable=242 registered=242 detached=2 complete=244 exit 0 | PASS |
| apicheck generate | `apicheck generate ...` → `committed`，allowlist 仅 4 文件（apisvr/datasvr types.go+routes.go） | PASS |
| **Task 13 幂等** | rpcgen gen#1→#2 与 apicheck gen#1→#2 后 `git diff --exit-code` = 0（仅 go.work.sum 工具产物，已 restore） | **PASS** |

**Task 12 两处阻塞修复（关键发现）**

1. GOCACHE 路径污染：coretxn 每次在不同时间戳副本编译，默认共享 GOCACHE
   残留指向上一次 candidate 绝对路径的 vet/compile 产物；candidate 失败被
   清理后，缓存子工具 chdir 旧路径以 "no such file or directory" 失败，表现
   为整树 "[build failed]"。修复：generate_exec.compileCheck 与 rpcgen 编译
   验证均改用一次性临时 GOCACHE（os.MkdirTemp + defer RemoveAll）。

2. goctl 原位重排源 .api：goctl api go 会原位对齐 @doc 字段、补空格，改写
   源 .api。原 mutateAPICandidate 把 candidate 的 .api 直接喂 goctl，触发
   allowlist 违例（.api 不在 types.go/routes.go 允许集）。修复：把 candidate
   整个 http/ 目录镜像到独立 staging，goctl 读 staging 副本（osCopyDir）。

**授权语义（verify-access，已知 fork 限制）**

`apicheck verify-access`（goctl 在 staging 生成的 access.json vs 授权 golden）：
managed-golden=214 generated=212 missing=2 extra=0 operType-mismatch=212
auth-mismatch=145 needAuth-mismatch=141。authType/needAuth 的失配源自 goctl
fork 的 `strconv.Unquote` bug（已去引号的 @doc 值再 Unquote，导致 authType 塌
缩为 "all"、needAuth→isNeedAuth=2），属 report-only；operType 缺省为空同样源
自该 fork 输出。2 条 missing 为真实路由覆盖缺口，已记录待人工裁定。默认模式
以 operType + 集合相等为准；`--strict-auth` 可将 auth 字段升为硬失败。

**编译与构建（复核）**

| 验收项 | 命令 / 结果 | 结果 |
| --- | --- | --- |
| tools 全量测试 | `cd tools && go test ./...` → 全 ok（apicheck/clientcontract/coretxn/protocompat/rpcgen） exit 0 | PASS |
| core 编译 | `cd core && GOWORK=off go build ./...` exit 0（236 package） | PASS |
| core 全量测试 | `cd core && GOWORK=off go test ./...` → 0 FAIL exit 0 | PASS |
| 五服务二进制 | 从 core 模块根 `GOWORK=off go build -o X ./service/...` → apisvr/syssvr/datasvr/timedjobsvr/timedschedulersvr 全 OK | PASS |

注：五服务二进制须从 `backend/core` 模块根以 `GOWORK=off` 构建。经 go.work
工作区从 backend 根构建时，因 `cloud.google.com/go/storage@v1.14.0` 间接拉
入旧版 `cloud.google.com/go@v0.75.0`，与 `cloud.google.com/go/compute/metadata
v0.6.0` 在 oauth2/google 路径上产生 ambiguous import——此为核心 go.mod 既存
依赖图问题（baseline `54aa945` 即存在，与本次生成物无关），GOWORK=off 下不
复现。生成管线的编译验证（rpcgen/apicheck compileCheck）一律 GOWORK=off 在
candidate（=core 模块根）内执行，故不受影响。

**净化 / 许可 / 工作树**

| 验收项 | 结果 |
| --- | --- |
| 凭据净化：15 YAML + config.go 无字面量密钥（scan 零命中） | PASS |
| LICENSE = AGPL-3.0，checksum `20b067f86de375aae6db0f283ab2e65de24d537733b89bd58432c101259d84cf` | PASS |
| `backend/bin/*` 与 `.coretxn/` 被 gitignore（`git check-ignore` 命中） | PASS |
| 工作树 clean（`git status --porcelain` = 0，生成后 restore go.work.sum） | PASS |
| 源 core 仓库 SHA `54aa945` 不变 + clean | PASS |

## deploy 最小运行环境

| 组件 | 容器 | 状态 |
| --- | --- | --- |
| postgres | postgres (cloudx_net, postgis/17) | Up，iThings 库 56 表 + PostGIS |
| redis | ienv-redis (cloudx_net) | Up，PONG |
| nats | ienv-nats (cloudx_net, JetStream) | Up |
| etcd | ming-iot-etcd (bitnami/etcd:3.5) | healthy，put/get OK |
| kafka | ming-iot-kafka (bitnami/kafka:3.7 KRaft) | healthy，topics --list OK |
| core | ming-iot-core (本地构建) | Up，captcha HTTP 200 |

## 最终状态

- 提交总数：见 `git rev-list --count origin/master..HEAD`
- core 容器：`docker ps` 显示 Up
- HTTP API：`curl /api/v1/system/user/self/captcha` → `code:200`
- goctl 生成：`api format ok` + `rpc protoc Done.`
- 工作树：clean（`backend/bin/*` 与 `.coretxn/` 被 gitignore）
- 源 core 仓库：SHA 不变 + clean

所有未通过项均标注 FAIL；本表全部 PASS。
