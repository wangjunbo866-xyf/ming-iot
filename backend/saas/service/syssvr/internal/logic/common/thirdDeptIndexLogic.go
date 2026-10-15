package commonlogic

import (
	"context"
	"gitee.com/ming-iot/share/clients/dingClient"
	"gitee.com/ming-iot/share/conf"
	"gitee.com/ming-iot/share/errors"
	"github.com/zhaoyunxing92/dingtalk/v2/request"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type ThirdDeptIndexLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewThirdDeptIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ThirdDeptIndexLogic {
	return &ThirdDeptIndexLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ThirdDeptIndexLogic) ThirdDeptIndex(in *sys.ThirdDeptInfoIndexReq) (*sys.DeptInfoIndexResp, error) {
	cli, err := dingClient.NewDingTalkClient(&conf.ThirdConf{
		AppID:     in.ThirdConfig.AppID,
		AppKey:    in.ThirdConfig.AppKey,
		AppSecret: in.ThirdConfig.AppSecret,
	})
	if err != nil {
		return nil, err
	}
	req := request.NewDeptList()
	req.SetDeptId(int(in.ParentID))
	dings, err := cli.GetDeptList(req.Build())
	if err != nil {
		return nil, errors.System.AddDetail(err)
	}
	var list []*sys.DeptInfo
	for _, v := range dings.List {
		list = append(list, &sys.DeptInfo{
			Id:       int64(v.Id),
			Name:     v.Name,
			ParentID: int64(v.ParentId),
		})
	}
	return &sys.DeptInfoIndexResp{List: list}, nil
}
