package info

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type MultiDeleteLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 批量删除租户操作权限
func NewMultiDeleteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MultiDeleteLogic {
	return &MultiDeleteLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *MultiDeleteLogic) MultiDelete(req *types.TenantAccessInfo) error {
	_, err := l.svcCtx.TenantRpc.TenantAccessMultiDelete(l.ctx, &sys.TenantAccessMultiSaveReq{
		Code:        req.Code,
		AccessCodes: req.AccessCodes,
	})

	return err
}
