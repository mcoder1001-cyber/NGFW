#!/usr/bin/env python3
"""Execute existing production native-IPsec checks on a disposable VPP only."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[3]


def fixture_available(namespaces, links):
    # The existing packet fixture uses fixed w8 names and table/interface IDs.
    # Refuse collisions rather than silently sharing another worker's objects.
    if any('w8' in line for line in namespaces.splitlines()):
        raise ValueError('slot8 namespace in use')
    if any('w8nwan' in line or 'w8nlan' in line for line in links.splitlines()):
        raise ValueError('slot8 veth in use')


def lab_environment(text):
    result = {}
    for line in text.splitlines():
        if line.startswith('export '):
            line = line[7:]
        name, separator, value = line.partition('=')
        if not separator or not name.startswith('NGFW_') or '\x00' in value:
            raise ValueError('unexpected slot environment')
        result[name] = value
    if result.get('NGFW_TEST_PREFIX') != 'w8':
        raise ValueError('native fixture requires reserved slot8')
    return result


def shared_identity():
    result = subprocess.run(['systemctl', 'show', 'vpp.service', '-p', 'MainPID', '-p', 'NRestarts'],
                            check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    return dict(line.split('=', 1) for line in result.stdout.splitlines() if '=' in line)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--peer-loss', action='store_true', help='also exercise default liveness and peer recovery')
    parser.add_argument('--skip-build', action='store_true', help='use explicit existing reviewed plugin and freshly built agent')
    parser.add_argument('--plugin-directory', type=Path)
    parser.add_argument('--agent', type=Path)
    args = parser.parse_args()
    if os.environ.get('NGFW_INTEGRATION') != '1':
        parser.exit(2, 'requires NGFW_INTEGRATION=1; this is not a unit or shared-VPP run\n')
    if os.geteuid() != 0:
        parser.exit(2, 'requires root for private namespaces\n')
    # Serialize the fixed fixture and refuse existing slot8 resources. This is
    # not permission to alter resources owned by another test.
    import fcntl
    with open('/run/lock/ngfw-p11-host.lock', 'a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        lab_lock = open('/run/lock/ngfw-lab.lock', 'a')
        fcntl.flock(lab_lock, fcntl.LOCK_SH)
        fixture_available(subprocess.check_output(['ip', 'netns', 'list'], text=True),
                          subprocess.check_output(['ip', '-o', 'link', 'show'], text=True))
        before = shared_identity()
        lab = lab_environment(subprocess.check_output([str(ROOT / 'tools/lab'), 'env', '8'], text=True))
        stamp = time.strftime('%Y%m%d-%H%M%S', time.gmtime()) + '-' + str(os.getpid())
        work = ROOT / '.scratch' / ('P11-host-' + stamp)
        work.mkdir(mode=0o700, parents=True)
        env = dict(os.environ, **lab, NGFW_LAB_LOCK_HELD='1')
        events = []

        def run(phase, argv, extra=None):
            log = work / (phase + '.log')
            with log.open('xb') as output:
                log.chmod(0o600)
                result = subprocess.run(argv, cwd=ROOT, env={**env, **(extra or {})},
                                        stdout=output, stderr=subprocess.STDOUT)
            events.append({'phase': phase, 'exit_code': result.returncode})
            if result.returncode:
                raise RuntimeError(phase + ' failed; private diagnostic log retained')
            return log

        try:
            if args.skip_build:
                if args.plugin_directory is None or args.agent is None:
                    raise ValueError('--skip-build requires plugin-directory and agent')
                plugin = args.plugin_directory.resolve(strict=True)
                agent = args.agent.resolve(strict=True)
            else:
                if args.plugin_directory or args.agent:
                    raise ValueError('explicit binaries require --skip-build')
                plugin = work / 'native-plugin/plugin'
                agent = work / 'ngfw-agent'
                run('build-plugin', [str(ROOT / 'tools/heavy.sh'), 'python3',
                                     'test/topology/ipsec/build-native-plugin.py', '--output', str(plugin.parent)])
                run('build-agent', [str(ROOT / 'tools/heavy.sh'), 'go', '-C', 'apps/agent',
                                    'build', '-o', str(agent), './cmd/ngfw-agent'])
            if not (plugin / 'ikev2_plugin.so').is_file() or not agent.is_file() or not os.access(agent, os.X_OK):
                raise ValueError('reviewed plugin and executable product agent required')
            for initiator in [False, True]:
                phase = 'initiator' if initiator else 'responder'
                log = run(phase, [str(ROOT / 'tools/heavy.sh'), 'python3',
                                  'test/topology/hardware-smoke/isolated-vpp.py', 'go', '-C', 'apps/agent',
                                  'test', './internal/desired', '-run', '^TestIKEv2NativePackets$',
                                  '-v', '-count=1', '-timeout', '4m'], {
                    'NGFW_NATIVE_INITIATOR': '1' if initiator else '0',
                    'NGFW_NATIVE_PEER_LOSS': '1' if args.peer_loss else '0',
                    'NGFW_NATIVE_FAST_DPD': '0',
                    'NGFW_NATIVE_AGENT_BIN': str(agent),
                    'NGFW_NATIVE_EVIDENCE_DIR': str(work / 'evidence'),
                    'NGFW_ISOLATED_PLUGIN_PATH': str(plugin) + ':/usr/lib/x86_64-linux-gnu/vpp_plugins',
                })
                text = log.read_text(errors='replace')
                if '--- PASS: TestIKEv2NativePackets' not in text or '--- SKIP:' in text:
                    raise RuntimeError('packet test did not execute and pass')
                required = ['plaintext IPIP absent', 'route withdrawal and recovery passed',
                            'production agent restart', 'rekey changed CHILD SPI', 'production owned rollback removed']
                if any(marker not in text for marker in required):
                    raise RuntimeError('required production lifecycle evidence absent')
            fixture_available(subprocess.check_output(['ip', 'netns', 'list'], text=True),
                              subprocess.check_output(['ip', '-o', 'link', 'show'], text=True))
            if shared_identity() != before:
                raise RuntimeError('shared VPP identity changed')
            events.append({'phase': 'shared-VPP-unchanged-and-slot-clean', 'exit_code': 0})
        except (OSError, ValueError, RuntimeError, subprocess.CalledProcessError) as error:
            events.append({'phase': 'acceptance', 'error': str(error)})
            result = 1
        else:
            result = 0
        summary = {'source_sha': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
                   'passed': result == 0, 'peer_loss_requested': args.peer_loss,
                   'shared_before': before, 'shared_after': shared_identity(), 'events': events,
                   'captures': {p.name: hashlib.sha256(p.read_bytes()).hexdigest()
                                for p in (work / 'evidence').glob('*.pcap')}}
        (work / 'summary.json').write_text(json.dumps(summary, indent=2) + '\n')
        print(json.dumps({'passed': result == 0, 'summary': str(work / 'summary.json')}))
        # No raw peer/API/SA log is printed or committed by this driver.
        return result


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print('Preflight failed: ' + str(error), file=sys.stderr)
        sys.exit(2)
