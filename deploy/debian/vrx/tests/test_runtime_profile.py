#!/usr/bin/env python3
"""Runtime installer preflight refusal; never call package management."""
import json
import os
import pathlib
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[4]


class RuntimeProfile(unittest.TestCase):
    def test_policy_guard_denies_activation_and_restores_existing_policy(self):
        for policy_kind in ['missing', 'file', 'symlink']:
            for fail in [False, True]:
                with self.subTest(policy_kind=policy_kind, fail=fail), tempfile.TemporaryDirectory() as directory:
                    root = pathlib.Path(directory)
                    for path in ['scripts', 'deploy/vpp', 'artifacts', 'bin', 'run/lock', 'usr/sbin']:
                        (root / path).mkdir(parents=True)
                    script = (ROOT / 'scripts/10-install-runtime.sh').read_text()
                    script = script.replace('[[ $EUID -eq 0 ]]', '[[ 1 -eq 1 ]]', 1)
                    for path in ['/usr/sbin/policy-rc.d', '/run/lock/vrx-runtime-install.lock', '/run/vrx-runtime-policy.']:
                        script = script.replace(path, str(root) + path)
                    (root / 'scripts/runtime.sh').write_text(script)
                    def executable(path, content):
                        file = root / path
                        file.write_text('#!/bin/sh\n' + content)
                        file.chmod(0o755)
                    executable('deploy/vpp/verify.sh', 'exit 0\n')
                    executable('bin/systemctl', 'exit 0\n')
                    executable('bin/apt-get', '''set +e
"$POLICY_FIXTURE" test-service start
status=$?
[ "$status" = 101 ] || exit 90
printf 'activation denied\\n' >> "$APT_FIXTURE_LOG"
[ "$APT_FIXTURE_FAIL" != 1 ] || exit 17
exit 0
''')
                    (root / 'artifacts/manifest.json').write_text(json.dumps(dict(packages=[
                        dict(ship=True, file=f'fixture-{index}.deb') for index in range(7)])))
                    policy = root / 'usr/sbin/policy-rc.d'
                    original = b'#!/bin/sh\nexit 0\n'
                    if policy_kind == 'file':
                        policy.write_bytes(original)
                        policy.chmod(0o711)
                        metadata = policy.stat()
                    elif policy_kind == 'symlink':
                        (root / 'usr/sbin/operator-policy').write_bytes(original)
                        policy.symlink_to('operator-policy')
                    environment = dict(os.environ, PATH=str(root / 'bin') + ':' + os.environ['PATH'],
                                       VRX_INSTALL_APPLIANCE='1', VRX_VPP_ARTIFACTS=str(root / 'artifacts'),
                                       POLICY_FIXTURE=str(policy), APT_FIXTURE_LOG=str(root / 'apt-log'),
                                       APT_FIXTURE_FAIL='1' if fail else '0')
                    result = subprocess.run(['bash', str(root / 'scripts/runtime.sh')],
                                            env=environment, capture_output=True)
                    self.assertEqual(result.returncode, 17 if fail else 0, result.stderr.decode())
                    self.assertIn('activation denied', (root / 'apt-log').read_text())
                    if policy_kind == 'missing': self.assertFalse(policy.exists())
                    elif policy_kind == 'file':
                        self.assertEqual(policy.read_bytes(), original)
                        self.assertEqual(policy.stat().st_mode, metadata.st_mode)
                        self.assertEqual(policy.stat().st_uid, metadata.st_uid)
                        self.assertEqual(policy.stat().st_gid, metadata.st_gid)
                    else:
                        self.assertTrue(policy.is_symlink())
                        self.assertEqual(os.readlink(policy), 'operator-policy')

    def test_explicit_appliance_and_artifact_inputs_required_before_apt(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            command = root / 'apt-get'
            command.write_text('#!/bin/sh\ntouch "$APT_FIXTURE_CALLED"\nexit 99\n')
            command.chmod(0o755)
            environment = dict(os.environ, PATH=str(root) + ':' + os.environ['PATH'],
                               APT_FIXTURE_CALLED=str(root / 'called'))
            environment.pop('VRX_VPP_ARTIFACTS', None)
            for explicit in ['0', '1']:
                environment['VRX_INSTALL_APPLIANCE'] = explicit
                result = subprocess.run(['bash', str(ROOT / 'scripts/10-install-runtime.sh')],
                                        env=environment, capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse((root / 'called').exists())


if __name__ == '__main__':
    unittest.main()
