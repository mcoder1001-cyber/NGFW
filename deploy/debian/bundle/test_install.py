"""Real synthetic dpkg fixtures plus captured commands; never installs packages."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('bundle_install', Path(__file__).with_name('install.py'))
INSTALL = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(INSTALL)
VERIFY = INSTALL.VERIFY


class InstallTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.delivery = self.root / 'delivery'
        self.delivery.mkdir()
        (self.delivery / 'vpp').mkdir()
        self.gate_calls = []
        original = VERIFY.run
        def gate(argv, limit=1024 * 1024, pass_fds=()):
            if argv[0] == 'bash':
                self.gate_calls.append(argv)
                self.assertIn('--install-gate', argv)
                self.assertIn('--require-files', argv)
                return 'synthetic VPP boundary only'
            return original(argv, limit, pass_fds)
        self.gate = patch.object(VERIFY, 'run', gate)
        self.gate.start()
        self.addCleanup(self.gate.stop)
        for name in sorted(VERIFY.runtime_roots()):
            self.archive(name, self.delivery / (name + '.deb'))
        vpp = self.delivery / 'vpp/vpp.deb'
        self.archive('vpp', vpp, '26.06-release+vrx1')
        metadata = VERIFY.metadata(vpp)
        (self.delivery / 'vpp/manifest.json').write_text(json.dumps({'packages': [
            {'package': 'vpp', 'file': 'vpp.deb', 'ship': True,
             'version': '26.06-release+vrx1', 'architecture': 'amd64',
             'sha256': metadata['sha256'], 'size': metadata['size']}]}))
        (self.delivery / 'vpp/SHA256SUMS').write_text('synthetic boundary fixture\n')
        self.plan = VERIFY.verify(self.delivery)
        self.manifest = self.root / 'trusted.json'
        self.manifest.write_text(json.dumps(self.plan))

    def archive(self, name, target, version='1.0'):
        source = self.root / ('source-' + name)
        (source / 'DEBIAN').mkdir(parents=True)
        (source / 'DEBIAN/control').write_text(
            f'Package: {name}\nVersion: {version}\nArchitecture: amd64\n'
            'Maintainer: Fixture <test@example.invalid>\nDescription: synthetic only\n')
        subprocess.run(['dpkg-deb', '--build', '--root-owner-group', str(source), str(target)],
                       capture_output=True, check=True)

    def test_default_plan_never_invokes_install(self):
        with patch.object(INSTALL, 'execute') as execute, patch.object(INSTALL, 'target_check') as target:
            with patch('sys.stdout'):
                self.assertEqual(INSTALL.main([str(self.delivery), '--manifest', str(self.manifest)]), 0)
            execute.assert_not_called()
            target.assert_not_called()

    def test_snapshot_independent_private_and_cleanup(self):
        with INSTALL.prepared(self.delivery, self.manifest) as (private, snapshot, plan):
            self.assertEqual(private.stat().st_mode & 0o777, 0o700)
            self.assertEqual((snapshot / 'vrx-agent.deb').stat().st_mode & 0o777, 0o600)
            (self.delivery / 'vrx-agent.deb').write_bytes(b'replaced source')
            INSTALL.check_archives(snapshot, plan)
            self.assertEqual(plan, self.plan)
        self.assertFalse(private.exists())

    def test_external_manifest_required_and_mismatch_rejected(self):
        inside = self.delivery / 'trusted.json'
        inside.write_text(self.manifest.read_text())
        with self.assertRaises(INSTALL.InvalidBundle):
            with INSTALL.prepared(self.delivery, inside):
                self.fail('internal manifest accepted')
        expected = self.plan.copy()
        expected['product_version'] = 'untrusted'
        self.manifest.write_text(json.dumps(expected))
        with self.assertRaises(INSTALL.InvalidBundle):
            with INSTALL.prepared(self.delivery, self.manifest):
                self.fail('mismatch accepted')

    def test_symlink_and_size_bound_fail_closed(self):
        (self.delivery / 'alias.deb').symlink_to(self.delivery / 'vrx-agent.deb')
        with self.assertRaises(INSTALL.InvalidBundle):
            with INSTALL.prepared(self.delivery, self.manifest):
                self.fail('symlink accepted')
        (self.delivery / 'alias.deb').unlink()
        with patch.object(VERIFY, 'MAX_ARCHIVE', 1):
            with self.assertRaises(INSTALL.InvalidBundle):
                with INSTALL.prepared(self.delivery, self.manifest):
                    self.fail('oversized accepted')

    def test_source_mutation_during_snapshot_rejected(self):
        original = INSTALL.os.read
        changed = False
        def mutate(fd, amount):
            nonlocal changed
            result = original(fd, amount)
            if result and not changed:
                changed = True
                (self.delivery / 'vrx-agent.deb').write_bytes(b'replacement')
            return result
        with patch.object(INSTALL.os, 'read', mutate):
            with self.assertRaises(INSTALL.InvalidBundle):
                with INSTALL.prepared(self.delivery, self.manifest):
                    self.fail('mutation accepted')

    def test_target_identity_checks(self):
        with patch.object(INSTALL.os, 'geteuid', return_value=1000):
            with self.assertRaises(INSTALL.InvalidBundle):
                INSTALL.target_check()
        with patch.object(INSTALL.os, 'geteuid', return_value=0), patch.object(
                INSTALL.Path, 'read_text', return_value='ID=debian\nVERSION_ID="26.04"\n'):
            with self.assertRaises(INSTALL.InvalidBundle):
                INSTALL.target_check()
        with patch.object(INSTALL.os, 'geteuid', return_value=0), patch.object(
                INSTALL.Path, 'read_text', return_value='ID=ubuntu\nVERSION_ID="26.04"\n'), patch.object(
                VERIFY, 'run', return_value='arm64\n'):
            with self.assertRaises(INSTALL.InvalidBundle):
                INSTALL.target_check()

    def test_apt_simulation_then_install_isolated_and_local_only(self):
        with INSTALL.prepared(self.delivery, self.manifest) as (private, snapshot, plan):
            with patch.object(INSTALL.subprocess, 'run') as run:
                INSTALL.execute(private, snapshot, plan)
            self.assertEqual(run.call_count, 2)
            first, second = run.call_args_list
            self.assertIn('--simulate', first.args[0])
            self.assertNotIn('--simulate', second.args[0])
            for call in (first, second):
                argv = call.args[0]
                self.assertEqual(argv[0], '/usr/bin/apt-get')
                self.assertIn('--no-download', argv)
                self.assertIn('--no-remove', argv)
                self.assertNotIn('update', argv)
                self.assertNotIn('--allow-downgrades', argv)
                self.assertEqual(argv[argv.index('--') + 1:],
                                 [str(snapshot / path) for path in plan['install_files']])
                self.assertIn('Dir::Etc::parts=parts', argv)
                self.assertIn('Acquire::http::Proxy=false', argv)
                self.assertIn('Acquire::https::Proxy=false', argv)
                self.assertEqual(call.kwargs['env']['APT_CONFIG'], str(private / 'apt-bootstrap.conf'))
                self.assertNotIn('http_proxy', call.kwargs['env'])
                self.assertTrue(call.kwargs['check'])
                self.assertNotIn('shell', call.kwargs)
            self.assertEqual((private / 'apt-etc/sources.list').read_text(), '')
            self.assertEqual(list((private / 'apt-etc/parts').iterdir()), [])

    def test_simulation_failure_prevents_install_and_cleans(self):
        private = None
        with self.assertRaises(subprocess.CalledProcessError):
            with INSTALL.prepared(self.delivery, self.manifest) as (private, snapshot, plan):
                with patch.object(INSTALL.subprocess, 'run', side_effect=subprocess.CalledProcessError(
                        100, 'apt-get')) as run:
                    INSTALL.execute(private, snapshot, plan)
                self.fail('simulation failure ignored')
        self.assertEqual(run.call_count, 1)
        self.assertFalse(private.exists())

    def test_hostile_environment_removed_through_preflight(self):
        malicious = self.root / 'hostile-bin'
        malicious.mkdir()
        marker = self.root / 'executed'
        for name in ('dpkg', 'dpkg-deb', 'bash'):
            executable = malicious / name
            executable.write_text('#!/bin/sh\nprintf bad > "' + str(marker) + '"\nexit 0\n')
            executable.chmod(0o700)
        hostile = {'PATH': str(malicious), 'BASH_ENV': str(malicious / 'bash'),
                   'APT_CONFIG': '/caller/apt.conf', 'LD_PRELOAD': '/caller/loader.so',
                   'PYTHONPATH': '/caller/python', 'TMPDIR': str(malicious),
                   'http_proxy': 'http://caller.invalid'}
        with patch.dict(os.environ, hostile):
            with INSTALL.prepared(self.delivery, self.manifest) as (private, _, _):
                self.assertEqual(private.parent, Path('/var/tmp'))
                self.assertEqual(dict(os.environ), {
                    'PATH': '/usr/sbin:/usr/bin:/sbin:/bin', 'LC_ALL': 'C',
                    'HOME': '/nonexistent'})
            self.assertEqual(os.environ['PATH'], str(malicious))
        self.assertFalse(marker.exists())

    def test_entry_and_depth_bounds(self):
        with patch.object(VERIFY, 'MAX_PACKAGES', 1):
            with self.assertRaises(INSTALL.InvalidBundle):
                with INSTALL.prepared(self.delivery, self.manifest):
                    self.fail('entry bound ignored')
        nested = self.delivery
        for _ in range(34):
            nested /= 'nested'
            nested.mkdir()
        with self.assertRaises(INSTALL.InvalidBundle):
            with INSTALL.prepared(self.delivery, self.manifest):
                self.fail('depth bound ignored')

    def test_private_archive_mutation_rejected_before_apt(self):
        with INSTALL.prepared(self.delivery, self.manifest) as (private, snapshot, plan):
            archive = snapshot / 'vrx-agent.deb'
            content = archive.read_bytes()
            archive.write_bytes(content[:-1] + bytes([content[-1] ^ 1]))
            with patch.object(INSTALL.subprocess, 'run') as run:
                with self.assertRaises(INSTALL.InvalidBundle):
                    INSTALL.execute(private, snapshot, plan)
                run.assert_not_called()


if __name__ == '__main__':
    result = unittest.TextTestRunner(verbosity=2).run(
        unittest.defaultTestLoader.loadTestsFromTestCase(InstallTests))
    sys.exit(0 if result.testsRun and result.wasSuccessful() and not result.skipped
             and not result.expectedFailures else 1)
