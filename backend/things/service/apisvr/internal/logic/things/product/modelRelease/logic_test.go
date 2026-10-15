package modelRelease

import (
	"context"
	stderrors "errors"
	"testing"

	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"gitee.com/ming-iot/things/service/dmsvr/client/productmanage"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
	"google.golang.org/grpc"
)

type releaseProductManageRecorder struct {
	productmanage.ProductManage
	read    *dm.ProductModelReleaseReadReq
	tslRead *dm.ProductModelReleaseTslReadReq
	err     error
}

func (f *releaseProductManageRecorder) ProductModelReleaseRead(_ context.Context, in *dm.ProductModelReleaseReadReq, _ ...grpc.CallOption) (*dm.ProductModelRelease, error) {
	f.read = in
	if f.err != nil {
		return nil, f.err
	}
	return &dm.ProductModelRelease{ReleaseID: in.ReleaseID, ProductID: in.ProductID, ReleaseNo: 4, BundleHash: "sha256:test"}, nil
}

func (f *releaseProductManageRecorder) ProductModelReleaseTslRead(_ context.Context, in *dm.ProductModelReleaseTslReadReq, _ ...grpc.CallOption) (*dm.ProductSchemaTslReadResp, error) {
	f.tslRead = in
	if f.err != nil {
		return nil, f.err
	}
	return &dm.ProductSchemaTslReadResp{Tsl: `{"properties":[]}`}, nil
}

func releaseServiceContext(fake productmanage.ProductManage) *svc.ServiceContext {
	return &svc.ServiceContext{SvrClient: svc.SvrClient{ProductM: fake}}
}

func TestProductModelReleaseReadUsesExactReleaseID(t *testing.T) {
	fake := &releaseProductManageRecorder{}
	req := &types.ProductModelReleaseReadReq{ProductID: "p-1", ReleaseID: 73}
	got, err := NewProductModelReleaseReadLogic(context.Background(), releaseServiceContext(fake)).ProductModelReleaseRead(req)
	if err != nil {
		t.Fatal(err)
	}
	if fake.read == nil || fake.read.ProductID != "p-1" || fake.read.ReleaseID != 73 {
		t.Fatalf("exact release request mismatch: %+v", fake.read)
	}
	if got.ReleaseID != 73 || got.ReleaseNo != 4 || got.BundleHash != "sha256:test" {
		t.Fatalf("release response mismatch: %+v", got)
	}
}

func TestProductModelReleaseTslReadPreservesActiveSelector(t *testing.T) {
	fake := &releaseProductManageRecorder{}
	req := &types.ProductModelReleaseTslReadReq{ProductID: "p-2", Active: true}
	got, err := NewProductModelReleaseTslReadLogic(context.Background(), releaseServiceContext(fake)).ProductModelReleaseTslRead(req)
	if err != nil {
		t.Fatal(err)
	}
	if fake.tslRead == nil || fake.tslRead.ProductID != "p-2" || !fake.tslRead.Active || fake.tslRead.ReleaseID != 0 {
		t.Fatalf("active release selector mismatch: %+v", fake.tslRead)
	}
	if got.Tsl != `{"properties":[]}` {
		t.Fatalf("flat TSL response mismatch: %+v", got)
	}
}

func TestProductModelReleaseReadReturnsMappedRPCError(t *testing.T) {
	fake := &releaseProductManageRecorder{err: stderrors.New("release missing")}
	_, err := NewProductModelReleaseReadLogic(context.Background(), releaseServiceContext(fake)).ProductModelReleaseRead(&types.ProductModelReleaseReadReq{ProductID: "p-1", ReleaseID: 5})
	if err == nil {
		t.Fatal("RPC error was swallowed")
	}
}
