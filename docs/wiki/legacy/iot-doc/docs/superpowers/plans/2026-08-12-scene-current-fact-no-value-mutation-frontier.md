# Scene Current Fact No-Value-Mutation Frontier Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 明确 Current Fact writer 即使不修改 Scene-live value slot，也能在 input completion 成立后推进 `readyThroughNextOffset`，并用 p3/B/C 例子消除“必须等 B 再上报”的误解。

**Architecture:** 规范文档拥有统一完成谓词，术语文档定义“无值变更完成”，解释文档负责数字例子，15 分钟导读只保留一句结论和下钻入口。普通确定性 no-op 不逐条落 outcome；只有既有规范点名的旁路、冲突、quarantine 等分支要求 reason-bearing terminal outcome。

**Tech Stack:** Markdown、POSIX shell、`rg`、Git；不修改运行时代码、wire/schema 或现有 HTML。

---

## File map

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene current fact设计.md`
  - 规范性拥有 writer input completion、无值变更完成、暂时失败和 cutover 的 frontier 规则。
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`
  - 定义“无值变更完成”，明确它不是新状态、wire 或逐输入持久化表。
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/水线与读取屏障详解.md`
  - 修正过强的 terminal-no-write 二选一表述，加入 p3/B/C 数字过程和 FAQ。
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md`
  - 增加一句“水线按 input completion 推进，不按 B 行 offset 推进”的摘要。
- Do not modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动端到端时序图.html`
- Do not modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html`

实施 worktree：`/home/wangjunbo/workspace-iot-platform/iot-doc/.worktrees/scene-waterline-docs`。父 `master` 仍有用户自己的四处未暂存修改，任何任务都不得写父工作区。

### Task 1: Make the writer completion predicate normative

**Files:**
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene current fact设计.md:64-90`
- Reference: `docs/superpowers/specs/2026-08-12-scene-current-fact-no-value-mutation-frontier-design.md`

- [ ] **Step 1: Verify the normative gap exists**

Run:

```bash
cd /home/wangjunbo/workspace-iot-platform/iot-doc/.worktrees/scene-waterline-docs
doc='亿级点位高吞吐架构/6.规则引擎与场景联动/Scene current fact设计.md'
! rg -n '无值变更完成（no-value-mutation completion）' "$doc"
! rg -n '普通确定性 no-op.*不.*逐条' "$doc"
```

Expected: both searches print nothing and the command exits 0, proving the completion branch is not yet normative.

- [ ] **Step 2: Replace §3 with the explicit completion contract**

Use `apply_patch` to preserve the writer table and replace the bullets after it with the following complete section body:

````markdown
### 3.1 正常写入与统一完成谓词

Materializer 一次解码写 history 与 Scene-live 槽（通用 latest 为可选目标，见设计 §8.5）。
对 source route 中 offset 为 `o` 的输入，只有同时满足以下条件，才能计入该 partition 的连续完成集合：

```text
inputComplete(o) =
    适用性判断已经终结
    AND 所有适用写入目标已经 durable
    AND 规范要求的 terminal evidence 已经 durable
    AND 不存在未解决的暂时性失败
```

- 需要修改 value slot 时，Current Fact 行属于适用目标，必须先 durable；
- 不需要修改 value slot 时，该目标不产生行写入，但不会因此阻止 input completion；
- 只有所有更早 offset 也已完成，writer 才能推进连续 `readyThroughNextOffset`；
- `readyThroughNextOffset` 与 Kafka committed offset 不是同一个字段，但二者都不得越过未完成输入；
- Admission 与 Runtime 都不是写入者；即使 Admission 为静态 eligible 的 CONTEXT_ONLY 更新发布
  SOURCE_DELTA，Runtime 也只能做零 fanout，不能借机写 Current Fact；
- 写入范围 = 静态 eligible ∩ LATEST_STATE ∩ production+LIVE（收缩策略见 §8）。

### 3.2 无值变更完成

**无值变更完成（no-value-mutation completion）**是 input completion 的逻辑分支，不是新的 wire
枚举、lookup 状态、Materializer 状态或数据库表。writer 已完成适用性判断、其他启用目标已经
durable、且不存在未决错误时，即使不创建、推进或删除 Scene-live value slot，该 input 也可以
完成并参与连续 frontier 推进。

- 普通确定性 no-op 不要求逐输入持久化独立 outcome；连续 frontier 是其 durable 汇总证明；
- frontier durable 前崩溃时，Kafka checkpoint 不得越过该输入，恢复后重放并重新执行确定性判断；
  frontier durable 后，它汇总证明此前连续 input 已完成；
- `BACKFILL/REPLAY/shadow` 不得创建、推进或删除 value slot；规范要求的 deterministic bypass
  terminal outcome 完成后必须推进 frontier；
- `OCCURRENCE` 永不写 Current Fact；其适用处理完成后同样计为无值变更完成；
- 技术 tombstone 只用于 subject 删除、tenure 结束或明确投影 GC，不得表示普通 no-op、旁路或业务
  `UNAVAILABLE`。

“某条事实没有被当前规则引用”不等于“不写 Current Fact”。是否写入由 writer 的全局静态 eligible
与 temporal contract 决定；被其他 Binding 纳入 Scene-live 投影的事实仍应更新自己的 value slot。

### 3.3 尚未完成与 retry

暂时性 store/catalog/contract 失败没有可信终态，当前 input 必须 retry，frontier 不得越过。本文
不新增任何 writer retry 状态；下游 Runtime 仍通过既有
`PROJECTION_NOT_READY/BEHIND` 或 `LOOKUP_FAILED` 观察投影落后或查询失败。

### 3.4 cutover 与迟到旧数据

owner/model/metric-plan cutover 经受信控制面发布流进入唯一 writer：semantic ID 缺失、
eligible=false、semanticContractHash 不同或 owner incarnation 改变时，writer 必须在 cutover
生效前写 visibility fence，防止旧槽位冒充新事实；只有该变更同时表示 subject 删除、ownership
tenure 结束或明确 projection GC 时，才允许技术 tombstone。晚到且不再适用的旧-release backlog
不得重新打开或推进新 value slot；它以 `SOURCE_ARTIFACT_NO_LONGER_APPLICABLE` terminal no-write
完成后，才可计入连续 frontier。

`projectionGeneration` 标识一次受控构建的 coverage lineage；initial build、source topology/route
变化或不连续重建时换代；row/frontier/seed 同属一个 generation，读者不得拼接两个 generation；
同一次受控构建可跨进程恢复，不是进程启动随机 UUID。
````

Do not change §4 or later sections in this task.

- [ ] **Step 3: Verify the normative postconditions**

Run:

```bash
doc='亿级点位高吞吐架构/6.规则引擎与场景联动/Scene current fact设计.md'
rg -n '无值变更完成（no-value-mutation completion）' "$doc"
rg -n '普通确定性 no-op 不要求逐输入持久化独立 outcome' "$doc"
rg -n 'OCCURRENCE.*无值变更完成' "$doc"
rg -n '暂时性 store/catalog/contract 失败.*frontier 不得越过' "$doc"
rg -n 'SOURCE_ARTIFACT_NO_LONGER_APPLICABLE.*terminal no-write' "$doc"
rg -n '^## 4\. 排序与冲突$' "$doc"
git diff --check -- "$doc"
```

Expected: all positive searches print one normative definition, no pseudo-enum is introduced, §4 remains `## 4. 排序与冲突`, and `git diff --check` prints nothing.

- [ ] **Step 4: Commit the normative contract**

```bash
git add -- '亿级点位高吞吐架构/6.规则引擎与场景联动/Scene current fact设计.md'
git diff --cached --check
git commit -m 'docs: define no-value-mutation completion'
```

Expected: one-file commit succeeds.

### Task 2: Define the term without inventing a state

**Files:**
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md:839-843`
- Reference: `亿级点位高吞吐架构/6.规则引擎与场景联动/Scene current fact设计.md:64-115`

- [ ] **Step 1: Verify the glossary gap exists**

```bash
terms='亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md'
! rg -n '^\*\*无值变更完成' "$terms"
```

Expected: no output and exit 0 through `!`.

- [ ] **Step 2: Add the glossary definition after `completion frontier / readyThroughNextOffset`**

Use `apply_patch` to insert this complete paragraph after the existing completion-frontier definition:

````markdown
**无值变更完成（no-value-mutation completion）。** writer 对某个 input 已完成适用性判断，且
所有其他启用目标已经 durable，但按全局静态 eligible、temporal contract 或既有旁路契约不需要
修改 Scene-live value slot。它是 input completion 的逻辑分支，不是新的 wire/lookup/Materializer
状态或逐输入持久化表；普通确定性 no-op 由连续 frontier 汇总证明。只有规范点名的 bypass、
conflict、quarantine、迟到旧 artifact 等分支，才必须先完成 reason-bearing terminal outcome。
尚未解决的 store/catalog/contract 失败不属于该终态，必须 retry，frontier 不得越过。
````

Retain the existing `completion frontier` paragraph and exact terminal reason names elsewhere.

- [ ] **Step 3: Verify terminology consistency**

```bash
terms='亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md'
rg -n '^\*\*无值变更完成（no-value-mutation completion）。\*\*' "$terms"
rg -n '不是新的 wire/lookup/Materializer 状态或逐输入持久化表' "$terms"
rg -n '普通确定性 no-op 由连续 frontier 汇总证明' "$terms"
! rg -n 'RETRYABLE_FAILURE' "$terms"
git diff --check -- "$terms"
```

Expected: three positive lines, no pseudo-enum, clean diff.

- [ ] **Step 4: Commit the glossary definition**

```bash
git add -- '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md'
git diff --cached --check
git commit -m 'docs: define no-value-mutation completion term'
```

Expected: one-file commit succeeds.

### Task 3: Explain p3/B/C and repair the guide summary

**Files:**
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/水线与读取屏障详解.md:128-150,253-265`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md:80-98`

- [ ] **Step 1: Verify the reader-facing gap and overstatement**

```bash
doc='亿级点位高吞吐架构/6.规则引擎与场景联动/水线与读取屏障详解.md'
guide='亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'
rg -n '当前事实行或明确的 terminal no-write 结果已经 durable' "$doc"
! rg -n '不需要等待 B 再次上报' "$doc" "$guide"
```

Expected: the first command prints the overstrong sentence; the second prints nothing and exits 0 through `!`.

- [ ] **Step 2: Replace explainer §4.2 and add the p3/B/C example**

Use `apply_patch` to replace the opening of §4.2 through the sentence immediately before `### 4.3 数字算例` with:

```markdown
### 4.2 实际水线：Current Fact writer 的连续完成进度

每种 Current Fact 来源有唯一 writer。writer 对每条 input 先判断适用目标，再按以下顺序推进：

1. 完成该 input 的适用性判断；
2. 将 Current Fact 行（若需要）以及 history、通用 latest 等其他启用目标 durable；
3. 对规范明确要求留证的旁路、冲突或 quarantine，将 terminal outcome durable；
4. 将该 input 计为完成，并确认 partition 中此前不存在 offset 空洞；
5. 再发布连续 `readyThroughNextOffset`。

不需要修改 Scene-live value slot 的 input 也能完成：普通确定性 no-op 不额外插入 outcome 行；
连续 frontier 是其 durable 汇总证明。暂时性 store/catalog/contract 失败仍未完成，必须 retry，
frontier 不得越过。

因此 `readyThroughNextOffset` 是 partition 的实际连续投影进度，不是某个 B 事实行的 source offset，
也不是 Runtime 从 Kafka 猜出的值或 TDengine 的最大时间戳。

#### p3 中 B 与 C 的例子

```text
p3/offset 100：B 湿度 60%，写 B Current Fact 行
                -> p3.readyThroughNextOffset = 101

p3/offset 101：C 不需要修改 Scene-live value slot
                -> 完成适用性判断与其他启用目标
                -> p3.readyThroughNextOffset = 102
```

A 的求值若捕获 `required(p3)=102`，只等待 writer 完成 p3/101，不需要等待 B 再次上报。B 的
上报间隔由 barrier 通过后的 freshness 检查处理。

这里“C 不需要写 Current Fact”必须来自 writer 的全局静态 eligible 或 temporal contract；仅仅
当前 A/B 规则没有引用 C 并不充分。C 若被其他 Binding 静态纳入 Scene-live 投影，仍会更新自己的
C value slot。
```

Keep the existing §4.3 moving-target example and later sections unchanged.

- [ ] **Step 3: Extend the troubleshooting table**

Insert these two rows into the §8 table before the fail-closed row:

```markdown
| p3 中 C 不写 Current Fact，是否必须等 B 再上报才能过 barrier？ | 不需要；C 的 input completion 使 p3 连续水线推进，B 的数据年龄由后续 freshness 判断 |
| 当前规则没有引用 C，就代表 C 不写 Current Fact 吗？ | 不代表；写入范围由 writer 的全局静态 eligible 与 temporal contract 决定 |
```

- [ ] **Step 4: Add the compact guide sentence**

In guide §6, after the required/actual formula and before the DIRECT_TD caveat, insert:

```markdown
实际水线记录 partition 的连续 input completion，不是 B 事实行自身的 source offset。某条 C 输入
按全局静态 eligible 与 temporal contract 无需修改 Current Fact 时，只要其他适用处理已经完成，
p3 水线仍会越过它；不需要等待 B 再次上报。B 是否过旧由 barrier 之后的 freshness 检查决定。
```

- [ ] **Step 5: Verify reader-facing postconditions**

```bash
doc='亿级点位高吞吐架构/6.规则引擎与场景联动/水线与读取屏障详解.md'
guide='亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'
! rg -n '当前事实行或明确的 terminal no-write 结果已经 durable' "$doc"
rg -n '不需要等待 B 再次上报' "$doc" "$guide"
rg -n '普通确定性 no-op 不额外插入 outcome 行' "$doc"
rg -n '当前 A/B 规则没有引用 C 并不充分' "$doc"
for file in "$doc" "$guide"; do
  rg -n '全局静态 eligible 与 temporal contract' "$file"
done
! rg -n 'RETRYABLE_FAILURE' "$doc" "$guide"
! rg -n '^```mermaid' "$guide"
git diff --check -- "$doc" "$guide"
```

Expected: stale sentence absent, both B/C conclusions present, no pseudo-enum or Mermaid, clean diff.

- [ ] **Step 6: Commit the reader-facing explanation**

```bash
git add -- \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/水线与读取屏障详解.md' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'
git diff --cached --check
git commit -m 'docs: explain no-write frontier progress'
```

Expected: two-file commit succeeds.

### Task 4: Run full semantic and repository verification

**Files:**
- Verify all four modified Markdown files
- Verify unchanged: both sequence-diagram HTML files
- Verify unchanged: parent worktree user modifications

- [ ] **Step 1: Run the semantic contract checks**

```bash
cd /home/wangjunbo/workspace-iot-platform/iot-doc/.worktrees/scene-waterline-docs
dir='亿级点位高吞吐架构/6.规则引擎与场景联动'
norm="$dir/Scene current fact设计.md"
terms="$dir/算子语义与术语.md"
doc="$dir/水线与读取屏障详解.md"
guide="$dir/00-15分钟读懂场景联动.md"

for file in "$norm" "$terms" "$doc"; do
  rg -q '无值变更完成' "$file"
done
for file in "$norm" "$terms"; do
  rg -q '普通确定性 no-op.*连续 frontier' "$file"
done
rg -q 'OCCURRENCE.*完成' "$norm"
for file in "$norm" "$doc"; do
  rg -q '暂时性 store/catalog/contract 失败' "$file"
  rg -q 'frontier 不得越过' "$file"
done
rg -q 'SOURCE_ARTIFACT_NO_LONGER_APPLICABLE.*terminal no-write' "$norm"
for file in "$doc" "$guide"; do
  rg -q '不需要等待 B 再次上报' "$file"
done
rg -q '当前 A/B 规则没有引用 C 并不充分' "$doc"
rg -q 'frontier durable 前崩溃.*Kafka checkpoint 不得越过' "$norm"
! rg -n 'RETRYABLE_FAILURE' "$norm" "$terms" "$doc" "$guide"
! rg -n '当前事实行或明确的 terminal no-write 结果已经 durable' "$doc"
! rg -n '^```mermaid' "$guide"
```

Expected: every positive check succeeds; all forbidden searches print nothing.

- [ ] **Step 2: Validate local links and fragments across the feature range**

Run this read-only validator:

```bash
python3 - <<'PY'
from pathlib import Path
from urllib.parse import unquote
import re, subprocess, sys, unicodedata

root = Path.cwd()
raw = subprocess.check_output([
    'git', 'diff', '--name-only', '-z', '4f75016..HEAD', '--', '*.md'
])
sources = [root / Path(x.decode()) for x in raw.split(b'\0') if x]
inline = re.compile(r'(?<!!)\[[^\]]*\]\(([^)]+)\)')
refdef = re.compile(r'^\s*\[[^\]]+\]:\s*(?:<([^>]+)>|(\S+))', re.M)
fence = re.compile(r'^\s*([\x60~]{3,})')

def visible_markdown(text):
    visible, opener_char, opener_len = [], None, 0
    for line in text.splitlines(keepends=True):
        match = fence.match(line)
        if opener_char is None:
            if match:
                opener_char, opener_len = match.group(1)[0], len(match.group(1))
                continue
            visible.append(line)
        elif (match and match.group(1)[0] == opener_char
              and len(match.group(1)) >= opener_len):
            opener_char, opener_len = None, 0
    return ''.join(visible)

def link_targets(text):
    visible = visible_markdown(text)
    yield from (m.group(1).strip() for m in inline.finditer(visible))
    yield from ((m.group(1) or m.group(2)).strip() for m in refdef.finditer(visible))

def slug(text):
    text = re.sub(r'<[^>]*>', '', text)
    text = text.replace(chr(96), '').replace('*', '').replace('~', '').lower()
    kept = []
    for char in text:
        category = unicodedata.category(char)
        if char in ' -_' or category[0] in ('L', 'N') or category in ('Mn', 'Mc'):
            kept.append(char)
    return ''.join(kept).strip().replace(' ', '-')

def anchors(path):
    text = path.read_text(encoding='utf-8')
    result, counts = set(), {}
    opener_char, opener_len = None, 0
    for line in text.splitlines():
        fence_match = fence.match(line)
        if opener_char is None:
            if fence_match:
                opener_char, opener_len = fence_match.group(1)[0], len(fence_match.group(1))
                continue
        elif (fence_match and fence_match.group(1)[0] == opener_char
              and len(fence_match.group(1)) >= opener_len):
            opener_char, opener_len = None, 0
            continue
        elif opener_char is not None:
            continue
        result.update(re.findall(r'<a\s+[^>]*id=["\']([^"\']+)["\'][^>]*>', line, re.I))
        match = re.match(r'^(#{1,6})\s+(.+?)\s*#*\s*$', line)
        if not match:
            continue
        base = slug(match.group(2))
        number = counts.get(base, 0)
        counts[base] = number + 1
        result.add(base if number == 0 else f'{base}-{number}')
    return result

cache, broken = {}, []
local_count = fragment_count = 0
for source in sources:
    for raw_target in link_targets(source.read_text(encoding='utf-8')):
        if re.match(r'^[a-zA-Z][a-zA-Z0-9+.-]*:', raw_target):
            continue
        local_count += 1
        target = raw_target.strip('<>')
        path_part, separator, fragment = target.partition('#')
        destination = (source if not path_part else source.parent / unquote(path_part)).resolve()
        if not destination.exists():
            broken.append((source, raw_target, 'missing target'))
            continue
        if separator and destination.suffix.lower() in ('.md', '.markdown'):
            fragment_count += 1
            cache.setdefault(destination, anchors(destination))
            if unquote(fragment) not in cache[destination]:
                broken.append((source, raw_target, 'missing fragment'))

for item in broken:
    print('BROKEN', *item, sep=' | ')
print(f'LOCAL_LINKS={local_count} FRAGMENTS={fragment_count} BROKEN={len(broken)}')
sys.exit(1 if broken else 0)
PY
```

Expected: `BROKEN=0` and exit 0.

- [ ] **Step 3: Verify Git scope and protected HTML hashes**

```bash
dir='亿级点位高吞吐架构/6.规则引擎与场景联动'
test "$(sha256sum "$dir/场景联动端到端时序图.html" | cut -d' ' -f1)" = \
  '020622580dc61a859e7332ee65a79ca53b03e6e1e4f3adad6494966d76685bbc'
test "$(sha256sum "$dir/场景联动完整案例端到端时序图.html" | cut -d' ' -f1)" = \
  '9b85e4c7219e9ef84dcc3bae77cf79c085abd0887865b5500e0896564d13c333'
git diff --check 4f75016..HEAD
git diff --check
git diff --cached --check
test -z "$(git diff --name-only 4f75016..HEAD -- '*.html')"
test -z "$(git status --porcelain)"
```

Expected: every command exits 0; feature worktree is clean and no HTML is in the feature range.

- [ ] **Step 4: Verify the parent worktree remains untouched**

Run from `/home/wangjunbo/workspace-iot-platform/iot-doc`:

```bash
cd /home/wangjunbo/workspace-iot-platform/iot-doc
test "$(git status --porcelain | wc -l)" -eq 4
test -z "$(git diff --cached --name-only)"
git -c core.quotepath=false status --short
rg -q '\(场景联动%20Runtime%20主规格总纲.md\)' \
  '亿级点位高吞吐架构/6.规则引擎与场景联动/README.md'
```

Expected: exactly the pre-existing README, complete-case HTML, and two appendix changes remain unstaged; the user’s percent-encoded README link remains present.

- [ ] **Step 5: Review the rendered Markdown**

Open the isolated-worktree versions of `Scene current fact设计.md` §3 and `水线与读取屏障详解.md` §4.2 in VS Code Markdown Preview. Verify:

1. the completion predicate fence renders intact;
2. the p3/B/C example remains readable at normal width;
3. headings do not disrupt the existing document hierarchy;
4. no Mermaid diagram or HTML file is edited during review.

Expected: all four observations pass. Do not commit screenshots or editor state.

### Task 5: Request final review

**Files:**
- Review the complete range from `fbb78f4` through the final implementation HEAD

- [ ] **Step 1: Request spec-compliance review**

Ask a fresh reviewer to verify every decision in
`docs/superpowers/specs/2026-08-12-scene-current-fact-no-value-mutation-frontier-design.md`, especially:

- ordinary no-value-mutation advances without a per-input outcome row;
- specified exceptional branches retain durable terminal outcomes;
- no new Materializer or lookup state exists;
- p3/C completion does not wait for B;
- C’s write scope is global, not per current rule;
- both HTML files and parent changes remain untouched.

Expected: `PASS` or concrete file/line findings.

- [ ] **Step 2: Request documentation-quality review**

After spec review passes, ask a separate reviewer to inspect technical accuracy, reader flow, duplication, and misleading terminology across all four modified documents.

Expected: `APPROVED` or concrete file/line findings. Address findings with focused commits and rerun Task 4 before completion.
