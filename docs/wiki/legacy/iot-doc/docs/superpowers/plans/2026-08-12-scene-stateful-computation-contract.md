# Scene Stateful Computation Contract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立一份统一的《场景有状态计算规范》，把 `SceneBindingState`、`latest/rolling`、5 类有状态算子、Timer 与可靠延迟动作的边界讲清楚；同时在 `things` 中建立可校验、可生成、可做漂移检查的机器契约，作为后续代码生成的唯一输入。

**Architecture:** `iot-doc` 中的中文规范负责解释语义与设计理由；`things/contracts/scene/stateful-computation-spec.yaml` 负责机器可读字段、约束、成熟度和测试向量。生成器先用 JSON Schema 与语义校验器验证 YAML，再计算规范化 SHA-256，原子生成 Go 类型、校验器、注册表、黄金用例和 Markdown 能力目录，并把目录同步到中文规范的受控标记区。旧文档保留稳定锚点和短摘要，不再复制规范正文。

**Tech Stack:** Go 1.24.4、`gopkg.in/yaml.v3`、`github.com/santhosh-tekuri/jsonschema/v6`、JSON Schema Draft 2020-12、Markdown、SHA-256。

---

## 执行约束

- 本计划横跨两个独立 Git 仓库：`iot-doc` 与 `things`。每条命令都必须在标注的仓库目录执行。
- 工作区根目录 `AGENTS.md` 明确禁止未经授权执行 `git add`、`git commit`、`git push`。因此本计划用 `git status` 作为阶段检查点；即使技能模板通常建议频繁提交，本次也不得自动提交。
- `things/AGENTS.md` 要求新增类型、函数和关键业务分支有中文注释；生成的 Go 文件头也必须声明“由生成器生成，请勿手改”。
- 不实现 Scene Runtime 的真实状态机、Timer 调度器或可靠延迟动作执行器。本次交付止于规范、机器契约、生成物和校验工具。
- 设计依据固定为 [2026-08-12-scene-stateful-computation-contract-design.md](../specs/2026-08-12-scene-stateful-computation-contract-design.md)。若实施中发现设计无法机器化，先记录为明确缺口并暂停对应任务，不得悄悄改变语义。

## 最终文件图

### `iot-doc` 新建

- `亿级点位高吞吐架构/6.规则引擎与场景联动/场景有状态计算规范.md`

### `iot-doc` 修改

- `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`
- `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`
- `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`
- `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md`
- `亿级点位高吞吐架构/6.规则引擎与场景联动/水线与读取屏障详解.md`
- `亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md`

### `things` 新建

- `contracts/scene/stateful-computation-spec.yaml`
- `contracts/scene/stateful-computation-spec.schema.json`
- `contracts/scene/gen/types.gen.go`
- `contracts/scene/gen/validators.gen.go`
- `contracts/scene/gen/registry.gen.go`
- `contracts/scene/gen/catalog.gen.md`
- `contracts/scene/gen/testdata/golden-cases.gen.json`
- `tools/scene-contract-gen/main.go`
- `tools/scene-contract-gen/main_test.go`
- `tools/scene-contract-gen/internal/contract/model.go`
- `tools/scene-contract-gen/internal/contract/load.go`
- `tools/scene-contract-gen/internal/contract/validate.go`
- `tools/scene-contract-gen/internal/contract/canonical.go`
- `tools/scene-contract-gen/internal/contract/load_test.go`
- `tools/scene-contract-gen/internal/contract/validate_test.go`
- `tools/scene-contract-gen/internal/contract/canonical_test.go`
- `tools/scene-contract-gen/internal/generate/types.go`
- `tools/scene-contract-gen/internal/generate/validators.go`
- `tools/scene-contract-gen/internal/generate/registry.go`
- `tools/scene-contract-gen/internal/generate/fixtures.go`
- `tools/scene-contract-gen/internal/generate/markdown.go`
- `tools/scene-contract-gen/internal/generate/generate_test.go`
- `tools/scene-contract-gen/internal/output/atomic.go`
- `tools/scene-contract-gen/internal/output/atomic_test.go`
- `tools/scene-contract-gen/internal/docsync/catalog.go`
- `tools/scene-contract-gen/internal/docsync/links.go`
- `tools/scene-contract-gen/internal/docsync/catalog_test.go`
- `tools/scene-contract-gen/internal/docsync/links_test.go`
- `tools/scene-contract-gen/testdata/valid-minimal.yaml`
- `tools/scene-contract-gen/testdata/invalid-unknown-field.yaml`
- `tools/scene-contract-gen/testdata/invalid-semantic.yaml`

### `things` 修改

- `go.mod`
- `go.sum`
- `Makefile`

## Task 1: 建立中文规范并固定旧锚点

**Files:**

- Create: `iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动/场景有状态计算规范.md`
- Modify: `iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`
- Reference: `iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md:614`
- Reference: `iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md:201`

- [ ] **Step 1: 记录两个仓库的实施前基线**

Run:

```bash
git -C /home/wangjunbo/workspace-iot-platform/iot-doc status --short --branch
git -C /home/wangjunbo/workspace-iot-platform/things status --short --branch
```

Expected:

- `iot-doc` 只包含已经确认的设计文档和本计划文件；若还有其他修改，先记录并避开。
- `things` 为 clean；若不为 clean，暂停对重叠文件的操作。

- [ ] **Step 2: 新建规范骨架，先使用“机器契约尚未建立”占位标记**

新文档标题固定为 `# 场景有状态计算规范`，章节顺序固定为：

```markdown
# 场景有状态计算规范

## 1. 文档定位与规范边界
## 2. 统一术语与分类
## 3. SceneBindingState：状态容器、身份与状态层次
## 4. 统一求值顺序与 latest-value join
## 5. latest 与 rolling：状态组件
## 6. 五类有状态算子
### 6.1 Edge / Change
### 6.2 Duration / For
### 6.3 Debounce
### 6.4 Cooldown
### 6.5 Hysteresis
## 7. Script 与 Alarm/FSM 扩展边界
## 8. Timer：运行机制
## 9. Reliable Delayed Action：持久效果
## 10. 冷启动、恢复、NOT_READY、新鲜度与 fail-closed
## 11. Scene 小状态与 Flink 大状态的边界
## 12. 完整案例
## 13. 机器契约与生成物
## 14. 规范性能力目录
## 15. 实施检查表
```

第 14 章必须先写入以下唯一标记；不得伪造版本号或哈希：

```markdown
<!-- BEGIN GENERATED: stateful-computation-catalog -->
> 机器契约尚未建立。本区将在 `things` 中的契约生成器完成后自动替换。
<!-- END GENERATED: stateful-computation-catalog -->
```

以下显式锚点必须放在对应标题之前，并作为 YAML `docsAnchor` 的唯一取值来源：

| 章节/能力 | 显式锚点 |
|---|---|
| SceneBindingState | `state-container-scene-binding-state` |
| latest | `state-component-latest` |
| rolling | `state-component-rolling` |
| Edge / Change | `stateful-operator-edge-change` |
| Duration / For | `stateful-operator-duration` |
| Debounce | `stateful-operator-debounce` |
| Cooldown | `stateful-operator-cooldown` |
| Hysteresis | `stateful-operator-hysteresis` |
| Script | `extension-script` |
| Alarm/FSM | `extension-alarm-fsm` |
| Timer | `runtime-facility-timer` |
| Reliable Delayed Action | `durable-effect-reliable-delayed-action` |
| 冷启动、恢复、NOT_READY、新鲜度与 fail-closed | `stateful-not-ready-freshness-fail-closed` |

- [ ] **Step 3: 把现有规范正文迁入唯一归属章节**

按以下映射迁移；正文可以因上下文调整标题和过渡句，但规范词 `必须`、`不得`、`仅当`、`fail-closed` 的含义不得弱化：

| 新规范章节 | 迁移来源 |
|---|---|
| 第 3 章 | `场景联动 Runtime 主规格总纲.md` 第 8 节；`算子语义与术语.md` 第 15 节中 SceneBindingState 字段与持久化约束 |
| 第 4 章 | `算子语义与术语.md` 第 3、4、5 节 |
| 第 5 章 | `算子语义与术语.md` 第 15.6 节 rolling 投影；latest 的事实读取部分只做状态组件解释，不复制 Current Fact 写入协议 |
| 第 6 章 | `算子语义与术语.md` 第 7 至 11 节 |
| 第 7 章 | 主规格中的 Script、Alarm/FSM 描述；明确它们是扩展，不属于 5 类内建有状态算子 |
| 第 8 章 | `算子语义与术语.md` 第 13 节、主规格第 10 节 Timer 部分 |
| 第 9 章 | `算子语义与术语.md` 第 14 节、主规格第 10 节 delayed action 部分 |
| 第 10 章 | `算子语义与术语.md` 第 6、12 节，主规格第 9.5 节，并链接《水线与读取屏障详解》 |
| 第 11 章 | 旧文档中 Scene 小状态配额与 Flink rolling 大状态的边界 |
| 第 12 章 | `00-15分钟读懂场景联动.md` 的“3 个温度点求平均值 + 门状态 + 启动两台风机”案例，只展开有状态计算部分并回链完整端到端图 |

第 2 章必须显式给出以下分类，不允许再用 `SceneBindingState` 代指全部有状态能力：

| 中文统称 | 英文规范名 | 成员 |
|---|---|---|
| 场景有状态计算 | Scene Stateful Computation | 本规范全部能力 |
| 状态容器 | State Container | `SceneBindingState` |
| 状态组件 | State Component | `latest`、`rolling` |
| 有状态算子 | Stateful Operator | Edge/Change、Duration、Debounce、Cooldown、Hysteresis |
| 扩展能力 | Extension | Script、Alarm/FSM |
| 运行机制 | Runtime Facility | Timer |
| 持久效果 | Durable Effect | Reliable Delayed Action |

- [ ] **Step 4: 把旧文档降级为术语入口，并保留稳定锚点**

`算子语义与术语.md` 保留共享术语、别名、上下文速查表与兼容锚点。将原第 4 至 14 节和第 15.6 节的规范正文替换成不超过 5 行的摘要、指向新规范对应章节的链接，并在每个摘要前加入以下显式锚点：

```html
<a id="4-输入更新与-latest-value-join"></a>
<a id="5-统一求值顺序"></a>
<a id="6-missing-与-freshness"></a>
<a id="61-水线新鲜度与-fail-closed-的配合"></a>
<a id="7-edge--change"></a>
<a id="8-duration--for"></a>
<a id="9-debounce"></a>
<a id="10-cooldown"></a>
<a id="11-hysteresis"></a>
<a id="12-类型化冷启动"></a>
<a id="13-普通-timer-实施契约"></a>
<a id="14-reliable-delayed-action"></a>
<a id="156-rolling-投影补充"></a>
```

文件开头新增归属声明：

```markdown
> 本文只维护共享术语、别名和兼容入口。有状态计算的规范性语义统一归《场景有状态计算规范》；同一规则若在两处重复，以后者为准。
```

- [ ] **Step 5: 验证占位标记与兼容锚点完整**

Run from `iot-doc`:

```bash
rg -n "BEGIN GENERATED: stateful-computation-catalog|机器契约尚未建立|END GENERATED: stateful-computation-catalog" "亿级点位高吞吐架构/6.规则引擎与场景联动/场景有状态计算规范.md"
rg -n '<a id="(4-输入更新与-latest-value-join|5-统一求值顺序|6-missing-与-freshness|61-水线新鲜度与-fail-closed-的配合|7-edge--change|8-duration--for|9-debounce|10-cooldown|11-hysteresis|12-类型化冷启动|13-普通-timer-实施契约|14-reliable-delayed-action|156-rolling-投影补充)"></a>' "亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md"
git diff --check
git status --short
```

Expected: 第一条恰好 3 行；第二条恰好 13 行；`git diff --check` 无输出。

## Task 2: 更新文档导航与规范归属

**Files:**

- Modify: `iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`
- Modify: `iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`
- Modify: `iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md`
- Modify: `iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动/水线与读取屏障详解.md`
- Modify: `iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md`

- [ ] **Step 1: 主规格只保留架构摘要**

在主规格第 7 至 10 节开头增加规范归属表：

| 内容 | 规范正文 |
|---|---|
| SceneBindingState、latest/rolling、内建有状态算子、Timer 语义 | 《场景有状态计算规范》 |
| Current Fact 写入、frontier、terminal no-write | 《Scene current fact设计》 |
| 读取屏障、水线、新鲜度、retry/fail-closed | 《水线与读取屏障详解》 |
| Runtime 拓扑、执行阶段和跨组件协议 | 本主规格 |

将第 7 至 10 节中的重复规则缩为架构摘要与链接；保留 schema 示例时，标注“非机器契约，字段约束见统一规范及 YAML”。不得删除主规格中跨组件调用顺序、责任边界和不变量。

- [ ] **Step 2: 更新 README 和 15 分钟导读**

README 的推荐阅读顺序固定为：

1. `00-15分钟读懂场景联动.md`
2. `场景联动 Runtime 主规格总纲.md`
3. `场景有状态计算规范.md`
4. `Scene current fact设计.md`
5. `水线与读取屏障详解.md`
6. `算子语义与术语.md`

在导读第一次出现 `SceneBindingState`、`rolling` 或有状态算子的位置增加一句：

```markdown
`SceneBindingState` 是状态容器，`latest/rolling` 是状态组件；Edge/Change、Duration、Debounce、Cooldown、Hysteresis 才统称“有状态算子”。完整语义与代码生成约束见《场景有状态计算规范》。
```

- [ ] **Step 3: 更新水线文档与 SLO 文档的规范链接**

`水线与读取屏障详解.md` 中原来指向旧文档显式锚点 `61-水线新鲜度与-fail-closed-的配合` 的位置改为新规范第 10 章的 `[《场景有状态计算规范》](场景有状态计算规范.md#stateful-not-ready-freshness-fail-closed)`；旧锚点仍保留，仅用于外部兼容。

`SLO故障测试与实施.md` 中“统一求值顺序、SceneBindingState、Timer”的语义依据改为新规范；Current Fact frontier 和读取屏障的测试依据继续分别指向原专门文档。

- [ ] **Step 4: 搜索残余的错误归属**

Run from `iot-doc`:

```bash
rg -n "算子语义与术语\.md#(4-|5-|6-|61-|7-|8-|9-|10-|11-|12-|13-|14-|156-)" "亿级点位高吞吐架构/6.规则引擎与场景联动"
rg -n "SceneBindingState.*(统称|等同于).*有状态" "亿级点位高吞吐架构/6.规则引擎与场景联动"
git diff --check
git status --short
```

Expected: 第一条只允许命中旧文档自身的兼容说明；第二条无输出；`git diff --check` 无输出。

## Task 3: 建立机器契约模型、JSON Schema 与加载器

**Files:**

- Modify: `things/go.mod`
- Modify: `things/go.sum`
- Create: `things/contracts/scene/stateful-computation-spec.schema.json`
- Create: `things/tools/scene-contract-gen/internal/contract/model.go`
- Create: `things/tools/scene-contract-gen/internal/contract/load.go`
- Create: `things/tools/scene-contract-gen/internal/contract/load_test.go`
- Create: `things/tools/scene-contract-gen/testdata/valid-minimal.yaml`
- Create: `things/tools/scene-contract-gen/testdata/invalid-unknown-field.yaml`

- [ ] **Step 1: 先写加载器失败测试**

`load_test.go` 至少包含以下三个测试函数：

```go
func TestLoad接受符合Schema的最小契约(t *testing.T)
func TestLoad拒绝未知字段(t *testing.T)
func TestLoad拒绝不支持的SchemaVersion(t *testing.T)
```

测试调用固定 API：

```go
func Load(specYAML, schemaJSON []byte) (*Contract, error)
```

断言要求：

- `valid-minimal.yaml` 能加载，`Contract.Contract.ID == "scene-stateful-computation"`。
- `invalid-unknown-field.yaml` 在 `statefulOperators[0]` 中加入 `surprise: true`，错误文本包含 `surprise`。
- 把 `contract.schemaVersion` 改为 `2` 后，错误文本包含 `schemaVersion` 与 `1`。

- [ ] **Step 2: 运行测试，确认因缺少实现而失败**

Run from `things`:

```bash
go test ./tools/scene-contract-gen/internal/contract -run 'TestLoad' -count=1
```

Expected: 编译失败，提示 `Load` 或 `Contract` 未定义。

- [ ] **Step 3: 引入明确版本的解析与 Schema 依赖**

Run from `things`:

```bash
go get github.com/santhosh-tekuri/jsonschema/v6@v6.0.2 gopkg.in/yaml.v3@v3.0.1
```

Expected: 两个包出现在 `go.mod` 的直接依赖区，`go.sum` 更新。

- [ ] **Step 4: 实现机器契约的类型模型**

`model.go` 中定义以下稳定枚举和顶层结构，并为每个导出类型、字段组和约束结构写中文注释：

```go
type Maturity string

const (
	MaturityDraft        Maturity = "DRAFT"
	MaturitySpecified    Maturity = "SPECIFIED"
	MaturityCodegenReady Maturity = "CODEGEN_READY"
	MaturityImplemented  Maturity = "IMPLEMENTED"
)

type Category string

const (
	CategoryStateContainer  Category = "STATE_CONTAINER"
	CategoryStateComponent  Category = "STATE_COMPONENT"
	CategoryStatefulOperator Category = "STATEFUL_OPERATOR"
	CategoryExtension       Category = "EXTENSION"
	CategoryRuntimeFacility Category = "RUNTIME_FACILITY"
	CategoryDurableEffect   Category = "DURABLE_EFFECT"
)

type Contract struct {
	Contract          ContractMetadata `yaml:"contract" json:"contract"`
	StateContainer    Capability       `yaml:"stateContainer" json:"stateContainer"`
	StateComponents   []Capability     `yaml:"stateComponents" json:"stateComponents"`
	StatefulOperators []Capability     `yaml:"statefulOperators" json:"statefulOperators"`
	Extensions        []Capability     `yaml:"extensions" json:"extensions"`
	RuntimeFacilities []Capability     `yaml:"runtimeFacilities" json:"runtimeFacilities"`
	DurableEffects    []Capability     `yaml:"durableEffects" json:"durableEffects"`
}

type ContractMetadata struct {
	ID            string `yaml:"id" json:"id"`
	SchemaVersion int    `yaml:"schemaVersion" json:"schemaVersion"`
	SpecVersion   string `yaml:"specVersion" json:"specVersion"`
}

type Capability struct {
	ID                 string             `yaml:"id" json:"id"`
	DisplayNameZh      string             `yaml:"displayNameZh" json:"displayNameZh"`
	DisplayNameEn      string             `yaml:"displayNameEn" json:"displayNameEn"`
	GoTypeName         string             `yaml:"goTypeName" json:"goTypeName"`
	Category           Category           `yaml:"category" json:"category"`
	Maturity           Maturity           `yaml:"maturity" json:"maturity"`
	CatalogSummary     string             `yaml:"catalogSummary" json:"catalogSummary"`
	DocsAnchor         string             `yaml:"docsAnchor" json:"docsAnchor"`
	Identity           []string           `yaml:"identity,omitempty" json:"identity,omitempty"`
	CommonFields       *[]Field           `yaml:"commonFields,omitempty" json:"commonFields,omitempty"`
	Quotas             *[]Quota           `yaml:"quotas,omitempty" json:"quotas,omitempty"`
	ConfigFields       *[]Field           `yaml:"configFields,omitempty" json:"configFields,omitempty"`
	StateFields        *[]Field           `yaml:"stateFields,omitempty" json:"stateFields,omitempty"`
	Rules              []Rule             `yaml:"rules,omitempty" json:"rules,omitempty"`
	Inputs             []string           `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	Outputs            []string           `yaml:"outputs,omitempty" json:"outputs,omitempty"`
	Phases             []string           `yaml:"phases,omitempty" json:"phases,omitempty"`
	Transitions        []string           `yaml:"transitions,omitempty" json:"transitions,omitempty"`
	Policies           PolicyRefs         `yaml:"policies" json:"policies"`
	Timer              *TimerDependency   `yaml:"timer,omitempty" json:"timer,omitempty"`
	SemanticHashFields []string           `yaml:"semanticHashFields,omitempty" json:"semanticHashFields,omitempty"`
	Compatibility      Compatibility      `yaml:"compatibility" json:"compatibility"`
	GoldenCases        []GoldenCase       `yaml:"goldenCases,omitempty" json:"goldenCases,omitempty"`
	Gaps               []string           `yaml:"gaps,omitempty" json:"gaps,omitempty"`
	Implementation     *ImplementationRef `yaml:"implementation,omitempty" json:"implementation,omitempty"`
}
```

`CommonFields/Quotas/ConfigFields/StateFields` 必须使用 slice 指针：`nil` 表示 YAML 根本未声明，非 nil 的空 slice 表示“明确声明该能力没有此类字段”。这是 `CODEGEN_READY` 区分“无配置”与“契约漏写”的 fail-closed 门禁，不得改回普通 slice。

同时定义：

- `Field`：`name`、`goName`、`type`、`required`、`nullable`、`default`、`enum`、`minimum`、`maximum`、`minItems`、`maxItems`、`quotaRef`、`serialization`、`semantic`、`mutuallyExclusiveWith`、递归 `fields`。
- `TypeRef`：`kind`、`ref`、`collection`、`mapKeyKind`；`kind` 仅允许 `string/bool/int32/int64/float64/duration_ms/timestamp_ms/json_value/opaque_json/ref/object`，`collection` 仅允许 `scalar/list/map`。
- `Rule`：`kind`、`field`、`otherField`、`whenField`、`whenEquals`、`value`；`kind` 仅允许 `required_when/forbidden_when/less_than/const`，序列化大小由 `Quota` 表达。
- `Quota`：`id`、`limit`、`unit`、`enforcement`；`unit` 仅允许 `bytes/items`，`enforcement` 首版仅允许 `HARD_REJECT`。
- `PolicyRefs`：`coldBootstrap`、`notReady`、`freshness`、`recovery`。
- `TimerDependency`：`required`、`timerKinds`。
- `Compatibility`：`preserveOnCompatibleChange`、`breakingFields`。
- `GoldenCase`：`id`、`kind`、`given`、`expect`，后两个字段使用 `map[string]any`；`kind` 仅允许 `NORMAL/BOUNDARY/CANCEL/NOT_READY/FAILURE/COLD_START`。
- `ImplementationRef`：`package`、`symbol`、`contractTest`、`sinceSpecVersion`。

- [ ] **Step 5: 写完整 JSON Schema**

Schema 必须声明：

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "urn:unitedrhino:contracts:scene:stateful-computation-spec:schema:1",
  "title": "Scene Stateful Computation Contract",
  "type": "object",
  "additionalProperties": false,
  "required": [
    "contract",
    "stateContainer",
    "stateComponents",
    "statefulOperators",
    "extensions",
    "runtimeFacilities",
    "durableEffects"
  ]
}
```

为 `contract`、`capability`、`field`、`typeRef`、`rule`、`quota`、`policyRefs`、`timerDependency`、`compatibility`、`goldenCase`、`implementationRef` 建立 `$defs`。所有对象均设 `additionalProperties: false`；`schemaVersion` 固定为 `const: 1`；`specVersion` 使用 SemVer 正则 `^[0-9]+\\.[0-9]+\\.[0-9]+$`；契约 ID 使用 `^[a-z][a-z0-9-]*$`，能力 ID 使用 `^[a-z][a-z0-9_]*$`；Go 类型名使用 `^[A-Z][A-Za-z0-9]*$`；文档锚点必须以 `#` 开头。

- [ ] **Step 6: 实现 Schema 校验和严格 YAML 解码**

`Load` 的处理顺序固定为：

1. 用 `yaml.v3` 把 YAML 解成通用值。
2. 把通用值转换为 JSON。
3. 用 `jsonschema/v6` 编译并校验 JSON Schema。
4. 用 `yaml.Decoder.KnownFields(true)` 再解成 `Contract`，防止模型与 Schema 漂移。

Task 3 只完成结构校验；Task 4 实现 `Validate` 后再把它接到 `Load` 的最后一步，避免引入随后删除的临时桩。

所有错误用 `%w` 保留根因，并包含阶段前缀：`解析 YAML`、`校验 JSON Schema`、`解码机器契约`、`校验语义`。

- [ ] **Step 7: 运行加载器测试**

Run from `things`:

```bash
gofmt -w tools/scene-contract-gen/internal/contract
go test ./tools/scene-contract-gen/internal/contract -run 'TestLoad' -count=1
git diff --check
git status --short
```

Expected: 测试 PASS；`git diff --check` 无输出；不执行 Git 暂存或提交。

## Task 4: 实现语义校验与规范化哈希

**Files:**

- Create: `things/tools/scene-contract-gen/internal/contract/validate.go`
- Create: `things/tools/scene-contract-gen/internal/contract/validate_test.go`
- Create: `things/tools/scene-contract-gen/internal/contract/canonical.go`
- Create: `things/tools/scene-contract-gen/internal/contract/canonical_test.go`
- Create: `things/tools/scene-contract-gen/testdata/invalid-semantic.yaml`

- [ ] **Step 1: 先写语义失败测试**

测试至少覆盖：

```go
func TestValidate拒绝重复CapabilityID(t *testing.T)
func TestValidate拒绝分类放错数组(t *testing.T)
func TestValidate拒绝CODEGENREADY缺少GoldenCase(t *testing.T)
func TestValidate拒绝IMPLEMENTED缺少实现引用(t *testing.T)
func TestValidate拒绝CODEGENREADY缺少必需GoldenCase类别(t *testing.T)
func TestValidate拒绝无法解析的字段引用(t *testing.T)
func TestValidate拒绝不安全的Go字段名(t *testing.T)
func TestValidate拒绝Hysteresis缺少LowLessThanHigh规则(t *testing.T)
func TestValidate拒绝Rolling超过统一配额(t *testing.T)
```

`invalid-semantic.yaml` 固定包含两个问题：重复的 `duration` ID，以及 `CODEGEN_READY` 能力没有 `goldenCases`。测试断言错误通过 `errors.Join` 同时报告两个问题，而不是遇到首错即返回。

- [ ] **Step 2: 运行测试，确认失败**

Run from `things`:

```bash
go test ./tools/scene-contract-gen/internal/contract -run 'TestValidate' -count=1
```

Expected: 测试失败，提示语义校验尚未实现或断言不成立。

- [ ] **Step 3: 实现完整语义校验器**

公开 API 固定为：

```go
func Validate(contract *Contract) error
```

校验器必须一次收集并排序输出以下错误：

- 全局 capability ID、`GoTypeName` 唯一。
- 每个数组中的 `category` 与数组类别一致，`stateContainer` 必须为 `STATE_CONTAINER`。
- `CODEGEN_READY` 必须同时有 `NORMAL`、`BOUNDARY/CANCEL`、`NOT_READY/FAILURE`、`COLD_START` 四组黄金用例；`IMPLEMENTED` 除黄金用例外还必须有非空 `implementation.package/symbol/contractTest/sinceSpecVersion`。
- `DRAFT` 能力必须有非空 `gaps`；它不得生成强类型配置或状态，因此 `commonFields/configFields/stateFields/quotas` 必须为 nil，而不是显式空数组。
- `SPECIFIED` 可以有字段，但不得出现在 Go 类型注册表。
- `CODEGEN_READY` 必须显式声明 `configFields`，即使该数组为空；状态容器还必须显式声明 `commonFields/quotas`，其他 `CODEGEN_READY` 能力必须显式声明 `stateFields`。这样遗漏字段契约会失败，明确无配置则合法。
- `stateContainer.identity` 必须非空且每项都引用 `commonFields`；只有状态容器可设置 `identity/commonFields/quotas`，其他分类必须使用 `configFields/stateFields`。
- 字段名、Go 字段名在同一结构内唯一；递归对象同样校验。
- `ref` 必须指向一个已声明的 `GoTypeName`；非 `ref` 类型不得设置 `ref`。
- `map` 只允许 `mapKeyKind: string`；`scalar/list` 不得设置 `mapKeyKind`。
- `minimum <= maximum`、`minItems <= maxItems`；default 必须落在 enum 和数值区间内。
- `semanticHashFields`、Rule 中的字段引用、`mutuallyExclusiveWith` 都必须可解析。
- `rolling.maxSamples <= 1000`、`rolling.maxStateBytes <= 32768`；`SceneBindingState` 必须声明 `maxSerializedBytes=32768 bytes/HARD_REJECT` quota。
- `hysteresis` 必须声明 `low < high`；`cooldown.mode` 必须固定为 `FIXED`；`edge_change.edgeKind` 只在 `variant=EDGE` 时必填。
- `docsAnchor` 全局唯一。

- [ ] **Step 4: 先写规范化哈希测试**

固定 API：

```go
func CanonicalJSON(contract *Contract) ([]byte, error)
func CanonicalSHA256(contract *Contract) (string, error)
```

测试必须证明：

1. 同一 YAML 仅改变注释、空行和对象键顺序，哈希不变。
2. capability 数组顺序变化，哈希不变。
3. `catalogSummary` 或 `maturity` 变化，哈希变化。
4. 输出格式严格为 `sha256:` 加 64 个小写十六进制字符。

- [ ] **Step 5: 实现规范化算法**

算法固定为：

- 深拷贝已经通过校验的类型模型。
- capability 数组按 `category + id` 排序。
- 字段数组按 `name` 排序；规则按 `kind + field + otherField` 排序；黄金用例按 `id` 排序；enum、phases、transitions、timerKinds、breakingFields、semanticHashFields 按字典序排序。
- `given/expect` 使用 `encoding/json` 编码；Go 标准库负责稳定排序 map key。
- 不把 YAML 注释、原始键顺序、文件路径、生成时间放入规范化模型。
- 使用 `json.Encoder`、禁用 HTML 转义，并去掉 Encoder 追加的最后一个换行；输出保持单行 JSON，不附加时间戳。

实现 `Validate` 后，将 `Load` 的最后一步固定为 `Validate(contract)`；删除任何临时桩，并增加 `TestLoad拒绝语义无效契约`，断言阶段前缀为 `校验语义`。

- [ ] **Step 6: 运行契约层全部测试**

Run from `things`:

```bash
gofmt -w tools/scene-contract-gen/internal/contract
go test ./tools/scene-contract-gen/internal/contract -count=1
go test -race ./tools/scene-contract-gen/internal/contract -count=1
git diff --check
```

Expected: 普通测试和 race 测试均 PASS。

## Task 5: 编写第一版完整 YAML 契约

**Files:**

- Create: `things/contracts/scene/stateful-computation-spec.yaml`
- Test: `things/tools/scene-contract-gen/internal/contract/load_test.go`

- [ ] **Step 1: 先写完整契约目录测试**

新增：

```go
func Test正式契约能力目录与成熟度固定(t *testing.T)
```

测试读取正式 YAML 和 Schema，并精确断言：

```text
scene_binding_state = CODEGEN_READY
latest = CODEGEN_READY
rolling = CODEGEN_READY
edge_change = CODEGEN_READY
duration = CODEGEN_READY
debounce = CODEGEN_READY
cooldown = CODEGEN_READY
hysteresis = CODEGEN_READY
timer = CODEGEN_READY
reliable_delayed_action = SPECIFIED
script = DRAFT
alarm_fsm = DRAFT
```

同时断言没有任何 `IMPLEMENTED` 能力。

- [ ] **Step 2: 运行测试，确认正式 YAML 缺失**

Run from `things`:

```bash
go test ./tools/scene-contract-gen/internal/contract -run 'Test正式契约能力目录与成熟度固定' -count=1
```

Expected: FAIL，提示正式契约文件不存在。

- [ ] **Step 3: 写契约元数据和能力目录**

固定元数据：

```yaml
contract:
  id: scene-stateful-computation
  schemaVersion: 1
  specVersion: 1.0.0
```

能力 ID、类别、Go 类型名固定如下：

| ID | Category | GoTypeName |
|---|---|---|
| `scene_binding_state` | `STATE_CONTAINER` | `SceneBindingState` |
| `latest` | `STATE_COMPONENT` | `LatestValueState` |
| `rolling` | `STATE_COMPONENT` | `RollingState` |
| `edge_change` | `STATEFUL_OPERATOR` | `EdgeChangeState` |
| `duration` | `STATEFUL_OPERATOR` | `DurationState` |
| `debounce` | `STATEFUL_OPERATOR` | `DebounceState` |
| `cooldown` | `STATEFUL_OPERATOR` | `CooldownState` |
| `hysteresis` | `STATEFUL_OPERATOR` | `HysteresisState` |
| `script` | `EXTENSION` | `ScriptState` |
| `alarm_fsm` | `EXTENSION` | `AlarmFSMState` |
| `timer` | `RUNTIME_FACILITY` | `TimerState` |
| `reliable_delayed_action` | `DURABLE_EFFECT` | `ReliableDelayedAction` |

每个 `docsAnchor` 必须从 Task 1 的显式锚点表逐项取值并带 `#` 前缀，例如 `edge_change` 使用 `#stateful-operator-edge-change`；不得依赖渲染器生成中文 slug。

所有 `CODEGEN_READY` 条目都必须在 YAML 中显式写 `configFields`：rolling 和 5 类算子写完整字段，`scene_binding_state/latest/timer` 写 `configFields: []`，表示已确认没有独立配置结构，而不是漏写契约。

- [ ] **Step 4: 固定状态容器和状态组件字段**

`scene_binding_state.identity` 固定为 `[tenantCode, bindingId, evaluationSubjectKey]`，并显式写 `configFields: []`。`commonFields` 首版固定为以下字段；`scriptState` 与 `alarmState` 使用 `opaque_json`，因为扩展内部契约仍为 DRAFT：

```text
schemaVersion:int32
tenantCode:string
bindingId:string
evaluationSubjectKey:string
processingNamespace:string
processingMode:string
identityAlgorithmVersion:string
configRevision:int64
semanticContractHash:string
lastSubjectAggregateVersion:int64 nullable
arguments:map<string,LatestValueState>
rollingArguments:map<string,RollingState>
edgeChangeStates:map<string,EdgeChangeState>
durationStates:map<string,DurationState>
debounceStates:map<string,DebounceState>
cooldownStates:map<string,CooldownState>
hysteresisStates:map<string,HysteresisState>
timerStates:map<string,TimerState>
members:map<string,opaque_json>
scriptState:opaque_json nullable
alarmState:opaque_json nullable
```

容器 quota 固定为 `id=maxSerializedBytes`、`limit=32768`、`unit=bytes`、`enforcement=HARD_REJECT`。

`latest` 显式写 `configFields: []`；状态字段固定为：`initialized`、`value`、`tombstone`、`eventTs`、`factVersion`、`observedAt`、`sourceEventId`、`lineage`。`lineage` 使用 `opaque_json`，并在摘要中指向 Current Fact 的 lineage 规范。

`rolling` 配置字段固定为：

```text
windowDurationMs:duration_ms required minimum=1 semantic=true
maxSamples:int32 required minimum=1 maximum=1000 default=1000 semantic=true
maxStateBytes:int32 required minimum=1 maximum=32768 default=32768 semantic=true
bootstrapPolicy:string required enum=[EMPTY,LOAD_BOUNDED_HISTORY] semantic=true
```

`rolling` 状态字段固定为：`samples:list<object>`、`sampleIdentityIndex:map<string,int32>`、`windowStartTs`、`windowEndTs`、`serializedBytes`；单个 sample 的递归字段为 `sampleId`、`eventTs`、`factVersion`、`value`、`sourceEventId`。

- [ ] **Step 5: 固定 5 类有状态算子的字段和交叉约束**

| 算子 | 配置字段 | 状态字段 | 必需规则 |
|---|---|---|---|
| `edge_change` | `variant={EDGE,CHANGE}`、`edgeKind={RISING,FALLING,ANY}`、`coldBootstrap={BASELINE_ONLY,EMIT_INITIAL}` | `initialized`、`previousPredicate`、`previousCanonicalValue`、`lastTransitionId` | `variant=EDGE` 时 `edgeKind` 必填；`variant=CHANGE` 时禁止 `edgeKind` |
| `duration` | `durationMs>=1`、`timerAnchorPolicy={OBSERVED_AT,EVENT_TS}`、`notReadyPolicy={CANCEL,KEEP_ARMED}` | `phase={IDLE,ARMED,FIRED}`、`armedByTriggerId`、`armedAt`、`dueAt`、`logicalTimerId`、`timerFireId`、`recheckPending` | 重复 true 不改 dueAt；false 按策略取消；到期必须重读并复核 |
| `debounce` | `stableForMs>=1`、`timerAnchorPolicy={OBSERVED_AT,EVENT_TS}`、`notReadyPolicy={CANCEL,KEEP_CANDIDATE}` | `committedValue`、`candidateValue`、`candidateSince`、`dueAt`、`logicalTimerId`、`timerFireId`、`recheckPending` | 候选值改变必须替换计时；回到 committed 值必须取消候选 |
| `cooldown` | `durationMs>=1`、`mode=FIXED` | `active`、`startedByTransitionId`、`acceptedAt`、`cooldownUntil`、`logicalTimerId` | 仅 AcceptedStateTransition 后启动；拒绝候选不得启动或续期 |
| `hysteresis` | `low:float64`、`high:float64`、`coldBootstrap={BASELINE_ONLY,EMIT_INITIAL}` | `initialized`、`currentBand={LOW,MIDDLE,HIGH}`、`latchedSide={UNKNOWN,LOW,HIGH}`、`lastTransitionId` | `low < high`；中间区保持 latchedSide，不产生反向切换 |

每个算子的 `semanticHashFields` 必须覆盖全部配置字段；所有 Timer 型算子声明 `timer.required: true` 和各自 `timerKinds`。

- [ ] **Step 6: 固定 Timer、延迟动作与 DRAFT 扩展边界**

`timer` 显式写 `configFields: []`；状态字段固定为：`processingNamespace`、`processingMode`、`identityAlgorithmVersion`、`bindingId`、`evaluationSubjectKey`、`operatorId`、`timerKind`、`armedByTriggerId`、`armedAt`、`dueAt`、`logicalTimerId`、`timerFireId`、`recheckPending`。黄金用例必须覆盖“旧 timerFireId 与当前槽不匹配时 no-op”。

`reliable_delayed_action` 标为 `SPECIFIED`，只声明 `inputs/outputs/phases/transitions/policies`，不生成 Go 类型。阶段固定为 `SCHEDULED/DUE/EXECUTING/SUCCEEDED/FAILED/CANCELLED`，并明确它不是普通 Timer 槽。

`script` 与 `alarm_fsm` 标为 `DRAFT`，省略 `commonFields/configFields/stateFields/quotas/goldenCases`，`gaps` 分别写明：

- Script：沙箱 ABI、状态版本迁移、资源配额和确定性 API 未冻结。
- Alarm/FSM：状态层级、确认/恢复协议、并发事件仲裁和版本迁移未冻结。

- [ ] **Step 7: 写入最小但完整的黄金用例集合**

每个 `CODEGEN_READY` 能力必须覆盖正常、边界/取消、`NOT_READY`/失败、冷启动四组门禁。首版用例 ID 固定为：

```text
scene-binding-state-round-trip
scene-binding-state-reject-over-32k
scene-binding-state-reject-incompatible-schema
scene-binding-state-cold-restore
latest-accept-higher-version
latest-reject-stale-version
latest-missing-is-not-ready
latest-cold-empty
rolling-insert-and-evict-expired
rolling-replace-same-sample-id
rolling-reject-over-1000-samples
rolling-coverage-not-ready
rolling-cold-empty
rolling-cold-load-bounded-history
edge-baseline-without-transition
edge-rising-transition
change-canonical-value-changed
edge-repeat-predicate-has-no-transition
edge-input-not-ready
duration-arm-on-true
duration-repeat-true-keeps-deadline
duration-false-cancels
duration-fire-rechecks-condition
duration-input-not-ready-cancels
duration-cold-no-arm
debounce-replace-candidate
debounce-return-to-committed-cancels
debounce-fire-commits-candidate
debounce-input-not-ready-cancels
debounce-cold-without-candidate
cooldown-starts-after-accepted-transition
cooldown-rejected-candidate-does-not-start
cooldown-fixed-window-does-not-slide
cooldown-input-not-ready-does-not-start
cooldown-cold-inactive
hysteresis-middle-band-keeps-latch
hysteresis-cross-high
hysteresis-cross-low
hysteresis-invalid-low-high
hysteresis-input-not-ready
hysteresis-cold-baseline
timer-arm-creates-stable-identity
timer-stale-fire-is-noop
timer-current-fire-enters-recheck
timer-missing-current-slot-is-noop
timer-cold-restore-reschedules
```

每个用例的 `given` 必须包含完整输入配置、旧状态和事件；`expect` 必须包含新状态、transition 或 error code。错误码固定采用：`STATE_QUOTA_EXCEEDED`、`STALE_INPUT_REJECTED`、`NOT_READY`、`INCOMPATIBLE_STATE_SCHEMA`、`INVALID_CONFIG`。每个能力的四组门禁由 Task 4 语义校验器强制，而不是只靠人工清点。

- [ ] **Step 8: 让正式契约通过全部校验**

Run from `things`:

```bash
go test ./tools/scene-contract-gen/internal/contract -count=1
git diff --check
```

Expected: PASS，且正式契约无 Schema 或语义错误。

## Task 6: 生成 Go 类型、配置校验器和注册表

**Files:**

- Create: `things/tools/scene-contract-gen/internal/generate/types.go`
- Create: `things/tools/scene-contract-gen/internal/generate/validators.go`
- Create: `things/tools/scene-contract-gen/internal/generate/registry.go`
- Create: `things/tools/scene-contract-gen/internal/generate/generate_test.go`
- Generate: `things/contracts/scene/gen/types.gen.go`
- Generate: `things/contracts/scene/gen/validators.gen.go`
- Generate: `things/contracts/scene/gen/registry.gen.go`

- [ ] **Step 1: 先写生成器黄金测试**

生成器包公开 API 固定为：

```go
type Files map[string][]byte

func All(contract *contract.Contract, canonicalHash string) (Files, error)
```

`generate_test.go` 必须断言：

- 相同模型连续生成两次，文件名和字节完全一致。
- 生成内容不含当前时间、绝对路径或随机值。
- `DRAFT` 与 `SPECIFIED` 能力不生成强类型结构体。
- 9 个 `CODEGEN_READY` 条目进入生产 Go 注册表；其中 `scene_binding_state` 是容器，另有 2 个状态组件、5 个算子和 1 个运行机制。`SPECIFIED/DRAFT` 只进入 Markdown 目录。
- 所有 `.go` 输出经过 `go/format` 后与原字节一致。
- `types.gen.go` 可以由 `go/parser.ParseFile` 解析。
- 测试把生成的 3 个 Go 文件和一份手写断言写入 `t.TempDir()` 下的临时 module，再执行 `go test`：空 `LatestValueConfig.Validate()` 成功；非法 hysteresis `low >= high` 返回 `ErrInvalidConfig`；rolling 超过 1000 个 sample、SceneBindingState 序列化超过 32 KiB 均返回 `ErrStateQuotaExceeded`。该子进程只操作测试临时目录。

- [ ] **Step 2: 运行测试，确认失败**

Run from `things`:

```bash
go test ./tools/scene-contract-gen/internal/generate -count=1
```

Expected: 编译失败，提示 `All` 未定义。

- [ ] **Step 3: 实现类型生成器**

生成包名固定为 `scenecontract`，所有生成 Go 文件头固定为：

```go
// Code generated by scene-contract-gen; DO NOT EDIT.
// 由 scene-contract-gen 根据场景有状态计算机器契约生成，请勿手工修改。
```

类型映射固定为：

| 契约类型 | Go 类型 |
|---|---|
| `string` | `string` |
| `bool` | `bool` |
| `int32` | `int32` |
| `int64`、`duration_ms`、`timestamp_ms` | `int64` |
| `float64` | `float64` |
| `json_value`、`opaque_json` | `json.RawMessage` |
| `ref` | 被引用能力的 `GoTypeName` |
| `list<T>` | `[]T` |
| `map<string,T>` | `map[string]T` |

可空标量生成指针；可空 slice、map、`json.RawMessage` 不额外套指针。递归 `object` 字段按父类型名加字段 GoName 生成命名结构，禁止匿名结构体，确保后续可引用。

为每个 `CODEGEN_READY` 能力生成状态结构和配置结构：状态容器读取 `commonFields`，其他能力读取 `stateFields`；配置类型名固定为 `<GoTypeName 去掉 State 后缀>Config`。显式空 `configFields` 生成一个空结构及恒成功的 `Validate`，从而保留稳定 API 并证明配置契约不是漏写。枚举生成独立字符串类型和常量。生成字段必须同时带 `json` 与 `yaml` tag。

- [ ] **Step 4: 实现配置与配额校验器生成**

每个生成配置类型实现：

```go
func (c <Type>Config) Validate() error
```

每个状态类型实现：

```go
func (s <Type>) ValidateState() error
```

生成代码必须执行 YAML 中的数值、枚举、必填、互斥、Rule 与 quota 约束。`SceneBindingState.ValidateState` 使用 `encoding/json.Marshal` 执行 `maxSerializedBytes` quota；`RollingState.ValidateState` 同时检查 sample 数量和 `serializedBytes`。错误通过哨兵错误和 `%w` 暴露：

```go
var (
	ErrInvalidConfig = errors.New("场景有状态计算配置无效")
	ErrStateQuotaExceeded = errors.New("场景状态超过配额")
)
```

- [ ] **Step 5: 实现能力注册表生成**

生成稳定类型：

```go
type Maturity string
type Category string

type CapabilityDescriptor struct {
	ID             string
	DisplayNameZh  string
	DisplayNameEn  string
	CatalogSummary string
	Category       Category
	Maturity       Maturity
	DocsAnchor     string
	GoTypeName     string
	CanonicalHash string
}

var CapabilityRegistry map[string]CapabilityDescriptor
```

生成器必须把上述变量初始化为包含 9 个 `CODEGEN_READY` 描述符的 map literal；源码中不得出现运行时 YAML 解析或 `init` 动态注册。

同一文件生成：

```go
const ContractSpecVersion = "1.0.0"
const CanonicalContractSHA256 = "sha256:<64 位小写十六进制>"
```

提供：

```go
func LookupCapability(id string) (CapabilityDescriptor, bool)
func CodegenReadyCapabilities() []CapabilityDescriptor
```

后者按 `Category + ID` 返回新切片，调用方无法修改内部注册表。

- [ ] **Step 6: 运行生成器测试**

Run from `things`:

```bash
gofmt -w tools/scene-contract-gen/internal/generate
go test ./tools/scene-contract-gen/internal/generate -count=1
go test -race ./tools/scene-contract-gen/internal/generate -count=1
git diff --check
```

Expected: 普通测试和 race 测试均 PASS。

## Task 7: 生成黄金用例、Markdown 目录并实现原子输出

**Files:**

- Create: `things/tools/scene-contract-gen/internal/generate/fixtures.go`
- Create: `things/tools/scene-contract-gen/internal/generate/markdown.go`
- Modify: `things/tools/scene-contract-gen/internal/generate/generate_test.go`
- Create: `things/tools/scene-contract-gen/internal/output/atomic.go`
- Create: `things/tools/scene-contract-gen/internal/output/atomic_test.go`
- Generate: `things/contracts/scene/gen/catalog.gen.md`
- Generate: `things/contracts/scene/gen/testdata/golden-cases.gen.json`

- [ ] **Step 1: 先写 fixture 和 Markdown 的失败测试**

新增断言：

- JSON 根对象包含 `specVersion`、`canonicalContractSha256`、`cases`。
- `cases` 按 `capabilityId + caseId` 排序，正好包含 Task 5 列出的用例。
- Markdown 第一段严格为：

```markdown
specVersion: 1.0.0
canonicalContractSha256: sha256:<64 位小写十六进制>
```

- Markdown 表格列严格固定为已批准设计中的 `分类 | 稳定 ID | 中文名称 | 成熟度`；Go 类型、规范锚点和缺口仍保留在 YAML 与 Go 注册表，不扩展嵌入中文规范的目录列。
- DRAFT 的缺口不得为空；IMPLEMENTED 数量为 0。

- [ ] **Step 2: 实现 fixtures 和 Markdown 生成**

`fixtures.go` 只从 `goldenCases` 读取，不在生成器源码中复制业务样例。JSON 使用两个空格缩进并以换行结束。

`markdown.go` 使用固定类别顺序：状态容器、状态组件、有状态算子、扩展能力、运行机制、持久效果；类别内按 ID 排序。Markdown 不写生成时间。

- [ ] **Step 3: 先写原子输出测试**

固定 API：

```go
func WriteTreeAtomic(root string, files map[string][]byte) error
func WriteFileAtomic(path string, data []byte) error
func Check(root string, files map[string][]byte) error
```

测试覆盖：

1. `WriteTreeAtomic` 创建全部文件。
2. 第二次同内容写入不改变文件字节。
3. 注入一个不可格式化的 Go 文件时，目标目录保持原样。
4. 目录交换任一步失败时恢复旧目录，不留下新旧文件混合状态。
5. `WriteFileAtomic` 只替换目标文件，不改动同目录其他文件。
6. `Check` 在文件缺失、内容不同、存在多余生成文件时返回排序后的差异清单。
7. 输出路径包含 `..` 或绝对路径时拒绝，防止越出 root。

- [ ] **Step 4: 实现临时目录与原子替换**

`WriteTreeAtomic` 必须：

1. 验证所有相对路径。
2. 在目标 root 的父目录创建同文件系统临时目录。
3. 写入全部文件并对 `.go` 执行 `format.Source`。
4. 重新读取并确认字节。
5. root 已存在时，先 rename 为唯一 backup，再把 staging rename 为 root；第二次 rename 失败时必须把 backup rename 回 root。
6. 新目录就位后删除 backup；不得逐文件替换而留下混合版本。
7. `WriteFileAtomic` 在目标文件同目录写临时文件、fsync、rename，只服务文档 marker 同步。
8. 不执行任何 Git 命令，不修改契约 YAML；只有显式 `sync-doc` 才能调用 `WriteFileAtomic` 修改中文规范。

- [ ] **Step 5: 运行生成与输出层测试**

Run from `things`:

```bash
gofmt -w tools/scene-contract-gen/internal/generate tools/scene-contract-gen/internal/output
go test ./tools/scene-contract-gen/internal/generate ./tools/scene-contract-gen/internal/output -count=1
go test -race ./tools/scene-contract-gen/internal/generate ./tools/scene-contract-gen/internal/output -count=1
git diff --check
```

Expected: 全部 PASS。

## Task 8: 实现文档同步、链接校验和 CLI

**Files:**

- Create: `things/tools/scene-contract-gen/internal/docsync/catalog.go`
- Create: `things/tools/scene-contract-gen/internal/docsync/catalog_test.go`
- Create: `things/tools/scene-contract-gen/internal/docsync/links.go`
- Create: `things/tools/scene-contract-gen/internal/docsync/links_test.go`
- Create: `things/tools/scene-contract-gen/main.go`
- Create: `things/tools/scene-contract-gen/main_test.go`
- Modify: `things/Makefile`

- [ ] **Step 1: 先写目录标记同步测试**

固定 API：

```go
func ReplaceCatalog(document, catalog []byte) ([]byte, error)
func CheckCatalog(document, catalog []byte) error
```

测试必须覆盖：标记区唯一时只替换区内内容；开始或结束标记缺失时报错；重复标记时报错；区外一个字节也不能改变；同步后再次同步字节不变。

- [ ] **Step 2: 实现目录同步**

标记常量固定为：

```go
const beginMarker = "<!-- BEGIN GENERATED: stateful-computation-catalog -->"
const endMarker = "<!-- END GENERATED: stateful-computation-catalog -->"
```

同步内容结构固定为“开始标记 + 换行 + catalog.gen.md 全文 + 结束标记 + 原文后缀”。写回文档时必须调用 `output.WriteFileAtomic`，不得用整目录替换影响文档同目录其他文件。

- [ ] **Step 3: 先写本地 Markdown 链接检查测试**

固定 API：

```go
func ValidateLocalLinks(repositoryRoot, scanRoot string) error
```

测试目录包含中文文件名、扫描目录外但仓库根内的合法目标、行内链接、reference-style 链接、相对路径、URL 编码路径、显式 `<a id>`、英文标题锚点、中文标题锚点、缺失文件、缺失锚点和越出仓库根的非法目标。再放入 fenced code、inline code 和 HTML comment 中的伪链接，断言它们不参与校验。HTTP(S)、`mailto:` 不做联网检查；本地图片只检查文件存在。

- [ ] **Step 4: 实现限定范围的链接检查**

实现规则：

- 只递归扫描 `scanRoot` 下 `.md`，但允许链接目标位于 `repositoryRoot` 的任意子目录。
- 解析行内链接、图片链接和 reference-style link definition/usage，跳过 fenced code、inline code 与 HTML comment；拆分路径与 `#anchor`，执行 URL decode。
- 相对路径以来源文件目录解析；经过 clean 和 symlink 解析后，拒绝越出 `repositoryRoot` 的路径。
- 锚点集合来自显式 `<a id="锚点名">` 和 Markdown 标题。标题 slug 规则为：转小写、去除标点、空白转 `-`、连续 `-` 合并；中文字符保留。
- 聚合并排序报告所有坏链，错误包含来源文件、目标和原因。

- [ ] **Step 5: 先写 CLI 行为测试或可调用的 `run` 单元**

`main.go` 内部入口固定为：

```go
func run(args []string, stdout, stderr io.Writer) error
```

子命令固定为：

```text
generate --spec <yaml> --schema <json-schema> --out <dir>
check --spec <yaml> --schema <json-schema> --out <dir>
sync-doc --spec <yaml> --schema <json-schema> --out <dir> --doc <markdown>
check-doc --spec <yaml> --schema <json-schema> --out <dir> --doc <markdown> --doc-root <repo-dir> --doc-scope <scan-dir>
```

参数缺失返回用法错误；`check` 和 `check-doc` 检测漂移时返回非零；所有成功输出都包含 `specVersion` 与 canonical hash。测试还必须覆盖：

- 输出目录不存在时，`generate` 允许首次生成。
- 已有目录的 hash 改变而 `specVersion` 未提升时，`generate` fail-closed 并提示先提升版本。
- 新 `specVersion` 小于已有版本时拒绝生成。
- `specVersion` 提升后允许生成；仅生成器排版变化而 canonical hash 不变时不强制提升业务版本。
- `sync-doc` 必须先通过本地生成物检查，之后才写文档；任一前置检查失败时文档字节不变。
- `check-doc` 只比较生成物、marker、hash 和本地链接，不写任何文件。

- [ ] **Step 6: 实现 CLI 和 Makefile 入口**

`Makefile` 新增目标：

```make
.PHONY: scene-contract-generate scene-contract-check scene-contract-sync-doc

scene-contract-generate:
	go run ./tools/scene-contract-gen generate --spec contracts/scene/stateful-computation-spec.yaml --schema contracts/scene/stateful-computation-spec.schema.json --out contracts/scene/gen

scene-contract-check:
	go run ./tools/scene-contract-gen check --spec contracts/scene/stateful-computation-spec.yaml --schema contracts/scene/stateful-computation-spec.schema.json --out contracts/scene/gen

scene-contract-sync-doc:
	go run ./tools/scene-contract-gen sync-doc --spec contracts/scene/stateful-computation-spec.yaml --schema contracts/scene/stateful-computation-spec.schema.json --out contracts/scene/gen --doc ../iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动/场景有状态计算规范.md
```

CLI 不推断工作区位置；跨仓库路径由调用方显式传入。`generate` 在覆盖已有目录前读取旧 `catalog.gen.md` 的版本与 hash：业务 hash 变化时要求新 SemVer 严格大于旧版本；首次生成不受此限制。该检查防止契约内容变化后仍沿用原 `specVersion`。

- [ ] **Step 7: 运行工具全量测试**

Run from `things`:

```bash
gofmt -w tools/scene-contract-gen
go test ./tools/scene-contract-gen/... -count=1
go test -race ./tools/scene-contract-gen/... -count=1
go vet ./tools/scene-contract-gen/...
git diff --check
```

Expected: 全部 PASS，`go vet` 和 `git diff --check` 无输出。

## Task 9: 生成正式产物并同步中文规范

**Files:**

- Generate: `things/contracts/scene/gen/types.gen.go`
- Generate: `things/contracts/scene/gen/validators.gen.go`
- Generate: `things/contracts/scene/gen/registry.gen.go`
- Generate: `things/contracts/scene/gen/catalog.gen.md`
- Generate: `things/contracts/scene/gen/testdata/golden-cases.gen.json`
- Modify: `iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动/场景有状态计算规范.md`

- [ ] **Step 1: 生成全部产物**

Run from `things`:

```bash
make scene-contract-generate
```

Expected: 输出 `specVersion: 1.0.0` 和一个 `sha256:` 哈希；5 个生成文件出现。

- [ ] **Step 2: 编译生成的 Go 包**

Run from `things`:

```bash
go test ./contracts/scene/gen -count=1
go vet ./contracts/scene/gen
```

Expected: PASS；`go vet` 无输出。

- [ ] **Step 3: 同步中文规范的生成目录**

Run from `things`:

```bash
make scene-contract-sync-doc
```

Expected: 第 14 章的临时提示被正式目录替换；开始和结束标记各保留一次。

- [ ] **Step 4: 验证 YAML、注册表、Markdown 和中文规范哈希一致**

Run from `things`:

```bash
make scene-contract-check
go run ./tools/scene-contract-gen check-doc --spec contracts/scene/stateful-computation-spec.yaml --schema contracts/scene/stateful-computation-spec.schema.json --out contracts/scene/gen --doc ../iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动/场景有状态计算规范.md --doc-root ../iot-doc --doc-scope ../iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动
```

Expected: 两条命令均成功，并报告相同的 canonical hash。

- [ ] **Step 5: 确认第二次生成没有漂移**

Run from `things`:

```bash
make scene-contract-generate
make scene-contract-check
git diff --check
```

Expected: 第二次生成不产生新的内容差异，检查 PASS。

## Task 10: 做跨仓库验收与人工语义复核

**Files:**

- Verify all files listed above.

- [ ] **Step 1: 运行 `things` 全量相关验证**

Run from `things`:

```bash
go test ./tools/scene-contract-gen/... ./contracts/scene/gen -count=1
go test -race ./tools/scene-contract-gen/... -count=1
go vet ./tools/scene-contract-gen/... ./contracts/scene/gen
make scene-contract-check
git diff --check
```

Expected: 全部 PASS，无格式或生成漂移。

- [ ] **Step 2: 运行文档链接与生成区验证**

Run from `things`:

```bash
go run ./tools/scene-contract-gen check-doc --spec contracts/scene/stateful-computation-spec.yaml --schema contracts/scene/stateful-computation-spec.schema.json --out contracts/scene/gen --doc ../iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动/场景有状态计算规范.md --doc-root ../iot-doc --doc-scope ../iot-doc/亿级点位高吞吐架构/6.规则引擎与场景联动
```

Run from `iot-doc`:

```bash
rg -n "机器契约尚未建立" "亿级点位高吞吐架构/6.规则引擎与场景联动/场景有状态计算规范.md"
rg -n "canonicalContractSha256: sha256:[0-9a-f]{64}" "亿级点位高吞吐架构/6.规则引擎与场景联动/场景有状态计算规范.md"
rg -n "IMPLEMENTED" "亿级点位高吞吐架构/6.规则引擎与场景联动/场景有状态计算规范.md"
git diff --check
```

Expected:

- 第一条无输出。
- 第二条恰好命中 1 次。
- 第三条只命中“当前没有 IMPLEMENTED 能力”的成熟度说明，不得把任何能力标成已实现。
- 文档链接检查与 `git diff --check` 均成功。

- [ ] **Step 3: 人工复核四条关键边界**

逐条对照中文规范、YAML 与生成目录，确认：

1. `SceneBindingState` 只叫“状态容器”，没有被列为有状态算子。
2. `latest/rolling` 只叫“状态组件”；rolling 的 1000 样本和 32 KiB 上限三处一致。
3. 5 类有状态算子的配置字段、冷启动、Timer 策略和黄金用例一致。
4. Reliable Delayed Action 没有被建模成普通 Timer；Script、Alarm/FSM 没有生成强类型实现。

- [ ] **Step 4: 展示最终变更范围，不提交**

Run:

```bash
git -C /home/wangjunbo/workspace-iot-platform/iot-doc status --short
git -C /home/wangjunbo/workspace-iot-platform/things status --short
git -C /home/wangjunbo/workspace-iot-platform/iot-doc diff --stat
git -C /home/wangjunbo/workspace-iot-platform/things diff --stat
```

Expected: 只出现本计划“最终文件图”列出的文件，以及已经确认的设计文档和本实施计划。若出现意外文件，停止并向用户说明；不得自动清理用户文件。

- [ ] **Step 5: 向用户交付验证结果并等待提交授权**

交付信息必须包含：

- 新的规范入口和机器契约入口。
- canonical hash。
- 生成和检查命令。
- 实际执行的测试及结果。
- 两个仓库各自的 `git status` 摘要。
- 明确说明尚未执行 `git add`、`git commit`、`git push`。
