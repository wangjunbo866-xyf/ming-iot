package info

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

// 获取部门列表
func NewIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *IndexLogic {
	return &IndexLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *IndexLogic) Index(req *types.DeptInfoIndexReq) (resp *types.DeptInfoIndexResp, err error) {
	ret, err := l.svcCtx.DeptM.DeptInfoIndex(l.ctx, utils.Copy[sys.DeptInfoIndexReq](req))
	if err != nil {
		return nil, err
	}
	return &types.DeptInfoIndexResp{
		PageResp: logic.ToPageResp(req.Page, ret.Total),
		List:     utils.CopySlice[types.DeptInfo](ret.List),
	}, nil
}
