#!/usr/bin/env bash
# PreToolUse hook entry point (Ralph Loop spec rules 13-15).
# Runs the default branch's copy of guard.py (origin/HEAD), so editing the local copy cannot
# switch the guard off; falls back to the working tree only when there is no remote copy yet
# (a fresh clone of a new repository). Uses the first Python that actually works, and blocks
# the call (exit 2) if none does or no guard can be found, so the guard never fails open.
dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="${CLAUDE_PROJECT_DIR:-$(cd "$dir/../.." && pwd)}"
guard="$dir/guard.py"
gitdir="$(git -C "$root" rev-parse --git-dir 2>/dev/null || true)"
if [ -n "$gitdir" ]; then
  case "$gitdir" in /*|?:*) ;; *) gitdir="$root/$gitdir" ;; esac
  # MSYS_NO_PATHCONV: Git Bash on Windows would otherwise rewrite "origin/HEAD:path" as a file path.
  sha="$(cd "$root" && MSYS_NO_PATHCONV=1 git rev-parse --verify -q "origin/HEAD:.claude/ralph/guard.py" 2>/dev/null || true)"
  if [ -n "$sha" ]; then
    cached="$gitdir/ralph-guard-$sha.py"
    if [ ! -s "$cached" ]; then
      git -C "$root" cat-file blob "$sha" > "$cached.tmp" 2>/dev/null && mv -f "$cached.tmp" "$cached"
    fi
    [ -s "$cached" ] && guard="$cached"
  fi
fi
[ -f "$guard" ] || { echo "Ralph guard: guard.py not found; blocked to stay safe" >&2; exit 2; }
for py in python3 python py; do
  if command -v "$py" >/dev/null 2>&1 && "$py" -c "" >/dev/null 2>&1; then
    exec "$py" "$guard" hook
  fi
done
echo "Ralph guard: no working Python found; blocked to stay safe" >&2
exit 2
