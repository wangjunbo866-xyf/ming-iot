package datamanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/def"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type DataOpenAccessReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDataOpenAccessReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DataOpenAccessReadLogic {
	return &DataOpenAccessReadLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DataOpenAccessReadLogic) DataOpenAccessRead(in *sys.WithID) (*sys.OpenAccess, error) {
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

	return utils.Copy[sys.OpenAccess](old), nil
}
