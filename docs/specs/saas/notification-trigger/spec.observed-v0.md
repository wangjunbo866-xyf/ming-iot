---
status: observed
version: observed-v0
---

# 通知触发人展示：现状基线

本文件记录已核实行为，不表示这些行为已被逐条批准。来源代码 commit：`7d4063ef3c0407e87fdc010ea86960ed5b050368`；该能力在主分支 `6742201e9666fd3b3067216b2efbb3b8ba7338d7` 中内容相同。

<a id="display-name"></a>
## 展示名称与触发方式

`triggerUserDisplayName` 对输入做首尾空白清理，顺序为非空 Account → 非空 Nick → 正数 UserID 的十进制文本 → 空串。

- manual：有名称时“由 {名称} 手动触发”，无名称时“由用户手动触发”。
- auto：有非“系统自动”的名称时“由 {名称} 触发”，否则“系统自动触发”。
- deviceButton：专用文案“{名称}触发了{动作}”，无名称时保留动作文案；通过 ruleScene 入口还要求 UserID > 0 才选专用格式。
- 其他类型：有名称时“由 {名称} 触发”，否则不增加触发人一行。

现有 `TestFormatSceneButtonTriggerLine` 明确验证 Account 优先于 Nick。manual/auto 现有测试覆盖仅昵称与匿名输入，尚未覆盖同时存在账号和昵称的组合。

展示格式函数不写回输入。资料补全 `enrichTriggerUserParams` 与格式化是不同环节：缺少昵称时可能从用户资料回退取得账号；本基线不把字段名 Nick 等同于已验证的真人昵称。

业务调用证据：`appPush.go` 对消息正文调用 `PrependTriggerUserLine`；`sendMsg.go` 对 ruleScene 调用 `FormatRuleSceneNotifyLine` 或 `PrependRuleSceneTriggerLine`。本轮仅验证格式化路径，未验证真实推送通道。

候选变化见 [Patch](patches/PATCH-20261014-display-name/change.md) 与 [反向入口](README.md#display-name-patches)。当前无生效覆盖。
