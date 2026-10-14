# 文档字段、生命周期与命令

## 目录与身份

模块路径为 `map/<业务域>/<功能模块>`，模块 `spec.md` 承载完整要求：初版先定义待实现的要求，正式生效后维护完整现行行为。模块 README 区分实施/生效状态并索引活动和历史 Change；初版文件和 `status: current` 本身不证明能力已实现或已生效。模块根不放实施 Plan。

每个真实 Delivery Issue 对应一个主 Change：`changes/<TYPE>-<仓库名>-<Issue编号>_<名称>/`。TYPE 为 `FEATURE`、`PATCH`、`BUGFIX` 或 `SUNSET`。跨模块通过 `affected_modules` 和其他模块索引连接，不复制正文。Change 内 `README.md` 是 Hub；`change.md`、`plan.md`、`review.md`、`evidence.md` 分别保存增量、实施步骤、独立评审与验证摘要。

## Front Matter

使用简单标量和缩进两空格的字符串列表。所有正式正文有 `document_kind`、`status`；导航 README 可省略。精确字段校验以 `tools/map_docs.py` 为准。

| document_kind | 其余必填字段 | status |
|---|---|---|
| spec | module | current |
| change | change_type、delivery_id、issue、hub、target_module、affected_repositories、affected_modules、target_anchors | draft、approved、implementing、verifying、effective、rejected、superseded |
| plan | delivery_id、issue、hub、change_spec、affected_repositories | draft、approved、in_progress、completed、superseded |
| review | delivery_id、issue、hub、repository_heads | pass、changes_required |
| evidence | delivery_id、issue、hub、repository_heads | verified、failed |
| adr | module | accepted、superseded |
| reference | module、source | current、superseded |

Delivery ID 如 `ming-iot-rd-docs#IKEOFY`；Issue URL 的仓库和编号必须对应。Git identity 如 `git:gitee.com/ming-industry/ming-iot`；字段中的本地引用为仓库相对路径，Markdown 链接相对当前文档。Review/Evidence 的 `repository_heads` 使用 `Git identity@40位小写Head`，正文表逐仓列出 `repository`、`base`、`exact_head`、`verdict`；`base` 与 `exact_head` 均使用完整 40 位提交 SHA，普通检查即校验，不等到激活时才检查。阶段交付可为同一仓库记录多个候选，每行必须对应真实验证。

## 状态与激活

Change 从 draft 经人批准为 approved，实施时为 implementing，候选验证时为 verifying。approved、implementing、verifying 占用目标锚点；冲突交由人裁决。Draft 可以并行讨论，不能提前覆盖主规格。

新模块按主流程先完成初版规格，再初始化引用其中真实锚点的 FEATURE Change/Plan。锚点使用 `<a id="lowercase-ascii-kebab-case"></a>`。`__module_root__` 仅保留给缺少主规格的历史/过渡候选兼容，不作为新交付的推荐顺序；激活前必须替换为真实锚点。生效候选的变更段落附近反向链接来源 Change，模块 README 把它纳入历史索引。

`activate` 校验准备合入的候选树：候选 Change 为 effective、主规格和来源指针完整、Review/Evidence 对各仓库和候选均通过、前置阶段已合入。当前 activation PR 可以仍为 Draft/open；该候选合入前，目标分支的现行规格继续权威。离线命令不能证明远端已合并：Hub 中的 merge revision 必须先经 Gitee/Git 回读。

激活时 Hub 使用交付表记录参与仓库与候选，表头为 `role | repository | pr | base | exact_head | status | merge_revision`。`role` 是 `prerequisite`（前置）或 `current`（当前候选）；前置行必须 `merged` 并有 40 位 merge revision，当前行可 `draft`、`open` 或 `merged`。`base` 和 `exact_head` 均为 40 位提交 SHA，不能用分支名代替；每行的仓库与 Head 必须在 Review/Evidence 中得到对应通过结论。当前未合入行的 merge revision 用 `-`。表是对已核实外部事实的记录，不是远端状态查询器；作者与 Reviewer 负责完整列出 Plan 的前置阶段，不能通过漏列依赖绕过合并顺序。

拒绝方案标为 rejected；被替代方案标为 superseded 并指向后继。未完成的外部反链、CI 或跨仓依赖如实记录，不写成 Effective。

## 命令

在选定文档 worktree 根执行，具体参数可查 `--help`。新模块先完成主流程第 2 步的初版规格；既有模块先读取现行规格及覆盖范围。再从已选交付的真实 Issue 和批准范围设置以下变量；缺值即停止，不提供可误抄到另一交付的默认值：

```bash
python tools/map_docs.py init \
  --docs-root . --module "${map_module:?设置目标模块路径}" \
  --type "${map_type:?设置Change TYPE}" --delivery-id "${map_delivery_id:?设置真实Delivery ID}" \
  --name "${map_name:?设置变更名称}" \
  --issue "${map_issue_url:?设置真实Issue URL}" \
  --repository "${map_repository:?设置Git identity}"
```

初始化只完成骨架；按主流程补齐正文、模块/地图索引和实际导航后，再运行 `python tools/map_docs.py check --docs-root .`。空骨架或缺少索引的全新目录不承诺立即通过 check。

只有申请激活时，才对已经准备好的目标 Change 执行（不把 init 后的草稿直接激活）：

```bash
python tools/map_docs.py activate --docs-root . \
  --change "${map_change:?设置目标Change目录}"
```

`init` 只生成 Hub 和 Change 骨架，不生成语义 Plan/Review/Evidence；已有内容冲突时不覆盖。`check` 和 `activate` 只读。

`check` 总是执行全树结构检查，并对相对基线新增/修改的 effective Change、或其目标主规格段落变化，复用 `activate` 的同一套门禁。默认基线为 `git merge-base origin/master HEAD`；可显式使用 `--base <可信提交>`。没有可用默认基线时保守检查全部 effective Change；显式基线错误则失败。CI 必须检出候选树并提供正确目标分支基线，不能用候选 HEAD 自证无差异。未变化的历史 effective Change 不因未来新 Change 占用同锚点而重新激活；普通 verifying 候选不强制激活。

导航必须是读者可见的实际链接：内联链接或被使用的 full/collapsed/shortcut reference link。单独的引用定义、图片和转义文字不算导航；未使用引用定义的目标仍参与失效链接检查。

导航采用受限 Markdown 排版：必要入口优先独立成段或使用顶层列表；四空格嵌套列表和双反斜杠前的链接目前可能被误拒，不以这些写法承载唯一入口。此限制不是完整 CommonMark 支持声明。

## 检查失败分流

- 授权内的文档错误（如缺字段、坏链接）：修正目标文档并重跑 check，直到通过或发现新的权限/语义缺口。
- map 内生效记录不明或覆盖冲突：保留现行权威和候选，记录冲突对象、锚点及依据，交有权限的人裁决；不得靠改 status 或覆盖主规格绕过。
- 疑似工具缺陷：保留精确 Head、命令、退出码及脱敏最小复现，在当前交付记录缺口。报错本身不授权修改校验规则；获得对应修复范围后再改工具与回归。

## 远端导航与本地注册

Issue 反链使用 `python tools/gitee_issue.py link --help` 所列参数。未验证 Gitee 原子条件更新能力之前，工具只 GET，绝不 PATCH 正文或标题：导航块缺失/过期时输出 `managed_block`、`manual_update_required=true` 并退出 2，由有权限的人在最新正文中补齐；随后重跑，只有两次读取确认导航正确才返回 `linked=true`、退出 0。错误退出 1。不得将退出 2 当作已完成反链，不输出原始人工正文。回读是当时观察，不保证未来不变，也不承诺人工编辑的原子性。Draft PR 使用 `python tools/gitee_pr.py --help`，一个调用对应一个仓库。CI 不持有这两个命令的令牌；外部写入按仓库授权执行。

团队分发源已在 `git:gitee.com/ming-industry/ai-native` 的 `skills/map-delivery/` 准备就绪（`source-ready`），来源迁移交付为 `ming-iot#IKJMKJ`。先发布团队源，再合入文档仓删除原件的迁移 PR；此前主工作树保留原件和现有正式入口。迁移 PR 中的文档仓注册兼容入口支持 `--skills-repo` 指定团队技能仓，按团队内容锁验证来源后安装；必需技能复检使用团队仓工具。主工作树的来源切换在该 PR 合入后执行。安装校验与宿主实际加载分别验证；分发成功不证明宿主已加载。正文和工具始终来自已核验的文档工作树，定位见 [文档仓定位](locator.md)；安装不改变该仓治理或授权边界。
