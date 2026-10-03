#!/usr/bin/env python3
"""Execute the package gate with real temporary debs and fake curl/APT/query."""
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
PIN = 'a399d92a622b4664d8d1231bc9b7f53a1d210255a0306fa091c3f63779f65f13'


class Containerlab(unittest.TestCase):
    def run_fixture(self, package='containerlab', version='0.79.0', architecture='amd64', digest_ok=True, apt_status=0, installed_version='0.79.0'):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            tree = root / 'package'
            control = tree / 'DEBIAN'
            control.mkdir(parents=True)
            (control / 'control').write_text(f'Package: {package}\nVersion: {version}\nArchitecture: {architecture}\nMaintainer: Fixture <fixture@example.invalid>\nDescription: offline package identity fixture\n')
            payload = tree / 'usr/bin'
            payload.mkdir(parents=True)
            (payload / 'containerlab').write_text('fixture, never executed\n')
            archive = root / 'fixture.deb'
            subprocess.run(['dpkg-deb', '--build', str(tree), str(archive)], check=True, capture_output=True)
            digest = hashlib.sha256(archive.read_bytes()).hexdigest() if digest_ok else '0' * 64
            stubs = root / 'stubs'
            stubs.mkdir()
            commands = {
                'curl': '#!/bin/sh\n[ "$1" = -fsSL ] && [ "$3" = -o ] || exit 90\ncp "$FIXTURE_ARCHIVE" "$4"\n',
                'apt-get': '#!/bin/sh\nprintf "apt %s\\n" "$*" >> "$CALL_LOG"\nexit "$APT_STATUS"\n',
                'dpkg-query': '#!/bin/sh\nprintf "query %s\\n" "$*" >> "$CALL_LOG"\nprintf "%s" "$INSTALLED_VERSION"\n',
                'dpkg-deb': '#!/bin/sh\nprintf "inspect %s\\n" "$3" >> "$CALL_LOG"\nexec /usr/bin/dpkg-deb "$@"\n',
            }
            for name, text in commands.items():
                stub = stubs / name
                stub.write_text(text)
                stub.chmod(0o755)
            source = (ROOT / 'scripts/40-install-lab.sh').read_text()
            start = source.index('# BEGIN VERIFIED CONTAINERLAB')
            end = source.index('# END VERIFIED CONTAINERLAB', start)
            fragment = source[start:end].replace('/tmp/ngfw-containerlab.', str(root / 'ngfw-containerlab.'))
            harness = f'set -euo pipefail\nCONTAINERLAB_VER=0.79.0\nCONTAINERLAB_URL=https://fixture.invalid/containerlab_0.79.0_linux_amd64.deb\nCONTAINERLAB_SHA256={digest}\nVIRT=() TRAFFIC=() ANALYSIS=() BASE=()\n'
            log = root / 'calls'
            result = subprocess.run(['bash', '-c', harness + fragment], capture_output=True, text=True,
                                    env=dict(os.environ, PATH=f'{stubs}:/usr/bin:/bin', FIXTURE_ARCHIVE=str(archive), CALL_LOG=str(log), APT_STATUS=str(apt_status), INSTALLED_VERSION=installed_version))
            return result, log.read_text() if log.exists() else '', list(root.glob('ngfw-containerlab.*'))

    def test_verified_exact_deb_reaches_apt_then_version_query(self):
        result, calls, work = self.run_fixture()
        self.assertEqual(result.returncode, 0, result.stderr)
        lines = calls.splitlines()
        self.assertEqual(lines[:4], ['inspect Package', 'inspect Version', 'inspect Architecture', 'apt update'])
        self.assertIn('apt install -y ', lines[4])
        self.assertTrue(lines[4].endswith('/containerlab_0.79.0_linux_amd64.deb'))
        self.assertEqual(lines[5], 'query -W -f=${Version} containerlab')
        self.assertEqual(work, [])

    def test_wrong_digest_before_package_inspection_and_apt(self):
        result, calls, work = self.run_fixture(digest_ok=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(calls, '')
        self.assertEqual(work, [])

    def test_wrong_metadata_refuses_before_apt(self):
        for changes in ({'package': 'foreign'}, {'version': '0.78.0'}, {'architecture': 'arm64'}):
            with self.subTest(changes=changes):
                result, calls, work = self.run_fixture(**changes)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn('apt ', calls)
                self.assertNotIn('query ', calls)
                self.assertEqual(work, [])

    def test_apt_failure_cleans_up_without_postinstall_query(self):
        result, calls, work = self.run_fixture(apt_status=42)
        self.assertEqual(result.returncode, 42)
        self.assertIn('apt update', calls)
        self.assertNotIn('query ', calls)
        self.assertEqual(work, [])

    def test_installed_version_mismatch_stops_continuation(self):
        result, calls, work = self.run_fixture(installed_version='0.78.0')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('installed containerlab version', result.stderr)
        self.assertIn('query ', calls)
        self.assertEqual(work, [])

    def test_nonmutating_config_and_fixed_source(self):
        result = subprocess.run(['bash', str(ROOT / 'scripts/40-install-lab.sh'), '--check-config'], text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), f'containerlab=0.79.0 sha256={PIN}')
        source = (ROOT / 'scripts/40-install-lab.sh').read_text()
        self.assertIn('_linux_amd64.deb', source)
        self.assertNotIn('get.containerlab.dev', source)
        self.assertNotIn('tarfile', source)


if __name__ == '__main__':
    unittest.main(verbosity=2)
