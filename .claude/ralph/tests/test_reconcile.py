"""Tests for .claude/ralph/reconcile.py. Run: python -m unittest discover -s .claude/ralph/tests"""

import json
import os
import sys
import unittest
from datetime import datetime, timedelta, timezone

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))

import reconcile as rc  # noqa: E402

NOW = datetime(2026, 11, 25, 12, 0, tzinfo=timezone.utc)
CFG = json.load(open(os.path.join(os.path.dirname(HERE), "config.json"), encoding="utf-8"))
L = CFG["labels"]


def ts(hours_ago):
    return (NOW - timedelta(hours=hours_ago)).strftime("%Y-%m-%dT%H:%M:%SZ")


class FakeGitHub:
    def __init__(self):
        self.issues = {}    # number -> issue dict
        self.events_ = {}   # number -> list
        self.comments_ = {}  # number -> list
        self.label_calls = []

    def add(self, n, labels=(), body="**Blocked by:** none", state="open", state_reason=None, events=(), comments=()):
        self.issues[n] = {"number": n, "labels": [{"name": l} for l in labels], "body": body,
                          "state": state, "state_reason": state_reason}
        self.events_[n] = list(events)
        self.comments_[n] = list(comments)

    def open_issues(self):
        return [dict(i, labels=list(i["labels"])) for n, i in sorted(self.issues.items()) if i["state"] == "open"]

    def closed_issues_with_label(self, label):
        return [dict(i) for i in self.issues.values() if i["state"] == "closed" and label in rc.label_names(i)]

    def issue(self, n):
        if n not in self.issues:
            raise RuntimeError("404")
        return self.issues[n]

    def events(self, n):
        return self.events_.get(n, [])

    def comments(self, n):
        return self.comments_.get(n, [])

    def set_labels(self, n, labels):
        self.label_calls.append((n, sorted(labels)))
        self.issues[n]["labels"] = [{"name": l} for l in labels]

    def comment(self, n, body):
        self.comments_.setdefault(n, []).append({"user": {"login": rc.BOT}, "body": body})

    def labels(self, n):
        return rc.label_names(self.issues[n])


class Fires:
    def __init__(self, fail=()):
        self.calls, self.fail = [], set(fail)

    def __call__(self, routine_id, token, n):
        self.calls.append((routine_id, n))
        return (False, "HTTP 500: boom") if n in self.fail else (True, "HTTP 200")


def run(gh, fires=None, cfg=None):
    fires = fires or Fires()
    rec = rc.Reconciler(cfg or CFG, gh, NOW, fires, {"writer": ("W", "tw"), "reviewer": ("R", "tr")})
    code = rec.run()
    return rec, fires, code


def labeled(label, hours_ago):
    return {"event": "labeled", "label": {"name": label}, "created_at": ts(hours_ago)}


class Config(unittest.TestCase):
    def test_repo_config_valid(self):
        self.assertEqual(rc.validate_config(CFG), [])

    def test_invalid(self):
        bad = dict(CFG, gates=[])
        self.assertTrue(rc.validate_config(bad))
        self.assertTrue(rc.validate_config(dict(CFG, max_writers=0)))
        self.assertTrue(rc.validate_config(dict(CFG, labels={"queued": "x"})))
        self.assertTrue(rc.validate_config(dict(CFG, labels=dict(L, writing=L["queued"]))))
        self.assertTrue(rc.validate_config([]))


class BlockedBy(unittest.TestCase):
    def test_parse(self):
        self.assertEqual(rc.parse_blocked_by("x\n**Blocked by:** none\n"), [])
        self.assertEqual(rc.parse_blocked_by("**Blocked by:** #23, #24"), [23, 24])
        self.assertEqual(rc.parse_blocked_by("**Blocked by:** #23 #24"), [23, 24])
        self.assertIsNone(rc.parse_blocked_by("no line here"))
        self.assertIsNone(rc.parse_blocked_by("**Blocked by:** the store issue"))
        self.assertIsNone(rc.parse_blocked_by("**Blocked by:**"))


class Writers(unittest.TestCase):
    def test_claims_and_fires_unblocked(self):
        gh = FakeGitHub()
        gh.add(1, [L["queued"], "bug"])
        _, fires, code = run(gh)
        self.assertEqual(code, 0)
        self.assertEqual(fires.calls, [("W", 1)])
        self.assertEqual(gh.labels(1), {L["writing"], "bug"})

    def test_single_label_call_per_claim(self):
        gh = FakeGitHub()
        gh.add(1, [L["queued"]])
        run(gh)
        self.assertEqual(gh.label_calls, [(1, [L["writing"]])])

    def test_blockers(self):
        gh = FakeGitHub()
        gh.add(10, state="closed", state_reason="completed")
        gh.add(11, state="closed", state_reason="not_planned")
        gh.add(12)
        gh.add(1, [L["queued"]], body="**Blocked by:** #10")
        gh.add(2, [L["queued"]], body="**Blocked by:** #11")
        gh.add(3, [L["queued"]], body="**Blocked by:** #12")
        gh.add(4, [L["queued"]], body="**Blocked by:** #999")
        gh.add(5, [L["queued"]], body="no blocked-by line")
        rec, fires, _ = run(gh)
        self.assertEqual(fires.calls, [("W", 1)])
        self.assertTrue(any("#5: needs a Blocked by line" in l for l in rec.lines))
        self.assertTrue(any("#2: blocked by #11" in l for l in rec.lines))

    def test_cap(self):
        gh = FakeGitHub()
        gh.add(1, [L["writing"]], events=[labeled(L["writing"], 0.5)])
        for n in range(2, 7):
            gh.add(n, [L["queued"]])
        _, fires, _ = run(gh)
        self.assertEqual([n for _, n in fires.calls], [2, 3])  # cap 3, one already active, oldest first

    def test_skips_issue_with_two_loop_labels(self):
        gh = FakeGitHub()
        gh.add(1, [L["queued"], L["reviewing"]], events=[labeled(L["reviewing"], 0.1)])
        _, fires, _ = run(gh)
        self.assertEqual(fires.calls, [])

    def test_fire_failure_reverts_and_fails(self):
        gh = FakeGitHub()
        gh.add(1, [L["queued"], "bug"])
        gh.add(2, [L["queued"]])
        rec, fires, code = run(gh, Fires(fail={1}))
        self.assertEqual(code, 1)
        self.assertEqual(gh.labels(1), {L["queued"], "bug"})
        self.assertEqual(gh.labels(2), {L["writing"]})  # a failure does not stop other claims
        self.assertTrue(any("FIRE FAILED" in l and "HTTP 500" in l for l in rec.lines))


class Reviewers(unittest.TestCase):
    def test_claims_review_queued(self):
        gh = FakeGitHub()
        gh.add(1, [L["review_queued"]])
        _, fires, _ = run(gh)
        self.assertEqual(fires.calls, [("R", 1)])
        self.assertEqual(gh.labels(1), {L["reviewing"]})

    def test_waits_while_writer_label_still_present(self):
        gh = FakeGitHub()
        gh.add(1, [L["review_queued"], L["writing"]], events=[labeled(L["writing"], 0.1)])
        _, fires, _ = run(gh)
        self.assertEqual(fires.calls, [])


class Stale(unittest.TestCase):
    def test_fresh_claim_kept(self):
        gh = FakeGitHub()
        gh.add(1, [L["writing"]], events=[labeled(L["writing"], 1.5)])
        run(gh)
        self.assertEqual(gh.labels(1), {L["writing"]})

    def test_stale_writer_released_and_refired(self):
        gh = FakeGitHub()
        gh.add(1, [L["writing"]], events=[labeled(L["writing"], 3)])
        _, fires, _ = run(gh)
        self.assertTrue(any(rc.EXPIRED_MARKER in c["body"] for c in gh.comments(1)))
        self.assertEqual(fires.calls, [("W", 1)])  # back to the queue, then claimed again

    def test_stale_reviewer_released(self):
        gh = FakeGitHub()
        gh.add(1, [L["reviewing"]], events=[labeled(L["reviewing"], 3)])
        _, fires, _ = run(gh)
        self.assertEqual(fires.calls, [("R", 1)])

    def test_second_consecutive_expiry_goes_to_human(self):
        gh = FakeGitHub()
        prior = {"user": {"login": rc.BOT}, "body": f"{rc.ROUTER_HEADER}\n{rc.EXPIRED_MARKER}\nreturned"}
        gh.add(1, [L["writing"]], events=[labeled(L["writing"], 3)], comments=[prior])
        _, fires, _ = run(gh)
        self.assertEqual(gh.labels(1), {L["human"]})
        self.assertEqual(fires.calls, [])
        self.assertIn("two runs expired without a hand-off", gh.comments(1)[-1]["body"])

    def test_handoff_between_expiries_resets(self):
        gh = FakeGitHub()
        prior = {"user": {"login": rc.BOT}, "body": f"{rc.ROUTER_HEADER}\n{rc.EXPIRED_MARKER}\nreturned"}
        summary = {"user": {"login": "okethan35"}, "body": "## Writer summary\n..."}
        gh.add(1, [L["writing"]], events=[labeled(L["writing"], 3)], comments=[prior, summary])
        _, fires, _ = run(gh)
        self.assertEqual(fires.calls, [("W", 1)])

    def test_forged_expiry_marker_ignored(self):
        gh = FakeGitHub()
        forged = {"user": {"login": "stranger"}, "body": rc.EXPIRED_MARKER}
        gh.add(1, [L["writing"]], events=[labeled(L["writing"], 3)], comments=[forged])
        _, fires, _ = run(gh)
        self.assertEqual(fires.calls, [("W", 1)])


class Closed(unittest.TestCase):
    def test_closed_issue_labels_stripped(self):
        gh = FakeGitHub()
        gh.add(1, [L["human"], "bug"], state="closed", state_reason="completed")
        run(gh)
        self.assertEqual(gh.labels(1), {"bug"})


if __name__ == "__main__":
    unittest.main()
