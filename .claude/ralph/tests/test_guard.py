"""Tests for .claude/ralph/guard.py. Run: python -m unittest discover -s .claude/ralph/tests"""

import json
import os
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
GUARD = os.path.join(os.path.dirname(HERE), "guard.py")
sys.path.insert(0, os.path.dirname(HERE))

import guard  # noqa: E402

CFG = {
    "default_branch": "main",
    "protected_paths": ["go.mod", "go.sum"],
    "test_globs": ["**/*_test.go"],
    "spec_file": "SPEC.md",
    "decisions_file": "DECISIONS.md",
    "lessons_file": "LESSONS.md",
}


def blocked(command, remote, cwd=None):
    try:
        guard.check_bash(command, CFG, cwd or tempfile.gettempdir(), remote)
        return None
    except guard.Blocked as b:
        return str(b)


class Globs(unittest.TestCase):
    def test_patterns(self):
        cases = [
            (".claude/**", ".claude/ralph/writer.md", True),
            (".claude/**", "claude/x", False),
            ("**/CLAUDE.md", "CLAUDE.md", True),
            ("**/CLAUDE.md", "internal/server/CLAUDE.md", True),
            ("**/*_test.go", "internal/store/ttl_test.go", True),
            ("**/*_test.go", "ttl_test.go", True),
            ("**/*_test.go", "internal/store/ttl.go", False),
            ("SPEC.md", "docs/SPEC.md", False),
            ("*.md", "a/b.md", False),
        ]
        for pat, path, want in cases:
            self.assertEqual(guard.matches_any(path, [pat]), want, (pat, path))

    def test_always_protected(self):
        pats = guard.protected_patterns({"protected_paths": []})
        for p in (".github/workflows/ci.yml", ".claude/settings.json", "pkg/CLAUDE.md"):
            self.assertTrue(guard.matches_any(p, pats), p)


class EverySession(unittest.TestCase):
    """Rule 13 holds locally and in the cloud."""

    def test_merge_blocked_everywhere(self):
        for remote in (False, True):
            self.assertIn("rule 13", blocked("gh pr merge 30 --squash", remote))
            self.assertIn("rule 13", blocked("gh api -X PUT repos/o/r/pulls/30/merge", remote))
            self.assertIn("rule 13", blocked('gh api -X PUT "$R/pulls/$P/merge"', remote))
            self.assertIn("rule 13", blocked("curl -X PUT https://api.github.com/repos/o/r/pulls/3/merge", remote))
            self.assertIn("rule 13", blocked("cd x && gh pr merge 1", remote))

    def test_decision_inline_blocked(self):
        self.assertIn("rule 13", blocked("gh api -X POST $R/issues/5/comments -f 'body=**Decision:** allow it'", False))
        self.assertIn("rule 13", blocked('gh issue comment 5 --body "**Decision:** yes"', True))

    def test_decision_from_file_blocked(self):
        d = tempfile.mkdtemp()
        with open(os.path.join(d, "c.md"), "w", encoding="utf-8") as f:
            f.write("\n**Decision:** tests may change\n")
        self.assertIn("rule 13", blocked("gh api -X POST $R/issues/5/comments -F body=@c.md", True, d))
        self.assertIn("rule 13", blocked("gh issue comment 5 --body-file c.md", False, d))

    def test_wrapped_merges_blocked(self):
        for cmd in (
            'bash -c "gh pr merge 5"',
            "echo $(gh pr merge 5)",
            "eval 'gh pr merge 5'",
            "env gh pr merge 5",
            "gh -R o/r pr merge 5",
            "gh api -X PUT repos/o/r/pulls/5//merge",
            "gh api graphql -f query='mutation { mergePullRequest(input: {}) { clientMutationId } }'",
        ):
            for remote in (False, True):
                self.assertIn("rule 13", blocked(cmd, remote) or "", cmd)

    def test_merge_words_that_are_not_merges(self):
        for cmd in (
            "git merge origin/main",
            "gh pr checks 40 && git merge origin/main",
            "gh api $R/pulls/40 --jq '.mergeable,.mergeable_state'",
        ):
            self.assertIsNone(blocked(cmd, True), cmd)

    def test_decision_in_input_json_blocked(self):
        d = tempfile.mkdtemp()
        with open(os.path.join(d, "b.json"), "w", encoding="utf-8") as f:
            json.dump({"body": "**Decision:** allow"}, f)
        self.assertIn("rule 13", blocked("gh api -X POST $R/issues/5/comments --input b.json", True, d))

    def test_ordinary_comment_allowed(self):
        d = tempfile.mkdtemp()
        with open(os.path.join(d, "c.md"), "w", encoding="utf-8") as f:
            f.write("## Writer summary\nPer the owner's **Decision:** comment, I did X.\n")
        self.assertIsNone(blocked("gh api -X POST $R/issues/5/comments -F body=@c.md", True, d))

    def test_local_session_otherwise_free(self):
        for cmd in ("git push origin main", "gh repo edit --visibility private", "gh api -X PATCH repos/o/r/issues/5"):
            self.assertIsNone(blocked(cmd, False), cmd)


class CloudOnly(unittest.TestCase):
    """Rule 14 holds when CLAUDE_CODE_REMOTE=true."""

    def test_push_rules(self):
        for cmd in (
            "git push origin main",
            "git push origin HEAD:main",
            "git push origin HEAD:refs/heads/main",
            "git push --force origin issue-24",
            "git push -f origin issue-24",
            "git push origin +issue-24",
            "git push --force-with-lease origin issue-24",
            "git -C . push --force origin issue-24",
        ):
            self.assertIn("rule 14", blocked(cmd, True) or "", cmd)
        for cmd in (
            "git push -u origin issue-24",
            "git push origin FETCH_HEAD:refs/heads/issue-30-x",
            "git pull origin main",
        ):
            self.assertIsNone(blocked(cmd, True), cmd)

    def test_bare_push_on_default_branch(self):
        d = tempfile.mkdtemp()
        subprocess.run(["git", "init", "-q", "-b", "main", d], check=True)
        self.assertIn("rule 14", blocked("git push", True, d))
        subprocess.run(["git", "-C", d, "checkout", "-q", "-b", "issue-7"], check=True)
        self.assertIsNone(blocked("git push", True, d))

    def test_repo_and_auth(self):
        for cmd in ("gh repo edit --default-branch x", "gh auth token", "gh auth status", "gh issue edit 5 --body x"):
            self.assertIn("rule 14", blocked(cmd, True) or "", cmd)

    def test_settings_api(self):
        for cmd in (
            "gh api -X PUT repos/o/r/rulesets/27895579 --input r.json",
            "gh api -X DELETE repos/o/r/branches/main/protection",
            "gh api -X PATCH repos/o/r -f visibility=private",
            "gh api -X PUT repos/o/r/actions/secrets/X -f encrypted_value=1",
        ):
            self.assertIn("rule 14", blocked(cmd, True) or "", cmd)
        self.assertIsNone(blocked("gh api repos/o/r/rulesets/27895579", True))

    def test_workflow_dispatch_and_pr_edits(self):
        for cmd in (
            "gh workflow run ralph-router.yml --ref x",
            "gh run rerun 12345",
            "gh api -X POST repos/o/r/actions/workflows/ralph-router.yml/dispatches -f ref=x",
            "gh api -X POST repos/o/r/actions/runs/123/rerun",
            "gh api -X PATCH repos/o/r/pulls/40 -f base=x",
            "gh api -X PATCH $R/pulls/$P -f state=closed",
        ):
            self.assertIn("rule 14", blocked(cmd, True) or "", cmd)
        for cmd in (
            "gh api repos/o/r/actions/runs?branch=issue-24",
            "gh api $R/pulls/40",
            "gh api \"$R/pulls/40/files?per_page=100\"",
            "gh workflow list",
        ):
            self.assertIsNone(blocked(cmd, True), cmd)

    def test_issue_and_comment_edits(self):
        for cmd in (
            "gh api -X PATCH $R/issues/24 -f body=x",
            "gh api -X PATCH $R/issues/comments/123 -f body=x",
            "gh api -X DELETE $R/issues/comments/$C",
        ):
            self.assertIn("rule 14", blocked(cmd, True) or "", cmd)

    def test_loop_commands_allowed(self):
        for cmd in (
            "gh api -X POST $R/issues/24/labels -f 'labels[]=needs-review'",
            "gh api -X DELETE $R/issues/24/labels/in-progress",
            "gh api -X POST $R/issues/24/comments -F body=@summary.md",
            "gh api \"$R/issues/24/events?per_page=100\"",
            "gh api -X POST $R/pulls -f title=x -f head=issue-24 -f base=main",
            "go test ./... && go test -race ./...",
            "git add internal/store/store.go && git commit -m 'issue-24: x'",
        ):
            self.assertIsNone(blocked(cmd, True), cmd)


class FileTools(unittest.TestCase):
    def setUp(self):
        self.root = tempfile.mkdtemp()

    def check(self, rel, remote):
        try:
            guard.check_file_tool({"file_path": os.path.join(self.root, rel)}, CFG, self.root, remote)
            return None
        except guard.Blocked as b:
            return str(b)

    def test_protected_in_cloud(self):
        for rel in (".claude/ralph/writer.md", ".github/workflows/ci.yml", "CLAUDE.md", "SPEC.md", "go.mod"):
            self.assertIn("rule 14", self.check(rel, True) or "", rel)

    def test_allowed(self):
        self.assertIsNone(self.check("internal/store/store.go", True))
        self.assertIsNone(self.check(".claude/ralph/writer.md", False))  # owner edits locally


class PrCheck(unittest.TestCase):
    def test_agent_pr_paths_and_tests(self):
        files = [
            {"filename": ".github/workflows/ci.yml", "status": "modified"},
            {"filename": "internal/store/ttl_test.go", "status": "modified"},
            {"filename": "internal/store/new_test.go", "status": "added"},
            {"filename": "internal/store/store.go", "status": "modified"},
            {"filename": "x/a_test.go", "status": "renamed", "previous_filename": "x/old_test.go"},
        ]
        got = guard.check_pr_files("issue-24-expire", files, CFG)
        self.assertEqual(len(got), 3, got)
        self.assertTrue(any(".github/workflows/ci.yml" in g for g in got))
        self.assertTrue(any("ttl_test.go" in g for g in got))
        self.assertFalse(any("new_test.go" in g for g in got))

    def test_owner_pr_not_checked(self):
        files = [{"filename": ".claude/ralph/writer.md", "status": "modified"}]
        self.assertEqual(guard.check_pr_files("task4-guards", files, CFG), [])


class Lint(unittest.TestCase):
    def test_cases(self):
        good = "---\nname: spec\ndescription: \"a: b\"\ndisable-model-invocation: true\n---\nbody"
        self.assertEqual(guard.lint_skill("spec", good), [])
        self.assertIn("not valid YAML", guard.lint_skill("spec", good.replace('"a: b"', "a: b: c"))[0])
        self.assertTrue(guard.lint_skill("spec", good.replace("disable-model-invocation", "allowed-tool")))
        self.assertTrue(guard.lint_skill("grill", good))
        self.assertTrue(guard.lint_skill("spec", good.replace("true", "false")))
        self.assertTrue(guard.lint_skill("spec", "\n" + good))

    def test_repo_skills_pass(self):
        skills = os.path.join(os.path.dirname(os.path.dirname(HERE)), "skills")
        self.assertEqual(guard.lint_dir(skills), [])


class HookProcess(unittest.TestCase):
    """The real entry point: exit codes, stderr, and failing closed."""

    def run_hook(self, payload, remote=False):
        env = dict(os.environ)
        env.pop("CLAUDE_CODE_REMOTE", None)
        if remote:
            env["CLAUDE_CODE_REMOTE"] = "true"
        return subprocess.run([sys.executable, GUARD, "hook"], input=payload, capture_output=True, text=True, env=env)

    def test_exit_codes(self):
        ok = self.run_hook(json.dumps({"tool_name": "Bash", "tool_input": {"command": "ls"}}))
        self.assertEqual(ok.returncode, 0, ok.stderr)
        bad = self.run_hook(json.dumps({"tool_name": "Bash", "tool_input": {"command": "gh pr merge 1"}}))
        self.assertEqual(bad.returncode, 2)
        self.assertEqual(len(bad.stderr.strip().splitlines()), 1)
        self.assertIn("rule 13", bad.stderr)

    def test_powershell_tool_is_checked(self):
        r = self.run_hook(json.dumps({"tool_name": "PowerShell", "tool_input": {"command": "gh pr merge 26 --squash"}}))
        self.assertEqual(r.returncode, 2)
        self.assertIn("rule 13", r.stderr)
        ok = self.run_hook(json.dumps({"tool_name": "PowerShell", "tool_input": {"command": "Get-ChildItem"}}))
        self.assertEqual(ok.returncode, 0, ok.stderr)

    def test_fails_closed_on_garbage(self):
        r = self.run_hook("not json")
        self.assertEqual(r.returncode, 2)
        self.assertIn("blocked to stay safe", r.stderr)


if __name__ == "__main__":
    unittest.main()
