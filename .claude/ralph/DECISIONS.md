# Ralph Loop - decision log

- **Firing lives in a GitHub Action** (rules 21-37): routines cannot hold fire secrets (network secrets cannot target Anthropic hosts; commit fc63d68).
- **Claims are serialized in the reconciler, not raced by agents** (22, 28-30): v1 let three writers run on one issue (commit 35d77d1); v2's remove-first claim still left the cap racy, with fires one second apart in Phase 8; and 4 of 7 writer fires did nothing.
- **Per-issue lock instead of repository interaction limits** (6): interaction limits expire after at most six months and need an admin token to renew.
- **Trust by author, not by app provenance** (5): local manual runs post with no `performed_via_github_app`, so a provenance check would reject the owner's own fallback runs.
- **Decisions must be unedited** (7): owner and agents share one GitHub identity, and editing a comment keeps its author.
- **Merge blocking is in every session; path blocking is cloud-only** (13-14): the owner merges in the GitHub UI and never through Claude, but edits `.claude/` locally. Self-merge stays a residual risk, because an injected agent could call the API directly; a separate bot identity would remove it and was deferred.
- **Bash pattern rules are a speed bump; GitHub rules are the boundary** (14, 16-18): permission patterns match command text, so `git -C . push --force` slips past them.
- **Branches are exactly `issue-<n>`** (40): LLM-made slugs differed between runs (#7 got three), and trust checks need a predictable name.
- **A failed fire reverts its claim** (31): a claimed issue must never exist without a run behind it.
- **Two expiries escalate** (25): rejected runs (for example, at a usage limit) would otherwise claim and release forever.
- **Writer on Sonnet, reviewer on Opus** (60): issues are tightly specified and gated by CI; review is where judgment pays.
- **ralph-guard has its own Actions event policy** (21, 61): GitHub blocks `pull_request_target` by default on public repositories from 2026-12-02. The policy allows that event for `ralph-guard.yml` only, and the router dropped its pull-request trigger, so nothing else needs it.
