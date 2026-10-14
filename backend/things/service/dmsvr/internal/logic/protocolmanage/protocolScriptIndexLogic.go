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

type ProtocolScriptIndexLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewProtocolScriptIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProtocolScriptIndexLogic {
	return &ProtocolScriptIndexLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 协议列表
func (l *ProtocolScriptIndexLogic) ProtocolScriptIndex(in *dm.ProtocolScriptIndexReq) (*dm.ProtocolScriptIndexResp, error) {
	var (
		size int64
		err  error
		piDB = relationDB.NewProtocolScriptRepo(l.ctx)
	)

	filter := relationDB.ProtocolScriptFilter{
		Name:          in.Name,
		Status:        in.Status,
		TriggerDir:    in.TriggerDir,
		TriggerTimer:  in.TriggerTimer,
		TriggerHandle: in.TriggerHandle,
		TriggerType:   in.TriggerType,
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

	info := utils.CopySlice[dm.ProtocolScript](di)
	return &dm.ProtocolScriptIndexResp{List: info, Total: size}, nil
}
