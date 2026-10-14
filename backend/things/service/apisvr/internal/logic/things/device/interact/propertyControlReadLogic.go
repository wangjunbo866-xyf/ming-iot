package interact

import (
	"context"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PropertyControlReadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPropertyControlReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PropertyControlReadLogic {
	return &PropertyControlReadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctxs.WithDefaultRoot(ctx),
		svcCtx: svcCtx,
	}
}

func (l *PropertyControlReadLogic) PropertyControlRead(req *types.DeviceInteractRespReadReq) (resp *types.DeviceInteractSendPropertyResp, err error) {
	dmReq := &dm.RespReadReq{
		ProductID:  req.ProductID,
		DeviceName: req.DeviceName,
		MsgToken:   req.MsgToken,
	}
	dmResp, err := l.svcCtx.DeviceInteract.PropertyControlRead(l.ctx, dmReq)
	if err != nil {
		er := errors.Fmt(err)
		l.Errorf("%s.rpc.PropertyRead req=%v err=%+v", utils.FuncName(), req, er)
		return nil, er
	}
	return &types.DeviceInteractSendPropertyResp{
		Code:     dmResp.Code,
		Msg:      dmResp.Msg,
		MsgToken: dmResp.MsgToken,
	}, nil
}
