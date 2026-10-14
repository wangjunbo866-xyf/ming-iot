package relationDB

import (
	"errors"
	"testing"

	"gitee.com/ming-iot/things/service/dmsvr/internal/domain/modelrelease"
	"gitee.com/ming-iot/things/share/domain/schema"
)

func TestProductModelModuleInsertRejectsLegacyPropertyRows(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	product := task4Product("legacy-product", 0)
	if err := db.WithContext(ctx).Create(product).Error; err != nil {
		t.Fatalf("seed legacy product: %v", err)
	}
	module := &DmModelModule{TenantCode: task4Tenant, ModelModuleCode: "environment", Name: "Environment"}
	if err := NewModelModuleRepo(db).Insert(ctx, task4Tenant, module); err != nil {
		t.Fatalf("seed model module: %v", err)
	}
	legacy := &DmProductSchema{
		TenantCode: product.TenantCode,
		ProductID:  product.ProductID,
		Identifier: "temperature",
		DmSchemaCore: DmSchemaCore{
			Type:       schema.AffordanceTypeProperty,
			Tag:        schema.TagCustom,
			Name:       "Temperature",
			Affordance: `{"define":{"type":"float"},"mode":"r"}`,
			RecordMode: schema.RecordModeAll,
		},
	}
	if err := db.WithContext(ctx).Create(legacy).Error; err != nil {
		t.Fatalf("seed legacy Property: %v", err)
	}

	err := NewProductModelModuleRepo(db).Insert(ctx, task4Tenant, &DmProductModelModule{
		TenantCode: task4Tenant,
		ProductID:  product.ProductID,
		ModuleID:   module.ID,
	})
	if !errors.Is(err, modelrelease.ErrMigrationRequired) {
		t.Fatalf("bind legacy product error = %v, want ErrMigrationRequired", err)
	}
	if got := activeTask4Count(t, db, (&DmProductModelModule{}).TableName(), "tenant_code = ? AND product_id = ?", task4Tenant, product.ProductID); got != 0 {
		t.Fatalf("bindings after rejected migration = %d, want 0", got)
	}
}
