# TeamAI 试点与正式接入

`ai-native` 保留 AI 原生实践与学习资料，同时通过普通脚本和腾讯 TeamAI 分发团队技能。技能放在 `skills/<name>/SKILL.md`，团队规则放在 `rules/`，业务规格与交付记录仍在研发文档仓。团队源为已有的 `https://gitee.com/ming-industry/ai-native.git`，当前状态为 `canonical`；本次验证范围是来源迁移与当前 WSL 宿主，成员安装、仓库私有可见性和权限分别验证。

来源迁移交付为 [ming-iot#IKJMKJ](https://gitee.com/ming-industry/ming-iot/issues/IKJMKJ)。团队源发布后，文档仓 [PR !11](https://gitee.com/ming-industry/ming-iot-rd-docs/pulls/11) 于 2026-10-04 15:01:55（Asia/Shanghai）合入，主树删除跟踪原件。当前 WSL 的六个 Codex/Claude 入口已切换，必需技能检查和三个目录的 Codex 发现均通过；精确来源与合入版本见 [验证记录](validation.md)。此切换通过普通注册脚本完成，真实成员 TeamAI init 仍另行接入。

## 普通脚本接入与旧入口迁移

从工作区根运行已合入文档仓的兼容入口；它独立读取来源契约、Git 身份及完整文件锁，不调用外部 Python 校验脚本：

```bash
python3 ming-iot-rd-docs/tools/setup_workspace_skills.py \
  --workspace-root "$PWD" --product-repo "$PWD/ming-iot" \
  --skills-repo "$PWD/ai-native" --migrate-known-source \
  --legacy-docs-root "$PWD/ming-iot-rd-docs"
python3 ai-native/tools/team_skills.py check \
  --workspace-root "$PWD" --project "$PWD/ming-iot" \
  --project "$PWD/ming-iot-rd-docs" --json
python3 ai-native/tools/check_codex_discovery.py \
  --cwd "$PWD" --cwd "$PWD/ming-iot" --cwd "$PWD/ming-iot-rd-docs" \
  --expected-source "$PWD/ai-native/skills/map-delivery"
```

迁移前先更新文档主树至已合入版本，确认跟踪原件已删除。`--migrate-known-source` 只接受已核实旧文档仓来源的符号链接，并允许原样重复命令；未知来源和普通目录保持冲突。Windows 符号链接权限受限时，使用 TeamAI 复制分发后运行必需检查；原生 Windows 尚未在本次验证中覆盖。

## 固定版本

- CLI：npm `teamai-cli@0.26.0`，2026-09-29 发布。
- 发布元数据 `gitHead`：`96e5331a2ea5dded15c866c4eaac946602bb0702`。
- CLI tarball SHA-512：`PsXmc8S7e6b88IK3Ze411q2Oj6i/2pl4FNg1t3WkXgLN2ndSY9H15Wwxg0KRjkbIaXH0tgNlsBlAq/N1DkhAug==`。
- 隔离试点使用 `node-linux-x64@22.21.1`，不修改系统 Node。脚本在 Linux x86_64 上运行。

核对日期为 2026-10-04。GitHub 当日 `main` 的 `package.json` 仍显示 `0.22.0`，与 npm latest 不一致；这里按照发布版本的 `gitHead` 检查行为。固定 CLI 版本不等于固定所有传递依赖，试点目录保留 npm lockfile 供审计。

## 当前配置

`teamai.yaml` 明确指定 Codex 技能安装到 `.agents/skills`。0.26.0 默认是 `.codex/skills`；仅当共享目录中已经存在对应技能时才自动优先使用 `.agents/skills`。团队配置显式指定路径可避免首装进入旧目录。

当前配置关闭自动升级、usage 上报、环境变量写入 shell profile、recall、分享提示、webhooks、团队 hooks 和 MCP 自动应用。没有模型、MCP、env、外部 sources、packages 或 post-pull scripts 配置。Codex `toolPaths` 只声明技能与规则路径，未声明 settings、hooks、MCP 或 agents 路径。`hooks/hooks.yaml` 另外禁用六个内置生命周期 hook，作为以后配置 hook 路径时的防护，因为 `sharing.hooks.autoApply: false` 单独只能控制团队 hooks。

0.26.0 的 doctor 在声明 Codex settings 路径后，会要求实际存在 TeamAI hooks，忽略全量禁用内置 hooks 的配置。当前手动同步模板省略 settings 路径，匹配实际不部署 hooks 的行为；没有掩盖 doctor 的检查失败。

这是一份手动同步试点配置。需要执行 `teamai pull` 才会更新技能，不能宣称每次会话自动同步。规则文件会下发到 `.codex/rules`；关闭 hooks 后，不能仅凭文件存在就认定 Codex 已加载这些规则。需要强制告知 agent 的规则继续通过各仓库已有 `AGENTS.md` 提供。

完整 TeamAI pull 也会镜像团队仓库根 `docs/` 的学习与接入文档，包括 `docs/skills/` 中的指南；它们作为普通文档下发，不会自动成为技能。技能安装只从根 `skills/` 获取。当前迁移后的隔离脚本仍只复制 `skills/`、`rules/`、`hooks/` 和团队配置，没有验证根 `docs/` 的镜像。`docs/skills/` 三份原有指南仍是本机未跟踪文件，保留但未自动加入提交；只有另行提交到远端后才能被远端消费者拉取。所有业务文档仍放在研发文档仓。

## 隔离试点

从本仓库运行：

```bash
python3 tools/run_teamai_pilot.py
```

脚本会：

1. 在 `.local/teamai-pilot/` 建立独立运行目录、用户 HOME、Codex HOME、npm cache 和 Git 配置。
2. 核对 npm 元数据和实际 CLI tarball 的固定完整性值，再以 `--ignore-scripts` 安装固定 CLI 与隔离 Node。
3. 创建本地 source、bare origin 和 consumer 仓库。合成 HTTPS URL 通过隔离 Git 配置映射至本地 bare origin，`GIT_ALLOW_PROTOCOL=file` 阻止真实 Git 网络传输。
4. 执行真正的 project `init`、`--dry-run pull`、`pull` 和 `doctor --json`。
5. 逐个核对 `map-delivery` 的源码文件与安装副本，记录 dry-run 变化、成员登记分支、CLI 版本和 doctor 结果。

结果保存在 `.local/teamai-pilot/result.json`；具体日志目录由结果给出。注册成员、Git push 和用户工具配置的所有副作用都留在隔离目录。本机真实 Codex 配置、团队远端和系统 Node 没有被初始化或修改。

0.26.0 的 `--dry-run pull` 保持部署到 consumer 项目的资源不变，但仍会更新隔离 HOME 中的 debug 日志，以及克隆的 Git `FETCH_HEAD` / `ORIG_HEAD` 元数据。这里把这些变化逐项记录；预演不意味着严格零文件写入。

本机有 Codex CLI 时，脚本还会在同一隔离 HOME 下调用 `app-server skills/list`，验证 consumer 项目的技能能被实际扫描，且路径与安装副本相符；不会启动模型回合。结果中的 `codex_discovery` 为 `null` 时，这项没有验证。扫描成功不等于当前已打开聊天已经重载技能列表。试点不证明私有 Gitee 的权限，也不证明 agent 实际阅读了技能。

## 正式仓库接入

采用已有的 `ming-industry/ai-native`。本机 Git `origin` 与 `teamai.yaml` 的 `repo` 均指向 `https://gitee.com/ming-industry/ai-native.git`。已发布的技能来源版本见验证记录；成员接入前，在 Gitee 核对实际仓库可见性、管理员资产维护权限和成员拉取权限，并确定是否允许成员登记分支。

### 当前远端观察

2026-10-04 接入前的 Git `ls-remote` 成功读取远端，HEAD 和 `refs/heads/master` 均为 `05fc076d2ae9b546063ec0c8c109aa5f0a6904f4`。这是发布团队技能候选前的历史观察，不是推送后的最新 Head。这证明该 Git 远端存在且当前环境可以读取，不能证明私有可见性、写权限或其他成员的权限。

同日接入前的仓库 API 查询返回 404；这不覆盖 Git 的读取结果，也不能用来断言仓库不存在或已确认私有。由有权限的组织成员核实 Gitee 设置及访问权限后，记录实际结论。当前没有在真实宿主执行 TeamAI init，也不把本地试点结果写成成员已接入。

在采用合适 Node 版本的开发环境中安装固定 CLI：

```bash
npm install -g teamai-cli@0.26.0
```

在各消费项目目录内执行，仓库 URL 使用已核实的现有 Git origin；执行前仍须完成上述权限核对和技能发布：

```bash
TEAM_AI_REPO_URL=https://gitee.com/ming-industry/ai-native.git
teamai init "$TEAM_AI_REPO_URL" --provider git --agent codex --scope project --no-inherit-user-scope
teamai --dry-run pull
teamai pull
teamai doctor --json
```

上述正式命令是供管理员和成员接入时使用的操作步骤，当前试点没有在真实宿主或真实远端执行它们。0.26.0 的 init 不会立即执行完整资源 pull，因此必须显式运行 pull。

`--provider git` 使用现有 Git SSH/credential helper，支持 Gitee 的 Git transport；它没有 Gitee 原生 PR API。`teamai push` 可以推送分支，但自动创建 PR 不受支持，此时命令可能非零退出，需要在 Gitee 网页上创建 PR。

正常 init 会创建本机配置、部署内置 `teamai` 技能，并尝试把成员登记推送到 `teamai-reports` 孤儿分支。远端缺少团队配置时，还会生成骨架并尝试推送默认分支。`usageReport: false` 不会阻止 init 的成员登记。因此先发布配置完整的仓库，再让成员 init；只读拉取权限下，成员登记 push 可能失败，CLI 会继续本地安装并告知失败。

普通 project scope 的资源写到项目，但 lifecycle hooks 的设置会写到用户 HOME 的工具目录。即使选择 project scope，也不能据此声称没有用户级配置副作用。当前模板禁用所有生命周期 hook；未来打开自动同步时，必须单独核对 Codex hook 加载、信任和上报行为。CLI 0.26.0 不会自动修改 Codex trust 状态。

安装后在相应项目的新 Codex 聊天中确认 `map-delivery` 出现在可用技能列表。worktree 按实际工作目录再次注册或验证；不要仅凭另一个 clone 的安装结果判断新 worktree 已完成。

## 必须安装与合并检查

TeamAI 允许本地 `excludedSkills`，doctor 按用户过滤后的预期资源检查，因此它不能单独保证必需技能存在。项目启动入口应执行本仓库提供的必需技能检查，检查来源、引用文件、版本和配置；必需技能不允许通过 exclude 绕过。

本机检查负责文件安装状态。CI 负责交付文档和代码结果，例如研发文档仓 `map_docs.py check`。CI 不能证明成员机器安装过技能，更不能证明模型阅读了技能。

## 官方依据

- [发布包元数据](https://registry.npmjs.org/teamai-cli/0.26.0)
- [0.26.0 配置 schema 与默认路径](https://github.com/Tencent/teamai-cli/blob/96e5331a2ea5dded15c866c4eaac946602bb0702/src/types.ts)
- [0.26.0 init 副作用](https://github.com/Tencent/teamai-cli/blob/96e5331a2ea5dded15c866c4eaac946602bb0702/src/init.ts)
- [0.26.0 pull 与上报](https://github.com/Tencent/teamai-cli/blob/96e5331a2ea5dded15c866c4eaac946602bb0702/src/pull.ts)
- [Codex 共享技能目录处理](https://github.com/Tencent/teamai-cli/blob/96e5331a2ea5dded15c866c4eaac946602bb0702/src/resources/skills.ts)
- [通用 Git provider 能力](https://github.com/Tencent/teamai-cli/blob/96e5331a2ea5dded15c866c4eaac946602bb0702/src/providers/git/index.ts)
- [内置 hook 禁用配置](https://github.com/Tencent/teamai-cli/blob/96e5331a2ea5dded15c866c4eaac946602bb0702/src/builtin-hooks.ts)
- [官方使用说明](https://github.com/Tencent/teamai-cli/blob/96e5331a2ea5dded15c866c4eaac946602bb0702/docs/usage-guide.zh-CN.md)
