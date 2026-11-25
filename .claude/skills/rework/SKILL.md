---
name: rework
description: Debrief a Ralph Loop issue the loop could not finish, decide with the owner how to rework it (reissue, split, rethink, shelve), then carry it out on GitHub.
disable-model-invocation: true
model: opus
effort: high
argument-hint: <issue number>
---

# /rework

The owner runs this on an issue the Ralph Loop could not finish. You are the reasoning layer above the loop: the writer and reviewer receive conclusions from you, never open problems. Three phases, in order. GitHub and the repo stay untouched until the owner says go at the end of phase 2.

Argument: the issue number. If none is given, ask for it.

## Phase 1: Debrief

Gather everything first (`gh issue`, `gh pr` and `gh api` all work here):
- The issue body, every comment, its labels, and its label events (`gh api repos/okethan35/gocache/issues/N/events?per_page=100`).
- Every PR named in a `## Writer summary`: its diff and state.
- CLAUDE.md, and the SPEC.md section the issue names.
- LESSONS.md: every entry touching the same spec section or files.
- The **lineage**: follow `**Supersedes:** #M` lines back to the root issue, and read each ancestor's LESSONS.md entry. If a descendant's writer summary reports `**Approach:** departed`, and its entry has no departure note yet, offer to add one in phase 3.
- **Dependents**: open issues whose `**Blocked by:**` line names this issue, with their labels.

In parallel, dispatch one Explore subagent (very thorough) on the code the issue touches: its state on main and on the issue branch, how neighbouring code and tests already solve the same problem (tests a writer could model on), and anything in the codebase that makes the ask hard. Ask for a summary with file:line references, not file dumps.

Then report to the owner, in this order:
1. **Path.** How the issue got here: the 2-round cap, a reviewer ESCALATE, a `## Writer escalation`, still in the loop, or LGTM. Never refuse; advise. An LGTM issue probably means the wrong number, so say so. A claimed issue (`in-progress` or `in-review`): say which agent holds it and since when, and that phase 3 waits for the claim to clear. An escalation: say whether one `**Decision:**` comment plus re-queueing would be enough on its own.
2. **Lineage warning**, when the work has failed before: "Nth failure for this work (#a → #b). The last rework changed X; it failed again at Y." Lean toward rethink or shelve.
3. **What happened**: per round, what the writer did and what the reviewer found, a few lines each. End with the findings still unresolved.
4. **Diagnosis**: the root cause, stated as a claim with its evidence, which the owner may reject. Name one: the issue under-specified (a missing required case, vague scope, wrong files list), the spec ambiguous or wrong, the task too big for one run, a loop prompt weakness (writer.md or reviewer.md: say exactly which line, the owner fixes it), or a writer miss on a well-specified issue.
5. **Options**: two or three concrete ways forward, each mapped to an outcome, with your recommendation.

The phase is done when the owner has the report. Draft nothing yet.

## Phase 2: Decide

Talk it through with the owner. The owner decides; you argue for what the evidence supports and say plainly when you disagree. The phase is done when the owner has chosen exactly one outcome and agreed its specifics:
- **reissue**: what changes in the issue.
- **split**: the pieces, with each one's rules, files and blockers.
- **rethink**: the exact SPEC.md text, and for each dependent whether it is kept, reworked later, or folded into the new track.
- **shelve**: the reason.

For reissue, split and rethink, also agree the `## Approach` hypotheses and the required test cases (format in [outcomes.md](outcomes.md)).

Then show everything phase 3 will do: each new issue body in full (for a reissue, as a diff against the old body), every dependent edit, every close, and the LESSONS.md entry. Ask once: go, or edit. Apply edits and show the changed parts again.

## Phase 3: Execute

Read [outcomes.md](outcomes.md) and follow the shared rules plus the section for the chosen outcome.

**Claim guard**: immediately before the first GitHub write, re-read the issue's labels. If it carries `in-progress` or `in-review`, a loop run holds it: stop, tell the owner, and wait. Releasing a claim is the owner's call; leave claim labels alone.

Finish with links to everything created, edited and closed, and which new issues start now versus wait on blockers.
