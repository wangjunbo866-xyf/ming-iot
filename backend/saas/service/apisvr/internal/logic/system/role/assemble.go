package role

import (
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/utils"
)

func ToRoleInfoTypes(in *sys.RoleInfo) *types.RoleInfo {
	return utils.Copy[types.RoleInfo](in)
}
func ToRoleInfosTypes(in []*sys.RoleInfo) (ret []*types.RoleInfo) {
	for _, v := range in {
		ret = append(ret, ToRoleInfoTypes(v))
	}
	return
}
