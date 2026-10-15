package projectmanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProjectInfoReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	PiDB *relationDB.ProjectInfoRepo
}

func NewProjectInfoReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProjectInfoReadLogic {
	return &ProjectInfoReadLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		PiDB:   relationDB.NewProjectInfoRepo(ctx),
	}
}

// 获取项目信息详情
func (l *ProjectInfoReadLogic) ProjectInfoRead(in *sys.ProjectWithID) (*sys.ProjectInfo, error) {
	ctxs.GetUserCtx(l.ctx).AllProject = true
	defer func() {
		ctxs.GetUserCtx(l.ctx).AllProject = false
	}()
	l.ctx = ctxs.WithDefaultRoot(l.ctx)

	po, err := l.PiDB.FindOneByFilter(l.ctx, relationDB.ProjectInfoFilter{ProjectID: in.ProjectID, WithTopAreas: in.WithTopAreas})
	if err != nil {
		return nil, err
	}
	return ProjectInfoToPb(l.ctx, l.svcCtx, po), nil
}
