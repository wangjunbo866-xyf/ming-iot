package relationDB

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"

	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/def"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/things/share/domain/schema"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const task4Tenant dataType.TenantCode = "task4-tenant"

func newProductInfoTask4DB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&DmProductInfo{},
		&DmProductConfig{},
		&DmProductCategory{},
		&DmProductCategorySchema{},
		&DmSchemaInfo{},
		&DmModelModule{},
		&DmModelModuleProperty{},
		&DmProductModelModule{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	// DmCommonSchema embeds a legacy fixed index name that collides with the
	// shared schema table under SQLite. Task 4 needs only these persisted fields.
	if err := db.Exec(`CREATE TABLE dm_common_schema (
		id INTEGER PRIMARY KEY,
		identifier TEXT NOT NULL,
		tag INTEGER NOT NULL DEFAULT 1,
		type INTEGER NOT NULL DEFAULT 1,
		name TEXT NOT NULL,
		affordance TEXT NOT NULL,
		record_mode INTEGER NOT NULL DEFAULT 1,
		deleted_time INTEGER NOT NULL DEFAULT 0
	)`).Error; err != nil {
		t.Fatalf("create common schema fixture table: %v", err)
	}
	return db
}

func task4Context() context.Context {
	return ctxs.SetUserCtx(context.Background(), &ctxs.UserCtx{TenantCode: string(task4Tenant)})
}

func task4Product(productID string, categoryID int64) *DmProductInfo {
	return &DmProductInfo{
		TenantCode:  dataType.TenantCodeWithDefaultR(task4Tenant),
		ProductID:   productID,
		ProductName: productID,
		CategoryID:  categoryID,
	}
}

func findTask4BaseModule(t *testing.T, db *gorm.DB, productID string) *DmModelModule {
	t.Helper()
	modules, err := NewModelModuleRepo(db).FindByFilter(task4Context(), task4Tenant, ModelModuleFilter{ModelModuleCode: BaseModelModuleCode(productID)}, nil)
	if err != nil {
		t.Fatalf("find base model module: %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("base model modules = %d, want 1", len(modules))
	}
	return modules[0]
}

func activeTask4Count(t *testing.T, db *gorm.DB, table, where string, args ...any) int64 {
	t.Helper()
	var count int64
	query := db.Table(table).Where("deleted_time = 0")
	if where != "" {
		query = query.Where(where, args...)
	}
	if err := query.Count(&count).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func TestProductInsertAtomicallyCreatesBaseModelModuleAndBinding(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	product := task4Product("product-atomic", def.NotClassified)

	if err := NewProductInfoRepo(db).Insert(ctx, product); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	module := findTask4BaseModule(t, db, product.ProductID)
	bindings, err := NewProductModelModuleRepo(db).FindByFilter(ctx, task4Tenant, ProductModelModuleFilter{ProductID: product.ProductID}, nil)
	if err != nil {
		t.Fatalf("find product-module binding: %v", err)
	}
	if len(bindings) != 1 || bindings[0].ModuleID != module.ID {
		t.Fatalf("bindings = %+v, want one binding to module %d", bindings, module.ID)
	}
	if module.ModelModuleCode != "base_"+product.ProductID {
		t.Fatalf("base model module code = %q", module.ModelModuleCode)
	}
	if err := NewModelModuleRepo(db).Insert(ctx, task4Tenant, &DmModelModule{TenantCode: task4Tenant, ModelModuleCode: "base_reserved", Name: "Reserved"}); !projecterrors.Parameter.Eq(err) {
		t.Fatalf("ordinary reserved module create error = %v, want project parameter error", err)
	}
}

func TestProductInsertRollsBackAllRowsWhenBaseBindingFails(t *testing.T) {
	db := newProductInfoTask4DB(t)
	injected := stderrors.New("injected base binding failure")
	if err := db.Callback().Create().Before("gorm:create").Register("task4:fail_base_binding", func(tx *gorm.DB) {
		if tx.Statement.Table == (&DmProductModelModule{}).TableName() {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatalf("register failure injection: %v", err)
	}

	product := task4Product("product-rollback", def.NotClassified)
	if err := NewProductInfoRepo(db).Insert(task4Context(), product); err == nil {
		t.Fatal("insert product succeeded, want injected binding failure")
	}
	checks := []struct {
		table string
		where string
		args  []any
	}{
		{table: (&DmProductInfo{}).TableName(), where: "tenant_code = ? AND product_id = ?", args: []any{task4Tenant, product.ProductID}},
		{table: (&DmModelModule{}).TableName(), where: "tenant_code = ?", args: []any{task4Tenant}},
		{table: (&DmProductModelModule{}).TableName(), where: "tenant_code = ? AND product_id = ?", args: []any{task4Tenant, product.ProductID}},
		{table: (&DmModelModuleProperty{}).TableName(), where: "tenant_code = ?", args: []any{task4Tenant}},
	}
	for _, check := range checks {
		if got := activeTask4Count(t, db, check.table, check.where, check.args...); got != 0 {
			t.Errorf("active %s rows after rollback = %d, want 0", check.table, got)
		}
	}
}

func TestBaseModelModuleCannotBindAnotherProduct(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	repo := NewProductInfoRepo(db)
	for _, productID := range []string{"product-owner", "product-other"} {
		if err := repo.Insert(ctx, task4Product(productID, def.NotClassified)); err != nil {
			t.Fatalf("insert %s: %v", productID, err)
		}
	}
	ownerModule := findTask4BaseModule(t, db, "product-owner")
	err := NewProductModelModuleRepo(db).Insert(ctx, task4Tenant, &DmProductModelModule{
		TenantCode: task4Tenant,
		ProductID:  "product-other",
		ModuleID:   ownerModule.ID,
	})
	if !projecterrors.Parameter.Eq(err) {
		t.Fatalf("cross-product base binding error = %v, want project parameter error", err)
	}
}

func TestBaseModelModuleCannotBeUnboundOrDeletedWhileProductExists(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	product := task4Product("product-protected", def.NotClassified)
	if err := NewProductInfoRepo(db).Insert(ctx, product); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	baseModule := findTask4BaseModule(t, db, product.ProductID)
	extraModule := &DmModelModule{TenantCode: task4Tenant, ModelModuleCode: "optional", Name: "Optional"}
	if err := NewModelModuleRepo(db).Insert(ctx, task4Tenant, extraModule); err != nil {
		t.Fatalf("insert ordinary module: %v", err)
	}
	if err := NewProductModelModuleRepo(db).Insert(ctx, task4Tenant, &DmProductModelModule{TenantCode: task4Tenant, ProductID: product.ProductID, ModuleID: extraModule.ID}); err != nil {
		t.Fatalf("bind ordinary module: %v", err)
	}

	if err := NewProductModelModuleRepo(db).Delete(ctx, task4Tenant, product.ProductID, baseModule.ID); !projecterrors.NotEmpty.Eq(err) {
		t.Fatalf("base unbind error = %v, want project not-empty error", err)
	}
	if err := NewModelModuleRepo(db).Delete(ctx, task4Tenant, baseModule.ID); !projecterrors.NotEmpty.Eq(err) {
		t.Fatalf("base delete error = %v, want project not-empty error", err)
	}
	if err := NewProductInfoRepo(db).Delete(ctx, product.ProductID); err != nil {
		t.Fatalf("delete owning product: %v", err)
	}
	for _, check := range []struct {
		table string
		where string
		args  []any
	}{
		{table: (&DmProductInfo{}).TableName(), where: "tenant_code = ? AND product_id = ?", args: []any{task4Tenant, product.ProductID}},
		{table: (&DmModelModule{}).TableName(), where: "tenant_code = ? AND id = ?", args: []any{task4Tenant, baseModule.ID}},
		{table: (&DmProductModelModule{}).TableName(), where: "tenant_code = ? AND product_id = ?", args: []any{task4Tenant, product.ProductID}},
	} {
		if got := activeTask4Count(t, db, check.table, check.where, check.args...); got != 0 {
			t.Errorf("active %s rows after product delete = %d, want 0", check.table, got)
		}
	}
}

func TestCategoryPropertiesEnterBaseModuleButEventsAndActionsRemainProductLevel(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	categoryID := int64(20)
	category := &DmProductCategory{ID: categoryID, Name: "Task 4 category", ParentID: def.RootNode, IDPath: fmt.Sprintf("%d-", categoryID), IsLeaf: 1}
	if err := db.Create(category).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	fixtures := []struct {
		id         int64
		identifier string
		kind       schema.AffordanceType
	}{
		{id: 101, identifier: "temperature", kind: schema.AffordanceTypeProperty},
		{id: 102, identifier: "overheat", kind: schema.AffordanceTypeEvent},
		{id: 103, identifier: "reset", kind: schema.AffordanceTypeAction},
	}
	for _, fixture := range fixtures {
		if err := db.Create(&DmProductCategorySchema{ID: fixture.id, ProductCategoryID: categoryID, Identifier: fixture.identifier}).Error; err != nil {
			t.Fatalf("map category schema %s: %v", fixture.identifier, err)
		}
		if err := db.Table((&DmCommonSchema{}).TableName()).Create(map[string]any{
			"id": fixture.id, "identifier": fixture.identifier, "tag": schema.TagOptional,
			"type": fixture.kind, "name": fixture.identifier, "affordance": `{}`, "record_mode": schema.RecordModeAll, "deleted_time": 0,
		}).Error; err != nil {
			t.Fatalf("create common schema %s: %v", fixture.identifier, err)
		}
	}

	product := task4Product("product-category", categoryID)
	if err := NewProductInfoRepo(db).Insert(ctx, product); err != nil {
		t.Fatalf("insert categorized product: %v", err)
	}
	baseModule := findTask4BaseModule(t, db, product.ProductID)
	properties, err := NewModelModulePropertyRepo(db).FindByFilter(ctx, task4Tenant, ModelModulePropertyFilter{ModuleID: baseModule.ID}, nil)
	if err != nil {
		t.Fatalf("find inherited module properties: %v", err)
	}
	if len(properties) != 1 || properties[0].Identifier != "temperature" || properties[0].Type != schema.AffordanceTypeProperty {
		t.Fatalf("inherited module properties = %+v", properties)
	}

	var productSchemas []DmSchemaInfo
	if err := db.Where("tenant_code = ? AND product_id = ? AND device_name IS NULL AND deleted_time = 0", task4Tenant, product.ProductID).Order("identifier").Find(&productSchemas).Error; err != nil {
		t.Fatalf("find inherited product schema: %v", err)
	}
	if len(productSchemas) != 2 || productSchemas[0].Identifier != "overheat" || productSchemas[0].Type != schema.AffordanceTypeEvent || productSchemas[1].Identifier != "reset" || productSchemas[1].Type != schema.AffordanceTypeAction {
		t.Fatalf("product-level inherited schemas = %+v", productSchemas)
	}
	var legacyPropertyCount int64
	if err := db.Model(&DmSchemaInfo{}).Where("tenant_code = ? AND product_id = ? AND device_name IS NULL AND type = ?", task4Tenant, product.ProductID, schema.AffordanceTypeProperty).Count(&legacyPropertyCount).Error; err != nil {
		t.Fatalf("count legacy product properties: %v", err)
	}
	if legacyPropertyCount != 0 {
		t.Fatalf("legacy product Property rows = %d, want 0", legacyPropertyCount)
	}
}
