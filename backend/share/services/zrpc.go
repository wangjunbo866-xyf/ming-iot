package services

import (
	"gitee.com/ming-iot/share/interceptors"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

type (
	RegisterFn = func(*grpc.Server)
)

func MustNewServer(c zrpc.RpcServerConf, register RegisterFn) *zrpc.RpcServer {
	if c.Etcd.Key == "" && c.Name != "" {
		c.Etcd.Key = c.Name
	}
	// 默认关闭 go-zero stat 拦截器，避免大量 RPC 请求体/响应体日志
	c.Middlewares.Stat = false
	// 默认关闭 otel trace 上报，未部署 Jaeger 时会持续打印 [otel] error
	c.Telemetry.Name = ""
	server, err := zrpc.NewServer(c, register)
	logx.Must(err)
	// 使用项目自定义摘要拦截器覆盖官方 stat 能力：method/cost/status/大小/调用方
	server.AddUnaryInterceptors(interceptors.StatInterceptor())
	return server
}
