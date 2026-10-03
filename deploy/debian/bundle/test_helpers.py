"""Real Debian fixtures and unchanged full VPP gate; no host installation.

These tiny synthetic payloads prove helper delivery, NOT real build provenance.
"""
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


HELPERS = module('helper_export', 'helpers.py')
RECIPIENT = module('recipient_bootstrap', 'recipient.py')
VERIFY = HELPERS.EXPORT.VERIFY


class HelpersTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary = tempfile.TemporaryDirectory(prefix='vrx-helper-fixture-')
        cls.root = Path(cls.temporary.name)
        cls.delivery = cls.root / 'delivery'
        cls.delivery.mkdir()
        (cls.delivery / 'vpp').mkdir()
        cls.helpers = cls.root / 'helpers.tar'
        cls.report_data = HELPERS.export_helpers(cls.helpers)
        cls.report = cls.root / 'trusted-helper-report.json'
        cls.report.write_text(json.dumps(cls.report_data, sort_keys=True))
        cls.report_digest = hashlib.sha256(cls.report.read_bytes()).hexdigest()
        cls.launcher = cls.root / 'recipient.py'
        shutil.copyfile(Path(RECIPIENT.__file__), cls.launcher)
        values = subprocess.run(['/bin/bash', '-c',
            'source "$1"; vrx_parse_version "$2"; '
            'printf "%s\\n" "$VPP_UPSTREAM_URL" "$VPP_TAG" "$VPP_TAG_OBJECT" "$VPP_COMMIT" '
            '"$VPP_DEB_VERSION" "$VPP_LOCAL_REV" "$VPP_PACKAGES" "$VPP_PACKAGES_SHIP"; '
            'vrx_pydeps_parse "$3"', 'fixture', str(HELPERS.ROOT / 'deploy/vpp/lib.sh'),
            str(HELPERS.ROOT / 'deploy/vpp/VERSION'), str(HELPERS.ROOT / 'deploy/vpp/pydeps.lock')],
            check=True, capture_output=True, text=True).stdout.splitlines()
        url, tag, tag_object, commit, base, revision, packages, ship = values[:8]
        version = base + '+vrx' + revision
        for name in sorted(VERIFY.runtime_roots()):
            cls.deb(name, '1.0', cls.delivery / (name + '.deb'))
        entries = []
        for name in packages.split():
            path = cls.delivery / 'vpp' / (name + '.deb')
            cls.deb(name, version, path)
            entries.append({'package': name, 'file': path.name, 'ship': name in ship.split(),
                            'version': version, 'architecture': 'amd64', 'size': path.stat().st_size,
                            'sha256': hashlib.sha256(path.read_bytes()).hexdigest()})
        build_patches = []
        for line in (HELPERS.ROOT / 'deploy/vpp/build-patches/series').read_text().splitlines():
            line = line.split('#', 1)[0].strip()
            if line:
                name = 'build-patches/' + line.split()[0]
                build_patches.append({'name': name, 'kind': 'build', 'sha256': hashlib.sha256(
                    (HELPERS.ROOT / 'deploy/vpp' / name).read_bytes()).hexdigest()})
        python = [dict(zip(('name', 'version', 'sha256', 'file', 'url'), line.split()))
                  for line in values[8:]]
        manifest = {'schema': 'vrx.vpp-debs.manifest/v2',
                    'upstream': {'url': url, 'tag': tag, 'tag_object': tag_object, 'commit': commit},
                    'version': version, 'variant': 'default', 'patches': [], 'options': {'demo': False},
                    'build': {'builder_dirty': False, 'build_patches': build_patches,
                              'inputs': {'python': python, 'dpdk_meson_venv':
                                         [f"{item['name']}=={item['version']}" for item in python]}},
                    'packages': entries}
        (cls.delivery / 'vpp/manifest.json').write_text(json.dumps(manifest))
        (cls.delivery / 'vpp/SHA256SUMS').write_text(''.join(
            entry['sha256'] + '  ' + entry['file'] + '\n' for entry in entries))
        # Full VPP tests + real archive metadata checks, without mocking run().
        with HELPERS.EXPORT.INSTALL.safe_environment():
            cls.plan = VERIFY.verify(cls.delivery)
        cls.manifest = cls.root / 'trusted-runtime.json'
        cls.manifest.write_text(json.dumps(cls.plan))

    @classmethod
    def tearDownClass(cls):
        cls.temporary.cleanup()

    @classmethod
    def deb(cls, name, version, path):
        source = cls.root / ('source-' + name)
        (source / 'DEBIAN').mkdir(parents=True, exist_ok=True)
        (source / 'DEBIAN/control').write_text(
            f'Package: {name}\nVersion: {version}\nArchitecture: amd64\n'
            'Maintainer: Fixture <test@example.invalid>\nDescription: synthetic, not release provenance\n')
        subprocess.run(['dpkg-deb', '--build', '--root-owner-group', str(source), str(path)],
                       check=True, capture_output=True)

    def invocation(self, **overrides):
        values = {'directory': self.delivery, 'manifest': self.manifest, 'helpers': self.helpers,
                  'helper-report': self.report, 'helper-report-sha256': self.report_digest}
        values.update(overrides)
        command = ['/usr/bin/python3', '-I', str(self.launcher), str(values.pop('directory'))]
        for key, value in values.items():
            command += ['--' + key, str(value)]
        return subprocess.run(command, cwd=self.root, capture_output=True, text=True,
                              env={'PATH': '/usr/bin:/bin', 'HOME': '/untrusted',
                                   'PYTHONPATH': str(self.root), 'BASH_ENV': '/untrusted/bashrc'})

    def test_outside_checkout_real_gate_and_readonly_preflight(self):
        # All executable project files used by the child are delivered helpers
        # in /var/tmp; cwd/PYTHONPATH are outside the checkout, Python uses -I.
        (self.root / 'sitecustomize.py').write_text('raise RuntimeError("untrusted Python startup")\n')
        result = self.invocation()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), {'mode': 'plan', 'manifest': self.plan})
        self.assertFalse(self.root.is_relative_to(HELPERS.ROOT))
        with tarfile.open(self.helpers) as archive:
            self.assertEqual(set(archive.getnames()), set(HELPERS.source_files()))
            for member in archive:
                self.assertEqual(archive.extractfile(member).read(), (HELPERS.ROOT / member.name).read_bytes())

    def test_deterministic_export_and_no_overwrite(self):
        output = self.root / 'second.tar'
        HELPERS.export_helpers(output)
        self.assertEqual(self.helpers.read_bytes(), output.read_bytes())
        with self.assertRaises(FileExistsError):
            HELPERS.export_helpers(output)
        self.assertEqual(list(self.root.glob('.vrx-helpers-*')), [])

    def test_modified_archive_and_modified_report_refuse_before_execution(self):
        modified = self.root / 'modified.tar'
        data = bytearray(self.helpers.read_bytes())
        data[1024] ^= 1
        modified.write_bytes(data)
        result = self.invocation(helpers=modified)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('helper archive differs', result.stderr)
        report = self.root / 'modified-report.json'
        rewritten = dict(self.report_data, sha256=hashlib.sha256(data).hexdigest())
        report.write_text(json.dumps(rewritten))
        result = self.invocation(helpers=modified, **{'helper-report': report})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('externally trusted SHA256', result.stderr)

    def altered(self, change):
        output = self.root / 'adversarial.tar'
        records = []
        with tarfile.open(self.helpers) as archive:
            for member in archive:
                records.append((member, archive.extractfile(member).read()))
        records = change(records)
        with tarfile.open(output, 'w', format=tarfile.USTAR_FORMAT) as archive:
            for member, content in records:
                archive.addfile(member, io.BytesIO(content) if member.isfile() else None)
        # A deliberately authorized archive hash does not authorize member
        # divergence: the independent report member inventory still controls it.
        report = dict(self.report_data, sha256=hashlib.sha256(output.read_bytes()).hexdigest(),
                      archive_bytes=output.stat().st_size)
        destination = self.root / 'unpacked'
        destination.mkdir(exist_ok=True)
        return output, report, destination

    def test_inventory_refuses_omission_extra_duplicate_link_and_mode(self):
        for case in ('missing', 'extra', 'duplicate', 'link', 'mode', 'traversal', 'modified'):
            with self.subTest(case=case):
                def change(records):
                    member, content = records[0]
                    if case == 'missing':
                        return records[1:]
                    if case == 'extra':
                        extra = tarfile.TarInfo('unknown.py')
                        extra.mode = 0o600
                        return records + [(extra, b'')]
                    if case == 'duplicate':
                        return records + [records[0]]
                    if case == 'link':
                        member.type = tarfile.SYMTYPE
                        member.linkname = '/tmp/escape'
                    elif case == 'mode':
                        member.mode = 0o777
                    elif case == 'traversal':
                        member.name = '../escape.py'
                    elif case == 'modified':
                        content = b'X' + content[1:]
                    return [(member, content)] + records[1:]
                output, report, destination = self.altered(change)
                with self.assertRaises(ValueError), patch.object(RECIPIENT.subprocess, 'run') as execute:
                    RECIPIENT.unpack_helpers(output, report, destination)
                execute.assert_not_called()
                shutil.rmtree(destination)

    def test_missing_dependency_fails_real_gate_even_with_reauthorized_inventory(self):
        def omit(records):
            return [(member, content) for member, content in records if member.name != 'deploy/vpp/lib.sh']
        output, report, destination = self.altered(omit)
        shutil.rmtree(destination)
        report['files'] = dict(report['files'])
        del report['files']['deploy/vpp/lib.sh']
        path = self.root / 'incomplete-trusted-report.json'
        path.write_text(json.dumps(report))
        result = self.invocation(helpers=output, **{'helper-report': path,
                                  'helper-report-sha256': hashlib.sha256(path.read_bytes()).hexdigest()})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('bash failed', result.stderr)

    def test_modified_runtime_manifest_fails_actual_preflight(self):
        path = self.root / 'wrong-runtime.json'
        path.write_text('{}')
        result = self.invocation(manifest=path)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('differs from trusted expected manifest', result.stderr)

    def test_nonregular_and_oversized_helpers_rejected(self):
        link = self.root / 'symlink.tar'
        link.symlink_to(self.helpers)
        self.assertNotEqual(self.invocation(helpers=link).returncode, 0)
        big = self.root / 'oversized.tar'
        with big.open('wb') as stream:
            stream.truncate(RECIPIENT.MAX_TAR + 1)
        result = self.invocation(helpers=big)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('bounded regular file', result.stderr)


if __name__ == '__main__':
    unittest.main()
