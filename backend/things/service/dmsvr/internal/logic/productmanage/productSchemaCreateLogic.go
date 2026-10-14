package productmanagelogic

import (
	"context"

	"gitee.com/ming-iot/saas/share/dataType"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dmsvr/internal/logic"
	"gitee.com/ming-iot/things/service/dmsvr/internal/repo/relationDB"
	"gitee.com/ming-iot/things/service/dmsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dmsvr/pb/dm"
	"gitee.com/ming-iot/things/share/domain/schema"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProductSchemaCreateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	PiDB *relationDB.ProductInfoRepo
	PsDB *relationDB.ProductSchemaRepo
	PpDB *relationDB.ProductPropertyDraftRepo
}

type productPropertyInserter interface {
	Insert(context.Context, dataType.TenantCode, *relationDB.DmProductSchema) error
}

type legacyPropertyStableCreator interface {
	CreateProperty(context.Context, *schema.Property, string) error
}

func persistProductProperty(
	ctx context.Context,
	propertyRepo productPropertyInserter,
	stableCreator legacyPropertyStableCreator,
	property *relationDB.DmProductSchema,
	legacy bool,
) error {
	if legacy && property.Tag == schema.TagCustom {
		if err := stableCreator.CreateProperty(ctx, relationDB.ToPropertyDo(property.Identifier, &property.DmSchemaCore), property.ProductID); err != nil {
			return errors.Database.AddDetail(err)
		}
	}
	return propertyRepo.Insert(ctx, dataType.TenantCode(property.TenantCode), property)
}

func NewProductSchemaCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductSchemaCreateLogic {
	return &ProductSchemaCreateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		PiDB:   relationDB.NewProductInfoRepo(ctx),
		PsDB:   relationDB.NewProductSchemaRepo(ctx),
		PpDB:   relationDB.NewProductPropertyDraftRepo(ctx),
	}
}

func (l *ProductSchemaCreateLogic) RuleCheck(in *dm.ProductSchemaCreateReq) (*relationDB.DmProductSchema, error) {
	if err := schema.IDValidate(in.Info.Identifier); err != nil {
		return nil, err
	}
	pi, err := l.svcCtx.ProductCache.GetData(l.ctx, in.Info.ProductID)
	if err != nil {
		if errors.Cmp(err, errors.NotFind) {
			return nil, errors.Parameter.AddMsgf("找不到该产品:%v", in.Info.ProductID)
		}
		return nil, err
	}
	if schema.AffordanceType(in.Info.Type) == schema.AffordanceTypeProperty {
		_, err = l.PpDB.FindOne(l.ctx, dataType.TenantCode(pi.TenantCode), in.Info.ProductID, in.Info.Identifier)
	} else {
		_, err = l.PsDB.FindOneByFilter(l.ctx, relationDB.ProductSchemaFilter{
			ProductID: in.Info.ProductID, Identifiers: []string{in.Info.Identifier},
		})
	}
	if err == nil {
		return nil, errors.Duplicate.AddMsgf("标识符在该产品中已经被使用:%s", in.Info.Identifier)
	}
	if !errors.Cmp(err, errors.NotFind) {
		return nil, err
	}

	po := utils.Copy[relationDB.DmProductSchema](in.Info)
	po.TenantCode = dataType.TenantCodeWithDefaultR(pi.TenantCode)
	var cs *relationDB.DmCommonSchema
	if in.Info.Tag != int64(schema.TagCustom) {
		cs, err = relationDB.NewCommonSchemaRepo(l.ctx).FindOneByFilter(l.ctx, relationDB.CommonSchemaFilter{Identifiers: []string{in.Info.Identifier}})
		if err != nil {
			if errors.Cmp(err, errors.NotFind) {
				return nil, errors.NotFind.AddMsgf("未发现绑定的通用物模型:%v", in.Info.Identifier)
			}
			return nil, err
		}
		po.IsCanSceneLinkage = cs.IsCanSceneLinkage
		po.FuncGroup = cs.FuncGroup
		po.ControlMode = cs.ControlMode
		po.UserPerm = cs.UserPerm
		po.RecordMode = cs.RecordMode
		po.IsPassword = cs.IsPassword
		if po.Name == "" {
			po.Name = cs.Name
		}
		if po.Required == 0 {
			po.Required = cs.Required
		}
		if po.IsCanSceneLinkage == 0 {
			po.IsCanSceneLinkage = cs.IsCanSceneLinkage
		}
		if po.FuncGroup == 0 {
			po.FuncGroup = cs.FuncGroup
		}
		if po.ControlMode == 0 {
			po.ControlMode = cs.ControlMode
		}
		if po.UserPerm != 0 {
			po.UserPerm = cs.UserPerm
		}
		if po.RecordMode == 0 {
			po.RecordMode = cs.RecordMode
		}
		if po.Order == 0 {
			po.Order = cs.Order
		}
		if po.IsPassword == 0 {
			po.IsPassword = cs.IsPassword
		}
		if po.ExtendConfig == "" {
			po.ExtendConfig = cs.ExtendConfig
		}
	}
	if err = logic.CheckAffordance(po.Identifier, &po.DmSchemaCore, cs); err != nil {
		return nil, err
	}
	return po, nil
}

// 新增产品
func (l *ProductSchemaCreateLogic) ProductSchemaCreate(in *dm.ProductSchemaCreateReq) (*dm.Empty, error) {
	if err := ctxs.IsRoot(l.ctx); err != nil {
		return nil, err
	}
	l.Infof("%s req=%v", utils.FuncName(), utils.Fmt(in))
	po, err := l.RuleCheck(in)
	if err != nil {
		l.Errorf("RuleCheck err:%v", err)
		return nil, err
	}
	if schema.AffordanceType(po.Type) == schema.AffordanceTypeProperty {
		legacy, resolveErr := l.PpDB.UsesLegacyProductSchema(l.ctx, dataType.TenantCode(po.TenantCode), po.ProductID)
		if resolveErr != nil {
			return nil, resolveErr
		}
		err = persistProductProperty(l.ctx, l.PpDB, l.svcCtx.SchemaManaRepo, po, legacy)
	} else {
		err = l.PsDB.Insert(l.ctx, po)
	}
	if err != nil {
		return nil, err
	}
	//清除缓存
	err = l.svcCtx.ProductSchemaRepo.SetData(l.ctx, in.Info.ProductID, nil)
	if err != nil {
		return nil, err
	}
	return &dm.Empty{}, nil
}
