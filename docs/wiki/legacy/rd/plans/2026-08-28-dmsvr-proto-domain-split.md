# dmsvr Proto 按领域兼容拆分实施计划

> **面向 Agent 执行者：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans` 逐项实施。步骤使用复选框跟踪。Plan Task 只是一个 Agent Todo 内部的工程检查点，不得为每个 Task 新建 LoopX Todo 或 User Gate。

**目标：** 在不改变 dmsvr Proto/gRPC、Go 公共 API、direct adapter、手工 `dmdirect` 装配和运行时注册语义的前提下，把单体 `dm.proto` 按批准的领域边界拆分，并建立可重复、可回滚、可证明的生成链。

**架构：** 先泛化现有 `rpcgen` 和 `clientcontract`，让 SaaS manifest v1 与 Things manifest v2 共存；再按规格中的 261 条消息归属手工拆分 Proto。所有生成发生在临时 candidate 中，依次通过工具链锁定、Proto 图、descriptor、Go API、direct、注册集合、旧 Share 路径、allowlist 和编译检查后，才由 `saastxn` 原子同步到业务树。

**技术栈：** Go 1.24.4、Protocol Buffers、联犀 goctl fork、Go AST、YAML v3、`protocompat`、`clientcontract`、`saastxn`、GNU Make、Things 可重复启动基线、LoopX。

---

## 0. 权威输入、授权和执行边界

实施依据是 [dmsvr Proto 按领域兼容拆分激活规格](../specs/2026-08-28-dmsvr-proto-domain-split-activation-design.md)。消息归属、import DAG、10 个 service、170 个 RPC、261 个消息、9 个运行时注册、`SchemaManage` 保持未注册、联犀 goctl 与手工 `dmdirect` 的边界，以该规格为准；本计划不得重新解释这些决策。

实施前只建立一次精确 User Gate。Gate 批准后，`ming-peer-01` 可以在隔离分支 `dmsvr-proto-domain-split` 内：

- 修改本计划列出的代码和生成物；
- 运行非破坏性的本地测试、构建、冷启动和暖启动；
- 在每个 Task 验证和范围检查通过后创建 checkpoint commit；
- 生成脱敏 Evidence，并把最终 diff 交给 `ming-review-claude` 独立 Review。

Gate 不授权 fast-forward merge、push、生产动作、数据库/volume 删除、历史重写或凭据变更。最终技术验收只开一次总体 User Gate；commit、merge、push 仍按项目规则分别授权。

实施期间固定以下约束：

1. `things/` 和 `iot-doc/` 永久只读；
2. `.loopx/`、`.codex/` 只由项目根 LoopX 维护，不复制到 worktree；
3. `schemaInfo.proto`、`dmdirect/**`、`etc/**`、logic、repo、model 均不得修改；
4. `backend/tools/goctl.lock` 不在本次写范围；若其摘要与实际二进制不符，停止并另开工具链修复 Todo；
5. Task 之间不创建普通 User Gate，遇到新授权边界才停止；
6. candidate 失败不得改变 live tree。

## 1. 文件责任图

### 1.1 新建工具文件

```text
backend/tools/internal/rpcgen/protograph.go          # descriptor 文件图、aggregate/public import 和环校验
backend/tools/internal/rpcgen/protograph_test.go     # manifest v2 Proto 图失败路径
backend/tools/internal/rpcgen/goctl.go               # backend/bin/goctl 路径与 lock 摘要校验
backend/tools/internal/rpcgen/goctl_test.go           # PATH 回退、摘要漂移和缺失二进制测试
backend/tools/internal/rpcgen/compat.go              # live/candidate 兼容快照与比较
backend/tools/internal/rpcgen/compat_test.go          # API/direct/注册/禁止修改合同测试
backend/tools/internal/rpcgen/generate_test.go        # candidate allowlist、失败回滚和二次零 diff 测试
backend/tools/internal/rpcgen/descriptor_test.go      # descriptor 集合和计数测试
backend/tools/cmd/rpcgen/main_test.go                 # --module-root 与 --saas 兼容入口测试
backend/things/service/dmsvr/proto/rpcgen.yaml        # Things manifest v2
backend/things/service/dmsvr/proto/dmShare.proto
backend/things/service/dmsvr/proto/protocolModel.proto
backend/things/service/dmsvr/proto/deviceManage.proto
backend/things/service/dmsvr/proto/productManage.proto
backend/things/service/dmsvr/proto/schemaManage.proto
backend/things/service/dmsvr/proto/protocolManage.proto
backend/things/service/dmsvr/proto/deviceGroup.proto
backend/things/service/dmsvr/proto/remoteConfig.proto
backend/things/service/dmsvr/proto/deviceMsg.proto
backend/things/service/dmsvr/proto/deviceInteract.proto
backend/things/service/dmsvr/proto/otaManage.proto
backend/things/service/dmsvr/proto/userDevice.proto
```

### 1.2 修改工具文件

```text
backend/tools/internal/rpcgen/manifest.go             # v1/v2 严格解析、规范化与公共路径模型
backend/tools/internal/rpcgen/manifest_test.go        # v1 回归与 v2 字段/重复/路径校验
backend/tools/internal/rpcgen/descriptor.go           # 统一生成 descriptor 并调用 Proto 图校验
backend/tools/internal/rpcgen/registry.go             # selector 与 registration 策略
backend/tools/internal/rpcgen/registry_test.go        # required/preserve_absent 精确集合
backend/tools/internal/rpcgen/aliases.go              # 从 live API 快照补齐 alias，不写死 sys
backend/tools/internal/rpcgen/generate.go             # module/runtime 路径泛化、candidate 验证与原子同步
backend/tools/internal/clientcontract/contract.go     # 可配置 PB selector
backend/tools/internal/clientcontract/contract_test.go# sys/dm/非法 selector
backend/tools/cmd/rpcgen/main.go                      # --module-root，新入口和 v1 别名
backend/tools/README.md                               # 两种 manifest 和固定工具链命令
```

### 1.3 手工和生成文件

`backend/things/service/dmsvr/proto/dm.proto` 改为只含 `syntax`、`package`、`go_package` 和全部 public imports 的聚合入口，不再定义 message、enum 或 service。`pb/dm/**`、十个 client package 和十个 server package 只能由受控生成器更新，禁止手工修补生成结果。

## 2. Task 与 checkpoint 总览

| Task | 有界产物 | 聚焦验证 | checkpoint 信息 |
| --- | --- | --- | --- |
| 1 | 通用 client selector | clientcontract 单测 | `test: freeze dmsvr client selector contract` |
| 2 | manifest v2、Proto 图、registration 与兼容快照 | rpcgen manifest/graph/registry/compat | `feat: add rpcgen manifest v2 compatibility contracts` |
| 3 | 固定 goctl 和通用 module/runtime 入口 | goctl/CLI/generate 单测 | `feat: attest rpcgen toolchain and module roots` |
| 4 | 领域 Proto 和 Things manifest | descriptor/归属/import DAG | `refactor: split dmsvr proto by domain` |
| 5 | candidate 生成与全兼容事务 | 失败回滚、allowlist、API/direct | `feat: generate dmsvr rpc artifacts transactionally` |
| 6 | 完整生成物与幂等结果 | 二次生成零 diff、全工具测试 | `chore: refresh dmsvr generated rpc artifacts` |
| 7 | 模块构建、冷暖启动与 Evidence | test/race/vet/build/runtime | `test: prove dmsvr proto split compatibility` |
| 8 | 独立 Review 与 LoopX 总体验收 Gate | Reviewer `PASS`、最终范围检查 | 不自动 merge/push |

以上 checkpoint 只有在实施 User Gate 明确批准后才可创建。

## 3. Task 1：泛化 client selector

**文件：**

- 修改：`backend/tools/internal/clientcontract/contract.go`
- 修改：`backend/tools/internal/clientcontract/contract_test.go`

- [ ] **Step 1：为 `sys`、`dm` 和非法 selector 写失败测试**

在 `contract_test.go` 增加：

```go
func TestExtractWithSelectorCapturesOnlySelectedPBAliases(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "device", "device.go", `package device
type A = dm.A
type B = sys.B
type C = other.C
type Device interface { Read(ctx int, in *A) (*A, error) }
func NewDevice(cli int) Device { return nil }
func NewDirectDevice(ctx int, svr int) Device { return nil }
`)
	doc, err := ExtractWithSelector(root, "dm")
	if err != nil { t.Fatal(err) }
	if len(doc.Aliases) != 1 || doc.Aliases[0].Target != "dm.A" {
		t.Fatalf("aliases = %+v", doc.Aliases)
	}
}

func TestExtractWithSelectorRejectsInvalidSelector(t *testing.T) {
	for _, selector := range []string{"", "pb/dm", "dm.x", "9dm"} {
		if _, err := ExtractWithSelector(t.TempDir(), selector); err == nil {
			t.Fatalf("selector %q accepted", selector)
		}
	}
}
```

- [ ] **Step 2：运行测试并确认先失败**

```bash
cd backend/tools
GOWORK=off go test ./internal/clientcontract -run 'TestExtractWithSelector' -count=1
```

期望：因 `ExtractWithSelector` 尚不存在而编译失败。

- [ ] **Step 3：实现兼容入口**

`Extract` 保持旧行为，新增明确 selector 入口：

```go
func Extract(root string) (Document, error) {
	return ExtractWithSelector(root, "sys")
}

func ExtractWithSelector(root, selector string) (Document, error) {
	if !token.IsIdentifier(selector) || selector == "_" {
		return Document{}, fmt.Errorf("invalid PB selector %q", selector)
	}
	// 保留现有目录遍历和排序；把 selector 传给 extractFromPackage。
}
```

`extractFromPackage` 只收集 `selector + "."` 前缀的公开 alias。现有 `Extract` 和 `cmd/clientcontract` 继续保持 SaaS `sys` 默认行为；Things 的 `dm` 快照只由 `rpcgen` 在兼容事务内调用 `ExtractWithSelector` 捕获，不扩大 `cmd/clientcontract` 的写入边界。

- [ ] **Step 4：运行聚焦测试和基线测试**

```bash
cd backend/tools
GOWORK=off go test ./internal/clientcontract -run 'TestExtractWithSelector' -count=1
GOWORK=off go test ./internal/clientcontract ./internal/rpcgen -count=1
```

期望：全部 `ok`，原 `sys` 测试不改调用方式仍通过。

- [ ] **Step 5：范围检查并创建 checkpoint**

```bash
git status --short
git diff --check
git diff --name-only | sort
```

只允许本 Task 列出的工具文件变化，随后创建 `test: freeze dmsvr client selector contract` checkpoint。

## 4. Task 2：实现 manifest v2、Proto 图、registration 与兼容快照

**文件：**

- 修改：`backend/tools/internal/rpcgen/manifest.go`
- 修改：`backend/tools/internal/rpcgen/manifest_test.go`
- 新建：`backend/tools/internal/rpcgen/protograph.go`
- 新建：`backend/tools/internal/rpcgen/protograph_test.go`
- 修改：`backend/tools/internal/rpcgen/descriptor.go`
- 新建：`backend/tools/internal/rpcgen/descriptor_test.go`
- 修改：`backend/tools/internal/rpcgen/registry.go`
- 修改：`backend/tools/internal/rpcgen/registry_test.go`
- 新建：`backend/tools/internal/rpcgen/compat.go`
- 新建：`backend/tools/internal/rpcgen/compat_test.go`

- [ ] **Step 1：写 v1/v2 严格解析测试**

测试必须证明：v1 现有 YAML 原样加载；v2 接受规格字段；未知字段、第二个 YAML 文档、重复 service/proto/packageDir/clientFile/serverFile、非法 registration、缺文件均失败。

```go
func TestLoadManifestV2Strict(t *testing.T) {
	path := writeManifest(t, `version: 2
runtime: dmsvr
modulePath: gitee.com/ming-iot/things
runtimeDir: service/dmsvr
protoDir: service/dmsvr/proto
pbDir: service/dmsvr/pb/dm
package: dm
goPackage: pb/dm
pbSelector: dm
aggregate: dm.proto
supportingProtos: [dmShare.proto, protocolModel.proto, schemaInfo.proto]
registry: {path: service/dmsvr/dmdirect/dmdirect.go, selector: dm}
services:
  - {proto: deviceManage.proto, service: DeviceManage, packageDir: devicemanage, clientFile: deviceManage.go, serverFile: deviceManageServer.go, registration: required}
`)
	m, err := LoadManifest(path)
	if err != nil { t.Fatal(err) }
	if m.Version != 2 || m.ModulePath != "gitee.com/ming-iot/things" || m.PBSelector != "dm" {
		t.Fatalf("manifest = %+v", m)
	}
}
```

- [ ] **Step 2：实现规范化模型**

`Manifest` 增加 `ModulePath`、`RuntimeDir`、`PBSelector`、`SupportingProtos`；registry 使用兼容 scalar/mapping 的 `RegistrySpec`；`ServiceEntry` 增加 `Registration`。

```go
type RegistrySpec struct {
	Path     string `yaml:"path"`
	Selector string `yaml:"selector"`
}

const (
	RegistrationRequired       = "required"
	RegistrationPreserveAbsent = "preserve_absent"
)
```

`LoadManifest` 使用 `yaml.Decoder.KnownFields(true)` 并拒绝第二份文档。v1 规范化默认值固定为：`modulePath=gitee.com/ming-iot/saas`、`runtimeDir=service/syssvr`、`pbSelector=sys`、`supportingProtos=shared`、所有 service 为 `required`。v2 不允许依赖这些默认值补齐缺失字段。

- [ ] **Step 3：写 aggregate/public import 和环测试**

用最小 `descriptorpb.FileDescriptorSet` fixture 覆盖：

- aggregate 定义 message/service；
- aggregate 漏掉一个 public import；
- supporting 与 service Proto 构成环；
- service 在两个文件重复定义；
- 正确 DAG。

```go
func TestCheckProtoGraphAcceptsAggregatePublicImportDAG(t *testing.T) {
	set := descriptorSet(
		file("dm.proto", []string{"dmShare.proto", "deviceManage.proto"}, []int32{0, 1}),
		file("dmShare.proto", nil, nil),
		serviceFile("deviceManage.proto", []string{"dmShare.proto"}, "DeviceManage"),
	)
	if err := CheckProtoGraph(validV2Manifest(), set); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 4：实现 descriptor 驱动的 Proto 图校验**

`protograph.go` 只消费 `FileDescriptorSet`：以 `Dependency` 建图、以 `PublicDependency` 验证 aggregate、以 DFS 三色标记检测环，并检查定义归属。不得用目录 glob 猜测文件。

```go
func CheckProtoGraph(m *Manifest, set *descriptorpb.FileDescriptorSet) error
func TopologicalProtoOrder(m *Manifest, set *descriptorpb.FileDescriptorSet) ([]string, error)
```

返回错误必须包含稳定阶段前缀：`manifest:`、`proto_graph:`、`aggregate:`、`registry:`。

- [ ] **Step 5：先写 registration 策略测试，再泛化 AST selector**

```go
func TestCheckRegistryPreservesAbsentService(t *testing.T) {
	m := validV2Manifest()
	m.Services[0].Registration = RegistrationRequired
	m.Services[1].Registration = RegistrationPreserveAbsent
	path := writeRegistry(t, `package dmdirect
func register() { dm.RegisterDeviceManageServer(grpcServer, svcCtx) }
`)
	result, err := CheckRegistry(path, m)
	if err != nil { t.Fatal(err) }
	if len(result.MissingRequired) != 0 || len(result.PresentPreserveAbsent) != 0 { t.Fatalf("%+v", result) }
}
```

`CheckRegistry` 和 `RegistryServices` 从 `m.Registry.Selector` 读取 AST selector。v1 的 `sys` 行为保持；v2 required 必须出现，preserve_absent 必须不出现，额外注册仍失败。

- [ ] **Step 6：在 v2 路径合同就绪后写兼容快照失败测试**

`compat.go` 定义：

```go
type CompatibilitySnapshot struct {
	Descriptor      protocompat.Contract
	ClientAPI       clientcontract.Document
	Registrations   []string
	DirectMethods   []DirectMethod
	DirectFacades   []string
	ProtectedSHA256 map[string]string
}

type DirectMethod struct {
	Package   string
	Receiver  string
	Name      string
	CallsSVR  string
}

func CaptureCompatibility(moduleRoot string, m *Manifest, protobufToolsDir string) (CompatibilitySnapshot, error)
func CompareCompatibility(before, after CompatibilitySnapshot) error
```

先用 v2 manifest fixture 写 RED 测试，固定：10 个构造函数、一个 direct receiver 调用 `d.svr.Read`、一个 facade `NewDevice(bool)`、9 个注册和一个受保护文件摘要。`CompareCompatibility` 必须对规范化排序后的值精确比较，并在 `ProtectedSHA256` 变化时返回包含相对路径的错误。

- [ ] **Step 7：实现 AST 捕获，不比较函数体以外的业务实现**

direct 方法只接受以下结构为“直接转发”：

```go
func (d *directDevice) Read(ctx context.Context, in *dm.ReadReq, opts ...grpc.CallOption) (*dm.ReadResp, error) {
	return d.svr.Read(ctx, in)
}
```

捕获 receiver、方法名和被调用 selector；任何额外控制流、调用其他 receiver 或缺少 `return d.svr.Read(...)` 这一类同名直调结构都失败。facade 只捕获 `RuntimeDir/dmdirect` 中导出的 `New*` 函数签名，不把其函数体纳入生成器所有权。所有目录和 selector 均来自已验证的 v2 manifest；不得写死 dmsvr 路径。v1 SaaS 回归只验证旧行为，不强制提供 v2 compatibility 配置。

- [ ] **Step 8：运行聚焦和全包测试**

```bash
cd backend/tools
GOWORK=off go test ./internal/rpcgen -run 'Test(LoadManifestV2|CheckProtoGraph|CheckRegistry|CaptureCompatibility|CompareCompatibility)' -count=1
GOWORK=off go test ./internal/rpcgen ./internal/clientcontract -count=1
```

- [ ] **Step 9：范围检查并创建 checkpoint**

执行 `git status --short`、`git diff --check` 和文件白名单比对，创建 `feat: add rpcgen manifest v2 compatibility contracts` checkpoint。

## 5. Task 3：固定联犀 goctl 并泛化 module/runtime 生成入口

**文件：**

- 新建：`backend/tools/internal/rpcgen/goctl.go`
- 新建：`backend/tools/internal/rpcgen/goctl_test.go`
- 修改：`backend/tools/internal/rpcgen/generate.go`
- 新建：`backend/tools/internal/rpcgen/generate_test.go`
- 修改：`backend/tools/cmd/rpcgen/main.go`
- 新建：`backend/tools/cmd/rpcgen/main_test.go`
- 修改：`backend/tools/README.md`

- [ ] **Step 1：写路径和摘要失败测试**

```go
func TestAttestGoctlRejectsPATHBinaryAndDigestDrift(t *testing.T) {
	backend := t.TempDir()
	writeExecutable(t, filepath.Join(backend, "bin", "goctl"), "locked")
	writeGoctlLock(t, filepath.Join(backend, "tools", "goctl.lock"), sha256Hex("different"))
	t.Setenv("PATH", filepath.Join(t.TempDir(), "bin")+":"+os.Getenv("PATH"))
	_, err := AttestGoctl(backend)
	if err == nil || !strings.Contains(err.Error(), "binarySha256") { t.Fatalf("err = %v", err) }
}
```

另测：二进制缺失、lock 缺 repository/commit/goVersion/target/binarySha256、相对路径逃逸、PATH 中存在伪 goctl 但 `backend/bin/goctl` 正确时仍只返回锁定路径。

- [ ] **Step 2：实现最小 lock attestation**

```go
type GoctlAttestation struct {
	BinaryPath  string
	BinarySHA256 string
	Repository  string
	Commit      string
	GoVersion   string
	GOOS        string
	GOARCH      string
	CGOEnabled  bool
	VersionText string
}

func AttestGoctl(backendRoot string) (GoctlAttestation, error)
```

函数只读取参数 `backendRoot` 下的 `tools/goctl.lock` 和 `bin/goctl`，校验 SHA-256，执行 `bin/goctl` 的绝对路径并用 `--version` 采集版本文本。repository、commit、toolchain 和 target 从 lock 进入 Evidence；binary SHA 是实际执行身份。不得调用 `exec.LookPath`，不得回退到 PATH。

- [ ] **Step 3：写 CLI 兼容测试**

命令合同：

```text
rpcgen check    --manifest FILE --module-root backend/things [--protobuf-tools DIR]
rpcgen generate --manifest FILE --module-root backend/things [--protobuf-tools DIR]
rpcgen check    --manifest FILE --saas backend/saas
rpcgen generate --manifest FILE --saas backend/saas --goctl backend/bin/goctl --protobuf-tools backend/bin/protobuf-tools
```

`--module-root` 与 `--saas` 同时出现必须退出 2；v2 禁止任意 `--goctl` 覆盖；v1 提供 `--goctl` 时其规范化绝对路径必须等于锁定路径。

- [ ] **Step 4：删除硬编码 syssvr/saas/sys 路径**

统一派生：

```go
func (m *Manifest) RuntimeRoot(moduleRoot string) string {
	return filepath.Join(moduleRoot, m.RuntimeDir)
}
func (m *Manifest) ClientRoot(moduleRoot string) string {
	return filepath.Join(m.RuntimeRoot(moduleRoot), "client")
}
func (m *Manifest) PBImportPath() string {
	return path.Join(m.ModulePath, m.RuntimeDir, m.GoPackage)
}
```

staging `go.mod` 使用 `m.ModulePath`，goctl `--module` 使用相同值；client/server 目的目录从 `RuntimeDir` 计算；alias import 使用 `PBImportPath()` 和 `PBSelector`。SaaS v1 规范化后必须产生与改造前相同路径。

- [ ] **Step 5：运行工具链聚焦测试**

```bash
cd backend/tools
GOWORK=off go test ./internal/rpcgen ./cmd/rpcgen -run 'Test(AttestGoctl|CLI|GenerateUsesModuleRoot|SaaSV1)' -count=1
bash scripts/build-goctl_test.sh
```

若 `backend/bin/goctl` 尚不存在，只运行脚本离线测试；真实构建留到 Task 5，并由 `build-goctl.sh build` 按 lock 生成。

- [ ] **Step 6：范围检查并创建 checkpoint**

确认没有修改 `goctl.lock`，创建 `feat: attest rpcgen toolchain and module roots` checkpoint。

## 6. Task 4：按批准归属手工拆分 Proto

**文件：**

- 修改：`backend/things/service/dmsvr/proto/dm.proto`
- 新建：`backend/things/service/dmsvr/proto/dmShare.proto`
- 新建：`backend/things/service/dmsvr/proto/protocolModel.proto`
- 新建：十个领域 service Proto
- 新建：`backend/things/service/dmsvr/proto/rpcgen.yaml`

- [ ] **Step 1：在临时目录捕获拆分前 descriptor 和计数**

```bash
baseline_dir=/tmp/ming-iot-dmsvr-proto-split-baseline
mkdir -p "$baseline_dir"
backend/bin/protobuf-tools/bin/protoc \
  -I backend/things/service/dmsvr/proto \
  -I backend/bin/protobuf-tools/include \
  --include_imports \
  --descriptor_set_out="$baseline_dir/dmsvr-before.protoset" \
  backend/things/service/dmsvr/proto/dm.proto \
  backend/things/service/dmsvr/proto/schemaInfo.proto
cd backend/tools
GOWORK=off go run ./cmd/protocompat \
  -old "$baseline_dir/dmsvr-before.protoset" \
  -new "$baseline_dir/dmsvr-before.protoset" \
  -package dm \
  -summary-json "$baseline_dir/dmsvr-before-summary.json"
```

期望摘要：10 services、170 methods、267 顶层 messages（`dm.proto` 261 + `schemaInfo.proto` 6）。临时文件不加入 Git。

- [ ] **Step 2：写入 manifest v2**

逐字使用激活规格第 7.1 节的 manifest。固定 `pbSelector: dm`、3 个 supporting Proto、10 个 service 和 registration；`SchemaManage` 唯一使用 `preserve_absent`。

- [ ] **Step 3：建立三个 supporting Proto**

每个文件保持：

```proto
syntax = "proto3";
package dm;
option go_package = "pb/dm";
```

`dmShare.proto` 只移动规格第 5.1 节 8 个跨域基础消息；`protocolModel.proto` 只移动第 5.2 节 3 个协议模型消息；`schemaInfo.proto` 保持原文件原摘要，不移动。

- [ ] **Step 4：建立十个 service Proto**

严格按规格第 5.3 至 5.12 节移动消息和对应 service，不修改任何声明内部文本。每个领域文件只 import 自己字段/请求/响应真实引用的 supporting 或其他领域文件；禁止复制消息解决依赖。

服务顺序固定为：`DeviceManage`、`ProductManage`、`SchemaManage`、`ProtocolManage`、`DeviceGroup`、`RemoteConfig`、`DeviceMsg`、`DeviceInteract`、`OtaManage`、`userDevice`。

- [ ] **Step 5：把 `dm.proto` 改为纯聚合入口**

```proto
syntax = "proto3";
package dm;
option go_package = "pb/dm";

import public "dmShare.proto";
import public "protocolModel.proto";
import public "schemaInfo.proto";
import public "deviceManage.proto";
import public "productManage.proto";
import public "schemaManage.proto";
import public "protocolManage.proto";
import public "deviceGroup.proto";
import public "remoteConfig.proto";
import public "deviceMsg.proto";
import public "deviceInteract.proto";
import public "otaManage.proto";
import public "userDevice.proto";
```

- [ ] **Step 6：运行结构和 descriptor 比较**

```bash
baseline_dir=/tmp/ming-iot-dmsvr-proto-split-baseline
cd backend/tools
GOWORK=off go run ./cmd/rpcgen check \
  --manifest ../things/service/dmsvr/proto/rpcgen.yaml \
  --module-root ../things \
  --protobuf-tools ../bin/protobuf-tools

../bin/protobuf-tools/bin/protoc \
  -I ../things/service/dmsvr/proto \
  -I ../bin/protobuf-tools/include \
  --include_imports \
  --descriptor_set_out="$baseline_dir/dmsvr-after.protoset" \
  ../things/service/dmsvr/proto/dm.proto

GOWORK=off go run ./cmd/protocompat \
  -old "$baseline_dir/dmsvr-before.protoset" \
  -new "$baseline_dir/dmsvr-after.protoset" \
  -package dm \
  -summary-json "$baseline_dir/dmsvr-after-summary.json"
```

期望：`compatible`；10/170/267 计数不变；`CheckProtoGraph` 通过。

- [ ] **Step 7：机械检查唯一归属和禁止变化**

运行 manifest/descriptor 测试，并对 `schemaInfo.proto`、`dmdirect/**` 做前后 SHA-256 比较。任何消息重复、漏归属或受保护文件变化都停止。

- [ ] **Step 8：范围检查并创建 checkpoint**

只允许 13 个 Proto 输入和 `rpcgen.yaml` 变化，创建 `refactor: split dmsvr proto by domain` checkpoint。

## 7. Task 5：实现 candidate 全兼容事务与受控同步

**文件：**

- 修改：`backend/tools/internal/rpcgen/aliases.go`
- 修改：`backend/tools/internal/rpcgen/generate.go`
- 修改：`backend/tools/internal/rpcgen/generate_test.go`
- 修改：`backend/tools/internal/rpcgen/compat.go`
- 修改：`backend/tools/internal/rpcgen/compat_test.go`
- 修改：`backend/tools/cmd/rpcgen/main.go`
- 修改：`backend/tools/README.md`

- [ ] **Step 1：写失败不污染和 allowlist 测试**

```go
func TestGenerateFailureLeavesLiveTreeUnchanged(t *testing.T) {
	root := generationFixture(t)
	before := treeDigest(t, root.ModuleRoot)
	err := Generate(context.Background(), root.OptionsWithFailingGoctl())
	if err == nil { t.Fatal("Generate succeeded") }
	if after := treeDigest(t, root.ModuleRoot); after != before {
		t.Fatalf("live tree changed: before=%s after=%s", before, after)
	}
}

func TestBuildAllowlistExcludesDmdirectLogicAndEtc(t *testing.T) {
	allow := buildAllowlist(validV2Manifest())
	for _, forbidden := range []string{"service/dmsvr/dmdirect/", "service/dmsvr/internal/logic/", "service/dmsvr/etc/"} {
		if pathAllowed(allow, forbidden) { t.Fatalf("allowed %s", forbidden) }
	}
}
```

- [ ] **Step 2：把 baseline 快照传入 candidate**

`Generate` 改为 options 结构，禁止参数位置继续膨胀：

```go
type GenerateOptions struct {
	Manifest         *Manifest
	ModuleRoot       string
	BackendRoot      string
	ProtobufToolsDir string
	Goctl            GoctlAttestation
}

func Generate(ctx context.Context, opts GenerateOptions) error
```

事务前捕获 live `CompatibilitySnapshot`；candidate 生成后捕获同一快照并比较。alias 补齐使用 live `ClientAPI`，不要求 Things 提交第二份手工 golden；SaaS v1 仍从现有 `clientContractGolden` 读取并与 live 快照交叉检查。

- [ ] **Step 3：按 module/runtime 构造 staging**

每个 service 独立 staging；`go.mod` 内容来自 manifest：

```go
func stagingGoMod(modulePath string) []byte {
	return []byte("module " + modulePath + "\n\ngo 1.24\n")
}
```

goctl 命令只能使用 `opts.Goctl.BinaryPath`。生成后只收集 manifest 指定的 client/server 文件；PB 由一次 protoc 生成。main、etc、startup、logic、svc、config、repo 等 staging 文件全部丢弃。

- [ ] **Step 4：增加旧 Share 路径和 protected tree 扫描**

candidate 通过同步前执行：

```go
func RejectForbiddenImports(root string, forbidden []string) error
func VerifyProtectedDigests(before, after map[string]string) error
```

禁止字符串至少包含 `gitee.com/unitedrhino/share`。protected 摘要至少覆盖 `schemaInfo.proto`、`dmdirect/**`、`etc/**`、`internal/logic/**`、`internal/repo/**`。

- [ ] **Step 5：按固定顺序执行 candidate Gate**

顺序不可交换：

1. manifest 与 Proto 图；
2. goctl attestation；
3. protoc/goctl 生成；
4. forbidden import；
5. descriptor compatibility；
6. client API 与 direct adapter；
7. 9 个 runtime 注册和 `SchemaManage` 缺席；
8. protected digest；
9. candidate `go test -run '^$' ./...`；
10. `saastxn` allowlist；
11. 原子同步。

错误使用稳定前缀，例如 `stage=manifest:`、`stage=goctl_attestation:`、`stage=descriptor:`、`stage=allowlist:`，并带目标相对路径；不得只返回底层 `exit status 1`。

- [ ] **Step 6：运行事务测试和 SaaS v1 回归**

```bash
cd backend/tools
GOWORK=off go test ./internal/rpcgen -run 'Test(Generate|BuildAllowlist|RejectForbiddenImports|SaaSV1)' -count=1
GOWORK=off go test ./internal/saastxn ./internal/protocompat ./internal/clientcontract ./internal/rpcgen -count=1
```

- [ ] **Step 7：范围检查并创建 checkpoint**

本 Task 不应产生业务生成物；创建 `feat: generate dmsvr rpc artifacts transactionally` checkpoint。

## 8. Task 6：生成正式 PB/client/server 并证明幂等

**文件：**

- 生成：`backend/things/service/dmsvr/pb/dm/**`
- 生成：第 1.3 节 manifest 声明的十个 `backend/things/service/dmsvr/client/` 子包
- 生成：第 1.3 节 manifest 声明的十个 `backend/things/service/dmsvr/internal/server/` 子包

- [ ] **Step 1：按 lock 构建工具链**

```bash
bash backend/tools/scripts/install-protobuf-tools.sh install
bash backend/tools/scripts/build-goctl.sh build
sha256sum -c <(printf '%s  %s\n' "$(awk '/^binarySha256:/ {print $2}' backend/tools/goctl.lock)" backend/bin/goctl)
```

`build-goctl.sh build` 必须校验 repository、commit、patch、Go 版本、target、source archive 和 binary SHA。任何失败停止，不修改 lock。

- [ ] **Step 2：运行正式生成**

```bash
cd backend/tools
GOWORK=off go run ./cmd/rpcgen generate \
  --manifest ../things/service/dmsvr/proto/rpcgen.yaml \
  --module-root ../things \
  --protobuf-tools ../bin/protobuf-tools
```

期望输出包含实际 goctl 绝对路径、SHA-256、10 services 和原子同步成功阶段。

- [ ] **Step 3：验证生成合同**

```bash
GOWORK=off go run ./cmd/rpcgen check \
  --manifest ../things/service/dmsvr/proto/rpcgen.yaml \
  --module-root ../things \
  --protobuf-tools ../bin/protobuf-tools
GOWORK=off go test ./internal/rpcgen \
  -run '^TestDMSVRGeneratedCompatibility$' \
  -count=1
```

`TestDMSVRGeneratedCompatibility` 使用 manifest 和 `CaptureCompatibility` 读取当前真实生成树。机器断言：10 interfaces、170 methods、20 constructors、2620 aliases、10 `NewDirect*`、170 direct methods；`dmdirect` 10 个 facade；注册恰为 9 且不含 `SchemaManage`。测试只读业务树，不保存第二份手工 golden。

- [ ] **Step 4：第二次生成必须零 diff**

先记录第一次生成后的 `git diff --binary` 摘要，再执行同一生成命令。第二次前后：

```bash
git status --porcelain=v1 > /tmp/dmsvr-status-before-second.txt
# 再次执行 rpcgen generate
git status --porcelain=v1 > /tmp/dmsvr-status-after-second.txt
diff -u /tmp/dmsvr-status-before-second.txt /tmp/dmsvr-status-after-second.txt
```

期望 `diff` 无输出。注意比较的是同一未提交工作树的状态投影，而不是要求工作树本身 clean。

- [ ] **Step 5：执行禁止范围检查**

```bash
! rg -n 'gitee\.com/unitedrhino/share' \
  backend/things/service/dmsvr/pb/dm \
  backend/things/service/dmsvr/client \
  backend/things/service/dmsvr/internal/server
git diff --exit-code -- backend/things/service/dmsvr/dmdirect backend/things/service/dmsvr/etc backend/things/service/dmsvr/internal/logic backend/things/service/dmsvr/internal/repo
git diff --check
```

- [ ] **Step 6：创建生成物 checkpoint**

只在所有合同和二次零 diff 通过后创建 `chore: refresh dmsvr generated rpc artifacts` checkpoint。

## 9. Task 7：全量验证、运行时回归和 Evidence

**文件：**

- 修改：`backend/tools/README.md`
- 私有 Evidence：LoopX runtime/evidence 目录，不提交原始日志
- 可选脱敏摘要：只有用户在总体验收 Gate 后另行批准才写入 `ming-iot-rd-docs/evidence/`

- [ ] **Step 1：运行工具测试、race 和 vet**

```bash
cd backend/tools
GOWORK=off go test ./internal/rpcgen ./internal/clientcontract ./internal/protocompat ./internal/saastxn ./cmd/rpcgen ./cmd/clientcontract ./cmd/protocompat -count=1
GOWORK=off go test -race ./internal/rpcgen ./internal/clientcontract ./internal/protocompat ./internal/saastxn -count=1
GOWORK=off go vet ./internal/rpcgen ./internal/clientcontract ./internal/protocompat ./internal/saastxn ./cmd/rpcgen ./cmd/clientcontract ./cmd/protocompat
```

- [ ] **Step 2：运行 Things 模块测试和构建**

```bash
cd backend/things
GOWORK=off go test ./service/dmsvr/... -count=1
GOWORK=off go test ./service/dgsvr/... ./service/apisvr/... -run '^$' -count=1
GOWORK=off go build ./service/dmsvr ./service/dgsvr ./service/apisvr
```

若入口包路径不是上述目录，使用当前 Makefile 已声明的对应构建目标；不得通过修改业务包适配测试。

- [ ] **Step 3：运行 SaaS v1 生成回归**

```bash
cd backend/tools
GOWORK=off go run ./cmd/rpcgen check \
  --manifest ../saas/service/syssvr/proto/rpcgen.yaml \
  --saas ../saas \
  --protobuf-tools ../bin/protobuf-tools
```

在临时 SaaS 副本执行一次 v1 generate，比较生成前后公共 API 与 allowlist；不得把 SaaS 生成差异同步进本分支。

- [ ] **Step 4：运行 fast、cold、warm 回归**

```bash
make -C deploy things-baseline-fast
make -C deploy things-baseline-up
make -C deploy things-baseline-verify
make -C deploy things-baseline-up
make -C deploy things-baseline-verify
```

第一组 `up + verify` 是受保护 cold run，第二组是 warm run。不得执行 reset、删除数据库、删除 volume 或使用 `docker compose down -v`。

- [ ] **Step 5：生成脱敏 Evidence 索引**

Evidence 必须按下表从机器读取，不允许手工填写摘要值：

| Evidence 字段 | 唯一来源 |
| --- | --- |
| `sourceCommit` | `git rev-parse HEAD` |
| `branch` | `git branch --show-current`，必须为 `dmsvr-proto-domain-split` |
| `manifestVersion` | 严格解析后的 `Manifest.Version`，必须为 2 |
| `goctl.path` | `GoctlAttestation.BinaryPath` 的仓库相对路径，必须为 `backend/bin/goctl` |
| `goctl.repository` / `commit` / `binarySha256` | `GoctlAttestation` 与实际二进制摘要；commit 必须为 `6579a20bfd96919a80af5a9cb4e12f03598d4bb3` |
| `counts` | descriptor、clientcontract、direct AST 和 registry AST 的机器计数，必须为 10/170/261/6/10/170/20/2620/9 |
| `protected` | Git diff、SHA-256 和 forbidden import 扫描，结果必须为 `dmdirectChanged=false`、`schemaInfoChanged=false`、`schemaManageRegistered=false`、`oldShareImportCount=0` |
| `idempotence.secondGenerationDiff` | 两次 status 投影的 `diff -u`，必须为 0 |
| `validation` | 本 Task 每条命令的退出码，unit/race/vet/build/cold/warm 均为 `PASS` |

实际 Evidence 还要记录每条命令的退出码、开始/结束时间、脱敏日志摘要和 artifact SHA。

- [ ] **Step 6：最终范围与工作树检查**

```bash
git status --short
git diff --check
git diff --name-only HEAD | sort
git diff --exit-code HEAD -- backend/things/service/dmsvr/dmdirect backend/things/service/dmsvr/etc backend/things/service/dmsvr/internal/logic backend/things/service/dmsvr/internal/repo backend/things/service/dmsvr/proto/schemaInfo.proto
```

实际 diff 必须是激活规格第 10 节 allowlist 的子集。

- [ ] **Step 7：创建验证 checkpoint**

验证通过并写回私有 Evidence 后创建 `test: prove dmsvr proto split compatibility` checkpoint。

## 10. Task 8：独立 Review、LoopX 写回和唯一总体验收 Gate

**文件：**

- Review 输出：本地研发资料或 LoopX 私有 Evidence 引用
- LoopX：项目根 `.loopx/registry.json` 与 active state，只通过 CLI 更新

- [ ] **Step 1：向 `ming-review-claude` 建立一个 Review Todo**

Review Todo 必须绑定 exact head commit，并只要求读取：激活规格、本计划、manifest、最终 diff、Evidence 和关键生成器测试。Reviewer 不继承实现 Agent 的结论，也不得在 Review 中修改实现。

- [ ] **Step 2：Reviewer 执行独立验证**

Review 至少回答：

1. 物理拆分是否保持 descriptor 和 Go API；
2. 联犀 goctl direct 与手工 `dmdirect` 边界是否保持；
3. `SchemaManage` 是否仍未注册；
4. allowlist、失败回滚、二次零 diff 是否有证据；
5. SaaS v1 是否回归；
6. cold/warm 是否是真实依赖探针，而非仅进程存活；
7. 是否存在夹带变更或不可接受剩余风险。

只接受 `PASS`、`BLOCK`、`INCONCLUSIVE`。`BLOCK` 或 `INCONCLUSIVE` 时回到同一 Agent Todo 修复，不创建新的普通 User Gate；任何修复都会使旧 Review 对应 head 失效。

- [ ] **Step 3：完成技术 Todo 前执行最终机器检查**

重新运行 Task 7 的最终验证，不复用修改前结果；确认 Reviewer 的 head 等于当前 `git rev-parse HEAD`，Evidence 中的 manifest 和 diff 摘要也等于当前值。

- [ ] **Step 4：完成 Agent Todo 并创建唯一总体验收 Gate**

LoopX 完成 `todo_b34005e78715` 时附上 Evidence、验证和 Review 引用，并原子创建一个 user_gate。Gate 只请求用户判断是否接受该候选进入后续提交/合并流程；不得把它解释为 merge 或 push 授权。

## 11. 规格覆盖自检

| 激活规格验收项 | 计划覆盖 |
| --- | --- |
| 261 消息唯一归属、DAG 无环 | Task 2、Task 4 |
| 10 service、170 RPC、字段语义 | Task 2、Task 4、Task 5 |
| Go API、direct、dmdirect、9 注册 | Task 2、Task 5、Task 6 |
| 锁定联犀 goctl、禁止 PATH 回退 | Task 3、Task 6 |
| 旧 Share import 零命中 | Task 5、Task 6 |
| SaaS v1 + Things v2 | Task 2、Task 3、Task 7 |
| candidate、allowlist、失败回滚 | Task 5 |
| 第二次生成零 diff | Task 6 |
| test/race/vet/build/cold/warm | Task 7 |
| Evidence 与独立 Review | Task 7、Task 8 |
| 单 Agent Todo、单总体 User Gate | Task 0、Task 8 |
| 不修复 SchemaManage、不改 dmdirect | Task 0、Task 5、Task 7 |

## 12. 停止条件

出现以下任一情况立即停止同一实现路径并向用户/控制器报告，不自行扩大范围：

- 需要修改 `goctl.lock`、联犀 goctl fork、`dmdirect/**`、`schemaInfo.proto`、业务 logic/repo/model 或运行时配置；
- 需要注册 `SchemaManage` 才能通过；
- descriptor 或公共 API 只能通过业务调用方改码才能兼容；
- 实际 Proto 依赖无法满足批准的消息归属和无环 DAG；
- candidate 越过 allowlist，或失败后 live tree 发生变化；
- cold/warm 需要破坏性清理、凭据变更或生产访问；
- Reviewer 发现规格与当前实现边界冲突。

此时保留所有已验证 checkpoint 和脱敏 Evidence，更新同一个 Agent Todo 为具体 blocker；不要用“先做完再补批准”的方式继续。
