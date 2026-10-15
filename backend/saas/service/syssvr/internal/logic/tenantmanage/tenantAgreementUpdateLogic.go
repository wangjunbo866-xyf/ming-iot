package tenantmanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type TenantAgreementUpdateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewTenantAgreementUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *TenantAgreementUpdateLogic {
	return &TenantAgreementUpdateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *TenantAgreementUpdateLogic) TenantAgreementUpdate(in *sys.TenantAgreement) (*sys.Empty, error) {
	if err := ctxs.IsAdmin(l.ctx); err != nil {
		return nil, err
	}
	old, err := relationDB.NewTenantAgreementRepo(l.ctx).FindOne(l.ctx, in.Id)
	if err != nil {
		return nil, err
	}
	if in.Name != "" {
		old.Name = in.Name
	}
	if in.Title != "" {
		old.Title = in.Title
	}
	if in.Content != "" {
		old.Content = in.Content
	}
	err = relationDB.NewTenantAgreementRepo(l.ctx).Update(l.ctx, old)
	return &sys.Empty{}, err
}
