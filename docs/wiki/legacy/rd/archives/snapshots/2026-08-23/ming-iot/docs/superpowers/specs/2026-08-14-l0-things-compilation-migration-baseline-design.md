# L0 Things 可编译迁移基线设计

状态：已确认并启动
日期：2026-08-14

## 1. 背景

`ming-iot` 当前已有 `backend/saas` 与 `backend/tools`，但还没有 Things 业务模块。后续物模型、Kafka 数据面、Reducer、Current Fact 和 Scene 都依赖一个可编译、可验证的 Things 控制面基线，因此不能直接从 Scene 开始实现。

联犀官方 `things` 仓库只作为只读来源。该仓库本地 `master` 比官方 `origin/master` 多两个既存提交；L0 不移动、不重置也不修改本地分支，只读取固定 Git 对象：

- 源仓库：`/home/wangjunbo/workspace-iot-platform/things`；
- 固定 ref：`refs/remotes/origin/master`；
- 固定 commit：`243ab1aa03b8a3f106761a81cdd9bd8827d448c8`；
- 固定 tree：`544b638ae3ee5e5f3a041bea22ca035dbb7e5859`；
- 源 module：`gitee.com/unitedrhino/things`；
- 目标 module：`gitee.com/ming-iot/things`；
- 本地 SaaS module：`gitee.com/ming-iot/saas`；
- 外部 Share：`gitee.com/unitedrhino/share v1.6.4`。

## 2. L0 目标

L0 交付一个受控迁移基线，而不是最终 Things 架构：

1. 从固定 commit 的精确 allowlist 生成 `backend/things`；
2. 保留现有 API、RPC、生成代码、测试和三个生产入口；
3. 把 Things 自引用改为 `gitee.com/ming-iot/things`，把 Core 引用改为本地 `gitee.com/ming-iot/saas`；
4. Share 继续作为外部不可变依赖，并把 SaaS 与 Things 对齐到 `v1.6.4`；
5. 在任何源文件进入目标 Git 历史前完成凭据、私钥和危险制品净化；
6. 提供可重复导入、逐字节验证、构建门禁和机器可读证据；
7. 让 L0 成为后续 Loop 可复刻的第一个 A1 级自动化样板。

## 3. 非目标

L0 明确不做：

- 不把 NATS 改成 Kafka；源码中的 NATS 行为原样保留，Kafka 由 L3 处理；
- 不采用联犀物模型作为最终物模型，模块级超级表与 `ModelRelease` 由 L2 处理；
- 不实现 Raw→Canonical、Materializer、Current Fact、Reducer、Command Outbox 或 Scene；
- 不改变数据库表、HTTP 路由、protobuf wire contract 或业务流程；
- 不重新生成 `.api`、protobuf、RPC client/server；
- 不迁移旧 deploy、Dockerfile、CI、脚本、shell、文档资产、手工压测或辅助工具；
- 不部署这个遗留兼容基线到生产；
- 不修改、提交或推送源 `things` 仓库；
- 不在自动流程中执行 `git commit` 或 `git push`。

## 4. 精确输入边界

固定 commit 共 1,183 个 tracked 文件。L0 只读取下列 976 个文件，共 4,866,935 blob bytes：

```text
LICENSE
go.mod
go.sum
sdk/protocol/**
service/apisvr/**
service/dgsvr/**
service/dmsvr/**
share/**
```

该选择集的未压缩 Git archive：

```text
bytes: 5754880
sha256: 32b998e4621dd88a39b618552187cfbd1be910851925702a07c9e4c82cb4d486
```

其余 207 个 tracked 文件全部排除：

```text
.github/**
.gitignore
AGENTS.md
CLAUDE.md
Dockerfile
Makefile
README.md
README.en.md
deploy/**
doc/**
script/**
shell/**
tests/**
thingsEEsvr-linux-amd64
tools/**
```

排除范围包含 83,128,504-byte ELF、3 个 `.env`、私钥/证书文件、旧部署配置和手工压测。目标 README、NOTICE、PROVENANCE 和 Makefile 由本项目生成，不复制上游版本。

## 5. 保留的运行与契约边界

三个生产入口保持不变：

| 产物 | main package | 入口文件 |
| --- | --- | --- |
| `thingsvr` | `./service/apisvr` | `service/apisvr/api.go` |
| `dgsvr` | `./service/dgsvr` | `service/dgsvr/dg.go` |
| `dmsvr` | `./service/dmsvr` | `service/dmsvr/dm.go` |

必须保留：

- 34 个 `.api` 文件；
- 4 个 `.proto` 文件；
- 7 个已提交 protobuf 生成文件；
- `service/dgsvr/client/**`、`service/dgsvr/internal/server/**`；
- `service/dmsvr/client/**`、`service/dmsvr/internal/server/**`；
- `share/rpcs/protocolSync/**`；
- `service/dmsvr/internal/logic/protocolmanage/script/{before,downAfter,upAfter}` 三个 `go:embed` 资源；
- allowlist 内 23 个 `_test.go` 文件。

L0 不重新生成这些文件。验证器从固定 commit 重新渲染候选树并与目标逐字节比较，所以除已声明转换外，API/RPC/生成物的任何变化都会失败。

## 6. 确定性迁移器

项目新增 `backend/tools/cmd/thingsbaseline` 和 `backend/tools/internal/thingsbaseline`，提供两个子命令：

```text
thingsbaseline import --lock LOCK --source SOURCE --backend BACKEND
thingsbaseline verify --lock LOCK --source SOURCE --backend BACKEND --evidence FILE
```

renderer 明确区分两种意图，不用一个含糊的“目标状态”同时服务 import 与 verify：

- **publish candidate**：`import` 使用，只允许 `backend/things` 不存在；完成后可通过同一已打开 backend 目录句柄执行 Linux `renameat2(RENAME_NOREPLACE)` 原子发布；
- **verification candidate**：`verify` 使用，只允许既有 target 是 backend 下名为 `things` 的真实直属目录；handle 从创建时即永久禁用 `Publish()`，只能读取报告和清理。

两种意图都要求原始 `SourceSnapshot` 先在内存中完成全部 module/security/vet 转换，未经净化的凭据和私钥不得写入任何 candidate；随后才在 `backend/.thingsbaseline-*` 创建候选目录并完成 tidy、最终扫描和 digest。publish candidate 遇到目标已存在、并发创建或 `EXDEV` 均 fail closed，禁止回退为覆盖 rename 或 copy。

Render 失败自行清理 candidate；成功返回 opaque handle 并冻结内容。`verify` 必须创建**两个**使用独立 HOME/cache 的 verification candidate，始终 defer cleanup，并要求两份 pre/final manifest、tree digest、转换/凭据/scanner 结果完全一致；受控命令的名称与退出码也必须一致，但下载顺序可能导致 stdout/stderr hash 与 tail 不同，它们属于各轮独立保存的观察日志，不参与产物可复现判定。然后以 candidate 为期望值，对现有 `backend/things` 做 fd-relative、no-follow 的只读扫描和逐路径比较。`import` 发布成功后 disarm cleanup，发布失败则清理 candidate。初始不存在的 target 在任意失败后仍不存在，预存 target 在 verify 前后必须逐字节、mode 和路径集合不变。

两条命令都读取 `backend/tools/things-baseline.lock`。lock 固定：

- repository、ref、commit、tree；
- 精确 include/exclude 根；
- 文件数、blob bytes、archive checksum；
- module 前缀转换及预期次数；
- 安全净化规则及预期次数；
- 目标模板和验证门禁版本。

迁移器通过 `git ls-tree` 与 `git cat-file` 读取 commit 对象，不依赖源 worktree 的 `HEAD`，也不要求本地分支与官方分支相同。所有生产 Git 命令丢弃继承的 `GIT_*` 环境，只加入固定 allowlist：关闭 replace object 与 optional locks、使用 literal pathspec、隔离 system/global Git 配置和 system attributes，并固定 locale；非 Git 环境及 `HOME` 保留。ref 只能通过 `show-ref --verify` 按完整名字解析。每个 blob 在读取后按 Git blob 对象格式重算 SHA-1，并校验 object ID；archive 采用流式字节计数和 SHA-256 校验，不把完整 tar 缓存在内存。读取前后 `.git/index` 必须 byte-identical。

源读取器把稳定后的 `HEAD`、ref 和包含全部未跟踪文件的 worktree status SHA-256 作为不含路径/正文的 `SourceRepositoryState` 返回；verifier 必须复用同一 Git plumbing API 在全部 gate 结束后再次采样，禁止自行拼一套 Git 命令。开始、ReadSource 结束和最终结束三者任一变化都使运行失败。该前后比较是用于发现用户工作区并发漂移的 best-effort 证据，不承担输出原子性的证明；固定 commit/tree、禁用 replace object、逐 blob object ID 与 archive 哈希才共同保证迁移输入的内容完整性，因此不对官方源码仓库加写锁。

迁移器拒绝：

- 非 40 位固定 SHA、ref 漂移或 tree 漂移；
- allowlist 以外路径、路径穿越、绝对路径、symlink 或特殊文件；
- 文件数、blob bytes、archive checksum 不匹配；
- 目标已存在、候选目录不在 backend 直属子目录；
- 预期转换次数不匹配；
- 目标中出现旧 Core/Things module 前缀、敏感文件或未声明差异。

## 7. 受控转换

### 7.1 Module 与 import

对 `go.sum` 之外的 UTF-8 源文件执行两个精确转换：

```text
gitee.com/unitedrhino/things -> gitee.com/ming-iot/things
gitee.com/unitedrhino/core   -> gitee.com/ming-iot/saas
```

固定快照预期分别命中 2,726 次和 94 次；计数以转换前的非 `go.sum` 原始 blob 为准。Core 的 94 次中包含 `go.mod` 顶部 1 条旧本地路径提示，该提示作为声明过的 go.mod 规范化被删除，仍计入已消费的源命中。Share 前缀不做全局改写。

目标 `go.mod` 保留上游依赖版本轴，把 Core require 改为 SaaS require，并增加：

```go
replace gitee.com/ming-iot/saas => ../saas
```

相对 replace 使 `backend/things` 在 `GOWORK=off` 下也能解析同仓 SaaS；禁止绝对路径。候选目录必须是 `backend` 的直属子目录，确保候选执行 `go mod tidy` 时同样解析到 `backend/saas`。

`go.sum` 不做字符串替换，由候选目录内的受控 `go mod tidy` 重建。权威渲染固定 `go1.25.4`，并使用新的私有 `HOME/GOCACHE/GOMODCACHE`；固定 `GOENV=off`、空 `GOFLAGS`、`GOWORK=off`、`GOTOOLCHAIN=local`、`GOPROXY=https://goproxy.cn,direct`、`GOSUMDB=sum.golang.org`，清空 private/no-proxy/no-sumdb/insecure 配置并丢弃全部继承的 `GO*`、`GIT_*`。每次权威渲染使用独立 cache，网络、checksum 或 toolchain 不匹配均失败并清理 candidate。tidy 后执行 `go mod verify`，且不得残留 `gitee.com/unitedrhino/core`。

> **缓存策略更新（2026-08-19）：** 本段的“每轮独立私有 `GOCACHE/GOMODCACHE`”要求已由
> [Thingsbaseline 共享 Go 缓存设计](./2026-08-19-thingsbaseline-shared-go-cache-design.md)替代。
> candidate、`HOME`、`GOTMPDIR`、workspace 和构建输出仍逐轮隔离；只有受信 Go 解析出的
> `GOCACHE/GOMODCACHE` 跨轮复用，所有测试显式使用 `-count=1`。

上游 `go.mod` 顶部两条注释掉的 Core/Share 本地路径提示必须恰好删除；迁移后它们会指向错误或不存在的同级目录。module 改写后必须由 `golang.org/x/mod/modfile` 解析有效 directive：新增前 replace 必须恰好只有两条固定 `cloud.google.com/go` path+version，不允许注释诱饵、额外/重复 replace 或已有 SaaS replace。L0 再结构化新增一条 SaaS 相对 replace，不借迁移机会改写其他依赖决策。

### 7.2 配置凭据净化

固定快照的 9 个 YAML 文件内有 55 行 credential-shaped 配置，其中 active 35 行、commented example 20 行。迁移器按完整语义路径将值替换为环境变量，注释示例也不得保留字面量：

| 语义 | 目标值 |
| --- | --- |
| `Database.DSN` | `${THINGS_DB_DSN}` |
| `TSDB.DSN` | `${THINGS_TSDB_DSN}` |
| `DevLink.OpenApi.ApiKey`、`DevLink.Mqtt.OpenApi.ApiKey` | `${THINGS_EMQX_API_KEY}` |
| `DevLink.OpenApi.SecretKey`、`DevLink.Mqtt.OpenApi.SecretKey` | `${THINGS_EMQX_SECRET_KEY}` |
| `OssConf.AccessKeyID` | `${THINGS_OSS_ACCESS_KEY_ID}` |
| `OssConf.AccessKeySecret` | `${THINGS_OSS_ACCESS_KEY_SECRET}` |
| `AuthWhite.Users[].Password` | `${THINGS_INTERNAL_AUTH_PASSWORD}` |

完整分布作为 lock 的具名 struct 字段固定为：`Database.DSN=10`、`TSDB.DSN=18`、`OssConf.AccessKeyID=3`、`OssConf.AccessKeySecret=3`、`AuthWhite.Users[].Password=11`、`DevLink.OpenApi.{ApiKey,SecretKey}=1+1`、`DevLink.Mqtt.OpenApi.{ApiKey,SecretKey}=4+4`；路径总和、active+commented 与 total 三者必须同为 55。

active YAML 由 `yaml.v3` AST 发现全部敏感 mapping key，再与只支持安全 block 重写的词法器逐项交叉，`Users` 下列表规范化为 `Users[]`。quoted key、flow mapping/sequence、alias、空值错误父路径或 AST/lexer 不一致均 fail closed。commented example 由独立保守 scanner 发现敏感 key，移除一个注释前缀后计算逻辑缩进；quoted/flow/TAB 等无法安全保持格式的写法拒绝，支持的输出保留原缩进和注释形式。错误不得包含原始值。

上游 `share/utils.ConfMustLoad` 在解析 YAML 前执行 `os.ExpandEnv`，无需引入新的配置加载器。转换后必须再次以 AST 和独立 scanner 按完整语义路径扫描，9/35/20/55、九路径分布完全匹配 lock，且每行只能是该路径对应的精确占位符；空值也不允许。

### 7.3 内嵌私钥净化

`service/dgsvr/internal/logic/deviceauth/loginAuthLogic.go` 含一个内嵌 RSA 私钥。迁移器不得把私钥正文复制到 lock、patch、测试 fixture、日志或目标历史。

转换器必须在任何 module 改写之前，先对原始 source blob 校验固定 SHA，然后只按声明边界删除 `clientKey` 块。公有测试证书继续逐字节保留；初始化逻辑改用 `encoding/pem` + `x509.ParseCertificate`。PEM 无法解码或证书无法解析时不得索引空切片或解引用 nil，只跳过仅用于诊断日志的 signature/raw 比较。注册放行、身份一致性、缓存读取、密码比较及其放行/拒绝条件不改变。

目标树中 `BEGIN ... PRIVATE KEY` 必须零命中。

### 7.4 Go 1.25 vet 兼容

源快照在 Go 1.25.4 下：

- `GOWORK=off go test -vet=off -run '^$' ./...`：PASS；
- 默认 vet：因一处 `AddDetailf(strProjectID)` 的动态 format 报错。

转换器同样必须先校验该文件的原始 source blob SHA，再进行任何 module 改写。目标把该调用改成 `AddDetailf("%s", strProjectID)`：旧形式必须恰好 1 次，新形式原先必须为 0，输出反向计数必须为旧 0、新 1。detail 文本和错误决策不变，默认 `go test` 必须通过。

### 7.5 过期 Schema 测试 fixture 兼容

最终全量门禁发现固定上游 commit 的 `share/domain/schema/schema_test.go` 仍使用旧 JSON 字段 `"id"`，而同 commit 的生产模型已经声明 `json:"identifier"`。结果是 `TestTempate` 将 12 个属性、3 个事件和 1 个动作全部解码为空标识，slice-to-map 一致性断言必然失败。这是上游测试数据与生产契约不一致，不是运行环境或迁移 module 改写造成。

L0 只修测试 fixture，不修改生产模型，也不跳过测试。转换器必须在 module 改写前对该单一固定路径执行以下 fail-closed 契约：

- 原始 blob SHA-256 必须为 `0468f6ebf1426919ee8ec1f789861874490e127c279bf8a54daac0c0ddda5791`；
- 原始字节 token `"id":` 必须恰好 45 次，`"identifier":` 必须为 0 次；
- 只把这 45 个 token 改为 `"identifier":`，其他字节保持不变；
- 转换后旧 token 必须为 0、新 token 必须为 45，输出 SHA-256 必须为 `77570823fce1f9393ce992c1303a40fb1c0f6e2b7e2f780d0965c9b4906d0c30`；
- lock 增加 `schemaFixtureIdentifierCount=45`、`schemaFixtureSourceSha256` 和 `schemaFixtureOutputSha256`；公开报告与 evidence 增加 `schemaFixtureIdentifierCount`。

任何路径缺失、输入/输出 SHA 不符、计数漂移、预先混入新 token 或额外正文变化都必须在落盘前失败。独立单元测试要覆盖这些失败边界，并真实执行修正后的 `TestTempate`；最终 `go test ./...` 不允许使用 `-skip`、`-run` 或 `-vet=off` 绕过。

## 8. 安全文件与来源声明

候选生成以下目标自有文件：

```text
backend/things/README.md
backend/things/NOTICE
backend/things/PROVENANCE.yaml
backend/things/Makefile
```

`LICENSE` 从固定 commit 原样复制，SHA-256 必须为：

```text
20b067f86de375aae6db0f283ab2e65de24d537733b89bd58432c101259d84cf
```

四个模板由 `backend/tools/templates/things-baseline/embed.go` 以只读 `embed.FS` 提供，不依赖运行 cwd。模板使用固定 token 集，未知或未替换 token 失败；最终四文件均为 `0644` 并做 byte-level golden。

README 必须固定三个入口、先进入 `backend/things` 后执行的构建/验证命令、L0 兼容基线及禁止直接生产部署。NOTICE 固定来源 URL、完整 commit 和 AGPL-3.0，禁止声明 Apache。PROVENANCE 使用稳定字段顺序和 schema，至少固定 repository/ref/commit/tree、source/target module、Share v1.6.4、license、源/目标文件数与六类转换计数（含 45 个 schema fixture identifier）；禁止时间戳、绝对路径、主机信息和随机 candidate 名。

Makefile 只提供 `build`、三个具名 build target 和精确 `clean`；产物只写入 `backend/bin/{thingsvr,dgsvr,dmsvr}`。禁止 `go mod tidy`、kill、nohup、rsync、远程下载或宽目录删除等副作用。

最终 manifest 必须恰好由 976 个变换后的源路径和 4 个模板路径组成，共 980 个 regular files。tidy 只允许改写既有 `go.mod/go.sum`，不得增删路径或改变其他文件。最终 tree digest 在权限归一和安全扫描后，按排序后的规范相对路径、mode 与内容 SHA-256 做长度前缀 framing；随机目录名、mtime、uid/gid 不参与。

renderer 成功只返回 opaque sealed handle。其公开 `Report()` 返回深拷贝的 pre/final manifest、tree digest、转换/凭据/scanner/tidy/verify 结果，不包含任何本机绝对路径或随机 candidate/cache 名；公开 API 不暴露 candidate 路径或写入口。后续 verifier 只能在同 package 内从 sealed root 创建隔离验证树，外部调用方仅能读取报告、清理；只有 publish candidate 额外允许执行一次原子发布，verification candidate 的 `Publish()` 必须稳定失败。

## 9. Workspace 与依赖

`backend/go.work` 增加 `./things`，最终包含：

```go
use (
    ./saas
    ./things
    ./tools
)
```

`backend/saas/go.mod` 的 `gitee.com/unitedrhino/share` 从 `v1.6.1` 对齐到 `v1.6.4`。这是 L0 唯一允许的 SaaS 依赖升级；必须在 `GOWORK=off` 和 workspace 两种模式回归 SaaS。

L0 不主动统一 Things 与 SaaS 的其他依赖版本。Go Workspace 的 MVS 结果只作为证据记录，后续依赖治理另开 Loop。

## 10. 安全门禁

候选和目标必须同时满足：

- 扩展名大小写不敏感地拒绝 `.pem/.ppk/.key/.p12/.pfx`，并拒绝 `.env` 与 `.env.*`；
- 在读取正文前拒绝大于 10 MiB 的文件；ELF 只按 magic 判断，不按扩展名；
- 私钥 marker 在 scanner 内也由字符串片段组装，候选、目标和迁移器测试源码均零命中，不设 `_test.go` 豁免；
- YAML 凭据复用 Task 2 完整语义扫描，55 行必须分别等于精确环境占位符，不允许空值或宽泛 `${VAR}`；
- `gitee.com/unitedrhino/core` 在全部 980 文件中为零；旧 Things 前缀在 976 个迁移源文件中为零，只允许出现在 byte-golden 已验证的 README/NOTICE/PROVENANCE 固定来源字段；
- `gitee.com/unitedrhino/share` 只作为设计允许的外部依赖；
- backend、candidate、target 均不得是 symlink；树内只允许目录和 regular file，拒绝 symlink、socket、device、FIFO 及 link count 大于 1 的 hardlink；
- candidate 初始为 `0700` 私有目录，exclusive create 写文件；扫描前所有目录归一为 `0755`，文件只能为 `0644/0755`，不依赖 umask；
- exclude 只按上游顶层根判断；`README.md/Makefile` 仅在内容与生成模板 byte-identical 时作为精确例外，嵌套同名目录不误拒绝；
- candidate 和 target 使用同一套 fd-relative/no-follow scanner 核心；scanner 在 read 前后复核 dev/inode、type、mode、size、link count 与目录成员集合，拒绝扫描期间的文件替换、chmod、目录交换或新增；最终扫描和 digest 后 candidate 不得再修改；
- target 比较以 candidate manifest 为期望值，差异只记录排序后的规范相对路径、expected/actual mode 和 SHA-256，不记录正文；差异数量和摘要有固定上限并显式标记 truncated；缺失、多余、mode、内容及任何 symlink/hardlink/FIFO/socket/device 都失败；
- 固定源已有 97 行 trailing-whitespace 基线，因此不做会改变上游语义的全树自动清理。renderer 以 `(path,line,lineSHA256)` 保存原始指纹，最终树只允许仍 byte-identical 的基线项，禁止新增 trailing whitespace；四模板必须零命中；
- `.gitignore` 精确保留 `/backend/bin/`，并新增且只新增 `/.worktrees/`、`/backend/.thingsbaseline-*/`。

## 11. 验证矩阵

### 11.1 迁移器

采用 TDD 覆盖：

- lock 字段、路径和计数校验；
- ref/commit/tree 漂移；
- allowlist 完整覆盖；
- 路径穿越、symlink、特殊文件；
- module 前缀预期次数；
- 55 行配置净化；
- 私钥不进入候选、fixture 和日志；
- vet 修复恰好一次；
- 目标已存在拒绝；
- import 失败不发布；
- verify 零差异与差异报告；
- JSON evidence 稳定字段。

### 11.2 Go 与构建

至少运行：

```text
tools-test       backend/tools 的真实受控 gate
saas-test        backend/saas 的 GOWORK=off 真实受控 gate
things-test      verification tree/things 的 GOWORK=off 真实受控 gate
workspace-test   临时 go.work 显式绑定 backend/saas、backend/tools 与 verification tree/things
things-build     从 verification tree/things 构建三个入口到 backend/bin
```

`verify` 不得在 `backend/things` target 上运行 test、tidy、verify、make 或任何可能产生写入的命令。它先从第二个 sealed candidate 通过 fd-relative/no-follow copy 创建私有 isolation tree，再在该 tree 上运行 Things/Workspace/build gate；target 只允许扫描和比较。临时 go.work、cache、HOME 和 isolation tree 均不得进入 evidence，结束后必须清理。

真实 `backend/go.work` 只登记三个本地 module，禁止对它执行 `go work sync`：Workspace 的 MVS build list 会把 Things 选择的较新依赖反向写入 SaaS/tools，甚至改写生成目标的 `go.mod/go.sum`。三个 module 各自以 `GOWORK=off` 管理和验证依赖；跨 module 兼容性只在 verifier 创建的临时 go.work 中测试。若生成目标被任何外部命令改写，必须由锁定迁移器整树重建，不做手工修补。

三个产物必须存在且可执行。`module-clean` 无条件盘点并比较 `backend/{go.work,go.work.sum}`、`backend/{saas,tools,things}/{go.mod,go.sum}` 的存在性、mode 与 SHA-256；任何 gate 前后差异都失败，不能只依赖 `git status`。

allowlist 内 23 个测试文件没有 build tag，也没有真实外部服务依赖。全量测试必须实际运行；不得把 `-run '^$'` 结果冒充完整测试。

## 12. 机器证据

`thingsbaseline verify` 写入 JSON，至少包含：

```text
schemaVersion / loopId / finalEvidence
startedAt / finishedAt / durationMs
source.repository / ref / commit / tree
source.headBefore / headAfter / refBefore / refAfter
source.statusDigestBefore / statusDigestAfter
source.selectedFiles / selectedBlobBytes / archiveSha256
render1.manifestSha256 / treeSha256 / files / credentialProfile / scan / tidy / verify
render2.manifestSha256 / treeSha256 / files / credentialProfile / scan / tidy / verify
reproducibility.result
target.logicalPath / module / beforeTreeSha256 / afterTreeSha256 / differences / truncated
transform.corePrefixCount / thingsPrefixCount
transform.credentialLineCount / privateKeyBlockCount / vetFixCount / schemaFixtureIdentifierCount
contracts.apiFiles / protoFiles / generatedProtoFiles / testFiles
phases[]: id / result / failureKind / summary
gates[]: id / command / result / execution / nullable exitCode / logSha256 / summary
overallResult
```

`result` 只允许 `PASS | FAIL | NOT_RUN`；进程 `execution` 至少区分 `EXITED | SPAWN_ERROR | TIMEOUT | CANCELLED | NOT_RUN`，spawn/timeout/cancel 时 `exitCode=null`，不得伪造 `-1`。任一 structural phase 或 gate 失败后，后续未运行 gate 必须按固定顺序写 `NOT_RUN`。日志保存 stdout/stderr 完整输出的组合 SHA-256，并在总计 4 KiB 内为两个流分别保留有标签、已清除 URL userinfo/本机绝对路径/环境值的 tail；任一流不得覆盖另一流的诊断，不嵌入完整日志、源码或凭据。

JSON 字段名固定为 lower-camel；空集合编码为 `[]` 而不是 `null`。耗时使用单调时钟，`finishedAt` 必须由 `startedAt + elapsed` 推导，且序列化后的时间差按毫秒截断后必须严格等于 `durationMs`，避免宿主机 NTP/WSL 时钟校正制造自相矛盾的证据。

最终 verify 的每个命令都使用参数数组、显式 cwd、固定 Go binary、超时和进程组 kill/wait；每轮 verify 创建新的私有 HOME/GOCACHE/GOMODCACHE/GOTMPDIR，丢弃继承的 `GO*`/`GIT_*` 并固定 proxy/checksum/toolchain。`--structural-only` 只输出 `finalEvidence=false`、`overallResult=NOT_FINAL` 的开发诊断，禁止作为 L0 PASS。

> **缓存策略更新（2026-08-19）：** 本段的“每轮独立私有 `GOCACHE/GOMODCACHE`”要求已由
> [Thingsbaseline 共享 Go 缓存设计](./2026-08-19-thingsbaseline-shared-go-cache-design.md)替代。
> candidate、`HOME`、`GOTMPDIR`、workspace 和构建输出仍逐轮隔离；只有受信 Go 解析出的
> `GOCACHE/GOMODCACHE` 跨轮复用，所有测试显式使用 `-count=1`。

evidence 路径必须位于 source、backend、target、candidate、isolation/cache 之外；父目录和目标均按 fd/no-follow 校验。JSON 通过同目录 exclusive temp file、`fsync(file)`、原子 rename 和 `fsync(parent)` 写入；已有 symlink 或特殊文件 fail closed。JSON 禁止 source/backend/candidate/cache 的绝对本机路径，target 只能写逻辑路径 `backend/things`。

本轮受审证据落盘为：

```text
docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.json
docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.md
```

JSON 是机器结果，Markdown 是人工解释。未执行的 gate 不得写 PASS；失败必须保留 exit code 和日志摘要。

## 13. 退出条件

L0 只有同时满足以下条件才完成：

1. 源 ref、commit、tree 和 worktree 状态前后不变；
2. 976 个固定输入被完整读取，207 个排除文件无一进入目标；
3. 两次独立 HOME/cache 的候选渲染一致，`verify` 对只读 target 报告零差异，且 target 前后 tree digest 一致；
4. secret/private-key/ELF/大文件/旧 module 前缀扫描全部通过；
5. Things、SaaS、tools 和整个 Go Workspace 测试通过；
6. `thingsvr`、`dgsvr`、`dmsvr` 三入口构建通过；
7. API/RPC/生成物除声明转换外逐字节一致；
8. JSON 与 Markdown evidence 完整；
9. `http://127.0.0.1:18081/` 的 L0 状态和证据摘要更新；
10. `git status` 只包含预期文件，等待用户审核；不自动提交。

## 14. 与后续 Loop 的关系

L0 完成后才解锁：

```text
L0 Things 可编译迁移基线
 ├─> L1 IoT 控制面内核
 └─> L3 Kafka 数据面基础

L1 -> L2 模块级物模型 / ModelRelease
L1 + L2 + L3 -> L4 Raw→Canonical
```

Scene 继续保持锁定，直到 L5 Current Fact、L6 Reducer 和 L7 Command/Action Outbox 全部具备可验证基线。
