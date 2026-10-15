//go:build integration

package relationDB

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/def"
	"gitee.com/ming-iot/things/service/dmsvr/internal/domain/modelrelease"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openModelReleasePostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MING_IOT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("MING_IOT_TEST_POSTGRES_DSN is required for the integration contract")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal("open PostgreSQL test database failed (DSN redacted)")
	}
	return db
}

func TestModelReleasePostgresContract(t *testing.T) {
	db := openModelReleasePostgres(t)
	if err := db.WithContext(context.Background()).AutoMigrate(
		&DmProductInfo{},
		&DmModelModule{},
		&DmModelModuleProperty{},
		&DmProductModelModule{},
		&DmProductModelRelease{},
	); err != nil {
		t.Fatalf("migrate PostgreSQL contracts: %v", err)
	}

	for _, table := range []string{"dm_module_info", "dm_module_schema_info", "dm_product_module", "dm_product_release"} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("missing table %s", table)
		}
	}

	type columnContract struct {
		Table, Column, DataType, DefaultContains string
	}
	contracts := []columnContract{
		{Table: "dm_product_info", Column: "current_release_id", DataType: "bigint", DefaultContains: "0"},
		{Table: "dm_module_info", Column: "model_module_code", DataType: "character varying"},
		{Table: "dm_product_release", Column: "bundle_json", DataType: "jsonb"},
		{Table: "dm_product_release", Column: "bundle_hash", DataType: "character varying"},
		{Table: "dm_product_release", Column: "status", DataType: "character varying", DefaultContains: "valid"},
	}
	for _, contract := range contracts {
		var dataType, defaultValue string
		err := db.Raw(`SELECT data_type, COALESCE(column_default, '') FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`, contract.Table, contract.Column).Row().Scan(&dataType, &defaultValue)
		if err != nil {
			t.Errorf("inspect %s.%s: %v", contract.Table, contract.Column, err)
			continue
		}
		if dataType != contract.DataType {
			t.Errorf("%s.%s type = %q, want %q", contract.Table, contract.Column, dataType, contract.DataType)
		}
		if contract.DefaultContains != "" && !strings.Contains(defaultValue, contract.DefaultContains) {
			t.Errorf("%s.%s default = %q, want it to contain %q", contract.Table, contract.Column, defaultValue, contract.DefaultContains)
		}
	}

	expectedIndexes := map[string][]string{
		"dm_module_info":        {"tenant_code", "model_module_code", "deleted_time"},
		"dm_module_schema_info": {"tenant_code", "module_id", "identifier", "deleted_time"},
		"dm_product_module":     {"tenant_code", "product_id", "module_id", "deleted_time"},
		"dm_product_release":    {"tenant_code", "product_id", "release_no"},
	}
	for table, columns := range expectedIndexes {
		var definitions []string
		if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND tablename = ?`, table).Scan(&definitions).Error; err != nil {
			t.Errorf("inspect indexes for %s: %v", table, err)
			continue
		}
		found := false
		for _, definition := range definitions {
			if !strings.Contains(definition, "UNIQUE") {
				continue
			}
			found = true
			for _, column := range columns {
				if !strings.Contains(definition, column) {
					found = false
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("missing unique index on %s(%s)", table, strings.Join(columns, ", "))
		}
	}
}

func TestModuleCodePostgresContract(t *testing.T) {
	db := openModelReleasePostgres(t)
	if err := db.WithContext(context.Background()).AutoMigrate(
		&DmDeviceModuleVersion{},
		&DmOtaFirmwareInfo{},
		&DmModelModule{},
	); err != nil {
		t.Fatalf("migrate module code PostgreSQL contracts: %v", err)
	}

	type tableContract struct {
		table         string
		newColumn     string
		forbidden     []string
		uniqueColumns []string
	}
	contracts := []tableContract{
		{
			table:         "dm_device_module_version",
			newColumn:     "firmware_module_code",
			forbidden:     []string{"module_code", "model_module_code"},
			uniqueColumns: []string{"product_id", "device_name", "firmware_module_code", "version"},
		},
		{
			table:         "dm_ota_firmware_info",
			newColumn:     "firmware_module_code",
			forbidden:     []string{"module_code", "model_module_code"},
			uniqueColumns: []string{"product_id", "version", "deleted_time"},
		},
		{
			table:         "dm_module_info",
			newColumn:     "model_module_code",
			forbidden:     []string{"module_code", "firmware_module_code"},
			uniqueColumns: []string{"tenant_code", "model_module_code", "deleted_time"},
		},
	}

	for _, contract := range contracts {
		t.Run(contract.table, func(t *testing.T) {
			var dataType string
			err := db.Raw(`SELECT data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`, contract.table, contract.newColumn).Row().Scan(&dataType)
			if err != nil {
				t.Fatalf("inspect %s.%s: %v", contract.table, contract.newColumn, err)
			}
			if dataType != "character varying" {
				t.Errorf("%s.%s type = %q, want character varying", contract.table, contract.newColumn, dataType)
			}
			for _, column := range contract.forbidden {
				var count int64
				if err := db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`, contract.table, column).Scan(&count).Error; err != nil {
					t.Fatalf("inspect forbidden column %s.%s: %v", contract.table, column, err)
				}
				if count != 0 {
					t.Errorf("forbidden column %s.%s still exists", contract.table, column)
				}
			}

			var definitions []string
			if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND tablename = ?`, contract.table).Scan(&definitions).Error; err != nil {
				t.Fatalf("inspect indexes for %s: %v", contract.table, err)
			}
			found := false
			for _, definition := range definitions {
				if !strings.Contains(definition, "UNIQUE") {
					continue
				}
				found = true
				for _, column := range contract.uniqueColumns {
					if !strings.Contains(definition, column) {
						found = false
						break
					}
				}
				if found {
					break
				}
			}
			if !found {
				t.Errorf("missing unique index on %s(%s)", contract.table, strings.Join(contract.uniqueColumns, ", "))
			}
		})
	}
}

func TestProductInsertPostgresAtomicity(t *testing.T) {
	db := openModelReleasePostgres(t)
	if err := db.AutoMigrate(&DmProductInfo{}, &DmModelModule{}, &DmModelModuleProperty{}, &DmProductModelModule{}); err != nil {
		t.Fatalf("migrate PostgreSQL Task 4 contracts: %v", err)
	}
	tenantCode := dataType.TenantCode("task4-integration")
	productID := "t4" + strconv.FormatInt(time.Now().UnixNano(), 36)
	ctx := ctxs.SetUserCtx(context.Background(), &ctxs.UserCtx{TenantCode: string(tenantCode)})
	newProduct := func() *DmProductInfo {
		return &DmProductInfo{
			TenantCode:  dataType.TenantCodeWithDefaultR(tenantCode),
			ProductID:   productID,
			ProductName: productID,
			CategoryID:  def.NotClassified,
		}
	}
	assertNoRows := func() {
		t.Helper()
		checks := []struct {
			table, where string
			args         []any
		}{
			{(&DmProductInfo{}).TableName(), "tenant_code = ? AND product_id = ? AND deleted_time = 0", []any{tenantCode, productID}},
			{(&DmModelModule{}).TableName(), "tenant_code = ? AND model_module_code = ? AND deleted_time = 0", []any{tenantCode, BaseModelModuleCode(productID)}},
			{(&DmProductModelModule{}).TableName(), "tenant_code = ? AND product_id = ? AND deleted_time = 0", []any{tenantCode, productID}},
		}
		for _, check := range checks {
			var count int64
			if err := db.Table(check.table).Where(check.where, check.args...).Count(&count).Error; err != nil {
				t.Fatalf("inspect %s after rollback: %v", check.table, err)
			}
			if count != 0 {
				t.Fatalf("%s has %d active rows after rollback", check.table, count)
			}
		}
	}

	rollback := errors.New("intentional outer rollback")
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := NewProductInfoRepo(tx).Insert(ctx, newProduct()); err != nil {
			return err
		}
		var bindings int64
		if err := tx.Table((&DmProductModelModule{}).TableName()).Where("tenant_code = ? AND product_id = ? AND deleted_time = 0", tenantCode, productID).Count(&bindings).Error; err != nil {
			return err
		}
		if bindings != 1 {
			t.Fatalf("bindings inside PostgreSQL transaction = %d, want 1", bindings)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("outer transaction error = %v, want intentional rollback", err)
	}
	assertNoRows()

	callbackName := "integration:fail-task4-binding"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == (&DmProductModelModule{}).TableName() {
			tx.AddError(errors.New("injected PostgreSQL binding failure"))
		}
	}); err != nil {
		t.Fatalf("register PostgreSQL failure injection: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })
	if err := NewProductInfoRepo(db).Insert(ctx, newProduct()); err == nil {
		t.Fatal("PostgreSQL insert succeeded despite injected binding failure")
	}
	assertNoRows()
}

func TestModelReleasePostgresConcurrentPublishAndExplicitActivate(t *testing.T) {
	db := openModelReleasePostgres(t)
	if err := db.AutoMigrate(&DmProductInfo{}, &DmSchemaInfo{}, &DmModelModule{}, &DmModelModuleProperty{}, &DmProductModelModule{}, &DmProductModelRelease{}); err != nil {
		t.Fatalf("migrate PostgreSQL release service contracts: %v", err)
	}
	tenantCode := dataType.TenantCode("release-integration")
	productID := "release" + strconv.FormatInt(time.Now().UnixNano(), 36)
	ctx := ctxs.SetUserCtx(context.Background(), &ctxs.UserCtx{TenantCode: string(tenantCode)})
	product := &DmProductInfo{
		TenantCode:  dataType.TenantCodeWithDefaultR(tenantCode),
		ProductID:   productID,
		ProductName: productID,
		CategoryID:  def.NotClassified,
	}
	if err := NewProductInfoRepo(db).Insert(ctx, product); err != nil {
		t.Fatalf("insert PostgreSQL product: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM dm_product_release WHERE tenant_code = ? AND product_id = ?", tenantCode, productID)
		db.Exec("DELETE FROM dm_product_module WHERE tenant_code = ? AND product_id = ?", tenantCode, productID)
		db.Exec("DELETE FROM dm_module_schema_info WHERE tenant_code = ?", tenantCode)
		db.Exec("DELETE FROM dm_module_info WHERE tenant_code = ?", tenantCode)
		db.Exec("DELETE FROM dm_product_info WHERE tenant_code = ? AND product_id = ?", tenantCode, productID)
	})

	service := modelrelease.NewService(NewModelReleaseUnitOfWork(db))
	ref := modelrelease.ProductRef{TenantCode: string(tenantCode), ProductID: productID}
	type result struct {
		release modelrelease.Release
		err     error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(createdBy int64) {
			defer wg.Done()
			<-start
			release, err := service.Publish(ctx, ref, createdBy)
			results <- result{release: release, err: err}
		}(int64(i + 1))
	}
	close(start)
	wg.Wait()
	close(results)
	releaseNos := map[int64]bool{}
	var first modelrelease.Release
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent publish: %v", result.err)
		}
		releaseNos[result.release.ReleaseNo] = true
		if first.ID == 0 {
			first = result.release
		}
	}
	if !releaseNos[1] || !releaseNos[2] || len(releaseNos) != 2 {
		t.Fatalf("concurrent release numbers = %v, want {1,2}", releaseNos)
	}
	if _, err := service.ReadActive(ctx, ref); !errors.Is(err, modelrelease.ErrNoActiveRelease) {
		t.Fatalf("publish activated a release implicitly: %v", err)
	}
	if err := service.Activate(ctx, modelrelease.ReleaseRef{ProductRef: ref, ReleaseID: first.ID}); err != nil {
		t.Fatalf("activate exact release: %v", err)
	}
	active, err := service.ReadActive(ctx, ref)
	if err != nil || active.ID != first.ID {
		t.Fatalf("active release = %+v, err=%v", active, err)
	}
}
