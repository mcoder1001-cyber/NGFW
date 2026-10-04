#!/usr/bin/env python3
"""Run unchanged OSPF root-mode FIB proof inside a private network namespace."""
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[3]


def main():
    if '--child' not in sys.argv:
        env = dict(os.environ)
        env['NGFW_FIB_HOST_NETNS'] = os.readlink('/proc/self/ns/net')
        before = subprocess.check_output(['systemctl', 'show', 'vpp', '-p', 'MainPID', '-p', 'NRestarts'])
        print('HOST_NETNS=' + env['NGFW_FIB_HOST_NETNS'], flush=True)
        rc = subprocess.call(['unshare', '--net', '--mount', '--propagation', 'private',
                              sys.executable, str(Path(__file__).resolve()), '--child'], env=env)
        after = subprocess.check_output(['systemctl', 'show', 'vpp', '-p', 'MainPID', '-p', 'NRestarts'])
        if before != after:
            raise SystemExit('shared VPP changed during private FIB proof')
        print('SHARED_VPP_UNCHANGED=' + after.decode().strip().replace('\n', ','), flush=True)
        raise SystemExit(rc)
    if os.environ.get('NGFW_SLOT') != '6':
        raise SystemExit('slot 6 required')
    private = os.readlink('/proc/self/ns/net')
    if private == os.environ['NGFW_FIB_HOST_NETNS']:
        raise SystemExit('refusing FIB test in host network namespace')
    print('PRIVATE_FIB_NETNS=' + private, flush=True)
    runtime = ROOT / '.scratch' / ('ospf-fib-' + str(os.getpid()))
    for directory in ['netns', 'frr']:
        source = runtime / directory
        source.mkdir(mode=0o755, parents=True)
        subprocess.run(['mount', '--bind', str(source), '/run/' + directory], check=True)
    subprocess.run(['ip', 'link', 'set', 'lo', 'up'], check=True)
    links = json.loads(subprocess.check_output(['ip', '-j', 'link', 'show']))
    # Loading IPv6 tunnel support can create this down fallback device in every
    # new namespace; neither it nor loopback connects to any host NIC.
    if any(link['ifname'] not in {'lo', 'ip6tnl0'} for link in links):
        raise SystemExit('private network namespace is not empty')
    os.environ.update(NGFW_OSPF_FIB='root', NGFW_ISOLATED_TEST_RUN='1')
    rc = subprocess.call([sys.executable, str(ROOT / 'test/topology/hardware-smoke/isolated-vpp.py'),
                          str(ROOT / 'test/topology/ospf/run.sh')], cwd=ROOT)
    print('PRIVATE_FIB_TEST_EXIT=' + str(rc), flush=True)
    raise SystemExit(rc)


if __name__ == '__main__':
    main()
