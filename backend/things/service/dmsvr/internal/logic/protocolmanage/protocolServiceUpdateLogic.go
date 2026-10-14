package protocolmanagelogic

import (
	"context"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"

	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProtocolServiceUpdateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewProtocolServiceUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProtocolServiceUpdateLogic {
	return &ProtocolServiceUpdateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 更新服务状态,只给服务调用
func (l *ProtocolServiceUpdateLogic) ProtocolServiceUpdate(in *dm.ProtocolService) (*dm.Empty, error) {
	if err := ctxs.IsRoot(l.ctx); err != nil {
		return nil, err
	}
	old, err := relationDB.NewProtocolServiceRepo(l.ctx).FindOneByFilter(l.ctx, relationDB.ProtocolServiceFilter{
		Code: in.Code,
		IP:   in.Ip,
		Port: in.Port,
	})
	if err != nil {
		if errors.Cmp(err, errors.NotFind) {
			return &dm.Empty{}, relationDB.NewProtocolServiceRepo(l.ctx).Insert(l.ctx, utils.Copy[relationDB.DmProtocolService](in))
		}
		return nil, err
	}
	old.Status = in.Status
	err = relationDB.NewProtocolServiceRepo(l.ctx).Update(l.ctx, old)
	return &dm.Empty{}, err
}
