package deviceinteractlogic

import (
	"context"
	"time"

	"gitee.com/ming-iot/share/def"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
	"gitee.com/ming-iot/things/share/devices"
)

const expiredDeviceControlMessage = "设备已到期，请续费后再控制"

// CheckControlAllowed 校验设备是否仍在有效期内，避免到期设备继续接收用户控制。
func CheckControlAllowed(ctx context.Context, svcCtx *svc.ServiceContext, core devices.Core) error {
	dev, err := svcCtx.DeviceCache.GetData(ctx, core)
	if err != nil {
		return err
	}
	return checkDeviceControlAllowed(dev, time.Now())
}

func checkDeviceControlAllowed(dev *dm.DeviceInfo, now time.Time) error {
	if dev != nil && dev.UserID > def.RootNode && dev.ExpTime != nil && dev.ExpTime.Value <= now.Unix() {
		return errors.NotEnable.WithMsg(expiredDeviceControlMessage)
	}
	return nil
}
