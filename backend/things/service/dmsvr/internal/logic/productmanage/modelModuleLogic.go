package productmanagelogic

import (
	"context"

	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/ctxs"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/stores"
	"gitee.com/ming-iot/things/service/dmsvr/internal/logic"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"
	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
	"gitee.com/ming-iot/things/share/domain/schema"
	"github.com/zeromicro/go-zero/core/logx"
)

func requestTenant(ctx context.Context) dataType.TenantCode {
	return dataType.TenantCode(ctxs.GetUserCtxNoNil(ctx).TenantCode)
}

func toModelModulePB(row *relationDB.DmModelModule) *dm.ModelModule {
	if row == nil {
		return nil
	}
	return &dm.ModelModule{
		ModelModuleID:   row.ID,
		ModelModuleCode: row.ModelModuleCode,
		Name:            row.Name,
		Description:     row.Description,
	}
}

func toModelModulePropertyPB(row *relationDB.DmModelModuleProperty) *dm.ModelModuleProperty {
	if row == nil {
		return nil
	}
	return &dm.ModelModuleProperty{
		ModelModulePropertyID: row.ID,
		ModelModuleID:         row.ModuleID,
		Identifier:            row.Identifier,
		Name:                  row.Name,
		Description:           row.Desc,
		Tag:                   int64(row.Tag),
		Required:              row.Required,
		IsCanSceneLinkage:     row.IsCanSceneLinkage,
		FuncGroup:             row.FuncGroup,
		UserPerm:              row.UserPerm,
		ControlMode:           row.ControlMode,
		RecordMode:            int64(row.RecordMode),
		IsPassword:            row.IsPassword,
		Affordance:            row.Affordance,
		ExtendConfig:          row.ExtendConfig,
		Order:                 row.Order,
	}
}

func modelModulePropertyRow(tenant dataType.TenantCode, in *dm.ModelModuleProperty) (*relationDB.DmModelModuleProperty, error) {
	if in == nil || in.ModelModuleID <= 0 || in.Identifier == "" {
		return nil, projecterrors.Parameter.AddDetail("modelModuleID and Property identifier are required")
	}
	if err := schema.IDValidate(in.Identifier); err != nil {
		return nil, err
	}
	return &relationDB.DmModelModuleProperty{
		ID:                in.ModelModulePropertyID,
		TenantCode:        tenant,
		ModuleID:          in.ModelModuleID,
		Identifier:        in.Identifier,
		Name:              in.Name,
		Desc:              in.Description,
		Tag:               schema.Tag(in.Tag),
		Type:              schema.AffordanceTypeProperty,
		Required:          in.Required,
		IsCanSceneLinkage: in.IsCanSceneLinkage,
		FuncGroup:         in.FuncGroup,
		UserPerm:          in.UserPerm,
		ControlMode:       in.ControlMode,
		RecordMode:        schema.RecordMode(in.RecordMode),
		IsPassword:        in.IsPassword,
		Affordance:        in.Affordance,
		ExtendConfig:      in.ExtendConfig,
		Order:             in.Order,
	}, nil
}

func invalidateModuleDrafts(ctx context.Context, svcCtx *svc.ServiceContext, tenant dataType.TenantCode, moduleID int64) error {
	if svcCtx == nil || svcCtx.ProductSchemaRepo == nil {
		return nil
	}
	bindings, err := relationDB.NewProductModelModuleRepo(ctx).FindByFilter(ctx, tenant, relationDB.ProductModelModuleFilter{ModuleID: moduleID}, nil)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		if err := svcCtx.ProductSchemaRepo.SetData(ctx, binding.ProductID, nil); err != nil {
			return err
		}
	}
	return nil
}

type ModelModuleCreateLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewModelModuleCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModuleCreateLogic {
	return &ModelModuleCreateLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ModelModuleCreateLogic) ModelModuleCreate(in *dm.ModelModuleCreateReq) (*dm.ModelModule, error) {
	if in == nil || in.ModelModuleCode == "" || in.Name == "" {
		return nil, projecterrors.Parameter.AddDetail("modelModuleCode and name are required")
	}
	if err := schema.IDValidate(in.ModelModuleCode); err != nil {
		return nil, err
	}
	tenant := requestTenant(l.ctx)
	row := &relationDB.DmModelModule{TenantCode: tenant, ModelModuleCode: in.ModelModuleCode, Name: in.Name, Description: in.Description}
	if err := relationDB.NewModelModuleRepo(l.ctx).Insert(l.ctx, tenant, row); err != nil {
		return nil, err
	}
	return toModelModulePB(row), nil
}

type ModelModuleUpdateLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewModelModuleUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModuleUpdateLogic {
	return &ModelModuleUpdateLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ModelModuleUpdateLogic) ModelModuleUpdate(in *dm.ModelModuleUpdateReq) (*dm.Empty, error) {
	if in == nil || in.ModelModuleID <= 0 || in.ModelModuleCode == "" || in.Name == "" {
		return nil, projecterrors.Parameter.AddDetail("modelModuleID, modelModuleCode and name are required")
	}
	if err := schema.IDValidate(in.ModelModuleCode); err != nil {
		return nil, err
	}
	tenant := requestTenant(l.ctx)
	row := &relationDB.DmModelModule{ID: in.ModelModuleID, TenantCode: tenant, ModelModuleCode: in.ModelModuleCode, Name: in.Name, Description: in.Description}
	if err := relationDB.NewModelModuleRepo(l.ctx).Update(l.ctx, tenant, row); err != nil {
		return nil, err
	}
	if err := invalidateModuleDrafts(l.ctx, l.ServiceContext, tenant, in.ModelModuleID); err != nil {
		return nil, err
	}
	return &dm.Empty{}, nil
}

type ModelModuleDeleteLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewModelModuleDeleteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModuleDeleteLogic {
	return &ModelModuleDeleteLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ModelModuleDeleteLogic) ModelModuleDelete(in *dm.ModelModuleDeleteReq) (*dm.Empty, error) {
	if in == nil || in.ModelModuleID <= 0 {
		return nil, projecterrors.Parameter.AddDetail("modelModuleID is required")
	}
	tenant := requestTenant(l.ctx)
	if err := relationDB.NewModelModuleRepo(l.ctx).Delete(l.ctx, tenant, in.ModelModuleID); err != nil {
		return nil, err
	}
	return &dm.Empty{}, nil
}

type ModelModuleIndexLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewModelModuleIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModuleIndexLogic {
	return &ModelModuleIndexLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ModelModuleIndexLogic) ModelModuleIndex(in *dm.ModelModuleIndexReq) (*dm.ModelModuleIndexResp, error) {
	if in == nil {
		in = &dm.ModelModuleIndexReq{}
	}
	tenant := requestTenant(l.ctx)
	filter := relationDB.ModelModuleFilter{IDs: in.ModelModuleIDs, ModelModuleCode: in.ModelModuleCode}
	repo := relationDB.NewModelModuleRepo(l.ctx)
	total, err := repo.CountByFilter(l.ctx, tenant, filter)
	if err != nil {
		return nil, err
	}
	rows, err := repo.FindByFilter(l.ctx, tenant, filter, logic.ToPageInfo(in.Page).WithDefaultOrder(stores.OrderBy{Field: "model_module_code", Sort: stores.OrderAsc}, stores.OrderBy{Field: "id", Sort: stores.OrderAsc}))
	if err != nil {
		return nil, err
	}
	list := make([]*dm.ModelModule, 0, len(rows))
	for _, row := range rows {
		list = append(list, toModelModulePB(row))
	}
	return &dm.ModelModuleIndexResp{List: list, Total: total}, nil
}

type ProductModelModuleBindLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewProductModelModuleBindLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelModuleBindLogic {
	return &ProductModelModuleBindLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ProductModelModuleBindLogic) ProductModelModuleBind(in *dm.ProductModelModuleBindReq) (*dm.Empty, error) {
	if in == nil || in.ProductID == "" || in.ModelModuleID <= 0 {
		return nil, projecterrors.Parameter.AddDetail("productID and modelModuleID are required")
	}
	tenant := requestTenant(l.ctx)
	row := &relationDB.DmProductModelModule{TenantCode: tenant, ProductID: in.ProductID, ModuleID: in.ModelModuleID, Order: in.Order}
	if err := relationDB.NewProductModelModuleRepo(l.ctx).Insert(l.ctx, tenant, row); err != nil {
		return nil, mapModelReleaseError(err)
	}
	if l.ProductSchemaRepo != nil {
		if err := l.ProductSchemaRepo.SetData(l.ctx, in.ProductID, nil); err != nil {
			return nil, err
		}
	}
	return &dm.Empty{}, nil
}

type ProductModelModuleUnbindLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewProductModelModuleUnbindLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelModuleUnbindLogic {
	return &ProductModelModuleUnbindLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ProductModelModuleUnbindLogic) ProductModelModuleUnbind(in *dm.ProductModelModuleUnbindReq) (*dm.Empty, error) {
	if in == nil || in.ProductID == "" || in.ModelModuleID <= 0 {
		return nil, projecterrors.Parameter.AddDetail("productID and modelModuleID are required")
	}
	tenant := requestTenant(l.ctx)
	if err := relationDB.NewProductModelModuleRepo(l.ctx).Delete(l.ctx, tenant, in.ProductID, in.ModelModuleID); err != nil {
		return nil, err
	}
	if l.ProductSchemaRepo != nil {
		if err := l.ProductSchemaRepo.SetData(l.ctx, in.ProductID, nil); err != nil {
			return nil, err
		}
	}
	return &dm.Empty{}, nil
}

type ProductModelModuleIndexLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewProductModelModuleIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelModuleIndexLogic {
	return &ProductModelModuleIndexLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ProductModelModuleIndexLogic) ProductModelModuleIndex(in *dm.ProductModelModuleIndexReq) (*dm.ProductModelModuleIndexResp, error) {
	if in == nil || in.ProductID == "" {
		return nil, projecterrors.Parameter.AddDetail("productID is required")
	}
	tenant := requestTenant(l.ctx)
	filter := relationDB.ProductModelModuleFilter{ProductID: in.ProductID}
	repo := relationDB.NewProductModelModuleRepo(l.ctx)
	total, err := repo.CountByFilter(l.ctx, tenant, filter)
	if err != nil {
		return nil, err
	}
	rows, err := repo.FindByFilter(l.ctx, tenant, filter, logic.ToPageInfo(in.Page))
	if err != nil {
		return nil, err
	}
	moduleRepo := relationDB.NewModelModuleRepo(l.ctx)
	list := make([]*dm.ProductModelModule, 0, len(rows))
	for _, row := range rows {
		module, err := moduleRepo.FindOne(l.ctx, tenant, row.ModuleID)
		if err != nil {
			return nil, err
		}
		list = append(list, &dm.ProductModelModule{BindingID: row.ID, ProductID: row.ProductID, ModelModuleID: row.ModuleID, Order: row.Order, ModelModule: toModelModulePB(module)})
	}
	return &dm.ProductModelModuleIndexResp{List: list, Total: total}, nil
}

type ModelModulePropertyCreateLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewModelModulePropertyCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModulePropertyCreateLogic {
	return &ModelModulePropertyCreateLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ModelModulePropertyCreateLogic) ModelModulePropertyCreate(in *dm.ModelModulePropertyCreateReq) (*dm.ModelModuleProperty, error) {
	tenant := requestTenant(l.ctx)
	row, err := modelModulePropertyRow(tenant, in.GetProperty())
	if err != nil {
		return nil, err
	}
	row.ID = 0
	if err := relationDB.NewModelModulePropertyRepo(l.ctx).Insert(l.ctx, tenant, row); err != nil {
		return nil, err
	}
	if err := invalidateModuleDrafts(l.ctx, l.ServiceContext, tenant, row.ModuleID); err != nil {
		return nil, err
	}
	return toModelModulePropertyPB(row), nil
}

type ModelModulePropertyUpdateLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewModelModulePropertyUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModulePropertyUpdateLogic {
	return &ModelModulePropertyUpdateLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ModelModulePropertyUpdateLogic) ModelModulePropertyUpdate(in *dm.ModelModulePropertyUpdateReq) (*dm.Empty, error) {
	tenant := requestTenant(l.ctx)
	row, err := modelModulePropertyRow(tenant, in.GetProperty())
	if err != nil {
		return nil, err
	}
	if row.ID <= 0 {
		return nil, projecterrors.Parameter.AddDetail("modelModulePropertyID is required")
	}
	if err := relationDB.NewModelModulePropertyRepo(l.ctx).Update(l.ctx, tenant, row); err != nil {
		return nil, err
	}
	if err := invalidateModuleDrafts(l.ctx, l.ServiceContext, tenant, row.ModuleID); err != nil {
		return nil, err
	}
	return &dm.Empty{}, nil
}

type ModelModulePropertyDeleteLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewModelModulePropertyDeleteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModulePropertyDeleteLogic {
	return &ModelModulePropertyDeleteLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ModelModulePropertyDeleteLogic) ModelModulePropertyDelete(in *dm.ModelModulePropertyDeleteReq) (*dm.Empty, error) {
	if in == nil || in.ModelModuleID <= 0 || in.ModelModulePropertyID <= 0 {
		return nil, projecterrors.Parameter.AddDetail("modelModuleID and modelModulePropertyID are required")
	}
	tenant := requestTenant(l.ctx)
	if err := relationDB.NewModelModulePropertyRepo(l.ctx).Delete(l.ctx, tenant, in.ModelModuleID, in.ModelModulePropertyID); err != nil {
		return nil, err
	}
	if err := invalidateModuleDrafts(l.ctx, l.ServiceContext, tenant, in.ModelModuleID); err != nil {
		return nil, err
	}
	return &dm.Empty{}, nil
}

type ModelModulePropertyIndexLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewModelModulePropertyIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModulePropertyIndexLogic {
	return &ModelModulePropertyIndexLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ModelModulePropertyIndexLogic) ModelModulePropertyIndex(in *dm.ModelModulePropertyIndexReq) (*dm.ModelModulePropertyIndexResp, error) {
	if in == nil || in.ModelModuleID <= 0 {
		return nil, projecterrors.Parameter.AddDetail("modelModuleID is required")
	}
	tenant := requestTenant(l.ctx)
	filter := relationDB.ModelModulePropertyFilter{ModuleID: in.ModelModuleID, Identifiers: in.Identifiers}
	repo := relationDB.NewModelModulePropertyRepo(l.ctx)
	total, err := repo.CountByFilter(l.ctx, tenant, filter)
	if err != nil {
		return nil, err
	}
	rows, err := repo.FindByFilter(l.ctx, tenant, filter, logic.ToPageInfo(in.Page).WithDefaultOrder(stores.OrderBy{Field: "order", Sort: stores.OrderAsc}, stores.OrderBy{Field: "identifier", Sort: stores.OrderAsc}))
	if err != nil {
		return nil, err
	}
	list := make([]*dm.ModelModuleProperty, 0, len(rows))
	for _, row := range rows {
		list = append(list, toModelModulePropertyPB(row))
	}
	return &dm.ModelModulePropertyIndexResp{List: list, Total: total}, nil
}
