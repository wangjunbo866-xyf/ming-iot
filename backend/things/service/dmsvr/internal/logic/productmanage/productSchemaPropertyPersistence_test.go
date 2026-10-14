package productmanagelogic

import (
	"context"
	stderrors "errors"
	"testing"

	"gitee.com/ming-iot/saas/share/dataType"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"
	"gitee.com/ming-iot/things/share/domain/schema"
)

type recordingProductPropertyInserter struct {
	calls int
}

func (r *recordingProductPropertyInserter) Insert(_ context.Context, _ dataType.TenantCode, _ *relationDB.DmProductSchema) error {
	r.calls++
	return nil
}

type recordingLegacyPropertyStableCreator struct {
	calls     int
	productID string
	err       error
}

func (r *recordingLegacyPropertyStableCreator) CreateProperty(_ context.Context, _ *schema.Property, productID string) error {
	r.calls++
	r.productID = productID
	return r.err
}

func TestPersistProductPropertyCreatesStableOnlyForLegacyCustomProperty(t *testing.T) {
	tests := []struct {
		name       string
		legacy     bool
		tag        schema.Tag
		stableCall int
	}{
		{name: "legacy custom property", legacy: true, tag: schema.TagCustom, stableCall: 1},
		{name: "bound custom property", legacy: false, tag: schema.TagCustom, stableCall: 0},
		{name: "legacy common property", legacy: true, tag: schema.TagOptional, stableCall: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inserter := &recordingProductPropertyInserter{}
			stableCreator := &recordingLegacyPropertyStableCreator{}
			property := testProductProperty("product-a", tt.tag)

			err := persistProductProperty(context.Background(), inserter, stableCreator, property, tt.legacy)
			if err != nil {
				t.Fatalf("persistProductProperty() error = %v", err)
			}
			if stableCreator.calls != tt.stableCall {
				t.Fatalf("CreateProperty calls = %d, want %d", stableCreator.calls, tt.stableCall)
			}
			if stableCreator.calls != 0 && stableCreator.productID != property.ProductID {
				t.Fatalf("CreateProperty productID = %q, want %q", stableCreator.productID, property.ProductID)
			}
			if inserter.calls != 1 {
				t.Fatalf("draft Insert calls = %d, want 1", inserter.calls)
			}
		})
	}
}

func TestPersistProductPropertyStopsWhenLegacyStableCreationFails(t *testing.T) {
	injected := stderrors.New("injected stable creation failure")
	inserter := &recordingProductPropertyInserter{}
	stableCreator := &recordingLegacyPropertyStableCreator{err: injected}

	err := persistProductProperty(context.Background(), inserter, stableCreator, testProductProperty("product-a", schema.TagCustom), true)
	if !projecterrors.Database.Eq(err) {
		t.Fatalf("persistProductProperty() error = %v, want Database", err)
	}
	if stableCreator.calls != 1 {
		t.Fatalf("CreateProperty calls = %d, want 1", stableCreator.calls)
	}
	if inserter.calls != 0 {
		t.Fatalf("draft Insert calls = %d, want 0", inserter.calls)
	}
}

func testProductProperty(productID string, tag schema.Tag) *relationDB.DmProductSchema {
	return &relationDB.DmProductSchema{
		TenantCode: "tenant-a",
		ProductID:  productID,
		Identifier: "temperature",
		DmSchemaCore: relationDB.DmSchemaCore{
			Tag:  tag,
			Type: schema.AffordanceTypeProperty,
		},
	}
}
