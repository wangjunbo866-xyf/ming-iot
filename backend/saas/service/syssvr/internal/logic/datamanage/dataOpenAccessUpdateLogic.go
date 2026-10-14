package datamanagelogic

import (
	"context"

	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/def"
	"gitee.com/ming-iot/share/errors"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type DataOpenAccessUpdateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDataOpenAccessUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DataOpenAccessUpdateLogic {
	return &DataOpenAccessUpdateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DataOpenAccessUpdateLogic) DataOpenAccessUpdate(in *sys.OpenAccess) (*sys.Empty, error) {
	uc := ctxs.GetUserCtxNoNil(l.ctx)
	old, err := relationDB.NewDataOpenAccessRepo(l.ctx).FindOne(l.ctx, in.Id)
	if err != nil {
		return nil, err
	}
	if string(old.TenantCode) != uc.TenantCode && !(uc.TenantCode == def.TenantCodeDefault && uc.IsAdmin) {
		return nil, errors.Permissions
	}
	if !uc.IsAdmin && old.UserID != uc.UserID {
		return nil, errors.Permissions
	}
	if in.Desc != "" {
		old.Desc = in.Desc
	}
	if in.IpRange != nil && !(len(in.IpRange) == 1 && in.IpRange[0] == "") { //兼容前端不规范传参
		old.IpRange = in.IpRange
	}
	err = relationDB.NewDataOpenAccessRepo(l.ctx).Update(l.ctx, old)
	return &sys.Empty{}, err
}
