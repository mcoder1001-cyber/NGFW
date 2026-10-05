#!/usr/bin/env python3
"""Install only in disposable appliance VMs; guarded D-012 post-handover fault."""
import argparse
import json
import os
from pathlib import Path
import signal
import stat
import subprocess


def verify(root, expected_boot, expected_pid, nonce, virtualized):
    marker = root / 'etc/ngfw/ha-acceptance-lab.json'
    for parent in (marker.parent, marker.parent.parent):
        info = parent.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
            raise RuntimeError('root-owned lab marker parents required')
    info = marker.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
        raise RuntimeError('root-owned immutable lab marker required')
    proof = json.loads(marker.read_text())
    boot = (root / 'proc/sys/kernel/random/boot_id').read_text().strip()
    if not virtualized or boot != expected_boot or proof != {'bootId': boot, 'handoverNonce': nonce}:
        raise RuntimeError('isolated VM identity/handover mismatch')
    if expected_pid <= 1:
        raise RuntimeError('invalid VPP PID')
    process = root / 'proc' / str(expected_pid)
    if (process / 'exe').resolve() != root / 'usr/bin/vpp':
        raise RuntimeError('PID is not the appliance VPP executable')
    if not any(line.endswith('/vpp.service') for line in (process / 'cgroup').read_text().splitlines()):
        raise RuntimeError('PID is not in the appliance VPP unit')
    fields = (process / 'stat').read_text().rsplit(')', 1)[1].split()
    return fields[19]  # field22 starttime, independently rechecked before signaling


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--boot-id', required=True)
    parser.add_argument('--pid', required=True, type=int)
    parser.add_argument('--handover-nonce', required=True)
    parser.add_argument('--after-handover', required=True, action='store_true')
    args = parser.parse_args()
    if os.geteuid() != 0 or not hasattr(os, 'pidfd_open'):
        raise RuntimeError('root and pidfd support required')
    virtualized = subprocess.run(['systemd-detect-virt', '--vm', '--quiet'],
        check=False, timeout=3).returncode == 0
    root = Path('/')
    start = verify(root, args.boot_id, args.pid, args.handover_nonce, virtualized)
    descriptor = os.pidfd_open(args.pid)
    try:
        if verify(root, args.boot_id, args.pid, args.handover_nonce, virtualized) != start:
            raise RuntimeError('VPP identity changed before fault injection')
        signal.pidfd_send_signal(descriptor, signal.SIGKILL)
    finally:
        os.close(descriptor)
    print(json.dumps({'bootId': args.boot_id, 'pid': args.pid, 'fault': 'VPP SIGKILL'}))


if __name__ == '__main__':
    main()
