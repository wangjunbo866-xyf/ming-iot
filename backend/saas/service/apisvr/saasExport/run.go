package saasExport

import (
	"net/http"

	"gitee.com/ming-iot/saas/service/apisvr/internal/config"
	"gitee.com/ming-iot/saas/service/apisvr/internal/handler"
	"gitee.com/ming-iot/saas/service/apisvr/internal/handler/system/proxy"
	"gitee.com/ming-iot/saas/service/apisvr/internal/startup"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	_ "gitee.com/ming-iot/saas/service/datasvr/dataExport"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/i18ns"
	"gitee.com/ming-iot/share/services"
	"gitee.com/ming-iot/share/utils"
	"github.com/zeromicro/go-zero/rest"
)

type (
	ApiCtx struct {
		Server *rest.Server
		SvcCtx *svc.ServiceContext
	}
)

var (
	c config.Config
)

func NewApi(apiCtx ApiCtx) ApiCtx {
	utils.ConfMustLoad("etc/api.yaml", &c)
	i18ns.InitWithFS("etc/i18n")
	apiCtx = runApi(apiCtx)
	return apiCtx
}

func runApi(apiCtx ApiCtx) ApiCtx {
	var server = apiCtx.Server
	ctx := svc.NewServiceContext(c)
	apiCtx.SvcCtx = ctx
	if server == nil {
		server = rest.MustNewServer(c.RestConf, rest.WithCustomCors(func(header http.Header) {
			header.Set("Access-Control-Allow-Headers", ctxs.HttpAllowHeader)
			header.Set("Access-Control-Allow-Origin", "*")
		}, nil, "*"),
			rest.WithNotFoundHandler(proxy.Handler(ctx)),
		)
		apiCtx.Server = server
	}
	handler.RegisterHandlers(server, ctx)
	//handler.RegisterWsHandlers(apiCtx.SvcCtx.Ws, ctx)
	startup.Init(ctx)
	services.InitApisvrs(server)
	//dataExport.Run(server)
	return apiCtx
}
