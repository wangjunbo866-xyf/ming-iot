# goctl 可复现构建身份

## 上游身份

- Repository: `https://gitee.com/unitedrhino/go-zero.git`
- Commit: `6579a20bfd96919a80af5a9cb4e12f03598d4bb3`
- fork 源码硬编码版本字符串 `1.9.2-1`（`goctl --version` 输出）不是可信身份，以 commit + patches + checksum 为准。

## 构建环境

- Go toolchain: `go1.24.4`（GOTOOLCHAIN，本机 go1.25.4 自动拉取）
- Target: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`
- Build flags: `-trimpath -buildvcs=false -ldflags=-buildid=`
- Workspace: 全程 `GOWORK=off`（见下「GOWORK 隔离」），不继承上层 ming-iot go.work。

## 补丁集（按序应用）

| 补丁 | sha256 | 用途 |
| --- | --- | --- |
| `0001-api-accept-supper.patch` | `a9020f5492b181ee8abeb77cb5d46909040129615356dec5d9c97cce8e475e99` | validator 接受 `supper` authType（core 历史契约） |
| `0002-api-new-required-metadata.patch` | `96c5239916bd999e147237d86332dfe38b8a34af2c6561b4bccea9d29a1c47cc` | api new/apigen 模板默认生成七项必填 metadata |
| `0003-api-restore-ws-routes.patch` | `9bcf07ce1c5dc1ffc121ae823588000d35a6ffd3d9d10fc5373a79e28ad95341` | 恢复 `--ws` flag 与 wsRoutes.go 生成 |
| `0004-api-update-strict-validation-fixtures.patch` | `78405b19ea0663cdd537b6db7683787f7bd8c126afa7ea35c31c038292f84ff6` | fork 自身 fixtures 与 strict validator 一致 + validator quote 容忍 + GOWORK=off no-op |

## 校验和

- `sourceArchiveSha256`: `d77571bf37492f951ad7b5c6a3360f5eabb08d8e03b741451e22bdc40ee7a8c2`
  （`git archive --prefix=go-zero/ <commit>` 未压缩 tar，与 patches 无关）
- `binarySha256`: `aece99c46b9118ed9d069a1876cc5acf8eea5caa8e3e03601da7641a6f40f27f`
  （`backend/bin/goctl`）

## 确定性

两次连续 `bootstrap` 输出完全一致（patch/source/binary 六行 checksum `diff` 无差异）。

## 定向测试（全部 PASS，无 skip）

`build-goctl.sh` 在构建前运行 fork 定向测试 `./api/spec ./api/gogen ./api/new ./api/apigen ./api/swagger ./api/access ./rpc/generator`，全部通过：

```
ok  	github.com/zeromicro/go-zero/tools/goctl/api/spec
ok  	github.com/zeromicro/go-zero/tools/goctl/api/gogen
ok  	github.com/zeromicro/go-zero/tools/goctl/api/new
ok  	github.com/zeromicro/go-zero/tools/goctl/api/apigen
ok  	github.com/zeromicro/go-zero/tools/goctl/api/swagger
?   	github.com/zeromicro/go-zero/tools/goctl/api/access	[no test files]
ok  	github.com/zeromicro/go-zero/tools/goctl/rpc/generator
```

0004 补丁修复的内容（相对 plan 描述的偏差，已在 commit message 说明）：

1. **testdata fixtures**：给 14 个预期成功的 `testdata/*.api` 加合法七字段 `@doc(...)`；保留 `invalid_api_file.api`、`has_inline_no_exist_test.api`、`nest_type_api.api` 三个预期 parser 失败的 fixture 原样不动。
2. **内联 fixture**：`gencomment_test.go`、`gensse_test.go`、`newservice_test.go` 中写临时文件的 `.api` 内容补 `@doc(...)` 七字段。
3. **validator quote 容忍（production code 偏差）**：`api/spec/validate.go` 的 `getAtDocValue`/`getAnnotationValue` 增加 `unquoteDocValue`。根因：parser 在 `GoctlExperimental=off` 时保留 `@doc` kv 值的外层引号（`"admin"`），`=on` 时剥离；strict validator 直接做等值/枚举校验，而 `api/access` 生成器却用 `Unquote`，两边不一致导致 `experimental=off` 的测试必失败。与 access 生成器对齐后两种解析路径校验结果一致。这是 fork 自身 bug，非业务语义改动。
4. **GOWORK=off no-op（production code 偏差）**：`util/ctx/gowork.go` 的 `isGoWork` 在 `go env GOWORK` 返回 `"off"` 时返回 `false`，使 `UpdateGoWorkIfExist` 成为真正 no-op。根因：`UpdateGoWorkIfExist` 会执行 `go work use .`，在固定构建环境（GOWORK=off）下会报 "no go.work file found" 并使 rpc/generator stream client 测试失败；不设 GOWORK=off 又会污染上层 go.work。此修改让 `GOWORK=off` 语义自洽，配合构建脚本的 `export GOWORK=off` 实现零污染可复现构建。

## GOWORK 隔离

fork clone 落在 `backend/.goctl-patch-work/`，处于 ming-iot go.work 覆盖范围。`build-goctl.sh` 给全部 fork test/build 子进程显式 `export GOWORK=off`，使构建完全脱离 workspace、可复现且不污染 `backend/go.work`。

## 使用

```bash
# 首次或更新 checksum 后
bash backend/tools/scripts/build-goctl.sh bootstrap

# 日常构建（校验 checksum）
bash backend/tools/scripts/build-goctl.sh build
backend/bin/goctl --version   # goctl version 1.9.2-1 linux/amd64
```

`backend/bin/goctl` 被 `.gitignore` 忽略。
