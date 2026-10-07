"""tools/lab vpp up|down|status and the env socket exports (LAB-vpp-per-slot), against fakes.

systemctl, systemd-run, vppctl, the VPP binary and ngfw-startupgen are stubs on PATH / env overrides; the runtime root, /dev/shm,
/proc/meminfo and the lab lock are temp paths. Nothing here starts a VPP or touches /run/vpp, /run/ngfw-test or /dev/shm.
Run: python3 -m unittest tools/tests/test_lab_vpp.py   (the up/down cases need root, like the tool itself)
"""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

LAB = Path(__file__).resolve().parents[1] / 'lab'
GOLDEN = Path(__file__).resolve().parents[2] / 'apps/agent/internal/renderers/vppstartup/testdata/lab-slot-20.golden'

FAKE_SYSTEMCTL = r'''#!/usr/bin/env bash
# state: $FAKE/units/<unit> exists = active
d="$FAKE/units"; mkdir -p "$d"; echo "systemctl $*" >> "$FAKE/calls"
case "$1" in
  is-active) [[ "$2" == --quiet ]] && shift; u="${2%.service}"; [[ -e "$d/$u" ]] && { echo active; exit 0; }; echo inactive; exit 3 ;;
  list-units) for f in "$d"/*; do [[ -e "$f" ]] && echo "$(basename "$f").service loaded active running fake"; done; exit 0 ;;
  show) u="${2%.service}"; case "$4" in
          ActiveState) [[ -e "$d/$u" ]] && echo active || echo inactive ;;
          MainPID) echo 0 ;;
        esac; exit 0 ;;
  stop) u="${2%.service}"; rm -f "$d/$u"; exit 0 ;;
  reset-failed) exit 0 ;;
esac
exit 0
'''
# systemd-run --quiet --unit=<u> ... <vpp> -c <conf>: mark the unit active and create the instance's sockets
FAKE_SYSTEMD_RUN = r'''#!/usr/bin/env bash
echo "systemd-run $*" >> "$FAKE/calls"
u=""; for a in "$@"; do [[ $a == --unit=* ]] && u="${a#--unit=}"; done
conf="${*: -1}"; dir="$(dirname "$conf")"
mkdir -p "$FAKE/units"; : > "$FAKE/units/$u"
for s in api cli stats; do python3 -c 'import socket,sys; socket.socket(socket.AF_UNIX).bind(sys.argv[1])' "$dir/$s.sock"; done
: > "$NGFW_LAB_SHM_DIR/${u#ngfw-vpp-}-global_vm"; : > "$NGFW_LAB_SHM_DIR/${u#ngfw-vpp-}-vpe-api"
'''
FAKE_VPPCTL = '#!/usr/bin/env bash\necho "vppctl $*" >> "$FAKE/calls"\necho "vpp v26.06-release fake"\n'
# a stand-in for ngfw-startupgen: writes the golden slot rendering with the requested slot/root substituted (or $FAKE_RENDER)
FAKE_STARTUPGEN = r'''#!/usr/bin/env bash
echo "startupgen $*" >> "$FAKE/calls"
slot=""; root=""; out=""
while (( $# )); do case "$1" in --lab-slot) slot=$2; shift ;; --lab-root) root=$2; shift ;; -o) out=$2; shift ;; esac; shift; done
if [[ -n ${FAKE_RENDER:-} ]]; then cp "$FAKE_RENDER" "$out"; exit 0; fi
sed -e "s#/run/ngfw-test/w20/vpp#$root/w$slot/vpp#g" -e "s#prefix w20#prefix w$slot#" "$GOLDEN" > "$out"
'''


@unittest.skipUnless(shutil.which('flock') and shutil.which('python3'), 'needs flock')
class LabVPPTests(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix='lab-vpp-test-'))
        self.addCleanup(shutil.rmtree, self.tmp, True)
        self.bin = self.tmp / 'bin'; self.bin.mkdir()
        for name, body in (('systemctl', FAKE_SYSTEMCTL), ('systemd-run', FAKE_SYSTEMD_RUN), ('vppctl', FAKE_VPPCTL),
                           ('ngfw-startupgen', FAKE_STARTUPGEN), ('vpp', '#!/bin/sh\nexit 0\n')):
            p = self.bin / name; p.write_text(body); p.chmod(0o755)
        (self.tmp / 'root').mkdir(); (self.tmp / 'shm').mkdir()
        self.meminfo(16 * 1024 * 1024)
        self.env = dict(os.environ, PATH=f'{self.bin}:{os.environ["PATH"]}', FAKE=str(self.tmp), GOLDEN=str(GOLDEN),
                        NGFW_LAB_VPP_ROOT=str(self.tmp / 'root'), NGFW_LAB_SHM_DIR=str(self.tmp / 'shm'),
                        NGFW_LAB_MEMINFO=str(self.tmp / 'meminfo'), NGFW_STARTUPGEN=str(self.bin / 'ngfw-startupgen'),
                        NGFW_LAB_VPP_BIN=str(self.bin / 'vpp'), NGFW_LAB_PLUGIN_DIR=str(self.tmp),
                        NGFW_LAB_LOCK=str(self.tmp / 'lab.lock'), NGFW_LAB_VPP_WAIT='5')
        for k in ('NGFW_LAB_VPP_MAX', 'NGFW_LAB_LOCK_HELD', 'NGFW_VPP_CLI_SOCKET', 'FAKE_RENDER'):
            self.env.pop(k, None)

    def meminfo(self, avail_kb):
        (self.tmp / 'meminfo').write_text(f'MemAvailable: {avail_kb} kB\nHugePages_Total: 4096\nHugePages_Free: 4000\n')

    def lab(self, *args, **extra):
        return subprocess.run([str(LAB), *args], env=dict(self.env, **extra), capture_output=True, text=True, check=False)

    def exports(self, slot):
        out = self.lab('env', str(slot))
        self.assertEqual(out.returncode, 0, out.stderr)
        return {k: v.strip('"') for k, v in (line[len('export '):].split('=', 1) for line in out.stdout.splitlines() if line.startswith('export '))}

    def test_env_names_shared_vpp_while_no_instance_runs(self):
        e = self.exports(20)
        self.assertEqual(e['NGFW_VPP_API_SOCKET'], '/run/vpp/api.sock')
        self.assertEqual(e['NGFW_VPP_CLI_SOCKET'], '/run/vpp/cli.sock')
        self.assertEqual(e['NGFW_AGENT_VPP_STATS_SOCKET'], '/run/vpp/stats.sock')
        self.assertEqual(e['NGFW_VPPCTL'], 'vppctl -s /run/vpp/cli.sock')

    def test_slot_numbers_refused(self):
        for bad in ('13', '0', '33', '020', 'x', ''):
            out = self.lab('vpp', 'up', bad) if bad else self.lab('vpp', 'up')
            self.assertNotEqual(out.returncode, 0, bad)
        out = self.lab('vpp', 'up', '13')
        self.assertIn('13 is reserved for tools/app', out.stderr)
        self.assertFalse((self.tmp / 'calls').exists() and 'systemd-run' in (self.tmp / 'calls').read_text())

    @unittest.skipUnless(os.geteuid() == 0, 'tools/lab vpp up/down require root')
    def test_up_env_status_down(self):
        up = self.lab('vpp', 'up', '20')
        self.assertEqual(up.returncode, 0, up.stdout + up.stderr)
        d = self.tmp / 'root/w20/vpp'
        self.assertIn('vpp: active', up.stdout)
        calls = (self.tmp / 'calls').read_text()
        self.assertIn('--unit=ngfw-vpp-w20', calls)
        self.assertIn('MemoryMax=1024M', calls)
        self.assertIn(f'--lab-slot 20 --lab-root {self.tmp / "root"}', calls)
        self.assertNotIn('vpp.service', calls)
        self.assertIn('"dpdk_plugin.so": false', (d / 'dataplane.json').read_text())
        e = self.exports(20)
        self.assertEqual(e['NGFW_VPP_API_SOCKET'], f'{d}/api.sock')
        self.assertEqual(e['NGFW_AGENT_VPP_API_SOCKET'], f'{d}/api.sock')
        self.assertEqual(e['NGFW_VPPCTL'], f'vppctl -s {d}/cli.sock')
        self.assertEqual(self.exports(3)['NGFW_VPP_API_SOCKET'], '/run/vpp/api.sock')   # other slots unaffected
        self.assertIn('already running', self.lab('vpp', 'up', '20').stdout)
        self.assertIn('slot: 20', self.lab('vpp', 'status').stdout)
        down = self.lab('vpp', 'down', '20')
        self.assertEqual(down.returncode, 0, down.stdout + down.stderr)
        self.assertFalse(d.exists())
        self.assertEqual(sorted(os.listdir(self.tmp / 'shm')), [])
        self.assertEqual(self.exports(20)['NGFW_VPP_API_SOCKET'], '/run/vpp/api.sock')

    @unittest.skipUnless(os.geteuid() == 0, 'tools/lab vpp up/down require root')
    def test_down_removes_only_its_own_shm(self):
        for f in ('w2-global_vm', 'w20-vpe-api', 'w2x', 'global_vm'):
            (self.tmp / 'shm' / f).write_text('')
        self.assertEqual(self.lab('vpp', 'down', '2').returncode, 0)
        self.assertEqual(sorted(os.listdir(self.tmp / 'shm')), ['global_vm', 'w20-vpe-api', 'w2x'])

    @unittest.skipUnless(os.geteuid() == 0, 'tools/lab vpp up/down require root')
    def test_up_refusals(self):
        self.assertIn('must be 1..4', self.lab('vpp', 'up', '20', NGFW_LAB_VPP_MAX='5').stderr)
        (self.tmp / 'units').mkdir(exist_ok=True)
        for u in ('ngfw-vpp-w3', 'ngfw-vpp-w4'):
            (self.tmp / 'units' / u).write_text('')
        out = self.lab('vpp', 'up', '20')
        self.assertIn('already run', out.stderr)
        self.assertEqual(self.lab('vpp', 'up', '20', NGFW_LAB_VPP_MAX='3').returncode, 0)   # raised cap
        self.lab('vpp', 'down', '20')
        self.assertEqual(self.lab('vpp', 'up', '12').returncode, 0)                          # the CI slot is not capped
        self.lab('vpp', 'down', '12')
        self.meminfo(8 * 1024 * 1024)
        (self.tmp / 'calls').write_text('')
        out = self.lab('vpp', 'up', '20', NGFW_LAB_VPP_MAX='4')
        self.assertIn('floor', out.stderr)
        self.assertNotIn('systemd-run', (self.tmp / 'calls').read_text())

    @unittest.skipUnless(os.geteuid() == 0, 'tools/lab vpp up/down require root')
    def test_rendering_naming_shared_vpp_is_refused(self):
        for bad in ('unix { runtime-dir /run/ngfw-test/w20/vpp cli-listen /run/vpp/cli.sock }\napi-segment { prefix w20 }\n',
                    'unix {\n  runtime-dir RUNTIME\n}\napi-segment {\n  prefix w20\n}\nsocksvr {\n  default\n}\n'):
            render = self.tmp / 'bad.conf'
            render.write_text(bad.replace('RUNTIME', str(self.tmp / 'root/w20/vpp')))
            out = self.lab('vpp', 'up', '20', FAKE_RENDER=str(render))
            self.assertNotEqual(out.returncode, 0)
            self.assertIn('names the shared VPP', out.stderr)
            self.assertFalse((self.tmp / 'root/w20/vpp').exists())
            self.assertNotIn('systemd-run', (self.tmp / 'calls').read_text())


if __name__ == '__main__':
    unittest.main()
