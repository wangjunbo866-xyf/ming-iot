package crud

import (
	"context"

	"gitee.com/ming-iot/saas/service/apisvr/internal/logic"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"

	"github.com/zeromicro/go-zero/core/logx"
)

type IndexLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取项目crud列表
func NewIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *IndexLogic {
	return &IndexLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *IndexLogic) Index(req *types.ProjectCrudIndexReq) (resp *types.ProjectCrudIndexResp, err error) {
	dmResp, err := l.svcCtx.ProjectM.ProjectCrudIndex(l.ctx, utils.Copy[sys.ProjectCrudIndexReq](req))
	if err != nil {
		er := errors.Fmt(err)
		l.Errorf("%s.rpc.ProjectManage req=%v err=%+v", utils.FuncName(), req, er)
		return nil, er
	}

	return &types.ProjectCrudIndexResp{
		PageResp: logic.ToPageResp(req.Page, dmResp.Total),
		List:     utils.CopySlice[types.ProjectCrud](dmResp.List),
	}, nil
}
