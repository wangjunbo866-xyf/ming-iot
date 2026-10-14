package projectmanagelogic

import (
	"context"

	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/stores"
	"gitee.com/ming-iot/share/utils"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProjectCrudIndexLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewProjectCrudIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProjectCrudIndexLogic {
	return &ProjectCrudIndexLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 获取项目crud列表
func (l *ProjectCrudIndexLogic) ProjectCrudIndex(in *sys.ProjectCrudIndexReq) (*sys.ProjectCrudIndexResp, error) {
	if in.Purpose == "" {
		return &sys.ProjectCrudIndexResp{}, errors.Parameter.AddMsg("purpose must not empty")
	}
	f := relationDB.ProjectCurlFilter{
		Purpose: in.Purpose,
		Params:  utils.CopyMap[stores.Compare](in.Params),
	}
	total, err := relationDB.NewProjectCurlRepo(l.ctx).CountByFilter(l.ctx, f)
	if err != nil {
		return nil, err
	}
	pos, err := relationDB.NewProjectCurlRepo(l.ctx).FindByFilter(l.ctx, f, utils.Copy[stores.PageInfo](in.Page).WithDefaultSort())
	if err != nil {
		return nil, err
	}
	return &sys.ProjectCrudIndexResp{List: ProjectCrudsToPb(l.ctx, l.svcCtx, pos), Total: total}, nil
}
