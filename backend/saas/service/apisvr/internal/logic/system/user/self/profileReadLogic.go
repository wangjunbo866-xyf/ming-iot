package self

import (
	"context"

	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/stores"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProfileReadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewProfileReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProfileReadLogic {
	return &ProfileReadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ProfileReadLogic) ProfileRead(req *types.UserProfileReadReq) (resp *types.UserProfileReadResp, err error) {
	ret, err := l.svcCtx.UserRpc.UserProfileRead(l.ctx, utils.Copy[sys.WithCode](req))
	if err != nil {
		return nil, err
	}
	resp = &types.UserProfileReadResp{
		UserProfile: utils.Copy2[types.UserProfile](ret),
	}
	if req.WithProjects {
		ret2, err := l.svcCtx.ProjectM.ProjectInfoIndex(l.ctx, &sys.ProjectInfoIndexReq{Page: &sys.PageInfo{
			Page:   1,
			Size:   20,
			Orders: []*sys.PageInfo_OrderBy{{Field: "createdTime", Sort: stores.OrderAsc}},
		}})
		if err != nil {
			return nil, err
		}
		resp.Projects = system.ProjectInfosToApi(ret2.List)
	}
	return resp, err
}
