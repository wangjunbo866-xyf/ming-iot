package opslogic

import (
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/saas/share/dataType"
)

func ToOpsWorkOrderPo(in *sys.OpsWorkOrder) *relationDB.SysOpsWorkOrder {
	if in == nil {
		return nil
	}
	return &relationDB.SysOpsWorkOrder{
		ID:          in.Id,
		AreaID:      dataType.AreaID(in.AreaID),
		RaiseUserID: in.RaiseUserID,
		IssueDesc:   in.IssueDesc,
		Number:      in.Number,
		Type:        in.Type,
		Params:      in.Params,
		Status:      in.Status,
	}
}
