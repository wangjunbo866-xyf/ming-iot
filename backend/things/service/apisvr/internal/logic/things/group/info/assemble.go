package info

import (
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
)

func ToGroupInfosTypes(in []*dm.GroupInfo) []*types.GroupInfo {
	return utils.CopySlice[types.GroupInfo](in)

}

func ToGroupInfoTypes(in *dm.GroupInfo) *types.GroupInfo {
	return utils.Copy[types.GroupInfo](in)

}

func ToGroupInfoPbTypes(in *types.GroupInfo) *dm.GroupInfo {
	return utils.Copy[dm.GroupInfo](in)
}
