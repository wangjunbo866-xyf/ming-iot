package info

import (
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/utils"
)

func ToProjectPb(in *types.ProjectInfo) *sys.ProjectInfo {
	return utils.Copy[sys.ProjectInfo](in)

}
