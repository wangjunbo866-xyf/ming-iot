package protocolmanagelogic

import (
	"context"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/internal/logic"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"

	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProtocolScriptDeviceIndexLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewProtocolScriptDeviceIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProtocolScriptDeviceIndexLogic {
	return &ProtocolScriptDeviceIndexLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 协议列表
func (l *ProtocolScriptDeviceIndexLogic) ProtocolScriptDeviceIndex(in *dm.ProtocolScriptDeviceIndexReq) (*dm.ProtocolScriptDeviceIndexResp, error) {
	var (
		size int64
		err  error
		piDB = relationDB.NewProtocolScriptDeviceRepo(l.ctx)
	)

	filter := relationDB.ProtocolScriptDeviceFilter{
		TriggerSrc: in.TriggerSrc,
		ProductID:  in.ProductID,
		ScriptID:   in.ScriptID,
		DeviceName: in.DeviceName,
		Status:     in.Status,
		WithScript: in.WithScript,
	}
	size, err = piDB.CountByFilter(l.ctx, filter)
	if err != nil {
		return nil, err
	}

	di, err := piDB.FindByFilter(l.ctx, filter,
		logic.ToPageInfo(in.Page),
	)
	if err != nil {
		return nil, err
	}

	info := utils.CopySlice[dm.ProtocolScriptDevice](di)
	return &dm.ProtocolScriptDeviceIndexResp{List: info, Total: size}, nil
}
