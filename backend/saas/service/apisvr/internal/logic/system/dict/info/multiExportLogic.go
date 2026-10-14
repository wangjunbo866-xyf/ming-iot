package info

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type MultiExportLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 删除字典信息
func NewMultiExportLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MultiExportLogic {
	return &MultiExportLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *MultiExportLogic) MultiExport(req *types.DictInfoMultExportReq) (resp *types.DictInfoMultExportResp, err error) {
	ret, err := l.svcCtx.DictM.DictMultiExport(l.ctx, utils.Copy[sys.DictMultiExportReq](req))

	return utils.Copy[types.DictInfoMultExportResp](ret), err
}
