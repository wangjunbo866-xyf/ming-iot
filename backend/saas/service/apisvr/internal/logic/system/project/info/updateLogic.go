package info

import (
	"context"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateLogic {
	return &UpdateLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateLogic) Update(req *types.ProjectInfo) error {
	_, err := l.svcCtx.ProjectM.ProjectInfoUpdate(l.ctx, ToProjectPb(req))
	if err != nil {
		er := errors.Fmt(err)
		l.Errorf("%s.rpc.ProjectManage req=%v err=%v", utils.FuncName(), req, er)
		return er
	}
	return nil
}
