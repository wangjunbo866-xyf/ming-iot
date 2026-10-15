# Ming IoT 本地 Share 模块迁移提案

> 日期：2026-08-25
>
> Goal：`ming-iot-framework-v1`
>
> 状态：后继顺序和边界已获用户批准；尚未激活，不创建实现 Todo，不构成代码修改授权
>
> 推荐执行配置：`gpt-5.6-sol`，`reasoning=high`

## 1. 结论

在 Things 最小服务启动基线完成并留下可重复运行证据后，将只读上游仓库 `share` 的
`v1.6.4` 精确迁入 `ming-iot/backend/share`，把 module path 改为
`gitee.com/ming-iot/share`，并原子切换 `saas`、`things` 与迁入模块自身的全部引用。

迁移完成后，`ming-iot` 不再依赖或引用 `gitee.com/unitedrhino/share`，也不保留以旧 module
path 为入口的长期 `replace` 兼容层。

## 2. 已确认事实

- 来源仓库：工作区只读目录 `share/`；
- 来源 Tag：`v1.6.4`；
- 来源 commit：`ece02ca9d6defd63356931a59e653950f27ac572`；
- 当前 `ming-iot/backend/saas/go.mod` 与 `ming-iot/backend/things/go.mod` 都依赖
  `gitee.com/unitedrhino/share v1.6.4`；
- 2026-08-25 盘点时，`ming-iot/backend` 内有约 1539 个 Go 文件引用旧 Share import path；
- 来源 Share 自身有 80 个文件引用旧 module path；
- 来源 Tag 没有跟踪 LICENSE、NOTICE 或 COPYING 文件，许可证与发布边界必须在迁移实施前单独确认。

数量用于界定当前规模，实施时必须重新生成精确文件清单，不能把这里的快照数字当作机械改写依据。

## 3. Objective

建立由 `ming-iot` 自己拥有、可在同一 Go Workspace 中原子演进的公共基础模块：

```text
ming-iot/backend/share
module gitee.com/ming-iot/share
```

迁移只改变源码归属、module identity 和 import path，不改变 Share 的业务语义和公开 Go API。

## 4. Dependencies

1. Things 最小服务启动基线已经完成并可重复执行；
2. 当前 L0 Things 编译基线继续通过；
3. 来源 `share@v1.6.4` 的 commit、工作树状态和 tracked 文件清单重新冻结；
4. 许可证、版权归属和 NOTICE 处理方式获得明确结论；
5. 使用独立 worktree，原始 `share/` 仓库永久只读。

## 5. 迁移边界

建议 write scopes：

```text
backend/share/**
backend/saas/**
backend/things/**
backend/tools/**
backend/go.work
backend/go.work.sum
```

上述目录范围较宽，是因为 Go import path 变化会影响两个业务 module、Share 自引用和既有基线工具。
正式 Todo 必须在实施前根据 `rg` 结果生成精确 allowlist；不得借此修改与 import 切换无关的业务代码。

原子切换至少包括：

1. 复制来源 tracked 文件，排除 `.git`、缓存、构建物和本地凭据；
2. 将 `backend/share/go.mod` 改为 `module gitee.com/ming-iot/share`；
3. 将 `backend/go.work` 纳入 `./share`；
4. 改写 Share 自引用、SaaS、Things 和相关工具契约中的旧 import path；
5. 删除 SaaS、Things 对旧 Share module 的 require；
6. 更新 Things 迁移基线中仍以旧 Share module 为真值的合同和 fixture；
7. 在同一候选树中完成全部编译、测试和启动回归后再提交。

## 6. 强制约束

- 不使用 `replace gitee.com/unitedrhino/share => ./share` 掩盖未迁移的 import；
- 不允许新旧 Share module 同时进入同一构建图，避免相同结构在 Go 中成为两组不同类型；
- 不升级 Share 依赖、不清理 API、不重构内部目录；
- 不修改原始 `share/` 仓库；
- 不自行杜撰许可证；许可证结论不明确时，允许完成只读分析，但不得把来源代码发布到 Gitee；
- 不把本迁移扩张成 Core、Kafka、NATS、缓存或场景联动重构。

## 7. Acceptance

1. `ming-iot/backend/share` 的来源文件集与冻结清单一致，除 module/import、项目级来源说明和经批准的
   许可证处理外没有语义差异；
2. `backend/go.work` 正确解析 `share`、`saas`、`things` 和 `tools`；
3. `go list -m` 证明 SaaS、Things 使用本地 `gitee.com/ming-iot/share`；
4. 对 `ming-iot` 当前树执行旧路径扫描，`gitee.com/unitedrhino/share` 命中数为零；
5. Share、SaaS、Things、Tools 的相关单元测试、Race 测试和 Vet 分层通过；
6. Things 最小服务启动基线迁移前后结果一致，差异都有明确解释；
7. 在清空单独准备的 module download 测试环境后，构建不会请求
   `gitee.com/unitedrhino/share`；
8. 原始 `share/` 仓库工作树保持干净；
9. Evidence 记录来源 commit、精确文件清单、旧路径零命中、测试结果和许可证结论。

## 8. Non-goals

- 不升级 Share 到 `v1.6.4` 之后的代码；
- 不改变缓存、事件总线、存储、上下文和错误码语义；
- 不在本 Loop 中替换 NATS 或引入 Kafka；
- 不顺带迁移 `core`；
- 不把 Share 拆成多个 Go module；
- 不创建兼容代理仓库或长期双写依赖。

## 9. 主要风险

- 约 1500 个调用文件意味着 import 切换必须机械、可审计且一次闭合；
- 新旧 module path 并存会产生 Go 类型身份分裂，编译通过也可能在接口装配处出现隐蔽问题；
- Share 含缓存、事件、存储和客户端基础能力，必须用已有启动基线验证运行时行为；
- 来源 Tag 缺少 tracked LICENSE，是发布前的明确 Gate，不得被测试全绿替代。

## 10. 激活条件

本提案不自动进入实施。只有同时满足以下条件，Planner 才能创建 Share 迁移 Todo：

1. Things 最小服务启动基线完成并 Review 通过；
2. 用户重新批准本提案的实施 write scope 和许可证处理结论；
3. Planner 将迁移拆成来源冻结、候选树原子切换、验证和独立 Review；
4. LoopX 为该 Todo 显式记录 `recommended_model=gpt-5.6-sol`、
   `recommended_reasoning=high`。

