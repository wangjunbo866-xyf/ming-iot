package usermanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/logic"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/stores"
	"gitee.com/ming-iot/share/utils"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

type UserInfoIndexLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	UiDB *relationDB.UserInfoRepo
}

func NewUserInfoIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UserInfoIndexLogic {
	return &UserInfoIndexLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		UiDB:   relationDB.NewUserInfoRepo(ctx),
	}
}

func (l *UserInfoIndexLogic) UserInfoIndex(in *sys.UserInfoIndexReq) (*sys.UserInfoIndexResp, error) {
	l.Infof("%s req=%+v", utils.FuncName(), in)
	f := relationDB.UserInfoFilter{
		UserName:       in.UserName,
		NickName:       in.NickName,
		Phone:          in.Phone,
		Email:          in.Email,
		UserIDs:        in.UserIDs,
		HasAccessAreas: in.HasAccessAreas,
		RoleCode:       in.RoleCode,
		DeptID:         in.DeptID,
	}
	if in.UpdatedTime != nil {
		f.UpdatedTime = stores.GetCmp(in.UpdatedTime.CmpType, time.Unix(in.UpdatedTime.Value, 0))
	}
	if in.Account != "" {
		f.Accounts = []string{in.Account}
	}
	ucs, err := l.UiDB.FindByFilter(l.ctx, f, logic.ToPageInfo(in.Page).WithDefaultOrder(stores.OrderBy{
		Field: "createdTime",
		Sort:  stores.OrderDesc,
	}))
	if err != nil {
		return nil, err
	}
	total, err := l.UiDB.CountByFilter(l.ctx, f)
	if err != nil {
		return nil, err
	}
	info := make([]*sys.UserInfo, 0, len(ucs))
	for _, uc := range ucs {
		info = append(info, UserInfoToPb(l.ctx, uc, l.svcCtx))
	}
	return &sys.UserInfoIndexResp{
		List:  info,
		Total: total,
	}, nil
}
