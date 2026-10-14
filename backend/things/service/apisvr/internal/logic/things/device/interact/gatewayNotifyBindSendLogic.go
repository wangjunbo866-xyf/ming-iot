package interact

import (
	"context"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GatewayNotifyBindSendLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGatewayNotifyBindSendLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GatewayNotifyBindSendLogic {
	return &GatewayNotifyBindSendLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctxs.WithDefaultRoot(ctx),
		svcCtx: svcCtx,
	}
}

func (l *GatewayNotifyBindSendLogic) GatewayNotifyBindSend(req *types.GatewayNotifyBindSendReq) error {
	_, err := l.svcCtx.DeviceInteract.GatewayNotifyBindSend(l.ctx, utils.Copy[dm.GatewayNotifyBindSendReq](req))

	return err
}
