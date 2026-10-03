#!/usr/bin/env python3
"""Exercise real installer boundaries with commands redirected to temporary files."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[4]


class PolicyBoundaries(unittest.TestCase):
    def run_fixture(self, case):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in ['scripts', 'deploy/vpp', 'artifacts', 'bin', 'run/lock', 'usr/sbin']:
                (root / name).mkdir(parents=True)
            # Only host paths/root precondition are redirected; production logic
            # and command ordering are otherwise unchanged.
            source = (ROOT / 'scripts/10-install-runtime.sh').read_text()
            source = source.replace('[[ $EUID -eq 0 ]]', '[[ 1 -eq 1 ]]', 1)
            for name in ['/usr/sbin/policy-rc.d', '/run/lock/ngfw-runtime-install.lock',
                         '/usr/sbin/.ngfw-runtime-policy-recovery']:
                source = source.replace(name, str(root) + name)
            script = root / 'scripts/runtime.sh'
            script.write_text(source)

            def executable(name, body):
                path = root / name
                path.write_text('#!/bin/sh\n' + body)
                path.chmod(0o755)

            executable('deploy/vpp/verify.sh', 'exit 0\n')
            executable('bin/systemctl', 'exit 0\n')
            executable('bin/apt-get', '''printf 'APT called\\n' >> "$CALL_LOG"
if [ "$CASE" = concurrent ]; then
  printf '#!/bin/sh\\nexit 77\\n' > "$POLICY_TARGET.new"
  /usr/bin/mv -T "$POLICY_TARGET.new" "$POLICY_TARGET"
fi
exit 0
''')
            executable('bin/mv', '''if [ "$CASE" = rename ]; then
  printf 'rename failed\\n' >> "$RENAME_LOG"
  exit 23
fi
exec /usr/bin/mv "$@"
''')
            (root / 'artifacts/manifest.json').write_text(json.dumps({'packages': [
                {'ship': True, 'file': f'fixture-{index}.deb'} for index in range(7)]}))
            policy = root / 'usr/sbin/policy-rc.d'
            original = b'#!/bin/sh\nexit 0\n'
            policy.write_bytes(original)
            policy.chmod(0o711)
            metadata = policy.stat()
            recovery = root / 'usr/sbin/.ngfw-runtime-policy-recovery'
            if case == 'stale':
                recovery.mkdir(mode=0o700)
                (recovery / 'original').write_bytes(b'older recovery evidence')
            env = dict(os.environ, PATH=str(root / 'bin') + ':' + os.environ['PATH'],
                       NGFW_INSTALL_APPLIANCE='1', NGFW_VPP_ARTIFACTS=str(root / 'artifacts'),
                       CASE=case, CALL_LOG=str(root / 'apt-log'), POLICY_TARGET=str(policy),
                       RENAME_LOG=str(root / 'rename-log'))
            result = subprocess.run(['bash', str(script)], env=env, capture_output=True)
            self.assertNotEqual(result.returncode, 0, result.stderr.decode())
            self.assertTrue(recovery.is_dir())
            self.assertEqual(recovery.stat().st_mode & 0o777, 0o700)
            if case == 'concurrent':
                self.assertTrue((root / 'apt-log').exists())
                self.assertEqual(policy.read_bytes(), b'#!/bin/sh\nexit 77\n')
                self.assertEqual((recovery / 'original').read_bytes(), original)
                self.assertIn(b'policy changed', result.stderr)
            else:
                self.assertFalse((root / 'apt-log').exists())
                self.assertEqual(policy.read_bytes(), original)
                self.assertEqual(policy.stat().st_mode, metadata.st_mode)
                if case == 'rename':
                    self.assertTrue((root / 'rename-log').exists())
                    self.assertEqual((recovery / 'original').read_bytes(), original)
                    self.assertIn(b'policy changed', result.stderr)
                else:
                    self.assertFalse((root / 'rename-log').exists())
                    self.assertEqual((recovery / 'original').read_bytes(), b'older recovery evidence')
                    self.assertIn(b'recovery pending', result.stderr)

    def test_failed_guard_rename_preserves_original_and_recovery_without_apt(self):
        self.run_fixture('rename')

    def test_concurrent_policy_replacement_is_never_overwritten(self):
        self.run_fixture('concurrent')

    def test_stale_prepublication_recovery_refuses_mutation(self):
        self.run_fixture('stale')


if __name__ == '__main__':
    unittest.main()
