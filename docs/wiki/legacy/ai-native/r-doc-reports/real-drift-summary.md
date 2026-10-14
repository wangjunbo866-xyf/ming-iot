# 真实任务三：知识失效检测（2026-10-12）

本轮评测已完成，当前 v3 候选未通过既定 420 秒运行验收。两份恢复组报告的内容核验通过：能识别过时接口字段，也没有把明确绑定旧提交的 Spec 历史基线误判为现行错误；但运行器均超时 ERROR，不能提升为通过。无技能组同样正确完成失效检测，修复版本引用判据后产物验收通过，因此未证明候选带来准确性或效率增益。

## 输入与边界

- 最终候选本轮未改源码：`rdoc-pilot-maintenance-v3`，SKILL SHA-256 `05710de6c26c3c51883753db7f58e1da34c51644e8e5ab1392fb2086b984f5ba`；59 文件身份见 `current-candidate.json`。
- 真实产品提交：`a6b28035c577131c1a4b0262af6ddbdc393104c5` → `e38d162feba9445c3bedb6ec78d96ee79dee9a49`。文档来源提交 `b16030c10cf2565ecfa16ab19ee43a3a273bec57`。
- 使用完整真实 `backend/things/service/apisvr/http/swagger.json` Git blob 作为父仓 vendor 参考。旧快照 SHA-256 `f3bdeb9163899005cccd40552e8b014e31d1d6d69c4869b7eac5503b9da9d64b`；当前快照 `5ff4b7a0a6c2b150cab95b77412764e105af14721736288de2fe942f7d444e78`。
- stale 与 aligned 两种隔离条件的产品子仓都固定为 after；各有 2927 个不可修改输入文件。旧快照与新代码配对是受控历史回放，不是当前真实仓库缺陷声明。仅允许生成根目录 report.json。
- 固定提示 SHA-256 `9bdfbd0524fce19c357b94487d178a9f063ce363c7c46e22bb1d75aeb48d3ea8`；每例 420 秒，max_turns=24。skill-up 0.12.0、Codex CLI 0.154.0，保持原默认模型配置；实际模型型号未从运行器结果可靠解析，不作型号比较。

## 结果（原始状态全部保留）

| 条件 | 输入 | 原始状态 | 秒 | shell 命令数 | 内容核验 | 运行验收 |
|---|---|---|---:|---:|---|---|
| candidate-v3 | stale-snapshot | FAIL | 290.846 | 7 | 无报告 | 未通过 |
| candidate-v3 | aligned-snapshot | FAIL | 207.497 | 7 | 无报告 | 未通过 |
| upstream | stale-snapshot | FAIL | 328.451 | 7 | 无报告 | 未通过 |
| without-rdoc | stale-snapshot | FAIL | 397.075 | 17 | PASS | PASS（判据修复后） |
| candidate-v3-recovery | stale-snapshot | ERROR | 419.691 | 21 | PASS | 未通过 |
| candidate-v3-recovery | aligned-snapshot | ERROR | 419.614 | 49 | PASS | 未通过 |

首轮候选两例与上游一例缺少报告，轨迹和宿主诊断定位到 Codex 临时 `codex-linux-sandbox` 启动路径消失；不能把它们解释成语义检测错误，删除原因尚未确定。仅进行一次恢复批次：新 CLI 进程、串行运行、局部 PATH 包装器关闭嵌套 Agent，保留 sandbox、候选、提示、默认模型及 420 秒限制。恢复组不再观测到该启动错误，仍超时；不能把不同运行条件合并做性能排名。

原始判据把带有 before revision 的正确引用只与当前工作树比较，导致无技能报告 raw FAIL。第一版修复支持 Git blob，第二版又接纳绑定冻结来源 revision 的 vendor 引用。两版修复及原始结果均保留；最终版本的 8 项正反例自检通过，覆盖错误版本、未注明版本的旧引用、漏报冲突、历史误报、用历史代码替代当前必要引用等。仅按原始产物重新评分，没有为无技能组再调用模型。两个候选 ERROR 始终保持未通过。

## 已验证的内容与不足

`DeviceModuleVersionIndexReq`、`DeviceModuleVersionReadReq`、`DeviceModuleVersion` 的旧快照字段均为 `moduleCode: integer/int64`，当前源码为 `firmwareModuleCode: string`。失效条件建议重新同步快照；已同步条件建议保留。Spec 第3节明确锚定 before revision，两份恢复报告都正确保留该历史事实。2927 文件以及父子仓 HEAD/index 的内容检查通过，未冒领产品测试或全仓验收。

r-doc 确定性 audit 在两种快照条件下给出完全相同的 241 条既有发现，不能区分此次字段语义变化。存在、链接、格式和来源哈希检查不能替代跨版本语义核验；这需要读取实际字段与其适用版本。本例中 Agent 做到了，但无技能组也做到了。

恢复组无全库 audit，仍有 21/49 条完成命令，并输出大量核验内容后超时。观察支持“交付范围与结束条件仍有成本”这一待审查问题，不能仅凭单次超时断言 r-doc 导致低效。元数据不兼容与任务二重复审计的问题仍在。

## 交给综合审查的决定

本阶段的评测记录完成；候选运行验收为 **NOT_ACCEPTED**，不是任务三全绿。下一阶段应结合全部三个任务评估继续 fork 的收益与维护负担，独立核查最终源码差异，并补最终 v3 发现路径证据。只有明确的指令或实现改动、以及可解释的验证假设，才安排受影响用例重跑；不原样重复，也不单纯扩大时限获得通过。

限制：只有一个真实业务历史切片；提示已给出三个目标 schema 和证据需求；无技能/上游仅覆盖失效条件；未覆盖旧 revision 但语义未变、缺失源码、其他文档类型。未发现独立 mqtt-gateway 和前端代码仓，不能宣称三个产品仓全验证。此次人工核对由执行者完成，最终独立审查尚未完成。OpenSpec 仍为候选方案，未采用。

## 复查入口

- `real-drift-recovery-analysis.json`：全部六次运行、提示/结果/报告/轨迹指纹。
- `real-drift-recovery-regrade.json`：内容检查与运行接受条件分别记录，保留 FAIL/ERROR。
- `real-drift-v1/manual-review.json`：执行者内容核对，明确不是独立审查。
- `real-drift-v1/deterministic-comparison.json`：两条件 audit 完全相同。
- `real-drift-v1/runtime-diagnosis.json`、`real-drift-recovery-v1/manifest.json`：恢复假设与变更条件。
- `../real-eval-drift-v1/manifest.json`、`input-integrity.json`、`judge-repair-v2/manifest.json` 与 `selfcheck.json`：来源及判据绑定。
- `real-task-03-acceptance.json`：证据完整性验收和候选运行结论；二者不得混淆。
