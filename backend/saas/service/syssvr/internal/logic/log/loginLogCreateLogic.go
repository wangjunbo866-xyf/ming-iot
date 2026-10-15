package loglogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/stores"
	"sync"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type LoginLogCreateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	LlDB *relationDB.LoginLogRepo
}

var asyncLoginInsert *stores.AsyncInsert[relationDB.SysLoginLog]
var loginOnce sync.Once

func NewLoginLogCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogCreateLogic {
	loginOnce.Do(func() {
		asyncLoginInsert = stores.NewAsyncInsert[relationDB.SysLoginLog](stores.GetTenantConn(ctx), "")
	})
	return &LoginLogCreateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		LlDB:   relationDB.NewLoginLogRepo(ctx),
	}
}

func (l *LoginLogCreateLogic) LoginLogCreate(in *sys.LoginLogCreateReq) (*sys.Empty, error) {
	uc := ctxs.GetUserCtxNoNil(l.ctx)
	asyncLoginInsert.AsyncInsert(&relationDB.SysLoginLog{
		TenantCode:    dataType.TenantCode(uc.TenantCode),
		AppCode:       in.AppCode,
		UserID:        in.UserID,
		UserName:      in.UserName,
		IpAddr:        in.IpAddr,
		LoginLocation: in.LoginLocation,
		Browser:       in.Browser,
		Os:            in.Os,
		Code:          in.Code,
		Msg:           in.Msg,
	})
	return &sys.Empty{}, nil
}
