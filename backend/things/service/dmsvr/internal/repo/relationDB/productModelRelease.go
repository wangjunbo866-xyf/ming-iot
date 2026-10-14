package relationDB

import (
	"context"

	"gitee.com/ming-iot/saas/share/dataType"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/stores"
	"gorm.io/gorm"
)

type ProductModelReleaseRepo struct{ db *gorm.DB }

func NewProductModelReleaseRepo(in any) *ProductModelReleaseRepo {
	return &ProductModelReleaseRepo{db: stores.GetCommonConn(in)}
}

func (p ProductModelReleaseRepo) scoped(ctx context.Context, tenantCode dataType.TenantCode) (*gorm.DB, error) {
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return nil, err
	}
	return p.db.WithContext(tenantCtx).Where("tenant_code = ?", tenantCode), nil
}

func (p ProductModelReleaseRepo) Insert(ctx context.Context, tenantCode dataType.TenantCode, data *DmProductModelRelease) error {
	if data == nil || data.TenantCode != tenantCode {
		return projecterrors.Parameter.AddDetail("model release must match the explicit tenant")
	}
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return err
	}
	var productCount int64
	if err := db.Table((&DmProductInfo{}).TableName()).Where("tenant_code = ? AND product_id = ? AND deleted_time = 0", tenantCode, data.ProductID).Count(&productCount).Error; err != nil {
		return modelReleaseDBError(err)
	}
	if productCount != 1 {
		return projecterrors.NotFind.AddDetail("product not found in tenant")
	}
	return modelReleaseDBError(db.Session(&gorm.Session{NewDB: true}).Create(data).Error)
}

func (p ProductModelReleaseRepo) FindOne(ctx context.Context, tenantCode dataType.TenantCode, id int64) (*DmProductModelRelease, error) {
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return nil, err
	}
	var result DmProductModelRelease
	if err := db.Where("id = ?", id).First(&result).Error; err != nil {
		return nil, modelReleaseDBError(err)
	}
	return &result, nil
}

func (p ProductModelReleaseRepo) FindByProduct(ctx context.Context, tenantCode dataType.TenantCode, productID string, page *stores.PageInfo) ([]*DmProductModelRelease, error) {
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return nil, err
	}
	if productID == "" {
		return nil, projecterrors.Parameter.AddDetail("productID is required")
	}
	var results []*DmProductModelRelease
	if err := page.ToGorm(db.Where("product_id = ?", productID).Order("release_no DESC")).Find(&results).Error; err != nil {
		return nil, modelReleaseDBError(err)
	}
	return results, nil
}

func (p ProductModelReleaseRepo) CountByProduct(ctx context.Context, tenantCode dataType.TenantCode, productID string) (int64, error) {
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return 0, err
	}
	if productID == "" {
		return 0, projecterrors.Parameter.AddDetail("productID is required")
	}
	var total int64
	if err := db.Model(&DmProductModelRelease{}).Where("product_id = ?", productID).Count(&total).Error; err != nil {
		return 0, modelReleaseDBError(err)
	}
	return total, nil
}
