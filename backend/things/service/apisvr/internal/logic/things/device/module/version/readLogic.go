package version

import (
	"context"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ReadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReadLogic {
	return &ReadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ReadLogic) Read(req *types.DeviceModuleVersionReadReq) (resp *types.DeviceModuleVersion, err error) {
	ret, err := l.svcCtx.DeviceM.DeviceModuleVersionRead(l.ctx, utils.Copy[dm.DeviceModuleVersionReadReq](req))
	return utils.Copy[types.DeviceModuleVersion](ret), err
}
