# ming-iot Authority 登记防错设计

## 1. 背景

`ming-iot/.loopx/registry.json` 已登记多项 Authority Source，但登记时没有先建立可读取的项目文档目录。结果是 `topic_authority` 可以回答“应该查哪项来源”，`source_ref_sha256` 可以识别来源契约，却无法帮助新 Chat 定位正文，形成了裸哈希。

LoopX 官方 `loopx-doc-registry` 工作流已经规定固定顺序：先更新项目自己的权威表面，再登记脱敏哈希契约，最后刷新状态。本设计不扩展 LoopX，而是在 `ming-iot` 项目侧补齐规则、目录和后置校验，使不同 Coding Agent 都能遵守同一个顺序。

## 2. 目标与非目标

### 2.1 目标

- 新 Chat 不依赖旧对话即可从 `ming-iot` Goal 找到文档目录，再找到对应正文；
- 任何新增或更新的 Authority Source 都必须先有可读取的项目文档映射；
- 裸哈希在 Authority 变更后的聚焦校验中失败；
- Codex、Claude Code、Kimi Code 等 Agent 都能从仓库级规则看到相同流程；
- 只使用 LoopX 现有 `register-authority-source`、`import-doc-registry-authority`、`refresh-state` 和状态回读能力。

### 2.2 非目标

- 不修改或 Fork LoopX；
- 不引入 `authority_source_registration_v1`、Locator Store 或新的 Resolve API；
- 不把绝对路径、私有 URL、凭据或文档正文写入公开的 `ming-iot` 仓库；
- 不把校验加入每次全量构建或慢速 pre-push；
- 不替代文档内容评审、读取授权和冲突裁决。

## 3. 方案比较

### 3.1 方案 A：修改 LoopX CLI 强制顺序

优点是所有项目都能得到硬门禁；缺点是扩大 LoopX 通用能力、偏离官方演进，并且 CLI 无法可靠判断每个项目如何存放正文。本方案拒绝采用。

### 3.2 方案 B：只依赖 Skill 和 `AGENTS.md`

优点是简单；缺点是仍属于行为约束，Agent 可以误调用 `register-authority-source`，机器也不会发现裸哈希。本方案只作为第一层防护，不能单独采用。

### 3.3 方案 C：仓库规则、官方目录和聚焦校验三层防护

这是推荐方案：Skill 定义标准流程，`AGENTS.md` 让不同 Agent 都能看到流程，`DOC_REGISTRY.yaml` 提供稳定定位，项目侧聚焦校验验证 Authority 与正文映射。它不改变 LoopX，同时能把本次失败转化为可重复检查。

## 4. 权威表面与数据模型

### 4.1 文档目录

新增本地权威目录：

```text
ming-iot-rd-docs/meta/DOC_REGISTRY.yaml
```

目录使用 LoopX 官方可读取的简单 YAML 结构：

```yaml
version: 1
updated_at: 2026-08-26
purpose: ming-iot 项目长期研发资料入口
status_definitions:
  current: 当前实施与决策依据
  historical: 仅用于理解历史约束
default_entry_docs:
  - README.md
topic_authority:
  development_control_plane: specs/2026-08-24-ming-iot-direct-loopx-harness-retirement-design.md
```

所有路径都以 `ming-iot-rd-docs` 仓库根目录为基准，必须是仓库内相对路径，不得使用绝对路径或 `..`。

首轮目录覆盖 `ming-iot` 当前七个业务/决策主题，并增加文档目录自身的入口主题：

| Topic | 本地权威文档 |
| --- | --- |
| `development_control_plane` | `specs/2026-08-24-ming-iot-direct-loopx-harness-retirement-design.md` |
| `first_delivery_decision` | `proposals/2026-08-23-first-real-delivery-loop.md` |
| `legacy_graph_retirement` | `specs/2026-08-23-ming-iot-rd-docs-archive-and-legacy-graph-retirement-design.md` |
| `local_share_migration` | `proposals/2026-08-25-ming-iot-local-share-migration.md` |
| `dmsvr_proto_domain_split` | `proposals/2026-08-25-dmsvr-proto-domain-split.md` |
| `engineering_organization_reference` | `references/unitedrhino-tech-arch-02.md` |
| `proto_generation_historical_reference` | `archives/snapshots/2026-08-23/ming-iot/docs/superpowers/specs/2026-07-23-core-goctl-migration-design.md` |
| `project_doc_registry` | `meta/DOC_REGISTRY.yaml` |

### 4.2 Goal 入口

`ming-iot/.codex/goals/ming-iot-framework-v1/ACTIVE_GOAL_STATE.md` 的 `Authority Sources` 必须声明：

- 项目文档目录的工作区相对位置；
- 先读取目录，再根据 Topic 打开正文；
- `.loopx/registry.json` 中的哈希只是来源指纹，不是文件定位器。

Active Goal State 是本机私有状态，可以保存工作区相对入口；公开仓库文件仍不得保存机器绝对路径。

### 4.3 哈希契约

正文完成目录登记并验证可读后，才允许执行：

1. `loopx import-doc-registry-authority --dry-run`；
2. 正式导入目录的脱敏摘要；
3. 新增单项来源时，先更新目录，再执行 `register-authority-source --dry-run` 和正式登记；
4. `refresh-state`；
5. `status` 或 `diagnose` 回读。

## 5. Agent 工作流约束

在 `ming-iot/AGENTS.md` 增加“持久资料登记”章节：

1. 任何需要跨 Chat 保留的设计、Proposal、研究资料、SOP、Evidence 或外部资料都触发 `loopx-doc-registry`；
2. 固定顺序为“更新项目权威表面 → 验证正文可读 → dry-run → 正式登记哈希 → refresh-state → 回读”；
3. 正文无法定位、目标 Goal 不明确或读取权限不明确时必须停止并写 Todo/Blocker；
4. 不得把只有 `source_ref_sha256` 的记录当成完整登记；
5. Authority 变更后必须运行项目侧聚焦校验。

这层规则是跨 Agent 的入口，Skill 仍是详细流程的权威来源，`AGENTS.md` 不复制 LoopX 内部实现。

## 6. 聚焦校验

在本地研发文档仓库增加：

```text
ming-iot-rd-docs/tools/check_authority_catalog.py
ming-iot-rd-docs/tests/test_authority_catalog.py
```

校验器只使用 Python 标准库，输入为显式的文档目录、文档仓库根目录和 `ming-iot/.loopx/registry.json`。它只读取以下稳定字段：

- `DOC_REGISTRY.yaml` 的 `default_entry_docs` 和 `topic_authority`；
- Goal 的 `authority_registry.topic_authority`；
- Goal 的 `authority_registry.project_materials`。

校验规则：

1. 文档目录存在，且只使用受支持的简单 YAML 结构；
2. `default_entry_docs` 和 `topic_authority` 中的路径都是仓库内相对路径；
3. 每个路径都存在并且是普通文件；
4. Registry 中每个 Topic 都能在文档目录找到；
5. Registry 中每个 Project Material 至少被一个 Topic 引用；
6. 每个 Topic 指向的 Project Material ID 必须存在；
7. 任一只有哈希、没有可读取文档映射的来源都使校验失败；
8. 输出只包含 Source ID、Topic、相对路径和错误代码，不输出绝对路径、原始 URL 或正文。

校验器不修改 Registry，不调用网络，不注册来源，也不成为全量构建依赖。它只在 Authority 或文档目录变化后运行。

## 7. 失败处理

- 文档路径不存在：停止登记，先修复目录或正文；
- Registry 有 Topic、目录无 Topic：判定为裸哈希风险并失败；
- 目录有 Topic、Registry 无 Topic：允许作为尚未激活的候选资料，但正式登记前必须完成映射；
- Project Material 未被 Topic 引用：失败，避免孤立来源；
- 读取权限不明确：不读取正文，写入用户 Gate 或 Agent Blocker；
- 来源发生冲突：遵循 Authority 的 `freshness`、`revision` 和 `conflict_rule`，不得由校验器自动裁决。

## 8. 测试策略

校验器采用 TDD，至少覆盖：

- 七项来源全部可解析时通过；
- Registry 多出一个裸哈希来源时失败；
- Topic 指向不存在的 Source ID 时失败；
- 文档路径不存在时失败；
- 绝对路径和越界路径被拒绝；
- 历史资料可以映射到 `archives/`，但仍必须存在；
- 输出不包含文档仓库绝对路径和原始来源引用。

配置与状态完成后还必须执行：

```text
聚焦单元测试
真实 DOC_REGISTRY 与真实本地 Registry 校验
loopx import-doc-registry-authority --dry-run
loopx status/diagnose 回读
git status --short
```

## 9. 迁移顺序与验收

迁移固定按以下顺序进行：

1. 创建并验证 `DOC_REGISTRY.yaml`；
2. 为现有七个 Topic 补齐本地相对路径，并登记文档目录自身的入口 Topic；
3. TDD 实现聚焦校验并证明当前裸哈希状态会失败；
4. 更新 `ming-iot/AGENTS.md`；
5. 更新 Active Goal 的目录入口；
6. 使用现有 LoopX CLI dry-run 并正式导入目录摘要；
7. 运行聚焦校验和状态回读；
8. 将本次用户纠正写回 Goal/Todo，避免规则只停留在 Chat。

验收条件：未来 Agent 从 `ming-iot` Goal 开始，不借助旧 Chat，可以定位 `DOC_REGISTRY.yaml`，根据任一已登记 Topic 打开对应正文；删除或遗漏任一映射时，聚焦校验稳定失败。
