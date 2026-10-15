# ming-iot 旧 Graph 当前树退役实施计划

> **供 Agent Worker 使用：** 必须使用 `superpowers:executing-plans` 逐项执行；修改行为必须遵守 `superpowers:test-driven-development`，完成声明前必须使用 `superpowers:verification-before-completion`。

**目标：** 从 `ming-iot` 当前树完整移除已经被 LoopX 取代的 `.graph` 与 `graphctl` 动态控制面，同时保留 `thingsbaseline` 等确定性工程校验器。

**架构：** 先用仓库契约测试锁定退役后置条件并观察 RED，再删除旧控制面、清理活动文档和 Go 依赖，最后以聚焦测试、全量测试、Race、Vet 和残留扫描形成可复查证据。历史重写与 `docs/superpowers/**` 清理不属于本次 Gate D。

**技术栈：** LoopX CLI、Git worktree、Go 1.24.4、Bash、`apply_patch`。

---

## 执行边界

- 实施 worktree：`/home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/retire-legacy-graph-current-tree`
- 实施分支：`retire-legacy-graph-current-tree`
- Goal：`ming-iot-framework-v1`
- Agent Todo：`todo_bf3a0b21c91c`
- 权威规格：`specs/2026-08-23-ming-iot-rd-docs-archive-and-legacy-graph-retirement-design.md`
- 不修改 `iot-doc`、`things`、IoT 业务代码、LoopX 安装目录或 Git 历史。
- 不删除 `thingsbaseline`、API/schema/generated consistency 等确定性校验器。
- `docs/superpowers/**` 是待后续处理的旧研发材料，不作为本次活动引用扫描对象。
- 本计划不授权 `git add`、commit、merge、push、生产操作或历史重写。

## 文件变更图

| 路径 | 操作 | 责任 |
| --- | --- | --- |
| `.graph/**` | 删除 | 移除旧 Registry、Loop 定义和 schema |
| `backend/tools/cmd/graphctl/**` | 删除 | 移除旧 Graph CLI |
| `backend/tools/internal/graphctl/**` | 删除 | 移除旧 Graph 实现和测试 |
| `backend/tools/internal/repositorycontract/legacy_graph_retirement_test.go` | 新增 | 防止旧 Graph 活动控制面回流 |
| `backend/tools/README.md` | 修改 | 删除 graphctl 生命周期说明 |
| `.gitignore` | 修改 | 删除 `.graph/runtime` 专用忽略规则 |
| `backend/tools/go.mod`、`backend/tools/go.sum` | 修改 | 清理只被 graphctl 使用的依赖 |

## Task 1：确认工作区与测试基线

- [x] 在 worktree 执行 `git status --short`，确认没有实施前的意外修改。
- [x] 运行 `GOWORK=off go test ./... -count=1`（工作目录 `backend/tools`）。
- [x] 若基线失败，停止删除并记录具体失败；不得把既有失败伪装为退役结果。
- [x] 用 `git ls-files` 和 `rg` 记录 `.graph`、`graphctl`、README 活动引用及 `jsonschema` 依赖基线。

## Task 2：以 TDD 锁定退役后置条件

**文件：** `backend/tools/internal/repositorycontract/legacy_graph_retirement_test.go`

- [x] 新增仓库契约测试，要求以下 tracked 路径不存在：`.graph/**`、`backend/tools/cmd/graphctl/**`、`backend/tools/internal/graphctl/**`。
- [x] 测试活动文件 `.gitignore`、`backend/tools/README.md`、`AGENTS.md` 不再包含 `graphctl`、`.graph/runtime`、`.graph/registry`、`.graph/loops`。
- [x] 运行 `GOWORK=off go test ./internal/repositorycontract -run TestLegacyGraphRetired -count=1`。
- [x] 确认测试因旧 Graph 仍存在而失败，记录 RED 证据后才进入删除步骤。

## Task 3：删除旧 Graph 当前树

- [x] 使用 `apply_patch` 删除 `.graph/**`。
- [x] 使用 `apply_patch` 删除 `backend/tools/cmd/graphctl/**`。
- [x] 使用 `apply_patch` 删除 `backend/tools/internal/graphctl/**`。
- [x] 删除 `backend/tools/README.md` 中 `R&D Graph local control plane` 及其后续旧生命周期说明。
- [x] 从 `.gitignore` 删除 `/.graph/runtime/`。
- [x] 不触碰 `docs/superpowers/**` 和任何历史提交。

## Task 4：清理依赖并获得 GREEN

- [x] 在 `backend/tools` 执行 `GOWORK=off go mod tidy`。
- [x] 确认 `github.com/santhosh-tekuri/jsonschema/v6` 已从 `go.mod/go.sum` 移除。
- [x] 确认仍被其它工具使用的 `gopkg.in/yaml.v3` 等依赖保留。
- [x] 运行聚焦契约测试并确认通过。

## Task 5：完整验证与残留扫描

- [x] `GOWORK=off go test ./... -count=1`
- [x] `GOWORK=off go test -race ./... -count=1`
- [x] `GOWORK=off go vet ./...`
- [x] 使用 `git ls-files` 确认三个退役路径无 tracked 文件。
- [x] 使用 `rg` 扫描活动文件，确认旧 Graph 不再是当前控制面；旧 `docs/superpowers/**` 只记为后续历史材料清理项。
- [x] 运行 LoopX `doctor`/状态读取，确认当前 Goal 仍由 LoopX 接管。

## Task 6：Evidence、Review 与交接

- [x] 写入 `evidence/2026-08-25-legacy-graph-current-tree-retirement/evidence.md`，记录基线、RED、GREEN、完整验证和残留范围。
- [x] 写入 `reviews/2026-08-25-legacy-graph-current-tree-retirement-review.md`；若没有独立 Reviewer，必须明确标注“执行者自检，待独立 Review”，不得冒充独立评审。
- [x] 回写 LoopX 本轮 validated progress，但在用户批准前不关闭提交/合并边界。
- [x] 展示两个仓库的 `git status --short`、关键 diff 和验证结果，等待用户单独授权提交。
