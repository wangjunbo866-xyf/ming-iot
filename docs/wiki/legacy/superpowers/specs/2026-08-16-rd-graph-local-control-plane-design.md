# 研发 Graph 本地控制平面设计

状态：书面规格已批准，待实施计划
日期：2026-08-16

## 1. 背景与决策

`ming-iot` 已完成 L0 Things 可编译迁移基线，但 Loop 的范围、依赖、门禁、证据和批准状态仍分散在规格、计划、Evidence、工具输出和手写 HTML 中。后续物模型、Kafka、Reducer、缓存与 Scene 会形成更多相互依赖的 Loop；如果继续依赖对话记忆和手工页面，AI 与开发者容易漏跑门禁、越过范围、复用过期证据，页面也会逐渐偏离仓库真实状态。

本设计建立一个仓库内、机器可读、可持续扩展的研发 Graph 本地控制平面。采用以下已确认方案：

- workspace 根 `AGENTS.md` 保留跨仓协作规则，新增仓库内 `ming-iot/AGENTS.md` 固化可随代码复用的项目规则；
- `.graph/` Registry 是 Loop 与依赖关系的唯一机器真源；
- `graphctl` 是薄编排器，领域判断继续由独立 Validator 负责；
- 本地 `pre-push` 执行 Push Gate，替代当前不可用的 Gitee CI；
- Validator、独立 Reviewer 与用户批准三者互不替代；
- `127.0.0.1:18081` 的页面完全由 Registry 和运行结果生成；
- 页面必须明确标识 `LOCAL_GATE`，不得冒充远程独立 CI。

这套控制平面与主流 Agent 工程的公开模式一致：持久规则、可复用工具、确定性门禁、独立评审和持续证据共同约束 AI；但 `.graph` Registry、Loop 状态和总控页面是 `ming-iot` 的项目级研发治理设计，不宣称复制任何厂商内部实现。

## 2. 目标

G0 交付以下能力：

1. 用机器 Registry 声明 Loop、直接依赖、修改范围、机器门禁、Evidence 和 Review 约束；
2. 把 G0 控制平面自身和既有 L0 都注册为 Loop，并由机器生成当前 Graph 与总控页面；
3. 根据 Git 变更自动计算“直接受影响 Loop + 全部下游依赖”；
4. 用同一条 `graphctl ci` 命令执行本地手动检查和 `pre-push` 门禁；
5. 保留现有 `thingsbaseline`，由 `graphctl` 编排而不是重写其领域逻辑；
6. 区分机器门禁、独立评审和用户生命周期，禁止“测试通过即自动批准”；
7. 产生机器可读状态和可浏览页面，为未来接入任意远程 CI 保留稳定入口。

## 3. 非目标

G0 明确不做：

- 不接入付费 Gitee CI，也不搭建 ECS 自托管 CI；
- 不自动创建、切换、合并或删除 Git worktree；
- 不自动调用 AI Reviewer，不实现完整 `approve/close` 命令状态机；
- 不并行执行 Validator；
- 不注册 L1 以后各业务 Loop 的实际门禁；
- 不替换或弱化 `thingsbaseline`；
- 不引入数据库、消息队列、远程控制面或页面写接口；
- 不修改联犀官方 `things/` 仓库；
- 不执行 `go work sync`；
- 不让 `graphctl` 自动 commit 或 push。

## 4. 四层架构

```text
规则层
  AGENTS.md + 已批准规格
        ↓
控制层
  .graph Registry + graphctl
        ↓
验证层
  Validator + Reviewer + 用户批准 + Evidence
        ↓
展示层
  .graph/runtime + 127.0.0.1:18081
```

各层职责如下：

- **规则层**描述稳定的行为边界和设计意图；规则不能替代可执行检查。
- **控制层**解析机器契约、计算依赖和影响范围、编排检查并推导状态。
- **验证层**分别提供确定性结果、语义评审和最终人工决策。
- **展示层**只呈现派生状态，不接受写操作，也不成为真源。

## 5. Registry 布局与真源边界

仓库根目录新增：

```text
ming-iot/.graph/
├── registry.yaml
├── loops/
│   ├── G0-rd-graph-control-plane.yaml
│   └── L0-things-baseline.yaml
├── schemas/
│   └── loop-v1.schema.json
└── runtime/                  # graphctl 生成，Git 忽略
    ├── index.html
    ├── graph-status.json
    ├── latest-run.json
    └── runs/
```

Registry 是一个由 `registry.yaml` 入口和 `loops/*.yaml` 组成的版本化机器契约。`graphctl` 必须按规范路径排序后加载全部 Loop；同一输入必须得到同一 Graph。

Registry 只保存声明性真源：

- Loop ID、标题和生命周期；
- 直接依赖 `dependsOn`；
- include/exclude 修改范围；
- 只读外部来源；
- required Gate；
- canonical Evidence 与 Review 约束；
- 用户授权后的 completion 引用。

`scope.include` 表示 Loop 拥有和允许触发影响分析的全部路径，可以包含规格、Evidence 和 Review；它不等于 Evidence 所证明的代码集合。Evidence 必须另行声明 `subjectPaths`，只包含被证明的实现、测试和契约输入。

普通实现路径可以被多个 Loop 的 include 匹配；此时它们都是直接受影响 Loop。只有 `.graph/loops/<ID>.yaml` 必须由同 ID Loop 独占。任何 canonical Evidence、Review 和 completion 引用都必须落入引用方 Loop 的 include，否则 Registry 校验失败。

Registry 不保存最近一次 `PASS/FAIL`、运行耗时、日志或 Web 服务状态。这些是派生数据，只能进入 `.graph/runtime/`。

`registry.yaml` 还必须把 `G0-rd-graph-control-plane` 声明为 `controlPlane.ownerLoop`。G0 拥有控制平面实现，但只有以下真正改变所有 Loop 解释、执行或强制方式的路径进入 `controlPlane.globalImpact`：

```text
.graph/registry.yaml
.graph/schemas/**
.githooks/**
AGENTS.md
.gitignore
backend/tools/cmd/graphctl/**
backend/tools/internal/graphctl/**
backend/tools/go.mod
backend/tools/go.sum
```

命中 `controlPlane.globalImpact` 时，`graphctl` 必须选择 G0 和全部非 planned Loop；全局控制平面文件没有同时落入 G0 的 `scope.include` 时，scope-check 必须 fail closed。G0 自己的 Loop 清单、设计规格和只影响 G0 的测试仍由 G0 普通 scope 拥有，不进入全局影响集合。

`.graph/loops/<ID>.yaml` 不是全局影响文件。每个 Loop 必须在自己的 `scope.include` 中显式拥有且只拥有对应清单；修改它只选择该 Loop 及其下游。`graphctl validate` 无论本次选择哪些 Loop 都先校验完整 Registry，因此未被选中的非法清单也不会漏过。这样新增 Evidence 引用或关闭单个 lifecycle 不会无意义地重跑全图。G0 作为页面中的真实 Loop 承载控制平面自身的 Gate、Evidence、Review 和 lifecycle，但“控制平面治理所有 Loop”是 Registry 的特殊影响规则，不伪造成所有业务 Loop 到 G0 的普通领域依赖边。

`ming-iot/AGENTS.md` 由仓库版本控制，workspace 根 `AGENTS.md` 继续提供本机跨仓规则，两者职责不同。

所有 Registry 内项目路径默认相对 `ming-iot` Git 根目录，使用 `/` 分隔。G0 glob 只支持字面路径、匹配单个路径段内字符的 `*`，以及匹配零到多个完整路径段的 `**`；拒绝字符类、反斜杠、空段、`.` 和 `..`。外部来源必须显式声明，G0 只允许形如 `../<workspace-sibling>` 的规范路径，例如 `../things`，解析后必须位于 `ming-iot` 父目录内且不能经符号链接逃逸；外部来源只参与只读输入和基线校验，不进入 `ming-iot` 的 Git diff。路径解析必须拒绝空路径、绝对路径、重复规范路径和指向错误类型的目标。

### 5.1 仓库内 AI 执行协议

`ming-iot/AGENTS.md` 必须把 Graph 工作流写成所有 AI 都能重复执行的仓库协议，而不是依赖某次对话记忆：

1. 每个实现任务必须先确定一个 owner Loop，并执行 `graphctl validate` 和 `graphctl context --loop <ID>`；
2. 如果目标不属于任何已有 Loop，或者需要改变依赖、范围、Gate 与完成条件，必须先走“设计规格获批 → Registry 变更”的流程，不能直接编码；
3. 实施期间只允许修改 owner Loop 及其本次批准的影响闭包所覆盖的路径；发现越界需求必须停下并重新设计；
4. 声称完成前必须对当前目标执行 `graphctl scope-check` 和 `graphctl ci`，并报告 run ID、受影响 Loop、Gate 结论和报告路径；历史 PASS 只要摘要不匹配就不可复用；
5. AI 不得自行把 lifecycle 改成 `closed`，不得把机器 PASS 表述成用户 APPROVED，也不得绕过 Hook、自动 commit 或 push。

`AGENTS.md` 是每次工作的入口和行为约束，`graphctl` 与 Hook 是可执行约束。两者必须引用同一 Registry，禁止在 `AGENTS.md` 中复制一份容易漂移的 Loop 清单或 Gate 命令。

### 5.2 Loop 最小契约

```yaml
apiVersion: graph.ming.io/v1
kind: Loop

metadata:
  id: L0-things-baseline
  title: Things 可编译迁移基线

spec:
  lifecycle: active
  dependsOn: []

  scope:
    include:
      - .graph/loops/L0-things-baseline.yaml
      - backend/things/**
      - backend/tools/cmd/thingsbaseline/**
      - backend/tools/internal/thingsbaseline/**
      - backend/tools/things-baseline.lock
      - backend/tools/templates/things-baseline/**
      - backend/tools/go.mod
      - backend/tools/go.sum
      - docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.json
      - docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.md
    externalSources:
      - path: ../things
        access: read-only

  gates:
    - id: things-baseline
      kind: command
      required: true
      validator:
        id: thingsbaseline
        version: v1
        sourcePaths:
          - backend/tools/cmd/thingsbaseline/**
          - backend/tools/internal/thingsbaseline/**
          - backend/tools/go.mod
          - backend/tools/go.sum
      workdir: backend/tools
      argv:
        - go
        - run
        - ./cmd/thingsbaseline
        - verify
        - --lock
        - ./things-baseline.lock
        - --source
        - ../../../things
        - --backend
        - ..
        - --evidence
        - ${RUN_DIR}/l0-things-evidence.json
      env:
        GOWORK: "off"
      timeout: 30m

  evidence:
    requiredForCompletion: true
    subjectPaths:
      - backend/things/**
      - backend/tools/cmd/thingsbaseline/**
      - backend/tools/internal/thingsbaseline/**
      - backend/tools/things-baseline.lock
      - backend/tools/templates/things-baseline/**
    canonical:
      - docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.json
      - docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.md

  review:
    requiredForCompletion: true
```

上例用于固定契约形状。实施必须从 L0 已有规格、计划、最终提交和 canonical Evidence 的实际路径集合生成候选 `scope.include`，人工审查后写成显式 allowlist；不得把示例列表直接当作最终 allowlist。`evidence.subjectPaths` 必须单独审查，禁止匹配 `.graph/loops/**`、`.graph/runtime/**`、canonical Evidence、Review 或 completion 元数据。L0 历史 Evidence 与 completion 摘要校验完成后，最终 Registry 才恢复 `lifecycle: closed`。

Gate 禁止保存一整段 Shell 字符串。`argv` 按参数数组直接传给进程；G0 只支持由 `graphctl` 进行的有限占位符替换。含占位符的参数只能是 `${RUN_DIR}` 或 `${RUN_DIR}/<clean-relative-path>`，不得出现其他变量或同一参数内的其他前后缀；解析后必须仍位于本次运行目录内。禁止 shell expansion、命令替换、管道、重定向和隐式 `eval`。每个 Gate 必须声明稳定的 Validator ID、协议版本和实现源码范围；源码范围改变后必须使旧运行结果失效。

`validator.sourcePaths` 与 `validator.artifacts` 的坐标系不同：前者是仓库源码 glob，必须受 Loop scope 约束；后者是 Gate 运行目录中的必需输出，只允许不含 glob、变量或路径逃逸的精确相对文件路径，不参与仓库 scope 覆盖校验。

## 6. graphctl 边界

`graphctl` 放在现有工具 module：

```text
backend/tools/cmd/graphctl
backend/tools/internal/graphctl/registry
backend/tools/internal/graphctl/graph
backend/tools/internal/graphctl/changes
backend/tools/internal/graphctl/scope
backend/tools/internal/graphctl/runner
backend/tools/internal/graphctl/evidence
backend/tools/internal/graphctl/review
backend/tools/internal/graphctl/render
```

`graphctl` 只理解 Loop、依赖、范围、Gate、Evidence 和 Review，不理解 Things、Kafka、物模型、Reducer 或 Scene 的业务语义。领域知识必须留在对应 Validator 中。

### 6.1 G0 命令

| 命令 | 行为 |
| --- | --- |
| `graphctl validate` | 校验入口、Schema、ID、路径、Gate、依赖存在性和 DAG |
| `graphctl context --loop ID` | 输出 Loop 范围、依赖、Gate、Evidence 和当前摘要，供人或 AI 使用 |
| `graphctl scope-check` | 根据指定 Git 范围识别变更、直接影响和下游影响并检查越界 |
| `graphctl status` | 读取 Registry、Evidence、Review 与最近运行记录，推导状态但不执行 Gate |
| `graphctl ci` | 执行本次受影响 Loop 及其全部下游的 Push Gate |
| `graphctl ci --all` | 按拓扑顺序执行全部 active/closed Loop 的 Push Gate |
| `graphctl evidence export --loop ID --run ID --output DIR` | 把一个可作为完成证据的干净 PASS 运行导出为不可覆盖的 canonical Evidence bundle |
| `graphctl render` | 原子生成机器 JSON 和只读 HTML |

所有查询类命令同时支持人类输出和 `--format=json`。机器输出必须有版本字段、稳定字段名和明确错误结构；普通日志不得混入 stdout 的 JSON。

### 6.2 ci 流程

```text
validate
  → 解析 Git base/head
  → 计算直接受影响 Loop
  → 沿反向依赖扩展全部下游
  → scope-check
  → 按拓扑顺序串行执行 required Validator
  → 校验本轮 runtime Evidence；校验已存在 canonical Evidence/Review 的新鲜度
  → 读取但不自动生成 Review
  → 写入 runtime 报告
  → render
```

G0 串行执行，优先获得稳定日志与简单故障定位。某个上游 required Gate 失败后，其依赖者标记 `BLOCKED` 并跳过，互不依赖的其他分支仍可继续，以便一次运行暴露更多独立问题。

每个 Gate 获得独立运行目录。`thingsbaseline verify` 的 `--evidence` 必须指向该目录，不能由日常 `graphctl ci` 覆盖已跟踪 canonical Evidence。`graphctl` 生成通用运行记录，领域 Validator 负责其领域 Evidence 内容。

Gate 日志最多保留 1 MiB 尾部。运行记录同时保存完整 stdout/stderr 合并字节流的 `logSha256`、完整字节数，以及磁盘保留尾部的 `retainedLogSha256`；canonical 导出校验后者对应的实际文件，前者继续证明运行时观察到的完整输出，禁止拿尾部文件冒充完整流重算。

`graphctl ci` 成功只表示 Push Gate 通过，不表示 Review 或用户批准完成。canonical Evidence 或 Review 缺失时，应在状态中显示待完成，但不能把普通修复提交的 Push Gate 判为失败。

## 7. 变更范围与依赖传播

Loop 只声明直接依赖。`graphctl` 构造 DAG、校验无环并生成反向索引。

默认执行范围是：

```text
变更文件命中的 Loop
  + 所有直接或间接依赖这些 Loop 的下游
```

不得要求开发者手工指定受影响 Loop。`--all` 是显式全量模式。

手工执行 `graphctl ci` 时，默认基准是 `merge-base(origin/master, HEAD)`，并把已提交、暂存和工作树变更纳入本地诊断。`pre-push` 必须使用 Git Hook stdin 提供的本地与远端 SHA：已有远端分支以远端 SHA 和本地 SHA 的 merge-base 为基准；新分支回退到 `origin/master` 与本地 SHA 的 merge-base。删除 ref 不执行代码 Gate，但仍校验 Hook 输入格式。

G0 的 `pre-push` 只接受一个非删除 ref，且要求该 ref 的 local SHA 等于当前 `HEAD`；同时要求 tracked、staged 和非 ignored untracked 状态为空。否则当前 worktree 不能证明待推送 commit，Hook 必须拒绝并给出原因，不能拿当前目录的测试结果替另一个 commit 背书。多 ref push、推送非当前分支和脏 worktree 的隔离验证留到未来自动 worktree Loop。

scope-check 的职责是证明本次变更被至少一个受影响 Loop 的 include 接受，且未命中任何禁止范围。外部只读仓库的脏状态不能作为 `ming-iot` push 的普通阻断条件；对应 Validator 可以通过固定 commit/tree/lock 证明所读取的来源。`things/` 的“不修改”约束继续由 workspace `AGENTS.md` 和只读迁移协议共同保证。

## 8. 机器门禁

机器门禁是所有可由确定性程序检查的研发约束集合，不是新服务，也不等同于 TDD Test。

```text
Gate       必须满足的条件
Validator  检查该条件的程序
Test Case  Validator 使用的具体输入和期望
graphctl   编排并汇总 Gate 的工具
```

TDD 产生的单元测试属于机器门禁的一类；Schema、修改范围、编译、契约、基线与 Evidence 新鲜度也可以是机器门禁。

L0 的 Push Gate 至少覆盖：

- Registry 和 DAG 合法；
- 变更范围不越界；
- `thingsbaseline verify` 通过；
- Registry 声明的 Go 测试、构建或既有 L0 门禁通过；
- 本次运行输出结构有效。

required Gate 任一失败或超时，聚合 `gateStatus=FAIL`。上游失败导致当前 Loop 未执行时为 `BLOCKED`。没有当前运行记录时为 `NOT_RUN`。之前的 PASS 与当前 scope、Registry 或 Validator 摘要不匹配时为 `STALE`。

## 9. 状态、Evidence、Review 与批准

每个 Loop 保留三条独立状态轴：

| 状态轴 | 值 | 决定者 |
| --- | --- | --- |
| `gateStatus` | `NOT_RUN/PASS/FAIL/STALE/BLOCKED` | Validator 与 `graphctl` |
| `reviewStatus` | `PENDING/PASS/CHANGES_REQUIRED/STALE` | 独立 Reviewer 的结构化结果 |
| `lifecycle` | `planned/active/closed` | 用户授权后修改 Registry |

总状态按以下优先级推导：

```text
lifecycle=planned                         → PLANNED
任何关键摘要不匹配                        → STALE
Gate FAIL                                 → GATE_FAILED
Gate BLOCKED / 上游 Gate 失败              → BLOCKED_BY_DEPENDENCY
Gate NOT_RUN                              → NOT_RUN
直接依赖尚未处于有效 APPROVED              → WAITING_FOR_DEPENDENCY
Gate PASS + required Review 缺失          → REVIEW_PENDING
Review 要求修改                           → CHANGES_REQUIRED
Gate PASS + Review PASS                   → READY_FOR_APPROVAL
以上均匹配 + lifecycle=closed             → APPROVED
```

除 `planned` 尚未进入执行外，`STALE` 优先于历史 `APPROVED`。即使 Registry 仍写 `closed`，代码、Registry、依赖批准、Gate、Evidence 或 Review 目标发生实质变化后，也不得继续显示 APPROVED。`WAITING_FOR_DEPENDENCY` 不妨碍开发者运行诊断 Gate，但禁止进入 Review 完成和关闭 lifecycle。

### 9.1 运行记录与 canonical Evidence

运行记录位于 `.graph/runtime/`，不提交 Git，只服务于当前排错和页面。

canonical Evidence 位于 `docs/superpowers/evidence/`，提交 Git 并长期积累。G0 保留当前平铺文件结构，不强制迁移既有 L0 Evidence。

结构化 Review 位于 `docs/superpowers/reviews/`，同样提交 Git。完成引用保存在对应 Loop Registry 条目中；runtime、Evidence、Review 和 completion 各自只有一个职责，不能互相充当真源。

新 Evidence 契约至少绑定：

```text
loopId
subjectCommit
scopeDigest
registryDigest
dependencyDigests
validators[] {
  gateId
  validatorId
  validatorVersion
  validatorDigest
}
commands
result
startedAt/duration
artifacts
```

canonical `scopeDigest` 只读取 `subjectCommit` 中被 `evidence.subjectPaths` 实际匹配的 Git 对象。路径按 UTF-8 字节升序排列，摘要输入以字节串 `SCOPE-DIGEST-V1\0` 开头；每个文件依次写入 4 字节大端路径长度、路径字节、4 字节大端 mode 长度、ASCII mode、8 字节大端内容长度和原始 blob 内容，最后取 SHA-256 并编码为 `sha256:<64 位小写十六进制>`。不得使用文件 mtime、绝对路径、用户名或随机运行目录。

`registryDigest` 不是整个 YAML 文件的内容摘要，而是当前 Loop 的“执行契约投影”：包含 API/Schema 版本、Loop ID、`dependsOn`、scope、Gate、Evidence 的 `subjectPaths`、Review 要求以及会改变执行语义的 Registry 全局配置；明确排除标题等展示字段、`lifecycle`、completion、canonical Evidence/Review 引用。该投影完成默认值解析后编码为版本化 canonical JSON，再取 SHA-256；对象键按 UTF-8 字节排序，数组保持契约顺序，不保留 YAML 注释和排版差异。这样补写 Evidence、Review 或 completion、关闭 lifecycle 不会让刚生成的证明自我失效，而执行契约变化一定会失效。

`dependencyDigests` 是按直接依赖 Loop ID 排序的映射，值为依赖 Loop 当前有效的 `approvalDigest`。`approvalDigest` 对依赖 Loop 的 ID、`subjectScopeDigest`、`registryDigest`、Validator 摘要集合、其 `dependencyDigests`、canonical Evidence 内容摘要和 Review 内容摘要组成的 canonical JSON 取 SHA-256。Loop 只有在所有直接依赖都处于有效 `APPROVED`，且本轮 Evidence、Review、completion 绑定的 `dependencyDigests` 一致时才能关闭；任一上游 approvalDigest 变化必须使下游变成 `STALE`。

`validatorDigest` 使用与 scope 相同的文件 framing，对 Gate 的 `validator.sourcePaths` 在 `subjectCommit` 中的实现文件取摘要；Validator ID 和协议版本另行保存。Evidence、Review 或 completion 元数据可以属于 `scope.include`，但不能进入 `evidence.subjectPaths` 或 `validator.sourcePaths`。Registry 入口和包含 lifecycle/completion 的 Loop YAML 也不得进入这两类文件范围，其执行语义由 `registryDigest` 负责证明。

canonical Evidence 只能证明一个干净、已提交且可由 Git 对象重建的 `subjectCommit`。手工 `graphctl ci` 可以把暂存、未暂存和非 ignored 未跟踪变更纳入诊断，但这类结果必须标记 `canonicalEligible=false`，只写 runtime，不能用于关闭 Loop。

`graphctl evidence export` 只接受 `PASS`、`canonicalEligible=true`、目标摘要仍与当前 Registry/依赖一致且 `subjectCommit` 等于当前 `HEAD` 的运行；运行记录中的 required Validator 身份、命令 argv/workdir、命令集合和声明 artifact 集合还必须与当前 Gate 契约完全一致。输出必须是 `docs/superpowers/evidence/` 下尚不存在的新目录，且生成的 `evidence.json`、日志和 artifact 全部被该 Loop 的 include/exclude scope 接受；命令以原子方式写入版本化 `evidence.json`，复制 Validator 声明为 Evidence 的必要 artifact，改写为 bundle 内相对引用并记录每个文件的 SHA-256。它不得覆盖旧 Evidence、修改 Registry、关闭 lifecycle、commit 或 push。既有 L0 平铺 Evidence 通过历史兼容读取，不强制改造成 bundle。

Push Gate 不要求 Completion Evidence 或 Review 已存在，否则普通修复提交将无法 push。它们决定 `READY_FOR_APPROVAL/APPROVED`，不决定普通 Push Gate 是否通过。

### 9.2 Reviewer

新 Loop 的 Reviewer 必须在独立上下文中读取目标 diff、Registry、规格、Validator Evidence 和未关闭 findings，并输出版本化 `review.json`。最小内容包括 Loop ID、目标 `scopeDigest`、`registryDigest`、`dependencyDigests`、所读规格与 Evidence 的内容摘要、Reviewer profile、结论、finding ID/严重度/状态和 Evidence 引用。任一评审输入改变都必须使 Review 变成 `STALE`。

`graphctl` 只检查 Review 格式、目标摘要和 finding 闭环，不能判断架构意见本身是否正确，也不能自行冒充 Reviewer。`REVIEW_PENDING` 与 `CHANGES_REQUIRED` 不阻止普通 push，以允许持续修复。

L0 已有 Markdown/JSON Evidence 记录独立复审和 APPROVED。G0 迁移时允许在 L0 Registry completion 中一次性引用并锁定这些历史文件的内容摘要，不要求改写历史 Evidence；该兼容入口只服务已完成 L0。后续 Loop 必须使用结构化 Review 契约。

### 9.3 用户批准

G0 不实现 `graphctl approve`。只有用户明确授权后，才能把 Registry 中的 lifecycle 改为 `closed` 并提交 completion。Schema 必须要求 `completion.subjectScopeDigest` 匹配 `sha256:` 加 64 位小写十六进制，`completion.registryDigest` 和 `completion.dependencyDigests` 与所引用 Evidence/Review 一致，`completion.evidenceRefs` 是至少一个已存在 canonical Evidence 的规范仓库相对路径。新 Loop 还必须提供一个已存在结构化 Review 的规范仓库相对路径 `completion.reviewRef`。L0 历史兼容入口可以不提供 `reviewRef`，但必须改用独立、仅允许 Loop ID `L0-things-baseline` 的 `legacyReviewRefs`，并固定每个历史文件的 SHA-256；两种 Review 入口互斥，不能借此放宽新 Loop 的结构化 Review 要求。

Registry 修改和 Git 历史共同构成批准记录。AI 不得自行把 `active` 改为 `closed`，也不得自行 commit 或 push。

## 10. 本地 pre-push 门禁

仓库提交 `.githooks/pre-push`，首次由开发者执行：

```bash
git config core.hooksPath .githooks
```

Hook 必须从 `git rev-parse --show-toplevel` 解析仓库根目录，不依赖调用 cwd，并把 Git stdin 中的 ref/SHA 原样解析为结构化参数后调用 `graphctl ci`。Hook 不得自行复制机器门禁逻辑。

以下问题阻止 push：

- Registry/Schema/DAG 非法；
- scope 越界；
- required Validator 失败、超时或无法启动；
- 运行报告或机器页面无法生成有效结构。

以下状态不阻止普通 push：

- `REVIEW_PENDING`；
- `CHANGES_REQUIRED`；
- Completion Evidence 尚未生成；
- `127.0.0.1:18081` Web 服务未运行。

仓库内 `ming-iot/AGENTS.md` 必须规定 AI 不得使用 `git push --no-verify`、删除 Hook、绕过失败或未经用户授权修改 `core.hooksPath`；workspace 根规则继续禁止未经授权 commit/push。本地 Git Hook 技术上仍可被人绕过，因此所有输出必须标识 `Enforcement: LOCAL_GATE`。

## 11. 总控页面与 systemd

`graphctl ci` 和 `graphctl render` 使用同一 renderer，先在 runtime 内生成完整临时输出，校验成功后再以原子 rename 发布。页面只读取：

- Registry 与依赖图；
- 最近一次运行结果；
- Gate、Review、Evidence 和 lifecycle；
- Hook 安装状态；
- 最近失败阶段和耗时。

现有 `iot-visual-companion.service` 改为只读提供 `ming-iot/.graph/runtime/`，固定监听 `127.0.0.1:18081`，保持 systemd enabled。页面可以定时读取 `graph-status.json`，但不提供修改 Registry、运行 Gate、批准 Loop 或写文件的 HTTP API。

Web 服务不可用不影响 `graphctl ci`；renderer 自身无法生成合法机器产物则属于控制平面失败并阻止 push。原 `iot-doc/.superpowers/visual-live/content/rd-graph-loop-control-v2.html` 保留为历史参考，不删除、不继续作为真源。

## 12. 错误处理与安全

- Registry、Schema、依赖和路径错误一律 fail closed；
- Gate 使用 argv 直接启动，不经过 shell；
- 每个 Gate 必须有正数 timeout，超时后终止其进程组并记录为 FAIL；
- stdout/stderr 进入有上限的运行日志，终端只显示摘要和详细报告路径；
- runtime 发布采用同目录临时文件和原子 rename，禁止半页状态；
- JSON 模式 stdout 只包含一个版本化文档，诊断写 stderr；
- `graphctl ci` 不修改 tracked 源码、canonical Evidence、Review 或 Registry；
- `graphctl` 不读取或打印凭据，不把完整环境变量写入报告；
- Validator 环境只允许 Registry 声明的受控覆盖，不允许 Registry 通过变量注入命令；
- 工具缺失、Git base 无法解析或运行环境不完整必须给出明确失败类别，不能降级为 PASS。

## 13. 测试策略

`graphctl` 实现采用 TDD。

单元测试覆盖：

- Registry Schema、版本、重复 ID、缺失依赖和依赖环；
- 拓扑排序、反向依赖与稳定顺序；
- include/exclude 路径匹配和仓库边界；
- Git base/head 与新分支解析；
- `scopeDigest` 的确定性和 framing；
- `scope.include` 与 `evidence.subjectPaths` 分离，证据提交不会自我失效；
- lifecycle/completion 变化不改变 `registryDigest`，执行契约变化必须改变它；
- 直接依赖 `approvalDigest` 变化使下游 Evidence、Review 和 completion 失效；
- Gate timeout、退出码、日志上限与进程清理；
- `gateStatus/reviewStatus/lifecycle` 状态推导；
- Evidence/Review 新鲜度；
- canonical Evidence export 对脏运行、非 PASS、过期摘要和已存在目标的拒绝；
- runtime 原子发布与 JSON/HTML renderer。

集成测试使用临时 Git 仓库和假 Validator，至少证明：

```text
修改 L0 → 选中 L0 及所有下游
修改单个 Loop 清单 → 只选中该 Loop 及下游
修改 controlPlane.globalImpact → 选中 G0 与全部非 planned Loop
越界修改 → Push Gate 失败
Validator 失败 → gateStatus=FAIL
上游失败 → 下游 BLOCKED
旧 Evidence → STALE
Review 缺失 → REVIEW_PENDING，但普通 push 可通过
非法 Registry → fail closed
同一确定性输入 → 相同 Graph 与页面结构
```

最终端到端验证包括：

- `GOWORK=off go test ./...` 在 `backend/tools` 通过；
- L0 Registry 能编排现有 `thingsbaseline`；
- 人工制造的越界和失败 Gate 能被 Hook 阻止；
- `.graph/runtime` 可由零状态重新生成；
- systemd 服务重启后 `127.0.0.1:18081` 可访问；
- `things/` 与当前用户的 `backend/go.work.sum` 修改不被触碰。

## 14. 迁移顺序

1. 在独立 `ming-iot` worktree 中实现 Registry Schema、G0 控制平面 Loop、L0 Loop 和 `graphctl` 核心测试；
2. 接入 `thingsbaseline`，将其日常 Evidence 输出定向到本次 runtime run 目录；
3. 运行 `validate/context/scope-check/status/ci/evidence export/render`，确认 L0 状态、证据导出和历史批准兼容；
4. 提交 Hook 文件并人工安装 `core.hooksPath`；
5. 在 G0 变更集成到 canonical `ming-iot` checkout 后生成首个完整 `.graph/runtime`；
6. 再修改现有 systemd 服务读取 canonical checkout 的 runtime 目录并验证端口，禁止指向临时 worktree；
7. 保留旧手写页面，仅停止把它作为当前状态入口；
8. 完成全部测试、Review 和用户批准后，才把 G0 lifecycle 关闭。

实施必须保留当前主工作树中用户已有的 `backend/go.work.sum` 修改，不得 reset、checkout 或混入 G0。任何 commit 和 push 均继续遵守 workspace `AGENTS.md`，必须由用户明确授权。

## 15. 验收标准

G0 只有同时满足以下条件才算完成：

1. G0 控制平面自身与 L0 都通过 Registry 进入状态计算和页面，G0 的特殊全局影响关系可见且不冒充领域依赖；
2. 手写 HTML 不再承载当前真源；
3. `graphctl ci` 默认检查受影响 Loop 与全部下游，`--all` 可全量执行；
4. scope 越界、required Gate 失败和 Registry 非法均能阻止 push；
5. Review Pending 不阻止普通 push，也不会被显示为 APPROVED；
6. canonical Evidence、Review 与 completion 的新鲜度可以机器判定；
7. 页面明确显示 `LOCAL_GATE` 和 Hook 安装状态；
8. WSL/systemd 重启后服务仍固定监听 `127.0.0.1:18081`；
9. `graphctl` 自身单元、集成和端到端测试全部通过；
10. `things/`、用户已有 `backend/go.work.sum` 和其他无关文件没有被修改。

## 16. 独立 Review 加固决策（2026-08-18）

首次独立 Review 证明现有实现虽然能从真实运行导出有效 Evidence，但对“已经存在的 Evidence、completion 和本地 runtime 是否可信”的重验仍不完整。G0 采用 fail closed 方案：Git 提交只赋予文件不可变身份，不自动赋予其语义可信性；`.graph/runtime/**` 始终是可丢弃、可被篡改的本地输入。控制面必须从 Registry、Git 对象和文件内容重新建立证明，不能相信文档自己声明的 `PASS` 或 digest。

本节是第 9、12、13 节的约束补充，不扩大 G0 的产品能力，也不引入远程身份、签名服务或自动批准。

### 16.1 统一的 Gate 证明校验

Evidence export、committed canonical Evidence discovery 和 runtime status 不得各自维护一套强弱不同的 Gate 校验。实现必须抽出一个内部共享的“当前 Registry Gate 契约”，至少包含：

- required Gate 的精确集合；
- 每个 Gate 的 `id/kind/argv/workdir`；
- Validator 的 `id/version/sourcePaths/artifacts` 与在指定 Git commit 上重算的 `validatorDigest`；
- 命令成功谓词 `result=PASS && exitCode=0 && failureKind=""`；
- 日志和声明 artifact 的精确集合、规范相对路径、大小与 SHA-256 约束。

canonical Evidence discovery 必须以 Evidence 的 `subjectCommit` 为输入重新验证：

1. commit 是仓库内真实可读的 commit object；
2. 使用当前 Loop 的 `evidence.subjectPaths` 在该 commit 上重新计算 `scopeDigest`；
3. 使用该 commit 上的 control-plane/Validator 输入重新计算 `registryDigest` 和全部 required `validatorDigest`；
4. `validators`、`commands` 和 `artifacts` 与 required Gate 契约一一对应，不允许缺失、重复或额外记录；
5. 顶层 `result=PASS` 必须由全部 required command 的 clean PASS 推导出来，不能只相信顶层字段；
6. bundle 中每个日志和 artifact 必须是 committed regular blob，并与记录的大小、完整流/保留流摘要一致。

任何一步失败，该 bundle 都不是 current canonical Evidence，不能进入 Review、completion 或 approval 计算。导出器继续做导出前后双重 HEAD/dirty/digest 校验，并调用同一个共享契约校验器，避免“导出时严格、读取时宽松”。

### 16.2 摘要使用真正的 canonical JSON

`registryDigest` 与 `approvalDigest` 的投影继续保持现有字段语义，但编码必须脱离 Go struct 声明顺序。内部 canonical JSON 编码器遵守以下固定规则：

- object key 按 UTF-8 原始字节升序排列，并递归应用到所有嵌套 object；
- array 保持契约定义的原始顺序；需要集合语义的输入必须在构造投影时先显式排序；
- 只接受投影实际使用的 `null/bool/string/signed integer/array/object`，拒绝浮点数、非有限数和无法规范表示的值；
- string 使用唯一、测试锁定的 JSON escaping，输出无空白、无尾随换行；
- 相同逻辑值无论来自 struct、map 或另一语言实现，都必须得到逐字节相同的编码。

实现必须提供包含乱序嵌套对象、UTF-8 key、数组和整数的 golden byte vector，并分别锁定 `registryDigest` 与 `approvalDigest` 的最终 SHA-256。现有非 canonical 实现产生的摘要不提供兼容旁路；旧 Evidence 保留为历史记录，但在新实现下自然判定为 STALE，随后按正常顺序重新生成。

### 16.3 completion 必须绑定真实 Evidence subject

`DeriveStatus` 保持纯状态机，不直接访问 Git；App 在调用它之前构造 completion proof。关闭 Loop 要显示 `APPROVED`，除原有引用和 digest 条件外还必须同时满足：

- `completion.subjectCommit` 是仓库内真实 commit；
- 在该 commit 上按当前 `evidence.subjectPaths` 重算的 scope，等于 `completion.subjectScopeDigest`；
- selected Review 引用的每个 canonical Evidence 都通过 16.1 的重验；
- completion 的 `evidenceRefs` 与 selected Review 的 Evidence 集合完全相等；
- 这些 Evidence 的 `subjectCommit` 全部等于 `completion.subjectCommit`，禁止用另一个 commit 的等价 digest 冒充批准对象；
- completion 的 Review ref、Registry/dependency digest 与当前目标完全一致。

App 把结构化的 completion proof 或明确的失败原因交给状态机；证明缺失、commit 不存在、scope 无法重建或 subject 不一致时必须为 `STALE`，且 dashboard 显示具体原因。下游 Evidence export 与普通 `status/render` 必须复用同一 completion proof，不再存在只有下游导出才执行的更强检查。

### 16.4 runtime 是不可信的诊断输入

runtime 不能提供 approval，只能提供最近一次 Gate 诊断；即便如此也必须防止伪造 PASS 和资源消耗。所有 runtime 读取统一经过安全 loader：

- run 目录名必须匹配版本化 run-ID 语法；目录、`run.json` 及中间路径不得经过 symlink；
- 使用 no-follow 打开后核对文件身份，限制单个 report 最大 16 MiB，严格 JSON 解码并拒绝未知字段和尾随文档；
- aggregate report 的 run ID、时间、result、Loop ID 唯一性和拓扑结果必须自洽；
- 每个 Loop record 必须绑定当前 scope/Registry/dependency/Validator identity，并满足 required Gate 的精确 command/artifact 集合及结果一致性；
- `PASS` 只能由完整 required Gate clean PASS 推导；空 commands、重复 Gate、额外 Gate、错误 argv/workdir 或不一致 aggregate result 一律拒绝；
- 最新 report 非法时 `status/render` fail closed，不能静默跳过后展示更旧 PASS，也不能发布一代声称成功的新页面。

这只能防止损坏、部分写入和通过普通文件篡改伪造状态；拥有同一 OS 用户权限且能同时伪造完整 runtime、日志与进程环境的攻击者不在 G0 的密码学威胁模型内。未来若需要抵御该攻击面，必须另行设计签名或远程可信执行，不在本次补救中顺手扩张。

### 16.5 TDD、证据失效与复审顺序

四项修复按独立 Review finding 顺序执行 RED-GREEN-REFACTOR，每项先增加最小失败用例：

1. committed Evidence 缺 command、command 非 clean PASS、digest 与 `subjectCommit` 不一致时必须被拒绝；
2. 不同 object 构造顺序得到同一 canonical bytes/digest，并匹配 golden vector；
3. 不存在的 completion commit、commit/scope 不匹配和 Evidence subject 不一致不得得到 `APPROVED`；
4. runtime symlink、超限文件、非法 run ID、空/重复/错误 command 和 aggregate 不一致不得产生 Gate PASS 或新页面。

每个 finding 的定向测试通过后再运行 `go test ./cmd/graphctl ./internal/graphctl/...`、race、vet、build、Registry/schema/hook 检查和完整 G0+L0 CI。因为 `backend/tools` 与 digest 算法都会变化，当前 L0/G0 Evidence 和首次 Review 只保留审计价值，不可继续用于批准。恢复顺序固定为：提交修复实现 → 干净全量 CI → 导出并提交 L0 Evidence → 再次干净全量 CI → 导出并提交 G0 Evidence → 将四个 G0 open finding 和首次 L0 PASS 结论交给新的两个独立 Reviewer → 只有双 Review PASS 后才请求 lifecycle closure。

`backend/go.work.sum` 若仍有本地自动改写，必须在 Evidence 阶段如实导致 L0 非 canonical-eligible；不得通过 ignore-index、assume-unchanged 或隐藏 dirty state 绕过。

## 17. 后续演进

G0 稳定后，每个新 Loop 只需增加版本化 Loop 文件、领域 Validator、Evidence/Review 契约和依赖关系。未来无论接入 GitHub Actions、ECS 自托管 Runner 或其他远程 CI，都只能调用同一个 `graphctl ci`，不得在远端复制另一套门禁语义。

并行 Gate、自动 Reviewer、批准命令、worktree 编排和跨仓远程控制面必须分别通过后续设计 Loop 引入，不能在 G0 中顺手扩张。
