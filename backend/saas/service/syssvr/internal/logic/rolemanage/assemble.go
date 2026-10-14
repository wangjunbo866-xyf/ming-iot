package rolemanagelogic

import (
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/utils"
)

func ToRoleInfoRpc(in *relationDB.SysRoleInfo) *sys.RoleInfo {
	return utils.Copy[sys.RoleInfo](in)
}

func ToRoleInfosRpc(in []*relationDB.SysRoleInfo) []*sys.RoleInfo {
	var ret []*sys.RoleInfo
	for _, v := range in {
		ret = append(ret, ToRoleInfoRpc(v))
	}
	return ret
}
