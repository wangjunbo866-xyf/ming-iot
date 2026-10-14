# NATS JetStream 三节点集群（官方 Helm Chart）

> [!WARNING]
> 本文档是历史 PoC/容量评估，不再是生产架构基线。现行决策是 Kafka Durable Raw + Durable Canonical，详见[企业级物联网平台总体架构设计](../enterprise-iot-platform-architecture-design.md)。本页的 JetStream stream/consumer/ACK、R3 和 3 天容量参数只用于历史对照，不得覆盖主设计中的 Kafka topic/transaction/offset/RF3 契约。

## 容量估算（消息约 5KB/条）

| 项 | 数值 |
|---|---|
| 常规速率 | 100万台 × 144条/天 = 1.44亿条/天，平均 3,333 msg/s ≈ 16.7 MB/s |
| 集中上报峰值 | 1分钟/次 → 16,667 msg/s ≈ 83 MB/s |
| 3 天保留量 | ≈ 2.16 TB 逻辑数据（S2 压缩后约减半） |
| R3 副本 | 每节点保存全量副本 → 每节点 3Ti SSD PVC |
| 网络 | 峰值含集群复制流量，节点间建议 ≥ 10GbE |

## 配置-吞吐对照表（R3、文件存储、生产建议值）

> 以下按消息 5KB/条估算，实际必须用 `nats bench` 在自己的磁盘/网络上验证（见文末）。
> R3 意味着集群内部复制流量约为业务写入量的 2 倍，磁盘写放大同理。

### 档位对照

| 档位 | 每节点 CPU / 内存 | 每节点磁盘 | 节点间网络 | 可支撑写入 | 适用场景 |
|---|---|---|---|---|---|
| 测试 | 2C / 4Gi | 100Gi 普通 SSD | 1GbE | ~1,000 msg/s（~5 MB/s） | 功能验证、联调 |
| **基线（当前 values）** | 8C / 16Gi（requests=limits） | 3Ti SSD | 10GbE | ~20,000 msg/s（~100 MB/s） | 100 万台 + 1 分钟集中上报 |
| 高负载 | 16C / 32Gi（requests=limits） | 3Ti+ NVMe | 25GbE | 50,000+ msg/s（~250 MB/s） | 多业务共用、未来扩容 |

> 各档位统一保持 **requests == limits（Guaranteed QoS）**，原因见下节。

### 关键配置与吞吐/资源的关系

| 配置 | 当前值 | 调大/调小的影响 | 生产建议 |
|---|---|---|---|
| `num_replicas` | 3 | 降到 1：吞吐约提升 2~3 倍，但丢节点即丢数据 | 生产必须 R3，靠加节点扩容而非降副本 |
| `compression` | `s2` | 开启：磁盘占用约减半、吞吐略降；关闭：CPU 更低 | Modbus 十六进制文本压缩率高，建议开启 |
| `max_age` | 3 天 | 保留越久，磁盘需求线性增长 | 保留时间只服务"重放验证"，长期数据尽快导出 Parquet |
| `max_bytes` | ~2.4TB | 到达上限后丢弃最老消息（`discard: old`） | 设为磁盘容量的 80% 左右，防止写爆 PVC |
| `write_deadline` | 60s | 磁盘慢时更大的容忍，但积压时延迟上升 | SSD 保持默认或 60s；机械盘不可用 |
| `max_payload` | 4MB | 大消息显著拉低 msg/s | 按实际报文上限设置，别留过大 |
| `duplicate_window` | 2 分钟 | 窗口越大，去重状态占用越多 | 与客户端重试间隔匹配即可 |
| `lame_duck_duration` | 120s | 滚动更新时客户端迁移时间 | 生产者多的场景保持 120s |

### 扩容判断信号

| 观察指标 | 说明 |
|---|---|
| JetStream API 延迟持续 > 100ms | CPU 或磁盘接近瓶颈 |
| 磁盘 util 持续 > 70% | 换更快磁盘或加节点拆 Stream |
| Stream 存储达 `max_bytes` 80% | 扩容 PVC 或缩短 `max_age` |
| 节点间网络 > 60% 带宽 | 复制流量挤占业务，升级网络或拆分集群 |

## 资源与 QoS（ACK 节点池场景）

ACK 节点池费用在购买 ECS 节点时已确定，`requests`/`limits` 只影响 Pod 在节点上的调度与分配，**不影响账单**。因此 NATS 设置为 requests == limits：

| 设计 | 原因 |
|---|---|
| Guaranteed QoS | 节点资源紧张时不被驱逐，保护 JetStream R3 quorum |
| 无 CPU 突发 | 集中上报峰值不会被 throttle，写入延迟平稳 |
| 按峰值预留 | NATS 独占节点，预留即使用，不存在“锁住浪费” |
| 无自动伸缩误判 | 若开启节点池弹性伸缩，autoscaler 按 requests 判断扩容，按峰值设置才不会扩不出来 |

如果压测后发现峰值远低于预期，可以整体下调数值，但保持 requests == limits。

## 部署前必改（values.yaml 中的 TODO）

1. `storageClassName: ssd-sc` → `kubectl get sc` 查集群实际的 SSD StorageClass
2. 所有 `CHANGE_ME_ROUTE_PASSWORD` / `CHANGE_ME_IOT_PASSWORD` 改为强随机密码（3 处需保持一致）

## 部署

```bash
helm repo add nats https://nats-io.github.io/k8s/helm/charts/
helm repo update

helm upgrade --install nats nats/nats \
  -n iot-msg --create-namespace \
  -f values.yaml
```

## 验证

```bash
# Pod 分散在 3 个节点上
kubectl -n iot-msg get pods -o wide

# 集群与 JetStream 状态
kubectl -n iot-msg exec deploy/nats-box -- nats server list
kubectl -n iot-msg exec deploy/nats-box -- nats stream info DEVICE_RAW

# 初始化 Job 日志（Stream 创建结果）
kubectl -n iot-msg logs job/nats-init-stream
```

## 客户端连接

集群内业务统一连：

```
nats://iot:<IOT_PASSWORD>@nats.iot-msg.svc.cluster.local:4222
```

## 安全说明

- 仅集群内使用，未开 TLS；已启用客户端账号认证和集群路由认证。
- 建议叠加 NetworkPolicy，只允许生产/消费方命名空间访问 4222：

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: nats-access
  namespace: iot-msg
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: nats
  policyTypes: [Ingress]
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              access-nats: "true"   # 需要访问的命名空间打此标签
      ports:
        - port: 4222
```

## 吞吐验证（nats bench）

部署后先用自带工具实测，不要只信估算：

```bash
# 写入压测：16 个生产者、5KB 消息、R3 副本
kubectl -n iot-msg exec deploy/nats-box -- \
  nats bench bench.test --pub 16 --size 5120 --msgs 1000000 --js --replicas 3

# 查看实时吞吐和延迟分布后，与上表对照判断档位是否够用
```

## 后续

- 3 天后长期分析走 Parquet：下一步加一个 Durable Consumer 服务（消费 `DEVICE_RAW` 批量写对象存储），不占 NATS 存储。
- 升级 NATS：修改 `values.yaml` 中 `container.image.tag` 后 `helm upgrade`，chart 会自动滚动更新（lame duck 优雅下线）。
