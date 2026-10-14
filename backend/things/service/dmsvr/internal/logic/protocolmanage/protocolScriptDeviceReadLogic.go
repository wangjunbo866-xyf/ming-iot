package protocolmanagelogic

import (
	"context"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"

	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProtocolScriptDeviceReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewProtocolScriptDeviceReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProtocolScriptDeviceReadLogic {
	return &ProtocolScriptDeviceReadLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 协议详情
func (l *ProtocolScriptDeviceReadLogic) ProtocolScriptDeviceRead(in *dm.WithID) (*dm.ProtocolScriptDevice, error) {
	po, err := relationDB.NewProtocolScriptDeviceRepo(l.ctx).FindOne(l.ctx, in.Id)
	return utils.Copy[dm.ProtocolScriptDevice](po), err
}
