package info

import (
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
)

func ToRpcDeviceInfo(req *types.DeviceInfo) *dm.DeviceInfo {
	return utils.Copy[dm.DeviceInfo](req)
}
