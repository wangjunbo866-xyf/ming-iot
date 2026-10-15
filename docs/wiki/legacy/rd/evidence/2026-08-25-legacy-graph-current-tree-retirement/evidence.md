# ming-iot 旧 Graph 当前树退役 Evidence

## 结论

- 日期：2026-08-25
- 仓库：`ming-iot`
- 分支：`retire-legacy-graph-current-tree`
- 起始提交：`4b80c51`
- Goal：`ming-iot-framework-v1`
- Agent Todo：`todo_bf3a0b21c91c`
- 实现提交：`151e193 refactor(graph): retire legacy local control plane`
- 结果：Gate D 当前树退役实现已 fast-forward 合并到本地 `master`，合并后验证通过；尚未 push。

## 实施范围

- 删除 `.graph/**`。
- 删除 `backend/tools/cmd/graphctl/**`。
- 删除 `backend/tools/internal/graphctl/**`。
- 删除 `backend/tools/README.md` 中旧 Graph 生命周期说明。
- 删除 `.gitignore` 中 `/.graph/runtime/`。
- 从 `backend/tools/go.mod`、`go.sum` 清理只供 graphctl 使用的 `jsonschema/v6` 及随之失去引用的间接依赖。
- 新增 `backend/tools/internal/repositorycontract/legacy_graph_retirement_test.go`，防止旧控制面文件或活动引用回流。

本轮未修改 `iot-doc`、`things`、IoT 业务代码、LoopX 安装目录、`docs/superpowers/**` 或 Git 历史。

## 基线证据

删除前执行：

```bash
cd backend/tools
GOWORK=off go test ./... -count=1
```

结果：通过。`graphctl`、`thingsbaseline` 及其它工具包在实施前均为绿色基线。

基线扫描确认旧控制面由 53 个 tracked 文件构成，其中包括 4 个 `.graph` 文件及 49 个 `graphctl` 源码/测试文件；README、`.gitignore` 和 Go 模块仍有活动引用。

## TDD 证据

先新增退役契约测试，再执行：

```bash
GOWORK=off go test ./internal/repositorycontract -run TestLegacyGraphRetired -count=1
```

RED 结果：按预期失败，明确报告三个旧目录仍含控制面，以及 `.gitignore`、README 仍含活动引用。

删除文件后首次 GREEN 尝试发现测试把 Git 不跟踪的空目录误判为旧控制面。根因确认后，契约改为检查退役路径下不得存在任何文件；空目录不属于可提交产物，也不会在重新检出后存在。再次执行相同命令通过。

## 完整验证

在 `backend/tools` 执行：

```bash
GOWORK=off go test ./... -count=1
GOWORK=off go test -race ./... -count=1
GOWORK=off go vet ./...
```

结果：三项全部通过；`thingsbaseline`、API contract、proto compatibility、RPC 生成与 SaaS transaction 等保留校验器继续通过。

残留扫描结果：

```text
legacy Graph index entries: all marked deleted (53 files)
legacy Graph working-tree files: none
active references: none
jsonschema dependency: none
```

`git diff --check` 通过，无空白错误。

## 本地合并与清理

用户选择“合并回本地 `master`”后执行：

```bash
git pull --ff-only
git merge --ff-only retire-legacy-graph-current-tree
```

结果：远端基线无需更新，`master` 从 `4b80c51` fast-forward 到 `151e193`。

合并时，主工作区暴露出此前被 `/.graph/runtime/` 忽略的 41 个旧 Dashboard/run 可再生文件，共 844 KiB。确认其均位于已批准退役范围后清除；随后重新执行全量测试、Race、Vet 和路径扫描，全部通过。功能 worktree 与功能分支已经删除，本地 `master` 工作区干净并领先 `origin/master` 1 个提交。

## LoopX 控制面验证

执行：

```bash
loopx doctor --agent-type codex-cli
loopx --format json --registry /root/.codex/loopx/registry.global.json status \
  --goal-id ming-iot-framework-v1 \
  --agent-id ming-peer-01 \
  --available-capability filesystem_write \
  --available-capability shell
```

结果：Doctor `ok=True`；状态合同 `errors=0`、`warnings=0`；当前 Todo 正确投影为旧 Graph 当前树退役，写边界与实施范围一致。

本地研发文档仓库同时执行：

```bash
python3 -m unittest discover -s tests -v
python3 tools/archive_manifest.py verify \
  --root archives \
  --manifest archives/manifests/archive-files.json
```

结果：8 项策略/归档测试通过，归档 manifest 校验通过；本轮新增计划、Evidence 和 Review 未修改封存的 `archives/**` 内容。

## 保留事项

- `docs/superpowers/**` 中的旧研发材料不属于本次 Gate D；其归档/删除与 Git 历史处理属于后续 Gate，不得混入当前提交。
- 本 Evidence 证明当前树的实施、提交、本地合并和验证结果，不替代独立 Review 或 push 批准。
