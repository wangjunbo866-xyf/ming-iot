# 完整 PR 差异检查

适用于 ming-specs 0.10.0-draft，沿用 S2 检查协议。业务规则仍在模块当前 spec；此文只定义检查输入、输出与局限。

## 谁维护什么

- 人/评审者确认业务决定、模块边界、轻量声明是否真实、未登记模块的处理与归档价值。字段齐全不是身份认证。
- AI 从实际 diff 找模块、填写 PR 声明、补指针、运行检查并解释差异；不能以代码已实现为由修改需求。
- 工具核对完整路径覆盖、当前版本/显式条款、活动 Patch 双向指针、归档字节与候选身份；不判断业务语义。
- 模块入口或实现路径变化时，在同一 PR 修改 [specs-map.json](../../../../tooling/specs-map.json)。映射只有路径和正文位置，不能写第二份规则。旧/新映射共同归属的路径仍须覆盖；删除一个已登记模块须另设计明确的退役迁移，本版不支持静默删除。

设计 Feature 可以先登记文档地图和来源索引；产品路径尚未认领时，不把文档登记说成差异映射已覆盖，实施前按真实范围补齐。

检查时读取候选实际映射；本地 P1 已在通知与 API 校验之外登记 `platform.eventbus`。映射到一个代码文件只说明应向哪个模块解释影响，不证明文件中的所有逻辑已有规格。

域、Feature、服务及 `module`、变更 `type`、检查 `kind` 的含义见[划分标准](../../../../docs/runbook/team-development.md#domain-feature-service)。不能因为长期目录叫 Feature 就填写 `kind=feature`，也不能把尚不支持自动检查的类型伪装为 Patch。

## 从实际 PR 生成输入

PR 正文中恰好一个行首 fenced `ming-specs` JSON 块；不要同时保留模板和示例两个块。只支持 schema_version 1。正文编辑触发独立工作流重跑；旧结果不能覆盖新的声明。

本地先提交候选，再将同一 JSON 保存到仓外临时文件，从仓库根执行：

```bash
python3 tooling/specs_pr.py --base <目标分支完整SHA> --head <候选完整SHA> --declaration <JSON路径>
```

GitHub 使用 `python3 tooling/specs_pr.py --event "$GITHUB_EVENT_PATH"`。工具核对检出的 HEAD 正是事件 head，计算 base/head 的唯一 merge-base。不能使用只包含最后一次提交的范围冒充完整 PR。输出 requested_base、merge_base、candidate_commit/tree、完整 changed_files 和 diff/declaration/map 指纹。读取 Git 对象，不读取未提交的规格修复；未提交路径另报。

仓库声明范围只能使用具体路径或目录后缀 `/**`，不支持任意全仓通配。Patch 本身沿用原 write_scope 格式。规则如下：

| 内容 | 声明 | 工具执行 |
| --- | --- | --- |
| 已登记模块的业务变化 | modules 中 kind=patch，指定 change 与 revision | v2 投影、当前 revision、confirmed/effective/本次 absorbed、双向指针、范围；同 PR 必须修改主规格 |
| 规则不变的修复/文档改进 | kind=no_behavior_change，spec_version、clauses、paths、reason | 当前版本、显式稳定条款、所有归属路径覆盖；不要求新 Patch |
| 普通文档 | exceptions 中 kind=documentation | 仅未登记模块、非 legacy 的 Markdown 路径；保留原因 |
| 团队工具 | kind=tooling | tooling、.github、.agents/skills、Makefile 或 docs/index.yaml；不能绕过登记模块 |
| 历史材料新增/整理 | kind=history | 只允许 legacy 中仍存在的路径；不能豁免删去的源文件 |
| 旧资料完整迁入 legacy | archives 中 from、to、reason | 实际删除源/新增目标、相同 blob 和模式；当前 spec/模块入口不能被静默归档 |
| 产品路径还没有登记模块 | kind=unmapped，paths、reason、reference | 只限 backend/deploy/apps/components，reference 为候选内可读文件；单独报告待评审缺口 |

`no_behavior_change` 的判定对象是约定的预期行为。修复错误实现可以改变错误输出，但要引用被恢复的现行条款；测试通过或同文件已有其他条款不能替代这一依据。规则有歧义或新增预期时按[小修改处理步骤](../../../../docs/runbook/team-development.md#small-changes)澄清和分类。权限、数据、兼容等高影响修复仍须对应验证，本声明不减免代码 PR 的现行检查。

`unmapped` 是明确暴露的治理缺口，不是“没有 Spec 就不用审”。未声明、越界或已登记模块试图用例外绕过会失败。相同路径属于多个模块时，必须分别解释其对每个模块的影响；单个宽范围认领不能替代其他模块。

行为变化在主规格中的文字是否正确、reason 是否可信、共享契约是否遗漏均由独立评审确认。将行为变化误标为 no_behavior_change 不会被结构检查自动识别。feature/sunset/blueprint 继续遵循团队约定；本检查器只自动检查 Patch 与不改变规则的声明，其他类型的状态机和退出迁移尚未实现，不能伪装类型取得绿色。

## 最小示例

规则不变的小修复，无需新 Patch 目录：

```json
{
  "schema_version": 1,
  "modules": [{
    "module": "saas.notification-trigger",
    "kind": "no_behavior_change",
    "spec_version": "current-v1",
    "clauses": ["display-name"],
    "paths": ["backend/saas/service/syssvr/internal/logic/notifymanage/triggerUserParams_test.go"],
    "reason": "补充现行昵称回退规则的回归，用户可见规则未变；需评审核对实际测试"
  }],
  "exceptions": [],
  "archives": []
}
```

多模块变更在 modules 增加另一条；业务变化使用下面字段，路径和版本以实际文件为准，不能直接复用已吸收的旧 Patch 来授权新行为：

```json
{"module":"engineering.api-contract","kind":"patch","change":"docs/specs/engineering/api-contract/patches/<实际任务>/check.json","revision":"R1"}
```

纯历史移动，无需为旧正文修复历史链接：

```json
{
  "schema_version": 1,
  "archives": [{"from":"docs/old-design.md","to":"docs/wiki/legacy/design/old-design.md","reason":"保留原始资料，退出当前入口"}]
}
```

移动已登记模块下的历史资料还要声明该模块的导航影响。当前主规格和模块入口本身必须继续存在并可读；归档不扩大删除权限。旧仓、备份和历史回执的保留要求不因检查成功而改变。

## 历史、可复现性与限制

模块当前入口/主规格的站内链接与锚点始终检查。活动 Patch，以及本 PR 新增、修改、引用的 Patch，检查结构和版本。未修改且未被引用的 absorbed/withdrawn Patch 保留原适用版本，不要求为了新规格改写历史。输出 historical_projections_not_revalidated 明示其未按当前规格重新验证；旧的独立 preflight 仍可对原提交核验。

链接检测支持当前文档使用的行内 Markdown、标题与独占一行的显式锚点，忽略 fenced 示例，不是完整 Markdown 渲染器；不访问外部链接。历史资料正文不递归纳入当前断链检查。

成功仅表示 `STRUCTURE_OK_REVIEW_REQUIRED`；semantic_review=NOT_RUN、authorization=NOT_VERIFIED、server_merge_rules=NOT_VERIFIED。测试和语义 Review 的有效性由实际回执核对。`make verify-pr` 包含检查器回归，完整 PR 检查另需真实 base/head/声明。零差异只可作结构检查，不能作为某个非空 PR 已检查的证据。

S2 工作流与检查器已随 PR #5 合并，真实托管结果见[合并记录](../../../../docs/changelog/2026-10-16-specs-merged-next-plan.md)；本地事件回放仍不能替代新候选的托管运行，也不自动启用服务器必需检查。本工具使用候选中的检查代码及映射，对它们的变更必须可信评审；不能充当抵抗恶意 PR 的安全边界。
