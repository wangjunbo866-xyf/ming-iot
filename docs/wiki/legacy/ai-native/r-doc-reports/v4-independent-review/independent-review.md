**实现审查：有两项可复现问题，当前候选不宜直接采用。** 已核对候选 64 个文件、16 份关键证据及其引用的 110 项证据指纹，均一致。旧 161 项回归仍绑定当前源码，本次未重跑。带标题链接的旧 P2 已修复；目录 profile 使用路径组件最长匹配，没有简单合并不同目录的状态。`exact_head`、身份和审批规则仍需项目校验器验证，compatibility PASS 不消除 103 项导航、6 项敏感内容基线。

- **P2：定点检查会越过受保护文件的读取边界。** [check_docs.py:34](/tmp/rdoc-v4-independent-review-2fq98jrc/candidate-v4-project/r-doc/scripts/check_docs.py:34) 调用的 [audit_docs.py:65](/tmp/rdoc-v4-independent-review-2fq98jrc/candidate-v4-project/r-doc/scripts/audit_docs.py:65) 对锚点目标直接读取，未应用排除项或受保护路径规则。纯内存复现：配置 `exclude: [private]`，只检查含 `[Target](private/.env.review.md#target)` 的 `safe.md`；工具仍读取目标并返回空 findings。影响是一次获准的安全文件检查可间接读取技能明确禁止打开的文件。应在读取目标前检查边界，并报告无法验证该锚点。
- **P2：部分无效配置导致异常，破坏结构化错误报告。** [config.py:148](/tmp/rdoc-v4-independent-review-2fq98jrc/candidate-v4-project/r-doc/scripts/rdoc/config.py:148) 对 `version: []` 执行集合成员判断，复现得到 `TypeError: unhashable type: 'list'`，定点检查无法输出 JSON 报告。它不会假通过，但会中断检查；这是 baseline 已存在、v4 仍继承的问题，应先检查类型并生成配置 finding。

结束契约已明确规定完成适用检查后结束，全库流程也已移入引用文件；但文字契约尚未得到稳定运行证据支持。父仓知识、子仓独立历史/PR/权限/发布，以及项目内容与通用工具分离的设计符合目标；目前仍是设计样板，不是迁移验收。

**证据审查：足以退回本候选，不足以判定哪条路线普遍更优。** [Kafka 验收报告](/tmp/rdoc-v4-independent-review-2fq98jrc/reports/v4-kafka/evaluation-acceptance.json)与原始结果一致：

| 处理 | 完整通过 | 保留产物通过 | 超时 |
|---|---:|---:|---:|
| 无技能 | 1/3 | 2/3 | 2/3 |
| v3 | 1/3 | 1/3 | 2/3 |
| v4 | 0/3 | 3/3 | 3/3 |

九次实际模型/effort 核验支持同条件描述，旧 ultra 结果不能混入速度比较。三组提示和约束一致，核心冻结指纹可追溯；case YAML 整文件摘要是事后补充，不能表述为事前完整冻结。强提示及“显式全库检查结果”措辞可能诱发全库审计；无技能一次长时间无输出根因未知；框架与会话 Token 存在缓存统计口径差异。因此，不能将审计次数、超时截断耗时或 Token 数直接解释为技能的因果效果。v4 的 3/3 产物通过也不能替代完整运行通过。

[旧六例复核](/tmp/rdoc-v4-independent-review-2fq98jrc/reports/v4-recheck/analysis.json)为 **4 PASS、1 FAIL、1 ERROR**；独立子仓场景超时且没有报告。对于已同步快照，我独立支持“字段命名歧义”的解释： [提示:19](/tmp/rdoc-v4-independent-review-2fq98jrc/real-eval-ota-v4/r-doc/evals/cases/aligned-snapshot.yaml:19)未定义 `code_field` 的命名层级；冻结判据要求 `firmwareModuleCode`，产物填写成员名 `FirmwareModuleCode`，同时另列正确的 `code_json_field`。原始源码轨迹和快照字段支持这种区分，三个 schema 的判据差异也仅在此字段。故该 FAIL **不能证明语义核验错误，也不能改判为 PASS**；它削弱了该用例评价业务语义能力的效度。

**结论：退回 v4 正式采用；路线建议继续一次有明确结束条件的小 fork 修补验证。** 这是当前工程决定，不是 fork 已被证明优于其他路线。先修复上述两项问题、补受影响回归；再预先明确 Go/JSON 字段契约，将定点维护与显式全库审计分成不同条件，保留原始失败，不靠单独加时消除失败。通过条件仍须同时包含正确产物、无越界及预算内正常结束。

现有证据尚未显示必须长期绕过大部分上游核心流程，暂不足以直接转向全面自建；可保留通用模块作为后续替换候选。无技能组并非自建路线试验，不能据此宣布自建胜出。