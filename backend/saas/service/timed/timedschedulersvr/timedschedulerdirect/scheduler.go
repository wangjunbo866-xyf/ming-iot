package timedschedulerdirect

import (
	client "gitee.com/ming-iot/saas/service/timed/timedschedulersvr/client/timedscheduler"
	server "gitee.com/ming-iot/saas/service/timed/timedschedulersvr/internal/server/timedscheduler"
)

var (
	schedulerSvr client.Timedscheduler
)

func NewScheduler(runSvr bool) client.Timedscheduler {
	svcCtx := GetSvcCtx()
	if runSvr {
		RunServer(svcCtx)
	}
	svr := client.NewDirectTimedscheduler(svcCtx, server.NewTimedschedulerServer(svcCtx))
	return svr
}
