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

type ProductSchemaMultiCreateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewProductSchemaMultiCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductSchemaMultiCreateLogic {
	return &ProductSchemaMultiCreateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 批量新增物模型,只新增没有的,已有的不处理
func (l *ProductSchemaMultiCreateLogic) ProductSchemaMultiCreate(in *dm.ProductSchemaMultiCreateReq) (*dm.Empty, error) {
	if err := ctxs.IsRoot(l.ctx); err != nil {
		return nil, err
	}
	if len(in.Identifiers) == 0 && len(in.List) == 0 {
		return &dm.Empty{}, errors.Parameter.AddMsg("identifiers list must set one")
	}
	createLogic := NewProductSchemaCreateLogic(l.ctx, l.svcCtx)
	var pos []*relationDB.DmProductSchema
	if len(in.Identifiers) != 0 {
		pi, err := l.svcCtx.ProductCache.GetData(l.ctx, in.ProductID)
		if err != nil {
			return nil, err
		}
		cs, err := relationDB.NewCommonSchemaRepo(l.ctx).FindByFilter(l.ctx, relationDB.CommonSchemaFilter{Identifiers: in.Identifiers}, nil)
		if err != nil {
			return nil, err
		}
		for _, c := range cs {
			c.Tag = schema.TagOptional
			pos = append(pos, &relationDB.DmProductSchema{
				TenantCode:   dataType.TenantCodeWithDefaultR(pi.TenantCode),
				ProductID:    in.ProductID,
				Identifier:   c.Identifier,
				DmSchemaCore: c.DmSchemaCore,
			})
		}
	}
	for _, v := range in.List {
		info := v
		info.ProductID = in.ProductID
		info.Tag = schema.TagOptional
		po, err := createLogic.RuleCheck(&dm.ProductSchemaCreateReq{Info: info})
		if err != nil {
			if !errors.Cmp(errors.Duplicate, err) {
				l.Errorf("RuleCheck err:%v", err)
				return nil, err
			}
			continue
		}
		pos = append(pos, po)
	}
	if len(pos) == 0 {
		return &dm.Empty{}, nil
	}
	propertyRepo := relationDB.NewProductPropertyDraftRepo(l.ctx)
	propertyLegacy := make(map[*relationDB.DmProductSchema]bool)
	for _, po := range pos {
		if po.Type != schema.AffordanceTypeProperty {
			continue
		}
		legacy, err := propertyRepo.UsesLegacyProductSchema(l.ctx, dataType.TenantCode(po.TenantCode), po.ProductID)
		if err != nil {
			return nil, err
		}
		propertyLegacy[po] = legacy
	}
	var productSchemas []*relationDB.DmProductSchema
	for _, po := range pos {
		if po.Type == schema.AffordanceTypeProperty {
			if err := persistProductProperty(l.ctx, propertyRepo, l.svcCtx.SchemaManaRepo, po, propertyLegacy[po]); err != nil {
				return nil, err
			}
			continue
		}
		productSchemas = append(productSchemas, po)
	}
	if len(productSchemas) != 0 {
		if err := relationDB.NewProductSchemaRepo(l.ctx).MultiInsert(l.ctx, productSchemas); err != nil {
			return nil, err
		}
	}
	for _, po := range pos {
		//清除缓存
		err := l.svcCtx.ProductSchemaRepo.SetData(l.ctx, po.ProductID, nil)
		if err != nil {
			l.Errorf("%s.SetData failure,err:%v", utils.FuncName(), err)
		}
	}

	return &dm.Empty{}, nil
}
