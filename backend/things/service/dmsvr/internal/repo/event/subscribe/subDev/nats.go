package subDev

import (
	"context"
	"gitee.com/ming-iot/share/clients"
	"gitee.com/ming-iot/share/conf"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/share/domain/deviceMsg"
	"github.com/nats-io/nats.go"
	"github.com/zeromicro/go-zero/core/logx"
	"time"
)

type (
	NatsClient struct {
		client *clients.NatsClient
	}
)

const (
	ThingsDeliverGroup = "things_dm_group"
	natsJsConsumerName = "dmsvr"
)

func newNatsClient(conf conf.EventConf, nodeID int64) (*NatsClient, error) {
	nc, err := clients.NewNatsClient2(conf.Mode, natsJsConsumerName, conf.Nats, nodeID)
	if err != nil {
		return nil, err
	}
	return &NatsClient{client: nc}, nil
}

func (n *NatsClient) Subscribe(handle Handle) error {
	return nil

}

func (n *NatsClient) queueSubscribeDevPublish(topic string,
	handleFunc func(ctx context.Context, msg *deviceMsg.PublishMsg) error) error {
	_, err := n.client.QueueSubscribe(topic, ThingsDeliverGroup,
		func(ctx context.Context, ts time.Time, msg []byte, natsMsg *nats.Msg) error {
			ctx = ctxs.WithRoot(ctx)
			defer utils.Recover(ctx)
			_ = ts
			ele, err := deviceMsg.GetDevPublish(ctx, msg)
			if err != nil {
				logx.WithContext(ctx).Errorf("%s.GetDevPublish failure err:%v", utils.FuncName(), err)
				return err
			}
			return handleFunc(ctx, ele)
		})
	if err != nil {
		return err
	}
	return nil
}
