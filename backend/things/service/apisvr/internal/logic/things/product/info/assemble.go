package info

import (
	"context"

	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
)

func productInfoToApi(ctx context.Context, v *dm.ProductInfo) *types.ProductInfo {
	if v == nil {
		return nil
	}
	if uc := ctxs.GetUserCtx(ctx); uc != nil && !uc.IsAdmin {
		v.Secret = ""        // 设备秘钥
		v.ProtocolConf = nil // 设备证书
		v.SubProtocolConf = nil
		v.Config = nil
	}
	return utils.Copy[types.ProductInfo](v)
}

func productInfoToRpc(req *types.ProductInfo) *dm.ProductInfo {
	return utils.Copy[dm.ProductInfo](req)
}
