# Ming IoT 本地 Share 模块迁移激活规格

> 日期：2026-08-27
>
> Goal：`ming-iot-framework-v1`
>
> 状态：已批准实施；批准仅适用于本文冻结的来源、精确 write scope 与许可证路线，commit、merge、push 仍需分别授权

## 1. 决策摘要

本规格确认 `share@v1.6.4` 的来源快照可以稳定复现。首次冻结时，公开证据不足以证明整个仓库可被重新分发：Tag 内没有仓库级 LICENSE、NOTICE 或 COPYING 文件，Gitee 仓库元数据的 `license` 也为空。四个文件中的 Apache-2.0 文件头只覆盖对应文件，不能据此推导整个 Share 仓库的授权。

2026-08-27，用户以权利人或公司有权责任人身份确认公司拥有该冻结版本相关代码的复制、修改和再发布权，批准迁入 `ming-iot` 并采用 `Apache-2.0`，同时要求保留已有第三方版权及 Apache-2.0 文件声明。该决定满足第 4.3 节第 3 条，关闭本次源码迁移的许可证 Gate，但不构成 commit、merge 或 push 授权。

因此本次激活结论是：

1. 允许从冻结 Git tree 把源码复制到 `ming-iot/backend/share`，并按第 5 节实施原子引用切换；
2. `backend/share` 采用仓库级 `Apache-2.0`，已有第三方版权和 Apache-2.0 文件头必须原样保留；
3. 源码迁移不等于获准向 Gitee 发布；push 仍需单独授权；
4. 真正开始源码迁移前，必须重新生成目标文件 allowlist，并确认其摘要与本规格一致，否则本批准失效。

## 2. 权威输入

- 迁移方向与非目标：`proposals/2026-08-25-ming-iot-local-share-migration.md`；
- 来源仓库：只读工作区 `share/`，公开仓库标识 `gitee.com/unitedrhino/share`；
- 来源快照：Tag `v1.6.4`，commit `ece02ca9d6defd63356931a59e653950f27ac572`；
- 机器证据：`evidence/2026-08-27-ming-iot-local-share-migration-activation/source-freeze.json`；
- 目标快照：`ming-iot` 本地 commit `61c9c1a928f23c97332ebf93ac84801591d170b9`。

若本文与已批准提案冲突，以更严格的许可证 Gate 和本文冻结的实施范围为准；若目标树发生变化，则重新盘点，不沿用当前数量和摘要。

## 3. 来源冻结

来源 Tag 当前满足以下可复现合同：

| 项目 | 冻结值 |
|---|---|
| Tag | `v1.6.4` |
| Commit | `ece02ca9d6defd63356931a59e653950f27ac572` |
| Tree | `21bdb52ed508152cf440b8d8a3b3ae3943a4b84a` |
| Tracked 文件数 | 255 |
| `git ls-tree -r` 摘要 | `6a7d2b40e5a6f6fdeb7d7a3d2f01fe1994efab3aa2c2f52ba352c50f4c4a5a6c` |
| `git archive` 摘要 | `8c90b2b3853ad86b422f9929d801d617645bac3dbf68f600e3736b3a32505acf` |
| 旧 module path 自引用文件 | 84 |
| 自引用路径清单摘要 | `5d57488bf044c8659a7f032561371d6eac756edf89169ea23ffed340c32beb42` |

实施只能从该 Git tree 的 tracked 文件生成候选树，禁止使用工作目录递归复制，以免带入 `.git`、缓存、构建物或本地配置。

## 4. 许可证与版权 Gate

### 4.1 已证实事实

- Tag 内没有仓库级 LICENSE、LICENCE、NOTICE 或 COPYING 文件；
- Gitee 仓库公开元数据没有声明许可证；
- `utils/keyMatch.go` 与 `utils/notify/notify_*.go` 共四个文件包含 Apache-2.0 文件头和第三方版权声明；
- 其余源码没有形成可验证的仓库级再分发授权。

### 4.2 不允许的推断

- 不能因为仓库可公开克隆，就推断允许复制、修改或再发布；
- 不能把四个文件的 Apache-2.0 声明扩大为整个仓库的许可证；
- 不能由 AI、开发人员或本项目自行补写一份许可证来替代权利人授权。

### 4.3 关闭 Gate 的有效证据

满足以下任一项后，才能进入源码迁移：

1. 上游权利人在来源仓库发布适用于该 Tag/Tree 的仓库级许可证，并冻结许可证文件摘要；
2. 权利人提供明确覆盖复制、修改和再发布的书面授权，项目只记录公开安全的授权结论与证据指针；
3. 经公司有权责任人确认，组织拥有全部相关代码权利并批准采用明确许可证，同时保留四个第三方文件的原版权和 Apache-2.0 声明。

若以上证据均不存在，迁移方案必须保持阻塞，继续使用外部 module 依赖；测试通过不能替代该 Gate。

## 5. 精确 write scope 决策包

后续实施只允许以下四类写入，任何新增类别都使批准失效：

| 类别 | 允许写入 | 允许变化 |
|---|---|---|
| A. 来源落位 | `backend/share/**` | 从冻结 Git tree 创建 255 个来源文件；仅 module/import path、经批准的来源说明及许可证材料可与上游不同 |
| B. SaaS 引用切换 | 当前 allowlist 中 `backend/saas/**` 的 818 个命中文件 | 机械替换旧 import/module path；不得调整业务逻辑 |
| C. Things 引用切换 | 当前 allowlist 中 `backend/things/**` 的 726 个命中文件 | 机械替换旧 import/module path，并更新与旧模块身份绑定的基线合同 |
| D. 工具与 Workspace | 当前 allowlist 中 `backend/tools/**` 的 4 个命中文件，以及 `backend/go.work`、必要时的 `backend/go.work.sum` | 更新工具合同、加入本地 Share module、删除旧模块下载依赖 |

当前 `backend` allowlist 合计 1548 个旧路径命中文件，其中 1539 个为 Go 文件、9 个为非 Go 文件。实施前必须重新运行同一盘点命令；只有路径清单和数量未发生变化时，才可沿用本批准。文件正文可以变化，但若路径集合变化，必须重新生成决策包。

### 5.1 明确排除

- `docs/superpowers/**` 中 5 个历史研发文档不参与机械改写；它们属于历史记录，不能因“零命中”目标而篡改；
- 不修改原始 `share/` 仓库；
- 不修改 `core/`、Kafka、NATS、缓存或场景联动语义；
- 不增加长期 `replace gitee.com/unitedrhino/share => ./share`；
- 不升级任何依赖，不做 API 清理或目录重构。

## 6. 原子实施顺序

后续实施计划必须按以下顺序形成一个候选树，不能把新旧 module 身份长期混用：

1. 再次验证来源 Tag、commit、tree、tracked 文件数和许可证 Gate；
2. 生成来源 tracked manifest 与目标旧路径 allowlist，比较本规格摘要；
3. 从 `git archive v1.6.4` 生成 `backend/share` 候选树；
4. 原子修改 Share module path、自引用、SaaS、Things、Tools 和 Workspace；
5. 执行旧路径扫描、module graph 验证、分层测试和 Things 启动回归；
6. 独立 Reviewer 验证来源差异、许可证证据、零旧依赖和运行时回归；
7. 只有全部 Evidence 与批准范围一致，才允许提交候选树；push 仍服从仓库级授权规则。

## 7. 验证边界

### 7.1 来源一致性

- `git rev-parse v1.6.4^{commit}` 与冻结 commit 一致；
- `git rev-parse v1.6.4^{tree}` 与冻结 tree 一致；
- tracked 文件数、`ls-tree` 摘要和 archive 摘要一致；
- 原始 `share/` 工作树保持干净。

### 7.2 依赖闭合

- SaaS、Things、Tools 和 Share 的 active build inputs 不再引用 `gitee.com/unitedrhino/share`；
- `go list -m` 证明三者解析到本地 `gitee.com/ming-iot/share`；
- 新旧 Share module 不同时出现在 module graph；
- 清空独立 module download 测试环境后，构建不请求旧 Share module。

### 7.3 回归验证

- Share、SaaS、Things、Tools 的聚焦测试先通过，再执行全量 test、Race 和 Vet；
- Things 可重复启动基线执行冷启动、协议探针、warm restart 与受保护数据库验证；
- 迁移前后运行时 Evidence 的差异必须逐项解释；
- Reviewer 必须独立核对候选树，不以实施 Agent 的口头说明替代证据。

## 8. 审批问题

用户只需判断以下两件事：

1. 是否批准第 5 节定义的实施 write scope，且接受路径集合变化会使批准失效；
2. 选择哪一种第 4.3 节许可证证据路线，并提供对应的权利人或有权责任人结论。

在两项都得到明确批准前，LoopX 不得创建“复制 Share 源码并切换 import”的实施 Todo，只能保留本 Gate 或继续只读核验。
