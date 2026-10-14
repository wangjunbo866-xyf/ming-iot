package common

import (
	"encoding/json"
	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system/common"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	"io"
	"net/http"
)

// 批量聚合接口请求
func ApiBatchAggHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ApiBatchAggReq
		body, err := io.ReadAll(r.Body)
		if err != nil {
			result.Http(w, r, nil, errors.Parameter.WithMsg("入参不正确:"+err.Error()))
		}
		err = json.Unmarshal(body, &req)
		if err != nil {
			result.Http(w, r, nil, errors.Parameter.WithMsg("入参不正确:"+err.Error()))
		}
		l := common.NewApiBatchAggLogic(r.Context(), svcCtx)
		resp, err := l.ApiBatchAgg(r, &req)
		result.Http(w, r, resp, err)
	}
}
