package dmdirect

import (
	client "gitee.com/ming-iot/things/service/dmsvr/client/deviceinteract"
	server "gitee.com/ming-iot/things/service/dmsvr/internal/server/deviceinteract"
)

var (
	deviceInteractSvr client.DeviceInteract
)

func NewDeviceInteract(runSvr bool) client.DeviceInteract {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	dmSvr := client.NewDirectDeviceInteract(svcCtx, server.NewDeviceInteractServer(svcCtx))
	return dmSvr
}
