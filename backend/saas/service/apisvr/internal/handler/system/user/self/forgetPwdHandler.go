package self

import (
	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system/user/self"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

func ForgetPwdHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.UserForgetPwdReq
		if err := httpx.Parse(r, &req); err != nil {
			result.Http(w, r, nil, errors.Parameter.WithMsg("入参不正确:"+err.Error()))
			return
		}
		l := self.NewForgetPwdLogic(r.Context(), svcCtx)
		err := l.ForgetPwd(&req)
		result.Http(w, r, nil, err)
	}
}
