# ming-iot 旧 Graph 当前树退役 Review

## Review 身份

- 类型：执行者自检
- 独立性：非独立 Review；当前环境没有可调用的独立 Reviewer Agent
- 状态：用户已批准本地提交与合并；push 前仍建议由独立 Reviewer 复核
- 对象：提交 `151e193` 相对 `4b80c51` 的变更

## 需求符合性

| 验收项 | 结果 | 依据 |
| --- | --- | --- |
| `.graph` 当前树退役 | 通过 | 4 个 tracked 文件均标记删除，工作树无文件 |
| `graphctl` 当前树退役 | 通过 | 49 个 tracked 源码/测试文件均标记删除，工作树无文件 |
| 活动说明不再指向旧 Graph | 通过 | README 与 `.gitignore` 已清理，活动引用扫描为零 |
| 保留确定性工程校验 | 通过 | `thingsbaseline` 等包的全量、Race、Vet 均通过 |
| 防止旧控制面回流 | 通过 | 新增仓库契约测试，先 RED 后 GREEN |
| 不扩大到历史重写 | 通过 | `docs/superpowers/**` 和 Git 历史未修改 |
| 不越过提交/推送权限 | 通过 | 用户选择选项 1 后才执行 commit 与本地 merge；未执行 push |

## 代码与测试检查

- 仓库根目录识别同时适用于普通 checkout 和 `.git` 为文件的 worktree。
- 契约只禁止退役目录中的文件，不把 Git 不跟踪的空目录当成可发布控制面。
- 活动引用检查限定在 `.gitignore`、`AGENTS.md` 和 `backend/tools/README.md`，避免把契约测试自身和待归档历史材料误报为回流。
- `go mod tidy` 只删除失去使用者的 `jsonschema/v6`、`regexp2` 条目；仍被其它工具使用的 `yaml.v3` 保留。
- `git diff --check`、聚焦测试、全量测试、Race 和 Vet 均通过。

## 风险与后续

1. 当前 Review 不是独立评审。合并前应由另一 Agent 或人工 Reviewer 检查删除范围和新契约测试。
2. `docs/superpowers/**` 仍保留旧 Graph 历史文本；这是已知后续 Gate，不应被误认为当前活动控制面。
3. 合并时发现并清除了主工作区原有的 41 个 `.graph/runtime` 可再生文件；合并后路径扫描确认 `.graph` 不存在。

## 自检结论

未发现阻止本地集成的 Critical 或 Important 问题。提交 `151e193` 已 fast-forward 合并到本地 `master`，合并后全量测试、Race 和 Vet 通过；不得把本自检当成独立 Review 已完成，也不构成 push 授权。
