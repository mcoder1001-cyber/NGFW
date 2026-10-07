"""Host-independent adversarial verification and actual grubenv lifecycle tests."""
import copy
import hashlib
import importlib.machinery
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
import runpy
from unittest.mock import patch

BASE = Path(__file__).resolve().parents[1]
loader = importlib.machinery.SourceFileLoader('upgrade', str(BASE / 'ngfw-upgrade'))
spec = importlib.util.spec_from_loader(loader.name, loader)
u = importlib.util.module_from_spec(spec)
loader.exec_module(u)


def root_tar(path, extra=None):
    files = {'etc/ngfw/appliance': b'ngfw\n', 'boot/vmlinuz': b'kernel', 'boot/initrd.img': b'initrd',
             'usr/sbin/ngfw-upgrade': b'cli', 'usr/lib/ngfw/ngfw-upgrade-prepare': b'prepare',
             'usr/lib/ngfw/ngfw-upgrade-health': b'health', 'usr/lib/ngfw/ngfw-upgrade-probe': b'probe',
             'usr/lib/systemd/system/ngfw-upgrade-health.service': b'unit',
             'usr/lib/systemd/system/ngfw-upgrade-prepare.service': b'unit',
             'usr/share/ngfw/vpp-manifest.json': b'{"version":"26.06-release+ngfw1"}\n'}
    with tarfile.open(path, 'w') as tf:
        for name, content in files.items():
            m = tarfile.TarInfo(name)
            m.size = len(content)
            m.mode = 0o644
            m.uid = os.getuid(); m.gid = os.getgid()
            tf.addfile(m, io.BytesIO(content))
        if extra:
            m, content = extra
            tf.addfile(m, io.BytesIO(content))
    return files


class Verification(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name)
        self.key = self.path / 'private.pem'
        self.pub = self.path / 'public.pem'
        u.run('openssl', 'genpkey', '-algorithm', 'ED25519', '-out', str(self.key))
        u.run('openssl', 'pkey', '-in', str(self.key), '-pubout', '-out', str(self.pub))
        os.chmod(self.key, 0o600)

    def bundle(self, extra=None, changes=None, signed=True):
        files = root_tar(self.path / 'root.tar', extra)
        u.run('zstd', '-q', '-f', '-o', str(self.path / 'rootfs.tar.zst'), str(self.path / 'root.tar'))
        m = {'format': 1, 'version': '1.1.0', 'min_from_version': '1.0.0',
             'migration_policy': 'backward-compatible',
             'sha256': {'rootfs.tar.zst': hashlib.sha256((self.path / 'rootfs.tar.zst').read_bytes()).hexdigest()},
             'vpp_manifest_sha256': hashlib.sha256(files['usr/share/ngfw/vpp-manifest.json']).hexdigest()}
        if changes:
            m.update(changes)
        (self.path / 'manifest.json').write_bytes(u.json_bytes(m))
        if signed:
            u.run('openssl', 'pkeyutl', '-sign', '-inkey', str(self.key), '-rawin',
                  '-in', str(self.path / 'manifest.json'), '-out', str(self.path / 'manifest.sig'))
        out = self.path / 'bundle.tar'
        with tarfile.open(out, 'w') as tf:
            for n in ('manifest.json', 'rootfs.tar.zst') + (('manifest.sig',) if signed else ()):
                tf.add(self.path / n, arcname=n)
        return out

    def verify(self, bundle):
        with tempfile.TemporaryDirectory(dir=self.path) as work:
            return u.verify(bundle, self.pub, '1.0.0', Path(work))

    def test_valid_signature_digest_and_snapshot(self):
        self.assertEqual(self.verify(self.bundle())['version'], '1.1.0')

    def test_unsigned_and_tampered_refused(self):
        with self.assertRaises(u.Refused):
            self.verify(self.bundle(signed=False))
        bundle = self.bundle()
        with tarfile.open(bundle) as tf:
            payloads = {m.name: tf.extractfile(m).read() for m in tf}
        raw = bytearray(payloads['rootfs.tar.zst'])
        raw[len(raw) // 2] ^= 1
        payloads['rootfs.tar.zst'] = raw
        with tarfile.open(bundle, 'w') as tf:
            for name, raw in payloads.items():
                m = tarfile.TarInfo(name)
                m.size = len(raw)
                tf.addfile(m, io.BytesIO(raw))
        with self.assertRaisesRegex(u.Refused, 'digest mismatch'):
            self.verify(bundle)

    def test_manifest_signature_tampered(self):
        bundle = self.bundle()
        with tarfile.open(bundle) as tf:
            payloads = {m.name: tf.extractfile(m).read() for m in tf}
        payloads['manifest.json'] = payloads['manifest.json'].replace(b'1.1.0', b'1.2.0')
        with tarfile.open(bundle, 'w') as tf:
            for name, raw in payloads.items():
                m = tarfile.TarInfo(name); m.size = len(raw)
                tf.addfile(m, io.BytesIO(raw))
        with self.assertRaises(subprocess.CalledProcessError):
            self.verify(bundle)

    def test_compatibility_and_vpp_provenance(self):
        for changes in ({'min_from_version': '2.0.0'}, {'version': '1.0.0'},
                        {'migration_policy': 'destructive'}, {'vpp_manifest_sha256': '0' * 64}):
            with self.subTest(changes=changes), self.assertRaises(u.Refused):
                self.verify(self.bundle(changes=changes))

    def test_traversal_special_nodes_symlink_ancestors_persistent_data(self):
        for name, kind, link in [('.. /bad', tarfile.CHRTYPE, ''), ('../bad', tarfile.REGTYPE, ''),
                                 ('/bad', tarfile.REGTYPE, ''), ('dev/evil', tarfile.REGTYPE, ''),
                                 ('data/leak', tarfile.REGTYPE, ''), ('var/lib/ngfw/leak', tarfile.REGTYPE, ''),
                                 ('usr/lib/ngfw', tarfile.SYMTYPE, '../../../escape'),
                                 ('boot/vmlinuz', tarfile.REGTYPE, '')]:
            m = tarfile.TarInfo(name); m.type = kind; m.linkname = link
            with self.subTest(name=name), self.assertRaises(u.Refused):
                self.verify(self.bundle(extra=(m, b'')))
        m = tarfile.TarInfo('usr/lib'); m.type = tarfile.SYMTYPE; m.linkname = '../other'
        with self.assertRaisesRegex(u.Refused, 'below symlink'):
            self.verify(self.bundle(extra=(m, b'')))

    def test_host_without_appliance_marker_refused(self):
        result = subprocess.run([str(BASE / 'ngfw-upgrade'), '--root', str(self.path), 'status', '--json'], capture_output=True)
        self.assertEqual(result.returncode, 1)
        self.assertFalse((self.path / 'data').exists())

    @unittest.skipUnless(os.geteuid() == 0, 'restoring daemon UID/GID requires root')
    def test_directory_ownership_restored_after_population(self):
        archive = self.path / 'dirs.tar'
        with tarfile.open(archive, 'w') as tf:
            m = tarfile.TarInfo('var/lib/valkey'); m.type = tarfile.DIRTYPE
            m.uid = 65534; m.gid = 65534; m.mode = 0o750
            tf.addfile(m)
            m = tarfile.TarInfo('var/lib/valkey/state'); m.size = 1
            m.uid = 65534; m.gid = 65534; m.mode = 0o600
            tf.addfile(m, io.BytesIO(b'x'))
        # Include required boot files from the common valid fixture.
        raw = self.path / 'required.tar'; root_tar(raw)
        with tarfile.open(archive, 'a') as tf, tarfile.open(raw) as source:
            for m in source:
                tf.addfile(m, source.extractfile(m))
        target = self.path / 'slot'; target.mkdir()
        u.unpack_root(archive, target, extract=True)
        self.assertEqual((target / 'var/lib/valkey').stat().st_uid, 65534)
        self.assertEqual((target / 'var/lib/valkey').stat().st_gid, 65534)
        self.assertEqual((target / 'var/lib/valkey').stat().st_mode & 0o777, 0o750)
        self.assertEqual((target / 'var/lib/valkey/state').read_bytes(), b'x')

    def test_builder_refuses_signing_key_inside_root_and_inode_alias(self):
        root = self.path / 'root'; root.mkdir()
        raw = self.path / 'source.tar'; root_tar(raw)
        u.unpack_root(raw, root, extract=True)
        inside = root / 'release.pem'
        inside.write_bytes(self.key.read_bytes()); inside.chmod(0o600)
        dest = self.path / 'refused.tar'
        args = [str(BASE / 'build-bundle'), '--root', str(root),
                '--version', '1.1.0', '--min-from-version', '1.0.0',
                '--vpp-manifest', str(root / 'usr/share/ngfw/vpp-manifest.json'), '--output', str(dest)]
        result = subprocess.run(args + ['--key', str(inside)], capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(dest.exists())
        inside.unlink(); os.link(self.key, inside)
        result = subprocess.run(args + ['--key', str(self.key)], capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(dest.exists())

    def test_builder_round_trip(self):
        root = self.path / 'root'; root.mkdir()
        raw = self.path / 'source.tar'
        root_tar(raw)
        u.unpack_root(raw, root, extract=True)
        # Production roots contain hardlinked utilities within the same subtree.
        (root / 'usr/bin').mkdir(parents=True)
        (root / 'usr/bin/original').write_bytes(b'hardlinked tool')
        os.link(root / 'usr/bin/original', root / 'usr/bin/second')
        for name in ('root/.ssh/id_ed25519', 'root/.gnupg/private-keys-v1.d/key', 'home/operator/.ssh/id_rsa'):
            credential = root / name; credential.parent.mkdir(parents=True, exist_ok=True)
            credential.write_bytes(b'operator credential must not ship')
        dest = self.path / 'built.tar'
        u.run(str(BASE / 'build-bundle'), '--root', str(root), '--key', str(self.key),
              '--version', '1.1.0', '--min-from-version', '1.0.0',
              '--vpp-manifest', str(root / 'usr/share/ngfw/vpp-manifest.json'), '--output', str(dest))
        self.assertEqual(self.verify(dest)['version'], '1.1.0')
        with tarfile.open(dest) as bundle:
            compressed = bundle.extractfile('rootfs.tar.zst').read()
        plain = subprocess.run(['zstd', '-q', '-d', '-c'], input=compressed, capture_output=True, check=True).stdout
        with tarfile.open(fileobj=io.BytesIO(plain)) as archive:
            self.assertFalse(any(name == 'root' or name.startswith(('root/', 'home/')) for name in archive.getnames()))


class Lifecycle(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(); self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        # Unit fixture runs as the hosted runner user: emulate only the root-owned
        # state storage boundary; lifecycle and real GRUB operations stay actual.
        class FixtureAppliance(u.Appliance):
            def state(self):
                return json.loads(self.statefile.read_bytes())
        self.app = object.__new__(FixtureAppliance)
        self.app.root = self.root; self.app.data = self.root / 'data'
        self.app.statefile = self.app.data / 'ngfw-upgrade/state.json'
        self.app.env = self.root / 'boot/efi/EFI/ngfw/grub/grubenv'
        self.app.active = 'A'
        self.app.env.parent.mkdir(parents=True)
        self.app.grub('create'); self.app.grub('set', 'saved_entry=ngfw-A')
        self.app.save({'format': 1, 'versions': {'A': '1.0.0', 'B': '1.1.0'},
                       'default_slot': 'A', 'staged_slot': 'B', 'pending_slot': None})

    def test_activate_confirm_actual_grubenv(self):
        self.app.activate()
        self.assertIn('next_entry=ngfw-B', self.app.grub('list').decode())
        self.assertIn('saved_entry=ngfw-A', self.app.grub('list').decode())
        # GRUB consumes one-shot state before jumping to the trial kernel.
        self.app.grub('unset', 'next_entry')
        self.app.active = 'B'
        with self.assertRaises(u.Refused):
            self.app.confirm()
        state = self.app.state(); state['migration_backup'] = '/data/pre-upgrade.dump'; self.app.save(state)
        self.app.confirm()
        self.assertIn('saved_entry=ngfw-B', self.app.grub('list').decode())
        self.assertTrue(self.app.status()['confirmed'])

    def test_failed_health_and_reboot_preserve_previous_default(self):
        self.app.activate(); self.app.active = 'B'
        self.app.rollback()
        env = self.app.grub('list').decode()
        self.assertIn('saved_entry=ngfw-A', env)
        self.assertNotIn('next_entry=', env)
        self.assertIsNone(self.app.state()['pending_slot'])
        self.assertEqual(self.app.state()['default_slot'], 'A')

    def test_refuse_stage_during_trial_before_write(self):
        self.app.activate()
        with patch.object(u, 'run') as execute, self.assertRaises(u.Refused):
            self.app.stage('/data/updates/untrusted.tar')
        execute.assert_not_called()


class HealthBoot(unittest.TestCase):
    def execute(self, healthy, state=None, probe_error=None):
        calls = []
        state = state or {'active_slot': 'B', 'pending_slot': 'B'}
        def fake(args, **kwargs):
            calls.append(tuple(args))
            result = type('Result', (), {'stdout': b''})()
            if args[1:] == ('status', '--json'):
                result.stdout = json.dumps(state).encode()
            elif args[0].endswith('ngfw-upgrade-probe') and probe_error is not None:
                raise probe_error
            elif args[0].endswith('ngfw-upgrade-probe') and not healthy:
                raise subprocess.CalledProcessError(1, args)
            return result
        clock = iter([0, 0, 181])
        with patch('subprocess.run', side_effect=fake), patch('time.monotonic', side_effect=lambda: next(clock)), patch('time.sleep'):
            with self.assertRaises(SystemExit) as exit_status:
                runpy.run_path(str(BASE / 'ngfw-upgrade-health'), run_name='__main__')
        return calls, exit_status.exception.code

    def test_health_confirms_only_trial(self):
        calls, code = self.execute(True)
        self.assertEqual(code, 0)
        self.assertIn(('/usr/sbin/ngfw-upgrade', 'confirm'), calls)
        self.assertNotIn(('/usr/sbin/ngfw-upgrade', 'rollback'), calls)

    def test_failed_health_rolls_back_and_reboots(self):
        calls, code = self.execute(False)
        self.assertEqual(code, 1)
        self.assertEqual(calls[-2:], [('/usr/sbin/ngfw-upgrade', 'rollback'), ('/usr/bin/systemctl', '--no-block', 'reboot')])

    def test_missing_or_nonexecutable_probe_rolls_back_after_deadline(self):
        for failure in (FileNotFoundError('missing probe'), PermissionError('nonexecutable probe')):
            with self.subTest(failure=type(failure).__name__):
                calls, code = self.execute(False, probe_error=failure)
                self.assertEqual(code, 1)
                self.assertEqual(calls[-2:], [('/usr/sbin/ngfw-upgrade', 'rollback'),
                                             ('/usr/bin/systemctl', '--no-block', 'reboot')])
                self.assertNotIn(('/usr/sbin/ngfw-upgrade', 'confirm'), calls)

    def test_unreadable_status_does_not_mutate_unknown_boot_state(self):
        with patch('subprocess.run', side_effect=FileNotFoundError('status unavailable')) as execute:
            with self.assertRaises(SystemExit) as result:
                runpy.run_path(str(BASE / 'ngfw-upgrade-health'), run_name='__main__')
        self.assertEqual(result.exception.code, 1)
        self.assertEqual(execute.call_count, 1)

    def test_old_slot_reconciles_consumed_trial_without_reboot(self):
        calls, code = self.execute(False, {'active_slot': 'A', 'pending_slot': 'B'})
        self.assertEqual(code, 0)
        self.assertEqual(calls[-1], ('/usr/sbin/ngfw-upgrade', 'rollback'))


if __name__ == '__main__':
    unittest.main()
