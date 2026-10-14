// 设备管理模块-dmsvr
package main

import (
	"context"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/dmdirect"
	_ "net/http/pprof"
)

func main() {
	defer utils.Recover(context.Background())
	svcCtx := dmdirect.GetSvcCtx()
	dmdirect.Run(svcCtx)
}
