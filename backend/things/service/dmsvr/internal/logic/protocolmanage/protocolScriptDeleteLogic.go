package protocolmanagelogic

import (
	"context"

	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"

	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProtocolScriptDeleteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewProtocolScriptDeleteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProtocolScriptDeleteLogic {
	return &ProtocolScriptDeleteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 协议删除
func (l *ProtocolScriptDeleteLogic) ProtocolScriptDelete(in *dm.WithID) (*dm.Empty, error) {
	if err := ctxs.IsAdmin(l.ctx); err != nil {
		return nil, err
	}
	old, err := relationDB.NewProtocolScriptRepo(l.ctx).FindOne(l.ctx, in.Id)
	if err != nil {
		return &dm.Empty{}, err
	}
	if !ctxs.CanHandTenant(l.ctx, old.TenantCode) {
		return nil, errors.Permissions
	}
	err = relationDB.NewProtocolScriptRepo(l.ctx).Delete(l.ctx, in.Id)
	return &dm.Empty{}, err
}
