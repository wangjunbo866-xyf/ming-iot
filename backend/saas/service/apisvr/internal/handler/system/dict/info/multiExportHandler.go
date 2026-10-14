package info

import (
	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system/dict/info"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/result"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

// 删除字典信息
func MultiExportHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DictInfoMultExportReq
		if err := httpx.Parse(r, &req); err != nil {
			result.Http(w, r, nil, errors.Parameter.WithMsg("入参不正确:"+err.Error()))
			return
		}

		l := info.NewMultiExportLogic(r.Context(), svcCtx)
		resp, err := l.MultiExport(&req)
		result.Http(w, r, resp, err)
	}
}
