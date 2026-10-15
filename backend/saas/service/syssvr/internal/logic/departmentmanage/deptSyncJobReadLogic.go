package departmentmanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeptSyncJobReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeptSyncJobReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeptSyncJobReadLogic {
	return &DeptSyncJobReadLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DeptSyncJobReadLogic) DeptSyncJobRead(in *sys.DeptSyncJobReadReq) (*sys.DeptSyncJob, error) {
	if err := ctxs.IsAdmin(l.ctx); err != nil {
		return nil, err
	}
	po, err := relationDB.NewDeptSyncJobRepo(l.ctx).FindOne(l.ctx, in.Id)
	return utils.Copy[sys.DeptSyncJob](po), err
}
