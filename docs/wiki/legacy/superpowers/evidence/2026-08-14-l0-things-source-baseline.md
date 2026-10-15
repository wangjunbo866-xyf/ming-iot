# L0 Things 固定源基线证据

本文件记录 L0 迁移前的只读源基线。固定输入来自联犀官方远端 ref 对应的 Git
对象，不读取本地 `master` 上额外的两个提交，也未修改官方 `things/` 工作树。

## 固定 Git 对象

| 项目 | 实测值 |
| --- | --- |
| repository | `https://gitee.com/unitedrhino/things` |
| ref | `refs/remotes/origin/master` |
| commit | `243ab1aa03b8a3f106761a81cdd9bd8827d448c8` |
| tree | `544b638ae3ee5e5f3a041bea22ca035dbb7e5859` |
| 本地分支状态 | `master...origin/master [ahead 2]`，工作树无修改 |
| tracked files | 1,183 |
| selected files | 976 |
| selected blob bytes | 4,866,935 |
| archive bytes | 5,754,880 |
| archive SHA-256 | `32b998e4621dd88a39b618552187cfbd1be910851925702a07c9e4c82cb4d486` |

## 编译与已知反例

- `GOWORK=off go test -vet=off -run '^$' ./...`：2026-08-15 复跑 PASS；这只证明
  源包可编译，不冒充完整测试。
- 默认 vet：2026-08-15 在
  `service/apisvr/internal/logic/things/device/edge/sendLogic.go:86` 稳定 FAIL，原因是
  `AddDetailf` 使用动态 format string。L0 转换器只把这一处改为显式 `%s`，并用源
  blob SHA 和 old/new 命中数锁定边界。
- credential-shaped YAML：9 个文件、55 行，其中 active 35、commented 20；目标
  只保留按完整语义路径映射的环境变量占位符。
- embedded private-key block：1 个；原始正文不进入 lock、测试 fixture、日志、
  candidate 或目标历史。

## 完整性说明

输入完整性由固定 commit/tree、禁用 Git replace object、逐 blob object ID 重算及
archive bytes/SHA-256 共同证明。HEAD/ref/status 前后比较只用于发现并发工作区漂移。
上述两轮 Go 命令结束后，官方仓库仍为 `ahead 2` 且工作树无修改。
