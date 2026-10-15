package category

import (
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/apisvr/internal/types"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
)

func ProductCategoryToApi(v *dm.ProductCategory) *types.ProductCategory {
	return utils.Copy[types.ProductCategory](v)
}

func productCategoriesToApi(in []*dm.ProductCategory) (ret []*types.ProductCategory) {
	for _, v := range in {
		ret = append(ret, ProductCategoryToApi(v))
	}
	return
}

func productCategoryToRpc(in *types.ProductCategory) *dm.ProductCategory {
	return utils.Copy[dm.ProductCategory](in)
}
