package area

import (
	"context"
	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system/area/info"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/def"
	"gitee.com/ming-iot/share/utils"
)

func ToDataAreaDetail(ctx context.Context, svcCtx *svc.ServiceContext, in []*sys.DataArea, areaMap map[int64]*sys.AreaInfo) (ret []*types.DataAreaDetail) {
	if in == nil {
		return
	}
	for _, v := range in {
		var ui *types.UserCore
		if svcCtx != nil && v.TargetType == def.TargetUser {
			u, err := svcCtx.UserCache.GetData(ctx, v.TargetID)
			if err != nil {
				continue
			}
			ui = utils.Copy[types.UserCore](u)
		}
		ret = append(ret, &types.DataAreaDetail{User: ui, TargetType: v.TargetType, UpdatedTime: v.UpdatedTime, TargetID: v.TargetID, AuthType: v.AuthType, IsAuthChildren: v.IsAuthChildren, AreaInfo: info.ToAreaInfoTypes(areaMap[v.AreaID])})
	}
	return
}

func ToAreaPbs(in []*types.DataArea) (ret []*sys.DataArea) {
	if in == nil {
		return
	}
	for _, v := range in {
		ret = append(ret, &sys.DataArea{AreaID: v.AreaID, AuthType: v.AuthType, IsAuthChildren: v.IsAuthChildren})
	}
	return
}
