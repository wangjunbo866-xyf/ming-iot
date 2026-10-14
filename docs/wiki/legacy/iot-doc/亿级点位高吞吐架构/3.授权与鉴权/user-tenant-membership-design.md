# 用户模型改造设计：全局账号 + 租户成员关系（B 方案）

> 状态：设计稿（2026-08-15）。承接 [thingsboard-permission-research.md](./thingsboard-permission-research.md) 第 10 章的身份体系对比与"身份物化"结论；多层租户预留衔接 [tenant-hierarchy.md](./tenant-hierarchy.md)。
> 决策记录：在"行即成员（A，现状）"与"全局账号 + membership（B）"之间选 **B**：`sys_user_info` 改造为全局账号表，新增 `sys_tenant_user` 成员关系表，一人一号，多租户各自身份。

---

## 1. 背景与目标

现状（已核实）：

- `sys_user_info` 的所有唯一索引都是 `(tenant_code, xxx)` 复合索引（`modelUser.go:14-29`），同一手机号/邮箱可在多个租户各占一行——"一人多租户 = 多个账号多套密码"。
- 身份（租户主/管理员/C 端）靠运行时推导：`userCheckToken` 聚合用户全部角色 + 租户表 `AdminUserID/AdminRoleID` 算 `IsSuperAdmin/IsAdmin`，且与 `openCheckToken`（按 supper code 字符串）两条路径公式不一致；角色 code `"admin"` 是运行时 magic string。
- "查某租户全部管理员/客户"需要 `sys_user_info ⋈ sys_user_role ⋈ sys_role_info` 三表 join。

目标：

1. **一人一号**：凭证（手机/邮箱/用户名/密码/三方 union）全局唯一，归 `sys_user_info`；改一次密码全租户生效。
2. **成员关系一等化**：`sys_tenant_user` 承载 (账号, 租户) 二元组，**身份物化**为 `user_type` 列——查租户管理员/客户变成单表索引查询，身份判定不再聚合角色。
3. **每租户独立身份与会话**：同一账号在 A 租户是管理员、B 租户是 C 端用户；单会话（LastTokenID）按租户隔离。
4. **语义修正**：消除 IsSuperAdmin 命名歧义、双路径推导不一致、角色 code magic string；角色降级为"授权载体"，不再承载身份判定。
5. **兼容演进**：`sys_user_role`/`sys_user_profile`/数据权限授权等既有表结构不动（它们已带 tenant_code 维度，语义自动落到 membership 上）；预留 tenant-hierarchy 的"租户超级管理员"。

非目标：不改变角色/菜单/API 授权/项目区域数据权限的既有模型；不引入多层租户（仅预留）。

---

## 2. 身份模型与 user_type 枚举

```
sys_user_info（全局账号）
   │ 1:N
sys_tenant_user（成员关系，身份物化在此行）
   │ 1:N
sys_user_role（租户内角色，授权用）
```

`user_type` 四值（新增 `share/def` 常量，命名建议 `def.UserTypeTenantOwner/Admin/Member/Client`）：

| 值 | 类型 | 语义 | 对应现状 |
|:-:|---|---|---|
| 1 | TenantOwner 租户主 | 租户的所有者，全权，一个租户一至多人 | supper 角色绑定 / `AdminUserID` 命中（IsSuperAdmin） |
| 2 | TenantAdmin 租户管理员 | 租户内的管理端用户 | admin 角色绑定（IsAdmin） |
| 3 | Member 普通成员 | 管理端普通用户 | 自定义角色 |
| 4 | Client C 端用户 | App/小程序终端用户 | client 角色 |

派生身份（**不物化，推导**，语义清晰且随租户树扩展）：

- **平台超级管理员** = `tenant_code == "default" && user_type ∈ {Owner, Admin}`（对齐现 `IsRoot()` 语义）。
- **租户超级管理员**（未来，tenant-hierarchy.md）= 子树根租户的 Owner，`user_type` 不需要新枚举值。
- **游客**：本设计不含游客身份（与 TB/联犀现状一致），匿名能力继续走免 token 白名单路由。

### 2.1 物化规则（写时维护）

`user_type` 是"角色绑定 + 租户表两个字段"的**缓存**，事实源不变，但在写路径同步维护：

| 触发点 | 动作 |
|---|---|
| 创建 membership（注册/邀请/管理员添加） | 按初始角色写 user_type |
| `sys_user_role` 增/删 | 同事务内重算该 (user, tenant) 的 user_type（命中 AdminRoleID → Owner；code=admin → Admin；code=client → Client；否则 Member） |
| `sys_tenant_info.AdminUserID/AdminRoleID` 变更 | 重算该租户内受影响用户的 user_type |
| 租户创建（tenantInfoCreateLogic） | owner 用户生成 `user_type=Owner` 的 membership |
| 兜底 | 每日对账任务：全量重算 diff，漂移告警 + 自动修复（幂等脚本复用） |

---

## 3. 表设计

### 3.1 sys_user_info 改造为全局账号表

逐字段落位（现状字段 → 去向）：

| 字段 | 去向 | 说明 |
|---|---|---|
| UserID | **留** | 主键，语义变为全局账号 ID |
| UserName / Email / Phone | **留**，唯一索引**全局化** | 删掉 tc 前缀复合索引，改单列全局唯一 |
| Password | **留** | 全局凭证 |
| NickName/HeadImg/Sex/City/Country/Province/Language | **留** | 全局个人档案 |
| LastIP/RegIP | **留** | 全局登录统计 |
| LastTokenID | **挪** sys_tenant_user | 单会话按租户隔离（见 5.3） |
| WechatUnionID/DingTalk(Union+User)ID/GoogleUserID/GithubUserID/AppleUserID | **留**，全局唯一 | 平台级跨应用稳定标识，可全局化 |
| WechatOpenID/HuaweiOpenID/HuaweiUnionID | **挪** 新表 `sys_user_oauth_binding` | **坑**：openid 是 per-应用标识（每个租户配不同微信/华为应用），现状唯一索引带 tenant_code 正因如此，不能全局唯一化；华为 union 随 open 成对搬 |
| Role（默认角色） | **挪** sys_tenant_user.default_role_id | 租户内属性 |
| IsAllData / DeviceCount / Tags / PubTags | **挪** sys_tenant_user | 租户内数据权限/配额/标签（Tags 注释"管理员可改"，是租户语义） |
| Status | **拆两层** | 账号级 status（全局封禁=所有租户不可登录）+ membership.status（租户内禁用） |
| TenantCode | **删**（M3） | 迁移期保留双写，M3 摘除 |

### 3.2 新表 sys_user_role 的兄弟：sys_tenant_user

```go
// 租户成员关系表：账号在某租户下的身份与状态
type SysTenantUser struct {
    ID            int64               `gorm:"column:id;type:BIGINT;primary_key;AUTO_INCREMENT"`
    TenantCode    dataType.TenantCode `gorm:"column:tenant_code;type:VARCHAR(50);NOT NULL;uniqueIndex:idx_sys_tenant_user_tc_uid"` // 租户编码
    UserID        int64               `gorm:"column:user_id;uniqueIndex:idx_sys_tenant_user_tc_uid;NOT NULL;type:BIGINT"`             // 全局账号ID
    UserType      int64               `gorm:"column:user_type;type:TINYINT;NOT NULL;default:3"`                                     // 1租户主 2租户管理员 3普通成员 4C端用户（物化，见2.1）
    Status        int64               `gorm:"column:status;type:TINYINT;NOT NULL;default:1"`                                        // 租户内状态: 1启用 2禁用
    DefaultRoleID int64               `gorm:"column:default_role_id;type:BIGINT"`                                                   // 默认角色
    IsAllData     int64               `gorm:"column:is_all_data;type:TINYINT;default:1;NOT NULL"`
    DeviceCount   int64               `gorm:"column:device_count;default:0"`
    Tags          map[string]string   `gorm:"column:tags;type:json;serializer:json"`
    PubTags       map[string]string   `gorm:"column:pub_tags;type:json;serializer:json"`
    LastTokenID   string              `gorm:"column:last_token_id;type:VARCHAR(128);default:''"`                                    // 本租户内最后会话（单会话按租户隔离）
    InviterUserID int64               `gorm:"column:inviter_user_id;type:BIGINT;default:0"`                                          // 邀请人（0=自助注册）
    stores.NoDelTime
    DeletedTime   stores.DeletedTime  `gorm:"column:deleted_time;default:0;uniqueIndex:idx_sys_tenant_user_tc_uid"`
}

func (m *SysTenantUser) TableName() string { return "sys_tenant_user" }
```

关键查询路径（全部单表/单索引）：

- 某账号的全部租户：`WHERE user_id=? AND status=1`
- 某租户的成员/管理员/C 端用户分页：`WHERE tenant_code=? AND user_type=?`
- 登录定位：`WHERE user_id=? AND tenant_code=?`

### 3.3 新表 sys_user_oauth_binding（三方 per-应用绑定）

```go
// 账号的三方登录绑定：per-应用的 openid 挂租户（租户配不同三方应用），union 类仍留在账号行
type SysUserOauthBinding struct {
    ID         int64               `gorm:"column:id;type:BIGINT;primary_key;AUTO_INCREMENT"`
    TenantCode dataType.TenantCode `gorm:"column:tenant_code;type:VARCHAR(50);NOT NULL;uniqueIndex:idx_sys_user_oauth_tc_p_oid"` // 租户（即三方应用归属）
    Provider   string              `gorm:"column:provider;type:VARCHAR(32);NOT NULL;uniqueIndex:idx_sys_user_oauth_tc_p_oid"`     // wechat/huawei/...
    OpenID     string              `gorm:"column:open_id;type:VARCHAR(128);NOT NULL;uniqueIndex:idx_sys_user_oauth_tc_p_oid"`     // per-app 标识
    UnionID    string              `gorm:"column:union_id;type:VARCHAR(128);index"`                                               // 冗余 union，便于反查账号
    UserID     int64               `gorm:"column:user_id;type:BIGINT;NOT NULL;index"`
    stores.NoDelTime
    DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:idx_sys_user_oauth_tc_p_oid"`
}
```

微信小程序/华为等按应用发放的 openid 走这张表；Google/GitHub/Apple/钉钉等平台级 ID 维持在账号行（全局唯一）。

---

## 4. 关联表影响清单（结构不动的部分）

| 表 | 影响 | 动作 |
|---|---|---|
| `sys_user_role` | 已有 `tenant_code` 冗余列，(user, role) 语义自动变成"该租户内的角色" | 结构不动；绑定时应用层校验"用户是该租户成员"；角色变更触发 user_type 重算 |
| `sys_user_profile` | (tenant, user, code) 配置 | 结构不动，语义即 membership 配置 |
| `sys_dept_user` / `sys_data_project` / `sys_data_area`（target=user） | 授权行挂 user，但 dept/project/area 本身属租户，定界不变 | 结构不动；`GetProjectAuth` 聚合入口改为显式传 tenantCode 过滤 |
| `sys_login_log` / `sys_oper_log` | 已带 tenant_code + user_id | 结构不动；迁移期历史 user_id 经映射解析（见 7.2），不回写历史日志 |
| things 侧 `dm_device_info.user_id`、`dm_user_device_share.shared_user_id`、`dm_user_device_collect` | FK 语义从"租户内用户行"变为"全局账号（在设备所属租户内有成员关系）" | 结构不动；**数据迁移需批量改写旧 user_id**（见 7.2）；dmsvr 的设备鉴权（`dmsvr/internal/logic/auth.go`）不改逻辑，只改 FK 数据 |
| `sys_data_open_access`（开放认证） | 行绑定 (tenant, userID) → 语义变为 membership | `openCheckToken` 的身份判定改读 `sys_tenant_user.user_type`，**顺带消除双路径不一致** |

---

## 5. 登录、会话与租户切换

### 5.1 两步登录

```
① 账号验证（全局）：user_name/phone/email + password（或三方 union/验证码）
      ↓
② 列出租户：sys_tenant_user WHERE user_id=? AND status=1
      ├─ 0 个 → 未加入任何租户（引导注册/申请）
      ├─ 1 个 → 直接进入
      └─ 多个 → 前端展示租户列表，用户选择
      ↓
③ 签发 token（claims 绑定租户）
```

- 现有 `UserSearch`（按 account 查、`Size:1` 取第一条）语义在多租户下是错的，改造为步骤 ② 的租户列表接口（免 token 组内）。
- 登录 RPC 不再从 header 的 tenantCode 定界账号；header 租户信息仅用于三方 openid 绑定（步骤 ① 的微信/华为登录需要知道当前租户的应用）。

### 5.2 Token 设计

`LoginClaims` 增加字段：

```go
type LoginClaims struct {
    UserID     int64
    TenantCode string  // 新增：token 与租户绑定
    AppCode    string
    DeviceID   string
}
```

- `UserCheckToken` 流程变为：解 JWT → 查账号行（全局 status、密码状态）→ 查 `sys_tenant_user`（该租户的 user_type/status/is_all_data）→ 角色（仅非 Owner/Admin 需要角色细节）→ ProjectAuth。**身份判定读列，不再聚合推导**。
- 租户切换接口：凭现有 token（或重新验证凭证）→ 校验目标租户 membership 有效 → 签发新租户 token。token 内不放 user_type 等可变信息，避免续期后陈旧。
- 迁移窗口：旧 token 无 TenantCode，按用户行的（唯一）租户补齐并强制重登，不做长期双兼容。

### 5.3 单会话（LastTokenID）

- 从账号行挪到 membership 行：**每个租户一个独立会话**，A 租户登录不踢 B 租户的会话；账号级封禁（status）踢全部。
- 滑动续期逻辑沿用，校验对象改为 `sys_tenant_user.last_token_id`。

### 5.4 注册/加入语义

- 全局账号不存在：现流程——创建账号 + 在当前租户创建 membership（`sys_tenant_config.RegisterRoleID` 决定初始 user_type）。
- **全局账号已存在**（手机号已注册）：租户内"注册"变为"加入"——验证码验证手机号后创建 membership（按租户配置自动通过或管理员审批）。这是 B 方案带来的新流程，需要产品确认交互。

---

## 6. UserCtx 与中间件改造

```go
// 语义化方法（新增，逐步冻结裸字段）
func (u *UserCtx) IsTenantOwner() bool   // membership.UserType == 1
func (u *UserCtx) IsTenantAdmin() bool   // membership.UserType ∈ {1,2}
func (u *UserCtx) IsPlatformAdmin() bool // TenantCode == "default" && IsTenantAdmin()（原 IsRoot 语义）
```

- `UserCtx.IsSuperAdmin/IsAdmin` 字段保留（pb 兼容）但改为由 user_type 填充：`IsSuperAdmin = IsTenantOwner()`、`IsAdmin = IsTenantAdmin()`；新代码一律用语义化方法。
- `roleApiAuthLogic` 改造：`IsSuperAdmin` 短路改读 `user_type`；`sys_access_info` 三档语义修正（admin 档显式化为 `user_type<=2` 判断，supper 档保持"default 租户"限制）——把现在"靠短路隐式实现"的行为变成显式规则。
- 角色 code `"admin"/"supper"/"client"` 从运行时判定路径退役，仅作为租户初始化的默认角色模板名。

---

## 7. 数据迁移方案

### 7.1 M1：建表与 1:1 回填（低风险）

1. 建 `sys_tenant_user`；按现 `sys_user_info` 每行生成一条 membership（tenant_code, user_id, user_type=按现公式重算, status, default_role_id=role, is_all_data, device_count, tags）。
2. 读路径切 membership（UserCheckToken），`sys_user_info` 的租户字段暂保留双写。
3. 上线对账任务。此步完成后即获得"单表查身份/成员列表"的全部收益。

### 7.2 M2：全局账号归并（高风险，dry-run 必须）

同一手机号/邮箱跨租户的多行账号归并为一个：

1. **归并键策略**（建议默认，可配置）：手机号或邮箱完全相同 → 归并；user_name/三方 openid 相同不自动归并（进人工清单）。
2. **主账号选择**：最近活跃（last_login/updated_time 最新）的行为主行；密码、档案取主行；被并行进入映射表 `sys_user_merge_map(old_user_id → account_user_id)`。
3. **FK 改写**（按 old_user_id 分批）：
   - saas：`sys_tenant_user.user_id`、`sys_user_role.user_id`、`sys_user_profile`、`sys_dept_user`、`sys_data_project/sys_data_area`(target=user)、`sys_tenant_info.AdminUserID`；
   - things：`dm_device_info.user_id`、`dm_user_device_share.shared_user_id`、`dm_user_device_collect`（设备量大，按租户+用户分批）；
   - 日志类（login/oper log）**不回写**，查询时 join 映射表解析。
4. 归并完成后建全局唯一索引（user_name/email/phone/union 类），拆除 tc 复合索引。
5. dry-run 报告：冲突清单（密码不同、档案不同、双方均活跃）→ 人工确认后执行。

### 7.3 M3：收尾

- 登录切两步/租户切换/LastTokenID 挪位/三方绑定表迁移（openid 数据搬 `sys_user_oauth_binding`）。
- 删除 `sys_user_info.tenant_code` 及双写代码；删除身份推导死代码（deptAuth 注释块、IsAllData 注释块、未消费常量）。

> 注：7.2 与 7.3 顺序可对调（先切登录再归并，或先归并再切登录）。建议先归并后切登录：两步登录的"租户列表"必须基于归并后的账号，否则列表会重复。

---

## 8. 风险与开放问题

| # | 风险/问题 | 处置 |
|---|---|---|
| 1 | **密码全局化扩大爆破面**：一个密码通所有租户 | 配合登录失败锁定（已有 LOCKOUT 类逻辑可加）、异地/新设备提醒；敏感租户可配置"进入需二次验证" |
| 2 | 微信/华为 openid per-应用，全局化会撞唯一索引 | 已拆 `sys_user_oauth_binding`（3.3） |
| 3 | C 端用户多租户的产品语义（设备分属不同租户） | things 查询全链路带租户上下文，天然隔离；但 App 端"切换租户"交互需产品确认 |
| 4 | 归并策略误伤（同人故意两套账号） | 默认只按手机/邮箱归并 + dry-run 人工清单；支持标记"不归并" |
| 5 | things 侧 FK 批量改写量大 | 按 (tenant, old_user_id) 分批 + 限速；改写期间新旧双读（先查新 id 未命中再查映射） |
| 6 | 开放认证 token 里的用户语义变化 | open access 行校验增加"membership 有效"；版本化 token |
| 7 | `sys_tenant_info.AdminUserID` 改写窗口与 user_type 重算的一致性 | 同事务；对账兜底 |
| 8 | 老 token 兼容 | 迁移窗口强制重登（claims 缺 tenant 直接判过期），不做长期双轨 |

---

## 9. 分期收益复述

- **M1 后**：身份判定单一来源（读列）、租户管理员/客户列表单表查询、双路径推导不一致消除、命名语义修正——即前述"身份物化"的全部收益，风险接近零。
- **M2 后**：一人一号、每租户独立身份与会话、三方绑定归位。
- **M3 后**：死代码清理，`sys_user_info` 纯净为全局账号表。
- 与 tenant-hierarchy.md 衔接：届时"租户超级管理员"= 子树根的 Owner，`user_type` 不加枚举、只加闭包表推导，membership 模型无需再动。

## 附录：关键现状证据

- `sys_user_info` 唯一索引全部带 tc 前缀：`saas/service/syssvr/internal/repo/relationDB/modelUser.go:14-29`
- 双路径推导不一致：`userChecktokenlogic.go:190`（按租户表绑定）vs `:147`（按 supper 字符串）
- 角色 code 为运行时判据：`userChecktokenlogic.go:149,193`
- `UserSearch` 取第一条：`apisvr/.../self/userSearchLogic.go:35-46`
- 租户初始化三角色：`tenantmanage/tenantInfoCreateLogic.go:119-153`
- TB 对照（全局唯一 email、authority 物化列）：`thingsboard/dao/src/main/resources/sql/schema-entities.sql:437-449`
