#!/usr/bin/env python3
"""P12 root-mode acceptance inside an owned outer netns and disposable VPP."""
import fcntl
import json
import os
from pathlib import Path
import subprocess
import sys
import signal
import time

ROOT = Path(__file__).resolve().parents[3]
SELF = str(Path(__file__).resolve())


def snapshot():
    commands = [
        ['systemctl', 'show', 'vpp', '-p', 'MainPID', '-p', 'NRestarts'],
        ['ip', '-4', 'route', 'show'],
        ['vppctl', '-s', '/run/vpp/cli.sock', 'show', 'lcp'],
    ]
    return [subprocess.check_output(command).decode() for command in commands]


def process_identity(pid):
    try:
        stat = Path(f'/proc/{pid}/stat').read_text()
        return (stat[stat.rindex(')') + 1:].split()[19], os.readlink(f'/proc/{pid}/ns/net'))
    except (FileNotFoundError, ProcessLookupError):
        return None


def members(namespaces):
    found = {}
    for entry in Path('/proc').iterdir():
        if entry.name.isdigit() and int(entry.name) != os.getpid():
            identity = process_identity(entry.name)
            if identity and identity[1] in namespaces:
                found[int(entry.name)] = identity
    return found


def private_run(command, runtime, current):
    # Track actual private namespaces while named peers exist; retain their inode
    # identity after handles disappear so surviving daemons cannot escape cleanup.
    namespaces = {current}
    process = subprocess.Popen(command, cwd=ROOT)
    result = 1
    try:
        deadline = time.monotonic() + 1250
        while process.poll() is None:
            for handle in (runtime / 'netns').iterdir():
                namespaces.add('net:[' + str(handle.stat().st_ino) + ']')
            if time.monotonic() >= deadline:
                raise RuntimeError('P12 private runner deadline exceeded')
            time.sleep(.1)
        result = process.returncode
    finally:
        leaked = members(namespaces)
        print('P12_PRIVATE_REMAINING=' + json.dumps(leaked), flush=True)
        # Each signal targets one observed PID, rechecking immutable starttime and
        # membership. The host namespace can never enter this set.
        for sig in [signal.SIGTERM, signal.SIGKILL]:
            for pid, identity in leaked.items():
                if (identity[1] != os.environ['NGFW_P12_HOST_NETNS']
                        and process_identity(pid) == identity):
                    try:
                        os.kill(pid, sig)
                    except ProcessLookupError:
                        pass
            until = time.monotonic() + 5
            while members(namespaces) and time.monotonic() < until:
                time.sleep(.1)
            if not members(namespaces):
                break
        process.wait(timeout=10)
        remaining = members(namespaces)
        print('P12_PRIVATE_PROCESSES_AFTER=' + json.dumps(remaining), flush=True)
        if leaked or remaining:
            result = result or 1
    return result


def main():
    args = sys.argv[1:]
    mode = args.pop(0) if args and args[0] in {'--child', '--test'} else ''
    if not mode:
        # Dedicated slot and full lab environment must already be manager assigned.
        if os.environ.get('NGFW_SLOT') != '14' or os.environ.get('NGFW_TEST_PREFIX') != 'w14':
            raise SystemExit('P12 recovery requires assigned slot 14 / w14')
        with open('/run/lock/ngfw-acceptance-slot14.lock', 'a') as slot_lock:
            fcntl.flock(slot_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            print('P12_SLOT14_EXCLUSIVE=ACQUIRED', flush=True)
            before = snapshot()
            env = dict(os.environ, NGFW_P12_HOST_NETNS=os.readlink('/proc/self/ns/net'))
            print('P12_SHARED_BEFORE=' + json.dumps(before), flush=True)
            try:
                result = subprocess.call(['unshare', '--net', '--mount', '--propagation', 'private',
                                          sys.executable, SELF, '--child', *args], env=env)
            finally:
                after = snapshot()
                print('P12_SHARED_AFTER=' + json.dumps(after), flush=True)
                if before != after:
                    raise SystemExit('P12 shared VPP/LCP/root routes changed')
            raise SystemExit(result)
    current = os.readlink('/proc/self/ns/net')
    if current in {os.environ.get('NGFW_P12_HOST_NETNS'), os.readlink('/proc/1/ns/net')}:
        raise SystemExit('refusing P12 in shared host root namespace')
    if mode == '--test':
        # Read the actual bind mount source, rather than trusting a caller's socket flag.
        mounts = [line.split() for line in Path('/proc/self/mountinfo').read_text().splitlines()]
        sources = [row[3] for row in mounts if row[4] == '/run/vpp']
        if len(sources) != 1 or '\\' in sources[0]:
            raise SystemExit('P12 private VPP bind mount ownership missing')
        owned = Path(sources[0]) / 'api.sock'
        scratch = ROOT / '.scratch'
        if not owned.is_relative_to(scratch) or not os.path.samefile(owned, '/run/vpp/api.sock'):
            raise SystemExit('P12 API does not match owned disposable VPP mount')
        # Our parent is the isolated launcher; its actual VPP child is our sibling.
        parent = os.getppid()
        children = Path(f'/proc/{parent}/task/{parent}/children').read_text().split()
        candidates = []
        for pid in children:
            try:
                if (Path(os.readlink(f'/proc/{pid}/exe')).name == 'vpp'
                        and os.readlink(f'/proc/{pid}/ns/net') == current):
                    candidates.append(pid)
            except FileNotFoundError:
                continue
        if len(candidates) != 1:
            raise SystemExit('P12 owned disposable VPP process missing or ambiguous')
        pid = candidates[0]
        stat = Path(f'/proc/{pid}/stat').read_text()
        start = stat[stat.rindex(')') + 1:].split()[19]
        os.environ.update(NGFW_P12_PRIVATE_API=str(owned), NGFW_P12_FIB='root',
                          NGFW_P12_PRIVATE_VPP_PID=pid, NGFW_P12_PRIVATE_VPP_START=start)
        print('P12_PRIVATE_VPP=' + json.dumps({'pid': pid, 'starttime': start,
                                              'netns': current}), flush=True)
        print('P12_PRIVATE_API=' + str(owned), flush=True)
        raise SystemExit(subprocess.call([str(ROOT / 'test/topology/bgp/run.sh'), *args], cwd=ROOT))
    runtime = ROOT / '.scratch' / ('p12-outer-' + str(os.getpid()))
    for name in ['netns', 'frr']:
        source = runtime / name
        source.mkdir(parents=True, mode=0o755)
        subprocess.run(['mount', '--bind', str(source), '/run/' + name], check=True)
    subprocess.run(['ip', 'link', 'set', 'lo', 'up'], check=True)
    links = json.loads(subprocess.check_output(['ip', '-j', 'link', 'show']))
    if any(link['ifname'] not in {'lo', 'ip6tnl0'} for link in links):
        raise SystemExit('P12 outer network namespace is not empty')
    os.environ['NGFW_ISOLATED_TEST_RUN'] = '1'
    print('P12_PRIVATE_NETNS=' + current, flush=True)
    result = private_run([sys.executable,
                              str(ROOT / 'test/topology/hardware-smoke/isolated-vpp.py'),
                              sys.executable, SELF, '--test', *args], runtime, current)
    if list((runtime / 'netns').iterdir()) or list((runtime / 'frr').iterdir()):
        raise SystemExit('P12 private namespace/FRR cleanup incomplete')
    print('P12_OUTER_CLEANUP=PASS', flush=True)
    raise SystemExit(result)


if __name__ == '__main__':
    main()
