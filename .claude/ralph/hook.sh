#!/usr/bin/env bash
# PreToolUse hook entry point (Ralph Loop spec rules 13-15).
# Runs guard.py with the first Python that actually works, and blocks the call
# (exit 2) if none does, so a missing interpreter never silently disables the guard.
dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
for py in python3 python py; do
  if command -v "$py" >/dev/null 2>&1 && "$py" -c "" >/dev/null 2>&1; then
    exec "$py" "$dir/guard.py" hook
  fi
done
echo "Ralph guard: no working Python found; blocked to stay safe" >&2
exit 2
