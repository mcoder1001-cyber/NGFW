#!/usr/bin/python3
"""Real filesystem migration fixtures, isolated from the host identity."""
import functools
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


def root_fixture(test):
    @functools.wraps(test)
    def execute(self):
        if os.geteuid() == 0:
            return test(self)
        result = subprocess.run(['sudo', '-n', sys.executable, __file__,
                                 'Identity.' + test.__name__], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('Ran 1 test', result.stderr)
        self.assertNotIn('skipped', result.stderr)
    return execute


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

    @root_fixture
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

    @root_fixture
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

    @root_fixture
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

    @root_fixture
    def test_refuses_foreign_owned_parent(self):
        root = self.fixture()
        state = root / IDENTITY.STATE.lstrip('/')
        state.mkdir(parents=True)
        os.chown(state, 65534, 65534)
        with self.assertRaises(PermissionError):
            IDENTITY.provision(str(root))
        self.assertFalse((root / 'etc/hostname').is_symlink())

    @root_fixture
    def test_replay_after_target_written_before_link_replacement(self):
        root = self.fixture()
        state = root / IDENTITY.STATE.lstrip('/')
        state.mkdir(parents=True)
        (state / 'hostname').write_bytes(b'existing hostname')
        IDENTITY.provision(str(root))
        self.assertTrue((root / 'etc/hostname').is_symlink())

    @root_fixture
    def test_conflicting_target_is_not_overwritten(self):
        root = self.fixture()
        state = root / IDENTITY.STATE.lstrip('/')
        state.mkdir(parents=True)
        (state / 'hostname').write_bytes(b'other content')
        with self.assertRaises(ValueError):
            IDENTITY.provision(str(root))
        self.assertEqual((state / 'hostname').read_bytes(), b'other content')
        self.assertFalse((root / 'etc/hostname').is_symlink())

    @root_fixture
    def test_timezone_escape_rejected(self):
        root = self.fixture()
        (root / 'etc/localtime').unlink()
        (root / 'etc/localtime').symlink_to('/usr/share/zoneinfo/../../etc/passwd')
        with self.assertRaises(ValueError):
            IDENTITY.provision(str(root))
        self.assertFalse((root / 'etc/hostname').is_symlink())

    @root_fixture
    def test_agent_capabilities_restore_foreign_private_config(self):
        root = self.fixture()
        daemon = root / 'daemon-config'
        daemon.mkdir(mode=0o700)
        config = daemon / 'secret.conf'
        config.write_bytes(b'NGFW_TEST_PRIVATE_CONFIG')
        config.chmod(0o600)
        os.chown(config, 65534, 65534)
        os.chown(daemon, 65534, 65534)
        script = r"""import os, pathlib, sys
folder = pathlib.Path(sys.argv[1])
path = folder / 'secret.conf'
try:
    content = path.read_bytes()
    old = path.stat()
except PermissionError:
    raise SystemExit(77)
def atomic(content):
    temp = folder / '.fixture-next'
    fd = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    try:
        os.write(fd, content)
        os.fchmod(fd, 0o600)
        try:
            os.fchown(fd, old.st_uid, old.st_gid)
        except PermissionError:
            raise SystemExit(78)
        os.fsync(fd)
    finally:
        os.close(fd)
    os.replace(temp, path)
    directory = os.open(folder, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(directory)
    finally:
        os.close(directory)
atomic(b'changed fixture')
assert path.read_bytes() == b'changed fixture'
atomic(content)
restored = path.stat()
assert path.read_bytes() == content
assert (restored.st_uid, restored.st_gid, restored.st_mode) == (old.st_uid, old.st_gid, old.st_mode)
"""
        # Prove each added capability is necessary; the old set has neither.
        for caps, expected in (('chown', 77), ('dac_override', 78), ('chown,+dac_override', 0)):
            with self.subTest(caps=caps):
                pending = daemon / '.fixture-next'
                if pending.exists():
                    pending.unlink()
                result = subprocess.run(['setpriv', '--bounding-set=-all,+' + caps,
                                         sys.executable, '-c', script, str(daemon)],
                                        capture_output=True, text=True)
                self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
        self.assertEqual(config.read_bytes(), b'NGFW_TEST_PRIVATE_CONFIG')

    @root_fixture
    def test_packaged_narrow_sandbox_paths(self):
        unit_path = SOURCE.parents[2] / 'deploy/systemd/ngfw-agent.service'
        if not unit_path.is_file():
            unit_path = SOURCE / 'stage/usr/lib/systemd/system/ngfw-agent.service'
        unit = unit_path.read_text()
        self.assertIn('ProtectSystem=strict', unit)
        capabilities = next(line.split('=', 1)[1].split() for line in unit.splitlines()
                            if line.startswith('CapabilityBoundingSet='))
        self.assertEqual(set(capabilities), {'CAP_NET_ADMIN', 'CAP_SYS_ADMIN', 'CAP_IPC_LOCK',
                                             'CAP_CHOWN', 'CAP_DAC_OVERRIDE'})
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
