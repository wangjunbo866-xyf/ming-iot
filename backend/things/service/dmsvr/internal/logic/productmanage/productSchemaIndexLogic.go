package productmanagelogic

import (
	"cmp"
	"context"
	"encoding/json"
	"sort"
	"strings"

	"gitee.com/ming-iot/saas/share/dataType"
	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/stores"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/internal/logic"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"
	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
	"gitee.com/ming-iot/things/share/domain/schema"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProductSchemaIndexLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	composeEditableModel func(context.Context, dataType.TenantCode, string) (*schema.Model, error)
}

func NewProductSchemaIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductSchemaIndexLogic {
	return &ProductSchemaIndexLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		composeEditableModel: func(ctx context.Context, tenant dataType.TenantCode, productID string) (*schema.Model, error) {
			return relationDB.NewProductDraftComposer(ctx).ComposeEditableModel(ctx, tenant, productID)
		},
	}
}

// 获取产品信息列表
func (l *ProductSchemaIndexLogic) ProductSchemaIndex(in *dm.ProductSchemaIndexReq) (*dm.ProductSchemaIndexResp, error) {
	if in == nil || in.ProductID == "" {
		return nil, projecterrors.Parameter.AddDetail("productID is required")
	}
	l.Infof("%s req=%v", utils.FuncName(), utils.Fmt(in))
	tenant := requestTenant(l.ctx)
	model, err := l.composeEditableModel(l.ctx, tenant, in.ProductID)
	if err != nil {
		return nil, err
	}
	if model == nil {
		return nil, projecterrors.System.AddDetail("editable product model is nil")
	}
	entries := flattenEditableSchemas(tenant, in.ProductID, model)
	filtered := entries[:0]
	for _, entry := range entries {
		if matchesProductSchemaIndex(entry, in) {
			filtered = append(filtered, entry)
		}
	}
	page := logic.ToPageInfo(in.Page).WithDefaultOrder(stores.OrderBy{
		Field: "order",
		Sort:  stores.OrderAsc,
	})
	sortEditableSchemas(filtered, page.Orders)
	total := int64(len(filtered))
	start := min(int(page.GetOffset()), len(filtered))
	end := min(start+int(page.GetLimit()), len(filtered))
	return &dm.ProductSchemaIndexResp{List: filtered[start:end], Total: total}, nil
}

func flattenEditableSchemas(tenant dataType.TenantCode, productID string, model *schema.Model) []*dm.ProductSchemaInfo {
	result := make([]*dm.ProductSchemaInfo, 0, len(model.Properties)+len(model.Events)+len(model.Actions))
	appendCore := func(identifier string, core relationDB.DmSchemaCore) {
		result = append(result, utils.Copy[dm.ProductSchemaInfo](&relationDB.DmProductSchema{
			TenantCode:   dataType.TenantCodeWithDefaultR(tenant),
			ProductID:    productID,
			Identifier:   identifier,
			DmSchemaCore: core,
		}))
	}
	for i := range model.Properties {
		appendCore(model.Properties[i].Identifier, relationDB.ToPropertyPo(&model.Properties[i]))
	}
	for i := range model.Events {
		appendCore(model.Events[i].Identifier, relationDB.ToEventPo(&model.Events[i]))
	}
	for i := range model.Actions {
		appendCore(model.Actions[i].Identifier, relationDB.ToActionPo(&model.Actions[i]))
	}
	return result
}

func matchesProductSchemaIndex(info *dm.ProductSchemaInfo, in *dm.ProductSchemaIndexReq) bool {
	if info == nil || info.ProductID != in.ProductID || info.Tag < int64(schema.TagCustom) || info.Tag > int64(schema.TagRequired) {
		return false
	}
	if in.Type != 0 && info.Type != in.Type || len(in.Types) != 0 && !containsInt64(in.Types, info.Type) {
		return false
	}
	if in.Tag != 0 && info.Tag != in.Tag || len(in.Identifiers) != 0 && !containsString(in.Identifiers, info.Identifier) {
		return false
	}
	if in.Name != "" && !strings.Contains(info.GetName().GetValue(), in.Name) || in.Identifier != "" && !strings.Contains(info.Identifier, in.Identifier) {
		return false
	}
	if in.IsCanSceneLinkage != 0 && info.IsCanSceneLinkage != in.IsCanSceneLinkage || in.FuncGroup != 0 && info.FuncGroup != in.FuncGroup || in.UserPerm != 0 && info.UserPerm != in.UserPerm || in.ControlMode != 0 && info.ControlMode != in.ControlMode {
		return false
	}
	if in.PropertyMode != "" {
		if info.Type != int64(schema.AffordanceTypeProperty) {
			return false
		}
		var affordance schema.PropertyAffordance
		if err := json.Unmarshal([]byte(info.GetAffordance().GetValue()), &affordance); err != nil || string(affordance.Mode) != in.PropertyMode {
			return false
		}
	}
	return true
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sortEditableSchemas(entries []*dm.ProductSchemaInfo, orders []stores.OrderBy) {
	sort.SliceStable(entries, func(i, j int) bool {
		for _, order := range orders {
			comparison := compareEditableSchema(entries[i], entries[j], order.Field)
			if comparison == 0 {
				continue
			}
			if order.Sort == stores.OrderDesc {
				return comparison > 0
			}
			return comparison < 0
		}
		return cmp.Compare(entries[i].Identifier, entries[j].Identifier) < 0
	})
}

func compareEditableSchema(left, right *dm.ProductSchemaInfo, field string) int {
	switch strings.ToLower(strings.ReplaceAll(field, "_", "")) {
	case "productid":
		return cmp.Compare(left.ProductID, right.ProductID)
	case "type":
		return cmp.Compare(left.Type, right.Type)
	case "tag":
		return cmp.Compare(left.Tag, right.Tag)
	case "identifier":
		return cmp.Compare(left.Identifier, right.Identifier)
	case "name":
		return cmp.Compare(left.GetName().GetValue(), right.GetName().GetValue())
	case "required":
		return cmp.Compare(left.Required, right.Required)
	case "iscanscenelinkage":
		return cmp.Compare(left.IsCanSceneLinkage, right.IsCanSceneLinkage)
	case "funcgroup":
		return cmp.Compare(left.FuncGroup, right.FuncGroup)
	case "userperm":
		return cmp.Compare(left.UserPerm, right.UserPerm)
	case "controlmode":
		return cmp.Compare(left.ControlMode, right.ControlMode)
	case "recordmode":
		return cmp.Compare(left.RecordMode, right.RecordMode)
	case "ispassword":
		return cmp.Compare(left.IsPassword, right.IsPassword)
	default:
		return cmp.Compare(left.Order, right.Order)
	}
}
