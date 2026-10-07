#!/usr/bin/env python3
"""Evidence-only observer for the single manager-authorized original P12 run."""
import fcntl
import json
import os
from pathlib import Path
import selectors
import shlex
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[4]
OUT = Path(__file__).resolve().parent
EXPECTED = {
    'NGFW_SLOT': '14', 'NGFW_TEST_PREFIX': 'w14',
    'NGFW_HTTP_PORT': '11400', 'NGFW_WEB_PORT': '15400',
    'NGFW_METRICS_PORT': '9241', 'NGFW_AGENT_SOCKET': '/run/ngfw-test/w14/agent.sock',
    'NGFW_PG_DATABASE': 'ngfw_w14', 'NGFW_VALKEY_DB': '14',
    'NGFW_VPP_TABLE_BASE': '14000', 'NGFW_LAB_LOCK': '/run/lock/ngfw-lab.lock',
    'NGFW_VPP_API_SOCKET': '/run/vpp/api.sock',
    'NGFW_VPP_CLI_SOCKET': '/run/vpp/cli.sock',
    'NGFW_VPP_STATS_SOCKET': '/run/vpp/stats.sock',
    'NGFW_AGENT_VPP_API_SOCKET': '/run/vpp/api.sock',
    'NGFW_AGENT_VPP_STATS_SOCKET': '/run/vpp/stats.sock',
    'NGFW_VPPCTL': 'vppctl -s /run/vpp/cli.sock',
}


def capture():
    scratch = ROOT / '.scratch'
    # Only this review worktree's newly created disposable runtimes; capture logs
    # before the unchanged fixtures delete them. No configs/secrets are copied.
    for runtime in scratch.glob('isolated-vpp-*'):
        sources = list(runtime.glob('*.log')) + list(runtime.glob('test-run/w14/**/*.log'))
        for source in sources:
            relative = source.relative_to(runtime)
            target = OUT / 'startup' / runtime.name / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            result = subprocess.run(['cp', '--', str(source), str(target)],
                                    stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
            if result.returncode and source.exists():
                print('OBSERVER_COPY_ERROR=' + result.stderr.decode().strip(), flush=True)


def main():
    if (OUT / 'single-attempt-started').exists():
        raise SystemExit('single-attempt evidence already exists; refusing another run')
    exports = subprocess.check_output([str(ROOT / 'tools/lab'), 'env', '14'], text=True)
    values = {}
    for line in exports.splitlines():
        if not line or line.startswith('#'):
            continue
        tokens = shlex.split(line)
        if len(tokens) != 2 or tokens[0] != 'export' or '=' not in tokens[1]:
            raise SystemExit('invalid tools/lab env export')
        key, value = tokens[1].split('=', 1)
        if key in values:
            raise SystemExit('duplicate slot export')
        values[key] = value
    if values != EXPECTED:
        raise SystemExit('slot exports do not exactly match validated slot14 fields')
    # Do not inherit Wave-B/REST, socket overrides, plugin/worker overrides or
    # unrelated integration flags. Preserve only normal process/build environment.
    env = {key: value for key, value in os.environ.items() if key in {
        'PATH', 'HOME', 'USER', 'LOGNAME', 'LANG', 'LC_ALL', 'TMPDIR',
        'GOCACHE', 'GOMODCACHE', 'GOTOOLCHAIN',
    }}
    env.update(values, PYTHONUNBUFFERED='1')
    for name in ('ngfw-acceptance-slot14.lock',):
        with open('/run/lock/' + name, 'r') as probe:
            fcntl.flock(probe, fcntl.LOCK_EX | fcntl.LOCK_NB)
            print('PRECONDITION_FREE=' + name, flush=True)
    for path in Path('/run/ngfw-test/w14').glob('frr*.lock'):
        with path.open('r') as probe:
            fcntl.flock(probe, fcntl.LOCK_EX | fcntl.LOCK_NB)
            print('PRECONDITION_FREE=' + str(path), flush=True)
    with open('/run/lock/ngfw-lab.lock', 'r') as lab:
        fcntl.flock(lab, fcntl.LOCK_SH | fcntl.LOCK_NB)
        print('PRECONDITION_SHARED_LAB_LOCK=ACQUIRED', flush=True)
        print('VALIDATED_SLOT=' + json.dumps(values, sort_keys=True), flush=True)
        print('LIVE_START_UTC=' + time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()), flush=True)
        started = time.monotonic()
        (OUT / 'single-attempt-started').mkdir()
        child = subprocess.Popen([str(ROOT / 'test/topology/frr-linuxcp/run-fib-root.sh')],
                                 cwd=ROOT, env=env, stdout=subprocess.PIPE,
                                 stderr=subprocess.STDOUT)
        print('OBSERVER_ORIGINAL_RUNNER_PID=' + str(child.pid), flush=True)
        selector = selectors.DefaultSelector()
        selector.register(child.stdout, selectors.EVENT_READ)
        try:
            while selector.get_map():
                for key, _ in selector.select(timeout=.2):
                    block = os.read(key.fd, 65536)
                    if block:
                        sys.stdout.buffer.write(block)
                        sys.stdout.buffer.flush()
                    else:
                        selector.unregister(key.fileobj)
                capture()
            result = child.wait()
            capture()
        finally:
            selector.close()
            child.stdout.close()
        print('ORIGINAL_RUN_EXIT=' + str(result), flush=True)
        print('ORIGINAL_RUN_SECONDS=' + str(round(time.monotonic() - started, 3)), flush=True)
        print('LIVE_END_UTC=' + time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()), flush=True)
    print('OBSERVER_SHARED_LAB_LOCK=RELEASED', flush=True)
    raise SystemExit(result)


if __name__ == '__main__':
    main()
