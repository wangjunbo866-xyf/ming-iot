package relationDB

import "testing"

func TestProductInfoUpdateCannotOverwriteConcurrentReleaseActivation(t *testing.T) {
	db := newProductInfoTask4DB(t)
	ctx := task4Context()
	product := task4Product("product-activation", 0)
	product.ID = 701
	if err := NewProductInfoRepo(db).Insert(ctx, product); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	stale := *product
	if err := db.WithContext(ctx).Model(&DmProductInfo{}).
		Where("tenant_code = ? AND product_id = ?", task4Tenant, product.ProductID).
		Update("current_release_id", int64(77)).Error; err != nil {
		t.Fatalf("activate release: %v", err)
	}

	stale.ProductName = "Updated display name"
	if err := NewProductInfoRepo(db).Update(ctx, &stale); err != nil {
		t.Fatalf("update product metadata: %v", err)
	}
	var currentReleaseID int64
	if err := db.WithContext(ctx).Model(&DmProductInfo{}).
		Select("current_release_id").
		Where("tenant_code = ? AND product_id = ?", task4Tenant, product.ProductID).
		Scan(&currentReleaseID).Error; err != nil {
		t.Fatalf("read current release: %v", err)
	}
	if currentReleaseID != 77 {
		t.Fatalf("current_release_id = %d, want concurrent activation 77", currentReleaseID)
	}
}
