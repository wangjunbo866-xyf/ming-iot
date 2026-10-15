package def

const (
	AppCore = "core"
	AppAll  = "all" //绑定所有模块
)

const (
	TenantCodeDefault = "default" //默认租户
)

func GetTenantCode[inT ~string](in inT) string {
	return string(in)
}

const (
	ModuleSystemManage = "systemManage" //租户内的系统管理
	ModuleTenantManage = "tenantManage" //租户系统管理
	ModuleThings       = "things"       //物联网模块
	ModuleView         = "view"         //大屏模块
	ModuleVideo        = "video"        //音视频
)

type AppType = string

const (
	AppTypeWeb    = "web"
	AppTypeNative = "native" //原生
	AppTypeMini   = "mini"
)

type AppSubType = string

const (
	//mini类型
	AppSubTypeWx   AppSubType = "wx"   //微信
	AppSubTypeWxE  AppSubType = "wxE"  //企业微信
	AppSubTypeDing AppSubType = "ding" //钉钉应用
	AppSubTypeHuawei  AppSubType = "huawei"

	//原生客户端类型
	AppSubTypeAndroid AppSubType = "android"
	AppSubTypeIos     AppSubType = "ios"
	AppSubTypeHarmony AppSubType = "harmony" //鸿蒙
	AppSubTypeWin     AppSubType = "win"     //Windows
)

type ThirdType = string

const (
	ThirdTypeWx      = "wx"      //微信
	ThirdTypeWxMiniP = "wxMiniP" //微信小程序
	ThirdTypeWxOpen  = "wxOpen"  //微信开放平台登录
	ThirdTypeDingApp = "dingApp" //钉钉应用(包含小程序,h5等方式)
)
