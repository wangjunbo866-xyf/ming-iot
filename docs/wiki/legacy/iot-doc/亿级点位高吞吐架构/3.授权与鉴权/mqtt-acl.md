# EMQX Topic ACL 设计（client attributes 双占位符通配）

> 状态：入口契约的一部分，与主架构文档 §7.1 一致。适用一期 EMQX 5.8；下一期 self-hosted Gateway 沿用同一语义。

## 目标

- 设备走普通连接/普通订阅（非共享订阅），必须用严格 ACL 保证"认证主体只能操作自己的 Topic"；
- ACL 规则数量与设备数量**无关**：设备增删只改认证数据源，不改授权规则；
- product 与 device 两级各自受控，网关 prefix 受控；
- 默认拒绝，最小授权。

## 前提

- 认证 username 固定为 identity 的合并形式：
  - 直连设备：`<productKey>.<deviceKey>`（如 `p123.d456`）
  - 网关：`<gatewayProductKey>.<gatewayDeviceKey>`（如 `gw1.g1`）
- `productKey/deviceKey` 均禁止含 `.` `/` `+` `#` `*` `>` 等字符（主架构 §7.1 token 规则，注册时强制校验），因此 username 用 `tokens(username, '.')` 拆分无二义；
- 设备凭证**不得**标记为 superuser（superuser 绕过全部 ACL 检查）；
- 授权规则自上而下 first-match 命中即生效，以拒绝收尾。

## 第一步：从 username 拆出 client attributes

```hocon
# emqx.conf：连接建立时从 username 机械拆出两个属性
mqtt.client_attrs_init = [
  {expression = "nth(1, tokens(username, '.'))", set_as_attr = productKey},
  {expression = "nth(2, tokens(username, '.'))", set_as_attr = deviceKey}
]
```

属性由**已认证身份**机械拆解：username 与密码对不上平台记录就连不上，因此 `${client_attrs.productKey}/${client_attrs.deviceKey}` 的可信度等同 `${username}`，但授权粒度细到 product、device 两段各自独立。后续如需更强绑定，可由 JWT/HTTP 认证在认证响应里直接返回 `client_attrs`（认证器返回值会覆盖 `client_attrs_init` 的提取值）。

## 第二步：授权规则

优先用 file ACL（acl.conf）承载以下规则；**`${client_attrs.*}` 是否被 file ACL 的 topic 替换引擎支持需 Phase 0 实测**（官方文档仅在 Built-in Database authorizer 示例中明确给出该用法）。不支持时改用 Built-in DB authorizer 存同样的规则集——规则仍是少量静态通配，不是每设备一条。

```erlang
%% ── 平台侧身份 ──
%% 上行桥接：Access Adapter 以共享订阅接管全量上行
{allow, {user, "platform.access-adapter"}, subscribe, ["$share/+/iot/v1/up/#", "iot/v1/up/#"]}.
%% 下行发布：EMQX publish adapter 向任意设备投递已编码命令
{allow, {user, "platform.publish-adapter"}, publish, ["iot/v1/down/#"]}.
%% presence：消费客户端上下线事件（按需裁剪 $SYS 范围）
{allow, {user, "platform.access-adapter"}, subscribe, ["$SYS/brokers/+/clients/#"]}.

%% ── 直连设备：只能发布自己的上行、订阅自己的下行 ──
{allow, all, publish,   ["iot/v1/up/+/d/${client_attrs.productKey}/${client_attrs.deviceKey}/+"]}.
{allow, all, subscribe, ["iot/v1/down/+/d/${client_attrs.productKey}/${client_attrs.deviceKey}/+"]}.

%% ── 网关：只能以自己的 prefix 收发（child 段任意，拓扑由平台复核）──
{allow, all, publish,   ["iot/v1/up/+/g/${client_attrs.productKey}/${client_attrs.deviceKey}/#"]}.
{allow, all, subscribe, ["iot/v1/down/+/g/${client_attrs.productKey}/${client_attrs.deviceKey}/#"]}.

%% ── 默认拒绝其他所有 ──
{deny, all}.
```

同时设置 `authorization.no_match = deny`，保证规则未命中时拒绝。

## 语义与边界

- 直连设备无法冒用其他 product/device：属性来自认证 username，伪造 productKey/deviceKey 属性等于伪造凭证；
- 直连凭证可以在 gateway grammar 位置用自己的属性伪造 child（`iot/v1/up/x/g/p123/d456/anything` 会命中网关通配规则）——这是通配方案的固有空隙，由 ProtocolApplication 的 DeviceBindingCache 复核关闭：identity 的注册类型（直连/网关）与 gateway-child membership 不符即进 quarantine。**因此协议层拓扑校验不可省略，ACL 只做第一道门**；
- `<messageKind>` 段用 `+` 通配，合法 kind 集合由 Adapter 语法校验与 ProtocolApplication 裁决，不进 ACL（避免 kind 演进引发授权规则变更）；
- `<protocolKey>` 段用 `+` 通配，合法 key 由控制面发布集合约束；设备只持有自己产品的协议凭证，发错 key 会在 ProtocolApplication 解析阶段进 quarantine；
- 平台侧身份（`platform.*`）使用独立凭证并受连接来源限制；变更这些规则需要与凭证轮换同样的变更流程；
- file ACL 变更通过受控同步任务下发并 reload；Built-in DB authorizer 规则经 REST API 受控写入。两者都纳入版本确认；设备接入/吊销只操作认证数据源，不触发授权规则变更；
- 需要入口即硬隔离的大租户不使用本共享规则集，改用专属 protocolKey/broker/NATS account（主架构 §14.5）。

## 与下行命令的衔接

`things/runtime/command` 的 CommandAttempt 冻结的下行 Topic 必须落在上述语法内：

```text
直连  iot/v1/down/<protocolKey>/d/<productKey>/<deviceKey>/<messageKind>
网关  iot/v1/down/<protocolKey>/g/<gatewayProductKey>/<gatewayDeviceKey>/<childProductKey>/<childDeviceKey>/<messageKind>
```

EMQX publish adapter 只是 transport 投递者；设备侧由 subscribe ACL 保证只能收到自己的命令 Topic。

## 参考

- [EMQX Client Attributes](https://docs.emqx.com/en/emqx/latest/client-attributes/client-attributes.html)：`client_attrs_init` 提取表达式与 `${client_attrs.*}` 用法（官方示例即 productId/deviceId 两级通配授权）；
- [EMQX File-based ACL](https://docs.emqx.com/en/emqx/latest/access-control/authz/file.html)：acl.conf 规则格式、first-match、`{client_attr, ...}` 匹配位；
- [EMQX Authorization Placeholders](https://docs.emqx.com/en/emqx/latest/access-control/authz/authz.html)：各授权源的占位符支持矩阵。
