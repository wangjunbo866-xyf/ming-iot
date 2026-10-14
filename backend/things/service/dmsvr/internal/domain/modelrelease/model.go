package modelrelease

import (
	"time"

	"gitee.com/ming-iot/things/share/domain/schema"
)

const ReleaseStatusValid = "valid"

type ProductRef struct {
	TenantCode string
	ProductID  string
}

func (r ProductRef) Valid() bool { return r.TenantCode != "" && r.ProductID != "" }

type ReleaseRef struct {
	ProductRef
	ReleaseID int64
}

type Release struct {
	ID            int64
	Product       ProductRef
	ReleaseNo     int64
	SchemaVersion int64
	Bundle        schema.SchemaBundle
	BundleHash    string
	Status        string
	CreatedTime   time.Time
}

type NewRelease struct {
	Product       ProductRef
	ReleaseNo     int64
	SchemaVersion int64
	Bundle        schema.SchemaBundle
	BundleJSON    []byte
	BundleHash    string
	CreatedBy     int64
}
