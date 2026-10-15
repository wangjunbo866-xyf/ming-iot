package departmentmanagelogic

import (
	"context"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeptUserMultiDeleteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeptUserMultiDeleteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeptUserMultiDeleteLogic {
	return &DeptUserMultiDeleteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DeptUserMultiDeleteLogic) DeptUserMultiDelete(in *sys.DeptUserMultiDeleteReq) (*sys.Empty, error) {
	// todo: add your logic here and delete this line

	return &sys.Empty{}, nil
}
