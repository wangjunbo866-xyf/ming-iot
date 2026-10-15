package rolemanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type RoleInfoCreateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	RiDB *relationDB.RoleInfoRepo
}

func NewRoleInfoCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RoleInfoCreateLogic {
	return &RoleInfoCreateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		RiDB:   relationDB.NewRoleInfoRepo(ctx),
	}
}

func (l *RoleInfoCreateLogic) RoleInfoCreate(in *sys.RoleInfo) (*sys.WithID, error) {
	if err := ctxs.IsAdmin(l.ctx); err != nil {
		return nil, err
	}
	po := relationDB.SysRoleInfo{
		Name:   in.Name,
		Desc:   in.Desc.GetValue(),
		Status: in.Status,
		Code:   in.Code,
	}
	if in.Code == "" {
		in.Code = utils.Random(10, 1)
	}
	err := l.RiDB.Insert(l.ctx, &po)
	if err != nil {
		return nil, err
	}
	return &sys.WithID{Id: po.ID}, nil
}
