# Scene Complete Case Sequence Diagrams Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增一份离线可读的 HTML，将“3 个温度点求平均值 + 门状态 + 两台风机与 Webhook”的完整链路表达为一张总览和五张大字时序图。

**Architecture:** 页面使用单文件 HTML、页面级 CSS 和六个独立内联 SVG。总览只表达阶段关系，五张细图分别承担 Canonical、共享投影、Scene 求值、动作接受、回复归并语义；每张图由独立滚动容器承载，避免把 12 条生命线压缩到同一画布。

**Tech Stack:** HTML5、CSS3、内联 SVG、Bash、`rg`、Python 3 标准库 `html.parser`、浏览器人工检查。

---

## File Structure

- Create: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html` — 自包含页面，包含总览、五张阶段图、图例和阶段语义卡。
- Preserve unchanged: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动端到端时序图.html` — 旧单温度点案例，不得修改。
- Reference only: `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md` — 新图的业务事实来源。

### Task 1: Establish executable acceptance checks

**Files:**
- Create: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html`
- Test: shell checks against the new HTML

- [ ] **Step 1: Record the old HTML blob for the preservation check**

Run:

```bash
old='亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动端到端时序图.html'
git hash-object "$old"
```

Expected: print one Git blob hash; save it in the task notes and compare it again in Task 4.

- [ ] **Step 2: Run the structural check before creating the new file**

Run:

```bash
target='亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
test -f "$target"
```

Expected: FAIL because the new HTML does not exist.

- [ ] **Step 3: Create the minimal valid page shell**

Create the target with this page structure; later tasks populate each empty section with its SVG content:

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>场景联动完整案例 — 端到端时序图</title>
  <style>/* Task 2 adds the complete page and SVG styles. */</style>
</head>
<body>
  <main class="page">
    <header id="top"></header>
    <nav class="stage-nav" aria-label="阶段导航"></nav>
    <section id="overview" data-diagram="overview"></section>
    <section id="phase-1" data-diagram="phase-1"></section>
    <section id="phase-2" data-diagram="phase-2"></section>
    <section id="phase-3" data-diagram="phase-3"></section>
    <section id="phase-4" data-diagram="phase-4"></section>
    <section id="phase-5" data-diagram="phase-5"></section>
  </main>
</body>
</html>
```

- [ ] **Step 4: Verify the shell passes the first executable checks**

Run:

```bash
target='亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
test -f "$target"
test "$(rg -c 'data-diagram=' "$target")" -eq 6
python3 - "$target" <<'PY'
from html.parser import HTMLParser
from pathlib import Path
import sys
class Parser(HTMLParser):
    pass
Parser().feed(Path(sys.argv[1]).read_text(encoding='utf-8'))
print('HTML parse OK')
PY
```

Expected: exit 0 and print `HTML parse OK`.

- [ ] **Step 5: Commit the shell**

```bash
git add '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
git commit -m 'docs: scaffold complete scene sequence page'
```

### Task 2: Build the visual system and overview

**Files:**
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html`

- [ ] **Step 1: Add page-level readability checks that currently fail**

Run:

```bash
target='亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
rg -q -- '--message-size: 15px' "$target"
rg -q -- '--participant-size: 16px' "$target"
rg -q 'class="diagram-scroll"' "$target"
rg -q 'Canonical.*Current Fact.*Scene.*Command' "$target"
```

Expected: FAIL because the visual system and overview have not been added.

- [ ] **Step 2: Implement the shared visual system**

Replace the empty style block with variables and shared classes that include these exact readability contracts:

```css
:root {
  --message-size: 15px;
  --participant-size: 16px;
  --helper-size: 13px;
  --ink: #172033;
  --muted: #526174;
  --line: #94a3b8;
  --ingress: #64748b;
  --stream: #334155;
  --fact: #0f766e;
  --scene: #2563eb;
  --command: #16a34a;
  --reply: #7c3aed;
  --failure: #dc2626;
}
* { box-sizing: border-box; }
body { margin: 0; background: #f8fafc; color: var(--ink); font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", sans-serif; }
.page { max-width: 1500px; margin: 0 auto; padding: 28px 24px 56px; }
.diagram-card { margin: 22px 0; padding: 20px; background: #fff; border: 1px solid #dbe3ec; border-radius: 14px; box-shadow: 0 8px 24px rgba(15, 23, 42, .06); }
.diagram-scroll { overflow-x: auto; padding: 8px 0 14px; }
.diagram-scroll svg { display: block; min-width: 980px; width: 100%; height: auto; }
.participant-title { font-size: var(--participant-size); font-weight: 700; }
.message { font-size: var(--message-size); fill: var(--ink); }
.helper { font-size: var(--helper-size); fill: var(--muted); }
@media (max-width: 760px) { .page { padding: 18px 12px 40px; } .diagram-card { padding: 14px 10px; } }
```

Add a title, scenario statement, color legend, and anchor links for `overview` and `phase-1` through `phase-5`.

- [ ] **Step 3: Implement the overview SVG**

Build a horizontal six-stage overview with these exact nodes and arrows:

```text
设备/接入 → Raw + Canonical → Flink + Current Fact → Scene Admission + Runtime
          → PG Action Outbox + Command Runtime → Reducer + AcceptedStateTransition
```

The overview must label three durable boundaries: `Canonical transaction`、`PG action acceptance`、`Reducer terminal commit` and must not include protocol field lists.

- [ ] **Step 4: Run the readability and overview checks**

Run the Step 1 commands again, followed by:

```bash
target='亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
for text in 'Canonical transaction' 'PG action acceptance' 'Reducer terminal commit'; do
  rg -q "$text" "$target" || exit 1
done
```

Expected: all commands exit 0.

- [ ] **Step 5: Commit the visual system and overview**

```bash
git add '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
git commit -m 'docs: add scene sequence overview'
```

### Task 3: Draw the five readable phase diagrams

**Files:**
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html`

- [ ] **Step 1: Run the semantic coverage test before adding the detail diagrams**

Run:

```bash
target='亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
for text in \
  'temp-01/02/03' 'door-01=CLOSED' 'sendOffsetsToTransaction' \
  'ComputedMetricUpdated' 'CONTEXT_ONLY' 'fixed read-barrier overlay' \
  'dueAt=triggerTime+2m' 'EffectSetReceipt' 'fan-01' 'fan-02' 'Webhook' \
  'duplicate / stale / conflict / unmatched' 'AcceptedStateTransition'; do
  rg -q "$text" "$target" || exit 1
done
```

Expected: FAIL on the first missing detail.

- [ ] **Step 2: Draw Phase 1 — reports become Durable Canonical**

Use at most five lifelines: `temp/door devices`、`Access Adapter`、`Raw Topic`、`ProtocolApplication`、`Canonical Topics`. Show parallel temperature and door reports, Raw production with `acks=all`, the redelivery loop, Kafka transaction production, and `sendOffsetsToTransaction(nextOffset)`. Add a two-column meaning card stating that Canonical durability does not imply Current Fact or Scene completion.

- [ ] **Step 3: Draw Phase 2 — independent consumers and facts**

Split the visual into two aligned sub-lanes so no SVG has more than seven lifelines. Cover Materializer writing `door-01=CLOSED`, Flink reading `temp-01/02/03` and producing `ComputedMetricUpdated`, the Metric CurrentFact Projector CAS, and Admission producing PROPERTY `CONTEXT_ONLY` and COMPUTED_METRIC trigger deltas. State explicitly that consumer groups progress independently.

- [ ] **Step 4: Draw Phase 3 — matching, join, duration and recheck**

Use five lifelines: `scene_event`、`Scene Runtime`、`Current Fact`、`scene_state`、`Timer callback`. Show matching, `fixed read-barrier overlay`, AVG and door reads, `dueAt=triggerTime+2m`, asynchronous state publication, timer callback, Current Fact re-read, and the false/NOT_READY versus true decision. State that timer expiry does not prove the predicate.

- [ ] **Step 5: Draw Phase 4 — action acceptance and command delivery**

Use seven lifelines: `Scene Runtime`、`PostgreSQL`、`Command Runtime`、`ProtocolApplication`、`Access Adapter`、`fan-01/02`、`Webhook dispatcher`. Show one transaction containing EffectSetReceipt, both DeviceCommand rows and Webhook; show rollback/retry versus commit; after commit, show effectId-idempotent command creation, encoding, MQTT QoS 1 delivery, and independent Webhook dispatch.

- [ ] **Step 6: Draw Phase 5 — reply reduction and accepted transition**

Use at most seven lifelines per SVG lane: `fan-01/02`、`Access Adapter/Raw`、`ProtocolApplication/Canonical`、`Command reducer`、`PostgreSQL`、`Derived stream`、`Scene Admission/scene_event`. Show reply observation production, terminal reducer transaction, the `duplicate / stale / conflict / unmatched` no-transition branch, legal monotonic transition with Outbox, asynchronous `AcceptedStateTransition`, and interested Binding fanout.

- [ ] **Step 7: Run semantic and participant-count checks**

Run the Step 1 semantic loop again, followed by:

```bash
target='亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
test "$(rg -c '<section id="phase-[1-5]"' "$target")" -eq 5
test "$(rg -c '本阶段成功意味着' "$target")" -eq 5
test "$(rg -c '本阶段成功不意味着' "$target")" -eq 5
```

Expected: exit 0.

- [ ] **Step 8: Commit all detail diagrams**

```bash
git add '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
git commit -m 'docs: draw complete scene sequence phases'
```

### Task 4: Validate structure, readability and preservation

**Files:**
- Test: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html`
- Verify unchanged: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动端到端时序图.html`

- [ ] **Step 1: Parse the final HTML and reject duplicate ids**

Run:

```bash
target='亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
python3 - "$target" <<'PY'
from collections import Counter
from html.parser import HTMLParser
from pathlib import Path
import sys
class IdParser(HTMLParser):
    def __init__(self):
        super().__init__(); self.ids = []
    def handle_starttag(self, tag, attrs):
        self.ids.extend(v for k, v in attrs if k == 'id')
p = IdParser(); p.feed(Path(sys.argv[1]).read_text(encoding='utf-8'))
duplicates = [key for key, count in Counter(p.ids).items() if count > 1]
assert not duplicates, f'duplicate ids: {duplicates}'
print(f'HTML parse OK; {len(p.ids)} unique ids')
PY
```

Expected: exit 0 and print `HTML parse OK` with the unique id count.

- [ ] **Step 2: Verify diagram count, font contracts and offline behavior**

Run:

```bash
target='亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
test "$(rg -c 'data-diagram=' "$target")" -eq 6
rg -q -- '--message-size: 15px' "$target"
rg -q -- '--participant-size: 16px' "$target"
! rg -n 'https?://|<script[^>]+src=|<link[^>]+href=' "$target"
git diff --check -- "$target"
```

Expected: exit 0 with no output from `git diff --check`.

- [ ] **Step 3: Verify the old HTML is untouched**

Run:

```bash
old='亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动端到端时序图.html'
git diff --exit-code HEAD -- "$old"
git hash-object "$old"
```

Expected: `git diff` exits 0 and the blob hash matches Task 1 Step 1.

- [ ] **Step 4: Perform browser checks**

Open the new HTML directly in a desktop browser at 100% zoom and verify:

```text
1. Overview is readable without zooming.
2. Each phase title and every primary message are legible.
3. At widths below 760px, each diagram scrolls horizontally without shrinking its text.
4. All phase navigation links land on the correct section.
5. Arrowheads, branch boxes and text do not overlap.
```

- [ ] **Step 5: Commit any verification-only corrections**

If browser inspection required corrections, commit only the new HTML:

```bash
git add '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html'
git commit -m 'docs: polish readable scene sequence diagrams'
```

If no correction was needed, do not create an empty commit.
