package pair

import (
	"net/http"

	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	pairlogic "gitee.com/ming-iot/things/service/apisvr/internal/logic/things/device/pair"
	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ConfirmHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DevicePairConfirmReq
		if err := httpx.Parse(r, &req); err != nil {
			result.Http(w, r, nil, errors.Parameter.WithMsg("入参不正确:"+err.Error()))
			return
		}

		l := pairlogic.NewConfirmLogic(r.Context(), svcCtx)
		resp, err := l.Confirm(&req)
		result.Http(w, r, resp, err)
	}
}
