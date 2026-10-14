package commonlogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type SlotInfoCreateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSlotInfoCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SlotInfoCreateLogic {
	return &SlotInfoCreateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *SlotInfoCreateLogic) SlotInfoCreate(in *sys.SlotInfo) (*sys.WithID, error) {
	if err := ctxs.IsRoot(l.ctx); err != nil {
		return nil, err
	}
	po := utils.Copy[relationDB.SysSlotInfo](in)
	po.ID = 0
	err := relationDB.NewSlotInfoRepo(l.ctx).Insert(l.ctx, po)
	return &sys.WithID{Id: po.ID}, err
}
