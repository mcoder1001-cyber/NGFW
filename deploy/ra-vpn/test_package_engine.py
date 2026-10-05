import importlib.util
from pathlib import Path
import subprocess
import unittest
from test_stage_engine import StageTests

spec = importlib.util.spec_from_file_location('package_engine', Path(__file__).with_name('package-engine.py'))
engine = importlib.util.module_from_spec(spec)
spec.loader.exec_module(engine)


class PackageTests(StageTests):
    def setUp(self):
        super().setUp()
        self.source = self.prefix / 'share/ngfw/engine-build.txt'
        self.source.write_text('version=6.1.0\nsource_sha256=fe6c97481298767213cfc2e9a1da29fdd8018d481ff4cb9cf0283099654f20d4\nrelease_fingerprint=948F158A4E76A27BF3D07532DF42C170B34DBA77\n')
        self.output = self.root / 'engine.deb'

    def test_package_control_and_payload_have_no_host_activation(self):
        engine.package(self.artifact, self.target, self.output)
        control = subprocess.check_output(['dpkg-deb', '--field', str(self.output)], text=True)
        self.assertIn('Package: ngfw-ra-engine', control)
        self.assertIn('libc6 (= 2.43-x)', control)
        extracted = self.root / 'extracted'
        subprocess.run(['dpkg-deb', '--raw-extract', str(self.output), str(extracted)], check=True)
        self.assertEqual(sorted(p.name for p in (extracted / 'DEBIAN').iterdir()), ['control'])
        self.assertEqual((extracted / 'opt/ngfw-ra/share/ngfw/engine-build.txt').read_bytes(), self.source.read_bytes())
        self.assertFalse((self.target / 'opt/ngfw-ra').exists())
        self.assertFalse((extracted / 'usr').exists())

    def test_wrong_source_receipt_refused_without_output(self):
        self.source.write_text('foreign build')
        with self.assertRaises(engine.stage.Refused):
            engine.package(self.artifact, self.target, self.output)
        self.assertFalse(self.output.exists())

    def test_existing_output_is_preserved(self):
        self.output.write_bytes(b'owned sentinel')
        with self.assertRaises(engine.stage.Refused):
            engine.package(self.artifact, self.target, self.output)
        self.assertEqual(self.output.read_bytes(), b'owned sentinel')

    def test_mismatch_refused_without_output(self):
        (self.target / 'etc/os-release').write_text('ID=debian\nVERSION_ID=12\n')
        with self.assertRaises(engine.stage.Refused):
            engine.package(self.artifact, self.target, self.output)
        self.assertFalse(self.output.exists())


if __name__ == '__main__':
    unittest.main()
