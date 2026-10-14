package protocolmanagelogic

import (
	"context"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"

	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProtocolScriptMultiExportLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewProtocolScriptMultiExportLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProtocolScriptMultiExportLogic {
	return &ProtocolScriptMultiExportLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ProtocolScriptMultiExportLogic) ProtocolScriptMultiExport(in *dm.ProtocolScriptExportReq) (*dm.ProtocolScriptExportResp, error) {
	scripts, err := relationDB.NewProtocolScriptRepo(l.ctx).FindByFilter(l.ctx, relationDB.ProtocolScriptFilter{IDs: in.Ids, WithDevices: true}, nil)
	if err != nil {
		return nil, err
	}

	return &dm.ProtocolScriptExportResp{Scripts: utils.MarshalNoErr(scripts)}, nil
}
