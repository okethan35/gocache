---
name: new-track
description: "Break an already-written, merged spec section into small Ralph Loop GitHub issues, check them for file overlap with open work, and after one confirmation publish them (the queue label starts the writers). Use when the owner says /new-track, \"queue this phase\", or \"turn this spec section into issues\"."
disable-model-invocation: true
argument-hint: <spec section>
---

# /new-track

Turns a spec section the owner has already written and merged into a track of small, independently reviewable Ralph Loop issues. This skill never writes or changes spec rules. If the section is missing, incomplete, or ambiguous, stop and say what is missing.

Argument: the spec section (heading or phase name). If none is given, ask which section.

**Config.** Read `.claude/ralph/config.json` from the default branch: `git fetch -q origin && git show origin/HEAD:.claude/ralph/config.json`. Below, `SPEC` is `config.spec_file`, `MAIN` is `config.default_branch`, `LESSONS` is `config.lessons_file`, the five loop labels are `config.labels` (`QUEUED` is `config.labels.queued`), and the writer cap is `config.max_writers`.

## 1. Read

- **The section, from `MAIN` only** (rule 57): `git show origin/MAIN:SPEC`. The section must be there word for word. If it exists only in your working tree, or differs from it, stop: "Commit and merge the spec section to MAIN first; writers start from MAIN and would not find it."
- All of that section. Number every rule if the spec numbers them; reuse those numbers.
- CLAUDE.md (conventions, module boundaries, testing rules).
- The code the section touches, enough to know which files each rule lands in.
- `LESSONS`: every entry for this section or its files. Turn each lesson into a required test case or a scope line in the drafts it affects.
- Open issues, for overlap: `gh issue list --state open --limit 100 --json number,title,labels,body`.

## 2. Slice

Split the section into issues. Each issue:
- Covers a contiguous group of spec rules that one writer run can finish and one reviewer can check (typically 1-4 files, one module boundary).
- Respects the module and package boundaries CLAUDE.md states. A change that crosses them usually becomes two issues linked by `Blocked by:`.
- Has a narrow "Files to touch" list. Unlisted files are out of scope for the writer.
- Never touches a protected path (`config.protected_paths`, `.claude/`, `.github/`, any `CLAUDE.md`, the spec, decisions and lessons files). Loop and spec changes are made by the owner, not the loop.

Order the issues so each one's blockers come first. Prefer fewer, independent chains over one long chain, so parallel writers have work.

## 3. Draft each issue in this exact format

Titles are plain descriptions ("EXPIRE and PERSIST wire commands"), never prefixed with `issue-` or a track name; the writer adds `issue-N:` to commits itself.

~~~
## Goal
<one or two sentences>

## Spec
SPEC **<section>, rules X-Y**. <Note any rules covered by other issues in this track.>

## Files to touch
- `<path>`: <what changes, which rules>
- The matching test files

## Tests
Write failing tests first, at least one per rule X-Y, then implement. Required cases at minimum:
- <concrete case -> expected result>

Do not modify existing tests. The loop runs the gates in `.claude/ralph/config.json`; do not list commands here.

## Out of scope
- <adjacent work belonging to another issue or a later phase>

**Blocked by:** none | #<n>, #<n>
~~~

Every issue needs the `**Blocked by:**` line, even when it is `none`: the reconciler will not start an issue without it.

Within the draft, refer to not-yet-created issues as `T1`, `T2`, ...; they are replaced with real numbers on publish.

## 4. Check overlap

For each draft, compare its "Files to touch" with every open issue that carries any of the five loop labels. For each shared file, record: draft, open issue number, file.

For each overlap, suggest either adding the open issue to `Blocked by:` (safe, serializes the work) or narrowing the draft's files. Never change a draft's blockers on your own; the owner decides.

## 5. Show and confirm once

Show the owner, in this order:
1. A table: ID, title, rules, files, blocked by.
2. **Rule coverage** (rule 57): a table with one row per rule in the section: rule number, the issue that covers it, and the required test case(s) that would fail if it were broken. Every rule needs at least one case. A rule about locking, concurrency or timing needs a case that fails without the protection (CLAUDE.md says how this project tests those). Fix any gap in the drafts before showing them.
3. The overlap warnings with suggestions, or "No overlap with open work."
4. Which issues will start immediately (unblocked; at most `config.max_writers` writers run at once, the rest queue).
5. The full body of each draft.

Then ask one question: publish as shown, publish with the suggested overlap blockers, or stop for edits. Apply any edits the owner asks for and show the changed drafts again. Do not create anything on GitHub before the owner says publish.

## 6. Publish

In order T1, T2, ...: replace `T<k>` references with the real issue numbers created so far, then
`gh issue create --title "<title>" --body-file <file> --label QUEUED`.

The `QUEUED` label lets the `ralph-router` reconciler start a writer for each issue; blocked issues and issues over the writer cap wait and are picked up as writers finish. Do not start routines yourself.

Finish with the list of created issue URLs, noting which are unblocked (starting now) and which are waiting on blockers.
