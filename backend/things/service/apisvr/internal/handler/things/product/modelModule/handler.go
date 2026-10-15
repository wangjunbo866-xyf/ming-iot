package modelModule

import (
	"context"
	"net/http"

	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	modulelogic "gitee.com/ming-iot/things/service/apisvr/internal/logic/things/product/modelModule"
	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func noResponse[Request any](svcCtx *svc.ServiceContext, call func(context.Context, *svc.ServiceContext, *Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Request
		if err := httpx.Parse(r, &req); err != nil {
			result.Http(w, r, nil, projecterrors.Parameter.WithMsg("入参不正确:"+err.Error()))
			return
		}
		result.Http(w, r, nil, call(r.Context(), svcCtx, &req))
	}
}

func withResponse[Request, Response any](svcCtx *svc.ServiceContext, call func(context.Context, *svc.ServiceContext, *Request) (*Response, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Request
		if err := httpx.Parse(r, &req); err != nil {
			result.Http(w, r, nil, projecterrors.Parameter.WithMsg("入参不正确:"+err.Error()))
			return
		}
		resp, err := call(r.Context(), svcCtx, &req)
		result.Http(w, r, resp, err)
	}
}

func ModelModuleCreateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return withResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ModelModuleCreateReq) (*types.ModelModuleInfo, error) {
		return modulelogic.NewModelModuleCreateLogic(ctx, svcCtx).ModelModuleCreate(req)
	})
}

func ModelModuleUpdateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return noResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ModelModuleUpdateReq) error {
		return modulelogic.NewModelModuleUpdateLogic(ctx, svcCtx).ModelModuleUpdate(req)
	})
}

func ModelModuleDeleteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return noResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ModelModuleDeleteReq) error {
		return modulelogic.NewModelModuleDeleteLogic(ctx, svcCtx).ModelModuleDelete(req)
	})
}

func ModelModuleIndexHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return withResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ModelModuleIndexReq) (*types.ModelModuleIndexResp, error) {
		return modulelogic.NewModelModuleIndexLogic(ctx, svcCtx).ModelModuleIndex(req)
	})
}

func ProductModelModuleBindHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return noResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ProductModelModuleBindReq) error {
		return modulelogic.NewProductModelModuleBindLogic(ctx, svcCtx).ProductModelModuleBind(req)
	})
}

func ProductModelModuleUnbindHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return noResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ProductModelModuleUnbindReq) error {
		return modulelogic.NewProductModelModuleUnbindLogic(ctx, svcCtx).ProductModelModuleUnbind(req)
	})
}

func ProductModelModuleIndexHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return withResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ProductModelModuleIndexReq) (*types.ProductModelModuleIndexResp, error) {
		return modulelogic.NewProductModelModuleIndexLogic(ctx, svcCtx).ProductModelModuleIndex(req)
	})
}

func ModelModulePropertyCreateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return withResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ModelModulePropertyCreateReq) (*types.ModelModulePropertyInfo, error) {
		return modulelogic.NewModelModulePropertyCreateLogic(ctx, svcCtx).ModelModulePropertyCreate(req)
	})
}

func ModelModulePropertyUpdateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return noResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ModelModulePropertyUpdateReq) error {
		return modulelogic.NewModelModulePropertyUpdateLogic(ctx, svcCtx).ModelModulePropertyUpdate(req)
	})
}

func ModelModulePropertyDeleteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return noResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ModelModulePropertyDeleteReq) error {
		return modulelogic.NewModelModulePropertyDeleteLogic(ctx, svcCtx).ModelModulePropertyDelete(req)
	})
}

func ModelModulePropertyIndexHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return withResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ModelModulePropertyIndexReq) (*types.ModelModulePropertyIndexResp, error) {
		return modulelogic.NewModelModulePropertyIndexLogic(ctx, svcCtx).ModelModulePropertyIndex(req)
	})
}
