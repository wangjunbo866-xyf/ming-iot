# 方案一：租户层级闭包表
**数据权限节点：租户节点**或**部门节点**，呈多层树状结构。

**数据权限继承：**父节点自动拥有所有子节点的数据权限。

**独立建模：** 独立层级闭包表(`**tenant_hierarchy、dept_hierarchy**`)来存储节点之间的父子关系。

**数据与节点关联：**在需要隔离的业务数据表中，添加 `**tenant_id、dept_id**` 字段指向具体的租户节点（通常是叶子节点或数据实际所属的租户）。

**权限基于层级推导：**通过查询层级关系表，动态确定租户（及其所有子孙）的ID集合，用于数据过滤。

## 租户数据节点
```go
// 租户层级表
type SysTenantHierarchy struct {
    // id编号
    ID                   int64  `gorm:"column:id;type:BIGINT;primary_key;AUTO_INCREMENT"`
    // 祖先租户编码
    AncestorTenantCode   string `gorm:"column:ancestor_tenant_code;type:VARCHAR(50);uniqueIndex:ad_d;NOT NULL"`
    // 后代租户编码
    DescendantTenantCode string `gorm:"column:descendant_tenant_code;type:VARCHAR(50);uniqueIndex:ad_d;NOT NULL"`
    //层级深度
    Depth                int8   `gorm:"column:depth;type:TINYINT;NOT NULL"`                                      
    stores.NoDelTime
    DeletedTime stores.DeletedTime `gorm:"column:deleted_time;default:0;uniqueIndex:ad_d"`
}

func (m *SysTenantHierarchy) TableName() string {
    return "sys_tenant_hierarchy"
}
```

**数据示例：**

| ancestor_tenant_code | descendant_tenant_code | depth |
| --- | --- | --- |
| 1 (Root) | 1 (Root) | 0 |
| 1 (Root) | 2 (ChildA) | 1 |
| 1 (Root) | 3 (ChildB) | 1 |
| 1 (Root) | 4 (Grandchild) | 2 |
| 1 (Root) | 5（5） | 3 |
| 1 (Root) | 6（6） | 4 |
| 2 (ChildA) | 2 (ChildA) | 0 |
| 2 (ChildA) | 4 (Grandchild) | 1 |
| 2 (ChildA) | 5（5） | 2 |
| 2 (ChildA) | 6（6） | 3 |
| 3 (ChildB) | 3 (ChildB) | 0 |
| 4 (Grandchild) | 4 (Grandchild) | 0 |
| 4 (Grandchild) | 5（5） | 1 |
| 4 (Grandchild) | 6（6） | 2 |
| 5（5） | 5（5） | 0 |
| 5（5） | 6（6） | 1 |
| 6（6） | 6（6） | 0 |




```go
func (p TenantHierarchyRepo) DeleteParentHierarchies(ctx context.Context, tenantCode string) error {
    return p.db.WithContext(ctx).Exec(`
        DELETE FROM sys_tenant_hierarchy 
        WHERE descendant_tenant_code IN (
            SELECT descendant_tenant_code 
            FROM sys_tenant_hierarchy 
            WHERE ancestor_tenant_code = $1
        ) 
        AND ancestor_tenant_code IN (
            SELECT ancestor_tenant_code 
            FROM sys_tenant_hierarchy 
            WHERE descendant_tenant_code = $1 AND ancestor_tenant_code != descendant_tenant_code
        )
    `, tenantCode).Error
}

func (p TenantHierarchyRepo) InsertParentHierarchies(ctx context.Context, tenantCode, ParentTenantCode string) error {
    return p.db.WithContext(ctx).Exec(`
        INSERT INTO sys_tenant_hierarchy (ancestor_tenant_code, descendant_tenant_code, depth)
        SELECT 
            supertree.ancestor_tenant_code, 
            subtree.descendant_tenant_code, 
            supertree.depth + subtree.depth + 1
        FROM 
            sys_tenant_hierarchy AS supertree
        CROSS JOIN 
            sys_tenant_hierarchy AS subtree
        WHERE 
            supertree.descendant_tenant_code = $2
            AND subtree.ancestor_tenant_code = $1
    `, tenantCode, ParentTenantCode).Error
}
```

+ **优势:**
    - **查询高效：** 查找一个租户的所有子孙（用于数据权限过滤）只需 `**WHERE ancestor_id = ?**`。
    - **查找祖先容易：** `**WHERE descendant_id = ?**`。
    - **计算深度简单：** 直接有 `**depth**` 字段。
    - **避免递归查询：** 在大多数数据库（尤其是MySQL）上性能远优于递归CTE。
+ **维护：** 需要在 `**tenants**` 表插入、更新（移动节点）、删除时，通过触发器或应用逻辑同步维护此表（计算新路径）。这是为查询性能付出的合理代价。

## 树形结构查询
### 系统超级管理员
可以增删改查所有租户

#### 整树查询
一次性把所有符合条件的租户查询出来，再转化为租户树。

+ 根据查询条件查询租户表，得到所有符合条件的租户ID；
+ 将租户列表转化为租户树（parentId）；

#### 分层查询
一层层查询

### 租户超级管理员
可以增删改查所有子租户

#### 整树查询
一次性把所有符合条件的租户查询出来，再转化为租户树

+ 根据查询条件查询租户表，得到所有符合条件的租户ID；
+ 根据租户层级表过滤符合条件的租户ID：祖先租户ID必须是当前账号登录租户ID；
+ 将租户列表转化为租户树（parentId）；

#### 分层查询
一层层查询

## 部门数据节点
todo

# 方案二：路径枚举
`**tenant_id**`** + **`**tenant_path**`** (路径枚举)：**

+ 在 `**tenants**` 表增加 `**path**` 字段 (如 `**/1/2/4/**` 表示 Root->ChildA->Grandchild)。
+ 查询子孙数据： `**SELECT ... WHERE tenant_path LIKE '/:currentTenantId/%'**` 或 `**WHERE tenant_path LIKE CONCAT(:currentTenantPath, '%')**`。
+ **优点：** 查询自身和直接子孙容易，维护比闭包表简单。
+ **缺点：**
    - 查找所有后代（非直接子孙）需要 `**LIKE**` 查询，可能无法有效利用索引，性能不如闭包表的直接 JOIN。
    - 移动节点代价高昂（需要更新该节点及其所有后代的 `**path**`）。
    - 依赖应用逻辑正确生成和维护路径字符串。



