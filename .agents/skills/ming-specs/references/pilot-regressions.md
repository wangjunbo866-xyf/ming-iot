# 真实 Patch 试跑与回归

本资料记录试跑方法，不另建业务权威。当前行为和验收从 [API 主规格](../../../../docs/specs/engineering/api-contract/spec.md)与[通知主规格](../../../../docs/specs/saas/notification-trigger/spec.md)读取，已吸收 Patch 只作追溯。

## 手动检查

在目标 ming-iot 工作树根运行。`<本次Patch投影>`、`<新仓实际基线SHA>` 必须替换为本次真实输入；旧试点的认领不覆盖任意后续差异。没有适用投影时按约定审查完整实际 diff，不硬套历史范围：

```bash
python3 .agents/skills/ming-specs/scripts/preflight.py \
  --change <本次Patch投影> \
  --base <新仓实际基线SHA> --staged
python3 -B -m unittest discover -s .agents/skills/ming-specs/scripts -p 'test_*.py' -v
go -C backend/tools test ./cmd/apicheck -run '^TestCLILint' -count=1
make verify-pr
```

分支检查把 `--staged` 替换为 `--head <完整commit>`。检查器从 Git 对象读 JSON、规格和 Patch，输出候选 tree、diff/输入指纹及完整变更路径；分别比较候选与当前索引、工作区、HEAD，避免固定旧提交时漏提示后来暂存或提交的修改。确认输出后还需检查逐条 AC、代码及实际测试，不能只引用退出 0。

`check.json` 是试跑的认领投影。v1 保留原有状态字段；v2 只保存 module/task、确认来源、类型、claim、spec#anchor、spec_version、backlink#anchor、write_scope，当前 status/revision 从 Change 开头元数据读取。路径相对仓库根，链接使用普通相对 Markdown 链接，条款使用显式 `<a id="..."></a>`；不支持所有 Markdown 语法。`write_scope` 为 Python fnmatch 模式（`*` 可跨 `/`），优先使用精确文件或明确目录的 `/**`，不把它当权限边界。

只检查已指定的 Patch，不自动发现全部模块，不校验人的身份、审批、状态转换、并发冲突或 Spec-Code 语义。工具退出 0 的结果仍为 `STRUCTURE_OK_REVIEW_REQUIRED`、`semantic_review=NOT_RUN`、`authorization=NOT_VERIFIED`。工具不检测门禁配置，输出 `hook_ci=NOT_VERIFIED`。当前 `knowledge-check` 已运行检查器回归，但实际 PR 的 base/head 与 Patch 映射尚未接入；不能混称规格门禁已启用。远程合并规则需另行回读。

新仓参考基线为 `c165a56531f58421a870a0b8794f24556b60d3b2`，S1 记录绑定本轮候选。旧仓 SHA 无法解析时保留旧回执；零差异检查只核验指针/字段，不证明实际修改范围、业务语义或回归通过。跨模块全差异目前需人工/AI 审查，不放宽旧认领来取得绿色。

## 必须保留的反例

| 场景 | 应看到的结果 |
| --- | --- |
| 真实 lint 指向不存在文件、空白或混合列表 | 编译后 CLI 退出 2，不含 lint: ok |
| 合法路径加 ./，文件实际有元数据问题 | 仍失败，不能因诊断文件路径不匹配漏检 |
| 暂存区反向指针缺失，工作区已修好但没暂存 | preflight 仍拒绝；重新暂存后才恢复 |
| 条款锚点缺失或候选路径超出认领投影 | preflight 退出 1，并指出实际缺口 |
| 本地未跟踪文件/未暂存修改 | 明确在候选外，不冒充被检查或被修复 |
| 固定 commit 后工作区改变 | 检查固定 commit；不把工作区内容替换进去 |
| 固定旧 commit 后修改已暂存 / 已提交 | candidate_vs_index/worktree/current_head 分别指出差异，不能用工作区干净证明当前内容都已检查 |

上述结构反例使用隔离临时 Git 仓库；只有 lint 修复是实际项目变更。负例不是生产事故记录，也不证明服务端已拦截。

## 固定版本交接

普通接手无需本节。仅在离线、跨权限或特定外部交付需要冻结输入，且已选择 v2、状态为 confirmed/effective 的 Patch 时生成实施快照。absorbed 历史 Patch 不再生成实施快照。Change 开头示例：

```yaml
---
status: confirmed
revision: R2
---
```

先提交并确定真实基线、候选 SHA，交出者运行：

```bash
python3 .agents/skills/ming-specs/scripts/preflight.py \
  --change <仓库相对check.json> --base <比较基线SHA> --head <候选SHA> \
  --emit-handoff > <仓库外的交接JSON>
```

检查退出码后，把该 JSON 保存为接收者可访问的任务附件。范围仍须覆盖本次差异；比较基线不能为规避越界或遗漏而缩小。正文格式调整后仍需确认含义未变。附件包含仓库相对路径和任务元数据，应遵循项目原有资料边界。

接收者先保存自身改动，在指定候选的干净检出中运行：

```bash
python3 .agents/skills/ming-specs/scripts/preflight.py \
  --change <同一路径> --base <同一基线SHA> --head <收到的候选SHA> \
  --handoff <收到的交接JSON>
```

快照的基线、候选 commit/tree、任务、模块、revision/status、范围、输入指纹或 diff 指纹不一致，均退出 1；已暂存、未暂存及后来提交的受跟踪改动也会阻止实施接收。HEAD 直接比较提交身份，相同内容的另一个提交也不接受。diff 指纹使用固定排序的原始 Git 对象差异（mode/blob/path），不依赖本机前缀、颜色、上下文等补丁显示配置。未跟踪文件仍明确列为候选外，不自动删除。快照绑定固定候选，不能将旧包重新生成后冒充接收成功。

匹配输出仍是 `STRUCTURE_OK_REVIEW_REQUIRED`，另带 `handoff_check=MATCHED_REVIEW_REQUIRED`，语义、真实授权与 Hook/CI 均未自动验证。生成包不是完成交接；人/AI 接收回执仍须复述规则并复跑行为验证。快照也不验证测试和 Review 附件真实性，更不证明它是远端最新版本或来自可信的人。

新增回归覆盖 H1 式旧 revision、H2 正常接收、每项绑定失配、暂存遗漏、正文历史版本误识别、脏工作树与后来提交、旧格式兼容和非实施状态。历史 H1/H2 原包没有这些机器字段，回放需明确标注字段适配和来源指纹，不能声称新工具当时已经运行。

## 新会话交接检查

独立检查者只给仓库、候选 commit 和本 Skill 入口：从地图与模块 README 找到完整当前规则、版本、责任缺口及验证命令，按需读取在途 Patch 和历史原因。复跑相关测试；仅对适用的当前投影运行真实差异 preflight，并说明工具未覆盖部分。报告语义差异及配置未知项，不能仅复述实施者报告。

历史试跑记录：当时 skill-up 0.12.0 在本机可用，但未配置其独立 Engine 的 API 凭据，框架运行记为 NOT_RUN；不把该时点配置视为当前事实。实际试跑、确定性回归和宿主原生独立评审分别留证，不将它们改称 skill-up 批量评测通过。
