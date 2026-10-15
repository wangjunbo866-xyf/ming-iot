package share

import (
	"net/http"

	"gitee.com/ming-iot/share/result"
	"gitee.com/ming-iot/things/service/apisvr/internal/logic/things/user/device/share"
	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
)

// 获取批量分享 Token 列表
func MultiGetTokenListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := share.NewMultiGetTokenListLogic(r.Context(), svcCtx)
		resp, err := l.MultiGetTokenList()
		result.Http(w, r, resp, err)
	}
}
