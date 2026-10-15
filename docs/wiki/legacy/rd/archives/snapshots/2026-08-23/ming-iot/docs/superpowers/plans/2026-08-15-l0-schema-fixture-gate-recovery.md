# L0 Schema Fixture Gate Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 以 SHA 锁定、可审计的单一测试 fixture 转换修复上游 `TestTempate`，同时让最终 gate 的 stdout/stderr 失败摘要都可见，再重建目标并取得真实 L0 PASS 证据。

**Architecture:** 在现有 fail-closed `TransformTree` 预检阶段增加第三个固定路径转换，只修改 `share/domain/schema/schema_test.go` 的 45 个旧 JSON token，不触碰生产模型。转换计数贯穿 lock、`TransformReport`、PROVENANCE 和 evidence；gate runner 对 stdout/stderr 分配独立摘要预算。目标仍只能由迁移器整树发布，最终测试与构建只在 isolation tree 中执行。

**Tech Stack:** Go 1.25.4、`crypto/sha256`、现有 `thingsbaseline` renderer/verifier、YAML lock、Git/JSON evidence、TDD。

**Repository policy:** 不修改 `/home/wangjunbo/workspace-iot-platform/things`；不执行 `go work sync`；不在真实 `backend/things` 上运行 test/tidy/build；未经用户明确授权不 `git add/commit/push`，所以本计划以 status/checkpoint 代替 commit 步骤。

---

## 文件结构

- Modify: `backend/tools/internal/thingsbaseline/contract.go` — 新增并校验 fixture lock 字段。
- Modify: `backend/tools/internal/thingsbaseline/contract_test.go` — 契约 RED/GREEN 测试。
- Modify: `backend/tools/things-baseline.lock` — 固定 45、输入 SHA、输出 SHA。
- Modify: `backend/tools/internal/thingsbaseline/transform.go` — 实现精确 fixture 转换并写入报告。
- Modify: `backend/tools/internal/thingsbaseline/transform_test.go` — 函数边界、全管线、固定源集成测试。
- Modify: `backend/tools/internal/thingsbaseline/render.go` — 增加 PROVENANCE token。
- Modify: `backend/tools/internal/thingsbaseline/render_test.go` — 模板 byte-golden。
- Modify: `backend/tools/internal/thingsbaseline/scan_test.go` — 更新完整报告 fixture。
- Modify: `backend/tools/templates/things-baseline/PROVENANCE.yaml` — 记录第六类转换。
- Modify: `backend/tools/internal/thingsbaseline/verify.go` — stdout/stderr 独立摘要预算。
- Modify: `backend/tools/internal/thingsbaseline/verify_test.go` — 双流保留、边界与脱敏测试。
- Modify: `docs/superpowers/plans/2026-08-14-l0-things-compilation-migration-baseline.md` — 同步主计划契约和实测状态。
- Create: `docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline-attempt-1-fail.json` — 保留首次 gate 失败事实。
- Create: `docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline-attempt-2-workspace-fail.json` — 保留 workspace gate 失败事实。
- Replace by verifier: `docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.json` — 最终机器证据。
- Create: `docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.md` — 人工证据解释。
- Modify: `backend/tools/cmd/apicheck/main_test.go` — 测试辅助进程使用当前 worktree，并隔离父 `GOWORK`。
- Create outside deliverable: `iot-doc/.superpowers/visual-live/content/rd-graph-l0-evidence-v3.html` — 常驻总控台最终页。

### Task 1：扩展严格 lock 契约

- [x] **Step 1: 写失败测试**

在 `validLockForTest()` 的 `TransformContract` 增加：

```go
SchemaFixtureIdentifierCount: 1,
SchemaFixtureSourceSHA256:    strings.Repeat("d", 64),
SchemaFixtureOutputSHA256:    strings.Repeat("e", 64),
```

并在 `TestValidateLockRejectsUnsafeContracts` 增加：

```go
{"zero schema fixture transforms", func(l *Lock) {
    l.Transforms.SchemaFixtureIdentifierCount = 0
}, "schemaFixtureIdentifierCount"},
{"bad schema fixture source SHA", func(l *Lock) {
    l.Transforms.SchemaFixtureSourceSHA256 = "bad"
}, "transform source SHA-256"},
{"bad schema fixture output SHA", func(l *Lock) {
    l.Transforms.SchemaFixtureOutputSHA256 = strings.Repeat("F", 64)
}, "transform output SHA-256"},
```

- [x] **Step 2: 运行 RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline -run 'TestValidateLock' -count=1
```

Expected: 编译失败，`TransformContract` 尚无三个字段。

- [x] **Step 3: 实现 lock 字段和稳定校验顺序**

`TransformContract` 增加：

```go
SchemaFixtureIdentifierCount int    `yaml:"schemaFixtureIdentifierCount"`
SchemaFixtureSourceSHA256    string `yaml:"schemaFixtureSourceSha256"`
SchemaFixtureOutputSHA256    string `yaml:"schemaFixtureOutputSha256"`
```

`ValidateLock` 在 private-key/vet 计数之后按以下顺序校验：

```go
if err := validatePositiveCount("transforms.schemaFixtureIdentifierCount", lock.Transforms.SchemaFixtureIdentifierCount); err != nil {
    return err
}
if !isLowercaseHex(lock.Transforms.PrivateKeySourceSHA256, 64) ||
    !isLowercaseHex(lock.Transforms.VetSourceSHA256, 64) ||
    !isLowercaseHex(lock.Transforms.SchemaFixtureSourceSHA256, 64) {
    return fmt.Errorf("transform source SHA-256 values must be 64 lowercase hex characters")
}
if !isLowercaseHex(lock.Transforms.SchemaFixtureOutputSHA256, 64) {
    return fmt.Errorf("transform output SHA-256 values must be 64 lowercase hex characters")
}
```

- [x] **Step 4: 更新真实 lock**

在 `transforms` 增加：

```yaml
  schemaFixtureIdentifierCount: 45
  schemaFixtureSourceSha256: 0468f6ebf1426919ee8ec1f789861874490e127c279bf8a54daac0c0ddda5791
  schemaFixtureOutputSha256: 77570823fce1f9393ce992c1303a40fb1c0f6e2b7e2f780d0965c9b4906d0c30
```

- [x] **Step 5: 运行 GREEN 和格式检查**

Run:

```bash
cd backend/tools
gofmt -w internal/thingsbaseline/contract.go internal/thingsbaseline/contract_test.go
GOWORK=off go test ./internal/thingsbaseline -run 'TestValidateLock|TestLoadLock' -count=1
```

Expected: PASS；严格 YAML 可加载新增字段，零值/畸形 SHA 均拒绝。

### Task 2：实现 SHA 锁定的 fixture 转换

- [x] **Step 1: 写纯函数 RED 测试**

新增 `TestFixSchemaFixtureIdentifiersIsExactAndFailClosed`，核心成功样例：

```go
source := []byte("`fixture` = {\n  \"id\": \"a\",\n  \"id\": \"b\"\n}\n")
want := []byte("`fixture` = {\n  \"identifier\": \"a\",\n  \"identifier\": \"b\"\n}\n")
got, count, err := FixSchemaFixtureIdentifiers(source, sha256Hex(source), sha256Hex(want), 2)
if err != nil || count != 2 || !bytes.Equal(got, want) {
    t.Fatalf("fixture transform = count %d data %q error %v", count, got, err)
}
```

表驱动失败样例必须分别覆盖：source SHA 错、output SHA 错、期望计数错、零旧 token、预存 `"identifier":`、输入输出共享 backing array。错误只能含计数/SHA/路径，不打印 fixture 正文。

- [x] **Step 2: 运行纯函数 RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline -run TestFixSchemaFixtureIdentifiers -count=1
```

Expected: 编译失败，函数尚不存在。

- [x] **Step 3: 实现最小纯函数**

在 `transform.go` 增加：

```go
const schemaFixtureSourcePath = "share/domain/schema/schema_test.go"

func FixSchemaFixtureIdentifiers(src []byte, expectedSourceSHA, expectedOutputSHA string, expectedCount int) ([]byte, int, error) {
    if sha256Digest(src) != expectedSourceSHA {
        return nil, 0, fmt.Errorf("schema fixture source SHA-256 mismatch")
    }
    oldToken := []byte(`"id":`)
    newToken := []byte(`"identifier":`)
    if bytes.Count(src, newToken) != 0 {
        return nil, 0, fmt.Errorf("schema fixture already contains identifier token")
    }
    count := bytes.Count(src, oldToken)
    if count != expectedCount || count <= 0 {
        return nil, 0, fmt.Errorf("schema fixture identifier count = %d, want %d", count, expectedCount)
    }
    out := bytes.ReplaceAll(src, oldToken, newToken)
    if bytes.Count(out, oldToken) != 0 || bytes.Count(out, newToken) != expectedCount {
        return nil, 0, fmt.Errorf("schema fixture reverse count mismatch")
    }
    if sha256Digest(out) != expectedOutputSHA {
        return nil, 0, fmt.Errorf("schema fixture output SHA-256 mismatch")
    }
    return append([]byte(nil), out...), count, nil
}
```

复用已有不导出 helper `sha256Digest([]byte) string`，不得增加第二套哈希语义。

- [x] **Step 4: 将转换接入完整预检管线**

`TransformReport` 增加：

```go
SchemaFixtureIdentifiers int `json:"schemaFixtureIdentifierCount"`
```

`TransformTree` 必须在创建 `rendered` 前：取得固定路径、调用 `FixSchemaFixtureIdentifiers`、校验返回 count 等于 lock；然后像 private/vet 一样覆盖 `rendered[schemaFixtureSourcePath]`，最后写入 `report.SchemaFixtureIdentifiers`。注释从“only two source-specific edits”改成“三个 SHA-locked source-specific edits”。路径缺失必须返回 `missing schema fixture source`。

- [x] **Step 5: 扩展 synthetic 与固定源测试**

`fullTransformFixture` 增加两 token fixture，并把 source/output SHA 与 count=2 写入 lock；`TransformTree` 期望报告增加 `SchemaFixtureIdentifiers: 2`，并断言旧 token 为 0、新 token 为 2。`TestTransformRealLockedSourceWhenEnabled` 期望：

```go
SchemaFixtureIdentifiers: 45,
```

同时增加 preflight 测试，分别篡改 fixture source SHA、output SHA、count，断言输入 map/byte slices 不变且未进入 module rewrite。

- [x] **Step 6: 运行 GREEN 与固定源集成**

Run:

```bash
cd backend/tools
gofmt -w internal/thingsbaseline/transform.go internal/thingsbaseline/transform_test.go
GOWORK=off go test ./internal/thingsbaseline -run 'TestFixSchemaFixture|TestTransformTree' -count=1
THINGS_BASELINE_SOURCE_REPOSITORY=/home/wangjunbo/workspace-iot-platform/things \
THINGS_BASELINE_LOCK="$PWD/things-baseline.lock" \
GOWORK=off go test ./internal/thingsbaseline -run TestTransformRealLockedSourceWhenEnabled -count=1 -v
```

Expected: PASS；真实报告为 `94/2726/55/1/1/45`，官方源码状态不变。

### Task 3：贯穿 PROVENANCE、报告与主计划

- [x] **Step 1: 写模板 RED 测试**

所有构造完整 `TransformReport` 的测试增加 `SchemaFixtureIdentifiers`。`templateGoldens` 的 PROVENANCE 期望增加：

```yaml
  schemaFixtureIdentifierCount: %d
```

并传入 `report.SchemaFixtureIdentifiers`。测试还要断言未替换 token `{{SCHEMA_FIXTURE_IDENTIFIER_COUNT}}` 不得残留。

- [x] **Step 2: 运行模板 RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline -run 'TestRenderTargetTemplates|TestCandidate' -count=1
```

Expected: FAIL，模板/renderer 尚未输出第六类转换。

- [x] **Step 3: 实现模板 token**

`renderTargetTemplates` 增加：

```go
"{{SCHEMA_FIXTURE_IDENTIFIER_COUNT}}": strconv.Itoa(report.SchemaFixtureIdentifiers),
```

`PROVENANCE.yaml` 增加：

```yaml
  schemaFixtureIdentifierCount: {{SCHEMA_FIXTURE_IDENTIFIER_COUNT}}
```

同步 `render_test.go`、`scan_test.go` 中的完整报告和 golden，不放宽未知 token 检查。

- [x] **Step 4: 同步主实施计划**

修改 `2026-08-14-l0-things-compilation-migration-baseline.md`：

- `TransformReport` 示例增加 `SchemaFixtureIdentifiers`；
- source-specific 转换从两个改为三个；
- PROVENANCE 从五类改为六类；
- import 期望改为 `94/2726/55/1/1/45`；
- Task 7 记录首次 `things-test` FAIL，禁止把它标为通过；
- 最终重跑成功后才勾选 Task 7。

- [x] **Step 5: 运行 renderer 全量测试**

Run:

```bash
cd backend/tools
gofmt -w internal/thingsbaseline/render.go internal/thingsbaseline/render_test.go internal/thingsbaseline/scan_test.go
GOWORK=off go test ./internal/thingsbaseline -count=1
```

Expected: PASS；报告、证据 JSON 和 PROVENANCE 使用同一 lower-camel 字段名。

### Task 4：修复 gate 双流诊断摘要

- [x] **Step 1: 写 stdout 不得被 stderr 覆盖的 RED 测试**

在 `TestExecVerificationGateRunnerModelsExitSpawnTimeoutAndRedaction/non-zero exit` 使用 shell 同时产生：

```sh
printf 'STDOUT-FAILURE-SENTINEL\n'
i=0; while [ "$i" -lt 9000 ]; do printf x >&2; i=$((i+1)); done
printf '\nSTDERR-DOWNLOAD-SENTINEL\n' >&2
exit 7
```

断言 summary 同时包含 `[stdout]`、`STDOUT-FAILURE-SENTINEL`、`[stderr]`、`STDERR-DOWNLOAD-SENTINEL`，长度不超过 4096，secret/cwd 仍零泄漏。

- [x] **Step 2: 运行 RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/thingsbaseline -run TestExecVerificationGateRunnerModelsExitSpawnTimeoutAndRedaction/non-zero -count=1
```

Expected: FAIL，当前最终截断只保留 stderr。

- [x] **Step 3: 实现独立流预算**

把当前先 join 再 tail 的逻辑改成：每个流先脱敏，再分别截取不超过 1900 bytes 的最新 tail；非空流加 `[stdout]`/`[stderr]` 标签和换行，组合后做最后的 4096-byte 防御检查。完整 `LogSHA256` 仍按原 framing 合并两个完整 stream hash，不改变哈希契约。spawn error 使用同一脱敏 helper，但只输出 `[runner]` 段。

- [x] **Step 4: 运行 gate/evidence 回归**

Run:

```bash
cd backend/tools
gofmt -w internal/thingsbaseline/verify.go internal/thingsbaseline/verify_test.go
GOWORK=off go test ./internal/thingsbaseline -run 'TestExecVerificationGateRunner|TestEvidence' -count=1
GOWORK=off go test -race ./internal/thingsbaseline -run TestExecVerificationGateRunner -count=1
```

Expected: PASS；双流 sentinel 均可见，摘要有界且已脱敏。

### Task 5：重建目标并取得最终证据

- [x] **Step 1: 运行工具层完整门禁**

Run:

```bash
cd backend/tools
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
git diff --check
```

Expected: 全部 PASS。

- [x] **Step 2: 保留首次失败证据**

在覆盖 canonical evidence 前执行：

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/l0-things-migration-baseline
jq -e '.overallResult == "FAIL" and (.gates[] | select(.id == "things-test") | .exitCode == 1)' \
  docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.json
cp docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.json \
  docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline-attempt-1-fail.json
cmp -s docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.json \
  docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline-attempt-1-fail.json
```

该文件只记录失败事实，不伪造缺失的 stdout 错误摘要。

- [x] **Step 3: 由迁移器整树重建目标**

先确认 `backend/things` 与旧结构证据 digest `1aeee85d09f734fa279680a113a1f0c97a21d4919a48822005caa339595ac64a` 一致且无用户文件；把它移动到唯一 `/tmp/l0-things-pre-schema-fixture-rebuild` 作为可恢复备份：

```bash
test -d backend/things
test ! -e /tmp/l0-things-pre-schema-fixture-rebuild
mv backend/things /tmp/l0-things-pre-schema-fixture-rebuild
```

然后运行：

```bash
cd backend/tools
GOWORK=off go run ./cmd/thingsbaseline import \
  --lock ./things-baseline.lock \
  --source /home/wangjunbo/workspace-iot-platform/things \
  --backend ..
```

Expected: 980 files、转换 `94/2726/55/1/1/45`；新 tree digest 与旧 digest 不同。执行：

```bash
diff -qr ../things /tmp/l0-things-pre-schema-fixture-rebuild > /tmp/l0-schema-fixture-rebuild.diff || true
test "$(wc -l < /tmp/l0-schema-fixture-rebuild.diff)" -eq 2
rg 'share/domain/schema/schema_test.go|PROVENANCE.yaml' /tmp/l0-schema-fixture-rebuild.diff
find /tmp/l0-things-pre-schema-fixture-rebuild -depth -delete
find /tmp/l0-schema-fixture-rebuild.diff -delete
```

除 `share/domain/schema/schema_test.go` 和 `PROVENANCE.yaml` 外，新旧目标逐路径/mode/content 必须相同；不使用宽 glob 删除。

- [x] **Step 4: 运行 structural verify**

```bash
cd backend/tools
GOWORK=off go run ./cmd/thingsbaseline verify \
  --structural-only \
  --lock ./things-baseline.lock \
  --source /home/wangjunbo/workspace-iot-platform/things \
  --backend .. \
  --evidence /tmp/l0-things-structural-schema-fixture.json
```

Expected: `NOT_FINAL`；双渲染一致、target differences=0、target/source unchanged。

- [x] **Step 5: 运行最终 verify**

```bash
cd backend/tools
GOWORK=off go run ./cmd/thingsbaseline verify \
  --lock ./things-baseline.lock \
  --source /home/wangjunbo/workspace-iot-platform/things \
  --backend .. \
  --evidence ../../docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.json
```

Expected: tools/saas/things/workspace/build 五个 gate 全部 `PASS/EXITED/0`；`TestTempate` 实际运行；module/target/isolation/source unchanged；`overallResult=PASS`、`finalEvidence=true`。

实测恢复链：fixture 修复后的第二次运行中 `things-test` 已 PASS，但 `workspace-test`
发现 `cmd/apicheck` 测试辅助进程硬编码主工作区并继承临时 `GOWORK`。该轮按原字节保存为
attempt-2；随后以两个回归测试锁定“当前 worktree + 子进程 `GOWORK=off`”，未放宽任何
生产门禁。最终运行五个 gate 全部 `PASS/EXITED/0`。

- [x] **Step 6: 复核构建产物与证据一致性**

验证三个 `backend/bin/{thingsvr,dgsvr,dmsvr}` 可执行且被忽略；JSON 时间自洽、空集合为 `[]`、不含本机绝对路径、两次 render digest 相同、转换计数 45。不得仅凭 CLI exit code 宣称成功。

- [x] **Step 7: 写 Markdown evidence 并更新 18081**

Markdown 解释首次失败、fixture 根因、A 方案边界、旧/新 tree digest、五个 gate、凭据/私钥/vet 事实和 L1/L3 解锁状态。创建 `rd-graph-l0-evidence-v3.html` 并让常驻首页显示 L0 PASS、A1、`source-unchanged`、canonical JSON/Markdown 路径及失败尝试证据。

- [x] **Step 8: 最终范围审计**

Run:

```bash
git diff --check
git status --short --branch
git diff --cached --quiet
git -C /home/wangjunbo/workspace-iot-platform/things status --short --branch
git -C /home/wangjunbo/workspace-iot-platform/things rev-parse refs/remotes/origin/master
find backend -maxdepth 1 -name '.thingsbaseline-*' -print
```

Expected: official ref 仍为 `243ab1aa03b8a3f106761a81cdd9bd8827d448c8`；官方 worktree 无新增修改；无 staged/commit/push；无临时 candidate/isolation/debug 目录；只列出本计划预期文件等待用户审核。
