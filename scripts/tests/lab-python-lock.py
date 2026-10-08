#!/usr/bin/env python3
"""Synthetic wheel metadata only: fixtures never supply production release pins."""
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import tempfile
import unittest
import zipfile

SCRIPT = Path(__file__).resolve().parents[1] / 'lab-python-lock.py'
DIRECT = ('pip', 'robotframework', 'robotframework-sshlibrary', 'scapy', 'pytest', 'requests')


class OfflineLock(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.wheels = self.root / 'wheels'
        self.wheels.mkdir()
        self.direct = self.root / 'direct.txt'
        self.direct.write_text(''.join(name + '==1.0\n' for name in DIRECT))
        self.output = self.root / 'output'
        for name in DIRECT:
            self.wheel(name, dependencies=('fixture-dependency>=1.0',) if name == 'requests' else ())
        self.wheel('fixture-dependency')
        self.args = [sys.executable, '-I', str(SCRIPT), '--direct', str(self.direct),
                     '--wheelhouse', str(self.wheels), '--output', str(self.output),
                     '--target-python', f'{sys.version_info.major}.{sys.version_info.minor}',
                     '--target-platform', f'{sys.platform}-{platform.machine()}',
                     '--target-os', ':'.join(platform.freedesktop_os_release().get(key, '')
                                           for key in ('ID', 'VERSION_ID'))]

    def wheel(self, name, dependencies=(), tag='py3-none-any', extra=None):
        stem = name.replace('-', '_')
        path = self.wheels / f'{stem}-1.0-{tag}.whl'
        directory = f'{stem}-1.0.dist-info'
        with zipfile.ZipFile(path, 'w') as wheel:
            wheel.writestr(directory + '/METADATA', 'Metadata-Version: 2.1\nName: ' + name +
                           '\nVersion: 1.0\n' + ''.join('Requires-Dist: ' + dep + '\n' for dep in dependencies))
            wheel.writestr(directory + '/WHEEL', 'Wheel-Version: 1.0\nGenerator: synthetic-fixture\n'
                           'Root-Is-Purelib: true\nTag: ' + tag + '\n')
            wheel.writestr(directory + '/RECORD', '')
            if extra:
                wheel.writestr(extra, 'never execute this content')
        return path

    def run_tool(self, args=None, environment=None):
        return subprocess.run(args or self.args, capture_output=True, text=True,
                              env=environment, timeout=40)

    def refused(self, result, message=None):
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn('REFUSED', result.stderr)
        if message:
            self.assertIn(message, result.stderr)
        self.assertFalse(self.output.exists())

    def test_complete_closure_and_digest_receipt(self):
        result = self.run_tool()
        self.assertEqual(result.returncode, 0, result.stderr)
        lock = (self.output / 'requirements.lock').read_bytes()
        receipt = json.loads((self.output / 'provenance.json').read_text())
        self.assertEqual(len(lock.splitlines()), 7)
        self.assertIn(b'fixture-dependency==1.0', lock)
        self.assertEqual(receipt['lock_sha256'], hashlib.sha256(lock).hexdigest())
        self.assertIn('unverified', receipt['status'])
        for artifact in receipt['artifacts']:
            self.assertEqual(artifact['sha256'], hashlib.sha256((self.wheels / artifact['wheel']).read_bytes()).hexdigest())
        # Candidate syntax interoperates with the actual installer's read-only preflight.
        result = subprocess.run(['bash', str(SCRIPT.with_name('40-install-lab.sh')), '--dry-run'],
                                env=dict(os.environ, NGFW_LAB_REQUIREMENTS=str(self.output / 'requirements.lock')),
                                capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('validated', result.stdout)

    def test_missing_transitive_dependency_refused(self):
        (self.wheels / 'fixture_dependency-1.0-py3-none-any.whl').unlink()
        self.refused(self.run_tool(), 'offline wheel resolution failed')

    def test_incompatible_wheel_refused(self):
        (self.wheels / 'scapy-1.0-py3-none-any.whl').unlink()
        self.wheel('scapy', tag='cp27-cp27m-win32')
        self.refused(self.run_tool(), 'offline wheel resolution failed')

    def test_explicit_runtime_mismatch_refused(self):
        for option, value in (('--target-python', '0.0'), ('--target-platform', 'linux-wrong'), ('--target-os', 'wrong:0')):
            with self.subTest(option=option):
                args = self.args.copy()
                args[args.index(option) + 1] = value
                self.refused(self.run_tool(args), 'differs from resolver runtime')

    def test_url_dependency_refused_before_pip(self):
        self.wheel('requests', dependencies=('fixture-dependency @ https://invalid.test/demo.whl',))
        self.refused(self.run_tool(), 'direct URL wheel dependency forbidden')

    def test_traversal_and_symlink_wheels_refused(self):
        path = self.wheel('requests', extra='../../outside')
        self.refused(self.run_tool(), 'unsafe wheel member path')
        path.unlink()
        path.symlink_to(self.direct)
        self.refused(self.run_tool())

    def test_direct_pins_no_options_floating_or_duplicates(self):
        for data in ('pytest\n', self.direct.read_text() + 'pip==1.0\n',
                     self.direct.read_text().replace('pip==1.0', '--extra-index-url https://invalid.test')):
            with self.subTest(data=data):
                self.direct.write_text(data)
                self.refused(self.run_tool())

    def test_host_pip_config_cannot_add_network_links(self):
        config = self.root / 'pip.conf'
        config.write_text('[global]\nfind-links = https://invalid.test/should-never-connect\n'
                          'index-url = https://invalid.test/should-never-connect\n')
        result = self.run_tool(environment=dict(os.environ, PIP_CONFIG_FILE=str(config),
                                                PIP_FIND_LINKS='https://invalid.test/should-never-connect'))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn('invalid.test', result.stderr)

    def test_existing_output_preserved(self):
        self.output.mkdir()
        sentinel = self.output / 'sentinel'
        sentinel.write_text('keep')
        result = self.run_tool()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('output must be a new directory', result.stderr)
        self.assertEqual(sentinel.read_text(), 'keep')


if __name__ == '__main__':
    unittest.main()
