package log

import (
	"context"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/utils"

	"github.com/zeromicro/go-zero/core/logx"
)

type OperIndexLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewOperIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *OperIndexLogic {
	return &OperIndexLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *OperIndexLogic) OperIndex(req *types.SysLogOperIndexReq) (resp *types.SysLogOperIndexResp, err error) {
	l.Infof("%s req=%v", utils.FuncName(), req)
	info, err := l.svcCtx.LogRpc.OperLogIndex(l.ctx, utils.Copy[sys.OperLogIndexReq](req))
	return utils.Copy[types.SysLogOperIndexResp](info), err
}
