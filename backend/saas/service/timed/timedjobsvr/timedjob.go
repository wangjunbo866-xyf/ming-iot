package main

import (
	"context"
	"gitee.com/ming-iot/saas/service/timed/timedjobsvr/timedjobdirect"
	"gitee.com/ming-iot/share/utils"
)

func main() {
	defer utils.Recover(context.Background())
	ctx := timedjobdirect.GetSvcCtx()
	timedjobdirect.Run(ctx)
}
