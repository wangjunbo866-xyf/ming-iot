package msg

import (
	"context"

	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PropertyAggIndexLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 聚合属性历史记录
func NewPropertyAggIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PropertyAggIndexLogic {
	return &PropertyAggIndexLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PropertyAggIndexLogic) PropertyAggIndex(req *types.DeviceMsgPropertyLogAggIndexReq) (resp *types.DeviceMsgPropertyLogAggIndexResp, err error) {
	ret, err := l.svcCtx.DeviceMsg.PropertyLogAggIndex(l.ctx, utils.Copy[dm.PropertyAggIndexReq](req))
	return utils.Copy[types.DeviceMsgPropertyLogAggIndexResp](ret), err
}
