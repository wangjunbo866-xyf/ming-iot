---
status: local-candidate
version: s1-1
date: 2026-10-15
---

# S1：当前主规格与新仓验证基线

## 范围、认领与决定

Goal：`ming-iot-knowledge-pilot`；任务：`todo_29298f77e9c377b0e10dc1e1`；执行者：`rdoc-pilot-codex`。当前用户授权收敛通知/API 校验主规格、建立新仓验证基线、同步修订约定与 Skill 并独立复审。长期模块负责人仍未分配；临时实施身份不补写为长期负责人。

分支 `codex/specs-current-baseline` 从规划候选 `f1e7453858b8a9ac1884cf44e5a7e302bb0bdee7` 开始。实际修改范围为两个模块文档、specs 地图、团队约定/提纲、ming-specs 说明与模板、资料索引及本记录。此任务是既有规则收敛与流程修订，不新增业务 Patch，不修改产品代码、测试、CI 或检查器。

| 决定 | 来源与范围 |
| --- | --- |
| 收敛现有两条规则，不重新取舍业务 | 当前用户 S1 指令；通知沿用原 R2/D2，API 沿用原任务级修复授权 |
| 合并前在候选内整理完整主规格 | 用户“同一功能持续演进”目标及已维护提纲，本轮授权同步修订 |
| 普通接手从代码与一致文档开始；快照按需 | 用户对交接包的质疑及提纲；不恢复小黄反馈任务 |
| 不启用 S2 实际 PR 规格门禁 | 本轮限定 S1；[后续规划](2026-10-15-post-migration-governance-plan.md)保留为待推进范围 |

旧仓、私有备份和旧回执继续保留；不重建 Goal。候选尚未远程交付或合并，本文不扩大为发布授权。

## 现行知识与历史的分工

- [通知 current-v1](../specs/saas/notification-trigger/spec.md)：完整名称优先级、场景按钮、正文兼容和边界；仅 manual 昵称优先。
- [API 校验 current-v1](../specs/engineering/api-contract/spec.md)：完整文件选择、元数据和只读退出规则。
- 两份旧 `spec.md` 按字节保存为各模块 `spec.observed-v0.md`；旧文中的“当前/候选”仅指当时。模块 README 是当前路由，历史不参与默认当前规则叠加。
- 原 Patch 标 absorbed，保留原决定和验收；check.json 更新为 current-v1 指针，API 投影升级 v2 以避免状态重复。旧任务号/写范围不扩大，不把历史认领用作 S1 全范围授权。
- [约定](../runbook/team-development.md)与 [Skill](../../.agents/skills/ming-specs/SKILL.md)修订为 0.4.0-draft。轻量修复复用原任务；完整主规格随候选评审；普通接手不强制独立包；缺陷反馈修复唯一正文和相关验证。

## 新仓验证基线

适用实现基线：`c165a56531f58421a870a0b8794f24556b60d3b2`（新仓 main）；本轮正文/Skill 差异基线：`f1e7453858b8a9ac1884cf44e5a7e302bb0bdee7`。不要拿旧仓 SHA 作为新仓命令的默认基线。完整候选 SHA 及测试/独立复审由 Goal 中本任务外部回执固定，避免本文自引用其所在提交。

从候选仓根执行：

```bash
make knowledge-check
go -C backend/saas test ./service/syssvr/internal/logic/notifymanage -count=1
go -C backend/tools test ./cmd/apicheck -count=1
git diff --check f1e7453858b8a9ac1884cf44e5a7e302bb0bdee7
```

`knowledge-check` 运行维护入口检查、团队工具回归和 preflight 自身回归；其当前固定清单不等于自动遍历全部新规格。因此本轮另核对所有修改 Markdown 的相对链接/显式锚点、版本指针、旧快照字节、未改写历史回执及真实完整 diff 范围。Skill 元数据另按技能校验器检查。

两份 absorbed Patch 可在固定候选上用 `--base <候选> --head <候选>` 核验历史到当前的结构指针；这是零差异结构检查，**不是 S1 全范围 preflight 或语义验收**。本轮跨模块差异没有单 Patch 投影，实际全差异由路径核对与独立复审覆盖；不缩小真实范围或放宽旧认领换取 PASS。

## 验证与评审结果

本地聚焦验证已完成：

| 检查 | 结果与边界 |
| --- | --- |
| knowledge-check | PASS；维护清单通过，工具测试 8 PASS / 1 SKIP（未安装 gitleaks），preflight 回归 25 PASS |
| 通知格式化整包 | PASS；不访问真实推送渠道 |
| apicheck CLI 整包 | PASS；包含编译 CLI 和真实树选择用例 |
| Skill 元数据与全部本次 Markdown 链接 | PASS；元数据合法、90 个相对链接及锚点可达，不等于宿主加载或业务验收 |
| 历史与边界 | 两份 observed-v0 与原 spec 字节一致；业务代码、检查器、CI、旧 handoff 与旧 changelog 未修改；19 个变更文件均在 S1 范围 |

上述记录对应本次内容，最终精确 Head、结构指针核验与独立复审由 Goal 本任务外部回执固定；启动审阅或本节通过文字不代替最终审阅回执。独立复审核对条款与实现/测试，以及不依赖聊天的仓库接手能力，不代写真人接收结果。全平台 verify-pr、托管 CI、生产/真实依赖在本轮文档修订中未重跑，不能继承旧运行结论冒充本轮 PASS。

## 限制与下一步

- 这是两项局部能力的当前知识基线，不是完整通知领域、全平台 API 或全部文档已治理。
- 真实数据库、设备、推送渠道、生产环境、真人验收与长期成本收益未在 S1 验证。
- 已有 CI 运行检查器回归，实际 PR base/head + 模块/条款映射尚未接入；远程合并规则未回读。S2 再处理实际差异覆盖与负例，不能把本轮手动 PASS 当自动门禁。
- 下一阶段仍须验证持续变更与新上下文接手，确认收益后再扩域；不因为有 Skill 就强制所有开发方法统一。
- 小黄反馈及依赖任务保持废弃，旧仓和备份保留。
