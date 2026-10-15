# Core 可编译迁移基线实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从固定的联犀 Core 提交迁入完整业务源码，形成无真实密钥、许可证清晰、五个服务均可构建的 `ming-iot/backend/core` 基线。

**Architecture:** 只从源 Git 提交归档中导入运行所需 tracked files，源仓库保持只读。目标继续使用原 module path 和外部 `gitee.com/unitedrhino/share` 依赖；Go Workspace 只编排本地 module。构建产物统一放到目标仓库的 `backend/bin`。

**Tech Stack:** Git archive、Go 1.24.4、Go Workspace、Make、go-zero v1.8.4、YAML 环境变量展开、AGPL-3.0。

---

## 前置条件

- 工作目录：`/home/wangjunbo/workspace-iot-platform/ming-iot`。
- 源仓库：`../core`。
- 固定源 SHA：`1b3110e5e17d5a0066d38930b67bc44fdcc3c6fc`。
- 目标 `backend/core` 必须为空；若不是空目录，停止并核对已有文件，禁止覆盖。
- 本计划迁移源仓 `tools/` 下两个 Go 辅助程序，使 Go module 的 236 个 package 保持完整。源仓的 `AGENTS.md`、`CLAUDE.md`、`.claude`、deploy、scripts、shell、package.json 和 Dockerfile 不迁移：它们包含旧仓协作规则、固定远端地址或不适用的私有镜像假设，不属于本次 Core module 边界；容器化另行设计。

### Task 1：冻结源提交和只编译基线

**Files:**

- Create: `docs/superpowers/evidence/2026-07-23-core-source-baseline.md`

- [ ] **Step 1: 验证源仓库身份与 clean 状态**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
test "$(git -C ../core rev-parse HEAD)" = "1b3110e5e17d5a0066d38930b67bc44fdcc3c6fc"
test -z "$(git -C ../core status --porcelain)"
git -C ../core ls-files | wc -l
git -C ../core ls-tree -rl HEAD | awk '{sum += $4} END {print sum}'
```

Expected: 两个 `test` 返回 0；tracked files 为 `1188`，tracked bytes 为盘点时记录的约 `21.1 MB`。

- [ ] **Step 2: 重跑源仓只编译基线**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/core
GOWORK=off go test -run '^$' ./...
```

Expected: exit 0。该命令只编译测试，不运行测试函数，也不要求数据库、Redis 或 NATS。

- [ ] **Step 3: 写入基线证据**

Create `docs/superpowers/evidence/2026-07-23-core-source-baseline.md`，内容必须精确包含：

```markdown
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
```

若 Step 2 实际失败，把结果写成 fail，并逐条记录失败 package；不得照抄 `pass`。

- [ ] **Step 4: 提交基线证据**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git add docs/superpowers/evidence/2026-07-23-core-source-baseline.md
git commit -m "记录 Core 迁移源基线"
```

### Task 2：按 allowlist 导入源码并在首次提交前净化凭据

**Files:**

- Create: `backend/core/go.mod`
- Create: `backend/core/go.sum`
- Create: `backend/core/LICENSE`
- Create: `backend/core/README.md`
- Create: `backend/core/README.en.md`
- Create: `backend/core/doc/**`
- Create: `backend/core/service/**`
- Create: `backend/core/share/**`
- Create: `backend/core/tools/**`
- Exclude: `backend/core/service/apisvr/.env`
- Modify: `backend/core/service/syssvr/internal/config/config.go`
- Modify: 15 个下列 YAML：
  - `backend/core/service/apisvr/etc/api.yaml`
  - `backend/core/service/apisvr/etc/apiDocker.yaml`
  - `backend/core/service/apisvr/etc/apiMinDocker.yaml`
  - `backend/core/service/apisvr/etc/common.yaml`
  - `backend/core/service/apisvr/etc/commonDocker.yaml`
  - `backend/core/service/apisvr/etc/commonMinDocker.yaml`
  - `backend/core/service/apisvr/etc/commonSmall.yaml`
  - `backend/core/service/apisvr/etc/commonWin.yaml`
  - `backend/core/service/apisvr/etc/dataDocker.yaml`
  - `backend/core/service/apisvr/etc/sys.yaml`
  - `backend/core/service/apisvr/etc/sysDocker.yaml`
  - `backend/core/service/datasvr/etc/data.yaml`
  - `backend/core/service/syssvr/etc/sys.yaml`
  - `backend/core/service/timed/timedjobsvr/etc/timedjob.yaml`
  - `backend/core/service/timed/timedschedulersvr/etc/timedscheduler.yaml`

- [ ] **Step 1: 确认目标为空并机械导入 tracked files**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
test -z "$(find backend/core -mindepth 1 -print -quit)"
git -C ../core archive --format=tar \
  1b3110e5e17d5a0066d38930b67bc44fdcc3c6fc \
  go.mod go.sum LICENSE README.md README.en.md doc service share tools \
  | tar --exclude='service/apisvr/.env' -xf - -C backend/core
test ! -e backend/core/service/apisvr/.env
test "$(git -C ../core rev-parse HEAD)" = "1b3110e5e17d5a0066d38930b67bc44fdcc3c6fc"
```

Expected: 目标出现 1161 个导入文件；`.env` 不存在；源提交不变。

- [ ] **Step 2: 运行预期失败的明文配置扫描**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
rg -nP '^[ ]*(DSN|AccessSecret|AccessKey|AccessKeyID|AccessKeySecret|AppKey|AppSecret|Password|SecondSecret|ThirdJwtSecret|Secret|AccessToken):[ ]+(?!\$\{|""|$)' \
  backend/core/service --glob '*.yaml' --glob '*.yml'
rg -n 'default=.*ThirdJwtSecret|ThirdJwtSecret.*default=' \
  backend/core/service/syssvr/internal/config/config.go
```

Expected: 两条命令都有命中，证明门禁能发现迁入的字面量凭据；在净化完成前禁止 `git add backend/core`。

- [ ] **Step 3: 将 YAML 字面量凭据替换为环境变量**

按配置语义使用以下固定映射，注释中的示例 DSN 也改成不含密码的环境变量注释：

| 配置语义 | 值 |
| --- | --- |
| 所有 `Database.DSN` | `${CORE_DB_DSN}` |
| `Rej.AccessSecret` | `${CORE_REJ_ACCESS_SECRET}` |
| `UserToken.AccessSecret` | `${CORE_USER_TOKEN_SECRET}` |
| OpenAuth `nodeRed` 密码 | `${CORE_OPENAUTH_NODERED_PASSWORD}` |
| OpenAuth `root` 密码 | `${CORE_OPENAUTH_ROOT_PASSWORD}` |
| `OssConf.AccessKeyID` | `${CORE_OSS_ACCESS_KEY_ID}` |
| `OssConf.AccessKeySecret` | `${CORE_OSS_ACCESS_KEY_SECRET}` |
| `WexinMiniprogram.AppSecret` | `${CORE_WECHAT_MINI_APP_SECRET}` |
| `Map.AccessKey` | `${CORE_MAP_ACCESS_KEY}` |
| `Map.AccessSecret` | `${CORE_MAP_ACCESS_SECRET}` |
| `ThirdJwtSecret` | `${THIRD_JWT_SECRET}` |
| `GoogleJWKS.AccessToken` 的非空值 | `${GOOGLE_JWKS_ACCESS_TOKEN}` |
| `UniPush.Secret` 的非空值 | `${YK_PUSH_HTTP_SECRET}` |
| SMS 厂商密钥的非空值 | `${CORE_SMS_ACCESS_KEY_ID}`、`${CORE_SMS_ACCESS_KEY_SECRET}`、`${CORE_SMS_APP_KEY}` |

`gitee.com/unitedrhino/share/utils.ConfMustLoad` 会执行环境变量展开，因此配置文件中保留 `${VAR}` 形式，不引入新的配置加载器。

- [ ] **Step 4: 删除 Go struct tag 中的默认 JWT secret**

In `backend/core/service/syssvr/internal/config/config.go` replace:

```go
ThirdJwtSecret string `json:",default=Xk9mQ2vR7pLw4nBz8jYf,env=THIRD_JWT_SECRET"`
```

with:

```go
ThirdJwtSecret string `json:",optional,env=THIRD_JWT_SECRET"`
```

- [ ] **Step 5: 运行净化门禁**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
test ! -e backend/core/service/apisvr/.env
test -z "$(rg -nP '^[ ]*(DSN|AccessSecret|AccessKey|AccessKeyID|AccessKeySecret|AppKey|AppSecret|Password|SecondSecret|ThirdJwtSecret|Secret|AccessToken):[ ]+(?!\$\{|""|$)' backend/core/service --glob '*.yaml' --glob '*.yml')"
test -z "$(rg -n 'default=.*ThirdJwtSecret|ThirdJwtSecret.*default=' backend/core/service/syssvr/internal/config/config.go)"
test "$(git -C ../core rev-parse HEAD)" = "1b3110e5e17d5a0066d38930b67bc44fdcc3c6fc"
test -z "$(git -C ../core status --porcelain)"
```

Expected: 全部 exit 0。

- [ ] **Step 6: 验证迁入 module 仍可编译**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/core
GOWORK=off go test -run '^$' ./...
```

Expected: exit 0；目标继续包含源 module 的 236 个 Go package，包括 `core/tools` 下两个辅助 main package。

- [ ] **Step 7: 确认净化完成，暂不提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git diff --check -- backend/core
test -z "$(git diff --cached --name-only)"
```

继续执行 Task 3，在修正上游 README 的许可证误述并加入 NOTICE 后一次提交，避免历史中出现“已迁入但许可证声明矛盾”的中间提交。

### Task 3：修正许可证和上游来源声明

**Files:**

- Create: `backend/core/NOTICE`
- Modify: `backend/core/README.md`
- Modify: `backend/core/README.en.md`
- Verify: `backend/core/LICENSE`

- [ ] **Step 1: 先证明 README 与 LICENSE 不一致**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
rg -n 'Apache License 2.0' backend/core/README.md backend/core/README.en.md
sha256sum backend/core/LICENSE
```

Expected: README 两处声称 Apache-2.0；LICENSE checksum 为 `20b067f86de375aae6db0f283ab2e65de24d537733b89bd58432c101259d84cf`，正文是 GNU AGPL-3.0。

- [ ] **Step 2: 创建 NOTICE**

Create `backend/core/NOTICE` with:

```text
ming-iot Core

This component is derived from UnitedRhino Core:
https://gitee.com/unitedrhino/core

Upstream baseline:
1b3110e5e17d5a0066d38930b67bc44fdcc3c6fc

The upstream component is licensed under the GNU Affero General Public
License version 3. This repository contains modifications. See LICENSE for
the complete license text.
```

- [ ] **Step 3: 修正中英文 README**

将两份 README 的 Apache-2.0 句子改成 GNU AGPL-3.0，并在许可证段落增加“本目录基于上述 upstream commit 修改”的来源说明；链接继续指向本目录 `LICENSE`。

- [ ] **Step 4: 验证并提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
test -z "$(rg -n 'Apache License' backend/core/README*.md)"
rg -n 'AGPL-3.0|GNU Affero' backend/core/README.md backend/core/README.en.md backend/core/NOTICE
test "$(sha256sum backend/core/LICENSE | awk '{print $1}')" = "20b067f86de375aae6db0f283ab2e65de24d537733b89bd58432c101259d84cf"
git add backend/core
git diff --cached --check
git commit -m "迁移并声明 Core 源码基线"
```

### Task 4：建立 Go Workspace 和目标仓库忽略规则

**Files:**

- Create: `backend/go.work`
- Create: `.gitignore`

- [ ] **Step 1: 创建最小 Go Workspace**

Create `backend/go.work`:

```go
go 1.24.4

use ./core
```

- [ ] **Step 2: 创建目标仓库忽略规则**

Create `.gitignore`:

```gitignore
/backend/bin/
/backend/.coretxn/
/backend/.goctl-patch-work/
**/.env
**/*.local.yaml
**/*.local.yml
```

不要复制源 `.gitignore` 中忽略 `/go.sum`、旧 `/cmd/*` 或 Java/Node 构建目录的规则；目标必须跟踪 `backend/core/go.sum`。

- [ ] **Step 3: 验证 Workspace 和 ignore 行为**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend
GOWORK="$PWD/go.work" go list -m -f '{{.Path}} {{.Dir}}'

cd ..
git check-ignore -q backend/bin/probe
git check-ignore -q backend/.coretxn/probe
git check-ignore -q backend/.goctl-patch-work/probe
git check-ignore -q backend/core/service/apisvr/.env
test -z "$(git check-ignore backend/core/go.sum)"
```

Expected: module 列表只有 `gitee.com/unitedrhino/core`；二进制、生成事务目录、goctl 补丁工作目录和敏感配置被忽略；`go.sum` 不被忽略。

- [ ] **Step 4: 提交 Workspace**

```bash
git add .gitignore backend/go.work backend/core/go.sum
git diff --cached --check
git commit -m "建立后端 Go Workspace"
```

### Task 5：重建五服务安全构建入口

**Files:**

- Create: `backend/core/Makefile`

- [ ] **Step 1: 写入 Makefile 行为测试脚本并观察失败**

在 Makefile 尚不存在时运行：

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/core
make -n build
```

Expected: `No rule to make target` 或找不到 Makefile，证明目标尚未实现。

- [ ] **Step 2: 创建不修改依赖、不宽目录删除的 Makefile**

Create `backend/core/Makefile`:

```make
BACKEND_DIR := $(abspath ..)
BIN_DIR := $(BACKEND_DIR)/bin
GO_BUILD := go build -tags no_k8s

.PHONY: build build.api build.sys build.data build.timedjob build.timedscheduler stage.config clean

build: build.api build.sys build.data build.timedjob build.timedscheduler

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build.api: | $(BIN_DIR)
	$(GO_BUILD) -o $(BIN_DIR)/coresvr ./service/apisvr

build.sys: | $(BIN_DIR)
	$(GO_BUILD) -o $(BIN_DIR)/syssvr ./service/syssvr

build.data: | $(BIN_DIR)
	$(GO_BUILD) -o $(BIN_DIR)/datasvr ./service/datasvr

build.timedjob: | $(BIN_DIR)
	$(GO_BUILD) -o $(BIN_DIR)/timedjobsvr ./service/timed/timedjobsvr

build.timedscheduler: | $(BIN_DIR)
	$(GO_BUILD) -o $(BIN_DIR)/timedschedulersvr ./service/timed/timedschedulersvr

stage.config: | $(BIN_DIR)
	mkdir -p $(BIN_DIR)/etc
	cp -R service/apisvr/etc/. $(BIN_DIR)/etc/

clean:
	rm -f $(BIN_DIR)/coresvr \
		$(BIN_DIR)/syssvr \
		$(BIN_DIR)/datasvr \
		$(BIN_DIR)/timedjobsvr \
		$(BIN_DIR)/timedschedulersvr
```

`build` 不依赖 `go mod tidy`，`clean` 只删除五个已解析的文件，且没有 killall、nohup 或远端 rsync 副作用。

API main 位于 `service/apisvr`，但现有 Makefile 与容器部署产物名是 `coresvr`。兼容迁移继续输出 `coresvr`；不在本计划中顺带改部署制品名。旧 `run.api` 期待 `apisvr` 的矛盾通过删除有副作用的旧 run targets 消除，后续若改名需单独 ADR。

- [ ] **Step 3: 构建五个入口**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/core
make build
for name in coresvr syssvr datasvr timedjobsvr timedschedulersvr; do
  test -x "../bin/$name"
done
git diff --exit-code -- go.mod go.sum
go test -run '^$' ./...
```

Expected: 五个可执行文件存在，module 文件无变化，只编译检查通过。

- [ ] **Step 4: 验证产物不进入 Git 并提交**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git check-ignore -q backend/bin/coresvr
test -z "$(git status --short backend/bin)"
git add backend/core/Makefile
git diff --cached --check
git commit -m "重建 Core 五服务构建入口"
```

### Task 6：阶段验收

- [ ] **Step 1: 从 Workspace 和 module 两层验证**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend
go list -m all >/dev/null

cd core
go test -run '^$' ./...
make build
```

- [ ] **Step 2: 重跑安全与来源门禁**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
test ! -e backend/core/service/apisvr/.env
test -z "$(git ls-files 'backend/bin/**' '**/.env')"
test -z "$(rg -nP '^[ ]*(DSN|AccessSecret|AccessKey|AccessKeyID|AccessKeySecret|AppKey|AppSecret|Password|SecondSecret|ThirdJwtSecret|Secret|AccessToken):[ ]+(?!\$\{|""|$)' backend/core/service --glob '*.yaml' --glob '*.yml')"
test -z "$(git -C ../core status --porcelain)"
test "$(git -C ../core rev-parse HEAD)" = "1b3110e5e17d5a0066d38930b67bc44fdcc3c6fc"
git diff --check
git status --short --branch
```

Expected: 所有门禁通过，目标 worktree 无未解释改动。此时才进入 proto 拆分计划。
