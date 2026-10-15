---
document_kind: spec
status: approved
delivery_issue: ming-iot#IKD4OR
code_repository: git:gitee.com/ming-industry/ming-iot
related_documents:
  - CONTEXT.md
  - adr/0001-gitee-is-shared-delivery-truth.md
  - adr/0002-private-rd-docs-and-contract-promotion.md
---

# Ming IoT Gitee 交付流程与统一 Verification Profile 设计

## 1. 文档状态

- 状态：已审批
- 日期：2026-09-03
- 适用仓库：`ming-iot`、`ming-iot-rd-docs`
- Delivery Issue：[ming-iot#IKD4OR](https://gitee.com/ming-industry/ming-iot/issues/IKD4OR)

本设计统一以下三件事：

1. 以 Gitee Issue/PR 承载团队共享的交付状态；
2. 以 `ming-iot` 根 `Makefile` 承载供应商无关的 Verification Profile；
3. 以一个 Gitee PR 模板显式连接 Delivery Issue、私有研发文档、验证结果、Review 和代码 Head。

本设计不修改或扩展 LoopX 通用能力，不引入新的 Harness、任务平台或第三方项目管理组件。

## 2. 背景与问题

当前研发事实分散在四个表面：

- `ming-iot` 保存产品代码、部署配置和项目专用工具；
- `ming-iot-rd-docs` 保存 Proposal、规格、计划、Review、Evidence 和历史归档；
- Gitee 已提供代码托管和流水线，但当前流水线直接拼接 Go 构建、测试与 vet 命令；
- LoopX 保存本地 Goal、Todo、Agent、User Gate 和执行 Evidence。

这种分层本身合理，但缺少稳定连接：

1. Proposal、RFC、Gitee Issue、Plan Task 和 Agent Todo 的职责容易重叠；
2. 新电脑无法从一条共享 Delivery Issue 找到精确的 RFC、规格和计划；
3. 本地开发、Gitee CI 和 Reviewer 可能执行不同命令；
4. `backend/tools` 已包含 Kafka、Things 启动和代码生成契约，但何时调用仍主要依赖具体实施计划；
5. 重型 Things 启动基线不适合塞入 2 CPU、4 GiB 的每次 Gitee PR 构建；
6. PR 缺少统一的 Issue、验证、Review、风险与回滚回执。

## 3. 设计依据

本设计吸收 LoopX 官方仓库维护实践，但只采用项目侧做法：

- 用持久状态替代聊天记忆；
- 将重复判断沉淀为契约、Skill 或 CLI；
- Issue 进入后分类，再形成聚焦、可审阅的 PR；
- Review 与批准绑定精确 Head；
- 发布、合并、权限和生产变更保留真实的人类决策边界；
- Smoke 只保留长期稳定行为，不为一次性过程无限增生。

参考资料：

- [LoopX：GitHub 仓库维护与运营自动化最佳实践（固定提交）](https://github.com/huangruiteng/loopx/blob/f8cefd2d452c2587ea558496ff961a6b8adcfc03/docs/community/github-maintenance-ops-best-practices.zh-CN.md)
- [Gitee：Issue 和 PullRequest 模板](https://help.gitee.com/base/%E5%BC%80%E5%8F%91%E5%8D%8F%E4%BD%9C/Issue%E5%92%8CPullRequest%E6%A8%A1%E6%9D%BF)
- [Gitee：PullRequest 关联 Issue](https://help.gitee.com/base/pullrequest/issue)

## 4. 目标与非目标

### 4.1 目标

1. 一个可独立验收的 Delivery Loop 对应一个 Gitee Delivery Issue 和一个主要代码 PR。
2. 重大架构工作有 RFC；普通功能和缺陷不被迫写 RFC。
3. 新需求的执行者能从 Delivery Issue 找到完整 Delivery Packet。
4. 根 `Makefile` 成为本地、CI、Reviewer 共用的验证入口。
5. Verification 按成本和风险分层，轻量检查自动运行，重型检查显式选择。
6. `backend/tools` 的项目契约通过根入口稳定暴露，而不是依赖聊天记忆。
7. Review、Evidence 与用户授权保持相互独立，并绑定精确候选 Head。

### 4.2 非目标

1. 不把 `ming-iot-rd-docs` 整体迁入 `ming-iot`。
2. 不把 Gitee 改造成完整项目管理平台，也不在本阶段引入 Trellis 等第三方组件。
3. 不把 `.loopx/`、`.codex/` 或原始私有 Evidence 提交到 Git。
4. 不修改 LoopX 源码、状态模型、命令或 Skill。
5. 不让 Gitee 云端 PR 流水线启动完整 Things 基础设施。
6. 不在本次改造中修复既有业务测试失败；失败必须如实暴露并另建 Delivery Issue。

## 5. 权威边界

四个组成共同构成核心开发人员的完整开发控制面，但权威级别不同：

| 表面 | 权威职责 | 丢失后的影响 |
| --- | --- | --- |
| `ming-iot` | 产品代码、测试、稳定项目契约、构建与验证入口 | 无法开发或交付产品 |
| `ming-iot-rd-docs` | RFC、规格、计划、完整 Review、Evidence 摘要和历史依据 | 无法可靠实施新的 Delivery Issue |
| Gitee | Issue、PR、共享提交、CI、负责人和交付状态 | 可短时离线开发，但不能团队同步和完成交付 |
| LoopX | 本地 Goal、Todo、Agent 编排、User Gate 和执行投影 | 可退化为手工开发，并可从前三者重建本地状态 |

LoopX 不是团队唯一事实源。任何必须跨成员、跨电脑恢复的信息都必须进入 Gitee 或两个 Git 仓库；LoopX 只保存可重建的本地长程执行状态。

## 6. 工作项与文档分类

### 6.1 Delivery Issue

所有准备执行的功能、缺陷、迁移和工程改造都必须有 `ming-iot` Gitee Issue。一个 Issue 对应一个可独立验收的交付结果和一个主要代码 PR。Plan Task 只用于展开实现步骤，不逐项创建 Issue。

统一 Delivery ID 使用：

```text
ming-iot#<Gitee Issue ident>
```

PR 使用 Gitee 支持的 `#<Issue ident>` 语法建立平台关联；私有研发文档同时保存完整 Delivery ID，避免跨仓库时失去项目语义。

### 6.2 RFC

根目录新增 `rfcs/`。只有满足以下条件的工作才写 RFC：

- 会改变长期架构、数据模型、兼容边界或团队工作方式；
- 决策难以逆转；
- 至少存在两个具有真实取舍的方案。

RFC 的状态为 `draft`、`accepted`、`rejected` 或 `superseded`。RFC 被接受只表示架构方向成立，不表示实现完成。架构型 Delivery Issue 必须先合并 RFC，再进入规格和计划。

### 6.3 Proposal

`proposals/` 暂时保留，只用于跨 Issue 的候选比较、投入取舍和交付排序，不再承载完整实现规格，也不授权编码。普通需求直接进入 Issue，架构决策进入 RFC。

在本流程完成 5 个真实 Delivery Loop 或运行 90 天后，以先到者为准复查：

- 若 Proposal 持续保存了 Gitee Issue/Milestone 无法清晰表达的跨交付取舍，则继续保留；
- 若 Proposal 只是重复 Issue 内容，则停止新增，将仍有价值的历史材料归档，并由 Gitee Issue/Milestone 承担入口。

既有 Proposal 不批量改写或迁移，只有在被重新激活时才按新分类处理。

### 6.4 Spec、Plan、Review 与 Evidence

- Spec 回答“要做成什么”，定义行为、边界、兼容性和验收标准；
- Plan 回答“怎样完成”，包含测试先行顺序、文件范围、Task 和验证命令；
- Review 保存独立 Reviewer 对精确代码 Head 的完整结论；
- Evidence 摘要保存稳定结果和原始证据指针，原始日志留在 Gitee CI Artifact 或 LoopX 私有 Evidence。

这些材料继续留在 `ming-iot-rd-docs`，不整体迁入产品仓。

## 7. Delivery Packet 与统一元数据

一个 Delivery Packet 由以下材料组成：

```text
Delivery Issue
├── RFC（仅架构型工作）
├── Spec
├── Plan
├── Review
└── Evidence 摘要
```

新建 RFC、Spec、Plan、Review 和 Evidence 摘要使用统一 YAML Front Matter：

```yaml
---
document_kind: spec
status: draft
delivery_issue: "ming-iot#<Gitee Issue ident>"
code_repository: git:gitee.com/ming-industry/ming-iot
related_documents:
  - rfcs/example.md
---
```

规则如下：

1. `document_kind` 必须是 `rfc`、`spec`、`plan`、`review` 或 `evidence`。
2. `status` 使用该文档类型允许的有限状态，不允许自由文本近义词。
3. `delivery_issue` 必须使用稳定 Delivery ID；Proposal 在候选阶段不受此字段约束。
4. `code_repository` 使用稳定 Git identity，不保存本机路径。
5. `related_documents` 只保存 `ming-iot-rd-docs` 仓库相对路径。
6. Review 和 Evidence 额外要求完整 `exact_head`；Spec 和 Plan 不在正文中保存自引用 Git 哈希。
7. 精确文档版本由 Git commit 或 Gitee PR 链接确定，不在文件内维护会自我失效的正文哈希。
8. 新文档立即执行该规则；既有材料仅在重新激活或实质修改时迁移，不做全仓机械改写。

`ming-iot-rd-docs` 的策略测试负责验证元数据结构、文档存在性和相互引用；不扩展 LoopX 来解析这些字段。

## 8. 文档审批与编码入口

### 8.1 架构型工作

```text
Gitee Issue
  → RFC PR 审阅并合并
  → Spec + Plan 文档 PR 审阅并合并
  → 创建或领取 LoopX Agent Todo
  → 开始代码实现
```

### 8.2 普通功能或缺陷

```text
Gitee Issue
  → Spec + Plan 文档 PR 审阅并合并
  → 创建或领取 LoopX Agent Todo
  → 开始代码实现
```

Spec 与 Plan 是两个独立文件，但可以放在同一个“可实施”文档 PR 中，减少人为等待。未经批准的 Draft 不能作为编码依据。紧急修复可使用精简 Spec/Plan，但不能跳过 Issue、验收标准和验证说明。

## 9. 稳定项目契约的晋升

RFC、Spec 和 Plan 不会被稳定项目契约取代。实现新需求时，Agent 必须同时读取：

1. `ming-iot` 中适用于当前代码的稳定项目契约；
2. `ming-iot-rd-docs` 中本次 Delivery Packet。

一次交付产生的规则按以下方式处理：

| 规则性质 | 权威位置 |
| --- | --- |
| 为什么选择某个架构 | RFC 或 ADR |
| 本次交付必须实现的行为 | Spec |
| 本次交付步骤 | Plan |
| 后续所有开发者都必须遵守且可机器检查 | 产品测试、Contract、Makefile 或工具 |
| 后续所有开发者都必须遵守但不能机器检查 | `ming-iot/AGENTS.md` 或稳定开发文档 |

晋升必须在对应代码 PR 中完成或明确声明“不产生新的稳定项目契约”。禁止复制整份研发文档到产品仓形成双重权威。

## 10. 根 Makefile 设计

`ming-iot/Makefile` 是验证入口，不重新实现底层工具。它只组合现有 Go、子目录 Makefile、Docker Compose 和 `backend/tools` 命令。

### 10.1 公共入口

```text
make help
make build
make test
make vet
make contract-static
make verify-fast VERIFY_SCOPE=<scope>
make verify-pr
make verify-integration VERIFY_SUITE=<suite>
make verify-release MING_HEAVY_VERIFY=1
```

公共规则：

1. 所有目标从仓库根执行，路径与当前 shell 目录无关。
2. 失败返回非零状态，不把 `skip`、依赖不可达或缺少参数伪装成 PASS。
3. 默认复用宿主 `GOCACHE` 和 `GOMODCACHE`；缓存只影响速度，不影响测试是否实际执行。
4. 根 Makefile 尊重调用方设置的 `GOPROXY`；流水线可用 `MING_GOPROXY` 生成运行时 `GOPROXY`，项目工具不得重新硬编码代理。
5. 验证目标不得写入受 Git 跟踪的业务文件；代码生成使用独立 `generate-*` 目标并由 diff 审阅。
6. 每个目标在开始时打印目标名、scope/suite、Go 版本和 Git Head；不得输出凭据。
7. 根入口不依赖 LoopX，普通开发者和 Gitee CI 均可直接运行。

### 10.2 底层职责

- `backend/saas/Makefile` 和 `backend/things/Makefile` 继续负责各自二进制构建；
- `deploy/Makefile` 继续负责隔离基础设施和 Things/Kafka 生命周期；
- `backend/tools` 继续负责项目专用代码生成、静态契约、集成探针和 Evidence 生成；
- 根 Makefile 只提供稳定名称和组合顺序，不复制这些实现。

## 11. 四档 Verification Profile

### 11.1 `verify-fast`

用途：开发内循环和提交前快速反馈，不作为单独的 PR 合并证明。

- 必须显式提供一个 `VERIFY_SCOPE`：`share`、`saas`、`things` 或 `tools`；
- 对该 scope 运行 Go test、go vet 和 `git diff --check`；
- 不构建全部服务，不启动 Docker，不访问外部服务；
- 多个 scope 通过多次显式调用完成，Evidence 记录每条命令。

目标：缓存命中后的常见 scope 在约 3 分钟内完成；若连续超过 5 分钟，应拆分测试或调查性能，而不是静默绕过。

### 11.2 `verify-pr`

用途：所有代码 PR 的统一自动门禁。

它固定执行：

1. `git diff --check`；
2. `share`、`saas`、`things`、`tools` 四个 Go module 的单元测试；
3. 四个 module 的 go vet；
4. SaaS 和 Things 全部服务二进制构建；
5. `contract-static` 中所有无需外部服务、不会修改业务树的项目契约；
6. 私有路径、凭据字面量、生成二进制和运行日志边界扫描。

`contract-static` 初始纳入：

- Kafka Topic Catalog、配置和依赖版本静态校验；
- Things 启动合同、Compose 展开和危险操作扫描，但不构建镜像、不启动服务；
- API/RPC 生成清单、锁文件和已提交生成结果的一致性检查；
- `backend/tools` 自身单元测试。

一次性 L0 导入验证、Kafka produce/consume、数据库集成测试和 Things 完整启动不属于 `verify-pr`。

### 11.3 `verify-integration`

用途：对受影响的外部依赖边界执行真实集成验证。

- 必须显式提供 `VERIFY_SUITE`；缺少或未知 suite 时失败；
- 初始 suite 为 `kafka` 和 `things-startup`；
- 每个 suite 使用隔离命名、独立数据库/Topic 和有界超时；
- 成功或失败后都执行安全清理，但不执行数据库 reset、共享卷删除或生产变更；
- 依赖不可用记为 FAIL/NOT_RUN，不得降级成 PASS；
- Spec/Plan 根据变更面声明需要哪些 suite，PR 模板逐项回执。

后续新增 suite 必须同时增加根 Make 入口、帮助文本、稳定测试和变更面说明，不引入第二份 Profile 注册表。

### 11.4 `verify-release`

用途：发布候选的完整验证，不等同于发布或部署授权。

- 必须显式设置 `MING_HEAVY_VERIFY=1`，防止误启动重型环境；
- 先运行 `verify-pr`，再运行全部当前 release-required integration suite；
- 使用具备 Docker 和足够 CPU、内存、磁盘的专用开发机或自托管 Runner；
- 输出精确 Head、镜像/二进制身份、suite 结论和 Evidence 指针；
- 任何 suite 未运行均不能形成 release PASS。

`verify-release` 不连接生产环境，不执行发布、数据迁移或凭据变更；这些操作仍需独立 User Gate。

## 12. 变更面与最低验证要求

| 变更面 | 最低要求 |
| --- | --- |
| 单一 Go module 内部实现 | 对应 `verify-fast` + `verify-pr` |
| `backend/tools`、代码生成清单或 lock | `VERIFY_SCOPE=tools` + `verify-pr`，必要时生成结果 parity |
| Kafka catalog、producer/consumer 或部署配置 | `verify-pr` + `VERIFY_SUITE=kafka` |
| Things 启动合同、服务配置、依赖或部署 | `verify-pr` + `VERIFY_SUITE=things-startup` |
| 数据库事务、迁移或兼容边界 | `verify-pr` + Spec 指定的数据库集成 suite；suite 未注册前不得声称完整集成通过 |
| 发布候选 | `verify-release` |
| 纯研发文档仓改动 | `ming-iot-rd-docs` 自身 pytest、Authority Catalog 和 `git diff --check` |

不允许开发者为了让 PR 变绿而自行降低最低档位。确需例外时，PR 必须说明未运行项、原因、风险承担者和补验证方式，并由 Reviewer 明确接受。

## 13. Gitee CI

Gitee 云端流水线只承担可在临时、低资源环境稳定运行的门禁：

1. PR 或候选分支更新时运行 `make verify-pr`；
2. 流水线设置 `GOPROXY="${MING_GOPROXY:-https://goproxy.cn,direct}"`，但 Makefile 不覆盖显式环境；
3. 缓存 `/go/pkg/mod` 和 Go build cache；缓存未命中时允许重新下载，不能跳过验证；
4. 普通 PR 验证不上传服务二进制 Artifact；需要制品时使用独立候选/发布阶段；
5. Integration 和 Release 由 Gitee 手工任务、专用 Runner 或开发机运行，结果回填 PR；
6. Gitee 配置只调用根 Make 目标，不再复制具体 Go package 列表。

若 `verify-pr` 在缓存命中后持续超过 15 分钟，必须以独立 Delivery Issue 调查；不能通过删除关键测试来掩盖耗时。

## 14. Gitee PR 模板

产品仓新增一个中文模板：

```text
.gitee/PULL_REQUEST_TEMPLATE.zh-CN.md
```

模板必须包含：

1. Delivery Issue：使用 Gitee `#<ident>` 关联语法；
2. 变更目的与边界；
3. RFC、Spec、Plan 的私有仓库相对路径和已合并文档 revision；
4. 稳定项目契约晋升：列出目标文件，或声明本次无新增；
5. Verification Profile：命令、结果、未运行项和原因；
6. 精确候选 Head；
7. 独立 Review：verdict、exact head、review ref、blocker/warning 数；
8. 风险、兼容性和回滚方法；
9. 明确声明未包含生产发布、凭据或权限操作。

模板不是证明本身。填写 `PASS` 不能替代 CI、Review 文档或 Evidence；候选 Head 改变后，旧 Review 失效，必须重新验证受影响部分并更新回执。

## 15. Review、Evidence 与双仓 PR 顺序

1. 实现 Agent 在 `ming-iot` 独立 worktree 完成代码、测试和候选提交。
2. 验证通过后推送候选分支并创建 Draft PR，使候选 Head 和 CI 状态成为团队可读事实。
3. 独立 Reviewer 只读 Draft PR 的精确代码候选，不修改实现；完整 Review 写入 `ming-iot-rd-docs/reviews/`，并绑定 `exact_head`。
4. 原始机器 Evidence 留在 Gitee CI Artifact 或 LoopX 私有 Evidence；`ming-iot-rd-docs/evidence/` 只保存脱敏摘要和稳定指针。
5. Review 有 blocker 时回到同一个代码 PR 修复，再对新 Head 复审。
6. 最终 Review/Evidence 文档先通过 `ming-iot-rd-docs` PR 进入其 `master`，代码 PR 再引用该稳定 revision 并退出 Draft。
7. `ming-iot` 代码 PR 通过 Gitee CI 与人工审查后，由有权限的人在 Gitee 合并；人的 Gitee 合并动作就是合并授权。
8. 若要求 Agent 合并 PR、执行发布、修改生产、处理凭据或变更权限，必须建立绑定精确目标的独立 User Gate。

## 16. LoopX 的使用边界

本设计只使用 LoopX 已有能力：

- Agent Todo 引用 Delivery ID 和已合并 Delivery Packet；
- 一个 Agent Todo 可以覆盖 Plan 的多个 Task，不逐 Task 建 Todo 或 User Gate；
- LoopX 保存执行投影、Agent 分工、薄 Evidence 指针和必要 User Gate；
- Gitee 与 Git 仓库保存可跨电脑恢复的事实；
- 本地 LoopX 状态丢失后，根据 Delivery Issue、文档 revision 和代码 PR 重建。

根 Makefile、Gitee CI、PR 模板和文档元数据都不能依赖修改 LoopX 才能工作。

## 17. 失败处理

- **验证失败：** 保留真实失败，不自动降级 Profile；修复后重新运行。
- **环境不足：** 标记 NOT_RUN，转到具备资源的开发机或 Runner，不伪造 PASS。
- **CI 与本地不一致：** 先比较 Head、Go 版本、环境变量和 Make 目标；以同一目标复现。
- **Review 过期：** 候选 Head 变化后，旧 Review 只能作为历史，不能继续授权合并。
- **文档链接失效：** 阻止进入编码或合并，先修复 Delivery Packet 元数据和 Git revision。
- **LoopX 状态丢失：** 从 Gitee Issue/PR 和两个仓库重建 Todo，不改写共享 Git 历史。
- **重型环境残留：** 运行目标的安全 cleanup；删除共享数据库、卷或缓存仍需明确授权。

## 18. 实施切片

规格获批后，实施计划按以下可回滚切片展开：

1. 创建并关联本流程的 Gitee Delivery Issue。
2. 修复 `ming-iot-rd-docs` 两项过期仓库策略测试，并增加 `rfcs/`、元数据和目录策略测试。
3. 在 `ming-iot` 增加根 Makefile及其帮助/路由测试，不改变现有底层命令语义。
4. 接入 `backend/tools` 的静态契约聚合入口。
5. 增加 Gitee 中文 PR 模板。
6. 将现有 Gitee Go 流水线改为调用 `make verify-pr`，保留缓存并移除重复命令。
7. 在可用环境分别验证 fast、PR、Kafka integration 和 Things startup integration。
8. 产出独立 Review、Evidence 摘要和双仓 PR；不在本实施中自动合并或发布。

实施过程中 Task 可以形成 checkpoint commit，但不逐 Task 建立 Agent Todo 或 User Gate。整个实施 Todo 完成后再进行一次汇总验收。

## 19. 验收标准

1. 新 Delivery Issue、RFC、Spec、Plan、Review 和 Evidence 的职责无重叠且可追溯。
2. `ming-iot-rd-docs` 新文档元数据可以被本仓策略测试校验。
3. `ming-iot` 根 Makefile 提供四档 Verification，`make help` 清楚说明输入与成本。
4. `verify-pr` 无外部服务依赖，并由 Gitee CI 和本地使用同一命令。
5. `verify-integration` 缺少 suite 或依赖时不会假 PASS。
6. `verify-release` 有显式重型运行确认，且不执行生产发布。
7. Gitee PR 模板能关联 Issue、Delivery Packet、精确 Head、Verification 和独立 Review。
8. `backend/tools` 的静态契约、Kafka 集成和 Things 启动能力分别落入正确档位。
9. Gitee 2 CPU、4 GiB PR 环境不启动完整 Things/Kafka 依赖栈。
10. 没有修改 LoopX 通用能力，也没有把 `.loopx/`、`.codex/`、原始日志或凭据加入 Git。
11. 两个仓库的策略测试、相关 Go 测试、`git diff --check` 和边界扫描通过。
12. Proposal 生命周期复查条件被记录，届时可以基于真实 Delivery Loop 决定保留或退役。
