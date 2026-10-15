package datamanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/cache"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/def"
	"gitee.com/ming-iot/share/errors"

	"github.com/zeromicro/go-zero/core/logx"
)

type DataProjectCreateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDataProjectCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DataProjectCreateLogic {
	return &DataProjectCreateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DataProjectCreateLogic) DataProjectCreate(in *sys.DataProjectSaveReq) (*sys.Empty, error) {
	if in.TargetID == 0 {
		return nil, errors.Parameter.AddDetail(in.TargetID).WithMsg("TargetID参数必填")
	}
	uc := ctxs.GetUserCtx(l.ctx)
	if in.ProjectID != 0 {
		uc.ProjectID = in.ProjectID
	} else {
		in.ProjectID = uc.ProjectID
	}
	project, err := relationDB.NewProjectInfoRepo(l.ctx).FindOne(l.ctx, in.ProjectID)
	if err != nil {
		return nil, err
	}
	if !(uc.IsAdmin || uc.UserID == project.AdminUserID && in.TargetType != def.TargetRole) {
		return nil, errors.Permissions.WithMsg("只有管理员才有权限授权")
	}
	err = relationDB.NewDataProjectRepo(l.ctx).Insert(l.ctx, &relationDB.SysDataProject{
		ProjectID:  in.ProjectID,
		TargetType: in.TargetType,
		TargetID:   in.TargetID,
		AuthType:   in.AuthType,
	})
	if err != nil && errors.Cmp(err, errors.Duplicate) {
		return nil, err
	}
	if in.TargetType == def.TargetUser {
		cache.ClearProjectAuth(in.TargetID)
		ProjectUserCount(l.ctx, in.ProjectID)
	}
	return &sys.Empty{}, nil
}
