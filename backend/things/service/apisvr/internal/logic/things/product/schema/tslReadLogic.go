package schema

import (
	"context"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type TslReadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewTslReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *TslReadLogic {
	return &TslReadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *TslReadLogic) TslRead(req *types.ProductSchemaTslReadReq) (resp *types.ProductSchemaTslReadResp, err error) {
	dmReq := &dm.ProductSchemaTslReadReq{
		ProductID: req.ProductID, //产品id
	}
	dmResp, err := l.svcCtx.ProductM.ProductSchemaTslRead(l.ctx, dmReq)
	if err != nil {
		er := errors.Fmt(err)
		l.Errorf("%s.rpc.ProductSchemaTslRead req=%v err=%+v", utils.FuncName(), req, er)
		return nil, er
	}
	return &types.ProductSchemaTslReadResp{Tsl: dmResp.Tsl}, nil
}
