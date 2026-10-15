package relationDB

import (
	"context"
	"strings"

	"gitee.com/ming-iot/saas/share/dataType"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/stores"
	"gorm.io/gorm"
)

type ModelModuleRepo struct{ db *gorm.DB }

type ModelModuleFilter struct {
	IDs             []int64
	ModelModuleCode string
}

const baseModelModuleCodePrefix = "base_"

func BaseModelModuleCode(productID string) string {
	return baseModelModuleCodePrefix + productID
}

func NewModelModuleRepo(in any) *ModelModuleRepo {
	return &ModelModuleRepo{db: stores.GetCommonConn(in)}
}

func (p ModelModuleRepo) scoped(ctx context.Context, tenantCode dataType.TenantCode) (*gorm.DB, error) {
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return nil, err
	}
	return p.db.WithContext(tenantCtx).Where("tenant_code = ?", tenantCode), nil
}

func (p ModelModuleRepo) Insert(ctx context.Context, tenantCode dataType.TenantCode, data *DmModelModule) error {
	if data == nil || data.TenantCode == "" || data.TenantCode != tenantCode || data.ModelModuleCode == "" {
		return projecterrors.Parameter.AddDetail("model module tenant and modelModuleCode must match the explicit tenant")
	}
	if strings.HasPrefix(data.ModelModuleCode, baseModelModuleCodePrefix) {
		return projecterrors.Parameter.AddDetail("base_ is reserved for product base model modules")
	}
	return p.insert(ctx, tenantCode, data)
}

func (p ModelModuleRepo) insertBase(ctx context.Context, tenantCode dataType.TenantCode, productID string, data *DmModelModule) error {
	if data == nil || productID == "" || data.ModelModuleCode != BaseModelModuleCode(productID) {
		return projecterrors.Parameter.AddDetail("invalid base model module code")
	}
	return p.insert(ctx, tenantCode, data)
}

func (p ModelModuleRepo) insert(ctx context.Context, tenantCode dataType.TenantCode, data *DmModelModule) error {
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return err
	}
	return modelReleaseDBError(db.Create(data).Error)
}

func (p ModelModuleRepo) FindOne(ctx context.Context, tenantCode dataType.TenantCode, id int64) (*DmModelModule, error) {
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return nil, err
	}
	var result DmModelModule
	if err := db.Where("id = ?", id).First(&result).Error; err != nil {
		return nil, modelReleaseDBError(err)
	}
	return &result, nil
}

func (p ModelModuleRepo) FindByFilter(ctx context.Context, tenantCode dataType.TenantCode, filter ModelModuleFilter, page *stores.PageInfo) ([]*DmModelModule, error) {
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return nil, err
	}
	if len(filter.IDs) != 0 {
		db = db.Where("id IN ?", filter.IDs)
	}
	if filter.ModelModuleCode != "" {
		db = db.Where("model_module_code = ?", filter.ModelModuleCode)
	}
	var results []*DmModelModule
	if err := page.ToGorm(db).Find(&results).Error; err != nil {
		return nil, modelReleaseDBError(err)
	}
	return results, nil
}

func (p ModelModuleRepo) CountByFilter(ctx context.Context, tenantCode dataType.TenantCode, filter ModelModuleFilter) (int64, error) {
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return 0, err
	}
	if len(filter.IDs) != 0 {
		db = db.Where("id IN ?", filter.IDs)
	}
	if filter.ModelModuleCode != "" {
		db = db.Where("model_module_code = ?", filter.ModelModuleCode)
	}
	var total int64
	if err := db.Model(&DmModelModule{}).Count(&total).Error; err != nil {
		return 0, modelReleaseDBError(err)
	}
	return total, nil
}

func (p ModelModuleRepo) Update(ctx context.Context, tenantCode dataType.TenantCode, data *DmModelModule) error {
	if data == nil || data.ID == 0 || data.TenantCode != tenantCode {
		return projecterrors.Parameter.AddDetail("model module identity must match the explicit tenant")
	}
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return err
	}
	var existing DmModelModule
	if err := db.Where("id = ?", data.ID).First(&existing).Error; err != nil {
		return modelReleaseDBError(err)
	}
	if strings.HasPrefix(existing.ModelModuleCode, baseModelModuleCodePrefix) {
		if data.ModelModuleCode != existing.ModelModuleCode {
			return projecterrors.NotEmpty.AddDetail("base model module code is immutable")
		}
	} else if strings.HasPrefix(data.ModelModuleCode, baseModelModuleCodePrefix) {
		return projecterrors.Parameter.AddDetail("base_ is reserved for product base model modules")
	}
	result := db.Model(&DmModelModule{}).Where("id = ?", data.ID).Updates(map[string]any{"model_module_code": data.ModelModuleCode, "name": data.Name, "description": data.Description})
	if result.Error != nil {
		return modelReleaseDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return projecterrors.NotFind.WithStack(1)
	}
	return nil
}

func (p ModelModuleRepo) Delete(ctx context.Context, tenantCode dataType.TenantCode, id int64) error {
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return err
	}
	var module DmModelModule
	if err := db.Where("id = ?", id).First(&module).Error; err != nil {
		return modelReleaseDBError(err)
	}
	if strings.HasPrefix(module.ModelModuleCode, baseModelModuleCodePrefix) {
		ownerProductID := strings.TrimPrefix(module.ModelModuleCode, baseModelModuleCodePrefix)
		var ownerCount int64
		if err := db.Session(&gorm.Session{NewDB: true}).Table((&DmProductInfo{}).TableName()).Where("tenant_code = ? AND product_id = ? AND deleted_time = 0", tenantCode, ownerProductID).Count(&ownerCount).Error; err != nil {
			return modelReleaseDBError(err)
		}
		if ownerCount != 0 {
			return projecterrors.NotEmpty.AddDetail("base model module cannot be deleted while its product exists")
		}
	}
	var bindingCount int64
	if err := db.Session(&gorm.Session{NewDB: true}).Table((&DmProductModelModule{}).TableName()).
		Where("tenant_code = ? AND module_id = ? AND deleted_time = 0", tenantCode, id).
		Count(&bindingCount).Error; err != nil {
		return modelReleaseDBError(err)
	}
	if bindingCount != 0 {
		return projecterrors.NotEmpty.AddDetail("model module cannot be deleted while bound to a product")
	}
	result := db.Session(&gorm.Session{NewDB: true}).Where("tenant_code = ? AND id = ?", tenantCode, id).Delete(&DmModelModule{})
	if result.Error != nil {
		return modelReleaseDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return projecterrors.NotFind.WithStack(1)
	}
	return nil
}
