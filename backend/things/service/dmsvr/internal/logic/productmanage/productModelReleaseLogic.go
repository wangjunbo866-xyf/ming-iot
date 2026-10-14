package productmanagelogic

import (
	"context"
	stderrors "errors"

	"gitee.com/ming-iot/share/ctxs"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/stores"
	"gitee.com/ming-iot/things/service/dmsvr/internal/domain/modelrelease"
	"gitee.com/ming-iot/things/service/dmsvr/internal/logic"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"
	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
	"github.com/zeromicro/go-zero/core/logx"
)

func modelReleaseRef(ctx context.Context, productID string) (modelrelease.ProductRef, error) {
	ref := modelrelease.ProductRef{TenantCode: ctxs.GetUserCtxNoNil(ctx).TenantCode, ProductID: productID}
	if !ref.Valid() {
		return modelrelease.ProductRef{}, projecterrors.Parameter.AddDetail("tenantCode and productID are required")
	}
	return ref, nil
}

func toProductModelReleasePB(release modelrelease.Release) (*dm.ProductModelRelease, error) {
	payload, err := release.Bundle.CanonicalBytes()
	if err != nil {
		return nil, err
	}
	createdTime := int64(0)
	if !release.CreatedTime.IsZero() {
		createdTime = release.CreatedTime.Unix()
	}
	return &dm.ProductModelRelease{
		ReleaseID:     release.ID,
		ProductID:     release.Product.ProductID,
		ReleaseNo:     release.ReleaseNo,
		SchemaVersion: release.SchemaVersion,
		Bundle:        string(payload),
		BundleHash:    release.BundleHash,
		Status:        release.Status,
		CreatedTime:   createdTime,
	}, nil
}

func mapModelReleaseError(err error) error {
	switch {
	case err == nil:
		return nil
	case stderrors.Is(err, modelrelease.ErrInvalidArgument):
		return projecterrors.Parameter.AddDetail("MODEL_RELEASE_INVALID_ARGUMENT")
	case stderrors.Is(err, modelrelease.ErrMigrationRequired):
		return projecterrors.NotEnable.AddDetail("MIGRATION_REQUIRED")
	case stderrors.Is(err, modelrelease.ErrHistoricalConflict):
		return projecterrors.Parameter.AddDetail("HISTORICAL_SCHEMA_CONFLICT")
	case stderrors.Is(err, modelrelease.ErrNoActiveRelease):
		return projecterrors.NotFind.AddDetail("NO_ACTIVE_RELEASE")
	case stderrors.Is(err, modelrelease.ErrReleaseMismatch):
		return projecterrors.NotFind.AddDetail("MODEL_RELEASE_NOT_FOUND")
	case stderrors.Is(err, modelrelease.ErrInvalidRelease):
		return projecterrors.System.AddDetail("INVALID_MODEL_RELEASE")
	default:
		return err
	}
}

type ProductModelReleasePublishLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewProductModelReleasePublishLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelReleasePublishLogic {
	return &ProductModelReleasePublishLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ProductModelReleasePublishLogic) ProductModelReleasePublish(in *dm.ProductModelReleasePublishReq) (*dm.ProductModelRelease, error) {
	if in == nil {
		return nil, projecterrors.Parameter.AddDetail("request is required")
	}
	ref, err := modelReleaseRef(l.ctx, in.ProductID)
	if err != nil {
		return nil, err
	}
	release, err := modelrelease.NewService(relationDB.NewModelReleaseUnitOfWork(l.ctx)).Publish(l.ctx, ref, ctxs.GetUserCtxNoNil(l.ctx).UserID)
	if err != nil {
		return nil, mapModelReleaseError(err)
	}
	return toProductModelReleasePB(release)
}

type ProductModelReleaseActivateLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewProductModelReleaseActivateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelReleaseActivateLogic {
	return &ProductModelReleaseActivateLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ProductModelReleaseActivateLogic) ProductModelReleaseActivate(in *dm.ProductModelReleaseActivateReq) (*dm.Empty, error) {
	if in == nil || in.ReleaseID <= 0 {
		return nil, projecterrors.Parameter.AddDetail("releaseID must be positive")
	}
	ref, err := modelReleaseRef(l.ctx, in.ProductID)
	if err != nil {
		return nil, err
	}
	err = modelrelease.NewService(relationDB.NewModelReleaseUnitOfWork(l.ctx)).Activate(l.ctx, modelrelease.ReleaseRef{ProductRef: ref, ReleaseID: in.ReleaseID})
	if err != nil {
		return nil, mapModelReleaseError(err)
	}
	return &dm.Empty{}, nil
}

type ProductModelReleaseReadLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewProductModelReleaseReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelReleaseReadLogic {
	return &ProductModelReleaseReadLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ProductModelReleaseReadLogic) ProductModelReleaseRead(in *dm.ProductModelReleaseReadReq) (*dm.ProductModelRelease, error) {
	if in == nil || in.ReleaseID <= 0 {
		return nil, projecterrors.Parameter.AddDetail("releaseID must be positive")
	}
	ref, err := modelReleaseRef(l.ctx, in.ProductID)
	if err != nil {
		return nil, err
	}
	release, err := modelrelease.NewService(relationDB.NewModelReleaseUnitOfWork(l.ctx)).ReadExact(l.ctx, modelrelease.ReleaseRef{ProductRef: ref, ReleaseID: in.ReleaseID})
	if err != nil {
		return nil, mapModelReleaseError(err)
	}
	return toProductModelReleasePB(release)
}

type ProductModelReleaseIndexLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewProductModelReleaseIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelReleaseIndexLogic {
	return &ProductModelReleaseIndexLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ProductModelReleaseIndexLogic) ProductModelReleaseIndex(in *dm.ProductModelReleaseIndexReq) (*dm.ProductModelReleaseIndexResp, error) {
	if in == nil {
		return nil, projecterrors.Parameter.AddDetail("request is required")
	}
	ref, err := modelReleaseRef(l.ctx, in.ProductID)
	if err != nil {
		return nil, err
	}
	tenant := requestTenant(l.ctx)
	repo := relationDB.NewProductModelReleaseRepo(l.ctx)
	total, err := repo.CountByProduct(l.ctx, tenant, ref.ProductID)
	if err != nil {
		return nil, err
	}
	rows, err := repo.FindByProduct(l.ctx, tenant, ref.ProductID, logic.ToPageInfo(in.Page).WithDefaultOrder(stores.OrderBy{Field: "release_no", Sort: stores.OrderDesc}))
	if err != nil {
		return nil, err
	}
	uow := relationDB.NewModelReleaseUnitOfWork(l.ctx)
	list := make([]*dm.ProductModelRelease, 0, len(rows))
	for _, row := range rows {
		release, err := uow.ReadRelease(l.ctx, modelrelease.ReleaseRef{ProductRef: ref, ReleaseID: row.ID})
		if err != nil {
			return nil, mapModelReleaseError(err)
		}
		item, err := toProductModelReleasePB(release)
		if err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return &dm.ProductModelReleaseIndexResp{List: list, Total: total}, nil
}

type ProductModelReleaseTslReadLogic struct {
	ctx context.Context
	*svc.ServiceContext
	logx.Logger
}

func NewProductModelReleaseTslReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelReleaseTslReadLogic {
	return &ProductModelReleaseTslReadLogic{ctx: ctx, ServiceContext: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ProductModelReleaseTslReadLogic) ProductModelReleaseTslRead(in *dm.ProductModelReleaseTslReadReq) (*dm.ProductSchemaTslReadResp, error) {
	if in == nil || (in.Active && in.ReleaseID != 0) || (!in.Active && in.ReleaseID <= 0) {
		return nil, projecterrors.Parameter.AddDetail("select either active release or one positive releaseID")
	}
	ref, err := modelReleaseRef(l.ctx, in.ProductID)
	if err != nil {
		return nil, err
	}
	service := modelrelease.NewService(relationDB.NewModelReleaseUnitOfWork(l.ctx))
	var release modelrelease.Release
	if in.Active {
		release, err = service.ReadActive(l.ctx, ref)
	} else {
		release, err = service.ReadExact(l.ctx, modelrelease.ReleaseRef{ProductRef: ref, ReleaseID: in.ReleaseID})
	}
	if err != nil {
		return nil, mapModelReleaseError(err)
	}
	model, err := release.Bundle.FlatModel()
	if err != nil {
		return nil, mapModelReleaseError(modelrelease.ErrInvalidRelease)
	}
	return &dm.ProductSchemaTslReadResp{Tsl: model.String()}, nil
}
