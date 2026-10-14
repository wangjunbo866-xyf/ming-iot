# Core 迁移实施路线图

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变现有 gRPC、direct client、HTTP 运行时契约和 `supper` 权限语义的前提下，把联犀 Core 迁入 `ming-iot/backend/core`，拆分 `syssvr` proto，并建立受控的 goctl/rpcgen 生成闭环。

**Architecture:** 迁移按四个顺序阶段执行，每一阶段形成可编译、可提交、可回退的稳定点。先冻结并迁移源码，再拆 RPC 契约，再补 HTTP 契约，最后才允许新生成器接管生成物。运行时继续是单一 `syssvr` RPC 进程；多个 proto 只是契约文件边界，不是服务拆分。

**Tech Stack:** Go 1.24.4 module、Go Workspace、go-zero v1.8.4、Protocol Buffers、gRPC、联犀 goctl fork `6579a20bfd96919a80af5a9cb4e12f03598d4bb3`、Git、Make。

---

## 0. 固定输入与不可变约束

- 源仓库：`/home/wangjunbo/workspace-iot-platform/core`。
- 源提交：`1b3110e5e17d5a0066d38930b67bc44fdcc3c6fc`。
- 目标仓库：`/home/wangjunbo/workspace-iot-platform/ming-iot`。
- 规格文档：`docs/superpowers/specs/2026-07-23-core-goctl-migration-design.md`。
- 源仓库在全过程只读，开始和结束都必须是 clean worktree。
- Go module path 保持 `gitee.com/unitedrhino/core`。
- `backend/core/tools` 是随上游 Core 保留的两个业务辅助程序；`backend/tools` 是 ming-iot 自己的生成/契约工具 module，二者边界不同。
- 运行时 go-zero 保持 v1.8.4；生成器版本独立锁定。
- gRPC 保持 15 个 service、215 个 RPC、256 个顶层 message；`sys.ops` 大小写不变。
- HTTP 同时冻结两套计数：源码定义 244 条，当前可达并注册 242 条。`system/oss.api` 的 2 条路由继续保持不可达。
- `supper` 保持系统级超级管理员语义，不改名、不双写。
- 不迁移 `.env`、本地 AI/IDE 配置、二进制、日志、Node 依赖或真实凭据。
- 每一个生成动作先写到临时目录；所有门禁通过后才按精确 allowlist 同步。

## 1. 执行顺序

```text
计划 1：可编译迁移基线
  └─> 计划 2：syssvr RPC 契约拆分
        └─> 计划 3：HTTP API 契约补全
              └─> 计划 4：goctl/rpcgen 生成闭环与总验收
```

不得调换以下边界：

1. 没有迁移基线和旧契约快照，不拆 proto；
2. descriptor 语义比较未通过，不替换 PB；
3. HTTP defined/reachable/registered 快照未冻结，不改 `.api` 元数据；
4. goctl 补丁测试、alias 基线和事务同步未通过，不覆盖 client/server/direct；
5. 第一次生成差异未审核并提交，不执行“两次生成均零 diff”的最终幂等验收。

## 2. 子计划

### 计划 1：Core 可编译迁移基线

文件：`docs/superpowers/plans/2026-07-23-core-migration-foundation.md`

交付物：

- `backend/core` 单一 Go module；
- `backend/go.work`；
- 已净化配置、正确 AGPL 声明和 NOTICE；
- 输出到 `backend/bin` 的五服务 Makefile；
- 源提交和已知基线记录。

退出门禁：

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/core
go test -run '^$' ./...
make build
```

五个二进制必须存在且 Git 忽略；源仓库仍干净。

### 计划 2：syssvr RPC 契约拆分

文件：`docs/superpowers/plans/2026-07-23-syssvr-proto-split.md`

交付物：

- `base.proto`、15 个领域 proto 和 public-import 聚合 `sys.proto`；
- 旧 descriptor、client 公开 Go API 和注册集合 golden；
- `protocompat` 语义比较器；
- 固定旧插件版本生成的拆分 PB；
- 15 个服务运行时注册测试。

退出门禁：

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/protocompat ./cmd/protocompat

cd ../core
go test ./service/syssvr/client/... ./service/syssvr/internal/server/... ./service/syssvr/sysdirect
go build -o /dev/null ./service/syssvr
```

比较器必须报告 `compatible: 15 services, 215 methods, 256 messages`。

### 计划 3：HTTP API 契约补全

文件：`docs/superpowers/plans/2026-07-23-core-api-contract-normalization.md`

交付物：

- defined=244、reachable=242、registered=242 的独立快照；
- `.api` 中缺失的 `AppleAppConfig`；
- 244 条定义路由的七项显式元数据；
- route、auth、needAuth、旧 operType 语义门禁；
- OpenAPI/access 临时生成与人工审核流程。

退出门禁：

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/apicontract ./cmd/apicheck
go run ./cmd/apicheck verify --backend ..
```

不得导入 `system/oss.api`，不得自动覆盖 `backend/core/service/apisvr/etc/init/access`。

### 计划 4：goctl/rpcgen 生成闭环与总验收

文件：`docs/superpowers/plans/2026-07-23-goctl-rpcgen-toolchain.md`

交付物：

- goctl URL、SHA、Go 版本、源码和二进制 checksum 锁；
- 四个可审计补丁及 fork 定向测试；
- `rpcgen.yaml`、manifest/DAG/注册检查；
- imported/public compatibility aliases；
- 完整 Core candidate、精确 allowlist、`coretxn` 原子目录交换；
- 连续两次生成零 diff 和最终五入口构建。

退出门禁：

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./...
go run ./cmd/rpcgen check \
  --manifest ../core/service/syssvr/proto/rpcgen.yaml \
  --core ../core

cd ../core
go test ./...
make build
```

在第一次生成差异审核并提交后，从 clean worktree 连续执行两次 `rpcgen generate`，每次后面都必须 `git diff --exit-code`。

## 3. 每个任务的通用协议

- [ ] 开始前执行 `git status --short --branch`，确认只存在预期改动。
- [ ] 若任务涉及行为或校验器，先添加会失败的最小测试并实际观察失败。
- [ ] 只实现令当前测试通过的最小改动。
- [ ] 运行任务内的 focused tests，再运行该阶段退出门禁。
- [ ] 使用 `git diff --check` 检查空白错误。
- [ ] 检查源仓库：

```bash
test -z "$(git -C /home/wangjunbo/workspace-iot-platform/core status --porcelain)"
```

- [ ] 一个任务一个中文原子提交；不夹带架构重构、依赖升级或格式化噪声。
- [ ] 更新本计划对应 checkbox，记录实际命令和结果；不得把未运行命令写成已通过。

## 4. 最终契约矩阵

| 契约 | 基线 | 最终断言 |
| --- | ---: | --- |
| protobuf package | `sys` | 完全相同 |
| protobuf service | 15 | 完整名称与大小写相同 |
| RPC method | 215 | 输入、输出、streaming 相同 |
| 顶层 message | 256 | 字段号、类型、oneof、enum、reserved、options 相同 |
| direct / gRPC | 15 client 接口 | 公共接口和调用路径相同 |
| HTTP route definitions | 244 | method/path/handler/request/response 相同 |
| HTTP type shapes | golden | 字段名、规范化类型和 tag 相同 |
| HTTP reachable/registered | 242 | 不新增 `system/oss.api` 两条路由 |
| API metadata | 244 × 7 | 全部显式、业务描述非模板化 |
| authType | `all/admin/supper` | 旧 effective auth 决策相同 |
| 构建入口 | 5 | 全部构建到 `backend/bin` |

## 5. 完成定义

- [ ] 四份子计划全部完成，所有 checkbox 有可复核证据。
- [ ] `backend/core` 与 `backend/tools` 各自 `go test ./...` 通过。
- [ ] 五个 main 入口全部构建成功。
- [ ] gRPC descriptor、client 公开 Go API、服务注册和 HTTP 双计数快照均无非预期差异。
- [ ] 生成器在 clean worktree 连续运行两次均无差异。
- [ ] secret 扫描无非占位凭据，`.env` 和 `backend/bin` 未被跟踪。
- [ ] `LICENSE`、`NOTICE`、README 的 AGPL-3.0 与来源说明一致。
- [ ] 源 `core` 仓库仍位于固定提交且工作树干净。
