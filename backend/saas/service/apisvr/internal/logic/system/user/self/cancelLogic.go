package self

import (
	"context"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"
	"github.com/zeromicro/go-zero/core/logx"
)

type CancelLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCancelLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CancelLogic {
	return &CancelLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CancelLogic) Cancel() error {
	uc := ctxs.GetUserCtx(l.ctx)
	_, err := l.svcCtx.UserRpc.UserInfoDelete(ctxs.WithAdmin(l.ctx), &sys.UserInfoDeleteReq{
		UserID: uc.UserID})
	if err != nil {
		er := errors.Fmt(err)
		l.Errorf("%s.rpc.InfoDelete err=%+v", utils.FuncName(), er)
		return er
	}
	return nil
}
