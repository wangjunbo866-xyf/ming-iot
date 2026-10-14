# Things 可重复启动与依赖契约基线实施计划

> **面向 Agent 执行者：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans` 逐项实施。每个任务使用复选框跟踪。用户已于 2026-08-26 对本计划剩余任务授予“验证通过后创建当前隔离分支 checkpoint commit”的持续授权，因此 Task 7B 起不再逐任务创建用户 Gate；每个任务完成验证、范围检查和 Evidence 写回后直接 checkpoint commit 并进入下一 Todo。Task 10 的技术 Todo 全部完成后，才创建一次 Things 启动基线总体验收 Gate。该授权不包含 merge、push、凭据操作、破坏性清理或生产变更，这些动作仍须按各自权限边界单独审批。

**目标：** 在当前 WSL 中建立一条可重复构建、启动和验证 Things 三个服务及其必要 SaaS RPC、真实基础设施依赖的本地基线。

**架构：** 宿主机复用 Go 缓存编译五个静态二进制，并一一封装为五个运行时镜像。根级 `deploy/Makefile` 调用项目专用 Go 控制器，按有限状态机启动外部 PostgreSQL 独立数据库、Compose 基础设施和五个业务角色；每个阶段都写入脱敏证据并在失败时停止。

**技术栈：** Go 1.24.4、GNU Make、Docker Engine、Docker Compose、PostgreSQL 17/PostGIS、Redis、etcd、NATS JetStream、TDengine 3.3.6.34、EMQX 5.8.9、Go `testing`、YAML v3。

---

## 0. 实施纪律与前置条件

本计划依据：[Things 可重复启动与依赖契约基线设计](../specs/2026-08-26-things-reproducible-startup-dependency-contract-baseline-design.md)。

正式实施前必须满足：

1. 从已注册项目根 `ming-iot/` 完成当前规格任务，并创建一个新的实现任务；
2. 新任务声明下文列出的精确写入范围和 `shell`、`filesystem_write`、`network`、`docker` 能力；
3. 使用 `superpowers:using-git-worktrees` 创建独立 worktree；
4. `.loopx/` 与 `.codex/` 只在已注册项目根由 LoopX 维护，不复制进 worktree；
5. `things/` 与 `iot-doc/` 始终只读；
6. 现有 PostgreSQL 的 `iThings` 数据库不得成为测试目标，也不得被初始化或清理脚本修改。

建议先 dry-run 新任务登记：

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
loopx todo add --dry-run \
  --goal-id ming-iot-framework-v1 \
  --role agent \
  --text "实施 Things 可重复启动与依赖契约基线。" \
  --task-class advancement_task \
  --action-kind implement_things_startup_baseline \
  --task-domain code \
  --claimed-by ming-peer-01 \
  --required-capability shell \
  --required-capability filesystem_write \
  --required-capability network \
  --required-capability docker \
  --required-write-scope 'deploy/**' \
  --required-write-scope 'backend/saas/Makefile' \
  --required-write-scope 'backend/things/Makefile' \
  --required-write-scope 'backend/tools/cmd/thingsstartbaseline/**' \
  --required-write-scope 'backend/tools/internal/thingsstartbaseline/**' \
  --required-write-scope 'backend/tools/README.md' \
  --required-write-scope '.gitignore'
```

确认 dry-run 输出后再把 `--dry-run` 改为 `--execute`。登记返回的新任务 ID 必须写入执行记录，再完成当前规格任务并把新任务声明为 successor。若 LoopX 版本不接受 `docker` 作为 capability，停止登记并使用该版本公开的等价容器执行能力名称，不能静默删掉这一能力约束。

## 1. 文件责任图

### 新建文件

```text
deploy/Makefile                                      # 唯一用户命令入口
deploy/images/Dockerfile.sys                        # syssvr 单二进制镜像
deploy/images/Dockerfile.timedjob                   # timedjobsvr 单二进制镜像
deploy/images/Dockerfile.dm                         # dmsvr 单二进制镜像
deploy/images/Dockerfile.dg                         # dgsvr 单二进制镜像
deploy/images/Dockerfile.api                        # thingsvr 单二进制镜像
deploy/profiles/things-baseline/compose.yaml         # 基础设施与五个业务角色
deploy/profiles/things-baseline/contract.yaml        # 机器可读启动合同
deploy/profiles/things-baseline/.env.example         # 变量名与非敏感默认值
deploy/profiles/things-baseline/config/saas/sys.yaml
deploy/profiles/things-baseline/config/saas/timedjob.yaml
deploy/profiles/things-baseline/config/things/dm.yaml
deploy/profiles/things-baseline/config/things/dg.yaml
deploy/profiles/things-baseline/config/things/api.yaml
deploy/profiles/things-baseline/config/emqx/base.hocon
deploy/profiles/things-baseline/scripts/bootstrap-postgres.sh
backend/tools/cmd/thingsstartbaseline/main.go
backend/tools/cmd/thingsstartbaseline/main_test.go
backend/tools/internal/thingsstartbaseline/model.go
backend/tools/internal/thingsstartbaseline/contract.go
backend/tools/internal/thingsstartbaseline/contract_test.go
backend/tools/internal/thingsstartbaseline/artifact.go
backend/tools/internal/thingsstartbaseline/artifact_test.go
backend/tools/internal/thingsstartbaseline/stage.go
backend/tools/internal/thingsstartbaseline/stage_test.go
backend/tools/internal/thingsstartbaseline/command.go
backend/tools/internal/thingsstartbaseline/orchestrator.go
backend/tools/internal/thingsstartbaseline/orchestrator_test.go
backend/tools/internal/thingsstartbaseline/probe.go
backend/tools/internal/thingsstartbaseline/probe_test.go
backend/tools/internal/thingsstartbaseline/evidence.go
backend/tools/internal/thingsstartbaseline/evidence_test.go
```

### 修改文件

```text
.gitignore                     # 忽略 deploy/.runtime、真实 env 和生成产物
backend/saas/Makefile          # 增加两个 baseline 静态构建目标
backend/things/Makefile        # 增加三个 baseline 静态构建目标
backend/tools/README.md        # 记录控制器用途和命令
deploy/README.md               # 记录新旧部署入口边界
```

默认不得修改 `backend/saas/service/**` 和 `backend/things/service/**`。真实启动若证明必须修改业务代码，应以结构化失败证据结束本任务，再创建独立修复任务。

## 2. 任务与自动 checkpoint

Task 1 至 Task 7A 已产生的逐任务用户 Gate 仅作为历史执行记录保留，不作为后续任务模板。Task 7B、Task 8、Task 9 和 Task 10 之间使用 Agent Todo successor 连续推进；除凭据、破坏性清理等独立权限边界外，中途不得再创建普通 checkpoint 用户 Gate。

### Task 1：固定机器可读启动合同与 CLI 外壳

**文件：**

- 新建：`deploy/profiles/things-baseline/contract.yaml`
- 新建：`backend/tools/internal/thingsstartbaseline/model.go`
- 新建：`backend/tools/internal/thingsstartbaseline/contract.go`
- 新建：`backend/tools/internal/thingsstartbaseline/contract_test.go`
- 新建：`backend/tools/cmd/thingsstartbaseline/main.go`
- 新建：`backend/tools/cmd/thingsstartbaseline/main_test.go`

- [ ] **Step 1：先写合同失败测试**

在 `contract_test.go` 固定以下行为：必须恰好有五个角色；角色 ID、二进制、镜像和配置路径均唯一；数据库名只能是 `ming_iot_things_baseline`；阶段顺序不可更改；未知 YAML 字段必须失败。

```go
func TestValidateContractRequiresFiveOneToOneRoles(t *testing.T) {
	c := validContract()
	c.Roles[4].Image = c.Roles[3].Image
	err := ValidateContract(&c)
	if err == nil || !strings.Contains(err.Error(), "roles.image must be unique") {
		t.Fatalf("ValidateContract() error = %v", err)
	}
}

func TestValidateContractRejectsProtectedDatabase(t *testing.T) {
	c := validContract()
	c.Database.RequiredName = "iThings"
	err := ValidateContract(&c)
	if err == nil || !strings.Contains(err.Error(), "database.requiredName") {
		t.Fatalf("ValidateContract() error = %v", err)
	}
}

func TestLoadContractRejectsUnknownField(t *testing.T) {
	path := writeContractFixture(t, "schemaVersion: 1\nunknown: true\n")
	if _, err := LoadContract(path); err == nil || !strings.Contains(err.Error(), "field unknown") {
		t.Fatalf("LoadContract() error = %v", err)
	}
}
```

- [ ] **Step 2：运行测试并确认先失败**

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsstartbaseline -run 'Test(ValidateContract|LoadContract)' -count=1
```

期望：编译失败，提示 `ValidateContract`、`LoadContract` 或合同类型尚未定义。

- [ ] **Step 3：写入合同模型和稳定枚举**

`model.go` 使用以下公共模型；阶段与错误码不得用自由文本代替：

```go
package thingsstartbaseline

type StageID string

const (
	StagePrecheck          StageID = "PRECHECK"
	StageDatabaseBootstrap StageID = "DATABASE_BOOTSTRAP"
	StageInfraReady        StageID = "INFRA_READY"
	StageTimedJobReady     StageID = "TIMEDJOB_READY"
	StageSysReady          StageID = "SYS_READY"
	StageDMReady           StageID = "DM_READY"
	StageDGProcessReady    StageID = "DG_PROCESS_READY"
	StageDGLinkReady       StageID = "DG_LINK_READY"
	StageAPIReady          StageID = "API_READY"
	StageVerified          StageID = "VERIFIED"
)

type ErrorCode string

const (
	CodeBuildFailed                  ErrorCode = "BUILD_FAILED"
	CodeConfigInvalid                ErrorCode = "CONFIG_INVALID"
	CodeExternalDependencyUnavailable ErrorCode = "EXTERNAL_DEPENDENCY_UNAVAILABLE"
	CodeDatabaseBootstrapFailed      ErrorCode = "DATABASE_BOOTSTRAP_FAILED"
	CodeDependencyNotReady           ErrorCode = "DEPENDENCY_NOT_READY"
	CodeRoleExited                   ErrorCode = "ROLE_EXITED"
	CodeRoleNotReady                 ErrorCode = "ROLE_NOT_READY"
	CodeContractMismatch             ErrorCode = "CONTRACT_MISMATCH"
	CodeTimeout                      ErrorCode = "TIMEOUT"
)

type Contract struct {
	SchemaVersion int              `yaml:"schemaVersion"`
	Profile       string           `yaml:"profile"`
	ProjectName   string           `yaml:"projectName"`
	Database      DatabaseContract `yaml:"database"`
	Roles         []RoleContract   `yaml:"roles"`
	Stages        []StageContract  `yaml:"stages"`
	Timeouts      TimeoutContract  `yaml:"timeouts"`
}

type DatabaseContract struct {
	RequiredName  string   `yaml:"requiredName"`
	ForbiddenNames []string `yaml:"forbiddenNames"`
}

type RoleContract struct {
	ID         string `yaml:"id"`
	Module     string `yaml:"module"`
	Binary     string `yaml:"binary"`
	Image      string `yaml:"image"`
	Dockerfile string `yaml:"dockerfile"`
	Config     string `yaml:"config"`
	Endpoint   string `yaml:"endpoint"`
}

type StageContract struct {
	ID         StageID  `yaml:"id"`
	Components []string `yaml:"components"`
}

type TimeoutContract struct {
	ProbeSeconds int `yaml:"probeSeconds"`
	TotalSeconds int `yaml:"totalSeconds"`
}
```

- [ ] **Step 4：实现严格合同加载与校验**

`LoadContract` 必须使用 `yaml.Decoder.KnownFields(true)`、拒绝第二个 YAML 文档，并调用 `ValidateContract`。`ValidateContract` 固定五个角色映射和十个阶段顺序：

```go
var requiredRoles = []RoleContract{
	{ID: "sys", Module: "saas", Binary: "syssvr", Image: "ming-iot-sys:baseline", Dockerfile: "deploy/images/Dockerfile.sys", Config: "deploy/profiles/things-baseline/config/saas/sys.yaml", Endpoint: "sys:9180"},
	{ID: "timedjob", Module: "saas", Binary: "timedjobsvr", Image: "ming-iot-timedjob:baseline", Dockerfile: "deploy/images/Dockerfile.timedjob", Config: "deploy/profiles/things-baseline/config/saas/timedjob.yaml", Endpoint: "timedjob:3256"},
	{ID: "dm", Module: "things", Binary: "dmsvr", Image: "ming-iot-dm:baseline", Dockerfile: "deploy/images/Dockerfile.dm", Config: "deploy/profiles/things-baseline/config/things/dm.yaml", Endpoint: "dm:9081"},
	{ID: "dg", Module: "things", Binary: "dgsvr", Image: "ming-iot-dg:baseline", Dockerfile: "deploy/images/Dockerfile.dg", Config: "deploy/profiles/things-baseline/config/things/dg.yaml", Endpoint: "dg:6166"},
	{ID: "api", Module: "things", Binary: "thingsvr", Image: "ming-iot-api:baseline", Dockerfile: "deploy/images/Dockerfile.api", Config: "deploy/profiles/things-baseline/config/things/api.yaml", Endpoint: "api:7788"},
}
```

`contract.yaml` 必须与该映射一致，并声明阶段序列：

```yaml
schemaVersion: 1
profile: things-baseline
projectName: ming-iot-things-baseline
database:
  requiredName: ming_iot_things_baseline
  forbiddenNames:
    - iThings
roles:
  - {id: sys, module: saas, binary: syssvr, image: "ming-iot-sys:baseline", dockerfile: deploy/images/Dockerfile.sys, config: deploy/profiles/things-baseline/config/saas/sys.yaml, endpoint: "sys:9180"}
  - {id: timedjob, module: saas, binary: timedjobsvr, image: "ming-iot-timedjob:baseline", dockerfile: deploy/images/Dockerfile.timedjob, config: deploy/profiles/things-baseline/config/saas/timedjob.yaml, endpoint: "timedjob:3256"}
  - {id: dm, module: things, binary: dmsvr, image: "ming-iot-dm:baseline", dockerfile: deploy/images/Dockerfile.dm, config: deploy/profiles/things-baseline/config/things/dm.yaml, endpoint: "dm:9081"}
  - {id: dg, module: things, binary: dgsvr, image: "ming-iot-dg:baseline", dockerfile: deploy/images/Dockerfile.dg, config: deploy/profiles/things-baseline/config/things/dg.yaml, endpoint: "dg:6166"}
  - {id: api, module: things, binary: thingsvr, image: "ming-iot-api:baseline", dockerfile: deploy/images/Dockerfile.api, config: deploy/profiles/things-baseline/config/things/api.yaml, endpoint: "api:7788"}
stages:
  - {id: PRECHECK, components: [docker, compose, contract, secrets, postgres]}
  - {id: DATABASE_BOOTSTRAP, components: [postgres]}
  - {id: INFRA_READY, components: [redis, etcd, nats, tdengine, emqx]}
  - {id: TIMEDJOB_READY, components: [timedjob]}
  - {id: SYS_READY, components: [sys]}
  - {id: DM_READY, components: [dm]}
  - {id: DG_PROCESS_READY, components: [dg]}
  - {id: DG_LINK_READY, components: [emqx, dg.rpc]}
  - {id: API_READY, components: [api]}
  - {id: VERIFIED, components: [all]}
timeouts:
  probeSeconds: 90
  totalSeconds: 900
```

- [ ] **Step 5：实现 CLI 参数边界**

`main.go` 只接受 `contract`、`manifest`、`stage`、`image`、`up`、`verify`、`logs` 七个子命令。缺少子命令、未知参数和重复参数返回退出码 2；执行失败返回 1；成功返回 0。CLI 测试通过注入接口验证不会在单元测试中调用 Docker。

七个子命令的参数合同必须在本任务一次固定，后续任务不得临时增加隐式默认值：

| 子命令 | 必填参数 |
|---|---|
| `contract` | `--contract`、`--env-file` |
| `manifest` | `--contract`、`--repo`、`--bin-dir`、`--output` |
| `stage` | `--contract`、`--repo`、`--env-file`、`--runtime-root` |
| `image` | `--contract`、`--repo`、`--runtime-root` |
| `up` | `--contract`、`--repo`、`--env-file`、`--compose-file`、`--runtime-root` |
| `verify` | `--contract`、`--repo`、`--env-file`、`--compose-file`、`--runtime-root` |
| `logs` | `--contract`、`--repo`、`--env-file`、`--compose-file`、`--runtime-root` |

所有路径进入业务逻辑前都转为绝对路径；`--env-file` 必须存在、是普通文件且权限为 `0600`。`contract` 同时校验合同和环境保护规则，但不得执行 Docker 或 SQL。

- [ ] **Step 6：运行聚焦测试**

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsstartbaseline ./cmd/thingsstartbaseline -count=1
```

期望：`ok`，且不存在真实 Docker 子进程。

- [ ] **Step 7：提交检查点**

展示：

```bash
git status --short
git diff -- deploy/profiles/things-baseline/contract.yaml backend/tools/cmd/thingsstartbaseline backend/tools/internal/thingsstartbaseline
```

聚焦验证和范围检查通过后可创建 checkpoint commit，建议提交信息：`feat: define things startup baseline contract`；不创建逐任务用户 Gate。

### Task 2：让现有模块 Makefile 生成五个静态二进制

**文件：**

- 修改：`backend/saas/Makefile`
- 修改：`backend/things/Makefile`
- 新建：`backend/tools/internal/thingsstartbaseline/artifact.go`
- 新建：`backend/tools/internal/thingsstartbaseline/artifact_test.go`

- [ ] **Step 1：先写产物校验测试**

测试必须拒绝：缺少任一二进制、非 ELF、包含动态依赖、摘要为空、manifest 中二进制数不是五。

```go
func TestValidateArtifactSetRejectsMissingRole(t *testing.T) {
	set := validArtifactSet(t)
	delete(set, "thingsvr")
	err := ValidateArtifactSet(set)
	if err == nil || !strings.Contains(err.Error(), "thingsvr") {
		t.Fatalf("ValidateArtifactSet() error = %v", err)
	}
}

func TestInspectELFRejectsDynamicExecutable(t *testing.T) {
	libs, err := importedLibraries("/bin/sh")
	if err != nil {
		t.Skipf("host /bin/sh is not ELF: %v", err)
	}
	if len(libs) == 0 {
		t.Fatalf("fixture unexpectedly static")
	}
}
```

- [ ] **Step 2：确认测试先失败**

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsstartbaseline -run 'Test(ValidateArtifactSet|InspectELF)' -count=1
```

期望：编译失败，产物 API 尚未定义。

- [ ] **Step 3：保留旧目标并增加 baseline 目标**

`backend/saas/Makefile` 增加：

```make
GO ?= go
BASELINE_GO_BUILD := env GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -tags no_k8s -trimpath

.PHONY: build.baseline build.baseline.sys build.baseline.timedjob
build.baseline: build.baseline.sys build.baseline.timedjob

build.baseline.sys: | $(BIN_DIR)
	umask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/syssvr ./service/syssvr

build.baseline.timedjob: | $(BIN_DIR)
	umask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/timedjobsvr ./service/timed/timedjobsvr
```

`backend/things/Makefile` 增加：

```make
BASELINE_GO_BUILD := env GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -tags no_k8s -trimpath

.PHONY: build.baseline build.baseline.api build.baseline.dg build.baseline.dm
build.baseline: build.baseline.api build.baseline.dg build.baseline.dm

build.baseline.api: | $(BIN_DIR)
	umask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/thingsvr ./service/apisvr

build.baseline.dg: | $(BIN_DIR)
	umask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/dgsvr ./service/dgsvr

build.baseline.dm: | $(BIN_DIR)
	umask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/dmsvr ./service/dmsvr
```

不得修改现有 `build` 目标的语义，避免影响旧工作流。

- [ ] **Step 4：实现 ELF、SHA-256 和 Git revision manifest**

`artifact.go` 使用标准库 `debug/elf`、`crypto/sha256` 和 `encoding/json`：

```go
type Artifact struct {
	Role       string `json:"role"`
	Binary     string `json:"binary"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	StaticELF  bool   `json:"staticElf"`
}

type BuildManifest struct {
	SchemaVersion int        `json:"schemaVersion"`
	GitRevision   string     `json:"gitRevision"`
	Artifacts     []Artifact `json:"artifacts"`
}
```

静态判定必须打开 ELF 并要求 `ImportedLibraries()` 返回空集合。manifest 先写同目录临时文件，`fsync` 后原子 rename 到 `deploy/.runtime/build-manifest.json`。

- [ ] **Step 5：运行五个真实构建和 manifest 校验**

```bash
make -C backend/saas build.baseline
make -C backend/things build.baseline
cd backend/tools
GOWORK=off go run ./cmd/thingsstartbaseline manifest \
  --contract ../../deploy/profiles/things-baseline/contract.yaml \
  --repo ../.. \
  --bin-dir ../bin \
  --output ../../deploy/.runtime/build-manifest.json
```

期望：五个文件存在，manifest 有五项，所有 `staticElf=true`，工作树没有新增被跟踪二进制。

- [ ] **Step 6：运行模块测试**

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsstartbaseline ./cmd/thingsstartbaseline -count=1
```

期望：`ok`。

- [ ] **Step 7：提交检查点**

聚焦验证和范围检查通过后可创建 checkpoint commit，建议提交信息：`build: add static baseline service artifacts`；不创建逐任务用户 Gate。

### Task 3：建立配置 staging、数据库保护和密钥边界

**文件：**

- 新建：`deploy/profiles/things-baseline/.env.example`
- 新建：`deploy/profiles/things-baseline/config/saas/sys.yaml`
- 新建：`deploy/profiles/things-baseline/config/saas/timedjob.yaml`
- 新建：`deploy/profiles/things-baseline/config/things/dm.yaml`
- 新建：`deploy/profiles/things-baseline/config/things/dg.yaml`
- 新建：`deploy/profiles/things-baseline/config/things/api.yaml`
- 新建：`backend/tools/internal/thingsstartbaseline/stage.go`
- 新建：`backend/tools/internal/thingsstartbaseline/stage_test.go`
- 修改：`.gitignore`

- [ ] **Step 1：先写保护测试**

```go
func TestValidateRuntimeEnvironmentRejectsIThingsDSN(t *testing.T) {
	env := validEnvironment()
	env["THINGS_DB_DSN"] = "postgres://app:redacted@host:5432/iThings?sslmode=disable"
	err := ValidateRuntimeEnvironment(env, "ming_iot_things_baseline")
	if err == nil || !errors.Is(err, ErrContractMismatch) {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateSecretFileRequires0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.local")
	if err := os.WriteFile(path, []byte("POSTGRES_ADMIN_PASSWORD=x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSecretFile(path); err == nil || !strings.Contains(err.Error(), "0600") {
		t.Fatalf("error = %v", err)
	}
}

func TestStageCreatesFiveIsolatedContexts(t *testing.T) {
	result, err := Stage(StageRequest{RepoRoot: fixtureRepo(t), RuntimeRoot: t.TempDir(), Contract: validContract()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contexts) != 5 {
		t.Fatalf("contexts = %d, want 5", len(result.Contexts))
	}
}
```

- [ ] **Step 2：确认测试先失败**

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsstartbaseline -run 'Test(ValidateRuntimeEnvironment|ValidateSecretFile|Stage)' -count=1
```

期望：编译失败，保护与 staging API 尚未定义。

- [ ] **Step 3：提交非敏感环境变量清单**

`.env.example` 只包含变量名和安全默认值：

```dotenv
POSTGRES_ADMIN_HOST=host.docker.internal
POSTGRES_ADMIN_PORT=5432
POSTGRES_ADMIN_USER=postgres
POSTGRES_ADMIN_PASSWORD=
THINGS_BASELINE_DB=ming_iot_things_baseline
THINGS_BASELINE_DB_USER=ming_iot_things_baseline
THINGS_BASELINE_DB_PASSWORD=
CORE_USER_TOKEN_SECRET=
THINGS_INTERNAL_AUTH_PASSWORD=
THINGS_EMQX_API_KEY=
THINGS_EMQX_SECRET_KEY=
TDENGINE_ROOT_PASSWORD=taosdata
THINGS_BASELINE_API_PORT=17788
THINGS_BASELINE_MQTT_PORT=11883
THINGS_BASELINE_EMQX_DASHBOARD_PORT=18084
```

真实值写入同目录 `.env.local`，权限必须为 `0600`，不得由程序自动填入弱默认密码。

- [ ] **Step 4：写入五份完整基线配置**

五份配置统一使用：

```yaml
Mode: dev
Log:
  Encoding: plain
```

并分别固定以下关键字段：

```yaml
# sys.yaml
Name: sys.rpc
ListenOn: 0.0.0.0:9180
Etcd: {Hosts: [etcd:2379], Key: sys.rpc}
Database: {DBType: pgsql, IsInitTable: true, DSN: "${CORE_DB_DSN}"}
CacheRedis: [{Host: "redis:6379", Type: node}]
Event: {Mode: natsJs, Nats: {Url: "nats://nats:4222", Consumer: syssvr}}
OssConf: {OssType: local, StorePath: /app/oss}
UserToken: {AccessSecret: "${CORE_USER_TOKEN_SECRET}", AccessExpire: 3600000}
Sms: {Enable: false}
TimedJobRpc:
  Enable: true
  Mode: grpc
  Conf: {Timeout: 1000000, Etcd: {Hosts: [etcd:2379], Key: timedjob.rpc}}
```

```yaml
# timedjob.yaml
Name: timedjob.rpc
ListenOn: 0.0.0.0:3256
Etcd: {Hosts: [etcd:2379], Key: timedjob.rpc}
Database: {DBType: pgsql, IsInitTable: true, DSN: "${CORE_DB_DSN}"}
CacheRedis: [{Host: "redis:6379", Type: node}]
Event: {Mode: natsJs, Nats: {Url: "nats://nats:4222", Consumer: timedjobsvr}}
```

```yaml
# dm.yaml
Name: dm.rpc
ListenOn: 0.0.0.0:9081
Etcd: {Hosts: [etcd:2379], Key: dm.rpc}
Database: {DBType: pgsql, IsInitTable: true, DSN: "${THINGS_DB_DSN}"}
CacheRedis: [{Host: "redis:6379", Type: node}]
TSDB: {DBType: tdengine, Driver: taosRestful, DSN: "${THINGS_TSDB_DSN}"}
Event: {Mode: natsJs, Nats: {Url: "nats://nats:4222", Consumer: dmsvr}}
OssConf: {OssType: local, StorePath: /app/oss}
AuthWhite:
  Users: [{UserName: dgsvr, Password: "${THINGS_INTERNAL_AUTH_PASSWORD}"}]
  IpRange: ["127.0.0.0/8", "172.16.0.0/12", "10.0.0.0/8"]
TimedJobRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Etcd: {Hosts: [etcd:2379], Key: timedjob.rpc}}}
SysRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Etcd: {Hosts: [etcd:2379], Key: sys.rpc}}}
```

```yaml
# dg.yaml
Name: dg.rpc
ListenOn: 0.0.0.0:6166
CacheRedis: [{Host: "redis:6379", Type: node}]
Event: {Mode: natsJs, Nats: {Url: "nats://nats:4222", Consumer: dgsvr}}
DmRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Endpoints: ["dm:9081"]}}
TimedJobRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Endpoints: ["timedjob:3256"]}}
DevLink:
  Mode: mqtt
  SubMode: emq
  Mqtt: {ClientID: dg.rpc, User: dgsvr, Pass: "${THINGS_INTERNAL_AUTH_PASSWORD}", Brokers: ["tcp://emqx:1883"]}
```

```yaml
# api.yaml
Name: api
Host: 0.0.0.0
Port: 7788
Etcd: {Hosts: [etcd:2379]}
CacheRedis: [{Host: "redis:6379", Type: node}]
Event: {Mode: natsJs, Nats: {Url: "nats://nats:4222", Consumer: things-api}}
OssConf: {OssType: local, StorePath: /app/oss}
SysRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Etcd: {Hosts: [etcd:2379], Key: sys.rpc}}}
DmRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Etcd: {Hosts: [etcd:2379], Key: dm.rpc}}}
TimedJobRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Etcd: {Hosts: [etcd:2379], Key: timedjob.rpc}}}
DgRpc: {Enable: true, Mode: grpc, Conf: {Timeout: 1000000, Endpoints: ["dg:6166"]}}
```

不得把 `TimedSchedulerRpc` 写入这些配置。

- [ ] **Step 5：实现 staging**

`Stage` 为每个角色创建 `deploy/.runtime/stage/<role>/`，内容仅为：

```text
bin/<该角色唯一二进制>
etc/<该角色配置名>.yaml
etc/i18n/              # sys、dm、api 需要
etc/init/              # sys、dm 需要
oss/
```

配置从已提交模板复制，i18n/init 从相应模块 `service/apisvr/etc/` 复制；任何符号链接、越出仓库路径或未在合同中的文件都必须失败。当前 `dmsvr` 启动路径会执行数据库迁移和 `etc/init` 初始化，因此 dm 资源不能省略；这些写入只允许落入已预检的 `ming_iot_things_baseline`。`CORE_LOCAL_SKIP_STARTUP_SIDE_EFFECTS=true` 只用于抑制 SaaS `syssvr` 已支持的额外启动副作用，不得假定它会跳过 `dmsvr` 初始化。staging 结束后写 `stage-manifest.json`，包含文件相对路径、模式和 SHA-256，不记录正文。

- [ ] **Step 6：更新忽略规则并验证**

在 `.gitignore` 追加：

```gitignore
/deploy/.runtime/
/deploy/profiles/things-baseline/.env.local
```

运行：

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsstartbaseline -run 'Test(ValidateRuntimeEnvironment|ValidateSecretFile|Stage)' -count=1
git check-ignore -v deploy/.runtime/test deploy/profiles/things-baseline/.env.local
```

期望：测试 `ok`，两个路径都被明确忽略。

- [ ] **Step 7：提交检查点**

聚焦验证和范围检查通过后可创建 checkpoint commit，建议提交信息：`feat: stage isolated things baseline configs`；不创建逐任务用户 Gate。

### Task 4：构建五个一一对应的运行时镜像

**文件：**

- 新建：`deploy/images/Dockerfile.sys`
- 新建：`deploy/images/Dockerfile.timedjob`
- 新建：`deploy/images/Dockerfile.dm`
- 新建：`deploy/images/Dockerfile.dg`
- 新建：`deploy/images/Dockerfile.api`
- 扩展：`backend/tools/internal/thingsstartbaseline/artifact_test.go`

- [ ] **Step 1：先写镜像合同测试**

测试读取五个 Dockerfile，要求：没有 `FROM golang`、`go build`、`go mod download`；每个文件只有一个合同二进制 `COPY`；`ENTRYPOINT` 与角色映射一致。

```go
func TestRuntimeDockerfilesAreOneRoleOneBinary(t *testing.T) {
	for _, role := range requiredRoles {
		body := readRepoFile(t, role.Dockerfile)
		for _, forbidden := range []string{"FROM golang", "go build", "go mod download"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s contains %q", role.Dockerfile, forbidden)
			}
		}
		if !strings.Contains(body, "COPY bin/"+role.Binary+" /app/"+role.Binary) {
			t.Fatalf("%s does not copy only %s", role.Dockerfile, role.Binary)
		}
	}
}
```

- [ ] **Step 2：确认测试先失败**

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsstartbaseline -run TestRuntimeDockerfilesAreOneRoleOneBinary -count=1
```

期望：失败，五个 Dockerfile 尚不存在。

- [ ] **Step 3：创建五个显式 Dockerfile**

每个文件使用相同基础层但写死自己的二进制。例如 `Dockerfile.sys`：

```dockerfile
FROM alpine:3.20
ARG VCS_REF
ARG BINARY_SHA256
LABEL org.opencontainers.image.revision=$VCS_REF \
      io.ming-iot.binary.sha256=$BINARY_SHA256 \
      io.ming-iot.role=sys
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY bin/syssvr /app/syssvr
COPY etc /app/etc
RUN mkdir -p /app/oss && chmod 0755 /app/syssvr
ENTRYPOINT ["/app/syssvr"]
```

其余四个文件只允许替换 role、二进制名和 `ENTRYPOINT`，分别为 `timedjobsvr`、`dmsvr`、`dgsvr`、`thingsvr`。禁止通过一个携带五个二进制的通用业务镜像恢复旧设计。

- [ ] **Step 4：构建并检查五个镜像**

`thingsstartbaseline stage` 先产生五个最小 build context。随后分别执行：

```bash
docker build -f deploy/images/Dockerfile.sys -t ming-iot-sys:baseline deploy/.runtime/stage/sys
docker build -f deploy/images/Dockerfile.timedjob -t ming-iot-timedjob:baseline deploy/.runtime/stage/timedjob
docker build -f deploy/images/Dockerfile.dm -t ming-iot-dm:baseline deploy/.runtime/stage/dm
docker build -f deploy/images/Dockerfile.dg -t ming-iot-dg:baseline deploy/.runtime/stage/dg
docker build -f deploy/images/Dockerfile.api -t ming-iot-api:baseline deploy/.runtime/stage/api
```

实际 Makefile 必须同时传入 `VCS_REF` 和对应 `BINARY_SHA256`。使用 `docker image inspect` 验证五个镜像的标签、Entrypoint 和 ID，并写入运行证据。

- [ ] **Step 5：运行镜像合同测试**

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsstartbaseline -run 'TestRuntimeDockerfiles|TestValidateArtifactSet' -count=1
```

期望：`ok`。

- [ ] **Step 6：提交检查点**

聚焦验证和范围检查通过后可创建 checkpoint commit，建议提交信息：`build: package five baseline runtime images`；不创建逐任务用户 Gate。

### Task 5：编排真实基础设施和独立 PostgreSQL 数据库

**文件：**

- 新建：`deploy/profiles/things-baseline/compose.yaml`
- 新建：`deploy/profiles/things-baseline/config/emqx/base.hocon`
- 新建：`deploy/profiles/things-baseline/scripts/bootstrap-postgres.sh`
- 扩展：`backend/tools/internal/thingsstartbaseline/contract_test.go`

- [ ] **Step 1：先写 Compose 静态合同测试**

测试要求服务集合至少包含 `postgres-bootstrap`、`redis`、`etcd`、`nats`、`tdengine`、`emqx`、`curl`、`grpcurl`、五个业务角色；不得出现 Kafka、MinIO、`timedscheduler`；业务角色不得带 `build:`；只允许 API、MQTT 和 EMQX Dashboard 映射宿主端口。

- [ ] **Step 2：写入基础设施 Compose**

`compose.yaml` 固定以下基础设施镜像与行为：

```yaml
name: ming-iot-things-baseline
networks:
  baseline: {}
volumes:
  redis_data: {}
  etcd_data: {}
  nats_data: {}
  tdengine_data: {}
  tdengine_log: {}
  emqx_data: {}
  local_oss: {}

services:
  postgres-bootstrap:
    image: postgres:17-alpine
    profiles: [tools]
    extra_hosts: ["host.docker.internal:host-gateway"]
    env_file: ["${THINGS_BASELINE_ENV_FILE:-.env.local}"]
    volumes:
      - ./scripts/bootstrap-postgres.sh:/usr/local/bin/bootstrap-postgres:ro
    entrypoint: ["/usr/local/bin/bootstrap-postgres"]

  redis:
    image: redis:7.4-alpine
    command: ["redis-server", "--appendonly", "yes"]
    volumes: ["redis_data:/data"]
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 3s
      timeout: 2s
      retries: 30
    networks: [baseline]

  etcd:
    image: quay.io/coreos/etcd:v3.5.21
    command: ["etcd", "--name=etcd", "--data-dir=/etcd-data", "--listen-client-urls=http://0.0.0.0:2379", "--advertise-client-urls=http://etcd:2379"]
    volumes: ["etcd_data:/etcd-data"]
    healthcheck:
      test: ["CMD", "etcdctl", "endpoint", "health"]
      interval: 3s
      timeout: 3s
      retries: 30
    networks: [baseline]

  nats:
    image: nats:2.10-alpine
    command: ["-js", "-sd", "/data", "-m", "8222"]
    volumes: ["nats_data:/data"]
    networks: [baseline]

  tdengine:
    image: tdengine/tdengine:3.3.6.34
    hostname: tdengine
    volumes: ["tdengine_data:/var/lib/taos", "tdengine_log:/var/log/taos"]
    networks: [baseline]

  emqx:
    image: emqx/emqx:5.8.9
    hostname: emqx
    environment:
      EMQX_NODE__NAME: emqx@emqx
      EMQX_API_KEY__BOOTSTRAP_FILE: /opt/emqx/etc/default_api_key.conf
    volumes:
      - emqx_data:/opt/emqx/data
      - ./config/emqx/base.hocon:/opt/emqx/etc/base.hocon:ro
      - ../../.runtime/secrets/emqx-api-key.conf:/opt/emqx/etc/default_api_key.conf:ro
    ports:
      - "127.0.0.1:${THINGS_BASELINE_MQTT_PORT:-11883}:1883"
      - "127.0.0.1:${THINGS_BASELINE_EMQX_DASHBOARD_PORT:-18084}:18083"
    healthcheck:
      test: ["CMD", "/opt/emqx/bin/emqx", "ctl", "status"]
      interval: 5s
      timeout: 10s
      retries: 30
    networks: [baseline]

  grpcurl:
    image: fullstorydev/grpcurl:v1.9.3-alpine
    profiles: [tools]
    networks: [baseline]

  curl:
    image: curlimages/curl:8.12.1
    profiles: [tools]
    networks: [baseline]
```

Compose 正式落盘前必须实际拉取固定标签并验证 `etcd`、`etcdctl`、`grpcurl` 和 `curl` 可执行；若固定标签的默认入口不符合上述调用方式，应换成兼容的固定标签并更新合同测试，不得放宽探针。

- [ ] **Step 3：创建 EMQX bootstrap 配置**

`base.hocon`：

```hocon
api_key {
  bootstrap_file = "etc/default_api_key.conf"
}
```

staging 从 `.env.local` 读取 `THINGS_EMQX_API_KEY` 和 `THINGS_EMQX_SECRET_KEY`，生成权限 `0600` 的：

```text
${THINGS_EMQX_API_KEY}:${THINGS_EMQX_SECRET_KEY}:viewer
```

文件只进入 `deploy/.runtime/secrets/`。EMQX 使用 API key 的 HTTP Basic Authentication；密钥不得进入 Compose 展开输出和证据正文。

- [ ] **Step 4：实现 PostgreSQL 幂等 bootstrap**

`bootstrap-postgres.sh` 必须以固定名称 fail-closed：

```bash
#!/usr/bin/env sh
set -eu
: "${POSTGRES_ADMIN_HOST:?missing POSTGRES_ADMIN_HOST}"
: "${POSTGRES_ADMIN_PORT:?missing POSTGRES_ADMIN_PORT}"
: "${POSTGRES_ADMIN_USER:?missing POSTGRES_ADMIN_USER}"
: "${POSTGRES_ADMIN_PASSWORD:?missing POSTGRES_ADMIN_PASSWORD}"
: "${THINGS_BASELINE_DB:?missing THINGS_BASELINE_DB}"
: "${THINGS_BASELINE_DB_USER:?missing THINGS_BASELINE_DB_USER}"
: "${THINGS_BASELINE_DB_PASSWORD:?missing THINGS_BASELINE_DB_PASSWORD}"
[ "$THINGS_BASELINE_DB" = "ming_iot_things_baseline" ] || { echo 'CONFIG_INVALID: protected database target'; exit 2; }
[ "$THINGS_BASELINE_DB_USER" = "ming_iot_things_baseline" ] || { echo 'CONFIG_INVALID: unexpected database role'; exit 2; }
export PGHOST="$POSTGRES_ADMIN_HOST" PGPORT="$POSTGRES_ADMIN_PORT" PGUSER="$POSTGRES_ADMIN_USER" PGPASSWORD="$POSTGRES_ADMIN_PASSWORD"
psql --dbname postgres --set=ON_ERROR_STOP=1 \
  --set=app_user="$THINGS_BASELINE_DB_USER" \
  --set=app_password="$THINGS_BASELINE_DB_PASSWORD" \
  --set=db_name="$THINGS_BASELINE_DB" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'app_user', :'app_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'app_user') \gexec
SELECT format('ALTER ROLE %I LOGIN PASSWORD %L', :'app_user', :'app_password') \gexec
SELECT format('CREATE DATABASE %I OWNER %I', :'db_name', :'app_user')
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = :'db_name') \gexec
SQL
psql --dbname "$THINGS_BASELINE_DB" --set=ON_ERROR_STOP=1 <<'SQL'
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS postgis_topology;
SELECT current_database();
SELECT postgis_full_version();
SQL
```

脚本不得包含 `DROP DATABASE`、`DROP ROLE`、`TRUNCATE` 或对 `iThings` 的引用。

- [ ] **Step 5：验证静态 Compose 和脚本保护**

```bash
THINGS_BASELINE_ENV_FILE="$(pwd)/deploy/profiles/things-baseline/.env.example" \
docker compose --env-file deploy/profiles/things-baseline/.env.example \
  -f deploy/profiles/things-baseline/compose.yaml config >/tmp/things-baseline-compose.yaml
rg -n 'iThings|DROP DATABASE|DROP ROLE|TRUNCATE' \
  deploy/profiles/things-baseline/compose.yaml \
  deploy/profiles/things-baseline/config \
  deploy/profiles/things-baseline/scripts
```

期望：Compose 解析成功；`rg` 无输出。`.env.example` 中空密钥只用于 `config` 解析，不能用于实际启动。

- [ ] **Step 6：提交检查点**

聚焦验证和范围检查通过后可创建 checkpoint commit，建议提交信息：`feat: define things baseline infrastructure`；不创建逐任务用户 Gate。

### Task 6：加入五个业务服务与有序启动状态机

**文件：**

- 扩展：`deploy/profiles/things-baseline/compose.yaml`
- 新建：`backend/tools/internal/thingsstartbaseline/command.go`
- 新建：`backend/tools/internal/thingsstartbaseline/orchestrator.go`
- 新建：`backend/tools/internal/thingsstartbaseline/orchestrator_test.go`

- [ ] **Step 1：先写状态机测试**

使用 fake command runner 和 fake probe，验证启动顺序严格为 infra → timedjob → sys → dm → dg → api；任一阶段失败后不执行后续命令；超时映射为 `TIMEOUT`。

```go
func TestOrchestratorRunsApprovedOrder(t *testing.T) {
	fake := newFakeRuntime()
	o := NewOrchestrator(fake, validContract(), t.TempDir())
	if err := o.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []StageID{StagePrecheck, StageDatabaseBootstrap, StageInfraReady, StageTimedJobReady, StageSysReady, StageDMReady, StageDGProcessReady, StageDGLinkReady, StageAPIReady, StageVerified}
	if diff := cmp.Diff(want, fake.completedStages); diff != "" {
		t.Fatalf("stage order mismatch (-want +got):\n%s", diff)
	}
}

func TestOrchestratorStopsAfterDMFailure(t *testing.T) {
	fake := newFakeRuntime()
	fake.failStage = StageDMReady
	err := NewOrchestrator(fake, validContract(), t.TempDir()).Up(context.Background())
	if diagnosticCode(err) != CodeRoleNotReady || slices.Contains(fake.completedStages, StageDGProcessReady) {
		t.Fatalf("error/stages = %v/%v", err, fake.completedStages)
	}
}
```

- [ ] **Step 2：确认测试先失败**

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsstartbaseline -run TestOrchestrator -count=1
```

期望：编译失败，状态机尚未实现。

- [ ] **Step 3：在 Compose 中加入五个服务**

五个服务都只引用预构建镜像，不声明 `build:`，共同挂载 `local_oss`，并设置：

```yaml
x-app-common: &app-common
  env_file: ["${THINGS_BASELINE_ENV_FILE:-.env.local}"]
  environment:
    CORE_DB_DSN: "postgres://${THINGS_BASELINE_DB_USER}:${THINGS_BASELINE_DB_PASSWORD}@host.docker.internal:${POSTGRES_ADMIN_PORT}/ming_iot_things_baseline?sslmode=disable"
    THINGS_DB_DSN: "postgres://${THINGS_BASELINE_DB_USER}:${THINGS_BASELINE_DB_PASSWORD}@host.docker.internal:${POSTGRES_ADMIN_PORT}/ming_iot_things_baseline?sslmode=disable"
    THINGS_TSDB_DSN: "root:${TDENGINE_ROOT_PASSWORD}@http(tdengine:6041)/"
    CORE_LOCAL_SKIP_STARTUP_SIDE_EFFECTS: "true"
    CORE_LOCAL_SKIP_AUTO_MIGRATE: "false"
  extra_hosts: ["host.docker.internal:host-gateway"]
  volumes: ["local_oss:/app/oss"]
  networks: [baseline]
  restart: "no"

services:
  timedjob:
    <<: *app-common
    image: ming-iot-timedjob:baseline
  sys:
    <<: *app-common
    image: ming-iot-sys:baseline
  dm:
    <<: *app-common
    image: ming-iot-dm:baseline
  dg:
    <<: *app-common
    image: ming-iot-dg:baseline
  api:
    <<: *app-common
    image: ming-iot-api:baseline
    ports: ["127.0.0.1:${THINGS_BASELINE_API_PORT:-17788}:7788"]
```

Compose `depends_on` 只表达直接依赖，不承担最终就绪判断。真正启动顺序由控制器逐阶段执行 `docker compose up -d <service>` 并等待探针成功。

- [ ] **Step 4：实现无 shell 的命令执行边界**

`command.go` 只接受 argv 数组并设置工作目录；不得使用 `sh -c`。stdout/stderr 分别计算 SHA-256，只保留最多 4 KiB 脱敏尾部。进程组超时后整体终止。

```go
type Command struct {
	Path string
	Args []string
	Dir  string
	Env  []string
}

type CommandResult struct {
	ExitCode     int
	DurationMS   int64
	StdoutSHA256 string
	StderrSHA256 string
	Summary      string
}

type CommandRunner interface {
	Run(context.Context, Command) (CommandResult, error)
}
```

控制器调用 Compose 时必须显式把已校验的 `--env-file` 绝对路径写入子进程环境变量 `THINGS_BASELINE_ENV_FILE`，同时继续传递 Compose 的 `--env-file` 参数。这样业务容器读取真实运行环境，而 Fast 校验可显式改用 `.env.example`，不存在对调用者当前目录或隐式 `.env` 的依赖。

脱敏器至少覆盖 `POSTGRES_ADMIN_PASSWORD`、`THINGS_BASELINE_DB_PASSWORD`、`CORE_USER_TOKEN_SECRET`、`THINGS_INTERNAL_AUTH_PASSWORD`、`THINGS_EMQX_API_KEY`、`THINGS_EMQX_SECRET_KEY` 和 TDengine 密码。

- [ ] **Step 5：实现状态机最小逻辑**

每个阶段在开始前写入 `RUNNING`，成功写 `PASS`，失败写 `FAIL` 和稳定错误码。状态文件原子更新；总超时由根 context 控制，单探针超时使用合同中的 `probeSeconds`。

- [ ] **Step 6：运行状态机测试和静态 Compose 校验**

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsstartbaseline -run 'TestOrchestrator|TestCommand' -count=1
cd ../..
THINGS_BASELINE_ENV_FILE="$(pwd)/deploy/profiles/things-baseline/.env.example" \
docker compose --env-file deploy/profiles/things-baseline/.env.example -f deploy/profiles/things-baseline/compose.yaml config >/dev/null
```

期望：全部成功。

- [ ] **Step 7：提交检查点**

聚焦验证和范围检查通过后可创建 checkpoint commit，建议提交信息：`feat: orchestrate ordered things baseline startup`；不创建逐任务用户 Gate。

### Task 7：实现协议级探针和脱敏证据

**文件：**

- 新建：`backend/tools/internal/thingsstartbaseline/probe.go`
- 新建：`backend/tools/internal/thingsstartbaseline/probe_test.go`
- 新建：`backend/tools/internal/thingsstartbaseline/evidence.go`
- 新建：`backend/tools/internal/thingsstartbaseline/evidence_test.go`
- 扩展：`backend/tools/internal/thingsstartbaseline/orchestrator.go`

- [ ] **Step 1：先写探针和证据测试**

测试覆盖：NATS `/healthz` 成功但 JetStream 未启用仍失败；TDengine ping 成功但 SQL 失败仍失败；dg 容器运行但 EMQX 无 `dg.rpc` 客户端仍失败；证据包含十个阶段且绝不包含传入密钥。

```go
func TestDGLinkProbeRequiresEMQXClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()
	err := ProbeDGLink(context.Background(), server.Client(), server.URL, "key", "secret", "dg.rpc")
	if diagnosticCode(err) != CodeDependencyNotReady {
		t.Fatalf("error = %v", err)
	}
}

func TestEvidenceRedactsSecrets(t *testing.T) {
	e := failedEvidenceFixture("super-secret-value")
	b, err := MarshalEvidence(e, []string{"super-secret-value"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("super-secret-value")) {
		t.Fatalf("evidence leaked secret: %s", b)
	}
}
```

- [ ] **Step 2：确认测试先失败**

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsstartbaseline -run 'Test(DGLinkProbe|Evidence|NATSProbe|TDengineProbe)' -count=1
```

期望：编译失败，探针和 evidence API 尚未定义。

- [ ] **Step 3：实现明确探针**

探针必须执行：

| 组件 | 命令或协议 |
|---|---|
| PostgreSQL | `postgres-bootstrap` 内 `pg_isready`；查询 `current_database()` 和 `postgis_full_version()` |
| Redis | `docker compose exec -T redis redis-cli ping`，必须为 `PONG` |
| etcd | `docker compose exec -T etcd etcdctl endpoint health` |
| NATS | 通过 `curl` 工具容器请求 `http://nats:8222/healthz`，再请求 `/jsz` 并确认 JetStream 可用 |
| TDengine | 通过 `curl` 工具容器请求 `http://tdengine:6041/-/ping`，再向 `/rest/sql` 执行 `show databases` |
| EMQX | `docker compose exec -T emqx /opt/emqx/bin/emqx ctl status` |
| 四个 gRPC 角色 | `docker compose run --rm --no-deps grpcurl -plaintext <endpoint> list` |
| dg 链路 | 使用 API key Basic Auth 查询 `/api/v5/clients/dg.rpc`，结果必须包含 clientid `dg.rpc` |
| API | 请求 `http://127.0.0.1:<api-port>/`，接受由 go-zero 返回的预期非 5xx 响应 |

所有 HTTP 响应体最多读取 1 MiB；任何超限响应以 `CONTRACT_MISMATCH` 失败。

- [ ] **Step 4：实现证据模型和原子写入**

```go
type StageEvidence struct {
	ID         StageID   `json:"id"`
	Result     string    `json:"result"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	DurationMS int64     `json:"durationMs"`
	Component  string    `json:"component,omitempty"`
	Code       ErrorCode `json:"code,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	LogRef     string    `json:"logRef,omitempty"`
}

type Evidence struct {
	SchemaVersion  int               `json:"schemaVersion"`
	RunID          string            `json:"runId"`
	GitRevision    string            `json:"gitRevision"`
	ContractSHA256 string            `json:"contractSha256"`
	OverallResult  string            `json:"overallResult"`
	Stages         []StageEvidence   `json:"stages"`
	Binaries       []Artifact        `json:"binaries"`
	Images         map[string]string `json:"images"`
}
```

每轮必须原子写入完整证据目录：

```text
deploy/.runtime/runs/<run-id>/
├── evidence.json
├── versions.json
├── compose-config.sha256
├── binary-sha256.json
└── logs/
```

`versions.json` 记录 Docker、Compose、五个固定基础设施镜像和五个业务镜像的版本/ID；`compose-config.sha256` 只记录已展开 Compose 的 SHA-256，不保存可能含密钥的展开正文；`binary-sha256.json` 从已验证的 build manifest 投影。日志引用只能是该 run 目录下的相对路径。任何文件写入均采用同目录临时文件、`fsync` 和原子 rename。

- [ ] **Step 5：运行聚焦测试、race 和 vet**

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsstartbaseline ./cmd/thingsstartbaseline -count=1
GOWORK=off go test -race ./internal/thingsstartbaseline ./cmd/thingsstartbaseline -count=1
GOWORK=off go vet ./internal/thingsstartbaseline ./cmd/thingsstartbaseline
```

期望：全部成功。

- [ ] **Step 6：提交检查点**

聚焦验证和范围检查通过后可创建 checkpoint commit，建议提交信息：`feat: add protocol probes and startup evidence`；不创建逐任务用户 Gate，随后直接进入 Task 8。

### Task 8：建立根级统一 Makefile 和操作文档

**文件：**

- 新建：`deploy/Makefile`
- 修改：`deploy/README.md`
- 修改：`backend/tools/README.md`
- 扩展：`backend/tools/cmd/thingsstartbaseline/main_test.go`

- [ ] **Step 1：先写 CLI 编排测试**

测试注入 fake executor，验证 `up` 调用 contract → manifest → stage → database → infra → roles → verify；`verify` 不执行构建或启动；`logs` 只读取 Compose 日志并经过统一脱敏器；未知命令退出 2。

- [ ] **Step 2：创建根级命令入口**

`deploy/Makefile` 固定：

```make
SHELL := /usr/bin/env bash
ROOT := $(abspath ..)
PROFILE := $(ROOT)/deploy/profiles/things-baseline
ENV_FILE ?= $(PROFILE)/.env.local
EXAMPLE_ENV := $(PROFILE)/.env.example
CONTRACT := $(PROFILE)/contract.yaml
TOOLS_DIR := $(ROOT)/backend/tools
BASELINE := cd $(TOOLS_DIR) && GOWORK=off go run ./cmd/thingsstartbaseline
COMPOSE := THINGS_BASELINE_ENV_FILE=$(ENV_FILE) docker compose --project-directory $(PROFILE) --env-file $(ENV_FILE) -f $(PROFILE)/compose.yaml

.PHONY: things-baseline-build things-baseline-image things-baseline-up things-baseline-verify things-baseline-logs things-baseline-down things-baseline-fast

things-baseline-build:
	$(MAKE) -C $(ROOT)/backend/saas build.baseline
	$(MAKE) -C $(ROOT)/backend/things build.baseline
	$(BASELINE) manifest --contract $(CONTRACT) --repo $(ROOT) --bin-dir $(ROOT)/backend/bin --output $(ROOT)/deploy/.runtime/build-manifest.json

things-baseline-image: things-baseline-build
	$(BASELINE) stage --contract $(CONTRACT) --repo $(ROOT) --env-file $(ENV_FILE) --runtime-root $(ROOT)/deploy/.runtime
	$(BASELINE) image --contract $(CONTRACT) --repo $(ROOT) --runtime-root $(ROOT)/deploy/.runtime

things-baseline-up: things-baseline-image
	$(BASELINE) up --contract $(CONTRACT) --repo $(ROOT) --env-file $(ENV_FILE) --compose-file $(PROFILE)/compose.yaml --runtime-root $(ROOT)/deploy/.runtime

things-baseline-verify:
	$(BASELINE) verify --contract $(CONTRACT) --repo $(ROOT) --env-file $(ENV_FILE) --compose-file $(PROFILE)/compose.yaml --runtime-root $(ROOT)/deploy/.runtime

things-baseline-logs:
	$(BASELINE) logs --contract $(CONTRACT) --repo $(ROOT) --env-file $(ENV_FILE) --compose-file $(PROFILE)/compose.yaml --runtime-root $(ROOT)/deploy/.runtime

things-baseline-down:
	$(COMPOSE) down

things-baseline-fast: things-baseline-build
	cd $(TOOLS_DIR) && GOWORK=off go test ./internal/thingsstartbaseline ./cmd/thingsstartbaseline -count=1
	THINGS_BASELINE_ENV_FILE=$(EXAMPLE_ENV) docker compose --project-directory $(PROFILE) --env-file $(EXAMPLE_ENV) -f $(PROFILE)/compose.yaml config >/dev/null
	! rg -n '(^|[/@])iThings([?[:space:]/]|$$)|DROP DATABASE|DROP ROLE|TRUNCATE' $(PROFILE)/compose.yaml $(PROFILE)/config $(PROFILE)/scripts
	! rg -n '(POSTGRES_ADMIN_PASSWORD|THINGS_BASELINE_DB_PASSWORD|CORE_USER_TOKEN_SECRET|THINGS_INTERNAL_AUTH_PASSWORD|THINGS_EMQX_API_KEY|THINGS_EMQX_SECRET_KEY)=[^[:space:]]+' $(EXAMPLE_ENV)
```

注意：`image` 子命令必须继续符合 Task 1 的命令白名单与退出码合同；不能只改 Makefile。

- [ ] **Step 3：记录新旧部署入口**

`deploy/README.md` 必须明确：

- `make -C deploy things-baseline-*` 是新的 Things 基线入口；
- 旧 `deploy/docker-compose.yml` 和 `deploy/saas/Dockerfile` 本轮保留但不被新入口调用；
- `down` 保留数据库和数据卷；
- reset 不属于默认命令，禁止自动运行；
- `.env.local` 创建与权限命令为 `install -m 0600 .env.example .env.local`，复制后必须替换所有空密钥。

`backend/tools/README.md` 记录七个子命令及职责：合同校验、产物证明、staging、镜像构建、有序启动、独立验证和脱敏日志导出；同时说明 evidence 由启动与验证流程统一产生。

CLI 与 Makefile 的合同测试还必须证明：不存在 `reset` 子命令或 `things-baseline-reset` 目标，`things-baseline-down` 不传 `--volumes`，也不执行任何数据库 SQL。本 Loop 以“没有隐式或半成品 reset 能力”实现精确删除边界；若未来新增 reset，必须另建带破坏性操作审批的 Todo。

- [ ] **Step 4：运行 Fast 门禁**

```bash
make -C deploy things-baseline-fast
```

期望：Go 测试与 Compose 解析全部通过，不启动任何容器。

- [ ] **Step 5：提交检查点**

Fast 门禁和范围检查通过后可创建 checkpoint commit，建议提交信息：`feat: add unified things baseline commands`；不创建逐任务用户 Gate，随后直接进入 Task 9。

### Task 9：在当前 WSL 执行冷启动、暖启动和故障矩阵

**文件：**

- 生成但忽略：`deploy/.runtime/**`
- 验收后由用户选择写入：`ming-iot-rd-docs/evidence/<date>-things-reproducible-startup-baseline/evidence.md`

- [ ] **Step 1：创建真实密钥文件并预检**

```bash
cd deploy/profiles/things-baseline
install -m 0600 .env.example .env.local
```

人工填入现有 PostgreSQL 管理连接和随机基线密钥。随后运行：

```bash
make -C deploy things-baseline-fast
```

期望：不输出任何密钥或完整 DSN。

- [ ] **Step 2：执行冷启动**

仅对精确基线资源执行一次经用户批准的清理，然后：

```bash
make -C deploy things-baseline-up
```

期望：十个阶段全部 `PASS`；PostgreSQL 当前数据库为 `ming_iot_things_baseline`；五个业务容器使用五个不同镜像；API 非 5xx；EMQX 中存在 `dg.rpc`。

- [ ] **Step 3：执行暖启动**

```bash
make -C deploy things-baseline-down
make -C deploy things-baseline-up
```

期望：保留数据库与卷后再次 `PASS`，bootstrap 幂等，不出现重复角色、重复数据库或不可恢复迁移错误。

- [ ] **Step 4：验证 `iThings` 硬保护**

使用权限为 `0600` 的临时 env 副本把目标数据库名改成 `iThings`，只运行 precheck：

```bash
umask 077
sed 's/^THINGS_BASELINE_DB=.*/THINGS_BASELINE_DB=iThings/' \
  deploy/profiles/things-baseline/.env.local \
  >/tmp/things-baseline-protected-db.env
cd backend/tools
GOWORK=off go run ./cmd/thingsstartbaseline contract \
  --contract ../../deploy/profiles/things-baseline/contract.yaml \
  --env-file /tmp/things-baseline-protected-db.env
```

期望：退出码 1，错误码 `CONTRACT_MISMATCH`，且没有启动容器或执行 SQL。

- [ ] **Step 5：逐项验证依赖失败**

分别停止 Redis、TDengine、EMQX，再运行 verify；每次测试后先恢复组件并等其健康，再进行下一项：

```bash
THINGS_BASELINE_ENV_FILE="$(pwd)/deploy/profiles/things-baseline/.env.local" \
docker compose --project-directory deploy/profiles/things-baseline --env-file deploy/profiles/things-baseline/.env.local -f deploy/profiles/things-baseline/compose.yaml stop redis
make -C deploy things-baseline-verify
```

期望：Redis 和 TDengine 返回 `DEPENDENCY_NOT_READY`；EMQX 停止后 dg 进程存在也不能通过 `DG_LINK_READY`。

- [ ] **Step 6：检查证据与 Git 边界**

```bash
rg -n 'postgres://[^[:space:]]+:[^[:space:]]+@|THINGS_EMQX_SECRET_KEY=' deploy/.runtime/runs
git status --short
```

期望：敏感扫描无命中；运行产物不出现在 Git status；只有计划内跟踪文件变化。

- [ ] **Step 7：记录验收结果并进入最终复核**

此任务不在中途创建用户 Gate，也不自动把原始运行证据写入 Git。将 run ID、阶段摘要、失败矩阵和 `git status --short` 写入 LoopX 私有 Evidence 后直接进入 Task 10；最终用户 Gate 再决定是否把选定的脱敏摘要写入 `ming-iot-rd-docs/evidence/`。凭据写入和精确基线资源的破坏性清理仍按独立权限边界处理。

### Task 10：最终验证、独立 Review 与 LoopX 写回

**文件：**

- 只读复核：本计划列出的全部实现文件
- 用户批准后新建：`ming-iot-rd-docs/reviews/2026-08-xx-things-reproducible-startup-baseline-review.md`

- [ ] **Step 1：运行最终 Fast 验证**

```bash
make -C deploy things-baseline-fast
cd backend/tools
GOWORK=off go test -race ./internal/thingsstartbaseline ./cmd/thingsstartbaseline -count=1
GOWORK=off go vet ./internal/thingsstartbaseline ./cmd/thingsstartbaseline
```

期望：全部成功。

- [ ] **Step 2：运行最终 Full 验证**

```bash
make -C deploy things-baseline-up
make -C deploy things-baseline-verify
```

期望：最终 evidence 为 `PASS`，十个阶段齐全。

- [ ] **Step 3：请求独立代码 Review**

使用 `superpowers:requesting-code-review`，Reviewer 必须逐项核对：

1. 五个业务镜像是否一一对应五个二进制；
2. `iThings` 是否在代码、脚本和运行路径中被硬保护；
3. 是否存在业务服务源码越界修改；
4. 探针是否证明协议可用而不只是进程存在；
5. evidence 是否能从规格追溯到命令、镜像和二进制；
6. 日志和证据是否泄露密钥；
7. Full 失败注入是否确实 fail-closed。

- [ ] **Step 4：展示最终 Git 范围**

```bash
git status --short
git diff --stat
git diff --check
```

若出现计划外文件，停止并请用户处理。不得通过删除用户文件来获得干净状态。

- [ ] **Step 5：完成技术 Todo 后创建总体验收 Gate**

Fast、Full、独立 Review 和最终 Git 范围检查全部完成后，先完成 Things 启动基线技术 Todo，再创建唯一的总体验收用户 Gate。该 Gate 展示最终 Evidence、Review、checkpoint commit 列表和剩余风险；commit、merge、push 仍按 Gate 中的精确动作分别授权。不得用“测试跑过”替代独立 Review 或最终用户批准。

## 3. 规格覆盖矩阵

| 规格要求 | 实施任务 |
|---|---|
| 五二进制、五镜像 | Task 2、Task 4 |
| 根级 `deploy/` 统一入口 | Task 8 |
| 独立 PostgreSQL 数据库与 `iThings` 保护 | Task 3、Task 5、Task 9 |
| Redis、etcd、NATS、TDengine、EMQX 真实依赖 | Task 5、Task 7、Task 9 |
| timedjob → sys → dm → dg → api 顺序 | Task 6 |
| 协议级就绪与 dg 链路 | Task 7 |
| 稳定失败码与有限超时 | Task 1、Task 6、Task 7 |
| 脱敏 evidence | Task 7、Task 9 |
| Fast / Full 分层 | Task 8、Task 9、Task 10 |
| 不改业务代码 | Task 0、Task 10 |
| Review 与用户批准不互相替代 | Task 10 |

## 4. 官方协议依据

- [EMQX REST API 与 bootstrap API key](https://docs.emqx.com/en/emqx/latest/admin/api.html)
- [EMQX 配置环境变量映射](https://docs.emqx.com/en/emqx/latest/configuration/configuration.html)
- [EMQX Docker 健康检查](https://docs.emqx.com/en/emqx/latest/deploy/install-docker.html)
- [TDengine Docker 部署与 SQL 验证](https://docs.taosdata.com/operations-and-tooling/operations/deployment/docker/)
- [taosAdapter `/-/ping`](https://docs.taosdata.com/reference/components/taosadapter/)
- [etcd endpoint health](https://etcd.io/docs/v3.5/tutorials/how-to-check-cluster-status/)
- [NATS 监控端点](https://docs.nats.io/running-a-nats-service/nats_admin/monitoring)
