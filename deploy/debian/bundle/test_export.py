"""Synthetic real Debian archives; stubbed VPP boundary, no installation."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('bundle_export', Path(__file__).with_name('export.py'))
EXPORT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(EXPORT)
VERIFY = EXPORT.VERIFY


class ExportTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.delivery = self.root / 'delivery'
        self.delivery.mkdir()
        (self.delivery / 'vpp').mkdir()
        original = VERIFY.run
        def gate(argv, limit=1024 * 1024, pass_fds=()):
            if argv[0] == 'bash':
                self.assertIn('--install-gate', argv)
                self.assertIn('--require-files', argv)
                return 'synthetic VPP provenance boundary, not release evidence'
            return original(argv, limit, pass_fds)
        self.gate = patch.object(VERIFY, 'run', gate)
        self.gate.start()
        self.addCleanup(self.gate.stop)
        for name in sorted(VERIFY.runtime_roots()):
            self.archive(name, self.delivery / (name + '.deb'))
        entries = []
        for name, shipping in (('vpp', True), ('vpp-dbg', False)):
            artifact = self.delivery / 'vpp' / (name + '.deb')
            self.archive(name, artifact, '26.06-release+vrx1')
            metadata = VERIFY.metadata(artifact)
            entries.append({'package': name, 'file': artifact.name, 'ship': shipping,
                'version': '26.06-release+vrx1', 'architecture': 'amd64',
                'sha256': metadata['sha256'], 'size': metadata['size']})
        (self.delivery / 'vpp/manifest.json').write_text(json.dumps({'packages': entries}))
        (self.delivery / 'vpp/SHA256SUMS').write_text('synthetic full VPP fixture\n')
        self.plan = VERIFY.verify(self.delivery)
        self.manifest = self.root / 'trusted.json'
        self.manifest.write_text(json.dumps(self.plan))
        self.output = self.root / 'delivery.tar'

    def archive(self, name, output, version='1.0'):
        source = self.root / ('source-' + name)
        (source / 'DEBIAN').mkdir(parents=True)
        (source / 'DEBIAN/control').write_text(
            f'Package: {name}\nVersion: {version}\nArchitecture: amd64\n'
            'Maintainer: Fixture <test@example.invalid>\nDescription: synthetic fixture\n')
        subprocess.run(['dpkg-deb', '--build', '--root-owner-group', str(source), str(output)],
                       check=True, capture_output=True)

    def assert_no_partials(self):
        self.assertEqual(list(self.root.glob('.vrx-export-*')), [])

    def test_deterministic_regular_members_and_verifier_roundtrip(self):
        report = EXPORT.export(self.delivery, self.manifest, self.output)
        second = self.root / 'second.tar'
        EXPORT.export(self.delivery, self.manifest, second)
        self.assertEqual(self.output.read_bytes(), second.read_bytes())
        self.assertEqual(self.output.stat().st_mode & 0o777, 0o600)
        restored = self.root / 'restored'
        restored.mkdir()
        with tarfile.open(self.output, 'r:') as archive:
            names = archive.getnames()
            self.assertEqual(names, sorted(names))
            expected = {entry['file'] for entry in self.plan['artifacts']} | {
                'vpp/vpp-dbg.deb', 'vpp/manifest.json', 'vpp/SHA256SUMS'}
            self.assertEqual(set(names), expected)
            self.assertNotIn('trusted.json', names)
            for member in archive.getmembers():
                self.assertTrue(member.isfile())
                self.assertEqual(member.mtime, 0)
                self.assertNotIn('..', Path(member.name).parts)
                self.assertFalse(Path(member.name).is_absolute())
                target = restored / member.name
                target.parent.mkdir(parents=True, exist_ok=True)
                with archive.extractfile(member) as stream:
                    target.write_bytes(stream.read())
        self.assertEqual(VERIFY.verify(restored), self.plan)
        self.assertNotIn('vpp/vpp-dbg.deb', self.plan['install_files'])
        self.assertEqual(report['members'], len(expected))
        self.assertEqual(report['archive_bytes'], self.output.stat().st_size)
        self.assertEqual(report['sha256'], hashlib.sha256(self.output.read_bytes()).hexdigest())
        self.assertGreater(report['archive_bytes'], report['bytes'])
        self.assert_no_partials()

    def test_trusted_manifest_mismatch_and_missing_runtime_rejected(self):
        original = self.manifest.read_text()
        self.manifest.write_text('{}')
        with self.assertRaises(EXPORT.InvalidBundle):
            EXPORT.export(self.delivery, self.manifest, self.output)
        self.manifest.write_text(original)
        (self.delivery / 'frr.deb').unlink()
        with self.assertRaises(EXPORT.InvalidBundle):
            EXPORT.export(self.delivery, self.manifest, self.output)
        self.assertFalse(self.output.exists())
        self.assert_no_partials()

    def test_existing_output_and_symlink_are_never_overwritten(self):
        self.output.write_bytes(b'keep existing')
        with self.assertRaises(EXPORT.InvalidBundle):
            EXPORT.export(self.delivery, self.manifest, self.output)
        self.assertEqual(self.output.read_bytes(), b'keep existing')
        self.output.unlink()
        self.output.symlink_to(self.manifest)
        with self.assertRaises(EXPORT.InvalidBundle):
            EXPORT.export(self.delivery, self.manifest, self.output)
        self.assertTrue(self.output.is_symlink())
        self.assertEqual(json.loads(self.manifest.read_text()), self.plan)

    def test_overlap_traversal_symlink_parent_and_writable_parent_rejected(self):
        for output in (self.delivery / 'inside.tar', self.manifest,
                       self.root / 'x/../escape.tar'):
            with self.assertRaises(EXPORT.InvalidBundle):
                EXPORT.export(self.delivery, self.manifest, output)
        alias = self.root / 'alias'
        alias.symlink_to(self.root, target_is_directory=True)
        with self.assertRaises(OSError):
            EXPORT.export(self.delivery, self.manifest, alias / 'outside.tar')
        writable = self.root / 'writable'
        writable.mkdir(mode=0o777)
        writable.chmod(0o777)
        with self.assertRaises(EXPORT.InvalidBundle):
            EXPORT.export(self.delivery, self.manifest, writable / 'outside.tar')

    def test_injected_write_failure_cleans_partial(self):
        def fail(stream, snapshot, files):
            stream.write(b'partial')
            raise OSError('synthetic write failure')
        with patch.object(EXPORT, 'write_tar', fail):
            with self.assertRaises(OSError):
                EXPORT.export(self.delivery, self.manifest, self.output)
        self.assertFalse(self.output.exists())
        self.assert_no_partials()

    def test_snapshot_archive_mutation_rejected_during_stream(self):
        original = EXPORT.write_tar
        def mutate(stream, snapshot, files):
            path = snapshot / 'vrx-agent.deb'
            data = path.read_bytes()
            path.write_bytes(data[:-1] + bytes([data[-1] ^ 1]))
            original(stream, snapshot, files)
        with patch.object(EXPORT, 'write_tar', mutate):
            with self.assertRaises(EXPORT.InvalidBundle):
                EXPORT.export(self.delivery, self.manifest, self.output)
        self.assertFalse(self.output.exists())
        self.assert_no_partials()

    def test_nonshipping_archive_and_ancillary_mutation_rejected(self):
        original = EXPORT.write_tar
        for relative in ('vpp/vpp-dbg.deb', 'vpp/SHA256SUMS'):
            def mutate(stream, snapshot, files):
                original(stream, snapshot, files)
                path = snapshot / relative
                path.write_bytes(path.read_bytes() + b'changed')
            with patch.object(EXPORT, 'write_tar', mutate):
                with self.assertRaises(EXPORT.InvalidBundle):
                    EXPORT.export(self.delivery, self.manifest, self.output)
            self.assertFalse(self.output.exists())
            self.assert_no_partials()

    def test_concurrent_destination_creator_wins_without_overwrite(self):
        original = EXPORT.os.link
        def race(source, destination, **kwargs):
            self.output.write_bytes(b'concurrent owner')
            return original(source, destination, **kwargs)
        with patch.object(EXPORT.os, 'link', race):
            with self.assertRaises(FileExistsError):
                EXPORT.export(self.delivery, self.manifest, self.output)
        self.assertEqual(self.output.read_bytes(), b'concurrent owner')
        self.assert_no_partials()

    def test_destination_parent_replacement_is_detected_and_cleaned(self):
        parent = self.root / 'publish'
        parent.mkdir(mode=0o700)
        output = parent / 'delivery.tar'
        moved = self.root / 'moved-publish'
        original = EXPORT.os.link
        def replace_parent(source, destination, **kwargs):
            parent.rename(moved)
            parent.mkdir(mode=0o700)
            output.write_bytes(b'replacement directory owner')
            return original(source, destination, **kwargs)
        with patch.object(EXPORT.os, 'link', replace_parent):
            with self.assertRaises(EXPORT.InvalidBundle):
                EXPORT.export(self.delivery, self.manifest, output)
        self.assertEqual(output.read_bytes(), b'replacement directory owner')
        self.assertEqual(list(moved.iterdir()), [])

    def test_unsafe_member_name_is_rejected(self):
        unsafe = self.delivery / 'unsafe directory'
        unsafe.mkdir()
        (self.delivery / 'frr.deb').rename(unsafe / 'frr.deb')
        self.plan = VERIFY.verify(self.delivery)
        self.manifest.write_text(json.dumps(self.plan))
        with self.assertRaises(EXPORT.InvalidBundle):
            EXPORT.export(self.delivery, self.manifest, self.output)
        self.assertFalse(self.output.exists())


if __name__ == '__main__':
    result = unittest.TextTestRunner(verbosity=2).run(
        unittest.defaultTestLoader.loadTestsFromTestCase(ExportTests))
    sys.exit(0 if result.testsRun and result.wasSuccessful() and not result.skipped
             and not result.expectedFailures else 1)
