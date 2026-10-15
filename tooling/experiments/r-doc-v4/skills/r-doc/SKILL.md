---
name: r-doc
description: "Govern documentation with bounded safe reads for public, interface, config, architecture, release, or docs changes; protect .env/secrets and skip unrelated code-only refactors."
metadata:
  version: "1.0.0-ming.4"
---

# r-doc: Project documentation governance

## Purpose

Maintain project documentation as a navigable, reviewable knowledge base with low context cost: have maintainers or AI agents read the project-root `AGENTS.md` first, then load only the minimum document set required for the current task through the indexes.

This is a project-level documentation standard, not a business-code implementation standard. The project's documentation hierarchy never overrides system instructions, the user's current request, or higher-level tool rules.

## When to use

Use this skill automatically in the following situations, or whenever the user explicitly invokes `$r-doc`:

- Initializing a project, organizing project knowledge, or completing missing documentation entry points;
- Working with plans, requirements, design, architecture decisions, APIs, testing, releases, deployment, rules, or process documents;
- Changing code, configuration, interfaces, data models, processes, deployment, or architecture;
- Checking documentation synchronization during code review, before merging, before release, or during maintenance;
- Finding missing, stale, duplicated, conflicting, broken, or incorrectly loaded documentation.

Do not start the full documentation-governance workflow for a purely local code refactor that does not affect public behavior, data, configuration, architecture, or project rules.

## Activation gate

Implicit activation is intentionally supported, but apply this gate before doing substantial work. Continue only when at least one condition is true:

- The user explicitly invokes `$r-doc` or asks for documentation governance;
- The task creates or changes public behavior, interfaces, configuration, data formats, architecture, processes, deployment, release behavior, or project rules;
- The task asks to initialize, index, audit, repair, synchronize, review, or publish project documentation.

Do not turn an unrelated code edit into a documentation project merely because the repository contains `AGENTS.md` or `docs/`.

## Choose the task mode

For a read-only question about context, ownership, behavior, or a historical change, use **discovery**:

1. Identify the available repositories and their entrypoints. Read the authoritative topic and the exact code or historical evidence needed for the question.
2. Separate intended behavior, inspected implementation, and historical verification. Bind claims to the source revision when available; mark missing repositories and unverified claims explicitly.
3. Return the requested answer or report with source paths, supporting excerpts, and remaining unknowns. Verify those citations and any requested source-integrity checks. Finish when each requested claim has evidence or an explicit limitation.

Discovery keeps the provided material unchanged. A read-only context question does not require a repository-wide audit, repair preview, metadata migration, or the governance report below.

For an authorized update to an existing topic with established ownership and a bounded set of affected documents, use **focused maintenance**:

1. Follow the entrypoint to the owning document and relevant sources. Establish the exact facts and versions before editing. Preserve existing project metadata, lifecycle meanings, approval boundaries, and explicitly historical statements.
2. Update the owning document and required navigation in place. Do not create a competing specification, translation, or directory merely to fit a template. Keep changes within the requested scope.
3. Inspect the final diff and changed paths. Check the changed Markdown files and affected navigation with `python <skill-root>/scripts/check_docs.py --root <project-root> --files <relative-path> ...`. This reads named files and their linked anchor targets, and reports its limited coverage. Verify source consistency and history preservation, then run the project's applicable metadata/lifecycle validator. If that validator is unavailable, report the unverified contract; a schema mismatch does not authorize converting project facts.
4. Report the changes, checks actually run, and remaining limitations. Keep historical evidence separate from tests or reviews performed now. Finish when the requested changes and applicable checks are complete; repeat checks only after new changes or unresolved failures.

Focused maintenance does not require a full repository audit or structural repair preview unless the user or project gate requires one. Do not claim that a local check satisfies a repository-wide gate. Record known baseline problems without expanding the change to clear them. For an established metadata convention, the explicit [metadata profile](references/project-config.md#project-metadata-profile) configures full-audit field and status checks; the project's validator remains authoritative for its additional rules.

Use the [full governance workflow](references/full-governance.md) for explicit audits, broad restructuring or initialization, requested merge/release gates, or unresolved ownership conflicts. Bounded reading and safety checks apply in every mode.

## Bounded and safe reading

Use a bounded, deny-by-default reading plan. Enumerate paths without opening their contents, using tools such as `rg --files` or `Get-ChildItem`; then read only named files or explicit source, documentation, and test directories required by the task.

- Before inspecting Git changes, list paths and statuses with `git status --short` and tracked changed paths with `git diff --name-only`. Classify paths first, exclude protected paths, then read diffs only for explicit safe paths related to documentation impact (for example, `git diff -- docs/api.md src/public_interface.py`). Never use an unrestricted `git diff` when changed paths could include protected files.
- Do not run content searches from `.` or the whole project root. Prefer a narrow search such as `rg -n 'term' src docs tests` and add exclusions explicitly when a broader scope is unavoidable.
- Do not use `rg --hidden` or equivalent whole-tree content scans for convenience. Inspect hidden directories only when the task names a specific path, such as `.agents/notes/`.
- Treat `.env`, `.env.*`, `secrets.*`, credential files, private keys, certificates, and files or directories whose names identify secrets as protected. Do not open, concatenate, execute, or pass them to search, test, or build commands. If the task appears to depend on one, report the path without its contents and request explicit direction.
- Never combine a known safe file read with a wildcard or recursive content read. Keep `AGENTS.md`, indexes, and target files as separate, explicit reads.

Path enumeration is not content reading, but a command that searches or prints file contents must still obey these boundaries.

## Non-negotiable constraints

1. Scope work to the project root. Prefer the Git root; for non-Git projects, use the project root explicitly identified by the user.
2. Apply the selected governance level. `AGENTS.md` remains the project entrypoint; `docs/README.md`, nested indexes, and topic metadata are required only at the `standard` or `strict` level. Keep link and sensitive-content checks at every level.
3. Keep `AGENTS.md` limited to the project overview, scope, quick start, context-loading order, key directories, commands, mandatory rules, prohibitions, and task routes. At `standard` and `strict`, include the `docs/` index; at `minimal`, link directly to the few maintained documents instead.
4. Keep each topic document focused on one subject. Split mixed, hard-to-locate, or overlong topics. Do not create empty documents just to fill a directory.
5. Keep documentation synchronized with code, configuration, interfaces, processes, deployment, and decision changes. Record important constraints, pitfalls, and non-obvious decisions.
6. Never write secrets, tokens, passwords, sensitive personal information, or real values that could bypass security controls. Stop writing and report suspected sensitive information.
7. Merge non-destructively when `AGENTS.md`, `docs/`, or existing documents are present: preserve facts and history instead of overwriting or deleting them. Report conflicts and propose a single source of truth.
8. By default, modify only documents, indexes, templates, metadata, and documentation comments. Read code and Git diffs only to determine documentation impact; do not modify business code.
9. Do not treat file existence as documentation completion. Check indexes, links, status, relationships, and consistency of affected content.

## Full governance route

For an explicit repository-wide audit, initialization, structural migration, or full stage gate, read [full-governance.md](references/full-governance.md) and follow its workflow and report.

## Document structure and indexes

Use this default structure and preserve a reasonable existing structure whenever possible:

```text
AGENTS.md
docs/
└── README.md
```

This is the `standard` structure. Under `minimal`, keep documents flat where practical and link each maintained document directly from `AGENTS.md`; do not create indexes or metadata solely to satisfy a template. Add topic directories and nested indexes when their navigation value justifies the maintenance cost. At `standard` and `strict`, each topic index defines its scope, lists its documents, states the reading order, and links to its parent and children. When adding, renaming, or moving a maintained document, update the nearest required index in the same change. Use [references/templates/AGENTS.md.source.txt](references/templates/AGENTS.md.source.txt) and [references/templates/README.template.md](references/templates/README.template.md) only when the selected level calls for them.

At `standard` and `strict`, use this context-loading order. At `minimal`, start with the root entrypoint and follow its direct links to relevant documents:

```text
AGENTS.md
→ docs/README.md
→ relevant topic README.md
→ target document
→ supplementary documents explicitly linked by the target
```

## Status, relationships, and conflicts

Topic documents normally use this lifecycle:

```text
draft → proposed → active → superseded → archived
```

When documentation disagrees with code, tests, or other documents, state the intended behavior, current behavior, and conflict location. Follow the user's explicit decision when it resolves the conflict. Ask only when the authoritative source or intended behavior remains materially unclear; do not silently rewrite implementation or erase historical evidence.

For metadata rules, read [references/metadata-schema.md](references/metadata-schema.md). For project-level overrides, read [references/project-config.md](references/project-config.md).

## Decision notes

Use the optional decision-note layer for non-trivial changes whose rationale, alternatives, or consequences would otherwise be lost in a commit or scattered across documents. It lives under `.agents/notes/` by convention and is discovered automatically when present; configure `decision_notes.root` when a project uses another in-root location. Notes use `proposed/`, `implemented/`, `rejected/`, or `archived/` lifecycle directories and one class directory such as `architecture/`, `feature/`, or `testing/`. The deterministic audit checks their frontmatter, lifecycle path, required sections, links, relationships, and sensitive content, but does not require a central high-churn index. Search existing notes before proposing a related decision, update the owning note when the decision remains the same, and create a linked successor when the decision reverses or materially changes.

Read [references/decision-notes.md](references/decision-notes.md) for the trigger boundary, format, lifecycle, and maintenance workflow. Use [references/templates/decision.md](references/templates/decision.md) only after confirming that a decision note is warranted.
For lifecycle operations, use `python scripts/decision_notes.py archive <note-path>` to preview an archive move. If the user's request authorizes that archive and the preview matches the requested scope, add `--apply` in the same task; otherwise ask only when the target or consequences are materially unclear. Supersession links and cycles are checked by the normal audit.

## Minimum report standard

For governance work, the final report must include:

```text
Current stage and inspection scope
Findings (missing, stale, conflicting, broken, or sensitive content)
Completed documentation updates
Remaining blockers and non-blockers
Verification methods and results
Current documentation stage gate: satisfied / not satisfied
```

Use templates only when they reduce repeated work. Read [references/templates/README.md](references/templates/README.md) first, then choose a template for the project type and current task. Do not generate batches of empty documents.
