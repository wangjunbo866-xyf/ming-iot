package modelrelease

import (
	"context"
	"errors"
	"testing"

	"gitee.com/ming-iot/things/share/domain/schema"
)

type fakeUnitOfWork struct {
	tx      *fakeTransaction
	release Release
	err     error
}

func (f *fakeUnitOfWork) WithinTransaction(ctx context.Context, fn func(Transaction) error) error {
	if f.err != nil {
		return f.err
	}
	return fn(f.tx)
}

func (f *fakeUnitOfWork) ReadRelease(context.Context, ReleaseRef) (Release, error) {
	return f.release, f.err
}

type fakeTransaction struct {
	product          schema.ProductSnapshot
	currentReleaseID int64
	draft            schema.ProductDraft
	history          []Release
	release          Release
	insertErr        error
	setErr           error
	inserted         *NewRelease
	activatedID      int64
}

func (f *fakeTransaction) LockProduct(context.Context, ProductRef) (schema.ProductSnapshot, int64, error) {
	return f.product, f.currentReleaseID, nil
}

func (f *fakeTransaction) LoadDraft(context.Context, ProductRef) (schema.ProductDraft, error) {
	return f.draft, nil
}

func (f *fakeTransaction) ListHistoricalReleases(context.Context, ProductRef) ([]Release, error) {
	return f.history, nil
}

func (f *fakeTransaction) ReadRelease(context.Context, ReleaseRef) (Release, error) {
	return f.release, nil
}

func (f *fakeTransaction) InsertRelease(_ context.Context, in NewRelease) (Release, error) {
	if f.insertErr != nil {
		return Release{}, f.insertErr
	}
	f.inserted = &in
	return Release{ID: 91, Product: in.Product, ReleaseNo: in.ReleaseNo, SchemaVersion: in.SchemaVersion, Bundle: in.Bundle, BundleHash: in.BundleHash, Status: ReleaseStatusValid}, nil
}

func (f *fakeTransaction) SetCurrentRelease(_ context.Context, _ ProductRef, id int64) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.activatedID = id
	return nil
}

func testProductRef() ProductRef { return ProductRef{TenantCode: "tenant-a", ProductID: "product-a"} }

func testProperty(identifier string, define schema.Define) schema.Property {
	return schema.Property{
		CommonParam: schema.CommonParam{Identifier: identifier, Name: identifier, RecordMode: schema.RecordModeAll},
		Mode:        schema.PropertyModeRW,
		Define:      define,
	}
}

func testDraft() schema.ProductDraft {
	ref := testProductRef()
	return schema.ProductDraft{
		Product: schema.ProductSnapshot{TenantCode: ref.TenantCode, ProductID: ref.ProductID},
		Modules: []schema.ModuleDraft{
			{ModuleID: 2, ModelModuleCode: "environment", Properties: schema.Properties{testProperty("temperature", schema.Define{Type: schema.DataTypeFloat, Min: "0", Max: "100", Start: "0", Step: "0.1"})}},
			{ModuleID: 1, ModelModuleCode: "base_product-a", Properties: schema.Properties{testProperty("enabled", schema.Define{Type: schema.DataTypeBool, Mapping: map[string]string{"0": "off", "1": "on"}})}},
		},
		Events:  schema.Events{{CommonParam: schema.CommonParam{Identifier: "alarm", Name: "Alarm"}, Type: schema.EventTypeAlert, Params: schema.Params{}}},
		Actions: schema.Actions{{CommonParam: schema.CommonParam{Identifier: "reset", Name: "Reset"}, Dir: schema.ActionDirDown, Input: schema.Params{}, Output: schema.Params{}}},
	}
}

func compiledRelease(t *testing.T, id, releaseNo int64, draft schema.ProductDraft) Release {
	t.Helper()
	bundle, err := schema.CompileSchemaBundle(draft)
	if err != nil {
		t.Fatalf("compile bundle: %v", err)
	}
	hash, err := bundle.SHA256()
	if err != nil {
		t.Fatalf("hash bundle: %v", err)
	}
	return Release{ID: id, Product: testProductRef(), ReleaseNo: releaseNo, SchemaVersion: bundle.SchemaVersion, Bundle: bundle, BundleHash: hash, Status: ReleaseStatusValid}
}

func newTestService(tx *fakeTransaction) *Service {
	return NewService(&fakeUnitOfWork{tx: tx})
}

func TestPublishRejectsProductWithoutModelModuleBinding(t *testing.T) {
	tx := &fakeTransaction{product: testDraft().Product, draft: testDraft()}
	tx.draft.Modules = nil
	_, err := newTestService(tx).Publish(context.Background(), testProductRef(), 7)
	if !errors.Is(err, ErrMigrationRequired) {
		t.Fatalf("publish error = %v, want migration required", err)
	}
}

func TestPublishUsesPropertiesFromAllBoundModules(t *testing.T) {
	tx := &fakeTransaction{product: testDraft().Product, draft: testDraft()}
	release, err := newTestService(tx).Publish(context.Background(), testProductRef(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(release.Bundle.Model.Properties) != 2 || len(release.Bundle.PropertyRoutes) != 2 {
		t.Fatalf("published properties/routes = %d/%d, want 2/2", len(release.Bundle.Model.Properties), len(release.Bundle.PropertyRoutes))
	}
}

func TestPublishIncludesProductEventsAndActions(t *testing.T) {
	tx := &fakeTransaction{product: testDraft().Product, draft: testDraft()}
	release, err := newTestService(tx).Publish(context.Background(), testProductRef(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(release.Bundle.Model.Events) != 1 || len(release.Bundle.Model.Actions) != 1 {
		t.Fatalf("published events/actions = %d/%d, want 1/1", len(release.Bundle.Model.Events), len(release.Bundle.Model.Actions))
	}
}

func TestPublishRejectsIdentifierKindReuse(t *testing.T) {
	historical := testDraft()
	historical.Modules[0].Properties[0].Identifier = "alarm-v1"
	historical.Events[0].Identifier = "temperature"
	tx := &fakeTransaction{product: testDraft().Product, draft: testDraft(), history: []Release{compiledRelease(t, 1, 1, historical)}}
	_, err := newTestService(tx).Publish(context.Background(), testProductRef(), 7)
	if !errors.Is(err, ErrHistoricalConflict) {
		t.Fatalf("publish error = %v, want historical conflict", err)
	}
}

func TestPublishRejectsPublishedPropertyDefineChange(t *testing.T) {
	tx := &fakeTransaction{product: testDraft().Product, draft: testDraft(), history: []Release{compiledRelease(t, 1, 1, testDraft())}}
	tx.draft.Modules[0].Properties[0].Define.Max = "200"
	_, err := newTestService(tx).Publish(context.Background(), testProductRef(), 7)
	if !errors.Is(err, ErrHistoricalConflict) {
		t.Fatalf("publish error = %v, want historical conflict", err)
	}
}

func TestPublishAllowsLogicalPropertyRemovalWithoutChangingOldRelease(t *testing.T) {
	historical := compiledRelease(t, 1, 1, testDraft())
	draft := testDraft()
	draft.Modules[0].Properties = nil
	tx := &fakeTransaction{product: draft.Product, draft: draft, history: []Release{historical}}
	if _, err := newTestService(tx).Publish(context.Background(), testProductRef(), 7); err != nil {
		t.Fatalf("logical removal: %v", err)
	}
	if len(historical.Bundle.Model.Properties) != 2 {
		t.Fatalf("historical release was mutated: %+v", historical.Bundle.Model.Properties)
	}
}

func TestPublishRollsBackWhenInsertFails(t *testing.T) {
	want := errors.New("insert failed")
	tx := &fakeTransaction{product: testDraft().Product, draft: testDraft(), insertErr: want}
	if _, err := newTestService(tx).Publish(context.Background(), testProductRef(), 7); !errors.Is(err, want) {
		t.Fatalf("publish error = %v, want %v", err, want)
	}
	if tx.inserted != nil {
		t.Fatal("failed insert was reported as persisted")
	}
}

func TestActivateRequiresExactSameTenantAndProductRelease(t *testing.T) {
	release := compiledRelease(t, 8, 1, testDraft())
	release.Product.ProductID = "other-product"
	tx := &fakeTransaction{product: testDraft().Product, release: release}
	err := newTestService(tx).Activate(context.Background(), ReleaseRef{ProductRef: testProductRef(), ReleaseID: 8})
	if !errors.Is(err, ErrReleaseMismatch) || tx.activatedID != 0 {
		t.Fatalf("activate error/id = %v/%d, want mismatch/0", err, tx.activatedID)
	}
}

func TestReadActiveDoesNotResolveZeroToLatest(t *testing.T) {
	tx := &fakeTransaction{product: testDraft().Product, currentReleaseID: 0, release: compiledRelease(t, 9, 2, testDraft())}
	_, err := newTestService(tx).ReadActive(context.Background(), testProductRef())
	if !errors.Is(err, ErrNoActiveRelease) {
		t.Fatalf("read active error = %v, want no active release", err)
	}
}

func TestActivateRecomputesAndChecksBundleHash(t *testing.T) {
	release := compiledRelease(t, 8, 1, testDraft())
	release.BundleHash = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	tx := &fakeTransaction{product: testDraft().Product, release: release}
	err := newTestService(tx).Activate(context.Background(), ReleaseRef{ProductRef: testProductRef(), ReleaseID: 8})
	if !errors.Is(err, ErrInvalidRelease) || tx.activatedID != 0 {
		t.Fatalf("activate error/id = %v/%d, want invalid release/0", err, tx.activatedID)
	}
}
