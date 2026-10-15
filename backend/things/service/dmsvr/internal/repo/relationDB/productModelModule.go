package relationDB

import (
	"context"
	"strings"

	"gitee.com/ming-iot/saas/share/dataType"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/stores"
	"gitee.com/ming-iot/things/service/dmsvr/internal/domain/modelrelease"
	"gitee.com/ming-iot/things/share/domain/schema"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProductModelModuleRepo struct{ db *gorm.DB }

type ProductModelModuleFilter struct {
	ProductID string
	ModuleID  int64
}

func NewProductModelModuleRepo(in any) *ProductModelModuleRepo {
	return &ProductModelModuleRepo{db: stores.GetCommonConn(in)}
}

func (p ProductModelModuleRepo) scoped(ctx context.Context, tenantCode dataType.TenantCode) (*gorm.DB, error) {
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return nil, err
	}
	return p.db.WithContext(tenantCtx).Where("tenant_code = ?", tenantCode), nil
}

func (p ProductModelModuleRepo) Insert(ctx context.Context, tenantCode dataType.TenantCode, data *DmProductModelModule) error {
	if data == nil || data.TenantCode != tenantCode || data.ProductID == "" || data.ModuleID == 0 {
		return projecterrors.Parameter.AddDetail("product-module binding must match the explicit tenant")
	}
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return err
	}
	return p.db.WithContext(tenantCtx).Transaction(func(tx *gorm.DB) error {
		if err := requireTenantProductAndModule(tx, tenantCode, data.ProductID, data.ModuleID); err != nil {
			return err
		}
		var bindingCount int64
		if err := tx.Model(&DmProductModelModule{}).
			Where("tenant_code = ? AND product_id = ? AND deleted_time = 0", tenantCode, data.ProductID).
			Count(&bindingCount).Error; err != nil {
			return modelReleaseDBError(err)
		}
		if bindingCount == 0 {
			var legacyPropertyCount int64
			if err := tx.Model(&DmSchemaInfo{}).
				Where("tenant_code = ? AND product_id = ? AND device_name IS NULL AND type = ? AND deleted_time = 0", tenantCode, data.ProductID, schema.AffordanceTypeProperty).
				Count(&legacyPropertyCount).Error; err != nil {
				return modelReleaseDBError(err)
			}
			if legacyPropertyCount != 0 {
				return modelrelease.ErrMigrationRequired
			}
		}
		var module DmModelModule
		if err := tx.Where("tenant_code = ? AND id = ?", tenantCode, data.ModuleID).First(&module).Error; err != nil {
			return modelReleaseDBError(err)
		}
		if strings.HasPrefix(module.ModelModuleCode, baseModelModuleCodePrefix) && module.ModelModuleCode != BaseModelModuleCode(data.ProductID) {
			return projecterrors.Parameter.AddDetail("base model module can only bind its owning product")
		}
		return modelReleaseDBError(tx.Create(data).Error)
	})
}

func (p ProductModelModuleRepo) FindByFilter(ctx context.Context, tenantCode dataType.TenantCode, filter ProductModelModuleFilter, page *stores.PageInfo) ([]*DmProductModelModule, error) {
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return nil, err
	}
	if filter.ProductID != "" {
		db = db.Where("product_id = ?", filter.ProductID)
	}
	if filter.ModuleID != 0 {
		db = db.Where("module_id = ?", filter.ModuleID)
	}
	var results []*DmProductModelModule
	if err := page.ToGorm(db.Order("\"order\", id")).Find(&results).Error; err != nil {
		return nil, modelReleaseDBError(err)
	}
	return results, nil
}

func (p ProductModelModuleRepo) CountByFilter(ctx context.Context, tenantCode dataType.TenantCode, filter ProductModelModuleFilter) (int64, error) {
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return 0, err
	}
	if filter.ProductID != "" {
		db = db.Where("product_id = ?", filter.ProductID)
	}
	if filter.ModuleID != 0 {
		db = db.Where("module_id = ?", filter.ModuleID)
	}
	var total int64
	if err := db.Model(&DmProductModelModule{}).Count(&total).Error; err != nil {
		return 0, modelReleaseDBError(err)
	}
	return total, nil
}

func (p ProductModelModuleRepo) Delete(ctx context.Context, tenantCode dataType.TenantCode, productID string, moduleID int64) error {
	if tenantCode == "" || productID == "" || moduleID == 0 {
		return projecterrors.Parameter.AddDetail("tenant, product and module are required")
	}
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return err
	}
	return p.db.WithContext(tenantCtx).Transaction(func(tx *gorm.DB) error {
		var module DmModelModule
		if err := tx.Where("tenant_code = ? AND id = ?", tenantCode, moduleID).First(&module).Error; err != nil {
			return modelReleaseDBError(err)
		}
		if module.ModelModuleCode == BaseModelModuleCode(productID) {
			var productCount int64
			if err := tx.Table((&DmProductInfo{}).TableName()).Where("tenant_code = ? AND product_id = ? AND deleted_time = 0", tenantCode, productID).Count(&productCount).Error; err != nil {
				return modelReleaseDBError(err)
			}
			if productCount != 0 {
				return projecterrors.NotEmpty.AddDetail("base model module cannot be unbound while its product exists")
			}
		}
		var bindings []DmProductModelModule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_code = ? AND product_id = ?", tenantCode, productID).Find(&bindings).Error; err != nil {
			return modelReleaseDBError(err)
		}
		if len(bindings) <= 1 {
			return projecterrors.NotEmpty.AddDetail("a product must retain at least one model module binding")
		}
		result := tx.Where("tenant_code = ? AND product_id = ? AND module_id = ?", tenantCode, productID, moduleID).Delete(&DmProductModelModule{})
		if result.Error != nil {
			return modelReleaseDBError(result.Error)
		}
		if result.RowsAffected == 0 {
			return projecterrors.NotFind.WithStack(1)
		}
		return nil
	})
}

func requireTenantProductAndModule(db *gorm.DB, tenantCode dataType.TenantCode, productID string, moduleID int64) error {
	var productCount int64
	if err := db.Table((&DmProductInfo{}).TableName()).Where("tenant_code = ? AND product_id = ? AND deleted_time = 0", tenantCode, productID).Count(&productCount).Error; err != nil {
		return modelReleaseDBError(err)
	}
	var moduleCount int64
	if err := db.Table((&DmModelModule{}).TableName()).Where("tenant_code = ? AND id = ? AND deleted_time = 0", tenantCode, moduleID).Count(&moduleCount).Error; err != nil {
		return modelReleaseDBError(err)
	}
	if productCount != 1 || moduleCount != 1 {
		return projecterrors.NotFind.AddDetail("product or model module not found in tenant")
	}
	return nil
}
