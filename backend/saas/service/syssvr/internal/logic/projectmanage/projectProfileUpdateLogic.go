package projectmanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProjectProfileUpdateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewProjectProfileUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProjectProfileUpdateLogic {
	return &ProjectProfileUpdateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ProjectProfileUpdateLogic) ProjectProfileUpdate(in *sys.ProjectProfile) (*sys.Empty, error) {
	projectID := ctxs.GetUserCtxNoNil(l.ctx).ProjectID
	old, err := relationDB.NewProjectProfileRepo(l.ctx).FindOneByFilter(l.ctx, relationDB.ProjectProfileFilter{
		Code:      in.Code,
		ProjectID: projectID,
	})
	if err != nil {
		if !errors.Cmp(err, errors.NotFind) {
			return nil, err
		}
		old = &relationDB.SysProjectProfile{ProjectID: dataType.ProjectID(projectID), Code: in.Code}
	}
	old.Params = in.Params
	err = relationDB.NewProjectProfileRepo(l.ctx).Update(l.ctx, old)
	return &sys.Empty{}, err
}
