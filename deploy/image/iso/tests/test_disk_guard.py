"""Exercise the actual read-only early guard with an isolated lsblk executable."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

GUARD = Path(__file__).resolve().parents[1] / 'installer/vrx-disk-guard.sh'


class DiskGuardTests(unittest.TestCase):
    def guard(self, output, status=0, cmdline=''):
        with tempfile.TemporaryDirectory() as directory:
            stub = Path(directory) / 'lsblk'
            stub.write_text('#!/bin/sh\nprintf "%s\\n" "$VRX_TEST_LABELS"\nexit "$VRX_TEST_LSBLK_STATUS"\n')
            stub.chmod(0o755)
            env = dict(os.environ, PATH=directory + os.pathsep + os.environ['PATH'],
                       VRX_TEST_LABELS=output, VRX_TEST_LSBLK_STATUS=str(status))
            # Invoke in an if exactly as the installer does: errexit cannot hide a bug.
            return subprocess.run(
                ['bash', '-euo', 'pipefail', '-c',
                 'source "$1"; if vrx_check_reinstall "$2"; then echo proceed; else exit 1; fi',
                 '_', str(GUARD), cmdline], env=env, capture_output=True, text=True,
            )

    def test_inventory_failure_refuses_proceed(self):
        for output in ['', 'unrelated unrelated', 'vrx-rootA vrx-rootA']:
            result = self.guard(output, status=9)
            self.assertEqual(result.returncode, 1)
            self.assertNotIn('proceed', result.stdout)
            self.assertIn('inventory failed', result.stderr)

    def test_inventory_failure_cannot_be_overridden_by_reinstall(self):
        result = self.guard('', status=9, cmdline='autoinstall vrx.reinstall=1')
        self.assertEqual(result.returncode, 1)
        self.assertNotIn('proceed', result.stdout)

    def test_existing_partition_or_filesystem_label_refuses(self):
        for output in ['vrx-rootA other', 'other vrx-rootA', ' vrx-rootA']:
            result = self.guard(output)
            self.assertEqual(result.returncode, 1)
            self.assertIn('already exists', result.stderr)

    def test_blank_disk_and_unrelated_labels_allow(self):
        for output in ['', 'other other', 'vrx-rootAB other']:
            self.assertEqual(self.guard(output).returncode, 0)

    def test_explicit_reinstall_allows_known_existing_install(self):
        self.assertEqual(self.guard('vrx-rootA vrx-rootA', cmdline='autoinstall vrx.reinstall=1').returncode, 0)


if __name__ == '__main__':
    unittest.main()
