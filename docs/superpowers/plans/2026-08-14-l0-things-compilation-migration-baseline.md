# L0 Things 可编译迁移基线 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从联犀 Things 固定 Git 对象确定性生成 `ming-iot/backend/things`，完成净化、module 映射、三入口构建与可重复证据闭环。

**Architecture:** 项目级 `thingsbaseline` 工具读取 YAML lock，通过 `git ls-tree/cat-file/archive` 从固定 commit 构建 backend 直属候选目录；候选内完成前缀转换、安全净化、目标模板生成和 `go mod tidy`，全部门禁通过后才发布。`verify` 重新渲染候选并逐路径比较目标，同时运行 Go/构建 gate、输出 JSON evidence。

**Tech Stack:** Go 1.24.4 module、Go Workspace、Git plumbing、YAML v3、SHA-256、Make、AGPL-3.0。

---

## 执行约束

- 规格：`docs/superpowers/specs/2026-08-14-l0-things-compilation-migration-baseline-design.md`。
- 源仓库始终只读：`/home/wangjunbo/workspace-iot-platform/things`。
- 固定 commit：`243ab1aa03b8a3f106761a81cdd9bd8827d448c8`。
- 实施 worktree：`/home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/l0-things-migration-baseline`。
- 不执行 `git add`、`git commit` 或 `git push`。每个 Task 结束只运行 `git status` 和 `git diff --check`，等待最终人工审核。
- 任何测试失败先按根因分析处理，不扩大 L0 架构范围。

## 文件结构

手工创建或修改：

```text
.gitignore
backend/go.work
backend/go.work.sum
backend/saas/go.mod
backend/saas/go.sum
backend/tools/README.md
backend/tools/go.mod
backend/tools/go.sum
backend/tools/things-baseline.lock
backend/tools/cmd/thingsbaseline/{main.go,main_test.go}
backend/tools/internal/thingsbaseline/{contract,source,transform,render,verify,evidence}.go
backend/tools/internal/thingsbaseline/{contract,source,transform,render,verify,evidence}_test.go
backend/tools/templates/things-baseline/{README.md,NOTICE,PROVENANCE.yaml,Makefile}
docs/superpowers/evidence/2026-08-14-l0-things-source-baseline.md
docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.{json,md}
```

由工具生成、禁止人工逐文件编辑：

```text
backend/things/LICENSE
backend/things/go.mod
backend/things/go.sum
backend/things/sdk/protocol/**
backend/things/service/apisvr/**
backend/things/service/dgsvr/**
backend/things/service/dmsvr/**
backend/things/share/**
backend/things/{README.md,NOTICE,PROVENANCE.yaml,Makefile}
```

### Task 1：冻结 lock 与源读取契约

**Files:**

- Create: `backend/tools/things-baseline.lock`
- Create: `backend/tools/internal/thingsbaseline/contract.go`
- Create: `backend/tools/internal/thingsbaseline/contract_test.go`
- Create: `backend/tools/internal/thingsbaseline/source.go`
- Create: `backend/tools/internal/thingsbaseline/source_test.go`
- Modify: `backend/tools/go.mod`
- Modify: `backend/tools/go.sum`

- [x] **Step 1: 写 lock 失败测试**

`contract_test.go` 建立 table tests，拒绝：commit 非 40 位、ref 不是 `refs/remotes/`、include 有 `..`、include/exclude 重叠、计数为零、target module 仍为 UnitedRhino、source/target module 相同。

核心测试：

```go
func TestValidateLockRejectsUnsafeContracts(t *testing.T) {
    base := validLockForTest()
    tests := []struct {
        name   string
        mutate func(*Lock)
        want   string
    }{
        {"short commit", func(l *Lock) { l.Source.Commit = "243ab1aa" }, "40 lowercase hex"},
        {"moving branch", func(l *Lock) { l.Source.Ref = "master" }, "refs/remotes/"},
        {"path traversal", func(l *Lock) { l.Source.Include = append(l.Source.Include, "../secrets") }, "unsafe include"},
        {"empty count", func(l *Lock) { l.Source.SelectedFiles = 0 }, "selectedFiles"},
        {"same module", func(l *Lock) { l.Target.Module = l.Source.Module }, "target module"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := base.clone()
            tt.mutate(&got)
            err := ValidateLock(&got)
            if err == nil || !strings.Contains(err.Error(), tt.want) {
                t.Fatalf("ValidateLock() error = %v, want substring %q", err, tt.want)
            }
        })
    }
}
```

- [x] **Step 2: 运行测试并确认失败**

Run:

```bash
cd backend/tools
go test ./internal/thingsbaseline -run TestValidateLockRejectsUnsafeContracts -v
```

Expected: FAIL，package/类型尚不存在。

- [x] **Step 3: 创建精确 lock**

`backend/tools/things-baseline.lock` 写入：

```yaml
schemaVersion: 1
loopId: L0
source:
  repository: https://gitee.com/unitedrhino/things
  ref: refs/remotes/origin/master
  commit: 243ab1aa03b8a3f106761a81cdd9bd8827d448c8
  tree: 544b638ae3ee5e5f3a041bea22ca035dbb7e5859
  module: gitee.com/unitedrhino/things
  trackedFiles: 1183
  selectedFiles: 976
  selectedBlobBytes: 4866935
  archiveBytes: 5754880
  archiveSha256: 32b998e4621dd88a39b618552187cfbd1be910851925702a07c9e4c82cb4d486
  include: [LICENSE, go.mod, go.sum, sdk/protocol, service/apisvr, service/dgsvr, service/dmsvr, share]
  exclude: [.github, .gitignore, AGENTS.md, CLAUDE.md, Dockerfile, Makefile, README.md, README.en.md, deploy, doc, script, shell, tests, thingsEEsvr-linux-amd64, tools]
target:
  directory: things
  module: gitee.com/ming-iot/things
  saasModule: gitee.com/ming-iot/saas
  saasRelativePath: ../saas
  shareModule: gitee.com/unitedrhino/share
  shareVersion: v1.6.4
transforms:
  corePrefixCount: 94
  thingsPrefixCount: 2726
  credentialLineCount: 55
  credentialFileCount: 9
  credentialActiveLineCount: 35
  credentialCommentedLineCount: 20
  credentialPaths:
    databaseDSN: 10
    tsdbDSN: 18
    ossAccessKeyID: 3
    ossAccessKeySecret: 3
    authWhiteUsersPassword: 11
    devLinkOpenApiApiKey: 1
    devLinkOpenApiSecretKey: 1
    devLinkMqttOpenApiApiKey: 4
    devLinkMqttOpenApiSecretKey: 4
  privateKeyBlockCount: 1
  vetFixCount: 1
  privateKeySourceSha256: eea7f05a0fbfa2f55ce966f8f297ec8bdc2b6c0fc5aa2d63520d09c03f0233a3
  vetSourceSha256: 828fb556f1a9de66944cdeef5fa0aa1012409a12618396b1e79fed1a55e3c72a
contracts: {apiFiles: 34, protoFiles: 4, generatedProtoFiles: 7, testFiles: 23}
security: {maxFileBytes: 10485760}
```

- [x] **Step 4: 实现 lock 类型、加载和校验**

`contract.go` 定义 lock 映射类型。`LoadLock` 使用 `yaml.v3` 的 `KnownFields(true)` 拒绝未知字段；`ValidateLock` 按 Git slash path 语义使用 `path.Clean`，拒绝绝对路径、`.`、`..`、控制字符、重复和重叠根，并完整拒绝非法 refname。`gopkg.in/yaml.v3 v3.0.1` 从 indirect 改为 direct，不升级其他 tools 依赖。

- [x] **Step 5: 写并实现源 Git 集成测试**

`source_test.go` 在 `t.TempDir()` 创建最小 Git 仓库并验证：固定 commit 读取、ref 漂移失败、replace object 不影响固定对象、symlink mode `120000` 失败、路径与畸形 NUL 记录拒绝、单文件上限、全树文件数、逐 blob object ID、路径排序稳定、读取前后 HEAD/ref/status digest 不变。测试 Git 命令隔离 system/global config、签名和 hooks。

`source.go` 只通过参数数组调用：

```text
git show-ref --verify --hash refs/remotes/origin/master
git rev-parse 243ab1aa03b8a3f106761a81cdd9bd8827d448c8^{tree}
git rev-parse HEAD
git status --porcelain=v1 -z --untracked-files=all --ignore-submodules=none
git ls-tree -rz -r --full-tree --name-only 243ab1aa03b8a3f106761a81cdd9bd8827d448c8
git ls-tree -rz -l 243ab1aa03b8a3f106761a81cdd9bd8827d448c8 -- LICENSE go.mod go.sum sdk/protocol service/apisvr service/dgsvr service/dmsvr share
git cat-file blob（对上一条命令返回的每个已验证 blob object ID 分别调用）
git archive --format=tar 243ab1aa03b8a3f106761a81cdd9bd8827d448c8 -- LICENSE go.mod go.sum sdk/protocol service/apisvr service/dgsvr service/dmsvr share
```

所有生产 Git 命令丢弃继承的全部 `GIT_*` 变量，只按固定顺序加入 `GIT_NO_REPLACE_OBJECTS=1`、`GIT_CONFIG_NOSYSTEM=1`、`GIT_CONFIG_GLOBAL=os.DevNull`、`GIT_TERMINAL_PROMPT=0`、`GIT_OPTIONAL_LOCKS=0`、`GIT_LITERAL_PATHSPECS=1`、`GIT_ATTR_NOSYSTEM=1`，并覆盖 `LC_ALL=C`、`LANG=C`；保留所有非 Git 环境且不修改 `HOME`。stale-index 测试必须证明 `ReadSource` 前后 `.git/index` byte-identical，hostile-env 测试必须证明 repository/index/object/config 注入无效。只接受 mode `100644/100755`；实际 Git 路径必须重新通过安全校验。读取每个文件前按 `ls-tree` size 执行 10 MiB 上限，读取后按 `blob <size>\0<data>` 重算 SHA-1 并匹配 object ID；archive stdout 流式计算 bytes/SHA-256。全树文件数、选中文件数、blob bytes 和 archive bytes/SHA-256 必须分别匹配 lock。

- [x] **Step 6: 运行 focused tests 和检查点**

Run:

```bash
cd backend/tools
go test ./internal/thingsbaseline -run 'Test(Validate|Load|ReadSource)' -v
cd ../..
git diff --check
git status --short
```

Expected: focused tests PASS；仅出现本 Task 预期文件。

### Task 2：实现 module、安全和 vet 确定性转换

**Files:**

- Create: `backend/tools/internal/thingsbaseline/transform.go`
- Create: `backend/tools/internal/thingsbaseline/transform_test.go`
- Modify: `backend/tools/internal/thingsbaseline/contract.go`
- Modify: `backend/tools/internal/thingsbaseline/contract_test.go`
- Modify: `backend/tools/things-baseline.lock`
- Modify: `backend/tools/go.mod`
- Modify: `backend/tools/go.sum`

- [x] **Step 1: 写 module 转换失败测试**

测试输入覆盖 Go import、go.mod require、Markdown 引用和 go.sum。断言：转换前的非 go.sum 原始 blob 中 Core/Things 前缀按 lock 精确为 94/2726；Share 不做全局改写；go.sum byte-identical。`go.mod` golden 必须恰好删除顶部两条注释 Core/Share 本地路径提示，保留两条有效 `cloud.google.com/go` 版本 replace，并在同一 replace block 中只增加一个相对 SaaS replace。被删除提示中的 Core 命中仍计入 94；不得把正常的上游版本钉住误删为“本地路径 replace”。命中数少/多、重复 SaaS replace或 cloud replace 被改动均失败。

- [x] **Step 2: 写配置净化失败测试**

以虚构值 `literal-db-value`、`literal-key-value`、`literal-password-value` 构造 YAML，不复制上游凭据。lock 以具名 struct 字段冻结 9 文件、35 active、20 commented、总计 55 及九条路径分布，三种求和必须一致。table test 覆盖 active/commented、完整 mapping/sequence 栈和 `Users[]` 规范化；DSN、ApiKey、SecretKey、AccessKeyID、AccessKeySecret、Password 每类至少构造 active/commented 错误父路径，空值也不得绕过，错误不得泄漏值。

active YAML 由 `yaml.v3` AST 发现全部敏感 mapping key，再与可安全保持格式的 block lexer 逐项交叉；quoted key、flow mapping/sequence、alias 或 AST/lexer 不一致全部 fail closed。commented example 使用独立保守扫描，quoted/flow/TAB 等无法安全重写的敏感形式同样拒绝。输出再以 AST + scanner 独立反向验证精确 placeholder 和完整 profile；“55 个合法项 + 1 个绕过项”必须失败。

预期映射精确为：

```go
var credentialEnv = map[string]string{
    "Database.DSN":              "${THINGS_DB_DSN}",
    "TSDB.DSN":                  "${THINGS_TSDB_DSN}",
    "DevLink.OpenApi.ApiKey":         "${THINGS_EMQX_API_KEY}",
    "DevLink.OpenApi.SecretKey":      "${THINGS_EMQX_SECRET_KEY}",
    "DevLink.Mqtt.OpenApi.ApiKey":    "${THINGS_EMQX_API_KEY}",
    "DevLink.Mqtt.OpenApi.SecretKey": "${THINGS_EMQX_SECRET_KEY}",
    "OssConf.AccessKeyID":            "${THINGS_OSS_ACCESS_KEY_ID}",
    "OssConf.AccessKeySecret":        "${THINGS_OSS_ACCESS_KEY_SECRET}",
    "AuthWhite.Users[].Password":     "${THINGS_INTERNAL_AUTH_PASSWORD}",
}
```

叶子 key 相同但完整语义路径不在上述 allowlist 时必须失败，不能按字段名或 `OpenApi.*` 后缀做宽泛替换。替换后重新扫描并验证每个目标占位符、完整路径分布和 active/commented 数量。

- [x] **Step 3: 写私钥边界和 vet 失败测试**

测试通过字符串拼接构造 PEM marker，私钥正文只用 `fixture-body`。断言：source SHA 不符、零块、多块和边界不完整均拒绝；恰好一个 key block 被移除；`crypto/tls` 改为 `encoding/pem`；公有证书 raw literal payload 逐字节保留；输出/日志不含 fixture body。把 stdlib-only transformed fixture 写入 `t.TempDir` module 并真实执行 `go test`，覆盖 PEM decode nil、x509 parse error、非 nil cert + Certificate，以及注册、身份一致性、cache、password、成功分支，证明只跳过诊断且不改变业务决策。vet fixture 断言 SHA 错、零命中、多命中和 old1+preexisting-new1 失败，成功时旧 0、新 1。两个 fixture 都含旧 module 前缀，以证明先验原始 SHA、再完成 module 改写。

- [x] **Step 4: 运行测试并确认失败**

Run:

```bash
cd backend/tools
go test ./internal/thingsbaseline -run 'TestTransform|TestSanitize' -v
```

Expected: FAIL，转换函数尚不存在。

- [x] **Step 5: 实现转换器**

`transform.go` 提供固定 API：

```go
type RenderedFile struct {
    Mode string
    Data []byte
}

type TransformReport struct {
    CorePrefixes             int `json:"corePrefixCount"`
    ThingsPrefixes           int `json:"thingsPrefixCount"`
    CredentialLines          int `json:"credentialLineCount"`
    PrivateKeyBlocks         int `json:"privateKeyBlockCount"`
    VetFixes                 int `json:"vetFixCount"`
    SchemaFixtureIdentifiers int `json:"schemaFixtureIdentifierCount"`
}

func TransformTree(files map[string]SourceFile, lock *Lock) (map[string]RenderedFile, TransformReport, error)
func SanitizeCredentialYAML(path string, src []byte) ([]byte, int, error)
func RemoveEmbeddedPrivateKey(src []byte, expectedSHA string) ([]byte, int, error)
func FixDynamicFormat(src []byte, expectedSHA string) ([]byte, int, error)
func FixSchemaFixtureIdentifiers(src []byte, expectedSourceSHA, expectedOutputSHA string, expectedCount int) ([]byte, int, error)
```

`TransformTree` 的 map key 是唯一规范路径，必须等于输入 `SourceFile.Path`；`RenderedFile.Mode` 继续使用经 Task 1 验证的 Git mode `100644/100755`。输入和输出 `Data` 不得共享可变 backing array，任何失败都不得修改调用者传入的 map 或 byte slice。

实现顺序和限制：

1. 先从输入 map 取得三个 source-specific 文件的原始 `SourceFile.Data`，在任何 module/YAML 改写前验证全部三个 lock SHA；验证失败不产生部分输出。第三个固定测试路径是 `share/domain/schema/schema_test.go`；
2. 对私钥/vet 两个特殊文件在原始 blob 上完成定点转换；private-key marker 由字符串片段运行时拼出，不在工具源码留下连续 marker；删除后用 `go/format.Source`，命中必须为 1。对 `share/domain/schema/schema_test.go` 的 fixture 仅执行精确 token/count/output-SHA 的 migration-only 校正，生产 model 不修改；
3. 对所有非 `go.sum` 原始 blob 先统计 Core/Things 源命中，必须分别为 94/2726，再对三个定点转换结果执行 module 替换；`go.sum` 留给 tidy 且 byte-identical；
4. `go.mod` 删除恰好两条旧注释本地路径提示、替换 module 后用 `golang.org/x/mod/modfile` 解析有效 directives；新增前有效 replace 必须恰好是两条固定 cloud path+version 且无 SaaS/额外/重复项，再结构化增加唯一 SaaS 相对 replace；注释诱饵不得满足门禁；
5. 只对 YAML 做凭据处理；active AST 与 block lexer 交叉，commented 使用独立保守扫描；保留原缩进及 `#`，lock 中 9/35/20/55 和九路径分布必须在前后完全一致；
6. 净化后反向验证私钥块旧 0、vet 旧 0/新 1、fixture 旧 token `"id":` 为 0/新 token `"identifier":` 为 45、凭据只含目标占位符、旧 module 前缀除尚未 tidy 的 `go.sum` 外为 0；
7. 只处理有效 UTF-8；错误包含路径/行号/语义路径但不打印敏感值。

- [x] **Step 6: 运行转换测试和静态扫描**

Run:

```bash
cd backend/tools
go test ./internal/thingsbaseline -run 'TestTransform|TestSanitize' -v
test -z "$(rg -n 'BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY' internal/thingsbaseline)"
```

Expected: PASS；工具源码无连续私钥 marker。

### Task 3：实现候选渲染、目标模板与安全门禁

**Files:**

- Modify: `.gitignore`
- Create: `backend/tools/internal/thingsbaseline/render.go`
- Create: `backend/tools/internal/thingsbaseline/render_test.go`
- Create: `backend/tools/templates/things-baseline/README.md`
- Create: `backend/tools/templates/things-baseline/NOTICE`
- Create: `backend/tools/templates/things-baseline/PROVENANCE.yaml`
- Create: `backend/tools/templates/things-baseline/Makefile`
- Create: `backend/tools/templates/things-baseline/embed.go`

`.gitignore` 增加 `/.worktrees/` 与 `/backend/.thingsbaseline-*/`；现有 `/backend/bin/` 保持不变。候选和构建产物不得进入证据分支。

- [x] **Step 1: 写候选安全测试**

使用 fake `SourceSnapshot` 和临时 backend，先证明 renderer 在内存完成 TransformTree 后才创建/写 candidate，原始凭据和私钥从未落盘。candidate 只能由 `os.MkdirTemp(realBackend, ".thingsbaseline-")` 创建；backend 外、嵌套路径及 backend/candidate/target symlink 全部拒绝。初始目录固定 `0700`，exclusive create 后归一为 dirs `0755`、files `0644/0755`，umask 不影响。

发布通过同一已打开 backend fd 调用 `renameat2(RENAME_NOREPLACE)`；target 为文件、symlink、空/非空目录或在 precheck 后并发创建均拒绝且原内容不变，`EXDEV` 禁止 copy fallback。source/transform/template/tidy/scan/digest/publish 各阶段注入失败后：原本不存在的 target 仍不存在、candidate 被清理；预存 target byte-identical。

scanner table 覆盖大小写不敏感的危险扩展、`.env/.env.*`、ELF magic、先 size 后 read、私钥 marker、完整凭据语义、旧 module、symlink/hardlink/FIFO/socket/device、顶层 exclude 与嵌套同名、四模板精确例外、97 行 trailing-whitespace 指纹不得新增。candidate 和 target 必须复用同一 scanner。

- [x] **Step 2: 创建安全模板**

`embed.go` 用只读 `embed.FS` 精确嵌入四个模板，使 renderer 不依赖 cwd。模板只允许固定 token，未知/未替换 token 失败；最终逐字节 golden 且 mode 均为 `0644`。

README 固定模块用途、三个入口、先进入 `backend/things` 后执行的构建/验证命令、L0 兼容基线和禁止直接生产部署。NOTICE 固定来源 URL、完整 commit、AGPL-3.0 且不得声明 Apache。PROVENANCE 使用稳定字段顺序/schema，固定 repository/ref/commit/tree、source/target module、Share v1.6.4、license、976/980 文件数和六类转换计数；禁止时间戳、绝对路径、主机/candidate 信息。

Makefile 内容固定为：

```make
BACKEND_DIR := $(abspath ..)
BIN_DIR := $(BACKEND_DIR)/bin
GO_BUILD := GOWORK=off go build

.PHONY: build build.api build.dg build.dm clean

build: build.api build.dg build.dm

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build.api: | $(BIN_DIR)
	$(GO_BUILD) -o $(BIN_DIR)/thingsvr ./service/apisvr

build.dg: | $(BIN_DIR)
	$(GO_BUILD) -o $(BIN_DIR)/dgsvr ./service/dgsvr

build.dm: | $(BIN_DIR)
	$(GO_BUILD) -o $(BIN_DIR)/dmsvr ./service/dmsvr

clean:
	rm -f $(BIN_DIR)/thingsvr $(BIN_DIR)/dgsvr $(BIN_DIR)/dmsvr
```

- [x] **Step 3: 实现 renderer**

冻结 API/所有权：`RenderCandidate` 失败自行清理，成功返回 opaque handle；handle 的 `Cleanup` 幂等；`Publish` 仅执行原子 no-replace rename，成功后 disarm。verify 调用方始终 defer cleanup，import 发布失败也 cleanup。最终 scan/digest 后 handle 进入 sealed 状态，不允许再写。

`render.go` 固定执行：校验 lock/snapshot/backend 物理边界 → **在内存执行 TransformTree** → 创建 backend 直属私有 candidate → 按排序路径 exclusive 写 976 个净化文件 → 从 embed.FS 渲染四模板 → 记录 980-path pre-tidy manifest → 受控 `go mod tidy` → `go mod verify` → 确认只改 `go.mod/go.sum` 且仍为 980 paths → 权限归一 → 安全扫描 → 计算稳定 tree digest → seal handle。未经净化的 SourceFile.Data 绝不写磁盘。

opaque handle 必须提供 `Report() CandidateReport`，返回深拷贝的只读结果：pre/final manifest、tree digest、`TransformReport`、完整 credential profile、scanner 结果、固定 Go 版本及 tidy/verify 结果；报告不得包含 backend/candidate/cache 的本地绝对路径或随机目录名。handle 不提供 path/write API；仅同 package 的 Task 4 verifier 可使用内部 sealed root 做隔离验证，CLI/调用方只能读取报告、`Cleanup` 或 `Publish`。

权威 tidy 固定 `go1.25.4`；每次创建独立私有 `HOME/GOCACHE/GOMODCACHE`，丢弃继承的 `GO*`/`GIT_*` 后固定 `GOENV=off`、空 `GOFLAGS`、`GOWORK=off`、`GOTOOLCHAIN=local`、`GOPROXY=https://goproxy.cn,direct`、`GOSUMDB=sum.golang.org`，并清空 private/no-proxy/no-sumdb/insecure。fake runner 断言 cwd/args/env；网络/checksum/cancel/版本失败均清理，stderr 有界并清除 URL userinfo/环境值。两个独立 cache 渲染必须得到相同 manifest/digest。

最终 manifest 恰好 980 个 regular files。tree digest 对排序后的规范 path、最终 mode、content SHA-256 做长度前缀 framing，不含 candidate 名、mtime、uid/gid。LICENSE SHA-256 必须为 `20b067f86de375aae6db0f283ab2e65de24d537733b89bd58432c101259d84cf`。旧 Things 前缀只允许在 byte-golden 模板的固定来源字段；旧 Core 全树为零。固定源的 97 行 trailing whitespace 以 path/line/line-SHA 指纹保留，禁止任何新增，模板必须零命中。

- [x] **Step 4: 运行 renderer tests**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline -run 'TestRender|TestCandidate|TestSecurity' -v
```

Expected: PASS。

### Task 4：实现双渲染 verify、稳定 evidence 与 CLI

**Files:**

- Modify: `backend/tools/internal/thingsbaseline/source.go`
- Modify: `backend/tools/internal/thingsbaseline/source_test.go`
- Modify: `backend/tools/internal/thingsbaseline/render.go`
- Modify: `backend/tools/internal/thingsbaseline/render_test.go`
- Modify: `backend/tools/internal/thingsbaseline/scan.go`
- Modify: `backend/tools/internal/thingsbaseline/scan_test.go`
- Create: `backend/tools/internal/thingsbaseline/verify.go`
- Create: `backend/tools/internal/thingsbaseline/verify_test.go`
- Create: `backend/tools/internal/thingsbaseline/evidence.go`
- Create: `backend/tools/internal/thingsbaseline/evidence_test.go`
- Create: `backend/tools/cmd/thingsbaseline/main.go`
- Create: `backend/tools/cmd/thingsbaseline/main_test.go`
- Modify: `backend/tools/README.md`

- [x] **Step 1: 写 source final-state 与候选意图失败测试**

`ReadSource` 返回不含路径/正文的 `SourceRepositoryState{Head, Ref, StatusSHA256}`；新增复用同一 hostile-env Git plumbing 的 `CaptureSourceRepositoryState`，覆盖 HEAD/ref/status 漂移和 context cancel。verification candidate 只在既有 target 为 backend 直属真实目录时成功，且 `Publish()` 永久失败；publish candidate 继续要求 target 不存在。两种 candidate 的失败都不得改变 target。

- [x] **Step 2: 写 fd-relative target 比较失败测试**

覆盖相同树 PASS、缺文件、多文件、mode 变化、内容变化、差异排序与固定上限；差异只包含 path、expected/actual mode/SHA-256，不输出正文。target root/子树 symlink、hardlink、FIFO、socket、device、扫描期间 file replace/chmod/directory exchange/entry mutation全部失败。扫描前后 target manifest/digest 必须一致。

- [x] **Step 3: 写双渲染、isolation tree 与 gate 失败测试**

fake renderer/runner 证明：verify 必须执行两次独立 HOME/cache render；任一产物 report 字段或受控命令名称/退出码差异都失败，独立 cache 的下载日志 hash/tail 仅各自留证、不参与产物可复现判定；target 只被只读扫描，从第二个 sealed candidate fd-relative copy 出 isolation tree 后才执行 Things/Workspace/build。任一 gate 失败后其余 gate 固定为 `NOT_RUN`；`module-clean` 无条件比较 backend go.work/go.work.sum 与 saas/tools/things go.mod/go.sum；结束复用 Task 1 API 比较 source state。

- [x] **Step 4: 写 evidence 与安全落盘失败测试**

固定 clock，断言 schema/lower-camel 字段/排序稳定，空集合必须为 `[]`；单调耗时推导 `finishedAt`，序列化时间差必须等于 `durationMs`。result 只允许 `PASS|FAIL|NOT_RUN`，execution 区分 `EXITED|SPAWN_ERROR|TIMEOUT|CANCELLED|NOT_RUN`，无进程退出时 `exitCode=null`。日志只存完整 SHA-256 与有界脱敏 tail。拒绝 evidence 位于 source/backend/target/candidate/cache 内、symlink parent/target 或特殊文件；同目录 exclusive temp + fsync + atomic rename，JSON 不含本机绝对路径。`--structural-only` 必须是 `finalEvidence=false/NOT_FINAL`。

- [x] **Step 5: 写 CLI 参数和退出码失败测试**

CLI 暴露 `run(ctx, args, stdout, stderr) int`。`import`/`verify` 缺必需 flag、重复/未知 flag、多余 positional argument 或未知子命令均 exit 2；运行失败 exit 1，成功 exit 0；cancel 能传入 renderer/gate runner。帮助信息不得把本机绝对路径写成默认值。

- [x] **Step 6: 运行 RED tests**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline ./cmd/thingsbaseline \
  -run 'Test(SourceRepositoryState|VerificationCandidate|CompareTarget|Verify|Evidence|CLI)' -count=1 -v
```

Expected: FAIL，新增 API/类型尚不存在或行为未满足。

- [x] **Step 7: 实现 source state、双候选意图和 target compare**

不复制 Git 命令：公开 state API 调用 Task 1 的 `readRepositoryState`。renderer 内部以明确 enum 打开 backend：publish 模式要求 target `ENOENT`，verify 模式要求 `S_IFDIR` 且 no-follow；verify handle 带不可逆 `publishAllowed=false`。target 通过 backend fd/openat 打开并复用 lenient inventory scanner；比较结果稳定、有界、无正文。

- [x] **Step 8: 实现 Verify、受控 gates 与 isolation tree**

固定顺序：source/read → render-1 → render-2 → reproducibility → target-before/compare → isolation-copy → tools-test → saas-test → things-test → workspace-test → things-build → module-clean → target-after/unchanged → source-unchanged。命令使用参数数组、显式 cwd、固定 Go binary、每轮新 HOME/GOCACHE/GOMODCACHE/GOTMPDIR、清空继承 GO*/GIT_*、固定 proxy/checksum/toolchain、单 gate timeout 和 process-group kill/wait。workspace 使用临时 go.work 显式绑定真实 saas/tools 与 isolation things，禁止解析到 target。

- [x] **Step 9: 实现 evidence、CLI 与 README**

README 写入准确 import/verify 用法；evidence 使用逻辑 target `backend/things`，同时保存两次 manifest/tree digest、凭据画像、scanner/tidy/verify、structural phases、gates 和 bounded differences。CLI import 在 publish 前再次检查 source state；verify 即使失败也尽量原子写入可安全落盘的 FAIL evidence，落盘失败本身 exit 1。

- [x] **Step 10: 运行 Task 4 全量门禁**

Run:

```bash
cd backend/tools
GOWORK=off go test ./...
GOWORK=off go test -race ./internal/thingsbaseline ./cmd/thingsbaseline
GOWORK=off go vet ./...
git diff --check
```

Expected: 所有既有和新增 tools tests PASS；target 尚未生成，未运行的首次 import/build 不写 PASS。

### Task 5：记录源基线并首次导入

**Files:**

- Create: `docs/superpowers/evidence/2026-08-14-l0-things-source-baseline.md`
- Generate: `backend/things/**`

- [x] **Step 1: 写源基线证据**

记录以下实测结果，不写私钥或字面量凭据：

```text
ref/commit/tree = fixed values
tracked files = 1183
selected files = 976
selected blob bytes = 4866935
archive bytes = 5754880
archive sha256 = 32b998e4621dd88a39b618552187cfbd1be910851925702a07c9e4c82cb4d486
local branch = master, ahead origin/master by 2
default vet baseline = FAIL at sendLogic.go dynamic AddDetailf
GOWORK=off go test -vet=off -run '^$' ./... = PASS
credential-shaped YAML lines = 55
embedded private-key blocks = 1
```

- [x] **Step 2: 运行首次 import**

Run:

```bash
cd backend/tools
test ! -e ../things
go run ./cmd/thingsbaseline import \
  --lock ./things-baseline.lock \
  --source /home/wangjunbo/workspace-iot-platform/things \
  --backend ..
```

Expected: `backend/things` 一次性出现；报告 source=976、目标 tree digest、转换计数 94/2726/55/1/1/45；源状态不变。

- [x] **Step 3: 立即运行 structural verify**

Run:

```bash
cd backend/tools
GOWORK=off go run ./cmd/thingsbaseline verify \
  --structural-only \
  --lock ./things-baseline.lock \
  --source /home/wangjunbo/workspace-iot-platform/things \
  --backend .. \
  --evidence /tmp/l0-things-structural.json
```

Expected: 重渲染与目标零差异；安全门禁 PASS。

### Task 6：接入 Workspace 并对齐 Share v1.6.4

**Files:**

- Modify: `.gitignore`
- Modify: `backend/go.work`
- Inspect: `backend/go.work.sum`
- Modify: `backend/saas/go.mod`
- Modify: `backend/saas/go.sum`

- [x] **Step 1: 更新忽略规则**

根 `.gitignore` 增加：

```gitignore
/.worktrees/
/backend/.thingsbaseline-*/
```

保留既有 `/backend/bin/` 和 secret/local config 规则。

- [x] **Step 2: 对齐 SaaS Share**

Run:

```bash
cd backend/saas
GOWORK=off go mod edit -require=gitee.com/unitedrhino/share@v1.6.4
GOWORK=off go mod tidy
GOWORK=off go test ./...
```

Expected: SaaS standalone PASS；除 Share v1.6.4 及 tidy 必需校验值外不主动升级其他依赖。

- [x] **Step 3: 更新 Go Workspace**

Run:

```bash
cd backend
go work use ./things
go list -m -f '{{.Path}} {{.Dir}}'
```

禁止运行 `go work sync`；它会把 Workspace 的 MVS 结果反向写入 SaaS/tools/生成目标。Expected: 精确包含 `gitee.com/ming-iot/saas`、`gitee.com/ming-iot/things`、`ming-iot/backend/tools` 三个本地 module，module 文件除明确的 Share 对齐外不变。

- [x] **Step 4: 检查 module 身份与旧前缀**

Run:

```bash
cd backend
test "$(cd things && GOWORK=off go list -m -f '{{.Path}}')" = "gitee.com/ming-iot/things"
test -z "$(rg -n 'gitee\.com/unitedrhino/(core|things)' things --glob '!README.md' --glob '!NOTICE' --glob '!PROVENANCE.yaml')"
test "$(rg -l 'gitee\.com/unitedrhino/(core|things)' things | sort)" = $'things/NOTICE\nthings/PROVENANCE.yaml\nthings/README.md'
rg -n 'gitee.com/unitedrhino/share v1.6.4' saas/go.mod things/go.mod
```

Expected: 全部通过；旧 Core/Things 前缀仅作为固定上游来源出现在三个模板文件中。

### Task 7：全量验证、重复证据和总控台

恢复状态记录：首次最终 verify 已实际到达 `things-test`，并以 exit 1 失败；原因是上游 `TestTempate` fixture 仍使用 45 个旧 `id` key，而当前 model 期待 `identifier`。失败证据暂保存在 canonical JSON，直至恢复后的 verify 成功重跑；本 Task 不因此标记通过。

**Files:**

- Create: `docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.json`
- Create: `docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.md`
- Create outside Git deliverable: `iot-doc/.superpowers/visual-live/content/rd-graph-l0-evidence-v3.html`

- [x] **Step 1: 运行最终 verify**

Run:

```bash
cd backend/tools
GOWORK=off go run ./cmd/thingsbaseline verify \
  --lock ./things-baseline.lock \
  --source /home/wangjunbo/workspace-iot-platform/things \
  --backend .. \
  --evidence ../../docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.json
```

Expected: structural、tools、saas、things、workspace、三入口 build、module-clean、source-unchanged 全部 PASS。

- [x] **Step 2: 人工复核三产物和 Git 忽略**

Run:

```bash
for name in thingsvr dgsvr dmsvr; do
  test -x "backend/bin/$name"
  git check-ignore -q "backend/bin/$name"
done
test -z "$(git ls-files 'backend/bin/**' '**/.env' '**/*.pem' '**/*.ppk' '**/*.key')"
```

Expected: 三产物可执行且未跟踪；敏感路径未跟踪。

- [x] **Step 3: 写 Markdown 证据解释**

Markdown 引用 JSON 的 run 时间、tree digest 和 gate 结果，并解释：源默认 vet 的既存失败及目标修复；55 行配置净化；私钥从未进入目标历史；NATS/旧物模型是 L0 有意保留；L1/L3 已解锁，L2 仍依赖 L1，Scene 仍锁定。

- [x] **Step 4: 更新 18081 总控台**

创建全新语义文件名的 dashboard page，不覆盖 V2。页面显示 L0 各 gate、evidence 路径、tree digest、自动化等级 A1，以及 L1/L3 解锁状态。

Run:

```bash
curl --noproxy '*' -fsS http://127.0.0.1:18081/ | \
  rg 'L0|Things 可编译迁移基线|A1|source-unchanged'
```

Expected: HTTP 页面包含全部 marker。

- [x] **Step 5: 最终范围与质量检查**

Run:

```bash
git diff --check
git status --short --branch
git -C /home/wangjunbo/workspace-iot-platform/things status --short --branch
git -C /home/wangjunbo/workspace-iot-platform/things rev-parse refs/remotes/origin/master
```

Expected:

- `ming-iot` 只包含本计划文件；
- `things` 仍是本地 master ahead 2，工作树无新增修改；
- official ref 仍为 `243ab1aa03b8a3f106761a81cdd9bd8827d448c8`；
- 无 staged 文件、无 commit、无 push。

## 完成定义

- [ ] Task 1–7 全部实际执行，未运行项不标 PASS。
- [ ] `backend/things` 由工具生成且第二次渲染零差异。
- [ ] 所有测试、三入口构建、安全和来源门禁通过。
- [ ] JSON/Markdown evidence 与 18081 总控台一致。
- [ ] 变更范围已列给用户审核，等待明确提交授权。
