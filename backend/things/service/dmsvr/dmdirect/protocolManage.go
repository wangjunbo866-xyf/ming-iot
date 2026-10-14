package dmdirect

import (
	client "gitee.com/ming-iot/things/service/dmsvr/client/protocolmanage"
	server "gitee.com/ming-iot/things/service/dmsvr/internal/server/protocolmanage"
)

var (
	protocolManageSvr client.ProtocolManage
)

func NewProtocolManage(runSvr bool) client.ProtocolManage {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	dmSvr := client.NewDirectProtocolManage(svcCtx, server.NewProtocolManageServer(svcCtx))
	return dmSvr
}
