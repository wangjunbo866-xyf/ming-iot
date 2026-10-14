# OpenSpec 原生功能、补丁与退役演进核查

日期：2026-10-10。固定版本：OpenSpec `v1.14.1`，完整 Git SHA `87c3595ace6a2e22957f39ebe5b74c2f2316e0cb`。依据官方网页和该版本源码静态核读；未安装或运行 OpenSpec，也未创建业务 Change。

## 结论

原生 OpenSpec 可以保留团队需要的 **feature / patch** 区分，也能表达 sunset；blueprint 可以作为探索分类。四者是团队的语义约定，不是 OpenSpec 内置四类状态机。它们可以共用原生 Change、delta 和 archive，无需建立第二套规格、补丁覆盖链或另一个框架。

业务地图按持久能力分，Change 按一次修改意图分；主规格保存已整合的行为全文，读者不需要从最初 feature 再顺次叠加全部 patch 才知道现行要求。该结论来自下面的原生差量语义；具体团队目录和批准策略仍待选择。

## 已确认事实

| 问题 | v1.14.1 事实及官方证据 | 对团队的含义（推论/建议） |
| --- | --- | --- |
| 主规格与差量 | `openspec/specs` 是主规格；`changes/<change>/specs` 表达 ADDED/MODIFIED/REMOVED/RENAMED；归档将差量并回主规格，保留 Change 历史。[Concepts](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/concepts.md) | 每个能力只维护一份现行全文；feature/patch 是修改它的历史，不是并行权威。规格集合可能尚未覆盖所有旧代码，“现行全文”仅指已建模能力。 |
| MODIFIED 的完整度 | 必须提供受影响**单条 requirement 的完整新正文和全部 scenarios**；默认 schema 要求从现行要求整块复制再修改；归档实现还检查原有场景是否遗漏。[schema](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/schemas/spec-driven/schema.yaml)、[替换实现](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/specs-apply.ts#L527) | patch 无需复制整个 feature 或 capability。不要只写“将 30 秒改成 60 秒”而丢失其余场景；也不要把所有无关要求复制进补丁。 |
| 四类分类 | change metadata 已知字段没有 feature/patch/sunset/blueprint 类型枚举；未知字段不形成原生能力。Change 名遵循 lowercase kebab-case，允许小写字母、数字、单连字符。[metadata schema](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/change-metadata/schema.ts)、[名称校验](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/utils/change-utils.ts#L44) | `feature-telemetry-query`、`patch-telemetry-query-timeout`、`sunset-legacy-ingest` 都是合法命名示例。前缀方便识别，不会自动改变流程或批准状态。 |
| Change 层级 | 活动 Change 的发现读取 `changes/` 下第一层目录；能力规格的发现则是递归的。[item-discovery](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/utils/item-discovery.ts) | 不要为四类创建 `changes/feature/foo/`、`changes/patch/bar/`；用一级 Change slug 加前缀，或正文说明分类。 |
| 不改变既定行为的修复 | `.openspec.yaml` 支持 boolean `skip_specs: true`；无 delta 可以校验，specs artifact 显式跳过；若同时存在 spec delta，文档说明会被判冲突。[示例 Recipe 5](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/examples.md#recipe-5-a-refactor-with-no-behavior-change)、[metadata](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/change-metadata/schema.ts#L49) | 代码修复若只是让实现重新符合已存在的行为要求，可保留 patch、任务及验证，不凭空修改规范。若新增/改变行为要求或场景，则正常写 delta，不用 skip 绕过。 |
| skip 的 CLI 边界 | `new change` 的创建选项未提供 `--skip-specs`；长期标记写在变更 metadata。`archive --skip-specs` 是一次归档跳过规格更新，含义不同。[CLI 注册](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/cli/index.ts#L802)、[CLI 文档](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/cli.md#openspec-archive) | 先用原生命令建立 Change，再编辑其 metadata；不要发明 `new change --skip-specs` 命令，也不要用 archive flag 掩盖未处理的业务差量。 |
| 部分行为退役 | REMOVED 从主规格移除指定要求；默认 schema 指导给出 Reason 和 Migration。[schema](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/schemas/spec-driven/schema.yaml) | sunset 不必等于整个模块消失；可只退役旧接口、旧协议或某项行为，其余要求保持。 |
| 整个 capability 退役 | 删除最后一条 requirement 时，需 `.openspec.yaml` 的 `retire_capabilities: true`；符合条件才删除主 `spec.md`。额外 Notes/注释等可能阻止退役，避免静默丢内容；`--no-validate` 不执行这种退役。[Writing Specs](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/writing-specs.md#pick-the-right-kind-of-delta)、[CLI](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/cli.md#openspec-archive) | 不靠空规格伪装已退役；批准的迁移、替代入口和停用安排保留在 Change/导航。删除文件不抹除已提交 Git 历史。 |
| blueprint 探索 | explore 是讨论与调查模式，不默认写文件或归档；用户要求捕获时可以建 Change 并写明确指定的产物。[Explore](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/explore.md) | 可先保留在会话或团队研究区；要持久规划再转 Change。`blueprint-` 前缀没有原生“未批准”隔离语义，必须明确标识，不写成当前行为或自动授权实施。 |

两个 metadata 示例是独立情形，不应机械地组合使用；已有 `schema`/`created` 等字段应保留：

```yaml
# 无规格行为变更：changes/patch-.../.openspec.yaml
schema: spec-driven
skip_specs: true
```

```yaml
# 明确允许退役满足条件的 capability：changes/sunset-.../.openspec.yaml
schema: spec-driven
retire_capabilities: true
```

## 层次 capability 路径：有原生支持，仍要固定版本

对 `things/telemetry-ingest` 这类路径，v1.14.1 静态实现链已核对：

- **list**：使用递归 `discoverSpecFiles`，返回 `/` 分隔的相对 ID；[list.ts](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/list.ts#L263)、[discovery](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/utils/spec-discovery.ts#L23)。
- **show**：用所给 spec ID 拼接 `<specsDir>/<id>/spec.md`，递归发现也用于候选列表；[spec.ts](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/commands/spec.ts#L104)。建议显式 `openspec show things/telemetry-ingest --type spec`，避免与同名 Change 混淆。
- **默认 schema**：明确支持 `identity/user-auth` 式路径，要求 delta 保留现行能力完整路径；[schema.yaml](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/schemas/spec-driven/schema.yaml)。
- **archive**：递归发现 delta，并将相对 ID 映射到主规格的相同路径；[specs-apply.ts](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/specs-apply.ts#L118)。

因此不能笼统说 OpenSpec 只支持平铺能力。这里是静态核查，未实测当前团队宿主、旧版本或自定义 schema；源码注释也表明递归支持修复过遗漏嵌套规格的问题。团队混用旧版且暂不做兼容试验时，可先用 `things-telemetry-ingest` 这类 flat ID，把业务层级留在薄导航；统一固定版本并在隔离样例走通 list/show/validate/archive 后再选择层次路径。不要为展示分类而随意移动已存在的 capability。

## archive/sync 与实际发布是两条事实链

`/opsx:sync` 的工作是将差量整合回主规格，可以在 Change 仍活动时进行；`archive` 通常整合差量并将 Change 移到带日期的历史目录。[sync 工作流](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/src/core/templates/workflows/sync-specs.ts)、[archive 说明](https://github.com/Fission-AI/OpenSpec/blob/87c3595ace6a2e22957f39ebe5b74c2f2316e0cb/docs/cli.md#openspec-archive)

据此必须区分“主规格已整合”与“批准、代码合并、部署环境已生效”。这些原生动作没有自动证明团队批准或生产部署；团队应自行规定何时 sync/archive，并保留目标环境、版本与发布事实的链接。archive 的命令确认或 `--yes` 也不是业务批准证明。

## 薄业务地图的建议（未写入业务仓）

按稳定业务能力组织，例如“Things → 设备接入、遥测接收、遥测查询、设备在线状态”；这是示意划分，尚非对现有平台模块的认定。前端/API/存储/消息链路作为每项能力的实现入口，不另复制一份行为规格。

导航每项只需列能力名称、唯一 spec 路径、必要代码/接口/数据入口、当前相关 Change 链接。`feature-*` 创建或扩展能力，`patch-*` 修改既有能力或修复实现，`sunset-*` 记录退役范围；一次 Change 可以触及多个能力。`blueprint` 探索单列为未批准，不与当前能力混写。分类可用命名前缀和说明完成，不要求四套 schema、四棵目录或四份状态。

## 核查与提交验证

仅静态核读官方固定版本，事实核查未执行 CLI；源码缓存位于 `/tmp/openspec-feature-patch-v1.14.1`。只提交本文，不修改主树票据、导航或业务规格。

提交前按 README 检查：`python3 -m unittest discover -s tests -v`，52 项通过（2.121 秒）；`python3 tools/team_skills.py validate` 返回 `validate: OK (map-delivery)`。它们仅验证本文所在配置仓的既有工具，不证明 OpenSpec 的实际行为或本文示例已实施。
