package relationDB

import (
	"bytes"
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/ctxs"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/things/share/domain/schema"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var modelReleaseTestID atomic.Int64

const (
	tenantA dataType.TenantCode = "tenant-a"
	tenantB dataType.TenantCode = "tenant-b"
)

func newModelReleaseTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&DmProductInfo{},
		&DmSchemaInfo{},
		&DmModelModule{},
		&DmModelModuleProperty{},
		&DmProductModelModule{},
		&DmProductModelRelease{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func createTestProduct(t *testing.T, db *gorm.DB, tenant dataType.TenantCode, productID string) {
	t.Helper()
	if err := db.Table((&DmProductInfo{}).TableName()).Create(map[string]any{
		"tenant_code":  string(tenant),
		"product_id":   productID,
		"product_name": productID,
		"customUi":     `{}`,
		"deleted_time": 0,
	}).Error; err != nil {
		t.Fatalf("create product %q: %v", productID, err)
	}
}

func createTestModule(t *testing.T, db *gorm.DB, tenant dataType.TenantCode, code string) *DmModelModule {
	t.Helper()
	module := &DmModelModule{ID: modelReleaseTestID.Add(1), TenantCode: tenant, ModelModuleCode: code, Name: code}
	if err := NewModelModuleRepo(db).Insert(context.Background(), tenant, module); err != nil {
		t.Fatalf("create module %q: %v", code, err)
	}
	return module
}

func canonicalReleaseBundle(t *testing.T, tenant dataType.TenantCode, productID string) ([]byte, string) {
	t.Helper()
	bundle, err := schema.CompileSchemaBundle(schema.ProductDraft{
		Product: schema.ProductSnapshot{TenantCode: string(tenant), ProductID: productID},
		Modules: []schema.ModuleDraft{{
			ModuleID:        1,
			ModelModuleCode: "base_" + productID,
			Properties:      schema.Properties{},
		}},
		Events:  schema.Events{},
		Actions: schema.Actions{},
	})
	if err != nil {
		t.Fatalf("compile release bundle: %v", err)
	}
	payload, err := bundle.CanonicalBytes()
	if err != nil {
		t.Fatalf("canonical release bundle: %v", err)
	}
	hash, err := bundle.SHA256()
	if err != nil {
		t.Fatalf("hash release bundle: %v", err)
	}
	return payload, hash
}

func TestModelModuleCodeIsUniqueWithinTenant(t *testing.T) {
	db := newModelReleaseTestDB(t)
	repo := NewModelModuleRepo(db)

	if err := repo.Insert(context.Background(), tenantA, &DmModelModule{TenantCode: tenantA, ModelModuleCode: "environment", Name: "Environment"}); err != nil {
		t.Fatalf("insert first module: %v", err)
	}
	if err := repo.Insert(context.Background(), tenantA, &DmModelModule{TenantCode: tenantA, ModelModuleCode: "environment", Name: "Duplicate"}); !projecterrors.Duplicate.Eq(err) {
		t.Fatalf("duplicate module error = %v, want project duplicate error", err)
	}
	if err := repo.Insert(context.Background(), tenantB, &DmModelModule{TenantCode: tenantB, ModelModuleCode: "environment", Name: "Other tenant"}); err != nil {
		t.Fatalf("same code in another tenant: %v", err)
	}
}

func TestModelModulePropertyRequiresSameTenantAndPropertyKind(t *testing.T) {
	db := newModelReleaseTestDB(t)
	module := createTestModule(t, db, tenantA, "environment")
	repo := NewModelModulePropertyRepo(db)

	property := &DmModelModuleProperty{
		TenantCode: tenantA,
		ModuleID:   module.ID,
		Identifier: "temperature",
		Type:       schema.AffordanceTypeProperty,
		Name:       "Temperature",
		Affordance: `{}`,
	}
	if err := repo.Insert(context.Background(), tenantA, property); err != nil {
		t.Fatalf("insert property: %v", err)
	}

	crossTenant := *property
	crossTenant.ID = 0
	crossTenant.TenantCode = tenantB
	crossTenant.Identifier = "humidity"
	if err := repo.Insert(context.Background(), tenantB, &crossTenant); !projecterrors.NotFind.Eq(err) {
		t.Fatalf("cross-tenant property error = %v, want project not-found error", err)
	}

	event := *property
	event.ID = 0
	event.Identifier = "alarm"
	event.Type = schema.AffordanceTypeEvent
	if err := repo.Insert(context.Background(), tenantA, &event); !projecterrors.Type.Eq(err) {
		t.Fatalf("event insert error = %v, want project type error", err)
	}
}

func TestModelModulePropertyNormalizesRecordPolicy(t *testing.T) {
	db := newModelReleaseTestDB(t)
	module := createTestModule(t, db, tenantA, "history-policy")
	repo := NewModelModulePropertyRepo(db)

	insertAndRead := func(identifier string, recordMode schema.RecordMode, legacyIsNoRecord bool) *DmModelModuleProperty {
		t.Helper()
		property := &schema.Property{
			CommonParam: schema.CommonParam{Identifier: identifier, Name: identifier, RecordMode: recordMode},
			Mode:        schema.PropertyModeRW,
			Define:      schema.Define{Type: schema.DataTypeBool},
			IsNoRecord:  legacyIsNoRecord,
		}
		row := ToModelModulePropertyPo(string(tenantA), module.ID, property)
		if err := repo.Insert(context.Background(), tenantA, row); err != nil {
			t.Fatalf("insert %s: %v", identifier, err)
		}
		var got DmModelModuleProperty
		tenantCtx := ctxs.SetUserCtx(context.Background(), &ctxs.UserCtx{TenantCode: string(tenantA)})
		if err := db.WithContext(tenantCtx).First(&got, row.ID).Error; err != nil {
			t.Fatalf("read %s: %v", identifier, err)
		}
		return &got
	}

	legacy := insertAndRead("legacy", 0, true)
	if legacy.RecordMode != schema.RecordModeAll {
		t.Fatalf("legacy RecordMode = %d, want %d", legacy.RecordMode, schema.RecordModeAll)
	}
	var legacyAffordance schema.PropertyAffordance
	if err := json.Unmarshal([]byte(legacy.Affordance), &legacyAffordance); err != nil {
		t.Fatal(err)
	}
	if legacyAffordance.IsNoRecord {
		t.Fatal("legacy IsNoRecord overrode RecordMode=0")
	}

	none := insertAndRead("none", schema.RecordModeNone, false)
	var noneAffordance schema.PropertyAffordance
	if err := json.Unmarshal([]byte(none.Affordance), &noneAffordance); err != nil {
		t.Fatal(err)
	}
	if !noneAffordance.IsNoRecord || !ToModelModulePropertyDo(none).IsNoRecord {
		t.Fatal("RecordModeNone did not derive IsNoRecord=true")
	}

	invalid := ToModelModulePropertyPo(string(tenantA), module.ID, &schema.Property{
		CommonParam: schema.CommonParam{Identifier: "invalid", Name: "invalid", RecordMode: 99},
		Mode:        schema.PropertyModeRW,
		Define:      schema.Define{Type: schema.DataTypeBool},
	})
	if err := repo.Insert(context.Background(), tenantA, invalid); !projecterrors.Parameter.Eq(err) {
		t.Fatalf("invalid RecordMode error = %v, want project parameter error", err)
	}
}

func TestProductModelModuleRejectsCrossTenantAndDuplicateBinding(t *testing.T) {
	db := newModelReleaseTestDB(t)
	createTestProduct(t, db, tenantA, "product-a")
	createTestProduct(t, db, tenantB, "product-b")
	moduleA := createTestModule(t, db, tenantA, "environment")
	moduleB := createTestModule(t, db, tenantB, "environment")
	repo := NewProductModelModuleRepo(db)

	binding := &DmProductModelModule{TenantCode: tenantA, ProductID: "product-a", ModuleID: moduleA.ID}
	if err := repo.Insert(context.Background(), tenantA, binding); err != nil {
		t.Fatalf("insert binding: %v", err)
	}
	if err := repo.Insert(context.Background(), tenantA, &DmProductModelModule{TenantCode: tenantA, ProductID: "product-a", ModuleID: moduleA.ID}); !projecterrors.Duplicate.Eq(err) {
		t.Fatalf("duplicate binding error = %v, want project duplicate error", err)
	}
	if err := repo.Insert(context.Background(), tenantA, &DmProductModelModule{TenantCode: tenantA, ProductID: "product-a", ModuleID: moduleB.ID}); !projecterrors.NotFind.Eq(err) {
		t.Fatalf("cross-tenant module error = %v, want project not-found error", err)
	}
}

func TestProductModelModuleRejectsLastBindingRemoval(t *testing.T) {
	db := newModelReleaseTestDB(t)
	createTestProduct(t, db, tenantA, "product-a")
	first := createTestModule(t, db, tenantA, "environment")
	second := createTestModule(t, db, tenantA, "energy")
	repo := NewProductModelModuleRepo(db)
	ctx := context.Background()

	for _, module := range []*DmModelModule{first, second} {
		if err := repo.Insert(ctx, tenantA, &DmProductModelModule{TenantCode: tenantA, ProductID: "product-a", ModuleID: module.ID}); err != nil {
			t.Fatalf("insert binding: %v", err)
		}
	}
	if err := repo.Delete(ctx, tenantA, "product-a", second.ID); err != nil {
		t.Fatalf("remove non-last binding: %v", err)
	}
	if err := repo.Delete(ctx, tenantA, "product-a", first.ID); !projecterrors.NotEmpty.Eq(err) {
		t.Fatalf("last binding removal error = %v, want project not-empty error", err)
	}
}

func TestProductSchemaRepoExcludesDeviceRows(t *testing.T) {
	db := newModelReleaseTestDB(t)
	rows := []map[string]any{
		{"tenant_code": string(tenantA), "product_id": "product-a", "identifier": "temperature", "device_name": nil, "tag": schema.TagCustom, "type": schema.AffordanceTypeEvent, "name": "Product temperature", "affordance": `{}`},
		{"tenant_code": string(tenantA), "product_id": "product-a", "identifier": "temperature", "device_name": "device-a", "tag": schema.TagDeviceCustom, "type": schema.AffordanceTypeEvent, "name": "Device temperature", "affordance": `{}`},
	}
	for _, row := range rows {
		if err := db.Table((&DmSchemaInfo{}).TableName()).Create(row).Error; err != nil {
			t.Fatalf("seed shared schema table: %v", err)
		}
	}

	ctx := ctxs.SetUserCtx(context.Background(), &ctxs.UserCtx{TenantCode: string(tenantA)})
	got, err := NewProductSchemaRepo(db).FindByFilter(ctx, ProductSchemaFilter{ProductID: "product-a"}, nil)
	if err != nil {
		t.Fatalf("find product schema: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Product temperature" {
		t.Fatalf("product schema rows = %+v, want only product-level row", got)
	}
}

func TestProductModelReleaseIsImmutable(t *testing.T) {
	db := newModelReleaseTestDB(t)
	createTestProduct(t, db, tenantA, "product-a")
	bundle, bundleHash := canonicalReleaseBundle(t, tenantA, "product-a")
	release := &DmProductModelRelease{
		TenantCode:    tenantA,
		ProductID:     "product-a",
		ReleaseNo:     1,
		SchemaVersion: schema.SchemaBundleVersion,
		BundleJSON:    bundle,
		BundleHash:    bundleHash,
	}
	if err := NewProductModelReleaseRepo(db).Insert(context.Background(), tenantA, release); err != nil {
		t.Fatalf("insert release: %v", err)
	}
	if release.Status != ModelReleaseStatusValid {
		t.Fatalf("release status = %q, want %q", release.Status, ModelReleaseStatusValid)
	}

	err := db.Model(&DmProductModelRelease{}).Where("id = ?", release.ID).Updates(map[string]any{
		"bundle_json": []byte(`{"changed":true}`),
		"bundle_hash": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}).Error
	if !projecterrors.Method.Eq(err) {
		t.Fatalf("snapshot update error = %v, want project method error", err)
	}

	got, err := NewProductModelReleaseRepo(db).FindOne(context.Background(), tenantA, release.ID)
	if err != nil {
		t.Fatalf("find release: %v", err)
	}
	if !bytes.Equal(got.BundleJSON, bundle) || got.BundleHash != release.BundleHash {
		t.Fatalf("immutable snapshot changed: %+v", got)
	}
}

func TestProductModelReleaseNumberIsUniquePerProduct(t *testing.T) {
	db := newModelReleaseTestDB(t)
	createTestProduct(t, db, tenantA, "product-a")
	createTestProduct(t, db, tenantA, "product-b")
	repo := NewProductModelReleaseRepo(db)
	newRelease := func(productID string) *DmProductModelRelease {
		bundle, hash := canonicalReleaseBundle(t, tenantA, productID)
		return &DmProductModelRelease{TenantCode: tenantA, ProductID: productID, ReleaseNo: 1, SchemaVersion: schema.SchemaBundleVersion, BundleJSON: bundle, BundleHash: hash}
	}

	if err := repo.Insert(context.Background(), tenantA, newRelease("product-a")); err != nil {
		t.Fatalf("insert first release: %v", err)
	}
	if err := repo.Insert(context.Background(), tenantA, newRelease("product-a")); !projecterrors.Duplicate.Eq(err) {
		t.Fatalf("duplicate release error = %v, want project duplicate error", err)
	}
	if err := repo.Insert(context.Background(), tenantA, newRelease("product-b")); err != nil {
		t.Fatalf("same release number on another product: %v", err)
	}
}
