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

type DataOpenAccessDeleteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDataOpenAccessDeleteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DataOpenAccessDeleteLogic {
	return &DataOpenAccessDeleteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DataOpenAccessDeleteLogic) DataOpenAccessDelete(in *sys.WithID) (*sys.Empty, error) {
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
	err = relationDB.NewDataOpenAccessRepo(l.ctx).Delete(l.ctx, in.Id)
	return &sys.Empty{}, err
}
