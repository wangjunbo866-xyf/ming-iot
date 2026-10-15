package info

import (
	"context"
	"gitee.com/ming-iot/saas/service/apisvr/internal/logic"
	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system/role"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type IndexLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *IndexLogic {
	return &IndexLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *IndexLogic) Index(req *types.RoleInfoIndexReq) (resp *types.RoleInfoIndexResp, err error) {
	ret, err := l.svcCtx.RoleRpc.RoleInfoIndex(l.ctx, &sys.RoleInfoIndexReq{
		Page:   logic.ToSysPageRpc(req.Page),
		Name:   req.Name,
		Status: req.Status,
		Ids:    req.IDs,
		Codes:  req.Codes,
	})
	if err != nil {
		return nil, err
	}

	return &types.RoleInfoIndexResp{List: role.ToRoleInfosTypes(ret.List), PageResp: logic.ToPageResp(req.Page, ret.Total)}, nil
}
