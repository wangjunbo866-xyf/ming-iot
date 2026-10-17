# ming-iot 协作入口

适用于人工开发和任何 coding agent；子目录的更具体约定同时适用。

## 开始任务

1. 新成员、新 Agent 或新 clone 先读[跨 Agent 接入](docs/runbook/cross-agent-development.md)，确认任务来源、授权范围、当前分支与实际能力。普通开发以原任务/Issue/PR 记录责任、决定和验证，不要求安装 LoopX、创建 Goal 或复制维护者私有状态。
2. 需求澄清、功能调整、小修复或接手，从[业务地图](docs/specs/README.md)进入模块 README 和当前 spec，遵循[团队约定](docs/runbook/team-development.md)。已采用规格维护方法的任务可读取 [ming-specs](.agents/skills/ming-specs/SKILL.md)；宿主未发现 Skill 时可直接读文件。业务取舍由实际负责人确认，AI 建议不代替批准。
3. **任务已明确纳入 LoopX 时**，先读 [LoopX 任务约定](docs/runbook/loopx-development.md)，核验指定 Goal、身份、Todo、写范围和 Gate。连接不可用则恢复该任务入口；不能改称普通任务绕开已有约束。仅发现仓内有 LoopX 文件或 Goal 名称不表示当前任务已被纳管。
4. 明确规则是否变化及验证方法后，在独立分支/工作树修改；保留已有候选和无关改动。旧仓、历史回执和私有备份继续保留；当前产品修改在本仓进行。

## 仓库地图

- 代码：`backend/saas`、`backend/share`、`backend/things`。Core/Share/Things 已合入，不从同名兄弟仓找实现。
- 目标、路线和验收：[AI 协作研发总纲](docs/project-charter.md)。业务规则：[知识导航](docs/README.md)和[来源索引](docs/index.yaml)。历史材料只解释原时点。
- 日常准备：[开发入口](docs/runbook/daily-development.md)。验证环境：[部署说明](deploy/README.md)。ThingsBoard/AirMQ 为仓外学习参考；旧 r-doc、map-delivery 和分发工具不作为默认依赖。
- 现有长程工作使用同一 `ming-iot-knowledge-pilot`，经 `tooling/loopx` 接入；迁移事实与保留边界见 [M5](docs/changelog/2026-10-15-m5-cutover.md)。小黄反馈及其依赖任务保持废弃。

## 规格、差异与交付

- 规则和人的决定写回模块当前正文或原任务可追溯来源；导航、Skill、索引只指向它。当前来源登记在 `docs/index.yaml`；LoopX 资料登记仅用于已纳管任务。
- 规则不变的小修改引用现行版本、条款、范围、原因及验证；不为凑文件数新建 Patch。规则变化按团队约定确认，并在同一候选同步主规格、Patch 和双向指针。
- `make knowledge-check` 检查显式文档范围及工具回归。普通代码 PR 运行 `make verify-pr`；真实依赖另选 integration suite，发布候选才使用带重型确认的 release profile。具体选择看 `make help` 和部署说明。
- 完整 PR 差异另用[PR 检查约定](.agents/skills/ming-specs/references/pr-check.md)：明确目标分支 tip、候选完整 Head 和声明。`make verify-pr` 不自动取得 PR 正文；结构通过不等于语义、授权或真实依赖验证通过。
- 实施者提供不可变候选、实际检查结果和交付物；独立 Reviewer 核对同一 Head，未经授权不修改实现。候选变化后重验并复审受影响部分。未运行、失败和依赖缺失分别记录，不通过缩小范围获得绿色。
- 普通交付复用原任务/PR/CI 保存结果，从模块入口和验证方法接手，不默认生成交接包。长期 Smoke 只覆盖可复用契约、真实回归或代表性路径，临时分析留在当次证据。

## Git、公开和高风险边界

- 已授权任务在独立分支/工作树、约定写范围内完成验证与公共/私有边界扫描后，Agent 可本地 commit。LoopX 任务另须满足其 Todo 和准入约束。只有当前任务允许远程交付时才 push 或创建/更新 PR；旧发布回执不自动授权新候选。
- 跟踪文件通过 PR 进入默认分支，禁止直接 push 默认分支的产品变更。首次空仓可建立不含文件的根提交作为 PR 基线。
- PR 默认由有权限的人评审和合并；人执行合并即为该次授权。Agent 合并需绑定精确 PR、Head、目标分支和方式的授权；LoopX 任务另登记对应 User Gate。
- 发布、生产、凭据处理和权限变更及重写已发布共享功能分支需精确目标与动作的授权；LoopX 任务须绑定相应 Gate。受保护分支历史禁止重写。
- 公开前遵守 [PUBLICATION_POLICY](tooling/policy/PUBLICATION_POLICY.json)，HOLD 状态不发布。原始日志、私有路径、凭据、数据库和维护者个人配置不进入公开提交。
- 本地检查不代替托管 CI；CI 绿色不证明必需检查已启用，也不授予合并、发布或生产权限。本轮跨 Agent 试跑不启用新强制门禁。
