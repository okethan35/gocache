# /rework outcomes

Phase 3 of [SKILL.md](SKILL.md). Apply the shared rules, then the chosen outcome's steps in order.

## Shared rules

**Order.** A merged spec change (rethink only) comes first, then new issues, then dependent edits, then closing the old issue and PR, then the LESSONS entry. Closing the old issue any earlier unblocks its dependents: the writer treats any closed blocker as done.

**New issues.** Draft with `/new-track`'s issue template (`.claude/skills/new-track/SKILL.md`, "Draft each issue") and run its overlap check ("Check overlap"), leaving the old issue out of the overlap check: it shares every file by design. Add to each:
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

Publish in blocker order with `gh issue create --title T --body-file F --label QUEUED`, replacing `T1`, `T2`, ... with real numbers as they are created.

**Carry-over**, for an issue whose agreed base is the old branch. The `QUEUED` label lets the reconciler start a writer at once, so the branch must exist before the label does:
1. Create the issue without `--label`.
2. Push the old branch's head under the new number: `git fetch origin OLD_BRANCH && git push origin FETCH_HEAD:refs/heads/issue-NEW`. The branch is exactly `issue-NEW`, with no slug: writers and reviewers accept nothing else.
3. Add the label: `gh issue edit NEW --add-label QUEUED`.

The writer finds the branch in its Step 3, merges `MAIN` into it, and opens a new PR. A carried-over issue keeps the old issue's `**Blocked by:**` line unless the owner changed it in phase 2. In a split, only the piece the owner named carries over.

**Dependents.** Edit a dependent only while it carries no `WRITING`, `REVIEW_QUEUED` or `REVIEWING` label. For one that does, list it to the owner and leave it alone, and do not close the old issue until the owner decides what happens to it (rule 58): closing the old issue would unblock it while it still points at the failed work. Edit bodies with `gh issue edit N --body-file F`.

**Pausing a dependent.** Remove `QUEUED` (the reconciler only starts `QUEUED` issues) and comment `Paused: blocked by #OLD, which is being reworked. Re-add QUEUED once its replacement is settled.` If the removal returns 404, the reconciler just claimed it: tell the owner.

**Closing.** `gh issue close OLD --reason "not planned" --comment "<outcome line>"`. Close the old PR with `gh pr close P --comment "Superseded: see #OLD."` and keep its branch.

**Branches.** Make every branch this skill creates in a separate worktree from the default branch, never in the owner's checkout: `git fetch origin && git worktree add ../rework-OLD-<kind> -b rework-OLD-<kind> origin/MAIN`. Stage only the target file, and remove the worktree (`git worktree remove`) after pushing.

**Guardrails.** Leave `.claude/` and `.github/` untouched, and never start a comment with `**Decision:**`.

## Reissue

1. Create the new issue.
2. In each dependent, replace `#OLD` with `#NEW` in `**Blocked by:**`.
3. Close the old issue (`Superseded by #NEW.`) and its PR.
4. LESSONS entry.

## Split

1. Create the pieces in blocker order, each with `**Supersedes:** #OLD`.
2. In each dependent, replace `#OLD` with every piece, unless the owner narrowed that in phase 2.
3. Close the old issue (`Split into #A, #B.`) and its PR.
4. LESSONS entry.

## Rethink

1. Pause every dependent.
2. Spec: on a branch `rework-OLD-spec` (see **Branches**), apply exactly the approved text to SPEC and nothing else. Commit, push, and open a PR. Ask the owner to merge it and wait. When the owner says merged, `git fetch origin` and confirm the text is on `origin/MAIN`. If it is not, say so and wait again.
3. Draft the new track against the merged spec and create it (shared rules).
4. Apply each dependent's phase 2 decision. **Kept**: point `**Blocked by:**` at the new issues and re-add `QUEUED`. **Reworked later**: leave it paused. **Folded**: close it as not planned, `Folded into #N.`
5. Close the old issue (`Reworked: spec PR #S, new track #A, #B.`) and its PR.
6. LESSONS entry.

## Shelve

1. Pause every dependent.
2. Close the old issue (`Shelved: <reason>. See LESSONS L<n>.`, with `n` computed from `origin/MAIN`'s copy of LESSONS) and its PR.
3. LESSONS entry, listing the paused dependents.

## LESSONS entry

Append one entry in the format at the top of LESSONS, plus any departure notes agreed in phase 1, on a branch `rework-OLD-lessons` (see **Branches**). Number it from `origin/MAIN`'s copy, not your working tree. Commit, push, open a PR, and give the owner the link to merge.
