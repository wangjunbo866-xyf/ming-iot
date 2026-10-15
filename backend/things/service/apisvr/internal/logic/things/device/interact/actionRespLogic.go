package interact

import (
	"context"

	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"github.com/zeromicro/go-zero/core/logx"
)

type ActionRespLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 设备回复行为调用结果
func NewActionRespLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ActionRespLogic {
	return &ActionRespLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctxs.WithDefaultRoot(ctx),
		svcCtx: svcCtx,
	}
}

func (l *ActionRespLogic) ActionResp(req *types.DeviceInteractActionRespReq) error {
	dmReq := &dm.ActionRespReq{
		ProductID:    req.ProductID,
		DeviceName:   req.DeviceName,
		MsgToken:     req.MsgToken,
		OutputParams: req.OutputParams,
		Msg:          req.Msg,
		Code:         req.Code,
	}
	_, err := l.svcCtx.DeviceInteract.ActionResp(l.ctx, dmReq)
	if err != nil {
		er := errors.Fmt(err)
		l.Errorf("%s.rpc.ActionResp req=%v err=%+v", utils.FuncName(), req, er)
		return er
	}
	return nil
}
