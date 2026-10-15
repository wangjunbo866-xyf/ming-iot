package notifymanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type NotifyConfigTemplateIndexLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewNotifyConfigTemplateIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *NotifyConfigTemplateIndexLogic {
	return &NotifyConfigTemplateIndexLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *NotifyConfigTemplateIndexLogic) NotifyConfigTemplateIndex(in *sys.NotifyConfigTemplateIndexReq) (*sys.NotifyConfigTemplateIndexResp, error) {
	db := relationDB.NewNotifyConfigTemplateRepo(l.ctx)
	pos, err := db.FindByFilter(l.ctx, relationDB.NotifyConfigTemplateFilter{
		NotifyCode: in.NotifyCode,
		Type:       in.Type,
	}, nil)
	return &sys.NotifyConfigTemplateIndexResp{List: utils.CopySlice[sys.NotifyConfigTemplate](pos)}, err
}
