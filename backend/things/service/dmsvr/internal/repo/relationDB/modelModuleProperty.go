package relationDB

import (
	"context"

	"gitee.com/ming-iot/saas/share/dataType"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/stores"
	"gitee.com/ming-iot/things/share/domain/schema"
	"gorm.io/gorm"
)

type ModelModulePropertyRepo struct{ db *gorm.DB }

type ModelModulePropertyFilter struct {
	ModuleID    int64
	Identifiers []string
}

func NewModelModulePropertyRepo(in any) *ModelModulePropertyRepo {
	return &ModelModulePropertyRepo{db: stores.GetCommonConn(in)}
}

func (p ModelModulePropertyRepo) scoped(ctx context.Context, tenantCode dataType.TenantCode) (*gorm.DB, error) {
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return nil, err
	}
	return p.db.WithContext(tenantCtx).Where("tenant_code = ?", tenantCode), nil
}

func (p ModelModulePropertyRepo) Insert(ctx context.Context, tenantCode dataType.TenantCode, data *DmModelModuleProperty) error {
	if data == nil || data.TenantCode != tenantCode || data.ModuleID == 0 || data.Identifier == "" {
		return projecterrors.Parameter.AddDetail("module property identity must match the explicit tenant")
	}
	if data.Type != schema.AffordanceTypeProperty {
		return projecterrors.Type.AddDetail("model module schema only accepts Property")
	}
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return err
	}
	var moduleCount int64
	if err := db.Table((&DmModelModule{}).TableName()).Where("id = ? AND deleted_time = 0", data.ModuleID).Count(&moduleCount).Error; err != nil {
		return modelReleaseDBError(err)
	}
	if moduleCount != 1 {
		return projecterrors.NotFind.AddDetail("model module not found in tenant")
	}
	return modelReleaseDBError(db.Session(&gorm.Session{NewDB: true}).Create(data).Error)
}

func (p ModelModulePropertyRepo) FindByFilter(ctx context.Context, tenantCode dataType.TenantCode, filter ModelModulePropertyFilter, page *stores.PageInfo) ([]*DmModelModuleProperty, error) {
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return nil, err
	}
	if filter.ModuleID != 0 {
		db = db.Where("module_id = ?", filter.ModuleID)
	}
	if len(filter.Identifiers) != 0 {
		db = db.Where("identifier IN ?", filter.Identifiers)
	}
	var results []*DmModelModuleProperty
	if err := page.ToGorm(db).Find(&results).Error; err != nil {
		return nil, modelReleaseDBError(err)
	}
	return results, nil
}

func (p ModelModulePropertyRepo) CountByFilter(ctx context.Context, tenantCode dataType.TenantCode, filter ModelModulePropertyFilter) (int64, error) {
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return 0, err
	}
	if filter.ModuleID != 0 {
		db = db.Where("module_id = ?", filter.ModuleID)
	}
	if len(filter.Identifiers) != 0 {
		db = db.Where("identifier IN ?", filter.Identifiers)
	}
	var total int64
	if err := db.Model(&DmModelModuleProperty{}).Count(&total).Error; err != nil {
		return 0, modelReleaseDBError(err)
	}
	return total, nil
}

func (p ModelModulePropertyRepo) Update(ctx context.Context, tenantCode dataType.TenantCode, data *DmModelModuleProperty) error {
	if data == nil || data.ID == 0 || data.TenantCode != tenantCode || data.ModuleID == 0 || data.Identifier == "" {
		return projecterrors.Parameter.AddDetail("module property identity must match the explicit tenant")
	}
	if data.Type != schema.AffordanceTypeProperty {
		return projecterrors.Type.AddDetail("model module schema only accepts Property")
	}
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return err
	}
	var existing DmModelModuleProperty
	if err := db.Where("id = ?", data.ID).Take(&existing).Error; err != nil {
		return modelReleaseDBError(err)
	}
	if existing.ModuleID != data.ModuleID || existing.Identifier != data.Identifier {
		return projecterrors.Parameter.AddDetail("module property module and identifier are immutable")
	}
	if existing.Type != schema.AffordanceTypeProperty {
		return projecterrors.Type.AddDetail("model module schema only accepts Property")
	}
	if err := data.BeforeSave(nil); err != nil {
		return err
	}
	updateDB, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return err
	}
	result := updateDB.Session(&gorm.Session{SkipHooks: true}).Model(&DmModelModuleProperty{}).
		Where("id = ? AND module_id = ?", data.ID, data.ModuleID).
		Updates(map[string]any{
			"tag":                  data.Tag,
			"extend_config":        data.ExtendConfig,
			"required":             data.Required,
			"name":                 data.Name,
			"desc":                 data.Desc,
			"is_can_scene_linkage": data.IsCanSceneLinkage,
			"func_group":           data.FuncGroup,
			"control_mode":         data.ControlMode,
			"user_auth":            data.UserPerm,
			"record_mode":          data.RecordMode,
			"is_password":          data.IsPassword,
			"affordance":           data.Affordance,
			"order":                data.Order,
		})
	if result.Error != nil {
		return modelReleaseDBError(result.Error)
	}
	return nil
}

func (p ModelModulePropertyRepo) Delete(ctx context.Context, tenantCode dataType.TenantCode, moduleID, id int64) error {
	db, err := p.scoped(ctx, tenantCode)
	if err != nil {
		return err
	}
	result := db.Where("id = ? AND module_id = ?", id, moduleID).Delete(&DmModelModuleProperty{})
	if result.Error != nil {
		return modelReleaseDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return projecterrors.NotFind.WithStack(1)
	}
	return nil
}
