"""Real archive stage fixtures; provenance stub is never product acceptance."""
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tarfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))
from test_verify_inputs import IntakeTests

SPEC = importlib.util.spec_from_file_location('p11_stage', Path(__file__).with_name('prepare_stage.py'))
STAGE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(STAGE)


class StageTests(IntakeTests):
    def setUp(self):
        super().setUp()
        self.output = self.root / 'stage'
        # Share the intake fixture's explicitly stubbed gate, all other verification real.
        from test_verify_inputs import INPUT
        self.intake_patch = patch.object(STAGE, 'INTAKE', INPUT)
        STAGE.InvalidInputs = INPUT.InvalidInputs
        self.intake_patch.start()
        self.addCleanup(self.intake_patch.stop)
        for name in ('vpp-dev', 'libvppinfra-dev'):
            tree = self.root / ('source-' + name)
            (tree / 'usr/include').mkdir(parents=True)
            (tree / 'usr/include' / (name + '.h')).write_bytes(b'actual fixture header\n')
            artifact = self.vpp / (name + '.deb')
            subprocess.run(['dpkg-deb', '--build', '--root-owner-group', str(tree), str(artifact)],
                           check=True, capture_output=True)
            meta = INPUT.VERIFY.metadata(artifact)
            entry = next(e for e in self.entries if e['package'] == name)
            entry.update(sha256=meta['sha256'], size=meta['size'])
        self.manifest()

    def prepare(self, digest=None):
        return STAGE.prepare(self.vpp, self.source, '5.9.6', self.digest if digest is None else digest, self.output)

    def clean_failure(self):
        self.assertFalse(self.output.exists())
        self.assertEqual(list(self.root.glob('.p11-stage-*')), [])

    def test_materialised_bytes_and_private_modes(self):
        report = self.prepare()
        self.assertEqual(report['mode'], 'materialised-build-prerequisites')
        self.assertFalse(report['release_approved'])
        self.assertFalse(report['builder_ready'])
        self.assertEqual((self.output / 'source/strongswan-5.9.6/configure').read_bytes(), b'fixture')
        for name in ('vpp-dev', 'libvppinfra-dev'):
            self.assertEqual((self.output / f'vpp-dev/{name}/usr/include/{name}.h').read_bytes(),
                             b'actual fixture header\n')
        self.assertEqual(json.loads((self.output / 'intake.json').read_text()), report)
        for path in [self.output, *self.output.rglob('*')]:
            self.assertEqual(path.stat().st_mode & 0o077, 0)
        self.assertEqual(list(self.root.glob('.p11-stage-*')), [])

    def test_public_trust_boundary_and_wrong_digest_cleanup(self):
        for digest in (None, '', 123, 'bad', 'A' * 64):
            with patch.object(STAGE.INTAKE, 'verified_snapshot') as intake:
                with self.assertRaises(STAGE.InvalidInputs):
                    STAGE.prepare(self.vpp, self.source, '5.9.6', digest, self.output)
                intake.assert_not_called()
            self.clean_failure()
        with self.assertRaises(STAGE.InvalidInputs): self.prepare('0' * 64)
        self.clean_failure()

    def test_source_and_dev_original_replacement_after_verification(self):
        original = STAGE.extract
        changed = False
        def replacement(raw, target, budget, source=False):
            nonlocal changed
            if not changed:
                self.source.write_bytes(b'tampered original')
                for entry in self.entries:
                    (self.vpp / entry['file']).write_bytes(b'tampered original')
                changed = True
            return original(raw, target, budget, source)
        with patch.object(STAGE, 'extract', replacement):
            self.prepare()
        self.assertEqual((self.output / 'source/strongswan-5.9.6/configure').read_bytes(), b'fixture')
        self.assertEqual((self.output / 'vpp-dev/vpp-dev/usr/include/vpp-dev.h').read_bytes(),
                         b'actual fixture header\n')

    def test_aggregate_decoded_budget_and_no_partial_publication(self):
        original = STAGE.extract
        calls = []
        def observe(raw, target, budget, source=False):
            self.assertFalse(self.output.exists())
            calls.append(target.name)
            return original(raw, target, budget, source)
        # Each tiny fixture tar fits; their aggregate must still be refused.
        with patch.object(STAGE, 'MAX_TOTAL', 25000), patch.object(STAGE, 'extract', observe):
            with self.assertRaises(STAGE.InvalidInputs): self.prepare()
        self.assertEqual(calls, ['source', 'vpp-dev'])
        self.clean_failure()

    def test_budgets_and_late_failure_cleanup(self):
        for bound in ('MAX_FILE', 'MAX_TOTAL', 'MAX_TAR', 'MAX_MEMBERS'):
            with patch.object(STAGE, bound, 1):
                with self.assertRaises(STAGE.InvalidInputs): self.prepare()
            self.clean_failure()
        original = STAGE.deb_tar
        calls = 0
        def fail_second(package, raw, decoded):
            nonlocal calls
            calls += 1
            if calls == 2:
                raise OSError('injected late decoding failure')
            return original(package, raw, decoded)
        with patch.object(STAGE, 'deb_tar', fail_second):
            with self.assertRaises(OSError): self.prepare()
        self.clean_failure()

    def test_output_overwrite_and_atomic_race_refused(self):
        self.output.mkdir()
        (self.output / 'keep').write_bytes(b'owned')
        with self.assertRaises(STAGE.InvalidInputs): self.prepare()
        self.assertEqual((self.output / 'keep').read_bytes(), b'owned')
        (self.output / 'keep').unlink()
        self.output.rmdir()
        original = STAGE.publish
        def race(fd, tree, name):
            self.output.mkdir()
            return original(fd, tree, name)
        with patch.object(STAGE, 'publish', race):
            with self.assertRaises(STAGE.InvalidInputs): self.prepare()
        self.assertEqual(list(self.output.iterdir()), [])
        self.assertEqual(list(self.root.glob('.p11-stage-*')), [])

    def test_output_symlink_and_writable_parent_refused(self):
        self.output.symlink_to(self.source)
        before = self.source.read_bytes()
        with self.assertRaises(STAGE.InvalidInputs): self.prepare()
        self.assertEqual(self.source.read_bytes(), before)
        self.output.unlink()
        self.root.chmod(0o777)
        try:
            with self.assertRaises(STAGE.InvalidInputs): self.prepare()
        finally:
            self.root.chmod(0o700)
        self.clean_failure()

    def test_dev_unsafe_members_cleanup(self):
        for path, kind in (('../escape', tarfile.REGTYPE), ('/absolute', tarfile.REGTYPE),
                           ('./usr/../escape', tarfile.REGTYPE), ('./usr//empty', tarfile.REGTYPE),
                           ('./link', tarfile.SYMTYPE), ('./hard', tarfile.LNKTYPE),
                           ('./fifo', tarfile.FIFOTYPE), ('./device', tarfile.CHRTYPE)):
            def malicious(package, raw, decoded):
                with tarfile.open(fileobj=raw, mode='w') as archive:
                    member = tarfile.TarInfo(path)
                    member.type = kind
                    member.linkname = '/etc/passwd'
                    archive.addfile(member)
                raw.seek(0)
            with patch.object(STAGE, 'deb_tar', malicious):
                with self.assertRaises(STAGE.InvalidInputs): self.prepare()
            self.clean_failure()
        def duplicates(package, raw, decoded):
            with tarfile.open(fileobj=raw, mode='w') as archive:
                for _ in range(2): archive.addfile(tarfile.TarInfo('./same'))
            raw.seek(0)
        with patch.object(STAGE, 'deb_tar', duplicates):
            with self.assertRaises(STAGE.InvalidInputs): self.prepare()
        self.clean_failure()

    def test_symlink_ancestor_and_input_overlap_refused(self):
        alias = self.root / 'alias'
        alias.symlink_to(self.root, target_is_directory=True)
        parent = self.root / 'parent'
        parent.mkdir()
        self.output = alias / 'parent' / 'stage'
        with self.assertRaises(OSError): self.prepare()
        self.assertFalse((parent / 'stage').exists())
        self.output = self.vpp / 'stage'
        before = sorted(p.name for p in self.vpp.iterdir())
        with self.assertRaises(STAGE.InvalidInputs): self.prepare()
        self.assertEqual(sorted(p.name for p in self.vpp.iterdir()), before)
        self.output = self.source
        with self.assertRaises(STAGE.InvalidInputs): self.prepare()

    def test_parent_replacement_before_and_during_publication_rolls_back(self):
        for during in (False, True):
            parent = self.root / ('parent-' + str(during))
            parent.mkdir()
            moved = self.root / ('moved-' + str(during))
            self.output = parent / 'stage'
            original = STAGE.publish
            def replace(fd, tree, name):
                if during:
                    original(fd, tree, name)
                parent.rename(moved)
                parent.mkdir()
                if not during:
                    original(fd, tree, name)
            # Use a separate wrapper only for initial publication, preserving rollback.
            def wrapped(fd, tree, name):
                if name == 'stage':
                    replace(fd, tree, name)
                else:
                    original(fd, tree, name)
            with patch.object(STAGE, 'publish', wrapped):
                with self.assertRaises(STAGE.InvalidInputs): self.prepare()
            self.assertEqual(list(parent.iterdir()), [])
            self.assertEqual(list(moved.iterdir()), [])

    def test_real_linked_deb_is_refused_after_verified_intake(self):
        from test_verify_inputs import INPUT
        tree = self.root / 'source-vpp-dev'
        (tree / 'usr/include/link.h').symlink_to('/etc/passwd')
        artifact = self.vpp / 'vpp-dev.deb'
        subprocess.run(['dpkg-deb', '--build', '--root-owner-group', str(tree), str(artifact)],
                       check=True, capture_output=True)
        meta = INPUT.VERIFY.metadata(artifact)
        entry = next(e for e in self.entries if e['package'] == 'vpp-dev')
        entry.update(sha256=meta['sha256'], size=meta['size'])
        self.manifest()
        with self.assertRaises(STAGE.InvalidInputs): self.prepare()
        self.clean_failure()

    def test_real_stage_gate_refuses_synthetic_build(self):
        self.gate.stop()
        with self.assertRaises(STAGE.InvalidInputs): self.prepare()
        self.clean_failure()


if __name__ == '__main__':
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(StageTests)
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    sys.exit(0 if result.testsRun and result.wasSuccessful() and not result.skipped else 1)
