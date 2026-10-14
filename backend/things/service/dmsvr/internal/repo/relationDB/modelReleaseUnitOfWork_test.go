package relationDB

import (
	"errors"
	"testing"

	"gitee.com/ming-iot/share/def"
	"gitee.com/ming-iot/things/service/dmsvr/internal/domain/modelrelease"
	"gitee.com/ming-iot/things/share/domain/schema"
	"gorm.io/gorm"
)

func newModelReleaseServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newProductInfoTask4DB(t)
	if err := db.AutoMigrate(&DmProductModelRelease{}); err != nil {
		t.Fatalf("migrate model release: %v", err)
	}
	return db
}

func TestModelReleaseServicePublishesAndExplicitlyActivatesRepositoryDraft(t *testing.T) {
	db := newModelReleaseServiceTestDB(t)
	ctx := task4Context()
	productID := "product-release-service"
	product := task4Product(productID, def.NotClassified)
	if err := NewProductInfoRepo(db).Insert(ctx, product); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	if err := NewProductPropertyDraftRepo(db).Insert(ctx, task4Tenant, task5PropertyDraft(productID, "enabled")); err != nil {
		t.Fatalf("insert Property: %v", err)
	}
	event := &DmProductSchema{
		TenantCode: product.TenantCode,
		ProductID:  productID,
		Identifier: "alarm",
		DmSchemaCore: ToEventPo(&schema.Event{
			CommonParam: schema.CommonParam{Identifier: "alarm", Name: "Alarm"},
			Type:        schema.EventTypeAlert,
			Params:      schema.Params{},
		}),
	}
	if err := NewProductSchemaRepo(db).Insert(ctx, event); err != nil {
		t.Fatalf("insert Event: %v", err)
	}

	service := modelrelease.NewService(NewModelReleaseUnitOfWork(db))
	ref := modelrelease.ProductRef{TenantCode: string(task4Tenant), ProductID: productID}
	published, err := service.Publish(ctx, ref, 17)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if published.ReleaseNo != 1 || len(published.Bundle.Model.Properties) != 1 || len(published.Bundle.Model.Events) != 1 {
		t.Fatalf("published release = %+v", published)
	}
	var before DmProductInfo
	if err := db.WithContext(ctx).Where("tenant_code = ? AND product_id = ?", task4Tenant, productID).First(&before).Error; err != nil {
		t.Fatal(err)
	}
	if before.CurrentReleaseID != 0 {
		t.Fatalf("publish activated release implicitly: current=%d", before.CurrentReleaseID)
	}
	if _, err := service.ReadActive(ctx, ref); !errors.Is(err, modelrelease.ErrNoActiveRelease) {
		t.Fatalf("read inactive product error = %v, want no active release", err)
	}
	if err := service.Activate(ctx, modelrelease.ReleaseRef{ProductRef: ref, ReleaseID: published.ID}); err != nil {
		t.Fatalf("activate: %v", err)
	}
	active, err := service.ReadActive(ctx, ref)
	if err != nil || active.ID != published.ID {
		t.Fatalf("active release = %+v, err=%v", active, err)
	}
}

func TestModelReleaseServiceRejectsUnmigratedRepositoryProduct(t *testing.T) {
	db := newModelReleaseServiceTestDB(t)
	ctx := task4Context()
	productID := "product-unmigrated"
	product := task4Product(productID, def.NotClassified)
	if err := db.WithContext(ctx).Create(product).Error; err != nil {
		t.Fatalf("seed unmigrated product: %v", err)
	}
	service := modelrelease.NewService(NewModelReleaseUnitOfWork(db))
	_, err := service.Publish(ctx, modelrelease.ProductRef{TenantCode: string(task4Tenant), ProductID: productID}, 17)
	if !errors.Is(err, modelrelease.ErrMigrationRequired) {
		t.Fatalf("publish error = %v, want migration required", err)
	}
}

func TestModelReleaseServiceRejectsTamperedRepositoryRelease(t *testing.T) {
	db := newModelReleaseServiceTestDB(t)
	ctx := task4Context()
	productID := "product-tampered-release"
	if err := NewProductInfoRepo(db).Insert(ctx, task4Product(productID, def.NotClassified)); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	service := modelrelease.NewService(NewModelReleaseUnitOfWork(db))
	ref := modelrelease.ProductRef{TenantCode: string(task4Tenant), ProductID: productID}
	published, err := service.Publish(ctx, ref, 17)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	badHash := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if err := db.Exec("UPDATE dm_product_release SET bundle_hash = ? WHERE id = ?", badHash, published.ID).Error; err != nil {
		t.Fatalf("tamper release: %v", err)
	}
	err = service.Activate(ctx, modelrelease.ReleaseRef{ProductRef: ref, ReleaseID: published.ID})
	if !errors.Is(err, modelrelease.ErrInvalidRelease) {
		t.Fatalf("activate error = %v, want invalid release", err)
	}
}
