package server

import (
	"context"
	"gitee.com/ming-iot/share/conf"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/things/share/domain/deviceMsg"
)

type (
	Server interface {
		Subscribe(handle Handle) error
	}
	Handle       func(ctx context.Context) ServerHandle
	ServerHandle interface {
		ActionCheck(req *deviceMsg.PublishMsg) error
	}
)

func NewServer(c conf.EventConf, nodeID int64) (Server, error) {
	switch c.Mode {
	case conf.EventModeNats, conf.EventModeNatsJs:
		return newNatsClient(c, nodeID)
	}
	return nil, errors.Parameter.AddMsgf("mode:%v not support", c.Mode)
}
