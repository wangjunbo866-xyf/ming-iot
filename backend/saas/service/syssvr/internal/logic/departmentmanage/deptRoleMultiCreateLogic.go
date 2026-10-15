package departmentmanagelogic

import (
	"context"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeptRoleMultiCreateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeptRoleMultiCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeptRoleMultiCreateLogic {
	return &DeptRoleMultiCreateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DeptRoleMultiCreateLogic) DeptRoleMultiCreate(in *sys.DeptRoleMultiSaveReq) (*sys.Empty, error) {
	// todo: add your logic here and delete this line

	return &sys.Empty{}, nil
}
