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
            for fail in [False, True, "crash"]:
                with self.subTest(policy_kind=policy_kind, fail=fail), tempfile.TemporaryDirectory() as directory:
                    root = pathlib.Path(directory)
                    for path in ['scripts', 'deploy/vpp', 'artifacts', 'bin', 'run/lock', 'usr/sbin']:
                        (root / path).mkdir(parents=True)
                    script = (ROOT / 'scripts/10-install-runtime.sh').read_text()
                    script = script.replace('[[ $EUID -eq 0 ]]', '[[ 1 -eq 1 ]]', 1)
                    for path in ['/usr/sbin/policy-rc.d', '/run/lock/ngfw-runtime-install.lock', '/usr/sbin/.ngfw-runtime-policy-recovery']:
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
[ "$APT_FIXTURE_FAIL" != crash ] || { kill -KILL "$PPID"; exit 18; }
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
                                       NGFW_INSTALL_APPLIANCE='1', NGFW_VPP_ARTIFACTS=str(root / 'artifacts'),
                                       POLICY_FIXTURE=str(policy), APT_FIXTURE_LOG=str(root / 'apt-log'),
                                       APT_FIXTURE_FAIL='crash' if fail == 'crash' else ('1' if fail else '0'))
                    result = subprocess.run(['bash', str(root / 'scripts/runtime.sh')],
                                            env=environment, capture_output=True)
                    if fail == 'crash':
                        self.assertEqual(result.returncode, -9, result.stderr.decode())
                        recovery = root / 'usr/sbin/.ngfw-runtime-policy-recovery'
                        self.assertTrue(recovery.is_dir())
                        self.assertEqual(recovery.stat().st_mode & 0o777, 0o700)
                        self.assertIn(b'exit 101', policy.read_bytes())
                        if policy_kind == 'file':
                            self.assertEqual((recovery / 'original').read_bytes(), original)
                        elif policy_kind == 'symlink':
                            self.assertEqual(os.readlink(recovery / 'original'), 'operator-policy')
                        log = (root / 'apt-log').read_bytes()
                        retry = subprocess.run(['bash', str(root / 'scripts/runtime.sh')],
                                               env=environment, capture_output=True)
                        self.assertNotEqual(retry.returncode, 0)
                        self.assertIn(b'recovery pending', retry.stderr)
                        self.assertEqual((root / 'apt-log').read_bytes(), log)
                        continue
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
            environment.pop('NGFW_VPP_ARTIFACTS', None)
            for explicit in ['0', '1']:
                environment['NGFW_INSTALL_APPLIANCE'] = explicit
                result = subprocess.run(['bash', str(ROOT / 'scripts/10-install-runtime.sh')],
                                        env=environment, capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse((root / 'called').exists())


if __name__ == '__main__':
    unittest.main()
