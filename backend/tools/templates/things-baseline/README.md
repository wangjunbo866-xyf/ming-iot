# Ming IoT Things

这是 Ming IoT 的 Things 服务 L0 兼容迁移基线，来源固定为 `{{SOURCE_REPOSITORY}}` 的 commit `{{SOURCE_COMMIT}}`。

## 服务入口

- API：`./service/apisvr`
- 设备网关：`./service/dgsvr`
- 设备管理：`./service/dmsvr`

## 本地验证与构建

先进入 `backend/things`，再执行：

```bash
GOWORK=off go test ./...
GOWORK=off go vet ./...
make build
```

本目录只证明 L0 可编译兼容基线，不代表已经完成 Kafka、模块级物模型、Reducer 或 Scene Runtime 改造。禁止未经后续 Loop 门禁直接用于生产部署。
