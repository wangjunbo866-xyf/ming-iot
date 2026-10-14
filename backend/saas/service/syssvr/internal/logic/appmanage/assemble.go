package appmanagelogic

import (
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/utils"
)

func ToAppInfoPo(in *sys.AppInfo) *relationDB.SysAppInfo {
	return utils.Copy[relationDB.SysAppInfo](in)
}

func ToAppInfoPb(in *relationDB.SysAppInfo) *sys.AppInfo {
	return utils.Copy[sys.AppInfo](in)
}

func ToAppInfosPb(in []*relationDB.SysAppInfo) []*sys.AppInfo {
	var ret []*sys.AppInfo
	for _, v := range in {
		ret = append(ret, ToAppInfoPb(v))
	}
	return ret
}
