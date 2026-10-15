package svc

import "gitee.com/ming-iot/things/share/rpcs/protocolSync/internal/config"

type ServiceContext struct {
	Config config.Config
}

func NewServiceContext(c config.Config) *ServiceContext {
	return &ServiceContext{
		Config: c,
	}
}
