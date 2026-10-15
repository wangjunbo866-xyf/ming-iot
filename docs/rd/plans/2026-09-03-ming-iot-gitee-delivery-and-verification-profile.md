---
document_kind: plan
status: approved
delivery_issue: ming-iot#IKD4OR
code_repository: git:gitee.com/ming-industry/ming-iot
related_documents:
  - specs/2026-09-03-ming-iot-gitee-delivery-and-verification-profile-design.md
  - CONTEXT.md
  - adr/0001-gitee-is-shared-delivery-truth.md
  - adr/0002-private-rd-docs-and-contract-promotion.md
---

# Ming IoT Gitee 交付流程与统一 Verification Profile 实施计划

> **执行说明：** 实施者按任务顺序执行，行为变更采用测试先行。Plan Task 是仓库级 Agent Todo 内部的工程检查点，不逐项创建 LoopX Todo 或 User Gate。本计划因涉及两个 Git 仓库，只建立两个有界的仓库级 Agent Todo；全部实现、验证、独立 Review 与双仓文档回执完成后，再建立一次 Delivery 汇总验收 Gate。

**目标：** 让 `ming-iot#IKD4OR` 形成一条可跨成员、跨电脑恢复的交付路径，并让本地开发、Gitee CI 与独立 Reviewer 统一调用 `ming-iot` 根 `Makefile` 中的四档 Verification Profile。

**架构：** `ming-iot-rd-docs` 维护 Delivery Packet、文档分类和元数据策略；`ming-iot` 维护根验证入口、项目契约、Gitee PR 模板与流水线。根 `Makefile` 只编排现有 Go、子目录 Makefile、Docker Compose 和 `backend/tools`，不复制底层实现，不依赖 LoopX。

**技术栈：** GNU Make、Go 1.24、Go `testing`、Python 3 `unittest/pytest`、Docker Compose、Gitee Go YAML、Markdown/YAML Front Matter。

**设计依据：** `specs/2026-09-03-ming-iot-gitee-delivery-and-verification-profile-design.md`

**Delivery Issue：** [ming-iot#IKD4OR](https://gitee.com/ming-industry/ming-iot/issues/IKD4OR)

## 执行拓扑与边界

只建立以下两个 Agent Todo，并共同引用 `ming-iot#IKD4OR`：

1. **研发文档仓 Todo**
   - `task_repository`: `git:gitee.com/ming-industry/ming-iot-rd-docs`
   - 写范围：`AGENTS.md`、`README.md`、`CONTEXT.md`、`adr/**`、`rfcs/**`、`specs/**`、`plans/**`、`reviews/**`、`evidence/**`、`meta/DOC_REGISTRY.yaml`、`tools/check_delivery_metadata.py`、`tests/test_delivery_metadata.py`、`tests/test_repository_policy.py`
2. **产品代码仓 Todo**
   - `task_repository`: `git:gitee.com/ming-industry/ming-iot`
   - 写范围：`Makefile`、`.gitee/**`、`.workflow/build-test-vet-202608301305.yml`、`AGENTS.md`、`backend/tools/README.md`、`backend/tools/internal/repositorycontract/**`、`deploy/Makefile`、`deploy/README.md`

两个 Todo 使用各自仓库内的独立 worktree。不得修改 LoopX 源码、Skill、`.loopx/` 或 `.codex/`；不得启动生产发布、处理真实凭据或删除共享数据库/卷。

允许在一个 Todo 内形成有意义的 checkpoint commit，但 commit 不是验收 Gate：产品仓建议在 Task 4、Task 5、Task 7 后各形成一个可回滚提交；研发文档仓仍遵守其当前 `AGENTS.md`，开始执行时一次性申请本 Todo 精确范围内的 `git add/commit` 授权，不逐 Task 重复申请。推送候选分支可按仓库团队规则执行；合并、发布和高风险动作遵守精确授权边界。

## Task 1：修复研发文档仓策略基线

**文件：**

- 修改：`tests/test_repository_policy.py`
- 修改：`README.md`
- 修改：`AGENTS.md`
- 新建：`rfcs/README.md`
- 保留并复核：`CONTEXT.md`
- 保留并复核：`adr/0001-gitee-is-shared-delivery-truth.md`
- 保留并复核：`adr/0002-private-rd-docs-and-contract-promotion.md`

- [ ] **Step 1：先把过期策略测试改成当前已批准边界**

将“仓库没有 remote”改为“只有 `origin`，且 fetch/push URL 都是批准的私有 Gitee 仓库”；将 hook 测试拆成允许和拒绝两条；把 `adr`、`rfcs` 加入必需目录。

核心断言：

```python
APPROVED_ORIGIN = "https://gitee.com/ming-industry/ming-iot-rd-docs.git"

def test_repository_uses_approved_origin(self) -> None:
    result = git("remote", "get-url", "--all", "origin")
    self.assertEqual(result.returncode, 0, result.stderr)
    self.assertEqual(result.stdout.splitlines(), [APPROVED_ORIGIN])

def test_pre_push_allows_approved_origin(self) -> None:
    result = run_hook(APPROVED_ORIGIN)
    self.assertEqual(result.returncode, 0, result.stderr)

def test_pre_push_rejects_unapproved_remote(self) -> None:
    result = run_hook("https://example.invalid/ming-iot-rd-docs.git")
    self.assertNotEqual(result.returncode, 0)
    self.assertIn("may only be pushed", result.stderr)
```

运行：

```bash
python3 -m pytest -q tests/test_repository_policy.py
```

预期：测试先因 `rfcs/` 不存在或旧断言未实现而失败；失败原因只涉及本 Task 的预期差异。

- [ ] **Step 2：创建 RFC 入口并修正文档仓定位**

`rfcs/README.md` 只定义以下规则：

- RFC 仅用于长期、难逆且存在真实取舍的架构决策；
- 状态只允许 `draft/accepted/rejected/superseded`；
- 普通功能和缺陷直接使用 Delivery Issue + Spec + Plan；
- RFC 接受不等于实现完成。

把 `README.md` 首句中的 `Local-only` 改为“private R&D authority”，并同时列出 `rfcs/` 与 `adr/`。在 `AGENTS.md` 固化 Delivery ID、文档分类、统一元数据和“Plan Task ≠ Agent Todo”，不复制 LoopX 内部实现。

- [ ] **Step 3：复核术语与 ADR**

确认 `CONTEXT.md` 和两个 ADR 不包含本机绝对路径、凭据、聊天记录或对 LoopX 通用能力的扩展要求；Delivery Issue、RFC、Proposal、Spec、Plan、Review、Evidence、Stable Project Contract 和 User Gate 的定义与批准规格一致。

- [ ] **Step 4：运行策略测试**

```bash
python3 -m pytest -q tests/test_repository_policy.py
git diff --check
```

预期：全部通过，且不改变 `archives/`。

## Task 2：建立 Delivery Packet 元数据校验

**文件：**

- 新建：`tools/check_delivery_metadata.py`
- 新建：`tests/test_delivery_metadata.py`
- 修改：`meta/DOC_REGISTRY.yaml`

- [ ] **Step 1：先写元数据失败测试**

用临时目录覆盖以下场景：

- 2026-09-03 以前的既有文件不被批量强制迁移；
- 新 RFC、Spec、Plan、Review、Evidence 缺 Front Matter 时失败；
- `document_kind` 与目录不一致时失败；
- 非法 `status`、非 `ming-iot#I...` Delivery ID、错误代码仓 identity 时失败；
- `related_documents` 使用绝对路径、`..`、反斜线或不存在文件时失败；
- Review/Evidence 缺少 40 位小写十六进制 `exact_head` 时失败；
- 合法 Delivery Packet 通过，并按路径稳定排序输出错误码。

建议稳定错误码：

```text
FRONT_MATTER_MISSING
DOCUMENT_KIND_INVALID
DOCUMENT_KIND_PATH_MISMATCH
STATUS_INVALID
DELIVERY_ISSUE_INVALID
CODE_REPOSITORY_INVALID
RELATED_DOCUMENT_INVALID
RELATED_DOCUMENT_NOT_FOUND
EXACT_HEAD_INVALID
```

运行：

```bash
python3 -m pytest -q tests/test_delivery_metadata.py
```

预期：因校验器尚未实现而失败。

- [ ] **Step 2：实现无额外依赖的严格校验器**

`check_delivery_metadata.py` 使用 Python 标准库读取文件开头的受限 YAML Front Matter，只支持本契约需要的标量和 `related_documents` 列表，不引入新的 YAML 依赖。状态集合固定为：

```python
ALLOWED_STATUS = {
    "rfc": {"draft", "accepted", "rejected", "superseded"},
    "spec": {"draft", "approved", "superseded"},
    "plan": {"draft", "approved", "completed", "superseded"},
    "review": {"pass", "pass_with_warnings", "changes_requested", "rejected"},
    "evidence": {"pass", "fail", "partial", "not_run"},
}
```

CLI：

```bash
python3 tools/check_delivery_metadata.py --docs-root . --since 2026-09-03
```

成功输出：

```text
delivery metadata: OK
```

任一问题写入 stderr 并返回 1；不得把解析失败降级为跳过。

- [ ] **Step 3：登记实施计划入口**

在 `meta/DOC_REGISTRY.yaml` 增加：

```yaml
  gitee_delivery_verification_implementation: plans/2026-09-03-ming-iot-gitee-delivery-and-verification-profile.md
```

先保证正文路径可读；只在文档提交进入稳定 Git revision 后，才按 `loopx-doc-registry` 执行 dry-run、正式 Authority 登记、`refresh-state` 与回读。不得把未提交 worktree 路径或裸哈希登记为来源。

- [ ] **Step 4：运行研发文档仓全量策略测试**

```bash
python3 -m pytest -q
python3 tools/check_delivery_metadata.py --docs-root . --since 2026-09-03
git diff --check
```

预期：全部通过。若出现与本改造无关的既有失败，保留完整错误并单独登记，不修改测试来伪造绿色。

## Task 3：先用产品仓契约测试锁定根验证接口

**文件：**

- 新建：`backend/tools/internal/repositorycontract/verification_profile_test.go`

- [ ] **Step 1：写根 Makefile 表面契约的失败测试**

测试必须从 Git 根定位文件，并断言：

1. 根 `Makefile` 存在；
2. `make help` 成功且列出八个公共目标；
3. `verify-pr` 的依赖顺序包含 `context diff-check build test vet contract-static boundary-scan`；
4. 缺少或错误的 `VERIFY_SCOPE` 立即非零退出；
5. 缺少或错误的 `VERIFY_SUITE` 立即非零退出；
6. 未设置 `MING_HEAVY_VERIFY=1` 时 `verify-release` 立即非零退出；
7. 失败路径不启动 Docker，也不产生 `backend/bin` 或 `deploy/.runtime`。

使用 `exec.Command("make", "-C", repoRoot, ...)` 调用只读帮助与 fail-fast 路径；为命令设置有界超时，超时视为失败。

运行：

```bash
go -C backend test ./tools/internal/repositorycontract -run 'TestVerificationProfile' -count=1
```

预期：因根 `Makefile` 尚不存在而失败。

- [ ] **Step 2：写仓库边界扫描的失败测试**

增加测试覆盖：

- `.loopx/`、`.codex/`、`backend/bin/`、`deploy/.runtime/`、`.env`、私钥文件与运行日志不得被 Git 跟踪；
- 允许已审阅的 `.env.example`；
- 通过拆分字符串构造敏感测试 marker，避免扫描器把测试源码本身误报；
- 输出只包含仓库相对路径，不泄露文件正文。

该测试是 `boundary-scan` 的唯一实现来源，根 Makefile 不再复制一套正则规则。

运行同一聚焦命令，预期仍为 RED。

## Task 4：实现根 Makefile 的快速与 PR 档位

**文件：**

- 新建：`Makefile`
- 修改：`deploy/Makefile`

- [ ] **Step 1：实现公共入口和 fail-fast 参数守卫**

根 Makefile 至少包含以下骨架；不得在其中设置或覆盖 `GOPROXY`：

```make
SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
BACKEND := $(ROOT)/backend
GO ?= go
VERIFY_SCOPE ?=
VERIFY_SUITE ?=
MING_HEAVY_VERIFY ?= 0

PKGS_share := gitee.com/ming-iot/share/...
PKGS_saas := gitee.com/ming-iot/saas/...
PKGS_things := gitee.com/ming-iot/things/...
PKGS_tools := ming-iot/backend/tools/...
ALL_PKGS := $(PKGS_share) $(PKGS_saas) $(PKGS_things) $(PKGS_tools)

.PHONY: help context diff-check build test vet contract-static boundary-scan \
        verify-fast verify-pr verify-integration verify-release

verify-pr: context diff-check build test vet contract-static boundary-scan
```

`context` 只打印目标、Go 版本、Git Head 与 scope/suite；不得打印完整环境。`make help` 对每个 Profile 说明成本和必填参数。

- [ ] **Step 2：实现 `build/test/vet/verify-fast`**

- `build` 调用 `$(MAKE) -C backend/saas build` 和 `$(MAKE) -C backend/things build`；
- `test`、`vet` 在 `backend/go.work` 下覆盖 share、saas、things、tools；
- `verify-fast` 只对显式单一 scope 执行 test、vet 和 `git diff --check`；
- 参数守卫必须在任何耗时命令之前执行。

- [ ] **Step 3：给 Things 基线增加真正的静态合同目标**

在 `deploy/Makefile` 增加不依赖 `things-baseline-build` 的 `things-baseline-contract`：

1. 用 `mktemp` 从 `.env.example` 创建临时文件；
2. `chmod 0600`；
3. 运行 `thingsstartbaseline contract`；
4. 运行 Compose `config --quiet`；
5. 运行已有危险操作和示例凭据扫描；
6. 用 shell trap 删除临时文件。

它不得构建二进制、镜像或启动服务。原 `things-baseline-fast` 保持兼容，但可复用这个静态目标，避免重复规则。

- [ ] **Step 4：聚合 `contract-static`**

按固定顺序调用：

```bash
go -C backend run ./tools/cmd/kafkacontract --repo-root .. --format text
make -C deploy things-baseline-contract
go -C backend/tools run ./cmd/apicheck verify --backend ..
go -C backend/tools run ./cmd/rpcgen check --manifest ../saas/service/syssvr/proto/rpcgen.yaml --saas ../saas
go -C backend/tools run ./cmd/rpcgen check --manifest ../things/service/dmsvr/proto/rpcgen.yaml --module-root ../things
```

这里的 `go -C backend` 会先把进程 cwd 切到 `backend/`，因此 `--repo-root ..` 必须解析到产品仓根；实施时用实际命令验证，不得凭路径猜测。任何校验失败立即返回非零。

- [ ] **Step 5：实现 `boundary-scan` 并转绿聚焦测试**

`boundary-scan` 只调用 Task 3 的仓库边界契约测试，不再嵌入第二份正则：

```bash
go -C backend test ./tools/internal/repositorycontract -run 'TestRepositoryDeliveryBoundary' -count=1
```

运行：

```bash
gofmt -w backend/tools/internal/repositorycontract/verification_profile_test.go
go -C backend test ./tools/internal/repositorycontract -run 'TestVerificationProfile|TestRepositoryDeliveryBoundary' -count=1
make help
make verify-fast VERIFY_SCOPE=tools
```

预期：全部通过；`verify-fast` 不启动 Docker。

## Task 5：实现显式 Integration 与 Release 档位

**文件：**

- 修改：`Makefile`
- 修改：`backend/tools/internal/repositorycontract/verification_profile_test.go`
- 修改：`deploy/README.md`
- 修改：`backend/tools/README.md`

- [ ] **Step 1：先补生命周期与失败语义测试**

测试根 Makefile 的 dry-run/帮助表面，确保：

- `VERIFY_SUITE=kafka` 路由到 Kafka up、ready、integration probe、down；
- `VERIFY_SUITE=things-startup` 路由到 Things up、verify、logs-on-failure、down；
- 两条路线都安装 cleanup trap；
- 未知 suite 不执行任何生命周期命令；
- `verify-release` 只有 `MING_HEAVY_VERIFY=1` 时才组合 `verify-pr` 和两个 suite；
- Profile 本身没有生产 deploy、数据库 reset、共享卷删除命令。

运行聚焦测试，预期先失败。

- [ ] **Step 2：实现 Kafka integration 路由**

语义顺序：

```text
kafka-up → kafka-ready → kafkacontract --integration → kafka-down
```

`kafka-down` 必须通过 trap 在成功和失败后运行；不得执行 `down -v`。broker 不可达必须失败，不得输出 PASS/skip。

- [ ] **Step 3：实现 Things startup integration 路由**

要求 `deploy/profiles/things-baseline/.env.local` 已存在且模式符合工具约束；缺失立即返回 NOT_RUN/非零。语义顺序：

```text
things-baseline-up → things-baseline-verify → things-baseline-down
```

失败时先输出有界脱敏日志，再安全 down；不调用不存在的 reset，也不删除数据库或卷。

- [ ] **Step 4：实现 release 防误触确认**

`verify-release` 先检查 `MING_HEAVY_VERIFY=1`，再依次运行：

```text
verify-pr
verify-integration VERIFY_SUITE=kafka
verify-integration VERIFY_SUITE=things-startup
```

它只验证发布候选，不执行发布、部署、凭据或权限操作。

- [ ] **Step 5：验证守卫与文档**

```bash
go -C backend test ./tools/internal/repositorycontract -run 'TestVerificationProfile' -count=1
make verify-integration              # 预期快速失败：缺 VERIFY_SUITE
make verify-integration VERIFY_SUITE=unknown  # 预期快速失败
make verify-release                  # 预期快速失败：缺 MING_HEAVY_VERIFY=1
git diff --check
```

不得仅为证明路由而启动重型环境；真实 suite 留到 Task 8。

## Task 6：增加统一 Gitee PR 模板

**文件：**

- 新建：`.gitee/PULL_REQUEST_TEMPLATE.zh-CN.md`
- 修改：`backend/tools/internal/repositorycontract/verification_profile_test.go`

- [ ] **Step 1：先写 PR 模板契约测试**

断言模板包含且只保留一套以下栏目：

```text
Delivery Issue
变更目的与边界
RFC / Spec / Plan
稳定项目契约晋升
Verification Profile
精确候选 Head
独立 Review
风险、兼容性与回滚
高风险操作声明
```

同时断言模板要求：

- `#<Issue ident>`；
- 文档仓相对路径与已合并 revision；
- 完整 40 位候选 Head；
- Review 的 verdict、exact head、review ref、blocker/warning 数；
- 未运行项、原因和补验证方式；
- 明确声明不包含发布、生产、凭据和权限操作。

运行聚焦测试，预期模板不存在而失败。

- [ ] **Step 2：实现中文模板**

模板只提供待填写结构，不预填 PASS，不把勾选框当 Evidence。说明 Gitee 创建 PR 时需选择该模板；若平台未自动载入，按官方页面选择“使用 Pull Request 模板”。

- [ ] **Step 3：转绿测试**

```bash
go -C backend test ./tools/internal/repositorycontract -run 'TestPullRequestTemplate' -count=1
git diff --check
```

## Task 7：把 Gitee Go 流水线切换到统一入口

**文件：**

- 修改：`.workflow/build-test-vet-202608301305.yml`
- 修改：`backend/tools/internal/repositorycontract/verification_profile_test.go`

- [ ] **Step 1：先写流水线配置契约测试**

测试读取 YAML 文本并验证：

- Go 版本主次为 `1.24`；
- 用户命令包含 `set -eu`；
- 只通过 `make verify-pr` 进入构建、测试、vet 和静态契约，不再复制 Go 包列表；
- CI 用 `MING_GOPROXY` 生成一次运行时 `GOPROXY`；
- 同时缓存 `/go/pkg/mod` 和 `/root/.cache/go-build`；
- 普通 PR 流水线不上传服务二进制 Artifact；
- 顶层 `triggers.pr.branches.precise` 包含 `master`；
- 不包含 `verify-integration`、`verify-release` 或 Docker 启动命令。

运行聚焦测试，预期当前手写命令/Artifact/manual trigger 导致失败。

- [ ] **Step 2：改为单入口流水线**

核心配置应收敛为：

```yaml
commands:
  - set -eu
  - go version
  - go env GOPATH GOMODCACHE GOCACHE
  - 'env GOPROXY="${MING_GOPROXY:-https://goproxy.cn,direct}" make verify-pr'
caches:
  - /go/pkg/mod
  - /root/.cache/go-build
```

删除普通 PR 的 `output/` 复制与 `BUILD_ARTIFACT`。顶层触发器按 Gitee 官方 YAML 语义配置：

```yaml
triggers:
  pr:
    branches:
      precise:
        - master
```

PR 触发会读取源分支最新 YAML，并构建 Gitee 自动预合并后的目标分支 + 源分支代码；因此流水线回执必须同时记录 Gitee 展示的候选身份，不能拿本地旧 Head 冒充。

- [ ] **Step 3：本地验证 YAML 与契约**

```bash
go -C backend test ./tools/internal/repositorycontract -run 'TestGiteeWorkflow' -count=1
git diff --check
```

预期：通过。若 Gitee UI 重新保存后规范化 YAML，只接受语义等价改动，并重新运行测试。

## Task 8：执行 Profile 矩阵并完成双仓交付回执

**文件：**

- 修改：`ming-iot/AGENTS.md`
- 修改：`ming-iot/backend/tools/README.md`
- 修改：`ming-iot/deploy/README.md`
- 新建：`ming-iot-rd-docs/reviews/2026-09-03-ming-iot-gitee-delivery-and-verification-profile-review.md`
- 新建：`ming-iot-rd-docs/evidence/2026-09-03-ming-iot-gitee-delivery-and-verification-profile/evidence.md`
- 修改：`ming-iot-rd-docs/meta/DOC_REGISTRY.yaml`

- [ ] **Step 1：完成稳定项目契约晋升**

在产品仓文档中只写后续所有开发者必须遵守的当前规则：根 Make 公共入口、Profile 选择、重型环境要求和 Gitee PR 回执。不要复制 RFC/Spec/Plan 的演进过程。

- [ ] **Step 2：运行轻量矩阵**

```bash
make verify-fast VERIFY_SCOPE=share
make verify-fast VERIFY_SCOPE=saas
make verify-fast VERIFY_SCOPE=things
make verify-fast VERIFY_SCOPE=tools
make verify-pr
```

每条命令记录 exact head、开始/结束时间、退出码与日志指针。若发现既有业务测试失败：

1. 不修改 Profile、不隐藏失败、不声称 PASS；
2. 在 Gitee 新建独立缺陷 Issue；
3. 在本 Delivery Evidence 中标记 blocker 和对应 Issue；
4. 当前 PR 保持 Draft，等待缺陷解决或负责人明确调整交付边界。

- [ ] **Step 3：在合适环境运行真实集成矩阵**

```bash
make verify-integration VERIFY_SUITE=kafka
make verify-integration VERIFY_SUITE=things-startup
MING_HEAVY_VERIFY=1 make verify-release
```

Kafka 可在具备 Docker 的开发机运行；Things startup 必须使用已批准的 WSL2/专用 Runner、独立 PostgreSQL 数据库和本地 `0600` 环境文件。缺少资源或依赖时记录 `NOT_RUN` 并保持非零，不能伪造绿色。`verify-release` 只在前两项能够稳定运行后执行，避免重复消耗。

- [ ] **Step 4：推送候选分支并创建 Draft PR**

产品仓 PR：

- 标题包含 `IKD4OR`；
- 正文使用统一模板并关联 `#IKD4OR`；
- 引用已经合并的 Spec/Plan 文档 revision；
- 保持 Draft，等待 Gitee `verify-pr` 与独立 Review。

不得由本计划默认合并 PR，也不得发布制品或操作生产。

- [ ] **Step 5：执行精确 Head 的独立 Review**

Reviewer 只读产品候选 Head，不修改代码、不提交实现。Review 文档 Front Matter 使用：

```yaml
---
document_kind: review
status: pass_with_warnings
delivery_issue: ming-iot#IKD4OR
code_repository: git:gitee.com/ming-industry/ming-iot
exact_head: <40-hex-code-head>
related_documents:
  - specs/2026-09-03-ming-iot-gitee-delivery-and-verification-profile-design.md
  - plans/2026-09-03-ming-iot-gitee-delivery-and-verification-profile.md
---
```

实际 `status` 必须来自 Review 结论。候选 Head 改变后，旧 Review 只能保留为历史，必须对新 Head 重新 Review。

- [ ] **Step 6：写 Evidence 摘要并先完成文档 PR**

Evidence 摘要只保存：

- Delivery ID；
- 产品 exact head；
- Profile/suite 结论；
- Gitee CI 构建链接或编号；
- 原始 LoopX Evidence 稳定指针；
- Review 相对路径与 verdict；
- blocker/warning 数；
- 未运行项和原因。

不内联原始日志、命令大段输出、凭据或本机绝对路径。将 Review/Evidence 登记到 `DOC_REGISTRY.yaml`，通过元数据和全仓策略测试后，先把研发文档 PR 合并到 `ming-iot-rd-docs/master`。

- [ ] **Step 7：更新代码 PR 并进入最终人审**

代码 PR 引用稳定的文档 revision，确认：

```bash
make verify-pr
python3 -m pytest -q  # 在 ming-iot-rd-docs
git diff --check      # 两个仓库分别执行
```

全部结果与 PR Head 一致后退出 Draft。由有权限的人在 Gitee 完成最终 Review 和合并；只有要求 Agent 代为合并时，才建立绑定精确 PR、Head、目标分支和合并方式的 User Gate。

## 最终验收

满足以下条件后，两个仓库级 Agent Todo 才能形成一次 Delivery 汇总验收 Gate：

1. `ming-iot#IKD4OR` 能定位已合并的 Spec、Plan、Review 和 Evidence 摘要；
2. 根 Makefile 八个公共目标行为与帮助文本一致；
3. `verify-pr` 不依赖 Docker/外部服务，且本地与 Gitee CI 调用同一命令；
4. integration/release 缺少参数、依赖或重型确认时 fail-closed；
5. Gitee PR 模板连接 Issue、Delivery Packet、exact head、验证、Review 和回滚；
6. Gitee PR 流水线不构建重型环境、不上传普通验证二进制；
7. 文档元数据、目录、remote/hook 和 Authority Catalog 策略测试通过；
8. 未修改 LoopX 通用能力，未提交 `.loopx/`、`.codex/`、凭据或原始运行日志；
9. 既有失败被如实登记为独立 Issue，没有通过降低门禁获得假 PASS；
10. 独立 Review 绑定最终产品 Head，文档 PR 已先进入稳定 revision。
