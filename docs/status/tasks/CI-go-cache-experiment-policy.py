#!/usr/bin/env python3
"""Apply inert proposal in temporary source tree; never run workflow/cache/build."""
import pathlib
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
WORKFLOW = '.github/workflows/ci.yml'
BASE = (ROOT / WORKFLOW).read_text()
PATCH = pathlib.Path(__file__).with_name('CI-go-cache-experiment.patch').read_text()
OLD = '          cache: false\n'
NEW = "          cache: true\n          cache-dependency-path: '**/go.sum'\n"


def allowed(candidate):
    # Exact allowed delta preserves every original command, trigger, pin and permission.
    return BASE.count(OLD) == 1 and candidate == BASE.replace(OLD, NEW)


class ProposalPolicy(unittest.TestCase):
    def test_real_patch_applies_only_two_cache_inputs(self):
        self.assertEqual([x for x in PATCH.splitlines() if x.startswith(('--- ', '+++ '))],
                         ['--- a/' + WORKFLOW, '+++ b/' + WORKFLOW])
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory); target = root / WORKFLOW
            target.parent.mkdir(parents=True); target.write_text(BASE)
            result = subprocess.run(['git', 'apply', '--check', '-'], input=PATCH,
                                    cwd=root, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            result = subprocess.run(['git', 'apply', '-'], input=PATCH,
                                    cwd=root, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertTrue(allowed(target.read_text()))
            self.assertEqual([x.relative_to(root).as_posix() for x in root.rglob('*') if x.is_file()], [WORKFLOW])

    def test_pin_version_permission_gate_and_cache_scope_changes_refuse(self):
        candidate = BASE.replace(OLD, NEW)
        self.assertTrue(allowed(candidate))
        mutations = [
            ("go-version: '1.26.0'", "go-version: '1.26.x'"),
            ('contents: read', 'contents: write'),
            ('40f1582b2485089dde7abd97c1529aa768e1baff', 'main'),
            ('tools/ci.sh "${args[@]}"', 'true'),
            ("cache-dependency-path: '**/go.sum'", "cache-dependency-path: 'apps/agent/go.sum'"),
            ('          cache: true', '          cache: true\n          cache-mode: read-write'),
            ('      - name: Run repository gate', "      - name: Run repository gate\n        if: steps.go.outputs.cache-hit != 'true'"),
        ]
        for old, new in mutations:
            with self.subTest(old=old):
                self.assertIn(old, candidate)
                self.assertFalse(allowed(candidate.replace(old, new)))

    def test_all_committed_sum_files_covered_without_root_sum(self):
        entries = subprocess.check_output(['git', 'ls-tree', '-r', '--name-only', 'HEAD'], cwd=ROOT, text=True).splitlines()
        sums = [x for x in entries if x.endswith('/go.sum') or x == 'go.sum']
        modules = [x for x in entries if x.endswith('/go.mod') or x == 'go.mod']
        self.assertEqual(len(sums), 17); self.assertEqual(len(modules), 22)
        self.assertNotIn('go.sum', sums)
        self.assertTrue(all(pathlib.PurePosixPath(x).match('**/go.sum') for x in sums))

    def test_original_race_count_one_and_quick_dispatch_remain(self):
        agent = (ROOT / 'apps/agent/Makefile').read_text()
        self.assertIn('-race -count=1', agent)
        self.assertIn('args=(quick)', BASE)
        self.assertIn('tools/ci.sh "${args[@]}"', BASE)
        self.assertNotIn('cache-hit', BASE)
        self.assertNotIn('pull_request_target', BASE)
        self.assertIn('          cache: false', (ROOT / WORKFLOW).read_text())


if __name__ == '__main__':
    result = unittest.TextTestRunner(verbosity=2).run(unittest.TestLoader().loadTestsFromTestCase(ProposalPolicy))
    raise SystemExit(0 if (result.testsRun == 4 and result.wasSuccessful() and not result.skipped
                          and not result.expectedFailures and not result.unexpectedSuccesses) else 1)
