package dictmanagelogic

import (
	"context"
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/stores"
	"gorm.io/gorm"

	"gitee.com/ming-iot/saas/service/syssvr/internal/svc"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"

	"github.com/zeromicro/go-zero/core/logx"
)

type DictInfoDeleteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDictInfoDeleteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DictInfoDeleteLogic {
	return &DictInfoDeleteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DictInfoDeleteLogic) DictInfoDelete(in *sys.WithID) (*sys.Empty, error) {
	if err := ctxs.IsRoot(l.ctx); err != nil {
		return nil, err
	}
	old, err := relationDB.NewDictInfoRepo(l.ctx).FindOne(l.ctx, in.Id)
	if err != nil {
		return nil, err
	}
	err = stores.GetCommonConn(l.ctx).Transaction(func(tx *gorm.DB) error {
		err := relationDB.NewDictInfoRepo(tx).Delete(l.ctx, in.Id)
		if err != nil {
			return err
		}
		err = relationDB.NewDictDetailRepo(tx).DeleteByFilter(l.ctx, relationDB.DictDetailFilter{DictCode: old.Code})
		return err
	})
	return &sys.Empty{}, err
}
