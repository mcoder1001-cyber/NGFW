#!/usr/bin/env python3
"""Send one classified answer to this task's existing SSH stdin pipe only."""
import json
import os
from pathlib import Path
import shutil
import stat
import sys
import time

TASK = Path('/root/ngfw-wt/hardware-211-20261010/docs/status/tasks')
PRIVATE = Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211')
TRANSCRIPT = PRIVATE / 'repair-interactive-transcript.raw'
DRIVER = TASK / 'hardware-211-20261010-fsck-session.py'
EXPECTED_CHILD = [
    'ssh', '-tt', '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes',
    '-o', 'HostKeyAlias=172.30.110.211', '-o', 'ConnectTimeout=10', '-p',
    '2222', 'root@172.30.110.211',
    '/bin/bash --noprofile --norc /root/recovery-private/repair-wrapper.sh --repair',
]


def command(pid):
    return (Path('/proc') / str(pid) / 'cmdline').read_bytes().rstrip(b'\0').decode().split('\0')


def parent(pid):
    data = (Path('/proc') / str(pid) / 'status').read_text()
    return int(next(line.split()[1] for line in data.splitlines() if line.startswith('PPid:')))


def identity(pid):
    proc = Path('/proc') / str(pid)
    fields = (proc / 'stat').read_text().rsplit(')', 1)[1].split()
    return {'pid': pid, 'parent': parent(pid), 'command': command(pid),
            'exe': str((proc / 'exe').resolve(strict=True)),
            'start_ticks': fields[19], 'uid': proc.stat().st_uid}


assert len(sys.argv) == 2 and sys.argv[1] in ('probe', 'y', 'n')
assert os.geteuid() == 0
os.umask(0o077)
assert stat.S_IMODE(PRIVATE.stat().st_mode) == 0o700 and PRIVATE.stat().st_uid == 0
assert TRANSCRIPT.is_file() and not TRANSCRIPT.is_symlink()
assert stat.S_IMODE(TRANSCRIPT.stat().st_mode) == 0o600
drivers = []
for entry in Path('/proc').iterdir():
    if not entry.name.isdecimal():
        continue
    try:
        argv = command(int(entry.name))
        cwd = (entry / 'cwd').resolve(strict=True)
    except (FileNotFoundError, ProcessLookupError, PermissionError):
        continue
    if argv == ['python3', str(DRIVER.relative_to(TASK.parent.parent.parent))] and cwd == TASK.parent.parent.parent:
        drivers.append(int(entry.name))
assert len(drivers) == 1, 'unique owned live driver required'
driver = drivers[0]
driver_identity = identity(driver)
assert driver_identity['uid'] == 0
assert driver_identity['exe'] == str(Path(shutil.which('python3')).resolve(strict=True))
assert any(entry.stat().st_dev == TRANSCRIPT.stat().st_dev and entry.stat().st_ino == TRANSCRIPT.stat().st_ino
           for entry in (Path('/proc') / str(driver) / 'fd').iterdir())
children = []
for entry in Path('/proc').iterdir():
    if not entry.name.isdecimal():
        continue
    try:
        pid = int(entry.name)
        if parent(pid) == driver and command(pid) == EXPECTED_CHILD:
            children.append(pid)
    except (FileNotFoundError, ProcessLookupError, PermissionError):
        continue
assert len(children) == 1, 'unique exact task SSH child required'
child = children[0]
child_identity = identity(child)
assert child_identity['uid'] == 0
assert child_identity['exe'] == str(Path(shutil.which('ssh')).resolve(strict=True))
child_input = Path('/proc') / str(child) / 'fd/0'
input_stat = child_input.stat()
assert stat.S_ISFIFO(input_stat.st_mode), 'SSH input must be a pipe'
writes = []
for entry in (Path('/proc') / str(driver) / 'fd').iterdir():
    value = entry.stat()
    info = (entry.parent.parent / 'fdinfo' / entry.name).read_text()
    flags = int(next(line.split()[1] for line in info.splitlines() if line.startswith('flags:')), 8)
    if stat.S_ISFIFO(value.st_mode) and value.st_dev == input_stat.st_dev and value.st_ino == input_stat.st_ino and flags & os.O_ACCMODE == os.O_WRONLY:
        writes.append(entry)
assert len(writes) == 1, 'unique owned driver output pipe required'
fd = os.open(writes[0], os.O_WRONLY | os.O_CLOEXEC | os.O_NONBLOCK)
try:
    value = os.fstat(fd)
    assert (value.st_dev, value.st_ino) == (input_stat.st_dev, input_stat.st_ino)
    assert identity(driver) == driver_identity and identity(child) == child_identity
    current_input = child_input.stat()
    assert (current_input.st_dev, current_input.st_ino) == (value.st_dev, value.st_ino)
    before = TRANSCRIPT.stat().st_size
    if sys.argv[1] != 'probe':
        assert os.write(fd, sys.argv[1].encode()) == 1
finally:
    os.close(fd)
record = {'unix_time': time.time(), 'driver': driver_identity, 'ssh': child_identity,
          'pipe_inode': input_stat.st_ino, 'transcript_bytes_before': before,
          'answer': sys.argv[1], 'bytes_written': 0 if sys.argv[1] == 'probe' else 1,
          'newline_written': False}
with (PRIVATE / 'repair-single-byte-answers.jsonl').open('ab') as ledger:
    ledger.write((json.dumps(record, sort_keys=True) + '\n').encode())
    ledger.flush()
    os.fsync(ledger.fileno())
print('IDENTITY_PROBE_PASS' if sys.argv[1] == 'probe' else 'EXPLICIT_SINGLE_BYTE_SENT ' + sys.argv[1])
