package relationDB

import (
	"encoding/json"

	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/def"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/internal/domain/shadow"
	"gitee.com/ming-iot/things/share/domain/schema"
)

func ToPropertyPo(in *schema.Property) DmSchemaCore {
	recordMode := in.RecordMode
	if recordMode == 0 {
		recordMode = schema.RecordModeAll
	}
	defineStr, _ := json.Marshal(schema.DoToPropertyAffordance(in))
	return DmSchemaCore{
		Type:              schema.AffordanceTypeProperty,
		Name:              in.Name,
		ExtendConfig:      in.ExtendConfig,
		Desc:              in.Desc,
		Required:          def.ToIntBool[int64](in.Required),
		Affordance:        string(defineStr),
		Tag:               in.Tag,
		IsCanSceneLinkage: in.IsCanSceneLinkage,
		FuncGroup:         in.FuncGroup,
		ControlMode:       in.ControlMode,
		UserPerm:          in.UserPerm,
		RecordMode:        recordMode,
		IsPassword:        int64(in.IsPassword),
		Order:             in.Order,
	}
}

func ToModelModulePropertyPo(tenantCode string, moduleID int64, in *schema.Property) *DmModelModuleProperty {
	core := ToPropertyPo(in)
	return &DmModelModuleProperty{
		TenantCode:        dataType.TenantCode(tenantCode),
		ModuleID:          moduleID,
		Identifier:        in.Identifier,
		Tag:               core.Tag,
		Type:              core.Type,
		ExtendConfig:      core.ExtendConfig,
		Required:          core.Required,
		Name:              core.Name,
		Desc:              core.Desc,
		IsCanSceneLinkage: core.IsCanSceneLinkage,
		FuncGroup:         core.FuncGroup,
		ControlMode:       core.ControlMode,
		UserPerm:          core.UserPerm,
		RecordMode:        core.RecordMode,
		IsPassword:        core.IsPassword,
		Affordance:        core.Affordance,
		Order:             core.Order,
	}
}

func ToModelModulePropertyDo(in *DmModelModuleProperty) *schema.Property {
	return ToPropertyDo(in.Identifier, &DmSchemaCore{
		Tag:               in.Tag,
		Type:              in.Type,
		ExtendConfig:      in.ExtendConfig,
		Required:          in.Required,
		Name:              in.Name,
		Desc:              in.Desc,
		IsCanSceneLinkage: in.IsCanSceneLinkage,
		FuncGroup:         in.FuncGroup,
		ControlMode:       in.ControlMode,
		UserPerm:          in.UserPerm,
		RecordMode:        in.RecordMode,
		IsPassword:        in.IsPassword,
		Affordance:        in.Affordance,
		Order:             in.Order,
	})
}

func ToCommonParam(identifier string, in *DmSchemaCore) schema.CommonParam {
	return schema.CommonParam{
		Identifier:        identifier,
		Name:              in.Name,
		Desc:              in.Desc,
		ExtendConfig:      in.ExtendConfig,
		Required:          def.ToBool(in.Required),
		IsCanSceneLinkage: in.IsCanSceneLinkage,
		FuncGroup:         in.FuncGroup, // 功能分类: 1:普通功能 2:系统功能
		ControlMode:       in.ControlMode,
		UserPerm:          in.UserPerm, //用户权限操作: r(只读) rw(可读可写)
		RecordMode:        in.RecordMode,
		IsPassword:        in.IsPassword,
		Tag:               in.Tag,
	}
}

func ToPropertyDo(identifier string, in *DmSchemaCore) *schema.Property {
	affordance := schema.PropertyAffordance{}
	_ = json.Unmarshal([]byte(in.Affordance), &affordance)
	do := &schema.Property{
		CommonParam: ToCommonParam(identifier, in),
		Define:      affordance.Define,
		Mode:        affordance.Mode,
		IsUseShadow: affordance.IsUseShadow,
		IsNoRecord:  affordance.IsNoRecord,
	}
	if do.RecordMode == 0 {
		do.RecordMode = schema.RecordModeAll
	}
	do.IsNoRecord = do.RecordMode == schema.RecordModeNone
	newAffordance, _ := json.Marshal(affordance)
	in.Affordance = string(newAffordance)
	do.ValidateWithFmt()
	return do
}

func ToEventPo(in *schema.Event) DmSchemaCore {
	define := schema.EventAffordance{
		Type:   in.Type,
		Params: in.Params,
	}
	defineStr, _ := json.Marshal(define)
	return DmSchemaCore{
		Type:              schema.AffordanceTypeEvent,
		Name:              in.Name,
		Desc:              in.Desc,
		ExtendConfig:      in.ExtendConfig,
		Required:          def.ToIntBool[int64](in.Required),
		Affordance:        string(defineStr),
		RecordMode:        in.RecordMode,
		FuncGroup:         in.FuncGroup,
		IsPassword:        in.IsPassword,
		ControlMode:       in.ControlMode,
		UserPerm:          in.UserPerm,
		IsCanSceneLinkage: in.IsCanSceneLinkage,
		//IsShareAuthPerm:   in.IsShareAuthPerm,
		Tag: in.Tag,
	}
}

func ToEventDo(identifier string, in *DmSchemaCore) *schema.Event {
	affordance := schema.EventAffordance{}
	_ = json.Unmarshal([]byte(in.Affordance), &affordance)
	do := &schema.Event{
		CommonParam: ToCommonParam(identifier, in),
		Type:        affordance.Type,
		Params:      affordance.Params,
	}
	newAffordance, _ := json.Marshal(affordance)
	in.Affordance = string(newAffordance)
	do.ValidateWithFmt()
	return do
}

func ToActionPo(in *schema.Action) DmSchemaCore {
	define := schema.ActionAffordance{
		Input:  in.Input,
		Output: in.Output,
	}
	defineStr, _ := json.Marshal(define)
	return DmSchemaCore{
		Type:              schema.AffordanceTypeAction,
		Name:              in.Name,
		ExtendConfig:      in.ExtendConfig,
		Desc:              in.Desc,
		Required:          def.ToIntBool[int64](in.Required),
		Affordance:        string(defineStr),
		RecordMode:        in.RecordMode,
		IsCanSceneLinkage: in.IsCanSceneLinkage,
		FuncGroup:         in.FuncGroup,
		ControlMode:       in.ControlMode,
		IsPassword:        in.IsPassword,
		UserPerm:          in.UserPerm,
		Tag:               in.Tag,
	}
}

func ToAffordancePo(in any) string {
	var defineStr []byte
	switch in.(type) {
	case *schema.Event:
		af := in.(*schema.Event)
		define := schema.EventAffordance{
			Type:   af.Type,
			Params: af.Params,
		}
		defineStr, _ = json.Marshal(define)
	case *schema.Action:
		af := in.(*schema.Action)
		define := schema.ActionAffordance{
			Input:  af.Input,
			Output: af.Output,
		}
		defineStr, _ = json.Marshal(define)
	case *schema.Property:
		af := in.(*schema.Property)
		define := schema.PropertyAffordance{
			Define:      af.Define,
			Mode:        af.Mode,
			IsUseShadow: af.IsUseShadow,
			IsNoRecord:  af.IsNoRecord,
		}
		defineStr, _ = json.Marshal(define)
	}
	return string(defineStr)
}

func ToActionDo(identifier string, in *DmSchemaCore) *schema.Action {
	affordance := schema.ActionAffordance{}
	_ = json.Unmarshal([]byte(in.Affordance), &affordance)
	do := &schema.Action{
		CommonParam: ToCommonParam(identifier, in),
		Input:       affordance.Input,
		Output:      affordance.Output,
		Dir:         affordance.Dir,
	}
	newAffordance, _ := json.Marshal(affordance)
	in.Affordance = string(newAffordance)
	do.ValidateWithFmt()
	return do
}

func ToSchemaDo(productID string, in []*DmSchemaInfo) *schema.Model {
	model := schema.Model{
		Profile: schema.Profile{ProductID: productID},
	}
	if len(in) == 0 {
		return &model
	}
	for _, v := range in {
		switch schema.AffordanceType(v.Type) {
		case schema.AffordanceTypeProperty:
			model.Properties = append(model.Properties, *ToPropertyDo(v.Identifier, &v.DmSchemaCore))
		case schema.AffordanceTypeEvent:
			model.Events = append(model.Events, *ToEventDo(v.Identifier, &v.DmSchemaCore))
		case schema.AffordanceTypeAction:
			model.Actions = append(model.Actions, *ToActionDo(v.Identifier, &v.DmSchemaCore))
		}
	}
	model.ValidateWithFmt()
	return &model
}

func ToDeviceSchemaDo(productID string, in []*DmDeviceSchema) *schema.Model {
	model := schema.Model{
		Profile: schema.Profile{ProductID: productID},
	}
	if len(in) == 0 {
		return &model
	}
	for _, v := range in {
		switch schema.AffordanceType(v.Type) {
		case schema.AffordanceTypeProperty:
			model.Properties = append(model.Properties, *ToPropertyDo(v.Identifier, &v.DmSchemaCore))
		case schema.AffordanceTypeEvent:
			model.Events = append(model.Events, *ToEventDo(v.Identifier, &v.DmSchemaCore))
		case schema.AffordanceTypeAction:
			model.Actions = append(model.Actions, *ToActionDo(v.Identifier, &v.DmSchemaCore))
		}
	}
	model.ValidateWithFmt()
	return &model
}

func CommonToSchemaDo(in []*DmCommonSchema) *schema.Model {
	model := schema.Model{}
	if len(in) == 0 {
		return &model
	}
	for _, v := range in {
		switch schema.AffordanceType(v.Type) {
		case schema.AffordanceTypeProperty:
			model.Properties = append(model.Properties, *ToPropertyDo(v.Identifier, &v.DmSchemaCore))
		case schema.AffordanceTypeEvent:
			model.Events = append(model.Events, *ToEventDo(v.Identifier, &v.DmSchemaCore))
		case schema.AffordanceTypeAction:
			model.Actions = append(model.Actions, *ToActionDo(v.Identifier, &v.DmSchemaCore))
		}
	}
	model.ValidateWithFmt()
	return &model
}

func ToShadowPo(info *shadow.Info) *DmDeviceShadow {
	return &DmDeviceShadow{
		ID:                info.ID,
		ProductID:         info.ProductID,
		DeviceName:        info.DeviceName,
		UpdatedDeviceTime: utils.TimeToNullTime(info.UpdatedDeviceTime),
		DataID:            info.DataID,
		Value:             info.Value,
	}
}
func ToShadowDo(in *DmDeviceShadow) *shadow.Info {
	return &shadow.Info{
		ID:                in.ID,
		ProductID:         in.ProductID,
		DeviceName:        in.DeviceName,
		DataID:            in.DataID,
		Value:             in.Value,
		UpdatedDeviceTime: utils.NullTimeToTime(in.UpdatedDeviceTime),
		CreatedTime:       in.CreatedTime,
		UpdatedTime:       in.UpdatedTime,
	}
}
func ToShadowsDo(in []*DmDeviceShadow) []*shadow.Info {
	if in == nil {
		return nil
	}
	var ret []*shadow.Info
	for _, v := range in {
		ret = append(ret, ToShadowDo(v))
	}
	return ret
}
