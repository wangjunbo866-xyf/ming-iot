package relationDB

import (
	"context"

	"gitee.com/ming-iot/saas/share/dataType"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/stores"
	"gitee.com/ming-iot/things/share/domain/schema"
	"gorm.io/gorm"
)

// ProductDraftComposer is the only database-to-draft projection used by
// editable TSL reads and immutable release publication.
type ProductDraftComposer struct{ db *gorm.DB }

func NewProductDraftComposer(in any) *ProductDraftComposer {
	return &ProductDraftComposer{db: stores.GetCommonConn(in)}
}

// Compose returns the module-native draft. It deliberately does not synthesize
// a module for legacy products, so release publication can require migration.
func (c ProductDraftComposer) Compose(ctx context.Context, tenantCode dataType.TenantCode, productID string) (schema.ProductDraft, error) {
	if tenantCode == "" || productID == "" {
		return schema.ProductDraft{}, projecterrors.Parameter.AddDetail("tenantCode and productID are required")
	}
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return schema.ProductDraft{}, err
	}
	draft := schema.ProductDraft{
		Product: schema.ProductSnapshot{TenantCode: string(tenantCode), ProductID: productID},
		Modules: []schema.ModuleDraft{},
		Events:  schema.Events{},
		Actions: schema.Actions{},
	}

	bindings, err := NewProductModelModuleRepo(c.db).FindByFilter(tenantCtx, tenantCode, ProductModelModuleFilter{ProductID: productID}, nil)
	if err != nil {
		return schema.ProductDraft{}, err
	}
	for _, binding := range bindings {
		module, err := NewModelModuleRepo(c.db).FindOne(tenantCtx, tenantCode, binding.ModuleID)
		if err != nil {
			return schema.ProductDraft{}, err
		}
		rows, err := NewModelModulePropertyRepo(c.db).FindByFilter(tenantCtx, tenantCode, ModelModulePropertyFilter{ModuleID: module.ID}, nil)
		if err != nil {
			return schema.ProductDraft{}, err
		}
		properties := make(schema.Properties, 0, len(rows))
		for _, row := range rows {
			if row.TenantCode != tenantCode || row.ModuleID != module.ID || row.Type != schema.AffordanceTypeProperty {
				return schema.ProductDraft{}, projecterrors.Type.AddDetail("model module contains invalid Property data")
			}
			properties = append(properties, *ToModelModulePropertyDo(row))
		}
		draft.Modules = append(draft.Modules, schema.ModuleDraft{
			ModuleID:        module.ID,
			ModelModuleCode: module.ModelModuleCode,
			Properties:      properties,
		})
	}

	rows, err := NewProductSchemaRepo(c.db).FindByFilter(tenantCtx, ProductSchemaFilter{
		ProductID: productID,
		Types:     []int64{int64(schema.AffordanceTypeEvent), int64(schema.AffordanceTypeAction)},
	}, nil)
	if err != nil {
		return schema.ProductDraft{}, err
	}
	for _, row := range rows {
		if string(row.TenantCode) != string(tenantCode) || row.ProductID != productID {
			return schema.ProductDraft{}, projecterrors.Permissions.AddDetail("product schema crossed tenant or product boundary")
		}
		switch row.Type {
		case schema.AffordanceTypeEvent:
			draft.Events = append(draft.Events, *ToEventDo(row.Identifier, &row.DmSchemaCore))
		case schema.AffordanceTypeAction:
			draft.Actions = append(draft.Actions, *ToActionDo(row.Identifier, &row.DmSchemaCore))
		default:
			return schema.ProductDraft{}, projecterrors.Type.AddDetail("product-level draft only accepts Event and Action")
		}
	}
	return draft, nil
}

// ComposeEditableModel projects a draft to the existing flat TSL surface.
// Legacy Property rows are read only when the product has no module binding.
func (c ProductDraftComposer) ComposeEditableModel(ctx context.Context, tenantCode dataType.TenantCode, productID string) (*schema.Model, error) {
	draft, err := c.Compose(ctx, tenantCode, productID)
	if err != nil {
		return nil, err
	}
	if len(draft.Modules) != 0 {
		bundle, err := schema.CompileSchemaBundle(draft)
		if err != nil {
			return nil, err
		}
		return bundle.FlatModel()
	}

	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return nil, err
	}
	rows, err := NewProductSchemaRepo(c.db).FindByFilter(tenantCtx, ProductSchemaFilter{
		ProductID: productID,
		Type:      int64(schema.AffordanceTypeProperty),
	}, nil)
	if err != nil {
		return nil, err
	}
	model := &schema.Model{
		Version:    "1.0",
		Properties: schema.Properties{},
		Events:     draft.Events,
		Actions:    draft.Actions,
		Profile:    schema.Profile{ProductID: productID},
	}
	for _, row := range rows {
		if string(row.TenantCode) != string(tenantCode) || row.ProductID != productID {
			return nil, projecterrors.Permissions.AddDetail("legacy product Property crossed tenant or product boundary")
		}
		model.Properties = append(model.Properties, *ToPropertyDo(row.Identifier, &row.DmSchemaCore))
	}
	if err := model.ValidateWithFmt(); err != nil {
		return nil, err
	}
	return model, nil
}
