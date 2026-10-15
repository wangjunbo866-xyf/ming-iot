# ming-iot 部署

## 根 Verification Profile 与部署边界

日常验证从仓库根目录调用统一入口：

```bash
make verify-fast VERIFY_SCOPE=tools
make verify-pr
make verify-integration VERIFY_SUITE=kafka
make verify-integration VERIFY_SUITE=things-startup
MING_HEAVY_VERIFY=1 make verify-release
```

`verify-pr` 只执行构建、Go 测试、vet 和仓库静态合同，不启动 Kafka、Things 或其他
外部服务。`verify-integration` 才会启动显式选择的本地 suite，并通过 shell trap 在成功或
失败后运行安全 `down`；它不删除卷、不重置数据库，也不连接生产环境。

`things-startup` 要求 `deploy/profiles/things-baseline/.env.local` 已存在、不是符号链接且
权限为 `0600`。条件不足时命令以 `NOT_RUN` 和非零状态退出，不会降级成 PASS。
`verify-release` 还要求显式设置 `MING_HEAVY_VERIFY=1`，但它仍只做候选验证，不执行发布或部署。

## Kafka 4.3.1 基础部署契约

Kafka 基础 profile 的统一入口是 `make -C deploy kafka-*`：

```bash
make -C deploy kafka-config
make -C deploy kafka-up
make -C deploy kafka-ready
make -C deploy kafka-bootstrap
make -C deploy kafka-down
```

该 profile 是固定 tag 与 digest 的单节点 KRaft local-only 环境，只用于本地开发和
集成探针。生产静态安全基线、显式 Topic Catalog 和清理说明见 `deploy/kafka/README.md`。
它不会替换或迁移现有 NATS 业务链路。

## Things 可重复启动基线

新的 Things 基线统一入口是 `make -C deploy things-baseline-*`。常用命令如下：

```bash
# 首次准备本地私密配置；复制后必须替换所有空密钥
cd deploy/profiles/things-baseline
install -m 0600 .env.example .env.local

# 回到仓库根目录
make -C deploy things-baseline-fast
make -C deploy things-baseline-contract
make -C deploy things-baseline-up
make -C deploy things-baseline-verify
make -C deploy things-baseline-logs
make -C deploy things-baseline-down
```

`things-baseline-contract` 使用临时 `0600` 环境文件执行合同、Compose 展开和静态安全扫描，
不构建二进制或镜像，也不启动容器。`things-baseline-fast` 保持原有构建与工具测试语义，并
复用该静态合同。
`things-baseline-up` 按合同构建五个服务镜像并顺序启动；`verify` 只复核现有服务；
`logs` 只导出有界脱敏日志。down 保留数据库和数据卷，不传 `--volumes`，也不执行数据库 SQL。

reset 不属于默认命令，也不存在 `things-baseline-reset` 目标。任何清库、删卷或重置能力都必须
另建带破坏性操作审批的 Todo，不能由日常命令隐式触发。

旧入口 `deploy/docker-compose.yml` 和 `deploy/saas/Dockerfile` 本轮保留，供既有 SaaS 环境兼容；
新的 Things 基线 Makefile 不会调用它们。

## 旧 SaaS 部署入口

本目录提供把 saas（单体 `saassvr`）跑起来的最小运行环境。

## 目录结构

```
deploy/
├── docker-compose.yml      # 最小运行环境（本目录的主入口）
├── saas/
│   └── Dockerfile          # 本地构建 saassvr 镜像（不依赖私有 registry）
└── docker/run-all/         # 上游 UnitedRhino 编排参考（依赖私有镜像，非本环境，.gitignore 忽略）
```

## 环境成员（目标要求的五个组件）

本 compose 定义了下列成员（`docker compose ps` 可见全部）：

| 组件 | 容器 | 来源 | saas 连接方式 |
| --- | --- | --- | --- |
| **postgres** | `postgres`（cloudx_net 既有） | postgis/17, pw=password, 库 iThings | DBType=pgsql（share GetConn 支持）；需 PostGIS 扩展 |
| **redis** | `ienv-redis`（cloudx_net 既有） | redis-7, 无密码 | cache + 节点 ID |
| **etcd** | `ming-iot-etcd`（本 compose） | bitnami/etcd:3.5 | 服务发现（direct 模式下 saas 不强依赖；grpc 拆分部署时用） |
| **kafka** | `ming-iot-kafka`（本 compose） | bitnami/kafka:3.7 KRaft | 作为环境成员就位（saas 当前事件层是 NATS；kafka 供平台其它服务/未来集成） |
| **saas** | `ming-iot-saas`（本 compose） | 本地构建 saassvr | 单体入口，HTTP 7777 |

> 消息层补充：saas 源码原生绑定 `nats.go`（share 事件总线 `options=nats\|natsJs\|direct`，无 Kafka 代码），运行时通过 cloudx_net 的 `ienv-nats`（JetStream）收发事件。本环境额外提供 Kafka 实例，满足「环境含 kafka」要求；saas 自身不改事件层（重写为 Kafka 需改外部 `share` module，违反 spec「不重写业务/不内嵌改 share」非目标）。

## 前置条件

`docker-compose.yml` 让 saas/etcd/kafka 加入名为 `cloudx_net` 的 external 网络，复用该网络里已在运行的 postgres/redis/nats。若该网络不存在，需先创建并启动这些 infra。

PostgreSQL 的 `iThings` 库需建库 + 启用 PostGIS（首次部署执行一次）：

```bash
docker exec postgres psql -U postgres -c 'CREATE DATABASE "iThings";'        # 库不存在时
docker exec postgres psql -U postgres -d iThings -c "CREATE EXTENSION IF NOT EXISTS postgis;"
docker exec postgres psql -U postgres -d iThings -c "CREATE EXTENSION IF NOT EXISTS postgis_topology;"
```

## 启动

```bash
cd deploy
docker compose up -d --build
docker compose logs -f saas   # 首次启动会 AutoMigrate 建表 + 导入 etc/init 种子数据
```

验证全部成员：

```bash
docker compose ps             # 应见 saas/etcd/kafka 三行（postgres/redis/nats 在 cloudx_net 既有）
# HTTP API（captcha 应返回 base64 图片）
curl -s -X POST http://localhost:7777/api/v1/system/user/self/captcha \
  -H 'Content-Type: application/json' -d '{"type":"image","use":"login"}'
```

端口：`7777`（saas HTTP）、`9180`（sys rpc，direct 模式不监听）、`7061`（dev server）、`12379/12380`（etcd，避开既有 2379/2380）、`19092`（kafka，避开常用 9092）。

## 配置机制

saas 通过 `confSuffix=Deploy` 加载 `*Deploy.yaml`：

- `apisvr/etc/apiDeploy.yaml` ← 入口（完整替换 `api.yaml`）
- `apisvr/etc/commonDeploy.yaml` ← DB/Redis/Nats/Rpc（被 `apiDeploy` 与 `sysDeploy` 前置合并）
- `apisvr/etc/sysDeploy.yaml` ← syssvr（`sysdirect` 加载）

密钥通过环境变量注入（`${CORE_DB_DSN}` 等占位由 `os.ExpandEnv` 展开）；最小环境用占位值，生产须替换为强随机值。
