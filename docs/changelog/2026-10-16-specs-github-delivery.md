# S1–S3 GitHub 交付

2026-10-16 用户授权将 S1–S3 候选推送至 `wangjunbo866-xyf/ming-iot`，创建 PR、完成真实 CI 和独立复审，暂不合并。复用 `ming-iot-knowledge-pilot`；旧仓、私有备份和历史回执继续保留，小黄反馈任务保持废弃。

## 候选范围

| 阶段 | 保留的完整提交 | 内容 |
| --- | --- | --- |
| S1 | `56b3b811dbece915a787bd2decdbbe24279102f2` | 通知和 API 校验当前主规格、旧版本快照、团队约定与 Skill |
| S2 | `f08e6dd943b4ba9df9a26252080760b607d50f9f` | 完整 PR 差异与模块映射、多模块/轻量声明/历史归档检查及 CI |
| S3 | `c54d15b7572c5f7bf295ec18a17df6dfaf30a9bc` | 全新会话接手演练、通知调用边界说明和版本导航修正 |

交付分支 `codex/specs-s1-s3-delivery` 从 S3 派生，包含以上提交；创建时 GitHub `main` 为 `c165a56531f58421a870a0b8794f24556b60d3b2`。本次附加交付记录和当前授权范围，不修改产品行为。远程目标有变化时重新核对实际 merge-base 与完整 PR 差异。

历史阶段记录继续反映当时的本地验收和未运行项：[S1](2026-10-15-specs-current-baseline.md)、[S2](2026-10-16-specs-pr-check.md)、[S3](2026-10-16-specs-clean-clone-takeover.md)。历史 PASS 不自动成为最终候选的 PASS。

## 本次验收与证据归属

1. 最终提交与树保持可定位；远程 PR Head 必须与本地最终候选一致，完整 PR 声明覆盖基线至候选的全部路径。
2. 在该候选运行完整 `make verify-pr`、工作流静态校验、完整可达历史公开边界扫描及发布清单检查；保留失败及修复后的日志。
3. GitHub `Monorepo checks / verify` 与 `Specs PR check / specs-pr` 均须实际运行成功。保存 run、attempt、Head、PR 声明及完整差异指纹；正文修改后重新核对 Specs 检查，目标基线改变后重新验证适用范围。
4. 两位独立 AI 评审分别检查规范和规格，审阅全部主分支差异并绑定同一最终 Head；由实施者回读结论并处理发现，不能用启动评审替代结果。
5. PR 正文记录最终 Head、验证和复审摘要；详细回执由同一 Goal 的本次交付记录定位。此文定义验收口径，不提前声称尚未取得的远程结果，也不在提交中嵌入自身最终 SHA。

当前规范入口是 [团队开发约定](../runbook/team-development.md)，结构检查口径是 [完整 PR 差异检查](../../.agents/skills/ming-specs/references/pr-check.md)，公开范围是 [发布清单](../../tooling/policy/PUBLICATION_POLICY.json)。

## 结论边界

工具成功只证明结构、测试和扫描所覆盖的范围；业务语义仍由评审确认。S3 的无聊天历史 AI 接手不是真人交接；其路径隔离限制和采样限制保留原记录。服务端必需检查、CODEOWNERS、真实数据库/Kafka/设备、生产验收及长期团队收益均不由本次服务无关 CI 证明。

本轮完成条件是可评审、CI 已验证的开放 PR。合并、发布、删除旧源或备份仍不在本轮授权范围内。
