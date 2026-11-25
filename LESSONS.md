# Lessons

What went wrong when the Ralph Loop could not finish an issue, and what changed because of it. `/rework` appends one entry per run, whatever the outcome; `/rework` and `/new-track` read it before drafting issues. Entries are append-only: correct one by adding a note under it.

## Entry format

```
## L<n>: #<issue> <short title> (YYYY-MM-DD)
**Path:** cap | reviewer escalation | writer escalation | early
**Outcome:** reissued as #N | split into #A, #B | rethink: spec PR #S, new track #A, #B | shelved
**Lineage:** #root -> ... -> #this (omit for a first failure)
**Spec:** SPEC.md <section>, rules X-Y; files: <paths>
**Failed because:** the root cause, one or two sentences.
**Changed:** what the replacement does differently, or why the work is shelved.
```

When a writer on the replacement reports `**Approach:** departed`, add under the entry:

```
**Departure (#N):** hypothesis was X; what worked was Y.
```

## Entries

None yet.
