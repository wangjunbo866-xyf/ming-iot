package svc

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"gitee.com/ming-iot/saas/service/syssvr/client/areamanage"
	"gitee.com/ming-iot/saas/service/syssvr/client/common"
	"gitee.com/ming-iot/saas/service/syssvr/client/datamanage"
	"gitee.com/ming-iot/saas/service/syssvr/client/departmentmanage"
	"gitee.com/ming-iot/saas/service/syssvr/client/dictmanage"
	"gitee.com/ming-iot/saas/service/syssvr/client/projectmanage"
	"gitee.com/ming-iot/saas/service/syssvr/client/tenantmanage"
	"gitee.com/ming-iot/saas/service/syssvr/client/usermanage"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/saas/service/syssvr/sysExport"
	"gitee.com/ming-iot/saas/service/timed/timedjobsvr/client/timedmanage"
	"gitee.com/ming-iot/saas/share/dataType"
	ws "gitee.com/ming-iot/saas/share/websocket"
	"gitee.com/ming-iot/share/caches"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/eventBus"
	"gitee.com/ming-iot/share/oss"
	"gitee.com/ming-iot/share/stores"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/internal/config"
	"gitee.com/ming-iot/things/service/dmsvr/internal/domain/deviceBind"
	"gitee.com/ming-iot/things/service/dmsvr/internal/domain/deviceGroup"
	"gitee.com/ming-iot/things/service/dmsvr/internal/domain/deviceLog"
	"gitee.com/ming-iot/things/service/dmsvr/internal/domain/protocol"
	"gitee.com/ming-iot/things/service/dmsvr/internal/domain/userShared"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/cache"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/event/publish/pubApp"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/event/publish/pubDev"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/tsDB/abnormalLogRepo"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/tsDB/hubLogRepo"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/tsDB/schemaDataRepo"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/tsDB/sdkLogRepo"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/tsDB/sendLogRepo"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/tsDB/statusLogRepo"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
	"gitee.com/ming-iot/things/share/devices"
	"gitee.com/ming-iot/things/share/domain/deviceMsg/msgThing"
	"gitee.com/ming-iot/things/share/domain/schema"
	"gitee.com/ming-iot/things/share/topics"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/core/syncx"
)

type ServiceContext struct {
	Config               config.Config
	GroupConfig          map[string]*deviceGroup.GroupDetail
	PubDev               pubDev.PubDev
	PubApp               pubApp.PubApp
	ScriptTrans          *protocol.ScriptTrans
	OssClient            *oss.Client
	TimedM               timedmanage.TimedManage
	ProductSchemaRepo    *caches.Cache[schema.Model, string]
	DeviceSchemaRepo     *caches.Cache[schema.Model, devices.Core]
	SchemaManaRepo       msgThing.SchemaDataRepo
	HubLogRepo           deviceLog.HubRepo
	StatusRepo           deviceLog.StatusRepo
	SendRepo             deviceLog.SendRepo
	AbnormalRepo         deviceLog.AbnormalRepo
	SDKLogRepo           deviceLog.SDKRepo
	Cache                kv.Store
	DeviceStatus         *cache.DeviceStatus
	FastEvent            *eventBus.FastEvent
	AreaM                areamanage.AreaManage
	UserM                usermanage.UserManage
	DictM                dictmanage.DictManage
	Common               common.Common
	DataM                datamanage.DataManage
	ProjectM             projectmanage.ProjectManage
	ProductCache         *caches.Cache[dm.ProductInfo, string]
	DeviceCache          *caches.Cache[dm.DeviceInfo, devices.Core]
	UserDeviceShare      *caches.Cache[dm.UserDeviceShareInfo, userShared.UserShareKey]
	UserMultiDeviceShare *cache.UserMultiDeviceShareManager
	DeviceBindToken      *caches.Cache[deviceBind.TokenInfo, string]
	TenantCache          sysExport.TenantCacheT
	ProjectCache         sysExport.ProjectCacheT
	AreaCache            sysExport.AreaCacheT
	WebHook              *sysExport.Webhook
	Slot                 sysExport.SlotCacheT
	UserSubscribe        *ws.UserSubscribe
	GatewayCanBind       *cache.GatewayCanBind
	NodeID               int64
	BindChange           *cache.BindChange
	DeptM                departmentmanage.DepartmentManage
}

func NewServiceContext(c config.Config) *ServiceContext {
	var (
		timedM   timedmanage.TimedManage
		areaM    areamanage.AreaManage
		projectM projectmanage.ProjectManage
		Common   common.Common
		DeptM    departmentmanage.DepartmentManage
	)
	caches.InitStore(c.CacheRedis)
	stores.InitConn(c.Database)
	err := relationDB.Migrate(c.Database)
	if err != nil {
		logx.Error("dmsvr 初始化数据库错误 err", err)
		os.Exit(-1)
	}
	nodeID := utils.GetNodeID(c.CacheRedis, c.Name)
	ca := kv.NewStore(c.CacheRedis)

	ossClient, err := oss.NewOssClient(c.OssConf)
	if err != nil {
		logx.Errorf("NewOss err err:%v", err)
		os.Exit(-1)
	}
	serverMsg, err := eventBus.NewFastEvent(c.Event, c.Name, nodeID)
	logx.Must(err)

	getProductSchemaModel, err := caches.NewCache(caches.CacheConfig[schema.Model, string]{
		KeyType:   topics.ServerCacheKeyDmProductSchema,
		FastEvent: serverMsg,
		GetData: func(ctx context.Context, key string) (*schema.Model, error) {
			product, err := relationDB.NewProductInfoRepo(ctx).FindOneByFilter(ctx, relationDB.ProductFilter{ProductIDs: []string{key}})
			if err != nil {
				return nil, err
			}
			schemaModel, err := relationDB.NewProductDraftComposer(ctx).ComposeEditableModel(ctx, dataType.TenantCode(product.TenantCode), key)
			if err != nil {
				return nil, err
			}
			err = schemaModel.ValidateWithFmt()
			if err != nil {
				logx.WithContext(ctx).Errorf("产品物模型初始化失败,产品ID:%v err:%v", key, err.Error())
			}
			return schemaModel, nil
		},
		Fmt: func(ctx context.Context, key string, data *schema.Model) *schema.Model {
			data.ValidateWithFmt()
			return data
		},
		ExpireTime: 10 * time.Minute,
	})
	logx.Must(err)
	var sf = syncx.NewSingleFlight()
	getDeviceSchemaModel, err := caches.NewCache(caches.CacheConfig[schema.Model, devices.Core]{
		KeyType:   topics.ServerCacheKeyDmDeviceSchema,
		FastEvent: serverMsg,
		GetData: func(ctx context.Context, key devices.Core) (*schema.Model, error) {
			if key.ProductID == "" || key.DeviceName == "" {
				return nil, errors.Parameter.AddMsgf("产品ID和设备ID必填")
			}
			db := relationDB.NewDeviceSchemaRepo(ctx)
			dbSchemas, err := db.FindByFilter(ctx, relationDB.DeviceSchemaFilter{
				ProductID: key.ProductID, DeviceName: key.DeviceName}, nil)
			if err != nil {
				return nil, err
			}
			schemaModel := relationDB.ToDeviceSchemaDo(key.ProductID, dbSchemas)
			schemaModel.ValidateWithFmt()
			return schemaModel, nil
		},
		Fmt: func(ctx context.Context, key devices.Core, data *schema.Model) *schema.Model {
			v, _ := sf.Do(key.ProductID, func() (any, error) {
				ps, err := getProductSchemaModel.GetData(ctx, key.ProductID)
				if err != nil {
					logx.WithContext(ctx).Error(err)
				}
				newOne := data.Copy().Aggregation(ps)
				err = newOne.ValidateWithFmt()
				if err != nil {
					logx.WithContext(ctx).Error(err)
				}
				return newOne, nil
			})
			return v.(*schema.Model)
		},
		ExpireTime: 20 * time.Minute,
	})
	getProductSchemaModel.AddNotifySlot(func(ctx context.Context, keyB []byte) {
		productID := string(keyB)
		getDeviceSchemaModel.DeleteByFunc(func(key string) bool {
			return deviceSchemaCacheBelongsToProduct(key, productID)
		})
	})
	logx.Must(err)

	timedM = timedmanage.NewTimedManage(c.TimedJobRpc.MustNewClient())
	areaM = areamanage.NewAreaManage(c.SysRpc.MustNewClient())
	userM := usermanage.NewUserManage(c.SysRpc.MustNewClient())
	dictM := dictmanage.NewDictManage(c.SysRpc.MustNewClient())
	dataM := datamanage.NewDataManage(c.SysRpc.MustNewClient())
	DeptM = departmentmanage.NewDepartmentManage(c.SysRpc.MustNewClient())

	projectM = projectmanage.NewProjectManage(c.SysRpc.MustNewClient())
	Common = common.NewCommon(c.SysRpc.MustNewClient())
	tenantCache, err := sysExport.NewTenantInfoCache(tenantmanage.NewTenantManage(c.SysRpc.MustNewClient()), serverMsg)
	logx.Must(err)
	webHook, err := sysExport.NewTenantOpenWebhook(tenantmanage.NewTenantManage(c.SysRpc.MustNewClient()), serverMsg)
	logx.Must(err)
	//webHook.Publish(ctxs.BindTenantCode(context.Background(), "default"),
	//	tenantOpenWebhook.CodeDmDeviceConn, application.ConnectMsg{Device: devices.Core{
	//		ProductID:  "123",
	//		DeviceName: "123",
	//	}, Timestamp: time.Now().UnixMilli()})
	Slot, err := sysExport.NewSlotCache(common.NewCommon(c.SysRpc.MustNewClient()))
	logx.Must(err)
	projectC, err := sysExport.NewProjectInfoCache(projectmanage.NewProjectManage(c.SysRpc.MustNewClient()), serverMsg)
	logx.Must(err)
	areaC, err := sysExport.NewAreaInfoCache(areamanage.NewAreaManage(c.SysRpc.MustNewClient()), serverMsg)
	logx.Must(err)
	dc, err := dictM.DictDetailIndex(ctxs.WithRoot(nil), &sys.DictDetailIndexReq{DictCode: deviceGroup.DictCode, Page: &sys.PageInfo{Orders: []*sys.PageInfo_OrderBy{{Field: "sort", Sort: 1}}}})
	logx.Must(err)
	gd := deviceGroup.NewGroupDetail(dc.List)
	deviceDataR := schemaDataRepo.NewDeviceDataRepo(c.TSDB, func(ctx context.Context, productID devices.Core) (*schema.Model, error) {
		return getProductSchemaModel.GetData(ctx, productID.ProductID)
	}, getDeviceSchemaModel.GetData, ca, gd)
	err = deviceDataR.Init(context.Background())
	logx.Must(err)

	hubLogR := hubLogRepo.NewHubLogRepo(c.TSDB, gd)
	abnormalR := abnormalLogRepo.NewAbnormalLogRepo(c.TSDB, gd)
	sdkLogR := sdkLogRepo.NewSDKLogRepo(c.TSDB, gd)
	statusR := statusLogRepo.NewStatusLogRepo(c.TSDB, gd)
	sendR := sendLogRepo.NewSendLogRepo(c.TSDB, gd)
	script := protocol.NewScriptTrans()
	us := ws.NewUserSubscribe(ca, serverMsg)
	pd, err := pubDev.NewPubDev(serverMsg, script, us)
	if err != nil {
		logx.Error("NewPubDev err", err)
		os.Exit(-1)
	}
	pa, err := pubApp.NewPubApp(serverMsg)
	if err != nil {
		logx.Error("NewPubApp err", err)
		os.Exit(-1)
	}
	return &ServiceContext{
		FastEvent:         serverMsg,
		TenantCache:       tenantCache,
		Config:            c,
		OssClient:         ossClient,
		TimedM:            timedM,
		GroupConfig:       deviceGroup.ToMapGroup(dc.List),
		AreaM:             areaM,
		ProjectM:          projectM,
		PubApp:            pa,
		PubDev:            pd,
		Cache:             ca,
		UserM:             userM,
		DictM:             dictM,
		DeptM:             DeptM,
		Common:            Common,
		DataM:             dataM,
		UserSubscribe:     us,
		ProductSchemaRepo: getProductSchemaModel,
		DeviceSchemaRepo:  getDeviceSchemaModel,
		SchemaManaRepo:    deviceDataR,
		DeviceStatus:      cache.NewDeviceStatus(ca),
		GatewayCanBind:    cache.NewGatewayCanBind(ca),
		BindChange:        cache.NewBindChange(ca),
		HubLogRepo:        hubLogR,
		SDKLogRepo:        sdkLogR,
		AbnormalRepo:      abnormalR,
		StatusRepo:        statusR,
		SendRepo:          sendR,
		WebHook:           webHook,
		NodeID:            nodeID,
		Slot:              Slot,
		ProjectCache:      projectC,
		ScriptTrans:       script,
		AreaCache:         areaC,
	}
}

func deviceSchemaCacheBelongsToProduct(cacheKey, productID string) bool {
	if productID == "" {
		return false
	}
	var key devices.Core
	if err := json.Unmarshal([]byte(cacheKey), &key); err != nil {
		return false
	}
	return key.ProductID == productID
}

func (s *ServiceContext) WithDeviceTenant(ctx context.Context, dev devices.Core) context.Context {
	di, err := s.DeviceCache.GetData(ctx, dev)
	if err != nil {
		return ctx
	}
	return ctxs.BindTenantCode(ctx, di.TenantCode, di.ProjectID)
}

//func test() {
//	i := interp.New(interp.Options{})
//
//	i.Use(stdlib.Symbols)
//
//	_, err := i.Eval(`import "ur/ur"`)
//	if err != nil {
//		panic(err)
//	}
//
//	funv, err := i.Eval(`ur.Hello`)
//	if err != nil {
//		panic(err)
//	}
//	fn, ok := funv.Interface().(func(context.Context, string))
//	if !ok {
//		panic(ok)
//	}
//	fn(context.Background(), "hello world")
//}
