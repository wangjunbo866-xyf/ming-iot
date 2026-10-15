package pubInner

import (
	//"gitee.com/ming-iot/things/service/dgsvr/internal/domain"
	"context"
	"gitee.com/ming-iot/share/conf"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/things/share/devices"
)

type ConnType int8

const (
	Connect ConnType = iota
	DisConnect
)

type (
	PubInner interface {
		DevPubMsg(ctx context.Context, publishMsg *devices.DevPublish) error
		// PubConn 向内部发布连接及断连消息
		PubConn(ctx context.Context, conn ConnType, info *devices.DevConn) error
	}
)

func NewPubInner(c conf.EventConf, protocolCode string, nodeID int64) (PubInner, error) {
	switch c.Mode {
	case conf.EventModeNats, conf.EventModeNatsJs:
		return newNatsClient(c, protocolCode, nodeID)
	}
	return nil, errors.Parameter.AddMsgf("mode:%v not support", c.Mode)
}
