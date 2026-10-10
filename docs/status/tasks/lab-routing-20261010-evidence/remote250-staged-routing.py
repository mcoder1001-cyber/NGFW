#!/usr/bin/env python3
"""Supplemental250 routing proof with source-paired FRR in a private mount tree."""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import sys

TASK = Path('/root/ngfw-routing250-20261010')
ROOT = TASK / 'source'
STAGE = TASK / 'frr-stage'
ARCHIVE_SHA = '77e747b57f2027064279a8f463c87ea33add27964ac049956f697ebae08ff67b'
TARGETS = ['/usr/lib/frr', '/usr/lib/x86_64-linux-gnu/frr', '/usr/bin/vtysh']


def verify_stage():
    if hashlib.sha256((TASK / 'frr-10.7.1-complete.tar').read_bytes()).hexdigest() != ARCHIVE_SHA:
        raise SystemExit('source-paired FRR archive changed')
    for directory in [TASK, STAGE]:
        for path in [directory, *directory.parents]:
            info = path.lstat()
            if info.st_uid != 0 or info.st_mode & 0o022 or stat.S_ISLNK(info.st_mode):
                raise SystemExit('FRR staging directory protection refused')
    manifest = json.loads((TASK / 'frr-stage-manifest.json').read_text())
    observed = {str(path.relative_to(STAGE)) for path in STAGE.rglob('*') if path.is_symlink() or path.is_file()}
    if observed != set(manifest):
        raise SystemExit('FRR staging file membership changed')
    for relative, expected in manifest.items():
        path = STAGE / relative
        if not path.resolve(strict=True).is_relative_to(STAGE):
            raise SystemExit('staged artifact escapes own tree')
        info = path.lstat()
        if info.st_uid != 0 or (not stat.S_ISLNK(info.st_mode) and info.st_mode & 0o022):
            raise SystemExit('FRR artifact owner/mode refused')
        if 'link' in expected:
            if not path.is_symlink() or os.readlink(path) != expected['link']:
                raise SystemExit('FRR artifact link changed')
        elif not stat.S_ISREG(info.st_mode) or hashlib.sha256(path.read_bytes()).hexdigest() != expected['sha256']:
            raise SystemExit('FRR artifact bytes changed')
    print('SOURCE_PAIRED_FRR_ARTIFACTS=' + str(len(manifest)), flush=True)


def host_snapshot():
    commands = [['systemctl', 'show', 'vpp', 'frr', '-p', 'MainPID', '-p', 'NRestarts'],
                ['ip', '-4', 'route', 'show'],
                ['vppctl', '-s', '/run/vpp/cli.sock', 'show', 'lcp'],
                ['sha256sum', '/usr/lib/frr/zebra', '/usr/bin/vtysh', '/usr/lib/x86_64-linux-gnu/frr/libfrr.so.0']]
    return [subprocess.check_output(command, text=True) for command in commands]


def main():
    if len(sys.argv) not in (2, 3) or sys.argv[1] not in ('p12', 'ospf') or os.geteuid() != 0:
        raise SystemExit('fixed root routing suite required')
    suite = sys.argv[1]
    if len(sys.argv) == 3:
        if sys.argv[2] != '--child':
            raise SystemExit('fixed mount child required')
        fd = int(os.environ['NGFW_ROUTING_ORIGINAL_MOUNT_FD'])
        if not 3 <= fd <= 1024 or fcntl.ioctl(fd, 0xb703) != 0x20000:
            raise SystemExit('original mount handle refused')
        original = os.fstat(fd)
        current = os.stat('/proc/self/ns/mnt')
        if (original.st_dev, original.st_ino) == (current.st_dev, current.st_ino):
            raise SystemExit('FRR stage refused in original mount namespace')
        os.close(fd)
        verify_stage()
        subprocess.run(['mount', '--make-rprivate', '/'], check=True)
        # ExecRunner deliberately supplies a sanitized environment. Put exact
        # dependencies in the normal loader path through a read-only union,
        # preserving both that environment policy and the original host files.
        libraries = '/usr/lib/x86_64-linux-gnu'
        base = TASK / 'base-libs'
        base.mkdir(mode=0o700, exist_ok=True)
        info = base.lstat()
        if (not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022
                or base != base.resolve(strict=True)):
            raise SystemExit('private base library directory refused')
        subprocess.run(['mount', '--bind', libraries, str(base)], check=True)
        subprocess.run(['mount', '-o', 'remount,bind,ro', str(base)], check=True)
        dependencies = STAGE / 'extra-libs'
        subprocess.run(['mount', '-t', 'overlay', 'overlay', '-o',
                        'ro,lowerdir=' + str(dependencies) + ':' + str(base), libraries], check=True)
        for target in TARGETS:
            source = STAGE / target.removeprefix('/')
            subprocess.run(['mount', '--bind', str(source), target], check=True)
            subprocess.run(['mount', '-o', 'remount,bind,ro', target], check=True)
        print(subprocess.check_output(['/usr/lib/frr/zebra', '--version'], text=True).splitlines()[0], flush=True)
        slot = '14' if suite == 'p12' else '6'
        env = dict(os.environ)
        for line in subprocess.check_output([str(ROOT / 'tools/lab'), 'env', slot], text=True).splitlines():
            if line.startswith('export '):
                import shlex
                key, value = line[7:].split('=', 1)
                env[key] = shlex.split(value)[0]
        env.update(NGFW_ROUTING_TEST_BIN=str(TASK / 'bin/agent.test'),
                   NGFW_ROUTING_PREFLIGHT_BIN=str(TASK / 'bin/ngfw-vpp-preflight'),
                   TMPDIR='/run/w' + slot, GOTMPDIR='/run/w' + slot)
        script = ROOT / 'test/topology' / ('frr-linuxcp' if suite == 'p12' else 'ospf') / 'private-fib.py'
        command = ['python3', str(script)]
        if suite == 'ospf':
            command = ['flock', '-n', '/run/lock/ngfw-acceptance-slot6.lock'] + command
        result = subprocess.call(command, cwd=ROOT, env=env)
        verify_stage()
        raise SystemExit(result)
    verify_stage()
    before = host_snapshot()
    print('HOST_BEFORE=' + json.dumps(before), flush=True)
    fd = os.open('/proc/self/ns/mnt', os.O_RDONLY)
    env = dict(os.environ, NGFW_ROUTING_ORIGINAL_MOUNT_FD=str(fd))
    try:
        result = subprocess.call(['unshare', '--mount', '--propagation', 'private', '--',
                                  'python3', str(Path(__file__).resolve()), suite, '--child'],
                                 env=env, pass_fds=(fd,))
    finally:
        os.close(fd)
        after = host_snapshot()
        print('HOST_AFTER=' + json.dumps(after), flush=True)
        if before != after:
            raise SystemExit('installed FRR/shared VPP/root routes changed')
    print('HOST_INSTALLED_FRR_VPP_UNCHANGED=PASS', flush=True)
    raise SystemExit(result)


if __name__ == '__main__':
    main()
