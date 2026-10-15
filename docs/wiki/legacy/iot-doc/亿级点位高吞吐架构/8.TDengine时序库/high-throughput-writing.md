# TDengine 高效写入：写入能力评估与改进计划

> 场景：百万设备 × 500 测点（400 float / 50 int / 50 string(50)），每设备 5 分钟 1 条消息，每天 12 小时，500 点位拆 3 张超级表（200 float / 200 float+50 int / 50 string）
> 配套文档：[容量规划](./README.md) | 版本：3.3.6.34 (LTS)
> 官方依据：[高效写入](https://docs.taosdata.com/developer-guide/high-throughput/)、[maxSqlLength](https://docs.taosdata.com/operations-and-tooling/components/taosc/#maxsqllength)

---

## 一、写入负载基线（来自容量规划）

| 指标 | 数值 |
|---|---|
| 消息速率 | 1,000,000 ÷ 300s ≈ **3,333 条消息/s** |
| 行速率 | ×3 张超级表 = **10,000 行/s** |
| 测点速率 | ×500 点位 = **167 万测点/s** |
| 裸数据量 | ~14.4 MB/s（620 GB/天 ÷ 12h 写入窗口） |

## 二、写入能力评估（基于 maxSqlLength = 4,194,304）

> 集群 `maxSQLLength` 配置为 4 MB（4,194,304 字符），单条 SQL 长度上限即批大小上限。
> **注意**：`maxSQLLength` 参数从 **v3.3.6.34 / v3.3.8.8** 开始才支持，当前集群版本 3.3.6.34 满足要求；若使用 3.3.6.13 等更早版本，该参数无效且上限仍为 1MB。

### 2.1 单条消息的 SQL 文本长度估算

SQL 文本形态比二进制裸数据大，逐表估算（ worst case，字符串按满 50 字符、数值含分隔符）：

| 超级表 | 估算 | 小计 |
|---|---|---|
| 表1（200 float） | ts 14B + 200 × ~11 字符 | ~2.2 KB |
| 表2（200 float + 50 int） | ts 14B + 200×11 + 50×6 | ~2.5 KB |
| 表3（50 string） | ts 14B + 50 × 52（含引号） | ~2.6 KB |
| 子表名 ×3（`tb_xxx VALUES(...)`） | 3 × ~20 字符 | ~0.1 KB |
| **单消息合计** | | **~7.4 KB（worst case）** |

- 典型值（字符串不满 50 字符、float 值较短）：~5 KB/消息
- 若使用自动建表（`USING ... TAGS(...)`），每条再 +100~200 B 标签文本

### 2.2 单条 4MB SQL 的承载能力

| 口径 | 消息数 | 行数 | 测点数 |
|---|---|---|---|
| Worst case（7.4 KB/消息） | 4194304 ÷ 7578 ≈ **553 条消息** | ~1,660 行 | **~27.7 万测点** |
| 典型值（5 KB/消息） | ≈ **840 条消息** | ~2,500 行 | **~42 万测点** |

### 2.3 需求 vs 供给

**集群需要消化的 SQL 速率**：

```
3,333 条消息/s ÷ 553~840 条消息/SQL ≈ 4~6 条 SQL/s（全集群）
```

**对应网络带宽**：

```
5 条 SQL/s × 4 MB ≈ 20~24 MB/s
```

> ✅ 与容量规划中"业务入口带宽 ≈ 22 MB/s（14.4 MB/s × 1.5 协议开销）"的结论互相印证——SQL 文本化带来的 ~1.5 倍膨胀恰好就是协议开销系数。

**写入线程（客户端）数估算**：

假设单条 4MB SQL 端到端耗时 ~200ms（网络 + 解析 + 落盘，**需实测校准**）：

| 写入线程 | 吞吐能力 | 对 3,333 msg/s 需求的余量 |
|---|---|---|
| 1 | ~5 SQL/s ≈ 2,800 msg/s | 0.8× ❌ 不足 |
| 4 | ~11,000 msg/s | 3.3× |
| **8（建议）** | ~22,000 msg/s | **6.7× ✅** |
| 16 | ~44,000 msg/s | 13× |

> **术语区分**：本节的"写入线程"指**网关应用（客户端）侧**拼 SQL 并发送的线程，在网关代码中控制，与服务端两个同名概念无关：
> - `numOfCommitThreads`（values.yaml 配 8）：服务端 taosd 把 memtable 落盘的**刷盘提交线程**；
> - "每 CPU 核服务 1~2 vnode"：服务端 **CPU 规划口径**，用于决定节点核数（30 vnode/节点 → 32 核），不是具体线程数。
> 
> 三者在同一条流水线上各司其职：客户端写入线程喂数据 → 服务端 vnode 并行写入内存（并行度由 vnode 数决定，非客户端线程数）→ 提交线程落盘。

**结论**：4MB 批量 + 8 个写入线程（每线程独立连接）即可承载当前负载并保留 6 倍以上余量；服务端侧按官方口径（单核 ≥1 万写入请求/s、每请求 200+ 行时单核可达 100 万行/s），10,000 行/s 远低于瓶颈，**写入能力充足，瓶颈不在数据库**。

> ⚠️ 注意：官方指出批量"超过一定阈值其优势会消失"。4MB 是上限而非必然最优，建议上线前用 taosBenchmark 对比 1000 行 / 2500 行 / 满 4MB 三档批大小的实际吞吐，取拐点值。

### 2.4 官方 Benchmark 数据对标：行宽红利 vs 三副本折扣

**官方 TSBS 实测数据**（来源：[TSBS IoT 报告](https://tdengine.com/tsbs-iot-performance-report-tdengine-influxdb-and-timescaledb/)、[TDengine vs InfluxDB 3 Core](https://tdengine.com/tdengine-oss-vs-influxdb-core-tsbs-performance-results/)）：

| 报告 | 硬件 | 场景 | 写入能力 |
|---|---|---|---|
| 报告一 | AWS r4.8xlarge：32 vCPU / 244GB / SSD 3000 IOPS·125MB/s，12 vnodes | 100 万设备 × 576 行 | **283 万 metrics/s** |
| 报告二 | 40 核 Xeon / 251GB，TDengine 3.3.6.3，6 vgroups | IoT 100 万设备 × 10 测点 | **437 万 metrics/s** |
| 报告二 | 同上 | DevOps 100 万设备 × 10 测点 | **784 万 metrics/s** |
| 容量规划文档 | — | 单核口径 | ≥1 万写入请求/s；每请求 200+ 行时单核 100 万行/s |
| 多级存储文档 | 集群多挂载点 | — | 持续写入可达 3 亿测点/s |

**⚠️ 口径警告：以上全部为单机单副本数据，官方没有发布三副本集群的写入 benchmark。**

**与官方压测机的硬件对比**：CPU 同档（32 核 vs 32~40 核）、磁盘远强（NVMe vs 125MB/s SSD）、内存较少（64G vs 244G，但官方实测写入期 CPU <17%，瓶颈不在内存）——**硬件维度不输官方压测环境**。

**行宽红利**：官方 TSBS 是 10 列/行的窄行，本场景是 200/250/50 列的宽行。TDengine 写入开销主要按"行"计（解析、路由、WAL、memtable 均按行），一行塞 200 点位与 10 点位成本几乎相同。本场景峰值仅 **3 万行/s**（10,000 msg/s × 3 表），500 万测点/s 是"3 万宽行"而非"500 万窄行"，压力远小于数字观感。

**三副本折扣（官方无数据，经验推断）**：Raft 复制增加网络开销与提交延迟，写入吞吐通常打 7~8 折；且三副本下每节点承担全量写入，加节点不提升写入吞吐。

**对标结论**：

| 负载 | 测点速率 | 评估 |
|---|---|---|
| 常规 3,333 msg/s | 167 万/s | ≈ 官方单机成绩的 40~60%，即使三副本打 7 折仍有 **~1.8 倍余量** ✅ |
| 集中上报 10,000 msg/s | 500 万/s | 宽行红利下大概率可承载，但**已超出官方公开数据覆盖范围，必须实测**（见第六章） ⚠️ |

## 三、官方高效写入七要素对照盘点

| # | 官方要素 | 当前状态 | 说明 |
|---|---|---|---|
| 1 | **批量写入** | ✅ 已做到 | 单条 SQL 拼满 4MB（maxSqlLength=4194304），约 550~840 条消息/批 |
| 2 | **多线程写入** | ✅ 已做到 | 多线程写入，每线程独立连接 |
| 3 | **写入相邻性** | ⚠️ 不适用 | 同子表做不到（设备 5 分钟才传一次，批内天然跨表）；**同超级表归并的 vnode 收益经源码核实不存在**（`SSubmitTbData` 顺序由客户端哈希桶序决定、逐表处理全是 uid 哈希定位，见 5.6），无需为此调整拼 SQL 顺序 |
| 4 | **提前建表** | ❌ 未做到 | 只提前建了 3 张超级表，300 万子表靠自动建表创建；每次写入都带 `USING ... TAGS(...)`，服务端频繁检查表是否存在 |
| 5 | **写入方式** | ❌ 未遵守 | 当前用 SQL + 自动建表（七要素中效率最低的常规方式）。**原因**：自动建表方便随时修改标签和列定义，业务灵活性优先 |
| 6 | **避免乱序** | ✅ 天然满足 | 设备两次消息间隔 5 分钟，同一子表数据天然按时间戳升序，基本不会乱序（设备补传/重传除外） |
| 7 | **开启压缩** | ❌ 未开启 | 使用 WebSocket 连接，未开启传输压缩 |

## 四、后期改进计划

按"收益/改造成本"排序：

### P1 — 低成本高收益，优先落地

| 改进项 | 做法 | 预期收益 | 代价 |
|---|---|---|---|
| **开启传输压缩** | WebSocket 连接开启压缩 + 服务端 `compressMsgSize 1024`（values.yaml 已配） | 入口带宽 22 → ~15 MB/s；字符串占比高的报文压缩收益明显 | 少量 CPU 换带宽，几乎零成本 |

### P2 — 需要配套机制，中期改进

| 改进项 | 做法 | 预期收益 | 代价与风险 |
|---|---|---|---|
| **提前建子表** | 设备注册/开通流程中批量预建 3 张子表（多表 CREATE 按 vgroup 分批直发 vnode，不经过 mnode）；运行期新设备兜底仍走自动建表 | 大头在客户端：写入不再传 TAGS 文本（每消息省 100~200B×3）+ 免逐 tag 解析构建（即使表已缓存 tags 也会被解析再丢弃，见 5.6）；服务端免存在性检查仅对未缓存表（小~中）；**注意：新表首写的 missCache catalog 串行往返不会因预建而消失**，需配合注册时缓存预热（5.6 末节） | 需建表服务与设备开通流程联动；子表元数据批量创建本身有开销（建议 ≤1000 表/批） |
| **改参数绑定写入（stmt）** | 热路径用参数绑定（免 SQL 解析）。**注意**：连接器的"高效写入特性"（`asyncWrite=stmt`，自动线程 + 按子表队列 + 批量发送）目前**仅 Java JDBC 3.6.0+ 支持，Go 连接器（driver-go）无此特性**——Go 网关需自建 worker 池 + 队列（现有架构即是），或直接用 stmt 参数绑定替代 SQL 拼接 | 免解析 + 免组 SQL，客户端 CPU 和网络进一步下降（源码分析见第五章：**收益仅在客户端/Adapter，服务端两条路径等价**） | **最大障碍**：schema 变更（改列/改标签）需重建 stmt，必须配套 schema 版本管理机制——这正是当初选自动建表的原因，改动前需评估变更频率 |

### P3 — 无需行动，保持现状

| 项 | 结论 |
|---|---|
| 避免乱序 | 设备 5 分钟周期 + 网关按设备会话保序，天然有序；只需保证同一设备的消息固定路由到同一写入线程/队列（防网关内部乱序） |
| 同子表写入相邻 | 物理上做不到也不必要：单设备 5 分钟一行，按子表攒批只能等超时刷出，纯增延迟无吞吐收益。**本场景的攒批维度是"跨设备时间窗聚合"**（同窗口多设备拼一条 4MB SQL），而非同子表攒批。需要绑定的不是"子表→队列"，而是"**设备→写入线程**"（按设备 hash 固定路由，仅用于保证同设备消息有序） |
| 同超级表归并拼接 | 经源码核实收益≈0，**从 P1 降级**（见 5.6）：文本顺序经客户端 `pTableBlockHashObj` 哈希归并后传不到 vnode（`SSubmitTbData` 顺序=哈希桶序），vnode 逐表处理全是 uid 哈希定位、无顺序 locality；仅当同表在同一 SQL 中分散出现多次时可省少量客户端重复查找（auth/meta/USING 重解析），本场景每表只出现一次，无需为此调整拼 SQL 逻辑 |

## 五、源码级分析：SQL 批量插入 vs Stmt 参数绑定（服务端视角）

> 分析对象：TDengine 3.x 源码（本仓库 `TDengine/` 目录）。目的：回答"P2 改参数绑定"的收益究竟落在流水线的哪一段。

### 5.1 核心结论：vnode 从不解析 INSERT SQL 文本

TDengine 3.x 的 SQL 解析 100% 发生在**客户端进程**（原生连接为 libtaos.so；WebSocket 场景为 taosAdapter 进程），vnode/dnode 服务端收到的已是二进制消息：

- 客户端解析入口：`clientImpl.c:376 parseSql()` → `parser.c:591 qParseSqlSyntax()`，parser 库链接在客户端；
- vnode 写入入口：`vnodeSvr.c:3216 vnodeProcessSubmitReq()`，第一步即 `tDecodeSubmitReq()`（`:3247`）解码二进制 `SSubmitReq2`，全程无 SQL 文本处理；
- SQL 拼接与 Stmt 绑定最终都经 `parInsertUtil.c:1089 buildSubmitReq()` 序列化成**同一种 `SSubmitReq2`**，走同一条 `TDMT_VND_SUBMIT` RPC、同一个服务端处理函数。

**推论：服务端视角两者开销几乎相同；差异全部在"谁把数据变成 SSubmitReq2"的前端阶段。** 服务端没有任何"SQL 解析"度量点（只有 `vnodeSvr.c:918` 附近的 `writeMetrics.apply_time` 度量解码+落盘），因为该成本本就不在服务端发生。

### 5.2 SQL 大文本的客户端解析路径与成本

1. `parser.c:594` 附近：`qIsInsertValuesSql()` 识别 `INSERT INTO ... VALUES` 后**绕过 lemon AST 语法分析器**，直接走手写插值解析器——无 AST/逻辑计划/优化器阶段；
2. `parInsertSql.c:4274 parseInsertBody()`：逐表解析 `tb_name [USING ... TAGS] VALUES (...)` 子句；同表多处出现经 `pTableBlockHashObj` 哈希归并到同一数据块；
3. 每行 `parseOneRow()` → 逐列 `parseValueTokenImpl()`（`:2617`）：tokenizer 逐字符切分 + 按 schema 类型转换（时间戳解析、浮点 `strtod`、字符串转义/去引号最贵）；
4. 每行 `tRowBuild()`（`:3581`）构建 `SRow`；
5. `insMergeTableDataCxt()`（`parInsertUtil.c:998`）**按 vgId 归并**，每个 vgroup 序列化一条 `SSubmitReq2`。

⚠️ 隐性成本（missCache，已按源码核实修正）：**客户端元数据缓存未命中时（新表/schema 变更后的首次写入），解析在第一个未缓存表处中断**（`parInsertSql.c:4279`，循环条件含 `!pCxt->missCache`），`buildInsertCatalogReq()`（`:4824-4842`）只把**当前这一张表**（+其超级表、db vgroup hash、auth）放入请求；拉回后 `parseInsertSqlFromTable()`（`:4709`）**从失败表处断点续传**，已解析的行保留在 `pTableBlockHashObj` 中不丢——**不是整段重扫**。但 **catalog 往返次数 = 本条 SQL 中未缓存表的数量，每表一次、串行**：一条混入 N 张新表的 4MB SQL 要串行往返 N 次，这是新设备首写的主要客户端开销（对治见 5.6 末节）。稳定期（表均已缓存）不再发生；本场景 300 万子表靠自动建表，新设备首次写入会持续触发该路径。

### 5.3 Stmt 的客户端路径与缓存机制

1. **Prepare**（`clientStmt2.c:2158` 附近）：仅提取 dbName，不解析 SQL 全文；
2. **首次 Bind** 触发一次 `stmtParseSql()`，随后多级缓存跨 Execute 复用：
   - 表元数据/schema（`pTableCache`）；
   - `STableDataCxt` 模板（`pBlockHash`，命中后 `needParse=false`，完全跳过解析与 meta 请求）；
   - SQL 中混写的常量值（`pParsedValues`，只解析一次）；
3. **Bind 数据**（`parInsertStmt.c:1048` 附近）：二进制 buffer 直接按列追加到 `SColData[]`（列式格式），无 tokenizer、无字符串转换；
4. **Execute**（`clientStmt2.c:3359` 附近）：所有累积数据一次性按 vgroup 打包成 `SSubmitReq2`——**与 SQL 插入完全同一发送路径**（同 plan、同 scheduler、同 `TDMT_VND_SUBMIT`）。

### 5.4 vnode 侧：同一函数、同一消息、同一份落盘成本

- 行格式（SQL 插入）走 `tsdbInsertRowDataToTable()`（`tsdbMemTable.c:718`），列格式（stmt2 默认）走 `tsdbInsertColDataToTable()`（`:618`）——同等级一等公民，经 `tsdbInsertTableData()`（`:119`）最终都进同一 memtable 跳表；
- raft WAL 复制、apply 耗时两者完全一致。

### 5.5 对本场景决策的修正

| 前提 | 结论 |
|---|---|
| 网关 taosc 原生直连 | 解析开销在网关进程；**网关 CPU/带宽有富余时，大 SQL 与 Stmt 服务端吞吐等价**，改 Stmt 的收益只剩"省网关 CPU" |
| 网关 WebSocket（taosAdapter）且 Adapter 与 dnode 同机 | SQL 文本解析落在 Adapter 进程，表现为"服务端机器"CPU 升高；改 Stmt 或迁原生连接可消掉这部分 |
| schema 频繁变更（当前依赖自动建表灵活性） | Stmt 需重建 + schema 版本管理机制，维持 SQL 拼接更划算 |

**对 P2"改参数绑定写入"的修正**：决策依据应从"提升写入吞吐"修正为"**降低网关（或 Adapter）CPU**"。在网关 CPU 有富余、且 schema 变更频繁的前提下，P2 优先级可下调，资源优先投给提前建子表（P1 同超级表归并的收益经 5.6 核实≈0）。

### 5.6 收益核实：同超级表归并拼接 vs 提前建子表

> 对第三章要素 3（写入相邻性）与 P1/P2 两个改进项的源码核实。调用链见 5.1~5.5，此处只列收益构成。

#### 同超级表归并拼接：vnode 侧收益不存在

- 客户端 `pTableBlockHashObj` 哈希归并使**文本顺序在解析期无关紧要**：同一子表无论分散还是相邻出现，归并结果相同（`parInsertUtil.c:397-417 insGetTableDataCxt`）；
- `insMergeTableDataCxt` 用 `taosHashIterate` 遍历哈希（`parInsertUtil.c:1010`），`SSubmitTbData` 顺序 = **哈希桶序**——网关怎么排 SQL 文本都控制不了 vnode 看到的表顺序；
- vnode 逐表处理全部是 uid 哈希定位：`metaGetInfo` 走 `sEntryCache`（`metaCache.c:342-434`，无容量上限、无 LRU 淘汰，热身后命中率 ~100%，微秒级）；memtable 按 uid 哈希桶定位 `STbData`（`tsdbMemTable.c:398-467`），每表独立跳表，无跨表 locality；唯一顺序敏感的 `tsdbUpdateSkm` 默认不在插入路径（`tglobal.c:478 tsUpdateCacheBatch=true`，`tsdbMemTable.c:762-763` 被跳过）。

真实收益仅剩客户端 CPU 两项：

| 收益 | 机制 | 量级 |
|---|---|---|
| 同表分散出现 K 次 → 1 次 | 省 (K-1)×（auth 缓存查找 + meta 缓存查找 + USING/tags 重解析，`parInsertSql.c:2237, 2187, 180-198`） | 小~中；**每表只出现一次则为零**（本场景每设备每窗口 1 消息，即属此列） |
| 同表行 ts 乱序时省排序 | `ordered=true` 跳过 `tRowSort`/`tRowMerge`（`parInsertUtil.c:1044-1058`） | 无~中（时间窗拼接近似有序时本就跳过） |

**结论：P1"同超级表归并拼接"对本场景收益≈0，已移入 P3。**

#### 提前建子表：大头在客户端文本与解析

| # | 收益 | 机制与证据 | 量级 |
|---|---|---|---|
| a | SQL 文本/网络/tokenize 减少 | 去掉每表 `USING stN TAGS(...)` 子句（tags 文本在 4MB SQL 中占比可观） | 中 |
| b | 客户端免 tags 解析 | 每次表出现省逐 tag `parseTagValue` + STag 构建；**即便表已缓存，tags 也会被完整解析再丢弃**（`parInsertSql.c:180-198, 1863-1963`） | 中 |
| c | 服务端免存在性检查 | 每批每（客户端未缓存的）表省 2 次 name 索引 B-tree 查找（leader 预处理 `vnodeSvr.c:113` + apply `metaTable2.c:292`） | 小~中（页缓存命中微秒级；3M 表 name 索引冷页有读盘风险） |
| d | 新表创建移出写入 apply | 7 个 TDB 写入 + 缓存失效 + `tsdbCacheNewTable`（`metaEntry2.c:1327-1368`）改在注册时做；多表 CREATE 按 vgroup 分批、一批一消息一直发 vnode（`parTranslater.c:24620-24647`、`planLogicCreater.c:3697-3717`），RPC/raft 按批摊销 | 中（仅新设备） |
| e | ~~减少客户端 catalog 往返~~ | **不存在**：预建的表不在客户端 catalog 缓存，新表首写照样 missCache、每表一次串行往返（`parInsertSql.c:2181-2197`、`clientMain.c:1934-1966`） | 无 |
| f | ~~避免 mnode/sdb/raft、避免 apply 阻塞~~ | **不存在**：自动建表本就不走 mnode/sdb（`vnodeHandleAutoCreateTable` 本地 `metaCreateTable2`），无独立 raft；预建的 CREATE 同样要在 apply 执行 | 无 |

#### 关键洞察：新设备首写的真正瓶颈不在两个优化覆盖范围

missCache 时 catalog 请求只装当前一张表（见 5.2），**往返次数 = SQL 中未缓存表数，串行**。一条混入 N 张新表的 4MB SQL 要串行往返 N 次。对治办法：

- 设备注册/预建表时，让网关写入连接先 `DESCRIBE`/SELECT 预热该表进 catalog 缓存；
- 或拼 SQL 时将新表与老表分批：新表批次单独写，避免阻塞老数据批次。

## 六、验证与监控

1. **压测校准**：上线前用 taosBenchmark 按真实表结构（200/250/50 列三表）+ 4MB 批大小实测，输出：单 SQL 耗时、吞吐拐点批大小、实际压缩率（反哺容量规划的 5x 假设）
2. **运行期监控**：
   - 写入侧：SQL 成功率、单 SQL 耗时 P99、批内消息数分布
   - 服务端：taosKeeper 指标（vnode 写入速率、WAL 写入延迟、memtable 落盘频率）
3. **回归基线**：改进项每次上线后对比写入耗时 P99 与服务端 CPU，验证收益是否符合预期

## 七、FAQ

### Q1：需要把每节点内存扩到官方压测机的 250GB 吗？

不需要。官方压测机的 244/251GB 是 AWS 实例固有规格，不是 TDengine 的需求；报告实测写入期 TDengine 资源占用很低（CPU <17%）。内存真实去向：

| 用途 | 受益方 | 本场景需求 |
|---|---|---|
| vnode buffer + cache（官方公式） | 写入 | ~6 GB/节点保底 |
| 查询工作集 + RPC + 元数据 | 查询 | ~10-16 GB |
| OS page cache（剩余内存自然充任） | **主要是查询**，对写入几乎无贡献 | 64GB 下已有 ~40GB 空闲 |
| 250GB 的增量 | 仅"缓存 200GB+ 热数据做高频历史查询"才有意义 | 按查询模式决定，非写入需要 |

写入主导场景 64GB 足够；重查询再升 128GB，250GB 是把"压测机规格"误读为"推荐配置"。

### Q2：Raft follower 写页缓存即成功，页缓存是不是越大越好？

**机制理解正确**（默认 `walLevel 1`）：

```
leader 收客户端写 → 本地 WAL（页缓存）→ 转发 follower
follower 追加 WAL（页缓存）→ 即回 ACK
quorum 2/3 ACK → 写入成功返回
持久性 = 多台机器页缓存都有副本，而非单机 fsync
```

因此三副本写延迟主要取决于网络 RTT 而非磁盘 fsync。

**但"页缓存越大越好"的推论不成立**——页缓存对写入是"过道"而非"蓄水池"：

1. 写吞吐上限是**磁盘回写带宽**而非页缓存大小：内核按 `dirty_background_ratio` 持续刷脏页，本场景稳态 WAL ~15MB/s、峰值 ~50MB/s，NVMe 随手刷掉，脏页攒不起来；
2. 真正的写缓冲是 TDengine 自己的 **memtable**（`BUFFER 128MB` × 30 vnode ≈ 3.8GB/节点，已配置），它才负责吸收写入突发；落盘由 `numOfCommitThreads 8` 负责；
3. 页缓存大只加速**读查询**（热数据命中），对写入 TPS 提升约等于零。64GB 加到 250GB，写入 TPS 基本不变。

写入路径调优的正确位置：memtable（BUFFER）→ WAL 盘性能 + commit 线程 → 副本间网络 RTT；可选 `walLevel 2` + fsync 换更强持久性（三副本下一般无需开）。

### Q3：方案A（INSERT 带 USING TAGS 自动建表）vs 方案B（预建子表裸 INSERT），性能区别大吗？方案B写入前不检查元数据吗？

**稳态区别不大，且方案A的"自动建表检查"稳态根本不发生。** 客户端行为（`parInsertSql.c:2382-2398 parseUsingTableNameImpl`）：表 meta 已在客户端 catalog 缓存时（`ctbCacheHit=true`）走 `ignoreUsingClauseAndCheckTagValues()`——tags 子句仍被完整解析（校验后丢弃），但**跳过 `buildCreateTbReq`**，提交消息不带 `pCreateTbReq`；服务端 `vnodeHandleAutoCreateTable` 对 `pCreateTbReq == NULL` 的表直接跳过（`vnodeSvr.c:2312-2314`）。自动建表的服务端开销只发生在"客户端没见过这张表"的写入上。

稳态对比：

| 环节 | 方案A（带 USING TAGS） | 方案B（裸 INSERT） |
|---|---|---|
| 服务端逐表 `metaGetInfo` ×2 + schema 校验 | 有（`sEntryCache` 哈希，微秒级） | **完全一样** |
| 服务端 memtable 插入 / WAL / raft | 相同 | 相同 |
| 服务端自动建表检查 | **不发生**（客户端不带建表请求） | 无 |
| 客户端 tags 文本 + 逐 tag 解析 | **每次都付**：`parseTagsClause` 逐值 `parseTagValue` + 构建 STag 再丢弃（`parInsertSql.c:180-198`） | 无 |
| SQL 文本长度 / tokenize / 网络字节 | 多 ~100~200B×表数 | 省 |

即：建表/改 tags/改 field 是一次性操作，但 **tags/fields 文本解析不是一次性的，是每批次每表都付的客户端 CPU**——这是方案A稳态唯一持续多出的成本，量级中等偏小（tags 数 ≪ fields 数，文本多 5~10%）。

**方案B写入前照样要检查元数据，一个都躲不掉**：

- 客户端每表必须从 catalog 缓存取 meta（schema/uid/vgroup），未命中则 missCache → 每表一次**串行** catalog 往返；**预建的表同样不在客户端缓存，方案B新表首写照样付**（5.6 收益 e"不存在"的原因）；
- 服务端 `vnodeHandleDataWrite` 每表 `metaGetInfo` ×2 + schema version 校验（`vnodeSvr.c:2452-2488`）——写数据本身的必要校验，两方案都有。

方案B免掉的只是"自动建表"这个**附加路径**（name B-tree 存在性检查 + 一次性 7 个 TDB 写入创建），不是写入本身的元数据校验。

**结论**：稳态吞吐两者基本无差别；方案B的真实收益在新表首写（免 name B-tree 检查 + 创建移出写入 apply）和客户端 tags 文本/解析。方案A多付的这点客户端 CPU 买到的是**随时改 tags/列定义不用 ALTER 的灵活性**——schema 变更频繁则花得值，schema 稳定则方案B值得做（完整收益分解见 5.6）。

## 八、参考

- [官方：高效写入（七要素 + 连接器高效写入特性）](https://docs.taosdata.com/developer-guide/high-throughput/)
- [官方：maxSqlLength 配置](https://docs.taosdata.com/operations-and-tooling/components/taosc/#maxsqllength)
- [容量规划（本目录 README）](./README.md)
