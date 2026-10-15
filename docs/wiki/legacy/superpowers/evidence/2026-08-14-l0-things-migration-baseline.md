# L0 Things 可编译迁移基线证据

## 当前结论

L0 于 2026-08-15 完成 fail-closed 恢复后的最终机器验证和独立最终复审，结论为
**APPROVED**：固定联犀 Things Git 对象可确定性生成 `backend/things`，11 个 phase、
5 个 gate、双渲染、目标与源仓库一致性全部通过；原 1 个 Critical、3 个 Important 和
1 个 Minor finding 全部关闭。L0 现已获准完成，并解锁 L1/L3。

- 权威机器证据：[最终 PASS JSON](./2026-08-14-l0-things-migration-baseline.json)
- 第一次失败证据：[Things fixture gate FAIL](./2026-08-14-l0-things-migration-baseline-attempt-1-fail.json)
- 第二次失败证据：[Workspace gate FAIL](./2026-08-14-l0-things-migration-baseline-attempt-2-workspace-fail.json)
- 第三次审查拒绝证据：[旧 verifier PASS / final review rejected](./2026-08-14-l0-things-migration-baseline-attempt-3-review-rejected.json)
- 固定输入说明：[Things 固定源基线](./2026-08-14-l0-things-source-baseline.md)

最终运行时间为 `2026-08-15T12:20:01.745046532+08:00` 至
`2026-08-15T12:29:02.32566986+08:00`，总耗时 540,580 ms。canonical JSON 的
SHA-256 为 `eaa13e8fec4c430b2df4f3459fecc60a271577d5790a41c0f1abcffed9158123`。

## 固定输入与确定性输出

| 项目 | 值 |
| --- | --- |
| source ref | `refs/remotes/origin/master` |
| source commit | `243ab1aa03b8a3f106761a81cdd9bd8827d448c8` |
| source tree | `544b638ae3ee5e5f3a041bea22ca035dbb7e5859` |
| source index SHA-256 | `aff3e1e0ae168a515ba1d388725265fe8a3ccdcd8f4e9c21a689fa171514e8bf` |
| selected source files | 976 |
| generated target files | 980 |
| archive SHA-256 | `32b998e4621dd88a39b618552187cfbd1be910851925702a07c9e4c82cb4d486` |
| manifest SHA-256 | `a936310b85a65287f27a2b3a93c36baa373d3e1506fb0292bed83914418cb3ae` |
| final tree SHA-256 | `a0696daec9fa020573b41c4a31873ae1eb7473650a59a65a69355eeb6c21e672` |

两次独立 HOME/cache 渲染得到相同 manifest 和 tree SHA-256；目标扫描前后也是同一
tree SHA-256，target differences 为 0。官方源的 HEAD、固定 ref、完整 status SHA-256
和 index 字节 SHA-256 在读取前后完全一致。

## 四次尝试如何形成证据链

| 尝试 | tree SHA-256 | 到达位置 | 结论 |
| --- | --- | --- | --- |
| attempt 1 | `1aeee85d09f7…` | `things-test` | fixture gate FAIL |
| attempt 2 | `8d2ab9f63e29…` | `workspace-test` | workspace gate FAIL |
| attempt 3 | `8d2ab9f63e29…` | 旧命令 gate 全通过 | final review rejected |
| recovered final | `a0696daec9fa…` | 新 verifier 全部 phase/gate + 独立复审 | APPROVED |

attempt 1 的根因是固定测试 fixture 中 45 个旧 key 与当前模型不一致；修复只作用于
SHA 锁定的测试输入，不修改生产模型。attempt 2 证明单模块测试不能替代 workspace
测试：测试辅助进程必须从当前 checkout 解析 backend，并明确使用 `GOWORK=off`。

attempt 3 尤其重要：命令实际通过并不等于证明充分。最终审查发现旧 verifier 仍可产生
false-PASS，因此旧 canonical 在覆盖前按原字节保存并明确标记 review-rejected，而不是
删除这段历史或继续把它称为最终证据。

## Final-review recovery 的五项收紧

1. **Build output contract**：使用已打开并验证为 `cmd/go@go1.25.4` 的同一 Go 文件描述符；
   构建前只清理三个固定旧产物，构建后要求它们恰为本轮生成的 single-link、0755、非空
   regular file，并再次校验 identity、metadata、内容 SHA 与 Go build info。
2. **非 YAML 凭据契约**：两个固定源码文件在 source SHA、语义路径、精确计数和 output
   SHA 同时匹配时替换 5 个值；独立反向扫描要求只剩声明的环境变量占位符。
3. **Source index 证明**：`SourceRepositoryState` 从 HEAD/ref/status 三项扩展为
   HEAD/ref/status/index 四项；index 采用 no-follow、single-link、流式 SHA 和读前后
   identity/mode/size 复核。
4. **统一日志边界**：render stdout/stderr 都使用 raw-byte 流式脱敏器；SHA 覆盖完整原始流，
   每个 tail 不超过 1,900 bytes、合计不超过 3,800 bytes。本次 evidence 的最大单流及
   合计 tail 都是 1,899 bytes。
5. **PASS 一致性谓词**：序列化器只接受固定顺序的 11 个 PASS phase 与 5 个
   `PASS/EXITED/0` gate，并同时验证双 render、target、source、artifact 证明；缺失、乱序、
   NOT_RUN 或前后矛盾均不能写成 PASS。

## 最终门禁与构建产物

| Gate | Result | Exit | 耗时 |
| --- | --- | ---: | ---: |
| `tools-test` | PASS | 0 | 17,059 ms |
| `saas-test` | PASS | 0 | 113,612 ms |
| `things-test` | PASS | 0 | 141,977 ms |
| `workspace-test` | PASS | 0 | 191,244 ms |
| `things-build` | PASS | 0 | 13,083 ms |

| Artifact | Mode | Size | SHA-256 |
| --- | --- | ---: | --- |
| `backend/bin/dgsvr` | `0755` | 161,051,378 | `f3bf3301c542ff92478d425deb7e330d714a58eb5002484468c3104c17f56a4e` |
| `backend/bin/dmsvr` | `0755` | 160,359,945 | `9a20713900ba02de73f2ca597af7d45bb8b41b59486ab80810fa425cabe25417` |
| `backend/bin/thingsvr` | `0755` | 165,476,814 | `efa981b0d777afd15615cddc7160cfc6258a7dcad27f22223cf6a5dafd32384b` |

独立只读审计已再次从磁盘计算三份 artifact 的 mode、link count、size 和 SHA，并与 JSON
逐项匹配。`backend/bin/` 被 Git 忽略，构建产物不进入源码提交。

## 独立最终复审

独立 reviewer 逐项复核原五个 finding，结论均为 **CLOSED**，没有剩余 Critical、
Important 或 Minor。复审实际重跑：

- `backend/tools` 的 `GOWORK=off go test -count=1 ./...`；
- `backend/tools` 的 `GOWORK=off go test -race -count=1 ./...`；
- `backend/tools` 的 `GOWORK=off go vet ./...`；
- `backend/things` 的 `GOWORK=off go test -count=1 ./...`；
- 当前官方源真实转换测试、`git diff --check` 和 `gofmt -l`；
- canonical、生成树、三个 artifact、官方源状态及临时目录的只读对照。

复审确认：无 staged 文件、无 candidate/备份残留，官方 `things` 仓库干净且未被修改。

## 转换与安全边界

最终转换报告为 `94/2726/55/1/1/45/5`：Core/Things 前缀分别转换 94/2,726 处；
9 个 YAML 文件中的 55 行凭据形态配置全部占位符化；1 个嵌入私钥块被移除；既存 vet
问题只做 1 处 SHA 锁定修复；测试 fixture 转换 45 处；两份非 YAML 文件净化 5 个值。

证据不保存本机绝对路径、随机 candidate/cache 路径或凭据正文。原始凭据不会从本文、
lock、测试输出或诊断 tail 暴露；日志只保存完整 raw SHA 与有界脱敏摘要。

## Program Graph 状态

L0 当前是 **APPROVED · A1**。L1（IoT 控制面内核）与 L3（Kafka 数据面基础）已经解锁，
可以并行启动；L2 继续等待 L1 的控制面边界；Scene Runtime 继续等待后续 join 点。

当前分支未执行 `git add`、`git commit` 或 `git push`。
