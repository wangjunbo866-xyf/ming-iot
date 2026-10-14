package schema

import (
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	"gitee.com/ming-iot/things/service/apisvr/internal/logic/things/device/schema"
	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

// 批量创建产品物模型
func MultiCreateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DeviceSchemaMultiCreateReq
		if err := httpx.Parse(r, &req); err != nil {
			result.Http(w, r, nil, errors.Parameter.WithMsg("入参不正确:"+err.Error()))
			return
		}

		l := schema.NewMultiCreateLogic(r.Context(), svcCtx)
		err := l.MultiCreate(&req)
		result.Http(w, r, nil, err)
	}
}
