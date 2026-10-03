"""Exercise real image-root credential lifecycle under uncertain database responses."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

BIN = Path(__file__).resolve().parents[1] / 'common/bin'


class ConsoleRecoveryTests(unittest.TestCase):
    def setUp(self):
        self.initialize_fixture()

    def initialize_fixture(self):
        self.temp = tempfile.TemporaryDirectory(prefix='ngfw-console-recovery-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / 'image'
        self.root.mkdir()
        self.query = Path(self.temp.name) / 'query'
        self.env = {**os.environ, 'NGFW_CONSOLE_BANNER_TEST_QUERY': str(self.query)}
        self.run_script('ngfw-bootstrap-password')
        self.run_script('ngfw-console-banner', 'render')
        done = self.root / 'var/lib/ngfw/firstboot.done'
        done.parent.mkdir(parents=True, exist_ok=True)
        done.touch()
        self.password = self.root / 'var/lib/ngfw-image/console/bootstrap-password'
        self.issue = self.root / 'etc/issue.d/50-ngfw-console.issue'
        self.password_before = self.password.read_bytes()
        self.issue_before = self.issue.read_bytes()
        self.secret = self.password_before.decode().splitlines()[1]

    def run_script(self, name, *args):
        return subprocess.run([str(BIN / name), *args, '--root', str(self.root)],
                              env=self.env, capture_output=True, text=True, check=True)

    def reply(self, output, status=0):
        # Fixed script reads fixture bytes, never interpolates them as shell code.
        (self.query.parent / 'response').write_text(output)
        self.query.write_text('#!/bin/sh\ncat "$(dirname "$0")/response"\nexit %d\n' % status)
        self.query.chmod(0o700)

    def assert_preserved_then_recover(self, output, status):
        self.reply(output, status)
        result = self.run_script('ngfw-console-banner', 'check-login')
        self.assertEqual(self.password.read_bytes(), self.password_before)
        self.assertEqual(self.password.stat().st_mode & 0o777, 0o600)
        self.assertEqual(self.issue.read_bytes(), self.issue_before)
        self.assertEqual(self.issue.stat().st_mode & 0o777, 0o600)
        self.assertNotIn(self.secret, result.stdout + result.stderr)
        self.reply('0\n')
        result = self.run_script('ngfw-console-banner', 'check-login')
        self.assertFalse(self.password.exists())
        self.assertNotIn(self.secret, self.issue.read_text())
        self.assertNotIn(self.secret, result.stdout + result.stderr)
        self.assertEqual(self.issue.stat().st_mode & 0o777, 0o644)
        self.assertIn('https://', self.issue.read_text())
        clean_issue = self.issue.read_bytes()
        self.run_script('ngfw-console-banner', 'check-login')
        self.assertEqual(self.issue.read_bytes(), clean_issue)

    def test_partial_zero_from_failed_database_keeps_credentials_until_recovery(self):
        self.assert_preserved_then_recover('0\n', 2)

    def test_large_and_leading_zero_positive_counts_preserve_credentials(self):
        for output in ('9223372036854775808\n', '18446744073709551616\n', '008\n'):
            with self.subTest(output=output):
                self.initialize_fixture()
                self.assert_preserved_then_recover(output, 0)

    def test_decimal_zero_with_leading_zeros_permits_cleanup(self):
        self.reply('000\n')
        self.run_script('ngfw-console-banner', 'check-login')
        self.assertFalse(self.password.exists())
        self.assertNotIn(self.secret, self.issue.read_text())

    def test_malformed_successful_database_responses_keep_credentials_until_recovery(self):
        for output in ('', '-1\n', 'not-a-count\n', '0\n0\n'):
            with self.subTest(output=output):
                # Fresh credentials for each independent uncertainty/recovery cycle.
                self.initialize_fixture()
                self.assert_preserved_then_recover(output, 0)


if __name__ == '__main__':
    unittest.main()
