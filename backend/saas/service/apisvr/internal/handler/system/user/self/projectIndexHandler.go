package self

import (
	"net/http"

	"gitee.com/ming-iot/share/result"

	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system/user/self"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
)

func ProjectIndexHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := self.NewProjectIndexLogic(r.Context(), svcCtx)
		resp, err := l.ProjectIndex()
		result.Http(w, r, resp, err)
	}
}
