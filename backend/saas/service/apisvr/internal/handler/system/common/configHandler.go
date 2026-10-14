package common

import (
	"net/http"

	"gitee.com/ming-iot/share/result"

	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system/common"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
)

func ConfigHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := common.NewConfigLogic(r.Context(), svcCtx)
		resp, err := l.Config()
		result.Http(w, r, resp, err)
	}
}
