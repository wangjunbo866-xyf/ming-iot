# 单仓本地与 CI 验证

适用 Linux amd64，Go 1.25.4、Python 3.12、Git、Make、curl、unzip、ripgrep（`rg`）、Docker Compose v2。`verify-pr` 仅用 Compose 校验配置，不启动服务；Kafka、数据库、设备测试仍需单独运行并记录。

CI 从 Ubuntu 软件源安装 ripgrep。部署静态扫描只把退出码 1（无匹配）视为通过；发现禁止内容、工具缺失或读取失败均使检查失败。两条扫描实际完成后才输出 `Things baseline safety scans PASS`。

现有 Things baseline 测试构造 0755 的公开模板目录，验证进程使用 umask 022；私有证据父目录仍保持 0700。

从干净的完整 clone 根目录执行：

```sh
make ci-tools
make publication-check
umask 022
make verify-pr
.cache/ci-tools/actionlint -shellcheck= .github/workflows/monorepo.yml
```

`ci-tools` 从官方发行包下载固定版本及校验和，仅写入忽略的 `.cache/ci-tools`。Protobuf 工具从空 `backend/bin/protobuf-tools` 自动安装，版本/摘要由现有 lock 固定；允许使用 Go 模块与编译缓存，但无需任何旧源仓文件。

GitHub workflow 使用普通 pull_request 和 main/master push、只读 contents 权限、完整历史 checkout、关闭凭据持久化。Actions 固定到完整提交；没有部署、发布、合并或写权限任务。可读的固定版本来源为 [checkout](https://github.com/actions/checkout)、[setup-go](https://github.com/actions/setup-go)、[setup-python](https://github.com/actions/setup-python)、[Gitleaks](https://github.com/gitleaks/gitleaks)、[actionlint](https://github.com/rhysd/actionlint)。Hosted Actions 的实际结论绑定 PR Head 和 run/attempt；本地通过不证明远程通过。分支保护配置单独核验，不由一次 CI 通过推定已启用。

`publication-check` 拒绝浅克隆和未提交候选，扫描 HEAD 与所有本地 refs 的完整可达对象（包含已删除文件的旧版本、提交/标签消息），检查每个提交的路径、gitlink、符号链接及保留许可摘要。它按字节模式扫描所有 blob；ZIP/GZIP/TAR 按每层字节识别并展开，固定扫描名避免原成员名触发忽略规则。超过 4 层、单成员 64 MiB、累计展开 512 MiB 或 10000 成员、加密/链接成员及其他已识别归档格式均失败。不会向磁盘提取原成员路径，不做图片 OCR 或法律权属判定。所有原始扫描内容临时存放在私有目录，输出只有对象 ID、规则与行号。

误报例外只允许已复核的完整内容 SHA-256、规则、行号和原因；内容改变即失效。不使用目录级排除、扫描 baseline、行内放行或被扫描仓库提供的 Gitleaks 配置。维护例外需独立复审。

公开前另运行 `make publication-ready`。它在发布清单不是 ready 或存在待决项时失败，不因技术 CI 通过而改变。25文件范围及本次远程授权已记录；已有来源限制继续保留。许可清单与发布状态的权威是 [PUBLICATION_POLICY.json](../meta/PUBLICATION_POLICY.json)。

迁移采用独立的新历史；M3 及其完整源历史保留在已恢复验证的私有备份中。全新 clone 验证只针对本轮拟公开 refs，不把私有历史一并传入。候选发生变化后，重新绑定完整 Head、验证和独立 Review。
