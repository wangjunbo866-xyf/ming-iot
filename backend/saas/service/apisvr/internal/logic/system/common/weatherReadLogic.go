package common

import (
	"context"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/utils"
	"github.com/zeromicro/go-zero/core/logx"
)

type WeatherReadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewWeatherReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WeatherReadLogic {
	return &WeatherReadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *WeatherReadLogic) WeatherRead(req *types.WeatherReadReq) (resp *types.WeatherReadResp, err error) {
	ret, err := l.svcCtx.Common.WeatherRead(l.ctx, utils.Copy[sys.WeatherReadReq](req))
	return utils.Copy[types.WeatherReadResp](ret), err
}
