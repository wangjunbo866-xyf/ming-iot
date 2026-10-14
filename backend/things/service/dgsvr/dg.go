package main

import (
	"context"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dgsvr/dgdirect"
)

func main() {
	defer utils.Recover(context.Background())
	svcCtx := dgdirect.GetSvcCtx()
	dgdirect.Run(svcCtx)
}
