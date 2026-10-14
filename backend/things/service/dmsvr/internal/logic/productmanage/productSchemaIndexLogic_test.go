package productmanagelogic

import (
	"context"
	"testing"

	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
	"gitee.com/ming-iot/things/share/domain/schema"
)

func TestProductSchemaIndexProjectsBoundModuleProperties(t *testing.T) {
	ctx := ctxs.SetUserCtx(context.Background(), &ctxs.UserCtx{TenantCode: "tenant-a"})
	logic := NewProductSchemaIndexLogic(ctx, &svc.ServiceContext{})
	logic.composeEditableModel = func(_ context.Context, tenant dataType.TenantCode, productID string) (*schema.Model, error) {
		if tenant != "tenant-a" || productID != "product-a" {
			t.Fatalf("compose identity = %q/%q", tenant, productID)
		}
		return &schema.Model{
			Properties: schema.Properties{
				{CommonParam: schema.CommonParam{Identifier: "humidity", Name: "Humidity", Tag: schema.TagCustom, Order: 2}, Mode: schema.PropertyModeR},
				{CommonParam: schema.CommonParam{Identifier: "temperature", Name: "Temperature", Tag: schema.TagRequired, Order: 1}, Mode: schema.PropertyModeRW},
			},
			Events: schema.Events{{CommonParam: schema.CommonParam{Identifier: "overheat", Name: "Overheat", Tag: schema.TagCustom, Order: 3}}},
		}, nil
	}

	resp, err := logic.ProductSchemaIndex(&dm.ProductSchemaIndexReq{
		ProductID:    "product-a",
		Type:         int64(schema.AffordanceTypeProperty),
		PropertyMode: string(schema.PropertyModeRW),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 || len(resp.List) != 1 || resp.List[0].Identifier != "temperature" {
		t.Fatalf("index response = %+v, want only bound-module temperature Property", resp)
	}
	if resp.List[0].ProductID != "product-a" || resp.List[0].TenantCode != "tenant-a" {
		t.Fatalf("projected identity = %+v", resp.List[0])
	}
}
