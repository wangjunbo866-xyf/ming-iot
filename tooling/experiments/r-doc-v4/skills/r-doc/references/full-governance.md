# Full governance mode

Read this workflow for explicit whole-repository audits, initialization, structural migrations, or repository-wide gates. Discovery and focused maintenance complete through their own route in [SKILL.md](../SKILL.md#choose-the-task-mode).

## Standard workflow

For governance work selected above, choose the project governance level and task-impact workflow separately. A lower governance level reduces documentation scaffolding; it never disables safe-reading, link-target, sensitive-content, preservation, or conflict checks.

### Project governance levels

| Level | Use when | Required structure |
| --- | --- | --- |
| `minimal` | A personal repository or a small project with a few maintained documents. | Keep a concise root `AGENTS.md`. Link relevant documents directly from it. `docs/README.md`, nested indexes, and frontmatter are optional. |
| `standard` | The default for shared projects and ongoing maintenance. | Require root `AGENTS.md`, the configured documentation-root `README.md`, nested `README.md` indexes for topic directories, and parent/child navigation. |
| `strict` | Long-lived team, release-critical, or high-assurance documentation. | Use the standard structure, configure required document types, relationships, and lifecycle gates, and treat audit warnings as failures. |

Set `governance_level: minimal`, `governance_level: standard`, or `governance_level: strict` in `.r-doc.yaml`; without it, use `standard`. The audit and repair helpers honor the selected level. In `minimal`, audit still checks links and sensitive content. In this pilot, every maintained document under the configured documentation root must be reachable from `AGENTS.md` through links in maintained Markdown files; a disconnected index is insufficient. Root Markdown files may provide navigation without each requiring an incoming link.

### Task impact and authorization

| Impact | Typical scope | Workflow |
| --- | --- | --- |
| Low | A typo, one topic update, or mechanical index/link/date repair with established facts. | Read the relevant entrypoint and target only; make the requested change and run the relevant check. |
| Medium | A bounded public-interface, configuration, process, or cross-document change. | Summarize the impact and affected documents, inspect only relevant safe diffs and sources, then update and verify. |
| High | A broad initialization or audit, conflicting sources of truth, sensitive-data handling, release decisions, or consequential moves/archives. | Inventory affected paths and evidence, outline the proposed changes, and pause only for a material ambiguity or an action beyond the user's authorized scope. |

An explicit request to change named documentation, or to repair the documented issue, authorizes that work and the mechanical index/link changes it requires. State the scope and proceed without a separate plan-confirmation turn. Ask only when a fact, target scope, or consequence is materially unclear; do not turn a status update or focused plan into a permission gate.

1. Identify the current development stage, task scope, and document types that may be affected.
2. Read the project-root `AGENTS.md` if it exists, the documentation-root index when required by the selected governance level, relevant nested indexes if present, project-level `.r-doc.yaml`, and documents directly related to the task.
3. Inspect project structure and list changed paths/status first. Exclude protected paths before reading any diff; inspect only explicit safe diffs and relevant code needed to determine documentation impact.
4. Build a document inventory and mark missing, stale, unindexed, broken, status-invalid, duplicated, conflicting, or suspiciously sensitive content.
5. Resolve only material ambiguities. Give a concise scope/plan update before broad work, but do not wait for another confirmation when the user has already authorized the change.
6. Create or update entrypoints, indexes, topic documents, metadata, and project configuration within the authorized scope. Apply safe mechanical repairs in the same task after reviewing the preview; ask only if it exposes a semantic conflict or out-of-scope change.
7. Run the deterministic helpers in `scripts/` when the target environment can execute them. At minimum, run `audit_docs.py --root <project-root>`; use `--strict` for release or merge gates. If a helper cannot run, perform the equivalent checks and report the limitation.
8. Verify context-loading order, required index coverage, links, applicable metadata, relationships, stage gates, and sensitive-content checks.
9. Produce a governance report covering scope, findings, completed updates, blockers, non-blockers, verification evidence, and whether the current stage gate is satisfied.

For stage checklists, read [references/lifecycle-checklists.md](lifecycle-checklists.md). For the complete operating workflow, read [references/workflow.md](workflow.md).
For deterministic checks and temporary-project QA, read [references/verification.md](verification.md).
For safe structural repairs, read [references/repair.md](repair.md). For concrete scenarios and common pitfalls, read [references/examples.md](examples.md) and [references/pitfalls.md](pitfalls.md).
For version-to-version adoption notes, read [references/migration-matrix.md](migration-matrix.md).
For cross-agent behavior evaluation, read [references/agent-evaluation.md](agent-evaluation.md) and run its evidence validator when evaluation artifacts are available. Package tests do not replace transcript-and-diff evaluation against a real agent.

## 60-second path

1. Select the task mode above. Discovery and focused maintenance follow their own completion checks; the remaining steps apply to full governance work.
2. Read `AGENTS.md`, the documentation-root index when required by the selected governance level, and the relevant topic index if present.
3. Preview safe structural repairs with `python scripts/repair_docs.py --root <project-root>`.
4. If the request authorizes the documentation repair and the preview contains only safe mechanical changes, apply the displayed repairs with `--apply` in the same task, then run `audit_docs.py --strict`. Do not ask for a second confirmation. Pause only for a material scope, fact, or risk ambiguity.
5. Report automated results separately from semantic conflicts and decisions that still need confirmation.

