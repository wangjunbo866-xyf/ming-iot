package dataExport

import (
	"flag"
	"sync"

	"gitee.com/ming-iot/saas/service/datasvr/internal/config"
	"gitee.com/ming-iot/saas/service/datasvr/internal/handler"
	"gitee.com/ming-iot/saas/service/datasvr/internal/svc"
	"gitee.com/ming-iot/share/i18ns"
	"gitee.com/ming-iot/share/services"
	"gitee.com/ming-iot/share/utils"
	"github.com/zeromicro/go-zero/rest"
)

var once sync.Once

func init() {
	services.RegisterApisvr(func(server *rest.Server) error {
		once.Do(func() {
			flag.Parse()
			var c config.Config
			utils.ConfMustLoad("etc/data.yaml", &c)
			i18ns.InitWithFS("etc/i18n")
			ctx := svc.NewServiceContext(c)
			handler.RegisterHandlers(server, ctx)
		})
		return nil
	})
}

func Run(server *rest.Server) *rest.Server {
	flag.Parse()
	var c config.Config
	utils.ConfMustLoad("etc/data.yaml", &c)
	i18ns.InitWithFS("etc/i18n")

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)
	return server
}
