// 系统管理模块-syssvr
package main

import (
	"context"

	"gitee.com/ming-iot/saas/service/syssvr/sysdirect"
	"gitee.com/ming-iot/share/utils"
)

func main() {
	defer utils.Recover(context.Background())
	svcCtx := sysdirect.GetSvcCtx()
	sysdirect.Run(svcCtx)
}
