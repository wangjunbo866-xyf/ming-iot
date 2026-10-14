package modelRelease

import (
	"context"
	"net/http"

	projecterrors "gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	releaselogic "gitee.com/ming-iot/things/service/apisvr/internal/logic/things/product/modelRelease"
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

func ProductModelReleasePublishHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return withResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ProductModelReleasePublishReq) (*types.ProductModelReleaseInfo, error) {
		return releaselogic.NewProductModelReleasePublishLogic(ctx, svcCtx).ProductModelReleasePublish(req)
	})
}

func ProductModelReleaseActivateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return noResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ProductModelReleaseActivateReq) error {
		return releaselogic.NewProductModelReleaseActivateLogic(ctx, svcCtx).ProductModelReleaseActivate(req)
	})
}

func ProductModelReleaseReadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return withResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ProductModelReleaseReadReq) (*types.ProductModelReleaseInfo, error) {
		return releaselogic.NewProductModelReleaseReadLogic(ctx, svcCtx).ProductModelReleaseRead(req)
	})
}

func ProductModelReleaseIndexHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return withResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ProductModelReleaseIndexReq) (*types.ProductModelReleaseIndexResp, error) {
		return releaselogic.NewProductModelReleaseIndexLogic(ctx, svcCtx).ProductModelReleaseIndex(req)
	})
}

func ProductModelReleaseTslReadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return withResponse(svcCtx, func(ctx context.Context, svcCtx *svc.ServiceContext, req *types.ProductModelReleaseTslReadReq) (*types.ProductModelReleaseTslReadResp, error) {
		return releaselogic.NewProductModelReleaseTslReadLogic(ctx, svcCtx).ProductModelReleaseTslRead(req)
	})
}
