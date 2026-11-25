#!/usr/bin/env python3
"""Ralph Loop guard (.claude/ralph/SPEC.md rules 13-20).

Modes:
  guard.py hook        PreToolUse hook. Reads the tool call as JSON on stdin.
                       Exit 0 allows the call; exit 2 blocks it with a one-line
                       reason on stderr (rule 15).
  guard.py pr          The ralph-guard CI check (rules 17-19). Runs on
                       pull_request_target from the base branch and reads the PR
                       only through the GitHub API; it never runs PR code.
  guard.py lint [DIR]  Lint SKILL.md frontmatter under DIR (default .claude/skills).

Standard library only, except PyYAML for the frontmatter lint.
"""

import json
import os
import re
import shlex
import subprocess
import sys

ALWAYS_PROTECTED = [".claude/**", ".github/**", "**/CLAUDE.md"]
DEFAULT_CONFIG = {
    "default_branch": "main",
    "protected_paths": [],
    "test_globs": [],
    "spec_file": "SPEC.md",
    "decisions_file": "DECISIONS.md",
    "lessons_file": "LESSONS.md",
}
KNOWN_SKILL_FIELDS = {
    "name", "description", "when_to_use", "argument-hint", "arguments",
    "disable-model-invocation", "user-invocable", "allowed-tools",
    "disallowed-tools", "model", "effort", "context", "agent", "background",
    "hooks", "paths", "shell", "license", "compatibility", "metadata",
}
DECISION = "**Decision:**"
NUM = r"(?:\d+|\$\{?\w+\}?)"  # an issue/PR number, or a shell variable holding one


class Blocked(Exception):
    def __init__(self, rule, reason):
        super().__init__(f"Ralph guard (rule {rule}): {reason}")


# ---------- globs and config ----------

def glob_to_regex(pattern):
    """Gitignore-style glob for repo-relative paths: ** spans directories, * and ? do not."""
    out, i = "", 0
    while i < len(pattern):
        if pattern.startswith("**/", i):
            out += "(?:.*/)?"
            i += 3
        elif pattern.startswith("**", i):
            out += ".*"
            i += 2
        elif pattern[i] == "*":
            out += "[^/]*"
            i += 1
        elif pattern[i] == "?":
            out += "[^/]"
            i += 1
        else:
            out += re.escape(pattern[i])
            i += 1
    return re.compile(out + r"\Z")


def matches_any(path, patterns):
    path = path.replace("\\", "/")
    if path.startswith("./"):
        path = path[2:]
    return any(glob_to_regex(p).match(path) for p in patterns)


def protected_patterns(cfg):
    """Rule 4: config.protected_paths plus the paths that are always protected."""
    pats = list(ALWAYS_PROTECTED) + list(cfg.get("protected_paths") or [])
    for key in ("spec_file", "decisions_file", "lessons_file"):
        if cfg.get(key):
            pats.append(cfg[key])
    return pats


def _run(args, cwd=None):
    try:
        r = subprocess.run(args, cwd=cwd, capture_output=True, text=True, timeout=15)
        return r.stdout if r.returncode == 0 else None
    except (OSError, subprocess.TimeoutExpired):
        return None


def load_config(root):
    """Rule 2: config from the remote default branch. Falls back to the working tree
    only when no remote copy exists (for example a fresh local clone); the always-protected
    paths apply either way."""
    text = None
    for ref in ("origin/HEAD", "origin/main", "origin/master"):
        text = _run(["git", "show", f"{ref}:.claude/ralph/config.json"], cwd=root)
        if text:
            break
    if not text:
        try:
            with open(os.path.join(root, ".claude", "ralph", "config.json"), encoding="utf-8") as f:
                text = f.read()
        except OSError:
            text = None
    cfg = dict(DEFAULT_CONFIG)
    if text:
        try:
            cfg.update(json.loads(text))
        except ValueError:
            pass
    return cfg


# ---------- hook: Bash commands ----------

def split_segments(command):
    """Split a shell command line into simple commands on ; && || | and newlines."""
    return [s for s in re.split(r"&&|\|\||[;|\n]", command) if s.strip()]


def tokens(segment):
    try:
        return shlex.split(segment, posix=True)
    except ValueError:
        return segment.split()


def http_method(toks):
    for i, t in enumerate(toks):
        if t in ("-X", "--method") and i + 1 < len(toks):
            return toks[i + 1].upper()
        if t.startswith("--method="):
            return t.split("=", 1)[1].upper()
        if t.startswith("-X") and len(t) > 2:
            return t[2:].upper()
    if any(t in ("-f", "-F", "--field", "--raw-field", "--input", "-d", "--data") for t in toks):
        return "POST"
    return "GET"


def is_api_call(toks):
    joined = " ".join(toks)
    return (len(toks) >= 2 and toks[0] == "gh" and toks[1] == "api") or (
        toks and os.path.basename(toks[0]) in ("curl", "wget") and "api.github.com" in joined
    )


def read_text(path, cwd):
    try:
        with open(os.path.join(cwd, path), encoding="utf-8", errors="replace") as f:
            return f.read()
    except OSError:
        return ""


def comment_bodies(toks, cwd):
    """Bodies of a comment-posting command, as far as they can be read."""
    bodies = []
    for i, t in enumerate(toks):
        nxt = toks[i + 1] if i + 1 < len(toks) else ""
        if t in ("-f", "-F", "--field", "--raw-field") and nxt.startswith("body="):
            v = nxt[len("body="):]
            bodies.append(read_text(v[1:], cwd) if v.startswith("@") and t in ("-F", "--field") else v)
        elif t.startswith(("--field=body=", "--raw-field=body=")):
            bodies.append(t.split("body=", 1)[1])
        elif t in ("--body", "-b"):
            bodies.append(nxt)
        elif t.startswith("--body="):
            bodies.append(t.split("=", 1)[1])
        elif t == "--body-file" or (t == "-F" and not nxt.startswith("body=") and toks[:2] != ["gh", "api"]):
            bodies.append(read_text(nxt, cwd))
        elif t.startswith("--body-file="):
            bodies.append(read_text(t.split("=", 1)[1], cwd))
    return bodies


def posts_comment(toks):
    if len(toks) >= 3 and toks[0] == "gh" and toks[1] in ("issue", "pr") and toks[2] in ("comment", "review"):
        return True
    return is_api_call(toks) and http_method(toks) in ("POST", "PATCH") and any(
        re.search(r"/comments\b|/reviews\b", t) for t in toks
    )


RAW_MERGE = re.compile(r"\bgh\s+(?:(?:-R|--repo)[\s=]+\S+\s+)?pr\s+merge\b|pulls/[^\s/]*/+merge\b|mergePullRequest|enablePullRequestAutoMerge", re.I)


def check_raw(command):
    """Rule 13 on the raw text, so wrappers (bash -c, $(...), eval, env, xargs) cannot hide a merge."""
    if RAW_MERGE.search(command):
        raise Blocked(13, "agents never merge PRs; the owner merges in the GitHub UI")


def input_bodies(toks, cwd):
    """`gh api --input FILE` bodies, so a JSON payload cannot hide a Decision."""
    out = []
    for i, t in enumerate(toks):
        path = toks[i + 1] if t == "--input" and i + 1 < len(toks) else (t.split("=", 1)[1] if t.startswith("--input=") else None)
        if path and path != "-":
            try:
                body = json.loads(read_text(path, cwd) or "{}").get("body", "")
                out.append(body if isinstance(body, str) else "")
            except (ValueError, AttributeError):
                pass
    return out


def check_every_session(toks, cwd):
    """Rule 13: active locally and in the cloud."""
    joined = " ".join(toks)
    if len(toks) >= 3 and toks[0] == "gh" and toks[1] == "pr" and toks[2] == "merge":
        raise Blocked(13, "agents never merge PRs; the owner merges in the GitHub UI")
    if re.search(rf"pulls/{NUM}/merge\b", joined):
        raise Blocked(13, "agents never call the PR merge API")
    if posts_comment(toks):
        for body in comment_bodies(toks, cwd) + input_bodies(toks, cwd):
            if body.lstrip().startswith(DECISION):
                raise Blocked(13, f"agents never post a comment starting with {DECISION}")


SETTINGS_PATHS = re.compile(
    r"(?:^|/)(?:rulesets|branches/[^/\s]+/protection|actions/(?:secrets|variables|permissions)"
    r"|collaborators|hooks|keys|interaction-limits|environments|vulnerability-alerts"
    r"|automated-security-fixes|transfer|topics)\b"
)
WORKFLOW_RUNS = re.compile(r"(?:^|/)actions/(?:workflows/[^/\s]+/dispatches|runs/[^/\s]+/rerun)")
PULL_PATH = re.compile(rf"(?:^|/)pulls/{NUM}/?(?:\?|$)")
REPO_ROOT_PATH = re.compile(r"(?:^|/)repos/[^/\s?]+/[^/\s?]+/?(?:\?|$)")
ISSUE_OR_COMMENT = re.compile(rf"(?:^|/)(?:issues/{NUM}|issues/comments/{NUM}|pulls/comments/{NUM})/?(?:\?|$)")


def api_paths(toks):
    return [t.split("api.github.com/", 1)[-1] for t in toks[1:] if not t.startswith("-") and ("/" in t)]


def check_git_push(toks, default_branch, cwd):
    if "push" not in toks:
        return
    args = toks[toks.index("push") + 1:]
    for a in args:
        if a in ("--force", "-f", "--mirror") or a.startswith(("--force-with-lease", "--force-if-includes")):
            raise Blocked(14, "force pushes are not allowed")
        if re.fullmatch(r"-[a-zA-Z]*f[a-zA-Z]*", a):
            raise Blocked(14, "force pushes are not allowed")
    refspecs = [a for a in args if not a.startswith("-")][1:]  # first positional is the remote
    db = default_branch
    for r in refspecs:
        if r.startswith("+"):
            raise Blocked(14, "force pushes (+refspec) are not allowed")
        dst = r.split(":", 1)[-1]
        if dst in (db, f"refs/heads/{db}") or r == db:
            raise Blocked(14, f"agents never push to {db}")
    if not refspecs:
        current = (_run(["git", "symbolic-ref", "--short", "HEAD"], cwd=cwd) or "").strip()
        if current == db:
            raise Blocked(14, f"agents never push to {db} (current branch is {db})")


def check_cloud_only(toks, cfg, cwd):
    """Rule 14: active only when CLAUDE_CODE_REMOTE=true."""
    if not toks:
        return
    if toks[0] == "gh" and len(toks) >= 2 and toks[1] in ("repo", "auth"):
        raise Blocked(14, f"`gh {toks[1]}` is not allowed in cloud runs")
    if toks[0] == "gh" and len(toks) >= 2 and toks[1] == "workflow" and len(toks) >= 3 and toks[2] in ("run", "enable", "disable"):
        raise Blocked(14, "agents never start, enable or disable workflows")
    if toks[0] == "gh" and len(toks) >= 3 and toks[1] == "run" and toks[2] in ("rerun", "cancel", "delete"):
        raise Blocked(14, "agents never re-run, cancel or delete workflow runs")
    if toks[0] == "gh" and len(toks) >= 3 and toks[1] in ("issue", "pr") and toks[2] in ("edit", "delete", "close", "lock", "transfer"):
        raise Blocked(14, f"`gh {toks[1]} {toks[2]}` is not allowed in cloud runs")
    if toks[0] == "git" and "push" in toks:
        check_git_push(toks, cfg.get("default_branch") or "main", cwd)
    if is_api_call(toks):
        method = http_method(toks)
        for p in api_paths(toks):
            if SETTINGS_PATHS.search(p) and method != "GET":
                raise Blocked(14, "agents never change rulesets, branch protection or repository settings")
            if REPO_ROOT_PATH.search(p) and method != "GET":
                raise Blocked(14, "agents never change repository settings")
            if WORKFLOW_RUNS.search(p) and method != "GET":
                raise Blocked(14, "agents never dispatch or re-run workflows")
            if PULL_PATH.search(p) and method in ("PATCH", "PUT", "DELETE"):
                raise Blocked(14, "agents never edit pull requests (base, state or title)")
            if ISSUE_OR_COMMENT.search(p) and method in ("PATCH", "DELETE"):
                raise Blocked(14, "agents never edit or delete issues or comments")


def check_bash(command, cfg, cwd, remote):
    check_raw(command)
    for seg in split_segments(command):
        toks = tokens(seg)
        # Skip leading env assignments such as FOO=bar gh ...
        while toks and re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*=.*", toks[0]):
            toks = toks[1:]
        if not toks:
            continue
        check_every_session(toks, cwd)
        if remote:
            check_cloud_only(toks, cfg, cwd)


# ---------- hook: file tools ----------

def repo_relative(path, root):
    try:
        rel = os.path.relpath(os.path.abspath(path), os.path.abspath(root))
    except ValueError:  # different drive on Windows
        return None
    rel = rel.replace("\\", "/")
    return None if rel.startswith("../") or rel == ".." else rel


def check_file_tool(tool_input, cfg, root, remote):
    if not remote:
        return
    path = tool_input.get("file_path") or tool_input.get("notebook_path")
    if not path:
        return
    rel = repo_relative(path, root)
    if rel is not None and matches_any(rel, protected_patterns(cfg)):
        raise Blocked(14, f"{rel} is a protected path")


def hook_main(stdin_text, env):
    data = json.loads(stdin_text or "{}")
    tool = data.get("tool_name", "")
    tool_input = data.get("tool_input") or {}
    cwd = data.get("cwd") or os.getcwd()
    root = env.get("CLAUDE_PROJECT_DIR") or (_run(["git", "rev-parse", "--show-toplevel"], cwd=cwd) or cwd).strip()
    remote = env.get("CLAUDE_CODE_REMOTE") == "true"
    cfg = load_config(root)
    if tool in ("Bash", "PowerShell"):  # PowerShell runs the same gh and git commands on Windows
        check_bash(tool_input.get("command", ""), cfg, cwd, remote)
    elif tool in ("Edit", "Write", "MultiEdit", "NotebookEdit"):
        check_file_tool(tool_input, cfg, root, remote)


# ---------- lint ----------

def lint_skill(dirname, text):
    """Rules 19-20. Returns a list of problems for one SKILL.md."""
    import yaml

    problems = []
    if not text.startswith("---\n") and not text.startswith("---\r\n"):
        return ["frontmatter must start with --- on line 1"]
    parts = re.split(r"^---\s*$", text, maxsplit=2, flags=re.M)
    if len(parts) < 3:
        return ["frontmatter has no closing ---"]
    try:
        fm = yaml.safe_load(parts[1])
    except yaml.YAMLError as e:
        return [f"frontmatter is not valid YAML: {str(e).splitlines()[0]}"]
    if not isinstance(fm, dict):
        return ["frontmatter is not a mapping"]
    unknown = sorted(set(fm) - KNOWN_SKILL_FIELDS)
    if unknown:
        problems.append(f"unknown field(s): {', '.join(unknown)}")
    if fm.get("name") != dirname:
        problems.append(f"name {fm.get('name')!r} does not match directory {dirname!r}")
    if fm.get("disable-model-invocation") is not True:
        problems.append("disable-model-invocation must be true")
    return problems


def lint_dir(skills_dir):
    failures = []
    if not os.path.isdir(skills_dir):
        return failures
    for d in sorted(os.listdir(skills_dir)):
        p = os.path.join(skills_dir, d, "SKILL.md")
        if os.path.isfile(p):
            with open(p, encoding="utf-8") as f:
                failures += [f"{d}/SKILL.md: {m}" for m in lint_skill(d, f.read())]
    return failures


# ---------- pr: the ralph-guard check ----------

def check_pr_files(head_ref, files, cfg):
    """Rule 18. files: dicts with filename, status and optional previous_filename."""
    if not re.fullmatch(r"issue-\d+(?:-[a-z0-9-]+)?", head_ref or ""):
        return []
    failures = []
    prot = protected_patterns(cfg)
    tests = cfg.get("test_globs") or []
    for f in files:
        names = [f["filename"]] + ([f["previous_filename"]] if f.get("previous_filename") else [])
        for n in names:
            if matches_any(n, prot):
                failures.append(f"{n}: agent PRs may not change protected paths")
        if f.get("status") in ("modified", "removed", "renamed", "changed") and any(matches_any(n, tests) for n in names):
            failures.append(f"{f['filename']}: agent PRs may not modify, remove or rename an existing test ({f['status']})")
    return failures


def gh_json(path):
    out = subprocess.run(["gh", "api", "--paginate", "--slurp", path], capture_output=True, text=True, check=True).stdout
    pages = json.loads(out)
    return [x for page in pages for x in (page if isinstance(page, list) else [page])]


def pr_main():
    import base64

    repo = os.environ["GITHUB_REPOSITORY"]
    with open(os.environ["GITHUB_EVENT_PATH"], encoding="utf-8") as f:
        pr = json.load(f)["pull_request"]
    cfg = load_config(os.getcwd())  # default-branch checkout
    failures = []
    default = cfg.get("default_branch") or "main"
    if pr["base"]["ref"] != default:  # rule 17: only PRs into the default branch are judged
        failures.append(f"base branch is {pr['base']['ref']!r}; ralph-guard only passes PRs into {default!r}")
    files = gh_json(f"repos/{repo}/pulls/{pr['number']}/files?per_page=100")
    failures += check_pr_files(pr["head"]["ref"], files, cfg)

    sha = pr["head"]["sha"]
    tree = json.loads(subprocess.run(
        ["gh", "api", f"repos/{repo}/git/trees/{sha}?recursive=1"], capture_output=True, text=True, check=True
    ).stdout)
    for item in tree.get("tree", []):
        m = re.fullmatch(r"\.claude/skills/([^/]+)/SKILL\.md", item["path"])
        if m and item["type"] == "blob":
            blob = json.loads(subprocess.run(
                ["gh", "api", f"repos/{repo}/git/blobs/{item['sha']}"], capture_output=True, text=True, check=True
            ).stdout)
            text = base64.b64decode(blob["content"]).decode("utf-8", errors="replace")
            failures += [f"{m.group(1)}/SKILL.md: {p}" for p in lint_skill(m.group(1), text)]

    for line in failures:
        print(f"::error::{line}")
    print(f"ralph-guard: {len(failures)} problem(s)")
    return 1 if failures else 0


def main(argv):
    mode = argv[1] if len(argv) > 1 else ""
    if mode == "hook":
        try:
            hook_main(sys.stdin.read(), os.environ)
        except Blocked as b:
            print(str(b), file=sys.stderr)
            return 2
        except Exception as e:  # fail closed: an unreadable call is blocked, not allowed
            print(f"Ralph guard: could not check this call ({type(e).__name__}: {e}); blocked to stay safe", file=sys.stderr)
            return 2
        return 0
    if mode == "pr":
        return pr_main()
    if mode == "lint":
        failures = lint_dir(argv[2] if len(argv) > 2 else os.path.join(".claude", "skills"))
        for line in failures:
            print(line)
        return 1 if failures else 0
    print(__doc__, file=sys.stderr)
    return 64


if __name__ == "__main__":
    sys.exit(main(sys.argv))
