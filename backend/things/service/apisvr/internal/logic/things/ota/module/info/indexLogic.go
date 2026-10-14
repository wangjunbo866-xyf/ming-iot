package info

import (
	"context"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type IndexLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *IndexLogic {
	return &IndexLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *IndexLogic) Index(req *types.OtaModuleInfoIndexReq) (resp *types.OtaModuleInfoIndexResp, err error) {
	ret, err := l.svcCtx.OtaM.OtaModuleInfoIndex(l.ctx, utils.Copy[dm.OtaModuleInfoIndexReq](req))
	if err != nil {
		return nil, err
	}
	return utils.Copy[types.OtaModuleInfoIndexResp](ret), nil
}
