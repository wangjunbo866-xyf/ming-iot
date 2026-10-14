package msg

import (
	"context"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/apisvr/internal/logic"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type StatusLogIndexLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewStatusLogIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *StatusLogIndexLogic {
	return &StatusLogIndexLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctxs.WithDefaultRoot(ctx),
		svcCtx: svcCtx,
	}
}

func (l *StatusLogIndexLogic) StatusLogIndex(req *types.DeviceMsgStatusLogIndexReq) (resp *types.DeviceMsgStatusLogIndexResp, err error) {
	dmResp, err := l.svcCtx.DeviceMsg.StatusLogIndex(l.ctx, &dm.StatusLogIndexReq{
		DeviceName: req.DeviceName,
		ProductID:  req.ProductID,
		TimeStart:  req.TimeStart,
		TimeEnd:    req.TimeEnd,
		Page:       logic.ToDmPageRpc(req.Page),
		Status:     req.Status,
	})
	if err != nil {
		er := errors.Fmt(err)
		l.Errorf("%s.rpc.HubLogIndex req=%v err=%+v", utils.FuncName(), req, er)
		return nil, er
	}
	info := make([]*types.DeviceMsgStatusLogInfo, 0, len(dmResp.List))
	for _, v := range dmResp.List {
		info = append(info, &types.DeviceMsgStatusLogInfo{
			Timestamp:  v.Timestamp,
			Status:     v.Status,
			ProductID:  v.ProductID,
			DeviceName: v.DeviceName,
		})
	}
	return &types.DeviceMsgStatusLogIndexResp{List: info, PageResp: logic.ToPageResp(req.Page, dmResp.Total)}, err
}
