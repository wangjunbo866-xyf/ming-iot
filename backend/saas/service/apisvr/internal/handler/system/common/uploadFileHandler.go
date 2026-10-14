package common

import (
	"gitee.com/ming-iot/share/utils"
	"net/http"

	"gitee.com/ming-iot/share/result"

	"gitee.com/ming-iot/saas/service/apisvr/internal/logic/system/common"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
)

func UploadFileHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer utils.Recover(r.Context())
		l := common.NewUploadFileLogic(r.Context(), svcCtx, r)
		resp, err := l.UploadFile()
		result.Http(w, r, resp, err)
	}
}
