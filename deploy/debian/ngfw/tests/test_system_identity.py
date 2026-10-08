#!/usr/bin/python3
"""Real filesystem migration fixtures, isolated from the host identity."""
import importlib.util
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest

SOURCE = pathlib.Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location('identity', SOURCE / 'assets/provision-system-identity.py')
IDENTITY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(IDENTITY)


class Identity(unittest.TestCase):
    def fixture(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = pathlib.Path(temporary.name)
        (root / 'etc').mkdir()
        for name in IDENTITY.FILES:
            if name == 'localtime':
                (root / 'etc' / name).symlink_to('/usr/share/zoneinfo/Asia/Tehran')
            else:
                (root / 'etc' / name).write_bytes(('existing ' + name).encode())
        return root

    def test_migration_preserves_content_and_reconfiguration(self):
        root = self.fixture()
        IDENTITY.provision(str(root))
        state = root / IDENTITY.STATE.lstrip('/')
        for name in IDENTITY.FILES:
            self.assertEqual(os.readlink(root / 'etc' / name), IDENTITY.STATE + '/' + name)
            if name != 'localtime':
                self.assertEqual((state / name).read_bytes(), ('existing ' + name).encode())
                self.assertEqual((state / name).stat().st_mode & 0o777, 0o644)
        self.assertEqual(os.readlink(state / 'localtime'), '/usr/share/zoneinfo/Asia/Tehran')
        (state / 'hostname').write_bytes(b'new runtime hostname')
        IDENTITY.provision(str(root))
        self.assertEqual((state / 'hostname').read_bytes(), b'new runtime hostname')
        self.assertEqual(state.stat().st_mode & 0o777, 0o755)
        self.assertEqual(state.stat().st_uid, 0)
        self.assertTrue((root / 'etc/systemd/resolved.conf.d').is_dir())

    def test_all_sources_preflighted_before_any_replacement(self):
        for kind in ('symlink', 'directory', 'hardlink', 'writable', 'oversize'):
            with self.subTest(kind=kind):
                root = self.fixture()
                target = root / 'etc/motd'
                target.unlink()
                if kind == 'symlink':
                    target.symlink_to('/etc/passwd')
                elif kind == 'directory':
                    target.mkdir()
                elif kind == 'hardlink':
                    os.link(root / 'etc/hostname', target)
                else:
                    target.write_bytes(b'x' * (IDENTITY.LIMIT + 1) if kind == 'oversize' else b'keep')
                    if kind == 'writable':
                        target.chmod(0o666)
                with self.assertRaises((ValueError, PermissionError)):
                    IDENTITY.provision(str(root))
                self.assertFalse((root / 'etc/hostname').is_symlink())
                self.assertEqual((root / 'etc/hostname').read_bytes(), b'existing hostname')

    def test_refuses_parent_symlink_or_writable_parent(self):
        for kind in ('symlink', 'writable'):
            with self.subTest(kind=kind):
                root = self.fixture()
                (root / 'var/lib').mkdir(parents=True)
                state = root / IDENTITY.STATE.lstrip('/')
                if kind == 'symlink':
                    external = root / 'external'
                    external.mkdir()
                    (external / 'sentinel').write_bytes(b'keep')
                    state.symlink_to(external)
                else:
                    state.mkdir()
                    state.chmod(0o777)
                with self.assertRaises(OSError):
                    IDENTITY.provision(str(root))
                self.assertFalse((root / 'etc/hostname').is_symlink())
                if kind == 'symlink':
                    self.assertEqual(list(external.iterdir()), [external / 'sentinel'])

    def test_refuses_foreign_owned_parent(self):
        root = self.fixture()
        state = root / IDENTITY.STATE.lstrip('/')
        state.mkdir(parents=True)
        os.chown(state, 65534, 65534)
        with self.assertRaises(PermissionError):
            IDENTITY.provision(str(root))
        self.assertFalse((root / 'etc/hostname').is_symlink())

    def test_replay_after_target_written_before_link_replacement(self):
        root = self.fixture()
        state = root / IDENTITY.STATE.lstrip('/')
        state.mkdir(parents=True)
        (state / 'hostname').write_bytes(b'existing hostname')
        IDENTITY.provision(str(root))
        self.assertTrue((root / 'etc/hostname').is_symlink())

    def test_conflicting_target_is_not_overwritten(self):
        root = self.fixture()
        state = root / IDENTITY.STATE.lstrip('/')
        state.mkdir(parents=True)
        (state / 'hostname').write_bytes(b'other content')
        with self.assertRaises(ValueError):
            IDENTITY.provision(str(root))
        self.assertEqual((state / 'hostname').read_bytes(), b'other content')
        self.assertFalse((root / 'etc/hostname').is_symlink())

    def test_timezone_escape_rejected(self):
        root = self.fixture()
        (root / 'etc/localtime').unlink()
        (root / 'etc/localtime').symlink_to('/usr/share/zoneinfo/../../etc/passwd')
        with self.assertRaises(ValueError):
            IDENTITY.provision(str(root))
        self.assertFalse((root / 'etc/hostname').is_symlink())

    def test_packaged_narrow_sandbox_paths(self):
        unit = (SOURCE.parents[2] / 'deploy/systemd/ngfw-agent.service').read_text()
        self.assertIn('ProtectSystem=strict', unit)
        self.assertIn('CAP_CHOWN', unit)
        self.assertIn('ReadWritePaths=/var/lib/ngfw-system-identity /etc/systemd/resolved.conf.d', unit)
        for line in unit.splitlines():
            if line.startswith('ReadWritePaths='):
                self.assertNotIn('/etc', line.split('=', 1)[1].split())
        self.assertIn('assets/provision-system-identity.py usr/lib/ngfw/',
                      (SOURCE / 'debian/ngfw-agent.install').read_text())
        self.assertIn('/usr/bin/python3 /usr/lib/ngfw/provision-system-identity.py',
                      (SOURCE / 'debian/ngfw-agent.postinst').read_text())


if __name__ == '__main__':
    if os.geteuid() != 0:
        # These fixtures verify actual root ownership. Never silently skip it.
        raise SystemExit(subprocess.call(['sudo', '-n', sys.executable, __file__, *sys.argv[1:]]))
    unittest.main()
