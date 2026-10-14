package conf

import "github.com/zeromicro/go-zero/zrpc"

const (
	ClientModeGrpc   = "grpc"
	ClientModeDirect = "direct"
)

type RpcClientConf struct {
	Conf     zrpc.RpcClientConf `json:",optional"`
	Mode     string             `json:",default=direct,options=grpc|direct"` //direct如果服务没有使用则都是使用grpc
	RunProxy bool               `json:",default=true"`                       //是否开启grpc服务
	Enable   bool               `json:",default=true"`
}

// MustNewClient 创建 RPC 客户端，默认关闭 go-zero duration interceptor 的失败日志打印
func (c RpcClientConf) MustNewClient() zrpc.Client {
	c.Conf.Middlewares.Duration = false
	return zrpc.MustNewClient(c.Conf)
}

type ModeConf struct {
}
