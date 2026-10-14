package productmanagelogic

import (
	"context"
	"errors"
	"testing"

	"gitee.com/ming-iot/saas/share/dataType"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/things/service/dmsvr/internal/domain/modelrelease"
	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
	"gitee.com/ming-iot/things/share/domain/schema"
)

func TestModelModulePropertyRowForcesPropertyKind(t *testing.T) {
	row, err := modelModulePropertyRow(dataType.TenantCode("tenant-a"), &dm.ModelModuleProperty{
		ModelModuleID: 9,
		Identifier:    "temperature",
		RecordMode:    int64(schema.RecordModeAll),
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.TenantCode != "tenant-a" || row.ModuleID != 9 || row.Type != schema.AffordanceTypeProperty {
		t.Fatalf("unexpected module Property row: %+v", row)
	}
}

func TestMapModelReleaseErrorUsesStableProjectCodes(t *testing.T) {
	cases := []struct {
		in   error
		want *projecterrors.CodeError
	}{
		{modelrelease.ErrInvalidArgument, projecterrors.Parameter},
		{modelrelease.ErrMigrationRequired, projecterrors.NotEnable},
		{modelrelease.ErrHistoricalConflict, projecterrors.Parameter},
		{modelrelease.ErrNoActiveRelease, projecterrors.NotFind},
		{modelrelease.ErrReleaseMismatch, projecterrors.NotFind},
		{modelrelease.ErrInvalidRelease, projecterrors.System},
	}
	for _, tc := range cases {
		if got := mapModelReleaseError(tc.in); !tc.want.Eq(got) {
			t.Errorf("mapModelReleaseError(%v) = %v, want code %d", tc.in, got, tc.want.Code)
		}
	}
	other := errors.New("other")
	if got := mapModelReleaseError(other); got != other {
		t.Fatalf("unclassified error changed: %v", got)
	}
}

func TestProductModelReleaseTslReadRequiresExplicitSelection(t *testing.T) {
	logic := NewProductModelReleaseTslReadLogic(context.Background(), &svc.ServiceContext{})
	for _, req := range []*dm.ProductModelReleaseTslReadReq{
		{ProductID: "p1"},
		{ProductID: "p1", Active: true, ReleaseID: 7},
	} {
		if _, err := logic.ProductModelReleaseTslRead(req); !projecterrors.Parameter.Eq(err) {
			t.Errorf("request %+v error = %v, want Parameter", req, err)
		}
	}
}
