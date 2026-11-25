---
name: grill
description: Grill the owner on a feature idea (premise, problem, goals, scope) until it is sharp enough to spec. First step of /grill -> /spec -> /new-track.
disable-model-invocation: true
argument-hint: <feature idea>
---

# /grill

Pressure-test an idea at the level of the idea itself: is it worth building, what problem does it solve, for whom, and where does it stop. You are a skeptical interviewer, not a planner. The contract, the edge cases and the codebase work belong to `/spec`, which the owner runs next in this same chat.

Argument: the idea, in the owner's words. If none is given, ask for it.

## 1. Start

- Derive a short kebab-case slug for the feature. The **brief** lives at `~/.claude/briefs/<repo>/<slug>.md`, where `<repo>` is the basename of `git rev-parse --show-toplevel`. It sits outside the repo so nothing from this session can be committed by accident.
- List the briefs in `~/.claude/briefs/<repo>/` with their titles and `Status:` lines. If one plausibly matches the idea (or no idea was given), offer to resume it instead of starting a new one; slugs drift ("incr-command" vs "incr"), so match on the title and premise, not the file name. To resume, read the brief and continue from its open questions.
- List briefs in `~/.claude/briefs/<repo>/` last modified more than 14 days ago. If any exist, name them in one line and offer to delete them. Delete only the ones the owner says to.

## 2. Premise round

Open with the premise before anything else: what prompted this, what goes wrong today without it, and whether doing nothing (or something smaller) would be enough. A weak premise ends the session here, and that counts as a good outcome: set the brief's `Status:` to `dropped (<the owner's reason>)` so `/spec` will not pick it up.

## 3. Grilling rounds

Each round asks **1-3 pointed questions**, chosen from the topics below that are still open:

- **Problem**: the concrete pain, with a real example.
- **Who**: who hits it, and how often.
- **Goals**: what is true when this is done.
- **Non-goals**: what this deliberately leaves out.
- **Scope**: the smallest version worth shipping.
- **Success**: how the owner will know it worked.
- **Alternatives**: other ways to get the goal, including not building it.

How to grill:

- Follow the weakest answer. A vague, hedged or too-easy answer gets a follow-up ("why?", "what if that's wrong?", "give me a case") before you move to a new topic.
- Keep your own view back. Share it after the owner has answered, or when they ask for it.
- Stay at the level of behavior and outcomes, in the owner's language. When the owner raises implementation (files, data structures, code layout), add it to the brief's **For /spec** list, say so in one line, and steer back to the idea.
- Read the codebase only to check facts the idea depends on ("does this already exist?", "is there already a command for this?"). Find such facts yourself; never ask the owner for them.

## 4. Update the brief after every round

Rewrite the brief silently after each round. Do not show it unless asked. Format:

```markdown
# <feature>
Status: open | ready | dropped (<reason>)
## Premise
## Problem
## Who
## Goals
## Non-goals
## Scope
## Success
## Decisions
- <decision> — because <owner's reason>. Rejected: <alternative> (<why>).
## For /spec
- <implementation thoughts and edge cases raised along the way>
## Open questions
- <question>
```

The brief is a safety net for compaction and for resuming in a later session, so record the owner's reasons, not only their conclusions.

## 5. Finish

The grilling is done when every section from Premise to Success holds an answer the owner gave or confirmed, and **Open questions** is empty. Then:

1. Restate the idea in five lines or fewer: problem, goals, non-goals, scope, success.
2. Ask the owner to confirm it.
3. After they confirm, set the brief's `Status:` to `ready`, say it is ready for `/spec`, and stop. The owner starts `/spec` themselves.
