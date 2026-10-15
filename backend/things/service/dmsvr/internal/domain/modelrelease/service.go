package modelrelease

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"gitee.com/ming-iot/things/share/domain/schema"
)

type Service struct{ unitOfWork UnitOfWork }

func NewService(unitOfWork UnitOfWork) *Service { return &Service{unitOfWork: unitOfWork} }

func (s *Service) Publish(ctx context.Context, ref ProductRef, createdBy int64) (Release, error) {
	if s == nil || s.unitOfWork == nil || !ref.Valid() {
		return Release{}, ErrInvalidArgument
	}
	var published Release
	err := s.unitOfWork.WithinTransaction(ctx, func(tx Transaction) error {
		product, _, err := tx.LockProduct(ctx, ref)
		if err != nil {
			return err
		}
		if product.TenantCode != ref.TenantCode || product.ProductID != ref.ProductID {
			return ErrReleaseMismatch
		}
		draft, err := tx.LoadDraft(ctx, ref)
		if err != nil {
			return err
		}
		if len(draft.Modules) == 0 {
			return ErrMigrationRequired
		}
		if draft.Product != product {
			return fmt.Errorf("%w: draft product identity mismatch", ErrReleaseMismatch)
		}
		bundle, err := schema.CompileSchemaBundle(draft)
		if err != nil {
			return err
		}
		history, err := tx.ListHistoricalReleases(ctx, ref)
		if err != nil {
			return err
		}
		if err := validateHistoricalInvariants(ref, bundle, history); err != nil {
			return err
		}
		releaseNo := int64(1)
		for _, previous := range history {
			if previous.ReleaseNo >= releaseNo {
				releaseNo = previous.ReleaseNo + 1
			}
		}
		payload, err := bundle.CanonicalBytes()
		if err != nil {
			return err
		}
		hash, err := bundle.SHA256()
		if err != nil {
			return err
		}
		published, err = tx.InsertRelease(ctx, NewRelease{
			Product:       ref,
			ReleaseNo:     releaseNo,
			SchemaVersion: bundle.SchemaVersion,
			Bundle:        bundle,
			BundleJSON:    payload,
			BundleHash:    hash,
			CreatedBy:     createdBy,
		})
		return err
	})
	return published, err
}

func (s *Service) Activate(ctx context.Context, ref ReleaseRef) error {
	if s == nil || s.unitOfWork == nil || !ref.ProductRef.Valid() || ref.ReleaseID <= 0 {
		return ErrInvalidArgument
	}
	return s.unitOfWork.WithinTransaction(ctx, func(tx Transaction) error {
		product, _, err := tx.LockProduct(ctx, ref.ProductRef)
		if err != nil {
			return err
		}
		if product.TenantCode != ref.TenantCode || product.ProductID != ref.ProductID {
			return ErrReleaseMismatch
		}
		release, err := tx.ReadRelease(ctx, ref)
		if err != nil {
			return err
		}
		if err := validateStoredRelease(ref, release); err != nil {
			return err
		}
		return tx.SetCurrentRelease(ctx, ref.ProductRef, release.ID)
	})
}

func (s *Service) ReadExact(ctx context.Context, ref ReleaseRef) (Release, error) {
	if s == nil || s.unitOfWork == nil || !ref.ProductRef.Valid() || ref.ReleaseID <= 0 {
		return Release{}, ErrInvalidArgument
	}
	release, err := s.unitOfWork.ReadRelease(ctx, ref)
	if err != nil {
		return Release{}, err
	}
	if err := validateStoredRelease(ref, release); err != nil {
		return Release{}, err
	}
	return release, nil
}

func (s *Service) ReadActive(ctx context.Context, ref ProductRef) (Release, error) {
	if s == nil || s.unitOfWork == nil || !ref.Valid() {
		return Release{}, ErrInvalidArgument
	}
	var active Release
	err := s.unitOfWork.WithinTransaction(ctx, func(tx Transaction) error {
		product, currentReleaseID, err := tx.LockProduct(ctx, ref)
		if err != nil {
			return err
		}
		if product.TenantCode != ref.TenantCode || product.ProductID != ref.ProductID {
			return ErrReleaseMismatch
		}
		if currentReleaseID == 0 {
			return ErrNoActiveRelease
		}
		active, err = tx.ReadRelease(ctx, ReleaseRef{ProductRef: ref, ReleaseID: currentReleaseID})
		if err != nil {
			return err
		}
		return validateStoredRelease(ReleaseRef{ProductRef: ref, ReleaseID: currentReleaseID}, active)
	})
	return active, err
}

func validateHistoricalInvariants(ref ProductRef, current schema.SchemaBundle, history []Release) error {
	type historicalField struct {
		kind   schema.AffordanceType
		define []byte
	}
	seen := make(map[string]historicalField)
	for _, release := range history {
		if release.Product != ref {
			return fmt.Errorf("%w: historical release product mismatch", ErrInvalidRelease)
		}
		normalized, err := normalizedStoredBundle(release)
		if err != nil {
			return err
		}
		for _, property := range normalized.Model.Properties {
			definition, err := json.Marshal(property.Define)
			if err != nil {
				return err
			}
			if previous, ok := seen[property.Identifier]; ok && (previous.kind != schema.AffordanceTypeProperty || !bytes.Equal(previous.define, definition)) {
				return fmt.Errorf("%w: historical Property %q is inconsistent", ErrInvalidRelease, property.Identifier)
			}
			seen[property.Identifier] = historicalField{kind: schema.AffordanceTypeProperty, define: definition}
		}
		for _, event := range normalized.Model.Events {
			if previous, ok := seen[event.Identifier]; ok && previous.kind != schema.AffordanceTypeEvent {
				return fmt.Errorf("%w: historical identifier %q changed kind", ErrInvalidRelease, event.Identifier)
			}
			seen[event.Identifier] = historicalField{kind: schema.AffordanceTypeEvent}
		}
		for _, action := range normalized.Model.Actions {
			if previous, ok := seen[action.Identifier]; ok && previous.kind != schema.AffordanceTypeAction {
				return fmt.Errorf("%w: historical identifier %q changed kind", ErrInvalidRelease, action.Identifier)
			}
			seen[action.Identifier] = historicalField{kind: schema.AffordanceTypeAction}
		}
	}
	for _, property := range current.Model.Properties {
		if previous, ok := seen[property.Identifier]; ok {
			if previous.kind != schema.AffordanceTypeProperty {
				return fmt.Errorf("%w: identifier %q changed kind", ErrHistoricalConflict, property.Identifier)
			}
			definition, err := json.Marshal(property.Define)
			if err != nil {
				return err
			}
			if !bytes.Equal(previous.define, definition) {
				return fmt.Errorf("%w: Property %q changed data type or value domain", ErrHistoricalConflict, property.Identifier)
			}
		}
	}
	for _, event := range current.Model.Events {
		if previous, ok := seen[event.Identifier]; ok && previous.kind != schema.AffordanceTypeEvent {
			return fmt.Errorf("%w: identifier %q changed kind", ErrHistoricalConflict, event.Identifier)
		}
	}
	for _, action := range current.Model.Actions {
		if previous, ok := seen[action.Identifier]; ok && previous.kind != schema.AffordanceTypeAction {
			return fmt.Errorf("%w: identifier %q changed kind", ErrHistoricalConflict, action.Identifier)
		}
	}
	return nil
}

func validateStoredRelease(ref ReleaseRef, release Release) error {
	if release.ID != ref.ReleaseID || release.Product != ref.ProductRef {
		return ErrReleaseMismatch
	}
	_, err := normalizedStoredBundle(release)
	return err
}

func normalizedStoredBundle(release Release) (schema.SchemaBundle, error) {
	if release.ID <= 0 || release.ReleaseNo <= 0 || release.SchemaVersion != schema.SchemaBundleVersion || release.Status != ReleaseStatusValid {
		return schema.SchemaBundle{}, ErrInvalidRelease
	}
	payload, err := release.Bundle.CanonicalBytes()
	if err != nil {
		return schema.SchemaBundle{}, fmt.Errorf("%w: %v", ErrInvalidRelease, err)
	}
	hash, err := release.Bundle.SHA256()
	if err != nil {
		return schema.SchemaBundle{}, fmt.Errorf("%w: %v", ErrInvalidRelease, err)
	}
	if hash != release.BundleHash {
		return schema.SchemaBundle{}, fmt.Errorf("%w: bundle hash mismatch", ErrInvalidRelease)
	}
	var normalized schema.SchemaBundle
	if err := json.Unmarshal(payload, &normalized); err != nil {
		return schema.SchemaBundle{}, fmt.Errorf("%w: %v", ErrInvalidRelease, err)
	}
	return normalized, nil
}
