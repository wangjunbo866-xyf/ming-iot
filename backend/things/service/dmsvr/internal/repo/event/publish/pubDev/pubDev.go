package pubDev

import (
	"context"
	"time"

	ws "gitee.com/ming-iot/saas/share/websocket"
	"gitee.com/ming-iot/share/eventBus"
	"gitee.com/ming-iot/things/service/dmsvr/internal/domain/protocol"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
	"gitee.com/ming-iot/things/share/domain/deviceMsg"
)

type (
	CompareMsg func(payload []byte) bool

	PubDev interface {
		PublishToDev(ctx context.Context, di *dm.DeviceInfo, msg *deviceMsg.PublishMsg) error
		ReqToDeviceSync(ctx context.Context, di *dm.DeviceInfo, reqMsg *deviceMsg.PublishMsg, timeout time.Duration, compareMsg CompareMsg) ([]byte, error)
	}
)

var s *protocol.ScriptTrans
var us *ws.UserSubscribe

func NewPubDev(fast *eventBus.FastEvent, S *protocol.ScriptTrans, US *ws.UserSubscribe) (PubDev, error) {
	s = S
	us = US
	return newPubDevClient(fast)
}
