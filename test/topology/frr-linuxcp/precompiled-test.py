#!/usr/bin/env python3
"""Run a fixed current-source routing acceptance binary, refusing empty/skipped runs."""
import os
from pathlib import Path
import stat
import subprocess
import sys


def protected_executable(value):
    path = Path(value)
    if not path.is_absolute() or path != path.resolve(strict=True):
        raise SystemExit('routing binary must be an exact absolute path')
    for parent in [path, *path.parents]:
        info = parent.lstat()
        if info.st_uid != 0 or info.st_mode & 0o022 or stat.S_ISLNK(info.st_mode):
            raise SystemExit('routing binary protection refused')
    info = path.stat()
    if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or not os.access(path, os.X_OK):
        raise SystemExit('routing binary executable refused')
    return path


def main():
    names = {'p12': 'TestP12TopologyOnHost', 'ospf': 'TestOSPFTopologyOnHost'}
    if len(sys.argv) != 2 or sys.argv[1] not in names or os.geteuid() != 0:
        raise SystemExit('fixed root routing suite required')
    name = names[sys.argv[1]]
    binary = protected_executable(os.environ['NGFW_ROUTING_TEST_BIN'])
    preflight = protected_executable(os.environ['NGFW_ROUTING_PREFLIGHT_BIN'])
    env = dict(os.environ, NGFW_ROUTING_PREFLIGHT_BIN=str(preflight))
    root = Path(__file__).resolve().parents[3]
    command = [str(binary), '-test.run=^' + name + '$', '-test.count=1', '-test.v', '-test.timeout=20m']
    result = subprocess.run(command, cwd=root / 'apps/agent/internal/agent', env=env,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    print(result.stdout, end='', flush=True)
    lines = result.stdout.splitlines()
    if (not any(line == '=== RUN   ' + name for line in lines)
            or not any(line.startswith('--- PASS: ' + name + ' (') for line in lines)
            or any(line.lstrip().startswith('--- SKIP:') for line in lines)):
        raise SystemExit('required routing acceptance did not pass without skips')
    raise SystemExit(result.returncode)


if __name__ == '__main__':
    main()
