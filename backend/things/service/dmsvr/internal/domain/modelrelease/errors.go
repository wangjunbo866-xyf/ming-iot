package modelrelease

import "errors"

var (
	ErrInvalidArgument    = errors.New("model release: invalid argument")
	ErrMigrationRequired  = errors.New("model release: product requires model-module migration")
	ErrHistoricalConflict = errors.New("model release: historical schema invariant violated")
	ErrNoActiveRelease    = errors.New("model release: no active release")
	ErrReleaseMismatch    = errors.New("model release: release does not belong to product")
	ErrInvalidRelease     = errors.New("model release: invalid or corrupted release")
)
