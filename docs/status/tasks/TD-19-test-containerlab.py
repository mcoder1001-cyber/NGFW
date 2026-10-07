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

import importlib.util
spec = importlib.util.spec_from_file_location('safe_fixture', ROOT / 'scripts/tests/td19-safe-root-fixtures.py')
safe_fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(safe_fixture)


class NativeLab(unittest.TestCase):
    def run_entry(self, args=(), fail_at='install'):
        f = safe_fixture.Fixture()
        try:
            f.env['FAIL_AT'] = fail_at
            result = f.run('40-install-lab.sh', args)
            return result, f.calls()
        finally:
            f.close()

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
