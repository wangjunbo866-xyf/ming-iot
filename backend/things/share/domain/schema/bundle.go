package schema

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	shareerrors "gitee.com/ming-iot/share/errors"
)

const (
	SchemaBundleVersion int64 = 1
	schemaModelVersion        = "1.0"
)

// SchemaBundle is the complete, immutable product-model payload persisted by a
// release. Its slices intentionally do not use omitempty: canonical JSON uses
// [] for an empty collection.
type SchemaBundle struct {
	SchemaVersion  int64           `json:"schemaVersion"`
	Product        ProductSnapshot `json:"product"`
	Model          BundleModel     `json:"model"`
	PropertyRoutes []PropertyRoute `json:"propertyRoutes"`
}

type ProductSnapshot struct {
	TenantCode string `json:"tenantCode"`
	ProductID  string `json:"productId"`
}

type BundleModel struct {
	Version    string     `json:"version"`
	Properties Properties `json:"properties"`
	Events     Events     `json:"events"`
	Actions    Actions    `json:"actions"`
	Profile    Profile    `json:"profile"`
}

type PropertyRoute struct {
	Identifier      string `json:"identifier"`
	ModuleID        int64  `json:"moduleId"`
	ModelModuleCode string `json:"modelModuleCode"`
}

// ProductDraft is the single input to both the one-module and multi-module
// compiler paths. Events and actions remain product-level draft data.
type ProductDraft struct {
	Product ProductSnapshot `json:"product"`
	Modules []ModuleDraft   `json:"modules"`
	Events  Events          `json:"events"`
	Actions Actions         `json:"actions"`
}

// ModuleDraft contains properties only. Event and action routing is therefore
// impossible to represent accidentally.
type ModuleDraft struct {
	ModuleID        int64      `json:"moduleId"`
	ModelModuleCode string     `json:"modelModuleCode"`
	Properties      Properties `json:"properties"`
}

// CompileSchemaBundle validates and normalizes a product draft without
// mutating the caller's data.
func CompileSchemaBundle(draft ProductDraft) (SchemaBundle, error) {
	copy, err := cloneProductDraft(draft)
	if err != nil {
		return SchemaBundle{}, err
	}
	if copy.Product.TenantCode == "" {
		return SchemaBundle{}, bundleParameterErrorf("schema bundle product tenantCode is required")
	}
	if copy.Product.ProductID == "" {
		return SchemaBundle{}, bundleParameterErrorf("schema bundle product productId is required")
	}
	if len(copy.Modules) == 0 {
		return SchemaBundle{}, bundleParameterErrorf("schema bundle requires at least one model module")
	}

	sort.Slice(copy.Modules, func(i, j int) bool {
		if copy.Modules[i].ModuleID != copy.Modules[j].ModuleID {
			return copy.Modules[i].ModuleID < copy.Modules[j].ModuleID
		}
		return copy.Modules[i].ModelModuleCode < copy.Modules[j].ModelModuleCode
	})

	moduleIDs := make(map[int64]struct{}, len(copy.Modules))
	modelModuleCodes := make(map[string]struct{}, len(copy.Modules))
	identifiers := make(map[string]AffordanceType)
	properties := make(Properties, 0)
	routes := make([]PropertyRoute, 0)

	for _, module := range copy.Modules {
		if module.ModuleID <= 0 {
			return SchemaBundle{}, bundleParameterErrorf("model module id must be positive: %d", module.ModuleID)
		}
		if module.ModelModuleCode == "" {
			return SchemaBundle{}, bundleParameterErrorf("model module code is required for module %d", module.ModuleID)
		}
		if _, exists := moduleIDs[module.ModuleID]; exists {
			return SchemaBundle{}, bundleParameterErrorf("duplicate model module id: %d", module.ModuleID)
		}
		if _, exists := modelModuleCodes[module.ModelModuleCode]; exists {
			return SchemaBundle{}, bundleParameterErrorf("duplicate model module code: %s", module.ModelModuleCode)
		}
		moduleIDs[module.ModuleID] = struct{}{}
		modelModuleCodes[module.ModelModuleCode] = struct{}{}

		sort.Slice(module.Properties, func(i, j int) bool {
			return module.Properties[i].Identifier < module.Properties[j].Identifier
		})
		for _, input := range module.Properties {
			property, err := normalizeProperty(input)
			if err != nil {
				return SchemaBundle{}, bundleParameterErrorf("normalize property %q: %v", input.Identifier, err)
			}
			if previous, exists := identifiers[property.Identifier]; exists {
				return SchemaBundle{}, identifierCollisionError(property.Identifier, previous, AffordanceTypeProperty)
			}
			identifiers[property.Identifier] = AffordanceTypeProperty
			properties = append(properties, property)
			routes = append(routes, PropertyRoute{
				Identifier:      property.Identifier,
				ModuleID:        module.ModuleID,
				ModelModuleCode: module.ModelModuleCode,
			})
		}
	}

	events := copy.Events
	if events == nil {
		events = Events{}
	}
	for i := range events {
		events[i].Params = normalizeParams(events[i].Params)
		if err := events[i].ValidateWithFmt(); err != nil {
			return SchemaBundle{}, bundleParameterErrorf("validate event %q: %v", events[i].Identifier, err)
		}
		events[i].Params = normalizeParams(events[i].Params)
		if previous, exists := identifiers[events[i].Identifier]; exists {
			return SchemaBundle{}, identifierCollisionError(events[i].Identifier, previous, AffordanceTypeEvent)
		}
		identifiers[events[i].Identifier] = AffordanceTypeEvent
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Identifier < events[j].Identifier })

	actions := copy.Actions
	if actions == nil {
		actions = Actions{}
	}
	for i := range actions {
		actions[i].Input = normalizeParams(actions[i].Input)
		actions[i].Output = normalizeParams(actions[i].Output)
		if err := actions[i].ValidateWithFmt(); err != nil {
			return SchemaBundle{}, bundleParameterErrorf("validate action %q: %v", actions[i].Identifier, err)
		}
		actions[i].Input = normalizeParams(actions[i].Input)
		actions[i].Output = normalizeParams(actions[i].Output)
		if previous, exists := identifiers[actions[i].Identifier]; exists {
			return SchemaBundle{}, identifierCollisionError(actions[i].Identifier, previous, AffordanceTypeAction)
		}
		identifiers[actions[i].Identifier] = AffordanceTypeAction
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].Identifier < actions[j].Identifier })

	sort.Slice(properties, func(i, j int) bool { return properties[i].Identifier < properties[j].Identifier })
	sort.Slice(routes, func(i, j int) bool { return routes[i].Identifier < routes[j].Identifier })
	if properties == nil {
		properties = Properties{}
	}
	if routes == nil {
		routes = []PropertyRoute{}
	}

	return SchemaBundle{
		SchemaVersion: SchemaBundleVersion,
		Product:       copy.Product,
		Model: BundleModel{
			Version:    schemaModelVersion,
			Properties: properties,
			Events:     events,
			Actions:    actions,
			Profile:    Profile{ProductID: copy.Product.ProductID},
		},
		PropertyRoutes: routes,
	}, nil
}

// CanonicalBytes returns the stable wire representation used for persistence
// and hashing.
func (b SchemaBundle) CanonicalBytes() ([]byte, error) {
	copy, err := cloneSchemaBundle(b)
	if err != nil {
		return nil, err
	}
	if err := validateAndNormalizeSchemaBundle(&copy); err != nil {
		return nil, err
	}
	sort.Slice(copy.Model.Properties, func(i, j int) bool {
		return copy.Model.Properties[i].Identifier < copy.Model.Properties[j].Identifier
	})
	sort.Slice(copy.Model.Events, func(i, j int) bool {
		return copy.Model.Events[i].Identifier < copy.Model.Events[j].Identifier
	})
	sort.Slice(copy.Model.Actions, func(i, j int) bool {
		return copy.Model.Actions[i].Identifier < copy.Model.Actions[j].Identifier
	})
	sort.Slice(copy.PropertyRoutes, func(i, j int) bool {
		return copy.PropertyRoutes[i].Identifier < copy.PropertyRoutes[j].Identifier
	})
	payload, err := json.Marshal(copy)
	if err != nil {
		return nil, err
	}
	// A single terminal LF makes the canonical payload directly storable as a
	// text fixture without a hidden trim step. The LF is part of the hash.
	return append(payload, '\n'), nil
}

func (b SchemaBundle) SHA256() (string, error) {
	payload, err := b.CanonicalBytes()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// FlatModel adapts the release bundle back to the legacy flat TSL surface.
func (b SchemaBundle) FlatModel() (*Model, error) {
	payload, err := json.Marshal(b.Model)
	if err != nil {
		return nil, err
	}
	var model Model
	if err := json.Unmarshal(payload, &model); err != nil {
		return nil, err
	}
	model.init()
	return &model, nil
}

func normalizeProperty(property Property) (Property, error) {
	switch property.RecordMode {
	case 0:
		property.RecordMode = RecordModeAll
	case RecordModeAll, RecordModeAuto, RecordModeNone:
	default:
		return Property{}, bundleParameterErrorf("unsupported recordMode: %d", property.RecordMode)
	}
	property.IsNoRecord = property.RecordMode == RecordModeNone
	normalizeDefine(&property.Define)
	if err := property.ValidateWithFmt(); err != nil {
		return Property{}, err
	}
	normalizeDefine(&property.Define)
	return property, nil
}

func normalizeParams(params Params) Params {
	if params == nil {
		params = Params{}
	}
	for i := range params {
		normalizeDefine(&params[i].Define)
	}
	sort.Slice(params, func(i, j int) bool { return params[i].Identifier < params[j].Identifier })
	return params
}

func normalizeDefine(define *Define) {
	if define == nil {
		return
	}
	for i := range define.Specs {
		normalizeDefine(&define.Specs[i].DataType)
	}
	sort.Slice(define.Specs, func(i, j int) bool {
		return define.Specs[i].Identifier < define.Specs[j].Identifier
	})
	normalizeDefine(define.ArrayInfo)
}

func identifierCollisionError(identifier string, previous, current AffordanceType) error {
	return bundleParameterErrorf("schema identifier %q collides between %s and %s", identifier, previous.String(), current.String())
}

func validateAndNormalizeSchemaBundle(bundle *SchemaBundle) error {
	if bundle.SchemaVersion != SchemaBundleVersion {
		return bundleParameterErrorf("unsupported schema bundle version: %d", bundle.SchemaVersion)
	}
	if bundle.Product.TenantCode == "" || bundle.Product.ProductID == "" {
		return bundleParameterErrorf("schema bundle product tenantCode and productId are required")
	}
	if bundle.Model.Version != schemaModelVersion {
		return bundleParameterErrorf("unsupported flat model version: %q", bundle.Model.Version)
	}
	if bundle.Model.Profile.ProductID != bundle.Product.ProductID {
		return bundleParameterErrorf(
			"flat model productId %q does not match bundle productId %q",
			bundle.Model.Profile.ProductID,
			bundle.Product.ProductID,
		)
	}
	if bundle.Model.Properties == nil {
		bundle.Model.Properties = Properties{}
	}
	if bundle.Model.Events == nil {
		bundle.Model.Events = Events{}
	}
	if bundle.Model.Actions == nil {
		bundle.Model.Actions = Actions{}
	}
	if bundle.PropertyRoutes == nil {
		bundle.PropertyRoutes = []PropertyRoute{}
	}

	identifiers := make(map[string]AffordanceType, len(bundle.Model.Properties)+len(bundle.Model.Events)+len(bundle.Model.Actions))
	propertyIdentifiers := make(map[string]struct{}, len(bundle.Model.Properties))
	for i := range bundle.Model.Properties {
		property := &bundle.Model.Properties[i]
		if property.RecordMode != RecordModeAll && property.RecordMode != RecordModeAuto && property.RecordMode != RecordModeNone {
			return bundleParameterErrorf("property %q has unsupported recordMode: %d", property.Identifier, property.RecordMode)
		}
		if property.IsNoRecord != (property.RecordMode == RecordModeNone) {
			return bundleParameterErrorf("property %q has inconsistent recordMode and isNoRecord", property.Identifier)
		}
		normalizeDefine(&property.Define)
		if err := property.ValidateWithFmt(); err != nil {
			return bundleParameterErrorf("validate property %q: %v", property.Identifier, err)
		}
		normalizeDefine(&property.Define)
		if previous, exists := identifiers[property.Identifier]; exists {
			return identifierCollisionError(property.Identifier, previous, AffordanceTypeProperty)
		}
		identifiers[property.Identifier] = AffordanceTypeProperty
		propertyIdentifiers[property.Identifier] = struct{}{}
	}
	for i := range bundle.Model.Events {
		event := &bundle.Model.Events[i]
		event.Params = normalizeParams(event.Params)
		if err := event.ValidateWithFmt(); err != nil {
			return bundleParameterErrorf("validate event %q: %v", event.Identifier, err)
		}
		event.Params = normalizeParams(event.Params)
		if previous, exists := identifiers[event.Identifier]; exists {
			return identifierCollisionError(event.Identifier, previous, AffordanceTypeEvent)
		}
		identifiers[event.Identifier] = AffordanceTypeEvent
	}
	for i := range bundle.Model.Actions {
		action := &bundle.Model.Actions[i]
		action.Input = normalizeParams(action.Input)
		action.Output = normalizeParams(action.Output)
		if err := action.ValidateWithFmt(); err != nil {
			return bundleParameterErrorf("validate action %q: %v", action.Identifier, err)
		}
		action.Input = normalizeParams(action.Input)
		action.Output = normalizeParams(action.Output)
		if previous, exists := identifiers[action.Identifier]; exists {
			return identifierCollisionError(action.Identifier, previous, AffordanceTypeAction)
		}
		identifiers[action.Identifier] = AffordanceTypeAction
	}

	if len(bundle.PropertyRoutes) != len(bundle.Model.Properties) {
		return bundleParameterErrorf(
			"property route count %d does not match property count %d",
			len(bundle.PropertyRoutes),
			len(bundle.Model.Properties),
		)
	}
	routed := make(map[string]struct{}, len(bundle.PropertyRoutes))
	modelModuleCodesByID := make(map[int64]string)
	moduleIDsByModelModuleCode := make(map[string]int64)
	for _, route := range bundle.PropertyRoutes {
		if _, exists := propertyIdentifiers[route.Identifier]; !exists {
			return bundleParameterErrorf("property route %q has no matching property", route.Identifier)
		}
		if _, exists := routed[route.Identifier]; exists {
			return bundleParameterErrorf("duplicate property route: %s", route.Identifier)
		}
		if route.ModuleID <= 0 || route.ModelModuleCode == "" {
			return bundleParameterErrorf("property route %q requires moduleId and modelModuleCode", route.Identifier)
		}
		if modelModuleCode, exists := modelModuleCodesByID[route.ModuleID]; exists && modelModuleCode != route.ModelModuleCode {
			return bundleParameterErrorf(
				"moduleId %d maps to conflicting model module codes %q and %q",
				route.ModuleID,
				modelModuleCode,
				route.ModelModuleCode,
			)
		}
		if moduleID, exists := moduleIDsByModelModuleCode[route.ModelModuleCode]; exists && moduleID != route.ModuleID {
			return bundleParameterErrorf(
				"modelModuleCode %q maps to conflicting module ids %d and %d",
				route.ModelModuleCode,
				moduleID,
				route.ModuleID,
			)
		}
		routed[route.Identifier] = struct{}{}
		modelModuleCodesByID[route.ModuleID] = route.ModelModuleCode
		moduleIDsByModelModuleCode[route.ModelModuleCode] = route.ModuleID
	}
	return nil
}

func bundleParameterErrorf(format string, args ...any) error {
	return shareerrors.Parameter.WithMsgf(format, args...)
}

func cloneProductDraft(draft ProductDraft) (ProductDraft, error) {
	payload, err := json.Marshal(draft)
	if err != nil {
		return ProductDraft{}, fmt.Errorf("copy product draft: %w", err)
	}
	var copy ProductDraft
	if err := json.Unmarshal(payload, &copy); err != nil {
		return ProductDraft{}, fmt.Errorf("copy product draft: %w", err)
	}
	return copy, nil
}

func cloneSchemaBundle(bundle SchemaBundle) (SchemaBundle, error) {
	payload, err := json.Marshal(bundle)
	if err != nil {
		return SchemaBundle{}, fmt.Errorf("copy schema bundle: %w", err)
	}
	var copy SchemaBundle
	if err := json.Unmarshal(payload, &copy); err != nil {
		return SchemaBundle{}, fmt.Errorf("copy schema bundle: %w", err)
	}
	return copy, nil
}
