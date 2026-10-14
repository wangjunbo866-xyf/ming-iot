package module

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type MultiUpdateLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewMultiUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MultiUpdateLogic {
	return &MultiUpdateLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *MultiUpdateLogic) MultiUpdate(req *types.AppModuleMultiUpdateReq) error {
	_, err := l.svcCtx.AppRpc.AppModuleMultiUpdate(l.ctx, &sys.AppModuleMultiUpdateReq{ModuleCodes: req.ModuleCodes, Code: req.Code})
	return err
}
