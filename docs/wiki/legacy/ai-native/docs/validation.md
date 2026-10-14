# 来源迁移与本机验证

验证日期：2026-10-04。环境为 WSL Linux x86_64，Python 3.12.3、Git、Codex CLI 0.160.0。团队源为已有的 `ai-native`；下文分别记录正式来源切换，以及来源准备阶段的自动测试、技能完整性、隔离分发和发现检查。结果不证明仓库私有可见性、成员权限或成员已完成接入。

## 正式来源与当前宿主切换

来源迁移绑定 [ming-iot#IKJMKJ](https://gitee.com/ming-industry/ming-iot/issues/IKJMKJ)，目前状态为 `canonical`、`migration_complete: true`。完成范围是唯一团队源码归属迁移及当前 WSL 宿主注册、内容检查和 Codex 扫描；其他成员、原生 Windows 与未来 worktree 的安装需分别验证。

- 切换前已发布团队源：`git:gitee.com/ming-industry/ai-native`，精确 Head `4277290d01083f983d41948b6703a9cea6bc67fd`，技能路径 `skills/map-delivery`。状态收尾只更新元数据与说明，技能内容和锁保持该发布版本。
- 文档仓 [PR !11](https://gitee.com/ming-industry/ming-iot-rd-docs/pulls/11)：已回读 `state=merged`、`draft=false`，PR Head `ef9a37f57c5584f30fbe5fdcbdfb8036e3738a4c`，合入时间 `2026-10-04T15:01:55+08:00`。
- 文档主树已快进至 `b16030c10cf2565ecfa16ab19ee43a3a273bec57`。合并父版本为旧主树 `93889ad506bbe7575b8fa9c546a486a06e882544` 与上述 PR Head；合并树与 PR 候选一致，旧技能的两个跟踪文件已删除。
- 已合入注册器返回 `5 migrated`、`1 created`；工作区、`ming-iot` 与文档主树的 Codex/Claude 六个相对链接均指向本仓库正式源码。
- 必需技能检查对工作区及两个项目通过，定位器明确绑定文档主树并返回合入版本 `b16030c10cf2565ecfa16ab19ee43a3a273bec57`。
- 本机 `skills/list` 对上述三个目录均返回已启用的 `ai-native/skills/map-delivery/SKILL.md`，每个目录扫描错误数为 0。结果保存在忽略目录 `.local/discovery-after-formal-cutover.json`，未启动模型轮次。

这些结果不证明当前已打开聊天重载了技能目录、模型每次阅读了技能、团队成员完成安装或仓库权限已核实。本机切换采用普通注册脚本，未在真实宿主执行 TeamAI init；TeamAI 的分发验证仍归属于下文的隔离试点。

## 迁移后验证状态

在 `/home/wangjunbo/workspace-iot-platform/ai-native` 复跑自动测试：52 项通过，用时 1.792 秒；技能完整性 JSON 检查通过。完整隔离 TeamAI 试点返回 `ok: true`、`doctor_exit_code: 0`，没有缺失或不一致文件，新运行目录和技能安装路径均位于本仓库的 `.local/teamai-pilot/`。

原有 `Steps of AI Adoption.md` 和 `docs/skills/` 三份学习指南保留，内容哈希未变化，仍未跟踪，不纳入本次提交。迁移前的 `.local/` 历史记录保留，仅证明当时的观察；迁移后以新试点结果和发现快照为准。原候选仓库已经删除，旧初始提交通过本仓库 `.git/team-skills-migration/source.bundle` 保留，bundle 已校验；不把备份提交或运行证据发布为团队资产。

已通过 Git `ls-remote` 确认 `https://gitee.com/ming-industry/ai-native.git` 可读取；2026-10-04 接入前的远端 HEAD 与 master 为 `05fc076d2ae9b546063ec0c8c109aa5f0a6904f4`，这是发布团队技能候选前的历史观察。同日接入前的仓库 API 查询返回 404，私有可见性和成员权限尚未核实。

## 技能与安装检查

- `python3 -m unittest discover -s tests -v`：52 项测试通过，覆盖独立文档 worktree、错误身份和定位歧义、安装冲突、Git 排除路径逃逸、回滚、复制副本、必需技能被排除、禁用配置、重复或非法 YAML 头部和完整内容版本。
- `python3 tools/team_skills.py validate --json`：通过，技能完整文件 SHA-256 与 `skills.lock.json` 一致。
- skill-creator 的 `quick_validate.py skills/map-delivery`：迁移前对同一技能内容验证通过。
- 文档仓本地身份为 `git:gitee.com/ming-industry/ming-iot-rd-docs`，验证 Head 为 `93889ad506bbe7575b8fa9c546a486a06e882544`。

## 历史来源准备

以下记录属于文档 PR 合入前的来源准备阶段。团队源身份为 `git:gitee.com/ming-industry/ai-native`，当时状态为 `source-ready`；provenance 保留旧文档仓来源及 `93889ad506bbe7575b8fa9c546a486a06e882544`。当时迁移 PR 尚待合入、`migration_complete` 为 `false`，主工作树保留原件。当前完成状态以上文正式切换记录为准。

本轮只调整来源与注册参考段落，定位脚本和已批准的业务交付语义不变。`team_skills.py lock --json` 已更新锁文件；`validate --json` 返回 `ok: true`，skill-creator 的 `quick_validate.py` 通过，`git diff --check` 通过。技能差异仅为 workflow 最后一段来源说明，locator 与入口正文未修改。

最终来源内容复验：52 项 unittest 通过，用时 1.763 秒；完整隔离 TeamAI 试点返回 `ok: true`、`doctor_exit_code: 0`，4 个源码文件与安装副本一致，没有缺失或摘要差异。隔离 Codex 发现结果通过，没有扫描错误。独立 Reviewer 另行枚举 4 个源码文件并核对锁，未发现阻断项。上述结果支持来源发布，不证明文档主树已经合入迁移。

## 分发与真实发现接口

固定 npm `teamai-cli@0.26.0`，隔离 Node 为 `22.21.1`。真实 CLI 在独立 HOME/CODEX_HOME 和本地 bare Git origin 中完成 init、pull、doctor；doctor 全部 8 项检查通过。技能的 4 个源码文件逐个比对一致。必需技能检查器对 TeamAI 安装副本通过。

Codex app-server `skills/list` 在迁移后的隔离消费者中发现已启用的 `map-delivery`，没有扫描错误。首次试点中，普通相对链接注册在另一隔离工作区和产品仓中，也通过同一真实发现接口。

历史观察：当前工作区和 `ming-iot` 在首次注册前均未发现技能；通过文档仓原注册脚本注册后，两处发现并启用文档仓原件。接入 `ai-native` 的来源准备阶段再次读取，两处仍为 `passed: true`，扫描错误数为 0，当时主工作树入口尚未切换。正式切换后的三个目录发现结果见上文，来源已是 `ai-native`。

这些发现检查没有启动模型轮次。本轮聊天注入的可用技能目录已经包含 `map-delivery`；桌面界面显示、其他成员和新聊天的加载仍须分别确认。

## 独立只读使用

独立 agent 从 `ming-iot` 进入、按候选技能定位文档仓、读取业务地图和文档管理模块，并运行文档仓只读 `map_docs.py check`：通过。它区分了现行依据、候选和本地观察，未改写交付文档或访问远端。

使用中发现既有 IKEOFY Plan 第 15 行仍引用 `superpowers:subagent-driven-development` / `superpowers:executing-plans`，第 504 行还要求 `superpowers:writing-skills`。这是已批准计划的旧执行依赖，来源准备不会自行替换它；实际执行该计划时按其批准范围处理。

## 保留的边界

- 采用已有的 `ming-industry/ai-native`，Git 读取成功。接入前仓库 API 查询返回 404，仓库私有可见性和成员权限仍未核实。
- TeamAI 试点是手动同步，不部署真实宿主生命周期 hooks。自动同步和规则注入仍须另行验证。
- `--dry-run pull` 没有改变试点项目文件，但会改变隔离 HOME 中的 Git fetch 元数据和调试日志，不能宣称零写入。
- TeamAI init 的成员登记只写入本地 bare origin 的 `teamai-reports`，没有真实远端写入；真实成员接入时需考虑这个行为。
- 完整 TeamAI pull 会镜像远端已提交的根 `docs/`，包括 `docs/skills/` 指南；隔离试点未验证这个通道。本机原有未跟踪学习资料不因迁移或镜像说明被自动纳入提交。
- 文档仓 PR !11 已合入，跟踪原件已删除，当前 WSL 主树入口已切换；`migration_complete: true` 只覆盖来源迁移与该宿主，不代表所有成员已接入。
- 52 项自动测试和 WSL 发现检查不代表 Windows 原生 Codex、其他成员机器或所有未来 worktree 已验证。

原始试点输出、发现前后结果、独立使用记录留在忽略目录 `.local/`；共享仓库保留此稳定摘要和可重跑脚本。

历史来源准备结果：`.local/teamai-pilot/result.json`、`.local/discovery-after-ai-native-migration.json`。正式切换的发现结果：`.local/discovery-after-formal-cutover.json`。这些文件未提交；重跑入口见仓库 README。
