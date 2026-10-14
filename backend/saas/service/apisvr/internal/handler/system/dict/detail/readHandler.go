package detail

import (
	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system/dict/detail"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

// 获取字典详情单个
func ReadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DictDetailReadReq
		if err := httpx.Parse(r, &req); err != nil {
			result.Http(w, r, nil, errors.Parameter.WithMsg("入参不正确:"+err.Error()))
			return
		}

		l := detail.NewReadLogic(r.Context(), svcCtx)
		resp, err := l.Read(&req)
		result.Http(w, r, resp, err)
	}
}
