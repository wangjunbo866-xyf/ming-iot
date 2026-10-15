# R&D Graph 分层反馈控制面设计

> 日期：2026-08-18
> 状态：书面规格已确认，待提交与分阶段实施
> 适用仓库：`ming-iot`
> 依赖规格：`2026-08-16-rd-graph-local-control-plane-design.md`

## 1. 背景与问题

现有 G0 把受影响 Loop 的 required Gate 放在同一条同步 `graphctl ci` 路径中，并由 `pre-push` 调用。这个模型可以提供严格证明，但不适合作为团队的日常反馈回路。

2026-08-17 最近一次完整 PASS runtime 的实测数据为：

| 项目 | 耗时 |
|---|---:|
| 整次运行 | 578,410 ms，约 9 分 38 秒 |
| `G0-rd-graph-control-plane/graphctl-tests` | 313 ms |
| `L0-things-baseline/things-baseline` | 565,696 ms，约 9 分 26 秒 |

耗时主体不是 Graph 计算，而是 L0 完整迁移基线验证。把它放进每次 push，会把个人纪律变成所有团队成员的同步等待。独立 Review 若按每个微型 Task 串行执行，也会造成同样的问题。

本设计把“日常快速反馈”“完整机器验证”“独立 Review/完成批准”拆成三个时间尺度。严格性不取消，只从每次 push 的关键路径移到后台 FULL 与里程碑完成路径。

## 2. 目标

1. `pre-push` 的 FAST 全局硬预算为 90 秒，目标 P95 小于 60 秒。
2. 继续按 Graph 计算“直接受影响 Loop + 全部下游”，不牺牲依赖传播语义。
3. 慢 Gate 在固定 commit 的隔离环境中后台运行，不读取开发者后续修改。
4. FULL、Evidence、Review 继续 fail closed，但只阻止 Loop 完成、正式集成和发布，不阻止普通修复 push。
5. 团队和 Agent 的稳定入口压缩为 `context`、`check`、`prepare` 三个动作。
6. 核心协议不绑定 Sol、Luna 或任何具体模型；模型只是可替换执行者。
7. 所有页面和状态继续由机器 Registry/runtime 生成。
8. 修复 Bug 时无需创建任务级 Loop，也无需人工扫描 Loop 清单；路径到 owner/affected Graph 的映射由机器完成。

## 3. 非目标

- 本阶段不提供远端强制门禁、密码学签名或恶意本地用户防护。
- 本阶段不实现跨 commit 的智能 Gate 结果复用。
- 本阶段不自动执行 `git add`、`git commit`、`git push`、lifecycle closure 或 completion 写入。
- 本阶段不把普通后台 FULL 结果自动提交为 canonical Evidence。
- 本阶段不把 ECS 远端 Runner 混入本地控制面；它应作为独立后续 Loop 设计。
- 本阶段不把 Bug、需求单或单次提交建模为 Loop。

## 4. 核心术语与职责

### 4.1 FAST

FAST 是日常 `pre-push` 机器门禁：

- 输入是 Git Hook 提供的待推送 commit/range；
- 计算完整受影响图，但只执行 `executionClass=fast` 的 Gate；
- 90 秒是整次运行的总预算，不是每个 Gate 各 90 秒；
- PASS 只允许 push，不产生完成 Evidence，也不代表 Loop APPROVED；
- required FAST Gate 失败或总预算耗尽时阻止 push。

### 4.2 FULL

FULL 是后台或显式前台完整验证：

- 对同一受影响图执行所有 required Gate，包括 FAST Gate 与 FULL Gate；
- 绑定一个不可变的 `subjectCommit`；
- 普通 FULL 的 PASS/FAIL 写入可丢弃 runtime；
- 同一 commit 的合格 FULL PASS 才能成为 canonical Evidence 的输入；
- FULL 失败不阻止后续普通 push，但阻止 `prepare`、完成和发布。

### 4.3 REVIEW

Review 不是 Gate，不进入 `pre-push`：

- 普通微型修改只要求 TDD、FAST 和实现者自审；
- 一个可运行功能切片完成后，可以进行异步增量 Review；
- Registry、门禁、Evidence、安全、并发和数据迁移等高风险变更可以提前发起 Review，但不得占用 push 关键路径；
- Loop 完成或发布前必须具有绑定当前目标的独立 Review PASS；
- Review 超时只阻止完成路径，不阻止继续 push 修复。

### 4.4 COMPLETION

Completion 保持原有强语义：FULL canonical Evidence、Review、摘要新鲜度、依赖批准和用户明确授权必须同时满足。FAST PASS 不能替代其中任何一项。

## 5. Registry 机器契约

Registry 增加版本化执行策略：

```yaml
executionPolicy:
  version: fast-full-v1
  fastBudget: 90s
  fullMode: queued
  fullConcurrency: 1
```

每个 Gate 必须显式声明执行类别：

```yaml
gates:
  - id: graphctl-tests
    executionClass: fast
    required: true
    # 其余现有字段不变

  - id: things-baseline
    executionClass: full
    required: true
```

固定约束：

1. `executionClass` 只能是 `fast` 或 `full`，不得依靠默认值猜测。
2. FULL profile 包含 FAST Gate；FAST profile 不包含 FULL Gate。
3. required FAST Gate 的单 Gate timeout 不得大于全局 FAST budget。
4. `fullMode` 只能是迁移态 `manual` 或目标态 `queued`；`manual` 必须在页面显著显示 `FULL_MANUAL`。
5. Gate 类别、预算和调度语义进入 Registry digest；改变它们会使旧证明失效。
6. 旧版 `graphctl` 遇到新增字段必须因严格 Schema fail closed，不能静默忽略。
7. Registry 迁移必须在同一受控变更中为所有现有 Gate 补齐类别。

首轮迁移中：

- `G0/graphctl-tests` 为 FAST；后续 race、全量端到端检查为 FULL。
- `L0/things-baseline` 为 FULL。
- 若 L0 需要日常保护，应另外提取秒级契约/Schema 检查作为 FAST Gate，不能把完整基线重新放回 FAST。

## 6. 稳定的人机入口

团队和 Agent 只需要理解三个动作：

```text
graphctl context [--path PATH]   # 从候选路径或当前 Git diff 获取上下文
graphctl check                   # 执行 FAST；pre-push 自动调用
graphctl prepare --loop <ID> [--subject <COMMIT>]  # 进入/恢复里程碑流程
```

### 6.1 `graphctl context`

`context` 可以重复接收 `--path`。提供路径时用于修改前预判；未提供 `--path/--loop` 时自动读取 staged、unstaged 和非 ignored untracked Git diff。只有自然语言 Bug 描述而尚未定位代码时，允许先做只读搜索；找到第一个候选文件后必须调用 `context`，不得靠人工浏览所有 Loop YAML 猜 owner。

命令同时提供人类文本和严格 JSON，至少返回：

- policy/schema version；
- 每个输入路径的唯一 primary active owner；
- watches 命中的附加直接影响 Loop；
- 经 `dependsOn` 传播后的完整 affected Loop；
- 允许与禁止修改的路径；
- FAST budget；
- 将执行的 FAST Gate、历史耗时与预计总成本；
- 最近 FAST、FULL、Review、Completion 状态；
- `UNOWNED_PATH`、`AMBIGUOUS_OWNER` 和异常宽 fan-out；
- 当前阻断原因；
- 有序 `nextActions`。

Agent 不应自行推导 owner 或哪些 Gate 要运行，而应读取这个结果。`pre-push` 必须基于最终待推送 diff 重新计算一次权威影响图，不能信任开发前的预判结果。

### 6.2 `graphctl check`

`pre-push` 只调用 `check`。它完成范围计算、FAST 调度和同 commit 缓存。`fullMode=queued` 时还必须原子入队 FULL：FAST PASS 但入队失败时，以 `FULL_ENQUEUE_FAILED` 快速失败；后台 worker 暂时离线但队列已成功落盘时允许 push。迁移期 `fullMode=manual` 不入队，允许 push，但命令和页面都必须明确提示 FULL 尚需人工执行。

### 6.3 `graphctl prepare`

`prepare` 是显式里程碑操作，不是日常命令。它是幂等、可恢复的状态推进命令，不得在一个前台进程中等待九分钟 FULL 或等待 Reviewer。首次调用默认把当前 HEAD 固定为 `targetSubjectCommit`；后续调用使用第一次返回的 `--subject` 恢复同一流程。Evidence/Review 可以提交在后续控制提交中，但其中证明的 subject 始终是这个固定 commit。

每次调用前当前工作树必须干净；命令只推进到下一个异步或人工提交边界，并返回结构化 `READY/PENDING/BLOCKED`、关联 ID 和精确 `nextAction`：

1. 要求 `targetSubjectCommit` 真实可读，并确认当前干净 HEAD 的 scope、Registry、Validator 和 dependency digest 仍与目标证明一致；Evidence/Review 控制提交因不属于 subject 可以继续，任何实现或契约变化都返回 `BLOCKED/TARGET_STALE`，必须以新 HEAD 重新开始；
2. 缺少同一 commit 的 FULL PASS 时，创建或升级为不可取消的里程碑 FULL，立即返回 `PENDING/FULL`；
3. FULL PASS 后的下一次调用导出 canonical Evidence，但不自动 stage/commit，并返回 `PENDING/EVIDENCE_COMMIT`；
4. Evidence 已按授权提交后，下一次调用生成 runtime Review 请求，立即返回 `PENDING/REVIEW`；
5. Reviewer 生成 Review 文件后返回 `PENDING/REVIEW_COMMIT`，仍由用户授权提交；
6. 已提交 Review PASS 后的下一次调用输出 `READY` completion 候选；
7. 最终关闭、stage 和 commit 仍需用户逐次明确授权。

现有 `graphctl ci` 在迁移期保留为显式同步 FULL 诊断入口，不再由 Hook 调用，并输出弃用提示；稳定后是否删除由后续兼容决策决定。

## 7. FAST 调度

1. 先计算直接受影响 Loop 和全部下游 Loop。
2. 按 DAG 依赖顺序建立可运行集合。
3. 互不依赖的 FAST Gate 使用有界并行；并行度不得突破本机资源策略。
4. required FAST Gate 失败后，取消其尚未开始的下游；已经开始的命令必须按现有进程组清理协议结束。
5. 全局 deadline 到达后，未完成 Gate 记为 `FAST_BUDGET_EXCEEDED`，本次 push 失败。
6. 一个 Gate 经常逼近预算时，必须拆分或重分类为 FULL；不得扩大预算来掩盖设计问题。
7. FAST 运行仍生成 runtime 日志和 duration，供 P50/P95 统计，但不能导出 canonical Evidence。

## 8. FULL 队列与隔离执行

### 8.1 原子队列

队列位于 ignored `.graph/runtime/queue/`，通过临时文件、fsync 和原子 rename 发布。Job 至少绑定：

- job ID、profile、原因；
- subject commit 与 push ref；
- affected Loop 集合；
- Registry/control-plane digest；
- 创建时间与可取消标志。

同一 ref 的普通 push Job 只保留最新待运行 commit。被替代 Job 记为 `SUPERSEDED`。由 `prepare` 创建的里程碑 Job 不可被后续普通 push 取消。

### 8.2 Worker

本地 worker 默认 `fullConcurrency=1`：

- 低 CPU/IO 优先级运行重型 Gate；
- FAST 获得交互优先级；
- WSL/systemd 重启后扫描原子队列继续处理；
- worker 离线时页面显示 `QUEUED/WORKER_OFFLINE`，队列本身不丢失；
- stale/superseded Job 不得发布为当前 PASS。

### 8.3 固定 commit 隔离

后台 FULL 不能在开发工作树中运行。Worker 必须为 `subjectCommit` 创建临时隔离 checkout/worktree，并在运行前验证 HEAD、Registry 与 Job 一致。外部只读源必须解析为受信绝对路径或受控映射，不能因临时目录层级改变而解析到错误位置。

完成后只保留 runtime、日志和声明 artifact；临时工作树按明确路径安全清理。清理失败记录告警，不得递归删除宽泛目录。

## 9. 保守缓存

首版只复用“同一 commit + 同一 Gate 契约”的 PASS，不做跨 commit 智能复用。

缓存键至少包含：

- policy version、subject commit、Loop ID、Gate ID；
- scope、Registry、Validator、dependency digest；
- argv、workdir、env 与 runner/toolchain fingerprint；
- 外部源内容/版本摘要。

缓存位于 ignored `.graph/runtime/cache/`，可随时删除。只缓存完整 PASS；FAIL、BLOCKED、TIMEOUT 不缓存。缓存记录或其日志/artifact 缺失、损坏、摘要不一致时视为 miss 并重跑，绝不能降级为 PASS。

同一 commit 的 FULL 可以复用 FAST 已完成的同一 Gate，但 canonical Evidence 仍必须证明 FULL profile 的全部 required Gate 已满足。Go 自带编译/测试缓存继续正常使用，它与 Graph 结果缓存是两层不同机制。

## 10. Review 与模型解耦

核心协议定义的是角色，而不是模型名称：

```text
implementer
spec-reviewer
quality-reviewer
```

Sol、Luna、未来模型或人工 Reviewer 只是本地角色映射。模型/provider/version 可以写入 Review 审计元数据，但不能成为 Gate、摘要或批准协议的固定名称。

`AGENTS.md`、`CLAUDE.md` 或其他适配文件只保留短启动规则，并统一要求读取 `graphctl context`；不得复制 Registry、Gate 清单和状态推导。不同 Agent 是否识别某个说明文件，不影响 Hook、Schema、摘要和 `prepare` 的机器强制。

Review 使用上次批准点到目标 commit 的增量、当前规格/Evidence 和未关闭 finding，不重复审查整个项目。Reviewer 超时记录 `REVIEW_TIMEOUT`；普通开发继续，完成路径保持阻断。

## 11. 状态模型与失败语义

总控页面分别展示四条状态轴：

```text
FAST       PASS / FAIL / BUDGET_EXCEEDED
FULL       QUEUED / WORKER_OFFLINE / RUNNING / PASS / FAIL / SUPERSEDED
REVIEW     NOT_REQUIRED / PENDING / PASS / CHANGES_REQUIRED / TIMEOUT
COMPLETION OPEN / READY / APPROVED
```

规则：

- FAST FAIL/BUDGET_EXCEEDED：阻止当前 push。
- FULL_ENQUEUE_FAILED：快速阻止当前 push；已原子入队但 worker 离线不阻止。
- FULL FAIL：不阻止普通 push，阻止 `prepare` 完成、正式集成和发布。
- FULL SUPERSEDED：不是失败，也不能作为当前 PASS。
- REVIEW CHANGES_REQUIRED/TIMEOUT：不阻止普通 push，阻止完成。
- cache corruption：删掉/隔离损坏项并重跑，不产生 PASS。
- 未识别 policy/schema version：fail closed，并给出升级命令。
- 任一 runtime、Evidence 或 Review 输入变化：按各自摘要规则变为 STALE。

Dashboard 必须继续明确显示 `LOCAL_GATE`，不能暗示本地 Hook 具有远端分支保护的强制力。

## 12. 日常与里程碑数据流

### 12.1 日常 push

```text
developer/agent edits
  -> git push
  -> pre-push: graphctl check
  -> compute affected graph
  -> run/reuse FAST within 90s
  -> atomically enqueue FULL(subjectCommit)
  -> push proceeds
  -> isolated worker runs FULL asynchronously
  -> runtime/dashboard updated
```

### 12.2 Loop 完成

```text
graphctl prepare --loop Lx
  -> pin targetSubjectCommit; require clean current worktree
  -> no target FULL PASS: enqueue/upgrade non-cancellable FULL, return PENDING/FULL
  -> rerun with --subject after FULL PASS: export canonical Evidence,
     return PENDING/EVIDENCE_COMMIT
  -> user-authorized Evidence commit
  -> rerun with --subject: issue incremental Review request,
     return PENDING/REVIEW
  -> Reviewer produces Review; user-authorized Review commit
  -> rerun with --subject after Review PASS: derive READY completion candidate
  -> wait for explicit human authorization
  -> lifecycle/completion update and commit
```

## 13. 测试与验收

### 13.1 契约测试

- Schema 拒绝缺失/未知 `executionClass` 和非法 budget。
- FULL profile 精确包含 FAST+FULL required Gate；FAST 只包含 FAST required Gate。
- FAST runtime 不能被 canonical Evidence export 接受。
- policy/Gate class 变化使相关摘要与缓存失效。
- 未知 policy version fail closed。
- changed/untracked 路径零 active owner 或多 active owner 时 fail closed；重叠 `watches` 合法。
- closed Loop 不参与 active ownership，关闭前缺少 successor owner 时拒绝 closure。
- `context --path` 与 Git diff 模式产生相同、排序稳定的 owner/affected Graph。
- 当前 L0 回归：`internal/graphctl/**` 变化不再直接影响 L0，L0 真实输入变化仍会影响 L0。

### 13.2 调度测试

- 使用 fake clock/executor 锁定全局 90 秒预算，不运行真实慢命令。
- DAG 依赖、并行上限、失败取消和进程清理确定可重复。
- 同 ref 队列 coalescing、SUPERSEDED、不可取消 prepare Job 正确。
- 原子队列在进程崩溃与重启后不出现半个 Job。

### 13.3 隔离与安全测试

- FULL 只读取指定 commit，开发工作树随后修改不影响结果。
- 临时路径、symlink、外部源映射和清理边界 fail closed。
- runtime/cache 损坏不能伪造 PASS。
- `things/` before/after 状态完全一致。
- `backend/go.work.sum` 等用户已有修改不得被隐藏或自动修正。

### 13.4 端到端验收

- 当前 G0 FAST 在秒级完成。
- 修改 L0 作用域时，pre-push 不再同步执行九分钟 `things-baseline`。
- 同 commit 重复 push 命中保守 FAST 缓存。
- push 后 FULL 正确 QUEUED/RUNNING/PASS 或 FAIL。
- FULL FAIL 后仍可 push 修复；`prepare` 必须拒绝。
- FAST P95 小于 60 秒且任何运行不超过 90 秒硬截止。
- 页面完整显示四条状态轴和 `LOCAL_GATE`。

## 14. 实施拆分与分阶段迁移

本规格是控制面方向规格，不得实现成一个长时间、难审查的单体任务。实施拆成四个可独立验收的子项目，每个子项目拥有自己的实现计划和一次里程碑 Review：

| 子项目 | 范围 | 完成后价值 |
|---|---|---|
| O0 Ownership | `owns/watches`、唯一 active owner、closed 历史冻结、路径/diff context、收窄 L0 | Bug 无需建 Loop，避免无关 Loop fan-out/stale |
| F0 FAST 路径 | execution policy/class、`graphctl check`、90 秒预算、同 commit 缓存、Hook 切换 | 立即把九分钟 L0 Gate 移出 push 关键路径 |
| F1 FULL 执行器 | 原子队列、coalescing、隔离工作树、低优先级 worker、重启恢复 | 自动积累完整验证结果而不阻塞开发 |
| F2 Prepare/Review | 幂等 prepare 状态机、Evidence、Review request、四轴页面 | 形成低频但严格的完成路径 |

为保持实现与 Review 的时间盒，子项目可以拆成多个顺序计划，但不得用局部计划完成冒充子项目完成。O0 首先执行 O0a（active `owns/watches`、唯一 owner、路径/diff Context、收窄 L0），再执行 O0b（closed Loop 的历史证明投影与 closure handoff）；二者都验收通过后 O0 才完成。

F0 上线时先使用 `fullMode=manual`；F1 验收通过后在独立受控变更中切到 `fullMode=queued`。这样不会为了等待后台执行器而延迟最有价值的 FAST 改造，也不会假装尚未存在的自动化已经生效。

整体顺序为：

1. **收口当前安全加固**：保留 Task 18 已完成的 Gate proof 修复；采用一次时间盒复审，不恢复“每个微任务双重长审查”。
2. **Ownership**：加入 `owns/watches`、唯一 active owner、closed 历史语义和自动 context；首先消除 L0 的宽泛 `backend/tools/**/*.go`。
3. **Registry/Profile**：加入 execution policy/class、Schema 和 digest；显式迁移 G0/L0 Gate。
4. **FAST Hook**：实现 `graphctl check`、90 秒全局预算和同 commit 保守缓存；切换 Hook 后确认 L0 慢 Gate 不在关键路径。
5. **FULL Queue/Worker**：实现原子队列、coalescing、固定 commit 隔离、低优先级 worker 和重启恢复。
6. **Dashboard**：展示 FAST/FULL/REVIEW/COMPLETION 四轴状态和耗时分位。
7. **Prepare**：接入 FULL Evidence、增量 Review 请求和 completion 候选，不自动 commit/approve。
8. **后续远端 Loop**：在 ECS 或 Git 服务端建立不可本地绕过的远端门禁；不属于本规格实现范围。

在 FULL worker 上线前，`graphctl ci` 可继续作为人工显式 FULL；但 Hook 一旦切到 FAST，就必须由 `fullMode=manual` 驱动页面明确标注 `FULL_MANUAL`，不能假装已有自动后台验证。

## 15. 与现有 G0 加固任务的关系

本设计不撤销 2026-08-18 独立 Review 已发现的 Evidence、completion 和 runtime 信任边界问题：

- canonical Evidence 仍必须使用共享 Gate contract 重验；
- canonical JSON、completion subject proof 和安全 runtime loader 仍需完成；
- 新 FAST runtime 必须在后续 runtime hardening 中增加 profile-aware 精确证明；
- canonical Evidence 永远只接受 FULL profile。

因此 Task 18 的当前未提交修复是本设计的安全前置，不应丢弃；Task 21 的 runtime 设计必须在实施前按 FAST/FULL profile 调整，避免先实现“所有 runtime 都必须包含全部 Gate”后马上返工。

## 16. 强制力边界

本地 Graph 可以让正常开发者和协作 Agent 走统一 paved road，但无法阻止拥有同一 OS/Git 权限的执行者使用 `git push --no-verify` 或篡改本地工具。模型升级后，机器 Schema、Hook 和 CLI 仍保持一致；模型是否完整理解说明文档不再是主要正确性来源。

团队需要真正不可绕过的强制力时，必须增加受保护的远端接收门禁。此前页面和 Evidence 始终诚实标注 `LOCAL_GATE`。

## 17. Loop 粒度、所有权与历史新鲜度

### 17.1 Loop 不是任务单

Loop 表示长期组件、共享契约或有独立生命周期的阶段性迁移，不表示 Bug、需求单、一次提交或一次 AI 会话。

- 普通 Bug 在已有 active owner Loop 内修复，不创建新 Loop。
- Bug 跨多个组件时，所有直接 owner 分别进入 affected 集合，不默认增加“Bug 总控 Loop”。
- 只有协议 cutover、数据迁移、跨组件长期协调等本身需要独立 Gate/Evidence/Review/lifecycle 时，才创建协调 Loop。
- Issue ID、Bug ID 和任务描述可以作为 runtime/job 元数据，但不能成为 ownership 真源。

### 17.2 `owns` 与 `watches`

当前 `scope.include` 同时承担所有权和影响订阅，容易产生宽泛重叠。新策略把它拆开：

```yaml
scope:
  owns:
    - backend/tools/cmd/graphctl/**
    - backend/tools/internal/graphctl/**
  watches:
    - .graph/schemas/**
  exclude: []
```

语义如下：

1. 每个发生变化的受管实现路径必须匹配且只匹配一个 `lifecycle=active` Loop 的 `owns`。
2. `watches` 可以重叠，只表示该 Loop 需要进入直接影响集合，不赋予所有权。
3. 一个路径无人拥有时返回 `UNOWNED_PATH`；多个 active owner 命中时返回 `AMBIGUOUS_OWNER`。两者都必须在实现开始前或最迟 pre-push fail closed。
4. 共享代码必须由独立共享 Loop 拥有，消费者通过 `dependsOn` 或精确 `watches` 建立影响关系，不能用多个宽泛 `owns` 代替依赖建模。
5. `evidence.subjectPaths` 必须是 owner 的真实语义输入，不得用仓库级通配符图省事；`scope.owns`、`watches` 与 subject 的差异必须可在 context 中解释。
6. `globalImpact` 仍可使所有 Loop 受影响，但对应路径本身仍有唯一 owner。

`graphctl validate` 对当前 tracked 路径检查 active owner 冲突；`context/scope-check/pre-push` 对本轮全部 changed/untracked 路径执行零 owner/多 owner 的权威检查。

### 17.3 影响与 STALE 不是同一件事

路径变化后的处理分三层：

- **owned/watched/dependsOn affected**：决定哪些 FAST/FULL 需要运行；
- **current applicability**：scope、Registry、Validator 或 dependency digest 改变时，旧证明不再适用于当前代码；
- **historical validity**：已提交 Evidence 只证明其 `subjectCommit`，未来 HEAD 改变不会抹掉该历史事实。

因此“受影响 Loop 多”不等于“所有历史 Evidence 被否定”。页面必须分别表达 current STALE 与 historical PASS，不能把历史审计记录显示成损坏。

### 17.4 closed Loop 冻结

阶段性 Loop 关闭后成为不可变历史证明，不再充当未来代码的 active owner，也不因后续普通实现变化反复转为 STALE。关闭前必须保证仍会演进的每个实现路径已经由 active successor/component Loop 接管。

closed Loop 仍需校验其 completion、Evidence、Review 和 Git object 没有损坏或被篡改，但不再把当前 HEAD 的实现摘要与历史 subject 摘要比较。当前系统若要依赖持续演进的能力，应依赖 active component/successor，而不是把一个历史迁移 Loop 当作永久 owner。

### 17.5 当前 L0 修正

`L0-things-baseline` 是阶段性迁移基线，不应拥有整个 `backend/tools/**/*.go`。O0 必须：

1. 删除该宽泛 `scope` 与 `evidence.subjectPaths`；
2. 只保留 `thingsbaseline` 命令、实现、模板、lock 及通过依赖分析确认的真实共享输入；
3. 让 graphctl 代码只由 G0 拥有；
4. 对真正共享的 tools 包建立明确 owner/dependency；
5. 用回归测试证明修改 `internal/graphctl/**` 不再直接影响 L0，而修改 L0 的真实输入仍会影响 L0。

### 17.6 Loop 大小准则

Loop 不追求最大或最小，而追求“最小的、长期稳定、可独立验证和演进的能力边界”。目录层级、Bug 数量和团队任务单都不能直接决定 Loop 边界。

以下特征通常应保留在同一 Loop：

- 经常共同修改并具有高内聚；
- 使用相同 Gate 和验证成本；
- 具有相同发布、回滚和批准边界；
- 由同一长期组件承担语义责任。

以下信号说明应拆分：

- 两部分长期独立修改；
- 一部分适合秒级 FAST，另一部分只能执行十分钟级 FULL；
- 发布、回滚、数据迁移或安全批准边界不同；
- 宽泛 `owns/subjectPaths` 经常让无关修改产生 fan-out 或 current STALE。

推荐使用“组件 Loop + 聚合 Loop”：组件 Loop 唯一拥有实现代码；聚合/发布 Loop 可以没有组件代码 ownership，通过 `dependsOn` 和精确 `watches` 执行后台集成验证。聚合 Loop 若有自己的集成测试、部署清单或发布契约，只拥有这些聚合资产，不能再次拥有子组件源码。

遗留代码不要求在启用 Graph 前一次性完成理想拆分。允许先建立较粗但唯一、可解释的 active owner，再根据机器积累的 co-change、Gate duration、fan-out 和 stale 数据提出拆分建议。拆分始终是受审查的 Registry/架构变更，不能由模型自动重写 ownership。
