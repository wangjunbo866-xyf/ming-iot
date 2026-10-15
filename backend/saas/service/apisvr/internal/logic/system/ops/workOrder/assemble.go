package workOrder

import (
	"gitee.com/ming-iot/saas/service/apisvr/internal/types"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/utils"
)

func ToOpsWorkOrderPb(in *types.OpsWorkOrder) *sys.OpsWorkOrder {
	return utils.Copy[sys.OpsWorkOrder](in)
}
