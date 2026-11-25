#!/usr/bin/env python3
"""Ralph Loop reconciler (.claude/ralph/SPEC.md rules 3, 21-31, 37).

Run by .github/workflows/ralph-router.yml from the default branch. Every run
recomputes from current GitHub state and ignores which event started it (rule 22):

  validate config -> strip loop labels from closed issues -> release stale claims
  -> claim and fire reviewers -> claim and fire writers -> write the summary

The reconciler is the only thing that claims issues or starts routines.
Standard library only. `reconcile.py --dry-run` prints what it would do without writing.
"""

import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.request
from datetime import datetime, timezone

LABEL_KEYS = ("queued", "writing", "review_queued", "reviewing", "human")
ROUTER_HEADER = "## Ralph router"
EXPIRED_MARKER = "<!-- ralph:expired -->"
BOT = "github-actions[bot]"
FIRE_URL = "https://api.anthropic.com/v1/claude_code/routines/{id}/fire"


# ---------- config (rules 1-3) ----------

def validate_config(cfg):
    """Return a list of problems; empty means valid."""
    if not isinstance(cfg, dict):
        return ["config is not a JSON object"]
    problems = []
    for key in ("owner", "repo", "default_branch", "ci_check", "spec_file", "decisions_file", "lessons_file"):
        if not isinstance(cfg.get(key), str) or not cfg.get(key):
            problems.append(f"{key} must be a non-empty string")
    gates = cfg.get("gates")
    if not isinstance(gates, list) or not gates or not all(isinstance(g, str) and g for g in gates):
        problems.append("gates must be a non-empty list of commands")
    for key in ("protected_paths", "test_globs"):
        if not isinstance(cfg.get(key), list) or not all(isinstance(p, str) for p in cfg.get(key) or []):
            problems.append(f"{key} must be a list of strings")
    for key in ("max_writers", "claim_ttl_hours", "max_review_rounds"):
        v = cfg.get(key)
        if not isinstance(v, int) or isinstance(v, bool) or v < 1:
            problems.append(f"{key} must be a positive integer")
    labels = cfg.get("labels")
    if not isinstance(labels, dict) or any(not isinstance(labels.get(k), str) or not labels.get(k) for k in LABEL_KEYS):
        problems.append(f"labels must name all of: {', '.join(LABEL_KEYS)}")
    elif len(set(labels[k] for k in LABEL_KEYS)) != len(LABEL_KEYS):
        problems.append("labels must all be different")
    if isinstance(cfg.get("repo"), str) and not re.fullmatch(r"[\w.-]+/[\w.-]+", cfg["repo"]):
        problems.append("repo must look like OWNER/NAME")
    return problems


# ---------- GitHub access ----------

class GitHub:
    """Thin wrapper over `gh api`. Tests replace it with a fake.
    With dry_run, writes are printed instead of sent."""

    def __init__(self, repo, dry_run=False):
        self.repo, self.dry_run = repo, dry_run

    def _api(self, args, input_obj=None):
        r = subprocess.run(
            ["gh", "api"] + args,
            input=json.dumps(input_obj) if input_obj is not None else None,
            capture_output=True, text=True,
        )
        if r.returncode != 0:
            raise RuntimeError(f"gh api {' '.join(args)} failed: {r.stderr.strip() or r.stdout.strip()}")
        return json.loads(r.stdout) if r.stdout.strip() else None

    def _paged(self, path):
        pages = self._api(["--paginate", "--slurp", path])
        return [x for page in pages for x in page]

    def open_issues(self):
        items = self._paged(f"repos/{self.repo}/issues?state=open&per_page=100&sort=created&direction=asc")
        return [i for i in items if "pull_request" not in i]

    def closed_issues_with_label(self, label):
        items = self._paged(f"repos/{self.repo}/issues?state=closed&labels={label}&per_page=100")
        return [i for i in items if "pull_request" not in i]

    def issue(self, n):
        return self._api([f"repos/{self.repo}/issues/{n}"])

    def events(self, n):
        return self._paged(f"repos/{self.repo}/issues/{n}/events?per_page=100")

    def comments(self, n):
        return self._paged(f"repos/{self.repo}/issues/{n}/comments?per_page=100")

    def set_labels(self, n, labels):
        """Rule 29: one call that sets the issue's full label set."""
        if self.dry_run:
            print(f"[dry-run] #{n} labels -> {sorted(labels)}")
            return
        self._api(["-X", "PUT", f"repos/{self.repo}/issues/{n}/labels", "--input", "-"], {"labels": sorted(labels)})

    def comment(self, n, body):
        if self.dry_run:
            print(f"[dry-run] #{n} comment: {body.splitlines()[-1]}")
            return
        self._api(["-X", "POST", f"repos/{self.repo}/issues/{n}/comments", "--input", "-"], {"body": body})


def fire(routine_id, token, issue_number):
    """Rule 30: the issue number is the whole payload. Returns (ok, detail)."""
    if not routine_id or not token:
        return False, "routine ID or fire token is not configured"
    req = urllib.request.Request(
        FIRE_URL.format(id=routine_id),
        data=json.dumps({"text": str(issue_number)}).encode(),
        method="POST",
        headers={
            "Authorization": f"Bearer {token}",
            "anthropic-version": os.environ.get("RALPH_ANTHROPIC_VERSION", "2026-10-21"),
            "Content-Type": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return True, f"HTTP {resp.status}"
    except urllib.error.HTTPError as e:
        return False, f"HTTP {e.code}: {e.read().decode(errors='replace')[:500]}"
    except (urllib.error.URLError, TimeoutError) as e:
        return False, f"request failed: {e}"


# ---------- pure helpers ----------

def label_names(issue):
    return {l["name"] if isinstance(l, dict) else l for l in issue.get("labels", [])}


def parse_blocked_by(body):
    """Rule 27. Returns a list of issue numbers, [] for `none`, or None if missing/unparseable."""
    for line in (body or "").splitlines():
        if line.startswith("**Blocked by:**"):
            rest = line[len("**Blocked by:**"):].strip()
            if rest.lower() == "none":
                return []
            parts = [p for p in re.split(r"[,\s]+", rest) if p]
            if parts and all(re.fullmatch(r"#\d+", p) for p in parts):
                return [int(p[1:]) for p in parts]
            return None
    return None


def parse_time(s):
    return datetime.strptime(s, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=timezone.utc)


def last_labeled_at(events, label):
    times = [parse_time(e["created_at"]) for e in events if e.get("event") == "labeled" and (e.get("label") or {}).get("name") == label]
    return max(times) if times else None


def is_second_expiry(comments):
    """Rule 25: an expiry with no writer summary or reviewer pass since the previous expiry."""
    last_expiry = None
    for i, c in enumerate(comments):
        body = c.get("body") or ""
        if c.get("user", {}).get("login") == BOT and EXPIRED_MARKER in body:
            last_expiry = i
    if last_expiry is None:
        return False
    for c in comments[last_expiry + 1:]:
        if (c.get("body") or "").startswith(("## Writer summary", "## Reviewer pass")):
            return False
    return True


# ---------- the run ----------

class Reconciler:
    def __init__(self, cfg, gh, now, fire_fn, routines):
        self.cfg, self.gh, self.now, self.fire_fn, self.routines = cfg, gh, now, fire_fn, routines
        self.L = cfg["labels"]
        self.loop = {self.L[k] for k in LABEL_KEYS}
        self.lines, self.failed = [], False

    def log(self, line):
        self.lines.append(line)

    def relabel(self, issue, loop_label):
        """Replace all loop labels with loop_label (or none), keep every other label."""
        keep = label_names(issue) - self.loop
        new = keep | ({loop_label} if loop_label else set())
        self.gh.set_labels(issue["number"], new)
        issue["labels"] = [{"name": n} for n in new]

    def strip_closed(self):
        seen = set()
        for label in sorted(self.loop):
            for issue in self.gh.closed_issues_with_label(label):
                if issue["number"] not in seen:
                    seen.add(issue["number"])
                    self.relabel(issue, None)
                    self.log(f"#{issue['number']}: closed, loop labels removed")

    def release_stale(self, issues):
        ttl = self.cfg["claim_ttl_hours"] * 3600
        back = {self.L["writing"]: self.L["queued"], self.L["reviewing"]: self.L["review_queued"]}
        for issue in issues:
            names = label_names(issue)
            for claim, queue in back.items():
                if claim not in names:
                    continue
                since = last_labeled_at(self.gh.events(issue["number"]), claim)
                if since is None or (self.now - since).total_seconds() <= ttl:
                    continue
                n = issue["number"]
                hours = self.cfg["claim_ttl_hours"]
                if is_second_expiry(self.gh.comments(n)):
                    self.gh.comment(n, f"{ROUTER_HEADER}\n{EXPIRED_MARKER}\ntwo runs expired without a hand-off. `{claim}` held for more than {hours}h twice in a row; handing to the owner.")
                    self.relabel(issue, self.L["human"])
                    self.log(f"#{n}: second expired `{claim}` claim -> `{self.L['human']}`")
                else:
                    self.gh.comment(n, f"{ROUTER_HEADER}\n{EXPIRED_MARKER}\n`{claim}` held for more than {hours}h with no hand-off; returned to `{queue}`.")
                    self.relabel(issue, queue)
                    self.log(f"#{n}: expired `{claim}` claim -> `{queue}`")

    def claim_and_fire(self, issue, claim_label, role):
        n = issue["number"]
        before = label_names(issue)
        self.relabel(issue, claim_label)
        routine_id, token = self.routines.get(role, (None, None))
        ok, detail = self.fire_fn(routine_id, token, n)
        if ok:
            self.log(f"#{n}: claimed `{claim_label}`, fired {role} ({detail})")
            return True
        self.gh.set_labels(n, before)  # rule 31: never leave a claim without a run
        issue["labels"] = [{"name": x} for x in before]
        self.log(f"#{n}: FIRE FAILED for {role}, claim reverted: {detail}")
        self.failed = True
        return False

    def claim_reviewers(self, issues):
        for issue in issues:
            names = label_names(issue)
            if self.L["review_queued"] in names and not (names & (self.loop - {self.L["review_queued"]})):
                self.claim_and_fire(issue, self.L["reviewing"], "reviewer")

    def claim_writers(self, issues):
        active = sum(1 for i in issues if self.L["writing"] in label_names(i))
        cap = self.cfg["max_writers"]
        for issue in issues:  # oldest first (rule 28)
            names = label_names(issue)
            n = issue["number"]
            if self.L["queued"] not in names:
                continue
            others = names & (self.loop - {self.L["queued"]})
            if others:
                self.log(f"#{n}: skipped, also has {', '.join(sorted(others))}")
                continue
            blockers = parse_blocked_by(issue.get("body"))
            if blockers is None:
                self.log(f"#{n}: needs a Blocked by line")
                continue
            open_blockers = []
            for b in blockers:
                try:
                    bi = self.gh.issue(b)
                    done = bi.get("state") == "closed" and bi.get("state_reason") == "completed"
                except RuntimeError:
                    done = False
                if not done:
                    open_blockers.append(f"#{b}")
            if open_blockers:
                self.log(f"#{n}: blocked by {', '.join(open_blockers)}")
                continue
            if active >= cap:
                self.log(f"#{n}: eligible, waiting (writer cap {cap} reached)")
                continue
            if self.claim_and_fire(issue, self.L["writing"], "writer"):
                active += 1

    def run(self):
        self.strip_closed()
        issues = self.gh.open_issues()
        self.release_stale(issues)
        self.claim_reviewers(issues)
        self.claim_writers(issues)
        if not self.lines:
            self.log("nothing to do")
        return 1 if self.failed else 0


def write_summary(lines, header="Ralph reconciler"):
    text = f"### {header}\n\n" + "\n".join(f"- {l}" for l in lines) + "\n"
    print(text)
    path = os.environ.get("GITHUB_STEP_SUMMARY")
    if path:
        with open(path, "a", encoding="utf-8") as f:
            f.write(text)


def main(argv=()):
    dry_run = "--dry-run" in argv
    path = os.path.join(".claude", "ralph", "config.json")
    try:
        with open(path, encoding="utf-8") as f:
            cfg = json.load(f)
    except (OSError, ValueError) as e:
        cfg, problems = None, [f"cannot read {path}: {e}"]
    else:
        problems = validate_config(cfg)
    if problems:  # rule 3: fail, claim and fire nothing
        write_summary([f"INVALID CONFIG: {p}" for p in problems] + ["nothing was claimed or fired"])
        return 1
    routines = {
        "writer": (os.environ.get("WRITER_ROUTINE_ID"), os.environ.get("WRITER_TOKEN")),
        "reviewer": (os.environ.get("REVIEWER_ROUTINE_ID"), os.environ.get("REVIEWER_TOKEN")),
    }
    fire_fn = (lambda rid, tok, n: (True, "dry-run, not fired")) if dry_run else fire
    rec = Reconciler(cfg, GitHub(cfg["repo"], dry_run), datetime.now(timezone.utc), fire_fn, routines)
    try:
        code = rec.run()
    except RuntimeError as e:
        rec.log(f"ERROR: {e}")
        code = 1
    write_summary(rec.lines)
    return code


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
