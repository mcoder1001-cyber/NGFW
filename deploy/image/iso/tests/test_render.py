"""Pure renderer regression tests: no host changes or ISO inputs."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('render', Path(__file__).resolve().parents[1] / 'lib/render.py')
render = importlib.util.module_from_spec(spec)
spec.loader.exec_module(render)


class RenderTests(unittest.TestCase):
    def test_storage_without_final_newline(self):
        result = render.user_data('autoinstall:\n@STORAGE@\n  ssh: {}\n', 'storage: {}', '1.0', 'ubuntu-server')
        self.assertEqual(result, 'autoinstall:\n  storage: {}\n  ssh: {}\n')

    def test_invalid_source_and_version(self):
        for version, source in [('1.0;echo', 'ubuntu-server'), ('1.0', 'bogus')]:
            with self.assertRaises(SystemExit):
                render.user_data('@STORAGE@\n', '', version, source)

    def test_unresolved_placeholder(self):
        with self.assertRaises(SystemExit):
            render.user_data('@STORAGE@', 'storage: {}\n', '1.0', 'ubuntu-server')

    def test_missing_boot_inputs(self):
        for cfg in ['', 'menuentry "Ubuntu" {}\n', 'linux /casper/vmlinuz ---\ninitrd /casper/initrd\n']:
            with self.assertRaisesRegex(SystemExit, 'base GRUB config'):
                render.grub(cfg, '1.0', '')

    def test_comment_is_not_a_menu(self):
        cfg = '# menuentry rescue\nmenuentry "Ubuntu" {\nlinux /casper/vmlinuz ---\ninitrd /casper/initrd\n}\n'
        result = render.grub(cfg, '1.0', '')
        self.assertTrue(result.startswith('set timeout=5\n# menuentry rescue\n'))
        self.assertIn('set default=0\nmenuentry "Install VRX', result)
        self.assertIn('submenu "Ubuntu Server installer (interactive, no VRX)" {\nmenuentry "Ubuntu"', result)

    def test_kernel_argument_injection(self):
        with self.assertRaises(SystemExit):
            render.grub('', '1.0', ' console=tty0;reboot')

    def test_destructive_menu_labels_match_largest_disk_selection(self):
        cfg = 'menuentry "Ubuntu" {\nlinux /casper/vmlinuz ---\ninitrd /casper/initrd\n}\n'
        result = render.grub(cfg, '1.0', '')
        self.assertIn('menuentry "Install VRX 1.0 (unattended: ERASES the largest disk)"', result)
        self.assertIn('menuentry "Reinstall VRX 1.0 (ERASES the largest disk)"', result)
        self.assertNotIn('ERASES an existing VRX installation', result)
        self.assertEqual(result.count(' vrx.reinstall=1'), 1)
        self.assertIn('set default=0\nmenuentry "Install VRX', result)


if __name__ == '__main__':
    unittest.main()
