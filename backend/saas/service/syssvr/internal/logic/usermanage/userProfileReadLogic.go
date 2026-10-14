package usermanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type UserProfileReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUserProfileReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UserProfileReadLogic {
	return &UserProfileReadLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UserProfileReadLogic) UserProfileRead(in *sys.WithCode) (*sys.UserProfile, error) {
	ret, err := relationDB.NewUserProfileRepo(l.ctx).FindOneByFilter(l.ctx, relationDB.UserProfileFilter{
		Code:   in.Code,
		UserID: ctxs.GetUserCtxNoNil(l.ctx).UserID,
	})
	if errors.Cmp(err, errors.NotFind) {
		return &sys.UserProfile{
			Code:   in.Code,
			Params: "",
		}, nil
	}
	return utils.Copy[sys.UserProfile](ret), err
}
