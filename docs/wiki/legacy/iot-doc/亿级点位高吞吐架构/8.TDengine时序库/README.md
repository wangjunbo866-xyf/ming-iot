# TDengine 3.3.6.34 (LTS) 三节点集群容量规划

> 场景：百万设备 IoT 遥测接入 | 部署方式：官方 Helm Chart（K8s）| 版本：3.3.6.34 (LTS)
> 计算依据：TDengine [官方容量规划公式（内存/CPU/存储/网络）](https://docs.taosdata.com/operations-and-tooling/operations/planning/)

---

## 一、业务输入与假设

| 项目 | 取值 |
|---|---|
| 设备数 | 1,000,000 |
| 数据模型 | 500 测点拆 3 张超级表 → 每设备 3 张子表，共 **300 万子表** |
| 单条消息 | 400 float(4B) + 50 int(4B) + 50 varchar(50) + ts(8B) |
| 上报频率 | 每设备 5 分钟 1 条，每天 12 小时 → **144 条/设备/天** |
| 集群拓扑 | 3 dnode，replica 3（三副本高可用） |

假设说明：

- 500 点位按 200/200/100 拆 3 表，每条消息在 3 张子表各写入 1 行，总数据量不变
- 字符串按满 50 字符保守估算（最坏情况）
- 保留周期（KEEP）以 90 天为示例，其余周期见存储小节

## 二、数据量测算

单行原始大小：

```
ts 8B + 400×4B(float) + 50×4B(int) + 50×50B(string) ≈ 4.3 KB/消息
```

| 指标 | 数值 |
|---|---|
| 写入行速率 | 100万 ÷ 300s ≈ 3,333 消息/s → **10,000 行/s（167 万测点/s）** |
| 日增行数 | 100万 × 144 × 3 = **4.32 亿行/天** |
| 日增原始数据 | 100万 × 144 × 4.3KB ≈ **620 GB/天（裸数据）** |
| 入口带宽（原始） | 620GB ÷ 43200s ≈ **14.4 MB/s** |

## 三、存储容量

官方口径：压缩率通常**不低于 5 倍**。本场景字符串占报文约 58%（压缩偏差），保守按 **5 倍** 规划：

- 逻辑数据（单副本）：620 ÷ 5 ≈ **124 GB/天**
- 物理数据（replica 3）：124 × 3 ≈ **372 GB/天（全集群）**，每节点约 124 GB/天

按保留周期折算**每节点**数据盘（已含副本分摊）：

| 保留周期 | 集群物理总量 | 每节点 | 建议 dataSize（+30% 余量 + WAL） |
|---|---|---|---|
| 30 天 | ~11.2 TB | ~3.7 TB | **5 Ti** |
| 90 天 | ~33.5 TB | ~11.2 TB | **15 Ti** ✅ 本文示例 |
| 180 天 | ~67 TB | ~22.3 TB | **29 Ti** |
| 365 天 | ~136 TB | ~45 TB | **58 Ti** |

磁盘要求：

- **NVMe SSD 或高 IOPS 云盘**，K8s 侧对应独立 StorageClass
- 如需降本：启用多级存储，热数据（近 7 天）SSD、冷数据大容量 HDD/对象存储
- 官方 Chart 默认 `storage.dataSize: 100Gi`，**必须修改**

## 四、内存

官方公式：`vgroups × replica × (buffer + pages × pagesize + cachesize)`

**vgroup 规划**：300 万子表，每 vnode 控制在 10 万级子表 → **vgroups = 30**

- 全集群 vnode 数：30 × 3 = 90，**每节点 30 个 vnode**
- 注意：三副本下每节点都持有全量数据的一份副本，各节点负载相同

单 vnode 内存（buffer=128MB, pages=256, pagesize=4KB, cachesize=64MB，开启 last_row 缓存）：

```
128MB + 256×4KB + 64MB ≈ 193 MB
```

- 每节点 taosd 数据缓存：30 × 193MB ≈ **6 GB**（保底占用）

### 每节点 64 GB 的推导

官方公式只能算出约 6 GB，64 GB 是在此基础上叠加的实际占用与余量：

| 组成 | 估算 | 说明 |
|---|---|---|
| vnode 缓存（公式值） | ~6 GB | 30 vnode × 193MB，写缓冲 + 页缓存 + last_row 缓存 |
| 表元数据缓存 | 1~2 GB | 300 万子表 schema/meta 在 vnode 侧缓存（10 万表/vnode） |
| 查询执行内存 | 4~10 GB | 超级表聚合/降采样/并发查询工作集——官方明确"难以预测"，为最大不确定项 |
| RPC/连接缓冲 | 1~2 GB | `rpcQueueMemoryAllowed` 默认按物理内存 10% 预留 |
| mnode + 内部组件 | ~0.5 GB | 元数据管理、心跳、选举 |
| 辅助进程 | ~0.5 GB | taosAdapter/taosKeeper/taos-explorer（官方：约为 taosd 的 1/10 到几分之一） |
| OS page cache + K8s 系统 | 4~8 GB | 页缓存是读查询性能关键，另含 kubelet/系统 Pod |
| **实际工作负载小计** | **16~24 GB** | |
| 余量系数 ×2.5 | ≈48~64 GB | 与官方"CPU ≤50% 水位"同构的水位思想，应对查询尖峰 |
| **取整机型规格** | **64 GB** | 重查询场景 128 GB（更充裕 page cache，热数据全内存命中） |

> 右-size 方法：可先按 32 GB 部署，用 taosKeeper + Prometheus 观察 taosd RSS 与查询内存峰值，跑到 60~70% 再升配（K8s 改 limits 滚动重启即可）。本方案直接按 64 GB 固化，省去初期调参。

## 五、CPU

| 维度 | 测算 |
|---|---|
| 写入 | 官方口径：单核每秒 ≥10,000 写入请求。本场景 1 万行/s ≈ 1~2 核，瓶颈不在写入 |
| 分片（官方文档口径） | 每核可服务 1~2 个 vnode：90 vnodes ÷ (1~2) = 全集群 45~90 核 → **每节点 15~30 核** |
| 查询余量（经验值） | 官方明确查询消耗"难以预测，需按实际场景压测"；超级表聚合查询预留余量 |

**两档建议**：

- **下限档（纯官方口径）：每节点 24~32 核** —— 满足 vnode 配比 + 写入 + 副本同步，轻查询场景够用
- **推荐档（叠加查询余量）：每节点 32~48 核** —— 承载超级表聚合/降采样等重查询

无论哪档，日常 CPU 利用率保持 ≤ 50%（官方红线），超线即横向扩容或加核。

## 六、网络

| 流量 | 估算 |
|---|---|
| 业务入口 | 14.4 MB/s × 1.5（协议开销）≈ 22 MB/s |
| 副本复制（leader → 2 follower） | ~2× 写入流量 ≈ 29 MB/s/节点 |
| 每节点合计（含查询） | ~60 MB/s ≈ 0.5 Gbps |

- 千兆为底线，**推荐万兆（10GbE）**
- 集群内部流量独立 VLAN，丢包率 < 0.01%
- 开放端口：6030（taosc）、6041（REST/WebSocket）、6043（taosKeeper）、6060（taosExplorer）

## 七、客户端（写入网关侧）

- 原生 taosc：`M = T×S×3 + N/4096 + 100`（N=300万表, T=100线程, S=1MB）≈ **1.2 GB**
- **推荐 WebSocket/RESTful**：每连接固定 ~8MB，与集群升级解耦
- 批量写入：按设备聚合 ≥1000 行/批 INSERT，WebSocket 开启 `batchfetch=true`

## 八、推荐配置汇总

**K8s Worker 节点 × 3**（TDengine Pod 独占，配置反亲和）：

| 资源 | 推荐值 |
|---|---|
| CPU | 固定 32 核（独占 Pod，requests=limits，Guaranteed QoS；重查询可升至 48，见 CPU 小节） |
| 内存 | 64 GB（重查询 128 GB） |
| 数据盘 | 15 Ti NVMe（KEEP 90 天示例，见存储表） |
| 网络 | 万兆 |

**Helm values**：见同目录 [`tdengine-values.yaml`](./tdengine-values.yaml)

**相关文档**：[高效写入：写入能力评估与改进计划](./high-throughput-writing.md)

**建库 SQL**：

```sql
CREATE DATABASE iot
  PRECISION 'ms'
  VGROUPS 30
  REPLICA 3
  BUFFER 128
  PAGES 256 PAGESIZE 4
  CACHEMODEL 'both' CACHESIZE 64
  DURATION 10d
  KEEP 90d;          -- 按实际保留周期调整，dataSize 同步调整
```

**建表示例**（3 张超级表按 200/200/100 点位拆分，列名示意）：

```sql
USE iot;
-- 表1：float 点位 200 个
CREATE STABLE st_float_a (ts TIMESTAMP, f001 FLOAT, ..., f200 FLOAT)
  TAGS (product_id VARCHAR(32), module_id VARCHAR(32), location VARCHAR(64));
-- 表2：float 200 + int 50
CREATE STABLE st_float_b (ts TIMESTAMP, f201 FLOAT, ..., f400 FLOAT,
  i001 INT, ..., i050 INT) TAGS (...);
-- 表3：string 50
CREATE STABLE st_str (ts TIMESTAMP, s001 VARCHAR(50), ..., s050 VARCHAR(50))
  TAGS (...);
```

## 九、部署与验证

```bash
# 1. 确认 StorageClass
kubectl get sc

# 2. 安装（官方 Chart，需 >= 3.5.0 的 services.* 新 schema，与 values 文件匹配）
helm install tdengine tdengine-3.5.0.tgz -f tdengine-values.yaml
# 或添加 repo 后安装：helm repo add tdengine https://taosdata.github.io/TDengine-Operator

# 3. 验证集群
kubectl exec tdengine-0 -- taos -s "show dnodes; show mnodes;"

# 4. 建库建表后检查 vnode 分布是否均匀
kubectl exec tdengine-0 -- taos -s \
  "select * from information_schema.ins_vnodes;"

# 5. 监控：taosKeeper + Prometheus，关注 CPU（>50% 告警）、磁盘水位
```

## 十、规划依据：官方文档 vs 经验假设

> 官方依据：[TDengine 容量规划](https://docs.taosdata.com/operations-and-tooling/operations/planning/)（v3.3 文档）

**严格按官方文档的部分**：

| 项 | 文档口径 | 本文应用 |
|---|---|---|
| 内存公式 | `vgroups × replica × (buffer + pages × pagesize + cachesize)` | 原样使用 |
| CPU-分片 | 每个 CPU 核服务 1~2 个 vnode | 原样使用（两档建议的基准） |
| CPU-写入 | 单核每秒 ≥10,000 写入请求 | 原样使用 |
| CPU 水位 | 使用率不超 50%，超线扩容 | 原样使用 |
| 存储 | `numOfTables × rowSize × rows`，压缩率 ≥5 倍 | 原样使用 |
| 客户端内存 | `M = T×S×3 + N/4096 + 100`；WebSocket 每连接 8MB | 原样使用 |
| 网络 | 万兆 / 独立 VLAN / 丢包率 < 万分之一 | 原样使用 |

**文档未给出、属于经验假设的部分（可调参数）**：

1. **vgroups=30**："每 vnode 控制在 10 万级子表"为社区实践经验，由此推出 300万÷10万=30；
2. **buffer=128MB / cachesize=64MB**：文档说明"具体数值须根据实际情况调整"，此为推荐值；
3. **CPU 推荐档 32~48 核**：在官方 15~30 核/节点基础上叠加的查询余量，非文档要求；
4. **压缩率取 5x 下限**（字符串占比高，取保守值）、**+30% 存储余量**、网络带宽估算系数，均为保守化假设，建议用 taosBenchmark 实测压缩率后修正。

## 十一、关键风险与注意事项

1. **三副本下每节点存全量数据**：加节点只缓解查询压力，写入吞吐不随节点数提升；设备量翻倍时应扩到 6 节点并将 vgroups 翻倍。
2. **字符串列是压缩短板**：若 50 个字符串实际为枚举/重复值，压缩率可达 10 倍以上、存储减半——建议先用 1% 真实数据实测压缩率再最终定盘。
3. **预留 50% 资源余量** 应对业务增长与流量尖峰（官方建议）。
4. K8s 侧三个 Pod 必须反亲和调度到不同节点；使用有状态存储（StatefulSet + PVC），禁止多个 Pod 共用 PV。
5. 滚动重启/节点下线后 dnode 恢复需要时间，Chart 中 `startupProbe` 超时要调大，避免 Pod 反复重启。

## 十二、阿里云 ACK 节点规格推荐

> 本章补充 ACK 节点池（Node Pool）可选的 ECS 实例规格，用于承载 3 节点三副本 TDengine 集群（每个 Pod 固定 32C/64G）。

### 推荐 ECS 实例规格族

| 实例族 | CPU:内存 | 特点 | 适合场景 |
|---|---|---|---|
| **计算型 c9i / c8i**（Intel） | 1:2 | 最新 Intel Xeon 6 / Sapphire Rapids，高网络带宽、高 PPS | **最推荐**，32C/64G、64C/128G 刚好匹配 |
| **计算型 c9a / c8a**（AMD） | 1:2 | AMD EPYC Turin/Genoa，性价比高 | 同配置比 Intel 便宜 |
| **通用型 g9i / g8i**（Intel） | 1:4 | 内存更多，网络/磁盘性能同样强劲 | 需要更大 page cache 或未来重查询 |
| **本地 SSD 型 i4g / i5g** | 1:4 | 带 NVMe 本地盘，μs 级延迟 | 追求 WAL/落盘极致 I/O |

### 具体规格推荐表

Pod 固定 **32C/64G**，节点需 **>64G 内存**（给 OS/kubelet/系统 Pod 留余量）。

| 方案 | ECS 实例规格 | vCPU | 内存 | 网络带宽 | 云盘 IOPS | 本地 NVMe | 评价 |
|---|---|---|---|---|---|---|---|
| 经济型 | `ecs.c8i.8xlarge` / `ecs.c9i.8xlarge` | 32 | 64G | 20/25 Gbps | 20万 | 无 | ❌ 不推荐，节点几乎无余量 |
| **推荐** | `ecs.c8i.12xlarge` / `ecs.c9i.12xlarge` | 48 | 96G | 25/32 Gbps | 24-30万 | 无 | ✅ **最佳平衡** |
| 宽裕型 | `ecs.c8i.16xlarge` / `ecs.c9i.16xlarge` | 64 | 128G | 32-36 Gbps | 30-40万 | 无 | ✅ 查询余量大 |
| 高内存 | `ecs.g8i.8xlarge` / `ecs.g9i.8xlarge` | 32 | 128G | 20/25 Gbps | 20万 | 无 | 内存优先，CPU 同档 |
| 本地 NVMe | `ecs.i4g.8xlarge` / `ecs.i5g.8xlarge` | 32 | 128G | 16/25 Gbps | 15-20万 | 1×1919 GB | 写入 I/O 极致，但本地盘有单点风险 |

> 三节点集群建议购买 **3 台同规格**实例，形成统一节点池。

### 存储/StorageClass 选择

| 存储类型 | 推荐 | 说明 |
|---|---|---|
| **ESSD AutoPL** | ✅ 首选 | IOPS 随容量线性增长，适合云盘场景；用作 TDengine data 盘 |
| **ESSD PL-X** | 可选 | 更高单盘性能，但价格贵 |
| **本地 NVMe SSD**（i4g/i5g） | 写入极致 | 需要额外配本地盘 StorageClass 或 LVM；注意本地盘随实例释放，必须靠三副本保证数据安全 |

### 价格参考

阿里云 ECS 价格随地域、购买时长（包年包月/按量/抢占式）、活动优惠变化较大，建议用官方工具实时估算：

- [阿里云 ECS 价格计算器](https://www.aliyun.com/price/product?#/ecs/detail)

**选型一句话建议**：

- **预算优先**：`ecs.c8i.12xlarge` × 3（48C/96G，AMD 可选 `c8a.12xlarge` 更便宜）
- **性能优先**：`ecs.c9i.16xlarge` × 3（64C/128G，最新 Granite Rapids）
- **I/O 极致**：`ecs.i4g.8xlarge` × 3（32C/128G + 1.9TB NVMe 本地盘），但需接受本地盘生命周期绑定
