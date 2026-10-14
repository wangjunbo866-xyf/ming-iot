package remoteconfiglogic

import (
	"context"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"

	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"github.com/zeromicro/go-zero/core/logx"
)

type RemoteConfigCreateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	PrcDB *relationDB.ProductRemoteConfigRepo
}

func NewRemoteConfigCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoteConfigCreateLogic {
	return &RemoteConfigCreateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		PrcDB:  relationDB.NewProductRemoteConfigRepo(ctx),
	}
}

func (l *RemoteConfigCreateLogic) RemoteConfigCreate(in *dm.RemoteConfigCreateReq) (*dm.Empty, error) {
	err := l.PrcDB.Insert(l.ctx, &relationDB.DmProductRemoteConfig{
		ProductID: in.ProductID,
		Content:   in.Content,
	})
	if err != nil {
		return nil, errors.Database.AddDetail(err)
	}
	return &dm.Empty{}, nil
}
