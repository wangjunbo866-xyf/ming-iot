package modelrelease

import (
	"context"

	"gitee.com/ming-iot/things/share/domain/schema"
)

type UnitOfWork interface {
	WithinTransaction(context.Context, func(Transaction) error) error
	ReadRelease(context.Context, ReleaseRef) (Release, error)
}

type Transaction interface {
	LockProduct(context.Context, ProductRef) (schema.ProductSnapshot, int64, error)
	LoadDraft(context.Context, ProductRef) (schema.ProductDraft, error)
	ListHistoricalReleases(context.Context, ProductRef) ([]Release, error)
	ReadRelease(context.Context, ReleaseRef) (Release, error)
	InsertRelease(context.Context, NewRelease) (Release, error)
	SetCurrentRelease(context.Context, ProductRef, int64) error
}
