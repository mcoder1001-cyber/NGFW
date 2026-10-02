#!/usr/bin/env python3
"""Run the actual remote verifier as redirected local fixtures, without SSH/APT."""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]


class TransferGate(unittest.TestCase):
    def run_fixture(self, fault):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'package/DEBIAN').mkdir(parents=True)
            (root / 'bin').mkdir()
            control = 'Package: vpp\nVersion: 26.06-release+vrx1\nArchitecture: amd64\nMaintainer: Fixture <test@example.invalid>\nDescription: Fixture\n'
            (root / 'package/DEBIAN/control').write_text(control)
            package = root / 'vpp.deb'
            subprocess.run(['dpkg-deb', '--build', '--root-owner-group', str(root / 'package'), str(package)],
                           check=True, capture_output=True)
            entry = dict(package='vpp', version='26.06-release+vrx1', architecture='amd64',
                         file='vpp.deb', sha256=hashlib.sha256(package.read_bytes()).hexdigest())
            selection = dict(version=entry['version'], packages=[entry])
            policy = root / 'policy-rc.d'
            policy.write_text('#!/bin/sh\nexit ' + ('0' if fault == 'policy' else '101') + '\n')
            policy.chmod(0o755)
            for command, text in [('apt-get', 'echo APT >> "$CALL_LOG"\n'),
                                  ('dpkg-query', 'printf "%s" "$INSTALLED_VERSION"\n')]:
                path = root / 'bin' / command
                path.write_text('#!/bin/sh\n' + text)
                path.chmod(0o755)
            if fault == 'hash': entry['sha256'] = '0' * 64
            if fault == 'control': entry['architecture'] = 'arm64'
            if fault == 'symlink':
                package.rename(root / 'original.deb')
                package.symlink_to('original.deb')
            source = (ROOT / 'tools/lab').read_text()
            match = re.search(r'printf .*?\| run_on "\$vm" python3 -c \'\n(.*?)\n\' "\$remote_dir"', source, re.S)
            self.assertIsNotNone(match)
            code = match.group(1).replace('/usr/sbin/policy-rc.d', str(policy))
            # Model only owner identity on the isolated fixture, not its mode/path.
            code = code.replace('policy.stat().st_uid!=0', f'policy.stat().st_uid!={os.getuid()}')
            env = dict(os.environ, PATH=str(root / 'bin') + ':' + os.environ['PATH'],
                       CALL_LOG=str(root / 'calls'),
                       INSTALLED_VERSION='wrong' if fault == 'installed' else entry['version'])
            result = subprocess.run(['python3', '-c', code, str(root)], input=json.dumps(selection),
                                    text=True, env=env, capture_output=True)
            if fault == 'none':
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual((root / 'calls').read_text(), 'APT\n')
            else:
                self.assertNotEqual(result.returncode, 0)
                if fault == 'installed': self.assertTrue((root / 'calls').exists())
                else: self.assertFalse((root / 'calls').exists())

    def test_verified_transfer_installs_then_checks_manifest_version(self):
        self.run_fixture('none')

    def test_digest_control_symlink_and_start_policy_fail_before_apt(self):
        for fault in ['hash', 'control', 'symlink', 'policy']:
            with self.subTest(fault=fault): self.run_fixture(fault)

    def test_installed_version_mismatch_refuses_configuration_continuation(self):
        self.run_fixture('installed')


if __name__ == '__main__':
    unittest.main(verbosity=2)
