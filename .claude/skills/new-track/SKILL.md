---
name: new-track
description: Break an already-written SPEC.md section into small Ralph Loop GitHub issues, check them for file overlap with open work, and after one confirmation publish them (the ralph-loop label starts the writers). Use when the owner says /new-track, "queue this phase", or "turn this spec section into issues".
---

# /new-track

Turns a SPEC.md section the owner has already written into a track of small, independently reviewable `ralph-loop` issues. This skill never writes or changes spec rules. If the section is missing, incomplete, or ambiguous, stop and say what is missing.

Argument: the SPEC.md section (heading or phase name). If none is given, ask which section.

## 1. Read

- The named SPEC.md section, all of it. Number every rule if the spec numbers them; reuse those numbers.
- CLAUDE.md (conventions, package boundaries, testing philosophy).
- The code the section touches, enough to know which files each rule lands in.
- LESSONS.md: every entry for this section or its files. Turn each lesson into a required test case or a scope line in the drafts it affects.
- Open issues, for overlap: `gh issue list --state open --limit 100 --json number,title,labels,body`.

## 2. Slice

Split the section into issues. Each issue:
- Covers a contiguous group of spec rules that one writer run can finish and one reviewer can check (typically 1-4 files, one package boundary).
- Respects CLAUDE.md package rules (store has no networking, protocol has no store, and so on). A change that crosses packages usually becomes two issues linked by `Blocked by:`.
- Has a narrow "Files to touch" list. Unlisted files are out of scope for the writer.
- Never touches `.claude/` or `.github/`. Loop changes are made by the owner, not the loop.

Order the issues so each one's blockers come first. Prefer fewer, independent chains over one long chain, so parallel writers have work.

## 3. Draft each issue in this exact format

~~~
## Goal
<one or two sentences>

## Spec
SPEC.md **<section>, rules X-Y**. <Note any rules covered by other issues in this track.>

## Files to touch
- `<path>`: <what changes, which rules>
- The matching `_test.go` files

## Tests
Write failing tests first (CLAUDE.md testing philosophy), at least one per rule X-Y, then implement. Required cases at minimum:
- <concrete case -> expected result>

Do not modify existing tests.

Gate:
```
go test ./...
go test -race ./...
```

## Out of scope
- <adjacent work belonging to another issue or a later phase>

**Blocked by:** none | #<n>, #<n>
~~~

Within the draft, refer to not-yet-created issues as `T1`, `T2`, ...; they are replaced with real numbers on publish.

## 4. Check overlap

For each draft, compare its "Files to touch" with every open issue that has any of `ralph-loop`, `in-progress`, `needs-review`, `in-review`, `human-review`. For each shared file, record: draft, open issue number, file.

For each overlap, suggest either adding the open issue to `Blocked by:` (safe, serializes the work) or narrowing the draft's files. Never change a draft's blockers on your own; the owner decides.

## 5. Show and confirm once

Show the owner, in this order:
1. A table: ID, title, rules, files, blocked by.
2. The overlap warnings with suggestions, or "No overlap with open work."
3. Which issues will start immediately (unblocked; at most 3 writers run at once, the rest queue).
4. The full body of each draft.

Then ask one question: publish as shown, publish with the suggested overlap blockers, or stop for edits. Apply any edits the owner asks for and show the changed drafts again. Do not create anything on GitHub before the owner says publish.

## 6. Publish

In order T1, T2, ...: replace `T<k>` references with the real issue numbers created so far, then
`gh issue create --title "<title>" --body-file <file> --label ralph-loop`.

The `ralph-loop` label starts a writer for each issue through the `ralph-router` GitHub Action; blocked issues and issues over the 3-writer cap wait and are picked up as writers finish. Do not start routines yourself.

Finish with the list of created issue URLs, noting which are unblocked (starting now) and which are waiting on blockers.
