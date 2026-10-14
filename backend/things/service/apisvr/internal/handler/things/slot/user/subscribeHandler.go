package user

import (
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	"gitee.com/ming-iot/things/service/apisvr/internal/logic/things/slot/user"
	"gitee.com/ming-iot/things/service/apisvr/internal/svc"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

// 插槽用户订阅
func SubscribeHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.SlotUserSubscribe
		if err := httpx.Parse(r, &req); err != nil {
			result.Http(w, r, nil, errors.Parameter.WithMsg("入参不正确:"+err.Error()))
			return
		}

		l := user.NewSubscribeLogic(r.Context(), svcCtx)
		resp, err := l.Subscribe(&req)
		result.Http(w, r, resp, err)
	}
}
