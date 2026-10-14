package user

import (
	"context"
	"gitee.com/ming-iot/saas/service/apisvr/internal/logic"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type IndexLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取部门授权列表
func NewIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *IndexLogic {
	return &IndexLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *IndexLogic) Index(req *types.DeptUserIndexReq) (resp *types.DeptUserIndexResp, err error) {
	ret, err := l.svcCtx.DeptM.DeptUserIndex(l.ctx, utils.Copy[sys.DeptUserIndexReq](req))
	if err != nil {
		return nil, err
	}
	list := utils.CopySlice[types.DeptUser](ret.List)
	for i, v := range list {
		user, err := l.svcCtx.UserCache.GetData(l.ctx, v.UserID)
		if err != nil {
			l.Error(err)
		}
		list[i].User = utils.Copy[types.UserCore](user)
	}
	return &types.DeptUserIndexResp{
		PageResp: logic.ToPageResp(req.Page, ret.Total),
		List:     list,
	}, nil
}
