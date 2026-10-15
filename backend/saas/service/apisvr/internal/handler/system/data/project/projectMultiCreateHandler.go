package project

import (
	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system/data/project"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

// 批量更新授权项目权限
func ProjectMultiCreateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DataProjectMultiSaveReq
		if err := httpx.Parse(r, &req); err != nil {
			result.Http(w, r, nil, errors.Parameter.WithMsg("入参不正确:"+err.Error()))
			return
		}

		l := project.NewProjectMultiCreateLogic(r.Context(), svcCtx)
		err := l.ProjectMultiCreate(&req)
		result.Http(w, r, nil, err)
	}
}
