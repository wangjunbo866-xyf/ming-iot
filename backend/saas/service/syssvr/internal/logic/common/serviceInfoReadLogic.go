package commonlogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceInfoReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewServiceInfoReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ServiceInfoReadLogic {
	return &ServiceInfoReadLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ServiceInfoReadLogic) ServiceInfoRead(in *sys.WithCode) (*sys.ServiceInfo, error) {
	if err := ctxs.IsRoot(l.ctx); err != nil {
		return nil, err
	}
	ret, err := relationDB.NewServiceInfoRepo(l.ctx).FindOneByFilter(l.ctx, relationDB.ServiceInfoFilter{
		Code: in.Code,
	})
	if errors.Cmp(err, errors.NotFind) {
		return &sys.ServiceInfo{
			Code: in.Code,
		}, nil
	}
	return utils.Copy[sys.ServiceInfo](ret), err
}
