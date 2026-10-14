# O0b Closed Loop History and Ownership Handoff Design

**Date:** 2026-08-18

**Status:** Draft for written review

**Parent design:** `2026-08-18-rd-graph-fast-full-control-plane-design.md`

**Predecessor:** `2026-08-18-o0a-graph-active-ownership-context.md`

## 1. Purpose

O0b fixes one lifecycle ambiguity: a Loop that was validly closed at commit A must not look unfinished merely because the repository later advances to commit B.

A closed Loop is an immutable historical proof. Code that continues to evolve must be owned by an active successor. In plain terms, closing a Loop means preserving its graduation record and handing ongoing work to a new owner.

O0b completes the historical half of O0. O0a already ensures that only active Loops route current changes; O0b makes closed proof validation stable and verifies the ownership handoff.

## 2. Decisions

1. Active Loops continue to derive status from current `HEAD`, current runtime Gate results and current committed Evidence/Review.
2. Closed Loops derive status from `completion.subjectCommit` and the exact Evidence/Review references sealed by completion.
3. A later ordinary implementation change must not invalidate a valid closed Loop.
4. Missing or altered historical proof remains fail closed.
5. Existing tracked implementation files selected by a closed Loop's `evidence.subjectPaths` must each have exactly one active owner.
6. Historical plans, Evidence and Review files that are not semantic implementation inputs do not require a successor. If no active Loop owns them, they remain frozen and a later attempted modification is rejected as unowned.
7. O0b implements generic semantics and tests only. It does not close G0 or L0.

## 3. Non-goals

O0b does not:

- implement `finalize`, `prepare` or `approve`;
- generate or repair G0/L0 Evidence or Review;
- change G0/L0 lifecycle or completion records;
- introduce FAST/FULL profiles, queues, workers or caches;
- change business services or the read-only external `things` repository;
- provide a remote, non-bypassable enforcement boundary.

## 4. Status model

`LoopStatus` and the dashboard view gain a proof basis:

```text
CURRENT     active Loop evaluated against current HEAD
HISTORICAL  closed Loop evaluated against completion.subjectCommit
```

For a historical status, the machine also exposes:

- `proofSubjectCommit`: the completion subject commit;
- `handoffLoopIds`: the sorted active successor Loop IDs that own current semantic implementation files.

The existing `overall` state remains authoritative:

- a valid closed proof is `APPROVED` with `proofBasis=HISTORICAL`;
- an invalid closed proof is `STALE` with a concrete historical-proof reason;
- the dashboard renders the latter as `HISTORICAL_PROOF_CORRUPT`, avoiding confusion with ordinary current-code staleness.

No new completion claim is inferred from Markdown prose or runtime HTML.

## 5. Historical proof projection

### 5.1 Inputs

For a closed Loop, the projection starts from committed data only:

- the current closed Loop manifest and its `completion` record;
- `completion.subjectCommit` as a real Git commit object;
- the Registry and Loop execution contract stored at that commit;
- the exact completion Evidence reference set;
- either the exact structured Review reference or the existing L0-only `legacyReviewRefs` compatibility path;
- the sealed dependency digest map.

The latest runtime run, current worktree dirtiness and newer unreferenced Evidence/Review candidates are not historical-proof inputs.

### 5.2 Reconstruction

The implementation reads historical files directly from Git objects without checking out or mutating a worktree. From `completion.subjectCommit`, it reconstructs:

- subject scope digest;
- Registry execution digest;
- required Validator identities and digests.

The historical Registry and Loop ID must be present and valid. The current closed manifest's execution contract must still project to the same historical Registry digest when evaluated with the historical Registry/control-plane inputs. This detects post-closure edits to scope, Gates or validation semantics while allowing the already-excluded lifecycle, completion and display metadata to differ.

### 5.3 Exact artifact validation

Historical validation follows completion references exactly; it never selects the newest candidate:

1. Every referenced file must be a committed regular Git blob at the canonical repository path.
2. Its content SHA-256 must equal the completion reference.
3. Canonical Evidence must be a clean PASS for the historical subject, Registry, Validators and sealed dependency digests.
4. Structured Review must be PASS, have no open findings and bind the same historical target and exact Evidence set.
5. The existing L0-only legacy path remains restricted to L0 and validates every `legacyReviewRefs` content hash; O0b does not use this exception to close L0.
6. Completion fields must exactly match the reconstructed projection and selected artifacts.

Only after these checks does the machine derive the stable historical `approvalDigest`.

### 5.4 Stable dependency behavior

Closed history preserves the dependency digest map sealed at approval time. It does not compare that map with a dependency's later active state. Any edit to the sealed map, Evidence, Review or completion changes the reconstructed approval and fails validation.

This prevents normal successor evolution from rewriting history while retaining an immutable audit chain.

## 6. Ownership handoff

The current active owner graph remains the source of truth for future edits. Closed Loops never route current changes.

During `graphctl validate`, the implementation obtains the current committed tracked paths. For every closed Loop:

1. select the existing tracked files matched by its `evidence.subjectPaths`;
2. resolve owners using current active `scope.owns` and `exclude` semantics;
3. require exactly one active owner for every selected file;
4. aggregate and sort those owner IDs as `handoffLoopIds`.

Failure is typed and deterministic:

- zero active owners: `CLOSED_HANDOFF_MISSING`;
- multiple active owners: `CLOSED_HANDOFF_AMBIGUOUS`.

The check is file-based and intentionally limited to existing semantic inputs. A future new path still passes through the normal changed-path resolver and fails with `UNOWNED_PATH` unless an active Loop owns it.

## 7. Application flow

Status calculation branches once by lifecycle:

```text
planned -> existing PLANNED projection
active  -> existing current-HEAD projection
closed  -> historical completion projection
```

The closed branch:

1. builds the historical proof projection;
2. validates exact Evidence/Review/completion references;
3. validates current ownership handoff;
4. returns `HISTORICAL + APPROVED`, or `HISTORICAL + STALE` with precise corruption reasons.

It does not read current runtime Gate state and does not add current dirty-path staleness. Current changes affect active successors through the existing O0a resolver.

## 8. Dashboard behavior

The dashboard labels proof basis explicitly.

A valid closed Loop is presented as:

```text
Historical proof: APPROVED
Subject commit: <commit>
Current owners: <active successor IDs>
```

An invalid closed proof is presented as `HISTORICAL_PROOF_CORRUPT` with the concrete missing or mismatched object. It must not be rendered as an ordinary active `STALE` card.

Historical attempt cards remain audit history and must be visually separated from current alerts.

## 9. Failure handling

Historical status fails closed when:

- the subject commit or required historical Registry/Loop object is unavailable;
- reconstructed scope, Registry or Validator digests differ from completion;
- current closed execution semantics differ from the historical projection;
- an Evidence/Review reference is absent, non-canonical, not a regular blob or hash-mismatched;
- Evidence is not a clean historical PASS;
- Review is not PASS or contains open findings;
- completion references or dependency digests are inconsistent;
- ownership handoff is missing or ambiguous.

Errors must identify the Loop and failing object or path without leaking file contents. A damaged closed Loop cannot provide an `approvalDigest` to downstream Loops.

## 10. Test strategy

Implementation follows TDD and covers four behavioral groups.

### 10.1 Historical stability

- close a fixture Loop at commit A;
- advance HEAD with an ordinary successor-owned implementation change;
- assert the fixture remains `HISTORICAL + APPROVED` with the same `approvalDigest`;
- assert only the active successor is affected by the new path.

### 10.2 Historical corruption

Reject missing subject commits, modified/missing Evidence or Review refs, contract digest mismatches, open findings and unavailable Git objects. No case may fall back to a current runtime PASS.

### 10.3 Handoff

Test exactly one active successor, no successor and two active successors. Historical-only files outside `evidence.subjectPaths` do not require handoff.

### 10.4 Regression

Active and planned status precedence, O0a ownership routing, Evidence selection and current dirty-path behavior remain unchanged.

Run targeted package tests throughout implementation, followed by one G0 milestone verification. O0b does not intentionally run the slow L0 migration baseline Gate.

## 11. Acceptance criteria

O0b is accepted when:

1. closed status is reconstructed from `completion.subjectCommit`, not current HEAD;
2. future successor-owned changes do not stale a valid historical proof;
3. proof tampering or missing Git objects fail closed with a precise reason;
4. current tracked semantic implementation files have exactly one active successor owner;
5. active/planned Loop behavior is unchanged;
6. dashboard separates historical proof from current applicability;
7. no real G0/L0 lifecycle, completion, Evidence or Review is changed;
8. targeted tests and the single G0 milestone verification pass;
9. one independent O0b milestone Review records PASS before O0b is called accepted.

After O0b acceptance, the O0 Ownership implementation scope is complete and F0 FAST-path design can proceed. Closing any real Graph Loop remains a separate Evidence/Review/user-approval operation.
