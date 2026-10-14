package common

import (
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
)

func ToSchemaInfoRpc(in *types.CommonSchemaInfo) *dm.CommonSchemaInfo {
	return utils.Copy[dm.CommonSchemaInfo](in)
}

func ToSchemaInfoTypes(in *dm.CommonSchemaInfo) *types.CommonSchemaInfo {
	return utils.Copy[types.CommonSchemaInfo](in)
}
