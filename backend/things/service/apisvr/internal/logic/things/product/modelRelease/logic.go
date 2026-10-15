package modelRelease

import (
	"context"

	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/things/service/apisvr/internal/logic"
	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
	"github.com/zeromicro/go-zero/core/logx"
)

type baseLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func newBaseLogic(ctx context.Context, svcCtx *svc.ServiceContext) *baseLogic {
	return &baseLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *baseLogic) rpcError(operation string, request any, err error) error {
	if err == nil {
		return nil
	}
	mapped := projecterrors.Fmt(err)
	l.Errorf("%s request=%v err=%+v", operation, request, mapped)
	return mapped
}

func releaseInfo(in *dm.ProductModelRelease) *types.ProductModelReleaseInfo {
	if in == nil {
		return nil
	}
	return &types.ProductModelReleaseInfo{
		ReleaseID: in.ReleaseID, ProductID: in.ProductID, ReleaseNo: in.ReleaseNo,
		SchemaVersion: in.SchemaVersion, Bundle: in.Bundle, BundleHash: in.BundleHash,
		Status: in.Status, CreatedTime: in.CreatedTime,
	}
}

type ProductModelReleasePublishLogic struct{ *baseLogic }

func NewProductModelReleasePublishLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelReleasePublishLogic {
	return &ProductModelReleasePublishLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ProductModelReleasePublishLogic) ProductModelReleasePublish(req *types.ProductModelReleasePublishReq) (*types.ProductModelReleaseInfo, error) {
	resp, err := l.svcCtx.ProductM.ProductModelReleasePublish(l.ctx, &dm.ProductModelReleasePublishReq{ProductID: req.ProductID})
	if err != nil {
		return nil, l.rpcError("ProductM.ProductModelReleasePublish", req, err)
	}
	return releaseInfo(resp), nil
}

type ProductModelReleaseActivateLogic struct{ *baseLogic }

func NewProductModelReleaseActivateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelReleaseActivateLogic {
	return &ProductModelReleaseActivateLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ProductModelReleaseActivateLogic) ProductModelReleaseActivate(req *types.ProductModelReleaseActivateReq) error {
	_, err := l.svcCtx.ProductM.ProductModelReleaseActivate(l.ctx, &dm.ProductModelReleaseActivateReq{
		ProductID: req.ProductID, ReleaseID: req.ReleaseID,
	})
	return l.rpcError("ProductM.ProductModelReleaseActivate", req, err)
}

type ProductModelReleaseReadLogic struct{ *baseLogic }

func NewProductModelReleaseReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelReleaseReadLogic {
	return &ProductModelReleaseReadLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ProductModelReleaseReadLogic) ProductModelReleaseRead(req *types.ProductModelReleaseReadReq) (*types.ProductModelReleaseInfo, error) {
	resp, err := l.svcCtx.ProductM.ProductModelReleaseRead(l.ctx, &dm.ProductModelReleaseReadReq{
		ProductID: req.ProductID, ReleaseID: req.ReleaseID,
	})
	if err != nil {
		return nil, l.rpcError("ProductM.ProductModelReleaseRead", req, err)
	}
	return releaseInfo(resp), nil
}

type ProductModelReleaseIndexLogic struct{ *baseLogic }

func NewProductModelReleaseIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelReleaseIndexLogic {
	return &ProductModelReleaseIndexLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ProductModelReleaseIndexLogic) ProductModelReleaseIndex(req *types.ProductModelReleaseIndexReq) (*types.ProductModelReleaseIndexResp, error) {
	resp, err := l.svcCtx.ProductM.ProductModelReleaseIndex(l.ctx, &dm.ProductModelReleaseIndexReq{
		Page: logic.ToDmPageRpc(req.Page), ProductID: req.ProductID,
	})
	if err != nil {
		return nil, l.rpcError("ProductM.ProductModelReleaseIndex", req, err)
	}
	items := make([]*types.ProductModelReleaseInfo, 0, len(resp.List))
	for _, item := range resp.List {
		items = append(items, releaseInfo(item))
	}
	return &types.ProductModelReleaseIndexResp{List: items, PageResp: logic.ToPageResp(req.Page, resp.Total)}, nil
}

type ProductModelReleaseTslReadLogic struct{ *baseLogic }

func NewProductModelReleaseTslReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelReleaseTslReadLogic {
	return &ProductModelReleaseTslReadLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ProductModelReleaseTslReadLogic) ProductModelReleaseTslRead(req *types.ProductModelReleaseTslReadReq) (*types.ProductModelReleaseTslReadResp, error) {
	resp, err := l.svcCtx.ProductM.ProductModelReleaseTslRead(l.ctx, &dm.ProductModelReleaseTslReadReq{
		ProductID: req.ProductID, ReleaseID: req.ReleaseID, Active: req.Active,
	})
	if err != nil {
		return nil, l.rpcError("ProductM.ProductModelReleaseTslRead", req, err)
	}
	return &types.ProductModelReleaseTslReadResp{Tsl: resp.Tsl}, nil
}
