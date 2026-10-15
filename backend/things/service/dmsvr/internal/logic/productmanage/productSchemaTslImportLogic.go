package productmanagelogic

import (
	"context"

	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"
	"gitee.com/ming-iot/things/share/domain/schema"
	"github.com/spf13/cast"

	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProductSchemaTslImportLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	PiDB *relationDB.ProductInfoRepo
}

func NewProductSchemaTslImportLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductSchemaTslImportLogic {
	return &ProductSchemaTslImportLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		PiDB:   relationDB.NewProductInfoRepo(ctx),
	}
}

// 删除产品
func (l *ProductSchemaTslImportLogic) ProductSchemaTslImport(in *dm.ProductSchemaTslImportReq) (*dm.Empty, error) {
	if err := ctxs.IsRoot(l.ctx); err != nil {
		return nil, err
	}
	l.Infof("%s req:%v", utils.FuncName(), in)
	pi, err := l.PiDB.FindOneByFilter(l.ctx, relationDB.ProductFilter{ProductIDs: []string{in.ProductID}})
	if err != nil {
		if errors.Cmp(err, errors.NotFind) {
			return nil, errors.Parameter.AddDetail("not find ProductID id:" + cast.ToString(in.ProductID))
		}
		return nil, err
	}
	t, err := schema.ValidateWithFmt([]byte(in.Tsl))
	if err != nil {
		return nil, err
	}
	propertyRepo := relationDB.NewProductPropertyDraftRepo(l.ctx)
	legacy, err := propertyRepo.UsesLegacyProductSchema(l.ctx, dataType.TenantCode(pi.TenantCode), in.ProductID)
	if err != nil {
		return nil, err
	}
	if legacy { // 只有未迁移产品继续维护旧“一属性一表”运行时结构。
		db := relationDB.NewProductSchemaRepo(l.ctx)
		dbSchemas, err := db.FindByFilter(l.ctx, relationDB.ProductSchemaFilter{ProductID: in.ProductID}, nil)
		if err != nil {
			return nil, err
		}
		oldT := relationDB.ToSchemaDo(in.ProductID, utils.CopySlice[relationDB.DmSchemaInfo](dbSchemas))
		if err := l.svcCtx.SchemaManaRepo.DeleteProduct(l.ctx, oldT, in.ProductID); err != nil {
			l.Errorf("%s.SchemaManaRepo.InitProduct failure,err:%v", utils.FuncName(), err)
			return nil, err
		}
		if err := l.svcCtx.SchemaManaRepo.InitProduct(l.ctx, t, in.ProductID); err != nil {
			l.Errorf("%s.SchemaManaRepo.InitProduct failure,err:%v", utils.FuncName(), err)
			return nil, err
		}
	}
	if err := propertyRepo.ReplaceFlatModel(l.ctx, pi, t); err != nil {
		l.Errorf("%s.ReplaceFlatModel failure,err:%v", utils.FuncName(), err)
		return nil, err
	}
	err = l.svcCtx.ProductSchemaRepo.SetData(l.ctx, in.ProductID, t)
	if err != nil {
		return nil, err
	}
	return &dm.Empty{}, nil
}
