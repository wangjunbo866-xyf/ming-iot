package timedmanagelogic

import (
	"context"

	"gitee.com/ming-iot/saas/service/timed/internal/repo/relationDB"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"

	"gitee.com/ming-iot/saas/service/timed/timedjobsvr/internal/svc"
	"gitee.com/ming-iot/saas/service/timed/timedjobsvr/pb/timedjob"

	"github.com/zeromicro/go-zero/core/logx"
)

type TaskInfoCreateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewTaskInfoCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *TaskInfoCreateLogic {
	return &TaskInfoCreateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *TaskInfoCreateLogic) TaskInfoCreate(in *timedjob.TaskInfo) (*timedjob.Response, error) {
	l.Infof("TaskInfoCreate,in:%v", utils.Fmt(in))
	po := ToTaskInfoPo(in)
	err := relationDB.NewTaskInfoRepo(l.ctx).Insert(l.ctx, po)
	if err != nil {
		if errors.Cmp(err, errors.Duplicate) {
			return &timedjob.Response{}, nil
		}
		return nil, err
	}
	return &timedjob.Response{}, nil
}
