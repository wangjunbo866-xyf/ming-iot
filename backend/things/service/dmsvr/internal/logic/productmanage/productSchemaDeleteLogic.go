package productmanagelogic

import (
	"context"

	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"
	"gitee.com/ming-iot/things/share/domain/schema"

	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProductSchemaDeleteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	PsDB *relationDB.ProductSchemaRepo
	PiDB *relationDB.ProductInfoRepo
	PpDB *relationDB.ProductPropertyDraftRepo
}

func NewProductSchemaDeleteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductSchemaDeleteLogic {
	return &ProductSchemaDeleteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		PsDB:   relationDB.NewProductSchemaRepo(ctx),
		PiDB:   relationDB.NewProductInfoRepo(ctx),
		PpDB:   relationDB.NewProductPropertyDraftRepo(ctx),
	}
}

// 删除产品
func (l *ProductSchemaDeleteLogic) ProductSchemaDelete(in *dm.ProductSchemaDeleteReq) (*dm.Empty, error) {
	if err := ctxs.IsRoot(l.ctx); err != nil {
		return nil, err
	}
	l.Infof("%s req=%v", utils.FuncName(), utils.Fmt(in))
	pi, err := l.PiDB.FindOneByFilter(l.ctx, relationDB.ProductFilter{ProductIDs: []string{in.ProductID}})
	if err != nil {
		return nil, err
	}
	tenantCode := dataType.TenantCode(pi.TenantCode)
	po, err := l.PsDB.FindOneByFilter(l.ctx, relationDB.ProductSchemaFilter{
		ProductID: in.ProductID, Identifiers: []string{in.Identifier},
	})
	if errors.Cmp(err, errors.NotFind) {
		po, err = l.PpDB.FindOne(l.ctx, tenantCode, in.ProductID, in.Identifier)
	}
	if err != nil {
		if errors.Cmp(err, errors.NotFind) {
			return nil, errors.Parameter.AddMsg("标识符未找到")
		}
		return nil, err
	}
	if po.Tag == schema.TagRequired {
		return nil, errors.Parameter.AddMsg("必选物模型不能删除")
	}
	if schema.AffordanceType(po.Type) == schema.AffordanceTypeProperty {
		legacy, err := l.PpDB.UsesLegacyProductSchema(l.ctx, tenantCode, in.ProductID)
		if err != nil {
			return nil, err
		}
		if legacy {
			t, err := l.svcCtx.ProductSchemaRepo.GetData(l.ctx, in.ProductID)
			if err != nil {
				return nil, err
			}
			p, ok := t.Property[in.Identifier]
			if ok {
				if err := l.svcCtx.SchemaManaRepo.DeleteProperty(l.ctx, p, in.ProductID, in.Identifier); err != nil {
					l.Errorf("%s.DeleteProperty failure,err:%v", utils.FuncName(), err)
					return nil, errors.Database.AddDetail(err)
				}
			}
		}
	}
	if po.Type == schema.AffordanceTypeProperty {
		err = l.PpDB.Delete(l.ctx, tenantCode, in.ProductID, in.Identifier)
	} else {
		err = l.PsDB.DeleteByFilter(l.ctx, relationDB.ProductSchemaFilter{ID: po.ID})
	}
	if err != nil {
		return nil, err
	}
	//清除缓存
	err = l.svcCtx.ProductSchemaRepo.SetData(l.ctx, in.ProductID, nil)
	if err != nil {
		return nil, err
	}
	return &dm.Empty{}, nil
}
