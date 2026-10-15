package project

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProjectMultiCreateLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 批量更新授权项目权限
func NewProjectMultiCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProjectMultiCreateLogic {
	return &ProjectMultiCreateLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ProjectMultiCreateLogic) ProjectMultiCreate(req *types.DataProjectMultiSaveReq) error {
	_, err := l.svcCtx.DataM.DataProjectMultiCreate(l.ctx, utils.Copy[sys.DataProjectMultiSaveReq](req))
	return err
}
