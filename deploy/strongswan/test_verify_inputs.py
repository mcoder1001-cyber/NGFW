"""Real synthetic .deb/tar inputs; VPP gate stubbed, no product build claim."""
import contextlib
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('p11_inputs', Path(__file__).with_name('verify_inputs.py'))
INPUT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(INPUT)


class IntakeTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.vpp = self.root / 'vpp'
        self.vpp.mkdir()
        self.source = self.root / 'source.tar.bz2'
        self.version = '26.06-release+vrx1'
        self.entries = []
        for name in INPUT.REQUIRED:
            tree = self.root / ('source-' + name)
            (tree / 'DEBIAN').mkdir(parents=True)
            (tree / 'DEBIAN/control').write_text(
                f'Package: {name}\nVersion: {self.version}\nArchitecture: amd64\n'
                'Maintainer: Fixture <fixture@example.invalid>\nDescription: synthetic only\n')
            artifact = self.vpp / (name + '.deb')
            subprocess.run(['dpkg-deb', '--build', '--root-owner-group', str(tree), str(artifact)],
                           check=True, capture_output=True)
            meta = INPUT.VERIFY.metadata(artifact)
            self.entries.append({'package': name, 'file': artifact.name, 'version': self.version,
                'architecture': 'amd64', 'sha256': meta['sha256'], 'size': meta['size']})
        self.manifest()
        self.tar()
        original = INPUT.VERIFY.run
        def gate(argv, limit=1024 * 1024, pass_fds=()):
            if argv[0] == 'bash':
                self.assertIn('--install-gate', argv)
                self.assertIn('--require-files', argv)
                self.assertNotIn('--no-tests', argv)
                self.assertNotEqual(Path(argv[-2]), self.vpp)
                return 'synthetic provenance stub'
            return original(argv, limit, pass_fds)
        self.gate = patch.object(INPUT.VERIFY, 'run', gate)
        self.gate.start()
        self.addCleanup(self.gate.stop)

    def manifest(self):
        (self.vpp / 'manifest.json').write_text(json.dumps({'version': self.version, 'packages': self.entries}))
        (self.vpp / 'SHA256SUMS').write_text('synthetic boundary stub\n')

    def tar(self, extra=None, omit_parser=False, executable=True):
        prefix = 'strongswan-5.9.6/'
        with tarfile.open(self.source, 'w:bz2') as archive:
            names = [prefix + 'configure']
            if not omit_parser:
                names.append(prefix + 'src/libstrongswan/settings/settings_parser.c')
            for name in names:
                member = tarfile.TarInfo(name)
                member.mode = 0o755 if executable else 0o644
                member.size = 7
                archive.addfile(member, io.BytesIO(b'fixture'))
            if extra:
                archive.addfile(extra, io.BytesIO(b'x' * extra.size) if extra.isfile() else None)
        self.digest = hashlib.sha256(self.source.read_bytes()).hexdigest()

    def verify(self, digest=None, version='5.9.6'):
        return INPUT.verify(self.vpp, self.source, version, digest or self.digest)

    def test_positive_reports_hashes_without_modifying_inputs(self):
        before = {p.name: p.read_bytes() for p in self.vpp.iterdir()}
        report = self.verify()
        self.assertFalse(report['release_approved'])
        self.assertEqual(report['source']['sha256'], self.digest)
        self.assertEqual(set(report['staging_packages']), set(INPUT.REQUIRED))
        self.assertEqual(report['source']['expected_origin'], 'https://download.strongswan.org/strongswan-5.9.6.tar.bz2')
        self.assertEqual(before, {p.name: p.read_bytes() for p in self.vpp.iterdir()})

    def test_hash_version_and_metadata_mismatch(self):
        with self.assertRaises(INPUT.InvalidInputs): self.verify('0' * 64)
        with self.assertRaises(INPUT.InvalidInputs): self.verify('bad')
        for missing in (None, '', 123):
            with self.assertRaises(INPUT.InvalidInputs):
                INPUT.verify(self.vpp, self.source, '5.9.6', missing)
        with contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(INPUT.main(['--vpp-output', str(self.vpp), '--source', str(self.source),
                '--source-version', '5.9.6', '--source-sha256', '']), 1)
        with self.assertRaises(INPUT.InvalidInputs): self.verify(version='6.0.0')
        for key, value in (('sha256', '0' * 64), ('version', '26.06-release'), ('architecture', 'arm64')):
            original = self.entries[0][key]
            self.entries[0][key] = value
            self.manifest()
            with self.assertRaises(INPUT.InvalidInputs): self.verify()
            self.entries[0][key] = original

    def test_missing_duplicate_and_unsafe_staging_entries(self):
        first = self.entries.pop(0)
        self.manifest()
        with self.assertRaises(INPUT.InvalidInputs): self.verify()
        self.entries.extend([first, first])
        self.manifest()
        with self.assertRaises(INPUT.InvalidInputs): self.verify()
        self.entries.pop()
        first['file'] = '../outside.deb'
        self.manifest()
        with self.assertRaises(INPUT.InvalidInputs): self.verify()

    def test_tar_paths_duplicates_links_and_special_files_refused(self):
        for name in ('../outside', '/absolute', 'strongswan-5.9.6/../outside',
                     'strongswan-5.9.6//empty', 'strongswan-5.9.6/configure',
                     'wrong-version/configure', 'strongswan-5.9.6/' + 'x' * 1024):
            self.tar(tarfile.TarInfo(name))
            with self.assertRaises(INPUT.InvalidInputs): self.verify()
        for kind in (tarfile.SYMTYPE, tarfile.LNKTYPE, tarfile.FIFOTYPE):
            member = tarfile.TarInfo('strongswan-5.9.6/unsafe')
            member.type = kind
            member.linkname = '/etc/passwd'
            self.tar(member)
            with self.assertRaises(INPUT.InvalidInputs): self.verify()

    def test_missing_generated_files_and_nonexecutable_configure(self):
        self.tar(omit_parser=True)
        with self.assertRaises(INPUT.InvalidInputs): self.verify()
        self.tar(executable=False)
        with self.assertRaises(INPUT.InvalidInputs): self.verify()

    def test_source_and_expanded_bounds(self):
        with patch.object(INPUT, 'MAX_SOURCE', 1):
            with self.assertRaises(INPUT.InvalidInputs): self.verify()
        with patch.object(INPUT, 'MAX_EXPANDED', 1):
            with self.assertRaises(INPUT.InvalidInputs): self.verify()

    def test_source_symlink_and_replacement_refused(self):
        link = self.root / 'alias.tar.bz2'
        link.symlink_to(self.source)
        with self.assertRaises(OSError): INPUT.verify(self.vpp, link, '5.9.6', self.digest)
        original = os.read
        replaced = False
        source_inode = self.source.stat().st_ino
        def swap(fd, size):
            nonlocal replaced
            result = original(fd, size)
            if not replaced and os.fstat(fd).st_ino == source_inode:
                replacement = self.root / 'replacement'
                replacement.write_bytes(self.source.read_bytes())
                replacement.replace(self.source)
                replaced = True
            return result
        with patch.object(INPUT.os, 'read', swap):
            with self.assertRaises(INPUT.InvalidInputs): self.verify()

    def test_truncated_bzip2_cli_refuses_without_traceback(self):
        self.source.write_bytes(self.source.read_bytes()[:24])
        digest = hashlib.sha256(self.source.read_bytes()).hexdigest()
        errors = io.StringIO()
        with contextlib.redirect_stderr(errors):
            result = INPUT.main(['--vpp-output', str(self.vpp), '--source', str(self.source),
                '--source-version', '5.9.6', '--source-sha256', digest])
        self.assertEqual(result, 1)
        self.assertIn('P11 input verification failed:', errors.getvalue())
        self.assertNotIn('Traceback', errors.getvalue())

    def test_real_vpp_gate_refuses_synthetic_build(self):
        self.gate.stop()
        with self.assertRaises(INPUT.InvalidInputs): self.verify()


if __name__ == '__main__':
    result = unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(IntakeTests))
    sys.exit(0 if result.testsRun and result.wasSuccessful() and not result.skipped else 1)
