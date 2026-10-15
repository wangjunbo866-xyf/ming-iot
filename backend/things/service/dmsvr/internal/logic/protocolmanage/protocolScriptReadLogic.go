package protocolmanagelogic

import (
	"context"

	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"

	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProtocolScriptReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewProtocolScriptReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProtocolScriptReadLogic {
	return &ProtocolScriptReadLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 协议详情
func (l *ProtocolScriptReadLogic) ProtocolScriptRead(in *dm.WithID) (*dm.ProtocolScript, error) {
	po, err := relationDB.NewProtocolScriptRepo(l.ctx).FindOne(l.ctx, in.Id)
	return utils.Copy[dm.ProtocolScript](po), err
}
