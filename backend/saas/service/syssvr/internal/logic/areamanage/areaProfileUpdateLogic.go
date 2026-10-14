package areamanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/errors"

	"github.com/zeromicro/go-zero/core/logx"
)

type AreaProfileUpdateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewAreaProfileUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AreaProfileUpdateLogic {
	return &AreaProfileUpdateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *AreaProfileUpdateLogic) AreaProfileUpdate(in *sys.AreaProfile) (*sys.Empty, error) {
	old, err := relationDB.NewAreaProfileRepo(l.ctx).FindOneByFilter(l.ctx, relationDB.AreaProfileFilter{
		Code:   in.Code,
		AreaID: in.AreaID,
	})
	if err != nil {
		if !errors.Cmp(err, errors.NotFind) {
			return nil, err
		}
		old = &relationDB.SysAreaProfile{AreaID: dataType.AreaID(in.AreaID), Code: in.Code}
	}
	old.Params = in.Params
	err = relationDB.NewAreaProfileRepo(l.ctx).Update(l.ctx, old)
	return &sys.Empty{}, err
}
