#!/usr/bin/env python3
"""Native lab regression; legacy filename keeps the fixed strict runner coverage.

Forbidden containerlab installer tests are replaced with actual entry-point APT
plan/refusal checks. No package operation or network download succeeds; the
recording APT stub stops execution before any system venv writes.
"""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]


class NativeLab(unittest.TestCase):
    def run_entry(self, args=(), fail_at='install'):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            log = root / 'calls'
            recorder = '''#!/usr/bin/python3
import json, os, pathlib, sys
name = pathlib.Path(sys.argv[0]).name
with open(os.environ['CALL_LOG'], 'a') as stream:
    stream.write(json.dumps([name, *sys.argv[1:]]) + '\\n')
raise SystemExit(42 if name == 'apt-get' and sys.argv[1] == os.environ['FAIL_AT'] else
                 0 if name == 'apt-get' and sys.argv[1] == 'update' else 91)
'''
            for command in ('apt-get', 'curl', 'dpkg-deb', 'dpkg-query', 'sha256sum',
                            'mktemp', 'python3', 'pip', 'systemctl', 'go', 'tar', 'rm'):
                stub = root / command
                stub.write_text(recorder)
                stub.chmod(0o755)
            env = dict(os.environ, PATH=f'{root}:/usr/bin:/bin', CALL_LOG=str(log), FAIL_AT=fail_at)
            # Only UID gate is bypassed for unprivileged hosted fixture runners.
            source = (ROOT / 'scripts/40-install-lab.sh').read_text()
            entry = root / 'entry.sh'
            entry.write_text(source.replace('[[ $EUID -eq 0 ]]', '[[ 1 -eq 1 ]]', 1))
            result = subprocess.run(['bash', str(entry), *args],
                                    env=env, text=True, capture_output=True, timeout=10)
            calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
            return result, calls

    def test_native_package_plan_without_download_or_virtualization(self):
        result, calls = self.run_entry()
        self.assertEqual(result.returncode, 42, result.stderr)
        self.assertEqual(calls, [
            ['apt-get', 'update'],
            ['apt-get', 'install', '-y', 'iperf3', 'netperf', 'tshark', 'tcpdump',
             'python3-venv', 'python3-pip', 'git', 'curl', 'jq', 'frr'],
        ])

    def test_update_failure_never_reaches_install_or_venv(self):
        result, calls = self.run_entry(fail_at='update')
        self.assertEqual(result.returncode, 42, result.stderr)
        self.assertEqual(calls, [['apt-get', 'update']])

    def test_check_config_has_no_package_or_download_commands(self):
        result, calls = self.run_entry(['--check-config'])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [])
        self.assertEqual(result.stdout.strip(), 'lab=vmware native-tools=iperf3,netperf,tshark,tcpdump,frr')

    def test_unknown_or_extra_arguments_refuse_before_package_commands(self):
        for args in (['--unknown'], ['--check-config', '--extra'], ['--apply']):
            with self.subTest(args=args):
                result, calls = self.run_entry(args)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('usage:', result.stderr)
                self.assertEqual(calls, [])

    def test_forbidden_installer_source_is_absent(self):
        source = (ROOT / 'scripts/40-install-lab.sh').read_text()
        for forbidden in ('containerlab', 'docker.io', 'docker-compose', 'qemu-kvm', 'libvirt',
                          'virtinst', 'bridge-utils', 'ovmf', 'CONTAINERLAB', 'VIRT='):
            with self.subTest(forbidden=forbidden):
                self.assertFalse(forbidden in source, f'forbidden installer component: {forbidden}')


if __name__ == '__main__':
    unittest.main(verbosity=2)
