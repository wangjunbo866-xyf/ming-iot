# Thingsbaseline 隔离构建与 Go 模块代理配置设计

## 1. 背景

`thingsbaseline verify` 当前把 `things-build` 的物理输出写到仓库共享目录 `backend/bin/`，同时又要求该目录只能包含 `thingsvr`、`dgsvr`、`dmsvr`。因此，开发者本地已有的其他服务二进制虽然被 Git 忽略、不会上传，仍会令精确产物校验失败。

候选树渲染和门禁环境还固定使用 `GOPROXY=https://goproxy.cn,direct`。它会继续继承 `HTTP_PROXY`、`HTTPS_PROXY` 等传输代理变量，但无法按一次执行的需要选择其他 Go 模块镜像。

## 2. 目标与非目标

### 2.1 目标

1. `things-build` 不读取、覆盖或删除仓库的 `backend/bin/`。
2. 构建产物仍按精确契约校验，并在 Evidence 中保持 `backend/bin/{dgsvr,dmsvr,thingsvr}` 逻辑路径。
3. 为 Ming 仓库内的受控 Go 工具定义统一的 `MING_GOPROXY` 输入契约；默认行为保持 `GOPROXY=https://goproxy.cn,direct`，同时允许调用方显式选择其他受控模块镜像。
4. `HTTP_PROXY`、`HTTPS_PROXY` 及其小写形式继续传给 Go 子进程，使无法直连互联网的环境仍可工作。
5. 任何代理配置不得通过命令摘要或错误信息泄露凭据。

### 2.2 非目标

1. 不提交或上传 `backend/bin/` 中的服务二进制。
2. 不改变 `thingsbaseline` 的 canonical Evidence schema。
3. 不在本修复中实现跨运行 Go 缓存、下载重试或门禁加速。
4. 不继承任意全局 `GOPROXY`，避免重新引入未经声明的 GO 环境注入。
5. 不在只有一个消费者时创建尚无 Registry owner 的公共 `internal/toolenv` 包；出现第二个消费者后再单独设计共享实现与所有权。

## 3. 方案比较

### 3.1 采用：私有目录 + Ming 工具链覆盖变量

- 在现有 `.thingsbaseline-gates-*` 私有根目录下创建构建输出目录。
- `BIN_DIR` 和物理输出契约都指向该私有目录；`logicalRoot` 继续是 `backend/bin`。
- 新增仓库级输入 `MING_GOPROXY`；未设置时使用现有默认值，本轮先由 `thingsbaseline` 消费。
- 全局 `GOPROXY` 仍被过滤，传输层代理变量仍被继承。

该方案同时消除共享目录耦合和模块镜像硬编码问题，并保持 Evidence 语义稳定。

### 3.2 不采用：门禁前临时清空 `backend/bin`

这只能作为人工应急操作，会暂时改变开发者环境，并且容易在失败或中断后留下错误现场。

### 3.3 不采用：直接继承全局 `GOPROXY`

实现最少，但会让用户 shell、IDE 或宿主配置隐式改变门禁行为，削弱当前固定 GO 环境的可复现边界。

## 4. 详细设计

### 4.1 构建目录隔离

`buildDefaultVerificationGates` 已创建一次性 `cacheRoot`。在其中增加 `build-output` 目录，并把它同时用于：

- Make 参数 `BIN_DIR=<cacheRoot>/build-output`；
- `gateOutputContract.directory`；
- 产物打开、哈希、模式检查和精确文件名检查。

`gateOutputContract.logicalRoot` 保持 `backend/bin`，所以外部 Evidence 仍只表达发布时的逻辑产物位置，不暴露临时绝对路径。现有统一 redaction 包含 `cacheRoot`，现有 cleanup 在门禁结束后删除整个私有根目录。

仓库的真实 `backend/bin/` 不再进入读取、删除、写入或精确目录比较流程。

### 4.2 Go 模块镜像配置

新增 Ming 受控 Go 工具统一环境变量：

```text
MING_GOPROXY
```

它只控制本仓库工具启动的 Go 子进程，不配置业务服务运行时，也不替代 `HTTP_PROXY`、`HTTPS_PROXY`。当前优先级为：

```text
MING_GOPROXY > 默认 https://goproxy.cn,direct
```

未来某个工具若增加更明确的命令行参数，则优先级扩展为“工具参数 > `MING_GOPROXY` > 默认值”。

解析规则：

1. 未设置或为空时，使用 `https://goproxy.cn,direct`。
2. 设置后，值按 GOPROXY 条目解析；条目可由 `,` 或 `|` 分隔。
3. 每个条目只能是 `direct`、`off`，或不含 userinfo、query、fragment 的绝对 `https://` URL。
4. 非法配置在启动任何 Go 子进程前失败；错误只报告条目位置和失败类型，不回显原值。
5. `MING_GOPROXY` 本身不传给子进程；子进程只收到解析后的 `GOPROXY`。

这允许本次门禁显式使用：

```text
MING_GOPROXY=https://proxy.golang.org,direct
```

与此同时，当前 `HTTP_PROXY=http://127.0.0.1:7890` 和 `HTTPS_PROXY=http://127.0.0.1:7890` 仍按原逻辑继承。因此，切换模块镜像不会绕过全局网络代理。

### 4.3 Evidence 与日志

本修复不调整 Evidence schema：模块下载端点是执行环境，不改变迁移目标树、产物逻辑路径或内容摘要。成功 Evidence 继续依靠源码、目标树、命令日志和产物摘要证明结果。

代理覆盖值不得写入 Evidence，也不得原样进入错误摘要。是否使用覆盖由调用命令和本地运行上下文负责记录。

## 5. 错误处理与清理

1. 非法 `MING_GOPROXY`：在创建候选 Go 命令前返回配置错误。
2. 网络错误：保持现有 fail-closed 行为，不把下载失败误判为 PASS。
3. 构建失败或产物不符合契约：私有目录由现有 cleanup 回收。
4. 仓库 `backend/bin/`：无论门禁 PASS、FAIL、取消或超时，都不应发生变化。

## 6. 测试与验收

### 6.1 单元测试

1. 预先在仓库 `backend/bin/` 放置无关文件，默认门禁规格仍选择私有输出目录，并且 cleanup 后无关文件保持不变。
2. 物理输出目录位于 `.thingsbaseline-gates-*` 根内，而 `logicalRoot` 仍为 `backend/bin`。
3. 未设置 `MING_GOPROXY` 时继续使用默认 goproxy.cn。
4. `MING_GOPROXY` 可选择官方 Go 模块代理，并覆盖被隔离的全局 `GOPROXY`。
5. `HTTP_PROXY`、`HTTPS_PROXY` 继续存在于子进程环境。
6. 带凭据或非 HTTPS 的覆盖值被拒绝，错误不包含原始敏感值。

### 6.2 最终验收

1. `GOWORK=off go test ./internal/thingsbaseline -count=1`
2. `GOWORK=off go test ./cmd/thingsbaseline ./internal/thingsbaseline -count=1`
3. `graphctl scope-check`
4. 在 `MING_GOPROXY=https://proxy.golang.org,direct` 且保留系统 `HTTP(S)_PROXY` 的环境中执行 push tuple `graphctl ci`。
5. 确认本地 `backend/bin/` 文件清单和内容摘要在门禁前后不变。
