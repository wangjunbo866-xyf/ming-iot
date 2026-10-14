package dictmanagelogic

import (
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/utils"
)

func ToDictInfoPb(in *relationDB.SysDictInfo) *sys.DictInfo {
	return utils.Copy[sys.DictInfo](in)
}

func ToDictInfosPb(in []*relationDB.SysDictInfo) (list []*sys.DictInfo) {
	for _, v := range in {
		list = append(list, ToDictInfoPb(v))
	}
	return
}
func ToDictDetailsPb(in []*relationDB.SysDictDetail) []*sys.DictDetail {
	return utils.CopySlice[sys.DictDetail](in)

}
