package svc

import (
	"gitee.com/ming-iot/saas/service/timed/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/service/timed/timedjobsvr/client/timedmanage"
	"gitee.com/ming-iot/saas/service/timed/timedjobsvr/timedjobdirect"
	"gitee.com/ming-iot/saas/service/timed/timedschedulersvr/internal/config"
	"gitee.com/ming-iot/saas/share/localdev"
	"gitee.com/ming-iot/share/clients"
	"gitee.com/ming-iot/share/conf"
	"gitee.com/ming-iot/share/stores"
	"gitee.com/ming-iot/share/utils"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"os"
)

type ServiceContext struct {
	Config       config.Config
	Scheduler    *clients.TimedScheduler
	Store        kv.Store
	SchedulerRun bool //只启动单例
	TimedM       timedmanage.TimedManage
	Queue        *clients.NatsClient
	NodeID       int64
}

func NewServiceContext(c config.Config) *ServiceContext {
	var (
		TimedM timedmanage.TimedManage
	)

	stores.InitConn(c.Database)
	if localdev.SkipAutoMigrate() {
		logx.Infof("本地开发模式: 跳过 timedschedulersvr 数据库自动迁移")
	} else {
		err := relationDB.Migrate(c.Database)
		if err != nil {
			logx.Error("初始化数据库错误 err", err)
			os.Exit(-1)
		}
	}
	Scheduler := clients.NewTimedScheduler(c.CacheRedis)
	if c.TimedJobRpc.Enable {
		if c.TimedJobRpc.Mode == conf.ClientModeGrpc {
			TimedM = timedmanage.NewTimedManage(c.TimedJobRpc.MustNewClient())
		} else {
			TimedM = timedjobdirect.NewTimedJob(c.TimedJobRpc.RunProxy)
		}
	}
	nodeID := utils.GetNodeID(c.CacheRedis, c.Name)
	queue, err := clients.NewNatsClient2(c.Event.Mode, c.Name, c.Event.Nats, nodeID)
	logx.Must(err)
	return &ServiceContext{
		Scheduler: Scheduler,
		Config:    c,
		Store:     kv.NewStore(c.CacheRedis),
		TimedM:    TimedM,
		Queue:     queue,
		NodeID:    nodeID,
	}
}
