# Thingsbaseline 共享 Go 缓存设计

## 1. 背景

`thingsbaseline verify` 当前为每次候选渲染和最终门禁创建独立的
`HOME/GOCACHE/GOMODCACHE/GOTMPDIR`。该策略隔离充分，但每轮都会重新下载模块、重新编译依赖；外层
Graph 超时直接终止进程时，进程内的 defer 无法执行，还可能留下数 GiB 的
`.thingsbaseline-gates-*` 目录。

2026-08-19 的一次真实 L0 运行在 30 分钟后超时，遗留目录约 6.3 GiB，其中包括约 1.2 GiB
模块缓存、2.7 GiB 编译缓存和 2.5 GiB 临时构建数据。与此同时，宿主环境已有长期复用的
`GOCACHE` 和 `GOMODCACHE`。继续为每轮复制一套冷缓存没有必要，也不符合本机长期 Runner 和云端
CI 恢复远程缓存的常见执行模型。

## 2. 决策与适用范围

`thingsbaseline` 改为受控复用宿主 Go 的模块缓存和编译缓存，同时继续隔离会改变候选树、真实仓库或
构建产物的目录。

本规格替代《L0 Things 可编译迁移基线设计》中“每轮必须创建独立私有
`GOCACHE/GOMODCACHE`”的要求，但只替代缓存路径策略。以下约束继续有效：

- 两次 Render 必须使用不同 candidate、不同私有 `HOME` 和不同私有 `GOTMPDIR`；
- 两次 Render 的 manifest、tree digest、转换结果和受控命令结果必须一致；
- 真实 `backend/things` 只读，真实 `backend/bin` 不得被读取、覆盖或清理；
- 继承的任意 `GO*`、`GIT_*` 仍不得直接进入候选命令环境；
- 固定 Go 工具链、模块代理、校验和数据库和 Evidence 语义保持不变。

本次同时适用于 `thingsbaseline` 的候选 `go mod tidy`、`go mod verify` 和最终验证 Gate。它不修改
`saastxn`、`rpcgen` 等其他工具已有的一次性 `GOCACHE` 策略；这些工具若要共享缓存，必须单独验证其
随机路径行为。

## 3. 目标与非目标

### 3.1 目标

1. 跨 `thingsbaseline` 运行复用 `GOMODCACHE`，避免重复下载相同模块。
2. 跨运行复用 `GOCACHE`，避免重复编译相同依赖和包。
3. 所有测试每轮真实执行，不允许 Go test-result cache 代替本轮测试。
4. 保留私有 `HOME`、`GOTMPDIR`、candidate、临时 workspace 和构建输出。
5. 用真实回归测试证明共享编译缓存不会引用已删除的旧 candidate 路径。
6. 连续运行两轮 L0，分别验证正确性和热缓存耗时。

### 3.2 非目标

1. 不实现 FAST/FULL profile、队列、worker 或 Graph 结果缓存。
2. 不修改 pre-push Hook。
3. 不新增缓存服务、缓存清理守护进程或自动 `go clean`。
4. 不修改 canonical Evidence schema。
5. 不修改外部只读 `../things` 仓库。
6. 不自动删除已经遗留的 `.thingsbaseline-gates-*` 目录。

## 4. 方案比较

### 4.1 采用：共享两个 Go 缓存并增加随机路径回归保护

共享 `GOCACHE/GOMODCACHE` 能同时消除重复网络下载和重复编译。所有 `go test` 显式增加
`-count=1`，保证只复用编译结果，不复用测试结果。针对仓库中已有的旧 candidate 绝对路径故障记录，
必须先建立真实跨路径回归测试；测试不通过时不得发布该策略。

### 4.2 不采用：只共享 `GOMODCACHE`

该方案风险最低，但每轮仍需重建数 GiB 编译缓存，无法完整解决当前门禁耗时和磁盘写放大。它只作为
方案 4.1 被实证否定后重新设计时的候选，不允许实现过程静默降级。

### 4.3 不采用：固定 candidate 路径并增加全局锁

固定路径有利于编译缓存命中，但会引入并发串行、崩溃恢复、目录身份校验和锁租约等新问题，超出本次
缓存修复范围。

## 5. 缓存路径解析与环境契约

### 5.1 受信解析

工具使用已经完成版本校验和句柄保留的 Go 可执行文件运行：

```text
go env GOCACHE GOMODCACHE
```

解析发生在候选命令环境被净化之前，但返回值不直接作为完整环境继承。解析器只接受两个非空绝对路径；
路径存在时必须是目录且调用者可使用，路径不存在时允许 Go 按其正常语义创建。解析失败必须发生在创建
candidate 或启动 Gate 之前。

调用方显式设置的 Go cache 环境属于本机或 CI Runner 的运维配置，可以被受信 `go env` 解析；其他
`GO*` 值仍全部丢弃。缓存绝对路径不得进入 Evidence、命令展示或未脱敏错误摘要。

### 5.2 子进程环境

每轮继续创建私有目录：

```text
HOME=<run-private>/home
GOTMPDIR=<run-private>/gotmp
```

所有 Go 子进程显式使用解析出的共享路径：

```text
GOCACHE=<trusted shared build cache>
GOMODCACHE=<trusted shared module cache>
```

其余固定项继续保持：

```text
GOENV=off
GOFLAGS=
GOWORK=off
GOTOOLCHAIN=local
GOPROXY=<resolved MING_GOPROXY>
GOSUMDB=sum.golang.org
GOPRIVATE=
GONOPROXY=
GONOSUMDB=
GOINSECURE=
```

Git 配置隔离、locale 固定和 `HTTP_PROXY/HTTPS_PROXY` 传递及脱敏规则不变。共享缓存路径不得加入本轮
cleanup；私有根目录、临时 workspace 和 `build-output` 仍由本轮 cleanup 管理。

### 5.3 测试执行语义

以下四个 Gate 的 `go test` 参数必须显式包含 `-count=1`：

- `tools-test`；
- `saas-test`；
- `things-test`；
- `workspace-test`。

`-count=1` 禁止复用 test-result cache，但不会禁止 Go 复用模块下载和编译产物。`things-build` 继续
使用共享编译缓存和私有 `BIN_DIR`，不增加测试参数。

## 6. 一轮执行流程

一轮完整验证按固定顺序执行：

1. 用受信 Go 工具解析并校验共享 `GOCACHE/GOMODCACHE`。
2. 创建 Render 1 的私有 `HOME/GOTMPDIR/candidate`，执行 `go mod tidy` 和 `go mod verify`。
3. 创建 Render 2 的另一组私有目录，使用相同共享缓存再次渲染。
4. 比较两份 manifest、tree digest、转换/凭据/scanner 结果及受控命令结果。
5. 从第二个 sealed candidate 创建私有 verification tree。
6. 使用相同共享缓存执行四个 `go test -count=1` Gate 和一个隔离构建 Gate。
7. 完成 module-clean、target/source unchanged 等既有检查。
8. 清理本轮私有目录，不删除、截断或替换共享缓存。

缓存命中与否不参与产物可重复性判定。下载和编译日志可能因命中情况不同，但仍只作为本轮观察日志，
不得改变 manifest、tree digest 或 PASS 谓词。

## 7. 随机 candidate 路径回归保护

仓库已有 `saastxn`/`rpcgen` 经验：随机 candidate 路径使用共享 `GOCACHE` 时，旧缓存可能引用已删除目录，
导致 vet/compile 子工具报 `no such file or directory`。因此共享编译缓存必须由真实测试证明，而不能只用
fake runner 验证环境字符串。

回归测试使用测试专属共享缓存，步骤如下：

1. 创建随机路径 A 和一个可执行 vet/compile/test 的最小 Go fixture。
2. 使用共享测试 `GOCACHE/GOMODCACHE` 执行 `go test -count=1 ./...`。
3. 删除 A。
4. 在不同随机路径 B 写入内容相同的 fixture。
5. 使用同一共享测试缓存再次执行相同命令。
6. 要求第二次退出码为零，输出不引用路径 A，且测试确实执行而不是显示 `(cached)`。

实现完成后还必须用真实 L0 连续两轮验证该结论。任一回归失败时，方案 4.1 判定为不可交付：停止实现、
保留失败证据并回到设计阶段；不得静默切换为仅共享模块缓存，也不得忽略失败继续提交。

## 8. 并发、错误与清理

1. Go 的模块缓存和编译缓存负责其自身并发访问；本方案不增加全局锁。
2. 缓存路径无法解析、不是绝对路径、已存在但不是目录或不可使用时，在候选写入前失败。
3. 缓存内容损坏、权限变化或校验失败时，相关 Go 命令和整轮 Gate fail-closed。
4. 工具不得为了自动恢复而删除共享缓存；是否执行 `go clean -cache` 或 `go clean -modcache` 由用户明确决定。
5. 网络仅在模块缓存缺失时参与；网络或 checksum 失败继续 fail-closed。
6. 受控错误和取消会执行既有 cleanup。外层进程被强制终止时仍可能留下私有临时目录，但其中不再包含
   独立 `GOCACHE/GOMODCACHE` 副本；本规格不引入自动 stale-directory reaper。

## 9. 测试

### 9.1 单元与集成测试

1. 两次环境构造得到相同的共享 `GOCACHE/GOMODCACHE`。
2. 两次环境构造得到不同的私有 `HOME/GOTMPDIR`。
3. 恶意继承的其他 `GO*`、`GIT_*` 继续被过滤。
4. `MING_GOPROXY`、传输代理和敏感值脱敏行为保持不变。
5. 四个测试 Gate 均包含 `-count=1`，构建 Gate 不受影响。
6. 跨随机 candidate 路径的真实共享缓存回归通过。
7. 缓存路径不出现在 Evidence、展示命令或错误摘要中。
8. PASS、FAIL 和 cleanup 后真实 `backend/bin` 保持逐字节不变。

### 9.2 完整验收

1. 运行 `graphctl validate` 和 `graphctl scope-check`。
2. 运行 targeted tests、`go test ./...`、race test 和 `go vet ./...`。
3. 使用同一宿主缓存连续执行两轮真实 L0 Gate。
4. 第一轮证明功能正确并填充缺失缓存；第二轮证明热缓存复用。
5. 两轮均须 PASS，真实 `backend/bin` 文件清单与摘要前后不变。
6. 第二轮不得创建新的私有 `GOCACHE/GOMODCACHE`，并记录总耗时及各 Gate 耗时。

热缓存完整 L0 的体验目标是不超过约 3 分钟。该目标不是降低正确性门禁的理由：如果两轮均 PASS 但
热缓存仍超过目标，可以保留已经证明安全的缓存优化，但不得宣称 pre-push 体验已解决；后续再单独评估
FAST/FULL。若正确性或随机路径回归失败，则本方案不能交付。

## 10. 文档与迁移

实现时同步完成：

1. 在《L0 Things 可编译迁移基线设计》的私有缓存条款旁加入本规格的替代说明；历史计划保留原貌。
2. 更新 `backend/tools/README.md`，说明共享缓存、`-count=1`、缓存故障诊断和手工清理边界。
3. 不修改已有 canonical Evidence schema，也不把缓存路径加入 subject digest。
4. 当前约 6.3 GiB 的遗留目录保持不动；新实现验证完成后再由用户单独批准是否删除。
