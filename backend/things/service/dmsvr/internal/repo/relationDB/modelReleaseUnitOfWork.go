package relationDB

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/ctxs"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/stores"
	"gitee.com/ming-iot/things/service/dmsvr/internal/domain/modelrelease"
	"gitee.com/ming-iot/things/share/domain/schema"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ModelReleaseUnitOfWork struct{ db *gorm.DB }

type modelReleaseTransaction struct{ db *gorm.DB }

func NewModelReleaseUnitOfWork(in any) *ModelReleaseUnitOfWork {
	return &ModelReleaseUnitOfWork{db: stores.GetCommonConn(in)}
}

func (u ModelReleaseUnitOfWork) WithinTransaction(ctx context.Context, fn func(modelrelease.Transaction) error) error {
	if fn == nil {
		return projecterrors.Parameter.AddDetail("transaction callback is required")
	}
	return modelReleaseDBError(u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(modelReleaseTransaction{db: tx})
	}))
}

func (u ModelReleaseUnitOfWork) ReadRelease(ctx context.Context, ref modelrelease.ReleaseRef) (modelrelease.Release, error) {
	return readModelRelease(ctx, u.db, ref)
}

func (t modelReleaseTransaction) LockProduct(ctx context.Context, ref modelrelease.ProductRef) (schema.ProductSnapshot, int64, error) {
	tenantCode := dataType.TenantCode(ref.TenantCode)
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return schema.ProductSnapshot{}, 0, err
	}
	var product DmProductInfo
	err = t.db.WithContext(tenantCtx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tenant_code = ? AND product_id = ? AND deleted_time = 0", tenantCode, ref.ProductID).
		First(&product).Error
	if err != nil {
		return schema.ProductSnapshot{}, 0, modelReleaseDBError(err)
	}
	return schema.ProductSnapshot{TenantCode: ref.TenantCode, ProductID: product.ProductID}, product.CurrentReleaseID, nil
}

func (t modelReleaseTransaction) LoadDraft(ctx context.Context, ref modelrelease.ProductRef) (schema.ProductDraft, error) {
	tenantCode := dataType.TenantCode(ref.TenantCode)
	return NewProductDraftComposer(t.db).Compose(ctx, tenantCode, ref.ProductID)
}

func (t modelReleaseTransaction) ListHistoricalReleases(ctx context.Context, ref modelrelease.ProductRef) ([]modelrelease.Release, error) {
	tenantCode := dataType.TenantCode(ref.TenantCode)
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return nil, err
	}
	rows, err := NewProductModelReleaseRepo(t.db).FindByProduct(tenantCtx, tenantCode, ref.ProductID, nil)
	if err != nil {
		return nil, err
	}
	releases := make([]modelrelease.Release, 0, len(rows))
	for _, row := range rows {
		release, err := toDomainModelRelease(row)
		if err != nil {
			return nil, err
		}
		releases = append(releases, release)
	}
	return releases, nil
}

func (t modelReleaseTransaction) ReadRelease(ctx context.Context, ref modelrelease.ReleaseRef) (modelrelease.Release, error) {
	return readModelRelease(ctx, t.db, ref)
}

func (t modelReleaseTransaction) InsertRelease(ctx context.Context, in modelrelease.NewRelease) (modelrelease.Release, error) {
	tenantCode := dataType.TenantCode(in.Product.TenantCode)
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return modelrelease.Release{}, err
	}
	canonical, err := in.Bundle.CanonicalBytes()
	if err != nil {
		return modelrelease.Release{}, err
	}
	if !bytes.Equal(canonical, in.BundleJSON) {
		return modelrelease.Release{}, projecterrors.Parameter.AddDetail("release bundle bytes are not canonical")
	}
	hash, err := in.Bundle.SHA256()
	if err != nil {
		return modelrelease.Release{}, err
	}
	if hash != in.BundleHash {
		return modelrelease.Release{}, projecterrors.Parameter.AddDetail("release bundle hash mismatch")
	}
	row := &DmProductModelRelease{
		TenantCode:    tenantCode,
		ProductID:     in.Product.ProductID,
		ReleaseNo:     in.ReleaseNo,
		SchemaVersion: in.SchemaVersion,
		BundleJSON:    SchemaBundleJSON(in.BundleJSON),
		BundleHash:    in.BundleHash,
		Status:        ModelReleaseStatusValid,
		CreatedBy:     stores.CreatedBy(in.CreatedBy),
	}
	if err := NewProductModelReleaseRepo(t.db).Insert(tenantCtx, tenantCode, row); err != nil {
		return modelrelease.Release{}, err
	}
	return toDomainModelRelease(row)
}

func (t modelReleaseTransaction) SetCurrentRelease(ctx context.Context, ref modelrelease.ProductRef, releaseID int64) error {
	if releaseID <= 0 {
		return projecterrors.Parameter.AddDetail("releaseID must be positive")
	}
	tenantCode := dataType.TenantCode(ref.TenantCode)
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return err
	}
	result := t.db.WithContext(tenantCtx).Model(&DmProductInfo{}).
		Where("tenant_code = ? AND product_id = ? AND deleted_time = 0", tenantCode, ref.ProductID).
		Update("current_release_id", releaseID)
	if result.Error != nil {
		return modelReleaseDBError(result.Error)
	}
	if result.RowsAffected != 1 {
		return projecterrors.NotFind.AddDetail("product not found in tenant")
	}
	return nil
}

func readModelRelease(ctx context.Context, db *gorm.DB, ref modelrelease.ReleaseRef) (modelrelease.Release, error) {
	tenantCode := dataType.TenantCode(ref.TenantCode)
	tenantCtx, err := withExplicitTenant(ctx, tenantCode)
	if err != nil {
		return modelrelease.Release{}, err
	}
	row, err := NewProductModelReleaseRepo(db).FindOne(tenantCtx, tenantCode, ref.ReleaseID)
	if err != nil {
		return modelrelease.Release{}, err
	}
	if row.ProductID != ref.ProductID {
		return modelrelease.Release{}, modelrelease.ErrReleaseMismatch
	}
	return toDomainModelRelease(row)
}

func toDomainModelRelease(row *DmProductModelRelease) (modelrelease.Release, error) {
	if row == nil {
		return modelrelease.Release{}, projecterrors.NotFind.WithStack(1)
	}
	var bundle schema.SchemaBundle
	if err := json.Unmarshal(row.BundleJSON, &bundle); err != nil {
		return modelrelease.Release{}, modelrelease.ErrInvalidRelease
	}
	return modelrelease.Release{
		ID:            row.ID,
		Product:       modelrelease.ProductRef{TenantCode: string(row.TenantCode), ProductID: row.ProductID},
		ReleaseNo:     row.ReleaseNo,
		SchemaVersion: row.SchemaVersion,
		Bundle:        bundle,
		BundleHash:    row.BundleHash,
		Status:        row.Status,
		CreatedTime:   row.CreatedTime,
	}, nil
}

func withExplicitTenant(ctx context.Context, tenantCode dataType.TenantCode) (context.Context, error) {
	if tenantCode == "" {
		return nil, projecterrors.Parameter.AddDetail("tenantCode is required")
	}
	if user := ctxs.GetUserCtx(ctx); user != nil && user.TenantCode != "" && user.TenantCode != string(tenantCode) && !(user.IsRoot() && user.AllTenant) {
		return nil, projecterrors.Permissions.AddDetail("explicit tenant does not match request tenant")
	}
	return ctxs.SetUserCtx(ctx, &ctxs.UserCtx{TenantCode: string(tenantCode)}), nil
}

func modelReleaseDBError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*projecterrors.CodeError); ok {
		return err
	}
	for _, domainErr := range []error{
		modelrelease.ErrInvalidArgument,
		modelrelease.ErrMigrationRequired,
		modelrelease.ErrHistoricalConflict,
		modelrelease.ErrNoActiveRelease,
		modelrelease.ErrReleaseMismatch,
		modelrelease.ErrInvalidRelease,
	} {
		if errors.Is(err, domainErr) {
			return err
		}
	}
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return projecterrors.Duplicate.AddDetail(err)
	}
	return stores.ErrFmt(err)
}
