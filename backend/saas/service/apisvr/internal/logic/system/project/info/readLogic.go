package info

import (
	"context"
	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ReadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReadLogic {
	return &ReadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ReadLogic) Read(req *types.ProjectWithID) (resp *types.ProjectInfo, err error) {
	dmResp, err := l.svcCtx.ProjectM.ProjectInfoRead(l.ctx, &sys.ProjectWithID{ProjectID: req.ProjectID,
		WithTopAreas: req.WithTopAreas,
	})
	if err != nil {
		er := errors.Fmt(err)
		l.Errorf("%s rpc.ProjectManage req=%v err=%+v", utils.FuncName(), req, er)
		return nil, er
	}
	var user *sys.UserInfo
	if req.WithAdminUser {
		user, err = l.svcCtx.UserCache.GetData(l.ctx, dmResp.AdminUserID)
		if err != nil {
			l.Error(err)
		}
	}

	return system.ProjectInfoToApi(dmResp, user), nil
}
