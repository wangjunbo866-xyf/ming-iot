package opslogic

import (
	"context"
	"database/sql"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/saas/share/domain/ops"
	"gitee.com/ming-iot/saas/share/topics"
	"github.com/zeromicro/go-zero/core/logx"
	"time"
)

type OpsWorkOrderUpdateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewOpsWorkOrderUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *OpsWorkOrderUpdateLogic {
	return &OpsWorkOrderUpdateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *OpsWorkOrderUpdateLogic) OpsWorkOrderUpdate(in *sys.OpsWorkOrder) (*sys.Empty, error) {
	old, err := relationDB.NewOpsWorkOrderRepo(l.ctx).FindOne(l.ctx, in.Id)
	if err != nil {
		return nil, err
	}
	var isFinish bool
	if in.Status != 0 && in.Status > old.Status {
		switch in.Status {
		case ops.WorkOrderStatusHandling:
			if old.Status == ops.WorkOrderStatusWait {
				old.Status = in.Status
				old.HandleTime = sql.NullTime{Valid: true, Time: time.Now()}
			}
		case ops.WorkOrderStatusFinished:
			if old.Status == ops.WorkOrderStatusHandling {
				old.Status = in.Status
				old.FinishedTime = sql.NullTime{Valid: true, Time: time.Now()}
				isFinish = true
			}
		}
	}
	err = relationDB.NewOpsWorkOrderRepo(l.ctx).Update(l.ctx, old)
	if err == nil && isFinish {
		err = l.svcCtx.FastEvent.Publish(l.ctx, topics.CoreOpsWorkOrderFinish, old.ID)
		if err != nil {
			l.Error(err)
		}
	}
	return &sys.Empty{}, err
}
