---
document_kind: evidence
status: partial
delivery_issue: ming-iot#IKJMKJ
code_repository: git:gitee.com/ming-industry/ming-iot
exact_head: 75ecbaa6f69480f86e6a55380bd07c50f298b81f
related_documents:
  - specs/2026-10-04-map-delivery-ai-native-source-design.md
  - plans/2026-10-04-map-delivery-ai-native-source.md
  - reviews/2026-10-04-map-delivery-ai-native-source-review.md
  - meta/SKILLS_SOURCE.json
---

# map-delivery 团队来源迁移验证

## 身份与状态

交付为 [ming-iot#IKJMKJ](https://gitee.com/ming-industry/ming-iot/issues/IKJMKJ)。实际文档任务仓为 `git:gitee.com/ming-industry/ming-iot-rd-docs`，独立候选分支为 `docs/map-delivery-ai-native-source`，Base 为 `93889ad506bbe7575b8fa9c546a486a06e882544`，受测实现 Head 为 `75ecbaa6f69480f86e6a55380bd07c50f298b81f`。统一元数据中的产品 Git identity 表示 Delivery Issue 所属交付体系，不代替实际文档任务仓。

团队来源仓为 `git:gitee.com/ming-industry/ai-native`，已发布并通过 Git 远端回读的 source-ready Head 为 `4277290d01083f983d41948b6703a9cea6bc67fd`。其完整锁包含四个技能文件。文档候选删除两个旧受跟踪文件，并保留被忽略的本机入口；当前实际文档主树尚未合入，旧原件和已有入口仍保留。

本摘要的 `partial` 表示跨仓正式切换未完成；下列实现验证通过。PR 合入、本机切换和团队成员接入不由本地测试替代，`migration_complete` 仍为 false。后续 Review/Evidence 提交只增加交付记录；候选 PR 的最终 Head 另由 `tools/gitee_pr.py` 回读绑定。

## 实现验证

环境为 2026-10-04 的 Linux/WSL、Python 3.12.3、Git 与 Codex CLI 0.160.0。

| 验证 | 观察结果 |
| --- | --- |
| `python3 -m unittest discover -s tests -v` | 205 项通过，25.517 秒；包含 33 项来源注册测试 |
| `python3 tools/check_delivery_metadata.py --docs-root . --since 2026-09-03` | 通过 |
| `python3 tools/map_docs.py check --docs-root .` | 通过 |
| `python3 tools/archive_manifest.py verify --root archives --manifest archives/manifests/archive-files.json` | 通过，封存内容未改 |
| `git diff --check` | 通过 |
| 发布前团队源完整锁、定位及安装测试 | 来源准备时 52 项测试通过，1.763 秒；锁与四文件一致，结果见来源提交的 `docs/validation.md` |
| 发布前固定 `teamai-cli@0.26.0` 的隔离分发 | doctor 退出 0，八项通过；本轮另与 source-ready 提交逐字节复核六个 skills/rules/hooks 分发资源和四个已安装技能文件，全部一致 |

系统只提供 `python3`，既有 pre-push 与测试夹具使用 `python`。完整测试使用忽略目录中的别名 `.local/skill-source-migration/bin/python` 指向现有 Python 3，并只在子进程 PATH 加入该目录；未安装全局包或改变共享 hook。

Git checkout 不保留非执行权限位。封存清单中四个 `.graph` 文件要求 0664，候选初次检出为 0644；只在独立 worktree 恢复其已封存权限后完成校验。文件内容、清单和 Git 中的 archives 差异均未变；没有重建清单使校验通过。

来源仓的 52 项测试与 TeamAI 完整试点是 source-ready 发布前对相同内容运行的验证，不是文档 H2 上的新运行。试点结果留在来源仓 `.local/teamai-pilot/result.json`，本轮对固定 `4277290d01083f983d41948b6703a9cea6bc67fd` 的内容复核回执为 `.local/teamai-pilot/source-ready-content-receipt.json`。试点没有部署真实宿主 hooks 或向真实远端写入成员登记。

## 隔离消费者与真实发现

从固定团队源与文档实现提交克隆临时消费者，并使用独立 HOME、CODEX_HOME、Git 配置和仅允许 file 协议的 Git 环境。没有把候选注册器用于真实工作区。

- 预置六个旧来源入口，常规注册拒绝覆盖，六个链接保持原状。
- 显式已知来源迁移成功转换六个入口；常规注册以及原样带迁移参数重跑均显示六个 existing，链接时间不变。同一文档根已经成为新来源入口的完成状态也可重复执行。
- 工作区、产品仓、文档仓的 `.agents` 与 `.claude` 入口均指向已核验新源；Git 本地排除生效，没有生成跟踪副本。
- 团队仓 `team_skills.py check` 对工作区、产品和文档入口通过；文档定位器回读目标文档根、批准身份及受测完整 Head。
- 真实 Codex app-server `skills/list` 在工作区、产品仓和文档仓三个 cwd 均发现已启用的新来源 `map-delivery`，三处扫描错误数均为 0；没有启动模型轮次。

独立评审见 [Review](../reviews/2026-10-04-map-delivery-ai-native-source-review.md)。首次固定 Head `48b2a9b5ac438bd64bbbd552cac544f067e80a7f` 的完整 204 项测试通过，但 reviewer 独立发现原样迁移重跑失败。当前实现修正该 P2 并增加同命令重跑及未知间接链接拒绝反例，未把首次通过套用到新实现。

H2 独立复审为 pass，六个额外反例与 33 项安装测试通过，Standards 和 Spec 均无新增可操作发现。另一个只读检查者核对 Base 到 H2 的全部变更均在获批 write scopes 内，archives 与历史 IKEOFY 差异为空；四份用户未跟踪学习文档的 SHA-256 与原迁移清单一致，原仓 `ming-ai-config` 已不存在。真实旧入口保持原状。增量及来源的启发式边界扫描未发现疑似真实凭据或完整私有业务文档复制，唯一 URL 模板命中属于回归夹具；此扫描不证明任意未知凭据或部分改写复制均不存在。范围回执保留在实际文档主树 `.local/skill-source-migration/h2-boundary-check.json`。

## 证据与限制

原始运行材料保留在候选 worktree 的忽略目录 `.local/skill-source-migration/`：`full-tests-fixed.log`、`integration.py`、`integration-result.json`、`archive-checkout-preparation.json` 和 Review Todo。共享仓库仅保留本摘要、可重跑的工具与回归测试，不提交可再生日志、原始会话或凭据。

验证支持当前 WSL 的符号链接注册和普通可恢复异常回滚，不覆盖原生 Windows、并发人工修改、持续不可写介质、进程或电源中断的事务恢复。Git 远端读取不证明 ai-native 私有可见性、团队成员权限、云端 CI 已执行或分支保护已启用。当前没有真实主树切换或成员安装完成事实；后续按计划 Task 4 收集实际切换证据。
