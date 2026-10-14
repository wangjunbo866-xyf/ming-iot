package main

import (
	"context"
	"fmt"

	"gitee.com/ming-iot/share/i18ns"
	"gitee.com/ming-iot/share/interceptors"
	"gitee.com/ming-iot/share/services"

	"gitee.com/ming-iot/things/share/rpcs/protocolSync/internal/config"
	protocolsyncServer "gitee.com/ming-iot/things/share/rpcs/protocolSync/internal/server/protocolsync"
	"gitee.com/ming-iot/things/share/rpcs/protocolSync/internal/startup"
	"gitee.com/ming-iot/things/share/rpcs/protocolSync/internal/svc"
	"gitee.com/ming-iot/things/share/rpcs/protocolSync/pb/protocolSync"

	"gitee.com/ming-iot/share/utils"
	"github.com/zeromicro/go-zero/core/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	defer utils.Recover(context.Background())
	var c config.Config
	utils.ConfMustLoad("etc/protocolSync.yaml", &c)
	i18ns.InitWithFS("etc/i18n")

	svcCtx := svc.NewServiceContext(c)
	startup.Init(svcCtx)
	s := services.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		protocolSync.RegisterProtocolSyncServer(grpcServer, protocolsyncServer.NewProtocolSyncServer(svcCtx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()
	s.AddUnaryInterceptors(interceptors.Ctxs, interceptors.Error)

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
