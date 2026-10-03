"""Synthetic real dpkg-deb fixtures; not product or appliance acceptance."""
import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import json
import sys

SPEC = importlib.util.spec_from_file_location('bundle_verify', Path(__file__).with_name('verify.py'))
VERIFY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(VERIFY)


class BundleTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)

    def archive(self, name, version='1.0', extra='', architecture='amd64', lower_required=False, mixed_required=False):
        source = self.root / (name + '-source')
        (source / 'DEBIAN').mkdir(parents=True)
        required = (f'package: {name}\nversion: {version}\narchitecture: {architecture}\n'
                    if lower_required else
                    f'Package: {name}\nVersion: {version}\nArchitecture: {architecture}\n')
        if mixed_required:
            required = required.replace('Package:', 'pAcKaGe:').replace(
                'Version:', 'vErSiOn:').replace('Architecture:', 'aRcHiTeCtUrE:')
        (source / 'DEBIAN/control').write_text(
            required +
            'Maintainer: Test <test@example.invalid>\nDescription: synthetic fixture\n' + extra)
        artifact = self.root / (name + '.deb')
        subprocess.run(['dpkg-deb', '--build', '--root-owner-group', str(source), str(artifact)],
                       check=True, capture_output=True)
        return artifact

    def test_real_deb_metadata_hash_and_architecture(self):
        path = self.archive('vrx-agent')
        package = VERIFY.metadata(path)
        self.assertEqual(package['fields']['Package'], 'vrx-agent')
        self.assertEqual(len(package['sha256']), 64)
        path.write_bytes(path.read_bytes() + b'changed')
        self.assertNotEqual(VERIFY.metadata(path)['sha256'], package['sha256'])
        with self.assertRaises(VERIFY.InvalidBundle):
            VERIFY.metadata(self.archive('foreign', architecture='arm64'))

    def test_symlink_rejected(self):
        original = self.archive('vrx-api')
        link = self.root / 'alias.deb'
        link.symlink_to(original)
        with self.assertRaises(VERIFY.InvalidBundle):
            VERIFY.metadata(link)

    def test_dependency_closure_versions_and_virtual_provides(self):
        consumer = VERIFY.metadata(self.archive('consumer', extra=
            'Pre-Depends: base (>= 2)\nDepends: missing | virtual (= 3)\n'))
        base = VERIFY.metadata(self.archive('base', version='2'))
        provider = VERIFY.metadata(self.archive('provider', extra='Provides: virtual (= 3)\n'))
        packages = {'consumer': consumer, 'base': base, 'provider': provider}
        VERIFY.validate_set(packages, {'consumer'})
        del packages['base']
        with self.assertRaises(VERIFY.InvalidBundle):
            VERIFY.validate_set(packages, {'consumer'})
        packages['base'] = VERIFY.metadata(self.archive('base-old', version='1'))
        with self.assertRaises(VERIFY.InvalidBundle):
            VERIFY.validate_set(packages, {'consumer'})

    def test_missing_roots_conflicts_and_unversioned_provider(self):
        one = VERIFY.metadata(self.archive('one', extra='Conflicts: virtual\n'))
        two = VERIFY.metadata(self.archive('two', extra='Provides: virtual\n'))
        with self.assertRaises(VERIFY.InvalidBundle):
            VERIFY.validate_set({'one': one}, {'vrx-meta'})
        with self.assertRaises(VERIFY.InvalidBundle):
            VERIFY.validate_set({'one': one, 'two': two}, {'one'})
        consumer = VERIFY.metadata(self.archive('consumer', extra='Depends: virtual (>= 1)\n'))
        with self.assertRaises(VERIFY.InvalidBundle):
            VERIFY.validate_set({'consumer': consumer, 'two': two}, {'consumer'})

    def test_self_conflict_provides_allowed(self):
        package = VERIFY.metadata(self.archive('one', extra='Provides: virtual\nConflicts: virtual\n'))
        VERIFY.validate_set({'one': package}, {'one'})

    def test_unsupported_relationship_fail_closed(self):
        for relation in ('foreign:arm64', 'foreign:any', 'foreign:native',
                         'foreign:amd64', 'one [amd64]', 'one <!profile>', 'one $(bad)'):
            with self.assertRaises(VERIFY.InvalidBundle):
                VERIFY.relations(relation)

    def test_runtime_roots_use_existing_installer(self):
        roots = VERIFY.runtime_roots()
        self.assertTrue(VERIFY.PRODUCT <= roots)
        self.assertTrue({'frr', 'frr-pythontools', 'nodejs', 'postgresql-18'} <= roots)

    def test_control_output_bound(self):
        with self.assertRaises(VERIFY.InvalidBundle):
            VERIFY.run([sys.executable, '-c', 'print("x" * 70000)'], VERIFY.MAX_CONTROL)

    def test_bundle_inventory_and_missing_runtime_with_stubbed_vpp_boundary(self):
        # Real synthetic .deb inputs; only VPP provenance gate is stubbed. This
        # cannot establish genuine VPP build, release trust or install acceptance.
        vpp_root = self.root / 'vpp'
        vpp_root.mkdir()
        vpp = self.archive('vpp', version='26.06-release+vrx1')
        vpp.rename(vpp_root / vpp.name)
        vpp_metadata = VERIFY.metadata(vpp_root / 'vpp.deb')
        (vpp_root / 'manifest.json').write_text(json.dumps({'packages': [
            {'package': 'vpp', 'file': 'vpp.deb', 'ship': True,
             'version': '26.06-release+vrx1', 'architecture': 'amd64',
             'sha256': vpp_metadata['sha256'], 'size': vpp_metadata['size']}]}))
        for name in VERIFY.runtime_roots():
            self.archive(name)
        original_run = VERIFY.run
        gate_calls = []
        def fake_gate(argv, limit=1024 * 1024, pass_fds=()):
            if argv[0] == 'bash':
                gate_calls.append(argv)
                self.assertIn('--install-gate', argv)
                self.assertNotIn('--no-tests', argv)
                return 'synthetic provenance boundary stub'
            return original_run(argv, limit, pass_fds)
        with patch.object(VERIFY, 'run', fake_gate):
            plan = VERIFY.verify(self.root)
            self.assertEqual(plan['product_version'], '1.0')
            self.assertTrue(all(not Path(path).is_absolute() for path in plan['install_files']))
            (self.root / 'frr.deb').unlink()
            with self.assertRaises(VERIFY.InvalidBundle):
                VERIFY.verify(self.root)
        self.assertEqual(len(gate_calls), 2)

    def test_real_vpp_gate_rejects_missing_build(self):
        (self.root / 'vpp').mkdir()
        (self.root / 'vpp/manifest.json').write_text('{}')
        with self.assertRaises(VERIFY.InvalidBundle):
            VERIFY.verify(self.root)

    def test_snapshot_metadata_hash_survives_source_replacement(self):
        source = self.archive('original')
        replacement = self.archive('replacement')
        original_run = VERIFY.run
        def replace_at_inspection(argv, limit=1024 * 1024, pass_fds=()):
            if argv[0] == 'dpkg-deb':
                replacement.replace(source)
            return original_run(argv, limit, pass_fds)
        with patch.object(VERIFY, 'run', replace_at_inspection):
            with self.assertRaises(VERIFY.InvalidBundle):
                VERIFY.metadata(source)

    def test_relationship_and_archive_limits(self):
        with self.assertRaises(VERIFY.InvalidBundle):
            VERIFY.relations(','.join(['one'] * 257))
        with self.assertRaises(VERIFY.InvalidBundle):
            VERIFY.relations('|'.join(['one'] * 33))
        with patch.object(VERIFY, 'MAX_ARCHIVE', 1):
            with self.assertRaises(VERIFY.InvalidBundle):
                VERIFY.metadata(self.archive('too-large'))

    def test_versions_and_breaks(self):
        for version in ('bad version', 'foo', '-1', '1/' ):
            with self.assertRaises(VERIFY.InvalidBundle):
                VERIFY.validate_version(version)
        one = VERIFY.metadata(self.archive('one', extra='Breaks: two (<< 2)\n'))
        two = VERIFY.metadata(self.archive('two', version='1'))
        with self.assertRaises(VERIFY.InvalidBundle):
            VERIFY.validate_set({'one': one, 'two': two}, {'one'})
        with self.assertRaises(VERIFY.InvalidBundle):
            VERIFY.relations('virtual (= nonsense)')

    def test_real_deb_case_insensitive_dependencies_and_predepends(self):
        for index, key in enumerate(('depends', 'dEpEnDs', 'pre-depends', 'pRe-DePeNdS')):
            base_name = 'base-' + str(index)
            consumer = VERIFY.metadata(self.archive('consumer-' + str(index), extra=
                key + ': ' + base_name + ' (>= 2)\n'))
            self.assertIn('Depends' if 'pre' not in key.lower() else 'Pre-Depends', consumer['fields'])
            with self.assertRaises(VERIFY.InvalidBundle):
                VERIFY.validate_set({'consumer': consumer}, set())
            too_old = VERIFY.metadata(self.archive(base_name, version='1'))
            with self.assertRaises(VERIFY.InvalidBundle):
                VERIFY.validate_set({'consumer': consumer, base_name: too_old}, set())
        base = VERIFY.metadata(self.archive('base', version='2'))
        valid = VERIFY.metadata(self.archive('valid', extra='dEpEnDs: base (>= 2)\n'))
        VERIFY.validate_set({'valid': valid, 'base': base}, set())

    def test_real_deb_case_insensitive_provides_and_conflicts(self):
        provider = VERIFY.metadata(self.archive('provider', extra=
            'pRoViDeS: virtual (= 3)\nMulti-arch: foreign\n'))
        consumer = VERIFY.metadata(self.archive('consumer', extra='depends: virtual (= 3)\n'))
        VERIFY.validate_set({'provider': provider, 'consumer': consumer}, set())
        self.assertEqual(provider['fields']['Multi-Arch'], 'foreign')
        for index, key in enumerate(('conflicts', 'cOnFlIcTs', 'breaks', 'bReAkS')):
            incompatible = VERIFY.metadata(self.archive('incompatible-' + str(index), extra=
                key + ': virtual (>= 2)\n'))
            with self.assertRaises(VERIFY.InvalidBundle):
                VERIFY.validate_set({'provider': provider, 'incompatible': incompatible}, set())

    def test_real_deb_lowercase_required_control_fields(self):
        for name, options in (('lowercase', {'lower_required': True}),
                              ('mixedcase', {'mixed_required': True})):
            package = VERIFY.metadata(self.archive(name, **options))
            self.assertEqual(package['fields']['Package'], name)
            self.assertEqual(package['fields']['Version'], '1.0')
            self.assertEqual(package['fields']['Architecture'], 'amd64')

    def test_case_insensitive_duplicates_parser_only(self):
        # dpkg-deb itself rejects duplicate control fields, so malformed duplicate
        # input is a direct parser regression, not a valid built-deb fixture.
        for first, second in (('Depends', 'depends'), ('Package', 'PACKAGE'),
                              ('X-Unknown', 'x-unknown')):
            with self.assertRaises(VERIFY.InvalidBundle):
                VERIFY.parse_control(first + ': one\n' + second + ': two\n')
        parsed = VERIFY.parse_control('dEpEnDs: base\n | alternative\nX-Custom: value\n')
        self.assertEqual(parsed, {'Depends': 'base | alternative', 'X-Custom': 'value'})


if __name__ == '__main__':
    result = unittest.TextTestRunner(verbosity=2).run(
        unittest.defaultTestLoader.loadTestsFromTestCase(BundleTests))
    sys.exit(0 if result.testsRun and result.wasSuccessful() and not result.skipped
             and not result.expectedFailures else 1)
