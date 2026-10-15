package appmanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type AppModuleMultiUpdateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewAppModuleMultiUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AppModuleMultiUpdateLogic {
	return &AppModuleMultiUpdateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *AppModuleMultiUpdateLogic) AppModuleMultiUpdate(in *sys.AppModuleMultiUpdateReq) (*sys.Empty, error) {
	if err := ctxs.IsRoot(l.ctx); err != nil {
		return nil, err
	}
	err := relationDB.NewAppModuleRepo(l.ctx).MultiUpdate(l.ctx, in.Code, in.ModuleCodes)
	return &sys.Empty{}, err
}
