package sysdirect

import (
	"gitee.com/ming-iot/saas/service/syssvr/client/ops"
	client "gitee.com/ming-iot/saas/service/syssvr/client/usermanage"
	opsServer "gitee.com/ming-iot/saas/service/syssvr/internal/server/ops"
	server "gitee.com/ming-iot/saas/service/syssvr/internal/server/usermanage"

	clientNotify "gitee.com/ming-iot/saas/service/syssvr/client/notifymanage"
	serverNotify "gitee.com/ming-iot/saas/service/syssvr/internal/server/notifymanage"

	clientDeptM "gitee.com/ming-iot/saas/service/syssvr/client/departmentmanage"
	serverDeptM "gitee.com/ming-iot/saas/service/syssvr/internal/server/departmentmanage"

	clientRole "gitee.com/ming-iot/saas/service/syssvr/client/rolemanage"
	serverRole "gitee.com/ming-iot/saas/service/syssvr/internal/server/rolemanage"

	clientAccess "gitee.com/ming-iot/saas/service/syssvr/client/accessmanage"
	serverAccess "gitee.com/ming-iot/saas/service/syssvr/internal/server/accessmanage"

	clientData "gitee.com/ming-iot/saas/service/syssvr/client/datamanage"
	serverData "gitee.com/ming-iot/saas/service/syssvr/internal/server/datamanage"

	clientDict "gitee.com/ming-iot/saas/service/syssvr/client/dictmanage"
	serverDict "gitee.com/ming-iot/saas/service/syssvr/internal/server/dictmanage"

	clientModule "gitee.com/ming-iot/saas/service/syssvr/client/modulemanage"
	serverModule "gitee.com/ming-iot/saas/service/syssvr/internal/server/modulemanage"

	clientLog "gitee.com/ming-iot/saas/service/syssvr/client/log"
	serverLog "gitee.com/ming-iot/saas/service/syssvr/internal/server/log"

	clientCommon "gitee.com/ming-iot/saas/service/syssvr/client/common"
	serverCommon "gitee.com/ming-iot/saas/service/syssvr/internal/server/common"

	clientApp "gitee.com/ming-iot/saas/service/syssvr/client/appmanage"
	serverApp "gitee.com/ming-iot/saas/service/syssvr/internal/server/appmanage"

	clientTenant "gitee.com/ming-iot/saas/service/syssvr/client/tenantmanage"
	serverTenant "gitee.com/ming-iot/saas/service/syssvr/internal/server/tenantmanage"

	clientProject "gitee.com/ming-iot/saas/service/syssvr/client/projectmanage"
	serverProject "gitee.com/ming-iot/saas/service/syssvr/internal/server/projectmanage"

	clientArea "gitee.com/ming-iot/saas/service/syssvr/client/areamanage"
	serverArea "gitee.com/ming-iot/saas/service/syssvr/internal/server/areamanage"
)

func NewUser(runSvr bool) client.UserManage {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return client.NewDirectUserManage(svcCtx, server.NewUserManageServer(svcCtx))
}

func NewRole(runSvr bool) clientRole.RoleManage {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return clientRole.NewDirectRoleManage(svcCtx, serverRole.NewRoleManageServer(svcCtx))
}
func NewAccess(runSvr bool) clientAccess.AccessManage {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return clientAccess.NewDirectAccessManage(svcCtx, serverAccess.NewAccessManageServer(svcCtx))
}

func NewData(runSvr bool) clientData.DataManage {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return clientData.NewDirectDataManage(svcCtx, serverData.NewDataManageServer(svcCtx))
}

func NewDict(runSvr bool) clientDict.DictManage {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return clientDict.NewDirectDictManage(svcCtx, serverDict.NewDictManageServer(svcCtx))
}

func NewModule(runSvr bool) clientModule.ModuleManage {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return clientModule.NewDirectModuleManage(svcCtx, serverModule.NewModuleManageServer(svcCtx))
}

func NewCommon(runSvr bool) clientCommon.Common {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return clientCommon.NewDirectCommon(svcCtx, serverCommon.NewCommonServer(svcCtx))
}

func NewLog(runSvr bool) clientLog.Log {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return clientLog.NewDirectLog(svcCtx, serverLog.NewLogServer(svcCtx))
}

func NewApp(runSvr bool) clientApp.AppManage {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return clientApp.NewDirectAppManage(svcCtx, serverApp.NewAppManageServer(svcCtx))
}

func NewTenantManage(runSvr bool) clientTenant.TenantManage {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return clientTenant.NewDirectTenantManage(svcCtx, serverTenant.NewTenantManageServer(svcCtx))
}

func NewProjectManage(runSvr bool) clientProject.ProjectManage {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return clientProject.NewDirectProjectManage(svcCtx, serverProject.NewProjectManageServer(svcCtx))
}
func NewAreaManage(runSvr bool) clientArea.AreaManage {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return clientArea.NewDirectAreaManage(svcCtx, serverArea.NewAreaManageServer(svcCtx))
}

func NewOps(runSvr bool) ops.Ops {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return ops.NewDirectOps(svcCtx, opsServer.NewOpsServer(svcCtx))
}

func NewNotify(runSvr bool) clientNotify.NotifyManage {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return clientNotify.NewDirectNotifyManage(svcCtx, serverNotify.NewNotifyManageServer(svcCtx))
}

func NewDeptM(runSvr bool) clientDeptM.DepartmentManage {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	return clientDeptM.NewDirectDepartmentManage(svcCtx, serverDeptM.NewDepartmentManageServer(svcCtx))
}
