"""Production verifier regressions using real small Debian archives; no installs."""
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest

ISO = Path(__file__).resolve().parents[1]
CONTRACT = ISO.parents[1] / 'vpp/VERSION'
spec = importlib.util.spec_from_file_location('verify_manifest', ISO / 'lib/verify-vpp-manifest.py')
verifier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verifier)


class ManifestTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='ngfw-vpp-manifest-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.pool = self.root / 'pool'
        self.pool.mkdir()
        values = dict(line.split('=', 1) for line in CONTRACT.read_text().splitlines()
                      if line.startswith(('VPP_PACKAGES=', 'VPP_PACKAGES_SHIP=')))
        names = shlex.split(values['VPP_PACKAGES'])[0].split()
        ship = shlex.split(values['VPP_PACKAGES_SHIP'])[0].split()
        entries = []
        for name in names:
            filename = name + '_26.06-release+ngfw1_amd64.deb'
            digest = '0' * 64
            if name in ship:
                build = self.root / ('build-' + name)
                (build / 'DEBIAN').mkdir(parents=True)
                (build / 'DEBIAN/control').write_text(
                    f'Package: {name}\nVersion: 26.06-release+ngfw1\nArchitecture: amd64\n'
                    'Maintainer: Test <test@example.invalid>\nDescription: verifier fixture\n')
                artifact = self.pool / filename
                subprocess.run(['dpkg-deb', '--build', str(build), str(artifact)],
                               check=True, capture_output=True)
                digest = hashlib.sha256(artifact.read_bytes()).hexdigest()
            entries.append(dict(package=name, version='26.06-release+ngfw1', architecture='amd64',
                                file=filename, sha256=digest, ship=name in ship))
        self.manifest = dict(schema='ngfw.vpp-debs.manifest/v2', version='26.06-release+ngfw1',
                             upstream=dict(commit='fixture'), packages=entries)

    def verify(self, manifest=None):
        path = self.root / 'manifest.json'
        path.write_text(json.dumps(self.manifest if manifest is None else manifest))
        verifier.verify(path, self.root, self.manifest['version'], CONTRACT)

    def test_valid_producer_contract(self):
        self.verify()

    def test_incomplete_and_duplicate_manifests(self):
        for packages in ([], self.manifest['packages'][1:],
                         [p for p in self.manifest['packages'] if p['package'] != 'vpp'],
                         self.manifest['packages'] + [self.manifest['packages'][0]]):
            with self.subTest(packages=len(packages)), self.assertRaises(ValueError):
                self.verify({**self.manifest, 'packages': packages})

    def test_entry_integrity(self):
        for field, value in [('ship', False), ('ship', 1), ('version', 'other'),
                             ('file', '../escape.deb'), ('file', '*.deb'),
                             ('sha256', 'f' * 64), ('architecture', 'arm64')]:
            manifest = copy.deepcopy(self.manifest)
            entry = next(p for p in manifest['packages'] if p['package'] == 'vpp')
            entry[field] = value
            with self.subTest(field=field, value=value), self.assertRaises(ValueError):
                self.verify(manifest)

    def test_duplicate_filename(self):
        manifest = copy.deepcopy(self.manifest)
        manifest['packages'][1]['file'] = manifest['packages'][0]['file']
        with self.assertRaises(ValueError):
            self.verify(manifest)

    def test_missing_duplicate_and_escaping_repository_artifacts(self):
        artifact = next(self.pool.glob('vpp_*.deb'))
        duplicate = self.pool / 'duplicate.deb'
        duplicate.write_bytes(artifact.read_bytes())
        with self.assertRaises(ValueError):
            self.verify()
        duplicate.unlink()
        outside = self.root / 'outside.deb'
        artifact.rename(outside)
        with self.assertRaises(ValueError):
            self.verify()
        artifact.symlink_to(outside)
        with self.assertRaises(ValueError):
            self.verify()


if __name__ == '__main__':
    unittest.main()
