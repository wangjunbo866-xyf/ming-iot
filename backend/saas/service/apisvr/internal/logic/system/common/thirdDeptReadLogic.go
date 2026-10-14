package common

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ThirdDeptReadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取第三方的部门信息,Children只能获取一层,需要递归获取
func NewThirdDeptReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ThirdDeptReadLogic {
	return &ThirdDeptReadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ThirdDeptReadLogic) ThirdDeptRead(req *types.ThirdDeptInfoReadReq) (resp *types.DeptInfo, err error) {
	ret, err := l.svcCtx.Common.ThirdDeptRead(l.ctx, utils.Copy[sys.ThirdDeptInfoReadReq](req))
	return utils.Copy[types.DeptInfo](ret), err
}
