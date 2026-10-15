# ai-native

本仓库汇集团队的 AI 原生实践与学习资料，并管理共享 Skills、Rules、分发版本和必需技能检查。业务规格、Plan、Review、Evidence 与 Gitee 工具继续保存在 `ming-iot-rd-docs`。

共享源使用已有的 [ming-industry/ai-native](https://gitee.com/ming-industry/ai-native)，本机 `origin` 为 `https://gitee.com/ming-industry/ai-native.git`，已通过 Git 确认可以读取。2026-10-04 接入前的仓库 API 查询返回 404，私有可见性和团队成员权限仍未核实；Git 可读取不替代这两项检查。

`skills/map-delivery` 已成为正式团队源码（`canonical`），来源迁移绑定 [ming-iot#IKJMKJ](https://gitee.com/ming-industry/ming-iot/issues/IKJMKJ)。文档仓 [迁移 PR !11](https://gitee.com/ming-industry/ming-iot-rd-docs/pulls/11) 已合入，原跟踪副本已删除。当前 WSL 工作区、产品仓和文档主树的六个 Codex/Claude 入口已切换至本仓库，必需技能检查与 Codex 实际发现通过。精确版本与验证范围见 [验证记录](docs/validation.md)；团队成员安装另行验证。

## 现有本地学习资料（尚未发布）

本机已有 `Steps of AI Adoption.md`，以及 `docs/skills/` 下的 map-delivery、Matt Pocock 和 STE100 三份指南。这些资料已保留，目前仍是未跟踪文件，本次接入没有把它们纳入提交；共享导航须待资料另行发布后建立。`docs/skills/` 是学习文档，实际可分发技能以根 `skills/` 为准。完整 TeamAI pull 会镜像远端已提交的根 `docs/` 内容；上述指南将来提交后也会作为普通文档下发。

## 仓库验证

注册与检查脚本需要 Python 3.11+ 和 Git。直接注册使用目录符号链接；Windows 环境应先验证链接权限，或使用 TeamAI 的复制分发并运行同一内容检查。安装器要求相关工作树的有效 Git 目录位于所选工作区内。

```bash
python3 -m unittest discover -s tests -v
python3 tools/team_skills.py validate
```

技能内容变更后，在评审通过的内容上运行 `python3 tools/team_skills.py lock` 更新 `skills.lock.json`。检查使用锁定的完整文件列表和 SHA-256，不因 TeamAI 的个人排除设置放宽必需技能要求。

当前 profile 的 YAML 头部只接受唯一的 `name`、`description` 两个非空单行字符串字段，支持普通文本、单引号和 JSON 兼容双引号。多行值、嵌套元数据及其他 YAML 构造明确拒绝；需要这些语法的新技能须先扩展并验证解析器。

## 直接注册与检查

成员克隆本仓库和文档仓，在选定工作区注册相关项目。以下示例在工作区根执行；路径由成员自己的目录布局决定。

```bash
python3 ai-native/tools/team_skills.py install \
  --workspace-root "$PWD" --project "$PWD/ming-iot" \
  --project "$PWD/ming-iot-rd-docs"
python3 ai-native/tools/team_skills.py check \
  --workspace-root "$PWD" --project "$PWD/ming-iot" \
  --project "$PWD/ming-iot-rd-docs" --json
```

安装入口是 `.agents/skills/map-delivery`，指向本仓库技能目录。源代码随 Git 分享，本机链接通过 Git 本地排除规则处理。已存在但来源不同的入口会报冲突，不会覆盖。

已合入的文档仓 `tools/setup_workspace_skills.py` 保留兼容入口，通过 `--skills-repo` 指定 `ai-native`，独立读取来源契约、Git 身份与完整锁库存，不执行来源仓脚本。它为工作区、产品仓和文档仓建立 Codex 与 Claude 的六个入口。迁移已核实的旧文档仓链接时，从工作区根运行：

```bash
python3 ming-iot-rd-docs/tools/setup_workspace_skills.py \
  --workspace-root "$PWD" --product-repo "$PWD/ming-iot" \
  --skills-repo "$PWD/ai-native" --migrate-known-source \
  --legacy-docs-root "$PWD/ming-iot-rd-docs"
```

先更新文档主树至已合入版本，使跟踪原件已删除。迁移选项只接受已核实旧来源的符号链接；未知来源和普通目录仍报冲突。同一迁移命令可以重复执行。

TeamAI 安装的完整副本也可通过同一 `check` 检查。检查不安装或删除技能，不访问远端。它检查本机文件与适用的 Codex 禁用配置；不能证明宿主已发现技能或模型确实阅读技能。安装后在对应项目的新聊天中确认可用技能列表；必要时重启 Codex。

安装环境中有 `codex` 命令时，可查询真实宿主的只读发现接口：

```bash
python3 ai-native/tools/check_codex_discovery.py \
  --cwd "$PWD" --cwd "$PWD/ming-iot" --cwd "$PWD/ming-iot-rd-docs" \
  --expected-source "$PWD/ai-native/skills/map-delivery"
```

此命令调用本地 `skills/list`，不启动模型轮次。来源切换后，当前工作区、`ming-iot` 与文档主树均发现启用的 `ai-native` 正式版，扫描错误数为 0。桌面界面显示、其他成员或新聊天的加载仍须分别确认。

## 定位文档 worktree

在工作区根直接使用已准备的团队源定位脚本：

```bash
python3 ai-native/skills/map-delivery/scripts/resolve_docs.py \
  --docs-root "$PWD/ming-iot-rd-docs" --json
```

明确传入当前交付所选的文档 worktree。解析器只读取 Git 身份和所需入口，不访问 Gitee，不创建交付文档。解析成功后，在返回目录中读取根及模块 `AGENTS.md`，再执行文档仓工具。

## TeamAI 试点

固定使用 `teamai-cli@0.26.0`；它需要 Node.js 20 或更新版本。实际安装路径和初始化副作用以该发布包的试点结果为准。试点脚本在隔离目录使用独立宿主配置与本地 Git 远端，不使用真实账号或团队远端。

```bash
python3 tools/run_teamai_pilot.py
```

正常成员接入的完整步骤见 [TeamAI 接入](docs/teamai.md)。TeamAI 负责同步，项目检查负责必需技能完整性，受保护分支的必需 CI 检查负责交付结果。个人排除、断网或同步失败不能被解释为安装成功。

## 已完成迁移与后续接入

团队源先以 `source-ready` 发布，经独立审查后，文档仓删除原件的 PR 合入；主树更新和当前 WSL 来源切换验证完成后，状态改为 `canonical`、`migration_complete: true`。本次完成范围是来源迁移与当前宿主，精确版本和时间保存在 [验证记录](docs/validation.md)。

团队成员接入时，核实仓库可见性与访问权限，在各自工作区和 worktree 安装并检查。将必需技能检查接入团队开发入口；文档与代码结果由 PR 必需检查约束。

来源迁移不改写已批准交付的业务语义。文档管理 IKEOFY Plan 中既有的 Superpowers 执行依赖应在实际执行该计划时按批准范围处理。

本地安装状态不能由远端 CI 证明。CI 应安装固定版本到自己的执行环境，校验技能及交付结果；成员机器的安装由本地检查和宿主发现验证完成。

## 来源

- [Codex 本地技能发现和符号链接](https://learn.chatgpt.com/docs/build-skills)
- [Tencent TeamAI](https://github.com/Tencent/teamai-cli)
- [TeamAI 使用指南](https://github.com/Tencent/teamai-cli/blob/main/docs/usage-guide.zh-CN.md)
- [来源版本](provenance.json)
- [已完成验证与边界](docs/validation.md)
