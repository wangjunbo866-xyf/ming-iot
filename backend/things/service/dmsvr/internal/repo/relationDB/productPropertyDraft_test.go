package relationDB

import (
	"encoding/json"
	"testing"

	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/def"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/things/share/domain/schema"
)

func task5PropertyDraft(productID, identifier string) *DmProductSchema {
	return &DmProductSchema{
		TenantCode: dataType.TenantCodeWithDefaultR(task4Tenant),
		ProductID:  productID,
		Identifier: identifier,
		DmSchemaCore: ToPropertyPo(&schema.Property{
			CommonParam: schema.CommonParam{Identifier: identifier, Name: identifier, RecordMode: schema.RecordModeAll},
			Mode:        schema.PropertyModeRW,
			Define:      schema.Define{Type: schema.DataTypeBool},
		}),
	}
}

func TestFlatPropertyWriteRoutesToOnlyBoundModelModule(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	productID := "product-flat"
	if err := NewProductInfoRepo(db).Insert(ctx, task4Product(productID, def.NotClassified)); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	repo := NewProductPropertyDraftRepo(db)
	if err := repo.Insert(ctx, task4Tenant, task5PropertyDraft(productID, "switch")); err != nil {
		t.Fatalf("insert flat Property: %v", err)
	}

	base := findTask4BaseModule(t, db, productID)
	properties, err := NewModelModulePropertyRepo(db).FindByFilter(ctx, task4Tenant, ModelModulePropertyFilter{ModuleID: base.ID}, nil)
	if err != nil {
		t.Fatalf("find module properties: %v", err)
	}
	if len(properties) != 1 || properties[0].Identifier != "switch" {
		t.Fatalf("module properties = %+v, want switch", properties)
	}
	var legacyCount int64
	if err := db.Model(&DmSchemaInfo{}).Where("tenant_code = ? AND product_id = ? AND device_name IS NULL AND type = ?", task4Tenant, productID, schema.AffordanceTypeProperty).Count(&legacyCount).Error; err != nil {
		t.Fatalf("count legacy properties: %v", err)
	}
	if legacyCount != 0 {
		t.Fatalf("legacy Property rows = %d, want 0", legacyCount)
	}
}

func TestFlatPropertyWriteRejectsAmbiguousMultiModuleProduct(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	productID := "product-many"
	if err := NewProductInfoRepo(db).Insert(ctx, task4Product(productID, def.NotClassified)); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	extra := &DmModelModule{TenantCode: task4Tenant, ModelModuleCode: "environment", Name: "Environment"}
	if err := NewModelModuleRepo(db).Insert(ctx, task4Tenant, extra); err != nil {
		t.Fatalf("insert extra module: %v", err)
	}
	if err := NewProductModelModuleRepo(db).Insert(ctx, task4Tenant, &DmProductModelModule{TenantCode: task4Tenant, ProductID: productID, ModuleID: extra.ID, Order: 2}); err != nil {
		t.Fatalf("bind extra module: %v", err)
	}
	err := NewProductPropertyDraftRepo(db).Insert(ctx, task4Tenant, task5PropertyDraft(productID, "switch"))
	if !projecterrors.Parameter.Eq(err) {
		t.Fatalf("ambiguous flat Property error = %v, want project parameter error", err)
	}
}

func TestBoundProductNeverWritesLegacyPropertyRow(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	productID := "product-no-legacy"
	if err := NewProductInfoRepo(db).Insert(ctx, task4Product(productID, def.NotClassified)); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	if err := NewProductPropertyDraftRepo(db).Insert(ctx, task4Tenant, task5PropertyDraft(productID, "switch")); err != nil {
		t.Fatalf("insert Property: %v", err)
	}
	var count int64
	if err := db.Model(&DmSchemaInfo{}).Where("tenant_code = ? AND product_id = ? AND device_name IS NULL AND type = ?", task4Tenant, productID, schema.AffordanceTypeProperty).Count(&count).Error; err != nil {
		t.Fatalf("count legacy Property rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("legacy Property rows = %d, want 0", count)
	}
}

func TestFlatPropertyUpdateAndDeleteStayInModule(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	productID := "product-update"
	if err := NewProductInfoRepo(db).Insert(ctx, task4Product(productID, def.NotClassified)); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	repo := NewProductPropertyDraftRepo(db)
	if err := repo.Insert(ctx, task4Tenant, task5PropertyDraft(productID, "switch")); err != nil {
		t.Fatalf("insert Property: %v", err)
	}
	row, err := repo.FindOne(ctx, task4Tenant, productID, "switch")
	if err != nil {
		t.Fatalf("find Property: %v", err)
	}
	row.Name = "Updated switch"
	if err := repo.Update(ctx, task4Tenant, row); err != nil {
		t.Fatalf("update Property: %v", err)
	}
	got, err := repo.FindOne(ctx, task4Tenant, productID, "switch")
	if err != nil || got.Name != "Updated switch" {
		t.Fatalf("updated Property = %+v, err=%v", got, err)
	}
	if err := repo.Delete(ctx, task4Tenant, productID, "switch"); err != nil {
		t.Fatalf("delete Property: %v", err)
	}
	if _, err := repo.FindOne(ctx, task4Tenant, productID, "switch"); !projecterrors.NotFind.Eq(err) {
		t.Fatalf("find deleted Property error = %v, want not found", err)
	}
}

func TestModelModulePropertyIdentifierCannotChange(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	productID := "product-immutable-id"
	if err := NewProductInfoRepo(db).Insert(ctx, task4Product(productID, def.NotClassified)); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	repo := NewProductPropertyDraftRepo(db)
	if err := repo.Insert(ctx, task4Tenant, task5PropertyDraft(productID, "before")); err != nil {
		t.Fatalf("insert Property: %v", err)
	}
	base := findTask4BaseModule(t, db, productID)
	rows, err := NewModelModulePropertyRepo(db).FindByFilter(ctx, task4Tenant, ModelModulePropertyFilter{ModuleID: base.ID, Identifiers: []string{"before"}}, nil)
	if err != nil || len(rows) != 1 {
		t.Fatalf("find module Property: rows=%+v err=%v", rows, err)
	}
	changed := *rows[0]
	changed.Identifier = "after"
	if err := NewModelModulePropertyRepo(db).Update(ctx, task4Tenant, &changed); !projecterrors.Parameter.Eq(err) {
		t.Fatalf("identifier change error = %v, want project parameter error", err)
	}
}

func TestFlatModelReplaceUsesBoundModuleForProperties(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	product := task4Product("product-import", def.NotClassified)
	if err := NewProductInfoRepo(db).Insert(ctx, product); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	model := &schema.Model{
		Properties: schema.Properties{
			{CommonParam: schema.CommonParam{Identifier: "temperature", Name: "Temperature", RecordMode: schema.RecordModeAll}, Mode: schema.PropertyModeRW, Define: schema.Define{Type: schema.DataTypeFloat}},
			{CommonParam: schema.CommonParam{Identifier: "enabled", Name: "Enabled", RecordMode: schema.RecordModeNone}, Mode: schema.PropertyModeRW, Define: schema.Define{Type: schema.DataTypeBool}},
		},
		Events:  schema.Events{{CommonParam: schema.CommonParam{Identifier: "alarm", Name: "Alarm"}}},
		Actions: schema.Actions{{CommonParam: schema.CommonParam{Identifier: "reset", Name: "Reset"}}},
	}
	if err := NewProductPropertyDraftRepo(db).ReplaceFlatModel(ctx, product, model); err != nil {
		t.Fatalf("replace flat model: %v", err)
	}
	base := findTask4BaseModule(t, db, product.ProductID)
	properties, err := NewModelModulePropertyRepo(db).FindByFilter(ctx, task4Tenant, ModelModulePropertyFilter{ModuleID: base.ID}, nil)
	if err != nil || len(properties) != 2 {
		t.Fatalf("module properties = %+v, err=%v", properties, err)
	}
	productSchemas, err := NewProductSchemaRepo(db).FindByFilter(ctx, ProductSchemaFilter{ProductID: product.ProductID}, nil)
	if err != nil || len(productSchemas) != 2 {
		t.Fatalf("product Event/Action rows = %+v, err=%v", productSchemas, err)
	}
	for _, item := range productSchemas {
		if item.Type == schema.AffordanceTypeProperty {
			t.Fatalf("Property %q leaked into product-level storage", item.Identifier)
		}
	}
}

func TestLegacyProductPropertyWriteRemainsUntilMigration(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	productID := "product-legacy"
	if err := db.WithContext(ctx).Create(task4Product(productID, def.NotClassified)).Error; err != nil {
		t.Fatalf("seed legacy product: %v", err)
	}
	if err := NewProductPropertyDraftRepo(db).Insert(ctx, task4Tenant, task5PropertyDraft(productID, "switch")); err != nil {
		t.Fatalf("insert legacy Property: %v", err)
	}
	var legacyCount int64
	if err := db.Model(&DmSchemaInfo{}).Where("tenant_code = ? AND product_id = ? AND device_name IS NULL AND identifier = ?", task4Tenant, productID, "switch").Count(&legacyCount).Error; err != nil {
		t.Fatalf("count legacy property: %v", err)
	}
	if legacyCount != 1 {
		t.Fatalf("legacy Property rows = %d, want 1", legacyCount)
	}
}

func TestModelModuleRejectsEventAndAction(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	productID := "product-kinds"
	if err := NewProductInfoRepo(db).Insert(ctx, task4Product(productID, def.NotClassified)); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	for _, kind := range []schema.AffordanceType{schema.AffordanceTypeEvent, schema.AffordanceTypeAction} {
		row := task5PropertyDraft(productID, "wrong-kind")
		row.Type = kind
		if err := NewProductPropertyDraftRepo(db).Insert(ctx, task4Tenant, row); !projecterrors.Type.Eq(err) {
			t.Fatalf("kind %d error = %v, want project type error", kind, err)
		}
	}
}

func TestModelModuleCannotCrossTenantBoundary(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	productID := "product-tenant-boundary"
	if err := NewProductInfoRepo(db).Insert(ctx, task4Product(productID, def.NotClassified)); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	base := findTask4BaseModule(t, db, productID)
	otherTenant := dataType.TenantCode("other-tenant")
	row := ToModelModulePropertyPo(string(otherTenant), base.ID, ToPropertyDo("foreign", &task5PropertyDraft(productID, "foreign").DmSchemaCore))
	if err := NewModelModulePropertyRepo(db).Insert(ctx, otherTenant, row); !projecterrors.Permissions.Eq(err) {
		t.Fatalf("cross-tenant insert error = %v, want permissions error", err)
	}
	if _, err := NewModelModuleRepo(db).FindOne(ctx, otherTenant, base.ID); !projecterrors.Permissions.Eq(err) {
		t.Fatalf("cross-tenant module read error = %v, want permissions error", err)
	}
}

func TestBaseNamespaceCannotBeCreatedByUser(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	module := &DmModelModule{TenantCode: task4Tenant, ModelModuleCode: "base_user_defined", Name: "Forbidden"}
	if err := NewModelModuleRepo(db).Insert(ctx, task4Tenant, module); !projecterrors.Parameter.Eq(err) {
		t.Fatalf("base namespace error = %v, want parameter error", err)
	}
}

func TestRecordModeOverridesLegacyIsNoRecordOnWrite(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	productID := "product-record-mode"
	if err := NewProductInfoRepo(db).Insert(ctx, task4Product(productID, def.NotClassified)); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	draft := task5PropertyDraft(productID, "power")
	draft.RecordMode = schema.RecordModeAll
	var affordance schema.PropertyAffordance
	if err := json.Unmarshal([]byte(draft.Affordance), &affordance); err != nil {
		t.Fatalf("decode affordance: %v", err)
	}
	affordance.IsNoRecord = true
	payload, err := json.Marshal(affordance)
	if err != nil {
		t.Fatalf("encode affordance: %v", err)
	}
	draft.Affordance = string(payload)
	if err := NewProductPropertyDraftRepo(db).Insert(ctx, task4Tenant, draft); err != nil {
		t.Fatalf("insert property: %v", err)
	}
	base := findTask4BaseModule(t, db, productID)
	rows, err := NewModelModulePropertyRepo(db).FindByFilter(ctx, task4Tenant, ModelModulePropertyFilter{ModuleID: base.ID}, nil)
	if err != nil || len(rows) != 1 {
		t.Fatalf("module properties = %+v, err=%v", rows, err)
	}
	property := ToModelModulePropertyDo(rows[0])
	if property.RecordMode != schema.RecordModeAll || property.IsNoRecord {
		t.Fatalf("record policy = mode %d, isNoRecord %v; want all,false", property.RecordMode, property.IsNoRecord)
	}
}

func TestBoundModelModuleCannotBeDeleted(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	productID := "product-bound-module"
	if err := NewProductInfoRepo(db).Insert(ctx, task4Product(productID, def.NotClassified)); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	module := &DmModelModule{TenantCode: task4Tenant, ModelModuleCode: "shared", Name: "Shared"}
	if err := NewModelModuleRepo(db).Insert(ctx, task4Tenant, module); err != nil {
		t.Fatalf("insert module: %v", err)
	}
	if err := NewProductModelModuleRepo(db).Insert(ctx, task4Tenant, &DmProductModelModule{TenantCode: task4Tenant, ProductID: productID, ModuleID: module.ID, Order: 2}); err != nil {
		t.Fatalf("bind module: %v", err)
	}
	if err := NewModelModuleRepo(db).Delete(ctx, task4Tenant, module.ID); !projecterrors.NotEmpty.Eq(err) {
		t.Fatalf("delete bound module error = %v, want not-empty error", err)
	}
}
