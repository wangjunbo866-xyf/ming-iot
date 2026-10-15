package accessmanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/def"

	"github.com/zeromicro/go-zero/core/logx"
)

type AccessInfoCreateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewAccessInfoCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AccessInfoCreateLogic {
	return &AccessInfoCreateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *AccessInfoCreateLogic) AccessInfoCreate(in *sys.AccessInfo) (*sys.WithID, error) {
	po := ToAccessPo(in)
	po.ID = 0
	po.Apis = nil
	err := relationDB.NewAccessRepo(l.ctx).Insert(l.ctx, po)
	if err != nil {
		return nil, err
	}
	err = relationDB.NewTenantAccessRepo(l.ctx).Insert(l.ctx, &relationDB.SysTenantAccess{
		TenantCode: def.TenantCodeDefault,
		AccessCode: po.Code,
	})
	if err != nil {
		l.Error(err)
	}
	return &sys.WithID{Id: po.ID}, nil
}
