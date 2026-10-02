#!/usr/bin/env python3
"""Execute actual cmd_provision with every remote operation replaced by a recorder."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]


class ProvisionOrder(unittest.TestCase):
    def run_fixture(self, verifier_status, confirmed=True):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'scripts').mkdir()
            preflight = root / 'scripts/00-add-repos.sh'
            preflight.write_text('''#!/bin/sh
[ "$1" = --check-artifacts ] || exit 90
printf 'preflight\\n' >> "$CALL_LOG"
[ "$VERIFY_STATUS" = 0 ] || exit "$VERIFY_STATUS"
printf '{"version":"26.06-release+vrx1","packages":[{"file":"a.deb"},{"file":"b.deb"},{"file":"c.deb"},{"file":"d.deb"},{"file":"e.deb"},{"file":"f.deb"},{"file":"g.deb"}]}\\n'
''')
            preflight.chmod(0o755)
            text = (ROOT / 'tools/lab').read_text()
            start = text.index('cmd_provision() {')
            end = text.index('\n# ---------------------------------------------------------------- restart', start)
            command = text[start:end]
            harness = '''set -euo pipefail
SSH_OPTS=()
c_yel= c_off= c_grn=
is_local(){ return 1; }
is_planned(){ return 1; }
inv_file(){ return 0; }
inv_get(){ case "$2" in role) echo vrx;; mgmt) echo 192.0.2.1;; vpp.source) echo "$ROOT/artifacts";; *) echo "${3:-fixture}";; esac; }
remote_ip(){ echo 192.0.2.1; }
inv_nics(){ :; }
render_startup_conf(){ echo 'dpdk { no-pci }'; }
hdr(){ :; }
warn(){ :; }
die(){ echo "$*" >&2; exit 1; }
run_on(){ printf 'remote:%s\\n' "$*" >> "$CALL_LOG"; return 29; }
scp(){ printf 'scp\\n' >> "$CALL_LOG"; return 99; }
ssh(){ printf 'ssh\\n' >> "$CALL_LOG"; return 99; }
''' + command + '\ncmd_provision vrx-b --apply\n'
            env = dict(os.environ, ROOT=str(root), CALL_LOG=str(root / 'calls'),
                       VERIFY_STATUS=str(verifier_status), VRX_LAB_REMOTE_APPLY='1' if confirmed else '0')
            result = subprocess.run(['bash', '-c', harness], env=env, capture_output=True)
            log = (root / 'calls').read_text().splitlines() if (root / 'calls').exists() else []
            self.assertNotEqual(result.returncode, 0)
            return result, log

    def test_local_artifact_failure_prevents_all_remote_operations(self):
        result, log = self.run_fixture(17)
        self.assertEqual(result.returncode, 17)
        self.assertEqual(log, ['preflight'])

    def test_unconfirmed_apply_prevents_preflight_and_remote_operations(self):
        _, log = self.run_fixture(0, confirmed=False)
        self.assertEqual(log, [])

    def test_remote_os_refusal_precedes_any_staging_or_host_mutation(self):
        result, log = self.run_fixture(0)
        self.assertEqual(result.returncode, 29)
        self.assertEqual(len(log), 2)
        self.assertEqual(log[0], 'preflight')
        self.assertIn('/etc/os-release', log[1])
        self.assertNotIn('sysctl', '\n'.join(log))
        self.assertNotIn('scp', log)


if __name__ == '__main__':
    unittest.main(verbosity=2)
