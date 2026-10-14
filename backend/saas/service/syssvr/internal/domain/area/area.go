package area

import (
	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/share/def"
)

var (
	RootNode = relationDB.SysAreaInfo{
		ProjectID:    0,
		AreaID:       def.RootNode,
		ParentAreaID: 0,
		AreaName:     "全部区域",
		Desc:         "虚拟根节点",
	}
	NotClassified = relationDB.SysAreaInfo{
		ProjectID:    0,
		AreaID:       def.NotClassified,
		ParentAreaID: 0,
		AreaName:     "未分类",
		Desc:         "虚拟根节点",
	}
)
