package usermanagelogic

import (
	"context"
	"strings"

	"gitee.com/ming-iot/saas/service/syssvr/internal/defext"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/def"
	"gitee.com/ming-iot/share/errors"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type UserPushClientReportLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUserPushClientReportLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UserPushClientReportLogic {
	return &UserPushClientReportLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UserPushClientReportLogic) UserPushClientReport(in *sys.UserPushClientReportReq) (*sys.Empty, error) {
	cid := strings.TrimSpace(in.GetPushClientId())
	platform := defext.NormalizePushClientPlatform(in.GetPlatform())
	uc := ctxs.GetUserCtxNoNil(l.ctx)
	repo := relationDB.NewUserPushClientRepo(l.ctx)
	tenantCode := dataType.TenantCode(uc.TenantCode)

	if platform == defext.PushClientPlatformUnbind {
		if cid == "" {
			return nil, errors.Parameter.AddMsg("pushClientId required for unbind")
		}
		if err := repo.DeactivateForUser(l.ctx, tenantCode, uc.UserID, cid); err != nil {
			return nil, err
		}
		l.Infof("push client unbind userID=%d cid=%s", uc.UserID, cid)
		return &sys.Empty{}, nil
	}

	if cid == "" {
		return nil, errors.Parameter.AddMsg("pushClientId required")
	}
	if platform == "" {
		return nil, errors.Parameter.AddMsg("platform required")
	}
	appID := strings.TrimSpace(in.GetAppId())
	if appID == "" {
		appID = l.svcCtx.Config.UniPush.AppId
	}
	if appID == "" {
		appID = "__UNI__F82AD01"
	}
	po := &relationDB.SysUserPushClient{
		TenantCode:   tenantCode,
		UserID:       uc.UserID,
		PushClientID: cid,
		Platform:     platform,
		AppID:        appID,
		AppVersion:   strings.TrimSpace(in.GetAppVersion()),
		IsActive:     def.True,
	}
	if err := repo.Upsert(l.ctx, po); err != nil {
		return nil, err
	}
	if err := repo.DeactivateOtherCidsForUser(l.ctx, tenantCode, uc.UserID, cid); err != nil {
		return nil, err
	}
	if err := repo.DeactivateOtherUsersByCid(l.ctx, tenantCode, uc.UserID, cid); err != nil {
		return nil, err
	}
	return &sys.Empty{}, nil
}
