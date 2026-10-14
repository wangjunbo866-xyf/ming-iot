package svc

import (
	"gitee.com/ming-iot/saas/service/datasvr/internal/config"
	"gitee.com/ming-iot/saas/service/datasvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/service/syssvr/client/common"
	"gitee.com/ming-iot/saas/service/syssvr/client/log"
	role "gitee.com/ming-iot/saas/service/syssvr/client/rolemanage"
	tenant "gitee.com/ming-iot/saas/service/syssvr/client/tenantmanage"
	user "gitee.com/ming-iot/saas/service/syssvr/client/usermanage"
	"gitee.com/ming-iot/saas/service/syssvr/sysExport"
	"gitee.com/ming-iot/saas/service/syssvr/sysdirect"
	"gitee.com/ming-iot/saas/share/localdev"
	"gitee.com/ming-iot/saas/share/middlewares"
	"gitee.com/ming-iot/share/conf"
	"gitee.com/ming-iot/share/stores"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
)

type ServiceContext struct {
	Config         config.Config
	CheckTokenWare rest.Middleware
	Slot           sysExport.SlotCacheT
	InitCtxsWare   rest.Middleware
}

func NewServiceContext(c config.Config) *ServiceContext {
	stores.InitConn(c.Database)
	if localdev.SkipAutoMigrate() {
		logx.Infof("本地开发模式: 跳过 datasvr 数据库自动迁移")
	} else {
		logx.Must(relationDB.Migrate(c.Database))
	}
	var (
		TenantRpc tenant.TenantManage
		LogRpc    log.Log
		UserRpc   user.UserManage
		AuthRpc   role.RoleManage
		Common    common.Common
	)
	if c.SysRpc.Mode == conf.ClientModeDirect {
		TenantRpc = sysdirect.NewTenantManage(c.SysRpc.RunProxy)
		LogRpc = sysdirect.NewLog(c.SysRpc.RunProxy)
		UserRpc = sysdirect.NewUser(c.SysRpc.RunProxy)
		AuthRpc = sysdirect.NewRole(c.SysRpc.RunProxy)
		Common = sysdirect.NewCommon(c.SysRpc.RunProxy)
	} else {
		TenantRpc = tenant.NewTenantManage(c.SysRpc.MustNewClient())
		LogRpc = log.NewLog(c.SysRpc.MustNewClient())
		UserRpc = user.NewUserManage(c.SysRpc.MustNewClient())
		AuthRpc = role.NewRoleManage(c.SysRpc.MustNewClient())
		Common = common.NewCommon(c.SysRpc.MustNewClient())
	}
	Slot, err := sysExport.NewSlotCache(Common)
	logx.Must(err)
	return &ServiceContext{
		Config:         c,
		CheckTokenWare: middlewares.NewCheckTokenWareMiddleware(UserRpc, AuthRpc, TenantRpc, LogRpc).Handle,
		InitCtxsWare:   middlewares.InitMiddleware,
		Slot:           Slot,
	}
}
