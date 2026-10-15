---
document_kind: review
status: pass
delivery_issue: ming-iot#IKJMKJ
code_repository: git:gitee.com/ming-industry/ming-iot
exact_head: 75ecbaa6f69480f86e6a55380bd07c50f298b81f
related_documents:
  - specs/2026-10-04-map-delivery-ai-native-source-design.md
  - plans/2026-10-04-map-delivery-ai-native-source.md
---

# map-delivery 来源迁移独立评审

## 审查身份与范围

- 文档仓：`git:gitee.com/ming-industry/ming-iot-rd-docs`。
- Base：`93889ad506bbe7575b8fa9c546a486a06e882544`。
- 当前固定实现 Head（H2）：`75ecbaa6f69480f86e6a55380bd07c50f298b81f`。
- 首次固定实现 Head（H1）：`48b2a9b5ac438bd64bbbd552cac544f067e80a7f`，当次结论为 `changes_requested`，发现及验证记录保留如下。
- 团队来源仓：`git:gitee.com/ming-industry/ai-native`，固定来源 Head：`4277290d01083f983d41948b6703a9cea6bc67fd`。
- 获批 Review Todo 绑定上述版本，仅授权写本评审；未修改实现、提交、推送或执行合并。

首次完整审查 Base 到 H1 的 11 个变更文件，并对来源文件、完整锁和来源说明独立核对；随后审查 H1 到 H2 的注册器及安装测试两文件增量。Standards 与 Spec 由两个只读子审分别检查，主审运行行为反例；本评审正文的后续提交不改变受审实现身份。

## H1 Standards

0 项可操作发现。变更在批准范围及独立 `.worktrees/` 中；正式 Spec、Plan 使用中文，交付元数据及仓库相对引用符合现行根约定。未修改 archives、历史 IKEOFY Change/Plan 或既有授权边界。没有需要提出的实质性代码结构问题。

## H1 Spec

### P2：原样重复迁移命令失败

H1 的 `tools/setup_workspace_skills.py:338-339` 无条件拒绝旧 canonical 路径成为符号链接，但首次成功迁移会把同一文档仓的 `.agents/skills/map-delivery` 建成新来源链接。再次执行计划 Task 4 的原样命令时，来源和全部六个入口已正确，脚本仍在检查旧来源时退出失败。

隔离反例采用固定 Head 的注册器及测试夹具：为工作区和产品仓预置四个指向文档仓旧路径的链接，旧路径不存在；连续两次执行相同参数 `--migrate-known-source --legacy-docs-root <该文档仓根>`。第一次退出 0，第二次退出 1，并报告“旧来源 canonical 路径已被符号链接改写”。六个入口仍正确，未观察到内容损坏。

这一行为违反规格“重复执行幂等”和计划“重复注册应无额外变更”。现有回归在首次迁移后去掉迁移参数再运行，未覆盖成员重复执行同一初始化命令的情况。应允许旧 canonical 已准确指向本次已核验新来源的完成状态，同时继续拒绝未知来源链接；补充原样二次迁移及未知旧 canonical 链接拒绝回归后，固定新实现 Head 重审。

## H1 独立验证

- 独立枚举来源 4 个分发文件并计算 SHA-256，与固定来源 `skills.lock.json` 的库存和摘要完全一致，无缺失或额外项。业务 SKILL 仅改变地图定位和第一步文档定位；workflow 最后注册段落之前的内容与 Base 一致，保留交付语义和授权边界。
- 16 个额外隔离反例通过：缺失、额外及篡改文件；手工重算锁后的错误或重复 name、YAML tag/map/坏引号；本地引用越界、百分号编码越界及缺失引用；错误来源身份；迁移时未知悬空链接全局预检；较晚链接替换失败的原始链接回滚；原子 exclude 替换失败后较早 exclude 和链接回滚；文档、产品、来源三个 linked worktree 联合安装及普通命令幂等。
- 从固定实现 Head 单独克隆文档仓，在没有产品仓或团队仓兄弟 checkout 的环境运行 repository policy 与 workspace skills 测试：43 项通过。仅在临时目录设置已批准 origin、仓库 hook 路径和 `python` 到现有 Python 3 的别名，没有创建外部凭据。
- 原样重复迁移命令的独立反例失败，构成本次唯一明确的要求缺口。未因既有完整测试通过而忽略该缺口。

## H1 结论与限制

Standards：0 项；Spec：1 项 P2。H1 固定实现 Head 的结论为 `changes_requested`，请求修正后重新评审。

来源说明仍为 `source-ready`，文档迁移为 `pending`，`migration_complete=false`，与分阶段交付计划一致。本评审不证明文档 PR 已合入、主工作树已切换或团队成员已安装。主审没有重复执行报告的完整 204 项测试，没有验证远端 PR、云端 CI、分支保护或宿主发现；后者由交付者另外收集证据。验证范围为当前 Linux/WSL 与普通可恢复运行失败，不覆盖原生 Windows、并发人工修改、持续不可写介质或进程/电源中断的事务恢复。

## H2 增量复审与当前结论

H2 仅修订注册器及安装回归。已完成的旧 canonical 入口只有在词法目标与实际解析目标均准确指向本次已核验新 canonical 时获准；经未知别名间接解析到同一来源仍拒绝，未扩大任意覆盖或授权范围。回归改为原样重跑迁移参数，并新增间接旧 canonical 链接拒绝案例。

主审从 Git 克隆并检出固定 H2，在临时夹具中使用固定来源 Head 的原始文件，独立执行六个反例，均通过：

1. 同一显式迁移命令连续运行三次，后两次均为六个 `existing`，链接时间与 exclude 内容不变。
2. 使用默认旧来源路径的同一迁移命令重复执行成功。
3. 指向正确来源的未知间接别名仍拒绝，全部原链接和 exclude 保持原状。
4. 未知悬空旧 canonical 链接仍拒绝，未创建其余入口。
5. 旧 canonical 已直接指向新来源时，较晚链接替换故障精确回滚先前迁移，保留原链接文字与 exclude，无残留临时链接。
6. 安装完成后把旧文档仓 identity 改错，迁移仍失败且原状态不变。

固定 H2 的 33 项安装测试通过。Standards 和 Spec 两个独立增量子审均无新增可操作发现；H1 的唯一 P2 已修复，当前结论为 `pass`。

另外只读核对交付者的 H2 `integration-result.json`：文档和来源 Head 与本评审一致，六个旧入口迁移及迁移参数重跑成功，三个隔离 cwd 的 Codex 发现均启用新源且扫描错误为 0，定位器返回 H2，实际工作区未切换。完整测试日志确实记录 205 项通过；归档准备摘要仅记录四个已封存权限位恢复。这是对所给运行材料的核对，未由主审再次执行整套 205 项测试或宿主 RPC。

本次通过仅覆盖固定候选实现及上述局部材料；不表示 PR 已合入、真实主树已切换、成员安装完成、云端 CI 或分支保护已验证。H1 已记录的操作系统、持续故障、并发修改及进程/电源中断限制仍适用。后续变更实现时，应固定新 Head 验证受影响范围，不沿用本回执。
