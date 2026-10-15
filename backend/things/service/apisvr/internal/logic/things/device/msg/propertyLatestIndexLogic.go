package msg

import (
	"context"

	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/apisvr/internal/logic"
	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"github.com/zeromicro/go-zero/core/logx"
)

type PropertyLatestIndexLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPropertyLatestIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PropertyLatestIndexLogic {
	return &PropertyLatestIndexLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctxs.WithDefaultRoot(ctx),
		svcCtx: svcCtx,
	}
}

func (l *PropertyLatestIndexLogic) PropertyLatestIndex(req *types.DeviceMsgPropertyLatestIndexReq) (resp *types.DeviceMsgPropertyIndexResp, err error) {
	dmResp, err := l.svcCtx.DeviceMsg.PropertyLatestIndex(l.ctx, utils.Copy[dm.PropertyLatestIndexReq](req))
	if err != nil {
		er := errors.Fmt(err)
		l.Errorf("%s.rpc.GetDeviceData req=%v err=%+v", utils.FuncName(), req, er)
		return nil, er
	}
	info := make([]*types.DeviceMsgPropertyLogInfo, 0, len(dmResp.List))
	for _, v := range dmResp.List {
		info = append(info, utils.Copy[types.DeviceMsgPropertyLogInfo](v))
	}
	return &types.DeviceMsgPropertyIndexResp{
		PageResp: logic.ToPageResp(nil, dmResp.Total),
		List:     info,
	}, nil
}
