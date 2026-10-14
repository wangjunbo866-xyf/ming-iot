package relationDB

import (
	"context"
	"fmt"

	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/def"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/stores"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/share/devices"
	"gitee.com/ming-iot/things/share/domain/schema"
	"gorm.io/gorm"
)

type ProductInfoRepo struct {
	db *gorm.DB
}

type ProductFilter struct {
	TenantCode    string
	DeviceType    int64
	DeviceTypes   []int64
	ProductName   string
	ProductIDs    []string
	ProductNames  []string
	Tags          map[string]string
	ProtocolConf  map[string]string
	WithProtocol  bool
	WithCategory  bool
	WithConfig    bool
	ProtocolCode  string
	ProtocolType  string
	ProtocolTrans string
	CategoryIDs   []int64
	SceneMode     string
	SceneModes    []string
	Status        devices.ProductStatus
	Statuses      []devices.ProductStatus
	AreaID        int64
	AreaIDPath    string
	NetType       int64
}

func NewProductInfoRepo(in any) *ProductInfoRepo {
	return &ProductInfoRepo{db: stores.GetCommonConn(in)}
}

func (p ProductInfoRepo) fmtFilter(ctx context.Context, f ProductFilter) *gorm.DB {
	db := p.db.WithContext(ctx)
	if f.TenantCode != "" {
		db = db.Where("tenant_code = ?", f.TenantCode)
	}
	if f.DeviceType != 0 {
		db = db.Where("device_type=?", f.DeviceType)
	}
	if len(f.DeviceTypes) != 0 {
		db = db.Where("device_type in ?", f.DeviceTypes)
	}
	if f.AreaID != 0 {
		subQuery := p.db.Model(&DmDeviceInfo{}).Distinct("product_id").Select("product_id").Where("area_id=?", f.AreaID)
		db = db.Where("product_id in (?)", subQuery)
	}
	if f.WithConfig {
		db = db.Preload("Config")
	}
	if f.AreaIDPath != "" {
		subQuery := p.db.Model(&DmDeviceInfo{}).Distinct("product_id").Select("product_id").Where("area_id_path like ?", f.AreaIDPath+"%")
		db = db.Where("product_id in (?)", subQuery)
	}
	if len(f.CategoryIDs) != 0 {
		db = db.Where("category_id in ?", f.CategoryIDs)
	}

	if f.ProtocolCode != "" {
		db = db.Where("protocol_code=? or sub_protocol_code=?", f.ProtocolCode, f.ProtocolCode)
	}
	if f.ProtocolType != "" || f.ProtocolTrans != "" {
		subQuery := p.db.Model(&DmProtocolInfo{}).Select("code")
		if f.ProtocolType != "" {
			subQuery = subQuery.Where(
				fmt.Sprintf("%s = ?", stores.Col("type")), f.ProtocolType)
		}
		if f.ProtocolTrans != "" {
			subQuery = subQuery.Where(
				fmt.Sprintf("%s = ?", stores.Col("trans_protocol")), f.ProtocolTrans)
		}
		db = db.Where("protocol_code in (?) or sub_protocol_code in (?)", subQuery, subQuery)
	}
	if f.SceneMode != "" {
		db = db.Where("scene_mode=?", f.SceneMode)
	}
	if len(f.SceneModes) != 0 {
		db = db.Where("scene_mode in ?", f.SceneModes)
	}
	if f.Status != 0 {
		db = db.Where("status=?", f.Status)
	}
	if len(f.Statuses) != 0 {
		db = db.Where("status in ?", f.Statuses)
	}
	if f.WithProtocol {
		db = db.Preload("Protocol").Preload("SubProtocol")
	}
	if f.WithCategory {
		db = db.Preload("Category")
	}
	if f.ProductName != "" {
		db = db.Where("product_name like ?", "%"+f.ProductName+"%")
	}
	if len(f.ProductIDs) != 0 {
		db = db.Where("product_id in ?", f.ProductIDs)
	}
	if len(f.ProductNames) != 0 {
		db = db.Where("product_name in ?", f.ProductNames)
	}
	if f.ProtocolConf != nil {
		for k, v := range f.ProtocolConf {
			db = stores.CmpJsonObjEq(k, v).Where(db, "protocol_conf")
		}
	}
	if f.Tags != nil {
		for k, v := range f.Tags {
			db = stores.CmpJsonObjEq(k, v).Where(db, "tags")
		}
	}
	if f.NetType != 0 {
		db = db.Where("net_type = ?", f.NetType)
	}
	return db
}

func (p ProductInfoRepo) Insert(ctx context.Context, data *DmProductInfo) error {
	if data == nil || data.ProductID == "" || data.TenantCode == "" {
		return projecterrors.Parameter.AddDetail("product tenant and productID are required")
	}
	tenantCode := dataType.TenantCode(data.TenantCode)
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return err
	}
	err = p.db.WithContext(tenantCtx).Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(tenantCtx).Create(data).Error; err != nil {
			return err
		}

		baseModule := &DmModelModule{
			TenantCode:      tenantCode,
			ModelModuleCode: BaseModelModuleCode(data.ProductID),
			Name:            "基础属性",
		}
		if err := NewModelModuleRepo(tx).insertBase(tenantCtx, tenantCode, data.ProductID, baseModule); err != nil {
			return err
		}
		if err := NewProductModelModuleRepo(tx).Insert(tenantCtx, tenantCode, &DmProductModelModule{
			TenantCode: tenantCode,
			ProductID:  data.ProductID,
			ModuleID:   baseModule.ID,
			Order:      1,
		}); err != nil {
			return err
		}
		return inheritCategorySchema(tx, tenantCtx, data, baseModule.ID)
	})
	return stores.ErrFmt(err)
}

func inheritCategorySchema(tx *gorm.DB, ctx context.Context, product *DmProductInfo, baseModuleID int64) error {
	if product.CategoryID == def.NotClassified {
		return nil
	}
	categoryIDs := []int64{product.CategoryID}
	if product.CategoryID > def.NotClassified {
		category, err := NewProductCategoryRepo(tx).FindOne(ctx, product.CategoryID)
		if err != nil {
			return err
		}
		categoryIDs = append(categoryIDs, utils.GetIDPath(category.IDPath)...)
	}
	categorySchemas, err := NewProductCategorySchemaRepo(tx).FindByFilter(ctx, ProductCategorySchemaFilter{ProductCategoryIDs: categoryIDs}, nil)
	if err != nil {
		return err
	}
	if len(categorySchemas) == 0 {
		return nil
	}
	identifiers := utils.ToSliceWithFunc(categorySchemas, func(in *DmProductCategorySchema) string {
		return in.Identifier
	})
	commonSchemas, err := NewCommonSchemaRepo(tx).FindByFilter(ctx, CommonSchemaFilter{Identifiers: identifiers}, nil)
	if err != nil {
		return err
	}
	if len(commonSchemas) == 0 {
		return nil
	}

	tenantCode := dataType.TenantCode(product.TenantCode)
	productSchemas := make([]*DmProductSchema, 0, len(commonSchemas))
	for _, common := range commonSchemas {
		core := common.DmSchemaCore
		core.Tag = schema.TagRequired
		if core.Type == schema.AffordanceTypeProperty {
			property := ToPropertyDo(common.Identifier, &core)
			row := ToModelModulePropertyPo(string(tenantCode), baseModuleID, property)
			if err := NewModelModulePropertyRepo(tx).Insert(ctx, tenantCode, row); err != nil {
				return err
			}
			continue
		}
		if core.Type != schema.AffordanceTypeEvent && core.Type != schema.AffordanceTypeAction {
			return projecterrors.Type.AddDetail("category schema must be Property, Event or Action")
		}
		productSchemas = append(productSchemas, &DmProductSchema{
			TenantCode:   product.TenantCode,
			ProductID:    product.ProductID,
			Identifier:   common.Identifier,
			DmSchemaCore: core,
		})
	}
	if len(productSchemas) != 0 {
		if err := tx.WithContext(ctx).Create(productSchemas).Error; err != nil {
			return err
		}
	}
	return NewProductConfigRepo(tx).Insert(ctx, &DmProductConfig{ProductID: product.ProductID})
}

func (p ProductInfoRepo) FindOneByFilter(ctx context.Context, f ProductFilter) (*DmProductInfo, error) {
	var result DmProductInfo
	db := p.fmtFilter(ctx, f)
	err := db.Preload("Config").First(&result).Error
	if err != nil {
		return nil, stores.ErrFmt(err)
	}
	return &result, nil
}

func (p ProductInfoRepo) Update(ctx context.Context, data *DmProductInfo) error {
	data2 := *data
	data2.Config = nil
	// Model release activation is the only writer of current_release_id. A
	// metadata update may hold a stale product snapshot and must not undo a
	// concurrent activation or explicit rollback.
	err := p.db.WithContext(ctx).Omit("current_release_id").Where("product_id = ?", data.ProductID).Save(&data2).Error
	return stores.ErrFmt(err)
}

func (d ProductInfoRepo) UpdateWithField(ctx context.Context, f ProductFilter, updates map[string]any) error {
	db := d.fmtFilter(ctx, f)
	err := db.Model(&DmProductInfo{}).Updates(updates).Error
	return stores.ErrFmt(err)
}

func (p ProductInfoRepo) Delete(ctx context.Context, productID string) error {
	if productID == "" {
		return projecterrors.Parameter.AddDetail("productID is required")
	}
	db := p.db.WithContext(ctx)
	err := db.Transaction(func(tx *gorm.DB) error {
		var product DmProductInfo
		if err := tx.Where("product_id = ?", productID).First(&product).Error; err != nil {
			return err
		}
		tenantCode := dataType.TenantCode(product.TenantCode)
		var baseModules []DmModelModule
		if err := tx.Where("tenant_code = ? AND model_module_code = ? AND deleted_time = 0", tenantCode, BaseModelModuleCode(productID)).Find(&baseModules).Error; err != nil {
			return err
		}
		if len(baseModules) > 1 {
			return projecterrors.Duplicate.AddDetail("multiple active base model modules found")
		}
		if len(baseModules) == 1 {
			baseModule := baseModules[0]
			var foreignBindings int64
			if err := tx.Table((&DmProductModelModule{}).TableName()).Where("tenant_code = ? AND module_id = ? AND product_id <> ? AND deleted_time = 0", tenantCode, baseModule.ID, productID).Count(&foreignBindings).Error; err != nil {
				return err
			}
			if foreignBindings != 0 {
				return projecterrors.NotEmpty.AddDetail("base model module has a foreign product binding")
			}
			if err := tx.Where("tenant_code = ? AND module_id = ?", tenantCode, baseModule.ID).Delete(&DmModelModuleProperty{}).Error; err != nil {
				return err
			}
			if err := tx.Where("tenant_code = ? AND id = ?", tenantCode, baseModule.ID).Delete(&DmModelModule{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("tenant_code = ? AND product_id = ?", tenantCode, productID).Delete(&DmProductModelModule{}).Error; err != nil {
			return err
		}
		if err := tx.Where("product_id = ?", productID).Delete(&DmProductConfig{}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_code = ? AND product_id = ?", tenantCode, productID).Delete(&DmProductInfo{}).Error
	})
	return stores.ErrFmt(err)
}

func (p ProductInfoRepo) FindByFilter(ctx context.Context, f ProductFilter, page *stores.PageInfo) ([]*DmProductInfo, error) {
	var results []*DmProductInfo
	db := p.fmtFilter(ctx, f).Model(&DmProductInfo{})
	db = page.ToGorm(db)
	err := db.Preload("Config").Find(&results).Error
	if err != nil {
		return nil, stores.ErrFmt(err)
	}
	return results, nil
}

func (p ProductInfoRepo) CountByFilter(ctx context.Context, f ProductFilter) (size int64, err error) {
	db := p.fmtFilter(ctx, f).Model(&DmProductInfo{})
	err = db.Count(&size).Error
	return size, stores.ErrFmt(err)
}
