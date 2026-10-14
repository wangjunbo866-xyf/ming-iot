package info

import (
	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system/tenant/access/info"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

// 批量创建租户操作权限
func MultiCreateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.TenantAccessInfo
		if err := httpx.Parse(r, &req); err != nil {
			result.Http(w, r, nil, errors.Parameter.WithMsg("入参不正确:"+err.Error()))
			return
		}

		l := info.NewMultiCreateLogic(r.Context(), svcCtx)
		err := l.MultiCreate(&req)
		result.Http(w, r, nil, err)
	}
}
