package app

import (
	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system/tenant/app"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

func UpdateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.TenantAppInfo
		if err := httpx.Parse(r, &req); err != nil {
			result.Http(w, r, nil, errors.Parameter.WithMsg("入参不正确:"+err.Error()))
			return
		}

		l := app.NewUpdateLogic(r.Context(), svcCtx)
		err := l.Update(&req)
		result.Http(w, r, nil, err)
	}
}
