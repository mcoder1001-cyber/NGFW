import hashlib
import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

BASE = Path(__file__).resolve().parents[1]


def load(name, relative):
    spec = importlib.util.spec_from_file_location(name, BASE / relative)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


stage = load('stage', 'stage.py')
check = load('check', 'check.py')
signing = load('signing', 'signing/verify.py')


class Hardening(unittest.TestCase):
    def test_offline_stage_and_drift(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            stage.stage(root)
            self.assertFalse(any(s == 'FAIL' for _, s, _ in check.inspect(root)))
            target = root / 'etc/sysctl.d/60-ngfw.conf'
            target.write_text(target.read_text().replace('kptr_restrict = 2', 'kptr_restrict = 0'))
            self.assertTrue(any(s == 'FAIL' for _, s, _ in check.inspect(root)))

    def test_symlink_and_running_root_refused_before_write(self):
        with self.assertRaises(ValueError):
            stage.stage('/')
        with tempfile.TemporaryDirectory() as directory, tempfile.TemporaryDirectory() as outside:
            root = Path(directory)
            (root / 'etc').symlink_to(outside)
            with self.assertRaises(ValueError):
                stage.stage(root)
            self.assertEqual(list(Path(outside).iterdir()), [])

    def test_ssh_requires_key_and_invalid_interface_has_no_writes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with self.assertRaises(ValueError):
                stage.stage(root, ssh_key_only=True)
            self.assertEqual(list(root.iterdir()), [])
            for interface in ['all.x', 'all', 'default', 'lo']:
                with self.assertRaises(ValueError):
                    stage.stage(root, management_interface=interface)
            self.assertEqual(list(root.iterdir()), [])
            keys = root / 'root/.ssh/authorized_keys'
            keys.parent.mkdir(parents=True)
            private = root / 'ephemeral-key'
            subprocess.run(['ssh-keygen', '-q', '-t', 'ed25519', '-N', '', '-f', str(private)], check=True)
            keys.write_text(private.with_suffix('.pub').read_text())
            private.unlink()
            private.with_suffix('.pub').unlink()
            stage.stage(root, ssh_key_only=True, management_interface='mgmt0')
            self.assertFalse(any(s == 'FAIL' for _, s, _ in check.inspect(root)))
            stage.stage(root)
            self.assertFalse(any(s == 'FAIL' for _, s, _ in check.inspect(root)))
            self.assertEqual((root / 'etc/sysctl.d/61-ngfw-management.conf').read_text(),
                             'net.ipv4.conf.mgmt0.rp_filter = 2\n')

    def test_management_drift_and_repeat_staging_preserves_selection(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            stage.stage(root, management_interface='mgmt0')
            stage.stage(root)
            self.assertFalse(any(s == 'FAIL' for _, s, _ in check.inspect(root)))
            target = root / 'etc/sysctl.d/61-ngfw-management.conf'
            target.write_text('net.ipv4.conf.all.rp_filter = 1\n')
            self.assertTrue(any(s == 'FAIL' for _, s, _ in check.inspect(root)))

    def test_undeclared_optional_file_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            stage.stage(root)
            target = root / 'etc/sysctl.d/61-ngfw-management.conf'
            target.write_text('net.ipv4.conf.all.rp_filter = 1\n')
            self.assertTrue(any(s == 'FAIL' for _, s, _ in check.inspect(root)))
            with self.assertRaises(ValueError):
                stage.stage(root)

    def test_compatibility_permissions_preserved(self):
        agent = (BASE / 'systemd/ngfw-agent.service.d/10-ngfw-hardening.conf').read_text()
        self.assertIn('AF_NETLINK', agent)
        bounds = [line.split('=', 1)[1] for line in agent.splitlines()
                  if line.startswith('CapabilityBoundingSet=')]
        self.assertEqual(bounds, ['', 'CAP_NET_ADMIN CAP_SYS_ADMIN CAP_IPC_LOCK CAP_CHOWN CAP_DAC_OVERRIDE'])
        self.assertIn('ReadWritePaths=/var/lib/ngfw-system-identity /etc/systemd/resolved.conf.d', agent)
        self.assertIn('/etc/kea', agent)
        self.assertIn('@mount', agent)
        self.assertFalse(list((BASE / 'systemd').glob('vpp*')))
        baseline = (BASE / 'baseline/60-ngfw.conf').read_text()
        self.assertNotIn('net.ipv4.conf.all.rp_filter', baseline)


class Signing(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory()
        cls.home = Path(cls.temp.name)
        cls.home.chmod(0o700)
        subprocess.run(['gpg', '--homedir', str(cls.home), '--batch', '--pinentry-mode', 'loopback',
                        '--passphrase', '', '--quick-generate-key', 'NGFW signing test', 'ed25519', 'sign', '0'],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        cls.key = cls.home / 'public.gpg'
        with cls.key.open('wb') as output:
            subprocess.run(['gpg', '--homedir', str(cls.home), '--export'], check=True, stdout=output)

    @classmethod
    def tearDownClass(cls):
        subprocess.run(['gpgconf', '--homedir', str(cls.home), '--kill', 'gpg-agent'], check=True)
        cls.temp.cleanup()

    def repo(self, root, relative='main/binary-amd64/Packages'):
        directory = root / 'dists/resolute'
        member = directory / relative
        member.parent.mkdir(parents=True)
        member.write_bytes(b'Package: fixture\nVersion: 1\n')
        release = directory / 'Release'
        release.write_text('Suite: resolute\nSHA256:\n ' + hashlib.sha256(member.read_bytes()).hexdigest() +
                           ' ' + str(member.stat().st_size) + ' ' + relative + '\n')
        self.sign(release)
        return directory

    def sign(self, release):
        subprocess.run(['gpg', '--homedir', str(self.home), '--batch', '--yes', '--detach-sign',
                        '--output', str(release) + '.gpg', str(release)], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def test_signing_key_rotation_overlap_and_retirement(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            directory = self.repo(root)
            release = directory / 'Release'
            old_signature = (directory / 'Release.gpg').read_bytes()
            new_home = root / 'new-key'
            new_home.mkdir(mode=0o700)
            try:
                subprocess.run(['gpg', '--homedir', str(new_home), '--batch', '--pinentry-mode',
                                'loopback', '--passphrase', '', '--quick-generate-key',
                                'NGFW rotation fixture', 'ed25519', 'sign', '0'], check=True,
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                new_key = root / 'new-public.gpg'
                with new_key.open('wb') as output:
                    subprocess.run(['gpg', '--homedir', str(new_home), '--export'],
                                   check=True, stdout=output)
                overlap = root / 'overlap.gpg'
                overlap.write_bytes(self.key.read_bytes() + new_key.read_bytes())
                self.assertEqual(signing.verify(root, overlap, 'resolute'), 1)
                with self.assertRaises(subprocess.CalledProcessError):
                    signing.verify(root, new_key, 'resolute')
                subprocess.run(['gpg', '--homedir', str(new_home), '--batch', '--yes',
                                '--detach-sign', '--output', str(release) + '.gpg', str(release)],
                               check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                self.assertEqual(signing.verify(root, overlap, 'resolute'), 1)
                self.assertEqual(signing.verify(root, new_key, 'resolute'), 1)
                with self.assertRaises(subprocess.CalledProcessError):
                    signing.verify(root, self.key, 'resolute')
                (directory / 'Release.gpg').write_bytes(old_signature)
                with self.assertRaises(subprocess.CalledProcessError):
                    signing.verify(root, new_key, 'resolute')
            finally:
                subprocess.run(['gpgconf', '--homedir', str(new_home), '--kill', 'gpg-agent'],
                               check=True)

    def test_signed_repo_tampering_and_missing_signature(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            directory = self.repo(root)
            self.assertEqual(signing.verify(root, self.key, 'resolute'), 1)
            release = directory / 'Release'
            original = release.read_text()
            release.write_text(original + 'Origin: modified\n')
            with self.assertRaises(subprocess.CalledProcessError):
                signing.verify(root, self.key, 'resolute')
            release.write_text(original)
            (directory / 'main/binary-amd64/Packages').write_text('tampered')
            with self.assertRaises(ValueError):
                signing.verify(root, self.key, 'resolute')
            (directory / 'Release.gpg').unlink()
            with self.assertRaises(ValueError):
                signing.verify(root, self.key, 'resolute')

    def test_metadata_replaced_after_gpgv_cannot_change_authenticated_members(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            directory = self.repo(root)
            original_run = subprocess.run
            def replace_after_authentication(argv, **kwargs):
                result = original_run(argv, **kwargs)
                if argv[0] == 'gpgv':
                    (directory / 'Release').write_text('SHA256:\n ' + '0' * 64 + ' 0 main/binary-amd64/Packages\n')
                    (directory / 'main/binary-amd64/Packages').write_bytes(b'')
                return result
            with patch.object(signing.subprocess, 'run', side_effect=replace_after_authentication):
                with self.assertRaises(ValueError):
                    signing.verify(root, self.key, 'resolute')

    def test_wrong_key_and_signed_traversal(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            directory = self.repo(root)
            unknown = root / 'unknown.gpg'
            unknown.write_bytes(b'')
            with self.assertRaises(subprocess.CalledProcessError):
                signing.verify(root, unknown, 'resolute')
            release = directory / 'Release'
            release.write_text('SHA256:\n ' + '0' * 64 + ' 1 ../escape\n')
            self.sign(release)
            with self.assertRaises(ValueError):
                signing.verify(root, self.key, 'resolute')
