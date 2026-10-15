# O0a Active Ownership and Context Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace overlapping `scope.include` routing with one active owner plus overlapping watches, make `graphctl context` discover ownership from paths or the current Git diff, and narrow L0 so unrelated graphctl implementation changes are no longer a direct L0 match.

**Architecture:** Registry scope becomes `owns + watches - exclude`. The Registry package owns strict contract validation and reusable path predicates; the scope package resolves exactly one active owner per changed path, adds active watchers, and propagates their union through the DAG. `graphctl context` is a read-only projection over that same resolver, so developer guidance and pre-push authority cannot drift.

**Tech Stack:** Go 1.24 language level, strict YAML/JSON Schema, Git plumbing commands, the existing bounded glob analyzers, table-driven Go tests, and repository-native `.graph` manifests.

---

## Relationship to the approved design

This is the first bounded plan under O0 in `2026-08-18-rd-graph-fast-full-control-plane-design.md`.

- O0a implements active `owns/watches`, unique changed-path ownership, active-only routing, path/diff Context, and the real G0/L0 migration.
- O0b will separately implement closed-Loop historical proof projection and closure handoff validation. Until O0b passes, do not claim the umbrella O0 subproject complete.
- F0/F1/F2 remain separate plans. This plan does not add FAST/FULL execution classes, queues, caches, `check`, or `prepare`.
- Task 18 Gate-proof hardening is a prerequisite. Preserve its current uncommitted files and finish it as an independently reviewed change before executing O0a.

## Execution constraints

- Work only in `/home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/g0-rd-graph-local-control-plane`.
- Never modify `/home/wangjunbo/workspace-iot-platform/things`; verify it stays clean.
- Never run `go work sync`.
- Preserve the user-owned `backend/go.work.sum` modification; never stage or rewrite it.
- Do not mix Task 18 files into an O0a commit. Before implementation, require a clean Task 18 boundary or explicitly record the remaining pre-existing paths.
- Every commit step is a checkpoint, not standing authorization. Run `git add` or `git commit` only after explicit user authorization for that exact change set. Never push without separate authorization.
- Use targeted tests during TDD. Run the current slow all-Loop Gate at most once at the final O0a milestone; do not put it between tasks.
- O0a changes the machine contract atomically: old `scope.include` must be rejected after migration, not silently supported as an alias.

## File and responsibility map

| File | Responsibility in O0a |
|---|---|
| `.graph/schemas/loop-v1.schema.json` | Strict `owns/watches` document shape; reject legacy `include` |
| `.graph/loops/G0-rd-graph-control-plane.yaml` | Own Graph/control assets and shared tools metadata |
| `.graph/loops/L0-things-baseline.yaml` | Own only L0 assets; watch shared tool/control inputs |
| `backend/tools/internal/graphctl/registry/types.go` | In-memory `ScopeSpec` contract |
| `backend/tools/internal/graphctl/registry/validate.go` | Scope coverage, manifest ownership, subject/validator semantics |
| `backend/tools/internal/graphctl/registry/impact.go` | Pure fail-closed validation used by impact routing |
| `backend/tools/internal/graphctl/scope/impact.go` | Unique owner, watchers, direct and downstream impact |
| `backend/tools/internal/graphctl/changes/git.go` | Deterministic tracked-path discovery for `validate` |
| `backend/tools/internal/graphctl/app.go` | Compose validate and Context modes |
| `backend/tools/cmd/graphctl/main.go` | Repeatable `--path`, JSON error codes, human output |
| `backend/tools/internal/graphctl/evidence/discover.go` | Evidence discovery only under owned output paths |
| `backend/tools/internal/graphctl/review/discover.go` | Spec/Review discovery only under owned paths |
| `AGENTS.md`, `backend/tools/README.md` | Thin operator/Agent entry points using Context |

### Task 1: Change the strict Scope contract to `owns/watches`

**Files:**
- Modify: `.graph/schemas/loop-v1.schema.json`
- Modify: `.graph/loops/G0-rd-graph-control-plane.yaml`
- Modify: `.graph/loops/L0-things-baseline.yaml`
- Modify: `backend/tools/internal/graphctl/registry/types.go`
- Modify: `backend/tools/internal/graphctl/registry/registry_test.go`
- Modify: `backend/tools/internal/graphctl/registry/validate.go`
- Modify: `backend/tools/internal/graphctl/registry/impact.go`
- Modify: `backend/tools/internal/graphctl/scope/impact.go`
- Modify: `backend/tools/internal/graphctl/scope/scope_test.go`
- Modify: `backend/tools/internal/graphctl/evidence/digest.go`
- Modify: `backend/tools/internal/graphctl/evidence/discover.go`
- Modify: `backend/tools/internal/graphctl/evidence/evidence_test.go`
- Modify: `backend/tools/internal/graphctl/evidence/export_test.go`
- Modify: `backend/tools/internal/graphctl/review/discover.go`
- Modify: `backend/tools/internal/graphctl/review/review_test.go`
- Modify: `backend/tools/internal/graphctl/runner/runner_test.go`
- Modify: `backend/tools/internal/graphctl/app_test.go`

- [ ] **Step 1: Write the failing strict-contract tests**

Add beside `TestValidateLoopRejectsInvalidBasicFields`:

```go
func TestScopeContractRequiresOwnsAndAllowsWatches(t *testing.T) {
	root := writeRegistryFixture(t, validLoopYAML())
	reg, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	scope := reg.ByID["G0-rd-graph-control-plane"].Spec.Scope
	if !reflect.DeepEqual(scope.Owns, []string{
		".graph/registry.yaml",
		".graph/loops/G0-rd-graph-control-plane.yaml",
		"backend/tools/**",
	}) {
		t.Fatalf("scope.owns = %#v", scope.Owns)
	}
	if !reflect.DeepEqual(scope.Watches, []string{"shared/**"}) {
		t.Fatalf("scope.watches = %#v", scope.Watches)
	}
}

func TestScopeContractRejectsLegacyInclude(t *testing.T) {
	legacy := strings.Replace(validLoopYAML(), "    owns:\n", "    include:\n", 1)
	_, err := Load(writeRegistryFixture(t, legacy))
	if err == nil || !strings.Contains(err.Error(), "include") {
		t.Fatalf("Load(legacy include) error = %v", err)
	}
}
```

Change `validLoopYAML()` scope to:

```yaml
  scope:
    owns:
      - .graph/registry.yaml
      - .graph/loops/G0-rd-graph-control-plane.yaml
      - backend/tools/**
    watches:
      - shared/**
```

- [ ] **Step 2: Run the focused test and capture RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/registry -run 'TestScopeContract' -count=1
```

Expected: FAIL because the schema rejects `owns/watches` and `ScopeSpec` has no corresponding fields.

- [ ] **Step 3: Replace the Go contract**

Replace `ScopeSpec` in `registry/types.go` with:

```go
type ScopeSpec struct {
	Owns            []string         `yaml:"owns" json:"owns"`
	Watches         []string         `yaml:"watches,omitempty" json:"watches,omitempty"`
	Exclude         []string         `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	ExternalSources []ExternalSource `yaml:"externalSources,omitempty" json:"externalSources,omitempty"`
}
```

Change `ValidateLoop` to:

```go
if len(loop.Spec.Scope.Owns) == 0 {
	return fmt.Errorf("loop scope.owns is required")
}
```

- [ ] **Step 4: Replace the JSON Schema scope definition**

Use this exact shape; do not retain an `include` property:

```json
"scope": {
  "type": "object",
  "additionalProperties": false,
  "required": ["owns"],
  "properties": {
    "owns": {
      "type": "array",
      "minItems": 1,
      "maxItems": 256,
      "uniqueItems": true,
      "items": { "$ref": "#/$defs/path" }
    },
    "watches": {
      "type": "array",
      "maxItems": 256,
      "uniqueItems": true,
      "items": { "$ref": "#/$defs/path" }
    },
    "exclude": {
      "type": "array",
      "maxItems": 256,
      "uniqueItems": true,
      "items": { "$ref": "#/$defs/path" }
    },
    "externalSources": {
      "type": "array",
      "maxItems": 256,
      "uniqueItems": true,
      "items": { "$ref": "#/$defs/externalSource" }
    }
  }
}
```

- [ ] **Step 5: Mechanically migrate every compile-time reference**

Across the exact files listed in this task, make these replacements before introducing new routing behavior:

```text
ScopeSpec{Include: values}       -> ScopeSpec{Owns: values}
.Spec.Scope.Include             -> .Spec.Scope.Owns
"scope.include"                -> "scope.owns"
YAML key "include:"            -> YAML key "owns:"
```

Rename `scope.include` to `scope.owns` in both real Loop manifests without narrowing their lists yet. In G0, add the two exact bootstrap-owned files `docs/superpowers/specs/2026-08-18-rd-graph-fast-full-control-plane-design.md` and `docs/superpowers/plans/2026-08-18-o0a-graph-active-ownership-context.md`; Task 4 later replaces them with the reviewed `*-graph-*.md` family. Add `watches` only where a test specifically exercises watched inputs. Do not add a compatibility field or fallback accessor.

Run:

```bash
rg -n 'Scope\.Include|scope\.include|include:' .graph/loops backend/tools/internal/graphctl backend/tools/cmd/graphctl
```

Expected: no output. Historical design documents are outside this mechanical scan.

- [ ] **Step 6: Run the complete graphctl suite**

Removing the Go field affects every importing package, so run:

```bash
cd backend/tools
GOWORK=off go test ./cmd/graphctl ./internal/graphctl/... -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit checkpoint**

After explicit user authorization only:

```bash
git add .graph/schemas/loop-v1.schema.json .graph/loops/G0-rd-graph-control-plane.yaml .graph/loops/L0-things-baseline.yaml backend/tools/internal/graphctl
git commit -m "refactor(graph): define owned and watched scope"
```

### Task 2: Centralize owned, watched, and input-path semantics

**Files:**
- Modify: `.graph/loops/G0-rd-graph-control-plane.yaml`
- Modify: `.graph/loops/L0-things-baseline.yaml`
- Modify: `backend/tools/internal/graphctl/registry/validate.go`
- Modify: `backend/tools/internal/graphctl/registry/impact.go`
- Modify: `backend/tools/internal/graphctl/registry/registry_test.go`
- Modify: `backend/tools/internal/graphctl/evidence/discover.go`
- Modify: `backend/tools/internal/graphctl/review/discover.go`
- Modify: `backend/tools/internal/graphctl/evidence/evidence_test.go`
- Modify: `backend/tools/internal/graphctl/evidence/export_test.go`

- [ ] **Step 1: Write failing path-semantics tests**

Add in `registry_test.go`:

```go
func TestScopePathRolesApplyOneSharedExclude(t *testing.T) {
	spec := ScopeSpec{
		Owns:    []string{"owned/**"},
		Watches: []string{"shared/**"},
		Exclude: []string{"**/generated/**"},
	}
	tests := []struct {
		path                 string
		owns, watches, input bool
	}{
		{path: "owned/main.go", owns: true, input: true},
		{path: "shared/model.go", watches: true, input: true},
		{path: "owned/generated/main.go"},
		{path: "shared/generated/model.go"},
		{path: "other/file.go"},
	}
	for _, test := range tests {
		if got := ScopeOwnsPath(spec, test.path); got != test.owns {
			t.Errorf("ScopeOwnsPath(%q) = %t, want %t", test.path, got, test.owns)
		}
		if got := ScopeWatchesPath(spec, test.path); got != test.watches {
			t.Errorf("ScopeWatchesPath(%q) = %t, want %t", test.path, got, test.watches)
		}
		if got := ScopeIncludesInputPath(spec, test.path); got != test.input {
			t.Errorf("ScopeIncludesInputPath(%q) = %t, want %t", test.path, got, test.input)
		}
	}
}
```

Add this watched-input fixture test:

```go
func TestWatchedInputsSatisfySubjectAndValidatorCoverage(t *testing.T) {
	loopYAML := validLoopYAML()
	loopYAML = strings.Replace(loopYAML,
		"        sourcePaths:\n          - backend/tools/**\n",
		"        sourcePaths:\n          - shared/**\n", 1)
	loopYAML = strings.Replace(loopYAML,
		"    subjectPaths:\n      - backend/tools/**\n",
		"    subjectPaths:\n      - shared/**\n", 1)
	if _, err := Load(writeRegistryFixture(t, loopYAML)); err != nil {
		t.Fatalf("watched semantic inputs rejected: %v", err)
	}
}
```

Extend the existing canonical-path validation table with a case whose Evidence/Review path appears only in `watches`; expect rejection containing `not covered by Loop scope.owns`. This reuses that table's real-file setup and changes only its manifest string.

- [ ] **Step 2: Verify RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/registry -run 'TestScopePathRoles|TestWatchedInputs' -count=1
```

Expected: FAIL because the role predicates do not exist and validation still assumes one include list.

- [ ] **Step 3: Add reusable role predicates**

Add to `registry/validate.go`:

```go
func ScopeOwnsPath(scope ScopeSpec, candidate string) bool {
	return scopeMatchesPatterns(scope.Owns, scope.Exclude, candidate)
}

func ScopeWatchesPath(scope ScopeSpec, candidate string) bool {
	return scopeMatchesPatterns(scope.Watches, scope.Exclude, candidate)
}

func ScopeIncludesInputPath(scope ScopeSpec, candidate string) bool {
	return ScopeOwnsPath(scope, candidate) || ScopeWatchesPath(scope, candidate)
}

func scopeMatchesPatterns(includes, excludes []string, candidate string) bool {
	included := false
	for _, pattern := range includes {
		if matchPattern(pattern, candidate) {
			included = true
			break
		}
	}
	if !included {
		return false
	}
	for _, pattern := range excludes {
		if matchPattern(pattern, candidate) {
			return false
		}
	}
	return true
}
```

Replace the old coverage helper with:

```go
func scopeCoversOwnedPattern(scope ScopeSpec, candidate string, analyzer *globAnalyzer) (bool, error) {
	return patternsCoverEffective(scope.Owns, scope.Exclude, candidate, analyzer)
}

func scopeCoversInputPattern(scope ScopeSpec, candidate string, analyzer *globAnalyzer) (bool, error) {
	patterns := append(append([]string(nil), scope.Owns...), scope.Watches...)
	return patternsCoverEffective(patterns, scope.Exclude, candidate, analyzer)
}

func patternsCoverEffective(includes, excludes []string, candidate string, analyzer *globAnalyzer) (bool, error) {
	covered, err := analyzer.languageCoveredBy(candidate, includes)
	if err != nil || !covered {
		return covered, err
	}
	for _, excluded := range excludes {
		intersects, err := analyzer.languagesIntersect(candidate, excluded)
		if err != nil || intersects {
			return false, err
		}
	}
	return true, nil
}
```

- [ ] **Step 4: Apply role rules to Registry validation**

Make these substitutions:

```text
globalImpact coverage              -> control-plane owner's effective owns only
Loop manifest/canonical output     -> effective owns only
evidence.subjectPaths              -> effective owns + watches
Gate validator.sourcePaths         -> effective owns + watches
protected .git/runtime checks      -> owns + watches + exclude
pattern-count budget               -> owns + watches + exclude + existing lists
```

`ValidateImpactContract` must reject a non-active `controlPlane.ownerLoop`, require every Loop's `scope.owns`, validate optional `scope.watches`, and verify global impact with `scopeCoversOwnedPattern`.

Change `validateManifestOwnership` so `.graph/registry.yaml` and every `.graph/loops/*.yaml` manifest are owned only by `controlPlane.ownerLoop`. A component Loop may watch its own manifest, but its active implementation ownership must not overlap the control-plane metadata owner. Keep the existing sorted, fail-closed owner diagnostics.

In the same step, replace G0's exact own path `.graph/loops/G0-rd-graph-control-plane.yaml` with `.graph/loops/**`; remove `.graph/loops/L0-things-baseline.yaml` from L0 `owns` and add it to L0 `watches`. This keeps the real Registry valid at the exact commit where manifest ownership semantics change.

- [ ] **Step 5: Centralize Evidence and Review discovery**

In `evidence/discover.go` and `review/discover.go`, replace local ownership helpers with:

```go
registry.ScopeOwnsPath(loop.Spec.Scope, repoPath)
```

Remove duplicated helpers. The field migration already happened in Task 1; this step only introduces watched-input semantics and shared role predicates.

- [ ] **Step 6: Run affected packages**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/registry ./internal/graphctl/evidence ./internal/graphctl/review -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit checkpoint**

After explicit user authorization only:

```bash
git add .graph/loops/G0-rd-graph-control-plane.yaml .graph/loops/L0-things-baseline.yaml backend/tools/internal/graphctl/registry backend/tools/internal/graphctl/evidence backend/tools/internal/graphctl/review
git commit -m "refactor(graph): enforce scope role semantics"
```

### Task 3: Resolve one active owner and overlapping active watches

**Files:**
- Modify: `backend/tools/internal/graphctl/scope/impact.go`
- Modify: `backend/tools/internal/graphctl/scope/scope_test.go`

- [ ] **Step 1: Replace overlap-acceptance tests with ownership tests**

Add these contracts in `scope_test.go`:

```go
func TestAffectedReturnsOneOwnerAndOverlappingWatches(t *testing.T) {
	owner := testLoop("OWNER", registry.LifecycleActive, []string{"src/**"})
	watchA := testLoop("WATCH-A", registry.LifecycleActive, []string{"watch-a/**"})
	watchA.Spec.Scope.Watches = []string{"src/**"}
	watchB := testLoop("WATCH-B", registry.LifecycleActive, []string{"watch-b/**"})
	watchB.Spec.Scope.Watches = []string{"src/pkg/**"}
	reg, dag := testRegistryDAG(t, "OWNER", []string{"control/**"}, watchB, owner, watchA)

	impact, err := Affected(reg, dag, []string{"src/pkg/main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(impact.OwnerIDs, []string{"OWNER"}) ||
		!reflect.DeepEqual(impact.WatchIDs, []string{"WATCH-A", "WATCH-B"}) ||
		!reflect.DeepEqual(impact.DirectIDs, []string{"OWNER", "WATCH-A", "WATCH-B"}) {
		t.Fatalf("impact roles = %#v", impact)
	}
	wantPath := []PathImpact{{
		Path: "src/pkg/main.go", OwnerLoopID: "OWNER",
		WatchLoopIDs: []string{"WATCH-A", "WATCH-B"},
	}}
	if !reflect.DeepEqual(impact.Paths, wantPath) {
		t.Fatalf("path impact = %#v, want %#v", impact.Paths, wantPath)
	}
}

func TestAffectedRejectsMultipleActiveOwners(t *testing.T) {
	first := testLoop("A", registry.LifecycleActive, []string{"src/**"})
	second := testLoop("B", registry.LifecycleActive, []string{"src/pkg/**"})
	reg, dag := testRegistryDAG(t, "A", []string{"control/**"}, first, second)
	_, err := Affected(reg, dag, []string{"src/pkg/main.go"})
	var ambiguous *ErrAmbiguousOwner
	if !errors.As(err, &ambiguous) || ambiguous.Path != "src/pkg/main.go" ||
		!reflect.DeepEqual(ambiguous.OwnerLoopIDs, []string{"A", "B"}) {
		t.Fatalf("error = %#v", err)
	}
}

func TestAffectedIgnoresPlannedAndClosedRouting(t *testing.T) {
	active := testLoop("ACTIVE", registry.LifecycleActive, []string{"src/**"})
	closed := testLoop("CLOSED", registry.LifecycleClosed, []string{"src/**"})
	planned := testLoop("PLANNED", registry.LifecyclePlanned, []string{"other/**"}, "ACTIVE")
	closed.Spec.Scope.Watches = []string{"src/**"}
	reg, dag := testRegistryDAG(t, "ACTIVE", []string{"control/**"}, planned, closed, active)
	impact, err := Affected(reg, dag, []string{"src/main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(impact.LoopIDs, []string{"ACTIVE"}) {
		t.Fatalf("affected Loops = %v", impact.LoopIDs)
	}
}
```

- [ ] **Step 2: Verify RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/scope -run 'TestAffected(ReturnsOneOwner|RejectsMultiple|IgnoresPlanned)' -count=1
```

Expected: FAIL because overlap is currently accepted as multiple owners and role-specific output does not exist.

- [ ] **Step 3: Add typed output and errors**

Use these public shapes in `scope/impact.go`:

```go
type PathImpact struct {
	Path         string   `json:"path"`
	OwnerLoopID  string   `json:"ownerLoopId"`
	WatchLoopIDs []string `json:"watchLoopIds"`
}

type Impact struct {
	ChangedPaths []string     `json:"changedPaths"`
	Paths        []PathImpact `json:"paths"`
	OwnerIDs     []string     `json:"ownerLoopIds"`
	WatchIDs     []string     `json:"watchLoopIds"`
	DirectIDs    []string     `json:"directLoopIds"`
	LoopIDs      []string     `json:"affectedLoopIds"`
	Global       bool         `json:"globalImpact"`
}

type ErrAmbiguousOwner struct {
	Path         string
	OwnerLoopIDs []string
}

func (err *ErrAmbiguousOwner) Error() string {
	return fmt.Sprintf("scope: ambiguous active owners for changed path %q: %s", err.Path, strings.Join(err.OwnerLoopIDs, ", "))
}
```

Retain `ErrUnownedPath` so callers continue using `errors.As`.

- [ ] **Step 4: Rewrite the per-path resolver**

For every normalized path:

1. Consider only `lifecycle=active` Loops.
2. Match owner with `registry.ScopeOwnsPath` and watchers with `registry.ScopeWatchesPath`.
3. Return `ErrUnownedPath` for zero owners and sorted `ErrAmbiguousOwner` for more than one.
4. Exclude the owner ID from that path's watcher list.
5. Build `DirectIDs` as the sorted union of `OwnerIDs` and `WatchIDs`.
6. On global impact, seed every active Loop, never planned or closed.
7. After `dag.Affected`, retain only active Loops in `LoopIDs`.

Initialize every collection to an empty non-nil slice in `emptyImpact`; deterministic JSON must not emit `null`.

- [ ] **Step 5: Add tracked-overlap validation**

Add:

```go
func ValidateTrackedOwnership(reg *registry.Registry, tracked []string) error
```

It validates/sorts paths and checks only multiple active owners. It intentionally does not reject all unowned tracked repository files, because untouched legacy areas have not all entered the Graph. Changed and untracked paths remain fail-closed through `Affected`.

- [ ] **Step 6: Update all scope fixtures and assertions**

Change `testLoop` to populate `ScopeSpec{Owns: owns}`. Update exact `Impact` comparisons to include `Paths`, `OwnerIDs`, and `WatchIDs`. Change the old global test expectation so closed/planned Loops are absent.

- [ ] **Step 7: Run scope tests**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/scope -count=1
```

Expected: PASS.

- [ ] **Step 8: Hold the atomic migration boundary**

Do not commit Task 3 by itself: unique-owner routing would make the current broad G0/L0 overlap fail before the manifests are narrowed. Continue directly into Task 4, then commit Tasks 3 and 4 together only after the real Registry and tracked-path validator pass as one unit.

### Task 4: Make `validate` detect conflicts in current tracked files

**Files:**
- Modify: `.graph/loops/G0-rd-graph-control-plane.yaml`
- Modify: `.graph/loops/L0-things-baseline.yaml`
- Modify: `backend/tools/internal/graphctl/changes/git.go`
- Modify: `backend/tools/internal/graphctl/changes/changes_test.go`
- Modify: `backend/tools/internal/graphctl/app.go`
- Modify: `backend/tools/internal/graphctl/app_test.go`
- Modify: `backend/tools/internal/graphctl/registry/registry_test.go`

- [ ] **Step 1: Write failing tracked-path tests**

In `changes_test.go`, add:

```go
func TestTrackedPathsReturnsOnlySortedTrackedFiles(t *testing.T) {
	fixture := newGitFixture(t)
	writeTestFile(t, fixture.repo, "untracked.txt", "untracked\n")
	writeTestFile(t, fixture.repo, "cache.ignored", "ignored\n")
	got, err := (Client{RepoRoot: fixture.repo, Run: ExecRunner{}}).TrackedPaths(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := byteSorted(".gitignore", "base.txt", "loop-a/old.txt")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TrackedPaths() = %q, want %q", got, want)
	}
}

func TestTrackedPathsRejectsMalformedNULStream(t *testing.T) {
	client := Client{RepoRoot: "/repo", Run: &staticRunner{output: []byte("unterminated")}}
	if _, err := client.TrackedPaths(context.Background()); err == nil || !strings.Contains(err.Error(), "NUL") {
		t.Fatalf("TrackedPaths() error = %v", err)
	}
}
```

In `app_test.go`, extend `newAppRepo` in one test with a second active Loop that owns `src/shared.go`, commit that file, and assert:

```go
_, err := (App{Git: changes.ExecRunner{}}).Validate(context.Background(), ValidateRequest{RepoRoot: fixture.repo})
var ambiguous *scope.ErrAmbiguousOwner
if !errors.As(err, &ambiguous) || ambiguous.Path != "src/shared.go" ||
	!reflect.DeepEqual(ambiguous.OwnerLoopIDs, []string{"G0", "SECOND"}) {
	t.Fatalf("Validate() error = %#v", err)
}
```

The second manifest must use the same minimal Gate/Review/Evidence shape as `G0`, be stored at `.graph/loops/SECOND.yaml`, and leave `.graph/loops/**` owned by G0. Do not weaken manifest ownership to construct this fixture.

Also add the real-repository assertions shown below before changing the manifests:

```go
if ScopeOwnsPath(l0.Spec.Scope, "backend/tools/internal/graphctl/app.go") {
	t.Fatal("L0 still owns graphctl implementation")
}
if !ScopeOwnsPath(g0.Spec.Scope, "backend/tools/internal/graphctl/app.go") {
	t.Fatal("G0 does not own graphctl implementation")
}
if !ScopeOwnsPath(g0.Spec.Scope, "backend/tools/go.mod") ||
	!ScopeWatchesPath(l0.Spec.Scope, "backend/tools/go.mod") {
	t.Fatal("shared tools module ownership/watch contract is wrong")
}
```

- [ ] **Step 2: Verify RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/changes ./internal/graphctl -run 'Test(TrackedPaths|ValidateRejectsTrackedOwnershipConflict)' -count=1
```

Expected: FAIL because `TrackedPaths` is undefined and `Validate` does not inspect tracked files.

- [ ] **Step 3: Implement deterministic tracked discovery**

Add to `changes/git.go`:

```go
func (client Client) TrackedPaths(ctx context.Context) ([]string, error) {
	if err := client.validate(); err != nil {
		return nil, err
	}
	output, err := client.Run.Run(ctx, client.RepoRoot, "ls-files", "--cached", "-z")
	if err != nil {
		return nil, fmt.Errorf("changes: list tracked paths: %w", err)
	}
	paths, err := parseNULRecords(output)
	if err != nil {
		return nil, fmt.Errorf("changes: parse tracked paths: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}
```

- [ ] **Step 4: Compose ownership validation in `App.Validate`**

Change the method to use its context and configured Git runner:

```go
func (app App) Validate(ctx context.Context, request ValidateRequest) (ValidateResult, error) {
	reg, dag, gitRunner, err := app.load(request.RepoRoot)
	if err != nil {
		return ValidateResult{}, err
	}
	tracked, err := (changes.Client{RepoRoot: reg.RepoRoot, Run: gitRunner}).TrackedPaths(ctx)
	if err != nil {
		return ValidateResult{}, err
	}
	if err := scope.ValidateTrackedOwnership(reg, tracked); err != nil {
		return ValidateResult{}, err
	}
	return ValidateResult{SchemaVersion: 1, LoopIDs: append([]string(nil), dag.Order()...)}, nil
}
```

- [ ] **Step 5: Narrow the real G0/L0 ownership before enabling the validator**

Use the final G0 owned list:

```yaml
scope:
  owns:
    - .graph/registry.yaml
    - .graph/loops/**
    - .graph/schemas/**
    - .githooks/**
    - AGENTS.md
    - .gitignore
    - backend/tools/cmd/graphctl/**
    - backend/tools/internal/graphctl/**
    - backend/tools/go.mod
    - backend/tools/go.sum
    - backend/tools/README.md
    - docs/superpowers/specs/*-graph-*.md
    - docs/superpowers/plans/*-graph-*.md
    - docs/superpowers/evidence/G0-rd-graph-control-plane/**
    - docs/superpowers/reviews/G0-rd-graph-control-plane/**
```

Use the final L0 roles and remove `backend/tools/**/*.go`:

```yaml
scope:
  owns:
    - backend/go.work
    - backend/go.work.sum
    - backend/saas/go.mod
    - backend/saas/go.sum
    - backend/things/**
    - backend/tools/cmd/thingsbaseline/**
    - backend/tools/internal/thingsbaseline/**
    - backend/tools/templates/things-baseline/**
    - backend/tools/things-baseline.lock
    - docs/superpowers/specs/2026-08-14-l0-things-compilation-migration-baseline-design.md
    - docs/superpowers/plans/2026-08-14-l0-things-compilation-migration-baseline.md
    - docs/superpowers/plans/2026-08-15-l0-schema-fixture-gate-recovery.md
    - docs/superpowers/plans/2026-08-15-l0-final-review-fail-closed-recovery.md
    - docs/superpowers/evidence/2026-08-14-l0-things-*.json
    - docs/superpowers/evidence/2026-08-14-l0-things-*.md
    - docs/superpowers/evidence/L0-things-baseline/**
    - docs/superpowers/reviews/L0-things-baseline/**
  watches:
    - .graph/loops/L0-things-baseline.yaml
    - .gitignore
    - backend/tools/README.md
    - backend/tools/go.mod
    - backend/tools/go.sum
```

Set L0 `evidence.subjectPaths` to:

```yaml
subjectPaths:
  - backend/go.work
  - backend/go.work.sum
  - backend/saas/go.mod
  - backend/saas/go.sum
  - backend/things/**
  - backend/tools/cmd/thingsbaseline/**
  - backend/tools/internal/thingsbaseline/**
  - backend/tools/go.mod
  - backend/tools/go.sum
  - backend/tools/templates/things-baseline/**
  - backend/tools/things-baseline.lock
```

- [ ] **Step 6: Run focused and package tests**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/changes ./internal/graphctl -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit checkpoint**

After explicit user authorization only:

```bash
git add .graph/loops/G0-rd-graph-control-plane.yaml .graph/loops/L0-things-baseline.yaml backend/tools/internal/graphctl/scope backend/tools/internal/graphctl/changes backend/tools/internal/graphctl/app.go backend/tools/internal/graphctl/app_test.go backend/tools/internal/graphctl/registry/registry_test.go
git commit -m "feat(graph): validate tracked ownership conflicts"
```

### Task 5: Add path and diff modes to `graphctl context`

**Files:**
- Modify: `backend/tools/internal/graphctl/app.go`
- Modify: `backend/tools/internal/graphctl/app_test.go`
- Modify: `backend/tools/cmd/graphctl/main.go`
- Modify: `backend/tools/cmd/graphctl/main_test.go`

- [ ] **Step 1: Write failing application tests**

Add a path/diff equivalence test:

```go
func TestContextPathAndDiffModesUseTheSameResolver(t *testing.T) {
	fixture := newAppRepo(t)
	app := App{Git: changes.ExecRunner{}}
	pathResult, err := app.Context(context.Background(), ContextRequest{
		RepoRoot: fixture.repo,
		Paths:    []string{"src/subject.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeAppFile(t, fixture.repo, "src/subject.txt", []byte("changed\n"))
	diffResult, err := app.Context(context.Background(), ContextRequest{RepoRoot: fixture.repo})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pathResult.Impact, diffResult.Impact) {
		t.Fatalf("path impact = %#v, diff impact = %#v", pathResult.Impact, diffResult.Impact)
	}
}
```

Also assert `--loop G0` compatibility, a future repo-relative path, loop/path mutual exclusion, no-flag diff mode, and typed unowned/ambiguous failures.

- [ ] **Step 2: Write failing CLI tests**

Change the old “missing context Loop” case into a successful no-flag Context route. Add repeated-path routing:

```go
{
	name: "context paths",
	args: []string{"context", "--repo-root=/repo", "--path", "b.go", "--path=a.go"},
	wantMethod: "context",
	assert: func(t *testing.T, app *recordingApplication) {
		want := graphctl.ContextRequest{RepoRoot: "/repo", Paths: []string{"b.go", "a.go"}}
		if !reflect.DeepEqual(app.contextRequest, want) {
			t.Fatalf("Context request = %#v, want %#v", app.contextRequest, want)
		}
	},
},
```

- [ ] **Step 3: Verify RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl ./cmd/graphctl -run 'TestContext|TestCLI' -count=1
```

Expected: FAIL because Context still requires a Loop ID and `--path` is unknown.

- [ ] **Step 4: Add Context result types**

Use:

```go
type ContextRequest struct {
	RepoRoot string
	LoopID   string
	Paths    []string
}

type ContextLoop struct {
	Loop   *registry.Loop      `json:"loop"`
	Status evidence.LoopStatus `json:"status"`
}

type ContextResult struct {
	SchemaVersion int           `json:"schemaVersion"`
	Mode          string        `json:"mode"`
	Impact        scope.Impact  `json:"impact"`
	Loops         []ContextLoop `json:"loops"`
	NextActions   []string      `json:"nextActions"`
}
```

Valid modes are `loop`, `path`, and `diff`; always emit non-nil `Loops` and `NextActions`.

- [ ] **Step 5: Implement the three application modes**

In `App.Context`:

1. Load Registry/DAG/Git once and reject both LoopID and Paths.
2. Loop mode keeps the registered-Loop check and returns exactly its contract/status.
3. Path mode calls `scope.Affected` with supplied paths.
4. Diff mode calls `ManualRange`, `ChangedPaths`, then the same resolver.
5. Index `Status` by Loop ID and return entries in `impact.LoopIDs` order.
6. For non-empty impact return `nextActions=["continue only within the listed owner scopes", "run graphctl scope-check before the milestone Gate"]`.
7. For empty diff return `nextActions=["locate a candidate file, then run graphctl context --path PATH"]`.

- [ ] **Step 6: Add repeatable CLI `--path` and typed errors**

Add a `stringListFlag` implementing `flag.Value`, register it with `set.Var`, and allow duplicate `--path` only for Context. Reject `--loop` plus paths and pass a defensive copy to `ContextRequest`.

Use this implementation and add `paths stringListFlag` to `commandOptions`:

```go
type stringListFlag []string

func (values *stringListFlag) String() string {
	return strings.Join(*values, ",")
}

func (values *stringListFlag) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("path must not be blank")
	}
	*values = append(*values, value)
	return nil
}
```

Extend `scopeDocument` so `scope-check --format=json` does not discard the new authority data:

```go
type scopeDocument struct {
	SchemaVersion   int                `json:"schemaVersion"`
	ChangedPaths    []string           `json:"changedPaths"`
	Paths           []scope.PathImpact `json:"paths"`
	OwnerLoopIDs    []string           `json:"ownerLoopIds"`
	WatchLoopIDs    []string           `json:"watchLoopIds"`
	DirectLoopIDs   []string           `json:"directLoopIds"`
	AffectedLoopIDs []string           `json:"affectedLoopIds"`
	GlobalImpact    bool               `json:"globalImpact"`
}
```

Populate every field directly from `scope.Impact`; update the human `scope-check` output to show owners and watches separately.

Change duplicate detection to receive the command and skip only repeated Context paths:

```go
func rejectDuplicateFlags(command string, args []string) error {
	seen := make(map[string]struct{})
	for _, argument := range args {
		if argument == "--" {
			break
		}
		if !strings.HasPrefix(argument, "-") || argument == "-" {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimLeft(argument, "-"), "=")
		if name == "" {
			continue
		}
		name = "--" + name
		if command == "context" && name == "--path" {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("duplicate flag %s", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}
```

In `writeOperationalFailure`, map typed errors before the generic control-plane mapping:

```go
var unowned *scope.ErrUnownedPath
var ambiguous *scope.ErrAmbiguousOwner
switch {
case errors.As(err, &unowned):
	detail.Code = "UNOWNED_PATH"
case errors.As(err, &ambiguous):
	detail.Code = "AMBIGUOUS_OWNER"
}
```

Human output must print mode, paths, owners, watches, affected Loops, global impact, per-Loop status, and ordered next actions.

- [ ] **Step 7: Run application and CLI tests**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl ./cmd/graphctl -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit checkpoint**

After explicit user authorization only:

```bash
git add backend/tools/internal/graphctl/app.go backend/tools/internal/graphctl/app_test.go backend/tools/cmd/graphctl/main.go backend/tools/cmd/graphctl/main_test.go
git commit -m "feat(graph): discover context from paths and diff"
```

### Task 6: Lock repository regressions and update operator guidance

**Files:**
- Verify: `.graph/loops/G0-rd-graph-control-plane.yaml`
- Verify: `.graph/loops/L0-things-baseline.yaml`
- Verify: `backend/tools/internal/graphctl/registry/registry_test.go`
- Verify: `backend/tools/internal/graphctl/app_test.go`
- Modify: `AGENTS.md`
- Modify: `backend/tools/README.md`

- [ ] **Step 1: Confirm the repository regression assertions introduced in Task 4**

Require `TestRepositoryRegistryDeclaresG0AndRealL0` to retain these exact assertions:

```go
if ScopeOwnsPath(l0.Spec.Scope, "backend/tools/internal/graphctl/app.go") {
	t.Fatal("L0 still owns graphctl implementation")
}
if !ScopeOwnsPath(g0.Spec.Scope, "backend/tools/internal/graphctl/app.go") {
	t.Fatal("G0 does not own graphctl implementation")
}
if !ScopeOwnsPath(g0.Spec.Scope, "backend/tools/go.mod") ||
	!ScopeWatchesPath(l0.Spec.Scope, "backend/tools/go.mod") {
	t.Fatal("shared tools module ownership/watch contract is wrong")
}
if !ScopeOwnsPath(g0.Spec.Scope, ".graph/loops/L0-things-baseline.yaml") ||
	!ScopeWatchesPath(l0.Spec.Scope, ".graph/loops/L0-things-baseline.yaml") {
	t.Fatal("Loop manifest ownership/watch contract is wrong")
}
```

In `app_test.go`, retain global-impact propagation but separately assert that a graphctl path has owner G0, no L0 watcher, and L0 appears only because `Global=true`.

- [ ] **Step 2: Verify the migrated Registry remains GREEN**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/registry ./internal/graphctl -run 'TestRepositoryRegistry' -count=1
```

Expected: PASS. A failure here means Task 4's atomic ownership migration was incomplete.

- [ ] **Step 3: Inspect the final G0 ownership**

Require G0 `scope.owns` to equal this final list; do not edit it if already exact:

```yaml
scope:
  owns:
    - .graph/registry.yaml
    - .graph/loops/**
    - .graph/schemas/**
    - .githooks/**
    - AGENTS.md
    - .gitignore
    - backend/tools/cmd/graphctl/**
    - backend/tools/internal/graphctl/**
    - backend/tools/go.mod
    - backend/tools/go.sum
    - backend/tools/README.md
    - docs/superpowers/specs/*-graph-*.md
    - docs/superpowers/plans/*-graph-*.md
    - docs/superpowers/evidence/G0-rd-graph-control-plane/**
    - docs/superpowers/reviews/G0-rd-graph-control-plane/**
```

Keep G0 Evidence subject paths limited to implementation/contract inputs; do not add specs, plans, runtime, Evidence, Review, or Loop manifests to `subjectPaths`.

- [ ] **Step 4: Inspect the final L0 ownership and watches**

Require these lists and verify `backend/tools/**/*.go` is absent:

```yaml
scope:
  owns:
    - backend/go.work
    - backend/go.work.sum
    - backend/saas/go.mod
    - backend/saas/go.sum
    - backend/things/**
    - backend/tools/cmd/thingsbaseline/**
    - backend/tools/internal/thingsbaseline/**
    - backend/tools/templates/things-baseline/**
    - backend/tools/things-baseline.lock
    - docs/superpowers/specs/2026-08-14-l0-things-compilation-migration-baseline-design.md
    - docs/superpowers/plans/2026-08-14-l0-things-compilation-migration-baseline.md
    - docs/superpowers/plans/2026-08-15-l0-schema-fixture-gate-recovery.md
    - docs/superpowers/plans/2026-08-15-l0-final-review-fail-closed-recovery.md
    - docs/superpowers/evidence/2026-08-14-l0-things-*.json
    - docs/superpowers/evidence/2026-08-14-l0-things-*.md
    - docs/superpowers/evidence/L0-things-baseline/**
    - docs/superpowers/reviews/L0-things-baseline/**
  watches:
    - .graph/loops/L0-things-baseline.yaml
    - .gitignore
    - backend/tools/README.md
    - backend/tools/go.mod
    - backend/tools/go.sum
```

Require L0 `evidence.subjectPaths` to equal:

```yaml
subjectPaths:
  - backend/go.work
  - backend/go.work.sum
  - backend/saas/go.mod
  - backend/saas/go.sum
  - backend/things/**
  - backend/tools/cmd/thingsbaseline/**
  - backend/tools/internal/thingsbaseline/**
  - backend/tools/go.mod
  - backend/tools/go.sum
  - backend/tools/templates/things-baseline/**
  - backend/tools/things-baseline.lock
```

- [ ] **Step 5: Update thin human/Agent instructions**

In `AGENTS.md`, replace pre-change owner lookup with:

```text
已知候选文件时运行 graphctl context --path PATH；已有修改时运行无参数 graphctl context。
读取机器返回的 owner、watches、affected Loops 和 nextActions，不人工扫描 Loop YAML 推导归属。
UNOWNED_PATH 或 AMBIGUOUS_OWNER 时停止实现，先修正 Registry/架构边界。
```

In `backend/tools/README.md`, document all three Context modes and retain `--loop` as the compatibility inspection mode.

- [ ] **Step 6: Run the new Context against the repository Registry**

Run:

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl validate --repo-root ../..
GOWORK=off go run ./cmd/graphctl context --repo-root ../.. --path backend/tools/internal/graphctl/app.go --format=json
```

Expected: Registry valid; the path has G0 as owner, no L0 watch, and any L0 appearance is explained by global impact.

- [ ] **Step 7: Commit checkpoint**

After explicit user authorization only:

```bash
git add AGENTS.md backend/tools/README.md
git commit -m "docs(graph): route work through machine context"
```

### Task 7: Verify O0a once, without restoring per-task slow gates

**Files:**
- Verify only; no expected source creation
- Preserve: `backend/go.work.sum`
- Preserve read-only: `/home/wangjunbo/workspace-iot-platform/things`

- [ ] **Step 1: Format only touched Go files**

Run `git diff --name-only -- '*.go'`, verify it is a subset of the following list, then run:

```bash
gofmt -w backend/tools/cmd/graphctl/main.go backend/tools/cmd/graphctl/main_test.go backend/tools/internal/graphctl/app.go backend/tools/internal/graphctl/app_test.go backend/tools/internal/graphctl/changes/git.go backend/tools/internal/graphctl/changes/changes_test.go backend/tools/internal/graphctl/registry/types.go backend/tools/internal/graphctl/registry/validate.go backend/tools/internal/graphctl/registry/impact.go backend/tools/internal/graphctl/registry/registry_test.go backend/tools/internal/graphctl/scope/impact.go backend/tools/internal/graphctl/scope/scope_test.go backend/tools/internal/graphctl/evidence/digest.go backend/tools/internal/graphctl/evidence/discover.go backend/tools/internal/graphctl/evidence/evidence_test.go backend/tools/internal/graphctl/evidence/export_test.go backend/tools/internal/graphctl/review/discover.go
```

Do not run a repository-wide formatter.

- [ ] **Step 2: Run the complete graphctl unit suite**

Run:

```bash
cd backend/tools
GOWORK=off go test ./cmd/graphctl ./internal/graphctl/... -count=1
```

Expected: PASS.

- [ ] **Step 3: Run machine Registry and Context checks**

Run:

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl validate --repo-root ../..
GOWORK=off go run ./cmd/graphctl context --repo-root ../.. --path backend/tools/internal/graphctl/app.go --format=json
GOWORK=off go run ./cmd/graphctl context --repo-root ../.. --format=json
GOWORK=off go run ./cmd/graphctl scope-check --repo-root ../.. --format=json
```

Expected:

- Registry valid;
- graphctl path owner is G0;
- L0 is not a direct owner/watcher for graphctl code;
- all current changed/untracked paths have one active owner;
- the pre-existing `backend/go.work.sum` is owned by L0 but remains untouched.

- [ ] **Step 4: Run one milestone FULL under the current control plane**

Run this once, not after every task:

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl ci --repo-root ../..
```

Expected: PASS with a machine run ID. L0 may still take about ten minutes until F0 moves it out of the push path. If it exceeds the existing Gate timeout or fails, retain runtime/logs and diagnose; do not loop blindly.

- [ ] **Step 5: Check mutation boundaries**

Run:

```bash
git diff --check
git status --short
git -C /home/wangjunbo/workspace-iot-platform/things status --short
```

Expected: no whitespace errors; `things` output empty; `backend/go.work.sum` remains only the pre-existing user modification and is not staged.

- [ ] **Step 6: Perform one time-boxed O0a milestone Review**

Review only the O0a delta against this plan and Sections 6.1, 13.1, 14, and 17 of the approved design. Check this exact list:

```text
legacy include is rejected
each changed path has exactly one active owner
watches may overlap without ownership
closed/planned Loops do not route current work
Context path and diff modes share one resolver
L0 no longer directly matches graphctl implementation
things remains read-only
go.work.sum remains user-owned and untouched
```

- [ ] **Step 7: Final commit checkpoint**

Show `git status --short` and the exact staged proposal first. After explicit user authorization only:

```bash
git add .graph/schemas/loop-v1.schema.json .graph/loops/G0-rd-graph-control-plane.yaml .graph/loops/L0-things-baseline.yaml AGENTS.md backend/tools/README.md backend/tools/cmd/graphctl/main.go backend/tools/cmd/graphctl/main_test.go backend/tools/internal/graphctl/app.go backend/tools/internal/graphctl/app_test.go backend/tools/internal/graphctl/changes/git.go backend/tools/internal/graphctl/changes/changes_test.go backend/tools/internal/graphctl/registry/types.go backend/tools/internal/graphctl/registry/validate.go backend/tools/internal/graphctl/registry/impact.go backend/tools/internal/graphctl/registry/registry_test.go backend/tools/internal/graphctl/scope/impact.go backend/tools/internal/graphctl/scope/scope_test.go backend/tools/internal/graphctl/evidence/digest.go backend/tools/internal/graphctl/evidence/discover.go backend/tools/internal/graphctl/evidence/evidence_test.go backend/tools/internal/graphctl/evidence/export_test.go backend/tools/internal/graphctl/review/discover.go docs/superpowers/specs/2026-08-18-rd-graph-fast-full-control-plane-design.md docs/superpowers/plans/2026-08-18-o0a-graph-active-ownership-context.md
git commit -m "feat(graph): establish active ownership context"
```

Do not stage `backend/go.work.sum`, ignored runtime, or anything under `things`. Task 18 files must already be in their own approved commit before this final checkpoint.

## O0a completion criteria

O0a is complete only when all of these are true:

1. Strict Schema accepts `owns/watches` and rejects `include`.
2. Every changed/untracked path has exactly one active owner; multiple active owners fail with a stable typed error.
3. Active watches may overlap and join direct/downstream impact without acquiring ownership.
4. Planned and closed Loops do not route current changes.
5. `graphctl validate` rejects conflicts on currently tracked files.
6. `graphctl context --path`, diff Context, and authoritative scope-check use the same resolver.
7. G0 owns shared Graph/tooling assets; L0 owns only L0 assets and watches shared inputs.
8. Graphctl implementation changes do not directly match L0, although control-plane `globalImpact` may still affect L0.
9. Targeted tests and the one milestone Gate pass with recorded output.
10. No mutation occurred in `things` or the pre-existing `backend/go.work.sum`.

O0 remains open after O0a. Write and execute the O0b historical-lifecycle plan before declaring closed-Loop history frozen under the new semantics.

## Design traceability

| Approved design requirement | O0a coverage |
|---|---|
| Section 6.1 path/diff Context | Tasks 4-5 |
| Section 13 unique owner and Context equivalence tests | Tasks 3-5 |
| Section 14 O0 ownership first | All tasks; O0a is the first bounded slice |
| Section 17.1 Bug is not a Loop | Task 5 machine lookup plus Task 6 AGENTS guidance |
| Section 17.2 `owns/watches` and fail-closed errors | Tasks 1-4 |
| Section 17.3 affected is distinct from historical validity | Active affected routing in Task 3; historical projection deliberately belongs to O0b |
| Section 17.4 closed Loop freeze | Routing half in Task 3; proof/history half is the explicit O0b gap |
| Section 17.5 narrow L0 | Task 4, with Task 6 operator regression checks |
| Section 17.6 component ownership over broad aggregation | Tasks 2 and 4 |
