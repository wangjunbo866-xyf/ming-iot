package menu

import (
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/utils"
)

func ToMenuInfoRpc(in *types.MenuInfo) *sys.MenuInfo {
	return utils.Copy[sys.MenuInfo](in)
}
func ToMenuInfosRpc(in []*types.MenuInfo) (ret []*sys.MenuInfo) {
	for _, v := range in {
		ret = append(ret, ToMenuInfoRpc(v))
	}
	return
}
