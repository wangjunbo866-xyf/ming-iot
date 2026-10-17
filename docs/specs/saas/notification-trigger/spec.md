---
status: effective
version: current-v1
source_revision: c165a56531f58421a870a0b8794f24556b60d3b2
---

# 通知触发人展示：当前主规格

本规格覆盖通知触发人格式化及正文组装，不涵盖整个通知领域。适用代码是上方新仓 main 基线；本轮只收敛文档，不改变业务行为。手动昵称优先沿用 [R2 的 D2 与 AC1–AC4](patches/PATCH-20261014-display-name/change.md)；其余行为由既有实现与测试核实，不扩展为新的业务承诺。旧现状原文保留在 [observed-v0 快照](spec.observed-v0.md)，仅用于追溯。

`effective` 限上述代码版本的格式化行为；文档候选、验证与最终评审另见 [S1 记录](../../../changelog/2026-10-15-specs-current-baseline.md)，不代表生产部署或真人验收。

<a id="display-name"></a>
## DISPLAY-NAME：名称与触发方式

名称先清除首尾空白。正数 UserID 回退为十进制文本，零/负数不作为名称。

| 格式化入口 / 类型 | 名称优先级 | 输出 |
| --- | --- | --- |
| `FormatTriggerUserLine` / manual | Nick → Account → 正数 UserID → 空 | 有名称：`由 {名称} 手动触发`；无名称：`由用户手动触发` |
| 同入口 / auto | Account → Nick → 正数 UserID → 空 | 非空且不等于 `系统自动`：`由 {名称} 触发`；否则 `系统自动触发` |
| 同入口 / deviceButton、未知或空类型 | Account → Nick → 正数 UserID → 空 | 有名称：`由 {名称} 触发`；无名称：空串 |

只有 manual 改为昵称优先。直接使用通用入口处理 deviceButton 不会自动选择专用动作格式。

<a id="scene-button"></a>
## SCENE-BUTTON：场景动作格式

`FormatSceneButtonTriggerLine` 使用 Account → Nick → 正数 UserID，清理动作首尾空白；有名称时输出 `{名称}触发了{动作}`，无名称时输出 `触发了{动作}`，动作为空则回退通用入口。

`FormatRuleSceneNotifyLine` 仅在 Type=deviceButton 且 UserID>0 时选择上述专用入口；其他输入走通用入口。动作取正文中清理空白后的非空行，以 Unicode 字符数最长的一行为准，相同长度取首个；不把“信息最完整”作为可额外推断的规则。直接调用 `PrependRuleSceneTriggerLine` 时，manual 复用下述手动兼容处理，其他类型使用场景入口。发送流程是否调用该函数见 [调用边界](#call-sites)。

<a id="body-compatibility"></a>
## BODY-COMPATIBILITY：正文与旧格式

`PrependTriggerUserLine` 和兼容别名 `AppendTriggerUserLine` 都把触发人行放在正文前；直接调用 `PrependRuleSceneTriggerLine` 处理 manual 时也沿用它。这些是辅助函数的规则，不表示发送流程会为所有 manual 通知添加前缀。

- manual 有非空 Account，且旧账号行 `由 {trim(Account)} 手动触发` 与当前行不同时，清理正文首尾空白后，仅移除匹配的首部完整行、尾部完整行或唯一整行，再组装新行。非 manual 不执行此清理。
- 不清理正文中间的同文行、部分匹配或其他账号的行；不是任意全文替换，也不是循环清理所有重复历史行。
- 通用组装：触发人行为空时原样返回正文；否则清理正文首尾空白。正文已经等于当前行或以当前完整行开头时保留，不重复添加。否则将尾部匹配的完整当前行移到前面；空正文返回当前行，其余返回 `当前行 + 换行 + 正文`。

以上兼容条件使用旧版实际产生的账号行验证，不能用新的昵称行冒充旧输入。

<a id="call-sites"></a>
## CALL-SITES：已观察到的调用边界

以下仅记录适用源码版本的调用条件，帮助判断上述规则何时执行；不扩展为整个通知领域的业务承诺。

| 调用位置 / 条件 | 当前处理 |
| --- | --- |
| [SendNotifyMsg](../../../../backend/saas/service/syssvr/internal/logic/notifymanage/sendMsg.go) / ruleScene 且 manual | 模板渲染后跳过触发人前缀组装，保留渲染后的正文；不调用 `PrependRuleSceneTriggerLine` |
| 同位置 / ruleScene、deviceButton 且系统通知 | 以 `FormatRuleSceneNotifyLine` 的单行输出替换正文；函数内部仍按 UserID 判断是否用场景动作格式 |
| 同位置 / ruleScene、其他非空且非 manual 类型 | 以 `PrependRuleSceneTriggerLine` 组装正文 |
| 同位置 / 非 ruleScene 或空触发类型 | 跳过该 ruleScene 组装分支；类型判断使用资料补全后的字段 |
| [pushAppNotify](../../../../backend/saas/service/syssvr/internal/logic/notifymanage/appPush.go) / ruleScene | 跳过触发人前缀组装，不会为 manual 场景通知补加一行 |
| 同位置 / 非 ruleScene | 到达正文构造处时，通用格式化行非空才调用 `PrependTriggerUserLine` |

因此，“manual 昵称优先”限定在实际调用格式化函数的路径，不能据此修改发送层使所有手动场景通知新增昵称行。`pushAppNotify` 在正文构造前还会查询客户端，可能提前返回；表中不是推送成功保证。此节由源码核对，现有辅助函数单测不构成上述发送分支、数据库或真实通道的集成验证。

<a id="boundary"></a>
## BOUNDARY：边界及证据强度

格式化函数按值读取 `triggerUserFields`，不改写传入字段；`enrichTriggerUserParams` 是另一环节，可能查询资料并写回参数。Nick 可能来自用户名/账号回退，不保证是经过核验的真人昵称。此轮未修改身份、数据库、消息载荷、推送通道、Things 或前端。

实现与聚焦测试位于：

- [triggerUserParams.go](../../../../backend/saas/service/syssvr/internal/logic/notifymanage/triggerUserParams.go)
- [triggerUserParams_test.go](../../../../backend/saas/service/syssvr/internal/logic/notifymanage/triggerUserParams_test.go)
- 调用线索：[appPush.go](../../../../backend/saas/service/syssvr/internal/logic/notifymanage/appPush.go)、[sendMsg.go](../../../../backend/saas/service/syssvr/internal/logic/notifymanage/sendMsg.go)。调用存在不证明真实渠道运行成功。

## 验收与验证入口

从当前新仓根运行（无需历史仓 SHA）：

```bash
go -C backend/saas test ./service/syssvr/internal/logic/notifymanage -count=1
```

| 条款 | 现有验证 |
| --- | --- |
| DISPLAY-NAME | `TestFormatTriggerUserLine*`：同时有昵称/账号、空白、ID、匿名、非 manual 账号优先 |
| SCENE-BUTTON | `TestFormatSceneButtonTriggerLine`、`TestCompactRuleSceneActionBody`、`TestCompactRuleSceneActionBodyCountsRunesNotBytes`、`TestCompactRuleSceneActionBodyEqualRunesKeepsFirst`、`TestRuleSceneNotifyLinePicksRuneLongestAction`、`TestRuleSceneTriggerKeepsDeviceButtonBehavior`：账号优先的动作格式；多行取 Unicode 字符数最长的非空行且区别于按字节比较；同字符数取首行（含空白行与首尾空白清理用例）；deviceButton 入口级选取与非正 UserID 回退 |
| BODY-COMPATIBILITY | `TestPrependTriggerUserLine`、`TestManualTriggerBodyCompatibility`、`TestNonManualTriggerBodyPreservesManualAccountLine` |
| BOUNDARY | 格式化函数值参数由源码核对；不将此核对冒充数据库或输入不变的专门运行测试 |

测试证明所含用例及包级回归，不证明所有组合。真实数据库、设备、推送渠道和生产部署未在 S1 验证。人的长期模块负责人仍未分配；本次范围授权见 [模块入口](README.md)。

## 变更来源

[PATCH-20261014-display-name](patches/PATCH-20261014-display-name/change.md) 的 R2 已吸收到 current-v1 的 DISPLAY-NAME、BODY-COMPATIBILITY、BOUNDARY。主规格是当前入口，Patch 保存来源、取舍和原回执；[历史导航](README.md#display-name-patches) 保留反向指针。
