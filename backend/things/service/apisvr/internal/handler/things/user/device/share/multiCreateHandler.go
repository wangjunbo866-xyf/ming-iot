package share

import (
	"net/http"

	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	"gitee.com/ming-iot/things/service/apisvr/internal/logic/things/user/device/share"
	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// 生成批量分享设备二维码
func MultiCreateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.UserDeviceShareMultiInfo
		if err := httpx.Parse(r, &req); err != nil {
			result.Http(w, r, nil, errors.Parameter.WithMsg("入参不正确:"+err.Error()))
			return
		}

		l := share.NewMultiCreateLogic(r.Context(), svcCtx)
		resp, err := l.MultiCreate(&req)
		result.Http(w, r, resp, err)
	}
}
