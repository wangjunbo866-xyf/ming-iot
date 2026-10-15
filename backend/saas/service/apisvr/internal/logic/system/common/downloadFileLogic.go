package common

import (
	"context"
	"gitee.com/ming-iot/saas/service/apisvr/internal/svc"
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/oss"
	"gitee.com/ming-iot/share/result"
	"net/http"

	"github.com/zeromicro/go-zero/core/logx"
)

type DownloadFileLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDownloadFileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DownloadFileLogic {
	return &DownloadFileLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DownloadFileLogic) DownloadFile(req *types.DownloadFileReq, w http.ResponseWriter, r *http.Request) {
	ossLocal, ok := l.svcCtx.OssClient.Handle.(*oss.Local)
	if !ok {
		result.Http(w, r, nil, errors.System.AddMsgf("本地存储不支持"))
		return
	}
	err := ossLocal.DownloadFile(l.ctx, req.FilePath, req.Sign, w)
	if err != nil {
		result.Http(w, r, nil, err)
		return
	}

	return
}
