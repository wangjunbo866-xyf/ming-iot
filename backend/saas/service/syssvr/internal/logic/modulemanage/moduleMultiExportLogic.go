package modulemanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type ModuleMultiExportLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewModuleMultiExportLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModuleMultiExportLogic {
	return &ModuleMultiExportLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ModuleMultiExportLogic) ModuleMultiExport(in *sys.ModuleMultiExportReq) (*sys.ModuleMultiExportResp, error) {
	pos, err := relationDB.NewModuleInfoRepo(l.ctx).FindByFilter(l.ctx, relationDB.ModuleInfoFilter{Codes: in.ModuleCodes, WithMenus: true}, nil)
	if err != nil {
		return nil, err
	}
	return &sys.ModuleMultiExportResp{Modules: utils.MarshalNoErr(pos)}, nil
}
