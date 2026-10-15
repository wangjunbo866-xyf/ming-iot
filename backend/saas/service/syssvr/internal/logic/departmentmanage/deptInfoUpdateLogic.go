package departmentmanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeptInfoUpdateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeptInfoUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeptInfoUpdateLogic {
	return &DeptInfoUpdateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DeptInfoUpdateLogic) DeptInfoUpdate(in *sys.DeptInfo) (*sys.Empty, error) {
	if err := ctxs.IsAdmin(l.ctx); err != nil {
		return nil, err
	}
	old, err := relationDB.NewDeptInfoRepo(l.ctx).FindOne(l.ctx, in.Id)
	if err != nil {
		return nil, err
	}
	old.Name = in.Name
	old.Status = in.Status
	old.Sort = in.Sort
	old.Desc = in.Desc.GetValue()
	err = relationDB.NewDeptInfoRepo(l.ctx).Update(l.ctx, old)
	return &sys.Empty{}, err
}
