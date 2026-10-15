# Ming IoT 本地 Share 模块迁移实施计划

> **Agent 执行要求：** 实施时必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，并按 Task 顺序执行。Task/Step 只在本计划和 Evidence 中跟踪，不逐项创建 LoopX Agent Todo，也不逐项建立 User Gate。

**目标：** 将冻结的 `share@v1.6.4` 原子迁入 `ming-iot/backend/share`，把所有活跃构建输入切换为 `gitee.com/ming-iot/share`，完成许可证、依赖闭合、分层测试、Things 启动回归和独立 Review Evidence。

**架构：** 迁移由一个持续的 LoopX Agent Todo 承载，在独立 Git worktree 中形成单一候选树。来源只通过冻结 Tag 的 `git archive` 导入；旧路径机械替换由已批准的 1548 文件 allowlist 限定，启动适配只允许修改既有启动基线 checkpoint 已批准的两个 Makefile 与一个契约测试，并通过仓库合同测试持续禁止旧模块路径重新进入活跃构建输入。

**技术栈：** Git、Go 1.24.4、Go Workspace、Go test/race/vet、Docker Compose、`thingsstartbaseline`、Apache-2.0。

---

## 执行约束

- 目标 worktree：`ming-iot/.worktrees/local-share-migration`，分支 `local-share-migration`。
- 来源仓库 `share/` 永久只读；只能读取 Tag `v1.6.4`，禁止修改其工作树。
- 许可证采用 `Apache-2.0`；保留 `utils/keyMatch.go` 和 `utils/notify/notify_*.go` 的已有第三方版权及 Apache-2.0 声明。
- 本计划不授权 `git add`、`git commit`、merge 或 push。完成全部 Task 后只建立一次汇总 User Gate。
- 1548 个机械替换路径、3 个启动适配路径、冻结摘要、许可证授权或 LoopX checkpointed write scope 任一变化时立即停止，不得自行扩大范围。

### Task 1：建立迁移后仓库合同的失败测试

**文件：**

- 创建：`backend/tools/internal/repositorycontract/local_share_migration_test.go`

- [x] **Step 1：编写迁移合同测试**

测试必须从 `git rev-parse --show-toplevel` 获得仓库根，并验证以下固定合同：

```go
const (
	oldShareModule = "gitee.com/unitedrhino/share"
	newShareModule = "gitee.com/ming-iot/share"
	frozenCommit   = "ece02ca9d6defd63356931a59e653950f27ac572"
	frozenTree     = "21bdb52ed508152cf440b8d8a3b3ae3943a4b84a"
)

var allowedHistoricalReferences = map[string]struct{}{
	"docs/superpowers/plans/2026-07-23-core-migration-foundation.md": {},
	"docs/superpowers/plans/2026-07-23-goctl-rpcgen-toolchain.md": {},
	"docs/superpowers/plans/2026-08-14-l0-things-compilation-migration-baseline.md": {},
	"docs/superpowers/specs/2026-07-23-core-goctl-migration-design.md": {},
	"docs/superpowers/specs/2026-08-14-l0-things-compilation-migration-baseline-design.md": {},
}
```

测试用例必须覆盖：

1. `backend/share/go.mod` 的 module 行等于 `gitee.com/ming-iot/share`；
2. `backend/share/SOURCE.json` 的 commit/tree 等于冻结值，许可证等于 `Apache-2.0`，规范化来源树摘要等于固定值；
3. `backend/go.work` 包含 `./share`；
4. `git grep -l gitee.com/unitedrhino/share` 只允许返回上述 5 个历史文档；
5. 四个第三方文件仍包含原有 `Copyright` 与 `Licensed under the Apache License, Version 2.0` 文本。

- [x] **Step 2：运行测试并确认先失败**

```bash
cd backend/tools
go test ./internal/repositorycontract -run LocalShare -count=1
```

预期：FAIL，原因是 `backend/share` 尚不存在或 workspace 尚未登记 `./share`。

### Task 2：复核冻结来源与精确 allowlist

**读取：**

- `/home/wangjunbo/workspace-iot-platform/share`
- `specs/2026-08-27-ming-iot-local-share-migration-activation-design.md`
- `evidence/2026-08-27-ming-iot-local-share-migration-activation/source-freeze.json`

- [x] **Step 1：验证来源身份**

```bash
cd /home/wangjunbo/workspace-iot-platform/share
test "$(git rev-parse 'v1.6.4^{commit}')" = ece02ca9d6defd63356931a59e653950f27ac572
test "$(git rev-parse 'v1.6.4^{tree}')" = 21bdb52ed508152cf440b8d8a3b3ae3943a4b84a
test "$(git ls-tree -r --name-only v1.6.4 | wc -l)" -eq 255
test "$(git ls-tree -r v1.6.4 | sort | sha256sum | awk '{print $1}')" = 6a7d2b40e5a6f6fdeb7d7a3d2f01fe1994efab3aa2c2f52ba352c50f4c4a5a6c
test "$(git archive --format=tar v1.6.4 | sha256sum | awk '{print $1}')" = 8c90b2b3853ad86b422f9929d801d617645bac3dbf68f600e3736b3a32505acf
test -z "$(git status --short)"
```

预期：全部退出码为 0。

- [x] **Step 2：验证目标 allowlist**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/local-share-migration
test "$(git grep -l 'gitee.com/unitedrhino/share' -- backend/saas backend/things backend/tools | wc -l)" -eq 1548
test "$(git grep -l 'gitee.com/unitedrhino/share' -- backend/saas | wc -l)" -eq 818
test "$(git grep -l 'gitee.com/unitedrhino/share' -- backend/things | wc -l)" -eq 726
test "$(git grep -l 'gitee.com/unitedrhino/share' -- backend/tools | wc -l)" -eq 4
test "$(git grep -l 'gitee.com/unitedrhino/share' | sha256sum | awk '{print $1}')" = 5d6a4fc25d343b03e3daaa12293096dd7c3b952b916abdf6264ca701ae30e3e2
```

预期：全部退出码为 0；任何数量或摘要变化都终止迁移。

### Task 3：从冻结 Git tree 创建本地 Share module

**文件：**

- 创建：`backend/share/**`
- 修改：`backend/share/go.mod`
- 创建：`backend/share/LICENSE`
- 创建：`backend/share/SOURCE.json`

- [x] **Step 1：通过 Git archive 导入来源文件**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/local-share-migration
test ! -e backend/share
mkdir backend/share
git -C /home/wangjunbo/workspace-iot-platform/share archive --format=tar v1.6.4 | tar -xf - -C backend/share
test "$(find backend/share -type f | wc -l)" -eq 255
```

预期：只创建冻结 tree 的 255 个 tracked 文件，不包含 `.git`、缓存或构建物。

- [x] **Step 2：切换 Share 自身 module/import path**

```bash
mapfile -d '' files < <(rg -l -0 --fixed-strings 'gitee.com/unitedrhino/share' backend/share)
test "${#files[@]}" -eq 84
perl -pi -e 's#gitee\.com/unitedrhino/share#gitee.com/ming-iot/share#g' "${files[@]}"
test "$(head -n 1 backend/share/go.mod)" = 'module gitee.com/ming-iot/share'
```

预期：84 个来源自引用文件完成机械替换，未改业务逻辑。

- [x] **Step 3：加入许可证和来源身份文件**

`backend/share/LICENSE` 使用 Apache License 2.0 标准全文；`backend/share/SOURCE.json` 使用以下确定内容：

```json
{
  "schema_version": "ming_iot_local_share_source_v1",
  "source_repository": "gitee:unitedrhino/share",
  "source_tag": "v1.6.4",
  "commit": "ece02ca9d6defd63356931a59e653950f27ac572",
  "tree": "21bdb52ed508152cf440b8d8a3b3ae3943a4b84a",
  "tracked_file_count": 255,
  "archive_sha256": "8c90b2b3853ad86b422f9929d801d617645bac3dbf68f600e3736b3a32505acf",
  "archive_format": "git-archive-tar",
  "archive_command": "git archive --format=tar v1.6.4",
  "normalized_tree_sha256": "85e564cd406cb90738bfbf04f7789471ee61854a98e837bb604cb09243f31867",
  "license": "Apache-2.0",
  "authorization_route": "rights_holder_or_authorized_company_owner",
  "authorization_date": "2026-08-27",
  "third_party_notices_preserved": true
}
```

不得把用户原始声明、公司内部信息或聊天内容写入公开源码仓库。

### Task 4：原子切换 SaaS、Things、Tools 与 Workspace

**文件：**

- 修改：批准 allowlist 中 `backend/saas/**` 的 818 个文件
- 修改：批准 allowlist 中 `backend/things/**` 的 726 个文件
- 修改：批准 allowlist 中 `backend/tools/**` 的 4 个文件
- 修改：`backend/go.work`
- 必要时修改：`backend/go.work.sum`

- [x] **Step 1：对冻结 allowlist 做唯一机械替换**

```bash
mapfile -d '' files < <(git grep -lz 'gitee.com/unitedrhino/share' -- backend/saas backend/things backend/tools)
test "${#files[@]}" -eq 1548
perl -pi -e 's#gitee\.com/unitedrhino/share#gitee.com/ming-iot/share#g' "${files[@]}"
```

预期：只修改批准的 1548 个文件；5 个历史文档保持不变。

- [x] **Step 2：把 Share 加入 Go Workspace**

将 `backend/go.work` 的 `use` 块改为：

```go
use (
	./saas
	./share
	./things
	./tools
)
```

SaaS 与 Things 的 `go.mod` 中 Share 依赖应为：

```go
gitee.com/ming-iot/share v0.0.0
```

不得增加 `replace gitee.com/unitedrhino/share => ./share`。若当前 Go 工具链在加载完整 Workspace graph 时仍尝试下载新模块，则只允许在 `backend/go.work` 增加以下精确版本本地解析：

```go
replace gitee.com/ming-iot/share v0.0.0 => ./share
```

该规则只闭合新模块的本地 Workspace graph，不得泛化成无版本 replace，也不得保留任何旧模块身份。

- [x] **Step 3：格式化并验证 Workspace，不改写冻结模块依赖**

```bash
cd backend
go work edit -fmt
gofmt -w tools/internal/repositorycontract/local_share_migration_test.go
go list -m all >/dev/null
git diff --exit-code -- tools/go.mod tools/go.sum
test "$(sha256sum share/go.sum | cut -d' ' -f1)" = "612dddc37326f4c5808c99fe72889a0827a074c926737738da86a9b3854461a5"
```

预期：Workspace graph 可加载，`backend/tools/go.mod`、`backend/tools/go.sum` 与冻结 Share 的 `go.sum` 不发生隐式升级。不得在最终候选树上运行会改写成员模块依赖的 `go work sync`；若为了诊断运行，必须立即复核并恢复所有非授权依赖变化。

### Task 5：使仓库合同和 module graph 通过

- [x] **Step 1：运行聚焦合同测试**

```bash
cd backend/tools
go test ./internal/repositorycontract -run LocalShare -count=1
```

预期：PASS。

- [x] **Step 2：验证旧路径只存在于历史文档**

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/local-share-migration
git -c core.quotePath=false grep -l 'gitee.com/unitedrhino/share' | sort
```

预期：只输出 Task 1 固定的 5 个 `docs/superpowers/**` 文件。

- [x] **Step 3：验证 Workspace 解析到本地 Share**

```bash
cd backend
go list -m -json gitee.com/ming-iot/share | jq -e '.Path == "gitee.com/ming-iot/share" and .Dir != null'
! go list -m all | rg '^gitee\.com/unitedrhino/share([[:space:]]|$)'
go list -m all | rg '^gitee\.com/ming-iot/share([[:space:]]|$)'
```

预期：新模块具有本地 `Dir`，旧模块不在 graph 中。

### Task 6：执行分层 Go 验证

- [x] **Step 1：Share module 验证**

```bash
cd backend/share
GOWORK=off go test ./... -count=1
GOWORK=off go test -race ./... -count=1
GOWORK=off go vet ./...
```

- [x] **Step 2：Tools、SaaS、Things 验证**

```bash
cd backend/tools && go test ./... -count=1 && go test -race ./... -count=1 && go vet ./...
cd ../saas && go test ./... -count=1 && go test -race ./... -count=1 && go vet ./...
cd ../things && go test ./... -count=1 && go test -race ./... -count=1 && go vet ./...
```

预期：全部 PASS；若出现迁移前已有失败，必须保留原始命令、退出码和迁移前对照，不能把失败改写成通过。

### Task 7：执行 Things 启动回归并发布本地 Evidence

**运行时文件：**

- 修改：`backend/saas/Makefile`
- 修改：`backend/things/Makefile`
- 修改：`backend/tools/internal/thingsstartbaseline/artifact_test.go`
- 读取：`deploy/profiles/things-baseline/.env.local`
- 生成但不跟踪：`deploy/.runtime/**`
- 写入本地研发仓库：`evidence/2026-08-27-ming-iot-local-share-migration/**`

- [x] **Step 1：运行快速静态基线**

```bash
make -C deploy things-baseline-fast
```

预期：Go 聚焦测试、Compose 配置、数据库保护扫描和凭据扫描全部通过。

仓库级 SaaS/Things 构建必须使用父级 `backend/go.work` 解析本地 Share，两个 Makefile 不得再以 `GOWORK=off` 关闭 workspace。`thingsstartbaseline` 工具自身仍可使用 `GOWORK=off` 保持工具模块隔离。

- [x] **Step 2：运行冷启动与协议探针**

```bash
make -C deploy things-baseline-down
make -C deploy things-baseline-up
make -C deploy things-baseline-verify
```

预期：SaaS 与 Things 五个服务镜像启动，PostgreSQL 独立数据库、Redis、etcd、NATS、TDengine 和 EMQX 探针通过；Evidence 不含凭据。

- [x] **Step 3：运行 warm restart 复核**

```bash
make -C deploy things-baseline-down
make -C deploy things-baseline-up
make -C deploy things-baseline-verify
```

预期：保留受保护数据库和数据卷后再次通过，且不存在 DROP DATABASE、DROP ROLE 或 TRUNCATE 行为。

- [x] **Step 4：形成 Evidence 摘要**

本地 Evidence 必须记录：冻结来源复核、实际 Git diff 路径集合、许可证文件摘要、5 个历史引用、module graph、各测试命令与退出码、冷/热启动运行目录、脱敏日志指针和未解决问题。不得复制凭据或大体积可再生日志。

### Task 8：独立 Review 与唯一用户验收 Gate

- [ ] **Step 1：交给独立 Reviewer**

Reviewer 使用独立 Agent 身份和干净上下文，核对：来源差异仅为 module/import path、许可证和 `SOURCE.json`；规范化后的 255 文件来源树摘要等于 `SOURCE.json.normalized_tree_sha256`；1548 个机械变更全部在 allowlist，额外修改只包含 `backend/go.work` 和 3 个启动适配文件；业务逻辑没有混入；旧 module graph 已消失；分层测试和 Things 启动 Evidence 可重放。

2026-08-28 曾执行一次无会话持久化的 Claude Code 只读辅助审查，但该进程没有绑定 LoopX 注册 Agent `ming-review-claude`，因此不能关闭本 Step；其技术意见只作为辅助材料，正式结论仍须由已注册 Reviewer Todo 产生。

还必须核对两个已知处置：

1. 上游 `backend/share/.gitignore` 保持冻结来源原样，因此会忽略同样来自上游的 `backend/share/go.sum`；最终获准暂存时必须对该文件执行强制暂存，并验证它已经进入 Git 索引；
2. `backend/share/oss/AWS_STS.md` 保留上游公开文档中的 AccessKeyID、ARN 与 bucket 示例标识。Secret 字段为占位符，未发现 SecretAccessKey 或私钥；该内容作为“上游非秘密标识”记录，不得把它误报成可独立认证的凭据，也不得在公开 Evidence 中复制具体标识值。

- [x] **Step 2：验证最终候选树**

```bash
git status --short
git diff --stat
git -c core.whitespace=cr-at-eol diff --check
git diff --name-only | sort -u
```

预期：只有本计划批准的机械迁移、workspace 与启动适配文件；没有缓存、运行时产物、凭据或来源仓库改动。`core.whitespace=cr-at-eol` 只用于识别两个来源文件已有的 CRLF，不得借此忽略普通尾随空白。

- [ ] **Step 3：建立一次汇总 User Gate**

向用户展示最终 `git status`、关键 Evidence、Reviewer 结论和风险。只有用户明确授权后才能 `git add`/`git commit`；merge 与 push 仍分别等待授权。

若用户随后明确授权暂存/提交，暂存动作必须额外执行并验证：

```bash
git add <本计划批准的其余路径>
git add -f backend/share/go.sum
git ls-files --error-unmatch backend/share/go.sum
git diff --cached --name-only -- backend/share/go.sum
```

预期：两个验证命令均只输出 `backend/share/go.sum`。未获得提交授权前不得为了验证而提前执行 `git add`。
