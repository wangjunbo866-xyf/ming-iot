# EMQX 路由表设计走读（供后续扩展参考）

> 源码级走读 EMQX 5.x 的路由表实现，目的是为 mqtt-gateway 后续可能的「opt-in 通用 topic 路由」（见 [doc #14](../../14.cloud-mqtt-gateway-downlink-topic-routing-design.md)）储备设计参考。
> EMQX 源码：本地 `/home/wangjunbo/workspace-iot/emqx`（5.x，apps/emqx/src）。
> 走读日期：2026-06-27。

---

## 0. 一句话总览

EMQX 的路由表把"谁订了什么"压缩成 **topic → node 的去重边表**，并把**精确 topic（哈希表 O(1)）**和**通配 filter（有序集上的 skip-search）**分两张表存；跨节点用 **mria 异步复制**。最妙的是它的通配匹配是**无指针 trie**——在 ETS ordered_set 上用"跳跃式有序搜索"实现 trie 语义，复杂度 O(匹配数 × 层级)，且对 Go 的 GC 友好性有直接借鉴意义。

---

## 1. 分层（别和会话注册表搞混）

EMQX 有两个独立的"谁在哪"结构，职责不同：

| 结构 | 映射 | 用途 | 模块 |
|---|---|---|---|
| **路由表** | topic → node | **发布路由**：消息该转发到哪些节点 | `emqx_router` / `emqx_broker` |
| **会话注册表** | clientid → node/pid | **会话唯一性**：takeover/kick 同名连接 | `emqx_cm_registry` / `emqx_cm` |

本文只讲**路由表**（注册表见 [connect-auth-design-review.md](./connect-auth-design-review.md) §2.2）。

路由的三层：
```text
emqx_broker      ：节点本地订阅表（topic→本节点哪些 client），以及"首订阅才写路由/末退订才删路由"的去重闸
emqx_router      ：集群路由表（topic→node），mria 复制
emqx_trie_search ：通配匹配算法（在有序集上跑）
```

---

## 2. 核心数据结构：两张表，精确与通配分离

`emqx_router:create_tables_v3/0`（[emqx_router.erl:137-195](../../../../emqx/apps/emqx/src/emqx_router.erl#L137)）建**两张** mria 表：

```text
ROUTE_TAB（bag）         ：精确 topic → dest。 #route{topic, dest}
   例：#route{topic = <<"foo/bar/baz">>, dest = 'emqx@node1'}
   查找 = ets:lookup(Topic) → O(1) 哈希

ROUTE_TAB_FILTERS（ordered_set）：通配 filter → dest，存成 topic_index key
   例：{routeidx, {[<<"foo">>,'+',<<"bar">>], {'emqx@node1'}}}   % foo/+/bar
   匹配 = trie_search 在有序集上跳跃搜索
```

**写入只进其中一张**（[emqx_router.erl:386-393](../../../../emqx/apps/emqx/src/emqx_router.erl#L386)）：`emqx_trie_search:filter(Topic)` 判断是不是通配——是 → filters 表；否 → route 表。

**发布匹配 = 两表相加**（[emqx_router.erl:424-432](../../../../emqx/apps/emqx/src/emqx_router.erl#L424)）：
```erlang
match_routes_v2(Topic) ->
    lookup_route_tab(Topic)                        %% 精确：O(1) ets:lookup
      ++ [match_to_route(M) || M <- match_filters(Topic)].  %% 通配：trie_search
```

> **借鉴点①**：绝大多数订阅是精确 topic（尤其设备自己的命令 topic）。把它们放哈希表走 O(1)，**只有通配才付匹配代价**。这是路由不拖垮发布热路径的第一招。

`dest = node() | {group(), node()}`（[emqx_router.erl:87](../../../../emqx/apps/emqx/src/emqx_router.erl#L87)）——普通订阅 dest=node，**共享订阅** dest={group,node}（共享语义编码进 dest，不另建表）。

---

## 3. 关键设计：topic → **node** 去重（路由表为什么不爆）

路由表的 dest 是 **node 不是 client**。节点上有 1 万个 client 订 `foo`，路由表里只有**一条** `foo→本node`。这靠 broker 的引用去重实现：

- 订阅时（[emqx_broker.erl:655, 802-804](../../../../emqx/apps/emqx/src/emqx_broker.erl#L802)）：`maybe_add_route(Existed, Topic, _)`——`Existed=false`（本节点该 topic 的**首个**订阅者）才 `sync_route(add)`；已存在则只更新本地订阅表，**不碰集群路由**。
- 退订时（[emqx_broker.erl:706-723](../../../../emqx/apps/emqx/src/emqx_broker.erl#L706)）：本节点**最后一个**订阅者走了才 `maybe_delete_route`。

```text
路由表规模 = O(唯一 topic × node)，不是 O(订阅数 / 设备数)
跨节点复制量 = 每个 (topic,node) 边只复制一次
```

> **借鉴点②（最重要）**：这正印证了我们的结论——**设备自身的 exact topic 根本不该膨胀集群路由**。EMQX 用"per-node 去重"把它压到边表；我们更进一步用 **presence 点查** 处理设备下行（连边表都不需要），通配路由表只留少量广订阅。两者同源：**别让 N 个设备订阅变成 N 条复制条目。**

---

## 4. 通配匹配：无指针 trie（对 Go GC 的直接启发）

`emqx_trie_search`（[emqx_trie_search.erl:18-85](../../../../emqx/apps/emqx/src/emqx_trie_search.erl#L18)）不是 pointer-linked 树，而是**在 ETS ordered_set 上跑 trie 语义**：filter 解析成有序 key，匹配时利用"有序集能找到给定前缀的 immediate-next"，**跳跃式**跳过不可能匹配的区间。

```text
朴素枚举所有可能订阅 = O(2^层级)        ← 不可行
朴素扫所有订阅       = O(订阅总数×层级)  ← 百万级太慢
EMQX skip-search    = O(匹配数 N × 层级 L) ← N 是真正命中的条数（emqx_trie_search.erl:84）
```

机制（源码注释的精华，a/b/c 匹配示例）：从有序集某点出发，比较当前 key 与目标 topic 的公共前缀，据此**猜下一个 word 能否是 `+`/具体值，并 ets:next 跳到下一个可能匹配的前缀**，跳过整段不可能命中的区间；`#` 只是一次"是否以 # 结尾"的额外比较。

> **借鉴点③（解你的 Go GC 顾虑）**：你之前担心"通配路由 trie 是指针密集结构 → Go GC 墙 / 堆外难"。EMQX 给了第三条路：**不要 pointer-linked trie，用一个有序结构（Go 里可用有序 slice / B-tree / 跳表）+ skip-search 实现 trie 语义**。有序数组是 noscan 友好的，匹配是顺序/跳跃访问（cache 友好），**既躲开 GC 墙又不必手写堆外分配器**。这比"堆外 pointer trie"优雅得多，是你做 opt-in 通配匹配器时最该抄的一招。

---

## 5. 复制与并发（mria，异步弱一致）

- **存储**：`ram_copies`（即 ETS，堆外 C 存储，**不归 BEAM 进程 GC 管**）+ `rlog_shard`（[emqx_router.erl:145-163](../../../../emqx/apps/emqx/src/emqx_router.erl#L145)）。
- **写**：`mria:dirty_write` / `mria:async_dirty`（[emqx_router.erl:260, 299](../../../../emqx/apps/emqx/src/emqx_router.erl#L260)）——**异步、弱一致**，不是 raft 强一致（对比 comqtt 用强 raft）。
- **批量**：`do_batch` → `mria:async_dirty(..., mria_batch_run, Batch)`（[emqx_router.erl:294-299](../../../../emqx/apps/emqx/src/emqx_router.erl#L294)）；`emqx_broker` 经 `emqx_router_syncer:push` 批量下推（[emqx_broker.erl:850-852](../../../../emqx/apps/emqx/src/emqx_broker.erl#L850)）——**合并写，削复制开销**。
- **分片**：`pick(Topic) = gproc_pool:pick_worker(router_pool, Topic)`（[emqx_router.erl:353](../../../../emqx/apps/emqx/src/emqx_router.erl#L353)）——路由写按 topic 哈希分到一组 router 进程，**避免单点写竞争**。
- **核心/副本**：mria core/replicant，replicant 异步只读副本；v3 用 `merge_table`+`auto_clean`+`node_pattern`，节点掉线**自动清理其路由**（[emqx_router.erl:145-151, 331-340](../../../../emqx/apps/emqx/src/emqx_router.erl#L331)）。

> **借鉴点④**：路由用**异步弱一致 + 批量 + 分片写**，不要强一致（前文已论证路由不需要强一致）。我们若加通配路由表，同样走"gossip/异步同步 + 批量"，不上 raft。

---

## 6. schema 演进史 = 路由表被反复重构（旁证它是最难的部分）

`emqx_router` 维护 v1/v2/v3 三套 schema + 复杂的集群版本协商（[emqx_router.erl:89-93, 580-741](../../../../emqx/apps/emqx/src/emqx_router.erl#L580)）：

```text
v1：已废弃（5.10 起不支持）
v2：常规 mria 表 + dirty_shard
v3：mria merged tables + auto_clean（当前）—— 改进点是节点掉线自动清路由、读写并发
```

启动时全集群协商用哪个版本，冲突就拒绝启动（防止半升级状态下路由不一致）。

> **启示**：连 EMQX 都把路由表存储**重构了两三次**——这是整个 broker 里最难、最容易踩坑的子系统。**这正是我们红线 #10 一阶段不做通用路由的理由**：能用 presence 点查解决就别碰它；真要做也要当成一等大工程，参考 EMQX 的分表/去重/skip-search/异步批量，别从零拍脑袋。

---

## 7. 映射到 mqtt-gateway（后续扩展时的 checklist）

当客户提出"通用 topic 订阅"需求、你要实现 [doc #14 的 opt-in topic routing](../../14.cloud-mqtt-gateway-downlink-topic-routing-design.md) 时，按 EMQX 这套抄：

```text
✅ 抄：
  1. exact / wildcard 分表 —— 精确 topic 走哈希点查（或干脆 presence），只有通配进匹配器。
  2. topic→node 去重 —— 设备级订阅永远不进集群表；表规模 = O(唯一通配 filter × node)。
  3. skip-search 匹配器 —— 有序结构（slice/B-tree/跳表）+ 跳跃搜索，而非 pointer-linked trie
     （noscan 友好、cache 友好、躲 Go GC 墙）。
  4. 异步弱一致 + 批量 + 按 topic 分片写 —— 不上 raft。
  5. dest 编码共享订阅 —— {group,node}，不另建表。

❌ 别抄：
  - mria / Erlang 分布式 —— 我们外置 Redis/Kafka，集群协调不在进程内复制。
  - 全功能 broker —— retain / 持久会话 / QoS2 仍按红线 #10 排除。
  - 强一致路由 —— 路由不需要（连 EMQX 都用异步弱一致）。
```

**一句话**：EMQX 路由表的精华是「**去重成边表 + 精确/通配分治 + 无指针 skip-search 匹配 + 异步批量复制**」。这四样和我们的红线/GC 约束完全兼容，是后续扩展的现成蓝本；而它的 mria/Erlang 复制底座我们用 Redis/Kafka 外置替代。

---

## 参考源码

- 路由表：`emqx/apps/emqx/src/emqx_router.erl`
- 本地订阅 + 去重闸：`emqx/apps/emqx/src/emqx_broker.erl`
- 通配索引 API：`emqx/apps/emqx/src/emqx_topic_index.erl`
- skip-search 匹配算法：`emqx/apps/emqx/src/emqx_trie_search.erl`
- 批量同步：`emqx/apps/emqx/src/emqx_router_syncer.erl`
- 掉线清理：`emqx/apps/emqx/src/emqx_router_helper.erl`
- 共享订阅：`emqx/apps/emqx/src/emqx_shared_sub.erl`
- 会话注册表（对比）：`emqx/apps/emqx/src/emqx_cm_registry.erl`
