package sysdirect

import (
	"context"
	"fmt"
	"sync"

	"gitee.com/ming-iot/saas/service/syssvr/internal/config"
	accessmanageServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/accessmanage"
	appmanageServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/appmanage"
	areamanageServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/areamanage"
	commonServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/common"
	datamanageServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/datamanage"
	deptMServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/departmentmanage"
	dictMServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/dictmanage"
	logServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/log"
	modulemanageServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/modulemanage"
	notifymanageServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/notifymanage"
	opsServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/ops"
	projectmanageServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/projectmanage"
	rolemanageServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/rolemanage"
	tenantmanageServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/tenantmanage"
	usermanageServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/usermanage"
	"gitee.com/ming-iot/saas/service/syssvr/internal/startup"
	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/saas/share/localdev"
	"gitee.com/ming-iot/share/i18ns"
	"gitee.com/ming-iot/share/interceptors"
	"gitee.com/ming-iot/share/services"
	"gitee.com/ming-iot/share/utils"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type Config = config.Config

var (
	ctxSvc     *svc.ServiceContext
	svcOnce    sync.Once
	runSvrOnce sync.Once
	c          config.Config
)

func GetSvcCtx() *svc.ServiceContext {
	svcOnce.Do(func() {
		utils.ConfMustLoad("etc/sys.yaml", &c)
		i18ns.InitWithFS("etc/i18n")
		ctxSvc = svc.NewServiceContext(c)
		startup.Init(ctxSvc)
		if !localdev.SkipStartupSideEffects() {
			startup.TableInit(ctxSvc)
		} else {
			logx.Infof("本地开发模式: 跳过 syssvr 启动表数据导入")
		}
		logx.Infof("enabled syssvr")
	})
	return ctxSvc
}

// RunServer 如果是直连模式,同时提供Grpc的能力
func RunServer(svcCtx *svc.ServiceContext) {
	runSvrOnce.Do(func() {
		utils.Go(context.Background(), func() {
			Run(svcCtx)
		})
	})
}

// registerServers 把 syssvr 的 15 个领域 gRPC 服务注册到 grpcServer。
// 从 Run 的 services.MustNewServer 回调中原样抽取，注册顺序、interceptor 和 startup 均未调整。
func registerServers(grpcServer grpc.ServiceRegistrar, svcCtx *svc.ServiceContext) {
	sys.RegisterUserManageServer(grpcServer, usermanageServer.NewUserManageServer(svcCtx))
	sys.RegisterAccessManageServer(grpcServer, accessmanageServer.NewAccessManageServer(svcCtx))
	sys.RegisterRoleManageServer(grpcServer, rolemanageServer.NewRoleManageServer(svcCtx))
	sys.RegisterAppManageServer(grpcServer, appmanageServer.NewAppManageServer(svcCtx))
	sys.RegisterModuleManageServer(grpcServer, modulemanageServer.NewModuleManageServer(svcCtx))
	sys.RegisterCommonServer(grpcServer, commonServer.NewCommonServer(svcCtx))
	sys.RegisterLogServer(grpcServer, logServer.NewLogServer(svcCtx))
	sys.RegisterProjectManageServer(grpcServer, projectmanageServer.NewProjectManageServer(svcCtx))
	sys.RegisterAreaManageServer(grpcServer, areamanageServer.NewAreaManageServer(svcCtx))
	sys.RegisterTenantManageServer(grpcServer, tenantmanageServer.NewTenantManageServer(svcCtx))
	sys.RegisterDataManageServer(grpcServer, datamanageServer.NewDataManageServer(svcCtx))
	sys.RegisterOpsServer(grpcServer, opsServer.NewOpsServer(svcCtx))
	sys.RegisterNotifyManageServer(grpcServer, notifymanageServer.NewNotifyManageServer(svcCtx))
	sys.RegisterDepartmentManageServer(grpcServer, deptMServer.NewDepartmentManageServer(svcCtx))
	sys.RegisterDictManageServer(grpcServer, dictMServer.NewDictManageServer(svcCtx))
}

func Run(svcCtx *svc.ServiceContext) {
	c := svcCtx.Config
	s := services.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		registerServers(grpcServer, svcCtx)
		//if c.Mode == service.DevMode || c.Mode == service.TestMode {
		reflection.Register(grpcServer)
		//}
	})
	defer s.Stop()
	s.AddUnaryInterceptors(interceptors.Ctxs, interceptors.Error)
	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
