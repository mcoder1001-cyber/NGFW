#!/usr/bin/env python3
"""Actual installer entries, exclusively recording stubs and disposable roots."""
import importlib.util
from pathlib import Path
import unittest
import subprocess
import os

spec = importlib.util.spec_from_file_location('fixture', Path(__file__).with_name('td19-safe-root-fixtures.py'))
fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture)


class SafeRoot(unittest.TestCase):
    def setUp(self):
        self.f = fixture.Fixture()
        self.addCleanup(self.f.close)

    def test_dry_run_has_no_effect_commands_or_writes(self):
        self.f.env['NGFW_VPP_ARTIFACTS'] = str(self.f.base / 'not-created')
        for script in ('00-add-repos.sh', '20-install-build.sh', '40-install-lab.sh'):
            result = self.f.run(script, ['--dry-run'])
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn('DRY-RUN', result.stdout)
            self.assertEqual(self.f.calls(), [])
            self.assertEqual(list(self.f.root.iterdir()), [])

    def test_alternate_root_without_stubs_refused(self):
        self.f.env.pop('NGFW_INSTALL_STUBS')
        result = self.f.run('20-install-build.sh')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('recording stubs', result.stderr)
        self.assertEqual(self.f.calls(), [])

    def test_shadowed_effect_command_refused(self):
        (self.f.stubs / 'apt-get').unlink()
        result = self.f.run('20-install-build.sh')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('recording harness', result.stderr)
        self.assertEqual(self.f.calls(), [])

    def test_unknown_regular_command_and_rooted_executables_refused(self):
        sentinel = self.f.base / 'outside-sentinel'
        payload = '#!/bin/sh\necho unsafe > "' + str(sentinel) + '"\necho "go version go1.26.0 linux/amd64"\n'
        for destination in (self.f.stubs / 'apt-get', self.f.root / 'usr/local/go/bin/go',
                            self.f.root / 'opt/ngfw-build/go/bin/corepack', self.f.root / 'opt/ngfw-test/bin/pip'):
            with self.subTest(destination=str(destination)):
                destination.parent.mkdir(parents=True, exist_ok=True)
                destination.write_text(payload); destination.chmod(0o755)
                result = self.f.run('20-install-build.sh')
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('REFUSED', result.stderr)
                self.assertFalse(sentinel.exists())
                self.assertEqual(self.f.calls(), [])
                if destination.parent == self.f.stubs:
                    destination.write_bytes((fixture.ROOT / 'scripts/install-recording-stub.py').read_bytes())
                else:
                    destination.unlink()

    def test_symlink_escape_refused_before_commands(self):
        (self.f.root / 'usr').symlink_to(self.f.base, target_is_directory=True)
        result = self.f.run('20-install-build.sh')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('symlinks', result.stderr)
        self.assertEqual(self.f.calls(), [])

    def test_native_metadata_path_is_read_only_and_standard_symlink_supported(self):
        helper = fixture.ROOT / 'scripts/install-common.sh'
        result = subprocess.run(['bash', '-c', 'source "$1"; NGFW_INSTALL_ROOT=/; ngfw_install_os_release',
                                 'native-metadata-fixture', str(helper)],
                                env=dict(os.environ, PATH='/usr/bin:/bin'), text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        metadata = Path(result.stdout.strip())
        expected = Path('/usr/lib/os-release') if Path('/etc/os-release').is_symlink() else Path('/etc/os-release')
        self.assertEqual(metadata, expected)
        self.assertTrue(metadata.is_file())
        self.assertFalse(metadata.is_symlink())
        self.assertEqual(self.f.calls(), [])

    def test_bad_go_archive_refused_before_tar(self):
        result = self.f.run('20-install-build.sh')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('curl', [call[0] for call in self.f.calls()])
        self.assertNotIn('tar', [call[0] for call in self.f.calls()])
        self.assertFalse((self.f.root / 'etc/profile.d/go.sh').exists())

    def test_build_success_is_rooted_and_pnpm_pinned(self):
        self.f.existing_go()
        self.f.env['COREPACK_FAIL'] = '1'
        result = self.f.run('20-install-build.sh')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(['npm', 'install', '-g', 'pnpm@12.5.1'], self.f.calls())
        installs = [c for c in self.f.calls() if c[:2] == ['go', 'install']]
        self.assertEqual(len(installs), 3)
        self.assertTrue((self.f.root / 'etc/profile.d/go.sh').is_file())

    def test_lab_lock_missing_or_floating_refused_before_apt(self):
        for value in (None, 'robotframework\n', 'pip==1.0 --hash=sha256:' + 'a'*64):
            if value is None:
                self.f.env.pop('NGFW_LAB_REQUIREMENTS')
            else:
                self.f.env['NGFW_LAB_REQUIREMENTS'] = str(self.f.lock)
                self.f.lock.write_text(value)
            result = self.f.run('40-install-lab.sh')
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(self.f.calls(), [])
            self.assertEqual(list(self.f.root.iterdir()), [])

    def test_lab_success_uses_hashed_snapshot_in_fake_venv(self):
        result = self.f.run('40-install-lab.sh')
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.f.calls()
        self.assertIn(['python3', '-m', 'venv', str(self.f.root / 'opt/ngfw-test')], calls)
        pip = next(c for c in calls if c[0] == 'pip')
        self.assertIn('--require-hashes', pip)
        self.assertTrue(pip[-1].startswith(str(self.f.root / 'tmp') + '/'))
        self.assertNotEqual(pip[-1], str(self.f.lock))
        self.assertEqual(list((self.f.root / 'tmp').glob('ngfw-lab-lock.*')), [])

    def test_repo_changed_pins_refused_even_in_dry_run(self):
        self.f.env.update(NGFW_VPP_ARTIFACTS=str(self.f.base), NGFW_FRR_KEY_FINGERPRINTS='A'*40)
        result = self.f.run('00-add-repos.sh', ['--dry-run'])
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('D-238', result.stderr)
        self.assertEqual(self.f.calls(), [])


if __name__ == '__main__':
    unittest.main(verbosity=2)
