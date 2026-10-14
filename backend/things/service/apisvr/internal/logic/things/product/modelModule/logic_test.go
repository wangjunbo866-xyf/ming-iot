package modelModule

import (
	"context"
	stderrors "errors"
	"testing"

	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"gitee.com/ming-iot/things/service/dmsvr/client/productmanage"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type moduleProductManageRecorder struct {
	productmanage.ProductManage
	propertyCreate *dm.ModelModulePropertyCreateReq
	bind           *dm.ProductModelModuleBindReq
	err            error
}

func (f *moduleProductManageRecorder) ModelModulePropertyCreate(_ context.Context, in *dm.ModelModulePropertyCreateReq, _ ...grpc.CallOption) (*dm.ModelModuleProperty, error) {
	f.propertyCreate = in
	if f.err != nil {
		return nil, f.err
	}
	result := proto.Clone(in.Property).(*dm.ModelModuleProperty)
	result.ModelModulePropertyID = 91
	return result, nil
}

func (f *moduleProductManageRecorder) ProductModelModuleBind(_ context.Context, in *dm.ProductModelModuleBindReq, _ ...grpc.CallOption) (*dm.Empty, error) {
	f.bind = in
	return &dm.Empty{}, f.err
}

func moduleServiceContext(fake productmanage.ProductManage) *svc.ServiceContext {
	return &svc.ServiceContext{SvrClient: svc.SvrClient{ProductM: fake}}
}

func TestModelModulePropertyCreateMapsCompleteProperty(t *testing.T) {
	fake := &moduleProductManageRecorder{}
	req := &types.ModelModulePropertyCreateReq{ModelModulePropertyInfo: &types.ModelModulePropertyInfo{
		ModelModuleID: 7, Identifier: "temperature", Name: "温度", Description: "环境温度",
		Tag: 2, Required: 1, IsCanSceneLinkage: 1, FuncGroup: 2, UserPerm: 3,
		ControlMode: 2, RecordMode: 2, IsPassword: 1, Affordance: `{"define":{"type":"float"}}`,
		ExtendConfig: `{"unit":"celsius"}`, Order: 8,
	}}
	got, err := NewModelModulePropertyCreateLogic(context.Background(), moduleServiceContext(fake)).ModelModulePropertyCreate(req)
	if err != nil {
		t.Fatal(err)
	}
	if fake.propertyCreate == nil || fake.propertyCreate.Property == nil {
		t.Fatal("RPC property request was not sent")
	}
	sent := fake.propertyCreate.Property
	if sent.ModelModuleID != 7 || sent.Identifier != "temperature" || sent.RecordMode != 2 || sent.Affordance != req.Affordance || sent.ExtendConfig != req.ExtendConfig {
		t.Fatalf("RPC property request lost fields: %+v", sent)
	}
	if got.ModelModulePropertyID != 91 || got.ModelModuleID != 7 || got.Identifier != "temperature" {
		t.Fatalf("HTTP property response mismatch: %+v", got)
	}
}

func TestProductModelModuleBindMapsExplicitIDsAndErrors(t *testing.T) {
	fake := &moduleProductManageRecorder{err: stderrors.New("bind rejected")}
	req := &types.ProductModelModuleBindReq{ProductID: "p-1", ModelModuleID: 42, Order: 3}
	err := NewProductModelModuleBindLogic(context.Background(), moduleServiceContext(fake)).ProductModelModuleBind(req)
	if err == nil {
		t.Fatal("RPC error was swallowed")
	}
	if fake.bind == nil || fake.bind.ProductID != "p-1" || fake.bind.ModelModuleID != 42 || fake.bind.Order != 3 {
		t.Fatalf("RPC binding request mismatch: %+v", fake.bind)
	}
}
