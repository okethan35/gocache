---
name: spec
description: Turn a grilled idea into an owner-approved SPEC.md section: explore the codebase for collisions, settle edge cases, write the text on approval. Second step of /grill -> /spec -> /new-track.
disable-model-invocation: true
argument-hint: [feature or brief path]
---

# /spec

Turn a settled idea into a SPEC.md section that `/new-track` can slice. The idea itself (premise, goals, scope) was settled by `/grill`. This skill settles the contract: exact behavior, edge cases, and how the feature meets the existing code.

## 1. Find the source

- **Chat first.** If this conversation holds a `/grill` session, work from it.
- **Brief second.** The brief is `~/.claude/briefs/<repo>/<slug>.md` (`<repo>` is the basename of `git rev-parse --show-toplevel`). Read it whenever it exists; it fills in whatever compaction dropped from the chat. If several briefs exist and none matches the argument, ask which one.
- **Neither.** Ask the owner which file to work from, or suggest running `/grill` first.

**Gate:** if the brief or the chat still has open idea-level questions (problem, goals, scope), name them and send the owner back to `/grill`. Skip the gate only when the owner says "skip grill" (for small changes). Then take the idea as the owner states it.

## 2. Orient

- If `graphify-out/` exists at the repo root, run `graphify update .`, then read `graphify-out/GRAPH_REPORT.md`.
- Otherwise read CLAUDE.md and the directory layout.
- Read SPEC.md section headings and the sections nearest this feature, so the draft matches the house style and you know which edge-case classes this project already pins.

## 3. Fact-finder

Every codebase question goes through the **fact-finder**: a read-only sub-agent (`Explore`) that returns conclusions, never file contents. Dispatch independent questions in parallel. Each prompt asks for:

- **Answer** to the question, in a few lines.
- **Evidence** as `file:line` references.
- **Confidence**: high, medium or low, with what would raise it.
- **Collisions**: CLAUDE.md invariants the feature touches, existing tests that pin related behavior, and SPEC.md rules it overlaps or contradicts.

Tell the fact-finder which search tools to use: `graphify query`, `graphify path` and `graphify explain` when `graphify-out/` exists, plus a language server tool (definitions, references) when one is available, with Grep and Read as the fallback.

Every collision becomes a question for the owner in step 4. A contradiction with an existing SPEC.md rule is the owner's call, always.

## 4. Contract rounds

Work the open decisions as a **design tree**. Each round asks the whole **frontier**: every decision whose prerequisites are settled. Number each question, word it so "yes" accepts your recommendation, and give the recommendation with its basis: a `file:line`, a CLAUDE.md invariant, a SPEC.md rule, an outside source, or "judgment call". Carry the brief's **For /spec** items in as candidate questions. After each round, add the settled decisions to the brief.

Before drafting, run one **edge-case round**. Go through each class that applies to the feature and turn each into a decision:

- Malformed and boundary input: empty, missing, oversized, the exact limits, negative numbers.
- Overwrite and repeat behavior: doing it twice, doing it on something that doesn't exist.
- Interaction with the classes the existing SPEC.md already pins (for gocache: expiry, snapshots, the wire format).
- Concurrency: two clients at once.
- Shutdown and failure: mid-command, mid-write, after a restart.
- The exact output for every case above.

The rounds are done when every frontier is empty and every collision from step 3 has a decision.

## 5. Draft and approve

1. Draft the SPEC.md section in the existing style: a phase heading, numbered rules, exact outputs. Each rule is one testable statement.
2. Draft DECISIONS.md lines only for rules that would look arbitrary without their reason. Rationale for obvious rules stays out.
3. Show the owner the exact text of both, and where each goes.
4. Apply the owner's edits and show the changed text again, until they approve.
5. On approval, write the text into SPEC.md and DECISIONS.md. Leave committing to the owner.
6. Delete the brief.
7. Tell the owner the section is ready for `/new-track <section>`.
