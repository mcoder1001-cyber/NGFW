import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import xml.etree.ElementTree as ET

REPO = Path(__file__).resolve().parents[3]
VM = REPO / 'deploy/image/vm'
spec = importlib.util.spec_from_file_location('ngfw_image', VM / 'image.py')
image = importlib.util.module_from_spec(spec)
spec.loader.exec_module(image)


class ImageTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name) / 'root'
        self.root.mkdir()

    def tearDown(self):
        self.temp.cleanup()

    def write(self, path, value='fixture'):
        image.put(self.root, path, value)

    def fixture(self, profile='vm'):
        self.write('etc/ssh/ssh_host_ed25519_key', 'old-key')
        self.write('etc/shadow', 'root:!:0:0:99999:7:::\nngfw-api:*:0:0:99999:7:::\n')
        self.write('boot/vmlinuz-6.17.0-1-generic')
        self.write('boot/initrd.img-6.17.0-1-generic')
        for path in ['boot/efi/EFI/BOOT/BOOTX64.EFI', 'etc/ngfw/appliance',
                     'usr/lib/ngfw-image/ngfw-bootstrap-password', 'usr/sbin/nft',
                     'etc/systemd/system/ngfw-bootstrap-password.service',
                     'usr/lib/systemd/system/ngfw-firstboot.service', 'usr/lib/systemd/system/ngfw-firewall-bootstrap.service',
                     'usr/lib/systemd/system/ngfw-agent.service', 'usr/lib/systemd/system/ngfw-api.service',
                     'usr/lib/systemd/system/nftables.service']:
            self.write(path)
        image.configure(self.root, profile)
        image.grub_config(self.root)
        packages = ['ngfw-meta', 'cloud-init', 'openssh-server', 'nftables', 'grub-pc-bin', 'grub-efi-amd64-bin']
        output = ''.join(p + '\t1.0\tinstalled\n' for p in packages) + 'vpp\t26.06-1\tinstalled\n'
        return subprocess.CompletedProcess([], 0, stdout=output)

    def test_layout_consumes_shared_labels(self):
        parts = image.layout(96)
        self.assertEqual(7, len(parts))
        self.assertEqual('ngfw-rootB', parts[3]['name'])
        self.assertEqual('-', parts[3]['mount'])
        self.assertEqual(20 * image.GIB // 512, parts[3]['size'])
        for prev, part in zip(parts, parts[1:]):
            self.assertEqual(prev['start'] + prev['size'], part['start'])
            self.assertEqual(0, part['start'] % 2048)
        self.assertLess(parts[-1]['start'] + parts[-1]['size'], 96 * image.GIB // 512)
        for size in [0, 95, 4097]:
            with self.assertRaises(ValueError): image.layout(size)

    def test_completion_bridge_tracks_current_packaging(self):
        self.fixture()
        marker = self.root / 'var/lib/ngfw/firstboot.done'
        self.assertTrue(marker.is_symlink())
        self.assertFalse(marker.exists())
        self.write('var/lib/ngfw/firstboot-complete', 'completed\n')
        self.assertTrue(marker.is_file())
        self.assertEqual('completed\n', marker.read_text())
        self.assertIn('/var/lib/ngfw/firstboot-complete', (REPO / 'deploy/debian/ngfw/assets/firstboot.sh').read_text())
        self.assertIn('firstboot.done', (image.COMMON / 'systemd/ngfw-bootstrap-password.service').read_text())

    def test_profiles_and_identity(self):
        for profile, expected in [('vm', ['NoCloud', 'ConfigDrive', 'None']), ('aws', ['Ec2', 'None']),
                                  ('azure', ['Azure', 'None']), ('gcp', ['GCE', 'None'])]:
            with self.subTest(profile=profile):
                root = self.root / profile; root.mkdir()
                image.configure(root, profile)
                config = (root / 'etc/cloud/cloud.cfg.d/90-ngfw-image.cfg').read_text()
                self.assertIn('datasource_list: ' + json.dumps(expected), config)
                self.assertIn('users: []', config)
                self.assertEqual(b'', (root / 'etc/machine-id').read_bytes())
                self.assertNotIn('ngfw-rootB', (root / 'etc/fstab').read_text().splitlines()[-1])

    def test_target_cannot_escape_via_symlink(self):
        outside = Path(self.temp.name) / 'outside'; outside.mkdir()
        (self.root / 'etc').symlink_to(outside)
        with self.assertRaises(ValueError): image.configure(self.root, 'vm')
        self.assertEqual([], list(outside.iterdir()))
        with self.assertRaises(ValueError): image.configure(Path('/'), 'vm')

    def test_grub_rejects_host_root_and_aliased_roots_before_kernel_read(self):
        with patch.object(Path, 'glob', side_effect=AssertionError('must reject before reading kernels')):
            with self.assertRaisesRegex(ValueError, 'canonical offline root'):
                image.grub_config(Path('/'))
        self.write('boot/vmlinuz-fixture')
        self.write('boot/initrd.img-fixture')
        alias = Path(self.temp.name) / 'alias'
        alias.symlink_to(self.root)
        with self.assertRaisesRegex(ValueError, 'canonical offline root'):
            image.grub_config(alias)
        self.assertFalse((self.root / 'boot/grub/grub.cfg').exists())
        parent_alias = Path(self.temp.name) / 'parent-alias'
        parent_alias.symlink_to(Path(self.temp.name))
        with self.assertRaisesRegex(ValueError, 'canonical offline root'):
            image.grub_config(parent_alias / 'root')

    def test_grub_preflight_preserves_external_boot_and_config(self):
        for relative in ('boot', 'boot/grub', 'boot/grub/grub.cfg'):
            with self.subTest(relative=relative), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary) / 'root'; root.mkdir()
                outside = Path(temporary) / 'outside'; outside.mkdir()
                target = outside / 'grub.cfg' if relative.endswith('grub.cfg') else outside
                sentinel = outside / 'grub.cfg'; sentinel.write_text('preserve external boot')
                if relative != 'boot':
                    (root / 'boot').mkdir()
                    (root / 'boot/vmlinuz-fixture').write_text('kernel')
                    (root / 'boot/initrd.img-fixture').write_text('initrd')
                destination = root / relative; destination.parent.mkdir(parents=True, exist_ok=True)
                destination.symlink_to(target)
                with self.assertRaisesRegex(ValueError, 'symlink image mutation'):
                    image.grub_config(root)
                self.assertEqual(sentinel.read_text(), 'preserve external boot')
                self.assertEqual([sentinel], list(outside.iterdir()))

    def test_every_mutation_parent_preflight_preserves_outside(self):
        for relative, leaf in [('var/lib/dbus', 'machine-id'), ('etc/ssh', 'ssh_host_ed25519_key'),
                               ('var/lib/ngfw', 'firstboot.done'),
                               ('etc/systemd/system/multi-user.target.wants', 'sentinel')]:
            with self.subTest(relative=relative), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary) / 'root'; root.mkdir()
                outside = Path(temporary) / 'outside'; outside.mkdir()
                sentinel = outside / leaf; sentinel.write_text('must remain unchanged')
                destination = root / relative; destination.parent.mkdir(parents=True)
                destination.symlink_to(outside)
                before = sorted(str(p.relative_to(root)) for p in root.rglob('*'))
                with self.assertRaises(ValueError): image.configure(root, 'vm')
                self.assertEqual('must remain unchanged', sentinel.read_text())
                self.assertEqual([leaf], [p.name for p in outside.iterdir()])
                self.assertEqual(before, sorted(str(p.relative_to(root)) for p in root.rglob('*')))

    def test_complete_inspection_and_secret_rejection(self):
        response = self.fixture()
        with patch.object(image.subprocess, 'run', return_value=response):
            report = image.validate(self.root, 'vm')
            self.assertEqual('26.06-1', report['packages']['vpp'])
            for path in ['etc/ssh/ssh_host_ed25519_key', 'etc/ngfw/bootstrap.env', 'etc/ngfw/api.env', 'var/lib/ngfw/secret.key']:
                self.write(path)
                with self.assertRaises(ValueError): image.validate(self.root, 'vm')
                (self.root / path).unlink()
            self.write('etc/machine-id', 'host-identity')
            with self.assertRaises(ValueError): image.validate(self.root, 'vm')

    def test_required_files_packages_password_and_datasource(self):
        response = self.fixture()
        with patch.object(image.subprocess, 'run', return_value=response):
            with self.assertRaises(ValueError): image.validate(self.root, 'azure')
            self.write('etc/shadow', 'root:unlocked:0:0:99999:7:::\n')
            with self.assertRaises(ValueError): image.validate(self.root, 'vm')
            self.write('etc/shadow', 'root:!:0:0:99999:7:::\n')
            (self.root / 'boot/efi/EFI/BOOT/BOOTX64.EFI').unlink()
            with self.assertRaises(ValueError): image.validate(self.root, 'vm')
        self.write('boot/efi/EFI/BOOT/BOOTX64.EFI')
        with patch.object(image.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0, stdout='vpp\t25.10\tinstalled\n')):
            with self.assertRaises(ValueError): image.validate(self.root, 'vm')

    def test_grub_requires_kernel_initrd_and_shared_cmdline(self):
        self.write('boot/vmlinuz-6.17.0-1-generic')
        with self.assertRaises(ValueError): image.grub_config(self.root)
        self.write('boot/initrd.img-6.17.0-1-generic')
        image.grub_config(self.root)
        config = (self.root / 'boot/grub/grub.cfg').read_text()
        self.assertIn('root=LABEL=ngfw-rootA', config)
        self.assertIn('console=ttyS0,115200n8', config)
        self.assertIn('intel_iommu=on', config)
        self.write('boot/vmlinuz-6.17.0-2-generic')
        with self.assertRaises(ValueError): image.grub_config(self.root)

    def test_ovf_references_hardware_and_tar_safe_filename(self):
        disk = self.root / 'ngfw.vmdk'; disk.write_bytes(b'fixture')
        output = self.root / 'ngfw.ovf'
        image.ovf(output, disk, 96 * image.GIB)
        doc = ET.parse(output)
        ns = {'ovf': 'http://schemas.dmtf.org/ovf/envelope/1',
              'rasd': 'http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_ResourceAllocationSettingData'}
        file = doc.find('ovf:References/ovf:File', ns)
        self.assertEqual('ngfw.vmdk', file.attrib['{' + ns['ovf'] + '}href'])
        self.assertEqual('7', file.attrib['{' + ns['ovf'] + '}size'])
        items = doc.findall('.//ovf:Item', ns)
        hardware = {i.find('rasd:ResourceType', ns).text: i for i in items}
        self.assertEqual('16384', hardware['4'].find('rasd:VirtualQuantity', ns).text)
        self.assertEqual('VmxNet3', hardware['10'].find('rasd:ResourceSubType', ns).text)
        self.assertEqual('ovf:/disk/disk1', hardware['17'].find('rasd:HostResource', ns).text)

    def test_root_build_is_opt_in_and_help_is_safe(self):
        import os
        env = dict(os.environ, NGFW_INTEGRATION='0')
        result = subprocess.run([str(VM / 'build.sh')], env=env, capture_output=True, text=True)
        self.assertNotEqual(0, result.returncode)
        self.assertIn('NGFW_INTEGRATION=1', result.stderr)
        result = subprocess.run([str(VM / 'build.sh'), '--help'], env=env, capture_output=True, text=True)
        self.assertEqual(0, result.returncode)
        self.assertIn('--pool-key-fpr', result.stdout)

    @unittest.skipUnless(shutil.which('qemu-img'), 'qemu-img absent; format roundtrip requires it')
    def test_real_small_format_roundtrips(self):
        raw = self.root / 'disk.raw'
        with raw.open('wb') as f:
            f.write(b'NGFW-format-fixture'); f.truncate(16 * 1024 * 1024)
        for fmt, option in [('qcow2', 'compat=1.1'), ('vmdk', 'subformat=streamOptimized'),
                            ('vhdx', 'subformat=dynamic'), ('vpc', 'subformat=fixed,force_size=on')]:
            with self.subTest(fmt=fmt):
                target = self.root / ('disk.' + fmt)
                subprocess.run(['qemu-img', 'convert', '-f', 'raw', '-O', fmt, '-o', option, str(raw), str(target)], check=True, capture_output=True)
                subprocess.run(['qemu-img', 'compare', '-f', 'raw', '-F', fmt, str(raw), str(target)], check=True, capture_output=True)
                info = json.loads(subprocess.check_output(['qemu-img', 'info', '-f', fmt, '--output=json', str(target)]))
                self.assertEqual(16 * 1024 * 1024, info['virtual-size'])


if __name__ == '__main__':
    unittest.main()
