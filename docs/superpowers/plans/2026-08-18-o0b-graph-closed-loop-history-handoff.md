# O0b Closed Loop History and Ownership Handoff Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a closed Loop an immutable historical proof while requiring every current semantic file it leaves behind to have exactly one active successor owner.

**Architecture:** Status calculation branches by lifecycle. Active and planned Loops retain the current-HEAD path; closed Loops reconstruct their exact contract from `completion.subjectCommit`, validate only the completion-sealed Evidence/Review, and receive current owner handoff metadata from the scope resolver. The same historical projector supplies downstream approval digests during Evidence export, so status and export cannot disagree.

**Tech Stack:** Go 1.24, Git object plumbing (`ls-tree`, `cat-file`, `rev-parse`), YAML/JSON Schema Registry validation, existing graphctl Evidence/Review digest contracts, Go `testing`, embedded HTML templates.

---

## Implementation boundaries

This plan implements only the approved O0b design:

- no `finalize`, `prepare` or `approve` command;
- no real G0/L0 lifecycle or completion change;
- no generation or repair of canonical G0/L0 Evidence/Review;
- no FAST/FULL queue, worker or cache;
- no mutation under `/home/wangjunbo/workspace-iot-platform/things`;
- no intentional execution of the slow L0 migration Gate;
- no `go work sync`;
- no `git add`, commit or push without a fresh explicit user authorization.

The O0b milestone review is recorded as Markdown, not as canonical `review.json`. It proves that an independent reviewer checked this delta, but it does not claim that G0 is complete.

## File and responsibility map

| File | Responsibility after O0b |
|---|---|
| `backend/tools/internal/graphctl/registry/load.go` | Shared Registry decoding and validation pipeline |
| `backend/tools/internal/graphctl/registry/load_commit.go` | Read canonical Registry files from one immutable Git commit |
| `backend/tools/internal/graphctl/registry/load_commit_test.go` | Commit-scoped Registry safety and immutability tests |
| `backend/tools/internal/graphctl/registry/validate.go` | Source-aware Registry semantic, path and artifact validation |
| `backend/tools/internal/graphctl/scope/handoff.go` | Resolve and validate closed-to-active file ownership handoff |
| `backend/tools/internal/graphctl/scope/handoff_test.go` | Success, missing and ambiguous handoff tests |
| `backend/tools/internal/graphctl/review/exact.go` | Load and hash-check one exact committed Review or legacy Review ref |
| `backend/tools/internal/graphctl/review/review_test.go` | Exact Review ref regression tests |
| `backend/tools/internal/graphctl/evidence/historical.go` | Reconstruct one closed Loop's immutable proof and approval digest |
| `backend/tools/internal/graphctl/evidence/historical_test.go` | Historical stability and corruption fixture tests |
| `backend/tools/internal/graphctl/evidence/status.go` | `CURRENT`/`HISTORICAL` status model and pure historical status projection |
| `backend/tools/internal/graphctl/evidence/evidence_test.go` | Status precedence and proof-basis unit tests |
| `backend/tools/internal/graphctl/evidence/export.go` | Reuse historical approvals for closed dependencies |
| `backend/tools/internal/graphctl/evidence/export_test.go` | Export dependency regression tests |
| `backend/tools/internal/graphctl/app.go` | Lifecycle branch, handoff integration, and dashboard model mapping |
| `backend/tools/internal/graphctl/app_test.go` | End-to-end historical status and `validate` handoff behavior |
| `backend/tools/internal/graphctl/render/model.go` | Historical fields in the machine dashboard document |
| `backend/tools/internal/graphctl/render/render.go` | Historical view validation and display label helper |
| `backend/tools/internal/graphctl/render/index.html.tmpl` | Visually distinguish historical proof from current alerts |
| `backend/tools/internal/graphctl/render/render_test.go` | JSON/HTML historical rendering tests |
| `backend/tools/cmd/graphctl/main.go` | Human status output and typed handoff error codes |
| `backend/tools/cmd/graphctl/main_test.go` | CLI output/error-code tests |
| `backend/tools/README.md` | Operator explanation of current versus historical proof |

## Public contracts introduced by this plan

Use these exact names throughout all tasks:

```go
// registry
func LoadAtCommit(ctx context.Context, repoRoot, commit string) (*Registry, error)

// scope
type ClosedHandoff struct {
	LoopID       string
	OwnerLoopIDs []string
}

type ErrClosedHandoffMissing struct {
	LoopID string
	Path   string
}

type ErrClosedHandoffAmbiguous struct {
	LoopID       string
	Path         string
	OwnerLoopIDs []string
}

func ResolveClosedHandoff(reg *registry.Registry, loop *registry.Loop, tracked []string) (ClosedHandoff, error)
func ValidateClosedHandoffs(reg *registry.Registry, tracked []string) error

// review
func LoadCommittedRef(ctx context.Context, repoRoot string, ref FileRef) (Candidate, error)
func VerifyCommittedRefs(ctx context.Context, repoRoot string, refs []FileRef) error

// evidence
type ProofBasis string

const (
	ProofBasisCurrent    ProofBasis = "CURRENT"
	ProofBasisHistorical ProofBasis = "HISTORICAL"
)

type HistoricalProjection struct {
	SubjectCommit     string
	ScopeDigest       string
	RegistryDigest    string
	ValidatorDigests  map[string]string
	DependencyDigests map[string]string
	EvidenceRefs      []registry.ArtifactRef
	ReviewRef         *registry.ArtifactRef
	ApprovalDigest    string
}

func ProjectClosedHistory(ctx context.Context, repoRoot string, currentLoop *registry.Loop) (HistoricalProjection, error)
```

`LoopStatus` gains these JSON fields:

```go
ProofBasis         ProofBasis `json:"proofBasis"`
ProofSubjectCommit string     `json:"proofSubjectCommit,omitempty"`
HandoffLoopIDs     []string   `json:"handoffLoopIds"`
```

Active and planned Loops always emit `proofBasis=CURRENT`, an empty `handoffLoopIds` array and no `proofSubjectCommit`. Closed Loops always emit `proofBasis=HISTORICAL` even when their proof is corrupt.

### Task 1: Load a historical Registry directly from Git objects

**Files:**

- Create: `backend/tools/internal/graphctl/registry/load_commit.go`
- Create: `backend/tools/internal/graphctl/registry/load_commit_test.go`
- Modify: `backend/tools/internal/graphctl/registry/load.go`
- Modify: `backend/tools/internal/graphctl/registry/validate.go`

- [x] **Step 1: Write failing commit-isolation tests**

Create a real temporary Git repository from the existing Registry fixture, commit version A, then change the worktree to version B. Add tests with these assertions:

```go
func TestLoadAtCommitUsesOnlyRequestedGitTree(t *testing.T) {
	repo, commitA := commitRegistryFixture(t, validLoopYAML())
	replaceFile(t, filepath.Join(repo, ".graph/loops/G0-rd-graph-control-plane.yaml"), "title: Graph control", "title: Worktree only")

	reg, err := LoadAtCommit(context.Background(), repo, commitA)
	if err != nil {
		t.Fatalf("LoadAtCommit() error = %v", err)
	}
	if got := reg.ByID["G0-rd-graph-control-plane"].Metadata.Title; got != "Graph control" {
		t.Fatalf("historical title = %q, want Graph control", got)
	}
	if content, err := os.ReadFile(filepath.Join(repo, ".graph/loops/G0-rd-graph-control-plane.yaml")); err != nil || !bytes.Contains(content, []byte("Worktree only")) {
		t.Fatalf("LoadAtCommit mutated worktree: content=%q err=%v", content, err)
	}
}

func TestLoadAtCommitRejectsUnavailableCommitAndNonRegularCanonicalBlob(t *testing.T) {
	repo, _ := commitRegistryFixture(t, validLoopYAML())
	if _, err := LoadAtCommit(context.Background(), repo, strings.Repeat("f", 40)); err == nil || !strings.Contains(err.Error(), "commit") {
		t.Fatalf("missing commit error = %v", err)
	}

	runGitRegistry(t, repo, "rm", ".graph/registry.yaml")
	if err := os.Symlink("loops/G0-rd-graph-control-plane.yaml", filepath.Join(repo, ".graph/registry.yaml")); err != nil {
		t.Fatal(err)
	}
	commit := commitAllRegistry(t, repo, "symlink registry")
	if _, err := LoadAtCommit(context.Background(), repo, commit); err == nil || !strings.Contains(err.Error(), "regular blob") {
		t.Fatalf("symlink Registry error = %v", err)
	}
}
```

Also table-test unknown YAML fields, a second YAML document, malformed Schema JSON, duplicate Loop IDs and a canonical document larger than the existing Registry limits. These cases must fail with the same validation vocabulary as `Load`.

- [x] **Step 2: Run the new tests and observe the intended RED state**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/registry -run 'TestLoadAtCommit' -count=1
```

Expected: compile failure because `LoadAtCommit` and the test fixture helpers do not exist.

- [x] **Step 3: Extract one shared decode/validate pipeline**

Refactor `Load` so filesystem and Git readers feed the same private source contract:

```go
type registrySource interface {
	Read(path string, maximum int64) ([]byte, error)
	LoopManifests() ([]string, error)
}

func loadFromSource(repoRoot string, source registrySource) (*Registry, error) {
	entryBytes, err := source.Read(registryPath, maxYAMLDocumentBytes)
	if err != nil {
		return nil, fmt.Errorf("Registry entry %s: %w", registryPath, err)
	}
	entry, rawEntry, err := decodeStrictBytes[RegistryFile](registryPath, entryBytes)
	if err != nil {
		return nil, err
	}
	if err := validateRegistryEntry(entry); err != nil {
		return nil, fmt.Errorf("validate %s: %w", registryPath, err)
	}

	schemaBytes, err := source.Read(canonicalSchema, maxYAMLDocumentBytes)
	if err != nil {
		return nil, fmt.Errorf("schema %s: %w", canonicalSchema, err)
	}
	schema, err := compileCanonicalSchema(schemaBytes)
	if err != nil {
		return nil, err
	}
	return decodeRegistryLoops(repoRoot, source, schema, entry, rawEntry)
}
```

Keep `Load`'s existing physical containment and symlink checks in its filesystem source. Do not relax any existing Registry test to make the refactor pass.

The selected source must remain authoritative throughout validation. Project path/type checks and referenced artifact hashing use that source; historical validation must never fall back to current worktree files. Mutable external workspace siblings are checked physically only by filesystem `Load`; historical loading validates their committed declaration grammar and read-only access contract without dereferencing the current sibling.

- [x] **Step 4: Implement the immutable Git source**

`LoadAtCommit` must first resolve `commit^{commit}`, then build a path-indexed snapshot from one NUL-delimited `git ls-tree` call. Accept only modes `100644` and `100755`, read content with `git cat-file blob` plus the parsed tree entry's object ID, and enforce the caller's byte limit before decoding:

```go
func LoadAtCommit(ctx context.Context, repoRoot, commit string) (*Registry, error) {
	if ctx == nil {
		return nil, fmt.Errorf("registry: context is required")
	}
	root, err := resolveRepositoryRoot(repoRoot)
	if err != nil {
		return nil, err
	}
	source, err := newGitRegistrySource(ctx, root, commit)
	if err != nil {
		return nil, err
	}
	return loadFromSource(root, source)
}
```

Sort Loop manifest paths bytewise before decoding. The loader must not call `git checkout`, create a worktree or write repository files.

All Git plumbing must use a read-only runner that removes inherited `GIT_*` variables, disables replacement objects, lazy fetches and optional locks, and isolates global/system Git configuration. Tests cover hostile Git environment redirection, replacement objects and an unavailable promised blob.

The implementation also bounds streamed Git input before accumulation: scalar output 4 KiB, diagnostics 64 KiB, tree output 32 MiB, one tree record 16 KiB, 100,000 tree entries and one canonical Evidence/Review artifact 8 MiB. Tree parsing and artifact hashing are streaming; limit failures kill and reap Git.

- [x] **Step 5: Run all Registry tests**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/registry -count=1
```

Expected: PASS, including every pre-O0b `Load` security test.

- [x] **Step 6: Prepare the Task 1 commit checkpoint**

Run `git status --short` and show the exact four Task 1 implementation files. Commit only after explicit authorization:

```bash
git add backend/tools/internal/graphctl/registry/load.go backend/tools/internal/graphctl/registry/load_commit.go backend/tools/internal/graphctl/registry/load_commit_test.go backend/tools/internal/graphctl/registry/validate.go
git commit -m "feat(graph): load registry contracts from git commits"
```

### Task 2: Enforce closed-to-active ownership handoff

**Files:**

- Create: `backend/tools/internal/graphctl/scope/handoff.go`
- Create: `backend/tools/internal/graphctl/scope/handoff_test.go`
- Modify: `backend/tools/internal/graphctl/scope/impact.go`
- Modify: `backend/tools/internal/graphctl/registry/validate.go`
- Modify: `backend/tools/internal/graphctl/app.go`
- Modify: `backend/tools/internal/graphctl/app_test.go`
- Modify: `backend/tools/cmd/graphctl/main.go`
- Modify: `backend/tools/cmd/graphctl/main_test.go`

- [x] **Step 1: Write pure resolver tests**

Construct one closed Loop whose `evidence.subjectPaths` selects `src/a.go` and `src/b.go`. Cover these exact outcomes:

```go
func TestResolveClosedHandoffAggregatesUniqueActiveOwners(t *testing.T) {
	closed := handoffLoop("OLD", registry.LifecycleClosed, []string{"legacy/**"}, []string{"src/**"})
	a := handoffLoop("A", registry.LifecycleActive, []string{"src/a.go"}, []string{"src/a.go"})
	b := handoffLoop("B", registry.LifecycleActive, []string{"src/b.go"}, []string{"src/b.go"})
	reg := handoffRegistry(t, closed, b, a)

	got, err := ResolveClosedHandoff(reg, closed, []string{"docs/history.md", "src/b.go", "src/a.go"})
	if err != nil {
		t.Fatalf("ResolveClosedHandoff() error = %v", err)
	}
	if want := (ClosedHandoff{LoopID: "OLD", OwnerLoopIDs: []string{"A", "B"}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("handoff = %#v, want %#v", got, want)
	}
}
```

Add missing-owner and two-active-owner tests using `errors.As` with `ErrClosedHandoffMissing` and `ErrClosedHandoffAmbiguous`. Add regressions proving planned/closed owners are ignored, `scope.exclude` is honored, duplicate tracked paths are normalized, and historical files outside `subjectPaths` do not require a successor.

- [x] **Step 2: Verify the handoff tests fail**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/scope -run 'TestResolveClosedHandoff|TestValidateClosedHandoffs' -count=1
```

Expected: compile failure for the new types/functions.

- [x] **Step 3: Implement deterministic typed handoff resolution**

Reuse `normalizedChangedPaths`, `prepareRegistry`, `scopeMatches` and `scopeOwns`; do not introduce a second glob language. Error strings must begin with their stable machine code:

```go
func (err *ErrClosedHandoffMissing) Error() string {
	return fmt.Sprintf("CLOSED_HANDOFF_MISSING: closed Loop %q path %q has no active successor owner", err.LoopID, err.Path)
}

func (err *ErrClosedHandoffAmbiguous) Error() string {
	return fmt.Sprintf("CLOSED_HANDOFF_AMBIGUOUS: closed Loop %q path %q has active successor owners: %s", err.LoopID, err.Path, strings.Join(err.OwnerLoopIDs, ", "))
}
```

Walk normalized tracked paths in byte order. For each path selected by the closed Loop's `evidence.subjectPaths`, collect only active owners and require exactly one. Return sorted unique owner IDs. An empty current selected-file set is a valid empty handoff.

The public resolver must also remain fail-closed for caller-built Registry values. Reuse a source-free Registry validator for `evidence.subjectPaths` so required/non-empty, 256-item, normalized-uniqueness, UTF-8 and pattern-size constraints are enforced before copying or sorting. Oversized patterns are rejected by length before scanning, normalization or quoted error formatting.

Every ownership/watch glob evaluation must consume the supplied matcher budget; no unbudgeted preflight is allowed. `ValidateClosedHandoffs` retains one base budget shared by all closed Loops. `Affected`, which evaluates both ownership and watches, receives two base budgets to preserve its documented practical workload while still bounding all matching work.

- [x] **Step 4: Add the fail-closed Registry validation hook**

In `App.Validate`, validate closed handoffs before the generic tracked-ownership check:

```go
if err := scope.ValidateClosedHandoffs(reg, tracked); err != nil {
	return ValidateResult{}, err
}
if err := scope.ValidateTrackedOwnership(reg, tracked); err != nil {
	return ValidateResult{}, err
}
```

This order is required because two active owners for a historical subject are specifically a `CLOSED_HANDOFF_AMBIGUOUS` failure; unrelated ambiguous tracked paths must continue to return the existing `ErrAmbiguousOwner`. Add real-repository app tests showing `Validate` accepts one successor and returns the typed missing/ambiguous errors for zero/two successors, plus a regression preserving the generic error for a path outside every closed Loop's `evidence.subjectPaths`.

- [x] **Step 5: Expose stable JSON CLI error codes**

Extend `writeOperationalFailure` with `errors.As` branches for the two new errors. Add table rows expecting `CLOSED_HANDOFF_MISSING` and `CLOSED_HANDOFF_AMBIGUOUS`; keep existing `UNOWNED_PATH` and `AMBIGUOUS_OWNER` unchanged.

- [x] **Step 6: Run resolver, app and CLI tests**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/scope ./internal/graphctl -run 'Handoff|ValidateRejectsClosed|ValidateAcceptsClosed' -count=1
GOWORK=off go test ./cmd/graphctl -run 'Operational|Handoff' -count=1
```

Expected: PASS.

- [x] **Step 7: Prepare the Task 2 commit checkpoint**

Show status first; after explicit authorization only:

```bash
git add backend/tools/internal/graphctl/scope/handoff.go backend/tools/internal/graphctl/scope/handoff_test.go backend/tools/internal/graphctl/scope/impact.go backend/tools/internal/graphctl/registry/validate.go backend/tools/internal/graphctl/app.go backend/tools/internal/graphctl/app_test.go backend/tools/cmd/graphctl/main.go backend/tools/cmd/graphctl/main_test.go
git commit -m "feat(graph): validate closed ownership handoffs"
```

### Task 3: Reconstruct and validate an exact historical proof

**Files:**

- Create: `backend/tools/internal/graphctl/review/exact.go`
- Modify: `backend/tools/internal/graphctl/review/review_test.go`
- Create: `backend/tools/internal/graphctl/evidence/historical.go`
- Create: `backend/tools/internal/graphctl/evidence/historical_test.go`
- Modify: `backend/tools/internal/graphctl/evidence/discover.go`

- [x] **Step 1: Write exact Review-ref tests**

Commit two structured Reviews for the same Loop and make the completion ref point to the older one. Assert `LoadCommittedRef` returns exactly that ref, not the Review with the newest `reviewedAt`. Test absent path, hash mismatch, symlink/non-regular path, malformed JSON and cancellation.

For legacy refs, assert `VerifyCommittedRefs` verifies every supplied path/hash and rejects refs outside `docs/superpowers/reviews/`.

- [x] **Step 2: Run Review tests in RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/review -run 'TestLoadCommittedRef|TestVerifyCommittedRefs' -count=1
```

Expected: compile failure for the exact-ref functions.

- [x] **Step 3: Implement exact committed Review loading**

Reuse `readCommittedTree`, `readCommittedFile`, `validateRef`, `verifyTreeRef` and strict `decodeBytes`:

```go
func LoadCommittedRef(ctx context.Context, repoRoot string, ref FileRef) (Candidate, error) {
	if ctx == nil {
		return Candidate{}, fmt.Errorf("review: context is required")
	}
	if err := validateRef("completion Review ref", ref, "docs/superpowers/reviews/"); err != nil {
		return Candidate{}, err
	}
	tree, err := readCommittedTree(ctx, repoRoot)
	if err != nil {
		return Candidate{}, err
	}
	if err := verifyTreeRef(ctx, repoRoot, tree, ref); err != nil {
		return Candidate{}, fmt.Errorf("review: exact ref %q: %w", ref.Path, err)
	}
	content, err := readCommittedFile(ctx, repoRoot, tree, ref.Path)
	if err != nil {
		return Candidate{}, err
	}
	document, err := decodeBytes(content)
	if err != nil {
		return Candidate{}, fmt.Errorf("review: strict-decode exact ref %q: %w", ref.Path, err)
	}
	return Candidate{Ref: ref, Review: document}, nil
}
```

`VerifyCommittedRefs` sorts a copy, rejects duplicate paths, and verifies each exact committed regular blob. It never discovers or selects another candidate.

- [x] **Step 4: Write a complete historical fixture in RED**

Create a real Git history:

1. commit A contains an active historical Loop, validator, subject and Registry;
2. a later commit contains clean canonical Evidence and PASS Review bound to commit A;
3. the current manifest is changed to `closed` with an exact completion record;
4. a final commit changes a successor-owned source file that is outside the historical subject.

Assert:

```go
projection, err := ProjectClosedHistory(context.Background(), fixture.repo, fixture.closedLoop)
if err != nil {
	t.Fatalf("ProjectClosedHistory() error = %v", err)
}
if projection.SubjectCommit != fixture.subjectCommit || projection.ApprovalDigest == "" {
	t.Fatalf("historical projection = %#v", projection)
}
if !reflect.DeepEqual(projection.EvidenceRefs, fixture.completion.EvidenceRefs) {
	t.Fatalf("Evidence refs = %#v, want completion refs", projection.EvidenceRefs)
}
```

Add table cases for missing commit, missing historical Registry/Loop, current execution-contract mutation, scope/Registry/Validator mismatch, missing/hash-mismatched Evidence, non-PASS Evidence, missing/hash-mismatched Review, open Review finding, dependency key mismatch and changed completion refs. Assert every failure returns an empty `ApprovalDigest`.

- [x] **Step 5: Run historical tests in RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/evidence -run 'TestProjectClosedHistory' -count=1
```

Expected: compile failure for `HistoricalProjection` and `ProjectClosedHistory`.

- [x] **Step 6: Add an exact canonical Evidence loader**

Extract the existing per-candidate logic from `DiscoverCanonical` into a private function that accepts a prebuilt HEAD tree, one exact ref and one historical Gate contract. The exact path must:

- validate the repository path and active/historical scope ownership;
- verify the exact Evidence file hash before decoding;
- verify every referenced log/artifact regular blob and digest;
- validate the canonical envelope and clean PASS Gate proof;
- return an error immediately instead of a discovery rejection.

Use this signature:

```go
func loadExactCanonicalRefs(ctx context.Context, repoRoot string, loop *registry.Loop, contract GateContract, refs []registry.ArtifactRef) ([]CanonicalCandidate, error)
```

Keep `DiscoverCanonical` behavior unchanged by having it call the same extracted candidate validator and continue producing sorted rejections.

- [x] **Step 7: Implement the historical projector**

Use this ordered algorithm in `historical.go`:

```go
func ProjectClosedHistory(ctx context.Context, repoRoot string, currentLoop *registry.Loop) (HistoricalProjection, error) {
	projection := emptyHistoricalProjection()
	completion, err := requireClosedCompletion(currentLoop)
	if err != nil {
		return projection, err
	}
	projection.SubjectCommit = completion.SubjectCommit

	historicalRegistry, err := registry.LoadAtCommit(ctx, repoRoot, completion.SubjectCommit)
	if err != nil {
		return projection, fmt.Errorf("historical proof: load subject Registry: %w", err)
	}
	historicalLoop := historicalRegistry.ByID[currentLoop.Metadata.ID]
	if historicalLoop == nil {
		return projection, fmt.Errorf("historical proof: Loop %q is absent at subjectCommit", currentLoop.Metadata.ID)
	}
	if err := requireDependencyKeySet(historicalLoop.Spec.DependsOn, completion.DependencyDigests); err != nil {
		return projection, err
	}

	contract, err := BuildGateContract(ctx, repoRoot, completion.SubjectCommit, historicalRegistry, historicalLoop, completion.DependencyDigests)
	if err != nil {
		return projection, fmt.Errorf("historical proof: rebuild Gate contract: %w", err)
	}
	if err := validateCompletionContract(completion, contract); err != nil {
		return projection, err
	}
	if err := validateCurrentClosedContract(ctx, repoRoot, historicalRegistry, currentLoop, contract.RegistryDigest); err != nil {
		return projection, err
	}

	candidates, err := loadExactCanonicalRefs(ctx, repoRoot, historicalLoop, contract, completion.EvidenceRefs)
	if err != nil {
		return projection, fmt.Errorf("historical proof: exact Evidence: %w", err)
	}
	return finishHistoricalApproval(ctx, repoRoot, currentLoop, contract, candidates)
}
```

`validateCurrentClosedContract` computes `ControlPlaneDigest` from the subject commit and historical Registry, then computes `RegistryDigest` with the historical Registry entry plus the current closed Loop. Lifecycle, title and completion remain excluded by the existing Registry projection; scope, Gates, dependencies, Evidence and Review requirements remain protected.

For structured Review, load only `completion.reviewRef`, validate PASS/no open findings, and bind it to the exact historical scope/Registry/dependency digests and completion Evidence set. Treat the Review document's exact, hash-verified `specRefs` as the sealed historical required-spec set; do not discover newer specs owned by the current closed scope.

For `legacyReviewRefs`, require the exact L0 compatibility ID already enforced by Registry validation, verify every exact ref, and omit `ReviewRef` from `ApprovalDigest` as the existing compatibility path does.

- [x] **Step 8: Run Review and Evidence suites**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/review ./internal/graphctl/evidence -count=1
```

Expected: PASS; newer unreferenced Evidence/Review candidates do not affect the historical projection.

- [x] **Step 9: Prepare the Task 3 commit checkpoint**

After showing status and receiving explicit authorization only:

```bash
git add backend/tools/internal/graphctl/review/exact.go backend/tools/internal/graphctl/review/review_test.go backend/tools/internal/graphctl/evidence/historical.go backend/tools/internal/graphctl/evidence/historical_test.go backend/tools/internal/graphctl/evidence/discover.go
git commit -m "feat(graph): reconstruct closed loop history"
```

### Task 4: Route status and export through the historical projector

**Files:**

- Modify: `backend/tools/internal/graphctl/evidence/status.go`
- Modify: `backend/tools/internal/graphctl/evidence/evidence_test.go`
- Modify: `backend/tools/internal/graphctl/evidence/export.go`
- Modify: `backend/tools/internal/graphctl/evidence/export_test.go`
- Modify: `backend/tools/internal/graphctl/app.go`
- Modify: `backend/tools/internal/graphctl/app_test.go`

- [x] **Step 1: Write pure proof-basis/status tests**

Add a `DeriveHistoricalStatus` input that carries a projection, handoff IDs and a validation error:

```go
type HistoricalStatusInput struct {
	Loop           *registry.Loop
	Projection     HistoricalProjection
	HandoffLoopIDs []string
	ValidationErr  error
}
```

Test a valid projection returns `Gate=PASS`, `Review=PASS`, `Lifecycle=closed`, `Overall=APPROVED`, `ProofBasis=HISTORICAL`, exact subject commit, sorted handoff IDs and the projection approval digest. Test a validation error returns `Overall=STALE`, `Gate=STALE`, `Review=STALE`, no approval digest and a concrete `historical proof:` reason.

Add regressions that existing `DeriveStatus` always emits `ProofBasis=CURRENT` and a non-nil empty `HandoffLoopIDs` slice for active/planned inputs.

- [x] **Step 2: Run the status tests in RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/evidence -run 'HistoricalStatus|ProofBasis' -count=1
```

Expected: compile failure for the proof-basis fields and historical status function.

- [x] **Step 3: Implement the pure historical status projection**

Add `ProofBasis` and fields to `LoopStatus`, default current status in `DeriveStatus`, and add:

```go
func DeriveHistoricalStatus(input HistoricalStatusInput) LoopStatus {
	status := LoopStatus{
		GateStatus: GatePass,
		ReviewStatus: ReviewPass,
		Overall: OverallApproved,
		ProofBasis: ProofBasisHistorical,
		HandoffLoopIDs: append([]string(nil), input.HandoffLoopIDs...),
		CanonicalEvidence: []registry.ArtifactRef{},
		StaleReasons: []string{},
	}
	sort.Strings(status.HandoffLoopIDs)
	if input.Loop != nil {
		status.LoopID = input.Loop.Metadata.ID
		status.Lifecycle = input.Loop.Spec.Lifecycle
		if input.Loop.Spec.Completion != nil {
			status.ProofSubjectCommit = input.Loop.Spec.Completion.SubjectCommit
		}
	}
	if input.ValidationErr != nil {
		status.GateStatus = GateStale
		status.ReviewStatus = ReviewStale
		status.Overall = OverallStale
		status.StaleReasons = []string{"historical proof: " + input.ValidationErr.Error()}
		return status
	}
	status.ProofSubjectCommit = input.Projection.SubjectCommit
	status.ScopeDigest = input.Projection.ScopeDigest
	status.RegistryDigest = input.Projection.RegistryDigest
	status.ValidatorDigests = cloneDigestMap(input.Projection.ValidatorDigests)
	status.DependencyDigests = cloneDigestMap(input.Projection.DependencyDigests)
	status.CanonicalEvidence = append([]registry.ArtifactRef(nil), input.Projection.EvidenceRefs...)
	status.ReviewRef = cloneArtifactRef(input.Projection.ReviewRef)
	status.ApprovalDigest = input.Projection.ApprovalDigest
	return status
}
```

Add a package-local `cloneArtifactRef` helper that returns `nil` or a value copy. Before the success copy, reject a nil/non-closed Loop, missing projection digests, duplicate handoff IDs and an empty approval digest as historical corruption rather than emitting APPROVED. Sorting is normalization; duplicates are invalid.

- [x] **Step 4: Add the single lifecycle branch in `statusLoaded`**

Fetch tracked paths once. Keep planned handling first. For a closed Loop:

1. call `scope.ResolveClosedHandoff`;
2. call `evidence.ProjectClosedHistory`;
3. join either error into the historical validation error;
4. derive historical status and continue to the next DAG node.

Only active Loops may enter current Evidence discovery, Review selection, runtime Gate lookup and dirty-path staleness. Change `currentSnapshots` to skip every lifecycle except active.

Add an app integration test proving all of the following in one real Git fixture:

- closed status remains APPROVED after a successor-owned source commit;
- approval digest remains byte-identical;
- dirty successor worktree changes stale only the active successor;
- a newer runtime PASS for the closed ID is ignored;
- corrupting an exact historical ref yields historical STALE and no approval digest;
- planned and active status precedence is unchanged.

- [x] **Step 5: Replace export's current-HEAD closed approval path**

In `currentExportProof`, handle closed ancestors before building a current Gate contract:

```go
if loop.Spec.Lifecycle == registry.LifecycleClosed {
	if _, err := scope.ResolveClosedHandoff(reg, loop, tracked); err != nil {
		continue
	}
	projection, err := ProjectClosedHistory(ctx, repoRoot, loop)
	if err != nil {
		continue
	}
	approved[loopID] = projection.ApprovalDigest
	continue
}
```

Load `tracked` once before the DAG walk with `changes.Client{RepoRoot: repoRoot, Run: changes.ExecRunner{}}. An empty handoff is valid when no historical subject file still exists. Remove `currentClosedApproval` after all callers use `ProjectClosedHistory`.

Add export tests proving an active target accepts a stable closed dependency approval after successor code advances and rejects that dependency when its historical artifact or handoff is corrupt.

- [x] **Step 6: Run status, app and export suites**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/evidence ./internal/graphctl -count=1
```

Expected: PASS; no test relies on current runtime state to approve a closed Loop.

- [x] **Step 7: Prepare the Task 4 commit checkpoint**

After status review and explicit authorization only:

```bash
git add backend/tools/internal/graphctl/evidence/status.go backend/tools/internal/graphctl/evidence/evidence_test.go backend/tools/internal/graphctl/evidence/export.go backend/tools/internal/graphctl/evidence/export_test.go backend/tools/internal/graphctl/app.go backend/tools/internal/graphctl/app_test.go
git commit -m "feat(graph): project closed status from history"
```

### Task 5: Expose historical proof clearly in CLI and dashboard

**Files:**

- Modify: `backend/tools/internal/graphctl/render/model.go`
- Modify: `backend/tools/internal/graphctl/render/render.go`
- Modify: `backend/tools/internal/graphctl/render/index.html.tmpl`
- Modify: `backend/tools/internal/graphctl/render/render_test.go`
- Modify: `backend/tools/internal/graphctl/app.go`
- Modify: `backend/tools/internal/graphctl/app_test.go`
- Modify: `backend/tools/cmd/graphctl/main.go`
- Modify: `backend/tools/cmd/graphctl/main_test.go`

- [x] **Step 1: Write renderer normalization tests**

Extend `LoopView` with the same three historical fields. Add table tests enforcing:

- active/planned views require `CURRENT`, no subject commit and no handoff IDs;
- closed views require `HISTORICAL` and a valid 40-64 lowercase hexadecimal subject commit;
- handoff IDs are sorted and unique;
- a valid historical approved card renders `Historical proof: APPROVED`;
- a historical stale card renders `HISTORICAL_PROOF_CORRUPT` while JSON `overall` remains `STALE`.

Use this fixture shape:

```go
LoopView{
	ID: "L0-history", Title: "Historical", DependsOn: []string{},
	GateStatus: evidence.GatePass, ReviewStatus: evidence.ReviewPass,
	Lifecycle: registry.LifecycleClosed, Overall: evidence.OverallApproved,
	ProofBasis: evidence.ProofBasisHistorical,
	ProofSubjectCommit: strings.Repeat("a", 40),
	HandoffLoopIDs: []string{"L1-successor"},
}
```

- [x] **Step 2: Run renderer tests in RED**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/render -run 'Historical|ProofBasis' -count=1
```

Expected: compile failure for the new view fields.

- [x] **Step 3: Map and normalize historical fields**

Copy the three fields in `buildGraphStatus`. In `normalizeGraphStatus`, validate lifecycle/basis combinations and normalize `HandoffLoopIDs` with the existing sorted-unique helper. Preserve empty arrays across the JSON round trip.

Add this template helper:

```go
"displayOverall": func(loop LoopView) string {
	if loop.ProofBasis == evidence.ProofBasisHistorical && loop.Overall == evidence.OverallStale {
		return "HISTORICAL_PROOF_CORRUPT"
	}
	return string(loop.Overall)
},
```

- [x] **Step 4: Render a separate historical block**

Use `{{displayOverall .}}` for the visible card status. Under the facts block, render:

```html
{{if eq .ProofBasis "HISTORICAL"}}
<div class="historical-proof">
  <div><span class="label">Historical proof</span><span class="status {{statusClass .Overall}}">{{displayOverall .}}</span></div>
  <div><span class="label">Subject commit</span><code>{{.ProofSubjectCommit}}</code></div>
  <div><span class="label">Current owners</span><span>{{if .HandoffLoopIDs}}{{join .HandoffLoopIDs ", "}}{{else}}none (no current subject files){{end}}</span></div>
</div>
{{end}}
```

Do not relabel historical attempt cards as current alerts.

- [x] **Step 5: Include basis in human CLI output**

For `status` and Loop lines under `context`, append `Basis=` followed by the proof-basis value. For historical entries append `Subject=` followed by the commit and `Handoff=` followed by the owner list. JSON output already receives the typed fields from `LoopStatus`.

Update CLI tests to assert the exact tokens `Basis=HISTORICAL`, `Subject=` and `Handoff=`. Keep all existing columns.

- [x] **Step 6: Run renderer, app and CLI suites**

Run:

```bash
cd backend/tools
GOWORK=off go test ./internal/graphctl/render ./internal/graphctl ./cmd/graphctl -count=1
```

Expected: PASS and deterministic JSON/HTML output.

- [x] **Step 7: Prepare the Task 5 commit checkpoint**

After status review and explicit authorization only:

```bash
git add backend/tools/internal/graphctl/render/model.go backend/tools/internal/graphctl/render/render.go backend/tools/internal/graphctl/render/index.html.tmpl backend/tools/internal/graphctl/render/render_test.go backend/tools/internal/graphctl/app.go backend/tools/internal/graphctl/app_test.go backend/tools/cmd/graphctl/main.go backend/tools/cmd/graphctl/main_test.go
git commit -m "feat(graph): display historical loop proofs"
```

### Task 6: Document, verify and independently review O0b

**Files:**

- Modify: `backend/tools/README.md`
- Create after independent review: `docs/superpowers/reviews/G0-rd-graph-control-plane/O0b-20260818/milestone-review.md`

- [x] **Step 1: Document the operator model**

Add a concise section to `backend/tools/README.md` containing this distinction:

```text
CURRENT means graphctl is evaluating an active/planned Loop against current HEAD,
current runtime Gate state and current committed Evidence/Review.

HISTORICAL means graphctl reconstructed a closed Loop from completion.subjectCommit
and exact completion refs. Later successor code does not stale this proof. Missing or
altered historical objects produce HISTORICAL_PROOF_CORRUPT and no approvalDigest.

handoffLoopIds are current active owners, not historical approvers.
```

Include the two typed handoff error codes and the repair action: add or narrow an active owner; never rewrite completion merely to clear a handoff failure.

- [x] **Step 2: Format and run the complete targeted suite**

Run:

```bash
gofmt -w backend/tools/cmd/graphctl/main.go backend/tools/cmd/graphctl/main_test.go backend/tools/internal/graphctl/app.go backend/tools/internal/graphctl/app_test.go backend/tools/internal/graphctl/registry/load.go backend/tools/internal/graphctl/registry/load_commit.go backend/tools/internal/graphctl/registry/load_commit_test.go backend/tools/internal/graphctl/registry/validate.go backend/tools/internal/graphctl/scope/handoff.go backend/tools/internal/graphctl/scope/handoff_test.go backend/tools/internal/graphctl/review/exact.go backend/tools/internal/graphctl/review/review_test.go backend/tools/internal/graphctl/evidence/discover.go backend/tools/internal/graphctl/evidence/historical.go backend/tools/internal/graphctl/evidence/historical_test.go backend/tools/internal/graphctl/evidence/status.go backend/tools/internal/graphctl/evidence/evidence_test.go backend/tools/internal/graphctl/evidence/export.go backend/tools/internal/graphctl/evidence/export_test.go backend/tools/internal/graphctl/render/model.go backend/tools/internal/graphctl/render/render.go backend/tools/internal/graphctl/render/render_test.go
cd backend/tools
GOWORK=off go test ./cmd/graphctl ./internal/graphctl/... -count=1
GOWORK=off go test -race ./cmd/graphctl ./internal/graphctl/... -count=1
GOWORK=off go vet ./cmd/graphctl ./internal/graphctl/...
mkdir -p .bin
GOWORK=off go build -o .bin/graphctl ./cmd/graphctl
```

Expected: all commands exit 0. This exact `go test` target is the registered G0 Gate command and is the one O0b milestone verification. Do not invoke the L0 Things baseline.

- [x] **Step 3: Validate Registry, ownership and repository boundaries**

Run:

```bash
cd backend/tools
GOWORK=off go run ./cmd/graphctl validate --repo-root ../..
GOWORK=off go run ./cmd/graphctl context --repo-root ../..
GOWORK=off go run ./cmd/graphctl scope-check --repo-root ../..
cd ../..
git diff --check
git status --short
git -C /home/wangjunbo/workspace-iot-platform/things status --short
```

Expected:

- Registry valid;
- all O0b files are owned by G0;
- `things` output is empty;
- no G0/L0 manifest, completion, Evidence or canonical Review changed;
- `.graph/runtime/` remains ignored;
- no unrelated main-worktree file appears in this branch.

- [x] **Step 4: Run one fresh independent O0b review**

Give a fresh reviewer only the approved design, this plan, the O0b diff and test output. The reviewer checks this exact list:

```text
closed status reads completion.subjectCommit, not current HEAD
only exact completion Evidence/Review refs are accepted
current runtime and dirty paths cannot approve/stale closed history
current closed execution-contract mutation is detected
sealed dependency digests do not follow later dependency evolution
every existing historical subject file has exactly one active owner
corrupt history emits no approvalDigest
active/planned behavior is unchanged
dashboard distinguishes historical corruption from active staleness
no real G0/L0 lifecycle, completion, Evidence or canonical Review changed
```

If and only if there are no open findings, record:

```markdown
# O0b Milestone Review

- Scope: O0b closed Loop history and ownership handoff delta
- Verdict: PASS
- Canonical G0 Review: no
- Subject commit: exact object ID printed by `git rev-parse --verify HEAD^{commit}` at review start
- Verification: G0 graphctl tests, race, vet, build, Registry validate and scope-check passed
- Open findings: none
```

The reviewer must run `git rev-parse --verify HEAD^{commit}` first and insert that literal output into the file with `apply_patch`. This Markdown artifact is not named `review.json`, so graphctl cannot mistake it for G0 completion Review.

- [x] **Step 5: Prepare the final implementation checkpoint**

Show `git status --short`, the test evidence and the exact staged proposal. Only after explicit user authorization:

```bash
git add backend/tools/README.md backend/tools/cmd/graphctl/main.go backend/tools/cmd/graphctl/main_test.go backend/tools/internal/graphctl/app.go backend/tools/internal/graphctl/app_test.go backend/tools/internal/graphctl/registry/load.go backend/tools/internal/graphctl/registry/load_commit.go backend/tools/internal/graphctl/registry/load_commit_test.go backend/tools/internal/graphctl/registry/validate.go backend/tools/internal/graphctl/scope/handoff.go backend/tools/internal/graphctl/scope/handoff_test.go backend/tools/internal/graphctl/review/exact.go backend/tools/internal/graphctl/review/review_test.go backend/tools/internal/graphctl/evidence/discover.go backend/tools/internal/graphctl/evidence/historical.go backend/tools/internal/graphctl/evidence/historical_test.go backend/tools/internal/graphctl/evidence/status.go backend/tools/internal/graphctl/evidence/evidence_test.go backend/tools/internal/graphctl/evidence/export.go backend/tools/internal/graphctl/evidence/export_test.go backend/tools/internal/graphctl/render/model.go backend/tools/internal/graphctl/render/render.go backend/tools/internal/graphctl/render/index.html.tmpl backend/tools/internal/graphctl/render/render_test.go docs/superpowers/plans/2026-08-18-o0b-graph-closed-loop-history-handoff.md docs/superpowers/reviews/G0-rd-graph-control-plane/O0b-20260818/milestone-review.md
git commit -m "feat(graph): preserve closed loop historical proof"
```

Do not stage `.graph/runtime/`, any real completion/Evidence/`review.json`, `backend/go.work.sum`, or anything under `things`.

## O0b completion criteria

O0b is complete only when all of these are true:

1. `LoadAtCommit` validates canonical Registry documents without checkout/worktree mutation.
2. closed status is reconstructed from `completion.subjectCommit` and exact sealed artifacts.
3. ordinary active-successor changes leave a valid historical approval digest unchanged.
4. missing/tampered Git objects, artifacts, contracts or completion fields fail closed with no approval digest.
5. every existing tracked historical subject file has exactly one active owner.
6. active and planned status, routing, current Evidence selection and dirty-path behavior remain unchanged.
7. status JSON, human CLI and dashboard explicitly expose `CURRENT` versus `HISTORICAL`.
8. export uses the same historical approval projector as status.
9. targeted G0 tests, race, vet, build, Registry validation and scope-check pass.
10. an independent O0b milestone review records PASS with no open finding.
11. no real G0/L0 lifecycle, completion, Evidence or canonical Review changed.

## Design traceability

| Approved O0b design section | Plan coverage |
|---|---|
| 4 Status model | Tasks 4-5 |
| 5.1 Historical inputs | Tasks 1 and 3 |
| 5.2 Reconstruction | Tasks 1 and 3 |
| 5.3 Exact artifact validation | Task 3 |
| 5.4 Stable dependencies | Tasks 3-4 |
| 6 Ownership handoff | Task 2 and Task 4 integration |
| 7 Application lifecycle branch | Task 4 |
| 8 Dashboard behavior | Task 5 |
| 9 Fail-closed handling | Tasks 2-4 |
| 10 Test strategy | Tests in every task plus Task 6 |
| 11 Acceptance criteria | Task 6 and completion checklist |
