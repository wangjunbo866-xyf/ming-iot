package deviceMsgEvent

import (
	"context"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/share/domain/deviceMsg"
	"github.com/zeromicro/go-zero/core/logx"
)

type ShadowLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewShadowLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ShadowLogic {
	return &ShadowLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ShadowLogic) Handle(msg *deviceMsg.PublishMsg) (respMsg *deviceMsg.PublishMsg, err error) {
	l.Infof("%s req=%+v", utils.FuncName(), msg)
	// todo
	return
}
