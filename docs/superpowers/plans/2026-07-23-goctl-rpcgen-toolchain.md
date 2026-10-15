# 固定 goctl 与 rpcgen 生成闭环实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把联犀 goctl fork 固定为可审计、可重复构建的项目工具，并用 `rpcgen`/`apicheck` 在临时目录中安全生成 RPC 与 API 代码；任何失败都不修改业务工作区，成功生成两次必须零差异。

**Architecture:** 项目只保存 upstream URL、完整 SHA、补丁和 checksum，不复制 fork。`rpcgen` 以显式 manifest 编排 protoc 和每领域 goctl，补齐旧 client alias、校验完整 client 公开 Go API 与服务注册，并在临时 Core 副本编译通过后按精确 allowlist 同步。API 同样先在临时 Core 副本生成，只接管完全生成的 types/routes；handler/logic/main/config/svc/startup 保持人工所有权。

**Tech Stack:** UnitedRhino go-zero/goctl fork、Git patches、Go 1.24.4、protoc 34.1、gRPC、YAML manifest、Go AST、SHA-256、Make。

---

## 前置条件

- 已按顺序完成前三份计划。
- `backend/core/service/syssvr/proto` 已拆分且 descriptor compatible。
- `backend/core/service/syssvr/client/testdata/public-api.json` 已冻结。
- `apicheck verify --backend ..` 报告 defined=244、reachable=242、registered=242、metadata=244。
- 目标仓库和源仓库均 clean。

## 固定上游事实

- Repository: `https://gitee.com/unitedrhino/go-zero.git`
- Commit: `6579a20bfd96919a80af5a9cb4e12f03598d4bb3`
- fork 源码硬编码的 `1.9.2-1` 不是可信身份，必须以 SHA + patches + checksum 识别。
- `rpc protoc` 一次只接受一个 proto；`-m` 表示单文件多 service，不表示多 proto 输入。
- fork 当前已有 unary/streaming direct client；本项目做回归测试，不预设额外业务补丁。
- fork 当前严格 API validator 不接受旧 `supper`。
- fork 的 `api new`/`api` 模板没有七项必填 metadata。
- fork 在 `genroutes.go` 构造 `wsBuilder` 并删除 `wsRoutes.go`，但没有写回；`--ws` 参数也在 2026 年更新中丢失。
- fork strict validator 已启用，但自身 `api/gogen` 和 `api/new` 测试 fixtures 仍不满足 metadata，原始定向测试不是全绿。
- Swagger 读取 route `action` 写扩展字段；access 生成器仍读取旧 `operType` 或 URL 推导 `add/modify/delete/find/other`。本次不把 action 直接塞入旧 operType。

### Task 1：创建 goctl lock 与只在临时目录构建的脚本

**Files:**

- Create: `backend/tools/goctl.lock`
- Create: `backend/tools/scripts/build-goctl.sh`
- Create: `backend/tools/scripts/build-goctl_test.sh`
- Create: `backend/tools/README.md`

- [ ] **Step 1: 创建 lock schema**

`backend/tools/goctl.lock` 使用 YAML，字段固定为：

```yaml
repository: https://gitee.com/unitedrhino/go-zero.git
commit: 6579a20bfd96919a80af5a9cb4e12f03598d4bb3
goVersion: go1.24.4
target:
  goos: linux
  goarch: amd64
  cgoEnabled: false
patches:
  - path: patches/goctl/0001-api-accept-supper.patch
  - path: patches/goctl/0002-api-new-required-metadata.patch
  - path: patches/goctl/0003-api-restore-ws-routes.patch
  - path: patches/goctl/0004-api-update-strict-validation-fixtures.patch
```

每个 patch object 的 `sha256` 以及顶层 `sourceArchiveSha256`/`binarySha256` 在 Task 6 首次可信构建并复核实际值后才加入，不能以空字符串或占位值进入文件。普通 build 缺少任一 checksum 必须失败；bootstrap 在可信源码和补丁测试全部通过后打印实际值，维护者通过 review 后写回 lock；后续构建只验证，不自动更新。

- [ ] **Step 2: 先写可执行的 build 脚本失败测试**

脚本接口：

```bash
bash backend/tools/scripts/build-goctl.sh bootstrap
bash backend/tools/scripts/build-goctl.sh build
```

`build-goctl_test.sh` 创建自包含临时 fixture，把 fake `git`/`go`/`sha256sum` 放到临时 `PATH`，逐项注入：commit 不一致、patch 缺失/checksum 不符、`git apply --check` 失败、Go 版本不符、fork tests 失败、source archive checksum 不符、binary checksum 不符。每个 case 比较 `backend/core` 的路径/mode/content SHA-256，证明脚本失败不触碰业务树；成功 case 还断言输出只能位于显式 repo root 的 `backend/bin/goctl`，不能落入 fork cwd。

先只创建 test runner 并观察 red：

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
bash backend/tools/scripts/build-goctl_test.sh
```

Expected: 因 `build-goctl.sh` 尚未实现或行为不完整而失败。

- [ ] **Step 3: 实现确定性构建流程**

脚本先用 `repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"` 解析 ming-iot 根，并断言 `git -C "$repo_root" rev-parse --show-toplevel` 与之相同；所有输出路径都从该绝对根拼接，不继承临时 fork 的 cwd。流程固定为：

```text
mktemp -d
git clone --no-checkout repository
git checkout --detach exact commit
按 lock 顺序逐个 git apply --check && git apply
在 tools/goctl module 中运行定向 tests
GOTOOLCHAIN=go1.24.4 CGO_ENABLED=0 GOOS=linux GOARCH=amd64
从脚本位置解析 ming-iot repository root
go build -trimpath -buildvcs=false -ldflags=-buildid= -o "$repo_root/backend/bin/goctl" .
sha256sum source git archive 和 binary
```

bootstrap stdout 只能输出按固定顺序排列的 patch/source/binary checksum 行；clone 路径、测试日志和进度写 stderr，保证两次 bootstrap 输出可直接 `diff`。

定向 tests：

```bash
cd "$fork/tools/goctl"
go test ./api/spec ./api/gogen ./api/new ./api/apigen \
  ./api/swagger ./api/access ./rpc/generator
```

`backend/bin/goctl` 仍由根 `.gitignore` 忽略。

Run green:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
bash backend/tools/scripts/build-goctl_test.sh
```

- [ ] **Step 4: 写工具边界文档**

`backend/tools/README.md` 必须说明：

- 业务 workspace 禁止直接执行任意系统 goctl；
- runtime go-zero v1.8.4 与 goctl fork 是独立版本轴；
- `action` 服务于语义/API 文档，`operType` 服务于旧审计枚举；
- API/RPC 生成必须走项目 wrapper；
- checksum 更新必须单独 review。

- [ ] **Step 5: 提交 lock 骨架和脚本**

补丁尚未加入，普通 build 应按预期失败：

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
bash backend/tools/scripts/build-goctl.sh build
```

Expected: 报告缺少 patches 或 checksum。

```bash
git add backend/tools/goctl.lock backend/tools/scripts/build-goctl.sh \
  backend/tools/scripts/build-goctl_test.sh backend/tools/README.md
git diff --cached --check
git commit -m "建立 goctl 固定版本构建骨架"
```

## goctl 补丁开发的统一临时环境

Task 2–5 每个任务都在被 Git 忽略的确定路径 `backend/.goctl-patch-work/<patch-number>/go-zero` 从固定 SHA 新建开发 clone，再按编号应用已经完成的前序 patch；不得直接在 `ming-iot` 中 vendor fork。每个 Run block 都重新赋值该任务的绝对 `fork` 路径并校验 SHA，不能依赖上一条 shell 命令里的变量：

```bash
fork=/home/wangjunbo/workspace-iot-platform/ming-iot/backend/.goctl-patch-work/0001/go-zero
test ! -e "$fork"
mkdir -p "$(dirname "$fork")"
git clone https://gitee.com/unitedrhino/go-zero.git "$fork"
git -C "$fork" checkout --detach 6579a20bfd96919a80af5a9cb4e12f03598d4bb3
```

开发 0002 时先应用 0001，开发 0003 时先应用 0001–0002，开发 0004 时先应用 0001–0003。用 `apply_patch` 修改临时 clone；验证后把相对固定 SHA + 前序 patch 的本次 diff 写为对应 patch 文件。重新创建一份 clean clone，顺序执行 `git apply --check`/`git apply`，证明 patch 独立且顺序可用。

### Task 2：补丁 0001——保留 supper 契约

**Files:**

- Create: `backend/tools/patches/goctl/0001-api-accept-supper.patch`

- [ ] **Step 1: 在专用 fork 写失败测试**

按统一环境块创建 `0001` clone。固定 SHA 中还没有 `tools/goctl/api/spec/validate_test.go`，用 `apply_patch` **新建**该文件并增加最小合法 route fixture，分别断言：

- `authType: "supper"` 通过；
- `authType: "unknown"` 返回 `ErrInvalidAuthType`。

在未改 validator 前运行：

```bash
fork=/home/wangjunbo/workspace-iot-platform/ming-iot/backend/.goctl-patch-work/0001/go-zero
test "$(git -C "$fork" rev-parse HEAD)" = 6579a20bfd96919a80af5a9cb4e12f03598d4bb3
cd "$fork/tools/goctl"
go test ./api/spec -run TestValidateAuthTypeSupper -v
```

Expected: supper case fail。

- [ ] **Step 2: 最小修改 validator**

在 `tools/goctl/api/spec/validate.go` 的 `ValidAuthTypes` 中增加：

```go
"supper": true,
```

保留 `platform/admin/all/public/device`，不做 supper/platform 自动转换。

- [ ] **Step 3: 导出并验证 patch**

patch 只包含上述 production/test 文件。把它写入项目路径后，从 clean fork SHA 运行：

```bash
verify_fork=/home/wangjunbo/workspace-iot-platform/ming-iot/backend/.goctl-patch-work/verify-0001/go-zero
test ! -e "$verify_fork"
mkdir -p "$(dirname "$verify_fork")"
git clone https://gitee.com/unitedrhino/go-zero.git "$verify_fork"
git -C "$verify_fork" checkout --detach 6579a20bfd96919a80af5a9cb4e12f03598d4bb3
git -C "$verify_fork" apply --check /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools/patches/goctl/0001-api-accept-supper.patch
git -C "$verify_fork" apply /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools/patches/goctl/0001-api-accept-supper.patch
cd "$verify_fork/tools/goctl" && go test ./api/spec -run TestValidateAuthType -v
```

- [ ] **Step 4: 提交**

```bash
git add backend/tools/patches/goctl/0001-api-accept-supper.patch
git diff --cached --check
git commit -m "修复 goctl supper 契约校验"
```

### Task 3：补丁 0002——新建模板默认满足严格 metadata

**Files:**

- Create: `backend/tools/patches/goctl/0002-api-new-required-metadata.patch`

- [ ] **Step 1: 写模板生成失败测试**

创建 `backend/.goctl-patch-work/0002/go-zero` clone，校验固定 SHA，并先顺序应用已提交的 0001。然后在 fork 测试中分别调用 `api` 顶层模板和 `api new` 模板，解析结果后执行 `ApiSpec.Validate()`；原始模板应因缺少 summary 等字段失败。

```bash
fork=/home/wangjunbo/workspace-iot-platform/ming-iot/backend/.goctl-patch-work/0002/go-zero
test ! -e "$fork"
mkdir -p "$(dirname "$fork")"
git clone https://gitee.com/unitedrhino/go-zero.git "$fork"
git -C "$fork" checkout --detach 6579a20bfd96919a80af5a9cb4e12f03598d4bb3
git -C "$fork" apply /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools/patches/goctl/0001-api-accept-supper.patch
```

- [ ] **Step 2: 修改两个模板**

修改：

```text
tools/goctl/api/apigen/api.tpl
tools/goctl/api/new/api.tpl
```

每条示例 route 使用 block `@doc`，包含可通过 validator 的七字段；示例值使用业务无关但合法的 `example/view/admin/1`，description 明确提示创建后必须改成实际业务语义。模板自身不得再生成只有 `@handler` 的 route。

- [ ] **Step 3: 验证并导出 patch**

```bash
fork=/home/wangjunbo/workspace-iot-platform/ming-iot/backend/.goctl-patch-work/0002/go-zero
test "$(git -C "$fork" rev-parse HEAD)" = 6579a20bfd96919a80af5a9cb4e12f03598d4bb3
cd "$fork/tools/goctl"
go test ./api/apigen ./api/new -run 'Test.*Template|TestCreateService' -v
```

从固定 SHA + 0001 后验证 0002 能顺序应用。

- [ ] **Step 4: 提交**

```bash
git add backend/tools/patches/goctl/0002-api-new-required-metadata.patch
git diff --cached --check
git commit -m "修复 goctl API 新建模板元数据"
```

### Task 4：补丁 0003——恢复可选 WebSocket routes 生成

**Files:**

- Create: `backend/tools/patches/goctl/0003-api-restore-ws-routes.patch`

- [ ] **Step 1: 写三个失败测试**

创建 `backend/.goctl-patch-work/0003/go-zero` clone，校验固定 SHA，并先顺序应用已提交的 0001–0002。在 `tools/goctl/api/gogen/genroutes_test.go` 增加：

```bash
fork=/home/wangjunbo/workspace-iot-platform/ming-iot/backend/.goctl-patch-work/0003/go-zero
test ! -e "$fork"
mkdir -p "$(dirname "$fork")"
git clone https://gitee.com/unitedrhino/go-zero.git "$fork"
git -C "$fork" checkout --detach 6579a20bfd96919a80af5a9cb4e12f03598d4bb3
git -C "$fork" apply /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools/patches/goctl/0001-api-accept-supper.patch
git -C "$fork" apply /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools/patches/goctl/0002-api-new-required-metadata.patch
```

1. `--ws` flag 已注册；
2. `ws=true` 同时生成 `routes.go` 和 `wsRoutes.go`，后者使用 `ws.Server`、`ws.WithPrefix` 和相同 route 集合；
3. `ws=false` 生成 `routes.go`，且不删除预先存在的 `wsRoutes.go` sentinel。

原始 SHA 三项均失败。

- [ ] **Step 2: 恢复参数传递但保留现有公开函数兼容**

修改：

```text
tools/goctl/api/cmd.go
tools/goctl/api/gogen/gen.go
tools/goctl/api/gogen/genroutes.go
```

要求：

- 恢复 `gogen.VarBoolWs` 和 `api go --ws`；
- 现有 `DoGenProject(..., withTest bool)` 与 `DoGenProjectWithModule(..., withTest bool)` 签名继续可用，默认 `ws=false`；
- 增加内部 options 路径供 CLI 传 `ws=true`；
- `genRoutes` 总是写 `routes.go`；仅 `ws=true` 时删除/写入 `wsRoutes.go`；
- ws template 使用现 fork 的 `gitee.com/unitedrhino/share/websocket`；
- ws data 包含 `projectPkg` 与当前 route template 所需全部字段；
- `ws=false` 绝不删除历史 ws 文件。

- [ ] **Step 3: 验证并导出 patch**

```bash
fork=/home/wangjunbo/workspace-iot-platform/ming-iot/backend/.goctl-patch-work/0003/go-zero
test "$(git -C "$fork" rev-parse HEAD)" = 6579a20bfd96919a80af5a9cb4e12f03598d4bb3
cd "$fork/tools/goctl"
go test ./api/gogen -run 'Test.*Ws|Test.*WS|TestRoutes' -v
go test ./api/new
```

- [ ] **Step 4: 提交**

```bash
git add backend/tools/patches/goctl/0003-api-restore-ws-routes.patch
git diff --cached --check
git commit -m "恢复 goctl WebSocket 路由生成"
```

### Task 5：补丁 0004——使 fork 自身 fixtures 与 strict validator 一致

**Files:**

- Create: `backend/tools/patches/goctl/0004-api-update-strict-validation-fixtures.patch`

- [ ] **Step 1: 复现 upstream 已知失败**

创建 `backend/.goctl-patch-work/0004/go-zero` clone，校验固定 SHA，并顺序应用已提交的 0001–0003 后运行：

```bash
fork=/home/wangjunbo/workspace-iot-platform/ming-iot/backend/.goctl-patch-work/0004/go-zero
test ! -e "$fork"
mkdir -p "$(dirname "$fork")"
git clone https://gitee.com/unitedrhino/go-zero.git "$fork"
git -C "$fork" checkout --detach 6579a20bfd96919a80af5a9cb4e12f03598d4bb3
git -C "$fork" apply /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools/patches/goctl/0001-api-accept-supper.patch
git -C "$fork" apply /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools/patches/goctl/0002-api-new-required-metadata.patch
git -C "$fork" apply /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools/patches/goctl/0003-api-restore-ws-routes.patch
cd "$fork/tools/goctl"
go test ./api/gogen ./api/new
```

Expected: `gen_test.go`、`gencomment_test.go`、`gensse_test.go`、`newservice_test.go` 中旧 fixture 因缺七项 metadata 失败。

- [ ] **Step 2: 只更新测试 fixtures**

修改范围只能是：

```text
tools/goctl/api/gogen/gen_test.go
tools/goctl/api/gogen/gencomment_test.go
tools/goctl/api/gogen/gensse_test.go
tools/goctl/api/new/newservice_test.go
```

给这四个文件中所有预期成功的 fixture 加合法七字段；预期 parser/validator 失败的 case 保持原测试意图，不用绕过 validation。patch 不得修改 production code。

- [ ] **Step 3: 全量定向验证**

```bash
fork=/home/wangjunbo/workspace-iot-platform/ming-iot/backend/.goctl-patch-work/0004/go-zero
test "$(git -C "$fork" rev-parse HEAD)" = 6579a20bfd96919a80af5a9cb4e12f03598d4bb3
cd "$fork/tools/goctl"
go test ./api/spec ./api/gogen ./api/new ./api/apigen \
  ./api/swagger ./api/access ./rpc/generator
```

Expected: 全部 PASS。

- [ ] **Step 4: 提交**

```bash
git add backend/tools/patches/goctl/0004-api-update-strict-validation-fixtures.patch
git diff --cached --check
git commit -m "修复 goctl 严格校验测试样例"
```

### Task 6：锁定 checksum 并构建 goctl

**Files:**

- Modify: `backend/tools/goctl.lock`
- Create: `docs/superpowers/evidence/2026-07-23-goctl-build.md`

- [ ] **Step 1: bootstrap 两次并确认确定性**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
bash backend/tools/scripts/build-goctl.sh bootstrap > /tmp/goctl-bootstrap-1.txt
bash backend/tools/scripts/build-goctl.sh bootstrap > /tmp/goctl-bootstrap-2.txt
diff -u /tmp/goctl-bootstrap-1.txt /tmp/goctl-bootstrap-2.txt
```

Expected: source archive SHA 和 binary SHA 两次相同。

- [ ] **Step 2: 把实际 checksum 写入 lock 并用普通 build 验证**

将 bootstrap 输出的六个 64 位小写十六进制值写入对应字段：四个 patch object 各自增加 `sha256`，顶层增加 `sourceArchiveSha256` 与 `binarySha256`。然后：

```bash
bash backend/tools/scripts/build-goctl.sh build
backend/bin/goctl --version
sha256sum backend/bin/goctl
git check-ignore -q backend/bin/goctl
```

- [ ] **Step 3: 记录构建身份**

证据文件记录 repository、commit、四个 patch 各自 sha256、Go/GOOS/GOARCH/CGO、source archive SHA、binary SHA、定向 test 结果。不得只记录 fork 的显示版本。

- [ ] **Step 4: 提交**

```bash
git add backend/tools/goctl.lock docs/superpowers/evidence/2026-07-23-goctl-build.md
git diff --cached --check
git commit -m "锁定项目 goctl 可复现构建"
```

### Task 7：用固定 goctl 验证 API，并独立比较 access 语义

**Files:**

- Modify: `backend/tools/internal/apicontract/registered.go`
- Test: `backend/tools/internal/apicontract/registered_test.go`
- Modify: `backend/tools/cmd/apicheck/main.go`

- [ ] **Step 1: 增加 generated access 比较测试**

新增命令：

```bash
apicheck verify-access --backend BACKEND_ROOT --service SERVICE --file GENERATED_ACCESS_JSON
```

它对 `ManagedByAccess=true` 的路由比较 method/fullPath/authType/needAuth/operType 与 authorization golden，并断言生成集合与该子集相同；忽略 JSON 顺序、展示名称和 access 树物理顺序。写失败测试证明把 `action: view` 误写成旧 `operType: view` 会被 golden 捕获。

- [ ] **Step 2: strict validate 两个聚合入口**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
backend/bin/goctl api validate --api backend/core/service/apisvr/http/api.api
backend/bin/goctl api validate --api backend/core/service/datasvr/http/data.api
```

detached 的 2 条 OSS 定义由 `apicheck verify` 校验；不导入聚合入口。

- [ ] **Step 3: 在一个 shell 中生成并验证临时产物**

```bash
set -euo pipefail
cd /home/wangjunbo/workspace-iot-platform/ming-iot
tmp_api="$(mktemp -d)"
trap 'rm -rf -- "$tmp_api"' EXIT
mkdir -p "$tmp_api/apisvr-swagger" "$tmp_api/apisvr-access" \
  "$tmp_api/datasvr-swagger" "$tmp_api/datasvr-access"
backend/bin/goctl api swagger \
  --api backend/core/service/apisvr/http/api.api \
  --dir "$tmp_api/apisvr-swagger" --filename openapi.json \
  --host localhost --basepath / --schemes http
backend/bin/goctl api access \
  --api backend/core/service/apisvr/http/api.api \
  --dir "$tmp_api/apisvr-access" --filename access.json \
  --host localhost --basepath /
backend/bin/goctl api swagger \
  --api backend/core/service/datasvr/http/data.api \
  --dir "$tmp_api/datasvr-swagger" --filename openapi.json \
  --host localhost --basepath / --schemes http
backend/bin/goctl api access \
  --api backend/core/service/datasvr/http/data.api \
  --dir "$tmp_api/datasvr-access" --filename access.json \
  --host localhost --basepath /
cd backend/tools
go test ./internal/apicontract ./cmd/apicheck
go run ./cmd/apicheck verify --backend ..
go run ./cmd/apicheck verify-access --backend .. \
  --service apisvr \
  --file "$tmp_api/apisvr-access/access.json"
go run ./cmd/apicheck verify-access --backend .. \
  --service datasvr \
  --file "$tmp_api/datasvr-access/access.json"
```

先创建四个 `--dir` 目录；不得指向业务 `http/` 或 `etc/init/access`。上述生成与两个 `verify-access` 必须位于同一个 shell，确保 `tmp_api` 生命周期完整。

- [ ] **Step 4: 提交工具改动**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add backend/tools/internal/apicontract backend/tools/cmd/apicheck
git diff --cached --check
git commit -m "校验 goctl API 生成授权语义"
```

### Task 8：创建显式 rpcgen manifest 和校验器

**Files:**

- Create: `backend/core/service/syssvr/proto/rpcgen.yaml`
- Create: `backend/tools/internal/rpcgen/manifest.go`
- Create: `backend/tools/internal/rpcgen/descriptor.go`
- Create: `backend/tools/internal/rpcgen/registry.go`
- Test: `backend/tools/internal/rpcgen/manifest_test.go`
- Test: `backend/tools/internal/rpcgen/descriptor_test.go`
- Test: `backend/tools/internal/rpcgen/registry_test.go`
- Create: `backend/tools/cmd/rpcgen/main.go`
- Test: `backend/tools/cmd/rpcgen/main_test.go`

- [ ] **Step 1: 写 manifest 失败测试**

测试分别拒绝：package/goPackage 不一致、aggregate 少 public import、重复 service、领域 proto 无 service/多 service、manifest 名与 descriptor 不同、import cycle、实际 service 集合不是 15、registry 少/多注册。

先固定 YAML 依赖：

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go get gopkg.in/yaml.v3@v3.0.1
```

- [ ] **Step 2: 创建精确 manifest**

固定头部：

```yaml
version: 1
runtime: syssvr
protoDir: service/syssvr/proto
pbDir: service/syssvr/pb/sys
package: sys
goPackage: pb/sys
aggregate: sys.proto
shared:
  - base.proto
registry: service/syssvr/sysdirect/sysdirect.go
clientContractGolden: service/syssvr/client/testdata/public-api.json
```

services 按以下顺序和映射：

| proto | service | packageDir | clientFile | serverFile |
| --- | --- | --- | --- | --- |
| access.proto | AccessManage | accessmanage | accessManage.go | accessManageServer.go |
| app.proto | AppManage | appmanage | appManage.go | appManageServer.go |
| area.proto | AreaManage | areamanage | areaManage.go | areaManageServer.go |
| common.proto | Common | common | common.go | commonServer.go |
| data.proto | DataManage | datamanage | dataManage.go | dataManageServer.go |
| department.proto | DepartmentManage | departmentmanage | departmentManage.go | departmentManageServer.go |
| dict.proto | DictManage | dictmanage | dictManage.go | dictManageServer.go |
| log.proto | Log | log | log.go | logServer.go |
| module.proto | ModuleManage | modulemanage | moduleManage.go | moduleManageServer.go |
| notify.proto | NotifyManage | notifymanage | notifyManage.go | notifyManageServer.go |
| ops.proto | ops | ops | ops.go | opsServer.go |
| project.proto | ProjectManage | projectmanage | projectManage.go | projectManageServer.go |
| role.proto | RoleManage | rolemanage | roleManage.go | roleManageServer.go |
| tenant.proto | TenantManage | tenantmanage | tenantManage.go | tenantManageServer.go |
| user.proto | UserManage | usermanage | userManage.go | userManageServer.go |

每一行写成 manifest object；不使用 glob 或从驼峰猜文件名。

- [ ] **Step 3: 实现 descriptor/DAG/registry 校验**

`rpcgen check` 接口：

```bash
rpcgen check --manifest MANIFEST --core CORE_ROOT
```

它必须：加载 YAML；默认从 `filepath.Join(filepath.Dir(CORE_ROOT), "bin/protobuf-tools")` 解析固定 protoc，允许用 `--protobuf-tools DIR` 显式覆盖；生成临时 descriptor；调用 `protocompat` 内部库；验证 manifest 与 descriptor；解析 `registerServers` 中 15 个 `sys.Register*Server` AST call；与 manifest 和 descriptor 做三方精确集合比较。

- [ ] **Step 4: 验证并提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/rpcgen ./cmd/rpcgen
go run ./cmd/rpcgen check \
  --manifest ../core/service/syssvr/proto/rpcgen.yaml \
  --core ../core

cd ../..
git add backend/core/service/syssvr/proto/rpcgen.yaml \
  backend/tools/internal/rpcgen backend/tools/cmd/rpcgen
git diff --cached --check
git commit -m "建立 syssvr 多 proto 生成清单"
```

### Task 9：实现 compatibility alias 生成器

**Files:**

- Create: `backend/tools/internal/rpcgen/aliases.go`
- Test: `backend/tools/internal/rpcgen/aliases_test.go`
- Generated later: `backend/core/service/syssvr/client/*/compat_aliases.go`

- [ ] **Step 1: 写 alias 差集测试**

fixture 包含：golden 已有且 goctl 已生成、golden 缺失需补齐、同名目标不同、重复 alias、imported message。断言只为缺失项生成，冲突直接失败。

- [ ] **Step 2: 实现确定性 alias 文件**

每个 `compat_aliases.go`：

```go
// Code generated by rpcgen. DO NOT EDIT.

package <manifest.packageDir>

import "gitee.com/unitedrhino/core/service/syssvr/pb/sys"

type (
	// 按 alias name 排序，仅包含 goctl 结果缺少但 golden 要求的别名。
)
```

生成后用既有 `clientcontract verify` 比较完整公开面：alias 必须得到 14×254 + 255 = 3811，15 个 interface 及其 `New`/`NewDirect` signatures 也必须与 golden 相同。

- [ ] **Step 3: 测试并提交实现**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/rpcgen -run TestAliases -v

cd ../..
git add backend/tools/internal/rpcgen/aliases.go backend/tools/internal/rpcgen/aliases_test.go
git diff --cached --check
git commit -m "生成 syssvr client 兼容别名"
```

### Task 10：实现 RPC 临时生成、预同步编译和精确同步

**Files:**

- Create: `backend/tools/internal/rpcgen/generate.go`
- Create: `backend/tools/internal/rpcgen/candidate.go`
- Test: `backend/tools/internal/rpcgen/generate_test.go`
- Test: `backend/tools/internal/rpcgen/candidate_test.go`
- Modify: `backend/tools/cmd/rpcgen/main.go`

- [ ] **Step 1: 写事务失败测试**

通过 fake command runner 分别注入 protoc 失败、第三个领域 goctl 失败、descriptor diff、alias conflict、registry mismatch、candidate 编译失败、allowlist 外文件和 `coretxn` commit 前失败；每次对 live Core 树做 before/after SHA 清单，断言完全相同。

写 candidate 测试：只能替换 allowlist 文件；精确删除 manifest 计算出的陈旧生成文件；拒绝路径穿越、symlink 和 core root 之外路径。`coretxn` 的 exchange/恢复语义复用 proto 计划已经完成的测试，不再实现第二套逐文件同步器。

- [ ] **Step 2: 实现稳定生成流水线**

CLI：

```bash
rpcgen generate \
  --manifest MANIFEST \
  --core CORE_ROOT \
  --goctl GOCTL_BINARY \
  --protobuf-tools PROTOBUF_TOOL_DIR
```

执行顺序：

1. `check` 全部校验；
2. 通过 `coretxn` 在 `backend/.coretxn/<id>/candidate` 创建完整 Core candidate，并创建独立的 generator staging；
3. 把 `--protobuf-tools` 目录只 prepend 到子进程 `PATH`，并给全部 protoc/goctl/Go 子进程显式设置 `GOWORK=off`；一次显式列出全部 proto 调用其中固定的 protoc/go plugins，生成 PB；
4. 按 manifest 顺序对每个领域 proto 调用固定 goctl；每个领域使用带 `module gitee.com/unitedrhino/core` 的独立 `DOMAIN_PROJECT`（由 Go 代码以 mode `0644` 写入固定 `go.mod` 内容），并把 zrpc root 固定为 `DOMAIN_PROJECT/service/syssvr`，禁止依赖“第一次生成共享骨架”；每个失败测试还要比较 live `backend/go.work` 的 SHA-256，证明 fork 的 `UpdateGoWorkIfExist` 没有改写 workspace；
5. 每次必须以 `-m` 开启 service 分组，从 `service/syssvr/client/<service>` 和 `service/syssvr/internal/server/<service>` 只收集 manifest 指定文件；断言生成文件 import 指向 `gitee.com/unitedrhino/core/service/syssvr/{pb/sys,internal/logic/<service>,internal/svc}`，且源码中不存在 staging 绝对路径；不收集 main/config/svc/startup/logic；
6. 依据 client contract golden 中的 aliases 生成 `compat_aliases.go`，并校验 interface/constructor signatures；
7. 在 staging 运行 descriptor、registry、clientcontract 和生成路径 allowlist 校验；
8. 把 PB/client/server allowlist 覆盖进完整 candidate，复核 candidate 与 live Core 只有 allowlist 差异；在 candidate 内以 `GOWORK=off` 运行 focused tests 和 `go test -run '^$' ./...`；
9. 全部通过后调用 `coretxn`，持锁、复核 live 指纹并以一次 `RENAME_EXCHANGE` 原子提交整棵 Core；
10. 交换成功即报告 committed；旧 tree/journal 清理失败只告警并留给下次 recovery。

goctl 单领域命令固定为以下形态，`proto` 和输出目录来自 manifest/staging：

```bash
GOCTL rpc protoc CORE_ROOT/service/syssvr/proto/DOMAIN.proto \
  --go_out=DOMAIN_PROJECT/service/syssvr \
  --go-grpc_out=DOMAIN_PROJECT/service/syssvr \
  --zrpc_out=DOMAIN_PROJECT/service/syssvr \
  --proto_path=CORE_ROOT/service/syssvr/proto \
  --proto_path=PROTOBUF_TOOL_DIR/include \
  --style=goZero \
  --module=gitee.com/unitedrhino/core \
  -m
```

命令中的大写路径名是 `rpcgen` 内部解析后的绝对参数，不是要求 shell 直接执行的占位脚本。`-m` **不**表示多 proto 输入，而是即使单个领域只有一个 service，也要求 goctl 生成 `client/<service>`、`internal/server/<service>` 和对应 logic 分组，以匹配现有目录及 import。不得改回 compatibility mode。

- [ ] **Step 3: 定义同步 allowlist**

只允许：

```text
service/syssvr/pb/sys/<Task 6 的 32 个固定 PB 文件>
service/syssvr/client/<manifest.packageDir>/<manifest.clientFile>
service/syssvr/client/<manifest.packageDir>/compat_aliases.go
service/syssvr/internal/server/<manifest.packageDir>/<manifest.serverFile>
```

禁止同步：

```text
service/syssvr/sys.go
service/syssvr/sysdirect/**
service/syssvr/internal/config/**
service/syssvr/internal/svc/**
service/syssvr/internal/startup/**
service/syssvr/internal/logic/**
```

- [ ] **Step 4: 增加 unary/streaming direct 回归 fixture**

在测试 temp proto 中声明 unary、server stream、client stream、bidi stream；固定 goctl 生成后断言 gRPC 与 direct client 都实现同一个导出 interface，并运行一个不启动网络的 fake server 调用测试。

- [ ] **Step 5: 验证工具自身并提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/rpcgen ./cmd/rpcgen

cd ../..
git add backend/tools/internal/rpcgen backend/tools/cmd/rpcgen
git diff --cached --check
git commit -m "实现 rpcgen 事务生成与同步"
```

### Task 11：首次 RPC 重生成、审核并提交生成差异

**Files:**

- Regenerate: `backend/core/service/syssvr/pb/sys/*.pb.go`
- Regenerate: `backend/core/service/syssvr/client/*/*.go`
- Regenerate: `backend/core/service/syssvr/internal/server/*/*.go`
- Create: `backend/core/service/syssvr/client/*/compat_aliases.go`

- [ ] **Step 1: 从 clean tree 执行第一次生成**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go run ./cmd/rpcgen generate \
  --manifest ../core/service/syssvr/proto/rpcgen.yaml \
  --core ../core \
  --goctl ../bin/goctl \
  --protobuf-tools ../bin/protobuf-tools
```

- [ ] **Step 2: 审核预期差异**

只允许 Task 10 allowlist 中的生成文件变化。检查：

```bash
cd ../..
git status --short
git diff --stat
git diff -- backend/core/service/syssvr/sysdirect \
  backend/core/service/syssvr/internal/logic \
  backend/core/service/syssvr/internal/config \
  backend/core/service/syssvr/internal/svc
```

最后一条必须无输出。

- [ ] **Step 3: 运行契约、编译和 direct 门禁**

```bash
cd backend/tools
go run ./cmd/rpcgen check \
  --manifest ../core/service/syssvr/proto/rpcgen.yaml --core ../core
go run ./cmd/clientcontract verify \
  --root ../core/service/syssvr/client \
  --golden ../core/service/syssvr/client/testdata/public-api.json

cd ../core
go test ./service/syssvr/client/... ./service/syssvr/internal/server/... ./service/syssvr/sysdirect
go test -run '^$' ./...
```

- [ ] **Step 4: 提交第一次审核后的生成结果**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add backend/core/service/syssvr/pb/sys \
  backend/core/service/syssvr/client \
  backend/core/service/syssvr/internal/server
git diff --cached --check
git commit -m "接管 syssvr 可重生成代码"
```

### Task 12：实现并执行 API 安全重生成

**Files:**

- Create: `backend/tools/internal/apicontract/generate.go`
- Test: `backend/tools/internal/apicontract/generate_test.go`
- Modify: `backend/tools/cmd/apicheck/main.go`
- Regenerate: `backend/core/service/apisvr/internal/types/types.go`
- Regenerate: `backend/core/service/apisvr/internal/handler/routes.go`
- Regenerate: `backend/core/service/datasvr/internal/types/types.go`
- Regenerate: `backend/core/service/datasvr/internal/handler/routes.go`

- [ ] **Step 1: 写 API candidate 生成测试**

测试固定 goctl 失败、route golden 变化、生成额外 handler/logic、candidate 编译失败、allowlist 外差异和 `coretxn` commit 前失败时 live Core 零变化；另断言 API 路径复用既有 `coretxn`，没有逐文件写入 live Core 的代码路径。

- [ ] **Step 2: 增加 generate 命令**

```bash
apicheck generate --backend BACKEND_ROOT --goctl GOCTL_BINARY
```

执行协议：

1. 用 `coretxn` 创建完整 Core candidate，另复制一份完整 Core 到 generator scratch；所有 goctl/Go 子进程显式 `GOWORK=off`，并在失败测试中比较 live `backend/go.work` 的前后 SHA-256；
2. 分别在 scratch 的 apisvr/datasvr 运行 `goctl api go`，再运行完整 `apicheck verify`；
3. 从 scratch 的 `internal/types/types.go` 解析导出 type shape 并与 `types.golden.json` 比较；
4. 只把两套 `internal/types/types.go` 与 `internal/handler/routes.go` 共四个文件复制进 candidate；scratch 中新增/修改的 handler、logic、main、config、svc、middleware 一律不进入 candidate；
5. 复核 candidate 与 live Core 只有上述四个文件差异，并在 candidate 中运行 `GOWORK=off go test -run '^$' ./...`；编译失败禁止 commit；
6. 通过 `coretxn` 原子交换整棵 Core。本次不传 `--ws`，candidate 会保留 live Core 中历史未注册的 datasvr `wsRoutes.go`。

- [ ] **Step 3: 运行、审核并提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./internal/apicontract ./cmd/apicheck
go run ./cmd/apicheck generate --backend .. --goctl ../bin/goctl
go run ./cmd/apicheck verify --backend ..

cd ../core
go test -run '^$' ./...

cd ../..
git diff -- backend/core/service/apisvr/internal/handler/routes.go \
  backend/core/service/apisvr/internal/types/types.go \
  backend/core/service/datasvr/internal/handler/routes.go \
  backend/core/service/datasvr/internal/types/types.go
git add backend/tools/internal/apicontract backend/tools/cmd/apicheck \
  backend/core/service/apisvr/internal/{handler/routes.go,types/types.go} \
  backend/core/service/datasvr/internal/{handler/routes.go,types/types.go}
git diff --cached --check
git commit -m "接管 Core API 可重生成代码"
```

### Task 13：连续两次生成零差异

- [ ] **Step 1: 确认起点 clean**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
test -z "$(git status --porcelain)"
```

- [ ] **Step 2: 第一次完整生成后零 diff**

```bash
cd backend/tools
go run ./cmd/rpcgen generate \
  --manifest ../core/service/syssvr/proto/rpcgen.yaml \
  --core ../core --goctl ../bin/goctl \
  --protobuf-tools ../bin/protobuf-tools
go run ./cmd/apicheck generate --backend .. --goctl ../bin/goctl
cd ../..
git diff --exit-code
test -z "$(git status --porcelain)"
```

- [ ] **Step 3: 第二次完整生成后仍零 diff**

重复 Step 2 的全部命令。Expected: 两次 `git diff --exit-code` 和两次 clean-status 断言都返回 0。

### Task 14：最终全量验收

**Files:**

- Create: `docs/superpowers/evidence/2026-07-23-core-migration-verification.md`

- [ ] **Step 1: tools 全量测试**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
go test ./...
go run ./cmd/rpcgen check \
  --manifest ../core/service/syssvr/proto/rpcgen.yaml --core ../core
go run ./cmd/apicheck verify --backend ..
```

- [ ] **Step 2: Core 全量测试和五入口构建**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/core
CORE_LOCAL_SKIP_STARTUP_SIDE_EFFECTS=1 \
CORE_LOCAL_SKIP_AUTO_MIGRATE=1 \
go test ./...
make build
for name in coresvr syssvr datasvr timedjobsvr timedschedulersvr; do
  test -x "../bin/$name"
done
```

- [ ] **Step 3: 契约、安全、许可证和源仓门禁**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
test -z "$(git ls-files 'backend/bin/**' '**/.env')"
test -z "$(rg -nP '^[ ]*(DSN|AccessSecret|AccessKey|AccessKeyID|AccessKeySecret|AppKey|AppSecret|Password|SecondSecret|ThirdJwtSecret|Secret|AccessToken):[ ]+(?!\$\{|""|$)' backend/core/service --glob '*.yaml' --glob '*.yml')"
test "$(sha256sum backend/core/LICENSE | awk '{print $1}')" = "20b067f86de375aae6db0f283ab2e65de24d537733b89bd58432c101259d84cf"
test -z "$(git -C ../core status --porcelain)"
test "$(git -C ../core rev-parse HEAD)" = "1b3110e5e17d5a0066d38930b67bc44fdcc3c6fc"
git diff --check
```

- [ ] **Step 4: 写验收证据**

证据文件逐项记录实际命令、exit code 和摘要：

```text
RPC: 15 services / 215 methods / 256 messages compatible
HTTP: 244 defined / 242 reachable / 242 registered / 2 detached
Metadata: 244 routes × 7 fields complete
Auth: supper preserved; access auth/needAuth/operType unchanged
Generation: two consecutive clean runs
Tests: tools and core pass
Builds: five binaries pass
Secrets/license/source worktree: pass
```

任何未通过项写 fail 和错误，不得写成 pass。

- [ ] **Step 5: 提交验收证据**

```bash
git add docs/superpowers/evidence/2026-07-23-core-migration-verification.md
git diff --cached --check
git commit -m "记录 Core 迁移全量验收结果"
git status --short --branch
```

最终状态必须 clean；本计划不创建 `backend/cmd/allInOne`，不升级运行时 go-zero，不启用 `system/oss.api`，不自动覆盖历史 access 初始化数据。
