---
status: local-candidate
version: 0.1.0
date: 2026-10-17
---

# 隔离通知开发与验证

用于修改通知前取得真实运行对照，或在改动后重跑相同输入。它启动当前 syssvr，通过真实 gRPC 调用通知业务，再读取 PostgreSQL 中的消息与接收者记录；不替代[当前通知规格](../specs/saas/notification-trigger/spec.md)。本次结果见[试跑记录](../changelog/2026-10-17-notification-runtime-baseline.md)。

## 环境与启动

从当前候选仓根执行；需要 Go、Python 3、Docker Engine 和 Compose。Docker 固定本机 Unix socket，使用专属项目 `ming-iot-notification-baseline`、internal 网络及本项目持久卷；不发布宿主端口，也不复用旧环境。Windows 使用同一个 WSL 环境，不能把命令转到另一套 Docker 后将结果混用。

实际依赖为 PostGIS、Redis、NATS JetStream 和本仓构建的 syssvr；无需 etcd。普通 PostgreSQL 缺 geometry，不能替代 PostGIS。镜像准备、准确版本和入口见[部署说明](../../deploy/README.md)。运行阶段只使用本机缓存镜像，首次镜像及 Go 依赖准备需要开发网络；这不表示容器运行时允许访问外网。

配置中的密码和 token 字符串是这个封闭测试 profile 的公开样例，不是旧环境凭据。禁止将该配置用于生产或开启宿主端口；短信、UniPush 和定时服务调用关闭。测试使用内部 RPC 上下文，不证明登录或 HTTP 鉴权。

完整复验入口：

```bash
make verify-integration VERIFY_SUITE=notification
```

该入口构建候选、启动必要服务、发送四种样例、快照本任务已有消息、停止再启动、比较旧记录并再次发送；退出时停止本项目服务，保留数据。实际可执行范围以根 Makefile 和 profile 为准，失败退出不得记作通过。

需要保持服务运行以开发时，按下面的步骤单独操作：

```bash
make notification-baseline-contract
make notification-baseline-build
make notification-baseline-up
make notification-baseline-verify
make notification-baseline-logs
make notification-baseline-stop
```

每次代码或配置改变后重新 build，再 up 和 verify。服务启动成功不能代替 verify；仅复用旧镜像的测试也不能证明新候选。需要记录证据时固定代码 Head，并记录实际镜像 ID、构建产物 hash、命令退出码和日志；既有产品启动日志中的 gitInfo 不能替代该绑定。

## 输入和预期结果

probe 每次建立唯一 `nb-` 标记的样例用户和模板，调用真实 NotifyManage.NotifyConfigSend；不覆盖、清空或删除旧记录。当前 AutoMigrate 在独立库建立所需表及默认通知配置。

| 分支 | 真实 RPC 的正文预期 | 共同校验 |
| --- | --- | --- |
| manual / message | 保留模板正文，不插入触发人行 | subject、body、触发类型/ID/昵称/账号、通知类型、租户与唯一接收者关联 |
| auto / message | 系统自动触发行 + 原正文 | 同上 |
| deviceButton / message | 按键触发行 + 原正文，沿用账号优先 | 同上 |
| deviceButton / systemNotice | 仅按键触发行 | 同上；UniPush 关闭，只证明持久化与正文 |

manual 昵称优先是格式化函数的规则，当前 SendNotifyMsg 在 ruleScene/manual 分支不调用它追加前缀，详见主规格 CALL-SITES。运行基线按真实调用边界取预期，不能为了展示昵称而发明一条业务路径。直接格式化规则另由现有通知单测覆盖。

成功结果包含逐分支 PASS、唯一 marker、message ID、此前样例数。测试失败时保留日志与数据，先检查失败步骤、镜像构建、依赖健康和规格预期，再修复；不通过清库或绕过分支取得绿色。`stop` 保留容器及卷，重复 verify 会追加测试记录。

## 验证范围

已覆盖真实 RPC、模板渲染、触发人信息、事务落库、接收者关系及持久数据重启复验。现有单独 syssvr 仅初始化 NATS 并注册事件，未启动 FastEvent 消费；通知投递本身也未经过 Eventbus。

未覆盖 HTTP 鉴权、场景/设备告警上游、真实设备、外部推送、NATS 业务消费、Kafka 投递/故障恢复、全平台发布或生产验收。[Kafka R1](../specs/platform/eventbus/patches/PATCH-20261017-kafka-migration/change.md)需要另选实际事件验收链；此基线只承担通知回归。

默认 `make verify-pr` 仍是无外部服务检查；上面的 integration 需要明确选择。保留旧环境、旧仓、私有备份和历史回执，任何数据回收另行决定。
