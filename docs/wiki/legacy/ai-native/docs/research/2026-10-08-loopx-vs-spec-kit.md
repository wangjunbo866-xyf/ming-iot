# 长程任务控制：LoopX 与 Spec Kit

核查日期：2026-10-08。比较 **Spec Kit v1.1.2** 与本地 **LoopX 1.3.1** 安装及源码 `fc25014937ebdf90a8519825049f34f861d7d3fd`。依据文档、源码和安装元数据进行静态核查；没有安装或升级工具，没有创建 Goal/自动化，没有开展进程崩溃、并发恢复或外部写入故障试验。既有 map-delivery、项目授权与精确版本证据规则继续成立。

## Spec Kit 的定位与已确认机制

Spec Kit 的 workflow runner 执行显式 YAML 流程，可包含 Agent command/prompt、shell、gate、分支、循环和 fan-out/fan-in。它确实具有运行状态及恢复能力，不能只归类为提示词模板；但这些能力也不能直接推导为完整的长程目标控制面。[发布版工作流说明](https://github.com/github/spec-kit/blob/v1.1.2/docs/reference/workflows.md)、[内置步骤注册](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/__init__.py)。

| 比较维度 | v1.1.2 可确认事实 | 长程任务中的实际边界 |
| --- | --- | --- |
| 持久化 | run 目录保存 workflow.yml 副本、输入、步骤结果、当前状态及日志 | 保存的是流程状态；不是代码工作树、外部系统或模型会话的事务快照 |
| 恢复入口 | `resume` 读取已存定义、结果与输入，允许覆盖输入；仅接受 paused/failed | completed/aborted/running 不由这个入口直接恢复；恢复动作本身不重新定义业务目标 |
| 恢复粒度 | 从当前顶层 `current_step_index` 重新执行 | 嵌套 body 暂停/失败后，重新执行父步骤及其 body；不是任意子步骤的精确断点恢复 |
| 失败处理 | 普通步骤失败会停下；可显式配置 `continue_on_error: true` 供后续分支处理 | 不能把所有 workflow 都称为“失败必阻塞”；具体路径由 YAML 决定 |
| 循环与限额 | while/do-while 有 max_iterations，未配置时内核使用 10；shell 默认 timeout 为 300 秒，可配置 | 循环次数和单步骤超时不等于模型配额、跨轮总费用预算或配额耗尽后的唤醒恢复 |
| 并行 | fan-out 的 max_concurrency 默认串行；大于 1 使用有界线程池 | 同进程并行，不自动给每项建立独立代码工作树或身份；写冲突需要作者处理 |
| 审批 | gate 读取 TTY 选择，或由 verdict_input 自动作出选择；记录 output.choice | 没有从该实现证明真人身份、独立 Reviewer 身份或对产物摘要的批准绑定 |

表格依据：[engine.py](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/engine.py)、[shell 步骤](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/step/shell/__init__.py)、[while 步骤](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/step/while_loop/__init__.py)、[gate 步骤](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/step/gate/__init__.py)。

## 恢复、重跑和外部动作

对前一份选型记录中依据文档概述的恢复能力作进一步限定：若把它理解为“所有层级均精确恢复”，需要更正。以下源码核查表明，精确定位只到当前顶层步骤，执行本身采用重跑语义。

`execute` 在启动时保存 workflow.yml；`resume` 优先读取这份定义，合并新输入，再从当前顶层步骤重跑。engine 第 1374–1379 行明确说明：嵌套步骤不更新顶层索引，恢复会重复父步骤与内部流程，精确嵌套 step-path 是后续增强项。因此文档的恢复概述不能解读成每个子步骤都只执行一次。[恢复代码](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/engine.py#L1112)、[嵌套恢复边界](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/engine.py#L1374)。

每一步先存当前位置，再调用步骤实现，随后记录结果。shell 直接执行本地命令。**静态推论**：如果创建 Issue、发消息或部署已完成，而进程在记录结果前失败，重跑可能重复动作；嵌套恢复还可能再次执行此前成功的兄弟步骤。本次未看到这些通用步骤为外部平台建立幂等键、事务提交或副作用补偿，不能承诺 exactly-once。[执行顺序](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/engine.py#L1238)、[shell 调用](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/step/shell/__init__.py#L55)。

有副作用的步骤应由项目适配器负责：固定动作身份和候选摘要、写前查询、平台幂等键或条件更新、写后回读、结果持久化；未知结果先核对再重试。只是把命令放到 shell 后面，不能完成这些保证。这是本地设计建议，不是已存在的 Spec Kit 或 LoopX 接入。

键盘中断有捕获路径，可把 run 记为 paused。**静态推论**：强制终止或掉电可能留下 running，而 `resume` 的状态检查会拒绝它；核读入口未见自动识别失活执行者并接管的实现。没有实测该故障，也不排除外层工具或扩展提供恢复办法。[execute/resume](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/engine.py#L1092)、[CLI resume](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/command_resume.py)。

## 并发、批准与长期自治的证据边界

RunState 使用 `threading.Lock` 保护进程内数据更新，JSON 采用临时文件加 `os.replace` 写入；这能减少同一次运行内的线程写入竞争，但不是跨进程 lease，也没有证明两个调用者同时 resume 同一 run 的互斥与故障接管。[RunState](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/engine.py#L667)。

fan-out 停止时停止提交新项并取消未开始项；已运行项仍可结束，最终返回结果按输入顺序处理。取消排队任务不意味着撤销已发生的写入；这尤其影响多个 Agent 同时改同一仓库或写同一外部记录的设计。[fan-out 内核](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/engine.py#L1471)。

gate 的 verdict_input 非空匹配值可直接决定结果，非空输入 default 也会自动放行；为空时 TTY 询问，非 TTY 暂停。`on_reject` 支持 abort、retry 或 skip；retry 只是暂停等待再次执行 gate，不等于自动回到修复步骤。掌握运行输入的人或程序可以给 approve，源码没有把该值鉴定为授权人的签名。[GateStep](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/workflows/step/gate/__init__.py#L141)、[Gate Verdict Inputs](https://github.com/github/spec-kit/blob/v1.1.2/docs/reference/workflows.md#gate-verdict-inputs)。

官方文档明确 shell 使用调用者权限，没有 capability sandbox；`requires` 不构成权限门禁。Codex 集成会调用 `codex exec`，但 workflow resume 本身只恢复流程位置，不能据此声称恢复了 Agent 内部会话、模型预算和目标规划状态。[Codex 适配](https://github.com/github/spec-kit/blob/v1.1.2/src/specify_cli/integrations/codex/__init__.py)、[运行器权限说明](https://github.com/github/spec-kit/blob/v1.1.2/docs/reference/workflows.md#step-types)。

| 待比较的长程能力 | 本次 Spec Kit 证据范围 |
| --- | --- |
| 定时唤醒、持久队列、常驻扫描器 | 在核读的内置步骤注册、engine 和 resume 入口中未见；不能扩写为整个生态绝对不支持 |
| API 配额识别、耗尽后等待到重置时间 | 已确认单步骤 timeout/循环次数；未核实内置配额感知调度 |
| lease、心跳、执行者失活自动接管 | 已确认进程内线程锁；未核实跨执行者 lease 与自动接管 |
| 跨会话 Goal、动态选择下一项、重规划 | 已确认持久流程定义、结果和输入分支；未核实内置长期目标优先级与自主重规划控制面 |
| 独立身份审批与 exact-candidate 授权 | gate 记录选择；需要另加身份、摘要、有效期和服务器验证，不能用 approve 字符串替代 |

这些“未核实”只限于上述固定版本及文件范围。可以由外层控制器、定制步骤或 shell 接入实现的能力，与产品内置并经过本机验证的能力，应分别记录。

## 与现有长程控制器组合的候选方式

以下是待验证的架构建议：已有长程控制器可以继续拥有 Goal、优先级、唤醒与接管；Spec Kit runner 只执行一个范围固定的阶段包并交回状态和证据。避免两边同时拥有“唯一下一步”和同一份完成状态。

阶段包至少引用现有文档仓的批准来源、候选版本、允许动作、验证命令和停止条件；runner 的 completed 只表示阶段流程完成。交付生效仍由 map-delivery 和项目 CI 根据真实 Review/Evidence、逐仓 base/exact_head 判断。若现有控制器已经可靠覆盖这些阶段，增加 runner 需要证明额外收益，不能只为换一套 YAML。

## LoopX 本机核查

本机入口位于 `/root/.local/bin/loopx`，当前 shell 的 PATH 未发现该入口，但文件存在；本次只读包元数据确认安装版本为 1.3.1。源码仓位于 `/home/wangjunbo/workspace-skills/loopx`，HEAD 为 `fc25014937ebdf90a8519825049f34f861d7d3fd`，读取时工作树干净。没有因此调整 PATH、重装或运行项目生命周期命令。

对已安装包与该 checkout 的四个文件做字节摘要比较：`turn_driver/driver.py`、`executor.py`、`settlement.ts`、`todos/completion_transaction.py` 均相同。它把本次部分源码观察关联到本机安装，但不证明整包、App、后台服务和实际项目都使用相同版本或相同集成深度。没有读取用户私有 Goal 内容，也没有验证当前哪些长程任务正在运行。

### 核心事实

1. **以目标和当前局面生成下一轮工作。** Goal 周边保存 Todo、claim、gate、monitor、quota、证据与历史；Kernel 管理准入和接受，领域判断仍由 Agent/Capability 提供。宿主负责执行与唤醒。LoopX 的工作选择不只依赖一个预写步骤索引。[架构](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/docs/architecture.md)、[Turn 路由源码](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/loopx/control_plane/turn_driver/driver.py)。

2. **等待、继续、重规划分开。** interaction contract 区分有界交付、用户门禁、限定范围内的其他可执行任务、外部证据观察、无变化安静等待、自主重规划与恢复。反复无进展可以触发重规划义务，Agent 提交新的有据路线、后继工作或阻塞，Kernel 再接受；不能把它描述成控制面自动理解并修好业务。[交互模型](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/docs/state-interaction-model.md)、[Vision/Replan 合同](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/docs/reference/protocols/goal-vision-replan-contract-v0.md)。

3. **协作具有持久归属与执行占用。** 源码有 Task lease 的 owner、期限和写范围检查；Turn lane 使用进程持有的文件锁，拒绝同一 Goal/Agent 上第二个执行者。锁能阻止该入口内重复执行，不能证明绕开它的进程或所有远端服务都受控。[Task lease](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/loopx/control_plane/work_items/task_lease_acquire.ts)、[Turn lane fence](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/loopx/control_plane/turn_driver/lane_fence.py)。

4. **恢复区分业务产物、写回与结算。** 显式 Turn 集成有 validation、durable_writeback、quota_spend 等阶段；恢复原 effect 时先读取 provider，结果 unknown 保留未知，已提交结果可按原身份恢复。Todo completion 在写锁内复核来源 snapshot 和政策来源，变化时拒绝旧结果。[结算源码](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/loopx/control_plane/turn_driver/settlement.py)、[完成事务](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/loopx/control_plane/todos/completion_transaction.py)。这不为任意 shell/API 提供 exactly-once，也不意味着所有宿主均走 Turn journal。

5. **审批范围比普通选择节点具体。** decision_scope 与 required_decision_scopes 表达某个动作、工作线或目标被什么决定阻塞，使一个待批准动作不必阻塞其他独立工作。该合同本身不证明操作人已经通过独立身份认证，也不让旧回执永久有效。[Decision Scope](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/docs/reference/protocols/decision-scope-v0.md)。

6. **执行强度取决于接入路径。** 文档区分 in_loop、wrapper、passive_posthoc；纯事后观察不能在动作前拦截。原生 Host loop、App heartbeat、外部调度器或显式 Turn 驱动承担唤醒和执行，安装 CLI 本身不证明常驻运行。进程监督协议也明确不是沙箱，不保证外部副作用回滚或所有子进程即时停止。[适配深度](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/docs/state-interaction-model.md#agent-loop-adapter-depth)、[Turn 运行边界](https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/docs/reference/protocols/loopx-turn-v0.md)。

LoopX quota 是控制面的自动工作预算/记账；不能直接当作模型供应商账单，也不自动证明每种供应商配额重置都已接入。版本、Host、服务和 Goal 配置均可能影响实际路径；本次没有进行新的自治效率或可靠性排名。

## 对照与取舍

下表是基于双方上述已核实机制的选型判断。

| 维度 | LoopX | Spec Kit |
| --- | --- | --- |
| 主要管理对象 | 持续 Goal 与工作、权限、等待、证据局面 | 规格产物与一次 workflow run/step |
| 下一步选择 | 结合当前 Todo、gate、monitor、quota、重规划等重新准入 | 执行 YAML 后续步骤及已定义条件/循环 |
| 长时间等待 | 有 monitor、门禁和调度提示，靠选定 Host/调度接续 | 有 paused 和 resume；需调用者或外层系统再次驱动 |
| 变化与返工 | 支持把新证据变成后继任务、路线变化或阻塞 | 可以写修复分支/循环；不会仅凭 resume 自动获得长期目标规划机制 |
| 多执行者 | Agent/Todo 归属、claim/lease、Turn lane fence | 有界线程池 fan-out；未核实对应的跨执行者 lease |
| 故障恢复 | 支持已接入 Turn/Provider 的身份、回执、写回与结算恢复 | 当前顶层步骤重跑，嵌套恢复可能重复父流程 |
| 开发方法 | 由领域 Capability、Skills 和项目合同承接 | 自带规格驱动模板及可复用 YAML 步骤，便于统一单次开发程序 |
| 维护负担 | 需要维护 Host、状态来源、身份、Provider 与能力合同的配合 | 有限流程更容易阅读；跨日调度、共享执行权、外部幂等需另补 |
| 验收结论 | accepted transition/Turn settlement 不自动等于 Goal 验收 | completed 不自动等于代码正确、需求验收或已发布 |

**针对用户已使用 LoopX 的建议：保留 LoopX 作为长程总控，按缺口选择 Spec Kit。** 如果主要困难是需求写得不完整、规划和实现缺少统一步骤，可借 Spec Kit 的规格方法或有限 runner；如果主要困难是跨天等 CI/人审、换会话、改变计划、多人接力和防止空转，优先完善当前 LoopX 的接入与项目验证。

不默认要求同时装两套。已有 Skills/Capability 足以完成有界交付时，继续 LoopX + 当前项目流程即可；只有可复用步骤编排带来可验证收益时，才增加 Spec Kit runner。

### 可选组合契约（设计，未实现）

- LoopX 拥有 Goal/Todo 的优先级、执行权、唤醒、等待、批准与终局；Spec Kit 拥有一个有界阶段包的步骤进度。
- 保留一份业务规格。阶段包引用当前文档仓的提交/路径/摘要，不能让两套模板各自维护现行需求和完成状态。
- 关联 `goal_id + todo_id + turn_id + run_id + input_digest`；runner 交回 completed/paused/failed、产物引用、逐项验证与缺口，LoopX 的受控验证/写回路径接受后才推进 Todo。
- 等待 CI、人工决定或超出阶段预算时交回 LoopX，不让两边分别轮询同一事件。权威批准在 LoopX/项目平台登记，不能让实现 Agent 用 Spec Kit 输入 approve 代替它。
- 外层开始时检查一次权限不足以覆盖长子流程。暂停、撤销、lease 变化必须传递到 runner/Host，并在关键写入前重新检查；副作用使用独立幂等和读回适配，结果未知先核对。该桥接需要验证，当前没有完成。
- 试点按同一有限任务观察：测试失败、嵌套暂停、外部写入响应丢失、重复 resume、暂停后子进程、输入变化使旧审查失效；分别记录重跑范围、额外成本、人工介入和交付证据。不能借任一方历史 benchmark 声称另一方更慢或更差。

最终选择依据是用户的长程管理需求与现有流程缺口，而非项目名称中的 workflow。本次结论是架构适配判断，没有同任务对照实验可支持稳定性、成本或吞吐排名。
