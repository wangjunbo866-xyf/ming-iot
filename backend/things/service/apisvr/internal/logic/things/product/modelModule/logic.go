package modelModule

import (
	"context"

	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/things/service/apisvr/internal/logic"
	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
	"github.com/zeromicro/go-zero/core/logx"
)

type baseLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func newBaseLogic(ctx context.Context, svcCtx *svc.ServiceContext) *baseLogic {
	return &baseLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *baseLogic) rpcError(operation string, request any, err error) error {
	if err == nil {
		return nil
	}
	mapped := projecterrors.Fmt(err)
	l.Errorf("%s request=%v err=%+v", operation, request, mapped)
	return mapped
}

func moduleInfo(in *dm.ModelModule) *types.ModelModuleInfo {
	if in == nil {
		return nil
	}
	return &types.ModelModuleInfo{
		ModelModuleID:   in.ModelModuleID,
		ModelModuleCode: in.ModelModuleCode,
		Name:            in.Name,
		Description:     in.Description,
	}
}

func propertyRPC(in *types.ModelModulePropertyInfo) *dm.ModelModuleProperty {
	if in == nil {
		return nil
	}
	return &dm.ModelModuleProperty{
		ModelModulePropertyID: in.ModelModulePropertyID,
		ModelModuleID:         in.ModelModuleID,
		Identifier:            in.Identifier,
		Name:                  in.Name,
		Description:           in.Description,
		Tag:                   in.Tag,
		Required:              in.Required,
		IsCanSceneLinkage:     in.IsCanSceneLinkage,
		FuncGroup:             in.FuncGroup,
		UserPerm:              in.UserPerm,
		ControlMode:           in.ControlMode,
		RecordMode:            in.RecordMode,
		IsPassword:            in.IsPassword,
		Affordance:            in.Affordance,
		ExtendConfig:          in.ExtendConfig,
		Order:                 in.Order,
	}
}

func propertyInfo(in *dm.ModelModuleProperty) *types.ModelModulePropertyInfo {
	if in == nil {
		return nil
	}
	return &types.ModelModulePropertyInfo{
		ModelModulePropertyID: in.ModelModulePropertyID,
		ModelModuleID:         in.ModelModuleID,
		Identifier:            in.Identifier,
		Name:                  in.Name,
		Description:           in.Description,
		Tag:                   in.Tag,
		Required:              in.Required,
		IsCanSceneLinkage:     in.IsCanSceneLinkage,
		FuncGroup:             in.FuncGroup,
		UserPerm:              in.UserPerm,
		ControlMode:           in.ControlMode,
		RecordMode:            in.RecordMode,
		IsPassword:            in.IsPassword,
		Affordance:            in.Affordance,
		ExtendConfig:          in.ExtendConfig,
		Order:                 in.Order,
	}
}

type ModelModuleCreateLogic struct{ *baseLogic }

func NewModelModuleCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModuleCreateLogic {
	return &ModelModuleCreateLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ModelModuleCreateLogic) ModelModuleCreate(req *types.ModelModuleCreateReq) (*types.ModelModuleInfo, error) {
	resp, err := l.svcCtx.ProductM.ModelModuleCreate(l.ctx, &dm.ModelModuleCreateReq{
		ModelModuleCode: req.ModelModuleCode, Name: req.Name, Description: req.Description,
	})
	if err != nil {
		return nil, l.rpcError("ProductM.ModelModuleCreate", req, err)
	}
	return moduleInfo(resp), nil
}

type ModelModuleUpdateLogic struct{ *baseLogic }

func NewModelModuleUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModuleUpdateLogic {
	return &ModelModuleUpdateLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ModelModuleUpdateLogic) ModelModuleUpdate(req *types.ModelModuleUpdateReq) error {
	_, err := l.svcCtx.ProductM.ModelModuleUpdate(l.ctx, &dm.ModelModuleUpdateReq{
		ModelModuleID: req.ModelModuleID, ModelModuleCode: req.ModelModuleCode, Name: req.Name, Description: req.Description,
	})
	return l.rpcError("ProductM.ModelModuleUpdate", req, err)
}

type ModelModuleDeleteLogic struct{ *baseLogic }

func NewModelModuleDeleteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModuleDeleteLogic {
	return &ModelModuleDeleteLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ModelModuleDeleteLogic) ModelModuleDelete(req *types.ModelModuleDeleteReq) error {
	_, err := l.svcCtx.ProductM.ModelModuleDelete(l.ctx, &dm.ModelModuleDeleteReq{ModelModuleID: req.ModelModuleID})
	return l.rpcError("ProductM.ModelModuleDelete", req, err)
}

type ModelModuleIndexLogic struct{ *baseLogic }

func NewModelModuleIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModuleIndexLogic {
	return &ModelModuleIndexLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ModelModuleIndexLogic) ModelModuleIndex(req *types.ModelModuleIndexReq) (*types.ModelModuleIndexResp, error) {
	resp, err := l.svcCtx.ProductM.ModelModuleIndex(l.ctx, &dm.ModelModuleIndexReq{
		Page: logic.ToDmPageRpc(req.Page), ModelModuleIDs: req.ModelModuleIDs, ModelModuleCode: req.ModelModuleCode,
	})
	if err != nil {
		return nil, l.rpcError("ProductM.ModelModuleIndex", req, err)
	}
	items := make([]*types.ModelModuleInfo, 0, len(resp.List))
	for _, item := range resp.List {
		items = append(items, moduleInfo(item))
	}
	return &types.ModelModuleIndexResp{List: items, PageResp: logic.ToPageResp(req.Page, resp.Total)}, nil
}

type ProductModelModuleBindLogic struct{ *baseLogic }

func NewProductModelModuleBindLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelModuleBindLogic {
	return &ProductModelModuleBindLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ProductModelModuleBindLogic) ProductModelModuleBind(req *types.ProductModelModuleBindReq) error {
	_, err := l.svcCtx.ProductM.ProductModelModuleBind(l.ctx, &dm.ProductModelModuleBindReq{
		ProductID: req.ProductID, ModelModuleID: req.ModelModuleID, Order: req.Order,
	})
	return l.rpcError("ProductM.ProductModelModuleBind", req, err)
}

type ProductModelModuleUnbindLogic struct{ *baseLogic }

func NewProductModelModuleUnbindLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelModuleUnbindLogic {
	return &ProductModelModuleUnbindLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ProductModelModuleUnbindLogic) ProductModelModuleUnbind(req *types.ProductModelModuleUnbindReq) error {
	_, err := l.svcCtx.ProductM.ProductModelModuleUnbind(l.ctx, &dm.ProductModelModuleUnbindReq{
		ProductID: req.ProductID, ModelModuleID: req.ModelModuleID,
	})
	return l.rpcError("ProductM.ProductModelModuleUnbind", req, err)
}

type ProductModelModuleIndexLogic struct{ *baseLogic }

func NewProductModelModuleIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductModelModuleIndexLogic {
	return &ProductModelModuleIndexLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ProductModelModuleIndexLogic) ProductModelModuleIndex(req *types.ProductModelModuleIndexReq) (*types.ProductModelModuleIndexResp, error) {
	resp, err := l.svcCtx.ProductM.ProductModelModuleIndex(l.ctx, &dm.ProductModelModuleIndexReq{
		Page: logic.ToDmPageRpc(req.Page), ProductID: req.ProductID,
	})
	if err != nil {
		return nil, l.rpcError("ProductM.ProductModelModuleIndex", req, err)
	}
	items := make([]*types.ProductModelModuleInfo, 0, len(resp.List))
	for _, item := range resp.List {
		items = append(items, &types.ProductModelModuleInfo{
			BindingID: item.BindingID, ProductID: item.ProductID, ModelModuleID: item.ModelModuleID,
			Order: item.Order, ModelModule: moduleInfo(item.ModelModule),
		})
	}
	return &types.ProductModelModuleIndexResp{List: items, PageResp: logic.ToPageResp(req.Page, resp.Total)}, nil
}

type ModelModulePropertyCreateLogic struct{ *baseLogic }

func NewModelModulePropertyCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModulePropertyCreateLogic {
	return &ModelModulePropertyCreateLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ModelModulePropertyCreateLogic) ModelModulePropertyCreate(req *types.ModelModulePropertyCreateReq) (*types.ModelModulePropertyInfo, error) {
	resp, err := l.svcCtx.ProductM.ModelModulePropertyCreate(l.ctx, &dm.ModelModulePropertyCreateReq{Property: propertyRPC(req.ModelModulePropertyInfo)})
	if err != nil {
		return nil, l.rpcError("ProductM.ModelModulePropertyCreate", req, err)
	}
	return propertyInfo(resp), nil
}

type ModelModulePropertyUpdateLogic struct{ *baseLogic }

func NewModelModulePropertyUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModulePropertyUpdateLogic {
	return &ModelModulePropertyUpdateLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ModelModulePropertyUpdateLogic) ModelModulePropertyUpdate(req *types.ModelModulePropertyUpdateReq) error {
	_, err := l.svcCtx.ProductM.ModelModulePropertyUpdate(l.ctx, &dm.ModelModulePropertyUpdateReq{Property: propertyRPC(req.ModelModulePropertyInfo)})
	return l.rpcError("ProductM.ModelModulePropertyUpdate", req, err)
}

type ModelModulePropertyDeleteLogic struct{ *baseLogic }

func NewModelModulePropertyDeleteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModulePropertyDeleteLogic {
	return &ModelModulePropertyDeleteLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ModelModulePropertyDeleteLogic) ModelModulePropertyDelete(req *types.ModelModulePropertyDeleteReq) error {
	_, err := l.svcCtx.ProductM.ModelModulePropertyDelete(l.ctx, &dm.ModelModulePropertyDeleteReq{
		ModelModuleID: req.ModelModuleID, ModelModulePropertyID: req.ModelModulePropertyID,
	})
	return l.rpcError("ProductM.ModelModulePropertyDelete", req, err)
}

type ModelModulePropertyIndexLogic struct{ *baseLogic }

func NewModelModulePropertyIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModelModulePropertyIndexLogic {
	return &ModelModulePropertyIndexLogic{newBaseLogic(ctx, svcCtx)}
}

func (l *ModelModulePropertyIndexLogic) ModelModulePropertyIndex(req *types.ModelModulePropertyIndexReq) (*types.ModelModulePropertyIndexResp, error) {
	resp, err := l.svcCtx.ProductM.ModelModulePropertyIndex(l.ctx, &dm.ModelModulePropertyIndexReq{
		Page: logic.ToDmPageRpc(req.Page), ModelModuleID: req.ModelModuleID, Identifiers: req.Identifiers,
	})
	if err != nil {
		return nil, l.rpcError("ProductM.ModelModulePropertyIndex", req, err)
	}
	items := make([]*types.ModelModulePropertyInfo, 0, len(resp.List))
	for _, item := range resp.List {
		items = append(items, propertyInfo(item))
	}
	return &types.ModelModulePropertyIndexResp{List: items, PageResp: logic.ToPageResp(req.Page, resp.Total)}, nil
}
