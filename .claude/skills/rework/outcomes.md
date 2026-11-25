# /rework outcomes

Phase 3 of [SKILL.md](SKILL.md). Apply the shared rules, then the chosen outcome's steps in order.

## Shared rules

**Order.** A merged spec change (rethink only) comes first, then new issues, then dependent edits, then closing the old issue and PR, then the LESSONS.md entry. Closing the old issue any earlier unblocks its dependents: the writer treats any closed blocker as done.

**New issues.** Draft with `/new-track`'s issue template (`.claude/skills/new-track/SKILL.md`, section 3) and run its overlap check (section 4). Add to each:
- `**Supersedes:** #OLD` on the line after `**Blocked by:**`.
- Every unresolved reviewer finding, as a required case under `## Tests`.
- An `## Approach` section between `## Tests` and `## Out of scope`:

  ```
  ## Approach
  Hypotheses from earlier attempts. A different approach is fine if it passes every gate and holds up; report which in your writer summary.
  - Do: <approach and why, with file:line of code or tests to model on>
  - Avoid: <approach and why it failed>
  ```

  The reviewer enforces tests, not `## Approach`. Every Avoid that must hold also gets a required test case that fails if the writer does it anyway.

Publish in blocker order with `gh issue create --title T --body-file F --label ralph-loop`, replacing `T1`, `T2`, ... with real numbers as they are created.

**Dependents.** Edit a dependent only while it carries no `in-progress`, `needs-review` or `in-review` label. For one that does, list it to the owner and leave it alone. Edit bodies with `gh issue edit N --body-file F`.

**Pausing a dependent.** Remove `ralph-loop` (writers only take `ralph-loop` issues) and comment `Paused: blocked by #OLD, which is being reworked. Re-add ralph-loop once its replacement is settled.`

**Closing.** `gh issue close OLD --reason "not planned" --comment "<outcome line>"`. Close the old PR with `gh pr close P --comment "Superseded: see #OLD."` and keep its branch.

**Guardrails.** Leave `.claude/` and `.github/` untouched, and never start a comment with `**Decision:**`.

## Reissue

1. Create the new issue.
2. In each dependent, replace `#OLD` with `#NEW` in `**Blocked by:**`.
3. Close the old issue (`Superseded by #NEW.`) and its PR.
4. LESSONS.md entry.

## Split

1. Create the pieces in blocker order, each with `**Supersedes:** #OLD`.
2. In each dependent, replace `#OLD` with every piece, unless the owner narrowed that in phase 2.
3. Close the old issue (`Split into #A, #B.`) and its PR.
4. LESSONS.md entry.

## Rethink

1. Pause every dependent.
2. Spec: on a branch `rework-OLD-spec` from `origin/main`, apply exactly the approved text to SPEC.md and nothing else. Commit, push, and open a PR. Ask the owner to merge it and wait. When the owner says merged, `git fetch origin` and confirm the text is on `origin/main`. If it is not, say so and wait again.
3. Draft the new track against the merged spec and create it (shared rules).
4. Apply each dependent's phase 2 decision. **Kept**: point `**Blocked by:**` at the new issues and re-add `ralph-loop`. **Reworked later**: leave it paused. **Folded**: close it as not planned, `Folded into #N.`
5. Close the old issue (`Reworked: spec PR #S, new track #A, #B.`) and its PR.
6. LESSONS.md entry.

## Shelve

1. Pause every dependent.
2. Close the old issue (`Shelved: <reason>. See LESSONS.md L<n>.`) and its PR.
3. LESSONS.md entry, listing the paused dependents.

## LESSONS.md entry

Append one entry in the format at the top of LESSONS.md, plus any departure notes agreed in phase 1, on a branch `rework-OLD-lessons` from `origin/main`. Commit, push, open a PR, and give the owner the link to merge.
