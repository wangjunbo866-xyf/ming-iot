# ming-iot Delivery Pull Request

> 本模板只定义交付信息结构，不代表任何验证已经通过。请在 Gitee 创建 Pull Request 时选择本模板；若平台未自动载入，请在创建页面选择“使用 Pull Request 模板”。

## Delivery Issue

- Gitee Issue：`#<Issue ident>`
- Delivery ID：`<delivery-id>`

## 变更目的与边界

- 目的：`<本次交付解决的问题>`
- 包含范围：`<代码、配置或文档边界>`
- 明确不包含：`<延期项或非目标>`

## RFC / Spec / Plan

研发材料来自私有 `ming-iot-rd-docs` 仓库；每项必须填写仓库相对路径与已合并 revision。

| 类型 | 仓库相对路径 | 已合并 revision |
| --- | --- | --- |
| RFC | `<rfcs/...>` | `<40 位 Git revision>` |
| Spec | `<specs/...>` | `<40 位 Git revision>` |
| Plan | `<plans/...>` | `<40 位 Git revision>` |

## 稳定项目契约晋升

- 本次晋升到产品仓的稳定契约：`<测试、Contract、Makefile、工具或 AGENTS.md 规则>`
- 未产生新的稳定项目契约时说明原因：`<原因>`

## Verification Profile

不要用勾选框代替 Evidence。逐项填写实际命令、结论和证据指针；存在未运行项时必须填写原因和补验证方式。

| Profile / 检查 | 实际命令 | 结论 | Evidence ref |
| --- | --- | --- | --- |
| fast | `<命令>` | `<实际结论>` | `<证据指针>` |
| PR | `<命令>` | `<实际结论>` | `<证据指针>` |
| integration | `<命令或未运行项>` | `<实际结论>` | `<证据指针>` |
| release | `<命令或未运行项>` | `<实际结论>` | `<证据指针>` |

- 未运行项：`<项目>`
- 原因：`<环境、依赖或范围原因>`
- 补验证方式：`<负责人、环境和触发条件>`

## 精确候选 Head

- 候选 exact head（完整 40 位）：`<git rev-parse HEAD 输出>`
- 工作树状态：`<clean / dirty；dirty 不能作为最终候选>`

## 独立 Review

- reviewer：`<与实现者独立的 Reviewer>`
- verdict：`<Reviewer 的真实结论>`
- exact head：`<Review 覆盖的完整 40 位 Head>`
- review ref：`<稳定 Review 文档或平台记录>`
- blocker 数：`<数字>`
- warning 数：`<数字>`

## 风险、兼容性与回滚

- 已知风险：`<风险>`
- 兼容性变化：`<可观察变化或无>`
- 回滚边界：`<可恢复提交、配置或数据边界>`

## 高风险操作声明

本 PR 不包含发布、生产、凭据和权限操作，也不授权合并、发布或生产变更。若实际范围包含任一高风险操作，必须从本 PR 剥离并建立独立、精确的授权流程。
