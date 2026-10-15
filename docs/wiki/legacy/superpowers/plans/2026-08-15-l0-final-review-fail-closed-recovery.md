# L0 Final Review Fail-Closed Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 L0 最终审查发现的构建 false-PASS、非 YAML 凭据漏扫、source index 漏证、render 日志边界和 evidence 结果矛盾五类问题，重新生成可被独立审查接受的最终证据。

**Architecture:** 保留现有固定源、内存转换、sealed candidate 和 isolation gate 架构，只收紧已批准的 fail-closed 契约。构建 gate 使用已验证的绝对 Go 二进制并对三个本轮产物做前置清理和后置证明；两处固定非 YAML 凭据以 source/output SHA 和精确计数转换；source state 增加 index 字节摘要；render 命令复用统一流式脱敏器；只有结构完整且全部成功的证据才能声明 PASS。

**Tech Stack:** Go 1.25.4、Git plumbing、SHA-256、`openat`/`O_NOFOLLOW`、GNU Make、JSON/YAML、现有 `thingsbaseline` renderer/verifier。

---

## 执行约束

- worktree：`/home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/l0-things-migration-baseline`。
- 官方源 `/home/wangjunbo/workspace-iot-platform/things` 始终只读。
- 不运行 `go work sync`；所有 tools Go 命令使用 `GOWORK=off`。
- 不直接编辑生成目标中的单个文件；所有目标变化由迁移器整树重建。
- 不执行 `git add`、`git commit` 或 `git push`。
- 当前 canonical PASS 是“命令实际通过、最终审查拒绝”的历史事实；覆盖前保存为 attempt 3，不把它继续称为权威最终证据。

## 文件结构

- Modify: `backend/tools/internal/thingsbaseline/verify.go`
- Modify: `backend/tools/internal/thingsbaseline/verify_test.go`
- Modify: `backend/tools/templates/things-baseline/Makefile`
- Modify: `backend/tools/internal/thingsbaseline/render.go`
- Modify: `backend/tools/internal/thingsbaseline/render_test.go`
- Modify: `backend/tools/internal/thingsbaseline/runner_test.go`
- Modify: `backend/tools/internal/thingsbaseline/contract.go`
- Modify: `backend/tools/internal/thingsbaseline/contract_test.go`
- Modify: `backend/tools/things-baseline.lock`
- Modify: `backend/tools/internal/thingsbaseline/transform.go`
- Modify: `backend/tools/internal/thingsbaseline/transform_test.go`
- Modify: `backend/tools/internal/thingsbaseline/scan.go`
- Modify: `backend/tools/internal/thingsbaseline/scan_test.go`
- Modify: `backend/tools/templates/things-baseline/PROVENANCE.yaml`
- Modify: `backend/tools/internal/thingsbaseline/source.go`
- Modify: `backend/tools/internal/thingsbaseline/source_test.go`
- Modify: `backend/tools/internal/thingsbaseline/evidence.go`
- Modify: `backend/tools/internal/thingsbaseline/evidence_test.go`
- Generate: `backend/things/**`
- Create: `docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline-attempt-3-review-rejected.json`
- Replace: `docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.json`
- Modify: `docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.md`
- Create outside Git deliverable: `iot-doc/.superpowers/visual-live/content/rd-graph-l0-review-blocked-v4.html`
- Create outside Git deliverable after recovery: `iot-doc/.superpowers/visual-live/content/rd-graph-l0-evidence-v4.html`

### Task 1：让 things-build 不能虚假通过

- [x] **Step 1: 写 RED 测试锁定绝对 Go 与新鲜产物**

在 `verify_test.go` 增加测试，要求默认 build specification：

```go
if !slices.Contains(build.Args, "GO="+dependencies.goBinary) {
    t.Fatalf("build gate does not pin the verified Go binary: %q", build.Args)
}
if build.Outputs == nil || !reflect.DeepEqual(build.Outputs.Names, []string{"thingsvr", "dgsvr", "dmsvr"}) {
    t.Fatalf("build output contract = %#v", build.Outputs)
}
```

增加 runner table test：输出目录预放三个 stale 文件，命令 exit 0 但不生成文件时必须 FAIL；生成 symlink、hardlink、0644、空文件、少一个文件时分别 FAIL；生成三个单链接 0755 regular files 时 PASS，并返回三个排序 artifact SHA-256。

- [x] **Step 2: 运行 RED**

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline -run 'Test(DefaultGateSpecificationsPinBuild|ExecVerificationGateRunnerRejectsMissingOrUnsafeExpectedOutputs)' -count=1 -v
```

Expected: FAIL，`GateSpecification` 尚无 output contract，Make 参数尚未锁定 Go。

- [x] **Step 3: 实现 output contract**

`GateSpecification` 增加内部字段：

```go
type GateOutputContract struct {
    Directory   string
    LogicalRoot string
    Names       []string
    Mode        uint32
}

type GateArtifact struct {
    LogicalPath string `json:"logicalPath"`
    Mode        string `json:"mode"`
    Size        int64  `json:"size"`
    SHA256      string `json:"sha256"`
    GoVersion   string `json:"goVersion"`
}
```

runner 在启动命令前以 no-follow 方式打开真实输出目录，只删除 contract 中三个精确名称；命令 exit 0 后重新 no-follow 打开并验证：名称集合精确、regular、`Nlink==1`、mode `0755`、size 大于 0、读取前后 identity/mode/size 不变。任何 prepare/postcondition 错误都返回 gate FAIL；成功时 `GateResult.Artifacts` 含三个排序条目。

- [x] **Step 4: 锁定 Make 使用的 Go**

模板改为：

```make
GO ?= go
GO_BUILD := GOWORK=off $(GO) build
```

默认 gate 继续调用受信 `/usr/bin/make`，但参数必须包含：

```text
GO=<productionRenderDependencies().goBinary>
BIN_DIR=<backend>/bin
build
```

`goBinary` 已由 `productionRenderDependencies` 预先验证为 `go1.25.4`；artifact evidence 的 `goVersion` 固定来自该已验证值。

- [x] **Step 5: 运行 GREEN**

```bash
cd backend/tools
gofmt -w internal/thingsbaseline/verify.go internal/thingsbaseline/verify_test.go
GOWORK=off go test ./internal/thingsbaseline -run 'Test(DefaultGateSpecificationsPinBuild|ExecVerificationGateRunner)' -count=1
GOWORK=off go test -race ./internal/thingsbaseline -run TestExecVerificationGateRunner -count=1
```

Expected: fake PATH 中的 `go` 不会被调用；缺失/陈旧/危险产物不能 PASS。

### Task 2：SHA 锁定地清除两处非 YAML 凭据值

- [x] **Step 1: 扩展 lock RED**

`TransformContract` 增加并严格校验：

```go
NonYAMLCredentialValueCount          int    `yaml:"nonYamlCredentialValueCount"`
CloudProtocolCredentialSourceSHA256 string `yaml:"cloudProtocolCredentialSourceSha256"`
CloudProtocolCredentialOutputSHA256 string `yaml:"cloudProtocolCredentialOutputSha256"`
ProductSecretSourceSHA256           string `yaml:"productSecretSourceSha256"`
ProductSecretOutputSHA256           string `yaml:"productSecretOutputSha256"`
```

测试拒绝 count 不是 5、任一 SHA 非 64 位小写 hex、未知 YAML 字段。

- [x] **Step 2: 写转换 RED**

固定路径：

```go
const cloudProtocolCredentialPath = "sdk/protocol/cloudProtocol.go"
const productSecretPath = "service/apisvr/etc/init/product/产品列表.json"
```

测试要求：cloud 文件恰好把 1 个 `accessKeyID` 与 1 个 `accessKeySecret` 值替换为
`${THINGS_CLOUD_ACCESS_KEY_ID}` / `${THINGS_CLOUD_ACCESS_KEY_SECRET}`；product JSON 恰好把
3 个 `Info.Secret` 值替换为 `${THINGS_INIT_PRODUCT_SECRET}`。source SHA、count、预存 placeholder、output SHA 任一不符均 fail closed；错误不得包含旧值。

- [x] **Step 3: 写真实 lock 值**

```yaml
  nonYamlCredentialValueCount: 5
  cloudProtocolCredentialSourceSha256: 19d2e755a768f31eacb0584c8af9fa4626ec1c5fa72123a31eb48617cd5bba4f
  cloudProtocolCredentialOutputSha256: 77a86ceeed33b20d2b9732caa809eb96402a8f44c094737bad7e8f890c62e913
  productSecretSourceSha256: 71fade09cc5e2f6dc4d3dabac13d33ee198de6d0dad9d419a005cf218134e7fc
  productSecretOutputSha256: ab26f6f167a094d6dad3f18e95ed52a1c078e37cd4709a671497e63fbf41b603
```

- [x] **Step 4: 接入 TransformTree 与独立反向扫描**

两个 source blob 必须和 private-key/vet/schema fixture 一样在创建 rendered map 前完成原始 SHA 预检。`TransformReport` 增加：

```go
NonYAMLCredentialValues int `json:"nonYamlCredentialValueCount"`
```

scanner 对两个固定输出执行独立验证：placeholder 与计数精确；product 文件必须可解析为 JSON，三个 `Info.Secret` 均为唯一允许 placeholder；cloud 文件中两个原始非 placeholder assignment 为 0。PROVENANCE 增加第七类计数。

- [x] **Step 5: 运行转换与真实源 GREEN**

```bash
cd backend/tools
gofmt -w internal/thingsbaseline/{contract,contract_test,transform,transform_test,scan,scan_test,render,render_test}.go
GOWORK=off go test ./internal/thingsbaseline -run 'Test(ValidateLock|SanitizeNonYAML|TransformTree|SecurityScan|RenderTargetTemplates)' -count=1
THINGS_BASELINE_SOURCE_REPOSITORY=/home/wangjunbo/workspace-iot-platform/things \
THINGS_BASELINE_LOCK="$PWD/things-baseline.lock" \
GOWORK=off go test ./internal/thingsbaseline -run TestTransformRealLockedSourceWhenEnabled -count=1 -v
```

Expected: 报告变为 `94/2726/55/1/1/45/5`，官方源状态不变。

### Task 3：把 Git index 字节摘要纳入 source-unchanged

- [x] **Step 1: 写 source state RED**

`SourceRepositoryState` 期望增加：

```go
IndexSHA256 string `json:"indexSha256"`
```

测试在 HEAD/ref/status 不变时对 fixture tracked file 执行
`git update-index --assume-unchanged tracked.txt`，断言 recaptured state 只因 index digest 改变而不相等；取消、缺 index、symlink index、special file、读中 replace 均失败。

- [x] **Step 2: 实现安全 index fingerprint**

通过同一 hardened Git runner 执行 `git rev-parse --git-path index`，解析 Git 返回路径；以 no-follow 打开最终 index，要求 regular、单链接和合理大小，流式计算完整 SHA-256，并复核读取前后 identity/mode/size。只把 digest 写入 state/evidence，不写本机 index 路径。

- [x] **Step 3: 运行 GREEN**

```bash
cd backend/tools
gofmt -w internal/thingsbaseline/source.go internal/thingsbaseline/source_test.go internal/thingsbaseline/verify_test.go
GOWORK=off go test ./internal/thingsbaseline -run 'TestSourceRepositoryState|TestReadSourceDoesNotRefreshStaleIndex|TestVerify' -count=1
```

Expected: final `source.before == source.after` 同时证明 HEAD/ref/status/index 四项相同。

### Task 4：让 render command 使用统一流式脱敏和 4 KiB 总预算

- [x] **Step 1: 写 RED**

在 `runner_test.go` 产生：stdout failure sentinel、超过 9 KiB stderr、跨 4 KiB 边界的 configured literal、超长 URL userinfo、UTF-8 截断。断言 `CommandReport` 同时保留 stdout/stderr tail，每流不超过 1,900 bytes、合计不超过 3,800 bytes，完整 raw SHA 不变，任何 secret/authority 不泄漏。

- [x] **Step 2: 复用 unifiedDiagnosticRedactor**

`CommandReport` 增加 `StdoutTail`。把 gate 使用的 raw-byte unified redactor 提取为同 package 可复用的有界 capture；render stdout/stderr 各使用 1,900-byte预算。hash 在脱敏前覆盖完整 raw stream，tail 在 raw 坐标上先脱敏后截断，EOF 必须 finalize。

- [x] **Step 3: 运行 GREEN**

```bash
cd backend/tools
gofmt -w internal/thingsbaseline/render.go internal/thingsbaseline/verify.go internal/thingsbaseline/runner_test.go internal/thingsbaseline/verify_test.go
GOWORK=off go test ./internal/thingsbaseline -run 'TestExecGoRunner|TestGateOutputBuffer|TestEvidence' -count=1
GOWORK=off go test -race ./internal/thingsbaseline -run 'TestExecGoRunner|TestGateOutputBuffer' -count=1
```

Expected: evidence 中任一 render command 的两个 tail 总长度不超过 3,800 bytes。

### Task 5：拒绝矛盾或不完整的 PASS evidence

- [x] **Step 1: 写 evidence RED**

table test 从合法 final PASS evidence 逐项构造：缺 phase、phase 顺序错、phase FAIL、缺 gate、gate 顺序错、gate NOT_RUN、exit 非 0、reproducibility/target/source 矛盾，均必须被 `MarshalEvidence` 拒绝。合法 final FAIL 仍允许保留已完成前缀和 NOT_RUN gate；合法 structural NOT_FINAL 必须是 7 个固定 phase 且 gates 为空。

- [x] **Step 2: 实现结果一致性谓词**

`overallResult=PASS` 时要求固定顺序 11 phases：

```text
source-read, render-1, render-2, reproducibility, target-compare,
isolation-copy, command-gates, isolation-unchanged, module-clean,
target-unchanged, source-unchanged
```

固定顺序 5 gates：`tools-test, saas-test, things-test, workspace-test, things-build`；全部必须 `PASS/EXITED/0`。同时要求双 render digest 相同、target differences 0、before/after 相同、source before/after 相同、reproducibility PASS。FAIL evidence 继续允许部分诊断，不能因 validator 过严而丢失失败证据。

- [x] **Step 3: 运行 GREEN**

```bash
cd backend/tools
gofmt -w internal/thingsbaseline/evidence.go internal/thingsbaseline/evidence_test.go
GOWORK=off go test ./internal/thingsbaseline -run TestEvidence -count=1
```

### Task 6：重建、重跑并取得审查接受的 evidence

- [x] **Step 1: 保存当前 review-rejected 证据**

验证 canonical 当前是 PASS、tree `8d2ab9f6…`，然后按原字节复制为 attempt 3；`cmp` 和 SHA 必须一致。该文件记录“旧 verifier 命令通过但最终安全审查拒绝”，不是新的 FAIL JSON。

- [x] **Step 2: 工具层完整门禁**

```bash
cd backend/tools
GOWORK=off go test -count=1 ./...
GOWORK=off go test -race -count=1 ./...
GOWORK=off go vet ./...
```

- [x] **Step 3: 由迁移器整树重建**

把现有 `backend/things` 移到唯一 `/tmp/l0-things-pre-final-review-rebuild`；import 成功后新旧 `diff -qr` 只允许 Makefile、PROVENANCE、cloudProtocol.go、产品列表 JSON 四条路径。验证后精确删除临时备份。

- [x] **Step 4: structural 与 final verify**

先运行 structural-only，要求双 render、target、source/index 均一致；再运行 final verify 覆盖 canonical。最终要求 11 phases 和 5 gates 全 PASS，build gate 含三个 artifact 证明，render tails 合计满足预算，转换报告含非 YAML count 5。

- [x] **Step 5: 更新解释证据和 18081**

Markdown 增加 attempt 3 与五项 final-review recovery；先创建 review-blocked V4 页面，最终复审通过后再创建 evidence V4 页面。页面不得在复审前显示 L0 已闭环。

- [x] **Step 6: 独立复审与最终审计**

把修复交回同一 final reviewer 复审原五项 finding；Critical/Important 未清零不得完成。随后执行 `git diff --check`、无 staged、无 candidate/temp、官方源 ref/status/index 不变、三个 artifact 与 JSON 相符。
