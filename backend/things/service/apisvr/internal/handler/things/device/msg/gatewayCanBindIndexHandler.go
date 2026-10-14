package msg

import (
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	"gitee.com/ming-iot/things/service/apisvr/internal/logic/things/device/msg"
	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

// 获取网关可以绑定的子设备列表
func GatewayCanBindIndexHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GatewayCanBindIndexReq
		if err := httpx.Parse(r, &req); err != nil {
			result.Http(w, r, nil, errors.Parameter.WithMsg("入参不正确:"+err.Error()))
			return
		}

		l := msg.NewGatewayCanBindIndexLogic(r.Context(), svcCtx)
		resp, err := l.GatewayCanBindIndex(&req)
		result.Http(w, r, resp, err)
	}
}
