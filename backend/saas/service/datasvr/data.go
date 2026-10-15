package main

import (
	"flag"
	"fmt"

	"gitee.com/ming-iot/share/i18ns"
	"gitee.com/ming-iot/share/utils"
	"github.com/zeromicro/go-zero/core/logx"

	"gitee.com/ming-iot/saas/service/datasvr/internal/config"
	"gitee.com/ming-iot/saas/service/datasvr/internal/handler"
	"gitee.com/ming-iot/saas/service/datasvr/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

func main() {
	flag.Parse()
	logx.DisableStat()
	var c config.Config
	utils.ConfMustLoad("etc/data.yaml", &c)
	i18ns.InitWithFS("etc/i18n")

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)
	server.PrintRoutes()
	fmt.Printf("Starting server at %s:%d...\n", c.Host, c.Port)
	server.Start()
}
