package dictmanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type DictDetailDeleteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDictDetailDeleteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DictDetailDeleteLogic {
	return &DictDetailDeleteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DictDetailDeleteLogic) DictDetailDelete(in *sys.WithID) (*sys.Empty, error) {
	if err := ctxs.IsRoot(l.ctx); err != nil {
		return nil, err
	}
	old, err := relationDB.NewDictDetailRepo(l.ctx).FindOne(l.ctx, in.Id)
	if err != nil {
		return nil, err
	}
	if old.IDPath != "" {
		err = relationDB.NewDictDetailRepo(l.ctx).DeleteByFilter(l.ctx, relationDB.DictDetailFilter{DictCode: old.DictCode, IDPath: old.IDPath})
	} else {
		err = relationDB.NewDictDetailRepo(l.ctx).Delete(l.ctx, in.Id)
	}
	return &sys.Empty{}, err
}
