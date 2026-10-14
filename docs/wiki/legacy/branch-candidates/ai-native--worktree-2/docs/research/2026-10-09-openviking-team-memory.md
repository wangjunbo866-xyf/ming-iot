# OpenViking 在 AI Native 小团队中的记忆与 Skills 边界

核查日期：2026-10-09。对应决策问题：怎样满足“个人记忆保持私有，经本人或责任人确认后才进入团队知识”，同时保留 OpenSpec 的规格权威、LoopX 的长程推进和成员的工具选择权。

## 结论与证据范围

**OpenViking 可以作为可选的上下文服务试点，已有真实的身份隔离、资源 ACL、记忆审计、快照和 Skills 管理代码；它本身不能替代团队知识的确认发布流程。** 首版建议将私人记忆留在各自 user 空间，把获准分享的结论发布为受控的团队 resource；OpenSpec 和 Git 发布版本继续决定正式需求与技能内容。以下方案属于研究建议，尚未决定部署。

本次证据分三类：**D** 为官方文档陈述；**S** 为阅读固定源码获得的支持；**R** 为本团队真实运行验证。本次有 D、S，**没有 R**：没有安装服务、导入个人数据、运行跨用户隔离测试或修改 LoopX Goal/配置。仓库自带测试仅作为作者的测试资产，没有在本次运行，不能声称实际隔离已经通过。

| 固定对象 | 本次核实 |
|---|---|
| OpenViking main 快照 | `7967eca5845a03320b87f48be7336233a69d42bc`，提交时间 2026-10-09 19:53:06 +08:00 |
| 远端枚举到的最高稳定版本标签 | `v0.4.23` → `df32bf6e50a40843438f9491a26069ca4bd08f1f`，该提交时间 2026-10-02；GitHub releases API 限流，本报告不把标签枚举说成已确认最新 Release |
| 许可证 | main 和 v0.4.23 主项目 LICENSE 都是 AGPLv3；README 单列 CLI/examples 的 Apache 2.0、Hermes 示例的 MIT。不能依据历史文章把整仓称为 Apache 2.0 |
| LoopX 本地源码 | `fc25014937ebdf90a8519825049f34f861d7d3fd`，只读核查；适配器存在不证明服务已启用或互操作已验收 |

来源：[OpenViking 固定提交][ov-commit]、[稳定标签对应提交][ov-release]、[许可证][ov-license]、[README 许可及商业版本][ov-readme]。下文 OpenViking 链接全部固定在 main 快照。抽查 ACL、多租户、snapshot、namespace、认证源码与 v0.4.23 没有差异；Skills/Session 文档有少量更新，不将 main 的全部行为外推给所有安装版本。

## 1. 个人与团队隔离：已有能力，默认配置仍需选择

| 问题 | 核查结果 | 对本团队的含义 |
|---|---|---|
| namespace 只是文件夹吗？ | D+S：`account_id` 是外层租户身份，`user_id` 决定私有空间；服务按请求身份处理路径，`namespace.is_accessible` 拒绝其他 user 的私有 URI。不是仅凭目录名自律。[身份文档][ov-tenant]、[namespace 源码][ov-namespace] | 一个团队可对应 account，每位成员使用各自 user key；Agent 身份应能归责到人或明确的服务身份。 |
| peer 能代替成员身份吗？ | D+S：actor peer 只限制该用户的 peers 子树；同一 user 的自己的记忆、共享资源不会因此隔离。[namespace 源码][ov-namespace] | 多位成员共用一个 user key，再换 peer 名字，不满足个人私有。 |
| 团队资源是否默认只读？ | D+S：共享 `resources` 默认账号内可见；ACL 默认关闭。开启后，根仍以 `user:* = manage` 作为继承默认。[ACL 文档][ov-acl]、[ACL 源码][ov-acl-code] | 正式团队知识应建 `restricted` 子目录，明确发布者和读者；只打开 ACL 开关不够。 |
| ACL 覆盖所有类型吗？ | D+S：资源 ACL 只适用于 `viking://resources/...`，支持用户/平面组、read/write/manage 和继承；私有资源不接受这一 ACL。[ACL 路由][ov-acl-router]、[ACL 源码][ov-acl-code] | 不应假定同一套细粒度 ACL 自动覆盖私人 memory 或共享 skills。 |
| 谁能越过共享限制？ | D+S：账户 ADMIN 对共享 resources 有隐含 manage；API-key ROOT 用于管理 API，不能当普通用户访问租户数据。[访问控制源码][ov-access]、[认证源码][ov-auth] | 管理身份与发布身份应分开记录；“普通成员读不到”不等于对基础设施运营者加密隔离。 |
| 撤权立即生效吗？ | D+S：ACL 随索引更新，文档明确不保证强一致；源码读取一个 ACL 索引快照。[ACL 文档][ov-acl]、[ACL 源码][ov-acl-code] | 撤权传播窗口需实测；严苛隔离场景可能需独立账户/实例，不应承诺即时撤权。 |

注意一个文档与源码差异：多租户页仍描述 `api_key` 模式无 root key 时进入 dev 模式；固定源码的显式 `api_key` 配置会拒绝缺少 root key，并提示使用显式 dev 或自动检测。部署验收以固定版本实际行为为准，不能复制概念页配置后假定安全状态。[认证源码][ov-auth]

当前官方 README 明确把 accounts/user isolation 与 opt-in ACL 列在开源服务能力中；商业 self-managed 说明增加分布式部署及官方支持，SaaS 有个人/企业计划。**“团队权限全部是企业版专属”已不符合这次核查的资料。** 本次未取得商业合同功能矩阵，不进一步推断企业版与 OSS 的全部差异。[README][ov-readme]

## 2. 自动记忆、来源、纠错、删除与版本

**自动提取存在，批准后共享需另做。** Session commit 先归档消息并返回后台任务，后续摘要、抽取、`memory_diff.json` 才完成；抽取可以创建、改写或删除已有记忆。其依据是会话、记忆策略和已有记忆。可限定 Self/Peer 及类型，不宜让一次“任务完成”自动成为“团队认可的事实”。[会话机制][ov-session]、[Memory API][ov-memory]

`memory_diff.json` 记录实际 adds/updates/deletes 和被策略跳过的操作，适合回读变化；它不保证结论正确，也不提供独立审批人的批准。需要区分三个标识：原始材料的版本、记忆的 URI/内容摘要、实际召回时的版本。Agent Evolution 可把成功读取 Experience 的轨迹与结果关联，但官方明确说读取关系不能证明使用或因果收益。[会话机制][ov-session]、[Agent Evolution][ov-evolution]、[lineage 源码][ov-lineage]

已有 `snapshot commit/log/show/diff/restore`，可保存所选文件树的不可变版本。因此不能说 OpenViking 没有版本或回滚能力。但它只覆盖已提交且未排除的文件，ACL 与向量索引不在快照中；恢复会产生新提交，索引需要重建。它和业务 OpenSpec/Git PR 的审查批准是不同记录。[Snapshot API][ov-snapshot]、[snapshot 源码][ov-snapshot-code]

纠错可通过明确的新材料/指令或内容接口完成，consolidation 能在同一空间内整理、合并和删除记忆；要检查 diff 与最终 L2 内容。删除 session 只删除会话和其中的审计归档，已经抽取的长期记忆保留，需要单独定位处理。因此“撤回原始会话”不等于“相关知识全部忘记”；快照、备份和已发出的副本还需团队定义保留策略。[Consolidation][ov-consolidation]、[Session 删除语义][ov-session-api]

在本次检查的概念/API 文档、memory 模块、服务路由与 ACL 源码中，没有找到完整的“个人提出 → 本人/责任人审查 → 批准后团队发布”状态机或强制 reviewer 机制。现有共享动作依赖身份和写权限；私有 resource 移到共享目录会继承目标 ACL。这是**限定检查范围内的未发现**，不是对所有插件或商业版本的不存在证明。[ACL 文档][ov-acl]、[Filesystem API][ov-fs]

## 3. Skills：管理与分发有基础，但仍需发布权威

OpenViking 支持 SKILL.md、辅助文件、ZIP 和 Git 来源，支持私有及账户共享 Skills。导入是存储、摘要与索引，执行、工具权限与依赖安装仍由消费它的 Agent/Harness 负责；把 MCP schema 转为说明也不等于注册可执行工具。[Skills API][ov-skills]

不能把它简化成“仅检索”：当前 API 已有 `.source.json` 来源记录、包文件清单、内容 SHA-256 和完整性 revision；更新可从记录的 Git ref 重新拉取。revision 是包内容版本，不是 Git commit；返回完整性快照后再下载还需核对 hash。read/update/delete 对显式 target 找不到时还会回退私有/共享根，同名技能应核验返回的 `root_uri`。[来源元数据源码][ov-skill-source]、[Skills API][ov-skills]

本团队建议：Git 继续保存待审核源码、测试与固定发布 commit；OpenViking 建技能目录、适用场景和可搜索镜像。个人实验技能可自由使用；进入团队“已发布”目录的技能带 `source_repo + commit + package_digest + owner + applicable_scope`。更新走发布事件，勿把 `main` 上的自动刷新当成已审核发布。只有实际消费端核对版本并加载，才能称为分发完成。这是团队治理设计，不是 OpenViking 自动承诺的流程。

## 4. 与 OpenSpec、LoopX 如何分工

建议保留三条单向权威关系：

```text
OpenSpec/Git 已生效规格 ──索引/引用──> OpenViking 上下文
个人工作记忆 ──明确确认与发布──> 团队知识资源
LoopX 当前任务状态 ──检索参考──> OpenViking
```

召回内容只能提供参考与出处；它不能批准 PR、重新打开已关闭的 Task、改变 Goal 授权或自动覆盖 OpenSpec。需正式改变需求的记忆结论，转成 OpenSpec change，由相同的规格验收处理。可索引完整规格以便检索，但正式判断必须回读所引用的 Git revision，标注镜像更新时间和失效状态。

LoopX 已有可复用适配，不必从零写一层“全部工具总控”。本次只读核查以下源码边界：

| 适配 | 已有内容 | 未证明的内容 |
|---|---|---|
| Context provider | `retrieve`、`sync`、scope 分类、版本检查及不确定写入回读；公共 resources 和私有 user/peer 用不同写路径。[源码][loopx-context] | 当前机器服务已配置；团队权限已通过。 |
| Project peer semantic preference | 可选项目范围 recall，用 Git origin 派生稳定 peer；默认有限次 find，global fallback 显式启用；消费函数仍负责应用。文档明确当前 repo 模板为权威，不在记忆中复制模板正文。[协议说明][loopx-peer] | 它是个人用户下的项目记忆范围，不自动变为多人共享项目知识。 |
| History export | 将清理后的运行结论导出为有校验的本地资料；源码明确 OV ingest/recall 是另一调用方负责的动作。[源码][loopx-export] | 执行 exporter 就已经上传/共享成功。 |
| Periodic report | 已有写入资源的 extension，声明 `openviking_context_write` 权限并校验执行 envelope。[源码][loopx-report] | 此研究没有启用、试写或回读该插件。 |
| Session memory adapter v0 | 文档是只读适配契约，live retrieval/writeback 仍有 future gate。[协议][loopx-adapter] | 不能用这份旧边界否定上述其他已存在实现，也不能用其他实现证明该契约已兑现。 |

## 5. 面向第一条 IoT 真实需求的最小验证

先明确维护责任：个人拥有私人记忆；模块负责人确认领域结论；一名知识发布者维护正式共享目录；平台维护者管账号、模型配置、备份和故障。角色可以由同一人兼任，但系统操作记录要能分辨其责任。

团队分享建议采用一个小闭环，复用现有 PR/Review 记录，不为记忆另建完整审批平台：

1. 作者提出可分享结论，附出处、适用范围、来源 revision、是否有私人信息、到期/失效条件。
2. 本人确认可分享；若包含团队业务事实，由责任人确认准确性。该确认绑定待发布内容 digest。
3. 发布者把同一版本复制为共享 resource，记录原始引用和批准记录；私人原文不必整体搬运。
4. 用其他成员身份验证能召回正确版本，并检查不能读到私人原文。
5. 规格变化、复盘推翻或作者撤回时，标记/删除共享派生结论、更新索引，再做否定查询；已经被消费的知识不能靠删除假定自动撤销。

以下是**待执行的验收场景**，不是测试结果：

| 场景 | 输入与操作 | 通过条件 |
|---|---|---|
| 两人私有隔离 | A、B 各自 user key，同 account；A 写偏好及 IoT 调试摘要；B 直接 read、ls、grep、find/search，另测跨 account | 私有内容与摘要均不暴露；用猜 URI 与空目标检索也不能绕过。 |
| 分享只认批准版本 | A 提出摘要 R1，批准后发布 R1；再把私人稿改 R2 | B 只能获得 R1；不把 R2 自动同步到正式知识；发布回读 digest 与批准记录一致。 |
| 正式目录限写 | 建 restricted 目录：成员 read、发布者 write/manage；直接 API 绕过客户端提交 | 普通成员无法直接改正式结论；ADMIN 例外可解释且留痕；撤权传播时间被量出。 |
| 规格冲突 | 正式 OpenSpec revision N 要求 30 秒，旧记忆说 60 秒 | Agent 回读 N，并把旧记忆当冲突/失效参考；实现和证据绑定 N。 |
| 纠错与删除 | 更新/删除已提取记忆，再分别删 session、快照恢复 | 文本、检索、摘要的效果符合预期；单删 session 不被误报为彻底忘记；备份保留策略明确。 |
| Skill 完整性 | 导入已审核固定 commit；制造同名私有/共享技能、修改辅助脚本、刷新来源 | 消费端核验实际 root、commit/包 hash；版本变化需重新认可；无法静默执行另一根的同名技能。 |
| LoopX 退化 | 需求执行到一半让上下文服务不可用，再恢复 | 可从 OpenSpec、代码、证据与 LoopX 当前状态继续；不能丢失验收或授权；恢复后只补上下文。 |
| 实际收益 | 对同一批交接问题，比较直接读 Git 文档与 OV 检索 | 记录正确出处命中、首次找到证据的时间、人工纠错、维护分钟数、token/服务成本；不以记忆条数判断成功。 |

**采纳条件**：隔离与发布验证通过，出处可回读，跨会话/跨 Agent 找资料明显减少耗时，且每周维护成本可接受。**暂缓条件**：知识量小且 Git 检索已够用；身份或私有边界无法落地；需要手工维护第二套规格；只能展示检索相关性而不能改善真实交接。研究建议是先验证个人连续性加少量已批准团队知识，再决定是否扩大共享 Skills 和更多 Agent 接入。

## 固定来源

[ov-commit]: https://github.com/volcengine/OpenViking/commit/7967eca5845a03320b87f48be7336233a69d42bc
[ov-release]: https://github.com/volcengine/OpenViking/commit/df32bf6e50a40843438f9491a26069ca4bd08f1f
[ov-license]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/LICENSE
[ov-readme]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/README.md
[ov-tenant]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/docs/en/concepts/11-multi-tenant.md
[ov-namespace]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/openviking/core/namespace.py
[ov-acl]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/docs/en/concepts/15-acl.md
[ov-acl-code]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/openviking/storage/acl.py
[ov-acl-router]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/openviking/server/routers/acl.py
[ov-access]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/openviking/storage/viking_fs/_access.py
[ov-auth]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/openviking/server/auth/plugins/api_key.py
[ov-session]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/docs/en/concepts/08-session.md
[ov-session-api]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/docs/en/api/05-sessions.md
[ov-memory]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/docs/en/api/16-memory.md
[ov-evolution]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/docs/en/api/19-agent-evolution.md
[ov-lineage]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/openviking/session/memory/experience_lineage.py
[ov-consolidation]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/docs/en/context-compilation/06-memory-consolidation.md
[ov-snapshot]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/docs/en/api/11-snapshot.md
[ov-snapshot-code]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/openviking/storage/viking_fs/_snapshot.py
[ov-fs]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/docs/en/api/03-filesystem.md
[ov-skills]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/docs/en/api/04-skills.md
[ov-skill-source]: https://github.com/volcengine/OpenViking/blob/7967eca5845a03320b87f48be7336233a69d42bc/openviking/server/skill_source_metadata.py
[loopx-context]: https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/loopx/capabilities/context_providers/openviking.py
[loopx-peer]: https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/loopx/capabilities/semantic_preference/docs/openviking-project-peer.md
[loopx-export]: https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/loopx/extensions/openviking_semantic_preference/history_export.py
[loopx-report]: https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/loopx/extensions/openviking_periodic_report/provider.py
[loopx-adapter]: https://github.com/loopx-project/loopx/blob/fc25014937ebdf90a8519825049f34f861d7d3fd/docs/reference/protocols/openviking-session-memory-adapter-v0.md
