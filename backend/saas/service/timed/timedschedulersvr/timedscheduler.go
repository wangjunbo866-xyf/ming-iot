package main

import (
	"context"
	"gitee.com/ming-iot/saas/service/timed/timedschedulersvr/timedschedulerdirect"
	"gitee.com/ming-iot/share/utils"
)

func main() {
	defer utils.Recover(context.Background())
	ctx := timedschedulerdirect.GetSvcCtx()
	timedschedulerdirect.Run(ctx)
}
