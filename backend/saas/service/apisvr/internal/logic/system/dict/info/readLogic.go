package info

import (
	"context"
	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system/dict"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
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

func (l *ReadLogic) Read(req *types.DictInfoReadReq) (resp *types.DictInfo, err error) {
	ret, err := l.svcCtx.DictM.DictInfoRead(l.ctx,
		utils.Copy[sys.DictInfoReadReq](req))
	return dict.ToInfoTypes(ret), err
}
