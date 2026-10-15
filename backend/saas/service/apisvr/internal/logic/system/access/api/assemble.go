package api

import (
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/utils"
)

func ToApiInfoPb(in *types.ApiInfo) *sys.ApiInfo {
	return utils.Copy[sys.ApiInfo](in)
}

func ToApiInfosTypes(in []*sys.ApiInfo) (ret []*types.ApiInfo) {
	for _, v := range in {
		ret = append(ret, ToApiInfoTypes(v))
	}
	return
}

func ToApiInfoTypes(in *sys.ApiInfo) *types.ApiInfo {
	return utils.Copy[types.ApiInfo](in)
}
