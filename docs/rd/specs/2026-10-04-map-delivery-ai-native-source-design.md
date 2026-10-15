---
document_kind: spec
status: approved
delivery_issue: ming-iot#IKJMKJ
code_repository: git:gitee.com/ming-industry/ming-iot
related_documents:
  - plans/2026-10-04-map-delivery-ai-native-source.md
  - meta/SKILLS_SOURCE.json
  - map/00_研发治理/01_文档管理/spec.md
---

# map-delivery 团队来源迁移规格

## 目标与现行依据

用户于 2026-10-04 选择已有 `ai-native` 管理团队技能，并授权继续执行来源迁移。真实交付身份为 [ming-iot#IKJMKJ](https://gitee.com/ming-industry/ming-iot/issues/IKJMKJ)，[实施计划](../plans/2026-10-04-map-delivery-ai-native-source.md) 组织验证和候选交付。

文档管理模块入口仍标明设计中，根 `AGENTS.md` 与顶层交付目录是现行规则。本次沿用该规则的独立 Spec + Plan 工程交付，不新建竞争 IKEOFY 锚点的活动 Change，不重开已关闭的 IKEOFY，也不倒改其历史 Change/Plan。模块主规格仅同步技能来源段落；来源迁移不代表整个模块实施或激活完成。

## 变更前后

当前主树跟踪 `.agents/skills/map-delivery/`，注册工具固定从文档仓读取源码，旧技能从自身安装位置定位文档仓，五个工作区/产品/Claude 入口指向文档仓原件。团队 AI 仓已有经过验证的可移植技能，但仍记录跨仓迁移未完成。

变更后，`git:gitee.com/ming-industry/ai-native` 的 `skills/map-delivery/` 是唯一被版本化的技能源码。`ming-iot-rd-docs` 保留文档规范、模板、工具和交付记录，跟踪 `meta/SKILLS_SOURCE.json` 来源契约；本机可以安装被忽略的技能入口，但不提交源码副本。技能通过独立定位器核验所选文档工作树，不从自身安装位置推断正文归属。

## 注册与安全要求

`tools/setup_workspace_skills.py` 保留既有命令入口，显式接受团队仓路径；常规布局默认从工作区 `ai-native` 定位。团队仓必须是身份正确的 Git 工作树，技能路径不得通过符号链接逃逸，文件全集和 SHA-256 必须与团队锁文件一致。来源验证和全部目标预检先于任何写入。

支持工作区、产品仓和文档仓各自的 `.agents/skills/map-delivery` 与 `.claude/skills/map-delivery`，共六个相对链接。重复执行幂等。正常安装拒绝覆盖异源链接、普通目录或受跟踪目标；显式迁移选项只处理已核验旧文档仓来源的链接，不能作为任意覆盖开关。

失败时回滚本次链接和 Git 本地排除文件修改；未知冲突、错误身份、内容篡改、路径逃逸和链接权限不足必须明确失败。真实 TeamAI 的完整复制副本由团队仓检查器验证，不因存在副本而删除或覆盖它。没有链接权限的宿主使用其官方安装方式，再分别验证内容和发现。

## 跨仓与生效要求

先发布来源准备就绪的 `ai-native` 版本，再提交文档迁移候选。文档主树原件在迁移 PR 合入前保留；候选 worktree 中删除原件并执行隔离安装测试。PR 合入后更新主树，再从已合入入口迁移本机注册。未知内容保持原状。

正式迁移完成必须绑定文档仓真实合入 revision、团队技能版本、完整内容检查和宿主实际发现结果。两仓分别记录状态，不声称原子提交。仅准备来源或创建 PR 时，`migration_complete` 保持 false。不能以本地测试替代成员接入、云端 CI 或分支保护验证。

## 验收与非目标

验收覆盖外部来源六入口注册和幂等、默认拒绝覆盖、已知来源迁移、全局预检、失败回滚、完整锁和路径安全、linked worktree，以及没有兄弟 checkout 的文档仓 CI。真实 Codex `skills/list` 在隔离消费者中确认发现新来源，显式定位器确认文档工作树身份。

不调整 IKEOFY 其他阶段，不修订其 Superpowers 历史记录，不自动合并文档 PR，不创建或扩大凭据，不启用真实 TeamAI 自动同步、hooks、遥测或成员上报。原始运行证据留在忽略目录，共享文件只记录稳定摘要。
