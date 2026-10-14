package self

import (
	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system/user/self"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/saas/share/middlewares"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

func CaptchaHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.UserCaptchaReq
		if err := httpx.Parse(r, &req); err != nil {
			result.Http(w, r, nil, errors.Parameter.WithMsg("入参不正确:"+err.Error()))
			return
		}
		userCtx, err := middlewares.NewCheckTokenWareMiddleware(svcCtx.UserRpc, svcCtx.RoleRpc, svcCtx.TenantRpc, svcCtx.LogRpc).UserAuth(w, r)
		if err == nil { //登录态也需要支持
			//注入 用户信息 到 ctx
			ctx2 := ctxs.SetUserCtx(r.Context(), userCtx)
			r = r.WithContext(ctx2)
		}
		r = ctxs.InitCtxWithReq(r)
		l := self.NewCaptchaLogic(r.Context(), svcCtx)
		resp, err := l.Captcha(&req)
		result.Http(w, r, resp, err)
	}
}
