#!/usr/bin/env python3
"""Exercise build-bootstrap configuration without running any host mutation."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]


class BuildPreflight(unittest.TestCase):
    def run_check(self, digest, args=()):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            log = root / 'calls'
            for command in ('apt-get', 'curl', 'go', 'tar', 'rm', 'corepack', 'npm'):
                stub = root / command
                stub.write_text('#!/bin/sh\nprintf "%s\\n" "$0" >> "$CALL_LOG"\nexit 91\n')
                stub.chmod(0o755)
            env = dict(os.environ, PATH=f'{root}:/usr/bin:/bin', CALL_LOG=str(log))
            env.pop('VRX_GO_SHA256', None)
            if digest is not None:
                env['VRX_GO_SHA256'] = digest
            result = subprocess.run(['bash', str(ROOT / 'scripts/20-install-build.sh'), *args],
                                    env=env, text=True, capture_output=True)
            return result, log.read_text() if log.exists() else ''

    def test_differing_or_malformed_override_before_host_mutation(self):
        for digest in ('', 'a' * 63, 'g' * 64, 'a' * 64, 'a' * 64 + '\n'):
            with self.subTest(digest=digest):
                result, calls = self.run_check(digest)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('VRX_GO_SHA256', result.stderr)
                self.assertEqual(calls, '')

    def test_valid_configuration_uses_repository_pins(self):
        result, calls = self.run_check(None, ['--check-config'])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, '')
        self.assertEqual(result.stdout.strip(),
                         'Go=1.26.0 protoc-gen-go=v1.36.12 protoc-gen-go-grpc=v1.6.2 govpp=v0.13.0')
        ci = (ROOT / '.github/workflows/ci.yml').read_text()
        module = (ROOT / 'apps/agent/go.mod').read_text()
        self.assertIn("go-version: '1.26.0'", ci)
        self.assertIn('protoc-gen-go@v1.36.12', ci)
        self.assertIn('protoc-gen-go-grpc@v1.6.2', ci)
        self.assertIn('go.fd.io/govpp v0.13.0', module)
        source = (ROOT / 'scripts/20-install-build.sh').read_text()
        self.assertNotIn('@latest', source)
        self.assertLess(source.index('sha256sum --check'), source.index('rm -rf /usr/local/go'))

    def test_invalid_cli_refuses_host_mutation(self):
        result, calls = self.run_check(None, ['--unknown'])
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('usage:', result.stderr)
        self.assertEqual(calls, '')


if __name__ == '__main__':
    unittest.main(verbosity=2)
