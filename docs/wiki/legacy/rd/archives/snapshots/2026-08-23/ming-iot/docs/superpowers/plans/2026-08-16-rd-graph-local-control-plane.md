# R&D Graph Local Control Plane Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build G0: a repository-native, Registry-driven local R&D control plane that registers G0 and L0, computes affected Loops, runs deterministic gates, tracks Evidence/Review freshness, and generates the dashboard served at `127.0.0.1:18081`.

**Architecture:** A strict YAML/JSON Registry feeds a deterministic DAG, Git change detector, scope engine, digest engine, serial gate runner, Evidence/Review status engine, and atomic renderer. `cmd/graphctl` remains a thin CLI over focused packages under `backend/tools/internal/graphctl`; the existing `thingsbaseline` remains the L0 domain Validator. The same `graphctl ci` path serves manual checks and the local `pre-push` gate.

**Tech Stack:** Go module language version 1.24.4 (current local toolchain 1.25.4), `gopkg.in/yaml.v3`, `github.com/santhosh-tekuri/jsonschema/v6 v6.0.3`, `golang.org/x/sys/unix`, Git plumbing commands, Bash Git hook, Go `html/template`, systemd, and Python's read-only static HTTP server.

---

## Execution constraints

- Work only in `/home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/g0-rd-graph-local-control-plane` until the explicit integration task.
- Treat `/home/wangjunbo/workspace-iot-platform/things` as read-only. Never edit it or run a command intended to update its checkout.
- Never run `go work sync`.
- Preserve the canonical worktree's pre-existing `backend/go.work.sum` modification. It is not part of G0.
- `graphctl ci` may write only ignored `.graph/runtime` output. It must not update tracked Evidence, Review, Registry, or source files.
- Every commit step below is a checkpoint, not standing authorization. Run `git add`/`git commit` only after the user explicitly authorizes commits for the current implementation session. Never push without a separate explicit authorization.
- Do not install the Git hook or alter systemd from the temporary worktree. Those are explicit operator steps near the end.
- In subagent-driven execution, use Sol as the coordinator and Luna only for bounded tasks with disjoint write sets (fixtures, table-driven tests, platform-specific safe-open helpers, and documentation checks). The main agent retains Registry semantics, shared interfaces, digest/security decisions, Evidence/Review acceptance, canonical integration, and systemd changes. Timebox read-only fleets; never put an immediate critical-path decision behind a nested-agent wait.
- Baseline already verified on 2026-08-16:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/g0-rd-graph-local-control-plane/backend/tools
GOWORK=off go test ./...
```

Expected baseline: all current `backend/tools` packages pass.

## File and package map

```text
ming-iot/
├── AGENTS.md
├── .gitignore
├── .githooks/
│   └── pre-push
├── .graph/
│   ├── registry.yaml
│   ├── loops/
│   │   ├── G0-rd-graph-control-plane.yaml
│   │   └── L0-things-baseline.yaml
│   └── schemas/
│       └── loop-v1.schema.json
├── docs/superpowers/
│   ├── evidence/G0-rd-graph-control-plane/
│   ├── reviews/G0-rd-graph-control-plane/
│   ├── specs/2026-08-16-rd-graph-local-control-plane-design.md
│   └── plans/2026-08-16-rd-graph-local-control-plane.md
└── backend/tools/
    ├── go.mod
    ├── go.sum
    ├── README.md
    ├── cmd/graphctl/
    │   ├── main.go
    │   └── main_test.go
    └── internal/graphctl/
        ├── app.go
        ├── app_test.go
        ├── registry/{types.go,load.go,validate.go,registry_test.go}
        ├── graph/{dag.go,dag_test.go}
        ├── changes/{git.go,push.go,changes_test.go}
        ├── scope/{match.go,impact.go,scope_test.go}
        ├── runner/{command.go,process_linux.go,process_other.go,ci.go,runner_test.go}
        ├── evidence/{model.go,digest.go,discover.go,status.go,export.go,atomic_linux.go,atomic_other.go,evidence_test.go}
        ├── review/{model.go,validate.go,discover.go,review_test.go}
        └── render/{model.go,render.go,index.html.tmpl,render_test.go}
```

`registry` owns parsing/validation; `graph` owns DAG ordering; `changes` owns Git ranges; `scope` owns impact; `runner` owns Gate processes; `evidence` owns digests/runtime/export/status; `review` owns Review validation; `render` owns generated output. `internal/graphctl` composes them, while `cmd/graphctl` only parses CLI input and maps errors to exit codes.

### Task 1: Define the Registry model and pin the schema dependency

**Files:**
- Create: `backend/tools/internal/graphctl/registry/types.go`
- Create: `backend/tools/internal/graphctl/registry/registry_test.go`
- Modify: `backend/tools/go.mod`
- Modify: `backend/tools/go.sum`

- [ ] **Step 1: Write the failing lifecycle test**

```go
package registry

import "testing"

func TestValidateLoopLifecycleContract(t *testing.T) {
	active := Loop{
		APIVersion: APIVersionV1,
		Kind:       KindLoop,
		Metadata:   Metadata{ID: "G0-rd-graph-control-plane", Title: "R&D Graph"},
		Spec: LoopSpec{
			Lifecycle: LifecycleActive,
			Scope: ScopeSpec{Include: []string{".graph/loops/G0-rd-graph-control-plane.yaml"}},
			Review: ReviewSpec{RequiredForCompletion: true},
			Evidence: EvidenceSpec{RequiredForCompletion: true, SubjectPaths: []string{"backend/tools/cmd/graphctl/**"}},
		},
	}
	if err := ValidateLoop(&active); err != nil {
		t.Fatalf("active loop rejected: %v", err)
	}
	closed := active
	closed.Metadata.ID = "L1-example"
	closed.Spec.Lifecycle = LifecycleClosed
	if err := ValidateLoop(&closed); err == nil || err.Error() != "closed loop requires completion" {
		t.Fatalf("closed loop error = %v", err)
	}
	active.Spec.Completion = &Completion{}
	if err := ValidateLoop(&active); err == nil || err.Error() != "completion requires closed lifecycle" {
		t.Fatalf("active completion error = %v", err)
	}
}
```

- [ ] **Step 2: Verify the red state**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/registry -run TestValidateLoopLifecycleContract
```

Expected: FAIL because `Loop` and `ValidateLoop` do not exist.

- [ ] **Step 3: Add the JSON Schema dependency**

Run:

```bash
cd backend/tools
GOWORK=off go get github.com/santhosh-tekuri/jsonschema/v6@v6.0.3
```

Expected: `go.mod` adds the direct requirement and `go.sum` records its checksums.

- [ ] **Step 4: Add stable contract types**

Implement `types.go` with these exact public shapes:

```go
package registry

const (
	APIVersionV1 = "graph.ming.io/v1"
	KindRegistry = "Registry"
	KindLoop     = "Loop"
)

type Lifecycle string

const (
	LifecyclePlanned Lifecycle = "planned"
	LifecycleActive  Lifecycle = "active"
	LifecycleClosed  Lifecycle = "closed"
)

type RegistryFile struct {
	APIVersion   string           `yaml:"apiVersion" json:"apiVersion"`
	Kind         string           `yaml:"kind" json:"kind"`
	Schema       string           `yaml:"schema" json:"schema"`
	LoopsDir     string           `yaml:"loopsDir" json:"loopsDir"`
	RuntimeDir   string           `yaml:"runtimeDir" json:"runtimeDir"`
	ControlPlane ControlPlaneSpec `yaml:"controlPlane" json:"controlPlane"`
}

type ControlPlaneSpec struct {
	OwnerLoop    string   `yaml:"ownerLoop" json:"ownerLoop"`
	GlobalImpact []string `yaml:"globalImpact" json:"globalImpact"`
}

type Loop struct {
	SourcePath string   `yaml:"-" json:"-"`
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       string   `yaml:"kind" json:"kind"`
	Metadata   Metadata `yaml:"metadata" json:"metadata"`
	Spec       LoopSpec `yaml:"spec" json:"spec"`
}

type Metadata struct {
	ID    string `yaml:"id" json:"id"`
	Title string `yaml:"title" json:"title"`
}

type LoopSpec struct {
	Lifecycle Lifecycle    `yaml:"lifecycle" json:"lifecycle"`
	DependsOn []string     `yaml:"dependsOn" json:"dependsOn"`
	Scope     ScopeSpec    `yaml:"scope" json:"scope"`
	Gates     []GateSpec   `yaml:"gates" json:"gates"`
	Evidence  EvidenceSpec `yaml:"evidence" json:"evidence"`
	Review    ReviewSpec   `yaml:"review" json:"review"`
	Completion *Completion `yaml:"completion,omitempty" json:"completion,omitempty"`
}

type ScopeSpec struct {
	Include         []string         `yaml:"include" json:"include"`
	Exclude         []string         `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	ExternalSources []ExternalSource `yaml:"externalSources,omitempty" json:"externalSources,omitempty"`
}

type ExternalSource struct {
	Path   string `yaml:"path" json:"path"`
	Access string `yaml:"access" json:"access"`
}

type GateSpec struct {
	ID        string            `yaml:"id" json:"id"`
	Kind      string            `yaml:"kind" json:"kind"`
	Required  bool              `yaml:"required" json:"required"`
	Validator ValidatorSpec     `yaml:"validator" json:"validator"`
	Workdir   string            `yaml:"workdir" json:"workdir"`
	Argv      []string          `yaml:"argv" json:"argv"`
	Env       map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Timeout   string            `yaml:"timeout" json:"timeout"`
}

type ValidatorSpec struct {
	ID          string   `yaml:"id" json:"id"`
	Version     string   `yaml:"version" json:"version"`
	SourcePaths []string `yaml:"sourcePaths" json:"sourcePaths"`
	Artifacts   []string `yaml:"artifacts,omitempty" json:"artifacts,omitempty"`
}

type EvidenceSpec struct {
	RequiredForCompletion bool     `yaml:"requiredForCompletion" json:"requiredForCompletion"`
	SubjectPaths          []string `yaml:"subjectPaths" json:"subjectPaths"`
	Canonical             []string `yaml:"canonical,omitempty" json:"canonical,omitempty"`
}

type ReviewSpec struct {
	RequiredForCompletion bool `yaml:"requiredForCompletion" json:"requiredForCompletion"`
}

type ArtifactRef struct {
	Path   string `yaml:"path" json:"path"`
	SHA256 string `yaml:"sha256" json:"sha256"`
}

type Completion struct {
	SubjectCommit      string            `yaml:"subjectCommit" json:"subjectCommit"`
	SubjectScopeDigest string            `yaml:"subjectScopeDigest" json:"subjectScopeDigest"`
	RegistryDigest     string            `yaml:"registryDigest" json:"registryDigest"`
	DependencyDigests  map[string]string `yaml:"dependencyDigests" json:"dependencyDigests"`
	EvidenceRefs       []ArtifactRef     `yaml:"evidenceRefs" json:"evidenceRefs"`
	ReviewRef          *ArtifactRef      `yaml:"reviewRef,omitempty" json:"reviewRef,omitempty"`
	LegacyReviewRefs   []ArtifactRef     `yaml:"legacyReviewRefs,omitempty" json:"legacyReviewRefs,omitempty"`
}

type Registry struct {
	RepoRoot string
	Entry    RegistryFile
	Loops    []*Loop
	ByID     map[string]*Loop
}
```

Implement `ValidateLoop` for the red test and stable basic checks: API version, kind, non-empty ID/title, valid lifecycle, non-empty include/subject paths, completion required for `closed`, and completion forbidden for `planned`/`active`.

- [ ] **Step 5: Run focused and compile-only tests**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/registry
GOWORK=off go test -run '^$' ./...
```

Expected: PASS.

- [ ] **Step 6: Commit checkpoint**

After explicit commit authorization only:

```bash
git add backend/tools/go.mod backend/tools/go.sum backend/tools/internal/graphctl/registry/types.go backend/tools/internal/graphctl/registry/registry_test.go
git commit -m "feat(graph): define registry contracts"
```

### Task 2: Load and validate Registry documents fail closed

**Files:**
- Create: `backend/tools/internal/graphctl/registry/load.go`
- Create: `backend/tools/internal/graphctl/registry/validate.go`
- Create: `.graph/schemas/loop-v1.schema.json`
- Modify: `backend/tools/internal/graphctl/registry/registry_test.go`

- [ ] **Step 1: Add failing loader/schema tests**

Create helpers that write a complete temporary `.graph/registry.yaml` and Loop file. Test unknown YAML fields, a second YAML document, JSON Schema violations, non-canonical schema/loops/runtime locations, duplicate IDs, filename/ID mismatch, missing owner, duplicate normalized paths, illegal globs, bad `${RUN_DIR}` placement, forbidden environment keys, non-positive timeouts, absolute project paths, external-source escape, unowned manifest, and non-L0 `legacyReviewRefs`.

```go
func TestLoadRejectsUnknownFieldAndSecondDocument(t *testing.T) {
	for _, tt := range []struct {
		name   string
		suffix string
		want   string
	}{
		{"unknown", "\nunknown: true\n", "field unknown not found"},
		{"second-document", "\n---\napiVersion: graph.ming.io/v1\nkind: Loop\n", "multiple YAML documents"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := writeRegistryFixture(t, validLoopYAML()+tt.suffix)
			_, err := Load(root)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}
```

Define `writeRegistryFixture` and `validLoopYAML` in the same test file; they must create all required fields and copy the real schema from the checkout.

- [ ] **Step 2: Verify the red state**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/registry
```

Expected: FAIL because `Load` and the schema do not exist.

- [ ] **Step 3: Write the complete Draft 2020-12 schema**

The schema must use `$schema=https://json-schema.org/draft/2020-12/schema` and `$id=https://ming.io/schemas/graph/loop-v1.schema.json`; its root `oneOf` validates both the Registry entry and Loop documents; every object uses `additionalProperties:false`; and `$defs` covers paths, digests, artifacts, Gates, Evidence, Review, and completion. Enforce:

- constants for API version and kind;
- lifecycle enum;
- Loop, Gate, and Validator IDs matching `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`, so IDs can never introduce path separators;
- non-empty unique arrays where order is semantic;
- `^sha256:[0-9a-f]{64}$` digests and `^[0-9a-f]{40,64}$` commits;
- Gate kind `command` and non-empty argv;
- `closed` requires completion, while `planned` and `active` forbid it;
- non-L0 completion requires `reviewRef` and forbids `legacyReviewRefs`;
- L0 completion may use exactly one of `reviewRef` or non-empty `legacyReviewRefs`.

- [ ] **Step 4: Implement strict loading**

Expose:

```go
func Load(repoRoot string) (*Registry, error)
func DecodeStrict[T any](path string) (T, error)
func ValidateRegistry(reg *Registry) error
func ValidatePattern(pattern string, external bool) error
func ValidateRunArgument(argument string) error
```

`DecodeStrict` uses `yaml.NewDecoder`, `KnownFields(true)`, and a second decode that must return `io.EOF`. `Load` reads the entry; parses schema bytes with `jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))`; registers that document with `compiler.AddResource(schemaID, doc)`; compiles the same `$id`; byte-sorts regular, non-symlink `*.yaml` Loop files; strict-decodes each file; converts the typed value through JSON into an instance value for schema validation; assigns `SourcePath`; builds `ByID`; and runs semantic validation. No schema/network fetch is allowed after the local resource is registered.

Semantic validation must enforce the project layout (`schema=.graph/schemas/loop-v1.schema.json`, `loopsDir=.graph/loops`, `runtimeDir=.graph/runtime`), self-manifest ownership, path grammar, canonical ref coverage, and timeout parsing. Reject `.git/**` and `.graph/runtime/**` from every Loop scope; reject Registry/Loop manifests, runtime, canonical Evidence, Review, and completion metadata from subject/Validator source sets; constrain canonical Evidence and Review refs to their respective `docs/superpowers` trees. `validator.sourcePaths` are repository globs covered by Loop scope, while `validator.artifacts` are exact, glob-free paths relative to the Gate run directory and therefore are not checked against repository scope. Also enforce allowed env keys (`GOWORK`, `GOFLAGS`, `CGO_ENABLED`, `TZ`, `LANG`, `LC_ALL`), clean run placeholders, external source containment under the workspace parent without symlink escape, file/hash existence for completion, and L0-only legacy review. Registry loading validates pattern syntax and containment but does not require every ordinary include/subject/Validator glob to match immediately; this permits one atomic change to introduce policy/source files and their Registry declarations. Digest execution rejects an entirely empty subject or Validator source set, and repository-contract tests after Task 14 prove every intended G0/L0 pattern resolves to the expected tracked file type. Canonical Evidence, Review, completion refs, literal workdirs, and external sources are not deferred: they must exist with the required type when declared.

- [ ] **Step 5: Run tests and normalize dependencies**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/registry
GOWORK=off go mod tidy
GOWORK=off go test ./internal/graphctl/registry -count=2
python3 -m json.tool ../../.graph/schemas/loop-v1.schema.json >/dev/null
```

Expected: PASS; `jsonschema/v6 v6.0.3` remains direct.

- [ ] **Step 6: Commit checkpoint**

After explicit commit authorization only:

```bash
git add .graph/schemas/loop-v1.schema.json backend/tools/go.mod backend/tools/go.sum backend/tools/internal/graphctl/registry
git commit -m "feat(graph): load registry fail closed"
```

### Task 3: Build a deterministic Loop DAG

**Files:**
- Create: `backend/tools/internal/graphctl/graph/dag.go`
- Create: `backend/tools/internal/graphctl/graph/dag_test.go`

- [ ] **Step 1: Write failing DAG tests**

```go
func TestDAGTopologicalOrderAndDownstreamClosure(t *testing.T) {
	loops := []*registry.Loop{
		{Metadata: registry.Metadata{ID: "L3"}, Spec: registry.LoopSpec{DependsOn: []string{"L1"}}},
		{Metadata: registry.Metadata{ID: "L0"}},
		{Metadata: registry.Metadata{ID: "L2"}, Spec: registry.LoopSpec{DependsOn: []string{"L1"}}},
		{Metadata: registry.Metadata{ID: "L1"}, Spec: registry.LoopSpec{DependsOn: []string{"L0"}}},
	}
	dag, err := Build(loops)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"L0", "L1", "L2", "L3"}, dag.Order()); diff != "" {
		t.Fatalf("Order mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"L0", "L1", "L2", "L3"}, dag.Affected([]string{"L0"})); diff != "" {
		t.Fatalf("Affected mismatch (-want +got):\n%s", diff)
	}
}
```

Also require deterministic errors for a missing dependency and `dependency cycle: L1 -> L2 -> L1`.

- [ ] **Step 2: Verify red**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/graph
```

Expected: FAIL because `Build` is undefined.

- [ ] **Step 3: Implement the DAG**

```go
type DAG struct {
	byID       map[string]*registry.Loop
	order      []string
	downstream map[string][]string
}

func Build(loops []*registry.Loop) (*DAG, error)
func (d *DAG) Order() []string
func (d *DAG) Affected(seedIDs []string) []string
func (d *DAG) OrderSubset(ids []string) []string
func (d *DAG) Loop(id string) (*registry.Loop, bool)
```

Use Kahn's algorithm with a lexicographically sorted ready queue. Sort reverse edges. If not every node is consumed, run deterministic DFS over sorted IDs/edges to report one concrete cycle. Return defensive slice copies.

- [ ] **Step 4: Run focused and race tests**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/graph
GOWORK=off go test -race ./internal/graphctl/graph
```

Expected: PASS.

- [ ] **Step 5: Commit checkpoint**

After explicit commit authorization only:

```bash
git add backend/tools/internal/graphctl/graph
git commit -m "feat(graph): add deterministic loop dag"
```

### Task 4: Match scopes and compute impact

**Files:**
- Create: `backend/tools/internal/graphctl/scope/match.go`
- Create: `backend/tools/internal/graphctl/scope/impact.go`
- Create: `backend/tools/internal/graphctl/scope/scope_test.go`

- [ ] **Step 1: Write failing scope tests**

Cover literals, `*` within one segment, `**` over zero or more segments, Chinese paths, excludes, overlapping owners, uncovered paths, one Loop manifest, and global impact.

```go
func TestAffectedLoopsDistinguishesManifestAndGlobalImpact(t *testing.T) {
	reg, dag := graphFixture(t)
	got, err := Affected(reg, dag, []string{".graph/loops/L0-things-baseline.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"L0-things-baseline"}, got.LoopIDs); diff != "" {
		t.Fatalf("manifest impact mismatch (-want +got):\n%s", diff)
	}
	got, err = Affected(reg, dag, []string{"backend/tools/cmd/graphctl/main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"G0-rd-graph-control-plane", "L0-things-baseline"}, got.LoopIDs); diff != "" {
		t.Fatalf("global impact mismatch (-want +got):\n%s", diff)
	}
}
```

- [ ] **Step 2: Verify red**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/scope
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement matching and impact**

```go
func Match(pattern, repoPath string) bool

type Impact struct {
	ChangedPaths []string `json:"changedPaths"`
	DirectIDs    []string `json:"directLoopIds"`
	LoopIDs      []string `json:"affectedLoopIds"`
	Global       bool     `json:"globalImpact"`
}

func Affected(reg *registry.Registry, dag *graph.DAG, changed []string) (Impact, error)
```

Split on `/`. Match `**` with memoized pattern/path indexes and `*` against runes within one segment. Apply excludes after includes. Multiple matches create multiple direct owners. Global paths seed G0 plus every non-planned Loop. Return typed `ErrUnownedPath` for any uncovered change. Sort/deduplicate all output.

- [ ] **Step 4: Run tests**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/scope
GOWORK=off go test -race ./internal/graphctl/scope
```

Expected: PASS.

- [ ] **Step 5: Commit checkpoint**

After explicit commit authorization only:

```bash
git add backend/tools/internal/graphctl/scope
git commit -m "feat(graph): compute scope impact"
```

### Task 5: Resolve Git changes and prove pre-push targets

**Files:**
- Create: `backend/tools/internal/graphctl/changes/git.go`
- Create: `backend/tools/internal/graphctl/changes/push.go`
- Create: `backend/tools/internal/graphctl/changes/changes_test.go`

- [ ] **Step 1: Write failing tests in real temporary Git repositories**

Initialize a temporary repository and local bare `origin` in test helpers. Cover committed, staged, unstaged, and non-ignored untracked paths; ignored files; a cross-Loop rename yielding both old and new paths; an explicit base/head range that deliberately excludes worktree changes; existing and new remote branches; deletion refs; malformed ref/SHA fields; a local SHA different from current HEAD; and a dirty worktree. Include a filename with spaces and a Chinese filename to prove NUL-delimited parsing. Task 14 separately exercises zero/one/multiple raw Hook stdin records.

- [ ] **Step 2: Verify red**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/changes
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement a shell-free Git client**

```go
type CommandRunner interface {
	Run(ctx context.Context, dir string, args ...string) ([]byte, error)
}

type Client struct {
	RepoRoot string
	Run      CommandRunner
}

type Range struct {
	Base            string
	Head            string
	IncludeWorktree bool
}

type PushRef struct {
	LocalRef  string
	LocalSHA  string
	RemoteRef string
	RemoteSHA string
	Delete    bool
}

func DiscoverRepoRoot(ctx context.Context, start string, run CommandRunner) (string, error)
func ValidatePushRef(ref PushRef) error
func (c Client) ManualRange(ctx context.Context) (Range, error)
func (c Client) PushRange(ctx context.Context, ref PushRef) (Range, error)
func (c Client) ChangedPaths(ctx context.Context, r Range) ([]string, error)
func (c Client) ProveCurrentPush(ctx context.Context, ref PushRef) error
```

Call `git` directly with argv. Manual mode unions and byte-sorts:

```text
git diff --name-only -z --no-renames BASE HEAD
git diff --cached --name-only -z --no-renames
git diff --name-only -z --no-renames
git ls-files --others --exclude-standard -z
```

Pre-push accepts one record. Validate the remote ref as a non-whitespace `refs/...` name. The local ref must also be `refs/...`, except that Git's literal `(delete)` is required when the local object ID is all zero. Validate object IDs as 40-64 lowercase hexadecimal characters; the all-zero object ID is allowed only in the deletion/new-ref positions. For a non-deletion update it requires `localSHA == git rev-parse HEAD` and an empty `git status --porcelain=v1 -z --untracked-files=all`. Existing branches use `merge-base(remoteSHA,localSHA)`; new branches use `merge-base(origin/master,localSHA)`. A deletion validates input but skips code Gates and the current-HEAD proof.

- [ ] **Step 4: Run focused and race tests**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/changes
GOWORK=off go test -race ./internal/graphctl/changes
```

Expected: PASS.

- [ ] **Step 5: Commit checkpoint**

After explicit commit authorization only:

```bash
git add backend/tools/internal/graphctl/changes
git commit -m "feat(graph): detect git impact safely"
```

### Task 6: Compute deterministic Evidence digests

**Files:**
- Create: `backend/tools/internal/graphctl/evidence/model.go`
- Create: `backend/tools/internal/graphctl/evidence/digest.go`
- Create: `backend/tools/internal/graphctl/evidence/evidence_test.go`

- [ ] **Step 1: Write failing digest tests**

Build temporary Git repositories containing a regular file, executable, symlink, and identical content under a different absolute checkout. Assert:

- input order, mtime, and checkout path do not change `scopeDigest`;
- path, mode, and content do change it;
- title, lifecycle, completion, canonical refs, and YAML formatting do not change `registryDigest`;
- dependencies, scope, Gate, subject paths, Review requirement, and global execution settings do change it;
- changing any G0 subject file that also belongs to `controlPlane.globalImpact` changes every Loop's `registryDigest` without creating a domain dependency edge, while G0-only documentation does not;
- Validator source changes alter `validatorDigest`;
- dependency `approvalDigest` changes alter downstream freshness;
- Registry/Loop YAML and Evidence/Review paths are rejected from subject and Validator source sets.

- [ ] **Step 2: Verify red**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/evidence -run 'Test(Scope|Registry|Validator|Approval)Digest'
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Define versioned Evidence models**

```go
type GateStatus string

const (
	GateNotRun GateStatus = "NOT_RUN"
	GatePass   GateStatus = "PASS"
	GateFail   GateStatus = "FAIL"
	GateStale  GateStatus = "STALE"
	GateBlocked GateStatus = "BLOCKED"
)

type ValidatorIdentity struct {
	GateID           string `json:"gateId"`
	ValidatorID      string `json:"validatorId"`
	ValidatorVersion string `json:"validatorVersion"`
	ValidatorDigest  string `json:"validatorDigest"`
}

type CommandRecord struct {
	GateID      string     `json:"gateId"`
	Argv        []string   `json:"argv"`
	Workdir     string     `json:"workdir"`
	Result      GateStatus `json:"result"`
	ExitCode    *int       `json:"exitCode"`
	DurationMS  int64      `json:"durationMs"`
	LogSHA256   string     `json:"logSha256"`
	RetainedLogSHA256 string `json:"retainedLogSha256"`
	LogPath     string     `json:"logPath"`
	LogBytes    int64      `json:"logBytes"`
	LogTruncated bool      `json:"logTruncated"`
	FailureKind string     `json:"failureKind,omitempty"`
}

type ArtifactRecord struct {
	GateID string `json:"gateId"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type RuntimeEvidence struct {
	SchemaVersion     int                 `json:"schemaVersion"`
	RunID             string              `json:"runId"`
	LoopID            string              `json:"loopId"`
	SubjectCommit     string              `json:"subjectCommit"`
	CanonicalEligible bool                `json:"canonicalEligible"`
	ScopeDigest       string              `json:"scopeDigest"`
	RegistryDigest    string              `json:"registryDigest"`
	DependencyDigests map[string]string   `json:"dependencyDigests"`
	Validators        []ValidatorIdentity `json:"validators"`
	Commands          []CommandRecord     `json:"commands"`
	Result            GateStatus          `json:"result"`
	StartedAt         time.Time           `json:"startedAt"`
	DurationMS        int64               `json:"durationMs"`
	Artifacts         []ArtifactRecord    `json:"artifacts"`
}

type CanonicalEvidence struct {
	RuntimeEvidence
	ExportedAt time.Time `json:"exportedAt"`
}

type ApprovalInput struct {
	LoopID             string
	ScopeDigest        string
	RegistryDigest     string
	ValidatorDigests   map[string]string
	DependencyDigests  map[string]string
	EvidenceRefs       []registry.ArtifactRef
	ReviewRef          registry.ArtifactRef
}

type RegistryDigestInput struct {
	Entry              registry.RegistryFile
	Loop               *registry.Loop
	ControlPlaneDigest string
}
```

Use schema version `1`. Normalize nil slices/maps before marshaling and append exactly one newline.

- [ ] **Step 4: Implement exact framing**

```go
func ScopeDigest(ctx context.Context, repoRoot, commit string, patterns []string) (string, error)
func ControlPlaneDigest(ctx context.Context, repoRoot, commit string, reg *registry.Registry) (string, error)
func RegistryDigest(input RegistryDigestInput) (string, error)
func ValidatorDigest(ctx context.Context, repoRoot, commit string, gate registry.GateSpec) (string, error)
func ApprovalDigest(input ApprovalInput) (string, error)
```

`ScopeDigest` uses `git ls-tree -r -z --full-tree` and `git cat-file blob OBJECT_ID`. Byte-sort paths, write `SCOPE-DIGEST-V1\0`, then big-endian path length/path, mode length/mode, content length/raw blob. Reject submodules and empty matches. `ValidatorDigest` uses the same record framing with `VALIDATOR-DIGEST-V1\0`.

`RegistryDigest` must marshal a dedicated projection containing execution semantics and excluding display title, lifecycle, completion, canonical refs, and source filename. Before computing any Loop registry digest, expand the owner G0 Loop's `evidence.subjectPaths` and `controlPlane.globalImpact` at the same subject commit, intersect the matched path sets, and frame that non-empty set with `CONTROL-PLANE-DIGEST-V1\0`. The frame includes the ordered global-impact patterns, only those ordered G0 subject patterns that select at least one global-impact file, and the sorted path/mode/content records; changing G0-only documentation patterns therefore remains local. Include the result as `ControlPlaneDigest` in every Loop projection. This binds all proof to the exact graphctl/schema/Hook/AGENTS control-plane bytes while keeping G0-only documentation local and keeping global impact separate from domain `dependsOn`. Use sorted key/value slices instead of maps. `ApprovalDigest` includes the Loop ID, scope/Registry/Validator/dependency digests, and canonical Evidence/Review content hashes. `ApprovalInput` is defined in this task so the evidence package compiles before Task 7 introduces `LoopStatus`.

- [ ] **Step 5: Run twice for determinism**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/evidence -run 'Test(Scope|Registry|Validator|Approval)Digest' -count=2
```

Expected: PASS both times.

- [ ] **Step 6: Commit checkpoint**

After explicit commit authorization only:

```bash
git add backend/tools/internal/graphctl/evidence
git commit -m "feat(graph): add deterministic evidence digests"
```

### Task 7: Validate Review/completion and derive Loop state

**Files:**
- Create: `backend/tools/internal/graphctl/review/model.go`
- Create: `backend/tools/internal/graphctl/review/validate.go`
- Create: `backend/tools/internal/graphctl/review/discover.go`
- Create: `backend/tools/internal/graphctl/review/review_test.go`
- Create: `backend/tools/internal/graphctl/evidence/discover.go`
- Create: `backend/tools/internal/graphctl/evidence/status.go`
- Modify: `backend/tools/internal/graphctl/evidence/evidence_test.go`

- [ ] **Step 1: Write failing Review and status tests**

Use real temporary Git repositories containing committed and uncommitted Evidence/Review files. Cover PASS with all findings closed, CHANGES_REQUIRED with an open finding, malformed/unknown JSON, hash mismatch, missing required Evidence, missing required Review, planned, Gate failure, blocked, not-run, unapproved dependency, matching closed completion, stale closed completion, L0-only legacy refs with exact hash validation, and the rule that uncommitted candidates are diagnostic input but never canonical. Cover deterministic selection of the newest valid current Review and rejection of an equal-time ambiguity.

- [ ] **Step 2: Verify red**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/review ./internal/graphctl/evidence
```

Expected: FAIL because Review/status APIs do not exist.

- [ ] **Step 3: Implement the Review contract**

```go
type Verdict string

const (
	VerdictPass            Verdict = "PASS"
	VerdictChangesRequired Verdict = "CHANGES_REQUIRED"
)

type Finding struct {
	ID       string   `json:"id"`
	Severity string   `json:"severity"`
	Status   string   `json:"status"`
	Summary  string   `json:"summary"`
	Evidence []string `json:"evidence"`
}

type FileRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type Review struct {
	SchemaVersion     int               `json:"schemaVersion"`
	LoopID            string            `json:"loopId"`
	ScopeDigest       string            `json:"scopeDigest"`
	RegistryDigest    string            `json:"registryDigest"`
	DependencyDigests map[string]string `json:"dependencyDigests"`
	SpecRefs          []FileRef         `json:"specRefs"`
	EvidenceRefs      []FileRef         `json:"evidenceRefs"`
	ReviewerProfile   string            `json:"reviewerProfile"`
	Verdict           Verdict           `json:"verdict"`
	Findings          []Finding         `json:"findings"`
	ReviewedAt        time.Time         `json:"reviewedAt"`
}

type Target struct {
	LoopID            string
	ScopeDigest       string
	RegistryDigest    string
	DependencyDigests map[string]string
	RequiredSpecRefs  []FileRef
	CurrentEvidence   []FileRef
}

type Candidate struct {
	Ref    FileRef
	Review Review
}

func Load(path string) (Review, error)
func Validate(repoRoot string, target Target, r Review) error
func DiscoverCommitted(ctx context.Context, repoRoot string, loop *registry.Loop) ([]Candidate, error)
func SelectCurrent(repoRoot string, target Target, candidates []Candidate) (*Candidate, error)
```

Use `json.Decoder.DisallowUnknownFields` and reject trailing documents. PASS forbids open findings. `RequiredSpecRefs` is the byte-sorted set of committed `docs/superpowers/specs/**` files matched by the Loop include; Review `specRefs` must equal it exactly. `evidenceRefs` must be non-empty and each item must name one current canonical Evidence candidate. Validate every path, hash, and bound digest against committed `HEAD` objects, not uncommitted working-tree bytes.

`DiscoverCommitted` only considers committed `review.json` files matched by the Loop include, byte-sorts paths, and never trusts an untracked Review. `SelectCurrent` first filters candidates through `Validate`, then selects the unique greatest `reviewedAt`; two valid candidates sharing that greatest timestamp are an ambiguity error instead of an arbitrary PASS.

In `evidence/discover.go`, define and implement:

```go
type CanonicalCandidate struct {
	Ref      registry.ArtifactRef
	Evidence CanonicalEvidence
}

func DiscoverCanonical(ctx context.Context, repoRoot string, loop *registry.Loop) ([]CanonicalCandidate, error)
```

Expand committed `HEAD` paths matched by the Loop include, consider only bundle files named `evidence.json` below `docs/superpowers/evidence/`, strict-decode them, verify `loopId`, every recorded artifact path/hash, and byte-sort the result. Historical L0 flat Evidence remains auditable through `evidence.canonical` and `legacyReviewRefs`; it is not mis-decoded as the new bundle schema. A candidate is current only when its scope/Registry/Validator/dependency digests match current values.

- [ ] **Step 4: Implement status precedence**

```go
type OverallStatus string

type ReviewStatus string

const (
	ReviewPending         ReviewStatus = "PENDING"
	ReviewPass            ReviewStatus = "PASS"
	ReviewChangesRequired ReviewStatus = "CHANGES_REQUIRED"
	ReviewStale           ReviewStatus = "STALE"
)

const (
	OverallPlanned           OverallStatus = "PLANNED"
	OverallStale             OverallStatus = "STALE"
	OverallGateFailed        OverallStatus = "GATE_FAILED"
	OverallBlocked           OverallStatus = "BLOCKED_BY_DEPENDENCY"
	OverallNotRun            OverallStatus = "NOT_RUN"
	OverallWaitingDependency OverallStatus = "WAITING_FOR_DEPENDENCY"
	OverallReviewPending     OverallStatus = "REVIEW_PENDING"
	OverallChangesRequired   OverallStatus = "CHANGES_REQUIRED"
	OverallReadyForApproval  OverallStatus = "READY_FOR_APPROVAL"
	OverallApproved          OverallStatus = "APPROVED"
)

type LoopStatus struct {
	LoopID            string                 `json:"loopId"`
	GateStatus        GateStatus             `json:"gateStatus"`
	ReviewStatus      ReviewStatus           `json:"reviewStatus"`
	Lifecycle         registry.Lifecycle     `json:"lifecycle"`
	Overall           OverallStatus          `json:"overall"`
	ScopeDigest       string                 `json:"scopeDigest"`
	RegistryDigest    string                 `json:"registryDigest"`
	ValidatorDigests  map[string]string      `json:"validatorDigests"`
	DependencyDigests map[string]string      `json:"dependencyDigests"`
	ApprovalDigest    string                 `json:"approvalDigest,omitempty"`
	CanonicalEvidence []registry.ArtifactRef `json:"canonicalEvidence"`
	ReviewRef         *registry.ArtifactRef  `json:"reviewRef,omitempty"`
	StaleReasons      []string               `json:"staleReasons"`
}

type StatusInput struct {
	Loop               *registry.Loop
	GateStatus         GateStatus
	CurrentScopeDigest string
	CurrentRegistryDigest string
	CurrentValidatorDigests map[string]string
	CurrentDependencyDigests map[string]string
	DependenciesApproved bool
	Review             *review.Review
	ReviewValidationErr error
	CanonicalEvidence  []registry.ArtifactRef
	SelectedReviewRef  *registry.ArtifactRef
}

func DeriveStatus(input StatusInput) LoopStatus
```

Apply the approved precedence exactly. A required Evidence bundle that has never been committed is reported as REVIEW_PENDING with an explicit missing-Evidence reason; a present but digest-invalid bundle is STALE. For an active Loop, the selected Review's `evidenceRefs` become the candidate canonical Evidence set shown in status. For a closed Loop, completion must bind exactly that selected Review ref and Evidence-ref set; any path/hash/set mismatch is STALE. Build `ApprovalInput` only from these exact refs, never from every historical candidate in the namespace. Digest mismatch overrides historical approval. REVIEW_PENDING and CHANGES_REQUIRED affect overall state but never the ordinary Push Gate exit code.

- [ ] **Step 5: Run focused and race tests**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/review ./internal/graphctl/evidence
GOWORK=off go test -race ./internal/graphctl/review ./internal/graphctl/evidence
```

Expected: PASS.

- [ ] **Step 6: Commit checkpoint**

After explicit commit authorization only:

```bash
git add backend/tools/internal/graphctl/review backend/tools/internal/graphctl/evidence
git commit -m "feat(graph): derive review and approval state"
```

### Task 8: Run Gate commands safely

**Files:**
- Create: `backend/tools/internal/graphctl/runner/command.go`
- Create: `backend/tools/internal/graphctl/runner/process_linux.go`
- Create: `backend/tools/internal/graphctl/runner/process_other.go`
- Create: `backend/tools/internal/graphctl/runner/runner_test.go`

- [ ] **Step 1: Write failing subprocess tests**

Use the current test binary as a helper subprocess, selected by `GRAPHCTL_HELPER_PROCESS=1`. Test PASS, non-zero exit, spawn failure, timeout with a child process, context cancellation, literal argv preservation, invalid placeholder, artifact collection, and 1 MiB log-tail truncation with full-stream SHA-256.

```go
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GRAPHCTL_HELPER_PROCESS") != "1" {
		return
	}
	switch os.Getenv("GRAPHCTL_HELPER_MODE") {
	case "pass":
		fmt.Fprint(os.Stdout, "ok")
		os.Exit(0)
	case "fail":
		fmt.Fprint(os.Stderr, "failed")
		os.Exit(7)
	case "wait":
		for {
			time.Sleep(time.Hour)
		}
	}
}
```

- [ ] **Step 2: Verify red**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/runner -run 'Test(Command|Timeout|Placeholder|Artifact)'
```

Expected: FAIL because `RunCommand` does not exist.

- [ ] **Step 3: Implement argv-only execution**

```go
type CommandRequest struct {
	RepoRoot string
	RunDir   string
	Gate     registry.GateSpec
}

type CommandResult struct {
	Record    evidence.CommandRecord
	Artifacts []evidence.ArtifactRecord
}

func RunCommand(ctx context.Context, request CommandRequest) (CommandResult, error)
func ExpandRunArgument(argument, runDir string) (string, error)
```

Use `exec.Command` with the validated, expanded argv array and no shell. Resolve `workdir` through symlinks and require the resulting directory to remain under the repository root. Inherit the process environment, then apply only Registry-validated overrides. Record the declarative argv with `${RUN_DIR}` intact and the workdir as a repository-relative path, so Evidence never embeds the checkout or random runtime absolute path. Stream stdout/stderr through a mutex-protected combined bounded writer that retains at most 1 MiB while hashing and counting the full byte stream; record `logSha256` for that full stream and `retainedLogSha256` for the retained tail, then write the retained log under the Gate run directory. `CommandRecord.logPath` is relative to the aggregate run directory and never contains a checkout or temporary absolute path.

On Linux set a new process group and kill the entire group on timeout/cancellation. `process_other.go` must compile and terminate the direct process, while recording that full process-group cleanup is unsupported. Required artifact paths are relative to the Gate run directory, regular files only, no symlink traversal, and recorded with size/SHA-256.

- [ ] **Step 4: Run focused and race tests**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/runner
GOWORK=off go test -race ./internal/graphctl/runner
```

Expected: PASS and no surviving helper process.

- [ ] **Step 5: Commit checkpoint**

After explicit commit authorization only:

```bash
git add backend/tools/internal/graphctl/runner
git commit -m "feat(graph): execute gates fail closed"
```

### Task 9: Orchestrate serial CI and write runtime reports

**Files:**
- Create: `backend/tools/internal/graphctl/runner/ci.go`
- Create: `backend/tools/internal/graphctl/app.go`
- Create: `backend/tools/internal/graphctl/app_test.go`
- Modify: `backend/tools/internal/graphctl/runner/runner_test.go`

- [ ] **Step 1: Write failing orchestration tests**

Inject a fake Gate executor, fake clock, and fake run-ID generator. Assert stable topological execution, required failure, timeout, independent-branch continuation, downstream BLOCKED, `--all` excluding planned Loops, collision-safe run-directory creation, runtime-only writes, canonical eligibility only for clean HEAD, relevant uncommitted changes making approval status STALE without turning a successful diagnostic Gate into FAIL, committed-only Evidence/Review discovery, and REVIEW_PENDING not changing a successful Push Gate exit.

- [ ] **Step 2: Verify red**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl -run TestCI
GOWORK=off go test ./internal/graphctl/runner -run TestCI
```

Expected: FAIL because `App.CI` and `runner.RunCI` do not exist.

- [ ] **Step 3: Implement serial orchestration**

```go
type GateExecutor interface {
	Run(context.Context, CommandRequest) (CommandResult, error)
}

type RunIDSource interface {
	New(time.Time) (string, error)
}

type CIRequest struct {
	RepoRoot  string
	Range     changes.Range
	All       bool
	PrePush   *changes.PushRef
}

type RunReport struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Enforcement   string                `json:"enforcement"`
	RunID         string                `json:"runId"`
	Base          string                `json:"base"`
	Head          string                `json:"head"`
	Impact        scope.Impact          `json:"impact"`
	Loops         []evidence.RuntimeEvidence `json:"loops"`
	Result        evidence.GateStatus   `json:"result"`
	StartedAt     time.Time             `json:"startedAt"`
	DurationMS    int64                 `json:"durationMs"`
}

func RunCI(ctx context.Context, reg *registry.Registry, dag *graph.DAG, request CIRequest, execute GateExecutor, ids RunIDSource) (RunReport, error)
```

Execute required Gates serially in topological order. A failed upstream marks downstream BLOCKED without spawning commands; independent branches continue. Compute digests before execution. Generate run IDs as `YYYYMMDDTHHMMSS.nnnnnnnnnZ-<16 lowercase hex>` using UTC time plus 8 bytes from `crypto/rand`, validate the format, and create the run directory exclusively; never reuse an existing run ID. Write each Gate under `.graph/runtime/runs/RUN_ID/LOOP_ID/GATE_ID`. A completed required-Gate failure is represented by `RunReport.result=FAIL` with a nil orchestration error so it can still be rendered; the CLI exits 1 from the result. Registry, Git, digest, run-directory, or publication failures return a typed control-plane error and can never degrade to PASS.

Implement `graphctl.App` with injected Git client, Gate executor, clock, and renderer. Define the root orchestration request/result types in `app.go` so the CLI never imports package-private details:

```go
type ValidateRequest struct{ RepoRoot string }
type ValidateResult struct {
	SchemaVersion int      `json:"schemaVersion"`
	LoopIDs       []string `json:"loopIds"`
}
type ContextRequest struct { RepoRoot, LoopID string }
type ContextResult struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Loop          *registry.Loop      `json:"loop"`
	Status        evidence.LoopStatus `json:"status"`
}
type ScopeRequest struct { RepoRoot, Base, Head string }
type StatusRequest struct{ RepoRoot string }
type StatusResult struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Loops         []evidence.LoopStatus `json:"loops"`
}
type CIRequest struct {
	RepoRoot, Base, Head string
	All bool
	PushRef *changes.PushRef
}
type ExportRequest struct { RepoRoot, LoopID, RunID, OutputDir string }
type RenderRequest struct{ RepoRoot string }
```

`RunCI` atomically writes its completed aggregate report to `.graph/runtime/runs/<run-id>/run.json`; incomplete run directories are never treated as current. `App.CI` loads and validates the Registry, resolves changes, computes impact, converts the root request to `runner.CIRequest`, runs Gates, validates any existing canonical Evidence/Review freshness, and passes the completed report to the renderer. The renderer alone publishes root `latest-run.json`. `App.Status` and post-CI status construction discover canonical candidates from committed `HEAD`; if staged, unstaged, or non-ignored untracked paths intersect a Loop's subject/Registry/Validator inputs, that Loop cannot remain APPROVED and receives a concrete STALE reason. Diagnostic Gate PASS remains independent from this approval freshness. The App must never create or modify tracked files.

- [ ] **Step 4: Run tests**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl ./internal/graphctl/runner
GOWORK=off go test -race ./internal/graphctl ./internal/graphctl/runner
```

Expected: PASS.

- [ ] **Step 5: Commit checkpoint**

After explicit commit authorization only:

```bash
git add backend/tools/internal/graphctl/app.go backend/tools/internal/graphctl/app_test.go backend/tools/internal/graphctl/runner
git commit -m "feat(graph): orchestrate local ci"
```

### Task 10: Export immutable canonical Evidence bundles

**Files:**
- Create: `backend/tools/internal/graphctl/evidence/export.go`
- Create: `backend/tools/internal/graphctl/evidence/atomic_linux.go`
- Create: `backend/tools/internal/graphctl/evidence/atomic_other.go`
- Modify: `backend/tools/internal/graphctl/evidence/evidence_test.go`
- Modify: `backend/tools/internal/graphctl/app.go`

- [ ] **Step 1: Write failing export tests**

Cover a clean PASS run, dirty/non-PASS/stale/not-current runs, a HEAD/worktree change introduced during export, output outside `docs/superpowers/evidence`, output not covered by Loop scope, target already existing, a concurrent target-creation race, symlink parent/target, missing or hash-mismatched Gate logs/declared artifacts, and successful copy/hash/rewrite into a new bundle.

- [ ] **Step 2: Verify red**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/evidence -run TestExport
```

Expected: FAIL because `Export` does not exist.

- [ ] **Step 3: Implement non-overwriting export**

```go
type ExportRequest struct {
	RepoRoot string
	Run      RuntimeEvidence
	Loop     *registry.Loop
	OutputDir string
	Now      time.Time
}

func Export(ctx context.Context, request ExportRequest) (string, error)
```

Require PASS, `canonicalEligible=true`, current HEAD equal to `subjectCommit`, current digests equal to the run, and a clean tracked/staged/non-ignored-untracked state before the export begins. Required Validator identities, command argv/workdir, command set, and declared artifact set must exactly match the current Gate contract. The output must be a not-yet-existing directory under `docs/superpowers/evidence`, and every generated `evidence.json`, log, and artifact path must match the Loop include/exclude scope.

Create a sibling temporary directory with mode `0700`, write versioned `evidence.json`, `logs/`, and `artifacts/`. Verify each retained Gate log against `CommandRecord.retainedLogSha256`; keep `logSha256` as the separately recorded full-stream digest, copy the retained bytes to `logs/<gate-id>.log`, and rewrite `logPath` to the bundle-relative path. Copy only Validator-declared regular artifacts without following symlinks into `artifacts/<gate-id>/<declared-relative-path>`, rewrite artifact paths relative to the bundle, and preserve/verify every size and SHA-256. After fsyncing the temporary tree, repeat the HEAD, Registry/dependency, digest, tracked/staged, and non-ignored-untracked checks; the structured status comparison may exclude exactly the exporter-created temporary directory and nothing else. If anything changed since the initial check, remove only that private temporary tree and fail. Then rename to the final non-existing destination. On Linux use `openat`/`O_NOFOLLOW` patterns adapted from `internal/thingsbaseline.WriteEvidenceAtomic` and `unix.Renameat2(..., unix.RENAME_NOREPLACE)` for the final publication, so a target created after the pre-check cannot be overwritten. The non-Linux implementation must use a platform-supported no-replace primitive or return an explicit unsupported-security error; it must never silently weaken into overwrite.

- [ ] **Step 4: Run export tests**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/evidence -run TestExport
GOWORK=off go test -race ./internal/graphctl/evidence
```

Expected: PASS and no temporary bundle remains after a rejected export.

- [ ] **Step 5: Commit checkpoint**

After explicit commit authorization only:

```bash
git add backend/tools/internal/graphctl/evidence backend/tools/internal/graphctl/app.go
git commit -m "feat(graph): export canonical evidence"
```

### Task 11: Render versioned machine state and a read-only dashboard

**Files:**
- Create: `backend/tools/internal/graphctl/render/model.go`
- Create: `backend/tools/internal/graphctl/render/render.go`
- Create: `backend/tools/internal/graphctl/render/index.html.tmpl`
- Create: `backend/tools/internal/graphctl/render/render_test.go`
- Modify: `backend/tools/internal/graphctl/app.go`

- [ ] **Step 1: Write failing renderer tests**

Build a two-Loop view containing APPROVED, STALE, GATE_FAILED, REVIEW_PENDING, and BLOCKED fixture variants. Assert:

- stable Loop/edge/order JSON;
- `schemaVersion=1` and one shared `generationId`;
- `Enforcement: LOCAL_GATE` in JSON and HTML;
- Hook installed/not-installed state;
- direct dependency edges separate from G0 global impact;
- failure phase, duration, Evidence, Review, and lifecycle visible;
- no write API, remote script, or external asset;
- interrupted publication leaves the previous valid files intact.

- [ ] **Step 2: Verify red**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/render
```

Expected: FAIL because the renderer does not exist.

- [ ] **Step 3: Define the view model**

```go
type GraphStatus struct {
	SchemaVersion int          `json:"schemaVersion"`
	GenerationID  string       `json:"generationId"`
	GeneratedAt   time.Time    `json:"generatedAt"`
	Enforcement   string       `json:"enforcement"`
	HookInstalled bool         `json:"hookInstalled"`
	Loops         []LoopView   `json:"loops"`
	Edges         []EdgeView   `json:"edges"`
	GlobalImpact  []string     `json:"globalImpact"`
	LatestRun     *RunSummary  `json:"latestRun"`
}

type RunSummary struct {
	RunID        string              `json:"runId"`
	Result       evidence.GateStatus `json:"result"`
	StartedAt    time.Time           `json:"startedAt"`
	DurationMS   int64               `json:"durationMs"`
	FailurePhase string              `json:"failurePhase,omitempty"`
}

type LatestRunDocument struct {
	SchemaVersion int               `json:"schemaVersion"`
	GenerationID  string            `json:"generationId"`
	Run           *runner.RunReport `json:"run"`
}

type EdgeView struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type LoopView struct {
	ID           string                 `json:"id"`
	Title        string                 `json:"title"`
	DependsOn    []string               `json:"dependsOn"`
	GateStatus   evidence.GateStatus    `json:"gateStatus"`
	ReviewStatus evidence.ReviewStatus  `json:"reviewStatus"`
	Lifecycle    registry.Lifecycle     `json:"lifecycle"`
	Overall      evidence.OverallStatus `json:"overall"`
	StaleReasons []string               `json:"staleReasons"`
}
```

Use sorted slices only. Emit one `EdgeView{From: dependency, To: dependent, Kind: "depends_on"}` per direct domain edge; never encode G0 global impact as a fake dependency edge. HTML must be self-contained, responsive, readable at normal browser zoom, and show summary cards plus a dependency table/list; do not attempt a heavy graph library in G0.

- [ ] **Step 4: Implement validated atomic publication**

```go
type Request struct {
	RuntimeDir string
	Status     GraphStatus
	LatestRun  *runner.RunReport
}

func Render(request Request) error
```

Generate `graph-status.json`, `latest-run.json`, and `index.html` into same-directory temporary files. `latest-run.json` is a `LatestRunDocument`, not a bare `RunReport`; before the first CI run it is still emitted with `run:null`. Both JSON documents and an HTML metadata attribute carry the same generation ID. Parse the generated JSON back into typed structs and parse/execute the HTML template before publication. Create the runtime root with mode `0755` and publish these three serving files with mode `0644`, so the existing non-root systemd service can read output even when graphctl was run by another local account; keep detailed Gate run directories/logs non-world-readable. Fsync, rename JSON first and `index.html` last, so a reader can detect an interrupted mixed generation. Never delete `runtime/runs`.

- [ ] **Step 5: Run renderer tests**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/render
GOWORK=off go test -race ./internal/graphctl/render
```

Expected: PASS.

- [ ] **Step 6: Commit checkpoint**

After explicit commit authorization only:

```bash
git add backend/tools/internal/graphctl/render backend/tools/internal/graphctl/app.go
git commit -m "feat(graph): render local control dashboard"
```

### Task 12: Expose the complete thin graphctl CLI

**Files:**
- Create: `backend/tools/cmd/graphctl/main.go`
- Create: `backend/tools/cmd/graphctl/main_test.go`
- Modify: `backend/tools/internal/graphctl/app.go`

- [ ] **Step 1: Write failing CLI tests**

Use an injected `application` interface. Cover usage errors (exit 2), operational failures (exit 1), PASS (exit 0), JSON stdout without logs, human output, duplicate flags, missing Loop, `--all`, explicit base/head, all four pre-push fields, and nested `evidence export` parsing.

- [ ] **Step 2: Verify red**

```bash
cd backend/tools
GOWORK=off go test ./cmd/graphctl
```

Expected: FAIL because the command does not exist.

- [ ] **Step 3: Implement exact commands and flags**

The usage contract is:

```text
graphctl validate [--repo-root DIR] [--format=human|json]
graphctl context --loop ID [--repo-root DIR] [--format=human|json]
graphctl scope-check [--repo-root DIR] [--base SHA --head SHA] [--format=human|json]
graphctl status [--repo-root DIR] [--format=human|json]
graphctl ci [--repo-root DIR] [--base SHA --head SHA] [--all] [--format=human|json]
graphctl ci --repo-root DIR --push-local-ref REF --push-local-sha SHA --push-remote-ref REF --push-remote-sha SHA
graphctl evidence export --loop ID --run ID --output DIR [--repo-root DIR] [--format=human|json]
graphctl render [--repo-root DIR] [--format=human|json]
```

Define:

```go
type application interface {
	Validate(context.Context, graphctl.ValidateRequest) (graphctl.ValidateResult, error)
	Context(context.Context, graphctl.ContextRequest) (graphctl.ContextResult, error)
	ScopeCheck(context.Context, graphctl.ScopeRequest) (scope.Impact, error)
	Status(context.Context, graphctl.StatusRequest) (graphctl.StatusResult, error)
	CI(context.Context, graphctl.CIRequest) (runner.RunReport, error)
	Export(context.Context, graphctl.ExportRequest) (string, error)
	Render(context.Context, graphctl.RenderRequest) error
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, app application) int

type errorDocument struct {
	SchemaVersion int         `json:"schemaVersion"`
	Error         errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Phase   string `json:"phase,omitempty"`
}
```

Follow `cmd/thingsbaseline`: reject duplicate flags, use context cancelled by SIGINT/SIGTERM, exit 2 for syntax/usage, exit 1 for execution/validation failure, and keep JSON stdout pure. In JSON mode, write exactly one `errorDocument` to stdout for either usage or operational failure and put any diagnostic stack/context on stderr; in human mode print concise diagnostics to stderr. Every JSON document has `schemaVersion` and ends with one newline.

- [ ] **Step 4: Run CLI and compile checks**

```bash
cd backend/tools
GOWORK=off go test ./cmd/graphctl
GOWORK=off go test -run '^$' ./...
GOWORK=off go build ./cmd/graphctl
```

Expected: PASS.

- [ ] **Step 5: Commit checkpoint**

After explicit commit authorization only:

```bash
git add backend/tools/cmd/graphctl backend/tools/internal/graphctl/app.go
git commit -m "feat(graph): expose graphctl commands"
```

### Task 13: Register G0 and the real L0 contract

**Files:**
- Create: `.graph/registry.yaml`
- Create: `.graph/loops/G0-rd-graph-control-plane.yaml`
- Create: `.graph/loops/L0-things-baseline.yaml`
- Modify: `backend/tools/internal/graphctl/registry/registry_test.go`
- Modify: `backend/tools/internal/graphctl/app_test.go`

- [ ] **Step 1: Write failing repository-contract tests**

Load the actual checkout and assert exact IDs, no domain dependency edge between G0/L0, G0 as control-plane owner, L0 external source read-only, L0 Gate invoking `thingsbaseline verify` with runtime Evidence, historical files in scope, G0 global impact selecting both active Loops, and every declared path having valid normalized syntax. The final tracked-file resolution assertions are added in Task 14 after AGENTS, Hook, and ignore policy exist.

- [ ] **Step 2: Verify red**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/registry ./internal/graphctl -run 'TestRepositoryRegistry'
```

Expected: FAIL because the Registry files do not exist.

- [ ] **Step 3: Create the Registry entry**

```yaml
apiVersion: graph.ming.io/v1
kind: Registry
schema: .graph/schemas/loop-v1.schema.json
loopsDir: .graph/loops
runtimeDir: .graph/runtime
controlPlane:
  ownerLoop: G0-rd-graph-control-plane
  globalImpact:
    - .graph/registry.yaml
    - .graph/schemas/**
    - .githooks/**
    - AGENTS.md
    - .gitignore
    - backend/tools/cmd/graphctl/**
    - backend/tools/internal/graphctl/**
    - backend/tools/go.mod
    - backend/tools/go.sum
```

- [ ] **Step 4: Create the active G0 Loop**

Use `lifecycle: active`, `dependsOn: []`, no completion, and this exact scope:

```yaml
scope:
  include:
    - .graph/loops/G0-rd-graph-control-plane.yaml
    - .graph/registry.yaml
    - .graph/schemas/**
    - .githooks/**
    - AGENTS.md
    - .gitignore
    - backend/tools/cmd/graphctl/**
    - backend/tools/internal/graphctl/**
    - backend/tools/go.mod
    - backend/tools/go.sum
    - backend/tools/README.md
    - docs/superpowers/specs/2026-08-16-rd-graph-local-control-plane-design.md
    - docs/superpowers/plans/2026-08-16-rd-graph-local-control-plane.md
    - docs/superpowers/evidence/G0-rd-graph-control-plane/**
    - docs/superpowers/reviews/G0-rd-graph-control-plane/**
```

Its required Gate is:

```yaml
- id: graphctl-tests
  kind: command
  required: true
  validator:
    id: graphctl
    version: v1
    sourcePaths:
      - .graph/schemas/**
      - backend/tools/cmd/graphctl/**
      - backend/tools/internal/graphctl/**
      - backend/tools/go.mod
      - backend/tools/go.sum
  workdir: backend/tools
  argv:
    - go
    - test
    - ./cmd/graphctl
    - ./internal/graphctl/...
  env:
    GOWORK: "off"
  timeout: 10m
```

Use this exact completion input contract:

```yaml
evidence:
  requiredForCompletion: true
  subjectPaths:
    - .graph/schemas/**
    - .githooks/**
    - AGENTS.md
    - .gitignore
    - backend/tools/cmd/graphctl/**
    - backend/tools/internal/graphctl/**
    - backend/tools/go.mod
    - backend/tools/go.sum
    - backend/tools/README.md
review:
  requiredForCompletion: true
```

This excludes `.graph/registry.yaml`, `.graph/loops/**`, runtime, Evidence, Review, completion, design, and plan from `subjectPaths`. Hook and repository policy are proven subject files, while `validator.sourcePaths` contains only the schema and implementation that define the `graphctl-tests` Validator.

- [ ] **Step 5: Create the active L0 Loop from real commit 311d76a**

Use this reviewed scope, not the shorter design example:

```yaml
scope:
  include:
    - .graph/loops/L0-things-baseline.yaml
    - .gitignore
    - backend/go.work
    - backend/go.work.sum
    - backend/saas/go.mod
    - backend/saas/go.sum
    - backend/things/**
    - backend/tools/README.md
    - backend/tools/**/*.go
    - backend/tools/go.mod
    - backend/tools/go.sum
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
  externalSources:
    - path: ../things
      access: read-only
```

The required Gate is:

```yaml
- id: things-baseline
  kind: command
  required: true
  validator:
    id: thingsbaseline
    version: v1
    sourcePaths:
      - backend/tools/cmd/thingsbaseline/**
      - backend/tools/internal/thingsbaseline/**
      - backend/tools/templates/things-baseline/**
      - backend/tools/things-baseline.lock
      - backend/tools/go.mod
      - backend/tools/go.sum
    artifacts:
      - l0-things-evidence.json
  workdir: backend/tools
  argv:
    - go
    - run
    - ./cmd/thingsbaseline
    - verify
    - --lock
    - ./things-baseline.lock
    - --source
    - ../../../things
    - --backend
    - ..
    - --evidence
    - ${RUN_DIR}/l0-things-evidence.json
  env:
    GOWORK: "off"
  timeout: 30m
```

Use this exact completion input contract:

```yaml
evidence:
  requiredForCompletion: true
  subjectPaths:
    - backend/go.work
    - backend/go.work.sum
    - backend/saas/go.mod
    - backend/saas/go.sum
    - backend/things/**
    - backend/tools/**/*.go
    - backend/tools/go.mod
    - backend/tools/go.sum
    - backend/tools/templates/things-baseline/**
    - backend/tools/things-baseline.lock
  canonical:
    - docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.json
    - docs/superpowers/evidence/2026-08-14-l0-things-migration-baseline.md
review:
  requiredForCompletion: true
```

This intentionally makes the old L0 completion stale when G0 adds packages to the tools module. Preserve the final, failed-attempt, and source-baseline Evidence files byte-for-byte; do not overwrite or delete them.

- [ ] **Step 6: Run actual Registry tests**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/registry ./internal/graphctl -run 'TestRepositoryRegistry'
GOWORK=off go run ./cmd/graphctl validate --repo-root ../.. --format=json
```

Expected: PASS; JSON names G0 and L0 in stable order.

- [ ] **Step 7: Commit checkpoint**

After explicit commit authorization only:

```bash
git add .graph backend/tools/internal/graphctl/registry/registry_test.go backend/tools/internal/graphctl/app_test.go
git commit -m "feat(graph): register G0 and L0"
```

### Task 14: Install repository policy and the local pre-push entry point

**Files:**
- Create: `AGENTS.md`
- Create: `.githooks/pre-push`
- Modify: `.gitignore`
- Modify: `backend/tools/README.md`
- Modify: `backend/tools/internal/graphctl/changes/changes_test.go`
- Modify: `backend/tools/internal/graphctl/app_test.go`

- [ ] **Step 1: Add failing policy/Hook tests**

Assert that `.graph/runtime` is ignored, the Hook is executable, Bash syntax is valid, the Hook forwards exactly one ref record into graphctl flags, and `graphctl status` reports whether `core.hooksPath` equals `.githooks`. Extend the repository-contract test to expand all literal implementation inputs and every subject/Validator set against tracked files, reject wrong file types, and prove each aggregate source set is non-empty now that the policy files exist. Future Evidence/Review namespace globs may remain empty until Task 16 creates their first immutable records.

- [ ] **Step 2: Verify red**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/changes ./internal/graphctl -run 'Test(Hook|HooksPath|RuntimeIgnored)'
```

Expected: FAIL because policy/Hook files are absent.

- [ ] **Step 3: Create the Hook**

```bash
#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
mapfile -t updates
if ((${#updates[@]} != 1)); then
  echo "graphctl pre-push: G0 requires exactly one ref update" >&2
  exit 1
fi
read -r -a fields <<<"${updates[0]}"
if ((${#fields[@]} != 4)); then
  echo "graphctl pre-push: malformed ref update" >&2
  exit 1
fi
local_ref=${fields[0]}
local_sha=${fields[1]}
remote_ref=${fields[2]}
remote_sha=${fields[3]}

cd "$repo_root/backend/tools"
exec env GOWORK=off go run ./cmd/graphctl ci \
  --repo-root "$repo_root" \
  --push-local-ref "$local_ref" \
  --push-local-sha "$local_sha" \
  --push-remote-ref "$remote_ref" \
  --push-remote-sha "$remote_sha"
```

Set mode `0755`. The Hook contains no Gate semantics.

- [ ] **Step 4: Create repository AGENTS rules**

`AGENTS.md` must require:

1. run `graphctl validate` and `graphctl context --loop LOOP_ID` before implementation;
2. stop and design/register a Loop when no scope covers the work;
3. remain inside owner/affected scope;
4. run `scope-check` and `ci` before completion claims;
5. report run ID, affected Loops, Gate result, and runtime report path;
6. never call `go work sync` or modify `../things`;
7. never equate PASS with APPROVED;
8. never close lifecycle, commit, push, bypass Hook, or change `core.hooksPath` without explicit user authorization.

Refer to Registry dynamically; do not duplicate Loop lists or Gate commands.

- [ ] **Step 5: Ignore runtime and document commands**

Add exactly `/.graph/runtime/` to `.gitignore`. Add a `R&D Graph local control plane` section to `backend/tools/README.md` with validate/context/scope/status/ci/evidence-export/render commands, Hook installation:

```bash
git config core.hooksPath .githooks
```

and the explicit label `Enforcement: LOCAL_GATE`.

- [ ] **Step 6: Run tests and shell checks**

```bash
chmod 0755 .githooks/pre-push
bash -n .githooks/pre-push
git check-ignore -v .graph/runtime/index.html
cd backend/tools
GOWORK=off go test ./internal/graphctl/changes ./internal/graphctl
```

Expected: PASS. Do not run `git config core.hooksPath` in the temporary worktree.

- [ ] **Step 7: Commit checkpoint**

After explicit commit authorization only:

```bash
git add AGENTS.md .githooks/pre-push .gitignore backend/tools/README.md backend/tools/internal/graphctl/changes/changes_test.go backend/tools/internal/graphctl/app_test.go
git commit -m "feat(graph): enforce local graph workflow"
```

### Task 15: Run the full machine gate and freeze the implementation commit

**Files:**
- Modify as required by failures: only files already listed in Tasks 1-14
- Verify: `docs/superpowers/specs/2026-08-16-rd-graph-local-control-plane-design.md`
- Verify: `docs/superpowers/plans/2026-08-16-rd-graph-local-control-plane.md`

- [ ] **Step 1: Run all deterministic checks**

```bash
cd backend/tools
GOWORK=off go test -count=1 ./...
GOWORK=off go test -race -count=1 ./cmd/graphctl ./internal/graphctl/...
GOWORK=off go vet ./cmd/graphctl ./internal/graphctl/...
GOWORK=off go build ./cmd/graphctl
cd ../..
git diff --check
test -z "$(gofmt -l backend/tools/cmd/graphctl backend/tools/internal/graphctl)"
bash -n .githooks/pre-push
```

Expected: every command exits 0, `gofmt -l` prints nothing.

- [ ] **Step 2: Run every graphctl query in both output modes**

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl validate --repo-root ../..
GOWORK=off go run ./cmd/graphctl validate --repo-root ../.. --format=json
GOWORK=off go run ./cmd/graphctl context --repo-root ../.. --loop G0-rd-graph-control-plane
GOWORK=off go run ./cmd/graphctl context --repo-root ../.. --loop L0-things-baseline --format=json
GOWORK=off go run ./cmd/graphctl scope-check --repo-root ../.. --format=json
GOWORK=off go run ./cmd/graphctl status --repo-root ../.. --format=json
GOWORK=off go run ./cmd/graphctl render --repo-root ../..
```

Expected: valid output; before Gate execution active Loops may be NOT_RUN/REVIEW_PENDING, never APPROVED.

- [ ] **Step 3: Verify renderer output and source isolation**

```bash
cd ../..
test -s .graph/runtime/index.html
python3 -m json.tool .graph/runtime/graph-status.json >/dev/null
python3 -m json.tool .graph/runtime/latest-run.json >/dev/null
git check-ignore -q .graph/runtime/index.html
git -C ../things rev-parse HEAD
git -C ../things status --porcelain=v1
git status --short
```

Expected: runtime ignored, external Things status unchanged from before G0, and only intended G0 tracked/untracked files appear. Do not require the external repository to be globally clean if it was already dirty; compare its before/after HEAD and status bytes.

- [ ] **Step 4: Run graphctl CI in diagnostic mode**

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl ci --repo-root ../.. --all --format=json
```

Expected: both G0 and L0 execute. The L0 Validator may take about ten minutes. Because the worktree still contains uncommitted implementation, the report must say `canonicalEligible=false` even when Gates PASS.

- [ ] **Step 5: Inspect the complete change boundary**

```bash
cd ../..
git status --short
git diff --stat
git diff -- . ':(exclude).graph/runtime'
```

Expected: no `../things` change, no `backend/go.work.sum` change, and no unrelated repository file.

- [ ] **Step 6: Request implementation-commit authorization**

Stop here if the user has not explicitly authorized commits. Present the exact status and machine results. After authorization, stage only the approved G0 files and create the implementation commit:

```bash
git add AGENTS.md .gitignore .githooks .graph backend/tools/go.mod backend/tools/go.sum backend/tools/README.md backend/tools/cmd/graphctl backend/tools/internal/graphctl docs/superpowers/specs/2026-08-16-rd-graph-local-control-plane-design.md docs/superpowers/plans/2026-08-16-rd-graph-local-control-plane.md
git commit -m "feat: establish local R&D graph control plane"
```

Do not add `.graph/runtime`. Do not push.

- [ ] **Step 7: Prove the committed worktree is clean**

```bash
git status --porcelain=v1
git show --stat --oneline HEAD
```

Expected: empty status and one bounded G0 implementation commit (or the explicitly authorized sequence of task commits).

### Task 16: Produce fresh L0/G0 Evidence, independent Reviews, and completion records

**Files:**
- Create: `docs/superpowers/evidence/L0-things-baseline/RUN_ID/evidence.json` and copied artifacts
- Create: `docs/superpowers/evidence/G0-rd-graph-control-plane/RUN_ID/evidence.json` and copied artifacts
- Create: `docs/superpowers/reviews/L0-things-baseline/REVIEW_ID/review.json`
- Create: `docs/superpowers/reviews/G0-rd-graph-control-plane/REVIEW_ID/review.json`
- Modify after explicit user approval: `.graph/loops/L0-things-baseline.yaml`
- Modify after explicit user approval: `.graph/loops/G0-rd-graph-control-plane.yaml`

- [ ] **Step 1: Run a clean full CI for both Loops**

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl ci --repo-root ../.. --all --format=json
```

Expected: G0 PASS, L0 PASS, and `canonicalEligible=true` for both. Record the run ID from JSON. The old L0 Evidence remains untouched.

- [ ] **Step 2: Export L0 Evidence first**

Use the exact run ID printed by Step 1:

```bash
GOWORK=off go run ./cmd/graphctl evidence export \
  --repo-root ../.. \
  --loop L0-things-baseline \
  --run "$L0_RUN_ID" \
  --output "docs/superpowers/evidence/L0-things-baseline/$L0_RUN_ID" \
  --format=json
```

`L0_RUN_ID` is a shell variable copied exactly from graphctl JSON, not a guessed value. Expected: a new non-overwritten bundle containing generic Evidence plus `l0-things-evidence.json`.

- [ ] **Step 3: Commit the L0 Evidence checkpoint after authorization**

Export makes the tree dirty, so a second canonical export is intentionally blocked until this bundle is committed. After explicit authorization:

```bash
git add docs/superpowers/evidence/L0-things-baseline
git commit -m "test(graph): record fresh L0 evidence"
```

Do not overwrite or remove the 2026-08-14/15 historical chain.

- [ ] **Step 4: Rerun clean CI and export G0 Evidence**

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl ci --repo-root ../.. --all --format=json
GOWORK=off go run ./cmd/graphctl evidence export \
  --repo-root ../.. \
  --loop G0-rd-graph-control-plane \
  --run "$G0_RUN_ID" \
  --output "docs/superpowers/evidence/G0-rd-graph-control-plane/$G0_RUN_ID" \
  --format=json
```

Set `G0_RUN_ID` to the exact second-run ID. Expected: the new bundle binds the current committed HEAD while its subject digest remains stable across the L0 Evidence-only commit.

- [ ] **Step 5: Commit the G0 Evidence checkpoint after authorization**

```bash
git add docs/superpowers/evidence/G0-rd-graph-control-plane
git commit -m "test(graph): record G0 control-plane evidence"
```

- [ ] **Step 6: Request independent Review for both affected Loops**

Use `superpowers:requesting-code-review` in fresh independent contexts. Each Reviewer must read the approved spec, complete diff from `311d76a`, current Registry, its Loop's canonical Evidence, prior open findings, and relevant historical L0 chain. Require two strict JSON outputs matching the Review schema, including repository-relative `specRefs` and `evidenceRefs` with exact `sha256:` hashes.

The L0 Reviewer must explicitly confirm that adding graphctl to `backend/tools` did not weaken `thingsbaseline` and that the new full L0 run supersedes, but does not erase, the historical completion. The G0 Reviewer must examine scope coverage, command injection, path/symlink handling, Git-range proof, digest self-reference, process-group timeout, evidence export, Hook behavior, and renderer truthfulness.

- [ ] **Step 7: Validate, commit, then recognize Review files after authorization**

```bash
python3 -m json.tool docs/superpowers/reviews/L0-things-baseline/$L0_REVIEW_ID/review.json >/dev/null
python3 -m json.tool docs/superpowers/reviews/G0-rd-graph-control-plane/$G0_REVIEW_ID/review.json >/dev/null
git status --short
git add docs/superpowers/reviews/L0-things-baseline docs/superpowers/reviews/G0-rd-graph-control-plane
git commit -m "docs(graph): record independent G0 and L0 reviews"
cd backend/tools
GOWORK=off go run ./cmd/graphctl status --repo-root ../.. --format=json
```

`L0_REVIEW_ID` and `G0_REVIEW_ID` come from the immutable Review directory names returned by the reviewers. Before commit, Review files are only candidates and cannot promote status. After the explicitly authorized commit, `graphctl status` discovers them from committed `HEAD`; each Loop is READY_FOR_APPROVAL only when its Review verdict is PASS, every finding is closed, and all bound inputs are current. If either says CHANGES_REQUIRED, return to the owning implementation task, rerun Gates, regenerate affected Evidence, and obtain a fresh Review.

- [ ] **Step 8: Present completion values and request lifecycle authorization**

Run:

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl context --repo-root ../.. --loop L0-things-baseline --format=json
GOWORK=off go run ./cmd/graphctl context --repo-root ../.. --loop G0-rd-graph-control-plane --format=json
```

Present the exact `subjectCommit`, `scopeDigest`, `registryDigest`, `dependencyDigests`, Evidence paths/hashes, and Review paths/hashes. Do not edit lifecycle until the user explicitly authorizes closing each Loop.

- [ ] **Step 9: Write completion only after user approval**

Patch each Loop with the exact machine-produced values. Append each fresh bundle's repository-relative `evidence.json` path to that Loop's `evidence.canonical`; keep the two old L0 final files there as audit history. Use current structured `reviewRef` for L0 and G0, and make each completion `evidenceRefs` point at the fresh bundle's `evidence.json` hash rather than a failed or superseded historical artifact. Do not use `legacyReviewRefs` for the new completion; that compatibility path remains covered by tests for reading the historical pre-G0 state.

Set lifecycle to `closed` only in the same patch as its complete matching completion record.

- [ ] **Step 10: Verify APPROVED without self-staleness**

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl validate --repo-root ../..
GOWORK=off go run ./cmd/graphctl status --repo-root ../.. --format=json
GOWORK=off go run ./cmd/graphctl render --repo-root ../..
```

Expected: both Loops show APPROVED; changing lifecycle/completion did not alter `registryDigest`.

- [ ] **Step 11: Commit completion after authorization**

```bash
cd ../..
git add .graph/loops/L0-things-baseline.yaml .graph/loops/G0-rd-graph-control-plane.yaml
git commit -m "docs(graph): approve G0 and refresh L0 completion"
```

Do not push.

### Task 17: Integrate into canonical ming-iot and cut over the persistent dashboard

**Files:**
- Canonical Git integration: `/home/wangjunbo/workspace-iot-platform/ming-iot`
- Create only after explicit operator approval: `/etc/systemd/system/iot-visual-companion.service.d/10-ming-iot-runtime.conf`
- Preserve: `/etc/systemd/system/iot-visual-companion.service`

- [ ] **Step 1: Use the branch-finishing workflow**

Invoke `superpowers:finishing-a-development-branch`. Present test/Evidence/Review results and integration options. Do not merge, rebase, delete the worktree, or push until the user selects and authorizes an option.

- [ ] **Step 2: Resolve the canonical untracked-spec collision safely**

Before integration, compare the already approved canonical untracked spec to the branch copy:

```bash
cmp -s \
  /home/wangjunbo/workspace-iot-platform/ming-iot/docs/superpowers/specs/2026-08-16-rd-graph-local-control-plane-design.md \
  /home/wangjunbo/workspace-iot-platform/ming-iot/.worktrees/g0-rd-graph-local-control-plane/docs/superpowers/specs/2026-08-16-rd-graph-local-control-plane-design.md
```

Expected: exit 0. Ask the user to authorize a canonical commit containing only that approved spec, or authorize another recoverable resolution. Never remove/move the file merely to make Git proceed. Never include `backend/go.work.sum`.

- [ ] **Step 3: Integrate while preserving the user's dirty file**

After the selected authorization, re-check:

```bash
git -C /home/wangjunbo/workspace-iot-platform/ming-iot status --short
git -C /home/wangjunbo/workspace-iot-platform/ming-iot diff -- backend/go.work.sum
```

Use the integration method selected in Step 1. Confirm the user's `backend/go.work.sum` bytes are unchanged before/after. Do not push.

- [ ] **Step 4: Generate runtime from the canonical checkout**

Because the canonical worktree may still contain the user's pre-existing `backend/go.work.sum` change, use an explicit committed range for this operational run; do not hide or mutate the dirty file:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools
GOWORK=off go run ./cmd/graphctl ci \
  --repo-root ../.. \
  --base "$(git -C ../.. merge-base origin/master HEAD)" \
  --head "$(git -C ../.. rev-parse HEAD)" \
  --all \
  --format=json
GOWORK=off go run ./cmd/graphctl render --repo-root ../..
```

Expected: canonical `.graph/runtime/index.html` and JSON exist. Since `backend/go.work.sum` is a real L0 compilation input, status may truthfully show L0 STALE and the operational run may be `canonicalEligible=false` while that user-owned change remains uncommitted. This run is for serving current state, not completion Evidence.

- [ ] **Step 5: Ask permission to install the local Hook**

After explicit approval only:

```bash
cd /home/wangjunbo/workspace-iot-platform/ming-iot
git config core.hooksPath .githooks
git config --get core.hooksPath
```

Expected: `.githooks`. Dashboard Hook state becomes installed after rerender.

- [ ] **Step 6: Ask permission for the systemd cutover**

Create this drop-in with `apply_patch` only after explicit operator approval:

```ini
[Unit]
Description=Ming IoT R&D Graph Dashboard
Documentation=
Documentation=file:///home/wangjunbo/workspace-iot-platform/ming-iot/docs/superpowers/specs/2026-08-16-rd-graph-local-control-plane-design.md

[Service]
Environment=
ExecStartPre=
ExecStart=
RuntimeDirectory=
BindReadOnlyPaths=
ReadWritePaths=
WorkingDirectory=/home/wangjunbo/workspace-iot-platform/ming-iot/.graph/runtime
ExecStartPre=/usr/bin/test -x /usr/bin/python3
ExecStartPre=/usr/bin/test -r /home/wangjunbo/workspace-iot-platform/ming-iot/.graph/runtime/index.html
ExecStart=/usr/bin/python3 -u -m http.server 18081 --bind 127.0.0.1 --directory /home/wangjunbo/workspace-iot-platform/ming-iot/.graph/runtime
```

Never point it at `.worktrees`. Keep the original base unit intact for rollback.

- [ ] **Step 7: Reload, restart, and verify the service**

After the approved drop-in exists:

```bash
systemd-analyze verify /etc/systemd/system/iot-visual-companion.service
systemctl daemon-reload
systemctl restart iot-visual-companion.service
systemctl is-enabled iot-visual-companion.service
systemctl is-active iot-visual-companion.service
ss -ltnp '( sport = :18081 )'
curl --noproxy '*' -fsS http://127.0.0.1:18081/ | rg -m1 'LOCAL_GATE'
curl --noproxy '*' -fsS http://127.0.0.1:18081/graph-status.json | python3 -m json.tool >/dev/null
journalctl -u iot-visual-companion.service -n 50 --no-pager
```

Expected: enabled, active, bound only to `127.0.0.1:18081`, generated Graph page, valid JSON, no service errors.

- [ ] **Step 8: Record rollback without executing it**

If cutover fails, the authorized rollback is to remove only `10-ming-iot-runtime.conf`, then daemon-reload/restart. Before any removal, show the exact target and get explicit approval. The original companion must then resume serving `iot-doc/.superpowers/visual-live`.

- [ ] **Step 9: Final repository boundary check**

```bash
git -C /home/wangjunbo/workspace-iot-platform/ming-iot status --short
git -C /home/wangjunbo/workspace-iot-platform/things status --short
curl --noproxy '*' -fsS http://127.0.0.1:18081/graph-status.json | python3 -m json.tool
```

Expected: only the user's pre-existing canonical changes remain; `things` before/after state is identical; machine page reflects Registry/runtime rather than the historical hand-written HTML.

## Review hardening addendum (2026-08-18)

Tasks 1-17 record the original G0 build. Tasks 18-23 are the mandatory continuation after the first independent Review and implement design section 16. Where an older task conflicts with this addendum, the addendum wins. Existing L0/G0 Evidence remains immutable history but cannot satisfy completion after the digest and proof contracts change.

The implementation order intentionally follows findings `G0-REV-001` through `G0-REV-004`. Each task starts RED and has its own authorized commit checkpoint. Sol may coordinate Luna workers only inside a task after the main agent fixes the shared interface. Luna write sets must be disjoint; no two agents may concurrently edit `digest.go`, `discover.go`, `status.go`, `export.go`, or `app.go`.

### Task 18: Revalidate canonical Evidence against one shared Gate contract

**Files:**
- Create: `backend/tools/internal/graphctl/evidence/proof.go`
- Create: `backend/tools/internal/graphctl/evidence/proof_test.go`
- Modify: `backend/tools/internal/graphctl/evidence/discover.go`
- Modify: `backend/tools/internal/graphctl/evidence/export.go`
- Modify: `backend/tools/internal/graphctl/evidence/evidence_test.go`
- Modify: `backend/tools/internal/graphctl/evidence/export_test.go`
- Modify: `backend/tools/internal/graphctl/app.go`
- Modify: `backend/tools/internal/graphctl/app_test.go`

- [ ] **Step 1: Add RED tests for forged committed Evidence**

Build each case from `newExportFixture(t)`, call `Export` once to create a structurally valid bundle, mutate only `evidence.json`, commit the bundle, load the Registry, and call the new discovery API. The table must cover all four attacks below:

```go
func TestDiscoverCanonicalRejectsForgedGateProof(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CanonicalEvidence)
		want   string
	}{
		{"missing required command", func(doc *CanonicalEvidence) {
			doc.Commands = nil
		}, "required command set"},
		{"non-clean PASS", func(doc *CanonicalEvidence) {
			code := 1
			doc.Commands[0].ExitCode = &code
			doc.Commands[0].FailureKind = "EXIT_NON_ZERO"
		}, "clean PASS"},
		{"scope not produced by subjectCommit", func(doc *CanonicalEvidence) {
			doc.ScopeDigest = digestOf('9')
		}, "scopeDigest"},
		{"validator not produced by subjectCommit", func(doc *CanonicalEvidence) {
			doc.Validators[0].ValidatorDigest = digestOf('8')
		}, "validatorDigest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newExportFixture(t)
			bundle := exportThenRewriteAndCommit(t, fixture, tt.mutate)
			reg, err := registry.Load(fixture.repo)
			if err != nil {
				t.Fatal(err)
			}
			discovery, err := DiscoverCanonical(
				context.Background(), fixture.repo, reg, reg.ByID["G0"], map[string]string{},
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(discovery.Candidates) != 0 || !rejectionContains(discovery.Rejections, bundle, tt.want) {
				t.Fatalf("discovery = %#v, want rejected %q containing %q", discovery, bundle, tt.want)
			}
		})
	}
}
```

`exportThenRewriteAndCommit` must decode with `decodeCanonicalEvidence`, apply the mutation, write with `MarshalCanonicalEvidence`, and commit exactly the generated bundle. `rejectionContains` must compare both repository path and reason text. These helpers live in `proof_test.go`; they must not bypass `Export` by manufacturing the original valid bundle.

- [ ] **Step 2: Run the focused test and confirm RED**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/evidence -run TestDiscoverCanonicalRejectsForgedGateProof -count=1
```

Expected: compile failure because `CanonicalDiscovery`, the new `DiscoverCanonical` signature, and helper contract do not exist. A passing test before production changes means the attack fixture is ineffective and must be corrected.

- [ ] **Step 3: Introduce the shared Gate contract**

Add these stable types and entry points to `proof.go`:

```go
package evidence

type RequiredGateContract struct {
	GateID           string
	Kind             string
	ValidatorID      string
	ValidatorVersion string
	ValidatorSourcePaths []string
	ValidatorDigest  string
	Argv             []string
	Workdir          string
	Artifacts        []string
}

type GateContract struct {
	LoopID            string
	SubjectCommit     string
	ScopeDigest       string
	RegistryDigest    string
	DependencyDigests map[string]string
	RequiredGates     []RequiredGateContract
}

func BuildGateContract(
	ctx context.Context,
	repoRoot string,
	commit string,
	reg *registry.Registry,
	loop *registry.Loop,
	dependencyDigests map[string]string,
) (GateContract, error)

func ValidateCanonicalGateProof(contract GateContract, candidate CanonicalEvidence) error
```

`BuildGateContract` must:

1. prove `commit^{commit}` exists;
2. compute `ControlPlaneDigest`, `ScopeDigest`, and `RegistryDigest` at that commit;
3. compute each required Gate's Validator digest at that commit and retain its exact `kind`, Validator source paths, and artifact declarations;
4. clone and preserve Registry order for Validator source, argv, and artifact arrays while byte-sorting only map-derived data;
5. reject duplicate required Gate IDs and invalid dependency digests.

`ValidateCanonicalGateProof` must compare subject, scope, Registry, dependencies, Validator identity/digest, required command set, argv, workdir, clean-PASS predicate, expected canonical log paths, and declared canonical artifact set. It must reject missing, duplicate, and extra entries. A canonical proof requires `CanonicalEligible=true`, top-level `PASS`, every required command `PASS`, `exitCode=0`, and empty `failureKind`.

- [ ] **Step 4: Make discovery distinguish current candidates from rejected history**

Replace the old return shape with:

```go
type CanonicalRejection struct {
	Path   string
	Reason string
}

type CanonicalDiscovery struct {
	Candidates []CanonicalCandidate
	Rejections []CanonicalRejection
}

func DiscoverCanonical(
	ctx context.Context,
	repoRoot string,
	reg *registry.Registry,
	loop *registry.Loop,
	dependencyDigests map[string]string,
) (CanonicalDiscovery, error)
```

Malformed JSON, unsafe paths, non-regular committed blobs, and unreadable Git objects remain hard errors. A well-formed historical bundle whose proof no longer matches is excluded from `Candidates` and added to sorted `Rejections`; this preserves history without allowing stale or forged data into Review selection. Every candidate must first pass `BuildGateContract(ctx, repoRoot, candidate.SubjectCommit, reg, loop, dependencyDigests)`, `ValidateCanonicalGateProof`, and the existing committed log/artifact byte verification.

Update `Status` and `currentClosedApproval` to consume only `Candidates`. If at least one bundle was found but all were rejected, append deterministic stale reasons containing each rejected path; never pass rejected refs to `review.SelectCurrent`.

- [ ] **Step 5: Reuse the same contract during export**

Delete the duplicate required-Gate/Validator/command/artifact set logic from `validateExportRun`. Keep runtime-file copying, no-replace publication, and before/after dirty-HEAD checks in `export.go`, but make `verifyCurrentExportState` build `GateContract` and validate the runtime record through a runtime-layout wrapper over the same internal set checker:

```go
func ValidateExportableRuntimeGateProof(contract GateContract, run RuntimeEvidence) error
```

The wrapper differs from canonical layout only in paths: runtime logs are `${LOOP_ID}/${GATE_ID}/command.log`, runtime artifacts retain Validator-relative paths, and exported records use `logs/${GATE_ID}.log` plus `artifacts/${GATE_ID}/${ARTIFACT_PATH}`. The identity, exact-set, clean-PASS, and digest predicates must be shared.

- [ ] **Step 6: Run finding-1 tests**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/evidence -run 'Test(DiscoverCanonical|Export|GateContract)' -count=1
GOWORK=off go test ./internal/graphctl -run 'Test(CICommittedOnlyReview|ExportLoadsExactRuntime)' -count=1
GOWORK=off go test ./internal/graphctl/evidence ./internal/graphctl -count=1
```

Expected: PASS. Existing historical Evidence tests must now assert `Candidates` and `Rejections` explicitly rather than treating every structurally valid old document as current.

- [ ] **Step 7: Commit checkpoint after explicit authorization**

```bash
git add \
  backend/tools/internal/graphctl/evidence/proof.go \
  backend/tools/internal/graphctl/evidence/proof_test.go \
  backend/tools/internal/graphctl/evidence/discover.go \
  backend/tools/internal/graphctl/evidence/export.go \
  backend/tools/internal/graphctl/evidence/evidence_test.go \
  backend/tools/internal/graphctl/evidence/export_test.go \
  backend/tools/internal/graphctl/app.go \
  backend/tools/internal/graphctl/app_test.go
git commit -m "fix(graph): revalidate canonical gate evidence"
```

Never stage `backend/go.work.sum`.

### Task 19: Replace struct-order digests with canonical JSON

**Files:**
- Create: `backend/tools/internal/graphctl/evidence/canonicaljson.go`
- Create: `backend/tools/internal/graphctl/evidence/canonicaljson_test.go`
- Modify: `backend/tools/internal/graphctl/evidence/digest.go`
- Modify: `backend/tools/internal/graphctl/evidence/evidence_test.go`

- [ ] **Step 1: Add RED canonical-byte and final-digest golden tests**

```go
func TestMarshalCanonicalJSONGolden(t *testing.T) {
	input := map[string]any{
		"z": []any{int64(2), "x"},
		"a": map[string]any{"中": "值", "a": int64(-3)},
	}
	want := []byte(`{"a":{"a":-3,"中":"值"},"z":[2,"x"]}`)
	got, err := marshalCanonicalJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("canonical JSON = %s, want %s", got, want)
	}
	escaped, err := marshalCanonicalJSON(map[string]any{"s": "<>&"})
	if err != nil {
		t.Fatal(err)
	}
	if wantEscaped := []byte(`{"s":"\u003c\u003e\u0026"}`); !bytes.Equal(escaped, wantEscaped) {
		t.Fatalf("escaped canonical JSON = %s, want %s", escaped, wantEscaped)
	}
}

func TestCanonicalDigestGoldenVectors(t *testing.T) {
	if got := mustRegistryDigest(t, baseRegistryDigestInput()); got != "sha256:dfca20c95ee13795646dc3ebea6aab627dae8e0f22a837dde3b047c0638cebeb" {
		t.Fatalf("RegistryDigest golden = %s", got)
	}
	input := baseApprovalDigestInput()
	got, err := ApprovalDigest(input)
	if err != nil {
		t.Fatal(err)
	}
	if got != "sha256:115a44fa1b1c68505618dadfd1b7fb5d8878fb9102a957ca567871d1faa2d7cb" {
		t.Fatalf("ApprovalDigest golden = %s", got)
	}
}
```

`baseApprovalDigestInput` must contain the exact values already used by `TestApprovalDigestIsDeterministicAndBindsDependencies`: Gate digests `c/d`, dependency `e`, sorted Evidence paths `a.json/b.json` with hashes `1/f`, and Review hash `2`. Also test nested maps inserted in opposite orders, UTF-8 keys, arrays preserving order, structs and maps producing identical bytes, and rejection of `float32/float64`, exponent numbers, every unsigned integer type, signed overflow, `-0`, channels, functions, complex numbers, and maps with non-string keys.

- [ ] **Step 2: Verify RED against the old struct-order implementation**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/evidence -run 'Test(MarshalCanonicalJSONGolden|CanonicalDigestGoldenVectors)' -count=1
```

Expected: compile failure because `marshalCanonicalJSON` and `baseApprovalDigestInput` do not exist; after adding only test helpers, the old digest hash must differ from the golden value.

- [ ] **Step 3: Implement recursive canonical JSON encoding**

`canonicaljson.go` owns one encoder. It first marshals the projection to ordinary JSON, decodes with `UseNumber`, then recursively writes only the allowed data model:

```go
func marshalCanonicalJSON(value any) ([]byte, error) {
	if err := validateCanonicalInputTypes(reflect.ValueOf(value)); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("canonical JSON: normalize: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var node any
	if err := decoder.Decode(&node); err != nil {
		return nil, fmt.Errorf("canonical JSON: decode normalized value: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("canonical JSON: trailing value")
		}
		return nil, fmt.Errorf("canonical JSON: trailing data: %w", err)
	}
	var output bytes.Buffer
	if err := writeCanonicalJSON(&output, node); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
```

`validateCanonicalInputTypes` recursively dereferences interfaces/pointers; accepts nil, bool, string, signed integer kinds, structs, arrays/slices, and maps whose keys are strings; and rejects floats, unsigned integers, complex values, functions, channels, unsafe pointers, and non-string map keys before JSON erases their original Go type. It walks exported struct fields and all collection elements.

`writeCanonicalJSON` handles only the normalized `nil`, `bool`, `string`, `json.Number`, `[]any`, and `map[string]any` nodes; sorts every object key with Go string byte order; preserves arrays; uses `json.Marshal(string)` for the locked escaping policy; parses numbers with `strconv.ParseInt(..., 10, 64)` and requires `strconv.FormatInt(parsed, 10)` to equal the original token. All other normalized values return an error. Output has no whitespace and no newline.

- [ ] **Step 4: Route only digest projections through the canonical encoder**

In `RegistryDigest` and `ApprovalDigest`, replace `json.Marshal(projection)` with `marshalCanonicalJSON(projection)`. Do not change `MarshalRuntimeEvidence`, `MarshalCanonicalEvidence`, runtime reports, Review JSON, or dashboard JSON; those are documents, not digest preimages. Keep the existing domain-separation prefixes and preimage field semantics unchanged.

- [ ] **Step 5: Run digest tests and document intentional staleness**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/evidence -run 'Test(RegistryDigest|ApprovalDigest|Canonical)' -count=2
GOWORK=off go test ./internal/graphctl/evidence ./internal/graphctl/review ./internal/graphctl -count=1
```

Expected: PASS. Repository status may now report historical Evidence as rejected/stale; do not add a compatibility branch for old digest bytes.

- [ ] **Step 6: Commit checkpoint after explicit authorization**

```bash
git add \
  backend/tools/internal/graphctl/evidence/canonicaljson.go \
  backend/tools/internal/graphctl/evidence/canonicaljson_test.go \
  backend/tools/internal/graphctl/evidence/digest.go \
  backend/tools/internal/graphctl/evidence/evidence_test.go
git commit -m "fix(graph): canonicalize approval digests"
```

Never stage `backend/go.work.sum`.

### Task 20: Bind completion to a real Git subject and selected Evidence

**Files:**
- Create: `backend/tools/internal/graphctl/evidence/completion.go`
- Create: `backend/tools/internal/graphctl/evidence/completion_test.go`
- Modify: `backend/tools/internal/graphctl/evidence/status.go`
- Modify: `backend/tools/internal/graphctl/evidence/evidence_test.go`
- Modify: `backend/tools/internal/graphctl/evidence/export.go`
- Modify: `backend/tools/internal/graphctl/app.go`
- Modify: `backend/tools/internal/graphctl/app_test.go`

- [ ] **Step 1: Add RED completion-proof tests**

Use one committed Review and one proof-valid canonical Evidence candidate. The integration table must assert `OverallStale`, no `ApprovalDigest`, and a specific stale reason for:

```go
func TestStatusRejectsUnprovedCompletionSubject(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*registry.Completion, *evidence.CanonicalEvidence, string)
		want   string
	}{
		{"nonexistent commit", func(c *registry.Completion, _ *evidence.CanonicalEvidence, _ string) {
			c.SubjectCommit = strings.Repeat("f", 40)
		}, "not a Git commit"},
		{"commit scope mismatch", func(c *registry.Completion, _ *evidence.CanonicalEvidence, _ string) {
			c.SubjectScopeDigest = hashAppBytes([]byte("wrong scope"))
		}, "does not produce subjectScopeDigest"},
		{"evidence belongs to another subject", func(_ *registry.Completion, e *evidence.CanonicalEvidence, previous string) {
			e.SubjectCommit = previous
		}, "Evidence subjectCommit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCompletionAppRepo(t)
			tt.mutate(fixture.completion, &fixture.canonical, fixture.previousCommit)
			fixture.commitEvidenceReviewAndCompletion(t)
			status, err := fixture.app.Status(context.Background(), StatusRequest{RepoRoot: fixture.repo})
			if err != nil {
				t.Fatal(err)
			}
			got := status.Loops[0]
			if got.Overall != evidence.OverallStale || got.ApprovalDigest != "" || !containsAppReason(got.StaleReasons, tt.want) {
				t.Fatalf("status = %#v, want STALE reason containing %q", got, tt.want)
			}
		})
	}
}
```

`newCompletionAppRepo` is created in `app_test.go`. It wraps the existing temporary Registry conventions, creates two real commits with identical `evidence.subjectPaths` bytes (the second commit changes only an excluded spec/presentation file), builds one valid canonical Evidence plus Review, and exposes `repo`, `previousCommit`, `completion`, `canonical`, and an `App`. `commitEvidenceReviewAndCompletion` writes all three documents and commits them. `containsAppReason` returns true only when one stale reason contains the requested substring. `completion_test.go` stays inside package `evidence` and tests `BuildCompletionProof` plus the pure `DeriveStatus` contract without importing the parent `graphctl` package. Add a pure-state test proving that a closed Loop cannot become `APPROVED` merely from syntactically matching completion fields when `CompletionProof` is absent.

- [ ] **Step 2: Run focused tests and confirm RED**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/evidence ./internal/graphctl \
  -run 'Test(StatusRejectsUnprovedCompletionSubject|DeriveStatusRequiresCompletionProof)' -count=1
```

Expected: FAIL because current `completionMismatchReasons` validates only syntax/current digests and `StatusInput` has no proof.

- [ ] **Step 3: Add the completion-proof contract**

```go
type CompletionProof struct {
	SubjectCommit      string
	SubjectScopeDigest string
	RegistryDigest     string
	DependencyDigests  map[string]string
	EvidenceRefs       []registry.ArtifactRef
	ReviewRef          *registry.ArtifactRef
}

type CompletionProofRequest struct {
	RepoRoot          string
	Loop              *registry.Loop
	SelectedEvidence  []CanonicalCandidate
	SelectedReview    *review.Review
	SelectedReviewRef *registry.ArtifactRef
}

func BuildCompletionProof(ctx context.Context, request CompletionProofRequest) (CompletionProof, error)
func SelectCanonicalCandidates(candidates []CanonicalCandidate, refs []review.FileRef) ([]CanonicalCandidate, error)
```

`SelectCanonicalCandidates` normalizes Review refs, rejects empty/duplicate refs, and resolves every ref by both path and SHA-256 to exactly one proof-valid candidate; it returns candidates in normalized ref order. `BuildCompletionProof` must require a closed Loop with completion plus a selected structured Review/ref; prove `subjectCommit^{commit}` exists; recompute `ScopeDigest` at that commit; require the selected candidate refs, the selected Review's Evidence refs, and `completion.evidenceRefs` to be exactly equal sets; require the selected Review ref to equal `completion.reviewRef`; require every selected Evidence document's `SubjectCommit` to equal the completion subject; require completion Registry/dependency digests to equal the selected Review; and reject empty or duplicate Evidence selections. It returns cloned normalized refs/maps only after every check succeeds. L0's legacy refs remain readable history, but they do not create this strengthened proof; Task 23 supplies L0 with a fresh structured Review before closure.

- [ ] **Step 4: Keep `DeriveStatus` pure and require the proof**

Extend `StatusInput`:

```go
CompletionProof         *CompletionProof
CompletionValidationErr error
```

For a closed Loop, `DeriveStatus` must mark `STALE` when the proof is nil or errored before considering Gate/Review precedence. `completionMismatchReasons` compares completion with the proof and current Registry/dependency values but performs no Git I/O. An active/planned Loop ignores a nil completion proof because completion is forbidden there by Registry validation.

- [ ] **Step 5: Build the proof in every approval path**

`App.Status` must first resolve every selected Review Evidence ref to exactly one proof-valid `CanonicalCandidate` by both path and SHA-256; unreferenced current candidates are not passed as selected Evidence, and a missing/duplicate match is a validation error. It then calls `BuildCompletionProof` with that selected subset and passes the proof/error to `DeriveStatus`. Refactor `currentClosedApproval` in `export.go` to use the same ref-to-candidate resolver and builder instead of its separate commit/scope checks. This ensures ordinary `status`, dashboard render, dependency approval, and downstream Evidence export all use identical completion strength.

- [ ] **Step 6: Run finding-3 tests**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/evidence -run 'Test(Completion|DeriveStatus)' -count=1
GOWORK=off go test ./internal/graphctl -run 'Test(Status|CICommittedOnlyReview)' -count=1
GOWORK=off go test ./internal/graphctl/evidence ./internal/graphctl -count=1
```

Expected: PASS, and no nonexistent or cross-subject completion produces `APPROVED`.

- [ ] **Step 7: Commit checkpoint after explicit authorization**

```bash
git add \
  backend/tools/internal/graphctl/evidence/completion.go \
  backend/tools/internal/graphctl/evidence/completion_test.go \
  backend/tools/internal/graphctl/evidence/status.go \
  backend/tools/internal/graphctl/evidence/evidence_test.go \
  backend/tools/internal/graphctl/evidence/export.go \
  backend/tools/internal/graphctl/app.go \
  backend/tools/internal/graphctl/app_test.go
git commit -m "fix(graph): bind completion to evidence subjects"
```

Never stage `backend/go.work.sum`.

### Task 21: Treat runtime reports as bounded untrusted input

**Files:**
- Create: `backend/tools/internal/graphctl/runtime_reports.go`
- Create: `backend/tools/internal/graphctl/runtime_reports_test.go`
- Create: `backend/tools/internal/graphctl/runtime_open_linux.go`
- Create: `backend/tools/internal/graphctl/runtime_open_other.go`
- Modify: `backend/tools/internal/graphctl/app.go`
- Modify: `backend/tools/internal/graphctl/app_test.go`
- Modify: `backend/tools/internal/graphctl/evidence/proof.go`
- Modify: `backend/tools/internal/graphctl/evidence/proof_test.go`

- [ ] **Step 1: Add RED safe-loader and semantic-forgery tests**

```go
func TestLoadLatestRuntimeFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		forge func(*testing.T, appRepo, string)
		want string
	}{
		{"invalid run directory id", forgeInvalidRunID, "invalid run ID"},
		{"symlink runs root", forgeRunsRootSymlink, "symlink"},
		{"symlink run directory", forgeRunDirectorySymlink, "symlink"},
		{"symlink run json", forgeRunJSONSymlink, "symlink"},
		{"oversized run json", forgeOversizedRunJSON, "exceeds 16777216 bytes"},
		{"unknown json field", forgeUnknownRunField, "unknown field"},
		{"trailing json document", forgeTrailingRunDocument, "trailing JSON"},
		{"empty commands with PASS", forgeEmptyPassCommands, "required command set"},
		{"duplicate gate command", forgeDuplicateCommand, "duplicate command"},
		{"wrong argv", forgeWrongArgv, "command contract"},
		{"aggregate result mismatch", forgeAggregateResult, "aggregate result"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newAppRepo(t)
			app := newRuntimeTestApp(t)
			runID := writeValidRuntimeReport(t, app, fixture)
			tt.forge(t, fixture, runID)
			before := snapshotRuntimeGeneration(t, fixture.repo)
			if _, err := app.Status(context.Background(), StatusRequest{RepoRoot: fixture.repo}); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Status() error = %v, want %q", err, tt.want)
			}
			if err := app.Render(context.Background(), RenderRequest{RepoRoot: fixture.repo}); err == nil {
				t.Fatal("Render() accepted forged runtime report")
			}
			assertRuntimeGenerationUnchanged(t, fixture.repo, before)
		})
	}
}
```

Add the deterministic setup and typed rewrite helper in the same test file:

```go
func newRuntimeTestApp(t *testing.T) App {
	t.Helper()
	return App{
		Git: changes.ExecRunner{}, GateExecutor: &appGateExecutor{},
		RunIDs: &sequenceRunIDs{ids: []string{"20260818T010000.000000000Z-0123456789abcdef"}},
		Clock: staticAppClock{now: time.Date(2026, 8, 18, 1, 0, 0, 0, time.UTC)},
	}
}

func writeValidRuntimeReport(t *testing.T, app App, fixture appRepo) string {
	t.Helper()
	report, err := app.CI(context.Background(), CIRequest{
		RepoRoot: fixture.repo, Base: fixture.head, Head: fixture.head, All: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return report.RunID
}

func rewriteRuntimeReport(t *testing.T, fixture appRepo, runID string, mutate func(*runner.RunReport)) {
	t.Helper()
	path := filepath.Join(fixture.repo, ".graph", "runtime", "runs", runID, "run.json")
	var report runner.RunReport
	decodeAppJSONFile(t, path, &report)
	mutate(&report)
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
```

Implement the semantic helpers with `rewriteRuntimeReport`: empty commands assigns `[]evidence.CommandRecord{}`; duplicate command appends `report.Loops[0].Commands[0]`; wrong argv assigns `[]string{"forged"}`; aggregate mismatch sets `report.Result=evidence.GateFail` while the Loop remains PASS. Unknown-field rewrites the top-level JSON through `map[string]any` with `document["unexpected"]=true`; trailing-document appends `{}\n`; oversized report writes `bytes.Repeat([]byte{'x'}, (16<<20)+1)`; the file-symlink case removes `run.json` and links it to `src/subject.txt`; the run-directory case moves the completed directory to an ordinary sibling and replaces the original run-ID path with a symlink; the runs-root case moves `.graph/runtime/runs` to an ordinary sibling and replaces `runs` with a relative symlink. The invalid-ID case copies the valid report into `.graph/runtime/runs/zz-invalid/run.json` so it sorts newer and cannot be skipped.

`snapshotRuntimeGeneration` and `assertRuntimeGenerationUnchanged` read `index.html`, `graph-status.json`, and `latest-run.json` into `map[string][]byte` and byte-compare them after failed `Render`.

- [ ] **Step 2: Verify RED**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl -run TestLoadLatestRuntimeFailsClosed -count=1
```

Expected: at least the `loadLatestLoopRuns` cases for invalid ID, oversized input, and forged PASS fail because it currently uses `os.ReadFile` and trusts weak record semantics.

- [ ] **Step 3: Move all runtime reads behind one loader**

`runtime_reports.go` owns:

```go
const maxRunReportBytes int64 = 16 << 20

type runtimeReportSet struct {
	Latest  *runner.RunReport
	ByLoop  map[string]evidence.RuntimeEvidence
}

func loadRuntimeReports(
	ctx context.Context,
	repoRoot string,
	reg *registry.Registry,
	dag *graph.DAG,
) (runtimeReportSet, error)

func loadExactRunReport(
	ctx context.Context,
	repoRoot string,
	runID string,
	reg *registry.Registry,
	dag *graph.DAG,
) (runner.RunReport, error)

func validateRunReportStructure(reg *registry.Registry, dag *graph.DAG, report runner.RunReport) error
```

Delete `loadLatestLoopRuns`, `loadLatestRunReport`, `loadExactRunReport`, and `decodeRunReport` from `app.go`. `Status`, `Render`, and `Export` must call the unified loader; no direct `os.ReadFile` of `.graph/runtime/runs/**/run.json` remains outside it.

`loadRuntimeReports` obtains run-directory names only through the no-follow directory helper, sorts completed run IDs descending, and validates reports in that order. The newest completed report becomes `Latest`; the scan continues only until `ByLoop` has the first record for every executable Registry Loop or no older completed report remains. An ordinary directory without `run.json` is an incomplete publication and is ignored. A symlink/non-directory entry, or an invalid run-ID directory that contains `run.json`, is an error. Any malformed report encountered while it is still needed is authoritative failure and cannot be skipped to obtain an older PASS.

- [ ] **Step 4: Implement no-follow bounded file opening per platform**

Expose these package-private functions from both platform files:

```go
func listRuntimeRunDirectoriesNoFollow(repoRoot string) ([]string, error)
func readRuntimeRegularFileNoFollow(repoRoot, relative string, limit int64) ([]byte, error)
```

Use `//go:build linux` in `runtime_open_linux.go` and `//go:build !linux` in `runtime_open_other.go`. On Linux, open the repository root once, then walk every relative component with `unix.Openat`: intermediate components use `O_RDONLY|O_DIRECTORY|O_CLOEXEC|O_NOFOLLOW`, and the final file uses `O_RDONLY|O_CLOEXEC|O_NOFOLLOW`. `listRuntimeRunDirectoriesNoFollow` opens `.graph/runtime/runs` this way, reads entries from that descriptor, and rejects symlink/non-directory entries before returning names; `readRuntimeRegularFileNoFollow` wraps only the final descriptor with `os.NewFile`, confirms regular type, and reads through `io.LimitReader(limit+1)`. Close every directory descriptor deterministically. The fallback uses component-wise `Lstat`, `os.Open`, `Stat`, and `os.SameFile` for both directory listing and file reading. Both implementations reject symlink components, non-directories, non-regular final files, identity changes, and payloads over the limit, including an empty symlinked runs root that contains no report to trigger a later file open.

- [ ] **Step 5: Validate aggregate and per-Loop semantics**

`validateRunReportStructure` must enforce schema/enforcement/run ID/timestamps/duration; unique Loop IDs; exact `Impact.LoopIDs` versus report Loops in DAG topological order; each record's `RunID` and `SubjectCommit` matching aggregate `RunID` and `Head`; known non-planned Loops only; `BLOCKED` only after an earlier selected dependency did not pass; and aggregate `PASS` iff all Loop results are `PASS` (empty affected run is allowed and remains diagnostic only). The loader must additionally prove `Head` and every matching `SubjectCommit` name a real commit object before classifying a proof as merely stale; a syntactically valid but nonexistent commit is malformed runtime, not historical evidence.

Add to `proof.go`:

```go
func ValidateRuntimeGateShape(run RuntimeEvidence) error

type RuntimeGateProofResult struct {
	Status       GateStatus
	StaleReasons []string
}

func ValidateRuntimeGateProof(contract GateContract, run RuntimeEvidence) (RuntimeGateProofResult, error)
func runtimeIdentityMismatchReasons(contract GateContract, run RuntimeEvidence) []string
```

`ValidateRuntimeGateShape` is Registry-independent. It validates required scalar/digest syntax, non-empty and unique Gate IDs, equal Validator/command Gate-ID sets for executable `PASS/FAIL` results, canonical run-relative log metadata/path, canonical artifact records, non-negative sizes/durations, and result derivation. `PASS` requires a non-empty command set and every command to be a clean PASS; `FAIL` requires a non-empty command set, at least one failed command, and `canonicalEligible=false`; `BLOCKED` keeps the runner's precomputed Validator identities but requires no commands/artifacts and `canonicalEligible=false`, while its dependency justification is checked by the aggregate validator. No completed report may contain `NOT_RUN` or `STALE` Loop results.

`ValidateRuntimeGateProof` first reuses the shape checker. It then compares Loop ID, real subject commit, scope, Registry, and dependency digests with `GateContract`. If `runtimeIdentityMismatchReasons` returns a non-empty sorted slice, return `RuntimeGateProofResult{Status: GateStale, StaleReasons: reasons}, nil` without claiming the old command contract is valid. If those identities match the current contract, enforce the exact required Gate/Validator set, Validator identity/digest, command argv/workdir, missing/duplicate/extra entries, and declared artifact set directly from `GateContract`; any mismatch is a hard error because a current-identity record cannot be explained as historical drift. A valid current proof returns its derived `PASS`, `FAIL`, or `BLOCKED` status and `[]string{}`.

After structural loading, `App.Status` builds the current contract for each Loop at current `HEAD` and current approved dependency digests. This ordering keeps an honest older report visible as `GateStale` while still rejecting empty/duplicate commands and a forged current PASS. Runtime log/artifact contents are never opened merely to grant approval—runtime is diagnostic only—but every referenced path must be canonical and contained below its exact run/Loop/Gate directory. `App.Export` uses `ValidateExportableRuntimeGateProof`, which accepts no stale result and performs the strict current-contract plus runtime-file checks needed for canonical export.

- [ ] **Step 6: Fail closed before publishing a dashboard generation**

The newest directory containing `run.json` is authoritative. If its run ID, path, bytes, JSON, or semantics are invalid, return an error; do not skip to an older PASS. `Status` and `Render` must fail before calling `render.Render`, so the previous atomic generation remains unchanged. A valid but old digest remains visible only as `GateStale`; it is not a structural loader error.

- [ ] **Step 7: Run finding-4 tests, including race checks**

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl -run 'Test(LoadLatestRuntime|Render)' -count=1
GOWORK=off go test ./internal/graphctl/evidence -run TestRuntimeGateProof -count=1
GOWORK=off go test -race ./internal/graphctl ./internal/graphctl/evidence
```

Expected: PASS. The symlink and file-replacement cases must remain deterministic under `-race`.

- [ ] **Step 8: Commit checkpoint after explicit authorization**

```bash
git add \
  backend/tools/internal/graphctl/runtime_reports.go \
  backend/tools/internal/graphctl/runtime_reports_test.go \
  backend/tools/internal/graphctl/runtime_open_linux.go \
  backend/tools/internal/graphctl/runtime_open_other.go \
  backend/tools/internal/graphctl/app.go \
  backend/tools/internal/graphctl/app_test.go \
  backend/tools/internal/graphctl/evidence/proof.go \
  backend/tools/internal/graphctl/evidence/proof_test.go
git commit -m "fix(graph): validate untrusted runtime reports"
```

Never stage `backend/go.work.sum`.

### Task 22: Prove all entry points enforce the same trust boundary

**Files:**
- Modify: `backend/tools/internal/graphctl/app_test.go`
- Modify: `backend/tools/internal/graphctl/evidence/export_test.go`
- Modify: `backend/tools/internal/graphctl/evidence/proof_test.go`
- Modify: `backend/tools/README.md`

- [ ] **Step 1: Add a cross-path regression matrix**

Lock the intentional distinction between malformed runtime and honest historical runtime while proving neither can become approval authority:

```go
type proofMutation func(*evidence.RuntimeEvidence)

func TestUntrustedProofCannotBecomeApprovalAuthority(t *testing.T) {
	missingCommand := proofMutation(func(run *evidence.RuntimeEvidence) {
		run.Commands = nil
	})
	t.Run("missing command is malformed", func(t *testing.T) {
		assertCommittedCandidateRejected(t, missingCommand, "required command set")
		assertRuntimeRenderRejectedWithoutPublish(t, missingCommand, "required command set")
		assertRuntimeExportRejectedWithoutTarget(t, missingCommand, "required command set")
	})

	staleScope := proofMutation(func(run *evidence.RuntimeEvidence) {
		run.ScopeDigest = hashAppBytes([]byte("forged scope"))
	})
	t.Run("scope mismatch is stale but never pass", func(t *testing.T) {
		assertCommittedCandidateRejected(t, staleScope, "scopeDigest")
		assertRuntimeRenderPublishesStale(t, staleScope, "scopeDigest")
		assertRuntimeExportRejectedWithoutTarget(t, staleScope, "scopeDigest")
	})
}
```

Each assertion helper creates its own `newAppRepo` so committing a canonical bundle cannot make the runtime-export subject stale for an unrelated reason. Give the helpers these exact contracts:

```go
func assertCommittedCandidateRejected(t *testing.T, mutate proofMutation, want string)
func assertRuntimeRenderRejectedWithoutPublish(t *testing.T, mutate proofMutation, want string)
func assertRuntimeRenderPublishesStale(t *testing.T, mutate proofMutation, want string)
func assertRuntimeExportRejectedWithoutTarget(t *testing.T, mutate proofMutation, want string)
```

`assertCommittedCandidateRejected` starts from an actual exported bundle, applies the mutation to `canonical.RuntimeEvidence`, commits only the rewritten `evidence.json`, and requires the candidate to appear only in `Rejections`, with no selected canonical ref and no `READY_FOR_APPROVAL`/`APPROVED`. `assertRuntimeRenderRejectedWithoutPublish` snapshots `index.html`, `graph-status.json`, and `latest-run.json`, rewrites the latest ignored `run.json`, requires the expected hard error from both `Status` and `Render`, and byte-compares the generation. `assertRuntimeRenderPublishesStale` requires `Status` and the newly rendered `graph-status.json` to expose `GateStale`, include the requested stale reason, and never show `READY_FOR_APPROVAL` or `APPROVED`. `assertRuntimeExportRejectedWithoutTarget` rewrites an otherwise exportable runtime record, calls `App.Export`, and proves neither the requested target nor an owned temporary sibling exists.

- [ ] **Step 2: Update operator documentation**

Document in `backend/tools/README.md`:

- runtime reports are untrusted, bounded diagnostics and are never approval authority;
- runtime hardening prevents accidental/ordinary local forgery but is not a signature or same-OS-user cryptographic trust boundary;
- canonical Evidence is re-proved from Registry plus `subjectCommit`, not trusted because it is committed;
- `registryDigest` and `approvalDigest` use canonical JSON and old values intentionally become stale;
- a closed Loop requires a real completion subject and exact Evidence/Review binding;
- local dirty `backend/go.work.sum` is reported truthfully and must not be hidden to obtain canonical eligibility.

- [ ] **Step 3: Run the complete local verification matrix**

```bash
cd backend/tools
GOWORK=off go test ./... -count=1
GOWORK=off go test ./cmd/graphctl ./internal/graphctl/... -count=1
GOWORK=off go test -race ./cmd/graphctl ./internal/graphctl/...
GOWORK=off go vet ./cmd/graphctl ./internal/graphctl/...
GOWORK=off go build ./cmd/graphctl
GOWORK=off go run ./cmd/graphctl validate --repo-root ../..
GOWORK=off go run ./cmd/graphctl scope-check --repo-root ../..
bash -n ../../.githooks/pre-push
```

Expected: every command exits 0. `scope-check` may still report the user-owned `backend/go.work.sum` change as affecting L0; that is truthful and is not bypassed.

- [ ] **Step 4: Inspect exact repository boundaries**

```bash
git status --short
git diff --check
git -C /home/wangjunbo/workspace-iot-platform/things status --short
```

Expected: only planned G0 files plus the pre-existing, unstaged `backend/go.work.sum` appear; `things` is byte-for-byte untouched relative to its recorded pre-task state.

- [ ] **Step 5: Commit checkpoint after explicit authorization**

```bash
git add \
  backend/tools/internal/graphctl/app_test.go \
  backend/tools/internal/graphctl/evidence/export_test.go \
  backend/tools/internal/graphctl/evidence/proof_test.go \
  backend/tools/README.md
git commit -m "test(graph): lock proof trust boundaries"
```

Never stage `backend/go.work.sum`.

### Task 23: Regenerate Evidence, obtain fresh Reviews, and request closure

**Files:**
- Create from the exact machine run ID: `docs/superpowers/evidence/L0-things-baseline/$L0_RUN_ID/**`
- Create from the exact machine run ID: `docs/superpowers/evidence/G0-rd-graph-control-plane/$G0_RUN_ID/**`
- Create from the immutable reviewer ID: `docs/superpowers/reviews/L0-things-baseline/$L0_REVIEW_ID/review.json`
- Create from the immutable reviewer ID: `docs/superpowers/reviews/G0-rd-graph-control-plane/$G0_REVIEW_ID/review.json`
- Modify only after separate lifecycle authorization: `.graph/loops/L0-things-baseline.yaml`
- Modify only after separate lifecycle authorization: `.graph/loops/G0-rd-graph-control-plane.yaml`

- [ ] **Step 1: Stop at the `backend/go.work.sum` truth boundary**

```bash
git status --short
git diff -- backend/go.work.sum
```

If `backend/go.work.sum` is modified, do not export Evidence. Present its exact diff and ask the user to choose a recoverable resolution: explicitly restore only that file, or intentionally include its change in a separately reviewed L0 change. Never use `.gitignore`, `assume-unchanged`, `skip-worktree`, stash tricks, or environment filtering to make the tree appear clean.

- [ ] **Step 2: Run clean full CI after the user resolves the file**

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl ci --repo-root ../.. --all --format=json
```

Expected: G0 and L0 `PASS`, both `canonicalEligible=true`. Record the exact run ID; do not infer it from directory sorting.

- [ ] **Step 3: Export and commit fresh L0 Evidence after authorization**

```bash
GOWORK=off go run ./cmd/graphctl evidence export \
  --repo-root ../.. \
  --loop L0-things-baseline \
  --run "$L0_RUN_ID" \
  --output "docs/superpowers/evidence/L0-things-baseline/$L0_RUN_ID" \
  --format=json
```

Inspect hashes and bundle contents, then request commit authorization. Commit only the new L0 bundle with `test(graph): record hardened L0 evidence`.

- [ ] **Step 4: Rerun clean CI, export G0 Evidence, and commit after authorization**

Run full `graphctl ci --all` again because the L0 Evidence commit changed HEAD. Export the G0 bundle using the second exact run ID, inspect it, and commit only that bundle with `test(graph): record hardened G0 evidence`.

- [ ] **Step 5: Obtain two independent fresh Reviews using Sol-managed Luna fleets**

Start two independent Sol coordinators, one for L0 and one for G0. Each Sol may dispatch Luna agents for disjoint checklists, but must return one strict Review verdict bound to the exact current spec, Registry, Evidence refs, scope digest, Registry digest, and dependency digests. Apply a fixed timebox and close all descendants after the verdict.

The G0 Review must explicitly close or reopen `G0-REV-001` through `G0-REV-004`. The L0 Review must carry forward the prior PASS conclusion, verify `thingsbaseline` was not weakened, and confirm `things/` remained read-only. Review candidates are not authoritative until schema-validated and committed after explicit authorization.

- [ ] **Step 6: Recompute status and present completion inputs**

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl status --repo-root ../.. --format=json
GOWORK=off go run ./cmd/graphctl context --repo-root ../.. --loop L0-things-baseline --format=json
GOWORK=off go run ./cmd/graphctl context --repo-root ../.. --loop G0-rd-graph-control-plane --format=json
```

Expected before lifecycle closure: both Loops are `READY_FOR_APPROVAL`, both Reviews are `PASS`, and all four original G0 findings are closed. If not, return to the owning task; never edit completion to force status.

- [ ] **Step 7: Request lifecycle authorization separately**

Present exact subject commit, scope/Registry/dependency digests, canonical Evidence refs/hashes, Review refs/hashes, and proposed completion records. Do not modify lifecycle or completion until the user explicitly approves closing each Loop.

- [ ] **Step 8: Verify approved state and commit only after authorization**

After the approved completion patch:

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl validate --repo-root ../..
GOWORK=off go run ./cmd/graphctl status --repo-root ../.. --format=json
GOWORK=off go run ./cmd/graphctl render --repo-root ../..
```

Expected: both Loops are `APPROVED`; generated dashboard state is truthful; changing completion metadata did not alter the digest preimage. Commit Loop manifests only after a final explicit authorization. Never push without a separate request.

## Spec coverage map

| Approved design area | Implementation tasks |
| --- | --- |
| Registry layout, schema, path/security rules | 1-2, 13 |
| AI protocol and repository memory | 14 |
| Deterministic DAG and downstream impact | 3-4 |
| Manual/pre-push Git ranges and exact-commit proof | 5, 12, 14 |
| Machine Gate semantics and process cleanup | 8-9 |
| Scope/Registry/Validator/dependency digests | 6 |
| Evidence, Review, lifecycle, L0 legacy boundary | 7, 10, 16 |
| Thin CLI and stable JSON | 12 |
| L0 thingsbaseline reuse | 13, 15-16 |
| Generated runtime and dashboard | 11, 15, 17 |
| LOCAL_GATE Hook | 14, 17 |
| Canonical checkout/systemd cutover | 17 |
| No change to things/go.work.sum, no auto commit/push | all tasks, especially 15-17 |
| Review finding 1: committed Evidence Gate re-proof | 18, 22-23 |
| Review finding 2: recursive canonical JSON digests | 19, 22-23 |
| Review finding 3: real completion subject binding | 20, 22-23 |
| Review finding 4: untrusted bounded runtime loader | 21-23 |
| Sol-managed Luna execution with disjoint writes and timeboxes | execution constraints, 18-23 |

## Plan completion gate

The implementation plan is complete only when:

- every checkbox has either execution evidence or an explicit user-approved deferral;
- all tests and real G0/L0 Gates pass;
- both current Reviews pass with no open findings;
- the user explicitly closes lifecycle;
- canonical runtime is served at `127.0.0.1:18081`;
- no claim relies on the legacy hand-written page;
- no commit or push occurred without explicit authorization.
