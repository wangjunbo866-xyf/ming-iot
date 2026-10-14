package relationDB

import (
	"testing"

	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/def"
	"gitee.com/ming-iot/things/share/domain/schema"
)

func TestDraftComposerUsesModulePropertiesAndProductEventsActions(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	productID := "product-composed-draft"
	if err := NewProductInfoRepo(db).Insert(ctx, task4Product(productID, def.NotClassified)); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	if err := NewProductPropertyDraftRepo(db).Insert(ctx, task4Tenant, task5PropertyDraft(productID, "module-property")); err != nil {
		t.Fatalf("insert module Property: %v", err)
	}
	// A stale legacy Property must not leak into a migrated product's draft.
	legacy := task5PropertyDraft(productID, "legacy-property")
	if err := db.WithContext(ctx).Create(legacy).Error; err != nil {
		t.Fatalf("insert stale legacy Property: %v", err)
	}
	event := &DmProductSchema{
		TenantCode: dataType.TenantCodeWithDefaultR(task4Tenant),
		ProductID:  productID,
		Identifier: "alarm",
		DmSchemaCore: ToEventPo(&schema.Event{
			CommonParam: schema.CommonParam{Identifier: "alarm", Name: "Alarm"},
			Type:        schema.EventTypeInfo,
			Params:      schema.Params{},
		}),
	}
	action := &DmProductSchema{
		TenantCode: dataType.TenantCodeWithDefaultR(task4Tenant),
		ProductID:  productID,
		Identifier: "reset",
		DmSchemaCore: ToActionPo(&schema.Action{
			CommonParam: schema.CommonParam{Identifier: "reset", Name: "Reset"},
			Dir:         schema.ActionDirDown,
			Input:       schema.Params{},
			Output:      schema.Params{},
		}),
	}
	if err := NewProductSchemaRepo(db).MultiInsert(ctx, []*DmProductSchema{event, action}); err != nil {
		t.Fatalf("insert product Event/Action: %v", err)
	}

	draft, err := NewProductDraftComposer(db).Compose(ctx, task4Tenant, productID)
	if err != nil {
		t.Fatalf("compose draft: %v", err)
	}
	if len(draft.Modules) != 1 || len(draft.Modules[0].Properties) != 1 || draft.Modules[0].Properties[0].Identifier != "module-property" {
		t.Fatalf("composed module properties = %+v", draft.Modules)
	}
	if len(draft.Events) != 1 || draft.Events[0].Identifier != "alarm" {
		t.Fatalf("composed events = %+v", draft.Events)
	}
	if len(draft.Actions) != 1 || draft.Actions[0].Identifier != "reset" {
		t.Fatalf("composed actions = %+v", draft.Actions)
	}
}

func TestDraftComposerFallsBackOnlyForUnmigratedProduct(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	composer := NewProductDraftComposer(db)

	migratedID := "product-migrated-read"
	if err := NewProductInfoRepo(db).Insert(ctx, task4Product(migratedID, def.NotClassified)); err != nil {
		t.Fatalf("insert migrated product: %v", err)
	}
	if err := NewProductPropertyDraftRepo(db).Insert(ctx, task4Tenant, task5PropertyDraft(migratedID, "module-property")); err != nil {
		t.Fatalf("insert migrated Property: %v", err)
	}
	if err := db.WithContext(ctx).Create(task5PropertyDraft(migratedID, "stale-legacy-property")).Error; err != nil {
		t.Fatalf("insert stale migrated Property: %v", err)
	}
	migrated, err := composer.ComposeEditableModel(ctx, task4Tenant, migratedID)
	if err != nil {
		t.Fatalf("compose migrated editable model: %v", err)
	}
	if len(migrated.Properties) != 1 || migrated.Properties[0].Identifier != "module-property" {
		t.Fatalf("migrated editable properties = %+v", migrated.Properties)
	}

	unmigratedID := "product-unmigrated-read"
	unmigrated := task4Product(unmigratedID, def.NotClassified)
	if err := db.WithContext(ctx).Create(unmigrated).Error; err != nil {
		t.Fatalf("seed unmigrated product: %v", err)
	}
	if err := db.WithContext(ctx).Create(task5PropertyDraft(unmigratedID, "legacy-property")).Error; err != nil {
		t.Fatalf("insert legacy Property: %v", err)
	}
	legacy, err := composer.ComposeEditableModel(ctx, task4Tenant, unmigratedID)
	if err != nil {
		t.Fatalf("compose legacy editable model: %v", err)
	}
	if len(legacy.Properties) != 1 || legacy.Properties[0].Identifier != "legacy-property" {
		t.Fatalf("legacy editable properties = %+v", legacy.Properties)
	}
	draft, err := composer.Compose(ctx, task4Tenant, unmigratedID)
	if err != nil {
		t.Fatalf("compose unmigrated release draft: %v", err)
	}
	if len(draft.Modules) != 0 {
		t.Fatalf("unmigrated release draft gained synthetic modules: %+v", draft.Modules)
	}
}

func TestDeviceSchemaAggregationDoesNotSeeOtherDeviceRows(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	productID := "product-device-schema-scope"
	productRow := &DmProductSchema{
		TenantCode: dataType.TenantCodeWithDefaultR(task4Tenant), ProductID: productID, Identifier: "product-property",
		DmSchemaCore: ToPropertyPo(&schema.Property{CommonParam: schema.CommonParam{Identifier: "product-property", Name: "Product", Tag: schema.TagCustom}, Define: schema.Define{Type: schema.DataTypeBool}}),
	}
	if err := NewProductSchemaRepo(db).Insert(ctx, productRow); err != nil {
		t.Fatalf("insert product schema fixture: %v", err)
	}
	deviceRows := []*DmDeviceSchema{
		{
			ProductID: productID, DeviceName: "device-a", Identifier: "device-a-property",
			DmSchemaCore: ToPropertyPo(&schema.Property{CommonParam: schema.CommonParam{Identifier: "device-a-property", Name: "A", Tag: schema.TagDeviceCustom}, Define: schema.Define{Type: schema.DataTypeBool}}),
		},
		{
			ProductID: productID, DeviceName: "device-b", Identifier: "device-b-property",
			DmSchemaCore: ToPropertyPo(&schema.Property{CommonParam: schema.CommonParam{Identifier: "device-b-property", Name: "B", Tag: schema.TagDeviceCustom}, Define: schema.Define{Type: schema.DataTypeBool}}),
		},
	}
	for _, row := range deviceRows {
		if err := NewDeviceSchemaRepo(db).Insert(ctx, row); err != nil {
			t.Fatalf("insert device schema fixture: %v", err)
		}
	}
	got, err := NewDeviceSchemaRepo(db).FindByFilter(ctx, DeviceSchemaFilter{ProductID: productID, DeviceName: "device-a", WithProductSchema: true}, nil)
	if err != nil {
		t.Fatalf("find device schema: %v", err)
	}
	identifiers := make(map[string]bool, len(got))
	for _, row := range got {
		identifiers[row.Identifier] = true
	}
	if !identifiers["product-property"] || !identifiers["device-a-property"] || identifiers["device-b-property"] {
		t.Fatalf("aggregated identifiers = %v", identifiers)
	}
}
