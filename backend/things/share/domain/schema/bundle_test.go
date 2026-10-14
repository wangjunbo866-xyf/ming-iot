package schema

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCompileSchemaBundleIsByteStable(t *testing.T) {
	draft := schemaBundleTestDraft()
	original, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}

	first, err := CompileSchemaBundle(draft)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompileSchemaBundle(schemaBundleEquivalentDraft())
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := first.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(firstBytes, []byte(`"modelModuleCode"`)) {
		t.Fatalf("canonical bundle missing modelModuleCode: %s", firstBytes)
	}
	if bytes.Contains(firstBytes, []byte(`"moduleCode"`)) {
		t.Fatalf("canonical bundle still exposes ambiguous moduleCode: %s", firstBytes)
	}
	if !bytes.Contains(firstBytes, []byte(`"version":"1.0"`)) {
		t.Fatalf("canonical bundle model version changed: %s", firstBytes)
	}
	secondBytes, err := second.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("semantic equivalents produced different bytes:\n%s\n%s", firstBytes, secondBytes)
	}
	firstHash, err := first.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := second.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("semantic equivalents produced different hashes: %s != %s", firstHash, secondHash)
	}
	assertNestedAffordanceOrderingIsStable(t)
	after, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, after) {
		t.Fatalf("CompileSchemaBundle mutated its input:\n%s\n%s", original, after)
	}
	assertSchemaBundleGolden(t, firstBytes, firstHash)

	nilDraft := ProductDraft{
		Product: ProductSnapshot{TenantCode: "tenant-a", ProductID: "product-a"},
		Modules: []ModuleDraft{{ModuleID: 1, ModelModuleCode: "base", Properties: nil}},
	}
	emptyDraft := ProductDraft{
		Product: ProductSnapshot{TenantCode: "tenant-a", ProductID: "product-a"},
		Modules: []ModuleDraft{{ModuleID: 1, ModelModuleCode: "base", Properties: Properties{}}},
		Events:  Events{},
		Actions: Actions{},
	}
	nilBundle, err := CompileSchemaBundle(nilDraft)
	if err != nil {
		t.Fatal(err)
	}
	emptyBundle, err := CompileSchemaBundle(emptyDraft)
	if err != nil {
		t.Fatal(err)
	}
	nilBytes, _ := nilBundle.CanonicalBytes()
	emptyBytes, _ := emptyBundle.CanonicalBytes()
	if !bytes.Equal(nilBytes, emptyBytes) {
		t.Fatalf("nil and empty collections differ:\n%s\n%s", nilBytes, emptyBytes)
	}
	for _, field := range []string{`"properties":[]`, `"events":[]`, `"actions":[]`, `"propertyRoutes":[]`} {
		if !bytes.Contains(nilBytes, []byte(field)) {
			t.Fatalf("canonical bundle must encode %s as an empty array: %s", field, nilBytes)
		}
	}

	nestedNil := ProductDraft{
		Product: ProductSnapshot{TenantCode: "tenant-a", ProductID: "product-a"},
		Modules: []ModuleDraft{{ModuleID: 1, ModelModuleCode: "base", Properties: Properties{}}},
		Events: Events{{
			CommonParam: CommonParam{Identifier: "notice", Name: "Notice"},
			Type:        EventTypeInfo,
		}},
		Actions: Actions{{
			CommonParam: CommonParam{Identifier: "ping", Name: "Ping"},
			Dir:         ActionDirDown,
		}},
	}
	nestedEmpty := ProductDraft{
		Product: ProductSnapshot{TenantCode: "tenant-a", ProductID: "product-a"},
		Modules: []ModuleDraft{{ModuleID: 1, ModelModuleCode: "base", Properties: Properties{}}},
		Events: Events{{
			CommonParam: CommonParam{Identifier: "notice", Name: "Notice"},
			Type:        EventTypeInfo,
			Params:      Params{},
		}},
		Actions: Actions{{
			CommonParam: CommonParam{Identifier: "ping", Name: "Ping"},
			Dir:         ActionDirDown,
			Input:       Params{},
			Output:      Params{},
		}},
	}
	nilNestedBundle, err := CompileSchemaBundle(nestedNil)
	if err != nil {
		t.Fatal(err)
	}
	emptyNestedBundle, err := CompileSchemaBundle(nestedEmpty)
	if err != nil {
		t.Fatal(err)
	}
	nilNestedBytes, _ := nilNestedBundle.CanonicalBytes()
	emptyNestedBytes, _ := emptyNestedBundle.CanonicalBytes()
	if !bytes.Equal(nilNestedBytes, emptyNestedBytes) {
		t.Fatalf("nested nil and empty collections differ:\n%s\n%s", nilNestedBytes, emptyNestedBytes)
	}
}

func TestCompileSchemaBundleUsesSamePathForOneAndManyModules(t *testing.T) {
	draft := schemaBundleTestDraft()
	one, err := CompileSchemaBundle(ProductDraft{
		Product: draft.Product,
		Modules: draft.Modules[:1],
		Events:  draft.Events,
		Actions: draft.Actions,
	})
	if err != nil {
		t.Fatal(err)
	}
	many, err := CompileSchemaBundle(draft)
	if err != nil {
		t.Fatal(err)
	}
	if len(one.PropertyRoutes) != 1 || one.PropertyRoutes[0].ModuleID != draft.Modules[0].ModuleID {
		t.Fatalf("single-module property route missing: %#v", one.PropertyRoutes)
	}
	if len(many.PropertyRoutes) != 2 {
		t.Fatalf("multi-module route count = %d, want 2", len(many.PropertyRoutes))
	}
	if got := []string{many.Model.Properties[0].Identifier, many.Model.Properties[1].Identifier}; !reflect.DeepEqual(got, []string{"status", "temperature"}) {
		t.Fatalf("flat properties not sorted through the common compiler path: %v", got)
	}
}

func TestSingleModuleFlatModeStillCarriesAllRoutesInBundle(t *testing.T) {
	draft := schemaBundleTestDraft()
	draft.Modules[0].Properties = append(draft.Modules[0].Properties, draft.Modules[1].Properties...)
	draft.Modules = draft.Modules[:1]
	bundle, err := CompileSchemaBundle(draft)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.PropertyRoutes) != len(bundle.Model.Properties) || len(bundle.PropertyRoutes) != 2 {
		t.Fatalf("routes=%+v properties=%+v", bundle.PropertyRoutes, bundle.Model.Properties)
	}
	for _, route := range bundle.PropertyRoutes {
		if route.ModuleID != draft.Modules[0].ModuleID || route.ModelModuleCode != draft.Modules[0].ModelModuleCode {
			t.Fatalf("single-module route lost its module identity: %+v", route)
		}
	}
}

func TestCompileSchemaBundleRoutesPropertiesOnly(t *testing.T) {
	bundle, err := CompileSchemaBundle(schemaBundleTestDraft())
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.PropertyRoutes) != len(bundle.Model.Properties) {
		t.Fatalf("routes = %d, properties = %d", len(bundle.PropertyRoutes), len(bundle.Model.Properties))
	}
	for _, route := range bundle.PropertyRoutes {
		if route.Identifier == "alarm" || route.Identifier == "reset" {
			t.Fatalf("event/action received a property route: %#v", route)
		}
		if route.ModuleID == 0 || route.ModelModuleCode == "" {
			t.Fatalf("incomplete property route: %#v", route)
		}
	}
}

func TestCompileSchemaBundleRejectsCrossKindIdentifierCollision(t *testing.T) {
	tests := map[string]func(*ProductDraft){
		"property-event": func(draft *ProductDraft) {
			draft.Events[0].Identifier = draft.Modules[0].Properties[0].Identifier
		},
		"property-action": func(draft *ProductDraft) {
			draft.Actions[0].Identifier = draft.Modules[0].Properties[0].Identifier
		},
		"event-action": func(draft *ProductDraft) {
			draft.Actions[0].Identifier = draft.Events[0].Identifier
		},
		"property-property": func(draft *ProductDraft) {
			draft.Modules[1].Properties[0].Identifier = draft.Modules[0].Properties[0].Identifier
		},
		"event-event": func(draft *ProductDraft) {
			draft.Events = append(draft.Events, draft.Events[0])
		},
		"action-action": func(draft *ProductDraft) {
			draft.Actions = append(draft.Actions, draft.Actions[0])
		},
	}
	for name, collide := range tests {
		t.Run(name, func(t *testing.T) {
			draft := schemaBundleTestDraft()
			collide(&draft)
			if _, err := CompileSchemaBundle(draft); err == nil {
				t.Fatalf("expected %s identifier collision to fail", name)
			}
		})
	}
}

func TestNormalizePropertyUsesRecordModeAsAuthority(t *testing.T) {
	draft := schemaBundleTestDraft()
	tests := []struct {
		name       string
		input      RecordMode
		want       RecordMode
		wantNoSave bool
	}{
		{name: "legacy-zero", input: 0, want: RecordModeAll},
		{name: "all", input: RecordModeAll, want: RecordModeAll},
		{name: "auto", input: RecordModeAuto, want: RecordModeAuto},
		{name: "none", input: RecordModeNone, want: RecordModeNone, wantNoSave: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := schemaBundleTestDraft()
			candidate.Modules[0].Properties[0].RecordMode = test.input
			candidate.Modules[0].Properties[0].IsNoRecord = !test.wantNoSave
			bundle, err := CompileSchemaBundle(candidate)
			if err != nil {
				t.Fatal(err)
			}
			var got Property
			for _, property := range bundle.Model.Properties {
				if property.Identifier == "temperature" {
					got = property
				}
			}
			if got.RecordMode != test.want || got.IsNoRecord != test.wantNoSave {
				t.Fatalf("normalization = (%d, %v), want (%d, %v)", got.RecordMode, got.IsNoRecord, test.want, test.wantNoSave)
			}
		})
	}
	if got := DoToPropertyAffordance(&draft.Modules[0].Properties[0]); got.IsNoRecord {
		t.Fatal("compatibility affordance allowed legacy IsNoRecord to override RecordMode=0")
	}
	if got := DoToPropertyAffordance(&draft.Modules[1].Properties[0]); !got.IsNoRecord {
		t.Fatal("compatibility affordance did not derive IsNoRecord from RecordModeNone")
	}

	draft.Modules[0].Properties[0].RecordMode = 99
	if _, err := CompileSchemaBundle(draft); err == nil {
		t.Fatal("expected invalid RecordMode to fail")
	}
}

func assertNestedAffordanceOrderingIsStable(t *testing.T) {
	t.Helper()
	first := schemaBundleTestDraft()
	first.Events[0].Params = append(first.Events[0].Params, Param{
		Identifier: "code",
		Name:       "Code",
		Define:     Define{Type: DataTypeInt, Min: "0", Max: "10"},
	})
	first.Events = append(first.Events, Event{
		CommonParam: CommonParam{Identifier: "heartbeat", Name: "Heartbeat"},
		Type:        EventTypeInfo,
		Params: Params{
			{Identifier: "zeta", Name: "Zeta", Define: Define{Type: DataTypeString, Max: "8"}},
			{Identifier: "alpha", Name: "Alpha", Define: Define{Type: DataTypeTimestamp}},
		},
	})
	first.Actions[0].Input = append(first.Actions[0].Input, Param{
		Identifier: "force",
		Name:       "Force",
		Define:     Define{Type: DataTypeBool},
	})
	first.Actions[0].Output = append(first.Actions[0].Output, Param{
		Identifier: "message",
		Name:       "Message",
		Define:     Define{Type: DataTypeString, Max: "64"},
	})
	first.Actions = append(first.Actions, Action{
		CommonParam: CommonParam{Identifier: "calibrate", Name: "Calibrate"},
		Dir:         ActionDirDown,
		Input: Params{
			{Identifier: "zeta", Name: "Zeta", Define: Define{Type: DataTypeInt, Min: "0", Max: "10"}},
			{Identifier: "alpha", Name: "Alpha", Define: Define{Type: DataTypeBool}},
		},
		Output: Params{},
	})
	second := cloneSchemaBundleDraftForTest(t, first)
	reverseEvents(second.Events)
	reverseActions(second.Actions)
	for i := range second.Events {
		reverseParams(second.Events[i].Params)
	}
	for i := range second.Actions {
		reverseParams(second.Actions[i].Input)
		reverseParams(second.Actions[i].Output)
	}

	firstBundle, err := CompileSchemaBundle(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBundle, err := CompileSchemaBundle(second)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := firstBundle.CanonicalBytes()
	secondBytes, _ := secondBundle.CanonicalBytes()
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("nested affordance ordering changed canonical bytes:\n%s\n%s", firstBytes, secondBytes)
	}
}

func cloneSchemaBundleDraftForTest(t *testing.T, draft ProductDraft) ProductDraft {
	t.Helper()
	payload, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	var clone ProductDraft
	if err := json.Unmarshal(payload, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func cloneSchemaBundleForTest(t *testing.T, bundle SchemaBundle) SchemaBundle {
	t.Helper()
	payload, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var clone SchemaBundle
	if err := json.Unmarshal(payload, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func reverseEvents(values Events) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseActions(values Actions) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseParams(values Params) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func TestReleasedBundleRebuildsExistingFlatTSLShape(t *testing.T) {
	bundle, err := CompileSchemaBundle(schemaBundleTestDraft())
	if err != nil {
		t.Fatal(err)
	}
	flat, err := bundle.FlatModel()
	if err != nil {
		t.Fatal(err)
	}
	if flat.Profile.ProductID != bundle.Product.ProductID {
		t.Fatalf("flat profile product = %q, want %q", flat.Profile.ProductID, bundle.Product.ProductID)
	}
	flatJSON, err := json.Marshal(flat)
	if err != nil {
		t.Fatal(err)
	}
	modelJSON, err := json.Marshal(bundle.Model)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(flatJSON, modelJSON) {
		t.Fatalf("flat model differs from bundle model:\n%s\n%s", flatJSON, modelJSON)
	}
}

func TestSchemaBundleRejectsPostCompileMutation(t *testing.T) {
	compiled, err := CompileSchemaBundle(schemaBundleTestDraft())
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*SchemaBundle){
		"schema-version": func(bundle *SchemaBundle) {
			bundle.SchemaVersion++
		},
		"record-mode": func(bundle *SchemaBundle) {
			bundle.Model.Properties[0].RecordMode = 99
		},
		"derived-legacy-policy": func(bundle *SchemaBundle) {
			bundle.Model.Properties[0].IsNoRecord = !bundle.Model.Properties[0].IsNoRecord
		},
		"duplicate-identifier": func(bundle *SchemaBundle) {
			bundle.Model.Events[0].Identifier = bundle.Model.Properties[0].Identifier
		},
		"missing-route": func(bundle *SchemaBundle) {
			bundle.PropertyRoutes = bundle.PropertyRoutes[:1]
		},
		"orphan-route": func(bundle *SchemaBundle) {
			bundle.PropertyRoutes[0].Identifier = "orphan"
		},
		"incomplete-route": func(bundle *SchemaBundle) {
			bundle.PropertyRoutes[0].ModuleID = 0
		},
		"module-id-to-conflicting-codes": func(bundle *SchemaBundle) {
			bundle.PropertyRoutes[0].ModuleID = bundle.PropertyRoutes[1].ModuleID
		},
		"module-code-to-conflicting-ids": func(bundle *SchemaBundle) {
			bundle.PropertyRoutes[0].ModelModuleCode = bundle.PropertyRoutes[1].ModelModuleCode
		},
		"product-mismatch": func(bundle *SchemaBundle) {
			bundle.Model.Profile.ProductID = "other-product"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			bundle := cloneSchemaBundleForTest(t, compiled)
			mutate(&bundle)
			if _, err := bundle.SHA256(); err == nil {
				t.Fatalf("expected %s mutation to prevent hashing", name)
			}
		})
	}
}

func schemaBundleTestDraft() ProductDraft {
	return ProductDraft{
		Product: ProductSnapshot{TenantCode: "tenant-a", ProductID: "product-a"},
		Modules: []ModuleDraft{
			{
				ModuleID:        20,
				ModelModuleCode: "environment",
				Properties: Properties{{
					CommonParam: CommonParam{Identifier: "temperature", Name: "Temperature", RecordMode: 0},
					Mode:        PropertyModeRW,
					IsNoRecord:  true,
					Define: Define{Type: DataTypeStruct, Specs: Specs{
						{Identifier: "zeta", Name: "Zeta", DataType: Define{Type: DataTypeBool, Mapping: map[string]string{"1": "on", "0": "off"}}},
						{Identifier: "alpha", Name: "Alpha", DataType: Define{Type: DataTypeInt, Min: "0", Max: "100", Start: "1", Step: "1"}},
					}},
				}},
			},
			{
				ModuleID:        10,
				ModelModuleCode: "status",
				Properties: Properties{{
					CommonParam: CommonParam{Identifier: "status", Name: "Status", RecordMode: RecordModeNone},
					Mode:        PropertyModeR,
					IsNoRecord:  false,
					Define:      Define{Type: DataTypeEnum, Mapping: map[string]string{"2": "warning", "1": "normal"}},
				}},
			},
		},
		Events: Events{{
			CommonParam: CommonParam{Identifier: "alarm", Name: "Alarm"},
			Type:        EventTypeAlert,
			Params: Params{{
				Identifier: "detail",
				Name:       "Detail",
				Define: Define{Type: DataTypeStruct, Specs: Specs{
					{Identifier: "zeta", Name: "Zeta", DataType: Define{Type: DataTypeString, Max: "64"}},
					{Identifier: "alpha", Name: "Alpha", DataType: Define{Type: DataTypeTimestamp}},
				}},
			}},
		}},
		Actions: Actions{{
			CommonParam: CommonParam{Identifier: "reset", Name: "Reset"},
			Dir:         ActionDirDown,
			Input: Params{{
				Identifier: "level",
				Name:       "Level",
				Define:     Define{Type: DataTypeEnum, Mapping: map[string]string{"2": "hard", "1": "soft"}},
			}},
			Output: Params{{
				Identifier: "accepted",
				Name:       "Accepted",
				Define:     Define{Type: DataTypeBool, Mapping: map[string]string{"1": "yes", "0": "no"}},
			}},
		}},
	}
}

func schemaBundleEquivalentDraft() ProductDraft {
	draft := schemaBundleTestDraft()
	draft.Modules[0], draft.Modules[1] = draft.Modules[1], draft.Modules[0]
	draft.Modules[1].Properties[0].Define.Specs[0], draft.Modules[1].Properties[0].Define.Specs[1] =
		draft.Modules[1].Properties[0].Define.Specs[1], draft.Modules[1].Properties[0].Define.Specs[0]
	draft.Modules[0].Properties[0].Define.Mapping = map[string]string{"1": "normal", "2": "warning"}
	draft.Events[0].Params[0].Define.Specs[0], draft.Events[0].Params[0].Define.Specs[1] =
		draft.Events[0].Params[0].Define.Specs[1], draft.Events[0].Params[0].Define.Specs[0]
	draft.Actions[0].Input[0].Define.Mapping = map[string]string{"1": "soft", "2": "hard"}
	return draft
}

func assertSchemaBundleGolden(t *testing.T, got []byte, gotHash string) {
	t.Helper()
	want, err := os.ReadFile("testdata/schema_bundle_v1.golden.json")
	if err != nil {
		t.Fatalf("read golden: %v; canonical=%s; sha256=%s", err, got, gotHash)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("canonical bundle differs from golden:\n got: %s\nwant: %s", got, want)
	}
	wantHash, err := os.ReadFile("testdata/schema_bundle_v1.sha256")
	if err != nil {
		t.Fatalf("read golden hash: %v; sha256=%s", err, gotHash)
	}
	if gotHash != strings.TrimSpace(string(wantHash)) {
		t.Fatalf("bundle hash = %s, want %s", gotHash, strings.TrimSpace(string(wantHash)))
	}
}
