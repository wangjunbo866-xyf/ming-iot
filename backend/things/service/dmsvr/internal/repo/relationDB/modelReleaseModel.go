package relationDB

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"gitee.com/ming-iot/saas/share/dataType"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/stores"
	"gitee.com/ming-iot/things/share/domain/schema"
	"gorm.io/gorm"
)

const ModelReleaseStatusValid = "valid"

var lowerSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type DmModelModule struct {
	ID              int64               `gorm:"column:id;primaryKey;autoIncrement"`
	TenantCode      dataType.TenantCode `gorm:"column:tenant_code;type:varchar(50);not null;uniqueIndex:idx_dm_module_info_tenant_code,priority:1"`
	ModelModuleCode string              `gorm:"column:model_module_code;type:varchar(105);not null;uniqueIndex:idx_dm_module_info_tenant_code,priority:2"`
	Name            string              `gorm:"column:name;type:varchar(100);not null"`
	Description     string              `gorm:"column:description;type:varchar(200);default:''"`
	stores.NoDelTime
	DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:idx_dm_module_info_tenant_code,priority:3"`
}

func (*DmModelModule) TableName() string { return "dm_module_info" }

type DmModelModuleProperty struct {
	ID                int64                 `gorm:"column:id;primaryKey;autoIncrement"`
	TenantCode        dataType.TenantCode   `gorm:"column:tenant_code;type:varchar(50);not null;uniqueIndex:idx_dm_module_schema_info_tenant_module_identifier,priority:1"`
	ModuleID          int64                 `gorm:"column:module_id;type:bigint;not null;index;uniqueIndex:idx_dm_module_schema_info_tenant_module_identifier,priority:2"`
	Identifier        string                `gorm:"column:identifier;type:varchar(100);not null;uniqueIndex:idx_dm_module_schema_info_tenant_module_identifier,priority:3"`
	Tag               schema.Tag            `gorm:"column:tag;type:smallint;default:1"`
	Type              schema.AffordanceType `gorm:"column:type;index:idx_dm_module_schema_info_type;type:smallint;default:1"`
	ExtendConfig      string                `gorm:"column:extend_config;type:text"`
	Required          int64                 `gorm:"column:required;type:smallint;default:2"`
	Name              string                `gorm:"column:name;type:varchar(100);not null"`
	Desc              string                `gorm:"column:desc;type:varchar(200)"`
	IsCanSceneLinkage int64                 `gorm:"column:is_can_scene_linkage;type:smallint;default:1"`
	FuncGroup         int64                 `gorm:"column:func_group;type:smallint;default:1"`
	ControlMode       int64                 `gorm:"column:control_mode;type:smallint;default:1"`
	UserPerm          int64                 `gorm:"column:user_auth;type:smallint;default:3"`
	RecordMode        schema.RecordMode     `gorm:"column:record_mode;type:smallint;default:1"`
	IsPassword        int64                 `gorm:"column:is_password;type:smallint;default:2"`
	Affordance        string                `gorm:"column:affordance;type:json;not null"`
	Order             int64                 `gorm:"column:order;type:bigint;default:1;not null"`
	stores.NoDelTime
	DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:idx_dm_module_schema_info_tenant_module_identifier,priority:4"`
}

func (*DmModelModuleProperty) TableName() string { return "dm_module_schema_info" }

func (m *DmModelModuleProperty) BeforeSave(*gorm.DB) error {
	if m == nil {
		return nil
	}
	if m.Type != schema.AffordanceTypeProperty {
		return projecterrors.Type.AddDetail("model module schema only accepts Property")
	}
	switch m.RecordMode {
	case 0:
		m.RecordMode = schema.RecordModeAll
	case schema.RecordModeAll, schema.RecordModeAuto, schema.RecordModeNone:
	default:
		return projecterrors.Parameter.AddDetail("model module Property has invalid RecordMode")
	}
	var affordance schema.PropertyAffordance
	if err := json.Unmarshal([]byte(normalizeJSONString(m.Affordance)), &affordance); err != nil {
		return projecterrors.Parameter.AddDetail("model module Property affordance is invalid JSON")
	}
	affordance.IsNoRecord = m.RecordMode == schema.RecordModeNone
	payload, err := json.Marshal(affordance)
	if err != nil {
		return projecterrors.Parameter.AddDetail(err)
	}
	m.Affordance = string(payload)
	return nil
}

type DmProductModelModule struct {
	ID         int64               `gorm:"column:id;primaryKey;autoIncrement"`
	TenantCode dataType.TenantCode `gorm:"column:tenant_code;type:varchar(50);not null;uniqueIndex:idx_dm_product_module_tenant_product_module,priority:1"`
	ProductID  string              `gorm:"column:product_id;type:varchar(100);not null;index;uniqueIndex:idx_dm_product_module_tenant_product_module,priority:2"`
	ModuleID   int64               `gorm:"column:module_id;type:bigint;not null;index;uniqueIndex:idx_dm_product_module_tenant_product_module,priority:3"`
	Order      int64               `gorm:"column:order;type:bigint;not null;default:1"`
	stores.NoDelTime
	DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:idx_dm_product_module_tenant_product_module,priority:4"`
}

func (*DmProductModelModule) TableName() string { return "dm_product_module" }

// SchemaBundleJSON stores canonical SchemaBundle bytes. PostgreSQL JSONB may
// normalize its textual output, so Scan restores the one canonical encoding.
type SchemaBundleJSON []byte

func (j SchemaBundleJSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	if !json.Valid(j) {
		return nil, fmt.Errorf("invalid schema bundle JSON")
	}
	return string(j), nil
}

func (j *SchemaBundleJSON) Scan(src any) error {
	var payload []byte
	switch value := src.(type) {
	case []byte:
		payload = value
	case string:
		payload = []byte(value)
	case nil:
		*j = nil
		return nil
	default:
		return fmt.Errorf("unsupported schema bundle JSON source %T", src)
	}
	var bundle schema.SchemaBundle
	if err := json.Unmarshal(payload, &bundle); err != nil {
		return err
	}
	canonical, err := bundle.CanonicalBytes()
	if err != nil {
		return err
	}
	*j = canonical
	return nil
}

type DmProductModelRelease struct {
	ID            int64               `gorm:"column:id;primaryKey;autoIncrement"`
	TenantCode    dataType.TenantCode `gorm:"column:tenant_code;type:varchar(50);not null;uniqueIndex:idx_dm_product_release_tenant_product_release_no,priority:1"`
	ProductID     string              `gorm:"column:product_id;type:varchar(100);not null;index;uniqueIndex:idx_dm_product_release_tenant_product_release_no,priority:2"`
	ReleaseNo     int64               `gorm:"column:release_no;type:bigint;not null;uniqueIndex:idx_dm_product_release_tenant_product_release_no,priority:3"`
	SchemaVersion int64               `gorm:"column:schema_version;type:bigint;not null"`
	BundleJSON    SchemaBundleJSON    `gorm:"column:bundle_json;type:jsonb;not null"`
	BundleHash    string              `gorm:"column:bundle_hash;type:varchar(64);not null"`
	Status        string              `gorm:"column:status;type:varchar(20);not null;default:'valid'"`
	CreatedBy     stores.CreatedBy    `gorm:"column:created_by"`
	CreatedTime   time.Time           `gorm:"column:created_time;index;sort:desc;autoCreateTime"`
}

func (*DmProductModelRelease) TableName() string { return "dm_product_release" }

func (m *DmProductModelRelease) BeforeCreate(*gorm.DB) error {
	if m.Status == "" {
		m.Status = ModelReleaseStatusValid
	}
	return m.validateSnapshot()
}

func (*DmProductModelRelease) BeforeUpdate(*gorm.DB) error {
	return projecterrors.Method.AddDetail("model release snapshots are immutable")
}

func (m *DmProductModelRelease) validateSnapshot() error {
	if m.TenantCode == "" || m.ProductID == "" || m.ReleaseNo <= 0 || m.SchemaVersion <= 0 {
		return projecterrors.Parameter.AddDetail("release tenant, product, release number and schema version are required")
	}
	if m.Status != ModelReleaseStatusValid {
		return projecterrors.Parameter.AddDetail("new model release status must be valid")
	}
	if !lowerSHA256Pattern.MatchString(m.BundleHash) {
		return projecterrors.Parameter.AddDetail("bundle hash must be 64 lowercase hexadecimal characters")
	}
	var bundle schema.SchemaBundle
	if err := json.Unmarshal(m.BundleJSON, &bundle); err != nil {
		return projecterrors.Parameter.AddDetail("bundle JSON is invalid")
	}
	canonical, err := bundle.CanonicalBytes()
	if err != nil {
		return projecterrors.Parameter.AddDetail(err)
	}
	if !bytes.Equal(canonical, m.BundleJSON) {
		return projecterrors.Parameter.AddDetail("bundle JSON is not the exact canonical encoding")
	}
	hash, err := bundle.SHA256()
	if err != nil {
		return projecterrors.Parameter.AddDetail(err)
	}
	if hash != m.BundleHash || bundle.SchemaVersion != m.SchemaVersion || bundle.Product.TenantCode != string(m.TenantCode) || bundle.Product.ProductID != m.ProductID {
		return projecterrors.Parameter.AddDetail("release metadata does not match canonical bundle")
	}
	return nil
}
