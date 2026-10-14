---
document_kind: plan
status: approved
delivery_issue: ming-iot#IKJMKJ
code_repository: git:gitee.com/ming-industry/ming-iot
related_documents:
  - specs/2026-10-04-map-delivery-ai-native-source-design.md
  - meta/SKILLS_SOURCE.json
---

# map-delivery 团队来源迁移计划

本计划执行 [已批准规格](../specs/2026-10-04-map-delivery-ai-native-source-design.md)，关联 [ming-iot#IKJMKJ](https://gitee.com/ming-industry/ming-iot/issues/IKJMKJ)。用户本轮明确的来源迁移指令是有界实施授权；每一项 Task 是同一仓库级 Agent Todo 内的检查点，不新建 LoopX Goal 或逐项 Gate。

## 仓库与候选顺序

1. `git:gitee.com/ming-industry/ai-native`：发布 source-ready 来源、定位器和完整锁，保持 `migration_complete: false`。记录精确版本，不宣称文档主树已删除原件。
2. `git:gitee.com/ming-industry/ming-iot-rd-docs`：从批准的目标分支在仓库内 `.worktrees/` 准备独立候选，删除跟踪源码，更新注册、契约、政策测试和本次文档；验证并独立 Review 后推送候选分支、创建 Draft PR。
3. 有权限的人在 Gitee 合并文档候选后，更新文档主树，从已合入脚本注册实际宿主。Agent 执行合并须另有绑定精确 PR、Head、目标分支和方式的授权；本计划不授予该权限。
4. 回读合入事实及宿主发现，才在团队仓记录 `migration_complete: true`。未满足第 3 步时保持待合入，不自动重定向现有工作区。

## Task 1：发布团队来源

更新团队仓 provenance、manifest 与说明，保留旧文档仓原始 revision 供追溯。技能正文继续使用已验证的独立文档定位器；注册参考说明新来源和分阶段切换。技能文件变更后更新完整锁并验证。保留既有未跟踪学习文档，不并入迁移提交。

## Task 2：文档候选与安装回归

新增 `meta/SKILLS_SOURCE.json`，把注册工具改为读取团队仓的唯一技能源。删除候选中的两个受跟踪技能文件；主树在 PR 合入前不动。

先覆盖来源缺失/错误/篡改、六入口、幂等、未知冲突无写入、已知来源迁移、符号链接和排除文件回滚、linked worktree，以及单仓 CI。保留原有 remote、hook、元数据、档案与地图检查语义，不通过放宽无关门禁使测试通过。

## Task 3：独立验证与候选发布

在选定文档 worktree 运行完整 unittest、`check_delivery_metadata.py --since 2026-09-03`、`map_docs.py check`、既有封存档案清单验证和 `git diff --check`。临时工作区执行实际新来源注册、团队仓内容检查、定位器和 Codex app-server `skills/list`；不把未合入脚本用于真实工作区切换。

固定实现 Head，独立 Reviewer 检查源码来源、内容锁、路径和覆盖边界、回滚、单仓 CI 及交付说明。Review/Evidence 记录完整 Head、命令、结论和限制。候选更新后重验受影响范围，不把旧 Head 通过套到新候选。

验证后只推送候选分支，通过 `tools/gitee_pr.py` 创建/更新 Draft PR。PR、Issue、规格、计划和验证导航补齐真实对象；缺失对象标记待建立，不生成假链接，不覆盖已有 Issue 正文。

## Task 4：合入后的实际切换

从已合入的文档主树运行，工作区布局不同则显式传入真实团队仓和旧文档工作树路径：

```bash
python3 ming-iot-rd-docs/tools/setup_workspace_skills.py \
  --workspace-root "$PWD" --product-repo "$PWD/ming-iot" \
  --skills-repo "$PWD/ai-native" --migrate-known-source \
  --legacy-docs-root "$PWD/ming-iot-rd-docs"
```

检查六个入口、被忽略的生成内容和 Codex 实际发现；重复注册应无额外变更。显式 `--docs-root` 验证所选文档 worktree，不因技能移动而改换交付仓。TeamAI 复制安装使用团队仓 `check` 与宿主发现验证；不覆盖其已有目录。

## 中断与恢复

来源已发布、文档 PR 未合入：主树旧入口继续可用，候选明确待合入。合入后尚未本机注册：新契约存在，旧链接可能悬空；运行已知来源迁移，失败则保留错误和原入口状态，不声称完成。任何未知目录、异源链接或权限失败交由维护者确认实际内容，不能扩大覆盖范围。
