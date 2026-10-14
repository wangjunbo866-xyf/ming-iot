package relationDB

import (
	"context"
	"database/sql"

	"gitee.com/ming-iot/saas/share/dataType"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/things/share/domain/schema"
	"gorm.io/gorm"
)

// ProductPropertyDraftRepo preserves the legacy flat Property API while making
// a bound ModelModule the only writable source for migrated products.
type ProductPropertyDraftRepo struct{ db *gorm.DB }

func NewProductPropertyDraftRepo(in any) *ProductPropertyDraftRepo {
	return &ProductPropertyDraftRepo{db: NewProductInfoRepo(in).db}
}

func (p ProductPropertyDraftRepo) resolveWriteTarget(ctx context.Context, tenantCode dataType.TenantCode, productID string) (moduleID int64, legacy bool, err error) {
	if tenantCode == "" || productID == "" {
		return 0, false, projecterrors.Parameter.AddDetail("tenantCode and productID are required")
	}
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return 0, false, err
	}
	var productCount int64
	if err := p.db.WithContext(tenantCtx).Table((&DmProductInfo{}).TableName()).Where("tenant_code = ? AND product_id = ? AND deleted_time = 0", tenantCode, productID).Count(&productCount).Error; err != nil {
		return 0, false, modelReleaseDBError(err)
	}
	if productCount != 1 {
		return 0, false, projecterrors.NotFind.AddDetail("product not found in tenant")
	}
	bindings, err := NewProductModelModuleRepo(p.db).FindByFilter(tenantCtx, tenantCode, ProductModelModuleFilter{ProductID: productID}, nil)
	if err != nil {
		return 0, false, err
	}
	switch len(bindings) {
	case 0:
		return 0, true, nil
	case 1:
		return bindings[0].ModuleID, false, nil
	default:
		return 0, false, projecterrors.Parameter.AddDetail("multiple model modules are bound; specify modelModuleID")
	}
}

func (p ProductPropertyDraftRepo) UsesLegacyProductSchema(ctx context.Context, tenantCode dataType.TenantCode, productID string) (bool, error) {
	_, legacy, err := p.resolveWriteTarget(ctx, tenantCode, productID)
	return legacy, err
}

func (p ProductPropertyDraftRepo) Insert(ctx context.Context, tenantCode dataType.TenantCode, data *DmProductSchema) error {
	if data == nil || data.ProductID == "" || data.Identifier == "" || dataType.TenantCode(data.TenantCode) != tenantCode {
		return projecterrors.Parameter.AddDetail("flat Property identity must match the explicit tenant")
	}
	if data.Type != schema.AffordanceTypeProperty {
		return projecterrors.Type.AddDetail("flat Property repository only accepts Property")
	}
	moduleID, legacy, err := p.resolveWriteTarget(ctx, tenantCode, data.ProductID)
	if err != nil {
		return err
	}
	if legacy {
		return NewProductSchemaRepo(p.db).Insert(ctx, data)
	}
	property := ToPropertyDo(data.Identifier, &data.DmSchemaCore)
	return NewModelModulePropertyRepo(p.db).Insert(ctx, tenantCode, ToModelModulePropertyPo(string(tenantCode), moduleID, property))
}

func (p ProductPropertyDraftRepo) FindOne(ctx context.Context, tenantCode dataType.TenantCode, productID, identifier string) (*DmSchemaInfo, error) {
	moduleID, legacy, err := p.resolveWriteTarget(ctx, tenantCode, productID)
	if err != nil {
		return nil, err
	}
	if legacy {
		return NewProductSchemaRepo(p.db).FindOneByFilter(ctx, ProductSchemaFilter{ProductID: productID, Identifiers: []string{identifier}, Type: int64(schema.AffordanceTypeProperty)})
	}
	rows, err := NewModelModulePropertyRepo(p.db).FindByFilter(ctx, tenantCode, ModelModulePropertyFilter{ModuleID: moduleID, Identifiers: []string{identifier}}, nil)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, projecterrors.NotFind.WithStack(1)
	}
	row := rows[0]
	return &DmSchemaInfo{
		ID:         row.ID,
		TenantCode: string(row.TenantCode),
		ProductID:  productID,
		DeviceName: sql.NullString{},
		Identifier: row.Identifier,
		DmSchemaCore: DmSchemaCore{
			Tag:               row.Tag,
			Type:              row.Type,
			ExtendConfig:      row.ExtendConfig,
			Required:          row.Required,
			Name:              row.Name,
			Desc:              row.Desc,
			IsCanSceneLinkage: row.IsCanSceneLinkage,
			FuncGroup:         row.FuncGroup,
			ControlMode:       row.ControlMode,
			UserPerm:          row.UserPerm,
			RecordMode:        row.RecordMode,
			IsPassword:        row.IsPassword,
			Affordance:        row.Affordance,
			Order:             row.Order,
		},
	}, nil
}

func (p ProductPropertyDraftRepo) Update(ctx context.Context, tenantCode dataType.TenantCode, data *DmSchemaInfo) error {
	if data == nil || data.ProductID == "" || data.Identifier == "" || data.TenantCode != string(tenantCode) || data.Type != schema.AffordanceTypeProperty {
		return projecterrors.Parameter.AddDetail("flat Property update must match the explicit tenant")
	}
	moduleID, legacy, err := p.resolveWriteTarget(ctx, tenantCode, data.ProductID)
	if err != nil {
		return err
	}
	if legacy {
		return NewProductSchemaRepo(p.db).Update(ctx, data)
	}
	existing, err := p.FindOne(ctx, tenantCode, data.ProductID, data.Identifier)
	if err != nil {
		return err
	}
	property := ToPropertyDo(data.Identifier, &data.DmSchemaCore)
	row := ToModelModulePropertyPo(string(tenantCode), moduleID, property)
	row.ID = existing.ID
	return NewModelModulePropertyRepo(p.db).Update(ctx, tenantCode, row)
}

func (p ProductPropertyDraftRepo) Delete(ctx context.Context, tenantCode dataType.TenantCode, productID, identifier string) error {
	moduleID, legacy, err := p.resolveWriteTarget(ctx, tenantCode, productID)
	if err != nil {
		return err
	}
	if legacy {
		return NewProductSchemaRepo(p.db).DeleteByFilter(ctx, ProductSchemaFilter{ProductID: productID, Identifiers: []string{identifier}, Type: int64(schema.AffordanceTypeProperty)})
	}
	existing, err := p.FindOne(ctx, tenantCode, productID, identifier)
	if err != nil {
		return err
	}
	return NewModelModulePropertyRepo(p.db).Delete(ctx, tenantCode, moduleID, existing.ID)
}

func (p ProductPropertyDraftRepo) ReplaceFlatModel(ctx context.Context, product *DmProductInfo, model *schema.Model) error {
	if product == nil || model == nil || product.ProductID == "" || product.TenantCode == "" {
		return projecterrors.Parameter.AddDetail("product and flat model are required")
	}
	tenantCode := dataType.TenantCode(product.TenantCode)
	moduleID, legacy, err := p.resolveWriteTarget(ctx, tenantCode, product.ProductID)
	if err != nil {
		return err
	}
	if legacy {
		return NewProductSchemaRepo(p.db).MultiUpdate(ctx, product, model)
	}
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return err
	}
	return modelReleaseDBError(p.db.WithContext(tenantCtx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_code = ? AND module_id = ?", tenantCode, moduleID).Delete(&DmModelModuleProperty{}).Error; err != nil {
			return err
		}
		if err := NewProductSchemaRepo(tx).DeleteByFilter(tenantCtx, ProductSchemaFilter{ProductID: product.ProductID}); err != nil {
			return err
		}
		moduleRepo := NewModelModulePropertyRepo(tx)
		for i := range model.Properties {
			property := model.Properties[i]
			if err := moduleRepo.Insert(tenantCtx, tenantCode, ToModelModulePropertyPo(string(tenantCode), moduleID, &property)); err != nil {
				return err
			}
		}
		productSchemas := make([]*DmProductSchema, 0, len(model.Events)+len(model.Actions))
		for i := range model.Events {
			event := model.Events[i]
			productSchemas = append(productSchemas, &DmProductSchema{TenantCode: product.TenantCode, ProductID: product.ProductID, Identifier: event.Identifier, DmSchemaCore: ToEventPo(&event)})
		}
		for i := range model.Actions {
			action := model.Actions[i]
			productSchemas = append(productSchemas, &DmProductSchema{TenantCode: product.TenantCode, ProductID: product.ProductID, Identifier: action.Identifier, DmSchemaCore: ToActionPo(&action)})
		}
		if len(productSchemas) != 0 {
			if err := tx.WithContext(tenantCtx).Create(productSchemas).Error; err != nil {
				return err
			}
		}
		return nil
	}))
}
