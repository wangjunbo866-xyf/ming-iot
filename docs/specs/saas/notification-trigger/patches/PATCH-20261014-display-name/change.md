---
status: confirmed
revision: R2
---

# PATCH-20261014-display-name：触发人名称优先级

- 类型：patch；原因：候选需求调整。
- 任务：`todo_60ac9c3f4982`；状态和 revision 以文件开头元数据为准。
- 基线：[observed-v0#display-name](../../spec.md#display-name)。
- 反向入口：[模块候选](../../README.md#display-name-patches)。
- 适用代码：本地实现候选 `824849b041882c20dbb0a4acdee092f50ea9793a`；observed 来源仍为 `7d4063ef3c0407e87fdc010ea86960ed5b050368`，本 Patch 尚未 effective。
- 当前授权：用户允许完善约定、选择真实规则并试跑；Q1/Q2 已在当前会话直接回答，范围见 D2/D3。本地候选已实现，未合并或部署；实际检查见[试跑报告](../../../../../changelog/2026-10-14-ming-specs-handoff-pilot.md)。

## 认领

需求负责人：当前用户（姓名未核验）；长期模块负责人未分配。本次 AI 协调与资料执行者为 `ming-specs-draft-codex`；接收实施者为 `notification_rule_implementer`，独立评审者为 `ming_specs_contract_review`，版本与结论记入回执；没有真人同事参与接收。

本次能力为通知触发人的可见名称。无前台写入；后端写范围限 `triggerUserParams.go` 与 `triggerUserParams_test.go`，以及本模块资料和团队约定/Skill/试跑报告。读取 `appPush.go`、`sendMsg.go` 理解调用关系；不改变接口字段、数据库、身份鉴权、触发方式含义、通道或 Things 源码。可见工作树及当前 lane 未发现本路径的并行修改；不能据此证明全团队排他认领。

## 来源与澄清

用户 2026-10-14 原始指示：“先不引入完整 AIDLC，把需求澄清、决策留存和分角色交接补进现有团队约定与 ming-specs，再选一条真实业务规则，试跑一次需求调整和人员交接，验证收益和维护成本。”

事实：现有格式化函数及测试明确账号优先；名称函数被多种触发方式共用。AI 提议：为帮助接收者识别操作者，考虑优先展示昵称。R1 起草时此项尚未确认；后来通过 Q1 获得限定范围的真实回答，见 D2。

| 问题 ID | 选项与影响 | 真实回答 | 阻塞动作 |
| --- | --- | --- | --- |
| Q1 | 仅 manual 优先 Nick，其他类型不变；或所有类型统一优先 Nick；或保持现状并换规则 | 当前用户选择“仅手动触发优先昵称（推荐）”，2026-10-14 当前会话异步问答 | D2；无遗留业务选择 |
| Q2 | 独立新 AI 会话接手演练；或由用户指定真实同事接收 | 当前用户选择“独立 AI 子代理接收，先做演练（推荐）”，同批问答 | D3；真人交接未验证 |

## 决定与 revision

| ID | 决定者 / 来源 / 日期 | 内容 | 范围与状态 |
| --- | --- | --- | --- |
| D0 | 当前用户 / 本轮明确指示 / 2026-10-14 | 完善约定、选真实规则试跑、评估收益成本 | confirmed：任务范围；不代表选择具体名称优先级 |
| D1 | AI / 现状调查 / 2026-10-14 | 建议仅 manual 调整为昵称优先，以保持 deviceButton 现有明确契约 | proposed：等待 Q1；R1 |
| D2 | 当前用户 / Q1 直接回答 / 2026-10-14 | 仅 manual 昵称优先，其余触发方式保持原行为 | confirmed：R2 的 AC1–AC4；替代 D1 待选结论，不授权统一修改所有类型 |
| D3 | 当前用户 / Q2 直接回答 / 2026-10-14 | 由不继承聊天历史的独立 AI 子代理接收；使用 gpt-6-astra / Extra High | confirmed：本地交接演练；不代表真人接收或团队正式采纳 |

| revision | 行为与验收 | 决定适用性 |
| --- | --- | --- |
| R1 | 初始候选，下列 AC 待用户选择后锁定 | D0 覆盖试跑；D1 尚未确认 |
| R2 | 锁定仅 manual 改动的 AC1–AC4，auto/deviceButton/未知类型不变 | D0 继续有效，D2 确认业务选择，D3 确认交接方式；R1 仅保留历史 |

## R2 已确认行为与验收

manual 使用 trim(Nick) → trim(Account) → 正数 UserID → 匿名兜底；auto、deviceButton、未知类型均保持 observed-v0。

- AC1：manual 同时有昵称“张三”和账号时，输出“由 张三 手动触发”。
- AC2：manual 昵称为空/全空白时回退 trim(Account)，再回退正数 UserID；没有可用名称时仍为“由用户手动触发”。
- AC3：非 manual 仍账号优先，包括 deviceButton 专用格式与 auto 的现有输出。
- AC4：正文前缀、尾部旧格式兼容和避免重复前缀规则保持；不修改传入字段，不改存储与消息载荷。

以后若改为统一昵称优先，必须新写 revision 并重新确认 AC3；不得仅凭改了共享 helper 就倒改 AC。现状测试通过不替新的业务取舍背书。

## 技术方案与验证

按 D2 锁定行为及验收，再写回归用例。共用名称函数的其他调用必须保留现状；实现者自行选择最小实现。重点验证同时存在 Account/Nick、空白回退、ID/匿名、非 manual 不变及正文组装。

现状聚焦测试已运行：`go -C backend/saas test ./service/syssvr/internal/logic/notifymanage -run 'Test(FormatTriggerUserLine|FormatSceneButtonTriggerLine|CompactRuleSceneActionBody|PrependTriggerUserLine)$' -count=1`，PASS。

候选行为测试经过 RED/GREEN。首轮独立审阅发现 AC4 的真实旧账号首尾行未覆盖，保持 R2 需求不变，按原确认修复代码并补回归。聚焦整包测试与 vet 通过；完整 Profile、提交差异与独立复审的各版本结果集中在[试跑报告](../../../../../changelog/2026-10-14-ming-specs-handoff-pilot.md)，不复制另一份动态通过状态。真实推送通道：NOT_RUN。确认、实现、合并、部署分别记录。
