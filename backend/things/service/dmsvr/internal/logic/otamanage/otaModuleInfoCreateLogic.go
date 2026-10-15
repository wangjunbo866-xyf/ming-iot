package otamanagelogic

import (
	"context"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"
	"gitee.com/ming-iot/things/share/domain/deviceMsg/msgOta"

	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"github.com/zeromicro/go-zero/core/logx"
)

type OtaModuleInfoCreateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewOtaModuleInfoCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *OtaModuleInfoCreateLogic {
	return &OtaModuleInfoCreateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *OtaModuleInfoCreateLogic) OtaModuleInfoCreate(in *dm.OtaModuleInfo) (*dm.WithID, error) {
	if err := ctxs.IsRoot(l.ctx); err != nil {
		return nil, err
	}
	l.ctx = ctxs.WithRoot(l.ctx)
	if in.Code == msgOta.FirmwareModuleCodeDefault || in.Code == "" {
		return nil, errors.Parameter.AddMsg("编码不能为空及不能为default")
	}
	_, err := l.svcCtx.ProductCache.GetData(l.ctx, in.ProductID)
	if err != nil {
		if errors.Cmp(err, errors.NotFind) {
			return nil, errors.Parameter.AddMsg("product not find")
		}
		return nil, err
	}
	po := utils.Copy[relationDB.DmOtaModuleInfo](in)
	in.Id = 0
	err = relationDB.NewOtaModuleInfoRepo(l.ctx).Insert(l.ctx, po)
	if err != nil {
		return nil, err
	}
	return &dm.WithID{Id: po.ID}, nil
}
