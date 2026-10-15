package subInner

import (
	"context"
	"gitee.com/ming-iot/share/conf"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/things/share/devices"
)

type (
	SubInner interface {
		SubToDevMsg(handle Handle) error
	}
	Handle         func(ctx context.Context) InnerSubHandle
	InnerSubHandle interface {
		PublishToDev(info *devices.InnerPublish) error
	}
)

func NewSubInner(c conf.EventConf, nodeID int64) (SubInner, error) {
	switch c.Mode {
	case conf.EventModeNats, conf.EventModeNatsJs:
		return newNatsClient(c, nodeID)
	}
	return nil, errors.Parameter.AddMsgf("mode:%v not support", c.Mode)
}
