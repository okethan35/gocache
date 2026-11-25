#!/usr/bin/env bash
# Ralph Loop bootstrap (.claude/ralph/SPEC.md rule 61). Safe to run again: every step
# checks what is already there and only creates or updates what is missing or different.
#
#   bash .claude/ralph/bootstrap.sh            apply
#   bash .claude/ralph/bootstrap.sh --dry-run  print what it would do, change nothing
#
# Needs: bash, git, python3 (or python), and `gh` logged in as a repo admin.
# Run it yourself, from the repo root, after the manual steps in SETUP.md.
# It never prints a token.
set -euo pipefail
export MSYS_NO_PATHCONV=1 # Git Bash on Windows: don't rewrite API paths

DRY=0
[ "${1:-}" = "--dry-run" ] && DRY=1

PY=""
for p in python3 python py; do
  if command -v "$p" >/dev/null 2>&1 && "$p" -c "" >/dev/null 2>&1; then PY="$p"; break; fi
done
[ -n "$PY" ] || { echo "bootstrap: no working Python found" >&2; exit 1; }
command -v gh >/dev/null || { echo "bootstrap: gh is not installed" >&2; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "bootstrap: run 'gh auth login' first" >&2; exit 1; }

CONFIG=".claude/ralph/config.json"
[ -f "$CONFIG" ] || { echo "bootstrap: run from the repo root ($CONFIG not found)" >&2; exit 1; }

cfg() { "$PY" -c 'import json,sys; c=json.load(open(sys.argv[1])); v=c
for k in sys.argv[2].split("."): v=v[k]
print(v)' "$CONFIG" "$1"; }

REPO="$(cfg repo)"
CI_CHECK="$(cfg ci_check)"
run() { if [ "$DRY" = 1 ]; then echo "  [dry-run] $*"; else "$@"; fi; }
step() { echo; echo "== $*"; }

echo "Ralph Loop bootstrap for $REPO$([ "$DRY" = 1 ] && echo ' (dry run)')"
perm="$(gh api "repos/$REPO" --jq '.permissions.admin' 2>/dev/null || echo false)"
[ "$perm" = "true" ] || { echo "bootstrap: your gh login is not an admin of $REPO" >&2; exit 1; }

# ---- 1. Labels -------------------------------------------------------------
step "Labels"
existing="$(gh label list --repo "$REPO" --limit 200 --json name --jq '.[].name')"
while IFS='|' read -r key color desc; do
  name="$(cfg "labels.$key")"
  if printf '%s\n' "$existing" | grep -qxF "$name"; then
    echo "  ok      $name"
  else
    echo "  create  $name"
    run gh label create "$name" --repo "$REPO" --color "$color" --description "$desc"
  fi
done <<'EOF'
queued|0075ca|Ralph Loop: queued for a writer
writing|5319e7|Ralph Loop: a writer run holds this issue
review_queued|e4e669|Ralph Loop: writer done, waiting for CI and a reviewer
reviewing|fbca04|Ralph Loop: a reviewer run holds this issue
human|d93f0b|Ralph Loop: the owner's turn
EOF

# ---- 2. Ruleset on the default branch (rule 16) --------------------------
step "Ruleset"
RULESET_NAME="Ralph Loop: default branch"
desired="$("$PY" - "$RULESET_NAME" "$CI_CHECK" <<'EOF'
import json, sys
name, ci = sys.argv[1], sys.argv[2]
actions_app = 15368  # GitHub Actions: only checks it reports can satisfy the rule
print(json.dumps({
    "name": name, "target": "branch", "enforcement": "active", "bypass_actors": [],
    "conditions": {"ref_name": {"include": ["~DEFAULT_BRANCH"], "exclude": []}},
    "rules": [
        {"type": "deletion"},
        {"type": "non_fast_forward"},
        {"type": "pull_request", "parameters": {
            "required_approving_review_count": 0, "dismiss_stale_reviews_on_push": False,
            "require_code_owner_review": False, "require_last_push_approval": False,
            "required_review_thread_resolution": False}},
        {"type": "required_status_checks", "parameters": {
            "strict_required_status_checks_policy": False,
            "required_status_checks": [
                {"context": ci, "integration_id": actions_app},
                {"context": "ralph-guard", "integration_id": actions_app}]}},
    ],
}))
EOF
)"
rid="$(gh api "repos/$REPO/rulesets" --jq ".[] | select(.name==\"$RULESET_NAME\") | .id" | head -1)"
tmp="$(mktemp)"; trap 'rm -f "$tmp"' EXIT
printf '%s' "$desired" > "$tmp"
if [ -n "$rid" ]; then
  echo "  update  '$RULESET_NAME' (id $rid)"
  run gh api -X PUT "repos/$REPO/rulesets/$rid" --input "$tmp" --silent
else
  echo "  create  '$RULESET_NAME'"
  run gh api -X POST "repos/$REPO/rulesets" --input "$tmp" --silent
fi
others="$(gh api "repos/$REPO/rulesets" --jq ".[] | select(.name!=\"$RULESET_NAME\") | .name")"
[ -n "$others" ] && echo "  note    other rulesets also apply: $(echo "$others" | paste -sd, -). Remove any that duplicate this one."

# ---- 3. Routine IDs as repository variables ------------------------------
step "Routine trigger IDs (repository variables)"
vars="$(gh variable list --repo "$REPO" --json name --jq '.[].name' 2>/dev/null || true)"
for role in WRITER REVIEWER; do
  var="RALPH_${role}_ROUTINE_ID"
  if printf '%s\n' "$vars" | grep -qxF "$var"; then
    echo "  ok      $var"
    continue
  fi
  if [ "$DRY" = 1 ]; then echo "  [dry-run] would ask for $var and set it"; continue; fi
  read -r -p "  $(printf %s "$role" | tr "[:upper:]" "[:lower:]") routine trigger ID (trig_...): " id
  [[ "$id" =~ ^trig_[A-Za-z0-9]+$ ]] || { echo "bootstrap: that does not look like a trigger ID" >&2; exit 1; }
  gh variable set "$var" --repo "$REPO" --body "$id"
  echo "  set     $var"
done

# ---- 4. Fire tokens as Actions secrets -----------------------------------
step "Routine fire tokens (Actions secrets)"
secrets="$(gh secret list --repo "$REPO" --json name --jq '.[].name')"
for role in WRITER REVIEWER; do
  sec="RALPH_${role}_FIRE_TOKEN"
  if printf '%s\n' "$secrets" | grep -qxF "$sec"; then
    echo "  ok      $sec (to replace it: gh secret set $sec --repo $REPO)"
    continue
  fi
  if [ "$DRY" = 1 ]; then echo "  [dry-run] would ask for $sec and store it"; continue; fi
  read -r -s -p "  $(printf %s "$role" | tr "[:upper:]" "[:lower:]") routine fire token (input hidden): " tok; echo
  [ -n "$tok" ] || { echo "bootstrap: empty token" >&2; exit 1; }
  printf '%s' "$tok" | gh secret set "$sec" --repo "$REPO"
  unset tok
  echo "  set     $sec"
done

# ---- 5. Secret scanning and push protection -------------------------------
step "Secret scanning and push protection"
state="$(gh api "repos/$REPO" --jq '[.security_and_analysis.secret_scanning.status, .security_and_analysis.secret_scanning_push_protection.status] | join(" ")' 2>/dev/null || echo "unknown unknown")"
if [ "$state" = "enabled enabled" ]; then
  echo "  ok      both enabled"
else
  echo "  enable  (now: $state)"
  printf '%s' '{"security_and_analysis":{"secret_scanning":{"status":"enabled"},"secret_scanning_push_protection":{"status":"enabled"}}}' > "$tmp"
  if [ "$DRY" = 1 ]; then
    echo "  [dry-run] gh api -X PATCH repos/$REPO --input <secret scanning settings>"
  elif ! gh api -X PATCH "repos/$REPO" --input "$tmp" --silent 2>/dev/null; then
    echo "  warn    could not enable it; private repos may need GitHub Secret Protection. The .gitignore and deny rules still apply."
  fi
fi

# ---- 6. Checks the loop depends on ---------------------------------------
step "Checks"
wf="$(gh api "repos/$REPO/actions/workflows" --jq '.workflows[].name')"
for w in ci ralph-router ralph-guard ralph-selftest; do
  if printf '%s\n' "$wf" | grep -qxF "$w"; then echo "  ok      workflow '$w'"; else echo "  MISSING workflow '$w' (see SETUP.md; 'ci' must be the name the router's workflow_run waits for)"; fi
done
grep -q '"hooks"' .claude/settings.json 2>/dev/null && echo "  ok      .claude/settings.json has the guard hook" || echo "  MISSING .claude/settings.json guard hook"
grep -qx '\*.sh text eol=lf' .gitattributes 2>/dev/null && echo "  ok      .gitattributes keeps *.sh as LF" || echo "  MISSING '*.sh text eol=lf' in .gitattributes (the hook breaks on Windows without it)"

echo
echo "Done. Next: the verification steps at the end of .claude/ralph/SETUP.md."
