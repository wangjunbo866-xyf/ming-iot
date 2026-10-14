# EventBus 包使用说明

EventBus 是一个统一的消息队列抽象层，支持 NATS 和 MQTT 两种消息队列系统。

## 特性

- 🚀 **统一接口**: 提供与消息队列无关的统一 API
- 🔄 **多协议支持**: 同时支持 NATS 和 MQTT
- 🎯 **主题转换**: 自动处理 NATS 和 MQTT 之间的主题格式转换
- ⚡ **高性能**: 异步消息处理，支持队列订阅

## 快速开始

### 1. 基本配置

```go
import (
    "gitee.com/ming-iot/share/conf"
    "gitee.com/ming-iot/share/eventBus"
)

// NATS 配置
config := conf.EventConf{
    Mode: conf.EventModeNats,
    Nats: conf.NatsConf{
        Url: "nats://localhost:4222",
    },
}
```

### 2. 创建事件总线

```go
// 创建 FastEvent 实例
bus, err := eventBus.NewFastEvent(config, "my-service", 1)
if err != nil {
    log.Fatal(err)
}

// 启动事件总线
if err := bus.Start(); err != nil {
    log.Fatal(err)
}
```

### 3. 订阅消息

```go
// 定义消息处理函数
handler := func(ctx context.Context, ts time.Time, body []byte) error {
    fmt.Printf("收到消息: %s\n", string(body))
    return nil
}

// 订阅主题
err := bus.Subscribe("server.test.echo", handler)

// 队列订阅（负载均衡）
err = bus.QueueSubscribe("server.test.queue", handler)
```

### 4. 发布消息

```go
ctx := context.Background()

// 发布简单消息
err := bus.Publish(ctx, "server.test.echo", "Hello World")

// 发布结构化数据
type User struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

user := User{ID: 1, Name: "张三"}
err = bus.Publish(ctx, "server.user.created", user)
```

## 高级用法

### 订阅管理

```go
// 订阅并获取订阅 ID
id, err := bus.SubscribeWithID("server.test.echo", handler)

// 使用 ID 取消订阅
err = bus.UnSubscribeWithID("server.test.echo", id)
```

### 主题通配符

```go
// 订阅所有 server 开头的消息
bus.Subscribe("server.*", handler)

// 订阅所有 server 及其子主题的消息
bus.Subscribe("server.>", handler)
```

### 错误处理

```go
handler := func(ctx context.Context, ts time.Time, body []byte) error {
    if err := processMessage(body); err != nil {
        return fmt.Errorf("处理消息失败: %w", err)
    }
    return nil
}
```

## 配置说明

### NATS 配置

```go
type NatsConf struct {
    Url   string // NATS 服务器地址
    User  string // 用户名（可选）
    Pass  string // 密码（可选）
    Token string // 认证令牌（可选）
}
```

### MQTT 配置

```go
type MqttConf struct {
    ClientID string   // 客户端 ID
    Brokers  []string // MQTT 代理地址列表
    User     string   // 用户名（可选）
    Pass     string   // 密码（可选）
    ConnNum  int      // 连接数量
}
```

## 主题转换规则

| NATS 主题 | MQTT 主题 | 说明 |
|-----------|-----------|------|
| `server.test` | `$inner/server/test` | 点号转换为斜杠 |
| `server.*.echo` | `$inner/server/+/echo` | 通配符转换 |
| `server.>` | `$inner/server/#` | 多级通配符转换 |

## 最佳实践

### 1. 服务启动顺序

```go
func main() {
    // 1. 创建事件总线
    bus, err := eventBus.NewFastEvent(config, "my-service", 1)
    
    // 2. 注册所有订阅
    registerSubscriptions(bus)
    
    // 3. 启动事件总线
    bus.Start()
    
    // 4. 启动其他服务
    startOtherServices()
}
```

### 2. 错误处理

```go
handler := func(ctx context.Context, ts time.Time, body []byte) error {
    defer func() {
        if r := recover(); r != nil {
            logx.WithContext(ctx).Errorf("消息处理发生 panic: %v", r)
        }
    }()
    
    return processBusinessLogic(ctx, body)
}
```

### 3. 性能优化

```go
// 使用队列订阅进行负载均衡
bus.QueueSubscribe("heavy.task", heavyTaskHandler)

// 避免在消息处理函数中进行耗时操作
handler := func(ctx context.Context, ts time.Time, body []byte) error {
    go func() {
        processHeavyTask(ctx, body)
    }()
    return nil
}
```

## 测试

```go
func TestEventBus(t *testing.T) {
    config := conf.EventConf{
        Mode: conf.EventModeNats,
        Nats: conf.NatsConf{
            Url: "nats://localhost:4222",
        },
    }
    
    bus, err := eventBus.NewFastEvent(config, "test-service", 1)
    assert.NoError(t, err)
    
    received := make(chan string, 1)
    bus.Subscribe("test.topic", func(ctx context.Context, ts time.Time, body []byte) error {
        received <- string(body)
        return nil
    })
    
    bus.Start()
    bus.Publish(context.Background(), "test.topic", "test message")
    
    select {
    case msg := <-received:
        assert.Equal(t, "test message", msg)
    case <-time.After(5 * time.Second):
        t.Fatal("消息接收超时")
    }
}
```

## 常见问题

**Q: 如何处理消息重复发送？**
A: 在消息处理函数中实现幂等性，或者使用消息 ID 进行去重。

**Q: 如何确保消息不丢失？**
A: 使用队列订阅，并确保消息处理函数正确返回错误。

**Q: 支持哪些消息格式？**
A: 支持任意格式的消息，建议使用 JSON 格式以便跨语言兼容。

## 版本兼容性

- Go 1.18+
- NATS 2.0+
- MQTT 3.1.1+