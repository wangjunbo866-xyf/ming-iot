package manage

import (
	"context"
	"gitee.com/ming-iot/saas/service/datasvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/datasvr/internal/svc"
	"gitee.com/ming-iot/saas/service/datasvr/internal/types"

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

func (l *UpdateLogic) Update(req *types.DataStatisticsManage) error {
	if err := ctxs.IsRoot(l.ctx); err != nil {
		return err
	}
	old, err := relationDB.NewStatisticsInfoRepo(l.ctx).FindOne(l.ctx, req.ID)
	if err != nil {
		return err
	}
	newPo := utils.Copy[relationDB.DataStatisticsInfo](req)
	newPo.NoDelTime = old.NoDelTime
	newPo.DeletedTime = old.DeletedTime
	err = relationDB.NewStatisticsInfoRepo(l.ctx).Update(l.ctx, newPo)
	return err
}
