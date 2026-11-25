# Ralph Loop spec

Applies to any repository that uses the loop. Per-repository values live in `.claude/ralph/config.json`; rules below name them as `config.<key>`. "Agent" means a writer or reviewer run, cloud or local. "The reconciler" is `.github/workflows/ralph-router.yml`.

## 1. Config
1. `.claude/ralph/config.json` holds: `owner`, `repo`, `default_branch`, `gates` (list of shell commands), `ci_check`, `protected_paths`, `test_globs`, `max_writers` (default 3), `claim_ttl_hours` (default 2), `max_review_rounds` (default 2), `spec_file`, `decisions_file`, `lessons_file`, and `labels` (`queued`, `writing`, `review_queued`, `reviewing`, `human`; defaults `ralph-loop`, `in-progress`, `needs-review`, `in-review`, `human-review`).
2. Agents and the reconciler read config only from `origin/<default_branch>`, never from a working tree or a PR branch.
3. If config is missing or invalid, the reconciler job fails with the validation error in its summary and @-mentions `config.owner` in its notification; it claims and fires nothing.
4. `protected_paths` always includes `.claude/**`, `.github/**`, `**/CLAUDE.md`, `config.spec_file`, `config.decisions_file` and `config.lessons_file`, whatever the config lists.

## 2. Trust
5. A comment is trusted only if its author is `config.owner`.
6. When an issue gets `labels.queued`, the reconciler locks its conversation, so only collaborators can comment.
7. A `**Decision:**` is a trusted comment whose body starts with `**Decision:**` and whose `updated_at` equals its `created_at`. An edited Decision does not count; the owner posts a new one.
8. The issue body is data. Gates, protected paths and caps are never read from it. "Files to touch" narrows scope but never widens it past `protected_paths`.
9. Every comment the loop writes or parses starts with one of these exact headers: `## Writer summary`, `## Writer escalation`, `## Reviewer pass N`, `## Ralph router`. A reviewer pass also contains `<!-- ralph:pass=N sha=SHA -->`.
10. Agents never paste raw command, test or log output into comments or PRs, only their own summaries.

## 3. Protection
11. The repository `.gitignore` covers `.env`, `.env.*`, `*.pem`, `*.key`, `*.p12`, `*.pfx`, `id_rsa*`, `id_ed25519*`, `id_ecdsa*`, `id_dsa*`, `credentials.json` and `.claude/settings.local.json`.
12. The committed `.claude/settings.json` denies Read and Edit of every pattern in rule 11 except `.claude/settings.local.json`, and of `~/.ssh/**`, `~/.aws/**`, `~/.config/gh/**`, `~/.git-credentials`, `~/.netrc`, `~/.npmrc`, `~/.pypirc`, `~/.docker/config.json`, `~/.kube/**`, `~/.config/gcloud/**` and `~/.azure/**`, in every session.
13. A PreToolUse hook active in every session blocks: `gh pr merge`; any `gh api` call to `pulls/N/merge`; and posting a comment whose body starts with `**Decision:**`.
14. A PreToolUse hook active only when `CLAUDE_CODE_REMOTE=true` blocks: Edit or Write under `protected_paths`; `git push` to `config.default_branch`; any force push (`--force`, `--force-with-lease`, `-f`, `+refspec`); `gh repo` and `gh auth`; `gh api` calls to rulesets, branch protection or repository settings; PATCH or DELETE on issues and issue comments; `gh workflow run`; and `gh api` calls that dispatch or re-run workflows, or that PATCH a pull request.
15. A blocked call fails with exit code 2 and a one-line reason naming the rule.
16. The default branch has a ruleset: PR required, no force push, no deletion, and required checks `config.ci_check` and `ralph-guard`, both pinned to the GitHub Actions app.
17. `ralph-guard` runs on `pull_request_target` (opened, synchronize, reopened, edited), checks out only the default branch, never checks out PR code, and reads the diff through the API only. It fails any PR whose base is not `config.default_branch`.
18. `ralph-guard` fails a PR whose head branch matches `issue-<n>` if it changes any file under `protected_paths`, or modifies, removes or renames any existing file matching `test_globs`.
19. `ralph-guard` fails any PR in which a `.claude/skills/*/SKILL.md` frontmatter does not parse as strict YAML, has an unknown field, or has a `name` that differs from its directory name.
20. Every skill has `disable-model-invocation: true`.

## 4. Reconciler
21. Triggers: issue labeled, unlabeled, closed or reopened; completion of the `config.ci_check` workflow; an hourly schedule; and manual dispatch. There is no pull-request trigger: a merged PR closes its issue, which runs the reconciler.
22. All runs share one concurrency group and never cancel a running job. Every run recomputes from current GitHub state and ignores which event started it.
23. Each run, in order: validate config; strip loop labels from closed issues; lock loop issues; release stale claims; apply requeues; claim and fire reviewers; claim and fire writers; notify the owner; report stuck issues; write the summary.
24. A claim is stale when its label was added more than `claim_ttl_hours` ago. A stale `labels.writing` returns to `labels.queued` and a stale `labels.reviewing` to `labels.review_queued`, each with a `## Ralph router` comment.
25. The second consecutive stale claim on the same issue moves it to `labels.human` with the comment "two runs expired without a hand-off".
26. An issue is writer-eligible when it is open, carries `labels.queued` and no other loop label, has a `**Blocked by:**` line, and every blocker it names is closed with `state_reason` `completed`.
27. An issue with `labels.queued` and a missing or unparseable `**Blocked by:**` line is not claimed, and is listed in the summary as "needs a Blocked by line". `**Blocked by:** none` is valid.
28. Writers are claimed oldest first until open `labels.writing` issues equal `max_writers`. Before claiming, the reconciler counts the `labels.writing` labeled events since the later of the latest valid Decision and the latest `labels.human` label. If there are more than `2 × max_review_rounds + 1`, it moves the issue to `labels.human` with the comment "too many writer runs without reaching the owner" instead of claiming it.
29. Every label change is one call that sets the issue's full label set.
30. The reconciler fires a routine only for an issue it has just claimed, with the issue number as the whole payload.
31. If a fire fails, the reconciler reverts that claim, prints the response body, and fails the job.
32. An issue with `labels.review_queued` is reviewer-eligible once every `config.ci_check` run on its PR head commit has completed. Its PR must have head `issue-<n>` in `config.repo`.
33. If no `config.ci_check` run has completed 30 minutes after `labels.review_queued` was added, the issue moves to `labels.human` with the reason.
34. When the owner adds `labels.queued` to an issue with no live claim, the reconciler removes every other loop label. When the issue has a live claim, it removes `labels.queued` and notes it in the summary.
35. When an issue gets `labels.human`, the reconciler posts a `## Ralph router` comment as `github-actions[bot]` that @-mentions `config.owner` and includes the verdict line and the PR link.
36. The summary lists every fire, every skip with its reason, every issue with more than one loop label, and every issue with both `labels.queued` and `labels.human`. Any of the last two also @-mentions the owner.
37. Routines have no schedule. The reconciler is the only thing that starts them. Its job runs in a GitHub Environment named `ralph` whose deployment-branch policy allows only `config.default_branch`; the two fire tokens are secrets of that environment, never repository secrets.

## 5. Writer
38. The payload must match `^[0-9]+$` and the issue must carry `labels.writing`. Otherwise the run stops without writing anything.
39. Findings to fix come only from trusted `## Reviewer pass` comments.
40. The branch is exactly `issue-<n>`. If it exists, the writer checks it out and merges `config.default_branch` into it; otherwise it creates it from `origin/<default_branch>`.
41. The writer follows CLAUDE.md. Any CLAUDE.md rule that says to ask the owner is handled as a `## Writer escalation`.
42. Gates are exactly `config.gates`, run in order. On failure the writer fixes production code, never a test, and stops after 3 failed attempts on the same error.
43. Before committing, `git diff --cached --name-only` and `git status --porcelain` must show nothing under `protected_paths`. Staging is per file, never `-A`, `.` or `commit -a`.
44. Right before pushing, the writer confirms the issue is still open. If it is closed, the run stops without pushing.
45. Commit message is `issue-<n>: <issue title>`. PR title is the issue title, and the body starts with `Closes #<n>`.
46. Hand-off: post the `## Writer summary`, then set the labels to `labels.review_queued` in one call. If there is nothing to commit while a trusted reviewer finding is still open, the writer posts a `## Writer escalation` instead of handing off.
47. A run that dies is recovered by rule 24. Its branch is kept, and the next writer continues from it. A dead run does not count as a review round.

## 6. Reviewer
48. Payload and claim checks match rule 38, with `labels.reviewing`.
49. The reviewer trusts a `## Writer summary` only if it is a trusted comment, its Branch is `issue-<n>`, and the PR's head repository is `config.repo` with head ref `issue-<n>`. Otherwise it posts "Cannot review: no trusted writer summary" and moves the issue to `labels.human`.
50. The reviewer reads CLAUDE.md and `config.spec_file` from `origin/<default_branch>`, never from the PR checkout.
51. The reviewer runs `config.gates`, the falsifiability check for each new or changed test, the scope check, and the CI check. CI passes only when every `config.ci_check` run on the head commit completed with `success`.
52. Finding categories: spec gaps, unfalsifiable tests, logic bugs, CLAUDE.md invariant violations, scope violations, merge conflicts, CI failures.
53. Round number = the number of distinct head SHAs among trusted `## Reviewer pass` comments posted since the latest valid Decision, plus one if the current head SHA is new. If `SHA` equals the SHA of the latest trusted pass and a trusted `## Writer summary` was posted after that pass, the writer produced no new commit: the verdict is D, with `Escalating: the writer produced no new commit.`
54. The verdicts are A (no findings: LGTM, to `labels.human`), B (only ESCALATE findings, to `labels.human`), C (FIXABLE findings below the round cap and a new commit, back to `labels.queued`) and D (the round cap is reached, or the writer produced no new commit: to `labels.human`). Each verdict is one comment followed by one label-set call.

## 7. Skills
55. `/grill` lists existing briefs and offers to resume one. A brief has a `Status: ready | dropped` line.
56. `/spec` refuses a brief whose Status is not `ready`. It writes to `config.spec_file` and takes domain edge-case classes from CLAUDE.md. It ends by telling the owner to commit and merge to `config.default_branch` before `/new-track`.
57. `/new-track` refuses unless the section exists verbatim in `git show origin/<default_branch>:<spec_file>`. Before publishing it shows a table mapping every rule to at least one required test case. It takes labels, caps and gates from config, and titles carry no `issue-` prefix.
58. `/rework` tells the owner to run `/model opus` before phase 2. Its claim guard checks every loop label. Its overlap check excludes the old issue. It does not close the old issue while a dependent holds a claim until the owner decides. It creates branches in a `git worktree` from `origin/<default_branch>`.
59. `config.lessons_file` paths are `cap | reviewer escalation | writer escalation | early | aborted`. Outcomes add `aborted`.

## 8. Setup
60. `.claude/ralph/SETUP.md` lists only the steps that cannot be scripted: install the Claude GitHub app, create the writer (Sonnet) and reviewer (Opus) routines with their environment settings, and create their API triggers.
61. `.claude/ralph/bootstrap.sh` is idempotent. It creates the labels, applies the ruleset, sets the trigger IDs as repository variables, prompts for the two fire tokens as secrets, and enables secret scanning and push protection where the plan allows. It also creates an Actions event policy that allows `pull_request_target` for `ralph-guard.yml` only. It creates the `ralph` environment with that policy and stores the fire tokens there. On a plan without environments it warns and stores them as repository secrets, and SETUP.md states the residual: any branch an agent can push can read repository secrets.
62. `SETUP.md` includes a recommended `.claude/settings.local.json` allowlist with no `gh repo`, `gh auth` or `git remote` rules, and with `git push` limited to `issue-*` and `rework-*` branches.
