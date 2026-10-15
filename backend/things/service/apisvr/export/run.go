package export

import (
	"gitee.com/ming-iot/share/i18ns"
	"gitee.com/ming-iot/share/services"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/apisvr/internal/config"
	"gitee.com/ming-iot/things/service/apisvr/internal/handler"
	"gitee.com/ming-iot/things/service/apisvr/internal/startup"
	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"github.com/zeromicro/go-zero/rest"
)

type (
	Config         = config.Config
	ServiceContext = svc.ServiceContext
	ApiCtx         struct {
		Server        *rest.Server
		SvcCtx        *ServiceContext
		NotInitHandle bool
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
		server = rest.MustNewServer(c.RestConf)
		apiCtx.Server = server
	}
	if !apiCtx.NotInitHandle {
		handler.RegisterHandlers(server, ctx)
	}
	startup.Init(apiCtx.SvcCtx)
	services.InitApisvrs(server)
	return apiCtx
}
