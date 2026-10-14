package appmanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type AppInfoCreateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewAppInfoCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AppInfoCreateLogic {
	return &AppInfoCreateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *AppInfoCreateLogic) AppInfoCreate(in *sys.AppInfo) (*sys.WithID, error) {
	if err := ctxs.IsRoot(l.ctx); err != nil {
		return nil, err
	}
	in.Id = 0
	po := ToAppInfoPo(in)
	err := relationDB.NewAppInfoRepo(l.ctx).Insert(l.ctx, po)
	return &sys.WithID{Id: po.ID}, err
}
