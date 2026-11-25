# Ralph Loop setup

How to add the Ralph Loop to a GitHub repository. It takes about 15 minutes, most of it in the routines UI. The contract is [SPEC.md](SPEC.md) and the reasons behind it are in [DECISIONS.md](DECISIONS.md).

## How the loop works

You plan and review; agents write and check the code.

1. `/grill <idea>` pressure-tests an idea into a brief.
2. `/spec` turns the brief into numbered rules in your spec file. You commit and merge it.
3. `/new-track <section>` slices the merged section into issues labeled `ralph-loop`.
4. The **reconciler** (`ralph-router` Action) claims each ready issue and starts the **writer** routine. The writer implements it on branch `issue-N`, opens a PR, and hands off.
5. After CI finishes, the reconciler starts the **reviewer** routine. It runs the gates, mutation-tests the new tests, and either sends the issue back for one fix round or hands it to you.
6. You get an @-mention on `human-review`. You review and merge the PR in the GitHub UI. Agents can never merge.
7. If the loop could not finish an issue, run `/rework <issue>`.

To re-queue an issue yourself, add `ralph-loop`; the reconciler clears the other loop labels. To give an instruction on an issue, post a comment starting with `**Decision:**` (do not edit it later; post a new one).

## What is in the loop

| Path | What it is |
|---|---|
| `.claude/ralph/` | Spec, decisions, `config.json`, the writer and reviewer prompts, the reconciler, the guard, their tests, this file and `bootstrap.sh` |
| `.claude/skills/{grill,spec,new-track,rework}/` | The owner skills |
| `.claude/settings.json` | Deny rules for secrets, and the guard hook |
| `.github/workflows/ralph-router.yml`, `ralph-guard.yml`, `ralph-selftest.yml` | Reconciler, PR guard, guard self-tests |
| `LESSONS.md` | What `/rework` learned (starts empty) |

Requirements: `bash`, `git`, `python3` (or `python`) and `gh`, locally and in the routine environment. The guard blocks every tool call if it cannot find a working Python, so it never fails open.

## 1. Copy the files

Copy everything in the table into the new repository, then add:

- `.gitignore`: `.env`, `.env.*`, `*.pem`, `*.key`, `id_*`, `.claude/settings.local.json`, `__pycache__/`
- `.gitattributes`: `*.sh text eol=lf` (without it, Windows checkouts break the hook)
- `CLAUDE.md`: your project's conventions, plus this pointer:

  ```markdown
  ## Agent workflow
  The Ralph Loop's contract, config and setup live in `.claude/ralph/` (SPEC.md, config.json, SETUP.md). Agents follow `.claude/ralph/SPEC.md`; this file holds project conventions only.
  ```

## 2. Edit `.claude/ralph/config.json`

| Key | Set it to |
|---|---|
| `owner` | Your GitHub login. Only your comments are trusted. |
| `repo` | `OWNER/NAME` |
| `default_branch` | Usually `main` |
| `gates` | The commands that must pass, for example `["npm test"]` or `["go test ./...", "go test -race ./..."]` |
| `ci_check` | The CI job name (step 3), usually `test` |
| `protected_paths` | Extra files agents may never change (dependency manifests, lockfiles). `.claude/`, `.github/`, `CLAUDE.md` and the three files below are always protected. |
| `test_globs` | Your test files, for example `["**/*_test.go"]` or `["**/*.test.ts"]`. Agent PRs may add tests but never change existing ones. |
| `spec_file`, `decisions_file`, `lessons_file` | Usually `SPEC.md`, `DECISIONS.md`, `LESSONS.md` |
| `max_writers`, `claim_ttl_hours`, `max_review_rounds` | `3`, `2`, `2` are good defaults |
| `labels` | Keep the defaults unless they clash with labels you already use |

## 3. CI

The repository needs a workflow whose `name:` is `ci`, with a job named the same as `config.ci_check`. Copy `.github/workflows/ci.yml` and replace only the steps under the job with your language's setup and tests. Keep `permissions: contents: read`, pin actions to commit SHAs, and trigger on `pull_request` plus `push` to the default branch only.

## 4. Manual steps (cannot be scripted)

1. **Install the Claude GitHub app** on the repository (only this repository, not all of them).
2. **Create the writer routine** in Claude Code routines:
   - Repository: this one. Model: **Sonnet**.
   - Prompt, exactly: `Follow .claude/ralph/writer.md exactly.`
   - Environment: network **Limited**, with package managers allowed. Add a setup script that installs your toolchain, for example the Go or Node version your project uses.
   - **No schedule.** The reconciler starts every run.
3. **Create the reviewer routine** the same way, with model **Opus** and prompt `Follow .claude/ralph/reviewer.md exactly.`
4. **Add an API trigger to each routine.** Copy each trigger ID (`trig_...`) and fire token. The token is shown once; keep it only until step 5.

## 5. Run the bootstrap

From the repository root:

```bash
bash .claude/ralph/bootstrap.sh --dry-run   # see what it will do
bash .claude/ralph/bootstrap.sh
```

It creates the loop labels and the default-branch ruleset (PR required, no force push or deletion, required checks `ci_check` and `ralph-guard` pinned to GitHub Actions). It also creates an Actions event policy that allows `pull_request_target` for `ralph-guard.yml` only: GitHub blocks that event by default on public repositories from December 2, 2026, and without it `ralph-guard` never runs, so no PR could merge. It asks for the two trigger IDs and stores them as repository variables. It creates the `ralph` environment, which only the default branch can use, and asks for the two fire tokens (input hidden) and stores them as secrets of that environment. It turns on secret scanning and push protection where your plan allows. Running it again changes only what is missing.

If the repository already has a ruleset on the default branch, the bootstrap says so; remove or merge the duplicate.

## 6. Recommended local settings

Your own `.claude/settings.local.json` is not committed. Keep its allowlist narrow, because `/rework` and `/new-track` read issue text in your session. Avoid allowing `gh repo`, `gh auth`, `git remote` or a bare `git push *`. For example:

```json
{
  "permissions": {
    "allow": [
      "Bash(git add *)",
      "Bash(git commit -m *)",
      "Bash(git push -u origin issue-*)",
      "Bash(git push -u origin rework-*)",
      "Bash(gh issue list *)",
      "Bash(gh issue view *)"
    ]
  }
}
```

Add your language's build and test commands (for example `Bash(go test *)`).

## 7. Verify

1. **Guard self-tests**: `python3 -m unittest discover -s .claude/ralph/tests` passes, and `python3 .claude/ralph/guard.py lint` prints nothing.
2. **Reconciler**: `python3 .claude/ralph/reconcile.py --dry-run` prints "nothing to do" on a repository with no loop issues.
3. **Cloud guard probe**: open a cloud session on the default branch, using the routines' environment, and run:

   ```
   This is a test of the repo's PreToolUse hooks. Run each command below exactly as written, one per tool call, and report whether each one ran or was blocked by a hook, quoting any hook message verbatim. Do not commit or push anything.
   1. echo "remote=$CLAUDE_CODE_REMOTE"
   2. gh pr merge 99999 --help
   3. git push --dry-run origin HEAD:main
   4. git push --dry-run --force origin HEAD:refs/heads/probe
   5. Use the Edit tool to add a blank line at the end of .claude/ralph/writer.md
   6. Use the Read tool to read .env
   ```

   Expected: 1 prints `remote=true`; 2 to 5 are blocked with `Ralph guard (rule 13)` or `(rule 14)`; 6 is denied by a permission rule.
4. **End to end**: open a tiny issue (for example a README line) with a `## Goal`, `## Files to touch`, and `**Blocked by:** none`, and add `ralph-loop`. Within a minute the `ralph-router` run should claim it and start the writer. Follow it through to your `human-review` @-mention.

## Residual risks

- **Repository-level Actions secrets are readable by any branch an agent can push.** An agent can push a branch with its own workflow, or run a workflow from its branch. Keep every secret you care about in an environment restricted to the default branch, like `ralph`, and none at repository level. On a private repository on GitHub Free, environments are not available: the bootstrap then stores the fire tokens as repository secrets, and this risk applies to them. Rotate a fire token (routines UI, then `gh secret set`) if you ever suspect it leaked.
- **Self-merge.** Agents act as you, so GitHub cannot require "someone else" to approve. Merges are blocked by the guard hook and by `ralph-guard`, but an injected agent calling the merge API in a way the hook does not recognise is not fully preventable without a separate bot account for the agents.
- **Single maintainer.** The loop trusts comments, labels and Decisions from `config.owner` only. Anyone you give write access to can label issues, edit issue bodies and push to `issue-N` branches, so treat collaborators as owner-equivalent or do not add any.

## Troubleshooting

- **The reconciler run fails with "routine ID or fire token is not configured"**: rerun the bootstrap.
- **A review never starts**: the router's `workflow_run` names the CI workflow `ci`; check that your CI workflow's `name:` matches, and that the job name equals `config.ci_check`.
- **A PR waits on `ralph-guard` forever**: either the guard workflow is not on the default branch yet (it does not run on the PR that first adds it; merge that PR before running the bootstrap, which makes `ralph-guard` required), or `pull_request_target` is blocked for it. Check Settings > Actions > Policies and Policy insights; rerun the bootstrap to create the policy.
- **Everything is blocked locally with "no working Python found"**: install Python 3, or make `python3` resolve to a real interpreter (on Windows the Store alias does not count).
