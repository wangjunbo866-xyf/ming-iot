package notifymanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type NotifyTemplateCreateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewNotifyTemplateCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *NotifyTemplateCreateLogic {
	return &NotifyTemplateCreateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *NotifyTemplateCreateLogic) NotifyTemplateCreate(in *sys.NotifyTemplate) (*sys.WithID, error) {
	po := utils.Copy[relationDB.SysNotifyTemplate](in)
	po.ID = 0
	err := relationDB.NewNotifyTemplateRepo(l.ctx).Insert(l.ctx, po)
	return &sys.WithID{Id: po.ID}, err
}
