# LoopX Bootstrap 实施计划双语发布设计

> 日期：2026-08-23
>
> 状态：设计已口头通过，等待书面规格复核
>
> 范围：只调整 `2026-08-23-loopx-personal-framework-bootstrap` 实施计划的语言组织和翻译，不改变任何任务、命令、边界、验收或授权语义。

## 1. 目标

将当前英文实施计划发布为中文权威版和英文翻译版，使中文读者能够完整执行计划，同时避免两份文档形成相互冲突的双重权威。

## 2. 文件组织

采用三个文件：

| 文件 | 角色 |
|---|---|
| `docs/superpowers/plans/2026-08-23-loopx-personal-framework-bootstrap.md` | 稳定语言入口，只提供权威说明和语言链接 |
| `docs/superpowers/plans/2026-08-23-loopx-personal-framework-bootstrap.zh-CN.md` | 完整中文版，唯一权威实施计划 |
| `docs/superpowers/plans/2026-08-23-loopx-personal-framework-bootstrap.en.md` | 完整英文翻译版，发生冲突时服从中文版 |

已有链接继续指向稳定入口，不因语言文件拆分而失效。

## 3. 权威与翻译规则

1. 中文版是唯一权威版本；英文版必须在页首明确标注其翻译属性和冲突处理规则。
2. 当前计划的实施语义保持不变，不增加、删除、合并或重排 Task 和 Step。
3. 标题、说明和表格中的自然语言可以翻译；机器标识、文件路径、命令、JSON/YAML 字段、版本、摘要、Git revision、状态枚举和提交消息不得翻译或改写。
4. 两份完整版本必须保留 Task 1–10、checkbox、文件清单、用户 Gate、提交授权和“第一阶段尚未完成”的边界。
5. fenced code block 内容逐字一致。若需要解释其中的英文或中文，在代码块外补充翻译，不修改可复制内容。
6. 英文版不得通过简写省略中文版的条件、失败路径、预期结果或安全限制。

## 4. 一致性校验

发布前执行确定性检查：

- 稳定入口中的两个相对链接均可解析；
- 中文版标记 `Canonical: zh-CN`，英文版标记 `Translation of: zh-CN canonical`；
- 两份完整版本都恰好包含 Task 1–10，顺序一致；
- Task 和 Step checkbox 数量一致；
- fenced code block 数量、语言标签和内容逐字一致；
- 以下关键标识在两份版本中均存在且数量一致：
  - `ming-iot-framework-v1`
  - `BOOTSTRAP_QUALIFIED`
  - `phaseOneComplete: false`
  - `WAITING_USER_SELECTION`
  - `403b9a6d6b7e2c60a1ff96ef0f681da272d04c36`
- 两份文档均不存在 `TODO`、`TBD` 或省略号占位步骤；
- `git diff --check` 通过。

校验只证明结构和机器内容对应；翻译语义仍需人工通读中文权威版和英文翻译版。

## 5. 非目标

- 不改变 LoopX 架构设计；
- 不执行 Bootstrap 实施计划；
- 不提交、合并或 push；
- 不建立自动翻译系统或长期双向同步服务；
- 不把中英文两份都声明为权威来源。

## 6. 验收

满足以下条件才算双语发布完成：

1. 三个文件均存在，稳定入口链接正确；
2. 中文版完整、可独立执行且被明确标记为权威；
3. 英文版完整、可独立阅读且明确服从中文版；
4. 结构、代码块和关键机器标识通过一致性检查；
5. Git 变更仅包含本规格和三个计划文件；
6. 未执行 `git commit` 或 `git push`。
