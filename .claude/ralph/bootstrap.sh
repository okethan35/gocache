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
# Temp file inside .git/: a relative path that both bash and a native Windows gh.exe can read.
tmp="$(mktemp "$(git rev-parse --git-dir)/ralph-bootstrap.XXXXXX")"; trap 'rm -f "$tmp"' EXIT
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

# ---- 2b. Actions event policy: allow pull_request_target for ralph-guard --
# GitHub blocks pull_request_target by default on public repositories from 2026-12-02
# unless an event policy allows it. ralph-guard is a required check and runs on that
# event, so without this policy no PR could ever merge. The policy targets only
# ralph-guard.yml and allows only that event (it is an allow-list for the workflow).
step "Actions event policy"
POLICY_NAME="Ralph Loop: allow pull_request_target for ralph-guard"
API_VER="X-GitHub-Api-Version: 2026-03-10"
printf '%s' "{\"name\":\"$POLICY_NAME\",\"enforcement\":\"active\",\"conditions\":{\"workflow_path\":{\"include\":[\".github/workflows/ralph-guard.yml\"],\"exclude\":[]}},\"rules\":[{\"type\":\"restrict_action_events\",\"parameters\":{\"allowed_events\":[\"pull_request_target\"]}}]}" > "$tmp"
pid="$(gh api -H "$API_VER" "repos/$REPO/actions/policies" --jq ".policies[] | select(.name==\"$POLICY_NAME\") | .id" 2>/dev/null | head -1 || true)"
if [ -n "$pid" ]; then
  echo "  update  '$POLICY_NAME' (id $pid)"
  run gh api -X PUT -H "$API_VER" "repos/$REPO/actions/policies/$pid" --input "$tmp" --silent
elif [ "$DRY" = 1 ]; then
  echo "  [dry-run] would create '$POLICY_NAME'"
elif gh api -X POST -H "$API_VER" "repos/$REPO/actions/policies" --input "$tmp" --silent 2>/dev/null; then
  echo "  create  '$POLICY_NAME'"
else
  echo "  warn    could not create the event policy. Public repos need it (Settings > Actions > Policies): allow pull_request_target for .github/workflows/ralph-guard.yml."
fi

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

# ---- 4. The `ralph` environment and the fire tokens (rule 37) ----------
# The tokens are secrets of an environment that only the default branch can deploy to,
# so a workflow an agent runs from another branch (workflow_dispatch --ref, on: push)
# cannot read them. Plans without environments fall back to repository secrets.
step "Environment 'ralph' (default branch only)"
DEFAULT_BRANCH="$(cfg default_branch)"
ENV_OK=0
printf '%s' '{"deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true}}' > "$tmp"
if [ "$DRY" = 1 ]; then
  echo "  [dry-run] create or update environment 'ralph' with a custom branch policy for '$DEFAULT_BRANCH'"
  ENV_OK=1
elif gh api -X PUT "repos/$REPO/environments/ralph" --input "$tmp" --silent 2>/dev/null; then
  ENV_OK=1
  have="$(gh api "repos/$REPO/environments/ralph/deployment-branch-policies" --jq '.branch_policies[] | .name + " " + (.type // "branch")' 2>/dev/null || true)"
  if printf '%s\n' "$have" | grep -qx "$DEFAULT_BRANCH branch"; then
    echo "  ok      environment 'ralph', deployable only from '$DEFAULT_BRANCH'"
  else
    printf '{"name":"%s","type":"branch"}' "$DEFAULT_BRANCH" > "$tmp"
    gh api -X POST "repos/$REPO/environments/ralph/deployment-branch-policies" --input "$tmp" --silent
    echo "  set     environment 'ralph', deployable only from '$DEFAULT_BRANCH'"
  fi
  extra="$(printf '%s\n' "$have" | grep -vx "$DEFAULT_BRANCH branch" | grep -v '^$' || true)"
  [ -n "$extra" ] && echo "  WARN    'ralph' also allows: $(echo "$extra" | paste -sd, -). Remove those branch policies."
else
  echo "  warn    environments are not available on this plan. The tokens go into repository secrets,"
  echo "          which any branch an agent can push can read (see SETUP.md, Residual risks)."
fi

step "Routine fire tokens"
if [ "$ENV_OK" = 1 ]; then where=(--env ralph); label="environment 'ralph'"; else where=(); label="repository"; fi
secrets="$(gh secret list --repo "$REPO" ${where[@]+"${where[@]}"} --json name --jq '.[].name' 2>/dev/null || true)"
repo_secrets="$(gh secret list --repo "$REPO" --json name --jq '.[].name')"
for role in WRITER REVIEWER; do
  sec="RALPH_${role}_FIRE_TOKEN"
  if printf '%s\n' "$secrets" | grep -qxF "$sec"; then
    echo "  ok      $sec in $label (to replace it: gh secret set $sec --repo $REPO ${where[*]+${where[*]}})"
  elif [ "$DRY" = 1 ]; then
    echo "  [dry-run] would ask for $sec and store it in $label"
  else
    read -r -s -p "  $(printf %s "$role" | tr "[:upper:]" "[:lower:]") routine fire token (input hidden): " tok; echo
    [ -n "$tok" ] || { echo "bootstrap: empty token" >&2; exit 1; }
    printf '%s' "$tok" | gh secret set "$sec" --repo "$REPO" ${where[@]+"${where[@]}"}
    unset tok
    echo "  set     $sec in $label"
  fi
  if [ "$ENV_OK" = 1 ] && printf '%s\n' "$repo_secrets" | grep -qxF "$sec"; then
    echo "  WARN    a repository-level $sec still exists and is readable from any branch."
    echo "          After the next reconciler run fires successfully, delete it: gh secret delete $sec --repo $REPO"
  fi
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
