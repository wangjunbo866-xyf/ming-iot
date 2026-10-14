# Kafka 4.3.1 基础部署契约

本目录固定 Apache Kafka `4.3.1` KRaft 部署边界。`compose.yaml` 是单节点
combined broker/controller 的 **local-only** profile，仅用于本地开发和真实集成探针，
不能作为高可用或生产容量证明。

## 常用命令

```bash
make -C deploy kafka-config
make -C deploy kafka-up
make -C deploy kafka-ready
make -C deploy kafka-bootstrap
make -C deploy kafka-logs
make -C deploy kafka-down
```

宿主机固定通过 `127.0.0.1:39092` 访问，避免与仓库旧部署入口占用的 `19092`
冲突。Compose 映射与 broker advertised listener 使用同一个固定端口；脚本只使用仓库
相对路径和非敏感默认值。

`kafka-down` 不删除命名卷。确需丢弃本地 Kafka 数据时，必须由操作者显式运行
`docker compose -f deploy/kafka/compose.yaml down --volumes`；这是破坏性清理，不属于
日常 Makefile 目标。

## 配置边界

- `version-lock.json` 固定镜像 tag 和 registry digest，禁止 `latest`。
- `config/local/server.properties` 允许本地 PLAINTEXT、RF=1 和 `minISR=1`，并明确
  标注 local-only。
- `config/production/server.properties` 只是机器可校验的生产静态契约样例，不由
  本地 compose 启动；实际部署必须渲染节点身份、地址和证书，并通过已声明的
  `FileConfigProvider` 从外部挂载文件解析密码材料。
- `catalog/data-plane.yaml` 是显式 Topic 输入。Raw Topic 只冻结协议级模板，
  `bootstrap-topics.sh` 仅创建其中的 concrete `managed_topics`。
- 所有受管 Topic 使用 `LogAppendTime`、关闭 auto-create，并声明 generation、owner、
  key codec、retention、生产 RF 与 `minISR`。

本基础 Loop 不迁移现有 NATS 业务链路，也不提供通用字符串 Topic/EventBus API。
