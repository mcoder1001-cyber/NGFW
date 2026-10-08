#!/usr/bin/env python3
"""Synthetic wheel metadata only: fixtures never supply production release pins."""
import argparse
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import tempfile
import unittest
from unittest import mock
import zipfile

SCRIPT = Path(__file__).resolve().parents[1] / 'lab-python-lock.py'
spec = importlib.util.spec_from_file_location('lab_lock', SCRIPT)
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)
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

    def generate_args(self):
        return argparse.Namespace(direct=self.direct, wheelhouse=self.wheels, output=self.output,
                                  target_python=self.args[-5], target_platform=self.args[-3],
                                  target_os=self.args[-1])

    def test_conditional_runtime_dependency_closure(self):
        version = f'{sys.version_info.major}.{sys.version_info.minor}'
        self.wheel('requests', dependencies=(
            f'fixture-dependency>=1.0; python_version == "{version}" and sys_platform == "{sys.platform}"',
            'absent-inactive>=1.0; python_version == "0.0"',
            'absent-platform>=1.0; sys_platform == "never-a-platform"'))
        result = self.run_tool()
        self.assertEqual(result.returncode, 0, result.stderr)
        lock = (self.output / 'requirements.lock').read_text()
        self.assertIn('fixture-dependency==1.0', lock)
        self.assertNotIn('absent-', lock)
        # The same active marker must fail when its transitive wheel is missing.
        for path in self.output.iterdir():
            path.unlink()
        self.output.rmdir()
        (self.wheels / 'fixture_dependency-1.0-py3-none-any.whl').unlink()
        self.refused(self.run_tool(), 'offline wheel resolution failed')

    def test_both_reports_validate_artifacts_and_duplicates(self):
        original = helper.resolve
        for stage in (1, 2):
            for fault in ('outside', 'digest', 'duplicate', 'malformed'):
                with self.subTest(stage=stage, fault=fault):
                    calls = []

                    def corrupted(*args):
                        report = copy.deepcopy(original(*args))
                        calls.append(True)
                        if len(calls) == stage:
                            if fault == 'outside':
                                report['install'][0]['download_info']['url'] = 'file:///outside/changed.whl'
                            elif fault == 'digest':
                                report['install'][0]['download_info']['archive_info']['hashes']['sha256'] = '0' * 64
                            elif fault == 'duplicate':
                                report['install'].append(copy.deepcopy(report['install'][0]))
                            else:
                                report['install'][0]['metadata'] = []
                        return report

                    with mock.patch.object(helper, 'resolve', side_effect=corrupted):
                        with self.assertRaises(ValueError):
                            helper.generate(self.generate_args())
                    self.assertFalse(self.output.exists())

    def test_compressed_bombs_refused_before_resolver(self):
        path = self.wheels / 'requests-1.0-py3-none-any.whl'
        for members, size in ((1, 2 * helper.LIMIT), (5, 14 * helper.LIMIT)):
            with self.subTest(members=members):
                with zipfile.ZipFile(path, 'w', compression=zipfile.ZIP_DEFLATED) as wheel:
                    wheel.writestr('requests-1.0.dist-info/METADATA',
                                   'Metadata-Version: 2.1\nName: requests\nVersion: 1.0\n')
                    for index in range(members):
                        # RECORD is eagerly read by pip; payload tests aggregate bound.
                        name = ('requests-1.0.dist-info/RECORD' if members == 1 else f'payload-{index}')
                        wheel.writestr(name, b'x' * size)
                self.assertLess(path.stat().st_size, helper.LIMIT)
                with mock.patch.object(helper, 'resolve') as resolver:
                    with self.assertRaisesRegex(ValueError, 'decompression bound exceeded'):
                        helper.generate(self.generate_args())
                    resolver.assert_not_called()
                self.assertFalse(self.output.exists())

    def test_output_symlink_swap_cannot_overwrite_existing_file(self):
        unrelated = self.root / 'unrelated'
        unrelated.mkdir()
        sentinel = unrelated / 'requirements.lock'
        sentinel.write_bytes(b'keep-existing')
        original_mkdir = helper.os.mkdir

        def swapped(path, *args, **kwargs):
            original_mkdir(path, *args, **kwargs)
            if path == self.output.name and kwargs.get('dir_fd') is not None:
                self.output.rename(self.root / 'moved-output')
                self.output.symlink_to(unrelated, target_is_directory=True)

        with mock.patch.object(helper.os, 'mkdir', side_effect=swapped):
            with self.assertRaises(OSError):
                helper.write_output(self.output, b'bad-overwrite', {})
        self.assertEqual(sentinel.read_bytes(), b'keep-existing')
        self.assertFalse((unrelated / 'provenance.json').exists())

    def test_output_swap_after_open_cleans_only_owned_directory(self):
        unrelated = self.root / 'unrelated'
        unrelated.mkdir()
        sentinel = unrelated / 'requirements.lock'
        sentinel.write_bytes(b'keep-existing')
        original_open = helper.os.open
        moved = self.root / 'moved-output'

        def swapped(path, *args, **kwargs):
            fd = original_open(path, *args, **kwargs)
            if path == 'requirements.lock' and kwargs.get('dir_fd') is not None:
                self.output.rename(moved)
                self.output.symlink_to(unrelated, target_is_directory=True)
            return fd

        with mock.patch.object(helper.os, 'open', side_effect=swapped):
            with self.assertRaisesRegex(ValueError, 'directory changed'):
                helper.write_output(self.output, b'candidate', {})
        self.assertEqual(sentinel.read_bytes(), b'keep-existing')
        self.assertEqual(list(moved.iterdir()), [])
        self.assertFalse((unrelated / 'provenance.json').exists())

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
