package tenantmanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type TenantAgreementReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewTenantAgreementReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *TenantAgreementReadLogic {
	return &TenantAgreementReadLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *TenantAgreementReadLogic) TenantAgreementRead(in *sys.WithIDCode) (*sys.TenantAgreement, error) {
	po, err := relationDB.NewTenantAgreementRepo(l.ctx).FindOneByFilter(l.ctx, relationDB.TenantAgreementFilter{
		ID:   in.Id,
		Code: in.Code,
	})
	return utils.Copy[sys.TenantAgreement](po), err
}
