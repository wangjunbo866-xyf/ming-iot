package devicemanagelogic

import (
	"context"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"
	"gitee.com/ming-iot/things/share/devices"

	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeviceProfileDeleteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeviceProfileDeleteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeviceProfileDeleteLogic {
	return &DeviceProfileDeleteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DeviceProfileDeleteLogic) DeviceProfileDelete(in *dm.DeviceProfileReadReq) (*dm.Empty, error) {
	l.ctx = ctxs.WithDefaultAllProject(l.ctx)
	err := relationDB.NewDeviceProfileRepo(l.ctx).DeleteByFilter(l.ctx, relationDB.DeviceProfileFilter{
		Code: in.Code,
		Device: devices.Core{
			ProductID:  in.Device.ProductID,
			DeviceName: in.Device.DeviceName,
		},
	})
	if err != nil {
		return nil, err
	}
	return &dm.Empty{}, nil
}
