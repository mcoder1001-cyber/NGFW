#!/usr/bin/env python3
"""Run the production archive/install block against temporary files and fake APT/curl."""
import hashlib
import io
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]


class Containerlab(unittest.TestCase):
    def run_fixture(self, kind='regular', digest_ok=True, apt_status=0):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            archive = root / 'fixture.tar.gz'
            with tarfile.open(archive, 'w:gz') as output:
                member = tarfile.TarInfo('containerlab' if kind != 'missing' else '../escape')
                member.mode = 0o6755
                if kind == 'symlink':
                    member.type = tarfile.SYMTYPE
                    member.linkname = '/etc/passwd'
                    output.addfile(member)
                else:
                    content = b'fixture binary\n'
                    member.size = len(content)
                    output.addfile(member, io.BytesIO(content))
                    if kind == 'duplicate':
                        output.addfile(member, io.BytesIO(content))
            digest = hashlib.sha256(archive.read_bytes()).hexdigest() if digest_ok else '0' * 64
            stubs = root / 'stubs'
            stubs.mkdir()
            curl = stubs / 'curl'
            curl.write_text('#!/bin/sh\n[ "$1" = -fsSL ] && [ "$3" = -o ] || exit 90\ncp "$FIXTURE_ARCHIVE" "$4"\n')
            curl.chmod(0o755)
            apt = stubs / 'apt-get'
            apt.write_text('#!/bin/sh\nprintf "apt %s\\n" "$*" >> "$CALL_LOG"\nexit "$APT_STATUS"\n')
            apt.chmod(0o755)
            destination = root / 'bin'
            destination.mkdir()
            binary = destination / 'containerlab'
            binary.write_bytes(b'original\n')
            binary.chmod(0o755)
            source = (ROOT / 'scripts/40-install-lab.sh').read_text()
            start = source.index('# BEGIN VERIFIED CONTAINERLAB')
            end = source.index('# END VERIFIED CONTAINERLAB', start)
            fragment = source[start:end].replace('/tmp/vrx-containerlab.', str(root / 'vrx-containerlab.')).replace('/usr/local/bin', str(destination))
            harness = f'set -euo pipefail\nCONTAINERLAB_URL=https://fixture.invalid/pinned\nCONTAINERLAB_SHA256={digest}\nVIRT=() TRAFFIC=() ANALYSIS=() BASE=()\n'
            log = root / 'calls'
            result = subprocess.run(['bash', '-c', harness + fragment], capture_output=True, text=True,
                                    env=dict(os.environ, PATH=f'{stubs}:/usr/bin:/bin', FIXTURE_ARCHIVE=str(archive), CALL_LOG=str(log), APT_STATUS=str(apt_status)))
            return result, binary.read_bytes(), binary.stat().st_mode & 0o7777, log.read_text() if log.exists() else '', list(destination.glob('.containerlab.*')), list(root.glob('vrx-containerlab.*'))

    def test_verified_binary_installed_without_archive_privilege_bits(self):
        result, content, mode, calls, targets, work = self.run_fixture()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(content, b'fixture binary\n')
        self.assertEqual(mode, 0o755)
        self.assertEqual(calls.splitlines(), ['apt update', 'apt install -y'])
        self.assertEqual(targets + work, [])

    def test_wrong_hash_preserves_original_before_apt(self):
        result, content, _, calls, targets, work = self.run_fixture(digest_ok=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(content, b'original\n')
        self.assertEqual(calls, '')
        self.assertEqual(targets + work, [])

    def test_links_missing_and_duplicate_binary_refused_before_apt(self):
        for kind in ('symlink', 'missing', 'duplicate'):
            with self.subTest(kind=kind):
                result, content, _, calls, targets, work = self.run_fixture(kind=kind)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(content, b'original\n')
                self.assertEqual(calls, '')
                self.assertEqual(targets + work, [])

    def test_apt_failure_preserves_existing_binary(self):
        result, content, _, calls, targets, work = self.run_fixture(apt_status=42)
        self.assertEqual(result.returncode, 42)
        self.assertEqual(content, b'original\n')
        self.assertEqual(calls.splitlines(), ['apt update'])
        self.assertEqual(targets + work, [])

    def test_nonmutating_config_and_fixed_source(self):
        result = subprocess.run(['bash', str(ROOT / 'scripts/40-install-lab.sh'), '--check-config'], text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), 'containerlab=0.79.0 sha256=f90d36d58bb6c4afd3b3a4dca006b81594c6d16f7a04be0184b03f44291085a2')
        source = (ROOT / 'scripts/40-install-lab.sh').read_text()
        self.assertNotIn('get.containerlab.dev', source)
        self.assertNotIn('bash -c', source)


if __name__ == '__main__':
    unittest.main(verbosity=2)
