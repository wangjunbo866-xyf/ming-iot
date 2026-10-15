# ming-specs 第一次真实 Patch 试跑

- 日期：2026-10-13；任务：`todo_3af9f9234500`。
- 范围：[模块认领、来源与 AC](../specs/engineering/api-contract/patches/PATCH-20261013-lint-selection/change.md)。
- 比较基线：`da80f056941e5f5e2ea1de38538ea2a062be8853`（0.1.0 草案）。
- 分支：`codex/team-specs-draft`；实现与资料在同一独立工作树。
- 交付边界：本地候选试跑；未合并、未发布，未将草案设为全团队制度。
- 状态：本地试跑已形成候选，完整门禁发现基线失败，独立检查按不可变候选留回执；最终候选完整 SHA、文件指纹、测试及独立评审回执由仓库主工作区的 `.local/materials/ming-specs-pilot-final.json` 绑定。该外部回执不进入本次自引用提交；本文件中的验证摘要可随 Git 候选一起分发。

## 真实问题及结果

旧版实际运行：

```bash
go -C backend/tools run ./cmd/apicheck lint --backend .. \
  --files service/apisvr/http/system/not-a-real-file.api
```

得到 `lint: ok (1 files)`、退出 0。另有空白列表误报检查 0 文件成功、重复的规范路径计数不一致。修复仅增加 lint 文件选择校验；保留已有元数据、golden 和业务语义。

新增回归先在旧实现运行，七个选择场景均出现预期失败；随后修复，`TestCLILint*` 通过。实际项目目录验证正常/异常选择；隔离 SaaS/Core 小样本验证规范化后仍能检出元数据缺失，以及成功/失败都不改写输入。

## 三项协作验证

| 验证 | 实际操作与观察 | 能证明 / 不能证明 |
| --- | --- | --- |
| 模块认领 | 按工具能力登记 engineering.api-contract，明确代码两文件、文档/Skill 范围、无前台/业务 API、无共享数据改动；检查可见 worktree 与在途 Todo | 本次委托及范围可交接；不能证明全团队认领排他或长期负责人已分配 |
| Patch 双向指针 | Patch 指向 observed-v0#lint-select，模块入口及主规格回链；检查器读取候选中的链接、锚点和版本 | 显式给定的 Patch 可往返定位；不能证明全库覆盖无冲突 |
| 提交差异提醒 | 固定暂存 tree `1042aaf15076677948742344c158f0bd9f5440af`，发现 Patch/测试已暂存而实现未暂存；报告 main.go/main_test.go 位于候选外，重暂存后复核 | 真正区分暂存区和工作区；不能据此声称已安装 Git Hook 或 CI |

暂存区初检的结构结果为 `STRUCTURE_OK_REVIEW_REQUIRED`。其语义结果是**发现差异**：AC1/AC2/AC3 在当时的待提交实现中未满足；工作区测试后来成功不能为这个旧 tree 背书。处理是按已授权 Patch 补实现、测试并重新暂存，不反向改写 AC 来接受漏检。

## 差异清单与路径覆盖

| ID / 条款 | 观察 | 处理及依据 | 复验 |
| --- | --- | --- | --- |
| D1 / AC1、AC2 | 旧 CLI 对不存在、空白、混合选择成功 | 当前任务授权的小修复；拒绝无实际覆盖的选择 | TestCLILintFileSelection |
| D2 / AC3、AC4 | 未规范化的 ./ 路径与诊断 key 不一致，重复项计数错误 | 同一选择层修复；统一规范化、范围核对及诊断过滤 | 真实路径用例与元数据问题小样本 |
| D3 / 候选边界 | 当时暂存候选缺实现，工作区已有修复 | 重新暂存并对新 tree 检查；保留旧报告作负面证据 | 最终 preflight 与独立 Review |
| R1 / 独立 Review | 初版检查固定历史提交时，未提示后来已暂存或已提交的变更；旧正文读取正确，边界提示不完整 | 增加 candidate 对 index/worktree/HEAD 的显式比较与两个反例 | 修复后 9 项结构回归；新候选复审 |

候选中所有路径的机器列表见 preflight 回执。语义归属如下：

| 路径 | 归属 / 不改变业务规格的理由 |
| --- | --- |
| backend/tools/cmd/apicheck/main.go、main_test.go | 本 Patch AC1–AC5 的实现与回归 |
| docs/specs/engineering/api-contract/** | 最小现状、认领与候选 Patch；仅本工具接入 |
| docs/specs/README.md | 登记该模块及真实采纳状态 |
| docs/runbook/team-development.md | 试跑所得治理约定修订；不改变业务行为 |
| .agents/skills/ming-specs/** | 执行流程、有限的手动检查器及隔离回归；不改变业务契约 |
| docs/meta/DOC_REGISTRY.yaml、本报告 | 正文映射和证据导航；不改变业务契约 |

## 验证与评审

| 检查 | 当前结果 |
| --- | --- |
| 旧实现回归 | EXPECTED_FAIL：7 个真实 CLI 选择用例复现问题 |
| 修复后 TestCLILint* | PASS：真实项目路径及隔离问题文件均验证 |
| preflight 隔离反例 | PASS：独立 Review 后增加历史候选与当前索引/HEAD 差异的两个反例，共 9 项；结果绑定见固定候选回执 |
| make verify-pr | FAIL（exit 2）：构建完成，测试阶段在 thingsbaseline 的两个既有测试失败；不能标记整体 PASS |
| 独立固定候选 Review / 新会话交接 | 初版 5bb2c650 已发现 R1 并修订；新会话已找到认领、基线、指针和证据，最终候选复审见回执 |
| make vet contract-static boundary-scan | PASS（exit 0）：补跑完整 Profile 在测试失败后未到达的阶段；不改变原 verify-pr 的 FAIL |
| skill-up Engine 批量评测 | NOT_RUN：无该引擎 API 凭据；本次使用真实项目执行和宿主独立检查 |
| 新规格 Hook、CI、服务端门禁 | NOT_CONFIGURED |
| 生产/集成服务验证 | 本工具选择层修复不需要启动外部服务；未执行部署 |

原始命令日志及机器回执保留在项目主工作区 `.local/materials/ming-specs-pilot/`。结构工具成功不等于语义符合需求；自然语言自评也不等于 CI 或人工审批。

### B1：独立基线缺陷记录（阻止合并，不阻止完成本次流程试跑）

`TestExecVerificationGateRunnerPinsMakeToAbsoluteGoDespitePATH` 和 `TestExecVerificationGateRunnerBuildPassesWithRestrictiveUmask` 在 Go 构建阶段报 `error obtaining VCS status: exit status 128`。主分支 `6742201e9666fd3b3067216b2efbb3b8ba7338d7` 上运行同名两测试也失败，确认无需本次 apicheck 修复即可复现；根因尚未确认。

复现：`go -C backend/tools test ./internal/thingsbaseline -run '^TestExecVerificationGateRunner(PinsMakeToAbsoluteGoDespitePATH|BuildPassesWithRestrictiveUmask)$' -count=1`。独立缺陷草稿保存在本地 Evidence 的 `baseline-defect.md`，未发布远端 Issue。本次未修改这些测试、没有使用 `-buildvcs=false`、没有缩小 verify-pr 以获得绿色。接续处理需要单独认领基线工具范围。

Patch 保持 confirmed，不能进入 effective/absorbed 或宣称达到合并条件；负责人、CI 和最终提交的评审仍按真实交付阶段处理。

## 对草案的实际修订

团队约定及 Skill 升为 **0.1.1-draft**：

1. 补“已有模块首次接入”：小范围 observed 基线、源码来源、缺陷与确认分开，避免为启动试跑虚构完整 Feature。
2. 区分任务级委托与逐条人工签字；记录临时范围责任，保留长期负责人的缺口。
3. 明确暂存 tree 与工作区分离；结构检查读 Git 对象并输出候选/输入指纹。
4. 增加有限的手动 preflight 和反例，明确退出 0 后仍需语义检查；不把局部试跑说成自动门禁上线。
5. 独立 Review 发现历史提交的覆盖提示遗漏，补当前索引/工作区/HEAD 三方差异；工具不自行断言未核验的 Hook/CI 配置。

首次接入包含一次性的最小基线和检查工具建设。本次后续同模块 Patch 可以复用这些材料，主要维护一份 change.md 和小型 check.json 投影；无需每次重建 Feature、设计和整套团队文档。机器投影仍有手工重复维护成本，本次未证明跨真人团队的长期成本收益。

下一步采纳应以这次实际交接与维护成本为依据；本轮不自动扩展到全部模块、安装团队 Hook 或迁移旧文档权威。
