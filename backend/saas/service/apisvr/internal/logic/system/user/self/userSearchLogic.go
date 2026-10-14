package self

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UserSearchLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUserSearchLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UserSearchLogic {
	return &UserSearchLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UserSearchLogic) UserSearch(req *types.UserSearchReq) (resp *types.UserSearchResp, err error) {
	if req.Account == "" {
		return nil, errors.NotFind
	}
	info, err := l.svcCtx.UserRpc.UserInfoIndex(l.ctx, &sys.UserInfoIndexReq{
		Page: &sys.PageInfo{
			Page: 1,
			Size: 1,
		},
		Account: req.Account,
	})
	if err != nil {
		return nil, err
	}
	if len(info.List) == 0 {
		return nil, errors.NotFind
	}
	return utils.Copy[types.UserSearchResp](info.List[0]), nil
}
